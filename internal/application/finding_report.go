package application

import (
	"context"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

type FindingReportStore interface {
	EnsureReadOnlyFanoutFindingReport(ctx context.Context,
		executionID string) (domain.FindingReport, bool, error)
	GetFindingReport(ctx context.Context, id string) (domain.FindingReport, error)
}

type FindingReportService struct {
	store FindingReportStore
}

func NewFindingReportService(store FindingReportStore) *FindingReportService {
	return &FindingReportService{store: store}
}

func (s *FindingReportService) GenerateReadOnlyFanout(ctx context.Context,
	executionID string,
) (domain.FindingReport, bool, error) {
	if s == nil || s.store == nil {
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	executionID = strings.TrimSpace(executionID)
	if !domain.ValidAgentID(executionID) {
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeInvalidArgument, "read-only fan-out execution id is invalid")
	}
	report, replayed, err := s.store.EnsureReadOnlyFanoutFindingReport(ctx, executionID)
	return report, replayed, apperror.Normalize(err)
}

func (s *FindingReportService) Get(ctx context.Context,
	id string,
) (domain.FindingReport, error) {
	if s == nil || s.store == nil {
		return domain.FindingReport{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) {
		return domain.FindingReport{}, apperror.New(
			apperror.CodeInvalidArgument, "finding report id is invalid")
	}
	value, err := s.store.GetFindingReport(ctx, id)
	return value, apperror.Normalize(err)
}
