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

type CaptureDockerProductionEvidenceRequest struct {
	ReviewID          string
	OperationKey      string
	RequestedBy       string
	OperatorConfirmed bool
}

func (s *SandboxManifestService) CaptureDockerProductionEvidence(ctx context.Context,
	request CaptureDockerProductionEvidenceRequest,
) (sandbox.DockerProductionEvidence, error) {
	if s == nil || s.store == nil || s.productionEvidence == nil {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence store and collector are required")
	}
	request.ReviewID = strings.TrimSpace(request.ReviewID)
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	request.RequestedBy = strings.TrimSpace(request.RequestedBy)
	if !domain.ValidAgentID(request.ReviewID) || strings.ContainsRune(request.ReviewID, 0) ||
		request.OperationKey == "" || len(request.OperationKey) > 4096 ||
		strings.ContainsRune(request.OperationKey, 0) ||
		!domain.ValidAgentID(request.RequestedBy) || strings.ContainsRune(request.RequestedBy, 0) {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence request is invalid")
	}
	if !request.OperatorConfirmed {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence capture requires explicit operator confirmation")
	}
	review, err := s.store.GetDockerStartGateReview(ctx, request.ReviewID)
	if err != nil {
		return sandbox.DockerProductionEvidence{}, apperror.Normalize(err)
	}
	if review.Validate() != nil || review.RequestedBy != request.RequestedBy ||
		review.StartGatePassed || review.ContainerStartAuthorized ||
		review.ProcessExecutionAuthorized || review.ArtifactCommitAuthorized {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence requires the same operator and a blocked v63 review")
	}
	keyDigest := runmutation.Fingerprint(sandbox.DockerProductionEvidenceOperationVersion,
		review.ID, request.OperationKey)
	requestFingerprint := sandbox.DockerProductionEvidenceCaptureRequestFingerprint(
		review.ID, review.RunID, review.AuthorityFingerprint,
		sandbox.DockerProductionEvidenceSuiteFingerprint(), request.RequestedBy)
	if operation, found, lookupErr := s.store.GetDockerProductionEvidenceOperation(ctx,
		keyDigest); lookupErr != nil {
		return sandbox.DockerProductionEvidence{}, apperror.Normalize(lookupErr)
	} else if found {
		if operation.RequestFingerprint != requestFingerprint ||
			operation.ReviewID != review.ID || operation.RunID != review.RunID ||
			operation.RequestedBy != request.RequestedBy {
			return sandbox.DockerProductionEvidence{}, apperror.New(
				apperror.CodeConflict,
				"Docker production evidence operation key changed request")
		}
		value, loadErr := s.store.GetDockerProductionEvidence(ctx, operation.EvidenceID)
		if loadErr != nil {
			return sandbox.DockerProductionEvidence{}, apperror.Normalize(loadErr)
		}
		value.Replayed = true
		return value, nil
	}
	observation, err := s.productionEvidence.Capture(ctx,
		sandbox.DockerProductionEvidenceCaptureRequest{
			ReviewID: review.ID, RunID: review.RunID,
			AuthorityFingerprint: review.AuthorityFingerprint,
		})
	if err != nil {
		return sandbox.DockerProductionEvidence{}, apperror.Normalize(err)
	}
	if err := observation.Validate(review.AuthorityFingerprint); err != nil {
		return sandbox.DockerProductionEvidence{}, apperror.Wrap(
			apperror.CodeInternal, "validate Docker production evidence observation", err)
	}
	if observation.RealDaemonContacted ||
		observation.Status == sandbox.DockerProductionEvidenceStatusComplete {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"real-daemon evidence capture requires a future write-ahead harness gate")
	}
	value, err := sandbox.NewDockerProductionEvidence(
		idgen.New("sandbox-docker-production-evidence"), keyDigest, request.RequestedBy,
		review, observation, true, time.Now().UTC())
	if err != nil {
		return sandbox.DockerProductionEvidence{}, apperror.Wrap(
			apperror.CodeInternal, "build Docker production evidence", err)
	}
	operation, err := sandbox.NewDockerProductionEvidenceOperation(keyDigest, value)
	if err != nil {
		return sandbox.DockerProductionEvidence{}, apperror.Wrap(
			apperror.CodeInternal, "build Docker production evidence operation", err)
	}
	stored, replayed, err := s.store.CreateDockerProductionEvidence(ctx, value, operation)
	if err != nil {
		return sandbox.DockerProductionEvidence{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	return stored, nil
}

func (s *SandboxManifestService) GetDockerProductionEvidence(ctx context.Context,
	id string,
) (sandbox.DockerProductionEvidence, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker production evidence store is required")
	}
	value, err := s.store.GetDockerProductionEvidence(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerProductionEvidence(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerProductionEvidence, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker production evidence store is required")
	}
	values, err := s.store.ListDockerProductionEvidence(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}
