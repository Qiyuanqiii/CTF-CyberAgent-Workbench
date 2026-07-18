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
)

func (s *SQLiteStore) GetRunExecutionHandoff(ctx context.Context,
	keyDigest string,
) (domain.RunExecutionHandoff, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return domain.RunExecutionHandoff{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Run execution handoff digest is invalid")
	}
	return getRunExecutionHandoffByKey(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) PrepareRunExecutionHandoff(ctx context.Context,
	operation domain.RunExecutionHandoffOperation,
) (domain.RunExecutionHandoff, bool, error) {
	probe := operation
	probe.EventSequence = 1
	probe.SelectedCount = 0
	if operation.EventSequence != 0 || operation.SelectedCount != 0 ||
		probe.Validate() != nil {
		return domain.RunExecutionHandoff{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Run execution handoff intent is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, found, err := getRunExecutionHandoffByKey(ctx, tx,
		operation.KeyDigest); err != nil {
		return domain.RunExecutionHandoff{}, false, err
	} else if found {
		if err := sameRunExecutionHandoffIntent(existing.Operation, operation); err != nil {
			return domain.RunExecutionHandoff{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.RunExecutionHandoff{}, false, err
		}
		return existing, true, nil
	}
	if err := lockRunControlTx(ctx, tx, operation.RunID); err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	run, err := getRunControlRunTx(ctx, tx, operation.RunID)
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	if run.Status != domain.RunRunning || run.SessionID != operation.SessionID {
		return domain.RunExecutionHandoff{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"Run execution handoff requires the exact running Run and Session")
	}
	if err := requireNoActiveRunControlLeaseTx(ctx, tx, run.ID,
		operation.CreatedAt); err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	items, err := selectRunExecutionHandoffItems(ctx, tx, operation.ID,
		run.ID, operation.MaxSteps)
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	operation.SelectedCount = len(items)
	event, err := newRunExecutionHandoffEvent(run,
		events.RunExecutionHandoffRequestedEvent, operation.ID, operation.CreatedAt,
		map[string]any{"max_steps": operation.MaxSteps,
			"selected_count": operation.SelectedCount})
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	storedEvent, err := insertRunEventTx(ctx, tx, event)
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	operation.EventSequence = storedEvent.Sequence
	if err := operation.Validate(); err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_execution_handoff_operations
		(id, operation_key_digest, request_fingerprint, protocol_version, run_id,
		session_id, requested_by, max_steps, selected_count, event_sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operation.ID, operation.KeyDigest,
		operation.RequestFingerprint, operation.ProtocolVersion, operation.RunID,
		operation.SessionID, operation.RequestedBy, operation.MaxSteps,
		operation.SelectedCount, operation.EventSequence, ts(operation.CreatedAt)); err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO run_execution_handoff_items
			(operation_id, ordinal, message_id, message_sequence, prepared)
			VALUES (?, ?, ?, ?, ?)`, item.OperationID, item.Ordinal, item.MessageID,
			item.MessageSequence, boolInt(item.Prepared)); err != nil {
			return domain.RunExecutionHandoff{}, false, err
		}
	}
	handoff := domain.RunExecutionHandoff{Operation: operation, Items: items}
	if len(items) == 0 {
		result, err := completeRunExecutionHandoffTx(ctx, tx, handoff, run,
			domain.RunExecutionHandoffCompleted, "queue_empty", "", 0, false, false,
			domain.RunExecutionLease{}, operation.CreatedAt)
		if err != nil {
			return domain.RunExecutionHandoff{}, false, err
		}
		handoff.Result = &result
	}
	if err := handoff.Validate(); err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	return handoff, false, nil
}

func (s *SQLiteStore) CompleteRunExecutionHandoff(ctx context.Context,
	operationID string, lease domain.RunExecutionLease,
	status domain.RunExecutionHandoffStatus, stopReason string, errorCode string,
	stepsCompleted int, modelCalled bool, toolCalled bool,
) (domain.RunExecutionHandoffResult, bool, error) {
	operationID = strings.TrimSpace(operationID)
	if !domain.ValidAgentID(operationID) || !status.Valid() {
		return domain.RunExecutionHandoffResult{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Run execution handoff completion is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	handoff, found, err := getRunExecutionHandoffByID(ctx, tx, operationID)
	if err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	if !found {
		return domain.RunExecutionHandoffResult{}, false,
			apperror.New(apperror.CodeNotFound, "Run execution handoff was not found")
	}
	if handoff.Result != nil {
		if lease.RunID != handoff.Operation.RunID ||
			lease.LeaseID != handoff.Result.LeaseID ||
			lease.Generation != handoff.Result.LeaseGeneration ||
			handoff.Result.Status != status || handoff.Result.StopReason != stopReason ||
			handoff.Result.ErrorCode != errorCode ||
			handoff.Result.StepsCompleted != stepsCompleted ||
			handoff.Result.ModelCalled != modelCalled || handoff.Result.ToolCalled != toolCalled {
			return domain.RunExecutionHandoffResult{}, false, apperror.New(
				apperror.CodeConflict,
				"Run execution handoff completion replay intent changed")
		}
		if err := tx.Commit(); err != nil {
			return domain.RunExecutionHandoffResult{}, false, err
		}
		return *handoff.Result, true, nil
	}
	if handoff.Operation.SelectedCount == 0 || lease.RunID != handoff.Operation.RunID {
		return domain.RunExecutionHandoffResult{}, false, apperror.New(
			apperror.CodeConflict, "Run execution handoff lease binding is invalid")
	}
	if err := requireRunExecutionLeaseTx(ctx, tx, lease.RunID, lease.LeaseID,
		lease.Generation); err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	run, err := getRunControlRunTx(ctx, tx, handoff.Operation.RunID)
	if err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	result, err := completeRunExecutionHandoffTx(ctx, tx, handoff, run, status,
		stopReason, errorCode, stepsCompleted, modelCalled, toolCalled, lease,
		time.Now().UTC())
	if err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	return result, false, nil
}

func completeRunExecutionHandoffTx(ctx context.Context, tx *sql.Tx,
	handoff domain.RunExecutionHandoff, run domain.Run,
	status domain.RunExecutionHandoffStatus, stopReason string, errorCode string,
	stepsCompleted int, modelCalled bool, toolCalled bool,
	lease domain.RunExecutionLease, completedAt time.Time,
) (domain.RunExecutionHandoffResult, error) {
	pending, prepared, committed, cancelled, err :=
		classifyRunExecutionHandoffItems(ctx, tx, handoff.Operation.ID)
	if err != nil {
		return domain.RunExecutionHandoffResult{}, err
	}
	event, err := newRunExecutionHandoffEvent(run,
		events.RunExecutionHandoffCompletedEvent, handoff.Operation.ID, completedAt,
		map[string]any{"status": status, "stop_reason": stopReason,
			"error_code": errorCode, "steps_completed": stepsCompleted,
			"selected_count": handoff.Operation.SelectedCount,
			"pending_count":  pending, "prepared_count": prepared,
			"committed_count": committed, "cancelled_count": cancelled,
			"model_called": modelCalled, "tool_called": toolCalled})
	if err != nil {
		return domain.RunExecutionHandoffResult{}, err
	}
	storedEvent, err := insertRunEventTx(ctx, tx, event)
	if err != nil {
		return domain.RunExecutionHandoffResult{}, err
	}
	result := domain.RunExecutionHandoffResult{
		OperationID: handoff.Operation.ID, Status: status, RunStatus: run.Status,
		StopReason: stopReason, ErrorCode: errorCode, StepsCompleted: stepsCompleted,
		ModelCalled: modelCalled, ToolCalled: toolCalled,
		PendingCount: pending, PreparedCount: prepared, CommittedCount: committed,
		CancelledCount: cancelled, CompletionEventSequence: storedEvent.Sequence,
		LeaseID: lease.LeaseID, LeaseGeneration: lease.Generation,
		CompletedAt: completedAt.UTC(),
	}
	if err := result.Validate(handoff.Operation.SelectedCount); err != nil {
		return domain.RunExecutionHandoffResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_execution_handoff_results
		(operation_id, status, run_status, stop_reason, error_code, steps_completed,
		model_called, tool_called, pending_count, prepared_count, committed_count, cancelled_count,
		completion_event_sequence, lease_id, lease_generation, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, result.OperationID,
		result.Status, result.RunStatus, result.StopReason, result.ErrorCode,
		result.StepsCompleted, boolInt(result.ModelCalled), boolInt(result.ToolCalled),
		result.PendingCount, result.PreparedCount,
		result.CommittedCount, result.CancelledCount, result.CompletionEventSequence,
		result.LeaseID, result.LeaseGeneration, ts(result.CompletedAt)); err != nil {
		return domain.RunExecutionHandoffResult{}, err
	}
	return result, nil
}

func selectRunExecutionHandoffItems(ctx context.Context, tx *sql.Tx,
	operationID string, runID string, limit int,
) ([]domain.RunExecutionHandoffItem, error) {
	rows, err := tx.QueryContext(ctx, `SELECT message.id, message.sequence,
		EXISTS (SELECT 1 FROM operator_steering_deliveries delivery
			WHERE delivery.message_id = message.id AND delivery.status = 'prepared')
		FROM operator_steering_messages message
		WHERE message.run_id = ? AND message.status = 'pending'
		ORDER BY message.sequence LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.RunExecutionHandoffItem, 0, limit)
	for rows.Next() {
		var item domain.RunExecutionHandoffItem
		var prepared int
		if err := rows.Scan(&item.MessageID, &item.MessageSequence, &prepared); err != nil {
			return nil, err
		}
		item.OperationID = operationID
		item.Ordinal = len(items) + 1
		item.Prepared = prepared != 0
		if err := item.Validate(); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func classifyRunExecutionHandoffItems(ctx context.Context, tx *sql.Tx,
	operationID string,
) (int, int, int, int, error) {
	var pending, prepared, committed, cancelled int
	err := tx.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN message.status = 'pending' AND NOT EXISTS (
			SELECT 1 FROM operator_steering_deliveries delivery
			WHERE delivery.message_id = message.id AND delivery.status = 'prepared') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN message.status = 'pending' AND EXISTS (
			SELECT 1 FROM operator_steering_deliveries delivery
			WHERE delivery.message_id = message.id AND delivery.status = 'prepared') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN message.status = 'committed' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN message.status = 'cancelled' THEN 1 ELSE 0 END), 0)
		FROM run_execution_handoff_items item
		JOIN operator_steering_messages message ON message.id = item.message_id
		WHERE item.operation_id = ?`, operationID).Scan(&pending, &prepared,
		&committed, &cancelled)
	return pending, prepared, committed, cancelled, err
}

func getRunExecutionHandoffByKey(ctx context.Context, queryer runControlQueryer,
	keyDigest string,
) (domain.RunExecutionHandoff, bool, error) {
	return getRunExecutionHandoff(ctx, queryer,
		`WHERE operation.operation_key_digest = ?`, keyDigest)
}

func getRunExecutionHandoffByID(ctx context.Context, queryer runControlQueryer,
	operationID string,
) (domain.RunExecutionHandoff, bool, error) {
	return getRunExecutionHandoff(ctx, queryer, `WHERE operation.id = ?`, operationID)
}

func getRunExecutionHandoff(ctx context.Context, queryer runControlQueryer,
	where string, value string,
) (domain.RunExecutionHandoff, bool, error) {
	var handoff domain.RunExecutionHandoff
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation.id, operation.protocol_version,
		operation.operation_key_digest, operation.request_fingerprint, operation.run_id,
		operation.session_id, operation.requested_by, operation.max_steps,
		operation.selected_count, operation.event_sequence, operation.created_at
		FROM run_execution_handoff_operations operation `+where, value).Scan(
		&handoff.Operation.ID, &handoff.Operation.ProtocolVersion,
		&handoff.Operation.KeyDigest, &handoff.Operation.RequestFingerprint,
		&handoff.Operation.RunID, &handoff.Operation.SessionID,
		&handoff.Operation.RequestedBy, &handoff.Operation.MaxSteps,
		&handoff.Operation.SelectedCount, &handoff.Operation.EventSequence, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunExecutionHandoff{}, false, nil
	}
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	handoff.Operation.CreatedAt = parseTS(createdAt)
	items, err := listRunExecutionHandoffItems(ctx, queryer, handoff.Operation.ID)
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	handoff.Items = items
	result, found, err := getRunExecutionHandoffResult(ctx, queryer,
		handoff.Operation.ID, handoff.Operation.SelectedCount)
	if err != nil {
		return domain.RunExecutionHandoff{}, false, err
	}
	if found {
		handoff.Result = &result
	}
	if err := handoff.Validate(); err != nil {
		return domain.RunExecutionHandoff{}, false,
			fmt.Errorf("validate stored Run execution handoff: %w", err)
	}
	return handoff, true, nil
}

func listRunExecutionHandoffItems(ctx context.Context, queryer runControlQueryer,
	operationID string,
) ([]domain.RunExecutionHandoffItem, error) {
	rowsQueryer, ok := queryer.(interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	})
	if !ok {
		return nil, errors.New("run execution handoff queryer cannot list items")
	}
	rows, err := rowsQueryer.QueryContext(ctx, `SELECT operation_id, ordinal, message_id,
		message_sequence, prepared FROM run_execution_handoff_items
		WHERE operation_id = ? ORDER BY ordinal`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.RunExecutionHandoffItem, 0)
	for rows.Next() {
		var item domain.RunExecutionHandoffItem
		var prepared int
		if err := rows.Scan(&item.OperationID, &item.Ordinal, &item.MessageID,
			&item.MessageSequence, &prepared); err != nil {
			return nil, err
		}
		item.Prepared = prepared != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func getRunExecutionHandoffResult(ctx context.Context, queryer runControlQueryer,
	operationID string, selectedCount int,
) (domain.RunExecutionHandoffResult, bool, error) {
	var result domain.RunExecutionHandoffResult
	var completedAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_id, status, run_status,
		stop_reason, error_code, steps_completed, model_called, tool_called,
		pending_count, prepared_count,
		committed_count, cancelled_count, completion_event_sequence, lease_id,
		lease_generation, completed_at FROM run_execution_handoff_results
		WHERE operation_id = ?`, operationID).Scan(&result.OperationID, &result.Status,
		&result.RunStatus, &result.StopReason, &result.ErrorCode,
		&result.StepsCompleted, &result.ModelCalled, &result.ToolCalled,
		&result.PendingCount, &result.PreparedCount,
		&result.CommittedCount, &result.CancelledCount,
		&result.CompletionEventSequence, &result.LeaseID, &result.LeaseGeneration,
		&completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunExecutionHandoffResult{}, false, nil
	}
	if err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	result.CompletedAt = parseTS(completedAt)
	if err := result.Validate(selectedCount); err != nil {
		return domain.RunExecutionHandoffResult{}, false, err
	}
	return result, true, nil
}

func sameRunExecutionHandoffIntent(existing domain.RunExecutionHandoffOperation,
	requested domain.RunExecutionHandoffOperation,
) error {
	if existing.ProtocolVersion != requested.ProtocolVersion ||
		existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.RunID != requested.RunID || existing.SessionID != requested.SessionID ||
		existing.RequestedBy != requested.RequestedBy ||
		existing.MaxSteps != requested.MaxSteps {
		return apperror.New(apperror.CodeConflict,
			"Run execution handoff idempotency key was already used for different intent")
	}
	return nil
}

func newRunExecutionHandoffEvent(run domain.Run, eventType string, subjectID string,
	at time.Time, payload any,
) (events.Event, error) {
	event, err := events.New(run.ID, run.MissionID, eventType,
		"run_execution_handoff", subjectID, payload)
	if err != nil {
		return events.Event{}, err
	}
	event.CreatedAt = at.UTC()
	return event, nil
}
