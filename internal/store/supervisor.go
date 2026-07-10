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

const (
	maxSupervisorErrorChars = 4096
	maxSupervisorInputChars = 64 * 1024
)

const supervisorCheckpointSelect = `SELECT run_id, next_turn, phase, attempt_id, last_error,
	pending_input, input_tokens, output_tokens, total_tokens, execution_millis, updated_at
	FROM run_supervisor_checkpoints WHERE run_id = ?`

func (s *SQLiteStore) GetSupervisorCheckpoint(ctx context.Context, runID string) (domain.SupervisorCheckpoint, bool, error) {
	checkpoint, err := scanSupervisorCheckpoint(s.db.QueryRowContext(ctx, supervisorCheckpointSelect, strings.TrimSpace(runID)))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SupervisorCheckpoint{}, false, nil
	}
	if err != nil {
		return domain.SupervisorCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (s *SQLiteStore) BeginSupervisorTurn(ctx context.Context, runID string, pendingInput string) (domain.SupervisorTurn, error) {
	if err := ctx.Err(); err != nil {
		return domain.SupervisorTurn{}, apperror.Normalize(err)
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeInvalidArgument, "run id is required")
	}
	pendingInput, err := normalizeSupervisorInput(pendingInput)
	if err != nil {
		return domain.SupervisorTurn{}, err
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
	if checkpoint.Phase == domain.SupervisorRunCompleted || checkpoint.Phase == domain.SupervisorRunFailed {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("run %s supervisor is finalized as %s", run.ID, checkpoint.Phase))
	}
	if checkpoint.NextTurn > run.Budget.MaxTurns {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeResourceExhausted,
			fmt.Sprintf("run %s exhausted its %d turn budget", run.ID, run.Budget.MaxTurns))
	}
	if run.Budget.MaxTokens > 0 && checkpoint.TotalTokens >= run.Budget.MaxTokens {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeResourceExhausted,
			fmt.Sprintf("run %s exhausted its %d token budget", run.ID, run.Budget.MaxTokens))
	}
	if run.Budget.TimeoutSeconds > 0 && checkpoint.ExecutionMillis >= run.Budget.TimeoutSeconds*1000 {
		return domain.SupervisorTurn{}, apperror.New(apperror.CodeDeadlineExceeded,
			fmt.Sprintf("run %s exhausted its %s execution timeout", run.ID, time.Duration(run.Budget.TimeoutSeconds)*time.Second))
	}
	if checkpoint.Phase == domain.SupervisorTurnStarted {
		if pendingInput != "" {
			switch {
			case checkpoint.PendingInput == "":
				checkpoint.PendingInput = pendingInput
				checkpoint.UpdatedAt = time.Now().UTC()
				if err := upsertSupervisorCheckpointTx(ctx, tx, checkpoint); err != nil {
					return domain.SupervisorTurn{}, err
				}
			case checkpoint.PendingInput != pendingInput:
				return domain.SupervisorTurn{}, apperror.New(apperror.CodeConflict, "supervisor turn already has different pending input")
			}
		}
		if err := tx.Commit(); err != nil {
			return domain.SupervisorTurn{}, err
		}
		return domain.SupervisorTurn{Run: run, Mission: mission, Checkpoint: checkpoint, Recovered: true}, nil
	}
	if checkpoint.Phase == domain.SupervisorTurnFailed && pendingInput == "" {
		pendingInput = checkpoint.PendingInput
	}

	checkpoint.Phase = domain.SupervisorTurnStarted
	checkpoint.AttemptID = idgen.New("attempt")
	checkpoint.PendingInput = pendingInput
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

func (s *SQLiteStore) BindSupervisorTurnInput(ctx context.Context, checkpoint domain.SupervisorCheckpoint, input string) (domain.SupervisorCheckpoint, error) {
	if err := checkpoint.Validate(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can bind input")
	}
	input, err := normalizeSupervisorInput(input)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if input == "" {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeInvalidArgument, "supervisor input is required")
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
	if current.Phase != domain.SupervisorTurnStarted || current.NextTurn != checkpoint.NextTurn || current.AttemptID != checkpoint.AttemptID {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "supervisor checkpoint changed before input binding")
	}
	if current.PendingInput != "" && current.PendingInput != input {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "supervisor turn already has different pending input")
	}
	if current.PendingInput == "" {
		current.PendingInput = input
		current.UpdatedAt = time.Now().UTC()
		if err := upsertSupervisorCheckpointTx(ctx, tx, current); err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	return current, nil
}

func (s *SQLiteStore) NextSupervisorModelAttempt(ctx context.Context, checkpoint domain.SupervisorCheckpoint) (int, error) {
	if err := checkpoint.Validate(); err != nil {
		return 0, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return 0, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can call a model")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint); err != nil {
		return 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id LIKE ?`, checkpoint.RunID,
		events.ModelStartedEvent, "model_gateway", supervisorModelSubjectPrefix(checkpoint)+"%").Scan(&count); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count + 1, nil
}

func (s *SQLiteStore) RecordSupervisorModelStarted(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt) (bool, error) {
	if err := checkpoint.Validate(); err != nil {
		return false, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return false, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can call a model")
	}
	attempt = sanitizeModelAttempt(attempt)
	if attempt.Outcome != "" || strings.TrimSpace(attempt.ErrorText) != "" {
		return false, apperror.New(apperror.CodeInvalidArgument, "started model attempt cannot have an outcome or error")
	}
	if err := attempt.ValidateStarted(); err != nil {
		return false, apperror.Wrap(apperror.CodeInvalidArgument, "invalid started model attempt", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	run, _, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
	if err != nil {
		return false, err
	}
	subject := supervisorModelSubject(checkpoint, attempt.Number)
	exists, err := supervisorModelEventExistsTx(ctx, tx, run.ID, events.ModelStartedEvent, subject)
	if err != nil {
		return false, err
	}
	if exists {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	var startedCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id LIKE ?`, run.ID,
		events.ModelStartedEvent, "model_gateway", supervisorModelSubjectPrefix(checkpoint)+"%").Scan(&startedCount); err != nil {
		return false, err
	}
	if attempt.Number != startedCount+1 {
		return false, apperror.New(apperror.CodeConflict, "model attempt number is not the next durable attempt")
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelStartedEvent, "model_gateway", subject, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "max_attempts": attempt.MaxAttempts,
		"provider": attempt.Provider, "model": attempt.Model,
	}); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) RecordSupervisorModelCompleted(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, response llm.ChatResponse) (domain.SupervisorCheckpoint, error) {
	attempt = sanitizeModelAttempt(attempt)
	if err := attempt.ValidateCompleted(); err != nil {
		return domain.SupervisorCheckpoint{}, apperror.Wrap(apperror.CodeInvalidArgument, "invalid completed model attempt", err)
	}
	if _, _, _, err := supervisorUsage(response.Usage); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	payload := map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "max_attempts": attempt.MaxAttempts,
		"provider": attempt.Provider, "model": attempt.Model, "outcome": attempt.Outcome,
		"elapsed_millis": attempt.Elapsed.Milliseconds(), "usage": response.Usage,
	}
	return s.recordSupervisorModelTerminal(ctx, checkpoint, attempt, events.ModelCompletedEvent, payload)
}

func (s *SQLiteStore) RecordSupervisorModelFailed(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt) (domain.SupervisorCheckpoint, error) {
	attempt = sanitizeModelAttempt(attempt)
	if err := attempt.ValidateFailed(); err != nil {
		return domain.SupervisorCheckpoint{}, apperror.Wrap(apperror.CodeInvalidArgument, "invalid failed model attempt", err)
	}
	payload := map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "max_attempts": attempt.MaxAttempts,
		"provider": attempt.Provider, "model": attempt.Model, "outcome": attempt.Outcome,
		"error": attempt.ErrorText, "elapsed_millis": attempt.Elapsed.Milliseconds(),
		"retry_after_millis": attempt.RetryAfter.Milliseconds(), "retry_planned": attempt.RetryPlanned,
	}
	return s.recordSupervisorModelTerminal(ctx, checkpoint, attempt, events.ModelFailedEvent, payload)
}

func (s *SQLiteStore) recordSupervisorModelTerminal(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, eventType string, payload map[string]any) (domain.SupervisorCheckpoint, error) {
	if err := checkpoint.Validate(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can finish a model attempt")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	defer func() { _ = tx.Rollback() }()
	run, current, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	subject := supervisorModelSubject(checkpoint, attempt.Number)
	started, err := supervisorModelEventExistsTx(ctx, tx, run.ID, events.ModelStartedEvent, subject)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if !started {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "model attempt was not started")
	}
	completed, err := supervisorModelEventExistsTx(ctx, tx, run.ID, events.ModelCompletedEvent, subject)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	failed, err := supervisorModelEventExistsTx(ctx, tx, run.ID, events.ModelFailedEvent, subject)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if (eventType == events.ModelCompletedEvent && completed) || (eventType == events.ModelFailedEvent && failed) {
		if err := tx.Commit(); err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
		return current, nil
	}
	if completed || failed {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "model attempt already has a terminal event")
	}
	elapsedMillis, err := supervisorElapsedMillis(attempt.Elapsed)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	current.ExecutionMillis, err = supervisorAddCounter(current.ExecutionMillis, elapsedMillis, "execution time")
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	current.UpdatedAt = time.Now().UTC()
	if err := upsertSupervisorCheckpointTx(ctx, tx, current); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, eventType, "model_gateway", subject, payload); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	return current, nil
}

func requireActiveSupervisorAttemptTx(ctx context.Context, tx *sql.Tx, checkpoint domain.SupervisorCheckpoint) (domain.Run, domain.SupervisorCheckpoint, error) {
	current, ok, err := getSupervisorCheckpointTx(ctx, tx, checkpoint.RunID)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	if !ok {
		return domain.Run{}, domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "supervisor checkpoint was not found")
	}
	if current.Phase != domain.SupervisorTurnStarted || current.NextTurn != checkpoint.NextTurn || current.AttemptID != checkpoint.AttemptID {
		return domain.Run{}, domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "supervisor checkpoint changed before model attempt")
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, checkpoint.RunID))
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	if run.Status != domain.RunRunning {
		return domain.Run{}, domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "run stopped before model attempt")
	}
	return run, current, nil
}

func supervisorModelEventExistsTx(ctx context.Context, tx *sql.Tx, runID string, eventType string, subject string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ? AND subject_id = ?`,
		runID, eventType, subject).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func supervisorModelSubjectPrefix(checkpoint domain.SupervisorCheckpoint) string {
	return checkpoint.AttemptID + "/model/"
}

func supervisorModelSubject(checkpoint domain.SupervisorCheckpoint, number int) string {
	return fmt.Sprintf("%s%d", supervisorModelSubjectPrefix(checkpoint), number)
}

func sanitizeModelAttempt(attempt llm.ModelAttempt) llm.ModelAttempt {
	attempt.Provider = redact.String(strings.TrimSpace(attempt.Provider))
	attempt.Model = redact.String(strings.TrimSpace(attempt.Model))
	attempt.ErrorText = redact.String(strings.TrimSpace(attempt.ErrorText))
	runes := []rune(attempt.ErrorText)
	if len(runes) > maxSupervisorErrorChars {
		attempt.ErrorText = string(runes[:maxSupervisorErrorChars])
	}
	return attempt
}

func (s *SQLiteStore) CompleteSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, response llm.ChatResponse, action domain.RootAction, decision policy.Decision, elapsed time.Duration) (domain.Run, domain.SupervisorCheckpoint, session.TurnMessages, error) {
	emptyMessages := session.TurnMessages{}
	if err := checkpoint.Validate(); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can complete")
	}
	if !decision.Allowed {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.New(apperror.CodeFailedPrecondition, "denied supervisor output cannot be completed")
	}
	action = sanitizeRootAction(action)
	if err := action.Validate(); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.Wrap(apperror.CodeFailedPrecondition, "invalid root lifecycle action", err)
	}
	response.Text = action.Message
	inputTokens, outputTokens, totalTokens, err := supervisorUsage(response.Usage)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	elapsedMillis, err := supervisorElapsedMillis(elapsed)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	defer func() { _ = tx.Rollback() }()
	current, ok, err := getSupervisorCheckpointTx(ctx, tx, checkpoint.RunID)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if !ok {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.New(apperror.CodeFailedPrecondition, "supervisor checkpoint was not found")
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, checkpoint.RunID))
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if current.Phase == supervisorPhaseForAction(action.Kind) && current.NextTurn == checkpoint.NextTurn+1 {
		if err := tx.Commit(); err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
		}
		return run, current, emptyMessages, nil
	}
	if current.Phase != domain.SupervisorTurnStarted || current.NextTurn != checkpoint.NextTurn || current.AttemptID != checkpoint.AttemptID {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.New(apperror.CodeConflict, "supervisor checkpoint changed before turn completion")
	}
	if run.Status != domain.RunRunning {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.New(apperror.CodeFailedPrecondition, "run stopped before turn completion")
	}
	if current.PendingInput == "" {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.New(apperror.CodeFailedPrecondition, "supervisor turn has no pending input")
	}
	userMessage, err := saveSessionMessageTx(ctx, tx, session.NewMessage(run.SessionID, "user", current.PendingInput))
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	assistantMessage, err := saveSessionMessageTx(ctx, tx, session.NewMessage(run.SessionID, "assistant", response.Text))
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.PolicyDecisionEvent, "policy", checkpoint.AttemptID, map[string]any{
		"context": "supervisor_assistant_response", "allowed": decision.Allowed,
		"needs_approval": decision.NeedsApproval, "risk": decision.Risk, "reason": decision.Reason,
	}); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.AgentTurnCompletedEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"user_message_id": userMessage.ID, "assistant_message_id": assistantMessage.ID,
		"provider": response.Provider, "model": response.Model, "usage": response.Usage,
		"lifecycle_action": action.Kind,
	}); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	current.NextTurn++
	current.InputTokens, err = supervisorAddCounter(current.InputTokens, inputTokens, "input token")
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	current.OutputTokens, err = supervisorAddCounter(current.OutputTokens, outputTokens, "output token")
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	current.TotalTokens, err = supervisorAddCounter(current.TotalTokens, totalTokens, "total token")
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	current.ExecutionMillis, err = supervisorAddCounter(current.ExecutionMillis, elapsedMillis, "execution time")
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	current.Phase = supervisorPhaseForAction(action.Kind)
	current.AttemptID = ""
	current.PendingInput = ""
	current.LastError = ""
	current.UpdatedAt = time.Now().UTC()
	switch action.Kind {
	case domain.RootActionFinish:
		if err := transitionSupervisorRunTx(ctx, tx, &run, domain.RunCompleted, action.Summary, current.UpdatedAt); err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
		}
	case domain.RootActionWait:
		if err := transitionSupervisorRunTx(ctx, tx, &run, domain.RunPaused, action.Reason, current.UpdatedAt); err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
		}
	}
	if err := upsertSupervisorCheckpointTx(ctx, tx, current); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorCheckpointedEvent, "run_supervisor", run.ID, map[string]any{
		"phase": current.Phase, "next_turn": current.NextTurn, "completed_turn": checkpoint.NextTurn,
		"input_tokens": current.InputTokens, "output_tokens": current.OutputTokens,
		"total_tokens": current.TotalTokens, "execution_millis": current.ExecutionMillis,
	}); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorActionEvent, "run_supervisor", run.ID, map[string]any{
		"action": action.Kind, "turn": checkpoint.NextTurn, "summary": action.Summary, "reason": action.Reason,
	}); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	if action.Kind == domain.RootActionFinish {
		if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorRunCompletedEvent, "run_supervisor", run.ID, map[string]any{
			"summary": action.Summary, "turns_completed": current.NextTurn - 1,
			"input_tokens": current.InputTokens, "output_tokens": current.OutputTokens,
			"total_tokens": current.TotalTokens, "execution_millis": current.ExecutionMillis,
		}); err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
		}
	}
	if action.Kind == domain.RootActionWait {
		if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorRunWaitingEvent, "run_supervisor", run.ID, map[string]any{
			"reason": action.Reason, "next_turn": current.NextTurn,
		}); err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	return run, current, session.TurnMessages{User: userMessage, Assistant: assistantMessage}, nil
}

func (s *SQLiteStore) FailSupervisorTurn(ctx context.Context, checkpoint domain.SupervisorCheckpoint, cause string, elapsed time.Duration) (domain.SupervisorCheckpoint, error) {
	if err := checkpoint.Validate(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	elapsedMillis, err := supervisorElapsedMillis(elapsed)
	if err != nil {
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
	current.ExecutionMillis, err = supervisorAddCounter(current.ExecutionMillis, elapsedMillis, "execution time")
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
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
		"execution_millis": current.ExecutionMillis,
	}); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	return current, nil
}

func (s *SQLiteStore) FinalizeSupervisorRun(ctx context.Context, runID string, target domain.RunStatus, summary string) (domain.Run, domain.SupervisorCheckpoint, error) {
	if target != domain.RunCompleted && target != domain.RunFailed {
		return domain.Run{}, domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeInvalidArgument, "supervisor final status must be completed or failed")
	}
	summary = redact.String(strings.TrimSpace(summary))
	if summary == "" {
		if target == domain.RunCompleted {
			summary = "run completed"
		} else {
			summary = "run failed"
		}
	}
	runes := []rune(summary)
	if len(runes) > maxSupervisorErrorChars {
		summary = string(runes[:maxSupervisorErrorChars])
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, strings.TrimSpace(runID)))
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	checkpoint, ok, err := getSupervisorCheckpointTx(ctx, tx, run.ID)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	if !ok {
		checkpoint = domain.SupervisorCheckpoint{
			RunID: run.ID, NextTurn: 1, Phase: domain.SupervisorIdle, UpdatedAt: time.Now().UTC(),
		}
	}
	if run.Status == target {
		if err := tx.Commit(); err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, err
		}
		return run, checkpoint, nil
	}
	if run.Terminal() {
		return domain.Run{}, domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict,
			fmt.Sprintf("run %s is already terminal as %s", run.ID, run.Status))
	}
	if checkpoint.Phase == domain.SupervisorTurnStarted {
		return domain.Run{}, domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "cannot finalize while a supervisor turn is started")
	}
	if target == domain.RunCompleted && checkpoint.Phase != domain.SupervisorIdle {
		return domain.Run{}, domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "only an idle supervisor can complete a run")
	}
	if err := transitionSupervisorRunTx(ctx, tx, &run, target, summary, time.Now().UTC()); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	checkpoint.AttemptID = ""
	checkpoint.PendingInput = ""
	checkpoint.UpdatedAt = run.UpdatedAt
	if target == domain.RunCompleted {
		checkpoint.Phase = domain.SupervisorRunCompleted
		checkpoint.LastError = ""
	} else {
		checkpoint.Phase = domain.SupervisorRunFailed
		checkpoint.LastError = summary
	}
	if err := upsertSupervisorCheckpointTx(ctx, tx, checkpoint); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	eventType := events.SupervisorRunCompletedEvent
	if target == domain.RunFailed {
		eventType = events.SupervisorRunFailedEvent
	}
	if err := appendSupervisorEventTx(ctx, tx, run, eventType, "run_supervisor", run.ID, map[string]any{
		"summary": summary, "turns_completed": checkpoint.NextTurn - 1,
		"input_tokens": checkpoint.InputTokens, "output_tokens": checkpoint.OutputTokens,
		"total_tokens": checkpoint.TotalTokens, "execution_millis": checkpoint.ExecutionMillis,
	}); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, err
	}
	return run, checkpoint, nil
}

func getSupervisorCheckpointTx(ctx context.Context, tx *sql.Tx, runID string) (domain.SupervisorCheckpoint, bool, error) {
	checkpoint, err := scanSupervisorCheckpoint(tx.QueryRowContext(ctx, supervisorCheckpointSelect, runID))
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
	checkpoint.PendingInput = redact.String(checkpoint.PendingInput)
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO run_supervisor_checkpoints
		(run_id, next_turn, phase, attempt_id, last_error, pending_input, input_tokens, output_tokens,
		total_tokens, execution_millis, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET next_turn=excluded.next_turn, phase=excluded.phase,
		attempt_id=excluded.attempt_id, last_error=excluded.last_error, pending_input=excluded.pending_input,
		input_tokens=excluded.input_tokens,
		output_tokens=excluded.output_tokens, total_tokens=excluded.total_tokens,
		execution_millis=excluded.execution_millis, updated_at=excluded.updated_at`,
		checkpoint.RunID, checkpoint.NextTurn, checkpoint.Phase, checkpoint.AttemptID,
		checkpoint.LastError, checkpoint.PendingInput, checkpoint.InputTokens, checkpoint.OutputTokens, checkpoint.TotalTokens,
		checkpoint.ExecutionMillis, ts(checkpoint.UpdatedAt))
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
	if err := row.Scan(&checkpoint.RunID, &checkpoint.NextTurn, &phase, &attempt, &lastError, &checkpoint.PendingInput,
		&checkpoint.InputTokens, &checkpoint.OutputTokens, &checkpoint.TotalTokens,
		&checkpoint.ExecutionMillis, &updated); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	checkpoint.Phase = domain.SupervisorPhase(phase)
	checkpoint.AttemptID = attempt.String
	checkpoint.LastError = lastError.String
	checkpoint.UpdatedAt = parseTS(updated)
	return checkpoint, checkpoint.Validate()
}

func supervisorUsage(usage llm.Usage) (int64, int64, int64, error) {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.TotalTokens < 0 {
		return 0, 0, 0, apperror.New(apperror.CodeFailedPrecondition, "provider returned negative token usage")
	}
	input := int64(usage.InputTokens)
	output := int64(usage.OutputTokens)
	total := int64(usage.TotalTokens)
	combined, err := supervisorAddCounter(input, output, "provider token usage")
	if err != nil {
		return 0, 0, 0, err
	}
	if total < combined {
		total = combined
	}
	return input, output, total, nil
}

func normalizeSupervisorInput(input string) (string, error) {
	input = redact.String(strings.TrimSpace(input))
	if len([]rune(input)) > maxSupervisorInputChars {
		return "", apperror.New(apperror.CodeResourceExhausted, "supervisor input exceeds 65536 characters")
	}
	return input, nil
}

func sanitizeRootAction(action domain.RootAction) domain.RootAction {
	action.Version = strings.TrimSpace(action.Version)
	action.Kind = domain.RootActionKind(strings.TrimSpace(string(action.Kind)))
	action.Message = redact.String(strings.TrimSpace(action.Message))
	action.Summary = redact.String(strings.TrimSpace(action.Summary))
	action.Reason = redact.String(strings.TrimSpace(action.Reason))
	return action
}

func supervisorPhaseForAction(kind domain.RootActionKind) domain.SupervisorPhase {
	switch kind {
	case domain.RootActionFinish:
		return domain.SupervisorRunCompleted
	case domain.RootActionWait:
		return domain.SupervisorWaiting
	case domain.RootActionContinue:
		return domain.SupervisorIdle
	default:
		return domain.SupervisorTurnFailed
	}
}

func transitionSupervisorRunTx(ctx context.Context, tx *sql.Tx, run *domain.Run, target domain.RunStatus, reason string, at time.Time) error {
	if run == nil {
		return apperror.New(apperror.CodeFailedPrecondition, "supervisor run is required")
	}
	expected := run.Status
	if err := run.Transition(target, at); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
	}
	configJSON, err := marshalRedactedJSON(run.Config)
	if err != nil {
		return err
	}
	budgetJSON, err := marshalRedactedJSON(run.Budget)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, config_json = ?, budget_json = ?,
		started_at = ?, finished_at = ?, updated_at = ? WHERE id = ? AND status = ?`,
		run.Status, configJSON, budgetJSON, nullableTS(run.StartedAt), nullableTS(run.FinishedAt),
		ts(run.UpdatedAt), run.ID, expected)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeConflict, "run changed before supervisor lifecycle transition")
	}
	statusEvent, err := events.New(run.ID, run.MissionID, events.RunStatusChangedEvent, "run_supervisor", run.ID, map[string]any{
		"from": expected, "to": target, "reason": redact.String(strings.TrimSpace(reason)),
	})
	if err != nil {
		return err
	}
	_, err = insertRunEventTx(ctx, tx, statusEvent)
	return err
}

func supervisorAddCounter(current, delta int64, name string) (int64, error) {
	if current < 0 || delta < 0 {
		return 0, apperror.New(apperror.CodeFailedPrecondition, name+" counter cannot be negative")
	}
	if delta > (1<<63-1)-current {
		return 0, apperror.New(apperror.CodeResourceExhausted, name+" counter overflow")
	}
	return current + delta, nil
}

func supervisorElapsedMillis(elapsed time.Duration) (int64, error) {
	if elapsed < 0 {
		return 0, apperror.New(apperror.CodeInvalidArgument, "supervisor elapsed time cannot be negative")
	}
	millis := elapsed.Milliseconds()
	if elapsed > 0 && millis == 0 {
		millis = 1
	}
	return millis, nil
}
