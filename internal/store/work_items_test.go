package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
)

func TestSQLiteWorkItemsPersistDependenciesAndTransactionalEvents(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "work board persistence")
	testAPIKey := "s" + "k-" + strings.Repeat("a", 26)

	dependency, dependencyEvent := createWorkItemTestItem(t, ctx, st, mission.ID, run.ID, "collect fixtures", nil)
	item, _ := createWorkItemTestItem(t, ctx, st, mission.ID, run.ID,
		"parse "+testAPIKey, []string{dependency.ID})
	loaded, err := st.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "parse [REDACTED:api-key]" || len(loaded.Dependencies) != 1 || loaded.Dependencies[0] != dependency.ID {
		t.Fatalf("unexpected stored work item: %#v", loaded)
	}

	listed, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected two work items, got %#v", listed)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countRunEventType(items, events.WorkItemCreatedEvent) != 2 ||
		strings.Contains(items[len(items)-1].PayloadJSON, testAPIKey) {
		t.Fatalf("work item events were not appended and redacted: %#v", items)
	}

	blockedStart := loaded
	if err := blockedStart.Transition(domain.WorkItemInProgress, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	blockedStart.Version++
	blockedEvent := newWorkItemChangedEvent(t, mission.ID, blockedStart, domain.WorkItemPending)
	if err := st.UpdateWorkItem(ctx, blockedStart, loaded.Version, blockedEvent); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected incomplete dependency rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}

	completedDependency := dependency
	if err := completedDependency.Transition(domain.WorkItemCompleted, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	completedDependency.Version++
	if err := st.UpdateWorkItem(ctx, completedDependency, dependency.Version,
		newWorkItemChangedEvent(t, mission.ID, completedDependency, dependency.Status)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateWorkItem(ctx, blockedStart, loaded.Version, blockedEvent); err != nil {
		t.Fatal(err)
	}
	started, err := st.GetWorkItem(ctx, item.ID)
	if err != nil || started.Status != domain.WorkItemInProgress || started.Version != 2 {
		t.Fatalf("dependency-gated start failed: %#v err=%v", started, err)
	}

	if dependencyEvent.Type != events.WorkItemCreatedEvent {
		t.Fatalf("test fixture event changed unexpectedly: %#v", dependencyEvent)
	}
	items, err = st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(items, events.WorkItemChangedEvent) != 2 {
		t.Fatalf("work item changes missing from timeline: %#v err=%v", items, err)
	}
}

func TestSQLiteWorkItemsRejectMissingCrossRunAndCyclicDependencies(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "dependency graph")
	otherMission, otherRun := createWorkItemTestRun(t, ctx, st, "other dependency graph")
	a, _ := createWorkItemTestItem(t, ctx, st, mission.ID, run.ID, "A", nil)
	b, _ := createWorkItemTestItem(t, ctx, st, mission.ID, run.ID, "B", []string{a.ID})

	missing := newWorkItemTestItem(run.ID, "missing", []string{"work-does-not-exist"})
	if err := st.CreateWorkItem(ctx, missing, newWorkItemCreatedEvent(t, mission.ID, missing)); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected missing dependency rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
	crossRun := newWorkItemTestItem(otherRun.ID, "cross run", []string{a.ID})
	if err := st.CreateWorkItem(ctx, crossRun, newWorkItemCreatedEvent(t, otherMission.ID, crossRun)); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected cross-run dependency rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}

	cycle := a
	if err := cycle.ApplyDetails(domain.WorkItemDetails{
		Title: cycle.Title, Description: cycle.Description, Priority: cycle.Priority, Owner: cycle.Owner,
		AcceptanceCriteria: cycle.AcceptanceCriteria, Dependencies: []string{b.ID},
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	cycle.Version++
	if err := st.UpdateWorkItem(ctx, cycle, a.Version,
		newWorkItemChangedEvent(t, mission.ID, cycle, a.Status)); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected cycle rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
	loaded, err := st.GetWorkItem(ctx, a.ID)
	if err != nil || len(loaded.Dependencies) != 0 || loaded.Version != 1 {
		t.Fatalf("cycle rejection changed the graph: %#v err=%v", loaded, err)
	}
}

func TestSQLiteAgentOwnedMemoryEnforcesRunAndViewerBoundaries(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "agent-owned memory")
	started, err := application.NewRunService(st).Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	run = started
	root, _, err := st.RegisterRootAgent(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	child, replayed, err := st.AdmitSpecialist(ctx, domain.SpecialistAdmission{
		AgentID: idgen.New("agent"), SessionID: idgen.New("sess"), RunID: run.ID,
		ParentAgentID: root.ID, Title: "memory specialist",
		Skills: []string{"note_create", "work_item_create"}, TurnLimit: 2, TokenLimit: 64,
		MaxChildren: 2, CreatedAt: time.Now().UTC(),
	}, "agent-memory-admission")
	if err != nil || replayed {
		t.Fatalf("specialist admission failed: child=%#v replayed=%t err=%v", child, replayed, err)
	}

	workService := application.NewWorkItemService(st)
	item, err := workService.Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "specialist-owned work", OwnerAgentID: child.ID,
	})
	if err != nil || item.OwnerAgentID != child.ID {
		t.Fatalf("Agent-owned WorkItem was not persisted: %#v err=%v", item, err)
	}
	filteredWork, err := st.ListWorkItems(ctx, domain.WorkItemFilter{
		RunID: run.ID, OwnerAgentID: child.ID,
	})
	if err != nil || len(filteredWork) != 1 || filteredWork[0].ID != item.ID {
		t.Fatalf("Agent-owned WorkItem filter failed: %#v err=%v", filteredWork, err)
	}

	noteService := application.NewNoteService(st)
	childOnly, err := noteService.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "private specialist note", Content: "child context",
		Visibility: string(domain.NoteVisibilityOwner), OwnerAgentID: child.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	runVisible, err := noteService.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "shared specialist note", Content: "run context",
		Visibility: string(domain.NoteVisibilityRun), OwnerAgentID: child.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyRoot, err := noteService.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "legacy root note", Content: "legacy context",
		Visibility: string(domain.NoteVisibilityOwner), Owner: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	rootVisible, err := st.ListNotes(ctx, domain.NoteFilter{
		RunID: run.ID, Viewer: "root", ViewerAgentID: root.ID,
	})
	if err != nil || !noteIDsEqual(rootVisible, []string{runVisible.ID, legacyRoot.ID}) {
		t.Fatalf("root Agent crossed specialist owner visibility: %#v err=%v", rootVisible, err)
	}
	childVisible, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID, ViewerAgentID: child.ID})
	if err != nil || !noteIDsEqual(childVisible, []string{childOnly.ID, runVisible.ID}) {
		t.Fatalf("specialist Agent visibility is incomplete: %#v err=%v", childVisible, err)
	}
	ownedNotes, err := st.ListNotes(ctx, domain.NoteFilter{RunID: run.ID, OwnerAgentID: child.ID})
	if err != nil || !noteIDsEqual(ownedNotes, []string{childOnly.ID, runVisible.ID}) {
		t.Fatalf("Agent-owned Note filter failed: %#v err=%v", ownedNotes, err)
	}
	missingAgentID := idgen.New("agent")
	if _, err := workService.Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "missing Agent work", OwnerAgentID: missingAgentID,
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("missing WorkItem owner returned code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := noteService.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "missing Agent note", Content: "must fail", OwnerAgentID: missingAgentID,
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("missing Note owner returned code=%s err=%v", apperror.CodeOf(err), err)
	}
	rootVisibility := string(domain.NoteVisibilityRoot)
	sharedPrivate, err := noteService.Update(ctx, application.UpdateNoteRequest{
		ID: childOnly.ID, ExpectedVersion: childOnly.Version, Visibility: &rootVisibility,
	})
	if err != nil || sharedPrivate.Visibility != domain.NoteVisibilityRoot || sharedPrivate.Owner != "" ||
		sharedPrivate.OwnerAgentID != child.ID {
		t.Fatalf("Note visibility change lost authoritative Agent ownership: %#v err=%v", sharedPrivate, err)
	}

	_, otherRun := createWorkItemTestRun(t, ctx, st, "other Agent-owned memory")
	otherRoot, _, err := st.RegisterRootAgent(ctx, otherRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workService.Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "cross-run work", OwnerAgentID: otherRoot.ID,
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("cross-Run WorkItem owner returned code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := noteService.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "cross-run note", Content: "must fail", OwnerAgentID: otherRoot.ID,
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("cross-Run Note owner returned code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := st.ListNotes(ctx, domain.NoteFilter{
		RunID: run.ID, ViewerAgentID: otherRoot.ID,
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("cross-Run Note viewer returned code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE work_items SET owner_agent_id = ? WHERE id = ?`,
		otherRoot.ID, item.ID); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("WorkItem ownership trigger did not reject cross-Run Agent: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE notes SET owner_agent_id = ? WHERE id = ?`,
		otherRoot.ID, childOnly.ID); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("Note ownership trigger did not reject cross-Run Agent: %v", err)
	}

	rootID := root.ID
	updated, err := workService.Update(ctx, application.UpdateWorkItemRequest{
		ID: item.ID, ExpectedVersion: item.Version, OwnerAgentID: &rootID,
	})
	if err != nil || updated.OwnerAgentID != root.ID || updated.Version != item.Version+1 {
		t.Fatalf("WorkItem Agent reassignment failed: %#v err=%v", updated, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var workOwnerAudited bool
	var noteOwnerAudited bool
	for _, event := range timeline {
		workOwnerAudited = workOwnerAudited || (event.Type == events.WorkItemCreatedEvent &&
			strings.Contains(event.PayloadJSON, child.ID))
		noteOwnerAudited = noteOwnerAudited || (event.Type == events.NoteCreatedEvent &&
			strings.Contains(event.PayloadJSON, child.ID))
	}
	if !workOwnerAudited || !noteOwnerAudited {
		t.Fatalf("Agent memory ownership was not audited: work=%t note=%t mission=%s",
			workOwnerAudited, noteOwnerAudited, mission.ID)
	}
	if _, err := application.NewRunService(st).Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE work_items SET owner_agent_id = ? WHERE id = ?`,
		child.ID, item.ID); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("WorkItem trigger accepted new terminal Agent ownership: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE notes SET owner_agent_id = ? WHERE id = ?`,
		root.ID, childOnly.ID); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("Note trigger accepted new terminal Agent ownership: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE notes SET owner_agent_id = owner_agent_id WHERE id = ?`,
		childOnly.ID); err != nil {
		t.Fatalf("Note trigger rejected unchanged terminal Agent ownership: %v", err)
	}
}

func TestSQLiteWorkItemOptimisticConcurrencyAllowsOneWriter(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "optimistic work board")
	item, _ := createWorkItemTestItem(t, ctx, st, mission.ID, run.ID, "shared", nil)

	updates := []domain.WorkItem{item, item}
	for index := range updates {
		details := domain.WorkItemDetails{Title: "writer " + string(rune('A'+index)), Priority: domain.WorkItemPriorityNormal}
		if err := updates[index].ApplyDetails(details, time.Now().UTC().Add(time.Duration(index)*time.Millisecond)); err != nil {
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
		go func() {
			ready.Done()
			<-start
			results <- st.UpdateWorkItem(ctx, update, item.Version,
				newWorkItemChangedEvent(t, mission.ID, update, item.Status))
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
			t.Fatalf("unexpected concurrent update error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", successes, conflicts)
	}
	loaded, err := st.GetWorkItem(ctx, item.ID)
	if err != nil || loaded.Version != 2 {
		t.Fatalf("unexpected winning update: %#v err=%v", loaded, err)
	}
}

func TestSQLiteWorkItemEventFailureRollsBackUpdate(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "atomic work board")
	item, createdEvent := createWorkItemTestItem(t, ctx, st, mission.ID, run.ID, "before", nil)
	updated := item
	if err := updated.ApplyDetails(domain.WorkItemDetails{Title: "after", Priority: domain.WorkItemPriorityHigh}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	updated.Version++
	duplicate := newWorkItemChangedEvent(t, mission.ID, updated, item.Status)
	duplicate.EventID = createdEvent.EventID
	if err := st.UpdateWorkItem(ctx, updated, item.Version, duplicate); err == nil {
		t.Fatal("expected duplicate event failure")
	}
	loaded, err := st.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "before" || loaded.Priority != domain.WorkItemPriorityNormal || loaded.Version != 1 {
		t.Fatalf("failed event append did not roll back update: %#v", loaded)
	}
}

func openWorkItemTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func createWorkItemTestRun(t *testing.T, ctx context.Context, st *SQLiteStore, goal string) (domain.Mission, domain.Run) {
	t.Helper()
	mission, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: goal, Profile: "code", Budget: domain.Budget{MaxTurns: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mission, run
}

func newWorkItemTestItem(runID string, title string, dependencies []string) domain.WorkItem {
	now := time.Now().UTC()
	return domain.WorkItem{
		ID:    idgen.New("work"),
		RunID: runID, Title: strings.TrimSpace(title), Status: domain.WorkItemPending,
		Priority: domain.WorkItemPriorityNormal, Dependencies: dependencies, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
}

func createWorkItemTestItem(t *testing.T, ctx context.Context, st *SQLiteStore, missionID string, runID string, title string, dependencies []string) (domain.WorkItem, events.Event) {
	t.Helper()
	item := newWorkItemTestItem(runID, title, dependencies)
	event := newWorkItemCreatedEvent(t, missionID, item)
	if err := st.CreateWorkItem(ctx, item, event); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded, event
}

func newWorkItemCreatedEvent(t *testing.T, missionID string, item domain.WorkItem) events.Event {
	t.Helper()
	event, err := events.New(item.RunID, missionID, events.WorkItemCreatedEvent, "work_item_test", item.ID, map[string]any{
		"title": item.Title, "status": item.Status, "priority": item.Priority, "dependencies": item.Dependencies,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func newWorkItemChangedEvent(t *testing.T, missionID string, item domain.WorkItem, previous domain.WorkItemStatus) events.Event {
	t.Helper()
	event, err := events.New(item.RunID, missionID, events.WorkItemChangedEvent, "work_item_test", item.ID, map[string]any{
		"from": previous, "to": item.Status, "version": item.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func countRunEventType(items []events.Event, eventType string) int {
	count := 0
	for _, item := range items {
		if item.Type == eventType {
			count++
		}
	}
	return count
}

func TestSQLiteWorkItemDependencyForeignKeyRejectsCrossRunRows(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, run := createWorkItemTestRun(t, ctx, st, "first raw graph")
	otherMission, otherRun := createWorkItemTestRun(t, ctx, st, "second raw graph")
	item, _ := createWorkItemTestItem(t, ctx, st, mission.ID, run.ID, "first raw item", nil)
	other, _ := createWorkItemTestItem(t, ctx, st, otherMission.ID, otherRun.ID, "second raw item", nil)
	_, err := st.db.ExecContext(ctx, `INSERT INTO work_item_dependencies
		(run_id, work_item_id, depends_on_id, created_at) VALUES (?, ?, ?, ?)`,
		run.ID, item.ID, other.ID, ts(time.Now().UTC()))
	if err == nil {
		t.Fatal("expected composite foreign key to reject a cross-run dependency")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("unexpected cross-run constraint error: %v", err)
	}
}
