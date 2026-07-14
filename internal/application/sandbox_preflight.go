package application

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
)

type PrepareSandboxPreflightRequest struct {
	ExecutionID  string
	Manifest     sandbox.Manifest
	OperationKey string
	RequestedBy  string
}

func (s *SandboxManifestService) PrepareDisabledPreflight(ctx context.Context,
	request PrepareSandboxPreflightRequest,
) (sandbox.DisabledPreflight, error) {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox preflight store, policy checker, and backend inspector are required")
	}
	executionID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.ExecutionID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox preflight request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, request.Manifest)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox preflight Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox preflight Manifest fingerprint failed", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_preflight_operation.v1",
		executionID, operationKey)
	if existing, found, lookupErr := s.store.GetSandboxPreflightOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DisabledPreflight{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayDisabledPreflight(ctx, executionID, requestedBy,
			manifestFingerprint, existing)
	}

	lifecycle, err := s.store.GetSandboxDisabledExecution(ctx, executionID)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Normalize(err)
	}
	if lifecycle.Cancellation != nil || lifecycle.Cleanup != nil ||
		lifecycle.Status != sandbox.LifecyclePrepared {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeFailedPrecondition,
			"cancelled or cleaned sandbox execution cannot enter preflight")
	}
	if lifecycle.Lease.Status != sandbox.ExecutionLeaseReleased {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox preflight requires a released execution lease")
	}
	execution := lifecycle.Execution
	if execution.RequestedBy != requestedBy || execution.ManifestFingerprint != manifestFingerprint ||
		execution.BackendEnabled || execution.ExecutionAuthorized || execution.BackendStarted {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeConflict,
			"sandbox preflight does not match the disabled execution")
	}
	validated, err := s.store.GetSandboxExecutionCandidate(ctx, execution.CandidateID)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Normalize(err)
	}
	candidate := validated.Candidate
	intent, run, mission, rootPath, err := s.revalidateExecutionCandidate(ctx, candidate,
		manifest, manifestFingerprint)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	if rootPath == "" || execution.PreparationID != intent.Preparation.ID ||
		execution.CandidateID != candidate.ID || execution.RunID != run.ID ||
		execution.MissionID != mission.ID || execution.WorkspaceID != mission.WorkspaceID ||
		execution.AuthorizationFingerprint != candidate.AuthorizationFingerprint ||
		execution.PolicyFingerprint != candidate.PolicyFingerprint ||
		execution.MountBindingFingerprint != candidate.MountBindingFingerprint ||
		manifest.Backend != intent.Preparation.Backend {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeConflict,
			"sandbox preflight v48-v50 authority chain changed")
	}
	if err := s.reverifySandboxLifecycleInputs(ctx, lifecycle, run.SessionID); err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	legacyOutputPlan := sandbox.NewOutputCapturePlan(manifest)
	if err := legacyOutputPlan.Validate(); err != nil ||
		legacyOutputPlan != execution.OutputPlan {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeConflict,
			"sandbox preflight output capture binding changed")
	}
	handshake, err := s.inspector.Inspect(ctx, manifest.Backend)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Normalize(err)
	}
	if err := handshake.Validate(); err != nil {
		return sandbox.DisabledPreflight{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox backend handshake made an unsupported claim", err)
	}
	outputPlan, err := sandbox.NewOutputExportPlan(manifest)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output export plan is invalid", err)
	}
	now := time.Now().UTC()
	preflight := sandbox.DisabledPreflight{
		ID: idgen.New("sandbox-preflight"), ExecutionID: execution.ID,
		CandidateID: candidate.ID, PreparationID: intent.Preparation.ID,
		RunID: run.ID, MissionID: mission.ID, WorkspaceID: mission.WorkspaceID,
		ProtocolVersion: sandbox.PreflightProtocolVersion, Backend: manifest.Backend,
		ManifestFingerprint:      execution.ManifestFingerprint,
		AuthorizationFingerprint: execution.AuthorizationFingerprint,
		PolicyFingerprint:        execution.PolicyFingerprint,
		MountBindingFingerprint:  execution.MountBindingFingerprint,
		InputArtifactDigest:      execution.InputArtifactDigest,
		Handshake:                handshake, OutputPlan: outputPlan,
		Status:         sandbox.PreflightStatusBackendDisabled,
		BackendEnabled: false, ExecutionAuthorized: false,
		ArtifactCommitAuthorized: false, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation := sandbox.PreflightOperation{
		KeyDigest: keyDigest, PreflightID: preflight.ID, ExecutionID: execution.ID,
		RunID: run.ID, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.PreflightRequestFingerprint(preflight)
	stored, replayed, err := s.store.CreateSandboxDisabledPreflight(ctx, preflight, operation)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetDisabledPreflight(ctx context.Context,
	id string,
) (sandbox.DisabledPreflight, error) {
	if s == nil || s.store == nil {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox preflight store is required")
	}
	value, err := s.store.GetSandboxDisabledPreflight(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDisabledPreflights(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DisabledPreflight, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox preflight store is required")
	}
	values, err := s.store.ListSandboxDisabledPreflights(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) replayDisabledPreflight(ctx context.Context,
	executionID, requestedBy, manifestFingerprint string, operation sandbox.PreflightOperation,
) (sandbox.DisabledPreflight, error) {
	if operation.ExecutionID != executionID || operation.RequestedBy != requestedBy {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeConflict,
			"sandbox preflight operation key was already used for different intent")
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, operation.PreflightID)
	if err != nil {
		return sandbox.DisabledPreflight{}, apperror.Normalize(err)
	}
	if preflight.ExecutionID != executionID || preflight.RequestedBy != requestedBy ||
		preflight.ManifestFingerprint != manifestFingerprint ||
		operation.RequestFingerprint != sandbox.PreflightRequestFingerprint(preflight) ||
		!operation.CreatedAt.Equal(preflight.CreatedAt) {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeConflict,
			"sandbox preflight replay intent changed")
	}
	preflight.Replayed = true
	return preflight, nil
}

func (s *SandboxManifestService) reverifySandboxLifecycleInputs(ctx context.Context,
	lifecycle sandbox.Lifecycle, sessionID string,
) error {
	execution := lifecycle.Execution
	if len(lifecycle.Inputs) != execution.InputArtifactCount {
		return apperror.New(apperror.CodeConflict,
			"sandbox preflight input Artifact count changed")
	}
	var total int64
	for index, binding := range lifecycle.Inputs {
		if binding.ExecutionID != execution.ID || binding.Ordinal != index+1 {
			return apperror.New(apperror.CodeConflict,
				"sandbox preflight input Artifact order changed")
		}
		blob, err := s.store.GetRunArtifact(ctx, binding.ArtifactID)
		if err != nil {
			return apperror.Wrap(apperror.CodeConflict,
				"sandbox preflight input Artifact is unavailable or corrupt", err)
		}
		if err := blob.Validate(); err != nil {
			return apperror.Wrap(apperror.CodeConflict,
				"sandbox preflight input Artifact integrity check failed", err)
		}
		if blob.RunID != execution.RunID || blob.SessionID != sessionID ||
			blob.WorkspaceID != execution.WorkspaceID || blob.ID != binding.ArtifactID ||
			blob.SHA256 != binding.SHA256 || blob.SizeBytes != binding.SizeBytes ||
			blob.MIME != binding.MIME || string(blob.Stream) != binding.Stream ||
			blob.SourceID != binding.SourceID || blob.Redacted != binding.Redacted {
			return apperror.New(apperror.CodeConflict,
				"sandbox preflight input Artifact binding changed")
		}
		total += blob.SizeBytes
		if total > sandbox.MaxInputArtifactTotalBytes {
			return apperror.New(apperror.CodeResourceExhausted,
				"sandbox preflight input Artifacts exceed the aggregate limit")
		}
	}
	if total != execution.InputArtifactBytes ||
		sandbox.InputArtifactBindingsDigest(lifecycle.Inputs) != execution.InputArtifactDigest {
		return apperror.New(apperror.CodeConflict,
			"sandbox preflight input Artifact aggregate changed")
	}
	return nil
}
