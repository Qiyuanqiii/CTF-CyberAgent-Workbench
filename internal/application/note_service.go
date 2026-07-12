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

type NoteStore interface {
	CreateNote(ctx context.Context, note domain.Note, event events.Event) error
	GetNote(ctx context.Context, id string) (domain.Note, error)
	ListNotes(ctx context.Context, filter domain.NoteFilter) ([]domain.Note, error)
	UpdateNote(ctx context.Context, note domain.Note, expectedVersion int64, event events.Event) error
	GetRun(ctx context.Context, id string) (domain.Run, error)
}

type NoteService struct {
	store NoteStore
}

type CreateNoteRequest struct {
	RunID        string
	Title        string
	Content      string
	Category     string
	Visibility   string
	Owner        string
	OwnerAgentID string
	Tags         []string
	SourceRefs   []string
	EvidenceIDs  []string
	Pinned       bool
}

type UpdateNoteRequest struct {
	ID              string
	ExpectedVersion int64
	Title           *string
	Content         *string
	Category        *string
	Visibility      *string
	Owner           *string
	OwnerAgentID    *string
	Tags            *[]string
	SourceRefs      *[]string
	EvidenceIDs     *[]string
	Pinned          *bool
}

func NewNoteService(store NoteStore) *NoteService {
	return &NoteService{store: store}
}

func (s *NoteService) Create(ctx context.Context, req CreateNoteRequest) (domain.Note, error) {
	if s == nil || s.store == nil {
		return domain.Note{}, apperror.New(apperror.CodeFailedPrecondition, "note store is required")
	}
	_, note, event, err := s.prepareCreate(ctx, req)
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.store.CreateNote(ctx, note, event); err != nil {
		return domain.Note{}, apperror.Normalize(err)
	}
	return s.store.GetNote(ctx, note.ID)
}

func (s *NoteService) prepareCreate(ctx context.Context, req CreateNoteRequest) (domain.Run, domain.Note, events.Event, error) {
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		return domain.Run{}, domain.Note{}, events.Event{}, apperror.New(apperror.CodeInvalidArgument, "note run id is required")
	}
	run, err := s.mutableRun(ctx, runID)
	if err != nil {
		return domain.Run{}, domain.Note{}, events.Event{}, err
	}
	category, err := domain.ParseNoteCategory(req.Category)
	if err != nil {
		return domain.Run{}, domain.Note{}, events.Event{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	visibility, err := domain.ParseNoteVisibility(req.Visibility)
	if err != nil {
		return domain.Run{}, domain.Note{}, events.Event{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	details, err := normalizeServiceNoteDetails(domain.NoteDetails{
		Title: req.Title, Content: req.Content, Category: category, Visibility: visibility, Owner: req.Owner,
		OwnerAgentID: req.OwnerAgentID,
		Tags:         slices.Clone(req.Tags), SourceRefs: slices.Clone(req.SourceRefs), EvidenceIDs: slices.Clone(req.EvidenceIDs), Pinned: req.Pinned,
	})
	if err != nil {
		return domain.Run{}, domain.Note{}, events.Event{}, err
	}
	now := time.Now().UTC()
	note := domain.Note{
		ID: idgen.New("note"), RunID: run.ID, Title: details.Title, Content: details.Content,
		Category: details.Category, Visibility: details.Visibility, Owner: details.Owner,
		OwnerAgentID: details.OwnerAgentID, Tags: details.Tags,
		SourceRefs: details.SourceRefs, EvidenceIDs: details.EvidenceIDs, Status: domain.NoteActive,
		Pinned: details.Pinned, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := note.Validate(); err != nil {
		return domain.Run{}, domain.Note{}, events.Event{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	event, err := events.New(run.ID, run.MissionID, events.NoteCreatedEvent, "note_service", note.ID, map[string]any{
		"title": note.Title, "category": note.Category, "visibility": note.Visibility, "owner": note.Owner,
		"owner_agent_id": note.OwnerAgentID,
		"tags":           note.Tags, "evidence_ids": note.EvidenceIDs, "source_count": len(note.SourceRefs),
		"pinned": note.Pinned, "status": note.Status, "version": note.Version,
	})
	if err != nil {
		return domain.Run{}, domain.Note{}, events.Event{}, err
	}
	return run, note, event, nil
}

func (s *NoteService) Get(ctx context.Context, id string) (domain.Note, error) {
	if s == nil || s.store == nil {
		return domain.Note{}, apperror.New(apperror.CodeFailedPrecondition, "note store is required")
	}
	note, err := s.store.GetNote(ctx, strings.TrimSpace(id))
	return note, apperror.Normalize(err)
}

func (s *NoteService) List(ctx context.Context, filter domain.NoteFilter) ([]domain.Note, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "note store is required")
	}
	notes, err := s.store.ListNotes(ctx, filter)
	return notes, apperror.Normalize(err)
}

func (s *NoteService) Update(ctx context.Context, req UpdateNoteRequest) (domain.Note, error) {
	if s == nil || s.store == nil {
		return domain.Note{}, apperror.New(apperror.CodeFailedPrecondition, "note store is required")
	}
	if req.Title == nil && req.Content == nil && req.Category == nil && req.Visibility == nil && req.Owner == nil &&
		req.OwnerAgentID == nil &&
		req.Tags == nil && req.SourceRefs == nil && req.EvidenceIDs == nil && req.Pinned == nil {
		return domain.Note{}, apperror.New(apperror.CodeInvalidArgument, "note update requires at least one changed field")
	}
	current, err := s.store.GetNote(ctx, strings.TrimSpace(req.ID))
	if err != nil {
		return domain.Note{}, apperror.Normalize(err)
	}
	run, err := s.mutableRun(ctx, current.RunID)
	if err != nil {
		return domain.Note{}, err
	}
	expectedVersion, err := resolveExpectedNoteVersion(current, req.ExpectedVersion)
	if err != nil {
		return domain.Note{}, err
	}
	details := domain.NoteDetails{
		Title: current.Title, Content: current.Content, Category: current.Category, Visibility: current.Visibility,
		Owner: current.Owner, OwnerAgentID: current.OwnerAgentID, Tags: slices.Clone(current.Tags),
		SourceRefs:  slices.Clone(current.SourceRefs),
		EvidenceIDs: slices.Clone(current.EvidenceIDs), Pinned: current.Pinned,
	}
	if req.Title != nil {
		details.Title = *req.Title
	}
	if req.Content != nil {
		details.Content = *req.Content
	}
	if req.Category != nil {
		category, err := domain.ParseNoteCategory(*req.Category)
		if err != nil {
			return domain.Note{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
		}
		details.Category = category
	}
	if req.Visibility != nil {
		visibility, err := domain.ParseNoteVisibility(*req.Visibility)
		if err != nil {
			return domain.Note{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
		}
		details.Visibility = visibility
		if visibility != domain.NoteVisibilityOwner && req.Owner == nil {
			details.Owner = ""
		}
	}
	if req.Owner != nil {
		details.Owner = *req.Owner
	}
	if req.OwnerAgentID != nil {
		details.OwnerAgentID = *req.OwnerAgentID
	}
	if req.Tags != nil {
		details.Tags = slices.Clone(*req.Tags)
	}
	if req.SourceRefs != nil {
		details.SourceRefs = slices.Clone(*req.SourceRefs)
	}
	if req.EvidenceIDs != nil {
		details.EvidenceIDs = slices.Clone(*req.EvidenceIDs)
	}
	if req.Pinned != nil {
		details.Pinned = *req.Pinned
	}
	details, err = normalizeServiceNoteDetails(details)
	if err != nil {
		return domain.Note{}, err
	}
	updated := current
	if err := updated.ApplyDetails(details, time.Now().UTC()); err != nil {
		return domain.Note{}, apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
	}
	changed := changedApplicationNoteFields(current, updated)
	if len(changed) == 0 {
		return current, nil
	}
	updated.Version = expectedVersion + 1
	event, err := events.New(updated.RunID, run.MissionID, events.NoteChangedEvent, "note_service", updated.ID, map[string]any{
		"status": updated.Status, "changed_fields": changed, "version": updated.Version,
	})
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.store.UpdateNote(ctx, updated, expectedVersion, event); err != nil {
		return domain.Note{}, apperror.Normalize(err)
	}
	return s.store.GetNote(ctx, updated.ID)
}

func (s *NoteService) Transition(ctx context.Context, id string, expectedVersion int64, target domain.NoteStatus) (domain.Note, error) {
	if s == nil || s.store == nil {
		return domain.Note{}, apperror.New(apperror.CodeFailedPrecondition, "note store is required")
	}
	current, err := s.store.GetNote(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Note{}, apperror.Normalize(err)
	}
	run, err := s.mutableRun(ctx, current.RunID)
	if err != nil {
		return domain.Note{}, err
	}
	expectedVersion, err = resolveExpectedNoteVersion(current, expectedVersion)
	if err != nil {
		return domain.Note{}, err
	}
	if current.Status == target {
		return current, nil
	}
	updated := current
	if err := updated.Transition(target, time.Now().UTC()); err != nil {
		return domain.Note{}, apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
	}
	updated.Version = expectedVersion + 1
	event, err := events.New(updated.RunID, run.MissionID, events.NoteChangedEvent, "note_service", updated.ID, map[string]any{
		"from": current.Status, "to": updated.Status, "version": updated.Version,
	})
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.store.UpdateNote(ctx, updated, expectedVersion, event); err != nil {
		return domain.Note{}, apperror.Normalize(err)
	}
	return s.store.GetNote(ctx, updated.ID)
}

func (s *NoteService) mutableRun(ctx context.Context, runID string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return domain.Run{}, apperror.Normalize(err)
	}
	if run.Terminal() {
		return domain.Run{}, apperror.New(apperror.CodeFailedPrecondition, fmt.Sprintf("terminal run %s cannot change its notes", run.ID))
	}
	return run, nil
}

func normalizeServiceNoteDetails(details domain.NoteDetails) (domain.NoteDetails, error) {
	details.Title = redact.String(details.Title)
	details.Content = redact.String(details.Content)
	details.Owner = redact.String(details.Owner)
	details.Tags = redactNoteStrings(details.Tags)
	details.SourceRefs = redactNoteStrings(details.SourceRefs)
	details.EvidenceIDs = redactNoteStrings(details.EvidenceIDs)
	normalized, err := domain.NormalizeNoteDetails(details)
	if err != nil {
		return domain.NoteDetails{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return normalized, nil
}

func redactNoteStrings(values []string) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = redact.String(value)
	}
	return out
}

func resolveExpectedNoteVersion(current domain.Note, requested int64) (int64, error) {
	if requested < 0 {
		return 0, apperror.New(apperror.CodeInvalidArgument, "note expected version cannot be negative")
	}
	if requested == 0 {
		return current.Version, nil
	}
	if requested != current.Version {
		return 0, apperror.New(apperror.CodeConflict,
			fmt.Sprintf("note %s changed concurrently: expected version %d, got %d", current.ID, requested, current.Version))
	}
	return requested, nil
}

func changedApplicationNoteFields(left domain.Note, right domain.Note) []string {
	fields := make([]string, 0, 9)
	if left.Title != right.Title {
		fields = append(fields, "title")
	}
	if left.Content != right.Content {
		fields = append(fields, "content")
	}
	if left.Category != right.Category {
		fields = append(fields, "category")
	}
	if left.Visibility != right.Visibility {
		fields = append(fields, "visibility")
	}
	if left.Owner != right.Owner {
		fields = append(fields, "owner")
	}
	if left.OwnerAgentID != right.OwnerAgentID {
		fields = append(fields, "owner_agent_id")
	}
	if !slices.Equal(left.Tags, right.Tags) {
		fields = append(fields, "tags")
	}
	if !slices.Equal(left.SourceRefs, right.SourceRefs) {
		fields = append(fields, "source_refs")
	}
	if !slices.Equal(left.EvidenceIDs, right.EvidenceIDs) {
		fields = append(fields, "evidence_ids")
	}
	if left.Pinned != right.Pinned {
		fields = append(fields, "pinned")
	}
	return fields
}
