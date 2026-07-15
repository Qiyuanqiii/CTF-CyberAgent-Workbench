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

type PlanDockerRuntimeInputsRequest struct {
	HandoffIntentID   string
	Manifest          sandbox.Manifest
	OperationKey      string
	RequestedBy       string
	OperatorConfirmed bool
}

func (s *SandboxManifestService) PlanDockerRuntimeInputs(ctx context.Context,
	request PlanDockerRuntimeInputsRequest,
) (sandbox.DockerRuntimeInputProjectionPlan, error) {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil ||
		s.hostInputStager == nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection store, policy checker, inspector, and sealed input provider are required")
	}
	if !request.OperatorConfirmed {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection requires explicit operator confirmation")
	}
	handoffIntentID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.HandoffIntentID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeInvalidArgument,
			"Docker runtime input projection request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, request.Manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeInvalidArgument,
			"Docker runtime input projection Manifest validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeInvalidArgument,
			"Docker runtime input projection Manifest fingerprint failed", err)
	}
	keyDigest := runmutation.Fingerprint(
		sandbox.DockerRuntimeInputProjectionOperationVersion,
		handoffIntentID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerRuntimeInputProjectionOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayDockerRuntimeInputProjection(ctx, handoffIntentID, requestedBy,
			manifestFingerprint, existing)
	}

	handoff, err := s.store.GetDockerHostInputHandoff(ctx, handoffIntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	if handoff.Handoff == nil || handoff.Handoff.Result.Validate() != nil ||
		!handoff.Handoff.Result.DaemonConsumed ||
		!handoff.Handoff.Result.ReadbackVerified ||
		!handoff.Handoff.Result.FinalMountReadOnly ||
		!handoff.Handoff.Result.CleanupConfirmed ||
		handoff.Handoff.Result.ContainerStarted ||
		handoff.Handoff.Result.ProcessExecuted ||
		handoff.Handoff.Result.OutputExported ||
		handoff.Handoff.Result.ProductionExecutionSubmitted ||
		handoff.Handoff.Result.ProductionVerified ||
		handoff.Handoff.Result.BackendEnabled ||
		handoff.Handoff.Result.ExecutionAuthorized ||
		handoff.Handoff.Result.ArtifactCommitAuthorized {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection requires a completed non-authorizing v59 handoff")
	}
	if existing, found, lookupErr := s.store.GetDockerRuntimeInputProjectionPlanByHandoff(
		ctx, handoff.Handoff.ID); lookupErr != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(lookupErr)
	} else if found {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeConflict,
			"Docker host input handoff already has a runtime projection plan under another operation key: "+existing.ID)
	}
	attempt, err := s.store.GetDockerContainerRehearsalAttempt(ctx,
		handoff.Intent.AttemptID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	containerPlan, err := s.store.GetDockerContainerPlan(ctx, handoff.Intent.PlanID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	staging, err := s.store.GetDockerHostInputStaging(ctx,
		handoff.Intent.StagingIntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	if attempt.Completion == nil ||
		attempt.Status != sandbox.DockerContainerAttemptStatusCompleted ||
		attempt.HostInputRequirement == nil || !attempt.HostInputRequirement.Required ||
		attempt.HostInputHandoffRequirement == nil ||
		!attempt.HostInputHandoffRequirement.Required || staging.Staging == nil ||
		attempt.Intent.ID != handoff.Intent.AttemptID ||
		attempt.Intent.PlanID != containerPlan.ID ||
		handoff.Handoff.AttemptID != attempt.Intent.ID ||
		handoff.Handoff.PlanID != containerPlan.ID ||
		staging.Intent.ID != handoff.Intent.StagingIntentID ||
		staging.Staging.ID != handoff.Intent.StagingID ||
		staging.Staging.StagingFingerprint != handoff.Intent.StagingFingerprint ||
		staging.Staging.Report.ReportFingerprint !=
			handoff.Intent.BundleReportFingerprint ||
		staging.Staging.Report.BundleDigest != handoff.Intent.BundleDigest ||
		staging.Staging.Report.BundleBytes != handoff.Intent.BundleBytes ||
		containerPlan.ManifestFingerprint != manifestFingerprint ||
		containerPlan.RequestedBy != requestedBy ||
		attempt.Intent.RequestedBy != requestedBy || handoff.Intent.RequestedBy != requestedBy ||
		containerPlan.NetworkMode != "disabled" || containerPlan.NetworkTargetCount != 0 ||
		containerPlan.EnvironmentCount != 0 || containerPlan.SecretReferenceCount != 0 ||
		!containerPlan.SimulationOnly || containerPlan.ProductionSubmitted ||
		containerPlan.ProductionVerified || containerPlan.BackendAvailable ||
		containerPlan.BackendEnabled || containerPlan.ExecutionAuthorized ||
		containerPlan.ArtifactCommitAuthorized {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeConflict,
			"Docker runtime input projection v54-v59 authority chain changed")
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, containerPlan.PreflightID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	authority, err := s.revalidateSandboxPreflightAuthority(ctx, preflight, manifest,
		manifestFingerprint, requestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, err
	}
	observation, err := s.store.GetDockerObservation(ctx, containerPlan.ObservationID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	if authority.run.ID != containerPlan.RunID ||
		authority.mission.ID != containerPlan.MissionID ||
		authority.lifecycle.Execution.ID != containerPlan.ExecutionID ||
		observation.ID != containerPlan.ObservationID ||
		observation.RequestedBy != requestedBy ||
		observation.ManifestFingerprint != manifestFingerprint ||
		observation.Report.Status != sandbox.DockerObservationStatusComplete ||
		!observation.Report.ObservationComplete || !observation.Report.ProductionObserved ||
		observation.Report.ProductionVerified || observation.Report.BackendAvailable ||
		observation.Report.BackendEnabled || observation.Report.ExecutionAuthorized ||
		observation.Report.ArtifactCommitAuthorized {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeConflict,
			"Docker runtime input projection v48-v54 authority changed")
	}
	spec, err := sandbox.CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection specification recompilation failed", err)
	}
	if err := sandbox.DockerContainerPlanMatchesSpec(containerPlan, spec); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeConflict,
			"Docker container plan changed before runtime input projection", err)
	}
	bundleRequest, err := s.dockerHostInputBundleRequest(ctx, authority, manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, err
	}
	provider, ok := s.hostInputStager.(sandbox.DockerHostInputBundleProvider)
	if !ok || provider == nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection requires a sealed bundle provider")
	}
	if err := provider.Probe(ctx, authority.rootPath); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection host input probe failed", err)
	}
	bundle, err := provider.Capture(ctx, bundleRequest)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection bundle recapture failed", err)
	}
	if bundle == nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection provider returned a nil bundle")
	}
	defer bundle.Close()
	report := bundle.Report()
	if report.Validate() != nil ||
		report.ReportFingerprint != staging.Staging.Report.ReportFingerprint ||
		report.BundleDigest != staging.Staging.Report.BundleDigest ||
		report.BundleBytes != staging.Staging.Report.BundleBytes ||
		report.ReadOnlyMountCount != containerPlan.ReadOnlyMountCount ||
		report.ArtifactCount != containerPlan.InputArtifactCount ||
		report.ArtifactBytes != bundleRequest.ArtifactBytes() ||
		report.ArtifactPayloadDigest != bundleRequest.ArtifactPayloadDigest() {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeConflict,
			"recaptured Docker host input bundle changed after the v59 handoff")
	}
	frozen := frozenHostInputBundle{HostInputBundle: bundle, report: report}
	compilation, err := sandbox.CompileDockerRuntimeInputProjectionBundle(ctx,
		manifest, frozen, handoff.Handoff.HandoffFingerprint)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection compilation failed", err)
	}
	now := time.Now().UTC()
	plan, err := sandbox.NewDockerRuntimeInputProjectionPlan(
		idgen.New("sandbox-docker-runtime-input-plan"), keyDigest, attempt,
		containerPlan, handoff, compilation, request.OperatorConfirmed, requestedBy, now)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeInternal,
			"Docker runtime input projection plan assembly failed", err)
	}
	operation, err := sandbox.NewDockerRuntimeInputProjectionOperation(keyDigest, plan)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Wrap(
			apperror.CodeInternal,
			"Docker runtime input projection operation assembly failed", err)
	}
	stored, replayed, err := s.store.CreateDockerRuntimeInputProjectionPlan(ctx,
		plan, operation)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetDockerRuntimeInputProjectionPlan(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputProjectionPlan, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input projection store is required")
	}
	value, err := s.store.GetDockerRuntimeInputProjectionPlan(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerRuntimeInputProjectionPlans(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputProjectionPlan, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input projection store is required")
	}
	values, err := s.store.ListDockerRuntimeInputProjectionPlans(ctx,
		strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) replayDockerRuntimeInputProjection(ctx context.Context,
	handoffIntentID, requestedBy, manifestFingerprint string,
	operation sandbox.DockerRuntimeInputProjectionOperation,
) (sandbox.DockerRuntimeInputProjectionPlan, error) {
	value, err := s.store.GetDockerRuntimeInputProjectionPlan(ctx, operation.ProjectionID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.Normalize(err)
	}
	if value.HandoffIntentID != handoffIntentID || value.HandoffID != operation.HandoffID ||
		value.ContainerPlanID != operation.ContainerPlanID || value.RunID != operation.RunID ||
		value.ManifestFingerprint != manifestFingerprint ||
		value.OperationKeyDigest != operation.KeyDigest ||
		value.RequestFingerprint != operation.RequestFingerprint ||
		value.RequestedBy != requestedBy || operation.RequestedBy != requestedBy ||
		!value.CreatedAt.Equal(operation.CreatedAt) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeConflict,
			"Docker runtime input projection replay intent changed")
	}
	value.Replayed = true
	return value, nil
}

type frozenHostInputBundle struct {
	sandbox.HostInputBundle
	report sandbox.HostInputBundleReport
}

func (bundle frozenHostInputBundle) Report() sandbox.HostInputBundleReport {
	return bundle.report
}
