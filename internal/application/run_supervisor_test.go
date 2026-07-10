package application_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolrun"
)

func TestRunSupervisorCompletesOneTurnAndEnforcesBudget(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "review supervisor", Profile: "review", Budget: domain.Budget{MaxTurns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	supervisor := application.NewRunSupervisor(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	result, err := supervisor.Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != application.LifecycleTurnCompleted || result.Turn != 1 || result.Recovered || result.Checkpoint.NextTurn != 2 || result.Checkpoint.Phase != domain.SupervisorIdle {
		t.Fatalf("unexpected lifecycle result: %#v", result)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected supervisor messages: %#v", messages)
	}
	toolRuns, err := st.ListToolRuns(ctx, toolrun.ListFilter{SessionID: run.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if len(toolRuns) != 0 {
		t.Fatalf("supervisor unexpectedly created tool runs: %#v", toolRuns)
	}
	before, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(before, events.AgentTurnStartedEvent) != 1 || countEventType(before, events.AgentTurnCompletedEvent) != 1 {
		t.Fatalf("unexpected supervisor timeline: %#v", before)
	}
	if _, err := supervisor.Step(ctx, run.ID); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("unexpected budget error code=%s err=%v", apperror.CodeOf(err), err)
	}
	after, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("budget rejection appended events: before=%d after=%d", len(before), len(after))
	}
}

func TestRunSupervisorRecoversStartedTurnAcrossStoreRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "resume checkpoint", Profile: "code", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	started, err := st.BeginSupervisorTurn(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Recovered || started.Checkpoint.Phase != domain.SupervisorTurnStarted {
		t.Fatalf("unexpected started checkpoint: %#v", started)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	supervisor := application.NewRunSupervisor(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	result, err := supervisor.Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Recovered || result.Turn != 1 || result.AttemptID != started.Checkpoint.AttemptID || result.Checkpoint.NextTurn != 2 {
		t.Fatalf("turn was not resumed from its checkpoint: %#v", result)
	}
	before, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(before, events.AgentTurnStartedEvent) != 1 || countEventType(before, events.AgentTurnCompletedEvent) != 1 {
		t.Fatalf("recovery duplicated lifecycle events: %#v", before)
	}
	checkpoint, err := st.CompleteSupervisorTurn(ctx, started.Checkpoint, "ignored duplicate", llm.ChatResponse{Text: "ignored", Provider: "mock", Model: "mock-code"}, policy.Decision{Allowed: true, Reason: "allowed"})
	if err != nil || checkpoint.NextTurn != 2 {
		t.Fatalf("idempotent completion failed checkpoint=%#v err=%v", checkpoint, err)
	}
	after, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("idempotent completion duplicated events: before=%d after=%d", len(before), len(after))
	}
}

func TestRunSupervisorRejectsToolCallsWithoutExecution(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "do not execute tools", Profile: "code", ModelRoute: "tool-test/model", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: "tool-test", Model: "model"})
	router.RegisterProvider(toolCallProvider{})
	supervisor := application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
	result, err := supervisor.Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("unexpected tool-call rejection code=%s err=%v", apperror.CodeOf(err), err)
	}
	if result.Checkpoint.Phase != domain.SupervisorTurnFailed || !strings.Contains(result.Checkpoint.LastError, "tool calls are disabled") {
		t.Fatalf("tool-call failure was not checkpointed: %#v", result)
	}
	runs, err := st.ListToolRuns(ctx, toolrun.ListFilter{SessionID: run.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("tool call was persisted or executed: %#v", runs)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.AgentTurnFailedEvent) != 1 || countEventType(items, events.AgentTurnCompletedEvent) != 0 {
		t.Fatalf("unexpected failed-turn events: %#v", items)
	}
}

func TestRunSupervisorCancellationBeforeBeginDoesNotCheckpoint(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "cancel before turn", Profile: "learn", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	supervisor := application.NewRunSupervisor(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	if _, err := supervisor.Step(cancelled, run.ID); apperror.CodeOf(err) != apperror.CodeCancelled {
		t.Fatalf("unexpected cancellation code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, ok, err := st.GetSupervisorCheckpoint(ctx, run.ID); err != nil || ok {
		t.Fatalf("cancelled preflight created a checkpoint ok=%t err=%v", ok, err)
	}
}

func TestRunSupervisorRedactsImmediateAndPersistedResponse(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	token := "t" + "p-" + strings.Repeat("a", 40)
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "redact response", Profile: "review", ModelRoute: "secret-test/model", Budget: domain.Budget{MaxTurns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: "secret-test", Model: "model"})
	router.RegisterProvider(secretResponseProvider{text: "observed " + token})
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Text, token[:11]) || !strings.Contains(result.Text, "[REDACTED:") {
		t.Fatalf("immediate response was not redacted: %q", result.Text)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if strings.Contains(message.Content, token[:11]) {
			t.Fatalf("persisted response contained secret: %#v", messages)
		}
	}
}

func countEventType(items []events.Event, eventType string) int {
	count := 0
	for _, item := range items {
		if item.Type == eventType {
			count++
		}
	}
	return count
}

type toolCallProvider struct{}

func (toolCallProvider) Name() string { return "tool-test" }

func (toolCallProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "tool-test"}}, nil
}

func (toolCallProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Text: "call a tool", Provider: "tool-test", Model: "model",
		ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "shell", Arguments: json.RawMessage(`{"command":"echo hi"}`)}},
	}, nil
}

func (p toolCallProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 1)
	chunks <- llm.ChatChunk{Text: response.Text, ToolCalls: response.ToolCalls, Done: true}
	close(chunks)
	return chunks, nil
}

func (toolCallProvider) SupportsTools(string) bool    { return true }
func (toolCallProvider) SupportsVision(string) bool   { return false }
func (toolCallProvider) SupportsJSONMode(string) bool { return false }

type secretResponseProvider struct {
	text string
}

func (secretResponseProvider) Name() string { return "secret-test" }

func (secretResponseProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "secret-test"}}, nil
}

func (p secretResponseProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Text: p.text, Provider: p.Name(), Model: "model"}, nil
}

func (p secretResponseProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 1)
	chunks <- llm.ChatChunk{Text: response.Text, Done: true}
	close(chunks)
	return chunks, nil
}

func (secretResponseProvider) SupportsTools(string) bool    { return false }
func (secretResponseProvider) SupportsVision(string) bool   { return false }
func (secretResponseProvider) SupportsJSONMode(string) bool { return false }
