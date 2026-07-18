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

type runControlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *SQLiteStore) GetRunLifecycleOperation(ctx context.Context,
	keyDigest string,
) (domain.RunLifecycleOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return domain.RunLifecycleOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Run lifecycle operation digest is invalid")
	}
	return getRunLifecycleOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) TransitionRunWithLifecycleOperation(ctx context.Context,
	operation domain.RunLifecycleOperation,
) (domain.RunLifecycleOperation, domain.Run, bool, error) {
	probe := operation
	probe.EventSequenceStart = 1
	probe.EventSequenceEnd = 1
	if probe.Action == domain.RunLifecycleStart {
		probe.EventSequenceEnd = 2
	}
	if operation.EventSequenceStart != 0 || operation.EventSequenceEnd != 0 ||
		probe.Validate() != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false,
			apperror.New(apperror.CodeInvalidArgument,
				"Run lifecycle operation intent is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, found, err := getRunLifecycleOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	} else if found {
		if err := sameRunLifecycleIntent(existing, operation); err != nil {
			return domain.RunLifecycleOperation{}, domain.Run{}, false, err
		}
		run, err := getRunControlRunTx(ctx, tx, existing.RunID)
		if err != nil {
			return domain.RunLifecycleOperation{}, domain.Run{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.RunLifecycleOperation{}, domain.Run{}, false, err
		}
		return existing, run, true, nil
	}
	if err := lockRunControlTx(ctx, tx, operation.RunID); err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	run, err := getRunControlRunTx(ctx, tx, operation.RunID)
	if err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	if run.Status != operation.ExpectedStatus {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			fmt.Sprintf("Run %s is %s; lifecycle action %s expected %s", run.ID,
				run.Status, operation.Action, operation.ExpectedStatus))
	}
	if err := requireNoActiveRunControlLeaseTx(ctx, tx, run.ID,
		operation.CreatedAt); err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	if operation.Action == domain.RunLifecyclePause {
		if err := requireQuiescentRunPauseTx(ctx, tx, run.ID); err != nil {
			return domain.RunLifecycleOperation{}, domain.Run{}, false, err
		}
	}

	sequences := make([]int64, 0, 2)
	if operation.Action == domain.RunLifecycleStart {
		var sequence int64
		run, sequence, err = transitionControlledRunTx(ctx, tx, run,
			domain.RunPreparing, operation.Action, operation.CreatedAt)
		if err != nil {
			return domain.RunLifecycleOperation{}, domain.Run{}, false, err
		}
		sequences = append(sequences, sequence)
		var second int64
		run, second, err = transitionControlledRunTx(ctx, tx, run,
			domain.RunRunning, operation.Action, operation.CreatedAt)
		if err != nil {
			return domain.RunLifecycleOperation{}, domain.Run{}, false, err
		}
		sequences = append(sequences, second)
	} else {
		var sequence int64
		run, sequence, err = transitionControlledRunTx(ctx, tx, run,
			operation.AppliedStatus, operation.Action, operation.CreatedAt)
		if err != nil {
			return domain.RunLifecycleOperation{}, domain.Run{}, false, err
		}
		sequences = append(sequences, sequence)
	}
	if err := syncControlledRunRootTx(ctx, tx, run); err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	operation.EventSequenceStart = sequences[0]
	operation.EventSequenceEnd = sequences[len(sequences)-1]
	if err := operation.Validate(); err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_lifecycle_operations
		(operation_key_digest, request_fingerprint, protocol_version, run_id, action,
		expected_status, applied_status, event_sequence_start, event_sequence_end,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.KeyDigest, operation.RequestFingerprint, operation.ProtocolVersion,
		operation.RunID, operation.Action, operation.ExpectedStatus, operation.AppliedStatus,
		operation.EventSequenceStart, operation.EventSequenceEnd, operation.RequestedBy,
		ts(operation.CreatedAt)); err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunLifecycleOperation{}, domain.Run{}, false, err
	}
	return operation, run, false, nil
}

func getRunLifecycleOperation(ctx context.Context, queryer runControlQueryer,
	keyDigest string,
) (domain.RunLifecycleOperation, bool, error) {
	var operation domain.RunLifecycleOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT protocol_version, operation_key_digest,
		request_fingerprint, run_id, action, expected_status, applied_status,
		event_sequence_start, event_sequence_end, requested_by, created_at
		FROM run_lifecycle_operations WHERE operation_key_digest = ?`, keyDigest).Scan(
		&operation.ProtocolVersion, &operation.KeyDigest, &operation.RequestFingerprint,
		&operation.RunID, &operation.Action, &operation.ExpectedStatus,
		&operation.AppliedStatus, &operation.EventSequenceStart,
		&operation.EventSequenceEnd, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunLifecycleOperation{}, false, nil
	}
	if err != nil {
		return domain.RunLifecycleOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.Validate(); err != nil {
		return domain.RunLifecycleOperation{}, false,
			fmt.Errorf("validate stored Run lifecycle operation: %w", err)
	}
	return operation, true, nil
}

func sameRunLifecycleIntent(existing domain.RunLifecycleOperation,
	requested domain.RunLifecycleOperation,
) error {
	if existing.ProtocolVersion != requested.ProtocolVersion ||
		existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.RunID != requested.RunID || existing.Action != requested.Action ||
		existing.ExpectedStatus != requested.ExpectedStatus ||
		existing.AppliedStatus != requested.AppliedStatus ||
		existing.RequestedBy != requested.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Run lifecycle idempotency key was already used for different intent")
	}
	return nil
}

func transitionControlledRunTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	target domain.RunStatus, action domain.RunLifecycleAction, at time.Time,
) (domain.Run, int64, error) {
	from := run.Status
	if err := run.Transition(target, at); err != nil {
		return domain.Run{}, 0, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, started_at = ?,
		finished_at = ?, updated_at = ? WHERE id = ? AND status = ?`, run.Status,
		nullableTS(run.StartedAt), nullableTS(run.FinishedAt), ts(run.UpdatedAt), run.ID, from)
	if err != nil {
		return domain.Run{}, 0, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return domain.Run{}, 0, err
		}
		return domain.Run{}, 0, apperror.New(apperror.CodeConflict,
			"Run lifecycle status changed concurrently")
	}
	event, err := events.New(run.ID, run.MissionID, events.RunStatusChangedEvent,
		"run_lifecycle_control", run.ID, map[string]any{
			"from": from, "to": target, "action": action,
		})
	if err != nil {
		return domain.Run{}, 0, err
	}
	event.CreatedAt = at.UTC()
	stored, err := insertRunEventTx(ctx, tx, event)
	if err != nil {
		return domain.Run{}, 0, err
	}
	return run, stored.Sequence, nil
}

func syncControlledRunRootTx(ctx context.Context, tx *sql.Tx, run domain.Run) error {
	mission, err := scanMission(tx.QueryRowContext(ctx, `SELECT id, goal, profile,
		workspace_id, scope_json, created_at, updated_at FROM missions WHERE id = ?`,
		run.MissionID))
	if err != nil {
		return err
	}
	projection, err := rootAgentProjectionForRunTx(ctx, tx, run)
	if err != nil {
		return err
	}
	if _, _, changed, err := syncRootAgentTx(ctx, tx, run, mission, projection,
		run.UpdatedAt); err != nil {
		return err
	} else if changed {
		_, err = createAgentGraphSnapshotTx(ctx, tx, run)
		return err
	}
	return nil
}

func lockRunControlTx(ctx context.Context, tx *sql.Tx, runID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`,
		runID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return err
		}
		return apperror.New(apperror.CodeNotFound, "Run was not found")
	}
	return nil
}

func getRunControlRunTx(ctx context.Context, tx *sql.Tx, runID string) (domain.Run, error) {
	return scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, runID))
}

func requireNoActiveRunControlLeaseTx(ctx context.Context, tx *sql.Tx, runID string,
	at time.Time,
) error {
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_execution_leases
		WHERE run_id = ? AND status = 'active' AND julianday(expires_at) > julianday(?)`,
		runID, ts(at)).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return apperror.New(apperror.CodeConflict,
			"Run lifecycle control requires no active execution lease")
	}
	return nil
}

func requireQuiescentRunPauseTx(ctx context.Context, tx *sql.Tx, runID string) error {
	var activeAgents, activeTurns, prepared int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_nodes
		WHERE run_id = ? AND (status = 'running' OR active_attempt_id <> '')`, runID).
		Scan(&activeAgents); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_supervisor_checkpoints
		WHERE run_id = ? AND phase = 'turn_started'`, runID).Scan(&activeTurns); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_steering_deliveries
		WHERE run_id = ? AND status = 'prepared'`, runID).Scan(&prepared); err != nil {
		return err
	}
	if activeAgents != 0 || activeTurns != 0 || prepared != 0 {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Run pause requires a quiescent Supervisor boundary")
	}
	return nil
}
