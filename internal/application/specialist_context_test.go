package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
)

func TestSpecialistTurnInputBoundsOwnedMemoryAndKeepsInstructionsMandatory(t *testing.T) {
	mission := domain.Mission{
		ID: "mission-context", Goal: strings.Repeat("mission ", 500),
		Profile: domain.ProfileReview, Scope: domain.Scope{NetworkMode: "disabled"},
	}
	child := domain.AgentNode{
		ID: "agent-child", RunID: "run-context", ParentID: "agent-root",
		Profile: domain.ProfileReview, Skills: []string{"review"},
		TurnLimit: 4, TokenLimit: 20000, TokensUsed: 100,
	}
	attempt := domain.AgentAttempt{
		ID: "attempt-context", RunID: child.RunID, AgentID: child.ID,
		ParentAgentID: child.ParentID, Turn: 1,
	}
	messages := make([]domain.AgentMessage, 0, domain.MaxSpecialistContextMessages)
	for index := range domain.MaxSpecialistContextMessages {
		payload, err := json.Marshal(domain.AgentInstructionPayload{
			Version: domain.SpecialistInstructionVersion,
			Instruction: fmt.Sprintf("mandatory parent instruction %d %s", index,
				strings.Repeat("focus ", 80)),
		})
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, domain.AgentMessage{
			ID: fmt.Sprintf("message-%d", index), RunID: child.RunID,
			SenderAgentID: child.ParentID, RecipientAgentID: child.ID,
			Sequence: int64(index + 1), Kind: domain.AgentMessageInstruction,
			Semantic: domain.AgentMessageSemanticMessage, PayloadJSON: string(payload),
			Status: domain.AgentMessagePending,
		})
	}
	workItems := make([]domain.WorkItem, 0, maxSpecialistWorkItems)
	for index := range maxSpecialistWorkItems {
		workItems = append(workItems, domain.WorkItem{
			ID: fmt.Sprintf("work-%02d", index), RunID: child.RunID,
			OwnerAgentID: child.ID, Status: domain.WorkItemPending,
			Priority: domain.WorkItemPriorityNormal, Title: fmt.Sprintf("owned work %d", index),
			Description: strings.Repeat("bounded work detail ", 80), Version: 1,
		})
	}
	notes := make([]domain.Note, 0, maxSpecialistNotes)
	for index := range maxSpecialistNotes {
		notes = append(notes, domain.Note{
			ID: fmt.Sprintf("note-%02d", index), RunID: child.RunID,
			OwnerAgentID: child.ID, Status: domain.NoteActive,
			Visibility: domain.NoteVisibilityOwner, Category: domain.NoteObservation,
			Title:   fmt.Sprintf("owned note %d", index),
			Content: strings.Repeat("bounded evidence detail ", 100), Version: 1,
		})
	}
	input, selection, err := specialistTurnInput(mission, child, attempt, messages,
		workItems, notes)
	if err != nil {
		t.Fatal(err)
	}
	if len([]byte(input)) > maxSpecialistInputBytes || selection.EstimatedTokens <= 0 ||
		selection.EstimatedTokens > selection.TokenBudget || len(selection.OmittedSources) == 0 {
		t.Fatalf("Specialist context was not bounded: bytes=%d selection=%#v",
			len([]byte(input)), selection)
	}
	for index, message := range messages {
		if !containsContextSource(selection.IncludedSources, "parent_instruction", message.ID) ||
			!strings.Contains(input, fmt.Sprintf("mandatory parent instruction %d", index)) ||
			strings.Contains(input, message.ID) {
			t.Fatalf("mandatory instruction %d was omitted or exposed its message ID", index)
		}
	}
	if !containsContextSource(selection.IncludedSources, "specialist_mission", mission.ID) {
		t.Fatal("Specialist mission source was omitted")
	}
	audit := supervisorModelContextAudit(selection)
	if err := audit.Validate(); err != nil {
		t.Fatalf("Specialist context audit is invalid: %v", err)
	}
	attemptModel := llm.ModelAttempt{
		Number: 1, MaxAttempts: 1, Provider: "mock", Model: "mock", Context: audit,
	}
	if err := attemptModel.ValidateStarted(); err != nil {
		t.Fatalf("Specialist model attempt rejected context audit: %v", err)
	}
}
