package domain

import (
	"strings"
	"testing"
	"time"
)

func validRun() Run {
	now := time.Now().UTC()
	return Run{
		ID: "run-test", MissionID: "mission-test", Status: RunCreated,
		Config: RunConfig{ModelRoute: "code"}, Budget: DefaultBudget(),
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestRunTransitionLifecycle(t *testing.T) {
	run := validRun()
	if err := run.Transition(RunPreparing, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(RunRunning, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if run.StartedAt == nil {
		t.Fatal("expected started timestamp")
	}
	if err := run.Transition(RunPaused, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(RunRunning, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(RunCompleted, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if !run.Terminal() || run.FinishedAt == nil {
		t.Fatalf("expected terminal run: %#v", run)
	}
}

func TestRunRejectsIllegalTransition(t *testing.T) {
	run := validRun()
	err := run.Transition(RunCompleted, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "created to completed") {
		t.Fatalf("expected illegal transition error, got %v", err)
	}
}

func TestScopeRejectsUnrestrictedNetwork(t *testing.T) {
	scope := DefaultScope("ws-demo")
	scope.NetworkMode = "unrestricted"
	if err := scope.Validate(); err == nil {
		t.Fatal("expected unrestricted network to be rejected")
	}
}

func TestBudgetRejectsTimeoutDurationOverflow(t *testing.T) {
	budget := DefaultBudget()
	budget.TimeoutSeconds = int64((1<<63-1)/int64(time.Second)) + 1
	if err := budget.Validate(); err == nil {
		t.Fatal("expected oversized timeout to be rejected")
	}
}

func TestBudgetRejectsNegativeToolCallLimit(t *testing.T) {
	budget := DefaultBudget()
	budget.MaxToolCalls = -1
	if err := budget.Validate(); err == nil {
		t.Fatal("expected negative tool-call budget to be rejected")
	}
}
