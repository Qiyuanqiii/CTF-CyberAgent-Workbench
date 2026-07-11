package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
)

const modelCancellationSelect = `SELECT id, run_id, attempt_id, model_attempt, status, reason,
	requested_by, requested_at, observed_at, resolved_at, resolution FROM run_model_cancellations`

func (s *SQLiteStore) RequestSupervisorModelCancellation(ctx context.Context,
	request domain.RequestModelCancellation,
) (domain.ModelCancellationResult, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return domain.ModelCancellationResult{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	normalized.Reason = redact.String(normalized.Reason)
	if runes := []rune(normalized.Reason); len(runes) > domain.MaxModelCancellationReasonRunes {
		normalized.Reason = string(runes[:domain.MaxModelCancellationReasonRunes])
	}
	if redact.String(normalized.RequestedBy) != normalized.RequestedBy {
		return domain.ModelCancellationResult{}, apperror.New(apperror.CodeInvalidArgument,
			"model cancellation requester cannot contain sensitive material")
	}
	keyDigest := runmutation.Fingerprint("model_cancellation_operation.v1", normalized.RunID,
		normalized.IdempotencyKey)
	fingerprint := runmutation.Fingerprint("model_cancellation_request.v1", normalized.RunID,
		normalized.AttemptID, strconv.Itoa(normalized.ModelAttempt), normalized.Reason, normalized.RequestedBy)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.ModelCancellationResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	operationFingerprint, cancellationID, found, err := getModelCancellationOperationTx(ctx, tx, keyDigest)
	if err != nil {
		return domain.ModelCancellationResult{}, err
	}
	if found {
		if operationFingerprint != fingerprint {
			return domain.ModelCancellationResult{}, apperror.New(apperror.CodeConflict,
				"model cancellation idempotency key was already used for different intent")
		}
		cancellation, err := getModelCancellationTx(ctx, tx, cancellationID)
		if err != nil {
			return domain.ModelCancellationResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.ModelCancellationResult{}, err
		}
		return domain.ModelCancellationResult{Cancellation: cancellation, Replayed: true}, nil
	}

	_, found, err = getModelCancellationTargetTx(ctx, tx, normalized.RunID,
		normalized.AttemptID, normalized.ModelAttempt)
	if err != nil {
		return domain.ModelCancellationResult{}, err
	}
	if found {
		return domain.ModelCancellationResult{}, apperror.New(apperror.CodeConflict,
			"model attempt already has a cancellation request under another idempotency key")
	}

	run, current, startedPayload, subject, err := requireCancellableModelAttemptTx(ctx, tx, normalized)
	if err != nil {
		return domain.ModelCancellationResult{}, err
	}
	now := time.Now().UTC()
	cancellation := domain.ModelCancellation{
		ID: idgen.New("cancel"), RunID: run.ID, AttemptID: current.AttemptID,
		ModelAttempt: normalized.ModelAttempt, Status: domain.ModelCancellationPending,
		Reason: normalized.Reason, RequestedBy: normalized.RequestedBy, RequestedAt: now,
	}
	if err := cancellation.Validate(); err != nil {
		return domain.ModelCancellationResult{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_model_cancellations
		(id, run_id, attempt_id, model_attempt, status, reason, requested_by, requested_at,
		 observed_at, resolved_at, resolution) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, '')`,
		cancellation.ID, cancellation.RunID, cancellation.AttemptID, cancellation.ModelAttempt,
		cancellation.Status, cancellation.Reason, cancellation.RequestedBy, ts(cancellation.RequestedAt)); err != nil {
		return domain.ModelCancellationResult{}, err
	}
	if err := insertModelCancellationOperationTx(ctx, tx, keyDigest, fingerprint, cancellation.ID); err != nil {
		return domain.ModelCancellationResult{}, err
	}
	alreadyAudited, err := supervisorModelEventExistsTx(ctx, tx, run.ID, events.ModelCancelRequestedEvent, subject)
	if err != nil {
		return domain.ModelCancellationResult{}, err
	}
	if !alreadyAudited {
		if err := appendSupervisorEventTx(ctx, tx, run, events.ModelCancelRequestedEvent,
			"control_api", subject, map[string]any{
				"turn": current.NextTurn, "attempt_id": current.AttemptID,
				"model_attempt":     normalized.ModelAttempt,
				"transport_attempt": startedPayload.transportAttempt(),
				"protocol_repair":   startedPayload.protocolRepair(), "tool_round": startedPayload.toolRound(),
				"provider": startedPayload.Provider, "model": startedPayload.Model,
				"cancellation_id": cancellation.ID, "requested_by": cancellation.RequestedBy,
				"reason": cancellation.Reason,
			}); err != nil {
			return domain.ModelCancellationResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.ModelCancellationResult{}, err
	}
	return domain.ModelCancellationResult{Cancellation: cancellation}, nil
}

func (s *SQLiteStore) ObserveSupervisorModelCancellation(ctx context.Context,
	checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt,
) (domain.ModelCancellation, bool, error) {
	if err := checkpoint.Validate(); err != nil {
		return domain.ModelCancellation{}, false, err
	}
	attempt = sanitizeModelAttempt(attempt)
	if err := attempt.ValidateStarted(); err != nil {
		return domain.ModelCancellation{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument, "invalid model cancellation observation attempt", err)
	}
	preflight, err := getModelCancellationRow(s.db.QueryRowContext(ctx, modelCancellationSelect+
		` WHERE run_id = ? AND attempt_id = ? AND model_attempt = ?`, checkpoint.RunID,
		checkpoint.AttemptID, attempt.Number))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ModelCancellation{}, false, nil
	}
	if err != nil {
		return domain.ModelCancellation{}, false, err
	}
	if preflight.Status != domain.ModelCancellationPending {
		return preflight, false, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.ModelCancellation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	run, current, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
	if err != nil {
		return domain.ModelCancellation{}, false, err
	}
	if err := requireSupervisorModelStartedMatchTx(ctx, tx, run.ID,
		supervisorModelSubject(checkpoint, attempt.Number), attempt); err != nil {
		return domain.ModelCancellation{}, false, err
	}
	executionLease, found, err := getRunExecutionLeaseTx(ctx, tx, run.ID)
	if err != nil {
		return domain.ModelCancellation{}, false, err
	}
	if !found {
		return domain.ModelCancellation{}, false,
			apperror.New(apperror.CodeConflict, "model cancellation worker lease disappeared")
	}
	cancellation, found, err := getModelCancellationTargetTx(ctx, tx, run.ID, current.AttemptID, attempt.Number)
	if err != nil {
		return domain.ModelCancellation{}, false, err
	}
	if !found || cancellation.Status != domain.ModelCancellationPending {
		if err := tx.Commit(); err != nil {
			return domain.ModelCancellation{}, false, err
		}
		return cancellation, false, nil
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE run_model_cancellations SET status = ?, observed_at = ?
		WHERE id = ? AND status = ?`, domain.ModelCancellationObserved, ts(now), cancellation.ID,
		domain.ModelCancellationPending)
	if err != nil {
		return domain.ModelCancellation{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.ModelCancellation{}, false, err
	}
	if rows != 1 {
		return domain.ModelCancellation{}, false,
			apperror.New(apperror.CodeConflict, "model cancellation changed before observation")
	}
	cancellation.Status = domain.ModelCancellationObserved
	cancellation.ObservedAt = &now
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelCancelObservedEvent,
		"run_supervisor", supervisorModelSubject(checkpoint, attempt.Number), map[string]any{
			"turn": current.NextTurn, "attempt_id": current.AttemptID,
			"model_attempt": attempt.Number, "cancellation_id": cancellation.ID,
			"requested_by": cancellation.RequestedBy, "requested_at": cancellation.RequestedAt,
			"observed_at": now, "worker_owner": executionLease.OwnerID,
		}); err != nil {
		return domain.ModelCancellation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ModelCancellation{}, false, err
	}
	return cancellation, true, nil
}

func (s *SQLiteStore) GetModelCancellation(ctx context.Context, id string) (domain.ModelCancellation, error) {
	id = strings.TrimSpace(id)
	if id == "" || len([]rune(id)) > domain.MaxModelCancellationIdentityRunes {
		return domain.ModelCancellation{}, apperror.New(apperror.CodeInvalidArgument,
			"model cancellation id is required and bounded")
	}
	return getModelCancellationRow(s.db.QueryRowContext(ctx, modelCancellationSelect+` WHERE id = ?`, id))
}

func requireCancellableModelAttemptTx(ctx context.Context, tx *sql.Tx,
	request domain.RequestModelCancellation,
) (domain.Run, domain.SupervisorCheckpoint, supervisorModelStartedPayload, string, error) {
	current, found, err := getSupervisorCheckpointTx(ctx, tx, request.RunID)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
	}
	if !found {
		if _, runErr := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json,
			budget_json, started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, request.RunID)); runErr != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", runErr
		}
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
			apperror.New(apperror.CodeFailedPrecondition, "model cancellation requires an active supervisor attempt")
	}
	if current.Phase != domain.SupervisorTurnStarted || current.AttemptID != request.AttemptID {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
			apperror.New(apperror.CodeConflict, "model cancellation does not match the active supervisor attempt")
	}
	lease, found, err := getRunExecutionLeaseTx(ctx, tx, request.RunID)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
	}
	if !found || !lease.ActiveAt(time.Now().UTC()) || current.LeaseID != lease.LeaseID ||
		current.LeaseGeneration != lease.Generation {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
			apperror.New(apperror.CodeFailedPrecondition, "model cancellation requires an active execution worker")
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, request.RunID))
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
	}
	if run.Status != domain.RunRunning {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
			apperror.New(apperror.CodeFailedPrecondition, "model cancellation requires a running Run")
	}
	subject := supervisorModelSubject(current, request.ModelAttempt)
	var payloadJSON string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id = ?`, run.ID,
		events.ModelStartedEvent, "model_gateway", subject).Scan(&payloadJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
				apperror.New(apperror.CodeFailedPrecondition, "model attempt is not active")
		}
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
	}
	payload, err := parseSupervisorModelStartedPayload(payloadJSON)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
	}
	if payload.ModelAttempt != request.ModelAttempt {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
			apperror.New(apperror.CodeConflict, "model cancellation attempt metadata is inconsistent")
	}
	var latestPayloadJSON string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM run_events
		WHERE run_id = ? AND type = ? AND source = ? AND subject_id LIKE ? ORDER BY sequence DESC LIMIT 1`,
		run.ID, events.ModelStartedEvent, "model_gateway", supervisorModelSubjectPrefix(current)+"%").
		Scan(&latestPayloadJSON); err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
	}
	latestPayload, err := parseSupervisorModelStartedPayload(latestPayloadJSON)
	if err != nil {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
	}
	if latestPayload.ModelAttempt != request.ModelAttempt {
		return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
			apperror.New(apperror.CodeFailedPrecondition,
				"model cancellation target is no longer the current model attempt")
	}
	for _, terminalType := range []string{events.ModelCompletedEvent, events.ModelFailedEvent} {
		terminal, err := supervisorModelEventExistsTx(ctx, tx, run.ID, terminalType, subject)
		if err != nil {
			return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "", err
		}
		if terminal {
			return domain.Run{}, domain.SupervisorCheckpoint{}, supervisorModelStartedPayload{}, "",
				apperror.New(apperror.CodeFailedPrecondition, "model attempt is already terminal")
		}
	}
	return run, current, payload, subject, nil
}

func resolveSupervisorModelCancellationTx(ctx context.Context, tx *sql.Tx, checkpoint domain.SupervisorCheckpoint,
	attempt llm.ModelAttempt,
) error {
	resolution := string(attempt.Outcome)
	if strings.TrimSpace(resolution) == "" {
		return apperror.New(apperror.CodeInvalidArgument, "model cancellation resolution requires an outcome")
	}
	_, err := tx.ExecContext(ctx, `UPDATE run_model_cancellations SET status = ?, resolved_at = ?, resolution = ?
		WHERE run_id = ? AND attempt_id = ? AND model_attempt = ? AND status IN (?, ?)`,
		domain.ModelCancellationResolved, ts(time.Now().UTC()), resolution, checkpoint.RunID,
		checkpoint.AttemptID, attempt.Number, domain.ModelCancellationPending, domain.ModelCancellationObserved)
	return err
}

func resolveSupersededModelCancellationsTx(ctx context.Context, tx *sql.Tx,
	checkpoint domain.SupervisorCheckpoint, nextModelAttempt int,
) error {
	if nextModelAttempt <= 0 {
		return apperror.New(apperror.CodeInvalidArgument, "next model attempt must be positive")
	}
	_, err := tx.ExecContext(ctx, `UPDATE run_model_cancellations SET status = ?, resolved_at = ?, resolution = ?
		WHERE run_id = ? AND attempt_id = ? AND model_attempt < ? AND status IN (?, ?)`,
		domain.ModelCancellationResolved, ts(time.Now().UTC()), "superseded", checkpoint.RunID,
		checkpoint.AttemptID, nextModelAttempt, domain.ModelCancellationPending, domain.ModelCancellationObserved)
	return err
}

func getModelCancellationOperationTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (string, string, bool, error) {
	var fingerprint, cancellationID string
	err := tx.QueryRowContext(ctx, `SELECT request_fingerprint, cancellation_id
		FROM run_model_cancellation_operations WHERE operation_key_digest = ?`, keyDigest).
		Scan(&fingerprint, &cancellationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return fingerprint, cancellationID, err == nil, err
}

func insertModelCancellationOperationTx(ctx context.Context, tx *sql.Tx, keyDigest string,
	fingerprint string, cancellationID string,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO run_model_cancellation_operations
		(operation_key_digest, request_fingerprint, cancellation_id, created_at) VALUES (?, ?, ?, ?)`,
		keyDigest, fingerprint, cancellationID, ts(time.Now().UTC()))
	return err
}

func getModelCancellationTargetTx(ctx context.Context, tx *sql.Tx, runID string, attemptID string,
	modelAttempt int,
) (domain.ModelCancellation, bool, error) {
	cancellation, err := getModelCancellationRow(tx.QueryRowContext(ctx, modelCancellationSelect+
		` WHERE run_id = ? AND attempt_id = ? AND model_attempt = ?`, runID, attemptID, modelAttempt))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ModelCancellation{}, false, nil
	}
	return cancellation, err == nil, err
}

func getModelCancellationTx(ctx context.Context, tx *sql.Tx, id string) (domain.ModelCancellation, error) {
	return getModelCancellationRow(tx.QueryRowContext(ctx, modelCancellationSelect+` WHERE id = ?`, id))
}

type modelCancellationRow interface {
	Scan(dest ...any) error
}

func getModelCancellationRow(row modelCancellationRow) (domain.ModelCancellation, error) {
	var cancellation domain.ModelCancellation
	var requestedAt string
	var observedAt, resolvedAt sql.NullString
	if err := row.Scan(&cancellation.ID, &cancellation.RunID, &cancellation.AttemptID,
		&cancellation.ModelAttempt, &cancellation.Status, &cancellation.Reason, &cancellation.RequestedBy,
		&requestedAt, &observedAt, &resolvedAt, &cancellation.Resolution); err != nil {
		return domain.ModelCancellation{}, err
	}
	cancellation.RequestedAt = parseTS(requestedAt)
	cancellation.ObservedAt = parseNullableTS(observedAt)
	cancellation.ResolvedAt = parseNullableTS(resolvedAt)
	if err := cancellation.Validate(); err != nil {
		return domain.ModelCancellation{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"invalid persisted model cancellation", err)
	}
	return cancellation, nil
}
