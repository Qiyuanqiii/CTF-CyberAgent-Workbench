package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strings"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

const runProgressGuardSelect = `SELECT run_id, protocol_version, state_fingerprint,
	action_fingerprint, repeated_action_count, stagnant_turn_count, repeat_threshold,
	stagnant_threshold, last_turn, status, reason_code, detected_at, updated_at
	FROM run_progress_guards`

func (s *SQLiteStore) GetRunProgressGuard(ctx context.Context, runID string) (
	domain.RunProgressGuard, bool, error,
) {
	if s == nil || s.db == nil {
		return domain.RunProgressGuard{}, false, errors.New("SQLite store is required")
	}
	runID = strings.TrimSpace(runID)
	guard, found, err := scanRunProgressGuard(s.db.QueryRowContext(ctx,
		runProgressGuardSelect+` WHERE run_id = ?`, runID))
	if err != nil || !found {
		return guard, found, err
	}
	return guard, true, guard.Validate()
}

func getRunProgressGuardTx(ctx context.Context, tx *sql.Tx, runID string) (
	domain.RunProgressGuard, bool, error,
) {
	guard, found, err := scanRunProgressGuard(tx.QueryRowContext(ctx,
		runProgressGuardSelect+` WHERE run_id = ?`, runID))
	if err != nil || !found {
		return guard, found, err
	}
	return guard, true, guard.Validate()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRunProgressGuard(row rowScanner) (domain.RunProgressGuard, bool, error) {
	var guard domain.RunProgressGuard
	var status, reason, updatedAt string
	var detectedAt sql.NullString
	err := row.Scan(&guard.RunID, &guard.ProtocolVersion, &guard.StateFingerprint,
		&guard.ActionFingerprint, &guard.RepeatedActionCount, &guard.StagnantTurnCount,
		&guard.RepeatThreshold, &guard.StagnantThreshold, &guard.LastTurn, &status,
		&reason, &detectedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunProgressGuard{}, false, nil
	}
	if err != nil {
		return domain.RunProgressGuard{}, false, err
	}
	guard.Status = domain.RunProgressGuardStatus(status)
	guard.Reason = domain.RunProgressReason(reason)
	guard.UpdatedAt = parseTS(updatedAt)
	if detectedAt.Valid && strings.TrimSpace(detectedAt.String) != "" {
		value := parseTS(detectedAt.String)
		guard.DetectedAt = &value
	}
	return guard, true, nil
}

func evaluateSupervisorProgressTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	checkpoint domain.SupervisorCheckpoint, action domain.RootAction,
	requestedAction domain.RootActionKind, at time.Time,
) (domain.RootAction, error) {
	previous, found, err := getRunProgressGuardTx(ctx, tx, run.ID)
	if err != nil {
		return domain.RootAction{}, err
	}
	countable := requestedAction == domain.RootActionContinue &&
		action.Kind == domain.RootActionContinue
	if !countable {
		if !found {
			return action, nil
		}
		reset := emptyRunProgressGuard(run.ID, checkpoint.NextTurn, at)
		if err := upsertRunProgressGuardTx(ctx, tx, reset); err != nil {
			return domain.RootAction{}, err
		}
		if previous.RepeatedActionCount > 0 || previous.StagnantTurnCount > 0 ||
			previous.Status == domain.RunProgressDetected {
			if err := appendSupervisorEventTx(ctx, tx, run,
				events.SupervisorProgressResetEvent, "run_supervisor", run.ID,
				map[string]any{"turn": checkpoint.NextTurn, "reason": "non_continue_action"}); err != nil {
				return domain.RootAction{}, err
			}
		}
		return action, nil
	}

	stateFingerprint, err := supervisorProgressStateFingerprintTx(ctx, tx, run.ID)
	if err != nil {
		return domain.RootAction{}, err
	}
	actionFingerprint := supervisorActionFingerprint(action)
	guard := domain.RunProgressGuard{
		RunID: run.ID, ProtocolVersion: domain.RunProgressGuardProtocolVersion,
		StateFingerprint: stateFingerprint, ActionFingerprint: actionFingerprint,
		RepeatedActionCount: 1, StagnantTurnCount: 1,
		RepeatThreshold:   domain.RunProgressRepeatThreshold,
		StagnantThreshold: domain.RunProgressStagnantThreshold,
		LastTurn:          checkpoint.NextTurn, Status: domain.RunProgressObserving, UpdatedAt: at,
	}
	resetReason := ""
	stateChanged := !found || previous.StateFingerprint != stateFingerprint
	switch {
	case found && previous.Status == domain.RunProgressDetected:
		resetReason = "explicit_resume"
		stateChanged = true
	case found && !stateChanged:
		guard.StagnantTurnCount = min(previous.StagnantTurnCount+1,
			guard.StagnantThreshold)
		if previous.ActionFingerprint == actionFingerprint {
			guard.RepeatedActionCount = min(previous.RepeatedActionCount+1,
				guard.RepeatThreshold)
		}
	case found:
		resetReason = "observable_state_changed"
	}

	switch {
	case guard.RepeatedActionCount >= guard.RepeatThreshold:
		guard.Status = domain.RunProgressDetected
		guard.Reason = domain.RunProgressRepeatedAction
		guard.DetectedAt = &at
	case guard.StagnantTurnCount >= guard.StagnantThreshold:
		guard.Status = domain.RunProgressDetected
		guard.Reason = domain.RunProgressNoObservableProgress
		guard.DetectedAt = &at
	}
	if err := guard.Validate(); err != nil {
		return domain.RootAction{}, err
	}
	if err := upsertRunProgressGuardTx(ctx, tx, guard); err != nil {
		return domain.RootAction{}, err
	}
	if resetReason != "" {
		if err := appendSupervisorEventTx(ctx, tx, run,
			events.SupervisorProgressResetEvent, "run_supervisor", run.ID,
			map[string]any{"turn": checkpoint.NextTurn, "reason": resetReason}); err != nil {
			return domain.RootAction{}, err
		}
	}
	if err := appendSupervisorEventTx(ctx, tx, run,
		events.SupervisorProgressObservedEvent, "run_supervisor", run.ID,
		map[string]any{
			"turn": checkpoint.NextTurn, "state_fingerprint": stateFingerprint,
			"action_fingerprint":    actionFingerprint,
			"state_changed":         stateChanged,
			"repeated_action_count": guard.RepeatedActionCount,
			"stagnant_turn_count":   guard.StagnantTurnCount,
			"repeat_threshold":      guard.RepeatThreshold,
			"stagnant_threshold":    guard.StagnantThreshold,
		}); err != nil {
		return domain.RootAction{}, err
	}
	if guard.Status != domain.RunProgressDetected {
		return action, nil
	}
	if err := appendSupervisorEventTx(ctx, tx, run,
		events.SupervisorLivelockDetectedEvent, "run_supervisor", run.ID,
		map[string]any{
			"turn": checkpoint.NextTurn, "reason": guard.Reason,
			"repeated_action_count": guard.RepeatedActionCount,
			"stagnant_turn_count":   guard.StagnantTurnCount,
			"repeat_threshold":      guard.RepeatThreshold,
			"stagnant_threshold":    guard.StagnantThreshold,
		}); err != nil {
		return domain.RootAction{}, err
	}
	action.Kind = domain.RootActionWait
	action.Summary = ""
	action.Reason = guard.WaitReason()
	return action, action.Validate()
}

func emptyRunProgressGuard(runID string, turn int, at time.Time) domain.RunProgressGuard {
	return domain.RunProgressGuard{
		RunID: runID, ProtocolVersion: domain.RunProgressGuardProtocolVersion,
		RepeatThreshold:   domain.RunProgressRepeatThreshold,
		StagnantThreshold: domain.RunProgressStagnantThreshold,
		LastTurn:          turn, Status: domain.RunProgressObserving, UpdatedAt: at,
	}
}

func upsertRunProgressGuardTx(ctx context.Context, tx *sql.Tx,
	guard domain.RunProgressGuard,
) error {
	if err := guard.Validate(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO run_progress_guards
		(run_id, protocol_version, state_fingerprint, action_fingerprint,
		repeated_action_count, stagnant_turn_count, repeat_threshold, stagnant_threshold,
		last_turn, status, reason_code, detected_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			protocol_version = excluded.protocol_version,
			state_fingerprint = excluded.state_fingerprint,
			action_fingerprint = excluded.action_fingerprint,
			repeated_action_count = excluded.repeated_action_count,
			stagnant_turn_count = excluded.stagnant_turn_count,
			repeat_threshold = excluded.repeat_threshold,
			stagnant_threshold = excluded.stagnant_threshold,
			last_turn = excluded.last_turn,
			status = excluded.status,
			reason_code = excluded.reason_code,
			detected_at = excluded.detected_at,
			updated_at = excluded.updated_at`,
		guard.RunID, guard.ProtocolVersion, guard.StateFingerprint, guard.ActionFingerprint,
		guard.RepeatedActionCount, guard.StagnantTurnCount, guard.RepeatThreshold,
		guard.StagnantThreshold, guard.LastTurn, guard.Status, guard.Reason,
		nullableTS(guard.DetectedAt), ts(guard.UpdatedAt))
	return err
}

func supervisorActionFingerprint(action domain.RootAction) string {
	normalizedMessage := strings.Join(strings.Fields(strings.TrimSpace(action.Message)), " ")
	sum := sha256.Sum256([]byte(string(action.Kind) + "\x00" + normalizedMessage))
	return hex.EncodeToString(sum[:])
}

func supervisorProgressStateFingerprintTx(ctx context.Context, tx *sql.Tx,
	runID string,
) (string, error) {
	digest := sha256.New()
	queries := []struct {
		label string
		sql   string
	}{
		{"mode", `SELECT json_array(id, revision, surface, phase, profile)
			FROM run_mode_snapshots WHERE run_id = ? ORDER BY revision DESC LIMIT 1`},
		{"work_items", `SELECT json_array(id, status, priority, owner, owner_agent_id, version)
			FROM work_items WHERE run_id = ? ORDER BY id`},
		{"notes", `SELECT json_array(id, status, category, visibility, owner, owner_agent_id, pinned, version)
			FROM notes WHERE run_id = ? ORDER BY id`},
		{"specialists", `SELECT json_array(id, parent_id, status, version)
			FROM agent_nodes WHERE run_id = ? AND role = 'specialist' ORDER BY id`},
		{"plan_proposals", `SELECT json_array(id, status, version)
			FROM plan_delivery_proposals WHERE run_id = ? ORDER BY id`},
		{"plan_selections", `SELECT json_array(id, proposal_id, direction_ordinal, version)
			FROM plan_delivery_selections WHERE run_id = ? ORDER BY id`},
		{"delegation_applications", `SELECT json_array(id, status, version)
			FROM specialist_delegation_applications WHERE run_id = ? ORDER BY id`},
		{"delivery_checkpoints", `SELECT json_array(id, work_item_id, mode_revision, work_item_version, version)
			FROM delivery_checkpoints WHERE run_id = ? ORDER BY id`},
	}
	for _, query := range queries {
		writeProgressHashValue(digest, query.label)
		rows, err := tx.QueryContext(ctx, query.sql, runID)
		if err != nil {
			return "", fmt.Errorf("read %s progress state: %w", query.label, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				return "", err
			}
			writeProgressHashValue(digest, value)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return "", err
		}
		if err := rows.Close(); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeProgressHashValue(digest hash.Hash, value string) {
	_, _ = fmt.Fprintf(digest, "%d:", len(value))
	_, _ = digest.Write([]byte(value))
}
