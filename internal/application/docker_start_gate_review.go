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

type ReviewDockerStartGateRequest struct {
	CleanupIntentID   string
	Manifest          sandbox.Manifest
	OperationKey      string
	RequestedBy       string
	OperatorConfirmed bool
}

func (s *SandboxManifestService) ReviewDockerStartGate(ctx context.Context,
	request ReviewDockerStartGateRequest,
) (sandbox.DockerStartGateReview, error) {
	if s == nil || s.store == nil || s.checker == nil {
		return sandbox.DockerStartGateReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker start-gate review store and policy checker are required")
	}
	if !request.OperatorConfirmed {
		return sandbox.DockerStartGateReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker start-gate design review requires explicit operator confirmation")
	}
	cleanupIntentID, operationKey, requestedBy, err := normalizeSandboxLifecycleOperation(
		request.CleanupIntentID, request.OperationKey, request.RequestedBy)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker start-gate review request is invalid", err)
	}
	manifest, manifestFingerprint, err := validateDockerRuntimeInputApplicationManifest(ctx,
		request.Manifest)
	if err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	keyDigest := runmutation.Fingerprint(sandbox.DockerStartGateReviewOperationVersion,
		cleanupIntentID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerStartGateReviewOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayDockerStartGateReview(ctx, cleanupIntentID, manifestFingerprint,
			requestedBy, existing)
	}

	cleanup, err := s.store.GetDockerRuntimeInputResourceCleanup(ctx, cleanupIntentID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(err)
	}
	if cleanup.Result == nil || cleanup.Replayed || cleanup.Result.Validate() != nil ||
		cleanup.Intent.RequestedBy != requestedBy ||
		cleanup.Intent.ManifestFingerprint != manifestFingerprint ||
		!cleanup.Result.TargetAbsent || !cleanup.Result.AllVolumesAbsent ||
		cleanup.Result.ForeignResourceDetected || cleanup.Result.ContainerStartAuthorized ||
		cleanup.Result.ProcessExecutionAuthorized || cleanup.Result.OutputExportAuthorized ||
		cleanup.Result.ArtifactCommitAuthorized {
		return sandbox.DockerStartGateReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker start-gate review requires a completed non-authorizing v62 cleanup")
	}
	if existing, found, lookupErr := s.store.GetDockerStartGateReviewByCleanup(ctx,
		cleanupIntentID); lookupErr != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(lookupErr)
	} else if found {
		return sandbox.DockerStartGateReview{}, apperror.New(apperror.CodeConflict,
			"Docker resource cleanup already has a start-gate review under another operation key: "+existing.ID)
	}
	application, projection, descriptor, err := s.rebuildDockerRuntimeInputResourceDescriptor(
		ctx, cleanup.Intent.ApplicationIntentID, manifest, requestedBy)
	if err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	if application.Result == nil || cleanup.Intent.ApplicationResultID != application.Result.ID ||
		cleanup.Intent.ProjectionID != projection.ID ||
		cleanup.Intent.ContainerPlanID != projection.ContainerPlanID ||
		cleanup.Intent.DescriptorFingerprint != descriptor.DescriptorFingerprint ||
		cleanup.Intent.RequestFingerprint != descriptor.RequestFingerprint ||
		cleanup.Intent.ApplicationResultFingerprint != descriptor.ApplicationResultFingerprint ||
		cleanup.Result.ApplicationIntentID != application.Intent.ID ||
		cleanup.Result.ApplicationResultID != application.Result.ID ||
		cleanup.Result.RequestFingerprint != descriptor.RequestFingerprint ||
		cleanup.Result.ApplicationResultFingerprint != descriptor.ApplicationResultFingerprint {
		return sandbox.DockerStartGateReview{}, apperror.New(apperror.CodeConflict,
			"Docker start-gate review v61-v62 authority changed")
	}
	containerPlan, err := s.store.GetDockerContainerPlan(ctx, projection.ContainerPlanID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(err)
	}
	preflight, err := s.store.GetSandboxDisabledPreflight(ctx, containerPlan.PreflightID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(err)
	}
	if preflight.Validate() != nil || preflight.RunID != application.Intent.RunID ||
		preflight.MissionID != application.Intent.MissionID ||
		preflight.WorkspaceID != application.Intent.WorkspaceID ||
		preflight.ManifestFingerprint != manifestFingerprint ||
		preflight.RequestedBy != requestedBy || preflight.BackendEnabled ||
		preflight.ExecutionAuthorized || preflight.ArtifactCommitAuthorized {
		return sandbox.DockerStartGateReview{}, apperror.New(apperror.CodeConflict,
			"Docker start-gate review v51 authority changed")
	}
	binding := sandbox.DockerStartGateReviewBinding{
		CleanupIntentID: cleanup.Intent.ID, CleanupResultID: cleanup.Result.ID,
		ApplicationIntentID: application.Intent.ID, ApplicationResultID: application.Result.ID,
		ProjectionID: projection.ID, ContainerPlanID: containerPlan.ID,
		PreflightID: preflight.ID, RunID: application.Intent.RunID,
		MissionID: application.Intent.MissionID, WorkspaceID: application.Intent.WorkspaceID,
		ManifestFingerprint:      manifestFingerprint,
		ThreatModelFingerprint:   preflight.Handshake.ThreatModelFingerprint,
		CleanupResultFingerprint: cleanup.Result.ResultFingerprint,
		MaxLogBytes:              preflight.OutputPlan.MaxOutputBytes,
	}
	review, err := sandbox.NewDockerStartGateReview(
		idgen.New("sandbox-docker-start-gate-review"), keyDigest, requestedBy, binding,
		request.OperatorConfirmed, time.Now().UTC())
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Wrap(apperror.CodeInternal,
			"Docker start-gate review assembly failed", err)
	}
	operation, err := sandbox.NewDockerStartGateReviewOperation(keyDigest, review)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Wrap(apperror.CodeInternal,
			"Docker start-gate review operation assembly failed", err)
	}
	stored, replayed, err := s.store.CreateDockerStartGateReview(ctx, review, operation)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) replayDockerStartGateReview(ctx context.Context,
	cleanupIntentID, manifestFingerprint, requestedBy string,
	operation sandbox.DockerStartGateReviewOperation,
) (sandbox.DockerStartGateReview, error) {
	if operation.CleanupIntentID != cleanupIntentID || operation.RequestedBy != requestedBy {
		return sandbox.DockerStartGateReview{}, apperror.New(apperror.CodeConflict,
			"Docker start-gate review operation key was used for a different request")
	}
	review, err := s.store.GetDockerStartGateReview(ctx, operation.ReviewID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(err)
	}
	if review.CleanupIntentID != cleanupIntentID ||
		review.ManifestFingerprint != manifestFingerprint || review.RequestedBy != requestedBy ||
		review.OperationKeyDigest != operation.KeyDigest ||
		operation.RequestFingerprint != sandbox.DockerStartGateReviewRequestFingerprint(review) ||
		!operation.CreatedAt.Equal(review.CreatedAt) {
		return sandbox.DockerStartGateReview{}, apperror.New(apperror.CodeConflict,
			"Docker start-gate review replay intent changed")
	}
	review.Replayed = true
	return review, nil
}

func (s *SandboxManifestService) GetDockerStartGateReview(ctx context.Context,
	id string,
) (sandbox.DockerStartGateReview, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerStartGateReview{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker start-gate review store is required")
	}
	value, err := s.store.GetDockerStartGateReview(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerStartGateReviews(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerStartGateReview, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker start-gate review store is required")
	}
	values, err := s.store.ListDockerStartGateReviews(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}
