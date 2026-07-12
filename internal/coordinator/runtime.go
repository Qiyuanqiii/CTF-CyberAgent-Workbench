package coordinator

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
)

type StartSpecialistAttemptRequest struct {
	RunID          string
	AgentID        string
	ParentAgentID  string
	Lease          domain.RunExecutionLease
	IdempotencyKey string
}

type SpecialistAttemptRequest struct {
	RunID          string
	AgentID        string
	AttemptID      string
	IdempotencyKey string
}

type RecordSpecialistUsageRequest struct {
	SpecialistAttemptRequest
	Usage domain.AgentAttemptUsage
}

type CrashSpecialistAttemptRequest struct {
	SpecialistAttemptRequest
	Failure domain.AgentAttemptFailure
}

type SpecialistAttemptResult struct {
	Attempt  domain.AgentAttempt
	Replayed bool
}

func NewWithSpecialistRuntime(store Store) *Coordinator {
	coordinator := New(store)
	coordinator.specialistRuntimeEnabled = true
	return coordinator
}

func (c *Coordinator) StartSpecialistAttempt(ctx context.Context,
	req StartSpecialistAttemptRequest,
) (SpecialistAttemptResult, error) {
	if err := c.requireSpecialistRuntime(); err != nil {
		return SpecialistAttemptResult{}, err
	}
	operationKey, err := normalizeRuntimeOperationKey(req.IdempotencyKey)
	if err != nil {
		return SpecialistAttemptResult{}, err
	}
	attempt, replayed, err := c.store.BeginSpecialistAttempt(ctx, domain.AgentAttemptStart{
		AttemptID: idgen.New("attempt"), RunID: strings.TrimSpace(req.RunID),
		AgentID: strings.TrimSpace(req.AgentID), ParentAgentID: strings.TrimSpace(req.ParentAgentID),
		Lease: req.Lease, StartedAt: time.Now().UTC(),
	}, operationKey)
	return SpecialistAttemptResult{Attempt: attempt, Replayed: replayed}, apperror.Normalize(err)
}

func (c *Coordinator) RecordSpecialistUsage(ctx context.Context,
	req RecordSpecialistUsageRequest,
) (SpecialistAttemptResult, error) {
	if err := c.requireSpecialistRuntime(); err != nil {
		return SpecialistAttemptResult{}, err
	}
	operationKey, err := normalizeRuntimeOperationKey(req.IdempotencyKey)
	if err != nil {
		return SpecialistAttemptResult{}, err
	}
	attempt, replayed, err := c.store.RecordSpecialistAttemptUsage(ctx,
		runtimeAttemptRef(req.SpecialistAttemptRequest), req.Usage, operationKey)
	return SpecialistAttemptResult{Attempt: attempt, Replayed: replayed}, apperror.Normalize(err)
}

func (c *Coordinator) ContinueSpecialistAttempt(ctx context.Context,
	req SpecialistAttemptRequest,
) (SpecialistAttemptResult, error) {
	if err := c.requireSpecialistRuntime(); err != nil {
		return SpecialistAttemptResult{}, err
	}
	operationKey, err := normalizeRuntimeOperationKey(req.IdempotencyKey)
	if err != nil {
		return SpecialistAttemptResult{}, err
	}
	attempt, replayed, err := c.store.ContinueSpecialistAttempt(ctx, runtimeAttemptRef(req),
		operationKey)
	return SpecialistAttemptResult{Attempt: attempt, Replayed: replayed}, apperror.Normalize(err)
}

func (c *Coordinator) CrashSpecialistAttempt(ctx context.Context,
	req CrashSpecialistAttemptRequest,
) (SpecialistAttemptResult, error) {
	if err := c.requireSpecialistRuntime(); err != nil {
		return SpecialistAttemptResult{}, err
	}
	operationKey, err := normalizeRuntimeOperationKey(req.IdempotencyKey)
	if err != nil {
		return SpecialistAttemptResult{}, err
	}
	attempt, replayed, err := c.store.CrashSpecialistAttempt(ctx,
		domain.AgentAttemptFailureRequest{
			Ref: runtimeAttemptRef(req.SpecialistAttemptRequest), Failure: req.Failure,
			NotificationMessageID: idgen.New("agentmsg"), FailedAt: time.Now().UTC(),
		}, operationKey)
	return SpecialistAttemptResult{Attempt: attempt, Replayed: replayed}, apperror.Normalize(err)
}

func (c *Coordinator) RecoverSpecialistAttempts(ctx context.Context,
	lease domain.RunExecutionLease,
) ([]domain.AgentAttempt, error) {
	if err := c.requireSpecialistRuntime(); err != nil {
		return nil, err
	}
	attempts, err := c.store.RecoverSpecialistAttempts(ctx, lease)
	return attempts, apperror.Normalize(err)
}

func (c *Coordinator) SpecialistAttempt(ctx context.Context,
	attemptID string,
) (domain.AgentAttempt, bool, error) {
	if err := c.requireSpecialistRuntime(); err != nil {
		return domain.AgentAttempt{}, false, err
	}
	attempt, found, err := c.store.GetAgentAttempt(ctx, strings.TrimSpace(attemptID))
	return attempt, found, apperror.Normalize(err)
}

func (c *Coordinator) SpecialistAttempts(ctx context.Context,
	agentID string,
) ([]domain.AgentAttempt, error) {
	if err := c.requireSpecialistRuntime(); err != nil {
		return nil, err
	}
	attempts, err := c.store.ListAgentAttempts(ctx, strings.TrimSpace(agentID))
	return attempts, apperror.Normalize(err)
}

func (c *Coordinator) requireSpecialistRuntime() error {
	if c == nil || c.store == nil {
		return apperror.New(apperror.CodeFailedPrecondition, "agent coordinator store is required")
	}
	if !c.specialistRuntimeEnabled {
		return apperror.New(apperror.CodeFailedPrecondition, "specialist runtime is disabled")
	}
	return nil
}

func runtimeAttemptRef(req SpecialistAttemptRequest) domain.AgentAttemptRef {
	return domain.AgentAttemptRef{
		RunID: strings.TrimSpace(req.RunID), AgentID: strings.TrimSpace(req.AgentID),
		AttemptID: strings.TrimSpace(req.AttemptID),
	}
}

func normalizeRuntimeOperationKey(key string) (string, error) {
	normalized, err := domain.NormalizeAgentOperationKey(key)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInvalidArgument,
			"Specialist attempt idempotency key is invalid", err)
	}
	return normalized, nil
}
