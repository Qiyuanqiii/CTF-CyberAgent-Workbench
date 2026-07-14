package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
	"cyberagent-workbench/internal/tools"
)

type ValidateSandboxExecutionCandidateRequest struct {
	PreparationID string
	Manifest      sandbox.Manifest
	ApprovalID    string
	OperationKey  string
	RequestedBy   string
}

func (s *SandboxManifestService) RequestApproval(ctx context.Context, preparationID,
	requestedBy string,
) (approval.Record, error) {
	if s == nil || s.store == nil {
		return approval.Record{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store is required")
	}
	preparationID = strings.TrimSpace(preparationID)
	requestedBy, err := normalizeSandboxOperator(requestedBy)
	if err != nil || !domain.ValidAgentID(preparationID) {
		return approval.Record{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox approval request identity is invalid", err)
	}
	intent, err := s.store.GetSandboxManifestIntent(ctx, preparationID)
	if err != nil {
		return approval.Record{}, apperror.Normalize(err)
	}
	if !intent.Validation.PolicyAllowed {
		return approval.Record{}, apperror.New(apperror.CodePolicyDenied,
			"policy-denied sandbox intent cannot request approval")
	}
	if !intent.Validation.NeedsApproval {
		return approval.Record{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox intent does not require approval")
	}
	if intent.Validation.ApprovalID != "" {
		return approval.Record{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox intent already has an approval binding")
	}
	run, err := s.store.GetRun(ctx, intent.Preparation.RunID)
	if err != nil {
		return approval.Record{}, apperror.Normalize(err)
	}
	if run.Terminal() {
		return approval.Record{}, apperror.New(apperror.CodeFailedPrecondition,
			"terminal Run cannot request sandbox execution approval")
	}
	now := time.Now().UTC()
	record, err := s.store.EnsureApproval(ctx, approval.Proposal{
		IdempotencyKey: approval.ProposalIdempotencyKey(sandboxApprovalToolName, preparationID),
		ProposalID:     preparationID, SessionID: run.SessionID,
		WorkspaceID: intent.Preparation.WorkspaceID, ToolName: sandboxApprovalToolName,
		ActionClass: sandboxApprovalActionClass, Mode: "per_call", Status: approval.StatusPending,
		RequestFingerprint: intent.Preparation.AuthorizationFingerprint,
		RequestedBy:        requestedBy, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return approval.Record{}, apperror.Normalize(err)
	}
	if err := requireExactSandboxApproval(record, run, intent, false); err != nil {
		return approval.Record{}, err
	}
	return record, nil
}

func (s *SandboxManifestService) ReviewApproval(ctx context.Context, preparationID string,
	action approval.Action, operationKey, reviewedBy, reason string,
) (approval.DecisionResult, error) {
	if s == nil || s.store == nil {
		return approval.DecisionResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store is required")
	}
	preparationID = strings.TrimSpace(preparationID)
	intent, err := s.store.GetSandboxManifestIntent(ctx, preparationID)
	if err != nil {
		return approval.DecisionResult{}, apperror.Normalize(err)
	}
	if !intent.Validation.PolicyAllowed || !intent.Validation.NeedsApproval {
		return approval.DecisionResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox intent has no reviewable approval requirement")
	}
	run, err := s.store.GetRun(ctx, intent.Preparation.RunID)
	if err != nil {
		return approval.DecisionResult{}, apperror.Normalize(err)
	}
	if action == approval.ActionApprove && run.Terminal() {
		return approval.DecisionResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"terminal Run sandbox approval cannot be approved")
	}
	if action == approval.ActionDeny && strings.TrimSpace(reason) == "" {
		return approval.DecisionResult{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox approval denial requires a reason")
	}
	reviewedBy, err = normalizeSandboxOperator(reviewedBy)
	if err != nil {
		return approval.DecisionResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox approval reviewer identity is invalid", err)
	}
	result, err := s.store.DecideApproval(ctx, approval.DecisionRequest{
		ProposalID: preparationID, IdempotencyKey: operationKey, Action: action,
		Reason: reason, ReviewedBy: reviewedBy,
	})
	if err != nil {
		return approval.DecisionResult{}, apperror.Normalize(err)
	}
	if err := requireExactSandboxApproval(result.Approval, run, intent,
		action == approval.ActionApprove); err != nil {
		return approval.DecisionResult{}, err
	}
	return result, nil
}

func (s *SandboxManifestService) ValidateExecutionCandidate(ctx context.Context,
	request ValidateSandboxExecutionCandidateRequest,
) (sandbox.ValidatedExecutionCandidate, error) {
	if s == nil || s.store == nil || s.checker == nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store and policy checker are required")
	}
	normalized, err := normalizeSandboxCandidateRequest(request)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution candidate request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, normalized.Manifest)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution candidate Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution candidate Manifest fingerprint failed", err)
	}
	requestFingerprint := sandbox.CandidateRequestFingerprint(normalized.PreparationID,
		manifestFingerprint, normalized.ApprovalID, normalized.RequestedBy)
	operationKeyDigest := runmutation.Fingerprint("sandbox_execution_candidate_operation.v1",
		normalized.PreparationID, normalized.OperationKey)
	if operation, found, err := s.store.GetSandboxExecutionCandidateOperation(ctx,
		operationKeyDigest); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	} else if found {
		return s.replayExecutionCandidate(ctx, normalized, requestFingerprint, operation)
	}

	intent, err := s.store.GetSandboxManifestIntent(ctx, normalized.PreparationID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	}
	if manifestFingerprint != intent.Preparation.ManifestFingerprint {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeConflict,
			"resupplied sandbox Manifest does not match the prepared intent")
	}
	if !intent.Validation.PolicyAllowed {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodePolicyDenied,
			"policy-denied sandbox intent cannot become an execution candidate")
	}
	run, err := s.store.GetRun(ctx, intent.Preparation.RunID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	}
	if run.Terminal() {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeFailedPrecondition,
			"terminal Run cannot validate a sandbox execution candidate")
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	}
	if mission.ID != intent.Preparation.MissionID || mission.WorkspaceID != intent.Preparation.WorkspaceID ||
		mission.Scope.WorkspaceID != mission.WorkspaceID {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeConflict,
			"sandbox execution candidate Mission or workspace binding changed")
	}
	workspace, err := s.store.GetSandboxWorkspace(ctx, mission.WorkspaceID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	}
	rootPath, err := validateSandboxWorkspaceBinding(workspace)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox execution candidate workspace binding is invalid", err)
	}
	normalizedScope, err := normalizeSandboxMissionScope(mission.Scope)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox execution candidate Mission scope is invalid", err)
	}
	if err := requireSandboxScopeSubset(manifest.Network, normalizedScope); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodePolicyDenied,
			"sandbox execution candidate attempted to widen Mission scope", err)
	}
	canonicalScope, err := json.Marshal(normalizedScope)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeInternal,
			"encode sandbox execution candidate scope", err)
	}
	workspaceFingerprint := runmutation.Fingerprint("sandbox_workspace_binding.v1",
		workspace.ID, rootPath)
	scopeFingerprint := runmutation.Fingerprint("sandbox_scope_binding.v1", string(canonicalScope))
	authorizationFingerprint := runmutation.Fingerprint("sandbox_authorization.v1",
		run.ID, mission.ID, workspace.ID, manifestFingerprint, workspaceFingerprint, scopeFingerprint)
	if workspaceFingerprint != intent.Preparation.WorkspaceFingerprint ||
		scopeFingerprint != intent.Preparation.ScopeFingerprint ||
		authorizationFingerprint != intent.Preparation.AuthorizationFingerprint {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeConflict,
			"sandbox execution candidate authorization binding changed after preparation")
	}
	canonicalManifest, err := manifest.CanonicalJSON()
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeInternal,
			"encode sandbox execution candidate Manifest", err)
	}
	decision := hardenSandboxDecision(s.checker.CheckToolCall(tools.Call{
		Name: sandboxApprovalToolName, Args: map[string]string{"intent": string(canonicalManifest)},
	}), manifest)
	policyFingerprint := runmutation.Fingerprint("sandbox_policy_decision.v1",
		authorizationFingerprint, fmt.Sprintf("%t", decision.Allowed),
		fmt.Sprintf("%t", decision.NeedsApproval), decision.Risk, decision.Reason)
	if !decision.Allowed {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodePolicyDenied,
			"current Policy denied the sandbox execution candidate")
	}
	if policyFingerprint != intent.Validation.PolicyFingerprint ||
		decision.NeedsApproval != intent.Validation.NeedsApproval {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeConflict,
			"sandbox Policy decision changed after preparation; prepare a new intent")
	}

	approvalStatus := sandbox.ApprovalNotRequired
	if intent.Validation.NeedsApproval {
		if intent.Validation.ApprovalID != "" {
			return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeFailedPrecondition,
				"sandbox execution candidate requires a v49 preparation-owned approval request")
		}
		if normalized.ApprovalID == "" {
			return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeFailedPrecondition,
				"sandbox execution candidate requires an exact approved approval id")
		}
		record, err := s.store.GetApproval(ctx, normalized.ApprovalID)
		if err != nil {
			return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
		}
		if err := requireExactSandboxApproval(record, run, intent, true); err != nil {
			return sandbox.ValidatedExecutionCandidate{}, err
		}
		approvalStatus = sandbox.ApprovalApproved
	} else if normalized.ApprovalID != "" {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution candidate does not accept approval for a no-approval intent")
	}

	mountBinding, err := sandbox.ResolveMountSources(ctx, rootPath, manifest)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox mount source resolution failed", err)
	}
	agentUsage, err := s.store.GetRunAgentUsage(ctx, run.ID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	}
	toolUsage, err := s.store.GetToolCallUsage(ctx, run.ID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	}
	if err := requireSandboxCandidateBudget(run.Budget, agentUsage, toolUsage.Consumed); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, err
	}
	now := time.Now().UTC()
	if lease, found, err := s.store.GetRunExecutionLease(ctx, run.ID); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	} else if found && lease.ActiveAt(now) {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox execution candidate requires a quiescent Run without an active execution lease")
	}
	candidate := sandbox.ExecutionCandidate{
		ID: idgen.New("sandbox-candidate"), PreparationID: intent.Preparation.ID,
		RunID: run.ID, MissionID: mission.ID, WorkspaceID: workspace.ID,
		ProtocolVersion:     sandbox.ExecutionCandidateProtocolVersion,
		ManifestFingerprint: manifestFingerprint, AuthorizationFingerprint: authorizationFingerprint,
		WorkspaceFingerprint: workspaceFingerprint, ScopeFingerprint: scopeFingerprint,
		PolicyFingerprint: policyFingerprint, MountBindingFingerprint: mountBinding.Fingerprint,
		ApprovalID: normalized.ApprovalID, ApprovalStatus: approvalStatus,
		MountCount: mountBinding.MountCount, RegularFileMountCount: mountBinding.RegularFileCount,
		DirectoryMountCount: mountBinding.DirectoryCount, TokensUsed: agentUsage.TotalTokens,
		ExecutionMillisUsed: agentUsage.TotalExecutionMillis, ToolCallsUsed: toolUsage.Consumed,
		BudgetChecked: true, LeaseQuiescent: true, BackendEnabled: false,
		ExecutionAuthorized: false, RequestedBy: normalized.RequestedBy, ValidatedAt: now,
	}
	operation := sandbox.CandidateOperation{
		KeyDigest: operationKeyDigest, RequestFingerprint: requestFingerprint,
		CandidateID: candidate.ID, PreparationID: candidate.PreparationID, RunID: candidate.RunID,
		RequestedBy: candidate.RequestedBy, CreatedAt: now,
	}
	stored, replayed, err := s.store.CreateSandboxExecutionCandidate(ctx, candidate, operation)
	stored.Replayed = replayed
	return stored, apperror.Normalize(err)
}

func (s *SandboxManifestService) GetExecutionCandidate(ctx context.Context, id string,
) (sandbox.ValidatedExecutionCandidate, error) {
	if s == nil || s.store == nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store is required")
	}
	value, err := s.store.GetSandboxExecutionCandidate(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListExecutionCandidates(ctx context.Context, runID string,
	limit int,
) ([]sandbox.ValidatedExecutionCandidate, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store is required")
	}
	values, err := s.store.ListSandboxExecutionCandidates(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) replayExecutionCandidate(ctx context.Context,
	request ValidateSandboxExecutionCandidateRequest, requestFingerprint string,
	operation sandbox.CandidateOperation,
) (sandbox.ValidatedExecutionCandidate, error) {
	if operation.PreparationID != request.PreparationID || operation.RequestedBy != request.RequestedBy ||
		operation.RequestFingerprint != requestFingerprint {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeConflict,
			"sandbox execution candidate operation key was already used for different intent")
	}
	stored, err := s.store.GetSandboxExecutionCandidate(ctx, operation.CandidateID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, apperror.Normalize(err)
	}
	if stored.Candidate.ID != operation.CandidateID ||
		stored.Candidate.PreparationID != operation.PreparationID ||
		stored.Candidate.RunID != operation.RunID || stored.Candidate.RequestedBy != operation.RequestedBy {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeInternal,
			"stored sandbox execution candidate replay binding is invalid")
	}
	stored.Replayed = true
	return stored, nil
}

func normalizeSandboxCandidateRequest(request ValidateSandboxExecutionCandidateRequest,
) (ValidateSandboxExecutionCandidateRequest, error) {
	originalOperationKey := request.OperationKey
	request.PreparationID = strings.TrimSpace(request.PreparationID)
	request.ApprovalID = strings.TrimSpace(request.ApprovalID)
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	requestedBy, err := normalizeSandboxOperator(request.RequestedBy)
	if err != nil {
		return ValidateSandboxExecutionCandidateRequest{}, err
	}
	request.RequestedBy = requestedBy
	if !domain.ValidAgentID(request.PreparationID) ||
		(request.ApprovalID != "" && !domain.ValidAgentID(request.ApprovalID)) {
		return ValidateSandboxExecutionCandidateRequest{}, errors.New("sandbox preparation or approval identity is invalid")
	}
	if request.OperationKey != originalOperationKey || !utf8.ValidString(request.OperationKey) {
		return ValidateSandboxExecutionCandidateRequest{}, errors.New("sandbox candidate operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil {
		return ValidateSandboxExecutionCandidateRequest{}, err
	}
	for _, current := range request.OperationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return ValidateSandboxExecutionCandidateRequest{}, errors.New("sandbox candidate operation key cannot contain whitespace or control characters")
		}
	}
	return request, nil
}

func normalizeSandboxOperator(value string) (string, error) {
	value = strings.TrimSpace(redact.String(value))
	if value == "" {
		value = "cli_operator"
	}
	if !domain.ValidAgentID(value) || strings.ContainsRune(value, 0) {
		return "", errors.New("bounded sandbox operator identity is required")
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return "", errors.New("sandbox operator identity cannot contain control characters")
		}
	}
	return value, nil
}

func requireExactSandboxApproval(record approval.Record, run domain.Run,
	intent sandbox.PreparedIntent, requireApproved bool,
) error {
	if err := record.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInternal, "stored sandbox approval is invalid", err)
	}
	if record.ProposalID != intent.Preparation.ID || record.RunID != run.ID ||
		record.SessionID != run.SessionID || record.WorkspaceID != intent.Preparation.WorkspaceID ||
		record.ToolName != sandboxApprovalToolName || record.ActionClass != sandboxApprovalActionClass ||
		record.Mode != "per_call" || record.RequestFingerprint != intent.Preparation.AuthorizationFingerprint {
		return apperror.New(apperror.CodeConflict,
			"sandbox approval does not match the exact preparation, Run, workspace, action, and fingerprint")
	}
	if requireApproved && record.Status != approval.StatusApproved {
		return apperror.New(apperror.CodeFailedPrecondition,
			"sandbox execution candidate requires an approved approval record")
	}
	return nil
}

func requireSandboxCandidateBudget(budget domain.Budget, usage domain.RunAgentUsage,
	toolCalls int64,
) error {
	if budget.MaxTokens > 0 && usage.TotalTokens >= budget.MaxTokens {
		return apperror.New(apperror.CodeResourceExhausted,
			"sandbox execution candidate requires remaining Run token budget")
	}
	if budget.TimeoutSeconds > 0 &&
		usage.TotalExecutionMillis >= budget.TimeoutSeconds*1000 {
		return apperror.New(apperror.CodeResourceExhausted,
			"sandbox execution candidate requires remaining Run execution-time budget")
	}
	if budget.MaxToolCalls > 0 && toolCalls >= budget.MaxToolCalls {
		return apperror.New(apperror.CodeResourceExhausted,
			"sandbox execution candidate requires remaining Run tool-call budget")
	}
	return nil
}
