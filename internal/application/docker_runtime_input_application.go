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

type ApplyDockerRuntimeInputsRequest struct {
	ProjectionID         string
	Manifest             sandbox.Manifest
	OperationKey         string
	RequestedBy          string
	OwnerID              string
	OperatorConfirmed    bool
	DaemonWriteConfirmed bool
}

type ResumeDockerRuntimeInputsRequest struct {
	IntentID             string
	Manifest             sandbox.Manifest
	RequestedBy          string
	OwnerID              string
	OperatorConfirmed    bool
	DaemonWriteConfirmed bool
}

func (s *SandboxManifestService) ApplyDockerRuntimeInputs(ctx context.Context,
	request ApplyDockerRuntimeInputsRequest,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	if err := s.validateDockerRuntimeInputApplicationService(); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if !request.OperatorConfirmed || !request.DaemonWriteConfirmed {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application requires operator and daemon-write confirmation")
	}
	projectionID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.ProjectionID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input application request is invalid", err)
	}
	ownerID := strings.TrimSpace(request.OwnerID)
	if ownerID == "" {
		ownerID = requestedBy
	}
	manifest, manifestFingerprint, err := validateDockerRuntimeInputApplicationManifest(ctx,
		request.Manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	keyDigest := runmutation.Fingerprint(sandbox.DockerRuntimeInputApplicationOperationVersion,
		projectionID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerRuntimeInputApplicationByOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Normalize(lookupErr)
	} else if found {
		if existing.Intent.ProjectionID != projectionID ||
			existing.Intent.ManifestFingerprint != manifestFingerprint ||
			existing.Intent.RequestedBy != requestedBy {
			return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
				apperror.CodeConflict,
				"Docker runtime input application operation key was used for different intent")
		}
		if existing.Result != nil {
			existing.Replayed = true
			return existing, nil
		}
		acquired, acquireErr := s.store.AcquireDockerRuntimeInputApplication(ctx,
			existing.Intent.ID, requestedBy, ownerID,
			sandbox.DefaultDockerRuntimeInputApplicationLeaseTTL)
		if acquireErr != nil {
			return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Normalize(acquireErr)
		}
		return s.executeDockerRuntimeInputApplication(ctx, acquired, manifest)
	}

	projection, err := s.store.GetDockerRuntimeInputProjectionPlan(ctx, projectionID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Normalize(err)
	}
	if projection.Replayed || projection.ManifestFingerprint != manifestFingerprint ||
		projection.RequestedBy != requestedBy {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input projection does not match apply request")
	}
	intent, err := sandbox.NewDockerRuntimeInputApplicationIntent(
		idgen.New("sandbox-docker-runtime-input-application"), keyDigest, projection,
		s.runtimeInputApply.Endpoint(), request.OperatorConfirmed,
		request.DaemonWriteConfirmed, requestedBy, time.Now().UTC())
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application intent assembly failed", err)
	}
	acquired, err := s.store.BeginDockerRuntimeInputApplication(ctx, intent, ownerID,
		sandbox.DefaultDockerRuntimeInputApplicationLeaseTTL)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Normalize(err)
	}
	if acquired.Replayed {
		return acquired.Record, nil
	}
	return s.executeDockerRuntimeInputApplication(ctx, acquired, manifest)
}

func (s *SandboxManifestService) ResumeDockerRuntimeInputs(ctx context.Context,
	request ResumeDockerRuntimeInputsRequest,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	if err := s.validateDockerRuntimeInputApplicationService(); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if !request.OperatorConfirmed || !request.DaemonWriteConfirmed {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application resume requires operator and daemon-write confirmation")
	}
	intentID, requestedBy, ownerID := strings.TrimSpace(request.IntentID),
		strings.TrimSpace(request.RequestedBy), strings.TrimSpace(request.OwnerID)
	if intentID == "" || requestedBy == "" {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application resume identity is required")
	}
	if ownerID == "" {
		ownerID = requestedBy
	}
	manifest, manifestFingerprint, err := validateDockerRuntimeInputApplicationManifest(ctx,
		request.Manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	existing, err := s.store.GetDockerRuntimeInputApplication(ctx, intentID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Normalize(err)
	}
	if existing.Intent.RequestedBy != requestedBy ||
		existing.Intent.ManifestFingerprint != manifestFingerprint {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input application resume authority changed")
	}
	if existing.Result != nil {
		existing.Replayed = true
		return existing, nil
	}
	acquired, err := s.store.AcquireDockerRuntimeInputApplication(ctx, intentID,
		requestedBy, ownerID, sandbox.DefaultDockerRuntimeInputApplicationLeaseTTL)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Normalize(err)
	}
	return s.executeDockerRuntimeInputApplication(ctx, acquired, manifest)
}

func (s *SandboxManifestService) executeDockerRuntimeInputApplication(ctx context.Context,
	acquired sandbox.DockerRuntimeInputApplicationAcquisition, manifest sandbox.Manifest,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	if acquired.Replayed || acquired.Record.Result != nil {
		return acquired.Record, nil
	}
	projection, err := s.store.GetDockerRuntimeInputProjectionPlan(ctx,
		acquired.Record.Intent.ProjectionID)
	if err != nil {
		return s.failDockerRuntimeInputApplication(ctx, acquired.Record, err)
	}
	rebuilt, err := s.rebuildDockerRuntimeInputApplication(ctx, projection, manifest,
		acquired.Record.Intent.RequestedBy)
	if err != nil {
		return s.failDockerRuntimeInputApplication(ctx, acquired.Record, err)
	}
	request, err := sandbox.NewDockerRuntimeInputApplicationRequest(acquired.Record.Intent,
		projection, rebuilt.compilation, rebuilt.writeRequest)
	if err != nil {
		return s.failDockerRuntimeInputApplication(ctx, acquired.Record, apperror.Wrap(
			apperror.CodeConflict, "Docker runtime input projection changed before apply", err))
	}
	result, err := s.runtimeInputApply.Apply(ctx, acquired.Record.Intent,
		acquired.Record.Lease, request)
	if err != nil {
		return s.failDockerRuntimeInputApplication(ctx, acquired.Record, err)
	}
	record, _, err := s.store.CompleteDockerRuntimeInputApplication(ctx, result,
		acquired.Record.Lease)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Normalize(err)
	}
	return record, nil
}

func (s *SandboxManifestService) failDockerRuntimeInputApplication(_ context.Context,
	record sandbox.DockerRuntimeInputApplicationRecord, cause error,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	code := sandbox.DockerRuntimeInputApplicationErrorCode(cause)
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failed, recordErr := s.store.RecordDockerRuntimeInputApplicationFailure(recordCtx,
		record.Intent.ID, record.Lease, code, time.Now().UTC())
	if recordErr != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, errors.Join(
			apperror.Normalize(cause), apperror.Normalize(recordErr))
	}
	stableCode := apperror.CodeFailedPrecondition
	switch code {
	case sandbox.DockerRuntimeInputApplicationErrorCanceled:
		stableCode = apperror.CodeCancelled
	case sandbox.DockerRuntimeInputApplicationErrorDeadline:
		stableCode = apperror.CodeDeadlineExceeded
	case sandbox.DockerRuntimeInputApplicationErrorConnection:
		stableCode = apperror.CodeUnavailable
	}
	return failed, apperror.Wrap(stableCode,
		"Docker runtime input application failed with "+code, cause)
}

func (s *SandboxManifestService) validateDockerRuntimeInputApplicationService() error {
	if s == nil || s.store == nil || s.checker == nil || s.inspector == nil ||
		s.hostInputStager == nil || s.runtimeInputApply == nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input application dependencies are required")
	}
	return nil
}

func validateDockerRuntimeInputApplicationManifest(ctx context.Context,
	value sandbox.Manifest,
) (sandbox.Manifest, string, error) {
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, value)
	if err != nil {
		return sandbox.Manifest{}, "", apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker runtime input application Manifest validation failed", err)
	}
	fingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.Manifest{}, "", apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker runtime input application Manifest fingerprint failed", err)
	}
	return manifest, fingerprint, nil
}

type dockerRuntimeInputApplicationRebuild struct {
	compilation  sandbox.DockerRuntimeInputProjectionCompilation
	writeRequest sandbox.DockerContainerWriteRequest
}

type dockerRuntimeInputResourceAuthority struct {
	handoff      sandbox.DockerHostInputHandoffRecord
	staging      sandbox.DockerHostInputStagingRecord
	preflight    sandboxPreflightAuthority
	writeRequest sandbox.DockerContainerWriteRequest
}

func (s *SandboxManifestService) rebuildDockerRuntimeInputApplication(ctx context.Context,
	projection sandbox.DockerRuntimeInputProjectionPlan, manifest sandbox.Manifest,
	requestedBy string,
) (dockerRuntimeInputApplicationRebuild, error) {
	rebuilt, err := s.rebuildDockerRuntimeInputResourceAuthority(ctx, projection, manifest,
		requestedBy)
	if err != nil {
		return dockerRuntimeInputApplicationRebuild{}, err
	}
	bundleRequest, err := s.dockerHostInputBundleRequest(ctx, rebuilt.preflight, manifest)
	if err != nil {
		return dockerRuntimeInputApplicationRebuild{}, err
	}
	provider, ok := s.hostInputStager.(sandbox.DockerHostInputBundleProvider)
	if !ok || provider == nil {
		return dockerRuntimeInputApplicationRebuild{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application requires a sealed bundle provider")
	}
	if err := provider.Probe(ctx, rebuilt.preflight.rootPath); err != nil {
		return dockerRuntimeInputApplicationRebuild{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application host input probe failed", err)
	}
	bundle, err := provider.Capture(ctx, bundleRequest)
	if err != nil {
		return dockerRuntimeInputApplicationRebuild{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application bundle recapture failed", err)
	}
	if bundle == nil {
		return dockerRuntimeInputApplicationRebuild{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application provider returned a nil bundle")
	}
	defer bundle.Close()
	report := bundle.Report()
	if report.Validate() != nil || rebuilt.staging.Staging == nil ||
		report.ReportFingerprint != rebuilt.staging.Staging.Report.ReportFingerprint ||
		report.ReportFingerprint != projection.BundleReportFingerprint ||
		report.BundleDigest != projection.BundleDigest || report.BundleBytes != projection.BundleBytes ||
		report.ReadOnlyMountCount != projection.ReadOnlyMountCount ||
		report.ArtifactCount != projection.InputArtifactCount ||
		report.ArtifactBytes != bundleRequest.ArtifactBytes() ||
		report.ArtifactPayloadDigest != bundleRequest.ArtifactPayloadDigest() {
		return dockerRuntimeInputApplicationRebuild{}, apperror.New(
			apperror.CodeConflict, "recaptured Docker runtime input changed after projection")
	}
	frozen := frozenHostInputBundle{HostInputBundle: bundle, report: report}
	compilation, err := sandbox.CompileDockerRuntimeInputProjectionBundle(ctx, manifest,
		frozen, rebuilt.handoff.Handoff.HandoffFingerprint)
	if err != nil {
		return dockerRuntimeInputApplicationRebuild{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application projection recompilation failed", err)
	}
	return dockerRuntimeInputApplicationRebuild{compilation: compilation,
		writeRequest: rebuilt.writeRequest}, nil
}

func (s *SandboxManifestService) rebuildDockerRuntimeInputResourceAuthority(ctx context.Context,
	projection sandbox.DockerRuntimeInputProjectionPlan, manifest sandbox.Manifest,
	requestedBy string,
) (dockerRuntimeInputResourceAuthority, error) {
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil || projection.Validate() != nil || projection.Replayed ||
		projection.ManifestFingerprint != manifestFingerprint ||
		projection.RequestedBy != requestedBy {
		return dockerRuntimeInputResourceAuthority{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input application projection authority is invalid")
	}
	handoff, err := s.store.GetDockerHostInputHandoff(ctx, projection.HandoffIntentID)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Normalize(err)
	}
	if handoff.Handoff == nil || handoff.Handoff.Result.Validate() != nil ||
		handoff.Handoff.ID != projection.HandoffID ||
		handoff.Intent.ID != projection.HandoffIntentID ||
		handoff.Intent.AttemptID != projection.AttemptID ||
		handoff.Intent.PlanID != projection.ContainerPlanID ||
		handoff.Handoff.HandoffFingerprint != projection.HandoffFingerprint ||
		!handoff.Handoff.Result.DaemonConsumed || !handoff.Handoff.Result.ReadbackVerified ||
		!handoff.Handoff.Result.FinalMountReadOnly || !handoff.Handoff.Result.CleanupConfirmed ||
		handoff.Handoff.Result.ContainerStarted || handoff.Handoff.Result.ProcessExecuted ||
		handoff.Handoff.Result.OutputExported ||
		handoff.Handoff.Result.ProductionExecutionSubmitted ||
		handoff.Handoff.Result.ProductionVerified || handoff.Handoff.Result.BackendEnabled ||
		handoff.Handoff.Result.ExecutionAuthorized ||
		handoff.Handoff.Result.ArtifactCommitAuthorized {
		return dockerRuntimeInputResourceAuthority{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input application v59 handoff changed")
	}
	attempt, err := s.store.GetDockerContainerRehearsalAttempt(ctx, projection.AttemptID)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Normalize(err)
	}
	containerPlan, err := s.store.GetDockerContainerPlan(ctx, projection.ContainerPlanID)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Normalize(err)
	}
	staging, err := s.store.GetDockerHostInputStaging(ctx, handoff.Intent.StagingIntentID)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Normalize(err)
	}
	if attempt.Completion == nil ||
		attempt.Status != sandbox.DockerContainerAttemptStatusCompleted ||
		attempt.HostInputRequirement == nil || !attempt.HostInputRequirement.Required ||
		attempt.HostInputHandoffRequirement == nil ||
		!attempt.HostInputHandoffRequirement.Required || staging.Staging == nil ||
		attempt.Intent.ID != projection.AttemptID ||
		attempt.Intent.PlanID != containerPlan.ID ||
		handoff.Handoff.AttemptID != attempt.Intent.ID ||
		handoff.Handoff.PlanID != containerPlan.ID ||
		staging.Intent.ID != handoff.Intent.StagingIntentID ||
		staging.Staging.ID != handoff.Intent.StagingID ||
		staging.Staging.StagingFingerprint != handoff.Intent.StagingFingerprint ||
		staging.Staging.Report.ReportFingerprint != handoff.Intent.BundleReportFingerprint ||
		staging.Staging.Report.BundleDigest != handoff.Intent.BundleDigest ||
		staging.Staging.Report.BundleBytes != handoff.Intent.BundleBytes ||
		containerPlan.ManifestFingerprint != manifestFingerprint ||
		containerPlan.MountBindingFingerprint != projection.MountBindingFingerprint ||
		containerPlan.InputArtifactDigest != projection.InputArtifactDigest ||
		containerPlan.AuthorityFingerprint != projection.AuthorityFingerprint ||
		containerPlan.SpecFingerprint != projection.SpecFingerprint ||
		containerPlan.PlanFingerprint != projection.ContainerPlanFingerprint ||
		containerPlan.RequestedBy != requestedBy || attempt.Intent.RequestedBy != requestedBy ||
		handoff.Intent.RequestedBy != requestedBy || containerPlan.NetworkMode != "disabled" ||
		containerPlan.NetworkTargetCount != 0 || containerPlan.EnvironmentCount != 0 ||
		containerPlan.SecretReferenceCount != 0 || !containerPlan.SimulationOnly ||
		containerPlan.ProductionSubmitted || containerPlan.ProductionVerified ||
		containerPlan.BackendAvailable || containerPlan.BackendEnabled ||
		containerPlan.ExecutionAuthorized || containerPlan.ArtifactCommitAuthorized {
		return dockerRuntimeInputResourceAuthority{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input application v54-v60 authority chain changed")
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, containerPlan.PreflightID)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Normalize(err)
	}
	authority, err := s.revalidateSandboxPreflightAuthority(ctx, preflight, manifest,
		manifestFingerprint, requestedBy)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, err
	}
	observation, err := s.store.GetDockerObservation(ctx, containerPlan.ObservationID)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Normalize(err)
	}
	if authority.run.ID != containerPlan.RunID ||
		authority.mission.ID != containerPlan.MissionID ||
		authority.lifecycle.Execution.ID != containerPlan.ExecutionID ||
		observation.ID != containerPlan.ObservationID || observation.RequestedBy != requestedBy ||
		observation.ManifestFingerprint != manifestFingerprint ||
		observation.Report.Status != sandbox.DockerObservationStatusComplete ||
		!observation.Report.ObservationComplete || !observation.Report.ProductionObserved ||
		observation.Report.ProductionVerified || observation.Report.BackendAvailable ||
		observation.Report.BackendEnabled || observation.Report.ExecutionAuthorized ||
		observation.Report.ArtifactCommitAuthorized {
		return dockerRuntimeInputResourceAuthority{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input application v48-v54 authority changed")
	}
	spec, err := sandbox.CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application specification recompilation failed", err)
	}
	if err := sandbox.DockerContainerPlanMatchesSpec(containerPlan, spec); err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Wrap(
			apperror.CodeConflict, "Docker container plan changed before runtime input application", err)
	}
	writeRequest, err := sandbox.NewDockerContainerWriteRequest(ctx, authority.rootPath, spec)
	if err != nil {
		return dockerRuntimeInputResourceAuthority{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input application host mount resolution failed", err)
	}
	return dockerRuntimeInputResourceAuthority{handoff: handoff, staging: staging,
		preflight: authority, writeRequest: writeRequest}, nil
}

func (s *SandboxManifestService) GetDockerRuntimeInputApplication(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker runtime input application store is required")
	}
	value, err := s.store.GetDockerRuntimeInputApplication(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerRuntimeInputApplications(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputApplicationRecord, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input application store is required")
	}
	values, err := s.store.ListDockerRuntimeInputApplications(ctx,
		strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}
