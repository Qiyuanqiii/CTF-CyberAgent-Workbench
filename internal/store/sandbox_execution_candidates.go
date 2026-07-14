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
	"cyberagent-workbench/internal/sandbox"
)

const sandboxExecutionCandidateSelect = `SELECT id, preparation_id, run_id, mission_id,
	workspace_id, protocol_version, manifest_fingerprint, authorization_fingerprint,
	workspace_fingerprint, scope_fingerprint, policy_fingerprint, mount_binding_fingerprint,
	approval_id, approval_status, mount_count, regular_file_mount_count, directory_mount_count,
	tokens_used, execution_millis_used, tool_calls_used, budget_checked, lease_quiescent,
	backend_enabled, execution_authorized, requested_by, validated_at
	FROM sandbox_execution_candidates`

func (s *SQLiteStore) GetSandboxExecutionCandidate(ctx context.Context,
	id string,
) (sandbox.ValidatedExecutionCandidate, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.ValidatedExecutionCandidate{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution candidate id is invalid")
	}
	value, err := getSandboxExecutionCandidate(ctx, s.db, id)
	return sandbox.ValidatedExecutionCandidate{Candidate: value}, err
}

func (s *SQLiteStore) ListSandboxExecutionCandidates(ctx context.Context, runID string,
	limit int,
) ([]sandbox.ValidatedExecutionCandidate, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution candidate Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 0 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution candidate list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, sandboxExecutionCandidateSelect+`
		WHERE run_id = ? ORDER BY validated_at, id LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sandbox.ValidatedExecutionCandidate, 0)
	for rows.Next() {
		candidate, err := scanSandboxExecutionCandidate(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, sandbox.ValidatedExecutionCandidate{Candidate: candidate})
	}
	return values, rows.Err()
}

func (s *SQLiteStore) GetSandboxExecutionCandidateOperation(ctx context.Context,
	keyDigest string,
) (sandbox.CandidateOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.CandidateOperation{}, false, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution candidate operation digest is invalid")
	}
	return getSandboxExecutionCandidateOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateSandboxExecutionCandidate(ctx context.Context,
	candidate sandbox.ExecutionCandidate, operation sandbox.CandidateOperation,
) (sandbox.ValidatedExecutionCandidate, bool, error) {
	if err := validateSandboxExecutionCandidateMutation(candidate, operation); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, candidate.RunID); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if existing, found, err := getSandboxExecutionCandidateOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	} else if found {
		return replaySandboxExecutionCandidate(ctx, tx, existing, operation)
	}
	intent, err := getSandboxManifestIntent(ctx, tx, candidate.PreparationID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	run, mission, err := getCoordinatorRunTx(ctx, tx, candidate.RunID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if run.Terminal() || run.MissionID != candidate.MissionID || mission.ID != candidate.MissionID ||
		mission.WorkspaceID != candidate.WorkspaceID || intent.Preparation.RunID != candidate.RunID ||
		intent.Preparation.MissionID != candidate.MissionID ||
		intent.Preparation.WorkspaceID != candidate.WorkspaceID || !intent.Validation.PolicyAllowed ||
		intent.Preparation.ManifestFingerprint != candidate.ManifestFingerprint ||
		intent.Preparation.AuthorizationFingerprint != candidate.AuthorizationFingerprint ||
		intent.Preparation.WorkspaceFingerprint != candidate.WorkspaceFingerprint ||
		intent.Preparation.ScopeFingerprint != candidate.ScopeFingerprint ||
		intent.Validation.PolicyFingerprint != candidate.PolicyFingerprint {
		return sandbox.ValidatedExecutionCandidate{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution candidate does not match its immutable Run and preparation binding")
	}
	if err := validateSandboxCandidateApprovalTx(ctx, tx, run, intent, candidate); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	usage, err := getRunAgentUsageTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	toolCalls, err := getRunToolCallCountTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if usage.TotalTokens != candidate.TokensUsed ||
		usage.TotalExecutionMillis != candidate.ExecutionMillisUsed ||
		toolCalls != candidate.ToolCallsUsed {
		return sandbox.ValidatedExecutionCandidate{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution candidate usage changed during validation")
	}
	if err := requireSandboxCandidateStoreBudget(run.Budget, usage, toolCalls); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if lease, found, err := getRunExecutionLeaseTx(ctx, tx, run.ID); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	} else if found && lease.ActiveAt(time.Now().UTC()) {
		return sandbox.ValidatedExecutionCandidate{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox execution candidate requires a quiescent Run")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_candidates
		(id, preparation_id, run_id, mission_id, workspace_id, protocol_version,
		manifest_fingerprint, authorization_fingerprint, workspace_fingerprint, scope_fingerprint,
		policy_fingerprint, mount_binding_fingerprint, approval_id, approval_status,
		mount_count, regular_file_mount_count, directory_mount_count, tokens_used,
		execution_millis_used, tool_calls_used, budget_checked, lease_quiescent,
		backend_enabled, execution_authorized, requested_by, validated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		candidate.ID, candidate.PreparationID, candidate.RunID, candidate.MissionID,
		candidate.WorkspaceID, candidate.ProtocolVersion, candidate.ManifestFingerprint,
		candidate.AuthorizationFingerprint, candidate.WorkspaceFingerprint,
		candidate.ScopeFingerprint, candidate.PolicyFingerprint,
		candidate.MountBindingFingerprint, candidate.ApprovalID, candidate.ApprovalStatus,
		candidate.MountCount, candidate.RegularFileMountCount, candidate.DirectoryMountCount,
		candidate.TokensUsed, candidate.ExecutionMillisUsed, candidate.ToolCallsUsed,
		boolInt(candidate.BudgetChecked), boolInt(candidate.LeaseQuiescent),
		boolInt(candidate.BackendEnabled), boolInt(candidate.ExecutionAuthorized),
		candidate.RequestedBy, ts(candidate.ValidatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxExecutionCandidateCreate(ctx, operation, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_candidate_operations
		(operation_key_digest, request_fingerprint, candidate_id, preparation_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.CandidateID, operation.PreparationID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxExecutionCandidateCreate(ctx, operation, err)
	}
	if err := appendSandboxExecutionCandidateEvent(ctx, tx, candidate); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxExecutionCandidateCreate(ctx, operation, err)
	}
	return sandbox.ValidatedExecutionCandidate{Candidate: candidate}, false, nil
}

func validateSandboxExecutionCandidateMutation(candidate sandbox.ExecutionCandidate,
	operation sandbox.CandidateOperation,
) error {
	if err := candidate.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution candidate is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution candidate operation is invalid", err)
	}
	if operation.CandidateID != candidate.ID || operation.PreparationID != candidate.PreparationID ||
		operation.RunID != candidate.RunID || operation.RequestedBy != candidate.RequestedBy ||
		!operation.CreatedAt.Equal(candidate.ValidatedAt) ||
		operation.RequestFingerprint != sandbox.CandidateRequestFingerprint(candidate.PreparationID,
			candidate.ManifestFingerprint, candidate.ApprovalID, candidate.RequestedBy) {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution candidate and operation bindings do not match")
	}
	return nil
}

func validateSandboxCandidateApprovalTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	intent sandbox.PreparedIntent, candidate sandbox.ExecutionCandidate,
) error {
	if !intent.Validation.NeedsApproval {
		if candidate.ApprovalID != "" || candidate.ApprovalStatus != sandbox.ApprovalNotRequired {
			return apperror.New(apperror.CodeConflict,
				"sandbox execution candidate added approval to a no-approval intent")
		}
		return nil
	}
	if candidate.ApprovalID == "" || candidate.ApprovalStatus != sandbox.ApprovalApproved {
		return apperror.New(apperror.CodeFailedPrecondition,
			"sandbox execution candidate requires exact approval")
	}
	record, err := getApprovalTx(ctx, tx, candidate.ApprovalID, "")
	if err != nil {
		return err
	}
	if record.ProposalID != candidate.PreparationID || record.RunID != run.ID ||
		record.SessionID != run.SessionID || record.WorkspaceID != candidate.WorkspaceID ||
		record.ToolName != "sandbox.manifest" || record.ActionClass != "sandbox_execute" ||
		record.Mode != "per_call" || record.Status != "approved" ||
		record.RequestFingerprint != candidate.AuthorizationFingerprint {
		return apperror.New(apperror.CodeConflict,
			"sandbox execution candidate approval binding is invalid")
	}
	return nil
}

func getRunToolCallCountTx(ctx context.Context, tx *sql.Tx, runID string) (int64, error) {
	var consumed int64
	err := tx.QueryRowContext(ctx, `SELECT consumed FROM run_tool_usage WHERE run_id = ?`, runID).
		Scan(&consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return consumed, err
}

func requireSandboxCandidateStoreBudget(budget domain.Budget, usage domain.RunAgentUsage,
	toolCalls int64,
) error {
	if budget.MaxTokens > 0 && usage.TotalTokens >= budget.MaxTokens {
		return apperror.New(apperror.CodeResourceExhausted,
			"sandbox execution candidate has no remaining token budget")
	}
	if budget.TimeoutSeconds > 0 && usage.TotalExecutionMillis >= budget.TimeoutSeconds*1000 {
		return apperror.New(apperror.CodeResourceExhausted,
			"sandbox execution candidate has no remaining execution-time budget")
	}
	if budget.MaxToolCalls > 0 && toolCalls >= budget.MaxToolCalls {
		return apperror.New(apperror.CodeResourceExhausted,
			"sandbox execution candidate has no remaining tool-call budget")
	}
	return nil
}

func appendSandboxExecutionCandidateEvent(ctx context.Context, tx *sql.Tx,
	candidate sandbox.ExecutionCandidate,
) error {
	event, err := events.New(candidate.RunID, candidate.MissionID,
		events.SandboxExecutionCandidateValidatedEvent, "sandbox_execution_candidate",
		candidate.ID, map[string]any{
			"protocol": candidate.ProtocolVersion, "preparation_id": candidate.PreparationID,
			"approval_status": candidate.ApprovalStatus,
			"approval_bound":  candidate.ApprovalID != "", "mount_count": candidate.MountCount,
			"regular_file_mount_count": candidate.RegularFileMountCount,
			"directory_mount_count":    candidate.DirectoryMountCount,
			"tokens_used":              candidate.TokensUsed,
			"execution_millis_used":    candidate.ExecutionMillisUsed,
			"tool_calls_used":          candidate.ToolCallsUsed, "budget_checked": true,
			"lease_quiescent": true, "manifest_content_stored": false,
			"backend_enabled": false, "execution_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = candidate.ValidatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}

func replaySandboxExecutionCandidate(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.CandidateOperation,
) (sandbox.ValidatedExecutionCandidate, bool, error) {
	if err := validateSandboxExecutionCandidateReplay(existing, requested); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	candidate, err := getSandboxExecutionCandidate(ctx, tx, existing.CandidateID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if err := validateStoredSandboxExecutionCandidateBinding(candidate, existing); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	return sandbox.ValidatedExecutionCandidate{Candidate: candidate, Replayed: true}, true, nil
}

func validateSandboxExecutionCandidateReplay(existing, requested sandbox.CandidateOperation) error {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.PreparationID != requested.PreparationID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"sandbox execution candidate operation key was already used for different intent")
	}
	return nil
}

func (s *SQLiteStore) recoverSandboxExecutionCandidateCreate(ctx context.Context,
	operation sandbox.CandidateOperation, original error,
) (sandbox.ValidatedExecutionCandidate, bool, error) {
	existing, found, err := getSandboxExecutionCandidateOperation(ctx, s.db, operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return sandbox.ValidatedExecutionCandidate{}, false, original
		}
		return sandbox.ValidatedExecutionCandidate{}, false, errors.Join(original, err)
	}
	if err := validateSandboxExecutionCandidateReplay(existing, operation); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	candidate, err := getSandboxExecutionCandidate(ctx, s.db, existing.CandidateID)
	if err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	if err := validateStoredSandboxExecutionCandidateBinding(candidate, existing); err != nil {
		return sandbox.ValidatedExecutionCandidate{}, false, err
	}
	return sandbox.ValidatedExecutionCandidate{Candidate: candidate, Replayed: true}, true, nil
}

func validateStoredSandboxExecutionCandidateBinding(candidate sandbox.ExecutionCandidate,
	operation sandbox.CandidateOperation,
) error {
	if candidate.ID != operation.CandidateID || candidate.PreparationID != operation.PreparationID ||
		candidate.RunID != operation.RunID || candidate.RequestedBy != operation.RequestedBy ||
		!candidate.ValidatedAt.Equal(operation.CreatedAt) ||
		operation.RequestFingerprint != sandbox.CandidateRequestFingerprint(candidate.PreparationID,
			candidate.ManifestFingerprint, candidate.ApprovalID, candidate.RequestedBy) {
		return apperror.New(apperror.CodeInternal,
			"stored sandbox execution candidate operation binding is invalid")
	}
	return nil
}

func getSandboxExecutionCandidate(ctx context.Context, queryer sandboxManifestQueryer,
	id string,
) (sandbox.ExecutionCandidate, error) {
	return scanSandboxExecutionCandidate(queryer.QueryRowContext(ctx,
		sandboxExecutionCandidateSelect+` WHERE id = ?`, id))
}

func scanSandboxExecutionCandidate(scanner interface{ Scan(...any) error }) (sandbox.ExecutionCandidate, error) {
	var candidate sandbox.ExecutionCandidate
	var budgetChecked, leaseQuiescent, backendEnabled, executionAuthorized int
	var validatedAt string
	if err := scanner.Scan(&candidate.ID, &candidate.PreparationID, &candidate.RunID,
		&candidate.MissionID, &candidate.WorkspaceID, &candidate.ProtocolVersion,
		&candidate.ManifestFingerprint, &candidate.AuthorizationFingerprint,
		&candidate.WorkspaceFingerprint, &candidate.ScopeFingerprint,
		&candidate.PolicyFingerprint, &candidate.MountBindingFingerprint,
		&candidate.ApprovalID, &candidate.ApprovalStatus, &candidate.MountCount,
		&candidate.RegularFileMountCount, &candidate.DirectoryMountCount,
		&candidate.TokensUsed, &candidate.ExecutionMillisUsed, &candidate.ToolCallsUsed,
		&budgetChecked, &leaseQuiescent, &backendEnabled, &executionAuthorized,
		&candidate.RequestedBy, &validatedAt); err != nil {
		return sandbox.ExecutionCandidate{}, err
	}
	candidate.BudgetChecked = budgetChecked != 0
	candidate.LeaseQuiescent = leaseQuiescent != 0
	candidate.BackendEnabled = backendEnabled != 0
	candidate.ExecutionAuthorized = executionAuthorized != 0
	candidate.ValidatedAt = parseTS(validatedAt)
	if err := candidate.Validate(); err != nil {
		return sandbox.ExecutionCandidate{}, fmt.Errorf("stored sandbox execution candidate is invalid: %w", err)
	}
	return candidate, nil
}

func getSandboxExecutionCandidateOperation(ctx context.Context, queryer sandboxManifestQueryer,
	keyDigest string,
) (sandbox.CandidateOperation, bool, error) {
	var operation sandbox.CandidateOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		candidate_id, preparation_id, run_id, requested_by, created_at
		FROM sandbox_execution_candidate_operations WHERE operation_key_digest = ?`, keyDigest).
		Scan(&operation.KeyDigest, &operation.RequestFingerprint, &operation.CandidateID,
			&operation.PreparationID, &operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.CandidateOperation{}, false, nil
	}
	if err != nil {
		return sandbox.CandidateOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}
