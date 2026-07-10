package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	CompleteSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, input string, response llm.ChatResponse, decision policy.Decision) (domain.SupervisorCheckpoint, error)
	FailSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, cause string) (domain.SupervisorCheckpoint, error)
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
	Checkpoint domain.SupervisorCheckpoint
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
		failure := s.recordFailure(ctx, &result, err)
		return result, failure
	}
	input := supervisorTurnInput(turn.Mission.Goal, turn.Checkpoint.NextTurn)
	request := llm.ChatRequest{
		Messages: supervisorMessages(history, input),
		Metadata: map[string]string{
			"run_id": turn.Run.ID, "mission_id": turn.Mission.ID, "session_id": turn.Run.SessionID,
			"turn": fmt.Sprint(turn.Checkpoint.NextTurn), "attempt_id": turn.Checkpoint.AttemptID,
		},
	}
	response, err := supervisorChat(ctx, s.router, turn.Run.Config.ModelRoute, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, apperror.Normalize(err)
		}
		failure := s.recordFailure(ctx, &result, err)
		return result, failure
	}
	if err := ctx.Err(); err != nil {
		return result, apperror.Normalize(err)
	}
	if len(response.ToolCalls) > 0 {
		err := apperror.New(apperror.CodeFailedPrecondition, "tool calls are disabled in the P2 supervisor foundation")
		failure := s.recordFailure(ctx, &result, err)
		return result, failure
	}
	decision := s.checker.CheckText("supervisor_assistant_response", response.Text)
	if !decision.Allowed {
		err := apperror.New(apperror.CodePolicyDenied, "policy denied supervisor response: "+decision.Reason)
		failure := s.recordFailure(ctx, &result, err)
		return result, failure
	}
	safeResponse := *response
	safeResponse.Text = redact.String(response.Text)
	checkpoint, err := s.store.CompleteSupervisorTurn(ctx, turn.Checkpoint, input, safeResponse, decision)
	if err != nil {
		return result, apperror.Normalize(err)
	}
	result.Status = LifecycleTurnCompleted
	result.Text = safeResponse.Text
	result.Provider = response.Provider
	result.Model = response.Model
	result.Usage = response.Usage
	result.Checkpoint = checkpoint
	return result, nil
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

func (s *RunSupervisor) recordFailure(ctx context.Context, result *LifecycleResult, cause error) error {
	classified := apperror.Normalize(cause)
	safeCause := apperror.Wrap(apperror.CodeOf(classified), redact.String(classified.Error()), classified)
	checkpoint, err := s.store.FailSupervisorTurn(ctx, result.Checkpoint, safeCause.Error())
	if err != nil {
		return errors.Join(safeCause, err)
	}
	result.Checkpoint = checkpoint
	return safeCause
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
		Role: "system", Content: "You are the CyberAgent Workbench root agent. Produce one safe, concise planning turn. Do not call tools in this supervisor foundation.",
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
