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

func (s *SQLiteStore) CreateFindingAcceptance(ctx context.Context,
	operation domain.FindingAcceptanceOperation,
	acceptance domain.FindingAcceptance,
) (domain.FindingAcceptance, bool, error) {
	operation = normalizeFindingAcceptanceOperation(operation)
	acceptance = normalizeFindingAcceptance(acceptance)
	if err := validateFindingAcceptanceMutation(operation, acceptance); err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireReadOnlyFanoutWriteLockTx(ctx, tx, operation.RunID); err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	existingOperation, found, err := getFindingAcceptanceOperation(
		ctx, tx, operation.KeyDigest)
	if err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	if found {
		if err := validateFindingAcceptanceReplay(existingOperation, operation); err != nil {
			return domain.FindingAcceptance{}, false, err
		}
		stored, err := getFindingAcceptance(ctx, tx, existingOperation.AcceptanceID)
		if err != nil {
			return domain.FindingAcceptance{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.FindingAcceptance{}, false, err
		}
		return stored, true, nil
	}
	finding, err := getFindingByID(ctx, tx, operation.FindingID)
	if err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	if finding.Acceptance != nil {
		return domain.FindingAcceptance{}, false, apperror.New(
			apperror.CodeConflict, fmt.Sprintf("finding was already accepted by %s",
				finding.Acceptance.DecidedBy))
	}
	if finding.Validation == nil ||
		finding.Validation.Status != domain.FindingStatusValidated {
		return domain.FindingAcceptance{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding acceptance requires an explicit validated decision")
	}
	validation := finding.Validation
	if finding.RunID != operation.RunID || acceptance.ReportID != finding.ReportID ||
		acceptance.FindingID != finding.ID || acceptance.RunID != finding.RunID ||
		acceptance.ValidationID != validation.ID ||
		acceptance.FromStatus != validation.Status {
		return domain.FindingAcceptance{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding acceptance does not match its validated finding")
	}
	acceptance.ValidationArtifactEvidenceCount = validation.ArtifactEvidenceCount
	acceptance.ValidationArtifactEvidenceDigest = validation.ArtifactEvidenceDigest
	if acceptance.CreatedAt.Before(validation.CreatedAt) {
		acceptance.CreatedAt = validation.CreatedAt
		operation.CreatedAt = validation.CreatedAt
	}
	if err := acceptance.Validate(); err != nil {
		return domain.FindingAcceptance{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding acceptance is invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_acceptance_decisions
		(id, report_id, finding_id, run_id, validation_id, from_status, status,
		validation_artifact_evidence_count, validation_artifact_evidence_digest,
		decided_by, reason, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, acceptance.ID,
		acceptance.ReportID, acceptance.FindingID, acceptance.RunID,
		acceptance.ValidationID, acceptance.FromStatus, acceptance.Status,
		acceptance.ValidationArtifactEvidenceCount,
		acceptance.ValidationArtifactEvidenceDigest, acceptance.DecidedBy,
		acceptance.Reason, acceptance.Version, ts(acceptance.CreatedAt)); err != nil {
		return domain.FindingAcceptance{}, false, normalizeFindingMutationError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_acceptance_operations
		(operation_key_digest, request_fingerprint, acceptance_id, validation_id,
		finding_id, run_id, decided_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.KeyDigest, operation.RequestFingerprint, acceptance.ID,
		operation.ValidationID, operation.FindingID, operation.RunID,
		operation.DecidedBy, ts(operation.CreatedAt)); err != nil {
		return domain.FindingAcceptance{}, false, normalizeFindingMutationError(err)
	}
	run, _, err := getCoordinatorRunTx(ctx, tx, operation.RunID)
	if err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.FindingAcceptedEvent,
		"operator", acceptance.ID, map[string]any{
			"acceptance_id": acceptance.ID, "report_id": acceptance.ReportID,
			"finding_id": acceptance.FindingID, "validation_id": acceptance.ValidationID,
			"from_status": acceptance.FromStatus, "status": acceptance.Status,
			"decided_by":                          acceptance.DecidedBy,
			"validation_artifact_evidence_count":  acceptance.ValidationArtifactEvidenceCount,
			"validation_artifact_evidence_digest": acceptance.ValidationArtifactEvidenceDigest,
			"acceptance_version":                  acceptance.Version,
		}); err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FindingAcceptance{}, false, err
	}
	return acceptance, false, nil
}

func (s *SQLiteStore) AttachFindingRemediationEvidence(ctx context.Context,
	operation domain.FindingRemediationEvidenceOperation,
	evidence domain.FindingArtifactEvidence,
) (domain.FindingArtifactEvidence, bool, error) {
	operation = normalizeFindingRemediationEvidenceOperation(operation)
	evidence = normalizeFindingArtifactEvidence(evidence)
	if err := validateFindingRemediationEvidenceMutation(operation, evidence); err != nil {
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
	existingOperation, found, err := getFindingRemediationEvidenceOperation(
		ctx, tx, operation.KeyDigest)
	if err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if found {
		if err := validateFindingRemediationEvidenceReplay(existingOperation,
			operation); err != nil {
			return domain.FindingArtifactEvidence{}, false, err
		}
		stored, err := getFindingRemediationEvidence(ctx, tx,
			existingOperation.EvidenceID)
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
	if finding.Acceptance == nil {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding remediation Evidence requires explicit acceptance")
	}
	if finding.Fix != nil {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeConflict, "finding already has an immutable fix decision")
	}
	if operation.AcceptanceID != finding.Acceptance.ID {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding remediation Evidence does not match its acceptance")
	}
	if len(finding.RemediationEvidence) >= domain.MaxFindingRemediationEvidence {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeResourceExhausted,
			"finding remediation Evidence limit was reached")
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
			"finding remediation Evidence does not match a same-Run frozen Artifact")
	}
	for _, validationEvidence := range finding.ArtifactEvidence {
		if validationEvidence.ArtifactID == artifactBlob.ID {
			return domain.FindingArtifactEvidence{}, false, apperror.New(
				apperror.CodeFailedPrecondition,
				"finding remediation Evidence must use a fresh Artifact")
		}
	}
	if err := requireFreshRemediationArtifactTx(ctx, tx, operation.RunID,
		artifactBlob.ID, finding.Acceptance.ID); err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if evidence.CreatedAt.Before(finding.Acceptance.CreatedAt) ||
		evidence.CreatedAt.Before(artifactBlob.CreatedAt) {
		return domain.FindingArtifactEvidence{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding remediation Evidence predates its acceptance or Artifact")
	}
	evidence.Ordinal = len(finding.RemediationEvidence) + 1
	if err := evidence.Validate(); err != nil {
		return domain.FindingArtifactEvidence{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding remediation Evidence is invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_remediation_evidence
		(id, report_id, finding_id, run_id, acceptance_id, ordinal, artifact_id,
		artifact_sha256, artifact_size_bytes, artifact_mime, artifact_stream,
		artifact_tool, artifact_source_id, artifact_redacted, attached_by, note,
		created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evidence.ID, evidence.ReportID, evidence.FindingID, evidence.RunID,
		finding.Acceptance.ID, evidence.Ordinal, evidence.ArtifactID,
		evidence.ArtifactSHA256, evidence.ArtifactSize, evidence.ArtifactMIME,
		evidence.ArtifactStream, evidence.ArtifactTool, evidence.ArtifactSource,
		evidence.ArtifactRedacted, evidence.AttachedBy, evidence.Note,
		ts(evidence.CreatedAt)); err != nil {
		return domain.FindingArtifactEvidence{}, false, normalizeFindingMutationError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_remediation_evidence_operations
		(operation_key_digest, request_fingerprint, evidence_id, acceptance_id,
		finding_id, artifact_id, run_id, attached_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, evidence.ID, operation.AcceptanceID,
		operation.FindingID, operation.ArtifactID, operation.RunID,
		operation.AttachedBy, ts(operation.CreatedAt)); err != nil {
		return domain.FindingArtifactEvidence{}, false, normalizeFindingMutationError(err)
	}
	run, _, err := getCoordinatorRunTx(ctx, tx, operation.RunID)
	if err != nil {
		return domain.FindingArtifactEvidence{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run,
		events.FindingRemediationEvidenceAttachedEvent, "operator", evidence.ID,
		map[string]any{
			"evidence_id": evidence.ID, "acceptance_id": finding.Acceptance.ID,
			"report_id": evidence.ReportID, "finding_id": evidence.FindingID,
			"artifact_id": evidence.ArtifactID, "ordinal": evidence.Ordinal,
			"artifact_sha256":     evidence.ArtifactSHA256,
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

func (s *SQLiteStore) CreateFindingFix(ctx context.Context,
	operation domain.FindingFixOperation,
	fix domain.FindingFix,
) (domain.FindingFix, bool, error) {
	operation = normalizeFindingFixOperation(operation)
	fix = normalizeFindingFix(fix)
	if err := validateFindingFixMutation(operation, fix); err != nil {
		return domain.FindingFix{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.FindingFix{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireReadOnlyFanoutWriteLockTx(ctx, tx, operation.RunID); err != nil {
		return domain.FindingFix{}, false, err
	}
	existingOperation, found, err := getFindingFixOperation(ctx, tx,
		operation.KeyDigest)
	if err != nil {
		return domain.FindingFix{}, false, err
	}
	if found {
		if err := validateFindingFixReplay(existingOperation, operation); err != nil {
			return domain.FindingFix{}, false, err
		}
		stored, err := getFindingFix(ctx, tx, existingOperation.FixID)
		if err != nil {
			return domain.FindingFix{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.FindingFix{}, false, err
		}
		return stored, true, nil
	}
	finding, err := getFindingByID(ctx, tx, operation.FindingID)
	if err != nil {
		return domain.FindingFix{}, false, err
	}
	if finding.Acceptance == nil {
		return domain.FindingFix{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding fix requires an explicit acceptance decision")
	}
	if finding.Fix != nil {
		return domain.FindingFix{}, false, apperror.New(
			apperror.CodeConflict, fmt.Sprintf("finding was already fixed by %s",
				finding.Fix.DecidedBy))
	}
	if len(finding.RemediationEvidence) == 0 {
		return domain.FindingFix{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding fix requires fresh remediation Artifact Evidence")
	}
	if finding.RunID != operation.RunID || fix.ReportID != finding.ReportID ||
		fix.FindingID != finding.ID || fix.RunID != finding.RunID ||
		fix.AcceptanceID != finding.Acceptance.ID ||
		fix.FromStatus != domain.FindingStatusAccepted {
		return domain.FindingFix{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding fix does not match its accepted finding")
	}
	digest, err := domain.FindingRemediationEvidenceDigest(finding.RemediationEvidence)
	if err != nil {
		return domain.FindingFix{}, false, err
	}
	fix.RemediationEvidenceCount = len(finding.RemediationEvidence)
	fix.RemediationEvidenceDigest = digest
	lastCreatedAt := finding.RemediationEvidence[len(finding.RemediationEvidence)-1].CreatedAt
	if fix.CreatedAt.Before(lastCreatedAt) {
		fix.CreatedAt = lastCreatedAt
		operation.CreatedAt = lastCreatedAt
	}
	if err := fix.Validate(); err != nil {
		return domain.FindingFix{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "finding fix is invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_fix_decisions
		(id, report_id, finding_id, run_id, acceptance_id, from_status, status,
		remediation_evidence_count, remediation_evidence_digest, decided_by,
		reason, version, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fix.ID, fix.ReportID, fix.FindingID, fix.RunID, fix.AcceptanceID,
		fix.FromStatus, fix.Status, fix.RemediationEvidenceCount,
		fix.RemediationEvidenceDigest, fix.DecidedBy, fix.Reason, fix.Version,
		ts(fix.CreatedAt)); err != nil {
		return domain.FindingFix{}, false, normalizeFindingMutationError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_fix_operations
		(operation_key_digest, request_fingerprint, fix_id, acceptance_id,
		finding_id, run_id, decided_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.KeyDigest, operation.RequestFingerprint, fix.ID,
		operation.AcceptanceID, operation.FindingID, operation.RunID,
		operation.DecidedBy, ts(operation.CreatedAt)); err != nil {
		return domain.FindingFix{}, false, normalizeFindingMutationError(err)
	}
	run, _, err := getCoordinatorRunTx(ctx, tx, operation.RunID)
	if err != nil {
		return domain.FindingFix{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.FindingFixedEvent,
		"operator", fix.ID, map[string]any{
			"fix_id": fix.ID, "acceptance_id": fix.AcceptanceID,
			"report_id": fix.ReportID, "finding_id": fix.FindingID,
			"from_status": fix.FromStatus, "status": fix.Status,
			"decided_by":                  fix.DecidedBy,
			"remediation_evidence_count":  fix.RemediationEvidenceCount,
			"remediation_evidence_digest": fix.RemediationEvidenceDigest,
			"fix_version":                 fix.Version,
		}); err != nil {
		return domain.FindingFix{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FindingFix{}, false, err
	}
	return fix, false, nil
}

func requireFreshRemediationArtifactTx(ctx context.Context, tx *sql.Tx,
	runID, artifactID, acceptanceID string,
) error {
	var artifactSequence, acceptanceSequence int64
	if err := tx.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT MAX(sequence) FROM run_events
			WHERE run_id = ? AND type = ? AND subject_id = ?), 0),
		COALESCE((SELECT MAX(sequence) FROM run_events
			WHERE run_id = ? AND type = ? AND subject_id = ?), 0)`,
		runID, events.ArtifactCreatedEvent, artifactID,
		runID, events.FindingAcceptedEvent, acceptanceID).
		Scan(&artifactSequence, &acceptanceSequence); err != nil {
		return err
	}
	if acceptanceSequence <= 0 || artifactSequence <= acceptanceSequence {
		return apperror.New(apperror.CodeFailedPrecondition,
			"finding remediation Evidence requires an Artifact created after acceptance")
	}
	return nil
}

func getFindingAcceptance(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.FindingAcceptance, error) {
	return scanFindingAcceptance(queryer.QueryRowContext(ctx,
		findingAcceptanceSelect+` WHERE id = ?`, id))
}

func getFindingRemediationEvidence(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.FindingArtifactEvidence, error) {
	return scanFindingArtifactEvidence(queryer.QueryRowContext(ctx,
		findingRemediationEvidenceSelect+` WHERE id = ?`, id))
}

func getFindingFix(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.FindingFix, error) {
	return scanFindingFix(queryer.QueryRowContext(ctx,
		findingFixSelect+` WHERE id = ?`, id))
}

func getFindingAcceptanceOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (domain.FindingAcceptanceOperation, bool, error) {
	var value domain.FindingAcceptanceOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, acceptance_id, validation_id, finding_id, run_id,
		decided_by, created_at FROM finding_acceptance_operations
		WHERE operation_key_digest = ?`, keyDigest).Scan(&value.KeyDigest,
		&value.RequestFingerprint, &value.AcceptanceID, &value.ValidationID,
		&value.FindingID, &value.RunID, &value.DecidedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FindingAcceptanceOperation{}, false, nil
	}
	if err != nil {
		return domain.FindingAcceptanceOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	return value, true, value.Validate()
}

func getFindingRemediationEvidenceOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (domain.FindingRemediationEvidenceOperation, bool, error) {
	var value domain.FindingRemediationEvidenceOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, evidence_id, acceptance_id, finding_id, artifact_id,
		run_id, attached_by, created_at FROM finding_remediation_evidence_operations
		WHERE operation_key_digest = ?`, keyDigest).Scan(&value.KeyDigest,
		&value.RequestFingerprint, &value.EvidenceID, &value.AcceptanceID,
		&value.FindingID, &value.ArtifactID, &value.RunID, &value.AttachedBy,
		&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FindingRemediationEvidenceOperation{}, false, nil
	}
	if err != nil {
		return domain.FindingRemediationEvidenceOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	return value, true, value.Validate()
}

func getFindingFixOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (domain.FindingFixOperation, bool, error) {
	var value domain.FindingFixOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, fix_id, acceptance_id, finding_id, run_id, decided_by,
		created_at FROM finding_fix_operations WHERE operation_key_digest = ?`,
		keyDigest).Scan(&value.KeyDigest, &value.RequestFingerprint, &value.FixID,
		&value.AcceptanceID, &value.FindingID, &value.RunID, &value.DecidedBy,
		&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FindingFixOperation{}, false, nil
	}
	if err != nil {
		return domain.FindingFixOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	return value, true, value.Validate()
}

func normalizeFindingAcceptance(value domain.FindingAcceptance) domain.FindingAcceptance {
	value.ID = strings.TrimSpace(value.ID)
	value.ReportID = strings.TrimSpace(value.ReportID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.FromStatus = domain.FindingStatus(strings.TrimSpace(string(value.FromStatus)))
	value.Status = domain.FindingStatus(strings.TrimSpace(string(value.Status)))
	value.ValidationID = strings.TrimSpace(value.ValidationID)
	value.ValidationArtifactEvidenceDigest = strings.TrimSpace(
		value.ValidationArtifactEvidenceDigest)
	value.DecidedBy = strings.TrimSpace(redact.String(value.DecidedBy))
	value.Reason = strings.TrimSpace(redact.String(value.Reason))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func normalizeFindingAcceptanceOperation(
	value domain.FindingAcceptanceOperation,
) domain.FindingAcceptanceOperation {
	value.KeyDigest = strings.TrimSpace(value.KeyDigest)
	value.RequestFingerprint = strings.TrimSpace(value.RequestFingerprint)
	value.AcceptanceID = strings.TrimSpace(value.AcceptanceID)
	value.ValidationID = strings.TrimSpace(value.ValidationID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.DecidedBy = strings.TrimSpace(redact.String(value.DecidedBy))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func normalizeFindingRemediationEvidenceOperation(
	value domain.FindingRemediationEvidenceOperation,
) domain.FindingRemediationEvidenceOperation {
	value.KeyDigest = strings.TrimSpace(value.KeyDigest)
	value.RequestFingerprint = strings.TrimSpace(value.RequestFingerprint)
	value.EvidenceID = strings.TrimSpace(value.EvidenceID)
	value.AcceptanceID = strings.TrimSpace(value.AcceptanceID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.ArtifactID = strings.TrimSpace(value.ArtifactID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.AttachedBy = strings.TrimSpace(redact.String(value.AttachedBy))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func normalizeFindingFix(value domain.FindingFix) domain.FindingFix {
	value.ID = strings.TrimSpace(value.ID)
	value.ReportID = strings.TrimSpace(value.ReportID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.AcceptanceID = strings.TrimSpace(value.AcceptanceID)
	value.FromStatus = domain.FindingStatus(strings.TrimSpace(string(value.FromStatus)))
	value.Status = domain.FindingStatus(strings.TrimSpace(string(value.Status)))
	value.RemediationEvidenceDigest = strings.TrimSpace(value.RemediationEvidenceDigest)
	value.DecidedBy = strings.TrimSpace(redact.String(value.DecidedBy))
	value.Reason = strings.TrimSpace(redact.String(value.Reason))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func normalizeFindingFixOperation(
	value domain.FindingFixOperation,
) domain.FindingFixOperation {
	value.KeyDigest = strings.TrimSpace(value.KeyDigest)
	value.RequestFingerprint = strings.TrimSpace(value.RequestFingerprint)
	value.FixID = strings.TrimSpace(value.FixID)
	value.AcceptanceID = strings.TrimSpace(value.AcceptanceID)
	value.FindingID = strings.TrimSpace(value.FindingID)
	value.RunID = strings.TrimSpace(value.RunID)
	value.DecidedBy = strings.TrimSpace(redact.String(value.DecidedBy))
	value.CreatedAt = value.CreatedAt.UTC()
	return value
}

func validateFindingAcceptanceMutation(operation domain.FindingAcceptanceOperation,
	acceptance domain.FindingAcceptance,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding acceptance operation is invalid", err)
	}
	if err := acceptance.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding acceptance is invalid", err)
	}
	if operation.AcceptanceID != acceptance.ID ||
		operation.ValidationID != acceptance.ValidationID ||
		operation.FindingID != acceptance.FindingID ||
		operation.RunID != acceptance.RunID || operation.DecidedBy != acceptance.DecidedBy ||
		!operation.CreatedAt.Equal(acceptance.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"finding acceptance operation does not match its decision")
	}
	return nil
}

func validateFindingRemediationEvidenceMutation(
	operation domain.FindingRemediationEvidenceOperation,
	evidence domain.FindingArtifactEvidence,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding remediation Evidence operation is invalid", err)
	}
	validationCopy := evidence
	validationCopy.Ordinal = 1
	if err := validationCopy.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding remediation Evidence is invalid", err)
	}
	if operation.EvidenceID != evidence.ID || operation.FindingID != evidence.FindingID ||
		operation.ArtifactID != evidence.ArtifactID || operation.RunID != evidence.RunID ||
		operation.AttachedBy != evidence.AttachedBy ||
		!operation.CreatedAt.Equal(evidence.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"finding remediation Evidence operation does not match its evidence")
	}
	return nil
}

func validateFindingFixMutation(operation domain.FindingFixOperation,
	fix domain.FindingFix,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"finding fix operation is invalid", err)
	}
	if err := fix.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "finding fix is invalid", err)
	}
	if operation.FixID != fix.ID || operation.AcceptanceID != fix.AcceptanceID ||
		operation.FindingID != fix.FindingID || operation.RunID != fix.RunID ||
		operation.DecidedBy != fix.DecidedBy || !operation.CreatedAt.Equal(fix.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"finding fix operation does not match its decision")
	}
	return nil
}

func validateFindingAcceptanceReplay(existing,
	request domain.FindingAcceptanceOperation,
) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.ValidationID != request.ValidationID ||
		existing.FindingID != request.FindingID || existing.RunID != request.RunID ||
		existing.DecidedBy != request.DecidedBy {
		return apperror.New(apperror.CodeConflict,
			"finding acceptance operation key was already used for different intent")
	}
	return nil
}

func validateFindingRemediationEvidenceReplay(existing,
	request domain.FindingRemediationEvidenceOperation,
) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.AcceptanceID != request.AcceptanceID ||
		existing.FindingID != request.FindingID ||
		existing.ArtifactID != request.ArtifactID || existing.RunID != request.RunID ||
		existing.AttachedBy != request.AttachedBy {
		return apperror.New(apperror.CodeConflict,
			"finding remediation Evidence operation key was already used for different intent")
	}
	return nil
}

func validateFindingFixReplay(existing, request domain.FindingFixOperation) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.AcceptanceID != request.AcceptanceID ||
		existing.FindingID != request.FindingID || existing.RunID != request.RunID ||
		existing.DecidedBy != request.DecidedBy {
		return apperror.New(apperror.CodeConflict,
			"finding fix operation key was already used for different intent")
	}
	return nil
}
