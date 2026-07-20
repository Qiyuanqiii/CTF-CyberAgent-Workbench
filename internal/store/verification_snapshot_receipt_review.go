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

const verificationSnapshotReceiptReviewSelect = `SELECT id, protocol_version,
	operation_key_digest, request_fingerprint, run_id, session_id, workspace_id,
	receipt_id, receipt_content_sha256, receipt_event_sequence, decision, reviewed_by,
	event_sequence, created_at FROM operator_verification_snapshot_receipt_reviews`

func (s *SQLiteStore) GetVerificationSnapshotReceiptReviewByOperation(ctx context.Context,
	keyDigest string,
) (verification.SnapshotReceiptReview, bool, error) {
	if !validStoreDigest(keyDigest) {
		return verification.SnapshotReceiptReview{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"verification snapshot receipt review operation digest is invalid")
	}
	value, err := scanVerificationSnapshotReceiptReview(s.db.QueryRowContext(ctx,
		verificationSnapshotReceiptReviewSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.SnapshotReceiptReview{}, false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) GetVerificationSnapshotReceiptReviewByReceipt(ctx context.Context,
	receiptID string,
) (verification.SnapshotReceiptReview, bool, error) {
	if receiptID != strings.TrimSpace(receiptID) || !domain.ValidAgentID(receiptID) {
		return verification.SnapshotReceiptReview{}, false, apperror.New(
			apperror.CodeInvalidArgument, "verification snapshot receipt identity is invalid")
	}
	value, err := scanVerificationSnapshotReceiptReview(s.db.QueryRowContext(ctx,
		verificationSnapshotReceiptReviewSelect+` WHERE receipt_id = ?`, receiptID))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.SnapshotReceiptReview{}, false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) ListVerificationSnapshotReceiptReviews(ctx context.Context,
	runID string, limit int,
) ([]verification.SnapshotReceiptReview, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification snapshot receipt review Run identity is invalid")
	}
	if limit < 1 || limit > verification.MaxSnapshotReceiptReviewHistory+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification snapshot receipt review limit is invalid")
	}
	rows, err := s.db.QueryContext(ctx, verificationSnapshotReceiptReviewSelect+
		` WHERE run_id = ? ORDER BY event_sequence DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]verification.SnapshotReceiptReview, 0, limit)
	for rows.Next() {
		value, err := scanVerificationSnapshotReceiptReview(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) RecordVerificationSnapshotReceiptReview(ctx context.Context,
	review verification.SnapshotReceiptReview,
) (verification.SnapshotReceiptReview, bool, error) {
	if review.EventSequence != 0 {
		return verification.SnapshotReceiptReview{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"new verification snapshot receipt review cannot carry an event sequence")
	}
	prepared := review
	prepared.EventSequence = review.ReceiptEventSequence + 1
	if err := prepared.Validate(); err != nil {
		return verification.SnapshotReceiptReview{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification snapshot receipt review is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`,
		review.RunID)
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	if rows != 1 {
		return verification.SnapshotReceiptReview{}, false, apperror.New(apperror.CodeNotFound,
			"verification snapshot receipt review Run was not found")
	}
	existing, found, err := getVerificationSnapshotReceiptReviewByOperationTx(ctx, tx,
		review.OperationKeyDigest)
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	if found {
		if !sameVerificationSnapshotReceiptReviewIntent(existing, review) {
			return verification.SnapshotReceiptReview{}, false, apperror.New(apperror.CodeConflict,
				"verification snapshot receipt review operation key was used for different intent")
		}
		if err := tx.Commit(); err != nil {
			return verification.SnapshotReceiptReview{}, false, err
		}
		return existing, true, nil
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, review.RunID))
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	var workspaceID, sessionWorkspaceID, sessionStatus, surface string
	if err := tx.QueryRowContext(ctx, `SELECT mission.workspace_id, session_record.workspace_id,
		session_record.status, mode.surface
		FROM missions mission JOIN sessions session_record ON session_record.id = ?
		JOIN run_mode_snapshots mode ON mode.run_id = ?
		WHERE mission.id = ? AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
			WHERE later.run_id = mode.run_id AND later.revision > mode.revision)`,
		review.SessionID, run.ID, run.MissionID).Scan(&workspaceID,
		&sessionWorkspaceID, &sessionStatus, &surface); err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	if run.SessionID != review.SessionID || workspaceID != review.WorkspaceID ||
		sessionWorkspaceID != review.WorkspaceID || sessionStatus != session.StatusActive ||
		surface != string(domain.ExecutionSurfaceCode) {
		return verification.SnapshotReceiptReview{}, false, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt review binding changed")
	}
	receipt, err := getVerificationSnapshotReceiptTx(ctx, tx, review.ReceiptID)
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	if receipt.RunID != review.RunID || receipt.SessionID != review.SessionID ||
		receipt.WorkspaceID != review.WorkspaceID ||
		receipt.ContentSHA256 != review.ReceiptContentSHA256 ||
		receipt.EventSequence != review.ReceiptEventSequence ||
		review.CreatedAt.Before(receipt.CreatedAt) {
		return verification.SnapshotReceiptReview{}, false, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt review escaped its exact receipt binding")
	}
	if _, found, err := getVerificationSnapshotReceiptReviewByReceiptTx(ctx, tx,
		review.ReceiptID); err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	} else if found {
		return verification.SnapshotReceiptReview{}, false, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt already has an immutable review")
	}
	event, err := events.New(run.ID, run.MissionID,
		events.VerificationSnapshotReviewRecordedEvent,
		"operator_verification_snapshot_receipt_review", review.ID, map[string]any{
			"receipt_id":             review.ReceiptID,
			"receipt_content_sha256": review.ReceiptContentSHA256,
			"receipt_event_sequence": review.ReceiptEventSequence,
			"decision":               review.Decision, "operator_reviewed": true,
			"metadata_only": true, "review_non_authorizing": true,
			"snapshot_accepted": false, "result_accepted": false,
			"result_inferred": false, "private_bodies_included": false,
			"operator_identity_included": false, "record_rewritten": false,
			"approval": false, "authority_granted": false, "execution_started": false,
		})
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	event.CreatedAt = review.CreatedAt
	event, err = insertRunEventTx(ctx, tx, event)
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	review.EventSequence = event.Sequence
	if err := review.Validate(); err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operator_verification_snapshot_receipt_reviews
		(id, protocol_version, operation_key_digest, request_fingerprint, run_id, session_id,
		workspace_id, receipt_id, receipt_content_sha256, receipt_event_sequence, decision,
		reviewed_by, event_sequence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		review.ID, review.ProtocolVersion, review.OperationKeyDigest, review.RequestFingerprint,
		review.RunID, review.SessionID, review.WorkspaceID, review.ReceiptID,
		review.ReceiptContentSHA256, review.ReceiptEventSequence, review.Decision,
		review.ReviewedBy, review.EventSequence, ts(review.CreatedAt))
	if err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return verification.SnapshotReceiptReview{}, false, err
	}
	return review, false, nil
}

func scanVerificationSnapshotReceiptReview(row scanner) (
	verification.SnapshotReceiptReview, error,
) {
	var value verification.SnapshotReceiptReview
	var created string
	if err := row.Scan(&value.ID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.RequestFingerprint, &value.RunID, &value.SessionID, &value.WorkspaceID,
		&value.ReceiptID, &value.ReceiptContentSHA256, &value.ReceiptEventSequence,
		&value.Decision, &value.ReviewedBy, &value.EventSequence, &created); err != nil {
		return verification.SnapshotReceiptReview{}, err
	}
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return verification.SnapshotReceiptReview{}, fmt.Errorf(
			"stored verification snapshot receipt review is invalid: %w", err)
	}
	return value, nil
}

func getVerificationSnapshotReceiptReviewByOperationTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (verification.SnapshotReceiptReview, bool, error) {
	value, err := scanVerificationSnapshotReceiptReview(tx.QueryRowContext(ctx,
		verificationSnapshotReceiptReviewSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.SnapshotReceiptReview{}, false, nil
	}
	return value, err == nil, err
}

func getVerificationSnapshotReceiptReviewByReceiptTx(ctx context.Context, tx *sql.Tx,
	receiptID string,
) (verification.SnapshotReceiptReview, bool, error) {
	value, err := scanVerificationSnapshotReceiptReview(tx.QueryRowContext(ctx,
		verificationSnapshotReceiptReviewSelect+` WHERE receipt_id = ?`, receiptID))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.SnapshotReceiptReview{}, false, nil
	}
	return value, err == nil, err
}

func sameVerificationSnapshotReceiptReviewIntent(left verification.SnapshotReceiptReview,
	right verification.SnapshotReceiptReview,
) bool {
	return left.ProtocolVersion == right.ProtocolVersion &&
		left.OperationKeyDigest == right.OperationKeyDigest &&
		left.RequestFingerprint == right.RequestFingerprint && left.RunID == right.RunID &&
		left.SessionID == right.SessionID && left.WorkspaceID == right.WorkspaceID &&
		left.ReceiptID == right.ReceiptID &&
		left.ReceiptContentSHA256 == right.ReceiptContentSHA256 &&
		left.ReceiptEventSequence == right.ReceiptEventSequence &&
		left.Decision == right.Decision && left.ReviewedBy == right.ReviewedBy
}
