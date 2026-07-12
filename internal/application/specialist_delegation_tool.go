package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/toolgateway"
)

type SpecialistDelegationMutationStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	CreateSpecialistDelegationProposal(ctx context.Context,
		operation domain.SpecialistDelegationOperation,
		proposal domain.SpecialistDelegationProposal,
		policyEvent events.Event, proposalEvent events.Event,
		toolEvent events.Event) (domain.SpecialistDelegationProposal, bool, error)
}

type SpecialistDelegationToolExecutor struct {
	store SpecialistDelegationMutationStore
}

func NewSpecialistDelegationToolExecutor(
	store SpecialistDelegationMutationStore,
) *SpecialistDelegationToolExecutor {
	return &SpecialistDelegationToolExecutor{store: store}
}

func (e *SpecialistDelegationToolExecutor) ProposeSpecialists(ctx context.Context,
	scope toolgateway.SpecialistDelegationContext,
	spec domain.SpecialistDelegationSpec,
) (toolgateway.SpecialistDelegationResult, error) {
	if e == nil || e.store == nil {
		return toolgateway.SpecialistDelegationResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "specialist delegation mutation store is required")
	}
	if err := scope.Validate(); err != nil {
		return toolgateway.SpecialistDelegationResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	normalized, err := domain.NormalizeSpecialistDelegationSpec(spec)
	if err != nil {
		return toolgateway.SpecialistDelegationResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "specialist delegation specification is invalid", err)
	}
	run, err := e.store.GetRun(ctx, scope.RunID)
	if err != nil {
		return toolgateway.SpecialistDelegationResult{}, apperror.Normalize(err)
	}
	now := time.Now().UTC()
	proposal := domain.SpecialistDelegationProposal{
		ID: idgen.New("delegation"), RunID: scope.RunID, RootAgentID: scope.RootAgentID,
		SessionID: scope.SessionID, WorkspaceID: scope.WorkspaceID,
		Status: domain.SpecialistDelegationProposed, Spec: normalized,
		RequestedBy: scope.RequestedBy, Version: 1, CreatedAt: now,
	}
	fingerprint, err := specialistDelegationFingerprint(scope, normalized)
	if err != nil {
		return toolgateway.SpecialistDelegationResult{}, err
	}
	operation := domain.SpecialistDelegationOperation{
		KeyDigest: runmutation.OperationKeyDigest(
			string(toolgateway.SpecialistDelegationProposeTool), scope.RunID, scope.OperationKey),
		RequestFingerprint: fingerprint, InvocationID: scope.InvocationID,
		ProposalID: proposal.ID, RunID: scope.RunID, SessionID: scope.SessionID,
		WorkspaceID: scope.WorkspaceID, RootAgentID: scope.RootAgentID,
		LeaseID: scope.LeaseID, LeaseGeneration: scope.LeaseGeneration,
		RequestedBy: scope.RequestedBy, CreatedAt: now,
	}
	policyEvent, proposalEvent, toolEvent, err := specialistDelegationEvents(run, scope, proposal)
	if err != nil {
		return toolgateway.SpecialistDelegationResult{}, err
	}
	stored, replayed, err := e.store.CreateSpecialistDelegationProposal(ctx, operation,
		proposal, policyEvent, proposalEvent, toolEvent)
	if err != nil {
		return toolgateway.SpecialistDelegationResult{}, apperror.Normalize(err)
	}
	return toolgateway.SpecialistDelegationResult{
		ProposalID: stored.ID, Status: stored.Status,
		AssignmentCount: len(stored.Spec.Assignments), Version: stored.Version,
		Replayed: replayed,
	}, nil
}

func specialistDelegationFingerprint(scope toolgateway.SpecialistDelegationContext,
	spec domain.SpecialistDelegationSpec,
) (string, error) {
	intent := struct {
		RunID       string                          `json:"run_id"`
		RootAgentID string                          `json:"root_agent_id"`
		SessionID   string                          `json:"session_id"`
		WorkspaceID string                          `json:"workspace_id"`
		RequestedBy string                          `json:"requested_by"`
		Spec        domain.SpecialistDelegationSpec `json:"spec"`
	}{
		RunID: scope.RunID, RootAgentID: scope.RootAgentID, SessionID: scope.SessionID,
		WorkspaceID: scope.WorkspaceID, RequestedBy: scope.RequestedBy, Spec: spec,
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("encode specialist delegation fingerprint: %w", err)
	}
	return runmutation.Fingerprint("specialist_delegation_request.v1", string(encoded)), nil
}

func specialistDelegationEvents(run domain.Run,
	scope toolgateway.SpecialistDelegationContext,
	proposal domain.SpecialistDelegationProposal,
) (events.Event, events.Event, events.Event, error) {
	policyEvent, err := events.New(run.ID, run.MissionID, events.PolicyDecisionEvent,
		"policy", scope.InvocationID, map[string]any{
			"context": "tool_run." + string(toolgateway.SpecialistDelegationProposeTool),
			"allowed": true, "needs_approval": false,
			"risk": scope.PolicyDecision.Risk, "reason": scope.PolicyDecision.Reason,
			"agent_id": scope.RootAgentID, "operator_review_required": true,
			"admission_authorized": false,
		})
	if err != nil {
		return events.Event{}, events.Event{}, events.Event{}, err
	}
	totalTurns := int64(0)
	totalTokens := int64(0)
	for _, assignment := range proposal.Spec.Assignments {
		totalTurns += assignment.TurnLimit
		totalTokens += assignment.TokenLimit
	}
	proposalEvent, err := events.New(run.ID, run.MissionID,
		events.AgentDelegationProposedEvent, "agent_coordinator", proposal.ID,
		map[string]any{
			"proposal_id": proposal.ID, "root_agent_id": proposal.RootAgentID,
			"protocol_version": proposal.Spec.Version, "status": proposal.Status,
			"assignment_count": len(proposal.Spec.Assignments),
			"suggested_turns":  totalTurns, "suggested_tokens": totalTokens,
			"operator_review_required": true, "admission_authorized": false,
		})
	if err != nil {
		return events.Event{}, events.Event{}, events.Event{}, err
	}
	toolEvent, err := events.New(run.ID, run.MissionID, events.ToolCompletedEvent,
		"agent_proposal_tool", scope.InvocationID, map[string]any{
			"invocation_id": scope.InvocationID,
			"tool_name":     toolgateway.SpecialistDelegationProposeTool,
			"target_id":     proposal.ID, "agent_id": scope.RootAgentID,
			"execution_backend": "agent_proposal", "admission_authorized": false,
		})
	if err != nil {
		return events.Event{}, events.Event{}, events.Event{}, err
	}
	return policyEvent, proposalEvent, toolEvent, nil
}
