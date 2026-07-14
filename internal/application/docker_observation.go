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

type ObserveDockerBackendRequest struct {
	EvidenceID         string
	OutputSimulationID string
	Manifest           sandbox.Manifest
	OperationKey       string
	RequestedBy        string
}

func (s *SandboxManifestService) ObserveDockerBackend(ctx context.Context,
	request ObserveDockerBackendRequest,
) (sandbox.DockerObservation, error) {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil ||
		s.dockerObserver == nil {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker observation store, policy checker, inspectors, and read-only observer are required")
	}
	evidenceID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.EvidenceID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker observation request is invalid", err)
	}
	simulationID := strings.TrimSpace(request.OutputSimulationID)
	if simulationID != request.OutputSimulationID || !domain.ValidAgentID(simulationID) ||
		strings.ContainsRune(simulationID, 0) {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeInvalidArgument,
			"Docker observation output simulation id is invalid")
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, request.Manifest)
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker observation Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker observation Manifest fingerprint failed", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_docker_observation_operation.v1",
		evidenceID, simulationID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerObservationOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerObservation{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayDockerObservation(ctx, evidenceID, simulationID, requestedBy,
			manifestFingerprint, existing)
	}

	evidence, err := s.store.GetSandboxBackendEvidence(ctx, evidenceID)
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Normalize(err)
	}
	simulation, err := s.store.GetSandboxOutputSimulation(ctx, simulationID)
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Normalize(err)
	}
	if simulation.EvidenceID != evidence.ID || evidence.ManifestFingerprint != manifestFingerprint ||
		evidence.RequestedBy != requestedBy || simulation.RequestedBy != requestedBy ||
		evidence.Report.TrustClass != sandbox.BackendEvidenceTrustSimulation ||
		evidence.Report.ProductionVerified || evidence.Report.BackendAvailable ||
		evidence.Report.BackendEnabled || evidence.Report.ExecutionAuthorized ||
		evidence.Report.ArtifactCommitAuthorized || !simulation.SimulationOnly ||
		simulation.ProductionArtifactCount != 0 || simulation.BackendEnabled ||
		simulation.ExecutionAuthorized || simulation.ArtifactCommitAuthorized {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeConflict,
			"Docker observation requires matching non-authorizing v52 evidence and output simulation")
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, evidence.PreflightID)
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Normalize(err)
	}
	authority, err := s.revalidateSandboxPreflightAuthority(ctx, preflight, manifest,
		manifestFingerprint, requestedBy)
	if err != nil {
		return sandbox.DockerObservation{}, err
	}
	if authority.lifecycle.Execution.ID != evidence.ExecutionID ||
		authority.run.ID != evidence.RunID || simulation.ExecutionID != evidence.ExecutionID ||
		simulation.PreflightID != evidence.PreflightID ||
		simulation.OutputPlanFingerprint != evidence.Report.OutputPlanFingerprint {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeConflict,
			"Docker observation v48-v52 authority chain changed")
	}

	observation := sandbox.DockerObservation{
		EvidenceID: evidence.ID, OutputSimulationID: simulation.ID,
		PreflightID: evidence.PreflightID, ExecutionID: evidence.ExecutionID,
		CandidateID: evidence.CandidateID, PreparationID: evidence.PreparationID,
		RunID: evidence.RunID, MissionID: evidence.MissionID, WorkspaceID: evidence.WorkspaceID,
		ManifestFingerprint:      evidence.ManifestFingerprint,
		AuthorizationFingerprint: evidence.AuthorizationFingerprint,
		PolicyFingerprint:        evidence.PolicyFingerprint,
		MountBindingFingerprint:  evidence.MountBindingFingerprint,
		InputArtifactDigest:      evidence.InputArtifactDigest,
		ThreatModelFingerprint:   evidence.ThreatModelFingerprint,
		OutputPlanFingerprint:    evidence.Report.OutputPlanFingerprint,
		Report:                   sandbox.DockerObservationReport{ImageDigest: evidence.Report.ImageDigest},
		RequestedBy:              requestedBy,
	}
	bindingFingerprint := sandbox.DockerObservationBindingFingerprint(observation)
	report, err := s.dockerObserver.Observe(ctx, sandbox.DockerObservationProbeRequest{
		BindingFingerprint: bindingFingerprint, ImageDigest: evidence.Report.ImageDigest,
	})
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"read-only Docker observation failed", err)
	}
	if err := report.Validate(); err != nil || report.BindingFingerprint != bindingFingerprint ||
		report.ImageDigest != evidence.Report.ImageDigest || report.ProductionVerified ||
		report.BackendAvailable || report.BackendEnabled || report.ExecutionAuthorized ||
		report.ArtifactCommitAuthorized {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeFailedPrecondition,
			"read-only Docker observer made an unsupported claim")
	}
	now := time.Now().UTC()
	observation.ID = idgen.New("sandbox-docker-observation")
	observation.Report = report
	observation.CreatedAt = now
	if err := observation.Validate(); err != nil {
		return sandbox.DockerObservation{}, apperror.Wrap(apperror.CodeInternal,
			"Docker observation assembly failed", err)
	}
	operation := sandbox.DockerObservationOperation{
		KeyDigest: keyDigest, ObservationID: observation.ID, EvidenceID: evidence.ID,
		OutputSimulationID: simulation.ID, RunID: evidence.RunID,
		RequestedBy: requestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.DockerObservationRequestFingerprint(observation)
	stored, replayed, err := s.store.CreateDockerObservation(ctx, observation, operation)
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetDockerObservation(ctx context.Context,
	id string,
) (sandbox.DockerObservation, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker observation store is required")
	}
	value, err := s.store.GetDockerObservation(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerObservations(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerObservation, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker observation store is required")
	}
	values, err := s.store.ListDockerObservations(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) replayDockerObservation(ctx context.Context,
	evidenceID, simulationID, requestedBy, manifestFingerprint string,
	operation sandbox.DockerObservationOperation,
) (sandbox.DockerObservation, error) {
	if operation.EvidenceID != evidenceID || operation.OutputSimulationID != simulationID ||
		operation.RequestedBy != requestedBy {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeConflict,
			"Docker observation operation key was used for different intent")
	}
	observation, err := s.store.GetDockerObservation(ctx, operation.ObservationID)
	if err != nil {
		return sandbox.DockerObservation{}, apperror.Normalize(err)
	}
	if observation.EvidenceID != evidenceID ||
		observation.OutputSimulationID != simulationID || observation.RequestedBy != requestedBy ||
		observation.ManifestFingerprint != manifestFingerprint ||
		operation.RequestFingerprint != sandbox.DockerObservationRequestFingerprint(observation) ||
		!operation.CreatedAt.Equal(observation.CreatedAt) {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeConflict,
			"Docker observation replay intent changed")
	}
	observation.Replayed = true
	return observation, nil
}
