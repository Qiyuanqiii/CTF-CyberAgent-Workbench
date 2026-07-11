package domain

import (
	"strings"
	"testing"
	"time"
)

func TestAgentNodeValidationAndTransitionBoundary(t *testing.T) {
	now := time.Now().UTC()
	node := AgentNode{
		ID: "agent-root", RunID: "run-1", SessionID: "session-1", Role: AgentRoleRoot,
		Profile: ProfileCode, Skills: []string{"model.chat", "profile.code"}, Status: AgentReady,
		TurnLimit: 10, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := node.Validate(); err != nil {
		t.Fatalf("valid root agent was rejected: %v", err)
	}
	if !node.CanTransition(AgentRunning) || !node.CanTransition(AgentCancelled) || node.CanTransition(AgentStatus("unknown")) {
		t.Fatalf("unexpected root transition set for %#v", node)
	}
	running := node
	running.Status = AgentRunning
	if err := running.Validate(); err == nil || !strings.Contains(err.Error(), "active attempt") {
		t.Fatalf("running agent without attempt was accepted: %v", err)
	}
	child := node
	child.ID = "agent-child"
	child.Role = AgentRoleSpecialist
	child.ParentID = node.ID
	child.Depth = 1
	child.SessionID = "session-2"
	if err := child.Validate(); err != nil {
		t.Fatalf("bounded specialist node was rejected: %v", err)
	}
	child.Depth = MaxAgentDepth + 1
	if err := child.Validate(); err == nil {
		t.Fatal("agent depth limit was not enforced")
	}
	overlong := node
	overlong.ID = strings.Repeat("a", MaxAgentIdentityRunes+1)
	if err := overlong.Validate(); err == nil {
		t.Fatal("agent identity length limit was not enforced")
	}
}

func TestNormalizeAgentSkillsIsDeterministicAndStrict(t *testing.T) {
	skills, err := NormalizeAgentSkills([]string{" Work_Item_Create ", "model.chat", "model.chat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills[0] != "model.chat" || skills[1] != "work_item_create" {
		t.Fatalf("skills were not normalized deterministically: %#v", skills)
	}
	if _, err := NormalizeAgentSkills([]string{"shell;exec"}); err == nil {
		t.Fatal("unsafe skill identifier was accepted")
	}
}

func TestAgentMessageRequiresBoundedJSONObject(t *testing.T) {
	now := time.Now().UTC()
	message := AgentMessage{
		ID: "message-1", RunID: "run-1", RecipientAgentID: "agent-root", Sequence: 1,
		Kind: AgentMessageInstruction, Semantic: AgentMessageSemanticMessage,
		PayloadJSON: `{"goal":"inspect"}`,
		Status:      AgentMessagePending, CreatedAt: now,
	}
	if err := message.Validate(); err != nil {
		t.Fatalf("valid agent message was rejected: %v", err)
	}
	message.PayloadJSON = `[]`
	if err := message.Validate(); err == nil {
		t.Fatal("non-object agent message payload was accepted")
	}
	message.PayloadJSON = `{"goal":"inspect"}`
	message.Status = AgentMessageConsumed
	if err := message.Validate(); err == nil {
		t.Fatal("consumed message without consumed_at was accepted")
	}
}

func TestAgentMessageSemanticSchemasAreStrict(t *testing.T) {
	now := time.Now().UTC()
	wake := AgentMessage{
		ID: "message-wake", RunID: "run-1", SenderAgentID: "agent-root",
		RecipientAgentID: "agent-child", Sequence: 1, Kind: AgentMessageControl,
		Semantic: AgentMessageSemanticWake, PayloadJSON: `{"reason":"dependency resolved"}`,
		Status: AgentMessagePending, CreatedAt: now,
	}
	if err := wake.Validate(); err != nil {
		t.Fatalf("valid wake message was rejected: %v", err)
	}
	wake.Kind = AgentMessageInstruction
	if err := wake.Validate(); err == nil {
		t.Fatal("wake message with a non-control kind was accepted")
	}
	wake.Kind = AgentMessageControl
	wake.PayloadJSON = `{"reason":"ready","extra":true}`
	if err := wake.Validate(); err == nil {
		t.Fatal("wake message with an unknown payload field was accepted")
	}

	dependency := wake
	dependency.ID = "message-dependency"
	dependency.Kind = AgentMessageNotification
	dependency.Semantic = AgentMessageSemanticDependency
	dependency.PayloadJSON = `{"dependency_id":"work-1","state":"satisfied"}`
	if err := dependency.Validate(); err != nil {
		t.Fatalf("valid dependency message was rejected: %v", err)
	}
	dependency.PayloadJSON = `{"dependency_id":"work-1","state":"maybe"}`
	if err := dependency.Validate(); err == nil {
		t.Fatal("dependency message with an unknown state was accepted")
	}
}

func TestAgentOperationKeyIsNormalizedAndBounded(t *testing.T) {
	key := "agent-operation-0001"
	if normalized, err := NormalizeAgentOperationKey(key); err != nil || normalized != key {
		t.Fatalf("valid operation key was rejected: normalized=%q err=%v", normalized, err)
	}
	for _, invalid := range []string{"short", " padded-operation-key ", strings.Repeat("x", MaxAgentOperationKeyBytes+1)} {
		if _, err := NormalizeAgentOperationKey(invalid); err == nil {
			t.Fatalf("invalid operation key was accepted: length=%d", len(invalid))
		}
	}
}

func TestSpecialistAdmissionRequiresBoundedIndependentResources(t *testing.T) {
	now := time.Now().UTC()
	admission := SpecialistAdmission{
		AgentID: "agent-child", SessionID: "session-child", RunID: "run-1",
		ParentAgentID: "agent-root", Title: "focused reviewer",
		Skills: []string{"model.chat", "note_create"}, TurnLimit: 2, TokenLimit: 200,
		MaxChildren: 2, CreatedAt: now,
	}
	if err := admission.Validate(); err != nil {
		t.Fatalf("valid specialist admission was rejected: %v", err)
	}
	invalid := admission
	invalid.Skills = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("specialist admission without skills was accepted")
	}
	invalid = admission
	invalid.TokenLimit = 0
	if err := invalid.Validate(); err == nil {
		t.Fatal("specialist admission without a token reservation was accepted")
	}
	invalid = admission
	invalid.MaxChildren = MaxAgentChildren + 1
	if err := invalid.Validate(); err == nil {
		t.Fatal("specialist admission above the graph capacity was accepted")
	}
}
