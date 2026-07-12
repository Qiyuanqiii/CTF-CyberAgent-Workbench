package coordinator

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
)

type FinishSpecialistRequest struct {
	RunID          string
	AgentID        string
	ParentAgentID  string
	AttemptID      string
	Report         domain.CompletionReport
	IdempotencyKey string
}

type FinishSpecialistResult struct {
	Completion domain.AgentCompletion
	Replayed   bool
}

func (c *Coordinator) FinishSpecialist(ctx context.Context,
	req FinishSpecialistRequest,
) (FinishSpecialistResult, error) {
	if c == nil || c.store == nil {
		return FinishSpecialistResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"agent coordinator store is required")
	}
	if !c.specialistRuntimeEnabled {
		return FinishSpecialistResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"specialist runtime is disabled")
	}
	operationKey, err := domain.NormalizeAgentOperationKey(req.IdempotencyKey)
	if err != nil {
		return FinishSpecialistResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Agent completion idempotency key is invalid", err)
	}
	report, err := domain.NormalizeCompletionReport(req.Report)
	if err != nil {
		return FinishSpecialistResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"completion report is invalid", err)
	}
	completion := domain.AgentCompletion{
		ID: idgen.New("completion"), RunID: strings.TrimSpace(req.RunID),
		AgentID: strings.TrimSpace(req.AgentID), ParentAgentID: strings.TrimSpace(req.ParentAgentID),
		AttemptID: strings.TrimSpace(req.AttemptID), Report: report,
		MessageID: idgen.New("agentmsg"), CreatedAt: time.Now().UTC(),
	}
	stored, replayed, err := c.store.FinishSpecialist(ctx, completion, operationKey)
	return FinishSpecialistResult{Completion: stored, Replayed: replayed}, apperror.Normalize(err)
}

func (c *Coordinator) Completion(ctx context.Context,
	agentID string,
) (domain.AgentCompletion, bool, error) {
	if c == nil || c.store == nil {
		return domain.AgentCompletion{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"agent coordinator store is required")
	}
	completion, found, err := c.store.GetAgentCompletion(ctx, strings.TrimSpace(agentID))
	return completion, found, apperror.Normalize(err)
}
