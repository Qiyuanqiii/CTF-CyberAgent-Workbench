package store

import (
	"context"
	"encoding/json"
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
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/toolgateway"
)

func TestSupervisorToolBatchAndResultEventsAreAtomicAndReplayable(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "supervisor-tools.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "supervisor tool persistence")
	if _, err := application.NewRunService(st).Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquireTestRunExecutionLease(t, ctx, st, run.ID), "persist tools")
	if err != nil {
		t.Fatal(err)
	}
	secret := "s" + "k-" + strings.Repeat("q", 28)
	rawPayload := `{"title":"Provider","content":"token=` + secret + `"}`
	safePayload, err := toolgateway.NormalizeStructuredMemoryPayload(toolgateway.NoteCreateTool,
		json.RawMessage(rawPayload))
	if err != nil {
		t.Fatal(err)
	}
	operationKey := runmutation.SupervisorToolOperationKey(run.ID, turn.Checkpoint.NextTurn,
		"note_create", string(safePayload))
	callID, err := runmutation.SupervisorToolCallID(operationKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	started := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test", Model: "model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, started); err != nil || !inserted {
		t.Fatalf("model start failed: inserted=%t err=%v", inserted, err)
	}
	completed := started
	completed.Outcome = llm.OutcomeSuccess
	response := llm.ChatResponse{
		Provider: "test", Model: "model", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		ToolCalls: []llm.ToolCall{{
			ID: callID, Name: "note_create", Arguments: json.RawMessage(rawPayload),
		}},
	}
	checkpoint, err := st.RecordSupervisorModelCompleted(ctx, turn.Checkpoint, completed, response)
	if err != nil {
		t.Fatal(err)
	}
	rounds, err := st.ListSupervisorToolRounds(ctx, checkpoint)
	if err != nil || len(rounds) != 1 || len(rounds[0].Calls) != 1 ||
		rounds[0].Calls[0].Status != domain.SupervisorToolPending ||
		strings.Contains(rounds[0].Calls[0].PayloadJSON, secret) ||
		!strings.Contains(rounds[0].Calls[0].PayloadJSON, "[REDACTED:") {
		t.Fatalf("pending supervisor tool batch is unsafe: %#v err=%v", rounds, err)
	}
	if _, err := st.db.ExecContext(ctx, `CREATE TRIGGER fail_supervisor_tool_result_event
		BEFORE INSERT ON run_events WHEN NEW.type = 'supervisor.tool_result_recorded'
		BEGIN SELECT RAISE(ABORT, 'injected supervisor tool result event failure'); END;`); err != nil {
		t.Fatal(err)
	}
	result := domain.SupervisorToolResult{
		CallID: rounds[0].Calls[0].CallID, Status: domain.SupervisorToolCompleted,
		ResultJSON: `{"status":"completed"}`, CompletedAt: time.Now().UTC(),
	}
	if _, _, err := st.RecordSupervisorToolResult(ctx, checkpoint, result); err == nil {
		t.Fatal("expected supervisor tool result event failure")
	}
	rounds, err = st.ListSupervisorToolRounds(ctx, checkpoint)
	if err != nil || rounds[0].Calls[0].Status != domain.SupervisorToolPending || rounds[0].Complete() {
		t.Fatalf("failed result event did not roll back call state: %#v err=%v", rounds, err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TRIGGER fail_supervisor_tool_result_event`); err != nil {
		t.Fatal(err)
	}
	stored, replayed, err := st.RecordSupervisorToolResult(ctx, checkpoint, result)
	if err != nil || replayed || stored.Status != domain.SupervisorToolCompleted {
		t.Fatalf("supervisor tool result did not commit: %#v replayed=%t err=%v", stored, replayed, err)
	}
	if _, replayed, err := st.RecordSupervisorToolResult(ctx, checkpoint, result); err != nil || !replayed {
		t.Fatalf("supervisor tool result replay was not idempotent: replayed=%t err=%v", replayed, err)
	}
	rounds, err = st.ListSupervisorToolRounds(ctx, checkpoint)
	if err != nil || !rounds[0].Complete() {
		t.Fatalf("supervisor tool round was not completed: %#v err=%v", rounds, err)
	}
	history, err := st.ListRunSupervisorToolRoundsPage(ctx, run.ID, 0, 1)
	if err != nil || len(history) != 1 || len(history[0].Calls) != 1 ||
		history[0].AttemptID != rounds[0].AttemptID || history[0].Calls[0].CallID != result.CallID ||
		!history[0].Complete() {
		t.Fatalf("historical supervisor tool round page is inconsistent: %#v err=%v", history, err)
	}
	emptyHistory, err := st.ListRunSupervisorToolRoundsPage(ctx, run.ID, 1, 1)
	if err != nil || len(emptyHistory) != 0 {
		t.Fatalf("historical supervisor tool round offset is inconsistent: %#v err=%v", emptyHistory, err)
	}
	eventList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(eventList, events.SupervisorToolBatchEvent) != 1 ||
		countRunEventType(eventList, events.SupervisorToolResultEvent) != 1 ||
		countRunEventType(eventList, events.SupervisorToolCompleteEvent) != 1 {
		t.Fatalf("supervisor tool event stream is inconsistent: %#v err=%v", eventList, err)
	}
}

func TestConcurrentSupervisorToolResultReplayConvergesAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "supervisor-tool-result-concurrency.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "concurrent supervisor tool result")
	if _, err := application.NewRunService(st).Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquireTestRunExecutionLease(t, ctx, st, run.ID), "persist one concurrent result")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := toolgateway.NormalizeStructuredMemoryPayload(toolgateway.WorkItemCreateTool,
		json.RawMessage(`{"title":"Convergent result"}`))
	if err != nil {
		t.Fatal(err)
	}
	operationKey := runmutation.SupervisorToolOperationKey(run.ID, turn.Checkpoint.NextTurn,
		string(toolgateway.WorkItemCreateTool), string(payload))
	callID, err := runmutation.SupervisorToolCallID(operationKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test", Model: "model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt); err != nil || !inserted {
		t.Fatalf("model start failed: inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeSuccess
	checkpoint, err := st.RecordSupervisorModelCompleted(ctx, turn.Checkpoint, attempt, llm.ChatResponse{
		Provider: "test", Model: "model", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		ToolCalls: []llm.ToolCall{{ID: callID, Name: string(toolgateway.WorkItemCreateTool), Arguments: payload}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := domain.SupervisorToolResult{
		CallID: callID, Status: domain.SupervisorToolCompleted,
		ResultJSON: `{"status":"completed"}`, CompletedAt: time.Now().UTC(),
	}
	type recordResult struct {
		replayed bool
		err      error
	}
	results := make(chan recordResult, 2)
	stores := []*SQLiteStore{st, other}
	var ready sync.WaitGroup
	ready.Add(len(stores))
	start := make(chan struct{})
	for _, current := range stores {
		current := current
		go func() {
			ready.Done()
			<-start
			_, replayed, err := current.RecordSupervisorToolResult(ctx, checkpoint, result)
			results <- recordResult{replayed: replayed, err: err}
		}()
	}
	ready.Wait()
	close(start)
	replayed := 0
	for range stores {
		outcome := <-results
		if outcome.err != nil {
			t.Fatalf("concurrent result recording failed: %v", outcome.err)
		}
		if outcome.replayed {
			replayed++
		}
	}
	if replayed != 1 {
		t.Fatalf("expected exactly one durable result replay, got %d", replayed)
	}
	rounds, err := st.ListSupervisorToolRounds(ctx, checkpoint)
	if err != nil || len(rounds) != 1 || !rounds[0].Complete() {
		t.Fatalf("concurrent result did not complete one round: %#v err=%v", rounds, err)
	}
	eventList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(eventList, events.SupervisorToolResultEvent) != 1 ||
		countRunEventType(eventList, events.SupervisorToolCompleteEvent) != 1 {
		t.Fatalf("concurrent result duplicated events: %#v err=%v", eventList, err)
	}
}

func TestSupervisorToolBatchStoreRejectsUnknownPayloadFieldsBeforePersistence(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "supervisor-tool-strict-store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "strict supervisor tool Store boundary")
	if _, err := application.NewRunService(st).Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquireTestRunExecutionLease(t, ctx, st, run.ID), "reject unknown tool fields")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test", Model: "model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt); err != nil || !inserted {
		t.Fatalf("model start failed: inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeSuccess
	_, err = st.RecordSupervisorModelCompleted(ctx, turn.Checkpoint, attempt, llm.ChatResponse{
		Provider: "test", Model: "model", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		ToolCalls: []llm.ToolCall{{
			ID: "untrusted-provider-id", Name: string(toolgateway.NoteCreateTool),
			Arguments: json.RawMessage(`{"title":"Strict","content":"memory","unknown":true}`),
		}},
	})
	if apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("unknown field crossed the Store boundary: code=%s err=%v", apperror.CodeOf(err), err)
	}
	rounds, listErr := st.ListSupervisorToolRounds(ctx, turn.Checkpoint)
	if listErr != nil || len(rounds) != 0 {
		t.Fatalf("rejected payload created a tool round: %#v err=%v", rounds, listErr)
	}
	eventList, listErr := st.ListRunEvents(ctx, run.ID)
	if listErr != nil || countRunEventType(eventList, events.ModelCompletedEvent) != 0 ||
		countRunEventType(eventList, events.SupervisorToolBatchEvent) != 0 {
		t.Fatalf("rejected payload left terminal events: %#v err=%v", eventList, listErr)
	}
	checkpoint, found, getErr := st.GetSupervisorCheckpoint(ctx, run.ID)
	if getErr != nil || !found || checkpoint.TotalTokens != 0 || checkpoint.Phase != domain.SupervisorTurnStarted {
		t.Fatalf("rejected payload changed the checkpoint: %#v found=%t err=%v", checkpoint, found, getErr)
	}
}

func TestSQLiteUpgradesSchemaV15ToSupervisorToolLoopWithoutLosingStructuredNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v15.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "preserve v15")
	note, err := application.NewNoteService(st).Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "Preserved", Content: "v15 note",
	})
	if err != nil {
		t.Fatal(err)
	}
	removeSchemaV16ForTest(t, st, ctx)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loaded, err := st.GetNote(ctx, note.ID)
	if err != nil || loaded.Content != "v15 note" {
		t.Fatalf("v15 Note was not preserved: %#v err=%v", loaded, err)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("v15 database did not upgrade to latest: version=%d err=%v", version, err)
	}
}
