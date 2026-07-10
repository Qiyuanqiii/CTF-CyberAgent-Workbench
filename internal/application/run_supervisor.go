package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
)

const maxSupervisorHistoryMessages = 20

type SupervisorStore interface {
	BeginSupervisorTurn(ctx context.Context, runID string) (domain.SupervisorTurn, error)
	CompleteSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, input string, response llm.ChatResponse, action domain.RootAction, decision policy.Decision, elapsed time.Duration) (domain.Run, domain.SupervisorCheckpoint, error)
	FailSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, cause string, elapsed time.Duration) (domain.SupervisorCheckpoint, error)
	FinalizeSupervisorRun(ctx context.Context, runID string, target domain.RunStatus, summary string) (domain.Run, domain.SupervisorCheckpoint, error)
	GetSupervisorCheckpoint(ctx context.Context, runID string) (domain.SupervisorCheckpoint, bool, error)
	GetRun(ctx context.Context, id string) (domain.Run, error)
	ListSessionMessages(ctx context.Context, sessionID string, includeCompacted bool) ([]session.Message, error)
}

type RunHandle struct {
	RunID     string
	MissionID string
	SessionID string
}

type LifecycleStatus string

const (
	LifecycleTurnCompleted LifecycleStatus = "turn_completed"
	LifecycleTurnFailed    LifecycleStatus = "turn_failed"
)

type LifecycleResult struct {
	Handle     RunHandle
	Status     LifecycleStatus
	Turn       int
	AttemptID  string
	Recovered  bool
	Text       string
	Provider   string
	Model      string
	Usage      llm.Usage
	Action     domain.RootAction
	RunStatus  domain.RunStatus
	Checkpoint domain.SupervisorCheckpoint
}

type LifecycleOutcome string

const (
	LifecycleOutcomeCompleted LifecycleOutcome = "completed"
	LifecycleOutcomeFailed    LifecycleOutcome = "failed"
)

type FinalizationResult struct {
	Run        domain.Run
	Checkpoint domain.SupervisorCheckpoint
	Outcome    LifecycleOutcome
	Summary    string
}

type ExecutionResult struct {
	RunID      string
	Steps      []LifecycleResult
	StopReason string
	RunStatus  domain.RunStatus
}

type RunSupervisor struct {
	store   SupervisorStore
	router  *llm.Router
	checker policy.Checker
}

func NewRunSupervisor(store SupervisorStore, router *llm.Router, checker policy.Checker) *RunSupervisor {
	return &RunSupervisor{store: store, router: router, checker: checker}
}

func (s *RunSupervisor) Step(ctx context.Context, runID string) (LifecycleResult, error) {
	if s == nil || s.store == nil || s.router == nil || s.checker == nil {
		return LifecycleResult{}, apperror.New(apperror.CodeFailedPrecondition, "run supervisor dependencies are required")
	}
	turn, err := s.store.BeginSupervisorTurn(ctx, runID)
	if err != nil {
		return LifecycleResult{}, apperror.Normalize(err)
	}
	result := LifecycleResult{
		Handle: RunHandle{RunID: turn.Run.ID, MissionID: turn.Mission.ID, SessionID: turn.Run.SessionID},
		Status: LifecycleTurnFailed, Turn: turn.Checkpoint.NextTurn, AttemptID: turn.Checkpoint.AttemptID,
		Recovered: turn.Recovered, Checkpoint: turn.Checkpoint,
	}
	if err := ctx.Err(); err != nil {
		return result, apperror.Normalize(err)
	}
	history, err := s.store.ListSessionMessages(ctx, turn.Run.SessionID, false)
	if err != nil {
		failure := s.recordFailure(ctx, &result, err, 0)
		return result, failure
	}
	input := supervisorTurnInput(turn.Mission.Goal, turn.Checkpoint.NextTurn)
	request := llm.ChatRequest{
		Messages: supervisorMessages(history, input),
		JSONMode: true,
		Metadata: map[string]string{
			"run_id": turn.Run.ID, "mission_id": turn.Mission.ID, "session_id": turn.Run.SessionID,
			"turn": fmt.Sprint(turn.Checkpoint.NextTurn), "attempt_id": turn.Checkpoint.AttemptID,
			"response_schema": domain.RootLifecycleVersion,
		},
	}
	if turn.Run.Budget.MaxTokens > 0 {
		remaining := turn.Run.Budget.MaxTokens - turn.Checkpoint.TotalTokens
		maxInt := int64(int(^uint(0) >> 1))
		if remaining > maxInt {
			remaining = maxInt
		}
		request.MaxTokens = int(remaining)
	}
	callCtx, cancel := supervisorModelContext(ctx, turn.Run.Budget, turn.Checkpoint)
	startedAt := time.Now()
	response, err := supervisorChat(callCtx, s.router, turn.Run.Config.ModelRoute, request)
	elapsed := time.Since(startedAt)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return result, apperror.Normalize(err)
		}
		failure := s.recordFailure(ctx, &result, err, elapsed)
		return result, failure
	}
	if response == nil {
		err := apperror.New(apperror.CodeFailedPrecondition, "provider returned an empty response")
		failure := s.recordFailure(ctx, &result, err, elapsed)
		return result, failure
	}
	if err := ctx.Err(); err != nil {
		return result, apperror.Normalize(err)
	}
	if len(response.ToolCalls) > 0 {
		err := apperror.New(apperror.CodeFailedPrecondition, "tool calls are disabled in the P2 supervisor foundation")
		failure := s.recordFailure(ctx, &result, err, elapsed)
		return result, failure
	}
	if response.Usage.InputTokens < 0 || response.Usage.OutputTokens < 0 || response.Usage.TotalTokens < 0 {
		err := apperror.New(apperror.CodeFailedPrecondition, "provider returned negative token usage")
		failure := s.recordFailure(ctx, &result, err, elapsed)
		return result, failure
	}
	action, err := parseRootAction(response.Text)
	if err != nil {
		failure := s.recordFailure(ctx, &result, err, elapsed)
		return result, failure
	}
	decision := s.checker.CheckText("supervisor_assistant_response", rootActionPolicyText(action))
	if !decision.Allowed {
		err := apperror.New(apperror.CodePolicyDenied, "policy denied supervisor response: "+decision.Reason)
		failure := s.recordFailure(ctx, &result, err, elapsed)
		return result, failure
	}
	safeAction := redactRootAction(action)
	safeResponse := *response
	safeResponse.Text = safeAction.Message
	updatedRun, checkpoint, err := s.store.CompleteSupervisorTurn(ctx, turn.Checkpoint, input, safeResponse, safeAction, decision, elapsed)
	if err != nil {
		return result, apperror.Normalize(err)
	}
	result.Status = LifecycleTurnCompleted
	result.Text = safeAction.Message
	result.Provider = response.Provider
	result.Model = response.Model
	result.Usage = response.Usage
	result.Action = safeAction
	result.RunStatus = updatedRun.Status
	result.Checkpoint = checkpoint
	return result, nil
}

func (s *RunSupervisor) Execute(ctx context.Context, runID string, maxSteps int) (ExecutionResult, error) {
	if s == nil || s.store == nil || s.router == nil || s.checker == nil {
		return ExecutionResult{}, apperror.New(apperror.CodeFailedPrecondition, "run supervisor dependencies are required")
	}
	if maxSteps <= 0 {
		return ExecutionResult{}, apperror.New(apperror.CodeInvalidArgument, "max steps must be positive")
	}
	result := ExecutionResult{RunID: strings.TrimSpace(runID), Steps: make([]LifecycleResult, 0)}
	for range maxSteps {
		run, err := s.store.GetRun(ctx, result.RunID)
		if err != nil {
			return result, apperror.Normalize(err)
		}
		result.RunStatus = run.Status
		if run.Terminal() {
			result.StopReason = "run_terminal"
			return result, nil
		}
		if run.Status == domain.RunPaused {
			result.StopReason = "run_paused"
			return result, nil
		}
		if run.Status == domain.RunWaitingApproval {
			result.StopReason = "waiting_approval"
			return result, nil
		}
		step, err := s.Step(ctx, result.RunID)
		if step.Turn > 0 {
			result.Steps = append(result.Steps, step)
		}
		if err != nil {
			result.StopReason = strings.ToLower(string(apperror.CodeOf(err)))
			return result, err
		}
		result.RunStatus = step.RunStatus
		switch step.Action.Kind {
		case domain.RootActionFinish:
			result.StopReason = "root_finish"
			return result, nil
		case domain.RootActionWait:
			result.StopReason = "root_wait"
			return result, nil
		}
	}
	run, err := s.store.GetRun(ctx, result.RunID)
	if err != nil {
		return result, apperror.Normalize(err)
	}
	result.RunStatus = run.Status
	result.StopReason = "step_limit"
	return result, nil
}

func (s *RunSupervisor) Finalize(ctx context.Context, runID string, outcome LifecycleOutcome, summary string) (FinalizationResult, error) {
	if s == nil || s.store == nil {
		return FinalizationResult{}, apperror.New(apperror.CodeFailedPrecondition, "run supervisor store is required")
	}
	var target domain.RunStatus
	switch outcome {
	case LifecycleOutcomeCompleted:
		target = domain.RunCompleted
	case LifecycleOutcomeFailed:
		target = domain.RunFailed
	default:
		return FinalizationResult{}, apperror.New(apperror.CodeInvalidArgument, "lifecycle outcome must be completed or failed")
	}
	run, checkpoint, err := s.store.FinalizeSupervisorRun(ctx, strings.TrimSpace(runID), target, summary)
	if err != nil {
		return FinalizationResult{}, apperror.Normalize(err)
	}
	return FinalizationResult{Run: run, Checkpoint: checkpoint, Outcome: outcome, Summary: redact.String(strings.TrimSpace(summary))}, nil
}

func (s *RunSupervisor) Checkpoint(ctx context.Context, runID string) (domain.SupervisorCheckpoint, bool, error) {
	if s == nil || s.store == nil {
		return domain.SupervisorCheckpoint{}, false, apperror.New(apperror.CodeFailedPrecondition, "run supervisor store is required")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return domain.SupervisorCheckpoint{}, false, apperror.New(apperror.CodeInvalidArgument, "run id is required")
	}
	checkpoint, ok, err := s.store.GetSupervisorCheckpoint(ctx, runID)
	if err != nil || ok {
		return checkpoint, ok, apperror.Normalize(err)
	}
	if _, err := s.store.GetRun(ctx, runID); err != nil {
		return domain.SupervisorCheckpoint{}, false, apperror.Normalize(err)
	}
	return domain.SupervisorCheckpoint{}, false, nil
}

func (s *RunSupervisor) recordFailure(ctx context.Context, result *LifecycleResult, cause error, elapsed time.Duration) error {
	classified := apperror.Normalize(cause)
	safeCause := apperror.Wrap(apperror.CodeOf(classified), redact.String(classified.Error()), classified)
	checkpoint, err := s.store.FailSupervisorTurn(ctx, result.Checkpoint, safeCause.Error(), elapsed)
	if err != nil {
		return errors.Join(safeCause, err)
	}
	result.Checkpoint = checkpoint
	return safeCause
}

func supervisorModelContext(ctx context.Context, budget domain.Budget, checkpoint domain.SupervisorCheckpoint) (context.Context, context.CancelFunc) {
	if budget.TimeoutSeconds <= 0 {
		return context.WithCancel(ctx)
	}
	remainingMillis := budget.TimeoutSeconds*1000 - checkpoint.ExecutionMillis
	if remainingMillis <= 0 {
		remainingMillis = 1
	}
	return context.WithTimeout(ctx, time.Duration(remainingMillis)*time.Millisecond)
}

func supervisorTurnInput(goal string, turn int) string {
	goal = strings.TrimSpace(goal)
	if turn <= 1 {
		return goal
	}
	return fmt.Sprintf("Continue mission at turn %d without executing tools: %s", turn, goal)
}

func supervisorMessages(history []session.Message, input string) []llm.Message {
	if len(history) > maxSupervisorHistoryMessages {
		history = history[len(history)-maxSupervisorHistoryMessages:]
	}
	messages := make([]llm.Message, 0, len(history)+2)
	messages = append(messages, llm.Message{
		Role: "system", Content: `You are the CyberAgent Workbench root agent. Do not call tools in this supervisor foundation. Return exactly one JSON object and no markdown using this schema: {"version":"root_lifecycle.v1","action":"continue|finish|wait","message":"user-facing result","summary":"required only for finish","reason":"required only for wait"}. Use continue when more work remains, finish only when the mission is complete without tool execution, and wait only when external input or a dependency is required.`,
	})
	for _, message := range history {
		if message.Role == "user" || message.Role == "assistant" || message.Role == "system" {
			messages = append(messages, llm.Message{Role: message.Role, Content: message.Content})
		}
	}
	return append(messages, llm.Message{Role: "user", Content: input})
}

func supervisorChat(ctx context.Context, router *llm.Router, route string, request llm.ChatRequest) (*llm.ChatResponse, error) {
	route = strings.TrimSpace(route)
	if strings.Contains(route, "/") {
		ref, err := llm.ParseModelRef(route)
		if err != nil {
			return nil, err
		}
		return router.ChatModelRef(ctx, ref, request)
	}
	return router.Chat(ctx, route, request)
}
