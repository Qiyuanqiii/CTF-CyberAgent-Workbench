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

const verificationAssociationSelect = `SELECT id, protocol_version, operation_key_digest,
	request_fingerprint, run_id, session_id, workspace_id, plan_id, plan_item_ordinal,
	plan_item_sha256, evidence_id, evidence_outcome, evidence_event_sequence,
	associated_by, event_sequence, created_at
	FROM operator_verification_plan_evidence_associations`

func (s *SQLiteStore) GetVerificationPlanEvidenceAssociationByOperation(ctx context.Context,
	keyDigest string,
) (verification.PlanEvidenceAssociation, bool, error) {
	if !validStoreDigest(keyDigest) {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.New(apperror.CodeInvalidArgument,
				"verification association operation digest is invalid")
	}
	value, err := scanVerificationAssociation(s.db.QueryRowContext(ctx,
		verificationAssociationSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.PlanEvidenceAssociation{}, false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) ListVerificationPlanEvidenceAssociations(ctx context.Context,
	runID string, limit int,
) ([]verification.PlanEvidenceAssociation, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification association Run identity is invalid")
	}
	if limit < 1 || limit > verification.MaxCoverageAssociations+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification association limit is invalid")
	}
	rows, err := s.db.QueryContext(ctx, verificationAssociationSelect+
		` WHERE run_id = ? ORDER BY event_sequence DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]verification.PlanEvidenceAssociation, 0, limit)
	for rows.Next() {
		value, err := scanVerificationAssociation(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) ListVerificationPlanItemEvidenceAssociations(ctx context.Context,
	runID string, planID string, ordinal int, limit int, highWater int64,
	beforeSequence int64, beforeID string,
) ([]verification.PlanEvidenceAssociation, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) ||
		planID != strings.TrimSpace(planID) || !domain.ValidAgentID(planID) ||
		ordinal < 1 || ordinal > verification.MaxPlanItems {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification plan item association binding is invalid")
	}
	if limit < 1 || limit > verification.MaxCoverageAssociations+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification plan item association limit is invalid")
	}
	if highWater < 0 || beforeSequence < 0 || beforeSequence > highWater ||
		((beforeSequence == 0) != (beforeID == "")) ||
		(beforeID != "" && (beforeID != strings.TrimSpace(beforeID) ||
			!domain.ValidAgentID(beforeID))) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification plan item association keyset is invalid")
	}
	query := verificationAssociationSelect +
		` WHERE run_id = ? AND plan_id = ? AND plan_item_ordinal = ?
		 AND event_sequence <= ?`
	arguments := []any{runID, planID, ordinal, highWater}
	if beforeSequence > 0 {
		query += ` AND (event_sequence < ? OR (event_sequence = ? AND id < ?))`
		arguments = append(arguments, beforeSequence, beforeSequence, beforeID)
	}
	query += ` ORDER BY event_sequence DESC, id DESC LIMIT ?`
	arguments = append(arguments, limit)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]verification.PlanEvidenceAssociation, 0, limit)
	for rows.Next() {
		value, err := scanVerificationAssociation(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) GetVerificationPlanItemCoverageSnapshot(ctx context.Context,
	runID string, planID string, ordinal int, highWater int64,
) (verification.PlanItemCoverageCount, bool, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) ||
		planID != strings.TrimSpace(planID) || !domain.ValidAgentID(planID) ||
		ordinal < 1 || ordinal > verification.MaxPlanItems || highWater < 0 {
		return verification.PlanItemCoverageCount{}, false, apperror.New(
			apperror.CodeInvalidArgument, "verification coverage snapshot binding is invalid")
	}
	query := `SELECT plan_id, plan_item_ordinal, plan_item_sha256, COUNT(*),
		SUM(CASE WHEN evidence_outcome = 'pass' THEN 1 ELSE 0 END),
		SUM(CASE WHEN evidence_outcome = 'fail' THEN 1 ELSE 0 END),
		SUM(CASE WHEN evidence_outcome = 'unknown' THEN 1 ELSE 0 END),
		MAX(event_sequence)
		FROM operator_verification_plan_evidence_associations
		WHERE run_id = ? AND plan_id = ? AND plan_item_ordinal = ? AND event_sequence <= ?
		GROUP BY plan_id, plan_item_ordinal, plan_item_sha256`
	var value verification.PlanItemCoverageCount
	err := s.db.QueryRowContext(ctx, query, runID, planID, ordinal, highWater).Scan(
		&value.PlanID, &value.PlanItemOrdinal, &value.PlanItemSHA256,
		&value.AssociatedEvidenceCount, &value.PassCount, &value.FailCount,
		&value.UnknownCount, &value.LatestAssociationEventSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return verification.PlanItemCoverageCount{}, false, nil
	}
	if err != nil {
		return verification.PlanItemCoverageCount{}, false, err
	}
	if err := value.Validate(); err != nil {
		return verification.PlanItemCoverageCount{}, false,
			fmt.Errorf("stored verification coverage snapshot is invalid: %w", err)
	}
	return value, true, nil
}

func (s *SQLiteStore) CountVerificationPlanItemAssociationsThroughAnchor(ctx context.Context,
	runID string, planID string, ordinal int, highWater int64, beforeSequence int64,
	beforeID string,
) (int, bool, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) ||
		planID != strings.TrimSpace(planID) || !domain.ValidAgentID(planID) ||
		ordinal < 1 || ordinal > verification.MaxPlanItems || highWater <= 0 ||
		beforeSequence <= 0 || beforeSequence > highWater ||
		beforeID != strings.TrimSpace(beforeID) || !domain.ValidAgentID(beforeID) {
		return 0, false, apperror.New(apperror.CodeInvalidArgument,
			"verification coverage page anchor is invalid")
	}
	var count int
	var anchorCount int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN event_sequence = ? AND id = ? THEN 1 ELSE 0 END), 0)
		FROM operator_verification_plan_evidence_associations
		WHERE run_id = ? AND plan_id = ? AND plan_item_ordinal = ? AND event_sequence <= ?
		AND (event_sequence > ? OR (event_sequence = ? AND id >= ?))`,
		beforeSequence, beforeID, runID, planID, ordinal, highWater,
		beforeSequence, beforeSequence, beforeID).Scan(&count, &anchorCount)
	if err != nil {
		return 0, false, err
	}
	return count, anchorCount == 1, nil
}

func (s *SQLiteStore) ListVerificationPlanCoverageCounts(ctx context.Context,
	runID string, planIDs []string,
) ([]verification.PlanItemCoverageCount, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification coverage Run identity is invalid")
	}
	if len(planIDs) == 0 {
		return []verification.PlanItemCoverageCount{}, nil
	}
	if len(planIDs) > verification.MaxPlanInventoryItems {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"verification coverage plan count is invalid")
	}
	arguments := make([]any, 0, len(planIDs)+1)
	arguments = append(arguments, runID)
	seen := make(map[string]struct{}, len(planIDs))
	placeholders := make([]string, len(planIDs))
	for index, planID := range planIDs {
		if planID != strings.TrimSpace(planID) || !domain.ValidAgentID(planID) {
			return nil, apperror.New(apperror.CodeInvalidArgument,
				"verification coverage plan identity is invalid")
		}
		if _, exists := seen[planID]; exists {
			return nil, apperror.New(apperror.CodeInvalidArgument,
				"verification coverage plan identity is duplicated")
		}
		seen[planID] = struct{}{}
		placeholders[index] = "?"
		arguments = append(arguments, planID)
	}
	query := `SELECT plan_id, plan_item_ordinal, plan_item_sha256, COUNT(*),
		SUM(CASE WHEN evidence_outcome = 'pass' THEN 1 ELSE 0 END),
		SUM(CASE WHEN evidence_outcome = 'fail' THEN 1 ELSE 0 END),
		SUM(CASE WHEN evidence_outcome = 'unknown' THEN 1 ELSE 0 END),
		MAX(event_sequence)
		FROM operator_verification_plan_evidence_associations
		WHERE run_id = ? AND plan_id IN (` + strings.Join(placeholders, ",") + `)
		GROUP BY plan_id, plan_item_ordinal, plan_item_sha256
		ORDER BY plan_id, plan_item_ordinal`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]verification.PlanItemCoverageCount, 0, len(planIDs)*verification.MaxPlanItems)
	for rows.Next() {
		var value verification.PlanItemCoverageCount
		if err := rows.Scan(&value.PlanID, &value.PlanItemOrdinal, &value.PlanItemSHA256,
			&value.AssociatedEvidenceCount, &value.PassCount, &value.FailCount,
			&value.UnknownCount, &value.LatestAssociationEventSequence); err != nil {
			return nil, err
		}
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("stored verification coverage is invalid: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) RecordVerificationPlanEvidenceAssociation(ctx context.Context,
	association verification.PlanEvidenceAssociation,
) (verification.PlanEvidenceAssociation, bool, error) {
	prepared := association
	prepared.EventSequence = association.EvidenceEventSequence + 1
	if association.EventSequence != 0 {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.New(apperror.CodeInvalidArgument,
				"new verification association cannot carry an event sequence")
	}
	if err := prepared.Validate(); err != nil {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument,
				"verification association is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`,
		association.RunID)
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	if rows != 1 {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.New(apperror.CodeNotFound, "verification association Run was not found")
	}
	existing, found, err := getVerificationAssociationByOperationTx(ctx, tx,
		association.OperationKeyDigest)
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	if found {
		if !sameVerificationAssociationIntent(existing, association) {
			return verification.PlanEvidenceAssociation{}, false,
				apperror.New(apperror.CodeConflict,
					"verification association operation key was used for different intent")
		}
		if err := tx.Commit(); err != nil {
			return verification.PlanEvidenceAssociation{}, false, err
		}
		return existing, true, nil
	}
	if linked, exists, err := getVerificationAssociationByEvidenceTx(ctx, tx,
		association.EvidenceID); err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	} else if exists {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.New(apperror.CodeConflict,
				"verification evidence is already associated with a plan item")
	} else if linked.ID != "" {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.New(apperror.CodeInternal, "verification association lookup is inconsistent")
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, association.RunID))
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	var workspaceID, sessionWorkspaceID, sessionStatus, surface string
	if err := tx.QueryRowContext(ctx, `SELECT mission.workspace_id, session_record.workspace_id,
		session_record.status, mode.surface
		FROM missions mission JOIN sessions session_record ON session_record.id = ?
		JOIN run_mode_snapshots mode ON mode.run_id = ?
		WHERE mission.id = ? AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
			WHERE later.run_id = mode.run_id AND later.revision > mode.revision)`,
		association.SessionID, run.ID, run.MissionID).Scan(&workspaceID,
		&sessionWorkspaceID, &sessionStatus, &surface); err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	if run.SessionID != association.SessionID || workspaceID != association.WorkspaceID ||
		sessionWorkspaceID != association.WorkspaceID || sessionStatus != session.StatusActive ||
		surface != string(domain.ExecutionSurfaceCode) {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.New(apperror.CodeConflict,
				"verification association Run, active Code Session, or Workspace binding changed")
	}
	plan, err := getVerificationPlanByIDTx(ctx, tx, association.PlanID)
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	evidence, err := getVerificationEvidenceByIDTx(ctx, tx, association.EvidenceID)
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	if plan.RunID != association.RunID || plan.SessionID != association.SessionID ||
		plan.WorkspaceID != association.WorkspaceID ||
		evidence.RunID != association.RunID || evidence.SessionID != association.SessionID ||
		evidence.WorkspaceID != association.WorkspaceID ||
		evidence.Outcome != association.EvidenceOutcome ||
		evidence.EventSequence != association.EvidenceEventSequence ||
		evidence.EventSequence <= plan.EventSequence ||
		association.PlanItemOrdinal > len(plan.Items) ||
		plan.Items[association.PlanItemOrdinal-1].ItemSHA256 != association.PlanItemSHA256 ||
		association.CreatedAt.Before(plan.CreatedAt) || association.CreatedAt.Before(evidence.CreatedAt) {
		return verification.PlanEvidenceAssociation{}, false,
			apperror.New(apperror.CodeConflict,
				"verification association plan, item, evidence, or causal binding changed")
	}
	event, err := events.New(run.ID, run.MissionID,
		events.VerificationPlanEvidenceAssociatedEvent,
		"operator_verification_association", association.ID, map[string]any{
			"plan_id": association.PlanID, "plan_item_ordinal": association.PlanItemOrdinal,
			"plan_item_sha256":        association.PlanItemSHA256,
			"evidence_id":             association.EvidenceID,
			"evidence_outcome":        association.EvidenceOutcome,
			"evidence_event_sequence": association.EvidenceEventSequence,
			"operator_associated":     true, "command_executed": false,
			"model_assertion": false, "result_inferred": false,
			"record_rewritten": false, "approval": false, "authority_granted": false,
		})
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	event.CreatedAt = association.CreatedAt
	event, err = insertRunEventTx(ctx, tx, event)
	if err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	association.EventSequence = event.Sequence
	if err := association.Validate(); err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operator_verification_plan_evidence_associations
		(id, protocol_version, operation_key_digest, request_fingerprint, run_id, session_id,
		workspace_id, plan_id, plan_item_ordinal, plan_item_sha256, evidence_id,
		evidence_outcome, evidence_event_sequence, associated_by, event_sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, association.ID,
		association.ProtocolVersion, association.OperationKeyDigest, association.RequestFingerprint,
		association.RunID, association.SessionID, association.WorkspaceID, association.PlanID,
		association.PlanItemOrdinal, association.PlanItemSHA256, association.EvidenceID,
		association.EvidenceOutcome, association.EvidenceEventSequence, association.AssociatedBy,
		association.EventSequence, ts(association.CreatedAt))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
			return verification.PlanEvidenceAssociation{}, false,
				apperror.New(apperror.CodeConflict,
					"verification evidence is already associated with a plan item")
		}
		return verification.PlanEvidenceAssociation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return verification.PlanEvidenceAssociation{}, false, err
	}
	return association, false, nil
}

func scanVerificationAssociation(row scanner) (verification.PlanEvidenceAssociation, error) {
	var value verification.PlanEvidenceAssociation
	var outcome, created string
	if err := row.Scan(&value.ID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.RequestFingerprint, &value.RunID, &value.SessionID, &value.WorkspaceID,
		&value.PlanID, &value.PlanItemOrdinal, &value.PlanItemSHA256, &value.EvidenceID,
		&outcome, &value.EvidenceEventSequence, &value.AssociatedBy, &value.EventSequence,
		&created); err != nil {
		return verification.PlanEvidenceAssociation{}, err
	}
	value.EvidenceOutcome = verification.Outcome(outcome)
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return verification.PlanEvidenceAssociation{},
			fmt.Errorf("stored verification association is invalid: %w", err)
	}
	return value, nil
}

func getVerificationAssociationByOperationTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (verification.PlanEvidenceAssociation, bool, error) {
	value, err := scanVerificationAssociation(tx.QueryRowContext(ctx,
		verificationAssociationSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.PlanEvidenceAssociation{}, false, nil
	}
	return value, err == nil, err
}

func getVerificationAssociationByEvidenceTx(ctx context.Context, tx *sql.Tx,
	evidenceID string,
) (verification.PlanEvidenceAssociation, bool, error) {
	value, err := scanVerificationAssociation(tx.QueryRowContext(ctx,
		verificationAssociationSelect+` WHERE evidence_id = ?`, evidenceID))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.PlanEvidenceAssociation{}, false, nil
	}
	return value, err == nil, err
}

func getVerificationPlanByIDTx(ctx context.Context, tx *sql.Tx,
	id string,
) (verification.Plan, error) {
	value, itemCount, err := scanVerificationPlanHeader(tx.QueryRowContext(ctx,
		verificationPlanSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.Plan{}, apperror.New(apperror.CodeNotFound,
			"verification plan was not found")
	}
	if err != nil {
		return verification.Plan{}, err
	}
	value.Items, err = loadVerificationPlanItems(ctx, tx, value.ID, itemCount)
	if err != nil {
		return verification.Plan{}, err
	}
	if err := value.Validate(); err != nil {
		return verification.Plan{}, err
	}
	return value, nil
}

func getVerificationEvidenceByIDTx(ctx context.Context, tx *sql.Tx,
	id string,
) (verification.Evidence, error) {
	value, err := scanVerificationEvidence(tx.QueryRowContext(ctx,
		verificationEvidenceSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return verification.Evidence{}, apperror.New(apperror.CodeNotFound,
			"verification evidence was not found")
	}
	return value, err
}

func sameVerificationAssociationIntent(left verification.PlanEvidenceAssociation,
	right verification.PlanEvidenceAssociation,
) bool {
	return left.ProtocolVersion == right.ProtocolVersion &&
		left.OperationKeyDigest == right.OperationKeyDigest &&
		left.RequestFingerprint == right.RequestFingerprint && left.RunID == right.RunID &&
		left.SessionID == right.SessionID && left.WorkspaceID == right.WorkspaceID &&
		left.PlanID == right.PlanID && left.PlanItemOrdinal == right.PlanItemOrdinal &&
		left.PlanItemSHA256 == right.PlanItemSHA256 && left.EvidenceID == right.EvidenceID &&
		left.EvidenceOutcome == right.EvidenceOutcome &&
		left.EvidenceEventSequence == right.EvidenceEventSequence &&
		left.AssociatedBy == right.AssociatedBy
}
