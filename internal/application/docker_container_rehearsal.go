package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
)

type RehearseDockerContainerRequest struct {
	PlanID            string
	AttemptID         string
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
	var durableAttempt *sandbox.DockerContainerRehearsalAttempt
	planID, requestedBy := "", strings.TrimSpace(request.RequestedBy)
	keyDigest := ""
	if attemptID := strings.TrimSpace(request.AttemptID); attemptID != "" {
		if strings.TrimSpace(request.PlanID) != "" || strings.TrimSpace(request.OperationKey) != "" {
			return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeInvalidArgument,
				"Docker attempt resume cannot also select a plan or operation key")
		}
		attempt, err := s.store.GetDockerContainerRehearsalAttempt(ctx, attemptID)
		if err != nil {
			return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
		}
		if requestedBy == "" || requestedBy != attempt.Intent.RequestedBy {
			return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeConflict,
				"Docker container attempt requester changed")
		}
		durableAttempt, planID, keyDigest = &attempt, attempt.Intent.PlanID,
			attempt.Intent.OperationKeyDigest
	} else {
		var operationKey string
		var err error
		planID, operationKey, requestedBy, err = normalizeSandboxLifecycleOperation(
			request.PlanID, request.OperationKey, request.RequestedBy)
		if err != nil {
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInvalidArgument,
				"Docker container rehearsal request is invalid", err)
		}
		keyDigest = runmutation.Fingerprint(
			"sandbox_docker_container_rehearsal_operation.v1", planID, operationKey)
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
	endpoint := s.dockerWriteTransport.Endpoint()
	if err := endpoint.Validate(); err != nil ||
		endpoint.Class != sandbox.DockerObservationEndpointLocalUnix {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker write transport endpoint is outside the fixed local boundary")
	}
	now := time.Now().UTC()
	intent, err := sandbox.NewDockerContainerAttemptIntent(
		idgen.New("sandbox-docker-attempt"), keyDigest, plan, writeRequest, endpoint,
		requestedBy, now)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInternal,
			"Docker container attempt assembly failed", err)
	}
	ownerID := idgen.New("sandbox-docker-attempt-owner")
	var acquisition sandbox.DockerContainerAttemptAcquisition
	if durableAttempt == nil {
		acquisition, err = s.store.BeginDockerContainerRehearsalAttempt(ctx, intent,
			ownerID, sandbox.DefaultDockerContainerAttemptLeaseTTL)
	} else {
		if intent.IntentFingerprint != durableAttempt.Intent.IntentFingerprint ||
			intent.RequestFingerprint != durableAttempt.Intent.RequestFingerprint ||
			intent.PlanID != durableAttempt.Intent.PlanID {
			return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeConflict,
				"Docker container attempt resume intent changed")
		}
		acquisition, err = s.store.AcquireDockerContainerRehearsalAttempt(ctx,
			durableAttempt.Intent.ID, requestedBy, ownerID,
			sandbox.DefaultDockerContainerAttemptLeaseTTL)
	}
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	return s.executeDockerContainerRehearsalAttempt(ctx, acquisition, plan, spec, writeRequest)
}

type ResumeDockerContainerRequest struct {
	AttemptID         string
	Manifest          sandbox.Manifest
	RequestedBy       string
	OperatorConfirmed bool
}

func (s *SandboxManifestService) ResumeDockerContainerRehearsal(ctx context.Context,
	request ResumeDockerContainerRequest,
) (sandbox.DockerContainerRehearsal, error) {
	return s.RehearseDockerContainer(ctx, RehearseDockerContainerRequest{
		AttemptID: request.AttemptID, Manifest: request.Manifest,
		RequestedBy: request.RequestedBy, OperatorConfirmed: request.OperatorConfirmed,
	})
}

func (s *SandboxManifestService) executeDockerContainerRehearsalAttempt(ctx context.Context,
	acquisition sandbox.DockerContainerAttemptAcquisition, plan sandbox.DockerContainerPlan,
	spec sandbox.DockerContainerSpec, writeRequest sandbox.DockerContainerWriteRequest,
) (sandbox.DockerContainerRehearsal, error) {
	attempt := acquisition.Attempt
	if attempt.Completion != nil {
		value, err := s.store.GetDockerContainerRehearsal(ctx,
			attempt.Completion.RehearsalID)
		if err != nil {
			return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
		}
		value.Replayed = true
		return value, nil
	}
	endpoint := s.dockerWriteTransport.Endpoint()
	if attempt.Stage == nil {
		stageResult, err := s.dockerWriteTransport.Stage(ctx, writeRequest)
		if err != nil {
			s.recordDockerContainerAttemptFailure(attempt,
				sandbox.DockerContainerAttemptFailureStage, err)
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
				apperror.CodeFailedPrecondition, "Docker container staging failed", err)
		}
		if stageResult.Validate() != nil || stageResult.EndpointClass != endpoint.Class ||
			stageResult.EndpointFingerprint != endpoint.Fingerprint ||
			stageResult.RequestFingerprint != writeRequest.RequestFingerprint ||
			stageResult.SpecFingerprint != spec.SpecFingerprint || stageResult.ContainerStarted ||
			stageResult.ProcessExecuted || stageResult.ImagePulled || stageResult.OutputExported ||
			stageResult.ProductionExecutionSubmitted || stageResult.ProductionVerified ||
			stageResult.BackendEnabled || stageResult.ExecutionAuthorized ||
			stageResult.ArtifactCommitAuthorized {
			claimErr := errors.New("docker stage transport returned an unsupported authority claim")
			s.recordDockerContainerAttemptFailure(attempt,
				sandbox.DockerContainerAttemptFailureStage, claimErr)
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
				apperror.CodeFailedPrecondition, claimErr.Error(), claimErr)
		}
		stage, err := sandbox.NewDockerContainerAttemptStage(attempt.Intent.ID,
			attempt.Lease.Generation, stageResult, time.Now().UTC())
		if err != nil {
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInternal,
				"Docker container stage checkpoint assembly failed", err)
		}
		stored, _, err := s.store.RecordDockerContainerAttemptStage(ctx, stage, attempt.Lease)
		if err != nil {
			s.recordDockerContainerAttemptFailure(attempt,
				sandbox.DockerContainerAttemptFailureStage, err)
			return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
		}
		attempt = stored
	}
	if attempt.Cleanup == nil {
		cleanupResult, err := s.dockerWriteTransport.Cleanup(ctx, writeRequest,
			attempt.Stage.Result)
		if err != nil {
			s.recordDockerContainerAttemptFailure(attempt,
				sandbox.DockerContainerAttemptFailureCleanup, err)
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
				apperror.CodeFailedPrecondition, "Docker exact container cleanup failed", err)
		}
		if cleanupResult.Validate() != nil ||
			cleanupResult.EndpointFingerprint != endpoint.Fingerprint ||
			cleanupResult.RequestFingerprint != writeRequest.RequestFingerprint ||
			cleanupResult.ContainerIDFingerprint !=
				attempt.Stage.Result.ContainerIDFingerprint || cleanupResult.ContainerStarted ||
			cleanupResult.ProcessExecuted || cleanupResult.OutputExported ||
			cleanupResult.ExecutionAuthorized || cleanupResult.ArtifactCommitAuthorized {
			claimErr := errors.New("docker cleanup transport returned an unsupported authority claim")
			s.recordDockerContainerAttemptFailure(attempt,
				sandbox.DockerContainerAttemptFailureCleanup, claimErr)
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
				apperror.CodeFailedPrecondition, claimErr.Error(), claimErr)
		}
		cleanup, err := sandbox.NewDockerContainerAttemptCleanup(attempt.Intent.ID,
			attempt.Lease.Generation, cleanupResult, time.Now().UTC())
		if err != nil {
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInternal,
				"Docker container cleanup checkpoint assembly failed", err)
		}
		stored, _, err := s.store.RecordDockerContainerAttemptCleanup(ctx, cleanup,
			attempt.Lease)
		if err != nil {
			s.recordDockerContainerAttemptFailure(attempt,
				sandbox.DockerContainerAttemptFailureCleanup, err)
			return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
		}
		attempt = stored
	}
	result, err := sandbox.NewDockerContainerWriteResultFromRecovery(endpoint, writeRequest,
		attempt.Stage.Result, attempt.Cleanup.Result)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInternal,
			"Docker recovery result assembly failed", err)
	}
	now := time.Now().UTC()
	rehearsal, err := sandbox.NewDockerContainerRehearsal(
		idgen.New("sandbox-docker-rehearsal"), plan, spec, result,
		attempt.Intent.RequestedBy, now)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInternal,
			"Docker container rehearsal assembly failed", err)
	}
	operation := sandbox.DockerContainerRehearsalOperation{
		KeyDigest: attempt.Intent.OperationKeyDigest, RehearsalID: rehearsal.ID,
		PlanID: plan.ID, RunID: plan.RunID, RequestedBy: attempt.Intent.RequestedBy,
		CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.DockerContainerRehearsalRequestFingerprint(rehearsal)
	completion, err := sandbox.NewDockerContainerAttemptCompletion(attempt.Intent.ID,
		rehearsal.ID, attempt.Lease.Generation, now)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(apperror.CodeInternal,
			"Docker container attempt completion assembly failed", err)
	}
	stored, replayed, err := s.store.CompleteDockerContainerRehearsalAttempt(ctx,
		completion, rehearsal, operation, attempt.Lease)
	if err != nil {
		s.recordDockerContainerAttemptFailure(attempt,
			sandbox.DockerContainerAttemptFailureCompletion, err)
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) recordDockerContainerAttemptFailure(
	attempt sandbox.DockerContainerRehearsalAttempt, phase string, cause error,
) {
	if attempt.Completion != nil || len(attempt.Failures) >= sandbox.MaxDockerContainerAttemptFailures {
		return
	}
	code, retryable := dockerContainerAttemptFailureMetadata(cause)
	failure, err := sandbox.NewDockerContainerAttemptFailure(attempt.Intent.ID,
		len(attempt.Failures)+1, attempt.Lease.Generation, phase, code, retryable,
		time.Now().UTC())
	if err != nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.store.FailDockerContainerRehearsalAttempt(recordCtx, failure, attempt.Lease)
}

func dockerContainerAttemptFailureMetadata(err error) (string, bool) {
	if code := sandbox.DockerContainerWriteErrorCode(err); code != "" {
		switch code {
		case sandbox.DockerContainerWriteFailureUnsafeExisting,
			sandbox.DockerContainerWriteFailureUnsafeImage,
			sandbox.DockerContainerWriteFailureConfigMismatch:
			return code, false
		default:
			return code, true
		}
	}
	if errors.Is(err, context.Canceled) {
		return sandbox.DockerContainerAttemptFailureCanceled, true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return sandbox.DockerContainerAttemptFailureDeadline, true
	}
	return sandbox.DockerContainerAttemptFailureCheckpoint, true
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

func (s *SandboxManifestService) GetDockerContainerRehearsalAttempt(ctx context.Context,
	id string,
) (sandbox.DockerContainerRehearsalAttempt, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerContainerRehearsalAttempt{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker container attempt store is required")
	}
	value, err := s.store.GetDockerContainerRehearsalAttempt(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerContainerRehearsalAttempts(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerContainerRehearsalAttempt, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker container attempt store is required")
	}
	values, err := s.store.ListDockerContainerRehearsalAttempts(ctx,
		strings.TrimSpace(runID), limit)
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
