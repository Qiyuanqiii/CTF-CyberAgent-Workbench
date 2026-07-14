package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
)

const operatorSteeringSelect = `SELECT id, run_id, session_id, sequence, status, content,
	content_sha256, requested_by, session_message_id, created_at, committed_at, cancelled_at
	FROM operator_steering_messages`

func (s *SQLiteStore) EnqueueOperatorSteering(ctx context.Context,
	request domain.EnqueueOperatorSteeringRequest,
) (domain.OperatorSteeringEnqueueResult, error) {
	result, _, err := s.enqueueOperatorSteering(ctx, request, false)
	return result, err
}

func (s *SQLiteStore) EnqueueOperatorSteeringIfBusy(ctx context.Context,
	request domain.EnqueueOperatorSteeringRequest,
) (domain.OperatorSteeringEnqueueResult, bool, error) {
	return s.enqueueOperatorSteering(ctx, request, true)
}

func (s *SQLiteStore) enqueueOperatorSteering(ctx context.Context,
	request domain.EnqueueOperatorSteeringRequest, onlyIfBusy bool,
) (domain.OperatorSteeringEnqueueResult, bool, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	normalized.Content = redact.String(normalized.Content)
	normalized.Content, err = domain.NormalizeOperatorSteeringContent(normalized.Content)
	if err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if redact.String(normalized.RequestedBy) != normalized.RequestedBy {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.New(apperror.CodeInvalidArgument,
				"operator steering requester cannot contain sensitive material")
	}
	contentDigest := domain.OperatorSteeringContentSHA256(normalized.Content)
	keyDigest := runmutation.Fingerprint("operator_steering_operation.v1", normalized.RunID,
		normalized.OperationKey)
	fingerprint := runmutation.Fingerprint("operator_steering_request.v1", normalized.RunID,
		normalized.SessionID, contentDigest, normalized.RequestedBy)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx,
		`UPDATE runs SET updated_at = updated_at WHERE id = ?`, normalized.RunID)
	if err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	if rows != 1 {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.New(apperror.CodeNotFound, "operator steering Run was not found")
	}
	operationFingerprint, messageID, found, err := getOperatorSteeringOperationTx(ctx, tx, keyDigest)
	if err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	if found {
		if operationFingerprint != fingerprint {
			return domain.OperatorSteeringEnqueueResult{}, false,
				apperror.New(apperror.CodeConflict,
					"operator steering operation key was already used for different intent")
		}
		message, err := getOperatorSteeringMessageTx(ctx, tx, messageID)
		if err != nil {
			return domain.OperatorSteeringEnqueueResult{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.OperatorSteeringEnqueueResult{}, false, err
		}
		return domain.OperatorSteeringEnqueueResult{Message: message, Replayed: true}, true, nil
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, normalized.RunID))
	if err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	if run.SessionID != normalized.SessionID {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.New(apperror.CodeConflict, "operator steering Run and Session binding changed")
	}
	if run.Status != domain.RunRunning && run.Status != domain.RunPaused {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.New(apperror.CodeFailedPrecondition,
				fmt.Sprintf("run %s cannot accept operator steering while %s", run.ID, run.Status))
	}
	if onlyIfBusy {
		busy, err := operatorSteeringBusyTx(ctx, tx, run.ID, time.Now().UTC())
		if err != nil {
			return domain.OperatorSteeringEnqueueResult{}, false, err
		}
		if !busy {
			if err := tx.Commit(); err != nil {
				return domain.OperatorSteeringEnqueueResult{}, false, err
			}
			return domain.OperatorSteeringEnqueueResult{}, false, nil
		}
	}
	var pendingCount, pendingBytes int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(length(CAST(content AS BLOB))), 0)
		FROM operator_steering_messages WHERE run_id = ? AND status = 'pending'`, run.ID).
		Scan(&pendingCount, &pendingBytes); err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	if pendingCount >= domain.MaxPendingOperatorSteering ||
		pendingBytes+len([]byte(normalized.Content)) > domain.MaxPendingOperatorSteeringBytes {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.New(apperror.CodeResourceExhausted,
				"operator steering queue reached its pending count or byte limit")
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1
		FROM operator_steering_messages WHERE run_id = ?`, run.ID).Scan(&sequence); err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	now := time.Now().UTC()
	message := domain.OperatorSteeringMessage{
		ID: idgen.New("steer"), RunID: run.ID, SessionID: run.SessionID, Sequence: sequence,
		Status: domain.OperatorSteeringPending, Content: normalized.Content,
		ContentSHA256: contentDigest, RequestedBy: normalized.RequestedBy, CreatedAt: now,
	}
	if err := message.Validate(); err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument, "invalid operator steering message", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operator_steering_messages
		(id, run_id, session_id, sequence, status, content, content_sha256, requested_by,
		 session_message_id, created_at, committed_at, cancelled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, NULL, NULL)`, message.ID, message.RunID,
		message.SessionID, message.Sequence, message.Status, message.Content,
		message.ContentSHA256, message.RequestedBy, ts(message.CreatedAt)); err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operator_steering_operations
		(operation_key_digest, request_fingerprint, message_id, run_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, keyDigest, fingerprint, message.ID, message.RunID,
		message.RequestedBy, ts(message.CreatedAt)); err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.OperatorSteeringQueuedEvent,
		"operator", message.ID, map[string]any{
			"sequence": message.Sequence, "status": message.Status,
			"pending_count": pendingCount + 1,
		}); err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.OperatorSteeringEnqueueResult{}, false, err
	}
	return domain.OperatorSteeringEnqueueResult{Message: message}, true, nil
}

func (s *SQLiteStore) GetOperatorSteering(ctx context.Context,
	id string,
) (domain.OperatorSteeringMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" || len([]rune(id)) > domain.MaxOperatorSteeringIdentityRunes {
		return domain.OperatorSteeringMessage{},
			apperror.New(apperror.CodeInvalidArgument, "operator steering id is required and bounded")
	}
	return getOperatorSteeringMessageRow(s.db.QueryRowContext(ctx,
		operatorSteeringSelect+` WHERE id = ?`, id))
}

func (s *SQLiteStore) ListOperatorSteering(ctx context.Context, runID string,
	limit int,
) ([]domain.OperatorSteeringMessage, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || len([]rune(runID)) > domain.MaxOperatorSteeringIdentityRunes {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"operator steering Run id is required and bounded")
	}
	if limit <= 0 || limit > domain.MaxOperatorSteeringListLimit {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			fmt.Sprintf("operator steering list limit must be between 1 and %d",
				domain.MaxOperatorSteeringListLimit))
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, session_id, sequence, status, content,
		content_sha256, requested_by, session_message_id, created_at, committed_at, cancelled_at
		FROM (SELECT * FROM operator_steering_messages WHERE run_id = ?
			ORDER BY sequence DESC LIMIT ?) ORDER BY sequence`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]domain.OperatorSteeringMessage, 0)
	for rows.Next() {
		value, err := getOperatorSteeringMessageRow(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) GetOperatorSteeringQueueSummary(ctx context.Context,
	runID string,
) (domain.OperatorSteeringQueueSummary, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || len([]rune(runID)) > domain.MaxOperatorSteeringIdentityRunes {
		return domain.OperatorSteeringQueueSummary{},
			apperror.New(apperror.CodeInvalidArgument,
				"operator steering Run id is required and bounded")
	}
	summary := domain.OperatorSteeringQueueSummary{RunID: runID}
	if err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN message.status = 'pending' AND NOT EXISTS
			(SELECT 1 FROM operator_steering_deliveries delivery
			 WHERE delivery.message_id = message.id AND delivery.status = 'prepared') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN message.status = 'pending' AND EXISTS
			(SELECT 1 FROM operator_steering_deliveries delivery
			 WHERE delivery.message_id = message.id AND delivery.status = 'prepared') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN message.status = 'committed' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN message.status = 'cancelled' THEN 1 ELSE 0 END), 0)
		FROM operator_steering_messages message WHERE message.run_id = ?`, runID).
		Scan(&summary.Pending, &summary.Prepared, &summary.Committed, &summary.Cancelled); err != nil {
		return domain.OperatorSteeringQueueSummary{}, err
	}
	next, err := getOperatorSteeringMessageRow(s.db.QueryRowContext(ctx, operatorSteeringSelect+
		` message WHERE run_id = ? AND status = 'pending' AND NOT EXISTS
			(SELECT 1 FROM operator_steering_deliveries delivery
			 WHERE delivery.message_id = message.id AND delivery.status = 'prepared')
		 ORDER BY sequence LIMIT 1`, runID))
	if err == nil {
		summary.Next = &next
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.OperatorSteeringQueueSummary{}, err
	}
	return summary, nil
}

func operatorSteeringBusyTx(ctx context.Context, tx *sql.Tx, runID string,
	now time.Time,
) (bool, error) {
	var busy int
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN
		EXISTS (SELECT 1 FROM run_execution_leases lease WHERE lease.run_id = ?
			AND lease.status = 'active' AND julianday(lease.expires_at) > julianday(?))
		OR EXISTS (SELECT 1 FROM run_supervisor_checkpoints checkpoint WHERE checkpoint.run_id = ?
			AND (checkpoint.phase = 'turn_started'
				OR (checkpoint.phase = 'turn_failed' AND checkpoint.pending_input != '')))
		OR EXISTS (SELECT 1 FROM operator_steering_messages message WHERE message.run_id = ?
			AND message.status = 'pending') THEN 1 ELSE 0 END`, runID, ts(now), runID, runID).
		Scan(&busy); err != nil {
		return false, err
	}
	return busy == 1, nil
}

func selectOperatorSteeringForTurnTx(ctx context.Context, tx *sql.Tx, runID string,
	preferredMessageID string,
) (domain.OperatorSteeringMessage, bool, error) {
	query := operatorSteeringSelect + ` message WHERE run_id = ? AND status = 'pending'
		AND NOT EXISTS (SELECT 1 FROM operator_steering_deliveries delivery
			WHERE delivery.message_id = message.id AND delivery.status = 'prepared')`
	args := []any{runID}
	if preferredMessageID != "" {
		query += ` AND id = ?`
		args = append(args, preferredMessageID)
	}
	query += ` ORDER BY sequence LIMIT 1`
	message, err := getOperatorSteeringMessageRow(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OperatorSteeringMessage{}, false, nil
	}
	return message, err == nil, err
}

func prepareOperatorSteeringDeliveryTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	checkpoint domain.SupervisorCheckpoint, message domain.OperatorSteeringMessage,
	at time.Time,
) (domain.OperatorSteeringDelivery, error) {
	delivery := domain.OperatorSteeringDelivery{
		ID: idgen.New("steer-delivery"), MessageID: message.ID, RunID: run.ID,
		AttemptID: checkpoint.AttemptID, Turn: checkpoint.NextTurn,
		Status: domain.OperatorSteeringDeliveryPrepared, PreparedAt: at,
	}
	if err := delivery.Validate(); err != nil {
		return domain.OperatorSteeringDelivery{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operator_steering_deliveries
		(id, message_id, run_id, attempt_id, turn, status, prepared_at, terminal_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`, delivery.ID, delivery.MessageID, delivery.RunID,
		delivery.AttemptID, delivery.Turn, delivery.Status, ts(delivery.PreparedAt)); err != nil {
		return domain.OperatorSteeringDelivery{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.OperatorSteeringPreparedEvent,
		"run_supervisor", delivery.ID, map[string]any{
			"message_id": message.ID, "sequence": message.Sequence,
			"attempt_id": checkpoint.AttemptID, "turn": checkpoint.NextTurn,
		}); err != nil {
		return domain.OperatorSteeringDelivery{}, err
	}
	return delivery, nil
}

func supersedeOperatorSteeringDeliveryTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	checkpoint domain.SupervisorCheckpoint, at time.Time,
) (string, bool, error) {
	var deliveryID, messageID string
	err := tx.QueryRowContext(ctx, `SELECT id, message_id FROM operator_steering_deliveries
		WHERE run_id = ? AND attempt_id = ? AND status = 'prepared'`, run.ID,
		checkpoint.AttemptID).Scan(&deliveryID, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE operator_steering_deliveries
		SET status = 'superseded', terminal_at = ? WHERE id = ? AND status = 'prepared'`,
		ts(at), deliveryID)
	if err != nil {
		return "", false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		if err != nil {
			return "", false, err
		}
		return "", false, apperror.New(apperror.CodeConflict,
			"operator steering delivery changed before supersession")
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.OperatorSteeringSupersededEvent,
		"run_supervisor", deliveryID, map[string]any{
			"message_id": messageID, "attempt_id": checkpoint.AttemptID,
			"turn": checkpoint.NextTurn,
		}); err != nil {
		return "", false, err
	}
	return messageID, true, nil
}

func commitOperatorSteeringDeliveryTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	checkpoint domain.SupervisorCheckpoint, userMessage session.Message, at time.Time,
) (domain.OperatorSteeringMessage, bool, error) {
	var deliveryID, messageID string
	err := tx.QueryRowContext(ctx, `SELECT id, message_id FROM operator_steering_deliveries
		WHERE run_id = ? AND attempt_id = ? AND status = 'prepared'`, run.ID,
		checkpoint.AttemptID).Scan(&deliveryID, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OperatorSteeringMessage{}, false, nil
	}
	if err != nil {
		return domain.OperatorSteeringMessage{}, false, err
	}
	message, err := getOperatorSteeringMessageTx(ctx, tx, messageID)
	if err != nil {
		return domain.OperatorSteeringMessage{}, false, err
	}
	if message.Status != domain.OperatorSteeringPending || message.Content != checkpoint.PendingInput ||
		message.SessionID != run.SessionID || userMessage.SessionID != run.SessionID ||
		userMessage.Content != message.Content || userMessage.ID <= 0 ||
		userMessage.Provenance.SourceKind != session.SourceOperatorMessage ||
		!userMessage.Provenance.InstructionAuthorized ||
		userMessage.Provenance.ContentSHA256 != message.ContentSHA256 {
		return domain.OperatorSteeringMessage{}, false,
			apperror.New(apperror.CodeConflict,
				"operator steering delivery does not match its Supervisor and Session message")
	}
	result, err := tx.ExecContext(ctx, `UPDATE operator_steering_deliveries
		SET status = 'committed', terminal_at = ? WHERE id = ? AND status = 'prepared'`,
		ts(at), deliveryID)
	if err != nil {
		return domain.OperatorSteeringMessage{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		if err != nil {
			return domain.OperatorSteeringMessage{}, false, err
		}
		return domain.OperatorSteeringMessage{}, false,
			apperror.New(apperror.CodeConflict,
				"operator steering delivery changed before commit")
	}
	result, err = tx.ExecContext(ctx, `UPDATE operator_steering_messages
		SET status = 'committed', session_message_id = ?, committed_at = ?
		WHERE id = ? AND status = 'pending'`, userMessage.ID, ts(at), message.ID)
	if err != nil {
		return domain.OperatorSteeringMessage{}, false, err
	}
	rows, err = result.RowsAffected()
	if err != nil || rows != 1 {
		if err != nil {
			return domain.OperatorSteeringMessage{}, false, err
		}
		return domain.OperatorSteeringMessage{}, false,
			apperror.New(apperror.CodeConflict,
				"operator steering message changed before commit")
	}
	message.Status = domain.OperatorSteeringCommitted
	message.SessionMessageID = userMessage.ID
	message.CommittedAt = &at
	if err := appendSupervisorEventTx(ctx, tx, run, events.OperatorSteeringCommittedEvent,
		"run_supervisor", deliveryID, map[string]any{
			"message_id": message.ID, "sequence": message.Sequence,
			"attempt_id": checkpoint.AttemptID, "turn": checkpoint.NextTurn,
			"session_message_id": userMessage.ID,
		}); err != nil {
		return domain.OperatorSteeringMessage{}, false, err
	}
	return message, true, nil
}

func pendingOperatorSteeringAfterCurrentTx(ctx context.Context, tx *sql.Tx,
	checkpoint domain.SupervisorCheckpoint,
) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_steering_messages message
		WHERE message.run_id = ? AND message.status = 'pending'
			AND message.id != COALESCE((SELECT delivery.message_id
				FROM operator_steering_deliveries delivery
				WHERE delivery.run_id = ? AND delivery.attempt_id = ?
					AND delivery.status = 'prepared' LIMIT 1), '')`,
		checkpoint.RunID, checkpoint.RunID, checkpoint.AttemptID).Scan(&count)
	return count, err
}

func operatorSteeringActionDeferredTx(ctx context.Context, tx *sql.Tx,
	checkpoint domain.SupervisorCheckpoint,
) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_events
		WHERE run_id = ? AND type = ? AND source = 'run_supervisor' AND subject_id = ?`,
		checkpoint.RunID, events.OperatorSteeringActionDeferredEvent,
		checkpoint.AttemptID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func cancelOperatorSteeringTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	source string, reason string, at time.Time,
) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_steering_messages
		WHERE run_id = ? AND status = 'pending'`, run.ID).Scan(&count); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE operator_steering_deliveries
		SET status = 'cancelled', terminal_at = ? WHERE run_id = ? AND status = 'prepared'`,
		ts(at), run.ID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE operator_steering_messages
		SET status = 'cancelled', cancelled_at = ? WHERE run_id = ? AND status = 'pending'`,
		ts(at), run.ID); err != nil {
		return 0, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.OperatorSteeringCancelledEvent,
		source, run.ID, map[string]any{
			"count": count, "reason": redact.String(strings.TrimSpace(reason)),
		}); err != nil {
		return 0, err
	}
	return count, nil
}

func lockRunningRunForSteeringTx(ctx context.Context, tx *sql.Tx, runID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at
		WHERE id = ? AND status = 'running'`, runID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Run stopped before operator steering boundary was acquired")
	}
	return nil
}

func requireNoPendingOperatorSteeringTx(ctx context.Context, tx *sql.Tx,
	runID string,
) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_steering_messages
		WHERE run_id = ? AND status = 'pending'`, runID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("Run has %d pending operator steering message(s)", count))
	}
	return nil
}

func getOperatorSteeringOperationTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (string, string, bool, error) {
	var fingerprint, messageID string
	err := tx.QueryRowContext(ctx, `SELECT request_fingerprint, message_id
		FROM operator_steering_operations WHERE operation_key_digest = ?`, keyDigest).
		Scan(&fingerprint, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return fingerprint, messageID, err == nil, err
}

func getOperatorSteeringMessageTx(ctx context.Context, tx *sql.Tx,
	id string,
) (domain.OperatorSteeringMessage, error) {
	return getOperatorSteeringMessageRow(tx.QueryRowContext(ctx,
		operatorSteeringSelect+` WHERE id = ?`, id))
}

type operatorSteeringRow interface {
	Scan(dest ...any) error
}

func getOperatorSteeringMessageRow(row operatorSteeringRow) (domain.OperatorSteeringMessage, error) {
	var message domain.OperatorSteeringMessage
	var sessionMessageID sql.NullInt64
	var createdAt string
	var committedAt, cancelledAt sql.NullString
	if err := row.Scan(&message.ID, &message.RunID, &message.SessionID, &message.Sequence,
		&message.Status, &message.Content, &message.ContentSHA256, &message.RequestedBy,
		&sessionMessageID, &createdAt, &committedAt, &cancelledAt); err != nil {
		return domain.OperatorSteeringMessage{}, err
	}
	message.SessionMessageID = sessionMessageID.Int64
	message.CreatedAt = parseTS(createdAt)
	message.CommittedAt = parseNullableTS(committedAt)
	message.CancelledAt = parseNullableTS(cancelledAt)
	if err := message.Validate(); err != nil {
		return domain.OperatorSteeringMessage{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"invalid persisted operator steering message", err)
	}
	return message, nil
}
