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

type ReviewDockerProductionEvidenceRequest struct {
	EvidenceID        string
	OperationKey      string
	Decision          string
	ReasonCode        string
	ReviewedBy        string
	OperatorConfirmed bool
}

func (s *SandboxManifestService) ReviewDockerProductionEvidence(ctx context.Context,
	request ReviewDockerProductionEvidenceRequest,
) (sandbox.DockerProductionEvidenceReview, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence review store is required")
	}
	if !request.OperatorConfirmed {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence review requires explicit operator confirmation")
	}
	evidenceID, operationKey, reviewedBy, err := normalizeSandboxLifecycleOperation(
		request.EvidenceID, request.OperationKey, request.ReviewedBy)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Wrap(
			apperror.CodeInvalidArgument,
			"Docker production evidence review request is invalid", err)
	}
	decision := strings.ToLower(strings.TrimSpace(request.Decision))
	reasonCode := strings.ToLower(strings.TrimSpace(request.ReasonCode))
	keyDigest := runmutation.Fingerprint(
		sandbox.DockerProductionEvidenceReviewOperationVersion, evidenceID, operationKey)
	if existing, found, lookupErr := s.store.GetDockerProductionEvidenceReviewOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Normalize(lookupErr)
	} else if found {
		return s.replayDockerProductionEvidenceReview(ctx, evidenceID, decision,
			reasonCode, reviewedBy, existing)
	}
	evidence, err := s.store.GetDockerProductionEvidence(ctx, evidenceID)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Normalize(err)
	}
	attempt, found, err := s.store.GetDockerProductionEvidenceAttemptByEvidence(ctx,
		evidenceID)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Normalize(err)
	}
	if !found || attempt.Result != nil || attempt.HarnessResult == nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence review requires a completed v67 harness receipt")
	}
	if existing, exists, lookupErr := s.store.GetDockerProductionEvidenceReviewByEvidence(ctx,
		evidenceID); lookupErr != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Normalize(lookupErr)
	} else if exists {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence already has an immutable review: "+existing.ID)
	}
	review, err := sandbox.NewDockerProductionEvidenceReview(
		idgen.New("sandbox-docker-production-evidence-review"), keyDigest, reviewedBy,
		decision, reasonCode, evidence, attempt, true, time.Now().UTC())
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Wrap(
			apperror.CodeInvalidArgument,
			"Docker production evidence review decision is invalid", err)
	}
	operation, err := sandbox.NewDockerProductionEvidenceReviewOperation(keyDigest, review)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Wrap(
			apperror.CodeInternal,
			"Docker production evidence review operation assembly failed", err)
	}
	stored, replayed, err := s.store.CreateDockerProductionEvidenceReview(ctx,
		review, operation)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) replayDockerProductionEvidenceReview(ctx context.Context,
	evidenceID, decision, reasonCode, reviewedBy string,
	operation sandbox.DockerProductionEvidenceReviewOperation,
) (sandbox.DockerProductionEvidenceReview, error) {
	if operation.EvidenceID != evidenceID || operation.ReviewedBy != reviewedBy {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence review operation key was used for another receipt")
	}
	review, err := s.store.GetDockerProductionEvidenceReview(ctx, operation.ReviewID)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.Normalize(err)
	}
	if review.EvidenceID != evidenceID || review.Decision != decision ||
		review.ReasonCode != reasonCode || review.ReviewedBy != reviewedBy ||
		review.OperationKeyDigest != operation.KeyDigest ||
		operation.RequestFingerprint !=
			sandbox.DockerProductionEvidenceReviewRequestFingerprint(review) ||
		!operation.CreatedAt.Equal(review.CreatedAt) {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence review replay decision changed")
	}
	review.Replayed = true
	return review, nil
}

func (s *SandboxManifestService) GetDockerProductionEvidenceReview(ctx context.Context,
	id string,
) (sandbox.DockerProductionEvidenceReview, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence review store is required")
	}
	value, err := s.store.GetDockerProductionEvidenceReview(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerProductionEvidenceReviews(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerProductionEvidenceReview, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker production evidence review store is required")
	}
	values, err := s.store.ListDockerProductionEvidenceReviews(ctx,
		strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}
