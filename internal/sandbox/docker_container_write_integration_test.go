//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const dockerWriteIntegrationImageEnv = "CYBERAGENT_DOCKER_WRITE_TEST_IMAGE_DIGEST"

type cancelAfterDockerCreateDoer struct {
	delegate dockerContainerWriteHTTPDoer
	cancel   context.CancelFunc
	once     sync.Once
}

func (doer *cancelAfterDockerCreateDoer) Do(request *http.Request) (*http.Response, error) {
	response, err := doer.delegate.Do(request)
	if err == nil && request.Method == http.MethodPost &&
		request.URL.Path == "/v"+DockerContainerWriteAPIVersion+"/containers/create" {
		doer.once.Do(doer.cancel)
	}
	return response, err
}

func TestDockerContainerWriteRealDaemonOptIn(t *testing.T) {
	imageDigest := strings.TrimSpace(os.Getenv(dockerWriteIntegrationImageEnv))
	if imageDigest == "" {
		t.Skip("set " + dockerWriteIntegrationImageEnv + " to a pre-existing Linux image digest")
	}
	if !ValidOCIImageDigest(imageDigest) {
		t.Fatalf("%s must be an exact sha256 image digest", dockerWriteIntegrationImageEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request := newDockerWriteIntegrationRequest(t, ctx, imageDigest)
	local := NewLocalDockerContainerWriteTransport()
	transport, ok := local.(dockerEngineContainerWriteTransport)
	if !ok {
		t.Fatalf("fixed local Docker write transport is unavailable: %T", local)
	}

	// Cancellation immediately after a successful create must still remove the container
	// through the transport's independent bounded cleanup context.
	cancelCtx, cancelAfterCreate := context.WithCancel(ctx)
	cancelling := transport
	cancelling.doer = &cancelAfterDockerCreateDoer{delegate: transport.doer,
		cancel: cancelAfterCreate}
	if _, err := cancelling.Rehearse(cancelCtx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("real Docker cancellation did not propagate: %v", err)
	}
	if _, found, err := transport.inspect(ctx, request.Spec.ContainerName); err != nil || found {
		t.Fatalf("real Docker cancellation left an orphan: found=%t err=%v", found, err)
	}

	// A matching unstarted orphan is safe to reconcile; a process is never started.
	if _, err := transport.create(ctx, request); err != nil {
		t.Fatalf("prepare real Docker orphan: %v", err)
	}
	result, err := transport.Rehearse(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReconciledContainerCount != 1 || result.ContainerStarted ||
		result.ProcessExecuted || result.DaemonWriteCount != 3 {
		t.Fatalf("real Docker reconciliation escaped the rehearsal boundary: %#v", result)
	}
	if _, found, err := transport.inspect(ctx, request.Spec.ContainerName); err != nil || found {
		t.Fatalf("real Docker rehearsal left a container: found=%t err=%v", found, err)
	}
}

func TestDockerHostInputHandoffRealDaemonOptIn(t *testing.T) {
	imageDigest := strings.TrimSpace(os.Getenv(dockerWriteIntegrationImageEnv))
	if imageDigest == "" {
		t.Skip("set " + dockerWriteIntegrationImageEnv + " to a pre-existing Linux image digest")
	}
	if !ValidOCIImageDigest(imageDigest) {
		t.Fatalf("%s must be an exact sha256 image digest", dockerWriteIntegrationImageEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	writeRequest, plan, manifest, workspaceRoot := newDockerWriteIntegrationFixture(
		t, ctx, imageDigest)
	writeTransport, ok := NewLocalDockerContainerWriteTransport().(dockerEngineContainerWriteTransport)
	if !ok {
		t.Fatal("fixed local Docker write transport is unavailable")
	}
	stageResult, err := writeTransport.Stage(ctx, writeRequest)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := writeTransport.Endpoint()
	attemptIntent, err := NewDockerContainerAttemptIntent("docker-handoff-integration-attempt",
		strings.Repeat("a", 64), plan, writeRequest, endpoint, plan.RequestedBy,
		time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	captureRequirement, err := NewDockerHostInputRequirement(attemptIntent, plan, true, true)
	if err != nil {
		t.Fatal(err)
	}
	handoffRequirement, err := NewDockerHostInputHandoffRequirement(attemptIntent, plan,
		captureRequirement, true, true)
	if err != nil {
		t.Fatal(err)
	}
	lease := DockerContainerAttemptLease{AttemptID: attemptIntent.ID,
		LeaseID: "docker-handoff-integration-lease", OwnerID: "integration_operator",
		Generation: 1, Status: DockerContainerAttemptLeaseActive,
		AcquiredAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	stage, err := NewDockerContainerAttemptStage(attemptIntent.ID, 1, stageResult,
		time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	attempt := DockerContainerRehearsalAttempt{Intent: attemptIntent,
		HostInputRequirement:        &captureRequirement,
		HostInputHandoffRequirement: &handoffRequirement,
		Status:                      DockerContainerAttemptStatusStaged, Lease: lease, Stage: &stage}
	stagingIntent, err := NewDockerHostInputStagingIntent(
		"docker-handoff-integration-staging-intent", strings.Repeat("b", 64),
		attempt, plan, manifest, plan.RequestedBy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := NewLocalDockerHostInputStager().(DockerHostInputBundleProvider)
	if !ok {
		t.Fatal("local Docker host input stager cannot retain a sealed bundle")
	}
	bundle, err := provider.Capture(ctx, HostInputBundleRequest{
		WorkspaceRoot: workspaceRoot, Manifest: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	report := bundle.Report()
	stagingValue, err := NewDockerHostInputStaging("docker-handoff-integration-staging",
		stagingIntent, 1, report, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	staging := DockerHostInputStagingRecord{Intent: stagingIntent, Staging: &stagingValue}
	handoffIntent, err := NewDockerHostInputHandoffIntent(
		"docker-handoff-integration-intent", strings.Repeat("f", 64),
		attempt, plan, staging, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	handoffRequest, err := NewDockerHostInputHandoffRequest(handoffIntent, writeRequest,
		stageResult, report)
	if err != nil {
		t.Fatal(err)
	}
	handoffTransport, ok := NewLocalDockerHostInputHandoffTransport().(dockerEngineContainerWriteTransport)
	if !ok {
		t.Fatal("fixed local Docker handoff transport is unavailable")
	}
	t.Cleanup(func() {
		_ = handoffTransport.cleanupDockerHostInputHandoff(handoffRequest)
	})
	result, err := handoffTransport.Handoff(ctx, handoffRequest, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if result.Validate() != nil || !result.DaemonConsumed || !result.ReadbackVerified ||
		!result.FinalMountReadOnly || !result.CleanupConfirmed || result.ContainerStarted ||
		result.ProcessExecuted || result.OutputExported {
		t.Fatalf("real daemon handoff escaped its boundary: %#v", result)
	}
	if _, found, err := writeTransport.inspect(ctx,
		writeRequest.Spec.ContainerName); err != nil || found {
		t.Fatalf("real daemon handoff left a target: found=%t err=%v", found, err)
	}
	if _, found, err := handoffTransport.inspectDockerHostInputContainer(ctx,
		handoffRequest.CarrierName); err != nil || found {
		t.Fatalf("real daemon handoff left a carrier: found=%t err=%v", found, err)
	}
	if _, found, err := handoffTransport.inspectDockerHostInputVolume(ctx,
		handoffRequest); err != nil || found {
		t.Fatalf("real daemon handoff left a volume: found=%t err=%v", found, err)
	}
}

func newDockerWriteIntegrationRequest(t *testing.T, ctx context.Context,
	imageDigest string,
) DockerContainerWriteRequest {
	request, _, _, _ := newDockerWriteIntegrationFixture(t, ctx, imageDigest)
	return request
}

func newDockerWriteIntegrationFixture(t *testing.T, ctx context.Context,
	imageDigest string,
) (DockerContainerWriteRequest, DockerContainerPlan, Manifest, string) {
	t.Helper()
	manifest := dockerContainerCompilerManifest()
	manifest.Network = NetworkScope{Mode: "disabled"}
	manifest.Environment = nil
	manifest.InputArtifactIDs = nil
	normalized, err := NormalizeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestFingerprint, err := normalized.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	outputPlan, err := NewOutputExportPlan(normalized)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	observation := DockerObservation{
		ID: "docker-observation-integration", EvidenceID: "docker-evidence-integration",
		OutputSimulationID: "docker-output-integration", PreflightID: "docker-preflight-integration",
		ExecutionID: "docker-execution-integration", CandidateID: "docker-candidate-integration",
		PreparationID: "docker-preparation-integration", RunID: "docker-run-integration",
		MissionID: "docker-mission-integration", WorkspaceID: "docker-workspace-integration",
		ManifestFingerprint: manifestFingerprint, AuthorizationFingerprint: digest,
		PolicyFingerprint: digest, MountBindingFingerprint: digest,
		InputArtifactDigest: digest, ThreatModelFingerprint: digest,
		OutputPlanFingerprint: outputPlan.Fingerprint,
		Report:                DockerObservationReport{ImageDigest: imageDigest},
		RequestedBy:           "integration_operator", CreatedAt: time.Now().UTC(),
	}
	observer := NewReadOnlyDockerProductionObserver(dockerContainerCompilerTransport{
		imageDigest: imageDigest, pids: true, ncpu: 8, memory: 8 * 1024 * 1024 * 1024,
	})
	report, err := observer.Observe(ctx, DockerObservationProbeRequest{
		BindingFingerprint: DockerObservationBindingFingerprint(observation),
		ImageDigest:        imageDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation.Report = report
	spec, err := CompileDockerContainerSpec(ctx, observation, normalized)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := NewInMemoryDockerWriteTransaction().Simulate(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewDockerContainerPlan("docker-plan-integration", observation, spec,
		transaction, observation.RequestedBy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"output", "src"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	request, err := NewDockerContainerWriteRequest(ctx, root, spec)
	if err != nil {
		t.Fatal(err)
	}
	return request, plan, normalized, root
}
