package application

import (
	"context"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

func TestSandboxBackendEvidenceAndOutputSimulationRevalidateReplayAndRedact(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, preflight := prepareDockerSandboxPreflight(t, ctx, service, run.ID,
		"evidence", "evidence_operator")
	imageDigest := "sha256:" + strings.Repeat("a", 64)
	evidenceRequest := RecordSandboxBackendEvidenceRequest{
		PreflightID: preflight.ID, Manifest: manifest, ImageDigest: imageDigest,
		OperationKey: "evidence-record-operation", RequestedBy: "evidence_operator",
	}
	evidence, err := service.RecordSimulatedBackendEvidence(ctx, evidenceRequest)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Replayed || evidence.Report.TrustClass != sandbox.BackendEvidenceTrustSimulation ||
		evidence.Report.ProductionVerified || evidence.Report.BackendAvailable ||
		evidence.Report.BackendEnabled || evidence.Report.ExecutionAuthorized ||
		evidence.Report.ArtifactCommitAuthorized || len(evidence.Report.Items) != 16 {
		t.Fatalf("backend evidence widened authority: %#v", evidence)
	}
	replayedEvidence, err := service.RecordSimulatedBackendEvidence(ctx, evidenceRequest)
	if err != nil || !replayedEvidence.Replayed || replayedEvidence.ID != evidence.ID {
		t.Fatalf("backend evidence replay diverged: %#v err=%v", replayedEvidence, err)
	}
	secondEvidence := evidenceRequest
	secondEvidence.OperationKey = "evidence-second-operation"
	if _, err := service.RecordSimulatedBackendEvidence(ctx, secondEvidence); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("one preflight accepted multiple evidence roots: %v", err)
	}

	fixture := sandbox.OutputFixture{ProtocolVersion: sandbox.OutputFixtureProtocolVersion,
		Outputs: []sandbox.OutputFixtureItem{
			{Kind: sandbox.OutputKindStdout, FileType: sandbox.OutputFileTypeStream, Content: "ok\n"},
			{Kind: sandbox.OutputKindStderr, FileType: sandbox.OutputFileTypeStream,
				Content: "API_KEY=sk-123456789012345678901234567890\n"},
		}}
	simulationRequest := SimulateSandboxOutputRequest{
		EvidenceID: evidence.ID, Manifest: manifest, Fixture: fixture,
		OperationKey: "output-simulate-operation", RequestedBy: "evidence_operator",
	}
	simulation, err := service.SimulateOutputTransaction(ctx, simulationRequest)
	if err != nil {
		t.Fatal(err)
	}
	if simulation.Replayed || simulation.Status != sandbox.OutputSimulationStatusCommitted ||
		!simulation.SimulationOnly || !simulation.AllOrNothing ||
		simulation.FakeArtifactCount != 2 || simulation.ProductionArtifactCount != 0 ||
		simulation.ArtifactCommitAuthorized || simulation.BackendEnabled ||
		simulation.ExecutionAuthorized || !simulation.Descriptors[1].Redacted {
		t.Fatalf("output simulation widened authority or skipped redaction: %#v", simulation)
	}
	replayedSimulation, err := service.SimulateOutputTransaction(ctx, simulationRequest)
	if err != nil || !replayedSimulation.Replayed || replayedSimulation.ID != simulation.ID {
		t.Fatalf("output simulation replay diverged: %#v err=%v", replayedSimulation, err)
	}
	changed := simulationRequest
	changed.Fixture.Outputs = append([]sandbox.OutputFixtureItem(nil), fixture.Outputs...)
	changed.Fixture.Outputs[0].Content = "changed"
	if _, err := service.SimulateOutputTransaction(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed output fixture reused an operation key: %v", err)
	}
	if _, err := service.CancelDisabledExecution(ctx, CancelSandboxExecutionRequest{
		ExecutionID: evidence.ExecutionID, OperationKey: "evidence-cancel-operation",
		RequestedBy: "evidence_operator",
	}); err != nil {
		t.Fatal(err)
	}
	afterCancel := simulationRequest
	afterCancel.OperationKey = "output-after-cancel-operation"
	if _, err := service.SimulateOutputTransaction(ctx, afterCancel); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("cancelled execution accepted another output simulation: %v", err)
	}

	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range timeline {
		if event.Type != events.SandboxBackendEvidenceRecordedEvent &&
			event.Type != events.SandboxOutputSimulationRecordedEvent {
			continue
		}
		if strings.Contains(event.PayloadJSON, root) ||
			strings.Contains(event.PayloadJSON, "sk-123456") ||
			strings.Contains(event.PayloadJSON, imageDigest) ||
			strings.Contains(event.PayloadJSON, preflight.OutputPlan.Slots[0].LocatorFingerprint) ||
			strings.Contains(event.PayloadJSON, simulation.FixtureDigest) {
			t.Fatalf("v52 event leaked private evidence or output data: %#v", event)
		}
	}
}

func TestSandboxOutputSimulationFailedFakeCommitLeavesNoLedger(t *testing.T) {
	ctx := context.Background()
	st, run, _ := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, preflight := prepareDockerSandboxPreflight(t, ctx, service, run.ID,
		"rollback", "evidence_operator")
	evidence, err := service.RecordSimulatedBackendEvidence(ctx, RecordSandboxBackendEvidenceRequest{
		PreflightID: preflight.ID, Manifest: manifest,
		ImageDigest:  "sha256:" + strings.Repeat("b", 64),
		OperationKey: "rollback-evidence", RequestedBy: "evidence_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := sandbox.OutputFixture{ProtocolVersion: sandbox.OutputFixtureProtocolVersion,
		Outputs: []sandbox.OutputFixtureItem{
			{Kind: sandbox.OutputKindStdout, FileType: sandbox.OutputFileTypeStream, Content: "out"},
			{Kind: sandbox.OutputKindStderr, FileType: sandbox.OutputFileTypeStream, Content: "err"},
		}}
	service.outputHarness = sandbox.InMemoryOutputHarness{FailCommitAtOrdinal: 2}
	request := SimulateSandboxOutputRequest{
		EvidenceID: evidence.ID, Manifest: manifest, Fixture: fixture,
		OperationKey: "rollback-output-operation", RequestedBy: "evidence_operator",
	}
	if _, err := service.SimulateOutputTransaction(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("injected fake commit failure was not surfaced: %v", err)
	}
	values, err := service.ListOutputSimulations(ctx, run.ID, 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("failed fake commit left a durable simulation: %#v err=%v", values, err)
	}
	service.outputHarness = sandbox.NewInMemoryOutputHarness()
	if _, err := service.SimulateOutputTransaction(ctx, request); err != nil {
		t.Fatalf("same operation could not recover after pre-commit failure: %v", err)
	}
}

func prepareDockerSandboxPreflight(t *testing.T, ctx context.Context,
	service *SandboxManifestService, runID, prefix, requestedBy string,
) (sandbox.Manifest, sandbox.DisabledPreflight) {
	t.Helper()
	manifest := sandboxManifestTestFixture()
	manifest.Backend = sandbox.BackendDocker
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
	decision, err := service.ReviewApproval(ctx, prepared.Preparation.ID, approval.ActionApprove,
		prefix+"-review-operation", requestedBy, "")
	if err != nil || decision.Approval.ID != record.ID {
		t.Fatalf("approve Docker Sandbox: %#v err=%v", decision, err)
	}
	validated, err := service.ValidateExecutionCandidate(ctx,
		ValidateSandboxExecutionCandidateRequest{
			PreparationID: prepared.Preparation.ID, Manifest: manifest, ApprovalID: record.ID,
			OperationKey: prefix + "-candidate-operation", RequestedBy: requestedBy,
		})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := service.BeginDisabledExecution(ctx, BeginSandboxExecutionRequest{
		CandidateID: validated.Candidate.ID, Manifest: manifest,
		OperationKey: prefix + "-begin-operation", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := service.PrepareDisabledPreflight(ctx, PrepareSandboxPreflightRequest{
		ExecutionID: lifecycle.Execution.ID, Manifest: manifest,
		OperationKey: prefix + "-preflight-operation", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest, preflight
}
