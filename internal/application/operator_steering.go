package application

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

type OperatorSteeringStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	EnqueueOperatorSteering(ctx context.Context,
		request domain.EnqueueOperatorSteeringRequest) (domain.OperatorSteeringEnqueueResult, error)
	EnqueueOperatorSteeringIfBusy(ctx context.Context,
		request domain.EnqueueOperatorSteeringRequest) (domain.OperatorSteeringEnqueueResult, bool, error)
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

type OperatorSteeringService struct {
	store OperatorSteeringStore
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
