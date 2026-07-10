package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
)

func TestSupervisorWorkBoardContextIsBoundedStructuredAndExcludesTerminalItems(t *testing.T) {
	items := make([]domain.WorkItem, 0, 26)
	for index := range 25 {
		items = append(items, domain.WorkItem{
			ID: fmt.Sprintf("work-%02d", index), Status: domain.WorkItemPending,
			Priority: domain.WorkItemPriorityHigh, Title: fmt.Sprintf("item %02d", index),
			Description:        strings.Repeat("description ", 700),
			AcceptanceCriteria: []string{strings.Repeat("criterion ", 100), strings.Repeat("second ", 100)},
			Dependencies:       []string{"work-dependency-1", "work-dependency-2"}, Version: 1,
		})
	}
	items = append(items, domain.WorkItem{
		ID: "work-terminal", Status: domain.WorkItemCompleted, Priority: domain.WorkItemPriorityCritical,
		Title: "must not appear", Version: 2,
	})
	contextText := supervisorWorkBoardContext(items)
	if contextText == "" {
		t.Fatal("expected work board context")
	}
	if len([]rune(contextText)) > maxSupervisorWorkBoardRunes {
		t.Fatalf("work board context exceeded bound: %d", len([]rune(contextText)))
	}
	if strings.Contains(contextText, "work-terminal") || strings.Contains(contextText, "must not appear") {
		t.Fatalf("terminal item was included: %s", contextText)
	}
	separator := strings.IndexByte(contextText, '\n')
	if separator < 0 {
		t.Fatalf("missing JSON envelope: %s", contextText)
	}
	var envelope supervisorWorkBoardEnvelope
	if err := json.Unmarshal([]byte(contextText[separator+1:]), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Version != "work_board.v1" || envelope.ActiveCount != 25 || envelope.IncludedCount == 0 ||
		envelope.IncludedCount >= envelope.ActiveCount || envelope.OmittedCount != envelope.ActiveCount-envelope.IncludedCount {
		t.Fatalf("unexpected bounded envelope: %#v", envelope)
	}
}
