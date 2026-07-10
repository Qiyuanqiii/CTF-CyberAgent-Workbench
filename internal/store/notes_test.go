package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
)

func TestSQLiteNotesPersistRelationsFiltersAndTransactionalEvents(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "note persistence")
	testAPIKey := "s" + "k-" + strings.Repeat("n", 26)
	note := newNoteTest(run.ID, "parser decision "+testAPIKey, "Keep strict JSON and record "+testAPIKey)
	note.Category = domain.NoteDecision
	note.Pinned = true
	note.Tags = []string{"security", "parser"}
	note.SourceRefs = []string{"docs/spec.md", "request " + testAPIKey}
	note.EvidenceIDs = []string{"evidence-2", "evidence-1"}
	createdEvent := newNoteCreatedEvent(t, mission.ID, note)
	if err := st.CreateNote(ctx, note, createdEvent); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(loaded.Title, testAPIKey) || strings.Contains(loaded.Content, testAPIKey) ||
		!strings.Contains(loaded.Content, "[REDACTED:api-key]") ||
		!slicesEqualStrings(loaded.Tags, []string{"parser", "security"}) ||
		!slicesEqualStrings(loaded.EvidenceIDs, []string{"evidence-1", "evidence-2"}) {
		t.Fatalf("unexpected stored note: %#v", loaded)
	}
	pinned := true
	listed, err := st.ListNotes(ctx, domain.NoteFilter{
		RunID: run.ID, Categories: []domain.NoteCategory{domain.NoteDecision}, Tags: []string{"security", "parser"}, Pinned: &pinned,
	})
	if err != nil || len(listed) != 1 || listed[0].ID != note.ID {
		t.Fatalf("note filter failed: %#v err=%v", listed, err)
	}

	updated := loaded
	if err := updated.ApplyDetails(domain.NoteDetails{
		Title: "parser decision", Content: "Use strict decoding.", Category: domain.NoteDecision,
		Visibility: domain.NoteVisibilityRoot, Tags: []string{"parser"}, SourceRefs: []string{"docs/spec.md"},
		EvidenceIDs: []string{"evidence-1"}, Pinned: false,
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	updated.Version++
	if err := st.UpdateNote(ctx, updated, loaded.Version, newNoteChangedEvent(t, mission.ID, updated, loaded.Status)); err != nil {
		t.Fatal(err)
	}
	archived := updated
	if err := archived.Transition(domain.NoteArchived, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	archived.Version++
	if err := st.UpdateNote(ctx, archived, updated.Version, newNoteChangedEvent(t, mission.ID, archived, updated.Status)); err != nil {
		t.Fatal(err)
	}
	loaded, err = st.GetNote(ctx, note.ID)
	if err != nil || loaded.Status != domain.NoteArchived || loaded.ArchivedAt == nil || loaded.Version != 3 ||
		loaded.Visibility != domain.NoteVisibilityRoot || len(loaded.Tags) != 1 {
		t.Fatalf("unexpected archived note: %#v err=%v", loaded, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(timeline, events.NoteCreatedEvent) != 1 || countRunEventType(timeline, events.NoteChangedEvent) != 2 {
		t.Fatalf("note events missing: %#v err=%v", timeline, err)
	}
	for _, event := range timeline {
		if strings.Contains(event.PayloadJSON, testAPIKey) {
			t.Fatalf("note event leaked token-shaped content: %#v", event)
		}
	}
}

func TestSQLiteNoteOptimisticConcurrencyAllowsOneWriter(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "note concurrency")
	note := newNoteTest(run.ID, "shared note", "initial")
	if err := st.CreateNote(ctx, note, newNoteCreatedEvent(t, mission.ID, note)); err != nil {
		t.Fatal(err)
	}
	current, err := st.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	updates := []domain.Note{current, current}
	for index := range updates {
		if err := updates[index].ApplyDetails(domain.NoteDetails{
			Title: updates[index].Title, Content: "writer " + string(rune('A'+index)),
			Category: updates[index].Category, Visibility: updates[index].Visibility,
		}, time.Now().UTC().Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		updates[index].Version++
	}
	results := make(chan error, len(updates))
	var ready sync.WaitGroup
	ready.Add(len(updates))
	start := make(chan struct{})
	for _, update := range updates {
		update := update
		event := newNoteChangedEvent(t, mission.ID, update, current.Status)
		go func() {
			ready.Done()
			<-start
			results <- st.UpdateNote(ctx, update, current.Version, event)
		}()
	}
	ready.Wait()
	close(start)
	var successes int
	var conflicts int
	for range updates {
		err := <-results
		switch apperror.CodeOf(err) {
		case "":
			successes++
		case apperror.CodeConflict:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent note error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", successes, conflicts)
	}
	loaded, err := st.GetNote(ctx, note.ID)
	if err != nil || loaded.Version != 2 {
		t.Fatalf("unexpected winning note: %#v err=%v", loaded, err)
	}
}

func TestSQLiteNoteEventFailureRollsBackRelationsAndRecord(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "note rollback")
	note := newNoteTest(run.ID, "before", "before content")
	note.Tags = []string{"before"}
	createdEvent := newNoteCreatedEvent(t, mission.ID, note)
	if err := st.CreateNote(ctx, note, createdEvent); err != nil {
		t.Fatal(err)
	}
	current, err := st.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated := current
	if err := updated.ApplyDetails(domain.NoteDetails{
		Title: "after", Content: "after content", Category: domain.NoteSummary,
		Visibility: domain.NoteVisibilityRun, Tags: []string{"after"},
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	updated.Version++
	duplicate := newNoteChangedEvent(t, mission.ID, updated, current.Status)
	duplicate.EventID = createdEvent.EventID
	if err := st.UpdateNote(ctx, updated, current.Version, duplicate); err == nil {
		t.Fatal("expected duplicate note event failure")
	}
	loaded, err := st.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "before" || loaded.Content != "before content" || loaded.Version != 1 ||
		!slicesEqualStrings(loaded.Tags, []string{"before"}) {
		t.Fatalf("failed event append did not roll back note: %#v", loaded)
	}
}

func TestSQLiteNoteRelationForeignKeyRejectsCrossRunRows(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "note relation run")
	_, otherRun := createWorkItemTestRun(t, ctx, st, "other note relation run")
	note := newNoteTest(run.ID, "scoped note", "content")
	if err := st.CreateNote(ctx, note, newNoteCreatedEvent(t, mission.ID, note)); err != nil {
		t.Fatal(err)
	}
	_, err := st.db.ExecContext(ctx, `INSERT INTO note_tags (run_id, note_id, tag, created_at) VALUES (?, ?, ?, ?)`,
		otherRun.ID, note.ID, "cross-run", ts(time.Now().UTC()))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("expected cross-run note relation foreign key failure, got %v", err)
	}
}

func TestSQLiteNoteViewerFilterEnforcesVisibility(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "note visibility")
	create := func(title string, visibility domain.NoteVisibility, owner string) domain.Note {
		note := newNoteTest(run.ID, title, "content")
		note.Visibility = visibility
		note.Owner = owner
		if err := st.CreateNote(ctx, note, newNoteCreatedEvent(t, mission.ID, note)); err != nil {
			t.Fatal(err)
		}
		return note
	}
	runNote := create("run note", domain.NoteVisibilityRun, "")
	rootNote := create("root note", domain.NoteVisibilityRoot, "")
	rootOwned := create("root owned", domain.NoteVisibilityOwner, "root")
	specialistOwned := create("specialist owned", domain.NoteVisibilityOwner, "specialist")
	rootVisible, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID, Statuses: []domain.NoteStatus{domain.NoteActive}, Viewer: "root"})
	if err != nil || !noteIDsEqual(rootVisible, []string{rootNote.ID, rootOwned.ID, runNote.ID}) {
		t.Fatalf("unexpected root-visible notes: %#v err=%v", rootVisible, err)
	}
	specialistVisible, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID, Viewer: "specialist"})
	if err != nil || !noteIDsEqual(specialistVisible, []string{runNote.ID, specialistOwned.ID}) {
		t.Fatalf("unexpected specialist-visible notes: %#v err=%v", specialistVisible, err)
	}
	if _, err := st.ListNotes(ctx, domain.NoteFilter{
		RunID: run.ID, Viewer: "root", Visibilities: []domain.NoteVisibility{domain.NoteVisibilityRun},
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("expected conflicting viewer filter rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID, Limit: -1}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("expected negative limit rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func newNoteTest(runID string, title string, content string) domain.Note {
	now := time.Now().UTC()
	return domain.Note{
		ID: idgen.New("note"), RunID: runID, Title: title, Content: content,
		Category: domain.NoteObservation, Visibility: domain.NoteVisibilityRun, Status: domain.NoteActive,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func newNoteCreatedEvent(t *testing.T, missionID string, note domain.Note) events.Event {
	t.Helper()
	event, err := events.New(note.RunID, missionID, events.NoteCreatedEvent, "note_test", note.ID, map[string]any{
		"title": note.Title, "category": note.Category, "visibility": note.Visibility, "version": note.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func newNoteChangedEvent(t *testing.T, missionID string, note domain.Note, previous domain.NoteStatus) events.Event {
	t.Helper()
	event, err := events.New(note.RunID, missionID, events.NoteChangedEvent, "note_test", note.ID, map[string]any{
		"from": previous, "to": note.Status, "version": note.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func slicesEqualStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func noteIDsEqual(notes []domain.Note, expected []string) bool {
	actual := make([]string, len(notes))
	for index := range notes {
		actual[index] = notes[index].ID
	}
	if len(actual) != len(expected) {
		return false
	}
	for _, id := range expected {
		found := false
		for _, actualID := range actual {
			found = found || actualID == id
		}
		if !found {
			return false
		}
	}
	return true
}
