package application

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
)

type WorkItemStore interface {
	CreateWorkItem(ctx context.Context, item domain.WorkItem, event events.Event) error
	GetWorkItem(ctx context.Context, id string) (domain.WorkItem, error)
	ListWorkItems(ctx context.Context, filter domain.WorkItemFilter) ([]domain.WorkItem, error)
	UpdateWorkItem(ctx context.Context, item domain.WorkItem, expectedVersion int64, event events.Event) error
	GetRun(ctx context.Context, id string) (domain.Run, error)
}

type WorkItemService struct {
	store WorkItemStore
}

type CreateWorkItemRequest struct {
	RunID              string
	Title              string
	Description        string
	Priority           string
	Owner              string
	AcceptanceCriteria []string
	Dependencies       []string
}

type UpdateWorkItemRequest struct {
	ID                 string
	ExpectedVersion    int64
	Title              *string
	Description        *string
	Priority           *string
	Owner              *string
	AcceptanceCriteria *[]string
	Dependencies       *[]string
}

func NewWorkItemService(store WorkItemStore) *WorkItemService {
	return &WorkItemService{store: store}
}

func (s *WorkItemService) Create(ctx context.Context, req CreateWorkItemRequest) (domain.WorkItem, error) {
	if s == nil || s.store == nil {
		return domain.WorkItem{}, apperror.New(apperror.CodeFailedPrecondition, "work item store is required")
	}
	_, item, event, err := s.prepareCreate(ctx, req)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := s.store.CreateWorkItem(ctx, item, event); err != nil {
		return domain.WorkItem{}, apperror.Normalize(err)
	}
	return s.store.GetWorkItem(ctx, item.ID)
}

func (s *WorkItemService) prepareCreate(ctx context.Context, req CreateWorkItemRequest) (domain.Run, domain.WorkItem, events.Event, error) {
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		return domain.Run{}, domain.WorkItem{}, events.Event{}, apperror.New(apperror.CodeInvalidArgument, "work item run id is required")
	}
	run, err := s.mutableRun(ctx, runID)
	if err != nil {
		return domain.Run{}, domain.WorkItem{}, events.Event{}, err
	}
	priority, err := domain.ParseWorkItemPriority(req.Priority)
	if err != nil {
		return domain.Run{}, domain.WorkItem{}, events.Event{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	now := time.Now().UTC()
	item := domain.WorkItem{
		ID: idgen.New("work"), RunID: run.ID, Status: domain.WorkItemPending, Priority: priority, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	details, err := normalizeServiceWorkItemDetails(item.ID, domain.WorkItemDetails{
		Title: req.Title, Description: req.Description, Priority: priority, Owner: req.Owner,
		AcceptanceCriteria: slices.Clone(req.AcceptanceCriteria), Dependencies: slices.Clone(req.Dependencies),
	})
	if err != nil {
		return domain.Run{}, domain.WorkItem{}, events.Event{}, err
	}
	item.Title = details.Title
	item.Description = details.Description
	item.Owner = details.Owner
	item.AcceptanceCriteria = details.AcceptanceCriteria
	item.Dependencies = details.Dependencies
	if err := item.Validate(); err != nil {
		return domain.Run{}, domain.WorkItem{}, events.Event{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	event, err := events.New(run.ID, run.MissionID, events.WorkItemCreatedEvent, "work_item_service", item.ID, map[string]any{
		"title": item.Title, "status": item.Status, "priority": item.Priority, "owner": item.Owner,
		"dependency_ids": item.Dependencies, "acceptance_count": len(item.AcceptanceCriteria), "version": item.Version,
	})
	if err != nil {
		return domain.Run{}, domain.WorkItem{}, events.Event{}, err
	}
	return run, item, event, nil
}

func (s *WorkItemService) Get(ctx context.Context, id string) (domain.WorkItem, error) {
	if s == nil || s.store == nil {
		return domain.WorkItem{}, apperror.New(apperror.CodeFailedPrecondition, "work item store is required")
	}
	item, err := s.store.GetWorkItem(ctx, strings.TrimSpace(id))
	return item, apperror.Normalize(err)
}

func (s *WorkItemService) List(ctx context.Context, filter domain.WorkItemFilter) ([]domain.WorkItem, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "work item store is required")
	}
	items, err := s.store.ListWorkItems(ctx, filter)
	return items, apperror.Normalize(err)
}

func (s *WorkItemService) Update(ctx context.Context, req UpdateWorkItemRequest) (domain.WorkItem, error) {
	if s == nil || s.store == nil {
		return domain.WorkItem{}, apperror.New(apperror.CodeFailedPrecondition, "work item store is required")
	}
	if req.Title == nil && req.Description == nil && req.Priority == nil && req.Owner == nil &&
		req.AcceptanceCriteria == nil && req.Dependencies == nil {
		return domain.WorkItem{}, apperror.New(apperror.CodeInvalidArgument, "work item update requires at least one changed field")
	}
	current, err := s.store.GetWorkItem(ctx, strings.TrimSpace(req.ID))
	if err != nil {
		return domain.WorkItem{}, apperror.Normalize(err)
	}
	run, err := s.mutableRun(ctx, current.RunID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	expectedVersion, err := resolveExpectedWorkItemVersion(current, req.ExpectedVersion)
	if err != nil {
		return domain.WorkItem{}, err
	}
	details := domain.WorkItemDetails{
		Title: current.Title, Description: current.Description, Priority: current.Priority, Owner: current.Owner,
		AcceptanceCriteria: slices.Clone(current.AcceptanceCriteria), Dependencies: slices.Clone(current.Dependencies),
	}
	changed := make([]string, 0, 6)
	if req.Title != nil {
		details.Title = *req.Title
		changed = append(changed, "title")
	}
	if req.Description != nil {
		details.Description = *req.Description
		changed = append(changed, "description")
	}
	if req.Priority != nil {
		priority, err := domain.ParseWorkItemPriority(*req.Priority)
		if err != nil {
			return domain.WorkItem{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
		}
		details.Priority = priority
		changed = append(changed, "priority")
	}
	if req.Owner != nil {
		details.Owner = *req.Owner
		changed = append(changed, "owner")
	}
	if req.AcceptanceCriteria != nil {
		details.AcceptanceCriteria = slices.Clone(*req.AcceptanceCriteria)
		changed = append(changed, "acceptance_criteria")
	}
	if req.Dependencies != nil {
		details.Dependencies = slices.Clone(*req.Dependencies)
		changed = append(changed, "dependencies")
	}
	details, err = normalizeServiceWorkItemDetails(current.ID, details)
	if err != nil {
		return domain.WorkItem{}, err
	}
	updated := current
	if err := updated.ApplyDetails(details, time.Now().UTC()); err != nil {
		return domain.WorkItem{}, apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
	}
	updated.Version = expectedVersion + 1
	event, err := events.New(updated.RunID, run.MissionID, events.WorkItemChangedEvent,
		"work_item_service", updated.ID, map[string]any{
			"status": updated.Status, "changed_fields": changed, "version": updated.Version,
		})
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := s.store.UpdateWorkItem(ctx, updated, expectedVersion, event); err != nil {
		return domain.WorkItem{}, apperror.Normalize(err)
	}
	return s.store.GetWorkItem(ctx, updated.ID)
}

func (s *WorkItemService) Transition(ctx context.Context, id string, expectedVersion int64, target domain.WorkItemStatus, reason string) (domain.WorkItem, error) {
	if s == nil || s.store == nil {
		return domain.WorkItem{}, apperror.New(apperror.CodeFailedPrecondition, "work item store is required")
	}
	current, err := s.store.GetWorkItem(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.WorkItem{}, apperror.Normalize(err)
	}
	run, err := s.mutableRun(ctx, current.RunID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	expectedVersion, err = resolveExpectedWorkItemVersion(current, expectedVersion)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if current.Status == target {
		return current, nil
	}
	updated := current
	if err := updated.Transition(target, redact.String(strings.TrimSpace(reason)), time.Now().UTC()); err != nil {
		return domain.WorkItem{}, apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
	}
	updated.Version = expectedVersion + 1
	event, err := events.New(updated.RunID, run.MissionID, events.WorkItemChangedEvent, "work_item_service", updated.ID, map[string]any{
		"from": current.Status, "to": updated.Status, "reason": updated.BlockedReason, "version": updated.Version,
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := s.store.UpdateWorkItem(ctx, updated, expectedVersion, event); err != nil {
		return domain.WorkItem{}, apperror.Normalize(err)
	}
	return s.store.GetWorkItem(ctx, updated.ID)
}

func (s *WorkItemService) mutableRun(ctx context.Context, runID string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return domain.Run{}, apperror.Normalize(err)
	}
	if run.Terminal() {
		return domain.Run{}, apperror.New(apperror.CodeFailedPrecondition, fmt.Sprintf("terminal run %s cannot change its work board", run.ID))
	}
	return run, nil
}

func normalizeServiceWorkItemDetails(itemID string, details domain.WorkItemDetails) (domain.WorkItemDetails, error) {
	details.Title = redact.String(details.Title)
	details.Description = redact.String(details.Description)
	details.Owner = redact.String(details.Owner)
	for index := range details.AcceptanceCriteria {
		details.AcceptanceCriteria[index] = redact.String(details.AcceptanceCriteria[index])
	}
	normalized, err := domain.NormalizeWorkItemDetails(itemID, details)
	if err != nil {
		return domain.WorkItemDetails{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return normalized, nil
}

func resolveExpectedWorkItemVersion(current domain.WorkItem, requested int64) (int64, error) {
	if requested < 0 {
		return 0, apperror.New(apperror.CodeInvalidArgument, "work item expected version cannot be negative")
	}
	if requested == 0 {
		return current.Version, nil
	}
	if requested != current.Version {
		return 0, apperror.New(apperror.CodeConflict, fmt.Sprintf("work item %s changed concurrently: expected version %d, got %d", current.ID, requested, current.Version))
	}
	return requested, nil
}
