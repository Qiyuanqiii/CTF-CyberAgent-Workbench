package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
)

func TestSpecialistModelCancellationIsExactIdempotentObservedAndResolved(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st,
		"Specialist cancellation ledger", 2, 64)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")),
		"specialist-cancellation-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	ref := attemptRef(attempt)
	modelAttempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "mock", Model: "mock-code",
	}
	if inserted, err := st.RecordSpecialistModelStarted(ctx, ref, modelAttempt); err != nil || !inserted {
		t.Fatalf("Specialist model did not start: inserted=%t err=%v", inserted, err)
	}
	request := domain.RequestSpecialistModelCancellation{
		RunID: fixture.Run.ID, AgentID: fixture.Child.ID, AttemptID: attempt.ID,
		ModelAttempt: 1, IdempotencyKey: "specialist-cancel-operation-0001",
		Reason: "operator stopped one child", RequestedBy: "store_test",
	}
	created, err := st.RequestSpecialistModelCancellation(ctx, request)
	if err != nil || created.Replayed || created.Cancellation.Status != domain.ModelCancellationPending {
		t.Fatalf("Specialist cancellation was not created: result=%#v err=%v", created, err)
	}
	replayed, err := st.RequestSpecialistModelCancellation(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Cancellation.ID != created.Cancellation.ID {
		t.Fatalf("Specialist cancellation replay drifted: result=%#v err=%v", replayed, err)
	}
	changed := request
	changed.Reason = "changed intent"
	if _, err := st.RequestSpecialistModelCancellation(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed Specialist cancellation replay was accepted: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	duplicate := request
	duplicate.IdempotencyKey = "specialist-cancel-operation-0002"
	if _, err := st.RequestSpecialistModelCancellation(ctx, duplicate); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("duplicate Specialist target was accepted: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	observed, changedState, err := st.ObserveSpecialistModelCancellation(ctx, ref, modelAttempt)
	if err != nil || !changedState || observed.Status != domain.ModelCancellationObserved ||
		observed.ObservedAt == nil {
		t.Fatalf("Specialist cancellation was not observed: cancellation=%#v changed=%t err=%v",
			observed, changedState, err)
	}
	if _, changedState, err := st.ObserveSpecialistModelCancellation(ctx, ref, modelAttempt); err != nil || changedState {
		t.Fatalf("Specialist cancellation observation replay changed state: changed=%t err=%v",
			changedState, err)
	}
	modelAttempt.Outcome = llm.OutcomeCancelled
	modelAttempt.ErrorText = "provider call cancelled"
	modelAttempt.Elapsed = time.Millisecond
	if _, err := st.RecordSpecialistModelFailed(ctx, ref, modelAttempt, nil); err != nil {
		t.Fatal(err)
	}
	resolved, err := st.GetSpecialistModelCancellation(ctx, created.Cancellation.ID)
	if err != nil || resolved.Status != domain.ModelCancellationResolved ||
		resolved.Resolution != string(llm.OutcomeCancelled) || resolved.ResolvedAt == nil {
		t.Fatalf("Specialist cancellation did not resolve with model outcome: %#v err=%v",
			resolved, err)
	}
	var rawKeyCount int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM specialist_model_cancellation_operations
		WHERE operation_key_digest = ? OR request_fingerprint = ?`,
		request.IdempotencyKey, request.IdempotencyKey).Scan(&rawKeyCount); err != nil {
		t.Fatal(err)
	}
	if rawKeyCount != 0 {
		t.Fatal("Specialist cancellation operation stored its raw idempotency key")
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE specialist_model_cancellation_operations
		SET request_fingerprint = 'changed' WHERE cancellation_id = ?`,
		created.Cancellation.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("Specialist cancellation operation was mutable: %v", err)
	}
	eventLog, err := st.ListRunEvents(ctx, fixture.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countRunEventType(eventLog, events.ModelCancelRequestedEvent) != 1 ||
		countRunEventType(eventLog, events.ModelCancelObservedEvent) != 1 {
		t.Fatalf("Specialist cancellation audit events are incomplete: %#v", eventLog)
	}
	for _, event := range eventLog {
		if strings.Contains(event.PayloadJSON, fixture.Lease.LeaseID) ||
			strings.Contains(event.PayloadJSON, request.IdempotencyKey) {
			t.Fatalf("Specialist cancellation event exposed private control data: %s",
				event.PayloadJSON)
		}
	}
}

func TestSpecialistModelCancellationResolvesWhenAttemptTerminates(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st,
		"Specialist cancellation termination", 1, 64)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")),
		"specialist-cancellation-terminal-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	ref := attemptRef(attempt)
	modelAttempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "mock", Model: "mock-code",
	}
	if inserted, err := st.RecordSpecialistModelStarted(ctx, ref, modelAttempt); err != nil || !inserted {
		t.Fatalf("Specialist model did not start: inserted=%t err=%v", inserted, err)
	}
	created, err := st.RequestSpecialistModelCancellation(ctx,
		domain.RequestSpecialistModelCancellation{
			RunID: fixture.Run.ID, AgentID: fixture.Child.ID, AttemptID: attempt.ID,
			ModelAttempt: 1, IdempotencyKey: "specialist-terminal-cancel-0001",
			RequestedBy: "store_test",
		})
	if err != nil {
		t.Fatal(err)
	}
	rolledBack := created.Cancellation.RequestedAt.Add(-time.Nanosecond)
	crashed, _, err := st.CrashSpecialistAttempt(ctx,
		domain.AgentAttemptFailureRequest{
			Ref: ref, Failure: domain.AgentAttemptFailure{
				Code: "runtime_failure", Reason: "worker stopped before model terminal",
			}, NotificationMessageID: idgen.New("agentmsg"), FailedAt: rolledBack,
		}, "specialist-terminal-crash-0001")
	if err != nil || crashed.Status != domain.AgentAttemptCrashed {
		t.Fatalf("Specialist attempt did not crash: attempt=%#v err=%v", crashed, err)
	}
	resolved, err := st.GetSpecialistModelCancellation(ctx, created.Cancellation.ID)
	if err != nil || resolved.Status != domain.ModelCancellationResolved ||
		resolved.Resolution != "attempt_terminated" || resolved.ResolvedAt == nil ||
		resolved.ResolvedAt.Before(resolved.RequestedAt) {
		t.Fatalf("terminated Specialist cancellation stayed live: %#v err=%v", resolved, err)
	}
}

func TestSpecialistModelCancellationConcurrentStoresConverge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specialist-cancellation-concurrent.db")
	primary, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primary.Close() })
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, primary,
		"concurrent Specialist cancellation", 2, 64)
	attempt, _, err := primary.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")),
		"specialist-concurrent-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	ref := attemptRef(attempt)
	modelAttempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "mock", Model: "mock-code",
	}
	if inserted, err := primary.RecordSpecialistModelStarted(ctx, ref,
		modelAttempt); err != nil || !inserted {
		t.Fatalf("Specialist model did not start: inserted=%t err=%v", inserted, err)
	}
	const workers = 8
	stores := make([]*SQLiteStore, workers)
	for index := range stores {
		stores[index], err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = stores[index].Close() })
	}
	request := domain.RequestSpecialistModelCancellation{
		RunID: fixture.Run.ID, AgentID: fixture.Child.ID, AttemptID: attempt.ID,
		ModelAttempt: 1, IdempotencyKey: "specialist-concurrent-cancel-0001",
		RequestedBy: "concurrency_test",
	}
	type outcome struct {
		result domain.SpecialistModelCancellationResult
		err    error
	}
	outcomes := make(chan outcome, workers)
	var start sync.WaitGroup
	start.Add(1)
	for _, current := range stores {
		current := current
		go func() {
			start.Wait()
			result, err := current.RequestSpecialistModelCancellation(ctx, request)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	start.Done()
	ids := make(map[string]struct{})
	created := 0
	for range workers {
		current := <-outcomes
		if current.err != nil {
			t.Fatal(current.err)
		}
		ids[current.result.Cancellation.ID] = struct{}{}
		if !current.result.Replayed {
			created++
		}
	}
	if len(ids) != 1 || created != 1 {
		t.Fatalf("concurrent Specialist cancellation did not converge: ids=%#v created=%d",
			ids, created)
	}
	var rows int
	if err := primary.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM specialist_model_cancellations`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("concurrent cancellation row count=%d err=%v", rows, err)
	}
	eventLog, err := primary.ListRunEvents(ctx, fixture.Run.ID)
	if err != nil || countRunEventType(eventLog, events.ModelCancelRequestedEvent) != 1 {
		t.Fatalf("concurrent cancellation event count drifted: events=%#v err=%v",
			eventLog, err)
	}
}
