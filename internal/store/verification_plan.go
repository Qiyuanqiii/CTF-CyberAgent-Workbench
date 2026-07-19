package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

const verificationPlanSelect = `SELECT id, protocol_version, operation_key_digest,
	request_fingerprint, run_id, session_id, workspace_id, title, summary,
	plan_sha256, redacted, authored_by, item_count, event_sequence, created_at
	FROM operator_verification_plans`

type verificationPlanQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *SQLiteStore) GetVerificationPlanByOperation(ctx context.Context,
	keyDigest string,
) (verification.Plan, bool, error) {
	if !validStoreDigest(keyDigest) {
		return verification.Plan{}, false, apperror.New(apperror.CodeInvalidArgument,
			"verification plan operation digest is invalid")
	}
	value, itemCount, err := scanVerificationPlanHeader(s.db.QueryRowContext(ctx,
		verificationPlanSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.Plan{}, false, nil
	}
	if err != nil {
		return verification.Plan{}, false, err
	}
	value.Items, err = loadVerificationPlanItems(ctx, s.db, value.ID, itemCount)
	if err != nil {
		return verification.Plan{}, false, err
	}
	if err := value.Validate(); err != nil {
		return verification.Plan{}, false,
			fmt.Errorf("stored verification plan is invalid: %w", err)
	}
	return value, true, nil
}

func (s *SQLiteStore) ListVerificationPlans(ctx context.Context, runID string,
	limit int,
) ([]verification.Plan, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification plan Run identity is invalid")
	}
	if limit < 1 || limit > verification.MaxPlanInventoryItems+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification plan limit is invalid")
	}
	rows, err := s.db.QueryContext(ctx, verificationPlanSelect+
		` WHERE run_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	type header struct {
		plan      verification.Plan
		itemCount int
	}
	headers := make([]header, 0, limit)
	for rows.Next() {
		value, itemCount, err := scanVerificationPlanHeader(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		headers = append(headers, header{plan: value, itemCount: itemCount})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	values := make([]verification.Plan, len(headers))
	for index, current := range headers {
		current.plan.Items, err = loadVerificationPlanItems(ctx, s.db,
			current.plan.ID, current.itemCount)
		if err != nil {
			return nil, err
		}
		if err := current.plan.Validate(); err != nil {
			return nil, fmt.Errorf("stored verification plan is invalid: %w", err)
		}
		values[index] = current.plan
	}
	return values, nil
}

func (s *SQLiteStore) RecordVerificationPlan(ctx context.Context,
	plan verification.Plan,
) (verification.Plan, bool, error) {
	prepared := plan
	prepared.EventSequence = 1
	if plan.EventSequence != 0 {
		return verification.Plan{}, false, apperror.New(apperror.CodeInvalidArgument,
			"new verification plan cannot carry an event sequence")
	}
	if err := prepared.Validate(); err != nil {
		return verification.Plan{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification plan is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return verification.Plan{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`,
		plan.RunID)
	if err != nil {
		return verification.Plan{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return verification.Plan{}, false, err
	}
	if rows != 1 {
		return verification.Plan{}, false,
			apperror.New(apperror.CodeNotFound, "verification plan Run was not found")
	}
	existing, found, err := getVerificationPlanTx(ctx, tx, plan.OperationKeyDigest)
	if err != nil {
		return verification.Plan{}, false, err
	}
	if found {
		if !sameVerificationPlanIntent(existing, plan) {
			return verification.Plan{}, false, apperror.New(apperror.CodeConflict,
				"verification plan operation key was used for different intent")
		}
		if err := tx.Commit(); err != nil {
			return verification.Plan{}, false, err
		}
		return existing, true, nil
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, plan.RunID))
	if err != nil {
		return verification.Plan{}, false, err
	}
	var workspaceID, sessionWorkspaceID, sessionStatus, surface string
	if err := tx.QueryRowContext(ctx, `SELECT mission.workspace_id, session_record.workspace_id,
		session_record.status, mode.surface
		FROM missions mission JOIN sessions session_record ON session_record.id = ?
		JOIN run_mode_snapshots mode ON mode.run_id = ?
		WHERE mission.id = ? AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
			WHERE later.run_id = mode.run_id AND later.revision > mode.revision)`,
		plan.SessionID, run.ID, run.MissionID).Scan(
		&workspaceID, &sessionWorkspaceID, &sessionStatus, &surface); err != nil {
		return verification.Plan{}, false, err
	}
	if run.SessionID != plan.SessionID || workspaceID != plan.WorkspaceID ||
		sessionWorkspaceID != plan.WorkspaceID || sessionStatus != session.StatusActive ||
		surface != string(domain.ExecutionSurfaceCode) {
		return verification.Plan{}, false, apperror.New(apperror.CodeConflict,
			"verification plan Run, active Code Session, or Workspace binding changed")
	}
	event, err := events.New(run.ID, run.MissionID,
		events.VerificationPlanRecordedEvent, "operator_verification_plan", plan.ID,
		map[string]any{
			"plan_sha256": plan.PlanSHA256, "item_count": len(plan.Items),
			"redacted": plan.Redacted, "guidance_only": true,
			"command_executed": false, "model_assertion": false,
			"result_inferred": false, "approval": false, "authority_granted": false,
		})
	if err != nil {
		return verification.Plan{}, false, err
	}
	event.CreatedAt = plan.CreatedAt
	event, err = insertRunEventTx(ctx, tx, event)
	if err != nil {
		return verification.Plan{}, false, err
	}
	plan.EventSequence = event.Sequence
	if err := plan.Validate(); err != nil {
		return verification.Plan{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operator_verification_plans
		(id, protocol_version, operation_key_digest, request_fingerprint, run_id,
		session_id, workspace_id, title, summary, plan_sha256, redacted, authored_by,
		item_count, event_sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.ProtocolVersion, plan.OperationKeyDigest, plan.RequestFingerprint,
		plan.RunID, plan.SessionID, plan.WorkspaceID, plan.Title, plan.Summary,
		plan.PlanSHA256, boolInt(plan.Redacted), plan.AuthoredBy, len(plan.Items),
		plan.EventSequence, ts(plan.CreatedAt)); err != nil {
		return verification.Plan{}, false, err
	}
	for _, item := range plan.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO operator_verification_plan_items
			(plan_id, ordinal, title, expected_observation, item_sha256, redacted)
			VALUES (?, ?, ?, ?, ?, ?)`, plan.ID, item.Ordinal, item.Title,
			item.ExpectedObservation, item.ItemSHA256, boolInt(item.Redacted)); err != nil {
			return verification.Plan{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return verification.Plan{}, false, err
	}
	return plan, false, nil
}

func scanVerificationPlanHeader(row scanner) (verification.Plan, int, error) {
	var value verification.Plan
	var redacted, itemCount int
	var created string
	if err := row.Scan(&value.ID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.RequestFingerprint, &value.RunID, &value.SessionID, &value.WorkspaceID,
		&value.Title, &value.Summary, &value.PlanSHA256, &redacted, &value.AuthoredBy,
		&itemCount, &value.EventSequence, &created); err != nil {
		return verification.Plan{}, 0, err
	}
	if (redacted != 0 && redacted != 1) || itemCount < 1 || itemCount > verification.MaxPlanItems {
		return verification.Plan{}, 0, errors.New("stored verification plan flags are invalid")
	}
	value.Redacted = redacted == 1
	value.CreatedAt = parseTS(created)
	return value, itemCount, nil
}

func loadVerificationPlanItems(ctx context.Context, queryer verificationPlanQueryer,
	planID string, expected int,
) ([]verification.PlanItem, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, title, expected_observation,
		item_sha256, redacted FROM operator_verification_plan_items
		WHERE plan_id = ? ORDER BY ordinal`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]verification.PlanItem, 0, expected)
	for rows.Next() {
		var item verification.PlanItem
		var redacted int
		if err := rows.Scan(&item.Ordinal, &item.Title, &item.ExpectedObservation,
			&item.ItemSHA256, &redacted); err != nil {
			return nil, err
		}
		if redacted != 0 && redacted != 1 {
			return nil, errors.New("stored verification plan item redaction is invalid")
		}
		item.Redacted = redacted == 1
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) != expected {
		return nil, errors.New("stored verification plan item count is invalid")
	}
	return items, nil
}

func getVerificationPlanTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (verification.Plan, bool, error) {
	value, itemCount, err := scanVerificationPlanHeader(tx.QueryRowContext(ctx,
		verificationPlanSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.Plan{}, false, nil
	}
	if err != nil {
		return verification.Plan{}, false, err
	}
	value.Items, err = loadVerificationPlanItems(ctx, tx, value.ID, itemCount)
	if err != nil {
		return verification.Plan{}, false, err
	}
	if err := value.Validate(); err != nil {
		return verification.Plan{}, false, err
	}
	return value, true, nil
}

func sameVerificationPlanIntent(left verification.Plan, right verification.Plan) bool {
	if left.ProtocolVersion != right.ProtocolVersion ||
		left.OperationKeyDigest != right.OperationKeyDigest ||
		left.RequestFingerprint != right.RequestFingerprint || left.RunID != right.RunID ||
		left.SessionID != right.SessionID || left.WorkspaceID != right.WorkspaceID ||
		left.Title != right.Title || left.Summary != right.Summary ||
		left.PlanSHA256 != right.PlanSHA256 || left.Redacted != right.Redacted ||
		left.AuthoredBy != right.AuthoredBy || len(left.Items) != len(right.Items) {
		return false
	}
	for index := range left.Items {
		if left.Items[index] != right.Items[index] {
			return false
		}
	}
	return true
}
