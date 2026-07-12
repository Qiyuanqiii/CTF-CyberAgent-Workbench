package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
)

func TestRootInboxContextPreparesOnlyBackedChildProtocolsAndSupersedesOnFailure(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st, "root inbox protocols", 1, 32)
	dependency := sendRootDependencyTestMessage(t, ctx, st, fixture.Run.ID,
		fixture.Child.ID, fixture.Root.ID, "dependency-context-0001")

	firstAttempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")), "root-inbox-child-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RecordSpecialistAttemptUsage(ctx, attemptRef(firstAttempt),
		domain.AgentAttemptUsage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
		"root-inbox-child-usage-0001"); err != nil {
		t.Fatal(err)
	}
	completion := newCompletionTestValue(fixture.Run.ID, fixture.Root.ID, fixture.Child.ID,
		firstAttempt.ID, domain.CompletionSucceeded, nil, nil, "specialist completed")
	completed, _, err := st.FinishSpecialist(ctx, completion, "root-inbox-child-finish-0001")
	if err != nil {
		t.Fatal(err)
	}
	secondChild, replayed, err := st.AdmitSpecialist(ctx, domain.SpecialistAdmission{
		AgentID: idgen.New("agent"), SessionID: idgen.New("sess"), RunID: fixture.Run.ID,
		ParentAgentID: fixture.Root.ID, Title: "failure specialist", Skills: []string{"model.chat"},
		TurnLimit: 1, TokenLimit: 32, MaxChildren: 2, CreatedAt: time.Now().UTC(),
	}, "root-inbox-second-admission")
	if err != nil || replayed {
		t.Fatalf("second Specialist admission failed: child=%#v replayed=%t err=%v",
			secondChild, replayed, err)
	}
	secondFixture := fixture
	secondFixture.Child = secondChild
	secondAttempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(secondFixture, idgen.New("attempt")), "root-inbox-child-start-0002")
	if err != nil {
		t.Fatal(err)
	}
	crashed, _, err := st.CrashSpecialistAttempt(ctx, domain.AgentAttemptFailureRequest{
		Ref: attemptRef(secondAttempt), Failure: domain.AgentAttemptFailure{
			Code: "provider_error", Reason: "provider stopped",
		}, NotificationMessageID: idgen.New("agentmsg"), FailedAt: time.Now().UTC(),
	}, "root-inbox-child-crash-0001")
	if err != nil {
		t.Fatal(err)
	}

	turn, err := st.BeginSupervisorTurn(ctx, fixture.Lease, "review child inbox")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint)
	if err != nil || batch.Recovered || len(batch.Messages) != 3 {
		t.Fatalf("root inbox batch is invalid: batch=%#v err=%v", batch, err)
	}
	if batch.Messages[0].ID != dependency.ID || batch.Messages[1].ID != completed.MessageID ||
		batch.Messages[2].ID != crashed.NotificationMessageID {
		t.Fatalf("root inbox batch order is invalid: %#v", batch.Messages)
	}
	if _, err := domain.DecodeAgentDependencyPayload(batch.Messages[0].PayloadJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.DecodeAgentCompletionInboxPayload(batch.Messages[1].PayloadJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.DecodeAgentAttemptFailurePayload(batch.Messages[2].PayloadJSON); err != nil {
		t.Fatal(err)
	}
	recovered, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint)
	if err != nil || !recovered.Recovered || len(recovered.Messages) != len(batch.Messages) {
		t.Fatalf("prepared batch did not replay: batch=%#v err=%v", recovered, err)
	}
	for index := range batch.Messages {
		if recovered.Messages[index].ID != batch.Messages[index].ID {
			t.Fatalf("prepared batch changed at index %d", index)
		}
	}
	if _, err := st.ConsumeAgentMessages(ctx, fixture.Root.ID, 4); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("manual consume stole a running root batch: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := st.RestoreAgentGraph(ctx, fixture.Run.ID); err != nil {
		t.Fatalf("prepared root inbox graph did not restore: %v", err)
	}
	if _, err := st.FailSupervisorTurn(ctx, turn.Checkpoint, "model unavailable", 0); err != nil {
		t.Fatal(err)
	}
	pending, err := st.ListAgentMessages(ctx, fixture.Root.ID, true, 10)
	if err != nil || len(pending) != 3 {
		t.Fatalf("failed turn consumed inbox messages: messages=%#v err=%v", pending, err)
	}
	deliveries, err := listRootInboxDeliveriesDB(ctx, st, fixture.Run.ID,
		domain.RootInboxDeliverySuperseded)
	if err != nil || len(deliveries) != 3 {
		t.Fatalf("delivery history is invalid: deliveries=%#v err=%v", deliveries, err)
	}
	for _, delivery := range deliveries {
		if delivery.Status != domain.RootInboxDeliverySuperseded {
			t.Fatalf("failed turn left a non-superseded delivery: %#v", delivery)
		}
	}
	if _, err := st.RestoreAgentGraph(ctx, fixture.Run.ID); err != nil {
		t.Fatalf("superseded root inbox graph did not restore: %v", err)
	}
}

func TestRootInboxContextCommitsExactlyOnceWithSupervisorTurn(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st, "root inbox commit", 1, 32)
	message := sendRootDependencyTestMessage(t, ctx, st, fixture.Run.ID,
		fixture.Child.ID, fixture.Root.ID, "dependency-commit-0001")
	turn, err := st.BeginSupervisorTurn(ctx, fixture.Lease, "consume child update")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint); err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test", Model: "model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt); err != nil || !inserted {
		t.Fatalf("model start failed: inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeSuccess
	response := llm.ChatResponse{
		Text: "child update processed", Provider: "test", Model: "model",
		Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
	}
	checkpoint, err := st.RecordSupervisorModelCompleted(ctx, turn.Checkpoint, attempt, response)
	if err != nil {
		t.Fatal(err)
	}
	action := domain.RootAction{
		Version: domain.RootLifecycleVersion, Kind: domain.RootActionContinue,
		Message: "child update processed",
	}
	if _, err := st.db.ExecContext(ctx, `CREATE TRIGGER fail_root_inbox_commit_event
		BEFORE INSERT ON run_events WHEN NEW.type = 'agent.inbox_context_committed'
		BEGIN SELECT RAISE(ABORT, 'forced root inbox commit event failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.CompleteSupervisorTurn(ctx, checkpoint, response, action,
		policy.Decision{Allowed: true}, 0); err == nil {
		t.Fatal("forced root inbox commit event failure did not abort turn completion")
	}
	stored, err := scanAgentMessage(st.db.QueryRowContext(ctx, agentMessageSelect+` WHERE id = ?`, message.ID))
	if err != nil || stored.Status != domain.AgentMessagePending {
		t.Fatalf("failed turn completion consumed message: message=%#v err=%v", stored, err)
	}
	prepared, err := listRootInboxDeliveriesDB(ctx, st, fixture.Run.ID,
		domain.RootInboxDeliveryPrepared)
	if err != nil || len(prepared) != 1 {
		t.Fatalf("failed turn completion changed delivery: deliveries=%#v err=%v", prepared, err)
	}
	if messages, err := st.ListSessionMessages(ctx, fixture.Run.SessionID, true); err != nil || len(messages) != 0 {
		t.Fatalf("failed turn completion left Session messages: messages=%#v err=%v", messages, err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TRIGGER fail_root_inbox_commit_event`); err != nil {
		t.Fatal(err)
	}
	updatedRun, completedCheckpoint, messages, err := st.CompleteSupervisorTurn(ctx, checkpoint,
		response, action, policy.Decision{Allowed: true}, 0)
	if err != nil || updatedRun.Status != domain.RunRunning ||
		completedCheckpoint.Phase != domain.SupervisorIdle || messages.User.ID == 0 ||
		messages.Assistant.ID == 0 {
		t.Fatalf("valid turn completion failed: checkpoint=%#v messages=%#v err=%v",
			completedCheckpoint, messages, err)
	}
	stored, err = scanAgentMessage(st.db.QueryRowContext(ctx, agentMessageSelect+` WHERE id = ?`, message.ID))
	if err != nil || stored.Status != domain.AgentMessageConsumed || stored.ConsumedAt == nil {
		t.Fatalf("successful turn did not consume message: message=%#v err=%v", stored, err)
	}
	committed, err := listRootInboxDeliveriesDB(ctx, st, fixture.Run.ID,
		domain.RootInboxDeliveryCommitted)
	if err != nil || len(committed) != 1 || committed[0].MessageID != message.ID {
		t.Fatalf("successful turn did not commit delivery: deliveries=%#v err=%v", committed, err)
	}
	if _, _, replayMessages, err := st.CompleteSupervisorTurn(ctx, checkpoint, response, action,
		policy.Decision{Allowed: true}, 0); err != nil || replayMessages.User.ID != 0 {
		t.Fatalf("turn completion replay was not stable: messages=%#v err=%v", replayMessages, err)
	}
	timeline, err := st.ListRunEvents(ctx, fixture.Run.ID)
	if err != nil || countRunEventType(timeline, events.AgentInboxContextPreparedEvent) != 1 ||
		countRunEventType(timeline, events.AgentInboxContextCommittedEvent) != 1 ||
		countRunEventType(timeline, events.AgentMessageConsumedEvent) != 1 {
		t.Fatalf("root inbox audit events are not exactly once: events=%#v err=%v", timeline, err)
	}
	if _, err := st.RestoreAgentGraph(ctx, fixture.Run.ID); err != nil {
		t.Fatalf("committed root inbox graph did not restore: %v", err)
	}
}

func TestRootInboxContextSurvivesLeaseTakeoverWithoutChangingBatch(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixtureWithoutLease(t, ctx, st,
		"root inbox takeover", 1, 32)
	message := sendRootDependencyTestMessage(t, ctx, st, fixture.Run.ID,
		fixture.Child.ID, fixture.Root.ID, "dependency-takeover-0001")
	firstLease, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: fixture.Run.ID, OwnerID: "root-inbox-worker-a", TTL: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, firstLease.Lease, "recover inbox context")
	if err != nil {
		t.Fatal(err)
	}
	firstBatch, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint)
	if err != nil || len(firstBatch.Messages) != 1 {
		t.Fatalf("first batch failed: batch=%#v err=%v", firstBatch, err)
	}
	waitForLeaseExpiry(firstLease.Lease)
	secondLease, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: fixture.Run.ID, OwnerID: "root-inbox-worker-b", TTL: time.Minute,
	})
	if err != nil || !secondLease.TookOver {
		t.Fatalf("root inbox lease was not taken over: acquisition=%#v err=%v", secondLease, err)
	}
	recoveredTurn, err := st.BeginSupervisorTurn(ctx, secondLease.Lease, "recover inbox context")
	if err != nil || !recoveredTurn.Recovered ||
		recoveredTurn.Checkpoint.AttemptID != turn.Checkpoint.AttemptID {
		t.Fatalf("Supervisor turn did not recover: turn=%#v err=%v", recoveredTurn, err)
	}
	recoveredBatch, err := st.PrepareRootInboxContext(ctx, recoveredTurn.Checkpoint)
	if err != nil || !recoveredBatch.Recovered || len(recoveredBatch.Messages) != 1 ||
		recoveredBatch.Messages[0].ID != message.ID {
		t.Fatalf("root inbox batch changed after takeover: batch=%#v err=%v", recoveredBatch, err)
	}
	if _, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale checkpoint prepared inbox after takeover: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	if _, err := st.FailSupervisorTurn(ctx, recoveredTurn.Checkpoint, "stop after recovery", 0); err != nil {
		t.Fatal(err)
	}
	pending, err := st.ListAgentMessages(ctx, fixture.Root.ID, true, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("recovered failure lost pending message: messages=%#v err=%v", pending, err)
	}
}

func TestRootInboxContextConcurrentPrepareAndStoreRestartReuseOneBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, first,
		"root inbox concurrent prepare", 1, 32)
	message := sendRootDependencyTestMessage(t, ctx, first, fixture.Run.ID,
		fixture.Child.ID, fixture.Root.ID, "dependency-concurrent-0001")
	turn, err := first.BeginSupervisorTurn(ctx, fixture.Lease, "concurrent inbox prepare")
	if err != nil {
		_ = second.Close()
		_ = first.Close()
		t.Fatal(err)
	}
	type result struct {
		batch domain.RootInboxContextBatch
		err   error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	for _, st := range []*SQLiteStore{first, second} {
		st := st
		go func() {
			ready.Done()
			<-start
			batch, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint)
			results <- result{batch: batch, err: err}
		}()
	}
	ready.Wait()
	close(start)
	created, recovered := 0, 0
	for range 2 {
		item := <-results
		if item.err != nil || len(item.batch.Messages) != 1 ||
			item.batch.Messages[0].ID != message.ID {
			_ = second.Close()
			_ = first.Close()
			t.Fatalf("concurrent prepare failed: batch=%#v err=%v", item.batch, item.err)
		}
		if item.batch.Recovered {
			recovered++
		} else {
			created++
		}
	}
	if created != 1 || recovered != 1 {
		_ = second.Close()
		_ = first.Close()
		t.Fatalf("expected one prepared batch and one replay, got created=%d recovered=%d",
			created, recovered)
	}
	if err := second.Close(); err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restarted, err := reopened.PrepareRootInboxContext(ctx, turn.Checkpoint)
	if err != nil || !restarted.Recovered || len(restarted.Messages) != 1 ||
		restarted.Messages[0].ID != message.ID {
		t.Fatalf("restart did not recover prepared batch: batch=%#v err=%v", restarted, err)
	}
	timeline, err := reopened.ListRunEvents(ctx, fixture.Run.ID)
	if err != nil || countRunEventType(timeline, events.AgentInboxContextPreparedEvent) != 1 {
		t.Fatalf("concurrent prepare duplicated audit events: events=%#v err=%v", timeline, err)
	}
	prepared, err := listRootInboxDeliveriesDB(ctx, reopened, fixture.Run.ID,
		domain.RootInboxDeliveryPrepared)
	if err != nil || len(prepared) != 1 {
		t.Fatalf("concurrent prepare duplicated deliveries: deliveries=%#v err=%v", prepared, err)
	}
}

func TestRootInboxContextPrepareEventFailureRollsBackBindingAndSnapshot(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st,
		"root inbox prepare rollback", 1, 32)
	message := sendRootDependencyTestMessage(t, ctx, st, fixture.Run.ID,
		fixture.Child.ID, fixture.Root.ID, "dependency-rollback-0001")
	turn, err := st.BeginSupervisorTurn(ctx, fixture.Lease, "prepare inbox atomically")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `CREATE TRIGGER fail_root_inbox_prepared_event
		BEFORE INSERT ON run_events WHEN NEW.type = 'agent.inbox_context_prepared'
		BEGIN SELECT RAISE(ABORT, 'forced root inbox prepare event failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint); err == nil {
		t.Fatal("forced root inbox prepare event failure did not abort binding")
	}
	prepared, err := listRootInboxDeliveriesDB(ctx, st, fixture.Run.ID,
		domain.RootInboxDeliveryPrepared)
	if err != nil || len(prepared) != 0 {
		t.Fatalf("failed prepare left delivery rows: deliveries=%#v err=%v", prepared, err)
	}
	pending, err := st.ListAgentMessages(ctx, fixture.Root.ID, true, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("failed prepare changed inbox: messages=%#v err=%v", pending, err)
	}
	if _, err := st.RestoreAgentGraph(ctx, fixture.Run.ID); err != nil {
		t.Fatalf("failed prepare changed graph snapshot: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TRIGGER fail_root_inbox_prepared_event`); err != nil {
		t.Fatal(err)
	}
	batch, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint)
	if err != nil || batch.Recovered || len(batch.Messages) != 1 {
		t.Fatalf("rolled-back prepare could not retry: batch=%#v err=%v", batch, err)
	}
}

func TestRootInboxContextRejectsUnbackedResultAtStoreAndSQLiteBoundaries(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st,
		"root inbox unbacked result", 1, 32)
	message, replayed, err := st.SendAgentMessage(ctx, domain.AgentMessage{
		ID: idgen.New("agentmsg"), RunID: fixture.Run.ID, SenderAgentID: fixture.Child.ID,
		RecipientAgentID: fixture.Root.ID, Kind: domain.AgentMessageResult,
		Semantic: domain.AgentMessageSemanticMessage, PayloadJSON: `{"result":"unbacked"}`,
		Status: domain.AgentMessagePending, CreatedAt: time.Now().UTC(),
	}, "unbacked-result-message-0001")
	if err != nil || replayed {
		t.Fatalf("unbacked fixture message failed: message=%#v replayed=%t err=%v",
			message, replayed, err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, fixture.Lease, "reject unbacked result")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := st.db.ExecContext(ctx, `INSERT INTO root_inbox_deliveries
		(run_id, root_agent_id, supervisor_attempt_id, turn_number, message_id, ordinal,
		status, prepared_at, resolved_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?, NULL)`,
		fixture.Run.ID, fixture.Root.ID, turn.Checkpoint.AttemptID, turn.Checkpoint.NextTurn,
		message.ID, domain.RootInboxDeliveryPrepared, ts(now)); err == nil {
		t.Fatal("SQLite accepted a root inbox result without a durable CompletionReport")
	}
	batch, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint)
	if err != nil || len(batch.Messages) != 0 {
		t.Fatalf("Store prepared an unbacked result: batch=%#v err=%v", batch, err)
	}
	pending, err := st.ListAgentMessages(ctx, fixture.Root.ID, true, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("unbacked result did not remain pending: messages=%#v err=%v", pending, err)
	}
}

func TestSchemaV25PreservesV24CoordinatorState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v24.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixtureWithoutLease(t, ctx, st,
		"root inbox migration", 1, 32)
	message := sendRootDependencyTestMessage(t, ctx, st, fixture.Run.ID,
		fixture.Child.ID, fixture.Root.ID, "dependency-migration-0001")
	for _, statement := range removeSchemaV25ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("downgrade v25 fixture with %q: %v", statement, err)
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
	pending, err := st.ListAgentMessages(ctx, fixture.Root.ID, true, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("v24 inbox was not preserved: messages=%#v err=%v", pending, err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, fixture.Run.ID)
	turn, err := st.BeginSupervisorTurn(ctx, lease, "migrated inbox")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := st.PrepareRootInboxContext(ctx, turn.Checkpoint)
	if err != nil || len(batch.Messages) != 1 || batch.Messages[0].ID != message.ID {
		t.Fatalf("migrated inbox did not prepare: batch=%#v err=%v", batch, err)
	}
	var migrationCount int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 25`).
		Scan(&migrationCount); err != nil || migrationCount != 1 {
		t.Fatalf("schema v25 ledger is inconsistent: count=%d err=%v", migrationCount, err)
	}
}

func sendRootDependencyTestMessage(t testing.TB, ctx context.Context, st *SQLiteStore,
	runID string, senderID string, rootID string, operationKey string,
) domain.AgentMessage {
	t.Helper()
	payload, err := json.Marshal(domain.AgentDependencyPayload{
		DependencyID: "work-dependency", State: domain.AgentDependencySatisfied,
		Reason: "dependency resolved",
	})
	if err != nil {
		t.Fatal(err)
	}
	message, replayed, err := st.SendAgentMessage(ctx, domain.AgentMessage{
		ID: idgen.New("agentmsg"), RunID: runID, SenderAgentID: senderID,
		RecipientAgentID: rootID, Kind: domain.AgentMessageNotification,
		Semantic: domain.AgentMessageSemanticDependency, PayloadJSON: string(payload),
		Status: domain.AgentMessagePending, CreatedAt: time.Now().UTC(),
	}, operationKey)
	if err != nil || replayed {
		t.Fatalf("dependency message failed: message=%#v replayed=%t err=%v", message, replayed, err)
	}
	return message
}

func listRootInboxDeliveriesDB(ctx context.Context, st *SQLiteStore, runID string,
	status domain.RootInboxDeliveryStatus,
) ([]domain.RootInboxDelivery, error) {
	tx, err := st.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	items, err := listRootInboxDeliveriesTx(ctx, tx, rootInboxDeliverySelect+
		` WHERE run_id = ? AND status = ? ORDER BY ordinal`, runID, status)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}
