package application_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/agent"
	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/store"
)

func TestTaskAdapterIsIdempotentAndAuditable(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	task := agent.Task{
		ID: "task-legacy", Kind: agent.TaskScript, Goal: "build parser", WorkspaceID: "ws-demo",
		Mode: "python", Status: agent.StatusCompleted, CreatedAt: time.Now().UTC(),
	}
	if err := st.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	adapter := application.NewTaskAdapter(st)
	first, err := adapter.Adapt(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.Mission.Profile != domain.ProfileScript || first.Run.Status != domain.RunCreated || first.Run.SessionID == "" {
		t.Fatalf("unexpected adaptation: %#v", first)
	}
	items, err := st.ListRunEvents(ctx, first.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := []string{events.RunCreatedEvent, events.SessionAttachedEvent, events.LegacyTaskAdaptedEvent}
	if len(items) != len(wantTypes) {
		t.Fatalf("unexpected initial events: %#v", items)
	}
	for index, want := range wantTypes {
		if items[index].Sequence != int64(index+1) || items[index].Type != want {
			t.Fatalf("unexpected event at %d: %#v", index, items[index])
		}
	}

	second, err := adapter.Adapt(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.Run.ID != first.Run.ID || second.Mission.ID != first.Mission.ID || second.Link != first.Link {
		t.Fatalf("repeat adaptation was not idempotent first=%#v second=%#v", first, second)
	}
	after, err := st.ListRunEvents(ctx, first.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(items) {
		t.Fatalf("repeat adaptation appended events: %#v", after)
	}
}

func TestTaskAdapterConcurrentCallsCreateOneRun(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	task := agent.Task{
		ID: "task-concurrent", Kind: agent.TaskReview, Goal: "review concurrency", Status: agent.StatusPending, CreatedAt: time.Now().UTC(),
	}
	if err := st.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	adapter := application.NewTaskAdapter(st)
	const callers = 8
	results := make(chan application.AdaptTaskResult, callers)
	errorsCh := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := adapter.Adapt(ctx, task.ID)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent adaptation failed: %v", err)
	}
	var runID string
	createdCount := 0
	resultCount := 0
	for result := range results {
		resultCount++
		if result.Created {
			createdCount++
		}
		if runID == "" {
			runID = result.Run.ID
		} else if result.Run.ID != runID {
			t.Fatalf("concurrent adaptation returned multiple runs: %s and %s", runID, result.Run.ID)
		}
	}
	if resultCount != callers || createdCount != 1 {
		t.Fatalf("unexpected concurrent results count=%d created=%d", resultCount, createdCount)
	}
	runs, err := st.ListRuns(ctx, domain.RunFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("concurrent adaptation persisted duplicate runs: %#v", runs)
	}
}

func TestTaskAdapterRejectsUnsupportedLegacyKind(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	task := agent.Task{ID: "task-unknown", Kind: "unknown", Goal: "unknown", Status: agent.StatusPending, CreatedAt: time.Now().UTC()}
	if err := st.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	_, err = application.NewTaskAdapter(st).Adapt(context.Background(), task.ID)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("unexpected error code %s: %v", apperror.CodeOf(err), err)
	}
}
