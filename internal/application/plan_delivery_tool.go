package application

import (
	"context"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/toolgateway"
)

type PlanDeliveryProposalMutationStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetRunMode(ctx context.Context, runID string) (domain.RunModeSnapshot, error)
	CreatePlanDeliveryProposal(ctx context.Context,
		operation domain.PlanDeliveryProposalOperation,
		proposal domain.PlanDeliveryProposal,
		policyEvent events.Event, proposalEvent events.Event,
		toolEvent events.Event) (domain.PlanDeliveryProposal, bool, error)
}

type PlanDeliveryToolExecutor struct {
	store PlanDeliveryProposalMutationStore
}

func NewPlanDeliveryToolExecutor(
	store PlanDeliveryProposalMutationStore,
) *PlanDeliveryToolExecutor {
	return &PlanDeliveryToolExecutor{store: store}
}

func (e *PlanDeliveryToolExecutor) ProposePlan(ctx context.Context,
	scope toolgateway.PlanDeliveryContext, spec domain.PlanDeliverySpec,
) (toolgateway.PlanDeliveryResult, error) {
	if e == nil || e.store == nil {
		return toolgateway.PlanDeliveryResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Plan/Delivery proposal mutation store is required")
	}
	if err := scope.Validate(); err != nil {
		return toolgateway.PlanDeliveryResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	normalized, err := domain.NormalizePlanDeliverySpec(spec)
	if err != nil {
		return toolgateway.PlanDeliveryResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Plan/Delivery specification is invalid", err)
	}
	run, err := e.store.GetRun(ctx, scope.RunID)
	if err != nil {
		return toolgateway.PlanDeliveryResult{}, apperror.Normalize(err)
	}
	mode, err := e.store.GetRunMode(ctx, scope.RunID)
	if err != nil {
		return toolgateway.PlanDeliveryResult{}, apperror.Normalize(err)
	}
	if mode.Phase != domain.ExecutionPhasePlan {
		return toolgateway.PlanDeliveryResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Plan/Delivery proposals are available only in Plan phase")
	}
	now := time.Now().UTC()
	proposal := domain.PlanDeliveryProposal{
		ID: idgen.New("plan-proposal"), RunID: scope.RunID,
		RootAgentID: scope.RootAgentID, SessionID: scope.SessionID,
		WorkspaceID: scope.WorkspaceID, ModeRevision: mode.Revision,
		Status: domain.PlanDeliveryProposalProposed, Spec: normalized,
		RequestedBy: scope.RequestedBy, Version: 1, CreatedAt: now,
	}
	proposal.Fingerprint = domain.PlanDeliveryProposalFingerprint(proposal)
	operation := domain.PlanDeliveryProposalOperation{
		KeyDigest: runmutation.OperationKeyDigest(
			string(toolgateway.PlanDeliveryProposeTool), scope.RunID,
			scope.OperationKey),
		RequestFingerprint: domain.PlanDeliveryProposalRequestFingerprint(proposal),
		InvocationID:       scope.InvocationID,
		ProposalID:         proposal.ID, RunID: scope.RunID, SessionID: scope.SessionID,
		WorkspaceID: scope.WorkspaceID, RootAgentID: scope.RootAgentID,
		LeaseID: scope.LeaseID, LeaseGeneration: scope.LeaseGeneration,
		RequestedBy: scope.RequestedBy, CreatedAt: now,
	}
	policyEvent, proposalEvent, toolEvent, err := planDeliveryProposalEvents(
		run, scope, proposal)
	if err != nil {
		return toolgateway.PlanDeliveryResult{}, err
	}
	stored, replayed, err := e.store.CreatePlanDeliveryProposal(ctx, operation,
		proposal, policyEvent, proposalEvent, toolEvent)
	if err != nil {
		return toolgateway.PlanDeliveryResult{}, apperror.Normalize(err)
	}
	return toolgateway.PlanDeliveryResult{
		ProposalID: stored.ID, Status: stored.Status,
		DirectionCount: len(stored.Spec.Directions), Version: stored.Version,
		Replayed: replayed,
	}, nil
}

func planDeliveryProposalEvents(run domain.Run, scope toolgateway.PlanDeliveryContext,
	proposal domain.PlanDeliveryProposal,
) (events.Event, events.Event, events.Event, error) {
	policyEvent, err := events.New(run.ID, run.MissionID, events.PolicyDecisionEvent,
		"policy", scope.InvocationID, map[string]any{
			"context": "tool_run." + string(toolgateway.PlanDeliveryProposeTool),
			"allowed": true, "needs_approval": false,
			"risk":                     scope.PolicyDecision.Risk,
			"reason":                   scope.PolicyDecision.Reason,
			"agent_id":                 scope.RootAgentID,
			"operator_choice_required": true,
			"selection_authorized":     false, "phase_change_authorized": false,
			"execution_authorized": false,
		})
	if err != nil {
		return events.Event{}, events.Event{}, events.Event{}, err
	}
	proposalEvent, err := events.New(run.ID, run.MissionID,
		events.PlanDeliveryProposedEvent, "plan_delivery", proposal.ID,
		map[string]any{
			"proposal_id":              proposal.ID,
			"protocol_version":         proposal.Spec.Version,
			"status":                   proposal.Status,
			"direction_count":          len(proposal.Spec.Directions),
			"mode_revision":            proposal.ModeRevision,
			"operator_choice_required": true,
			"selection_authorized":     false, "phase_change_authorized": false,
			"execution_authorized": false,
		})
	if err != nil {
		return events.Event{}, events.Event{}, events.Event{}, err
	}
	toolEvent, err := events.New(run.ID, run.MissionID, events.ToolCompletedEvent,
		"plan_proposal_tool", scope.InvocationID, map[string]any{
			"invocation_id": scope.InvocationID,
			"tool_name":     toolgateway.PlanDeliveryProposeTool,
			"target_id":     proposal.ID, "agent_id": scope.RootAgentID,
			"execution_backend":    "plan_proposal",
			"selection_authorized": false, "phase_change_authorized": false,
			"execution_authorized": false,
		})
	if err != nil {
		return events.Event{}, events.Event{}, events.Event{}, err
	}
	for _, event := range []*events.Event{&policyEvent, &proposalEvent, &toolEvent} {
		event.CreatedAt = proposal.CreatedAt
	}
	return policyEvent, proposalEvent, toolEvent, nil
}
