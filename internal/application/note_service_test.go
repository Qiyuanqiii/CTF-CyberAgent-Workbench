package application_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/store"
)

func TestNoteServiceCreateUpdateArchiveRestore(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "note service", Profile: "code", Budget: domain.Budget{MaxTurns: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewNoteService(st)
	if _, err := service.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "invalid", Content: "content", Visibility: "owner",
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("expected owner visibility validation, code=%s err=%v", apperror.CodeOf(err), err)
	}
	testAPIKey := "s" + "k-" + strings.Repeat("q", 26)
	note, err := service.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "parser decision", Content: "Never persist " + testAPIKey,
		Category: "decision", Visibility: "run", Tags: []string{"Security", "parser"},
		SourceRefs: []string{"docs/spec.md"}, EvidenceIDs: []string{"evidence-1"}, Pinned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if note.Version != 1 || note.Category != domain.NoteDecision || !note.Pinned ||
		strings.Contains(note.Content, testAPIKey) || !strings.Contains(note.Content, "[REDACTED:api-key]") ||
		!slices.Equal(note.Tags, []string{"parser", "security"}) {
		t.Fatalf("unexpected created note: %#v", note)
	}
	content := "Use strict JSON decoding."
	category := "summary"
	visibility := "root"
	tags := []string{"parser", "decision"}
	pinned := false
	updated, err := service.Update(ctx, application.UpdateNoteRequest{
		ID: note.ID, ExpectedVersion: note.Version, Content: &content, Category: &category,
		Visibility: &visibility, Tags: &tags, Pinned: &pinned,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Content != content || updated.Category != domain.NoteSummary ||
		updated.Visibility != domain.NoteVisibilityRoot || updated.Pinned ||
		!slices.Equal(updated.Tags, []string{"decision", "parser"}) {
		t.Fatalf("unexpected updated note: %#v", updated)
	}
	noChange, err := service.Update(ctx, application.UpdateNoteRequest{ID: updated.ID, ExpectedVersion: updated.Version, Content: &content})
	if err != nil || noChange.Version != updated.Version {
		t.Fatalf("no-op update should not append a version: %#v err=%v", noChange, err)
	}
	if _, err := service.Update(ctx, application.UpdateNoteRequest{ID: updated.ID, ExpectedVersion: 1, Content: &content}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("expected stale note conflict, code=%s err=%v", apperror.CodeOf(err), err)
	}
	archived, err := service.Transition(ctx, updated.ID, updated.Version, domain.NoteArchived)
	if err != nil || archived.Status != domain.NoteArchived || archived.ArchivedAt == nil || archived.Version != 3 {
		t.Fatalf("unexpected archived note: %#v err=%v", archived, err)
	}
	if _, err := service.Update(ctx, application.UpdateNoteRequest{ID: archived.ID, Title: stringPointer("changed")}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected archived update rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
	restored, err := service.Transition(ctx, archived.ID, archived.Version, domain.NoteActive)
	if err != nil || restored.Status != domain.NoteActive || restored.ArchivedAt != nil || restored.Version != 4 {
		t.Fatalf("unexpected restored note: %#v err=%v", restored, err)
	}
	listed, err := service.List(ctx, domain.NoteFilter{
		RunID: run.ID, Statuses: []domain.NoteStatus{domain.NoteActive}, Categories: []domain.NoteCategory{domain.NoteSummary},
		Visibilities: []domain.NoteVisibility{domain.NoteVisibilityRoot}, Tags: []string{"parser"},
	})
	if err != nil || len(listed) != 1 || listed[0].ID != note.ID {
		t.Fatalf("unexpected note list: %#v err=%v", listed, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countApplicationEvent(timeline, events.NoteCreatedEvent) != 1 || countApplicationEvent(timeline, events.NoteChangedEvent) != 3 {
		t.Fatalf("unexpected note timeline: %#v", timeline)
	}
}

func TestNoteServiceRejectsTerminalRunMutation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "terminal notes", Profile: "review", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewNoteService(st)
	note, err := service.Create(ctx, application.CreateNoteRequest{RunID: run.ID, Title: "before close", Content: "content"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Complete(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, application.CreateNoteRequest{RunID: run.ID, Title: "late", Content: "content"}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected terminal create rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := service.Transition(ctx, note.ID, 0, domain.NoteArchived); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected terminal transition rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestNoteServiceVisibilityChangeClearsOwnerAndAuditsBothFields(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "note owner transition", Profile: "code", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewNoteService(st)
	note, err := service.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "private", Content: "root memory", Visibility: "owner", Owner: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	visibility := "run"
	unchangedTitle := note.Title
	updated, err := service.Update(ctx, application.UpdateNoteRequest{ID: note.ID, Title: &unchangedTitle, Visibility: &visibility})
	if err != nil || updated.Visibility != domain.NoteVisibilityRun || updated.Owner != "" {
		t.Fatalf("visibility transition did not clear owner: %#v err=%v", updated, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var payload string
	for _, event := range timeline {
		if event.Type == events.NoteChangedEvent {
			payload = event.PayloadJSON
		}
	}
	var audit struct {
		ChangedFields []string `json:"changed_fields"`
	}
	if err := json.Unmarshal([]byte(payload), &audit); err != nil || len(audit.ChangedFields) != 2 ||
		!slices.Contains(audit.ChangedFields, "visibility") || !slices.Contains(audit.ChangedFields, "owner") {
		t.Fatalf("owner clear was not audited: %s err=%v", payload, err)
	}
	if slices.Contains(audit.ChangedFields, "title") {
		t.Fatalf("unchanged requested field was incorrectly audited: %s", payload)
	}
}
