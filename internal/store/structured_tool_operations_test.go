package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/toolgateway"
)

func TestStructuredWorkItemToolIsAtomicIdempotentAndAudited(t *testing.T) {
	st := openStructuredToolTestStore(t)
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "structured WorkItem")
	gateway := newStructuredToolTestGateway(st)
	payload := mustStructuredPayload(t, toolgateway.WorkItemCreateInput{
		Title: "Inspect parser", Description: "Review strict decoding", Priority: "high",
		AcceptanceCriteria: []string{"tests pass"},
	})
	call := toolgateway.ToolCall{
		Name: toolgateway.WorkItemCreateTool, Payload: payload, OperationKey: "stable-work-create",
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-structured", RequestedBy: "root",
	}
	first, err := gateway.Invoke(ctx, call)
	if err != nil || first.Result == nil || first.Result.Metadata["entity_kind"] != "work_item" ||
		first.Result.Metadata["replayed"] != "false" || first.Call.OperationKey != "" || first.Call.InvocationID == "" {
		t.Fatalf("unexpected structured WorkItem result: %#v err=%v", first, err)
	}
	itemID := first.Result.Metadata["entity_id"]
	item, err := st.GetWorkItem(ctx, itemID)
	if err != nil || item.RunID != run.ID || item.Status != domain.WorkItemPending || item.Title != "Inspect parser" {
		t.Fatalf("structured WorkItem was not persisted: %#v err=%v", item, err)
	}

	replayed, err := gateway.Invoke(ctx, call)
	if err != nil || replayed.Result == nil || replayed.Result.Metadata["entity_id"] != itemID ||
		replayed.Result.Metadata["replayed"] != "true" {
		t.Fatalf("structured WorkItem replay did not converge: %#v err=%v", replayed, err)
	}
	changed := call
	changed.Payload = mustStructuredPayload(t, toolgateway.WorkItemCreateInput{Title: "Different intent"})
	if _, err := gateway.Invoke(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed WorkItem intent did not conflict: code=%s err=%v", apperror.CodeOf(err), err)
	}

	items, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 1 || items[0].ID != itemID {
		t.Fatalf("idempotent WorkItem operation created duplicates: %#v err=%v", items, err)
	}
	usage, err := st.GetToolCallUsage(ctx, run.ID)
	if err != nil || usage.Consumed != 3 {
		t.Fatalf("structured tool invocation attempts were not budgeted: %#v err=%v", usage, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for eventType, want := range map[string]int{
		events.ToolBudgetChargedEvent: 3, events.PolicyDecisionEvent: 1,
		events.WorkItemCreatedEvent: 1, events.ToolCompletedEvent: 1,
	} {
		if got := countRunEventType(timeline, eventType); got != want {
			t.Fatalf("event %s count=%d want=%d timeline=%#v", eventType, got, want, timeline)
		}
	}
	operation, err := getStructuredOperationByKey(ctx, st.db,
		runmutation.OperationKeyDigest(string(toolgateway.WorkItemCreateTool), run.ID, call.OperationKey))
	if err != nil || operation.TargetID != itemID || operation.InvocationID != first.Call.InvocationID ||
		operation.TargetKind != runmutation.TargetWorkItem {
		t.Fatalf("structured WorkItem operation ledger is incomplete: %#v err=%v", operation, err)
	}
}

func TestStructuredNoteToolRedactsSecretsAndKeepsContentOutOfToolEvents(t *testing.T) {
	st := openStructuredToolTestStore(t)
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "structured Note")
	gateway := newStructuredToolTestGateway(st)
	token := "s" + "k-" + strings.Repeat("m", 28)
	operationKey := "note-operation-key-not-secret-but-not-persisted"
	outcome, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.NoteCreateTool,
		Payload: mustStructuredPayload(t, toolgateway.NoteCreateInput{
			Title: "Provider " + token, Content: "Observed token=" + token,
			Category: "observation", Visibility: "root", Tags: []string{"Provider"}, Pinned: true,
		}),
		OperationKey: operationKey, RunID: run.ID, SessionID: run.SessionID,
		WorkspaceID: "ws-structured", RequestedBy: "root",
	})
	if err != nil || outcome.Result == nil || strings.Contains(string(outcome.Call.Payload), token) ||
		!strings.Contains(string(outcome.Call.Payload), "[REDACTED:") || outcome.Call.OperationKey != "" {
		t.Fatalf("structured Note outcome was unsafe: %#v err=%v", outcome, err)
	}
	note, err := st.GetNote(ctx, outcome.Result.Metadata["entity_id"])
	if err != nil || strings.Contains(note.Title+note.Content, token) ||
		!strings.Contains(note.Content, "[REDACTED:") || note.Visibility != domain.NoteVisibilityRoot {
		t.Fatalf("structured Note persistence was unsafe: %#v err=%v", note, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range timeline {
		if strings.Contains(event.PayloadJSON, token) || strings.Contains(event.PayloadJSON, operationKey) ||
			(event.Type == events.ToolCompletedEvent && strings.Contains(event.PayloadJSON, note.Content)) {
			t.Fatalf("structured Note leaked into event %s: %s", event.Type, event.PayloadJSON)
		}
	}
	var rawKeyMatches int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM structured_tool_operations
		WHERE operation_key_digest = ? OR request_fingerprint = ?`, operationKey, operationKey).Scan(&rawKeyMatches); err != nil {
		t.Fatal(err)
	}
	if rawKeyMatches != 0 {
		t.Fatal("raw structured operation key was persisted")
	}
}

func TestStructuredNoteToolEventFailureRollsBackAndRetryRecovers(t *testing.T) {
	st := openStructuredToolTestStore(t)
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "structured rollback")
	gateway := newStructuredToolTestGateway(st)
	call := toolgateway.ToolCall{
		Name:         toolgateway.NoteCreateTool,
		Payload:      mustStructuredPayload(t, toolgateway.NoteCreateInput{Title: "Rollback", Content: "retry safely"}),
		OperationKey: "structured-rollback", RunID: run.ID, SessionID: run.SessionID,
		WorkspaceID: "ws-structured", RequestedBy: "root",
	}
	if _, err := st.db.ExecContext(ctx, `CREATE TRIGGER fail_structured_tool_completion
		BEFORE INSERT ON run_events WHEN NEW.type = 'tool.completed'
		BEGIN SELECT RAISE(ABORT, 'injected structured tool event failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Invoke(ctx, call); err == nil {
		t.Fatal("expected injected structured tool event failure")
	}
	notes, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID})
	if err != nil || len(notes) != 0 {
		t.Fatalf("failed structured mutation left a Note: %#v err=%v", notes, err)
	}
	var operations int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM structured_tool_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("failed structured mutation left %d operation rows", operations)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TRIGGER fail_structured_tool_completion`); err != nil {
		t.Fatal(err)
	}
	recovered, err := gateway.Invoke(ctx, call)
	if err != nil || recovered.Result == nil || recovered.Result.Metadata["replayed"] != "false" {
		t.Fatalf("structured mutation retry did not recover: %#v err=%v", recovered, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countRunEventType(timeline, events.ToolBudgetChargedEvent) != 2 ||
		countRunEventType(timeline, events.PolicyDecisionEvent) != 1 ||
		countRunEventType(timeline, events.NoteCreatedEvent) != 1 ||
		countRunEventType(timeline, events.ToolCompletedEvent) != 1 {
		t.Fatalf("structured retry event stream is inconsistent: %#v", timeline)
	}
}

func TestStructuredWorkItemConcurrentReplayConverges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "structured-concurrency.db")
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
	_, run := createStructuredToolTestRun(t, ctx, st, "structured concurrency")
	gateways := []*toolgateway.Gateway{newStructuredToolTestGateway(st), newStructuredToolTestGateway(other)}
	call := toolgateway.ToolCall{
		Name:         toolgateway.WorkItemCreateTool,
		Payload:      mustStructuredPayload(t, toolgateway.WorkItemCreateInput{Title: "One item"}),
		OperationKey: "concurrent-work-item", RunID: run.ID, SessionID: run.SessionID,
		WorkspaceID: "ws-structured", RequestedBy: "root",
	}
	const workers = 8
	type result struct {
		outcome toolgateway.Outcome
		err     error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for index := range workers {
		wg.Add(1)
		go func(gateway *toolgateway.Gateway) {
			defer wg.Done()
			outcome, err := gateway.Invoke(ctx, call)
			results <- result{outcome: outcome, err: err}
		}(gateways[index%len(gateways)])
	}
	wg.Wait()
	close(results)
	ids := map[string]bool{}
	created := 0
	for result := range results {
		if result.err != nil || result.outcome.Result == nil {
			t.Fatalf("concurrent structured invocation failed: %#v err=%v", result.outcome, result.err)
		}
		ids[result.outcome.Result.Metadata["entity_id"]] = true
		if result.outcome.Result.Metadata["replayed"] == "false" {
			created++
		}
	}
	if len(ids) != 1 || created != 1 {
		t.Fatalf("concurrent structured replay did not converge: ids=%#v created=%d", ids, created)
	}
	items, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("concurrent structured replay persisted duplicates: %#v err=%v", items, err)
	}
}

func TestSQLiteUpgradesSchemaV14ToStructuredToolsWithoutLosingNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v14.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run := createStructuredToolTestRun(t, ctx, st, "preserve v14")
	preserved, err := application.NewNoteService(st).Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "Preserved", Content: "v14 note",
	})
	if err != nil {
		t.Fatal(err)
	}
	removeSchemaV16ForTest(t, st, ctx)
	if _, err := st.db.ExecContext(ctx, `DROP TABLE structured_tool_operations`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 15`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if note, err := st.GetNote(ctx, preserved.ID); err != nil || note.Content != "v14 note" {
		t.Fatalf("v14 Note was not preserved: %#v err=%v", note, err)
	}
	outcome, err := newStructuredToolTestGateway(st).Invoke(ctx, toolgateway.ToolCall{
		Name:         toolgateway.NoteCreateTool,
		Payload:      mustStructuredPayload(t, toolgateway.NoteCreateInput{Title: "Upgraded", Content: "usable"}),
		OperationKey: "v15-upgrade", RunID: run.ID, SessionID: run.SessionID,
		WorkspaceID: "ws-structured", RequestedBy: "root",
	})
	if err != nil || outcome.Result == nil || outcome.Result.Metadata["entity_id"] == "" {
		t.Fatalf("v15 structured tool is unusable after migration: %#v err=%v", outcome, err)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("v14 database did not upgrade to latest: version=%d err=%v", version, err)
	}
}

func openStructuredToolTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "structured-tools.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func createStructuredToolTestRun(t *testing.T, ctx context.Context, st *SQLiteStore, goal string) (domain.Mission, domain.Run) {
	t.Helper()
	mission, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: goal, Profile: "code", WorkspaceID: "ws-structured",
		Budget: domain.Budget{MaxTurns: 5, MaxToolCalls: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mission, run
}

func newStructuredToolTestGateway(st *SQLiteStore) *toolgateway.Gateway {
	return toolgateway.New(st, policy.NewDefaultChecker()).
		WithStructuredMemoryExecutor(application.NewStructuredMemoryToolExecutor(st))
}

func mustStructuredPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
