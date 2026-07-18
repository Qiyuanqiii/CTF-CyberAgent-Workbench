package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

const runWakeConsumptionSelect = `SELECT id, protocol_version, intent_id, run_id,
	session_id, lease_id, generation, owner_id, handoff_operation_key_digest,
	max_steps, status, handoff_operation_id, stop_reason, error_code,
	prepared_event_sequence, completion_event_sequence, created_at, completed_at
	FROM run_wake_consumptions `

func (s *SQLiteStore) GetRunWakeConsumption(ctx context.Context, intentID string,
	generation int,
) (domain.RunWakeConsumption, bool, error) {
	value, err := scanRunWakeConsumption(s.db.QueryRowContext(ctx,
		runWakeConsumptionSelect+`WHERE intent_id = ? AND generation = ?`,
		intentID, generation))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunWakeConsumption{}, false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) PrepareRunWakeConsumption(ctx context.Context,
	value domain.RunWakeConsumption,
) (domain.RunWakeConsumption, bool, error) {
	if value.ProtocolVersion != domain.RunWakeConsumptionProtocolVersion ||
		value.Status != domain.RunWakeConsumptionPrepared || value.Generation < 1 ||
		value.Generation > domain.MaxRunWakeAttempts || value.MaxSteps < 1 ||
		value.MaxSteps > domain.MaxRunExecutionHandoffSteps || value.PreparedEventSequence != 0 ||
		value.CompletionEventSequence != 0 || value.CompletedAt != nil ||
		value.HandoffOperationID != "" || value.StopReason != "" || value.ErrorCode != "" ||
		value.CreatedAt.IsZero() {
		return domain.RunWakeConsumption{}, false,
			errors.New("run wake consumption preparation is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunWakeConsumption{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, found, err := getRunWakeConsumptionTx(ctx, tx, value.IntentID,
		value.Generation); err != nil {
		return domain.RunWakeConsumption{}, false, err
	} else if found {
		if !sameRunWakeConsumptionIntent(existing, value) {
			return domain.RunWakeConsumption{}, false,
				errors.New("run wake generation was already bound to a different handoff")
		}
		return existing, true, nil
	}
	intent, err := getRunWakeIntentTx(ctx, tx, value.IntentID)
	if err != nil {
		return domain.RunWakeConsumption{}, false, err
	}
	lease, err := getRunWakeLeaseTx(ctx, tx, value.LeaseID)
	if err != nil {
		return domain.RunWakeConsumption{}, false, err
	}
	if intent.RunID != value.RunID || intent.SessionID != value.SessionID ||
		intent.Status != domain.RunWakeLeased || intent.AttemptCount != value.Generation ||
		intent.ActiveLeaseID != value.LeaseID || lease.IntentID != value.IntentID ||
		lease.Generation != value.Generation || lease.OwnerID != value.OwnerID ||
		lease.Status != domain.RunWakeLeaseActive || value.CreatedAt.After(lease.ExpiresAt) {
		return domain.RunWakeConsumption{}, false,
			errors.New("run wake consumption lease binding is stale")
	}
	event, err := appendRunWakeEventTx(ctx, tx, value.RunID,
		events.RunWakeHandoffPreparedEvent, "run_wake_consumer", value.ID, map[string]any{
			"intent_id": value.IntentID, "generation": value.Generation,
			"max_steps":          value.MaxSteps,
			"handoff_key_digest": value.HandoffOperationKeyDigest,
			"execution_started":  false, "model_called": false, "tool_called": false,
		}, value.CreatedAt)
	if err != nil {
		return domain.RunWakeConsumption{}, false, err
	}
	value.PreparedEventSequence = event.Sequence
	if err := value.Validate(); err != nil {
		return domain.RunWakeConsumption{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_wake_consumptions
		(id, protocol_version, intent_id, run_id, session_id, lease_id, generation,
		 owner_id, handoff_operation_key_digest, max_steps, status,
		 handoff_operation_id, stop_reason, error_code, prepared_event_sequence,
		 completion_event_sequence, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'prepared', NULL, '', '', ?, NULL, ?, NULL)`,
		value.ID, value.ProtocolVersion, value.IntentID, value.RunID, value.SessionID,
		value.LeaseID, value.Generation, value.OwnerID,
		value.HandoffOperationKeyDigest, value.MaxSteps, value.PreparedEventSequence,
		ts(value.CreatedAt)); err != nil {
		return domain.RunWakeConsumption{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunWakeConsumption{}, false, err
	}
	return value, false, nil
}

func (s *SQLiteStore) CompleteRunWakeConsumption(ctx context.Context,
	consumptionID string, handoff domain.RunExecutionHandoff, now time.Time,
) (domain.RunWakeConsumption, domain.RunWakeIntent, bool, error) {
	if handoff.Result == nil || handoff.Result.Status != domain.RunExecutionHandoffCompleted ||
		now.IsZero() {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false,
			errors.New("successful run execution handoff is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	value, err := getRunWakeConsumptionByIDTx(ctx, tx, consumptionID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if value.Status == domain.RunWakeConsumptionCompleted {
		intent, err := getRunWakeIntentTx(ctx, tx, value.IntentID)
		return value, intent, true, err
	}
	if value.Status != domain.RunWakeConsumptionPrepared ||
		handoff.Operation.KeyDigest != value.HandoffOperationKeyDigest ||
		handoff.Operation.RunID != value.RunID || handoff.Operation.SessionID != value.SessionID ||
		handoff.Operation.RequestedBy != "run_wake_consumer" ||
		handoff.Operation.MaxSteps != value.MaxSteps {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false,
			errors.New("run wake completion handoff binding is invalid")
	}
	intent, err := getRunWakeIntentTx(ctx, tx, value.IntentID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	lease, err := getRunWakeLeaseTx(ctx, tx, value.LeaseID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if intent.Status != domain.RunWakeLeased || intent.ActiveLeaseID != value.LeaseID ||
		intent.AttemptCount != value.Generation || lease.Status != domain.RunWakeLeaseActive ||
		lease.Generation != value.Generation || lease.OwnerID != value.OwnerID {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false,
			errors.New("run wake completion ownership is stale")
	}
	event, err := appendRunWakeEventTx(ctx, tx, value.RunID,
		events.RunWakeCompletedEvent, "run_wake_consumer", value.ID, map[string]any{
			"intent_id": value.IntentID, "generation": value.Generation,
			"handoff_operation_id": handoff.Operation.ID,
			"stop_reason":          handoff.Result.StopReason,
			"steps_completed":      handoff.Result.StepsCompleted,
			"model_called":         handoff.Result.ModelCalled,
			"tool_called":          handoff.Result.ToolCalled,
		}, now)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE run_wake_consumptions
		SET status = 'completed', handoff_operation_id = ?, stop_reason = ?, error_code = '',
			completion_event_sequence = ?, completed_at = ?
		WHERE id = ? AND status = 'prepared'`, handoff.Operation.ID,
		handoff.Result.StopReason, event.Sequence, ts(now), value.ID); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE run_wake_leases SET status = 'released',
		ended_at = ? WHERE id = ? AND status = 'active'`, ts(now), value.LeaseID); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE run_wake_intents SET status = 'exhausted',
		next_wake_at = deadline_at, active_lease_id = NULL, updated_at = ?
		WHERE id = ? AND status = 'leased' AND active_lease_id = ?`,
		ts(now), value.IntentID, value.LeaseID); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	stored, err := getRunWakeConsumptionByIDTx(ctx, tx, value.ID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	storedIntent, err := getRunWakeIntentTx(ctx, tx, value.IntentID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	return stored, storedIntent, false, nil
}

func (s *SQLiteStore) FailRunWakeConsumption(ctx context.Context,
	consumptionID string, handoffOperationID string, stopReason string, errorCode string,
	now time.Time,
) (domain.RunWakeConsumption, domain.RunWakeIntent, bool, error) {
	if now.IsZero() || stopReason == "" || len(stopReason) > 64 ||
		errorCode == "" || len(errorCode) > 64 {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false,
			errors.New("run wake consumption failure is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	value, err := getRunWakeConsumptionByIDTx(ctx, tx, consumptionID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if value.Status != domain.RunWakeConsumptionPrepared {
		intent, lookupErr := getRunWakeIntentTx(ctx, tx, value.IntentID)
		return value, intent, true, lookupErr
	}
	intent, err := getRunWakeIntentTx(ctx, tx, value.IntentID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	lease, err := getRunWakeLeaseTx(ctx, tx, value.LeaseID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if intent.Status != domain.RunWakeLeased || intent.ActiveLeaseID != value.LeaseID ||
		intent.AttemptCount != value.Generation || lease.IntentID != value.IntentID ||
		lease.Generation != value.Generation || lease.OwnerID != value.OwnerID ||
		lease.Status != domain.RunWakeLeaseActive {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false,
			errors.New("run wake failure ownership is stale")
	}
	modelCalled := false
	toolCalled := false
	if handoffOperationID != "" {
		handoff, found, lookupErr := getRunExecutionHandoffByID(ctx, tx,
			handoffOperationID)
		if lookupErr != nil {
			return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, lookupErr
		}
		if !found || handoff.Operation.KeyDigest != value.HandoffOperationKeyDigest ||
			handoff.Operation.RunID != value.RunID ||
			handoff.Operation.SessionID != value.SessionID ||
			handoff.Operation.RequestedBy != "run_wake_consumer" ||
			handoff.Operation.MaxSteps != value.MaxSteps || handoff.Result == nil ||
			handoff.Result.Status != domain.RunExecutionHandoffFailed ||
			handoff.Result.StopReason != stopReason ||
			handoff.Result.ErrorCode != errorCode {
			return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false,
				errors.New("failed run wake handoff binding is invalid")
		}
		modelCalled = handoff.Result.ModelCalled
		toolCalled = handoff.Result.ToolCalled
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_steering_messages
		WHERE run_id = ? AND session_id = ? AND status = 'pending'`,
		value.RunID, value.SessionID).Scan(&pending); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	delay := runWakeBackoff(intent.BaseBackoffSeconds, intent.MaxBackoffSeconds,
		intent.AttemptCount)
	next := now.Add(time.Duration(delay) * time.Second)
	retry := pending > 0 && intent.AttemptCount < intent.MaxAttempts &&
		next.Before(intent.DeadlineAt)
	eventType := events.RunWakeExhaustedEvent
	status := domain.RunWakeExhausted
	if retry {
		eventType = events.RunWakeRetriedEvent
		status = domain.RunWakeQueued
	} else {
		next = intent.DeadlineAt
	}
	event, err := appendRunWakeEventTx(ctx, tx, value.RunID, eventType,
		"run_wake_coordinator", value.IntentID, map[string]any{
			"generation": value.Generation, "attempt_count": intent.AttemptCount,
			"backoff_seconds": delay, "stop_reason": stopReason,
			"error_code": errorCode, "handoff_operation_id": handoffOperationID,
			"execution_started": handoffOperationID != "", "model_called": modelCalled,
			"tool_called": toolCalled,
		}, now)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	var handoffID any
	if handoffOperationID != "" {
		handoffID = handoffOperationID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE run_wake_consumptions
		SET status = 'failed', handoff_operation_id = ?, stop_reason = ?, error_code = ?,
			completion_event_sequence = ?, completed_at = ?
		WHERE id = ? AND status = 'prepared'`, handoffID, stopReason, errorCode,
		event.Sequence, ts(now), value.ID); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	leaseStatus := domain.RunWakeLeaseReleased
	if now.After(lease.ExpiresAt) {
		leaseStatus = domain.RunWakeLeaseExpired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE run_wake_leases SET status = ?, ended_at = ?
		WHERE id = ? AND status = 'active'`, leaseStatus, ts(now), value.LeaseID); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE run_wake_intents SET status = ?,
		next_wake_at = ?, active_lease_id = NULL, updated_at = ?
		WHERE id = ? AND status = 'leased' AND active_lease_id = ?`, status,
		ts(next), ts(now), value.IntentID, value.LeaseID); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	stored, err := getRunWakeConsumptionByIDTx(ctx, tx, value.ID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	storedIntent, err := getRunWakeIntentTx(ctx, tx, value.IntentID)
	if err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunWakeConsumption{}, domain.RunWakeIntent{}, false, err
	}
	return stored, storedIntent, false, nil
}

func scanRunWakeConsumption(row scanner) (domain.RunWakeConsumption, error) {
	var value domain.RunWakeConsumption
	var status, createdAt string
	var handoffID, completedAt sql.NullString
	var completionSequence sql.NullInt64
	if err := row.Scan(&value.ID, &value.ProtocolVersion, &value.IntentID, &value.RunID,
		&value.SessionID, &value.LeaseID, &value.Generation, &value.OwnerID,
		&value.HandoffOperationKeyDigest, &value.MaxSteps, &status, &handoffID,
		&value.StopReason, &value.ErrorCode, &value.PreparedEventSequence,
		&completionSequence, &createdAt, &completedAt); err != nil {
		return domain.RunWakeConsumption{}, err
	}
	value.Status = domain.RunWakeConsumptionStatus(status)
	value.HandoffOperationID = handoffID.String
	value.CompletionEventSequence = completionSequence.Int64
	value.CreatedAt = parseTS(createdAt)
	value.CompletedAt = parseNullableTS(completedAt)
	if err := value.Validate(); err != nil {
		return domain.RunWakeConsumption{}, fmt.Errorf(
			"stored run wake consumption is invalid: %w", err)
	}
	return value, nil
}

func getRunWakeConsumptionTx(ctx context.Context, tx *sql.Tx, intentID string,
	generation int,
) (domain.RunWakeConsumption, bool, error) {
	value, err := scanRunWakeConsumption(tx.QueryRowContext(ctx,
		runWakeConsumptionSelect+`WHERE intent_id = ? AND generation = ?`,
		intentID, generation))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunWakeConsumption{}, false, nil
	}
	return value, err == nil, err
}

func getRunWakeConsumptionByIDTx(ctx context.Context, tx *sql.Tx,
	id string,
) (domain.RunWakeConsumption, error) {
	return scanRunWakeConsumption(tx.QueryRowContext(ctx,
		runWakeConsumptionSelect+`WHERE id = ?`, id))
}

func sameRunWakeConsumptionIntent(stored domain.RunWakeConsumption,
	requested domain.RunWakeConsumption,
) bool {
	return stored.ProtocolVersion == requested.ProtocolVersion &&
		stored.IntentID == requested.IntentID && stored.RunID == requested.RunID &&
		stored.SessionID == requested.SessionID && stored.LeaseID == requested.LeaseID &&
		stored.Generation == requested.Generation && stored.OwnerID == requested.OwnerID &&
		stored.HandoffOperationKeyDigest == requested.HandoffOperationKeyDigest &&
		stored.MaxSteps == requested.MaxSteps
}
