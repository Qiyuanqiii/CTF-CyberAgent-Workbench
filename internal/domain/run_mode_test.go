package domain_test

import (
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestRunModeSnapshotDefaultsAndImmutableTransition(t *testing.T) {
	now := time.Now().UTC()
	mission := domain.Mission{
		ID: "mission-mode", Goal: "plan a parser", Profile: domain.ProfileCode,
		WorkspaceID: "workspace-mode",
		Scope: domain.Scope{WorkspaceID: "workspace-mode", NetworkMode: "allowlist",
			AllowedTargets: []string{"lab.example"}},
		CreatedAt: now, UpdatedAt: now,
	}
	run := domain.Run{
		ID: "run-mode", MissionID: mission.ID, SessionID: "session-mode",
		Status: domain.RunCreated, Config: domain.RunConfig{ModelRoute: "code"},
		Budget: domain.DefaultBudget(), CreatedAt: now, UpdatedAt: now,
	}
	initial, err := domain.NewInitialRunModeSnapshot("snapshot-1", run, mission, "", "",
		"operator", "initial mode", now)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Surface != domain.ExecutionSurfaceCode ||
		initial.Phase != domain.ExecutionPhaseDeliver || initial.Revision != 1 {
		t.Fatalf("unexpected defaults: %#v", initial)
	}
	next, err := initial.Next("snapshot-2", domain.ExecutionPhasePlan,
		"operator", "review first", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != 2 || next.Phase != domain.ExecutionPhasePlan ||
		!next.SamePolicy(initial) {
		t.Fatalf("unexpected next snapshot: %#v", next)
	}
	next.Scope.AllowedTargets[0] = "changed.example"
	if initial.Scope.AllowedTargets[0] != "lab.example" {
		t.Fatal("run mode transition aliased the immutable scope")
	}
	if _, err := initial.Next("snapshot-2", initial.Phase, "operator", "same", now); err == nil {
		t.Fatal("same-phase transition was accepted")
	}
}

func TestRunModeValidationRejectsMalformedBoundaries(t *testing.T) {
	for _, value := range []string{"", "work"} {
		if _, err := domain.ParseExecutionSurface(value); err == nil {
			t.Fatalf("surface %q was accepted", value)
		}
	}
	for _, value := range []string{"", "execute"} {
		if _, err := domain.ParseExecutionPhase(value); err == nil {
			t.Fatalf("phase %q was accepted", value)
		}
	}
	if domain.ExecutionSurface(" code ").Valid() || domain.ExecutionPhase(" plan ").Valid() {
		t.Fatal("unnormalized stored mode enum was accepted")
	}
	now := time.Now().UTC()
	bad := domain.RunModeSnapshot{
		ID: "snapshot", RunID: "run", MissionID: "mission", Revision: 1,
		ProtocolVersion: domain.RunModeProtocolVersion,
		Surface:         domain.ExecutionSurfaceCode, Phase: domain.ExecutionPhasePlan,
		Profile: domain.ProfileCode, Scope: domain.Scope{NetworkMode: "disabled"},
		PolicyVersion: domain.RunModePolicyVersion, RequestedBy: "operator",
		Reason: strings.Repeat("x", domain.MaxRunModeReasonRunes+1), CreatedAt: now,
	}
	if err := bad.Validate(); err == nil {
		t.Fatal("oversized mode reason was accepted")
	}
	bad.Reason = "valid"
	bad.Scope.AllowedTargets = []string{" target "}
	if err := bad.Validate(); err == nil {
		t.Fatal("unnormalized target was accepted")
	}
	if !domain.CanChangeRunPhase(domain.RunCreated) ||
		!domain.CanChangeRunPhase(domain.RunPaused) ||
		domain.CanChangeRunPhase(domain.RunRunning) ||
		domain.CanChangeRunPhase(domain.RunCompleted) {
		t.Fatal("run phase status boundary is inconsistent")
	}
}
