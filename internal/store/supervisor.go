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
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
)

const maxSupervisorErrorChars = 4096

func (s *SQLiteStore) GetSupervisorCheckpoint(ctx context.Context, runID string) (domain.SupervisorCheckpoint, bool, error) {
	checkpoint, err := scanSupervisorCheckpoint(s.db.QueryRowContext(ctx, `SELECT run_id, next_turn, phase,
		attempt_id, last_error, updated_at FROM run_supervisor_checkpoints WHERE run_id = ?`, strings.TrimSpace(runID)))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SupervisorCheckpoint{}, false, nil
	}
	if err != nil {
		return domain.SupervisorCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (s *SQLiteStore) BeginSupervisorTurn(ctx context.Context, runID string) (domain.SupervisorTurn, error) {
	if err := ctx.Err(); err != nil {
		return domain.SupervisorTurn{}, apperror.Normalize(err)
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeInvalidArgument, "run id is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SupervisorTurn{}, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, runID))
	if err != nil {
		return domain.SupervisorTurn{}, err
	}
	if run.Status != domain.RunRunning {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("run %s is %s; supervisor requires running", run.ID, run.Status))
	}
	mission, err := scanMission(tx.QueryRowContext(ctx, `SELECT id, goal, profile, workspace_id, scope_json,
		created_at, updated_at FROM missions WHERE id = ?`, run.MissionID))
	if err != nil {
		return domain.SupervisorTurn{}, err
	}
	checkpoint, ok, err := getSupervisorCheckpointTx(ctx, tx, run.ID)
	if err != nil {
		return domain.SupervisorTurn{}, err
	}
	if !ok {
		checkpoint = domain.SupervisorCheckpoint{
			RunID: run.ID, NextTurn: 1, Phase: domain.SupervisorIdle, UpdatedAt: time.Now().UTC(),
		}
	}
	if checkpoint.NextTurn > run.Budget.MaxTurns {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeResourceExhausted,
			fmt.Sprintf("run %s exhausted its %d turn budget", run.ID, run.Budget.MaxTurns))
	}
	if checkpoint.Phase == domain.SupervisorTurnStarted {
		if err := tx.Commit(); err != nil {
			return domain.SupervisorTurn{}, err
		}
		return domain.SupervisorTurn{Run: run, Mission: mission, Checkpoint: checkpoint, Recovered: true}, nil
	}

	checkpoint.Phase = domain.SupervisorTurnStarted
	checkpoint.AttemptID = idgen.New("attempt")
	checkpoint.LastError = ""
	checkpoint.UpdatedAt = time.Now().UTC()
	if err := upsertSupervisorCheckpointTx(ctx, tx, checkpoint); err != nil {
		return domain.SupervisorTurn{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorCheckpointedEvent, "run_supervisor", run.ID, map[string]any{
		"phase": checkpoint.Phase, "next_turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
	}); err != nil {
		return domain.SupervisorTurn{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.AgentTurnStartedEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID, "recovered": false,
	}); err != nil {
		return domain.SupervisorTurn{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SupervisorTurn{}, err
	}
	return domain.SupervisorTurn{Run: run, Mission: mission, Checkpoint: checkpoint}, nil
}

func (s *SQLiteStore) CompleteSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, input string, response llm.ChatResponse, decision policy.Decision) (domain.SupervisorCheckpoint, error) {
	if err := checkpoint.Validate(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can complete")
	}
	if !decision.Allowed {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "denied supervisor output cannot be completed")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, ok, err := getSupervisorCheckpointTx(ctx, tx, checkpoint.RunID)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if !ok {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "supervisor checkpoint was not found")
	}
	if current.Phase == domain.SupervisorIdle && current.NextTurn == checkpoint.NextTurn+1 {
		if err := tx.Commit(); err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
		return current, nil
	}
	if current.Phase != domain.SupervisorTurnStarted || current.NextTurn != checkpoint.NextTurn || current.AttemptID != checkpoint.AttemptID {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "supervisor checkpoint changed before turn completion")
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, checkpoint.RunID))
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if run.Status != domain.RunRunning {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "run stopped before turn completion")
	}
	userMessage, err := saveSessionMessageTx(ctx, tx, session.NewMessage(run.SessionID, "user", input))
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	assistantMessage, err := saveSessionMessageTx(ctx, tx, session.NewMessage(run.SessionID, "assistant", response.Text))
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.PolicyDecisionEvent, "policy", checkpoint.AttemptID, map[string]any{
		"context": "supervisor_assistant_response", "allowed": decision.Allowed,
		"needs_approval": decision.NeedsApproval, "risk": decision.Risk, "reason": decision.Reason,
	}); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.AgentTurnCompletedEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"user_message_id": userMessage.ID, "assistant_message_id": assistantMessage.ID,
		"provider": response.Provider, "model": response.Model, "usage": response.Usage,
	}); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	current.NextTurn++
	current.Phase = domain.SupervisorIdle
	current.AttemptID = ""
	current.LastError = ""
	current.UpdatedAt = time.Now().UTC()
	if err := upsertSupervisorCheckpointTx(ctx, tx, current); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorCheckpointedEvent, "run_supervisor", run.ID, map[string]any{
		"phase": current.Phase, "next_turn": current.NextTurn, "completed_turn": checkpoint.NextTurn,
	}); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	return current, nil
}

func (s *SQLiteStore) FailSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, cause string) (domain.SupervisorCheckpoint, error) {
	if err := checkpoint.Validate(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	cause = redact.String(strings.TrimSpace(cause))
	if cause == "" {
		cause = "supervisor turn failed"
	}
	runes := []rune(cause)
	if len(runes) > maxSupervisorErrorChars {
		cause = string(runes[:maxSupervisorErrorChars])
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, ok, err := getSupervisorCheckpointTx(ctx, tx, checkpoint.RunID)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if !ok {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "supervisor checkpoint was not found")
	}
	if current.Phase == domain.SupervisorTurnFailed && current.AttemptID == checkpoint.AttemptID {
		if err := tx.Commit(); err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
		return current, nil
	}
	if current.Phase != domain.SupervisorTurnStarted || current.NextTurn != checkpoint.NextTurn || current.AttemptID != checkpoint.AttemptID {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "supervisor checkpoint changed before failure was recorded")
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, checkpoint.RunID))
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	current.Phase = domain.SupervisorTurnFailed
	current.LastError = cause
	current.UpdatedAt = time.Now().UTC()
	if err := upsertSupervisorCheckpointTx(ctx, tx, current); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.AgentTurnFailedEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID, "error": cause,
	}); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorCheckpointedEvent, "run_supervisor", run.ID, map[string]any{
		"phase": current.Phase, "next_turn": current.NextTurn, "attempt_id": current.AttemptID,
	}); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	return current, nil
}

func getSupervisorCheckpointTx(ctx context.Context, tx *sql.Tx, runID string) (domain.SupervisorCheckpoint, bool, error) {
	checkpoint, err := scanSupervisorCheckpoint(tx.QueryRowContext(ctx, `SELECT run_id, next_turn, phase,
		attempt_id, last_error, updated_at FROM run_supervisor_checkpoints WHERE run_id = ?`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SupervisorCheckpoint{}, false, nil
	}
	if err != nil {
		return domain.SupervisorCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func upsertSupervisorCheckpointTx(ctx context.Context, tx *sql.Tx, checkpoint domain.SupervisorCheckpoint) error {
	checkpoint.LastError = redact.String(checkpoint.LastError)
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO run_supervisor_checkpoints
		(run_id, next_turn, phase, attempt_id, last_error, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET next_turn=excluded.next_turn, phase=excluded.phase,
		attempt_id=excluded.attempt_id, last_error=excluded.last_error, updated_at=excluded.updated_at`,
		checkpoint.RunID, checkpoint.NextTurn, checkpoint.Phase, checkpoint.AttemptID,
		checkpoint.LastError, ts(checkpoint.UpdatedAt))
	return err
}

func appendSupervisorEventTx(ctx context.Context, tx *sql.Tx, run domain.Run, eventType string, source string, subjectID string, payload any) error {
	event, err := events.New(run.ID, run.MissionID, eventType, source, subjectID, payload)
	if err != nil {
		return err
	}
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}

func scanSupervisorCheckpoint(row scanner) (domain.SupervisorCheckpoint, error) {
	var checkpoint domain.SupervisorCheckpoint
	var phase string
	var attempt sql.NullString
	var lastError sql.NullString
	var updated string
	if err := row.Scan(&checkpoint.RunID, &checkpoint.NextTurn, &phase, &attempt, &lastError, &updated); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	checkpoint.Phase = domain.SupervisorPhase(phase)
	checkpoint.AttemptID = attempt.String
	checkpoint.LastError = lastError.String
	checkpoint.UpdatedAt = parseTS(updated)
	return checkpoint, checkpoint.Validate()
}
