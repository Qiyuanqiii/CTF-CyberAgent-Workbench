package app

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/sandbox"
)

var sandboxPreparationIDPattern = regexp.MustCompile(`sandbox-manifest-[0-9]{14}-[a-f0-9]{12}`)
var sandboxCandidateIDPattern = regexp.MustCompile(`sandbox-candidate-[0-9]{14}-[a-f0-9]{12}`)
var sandboxExecutionIDPattern = regexp.MustCompile(`sandbox-execution-[0-9]{14}-[a-f0-9]{12}`)
var sandboxPreflightIDPattern = regexp.MustCompile(`sandbox-preflight-[0-9]{14}-[a-f0-9]{12}`)
var sandboxEvidenceIDPattern = regexp.MustCompile(`sandbox-evidence-[0-9]{14}-[a-f0-9]{12}`)
var sandboxOutputSimulationIDPattern = regexp.MustCompile(`sandbox-output-sim-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerObservationIDPattern = regexp.MustCompile(`sandbox-docker-observation-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerPlanIDPattern = regexp.MustCompile(`sandbox-docker-plan-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerAttemptIDPattern = regexp.MustCompile(`sandbox-docker-attempt-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerRehearsalIDPattern = regexp.MustCompile(`sandbox-docker-rehearsal-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerHostInputIntentIDPattern = regexp.MustCompile(`sandbox-docker-host-input-intent-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerHostInputHandoffIntentIDPattern = regexp.MustCompile(`sandbox-docker-host-input-handoff-intent-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerRuntimeInputPlanIDPattern = regexp.MustCompile(`sandbox-docker-runtime-input-plan-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerRuntimeInputApplicationIDPattern = regexp.MustCompile(`sandbox-docker-runtime-input-application-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerRuntimeInputResourceInspectionIDPattern = regexp.MustCompile(`sandbox-docker-runtime-input-resource-inspection-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerRuntimeInputResourceCleanupIDPattern = regexp.MustCompile(`sandbox-docker-runtime-input-resource-cleanup-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerStartGateReviewIDPattern = regexp.MustCompile(`sandbox-docker-start-gate-review-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerProductionEvidenceIDPattern = regexp.MustCompile(`sandbox-docker-production-evidence-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerProductionEvidenceAttemptIDPattern = regexp.MustCompile(`sandbox-docker-production-evidence-attempt-[0-9]{14}-[a-f0-9]{12}`)
var sandboxDockerProductionEvidenceReviewIDPattern = regexp.MustCompile(`sandbox-docker-production-evidence-review-[0-9]{14}-[a-f0-9]{12}`)

type cliDockerProductionEvidenceHarness struct{}

func (cliDockerProductionEvidenceHarness) Capture(context.Context,
	sandbox.DockerProductionEvidenceCaptureRequest,
) (sandbox.DockerProductionEvidenceObservation, error) {
	return sandbox.DockerProductionEvidenceObservation{},
		fmt.Errorf("inert collector path must not run for CLI harness")
}

func (cliDockerProductionEvidenceHarness) HarnessEnabled() bool { return true }

func (cliDockerProductionEvidenceHarness) ReconcileHarness(context.Context,
	sandbox.DockerProductionEvidenceHarnessRequest,
) (sandbox.DockerProductionEvidenceHarnessInventory, error) {
	endpoint, err := sandbox.NewDockerObservationEndpoint(
		sandbox.DockerObservationEndpointLocalUnix)
	if err != nil {
		return sandbox.DockerProductionEvidenceHarnessInventory{}, err
	}
	return sandbox.NewDockerProductionEvidenceHarnessInventory(endpoint, nil)
}

func (cliDockerProductionEvidenceHarness) CaptureHarness(_ context.Context,
	request sandbox.DockerProductionEvidenceHarnessCaptureRequest,
) (sandbox.DockerProductionEvidenceObservation, error) {
	return sandbox.NewDockerProductionEvidenceHarnessObservation(
		request.AuthorityFingerprint, strings.Repeat("9", 64))
}

func executeTestCommandWithDockerProductionEvidence(t *testing.T,
	collector sandbox.DockerProductionEvidenceCollector, args ...string,
) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := executeContextWithConfig(context.Background(), args, &out, &errOut,
		func(app *App) { app.productionEvidence = collector })
	return out.String(), errOut.String(), code
}

type cliDockerPlanObservationTransport struct {
	imageDigest string
}

func (transport cliDockerPlanObservationTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (cliDockerPlanObservationTransport) Ping(context.Context) error { return nil }

func (cliDockerPlanObservationTransport) Version(context.Context) (sandbox.DockerDaemonVersion, error) {
	return sandbox.DockerDaemonVersion{APIVersion: "1.47", MinAPIVersion: "1.24",
		EngineVersion: "27.5.1", GitCommit: "abc123", OSType: "linux",
		Architecture: "amd64"}, nil
}

func (cliDockerPlanObservationTransport) Info(context.Context) (sandbox.DockerDaemonInfo, error) {
	return sandbox.DockerDaemonInfo{ID: "private-daemon", Name: "private-host",
		DockerRootDir: "/private/docker", ServerVersion: "27.5.1", OSType: "linux",
		Architecture: "amd64", Driver: "overlay2", CgroupDriver: "systemd",
		CgroupVersion: "2", DefaultRuntime: "runc", NCPU: 8,
		MemoryBytes: 8 * 1024 * 1024 * 1024, PidsLimit: true,
		SecurityOptions: []string{"name=rootless"}}, nil
}

func (transport cliDockerPlanObservationTransport) InspectImage(context.Context,
	string,
) (sandbox.DockerImageInspection, error) {
	return sandbox.DockerImageInspection{ID: "sha256:" + strings.Repeat("f", 64),
		RepoDigests: []string{"example.invalid/cli@" + transport.imageDigest},
		OSType:      "linux", Architecture: "amd64", SizeBytes: 4096,
		User: "root", RootFSType: "layers", GraphDriver: "overlay2"}, nil
}

func executeTestCommandWithDockerObserver(t *testing.T, observer sandbox.DockerProductionObserver,
	args ...string,
) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := executeContextWithConfig(context.Background(), args, &out, &errOut, func(app *App) {
		app.dockerObserver = observer
	})
	return out.String(), errOut.String(), code
}

type cliDockerWriteTransport struct {
	calls int
}

type cliDockerHostInputStager struct {
	probeCalls   int
	stageCalls   int
	captureCalls int
	lastBundle   *cliHostInputBundle
}

type cliHostInputBundle struct {
	*bytes.Reader
	report sandbox.HostInputBundleReport
	closed bool
}

func (bundle *cliHostInputBundle) Report() sandbox.HostInputBundleReport { return bundle.report }

func (bundle *cliHostInputBundle) Close() error {
	bundle.closed = true
	return nil
}

type cliDockerHostInputHandoffTransport struct {
	calls int
}

type cliDockerRuntimeInputApplicationTransport struct {
	calls int
}

type cliDockerRuntimeInputResourceInspector struct {
	calls int
}

func (transport *cliDockerRuntimeInputResourceInspector) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (transport *cliDockerRuntimeInputResourceInspector) Inspect(_ context.Context,
	descriptor sandbox.DockerRuntimeInputResourceDescriptor,
) (sandbox.DockerRuntimeInputResourceObservation, error) {
	transport.calls++
	return sandbox.DockerRuntimeInputResourceObservation{
		EndpointClass: transport.Endpoint().Class, EndpointFingerprint: transport.Endpoint().Fingerprint,
		TargetState:      sandbox.DockerRuntimeInputResourceTargetOwned,
		OwnedVolumeCount: len(descriptor.Mounts), DaemonReadCount: len(descriptor.Mounts) + 1,
		ObservedAt: time.Now().UTC(),
	}, nil
}

type cliDockerRuntimeInputResourceCleanupTransport struct {
	calls int
}

func (transport *cliDockerRuntimeInputResourceCleanupTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (transport *cliDockerRuntimeInputResourceCleanupTransport) Cleanup(_ context.Context,
	intent sandbox.DockerRuntimeInputResourceCleanupIntent,
	lease sandbox.DockerRuntimeInputResourceCleanupLease,
	descriptor sandbox.DockerRuntimeInputResourceDescriptor,
) (sandbox.DockerRuntimeInputResourceCleanupResult, error) {
	transport.calls++
	total := len(descriptor.Mounts) + 1
	return sandbox.NewDockerRuntimeInputResourceCleanupResult(
		fmt.Sprintf("cli-runtime-input-resource-cleanup-result-%d", transport.calls),
		intent, lease, descriptor, total, 0, total, 2*total, total, time.Now().UTC())
}

func (transport *cliDockerRuntimeInputApplicationTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (transport *cliDockerRuntimeInputApplicationTransport) Apply(_ context.Context,
	intent sandbox.DockerRuntimeInputApplicationIntent,
	lease sandbox.DockerRuntimeInputApplicationLease,
	request sandbox.DockerRuntimeInputApplicationRequest,
) (sandbox.DockerRuntimeInputApplicationResult, error) {
	transport.calls++
	count := len(request.Mounts)
	return sandbox.NewDockerRuntimeInputApplicationResult(
		fmt.Sprintf("cli-runtime-input-result-%d", transport.calls), intent, lease, request,
		strings.Repeat("b", 64), 3+5*count, 1+4*count, 0, time.Now().UTC())
}

func (stager *cliDockerHostInputStager) Probe(context.Context, string) error {
	stager.probeCalls++
	return nil
}

func (stager *cliDockerHostInputStager) Stage(_ context.Context,
	request sandbox.HostInputBundleRequest,
) (sandbox.HostInputBundleReport, error) {
	stager.stageCalls++
	_, report, err := stager.bundle(request)
	return report, err
}

func (stager *cliDockerHostInputStager) Capture(_ context.Context,
	request sandbox.HostInputBundleRequest,
) (sandbox.HostInputBundle, error) {
	stager.captureCalls++
	data, report, err := stager.bundle(request)
	if err != nil {
		return nil, err
	}
	bundle := &cliHostInputBundle{Reader: bytes.NewReader(data), report: report}
	stager.lastBundle = bundle
	return bundle, nil
}

func (stager *cliDockerHostInputStager) bundle(
	request sandbox.HostInputBundleRequest,
) ([]byte, sandbox.HostInputBundleReport, error) {
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	sourceParts := []string{"sandbox_host_input_source_snapshot.v1",
		strconv.Itoa(request.ReadOnlyMountCount())}
	mountOrdinal := 0
	for _, mount := range request.Manifest.Mounts {
		if mount.Access != sandbox.MountReadOnly {
			continue
		}
		mountOrdinal++
		name := fmt.Sprintf("mounts/%03d", mountOrdinal)
		header := &tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o555,
			Uid: 65532, Gid: 65532, ModTime: time.Unix(0, 0).UTC(),
			AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(),
			Format: tar.FormatPAX}
		if err := writer.WriteHeader(header); err != nil {
			return nil, sandbox.HostInputBundleReport{}, err
		}
		sourceParts = append(sourceParts,
			cliRuntimeInputFingerprint("sandbox_host_input_archive_path.v1", name),
			strconv.Itoa(int(tar.TypeDir)), "0",
			cliRuntimeInputFingerprint("sandbox_host_input_directory.v1", name))
	}
	for index, artifact := range request.Artifacts {
		content := []byte(artifact.Content)
		header := &tar.Header{Name: fmt.Sprintf("artifacts/%03d", index+1),
			Typeflag: tar.TypeReg, Mode: 0o444, Size: int64(len(content)),
			Uid: 65532, Gid: 65532, ModTime: time.Unix(0, 0).UTC(),
			AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(),
			Format: tar.FormatPAX}
		if err := writer.WriteHeader(header); err != nil {
			return nil, sandbox.HostInputBundleReport{}, err
		}
		if _, err := writer.Write(content); err != nil {
			return nil, sandbox.HostInputBundleReport{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, sandbox.HostInputBundleReport{}, err
	}
	digest := sha256.Sum256(output.Bytes())
	report, err := sandbox.NewHostInputBundleReport(sandbox.HostInputBundleMeasurements{
		ReadOnlyMountCount: request.ReadOnlyMountCount(), ArtifactCount: len(request.Artifacts),
		DirectoryCount: request.ReadOnlyMountCount(), ArtifactBytes: request.ArtifactBytes(),
		BundleBytes:           int64(output.Len()),
		SourceSnapshotDigest:  cliRuntimeInputFingerprint(sourceParts...),
		ArtifactPayloadDigest: request.ArtifactPayloadDigest(),
		BundleDigest:          hex.EncodeToString(digest[:]),
	}, time.Now().UTC())
	return append([]byte(nil), output.Bytes()...), report, err
}

func cliRuntimeInputFingerprint(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		value := []byte(part)
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{'|'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (transport *cliDockerHostInputHandoffTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (transport *cliDockerHostInputHandoffTransport) Handoff(_ context.Context,
	request sandbox.DockerHostInputHandoffRequest, _ sandbox.HostInputBundle,
) (sandbox.DockerHostInputHandoffResult, error) {
	transport.calls++
	return sandbox.NewDockerHostInputHandoffResult(transport.Endpoint(), request,
		strings.Repeat("e", 64), 9, 8, 0)
}

func (transport *cliDockerWriteTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (transport *cliDockerWriteTransport) Rehearse(_ context.Context,
	request sandbox.DockerContainerWriteRequest,
) (sandbox.DockerContainerWriteResult, error) {
	transport.calls++
	return sandbox.NewDockerContainerWriteResult(transport.Endpoint(), request,
		strings.Repeat("c", 64), 0)
}

func (transport *cliDockerWriteTransport) Stage(_ context.Context,
	request sandbox.DockerContainerWriteRequest,
) (sandbox.DockerContainerStageResult, error) {
	transport.calls++
	return sandbox.NewDockerContainerStageResult(transport.Endpoint(), request,
		strings.Repeat("c", 64), false)
}

func (transport *cliDockerWriteTransport) Cleanup(_ context.Context,
	request sandbox.DockerContainerWriteRequest, stage sandbox.DockerContainerStageResult,
) (sandbox.DockerContainerCleanupResult, error) {
	return sandbox.NewDockerContainerCleanupResult(transport.Endpoint(), request, stage, true)
}

func executeTestCommandWithDockerWriteTransport(t *testing.T,
	transport sandbox.DockerContainerWriteTransport, args ...string,
) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := executeContextWithConfig(context.Background(), args, &out, &errOut, func(app *App) {
		app.dockerWriteTransport = transport
	})
	return out.String(), errOut.String(), code
}

func executeTestCommandWithDockerInputStaging(t *testing.T,
	transport sandbox.DockerContainerWriteTransport, stager sandbox.DockerHostInputStager,
	args ...string,
) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := executeContextWithConfig(context.Background(), args, &out, &errOut, func(app *App) {
		app.dockerWriteTransport = transport
		app.hostInputStager = stager
	})
	return out.String(), errOut.String(), code
}

func executeTestCommandWithDockerInputHandoff(t *testing.T,
	writeTransport sandbox.DockerContainerWriteTransport,
	stager sandbox.DockerHostInputStager,
	handoff sandbox.DockerHostInputHandoffTransport,
	args ...string,
) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := executeContextWithConfig(context.Background(), args, &out, &errOut, func(app *App) {
		app.dockerWriteTransport = writeTransport
		app.hostInputStager = stager
		app.hostInputHandoff = handoff
	})
	return out.String(), errOut.String(), code
}

func executeTestCommandWithDockerRuntimeInput(t *testing.T,
	writeTransport sandbox.DockerContainerWriteTransport,
	stager sandbox.DockerHostInputStager,
	runtimeTransport sandbox.DockerRuntimeInputApplicationTransport,
	args ...string,
) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := executeContextWithConfig(context.Background(), args, &out, &errOut, func(app *App) {
		app.dockerWriteTransport = writeTransport
		app.hostInputStager = stager
		app.runtimeInputApply = runtimeTransport
	})
	return out.String(), errOut.String(), code
}

func executeTestCommandWithDockerRuntimeResources(t *testing.T,
	inspector sandbox.DockerRuntimeInputResourceInspector,
	cleanup sandbox.DockerRuntimeInputResourceCleanupTransport,
	args ...string,
) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := executeContextWithConfig(context.Background(), args, &out, &errOut, func(app *App) {
		app.runtimeResourceRead = inspector
		app.runtimeResourceClean = cleanup
	})
	return out.String(), errOut.String(), code
}

func TestSandboxCLIValidatesPreparesListsAndShowsMetadataOnly(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	template, stderr, code := executeTestCommand(t, "sandbox", "template")
	if code != 0 || stderr != "" || !strings.Contains(template, `"protocol_version": "sandbox_manifest.v1"`) ||
		!strings.Contains(template, `"backend": "noop"`) {
		t.Fatalf("unexpected sandbox template output=%s stderr=%s code=%d", template, stderr, code)
	}
	manifestPath := filepath.Join(t.TempDir(), "sandbox-manifest.json")
	if err := os.WriteFile(manifestPath, []byte(template), 0o600); err != nil {
		t.Fatal(err)
	}
	validated, stderr, code := executeTestCommand(t, "sandbox", "validate", manifestPath)
	if code != 0 || stderr != "" || !strings.Contains(validated, "valid: true") ||
		!strings.Contains(validated, "validator: noop") ||
		!strings.Contains(validated, "execution_authorized: false") {
		t.Fatalf("unexpected sandbox validation output=%s stderr=%s code=%d", validated, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "sandbox-demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "sandbox cli lifecycle",
		"--workspace", "sandbox-demo", "--profile", "code")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("missing Run id: %s", created)
	}
	prepared, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-operation-one")
	if code != 0 || stderr != "" || !strings.Contains(prepared, "policy_allowed: true") ||
		!strings.Contains(prepared, "approval_status: not_required") ||
		!strings.Contains(prepared, "execution_authorized: false") ||
		strings.Contains(prepared, "go test") {
		t.Fatalf("unexpected sandbox preparation output=%s stderr=%s code=%d", prepared, stderr, code)
	}
	preparationID := sandboxPreparationIDPattern.FindString(prepared)
	if preparationID == "" {
		t.Fatalf("missing sandbox preparation id: %s", prepared)
	}
	replayed, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-operation-one")
	if code != 0 || !strings.Contains(replayed, "preparation: "+preparationID) ||
		!strings.Contains(replayed, "replayed: true") {
		t.Fatalf("sandbox CLI replay failed output=%s stderr=%s code=%d", replayed, stderr, code)
	}
	listed, stderr, code := executeTestCommand(t, "run", "sandbox", "list", runID)
	if code != 0 || !strings.Contains(listed, preparationID) ||
		!strings.Contains(listed, "execution_authorized=false") {
		t.Fatalf("sandbox CLI list failed output=%s stderr=%s code=%d", listed, stderr, code)
	}
	shown, stderr, code := executeTestCommand(t, "run", "sandbox", "show", preparationID)
	if code != 0 || !strings.Contains(shown, "manifest_fingerprint:") ||
		!strings.Contains(shown, "backend_enabled: false") || strings.Contains(shown, `"arguments"`) {
		t.Fatalf("sandbox CLI show failed output=%s stderr=%s code=%d", shown, stderr, code)
	}
	candidate, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate", preparationID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-candidate-one")
	if code != 0 || stderr != "" || !strings.Contains(candidate, "budget_checked: true") ||
		!strings.Contains(candidate, "lease_quiescent: true") ||
		!strings.Contains(candidate, "execution_authorized: false") {
		t.Fatalf("sandbox CLI candidate failed output=%s stderr=%s code=%d", candidate, stderr, code)
	}
	candidateID := sandboxCandidateIDPattern.FindString(candidate)
	if candidateID == "" {
		t.Fatalf("missing sandbox candidate id: %s", candidate)
	}
	candidates, stderr, code := executeTestCommand(t, "run", "sandbox", "candidates", runID)
	if code != 0 || stderr != "" || !strings.Contains(candidates, candidateID) ||
		!strings.Contains(candidates, "execution_authorized=false") {
		t.Fatalf("sandbox CLI candidate list failed output=%s stderr=%s code=%d", candidates, stderr, code)
	}
	candidateShown, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate-show", candidateID)
	if code != 0 || stderr != "" || !strings.Contains(candidateShown, "mount_binding_fingerprint:") ||
		!strings.Contains(candidateShown, "backend_enabled: false") {
		t.Fatalf("sandbox CLI candidate show failed output=%s stderr=%s code=%d", candidateShown, stderr, code)
	}
	begun, stderr, code := executeTestCommand(t, "run", "sandbox", "begin", candidateID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-begin-operation")
	if code != 0 || stderr != "" || !strings.Contains(begun, "status: prepared") ||
		!strings.Contains(begun, "lease_status: released") ||
		!strings.Contains(begun, "backend_started: false") || strings.Contains(begun, "go test") ||
		strings.Contains(begun, "lease_id") || strings.Contains(begun, "owner_id") {
		t.Fatalf("sandbox CLI begin failed output=%s stderr=%s code=%d", begun, stderr, code)
	}
	executionID := sandboxExecutionIDPattern.FindString(begun)
	if executionID == "" {
		t.Fatalf("missing sandbox execution id: %s", begun)
	}
	beginReplay, stderr, code := executeTestCommand(t, "run", "sandbox", "begin", candidateID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-begin-operation")
	if code != 0 || stderr != "" || !strings.Contains(beginReplay, "execution: "+executionID) ||
		!strings.Contains(beginReplay, "replayed: true") {
		t.Fatalf("sandbox CLI begin replay failed output=%s stderr=%s code=%d", beginReplay, stderr, code)
	}
	executions, stderr, code := executeTestCommand(t, "run", "sandbox", "executions", runID)
	if code != 0 || stderr != "" || !strings.Contains(executions, executionID) ||
		!strings.Contains(executions, "backend_started=false") {
		t.Fatalf("sandbox CLI execution list failed output=%s stderr=%s code=%d", executions, stderr, code)
	}
	preflight, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight", executionID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-preflight-operation")
	if code != 0 || stderr != "" || !strings.Contains(preflight, "status: backend_disabled") ||
		!strings.Contains(preflight, "required_checks: 16") ||
		!strings.Contains(preflight, "verified_checks: 0") ||
		!strings.Contains(preflight, "partial_failure_policy: all_or_nothing") ||
		!strings.Contains(preflight, "artifact_commit_authorized: false") ||
		strings.Contains(preflight, "locator_fingerprint") ||
		strings.Contains(preflight, "container_identity_fingerprint") ||
		strings.Contains(preflight, "go test") {
		t.Fatalf("unexpected sandbox preflight output=%s stderr=%s code=%d", preflight, stderr, code)
	}
	preflightID := sandboxPreflightIDPattern.FindString(preflight)
	if preflightID == "" {
		t.Fatalf("missing sandbox preflight id: %s", preflight)
	}
	preflightReplay, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight", executionID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-preflight-operation")
	if code != 0 || stderr != "" || !strings.Contains(preflightReplay, "preflight: "+preflightID) ||
		!strings.Contains(preflightReplay, "replayed: true") {
		t.Fatalf("sandbox preflight replay failed output=%s stderr=%s code=%d", preflightReplay, stderr, code)
	}
	preflights, stderr, code := executeTestCommand(t, "run", "sandbox", "preflights", runID)
	if code != 0 || stderr != "" || !strings.Contains(preflights, preflightID) ||
		!strings.Contains(preflights, "backend_enabled=false") {
		t.Fatalf("sandbox preflight list failed output=%s stderr=%s code=%d", preflights, stderr, code)
	}
	preflightShown, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight-show", preflightID)
	if code != 0 || stderr != "" || !strings.Contains(preflightShown, "network_default_deny") ||
		!strings.Contains(preflightShown, "kind=stdout") ||
		strings.Contains(preflightShown, "locator_fingerprint") {
		t.Fatalf("sandbox preflight show failed output=%s stderr=%s code=%d", preflightShown, stderr, code)
	}
	cancelled, stderr, code := executeTestCommand(t, "run", "sandbox", "cancel", executionID,
		"--operation-key", "sandbox-cli-cancel-operation")
	if code != 0 || stderr != "" || !strings.Contains(cancelled, "status: cancel_requested") ||
		!strings.Contains(cancelled, "cancellation_requested: true") {
		t.Fatalf("sandbox CLI cancellation failed output=%s stderr=%s code=%d", cancelled, stderr, code)
	}
	cleaned, stderr, code := executeTestCommand(t, "run", "sandbox", "cleanup", executionID,
		"--operation-key", "sandbox-cli-cleanup-operation")
	if code != 0 || stderr != "" || !strings.Contains(cleaned, "status: cleaned") ||
		!strings.Contains(cleaned, "cleanup_outcome: backend_disabled") ||
		!strings.Contains(cleaned, "input_artifacts_verified: true") ||
		!strings.Contains(cleaned, "output_artifacts: 0") {
		t.Fatalf("sandbox CLI cleanup failed output=%s stderr=%s code=%d", cleaned, stderr, code)
	}
	executionShown, stderr, code := executeTestCommand(t, "run", "sandbox", "execution-show", executionID)
	if code != 0 || stderr != "" || !strings.Contains(executionShown, "cleanup_complete: true") ||
		strings.Contains(executionShown, "lease_id") || strings.Contains(executionShown, "owner_id") {
		t.Fatalf("sandbox CLI execution show failed output=%s stderr=%s code=%d", executionShown, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "list", runID,
		"--limit", "-1"); code != 2 || !strings.Contains(stderr, "between 1 and 200") {
		t.Fatalf("sandbox CLI accepted a negative list limit: code=%d stderr=%s", code, stderr)
	}
}

func TestSandboxCLIRejectsAmbiguousManifest(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	manifest := defaultSandboxManifestTemplate()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	malformed := strings.Replace(string(encoded), `"backend":"noop"`,
		`"backend":"noop","backend":"docker"`, 1)
	path := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(path, []byte(malformed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := executeTestCommand(t, "sandbox", "validate", path); code != 2 ||
		!strings.Contains(stderr, "duplicate field") {
		t.Fatalf("ambiguous sandbox manifest returned code=%d stderr=%s", code, stderr)
	}
}

func TestSandboxCLISimulatesBackendEvidenceAndAtomicOutputWithoutExecution(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	manifest := defaultSandboxManifestTemplate()
	manifest.Backend = sandbox.BackendDocker
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "sandbox-docker-manifest.json")
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(t.TempDir(), "sandbox-output-fixture.json")
	fixture := `{"protocol_version":"sandbox_output_fixture.v1","outputs":[{"kind":"stdout","file_type":"stream","content":"ok\\n"},{"kind":"stderr","file_type":"stream","content":"API_KEY=sk-123456789012345678901234567890\\n"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "sandbox-evidence-demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "sandbox evidence simulation",
		"--workspace", "sandbox-evidence-demo", "--profile", "code")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	prepared, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "sandbox-evidence-cli-prepare")
	if code != 0 || !strings.Contains(prepared, "approval_status: required") {
		t.Fatalf("Docker preparation failed output=%s stderr=%s code=%d", prepared, stderr, code)
	}
	preparationID := sandboxPreparationIDPattern.FindString(prepared)
	requested, stderr, code := executeTestCommand(t, "run", "sandbox", "request", preparationID)
	if code != 0 {
		t.Fatalf("Sandbox approval request failed: %s", stderr)
	}
	approvalID := approvalIDPattern.FindString(requested)
	if approvalID == "" {
		t.Fatalf("missing approval id: %s", requested)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "review", preparationID,
		"--decision", "approve", "--operation-key", "sandbox-evidence-cli-review"); code != 0 {
		t.Fatalf("Sandbox approval review failed: %s", stderr)
	}
	candidate, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate", preparationID,
		"--manifest", manifestPath, "--approval", approvalID,
		"--operation-key", "sandbox-evidence-cli-candidate")
	if code != 0 {
		t.Fatalf("Sandbox candidate failed: %s", stderr)
	}
	candidateID := sandboxCandidateIDPattern.FindString(candidate)
	begun, stderr, code := executeTestCommand(t, "run", "sandbox", "begin", candidateID,
		"--manifest", manifestPath, "--operation-key", "sandbox-evidence-cli-begin")
	if code != 0 {
		t.Fatalf("Sandbox begin failed: %s", stderr)
	}
	executionID := sandboxExecutionIDPattern.FindString(begun)
	preflight, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight", executionID,
		"--manifest", manifestPath, "--operation-key", "sandbox-evidence-cli-preflight")
	if code != 0 {
		t.Fatalf("Sandbox preflight failed: %s", stderr)
	}
	preflightID := sandboxPreflightIDPattern.FindString(preflight)
	imageDigest := "sha256:" + strings.Repeat("e", 64)
	evidence, stderr, code := executeTestCommand(t, "run", "sandbox", "evidence", preflightID,
		"--manifest", manifestPath, "--image-digest", imageDigest,
		"--operation-key", "sandbox-evidence-cli-record")
	if code != 0 || stderr != "" || !strings.Contains(evidence, "trust_class: simulation_only") ||
		!strings.Contains(evidence, "simulated_satisfied: 16") ||
		!strings.Contains(evidence, "production_verified: 0") ||
		!strings.Contains(evidence, "verified_checks: 0") ||
		!strings.Contains(evidence, "backend_enabled: false") ||
		strings.Contains(evidence, "\ncontainer_id:") || strings.Contains(evidence, "go test") {
		t.Fatalf("unexpected evidence output=%s stderr=%s code=%d", evidence, stderr, code)
	}
	evidenceID := sandboxEvidenceIDPattern.FindString(evidence)
	if evidenceID == "" {
		t.Fatalf("missing evidence id: %s", evidence)
	}
	evidenceReplay, stderr, code := executeTestCommand(t, "run", "sandbox", "evidence", preflightID,
		"--manifest", manifestPath, "--image-digest", imageDigest,
		"--operation-key", "sandbox-evidence-cli-record")
	if code != 0 || stderr != "" || !strings.Contains(evidenceReplay, "evidence: "+evidenceID) ||
		!strings.Contains(evidenceReplay, "replayed: true") {
		t.Fatalf("evidence replay failed output=%s stderr=%s code=%d", evidenceReplay, stderr, code)
	}
	simulated, stderr, code := executeTestCommand(t, "run", "sandbox", "output-simulate", evidenceID,
		"--manifest", manifestPath, "--fixture", fixturePath,
		"--operation-key", "sandbox-evidence-cli-output")
	if code != 0 || stderr != "" || !strings.Contains(simulated, "status: simulation_committed") ||
		!strings.Contains(simulated, "fake_artifacts: 2") ||
		!strings.Contains(simulated, "production_artifacts: 0") ||
		!strings.Contains(simulated, "artifact_commit_authorized: false") ||
		!strings.Contains(simulated, "redacted=true") || strings.Contains(simulated, "sk-123456") ||
		strings.Contains(simulated, "locator_fingerprint") || strings.Contains(simulated, "content") {
		t.Fatalf("unexpected output simulation=%s stderr=%s code=%d", simulated, stderr, code)
	}
	simulationID := sandboxOutputSimulationIDPattern.FindString(simulated)
	if simulationID == "" {
		t.Fatalf("missing output simulation id: %s", simulated)
	}
	listed, stderr, code := executeTestCommand(t, "run", "sandbox", "evidences", runID)
	if code != 0 || stderr != "" || !strings.Contains(listed, evidenceID) ||
		!strings.Contains(listed, "production_verified=0") {
		t.Fatalf("evidence list failed output=%s stderr=%s code=%d", listed, stderr, code)
	}
	outputList, stderr, code := executeTestCommand(t, "run", "sandbox", "output-simulations", runID)
	if code != 0 || stderr != "" || !strings.Contains(outputList, simulationID) ||
		!strings.Contains(outputList, "production_artifacts=0") {
		t.Fatalf("output simulation list failed output=%s stderr=%s code=%d", outputList, stderr, code)
	}
	shown, stderr, code := executeTestCommand(t, "run", "sandbox", "output-simulation-show", simulationID)
	if code != 0 || stderr != "" || !strings.Contains(shown, "simulation_only: true") ||
		strings.Contains(shown, "sk-123456") || strings.Contains(shown, "locator_fingerprint") {
		t.Fatalf("output simulation show leaked data output=%s stderr=%s code=%d", shown, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "observe", evidenceID,
		"--simulation", simulationID, "--manifest", manifestPath,
		"--operation-key", "sandbox-docker-observe"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-readonly-probe") {
		t.Fatalf("Docker observation skipped explicit confirmation: stderr=%s code=%d", stderr, code)
	}
	emptyObservations, stderr, code := executeTestCommand(t, "run", "sandbox", "observations", runID)
	if code != 0 || stderr != "" || !strings.Contains(emptyObservations,
		"no read-only Docker observations") {
		t.Fatalf("unconfirmed probe left an observation: output=%s stderr=%s code=%d",
			emptyObservations, stderr, code)
	}
	dockerObserver := sandbox.NewReadOnlyDockerProductionObserver(
		sandbox.NewUnavailableDockerReadOnlyTransport(sandbox.DockerObservationEndpointLocalNPipe,
			sandbox.DockerObservationFailureTransportUnsupported))
	observed, stderr, code := executeTestCommandWithDockerObserver(t, dockerObserver,
		"run", "sandbox", "observe", evidenceID,
		"--simulation", simulationID, "--manifest", manifestPath,
		"--operation-key", "sandbox-docker-observe", "--confirm-readonly-probe")
	if code != 0 || stderr != "" ||
		(!strings.Contains(observed, "status: daemon_unavailable") &&
			!strings.Contains(observed, "status: image_unavailable") &&
			!strings.Contains(observed, "status: observation_complete")) ||
		!strings.Contains(observed, "source: docker_engine_api_read_only") ||
		!strings.Contains(observed, "production_verified: false") ||
		!strings.Contains(observed, "backend_enabled: false") ||
		!strings.Contains(observed, "execution_authorized: false") ||
		strings.Contains(observed, "/var/run/docker.sock") ||
		strings.Contains(observed, "DockerRootDir") || strings.Contains(observed, "container_id") {
		t.Fatalf("unexpected read-only Docker observation output=%s stderr=%s code=%d",
			observed, stderr, code)
	}
	observationID := sandboxDockerObservationIDPattern.FindString(observed)
	if observationID == "" {
		t.Fatalf("missing Docker observation id: %s", observed)
	}
	observedReplay, stderr, code := executeTestCommandWithDockerObserver(t, dockerObserver,
		"run", "sandbox", "observe", evidenceID,
		"--simulation", simulationID, "--manifest", manifestPath,
		"--operation-key", "sandbox-docker-observe", "--confirm-readonly-probe")
	if code != 0 || stderr != "" || !strings.Contains(observedReplay,
		"observation: "+observationID) || !strings.Contains(observedReplay, "replayed: true") {
		t.Fatalf("Docker observation replay failed output=%s stderr=%s code=%d",
			observedReplay, stderr, code)
	}
	observationList, stderr, code := executeTestCommand(t, "run", "sandbox", "observations", runID)
	if code != 0 || stderr != "" || !strings.Contains(observationList, observationID) ||
		!strings.Contains(observationList, "production_verified=false") ||
		!strings.Contains(observationList, "execution_authorized=false") {
		t.Fatalf("Docker observation list failed output=%s stderr=%s code=%d",
			observationList, stderr, code)
	}
	observationShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"observation-show", observationID)
	if code != 0 || stderr != "" || !strings.Contains(observationShown, "verified_items: 0") ||
		!strings.Contains(observationShown, "private_mount_state:") ||
		strings.Contains(observationShown, "/var/run/docker.sock") ||
		strings.Contains(observationShown, "private-build-host") {
		t.Fatalf("Docker observation show leaked data output=%s stderr=%s code=%d",
			observationShown, stderr, code)
	}
}

func TestSandboxCLICompilesMetadataOnlyDockerPlanWithFakeWriteTransaction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	manifest := defaultSandboxManifestTemplate()
	manifest.Backend = sandbox.BackendDocker
	manifest.Mounts = []sandbox.Mount{
		{Source: "scripts", Target: "/workspace", Access: sandbox.MountReadOnly},
		{Source: "outputs", Target: "/output", Access: sandbox.MountReadWrite},
	}
	manifest.Output.Paths = []string{"/output/report.json"}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "sandbox-docker-plan-manifest.json")
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(t.TempDir(), "sandbox-docker-plan-output.json")
	fixture := `{"protocol_version":"sandbox_output_fixture.v1","outputs":[{"kind":"stdout","file_type":"stream","content":"ok"},{"kind":"stderr","file_type":"stream","content":"none"},{"kind":"file","file_type":"regular","content":"{}"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "docker-plan-demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "compile fake Docker plan",
		"--workspace", "docker-plan-demo", "--profile", "code")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	prepared, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "docker-plan-cli-prepare")
	if code != 0 {
		t.Fatalf("Docker plan preparation failed: output=%s stderr=%s", prepared, stderr)
	}
	preparationID := sandboxPreparationIDPattern.FindString(prepared)
	requested, stderr, code := executeTestCommand(t, "run", "sandbox", "request", preparationID)
	if code != 0 {
		t.Fatalf("Docker plan approval request failed: %s", stderr)
	}
	approvalID := approvalIDPattern.FindString(requested)
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "review", preparationID,
		"--decision", "approve", "--operation-key", "docker-plan-cli-review"); code != 0 {
		t.Fatalf("Docker plan approval failed: %s", stderr)
	}
	candidate, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate", preparationID,
		"--manifest", manifestPath, "--approval", approvalID,
		"--operation-key", "docker-plan-cli-candidate")
	if code != 0 {
		t.Fatalf("Docker plan candidate failed: %s", stderr)
	}
	candidateID := sandboxCandidateIDPattern.FindString(candidate)
	begun, stderr, code := executeTestCommand(t, "run", "sandbox", "begin", candidateID,
		"--manifest", manifestPath, "--operation-key", "docker-plan-cli-begin")
	if code != 0 {
		t.Fatalf("Docker plan begin failed: %s", stderr)
	}
	executionID := sandboxExecutionIDPattern.FindString(begun)
	preflight, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight", executionID,
		"--manifest", manifestPath, "--operation-key", "docker-plan-cli-preflight")
	if code != 0 {
		t.Fatalf("Docker plan preflight failed: %s", stderr)
	}
	preflightID := sandboxPreflightIDPattern.FindString(preflight)
	imageDigest := "sha256:" + strings.Repeat("6", 64)
	evidence, stderr, code := executeTestCommand(t, "run", "sandbox", "evidence", preflightID,
		"--manifest", manifestPath, "--image-digest", imageDigest,
		"--operation-key", "docker-plan-cli-evidence")
	if code != 0 {
		t.Fatalf("Docker plan evidence failed: %s", stderr)
	}
	evidenceID := sandboxEvidenceIDPattern.FindString(evidence)
	simulated, stderr, code := executeTestCommand(t, "run", "sandbox", "output-simulate",
		evidenceID, "--manifest", manifestPath, "--fixture", fixturePath,
		"--operation-key", "docker-plan-cli-output")
	if code != 0 {
		t.Fatalf("Docker plan output simulation failed: %s", stderr)
	}
	simulationID := sandboxOutputSimulationIDPattern.FindString(simulated)
	observer := sandbox.NewReadOnlyDockerProductionObserver(cliDockerPlanObservationTransport{
		imageDigest: imageDigest,
	})
	observed, stderr, code := executeTestCommandWithDockerObserver(t, observer,
		"run", "sandbox", "observe", evidenceID, "--simulation", simulationID,
		"--manifest", manifestPath, "--operation-key", "docker-plan-cli-observe",
		"--confirm-readonly-probe")
	if code != 0 || !strings.Contains(observed, "status: observation_complete") {
		t.Fatalf("Docker plan observation failed: output=%s stderr=%s code=%d", observed, stderr, code)
	}
	observationID := sandboxDockerObservationIDPattern.FindString(observed)
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-plan",
		observationID, "--manifest", manifestPath,
		"--operation-key", "docker-plan-cli-compile"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-fake-write") {
		t.Fatalf("Docker plan skipped explicit fake-write confirmation: stderr=%s code=%d", stderr, code)
	}
	empty, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-plans", runID)
	if code != 0 || stderr != "" || !strings.Contains(empty, "no Docker container plans") {
		t.Fatalf("unconfirmed fake write left a plan: output=%s stderr=%s code=%d", empty, stderr, code)
	}
	planned, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-plan",
		observationID, "--manifest", manifestPath,
		"--operation-key", "docker-plan-cli-compile", "--confirm-fake-write")
	if code != 0 || stderr != "" ||
		!strings.Contains(planned, "status: compiled_fake_transaction_committed") ||
		!strings.Contains(planned, "container_user: 65532:65532") ||
		!strings.Contains(planned, "read_only_rootfs: true") ||
		!strings.Contains(planned, "dedicated_output_mounts: 1") ||
		!strings.Contains(planned, "fake_write_steps: 7") ||
		!strings.Contains(planned, "daemon_writes: 0") ||
		!strings.Contains(planned, "production_submitted: false") ||
		!strings.Contains(planned, "execution_authorized: false") ||
		strings.Contains(planned, "/workspace") || strings.Contains(planned, "/output") ||
		strings.Contains(planned, "go test") || strings.Contains(planned, "private-daemon") {
		t.Fatalf("unexpected Docker plan output=%s stderr=%s code=%d", planned, stderr, code)
	}
	planID := sandboxDockerPlanIDPattern.FindString(planned)
	if planID == "" {
		t.Fatalf("missing Docker plan id: %s", planned)
	}
	replayed, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-plan",
		observationID, "--manifest", manifestPath,
		"--operation-key", "docker-plan-cli-compile", "--confirm-fake-write")
	if code != 0 || stderr != "" || !strings.Contains(replayed, "docker_plan: "+planID) ||
		!strings.Contains(replayed, "replayed: true") {
		t.Fatalf("Docker plan replay failed: output=%s stderr=%s code=%d", replayed, stderr, code)
	}
	listed, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-plans", runID)
	if code != 0 || stderr != "" || !strings.Contains(listed, planID) ||
		!strings.Contains(listed, "daemon_writes=0") ||
		!strings.Contains(listed, "production_submitted=false") {
		t.Fatalf("Docker plan list failed: output=%s stderr=%s code=%d", listed, stderr, code)
	}
	shown, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-plan-show", planID)
	if code != 0 || stderr != "" || !strings.Contains(shown, "controls:") ||
		!strings.Contains(shown, "fake_write_transaction:") ||
		strings.Contains(shown, "scripts") || strings.Contains(shown, "report.json") ||
		strings.Contains(shown, "private-host") {
		t.Fatalf("Docker plan show leaked data: output=%s stderr=%s code=%d", shown, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-rehearse",
		planID, "--manifest", manifestPath,
		"--operation-key", "docker-rehearsal-cli"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-daemon-write") {
		t.Fatalf("Docker rehearsal skipped explicit daemon-write confirmation: stderr=%s code=%d",
			stderr, code)
	}
	emptyRehearsals, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-rehearsals", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(emptyRehearsals, "no Docker container rehearsals") {
		t.Fatalf("unconfirmed daemon write left a rehearsal: output=%s stderr=%s code=%d",
			emptyRehearsals, stderr, code)
	}
	writer := &cliDockerWriteTransport{}
	hostInputStager := &cliDockerHostInputStager{}
	if _, stderr, code := executeTestCommandWithDockerInputStaging(t, writer,
		hostInputStager, "run", "sandbox", "docker-rehearse", planID,
		"--manifest", manifestPath, "--operation-key", "docker-rehearsal-cli",
		"--confirm-daemon-write", "--stage-host-inputs"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-host-input-staging") ||
		writer.calls != 0 || hostInputStager.probeCalls != 0 {
		t.Fatalf("Docker host input staging skipped separate confirmation: stderr=%s code=%d",
			stderr, code)
	}
	if _, stderr, code := executeTestCommandWithDockerInputStaging(t, writer,
		hostInputStager, "run", "sandbox", "docker-rehearse", planID,
		"--manifest", manifestPath, "--operation-key", "docker-rehearsal-cli",
		"--confirm-daemon-write", "--stage-host-inputs", "--confirm-host-input-staging",
		"--handoff-host-inputs"); code != 4 ||
		!strings.Contains(stderr, "requires staging and both explicit confirmations") ||
		writer.calls != 0 || hostInputStager.probeCalls != 0 {
		t.Fatalf("Docker host input handoff skipped separate confirmation: stderr=%s code=%d",
			stderr, code)
	}
	handoffTransport := &cliDockerHostInputHandoffTransport{}
	rehearsed, stderr, code := executeTestCommandWithDockerInputHandoff(t, writer,
		hostInputStager, handoffTransport,
		"run", "sandbox", "docker-rehearse", planID, "--manifest", manifestPath,
		"--operation-key", "docker-rehearsal-cli", "--confirm-daemon-write",
		"--stage-host-inputs", "--confirm-host-input-staging", "--handoff-host-inputs",
		"--confirm-host-input-handoff")
	if code != 0 || stderr != "" || writer.calls != 1 || hostInputStager.probeCalls != 1 ||
		hostInputStager.stageCalls != 0 || hostInputStager.captureCalls != 1 ||
		hostInputStager.lastBundle == nil || !hostInputStager.lastBundle.closed ||
		handoffTransport.calls != 1 ||
		!strings.Contains(rehearsed, "status: container_config_rehearsed_removed") ||
		!strings.Contains(rehearsed, "endpoint_class: local_unix") ||
		!strings.Contains(rehearsed, "daemon_writes: 2") ||
		!strings.Contains(rehearsed, "container_started: false") ||
		!strings.Contains(rehearsed, "process_executed: false") ||
		!strings.Contains(rehearsed, "production_execution_submitted: false") ||
		!strings.Contains(rehearsed, "execution_authorized: false") ||
		strings.Contains(rehearsed, "scripts") || strings.Contains(rehearsed, "/workspace") ||
		strings.Contains(rehearsed, strings.Repeat("c", 64)) {
		t.Fatalf("unexpected Docker rehearsal output=%s stderr=%s code=%d calls=%d",
			rehearsed, stderr, code, writer.calls)
	}
	rehearsalID := sandboxDockerRehearsalIDPattern.FindString(rehearsed)
	if rehearsalID == "" {
		t.Fatalf("missing Docker rehearsal id: %s", rehearsed)
	}
	hostInputList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-host-inputs", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(hostInputList, "status=host_inputs_descriptor_sealed") ||
		!strings.Contains(hostInputList, "daemon_consumed=false") ||
		!strings.Contains(hostInputList, "process_executed=false") ||
		strings.Contains(hostInputList, "scripts") || strings.Contains(hostInputList, home) {
		t.Fatalf("Docker host input list leaked data: output=%s stderr=%s code=%d",
			hostInputList, stderr, code)
	}
	hostInputIntentID := sandboxDockerHostInputIntentIDPattern.FindString(hostInputList)
	if hostInputIntentID == "" {
		t.Fatalf("missing Docker host input intent id: %s", hostInputList)
	}
	hostInputShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-host-input-show", hostInputIntentID)
	if code != 0 || stderr != "" ||
		!strings.Contains(hostInputShown, "descriptor_pinned: true") ||
		!strings.Contains(hostInputShown, "kernel_sealed: true") ||
		!strings.Contains(hostInputShown, "raw_content_persisted: false") ||
		!strings.Contains(hostInputShown, "daemon_consumed: false") ||
		!strings.Contains(hostInputShown, "execution_authorized: false") ||
		strings.Contains(hostInputShown, "scripts") || strings.Contains(hostInputShown, home) {
		t.Fatalf("Docker host input show leaked data: output=%s stderr=%s code=%d",
			hostInputShown, stderr, code)
	}
	handoffList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-host-input-handoffs", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(handoffList, "status=daemon_handoff_cleaned") ||
		!strings.Contains(handoffList, "daemon_reads=9") ||
		!strings.Contains(handoffList, "daemon_writes=8") ||
		!strings.Contains(handoffList, "daemon_consumed=true") ||
		!strings.Contains(handoffList, "process_executed=false") ||
		strings.Contains(handoffList, "scripts") || strings.Contains(handoffList, home) {
		t.Fatalf("Docker host input handoff list leaked data: output=%s stderr=%s code=%d",
			handoffList, stderr, code)
	}
	handoffIntentID := sandboxDockerHostInputHandoffIntentIDPattern.FindString(handoffList)
	if handoffIntentID == "" {
		t.Fatalf("missing Docker host input handoff intent id: %s", handoffList)
	}
	handoffShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-host-input-handoff-show", handoffIntentID)
	if code != 0 || stderr != "" ||
		!strings.Contains(handoffShown, "readback_verified: true") ||
		!strings.Contains(handoffShown, "final_mount_read_only: true") ||
		!strings.Contains(handoffShown, "cleanup_confirmed: true") ||
		!strings.Contains(handoffShown, "raw_content_retained: false") ||
		!strings.Contains(handoffShown, "execution_authorized: false") ||
		strings.Contains(handoffShown, "scripts") || strings.Contains(handoffShown, home) {
		t.Fatalf("Docker host input handoff show leaked data: output=%s stderr=%s code=%d",
			handoffShown, stderr, code)
	}
	if _, stderr, code := executeTestCommandWithDockerInputStaging(t, writer,
		hostInputStager, "run", "sandbox", "docker-runtime-input-plan", handoffIntentID,
		"--manifest", manifestPath, "--operation-key", "docker-runtime-input-cli"); code != 4 || !strings.Contains(stderr, "requires --confirm-runtime-input-plan") ||
		hostInputStager.captureCalls != 1 {
		t.Fatalf("Docker runtime input plan skipped confirmation: stderr=%s code=%d", stderr, code)
	}
	emptyRuntimePlans, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-plans", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(emptyRuntimePlans, "no Docker runtime input projection plans") {
		t.Fatalf("unconfirmed runtime input plan left state: output=%s stderr=%s code=%d",
			emptyRuntimePlans, stderr, code)
	}
	runtimePlanned, stderr, code := executeTestCommandWithDockerInputStaging(t, writer,
		hostInputStager, "run", "sandbox", "docker-runtime-input-plan", handoffIntentID,
		"--manifest", manifestPath, "--operation-key", "docker-runtime-input-cli",
		"--confirm-runtime-input-plan")
	if code != 0 || stderr != "" || hostInputStager.captureCalls != 2 ||
		hostInputStager.lastBundle == nil || !hostInputStager.lastBundle.closed ||
		!strings.Contains(runtimePlanned, "status: compiled_not_applied") ||
		!strings.Contains(runtimePlanned, "operator_confirmed: true") ||
		!strings.Contains(runtimePlanned, "exact_target_binding: true") ||
		!strings.Contains(runtimePlanned, "all_volumes_read_only: true") ||
		!strings.Contains(runtimePlanned, "raw_targets_stored: false") ||
		!strings.Contains(runtimePlanned, "daemon_contacted: false") ||
		!strings.Contains(runtimePlanned, "daemon_applied: false") ||
		!strings.Contains(runtimePlanned, "process_executed: false") ||
		!strings.Contains(runtimePlanned, "execution_authorized: false") ||
		strings.Contains(runtimePlanned, "scripts") ||
		strings.Contains(runtimePlanned, "/workspace") || strings.Contains(runtimePlanned, home) ||
		strings.Contains(runtimePlanned, "cyberagent-runtime-") {
		t.Fatalf("runtime input plan leaked data or widened authority: output=%s stderr=%s code=%d",
			runtimePlanned, stderr, code)
	}
	runtimePlanID := sandboxDockerRuntimeInputPlanIDPattern.FindString(runtimePlanned)
	if runtimePlanID == "" {
		t.Fatalf("missing Docker runtime input plan id: %s", runtimePlanned)
	}
	runtimeReplay, stderr, code := executeTestCommandWithDockerInputStaging(t, writer,
		hostInputStager, "run", "sandbox", "docker-runtime-input-plan", handoffIntentID,
		"--manifest", manifestPath, "--operation-key", "docker-runtime-input-cli",
		"--confirm-runtime-input-plan")
	if code != 0 || stderr != "" || hostInputStager.captureCalls != 2 ||
		!strings.Contains(runtimeReplay, "docker_runtime_input_plan: "+runtimePlanID) ||
		!strings.Contains(runtimeReplay, "replayed: true") {
		t.Fatalf("runtime input plan replay recaptured data: output=%s stderr=%s code=%d",
			runtimeReplay, stderr, code)
	}
	runtimeList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-plans", runID)
	if code != 0 || stderr != "" || !strings.Contains(runtimeList, runtimePlanID) ||
		!strings.Contains(runtimeList, "status=compiled_not_applied") ||
		!strings.Contains(runtimeList, "daemon_applied=false") ||
		strings.Contains(runtimeList, "/workspace") || strings.Contains(runtimeList, home) {
		t.Fatalf("runtime input plan list leaked data: output=%s stderr=%s code=%d",
			runtimeList, stderr, code)
	}
	runtimeShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-plan-show", runtimePlanID)
	if code != 0 || stderr != "" || !strings.Contains(runtimeShown, "projection_items:") ||
		!strings.Contains(runtimeShown, "root_directory=true") ||
		!strings.Contains(runtimeShown, "read_only=true") ||
		strings.Contains(runtimeShown, "scripts") || strings.Contains(runtimeShown, "/workspace") ||
		strings.Contains(runtimeShown, home) || strings.Contains(runtimeShown, "cyberagent-runtime-") {
		t.Fatalf("runtime input plan show leaked data: output=%s stderr=%s code=%d",
			runtimeShown, stderr, code)
	}
	runtimeTransport := &cliDockerRuntimeInputApplicationTransport{}
	if _, stderr, code := executeTestCommandWithDockerRuntimeInput(t, writer,
		hostInputStager, runtimeTransport, "run", "sandbox", "docker-runtime-input-apply",
		runtimePlanID, "--manifest", manifestPath, "--operation-key", "runtime-input-apply-cli"); code != 4 || !strings.Contains(stderr, "requires --confirm-runtime-input-apply and --confirm-daemon-write") ||
		runtimeTransport.calls != 0 || hostInputStager.captureCalls != 2 {
		t.Fatalf("runtime input apply skipped dual confirmation: stderr=%s code=%d", stderr, code)
	}
	emptyApplications, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-applications", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(emptyApplications, "no Docker runtime input application records") {
		t.Fatalf("unconfirmed runtime input apply left state: output=%s stderr=%s code=%d",
			emptyApplications, stderr, code)
	}
	runtimeApplied, stderr, code := executeTestCommandWithDockerRuntimeInput(t, writer,
		hostInputStager, runtimeTransport, "run", "sandbox", "docker-runtime-input-apply",
		runtimePlanID, "--manifest", manifestPath, "--operation-key", "runtime-input-apply-cli",
		"--confirm-runtime-input-apply", "--confirm-daemon-write")
	if code != 0 || stderr != "" || runtimeTransport.calls != 1 ||
		hostInputStager.captureCalls != 3 ||
		!strings.Contains(runtimeApplied, "status: volumes_applied_target_never_started") ||
		!strings.Contains(runtimeApplied, "target_container_present: true") ||
		!strings.Contains(runtimeApplied, "projection_readback_verified: true") ||
		!strings.Contains(runtimeApplied, "raw_targets_stored: false") ||
		!strings.Contains(runtimeApplied, "container_started: false") ||
		!strings.Contains(runtimeApplied, "process_executed: false") ||
		!strings.Contains(runtimeApplied, "execution_authorized: false") ||
		strings.Contains(runtimeApplied, "scripts") || strings.Contains(runtimeApplied, "/workspace") ||
		strings.Contains(runtimeApplied, home) || strings.Contains(runtimeApplied, "cyberagent-runtime-") {
		t.Fatalf("runtime input apply leaked data or widened authority: output=%s stderr=%s code=%d",
			runtimeApplied, stderr, code)
	}
	runtimeApplicationID := sandboxDockerRuntimeInputApplicationIDPattern.FindString(runtimeApplied)
	if runtimeApplicationID == "" {
		t.Fatalf("missing Docker runtime input application id: %s", runtimeApplied)
	}
	runtimeApplyReplay, stderr, code := executeTestCommandWithDockerRuntimeInput(t, writer,
		hostInputStager, runtimeTransport, "run", "sandbox", "docker-runtime-input-apply",
		runtimePlanID, "--manifest", manifestPath, "--operation-key", "runtime-input-apply-cli",
		"--confirm-runtime-input-apply", "--confirm-daemon-write")
	if code != 0 || stderr != "" || runtimeTransport.calls != 1 ||
		hostInputStager.captureCalls != 3 || !strings.Contains(runtimeApplyReplay, "replayed: true") {
		t.Fatalf("runtime input apply replay touched daemon or input: output=%s stderr=%s code=%d",
			runtimeApplyReplay, stderr, code)
	}
	runtimeApplications, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-applications", runID)
	if code != 0 || stderr != "" || !strings.Contains(runtimeApplications, runtimeApplicationID) ||
		!strings.Contains(runtimeApplications, "status=volumes_applied_target_never_started") ||
		!strings.Contains(runtimeApplications, "target_present=true") ||
		strings.Contains(runtimeApplications, "/workspace") || strings.Contains(runtimeApplications, home) {
		t.Fatalf("runtime input application list leaked data: output=%s stderr=%s code=%d",
			runtimeApplications, stderr, code)
	}
	runtimeApplicationShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-application-show", runtimeApplicationID)
	if code != 0 || stderr != "" ||
		!strings.Contains(runtimeApplicationShown, "target_container_fingerprint:") ||
		!strings.Contains(runtimeApplicationShown, "lease_status: released") ||
		!strings.Contains(runtimeApplicationShown, "raw_archive_bytes_stored: false") ||
		strings.Contains(runtimeApplicationShown, "/workspace") ||
		strings.Contains(runtimeApplicationShown, home) ||
		strings.Contains(runtimeApplicationShown, "cyberagent-runtime-") {
		t.Fatalf("runtime input application show leaked data: output=%s stderr=%s code=%d",
			runtimeApplicationShown, stderr, code)
	}
	runtimeResumeReplay, stderr, code := executeTestCommandWithDockerRuntimeInput(t, writer,
		hostInputStager, runtimeTransport, "run", "sandbox", "docker-runtime-input-apply-resume",
		runtimeApplicationID, "--manifest", manifestPath, "--confirm-runtime-input-apply",
		"--confirm-daemon-write")
	if code != 0 || stderr != "" || runtimeTransport.calls != 1 ||
		hostInputStager.captureCalls != 3 || !strings.Contains(runtimeResumeReplay, "replayed: true") {
		t.Fatalf("completed runtime input resume was not metadata-only: output=%s stderr=%s code=%d",
			runtimeResumeReplay, stderr, code)
	}
	resourceInspector := &cliDockerRuntimeInputResourceInspector{}
	resourceCleanup := &cliDockerRuntimeInputResourceCleanupTransport{}
	if _, stderr, code := executeTestCommandWithDockerRuntimeResources(t, resourceInspector,
		resourceCleanup, "run", "sandbox", "docker-runtime-input-resource-inspect",
		runtimeApplicationID, "--manifest", manifestPath, "--operation-key",
		"runtime-input-resource-inspect-cli"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-readonly-probe") ||
		resourceInspector.calls != 0 || hostInputStager.captureCalls != 3 {
		t.Fatalf("runtime resource inspection skipped confirmation: stderr=%s code=%d", stderr, code)
	}
	emptyResourceInspections, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-resource-inspections", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(emptyResourceInspections, "no Docker runtime input resource inspections") {
		t.Fatalf("unconfirmed runtime resource inspection left state: output=%s stderr=%s code=%d",
			emptyResourceInspections, stderr, code)
	}
	resourceInspected, stderr, code := executeTestCommandWithDockerRuntimeResources(t,
		resourceInspector, resourceCleanup, "run", "sandbox",
		"docker-runtime-input-resource-inspect", runtimeApplicationID,
		"--manifest", manifestPath, "--operation-key", "runtime-input-resource-inspect-cli",
		"--confirm-readonly-probe")
	if code != 0 || stderr != "" || resourceInspector.calls != 1 ||
		hostInputStager.captureCalls != 3 ||
		!strings.Contains(resourceInspected, "status: exact_owned_resources_present") ||
		!strings.Contains(resourceInspected, "cleanup_eligible: true") ||
		!strings.Contains(resourceInspected, "owned_target_never_started: true") ||
		!strings.Contains(resourceInspected, "raw_resource_names_stored: false") ||
		!strings.Contains(resourceInspected, "container_started: false") ||
		!strings.Contains(resourceInspected, "execution_authorized: false") ||
		strings.Contains(resourceInspected, "scripts") ||
		strings.Contains(resourceInspected, "/workspace") ||
		strings.Contains(resourceInspected, home) ||
		strings.Contains(resourceInspected, "cyberagent-runtime-") {
		t.Fatalf("runtime resource inspection leaked data or widened authority: output=%s stderr=%s code=%d",
			resourceInspected, stderr, code)
	}
	resourceInspectionID := sandboxDockerRuntimeInputResourceInspectionIDPattern.FindString(resourceInspected)
	if resourceInspectionID == "" {
		t.Fatalf("missing runtime resource inspection id: %s", resourceInspected)
	}
	resourceInspectionReplay, stderr, code := executeTestCommandWithDockerRuntimeResources(t,
		resourceInspector, resourceCleanup, "run", "sandbox",
		"docker-runtime-input-resource-inspect", runtimeApplicationID,
		"--manifest", manifestPath, "--operation-key", "runtime-input-resource-inspect-cli",
		"--confirm-readonly-probe")
	if code != 0 || stderr != "" || resourceInspector.calls != 1 ||
		hostInputStager.captureCalls != 3 ||
		!strings.Contains(resourceInspectionReplay, "replayed: true") {
		t.Fatalf("runtime resource inspection replay contacted daemon: output=%s stderr=%s code=%d",
			resourceInspectionReplay, stderr, code)
	}
	resourceInspections, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-resource-inspections", runID)
	if code != 0 || stderr != "" || !strings.Contains(resourceInspections, resourceInspectionID) ||
		!strings.Contains(resourceInspections, "cleanup_eligible=true") ||
		strings.Contains(resourceInspections, "/workspace") || strings.Contains(resourceInspections, home) {
		t.Fatalf("runtime resource inspection list leaked data: output=%s stderr=%s code=%d",
			resourceInspections, stderr, code)
	}
	resourceInspectionShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-resource-inspection-show", resourceInspectionID)
	if code != 0 || stderr != "" ||
		!strings.Contains(resourceInspectionShown, "inspection_fingerprint:") ||
		!strings.Contains(resourceInspectionShown, "raw_container_ids_stored: false") ||
		strings.Contains(resourceInspectionShown, "/workspace") ||
		strings.Contains(resourceInspectionShown, home) ||
		strings.Contains(resourceInspectionShown, "cyberagent-runtime-") {
		t.Fatalf("runtime resource inspection show leaked data: output=%s stderr=%s code=%d",
			resourceInspectionShown, stderr, code)
	}
	if _, stderr, code := executeTestCommandWithDockerRuntimeResources(t, resourceInspector,
		resourceCleanup, "run", "sandbox", "docker-runtime-input-resource-cleanup",
		resourceInspectionID, "--manifest", manifestPath, "--operation-key",
		"runtime-input-resource-cleanup-cli"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-resource-cleanup and --confirm-daemon-write") ||
		resourceCleanup.calls != 0 {
		t.Fatalf("runtime resource cleanup skipped confirmation: stderr=%s code=%d", stderr, code)
	}
	resourceCleaned, stderr, code := executeTestCommandWithDockerRuntimeResources(t,
		resourceInspector, resourceCleanup, "run", "sandbox",
		"docker-runtime-input-resource-cleanup", resourceInspectionID,
		"--manifest", manifestPath, "--operation-key", "runtime-input-resource-cleanup-cli",
		"--confirm-resource-cleanup", "--confirm-daemon-write")
	if code != 0 || stderr != "" || resourceCleanup.calls != 1 ||
		hostInputStager.captureCalls != 3 ||
		!strings.Contains(resourceCleaned, "status: exact_owned_resources_absent") ||
		!strings.Contains(resourceCleaned, "target_absent: true") ||
		!strings.Contains(resourceCleaned, "all_volumes_absent: true") ||
		!strings.Contains(resourceCleaned, "raw_resource_names_stored: false") ||
		!strings.Contains(resourceCleaned, "container_started: false") ||
		!strings.Contains(resourceCleaned, "execution_authorized: false") ||
		strings.Contains(resourceCleaned, "scripts") || strings.Contains(resourceCleaned, home) ||
		strings.Contains(resourceCleaned, "cyberagent-runtime-") {
		t.Fatalf("runtime resource cleanup leaked data or widened authority: output=%s stderr=%s code=%d",
			resourceCleaned, stderr, code)
	}
	resourceCleanupID := sandboxDockerRuntimeInputResourceCleanupIDPattern.FindString(resourceCleaned)
	if resourceCleanupID == "" {
		t.Fatalf("missing runtime resource cleanup id: %s", resourceCleaned)
	}
	resourceCleanupReplay, stderr, code := executeTestCommandWithDockerRuntimeResources(t,
		resourceInspector, resourceCleanup, "run", "sandbox",
		"docker-runtime-input-resource-cleanup", resourceInspectionID,
		"--manifest", manifestPath, "--operation-key", "runtime-input-resource-cleanup-cli",
		"--confirm-resource-cleanup", "--confirm-daemon-write")
	if code != 0 || stderr != "" || resourceCleanup.calls != 1 ||
		!strings.Contains(resourceCleanupReplay, "replayed: true") {
		t.Fatalf("runtime resource cleanup replay contacted daemon: output=%s stderr=%s code=%d",
			resourceCleanupReplay, stderr, code)
	}
	resourceCleanups, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-resource-cleanups", runID)
	if code != 0 || stderr != "" || !strings.Contains(resourceCleanups, resourceCleanupID) ||
		!strings.Contains(resourceCleanups, "status=exact_owned_resources_absent") ||
		strings.Contains(resourceCleanups, "/workspace") || strings.Contains(resourceCleanups, home) {
		t.Fatalf("runtime resource cleanup list leaked data: output=%s stderr=%s code=%d",
			resourceCleanups, stderr, code)
	}
	resourceCleanupShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-runtime-input-resource-cleanup-show", resourceCleanupID)
	if code != 0 || stderr != "" ||
		!strings.Contains(resourceCleanupShown, "result_fingerprint:") ||
		!strings.Contains(resourceCleanupShown, "lease_status: released") ||
		!strings.Contains(resourceCleanupShown, "raw_container_ids_stored: false") ||
		strings.Contains(resourceCleanupShown, "/workspace") || strings.Contains(resourceCleanupShown, home) {
		t.Fatalf("runtime resource cleanup show leaked data: output=%s stderr=%s code=%d",
			resourceCleanupShown, stderr, code)
	}
	resourceCleanupResume, stderr, code := executeTestCommandWithDockerRuntimeResources(t,
		resourceInspector, resourceCleanup, "run", "sandbox",
		"docker-runtime-input-resource-cleanup-resume", resourceCleanupID,
		"--manifest", manifestPath, "--confirm-resource-cleanup", "--confirm-daemon-write")
	if code != 0 || stderr != "" || resourceCleanup.calls != 1 ||
		!strings.Contains(resourceCleanupResume, "replayed: true") {
		t.Fatalf("completed runtime resource cleanup resume was not metadata-only: output=%s stderr=%s code=%d",
			resourceCleanupResume, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "docker-start-gate-review",
		resourceCleanupID, "--manifest", manifestPath, "--operation-key",
		"docker-start-gate-review-cli"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-design-review") || resourceCleanup.calls != 1 {
		t.Fatalf("Docker start-gate review skipped confirmation: stderr=%s code=%d", stderr, code)
	}
	emptyStartGates, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-start-gate-reviews", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(emptyStartGates, "no Docker start-gate reviews") {
		t.Fatalf("unconfirmed start-gate review left state: output=%s stderr=%s code=%d",
			emptyStartGates, stderr, code)
	}
	startGateReviewed, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-start-gate-review", resourceCleanupID, "--manifest", manifestPath,
		"--operation-key", "docker-start-gate-review-cli", "--confirm-design-review")
	if code != 0 || stderr != "" || resourceCleanup.calls != 1 ||
		!strings.Contains(startGateReviewed, "status: blocked") ||
		!strings.Contains(startGateReviewed, "decision: deny_start") ||
		!strings.Contains(startGateReviewed, "required_checks: 16") ||
		!strings.Contains(startGateReviewed, "production_verified_checks: 0") ||
		!strings.Contains(startGateReviewed, "start_gate_passed: false") ||
		!strings.Contains(startGateReviewed, "lifecycle_implementation_present: false") ||
		!strings.Contains(startGateReviewed, "wall_clock_supervision_unimplemented") ||
		!strings.Contains(startGateReviewed, "generation_fenced_reconcile") ||
		!strings.Contains(startGateReviewed, "raw_host_paths_stored: false") ||
		strings.Contains(startGateReviewed, "/workspace") ||
		strings.Contains(startGateReviewed, home) ||
		strings.Contains(startGateReviewed, "cyberagent-runtime-") {
		t.Fatalf("Docker start-gate review leaked data or widened authority: output=%s stderr=%s code=%d",
			startGateReviewed, stderr, code)
	}
	startGateReviewID := sandboxDockerStartGateReviewIDPattern.FindString(startGateReviewed)
	if startGateReviewID == "" {
		t.Fatalf("missing Docker start-gate review id: %s", startGateReviewed)
	}
	startGateReplay, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-start-gate-review", resourceCleanupID, "--manifest", manifestPath,
		"--operation-key", "docker-start-gate-review-cli", "--confirm-design-review")
	if code != 0 || stderr != "" || resourceCleanup.calls != 1 ||
		!strings.Contains(startGateReplay, "replayed: true") {
		t.Fatalf("Docker start-gate replay was not metadata-only: output=%s stderr=%s code=%d",
			startGateReplay, stderr, code)
	}
	startGateList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-start-gate-reviews", runID)
	if code != 0 || stderr != "" || !strings.Contains(startGateList, startGateReviewID) ||
		!strings.Contains(startGateList, "decision=deny_start") ||
		!strings.Contains(startGateList, "start_implemented=false") ||
		strings.Contains(startGateList, "/workspace") || strings.Contains(startGateList, home) {
		t.Fatalf("Docker start-gate list leaked data: output=%s stderr=%s code=%d",
			startGateList, stderr, code)
	}
	startGateShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-start-gate-review-show", startGateReviewID)
	if code != 0 || stderr != "" ||
		!strings.Contains(startGateShown, "review_fingerprint:") ||
		!strings.Contains(startGateShown, "artifact_commit_authorized: false") ||
		!strings.Contains(startGateShown, "implemented=false") ||
		strings.Contains(startGateShown, "/workspace") || strings.Contains(startGateShown, home) {
		t.Fatalf("Docker start-gate show leaked data: output=%s stderr=%s code=%d",
			startGateShown, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-capture", startGateReviewID,
		"--operation-key", "docker-production-evidence-cli"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-machine-capture") {
		t.Fatalf("Docker production evidence skipped confirmation: stderr=%s code=%d",
			stderr, code)
	}
	emptyProductionEvidence, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-captures", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(emptyProductionEvidence, "no Docker production evidence captures") {
		t.Fatalf("unconfirmed production evidence left state: output=%s stderr=%s code=%d",
			emptyProductionEvidence, stderr, code)
	}
	productionCaptured, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-capture", startGateReviewID,
		"--operation-key", "docker-production-evidence-cli", "--confirm-machine-capture")
	if code != 0 || stderr != "" ||
		!strings.Contains(productionCaptured, "trust_class: machine_observation_non_authorizing") ||
		!strings.Contains(productionCaptured, "required_checks: 16") ||
		!strings.Contains(productionCaptured, "observed_checks: 0") ||
		!strings.Contains(productionCaptured, "production_verified_checks: 0") ||
		!strings.Contains(productionCaptured, "real_daemon_contacted: false") ||
		!strings.Contains(productionCaptured, "process_execution_authorized: false") ||
		!strings.Contains(productionCaptured, "raw_daemon_payload_stored: false") ||
		!strings.Contains(productionCaptured, "state=not_observed") ||
		strings.Contains(productionCaptured, "/workspace") ||
		strings.Contains(productionCaptured, home) ||
		strings.Contains(productionCaptured, "cyberagent-runtime-") {
		t.Fatalf("Docker production evidence leaked data or widened authority: output=%s stderr=%s code=%d",
			productionCaptured, stderr, code)
	}
	productionEvidenceID := sandboxDockerProductionEvidenceIDPattern.FindString(productionCaptured)
	if productionEvidenceID == "" {
		t.Fatalf("missing Docker production evidence id: %s", productionCaptured)
	}
	productionAttemptID := sandboxDockerProductionEvidenceAttemptIDPattern.FindString(productionCaptured)
	if productionAttemptID == "" ||
		!strings.Contains(productionCaptured, "current_reconciliation_status: initial_generation_quiescent") ||
		!strings.Contains(productionCaptured, "lease_generation: 1") ||
		!strings.Contains(productionCaptured, "lease_status: released") ||
		!strings.Contains(productionCaptured, "real_daemon_contact_authorized: false") ||
		!strings.Contains(productionCaptured, "lease_identity_exposed: false") {
		t.Fatalf("missing Docker production evidence write-ahead attempt: %s", productionCaptured)
	}
	productionReplay, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-capture", startGateReviewID,
		"--operation-key", "docker-production-evidence-cli", "--confirm-machine-capture")
	if code != 0 || stderr != "" || !strings.Contains(productionReplay, "replayed: true") {
		t.Fatalf("Docker production evidence replay recollected: output=%s stderr=%s code=%d",
			productionReplay, stderr, code)
	}
	productionAttemptList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-attempts", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(productionAttemptList, productionAttemptID) ||
		!strings.Contains(productionAttemptList, "status=evidence_committed") ||
		!strings.Contains(productionAttemptList, "generation=1") ||
		!strings.Contains(productionAttemptList, "reconciliations=1") ||
		!strings.Contains(productionAttemptList, "real_daemon_contact_confirmed=false") ||
		strings.Contains(productionAttemptList, "lease_id") ||
		strings.Contains(productionAttemptList, "/workspace") ||
		strings.Contains(productionAttemptList, home) {
		t.Fatalf("Docker production evidence attempt list leaked data: output=%s stderr=%s code=%d",
			productionAttemptList, stderr, code)
	}
	productionAttemptShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-attempt-show", productionAttemptID)
	if code != 0 || stderr != "" ||
		!strings.Contains(productionAttemptShown, "status: evidence_committed") ||
		!strings.Contains(productionAttemptShown, "evidence: "+productionEvidenceID) ||
		!strings.Contains(productionAttemptShown, "current_reconciliation_fingerprint:") ||
		!strings.Contains(productionAttemptShown, "artifact_commit_authorized: false") ||
		strings.Contains(productionAttemptShown, "owner_id") ||
		strings.Contains(productionAttemptShown, "lease_id:") ||
		strings.Contains(productionAttemptShown, "/workspace") ||
		strings.Contains(productionAttemptShown, home) {
		t.Fatalf("Docker production evidence attempt show leaked data: output=%s stderr=%s code=%d",
			productionAttemptShown, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-attempt-resume", productionAttemptID); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-machine-capture") {
		t.Fatalf("Docker production evidence resume skipped confirmation: stderr=%s code=%d",
			stderr, code)
	}
	productionAttemptReplay, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-attempt-resume", productionAttemptID,
		"--confirm-machine-capture")
	if code != 0 || stderr != "" ||
		!strings.Contains(productionAttemptReplay, "docker_production_evidence_attempt: "+productionAttemptID) ||
		!strings.Contains(productionAttemptReplay, "docker_production_evidence: "+productionEvidenceID) ||
		!strings.Contains(productionAttemptReplay, "replayed: true") {
		t.Fatalf("Docker production evidence attempt replay failed: output=%s stderr=%s code=%d",
			productionAttemptReplay, stderr, code)
	}
	productionList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-captures", runID)
	if code != 0 || stderr != "" || !strings.Contains(productionList, productionEvidenceID) ||
		!strings.Contains(productionList, "observed=0") ||
		!strings.Contains(productionList, "start_authorized=false") ||
		strings.Contains(productionList, "/workspace") || strings.Contains(productionList, home) {
		t.Fatalf("Docker production evidence list leaked data: output=%s stderr=%s code=%d",
			productionList, stderr, code)
	}
	productionLimited, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-captures", runID, "--limit", "1")
	if code != 0 || stderr != "" || !strings.Contains(productionLimited, productionEvidenceID) {
		t.Fatalf("Docker production evidence trailing limit failed: output=%s stderr=%s code=%d",
			productionLimited, stderr, code)
	}
	productionShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-show", productionEvidenceID)
	if code != 0 || stderr != "" ||
		!strings.Contains(productionShown, "suite_fingerprint:") ||
		!strings.Contains(productionShown, "artifact_commit_authorized: false") ||
		!strings.Contains(productionShown, "sufficient_for_start=false") ||
		strings.Contains(productionShown, "/workspace") || strings.Contains(productionShown, home) {
		t.Fatalf("Docker production evidence show leaked data: output=%s stderr=%s code=%d",
			productionShown, stderr, code)
	}
	harnessCaptured, stderr, code := executeTestCommandWithDockerProductionEvidence(t,
		cliDockerProductionEvidenceHarness{}, "run", "sandbox",
		"docker-production-evidence-capture", startGateReviewID,
		"--operation-key", "docker-production-evidence-cli-harness",
		"--confirm-machine-capture")
	if code != 0 || stderr != "" ||
		!strings.Contains(harnessCaptured, "status: capture_complete") ||
		!strings.Contains(harnessCaptured, "observed_checks: 16") ||
		!strings.Contains(harnessCaptured, "production_verified_checks: 0") ||
		!strings.Contains(harnessCaptured, "real_daemon_contacted: true") ||
		!strings.Contains(harnessCaptured, "container_start_authorized: false") ||
		strings.Contains(harnessCaptured, "/workspace") ||
		strings.Contains(harnessCaptured, home) {
		t.Fatalf("CLI v67 harness evidence widened authority: output=%s stderr=%s code=%d",
			harnessCaptured, stderr, code)
	}
	harnessEvidenceID := sandboxDockerProductionEvidenceIDPattern.FindString(harnessCaptured)
	if harnessEvidenceID == "" || harnessEvidenceID == productionEvidenceID {
		t.Fatalf("missing distinct CLI v67 evidence id: %s", harnessCaptured)
	}
	emptyEvidenceReviews, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-reviews", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(emptyEvidenceReviews, "no Docker production evidence reviews") {
		t.Fatalf("unexpected pre-review list: output=%s stderr=%s code=%d",
			emptyEvidenceReviews, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-review", harnessEvidenceID,
		"--decision", "accepted", "--reason-code", "metadata_scope_accepted",
		"--operation-key", "docker-production-evidence-review-cli"); code != 4 ||
		!strings.Contains(stderr, "requires --confirm-evidence-review") {
		t.Fatalf("CLI evidence review skipped confirmation: stderr=%s code=%d", stderr, code)
	}
	evidenceReviewed, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-review", harnessEvidenceID,
		"--decision", "accepted", "--reason-code", "metadata_scope_accepted",
		"--operation-key", "docker-production-evidence-review-cli",
		"--confirm-evidence-review", "--operator", "evidence_reviewer")
	if code != 0 || stderr != "" ||
		!strings.Contains(evidenceReviewed, "decision: accepted") ||
		!strings.Contains(evidenceReviewed, "reason_code: metadata_scope_accepted") ||
		!strings.Contains(evidenceReviewed, "receipt_accepted: true") ||
		!strings.Contains(evidenceReviewed, "production_verified_checks: 0") ||
		!strings.Contains(evidenceReviewed, "blockers: 16") ||
		!strings.Contains(evidenceReviewed, "start_gate_passed: false") ||
		!strings.Contains(evidenceReviewed, "process_execution_authorized: false") ||
		!strings.Contains(evidenceReviewed, "freeform_reason_stored: false") ||
		strings.Contains(evidenceReviewed, "/workspace") ||
		strings.Contains(evidenceReviewed, home) {
		t.Fatalf("CLI evidence review leaked data or widened authority: output=%s stderr=%s code=%d",
			evidenceReviewed, stderr, code)
	}
	evidenceReviewID := sandboxDockerProductionEvidenceReviewIDPattern.FindString(
		evidenceReviewed)
	if evidenceReviewID == "" {
		t.Fatalf("missing CLI evidence review id: %s", evidenceReviewed)
	}
	evidenceReviewReplay, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-review", harnessEvidenceID,
		"--decision", "accepted", "--reason-code", "metadata_scope_accepted",
		"--operation-key", "docker-production-evidence-review-cli",
		"--confirm-evidence-review", "--operator", "evidence_reviewer")
	if code != 0 || stderr != "" || !strings.Contains(evidenceReviewReplay, "replayed: true") {
		t.Fatalf("CLI evidence review replay failed: output=%s stderr=%s code=%d",
			evidenceReviewReplay, stderr, code)
	}
	evidenceReviewList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-reviews", runID, "--limit", "1")
	if code != 0 || stderr != "" || !strings.Contains(evidenceReviewList, evidenceReviewID) ||
		!strings.Contains(evidenceReviewList, "receipt_accepted=true") ||
		!strings.Contains(evidenceReviewList, "verified=0") ||
		!strings.Contains(evidenceReviewList, "start_authorized=false") ||
		strings.Contains(evidenceReviewList, "/workspace") ||
		strings.Contains(evidenceReviewList, home) {
		t.Fatalf("CLI evidence review list leaked data: output=%s stderr=%s code=%d",
			evidenceReviewList, stderr, code)
	}
	evidenceReviewShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-review-show", evidenceReviewID)
	if code != 0 || stderr != "" ||
		!strings.Contains(evidenceReviewShown, "review_fingerprint:") ||
		!strings.Contains(evidenceReviewShown, "artifact_commit_authorized: false") ||
		!strings.Contains(evidenceReviewShown, "raw_daemon_payload_stored: false") ||
		strings.Contains(evidenceReviewShown, "/workspace") ||
		strings.Contains(evidenceReviewShown, home) {
		t.Fatalf("CLI evidence review show leaked data: output=%s stderr=%s code=%d",
			evidenceReviewShown, stderr, code)
	}
	rejectedHarnessCaptured, stderr, code := executeTestCommandWithDockerProductionEvidence(t,
		cliDockerProductionEvidenceHarness{}, "run", "sandbox",
		"docker-production-evidence-capture", startGateReviewID,
		"--operation-key", "docker-production-evidence-cli-harness-rejected",
		"--confirm-machine-capture")
	rejectedHarnessEvidenceID := sandboxDockerProductionEvidenceIDPattern.FindString(
		rejectedHarnessCaptured)
	if code != 0 || stderr != "" || rejectedHarnessEvidenceID == "" ||
		rejectedHarnessEvidenceID == harnessEvidenceID {
		t.Fatalf("create second CLI v67 evidence: output=%s stderr=%s code=%d",
			rejectedHarnessCaptured, stderr, code)
	}
	rejectedReview, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-review", rejectedHarnessEvidenceID,
		"--decision", "rejected", "--reason-code", "insufficient_evidence",
		"--operation-key", "docker-production-evidence-review-cli-rejected",
		"--confirm-evidence-review", "--operator", "evidence_reviewer")
	if code != 0 || stderr != "" ||
		!strings.Contains(rejectedReview, "decision: rejected") ||
		!strings.Contains(rejectedReview, "reason_code: insufficient_evidence") ||
		!strings.Contains(rejectedReview, "receipt_accepted: false") ||
		!strings.Contains(rejectedReview, "production_verified_checks: 0") ||
		!strings.Contains(rejectedReview, "blockers: 16") ||
		!strings.Contains(rejectedReview, "process_execution_authorized: false") ||
		strings.Contains(rejectedReview, "/workspace") ||
		strings.Contains(rejectedReview, home) {
		t.Fatalf("CLI rejected evidence review leaked data or widened authority: output=%s stderr=%s code=%d",
			rejectedReview, stderr, code)
	}
	rejectedReviewID := sandboxDockerProductionEvidenceReviewIDPattern.FindString(rejectedReview)
	if rejectedReviewID == "" || rejectedReviewID == evidenceReviewID {
		t.Fatalf("missing distinct rejected evidence review id: %s", rejectedReview)
	}
	rejectedReviewReplay, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-review", rejectedHarnessEvidenceID,
		"--decision", "rejected", "--reason-code", "insufficient_evidence",
		"--operation-key", "docker-production-evidence-review-cli-rejected",
		"--confirm-evidence-review", "--operator", "evidence_reviewer")
	if code != 0 || stderr != "" ||
		!strings.Contains(rejectedReviewReplay, "replayed: true") {
		t.Fatalf("CLI rejected evidence review replay failed: output=%s stderr=%s code=%d",
			rejectedReviewReplay, stderr, code)
	}
	allEvidenceReviews, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-production-evidence-reviews", runID, "--limit", "2")
	if code != 0 || stderr != "" ||
		!strings.Contains(allEvidenceReviews, evidenceReviewID) ||
		!strings.Contains(allEvidenceReviews, rejectedReviewID) ||
		!strings.Contains(allEvidenceReviews, "decision=rejected") ||
		!strings.Contains(allEvidenceReviews, "receipt_accepted=false") {
		t.Fatalf("CLI evidence review list omitted rejected decision: output=%s stderr=%s code=%d",
			allEvidenceReviews, stderr, code)
	}
	attemptList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-attempts", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(attemptList, "status=rehearsal_completed") ||
		!strings.Contains(attemptList, "generation=1") ||
		!strings.Contains(attemptList, "host_input_required=true") ||
		!strings.Contains(attemptList, "host_input_handoff_required=true") ||
		!strings.Contains(attemptList, "container_started=false") {
		t.Fatalf("Docker attempt list failed: output=%s stderr=%s code=%d",
			attemptList, stderr, code)
	}
	attemptID := sandboxDockerAttemptIDPattern.FindString(attemptList)
	if attemptID == "" {
		t.Fatalf("missing Docker attempt id: %s", attemptList)
	}
	attemptShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-attempt-show", attemptID)
	if code != 0 || stderr != "" ||
		!strings.Contains(attemptShown, "verified_controls:") ||
		!strings.Contains(attemptShown, "environment_empty") ||
		!strings.Contains(attemptShown, "execution_evidence=false") ||
		!strings.Contains(attemptShown, "lease_status: released") ||
		!strings.Contains(attemptShown, "host_input_requirement_durable: true") ||
		!strings.Contains(attemptShown, "host_input_required: true") ||
		!strings.Contains(attemptShown, "host_input_handoff_requirement_durable: true") ||
		!strings.Contains(attemptShown, "host_input_handoff_required: true") ||
		strings.Contains(attemptShown, "/workspace") ||
		strings.Contains(attemptShown, strings.Repeat("c", 64)) {
		t.Fatalf("Docker attempt show leaked data: output=%s stderr=%s code=%d",
			attemptShown, stderr, code)
	}
	resumed, stderr, code := executeTestCommandWithDockerWriteTransport(t, writer,
		"run", "sandbox", "docker-attempt-resume", attemptID, "--manifest", manifestPath,
		"--confirm-daemon-write")
	if code != 0 || stderr != "" || writer.calls != 1 ||
		!strings.Contains(resumed, "docker_rehearsal: "+rehearsalID) ||
		!strings.Contains(resumed, "replayed: true") {
		t.Fatalf("Docker attempt-id resume failed: output=%s stderr=%s code=%d calls=%d",
			resumed, stderr, code, writer.calls)
	}
	rehearsalReplay, stderr, code := executeTestCommandWithDockerInputStaging(t, writer,
		hostInputStager,
		"run", "sandbox", "docker-rehearse", planID, "--manifest", manifestPath,
		"--operation-key", "docker-rehearsal-cli", "--confirm-daemon-write")
	if code != 0 || stderr != "" || writer.calls != 1 || hostInputStager.captureCalls != 3 ||
		handoffTransport.calls != 1 ||
		!strings.Contains(rehearsalReplay, "docker_rehearsal: "+rehearsalID) ||
		!strings.Contains(rehearsalReplay, "replayed: true") {
		t.Fatalf("Docker rehearsal replay contacted transport: output=%s stderr=%s code=%d calls=%d",
			rehearsalReplay, stderr, code, writer.calls)
	}
	rehearsalList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-rehearsals", runID)
	if code != 0 || stderr != "" || !strings.Contains(rehearsalList, rehearsalID) ||
		!strings.Contains(rehearsalList, "daemon_writes=2") ||
		!strings.Contains(rehearsalList, "container_started=false") {
		t.Fatalf("Docker rehearsal list failed: output=%s stderr=%s code=%d",
			rehearsalList, stderr, code)
	}
	rehearsalShown, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-rehearsal-show", rehearsalID)
	if code != 0 || stderr != "" ||
		!strings.Contains(rehearsalShown, "write_transport_steps:") ||
		!strings.Contains(rehearsalShown, "remove_container") ||
		strings.Contains(rehearsalShown, "report.json") ||
		strings.Contains(rehearsalShown, strings.Repeat("c", 64)) {
		t.Fatalf("Docker rehearsal show leaked data: output=%s stderr=%s code=%d",
			rehearsalShown, stderr, code)
	}
}

func TestSandboxCLIApprovalRequestReviewAndDisabledCandidate(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	manifest := defaultSandboxManifestTemplate()
	manifest.Mounts[0].Access = "read_write"
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "sandbox-write-manifest.json")
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "sandbox-approval-demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "sandbox approval lifecycle",
		"--workspace", "sandbox-approval-demo", "--profile", "code")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	prepared, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-approval-prepare")
	if code != 0 || stderr != "" || !strings.Contains(prepared, "approval_status: required") {
		t.Fatalf("approval preparation failed output=%s stderr=%s code=%d", prepared, stderr, code)
	}
	preparationID := sandboxPreparationIDPattern.FindString(prepared)
	requested, stderr, code := executeTestCommand(t, "run", "sandbox", "request", preparationID,
		"--operator", "sandbox_cli_operator")
	if code != 0 || stderr != "" || !strings.Contains(requested, "status: pending") ||
		!strings.Contains(requested, "tool: sandbox.manifest") {
		t.Fatalf("approval request failed output=%s stderr=%s code=%d", requested, stderr, code)
	}
	approvalID := approvalIDPattern.FindString(requested)
	if approvalID == "" {
		t.Fatalf("approval request did not return an id: %s", requested)
	}
	reviewed, stderr, code := executeTestCommand(t, "run", "sandbox", "review", preparationID,
		"--decision", "approve", "--operation-key", "sandbox-cli-approval-review",
		"--reviewer", "sandbox_security_operator")
	if code != 0 || stderr != "" || !strings.Contains(reviewed, "status: approved") {
		t.Fatalf("approval review failed output=%s stderr=%s code=%d", reviewed, stderr, code)
	}
	candidate, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate", preparationID,
		"--manifest", manifestPath, "--approval", approvalID,
		"--operation-key", "sandbox-cli-approved-candidate",
		"--operator", "sandbox_cli_operator")
	if code != 0 || stderr != "" || !strings.Contains(candidate, "approval_status: approved") ||
		!strings.Contains(candidate, "backend_enabled: false") ||
		!strings.Contains(candidate, "execution_authorized: false") {
		t.Fatalf("approved disabled candidate failed output=%s stderr=%s code=%d", candidate, stderr, code)
	}
}
