package domain

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWorkItemValidateAndNormalizeDetails(t *testing.T) {
	now := time.Now().UTC()
	details, err := NormalizeWorkItemDetails("work-1", WorkItemDetails{
		Title: "  implement parser  ", Priority: WorkItemPriorityHigh, Owner: " agent ",
		AcceptanceCriteria: []string{" tests pass ", "JSON output", "tests pass"},
		Dependencies:       []string{"work-3", " work-2 ", "work-3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := WorkItem{
		ID: "work-1", RunID: "run-1", Status: WorkItemPending, Version: 1,
		Title: details.Title, Priority: details.Priority, Owner: details.Owner,
		AcceptanceCriteria: details.AcceptanceCriteria, Dependencies: details.Dependencies,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	if item.Title != "implement parser" || item.Owner != "agent" ||
		!slices.Equal(item.Dependencies, []string{"work-2", "work-3"}) ||
		!slices.Equal(item.AcceptanceCriteria, []string{"JSON output", "tests pass"}) {
		t.Fatalf("details were not normalized: %#v", item)
	}
}

func TestWorkItemTransitions(t *testing.T) {
	now := time.Now().UTC()
	item := WorkItem{
		ID: "work-1", RunID: "run-1", Title: "implement", Status: WorkItemPending,
		Priority: WorkItemPriorityNormal, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := item.Transition(WorkItemBlocked, "waiting for fixture", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if item.Status != WorkItemBlocked || item.BlockedReason != "waiting for fixture" {
		t.Fatalf("unexpected blocked item: %#v", item)
	}
	if err := item.Transition(WorkItemInProgress, "", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if item.BlockedReason != "" || item.Status != WorkItemInProgress {
		t.Fatalf("unblock did not clear reason: %#v", item)
	}
	if err := item.Transition(WorkItemCompleted, "", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if item.CompletedAt == nil || item.Status != WorkItemCompleted {
		t.Fatalf("completion did not set timestamp: %#v", item)
	}
	if err := item.Transition(WorkItemPending, "", now.Add(4*time.Minute)); err == nil {
		t.Fatal("expected terminal transition rejection")
	}
}

func TestWorkItemRejectsInvalidState(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name string
		item WorkItem
	}{
		{
			name: "self dependency",
			item: WorkItem{ID: "work-1", RunID: "run-1", Title: "x", Status: WorkItemPending,
				Priority: WorkItemPriorityNormal, Dependencies: []string{"work-1"}, Version: 1, CreatedAt: now, UpdatedAt: now},
		},
		{
			name: "blocked without reason",
			item: WorkItem{ID: "work-1", RunID: "run-1", Title: "x", Status: WorkItemBlocked,
				Priority: WorkItemPriorityNormal, Version: 1, CreatedAt: now, UpdatedAt: now},
		},
		{
			name: "oversized title",
			item: WorkItem{ID: "work-1", RunID: "run-1", Title: strings.Repeat("x", MaxWorkItemTitleRunes+1), Status: WorkItemPending,
				Priority: WorkItemPriorityNormal, Version: 1, CreatedAt: now, UpdatedAt: now},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.item.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := (&WorkItem{Status: WorkItemPending}).Transition(WorkItemBlocked, "", now); err == nil {
		t.Fatal("expected missing blocked reason rejection")
	}
}

func TestWorkItemTerminalDetailsAreImmutable(t *testing.T) {
	now := time.Now().UTC()
	completed := now
	item := WorkItem{
		ID: "work-1", RunID: "run-1", Title: "done", Status: WorkItemCompleted,
		Priority: WorkItemPriorityNormal, Version: 2, CreatedAt: now, UpdatedAt: now, CompletedAt: &completed,
	}
	if err := item.ApplyDetails(WorkItemDetails{Title: "changed"}, now.Add(time.Minute)); err == nil {
		t.Fatal("expected terminal update rejection")
	}
}
