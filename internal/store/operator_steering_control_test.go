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
)

func TestOperatorSteeringCancellationIsIdempotentImmutableAndAudited(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st, "operator steering cancellation")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "cancel this queued instruction",
		OperationKey: "operator-steering-cancel-enqueue-0001", RequestedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := domain.CancelOperatorSteeringRequest{
		MessageID: queued.Message.ID, OperationKey: "operator-steering-cancel-operation-0001",
		RequestedBy: "cancel_operator_identity", Reason: "requirement was withdrawn",
	}
	first, err := st.CancelOperatorSteering(ctx, request)
	if err != nil || first.Replayed || first.Message.Status != domain.OperatorSteeringCancelled ||
		first.Cancellation.Kind != domain.OperatorSteeringCancellationOperator {
		t.Fatalf("operator cancellation failed: result=%#v err=%v", first, err)
	}
	replayed, err := st.CancelOperatorSteering(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Cancellation.ID != first.Cancellation.ID ||
		replayed.Message.ID != first.Message.ID {
		t.Fatalf("operator cancellation replay drifted: result=%#v err=%v", replayed, err)
	}
	conflicting := request
	conflicting.Reason = "different cancellation intent"
	if _, err := st.CancelOperatorSteering(ctx, conflicting); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed cancellation intent was not rejected: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	differentKey := request
	differentKey.OperationKey = "operator-steering-cancel-operation-0002"
	if _, err := st.CancelOperatorSteering(ctx, differentKey); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("second cancellation fact was accepted: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	loaded, err := getOperatorSteeringCancellationRow(st.db.QueryRowContext(ctx,
		operatorSteeringCancellationSelect+` WHERE id = ?`, first.Cancellation.ID))
	if err != nil || loaded.Reason != request.Reason ||
		loaded.ReasonSHA256 != domain.OperatorSteeringContentSHA256(request.Reason) {
		t.Fatalf("persisted cancellation is invalid: value=%#v err=%v", loaded, err)
	}
	var rawKeys int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM operator_steering_cancellation_operations WHERE operation_key_digest = ?`,
		request.OperationKey).Scan(&rawKeys); err != nil || rawKeys != 0 {
		t.Fatalf("raw cancellation operation key was persisted: count=%d err=%v", rawKeys, err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE operator_steering_cancellations
		SET reason = 'tampered' WHERE id = ?`, first.Cancellation.ID); err == nil {
		t.Fatal("SQLite allowed cancellation fact mutation")
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM operator_steering_cancellation_operations
		WHERE cancellation_id = ?`, first.Cancellation.ID); err == nil {
		t.Fatal("SQLite allowed cancellation operation deletion")
	}

	uncancelled, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "cannot bypass the cancellation ledger",
		OperationKey: "operator-steering-direct-cancel-0001", RequestedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE operator_steering_messages
		SET status = 'cancelled', cancelled_at = ? WHERE id = ?`, ts(time.Now().UTC()),
		uncancelled.Message.ID); err == nil {
		t.Fatal("SQLite allowed pending cancellation without a cancellation fact")
	}
	factOnlyAt := time.Now().UTC()
	factOnly := domain.OperatorSteeringCancellation{
		ID: "steer-cancel-fact-only", MessageID: uncancelled.Message.ID, RunID: run.ID,
		Kind: domain.OperatorSteeringCancellationOperator, RequestedBy: "direct_sql_operator",
		Reason:       "fact without operation ledger",
		ReasonSHA256: domain.OperatorSteeringContentSHA256("fact without operation ledger"),
		CreatedAt:    factOnlyAt,
	}
	if err := factOnly.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO operator_steering_cancellations
		(id, message_id, run_id, kind, requested_by, reason, reason_sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, factOnly.ID, factOnly.MessageID, factOnly.RunID,
		factOnly.Kind, factOnly.RequestedBy, factOnly.Reason, factOnly.ReasonSHA256,
		ts(factOnly.CreatedAt)); err != nil {
		t.Fatalf("insert cancellation fact for trigger test: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE operator_steering_messages
		SET status = 'cancelled', cancelled_at = ? WHERE id = ?`, ts(factOnlyAt),
		uncancelled.Message.ID); err == nil {
		t.Fatal("SQLite allowed operator cancellation without its operation ledger")
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(timeline, events.OperatorSteeringCancelledEvent) != 1 {
		t.Fatalf("operator cancellation event count drifted: events=%#v err=%v", timeline, err)
	}
	for _, event := range timeline {
		if event.Type == events.OperatorSteeringCancelledEvent &&
			(strings.Contains(event.PayloadJSON, request.OperationKey) ||
				strings.Contains(event.PayloadJSON, request.Reason) ||
				strings.Contains(event.PayloadJSON, request.RequestedBy)) {
			t.Fatalf("private cancellation data leaked into event: %s", event.PayloadJSON)
		}
	}
}

func TestOperatorSteeringPreparedCancellationIsRejectedAndTerminalFactClosesIt(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st, "prepared steering cancellation")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "already prepared guidance",
		OperationKey: "operator-steering-prepared-enqueue-0001", RequestedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	turn, err := st.BeginSupervisorSteeringTurn(ctx, lease)
	if err != nil || turn.Checkpoint.PendingInput != queued.Message.Content {
		t.Fatalf("queued input was not prepared: turn=%#v err=%v", turn, err)
	}
	if _, err := st.CancelOperatorSteering(ctx, domain.CancelOperatorSteeringRequest{
		MessageID: queued.Message.ID, OperationKey: "operator-steering-prepared-cancel-0001",
		RequestedBy: "operator", Reason: "too late to cancel",
	}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("prepared cancellation was not rejected: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	var cancellations int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_steering_cancellations
		WHERE message_id = ?`, queued.Message.ID).Scan(&cancellations); err != nil || cancellations != 0 {
		t.Fatalf("rejected prepared cancellation wrote a fact: count=%d err=%v",
			cancellations, err)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	cancelledRun, err := application.NewRunService(st).Cancel(ctx, run.ID)
	if err != nil || cancelledRun.Status != domain.RunCancelled {
		t.Fatalf("terminal Run cancellation failed: run=%#v err=%v", cancelledRun, err)
	}
	stored, err := st.GetOperatorSteering(ctx, queued.Message.ID)
	if err != nil || stored.Status != domain.OperatorSteeringCancelled {
		t.Fatalf("terminal cancellation did not close prepared steering: value=%#v err=%v",
			stored, err)
	}
	var kind domain.OperatorSteeringCancellationKind
	if err := st.db.QueryRowContext(ctx, `SELECT kind FROM operator_steering_cancellations
		WHERE message_id = ?`, queued.Message.ID).Scan(&kind); err != nil ||
		kind != domain.OperatorSteeringCancellationRunTerminal {
		t.Fatalf("terminal cancellation fact is missing: kind=%s err=%v", kind, err)
	}
}

func TestOperatorSteeringTerminalCancellationBoundsUntrustedFailureReason(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st,
		"bounded terminal steering cancellation")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "close this on terminal failure",
		OperationKey: "operator-steering-terminal-reason-enqueue-0001", RequestedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	reason := strings.Repeat("failure-原因 ", 600) + "\x00\x07tail"
	failed, err := application.NewRunService(st).Fail(ctx, run.ID, reason)
	if err != nil || failed.Status != domain.RunFailed {
		t.Fatalf("long failure reason blocked terminal transition: run=%#v err=%v", failed, err)
	}
	cancellation, err := getOperatorSteeringCancellationRow(st.db.QueryRowContext(ctx,
		operatorSteeringCancellationSelect+` WHERE message_id = ?`, queued.Message.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len([]byte(cancellation.Reason)) > domain.MaxOperatorSteeringReasonBytes ||
		strings.ContainsAny(cancellation.Reason, "\x00\x07") ||
		cancellation.ReasonSHA256 != domain.OperatorSteeringContentSHA256(cancellation.Reason) {
		t.Fatalf("terminal cancellation reason is not bounded and valid: %#v", cancellation)
	}
	if _, err := domain.NormalizeOperatorSteeringCancellationReason(cancellation.Reason); err != nil {
		t.Fatalf("terminal cancellation reason is not domain-valid: %v", err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range timeline {
		if event.Type == events.OperatorSteeringCancelledEvent &&
			(strings.Contains(event.PayloadJSON, "reason") ||
				strings.Contains(event.PayloadJSON, cancellation.Reason)) {
			t.Fatalf("terminal cancellation reason leaked into queue event: %s",
				event.PayloadJSON)
		}
	}
}

func TestOperatorSteeringConcurrentCancellationConvergesAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator-steering-cancel-concurrent.db")
	firstStore, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, firstStore,
		"concurrent operator steering cancellation")
	run, err := application.NewRunService(firstStore).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := firstStore.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "cancel concurrently once",
		OperationKey: "operator-steering-concurrent-cancel-enqueue-0001", RequestedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	request := domain.CancelOperatorSteeringRequest{
		MessageID: queued.Message.ID, OperationKey: "operator-steering-concurrent-cancel-0001",
		RequestedBy: "operator", Reason: "single concurrent decision",
	}
	start := make(chan struct{})
	results := make(chan domain.OperatorSteeringCancellationResult, 2)
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
			result, cancelErr := current.CancelOperatorSteering(ctx, request)
			if cancelErr != nil {
				errorsFound <- cancelErr
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
	for cancelErr := range errorsFound {
		t.Error(cancelErr)
	}
	values := make([]domain.OperatorSteeringCancellationResult, 0, 2)
	for result := range results {
		values = append(values, result)
	}
	if len(values) != 2 || values[0].Cancellation.ID == "" ||
		values[0].Cancellation.ID != values[1].Cancellation.ID ||
		values[0].Replayed == values[1].Replayed {
		t.Fatalf("concurrent cancellation did not converge: %#v", values)
	}
	var facts, operations int
	if err := firstStore.db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM operator_steering_cancellations WHERE message_id = ?`, queued.Message.ID).
		Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if err := firstStore.db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM operator_steering_cancellation_operations WHERE message_id = ?`, queued.Message.ID).
		Scan(&operations); err != nil || facts != 1 || operations != 1 {
		t.Fatalf("concurrent cancellation duplicated ledgers: facts=%d operations=%d err=%v",
			facts, operations, err)
	}
}

func TestSQLiteUpgradesV45OperatorSteeringToCancellationControls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator-steering-v45-upgrade.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st, "v45 operator steering upgrade")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "preserve v45 pending guidance",
		OperationKey: "operator-steering-v45-upgrade-enqueue-0001", RequestedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV46ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("simulate schema v45 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if version, err := st.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v45 did not upgrade to %d: version=%d err=%v",
			LatestSchemaVersion, version, err)
	}
	preserved, err := st.GetOperatorSteering(ctx, queued.Message.ID)
	if err != nil || preserved.Status != domain.OperatorSteeringPending ||
		preserved.Content != queued.Message.Content {
		t.Fatalf("v45 pending steering was not preserved: value=%#v err=%v", preserved, err)
	}
	result, err := st.CancelOperatorSteering(ctx, domain.CancelOperatorSteeringRequest{
		MessageID: preserved.ID, OperationKey: "operator-steering-v46-upgrade-cancel-0001",
		RequestedBy: "operator", Reason: "upgrade path verified",
	})
	if err != nil || result.Message.Status != domain.OperatorSteeringCancelled {
		t.Fatalf("upgraded steering could not use v46 controls: result=%#v err=%v", result, err)
	}
}
