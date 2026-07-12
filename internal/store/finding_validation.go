package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
)

func (s *SQLiteStore) AttachFindingArtifactEvidence(ctx context.Context,
	operation domain.FindingArtifactEvidenceOperation,
	evidence domain.FindingArtifactEvidence,
) (domain.FindingArtifactEvidence, bool, error) {
	operation = normalizeFindingArtifactEvidenceOperation(operation)
	evidence = normalizeFindingArtifactEvidence(evidence)
	if err := validateFindingArtifactEvidenceMutation(operation, evidence); err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireReadOnlyFanoutWriteLockTx(ctx, tx, operation.RunID); err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	existingOperation, found, err := getFindingArtifactEvidenceOperation(
		ctx, tx, operation.KeyDigest)
	if err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if found {
		if err := validateFindingArtifactEvidenceReplay(existingOperation, operation); err != nil {
			return domain.FindingArtifactEvidence{}, false, err
		}
		stored, err := getFindingArtifactEvidence(ctx, tx, existingOperation.EvidenceID)
		if err != nil {
			return domain.FindingArtifactEvidence{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.FindingArtifactEvidence{}, false, err
		}
		return stored, true, nil
	}
	finding, err := getFindingByID(ctx, tx, operation.FindingID)
	if err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if finding.Validation != nil {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeConflict, "finding already has an immutable validation decision")
	}
	if len(finding.ArtifactEvidence) >= domain.MaxFindingArtifactEvidence {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeResourceExhausted, "finding Artifact Evidence limit was reached")
	}
	artifactBlob, err := getRunArtifactRow(tx.QueryRowContext(ctx,
		runArtifactSelect+` WHERE id = ?`, operation.ArtifactID))
	if err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if finding.RunID != operation.RunID || artifactBlob.RunID != operation.RunID ||
		evidence.ReportID != finding.ReportID || evidence.FindingID != finding.ID ||
		evidence.RunID != finding.RunID || evidence.ArtifactID != artifactBlob.ID ||
		evidence.ArtifactSHA256 != artifactBlob.SHA256 ||
		evidence.ArtifactSize != artifactBlob.SizeBytes ||
		evidence.ArtifactMIME != artifactBlob.MIME ||
		evidence.ArtifactStream != string(artifactBlob.Stream) ||
		evidence.ArtifactTool != artifactBlob.ToolName ||
		evidence.ArtifactSource != artifactBlob.SourceID ||
		evidence.ArtifactRedacted != artifactBlob.Redacted {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding Artifact Evidence does not match a same-Run frozen Artifact")
	}
	if evidence.CreatedAt.Before(finding.CreatedAt) ||
		evidence.CreatedAt.Before(artifactBlob.CreatedAt) {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding Artifact Evidence predates its finding or Artifact")
	}
	evidence.Ordinal = len(finding.ArtifactEvidence) + 1
	if err := evidence.Validate(); err != nil {
		return domain.FindingArtifactEvidence{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding Artifact Evidence is invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_artifact_evidence
		(id, report_id, finding_id, run_id, ordinal, artifact_id, artifact_sha256,
		artifact_size_bytes, artifact_mime, artifact_stream, artifact_tool,
		artifact_source_id, artifact_redacted, attached_by, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, evidence.ID,
		evidence.ReportID, evidence.FindingID, evidence.RunID, evidence.Ordinal,
		evidence.ArtifactID, evidence.ArtifactSHA256, evidence.ArtifactSize,
		evidence.ArtifactMIME, evidence.ArtifactStream, evidence.ArtifactTool,
		evidence.ArtifactSource, evidence.ArtifactRedacted, evidence.AttachedBy, evidence.Note,
		ts(evidence.CreatedAt)); err != nil {
		return domain.FindingArtifactEvidence{}, false, normalizeFindingMutationError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_artifact_evidence_operations
		(operation_key_digest, request_fingerprint, evidence_id, finding_id,
		artifact_id, run_id, attached_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.KeyDigest, operation.RequestFingerprint, evidence.ID,
		operation.FindingID, operation.ArtifactID, operation.RunID,
		operation.AttachedBy, ts(operation.CreatedAt)); err != nil {
		return domain.FindingArtifactEvidence{}, false, normalizeFindingMutationError(err)
	}
	run, _, err := getCoordinatorRunTx(ctx, tx, operation.RunID)
	if err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run,
		events.FindingArtifactEvidenceAttachedEvent, "operator", evidence.ID,
		map[string]any{
			"evidence_id": evidence.ID, "report_id": evidence.ReportID,
			"finding_id": evidence.FindingID, "artifact_id": evidence.ArtifactID,
			"ordinal": evidence.Ordinal, "artifact_sha256": evidence.ArtifactSHA256,
			"artifact_size_bytes": evidence.ArtifactSize,
			"artifact_mime":       evidence.ArtifactMIME,
			"artifact_stream":     evidence.ArtifactStream,
			"artifact_tool":       evidence.ArtifactTool,
			"artifact_redacted":   evidence.ArtifactRedacted,
			"attached_by":         evidence.AttachedBy,
		}); err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	return evidence, false, nil
}

func (s *SQLiteStore) CreateFindingValidation(ctx context.Context,
	operation domain.FindingValidationOperation,
	validation domain.FindingValidation,
) (domain.FindingValidation, bool, error) {
	operation = normalizeFindingValidationOperation(operation)
	validation = normalizeFindingValidation(validation)
	if err := validateFindingValidationMutation(operation, validation); err != nil {
		return domain.FindingValidation{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.FindingValidation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireReadOnlyFanoutWriteLockTx(ctx, tx, operation.RunID); err != nil {
		return domain.FindingValidation{}, false, err
	}
	existingOperation, found, err := getFindingValidationOperation(
		ctx, tx, operation.KeyDigest)
	if err != nil {
		return domain.FindingValidation{}, false, err
	}
	if found {
		if err := validateFindingValidationReplay(existingOperation, operation); err != nil {
			return domain.FindingValidation{}, false, err
		}
		stored, err := getFindingValidation(ctx, tx, existingOperation.ValidationID)
		if err != nil {
			return domain.FindingValidation{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.FindingValidation{}, false, err
		}
		return stored, true, nil
	}
	finding, err := getFindingByID(ctx, tx, operation.FindingID)
	if err != nil {
		return domain.FindingValidation{}, false, err
	}
	if finding.Validation != nil {
		return domain.FindingValidation{}, false, apperror.New(
			apperror.CodeConflict, fmt.Sprintf("finding was already %s by %s",
				finding.Validation.Status, finding.Validation.DecidedBy))
	}
	if finding.RunID != operation.RunID || validation.ReportID != finding.ReportID ||
		validation.FindingID != finding.ID || validation.RunID != finding.RunID ||
		validation.FromStatus != finding.Status {
		return domain.FindingValidation{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding validation does not match its immutable draft finding")
	}
	if validation.Status == domain.FindingStatusValidated &&
		len(finding.ArtifactEvidence) == 0 {
		return domain.FindingValidation{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"validated finding requires at least one frozen Artifact Evidence record")
	}
	digest, err := domain.FindingArtifactEvidenceDigest(finding.ArtifactEvidence)
	if err != nil {
		return domain.FindingValidation{}, false, err
	}
	validation.ArtifactEvidenceCount = len(finding.ArtifactEvidence)
	validation.ArtifactEvidenceDigest = digest
	if len(finding.ArtifactEvidence) > 0 {
		lastCreatedAt := finding.ArtifactEvidence[len(finding.ArtifactEvidence)-1].CreatedAt
		if validation.CreatedAt.Before(lastCreatedAt) {
			validation.CreatedAt = lastCreatedAt
			operation.CreatedAt = lastCreatedAt
		}
	}
	if err := validation.Validate(); err != nil {
		return domain.FindingValidation{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding validation is invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_validation_decisions
		(id, report_id, finding_id, run_id, from_status, status, decided_by, reason,
		artifact_evidence_count, artifact_evidence_digest, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, validation.ID,
		validation.ReportID, validation.FindingID, validation.RunID,
		validation.FromStatus, validation.Status, validation.DecidedBy,
		validation.Reason, validation.ArtifactEvidenceCount,
		validation.ArtifactEvidenceDigest, validation.Version,
		ts(validation.CreatedAt)); err != nil {
		return domain.FindingValidation{}, false, normalizeFindingMutationError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_validation_operations
		(operation_key_digest, request_fingerprint, validation_id, finding_id,
		run_id, status, decided_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.KeyDigest, operation.RequestFingerprint, validation.ID,
		operation.FindingID, operation.RunID, operation.Status,
		operation.DecidedBy, ts(operation.CreatedAt)); err != nil {
		return domain.FindingValidation{}, false, normalizeFindingMutationError(err)
	}
	run, _, err := getCoordinatorRunTx(ctx, tx, operation.RunID)
	if err != nil {
		return domain.FindingValidation{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run,
		events.FindingValidationDecidedEvent, "operator", validation.ID,
		map[string]any{
			"validation_id": validation.ID, "report_id": validation.ReportID,
			"finding_id": validation.FindingID, "from_status": validation.FromStatus,
			"status": validation.Status, "decided_by": validation.DecidedBy,
			"artifact_evidence_count":  validation.ArtifactEvidenceCount,
			"artifact_evidence_digest": validation.ArtifactEvidenceDigest,
			"validation_version":       validation.Version,
		}); err != nil {
		return domain.FindingValidation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FindingValidation{}, false, err
	}
	return validation, false, nil
}

func getFindingByID(ctx context.Context, queryer readOnlyFanoutQueryer,
	id string,
) (domain.Finding, error) {
	var reportID string
	if err := queryer.QueryRowContext(ctx, `SELECT report_id FROM findings WHERE id = ?`, id).
		Scan(&reportID); err != nil {
		return domain.Finding{}, err
	}
	report, err := getFindingReport(ctx, queryer, reportID)
	if err != nil {
		return domain.Finding{}, err
	}
	for _, finding := range report.Findings {
		if finding.ID == id {
			return finding, nil
		}
	}
	return domain.Finding{}, sql.ErrNoRows
}

func getFindingArtifactEvidence(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.FindingArtifactEvidence, error) {
	return scanFindingArtifactEvidence(queryer.QueryRowContext(ctx,
		findingArtifactEvidenceSelect+` WHERE id = ?`, id))
}

func getFindingValidation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.FindingValidation, error) {
	return scanFindingValidation(queryer.QueryRowContext(ctx,
		findingValidationSelect+` WHERE id = ?`, id))
}

func getFindingArtifactEvidenceOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (domain.FindingArtifactEvidenceOperation, bool, error) {
	var value domain.FindingArtifactEvidenceOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, evidence_id, finding_id, artifact_id, run_id,
		attached_by, created_at FROM finding_artifact_evidence_operations
		WHERE operation_key_digest = ?`, keyDigest).Scan(&value.KeyDigest,
		&value.RequestFingerprint, &value.EvidenceID, &value.FindingID,
		&value.ArtifactID, &value.RunID, &value.AttachedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FindingArtifactEvidenceOperation{}, false, nil
	}
	if err != nil {
		return domain.FindingArtifactEvidenceOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	return value, true, value.Validate()
}

func getFindingValidationOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (domain.FindingValidationOperation, bool, error) {
	var value domain.FindingValidationOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, validation_id, finding_id, run_id, status, decided_by,
		created_at FROM finding_validation_operations WHERE operation_key_digest = ?`,
		keyDigest).Scan(&value.KeyDigest, &value.RequestFingerprint,
		&value.ValidationID, &value.FindingID, &value.RunID, &value.Status,
		&value.DecidedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FindingValidationOperation{}, false, nil
	}
	if err != nil {
		return domain.FindingValidationOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	return value, true, value.Validate()
}

func normalizeFindingArtifactEvidence(value domain.FindingArtifactEvidence,
) domain.FindingArtifactEvidence {
	value.ID = strings.TrimSpace(value.ID)
	value.ReportID = strings.TrimSpace(value.ReportID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.ArtifactID = strings.TrimSpace(value.ArtifactID)
	value.ArtifactSHA256 = strings.TrimSpace(value.ArtifactSHA256)
	value.ArtifactMIME = strings.TrimSpace(value.ArtifactMIME)
	value.ArtifactStream = strings.TrimSpace(value.ArtifactStream)
	value.ArtifactTool = strings.TrimSpace(value.ArtifactTool)
	value.ArtifactSource = strings.TrimSpace(value.ArtifactSource)
	value.AttachedBy = strings.TrimSpace(redact.String(value.AttachedBy))
	value.Note = strings.TrimSpace(redact.String(value.Note))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func normalizeFindingArtifactEvidenceOperation(
	value domain.FindingArtifactEvidenceOperation,
) domain.FindingArtifactEvidenceOperation {
	value.KeyDigest = strings.TrimSpace(value.KeyDigest)
	value.RequestFingerprint = strings.TrimSpace(value.RequestFingerprint)
	value.EvidenceID = strings.TrimSpace(value.EvidenceID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.ArtifactID = strings.TrimSpace(value.ArtifactID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.AttachedBy = strings.TrimSpace(redact.String(value.AttachedBy))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func normalizeFindingValidation(value domain.FindingValidation) domain.FindingValidation {
	value.ID = strings.TrimSpace(value.ID)
	value.ReportID = strings.TrimSpace(value.ReportID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.FromStatus = domain.FindingStatus(strings.TrimSpace(string(value.FromStatus)))
	value.Status = domain.FindingStatus(strings.TrimSpace(string(value.Status)))
	value.DecidedBy = strings.TrimSpace(redact.String(value.DecidedBy))
	value.Reason = strings.TrimSpace(redact.String(value.Reason))
	value.ArtifactEvidenceDigest = strings.TrimSpace(value.ArtifactEvidenceDigest)
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func normalizeFindingValidationOperation(
	value domain.FindingValidationOperation,
) domain.FindingValidationOperation {
	value.KeyDigest = strings.TrimSpace(value.KeyDigest)
	value.RequestFingerprint = strings.TrimSpace(value.RequestFingerprint)
	value.ValidationID = strings.TrimSpace(value.ValidationID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.Status = domain.FindingStatus(strings.TrimSpace(string(value.Status)))
	value.DecidedBy = strings.TrimSpace(redact.String(value.DecidedBy))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func validateFindingArtifactEvidenceMutation(
	operation domain.FindingArtifactEvidenceOperation,
	evidence domain.FindingArtifactEvidence,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding Artifact Evidence operation is invalid", err)
	}
	validationCopy := evidence
	validationCopy.Ordinal = 1 // The Store assigns the serialized per-Finding ordinal.
	if err := validationCopy.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding Artifact Evidence is invalid", err)
	}
	if operation.EvidenceID != evidence.ID || operation.FindingID != evidence.FindingID ||
		operation.ArtifactID != evidence.ArtifactID || operation.RunID != evidence.RunID ||
		operation.AttachedBy != evidence.AttachedBy ||
		!operation.CreatedAt.Equal(evidence.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"finding Artifact Evidence operation does not match its evidence")
	}
	return nil
}

func validateFindingValidationMutation(operation domain.FindingValidationOperation,
	validation domain.FindingValidation,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding validation operation is invalid", err)
	}
	if err := validation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding validation is invalid", err)
	}
	if operation.ValidationID != validation.ID ||
		operation.FindingID != validation.FindingID ||
		operation.RunID != validation.RunID || operation.Status != validation.Status ||
		operation.DecidedBy != validation.DecidedBy ||
		!operation.CreatedAt.Equal(validation.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"finding validation operation does not match its decision")
	}
	return nil
}

func validateFindingArtifactEvidenceReplay(existing,
	request domain.FindingArtifactEvidenceOperation,
) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.FindingID != request.FindingID || existing.ArtifactID != request.ArtifactID ||
		existing.RunID != request.RunID || existing.AttachedBy != request.AttachedBy {
		return apperror.New(apperror.CodeConflict,
			"finding Artifact Evidence operation key was already used for different intent")
	}
	return nil
}

func validateFindingValidationReplay(existing,
	request domain.FindingValidationOperation,
) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.FindingID != request.FindingID || existing.RunID != request.RunID ||
		existing.Status != request.Status || existing.DecidedBy != request.DecidedBy {
		return apperror.New(apperror.CodeConflict,
			"finding validation operation key was already used for different intent")
	}
	return nil
}

func normalizeFindingMutationError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint failed") {
		return apperror.Wrap(apperror.CodeConflict,
			"finding lifecycle fact already exists", err)
	}
	if strings.Contains(message, "binding is invalid") {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"finding lifecycle binding was rejected", err)
	}
	return err
}
