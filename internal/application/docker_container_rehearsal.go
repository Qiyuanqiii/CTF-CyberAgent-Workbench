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

type RehearseDockerContainerRequest struct {
	PlanID            string
	Manifest          sandbox.Manifest
	OperationKey      string
	RequestedBy       string
	OperatorConfirmed bool
}

func (s *SandboxManifestService) RehearseDockerContainer(ctx context.Context,
	request RehearseDockerContainerRequest,
) (sandbox.DockerContainerRehearsal, error) {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil ||
		s.dockerWriteTransport == nil {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker rehearsal store, policy checker, inspectors, and write transport are required")
	}
	if !request.OperatorConfirmed {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container rehearsal requires explicit operator confirmation")
	}
	planID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.PlanID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container rehearsal request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, request.Manifest)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container rehearsal Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container rehearsal Manifest fingerprint failed", err)
	}
	keyDigest := runmutation.Fingerprint("sandbox_docker_container_rehearsal_operation.v1",
		planID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerContainerRehearsalOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayDockerContainerRehearsal(ctx, planID, requestedBy,
			manifestFingerprint, existing)
	}

	plan, err := s.store.GetDockerContainerPlan(ctx, planID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	if plan.RequestedBy != requestedBy || plan.ManifestFingerprint != manifestFingerprint ||
		plan.NetworkMode != "disabled" || plan.NetworkTargetCount != 0 ||
		plan.EnvironmentCount != 0 || plan.SecretReferenceCount != 0 ||
		!plan.SimulationOnly || plan.ProductionSubmitted || plan.ProductionVerified ||
		plan.BackendAvailable || plan.BackendEnabled || plan.ExecutionAuthorized ||
		plan.ArtifactCommitAuthorized {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker rehearsal requires an exact network-disabled, environment-free v54 plan")
	}

	observation, err := s.store.GetDockerObservation(ctx, plan.ObservationID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	evidence, err := s.store.GetSandboxBackendEvidence(ctx, plan.EvidenceID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	simulation, err := s.store.GetSandboxOutputSimulation(ctx, plan.OutputSimulationID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, plan.PreflightID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	authority, err := s.revalidateSandboxPreflightAuthority(ctx, preflight, manifest,
		manifestFingerprint, requestedBy)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, err
	}
	if authority.rootPath == "" || plan.ObservationID != observation.ID ||
		plan.EvidenceID != observation.EvidenceID ||
		plan.OutputSimulationID != observation.OutputSimulationID ||
		plan.PreflightID != observation.PreflightID || plan.ExecutionID != observation.ExecutionID ||
		plan.CandidateID != observation.CandidateID ||
		plan.PreparationID != observation.PreparationID || plan.RunID != observation.RunID ||
		plan.MissionID != observation.MissionID || plan.WorkspaceID != observation.WorkspaceID ||
		plan.ManifestFingerprint != observation.ManifestFingerprint ||
		plan.AuthorizationFingerprint != observation.AuthorizationFingerprint ||
		plan.PolicyFingerprint != observation.PolicyFingerprint ||
		plan.MountBindingFingerprint != observation.MountBindingFingerprint ||
		plan.InputArtifactDigest != observation.InputArtifactDigest ||
		plan.ThreatModelFingerprint != observation.ThreatModelFingerprint ||
		plan.OutputPlanFingerprint != observation.OutputPlanFingerprint ||
		plan.ObservationFingerprint != observation.Report.ObservationFingerprint ||
		plan.AuthorityFingerprint != sandbox.DockerContainerAuthorityFingerprint(observation) ||
		plan.ImageDigest != observation.Report.ImageDigest ||
		plan.OSType != observation.Report.ImageOSType ||
		plan.Architecture != observation.Report.ImageArchitecture ||
		observation.RequestedBy != requestedBy ||
		observation.Report.Status != sandbox.DockerObservationStatusComplete ||
		!observation.Report.ObservationComplete || !observation.Report.ProductionObserved ||
		observation.Report.ProductionVerified || observation.Report.BackendAvailable ||
		observation.Report.BackendEnabled || observation.Report.ExecutionAuthorized ||
		observation.Report.ArtifactCommitAuthorized ||
		!sandbox.DockerObservationSupportsContainerWrite(observation.Report) ||
		evidence.ID != plan.EvidenceID || evidence.PreflightID != plan.PreflightID ||
		evidence.ExecutionID != plan.ExecutionID || evidence.CandidateID != plan.CandidateID ||
		evidence.PreparationID != plan.PreparationID || evidence.RunID != plan.RunID ||
		evidence.MissionID != plan.MissionID || evidence.WorkspaceID != plan.WorkspaceID ||
		evidence.RequestedBy != requestedBy || evidence.ManifestFingerprint != manifestFingerprint ||
		evidence.AuthorizationFingerprint != plan.AuthorizationFingerprint ||
		evidence.PolicyFingerprint != plan.PolicyFingerprint ||
		evidence.MountBindingFingerprint != plan.MountBindingFingerprint ||
		evidence.InputArtifactDigest != plan.InputArtifactDigest ||
		evidence.ThreatModelFingerprint != plan.ThreatModelFingerprint ||
		evidence.Report.OutputPlanFingerprint != plan.OutputPlanFingerprint ||
		evidence.Report.ProductionVerified || evidence.Report.BackendAvailable ||
		evidence.Report.BackendEnabled || evidence.Report.ExecutionAuthorized ||
		evidence.Report.ArtifactCommitAuthorized || simulation.ID != plan.OutputSimulationID ||
		simulation.EvidenceID != evidence.ID || simulation.PreflightID != plan.PreflightID ||
		simulation.ExecutionID != plan.ExecutionID || simulation.RunID != plan.RunID ||
		simulation.MissionID != plan.MissionID || simulation.WorkspaceID != plan.WorkspaceID ||
		simulation.RequestedBy != requestedBy ||
		simulation.OutputPlanFingerprint != plan.OutputPlanFingerprint || !simulation.SimulationOnly ||
		simulation.ProductionArtifactCount != 0 || simulation.BackendEnabled ||
		simulation.ExecutionAuthorized || simulation.ArtifactCommitAuthorized ||
		authority.lifecycle.Execution.ID != plan.ExecutionID || authority.run.ID != plan.RunID ||
		authority.mission.ID != plan.MissionID {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeConflict,
			"Docker container rehearsal v48-v54 authority chain changed")
	}

	spec, err := sandbox.CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Docker container specification recompilation failed", err)
	}
	if err := sandbox.DockerContainerPlanMatchesSpec(plan, spec); err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeConflict,
			"Docker v54 plan changed before rehearsal", err)
	}
	writeRequest, err := sandbox.NewDockerContainerWriteRequest(ctx, authority.rootPath, spec)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Docker rehearsal host mount resolution failed", err)
	}
	result, err := s.dockerWriteTransport.Rehearse(ctx, writeRequest)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Docker create-inspect-remove rehearsal failed", err)
	}
	endpoint := s.dockerWriteTransport.Endpoint()
	if err := endpoint.Validate(); err != nil || endpoint.Class != sandbox.DockerObservationEndpointLocalUnix ||
		result.EndpointClass != endpoint.Class || result.EndpointFingerprint != endpoint.Fingerprint ||
		result.RequestFingerprint != writeRequest.RequestFingerprint ||
		result.SpecFingerprint != spec.SpecFingerprint || result.ContainerStarted ||
		result.ProcessExecuted || result.ImagePulled || result.OutputExported ||
		result.ProductionExecutionSubmitted || result.ProductionVerified ||
		result.BackendEnabled || result.ExecutionAuthorized || result.ArtifactCommitAuthorized {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker write transport returned an unsupported authority claim")
	}
	now := time.Now().UTC()
	rehearsal, err := sandbox.NewDockerContainerRehearsal(
		idgen.New("sandbox-docker-rehearsal"), plan, spec, result, requestedBy, now)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInternal,
			"Docker container rehearsal assembly failed", err)
	}
	operation := sandbox.DockerContainerRehearsalOperation{
		KeyDigest: keyDigest, RehearsalID: rehearsal.ID, PlanID: plan.ID,
		RunID: plan.RunID, RequestedBy: requestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.DockerContainerRehearsalRequestFingerprint(rehearsal)
	stored, replayed, err := s.store.CreateDockerContainerRehearsal(ctx, rehearsal, operation)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetDockerContainerRehearsal(ctx context.Context,
	id string,
) (sandbox.DockerContainerRehearsal, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container rehearsal store is required")
	}
	value, err := s.store.GetDockerContainerRehearsal(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerContainerRehearsals(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerContainerRehearsal, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container rehearsal store is required")
	}
	values, err := s.store.ListDockerContainerRehearsals(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) replayDockerContainerRehearsal(ctx context.Context,
	planID, requestedBy, manifestFingerprint string,
	operation sandbox.DockerContainerRehearsalOperation,
) (sandbox.DockerContainerRehearsal, error) {
	if operation.PlanID != planID || operation.RequestedBy != requestedBy {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeConflict,
			"Docker rehearsal operation key was used for different intent")
	}
	value, err := s.store.GetDockerContainerRehearsal(ctx, operation.RehearsalID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	if value.PlanID != planID || value.RequestedBy != requestedBy ||
		value.ManifestFingerprint != manifestFingerprint ||
		operation.RequestFingerprint != sandbox.DockerContainerRehearsalRequestFingerprint(value) ||
		!operation.CreatedAt.Equal(value.CreatedAt) {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeConflict,
			"Docker rehearsal replay intent changed")
	}
	value.Replayed = true
	return value, nil
}
