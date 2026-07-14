package application

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
	"cyberagent-workbench/internal/store"
)

type applicationDockerObservationTransport struct {
	imageDigest string
}

func (transport applicationDockerObservationTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (applicationDockerObservationTransport) Ping(context.Context) error { return nil }

func (applicationDockerObservationTransport) Version(context.Context) (sandbox.DockerDaemonVersion, error) {
	return sandbox.DockerDaemonVersion{
		APIVersion: "1.47", MinAPIVersion: "1.24", EngineVersion: "27.5.1",
		GitCommit: "abc123", OSType: "linux", Architecture: "amd64",
	}, nil
}

func (applicationDockerObservationTransport) Info(context.Context) (sandbox.DockerDaemonInfo, error) {
	return sandbox.DockerDaemonInfo{
		ID: "daemon-id", Name: "host", DockerRootDir: "/var/lib/docker",
		ServerVersion: "27.5.1", OSType: "linux", Architecture: "amd64",
		Driver: "overlay2", CgroupDriver: "systemd", CgroupVersion: "2",
		DefaultRuntime: "runc", NCPU: 4, MemoryBytes: 8 * 1024 * 1024 * 1024,
		PidsLimit: true, SecurityOptions: []string{"name=rootless"},
	}, nil
}

func (transport applicationDockerObservationTransport) InspectImage(context.Context,
	string,
) (sandbox.DockerImageInspection, error) {
	return sandbox.DockerImageInspection{
		ID:          "sha256:" + strings.Repeat("f", 64),
		RepoDigests: []string{"example.invalid/workbench@" + transport.imageDigest},
		OSType:      "linux", Architecture: "amd64", SizeBytes: 4096,
		User: "65532", RootFSType: "layers", GraphDriver: "overlay2",
	}, nil
}

type countingDockerObserver struct {
	delegate sandbox.DockerProductionObserver
	calls    int
	mutate   func(*sandbox.DockerObservationReport)
}

type barrierDockerObserver struct {
	delegate sandbox.DockerProductionObserver
	ready    chan struct{}
	calls    atomic.Int32
}

func (observer *barrierDockerObserver) Observe(ctx context.Context,
	request sandbox.DockerObservationProbeRequest,
) (sandbox.DockerObservationReport, error) {
	if observer.calls.Add(1) == 2 {
		close(observer.ready)
	}
	select {
	case <-ctx.Done():
		return sandbox.DockerObservationReport{}, ctx.Err()
	case <-observer.ready:
		return observer.delegate.Observe(ctx, request)
	}
}

func (observer *countingDockerObserver) Observe(ctx context.Context,
	request sandbox.DockerObservationProbeRequest,
) (sandbox.DockerObservationReport, error) {
	observer.calls++
	report, err := observer.delegate.Observe(ctx, request)
	if err == nil && observer.mutate != nil {
		observer.mutate(&report)
	}
	return report, err
}

func TestDockerObservationRevalidatesAuthorityReplaysAndNeverAuthorizes(t *testing.T) {
	ctx := context.Background()
	st, run, _ := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, preflight := prepareDockerSandboxPreflight(t, ctx, service, run.ID,
		"docker-observe", "docker_observer")
	imageDigest := "sha256:" + strings.Repeat("d", 64)
	evidence, simulation := prepareDockerObservationEvidence(t, ctx, service, manifest,
		preflight, imageDigest, "docker-observe", "docker_observer")
	observer := &countingDockerObserver{delegate: sandbox.NewReadOnlyDockerProductionObserver(
		applicationDockerObservationTransport{imageDigest: imageDigest})}
	service.dockerObserver = observer
	request := ObserveDockerBackendRequest{
		EvidenceID: evidence.ID, OutputSimulationID: simulation.ID, Manifest: manifest,
		OperationKey: "docker-observe-operation", RequestedBy: "docker_observer",
	}
	observed, err := service.ObserveDockerBackend(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Replayed || observed.Report.Status != sandbox.DockerObservationStatusComplete ||
		!observed.Report.ProductionObserved || observed.Report.ProductionVerified ||
		observed.Report.BackendAvailable || observed.Report.BackendEnabled ||
		observed.Report.ExecutionAuthorized || observed.Report.ArtifactCommitAuthorized ||
		observer.calls != 1 {
		t.Fatalf("Docker observation widened authority: observation=%#v calls=%d",
			observed, observer.calls)
	}
	replayed, err := service.ObserveDockerBackend(ctx, request)
	if err != nil || !replayed.Replayed || replayed.ID != observed.ID || observer.calls != 1 {
		t.Fatalf("Docker observation replay reprobed or diverged: %#v calls=%d err=%v",
			replayed, observer.calls, err)
	}
	changed := request
	changed.Manifest.TimeoutSeconds++
	if _, err := service.ObserveDockerBackend(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict || observer.calls != 1 {
		t.Fatalf("changed observation reused operation key: calls=%d err=%v", observer.calls, err)
	}
	if _, err := service.CancelDisabledExecution(ctx, CancelSandboxExecutionRequest{
		ExecutionID: evidence.ExecutionID, OperationKey: "docker-observe-cancel",
		RequestedBy: "docker_observer",
	}); err != nil {
		t.Fatal(err)
	}
	afterCancel := request
	afterCancel.OperationKey = "docker-observe-after-cancel"
	if _, err := service.ObserveDockerBackend(ctx, afterCancel); apperror.CodeOf(err) != apperror.CodeFailedPrecondition || observer.calls != 1 {
		t.Fatalf("cancelled authority reached Docker observer: calls=%d err=%v", observer.calls, err)
	}
}

func TestDockerObservationPersistsUnavailableAndRejectsObserverAuthorityClaims(t *testing.T) {
	ctx := context.Background()
	st, run, _ := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, preflight := prepareDockerSandboxPreflight(t, ctx, service, run.ID,
		"docker-unavailable", "docker_observer")
	imageDigest := "sha256:" + strings.Repeat("c", 64)
	evidence, simulation := prepareDockerObservationEvidence(t, ctx, service, manifest,
		preflight, imageDigest, "docker-unavailable", "docker_observer")
	service.dockerObserver = sandbox.NewReadOnlyDockerProductionObserver(
		sandbox.NewUnavailableDockerReadOnlyTransport(sandbox.DockerObservationEndpointLocalNPipe,
			sandbox.DockerObservationFailureTransportUnsupported))
	request := ObserveDockerBackendRequest{
		EvidenceID: evidence.ID, OutputSimulationID: simulation.ID, Manifest: manifest,
		OperationKey: "docker-unavailable-operation", RequestedBy: "docker_observer",
	}
	observed, err := service.ObserveDockerBackend(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Report.Status != sandbox.DockerObservationStatusDaemonUnavailable ||
		observed.Report.FailureCode != sandbox.DockerObservationFailureTransportUnsupported ||
		observed.Report.ProductionObserved || observed.Report.ProductionVerified {
		t.Fatalf("unavailable Docker observation is invalid: %#v", observed)
	}

	claiming := &countingDockerObserver{delegate: sandbox.NewReadOnlyDockerProductionObserver(
		applicationDockerObservationTransport{imageDigest: imageDigest}),
		mutate: func(report *sandbox.DockerObservationReport) {
			report.BackendAvailable = true
		}}
	service.dockerObserver = claiming
	request.OperationKey = "docker-unsupported-claim"
	if _, err := service.ObserveDockerBackend(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition || claiming.calls != 1 {
		t.Fatalf("unsupported Docker claim was accepted: calls=%d err=%v", claiming.calls, err)
	}
	values, err := service.ListDockerObservations(ctx, run.ID, 10)
	if err != nil || len(values) != 1 || values[0].ID != observed.ID {
		t.Fatalf("unsupported claim left a ledger row: values=%#v err=%v", values, err)
	}
}

func TestDockerObservationConcurrentApplicationRequestsConvergeAcrossStores(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "docker-observation-concurrent.db")
	primary, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primary.Close() })
	workspaceRoot := t.TempDir()
	if err := primary.SaveWorkspace(ctx, store.WorkspaceRecord{
		ID: "ws-docker-observation", Name: "docker observation", RootPath: workspaceRoot,
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := NewRunService(primary).Create(ctx, CreateRunRequest{
		Goal: "converge concurrent Docker observations", Profile: "code",
		WorkspaceID: "ws-docker-observation",
		Budget:      domain.Budget{MaxTurns: 4, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	primaryService := NewSandboxManifestService(primary, policy.NewDefaultChecker())
	manifest, preflight := prepareDockerSandboxPreflight(t, ctx, primaryService, run.ID,
		"docker-observe-concurrent", "docker_observer")
	imageDigest := "sha256:" + strings.Repeat("b", 64)
	evidence, simulation := prepareDockerObservationEvidence(t, ctx, primaryService, manifest,
		preflight, imageDigest, "docker-observe-concurrent", "docker_observer")

	secondary, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondary.Close() })
	secondaryService := NewSandboxManifestService(secondary, policy.NewDefaultChecker())
	observer := &barrierDockerObserver{
		delegate: sandbox.NewReadOnlyDockerProductionObserver(
			applicationDockerObservationTransport{imageDigest: imageDigest}),
		ready: make(chan struct{}),
	}
	primaryService.dockerObserver = observer
	secondaryService.dockerObserver = observer
	request := ObserveDockerBackendRequest{
		EvidenceID: evidence.ID, OutputSimulationID: simulation.ID, Manifest: manifest,
		OperationKey: "docker-observe-concurrent-operation", RequestedBy: "docker_observer",
	}

	services := []*SandboxManifestService{primaryService, secondaryService}
	results := make([]sandbox.DockerObservation, len(services))
	errorsFound := make([]error, len(services))
	var group sync.WaitGroup
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errorsFound[index] = services[index].ObserveDockerBackend(ctx, request)
		}(index)
	}
	group.Wait()
	if errorsFound[0] != nil || errorsFound[1] != nil || results[0].ID != results[1].ID ||
		results[0].Replayed == results[1].Replayed || observer.calls.Load() != 2 {
		t.Fatalf("concurrent Application observations diverged: results=%#v errors=%v calls=%d",
			results, errorsFound, observer.calls.Load())
	}
	observations, err := primary.ListDockerObservations(ctx, run.ID, 10)
	if err != nil || len(observations) != 1 || observations[0].ID != results[0].ID {
		t.Fatalf("concurrent Application observations did not converge in Store: values=%#v err=%v",
			observations, err)
	}
}

func prepareDockerObservationEvidence(t *testing.T, ctx context.Context,
	service *SandboxManifestService, manifest sandbox.Manifest, preflight sandbox.DisabledPreflight,
	imageDigest, prefix, requestedBy string,
) (sandbox.BackendEvidence, sandbox.OutputSimulation) {
	t.Helper()
	evidence, err := service.RecordSimulatedBackendEvidence(ctx,
		RecordSandboxBackendEvidenceRequest{
			PreflightID: preflight.ID, Manifest: manifest, ImageDigest: imageDigest,
			OperationKey: prefix + "-evidence", RequestedBy: requestedBy,
		})
	if err != nil {
		t.Fatal(err)
	}
	fixture := sandbox.OutputFixture{ProtocolVersion: sandbox.OutputFixtureProtocolVersion,
		Outputs: []sandbox.OutputFixtureItem{
			{Kind: sandbox.OutputKindStdout, FileType: sandbox.OutputFileTypeStream, Content: "out"},
			{Kind: sandbox.OutputKindStderr, FileType: sandbox.OutputFileTypeStream, Content: "err"},
		}}
	simulation, err := service.SimulateOutputTransaction(ctx, SimulateSandboxOutputRequest{
		EvidenceID: evidence.ID, Manifest: manifest, Fixture: fixture,
		OperationKey: prefix + "-simulation", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	return evidence, simulation
}
