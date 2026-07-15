package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
)

type RehearseDockerContainerRequest struct {
	PlanID                            string
	AttemptID                         string
	Manifest                          sandbox.Manifest
	OperationKey                      string
	RequestedBy                       string
	OperatorConfirmed                 bool
	StageHostInputs                   bool
	OperatorConfirmedHostInputStaging bool
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
	if request.StageHostInputs && !request.OperatorConfirmedHostInputStaging {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeFailedPrecondition,
			"Docker host input staging requires separate explicit operator confirmation")
	}
	if !request.StageHostInputs && request.OperatorConfirmedHostInputStaging {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeInvalidArgument,
			"Docker host input staging confirmation requires staging to be enabled")
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
	hostInputOperationKeyDigest := runmutation.Fingerprint(
		"sandbox_docker_host_input_staging_operation.v1", keyDigest)
	if durableAttempt != nil && !request.StageHostInputs {
		record, found, stagingErr := s.store.GetDockerHostInputStagingByAttempt(ctx,
			durableAttempt.Intent.ID)
		if stagingErr != nil {
			return sandbox.DockerContainerRehearsal{}, apperror.Normalize(stagingErr)
		}
		if found && record.Staging == nil {
			return sandbox.DockerContainerRehearsal{}, apperror.New(
				apperror.CodeFailedPrecondition,
				"Docker attempt has pending host input staging and requires explicit resume confirmation")
		}
	}
	if existing, found, lookupErr := s.store.GetDockerContainerRehearsalOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerContainerRehearsal{}, apperror.Normalize(lookupErr)
	} else if found {
		if request.StageHostInputs {
			record, staged, stagingErr := s.store.GetDockerHostInputStagingByOperation(ctx,
				hostInputOperationKeyDigest)
			if stagingErr != nil {
				return sandbox.DockerContainerRehearsal{}, apperror.Normalize(stagingErr)
			}
			if !staged || record.Staging == nil {
				return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeConflict,
					"completed Docker rehearsal has no matching host input staging evidence")
			}
		}
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
	if request.StageHostInputs {
		record, found, stagingErr := s.store.GetDockerHostInputStagingByOperation(ctx,
			hostInputOperationKeyDigest)
		if stagingErr != nil {
			return sandbox.DockerContainerRehearsal{}, apperror.Normalize(stagingErr)
		}
		if !found || record.Staging == nil {
			if s.hostInputStager == nil {
				return sandbox.DockerContainerRehearsal{}, apperror.New(
					apperror.CodeFailedPrecondition, "Docker host input stager is required")
			}
			if probeErr := s.hostInputStager.Probe(ctx, authority.rootPath); probeErr != nil {
				return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
					apperror.CodeFailedPrecondition, "Docker host input staging probe failed", probeErr)
			}
		}
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
	hostInputs := dockerHostInputStagingExecution{
		enabled: request.StageHostInputs, operationKeyDigest: hostInputOperationKeyDigest,
		manifest: manifest, authority: authority,
	}
	return s.executeDockerContainerRehearsalAttempt(ctx, acquisition, plan, spec, writeRequest,
		hostInputs)
}

type ResumeDockerContainerRequest struct {
	AttemptID                         string
	Manifest                          sandbox.Manifest
	RequestedBy                       string
	OperatorConfirmed                 bool
	StageHostInputs                   bool
	OperatorConfirmedHostInputStaging bool
}

func (s *SandboxManifestService) ResumeDockerContainerRehearsal(ctx context.Context,
	request ResumeDockerContainerRequest,
) (sandbox.DockerContainerRehearsal, error) {
	return s.RehearseDockerContainer(ctx, RehearseDockerContainerRequest{
		AttemptID: request.AttemptID, Manifest: request.Manifest,
		RequestedBy: request.RequestedBy, OperatorConfirmed: request.OperatorConfirmed,
		StageHostInputs:                   request.StageHostInputs,
		OperatorConfirmedHostInputStaging: request.OperatorConfirmedHostInputStaging,
	})
}

type dockerHostInputStagingExecution struct {
	enabled            bool
	operationKeyDigest string
	manifest           sandbox.Manifest
	authority          sandboxPreflightAuthority
}

func (s *SandboxManifestService) executeDockerContainerRehearsalAttempt(ctx context.Context,
	acquisition sandbox.DockerContainerAttemptAcquisition, plan sandbox.DockerContainerPlan,
	spec sandbox.DockerContainerSpec, writeRequest sandbox.DockerContainerWriteRequest,
	hostInputs dockerHostInputStagingExecution,
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
	if err := s.ensureDockerHostInputStaging(ctx, attempt, plan, hostInputs); err != nil {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		cleaned, cleanupErr := s.ensureDockerContainerAttemptCleanup(cleanupCtx, attempt,
			writeRequest, endpoint)
		cancelCleanup()
		if cleanupErr != nil {
			s.recordDockerContainerAttemptFailure(attempt,
				sandbox.DockerContainerAttemptFailureCleanup, cleanupErr)
			return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
				apperror.CodeFailedPrecondition, "Docker host input staging and cleanup failed",
				errors.Join(err, cleanupErr))
		}
		s.recordDockerContainerAttemptFailure(cleaned,
			sandbox.DockerContainerAttemptFailureStage, err)
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
			apperror.CodeFailedPrecondition, "Docker host input staging failed", err)
	}
	var err error
	attempt, err = s.ensureDockerContainerAttemptCleanup(ctx, attempt, writeRequest, endpoint)
	if err != nil {
		s.recordDockerContainerAttemptFailure(attempt,
			sandbox.DockerContainerAttemptFailureCleanup, err)
		return sandbox.DockerContainerRehearsal{}, apperror.Wrap(
			apperror.CodeFailedPrecondition, "Docker exact container cleanup failed", err)
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

func (s *SandboxManifestService) ensureDockerHostInputStaging(ctx context.Context,
	attempt sandbox.DockerContainerRehearsalAttempt, plan sandbox.DockerContainerPlan,
	request dockerHostInputStagingExecution,
) error {
	record, found, err := s.store.GetDockerHostInputStagingByAttempt(ctx, attempt.Intent.ID)
	if err != nil {
		return err
	}
	if found {
		if !dockerHostInputStagingMatchesAttempt(record, attempt, plan,
			request.operationKeyDigest) {
			return apperror.New(apperror.CodeConflict,
				"Docker host input staging record no longer matches the attempt")
		}
		if record.Staging != nil {
			return nil
		}
		if !request.enabled {
			return apperror.New(apperror.CodeFailedPrecondition,
				"Docker attempt has pending host input staging and requires explicit resume confirmation")
		}
	} else if !request.enabled {
		return nil
	}
	if s.hostInputStager == nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Docker host input stager is required")
	}
	if !found {
		intent, intentErr := sandbox.NewDockerHostInputStagingIntent(
			idgen.New("sandbox-docker-host-input-intent"), request.operationKeyDigest,
			attempt, plan, request.manifest, attempt.Intent.RequestedBy, time.Now().UTC())
		if intentErr != nil {
			return apperror.Wrap(apperror.CodeInternal,
				"Docker host input staging intent assembly failed", intentErr)
		}
		record, _, err = s.store.PrepareDockerHostInputStagingIntent(ctx, intent,
			attempt.Lease)
		if err != nil {
			return err
		}
		if !dockerHostInputStagingMatchesAttempt(record, attempt, plan,
			request.operationKeyDigest) {
			return apperror.New(apperror.CodeConflict,
				"Docker host input staging intent replay changed")
		}
	}
	if record.Staging != nil {
		return nil
	}
	bundleRequest, err := s.dockerHostInputBundleRequest(ctx, request.authority,
		request.manifest)
	if err != nil {
		return err
	}
	report, err := s.hostInputStager.Stage(ctx, bundleRequest)
	if err != nil {
		return err
	}
	if report.Validate() != nil ||
		report.ReadOnlyMountCount != bundleRequest.ReadOnlyMountCount() ||
		report.ArtifactCount != len(bundleRequest.Artifacts) ||
		report.ArtifactBytes != bundleRequest.ArtifactBytes() ||
		report.ArtifactPayloadDigest != bundleRequest.ArtifactPayloadDigest() {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Docker host input stager returned mismatched input evidence")
	}
	value, err := sandbox.NewDockerHostInputStaging(
		idgen.New("sandbox-docker-host-input-staging"), record.Intent,
		attempt.Lease.Generation, report, time.Now().UTC())
	if err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"Docker host input staging report is invalid", err)
	}
	record, _, err = s.store.RecordDockerHostInputStaging(ctx, value, attempt.Lease)
	if err != nil {
		return err
	}
	if record.Staging == nil || !dockerHostInputStagingMatchesAttempt(record, attempt,
		plan, request.operationKeyDigest) {
		return apperror.New(apperror.CodeConflict,
			"Docker host input staging commit changed")
	}
	return nil
}

func (s *SandboxManifestService) dockerHostInputBundleRequest(ctx context.Context,
	authority sandboxPreflightAuthority, manifest sandbox.Manifest,
) (sandbox.HostInputBundleRequest, error) {
	if authority.rootPath == "" || authority.run.SessionID == "" {
		return sandbox.HostInputBundleRequest{}, apperror.New(apperror.CodeConflict,
			"Docker host input staging authority is incomplete")
	}
	if err := s.reverifySandboxLifecycleInputs(ctx, authority.lifecycle,
		authority.run.SessionID); err != nil {
		return sandbox.HostInputBundleRequest{}, err
	}
	normalized, err := sandbox.NormalizeManifest(manifest)
	if err != nil {
		return sandbox.HostInputBundleRequest{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker host input staging Manifest is invalid", err)
	}
	if len(normalized.InputArtifactIDs) != len(authority.lifecycle.Inputs) {
		return sandbox.HostInputBundleRequest{}, apperror.New(apperror.CodeConflict,
			"Docker host input staging Artifact count changed")
	}
	artifacts := make([]sandbox.HostInputArtifact, 0, len(authority.lifecycle.Inputs))
	for index, binding := range authority.lifecycle.Inputs {
		blob, loadErr := s.store.GetRunArtifact(ctx, binding.ArtifactID)
		if loadErr != nil {
			return sandbox.HostInputBundleRequest{}, apperror.Wrap(apperror.CodeConflict,
				"Docker host input staging Artifact is unavailable", loadErr)
		}
		if blob.Validate() != nil || binding.Ordinal != index+1 ||
			binding.ArtifactID != normalized.InputArtifactIDs[index] ||
			blob.ID != binding.ArtifactID || blob.RunID != authority.run.ID ||
			blob.SessionID != authority.run.SessionID ||
			blob.WorkspaceID != authority.mission.WorkspaceID || blob.SHA256 != binding.SHA256 ||
			blob.SizeBytes != binding.SizeBytes || blob.MIME != binding.MIME ||
			string(blob.Stream) != binding.Stream || blob.SourceID != binding.SourceID ||
			blob.Redacted != binding.Redacted {
			return sandbox.HostInputBundleRequest{}, apperror.New(apperror.CodeConflict,
				"Docker host input staging Artifact binding changed")
		}
		artifacts = append(artifacts, sandbox.HostInputArtifact{
			Ordinal: binding.Ordinal, ArtifactID: blob.ID, SHA256: blob.SHA256,
			SizeBytes: blob.SizeBytes, MIME: blob.MIME, Stream: string(blob.Stream),
			SourceID: blob.SourceID, Redacted: blob.Redacted, Content: blob.Content,
		})
	}
	request := sandbox.HostInputBundleRequest{WorkspaceRoot: authority.rootPath,
		Manifest: normalized, Artifacts: artifacts}
	if err := request.Validate(); err != nil {
		return sandbox.HostInputBundleRequest{}, apperror.Wrap(apperror.CodeConflict,
			"Docker host input bundle request changed", err)
	}
	return request, nil
}

func dockerHostInputStagingMatchesAttempt(record sandbox.DockerHostInputStagingRecord,
	attempt sandbox.DockerContainerRehearsalAttempt, plan sandbox.DockerContainerPlan,
	operationKeyDigest string,
) bool {
	intent := record.Intent
	if record.Validate() != nil || attempt.Stage == nil || intent.AttemptID != attempt.Intent.ID ||
		intent.PlanID != plan.ID || intent.RunID != plan.RunID ||
		intent.MissionID != plan.MissionID || intent.WorkspaceID != plan.WorkspaceID ||
		intent.OperationKeyDigest != operationKeyDigest ||
		intent.AttemptIntentFingerprint != attempt.Intent.IntentFingerprint ||
		intent.RequestFingerprint != attempt.Intent.RequestFingerprint ||
		intent.ContainerIDFingerprint != attempt.Stage.Result.ContainerIDFingerprint ||
		intent.ManifestFingerprint != plan.ManifestFingerprint ||
		intent.MountBindingFingerprint != plan.MountBindingFingerprint ||
		intent.InputArtifactDigest != plan.InputArtifactDigest ||
		intent.AuthorityFingerprint != plan.AuthorityFingerprint ||
		intent.SpecFingerprint != plan.SpecFingerprint ||
		intent.PlanFingerprint != plan.PlanFingerprint ||
		intent.ReadOnlyMountCount != plan.ReadOnlyMountCount ||
		intent.InputArtifactCount != plan.InputArtifactCount ||
		intent.RequestedBy != plan.RequestedBy ||
		intent.PreparedGeneration > attempt.Lease.Generation {
		return false
	}
	return record.Staging == nil || record.Staging.LeaseGeneration <= attempt.Lease.Generation
}

func (s *SandboxManifestService) ensureDockerContainerAttemptCleanup(ctx context.Context,
	attempt sandbox.DockerContainerRehearsalAttempt,
	writeRequest sandbox.DockerContainerWriteRequest, endpoint sandbox.DockerObservationEndpoint,
) (sandbox.DockerContainerRehearsalAttempt, error) {
	if attempt.Cleanup != nil {
		return attempt, nil
	}
	cleanupResult, err := s.dockerWriteTransport.Cleanup(ctx, writeRequest,
		attempt.Stage.Result)
	if err != nil {
		return attempt, err
	}
	if cleanupResult.Validate() != nil ||
		cleanupResult.EndpointFingerprint != endpoint.Fingerprint ||
		cleanupResult.RequestFingerprint != writeRequest.RequestFingerprint ||
		cleanupResult.ContainerIDFingerprint != attempt.Stage.Result.ContainerIDFingerprint ||
		cleanupResult.ContainerStarted || cleanupResult.ProcessExecuted ||
		cleanupResult.OutputExported || cleanupResult.ExecutionAuthorized ||
		cleanupResult.ArtifactCommitAuthorized {
		return attempt, errors.New("docker cleanup transport returned an unsupported authority claim")
	}
	cleanup, err := sandbox.NewDockerContainerAttemptCleanup(attempt.Intent.ID,
		attempt.Lease.Generation, cleanupResult, time.Now().UTC())
	if err != nil {
		return attempt, fmt.Errorf("docker container cleanup checkpoint assembly failed: %w", err)
	}
	stored, _, err := s.store.RecordDockerContainerAttemptCleanup(ctx, cleanup, attempt.Lease)
	if err != nil {
		return attempt, err
	}
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
	if code := sandbox.DockerHostInputStagingErrorCode(err); code != "" {
		switch code {
		case sandbox.DockerHostInputStagingErrorUnsafeSource,
			sandbox.DockerHostInputStagingErrorResourceLimit:
			return sandbox.DockerContainerAttemptFailureCheckpoint, false
		default:
			return sandbox.DockerContainerAttemptFailureCheckpoint, true
		}
	}
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

func (s *SandboxManifestService) GetDockerHostInputStaging(ctx context.Context,
	id string,
) (sandbox.DockerHostInputStagingRecord, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerHostInputStagingRecord{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker host input staging store is required")
	}
	value, err := s.store.GetDockerHostInputStaging(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerHostInputStagings(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerHostInputStagingRecord, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker host input staging store is required")
	}
	values, err := s.store.ListDockerHostInputStagings(ctx, strings.TrimSpace(runID), limit)
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
