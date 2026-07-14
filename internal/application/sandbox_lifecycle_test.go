package application

import (
	"context"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
	"cyberagent-workbench/internal/toolgateway"
)

func TestSandboxDisabledLifecycleRecoversCancelsCleansAndReplays(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest := sandboxManifestTestFixture()
	validated := prepareSandboxLifecycleCandidate(t, ctx, service, run.ID, manifest,
		"lifecycle", "lifecycle_operator")
	begin := BeginSandboxExecutionRequest{
		CandidateID: validated.Candidate.ID, Manifest: manifest,
		OperationKey: "lifecycle-begin-operation", RequestedBy: "lifecycle_operator",
	}
	started, err := service.BeginDisabledExecution(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	if started.Replayed || started.Status != sandbox.LifecyclePrepared ||
		started.Execution.BackendEnabled || started.Execution.ExecutionAuthorized ||
		started.Execution.BackendStarted || started.Lease.Status != sandbox.ExecutionLeaseReleased ||
		started.Execution.CandidateID != validated.Candidate.ID || len(started.Inputs) != 0 {
		t.Fatalf("unexpected disabled Sandbox lifecycle: %#v", started)
	}
	replayed, err := service.BeginDisabledExecution(ctx, begin)
	if err != nil || !replayed.Replayed || replayed.Execution.ID != started.Execution.ID ||
		replayed.Lease.Status != sandbox.ExecutionLeaseReleased {
		t.Fatalf("Sandbox lifecycle replay diverged: %#v err=%v", replayed, err)
	}
	changed := begin
	changed.Manifest.Command.Arguments = []string{"test", "./internal/..."}
	if _, err := service.BeginDisabledExecution(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed Manifest reused begin operation key: %v", err)
	}

	cancel := CancelSandboxExecutionRequest{
		ExecutionID: started.Execution.ID, OperationKey: "lifecycle-cancel-operation",
		RequestedBy: "lifecycle_operator",
	}
	cancelled, err := service.CancelDisabledExecution(ctx, cancel)
	if err != nil || cancelled.Cancellation == nil ||
		cancelled.Status != sandbox.LifecycleCancelPending || cancelled.Replayed {
		t.Fatalf("Sandbox cancellation was not recorded: %#v err=%v", cancelled, err)
	}
	cancelReplay, err := service.CancelDisabledExecution(ctx, cancel)
	if err != nil || !cancelReplay.Replayed ||
		cancelReplay.Cancellation.ID != cancelled.Cancellation.ID {
		t.Fatalf("Sandbox cancellation replay diverged: %#v err=%v", cancelReplay, err)
	}
	if _, err := NewRunService(st).Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	clean := CleanupSandboxExecutionRequest{
		ExecutionID: started.Execution.ID, OperationKey: "lifecycle-cleanup-operation",
		ReconciledBy: "lifecycle_operator",
	}
	cleaned, err := service.CleanupDisabledExecution(ctx, clean)
	if err != nil || cleaned.Cleanup == nil ||
		cleaned.Status != sandbox.LifecycleCleanupComplete || cleaned.Replayed ||
		cleaned.Cleanup.Outcome != "backend_disabled" || cleaned.Cleanup.BackendStarted ||
		cleaned.Cleanup.OrphanDetected || cleaned.Cleanup.OrphanReaped ||
		!cleaned.Cleanup.InputArtifactsVerified || cleaned.Cleanup.OutputArtifactCount != 0 ||
		cleaned.Lease.Status != sandbox.ExecutionLeaseReleased ||
		cleaned.Lease.Generation != 2 {
		t.Fatalf("terminal-Run cleanup did not converge: %#v err=%v", cleaned, err)
	}
	cleanReplay, err := service.CleanupDisabledExecution(ctx, clean)
	if err != nil || !cleanReplay.Replayed || cleanReplay.Cleanup.ID != cleaned.Cleanup.ID {
		t.Fatalf("Sandbox cleanup replay diverged: %#v err=%v", cleanReplay, err)
	}

	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]int{
		events.SandboxExecutionPreparedEvent:         1,
		events.SandboxExecutionCancelRequestedEvent:  1,
		events.SandboxExecutionCleanupCompletedEvent: 1,
	}
	for _, event := range timeline {
		if _, ok := wanted[event.Type]; ok {
			wanted[event.Type]--
		}
		if strings.Contains(event.PayloadJSON, root) ||
			strings.Contains(event.PayloadJSON, started.Execution.InitialLeaseID) ||
			strings.Contains(event.PayloadJSON, started.Lease.OwnerID) ||
			strings.Contains(event.PayloadJSON, cleaned.Lease.OwnerID) ||
			strings.Contains(event.PayloadJSON, `"executable"`) {
			t.Fatalf("Sandbox lifecycle event leaked private intent or lease data: %#v", event)
		}
	}
	for eventType, remaining := range wanted {
		if remaining != 0 {
			t.Fatalf("Sandbox lifecycle event %s count mismatch: remaining=%d", eventType, remaining)
		}
	}
}

func TestSandboxDisabledLifecycleBindsInputArtifactAndRejectsCrossRunScope(t *testing.T) {
	ctx := context.Background()
	st, run, _ := newSandboxManifestTestRuntime(t, ctx)
	gateway := toolgateway.New(st, policy.NewDefaultChecker())
	proposal, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo lifecycle-evidence"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-sandbox",
		RequestedBy: "lifecycle_operator",
	})
	if err != nil || proposal.Proposal == nil {
		t.Fatalf("create Artifact source: %#v err=%v", proposal, err)
	}
	reviewed, err := gateway.Review(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ShellTool,
		ProposalID: proposal.Proposal.ID, ReviewedBy: "lifecycle_operator",
	})
	if err != nil || reviewed.Result == nil || reviewed.Result.Metadata["artifact_stdout_id"] == "" {
		t.Fatalf("capture lifecycle Artifact: %#v err=%v", reviewed, err)
	}
	artifactID := reviewed.Result.Metadata["artifact_stdout_id"]
	manifest := sandboxManifestTestFixture()
	manifest.InputArtifactIDs = []string{artifactID}
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	validated := prepareSandboxLifecycleCandidate(t, ctx, service, run.ID, manifest,
		"artifact", "lifecycle_operator")
	started, err := service.BeginDisabledExecution(ctx, BeginSandboxExecutionRequest{
		CandidateID: validated.Candidate.ID, Manifest: manifest,
		OperationKey: "artifact-begin-operation", RequestedBy: "lifecycle_operator",
	})
	if err != nil || len(started.Inputs) != 1 || started.Inputs[0].ArtifactID != artifactID ||
		started.Inputs[0].SHA256 == "" || started.Execution.InputArtifactBytes <= 0 ||
		started.Execution.InputArtifactDigest != sandbox.InputArtifactBindingsDigest(started.Inputs) {
		t.Fatalf("input Artifact binding is incomplete: %#v err=%v", started, err)
	}

	_, otherRun, err := NewRunService(st).Create(ctx, CreateRunRequest{
		Goal: "reject cross Run sandbox Artifact", Profile: "code", WorkspaceID: "ws-sandbox",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherManifest := sandboxManifestTestFixture()
	otherManifest.InputArtifactIDs = []string{artifactID}
	prepared, err := service.Prepare(ctx, PrepareSandboxManifestRequest{
		RunID: otherRun.ID, Manifest: otherManifest, OperationKey: "cross-run-prepare",
		RequestedBy: "lifecycle_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := service.ValidateExecutionCandidate(ctx, ValidateSandboxExecutionCandidateRequest{
		PreparationID: prepared.Preparation.ID, Manifest: otherManifest,
		OperationKey: "cross-run-candidate", RequestedBy: "lifecycle_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BeginDisabledExecution(ctx, BeginSandboxExecutionRequest{
		CandidateID: candidate.Candidate.ID, Manifest: otherManifest,
		OperationKey: "cross-run-begin-operation", RequestedBy: "lifecycle_operator",
	}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("cross-Run Artifact entered Sandbox lifecycle: %v", err)
	}
}

func prepareSandboxLifecycleCandidate(t *testing.T, ctx context.Context,
	service *SandboxManifestService, runID string, manifest sandbox.Manifest, prefix, requestedBy string,
) sandbox.ValidatedExecutionCandidate {
	t.Helper()
	prepared, err := service.Prepare(ctx, PrepareSandboxManifestRequest{
		RunID: runID, Manifest: manifest,
		OperationKey: prefix + "-prepare", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	validated, err := service.ValidateExecutionCandidate(ctx, ValidateSandboxExecutionCandidateRequest{
		PreparationID: prepared.Preparation.ID, Manifest: manifest,
		OperationKey: prefix + "-candidate", RequestedBy: requestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	return validated
}
