package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/runmutation"
)

func TestRunModeLedgerIsImmutable(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "run-mode-immutable.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "immutable mode", Surface: "cyber", Phase: "plan",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "immutable-mode-0001",
		RequestedBy: "operator", Reason: "approved plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE run_mode_snapshots SET phase = 'plan' WHERE id = ?`,
		`DELETE FROM run_mode_snapshots WHERE id = ?`,
	} {
		if _, err := st.db.ExecContext(ctx, statement, result.Mode.ID); err == nil {
			t.Fatalf("immutable mode statement succeeded: %s", statement)
		}
	}
	for _, statement := range []string{
		`UPDATE run_mode_operations SET requested_by = 'other' WHERE snapshot_id = ?`,
		`DELETE FROM run_mode_operations WHERE snapshot_id = ?`,
	} {
		if _, err := st.db.ExecContext(ctx, statement, result.Mode.ID); err == nil {
			t.Fatalf("immutable operation statement succeeded: %s", statement)
		}
	}
	loaded, err := st.GetRunMode(ctx, run.ID)
	if err != nil || loaded.ID != result.Mode.ID || loaded.Phase != domain.ExecutionPhaseDeliver {
		t.Fatalf("immutable mode changed: mode=%#v err=%v", loaded, err)
	}
}

func TestSchemaV40BackfillsCompatibilityRunMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema-v40-mode.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "legacy v40 Run", Profile: "review", Surface: "cyber", Phase: "plan",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV41ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("downgrade v41 fixture with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	mode, err := upgraded.GetRunMode(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Surface != domain.ExecutionSurfaceCode ||
		mode.Phase != domain.ExecutionPhaseDeliver || mode.Revision != 1 ||
		mode.RequestedBy != "schema_v41" || mode.Profile != domain.ProfileReview {
		t.Fatalf("unexpected compatibility mode: %#v", mode)
	}
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestPlanCompletionFailsAtEveryPersistenceBoundary(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "run-mode-completion-guard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "plan completion persistence guard", Phase: "plan",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	turn, err := st.BeginSupervisorTurn(ctx, lease, "prepare a bounded plan")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "mock", Model: "mock-code",
	}
	inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("model start inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeSuccess
	attempt.Elapsed = time.Millisecond
	response := llm.ChatResponse{
		Text: "plan ready", Provider: attempt.Provider, Model: attempt.Model,
		Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	}
	checkpoint, err := st.RecordSupervisorModelCompleted(ctx, turn.Checkpoint, attempt, response)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = st.CompleteSupervisorTurn(ctx, checkpoint, response, domain.RootAction{
		Version: domain.RootLifecycleVersion, Kind: domain.RootActionFinish,
		Message: "done", Summary: "plan complete",
	}, policy.Decision{Allowed: true, Reason: "allowed"}, time.Millisecond)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		!strings.Contains(err.Error(), "persistence boundary") {
		t.Fatalf("direct supervisor completion error = %v", err)
	}
	stored, err := st.GetRun(ctx, run.ID)
	if err != nil || stored.Status != domain.RunRunning {
		t.Fatalf("rejected completion changed Run: status=%s err=%v", stored.Status, err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE runs SET status = 'completed' WHERE id = ?`,
		run.ID); err == nil || !strings.Contains(err.Error(), "Plan-phase") {
		t.Fatalf("SQLite plan completion guard error = %v", err)
	}
}

func TestRunModeLeaseGuardUsesStoreClock(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "run-mode-store-clock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "lease guard store clock", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_ = acquireTestRunExecutionLease(t, ctx, st, run.ID)
	if _, err := service.Pause(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	current, err := st.GetRunMode(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	scopeJSON, err := json.Marshal(current.Scope)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.db.ExecContext(ctx, `INSERT INTO run_mode_snapshots
		(id, run_id, mission_id, revision, protocol_version, surface, phase, profile,
		scope_json, policy_version, requested_by, reason, created_at)
		VALUES (?, ?, ?, 2, ?, ?, 'plan', ?, ?, ?, 'operator', 'future timestamp', ?)`,
		"run-mode-future-clock", current.RunID, current.MissionID,
		current.ProtocolVersion, current.Surface, current.Profile, string(scopeJSON),
		current.PolicyVersion, ts(time.Now().UTC().Add(24*time.Hour)))
	if err == nil || !strings.Contains(err.Error(), "binding or transition") {
		t.Fatalf("future caller timestamp bypassed active lease: %v", err)
	}
}

func TestRunModeStoreRejectsUnredactedTransition(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "run-mode-redaction-boundary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "redaction boundary", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := st.GetRunMode(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := current.Next("run-mode-unredacted", domain.ExecutionPhasePlan,
		"operator", "API_KEY=sk-"+strings.Repeat("a", 32), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	operation := domain.RunModeOperation{
		KeyDigest: runmutation.Fingerprint("run_mode_operation.v1", run.ID,
			"unredacted-store-key-0001"),
		RequestFingerprint: runModeRequestFingerprint(next), SnapshotID: next.ID,
		RunID: next.RunID, RequestedBy: next.RequestedBy, CreatedAt: next.CreatedAt,
	}
	event, err := events.New(run.ID, next.MissionID, events.RunPhaseChangedEvent,
		"run_mode", next.ID, map[string]any{
			"protocol": next.ProtocolVersion, "revision": next.Revision,
			"surface": next.Surface, "from": current.Phase, "to": next.Phase,
			"policy_version": next.PolicyVersion, "network_mode": next.Scope.NetworkMode,
			"allowed_target_count": len(next.Scope.AllowedTargets),
			"requested_by":         next.RequestedBy, "reason": next.Reason,
			"capability_grant": false,
		})
	if err != nil {
		t.Fatal(err)
	}
	event.CreatedAt = next.CreatedAt
	if _, _, err := st.TransitionRunPhase(ctx, next, operation, event); apperror.CodeOf(err) != apperror.CodeInvalidArgument ||
		!strings.Contains(err.Error(), "redacted") {
		t.Fatalf("unredacted Store transition error = %v", err)
	}
	stored, err := st.GetRunMode(ctx, run.ID)
	if err != nil || stored.Revision != 1 {
		t.Fatalf("rejected Store transition changed mode: %#v err=%v", stored, err)
	}
}
