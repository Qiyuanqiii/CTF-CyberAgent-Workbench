package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
)

const specialistProtocolRepairSelect = `SELECT agent_attempt_id, run_id, agent_id,
	status, reason, requested_model_attempt, resolved_model_attempt, requested_at, resolved_at
	FROM specialist_protocol_repairs`

func (s *SQLiteStore) GetSpecialistProtocolRepair(ctx context.Context,
	attemptID string,
) (domain.SpecialistProtocolRepair, bool, error) {
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" || len([]rune(attemptID)) > domain.MaxAgentIdentityRunes {
		return domain.SpecialistProtocolRepair{}, false,
			apperror.New(apperror.CodeInvalidArgument, "Specialist repair Attempt id is invalid")
	}
	repair, err := scanSpecialistProtocolRepair(s.db.QueryRowContext(ctx,
		specialistProtocolRepairSelect+` WHERE agent_attempt_id = ?`, attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SpecialistProtocolRepair{}, false, nil
	}
	if err != nil {
		return domain.SpecialistProtocolRepair{}, false, err
	}
	return repair, true, nil
}

func getSpecialistProtocolRepairTx(ctx context.Context, tx *sql.Tx,
	attemptID string,
) (domain.SpecialistProtocolRepair, bool, error) {
	repair, err := scanSpecialistProtocolRepair(tx.QueryRowContext(ctx,
		specialistProtocolRepairSelect+` WHERE agent_attempt_id = ?`, attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SpecialistProtocolRepair{}, false, nil
	}
	if err != nil {
		return domain.SpecialistProtocolRepair{}, false, err
	}
	return repair, true, nil
}

func insertSpecialistProtocolRepairTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	attempt domain.AgentAttempt, modelAttempt int, reason string, now time.Time,
) (domain.SpecialistProtocolRepair, error) {
	reason = normalizeSpecialistRepairReason(reason)
	repair := domain.SpecialistProtocolRepair{
		AgentAttemptID: attempt.ID, RunID: attempt.RunID, AgentID: attempt.AgentID,
		Status: domain.SpecialistRepairPending, Reason: reason,
		RequestedModelAttempt: modelAttempt, RequestedAt: now.UTC(),
	}
	if err := repair.Validate(); err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_protocol_repairs
		(agent_attempt_id, run_id, agent_id, status, reason, requested_model_attempt,
		resolved_model_attempt, requested_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, NULL)`, repair.AgentAttemptID, repair.RunID,
		repair.AgentID, repair.Status, repair.Reason, repair.RequestedModelAttempt,
		ts(repair.RequestedAt)); err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.AgentProtocolRepairRequestedEvent,
		"specialist_runner", attempt.ID, map[string]any{
			"agent_id": attempt.AgentID, "agent_attempt_id": attempt.ID,
			"turn": attempt.Turn, "protocol_repair": 1,
			"model_attempt": modelAttempt, "reason": reason,
		}); err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	return repair, nil
}

func resolveSpecialistProtocolRepairTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	attempt domain.AgentAttempt, status domain.SpecialistProtocolRepairStatus,
	modelAttempt int, now time.Time,
) (domain.SpecialistProtocolRepair, error) {
	if status != domain.SpecialistRepairCompleted && status != domain.SpecialistRepairExhausted {
		return domain.SpecialistProtocolRepair{}, apperror.New(apperror.CodeInvalidArgument,
			"Specialist repair resolution status is invalid")
	}
	repair, found, err := getSpecialistProtocolRepairTx(ctx, tx, attempt.ID)
	if err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	if !found {
		return domain.SpecialistProtocolRepair{}, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist protocol repair was not requested")
	}
	if repair.Status != domain.SpecialistRepairPending {
		if repair.Status == status && repair.ResolvedModelAttempt == modelAttempt {
			return repair, nil
		}
		return domain.SpecialistProtocolRepair{}, apperror.New(apperror.CodeConflict,
			"Specialist protocol repair was already resolved differently")
	}
	resolvedAt := now.UTC()
	if resolvedAt.Before(repair.RequestedAt) {
		resolvedAt = repair.RequestedAt
	}
	updated := repair
	updated.Status = status
	updated.ResolvedModelAttempt = modelAttempt
	updated.ResolvedAt = &resolvedAt
	if err := updated.Validate(); err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE specialist_protocol_repairs
		SET status = ?, resolved_model_attempt = ?, resolved_at = ?
		WHERE agent_attempt_id = ? AND status = ?`, updated.Status,
		updated.ResolvedModelAttempt, ts(resolvedAt), attempt.ID, domain.SpecialistRepairPending)
	if err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	if err := requireSingleAgentAttemptUpdate(result,
		"Specialist protocol repair changed before resolution"); err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	eventType := events.AgentProtocolRepairCompletedEvent
	stage := "completed"
	if status == domain.SpecialistRepairExhausted {
		eventType = events.AgentProtocolRepairFailedEvent
		stage = "exhausted"
	}
	if err := appendSupervisorEventTx(ctx, tx, run, eventType, "specialist_runner",
		attempt.ID, map[string]any{
			"agent_id": attempt.AgentID, "agent_attempt_id": attempt.ID,
			"turn": attempt.Turn, "protocol_repair": 1,
			"model_attempt": modelAttempt, "reason": repair.Reason, "stage": stage,
		}); err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	return updated, nil
}

func abortSpecialistProtocolRepairTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	attempt domain.AgentAttempt, abortReason string, now time.Time,
) error {
	repair, found, err := getSpecialistProtocolRepairTx(ctx, tx, attempt.ID)
	if err != nil || !found || repair.Status != domain.SpecialistRepairPending {
		return err
	}
	resolvedAt := now.UTC()
	if resolvedAt.Before(repair.RequestedAt) {
		resolvedAt = repair.RequestedAt
	}
	result, err := tx.ExecContext(ctx, `UPDATE specialist_protocol_repairs
		SET status = ?, resolved_model_attempt = NULL, resolved_at = ?
		WHERE agent_attempt_id = ? AND status = ?`, domain.SpecialistRepairAborted,
		ts(resolvedAt), attempt.ID, domain.SpecialistRepairPending)
	if err != nil {
		return err
	}
	if err := requireSingleAgentAttemptUpdate(result,
		"Specialist protocol repair changed before abort"); err != nil {
		return err
	}
	return appendSupervisorEventTx(ctx, tx, run, events.AgentProtocolRepairFailedEvent,
		"specialist_runner", attempt.ID, map[string]any{
			"agent_id": attempt.AgentID, "agent_attempt_id": attempt.ID,
			"turn": attempt.Turn, "protocol_repair": 1,
			"reason": normalizeSpecialistRepairReason(abortReason), "stage": "aborted",
		})
}

func requireSpecialistProtocolRepairResolvedTx(ctx context.Context, tx *sql.Tx,
	attemptID string,
) error {
	repair, found, err := getSpecialistProtocolRepairTx(ctx, tx, attemptID)
	if err != nil || !found {
		return err
	}
	if repair.Status != domain.SpecialistRepairCompleted {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Specialist protocol repair is not completed")
	}
	return nil
}

func normalizeSpecialistRepairReason(reason string) string {
	reason = redact.String(strings.Join(strings.Fields(strings.TrimSpace(reason)), " "))
	if reason == "" {
		reason = "response failed strict specialist_lifecycle.v1 validation"
	}
	runes := []rune(reason)
	if len(runes) > domain.MaxSpecialistRepairReasonRunes {
		reason = string(runes[:domain.MaxSpecialistRepairReasonRunes])
		runes = []rune(reason)
	}
	for len([]byte(reason)) > domain.MaxSpecialistRepairReasonBytes && len(runes) > 0 {
		runes = runes[:len(runes)-1]
		reason = string(runes)
	}
	if !utf8.ValidString(reason) || reason == "" {
		return "response failed strict specialist_lifecycle.v1 validation"
	}
	return reason
}

func scanSpecialistProtocolRepair(row scanner) (domain.SpecialistProtocolRepair, error) {
	var repair domain.SpecialistProtocolRepair
	var status string
	var resolvedModel sql.NullInt64
	var requestedAt string
	var resolvedAt sql.NullString
	if err := row.Scan(&repair.AgentAttemptID, &repair.RunID, &repair.AgentID,
		&status, &repair.Reason, &repair.RequestedModelAttempt, &resolvedModel,
		&requestedAt, &resolvedAt); err != nil {
		return domain.SpecialistProtocolRepair{}, err
	}
	repair.Status = domain.SpecialistProtocolRepairStatus(status)
	if resolvedModel.Valid {
		repair.ResolvedModelAttempt = int(resolvedModel.Int64)
	}
	repair.RequestedAt = parseTS(requestedAt)
	repair.ResolvedAt = parseNullableTS(resolvedAt)
	return repair, repair.Validate()
}
