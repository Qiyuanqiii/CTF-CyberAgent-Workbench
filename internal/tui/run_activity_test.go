package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/fileedit"
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

func TestRunActivityRendersBoundedReadOnlyFileDiffDetail(t *testing.T) {
	now := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)
	model := &Model{focus: focusTools, width: 100, height: 24,
		activityView: activityEdits, runContext: runContext{
			Found: true,
			Run: domain.Run{ID: "run-edit", MissionID: "mission-edit",
				SessionID: "session-edit", Status: domain.RunRunning},
			FileEdits: []fileEditContext{{
				Preview: fileedit.Preview{ID: "edit-one", SessionID: "session-edit",
					WorkspaceID: "workspace-edit", Path: "src/main.go",
					Status: fileedit.StatusProposed, OriginalHash: strings.Repeat("a", 64),
					ProposedHash: strings.Repeat("b", 64), SecretsRedacted: true,
					CreatedAt: now, UpdatedAt: now},
				DiffLines: []string{"--- a/src/main.go", "+++ b/src/main.go",
					"@@ -1,1 +1,1 @@", "-old", "+new\\u001B[31m"},
			}},
		}}
	activity := model.renderActivity(80, 16)
	for _, want := range []string{"File Edits 1", "proposed src/main.go", "Enter: read-only diff"} {
		if !strings.Contains(activity, want) {
			t.Fatalf("FileEdit activity missing %q:\n%s", want, activity)
		}
	}
	if narrow := model.renderActivity(32, 12); !strings.Contains(narrow, "[Edits]") {
		t.Fatalf("narrow activity tabs hid the selected Edits view:\n%s", narrow)
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*Model)
	if cmd != nil || !model.editDetailOpen {
		t.Fatalf("Enter did not open the read-only detail: open=%t cmd=%#v", model.editDetailOpen, cmd)
	}
	detail := model.Snapshot()
	for _, want := range []string{"read-only diff", "src/main.go", "+new\\u001B[31m",
		"no approval or write authority"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("FileEdit detail missing %q:\n%s", want, detail)
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(*Model)
	if model.editDetailOpen {
		t.Fatal("Esc did not return from FileEdit detail")
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(*Model)
	if cmd != nil || !strings.Contains(model.status, "edits view is read-only") {
		t.Fatalf("Edits view exposed an approval action: status=%q cmd=%#v", model.status, cmd)
	}
}

func TestTUIDiffDisplayBoundsAndNeutralizesControls(t *testing.T) {
	input := "+safe\x1b[31m\n" + strings.Repeat("x", maxTUIDiffDisplayBytes+32)
	lines, truncated := boundedTUIDiffLines(input)
	joined := strings.Join(lines, "\n")
	if !truncated || len(lines) > maxTUIDiffDisplayLines+1 ||
		strings.ContainsRune(joined, '\x1b') || !strings.Contains(joined, `\u001B`) ||
		!strings.Contains(joined, "[diff truncated by TUI display bounds]") {
		t.Fatalf("TUI diff bounds failed: lines=%d truncated=%t", len(lines), truncated)
	}
}
