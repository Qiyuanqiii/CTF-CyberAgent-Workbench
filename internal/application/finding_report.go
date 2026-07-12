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
	CreateFindingAcceptance(ctx context.Context,
		operation domain.FindingAcceptanceOperation,
		acceptance domain.FindingAcceptance) (domain.FindingAcceptance, bool, error)
	AttachFindingRemediationEvidence(ctx context.Context,
		operation domain.FindingRemediationEvidenceOperation,
		evidence domain.FindingArtifactEvidence) (domain.FindingArtifactEvidence, bool, error)
	CreateFindingFix(ctx context.Context, operation domain.FindingFixOperation,
		fix domain.FindingFix) (domain.FindingFix, bool, error)
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

type DecideFindingAcceptanceRequest struct {
	FindingID    string
	OperationKey string
	Reason       string
	DecidedBy    string
}

type DecideFindingAcceptanceResult struct {
	Acceptance domain.FindingAcceptance
	Replayed   bool
}

func (s *FindingReportService) Accept(ctx context.Context,
	request DecideFindingAcceptanceRequest,
) (DecideFindingAcceptanceResult, error) {
	if s == nil || s.store == nil {
		return DecideFindingAcceptanceResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeDecideFindingAcceptanceRequest(request)
	if err != nil {
		return DecideFindingAcceptanceResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding acceptance request is invalid", err)
	}
	finding, err := s.store.GetFinding(ctx, normalized.FindingID)
	if err != nil {
		return DecideFindingAcceptanceResult{}, apperror.Normalize(err)
	}
	if finding.Validation == nil ||
		finding.Validation.Status != domain.FindingStatusValidated {
		return DecideFindingAcceptanceResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding acceptance requires an explicit validated decision")
	}
	now := time.Now().UTC()
	if now.Before(finding.Validation.CreatedAt) {
		now = finding.Validation.CreatedAt
	}
	acceptance := domain.FindingAcceptance{
		ID: idgen.New("finding-acceptance"), ReportID: finding.ReportID,
		FindingID: finding.ID, RunID: finding.RunID,
		FromStatus: domain.FindingStatusValidated, Status: domain.FindingStatusAccepted,
		ValidationID:                     finding.Validation.ID,
		ValidationArtifactEvidenceCount:  finding.Validation.ArtifactEvidenceCount,
		ValidationArtifactEvidenceDigest: finding.Validation.ArtifactEvidenceDigest,
		DecidedBy:                        normalized.DecidedBy, Reason: normalized.Reason,
		Version: 1, CreatedAt: now,
	}
	operation := domain.FindingAcceptanceOperation{
		KeyDigest: runmutation.OperationKeyDigest("finding_acceptance", finding.RunID,
			normalized.OperationKey),
		RequestFingerprint: runmutation.Fingerprint("finding_acceptance_request.v1",
			finding.ID, finding.Validation.ID, normalized.DecidedBy, normalized.Reason),
		AcceptanceID: acceptance.ID, ValidationID: finding.Validation.ID,
		FindingID: finding.ID, RunID: finding.RunID,
		DecidedBy: normalized.DecidedBy, CreatedAt: now,
	}
	stored, replayed, err := s.store.CreateFindingAcceptance(ctx, operation, acceptance)
	if err != nil {
		return DecideFindingAcceptanceResult{}, apperror.Normalize(err)
	}
	return DecideFindingAcceptanceResult{Acceptance: stored, Replayed: replayed}, nil
}

type AttachFindingRemediationEvidenceRequest struct {
	FindingID    string
	ArtifactID   string
	OperationKey string
	AttachedBy   string
	Note         string
}

type AttachFindingRemediationEvidenceResult struct {
	Evidence domain.FindingArtifactEvidence
	Replayed bool
}

func (s *FindingReportService) AttachRemediationEvidence(ctx context.Context,
	request AttachFindingRemediationEvidenceRequest,
) (AttachFindingRemediationEvidenceResult, error) {
	if s == nil || s.store == nil {
		return AttachFindingRemediationEvidenceResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeAttachFindingRemediationEvidenceRequest(request)
	if err != nil {
		return AttachFindingRemediationEvidenceResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding remediation Evidence request is invalid", err)
	}
	finding, err := s.store.GetFinding(ctx, normalized.FindingID)
	if err != nil {
		return AttachFindingRemediationEvidenceResult{}, apperror.Normalize(err)
	}
	if finding.Acceptance == nil {
		return AttachFindingRemediationEvidenceResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding remediation Evidence requires explicit acceptance")
	}
	blob, err := s.store.GetRunArtifact(ctx, normalized.ArtifactID)
	if err != nil {
		return AttachFindingRemediationEvidenceResult{}, apperror.Normalize(err)
	}
	for _, validationEvidence := range finding.ArtifactEvidence {
		if validationEvidence.ArtifactID == blob.ID {
			return AttachFindingRemediationEvidenceResult{}, apperror.New(
				apperror.CodeFailedPrecondition,
				"finding remediation Evidence must use a fresh Artifact")
		}
	}
	now := time.Now().UTC()
	if now.Before(finding.Acceptance.CreatedAt) {
		now = finding.Acceptance.CreatedAt
	}
	if now.Before(blob.CreatedAt) {
		now = blob.CreatedAt
	}
	evidence := domain.FindingArtifactEvidence{
		ID: idgen.New("finding-remediation-evidence"), ReportID: finding.ReportID,
		FindingID: finding.ID, RunID: finding.RunID,
		Ordinal: len(finding.RemediationEvidence) + 1, ArtifactID: blob.ID,
		ArtifactSHA256: blob.SHA256, ArtifactSize: blob.SizeBytes,
		ArtifactMIME: blob.MIME, ArtifactStream: string(blob.Stream),
		ArtifactTool: blob.ToolName, ArtifactSource: blob.SourceID,
		ArtifactRedacted: blob.Redacted, AttachedBy: normalized.AttachedBy,
		Note: normalized.Note, CreatedAt: now,
	}
	operation := domain.FindingRemediationEvidenceOperation{
		KeyDigest: runmutation.OperationKeyDigest("finding_remediation_evidence",
			finding.RunID, normalized.OperationKey),
		RequestFingerprint: runmutation.Fingerprint(
			"finding_remediation_evidence_request.v1", finding.ID,
			finding.Acceptance.ID, blob.ID, normalized.AttachedBy, normalized.Note),
		EvidenceID: evidence.ID, AcceptanceID: finding.Acceptance.ID,
		FindingID: finding.ID, ArtifactID: blob.ID, RunID: finding.RunID,
		AttachedBy: normalized.AttachedBy, CreatedAt: now,
	}
	stored, replayed, err := s.store.AttachFindingRemediationEvidence(ctx,
		operation, evidence)
	if err != nil {
		return AttachFindingRemediationEvidenceResult{}, apperror.Normalize(err)
	}
	return AttachFindingRemediationEvidenceResult{Evidence: stored, Replayed: replayed}, nil
}

type DecideFindingFixRequest struct {
	FindingID    string
	OperationKey string
	Reason       string
	DecidedBy    string
}

type DecideFindingFixResult struct {
	Fix      domain.FindingFix
	Replayed bool
}

func (s *FindingReportService) Fix(ctx context.Context,
	request DecideFindingFixRequest,
) (DecideFindingFixResult, error) {
	if s == nil || s.store == nil {
		return DecideFindingFixResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "finding report store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeDecideFindingFixRequest(request)
	if err != nil {
		return DecideFindingFixResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding fix request is invalid", err)
	}
	finding, err := s.store.GetFinding(ctx, normalized.FindingID)
	if err != nil {
		return DecideFindingFixResult{}, apperror.Normalize(err)
	}
	if finding.Acceptance == nil {
		return DecideFindingFixResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding fix requires an explicit acceptance decision")
	}
	if len(finding.RemediationEvidence) == 0 {
		return DecideFindingFixResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding fix requires fresh remediation Artifact Evidence")
	}
	digest, err := domain.FindingRemediationEvidenceDigest(finding.RemediationEvidence)
	if err != nil {
		return DecideFindingFixResult{}, err
	}
	now := time.Now().UTC()
	lastCreatedAt := finding.RemediationEvidence[len(finding.RemediationEvidence)-1].CreatedAt
	if now.Before(lastCreatedAt) {
		now = lastCreatedAt
	}
	fix := domain.FindingFix{
		ID: idgen.New("finding-fix"), ReportID: finding.ReportID,
		FindingID: finding.ID, RunID: finding.RunID,
		AcceptanceID: finding.Acceptance.ID, FromStatus: domain.FindingStatusAccepted,
		Status:                    domain.FindingStatusFixed,
		RemediationEvidenceCount:  len(finding.RemediationEvidence),
		RemediationEvidenceDigest: digest, DecidedBy: normalized.DecidedBy,
		Reason: normalized.Reason, Version: 1, CreatedAt: now,
	}
	operation := domain.FindingFixOperation{
		KeyDigest: runmutation.OperationKeyDigest("finding_fix", finding.RunID,
			normalized.OperationKey),
		RequestFingerprint: runmutation.Fingerprint("finding_fix_request.v1",
			finding.ID, finding.Acceptance.ID, digest,
			normalized.DecidedBy, normalized.Reason),
		FixID: fix.ID, AcceptanceID: finding.Acceptance.ID,
		FindingID: finding.ID, RunID: finding.RunID,
		DecidedBy: normalized.DecidedBy, CreatedAt: now,
	}
	stored, replayed, err := s.store.CreateFindingFix(ctx, operation, fix)
	if err != nil {
		return DecideFindingFixResult{}, apperror.Normalize(err)
	}
	return DecideFindingFixResult{Fix: stored, Replayed: replayed}, nil
}

type FindingArtifactEvidenceVerification struct {
	FindingID                 string               `json:"finding_id"`
	RunID                     string               `json:"run_id"`
	Status                    domain.FindingStatus `json:"status"`
	ValidationID              string               `json:"validation_id,omitempty"`
	ArtifactEvidenceCount     int                  `json:"artifact_evidence_count"`
	ArtifactEvidenceDigest    string               `json:"artifact_evidence_digest"`
	AcceptanceID              string               `json:"acceptance_id,omitempty"`
	RemediationEvidenceCount  int                  `json:"remediation_evidence_count"`
	RemediationEvidenceDigest string               `json:"remediation_evidence_digest"`
	FixID                     string               `json:"fix_id,omitempty"`
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
	if err := s.verifyFindingArtifactEvidenceSet(ctx, finding.RunID,
		finding.ArtifactEvidence, "validation"); err != nil {
		return FindingArtifactEvidenceVerification{}, err
	}
	validationDigest, err := domain.FindingArtifactEvidenceDigest(finding.ArtifactEvidence)
	if err != nil {
		return FindingArtifactEvidenceVerification{}, err
	}
	validationID := ""
	if finding.Validation != nil {
		validationID = finding.Validation.ID
		if finding.Validation.ArtifactEvidenceCount != len(finding.ArtifactEvidence) ||
			finding.Validation.ArtifactEvidenceDigest != validationDigest {
			return FindingArtifactEvidenceVerification{}, apperror.New(
				apperror.CodeConflict, "finding validation Evidence snapshot drifted")
		}
	}
	if err := s.verifyFindingArtifactEvidenceSet(ctx, finding.RunID,
		finding.RemediationEvidence, "remediation"); err != nil {
		return FindingArtifactEvidenceVerification{}, err
	}
	remediationDigest, err := domain.FindingRemediationEvidenceDigest(
		finding.RemediationEvidence)
	if err != nil {
		return FindingArtifactEvidenceVerification{}, err
	}
	acceptanceID := ""
	if finding.Acceptance != nil {
		acceptanceID = finding.Acceptance.ID
		if finding.Validation == nil ||
			finding.Acceptance.ValidationID != finding.Validation.ID ||
			finding.Acceptance.ValidationArtifactEvidenceCount !=
				finding.Validation.ArtifactEvidenceCount ||
			finding.Acceptance.ValidationArtifactEvidenceDigest !=
				finding.Validation.ArtifactEvidenceDigest {
			return FindingArtifactEvidenceVerification{}, apperror.New(
				apperror.CodeConflict, "finding acceptance validation snapshot drifted")
		}
	}
	fixID := ""
	if finding.Fix != nil {
		fixID = finding.Fix.ID
		if finding.Acceptance == nil || finding.Fix.AcceptanceID != finding.Acceptance.ID ||
			finding.Fix.RemediationEvidenceCount != len(finding.RemediationEvidence) ||
			finding.Fix.RemediationEvidenceDigest != remediationDigest {
			return FindingArtifactEvidenceVerification{}, apperror.New(
				apperror.CodeConflict, "finding fix remediation Evidence snapshot drifted")
		}
	}
	return FindingArtifactEvidenceVerification{
		FindingID: finding.ID, RunID: finding.RunID, Status: finding.EffectiveStatus(),
		ValidationID: validationID, ArtifactEvidenceCount: len(finding.ArtifactEvidence),
		ArtifactEvidenceDigest: validationDigest, AcceptanceID: acceptanceID,
		RemediationEvidenceCount:  len(finding.RemediationEvidence),
		RemediationEvidenceDigest: remediationDigest, FixID: fixID,
	}, nil
}

func (s *FindingReportService) verifyFindingArtifactEvidenceSet(ctx context.Context,
	runID string, evidenceSet []domain.FindingArtifactEvidence, label string,
) error {
	for _, evidence := range evidenceSet {
		blob, err := s.store.GetRunArtifact(ctx, evidence.ArtifactID)
		if err != nil {
			return apperror.Normalize(err)
		}
		if blob.RunID != runID || blob.SHA256 != evidence.ArtifactSHA256 ||
			blob.SizeBytes != evidence.ArtifactSize || blob.MIME != evidence.ArtifactMIME ||
			string(blob.Stream) != evidence.ArtifactStream ||
			blob.ToolName != evidence.ArtifactTool || blob.SourceID != evidence.ArtifactSource ||
			blob.Redacted != evidence.ArtifactRedacted {
			return apperror.New(apperror.CodeConflict, fmt.Sprintf(
				"finding %s Evidence no longer matches its frozen Artifact", label))
		}
	}
	return nil
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

func normalizeDecideFindingAcceptanceRequest(
	request DecideFindingAcceptanceRequest,
) (DecideFindingAcceptanceRequest, error) {
	request.FindingID = strings.TrimSpace(request.FindingID)
	request.Reason = strings.TrimSpace(redact.String(request.Reason))
	request.DecidedBy = strings.TrimSpace(redact.String(request.DecidedBy))
	if !domain.ValidAgentID(request.FindingID) || !domain.ValidAgentID(request.DecidedBy) ||
		strings.ContainsRune(request.FindingID, 0) || strings.ContainsRune(request.DecidedBy, 0) {
		return DecideFindingAcceptanceRequest{}, errors.New(
			"finding and operator identities are required")
	}
	if err := validateFindingOperationKey(request.OperationKey); err != nil {
		return DecideFindingAcceptanceRequest{}, err
	}
	if err := validateFindingValidationText(request.Reason, "acceptance reason"); err != nil {
		return DecideFindingAcceptanceRequest{}, err
	}
	return request, nil
}

func normalizeAttachFindingRemediationEvidenceRequest(
	request AttachFindingRemediationEvidenceRequest,
) (AttachFindingRemediationEvidenceRequest, error) {
	request.FindingID = strings.TrimSpace(request.FindingID)
	request.ArtifactID = strings.TrimSpace(request.ArtifactID)
	request.AttachedBy = strings.TrimSpace(redact.String(request.AttachedBy))
	request.Note = strings.TrimSpace(redact.String(request.Note))
	if !domain.ValidAgentID(request.FindingID) || !domain.ValidAgentID(request.ArtifactID) ||
		!domain.ValidAgentID(request.AttachedBy) || strings.ContainsRune(request.FindingID, 0) ||
		strings.ContainsRune(request.ArtifactID, 0) || strings.ContainsRune(request.AttachedBy, 0) {
		return AttachFindingRemediationEvidenceRequest{}, errors.New(
			"finding, Artifact, and operator identities are required")
	}
	if err := validateFindingOperationKey(request.OperationKey); err != nil {
		return AttachFindingRemediationEvidenceRequest{}, err
	}
	if err := validateFindingValidationText(request.Note, "remediation evidence note"); err != nil {
		return AttachFindingRemediationEvidenceRequest{}, err
	}
	return request, nil
}

func normalizeDecideFindingFixRequest(
	request DecideFindingFixRequest,
) (DecideFindingFixRequest, error) {
	request.FindingID = strings.TrimSpace(request.FindingID)
	request.Reason = strings.TrimSpace(redact.String(request.Reason))
	request.DecidedBy = strings.TrimSpace(redact.String(request.DecidedBy))
	if !domain.ValidAgentID(request.FindingID) || !domain.ValidAgentID(request.DecidedBy) ||
		strings.ContainsRune(request.FindingID, 0) || strings.ContainsRune(request.DecidedBy, 0) {
		return DecideFindingFixRequest{}, errors.New(
			"finding and operator identities are required")
	}
	if err := validateFindingOperationKey(request.OperationKey); err != nil {
		return DecideFindingFixRequest{}, err
	}
	if err := validateFindingValidationText(request.Reason, "fix reason"); err != nil {
		return DecideFindingFixRequest{}, err
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
