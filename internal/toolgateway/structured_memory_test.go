package toolgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/toolbudget"
)

type trackedStructuredStore struct {
	*memoryStore
	chargeMu sync.Mutex
	charges  int
}

func newTrackedStructuredStore() *trackedStructuredStore {
	return &trackedStructuredStore{memoryStore: newMemoryStore()}
}

func (s *trackedStructuredStore) ChargeToolCall(_ context.Context, request toolbudget.ChargeRequest) (toolbudget.Usage, error) {
	s.chargeMu.Lock()
	defer s.chargeMu.Unlock()
	s.charges++
	return toolbudget.Usage{
		RunID: request.RunID, Consumed: int64(s.charges), Limit: 100, Remaining: int64(100 - s.charges),
		Tracked: true, LastCharge: fmt.Sprintf("toolcall-%d", s.charges),
	}, nil
}

func (s *trackedStructuredStore) chargeCount() int {
	s.chargeMu.Lock()
	defer s.chargeMu.Unlock()
	return s.charges
}

type structuredExecutorStub struct {
	mu        sync.Mutex
	workCalls int
	noteCalls int
	lastScope StructuredMemoryContext
	lastNote  NoteCreateInput
	result    StructuredMutationResult
}

func (s *structuredExecutorStub) CreateWorkItem(_ context.Context, scope StructuredMemoryContext,
	_ WorkItemCreateInput,
) (StructuredMutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workCalls++
	s.lastScope = scope
	if s.result.EntityID == "" {
		return StructuredMutationResult{EntityID: "work-1", EntityKind: "work_item", Status: "pending", Version: 1}, nil
	}
	return s.result, nil
}

func (s *structuredExecutorStub) CreateNote(_ context.Context, scope StructuredMemoryContext,
	input NoteCreateInput,
) (StructuredMutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noteCalls++
	s.lastScope = scope
	s.lastNote = input
	if s.result.EntityID == "" {
		return StructuredMutationResult{EntityID: "note-1", EntityKind: "note", Status: "active", Version: 1}, nil
	}
	return s.result, nil
}

func TestStructuredMemoryToolDefinitionsAreValidAndCopied(t *testing.T) {
	definitions := StructuredMemoryToolDefinitions()
	if len(definitions) != 2 {
		t.Fatalf("unexpected structured tool definitions: %#v", definitions)
	}
	for _, definition := range definitions {
		if !definition.Name.Valid() || definition.Class != ClassRunMemory ||
			definition.Approval != ApprovalAutomatic || !json.Valid(definition.InputSchema) {
			t.Fatalf("invalid structured tool definition: %#v", definition)
		}
	}
	definitions[0].InputSchema[0] = '['
	fresh, found := StructuredMemoryToolDefinition(WorkItemCreateTool)
	if !found || !json.Valid(fresh.InputSchema) {
		t.Fatal("structured tool schema was mutated through a returned slice")
	}
	if _, _, err := decodeWorkItemCreateInput(json.RawMessage(
		`{"title":"x","dependencies":["work-20260711123456-abcdef012345"]}`)); err != nil {
		t.Fatalf("generated WorkItem dependency id was rejected: %v", err)
	}
}

func TestStructuredMemoryPayloadIsStrictAndValidatedBeforeBudgetCharge(t *testing.T) {
	store := newTrackedStructuredStore()
	gateway := New(store, policy.NewDefaultChecker()).WithStructuredMemoryExecutor(&structuredExecutorStub{})
	base := ToolCall{
		Name: WorkItemCreateTool, OperationKey: "strict-call", RunID: "run-1", SessionID: "sess-1",
		WorkspaceID: "ws-1", RequestedBy: "root",
	}
	for name, payload := range map[string]string{
		"unknown field":    `{"title":"x","unknown":true}`,
		"trailing JSON":    `{"title":"x"}{"title":"y"}`,
		"missing title":    `{}`,
		"invalid priority": `{"title":"x","priority":"urgent"}`,
	} {
		t.Run(name, func(t *testing.T) {
			call := base
			call.Payload = json.RawMessage(payload)
			if _, err := gateway.Invoke(t.Context(), call); err == nil {
				t.Fatal("expected structured payload rejection")
			}
		})
	}
	if store.chargeCount() != 0 {
		t.Fatalf("invalid structured payload consumed %d tool calls", store.chargeCount())
	}
}

func TestStructuredPayloadValidationDoesNotEchoSecret(t *testing.T) {
	store := newTrackedStructuredStore()
	gateway := New(store, policy.NewDefaultChecker()).WithStructuredMemoryExecutor(&structuredExecutorStub{})
	token := "s" + "k-" + strings.Repeat("d", 32)
	requests := []ToolCall{
		{
			Name:         WorkItemCreateTool,
			Payload:      json.RawMessage(fmt.Sprintf(`{"title":"x","dependencies":[%q]}`, "work-"+token)),
			OperationKey: "secret-dependency",
		},
		{
			Name:         WorkItemCreateTool,
			Payload:      json.RawMessage(fmt.Sprintf(`{"title":"x",%q:true}`, token)),
			OperationKey: "secret-field",
		},
		{
			Name:         NoteCreateTool,
			Payload:      json.RawMessage(fmt.Sprintf(`{"title":"x","content":"y","category":%q}`, token)),
			OperationKey: "secret-category",
		},
	}
	for _, call := range requests {
		call.RunID = "run-1"
		call.SessionID = "sess-1"
		call.WorkspaceID = "ws-1"
		call.RequestedBy = "root"
		_, err := gateway.Invoke(t.Context(), call)
		if err == nil || strings.Contains(err.Error(), token) {
			t.Fatalf("structured payload rejection leaked input for %s: %v", call.OperationKey, err)
		}
	}
	if store.chargeCount() != 0 {
		t.Fatalf("invalid secret-shaped payload consumed %d tool calls", store.chargeCount())
	}
}

func TestStructuredNoteToolUsesTrackedScopeAndRedactsOutcomePayload(t *testing.T) {
	store := newTrackedStructuredStore()
	executor := &structuredExecutorStub{}
	gateway := New(store, policy.NewDefaultChecker()).WithStructuredMemoryExecutor(executor)
	token := "s" + "k-" + strings.Repeat("v", 28)
	payload := json.RawMessage(fmt.Sprintf(`{"title":"Provider","content":"token=%s"}`, token))
	outcome, err := gateway.Invoke(t.Context(), ToolCall{
		Name: NoteCreateTool, Payload: payload, OperationKey: "provider-call-1",
		RunID: "run-1", SessionID: "sess-1", WorkspaceID: "ws-1", RequestedBy: "root",
	})
	if err != nil || outcome.Result == nil || outcome.Result.Metadata["entity_id"] != "note-1" ||
		outcome.Call.InvocationID != "toolcall-1" || outcome.Call.OperationKey != "" ||
		strings.Contains(string(outcome.Call.Payload), token) || !strings.Contains(string(outcome.Call.Payload), "[REDACTED:") {
		t.Fatalf("unexpected structured Note outcome: %#v err=%v", outcome, err)
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.noteCalls != 1 || executor.lastScope.OperationKey != "provider-call-1" ||
		executor.lastScope.InvocationID != "toolcall-1" || !strings.Contains(executor.lastNote.Content, token) {
		t.Fatalf("executor did not receive the authoritative structured call: %#v %#v", executor.lastScope, executor.lastNote)
	}
}

func TestStructuredMemoryPolicyDenialNeverInvokesExecutor(t *testing.T) {
	store := newTrackedStructuredStore()
	executor := &structuredExecutorStub{}
	gateway := New(store, policy.NewDefaultChecker()).WithStructuredMemoryExecutor(executor)
	for index, content := range []string{"masscan 0.0.0.0/0", "nmap approved target"} {
		outcome, err := gateway.Invoke(t.Context(), ToolCall{
			Name:         NoteCreateTool,
			Payload:      json.RawMessage(fmt.Sprintf(`{"title":"Blocked","content":%q}`, content)),
			OperationKey: fmt.Sprintf("denied-%d", index), RunID: "run-1", SessionID: "sess-1",
			WorkspaceID: "ws-1", RequestedBy: "root",
		})
		if err != nil || outcome.Decision.Allowed || outcome.Result == nil || outcome.Result.Status != StatusDenied {
			t.Fatalf("structured mutation was not denied: %#v err=%v", outcome, err)
		}
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.noteCalls != 0 || store.chargeCount() != 2 {
		t.Fatalf("denied structured mutation reached executor or skipped budget: calls=%d charges=%d",
			executor.noteCalls, store.chargeCount())
	}
}
