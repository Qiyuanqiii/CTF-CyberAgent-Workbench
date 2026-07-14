package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
)

type OperatorSteeringStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	EnqueueOperatorSteering(ctx context.Context,
		request domain.EnqueueOperatorSteeringRequest) (domain.OperatorSteeringEnqueueResult, error)
	EnqueueOperatorSteeringIfBusy(ctx context.Context,
		request domain.EnqueueOperatorSteeringRequest) (domain.OperatorSteeringEnqueueResult, bool, error)
	CancelOperatorSteering(ctx context.Context,
		request domain.CancelOperatorSteeringRequest) (domain.OperatorSteeringCancellationResult, error)
	GetOperatorSteering(ctx context.Context, id string) (domain.OperatorSteeringMessage, error)
	ListOperatorSteering(ctx context.Context, runID string,
		limit int) ([]domain.OperatorSteeringMessage, error)
	GetOperatorSteeringQueueSummary(ctx context.Context,
		runID string) (domain.OperatorSteeringQueueSummary, error)
}

type QueueOperatorSteeringRequest struct {
	RunID        string
	Content      string
	OperationKey string
	RequestedBy  string
}

type CancelQueuedOperatorSteeringRequest struct {
	MessageID    string
	OperationKey string
	RequestedBy  string
	Reason       string
}

type OperatorSteeringService struct {
	store OperatorSteeringStore
}

type DrainOperatorSteeringRequest struct {
	RunID    string
	MaxSteps int
}

type DrainOperatorSteeringResult struct {
	RunID     string
	Woke      bool
	Before    domain.OperatorSteeringQueueSummary
	After     domain.OperatorSteeringQueueSummary
	Execution ExecutionResult
}

type OperatorSteeringDrainService struct {
	store      SessionRunStore
	runs       *RunService
	supervisor *RunSupervisor
}

func NewOperatorSteeringDrainService(store SessionRunStore, router *llm.Router,
	checker policy.Checker,
) *OperatorSteeringDrainService {
	return &OperatorSteeringDrainService{
		store: store, runs: NewRunService(store),
		supervisor: NewRunSupervisor(store, router, checker),
	}
}

func (s *OperatorSteeringDrainService) WithActiveCalls(
	registry *ActiveCallRegistry,
) *OperatorSteeringDrainService {
	if s != nil && s.supervisor != nil {
		s.supervisor.WithActiveCalls(registry)
	}
	return s
}

func (s *OperatorSteeringDrainService) Drain(ctx context.Context,
	request DrainOperatorSteeringRequest,
) (DrainOperatorSteeringResult, error) {
	result := DrainOperatorSteeringResult{RunID: strings.TrimSpace(request.RunID)}
	if s == nil || s.store == nil || s.runs == nil || s.supervisor == nil {
		return result, apperror.New(apperror.CodeFailedPrecondition,
			"operator steering drain dependencies are required")
	}
	if request.MaxSteps <= 0 || request.MaxSteps > domain.MaxPendingOperatorSteering {
		return result, apperror.New(apperror.CodeInvalidArgument,
			"operator steering drain steps are invalid")
	}
	run, err := s.store.GetRun(ctx, result.RunID)
	if err != nil {
		return result, apperror.Normalize(err)
	}
	result.Before, err = s.store.GetOperatorSteeringQueueSummary(ctx, run.ID)
	if err != nil {
		return result, apperror.Normalize(err)
	}
	if result.Before.Pending+result.Before.Prepared == 0 {
		result.Execution = ExecutionResult{RunID: run.ID, RunStatus: run.Status,
			Steps: make([]LifecycleResult, 0), StopReason: "queue_empty"}
		result.After = result.Before
		return result, nil
	}
	switch run.Status {
	case domain.RunPaused:
	case domain.RunRunning:
	default:
		return result, apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("run %s cannot drain operator steering while %s", run.ID, run.Status))
	}
	result.Execution = ExecutionResult{
		RunID: run.ID, RunStatus: run.Status, Steps: make([]LifecycleResult, 0),
	}
	err = s.supervisor.withRunExecutionLease(ctx, run.ID, func(leaseCtx context.Context,
		lease domain.RunExecutionLease,
	) error {
		if run.Status == domain.RunPaused {
			resumed, resumeErr := s.runs.Resume(leaseCtx, run.ID)
			if resumeErr != nil {
				return apperror.Normalize(resumeErr)
			}
			result.Woke = true
			result.Execution.RunStatus = resumed.Status
		}
		return s.supervisor.drainOperatorSteeringWithLease(leaseCtx, lease,
			request.MaxSteps, &result.Execution)
	})
	drainErr := apperror.Normalize(err)
	result.After, err = s.store.GetOperatorSteeringQueueSummary(ctx, run.ID)
	if err != nil {
		if drainErr != nil {
			return result, drainErr
		}
		return result, apperror.Normalize(err)
	}
	return result, drainErr
}

func NewOperatorSteeringService(store OperatorSteeringStore) *OperatorSteeringService {
	return &OperatorSteeringService{store: store}
}

func (s *OperatorSteeringService) Enqueue(ctx context.Context,
	request QueueOperatorSteeringRequest,
) (domain.OperatorSteeringEnqueueResult, error) {
	if s == nil || s.store == nil {
		return domain.OperatorSteeringEnqueueResult{},
			apperror.New(apperror.CodeFailedPrecondition,
				"operator steering store is required")
	}
	run, err := s.store.GetRun(ctx, strings.TrimSpace(request.RunID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.OperatorSteeringEnqueueResult{},
				apperror.New(apperror.CodeNotFound, "operator steering Run was not found")
		}
		return domain.OperatorSteeringEnqueueResult{}, apperror.Normalize(err)
	}
	result, err := s.store.EnqueueOperatorSteering(ctx,
		domain.EnqueueOperatorSteeringRequest{
			RunID: run.ID, SessionID: run.SessionID, Content: request.Content,
			OperationKey: request.OperationKey, RequestedBy: request.RequestedBy,
		})
	return result, apperror.Normalize(err)
}

func (s *OperatorSteeringService) Cancel(ctx context.Context,
	request CancelQueuedOperatorSteeringRequest,
) (domain.OperatorSteeringCancellationResult, error) {
	if s == nil || s.store == nil {
		return domain.OperatorSteeringCancellationResult{},
			apperror.New(apperror.CodeFailedPrecondition,
				"operator steering store is required")
	}
	result, err := s.store.CancelOperatorSteering(ctx,
		domain.CancelOperatorSteeringRequest{
			MessageID: request.MessageID, OperationKey: request.OperationKey,
			RequestedBy: request.RequestedBy, Reason: request.Reason,
		})
	return result, apperror.Normalize(err)
}

func (s *OperatorSteeringService) Get(ctx context.Context,
	id string,
) (domain.OperatorSteeringMessage, error) {
	if s == nil || s.store == nil {
		return domain.OperatorSteeringMessage{},
			apperror.New(apperror.CodeFailedPrecondition,
				"operator steering store is required")
	}
	value, err := s.store.GetOperatorSteering(ctx, id)
	return value, apperror.Normalize(err)
}

func (s *OperatorSteeringService) List(ctx context.Context, runID string,
	limit int,
) ([]domain.OperatorSteeringMessage, domain.OperatorSteeringQueueSummary, error) {
	if s == nil || s.store == nil {
		return nil, domain.OperatorSteeringQueueSummary{},
			apperror.New(apperror.CodeFailedPrecondition,
				"operator steering store is required")
	}
	run, err := s.store.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.OperatorSteeringQueueSummary{},
				apperror.New(apperror.CodeNotFound, "operator steering Run was not found")
		}
		return nil, domain.OperatorSteeringQueueSummary{}, apperror.Normalize(err)
	}
	values, err := s.store.ListOperatorSteering(ctx, run.ID, limit)
	if err != nil {
		return nil, domain.OperatorSteeringQueueSummary{}, apperror.Normalize(err)
	}
	summary, err := s.store.GetOperatorSteeringQueueSummary(ctx, run.ID)
	return values, summary, apperror.Normalize(err)
}
