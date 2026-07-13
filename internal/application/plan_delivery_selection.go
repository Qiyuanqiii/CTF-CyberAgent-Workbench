package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
)

type PlanDeliverySelectionStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetRunMode(ctx context.Context, runID string) (domain.RunModeSnapshot, error)
	GetPlanDeliveryProposal(ctx context.Context,
		id string) (domain.PlanDeliveryProposal, error)
	ListPlanDeliveryProposals(ctx context.Context, runID string,
		limit int) ([]domain.PlanDeliveryProposal, error)
	GetPlanDeliverySelection(ctx context.Context,
		id string) (domain.PlanDeliverySelection, error)
	GetPlanDeliverySelectionByRun(ctx context.Context,
		runID string) (domain.PlanDeliverySelection, bool, error)
	SelectPlanDeliveryDirection(ctx context.Context,
		operation domain.PlanDeliverySelectionOperation,
		selection domain.PlanDeliverySelection, items []domain.WorkItem,
		note domain.Note, selectionEvent events.Event,
		itemEvents []events.Event, noteEvent events.Event,
	) (domain.PlanDeliverySelection, bool, error)
	GetWorkItem(ctx context.Context, id string) (domain.WorkItem, error)
	GetNote(ctx context.Context, id string) (domain.Note, error)
}

type PlanDeliveryService struct {
	store PlanDeliverySelectionStore
}

type SelectPlanDeliveryDirectionRequest struct {
	ProposalID   string
	Direction    int
	OperationKey string
	RequestedBy  string
}

type SelectPlanDeliveryDirectionResult struct {
	Selection domain.PlanDeliverySelection
	WorkItems []domain.WorkItem
	Note      domain.Note
	Replayed  bool
}

func NewPlanDeliveryService(store PlanDeliverySelectionStore) *PlanDeliveryService {
	return &PlanDeliveryService{store: store}
}

func (s *PlanDeliveryService) GetProposal(ctx context.Context,
	id string,
) (domain.PlanDeliveryProposal, error) {
	if s == nil || s.store == nil {
		return domain.PlanDeliveryProposal{}, apperror.New(
			apperror.CodeFailedPrecondition, "Plan/Delivery store is required")
	}
	proposal, err := s.store.GetPlanDeliveryProposal(ctx, strings.TrimSpace(id))
	return proposal, apperror.Normalize(err)
}

func (s *PlanDeliveryService) ListProposals(ctx context.Context,
	runID string, limit int,
) ([]domain.PlanDeliveryProposal, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Plan/Delivery store is required")
	}
	proposals, err := s.store.ListPlanDeliveryProposals(ctx,
		strings.TrimSpace(runID), limit)
	return proposals, apperror.Normalize(err)
}

func (s *PlanDeliveryService) SelectionForRun(ctx context.Context,
	runID string,
) (domain.PlanDeliverySelection, bool, error) {
	if s == nil || s.store == nil {
		return domain.PlanDeliverySelection{}, false, apperror.New(
			apperror.CodeFailedPrecondition, "Plan/Delivery store is required")
	}
	selection, found, err := s.store.GetPlanDeliverySelectionByRun(ctx,
		strings.TrimSpace(runID))
	return selection, found, apperror.Normalize(err)
}

func (s *PlanDeliveryService) Select(ctx context.Context,
	request SelectPlanDeliveryDirectionRequest,
) (SelectPlanDeliveryDirectionResult, error) {
	if s == nil || s.store == nil {
		return SelectPlanDeliveryDirectionResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Plan/Delivery store is required")
	}
	originalOperationKey := request.OperationKey
	request.ProposalID = strings.TrimSpace(request.ProposalID)
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	request.RequestedBy = strings.TrimSpace(redact.String(request.RequestedBy))
	if request.ProposalID == "" || request.OperationKey == "" ||
		request.RequestedBy == "" {
		return SelectPlanDeliveryDirectionResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"proposal, operation key, and operator identities are required")
	}
	if originalOperationKey != request.OperationKey || !utf8.ValidString(request.OperationKey) {
		return SelectPlanDeliveryDirectionResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"Plan/Delivery operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil {
		return SelectPlanDeliveryDirectionResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Plan/Delivery operation key is invalid", err)
	}
	for _, current := range request.OperationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return SelectPlanDeliveryDirectionResult{}, apperror.New(
				apperror.CodeInvalidArgument,
				"Plan/Delivery operation key cannot contain whitespace or control characters")
		}
	}
	if !domain.ValidAgentID(request.RequestedBy) || strings.ContainsRune(request.RequestedBy, 0) {
		return SelectPlanDeliveryDirectionResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"Plan/Delivery operator identity is invalid")
	}
	for _, current := range request.RequestedBy {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return SelectPlanDeliveryDirectionResult{}, apperror.New(
				apperror.CodeInvalidArgument,
				"Plan/Delivery operator identity cannot contain whitespace or control characters")
		}
	}
	if request.Direction < 1 ||
		request.Direction > domain.PlanDeliveryDirectionCount {
		return SelectPlanDeliveryDirectionResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"Plan/Delivery direction must be 1, 2, or 3")
	}
	proposal, err := s.store.GetPlanDeliveryProposal(ctx, request.ProposalID)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, apperror.Normalize(err)
	}
	run, err := s.store.GetRun(ctx, proposal.RunID)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, apperror.Normalize(err)
	}
	mode, err := s.store.GetRunMode(ctx, proposal.RunID)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, apperror.Normalize(err)
	}
	if run.Status != domain.RunPaused || mode.Phase != domain.ExecutionPhasePlan ||
		mode.Revision != proposal.ModeRevision {
		return SelectPlanDeliveryDirectionResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"direction choice requires the paused Run in the proposal's Plan revision")
	}
	direction := proposal.Spec.Directions[request.Direction-1]
	now := time.Now().UTC()
	if now.Before(proposal.CreatedAt) {
		now = proposal.CreatedAt
	}
	items, err := buildPlanDeliveryWorkItems(proposal, direction, now)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, err
	}
	note, err := buildPlanDeliveryHandoffNote(proposal, direction, now)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, err
	}
	selection := domain.PlanDeliverySelection{
		ID: idgen.New("plan-selection"), ProposalID: proposal.ID,
		RunID: proposal.RunID, RootAgentID: proposal.RootAgentID,
		DirectionOrdinal: direction.Ordinal, NoteID: note.ID,
		RequestedBy: request.RequestedBy, Version: 1, CreatedAt: now,
		Items: make([]domain.PlanDeliverySelectionItem, len(items)),
	}
	for index, item := range items {
		selection.Items[index] = domain.PlanDeliverySelectionItem{
			Ordinal: index + 1, ModuleOrdinal: index + 1, WorkItemID: item.ID,
		}
	}
	operation := domain.PlanDeliverySelectionOperation{
		KeyDigest: runmutation.OperationKeyDigest("plan_delivery_select",
			proposal.RunID, request.OperationKey),
		RequestFingerprint: domain.PlanDeliverySelectionRequestFingerprint(
			proposal.ID, proposal.RunID, direction.Ordinal, request.RequestedBy),
		SelectionID: selection.ID, ProposalID: proposal.ID,
		RunID: proposal.RunID, RequestedBy: request.RequestedBy, CreatedAt: now,
	}
	selectionEvent, itemEvents, noteEvent, err := planDeliverySelectionEvents(
		run, proposal, selection, items, note)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, err
	}
	stored, replayed, err := s.store.SelectPlanDeliveryDirection(ctx, operation,
		selection, items, note, selectionEvent, itemEvents, noteEvent)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, apperror.Normalize(err)
	}
	result := SelectPlanDeliveryDirectionResult{
		Selection: stored, Replayed: replayed,
		WorkItems: make([]domain.WorkItem, len(stored.Items)),
	}
	for index, selected := range stored.Items {
		result.WorkItems[index], err = s.store.GetWorkItem(ctx, selected.WorkItemID)
		if err != nil {
			return SelectPlanDeliveryDirectionResult{}, apperror.Normalize(err)
		}
	}
	result.Note, err = s.store.GetNote(ctx, stored.NoteID)
	if err != nil {
		return SelectPlanDeliveryDirectionResult{}, apperror.Normalize(err)
	}
	return result, nil
}

func buildPlanDeliveryWorkItems(proposal domain.PlanDeliveryProposal,
	direction domain.PlanDeliveryDirection, now time.Time,
) ([]domain.WorkItem, error) {
	items := make([]domain.WorkItem, len(direction.Modules))
	for index, module := range direction.Modules {
		item := domain.WorkItem{
			ID: idgen.New("work"), RunID: proposal.RunID,
			Status: domain.WorkItemPending, Priority: domain.WorkItemPriorityNormal,
			OwnerAgentID: proposal.RootAgentID, Version: 1,
			CreatedAt: now, UpdatedAt: now,
		}
		dependencies := make([]string, len(module.Dependencies))
		for dependencyIndex, dependency := range module.Dependencies {
			if dependency <= 0 || dependency > index {
				return nil, errors.New("Plan/Delivery module dependency is not backward-only")
			}
			dependencies[dependencyIndex] = items[dependency-1].ID
		}
		details, err := normalizeServiceWorkItemDetails(item.ID,
			domain.WorkItemDetails{
				Title: module.Title, Description: module.Objective,
				Priority:           domain.WorkItemPriorityNormal,
				OwnerAgentID:       proposal.RootAgentID,
				AcceptanceCriteria: module.AcceptanceCriteria,
				Dependencies:       dependencies,
			})
		if err != nil {
			return nil, err
		}
		item.Title = details.Title
		item.Description = details.Description
		item.Owner = details.Owner
		item.OwnerAgentID = details.OwnerAgentID
		item.AcceptanceCriteria = details.AcceptanceCriteria
		item.Dependencies = details.Dependencies
		if err := item.Validate(); err != nil {
			return nil, apperror.Wrap(apperror.CodeInvalidArgument,
				"selected Plan/Delivery WorkItem is invalid", err)
		}
		items[index] = item
	}
	return items, nil
}

func buildPlanDeliveryHandoffNote(proposal domain.PlanDeliveryProposal,
	direction domain.PlanDeliveryDirection, now time.Time,
) (domain.Note, error) {
	details, err := normalizeServiceNoteDetails(domain.NoteDetails{
		Title:    domain.PlanDeliveryHandoffTitle(direction),
		Content:  domain.PlanDeliveryHandoffContent(proposal, direction),
		Category: domain.NoteDecision, Visibility: domain.NoteVisibilityRun,
		OwnerAgentID: proposal.RootAgentID,
		Tags:         []string{"plan-delivery", "selected-direction"},
		SourceRefs:   []string{"plan_delivery:" + proposal.ID}, Pinned: true,
	})
	if err != nil {
		return domain.Note{}, err
	}
	note := domain.Note{
		ID: idgen.New("note"), RunID: proposal.RunID,
		Title: details.Title, Content: details.Content,
		Category: details.Category, Visibility: details.Visibility,
		Owner: details.Owner, OwnerAgentID: details.OwnerAgentID,
		Tags: details.Tags, SourceRefs: details.SourceRefs,
		EvidenceIDs: details.EvidenceIDs, Status: domain.NoteActive,
		Pinned: details.Pinned, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := note.Validate(); err != nil {
		return domain.Note{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"selected Plan/Delivery handoff Note is invalid", err)
	}
	return note, nil
}

func planDeliverySelectionEvents(run domain.Run,
	proposal domain.PlanDeliveryProposal, selection domain.PlanDeliverySelection,
	items []domain.WorkItem, note domain.Note,
) (events.Event, []events.Event, events.Event, error) {
	selectionEvent, err := events.New(run.ID, run.MissionID,
		events.PlanDeliveryDirectionSelectedEvent, "plan_delivery", selection.ID,
		map[string]any{
			"selection_id": selection.ID, "proposal_id": proposal.ID,
			"direction_ordinal": selection.DirectionOrdinal,
			"module_count":         len(selection.Items), "note_id": selection.NoteID,
			"phase_changed": false, "capability_grant": false,
		})
	if err != nil {
		return events.Event{}, nil, events.Event{}, err
	}
	selectionEvent.CreatedAt = selection.CreatedAt
	itemEvents := make([]events.Event, len(items))
	for index, item := range items {
		itemEvents[index], err = events.New(run.ID, run.MissionID,
			events.WorkItemCreatedEvent, "plan_delivery", item.ID,
			map[string]any{
				"selection_id": selection.ID, "proposal_id": proposal.ID,
				"direction_ordinal": selection.DirectionOrdinal,
				"module_ordinal":    index + 1, "status": item.Status,
				"priority": item.Priority, "owner_agent_id": item.OwnerAgentID,
				"dependency_count": len(item.Dependencies),
				"acceptance_count": len(item.AcceptanceCriteria),
				"version":          item.Version,
			})
		if err != nil {
			return events.Event{}, nil, events.Event{}, err
		}
		itemEvents[index].CreatedAt = selection.CreatedAt
	}
	noteEvent, err := events.New(run.ID, run.MissionID, events.NoteCreatedEvent,
		"plan_delivery", note.ID, map[string]any{
			"selection_id": selection.ID, "proposal_id": proposal.ID,
			"direction_ordinal": selection.DirectionOrdinal,
			"category":          note.Category, "visibility": note.Visibility,
			"owner_agent_id": note.OwnerAgentID, "pinned": note.Pinned,
			"status": note.Status, "version": note.Version,
		})
	if err != nil {
		return events.Event{}, nil, events.Event{}, err
	}
	noteEvent.CreatedAt = selection.CreatedAt
	return selectionEvent, itemEvents, noteEvent, nil
}
