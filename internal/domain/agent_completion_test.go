package domain

import (
	"strings"
	"testing"
	"time"
)

func TestCompletionReportNormalizationAndStrictDecode(t *testing.T) {
	report, err := NormalizeCompletionReport(CompletionReport{
		Version: CompletionReportVersion,
		Outcome: CompletionOutcome(" SUCCEEDED "), Summary: "  completed the focused review  ",
		WorkItemIDs: []string{"work-2", "work-1", "work-2"},
		NoteIDs:     []string{"note-2", "note-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Version != CompletionReportVersion || report.Outcome != CompletionSucceeded ||
		report.Summary != "completed the focused review" ||
		strings.Join(report.WorkItemIDs, ",") != "work-1,work-2" ||
		strings.Join(report.NoteIDs, ",") != "note-1,note-2" {
		t.Fatalf("completion report was not normalized: %#v", report)
	}
	decoded, err := DecodeCompletionReport(`{
		"version":"agent_completion.v1","outcome":"partial","summary":"handoff prepared",
		"work_item_ids":[],"note_ids":[]}`)
	if err != nil || decoded.Outcome != CompletionPartial || decoded.WorkItemIDs == nil || decoded.NoteIDs == nil {
		t.Fatalf("strict completion decode failed: report=%#v err=%v", decoded, err)
	}
	if _, err := DecodeCompletionReport(`{
		"version":"agent_completion.v1","outcome":"succeeded","summary":"done",
		"work_item_ids":[],"note_ids":[],"extra":true}`); err == nil {
		t.Fatal("completion report accepted an unknown field")
	}
}

func TestCompletionReportRejectsInvalidBoundsAndMetadata(t *testing.T) {
	cases := []CompletionReport{
		{Outcome: CompletionSucceeded, Summary: "missing version"},
		{Version: "agent_completion.v2", Outcome: CompletionSucceeded, Summary: "done"},
		{Version: CompletionReportVersion, Outcome: CompletionOutcome("failed"), Summary: "done"},
		{Version: CompletionReportVersion, Outcome: CompletionSucceeded, Summary: ""},
		{Version: CompletionReportVersion, Outcome: CompletionSucceeded, Summary: strings.Repeat("a", MaxCompletionSummaryBytes+1)},
		{Version: CompletionReportVersion, Outcome: CompletionSucceeded, Summary: "done", WorkItemIDs: []string{strings.Repeat("x", MaxCompletionReferenceBytes+1)}},
	}
	for index, report := range cases {
		if _, err := NormalizeCompletionReport(report); err == nil {
			t.Fatalf("invalid completion report %d was accepted: %#v", index, report)
		}
	}
	now := time.Now().UTC()
	report, err := NormalizeCompletionReport(CompletionReport{
		Version: CompletionReportVersion, Outcome: CompletionSucceeded, Summary: "done",
	})
	if err != nil {
		t.Fatal(err)
	}
	completion := AgentCompletion{
		ID: "completion-1", RunID: "run-1", AgentID: "agent-child",
		ParentAgentID: "agent-root", AttemptID: "attempt-1", Report: report,
		MessageID: "message-1", CreatedAt: now,
	}
	if err := completion.Validate(); err != nil {
		t.Fatalf("valid Agent completion was rejected: %v", err)
	}
	completion.ParentAgentID = completion.AgentID
	if err := completion.Validate(); err == nil {
		t.Fatal("completion accepted the child as its own parent")
	}
}
