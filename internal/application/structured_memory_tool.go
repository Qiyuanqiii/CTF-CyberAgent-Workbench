package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/toolgateway"
)

type StructuredMemoryMutationStore interface {
	WorkItemStore
	NoteStore
	CreateWorkItemToolOperation(ctx context.Context, operation runmutation.Operation, item domain.WorkItem,
		policyEvent events.Event, itemEvent events.Event, toolEvent events.Event) (domain.WorkItem, bool, error)
	CreateNoteToolOperation(ctx context.Context, operation runmutation.Operation, note domain.Note,
		policyEvent events.Event, noteEvent events.Event, toolEvent events.Event) (domain.Note, bool, error)
}

type StructuredMemoryToolExecutor struct {
	store     StructuredMemoryMutationStore
	workItems *WorkItemService
	notes     *NoteService
}

func NewStructuredMemoryToolExecutor(store StructuredMemoryMutationStore) *StructuredMemoryToolExecutor {
	return &StructuredMemoryToolExecutor{
		store: store, workItems: NewWorkItemService(store), notes: NewNoteService(store),
	}
}

func (e *StructuredMemoryToolExecutor) CreateWorkItem(ctx context.Context, scope toolgateway.StructuredMemoryContext,
	input toolgateway.WorkItemCreateInput,
) (toolgateway.StructuredMutationResult, error) {
	if e == nil || e.store == nil {
		return toolgateway.StructuredMutationResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"structured memory mutation store is required")
	}
	if err := scope.Validate(); err != nil {
		return toolgateway.StructuredMutationResult{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if scope.Tool != toolgateway.WorkItemCreateTool {
		return toolgateway.StructuredMutationResult{}, apperror.New(apperror.CodeInvalidArgument,
			"WorkItem executor received the wrong tool")
	}
	run, item, itemEvent, err := e.workItems.prepareCreate(ctx, CreateWorkItemRequest{
		RunID: scope.RunID, Title: input.Title, Description: input.Description, Priority: input.Priority,
		Owner: input.Owner, AcceptanceCriteria: input.AcceptanceCriteria, Dependencies: input.Dependencies,
	})
	if err != nil {
		return toolgateway.StructuredMutationResult{}, err
	}
	fingerprint, err := structuredWorkItemFingerprint(scope, item)
	if err != nil {
		return toolgateway.StructuredMutationResult{}, err
	}
	operation := structuredMemoryOperation(scope, runmutation.TargetWorkItem, item.ID, fingerprint, item.CreatedAt)
	policyEvent, toolEvent, err := structuredMemoryEvents(run, scope, item.ID)
	if err != nil {
		return toolgateway.StructuredMutationResult{}, err
	}
	stored, replayed, err := e.store.CreateWorkItemToolOperation(ctx, operation, item,
		policyEvent, itemEvent, toolEvent)
	if err != nil {
		return toolgateway.StructuredMutationResult{}, apperror.Normalize(err)
	}
	return toolgateway.StructuredMutationResult{
		EntityID: stored.ID, EntityKind: string(runmutation.TargetWorkItem), Status: string(stored.Status),
		Version: stored.Version, Replayed: replayed,
	}, nil
}

func (e *StructuredMemoryToolExecutor) CreateNote(ctx context.Context, scope toolgateway.StructuredMemoryContext,
	input toolgateway.NoteCreateInput,
) (toolgateway.StructuredMutationResult, error) {
	if e == nil || e.store == nil {
		return toolgateway.StructuredMutationResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"structured memory mutation store is required")
	}
	if err := scope.Validate(); err != nil {
		return toolgateway.StructuredMutationResult{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if scope.Tool != toolgateway.NoteCreateTool {
		return toolgateway.StructuredMutationResult{}, apperror.New(apperror.CodeInvalidArgument,
			"Note executor received the wrong tool")
	}
	run, note, noteEvent, err := e.notes.prepareCreate(ctx, CreateNoteRequest{
		RunID: scope.RunID, Title: input.Title, Content: input.Content, Category: input.Category,
		Visibility: input.Visibility, Owner: input.Owner, Tags: input.Tags, SourceRefs: input.SourceRefs,
		EvidenceIDs: input.EvidenceIDs, Pinned: input.Pinned,
	})
	if err != nil {
		return toolgateway.StructuredMutationResult{}, err
	}
	fingerprint, err := structuredNoteFingerprint(scope, note)
	if err != nil {
		return toolgateway.StructuredMutationResult{}, err
	}
	operation := structuredMemoryOperation(scope, runmutation.TargetNote, note.ID, fingerprint, note.CreatedAt)
	policyEvent, toolEvent, err := structuredMemoryEvents(run, scope, note.ID)
	if err != nil {
		return toolgateway.StructuredMutationResult{}, err
	}
	stored, replayed, err := e.store.CreateNoteToolOperation(ctx, operation, note,
		policyEvent, noteEvent, toolEvent)
	if err != nil {
		return toolgateway.StructuredMutationResult{}, apperror.Normalize(err)
	}
	return toolgateway.StructuredMutationResult{
		EntityID: stored.ID, EntityKind: string(runmutation.TargetNote), Status: string(stored.Status),
		Version: stored.Version, Replayed: replayed,
	}, nil
}

func structuredMemoryOperation(scope toolgateway.StructuredMemoryContext, kind runmutation.TargetKind,
	targetID string, fingerprint string, createdAt time.Time,
) runmutation.Operation {
	return runmutation.Operation{
		KeyDigest:          runmutation.OperationKeyDigest(string(scope.Tool), scope.RunID, scope.OperationKey),
		RequestFingerprint: fingerprint, InvocationID: scope.InvocationID, RunID: scope.RunID,
		SessionID: scope.SessionID, WorkspaceID: scope.WorkspaceID, ToolName: string(scope.Tool),
		TargetKind: kind, TargetID: targetID, RequestedBy: scope.RequestedBy, CreatedAt: createdAt,
	}
}

func structuredMemoryEvents(run domain.Run, scope toolgateway.StructuredMemoryContext,
	targetID string,
) (events.Event, events.Event, error) {
	policyEvent, err := events.New(run.ID, run.MissionID, events.PolicyDecisionEvent, "policy",
		scope.InvocationID, map[string]any{
			"context": "tool_run." + string(scope.Tool), "allowed": true, "needs_approval": false,
			"risk": scope.PolicyDecision.Risk, "reason": scope.PolicyDecision.Reason,
		})
	if err != nil {
		return events.Event{}, events.Event{}, err
	}
	toolEvent, err := events.New(run.ID, run.MissionID, events.ToolCompletedEvent,
		"structured_memory_tool", scope.InvocationID, map[string]any{
			"invocation_id": scope.InvocationID, "tool_name": scope.Tool,
			"target_id": targetID, "execution_backend": "run_memory",
		})
	if err != nil {
		return events.Event{}, events.Event{}, err
	}
	return policyEvent, toolEvent, nil
}

func structuredWorkItemFingerprint(scope toolgateway.StructuredMemoryContext, item domain.WorkItem) (string, error) {
	intent := struct {
		RunID              string                  `json:"run_id"`
		SessionID          string                  `json:"session_id"`
		WorkspaceID        string                  `json:"workspace_id"`
		RequestedBy        string                  `json:"requested_by"`
		Title              string                  `json:"title"`
		Description        string                  `json:"description"`
		Priority           domain.WorkItemPriority `json:"priority"`
		Owner              string                  `json:"owner"`
		AcceptanceCriteria []string                `json:"acceptance_criteria"`
		Dependencies       []string                `json:"dependencies"`
	}{
		RunID: scope.RunID, SessionID: scope.SessionID, WorkspaceID: scope.WorkspaceID,
		RequestedBy: scope.RequestedBy, Title: item.Title, Description: item.Description,
		Priority: item.Priority, Owner: item.Owner, AcceptanceCriteria: item.AcceptanceCriteria,
		Dependencies: item.Dependencies,
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("encode structured WorkItem fingerprint: %w", err)
	}
	return runmutation.Fingerprint("work_item_create.v1", string(encoded)), nil
}

func structuredNoteFingerprint(scope toolgateway.StructuredMemoryContext, note domain.Note) (string, error) {
	intent := struct {
		RunID       string                `json:"run_id"`
		SessionID   string                `json:"session_id"`
		WorkspaceID string                `json:"workspace_id"`
		RequestedBy string                `json:"requested_by"`
		Title       string                `json:"title"`
		Content     string                `json:"content"`
		Category    domain.NoteCategory   `json:"category"`
		Visibility  domain.NoteVisibility `json:"visibility"`
		Owner       string                `json:"owner"`
		Tags        []string              `json:"tags"`
		SourceRefs  []string              `json:"source_refs"`
		EvidenceIDs []string              `json:"evidence_ids"`
		Pinned      bool                  `json:"pinned"`
	}{
		RunID: scope.RunID, SessionID: scope.SessionID, WorkspaceID: scope.WorkspaceID,
		RequestedBy: scope.RequestedBy, Title: note.Title, Content: note.Content, Category: note.Category,
		Visibility: note.Visibility, Owner: note.Owner, Tags: note.Tags, SourceRefs: note.SourceRefs,
		EvidenceIDs: note.EvidenceIDs, Pinned: note.Pinned,
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("encode structured Note fingerprint: %w", err)
	}
	return runmutation.Fingerprint("note_create.v1", string(encoded)), nil
}
