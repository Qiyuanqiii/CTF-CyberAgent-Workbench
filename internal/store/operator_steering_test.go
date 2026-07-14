package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
)

func TestOperatorSteeringConcurrentReplayConvergesAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator-steering-concurrent.db")
	firstStore, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, firstStore,
		"concurrent operator steering replay")
	run, err := application.NewRunService(firstStore).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	request := domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "one concurrent intent",
		OperationKey: "operator-steering-concurrent-replay-0001", RequestedBy: "operator",
	}
	start := make(chan struct{})
	results := make(chan domain.OperatorSteeringEnqueueResult, 2)
	errorsFound := make(chan error, 2)
	var ready sync.WaitGroup
	var done sync.WaitGroup
	stores := []*SQLiteStore{firstStore, secondStore}
	ready.Add(len(stores))
	done.Add(len(stores))
	for _, current := range stores {
		current := current
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			result, enqueueErr := current.EnqueueOperatorSteering(ctx, request)
			if enqueueErr != nil {
				errorsFound <- enqueueErr
				return
			}
			results <- result
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	close(results)
	close(errorsFound)
	for enqueueErr := range errorsFound {
		t.Error(enqueueErr)
	}
	var values []domain.OperatorSteeringEnqueueResult
	for result := range results {
		values = append(values, result)
	}
	if len(values) != 2 || values[0].Message.ID == "" ||
		values[0].Message.ID != values[1].Message.ID ||
		values[0].Replayed == values[1].Replayed {
		t.Fatalf("concurrent replay did not converge: %#v", values)
	}
	listed, err := firstStore.ListOperatorSteering(ctx, run.ID, 10)
	if err != nil || len(listed) != 1 || listed[0].Sequence != 1 {
		t.Fatalf("concurrent replay persisted duplicate messages: %#v err=%v", listed, err)
	}
	timeline, err := firstStore.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(timeline, events.OperatorSteeringQueuedEvent) != 1 {
		t.Fatalf("concurrent replay duplicated audit events: %#v err=%v", timeline, err)
	}
}

func TestOperatorSteeringQueueRetriesAndCommitsExactlyOnceAtTurnBoundaries(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st, "durable operator steering")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstRequest := domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "inspect the first result",
		OperationKey: "operator-steering-store-first-0001", RequestedBy: "operator",
	}
	first, err := st.EnqueueOperatorSteering(ctx, firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := st.EnqueueOperatorSteering(ctx, firstRequest)
	if err != nil || !replayed.Replayed || replayed.Message.ID != first.Message.ID {
		t.Fatalf("enqueue replay did not converge: result=%#v err=%v", replayed, err)
	}
	if _, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "different intent",
		OperationKey: firstRequest.OperationKey, RequestedBy: "operator",
	}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("operation-key intent conflict was not rejected: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	second, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "then verify the final state",
		OperationKey: "operator-steering-store-second-0001", RequestedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	var rawKeys int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_steering_operations
		WHERE operation_key_digest IN (?, ?)`, firstRequest.OperationKey,
		"operator-steering-store-second-0001").Scan(&rawKeys); err != nil || rawKeys != 0 {
		t.Fatalf("raw operation key was persisted: count=%d err=%v", rawKeys, err)
	}

	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	started, err := st.BeginSupervisorTurn(ctx, lease, "")
	if err != nil || started.Checkpoint.PendingInput != first.Message.Content {
		t.Fatalf("first queued input was not prepared: turn=%#v err=%v", started, err)
	}
	failed, err := st.FailSupervisorTurn(ctx, started.Checkpoint, "retry transport", time.Millisecond)
	if err != nil || failed.Phase != domain.SupervisorTurnFailed {
		t.Fatalf("prepared turn failure was not durable: checkpoint=%#v err=%v", failed, err)
	}
	retried, err := st.BeginSupervisorTurn(ctx, lease, "")
	if err != nil || retried.Checkpoint.PendingInput != first.Message.Content ||
		retried.Checkpoint.AttemptID == started.Checkpoint.AttemptID {
		t.Fatalf("failed steering was not rebound to one new attempt: turn=%#v err=%v", retried, err)
	}
	var superseded, prepared int
	if err := st.db.QueryRowContext(ctx, `SELECT
		SUM(CASE WHEN status = 'superseded' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status = 'prepared' THEN 1 ELSE 0 END)
		FROM operator_steering_deliveries WHERE message_id = ?`, first.Message.ID).
		Scan(&superseded, &prepared); err != nil || superseded != 1 || prepared != 1 {
		t.Fatalf("retry delivery ledger is inconsistent: superseded=%d prepared=%d err=%v",
			superseded, prepared, err)
	}

	checkpoint, response := recordOperatorSteeringModelSuccess(t, ctx, st, retried.Checkpoint,
		"I am finished")
	finish := domain.RootAction{Version: domain.RootLifecycleVersion,
		Kind: domain.RootActionFinish, Message: response.Text, Summary: "done"}
	updatedRun, completed, messages, err := st.CompleteSupervisorTurn(ctx, checkpoint,
		response, finish, policy.Decision{Allowed: true}, 0)
	if err != nil || updatedRun.Status != domain.RunRunning || completed.Phase != domain.SupervisorIdle ||
		messages.User.Content != first.Message.Content {
		t.Fatalf("finish was not deferred for queued steering: run=%#v checkpoint=%#v messages=%#v err=%v",
			updatedRun, completed, messages, err)
	}
	firstStored, err := st.GetOperatorSteering(ctx, first.Message.ID)
	if err != nil || firstStored.Status != domain.OperatorSteeringCommitted ||
		firstStored.SessionMessageID != messages.User.ID {
		t.Fatalf("first steering was not committed exactly once: value=%#v err=%v", firstStored, err)
	}
	secondStored, err := st.GetOperatorSteering(ctx, second.Message.ID)
	if err != nil || secondStored.Status != domain.OperatorSteeringPending {
		t.Fatalf("second steering changed before its boundary: value=%#v err=%v", secondStored, err)
	}

	next, err := st.BeginSupervisorTurn(ctx, lease, "")
	if err != nil || next.Checkpoint.PendingInput != second.Message.Content {
		t.Fatalf("second queued input was not prepared in order: turn=%#v err=%v", next, err)
	}
	checkpoint, response = recordOperatorSteeringModelSuccess(t, ctx, st, next.Checkpoint,
		"final result")
	finish.Message = response.Text
	updatedRun, completed, _, err = st.CompleteSupervisorTurn(ctx, checkpoint, response,
		finish, policy.Decision{Allowed: true}, 0)
	if err != nil || updatedRun.Status != domain.RunCompleted ||
		completed.Phase != domain.SupervisorRunCompleted {
		t.Fatalf("final queued turn did not complete the Run: run=%#v checkpoint=%#v err=%v",
			updatedRun, completed, err)
	}
	summary, err := st.GetOperatorSteeringQueueSummary(ctx, run.ID)
	if err != nil || summary.Pending != 0 || summary.Prepared != 0 || summary.Committed != 2 {
		t.Fatalf("unexpected terminal queue summary: %#v err=%v", summary, err)
	}
	history, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(history) != 4 || history[0].Content != first.Message.Content ||
		history[2].Content != second.Message.Content {
		t.Fatalf("queued Session history was not committed in order exactly once: %#v err=%v",
			history, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(timeline, events.OperatorSteeringQueuedEvent) != 2 ||
		countRunEventType(timeline, events.OperatorSteeringPreparedEvent) != 3 ||
		countRunEventType(timeline, events.OperatorSteeringSupersededEvent) != 1 ||
		countRunEventType(timeline, events.OperatorSteeringCommittedEvent) != 2 ||
		countRunEventType(timeline, events.OperatorSteeringActionDeferredEvent) != 1 {
		t.Fatalf("operator steering audit events are incomplete: events=%#v err=%v", timeline, err)
	}
	for _, event := range timeline {
		if !strings.HasPrefix(event.Type, "operator.steering_") {
			continue
		}
		if strings.Contains(event.PayloadJSON, first.Message.Content) ||
			strings.Contains(event.PayloadJSON, second.Message.Content) ||
			strings.Contains(event.PayloadJSON, firstRequest.OperationKey) ||
			strings.Contains(event.PayloadJSON, firstRequest.RequestedBy) {
			t.Fatalf("steering content, operation key, or operator identity leaked into event %s: %s",
				event.Type, event.PayloadJSON)
		}
	}
}

func TestOperatorSteeringIfBusyAndTerminalCancellation(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st, "busy operator steering")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "queue only while busy",
		OperationKey: "operator-steering-busy-store-0001", RequestedBy: "operator",
	}
	if result, queued, err := st.EnqueueOperatorSteeringIfBusy(ctx, request); err != nil || queued ||
		result.Message.ID != "" {
		t.Fatalf("idle Run unexpectedly queued steering: result=%#v queued=%t err=%v",
			result, queued, err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	result, queued, err := st.EnqueueOperatorSteeringIfBusy(ctx, request)
	if err != nil || !queued || result.Message.Status != domain.OperatorSteeringPending {
		t.Fatalf("busy Run did not queue steering: result=%#v queued=%t err=%v",
			result, queued, err)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	cancelled, err := application.NewRunService(st).Cancel(ctx, run.ID)
	if err != nil || cancelled.Status != domain.RunCancelled {
		t.Fatalf("Run cancellation failed: run=%#v err=%v", cancelled, err)
	}
	stored, err := st.GetOperatorSteering(ctx, result.Message.ID)
	if err != nil || stored.Status != domain.OperatorSteeringCancelled || stored.CancelledAt == nil {
		t.Fatalf("terminal Run did not cancel pending steering: value=%#v err=%v", stored, err)
	}
}

func recordOperatorSteeringModelSuccess(t *testing.T, ctx context.Context, st *SQLiteStore,
	checkpoint domain.SupervisorCheckpoint, text string,
) (domain.SupervisorCheckpoint, llm.ChatResponse) {
	t.Helper()
	attempt := llm.ModelAttempt{Number: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "test", Model: "model"}
	inserted, err := st.RecordSupervisorModelStarted(ctx, checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("model start failed: inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeSuccess
	response := llm.ChatResponse{Text: text, Provider: "test", Model: "model",
		Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}}
	updated, err := st.RecordSupervisorModelCompleted(ctx, checkpoint, attempt, response)
	if err != nil {
		t.Fatal(err)
	}
	return updated, response
}
