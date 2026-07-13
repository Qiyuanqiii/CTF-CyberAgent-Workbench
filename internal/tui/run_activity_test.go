package tui

import (
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

func TestRunActivityViewsRenderEventsAgentsAndFindings(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	model := &Model{focus: focusTools, runContext: runContext{
		Found: true,
		Run: domain.Run{ID: "run-view", MissionID: "mission-view",
			SessionID: "session-view", Status: domain.RunRunning},
		Events: []events.Event{{EventID: "event-view", Version: events.EnvelopeVersion,
			RunID: "run-view", MissionID: "mission-view", Sequence: 7,
			Type: events.AgentStatusChangedEvent, Source: "coordinator",
			SubjectID: "agent-root", PayloadJSON: `{}`, CreatedAt: now}},
		EventSequence: 7,
		Agents: []agentContext{{Node: domain.AgentNode{ID: "agent-root", RunID: "run-view",
			SessionID: "session-view", Role: domain.AgentRoleRoot, Status: domain.AgentReady,
			TurnLimit: 4, TokenLimit: 1000, TurnsUsed: 1, TokensUsed: 120,
			Skills: []string{"code"}}}},
		FindingReports: []domain.FindingReportSummary{{ID: "report-one", RunID: "run-view",
			Status: domain.FindingReportGenerated, Title: "review findings",
			FindingCount: 1, EvidenceCount: 1,
			Severity: domain.FindingSeveritySummary{High: 1}}},
	}}

	for view, wants := range map[activityView][]string{
		activityEvents: {"Events 1 tail=#7", "#7 agent.status_changed", "source=coordinator"},
		activityAgents: {"Agents 1", "root/ready agent-root", "turns=1/4", "skills=code"},
		activityFindings: {"Findings 1 reports=1", "generated findings=1", "review findings",
			"severity c=0 h=1"},
	} {
		model.setActivityView(view)
		output := model.renderActivity(80, 16)
		for _, want := range wants {
			if !strings.Contains(output, want) {
				t.Fatalf("%s activity missing %q:\n%s", view, want, output)
			}
		}
	}
	projection, found := model.CurrentRunProjection()
	if !found || projection.Status != domain.RunRunning || projection.EventSequence != 7 ||
		projection.AgentCount != 1 || projection.FindingReportCount != 1 ||
		projection.FindingCount != 1 {
		t.Fatalf("unexpected TUI Run projection: %#v found=%t", projection, found)
	}
}

func TestTUITextRenderingNeutralizesTerminalControlCharacters(t *testing.T) {
	rendered := truncate("unsafe\x1b[31m title\nsecond", 80)
	if strings.ContainsRune(rendered, '\x1b') || strings.ContainsRune(rendered, '\n') ||
		!strings.Contains(rendered, `\u001B`) {
		t.Fatalf("terminal control text was not neutralized: %q", rendered)
	}
}
