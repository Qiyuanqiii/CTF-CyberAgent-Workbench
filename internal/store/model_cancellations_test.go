package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
)

func TestModelCancellationLedgerIsExactIdempotentRedactedAndResolved(t *testing.T) {
	st, err := Open(t.TempDir() + "/model-cancellation.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "test cross-process cancellation ledger", Profile: "code", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = application.NewRunService(st).Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "cancellation-test-worker", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquired.Lease, "cancel this call")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test-provider", Model: "test-model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt); err != nil || !inserted {
		t.Fatalf("model start inserted=%t err=%v", inserted, err)
	}
	key := "cancel-operation-0123456789"
	secret := "sk-" + strings.Repeat("z", 32)
	request := domain.RequestModelCancellation{
		RunID: run.ID, AttemptID: turn.Checkpoint.AttemptID, ModelAttempt: 1,
		IdempotencyKey: key, Reason: "operator token=" + secret, RequestedBy: "http_control",
	}
	created, err := st.RequestSupervisorModelCancellation(ctx, request)
	if err != nil || created.Replayed || created.Cancellation.Status != domain.ModelCancellationPending {
		t.Fatalf("unexpected cancellation create: %#v err=%v", created, err)
	}
	if strings.Contains(created.Cancellation.Reason, secret) ||
		!strings.Contains(created.Cancellation.Reason, "[REDACTED:secret]") {
		t.Fatalf("cancellation reason was not redacted: %q", created.Cancellation.Reason)
	}
	replayed, err := st.RequestSupervisorModelCancellation(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Cancellation.ID != created.Cancellation.ID {
		t.Fatalf("idempotent replay changed cancellation: %#v err=%v", replayed, err)
	}
	changed := request
	changed.ModelAttempt = 2
	if _, err := st.RequestSupervisorModelCancellation(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed idempotent intent code=%s err=%v", apperror.CodeOf(err), err)
	}
	duplicateKey := request
	duplicateKey.IdempotencyKey = "second-operation-0123456789"
	if _, err := st.RequestSupervisorModelCancellation(ctx, duplicateKey); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("second target key code=%s err=%v", apperror.CodeOf(err), err)
	}
	var operationCount int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_model_cancellation_operations
		WHERE cancellation_id = ?`, created.Cancellation.ID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 {
		t.Fatalf("cancellation has %d idempotency operations", operationCount)
	}
	var rawKeys int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_model_cancellation_operations
		WHERE operation_key_digest IN (?, ?)`, key, duplicateKey.IdempotencyKey).Scan(&rawKeys); err != nil {
		t.Fatal(err)
	}
	if rawKeys != 0 {
		t.Fatal("raw cancellation idempotency key was persisted")
	}

	wrong := turn.Checkpoint
	wrong.LeaseGeneration++
	if _, _, err := st.ObserveSupervisorModelCancellation(ctx, wrong, attempt); err == nil {
		t.Fatal("stale supervisor lease observed a cancellation")
	}
	observed, transitioned, err := st.ObserveSupervisorModelCancellation(ctx, turn.Checkpoint, attempt)
	if err != nil || !transitioned || observed.Status != domain.ModelCancellationObserved || observed.ObservedAt == nil {
		t.Fatalf("cancellation observation failed: %#v transitioned=%t err=%v", observed, transitioned, err)
	}
	if _, transitioned, err := st.ObserveSupervisorModelCancellation(ctx, turn.Checkpoint, attempt); err != nil || transitioned {
		t.Fatalf("cancellation observation replay transitioned=%t err=%v", transitioned, err)
	}
	attempt.Outcome = llm.OutcomeCancelled
	attempt.ErrorText = "provider call cancelled"
	if _, err := st.RecordSupervisorModelFailed(ctx, turn.Checkpoint, attempt); err != nil {
		t.Fatal(err)
	}
	resolved, err := st.GetModelCancellation(ctx, created.Cancellation.ID)
	if err != nil || resolved.Status != domain.ModelCancellationResolved ||
		resolved.Resolution != string(llm.OutcomeCancelled) || resolved.ResolvedAt == nil {
		t.Fatalf("cancellation was not resolved with model terminal state: %#v err=%v", resolved, err)
	}
	eventItems, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	requested, observedEvents := 0, 0
	for _, event := range eventItems {
		switch event.Type {
		case events.ModelCancelRequestedEvent:
			requested++
			if strings.Contains(event.PayloadJSON, secret) {
				t.Fatal("cancellation request event leaked a secret")
			}
		case events.ModelCancelObservedEvent:
			observedEvents++
			if strings.Contains(event.PayloadJSON, acquired.Lease.LeaseID) {
				t.Fatal("cancellation observation event exposed its fencing token")
			}
		}
	}
	if requested != 1 || observedEvents != 1 {
		t.Fatalf("unexpected cancellation audit counts: requested=%d observed=%d", requested, observedEvents)
	}
}

func TestModelCancellationRejectsInactiveOrMismatchedAttempt(t *testing.T) {
	st, err := Open(t.TempDir() + "/model-cancellation-reject.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "reject stale cancellation", Profile: "code", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := domain.RequestModelCancellation{
		RunID: run.ID, AttemptID: "attempt-missing", ModelAttempt: 1,
		IdempotencyKey: "inactive-operation-01234567", RequestedBy: "http_control",
	}
	if _, err := st.RequestSupervisorModelCancellation(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("inactive cancellation code=%s err=%v", apperror.CodeOf(err), err)
	}
	request.RunID = "run-missing"
	request.IdempotencyKey = "missing-run-operation-012345"
	if _, err := st.RequestSupervisorModelCancellation(ctx, request); apperror.CodeOf(apperror.Normalize(err)) != apperror.CodeNotFound {
		t.Fatalf("missing Run cancellation code=%s err=%v", apperror.CodeOf(apperror.Normalize(err)), err)
	}
}

func TestNewModelAttemptResolvesOrphanedCancellationAsSuperseded(t *testing.T) {
	st, err := Open(t.TempDir() + "/model-cancellation-superseded.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "resolve orphaned cancellation", Profile: "code", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	lease, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "supersede-test-worker", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, lease.Lease, "recover after worker exit")
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test-provider", Model: "test-model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, firstAttempt); err != nil || !inserted {
		t.Fatalf("first model start inserted=%t err=%v", inserted, err)
	}
	requested, err := st.RequestSupervisorModelCancellation(ctx, domain.RequestModelCancellation{
		RunID: run.ID, AttemptID: turn.Checkpoint.AttemptID, ModelAttempt: 1,
		IdempotencyKey: "orphaned-cancel-operation-012345", RequestedBy: "http_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RequestSupervisorModelCancellation(ctx, domain.RequestModelCancellation{
		RunID: run.ID, AttemptID: turn.Checkpoint.AttemptID, ModelAttempt: 1,
		IdempotencyKey: "secret-requester-operation-0123", RequestedBy: "sk-" + strings.Repeat("x", 32),
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("sensitive requester code=%s err=%v", apperror.CodeOf(err), err)
	}
	secondAttempt := firstAttempt
	secondAttempt.Number = 2
	secondAttempt.TransportAttempt = 2
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, secondAttempt); err != nil || !inserted {
		t.Fatalf("replacement model start inserted=%t err=%v", inserted, err)
	}
	resolved, err := st.GetModelCancellation(ctx, requested.Cancellation.ID)
	if err != nil || resolved.Status != domain.ModelCancellationResolved || resolved.Resolution != "superseded" {
		t.Fatalf("orphaned cancellation was not superseded: %#v err=%v", resolved, err)
	}
	thirdAttempt := firstAttempt
	thirdAttempt.Number = 3
	thirdAttempt.TransportAttempt = 3
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, thirdAttempt); err != nil || !inserted {
		t.Fatalf("third model start inserted=%t err=%v", inserted, err)
	}
	if _, err := st.RequestSupervisorModelCancellation(ctx, domain.RequestModelCancellation{
		RunID: run.ID, AttemptID: turn.Checkpoint.AttemptID, ModelAttempt: 2,
		IdempotencyKey: "stale-model-attempt-012345678", RequestedBy: "http_control",
	}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("stale model cancellation code=%s err=%v", apperror.CodeOf(err), err)
	}
}
