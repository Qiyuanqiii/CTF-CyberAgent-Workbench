package application

import (
	"context"
	"fmt"
	"slices"
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

type DeliveryCheckpointStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetRunMode(ctx context.Context, runID string) (domain.RunModeSnapshot, error)
	GetPlanDeliveryProposal(ctx context.Context, id string) (domain.PlanDeliveryProposal, error)
	GetPlanDeliverySelectionByRun(ctx context.Context, runID string) (domain.PlanDeliverySelection, bool, error)
	GetWorkItem(ctx context.Context, id string) (domain.WorkItem, error)
	GetNote(ctx context.Context, id string) (domain.Note, error)
	RecordDeliveryCheckpoint(ctx context.Context,
		operation domain.DeliveryCheckpointOperation,
		checkpoint domain.DeliveryCheckpoint, note domain.Note,
		checkpointEvent events.Event, noteEvent events.Event,
	) (domain.DeliveryCheckpoint, bool, error)
	GetDeliveryCheckpoint(ctx context.Context, id string) (domain.DeliveryCheckpoint, error)
	ListDeliveryCheckpoints(ctx context.Context, runID string, limit int) ([]domain.DeliveryCheckpoint, error)
}

type DeliveryCheckpointService struct {
	store DeliveryCheckpointStore
}

type RecordDeliveryCheckpointRequest struct {
	WorkItemID             string
	OperationKey           string
	RequestedBy            string
	FocusedVerification    string
	DiffAudit              string
	SecurityAudit          string
	FunctionalVerification string
	RobustnessAudit        string
	HandoffSummary         string
}

type RecordDeliveryCheckpointResult struct {
	Checkpoint domain.DeliveryCheckpoint
	Note       domain.Note
	Replayed   bool
}

func NewDeliveryCheckpointService(store DeliveryCheckpointStore) *DeliveryCheckpointService {
	return &DeliveryCheckpointService{store: store}
}

func (s *DeliveryCheckpointService) Record(ctx context.Context,
	request RecordDeliveryCheckpointRequest,
) (RecordDeliveryCheckpointResult, error) {
	if s == nil || s.store == nil {
		return RecordDeliveryCheckpointResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Delivery checkpoint store is required")
	}
	if err := normalizeDeliveryCheckpointRequest(&request); err != nil {
		return RecordDeliveryCheckpointResult{}, err
	}
	item, err := s.store.GetWorkItem(ctx, request.WorkItemID)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Normalize(err)
	}
	run, err := s.store.GetRun(ctx, item.RunID)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Normalize(err)
	}
	selection, selected, err := s.store.GetPlanDeliverySelectionByRun(ctx, run.ID)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Normalize(err)
	}
	if !selected {
		return RecordDeliveryCheckpointResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Delivery checkpoint requires an accepted Plan/Delivery direction")
	}
	selectedItem, found := deliverySelectionItem(selection, item.ID)
	if !found {
		return RecordDeliveryCheckpointResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"WorkItem is not part of the accepted Plan/Delivery direction")
	}
	proposal, err := s.store.GetPlanDeliveryProposal(ctx, selection.ProposalID)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Normalize(err)
	}
	if proposal.RunID != run.ID || selection.DirectionOrdinal < 1 ||
		selection.DirectionOrdinal > len(proposal.Spec.Directions) {
		return RecordDeliveryCheckpointResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Plan/Delivery source binding is inconsistent")
	}
	direction := proposal.Spec.Directions[selection.DirectionOrdinal-1]
	if selectedItem.ModuleOrdinal < 1 || selectedItem.ModuleOrdinal > len(direction.Modules) {
		return RecordDeliveryCheckpointResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "selected Delivery module is missing")
	}
	module := direction.Modules[selectedItem.ModuleOrdinal-1]
	if err := validateDeliveryWorkItemProjection(selection, module, item); err != nil {
		return RecordDeliveryCheckpointResult{}, err
	}
	mode, err := s.store.GetRunMode(ctx, run.ID)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Normalize(err)
	}
	if run.Status != domain.RunPaused || mode.Phase != domain.ExecutionPhaseDeliver ||
		item.Status != domain.WorkItemInProgress {
		return RecordDeliveryCheckpointResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Delivery checkpoint requires a paused Deliver-phase Run and an in-progress selected WorkItem")
	}
	now := time.Now().UTC()
	if now.Before(mode.CreatedAt) {
		now = mode.CreatedAt
	}
	checkpoint := domain.DeliveryCheckpoint{
		ID: idgen.New("delivery-checkpoint"), RunID: run.ID,
		SelectionID: selection.ID, ProposalID: proposal.ID, WorkItemID: item.ID,
		DirectionOrdinal: selection.DirectionOrdinal,
		ModuleOrdinal:    selectedItem.ModuleOrdinal, ModuleCount: len(selection.Items),
		ModeSnapshotID: mode.ID, ModeRevision: mode.Revision,
		WorkItemVersion:       item.Version,
		AcceptanceFingerprint: domain.DeliveryAcceptanceFingerprint(item.AcceptanceCriteria),
		SourceFingerprint:     domain.DeliverySourceFingerprint(proposal, selection, module, item),
		FocusedVerification:   request.FocusedVerification,
		DiffAudit:             request.DiffAudit, SecurityAudit: request.SecurityAudit,
		FullGateRequired:       selectedItem.ModuleOrdinal == len(selection.Items),
		FunctionalVerification: request.FunctionalVerification,
		RobustnessAudit:        request.RobustnessAudit,
		RequestedBy:            request.RequestedBy, Version: 1, CreatedAt: now,
	}
	if checkpoint.FullGateRequired {
		if checkpoint.FunctionalVerification == "" || checkpoint.RobustnessAudit == "" {
			return RecordDeliveryCheckpointResult{}, apperror.New(
				apperror.CodeInvalidArgument,
				"final Delivery module requires functional verification and robustness audit evidence")
		}
	} else if checkpoint.FunctionalVerification != "" || checkpoint.RobustnessAudit != "" {
		return RecordDeliveryCheckpointResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"functional and robustness evidence is reserved for the final module boundary")
	}
	note, err := buildDeliveryHandoffNote(checkpoint, item, request.HandoffSummary, now)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, err
	}
	checkpoint.HandoffNoteID = note.ID
	checkpoint.HandoffDigest = domain.DeliveryHandoffDigest(note.Title, note.Content)
	if err := checkpoint.Validate(); err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Delivery checkpoint is invalid", err)
	}
	operation := domain.DeliveryCheckpointOperation{
		KeyDigest: runmutation.OperationKeyDigest("delivery_checkpoint_record",
			run.ID, request.OperationKey),
		RequestFingerprint: domain.DeliveryCheckpointRequestFingerprint(checkpoint),
		CheckpointID:       checkpoint.ID, RunID: run.ID, WorkItemID: item.ID,
		RequestedBy: request.RequestedBy, CreatedAt: now,
	}
	checkpointEvent, noteEvent, err := deliveryCheckpointEvents(run, checkpoint, note)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, err
	}
	stored, replayed, err := s.store.RecordDeliveryCheckpoint(ctx, operation,
		checkpoint, note, checkpointEvent, noteEvent)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Normalize(err)
	}
	storedNote, err := s.store.GetNote(ctx, stored.HandoffNoteID)
	if err != nil {
		return RecordDeliveryCheckpointResult{}, apperror.Normalize(err)
	}
	return RecordDeliveryCheckpointResult{
		Checkpoint: stored, Note: storedNote, Replayed: replayed,
	}, nil
}

func (s *DeliveryCheckpointService) Get(ctx context.Context,
	id string,
) (domain.DeliveryCheckpoint, error) {
	if s == nil || s.store == nil {
		return domain.DeliveryCheckpoint{}, apperror.New(
			apperror.CodeFailedPrecondition, "Delivery checkpoint store is required")
	}
	value, err := s.store.GetDeliveryCheckpoint(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *DeliveryCheckpointService) List(ctx context.Context, runID string,
	limit int,
) ([]domain.DeliveryCheckpoint, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Delivery checkpoint store is required")
	}
	values, err := s.store.ListDeliveryCheckpoints(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func normalizeDeliveryCheckpointRequest(request *RecordDeliveryCheckpointRequest) error {
	if request == nil {
		return apperror.New(apperror.CodeInvalidArgument,
			"Delivery checkpoint request is required")
	}
	originalKey := request.OperationKey
	request.WorkItemID = strings.TrimSpace(request.WorkItemID)
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	request.RequestedBy = strings.TrimSpace(redact.String(request.RequestedBy))
	if request.WorkItemID == "" || request.OperationKey == "" || request.RequestedBy == "" {
		return apperror.New(apperror.CodeInvalidArgument,
			"WorkItem, operation key, and operator identities are required")
	}
	if originalKey != request.OperationKey || !utf8.ValidString(request.OperationKey) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Delivery checkpoint operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Delivery checkpoint operation key is invalid", err)
	}
	for _, current := range request.OperationKey + request.RequestedBy {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return apperror.New(apperror.CodeInvalidArgument,
				"Delivery operation key and operator cannot contain whitespace or control characters")
		}
	}
	if !domain.ValidAgentID(request.RequestedBy) || strings.ContainsRune(request.RequestedBy, 0) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Delivery checkpoint operator identity is invalid")
	}
	var err error
	for value, target := range map[string]*string{
		"focused verification": &request.FocusedVerification,
		"diff audit":           &request.DiffAudit, "security audit": &request.SecurityAudit,
	} {
		*target, err = domain.NormalizeDeliveryEvidence(redact.String(*target),
			value, domain.MaxDeliveryEvidenceRunes, false)
		if err != nil {
			return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
		}
	}
	for value, target := range map[string]*string{
		"functional verification": &request.FunctionalVerification,
		"robustness audit":        &request.RobustnessAudit,
	} {
		*target, err = domain.NormalizeDeliveryEvidence(redact.String(*target),
			value, domain.MaxDeliveryEvidenceRunes, true)
		if err != nil {
			return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
		}
	}
	request.HandoffSummary, err = domain.NormalizeDeliveryEvidence(
		redact.String(request.HandoffSummary), "handoff summary",
		domain.MaxDeliveryHandoffSummaryRunes, false)
	if err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return nil
}

func deliverySelectionItem(selection domain.PlanDeliverySelection,
	workItemID string,
) (domain.PlanDeliverySelectionItem, bool) {
	for _, item := range selection.Items {
		if item.WorkItemID == workItemID {
			return item, true
		}
	}
	return domain.PlanDeliverySelectionItem{}, false
}

func validateDeliveryWorkItemProjection(selection domain.PlanDeliverySelection,
	module domain.PlanDeliveryModule, item domain.WorkItem,
) error {
	if item.Title != module.Title || item.Description != module.Objective ||
		!slices.Equal(item.AcceptanceCriteria, module.AcceptanceCriteria) {
		return apperror.New(apperror.CodeFailedPrecondition,
			"selected WorkItem no longer matches its immutable Delivery module")
	}
	expectedDependencies := make([]string, len(module.Dependencies))
	for index, ordinal := range module.Dependencies {
		if ordinal < 1 || ordinal > len(selection.Items) {
			return apperror.New(apperror.CodeFailedPrecondition,
				"Delivery module dependency is outside the accepted selection")
		}
		expectedDependencies[index] = selection.Items[ordinal-1].WorkItemID
	}
	if !slices.Equal(item.Dependencies, expectedDependencies) {
		return apperror.New(apperror.CodeFailedPrecondition,
			"selected WorkItem dependencies no longer match the Delivery module")
	}
	return nil
}

func buildDeliveryHandoffNote(checkpoint domain.DeliveryCheckpoint,
	item domain.WorkItem, summary string, now time.Time,
) (domain.Note, error) {
	title := fmt.Sprintf("Delivery handoff: slice %d", checkpoint.ModuleOrdinal)
	var content strings.Builder
	content.WriteString("Delivery slice handoff\n\n")
	content.WriteString("Summary\n")
	content.WriteString(summary)
	content.WriteString("\n\nAcceptance criteria\n")
	for _, criterion := range item.AcceptanceCriteria {
		content.WriteString("- ")
		content.WriteString(criterion)
		content.WriteByte('\n')
	}
	content.WriteString("\nFocused verification\n")
	content.WriteString(checkpoint.FocusedVerification)
	content.WriteString("\n\nDiff audit\n")
	content.WriteString(checkpoint.DiffAudit)
	content.WriteString("\n\nSecurity audit\n")
	content.WriteString(checkpoint.SecurityAudit)
	if checkpoint.FullGateRequired {
		content.WriteString("\n\nFunctional verification\n")
		content.WriteString(checkpoint.FunctionalVerification)
		content.WriteString("\n\nRobustness audit\n")
		content.WriteString(checkpoint.RobustnessAudit)
	}
	content.WriteString("\n\nControl boundary\nEvidence is an operator attestation. Go binds this Note to the exact selected source, Deliver-mode revision, and WorkItem version; it grants no execution capability.")
	details, err := normalizeServiceNoteDetails(domain.NoteDetails{
		Title: title, Content: content.String(), Category: domain.NoteSummary,
		Visibility: domain.NoteVisibilityRun, OwnerAgentID: item.OwnerAgentID,
		Tags: []string{"delivery-checkpoint", "slice-handoff"},
		SourceRefs: []string{
			"plan_delivery_selection:" + checkpoint.SelectionID,
			"work_item:" + checkpoint.WorkItemID,
			"run_mode:" + checkpoint.ModeSnapshotID,
			"delivery_checkpoint:" + checkpoint.ID,
		},
		EvidenceIDs: []string{checkpoint.AcceptanceFingerprint,
			checkpoint.SourceFingerprint},
		Pinned: true,
	})
	if err != nil {
		return domain.Note{}, err
	}
	note := domain.Note{
		ID: idgen.New("note"), RunID: checkpoint.RunID,
		Title: details.Title, Content: details.Content, Category: details.Category,
		Visibility: details.Visibility, Owner: details.Owner,
		OwnerAgentID: details.OwnerAgentID,
		Tags:         details.Tags, SourceRefs: details.SourceRefs,
		EvidenceIDs: details.EvidenceIDs, Status: domain.NoteActive,
		Pinned: details.Pinned, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := note.Validate(); err != nil {
		return domain.Note{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Delivery handoff Note is invalid", err)
	}
	return note, nil
}

func deliveryCheckpointEvents(run domain.Run, checkpoint domain.DeliveryCheckpoint,
	note domain.Note,
) (events.Event, events.Event, error) {
	checkpointEvent, err := events.New(run.ID, run.MissionID,
		events.DeliveryCheckpointRecordedEvent, "delivery", checkpoint.ID,
		map[string]any{
			"checkpoint_id": checkpoint.ID, "selection_id": checkpoint.SelectionID,
			"proposal_id": checkpoint.ProposalID, "work_item_id": checkpoint.WorkItemID,
			"direction_ordinal": checkpoint.DirectionOrdinal,
			"module_ordinal":    checkpoint.ModuleOrdinal, "module_count": checkpoint.ModuleCount,
			"mode_snapshot_id":   checkpoint.ModeSnapshotID,
			"mode_revision":      checkpoint.ModeRevision,
			"work_item_version":  checkpoint.WorkItemVersion,
			"full_gate_required": checkpoint.FullGateRequired,
			"handoff_note_id":    checkpoint.HandoffNoteID,
			"version":            checkpoint.Version,
		})
	if err != nil {
		return events.Event{}, events.Event{}, err
	}
	checkpointEvent.CreatedAt = checkpoint.CreatedAt
	noteEvent, err := events.New(run.ID, run.MissionID, events.NoteCreatedEvent,
		"delivery", note.ID, map[string]any{
			"checkpoint_id": checkpoint.ID, "work_item_id": checkpoint.WorkItemID,
			"category": note.Category, "visibility": note.Visibility,
			"pinned": note.Pinned, "status": note.Status, "version": note.Version,
		})
	if err != nil {
		return events.Event{}, events.Event{}, err
	}
	noteEvent.CreatedAt = checkpoint.CreatedAt
	return checkpointEvent, noteEvent, nil
}
