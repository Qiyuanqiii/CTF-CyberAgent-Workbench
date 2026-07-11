package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

func TestRunSupervisorExecutesAllowlistedStructuredToolAndContinuesModel(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "supervisor-tool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop", domain.Budget{MaxTurns: 3, MaxToolCalls: 5})
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-call-1", "work_item_create", `{"title":"Inspect parser","priority":"high"}`),
		textResponse(rootActionResponse(domain.RootActionContinue, "work board updated", "", "")),
	}}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != application.LifecycleTurnCompleted || result.ToolRounds != 1 || result.ToolCalls != 1 ||
		result.ModelAttempts != 2 || result.Text != "work board updated" || result.Checkpoint.TotalTokens != 8 {
		t.Fatalf("unexpected structured tool lifecycle result: %#v", result)
	}
	items, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 1 || items[0].Title != "Inspect parser" {
		t.Fatalf("structured WorkItem was not created: %#v err=%v", items, err)
	}
	requests := provider.Requests()
	if len(requests) != 2 || len(requests[0].Tools) != 2 || hasToolResults(requests[0]) ||
		!hasToolResult(requests[1], "work_item") {
		t.Fatalf("model did not receive the structured tool transcript: %#v", requests)
	}
	eventList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for eventType, want := range map[string]int{
		events.ModelCompletedEvent: 2, events.SupervisorToolBatchEvent: 1,
		events.SupervisorToolResultEvent: 1, events.SupervisorToolCompleteEvent: 1,
		events.WorkItemCreatedEvent: 1, events.ToolCompletedEvent: 1,
	} {
		if got := countEventType(eventList, eventType); got != want {
			t.Fatalf("event %s count=%d want=%d events=%#v", eventType, got, want, eventList)
		}
	}
	for _, event := range eventList {
		if strings.Contains(event.PayloadJSON, "provider-call-1") ||
			(strings.Contains(event.PayloadJSON, "Inspect parser") &&
				(event.Type == events.SupervisorToolBatchEvent || event.Type == events.SupervisorToolResultEvent ||
					event.Type == events.SupervisorToolCompleteEvent || strings.HasPrefix(event.Type, "model."))) {
			t.Fatalf("provider id or tool payload leaked into event %s: %s", event.Type, event.PayloadJSON)
		}
	}
}

func TestRunSupervisorRecoversPendingToolResultAcrossStoreRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "supervisor-tool-restart.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop", domain.Budget{MaxTurns: 3, MaxToolCalls: 5})
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-before-restart", "work_item_create", `{"title":"Durable item"}`),
		textResponse(rootActionResponse(domain.RootActionContinue, "recovered tools", "", "")),
	}}
	failing := &failOnceToolResultStore{SQLiteStore: st, fail: true}
	first, err := newToolLoopSupervisor(failing, provider).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeInternal || first.Checkpoint.Phase != domain.SupervisorTurnStarted {
		t.Fatalf("tool result failure did not leave a recoverable turn: %#v err=%v", first, err)
	}
	items, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("tool mutation did not commit before the injected result failure: %#v err=%v", items, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	resumed, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Recovered || resumed.ToolRounds != 1 || resumed.ToolCalls != 1 || resumed.ModelAttempts != 2 {
		t.Fatalf("pending tool batch was not recovered: %#v", resumed)
	}
	items, err = st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 1 || items[0].Title != "Durable item" {
		t.Fatalf("tool replay created duplicates: %#v err=%v", items, err)
	}
	usage, err := st.GetToolCallUsage(ctx, run.ID)
	if err != nil || usage.Consumed != 2 {
		t.Fatalf("initial call and recovery replay were not both budgeted: %#v err=%v", usage, err)
	}
	eventList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countEventType(eventList, events.WorkItemCreatedEvent) != 1 ||
		countEventType(eventList, events.ToolCompletedEvent) != 1 ||
		countEventType(eventList, events.SupervisorToolResultEvent) != 1 {
		t.Fatalf("recovered tool events are inconsistent: %#v err=%v", eventList, err)
	}
}

func TestRunSupervisorReturnsPolicyDeniedToolResultWithoutCreatingNote(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "supervisor-tool-denied.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop", domain.Budget{MaxTurns: 3, MaxToolCalls: 5})
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-denied", "note_create", `{"title":"Unsafe","content":"masscan 0.0.0.0/0"}`),
		textResponse(rootActionResponse(domain.RootActionContinue, "unsafe note rejected", "", "")),
	}}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if err != nil || result.ToolCalls != 1 || !hasErrorToolResult(provider.Requests()[1], "POLICY_DENIED") {
		t.Fatalf("Policy denial was not returned as a tool result: %#v err=%v requests=%#v",
			result, err, provider.Requests())
	}
	notes, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID})
	if err != nil || len(notes) != 0 {
		t.Fatalf("Policy-denied Note was persisted: %#v err=%v", notes, err)
	}
}

func TestRunSupervisorSemanticToolKeySurvivesFailedTurnAndChangedProviderID(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "supervisor-tool-attempt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop", domain.Budget{MaxTurns: 3, MaxToolCalls: 8})
	payload := `{"title":"One semantic item"}`
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-id-first", "work_item_create", payload),
		textResponse(`not lifecycle JSON`),
		textResponse(`still not lifecycle JSON`),
		toolResponse("provider-id-second", "work_item_create", payload),
		textResponse(rootActionResponse(domain.RootActionContinue, "retry converged", "", "")),
	}}
	supervisor := newToolLoopSupervisor(st, provider)
	first, err := supervisor.Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || first.Checkpoint.Phase != domain.SupervisorTurnFailed {
		t.Fatalf("first turn attempt should fail protocol repair: %#v err=%v", first, err)
	}
	second, err := supervisor.Step(ctx, run.ID)
	if err != nil || second.ToolCalls != 1 || second.Text != "retry converged" {
		t.Fatalf("second turn attempt did not recover: %#v err=%v", second, err)
	}
	items, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("changed Provider call id duplicated semantic intent: %#v err=%v", items, err)
	}
	eventList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countEventType(eventList, events.WorkItemCreatedEvent) != 1 ||
		countEventType(eventList, events.ToolCompletedEvent) != 1 {
		t.Fatalf("semantic replay duplicated successful events: %#v err=%v", eventList, err)
	}
}

func TestRunSupervisorReturnsToolBudgetExhaustionAndResetsTransportAttemptsPerRound(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "supervisor-tool-budget.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop", domain.Budget{MaxTurns: 3, MaxToolCalls: 1})
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-note-1", "note_create", `{"title":"First","content":"saved"}`),
		toolResponse("provider-note-2", "note_create", `{"title":"Second","content":"over budget"}`),
		textResponse(rootActionResponse(domain.RootActionContinue, "budget observed", "", "")),
	}}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if err != nil || result.ToolRounds != 2 || result.ToolCalls != 2 || result.ModelAttempts != 3 ||
		!hasErrorToolResult(provider.Requests()[2], "RESOURCE_EXHAUSTED") {
		t.Fatalf("tool budget exhaustion was not returned to the model: %#v err=%v requests=%#v",
			result, err, provider.Requests())
	}
	notes, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID})
	if err != nil || len(notes) != 1 || notes[0].Title != "First" {
		t.Fatalf("over-budget Note was persisted: %#v err=%v", notes, err)
	}
	usage, err := st.GetToolCallUsage(ctx, run.ID)
	if err != nil || usage.Consumed != 1 || usage.ExhaustedAt == nil {
		t.Fatalf("tool budget ledger is inconsistent: %#v err=%v", usage, err)
	}
	eventList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 3; round++ {
		needle := `"tool_round":` + strconv.Itoa(round) + `,"transport_attempt":1`
		found := false
		for _, event := range eventList {
			if event.Type == events.ModelStartedEvent && strings.Contains(event.PayloadJSON, needle) {
				found = true
			}
		}
		if !found {
			t.Fatalf("tool round %d did not reset the transport attempt: %#v", round, eventList)
		}
	}
}

func TestRunSupervisorBoundsStructuredToolRounds(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "supervisor-tool-round-limit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop", domain.Budget{MaxTurns: 3, MaxToolCalls: 10})
	responses := make([]*llm.ChatResponse, 0, domain.MaxSupervisorToolRounds+2)
	for round := 1; round <= domain.MaxSupervisorToolRounds+2; round++ {
		responses = append(responses, toolResponse(fmt.Sprintf("provider-round-%d", round),
			"work_item_create", fmt.Sprintf(`{"title":"Round %d"}`, round)))
	}
	provider := &scriptedToolProvider{responses: responses}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		result.ToolRounds != domain.MaxSupervisorToolRounds ||
		result.ToolCalls != domain.MaxSupervisorToolRounds || result.ProtocolRepairs != 1 {
		t.Fatalf("structured tool round limit was not enforced: %#v err=%v", result, err)
	}
	items, listErr := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if listErr != nil || len(items) != domain.MaxSupervisorToolRounds {
		t.Fatalf("over-limit tool calls were executed: %#v err=%v", items, listErr)
	}
	eventList, listErr := st.ListRunEvents(ctx, run.ID)
	if listErr != nil || countEventType(eventList, events.SupervisorToolBatchEvent) != domain.MaxSupervisorToolRounds {
		t.Fatalf("tool batch limit event stream is inconsistent: %#v err=%v", eventList, listErr)
	}
}

func TestRunSupervisorReplaysRepeatedSemanticIntentAcrossToolRounds(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "supervisor-tool-repeat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop", domain.Budget{MaxTurns: 3, MaxToolCalls: 4})
	payload := `{"title":"Repeat safely"}`
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-repeat-1", "work_item_create", payload),
		toolResponse("provider-repeat-2", "work_item_create", payload),
		textResponse(rootActionResponse(domain.RootActionContinue, "repeat observed", "", "")),
	}}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	entityIDs := toolResultEntityIDs(provider.Requests()[2])
	if err != nil || result.ToolRounds != 2 || result.ToolCalls != 2 || len(entityIDs) != 2 ||
		entityIDs[0] == "" || entityIDs[0] != entityIDs[1] {
		t.Fatalf("repeated semantic tool intent did not replay: %#v err=%v requests=%#v",
			result, err, provider.Requests())
	}
	items, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("repeated semantic intent created duplicates: %#v err=%v", items, err)
	}
}

func toolResultEntityIDs(request llm.ChatRequest) []string {
	ids := make([]string, 0)
	for _, message := range request.Messages {
		for _, result := range message.ToolResults {
			var envelope struct {
				Metadata map[string]string `json:"metadata"`
			}
			if json.Unmarshal([]byte(result.Content), &envelope) == nil {
				ids = append(ids, envelope.Metadata["entity_id"])
			}
		}
	}
	return ids
}

type scriptedToolProvider struct {
	mu        sync.Mutex
	responses []*llm.ChatResponse
	requests  []llm.ChatRequest
}

func (*scriptedToolProvider) Name() string { return "tool-loop" }

func (*scriptedToolProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "tool-loop", Capabilities: []string{"chat", "tools"}}}, nil
}

func (p *scriptedToolProvider) Chat(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return nil, errors.New("scripted tool provider response queue is empty")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	copy := *response
	copy.ToolCalls = append([]llm.ToolCall(nil), response.ToolCalls...)
	return &copy, nil
}

func (p *scriptedToolProvider) StreamChat(ctx context.Context, request llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, request)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 2)
	if response.Text != "" {
		chunks <- llm.ChatChunk{Text: response.Text}
	}
	chunks <- llm.FinalChatChunk(response)
	close(chunks)
	return chunks, nil
}

func (*scriptedToolProvider) SupportsTools(string) bool    { return true }
func (*scriptedToolProvider) SupportsVision(string) bool   { return false }
func (*scriptedToolProvider) SupportsJSONMode(string) bool { return true }

func (p *scriptedToolProvider) Requests() []llm.ChatRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]llm.ChatRequest(nil), p.requests...)
}

type failOnceToolResultStore struct {
	*store.SQLiteStore
	mu   sync.Mutex
	fail bool
}

func (s *failOnceToolResultStore) RecordSupervisorToolResult(ctx context.Context,
	checkpoint domain.SupervisorCheckpoint, result domain.SupervisorToolResult,
) (domain.SupervisorToolCall, bool, error) {
	s.mu.Lock()
	if s.fail {
		s.fail = false
		s.mu.Unlock()
		return domain.SupervisorToolCall{}, false, apperror.New(apperror.CodeInternal,
			"injected supervisor tool result failure")
	}
	s.mu.Unlock()
	return s.SQLiteStore.RecordSupervisorToolResult(ctx, checkpoint, result)
}

func newToolLoopSupervisor(st application.RunSupervisorStore,
	provider *scriptedToolProvider,
) *application.RunSupervisor {
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	return application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
}

func toolResponse(id string, name string, payload string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Provider: "tool-loop", Model: "model", Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
		ToolCalls: []llm.ToolCall{{ID: id, Name: name, Arguments: json.RawMessage(payload)}},
	}
}

func textResponse(text string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Text: text, Provider: "tool-loop", Model: "model",
		Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
	}
}

func hasToolResults(request llm.ChatRequest) bool {
	for _, message := range request.Messages {
		if len(message.ToolResults) > 0 {
			return true
		}
	}
	return false
}

func hasToolResult(request llm.ChatRequest, value string) bool {
	for _, message := range request.Messages {
		for _, result := range message.ToolResults {
			if strings.Contains(result.Content, value) {
				return true
			}
		}
	}
	return false
}

func hasErrorToolResult(request llm.ChatRequest, code string) bool {
	for _, message := range request.Messages {
		for _, result := range message.ToolResults {
			if result.IsError && strings.Contains(result.Content, code) {
				return true
			}
		}
	}
	return false
}
