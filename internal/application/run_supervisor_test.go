package application_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	_, checkpoint, err := st.CompleteSupervisorTurn(ctx, started.Checkpoint, "ignored duplicate",
		llm.ChatResponse{Text: "ignored", Provider: "mock", Model: "mock-code"},
		domain.RootAction{Version: domain.RootLifecycleVersion, Kind: domain.RootActionContinue, Message: "ignored"},
		policy.Decision{Allowed: true, Reason: "allowed"}, 0)
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
	finalized, err := supervisor.Finalize(ctx, run.ID, application.LifecycleOutcomeFailed, "tool call rejected")
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Run.Status != domain.RunFailed || finalized.Checkpoint.Phase != domain.SupervisorRunFailed {
		t.Fatalf("failed turn did not finalize: %#v", finalized)
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
	if strings.Contains(result.Action.Message, token[:11]) {
		t.Fatalf("structured action contained secret: %#v", result.Action)
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
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if strings.Contains(item.PayloadJSON, token[:11]) {
			t.Fatalf("run event contained secret: %#v", item)
		}
	}
}

func TestRunSupervisorRejectsNilProviderResponse(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "nil provider response", Profile: "review", ModelRoute: "nil-test/model", Budget: domain.Budget{MaxTurns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: "nil-test", Model: "model"})
	router.RegisterProvider(nilResponseProvider{})
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || result.Checkpoint.Phase != domain.SupervisorTurnFailed {
		t.Fatalf("nil response was not checkpointed safely result=%#v code=%s err=%v", result, apperror.CodeOf(err), err)
	}
}

func TestRunSupervisorTracksAndEnforcesTokenBudget(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "token budget", Profile: "code", ModelRoute: "usage-test/model",
		Budget: domain.Budget{MaxTurns: 3, MaxTokens: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	provider := &fixedUsageProvider{}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	supervisor := application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
	result, err := supervisor.Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if provider.lastMaxTokens != 5 {
		t.Fatalf("remaining token budget was not forwarded: %d", provider.lastMaxTokens)
	}
	if !provider.lastJSONMode || provider.lastSchema != domain.RootLifecycleVersion {
		t.Fatalf("root lifecycle schema was not requested: json=%t schema=%q", provider.lastJSONMode, provider.lastSchema)
	}
	if result.Checkpoint.InputTokens != 2 || result.Checkpoint.OutputTokens != 3 || result.Checkpoint.TotalTokens != 5 || result.Checkpoint.ExecutionMillis < 0 {
		t.Fatalf("usage was not accumulated: %#v", result.Checkpoint)
	}
	if _, err := supervisor.Step(ctx, run.ID); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("unexpected token budget error code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestRunSupervisorEnforcesPersistedExecutionTimeout(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "timeout budget", Profile: "learn", Budget: domain.Budget{MaxTurns: 3, TimeoutSeconds: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := st.FailSupervisorTurn(ctx, turn.Checkpoint, "simulated timeout", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.ExecutionMillis != 1000 {
		t.Fatalf("elapsed execution time was not persisted: %#v", checkpoint)
	}
	supervisor := application.NewRunSupervisor(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	if _, err := supervisor.Step(ctx, run.ID); apperror.CodeOf(err) != apperror.CodeDeadlineExceeded {
		t.Fatalf("unexpected timeout code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestRunSupervisorAppliesRemainingExecutionDeadline(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "remaining deadline", Profile: "learn", ModelRoute: "blocking-test/model",
		Budget: domain.Budget{MaxTurns: 3, TimeoutSeconds: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.FailSupervisorTurn(ctx, turn.Checkpoint, "consume time", 999*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: "blocking-test", Model: "model"})
	router.RegisterProvider(blockingProvider{})
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeDeadlineExceeded {
		t.Fatalf("unexpected child deadline code=%s err=%v", apperror.CodeOf(err), err)
	}
	if result.Checkpoint.Phase != domain.SupervisorTurnFailed || result.Checkpoint.ExecutionMillis < 1000 {
		t.Fatalf("deadline failure did not accumulate elapsed time: %#v", result)
	}
}

func TestRunSupervisorFinalizationIsAtomicAndIdempotent(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "finalize supervisor", Profile: "review", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	supervisor := application.NewRunSupervisor(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	if _, err := supervisor.Step(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	finalized, err := supervisor.Finalize(ctx, run.ID, application.LifecycleOutcomeCompleted, "review complete")
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Run.Status != domain.RunCompleted || finalized.Run.FinishedAt == nil || finalized.Checkpoint.Phase != domain.SupervisorRunCompleted {
		t.Fatalf("unexpected finalization: %#v", finalized)
	}
	before, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(before, events.SupervisorRunCompletedEvent) != 1 {
		t.Fatalf("missing supervisor completion event: %#v", before)
	}
	if _, err := supervisor.Finalize(ctx, run.ID, application.LifecycleOutcomeCompleted, "repeat"); err != nil {
		t.Fatal(err)
	}
	after, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("repeat finalization appended events: before=%d after=%d", len(before), len(after))
	}
	if _, err := supervisor.Step(ctx, run.ID); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("terminal run accepted a step code=%s err=%v", apperror.CodeOf(err), err)
	}
	maxInt := int(^uint(0) >> 1)
	execution, err := supervisor.Execute(ctx, run.ID, maxInt)
	if err != nil {
		t.Fatalf("execute terminal run: %v", err)
	}
	if execution.StopReason != "run_terminal" || len(execution.Steps) != 0 {
		t.Fatalf("unexpected terminal execution result: %#v", execution)
	}
}

func TestRunSupervisorExecuteStopsAtBoundedStepLimit(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "bounded execution", Profile: "code", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	supervisor := application.NewRunSupervisor(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	result, err := supervisor.Execute(ctx, run.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Steps) != 2 || result.StopReason != "step_limit" || result.RunStatus != domain.RunRunning || result.Steps[1].Checkpoint.NextTurn != 3 {
		t.Fatalf("unexpected bounded execution: %#v", result)
	}
}

func TestRunSupervisorRootFinishCommitsTurnAndTerminalStateAtomically(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "finish through protocol", Profile: "review", ModelRoute: "lifecycle-test/model", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionFinish, "The review is complete.", "review complete", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	supervisor := application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
	execution, err := supervisor.Execute(ctx, run.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if execution.StopReason != "root_finish" || execution.RunStatus != domain.RunCompleted || len(execution.Steps) != 1 {
		t.Fatalf("unexpected finish execution: %#v", execution)
	}
	result := execution.Steps[0]
	if result.Action.Kind != domain.RootActionFinish || result.RunStatus != domain.RunCompleted || result.Checkpoint.Phase != domain.SupervisorRunCompleted || result.Checkpoint.NextTurn != 2 {
		t.Fatalf("unexpected finish result: %#v", result)
	}
	persisted, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != domain.RunCompleted || persisted.FinishedAt == nil {
		t.Fatalf("run was not finalized: %#v", persisted)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Content != "The review is complete." || strings.Contains(messages[1].Content, "root_lifecycle") {
		t.Fatalf("protocol JSON leaked into session history: %#v", messages)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.SupervisorActionEvent) != 1 || countEventType(items, events.SupervisorRunCompletedEvent) != 1 || countEventType(items, events.RunStatusChangedEvent) != 3 {
		t.Fatalf("unexpected finish event stream: %#v", items)
	}
	before := len(items)
	retryCheckpoint := domain.SupervisorCheckpoint{
		RunID: run.ID, NextTurn: 1, Phase: domain.SupervisorTurnStarted,
		AttemptID: result.AttemptID, UpdatedAt: result.Checkpoint.UpdatedAt,
	}
	_, retried, err := st.CompleteSupervisorTurn(ctx, retryCheckpoint, "ignored retry",
		llm.ChatResponse{Text: "ignored", Provider: provider.Name(), Model: "model"},
		domain.RootAction{Version: domain.RootLifecycleVersion, Kind: domain.RootActionFinish, Message: "ignored", Summary: "review complete"},
		policy.Decision{Allowed: true, Reason: "allowed"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Phase != domain.SupervisorRunCompleted || retried.NextTurn != 2 {
		t.Fatalf("unexpected idempotent finish checkpoint: %#v", retried)
	}
	if _, err := supervisor.Finalize(ctx, run.ID, application.LifecycleOutcomeCompleted, "repeat"); err != nil {
		t.Fatal(err)
	}
	items, err = st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != before {
		t.Fatalf("explicit completion duplicated protocol finalization: before=%d after=%d", before, len(items))
	}
}

func TestRunSupervisorRootWaitPausesAndResumesAtNextTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "wait through protocol", Profile: "learn", ModelRoute: "lifecycle-test/model", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionWait, "I need the user's choice.", "", "user input required"),
		rootActionResponse(domain.RootActionContinue, "Continuing with the supplied choice.", "", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	supervisor := application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
	execution, err := supervisor.Execute(ctx, run.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if execution.StopReason != "root_wait" || execution.RunStatus != domain.RunPaused || len(execution.Steps) != 1 || execution.Steps[0].Checkpoint.Phase != domain.SupervisorWaiting {
		t.Fatalf("unexpected wait result: %#v", execution)
	}
	parked, err := supervisor.Execute(ctx, run.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if parked.StopReason != "run_paused" || parked.RunStatus != domain.RunPaused || len(parked.Steps) != 0 || provider.calls != 1 {
		t.Fatalf("paused run did not remain parked: result=%#v calls=%d", parked, provider.calls)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service = application.NewRunService(st)
	supervisor = application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
	if _, err := service.Resume(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	continued, err := supervisor.Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Turn != 2 || continued.Action.Kind != domain.RootActionContinue || continued.RunStatus != domain.RunRunning || continued.Checkpoint.Phase != domain.SupervisorIdle || continued.Checkpoint.NextTurn != 3 {
		t.Fatalf("unexpected resumed result: %#v", continued)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.SupervisorRunWaitingEvent) != 1 || countEventType(items, events.SupervisorActionEvent) != 2 {
		t.Fatalf("unexpected wait event stream: %#v", items)
	}
}

func TestRunSupervisorRejectsMalformedRootActionWithoutMessages(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "reject malformed lifecycle", Profile: "code", ModelRoute: "lifecycle-test/model", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	provider := &lifecycleProvider{responses: []string{`{"version":"root_lifecycle.v1","action":"continue","message":"ok","unknown":true}`}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || result.Checkpoint.Phase != domain.SupervisorTurnFailed {
		t.Fatalf("malformed lifecycle action was not checkpointed: result=%#v code=%s err=%v", result, apperror.CodeOf(err), err)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("malformed action wrote session messages: %#v", messages)
	}
	persisted, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != domain.RunRunning {
		t.Fatalf("malformed action changed run status: %#v", persisted)
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
	return &llm.ChatResponse{Text: rootActionResponse(domain.RootActionContinue, p.text, "", ""), Provider: p.Name(), Model: "model"}, nil
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

type fixedUsageProvider struct {
	lastMaxTokens int
	lastJSONMode  bool
	lastSchema    string
}

func (*fixedUsageProvider) Name() string { return "usage-test" }

func (*fixedUsageProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "usage-test"}}, nil
}

func (p *fixedUsageProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	p.lastMaxTokens = req.MaxTokens
	p.lastJSONMode = req.JSONMode
	p.lastSchema = req.Metadata["response_schema"]
	return &llm.ChatResponse{
		Text: rootActionResponse(domain.RootActionContinue, "bounded response", "", ""), Provider: p.Name(), Model: "model",
		Usage: llm.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
	}, nil
}

func (p *fixedUsageProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 1)
	chunks <- llm.ChatChunk{Text: response.Text, Done: true}
	close(chunks)
	return chunks, nil
}

func (*fixedUsageProvider) SupportsTools(string) bool    { return false }
func (*fixedUsageProvider) SupportsVision(string) bool   { return false }
func (*fixedUsageProvider) SupportsJSONMode(string) bool { return false }

type blockingProvider struct{}

func (blockingProvider) Name() string { return "blocking-test" }

func (blockingProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "blocking-test"}}, nil
}

func (blockingProvider) Chat(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (p blockingProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	_, err := p.Chat(ctx, req)
	return nil, err
}

func (blockingProvider) SupportsTools(string) bool    { return false }
func (blockingProvider) SupportsVision(string) bool   { return false }
func (blockingProvider) SupportsJSONMode(string) bool { return false }

type nilResponseProvider struct{}

func (nilResponseProvider) Name() string { return "nil-test" }

func (nilResponseProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "nil-test"}}, nil
}

func (nilResponseProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}

func (nilResponseProvider) StreamChat(context.Context, llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	return nil, nil
}

func (nilResponseProvider) SupportsTools(string) bool    { return false }
func (nilResponseProvider) SupportsVision(string) bool   { return false }
func (nilResponseProvider) SupportsJSONMode(string) bool { return false }

type lifecycleProvider struct {
	responses []string
	calls     int
}

func (*lifecycleProvider) Name() string { return "lifecycle-test" }

func (*lifecycleProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "lifecycle-test"}}, nil
}

func (p *lifecycleProvider) Chat(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.calls >= len(p.responses) {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "lifecycle test response exhausted")
	}
	text := p.responses[p.calls]
	p.calls++
	return &llm.ChatResponse{
		Text: text, Provider: p.Name(), Model: "model",
		Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	}, nil
}

func (p *lifecycleProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 1)
	chunks <- llm.ChatChunk{Text: response.Text, Done: true}
	close(chunks)
	return chunks, nil
}

func (*lifecycleProvider) SupportsTools(string) bool    { return false }
func (*lifecycleProvider) SupportsVision(string) bool   { return false }
func (*lifecycleProvider) SupportsJSONMode(string) bool { return true }

func rootActionResponse(kind domain.RootActionKind, message string, summary string, reason string) string {
	encoded, err := json.Marshal(domain.RootAction{
		Version: domain.RootLifecycleVersion,
		Kind:    kind,
		Message: message,
		Summary: summary,
		Reason:  reason,
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
