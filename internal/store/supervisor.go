package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
	pending_input, repair_phase, repair_reason, input_tokens, output_tokens, total_tokens, execution_millis, updated_at
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
	if checkpoint.Phase == domain.SupervisorTurnFailed && pendingInput == "" {
		pendingInput = checkpoint.PendingInput
	}

	checkpoint.Phase = domain.SupervisorTurnStarted
	checkpoint.AttemptID = idgen.New("attempt")
	checkpoint.PendingInput = pendingInput
	checkpoint.RepairPhase = domain.ProtocolRepairNone
	checkpoint.RepairReason = ""
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

func (s *SQLiteStore) NextSupervisorModelAttempt(ctx context.Context, checkpoint domain.SupervisorCheckpoint, protocolRepair int) (int, int, error) {
	if err := checkpoint.Validate(); err != nil {
		return 0, 0, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return 0, 0, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can call a model")
	}
	if protocolRepair < 0 || protocolRepair > 1 {
		return 0, 0, apperror.New(apperror.CodeInvalidArgument, "protocol repair number must be zero or one")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	_, current, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
	if err != nil {
		return 0, 0, err
	}
	if protocolRepair == 0 && current.RepairPhase != domain.ProtocolRepairNone {
		return 0, 0, apperror.New(apperror.CodeConflict, "primary model attempt cannot run during protocol repair")
	}
	if protocolRepair == 1 && current.RepairPhase != domain.ProtocolRepairPending {
		return 0, 0, apperror.New(apperror.CodeFailedPrecondition, "protocol repair is not pending")
	}
	var totalCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id LIKE ?`, checkpoint.RunID,
		events.ModelStartedEvent, "model_gateway", supervisorModelSubjectPrefix(checkpoint)+"%").Scan(&totalCount); err != nil {
		return 0, 0, err
	}
	transportCount, err := supervisorModelPurposeCountTx(ctx, tx, checkpoint, protocolRepair)
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return totalCount + 1, transportCount + 1, nil
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
	if attempt.StreamEvents != 0 || attempt.StreamBytes != 0 {
		return false, apperror.New(apperror.CodeInvalidArgument, "started model attempt cannot have stream counters")
	}
	if err := attempt.ValidateStarted(); err != nil {
		return false, apperror.Wrap(apperror.CodeInvalidArgument, "invalid started model attempt", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	run, current, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
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
	transportCount, err := supervisorModelPurposeCountTx(ctx, tx, checkpoint, attempt.ProtocolRepair)
	if err != nil {
		return false, err
	}
	if attempt.TransportNumber() != transportCount+1 {
		return false, apperror.New(apperror.CodeConflict, "model transport attempt number is not the next durable attempt")
	}
	if attempt.ProtocolRepair == 0 && current.RepairPhase != domain.ProtocolRepairNone {
		return false, apperror.New(apperror.CodeConflict, "primary model attempt cannot run during protocol repair")
	}
	if attempt.ProtocolRepair == 1 && current.RepairPhase != domain.ProtocolRepairPending {
		return false, apperror.New(apperror.CodeFailedPrecondition, "protocol repair is not pending")
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelStartedEvent, "model_gateway", subject, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "transport_attempt": attempt.TransportNumber(),
		"max_attempts": attempt.MaxAttempts, "protocol_repair": attempt.ProtocolRepair,
		"provider": attempt.Provider, "model": attempt.Model,
	}); err != nil {
		return false, err
	}
	if attempt.ProtocolRepair == 1 && attempt.TransportNumber() == 1 {
		if err := appendSupervisorEventTx(ctx, tx, run, events.ProtocolRepairStartedEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
			"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID, "protocol_repair": 1,
			"model_attempt": attempt.Number,
		}); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) RecordSupervisorModelCancelRequested(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, reason string) (bool, error) {
	if err := checkpoint.Validate(); err != nil {
		return false, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return false, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can cancel a model call")
	}
	attempt = sanitizeModelAttempt(attempt)
	if attempt.Outcome != "" || strings.TrimSpace(attempt.ErrorText) != "" || attempt.StreamEvents != 0 || attempt.StreamBytes != 0 {
		return false, apperror.New(apperror.CodeInvalidArgument, "model cancellation requires the original started attempt")
	}
	if err := attempt.ValidateStarted(); err != nil {
		return false, apperror.Wrap(apperror.CodeInvalidArgument, "invalid model cancellation attempt", err)
	}
	reason = sanitizeSupervisorText(reason)
	if reason == "" {
		reason = "active model call cancellation requested"
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
	if err := requireSupervisorModelStartedMatchTx(ctx, tx, run.ID, subject, attempt); err != nil {
		return false, err
	}
	exists, err := supervisorModelEventExistsTx(ctx, tx, run.ID, events.ModelCancelRequestedEvent, subject)
	if err != nil {
		return false, err
	}
	if exists {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	for _, terminalType := range []string{events.ModelCompletedEvent, events.ModelFailedEvent} {
		exists, err := supervisorModelEventExistsTx(ctx, tx, run.ID, terminalType, subject)
		if err != nil {
			return false, err
		}
		if exists {
			return false, apperror.New(apperror.CodeFailedPrecondition, "model attempt is already terminal")
		}
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelCancelRequestedEvent, "run_supervisor", subject, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "transport_attempt": attempt.TransportNumber(),
		"protocol_repair": attempt.ProtocolRepair, "provider": attempt.Provider, "model": attempt.Model,
		"reason": reason,
	}); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) RecordSupervisorModelDelta(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, delta llm.ModelDelta) (bool, error) {
	if err := checkpoint.Validate(); err != nil {
		return false, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return false, apperror.New(apperror.CodeFailedPrecondition, "only a started supervisor turn can record model deltas")
	}
	attempt = sanitizeModelAttempt(attempt)
	if attempt.Outcome != "" || strings.TrimSpace(attempt.ErrorText) != "" || attempt.StreamEvents != 0 || attempt.StreamBytes != 0 {
		return false, apperror.New(apperror.CodeInvalidArgument, "model delta requires the original started attempt")
	}
	if err := attempt.ValidateStarted(); err != nil {
		return false, apperror.Wrap(apperror.CodeInvalidArgument, "invalid model delta attempt", err)
	}
	if err := delta.Validate(llm.MaxModelDeltaEvents, llm.MaxModelOutputBytes); err != nil {
		return false, apperror.Wrap(apperror.CodeInvalidArgument, "invalid model delta", err)
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
	modelSubject := supervisorModelSubject(checkpoint, attempt.Number)
	if err := requireSupervisorModelStartedMatchTx(ctx, tx, run.ID, modelSubject, attempt); err != nil {
		return false, err
	}
	for _, terminalType := range []string{events.ModelCompletedEvent, events.ModelFailedEvent} {
		exists, err := supervisorModelEventExistsTx(ctx, tx, run.ID, terminalType, modelSubject)
		if err != nil {
			return false, err
		}
		if exists {
			return false, apperror.New(apperror.CodeConflict, "model attempt is already terminal")
		}
	}
	deltaSubject := supervisorModelDeltaSubject(checkpoint, attempt.Number, delta.Sequence)
	exists, err := supervisorModelEventExistsTx(ctx, tx, run.ID, events.ModelDeltaEvent, deltaSubject)
	if err != nil {
		return false, err
	}
	if exists {
		var payloadJSON string
		if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM run_events
			WHERE run_id = ? AND type = ? AND source = ? AND subject_id = ?`, run.ID,
			events.ModelDeltaEvent, "model_gateway", deltaSubject).Scan(&payloadJSON); err != nil {
			return false, err
		}
		var payload supervisorModelDeltaPayload
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return false, apperror.Wrap(apperror.CodeFailedPrecondition, "invalid durable model delta payload", err)
		}
		if payload.Sequence != delta.Sequence || payload.ChunkCount != delta.ChunkCount || payload.ByteCount != delta.ByteCount ||
			payload.TotalBytes != delta.TotalBytes || payload.Done != delta.Done {
			return false, apperror.New(apperror.CodeConflict, "model delta replay does not match its durable event")
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	state, err := supervisorModelDeltaStateTx(ctx, tx, run.ID, modelSubject)
	if err != nil {
		return false, err
	}
	if state.Done {
		return false, apperror.New(apperror.CodeConflict, "model stream was already completed")
	}
	if delta.Sequence != state.Count+1 || delta.TotalBytes != state.TotalBytes+delta.ByteCount {
		return false, apperror.New(apperror.CodeConflict, "model delta is not the next durable stream update")
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelDeltaEvent, "model_gateway", deltaSubject, map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "transport_attempt": attempt.TransportNumber(),
		"protocol_repair": attempt.ProtocolRepair, "delta_sequence": delta.Sequence,
		"chunk_count": delta.ChunkCount, "delta_bytes": delta.ByteCount,
		"total_bytes": delta.TotalBytes, "done": delta.Done,
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
		"model_attempt": attempt.Number, "transport_attempt": attempt.TransportNumber(),
		"max_attempts": attempt.MaxAttempts, "protocol_repair": attempt.ProtocolRepair,
		"provider": attempt.Provider, "model": attempt.Model, "outcome": attempt.Outcome,
		"elapsed_millis": attempt.Elapsed.Milliseconds(), "usage": response.Usage,
		"stream_events": attempt.StreamEvents, "stream_bytes": attempt.StreamBytes,
	}
	return s.recordSupervisorModelTerminal(ctx, checkpoint, attempt, events.ModelCompletedEvent, payload, supervisorModelTerminalOptions{Usage: &response.Usage})
}

func (s *SQLiteStore) RecordSupervisorModelFailed(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt) (domain.SupervisorCheckpoint, error) {
	attempt = sanitizeModelAttempt(attempt)
	if err := attempt.ValidateFailed(); err != nil {
		return domain.SupervisorCheckpoint{}, apperror.Wrap(apperror.CodeInvalidArgument, "invalid failed model attempt", err)
	}
	payload := map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "transport_attempt": attempt.TransportNumber(),
		"max_attempts": attempt.MaxAttempts, "protocol_repair": attempt.ProtocolRepair,
		"provider": attempt.Provider, "model": attempt.Model, "outcome": attempt.Outcome,
		"error": attempt.ErrorText, "elapsed_millis": attempt.Elapsed.Milliseconds(),
		"retry_after_millis": attempt.RetryAfter.Milliseconds(), "retry_planned": attempt.RetryPlanned,
		"stream_events": attempt.StreamEvents, "stream_bytes": attempt.StreamBytes,
	}
	return s.recordSupervisorModelTerminal(ctx, checkpoint, attempt, events.ModelFailedEvent, payload, supervisorModelTerminalOptions{})
}

func (s *SQLiteStore) RecordSupervisorProtocolFailure(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, response llm.ChatResponse, reason string, requestRepair bool) (domain.SupervisorCheckpoint, error) {
	reason = sanitizeSupervisorText(reason)
	if reason == "" {
		reason = "provider returned an invalid root lifecycle response"
	}
	attempt = sanitizeModelAttempt(attempt)
	attempt.Outcome = llm.OutcomeInvalidResponse
	attempt.ErrorText = reason
	attempt.RetryAfter = 0
	attempt.RetryPlanned = false
	if err := attempt.ValidateFailed(); err != nil {
		return domain.SupervisorCheckpoint{}, apperror.Wrap(apperror.CodeInvalidArgument, "invalid protocol failure attempt", err)
	}
	if _, _, _, err := supervisorUsage(response.Usage); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	phase := domain.ProtocolRepairExhausted
	eventType := events.ProtocolRepairFailedEvent
	if requestRepair {
		if attempt.ProtocolRepair != 0 {
			return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeInvalidArgument, "only a primary model response can request protocol repair")
		}
		phase = domain.ProtocolRepairPending
		eventType = events.ProtocolRepairRequestedEvent
	} else if attempt.ProtocolRepair != 1 {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeInvalidArgument, "only a repair response can exhaust protocol repair")
	}
	payload := map[string]any{
		"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
		"model_attempt": attempt.Number, "transport_attempt": attempt.TransportNumber(),
		"max_attempts": attempt.MaxAttempts, "protocol_repair": attempt.ProtocolRepair,
		"provider": attempt.Provider, "model": attempt.Model, "outcome": attempt.Outcome,
		"error": attempt.ErrorText, "elapsed_millis": attempt.Elapsed.Milliseconds(),
		"retry_after_millis": 0, "retry_planned": false, "usage": response.Usage,
		"stream_events": attempt.StreamEvents, "stream_bytes": attempt.StreamBytes,
	}
	return s.recordSupervisorModelTerminal(ctx, checkpoint, attempt, events.ModelFailedEvent, payload, supervisorModelTerminalOptions{
		Usage: &response.Usage, RepairPhase: phase, RepairReason: reason, RepairEvent: eventType,
	})
}

type supervisorModelTerminalOptions struct {
	Usage        *llm.Usage
	RepairPhase  domain.ProtocolRepairPhase
	RepairReason string
	RepairEvent  string
}

func (s *SQLiteStore) recordSupervisorModelTerminal(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, eventType string, payload map[string]any, options supervisorModelTerminalOptions) (domain.SupervisorCheckpoint, error) {
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
	if err := requireSupervisorModelStartedMatchTx(ctx, tx, run.ID, subject, attempt); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	deltaState, err := supervisorModelDeltaStateTx(ctx, tx, run.ID, subject)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if attempt.StreamEvents != deltaState.Count || attempt.StreamBytes != deltaState.TotalBytes {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "model terminal stream counters do not match durable deltas")
	}
	if eventType == events.ModelCompletedEvent && deltaState.Count > 0 && !deltaState.Done {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "completed model attempt requires a completed stream")
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
	if attempt.ProtocolRepair == 0 && current.RepairPhase != domain.ProtocolRepairNone {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "primary model attempt cannot finish during protocol repair")
	}
	if attempt.ProtocolRepair == 1 && current.RepairPhase != domain.ProtocolRepairPending {
		return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "protocol repair is not pending")
	}
	if options.RepairPhase != domain.ProtocolRepairNone {
		switch options.RepairPhase {
		case domain.ProtocolRepairPending:
			if current.RepairPhase != domain.ProtocolRepairNone {
				return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeConflict, "protocol repair was already requested")
			}
		case domain.ProtocolRepairExhausted:
			if current.RepairPhase != domain.ProtocolRepairPending {
				return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeFailedPrecondition, "protocol repair is not pending")
			}
		default:
			return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeInvalidArgument, "invalid protocol repair transition")
		}
		if strings.TrimSpace(options.RepairReason) == "" || strings.TrimSpace(options.RepairEvent) == "" {
			return domain.SupervisorCheckpoint{}, apperror.New(apperror.CodeInvalidArgument, "protocol repair transition requires a reason and event")
		}
	}
	elapsedMillis, err := supervisorElapsedMillis(attempt.Elapsed)
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	current.ExecutionMillis, err = supervisorAddCounter(current.ExecutionMillis, elapsedMillis, "execution time")
	if err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if options.Usage != nil {
		inputTokens, outputTokens, totalTokens, err := supervisorUsage(*options.Usage)
		if err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
		current.InputTokens, err = supervisorAddCounter(current.InputTokens, inputTokens, "input token")
		if err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
		current.OutputTokens, err = supervisorAddCounter(current.OutputTokens, outputTokens, "output token")
		if err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
		current.TotalTokens, err = supervisorAddCounter(current.TotalTokens, totalTokens, "total token")
		if err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
	}
	if options.RepairPhase != domain.ProtocolRepairNone {
		current.RepairPhase = options.RepairPhase
		current.RepairReason = sanitizeSupervisorText(options.RepairReason)
	}
	current.UpdatedAt = time.Now().UTC()
	if err := upsertSupervisorCheckpointTx(ctx, tx, current); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, eventType, "model_gateway", subject, payload); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	if options.RepairPhase != domain.ProtocolRepairNone {
		if err := appendSupervisorEventTx(ctx, tx, run, options.RepairEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
			"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID,
			"protocol_repair": 1, "model_attempt": attempt.Number, "reason": current.RepairReason,
		}); err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
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

func supervisorModelPurposeCountTx(ctx context.Context, tx *sql.Tx, checkpoint domain.SupervisorCheckpoint, protocolRepair int) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT payload_json FROM run_events WHERE run_id = ? AND type = ? AND source = ?
		AND subject_id LIKE ? ORDER BY sequence`, checkpoint.RunID, events.ModelStartedEvent, "model_gateway",
		supervisorModelSubjectPrefix(checkpoint)+"%")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var payloadJSON string
		if err := rows.Scan(&payloadJSON); err != nil {
			return 0, err
		}
		payload, err := parseSupervisorModelStartedPayload(payloadJSON)
		if err != nil {
			return 0, err
		}
		if payload.protocolRepair() == protocolRepair {
			count++
		}
	}
	return count, rows.Err()
}

type supervisorModelStartedPayload struct {
	ModelAttempt     int    `json:"model_attempt"`
	TransportAttempt *int   `json:"transport_attempt"`
	MaxAttempts      int    `json:"max_attempts"`
	ProtocolRepair   *int   `json:"protocol_repair"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
}

func (p supervisorModelStartedPayload) transportAttempt() int {
	if p.TransportAttempt != nil {
		return *p.TransportAttempt
	}
	return p.ModelAttempt
}

func (p supervisorModelStartedPayload) protocolRepair() int {
	if p.ProtocolRepair != nil {
		return *p.ProtocolRepair
	}
	return 0
}

func parseSupervisorModelStartedPayload(payloadJSON string) (supervisorModelStartedPayload, error) {
	var payload supervisorModelStartedPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return supervisorModelStartedPayload{}, apperror.Wrap(apperror.CodeFailedPrecondition, "invalid durable model start payload", err)
	}
	if payload.ModelAttempt <= 0 || payload.MaxAttempts <= 0 || strings.TrimSpace(payload.Provider) == "" || strings.TrimSpace(payload.Model) == "" {
		return supervisorModelStartedPayload{}, apperror.New(apperror.CodeFailedPrecondition, "incomplete durable model start payload")
	}
	if payload.transportAttempt() <= 0 || payload.transportAttempt() > payload.MaxAttempts || payload.protocolRepair() < 0 || payload.protocolRepair() > 1 {
		return supervisorModelStartedPayload{}, apperror.New(apperror.CodeFailedPrecondition, "invalid durable model start counters")
	}
	return payload, nil
}

func requireSupervisorModelStartedMatchTx(ctx context.Context, tx *sql.Tx, runID string, subject string, attempt llm.ModelAttempt) error {
	var payloadJSON string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id = ?`, runID, events.ModelStartedEvent, "model_gateway", subject).Scan(&payloadJSON); err != nil {
		return err
	}
	payload, err := parseSupervisorModelStartedPayload(payloadJSON)
	if err != nil {
		return err
	}
	if payload.ModelAttempt != attempt.Number || payload.transportAttempt() != attempt.TransportNumber() ||
		payload.MaxAttempts != attempt.MaxAttempts || payload.protocolRepair() != attempt.ProtocolRepair ||
		payload.Provider != attempt.Provider || payload.Model != attempt.Model {
		return apperror.New(apperror.CodeConflict, "model terminal metadata does not match its durable start event")
	}
	return nil
}

func supervisorModelSubjectPrefix(checkpoint domain.SupervisorCheckpoint) string {
	return checkpoint.AttemptID + "/model/"
}

func supervisorModelSubject(checkpoint domain.SupervisorCheckpoint, number int) string {
	return fmt.Sprintf("%s%d", supervisorModelSubjectPrefix(checkpoint), number)
}

func supervisorModelDeltaSubject(checkpoint domain.SupervisorCheckpoint, number int, sequence int) string {
	return fmt.Sprintf("%s/delta/%d", supervisorModelSubject(checkpoint, number), sequence)
}

type supervisorModelDeltaPayload struct {
	Sequence   int  `json:"delta_sequence"`
	ChunkCount int  `json:"chunk_count"`
	ByteCount  int  `json:"delta_bytes"`
	TotalBytes int  `json:"total_bytes"`
	Done       bool `json:"done"`
}

type supervisorModelDeltaState struct {
	Count      int
	TotalBytes int
	Done       bool
}

func supervisorModelDeltaStateTx(ctx context.Context, tx *sql.Tx, runID string, modelSubject string) (supervisorModelDeltaState, error) {
	rows, err := tx.QueryContext(ctx, `SELECT payload_json FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id LIKE ? ORDER BY sequence`,
		runID, events.ModelDeltaEvent, "model_gateway", modelSubject+"/delta/%")
	if err != nil {
		return supervisorModelDeltaState{}, err
	}
	defer rows.Close()
	state := supervisorModelDeltaState{}
	for rows.Next() {
		var payloadJSON string
		if err := rows.Scan(&payloadJSON); err != nil {
			return supervisorModelDeltaState{}, err
		}
		var payload supervisorModelDeltaPayload
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return supervisorModelDeltaState{}, apperror.Wrap(apperror.CodeFailedPrecondition, "invalid durable model delta payload", err)
		}
		delta := llm.ModelDelta{
			Sequence: payload.Sequence, ChunkCount: payload.ChunkCount, ByteCount: payload.ByteCount,
			TotalBytes: payload.TotalBytes, Done: payload.Done,
		}
		if err := delta.Validate(llm.MaxModelDeltaEvents, llm.MaxModelOutputBytes); err != nil {
			return supervisorModelDeltaState{}, apperror.Wrap(apperror.CodeFailedPrecondition, "invalid durable model delta counters", err)
		}
		if payload.Sequence != state.Count+1 || payload.ChunkCount < 0 || payload.ByteCount < 0 ||
			payload.TotalBytes != state.TotalBytes+payload.ByteCount || state.Done {
			return supervisorModelDeltaState{}, apperror.New(apperror.CodeFailedPrecondition, "inconsistent durable model delta sequence")
		}
		state.Count++
		state.TotalBytes = payload.TotalBytes
		state.Done = payload.Done
	}
	if err := rows.Err(); err != nil {
		return supervisorModelDeltaState{}, err
	}
	if state.Count > llm.MaxModelDeltaEvents || state.TotalBytes > llm.MaxModelOutputBytes {
		return supervisorModelDeltaState{}, apperror.New(apperror.CodeFailedPrecondition, "durable model delta limits were exceeded")
	}
	return state, nil
}

func sanitizeModelAttempt(attempt llm.ModelAttempt) llm.ModelAttempt {
	attempt.Provider = redact.String(strings.TrimSpace(attempt.Provider))
	attempt.Model = redact.String(strings.TrimSpace(attempt.Model))
	attempt.ErrorText = sanitizeSupervisorText(attempt.ErrorText)
	return attempt
}

func sanitizeSupervisorText(value string) string {
	value = redact.String(strings.TrimSpace(value))
	runes := []rune(value)
	if len(runes) > maxSupervisorErrorChars {
		value = string(runes[:maxSupervisorErrorChars])
	}
	return value
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
	if _, _, _, err := supervisorUsage(response.Usage); err != nil {
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
	if current.RepairPhase == domain.ProtocolRepairExhausted {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, apperror.New(apperror.CodeFailedPrecondition, "exhausted protocol repair cannot complete a supervisor turn")
	}
	if err := requireLatestSupervisorModelCompletedTx(ctx, tx, run.ID, checkpoint); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	repairCompleted := current.RepairPhase == domain.ProtocolRepairPending
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
	if repairCompleted {
		if err := appendSupervisorEventTx(ctx, tx, run, events.ProtocolRepairCompletedEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
			"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID, "protocol_repair": 1,
		}); err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
		}
	}
	current.NextTurn++
	current.ExecutionMillis, err = supervisorAddCounter(current.ExecutionMillis, elapsedMillis, "execution time")
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, emptyMessages, err
	}
	current.Phase = supervisorPhaseForAction(action.Kind)
	current.AttemptID = ""
	current.PendingInput = ""
	current.RepairPhase = domain.ProtocolRepairNone
	current.RepairReason = ""
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

func requireLatestSupervisorModelCompletedTx(ctx context.Context, tx *sql.Tx, runID string, checkpoint domain.SupervisorCheckpoint) error {
	var eventType string
	if err := tx.QueryRowContext(ctx, `SELECT type FROM run_events
		WHERE run_id = ? AND source = ? AND subject_id LIKE ? AND type IN (?, ?, ?)
		ORDER BY sequence DESC LIMIT 1`, runID, "model_gateway", supervisorModelSubjectPrefix(checkpoint)+"%",
		events.ModelStartedEvent, events.ModelCompletedEvent, events.ModelFailedEvent).Scan(&eventType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperror.New(apperror.CodeFailedPrecondition, "supervisor turn has no completed model attempt")
		}
		return err
	}
	if eventType != events.ModelCompletedEvent {
		return apperror.New(apperror.CodeFailedPrecondition, "latest supervisor model attempt is not completed")
	}
	return nil
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
	repairAborted := current.RepairPhase == domain.ProtocolRepairPending
	repairReason := current.RepairReason
	current.Phase = domain.SupervisorTurnFailed
	current.RepairPhase = domain.ProtocolRepairNone
	current.RepairReason = ""
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
	if repairAborted {
		if err := appendSupervisorEventTx(ctx, tx, run, events.ProtocolRepairFailedEvent, "run_supervisor", checkpoint.AttemptID, map[string]any{
			"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID, "protocol_repair": 1,
			"reason": cause, "requested_reason": repairReason, "stage": "aborted",
		}); err != nil {
			return domain.SupervisorCheckpoint{}, err
		}
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
	checkpoint.RepairPhase = domain.ProtocolRepairNone
	checkpoint.RepairReason = ""
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
	checkpoint.RepairReason = redact.String(checkpoint.RepairReason)
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO run_supervisor_checkpoints
		(run_id, next_turn, phase, attempt_id, last_error, pending_input, repair_phase, repair_reason, input_tokens, output_tokens,
		total_tokens, execution_millis, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET next_turn=excluded.next_turn, phase=excluded.phase,
		attempt_id=excluded.attempt_id, last_error=excluded.last_error, pending_input=excluded.pending_input,
		repair_phase=excluded.repair_phase, repair_reason=excluded.repair_reason,
		input_tokens=excluded.input_tokens,
		output_tokens=excluded.output_tokens, total_tokens=excluded.total_tokens,
		execution_millis=excluded.execution_millis, updated_at=excluded.updated_at`,
		checkpoint.RunID, checkpoint.NextTurn, checkpoint.Phase, checkpoint.AttemptID,
		checkpoint.LastError, checkpoint.PendingInput, checkpoint.RepairPhase, checkpoint.RepairReason,
		checkpoint.InputTokens, checkpoint.OutputTokens, checkpoint.TotalTokens,
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
	var repairPhase string
	var updated string
	if err := row.Scan(&checkpoint.RunID, &checkpoint.NextTurn, &phase, &attempt, &lastError, &checkpoint.PendingInput,
		&repairPhase, &checkpoint.RepairReason,
		&checkpoint.InputTokens, &checkpoint.OutputTokens, &checkpoint.TotalTokens,
		&checkpoint.ExecutionMillis, &updated); err != nil {
		return domain.SupervisorCheckpoint{}, err
	}
	checkpoint.Phase = domain.SupervisorPhase(phase)
	checkpoint.AttemptID = attempt.String
	checkpoint.LastError = lastError.String
	checkpoint.RepairPhase = domain.ProtocolRepairPhase(repairPhase)
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
