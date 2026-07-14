package application

import (
	"context"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

func TestSandboxDisabledPreflightRevalidatesBindsAndReplays(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest := sandboxManifestTestFixture()
	validated := prepareSandboxLifecycleCandidate(t, ctx, service, run.ID, manifest,
		"preflight", "preflight_operator")
	lifecycle, err := service.BeginDisabledExecution(ctx, BeginSandboxExecutionRequest{
		CandidateID: validated.Candidate.ID, Manifest: manifest,
		OperationKey: "preflight-begin-operation", RequestedBy: "preflight_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := PrepareSandboxPreflightRequest{
		ExecutionID: lifecycle.Execution.ID, Manifest: manifest,
		OperationKey: "preflight-create-operation", RequestedBy: "preflight_operator",
	}
	created, err := service.PrepareDisabledPreflight(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Replayed || created.ExecutionID != lifecycle.Execution.ID ||
		created.Status != sandbox.PreflightStatusBackendDisabled || created.BackendEnabled ||
		created.ExecutionAuthorized || created.ArtifactCommitAuthorized ||
		created.Handshake.Available || created.Handshake.ContainerIdentity.Bound ||
		len(created.Handshake.Checks) != sandbox.MaxBackendChecks ||
		created.OutputPlan.SlotCount != 2 || created.OutputPlan.RawPathsStored ||
		created.OutputPlan.ExportEnabled || created.OutputPlan.ArtifactCommitAuthorized ||
		created.OutputPlan.PartialFailurePolicy != sandbox.OutputPartialFailureAllOrNothing ||
		created.OutputPlan.TruncationPolicy != sandbox.OutputTruncationAggregateHardCap ||
		created.OutputPlan.Fingerprint == "" {
		t.Fatalf("disabled Sandbox preflight widened authority: %#v", created)
	}
	for _, check := range created.Handshake.Checks {
		if !check.Required || check.Verified || check.EvidenceState != sandbox.BackendCheckEvidenceNotProbed {
			t.Fatalf("backend check made an unsupported claim: %#v", check)
		}
	}
	for _, slot := range created.OutputPlan.Slots {
		if slot.LocatorFingerprint == "" || slot.ArtifactCommitAuthorized {
			t.Fatalf("output slot was not opaque and disabled: %#v", slot)
		}
	}

	replayed, err := service.PrepareDisabledPreflight(ctx, request)
	if err != nil || !replayed.Replayed || replayed.ID != created.ID ||
		replayed.OutputPlan.Fingerprint != created.OutputPlan.Fingerprint {
		t.Fatalf("Sandbox preflight replay diverged: %#v err=%v", replayed, err)
	}
	secondKey := request
	secondKey.OperationKey = "preflight-second-create-operation"
	if _, err := service.PrepareDisabledPreflight(ctx, secondKey); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("second operation created another preflight for one execution: %v", err)
	}
	changed := request
	changed.Manifest.Resources.MaxOutputBytes /= 2
	if _, err := service.PrepareDisabledPreflight(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed Manifest reused a preflight operation: %v", err)
	}
	loaded, err := service.GetDisabledPreflight(ctx, created.ID)
	if err != nil || loaded.OutputPlan.Fingerprint != created.OutputPlan.Fingerprint {
		t.Fatalf("stored Sandbox preflight did not round trip: %#v err=%v", loaded, err)
	}
	listed, err := service.ListDisabledPreflights(ctx, run.ID, 10)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("Sandbox preflight list diverged: %#v err=%v", listed, err)
	}

	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range timeline {
		if event.Type != events.SandboxPreflightRecordedEvent {
			continue
		}
		count++
		if strings.Contains(event.PayloadJSON, root) ||
			strings.Contains(event.PayloadJSON, "go test") ||
			strings.Contains(event.PayloadJSON, created.OutputPlan.Slots[0].LocatorFingerprint) ||
			strings.Contains(event.PayloadJSON, created.OutputPlan.Fingerprint) {
			t.Fatalf("Sandbox preflight event leaked private intent: %#v", event)
		}
	}
	if count != 1 {
		t.Fatalf("Sandbox preflight event count is %d, want 1", count)
	}
}

func TestSandboxDisabledPreflightRejectsBackendClaimsAndCancelledExecution(t *testing.T) {
	ctx := context.Background()
	st, run, _ := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest := sandboxManifestTestFixture()
	validated := prepareSandboxLifecycleCandidate(t, ctx, service, run.ID, manifest,
		"preflight-reject", "preflight_operator")
	lifecycle, err := service.BeginDisabledExecution(ctx, BeginSandboxExecutionRequest{
		CandidateID: validated.Candidate.ID, Manifest: manifest,
		OperationKey: "preflight-reject-begin", RequestedBy: "preflight_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	service.inspector = forgedAvailableBackendInspector{}
	request := PrepareSandboxPreflightRequest{
		ExecutionID: lifecycle.Execution.ID, Manifest: manifest,
		OperationKey: "preflight-reject-create", RequestedBy: "preflight_operator",
	}
	if _, err := service.PrepareDisabledPreflight(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("forged backend availability entered preflight: %v", err)
	}
	service.inspector = sandbox.NewDisabledBackendInspector()
	if _, err := service.CancelDisabledExecution(ctx, CancelSandboxExecutionRequest{
		ExecutionID: lifecycle.Execution.ID, OperationKey: "preflight-reject-cancel",
		RequestedBy: "preflight_operator",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareDisabledPreflight(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("cancelled Sandbox execution entered preflight: %v", err)
	}
}

type forgedAvailableBackendInspector struct{}

func (forgedAvailableBackendInspector) Inspect(ctx context.Context,
	backend sandbox.Backend,
) (sandbox.BackendHandshake, error) {
	handshake, err := sandbox.NewDisabledBackendInspector().Inspect(ctx, backend)
	if err == nil {
		handshake.Available = true
	}
	return handshake, err
}
