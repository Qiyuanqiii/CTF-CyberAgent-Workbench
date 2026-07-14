package skills

import (
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestSpecialistContextMinimizesCodeSelectionAndRejectsTampering(t *testing.T) {
	registry, parent, mode, child, attempt := specialistContextFixture(t,
		domain.ProfileCode, domain.ExecutionSurfaceCode, []string{"plan-delivery", "code"})
	assembly, err := registry.AssembleSpecialistContext(parent, mode, child, attempt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if assembly.ItemCount != 1 || len(assembly.Items) != 1 ||
		assembly.Items[0].Name != "code" ||
		!strings.Contains(assembly.Items[0].Content, "Code workflow") ||
		strings.Contains(assembly.Items[0].Content, "Plan/Delivery workflow") ||
		assembly.TokenBudget != DefaultSpecialistContextTokenBudget ||
		assembly.TokenUpperBound <= 0 ||
		assembly.AssignmentFingerprint != SpecialistAssignmentFingerprint(child) {
		t.Fatalf("minimal Code Specialist context drifted: %#v", assembly)
	}
	if len(parent.Items) != 2 || parent.Items[0].Ordinal != 1 || parent.Items[1].Ordinal != 2 {
		t.Fatalf("parent selection was mutated: %#v", parent.Items)
	}
	request := assembly.Preparation()
	if err := request.Validate(); err != nil || request.ContextFingerprint != assembly.Fingerprint ||
		strings.Contains(request.ContextFingerprint, assembly.Items[0].Content) {
		t.Fatalf("metadata-only preparation drifted: %#v err=%v", request, err)
	}

	tampered := assembly
	tampered.Items = append([]ContextItem(nil), assembly.Items...)
	content := []byte(tampered.Items[0].Content)
	content[0] = 'X'
	tampered.Items[0].Content = string(content)
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "item is invalid") {
		t.Fatalf("tampered Specialist Skill body was accepted: %v", err)
	}
}

func TestSpecialistContextSeparatesCyberAndCodeCatalogs(t *testing.T) {
	registry, parent, mode, child, attempt := specialistContextFixture(t,
		domain.ProfileCode, domain.ExecutionSurfaceCyber, []string{"plan-delivery", "code"})
	cyberCode, err := registry.AssembleSpecialistContext(parent, mode, child, attempt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cyberCode.ItemCount != 0 || len(cyberCode.Items) != 0 ||
		cyberCode.TokenUpperBound != 0 || cyberCode.RedactionCount != 0 {
		t.Fatalf("Cyber surface received broad Code guidance: %#v", cyberCode)
	}

	registry, parent, mode, child, attempt = specialistContextFixture(t,
		domain.ProfileScript, domain.ExecutionSurfaceCyber,
		[]string{"plan-delivery", "script"})
	cyberScript, err := registry.AssembleSpecialistContext(parent, mode, child, attempt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cyberScript.ItemCount != 1 || cyberScript.Items[0].Name != "script" ||
		!strings.Contains(cyberScript.Items[0].Content, "Script workflow") ||
		strings.Contains(cyberScript.Items[0].Content, "Plan/Delivery workflow") {
		t.Fatalf("Cyber script guidance was not narrowly selected: %#v", cyberScript)
	}

	child.Skills = []string{"note_create"}
	if _, err := registry.AssembleSpecialistContext(parent, mode, child, attempt, 0); err == nil ||
		!strings.Contains(err.Error(), "model.chat") {
		t.Fatalf("Specialist without delegated model.chat was accepted: %v", err)
	}
}

func specialistContextFixture(t testing.TB, profile domain.Profile,
	surface domain.ExecutionSurface, names []string,
) (*Registry, Selection, domain.RunModeSnapshot, domain.AgentNode, domain.AgentAttempt) {
	t.Helper()
	registry, err := BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	selection, err := registry.ResolveSelection(ResolveSelectionRequest{
		SelectionID: "selection-specialist", RunID: "run-specialist",
		MissionID: "mission-specialist", Profile: profile, Names: names,
		TokenBudget: DefaultSelectionTokenBudget, RequestedBy: "operator",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	mode := domain.RunModeSnapshot{
		ID: "mode-specialist", RunID: selection.RunID, MissionID: selection.MissionID,
		Revision: 1, ProtocolVersion: domain.RunModeProtocolVersion,
		Surface: surface, Phase: domain.ExecutionPhaseDeliver, Profile: profile,
		Scope:         domain.Scope{NetworkMode: "disabled"},
		PolicyVersion: domain.RunModePolicyVersion, RequestedBy: "operator",
		Reason: "Specialist Skill test", CreatedAt: now,
	}
	child := domain.AgentNode{
		ID: "agent-specialist", RunID: selection.RunID, ParentID: "agent-root",
		SessionID: "session-specialist", Role: domain.AgentRoleSpecialist,
		Profile: profile, Skills: []string{"model.chat"}, Status: domain.AgentRunning,
		Depth: 1, ChildLimit: 0, TurnLimit: 2, TokenLimit: 128,
		TurnsUsed:       1,
		ActiveAttemptID: "attempt-specialist", Version: 2,
		CreatedAt: now, UpdatedAt: now,
	}
	attempt := domain.AgentAttempt{
		ID: child.ActiveAttemptID, RunID: child.RunID, AgentID: child.ID,
		ParentAgentID: child.ParentID, LeaseID: "lease-specialist", LeaseGeneration: 1,
		Turn: 1, Status: domain.AgentAttemptRunning, StartedAt: now, UpdatedAt: now,
	}
	return registry, selection, mode, child, attempt
}
