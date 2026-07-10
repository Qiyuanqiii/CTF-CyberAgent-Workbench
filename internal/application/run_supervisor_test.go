package application_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
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
	started, err := st.BeginSupervisorTurn(ctx, run.ID, "")
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
	_, checkpoint, _, err := st.CompleteSupervisorTurn(ctx, started.Checkpoint,
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

func TestRunSupervisorRecoversCustomPendingInputAcrossStoreRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "pending input recovery", Profile: "review", ModelRoute: "lifecycle-test/model", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	started, err := st.BeginSupervisorTurn(ctx, run.ID, "durable custom request")
	if err != nil {
		t.Fatal(err)
	}
	if started.Checkpoint.PendingInput != "durable custom request" {
		t.Fatalf("pending input was not checkpointed: %#v", started.Checkpoint)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "Recovered input.", "", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Recovered || result.UserMessage.Content != "durable custom request" || result.Checkpoint.PendingInput != "" || result.Checkpoint.NextTurn != 2 {
		t.Fatalf("custom input was not recovered exactly once: %#v", result)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(messages) != 2 || messages[0].Content != "durable custom request" {
		t.Fatalf("unexpected recovered messages: %#v err=%v", messages, err)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.AgentTurnStartedEvent) != 1 || countEventType(items, events.AgentTurnCompletedEvent) != 1 {
		t.Fatalf("custom recovery duplicated events: %#v", items)
	}
}

func TestRunSupervisorRejectsConflictingRecoveredInput(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "input conflict", Profile: "review", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	started, err := st.BeginSupervisorTurn(ctx, run.ID, "first durable request")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginSupervisorTurn(ctx, run.ID, "different request"); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("conflicting input code=%s err=%v", apperror.CodeOf(err), err)
	}
	checkpoint, ok, err := st.GetSupervisorCheckpoint(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("checkpoint lookup ok=%t err=%v", ok, err)
	}
	if checkpoint.AttemptID != started.Checkpoint.AttemptID || checkpoint.PendingInput != "first durable request" {
		t.Fatalf("conflict changed durable input: %#v", checkpoint)
	}
}

func TestRunSupervisorBoundsAndRedactsCustomInputBeforeCheckpoint(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "input boundary", Profile: "review", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginSupervisorTurn(ctx, run.ID, strings.Repeat("x", 64*1024+1)); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("oversized input code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, ok, err := st.GetSupervisorCheckpoint(ctx, run.ID); err != nil || ok {
		t.Fatalf("oversized input created checkpoint ok=%t err=%v", ok, err)
	}
	started, err := st.BeginSupervisorTurn(ctx, run.ID, "MIMO_API_KEY="+"t"+"p-"+strings.Repeat("1", 30))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(started.Checkpoint.PendingInput, "1234567890") || !strings.Contains(started.Checkpoint.PendingInput, "[REDACTED:") {
		t.Fatalf("pending input was not redacted: %q", started.Checkpoint.PendingInput)
	}
}

func TestRunSupervisorRetriesTransientProviderFailuresAndCommitsOnce(t *testing.T) {
	token := "t" + "p-" + strings.Repeat("r", 40)
	provider := &retrySequenceProvider{failures: []error{
		llm.NewProviderError(llm.OutcomeRetryable, "retry-test", "MIMO_API_KEY="+token, nil),
		llm.NewProviderError(llm.OutcomeRetryable, "retry-test", "connection reset", nil),
	}}
	_, st, run, supervisor := newRetrySupervisor(t, provider)
	supervisor.WithModelRetryPolicy(application.ModelRetryPolicy{MaxAttempts: 3})

	result, err := supervisor.StepWithInput(context.Background(), run.ID, "retry this request")
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 3 || result.ModelAttempts != 3 || result.ModelOutcome != llm.OutcomeSuccess || result.Status != application.LifecycleTurnCompleted {
		t.Fatalf("unexpected retry result calls=%d result=%#v", provider.calls, result)
	}
	messages, err := st.ListSessionMessages(context.Background(), run.SessionID, true)
	if err != nil || len(messages) != 2 || messages[0].Content != "retry this request" {
		t.Fatalf("retry duplicated messages: %#v err=%v", messages, err)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 3 || countEventType(items, events.ModelFailedEvent) != 2 ||
		countEventType(items, events.ModelCompletedEvent) != 1 || countEventType(items, events.AgentTurnCompletedEvent) != 1 {
		t.Fatalf("unexpected retry event stream: %#v", items)
	}
	foundNormalizedUsage := false
	for _, item := range items {
		if item.Type == events.ModelFailedEvent && strings.Contains(item.PayloadJSON, token[:12]) {
			t.Fatalf("model failure event leaked a token: %s", item.PayloadJSON)
		}
		if item.Type == events.ModelCompletedEvent && strings.Contains(item.PayloadJSON, `"input_tokens":2`) {
			foundNormalizedUsage = true
		}
	}
	if !foundNormalizedUsage {
		t.Fatalf("model completion usage is not normalized: %#v", items)
	}
}

func TestRunSupervisorDoesNotRetryPermanentProviderFailure(t *testing.T) {
	provider := &retrySequenceProvider{failures: []error{
		llm.NewProviderError(llm.OutcomePermanent, "retry-test", "invalid credentials", nil),
	}}
	_, st, run, supervisor := newRetrySupervisor(t, provider)
	supervisor.WithModelRetryPolicy(application.ModelRetryPolicy{MaxAttempts: 3})

	result, err := supervisor.StepWithInput(context.Background(), run.ID, "do not retry forever")
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || provider.calls != 1 {
		t.Fatalf("permanent failure code=%s calls=%d err=%v", apperror.CodeOf(err), provider.calls, err)
	}
	if result.ModelAttempts != 1 || result.ModelOutcome != llm.OutcomePermanent || result.Checkpoint.Phase != domain.SupervisorTurnFailed || result.Checkpoint.PendingInput != "do not retry forever" {
		t.Fatalf("unexpected permanent failure result: %#v", result)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 1 || countEventType(items, events.ModelFailedEvent) != 1 || countEventType(items, events.ModelCompletedEvent) != 0 {
		t.Fatalf("permanent failure event stream: %#v", items)
	}
}

func TestRunSupervisorPreservesPendingInputAfterRateLimitExhaustion(t *testing.T) {
	rateLimit := func() error {
		err := llm.NewProviderError(llm.OutcomeRateLimited, "retry-test", "capacity reached", nil)
		err.StatusCode = 429
		return err
	}
	provider := &retrySequenceProvider{failures: []error{rateLimit(), rateLimit(), rateLimit()}}
	_, st, run, supervisor := newRetrySupervisor(t, provider)
	supervisor.WithModelRetryPolicy(application.ModelRetryPolicy{MaxAttempts: 3})
	ctx := context.Background()

	first, err := supervisor.StepWithInput(ctx, run.ID, "durable rate-limited input")
	if apperror.CodeOf(err) != apperror.CodeResourceExhausted || provider.calls != 3 {
		t.Fatalf("rate limit code=%s calls=%d err=%v", apperror.CodeOf(err), provider.calls, err)
	}
	if first.Checkpoint.Phase != domain.SupervisorTurnFailed || first.Checkpoint.PendingInput != "durable rate-limited input" || first.ModelOutcome != llm.OutcomeRateLimited {
		t.Fatalf("rate limit did not preserve input: %#v", first)
	}
	second, err := supervisor.Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.UserMessage.Content != "durable rate-limited input" || second.ModelAttempts != 1 || provider.calls != 4 {
		t.Fatalf("rate-limited input was not resumed: calls=%d result=%#v", provider.calls, second)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(messages) != 2 {
		t.Fatalf("rate-limit recovery messages=%#v err=%v", messages, err)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 4 || countEventType(items, events.ModelFailedEvent) != 3 || countEventType(items, events.ModelCompletedEvent) != 1 {
		t.Fatalf("rate-limit recovery events: %#v", items)
	}
}

func TestRunSupervisorDoesNotRetryPastLongProviderRetryAfter(t *testing.T) {
	rateLimit := llm.NewProviderError(llm.OutcomeRateLimited, "retry-test", "retry later", nil)
	rateLimit.RetryAfter = time.Hour
	provider := &retrySequenceProvider{failures: []error{rateLimit}}
	_, st, run, supervisor := newRetrySupervisor(t, provider)
	supervisor.WithModelRetryPolicy(application.ModelRetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Second})

	result, err := supervisor.StepWithInput(context.Background(), run.ID, "respect retry after")
	if apperror.CodeOf(err) != apperror.CodeResourceExhausted || provider.calls != 1 || result.ModelAttempts != 1 {
		t.Fatalf("long retry-after code=%s calls=%d result=%#v err=%v", apperror.CodeOf(err), provider.calls, result, err)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Type == events.ModelFailedEvent {
			found = strings.Contains(item.PayloadJSON, `"retry_planned":false`) && strings.Contains(item.PayloadJSON, `"retry_after_millis":3600000`)
		}
	}
	if !found {
		t.Fatalf("long retry-after event was not bounded: %#v", items)
	}
}

func TestSupervisorModelTerminalReplayDoesNotDoubleChargeBudget(t *testing.T) {
	provider := &retrySequenceProvider{}
	_, st, run, _ := newRetrySupervisor(t, provider)
	ctx := context.Background()
	turn, err := st.BeginSupervisorTurn(ctx, run.ID, "idempotent model event")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{Number: 1, MaxAttempts: 3, Provider: provider.Name(), Model: "model"}
	outOfOrder := attempt
	outOfOrder.Number = 2
	if _, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, outOfOrder); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("out-of-order model attempt code=%s err=%v", apperror.CodeOf(err), err)
	}
	inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("model start inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeRetryable
	attempt.ErrorText = "temporary"
	attempt.Elapsed = 25 * time.Millisecond
	mismatched := attempt
	mismatched.Model = "other-model"
	if _, err := st.RecordSupervisorModelFailed(ctx, turn.Checkpoint, mismatched); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("mismatched model terminal code=%s err=%v", apperror.CodeOf(err), err)
	}
	first, err := st.RecordSupervisorModelFailed(ctx, turn.Checkpoint, attempt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.RecordSupervisorModelFailed(ctx, turn.Checkpoint, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExecutionMillis != 25 || second.ExecutionMillis != first.ExecutionMillis {
		t.Fatalf("terminal replay changed budget first=%#v second=%#v", first, second)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 1 || countEventType(items, events.ModelFailedEvent) != 1 {
		t.Fatalf("terminal replay duplicated events: %#v", items)
	}
}

func TestSupervisorProtocolFailureReplayIsAtomicAndIdempotent(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2, MaxTokens: 10})
	turn, err := st.BeginSupervisorTurn(ctx, run.ID, "idempotent protocol failure")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "lifecycle-test", Model: "model"}
	inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("model start inserted=%t err=%v", inserted, err)
	}
	response := llm.ChatResponse{Usage: llm.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5}}
	first, err := st.RecordSupervisorProtocolFailure(ctx, turn.Checkpoint, attempt, response, "invalid protocol", true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.RecordSupervisorProtocolFailure(ctx, turn.Checkpoint, attempt, response, "invalid protocol", true)
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalTokens != 5 || second.TotalTokens != first.TotalTokens || second.RepairPhase != domain.ProtocolRepairPending {
		t.Fatalf("protocol failure replay changed checkpoint first=%#v second=%#v", first, second)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelFailedEvent) != 1 || countEventType(items, events.ProtocolRepairRequestedEvent) != 1 {
		t.Fatalf("protocol failure replay duplicated events: %#v", items)
	}
}

func TestRunSupervisorCancellationDuringBackoffResumesNextModelAttempt(t *testing.T) {
	provider := &retrySequenceProvider{failures: []error{
		llm.NewProviderError(llm.OutcomeRetryable, "retry-test", "temporary outage", nil),
	}, delays: []time.Duration{20 * time.Millisecond}}
	path, st, run, supervisor := newRetrySupervisor(t, provider)
	supervisor.WithModelRetryPolicy(application.ModelRetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := supervisor.StepWithInput(ctx, run.ID, "resume after cancellation")
	if apperror.CodeOf(err) != apperror.CodeDeadlineExceeded || provider.calls != 1 {
		t.Fatalf("cancelled backoff code=%s calls=%d err=%v", apperror.CodeOf(err), provider.calls, err)
	}
	checkpoint, ok, err := st.GetSupervisorCheckpoint(context.Background(), run.ID)
	if err != nil || !ok || checkpoint.Phase != domain.SupervisorTurnStarted || checkpoint.PendingInput != "resume after cancellation" || checkpoint.ExecutionMillis < 10 {
		t.Fatalf("cancelled checkpoint ok=%t checkpoint=%#v err=%v", ok, checkpoint, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	supervisor = application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).WithModelRetryPolicy(
		application.ModelRetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Second},
	)
	resumed, err := supervisor.Step(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Recovered || resumed.ModelAttempts != 2 || resumed.UserMessage.Content != "resume after cancellation" || provider.calls != 2 || resumed.Checkpoint.ExecutionMillis < checkpoint.ExecutionMillis {
		t.Fatalf("cancelled attempt did not resume: calls=%d result=%#v", provider.calls, resumed)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 2 || countEventType(items, events.ModelFailedEvent) != 1 || countEventType(items, events.ModelCompletedEvent) != 1 || countEventType(items, events.AgentTurnFailedEvent) != 0 {
		t.Fatalf("cancel/resume event stream: %#v", items)
	}
}

func TestRunSupervisorAuditsCancellationDuringProviderCall(t *testing.T) {
	_, st, run, supervisor := newRetrySupervisor(t, blockingProvider{})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	result, err := supervisor.StepWithInput(ctx, run.ID, "cancel active provider")
	if apperror.CodeOf(err) != apperror.CodeDeadlineExceeded {
		t.Fatalf("provider cancellation code=%s err=%v", apperror.CodeOf(err), err)
	}
	if result.Checkpoint.Phase != domain.SupervisorTurnStarted || result.Checkpoint.PendingInput != "cancel active provider" || result.Checkpoint.ExecutionMillis < 20 {
		t.Fatalf("provider cancellation was not durably checkpointed: %#v", result)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundCancelled := false
	for _, item := range items {
		if item.Type == events.ModelFailedEvent && strings.Contains(item.PayloadJSON, `"outcome":"cancelled"`) {
			foundCancelled = true
		}
	}
	if countEventType(items, events.ModelStartedEvent) != 1 || countEventType(items, events.ModelFailedEvent) != 1 || countEventType(items, events.AgentTurnFailedEvent) != 0 || !foundCancelled {
		t.Fatalf("provider cancellation event stream: %#v", items)
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
	turn, err := st.BeginSupervisorTurn(ctx, run.ID, "")
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
	turn, err := st.BeginSupervisorTurn(ctx, run.ID, "")
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
	_, retried, _, err := st.CompleteSupervisorTurn(ctx, retryCheckpoint,
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

func TestRunSupervisorRepairsMalformedRootActionOnce(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2})
	invalidRaw := `{"version":"root_lifecycle.v1","action":"continue","message":"do-not-persist-secret-value","unknown":true}`
	provider := &lifecycleProvider{responses: []string{
		invalidRaw,
		rootActionResponse(domain.RootActionContinue, "protocol repaired", "", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || result.Status != application.LifecycleTurnCompleted || result.ModelAttempts != 2 ||
		result.ProtocolRepairs != 1 || result.ModelOutcome != llm.OutcomeSuccess || result.Checkpoint.TotalTokens != 4 {
		t.Fatalf("malformed lifecycle action was not repaired exactly once: calls=%d result=%#v", provider.calls, result)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Content != "protocol repaired" {
		t.Fatalf("repair did not commit exactly one clean session turn: %#v", messages)
	}
	if provider.requests[1].Metadata["protocol_repair"] != "1" {
		t.Fatalf("repair request metadata is missing: %#v", provider.requests[1].Metadata)
	}
	for _, message := range provider.requests[1].Messages {
		if strings.Contains(message.Content, invalidRaw) || strings.Contains(message.Content, "do-not-persist-secret-value") {
			t.Fatalf("repair prompt replayed the invalid model output: %#v", provider.requests[1].Messages)
		}
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if strings.Contains(item.PayloadJSON, "do-not-persist-secret-value") {
			t.Fatalf("invalid model output leaked into event stream: %s", item.PayloadJSON)
		}
	}
	if countEventType(items, events.ModelStartedEvent) != 2 || countEventType(items, events.ModelFailedEvent) != 1 ||
		countEventType(items, events.ModelCompletedEvent) != 1 || countEventType(items, events.ProtocolRepairRequestedEvent) != 1 ||
		countEventType(items, events.ProtocolRepairStartedEvent) != 1 || countEventType(items, events.ProtocolRepairCompletedEvent) != 1 ||
		countEventType(items, events.ProtocolRepairFailedEvent) != 0 {
		t.Fatalf("unexpected successful repair event stream: %#v", items)
	}
}

func TestRunSupervisorSeparatesRepairTransportAttemptsFromGlobalSequence(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2})
	provider := &lifecycleProvider{
		responses: []string{
			`{"version":"root_lifecycle.v1","action":"continue","message":"invalid","unknown":true}`,
			"",
			rootActionResponse(domain.RootActionContinue, "repair transport recovered", "", ""),
		},
		failures: []error{
			nil,
			llm.NewProviderError(llm.OutcomeRetryable, "lifecycle-test", "temporary repair transport failure", nil),
		},
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).WithModelRetryPolicy(
		application.ModelRetryPolicy{MaxAttempts: 3},
	).Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 3 || result.ModelAttempts != 3 || result.ProtocolRepairs != 1 || result.ModelOutcome != llm.OutcomeSuccess {
		t.Fatalf("repair transport counters were not independent: calls=%d result=%#v", provider.calls, result)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantAttempts := []string{
		`"model_attempt":1,"protocol_repair":0,"provider":"lifecycle-test","transport_attempt":1`,
		`"model_attempt":2,"protocol_repair":1,"provider":"lifecycle-test","transport_attempt":1`,
		`"model_attempt":3,"protocol_repair":1,"provider":"lifecycle-test","transport_attempt":2`,
	}
	found := make([]bool, len(wantAttempts))
	for _, item := range items {
		if item.Type != events.ModelStartedEvent {
			continue
		}
		for index, want := range wantAttempts {
			if strings.Contains(item.PayloadJSON, want) {
				found[index] = true
			}
		}
	}
	if slices.Contains(found, false) || countEventType(items, events.ProtocolRepairStartedEvent) != 1 {
		t.Fatalf("unexpected global/transport model sequence found=%v events=%#v", found, items)
	}
}

func TestRunSupervisorFailsAfterSecondMalformedRootAction(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2})
	provider := &lifecycleProvider{responses: []string{
		`{"version":"root_lifecycle.v1","action":"continue","message":"first","unknown":true}`,
		`{"version":"root_lifecycle.v1","action":"continue","message":"second","unknown":true}`,
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || provider.calls != 2 || result.ModelAttempts != 2 ||
		result.ProtocolRepairs != 1 || result.ModelOutcome != llm.OutcomeInvalidResponse || result.Checkpoint.Phase != domain.SupervisorTurnFailed ||
		result.Checkpoint.RepairPhase != domain.ProtocolRepairNone || result.Checkpoint.TotalTokens != 4 {
		t.Fatalf("second malformed response was not bounded: calls=%d result=%#v code=%s err=%v", provider.calls, result, apperror.CodeOf(err), err)
	}
	messages, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(messages) != 0 {
		t.Fatalf("failed repair wrote session messages: %#v err=%v", messages, err)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 2 || countEventType(items, events.ModelFailedEvent) != 2 ||
		countEventType(items, events.ModelCompletedEvent) != 0 || countEventType(items, events.ProtocolRepairRequestedEvent) != 1 ||
		countEventType(items, events.ProtocolRepairStartedEvent) != 1 || countEventType(items, events.ProtocolRepairCompletedEvent) != 0 ||
		countEventType(items, events.ProtocolRepairFailedEvent) != 1 {
		t.Fatalf("unexpected exhausted repair event stream: %#v", items)
	}
}

func TestRunSupervisorChargesInvalidResponseBeforeRepairBudgetCheck(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2, MaxTokens: 2})
	provider := &lifecycleProvider{responses: []string{
		`{"version":"root_lifecycle.v1","action":"continue","message":"invalid","unknown":true}`,
		rootActionResponse(domain.RootActionContinue, "must not be called", "", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeResourceExhausted || provider.calls != 1 || result.ProtocolRepairs != 1 ||
		result.Checkpoint.Phase != domain.SupervisorTurnFailed || result.Checkpoint.TotalTokens != 2 {
		t.Fatalf("invalid response did not consume budget before repair: calls=%d result=%#v code=%s err=%v", provider.calls, result, apperror.CodeOf(err), err)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ProtocolRepairRequestedEvent) != 1 || countEventType(items, events.ProtocolRepairStartedEvent) != 0 {
		t.Fatalf("budget exhaustion started a repair model call: %#v", items)
	}
}

func TestRunSupervisorResumesPendingProtocolRepairAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2, MaxTokens: 10})
	pending := persistProtocolRepairRequest(t, st, run.ID, "durable repair input", "lifecycle-test")
	if pending.RepairPhase != domain.ProtocolRepairPending || pending.TotalTokens != 2 {
		t.Fatalf("repair request was not checkpointed: %#v", pending)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "repaired after restart", "", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Recovered || provider.calls != 1 || result.ModelAttempts != 2 || result.ProtocolRepairs != 1 ||
		result.UserMessage.Content != "durable repair input" || result.Checkpoint.TotalTokens != 4 {
		t.Fatalf("pending protocol repair did not resume: calls=%d result=%#v", provider.calls, result)
	}
	if provider.requests[0].Metadata["protocol_repair"] != "1" || provider.requests[0].MaxTokens != 8 {
		t.Fatalf("recovered repair request lost metadata or budget: %#v", provider.requests[0])
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 2 || countEventType(items, events.ProtocolRepairRequestedEvent) != 1 ||
		countEventType(items, events.ProtocolRepairStartedEvent) != 1 || countEventType(items, events.ProtocolRepairCompletedEvent) != 1 {
		t.Fatalf("recovered repair duplicated or lost events: %#v", items)
	}
}

func TestRunSupervisorDoesNotRetryExhaustedProtocolRepairAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2, MaxTokens: 10})
	pending := persistProtocolRepairRequest(t, st, run.ID, "exhausted repair input", "lifecycle-test")
	attempt := llm.ModelAttempt{
		Number: 2, TransportAttempt: 1, MaxAttempts: 3, ProtocolRepair: 1, Provider: "lifecycle-test", Model: "model",
	}
	inserted, err := st.RecordSupervisorModelStarted(ctx, pending, attempt)
	if err != nil || !inserted {
		t.Fatalf("repair model start inserted=%t err=%v", inserted, err)
	}
	exhausted, err := st.RecordSupervisorProtocolFailure(ctx, pending, attempt,
		llm.ChatResponse{Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}}, "repair response remained invalid", false)
	if err != nil {
		t.Fatal(err)
	}
	if exhausted.RepairPhase != domain.ProtocolRepairExhausted || exhausted.TotalTokens != 4 {
		t.Fatalf("exhausted repair was not checkpointed: %#v", exhausted)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "must not run", "", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || provider.calls != 0 || !result.Recovered ||
		result.ProtocolRepairs != 1 || result.Checkpoint.Phase != domain.SupervisorTurnFailed || result.Checkpoint.TotalTokens != 4 {
		t.Fatalf("exhausted repair was retried after restart: calls=%d result=%#v code=%s err=%v", provider.calls, result, apperror.CodeOf(err), err)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 2 || countEventType(items, events.ProtocolRepairFailedEvent) != 1 {
		t.Fatalf("exhausted repair event stream changed on recovery: %#v", items)
	}
}

func TestRunSupervisorPersistsProtocolRepairWhenCancelledAfterResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	run := newStartedRunForProvider(t, st, "lifecycle-test", domain.Budget{MaxTurns: 2, MaxTokens: 10})
	ctx, cancel := context.WithCancel(context.Background())
	provider := &lifecycleProvider{responses: []string{
		`{"version":"root_lifecycle.v1","action":"continue","message":"invalid","unknown":true}`,
		rootActionResponse(domain.RootActionContinue, "repaired after cancellation", "", ""),
	}}
	provider.afterResponse = func(index int) {
		if index == 0 {
			cancel()
		}
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	first, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeCancelled || provider.calls != 1 || first.ModelAttempts != 1 ||
		first.ProtocolRepairs != 1 || first.Checkpoint.Phase != domain.SupervisorTurnStarted ||
		first.Checkpoint.RepairPhase != domain.ProtocolRepairPending || first.Checkpoint.TotalTokens != 2 {
		t.Fatalf("cancelled response did not persist repair state: calls=%d result=%#v code=%s err=%v", provider.calls, first, apperror.CodeOf(err), err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider.afterResponse = nil
	resumed, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Recovered || provider.calls != 2 || resumed.ModelAttempts != 2 || resumed.ProtocolRepairs != 1 ||
		resumed.UserMessage.Content != "root lifecycle protocol test" || resumed.Checkpoint.TotalTokens != 4 {
		t.Fatalf("cancelled protocol repair did not resume once: calls=%d result=%#v", provider.calls, resumed)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ProtocolRepairRequestedEvent) != 1 || countEventType(items, events.ProtocolRepairStartedEvent) != 1 ||
		countEventType(items, events.ProtocolRepairCompletedEvent) != 1 || countEventType(items, events.AgentTurnStartedEvent) != 1 {
		t.Fatalf("cancelled repair recovery duplicated lifecycle events: %#v", items)
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
	chunks := make(chan llm.ChatChunk, 2)
	chunks <- llm.ChatChunk{Text: response.Text}
	chunks <- llm.FinalChatChunk(response)
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
	chunks := make(chan llm.ChatChunk, 2)
	chunks <- llm.ChatChunk{Text: response.Text}
	chunks <- llm.FinalChatChunk(response)
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
	chunks := make(chan llm.ChatChunk, 2)
	chunks <- llm.ChatChunk{Text: response.Text}
	chunks <- llm.FinalChatChunk(response)
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
	responses     []string
	failures      []error
	requests      []llm.ChatRequest
	calls         int
	afterResponse func(int)
}

type retrySequenceProvider struct {
	failures []error
	delays   []time.Duration
	requests []llm.ChatRequest
	calls    int
}

func (*retrySequenceProvider) Name() string { return "retry-test" }

func (*retrySequenceProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "retry-test"}}, nil
}

func (p *retrySequenceProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index := p.calls
	p.calls++
	p.requests = append(p.requests, req)
	if index < len(p.delays) && p.delays[index] > 0 {
		timer := time.NewTimer(p.delays[index])
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if index < len(p.failures) && p.failures[index] != nil {
		return nil, p.failures[index]
	}
	return &llm.ChatResponse{
		Text:     rootActionResponse(domain.RootActionContinue, "provider recovered", "", ""),
		Provider: p.Name(), Model: "model", Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
	}, nil
}

func (p *retrySequenceProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 2)
	chunks <- llm.ChatChunk{Text: response.Text}
	chunks <- llm.FinalChatChunk(response)
	close(chunks)
	return chunks, nil
}

func (*retrySequenceProvider) SupportsTools(string) bool    { return false }
func (*retrySequenceProvider) SupportsVision(string) bool   { return false }
func (*retrySequenceProvider) SupportsJSONMode(string) bool { return true }

func newRetrySupervisor(t *testing.T, provider llm.Provider) (string, *store.SQLiteStore, domain.Run, *application.RunSupervisor) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "provider retry test", Profile: "review", ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	return path, st, run, application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
}

func newStartedRunForProvider(t *testing.T, st *store.SQLiteStore, providerName string, budget domain.Budget) domain.Run {
	t.Helper()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "root lifecycle protocol test", Profile: "review", ModelRoute: providerName + "/model", Budget: budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	return run
}

func persistProtocolRepairRequest(t *testing.T, st *store.SQLiteStore, runID string, input string, providerName string) domain.SupervisorCheckpoint {
	t.Helper()
	ctx := context.Background()
	turn, err := st.BeginSupervisorTurn(ctx, runID, input)
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: providerName, Model: "model",
	}
	inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("primary model start inserted=%t err=%v", inserted, err)
	}
	checkpoint, err := st.RecordSupervisorProtocolFailure(ctx, turn.Checkpoint, attempt,
		llm.ChatResponse{Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}}, "root lifecycle response was invalid", true)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func (*lifecycleProvider) Name() string { return "lifecycle-test" }

func (*lifecycleProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "lifecycle-test"}}, nil
}

func (p *lifecycleProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index := p.calls
	if index >= len(p.responses) && index >= len(p.failures) {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "lifecycle test response exhausted")
	}
	p.requests = append(p.requests, req)
	p.calls++
	if index < len(p.failures) && p.failures[index] != nil {
		return nil, p.failures[index]
	}
	if index >= len(p.responses) {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "lifecycle test response exhausted")
	}
	text := p.responses[index]
	if p.afterResponse != nil {
		p.afterResponse(index)
	}
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
	chunks := make(chan llm.ChatChunk, 2)
	chunks <- llm.ChatChunk{Text: response.Text}
	chunks <- llm.FinalChatChunk(response)
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
