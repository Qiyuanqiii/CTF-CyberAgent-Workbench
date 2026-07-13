package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/toolrun"
)

func TestModelPollsDurableRunEventsAndStopsAtTerminalState(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runService := application.NewRunService(st)
	_, run, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "refresh TUI from durable events", Profile: "code",
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runService.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	model := newRunEventTestModel(t, st, run.SessionID)
	initial, found := model.CurrentRunProjection()
	if !found || initial.AgentCount != 1 || initial.EventSequence <= 0 {
		t.Fatalf("initial TUI projection is incomplete: %#v found=%t", initial, found)
	}
	if _, err := application.NewWorkItemService(st).Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "event-driven refresh", Priority: "high",
	}); err != nil {
		t.Fatal(err)
	}

	tick := runEventPollTickMsg{runID: initial.RunID, missionID: initial.MissionID,
		afterSequence: initial.EventSequence}
	message := model.pollRunEventsCmd(tick)()
	result, ok := message.(runEventPollResultMsg)
	if !ok || result.err != nil || !result.changed || len(result.events) == 0 {
		t.Fatalf("event poll did not produce a projection refresh: %#v", message)
	}
	updated, next := model.Update(message)
	model = updated.(*Model)
	projection, _ := model.CurrentRunProjection()
	if next == nil || projection.EventSequence <= initial.EventSequence ||
		len(model.runContext.WorkItems) != 1 || !hasTUIEventType(model.runContext.Events,
		events.WorkItemCreatedEvent) {
		t.Fatalf("event refresh drifted: projection=%#v work=%#v events=%#v next=%#v",
			projection, model.runContext.WorkItems, model.runContext.Events, next)
	}

	if _, err := runService.Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	tick = runEventPollTickMsg{runID: projection.RunID, missionID: projection.MissionID,
		afterSequence: projection.EventSequence}
	updated, next = model.Update(model.pollRunEventsCmd(tick)())
	model = updated.(*Model)
	projection, _ = model.CurrentRunProjection()
	if next != nil || projection.Status != domain.RunCancelled || !model.runContext.Run.Terminal() {
		t.Fatalf("terminal event stream did not settle: projection=%#v next=%#v", projection, next)
	}
}

func hasTUIEventType(values []events.Event, eventType string) bool {
	for _, value := range values {
		if value.Type == eventType {
			return true
		}
	}
	return false
}

func TestModelDiscardsStaleRunEventPollResult(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-stale-events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "reject stale TUI event results", Profile: "review",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	model := newRunEventTestModel(t, st, run.SessionID)
	initial, _ := model.CurrentRunProjection()
	if _, err := application.NewNoteService(st).Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "new state", Content: "must not be rolled back",
		Category: "observation", Visibility: "root",
	}); err != nil {
		t.Fatal(err)
	}
	tick := runEventPollTickMsg{runID: initial.RunID, missionID: initial.MissionID,
		afterSequence: initial.EventSequence}
	stale := model.pollRunEventsCmd(tick)()
	if err := model.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	before, _ := model.CurrentRunProjection()
	updated, next := model.Update(stale)
	model = updated.(*Model)
	after, _ := model.CurrentRunProjection()
	if next == nil || before != after || len(model.runContext.Notes) != 1 {
		t.Fatalf("stale poll result changed current state: before=%#v after=%#v notes=%#v",
			before, after, model.runContext.Notes)
	}
}

func TestModelPollUsesStableSnapshotBeyondOneEventBatch(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-event-batch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "keep TUI tables aligned with the durable event tail", Profile: "review",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	model := newRunEventTestModel(t, st, run.SessionID)
	initial, _ := model.CurrentRunProjection()
	workItems := application.NewWorkItemService(st)
	for index := 0; index < tuiEventPollBatchSize+8; index++ {
		if _, err := workItems.Create(ctx, application.CreateWorkItemRequest{
			RunID: run.ID, Title: fmt.Sprintf("event batch item %02d", index),
			Priority: "normal",
		}); err != nil {
			t.Fatal(err)
		}
	}
	tick := runEventPollTickMsg{runID: initial.RunID, missionID: initial.MissionID,
		afterSequence: initial.EventSequence}
	message := model.pollRunEventsCmd(tick)()
	result, ok := message.(runEventPollResultMsg)
	if !ok || result.err != nil || len(result.events) != tuiEventPollBatchSize ||
		result.result.runContext.EventSequence <= result.events[len(result.events)-1].Sequence {
		t.Fatalf("test did not cross the event batch boundary: %#v", message)
	}
	updated, next := model.Update(message)
	model = updated.(*Model)
	projection, found := model.CurrentRunProjection()
	if !found || next == nil || projection.EventSequence != result.result.runContext.EventSequence ||
		len(model.runContext.WorkItems) != tuiEventPollBatchSize+8 ||
		model.runContext.Events[len(model.runContext.Events)-1].Sequence != projection.EventSequence {
		t.Fatalf("TUI applied a torn post-batch snapshot: projection=%#v result_tail=%d work=%d",
			projection, result.result.runContext.EventSequence, len(model.runContext.WorkItems))
	}
}

func TestModelRetriesWhenProjectionChangesMidSnapshot(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-snapshot-retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "retry a changing TUI snapshot", Profile: "code",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	hooked := &midSnapshotTUIStore{SQLiteStore: st}
	hooked.mutate = func() error {
		_, err := application.NewWorkItemService(st).Create(ctx, application.CreateWorkItemRequest{
			RunID: run.ID, Title: "committed during snapshot", Priority: "normal",
		})
		return err
	}
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	model, err := NewModel(ctx, sess, manager,
		toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns(), hooked)
	if err != nil {
		t.Fatal(err)
	}
	projection, found := model.CurrentRunProjection()
	latest, tailErr := st.LatestRunEventSequence(ctx, run.ID)
	if !found || tailErr != nil || len(model.runContext.WorkItems) != 1 ||
		projection.EventSequence != latest || hooked.listCalls < 2 {
		t.Fatalf("bounded snapshot retry failed: projection=%#v work=%#v latest=%d tailErr=%v calls=%d",
			projection, model.runContext.WorkItems, latest, tailErr, hooked.listCalls)
	}
}

func TestTUIEventValidationRejectsCrossScopeAndSequenceGap(t *testing.T) {
	run := domain.Run{ID: "run-one", MissionID: "mission-one"}
	first, err := events.New(run.ID, run.MissionID, events.RunCreatedEvent,
		"test", run.ID, map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	first.Sequence = 2
	if err := validateTUIEventBatch(run, 0, []events.Event{first}); err == nil {
		t.Fatal("TUI accepted a sequence gap")
	}
	first.Sequence = 1
	first.MissionID = "mission-two"
	if err := validateTUIEventBatch(run, 0, []events.Event{first}); err == nil {
		t.Fatal("TUI accepted a cross-Mission event")
	}
	first.MissionID = run.MissionID
	first.Type = strings.Repeat("x", maxTUIEventLabelRunes+1)
	if err := validateTUIEventBatch(run, 0, []events.Event{first}); err == nil {
		t.Fatal("TUI accepted oversized event metadata")
	}
}

func TestReadOnlyActivityViewsCannotApproveTools(t *testing.T) {
	model := &Model{activityView: activityAgents, focus: focusTools,
		toolRuns: []toolrun.ToolRun{{ID: "tool-one", Status: toolrun.StatusProposed}}}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(*Model)
	if command != nil || !strings.Contains(model.status, "read-only") {
		t.Fatalf("read-only activity triggered a tool action: command=%#v status=%q",
			command, model.status)
	}
}

func newRunEventTestModel(t *testing.T, st *store.SQLiteStore,
	sessionID string,
) *Model {
	t.Helper()
	ctx := context.Background()
	sess, err := st.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	model, err := NewModel(ctx, sess, sessionManager,
		toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns(), st)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

type midSnapshotTUIStore struct {
	*store.SQLiteStore
	once      sync.Once
	mutate    func() error
	mutateErr error
	listCalls int
}

func (s *midSnapshotTUIStore) ListWorkItems(ctx context.Context,
	filter domain.WorkItemFilter,
) ([]domain.WorkItem, error) {
	items, err := s.SQLiteStore.ListWorkItems(ctx, filter)
	s.listCalls++
	if err != nil {
		return nil, err
	}
	s.once.Do(func() {
		if s.mutate != nil {
			s.mutateErr = s.mutate()
		}
	})
	if s.mutateErr != nil {
		return nil, s.mutateErr
	}
	return items, nil
}
