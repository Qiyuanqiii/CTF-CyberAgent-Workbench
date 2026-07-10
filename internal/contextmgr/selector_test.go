package contextmgr

import (
	"strings"
	"testing"
)

func TestSelectSectionsHonorsPriorityBudgetAndSources(t *testing.T) {
	sections := []Section{
		{Kind: "note", SourceID: "low", Content: strings.Repeat("l", 40), Priority: 10},
		{Kind: "summary", SourceID: "summary-1", Content: strings.Repeat("s", 40), Priority: 100},
		{Kind: "note", SourceID: "high", Content: strings.Repeat("h", 40), Priority: 80},
	}
	selection, err := SelectSections(sections, 20)
	if err != nil {
		t.Fatal(err)
	}
	if selection.EstimatedTokens > selection.TokenBudget || len(selection.Sections) != 2 || len(selection.OmittedSources) != 1 {
		t.Fatalf("unexpected selection: %#v", selection)
	}
	if selection.Sections[0].SourceID != "summary-1" || selection.Sections[1].SourceID != "high" ||
		selection.OmittedSources[0].SourceID != "low" {
		t.Fatalf("priority ordering was not respected: %#v", selection)
	}
}

func TestSelectSectionsRedactsAndRejectsDuplicateSources(t *testing.T) {
	testAPIKey := "s" + "k-" + strings.Repeat("z", 26)
	selection, err := SelectSections([]Section{{
		Kind: "note", SourceID: "note-1", Content: "secret " + testAPIKey, Priority: 1,
	}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Sections) != 1 || strings.Contains(selection.Sections[0].Content, testAPIKey) ||
		!strings.Contains(selection.Sections[0].Content, "[REDACTED:api-key]") {
		t.Fatalf("selection did not redact content: %#v", selection)
	}
	_, err = SelectSections([]Section{
		{Kind: "note", SourceID: "same", Content: "one", Priority: 1},
		{Kind: "note", SourceID: "same", Content: "two", Priority: 2},
	}, 100)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate source rejection, got %v", err)
	}
}

func TestSelectSectionsIsDeterministicForEqualPriority(t *testing.T) {
	sections := []Section{
		{Kind: "note", SourceID: "first", Content: "first", Priority: 10},
		{Kind: "note", SourceID: "second", Content: "second", Priority: 10},
	}
	selection, err := SelectSections(sections, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Sections) != 2 || selection.Sections[0].SourceID != "first" || selection.Sections[1].SourceID != "second" {
		t.Fatalf("equal-priority order changed: %#v", selection)
	}
}
