package application_test

import (
	"context"
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

func TestWorkItemServiceCreateUpdateAndTransitions(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runService := application.NewRunService(st)
	_, run, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "service work board", Profile: "code", Budget: domain.Budget{MaxTurns: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewWorkItemService(st)
	testAPIKey := "s" + "k-" + strings.Repeat("a", 26)
	dependency, err := service.Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "prepare fixtures", Priority: "high", Owner: "planner",
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "implement " + testAPIKey, Description: "initial",
		Priority: "critical", Owner: "coder", AcceptanceCriteria: []string{"unit tests", "lint clean"},
		Dependencies: []string{dependency.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "implement [REDACTED:api-key]" || item.Version != 1 ||
		!slices.Equal(item.AcceptanceCriteria, []string{"lint clean", "unit tests"}) {
		t.Fatalf("unexpected created item: %#v", item)
	}
	description := "updated description"
	owner := ""
	priority := "normal"
	criteria := []string{"all tests pass"}
	updated, err := service.Update(ctx, application.UpdateWorkItemRequest{
		ID: item.ID, ExpectedVersion: item.Version, Description: &description, Owner: &owner,
		Priority: &priority, AcceptanceCriteria: &criteria,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Description != description || updated.Owner != "" ||
		updated.Priority != domain.WorkItemPriorityNormal || !slices.Equal(updated.AcceptanceCriteria, criteria) {
		t.Fatalf("unexpected updated item: %#v", updated)
	}
	if _, err := service.Update(ctx, application.UpdateWorkItemRequest{ID: item.ID, ExpectedVersion: 1, Owner: &owner}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("expected stale service update conflict, code=%s err=%v", apperror.CodeOf(err), err)
	}
	blocked, err := service.Transition(ctx, updated.ID, 0, domain.WorkItemBlocked, "waiting for fixture completion")
	if err != nil || blocked.Status != domain.WorkItemBlocked || blocked.Version != 3 {
		t.Fatalf("unexpected blocked item: %#v err=%v", blocked, err)
	}
	if _, err := service.Transition(ctx, blocked.ID, 0, domain.WorkItemCompleted, ""); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected blocked completion rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
	reopened, err := service.Transition(ctx, blocked.ID, 0, domain.WorkItemPending, "")
	if err != nil || reopened.BlockedReason != "" || reopened.Version != 4 {
		t.Fatalf("unexpected reopened item: %#v err=%v", reopened, err)
	}
	if _, err := service.Transition(ctx, reopened.ID, 0, domain.WorkItemInProgress, ""); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected dependency-gated start rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
	completedDependency, err := service.Transition(ctx, dependency.ID, 0, domain.WorkItemCompleted, "")
	if err != nil || completedDependency.CompletedAt == nil {
		t.Fatalf("unexpected completed dependency: %#v err=%v", completedDependency, err)
	}
	started, err := service.Transition(ctx, reopened.ID, 0, domain.WorkItemInProgress, "")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.Transition(ctx, started.ID, 0, domain.WorkItemCompleted, "")
	if err != nil || completed.CompletedAt == nil || completed.Version != 6 {
		t.Fatalf("unexpected completed item: %#v err=%v", completed, err)
	}
	if _, err := service.Update(ctx, application.UpdateWorkItemRequest{ID: completed.ID, Title: stringPointer("changed")}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected terminal item update rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}

	listed, err := service.List(ctx, domain.WorkItemFilter{RunID: run.ID, Statuses: []domain.WorkItemStatus{domain.WorkItemCompleted}})
	if err != nil || len(listed) != 2 {
		t.Fatalf("unexpected completed list: %#v err=%v", listed, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countApplicationEvent(timeline, events.WorkItemCreatedEvent) != 2 || countApplicationEvent(timeline, events.WorkItemChangedEvent) != 6 {
		t.Fatalf("unexpected work board timeline: %#v", timeline)
	}
}

func TestWorkItemServiceRejectsTerminalRunMutation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "closed board", Profile: "review", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Complete(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = application.NewWorkItemService(st).Create(ctx, application.CreateWorkItemRequest{RunID: run.ID, Title: "too late"})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected terminal run rejection, code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func stringPointer(value string) *string {
	return &value
}

func countApplicationEvent(items []events.Event, eventType string) int {
	count := 0
	for _, item := range items {
		if item.Type == eventType {
			count++
		}
	}
	return count
}
