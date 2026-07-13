package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

const (
	maxTUIEventIdentityRunes = 256
	maxTUIEventLabelRunes    = 256
	maxTUIEventPayloadBytes  = 1 << 20
)

// RunProjection is the small, stable state contract used to compare the TUI
// with other read surfaces without parsing terminal styling.
type RunProjection struct {
	RunID              string
	MissionID          string
	SessionID          string
	Status             domain.RunStatus
	Surface            domain.ExecutionSurface
	Phase              domain.ExecutionPhase
	ModeRevision       int64
	EventSequence      int64
	AgentCount         int
	FindingReportCount int
	FindingCount       int
}

func (m *Model) CurrentRunProjection() (RunProjection, bool) {
	if m == nil || !m.runContext.Found {
		return RunProjection{}, false
	}
	findingCount := 0
	for _, report := range m.runContext.FindingReports {
		findingCount += report.FindingCount
	}
	return RunProjection{
		RunID: m.runContext.Run.ID, MissionID: m.runContext.Run.MissionID,
		SessionID: m.runContext.Run.SessionID, Status: m.runContext.Run.Status,
		Surface: m.runContext.Mode.Surface, Phase: m.runContext.Mode.Phase,
		ModeRevision:  m.runContext.Mode.Revision,
		EventSequence: m.runContext.EventSequence, AgentCount: len(m.runContext.Agents),
		FindingReportCount: len(m.runContext.FindingReports), FindingCount: findingCount,
	}, true
}

func loadAgentContext(ctx context.Context, stateStore RunStateStore,
	run domain.Run,
) ([]agentContext, error) {
	nodes, err := stateStore.ListAgentNodes(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(nodes) > domain.MaxAgentNodesPerRun {
		return nil, errors.New("TUI Agent projection exceeds the Run node limit")
	}
	result := make([]agentContext, 0, len(nodes))
	for _, node := range nodes {
		if err := node.Validate(); err != nil || node.RunID != run.ID {
			return nil, errors.New("TUI Agent projection contains an invalid or cross-Run node")
		}
		view := agentContext{Node: node}
		completion, found, err := stateStore.GetAgentCompletion(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		if found {
			if err := completion.Validate(); err != nil || completion.RunID != run.ID ||
				completion.AgentID != node.ID {
				return nil, errors.New("TUI Agent completion projection is inconsistent")
			}
			copy := completion
			view.Completion = &copy
		}
		result = append(result, view)
	}
	return result, nil
}

func loadFindingContext(ctx context.Context, stateStore RunStateStore,
	run domain.Run,
) ([]domain.FindingReportSummary, error) {
	reports, err := stateStore.ListFindingReportSummariesPage(ctx, run.ID, 0,
		maxTUIFindingReports)
	if err != nil {
		return nil, err
	}
	for _, summary := range reports {
		if err := summary.Validate(); err != nil || summary.RunID != run.ID {
			return nil, errors.New("TUI Finding report summary is invalid or cross-Run")
		}
	}
	return reports, nil
}

func validateTUIEventBatch(run domain.Run, afterSequence int64,
	batch []events.Event,
) error {
	expected := afterSequence + 1
	for _, event := range batch {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("TUI Run event is invalid: %w", err)
		}
		if event.RunID != run.ID || event.MissionID != run.MissionID ||
			event.Sequence != expected ||
			!validTUIEventText(event.EventID, maxTUIEventIdentityRunes, false) ||
			!validTUIEventText(event.RunID, maxTUIEventIdentityRunes, false) ||
			!validTUIEventText(event.MissionID, maxTUIEventIdentityRunes, false) ||
			!validTUIEventText(event.Type, maxTUIEventLabelRunes, false) ||
			!validTUIEventText(event.Source, maxTUIEventLabelRunes, false) ||
			!validTUIEventText(event.SubjectID, maxTUIEventIdentityRunes, true) ||
			len([]byte(event.PayloadJSON)) > maxTUIEventPayloadBytes {
			return errors.New("TUI Run event stream is invalid, non-contiguous, or cross-scope")
		}
		expected++
	}
	return nil
}

func validTUIEventText(value string, maxRunes int, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return utf8.ValidString(value) && strings.TrimSpace(value) == value &&
		!strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= maxRunes &&
		len([]byte(value)) <= maxRunes*4
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
