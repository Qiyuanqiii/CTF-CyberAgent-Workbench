package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
)

const defaultGrantListLimit = 100
const maxGrantListLimit = 500

func (s *SQLiteStore) CreateSessionGrant(ctx context.Context, request approval.CreateGrantRequest) (approval.GrantResult, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return approval.GrantResult{}, err
	}
	normalized.Reason = redact.String(normalized.Reason)
	normalized.GrantedBy = redact.String(normalized.GrantedBy)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return approval.GrantResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	binding, bound, err := runBindingForSessionTx(ctx, tx, normalized.SessionID)
	if err != nil {
		return approval.GrantResult{}, err
	}
	if !bound {
		return approval.GrantResult{}, errors.New("session approval grants require a Run-bound session")
	}
	if normalized.WorkspaceID == "" {
		normalized.WorkspaceID = binding.WorkspaceID
	}
	if normalized.WorkspaceID != binding.WorkspaceID {
		return approval.GrantResult{}, errors.New("approval grant workspace does not match the attached run")
	}
	if !grantToolClassMatches(normalized.ToolName, normalized.ActionClass) {
		return approval.GrantResult{}, errors.New("approval grant tool and action class do not match")
	}
	fingerprint := approval.GrantRequestFingerprint(normalized)
	operationKey := approval.GrantOperationKeyDigest(normalized.IdempotencyKey)
	operation, found, err := getGrantOperationTx(ctx, tx, operationKey)
	if err != nil {
		return approval.GrantResult{}, err
	}
	if found {
		if operation.Action != "grant" || operation.RequestFingerprint != fingerprint || operation.ResultStatus != approval.GrantActive {
			return approval.GrantResult{}, errors.New("approval grant idempotency key was already used for a different operation")
		}
		grant, err := getSessionGrantTx(ctx, tx, operation.GrantID)
		if err != nil {
			return approval.GrantResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return approval.GrantResult{}, err
		}
		return approval.GrantResult{Grant: grant, Replayed: true}, nil
	}
	if err := requireGrantableRunTx(ctx, tx, binding.RunID); err != nil {
		return approval.GrantResult{}, err
	}
	grant, found, err := findActiveSessionGrant(ctx, tx, approval.GrantQuery{
		RunID: binding.RunID, SessionID: normalized.SessionID, WorkspaceID: normalized.WorkspaceID,
		ToolName: normalized.ToolName, ActionClass: normalized.ActionClass,
	})
	if err != nil {
		return approval.GrantResult{}, err
	}
	if !found {
		now := time.Now().UTC()
		grant = approval.SessionGrant{
			ID: idgen.New("grant"), RunID: binding.RunID, SessionID: normalized.SessionID,
			WorkspaceID: normalized.WorkspaceID, ToolName: normalized.ToolName, ActionClass: normalized.ActionClass,
			Status: approval.GrantActive, RequestFingerprint: fingerprint, Reason: normalized.Reason,
			GrantedBy: normalized.GrantedBy, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := grant.Validate(); err != nil {
			return approval.GrantResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO approval_session_grants
			(id, run_id, session_id, workspace_id, tool_name, action_class, status, request_fingerprint,
			 reason, revocation_reason, granted_by, revoked_by, version, created_at, updated_at, revoked_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, '', ?, ?, ?, NULL)`,
			grant.ID, grant.RunID, grant.SessionID, grant.WorkspaceID, grant.ToolName, grant.ActionClass,
			grant.Status, grant.RequestFingerprint, grant.Reason, grant.GrantedBy, grant.Version,
			ts(grant.CreatedAt), ts(grant.UpdatedAt)); err != nil {
			return approval.GrantResult{}, err
		}
		if err := appendGrantEventTx(ctx, tx, grant, events.ApprovalGrantCreatedEvent); err != nil {
			return approval.GrantResult{}, err
		}
	}
	if err := insertGrantOperationTx(ctx, tx, operationKey, grant.ID, "grant", fingerprint, approval.GrantActive); err != nil {
		return approval.GrantResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return approval.GrantResult{}, err
	}
	return approval.GrantResult{Grant: grant, Replayed: found}, nil
}

func (s *SQLiteStore) RevokeSessionGrant(ctx context.Context, request approval.RevokeGrantRequest) (approval.GrantResult, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return approval.GrantResult{}, err
	}
	normalized.Reason = redact.String(normalized.Reason)
	normalized.RevokedBy = redact.String(normalized.RevokedBy)
	fingerprint := approval.GrantRevocationFingerprint(normalized)
	operationKey := approval.GrantOperationKeyDigest(normalized.IdempotencyKey)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return approval.GrantResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	operation, found, err := getGrantOperationTx(ctx, tx, operationKey)
	if err != nil {
		return approval.GrantResult{}, err
	}
	if found {
		if operation.Action != "revoke" || operation.GrantID != normalized.GrantID ||
			operation.RequestFingerprint != fingerprint || operation.ResultStatus != approval.GrantRevoked {
			return approval.GrantResult{}, errors.New("approval grant idempotency key was already used for a different operation")
		}
		grant, err := getSessionGrantTx(ctx, tx, operation.GrantID)
		if err != nil {
			return approval.GrantResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return approval.GrantResult{}, err
		}
		return approval.GrantResult{Grant: grant, Replayed: true}, nil
	}
	grant, err := getSessionGrantTx(ctx, tx, normalized.GrantID)
	if err != nil {
		return approval.GrantResult{}, err
	}
	changed := false
	if grant.Status == approval.GrantActive {
		now := time.Now().UTC()
		result, err := tx.ExecContext(ctx, `UPDATE approval_session_grants SET status = ?, revocation_reason = ?,
			revoked_by = ?, version = version + 1, updated_at = ?, revoked_at = ?
			WHERE id = ? AND version = ? AND status = ?`, approval.GrantRevoked, normalized.Reason,
			normalized.RevokedBy, ts(now), ts(now), grant.ID, grant.Version, approval.GrantActive)
		if err != nil {
			return approval.GrantResult{}, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return approval.GrantResult{}, err
		}
		if rows != 1 {
			return approval.GrantResult{}, errors.New("approval grant changed concurrently")
		}
		grant.Status = approval.GrantRevoked
		grant.RevocationReason = normalized.Reason
		grant.RevokedBy = normalized.RevokedBy
		grant.Version++
		grant.UpdatedAt = now
		grant.RevokedAt = &now
		changed = true
		if err := appendGrantEventTx(ctx, tx, grant, events.ApprovalGrantRevokedEvent); err != nil {
			return approval.GrantResult{}, err
		}
	}
	if err := grant.Validate(); err != nil {
		return approval.GrantResult{}, err
	}
	if err := insertGrantOperationTx(ctx, tx, operationKey, grant.ID, "revoke", fingerprint, approval.GrantRevoked); err != nil {
		return approval.GrantResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return approval.GrantResult{}, err
	}
	return approval.GrantResult{Grant: grant, Replayed: !changed}, nil
}

func (s *SQLiteStore) AuthorizeApprovalWithSessionGrant(ctx context.Context, proposalID string, grantID string) (approval.DecisionResult, error) {
	proposalID = strings.TrimSpace(proposalID)
	grantID = strings.TrimSpace(grantID)
	if err := validateApprovalFilterIdentity("proposal id", proposalID, false); err != nil {
		return approval.DecisionResult{}, err
	}
	if err := validateApprovalFilterIdentity("grant id", grantID, false); err != nil {
		return approval.DecisionResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return approval.DecisionResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getApprovalTx(ctx, tx, "", proposalID)
	if err != nil {
		return approval.DecisionResult{}, err
	}
	if record.Status == approval.StatusApproved && record.GrantID == grantID {
		if err := tx.Commit(); err != nil {
			return approval.DecisionResult{}, err
		}
		return approval.DecisionResult{Approval: record, Replayed: true}, nil
	}
	if record.Status != approval.StatusPending {
		return approval.DecisionResult{}, fmt.Errorf("approval %s is already %s", record.ID, record.Status)
	}
	grant, err := getSessionGrantTx(ctx, tx, grantID)
	if err != nil {
		return approval.DecisionResult{}, err
	}
	if grant.Status != approval.GrantActive || grant.RunID != record.RunID || grant.SessionID != record.SessionID ||
		grant.WorkspaceID != record.WorkspaceID || grant.ToolName != record.ToolName || grant.ActionClass != record.ActionClass {
		return approval.DecisionResult{}, errors.New("session grant does not authorize this approval scope")
	}
	if err := requireGrantableRunTx(ctx, tx, grant.RunID); err != nil {
		return approval.DecisionResult{}, err
	}
	now := time.Now().UTC()
	reason := "authorized by active session grant"
	result, err := tx.ExecContext(ctx, `UPDATE tool_approvals SET status = ?, grant_id = ?, decision_reason = ?,
		reviewed_by = ?, version = version + 1, updated_at = ?, decided_at = ?
		WHERE id = ? AND version = ? AND status = ? AND EXISTS
			(SELECT 1 FROM approval_session_grants WHERE id = ? AND status = ?)`,
		approval.StatusApproved, grant.ID, reason, "session_grant", ts(now), ts(now), record.ID,
		record.Version, approval.StatusPending, grant.ID, approval.GrantActive)
	if err != nil {
		return approval.DecisionResult{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return approval.DecisionResult{}, err
	}
	if rows != 1 {
		return approval.DecisionResult{}, errors.New("approval or session grant changed concurrently")
	}
	record.Status = approval.StatusApproved
	record.GrantID = grant.ID
	record.DecisionReason = reason
	record.ReviewedBy = "session_grant"
	record.Version++
	record.UpdatedAt = now
	record.DecidedAt = &now
	if err := record.Validate(); err != nil {
		return approval.DecisionResult{}, err
	}
	if err := appendApprovalEventTx(ctx, tx, record, events.ApprovalDecidedEvent); err != nil {
		return approval.DecisionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return approval.DecisionResult{}, err
	}
	return approval.DecisionResult{Approval: record}, nil
}

func (s *SQLiteStore) FindActiveSessionGrant(ctx context.Context, query approval.GrantQuery) (approval.SessionGrant, bool, error) {
	return findActiveSessionGrant(ctx, s.db, query)
}

func (s *SQLiteStore) GetSessionGrant(ctx context.Context, id string) (approval.SessionGrant, error) {
	id = strings.TrimSpace(id)
	if err := validateApprovalFilterIdentity("grant id", id, false); err != nil {
		return approval.SessionGrant{}, err
	}
	return getSessionGrantRow(s.db.QueryRowContext(ctx, grantSelect+` WHERE id = ?`, id))
}

func (s *SQLiteStore) ListSessionGrants(ctx context.Context, filter approval.GrantListFilter) ([]approval.SessionGrant, error) {
	filter.RunID = strings.TrimSpace(filter.RunID)
	filter.SessionID = strings.TrimSpace(filter.SessionID)
	filter.ToolName = strings.TrimSpace(filter.ToolName)
	for label, value := range map[string]string{"run id": filter.RunID, "session id": filter.SessionID, "tool name": filter.ToolName} {
		if err := validateApprovalFilterIdentity(label, value, true); err != nil {
			return nil, err
		}
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, fmt.Errorf("invalid approval grant status %q", filter.Status)
	}
	if filter.Limit < 0 || filter.Limit > maxGrantListLimit {
		return nil, fmt.Errorf("approval grant limit must be between 0 and %d", maxGrantListLimit)
	}
	if filter.Limit == 0 {
		filter.Limit = defaultGrantListLimit
	}
	query := grantSelect + ` WHERE 1=1`
	var args []any
	if filter.RunID != "" {
		query += ` AND run_id = ?`
		args = append(args, filter.RunID)
	}
	if filter.SessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, filter.SessionID)
	}
	if filter.ToolName != "" {
		query += ` AND tool_name = ?`
		args = append(args, filter.ToolName)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []approval.SessionGrant
	for rows.Next() {
		grant, err := getSessionGrantRow(rows)
		if err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func findActiveSessionGrant(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query approval.GrantQuery) (approval.SessionGrant, bool, error) {
	query.RunID = strings.TrimSpace(query.RunID)
	query.SessionID = strings.TrimSpace(query.SessionID)
	query.WorkspaceID = strings.TrimSpace(query.WorkspaceID)
	query.ToolName = strings.TrimSpace(query.ToolName)
	query.ActionClass = strings.TrimSpace(query.ActionClass)
	for label, value := range map[string]string{
		"run id": query.RunID, "session id": query.SessionID, "workspace id": query.WorkspaceID,
		"tool name": query.ToolName, "action class": query.ActionClass,
	} {
		if err := validateApprovalFilterIdentity(label, value, label == "run id" || label == "workspace id"); err != nil {
			return approval.SessionGrant{}, false, err
		}
	}
	if query.SessionID == "" || query.ToolName == "" || query.ActionClass == "" {
		return approval.SessionGrant{}, false, errors.New("session grant lookup requires session, tool, and action class")
	}
	sqlText := grantSelect + ` JOIN runs ON runs.id = approval_session_grants.run_id
		JOIN sessions ON sessions.id = approval_session_grants.session_id
		WHERE approval_session_grants.session_id = ? AND approval_session_grants.workspace_id = ?
		AND approval_session_grants.tool_name = ? AND approval_session_grants.action_class = ?
		AND approval_session_grants.status = ? AND sessions.status = ? AND runs.status NOT IN (?, ?, ?)`
	args := []any{query.SessionID, query.WorkspaceID, query.ToolName, query.ActionClass, approval.GrantActive,
		session.StatusActive, domain.RunCompleted, domain.RunFailed, domain.RunCancelled}
	if query.RunID != "" {
		sqlText += ` AND approval_session_grants.run_id = ?`
		args = append(args, query.RunID)
	}
	grant, err := getSessionGrantRow(queryer.QueryRowContext(ctx, sqlText, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return approval.SessionGrant{}, false, nil
	}
	return grant, err == nil, err
}

func requireGrantableRunTx(ctx context.Context, tx *sql.Tx, runID string) error {
	var status domain.RunStatus
	var sessionStatus string
	if err := tx.QueryRowContext(ctx, `SELECT runs.status, sessions.status FROM runs
		JOIN sessions ON sessions.id = runs.session_id WHERE runs.id = ?`, strings.TrimSpace(runID)).
		Scan(&status, &sessionStatus); err != nil {
		return err
	}
	if status == domain.RunCompleted || status == domain.RunFailed || status == domain.RunCancelled {
		return fmt.Errorf("run %s is terminal and cannot use session approval grants", runID)
	}
	if sessionStatus != session.StatusActive {
		return fmt.Errorf("run %s session is not active and cannot use session approval grants", runID)
	}
	return nil
}

func grantToolClassMatches(toolName string, actionClass string) bool {
	switch toolName {
	case "shell":
		return actionClass == "shell"
	case "replace_file":
		return actionClass == "workspace_write"
	default:
		return false
	}
}

func appendGrantEventTx(ctx context.Context, tx *sql.Tx, grant approval.SessionGrant, eventType string) error {
	return appendRunEventForSessionTx(ctx, tx, grant.SessionID, eventType, "approval_store", grant.ID, map[string]any{
		"grant_id": grant.ID, "run_id": grant.RunID, "session_id": grant.SessionID,
		"workspace_id": grant.WorkspaceID, "tool_name": grant.ToolName, "action_class": grant.ActionClass,
		"status": grant.Status, "reason": grant.Reason, "revocation_reason": grant.RevocationReason,
		"granted_by": grant.GrantedBy, "revoked_by": grant.RevokedBy, "version": grant.Version,
	})
}

const grantSelect = `SELECT approval_session_grants.id, approval_session_grants.run_id,
	approval_session_grants.session_id, approval_session_grants.workspace_id, approval_session_grants.tool_name,
	approval_session_grants.action_class, approval_session_grants.status, approval_session_grants.request_fingerprint,
	approval_session_grants.reason, approval_session_grants.revocation_reason, approval_session_grants.granted_by,
	approval_session_grants.revoked_by, approval_session_grants.version, approval_session_grants.created_at,
	approval_session_grants.updated_at, approval_session_grants.revoked_at FROM approval_session_grants`

func getSessionGrantTx(ctx context.Context, tx *sql.Tx, id string) (approval.SessionGrant, error) {
	return getSessionGrantRow(tx.QueryRowContext(ctx, grantSelect+` WHERE id = ?`, strings.TrimSpace(id)))
}

func getSessionGrantRow(row scanner) (approval.SessionGrant, error) {
	var grant approval.SessionGrant
	var createdAt, updatedAt string
	var revokedAt sql.NullString
	if err := row.Scan(&grant.ID, &grant.RunID, &grant.SessionID, &grant.WorkspaceID, &grant.ToolName,
		&grant.ActionClass, &grant.Status, &grant.RequestFingerprint, &grant.Reason, &grant.RevocationReason,
		&grant.GrantedBy, &grant.RevokedBy, &grant.Version, &createdAt, &updatedAt, &revokedAt); err != nil {
		return approval.SessionGrant{}, err
	}
	grant.CreatedAt = parseTS(createdAt)
	grant.UpdatedAt = parseTS(updatedAt)
	if revokedAt.Valid {
		parsed := parseTS(revokedAt.String)
		grant.RevokedAt = &parsed
	}
	if err := grant.Validate(); err != nil {
		return approval.SessionGrant{}, err
	}
	return grant, nil
}

type grantOperation struct {
	GrantID            string
	Action             string
	RequestFingerprint string
	ResultStatus       approval.GrantStatus
}

func getGrantOperationTx(ctx context.Context, tx *sql.Tx, operationKey string) (grantOperation, bool, error) {
	var operation grantOperation
	err := tx.QueryRowContext(ctx, `SELECT grant_id, action, request_fingerprint, result_status
		FROM approval_grant_operations WHERE operation_key = ?`, operationKey).
		Scan(&operation.GrantID, &operation.Action, &operation.RequestFingerprint, &operation.ResultStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return grantOperation{}, false, nil
	}
	return operation, err == nil, err
}

func insertGrantOperationTx(ctx context.Context, tx *sql.Tx, operationKey string, grantID string, action string,
	fingerprint string, result approval.GrantStatus) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO approval_grant_operations
		(operation_key, grant_id, action, request_fingerprint, result_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, operationKey, grantID, action, fingerprint, result, ts(time.Now().UTC()))
	return err
}
