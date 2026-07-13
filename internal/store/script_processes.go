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
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/scriptprocess"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolgateway"
)

const defaultScriptProcessListLimit = 100
const maxScriptProcessListLimit = 500

func (s *SQLiteStore) CreateScriptProcessRun(ctx context.Context, request toolgateway.ScriptRunStoreRequest) (toolgateway.ScriptRunStoreResult, error) {
	process, err := normalizeStoredScriptProcess(request.Process)
	if err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	linkedSession := session.Session{
		ID: request.Session.ID, WorkspaceID: request.Session.WorkspaceID, Title: request.Session.Title,
		Route: request.Session.Route, Status: request.Session.Status,
		CreatedAt: request.Session.CreatedAt, UpdatedAt: request.Session.UpdatedAt,
	}
	if err := linkedSession.Validate(); err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	if process.RunID != request.Run.ID || process.SessionID != linkedSession.ID ||
		process.WorkspaceID != request.Mission.WorkspaceID {
		return toolgateway.ScriptRunStoreResult{}, errors.New("script process and Run creation identities do not match")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	existing, found, err := getScriptProcessByOperationTx(ctx, tx, process.OperationKeyDigest)
	if err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	if found {
		if existing.RequestFingerprint != process.RequestFingerprint {
			return toolgateway.ScriptRunStoreResult{}, apperror.New(apperror.CodeConflict,
				"script Run idempotency key was already used for a different request")
		}
		mission, err := scanMission(tx.QueryRowContext(ctx, `SELECT id, goal, profile, workspace_id, scope_json,
			created_at, updated_at FROM missions WHERE id = (SELECT mission_id FROM runs WHERE id = ?)`, existing.RunID))
		if err != nil {
			return toolgateway.ScriptRunStoreResult{}, err
		}
		run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
			started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, existing.RunID))
		if err != nil {
			return toolgateway.ScriptRunStoreResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return toolgateway.ScriptRunStoreResult{}, err
		}
		return toolgateway.ScriptRunStoreResult{Mission: mission, Run: run, Process: existing, Replayed: true}, nil
	}

	if err := createMissionRunTx(ctx, tx, request.Mission, request.Run, request.Mode, linkedSession,
		request.CreateSession, request.InitialEvents); err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	if err := insertInitialScriptToolChargeTx(ctx, tx, request.Mission, request.Run, process); err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	if err := insertScriptProcessTx(ctx, tx, process); err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	if err := projectScriptProcessTx(ctx, tx, process, "", false); err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	if err := syncScriptProcessApprovalTx(ctx, tx, process, "", false); err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return toolgateway.ScriptRunStoreResult{}, err
	}
	return toolgateway.ScriptRunStoreResult{
		Mission: request.Mission, Run: request.Run, Process: process,
	}, nil
}

func (s *SQLiteStore) SaveScriptProcess(ctx context.Context, process scriptprocess.Process) (scriptprocess.Process, error) {
	normalized, err := normalizeStoredScriptProcess(process)
	if err != nil {
		return scriptprocess.Process{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return scriptprocess.Process{}, err
	}
	defer func() { _ = tx.Rollback() }()
	previous, found, err := getScriptProcessTx(ctx, tx, normalized.ID)
	if err != nil {
		return scriptprocess.Process{}, err
	}
	previousStatus := ""
	if found {
		previousStatus = string(previous.Status)
		if err := validateScriptProcessTransition(previous, normalized); err != nil {
			return scriptprocess.Process{}, err
		}
	} else if normalized.Status != scriptprocess.StatusProposed && normalized.Status != scriptprocess.StatusDenied {
		return scriptprocess.Process{}, errors.New("new script process must begin as proposed or policy-denied")
	}
	if found {
		result, err := tx.ExecContext(ctx, `UPDATE script_process_proposals SET status = ?, risk = ?, policy_reason = ?,
			stdout = ?, stderr = ?, exit_code = ?, version = ?, updated_at = ? WHERE id = ? AND version = ?`,
			normalized.Status, normalized.Risk, normalized.PolicyReason, normalized.Stdout, normalized.Stderr,
			normalized.ExitCode, normalized.Version, ts(normalized.UpdatedAt), normalized.ID, previous.Version)
		if err != nil {
			return scriptprocess.Process{}, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return scriptprocess.Process{}, err
		}
		if rows != 1 {
			return scriptprocess.Process{}, errors.New("script process changed concurrently")
		}
	} else if err := insertScriptProcessTx(ctx, tx, normalized); err != nil {
		return scriptprocess.Process{}, err
	}
	if err := projectScriptProcessTx(ctx, tx, normalized, previousStatus, found); err != nil {
		return scriptprocess.Process{}, err
	}
	if err := syncScriptProcessApprovalTx(ctx, tx, normalized, previousStatus, found); err != nil {
		return scriptprocess.Process{}, err
	}
	if err := tx.Commit(); err != nil {
		return scriptprocess.Process{}, err
	}
	return normalized, nil
}

func (s *SQLiteStore) GetScriptProcess(ctx context.Context, id string) (scriptprocess.Process, error) {
	id = strings.TrimSpace(id)
	if id == "" || len([]rune(id)) > scriptprocess.MaxIdentityRunes {
		return scriptprocess.Process{}, errors.New("script process id is required and bounded")
	}
	return getScriptProcessRow(s.db.QueryRowContext(ctx, scriptProcessSelect+` WHERE id = ?`, id))
}

func (s *SQLiteStore) ListScriptProcesses(ctx context.Context, filter scriptprocess.ListFilter) ([]scriptprocess.Process, error) {
	filter.RunID = strings.TrimSpace(filter.RunID)
	filter.SessionID = strings.TrimSpace(filter.SessionID)
	for label, value := range map[string]string{"run id": filter.RunID, "session id": filter.SessionID} {
		if len([]rune(value)) > scriptprocess.MaxIdentityRunes {
			return nil, fmt.Errorf("script process %s exceeds %d characters", label, scriptprocess.MaxIdentityRunes)
		}
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, fmt.Errorf("invalid script process status %q", filter.Status)
	}
	if filter.Limit < 0 || filter.Limit > maxScriptProcessListLimit {
		return nil, fmt.Errorf("script process limit must be between 0 and %d", maxScriptProcessListLimit)
	}
	if filter.Limit == 0 {
		filter.Limit = defaultScriptProcessListLimit
	}
	query := scriptProcessSelect + ` WHERE 1=1`
	var args []any
	if filter.RunID != "" {
		query += ` AND run_id = ?`
		args = append(args, filter.RunID)
	}
	if filter.SessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, filter.SessionID)
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
	var processes []scriptprocess.Process
	for rows.Next() {
		process, err := getScriptProcessRow(rows)
		if err != nil {
			return nil, err
		}
		processes = append(processes, process)
	}
	return processes, rows.Err()
}

func normalizeStoredScriptProcess(process scriptprocess.Process) (scriptprocess.Process, error) {
	process.ID = strings.TrimSpace(process.ID)
	process.RunID = strings.TrimSpace(process.RunID)
	process.SessionID = strings.TrimSpace(process.SessionID)
	process.WorkspaceID = strings.TrimSpace(process.WorkspaceID)
	process.RequestedBy = redact.String(strings.TrimSpace(process.RequestedBy))
	process.Executable = redact.String(process.Executable)
	process.Arguments = append([]string(nil), process.Arguments...)
	for index := range process.Arguments {
		process.Arguments[index] = redact.String(process.Arguments[index])
	}
	process.PolicyReason = redact.String(strings.TrimSpace(process.PolicyReason))
	process.Stdout = redact.String(process.Stdout)
	process.Stderr = redact.String(process.Stderr)
	proposal, err := scriptprocess.NormalizeProposal(process.Proposal())
	if err != nil {
		return scriptprocess.Process{}, err
	}
	process.Executable = proposal.Executable
	process.Arguments = proposal.Arguments
	process.WorkingDirectory = proposal.WorkingDirectory
	process.RequestedBackend = proposal.RequestedBackend
	process.ExecutionMode = scriptprocess.ExecutionDisabled
	if process.CreatedAt.IsZero() {
		process.CreatedAt = time.Now().UTC()
	}
	if process.UpdatedAt.IsZero() {
		process.UpdatedAt = process.CreatedAt
	}
	if err := process.Validate(); err != nil {
		return scriptprocess.Process{}, err
	}
	return process, nil
}

func insertScriptProcessTx(ctx context.Context, tx *sql.Tx, process scriptprocess.Process) error {
	arguments, err := json.Marshal(process.Arguments)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO script_process_proposals
		(id, operation_key_digest, run_id, session_id, workspace_id, executable, arguments_json, working_directory,
		 requested_backend, execution_mode, status, risk, policy_reason, stdout, stderr, exit_code,
		 request_fingerprint, approval_fingerprint, requested_by, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		process.ID, process.OperationKeyDigest, process.RunID, process.SessionID, process.WorkspaceID,
		process.Executable, string(arguments), process.WorkingDirectory, process.RequestedBackend,
		process.ExecutionMode, process.Status, process.Risk, process.PolicyReason, process.Stdout,
		process.Stderr, process.ExitCode, process.RequestFingerprint, process.ApprovalFingerprint,
		process.RequestedBy, process.Version, ts(process.CreatedAt), ts(process.UpdatedAt))
	return err
}

func validateScriptProcessTransition(previous scriptprocess.Process, next scriptprocess.Process) error {
	if previous.ID != next.ID || previous.OperationKeyDigest != next.OperationKeyDigest || previous.RunID != next.RunID ||
		previous.SessionID != next.SessionID || previous.WorkspaceID != next.WorkspaceID ||
		previous.Executable != next.Executable || !stringSlicesEqual(previous.Arguments, next.Arguments) ||
		previous.WorkingDirectory != next.WorkingDirectory || previous.RequestedBackend != next.RequestedBackend ||
		previous.ExecutionMode != next.ExecutionMode || previous.RequestFingerprint != next.RequestFingerprint ||
		previous.ApprovalFingerprint != next.ApprovalFingerprint || previous.RequestedBy != next.RequestedBy ||
		!previous.CreatedAt.Equal(next.CreatedAt) {
		return errors.New("script process immutable identity or request changed")
	}
	if next.Version != previous.Version+1 {
		return errors.New("script process version changed concurrently")
	}
	allowed := map[scriptprocess.Status]map[scriptprocess.Status]bool{
		scriptprocess.StatusProposed: {scriptprocess.StatusApproved: true, scriptprocess.StatusDenied: true},
		scriptprocess.StatusApproved: {scriptprocess.StatusCompleted: true, scriptprocess.StatusFailed: true},
	}
	if !allowed[previous.Status][next.Status] {
		return fmt.Errorf("script process cannot transition from %s to %s", previous.Status, next.Status)
	}
	return nil
}

func projectScriptProcessTx(ctx context.Context, tx *sql.Tx, process scriptprocess.Process, previousStatus string, existed bool) error {
	if err := requireScriptProcessRunBindingTx(ctx, tx, process); err != nil {
		return err
	}
	if !existed {
		allowed := process.Status != scriptprocess.StatusDenied
		if err := appendRunEventForSessionTx(ctx, tx, process.SessionID, events.PolicyDecisionEvent,
			"policy", process.ID, map[string]any{
				"context": "tool_run.script_process", "allowed": allowed, "needs_approval": allowed,
				"risk": process.Risk, "reason": process.PolicyReason,
			}); err != nil {
			return err
		}
	}
	eventType, ok := scriptProcessEventType(process.Status)
	if !ok {
		return fmt.Errorf("invalid script process status %q", process.Status)
	}
	payload := map[string]any{
		"schema": scriptprocess.Schema, "session_id": process.SessionID, "workspace_id": process.WorkspaceID,
		"tool_name": string(toolgateway.ScriptProcessTool), "executable": process.Executable,
		"arguments": process.Arguments, "working_directory": process.WorkingDirectory,
		"requested_backend": process.RequestedBackend, "execution_mode": process.ExecutionMode,
		"status": process.Status, "previous_status": previousStatus, "risk": process.Risk,
		"reason": process.PolicyReason, "request_fingerprint": process.RequestFingerprint,
	}
	if process.Status == scriptprocess.StatusCompleted || process.Status == scriptprocess.StatusFailed {
		payload["stdout"] = process.Stdout
		payload["stderr"] = process.Stderr
		payload["exit_code"] = process.ExitCode
	}
	return appendRunEventForSessionTx(ctx, tx, process.SessionID, eventType, "script_process_store", process.ID, payload)
}

func scriptProcessEventType(status scriptprocess.Status) (string, bool) {
	switch status {
	case scriptprocess.StatusProposed:
		return events.ToolProposedEvent, true
	case scriptprocess.StatusApproved:
		return events.ToolApprovedEvent, true
	case scriptprocess.StatusDenied:
		return events.ToolDeniedEvent, true
	case scriptprocess.StatusCompleted:
		return events.ToolCompletedEvent, true
	case scriptprocess.StatusFailed:
		return events.ToolFailedEvent, true
	default:
		return "", false
	}
}

func syncScriptProcessApprovalTx(ctx context.Context, tx *sql.Tx, process scriptprocess.Process, previousStatus string, existed bool) error {
	status, mode, reviewer, decidedAt, err := approvalStateForScriptProcess(process, existed)
	if err != nil {
		return err
	}
	proposal := approval.Proposal{
		IdempotencyKey: approval.ProposalIdempotencyKey(string(toolgateway.ScriptProcessTool), process.ID),
		ProposalID:     process.ID, SessionID: process.SessionID, WorkspaceID: process.WorkspaceID,
		ToolName: string(toolgateway.ScriptProcessTool), ActionClass: string(toolgateway.ClassProcess),
		Mode: mode, Status: status, RequestFingerprint: process.ApprovalFingerprint,
		DecisionReason: process.PolicyReason, RequestedBy: process.RequestedBy, ReviewedBy: reviewer,
		CreatedAt: process.CreatedAt, UpdatedAt: process.UpdatedAt, DecidedAt: decidedAt,
	}
	if !existed || previousStatus == string(process.Status) {
		_, _, err := ensureApprovalTx(ctx, tx, proposal)
		return err
	}
	return requireApprovalStatusTx(ctx, tx, proposal, status)
}

func approvalStateForScriptProcess(process scriptprocess.Process, existed bool) (approval.Status, string, string, *time.Time, error) {
	if !existed && process.Status != scriptprocess.StatusProposed && process.Status != scriptprocess.StatusDenied {
		return "", "", "", nil, errors.New("new script process must begin as proposed or policy-denied")
	}
	switch process.Status {
	case scriptprocess.StatusProposed:
		return approval.StatusPending, string(toolgateway.ApprovalPerCall), "", nil, nil
	case scriptprocess.StatusDenied:
		decided := process.UpdatedAt
		if existed {
			return approval.StatusDenied, string(toolgateway.ApprovalPerCall), "operator", &decided, nil
		}
		return approval.StatusDenied, string(toolgateway.ApprovalNever), "policy", &decided, nil
	case scriptprocess.StatusApproved, scriptprocess.StatusCompleted, scriptprocess.StatusFailed:
		decided := process.UpdatedAt
		return approval.StatusApproved, string(toolgateway.ApprovalPerCall), "operator", &decided, nil
	default:
		return "", "", "", nil, fmt.Errorf("invalid script process status %q", process.Status)
	}
}

func insertInitialScriptToolChargeTx(ctx context.Context, tx *sql.Tx, mission domain.Mission, run domain.Run, process scriptprocess.Process) error {
	now := process.CreatedAt
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_tool_usage (run_id, consumed, updated_at)
		VALUES (?, 1, ?)`, run.ID, ts(now)); err != nil {
		return err
	}
	chargeID := idgen.New("toolcall")
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_tool_calls
		(id, run_id, session_id, workspace_id, tool_name, action_class, sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?)`, chargeID, run.ID, run.SessionID, mission.WorkspaceID,
		toolgateway.ScriptProcessTool, toolgateway.ClassProcess, ts(now)); err != nil {
		return err
	}
	event, err := events.New(run.ID, mission.ID, events.ToolBudgetChargedEvent, "tool_budget", chargeID, map[string]any{
		"charge_id": chargeID, "session_id": run.SessionID, "workspace_id": mission.WorkspaceID,
		"tool_name": toolgateway.ScriptProcessTool, "action_class": toolgateway.ClassProcess,
		"consumed": 1, "limit": run.Budget.MaxToolCalls,
	})
	if err != nil {
		return err
	}
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}

const scriptProcessSelect = `SELECT id, operation_key_digest, run_id, session_id, workspace_id, executable,
	arguments_json, working_directory, requested_backend, execution_mode, status, risk, policy_reason,
	stdout, stderr, exit_code, request_fingerprint, approval_fingerprint, requested_by, version,
	created_at, updated_at FROM script_process_proposals`

func getScriptProcessTx(ctx context.Context, tx *sql.Tx, id string) (scriptprocess.Process, bool, error) {
	process, err := getScriptProcessRow(tx.QueryRowContext(ctx, scriptProcessSelect+` WHERE id = ?`, strings.TrimSpace(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return scriptprocess.Process{}, false, nil
	}
	return process, err == nil, err
}

func getScriptProcessByOperationTx(ctx context.Context, tx *sql.Tx, operationKey string) (scriptprocess.Process, bool, error) {
	process, err := getScriptProcessRow(tx.QueryRowContext(ctx, scriptProcessSelect+` WHERE operation_key_digest = ?`, operationKey))
	if errors.Is(err, sql.ErrNoRows) {
		return scriptprocess.Process{}, false, nil
	}
	return process, err == nil, err
}

func requireScriptProcessRunBindingTx(ctx context.Context, tx *sql.Tx, process scriptprocess.Process) error {
	binding, found, err := runBindingForSessionTx(ctx, tx, process.SessionID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("script process session is not attached to a run")
	}
	if binding.RunID != process.RunID {
		return errors.New("script process run does not match its attached session")
	}
	if strings.TrimSpace(binding.WorkspaceID) != strings.TrimSpace(process.WorkspaceID) {
		return errors.New("script process workspace does not match its attached run")
	}
	return nil
}

func getScriptProcessRow(row scanner) (scriptprocess.Process, error) {
	var process scriptprocess.Process
	var argumentsJSON, createdAt, updatedAt string
	if err := row.Scan(&process.ID, &process.OperationKeyDigest, &process.RunID, &process.SessionID,
		&process.WorkspaceID, &process.Executable, &argumentsJSON, &process.WorkingDirectory,
		&process.RequestedBackend, &process.ExecutionMode, &process.Status, &process.Risk,
		&process.PolicyReason, &process.Stdout, &process.Stderr, &process.ExitCode,
		&process.RequestFingerprint, &process.ApprovalFingerprint, &process.RequestedBy,
		&process.Version, &createdAt, &updatedAt); err != nil {
		return scriptprocess.Process{}, err
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &process.Arguments); err != nil {
		return scriptprocess.Process{}, fmt.Errorf("decode script process arguments: %w", err)
	}
	process.CreatedAt = parseTS(createdAt)
	process.UpdatedAt = parseTS(updatedAt)
	if err := process.Validate(); err != nil {
		return scriptprocess.Process{}, err
	}
	return process, nil
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
