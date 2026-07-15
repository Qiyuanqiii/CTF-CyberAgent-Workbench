package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
	rehearsed, stderr, code := executeTestCommandWithDockerWriteTransport(t, writer,
		"run", "sandbox", "docker-rehearse", planID, "--manifest", manifestPath,
		"--operation-key", "docker-rehearsal-cli", "--confirm-daemon-write")
	if code != 0 || stderr != "" || writer.calls != 1 ||
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
	attemptList, stderr, code := executeTestCommand(t, "run", "sandbox",
		"docker-attempts", runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(attemptList, "status=rehearsal_completed") ||
		!strings.Contains(attemptList, "generation=1") ||
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
	rehearsalReplay, stderr, code := executeTestCommandWithDockerWriteTransport(t, writer,
		"run", "sandbox", "docker-rehearse", planID, "--manifest", manifestPath,
		"--operation-key", "docker-rehearsal-cli", "--confirm-daemon-write")
	if code != 0 || stderr != "" || writer.calls != 1 ||
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
