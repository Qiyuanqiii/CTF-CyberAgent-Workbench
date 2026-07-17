package tui

import (
	"fmt"
	"strings"
)

func (m *Model) renderEvents(width int, height int) string {
	lines := m.activityHeader(fmt.Sprintf("Events %d tail=#%d",
		len(m.runContext.Events), m.runContext.EventSequence), activityEvents, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	if len(m.runContext.Events) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-len(lines)-2)
	m.ensureEventVisible(visible)
	end := min(len(m.runContext.Events), m.eventScroll+visible)
	for index := m.eventScroll; index < end; index++ {
		event := m.runContext.Events[index]
		marker := " "
		if index == m.selectedEvent {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s #%d %s", marker,
			event.Sequence, event.Type), width))
	}
	selected := m.runContext.Events[m.selectedEvent]
	lines = append(lines, truncate("source="+selected.Source+
		" subject="+defaultText(selected.SubjectID, "-"), width))
	lines = append(lines, truncate(selected.CreatedAt.Format("15:04:05.000"), width))
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) renderAgents(width int, height int) string {
	lines := m.activityHeader(fmt.Sprintf("Agents %d", len(m.runContext.Agents)),
		activityAgents, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	if len(m.runContext.Agents) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-len(lines)-3)
	m.ensureAgentVisible(visible)
	end := min(len(m.runContext.Agents), m.agentScroll+visible)
	for index := m.agentScroll; index < end; index++ {
		agent := m.runContext.Agents[index].Node
		marker := " "
		if index == m.selectedAgent {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s/%s %s", marker,
			agent.Role, agent.Status, agent.ID), width))
	}
	selected := m.runContext.Agents[m.selectedAgent]
	node := selected.Node
	lines = append(lines, truncate(fmt.Sprintf("turns=%d/%d tokens=%d/%d", node.TurnsUsed,
		node.TurnLimit, node.TokensUsed, node.TokenLimit), width))
	lines = append(lines, truncate("skills="+defaultText(strings.Join(node.Skills, ","), "-"), width))
	if selected.Completion != nil {
		lines = append(lines, truncate("completion="+string(selected.Completion.Report.Outcome)+
			" "+singleLine(selected.Completion.Report.Summary), width))
	} else {
		lines = append(lines, "completion=-")
	}
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) renderExternalSkills(width int, height int) string {
	count := 0
	if m.runContext.ExternalSkills != nil {
		count = m.runContext.ExternalSkills.ItemCount
	}
	lines := m.activityHeader(fmt.Sprintf("External Skills %d", count),
		activitySkills, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	projection := m.runContext.ExternalSkills
	if projection == nil {
		lines = append(lines, "none")
		return strings.Join(windowTop(lines, height+1), "\n")
	}
	lines = append(lines, truncate(fmt.Sprintf(
		"%s/%s mode=r%d tokens=%d/%d confirmed=%t context=%t tools-granted=%t",
		projection.Surface, projection.Profile, projection.ModeRevision,
		projection.TokenUpperBound, projection.TokenBudget,
		projection.OperatorConfirmed, projection.ContextDeliveryAuthorized,
		projection.ToolCapabilityGrant), width))
	lines = append(lines, truncate(fmt.Sprintf(
		"delivery root=%d/%d specialist=%d/%d (committed/prepared)",
		projection.RootCommittedCount, projection.RootPreparedCount,
		projection.SpecialistCommittedCount, projection.SpecialistPreparedCount), width))
	for _, item := range projection.Items {
		specialist := ""
		if item.SpecialistEligible {
			specialist = " specialist"
		}
		lines = append(lines, truncate(fmt.Sprintf("#%d %s@%s tokens<=%d tools=%d trust=%s%s",
			item.Ordinal, item.Name, item.Version, item.TokenUpperBound,
			item.DeclaredToolCount, item.TrustClass, specialist), width))
	}
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) renderFindings(width int, height int) string {
	total := 0
	for _, report := range m.runContext.FindingReports {
		total += report.FindingCount
	}
	lines := m.activityHeader(fmt.Sprintf("Findings %d reports=%d", total,
		len(m.runContext.FindingReports)), activityFindings, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	if len(m.runContext.FindingReports) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-len(lines)-3)
	m.ensureFindingVisible(visible)
	end := min(len(m.runContext.FindingReports), m.findingScroll+visible)
	for index := m.findingScroll; index < end; index++ {
		report := m.runContext.FindingReports[index]
		marker := " "
		if index == m.selectedFinding {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s findings=%d", marker,
			report.Status, report.FindingCount), width))
		lines = append(lines, truncate("  "+report.Title, width))
	}
	selected := m.runContext.FindingReports[m.selectedFinding]
	lines = append(lines, truncate(fmt.Sprintf("severity c=%d h=%d m=%d l=%d i=%d",
		selected.Severity.Critical, selected.Severity.High, selected.Severity.Medium,
		selected.Severity.Low, selected.Severity.Info), width))
	lines = append(lines, truncate("report="+selected.ID+" evidence="+
		fmt.Sprint(selected.EvidenceCount), width))
	return strings.Join(windowTop(lines, height+1), "\n")
}
