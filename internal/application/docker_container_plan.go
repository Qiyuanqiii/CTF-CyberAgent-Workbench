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

type CompileDockerContainerPlanRequest struct {
	ObservationID string
	Manifest      sandbox.Manifest
	OperationKey  string
	RequestedBy   string
}

func (s *SandboxManifestService) CompileDockerContainerPlan(ctx context.Context,
	request CompileDockerContainerPlanRequest,
) (sandbox.DockerContainerPlan, error) {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil ||
		s.dockerWriter == nil {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container plan store, policy checker, inspectors, and fake writer are required")
	}
	observationID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.ObservationID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container plan request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, request.Manifest)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container plan Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container plan Manifest fingerprint failed", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_docker_container_plan_operation.v1",
		observationID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerContainerPlanOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerContainerPlan{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayDockerContainerPlan(ctx, observationID, requestedBy,
			manifestFingerprint, existing)
	}

	observation, err := s.store.GetDockerObservation(ctx, observationID)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Normalize(err)
	}
	if observation.RequestedBy != requestedBy ||
		observation.ManifestFingerprint != manifestFingerprint ||
		observation.Report.Status != sandbox.DockerObservationStatusComplete ||
		!observation.Report.ObservationComplete || !observation.Report.ProductionObserved ||
		observation.Report.ProductionVerified || observation.Report.BackendAvailable ||
		observation.Report.BackendEnabled || observation.Report.ExecutionAuthorized ||
		observation.Report.ArtifactCommitAuthorized {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container plan requires a matching complete, non-authorizing v53 observation")
	}
	evidence, err := s.store.GetSandboxBackendEvidence(ctx, observation.EvidenceID)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Normalize(err)
	}
	simulation, err := s.store.GetSandboxOutputSimulation(ctx, observation.OutputSimulationID)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Normalize(err)
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, observation.PreflightID)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Normalize(err)
	}
	authority, err := s.revalidateSandboxPreflightAuthority(ctx, preflight, manifest,
		manifestFingerprint, requestedBy)
	if err != nil {
		return sandbox.DockerContainerPlan{}, err
	}
	if evidence.ID != observation.EvidenceID || evidence.PreflightID != observation.PreflightID ||
		evidence.RequestedBy != requestedBy ||
		evidence.ExecutionID != observation.ExecutionID ||
		evidence.CandidateID != observation.CandidateID ||
		evidence.PreparationID != observation.PreparationID || evidence.RunID != observation.RunID ||
		evidence.MissionID != observation.MissionID ||
		evidence.WorkspaceID != observation.WorkspaceID ||
		simulation.ID != observation.OutputSimulationID || simulation.EvidenceID != evidence.ID ||
		simulation.RequestedBy != requestedBy ||
		simulation.PreflightID != evidence.PreflightID || simulation.ExecutionID != evidence.ExecutionID ||
		simulation.RunID != evidence.RunID || simulation.MissionID != evidence.MissionID ||
		simulation.WorkspaceID != evidence.WorkspaceID ||
		authority.lifecycle.Execution.ID != evidence.ExecutionID || authority.run.ID != evidence.RunID ||
		authority.mission.ID != evidence.MissionID ||
		observation.ManifestFingerprint != evidence.ManifestFingerprint ||
		observation.AuthorizationFingerprint != evidence.AuthorizationFingerprint ||
		observation.PolicyFingerprint != evidence.PolicyFingerprint ||
		observation.MountBindingFingerprint != evidence.MountBindingFingerprint ||
		observation.InputArtifactDigest != evidence.InputArtifactDigest ||
		observation.ThreatModelFingerprint != evidence.ThreatModelFingerprint ||
		observation.OutputPlanFingerprint != evidence.Report.OutputPlanFingerprint ||
		observation.OutputPlanFingerprint != simulation.OutputPlanFingerprint ||
		observation.Report.ImageDigest != evidence.Report.ImageDigest ||
		evidence.Report.ProductionVerified || evidence.Report.BackendAvailable ||
		evidence.Report.BackendEnabled || evidence.Report.ExecutionAuthorized ||
		evidence.Report.ArtifactCommitAuthorized || !simulation.SimulationOnly ||
		simulation.ProductionArtifactCount != 0 || simulation.BackendEnabled ||
		simulation.ExecutionAuthorized || simulation.ArtifactCommitAuthorized {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeConflict,
			"Docker container plan v48-v53 authority chain changed")
	}

	spec, err := sandbox.CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Docker container specification compilation failed", err)
	}
	transaction, err := s.dockerWriter.Simulate(ctx, spec)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Docker fake write transaction failed", err)
	}
	if err := transaction.Validate(); err != nil || transaction.SpecFingerprint != spec.SpecFingerprint ||
		transaction.DaemonWriteCount != 0 || transaction.BackendTouched ||
		transaction.ProductionSubmitted || transaction.ProductionVerified ||
		transaction.BackendEnabled || transaction.ExecutionAuthorized ||
		transaction.ArtifactCommitAuthorized {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker fake writer made an unsupported production claim")
	}
	now := time.Now().UTC()
	plan, err := sandbox.NewDockerContainerPlan(idgen.New("sandbox-docker-plan"), observation,
		spec, transaction, requestedBy, now)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Wrap(apperror.CodeInternal,
			"Docker container plan assembly failed", err)
	}
	operation := sandbox.DockerContainerPlanOperation{
		KeyDigest: keyDigest, PlanID: plan.ID, ObservationID: observation.ID,
		RunID: observation.RunID, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.DockerContainerPlanRequestFingerprint(plan)
	stored, replayed, err := s.store.CreateDockerContainerPlan(ctx, plan, operation)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetDockerContainerPlan(ctx context.Context,
	id string,
) (sandbox.DockerContainerPlan, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container plan store is required")
	}
	value, err := s.store.GetDockerContainerPlan(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerContainerPlans(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerContainerPlan, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container plan store is required")
	}
	values, err := s.store.ListDockerContainerPlans(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) replayDockerContainerPlan(ctx context.Context,
	observationID, requestedBy, manifestFingerprint string,
	operation sandbox.DockerContainerPlanOperation,
) (sandbox.DockerContainerPlan, error) {
	if operation.ObservationID != observationID || operation.RequestedBy != requestedBy {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeConflict,
			"Docker container plan operation key was used for different intent")
	}
	plan, err := s.store.GetDockerContainerPlan(ctx, operation.PlanID)
	if err != nil {
		return sandbox.DockerContainerPlan{}, apperror.Normalize(err)
	}
	if plan.ObservationID != observationID || plan.RequestedBy != requestedBy ||
		plan.ManifestFingerprint != manifestFingerprint ||
		operation.RequestFingerprint != sandbox.DockerContainerPlanRequestFingerprint(plan) ||
		!operation.CreatedAt.Equal(plan.CreatedAt) {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeConflict,
			"Docker container plan replay intent changed")
	}
	plan.Replayed = true
	return plan, nil
}
