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
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

const verificationEvidenceSelect = `SELECT id, protocol_version, operation_key_digest,
	request_fingerprint, run_id, session_id, workspace_id, outcome, title, summary,
	summary_sha256, redacted, recorded_by, event_sequence, created_at
	FROM operator_verification_evidence`

func (s *SQLiteStore) GetVerificationEvidence(ctx context.Context,
	id string,
) (verification.Evidence, error) {
	if id != strings.TrimSpace(id) || !domain.ValidAgentID(id) {
		return verification.Evidence{}, apperror.New(apperror.CodeInvalidArgument,
			"verification evidence identity is invalid")
	}
	value, err := scanVerificationEvidence(s.db.QueryRowContext(ctx,
		verificationEvidenceSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.Evidence{}, apperror.New(apperror.CodeNotFound,
			"verification evidence was not found")
	}
	return value, err
}

func (s *SQLiteStore) GetVerificationEvidenceByOperation(ctx context.Context,
	keyDigest string,
) (verification.Evidence, bool, error) {
	if !validStoreDigest(keyDigest) {
		return verification.Evidence{}, false, apperror.New(apperror.CodeInvalidArgument,
			"verification evidence operation digest is invalid")
	}
	value, err := scanVerificationEvidence(s.db.QueryRowContext(ctx,
		verificationEvidenceSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.Evidence{}, false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) ListVerificationEvidence(ctx context.Context,
	runID string, limit int,
) ([]verification.Evidence, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification evidence Run identity is invalid")
	}
	if limit < 1 || limit > verification.MaxInventoryItems+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification evidence limit is invalid")
	}
	rows, err := s.db.QueryContext(ctx, verificationEvidenceSelect+
		` WHERE run_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]verification.Evidence, 0, limit)
	for rows.Next() {
		value, err := scanVerificationEvidence(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) RecordVerificationEvidence(ctx context.Context,
	evidence verification.Evidence,
) (verification.Evidence, bool, error) {
	prepared := evidence
	prepared.EventSequence = 1
	if evidence.EventSequence != 0 {
		return verification.Evidence{}, false, apperror.New(apperror.CodeInvalidArgument,
			"new verification evidence cannot carry an event sequence")
	}
	if err := prepared.Validate(); err != nil {
		return verification.Evidence{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification evidence is invalid", err)
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return verification.Evidence{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`,
		evidence.RunID)
	if err != nil {
		return verification.Evidence{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return verification.Evidence{}, false, err
	}
	if rows != 1 {
		return verification.Evidence{}, false,
			apperror.New(apperror.CodeNotFound, "verification evidence Run was not found")
	}

	existing, found, err := getVerificationEvidenceTx(ctx, tx, evidence.OperationKeyDigest)
	if err != nil {
		return verification.Evidence{}, false, err
	}
	if found {
		if !sameVerificationEvidenceIntent(existing, evidence) {
			return verification.Evidence{}, false, apperror.New(apperror.CodeConflict,
				"verification evidence operation key was used for different intent")
		}
		if err := tx.Commit(); err != nil {
			return verification.Evidence{}, false, err
		}
		return existing, true, nil
	}

	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, evidence.RunID))
	if err != nil {
		return verification.Evidence{}, false, err
	}
	var workspaceID, sessionWorkspaceID, sessionStatus string
	if err := tx.QueryRowContext(ctx, `SELECT mission.workspace_id, session_record.workspace_id,
		session_record.status
		FROM missions mission JOIN sessions session_record ON session_record.id = ?
		WHERE mission.id = ?`, evidence.SessionID, run.MissionID).Scan(
		&workspaceID, &sessionWorkspaceID, &sessionStatus); err != nil {
		return verification.Evidence{}, false, err
	}
	if run.SessionID != evidence.SessionID || workspaceID != evidence.WorkspaceID ||
		sessionWorkspaceID != evidence.WorkspaceID || sessionStatus != session.StatusActive {
		return verification.Evidence{}, false, apperror.New(apperror.CodeConflict,
			"verification evidence Run, active Session, or Workspace binding changed")
	}

	event, err := events.New(run.ID, run.MissionID,
		events.VerificationEvidenceRecordedEvent, "operator_verification", evidence.ID,
		map[string]any{
			"outcome": evidence.Outcome, "summary_sha256": evidence.SummarySHA256,
			"redacted": evidence.Redacted, "command_executed": false,
			"model_assertion": false, "approval": false, "authority_granted": false,
		})
	if err != nil {
		return verification.Evidence{}, false, err
	}
	event.CreatedAt = evidence.CreatedAt
	event, err = insertRunEventTx(ctx, tx, event)
	if err != nil {
		return verification.Evidence{}, false, err
	}
	evidence.EventSequence = event.Sequence
	if err := evidence.Validate(); err != nil {
		return verification.Evidence{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operator_verification_evidence
		(id, protocol_version, operation_key_digest, request_fingerprint, run_id,
		session_id, workspace_id, outcome, title, summary, summary_sha256, redacted,
		recorded_by, event_sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, evidence.ID,
		evidence.ProtocolVersion, evidence.OperationKeyDigest, evidence.RequestFingerprint,
		evidence.RunID, evidence.SessionID, evidence.WorkspaceID, evidence.Outcome,
		evidence.Title, evidence.Summary, evidence.SummarySHA256, boolInt(evidence.Redacted),
		evidence.RecordedBy, evidence.EventSequence, ts(evidence.CreatedAt)); err != nil {
		return verification.Evidence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return verification.Evidence{}, false, err
	}
	return evidence, false, nil
}

func scanVerificationEvidence(row scanner) (verification.Evidence, error) {
	var value verification.Evidence
	var outcome string
	var redacted int
	var created string
	if err := row.Scan(&value.ID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.RequestFingerprint, &value.RunID, &value.SessionID, &value.WorkspaceID,
		&outcome, &value.Title, &value.Summary, &value.SummarySHA256, &redacted,
		&value.RecordedBy, &value.EventSequence, &created); err != nil {
		return verification.Evidence{}, err
	}
	if redacted != 0 && redacted != 1 {
		return verification.Evidence{}, errors.New("stored verification redaction flag is invalid")
	}
	value.Outcome = verification.Outcome(outcome)
	value.Redacted = redacted == 1
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return verification.Evidence{}, fmt.Errorf("stored verification evidence is invalid: %w", err)
	}
	return value, nil
}

func getVerificationEvidenceTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (verification.Evidence, bool, error) {
	value, err := scanVerificationEvidence(tx.QueryRowContext(ctx,
		verificationEvidenceSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.Evidence{}, false, nil
	}
	return value, err == nil, err
}

func sameVerificationEvidenceIntent(left verification.Evidence,
	right verification.Evidence,
) bool {
	return left.ProtocolVersion == right.ProtocolVersion &&
		left.OperationKeyDigest == right.OperationKeyDigest &&
		left.RequestFingerprint == right.RequestFingerprint && left.RunID == right.RunID &&
		left.SessionID == right.SessionID && left.WorkspaceID == right.WorkspaceID &&
		left.Outcome == right.Outcome && left.Title == right.Title &&
		left.Summary == right.Summary && left.SummarySHA256 == right.SummarySHA256 &&
		left.Redacted == right.Redacted && left.RecordedBy == right.RecordedBy
}
