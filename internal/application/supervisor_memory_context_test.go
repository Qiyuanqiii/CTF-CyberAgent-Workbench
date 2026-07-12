package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/domain"
)

func TestSupervisorMemoryContextHonorsTokenBudgetAndPriority(t *testing.T) {
	now := time.Now().UTC()
	workItems := make([]domain.WorkItem, 20)
	for index := range workItems {
		workItems[index] = domain.WorkItem{
			ID: fmt.Sprintf("work-%02d", index), RunID: "run-1", Title: fmt.Sprintf("work item %02d", index),
			Description: strings.Repeat("work detail ", 80), Status: domain.WorkItemPending,
			Priority: domain.WorkItemPriorityHigh, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}
	notes := make([]domain.Note, 40)
	for index := range notes {
		notes[index] = domain.Note{
			ID: fmt.Sprintf("note-%02d", index), RunID: "run-1", Title: fmt.Sprintf("note %02d", index),
			Content: strings.Repeat("durable note content ", 200), Category: domain.NoteObservation,
			Visibility: domain.NoteVisibilityRun, Status: domain.NoteActive, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}
	notes[0].Category = domain.NoteDecision
	notes[0].Pinned = true
	selection, err := supervisorMemoryContext(contextmgr.Summary{ID: 1, Content: strings.Repeat("summary ", 400)}, true, workItems, notes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.EstimatedTokens > maxSupervisorMemoryTokens || selection.TokenBudget != maxSupervisorMemoryTokens ||
		len(selection.OmittedSources) == 0 {
		t.Fatalf("memory selection did not enforce budget: %#v", selection)
	}
	for _, expected := range []struct{ kind, id string }{
		{"summary", "summary-1"}, {"work_board", "active"}, {"note", "note-00"},
	} {
		if !hasContextSource(selection.IncludedSources, expected.kind, expected.id) {
			t.Fatalf("priority source %s/%s was omitted: %#v", expected.kind, expected.id, selection)
		}
	}
}

func TestSupervisorMemoryContextAlwaysIncludesBoundedRootInboxFirst(t *testing.T) {
	now := time.Now().UTC()
	inbox := make([]domain.AgentMessage, domain.MaxRootInboxContextMessages)
	for index := range inbox {
		payload, err := json.Marshal(domain.AgentAttemptFailurePayload{
			Version: domain.AgentAttemptFailureVersion,
			AgentID: fmt.Sprintf("agent-child-%d", index), AttemptID: fmt.Sprintf("attempt-%d", index),
			FailureCode: "provider_error", Reason: strings.Repeat("failure detail ", 200),
			RetryScheduled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		inbox[index] = domain.AgentMessage{
			ID: fmt.Sprintf("message-%d", index), RunID: "run-1",
			SenderAgentID: fmt.Sprintf("agent-child-%d", index), RecipientAgentID: "agent-root",
			Sequence: int64(index + 1), Kind: domain.AgentMessageNotification,
			Semantic: domain.AgentMessageSemanticMessage, PayloadJSON: string(payload),
			Status: domain.AgentMessagePending, CreatedAt: now,
		}
	}
	selection, err := supervisorMemoryContext(contextmgr.Summary{
		ID: 9, Content: strings.Repeat("large summary context ", 1200),
	}, true, nil, nil, inbox)
	if err != nil {
		t.Fatal(err)
	}
	if selection.EstimatedTokens > maxSupervisorMemoryTokens {
		t.Fatalf("root inbox context exceeded memory budget: %#v", selection)
	}
	for _, message := range inbox {
		if !hasContextSource(selection.IncludedSources, "agent_inbox", message.ID) {
			t.Fatalf("bound root inbox message was omitted: message=%s selection=%#v",
				message.ID, selection)
		}
	}
}

func hasContextSource(sources []contextmgr.Source, kind string, id string) bool {
	for _, source := range sources {
		if source.Kind == kind && source.SourceID == id {
			return true
		}
	}
	return false
}
