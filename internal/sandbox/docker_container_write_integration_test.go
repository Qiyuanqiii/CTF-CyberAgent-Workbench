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

func newDockerWriteIntegrationRequest(t *testing.T, ctx context.Context,
	imageDigest string,
) DockerContainerWriteRequest {
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
	return request
}
