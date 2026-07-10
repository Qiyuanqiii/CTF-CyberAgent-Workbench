package application

import (
	"context"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
)

type SessionRunStore interface {
	RunStore
	SupervisorStore
	GetRunBySession(ctx context.Context, sessionID string) (domain.Run, bool, error)
}

type SessionRunChatExecutor struct {
	store      SessionRunStore
	runs       *RunService
	supervisor *RunSupervisor
}

func NewSessionRunChatExecutor(store SessionRunStore, router *llm.Router, checker policy.Checker) *SessionRunChatExecutor {
	return &SessionRunChatExecutor{
		store: store, runs: NewRunService(store), supervisor: NewRunSupervisor(store, router, checker),
	}
}

func (e *SessionRunChatExecutor) WithActiveCalls(registry *ActiveCallRegistry) *SessionRunChatExecutor {
	if e != nil && e.supervisor != nil {
		e.supervisor.WithActiveCalls(registry)
	}
	return e
}

func (e *SessionRunChatExecutor) ExecuteSessionTurn(ctx context.Context, sess session.Session, input string) (session.RunChatResult, bool, error) {
	if e == nil || e.store == nil || e.runs == nil || e.supervisor == nil {
		return session.RunChatResult{}, false, apperror.New(apperror.CodeFailedPrecondition, "session run chat dependencies are required")
	}
	run, attached, err := e.store.GetRunBySession(ctx, strings.TrimSpace(sess.ID))
	if err != nil || !attached {
		return session.RunChatResult{}, attached, apperror.Normalize(err)
	}
	if run.SessionID != sess.ID {
		return session.RunChatResult{}, true, apperror.New(apperror.CodeConflict, "run and session binding changed")
	}
	switch run.Status {
	case domain.RunCreated, domain.RunPreparing:
		run, err = e.runs.Start(ctx, run.ID)
	case domain.RunPaused:
		run, err = e.runs.Resume(ctx, run.ID)
	case domain.RunRunning:
	case domain.RunWaitingApproval:
		return session.RunChatResult{}, true, apperror.New(apperror.CodeFailedPrecondition, "run is waiting for approval")
	default:
		return session.RunChatResult{}, true, apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("run %s is terminal as %s", run.ID, run.Status))
	}
	if err != nil {
		return session.RunChatResult{}, true, apperror.Normalize(err)
	}
	result, err := e.supervisor.StepWithInput(ctx, run.ID, input)
	if err != nil {
		return session.RunChatResult{}, true, err
	}
	return session.RunChatResult{
		RunID: run.ID, UserMessage: result.UserMessage, ReplyMessage: result.ReplyMessage,
		Text: result.Text, Action: string(result.Action.Kind), RunStatus: string(result.RunStatus),
	}, true, nil
}
