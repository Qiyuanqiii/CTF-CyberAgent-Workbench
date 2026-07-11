package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/toolgateway"
)

func (s *SQLiteStore) ListSupervisorToolRounds(ctx context.Context,
	checkpoint domain.SupervisorCheckpoint,
) ([]domain.SupervisorToolRound, error) {
	if err := checkpoint.Validate(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		r.run_id, r.turn, r.attempt_id, r.round, r.model_attempt, r.created_at, r.completed_at,
		c.run_id, c.turn, c.attempt_id, c.round, c.position, c.model_attempt, c.call_id, c.tool_name,
		c.payload_json, c.status, c.result_json, c.error_code, c.created_at, c.completed_at
		FROM run_supervisor_tool_rounds r
		JOIN run_supervisor_tool_calls c
			ON c.run_id = r.run_id AND c.turn = r.turn AND c.attempt_id = r.attempt_id AND c.round = r.round
		WHERE r.run_id = ? AND r.turn = ? AND r.attempt_id = ? ORDER BY r.round, c.position`,
		checkpoint.RunID, checkpoint.NextTurn, checkpoint.AttemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rounds := make([]domain.SupervisorToolRound, 0, domain.MaxSupervisorToolRounds)
	for rows.Next() {
		round, call, err := scanSupervisorToolRoundCall(rows)
		if err != nil {
			return nil, err
		}
		if len(rounds) == 0 || rounds[len(rounds)-1].Round != round.Round {
			rounds = append(rounds, round)
		} else if rounds[len(rounds)-1].RunID != round.RunID || rounds[len(rounds)-1].Turn != round.Turn ||
			rounds[len(rounds)-1].AttemptID != round.AttemptID ||
			rounds[len(rounds)-1].ModelAttempt != round.ModelAttempt {
			return nil, apperror.New(apperror.CodeFailedPrecondition,
				"durable supervisor tool round rows are inconsistent")
		}
		rounds[len(rounds)-1].Calls = append(rounds[len(rounds)-1].Calls, call)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(rounds) > domain.MaxSupervisorToolRounds {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"durable supervisor tool round limit was exceeded")
	}
	for index := range rounds {
		if err := rounds[index].Validate(); err != nil {
			return nil, apperror.Wrap(apperror.CodeFailedPrecondition,
				"invalid durable supervisor tool round", err)
		}
	}
	return rounds, nil
}

func insertSupervisorToolRoundTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, calls []llm.ToolCall,
) error {
	normalized, err := normalizeSupervisorToolCallsForStore(calls, checkpoint.RunID,
		checkpoint.NextTurn, attempt.ToolRound+1)
	if err != nil {
		return err
	}
	if len(normalized) == 0 {
		return nil
	}
	var previous int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(round), 0) FROM run_supervisor_tool_rounds
		WHERE run_id = ? AND turn = ? AND attempt_id = ?`, checkpoint.RunID, checkpoint.NextTurn,
		checkpoint.AttemptID).Scan(&previous); err != nil {
		return err
	}
	round := previous + 1
	if round > domain.MaxSupervisorToolRounds || attempt.ToolRound != previous {
		return apperror.New(apperror.CodeResourceExhausted,
			"supervisor structured tool round limit was exhausted")
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_supervisor_tool_rounds
		(run_id, turn, attempt_id, round, model_attempt, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL)`, checkpoint.RunID, checkpoint.NextTurn, checkpoint.AttemptID,
		round, attempt.Number, ts(now)); err != nil {
		return err
	}
	names := make([]string, 0, len(normalized))
	for index, call := range normalized {
		if _, err := tx.ExecContext(ctx, `INSERT INTO run_supervisor_tool_calls
			(run_id, turn, attempt_id, round, position, model_attempt, call_id, tool_name, payload_json,
			 status, result_json, error_code, created_at, completed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, NULL)`,
			checkpoint.RunID, checkpoint.NextTurn, checkpoint.AttemptID, round, index+1, attempt.Number,
			call.ID, call.Name, string(call.Arguments), domain.SupervisorToolPending, ts(now)); err != nil {
			return err
		}
		names = append(names, call.Name)
	}
	return appendSupervisorEventTx(ctx, tx, run, events.SupervisorToolBatchEvent, "run_supervisor",
		supervisorToolRoundSubject(checkpoint, round), map[string]any{
			"turn": checkpoint.NextTurn, "attempt_id": checkpoint.AttemptID, "round": round,
			"model_attempt": attempt.Number, "tool_count": len(normalized), "tools": names,
		})
}

func normalizeSupervisorToolCallsForStore(calls []llm.ToolCall, runID string, turn int,
	round int,
) ([]llm.ToolCall, error) {
	if len(calls) == 0 || len(calls) > domain.MaxSupervisorToolCallsPerRound {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			fmt.Sprintf("supervisor tool batch must contain 1 to %d calls", domain.MaxSupervisorToolCallsPerRound))
	}
	normalized, err := llm.NormalizeToolCalls(calls)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, "invalid supervisor tool batch", err)
	}
	for index := range normalized {
		name := toolgateway.ToolName(normalized[index].Name)
		safe, err := toolgateway.NormalizeStructuredMemoryPayload(name, normalized[index].Arguments)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInvalidArgument,
				"invalid supervisor structured tool payload", err)
		}
		if len(safe) > domain.MaxSupervisorToolPayloadBytes {
			return nil, apperror.New(apperror.CodeResourceExhausted,
				"supervisor tool payload exceeds its durable limit")
		}
		normalized[index].Arguments = append(json.RawMessage(nil), safe...)
		operationKey := runmutation.SupervisorToolOperationKey(runID, turn, normalized[index].Name, string(safe))
		expectedID, err := runmutation.SupervisorToolCallID(operationKey, round)
		if err != nil || normalized[index].ID != expectedID {
			return nil, apperror.New(apperror.CodeInvalidArgument,
				"supervisor tool call id does not match its normalized intent")
		}
	}
	return normalized, nil
}

func (s *SQLiteStore) RecordSupervisorToolResult(ctx context.Context, checkpoint domain.SupervisorCheckpoint,
	result domain.SupervisorToolResult,
) (domain.SupervisorToolCall, bool, error) {
	if err := checkpoint.Validate(); err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return domain.SupervisorToolCall{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"only a started supervisor turn can record a tool result")
	}
	if err := result.Validate(); err != nil {
		return domain.SupervisorToolCall{}, false, apperror.Wrap(apperror.CodeInvalidArgument,
			"invalid supervisor tool result", err)
	}
	safeResult, err := redactJSONPayload(result.ResultJSON)
	if err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	result.ResultJSON = safeResult
	result.ErrorCode = strings.TrimSpace(result.ErrorCode)
	if err := result.Validate(); err != nil {
		return domain.SupervisorToolCall{}, false, apperror.Wrap(apperror.CodeInvalidArgument,
			"redacted supervisor tool result is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	run, current, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
	if err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	call, err := getSupervisorToolCallTx(ctx, tx, current, result.CallID)
	if err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	if call.Status.Terminal() {
		if call.Status != result.Status || call.ResultJSON != result.ResultJSON || call.ErrorCode != result.ErrorCode {
			return domain.SupervisorToolCall{}, false, apperror.New(apperror.CodeConflict,
				"supervisor tool result replay does not match its durable value")
		}
		if err := tx.Commit(); err != nil {
			return domain.SupervisorToolCall{}, false, err
		}
		return call, true, nil
	}
	completedAt := result.CompletedAt.UTC()
	update, err := tx.ExecContext(ctx, `UPDATE run_supervisor_tool_calls
		SET status = ?, result_json = ?, error_code = ?, completed_at = ?
		WHERE run_id = ? AND turn = ? AND attempt_id = ? AND call_id = ? AND status = ?`,
		result.Status, result.ResultJSON, result.ErrorCode, ts(completedAt), current.RunID, current.NextTurn,
		current.AttemptID, result.CallID, domain.SupervisorToolPending)
	if err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	rows, err := update.RowsAffected()
	if err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	if rows != 1 {
		return domain.SupervisorToolCall{}, false, apperror.New(apperror.CodeConflict,
			"supervisor tool call changed before its result was recorded")
	}
	call.Status = result.Status
	call.ResultJSON = result.ResultJSON
	call.ErrorCode = result.ErrorCode
	call.CompletedAt = &completedAt
	if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorToolResultEvent, "run_supervisor",
		call.CallID, map[string]any{
			"turn": call.Turn, "attempt_id": call.AttemptID, "round": call.Round,
			"position": call.Position, "tool": call.ToolName, "status": call.Status,
			"error_code": call.ErrorCode,
		}); err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_supervisor_tool_calls
		WHERE run_id = ? AND turn = ? AND attempt_id = ? AND round = ? AND status = ?`,
		call.RunID, call.Turn, call.AttemptID, call.Round, domain.SupervisorToolPending).Scan(&pending); err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	if pending == 0 {
		updated, err := tx.ExecContext(ctx, `UPDATE run_supervisor_tool_rounds SET completed_at = ?
			WHERE run_id = ? AND turn = ? AND attempt_id = ? AND round = ? AND completed_at IS NULL`,
			ts(completedAt), call.RunID, call.Turn, call.AttemptID, call.Round)
		if err != nil {
			return domain.SupervisorToolCall{}, false, err
		}
		changed, err := updated.RowsAffected()
		if err != nil {
			return domain.SupervisorToolCall{}, false, err
		}
		if changed == 1 {
			if err := appendSupervisorEventTx(ctx, tx, run, events.SupervisorToolCompleteEvent,
				"run_supervisor", supervisorToolRoundSubject(current, call.Round), map[string]any{
					"turn": call.Turn, "attempt_id": call.AttemptID, "round": call.Round,
				}); err != nil {
				return domain.SupervisorToolCall{}, false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.SupervisorToolCall{}, false, err
	}
	return call, false, nil
}

func getSupervisorToolCallTx(ctx context.Context, tx *sql.Tx, checkpoint domain.SupervisorCheckpoint,
	callID string,
) (domain.SupervisorToolCall, error) {
	return scanSupervisorToolCall(tx.QueryRowContext(ctx, `SELECT run_id, turn, attempt_id, round, position,
		model_attempt, call_id, tool_name, payload_json, status, result_json, error_code, created_at, completed_at
		FROM run_supervisor_tool_calls
		WHERE run_id = ? AND turn = ? AND attempt_id = ? AND call_id = ?`, checkpoint.RunID,
		checkpoint.NextTurn, checkpoint.AttemptID, strings.TrimSpace(callID)))
}

func scanSupervisorToolRoundCall(row scanner) (domain.SupervisorToolRound, domain.SupervisorToolCall, error) {
	var round domain.SupervisorToolRound
	var roundCreatedAt string
	var roundCompletedAt sql.NullString
	var call domain.SupervisorToolCall
	var callStatus string
	var callCreatedAt string
	var callCompletedAt sql.NullString
	if err := row.Scan(&round.RunID, &round.Turn, &round.AttemptID, &round.Round, &round.ModelAttempt,
		&roundCreatedAt, &roundCompletedAt, &call.RunID, &call.Turn, &call.AttemptID, &call.Round,
		&call.Position, &call.ModelAttempt, &call.CallID, &call.ToolName, &call.PayloadJSON, &callStatus,
		&call.ResultJSON, &call.ErrorCode, &callCreatedAt, &callCompletedAt); err != nil {
		return domain.SupervisorToolRound{}, domain.SupervisorToolCall{}, err
	}
	round.CreatedAt = parseTS(roundCreatedAt)
	if roundCompletedAt.Valid {
		value := parseTS(roundCompletedAt.String)
		round.CompletedAt = &value
	}
	call.Status = domain.SupervisorToolCallStatus(callStatus)
	call.CreatedAt = parseTS(callCreatedAt)
	if callCompletedAt.Valid {
		value := parseTS(callCompletedAt.String)
		call.CompletedAt = &value
	}
	if err := call.Validate(); err != nil {
		return domain.SupervisorToolRound{}, domain.SupervisorToolCall{},
			apperror.Wrap(apperror.CodeFailedPrecondition, "invalid durable supervisor tool call", err)
	}
	return round, call, nil
}

func scanSupervisorToolCall(row scanner) (domain.SupervisorToolCall, error) {
	var call domain.SupervisorToolCall
	var status string
	var createdAt string
	var completedAt sql.NullString
	if err := row.Scan(&call.RunID, &call.Turn, &call.AttemptID, &call.Round, &call.Position,
		&call.ModelAttempt, &call.CallID, &call.ToolName, &call.PayloadJSON, &status, &call.ResultJSON,
		&call.ErrorCode, &createdAt, &completedAt); err != nil {
		return domain.SupervisorToolCall{}, err
	}
	call.Status = domain.SupervisorToolCallStatus(status)
	call.CreatedAt = parseTS(createdAt)
	if completedAt.Valid {
		value := parseTS(completedAt.String)
		call.CompletedAt = &value
	}
	if err := call.Validate(); err != nil {
		return domain.SupervisorToolCall{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"invalid durable supervisor tool call", err)
	}
	return call, nil
}

func requireSupervisorToolsReadyTx(ctx context.Context, tx *sql.Tx,
	checkpoint domain.SupervisorCheckpoint,
) error {
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_supervisor_tool_calls
		WHERE run_id = ? AND turn = ? AND attempt_id = ? AND status = ?`, checkpoint.RunID,
		checkpoint.NextTurn, checkpoint.AttemptID, domain.SupervisorToolPending).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return apperror.New(apperror.CodeFailedPrecondition,
			"supervisor turn has unresolved structured tool calls")
	}
	var payloadJSON string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id LIKE ?
		ORDER BY sequence DESC LIMIT 1`, checkpoint.RunID, events.ModelCompletedEvent, "model_gateway",
		supervisorModelSubjectPrefix(checkpoint)+"%").Scan(&payloadJSON); err != nil {
		return err
	}
	var payload struct {
		ModelAttempt int `json:"model_attempt"`
		ToolCount    int `json:"tool_call_count"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"invalid durable completed model payload", err)
	}
	if payload.ModelAttempt <= 0 || payload.ToolCount != 0 {
		return apperror.New(apperror.CodeFailedPrecondition,
			"latest completed model response is not a root lifecycle action")
	}
	return nil
}

func supervisorToolRoundSubject(checkpoint domain.SupervisorCheckpoint, round int) string {
	return fmt.Sprintf("%s/tool/%d", checkpoint.AttemptID, round)
}
