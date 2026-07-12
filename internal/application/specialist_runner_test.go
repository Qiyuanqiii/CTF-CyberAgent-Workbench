package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/coordinator"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
)

func TestSpecialistRunnerExecutesInternalNoToolContinuation(t *testing.T) {
	provider := &specialistTestProvider{responses: []llm.ChatResponse{{
		Text: specialistResponse(t, domain.SpecialistAction{
			Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionContinue,
			Message: "continue the focused review",
		}), Usage: llm.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
	}}}
	st, run, child, runner := newSpecialistRunnerFixture(t, provider,
		domain.Budget{MaxTurns: 10}, 2, 32)
	result, err := runner.Step(context.Background(), run.ID, child.ID)
	if err != nil || result.AttemptStatus != domain.AgentAttemptContinued ||
		result.Action.Kind != domain.SpecialistActionContinue || result.Usage.TotalTokens != 5 ||
		result.ModelOutcome != llm.OutcomeSuccess || result.ModelAttempts != 1 {
		t.Fatalf("Specialist continuation failed: result=%#v err=%v", result, err)
	}
	updated, err := st.GetAgentNode(context.Background(), child.ID)
	if err != nil || updated.Status != domain.AgentReady || updated.TurnsUsed != 1 ||
		updated.TokensUsed != 5 || updated.ActiveAttemptID != "" {
		t.Fatalf("continued Specialist projection is invalid: child=%#v err=%v", updated, err)
	}
	messages, err := st.ListSessionMessages(context.Background(), child.SessionID, true)
	if err != nil || len(messages) != 2 || messages[0].Role != "user" ||
		messages[1].Role != "assistant" || messages[1].Content != "continue the focused review" {
		t.Fatalf("Specialist Session history is invalid: messages=%#v err=%v", messages, err)
	}
	if len(provider.requests) != 1 || len(provider.requests[0].Tools) != 0 ||
		!provider.requests[0].JSONMode ||
		provider.requests[0].Metadata["response_schema"] != domain.SpecialistLifecycleVersion ||
		provider.requests[0].MaxTokens != 32 {
		t.Fatalf("Specialist provider request escaped its no-tool budget: %#v", provider.requests)
	}
	assertSpecialistEventCounts(t, st, run.ID, map[string]int{
		events.ModelStartedEvent: 1, events.ModelCompletedEvent: 1,
		events.PolicyDecisionEvent: 1, events.AgentAttemptUsageRecordedEvent: 1,
		events.AgentTurnCompletedEvent: 1,
	})
}

func TestSpecialistRunnerFinishesWithCompletionReport(t *testing.T) {
	report := domain.CompletionReport{
		Version: domain.CompletionReportVersion, Outcome: domain.CompletionSucceeded,
		Summary: "review completed safely", WorkItemIDs: []string{}, NoteIDs: []string{},
	}
	provider := &specialistTestProvider{responses: []llm.ChatResponse{{
		Text: specialistResponse(t, domain.SpecialistAction{
			Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionFinish,
			Message: "the assigned review is complete", Report: &report,
		}), Usage: llm.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
	}}}
	st, run, child, runner := newSpecialistRunnerFixture(t, provider,
		domain.Budget{MaxTurns: 10}, 2, 32)
	result, err := runner.Step(context.Background(), run.ID, child.ID)
	if err != nil || result.AttemptStatus != domain.AgentAttemptFinished ||
		result.Completion.Report.Outcome != domain.CompletionSucceeded ||
		result.Completion.AttemptID != result.AttemptID {
		t.Fatalf("Specialist finish failed: result=%#v err=%v", result, err)
	}
	updated, err := st.GetAgentNode(context.Background(), child.ID)
	if err != nil || updated.Status != domain.AgentCompleted || updated.FinishedAt == nil {
		t.Fatalf("finished Specialist projection is invalid: child=%#v err=%v", updated, err)
	}
	childSession, err := st.GetSession(context.Background(), child.SessionID)
	if err != nil || childSession.Status != session.StatusArchived {
		t.Fatalf("finished Specialist Session was not archived: session=%#v err=%v",
			childSession, err)
	}
	root, found, err := st.GetRootAgent(context.Background(), run.ID)
	if err != nil || !found {
		t.Fatalf("root Agent was not found: found=%t err=%v", found, err)
	}
	inbox, err := st.ListAgentMessages(context.Background(), root.ID, true, 10)
	if err != nil || len(inbox) != 1 || inbox[0].Kind != domain.AgentMessageResult {
		t.Fatalf("completion did not reach root inbox: messages=%#v err=%v", inbox, err)
	}
}

func TestSpecialistRunnerRetriesTransportFailureAndChargesOnce(t *testing.T) {
	provider := &specialistTestProvider{
		failures: []error{llm.NewProviderError(llm.OutcomeRetryable,
			"specialist-test", "temporary reset", nil), nil},
		responses: []llm.ChatResponse{{}, {
			Text: specialistResponse(t, domain.SpecialistAction{
				Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionContinue,
				Message: "recovered after retry",
			}), Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
		}},
	}
	st, run, child, runner := newSpecialistRunnerFixture(t, provider,
		domain.Budget{MaxTurns: 10}, 2, 32)
	runner.WithModelRetryPolicy(application.ModelRetryPolicy{MaxAttempts: 2})
	result, err := runner.Step(context.Background(), run.ID, child.ID)
	if err != nil || result.ModelAttempts != 2 || result.Usage.TotalTokens != 4 ||
		result.AttemptStatus != domain.AgentAttemptContinued {
		t.Fatalf("Specialist retry failed: result=%#v err=%v", result, err)
	}
	assertSpecialistEventCounts(t, st, run.ID, map[string]int{
		events.ModelStartedEvent: 2, events.ModelFailedEvent: 1,
		events.ModelCompletedEvent: 1, events.AgentAttemptUsageRecordedEvent: 1,
	})
}

func TestSpecialistRunnerRefusesContinueAfterChildBudgetExhaustion(t *testing.T) {
	provider := &specialistTestProvider{responses: []llm.ChatResponse{{
		Text: specialistResponse(t, domain.SpecialistAction{
			Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionContinue,
			Message: "request another turn",
		}), Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	}}}
	st, run, child, runner := newSpecialistRunnerFixture(t, provider,
		domain.Budget{MaxTurns: 10}, 1, 16)
	result, err := runner.Step(context.Background(), run.ID, child.ID)
	if apperror.CodeOf(err) != apperror.CodeResourceExhausted ||
		result.AttemptStatus != domain.AgentAttemptCrashed || result.Usage.TotalTokens != 2 {
		t.Fatalf("exhausted child controlled its own continuation: result=%#v code=%s err=%v",
			result, apperror.CodeOf(err), err)
	}
	updated, loadErr := st.GetAgentNode(context.Background(), child.ID)
	if loadErr != nil || updated.Status != domain.AgentFailed || updated.FinishedAt == nil {
		t.Fatalf("exhausted child was not terminated: child=%#v err=%v", updated, loadErr)
	}
	childSession, loadErr := st.GetSession(context.Background(), child.SessionID)
	if loadErr != nil || childSession.Status != session.StatusArchived {
		t.Fatalf("exhausted child Session was not archived: session=%#v err=%v",
			childSession, loadErr)
	}
}

func TestSpecialistRunnerRejectsMalformedToolAndDangerousResponses(t *testing.T) {
	tests := []struct {
		name      string
		response  llm.ChatResponse
		wantCode  apperror.Code
		wantModel llm.Outcome
	}{
		{
			name: "malformed lifecycle",
			response: llm.ChatResponse{Text: `{"action":"continue"}`,
				Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
			wantCode: apperror.CodeFailedPrecondition, wantModel: llm.OutcomeInvalidResponse,
		},
		{
			name: "tool call",
			response: llm.ChatResponse{
				Text: specialistResponse(t, domain.SpecialistAction{
					Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionContinue,
					Message: "request a tool",
				}), Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				ToolCalls: []llm.ToolCall{{
					ID: "call-1", Name: "shell", Arguments: json.RawMessage(`{"command":"echo hi"}`),
				}},
			},
			wantCode: apperror.CodeFailedPrecondition, wantModel: llm.OutcomeInvalidResponse,
		},
		{
			name: "policy denial",
			response: llm.ChatResponse{
				Text: specialistResponse(t, domain.SpecialistAction{
					Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionContinue,
					Message: "run masscan against 0.0.0.0/0",
				}), Usage: llm.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
			},
			wantCode: apperror.CodePolicyDenied, wantModel: llm.OutcomeSuccess,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &specialistTestProvider{responses: []llm.ChatResponse{test.response}}
			st, run, child, runner := newSpecialistRunnerFixture(t, provider,
				domain.Budget{MaxTurns: 10}, 2, 32)
			result, err := runner.Step(context.Background(), run.ID, child.ID)
			if apperror.CodeOf(err) != test.wantCode ||
				result.AttemptStatus != domain.AgentAttemptCrashed ||
				result.ModelOutcome != test.wantModel || result.Usage.TotalTokens <= 0 {
				t.Fatalf("unsafe response was not contained: result=%#v code=%s err=%v",
					result, apperror.CodeOf(err), err)
			}
			messages, err := st.ListSessionMessages(context.Background(), child.SessionID, true)
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "policy denial" && len(messages) != 0 {
				t.Fatalf("policy-denied output entered Session history: %#v", messages)
			}
			timeline, err := st.ListRunEvents(context.Background(), run.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range timeline {
				if strings.HasPrefix(event.Type, "tool.") {
					t.Fatalf("unsafe child response executed a tool: %#v", event)
				}
			}
		})
	}
}

func TestSpecialistRunnerCancellationCrashesAttemptBeforeLeaseRelease(t *testing.T) {
	provider := &specialistTestProvider{block: true, started: make(chan struct{})}
	st, run, child, runner := newSpecialistRunnerFixture(t, provider,
		domain.Budget{MaxTurns: 10}, 2, 32)
	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result application.SpecialistTurnResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runner.Step(ctx, run.ID, child.ID)
		done <- outcome{result: result, err: err}
	}()
	select {
	case <-provider.started:
	case <-time.After(3 * time.Second):
		t.Fatal("Specialist provider did not start")
	}
	cancel()
	select {
	case got := <-done:
		if apperror.CodeOf(got.err) != apperror.CodeCancelled ||
			got.result.AttemptStatus != domain.AgentAttemptCrashed ||
			got.result.ModelOutcome != llm.OutcomeCancelled {
			t.Fatalf("cancelled Specialist did not commit failure: result=%#v code=%s err=%v",
				got.result, apperror.CodeOf(got.err), got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled Specialist did not return")
	}
	updated, err := st.GetAgentNode(context.Background(), child.ID)
	if err != nil || updated.Status != domain.AgentReady || updated.ActiveAttemptID != "" {
		t.Fatalf("cancelled child was left running: child=%#v err=%v", updated, err)
	}
	lease, found, err := st.GetRunExecutionLease(context.Background(), run.ID)
	if err != nil || !found || lease.Status != domain.RunExecutionLeaseReleased {
		t.Fatalf("cancelled child lease was not released: lease=%#v found=%t err=%v",
			lease, found, err)
	}
}

func TestSpecialistRunnerRecoversExpiredWorkerBeforeFreshTurn(t *testing.T) {
	provider := &specialistTestProvider{responses: []llm.ChatResponse{{
		Text: specialistResponse(t, domain.SpecialistAction{
			Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionContinue,
			Message: "fresh worker resumed safely",
		}), Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	}}}
	st, run, child, runner := newSpecialistRunnerFixture(t, provider,
		domain.Budget{MaxTurns: 10}, 3, 32)
	ctx := context.Background()
	oldLease, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "expired-specialist-worker", TTL: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	root, found, err := st.GetRootAgent(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("root Agent was not found: found=%t err=%v", found, err)
	}
	oldAttemptID := idgen.New("attempt")
	if _, _, err := st.BeginSpecialistAttempt(ctx, domain.AgentAttemptStart{
		AttemptID: oldAttemptID, RunID: run.ID, AgentID: child.ID,
		ParentAgentID: root.ID, Lease: oldLease.Lease, StartedAt: time.Now().UTC(),
	}, "expired-specialist-start-0001"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(oldLease.Lease.ExpiresAt) + 30*time.Millisecond)
	result, err := runner.Step(ctx, run.ID, child.ID)
	if err != nil || result.RecoveredAttempts != 1 || result.Turn != 2 ||
		result.AttemptStatus != domain.AgentAttemptContinued {
		t.Fatalf("fresh worker did not recover child: result=%#v err=%v", result, err)
	}
	oldAttempt, found, err := st.GetAgentAttempt(ctx, oldAttemptID)
	if err != nil || !found || oldAttempt.Status != domain.AgentAttemptCrashed ||
		oldAttempt.Failure.Code != "worker_lost" {
		t.Fatalf("expired attempt was not fenced: attempt=%#v found=%t err=%v",
			oldAttempt, found, err)
	}
}

func newSpecialistRunnerFixture(t testing.TB, provider llm.Provider, budget domain.Budget,
	turnLimit int64, tokenLimit int64,
) (*store.SQLiteStore, domain.Run, domain.AgentNode, *application.SpecialistRunner) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "perform a bounded Specialist review", Profile: "code",
		ModelRoute: provider.Name() + "/model", Budget: budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = service.Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, found, err := st.GetRootAgent(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("root Agent was not created: found=%t err=%v", found, err)
	}
	coord, err := coordinator.NewWithSpecialistAdmission(st,
		coordinator.SpecialistAdmissionPolicy{
			MaxChildren: 1, MaxTurnsPerChild: turnLimit, MaxTokensPerChild: tokenLimit,
		})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := coord.AdmitSpecialist(ctx, coordinator.AdmitSpecialistRequest{
		RunID: run.ID, ParentAgentID: root.ID, Title: "internal model Specialist",
		Skills: []string{"model.chat"}, TurnLimit: turnLimit, TokenLimit: tokenLimit,
		IdempotencyKey: "specialist-runner-admit-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	runner := application.NewSpecialistRunner(st, router, policy.NewDefaultChecker())
	return st, run, admitted.Agent, runner
}

type specialistTestProvider struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	failures  []error
	requests  []llm.ChatRequest
	calls     int
	block     bool
	started   chan struct{}
	startOnce sync.Once
}

func (*specialistTestProvider) Name() string { return "specialist-test" }

func (*specialistTestProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "specialist-test"}}, nil
}

func (p *specialistTestProvider) Chat(ctx context.Context,
	request llm.ChatRequest,
) (*llm.ChatResponse, error) {
	p.mu.Lock()
	index := p.calls
	p.calls++
	p.requests = append(p.requests, request)
	block := p.block
	p.mu.Unlock()
	if block {
		p.startOnce.Do(func() { close(p.started) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if index < len(p.failures) && p.failures[index] != nil {
		return nil, p.failures[index]
	}
	if index >= len(p.responses) {
		return nil, errors.New("Specialist test response exhausted")
	}
	response := p.responses[index]
	response.Provider = p.Name()
	response.Model = "model"
	return &response, nil
}

func (p *specialistTestProvider) StreamChat(ctx context.Context,
	request llm.ChatRequest,
) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, request)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 2)
	chunks <- llm.ChatChunk{Text: response.Text}
	chunks <- llm.FinalChatChunk(response)
	close(chunks)
	return chunks, nil
}

func (*specialistTestProvider) SupportsTools(string) bool    { return false }
func (*specialistTestProvider) SupportsVision(string) bool   { return false }
func (*specialistTestProvider) SupportsJSONMode(string) bool { return true }

func specialistResponse(t testing.TB, action domain.SpecialistAction) string {
	t.Helper()
	encoded, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func assertSpecialistEventCounts(t testing.TB, st *store.SQLiteStore, runID string,
	want map[string]int,
) {
	t.Helper()
	timeline, err := st.ListRunEvents(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for _, event := range timeline {
		counts[event.Type]++
	}
	for eventType, expected := range want {
		if counts[eventType] != expected {
			t.Fatalf("event %s count=%d want=%d timeline=%#v",
				eventType, counts[eventType], expected, timeline)
		}
	}
}
