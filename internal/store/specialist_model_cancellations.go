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

const specialistModelCancellationSelect = `SELECT id, run_id, agent_id,
	agent_attempt_id, model_attempt, status, reason, requested_by, requested_at,
	observed_at, resolved_at, resolution FROM specialist_model_cancellations`

func (s *SQLiteStore) RequestSpecialistModelCancellation(ctx context.Context,
	request domain.RequestSpecialistModelCancellation,
) (domain.SpecialistModelCancellationResult, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return domain.SpecialistModelCancellationResult{},
			apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	normalized.Reason = redact.String(normalized.Reason)
	if runes := []rune(normalized.Reason); len(runes) > domain.MaxModelCancellationReasonRunes {
		normalized.Reason = string(runes[:domain.MaxModelCancellationReasonRunes])
	}
	if redact.String(normalized.RequestedBy) != normalized.RequestedBy {
		return domain.SpecialistModelCancellationResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"Specialist model cancellation requester cannot contain sensitive material")
	}
	keyDigest := runmutation.Fingerprint("specialist_model_cancellation_operation.v1",
		normalized.RunID, normalized.IdempotencyKey)
	fingerprint := runmutation.Fingerprint("specialist_model_cancellation_request.v1",
		normalized.RunID, normalized.AgentID, normalized.AttemptID,
		strconv.Itoa(normalized.ModelAttempt), normalized.Reason, normalized.RequestedBy)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	storedFingerprint, cancellationID, found, err :=
		getSpecialistModelCancellationOperationTx(ctx, tx, keyDigest)
	if err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if found {
		if storedFingerprint != fingerprint {
			return domain.SpecialistModelCancellationResult{}, apperror.New(apperror.CodeConflict,
				"Specialist model cancellation idempotency key was already used for different intent")
		}
		cancellation, err := getSpecialistModelCancellationTx(ctx, tx, cancellationID)
		if err != nil {
			return domain.SpecialistModelCancellationResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.SpecialistModelCancellationResult{}, err
		}
		return domain.SpecialistModelCancellationResult{
			Cancellation: cancellation, Replayed: true,
		}, nil
	}
	_, found, err = getSpecialistModelCancellationTargetTx(ctx, tx, normalized.RunID,
		normalized.AgentID, normalized.AttemptID, normalized.ModelAttempt)
	if err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if found {
		return domain.SpecialistModelCancellationResult{}, apperror.New(apperror.CodeConflict,
			"Specialist model attempt already has a cancellation request under another idempotency key")
	}
	ref := domain.AgentAttemptRef{
		RunID: normalized.RunID, AgentID: normalized.AgentID, AttemptID: normalized.AttemptID,
	}
	attempt, child, run, err := loadActiveAgentAttemptTx(ctx, tx, ref)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = apperror.New(apperror.CodeNotFound,
				"Specialist cancellation AgentAttempt was not found")
		}
		return domain.SpecialistModelCancellationResult{}, err
	}
	call, found, err := getSpecialistModelCallTx(ctx, tx, attempt.ID, normalized.ModelAttempt)
	if err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if !found || call.Status != "started" || call.RunID != run.ID || call.AgentID != child.ID {
		return domain.SpecialistModelCancellationResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Specialist model attempt is not active")
	}
	var latest int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(model_attempt_number), 0)
		FROM specialist_model_calls WHERE agent_attempt_id = ?`, attempt.ID).Scan(&latest); err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if latest != normalized.ModelAttempt {
		return domain.SpecialistModelCancellationResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Specialist model cancellation target is no longer the current model attempt")
	}
	now := time.Now().UTC()
	cancellation := domain.SpecialistModelCancellation{
		ID: idgen.New("cancel"), RunID: run.ID, AgentID: child.ID,
		AttemptID: attempt.ID, ModelAttempt: call.ModelAttempt,
		Status: domain.ModelCancellationPending, Reason: normalized.Reason,
		RequestedBy: normalized.RequestedBy, RequestedAt: now,
	}
	if err := cancellation.Validate(); err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_model_cancellations
		(id, run_id, agent_id, agent_attempt_id, model_attempt, status, reason,
		 requested_by, requested_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cancellation.ID, cancellation.RunID, cancellation.AgentID,
		cancellation.AttemptID, cancellation.ModelAttempt, cancellation.Status,
		cancellation.Reason, cancellation.RequestedBy, ts(cancellation.RequestedAt)); err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_model_cancellation_operations
		(operation_key_digest, request_fingerprint, cancellation_id, created_at)
		VALUES (?, ?, ?, ?)`, keyDigest, fingerprint, cancellation.ID, ts(now)); err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelCancelRequestedEvent,
		"control_api", specialistModelSubject(attempt.ID, call.ModelAttempt), map[string]any{
			"agent_id": child.ID, "parent_agent_id": child.ParentID,
			"agent_attempt_id": attempt.ID, "turn": attempt.Turn,
			"model_attempt":     call.ModelAttempt,
			"transport_attempt": call.TransportAttempt,
			"protocol_repair":   call.ProtocolRepair,
			"provider":          call.Provider, "model": call.Model,
			"cancellation_id": cancellation.ID,
			"requested_by":    cancellation.RequestedBy, "reason": cancellation.Reason,
		}); err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SpecialistModelCancellationResult{}, err
	}
	return domain.SpecialistModelCancellationResult{Cancellation: cancellation}, nil
}

func (s *SQLiteStore) ObserveSpecialistModelCancellation(ctx context.Context,
	ref domain.AgentAttemptRef, modelAttempt llm.ModelAttempt,
) (domain.SpecialistModelCancellation, bool, error) {
	ref = normalizeAgentAttemptRef(ref)
	if err := ref.Validate(); err != nil {
		return domain.SpecialistModelCancellation{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument,
				"Specialist cancellation observation reference is invalid", err)
	}
	modelAttempt = sanitizeModelAttempt(modelAttempt)
	if err := validateSpecialistModelIdentity(modelAttempt); err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	preflight, err := getSpecialistModelCancellationRow(s.db.QueryRowContext(ctx,
		specialistModelCancellationSelect+` WHERE run_id = ? AND agent_id = ?
		AND agent_attempt_id = ? AND model_attempt = ?`, ref.RunID, ref.AgentID,
		ref.AttemptID, modelAttempt.Number))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SpecialistModelCancellation{}, false, nil
	}
	if err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	if preflight.Status != domain.ModelCancellationPending {
		return preflight, false, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, child, run, err := loadActiveAgentAttemptTx(ctx, tx, ref)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = apperror.New(apperror.CodeNotFound,
				"Specialist cancellation AgentAttempt was not found")
		}
		return domain.SpecialistModelCancellation{}, false, err
	}
	call, found, err := getSpecialistModelCallTx(ctx, tx, attempt.ID, modelAttempt.Number)
	if err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	if !found || call.Status != "started" {
		return domain.SpecialistModelCancellation{}, false,
			apperror.New(apperror.CodeFailedPrecondition,
				"Specialist cancellation target is no longer active")
	}
	if err := requireSpecialistModelIdentity(call, modelAttempt); err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	cancellation, found, err := getSpecialistModelCancellationTargetTx(ctx, tx, run.ID,
		child.ID, attempt.ID, modelAttempt.Number)
	if err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	if !found || cancellation.Status != domain.ModelCancellationPending {
		if err := tx.Commit(); err != nil {
			return domain.SpecialistModelCancellation{}, false, err
		}
		return cancellation, false, nil
	}
	now := time.Now().UTC()
	if now.Before(cancellation.RequestedAt) {
		now = cancellation.RequestedAt
	}
	result, err := tx.ExecContext(ctx, `UPDATE specialist_model_cancellations
		SET status = ?, observed_at = ? WHERE id = ? AND status = ?`,
		domain.ModelCancellationObserved, ts(now), cancellation.ID,
		domain.ModelCancellationPending)
	if err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	if err := requireSingleAgentAttemptUpdate(result,
		"Specialist model cancellation changed before observation"); err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	cancellation.Status = domain.ModelCancellationObserved
	cancellation.ObservedAt = &now
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelCancelObservedEvent,
		"specialist_runner", specialistModelSubject(attempt.ID, modelAttempt.Number),
		map[string]any{
			"agent_id": child.ID, "parent_agent_id": child.ParentID,
			"agent_attempt_id": attempt.ID, "turn": attempt.Turn,
			"model_attempt":   modelAttempt.Number,
			"cancellation_id": cancellation.ID,
			"requested_by":    cancellation.RequestedBy,
			"requested_at":    cancellation.RequestedAt, "observed_at": now,
		}); err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SpecialistModelCancellation{}, false, err
	}
	return cancellation, true, nil
}

func (s *SQLiteStore) GetSpecialistModelCancellation(ctx context.Context,
	id string,
) (domain.SpecialistModelCancellation, error) {
	id = strings.TrimSpace(id)
	if id == "" || len([]rune(id)) > domain.MaxModelCancellationIdentityRunes {
		return domain.SpecialistModelCancellation{}, apperror.New(
			apperror.CodeInvalidArgument,
			"Specialist model cancellation id is required and bounded")
	}
	return getSpecialistModelCancellationRow(s.db.QueryRowContext(ctx,
		specialistModelCancellationSelect+` WHERE id = ?`, id))
}

func resolveSpecialistModelCancellationTx(ctx context.Context, tx *sql.Tx,
	ref domain.AgentAttemptRef, modelAttempt int, resolution string, at time.Time,
) error {
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		return apperror.New(apperror.CodeInvalidArgument,
			"Specialist model cancellation resolution is required")
	}
	cancellation, found, err := getSpecialistModelCancellationTargetTx(ctx, tx,
		ref.RunID, ref.AgentID, ref.AttemptID, modelAttempt)
	if err != nil || !found {
		return err
	}
	at = monotonicSpecialistCancellationTime(cancellation, at)
	result, err := tx.ExecContext(ctx, `UPDATE specialist_model_cancellations
		SET status = ?, resolved_at = ?, resolution = ?
		WHERE id = ? AND status IN (?, ?)`, domain.ModelCancellationResolved, ts(at),
		resolution, cancellation.ID, domain.ModelCancellationPending,
		domain.ModelCancellationObserved)
	if err != nil {
		return err
	}
	return requireSingleAgentAttemptUpdate(result,
		"Specialist model cancellation changed before terminal resolution")
}

func resolveSupersededSpecialistModelCancellationsTx(ctx context.Context, tx *sql.Tx,
	ref domain.AgentAttemptRef, nextModelAttempt int, at time.Time,
) error {
	if nextModelAttempt <= 0 {
		return apperror.New(apperror.CodeInvalidArgument,
			"next Specialist model attempt must be positive")
	}
	cancellations, err := listUnresolvedSpecialistModelCancellationsTx(ctx, tx,
		`run_id = ? AND agent_id = ? AND agent_attempt_id = ? AND model_attempt < ?`,
		ref.RunID, ref.AgentID, ref.AttemptID, nextModelAttempt)
	if err != nil {
		return err
	}
	return resolveSpecialistModelCancellationRowsTx(ctx, tx, cancellations,
		"superseded", at)
}

func resolveTerminatedSpecialistModelCancellationsTx(ctx context.Context, tx *sql.Tx,
	attempt domain.AgentAttempt, resolution string, at time.Time,
) error {
	cancellations, err := listUnresolvedSpecialistModelCancellationsTx(ctx, tx,
		`run_id = ? AND agent_id = ? AND agent_attempt_id = ?`, attempt.RunID,
		attempt.AgentID, attempt.ID)
	if err != nil {
		return err
	}
	return resolveSpecialistModelCancellationRowsTx(ctx, tx, cancellations,
		resolution, at)
}

func listUnresolvedSpecialistModelCancellationsTx(ctx context.Context, tx *sql.Tx,
	where string, args ...any,
) ([]domain.SpecialistModelCancellation, error) {
	rows, err := tx.QueryContext(ctx, specialistModelCancellationSelect+` WHERE `+where+
		` AND status IN (?, ?) ORDER BY model_attempt`, append(args,
		domain.ModelCancellationPending, domain.ModelCancellationObserved)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cancellations []domain.SpecialistModelCancellation
	for rows.Next() {
		cancellation, err := getSpecialistModelCancellationRow(rows)
		if err != nil {
			return nil, err
		}
		cancellations = append(cancellations, cancellation)
	}
	return cancellations, rows.Err()
}

func resolveSpecialistModelCancellationRowsTx(ctx context.Context, tx *sql.Tx,
	cancellations []domain.SpecialistModelCancellation, resolution string, at time.Time,
) error {
	for _, cancellation := range cancellations {
		resolvedAt := monotonicSpecialistCancellationTime(cancellation, at)
		result, err := tx.ExecContext(ctx, `UPDATE specialist_model_cancellations
			SET status = ?, resolved_at = ?, resolution = ?
			WHERE id = ? AND status IN (?, ?)`, domain.ModelCancellationResolved,
			ts(resolvedAt), resolution, cancellation.ID, domain.ModelCancellationPending,
			domain.ModelCancellationObserved)
		if err != nil {
			return err
		}
		if err := requireSingleAgentAttemptUpdate(result,
			"Specialist model cancellation changed before resolution"); err != nil {
			return err
		}
	}
	return nil
}

func monotonicSpecialistCancellationTime(cancellation domain.SpecialistModelCancellation,
	at time.Time,
) time.Time {
	at = at.UTC()
	minimum := cancellation.RequestedAt
	if cancellation.ObservedAt != nil && cancellation.ObservedAt.After(minimum) {
		minimum = *cancellation.ObservedAt
	}
	if at.Before(minimum) {
		return minimum
	}
	return at
}

func getSpecialistModelCancellationOperationTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (string, string, bool, error) {
	var fingerprint, cancellationID string
	err := tx.QueryRowContext(ctx, `SELECT request_fingerprint, cancellation_id
		FROM specialist_model_cancellation_operations WHERE operation_key_digest = ?`,
		keyDigest).Scan(&fingerprint, &cancellationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return fingerprint, cancellationID, err == nil, err
}

func getSpecialistModelCancellationTargetTx(ctx context.Context, tx *sql.Tx,
	runID string, agentID string, attemptID string, modelAttempt int,
) (domain.SpecialistModelCancellation, bool, error) {
	cancellation, err := getSpecialistModelCancellationRow(tx.QueryRowContext(ctx,
		specialistModelCancellationSelect+` WHERE run_id = ? AND agent_id = ?
		AND agent_attempt_id = ? AND model_attempt = ?`, runID, agentID, attemptID,
		modelAttempt))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SpecialistModelCancellation{}, false, nil
	}
	return cancellation, err == nil, err
}

func getSpecialistModelCancellationTx(ctx context.Context, tx *sql.Tx,
	id string,
) (domain.SpecialistModelCancellation, error) {
	return getSpecialistModelCancellationRow(tx.QueryRowContext(ctx,
		specialistModelCancellationSelect+` WHERE id = ?`, id))
}

func getSpecialistModelCancellationRow(row scanner) (domain.SpecialistModelCancellation, error) {
	var cancellation domain.SpecialistModelCancellation
	var requestedAt string
	var observedAt, resolvedAt sql.NullString
	if err := row.Scan(&cancellation.ID, &cancellation.RunID, &cancellation.AgentID,
		&cancellation.AttemptID, &cancellation.ModelAttempt, &cancellation.Status,
		&cancellation.Reason, &cancellation.RequestedBy, &requestedAt, &observedAt,
		&resolvedAt, &cancellation.Resolution); err != nil {
		return domain.SpecialistModelCancellation{}, err
	}
	cancellation.RequestedAt = parseTS(requestedAt)
	cancellation.ObservedAt = parseNullableTS(observedAt)
	cancellation.ResolvedAt = parseNullableTS(resolvedAt)
	if err := cancellation.Validate(); err != nil {
		return domain.SpecialistModelCancellation{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"invalid persisted Specialist model cancellation", err)
	}
	return cancellation, nil
}
