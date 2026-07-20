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

const verificationSnapshotReceiptSelect = `SELECT id, protocol_version, operation_key_digest,
	request_fingerprint, run_id, session_id, workspace_id, plan_id, plan_sha256,
	plan_item_ordinal, plan_item_sha256, format, snapshot_high_water_event_sequence,
	associated_evidence_count, pass_count, fail_count, unknown_count,
	returned_association_count, associations_truncated, content_sha256, content_bytes,
	recorded_by, event_sequence, created_at
	FROM operator_verification_snapshot_receipts`

func (s *SQLiteStore) GetVerificationSnapshotReceiptByOperation(ctx context.Context,
	keyDigest string,
) (verification.SnapshotReceipt, bool, error) {
	if !validStoreDigest(keyDigest) {
		return verification.SnapshotReceipt{}, false, apperror.New(
			apperror.CodeInvalidArgument, "verification snapshot receipt operation digest is invalid")
	}
	value, err := scanVerificationSnapshotReceipt(s.db.QueryRowContext(ctx,
		verificationSnapshotReceiptSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.SnapshotReceipt{}, false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) ListVerificationSnapshotReceipts(ctx context.Context,
	runID string, limit int,
) ([]verification.SnapshotReceipt, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification snapshot receipt Run identity is invalid")
	}
	if limit < 1 || limit > verification.MaxSnapshotReceiptHistory+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification snapshot receipt limit is invalid")
	}
	rows, err := s.db.QueryContext(ctx, verificationSnapshotReceiptSelect+
		` WHERE run_id = ? ORDER BY event_sequence DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]verification.SnapshotReceipt, 0, limit)
	for rows.Next() {
		value, err := scanVerificationSnapshotReceipt(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) RecordVerificationSnapshotReceipt(ctx context.Context,
	receipt verification.SnapshotReceipt,
) (verification.SnapshotReceipt, bool, error) {
	if receipt.EventSequence != 0 {
		return verification.SnapshotReceipt{}, false, apperror.New(apperror.CodeInvalidArgument,
			"new verification snapshot receipt cannot carry an event sequence")
	}
	prepared := receipt
	prepared.EventSequence = receipt.SnapshotHighWaterEventSequence + 1
	if err := prepared.Validate(); err != nil {
		return verification.SnapshotReceipt{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification snapshot receipt is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`,
		receipt.RunID)
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	if rows != 1 {
		return verification.SnapshotReceipt{}, false, apperror.New(apperror.CodeNotFound,
			"verification snapshot receipt Run was not found")
	}
	existing, found, err := getVerificationSnapshotReceiptByOperationTx(ctx, tx,
		receipt.OperationKeyDigest)
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	if found {
		if !sameVerificationSnapshotReceiptIntent(existing, receipt) {
			return verification.SnapshotReceipt{}, false, apperror.New(apperror.CodeConflict,
				"verification snapshot receipt operation key was used for different intent")
		}
		if err := tx.Commit(); err != nil {
			return verification.SnapshotReceipt{}, false, err
		}
		return existing, true, nil
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, receipt.RunID))
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	var workspaceID, sessionWorkspaceID, sessionStatus, surface string
	if err := tx.QueryRowContext(ctx, `SELECT mission.workspace_id, session_record.workspace_id,
		session_record.status, mode.surface
		FROM missions mission JOIN sessions session_record ON session_record.id = ?
		JOIN run_mode_snapshots mode ON mode.run_id = ?
		WHERE mission.id = ? AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
			WHERE later.run_id = mode.run_id AND later.revision > mode.revision)`,
		receipt.SessionID, run.ID, run.MissionID).Scan(&workspaceID,
		&sessionWorkspaceID, &sessionStatus, &surface); err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	if run.SessionID != receipt.SessionID || workspaceID != receipt.WorkspaceID ||
		sessionWorkspaceID != receipt.WorkspaceID || sessionStatus != session.StatusActive ||
		surface != string(domain.ExecutionSurfaceCode) {
		return verification.SnapshotReceipt{}, false, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt Run, active Code Session, or Workspace binding changed")
	}
	plan, err := getVerificationPlanByIDTx(ctx, tx, receipt.PlanID)
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	if plan.RunID != receipt.RunID || plan.SessionID != receipt.SessionID ||
		plan.WorkspaceID != receipt.WorkspaceID || plan.PlanSHA256 != receipt.PlanSHA256 ||
		receipt.PlanItemOrdinal > len(plan.Items) ||
		plan.Items[receipt.PlanItemOrdinal-1].ItemSHA256 != receipt.PlanItemSHA256 ||
		receipt.CreatedAt.Before(plan.CreatedAt) {
		return verification.SnapshotReceipt{}, false, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt plan or item binding changed")
	}
	var associated, pass, fail, unknown int
	var highWater int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),
		COUNT(CASE WHEN evidence_outcome = 'pass' THEN 1 END),
		COUNT(CASE WHEN evidence_outcome = 'fail' THEN 1 END),
		COUNT(CASE WHEN evidence_outcome = 'unknown' THEN 1 END),
		COALESCE(MAX(event_sequence), 0)
		FROM operator_verification_plan_evidence_associations
		WHERE run_id = ? AND plan_id = ? AND plan_item_ordinal = ?`, receipt.RunID,
		receipt.PlanID, receipt.PlanItemOrdinal).Scan(&associated, &pass, &fail, &unknown,
		&highWater); err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	returned := associated
	if returned > verification.MaxCoverageAssociations {
		returned = verification.MaxCoverageAssociations
	}
	if highWater != receipt.SnapshotHighWaterEventSequence ||
		associated != receipt.AssociatedEvidenceCount || pass != receipt.PassCount ||
		fail != receipt.FailCount || unknown != receipt.UnknownCount ||
		returned != receipt.ReturnedAssociationCount ||
		(associated > verification.MaxCoverageAssociations) != receipt.AssociationsTruncated {
		return verification.SnapshotReceipt{}, false, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt no longer matches the current frozen snapshot")
	}
	event, err := events.New(run.ID, run.MissionID,
		events.VerificationSnapshotReceiptRecordedEvent,
		"operator_verification_snapshot_receipt", receipt.ID, map[string]any{
			"plan_id": receipt.PlanID, "plan_sha256": receipt.PlanSHA256,
			"plan_item_ordinal": receipt.PlanItemOrdinal,
			"plan_item_sha256":  receipt.PlanItemSHA256, "format": receipt.Format,
			"snapshot_high_water_event_sequence": receipt.SnapshotHighWaterEventSequence,
			"associated_evidence_count":          receipt.AssociatedEvidenceCount,
			"pass_count":                         receipt.PassCount, "fail_count": receipt.FailCount,
			"unknown_count":              receipt.UnknownCount,
			"returned_association_count": receipt.ReturnedAssociationCount,
			"associations_truncated":     receipt.AssociationsTruncated,
			"content_sha256":             receipt.ContentSHA256, "content_bytes": receipt.ContentBytes,
			"operator_recorded": true, "metadata_only": true,
			"snapshot_accepted": false, "result_accepted": false,
			"result_inferred": false, "private_bodies_included": false,
			"operator_identity_included": false, "record_rewritten": false,
			"approval": false, "authority_granted": false, "execution_started": false,
		})
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	event.CreatedAt = receipt.CreatedAt
	event, err = insertRunEventTx(ctx, tx, event)
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	receipt.EventSequence = event.Sequence
	if err := receipt.Validate(); err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operator_verification_snapshot_receipts
		(id, protocol_version, operation_key_digest, request_fingerprint, run_id, session_id,
		workspace_id, plan_id, plan_sha256, plan_item_ordinal, plan_item_sha256, format,
		snapshot_high_water_event_sequence, associated_evidence_count, pass_count, fail_count,
		unknown_count, returned_association_count, associations_truncated, content_sha256,
		content_bytes, recorded_by, event_sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		receipt.ID, receipt.ProtocolVersion, receipt.OperationKeyDigest,
		receipt.RequestFingerprint, receipt.RunID, receipt.SessionID, receipt.WorkspaceID,
		receipt.PlanID, receipt.PlanSHA256, receipt.PlanItemOrdinal, receipt.PlanItemSHA256,
		receipt.Format, receipt.SnapshotHighWaterEventSequence, receipt.AssociatedEvidenceCount,
		receipt.PassCount, receipt.FailCount, receipt.UnknownCount,
		receipt.ReturnedAssociationCount, receipt.AssociationsTruncated, receipt.ContentSHA256,
		receipt.ContentBytes, receipt.RecordedBy, receipt.EventSequence, ts(receipt.CreatedAt))
	if err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return verification.SnapshotReceipt{}, false, err
	}
	return receipt, false, nil
}

func scanVerificationSnapshotReceipt(row scanner) (verification.SnapshotReceipt, error) {
	var value verification.SnapshotReceipt
	var created string
	if err := row.Scan(&value.ID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.RequestFingerprint, &value.RunID, &value.SessionID, &value.WorkspaceID,
		&value.PlanID, &value.PlanSHA256, &value.PlanItemOrdinal, &value.PlanItemSHA256,
		&value.Format, &value.SnapshotHighWaterEventSequence, &value.AssociatedEvidenceCount,
		&value.PassCount, &value.FailCount, &value.UnknownCount,
		&value.ReturnedAssociationCount, &value.AssociationsTruncated, &value.ContentSHA256,
		&value.ContentBytes, &value.RecordedBy, &value.EventSequence, &created); err != nil {
		return verification.SnapshotReceipt{}, err
	}
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return verification.SnapshotReceipt{}, fmt.Errorf(
			"stored verification snapshot receipt is invalid: %w", err)
	}
	return value, nil
}

func getVerificationSnapshotReceiptByOperationTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (verification.SnapshotReceipt, bool, error) {
	value, err := scanVerificationSnapshotReceipt(tx.QueryRowContext(ctx,
		verificationSnapshotReceiptSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.SnapshotReceipt{}, false, nil
	}
	return value, err == nil, err
}

func sameVerificationSnapshotReceiptIntent(left verification.SnapshotReceipt,
	right verification.SnapshotReceipt,
) bool {
	return left.ProtocolVersion == right.ProtocolVersion &&
		left.OperationKeyDigest == right.OperationKeyDigest &&
		left.RequestFingerprint == right.RequestFingerprint && left.RunID == right.RunID &&
		left.SessionID == right.SessionID && left.WorkspaceID == right.WorkspaceID &&
		left.PlanID == right.PlanID && left.PlanSHA256 == right.PlanSHA256 &&
		left.PlanItemOrdinal == right.PlanItemOrdinal &&
		left.PlanItemSHA256 == right.PlanItemSHA256 && left.Format == right.Format &&
		left.SnapshotHighWaterEventSequence == right.SnapshotHighWaterEventSequence &&
		left.AssociatedEvidenceCount == right.AssociatedEvidenceCount &&
		left.PassCount == right.PassCount && left.FailCount == right.FailCount &&
		left.UnknownCount == right.UnknownCount &&
		left.ReturnedAssociationCount == right.ReturnedAssociationCount &&
		left.AssociationsTruncated == right.AssociationsTruncated &&
		left.ContentSHA256 == right.ContentSHA256 && left.ContentBytes == right.ContentBytes &&
		left.RecordedBy == right.RecordedBy
}
