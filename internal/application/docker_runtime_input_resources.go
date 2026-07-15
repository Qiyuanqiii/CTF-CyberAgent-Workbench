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

type InspectDockerRuntimeInputResourcesRequest struct {
	ApplicationIntentID string
	Manifest            sandbox.Manifest
	OperationKey        string
	RequestedBy         string
	OperatorConfirmed   bool
}

type CleanupDockerRuntimeInputResourcesRequest struct {
	InspectionID         string
	Manifest             sandbox.Manifest
	OperationKey         string
	RequestedBy          string
	OwnerID              string
	OperatorConfirmed    bool
	DaemonWriteConfirmed bool
}

type ResumeDockerRuntimeInputResourceCleanupRequest struct {
	IntentID             string
	Manifest             sandbox.Manifest
	RequestedBy          string
	OwnerID              string
	OperatorConfirmed    bool
	DaemonWriteConfirmed bool
}

func (s *SandboxManifestService) InspectDockerRuntimeInputResources(ctx context.Context,
	request InspectDockerRuntimeInputResourcesRequest,
) (sandbox.DockerRuntimeInputResourceInspection, error) {
	if err := s.validateDockerRuntimeInputResourceInspectionService(); err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, err
	}
	if !request.OperatorConfirmed {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input resource inspection requires read-only probe confirmation")
	}
	applicationID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.ApplicationIntentID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input resource inspection request is invalid", err)
	}
	manifest, manifestFingerprint, err := validateDockerRuntimeInputApplicationManifest(ctx,
		request.Manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, err
	}
	keyDigest := runmutation.Fingerprint(sandbox.DockerRuntimeInputResourceInspectionOperationVersion,
		applicationID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerRuntimeInputResourceInspectionByOperation(
		ctx, keyDigest); lookupErr != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.Normalize(lookupErr)
	} else if found {
		if existing.ApplicationIntentID != applicationID ||
			existing.ManifestFingerprint != manifestFingerprint ||
			existing.RequestedBy != requestedBy {
			return sandbox.DockerRuntimeInputResourceInspection{}, apperror.New(
				apperror.CodeConflict,
				"Docker runtime input resource inspection operation key was used for a different request")
		}
		existing.Replayed = true
		return finishDockerRuntimeInputResourceInspection(existing)
	}
	application, projection, descriptor, err := s.rebuildDockerRuntimeInputResourceDescriptor(
		ctx, applicationID, manifest, requestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, err
	}
	if projection.ID != application.Intent.ProjectionID ||
		s.runtimeResourceRead.Endpoint().Fingerprint != application.Intent.EndpointFingerprint {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input resource inspection endpoint authority changed")
	}
	observation, err := s.runtimeResourceRead.Inspect(ctx, descriptor)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, normalizeDockerRuntimeInputResourceError(
			"Docker runtime input resource inspection failed", err)
	}
	inspection, err := sandbox.NewDockerRuntimeInputResourceInspection(
		idgen.New("sandbox-docker-runtime-input-resource-inspection"), keyDigest, requestedBy,
		application, descriptor, observation)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.Wrap(
			apperror.CodeConflict, "Docker runtime input resource inspection evidence is invalid", err)
	}
	inspection, _, err = s.store.RecordDockerRuntimeInputResourceInspection(ctx, inspection)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.Normalize(err)
	}
	return finishDockerRuntimeInputResourceInspection(inspection)
}

func finishDockerRuntimeInputResourceInspection(
	value sandbox.DockerRuntimeInputResourceInspection,
) (sandbox.DockerRuntimeInputResourceInspection, error) {
	if value.Status == sandbox.DockerRuntimeInputResourceInspectionUnsafe {
		return value, apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input resource inspection found a foreign or changed resource")
	}
	return value, nil
}

func (s *SandboxManifestService) CleanupDockerRuntimeInputResources(ctx context.Context,
	request CleanupDockerRuntimeInputResourcesRequest,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	if err := s.validateDockerRuntimeInputResourceCleanupService(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if !request.OperatorConfirmed || !request.DaemonWriteConfirmed {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input resource cleanup requires operator and daemon-write confirmation")
	}
	inspectionID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.InspectionID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup request is invalid", err)
	}
	ownerID := strings.TrimSpace(request.OwnerID)
	if ownerID == "" {
		ownerID = requestedBy
	}
	manifest, manifestFingerprint, err := validateDockerRuntimeInputApplicationManifest(ctx,
		request.Manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	keyDigest := runmutation.Fingerprint(sandbox.DockerRuntimeInputResourceCleanupOperationVersion,
		inspectionID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerRuntimeInputResourceCleanupByOperation(
		ctx, keyDigest); lookupErr != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Normalize(lookupErr)
	} else if found {
		if existing.Intent.InspectionID != inspectionID ||
			existing.Intent.ManifestFingerprint != manifestFingerprint ||
			existing.Intent.RequestedBy != requestedBy {
			return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
				apperror.CodeConflict,
				"Docker runtime input resource cleanup operation key was used for a different intent")
		}
		if existing.Result != nil {
			existing.Replayed = true
			return existing, nil
		}
		acquired, acquireErr := s.store.AcquireDockerRuntimeInputResourceCleanup(ctx,
			existing.Intent.ID, requestedBy, ownerID,
			sandbox.DefaultDockerRuntimeInputResourceCleanupLeaseTTL)
		if acquireErr != nil {
			return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Normalize(acquireErr)
		}
		return s.executeDockerRuntimeInputResourceCleanup(ctx, acquired, manifest)
	}
	inspection, err := s.store.GetDockerRuntimeInputResourceInspection(ctx, inspectionID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Normalize(err)
	}
	if !inspection.CleanupEligible || inspection.ForeignResourceCount != 0 ||
		inspection.RequestedBy != requestedBy ||
		inspection.ManifestFingerprint != manifestFingerprint {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input resource inspection is not eligible for exact cleanup")
	}
	_, _, descriptor, err := s.rebuildDockerRuntimeInputResourceDescriptor(ctx,
		inspection.ApplicationIntentID, manifest, requestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if err := validateDockerRuntimeInputResourceInspectionDescriptor(inspection, descriptor); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	endpoint := s.runtimeResourceClean.Endpoint()
	if endpoint.Fingerprint != inspection.EndpointFingerprint {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input resource cleanup endpoint changed")
	}
	intent, err := sandbox.NewDockerRuntimeInputResourceCleanupIntent(
		idgen.New("sandbox-docker-runtime-input-resource-cleanup"), keyDigest, inspection,
		descriptor, endpoint, request.OperatorConfirmed, request.DaemonWriteConfirmed,
		requestedBy, time.Now().UTC())
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Docker runtime input resource cleanup intent assembly failed", err)
	}
	acquired, err := s.store.BeginDockerRuntimeInputResourceCleanup(ctx, intent, ownerID,
		sandbox.DefaultDockerRuntimeInputResourceCleanupLeaseTTL)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Normalize(err)
	}
	if acquired.Replayed {
		return acquired.Record, nil
	}
	return s.executeDockerRuntimeInputResourceCleanup(ctx, acquired, manifest)
}

func (s *SandboxManifestService) ResumeDockerRuntimeInputResourceCleanup(ctx context.Context,
	request ResumeDockerRuntimeInputResourceCleanupRequest,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	if err := s.validateDockerRuntimeInputResourceCleanupService(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if !request.OperatorConfirmed || !request.DaemonWriteConfirmed {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker runtime input resource cleanup resume requires operator and daemon-write confirmation")
	}
	intentID, requestedBy, ownerID := strings.TrimSpace(request.IntentID),
		strings.TrimSpace(request.RequestedBy), strings.TrimSpace(request.OwnerID)
	if intentID == "" || requestedBy == "" {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup resume identity is required")
	}
	if ownerID == "" {
		ownerID = requestedBy
	}
	manifest, manifestFingerprint, err := validateDockerRuntimeInputApplicationManifest(ctx,
		request.Manifest)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	existing, err := s.store.GetDockerRuntimeInputResourceCleanup(ctx, intentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Normalize(err)
	}
	if existing.Intent.RequestedBy != requestedBy ||
		existing.Intent.ManifestFingerprint != manifestFingerprint {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input resource cleanup resume authority changed")
	}
	if existing.Result != nil {
		existing.Replayed = true
		return existing, nil
	}
	acquired, err := s.store.AcquireDockerRuntimeInputResourceCleanup(ctx, intentID,
		requestedBy, ownerID, sandbox.DefaultDockerRuntimeInputResourceCleanupLeaseTTL)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Normalize(err)
	}
	return s.executeDockerRuntimeInputResourceCleanup(ctx, acquired, manifest)
}

func (s *SandboxManifestService) executeDockerRuntimeInputResourceCleanup(ctx context.Context,
	acquired sandbox.DockerRuntimeInputResourceCleanupAcquisition, manifest sandbox.Manifest,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	if acquired.Replayed || acquired.Record.Result != nil {
		return acquired.Record, nil
	}
	inspection, err := s.store.GetDockerRuntimeInputResourceInspection(ctx,
		acquired.Record.Intent.InspectionID)
	if err != nil {
		return s.failDockerRuntimeInputResourceCleanup(ctx, acquired.Record, err)
	}
	_, _, descriptor, err := s.rebuildDockerRuntimeInputResourceDescriptor(ctx,
		acquired.Record.Intent.ApplicationIntentID, manifest,
		acquired.Record.Intent.RequestedBy)
	if err != nil {
		return s.failDockerRuntimeInputResourceCleanup(ctx, acquired.Record, err)
	}
	if err := validateDockerRuntimeInputResourceInspectionDescriptor(inspection, descriptor); err != nil {
		return s.failDockerRuntimeInputResourceCleanup(ctx, acquired.Record, err)
	}
	intent := acquired.Record.Intent
	if intent.InspectionFingerprint != inspection.InspectionFingerprint ||
		intent.DescriptorFingerprint != descriptor.DescriptorFingerprint ||
		intent.RequestFingerprint != descriptor.RequestFingerprint ||
		intent.ApplicationResultFingerprint != descriptor.ApplicationResultFingerprint ||
		intent.EndpointFingerprint != s.runtimeResourceClean.Endpoint().Fingerprint {
		return s.failDockerRuntimeInputResourceCleanup(ctx, acquired.Record, apperror.New(
			apperror.CodeConflict, "Docker runtime input resource cleanup durable intent changed"))
	}
	result, err := s.runtimeResourceClean.Cleanup(ctx, intent, acquired.Record.Lease, descriptor)
	if err != nil {
		return s.failDockerRuntimeInputResourceCleanup(ctx, acquired.Record, err)
	}
	record, _, err := s.store.CompleteDockerRuntimeInputResourceCleanup(ctx, result,
		acquired.Record.Lease)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Normalize(err)
	}
	return record, nil
}

func (s *SandboxManifestService) failDockerRuntimeInputResourceCleanup(_ context.Context,
	record sandbox.DockerRuntimeInputResourceCleanupRecord, cause error,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	code := sandbox.DockerRuntimeInputResourceErrorCode(cause)
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failed, recordErr := s.store.RecordDockerRuntimeInputResourceCleanupFailure(recordCtx,
		record.Intent.ID, record.Lease, code, time.Now().UTC())
	if recordErr != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, errors.Join(
			apperror.Normalize(cause), apperror.Normalize(recordErr))
	}
	stableCode := apperror.CodeFailedPrecondition
	switch code {
	case sandbox.DockerRuntimeInputResourceErrorCanceled:
		stableCode = apperror.CodeCancelled
	case sandbox.DockerRuntimeInputResourceErrorDeadline:
		stableCode = apperror.CodeDeadlineExceeded
	case sandbox.DockerRuntimeInputResourceErrorConnection:
		stableCode = apperror.CodeUnavailable
	}
	return failed, apperror.Wrap(stableCode,
		"Docker runtime input resource cleanup failed with "+code, cause)
}

func (s *SandboxManifestService) rebuildDockerRuntimeInputResourceDescriptor(ctx context.Context,
	applicationID string, manifest sandbox.Manifest, requestedBy string,
) (sandbox.DockerRuntimeInputApplicationRecord, sandbox.DockerRuntimeInputProjectionPlan,
	sandbox.DockerRuntimeInputResourceDescriptor, error) {
	application, err := s.store.GetDockerRuntimeInputApplication(ctx, applicationID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{},
			sandbox.DockerRuntimeInputProjectionPlan{}, sandbox.DockerRuntimeInputResourceDescriptor{},
			apperror.Normalize(err)
	}
	if application.Result == nil || application.Replayed ||
		application.Intent.RequestedBy != requestedBy {
		return sandbox.DockerRuntimeInputApplicationRecord{},
			sandbox.DockerRuntimeInputProjectionPlan{}, sandbox.DockerRuntimeInputResourceDescriptor{},
			apperror.New(apperror.CodeFailedPrecondition,
				"Docker runtime input application is not a current completed authority")
	}
	projection, err := s.store.GetDockerRuntimeInputProjectionPlan(ctx,
		application.Intent.ProjectionID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{},
			sandbox.DockerRuntimeInputProjectionPlan{}, sandbox.DockerRuntimeInputResourceDescriptor{},
			apperror.Normalize(err)
	}
	rebuilt, err := s.rebuildDockerRuntimeInputResourceAuthority(ctx, projection, manifest,
		requestedBy)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{},
			sandbox.DockerRuntimeInputProjectionPlan{}, sandbox.DockerRuntimeInputResourceDescriptor{}, err
	}
	descriptor, err := sandbox.NewDockerRuntimeInputResourceDescriptor(application, projection,
		rebuilt.writeRequest)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{},
			sandbox.DockerRuntimeInputProjectionPlan{}, sandbox.DockerRuntimeInputResourceDescriptor{},
			apperror.Wrap(apperror.CodeConflict,
				"Docker runtime input resource descriptor changed after application", err)
	}
	return application, projection, descriptor, nil
}

func validateDockerRuntimeInputResourceInspectionDescriptor(
	inspection sandbox.DockerRuntimeInputResourceInspection,
	descriptor sandbox.DockerRuntimeInputResourceDescriptor,
) error {
	if inspection.Validate() != nil || descriptor.Validate() != nil || inspection.Replayed ||
		inspection.ApplicationIntentID != descriptor.ApplicationIntentID ||
		inspection.ApplicationResultID != descriptor.ApplicationResultID ||
		inspection.RunID != descriptor.RunID ||
		inspection.ManifestFingerprint != descriptor.ManifestFingerprint ||
		inspection.DescriptorFingerprint != descriptor.DescriptorFingerprint ||
		inspection.RequestFingerprint != descriptor.RequestFingerprint ||
		inspection.ApplicationResultFingerprint != descriptor.ApplicationResultFingerprint ||
		inspection.ProjectionCount != len(descriptor.Mounts) {
		return apperror.New(apperror.CodeConflict,
			"Docker runtime input resource inspection descriptor changed")
	}
	return nil
}

func normalizeDockerRuntimeInputResourceError(message string, err error) error {
	code := sandbox.DockerRuntimeInputResourceErrorCode(err)
	stableCode := apperror.CodeFailedPrecondition
	switch code {
	case sandbox.DockerRuntimeInputResourceErrorCanceled:
		stableCode = apperror.CodeCancelled
	case sandbox.DockerRuntimeInputResourceErrorDeadline:
		stableCode = apperror.CodeDeadlineExceeded
	case sandbox.DockerRuntimeInputResourceErrorConnection:
		stableCode = apperror.CodeUnavailable
	}
	return apperror.Wrap(stableCode, message+" with "+code, err)
}

func (s *SandboxManifestService) validateDockerRuntimeInputResourceInspectionService() error {
	if s == nil || s.store == nil || s.checker == nil || s.runtimeResourceRead == nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input resource inspection dependencies are required")
	}
	return nil
}

func (s *SandboxManifestService) validateDockerRuntimeInputResourceCleanupService() error {
	if err := s.validateDockerRuntimeInputResourceInspectionService(); err != nil {
		return err
	}
	if s.runtimeResourceClean == nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input resource cleanup transport is required")
	}
	return nil
}

func (s *SandboxManifestService) GetDockerRuntimeInputResourceInspection(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputResourceInspection, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker runtime input resource inspection store is required")
	}
	value, err := s.store.GetDockerRuntimeInputResourceInspection(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerRuntimeInputResourceInspections(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputResourceInspection, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input resource inspection store is required")
	}
	values, err := s.store.ListDockerRuntimeInputResourceInspections(ctx,
		strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func (s *SandboxManifestService) GetDockerRuntimeInputResourceCleanup(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker runtime input resource cleanup store is required")
	}
	value, err := s.store.GetDockerRuntimeInputResourceCleanup(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerRuntimeInputResourceCleanups(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker runtime input resource cleanup store is required")
	}
	values, err := s.store.ListDockerRuntimeInputResourceCleanups(ctx,
		strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}
