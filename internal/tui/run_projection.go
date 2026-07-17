package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/skills"
)

const (
	maxTUIEventIdentityRunes = 256
	maxTUIEventLabelRunes    = 256
	maxTUIEventPayloadBytes  = 1 << 20
)

// RunProjection is the small, stable state contract used to compare the TUI
// with other read surfaces without parsing terminal styling.
type RunProjection struct {
	RunID                   string
	MissionID               string
	SessionID               string
	Status                  domain.RunStatus
	Surface                 domain.ExecutionSurface
	Phase                   domain.ExecutionPhase
	ModeRevision            int64
	EventSequence           int64
	AgentCount              int
	FindingReportCount      int
	FindingCount            int
	PlanProposalID          string
	PlanDirection           int
	DeliveryCheckpointCount int
	DeliveryGateReadyCount  int
	SteeringPending         int
	SteeringPrepared        int
	SteeringCommitted       int
	SteeringCancelled       int
	ExternalSkillCount      int
	ExternalSkillTokens     int
	ExternalRootPrepared    int
	ExternalRootCommitted   int
	ExternalChildPrepared   int
	ExternalChildCommitted  int
}

func (m *Model) CurrentRunProjection() (RunProjection, bool) {
	if m == nil || !m.runContext.Found {
		return RunProjection{}, false
	}
	findingCount := 0
	for _, report := range m.runContext.FindingReports {
		findingCount += report.FindingCount
	}
	projection := RunProjection{
		RunID: m.runContext.Run.ID, MissionID: m.runContext.Run.MissionID,
		SessionID: m.runContext.Run.SessionID, Status: m.runContext.Run.Status,
		Surface: m.runContext.Mode.Surface, Phase: m.runContext.Mode.Phase,
		ModeRevision:  m.runContext.Mode.Revision,
		EventSequence: m.runContext.EventSequence, AgentCount: len(m.runContext.Agents),
		FindingReportCount: len(m.runContext.FindingReports), FindingCount: findingCount,
	}
	if m.runContext.PlanProposal != nil {
		projection.PlanProposalID = m.runContext.PlanProposal.ID
	}
	if m.runContext.PlanSelection != nil {
		projection.PlanDirection = m.runContext.PlanSelection.DirectionOrdinal
	}
	projection.DeliveryCheckpointCount = len(m.runContext.DeliveryCheckpoints)
	projection.DeliveryGateReadyCount = readyDeliveryCheckpointCount(
		m.runContext.DeliveryCheckpoints, m.runContext.WorkItems, m.runContext.Mode)
	projection.SteeringPending = m.runContext.Steering.Pending
	projection.SteeringPrepared = m.runContext.Steering.Prepared
	projection.SteeringCommitted = m.runContext.Steering.Committed
	projection.SteeringCancelled = m.runContext.Steering.Cancelled
	if external := m.runContext.ExternalSkills; external != nil {
		projection.ExternalSkillCount = external.ItemCount
		projection.ExternalSkillTokens = external.TokenUpperBound
		projection.ExternalRootPrepared = external.RootPreparedCount
		projection.ExternalRootCommitted = external.RootCommittedCount
		projection.ExternalChildPrepared = external.SpecialistPreparedCount
		projection.ExternalChildCommitted = external.SpecialistCommittedCount
	}
	return projection, true
}

func loadExternalSkillProjectionContext(ctx context.Context, stateStore RunStateStore,
	run domain.Run, mode domain.RunModeSnapshot,
) (*skills.ExternalSkillProjection, error) {
	projectionStore, ok := any(stateStore).(externalSkillProjectionStore)
	if !ok {
		return nil, nil
	}
	value, found, err := projectionStore.GetExternalSkillProjectionByRun(ctx, run.ID)
	if err != nil || !found {
		return nil, err
	}
	if err := value.Validate(); err != nil || value.RunID != run.ID ||
		value.Surface != mode.Surface || value.Profile != mode.Profile ||
		value.ModeRevision > mode.Revision {
		return nil, errors.New("TUI external Skill projection is invalid or cross-Run")
	}
	copy := skills.CloneExternalSkillProjection(value)
	return &copy, nil
}

func loadPlanDeliveryContext(ctx context.Context, stateStore RunStateStore,
	run domain.Run,
) (*domain.PlanDeliveryProposal, *domain.PlanDeliverySelection, error) {
	proposals, err := stateStore.ListPlanDeliveryProposals(ctx, run.ID, 1)
	if err != nil {
		return nil, nil, err
	}
	selection, selected, err := stateStore.GetPlanDeliverySelectionByRun(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}
	var proposal *domain.PlanDeliveryProposal
	if selected {
		value, err := stateStore.GetPlanDeliveryProposal(ctx, selection.ProposalID)
		if err != nil {
			return nil, nil, err
		}
		proposal = &value
	} else if len(proposals) != 0 {
		value := proposals[0]
		proposal = &value
	}
	if proposal != nil && (proposal.Validate() != nil || proposal.RunID != run.ID) {
		return nil, nil, errors.New("TUI Plan/Delivery proposal is invalid or cross-Run")
	}
	if selected {
		if selection.Validate() != nil || selection.RunID != run.ID || proposal == nil ||
			selection.ProposalID != proposal.ID {
			return nil, nil, errors.New("TUI Plan/Delivery selection is invalid or cross-Run")
		}
		copy := domain.ClonePlanDeliverySelection(selection)
		return proposal, &copy, nil
	}
	return proposal, nil, nil
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
