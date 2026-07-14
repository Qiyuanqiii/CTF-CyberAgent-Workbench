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
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
	"cyberagent-workbench/internal/tools"
)

const sandboxLifecycleLeaseTTL = 30 * time.Second

type BeginSandboxExecutionRequest struct {
	CandidateID  string
	Manifest     sandbox.Manifest
	OperationKey string
	RequestedBy  string
}

type CancelSandboxExecutionRequest struct {
	ExecutionID  string
	OperationKey string
	RequestedBy  string
}

type CleanupSandboxExecutionRequest struct {
	ExecutionID  string
	OperationKey string
	ReconciledBy string
}

func (s *SandboxManifestService) BeginDisabledExecution(ctx context.Context,
	request BeginSandboxExecutionRequest,
) (sandbox.Lifecycle, error) {
	if s == nil || s.store == nil || s.checker == nil {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox lifecycle store and policy checker are required")
	}
	normalized, err := normalizeSandboxBeginRequest(request)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, normalized.Manifest)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution Manifest fingerprint failed", err)
	}
	operationKeyDigest := runmutation.Fingerprint("sandbox_execution_operation.v1",
		normalized.CandidateID, normalized.OperationKey)
	if existing, found, err := s.store.GetSandboxExecutionOperation(ctx,
		operationKeyDigest); err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	} else if found {
		return s.replayDisabledExecution(ctx, normalized, manifestFingerprint, existing)
	}

	validated, err := s.store.GetSandboxExecutionCandidate(ctx, normalized.CandidateID)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	candidate := validated.Candidate
	intent, run, mission, rootPath, err := s.revalidateExecutionCandidate(ctx, candidate,
		manifest, manifestFingerprint)
	if err != nil {
		return sandbox.Lifecycle{}, err
	}
	executionID := idgen.New("sandbox-execution")
	inputs, inputBytes, err := s.bindSandboxInputArtifacts(ctx, executionID, manifest,
		run, mission.WorkspaceID)
	if err != nil {
		return sandbox.Lifecycle{}, err
	}
	if rootPath == "" || intent.Preparation.CancellationID == "" {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeInternal,
			"sandbox execution boundary lost its workspace or cancellation binding")
	}
	outputPlan := sandbox.NewOutputCapturePlan(manifest)
	if err := outputPlan.Validate(); err != nil {
		return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output capture plan is invalid", err)
	}
	now := time.Now().UTC()
	execution := sandbox.DisabledExecution{
		ID: executionID, CandidateID: candidate.ID, PreparationID: candidate.PreparationID,
		RunID: candidate.RunID, MissionID: candidate.MissionID,
		WorkspaceID: candidate.WorkspaceID, CancellationID: intent.Preparation.CancellationID,
		ProtocolVersion:          sandbox.DisabledExecutionProtocolVersion,
		ManifestFingerprint:      candidate.ManifestFingerprint,
		AuthorizationFingerprint: candidate.AuthorizationFingerprint,
		PolicyFingerprint:        candidate.PolicyFingerprint,
		MountBindingFingerprint:  candidate.MountBindingFingerprint,
		InputArtifactCount:       len(inputs), InputArtifactBytes: inputBytes,
		InputArtifactDigest: sandbox.InputArtifactBindingsDigest(inputs), OutputPlan: outputPlan,
		InitialLeaseID: idgen.New("sandbox-lease"), InitialLeaseGeneration: 1,
		BackendEnabled: false, ExecutionAuthorized: false, BackendStarted: false,
		RequestedBy: normalized.RequestedBy, CreatedAt: now,
	}
	operation := sandbox.ExecutionOperation{
		KeyDigest: operationKeyDigest, ExecutionID: execution.ID,
		CandidateID: execution.CandidateID, RunID: execution.RunID,
		RequestedBy: execution.RequestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.ExecutionRequestFingerprint(execution)
	lifecycle, replayed, err := s.store.CreateSandboxDisabledExecution(ctx, execution, inputs,
		operation, idgen.New("sandbox-preparer"), sandboxLifecycleLeaseTTL)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	lifecycle.Replayed = replayed
	if lifecycle.Lease.Generation == execution.InitialLeaseGeneration &&
		lifecycle.Lease.LeaseID == lifecycle.Execution.InitialLeaseID {
		released, _, releaseErr := s.store.ReleaseSandboxExecutionLease(ctx, lifecycle.Lease)
		if releaseErr != nil {
			return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeUnavailable,
				"sandbox execution was recorded but its disabled preparation lease was not released",
				releaseErr)
		}
		lifecycle.Lease = released
	}
	return lifecycle, nil
}

func (s *SandboxManifestService) CancelDisabledExecution(ctx context.Context,
	request CancelSandboxExecutionRequest,
) (sandbox.Lifecycle, error) {
	if s == nil || s.store == nil {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox lifecycle store is required")
	}
	executionID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.ExecutionID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox cancellation request is invalid", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_execution_cancel_operation.v1",
		executionID, operationKey)
	if existing, found, err := s.store.GetSandboxCancellationOperation(ctx, keyDigest); err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	} else if found {
		lifecycle, err := s.store.GetSandboxDisabledExecution(ctx, existing.ExecutionID)
		if err != nil {
			return sandbox.Lifecycle{}, apperror.Normalize(err)
		}
		if lifecycle.Cancellation == nil || existing.ExecutionID != executionID ||
			existing.RequestedBy != requestedBy ||
			existing.RequestFingerprint != sandbox.CancellationRequestFingerprint(executionID,
				lifecycle.Execution.CancellationID, requestedBy) {
			return sandbox.Lifecycle{}, apperror.New(apperror.CodeConflict,
				"sandbox cancellation operation key was already used for different intent")
		}
		lifecycle.Replayed = true
		return lifecycle, nil
	}
	lifecycle, err := s.store.GetSandboxDisabledExecution(ctx, executionID)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	if lifecycle.Cleanup != nil {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeFailedPrecondition,
			"cleaned sandbox execution cannot be cancelled")
	}
	now := time.Now().UTC()
	cancellation := sandbox.CancellationRequest{
		ID: idgen.New("sandbox-cancel-request"), ExecutionID: executionID,
		RunID: lifecycle.Execution.RunID, CancellationID: lifecycle.Execution.CancellationID,
		ProtocolVersion: sandbox.CancellationProtocolVersion, RequestedBy: requestedBy,
		RequestedAt: now,
	}
	operation := sandbox.CancellationOperation{
		KeyDigest: keyDigest, RequestID: cancellation.ID, ExecutionID: executionID,
		RunID: cancellation.RunID, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.CancellationRequestFingerprint(executionID,
		cancellation.CancellationID, requestedBy)
	_, replayed, err := s.store.CreateSandboxCancellation(ctx, cancellation, operation)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	stored, err := s.store.GetSandboxDisabledExecution(ctx, executionID)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) CleanupDisabledExecution(ctx context.Context,
	request CleanupSandboxExecutionRequest,
) (sandbox.Lifecycle, error) {
	if s == nil || s.store == nil {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox lifecycle store is required")
	}
	executionID, operationKey, reconciledBy, err := normalizeSandboxLifecycleOperation(
		request.ExecutionID, request.OperationKey, request.ReconciledBy)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox cleanup request is invalid", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_cleanup_operation.v1", executionID, operationKey)
	if existing, found, err := s.store.GetSandboxCleanupOperation(ctx, keyDigest); err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	} else if found {
		lifecycle, err := s.store.GetSandboxDisabledExecution(ctx, existing.ExecutionID)
		if err != nil {
			return sandbox.Lifecycle{}, apperror.Normalize(err)
		}
		if lifecycle.Cleanup == nil || existing.ExecutionID != executionID ||
			existing.ReconciledBy != reconciledBy ||
			existing.RequestFingerprint != sandbox.CleanupRequestFingerprint(executionID,
				lifecycle.Cleanup.CancellationObserved, reconciledBy) {
			return sandbox.Lifecycle{}, apperror.New(apperror.CodeConflict,
				"sandbox cleanup operation key was already used for different intent")
		}
		if lifecycle.Lease.Status == sandbox.ExecutionLeaseActive &&
			lifecycle.Lease.LeaseID == lifecycle.Cleanup.LeaseID &&
			lifecycle.Lease.Generation == lifecycle.Cleanup.LeaseGeneration {
			if _, _, releaseErr := s.store.ReleaseSandboxExecutionLease(ctx, lifecycle.Lease); releaseErr != nil {
				return sandbox.Lifecycle{}, apperror.Wrap(apperror.CodeUnavailable,
					"sandbox cleanup was recorded but its lease was not released", releaseErr)
			}
			lifecycle, err = s.store.GetSandboxDisabledExecution(ctx, executionID)
			if err != nil {
				return sandbox.Lifecycle{}, apperror.Normalize(err)
			}
		}
		lifecycle.Replayed = true
		return lifecycle, nil
	}

	lifecycle, err := s.store.GetSandboxDisabledExecution(ctx, executionID)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	if lifecycle.Cleanup != nil {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeConflict,
			"sandbox execution cleanup already exists under another operation key")
	}
	// The generation-one lease can only prepare this disabled record; it never starts a backend.
	// Releasing it here makes crash recovery immediate without taking ownership from a real worker.
	if lifecycle.Lease.Status == sandbox.ExecutionLeaseActive && lifecycle.Lease.Generation == 1 &&
		lifecycle.Lease.LeaseID == lifecycle.Execution.InitialLeaseID &&
		!lifecycle.Execution.BackendStarted {
		if _, _, err := s.store.ReleaseSandboxExecutionLease(ctx, lifecycle.Lease); err != nil {
			return sandbox.Lifecycle{}, apperror.Normalize(err)
		}
	}
	acquisition, err := s.store.AcquireSandboxExecutionLease(ctx, executionID,
		idgen.New("sandbox-reconciler"), "", sandboxLifecycleLeaseTTL)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	lease := acquisition.Lease
	releaseLease := func() error {
		_, _, releaseErr := s.store.ReleaseSandboxExecutionLease(ctx, lease)
		return releaseErr
	}
	current, err := s.store.GetSandboxDisabledExecution(ctx, executionID)
	if err != nil {
		return sandbox.Lifecycle{}, errors.Join(apperror.Normalize(err), releaseLease())
	}
	now := time.Now().UTC()
	result := sandbox.CleanupResult{
		ID: idgen.New("sandbox-cleanup"), ExecutionID: executionID,
		RunID: current.Execution.RunID, ProtocolVersion: sandbox.CleanupProtocolVersion,
		LeaseID: lease.LeaseID, LeaseGeneration: lease.Generation,
		CancellationObserved: current.Cancellation != nil, BackendStarted: false,
		OrphanDetected: false, OrphanReaped: false, InputArtifactsVerified: true,
		OutputArtifactCount: 0, CleanupComplete: true, Outcome: "backend_disabled",
		ReconciledBy: reconciledBy, CompletedAt: now,
	}
	operation := sandbox.CleanupOperation{
		KeyDigest: keyDigest, CleanupID: result.ID, ExecutionID: executionID,
		RunID: result.RunID, ReconciledBy: reconciledBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.CleanupRequestFingerprint(executionID,
		result.CancellationObserved, reconciledBy)
	_, replayed, completeErr := s.store.CompleteSandboxCleanup(ctx, result, operation, lease)
	releaseErr := releaseLease()
	if completeErr != nil || releaseErr != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(errors.Join(completeErr, releaseErr))
	}
	stored, err := s.store.GetSandboxDisabledExecution(ctx, executionID)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetDisabledExecution(ctx context.Context,
	id string,
) (sandbox.Lifecycle, error) {
	if s == nil || s.store == nil {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox lifecycle store is required")
	}
	value, err := s.store.GetSandboxDisabledExecution(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDisabledExecutions(ctx context.Context, runID string,
	limit int,
) ([]sandbox.Lifecycle, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox lifecycle store is required")
	}
	values, err := s.store.ListSandboxDisabledExecutions(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) revalidateExecutionCandidate(ctx context.Context,
	candidate sandbox.ExecutionCandidate, manifest sandbox.Manifest, manifestFingerprint string,
) (sandbox.PreparedIntent, domain.Run, domain.Mission, string, error) {
	intent, err := s.store.GetSandboxManifestIntent(ctx, candidate.PreparationID)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
	}
	if candidate.ManifestFingerprint != manifestFingerprint ||
		intent.Preparation.ManifestFingerprint != manifestFingerprint {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeConflict,
				"resupplied sandbox Manifest does not match the immutable candidate")
	}
	if !intent.Validation.PolicyAllowed || candidate.BackendEnabled || candidate.ExecutionAuthorized {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodePolicyDenied,
				"sandbox candidate is denied or already claims an unsupported capability")
	}
	run, err := s.store.GetRun(ctx, candidate.RunID)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
	}
	if run.Terminal() || run.MissionID != candidate.MissionID {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeFailedPrecondition,
				"terminal or rebound Run cannot enter the sandbox lifecycle")
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
	}
	if mission.ID != candidate.MissionID || mission.WorkspaceID != candidate.WorkspaceID ||
		mission.Scope.WorkspaceID != mission.WorkspaceID ||
		intent.Preparation.RunID != run.ID || intent.Preparation.MissionID != mission.ID ||
		intent.Preparation.WorkspaceID != mission.WorkspaceID {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeConflict,
				"sandbox execution Run, Mission, workspace, or preparation binding changed")
	}
	workspace, err := s.store.GetSandboxWorkspace(ctx, mission.WorkspaceID)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
	}
	rootPath, err := validateSandboxWorkspaceBinding(workspace)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.Wrap(apperror.CodeFailedPrecondition,
				"sandbox execution workspace binding is invalid", err)
	}
	normalizedScope, err := normalizeSandboxMissionScope(mission.Scope)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.Wrap(apperror.CodeFailedPrecondition,
				"sandbox execution Mission scope is invalid", err)
	}
	if err := requireSandboxScopeSubset(manifest.Network, normalizedScope); err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.Wrap(apperror.CodePolicyDenied,
				"sandbox execution attempted to widen Mission scope", err)
	}
	canonicalScope, err := json.Marshal(normalizedScope)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.Wrap(apperror.CodeInternal, "encode sandbox execution scope", err)
	}
	workspaceFingerprint := runmutation.Fingerprint("sandbox_workspace_binding.v1",
		workspace.ID, rootPath)
	scopeFingerprint := runmutation.Fingerprint("sandbox_scope_binding.v1", string(canonicalScope))
	authorizationFingerprint := runmutation.Fingerprint("sandbox_authorization.v1",
		run.ID, mission.ID, workspace.ID, manifestFingerprint, workspaceFingerprint,
		scopeFingerprint)
	if candidate.WorkspaceFingerprint != workspaceFingerprint ||
		candidate.ScopeFingerprint != scopeFingerprint ||
		candidate.AuthorizationFingerprint != authorizationFingerprint ||
		intent.Preparation.WorkspaceFingerprint != workspaceFingerprint ||
		intent.Preparation.ScopeFingerprint != scopeFingerprint ||
		intent.Preparation.AuthorizationFingerprint != authorizationFingerprint {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeConflict,
				"sandbox execution authorization binding changed")
	}
	canonicalManifest, err := manifest.CanonicalJSON()
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.Wrap(apperror.CodeInternal, "encode sandbox execution Manifest", err)
	}
	decision := hardenSandboxDecision(s.checker.CheckToolCall(tools.Call{
		Name: sandboxApprovalToolName, Args: map[string]string{"intent": string(canonicalManifest)},
	}), manifest)
	policyFingerprint := runmutation.Fingerprint("sandbox_policy_decision.v1",
		authorizationFingerprint, fmt.Sprintf("%t", decision.Allowed),
		fmt.Sprintf("%t", decision.NeedsApproval), decision.Risk, decision.Reason)
	if !decision.Allowed {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodePolicyDenied,
				"current Policy denied sandbox lifecycle creation")
	}
	if decision.NeedsApproval != intent.Validation.NeedsApproval ||
		policyFingerprint != candidate.PolicyFingerprint ||
		policyFingerprint != intent.Validation.PolicyFingerprint {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeConflict,
				"sandbox Policy decision changed; prepare a new intent and candidate")
	}
	if intent.Validation.NeedsApproval {
		if candidate.ApprovalID == "" || candidate.ApprovalStatus != sandbox.ApprovalApproved {
			return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
				apperror.New(apperror.CodeFailedPrecondition,
					"sandbox execution candidate is missing exact approval")
		}
		record, err := s.store.GetApproval(ctx, candidate.ApprovalID)
		if err != nil {
			return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
		}
		if err := requireExactSandboxApproval(record, run, intent, true); err != nil {
			return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", err
		}
	} else if candidate.ApprovalID != "" || candidate.ApprovalStatus != sandbox.ApprovalNotRequired {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeConflict,
				"sandbox no-approval candidate gained an unexpected approval binding")
	}
	mountBinding, err := sandbox.ResolveMountSources(ctx, rootPath, manifest)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.Wrap(apperror.CodeFailedPrecondition,
				"sandbox execution mount source resolution failed", err)
	}
	if mountBinding.Fingerprint != candidate.MountBindingFingerprint ||
		mountBinding.MountCount != candidate.MountCount ||
		mountBinding.RegularFileCount != candidate.RegularFileMountCount ||
		mountBinding.DirectoryCount != candidate.DirectoryMountCount {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeConflict,
				"sandbox execution mount source binding changed")
	}
	usage, err := s.store.GetRunAgentUsage(ctx, run.ID)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
	}
	toolUsage, err := s.store.GetToolCallUsage(ctx, run.ID)
	if err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
	}
	if usage.TotalTokens != candidate.TokensUsed ||
		usage.TotalExecutionMillis != candidate.ExecutionMillisUsed ||
		toolUsage.Consumed != candidate.ToolCallsUsed {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeConflict,
				"sandbox execution candidate budget snapshot changed")
	}
	if err := requireSandboxCandidateBudget(run.Budget, usage, toolUsage.Consumed); err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", err
	}
	if lease, found, err := s.store.GetRunExecutionLease(ctx, run.ID); err != nil {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "", apperror.Normalize(err)
	} else if found && lease.ActiveAt(time.Now().UTC()) {
		return sandbox.PreparedIntent{}, domain.Run{}, domain.Mission{}, "",
			apperror.New(apperror.CodeFailedPrecondition,
				"sandbox lifecycle creation requires a quiescent Run")
	}
	return intent, run, mission, rootPath, nil
}

func (s *SandboxManifestService) bindSandboxInputArtifacts(ctx context.Context,
	executionID string, manifest sandbox.Manifest, run domain.Run, workspaceID string,
) ([]sandbox.InputArtifactBinding, int64, error) {
	bindings := make([]sandbox.InputArtifactBinding, 0, len(manifest.InputArtifactIDs))
	var total int64
	for index, artifactID := range manifest.InputArtifactIDs {
		blob, err := s.store.GetRunArtifact(ctx, artifactID)
		if err != nil {
			return nil, 0, apperror.Wrap(apperror.CodeFailedPrecondition,
				"sandbox input Artifact is unavailable or corrupt", err)
		}
		if err := blob.Validate(); err != nil {
			return nil, 0, apperror.Wrap(apperror.CodeFailedPrecondition,
				"sandbox input Artifact integrity check failed", err)
		}
		if blob.RunID != run.ID || blob.SessionID != run.SessionID ||
			blob.WorkspaceID != workspaceID {
			return nil, 0, apperror.New(apperror.CodeConflict,
				"sandbox input Artifact is outside the exact Run, Session, or Workspace scope")
		}
		total += blob.SizeBytes
		if total > sandbox.MaxInputArtifactTotalBytes {
			return nil, 0, apperror.New(apperror.CodeResourceExhausted,
				"sandbox input Artifacts exceed the aggregate 16 MiB limit")
		}
		binding := sandbox.InputArtifactBinding{
			ExecutionID: executionID, Ordinal: index + 1, ArtifactID: blob.ID,
			SHA256: blob.SHA256, SizeBytes: blob.SizeBytes, MIME: blob.MIME,
			Stream: string(blob.Stream), SourceID: blob.SourceID, Redacted: blob.Redacted,
		}
		if err := binding.Validate(); err != nil {
			return nil, 0, apperror.Wrap(apperror.CodeInternal,
				"sandbox input Artifact binding is invalid", err)
		}
		bindings = append(bindings, binding)
	}
	return bindings, total, nil
}

func (s *SandboxManifestService) replayDisabledExecution(ctx context.Context,
	request BeginSandboxExecutionRequest, manifestFingerprint string,
	operation sandbox.ExecutionOperation,
) (sandbox.Lifecycle, error) {
	if operation.CandidateID != request.CandidateID || operation.RequestedBy != request.RequestedBy {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeConflict,
			"sandbox execution operation key was already used for different intent")
	}
	lifecycle, err := s.store.GetSandboxDisabledExecution(ctx, operation.ExecutionID)
	if err != nil {
		return sandbox.Lifecycle{}, apperror.Normalize(err)
	}
	if lifecycle.Execution.CandidateID != request.CandidateID ||
		lifecycle.Execution.ManifestFingerprint != manifestFingerprint ||
		operation.RequestFingerprint != sandbox.ExecutionRequestFingerprint(lifecycle.Execution) {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeConflict,
			"sandbox execution operation key was already used for different intent")
	}
	if lifecycle.Lease.Generation == 1 &&
		lifecycle.Lease.LeaseID == lifecycle.Execution.InitialLeaseID {
		released, _, releaseErr := s.store.ReleaseSandboxExecutionLease(ctx, lifecycle.Lease)
		if releaseErr != nil {
			return sandbox.Lifecycle{}, apperror.Normalize(releaseErr)
		}
		lifecycle.Lease = released
	}
	lifecycle.Replayed = true
	return lifecycle, nil
}

func normalizeSandboxBeginRequest(request BeginSandboxExecutionRequest,
) (BeginSandboxExecutionRequest, error) {
	candidateID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.CandidateID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return BeginSandboxExecutionRequest{}, err
	}
	request.CandidateID, request.OperationKey, request.RequestedBy = candidateID, operationKey, requestedBy
	return request, nil
}

func normalizeSandboxLifecycleOperation(id, operationKey, operator string) (string, string, string, error) {
	originalKey := operationKey
	id, operationKey = strings.TrimSpace(id), strings.TrimSpace(operationKey)
	operator, err := normalizeSandboxOperator(operator)
	if err != nil {
		return "", "", "", err
	}
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return "", "", "", errors.New("sandbox lifecycle identity is invalid")
	}
	if operationKey != originalKey || !utf8.ValidString(operationKey) {
		return "", "", "", errors.New("sandbox lifecycle operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(operationKey); err != nil {
		return "", "", "", err
	}
	for _, current := range operationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return "", "", "", errors.New("sandbox lifecycle operation key cannot contain whitespace or control characters")
		}
	}
	return id, operationKey, operator, nil
}
