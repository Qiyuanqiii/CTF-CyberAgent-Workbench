package application

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
)

type RecordSandboxBackendEvidenceRequest struct {
	PreflightID  string
	Manifest     sandbox.Manifest
	ImageDigest  string
	OperationKey string
	RequestedBy  string
}

func (s *SandboxManifestService) RecordSimulatedBackendEvidence(ctx context.Context,
	request RecordSandboxBackendEvidenceRequest,
) (sandbox.BackendEvidence, error) {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil ||
		s.evidenceClient == nil {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox evidence store, policy checker, inspectors, and fake client are required")
	}
	preflightID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.PreflightID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.BackendEvidence{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox evidence request is invalid", err)
	}
	imageDigest := strings.TrimSpace(request.ImageDigest)
	if imageDigest != request.ImageDigest || !sandbox.ValidOCIImageDigest(imageDigest) {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox evidence requires a normalized OCI sha256 image digest")
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, request.Manifest)
	if err != nil {
		return sandbox.BackendEvidence{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox evidence Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.BackendEvidence{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox evidence Manifest fingerprint failed", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_backend_evidence_operation.v1",
		preflightID, operationKey)
	if existing, found, lookupErr := s.store.GetSandboxBackendEvidenceOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.BackendEvidence{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayBackendEvidence(ctx, preflightID, requestedBy, manifestFingerprint,
			imageDigest, existing)
	}

	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, preflightID)
	if err != nil {
		return sandbox.BackendEvidence{}, apperror.Normalize(err)
	}
	authority, err := s.revalidateSandboxPreflightAuthority(ctx, preflight, manifest,
		manifestFingerprint, requestedBy)
	if err != nil {
		return sandbox.BackendEvidence{}, err
	}
	if manifest.Backend != sandbox.BackendDocker {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox backend evidence simulation currently requires a Docker Manifest")
	}
	report, err := s.evidenceClient.Probe(ctx, sandbox.BackendEvidenceProbeRequest{
		PreflightID: preflight.ID, Backend: preflight.Backend, Manifest: manifest,
		ManifestFingerprint:    preflight.ManifestFingerprint,
		ThreatModelFingerprint: preflight.Handshake.ThreatModelFingerprint,
		OutputPlanFingerprint:  preflight.OutputPlan.Fingerprint, ImageDigest: imageDigest,
	})
	if err != nil {
		return sandbox.BackendEvidence{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox fake backend evidence probe failed", err)
	}
	if err := report.Validate(); err != nil {
		return sandbox.BackendEvidence{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox fake backend evidence made an unsupported claim", err)
	}
	now := time.Now().UTC()
	evidence := sandbox.BackendEvidence{
		ID: idgen.New("sandbox-evidence"), PreflightID: preflight.ID,
		ExecutionID: preflight.ExecutionID, CandidateID: preflight.CandidateID,
		PreparationID: preflight.PreparationID, RunID: preflight.RunID,
		MissionID: preflight.MissionID, WorkspaceID: preflight.WorkspaceID,
		ManifestFingerprint:      preflight.ManifestFingerprint,
		AuthorizationFingerprint: preflight.AuthorizationFingerprint,
		PolicyFingerprint:        preflight.PolicyFingerprint,
		MountBindingFingerprint:  preflight.MountBindingFingerprint,
		InputArtifactDigest:      preflight.InputArtifactDigest,
		ThreatModelFingerprint:   preflight.Handshake.ThreatModelFingerprint,
		Report:                   report, RequestedBy: requestedBy, CreatedAt: now,
	}
	if evidence.ExecutionID != authority.lifecycle.Execution.ID ||
		evidence.RunID != authority.run.ID || evidence.MissionID != authority.mission.ID {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeConflict,
			"sandbox evidence authority chain changed during simulation")
	}
	operation := sandbox.BackendEvidenceOperation{
		KeyDigest: keyDigest, EvidenceID: evidence.ID, PreflightID: preflight.ID,
		RunID: preflight.RunID, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.BackendEvidenceRequestFingerprint(evidence)
	stored, replayed, err := s.store.CreateSandboxBackendEvidence(ctx, evidence, operation)
	if err != nil {
		return sandbox.BackendEvidence{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetBackendEvidence(ctx context.Context,
	id string,
) (sandbox.BackendEvidence, error) {
	if s == nil || s.store == nil {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox evidence store is required")
	}
	value, err := s.store.GetSandboxBackendEvidence(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListBackendEvidence(ctx context.Context,
	runID string, limit int,
) ([]sandbox.BackendEvidence, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox evidence store is required")
	}
	values, err := s.store.ListSandboxBackendEvidence(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

type SimulateSandboxOutputRequest struct {
	EvidenceID   string
	Manifest     sandbox.Manifest
	Fixture      sandbox.OutputFixture
	OperationKey string
	RequestedBy  string
}

func (s *SandboxManifestService) SimulateOutputTransaction(ctx context.Context,
	request SimulateSandboxOutputRequest,
) (sandbox.OutputSimulation, error) {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil ||
		s.outputHarness == nil {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox output simulation store, policy checker, inspector, and harness are required")
	}
	evidenceID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.EvidenceID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output simulation request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, request.Manifest)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output simulation Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output simulation Manifest fingerprint failed", err)
	}
	evidence, err := s.store.GetSandboxBackendEvidence(ctx, evidenceID)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Normalize(err)
	}
	if evidence.ManifestFingerprint != manifestFingerprint || evidence.RequestedBy != requestedBy {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeConflict,
			"sandbox output simulation does not match its evidence or operator")
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, evidence.PreflightID)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Normalize(err)
	}
	plan, err := sandbox.NewOutputExportPlan(manifest)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output simulation plan is invalid", err)
	}
	if plan.Fingerprint != preflight.OutputPlan.Fingerprint ||
		plan.Fingerprint != evidence.Report.OutputPlanFingerprint {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeConflict,
			"sandbox output simulation plan binding changed")
	}
	result, err := s.outputHarness.Simulate(ctx, plan, request.Fixture)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox in-memory output transaction failed", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_output_simulation_operation.v1",
		evidenceID, operationKey)
	if existing, found, lookupErr := s.store.GetSandboxOutputSimulationOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.OutputSimulation{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayOutputSimulation(ctx, evidence, requestedBy, result, existing)
	}
	authority, err := s.revalidateSandboxPreflightAuthority(ctx, preflight, manifest,
		manifestFingerprint, requestedBy)
	if err != nil {
		return sandbox.OutputSimulation{}, err
	}
	if authority.lifecycle.Execution.ID != evidence.ExecutionID ||
		authority.run.ID != evidence.RunID || evidence.Report.ProductionVerified ||
		evidence.Report.BackendAvailable || evidence.Report.BackendEnabled ||
		evidence.Report.ExecutionAuthorized || evidence.Report.ArtifactCommitAuthorized {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeConflict,
			"sandbox output simulation evidence authority changed")
	}
	now := time.Now().UTC()
	simulation := sandbox.OutputSimulation{
		ID: idgen.New("sandbox-output-sim"), EvidenceID: evidence.ID,
		PreflightID: evidence.PreflightID, ExecutionID: evidence.ExecutionID,
		RunID: evidence.RunID, MissionID: evidence.MissionID, WorkspaceID: evidence.WorkspaceID,
		ProtocolVersion:       sandbox.OutputSimulationProtocolVersion,
		Status:                sandbox.OutputSimulationStatusCommitted,
		OutputPlanFingerprint: plan.Fingerprint, FixtureDigest: result.FixtureDigest,
		TransactionDigest: result.TransactionDigest, ExpectedSlotCount: plan.SlotCount,
		StagedOutputCount: len(result.Descriptors), StagedOutputBytes: result.TotalBytes,
		FakeArtifactCount: result.FakeCommitCount, ProductionArtifactCount: 0,
		AllOrNothing: true, SimulationOnly: true, ArtifactCommitAuthorized: false,
		BackendEnabled: false, ExecutionAuthorized: false,
		Descriptors: result.Descriptors, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation := sandbox.OutputSimulationOperation{
		KeyDigest: keyDigest, SimulationID: simulation.ID, EvidenceID: evidence.ID,
		RunID: evidence.RunID, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.OutputSimulationRequestFingerprint(simulation)
	stored, replayed, err := s.store.CreateSandboxOutputSimulation(ctx, simulation, operation)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetOutputSimulation(ctx context.Context,
	id string,
) (sandbox.OutputSimulation, error) {
	if s == nil || s.store == nil {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox output simulation store is required")
	}
	value, err := s.store.GetSandboxOutputSimulation(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListOutputSimulations(ctx context.Context,
	runID string, limit int,
) ([]sandbox.OutputSimulation, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox output simulation store is required")
	}
	values, err := s.store.ListSandboxOutputSimulations(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

type sandboxPreflightAuthority struct {
	lifecycle sandbox.Lifecycle
	run       domain.Run
	mission   domain.Mission
}

func (s *SandboxManifestService) revalidateSandboxPreflightAuthority(ctx context.Context,
	preflight sandbox.DisabledPreflight, manifest sandbox.Manifest,
	manifestFingerprint, requestedBy string,
) (sandboxPreflightAuthority, error) {
	if err := preflight.Validate(); err != nil {
		return sandboxPreflightAuthority{}, apperror.Wrap(apperror.CodeInternal,
			"stored sandbox preflight is invalid", err)
	}
	if preflight.RequestedBy != requestedBy || preflight.ManifestFingerprint != manifestFingerprint ||
		preflight.Backend != manifest.Backend || preflight.BackendEnabled ||
		preflight.ExecutionAuthorized || preflight.ArtifactCommitAuthorized {
		return sandboxPreflightAuthority{}, apperror.New(apperror.CodeConflict,
			"sandbox preflight operator, Manifest, or disabled authority changed")
	}
	lifecycle, err := s.store.GetSandboxDisabledExecution(ctx, preflight.ExecutionID)
	if err != nil {
		return sandboxPreflightAuthority{}, apperror.Normalize(err)
	}
	if lifecycle.Cancellation != nil || lifecycle.Cleanup != nil ||
		lifecycle.Status != sandbox.LifecyclePrepared ||
		lifecycle.Lease.Status != sandbox.ExecutionLeaseReleased {
		return sandboxPreflightAuthority{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox preflight authority requires an uncancelled execution and released lease")
	}
	execution := lifecycle.Execution
	validated, err := s.store.GetSandboxExecutionCandidate(ctx, execution.CandidateID)
	if err != nil {
		return sandboxPreflightAuthority{}, apperror.Normalize(err)
	}
	candidate := validated.Candidate
	intent, run, mission, rootPath, err := s.revalidateExecutionCandidate(ctx, candidate,
		manifest, manifestFingerprint)
	if err != nil {
		return sandboxPreflightAuthority{}, err
	}
	if rootPath == "" || preflight.ExecutionID != execution.ID ||
		preflight.CandidateID != candidate.ID || preflight.PreparationID != intent.Preparation.ID ||
		preflight.RunID != run.ID || preflight.MissionID != mission.ID ||
		preflight.WorkspaceID != mission.WorkspaceID ||
		preflight.AuthorizationFingerprint != execution.AuthorizationFingerprint ||
		preflight.AuthorizationFingerprint != candidate.AuthorizationFingerprint ||
		preflight.PolicyFingerprint != execution.PolicyFingerprint ||
		preflight.PolicyFingerprint != candidate.PolicyFingerprint ||
		preflight.MountBindingFingerprint != execution.MountBindingFingerprint ||
		preflight.MountBindingFingerprint != candidate.MountBindingFingerprint ||
		preflight.InputArtifactDigest != execution.InputArtifactDigest {
		return sandboxPreflightAuthority{}, apperror.New(apperror.CodeConflict,
			"sandbox v48-v51 authority chain changed")
	}
	if err := s.reverifySandboxLifecycleInputs(ctx, lifecycle, run.SessionID); err != nil {
		return sandboxPreflightAuthority{}, err
	}
	legacyPlan := sandbox.NewOutputCapturePlan(manifest)
	if err := legacyPlan.Validate(); err != nil || legacyPlan != execution.OutputPlan {
		return sandboxPreflightAuthority{}, apperror.New(apperror.CodeConflict,
			"sandbox output capture binding changed")
	}
	outputPlan, err := sandbox.NewOutputExportPlan(manifest)
	if err != nil || outputPlan.Fingerprint != preflight.OutputPlan.Fingerprint {
		return sandboxPreflightAuthority{}, apperror.New(apperror.CodeConflict,
			"sandbox output export binding changed")
	}
	handshake, err := s.inspector.Inspect(ctx, manifest.Backend)
	if err != nil {
		return sandboxPreflightAuthority{}, apperror.Normalize(err)
	}
	if err := handshake.Validate(); err != nil ||
		handshake.ThreatModelFingerprint != preflight.Handshake.ThreatModelFingerprint ||
		handshake.Status != preflight.Handshake.Status || handshake.Available {
		return sandboxPreflightAuthority{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox disabled backend handshake changed")
	}
	return sandboxPreflightAuthority{lifecycle: lifecycle, run: run, mission: mission}, nil
}

func (s *SandboxManifestService) replayBackendEvidence(ctx context.Context,
	preflightID, requestedBy, manifestFingerprint, imageDigest string,
	operation sandbox.BackendEvidenceOperation,
) (sandbox.BackendEvidence, error) {
	if operation.PreflightID != preflightID || operation.RequestedBy != requestedBy {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeConflict,
			"sandbox evidence operation key was used for different intent")
	}
	evidence, err := s.store.GetSandboxBackendEvidence(ctx, operation.EvidenceID)
	if err != nil {
		return sandbox.BackendEvidence{}, apperror.Normalize(err)
	}
	if evidence.PreflightID != preflightID || evidence.RequestedBy != requestedBy ||
		evidence.ManifestFingerprint != manifestFingerprint || evidence.Report.ImageDigest != imageDigest ||
		operation.RequestFingerprint != sandbox.BackendEvidenceRequestFingerprint(evidence) ||
		!operation.CreatedAt.Equal(evidence.CreatedAt) {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeConflict,
			"sandbox evidence replay intent changed")
	}
	evidence.Replayed = true
	return evidence, nil
}

func (s *SandboxManifestService) replayOutputSimulation(ctx context.Context,
	evidence sandbox.BackendEvidence, requestedBy string, result sandbox.OutputSimulationResult,
	operation sandbox.OutputSimulationOperation,
) (sandbox.OutputSimulation, error) {
	if operation.EvidenceID != evidence.ID || operation.RequestedBy != requestedBy {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeConflict,
			"sandbox output simulation operation key was used for different intent")
	}
	simulation, err := s.store.GetSandboxOutputSimulation(ctx, operation.SimulationID)
	if err != nil {
		return sandbox.OutputSimulation{}, apperror.Normalize(err)
	}
	if simulation.EvidenceID != evidence.ID || simulation.RequestedBy != requestedBy ||
		simulation.FixtureDigest != result.FixtureDigest ||
		simulation.TransactionDigest != result.TransactionDigest ||
		operation.RequestFingerprint != sandbox.OutputSimulationRequestFingerprint(simulation) ||
		!operation.CreatedAt.Equal(simulation.CreatedAt) {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeConflict,
			"sandbox output simulation replay intent changed")
	}
	simulation.Replayed = true
	return simulation, nil
}
