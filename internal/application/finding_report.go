package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
)

type FindingReportStore interface {
	EnsureReadOnlyFanoutFindingReport(ctx context.Context,
		executionID string) (domain.FindingReport, bool, error)
	GetFindingReport(ctx context.Context, id string) (domain.FindingReport, error)
	GetFinding(ctx context.Context, id string) (domain.Finding, error)
	GetRunArtifact(ctx context.Context, id string) (artifact.Blob, error)
	AttachFindingArtifactEvidence(ctx context.Context,
		operation domain.FindingArtifactEvidenceOperation,
		evidence domain.FindingArtifactEvidence) (domain.FindingArtifactEvidence, bool, error)
	CreateFindingValidation(ctx context.Context,
		operation domain.FindingValidationOperation,
		validation domain.FindingValidation) (domain.FindingValidation, bool, error)
}

type FindingReportService struct {
	store FindingReportStore
}

func NewFindingReportService(store FindingReportStore) *FindingReportService {
	return &FindingReportService{store: store}
}

func (s *FindingReportService) GenerateReadOnlyFanout(ctx context.Context,
	executionID string,
) (domain.FindingReport, bool, error) {
	if s == nil || s.store == nil {
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	executionID = strings.TrimSpace(executionID)
	if !domain.ValidAgentID(executionID) {
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeInvalidArgument, "read-only fan-out execution id is invalid")
	}
	report, replayed, err := s.store.EnsureReadOnlyFanoutFindingReport(ctx, executionID)
	return report, replayed, apperror.Normalize(err)
}

func (s *FindingReportService) Get(ctx context.Context,
	id string,
) (domain.FindingReport, error) {
	if s == nil || s.store == nil {
		return domain.FindingReport{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) {
		return domain.FindingReport{}, apperror.New(
			apperror.CodeInvalidArgument, "finding report id is invalid")
	}
	value, err := s.store.GetFindingReport(ctx, id)
	return value, apperror.Normalize(err)
}

type AttachFindingArtifactEvidenceRequest struct {
	FindingID    string
	ArtifactID   string
	OperationKey string
	AttachedBy   string
	Note         string
}

type AttachFindingArtifactEvidenceResult struct {
	Evidence domain.FindingArtifactEvidence
	Replayed bool
}

func (s *FindingReportService) AttachArtifactEvidence(ctx context.Context,
	request AttachFindingArtifactEvidenceRequest,
) (AttachFindingArtifactEvidenceResult, error) {
	if s == nil || s.store == nil {
		return AttachFindingArtifactEvidenceResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeAttachFindingArtifactEvidenceRequest(request)
	if err != nil {
		return AttachFindingArtifactEvidenceResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding Artifact Evidence request is invalid", err)
	}
	finding, err := s.store.GetFinding(ctx, normalized.FindingID)
	if err != nil {
		return AttachFindingArtifactEvidenceResult{}, apperror.Normalize(err)
	}
	blob, err := s.store.GetRunArtifact(ctx, normalized.ArtifactID)
	if err != nil {
		return AttachFindingArtifactEvidenceResult{}, apperror.Normalize(err)
	}
	now := time.Now().UTC()
	if now.Before(finding.CreatedAt) {
		now = finding.CreatedAt
	}
	if now.Before(blob.CreatedAt) {
		now = blob.CreatedAt
	}
	evidence := domain.FindingArtifactEvidence{
		ID: idgen.New("finding-artifact-evidence"), ReportID: finding.ReportID,
		FindingID: finding.ID, RunID: finding.RunID,
		Ordinal: len(finding.ArtifactEvidence) + 1, ArtifactID: blob.ID,
		ArtifactSHA256: blob.SHA256, ArtifactSize: blob.SizeBytes,
		ArtifactMIME: blob.MIME, ArtifactStream: string(blob.Stream),
		ArtifactTool: blob.ToolName, ArtifactSource: blob.SourceID,
		ArtifactRedacted: blob.Redacted,
		AttachedBy:       normalized.AttachedBy, Note: normalized.Note, CreatedAt: now,
	}
	operation := domain.FindingArtifactEvidenceOperation{
		KeyDigest: runmutation.OperationKeyDigest("finding_artifact_evidence",
			finding.RunID, normalized.OperationKey),
		RequestFingerprint: runmutation.Fingerprint("finding_artifact_evidence_request.v1",
			finding.ID, blob.ID, normalized.AttachedBy, normalized.Note),
		EvidenceID: evidence.ID, FindingID: finding.ID, ArtifactID: blob.ID,
		RunID: finding.RunID, AttachedBy: normalized.AttachedBy, CreatedAt: now,
	}
	stored, replayed, err := s.store.AttachFindingArtifactEvidence(ctx, operation, evidence)
	if err != nil {
		return AttachFindingArtifactEvidenceResult{}, apperror.Normalize(err)
	}
	return AttachFindingArtifactEvidenceResult{Evidence: stored, Replayed: replayed}, nil
}

type DecideFindingValidationRequest struct {
	FindingID    string
	OperationKey string
	Status       domain.FindingStatus
	Reason       string
	DecidedBy    string
}

type DecideFindingValidationResult struct {
	Validation domain.FindingValidation
	Replayed   bool
}

func (s *FindingReportService) DecideValidation(ctx context.Context,
	request DecideFindingValidationRequest,
) (DecideFindingValidationResult, error) {
	if s == nil || s.store == nil {
		return DecideFindingValidationResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeDecideFindingValidationRequest(request)
	if err != nil {
		return DecideFindingValidationResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding validation request is invalid", err)
	}
	finding, err := s.store.GetFinding(ctx, normalized.FindingID)
	if err != nil {
		return DecideFindingValidationResult{}, apperror.Normalize(err)
	}
	if normalized.Status == domain.FindingStatusValidated &&
		len(finding.ArtifactEvidence) == 0 {
		return DecideFindingValidationResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"validated finding requires at least one frozen Artifact Evidence record")
	}
	digest, err := domain.FindingArtifactEvidenceDigest(finding.ArtifactEvidence)
	if err != nil {
		return DecideFindingValidationResult{}, err
	}
	now := time.Now().UTC()
	if now.Before(finding.CreatedAt) {
		now = finding.CreatedAt
	}
	if len(finding.ArtifactEvidence) > 0 {
		lastCreatedAt := finding.ArtifactEvidence[len(finding.ArtifactEvidence)-1].CreatedAt
		if now.Before(lastCreatedAt) {
			now = lastCreatedAt
		}
	}
	validation := domain.FindingValidation{
		ID: idgen.New("finding-validation"), ReportID: finding.ReportID,
		FindingID: finding.ID, RunID: finding.RunID, FromStatus: finding.Status,
		Status: normalized.Status, DecidedBy: normalized.DecidedBy,
		Reason: normalized.Reason, ArtifactEvidenceCount: len(finding.ArtifactEvidence),
		ArtifactEvidenceDigest: digest, Version: 1, CreatedAt: now,
	}
	operation := domain.FindingValidationOperation{
		KeyDigest: runmutation.OperationKeyDigest("finding_validation", finding.RunID,
			normalized.OperationKey),
		RequestFingerprint: runmutation.Fingerprint("finding_validation_request.v1",
			finding.ID, string(normalized.Status), normalized.DecidedBy, normalized.Reason),
		ValidationID: validation.ID, FindingID: finding.ID, RunID: finding.RunID,
		Status: normalized.Status, DecidedBy: normalized.DecidedBy, CreatedAt: now,
	}
	stored, replayed, err := s.store.CreateFindingValidation(ctx, operation, validation)
	if err != nil {
		return DecideFindingValidationResult{}, apperror.Normalize(err)
	}
	return DecideFindingValidationResult{Validation: stored, Replayed: replayed}, nil
}

type FindingArtifactEvidenceVerification struct {
	FindingID              string               `json:"finding_id"`
	RunID                  string               `json:"run_id"`
	Status                 domain.FindingStatus `json:"status"`
	ArtifactEvidenceCount  int                  `json:"artifact_evidence_count"`
	ArtifactEvidenceDigest string               `json:"artifact_evidence_digest"`
}

func (s *FindingReportService) VerifyArtifactEvidence(ctx context.Context,
	findingID string,
) (FindingArtifactEvidenceVerification, error) {
	if s == nil || s.store == nil {
		return FindingArtifactEvidenceVerification{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	findingID = strings.TrimSpace(findingID)
	if !domain.ValidAgentID(findingID) || strings.ContainsRune(findingID, 0) {
		return FindingArtifactEvidenceVerification{}, apperror.New(
			apperror.CodeInvalidArgument, "finding id is invalid")
	}
	finding, err := s.store.GetFinding(ctx, findingID)
	if err != nil {
		return FindingArtifactEvidenceVerification{}, apperror.Normalize(err)
	}
	for _, evidence := range finding.ArtifactEvidence {
		blob, err := s.store.GetRunArtifact(ctx, evidence.ArtifactID)
		if err != nil {
			return FindingArtifactEvidenceVerification{}, apperror.Normalize(err)
		}
		if blob.RunID != finding.RunID || blob.SHA256 != evidence.ArtifactSHA256 ||
			blob.SizeBytes != evidence.ArtifactSize || blob.MIME != evidence.ArtifactMIME ||
			string(blob.Stream) != evidence.ArtifactStream ||
			blob.ToolName != evidence.ArtifactTool || blob.SourceID != evidence.ArtifactSource ||
			blob.Redacted != evidence.ArtifactRedacted {
			return FindingArtifactEvidenceVerification{}, apperror.New(
				apperror.CodeConflict,
				"finding Artifact Evidence no longer matches its frozen Artifact")
		}
	}
	digest, err := domain.FindingArtifactEvidenceDigest(finding.ArtifactEvidence)
	if err != nil {
		return FindingArtifactEvidenceVerification{}, err
	}
	status := finding.Status
	if finding.Validation != nil {
		status = finding.Validation.Status
		if finding.Validation.ArtifactEvidenceCount != len(finding.ArtifactEvidence) ||
			finding.Validation.ArtifactEvidenceDigest != digest {
			return FindingArtifactEvidenceVerification{}, apperror.New(
				apperror.CodeConflict, "finding validation Evidence snapshot drifted")
		}
	}
	return FindingArtifactEvidenceVerification{
		FindingID: finding.ID, RunID: finding.RunID, Status: status,
		ArtifactEvidenceCount:  len(finding.ArtifactEvidence),
		ArtifactEvidenceDigest: digest,
	}, nil
}

func normalizeAttachFindingArtifactEvidenceRequest(
	request AttachFindingArtifactEvidenceRequest,
) (AttachFindingArtifactEvidenceRequest, error) {
	request.FindingID = strings.TrimSpace(request.FindingID)
	request.ArtifactID = strings.TrimSpace(request.ArtifactID)
	request.AttachedBy = strings.TrimSpace(redact.String(request.AttachedBy))
	request.Note = strings.TrimSpace(redact.String(request.Note))
	if !domain.ValidAgentID(request.FindingID) || !domain.ValidAgentID(request.ArtifactID) ||
		!domain.ValidAgentID(request.AttachedBy) || strings.ContainsRune(request.FindingID, 0) ||
		strings.ContainsRune(request.ArtifactID, 0) || strings.ContainsRune(request.AttachedBy, 0) {
		return AttachFindingArtifactEvidenceRequest{}, errors.New(
			"finding, Artifact, and operator identities are required")
	}
	if err := validateFindingOperationKey(request.OperationKey); err != nil {
		return AttachFindingArtifactEvidenceRequest{}, err
	}
	if err := validateFindingValidationText(request.Note, "evidence note"); err != nil {
		return AttachFindingArtifactEvidenceRequest{}, err
	}
	return request, nil
}

func normalizeDecideFindingValidationRequest(
	request DecideFindingValidationRequest,
) (DecideFindingValidationRequest, error) {
	request.FindingID = strings.TrimSpace(request.FindingID)
	request.Status = domain.FindingStatus(strings.TrimSpace(string(request.Status)))
	request.Reason = strings.TrimSpace(redact.String(request.Reason))
	request.DecidedBy = strings.TrimSpace(redact.String(request.DecidedBy))
	if !domain.ValidAgentID(request.FindingID) || !domain.ValidAgentID(request.DecidedBy) ||
		strings.ContainsRune(request.FindingID, 0) || strings.ContainsRune(request.DecidedBy, 0) {
		return DecideFindingValidationRequest{}, errors.New(
			"finding and operator identities are required")
	}
	if request.Status != domain.FindingStatusValidated &&
		request.Status != domain.FindingStatusRejected {
		return DecideFindingValidationRequest{}, fmt.Errorf(
			"unsupported finding validation status %q", request.Status)
	}
	if err := validateFindingOperationKey(request.OperationKey); err != nil {
		return DecideFindingValidationRequest{}, err
	}
	if err := validateFindingValidationText(request.Reason, "validation reason"); err != nil {
		return DecideFindingValidationRequest{}, err
	}
	return request, nil
}

func validateFindingOperationKey(value string) error {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return errors.New("finding operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(value); err != nil {
		return err
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return errors.New(
				"finding operation key cannot contain whitespace or control characters")
		}
	}
	return nil
}

func validateFindingValidationText(value string, label string) error {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) ||
		utf8.RuneCountInString(value) > domain.MaxFindingValidationTextRunes ||
		len([]byte(value)) > domain.MaxFindingValidationTextRunes*4 {
		return fmt.Errorf("%s is required and must be at most %d characters",
			label, domain.MaxFindingValidationTextRunes)
	}
	return nil
}
