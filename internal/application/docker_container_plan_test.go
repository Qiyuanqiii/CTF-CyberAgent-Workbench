package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

type countingDockerContainerWriter struct {
	delegate sandbox.DockerContainerTransactionHarness
	calls    int
	mutate   func(*sandbox.DockerWriteSimulation)
}

func (writer *countingDockerContainerWriter) Simulate(ctx context.Context,
	spec sandbox.DockerContainerSpec,
) (sandbox.DockerWriteSimulation, error) {
	writer.calls++
	result, err := writer.delegate.Simulate(ctx, spec)
	if err == nil && writer.mutate != nil {
		writer.mutate(&result)
	}
	return result, err
}

func TestDockerContainerPlanRevalidatesAuthorityReplaysAndNeverWritesDaemon(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-plan", "docker_plan_operator")
	writer := &countingDockerContainerWriter{delegate: sandbox.NewInMemoryDockerWriteTransaction()}
	service.WithDockerContainerTransactionHarness(writer)
	request := CompileDockerContainerPlanRequest{
		ObservationID: observation.ID, Manifest: manifest,
		OperationKey: "docker-plan-operation", RequestedBy: "docker_plan_operator",
	}
	plan, err := service.CompileDockerContainerPlan(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replayed || writer.calls != 1 || !plan.SimulationOnly ||
		plan.Transaction.DaemonWriteCount != 0 || plan.Transaction.BackendTouched ||
		plan.ProductionSubmitted || plan.ProductionVerified || plan.BackendAvailable ||
		plan.BackendEnabled || plan.ExecutionAuthorized || plan.ArtifactCommitAuthorized ||
		plan.WritableMountCount != 1 || plan.DedicatedOutputMounts != 1 ||
		plan.ReadOnlyMountCount != plan.MountCount-1 {
		t.Fatalf("Docker container plan widened authority: plan=%#v calls=%d", plan, writer.calls)
	}
	replayed, err := service.CompileDockerContainerPlan(ctx, request)
	if err != nil || !replayed.Replayed || replayed.ID != plan.ID || writer.calls != 1 {
		t.Fatalf("Docker container plan replay reran fake writer: plan=%#v calls=%d err=%v",
			replayed, writer.calls, err)
	}
	changed := request
	changed.Manifest.TimeoutSeconds++
	if _, err := service.CompileDockerContainerPlan(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict || writer.calls != 1 {
		t.Fatalf("changed Manifest reused Docker plan operation: calls=%d err=%v", writer.calls, err)
	}
	loaded, err := service.GetDockerContainerPlan(ctx, plan.ID)
	if err != nil || loaded.PlanFingerprint != plan.PlanFingerprint {
		t.Fatalf("load Docker plan: %#v err=%v", loaded, err)
	}
	listed, err := service.ListDockerContainerPlans(ctx, run.ID, 10)
	if err != nil || len(listed) != 1 || listed[0].ID != plan.ID {
		t.Fatalf("list Docker plans: %#v err=%v", listed, err)
	}

	if _, err := service.CancelDisabledExecution(ctx, CancelSandboxExecutionRequest{
		ExecutionID: observation.ExecutionID, OperationKey: "docker-plan-cancel",
		RequestedBy: "docker_plan_operator",
	}); err != nil {
		t.Fatal(err)
	}
	afterCancel := request
	afterCancel.OperationKey = "docker-plan-after-cancel"
	if _, err := service.CompileDockerContainerPlan(ctx, afterCancel); apperror.CodeOf(err) != apperror.CodeFailedPrecondition || writer.calls != 1 {
		t.Fatalf("cancelled authority reached Docker fake writer: calls=%d err=%v", writer.calls, err)
	}
}

func TestDockerContainerPlanRollsBackFakeFailureAndRejectsProductionClaims(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-plan-failure", "docker_plan_operator")
	failing := sandbox.NewInMemoryDockerWriteTransaction()
	failing.FailAtOrdinal = 4
	service.WithDockerContainerTransactionHarness(failing)
	request := CompileDockerContainerPlanRequest{ObservationID: observation.ID,
		Manifest: manifest, OperationKey: "docker-plan-failure-operation",
		RequestedBy: "docker_plan_operator"}
	if _, err := service.CompileDockerContainerPlan(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition || len(failing.Snapshot()) != 0 {
		t.Fatalf("failed fake writer left state: snapshot=%#v err=%v", failing.Snapshot(), err)
	}
	values, err := service.ListDockerContainerPlans(ctx, run.ID, 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("failed fake writer left a durable plan: %#v err=%v", values, err)
	}

	claiming := &countingDockerContainerWriter{
		delegate: sandbox.NewInMemoryDockerWriteTransaction(),
		mutate: func(result *sandbox.DockerWriteSimulation) {
			result.ProductionSubmitted = true
		},
	}
	service.WithDockerContainerTransactionHarness(claiming)
	if _, err := service.CompileDockerContainerPlan(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition || claiming.calls != 1 {
		t.Fatalf("fake writer production claim was accepted: calls=%d err=%v", claiming.calls, err)
	}
	values, err = service.ListDockerContainerPlans(ctx, run.ID, 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("unsupported fake claim left a durable plan: %#v err=%v", values, err)
	}

	service.WithDockerContainerTransactionHarness(sandbox.NewInMemoryDockerWriteTransaction())
	plan, err := service.CompileDockerContainerPlan(ctx, request)
	if err != nil || plan.Replayed {
		t.Fatalf("pre-commit failure could not recover with the same operation: %#v err=%v", plan, err)
	}
}

func prepareDockerContainerPlanAuthority(t *testing.T, ctx context.Context,
	service *SandboxManifestService, runID, root, prefix, requestedBy string,
) (sandbox.Manifest, sandbox.DockerObservation) {
	t.Helper()
	for _, name := range []string{"src", "output"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manifest := sandboxManifestTestFixture()
	manifest.Backend = sandbox.BackendDocker
	manifest.Mounts = []sandbox.Mount{
		{Source: "src", Target: "/workspace", Access: sandbox.MountReadOnly},
		{Source: "output", Target: "/output", Access: sandbox.MountReadWrite},
	}
	manifest.Output.Paths = []string{"/output/report.json"}
	prepared, err := service.Prepare(ctx, PrepareSandboxManifestRequest{
		RunID: runID, Manifest: manifest, OperationKey: prefix + "-prepare",
		RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.RequestApproval(ctx, prepared.Preparation.ID, requestedBy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewApproval(ctx, prepared.Preparation.ID, approval.ActionApprove,
		prefix+"-review", requestedBy, ""); err != nil {
		t.Fatal(err)
	}
	validated, err := service.ValidateExecutionCandidate(ctx,
		ValidateSandboxExecutionCandidateRequest{PreparationID: prepared.Preparation.ID,
			Manifest: manifest, ApprovalID: record.ID, OperationKey: prefix + "-candidate",
			RequestedBy: requestedBy})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := service.BeginDisabledExecution(ctx, BeginSandboxExecutionRequest{
		CandidateID: validated.Candidate.ID, Manifest: manifest,
		OperationKey: prefix + "-begin", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := service.PrepareDisabledPreflight(ctx, PrepareSandboxPreflightRequest{
		ExecutionID: lifecycle.Execution.ID, Manifest: manifest,
		OperationKey: prefix + "-preflight", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	imageDigest := "sha256:" + strings.Repeat("7", 64)
	evidence, err := service.RecordSimulatedBackendEvidence(ctx,
		RecordSandboxBackendEvidenceRequest{PreflightID: preflight.ID, Manifest: manifest,
			ImageDigest: imageDigest, OperationKey: prefix + "-evidence", RequestedBy: requestedBy})
	if err != nil {
		t.Fatal(err)
	}
	fixture := sandbox.OutputFixture{ProtocolVersion: sandbox.OutputFixtureProtocolVersion,
		Outputs: []sandbox.OutputFixtureItem{
			{Kind: sandbox.OutputKindStdout, FileType: sandbox.OutputFileTypeStream, Content: "stdout"},
			{Kind: sandbox.OutputKindStderr, FileType: sandbox.OutputFileTypeStream, Content: "stderr"},
			{Kind: sandbox.OutputKindFile, FileType: sandbox.OutputFileTypeRegular, Content: "{}"},
		}}
	simulation, err := service.SimulateOutputTransaction(ctx, SimulateSandboxOutputRequest{
		EvidenceID: evidence.ID, Manifest: manifest, Fixture: fixture,
		OperationKey: prefix + "-simulation", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.WithDockerProductionObserver(sandbox.NewReadOnlyDockerProductionObserver(
		applicationDockerObservationTransport{imageDigest: imageDigest}))
	observation, err := service.ObserveDockerBackend(ctx, ObserveDockerBackendRequest{
		EvidenceID: evidence.ID, OutputSimulationID: simulation.ID, Manifest: manifest,
		OperationKey: prefix + "-observation", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest, observation
}
