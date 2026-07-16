package application

import (
	"context"
	"errors"
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
	OwnerID           string
	OperatorConfirmed bool
}

type ResumeDockerProductionEvidenceRequest struct {
	AttemptID         string
	RequestedBy       string
	OwnerID           string
	OperatorConfirmed bool
}

type DockerProductionEvidenceCaptureResult struct {
	sandbox.DockerProductionEvidence
	Attempt sandbox.DockerProductionEvidenceAttemptRecord
}

func (s *SandboxManifestService) CaptureDockerProductionEvidence(ctx context.Context,
	request CaptureDockerProductionEvidenceRequest,
) (DockerProductionEvidenceCaptureResult, error) {
	if err := s.validateDockerProductionEvidenceAttemptService(); err != nil {
		return DockerProductionEvidenceCaptureResult{}, err
	}
	request.ReviewID = strings.TrimSpace(request.ReviewID)
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	request.RequestedBy = strings.TrimSpace(request.RequestedBy)
	request.OwnerID = strings.TrimSpace(request.OwnerID)
	if request.OwnerID == "" {
		request.OwnerID = request.RequestedBy
	}
	if !domain.ValidAgentID(request.ReviewID) || strings.ContainsRune(request.ReviewID, 0) ||
		request.OperationKey == "" || len(request.OperationKey) > 4096 ||
		strings.ContainsRune(request.OperationKey, 0) ||
		!domain.ValidAgentID(request.RequestedBy) || strings.ContainsRune(request.RequestedBy, 0) ||
		!domain.ValidAgentID(request.OwnerID) || strings.ContainsRune(request.OwnerID, 0) {
		return DockerProductionEvidenceCaptureResult{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence request is invalid")
	}
	if !request.OperatorConfirmed {
		return DockerProductionEvidenceCaptureResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence capture requires explicit operator confirmation")
	}
	review, err := s.loadDockerProductionEvidenceReview(ctx, request.ReviewID,
		request.RequestedBy)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, err
	}
	keyDigest := runmutation.Fingerprint(sandbox.DockerProductionEvidenceOperationVersion,
		review.ID, request.OperationKey)
	requestFingerprint := sandbox.DockerProductionEvidenceCaptureRequestFingerprint(
		review.ID, review.RunID, review.AuthorityFingerprint,
		sandbox.DockerProductionEvidenceSuiteFingerprint(), request.RequestedBy)
	if existing, found, lookupErr := s.store.GetDockerProductionEvidenceAttemptByOperation(ctx,
		keyDigest); lookupErr != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(lookupErr)
	} else if found {
		if existing.Attempt.RequestFingerprint != requestFingerprint ||
			existing.Attempt.ReviewID != review.ID || existing.Attempt.RunID != review.RunID ||
			existing.Attempt.RequestedBy != request.RequestedBy {
			return DockerProductionEvidenceCaptureResult{}, apperror.New(
				apperror.CodeConflict,
				"Docker production evidence attempt operation key changed request")
		}
		if existing.Result != nil {
			return s.replayDockerProductionEvidenceAttempt(ctx, existing)
		}
		acquired, acquireErr := s.store.AcquireDockerProductionEvidenceAttempt(ctx,
			existing.Attempt.ID, request.RequestedBy, request.OwnerID,
			sandbox.DefaultDockerProductionEvidenceAttemptLeaseTTL)
		if acquireErr != nil {
			return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(acquireErr)
		}
		return s.executeDockerProductionEvidenceAttempt(ctx, acquired)
	}
	if operation, found, lookupErr := s.store.GetDockerProductionEvidenceOperation(ctx,
		keyDigest); lookupErr != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(lookupErr)
	} else if found {
		if operation.RequestFingerprint != requestFingerprint ||
			operation.ReviewID != review.ID || operation.RunID != review.RunID ||
			operation.RequestedBy != request.RequestedBy {
			return DockerProductionEvidenceCaptureResult{}, apperror.New(
				apperror.CodeConflict,
				"Docker production evidence operation key changed request")
		}
		value, loadErr := s.store.GetDockerProductionEvidence(ctx, operation.EvidenceID)
		if loadErr != nil {
			return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(loadErr)
		}
		value.Replayed = true
		return DockerProductionEvidenceCaptureResult{DockerProductionEvidence: value}, nil
	}
	endpoint, err := sandbox.NewDockerObservationEndpoint(
		sandbox.DockerObservationEndpointLocalUnix)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Wrap(
			apperror.CodeInternal, "build Docker production evidence endpoint", err)
	}
	attempt, err := sandbox.NewDockerProductionEvidenceAttempt(
		idgen.New("sandbox-docker-production-evidence-attempt"), keyDigest,
		request.RequestedBy, review, endpoint, true,
		sandbox.DefaultDockerProductionEvidenceCaptureTimeout, time.Now().UTC())
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Wrap(
			apperror.CodeInternal, "build Docker production evidence attempt", err)
	}
	acquired, err := s.store.BeginDockerProductionEvidenceAttempt(ctx, attempt,
		request.OwnerID, sandbox.DefaultDockerProductionEvidenceAttemptLeaseTTL)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(err)
	}
	if acquired.Replayed && acquired.Record.Result != nil {
		return s.replayDockerProductionEvidenceAttempt(ctx, acquired.Record)
	}
	return s.executeDockerProductionEvidenceAttempt(ctx, acquired)
}

func (s *SandboxManifestService) ResumeDockerProductionEvidence(ctx context.Context,
	request ResumeDockerProductionEvidenceRequest,
) (DockerProductionEvidenceCaptureResult, error) {
	if err := s.validateDockerProductionEvidenceAttemptService(); err != nil {
		return DockerProductionEvidenceCaptureResult{}, err
	}
	request.AttemptID, request.RequestedBy, request.OwnerID = strings.TrimSpace(request.AttemptID),
		strings.TrimSpace(request.RequestedBy), strings.TrimSpace(request.OwnerID)
	if request.OwnerID == "" {
		request.OwnerID = request.RequestedBy
	}
	if !request.OperatorConfirmed {
		return DockerProductionEvidenceCaptureResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence resume requires explicit operator confirmation")
	}
	if !domain.ValidAgentID(request.AttemptID) || !domain.ValidAgentID(request.RequestedBy) ||
		!domain.ValidAgentID(request.OwnerID) || strings.ContainsRune(request.AttemptID, 0) ||
		strings.ContainsRune(request.RequestedBy, 0) || strings.ContainsRune(request.OwnerID, 0) {
		return DockerProductionEvidenceCaptureResult{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence resume identity is invalid")
	}
	existing, err := s.store.GetDockerProductionEvidenceAttempt(ctx, request.AttemptID)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(err)
	}
	if existing.Attempt.RequestedBy != request.RequestedBy {
		return DockerProductionEvidenceCaptureResult{}, apperror.New(
			apperror.CodeConflict, "Docker production evidence resume authority changed")
	}
	if existing.Result != nil {
		return s.replayDockerProductionEvidenceAttempt(ctx, existing)
	}
	acquired, err := s.store.AcquireDockerProductionEvidenceAttempt(ctx,
		existing.Attempt.ID, request.RequestedBy, request.OwnerID,
		sandbox.DefaultDockerProductionEvidenceAttemptLeaseTTL)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(err)
	}
	return s.executeDockerProductionEvidenceAttempt(ctx, acquired)
}

func (s *SandboxManifestService) executeDockerProductionEvidenceAttempt(ctx context.Context,
	acquired sandbox.DockerProductionEvidenceAttemptAcquisition,
) (DockerProductionEvidenceCaptureResult, error) {
	if acquired.Replayed || acquired.Record.Result != nil {
		return s.replayDockerProductionEvidenceAttempt(ctx, acquired.Record)
	}
	now := time.Now().UTC()
	reconciliation, err := sandbox.NewDockerProductionEvidenceReconciliation(
		acquired.Record.Attempt, acquired.Record.Lease, now)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"build Docker production evidence reconciliation checkpoint", err)
	}
	record, _, err := s.store.RecordDockerProductionEvidenceReconciliation(ctx,
		reconciliation, acquired.Record.Lease)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(err)
	}
	current, found := record.CurrentReconciliation()
	if !found || current.Generation != record.Lease.Generation {
		return DockerProductionEvidenceCaptureResult{}, apperror.New(
			apperror.CodeInternal,
			"Docker production evidence reconciliation checkpoint was not durable")
	}
	deadline := now.Add(time.Duration(record.Attempt.CaptureTimeoutMillis) * time.Millisecond)
	leaseDeadline := record.Lease.ExpiresAt.Add(-sandbox.DockerProductionEvidenceLeaseSafetyMargin)
	if leaseDeadline.Before(deadline) {
		deadline = leaseDeadline
	}
	if !deadline.After(time.Now().UTC()) {
		return s.failDockerProductionEvidenceAttempt(record,
			sandbox.DockerProductionEvidenceAttemptErrorDeadline,
			context.DeadlineExceeded)
	}
	captureRequest := sandbox.DockerProductionEvidenceCaptureRequest{
		ReviewID: record.Attempt.ReviewID, RunID: record.Attempt.RunID,
		AuthorityFingerprint: record.Attempt.AuthorityFingerprint,
		AttemptID:            record.Attempt.ID, LeaseGeneration: record.Lease.Generation,
		EndpointClass:       record.Attempt.EndpointClass,
		EndpointFingerprint: record.Attempt.EndpointFingerprint, DeadlineAt: deadline,
	}
	captureCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	observation, err := s.productionEvidence.Capture(captureCtx, captureRequest)
	if err != nil {
		return s.failDockerProductionEvidenceAttempt(record,
			dockerProductionEvidenceAttemptFailureCode(err), err)
	}
	if observation.RealDaemonContacted ||
		observation.Status == sandbox.DockerProductionEvidenceStatusComplete {
		return s.failDockerProductionEvidenceAttempt(record,
			sandbox.DockerProductionEvidenceAttemptErrorUnsafeContact,
			apperror.New(apperror.CodeFailedPrecondition,
				"real-daemon evidence capture remains disabled after the write-ahead checkpoint"))
	}
	if err := observation.Validate(record.Attempt.AuthorityFingerprint); err != nil {
		return s.failDockerProductionEvidenceAttempt(record,
			sandbox.DockerProductionEvidenceAttemptErrorInvalidResponse,
			apperror.Wrap(apperror.CodeInternal,
				"validate Docker production evidence observation", err))
	}
	review, err := s.loadDockerProductionEvidenceReview(ctx, record.Attempt.ReviewID,
		record.Attempt.RequestedBy)
	if err != nil {
		return s.failDockerProductionEvidenceAttempt(record,
			sandbox.DockerProductionEvidenceAttemptErrorPersistence, err)
	}
	value, err := sandbox.NewDockerProductionEvidence(
		idgen.New("sandbox-docker-production-evidence"), record.Attempt.OperationKeyDigest,
		record.Attempt.RequestedBy, review, observation, true, time.Now().UTC())
	if err != nil {
		return s.failDockerProductionEvidenceAttempt(record,
			sandbox.DockerProductionEvidenceAttemptErrorInvalidResponse,
			apperror.Wrap(apperror.CodeInternal, "build Docker production evidence", err))
	}
	operation, err := sandbox.NewDockerProductionEvidenceOperation(
		record.Attempt.OperationKeyDigest, value)
	if err != nil {
		return s.failDockerProductionEvidenceAttempt(record,
			sandbox.DockerProductionEvidenceAttemptErrorInvalidResponse,
			apperror.Wrap(apperror.CodeInternal,
				"build Docker production evidence operation", err))
	}
	result, err := sandbox.NewDockerProductionEvidenceAttemptResult(record.Attempt,
		record.Lease, current, value)
	if err != nil {
		return s.failDockerProductionEvidenceAttempt(record,
			sandbox.DockerProductionEvidenceAttemptErrorInvalidResponse,
			apperror.Wrap(apperror.CodeInternal,
				"build Docker production evidence attempt result", err))
	}
	completed, stored, replayed, err := s.store.CompleteDockerProductionEvidenceAttempt(ctx,
		value, operation, result, record.Lease)
	if err != nil {
		recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, recordErr := s.store.RecordDockerProductionEvidenceAttemptFailure(recordCtx,
			record.Attempt.ID, record.Lease,
			sandbox.DockerProductionEvidenceAttemptErrorPersistence, time.Now().UTC())
		cancel()
		if recordErr != nil {
			return DockerProductionEvidenceCaptureResult{}, errors.Join(
				apperror.Normalize(err), apperror.Normalize(recordErr))
		}
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(err)
	}
	stored.Replayed = replayed
	completed.Replayed = replayed
	return DockerProductionEvidenceCaptureResult{
		DockerProductionEvidence: stored, Attempt: completed,
	}, nil
}

func (s *SandboxManifestService) failDockerProductionEvidenceAttempt(
	record sandbox.DockerProductionEvidenceAttemptRecord, code string, cause error,
) (DockerProductionEvidenceCaptureResult, error) {
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failed, recordErr := s.store.RecordDockerProductionEvidenceAttemptFailure(recordCtx,
		record.Attempt.ID, record.Lease, code, time.Now().UTC())
	if recordErr != nil {
		return DockerProductionEvidenceCaptureResult{}, errors.Join(
			apperror.Normalize(cause), apperror.Normalize(recordErr))
	}
	stableCode := apperror.CodeFailedPrecondition
	switch code {
	case sandbox.DockerProductionEvidenceAttemptErrorCanceled:
		stableCode = apperror.CodeCancelled
	case sandbox.DockerProductionEvidenceAttemptErrorDeadline:
		stableCode = apperror.CodeDeadlineExceeded
	case sandbox.DockerProductionEvidenceAttemptErrorCollector:
		stableCode = apperror.CodeUnavailable
	case sandbox.DockerProductionEvidenceAttemptErrorInvalidResponse,
		sandbox.DockerProductionEvidenceAttemptErrorPersistence:
		stableCode = apperror.CodeInternal
	}
	return DockerProductionEvidenceCaptureResult{Attempt: failed}, apperror.Wrap(stableCode,
		"Docker production evidence attempt failed with "+code, cause)
}

func dockerProductionEvidenceAttemptFailureCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return sandbox.DockerProductionEvidenceAttemptErrorCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return sandbox.DockerProductionEvidenceAttemptErrorDeadline
	}
	return sandbox.DockerProductionEvidenceAttemptErrorCollector
}

func (s *SandboxManifestService) replayDockerProductionEvidenceAttempt(ctx context.Context,
	record sandbox.DockerProductionEvidenceAttemptRecord,
) (DockerProductionEvidenceCaptureResult, error) {
	if record.Result == nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence attempt has no committed evidence")
	}
	value, err := s.store.GetDockerProductionEvidence(ctx, record.Result.EvidenceID)
	if err != nil {
		return DockerProductionEvidenceCaptureResult{}, apperror.Normalize(err)
	}
	value.Replayed, record.Replayed = true, true
	return DockerProductionEvidenceCaptureResult{
		DockerProductionEvidence: value, Attempt: record,
	}, nil
}

func (s *SandboxManifestService) loadDockerProductionEvidenceReview(ctx context.Context,
	reviewID, requestedBy string,
) (sandbox.DockerStartGateReview, error) {
	review, err := s.store.GetDockerStartGateReview(ctx, reviewID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, apperror.Normalize(err)
	}
	if review.Validate() != nil || review.RequestedBy != requestedBy ||
		review.StartGatePassed || review.StartImplementationPresent ||
		review.ContainerStartAuthorized || review.ProcessExecutionAuthorized ||
		review.OutputExportAuthorized || review.ArtifactCommitAuthorized {
		return sandbox.DockerStartGateReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence requires the same operator and a blocked v63 review")
	}
	return review, nil
}

func (s *SandboxManifestService) validateDockerProductionEvidenceAttemptService() error {
	if s == nil || s.store == nil || s.productionEvidence == nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Docker production evidence store and collector are required")
	}
	return nil
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

func (s *SandboxManifestService) GetDockerProductionEvidenceAttempt(ctx context.Context,
	id string,
) (sandbox.DockerProductionEvidenceAttemptRecord, error) {
	if s == nil || s.store == nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence attempt store is required")
	}
	value, err := s.store.GetDockerProductionEvidenceAttempt(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) ListDockerProductionEvidenceAttempts(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerProductionEvidenceAttemptRecord, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Docker production evidence attempt store is required")
	}
	values, err := s.store.ListDockerProductionEvidenceAttempts(ctx,
		strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}
