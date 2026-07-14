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
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/sandbox"
)

const sandboxDisabledExecutionSelect = `SELECT id, candidate_id, preparation_id, run_id,
	mission_id, workspace_id, cancellation_id, protocol_version, manifest_fingerprint,
	authorization_fingerprint, policy_fingerprint, mount_binding_fingerprint,
	input_artifact_count, input_artifact_bytes, input_artifact_digest, capture_stdout,
	capture_stderr, output_path_count, max_output_bytes, output_plan_fingerprint,
	initial_lease_id, initial_lease_generation, backend_enabled, execution_authorized,
	backend_started, requested_by, created_at FROM sandbox_disabled_executions`

const sandboxExecutionLeaseSelect = `SELECT execution_id, lease_id, owner_id, generation,
	status, acquired_at, renewed_at, expires_at, released_at
	FROM sandbox_execution_leases WHERE execution_id = ?`

type sandboxLifecycleQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *SQLiteStore) CreateSandboxDisabledExecution(ctx context.Context,
	execution sandbox.DisabledExecution, inputs []sandbox.InputArtifactBinding,
	operation sandbox.ExecutionOperation, ownerID string, ttl time.Duration,
) (sandbox.Lifecycle, bool, error) {
	ownerID = strings.TrimSpace(ownerID)
	if err := validateSandboxExecutionCreation(execution, inputs, operation, ownerID, ttl); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.Lifecycle{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, execution.RunID); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if existing, found, err := getSandboxExecutionOperation(ctx, tx, operation.KeyDigest); err != nil {
		return sandbox.Lifecycle{}, false, err
	} else if found {
		return replaySandboxDisabledExecution(ctx, tx, existing, operation)
	}
	if _, err := getSandboxDisabledExecutionByCandidate(ctx, tx, execution.CandidateID); err == nil {
		return sandbox.Lifecycle{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution candidate already has a disabled execution")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return sandbox.Lifecycle{}, false, err
	}
	candidate, err := getSandboxExecutionCandidate(ctx, tx, execution.CandidateID)
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	intent, err := getSandboxManifestIntent(ctx, tx, execution.PreparationID)
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	run, mission, err := getCoordinatorRunTx(ctx, tx, execution.RunID)
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := validateSandboxExecutionCurrentBinding(candidate, intent, execution, run, mission); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := validateSandboxCandidateApprovalTx(ctx, tx, run, intent, candidate); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	usage, err := getRunAgentUsageTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	toolCalls, err := getRunToolCallCountTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if usage.TotalTokens != candidate.TokensUsed ||
		usage.TotalExecutionMillis != candidate.ExecutionMillisUsed ||
		toolCalls != candidate.ToolCallsUsed {
		return sandbox.Lifecycle{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution candidate usage changed before lifecycle creation")
	}
	if err := requireSandboxCandidateStoreBudget(run.Budget, usage, toolCalls); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if lease, found, err := getRunExecutionLeaseTx(ctx, tx, run.ID); err != nil {
		return sandbox.Lifecycle{}, false, err
	} else if found && lease.ActiveAt(time.Now().UTC()) {
		return sandbox.Lifecycle{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox lifecycle creation requires a quiescent Run")
	}
	if err := verifySandboxInputBindingsTx(ctx, tx, execution, inputs, run.SessionID); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := insertSandboxDisabledExecutionTx(ctx, tx, execution); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxDisabledExecutionCreate(ctx, operation, err)
	}
	for _, input := range inputs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_inputs
			(execution_id, ordinal, artifact_id, sha256, size_bytes, mime, stream, source_id, redacted)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, input.ExecutionID, input.Ordinal,
			input.ArtifactID, input.SHA256, input.SizeBytes, input.MIME, input.Stream,
			input.SourceID, boolInt(input.Redacted)); err != nil {
			return sandbox.Lifecycle{}, false, err
		}
	}
	lease := sandbox.ExecutionLease{
		ExecutionID: execution.ID, LeaseID: execution.InitialLeaseID, OwnerID: ownerID,
		Generation: execution.InitialLeaseGeneration, Status: sandbox.ExecutionLeaseActive,
		AcquiredAt: execution.CreatedAt, RenewedAt: execution.CreatedAt,
		ExpiresAt: execution.CreatedAt.Add(ttl),
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_leases
		(execution_id, lease_id, owner_id, generation, status, acquired_at, renewed_at,
		expires_at, released_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)`, lease.ExecutionID,
		lease.LeaseID, lease.OwnerID, lease.Generation, lease.Status, ts(lease.AcquiredAt),
		ts(lease.RenewedAt), ts(lease.ExpiresAt)); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_operations
		(operation_key_digest, request_fingerprint, execution_id, candidate_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.ExecutionID, operation.CandidateID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxDisabledExecutionCreate(ctx, operation, err)
	}
	if err := appendSandboxExecutionPreparedEvent(ctx, tx, execution, lease); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxDisabledExecutionCreate(ctx, operation, err)
	}
	return sandbox.Lifecycle{Execution: execution, Inputs: append([]sandbox.InputArtifactBinding(nil), inputs...),
		Lease: lease, Status: sandbox.LifecyclePrepared}, false, nil
}

func (s *SQLiteStore) GetSandboxDisabledExecution(ctx context.Context,
	id string,
) (sandbox.Lifecycle, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution id is invalid")
	}
	return getSandboxLifecycle(ctx, s.db, id)
}

func (s *SQLiteStore) ListSandboxDisabledExecutions(ctx context.Context, runID string,
	limit int,
) ([]sandbox.Lifecycle, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_disabled_executions
		WHERE run_id = ? ORDER BY created_at, id LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	values := make([]sandbox.Lifecycle, 0, len(ids))
	for _, id := range ids {
		value, err := getSandboxLifecycle(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (s *SQLiteStore) GetSandboxExecutionOperation(ctx context.Context,
	keyDigest string,
) (sandbox.ExecutionOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.ExecutionOperation{}, false, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution operation digest is invalid")
	}
	return getSandboxExecutionOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) AcquireSandboxExecutionLease(ctx context.Context, executionID,
	ownerID, leaseID string, ttl time.Duration,
) (sandbox.LeaseAcquisition, error) {
	executionID, ownerID, leaseID = strings.TrimSpace(executionID), strings.TrimSpace(ownerID), strings.TrimSpace(leaseID)
	if !domain.ValidAgentID(executionID) || !domain.ValidAgentID(ownerID) ||
		(leaseID != "" && !domain.ValidAgentID(leaseID)) || strings.ContainsRune(executionID+ownerID+leaseID, 0) {
		return sandbox.LeaseAcquisition{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution lease identity is invalid")
	}
	if safe := redact.String(ownerID); safe != ownerID {
		return sandbox.LeaseAcquisition{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution lease owner cannot contain sensitive material")
	}
	if err := sandbox.ValidateExecutionLeaseTTL(ttl); err != nil {
		return sandbox.LeaseAcquisition{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	execution, err := getSandboxDisabledExecution(ctx, tx, executionID)
	if err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, execution.RunID); err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	if _, found, err := getSandboxCleanupResult(ctx, tx, executionID); err != nil {
		return sandbox.LeaseAcquisition{}, err
	} else if found {
		return sandbox.LeaseAcquisition{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox execution cleanup is already complete")
	}
	current, found, err := getSandboxExecutionLease(ctx, tx, executionID)
	if err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	if !found {
		return sandbox.LeaseAcquisition{}, apperror.New(apperror.CodeInternal,
			"sandbox execution is missing its initial lease")
	}
	now := time.Now().UTC()
	if current.ActiveAt(now) {
		if leaseID == "" || current.LeaseID != leaseID || current.OwnerID != ownerID {
			return sandbox.LeaseAcquisition{}, apperror.New(apperror.CodeConflict,
				fmt.Sprintf("sandbox execution is leased through %s", current.ExpiresAt.Format(time.RFC3339Nano)))
		}
		current.RenewedAt, current.ExpiresAt = now, now.Add(ttl)
		result, err := tx.ExecContext(ctx, `UPDATE sandbox_execution_leases
			SET renewed_at = ?, expires_at = ? WHERE execution_id = ? AND lease_id = ?
				AND owner_id = ? AND generation = ? AND status = 'active'`, ts(current.RenewedAt),
			ts(current.ExpiresAt), current.ExecutionID, current.LeaseID, current.OwnerID,
			current.Generation)
		if err != nil {
			return sandbox.LeaseAcquisition{}, err
		}
		if err := requireSingleLeaseUpdate(result, "sandbox execution lease changed before renewal"); err != nil {
			return sandbox.LeaseAcquisition{}, err
		}
		if err := tx.Commit(); err != nil {
			return sandbox.LeaseAcquisition{}, err
		}
		return sandbox.LeaseAcquisition{Lease: current, Replayed: true}, nil
	}
	if leaseID != "" {
		return sandbox.LeaseAcquisition{}, apperror.New(apperror.CodeConflict,
			"sandbox execution lease replay token is no longer active")
	}
	previousGeneration := current.Generation
	tookOver := current.Status == sandbox.ExecutionLeaseActive
	next := sandbox.ExecutionLease{
		ExecutionID: executionID, LeaseID: newSandboxLeaseID(), OwnerID: ownerID,
		Generation: current.Generation + 1, Status: sandbox.ExecutionLeaseActive,
		AcquiredAt: now, RenewedAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := next.Validate(); err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_execution_leases SET lease_id = ?, owner_id = ?,
		generation = ?, status = ?, acquired_at = ?, renewed_at = ?, expires_at = ?, released_at = NULL
		WHERE execution_id = ? AND generation = ?`, next.LeaseID, next.OwnerID, next.Generation,
		next.Status, ts(next.AcquiredAt), ts(next.RenewedAt), ts(next.ExpiresAt), executionID,
		current.Generation)
	if err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	if err := requireSingleLeaseUpdate(result, "sandbox execution lease changed before takeover"); err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	eventType := events.SandboxExecutionLeaseAcquiredEvent
	if tookOver {
		eventType = events.SandboxExecutionLeaseTakenOverEvent
	}
	if err := appendSandboxLeaseEvent(ctx, tx, execution, next, eventType, previousGeneration); err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.LeaseAcquisition{}, err
	}
	return sandbox.LeaseAcquisition{Lease: next, TookOver: tookOver}, nil
}

func (s *SQLiteStore) ReleaseSandboxExecutionLease(ctx context.Context,
	expected sandbox.ExecutionLease,
) (sandbox.ExecutionLease, bool, error) {
	if err := expected.Validate(); err != nil {
		return sandbox.ExecutionLease{}, false, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution lease is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	execution, err := getSandboxDisabledExecution(ctx, tx, expected.ExecutionID)
	if err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, execution.RunID); err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	current, found, err := getSandboxExecutionLease(ctx, tx, expected.ExecutionID)
	if err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	if !found || !sameSandboxExecutionLease(current, expected) {
		return sandbox.ExecutionLease{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution lease was replaced before release")
	}
	if current.Status == sandbox.ExecutionLeaseReleased {
		if err := tx.Commit(); err != nil {
			return sandbox.ExecutionLease{}, false, err
		}
		return current, true, nil
	}
	now := time.Now().UTC()
	current.Status, current.ReleasedAt = sandbox.ExecutionLeaseReleased, &now
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_execution_leases SET status = 'released', released_at = ?
		WHERE execution_id = ? AND lease_id = ? AND owner_id = ? AND generation = ? AND status = 'active'`,
		ts(now), current.ExecutionID, current.LeaseID, current.OwnerID, current.Generation)
	if err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	if err := requireSingleLeaseUpdate(result, "sandbox execution lease changed before release"); err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	if err := appendSandboxLeaseEvent(ctx, tx, execution, current,
		events.SandboxExecutionLeaseReleasedEvent, current.Generation); err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	return current, false, nil
}

func (s *SQLiteStore) GetSandboxExecutionLease(ctx context.Context,
	executionID string,
) (sandbox.ExecutionLease, bool, error) {
	executionID = strings.TrimSpace(executionID)
	if !domain.ValidAgentID(executionID) || strings.ContainsRune(executionID, 0) {
		return sandbox.ExecutionLease{}, false, apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution lease execution id is invalid")
	}
	return getSandboxExecutionLease(ctx, s.db, executionID)
}

func (s *SQLiteStore) CreateSandboxCancellation(ctx context.Context,
	request sandbox.CancellationRequest, operation sandbox.CancellationOperation,
) (sandbox.CancellationRequest, bool, error) {
	if err := validateSandboxCancellationMutation(request, operation); err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, request.RunID); err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	if existing, found, err := getSandboxCancellationOperation(ctx, tx, operation.KeyDigest); err != nil {
		return sandbox.CancellationRequest{}, false, err
	} else if found {
		return replaySandboxCancellation(ctx, tx, existing, operation)
	}
	execution, err := getSandboxDisabledExecution(ctx, tx, request.ExecutionID)
	if err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	if execution.RunID != request.RunID || execution.CancellationID != request.CancellationID {
		return sandbox.CancellationRequest{}, false, apperror.New(apperror.CodeConflict,
			"sandbox cancellation does not match its execution")
	}
	if _, found, err := getSandboxCleanupResult(ctx, tx, execution.ID); err != nil {
		return sandbox.CancellationRequest{}, false, err
	} else if found {
		return sandbox.CancellationRequest{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox execution cleanup is already complete")
	}
	if _, found, err := getSandboxCancellation(ctx, tx, execution.ID); err != nil {
		return sandbox.CancellationRequest{}, false, err
	} else if found {
		return sandbox.CancellationRequest{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution already has a cancellation request")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_cancellations
		(id, execution_id, run_id, cancellation_id, protocol_version, requested_by, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, request.ID, request.ExecutionID, request.RunID,
		request.CancellationID, request.ProtocolVersion, request.RequestedBy, ts(request.RequestedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxCancellationCreate(ctx, operation, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_cancellation_operations
		(operation_key_digest, request_fingerprint, request_id, execution_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.RequestID, operation.ExecutionID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxCancellationCreate(ctx, operation, err)
	}
	if err := appendSandboxCancellationEvent(ctx, tx, execution, request); err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxCancellationCreate(ctx, operation, err)
	}
	return request, false, nil
}

func (s *SQLiteStore) GetSandboxCancellationOperation(ctx context.Context,
	keyDigest string,
) (sandbox.CancellationOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.CancellationOperation{}, false, apperror.New(apperror.CodeInvalidArgument,
			"sandbox cancellation operation digest is invalid")
	}
	return getSandboxCancellationOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CompleteSandboxCleanup(ctx context.Context, result sandbox.CleanupResult,
	operation sandbox.CleanupOperation, expectedLease sandbox.ExecutionLease,
) (sandbox.CleanupResult, bool, error) {
	if err := validateSandboxCleanupMutation(result, operation, expectedLease); err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, result.RunID); err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if existing, found, err := getSandboxCleanupOperation(ctx, tx, operation.KeyDigest); err != nil {
		return sandbox.CleanupResult{}, false, err
	} else if found {
		return replaySandboxCleanup(ctx, tx, existing, operation)
	}
	execution, err := getSandboxDisabledExecution(ctx, tx, result.ExecutionID)
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if execution.RunID != result.RunID {
		return sandbox.CleanupResult{}, false, apperror.New(apperror.CodeConflict,
			"sandbox cleanup does not match its execution Run")
	}
	currentLease, found, err := getSandboxExecutionLease(ctx, tx, execution.ID)
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if !found || !sameSandboxExecutionLease(currentLease, expectedLease) ||
		currentLease.LeaseID != result.LeaseID || currentLease.Generation != result.LeaseGeneration ||
		!currentLease.ActiveAt(time.Now().UTC()) {
		return sandbox.CleanupResult{}, false, apperror.New(apperror.CodeConflict,
			"sandbox cleanup lease fencing token is stale, released, or expired")
	}
	inputs, err := listSandboxExecutionInputs(ctx, tx, execution.ID)
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	run, _, err := getCoordinatorRunTx(ctx, tx, execution.RunID)
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if err := verifySandboxInputBindingsTx(ctx, tx, execution, inputs, run.SessionID); err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	_, cancellationFound, err := getSandboxCancellation(ctx, tx, execution.ID)
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if result.CancellationObserved != cancellationFound {
		return sandbox.CleanupResult{}, false, apperror.New(apperror.CodeConflict,
			"sandbox cleanup cancellation observation is stale")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_cleanup_results
		(id, execution_id, run_id, protocol_version, lease_id, lease_generation,
		cancellation_observed, backend_started, orphan_detected, orphan_reaped,
		input_artifacts_verified, output_artifact_count, cleanup_complete, outcome,
		reconciled_by, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.ID, result.ExecutionID, result.RunID, result.ProtocolVersion, result.LeaseID,
		result.LeaseGeneration, boolInt(result.CancellationObserved), boolInt(result.BackendStarted),
		boolInt(result.OrphanDetected), boolInt(result.OrphanReaped), boolInt(result.InputArtifactsVerified),
		result.OutputArtifactCount, boolInt(result.CleanupComplete), result.Outcome,
		result.ReconciledBy, ts(result.CompletedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxCleanupCreate(ctx, operation, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_cleanup_operations
		(operation_key_digest, request_fingerprint, cleanup_id, execution_id, run_id,
		reconciled_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.CleanupID, operation.ExecutionID,
		operation.RunID, operation.ReconciledBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxCleanupCreate(ctx, operation, err)
	}
	if err := appendSandboxCleanupEvent(ctx, tx, execution, result); err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxCleanupCreate(ctx, operation, err)
	}
	return result, false, nil
}

func (s *SQLiteStore) GetSandboxCleanupOperation(ctx context.Context,
	keyDigest string,
) (sandbox.CleanupOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.CleanupOperation{}, false, apperror.New(apperror.CodeInvalidArgument,
			"sandbox cleanup operation digest is invalid")
	}
	return getSandboxCleanupOperation(ctx, s.db, keyDigest)
}

func validateSandboxExecutionCreation(execution sandbox.DisabledExecution,
	inputs []sandbox.InputArtifactBinding, operation sandbox.ExecutionOperation,
	ownerID string, ttl time.Duration,
) error {
	if err := execution.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "sandbox execution is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox execution operation is invalid", err)
	}
	if !domain.ValidAgentID(ownerID) || strings.ContainsRune(ownerID, 0) || redact.String(ownerID) != ownerID {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution lease owner is invalid or contains sensitive material")
	}
	if err := sandbox.ValidateExecutionLeaseTTL(ttl); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if len(inputs) != execution.InputArtifactCount || len(inputs) > sandbox.MaxInputArtifacts {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution input Artifact count does not match")
	}
	var total int64
	for index, input := range inputs {
		if err := input.Validate(); err != nil {
			return apperror.Wrap(apperror.CodeInvalidArgument,
				"sandbox execution input Artifact is invalid", err)
		}
		if input.ExecutionID != execution.ID || input.Ordinal != index+1 {
			return apperror.New(apperror.CodeInvalidArgument,
				"sandbox execution input Artifact order or execution binding is invalid")
		}
		total += input.SizeBytes
	}
	if total != execution.InputArtifactBytes ||
		sandbox.InputArtifactBindingsDigest(inputs) != execution.InputArtifactDigest {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution input Artifact totals or digest do not match")
	}
	if operation.ExecutionID != execution.ID || operation.CandidateID != execution.CandidateID ||
		operation.RunID != execution.RunID || operation.RequestedBy != execution.RequestedBy ||
		!operation.CreatedAt.Equal(execution.CreatedAt) ||
		operation.RequestFingerprint != sandbox.ExecutionRequestFingerprint(execution) {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox execution and operation bindings do not match")
	}
	return nil
}

func validateSandboxExecutionCurrentBinding(candidate sandbox.ExecutionCandidate,
	intent sandbox.PreparedIntent, execution sandbox.DisabledExecution, run domain.Run,
	mission domain.Mission,
) error {
	if run.Terminal() || candidate.ID != execution.CandidateID ||
		candidate.PreparationID != execution.PreparationID || candidate.RunID != execution.RunID ||
		candidate.MissionID != execution.MissionID || candidate.WorkspaceID != execution.WorkspaceID ||
		candidate.ManifestFingerprint != execution.ManifestFingerprint ||
		candidate.AuthorizationFingerprint != execution.AuthorizationFingerprint ||
		candidate.PolicyFingerprint != execution.PolicyFingerprint ||
		candidate.MountBindingFingerprint != execution.MountBindingFingerprint ||
		candidate.RequestedBy != execution.RequestedBy || candidate.BackendEnabled ||
		candidate.ExecutionAuthorized || intent.Preparation.CancellationID != execution.CancellationID ||
		intent.Preparation.InputArtifactCount != execution.InputArtifactCount ||
		intent.Preparation.OutputCount != boolInt(execution.OutputPlan.CaptureStdout)+
			boolInt(execution.OutputPlan.CaptureStderr)+execution.OutputPlan.OutputPathCount ||
		intent.Preparation.MaxOutputBytes != execution.OutputPlan.MaxOutputBytes ||
		intent.Preparation.RunID != execution.RunID || intent.Preparation.MissionID != execution.MissionID ||
		intent.Preparation.WorkspaceID != execution.WorkspaceID || run.MissionID != execution.MissionID ||
		mission.ID != execution.MissionID || mission.WorkspaceID != execution.WorkspaceID {
		return apperror.New(apperror.CodeConflict,
			"sandbox execution does not match current candidate, preparation, and Run bindings")
	}
	return nil
}

func verifySandboxInputBindingsTx(ctx context.Context, tx *sql.Tx,
	execution sandbox.DisabledExecution, inputs []sandbox.InputArtifactBinding, sessionID string,
) error {
	if len(inputs) != execution.InputArtifactCount {
		return apperror.New(apperror.CodeConflict,
			"sandbox execution input Artifact count changed")
	}
	var total int64
	for index, input := range inputs {
		if input.ExecutionID != execution.ID || input.Ordinal != index+1 {
			return apperror.New(apperror.CodeConflict,
				"sandbox execution input Artifact order changed")
		}
		blob, err := getRunArtifactRow(tx.QueryRowContext(ctx, runArtifactSelect+` WHERE id = ?`, input.ArtifactID))
		if err != nil {
			return apperror.Wrap(apperror.CodeConflict,
				"sandbox execution input Artifact is missing or corrupt", err)
		}
		if blob.RunID != execution.RunID || blob.SessionID != sessionID ||
			blob.WorkspaceID != execution.WorkspaceID || blob.SHA256 != input.SHA256 ||
			blob.SizeBytes != input.SizeBytes || blob.MIME != input.MIME ||
			string(blob.Stream) != input.Stream || blob.SourceID != input.SourceID ||
			blob.Redacted != input.Redacted {
			return apperror.New(apperror.CodeConflict,
				"sandbox execution input Artifact metadata or scope changed")
		}
		total += input.SizeBytes
	}
	if total != execution.InputArtifactBytes ||
		sandbox.InputArtifactBindingsDigest(inputs) != execution.InputArtifactDigest {
		return apperror.New(apperror.CodeConflict,
			"sandbox execution input Artifact digest changed")
	}
	return nil
}

func insertSandboxDisabledExecutionTx(ctx context.Context, tx *sql.Tx,
	execution sandbox.DisabledExecution,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_disabled_executions
		(id, candidate_id, preparation_id, run_id, mission_id, workspace_id, cancellation_id,
		protocol_version, manifest_fingerprint, authorization_fingerprint, policy_fingerprint,
		mount_binding_fingerprint, input_artifact_count, input_artifact_bytes,
		input_artifact_digest, capture_stdout, capture_stderr, output_path_count,
		max_output_bytes, output_plan_fingerprint, initial_lease_id, initial_lease_generation,
		backend_enabled, execution_authorized, backend_started, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		execution.ID, execution.CandidateID, execution.PreparationID, execution.RunID,
		execution.MissionID, execution.WorkspaceID, execution.CancellationID,
		execution.ProtocolVersion, execution.ManifestFingerprint, execution.AuthorizationFingerprint,
		execution.PolicyFingerprint, execution.MountBindingFingerprint,
		execution.InputArtifactCount, execution.InputArtifactBytes, execution.InputArtifactDigest,
		boolInt(execution.OutputPlan.CaptureStdout), boolInt(execution.OutputPlan.CaptureStderr),
		execution.OutputPlan.OutputPathCount, execution.OutputPlan.MaxOutputBytes,
		execution.OutputPlan.Fingerprint, execution.InitialLeaseID,
		execution.InitialLeaseGeneration, boolInt(execution.BackendEnabled),
		boolInt(execution.ExecutionAuthorized), boolInt(execution.BackendStarted),
		execution.RequestedBy, ts(execution.CreatedAt))
	return err
}

func getSandboxLifecycle(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.Lifecycle, error) {
	execution, err := getSandboxDisabledExecution(ctx, queryer, id)
	if err != nil {
		return sandbox.Lifecycle{}, err
	}
	inputs, err := listSandboxExecutionInputs(ctx, queryer, id)
	if err != nil {
		return sandbox.Lifecycle{}, err
	}
	lease, found, err := getSandboxExecutionLease(ctx, queryer, id)
	if err != nil {
		return sandbox.Lifecycle{}, err
	}
	if !found {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeInternal,
			"stored sandbox execution is missing its lease")
	}
	cancellation, cancellationFound, err := getSandboxCancellation(ctx, queryer, id)
	if err != nil {
		return sandbox.Lifecycle{}, err
	}
	cleanup, cleanupFound, err := getSandboxCleanupResult(ctx, queryer, id)
	if err != nil {
		return sandbox.Lifecycle{}, err
	}
	lifecycle := sandbox.Lifecycle{Execution: execution, Inputs: inputs, Lease: lease,
		Status: sandbox.LifecyclePrepared}
	if cancellationFound {
		lifecycle.Cancellation, lifecycle.Status = &cancellation, sandbox.LifecycleCancelPending
	}
	if cleanupFound {
		lifecycle.Cleanup, lifecycle.Status = &cleanup, sandbox.LifecycleCleanupComplete
	}
	if len(inputs) != execution.InputArtifactCount ||
		sandbox.InputArtifactBindingsDigest(inputs) != execution.InputArtifactDigest {
		return sandbox.Lifecycle{}, apperror.New(apperror.CodeInternal,
			"stored sandbox execution input binding is invalid")
	}
	return lifecycle, nil
}

func getSandboxDisabledExecution(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DisabledExecution, error) {
	return scanSandboxDisabledExecution(queryer.QueryRowContext(ctx,
		sandboxDisabledExecutionSelect+` WHERE id = ?`, id))
}

func getSandboxDisabledExecutionByCandidate(ctx context.Context,
	queryer sandboxLifecycleQueryer, candidateID string,
) (sandbox.DisabledExecution, error) {
	return scanSandboxDisabledExecution(queryer.QueryRowContext(ctx,
		sandboxDisabledExecutionSelect+` WHERE candidate_id = ?`, candidateID))
}

func scanSandboxDisabledExecution(row scanner) (sandbox.DisabledExecution, error) {
	var execution sandbox.DisabledExecution
	var captureStdout, captureStderr, backendEnabled, executionAuthorized, backendStarted int
	var createdAt string
	if err := row.Scan(&execution.ID, &execution.CandidateID, &execution.PreparationID,
		&execution.RunID, &execution.MissionID, &execution.WorkspaceID,
		&execution.CancellationID, &execution.ProtocolVersion, &execution.ManifestFingerprint,
		&execution.AuthorizationFingerprint, &execution.PolicyFingerprint,
		&execution.MountBindingFingerprint, &execution.InputArtifactCount,
		&execution.InputArtifactBytes, &execution.InputArtifactDigest, &captureStdout,
		&captureStderr, &execution.OutputPlan.OutputPathCount,
		&execution.OutputPlan.MaxOutputBytes, &execution.OutputPlan.Fingerprint,
		&execution.InitialLeaseID, &execution.InitialLeaseGeneration, &backendEnabled,
		&executionAuthorized, &backendStarted, &execution.RequestedBy, &createdAt); err != nil {
		return sandbox.DisabledExecution{}, err
	}
	execution.OutputPlan.CaptureStdout = captureStdout != 0
	execution.OutputPlan.CaptureStderr = captureStderr != 0
	execution.BackendEnabled = backendEnabled != 0
	execution.ExecutionAuthorized = executionAuthorized != 0
	execution.BackendStarted = backendStarted != 0
	execution.CreatedAt = parseTS(createdAt)
	if err := execution.Validate(); err != nil {
		return sandbox.DisabledExecution{}, fmt.Errorf("stored sandbox execution is invalid: %w", err)
	}
	return execution, nil
}

func listSandboxExecutionInputs(ctx context.Context, queryer sandboxLifecycleQueryer,
	executionID string,
) ([]sandbox.InputArtifactBinding, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT execution_id, ordinal, artifact_id, sha256,
		size_bytes, mime, stream, source_id, redacted FROM sandbox_execution_inputs
		WHERE execution_id = ? ORDER BY ordinal`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []sandbox.InputArtifactBinding
	for rows.Next() {
		var value sandbox.InputArtifactBinding
		var redacted int
		if err := rows.Scan(&value.ExecutionID, &value.Ordinal, &value.ArtifactID,
			&value.SHA256, &value.SizeBytes, &value.MIME, &value.Stream, &value.SourceID,
			&redacted); err != nil {
			return nil, err
		}
		value.Redacted = redacted != 0
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("stored sandbox input Artifact binding is invalid: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func getSandboxExecutionLease(ctx context.Context, queryer sandboxLifecycleQueryer,
	executionID string,
) (sandbox.ExecutionLease, bool, error) {
	var lease sandbox.ExecutionLease
	var status, acquiredAt, renewedAt, expiresAt string
	var releasedAt sql.NullString
	err := queryer.QueryRowContext(ctx, sandboxExecutionLeaseSelect, executionID).Scan(
		&lease.ExecutionID, &lease.LeaseID, &lease.OwnerID, &lease.Generation, &status,
		&acquiredAt, &renewedAt, &expiresAt, &releasedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.ExecutionLease{}, false, nil
	}
	if err != nil {
		return sandbox.ExecutionLease{}, false, err
	}
	lease.Status = sandbox.ExecutionLeaseStatus(status)
	lease.AcquiredAt, lease.RenewedAt, lease.ExpiresAt = parseTS(acquiredAt), parseTS(renewedAt), parseTS(expiresAt)
	if releasedAt.Valid {
		value := parseTS(releasedAt.String)
		lease.ReleasedAt = &value
	}
	if err := lease.Validate(); err != nil {
		return sandbox.ExecutionLease{}, false, fmt.Errorf("stored sandbox execution lease is invalid: %w", err)
	}
	return lease, true, nil
}

func getSandboxExecutionOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.ExecutionOperation, bool, error) {
	var operation sandbox.ExecutionOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		execution_id, candidate_id, run_id, requested_by, created_at
		FROM sandbox_execution_operations WHERE operation_key_digest = ?`, keyDigest).Scan(
		&operation.KeyDigest, &operation.RequestFingerprint, &operation.ExecutionID,
		&operation.CandidateID, &operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.ExecutionOperation{}, false, nil
	}
	if err != nil {
		return sandbox.ExecutionOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}

func getSandboxCancellation(ctx context.Context, queryer sandboxLifecycleQueryer,
	executionID string,
) (sandbox.CancellationRequest, bool, error) {
	var request sandbox.CancellationRequest
	var requestedAt string
	err := queryer.QueryRowContext(ctx, `SELECT id, execution_id, run_id, cancellation_id,
		protocol_version, requested_by, requested_at FROM sandbox_execution_cancellations
		WHERE execution_id = ?`, executionID).Scan(&request.ID, &request.ExecutionID,
		&request.RunID, &request.CancellationID, &request.ProtocolVersion,
		&request.RequestedBy, &requestedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.CancellationRequest{}, false, nil
	}
	if err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	request.RequestedAt = parseTS(requestedAt)
	return request, true, request.Validate()
}

func getSandboxCancellationOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.CancellationOperation, bool, error) {
	var operation sandbox.CancellationOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		request_id, execution_id, run_id, requested_by, created_at
		FROM sandbox_execution_cancellation_operations WHERE operation_key_digest = ?`, keyDigest).Scan(
		&operation.KeyDigest, &operation.RequestFingerprint, &operation.RequestID,
		&operation.ExecutionID, &operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.CancellationOperation{}, false, nil
	}
	if err != nil {
		return sandbox.CancellationOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}

func getSandboxCleanupResult(ctx context.Context, queryer sandboxLifecycleQueryer,
	executionID string,
) (sandbox.CleanupResult, bool, error) {
	var result sandbox.CleanupResult
	var cancellationObserved, backendStarted, orphanDetected, orphanReaped int
	var inputsVerified, cleanupComplete int
	var completedAt string
	err := queryer.QueryRowContext(ctx, `SELECT id, execution_id, run_id, protocol_version,
		lease_id, lease_generation, cancellation_observed, backend_started, orphan_detected,
		orphan_reaped, input_artifacts_verified, output_artifact_count, cleanup_complete,
		outcome, reconciled_by, completed_at FROM sandbox_cleanup_results WHERE execution_id = ?`,
		executionID).Scan(&result.ID, &result.ExecutionID, &result.RunID,
		&result.ProtocolVersion, &result.LeaseID, &result.LeaseGeneration,
		&cancellationObserved, &backendStarted, &orphanDetected, &orphanReaped,
		&inputsVerified, &result.OutputArtifactCount, &cleanupComplete, &result.Outcome,
		&result.ReconciledBy, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.CleanupResult{}, false, nil
	}
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	result.CancellationObserved = cancellationObserved != 0
	result.BackendStarted = backendStarted != 0
	result.OrphanDetected = orphanDetected != 0
	result.OrphanReaped = orphanReaped != 0
	result.InputArtifactsVerified = inputsVerified != 0
	result.CleanupComplete = cleanupComplete != 0
	result.CompletedAt = parseTS(completedAt)
	return result, true, result.Validate()
}

func getSandboxCleanupOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.CleanupOperation, bool, error) {
	var operation sandbox.CleanupOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		cleanup_id, execution_id, run_id, reconciled_by, created_at
		FROM sandbox_cleanup_operations WHERE operation_key_digest = ?`, keyDigest).Scan(
		&operation.KeyDigest, &operation.RequestFingerprint, &operation.CleanupID,
		&operation.ExecutionID, &operation.RunID, &operation.ReconciledBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.CleanupOperation{}, false, nil
	}
	if err != nil {
		return sandbox.CleanupOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}

func validateSandboxCancellationMutation(request sandbox.CancellationRequest,
	operation sandbox.CancellationOperation,
) error {
	if err := request.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "sandbox cancellation is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox cancellation operation is invalid", err)
	}
	if operation.RequestID != request.ID || operation.ExecutionID != request.ExecutionID ||
		operation.RunID != request.RunID || operation.RequestedBy != request.RequestedBy ||
		!operation.CreatedAt.Equal(request.RequestedAt) ||
		operation.RequestFingerprint != sandbox.CancellationRequestFingerprint(request.ExecutionID,
			request.CancellationID, request.RequestedBy) {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox cancellation and operation bindings do not match")
	}
	return nil
}

func validateSandboxCleanupMutation(result sandbox.CleanupResult,
	operation sandbox.CleanupOperation, lease sandbox.ExecutionLease,
) error {
	if err := result.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "sandbox cleanup result is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox cleanup operation is invalid", err)
	}
	if err := lease.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "sandbox cleanup lease is invalid", err)
	}
	if result.ExecutionID != lease.ExecutionID || result.LeaseID != lease.LeaseID ||
		result.LeaseGeneration != lease.Generation || operation.CleanupID != result.ID ||
		operation.ExecutionID != result.ExecutionID || operation.RunID != result.RunID ||
		operation.ReconciledBy != result.ReconciledBy ||
		!operation.CreatedAt.Equal(result.CompletedAt) ||
		operation.RequestFingerprint != sandbox.CleanupRequestFingerprint(result.ExecutionID,
			result.CancellationObserved, result.ReconciledBy) {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox cleanup, operation, and lease bindings do not match")
	}
	return nil
}

func replaySandboxDisabledExecution(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.ExecutionOperation,
) (sandbox.Lifecycle, bool, error) {
	if err := validateSandboxExecutionReplay(existing, requested); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	lifecycle, err := getSandboxLifecycle(ctx, tx, existing.ExecutionID)
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := validateStoredSandboxExecutionOperation(lifecycle.Execution, existing); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	lifecycle.Replayed = true
	return lifecycle, true, nil
}

func validateSandboxExecutionReplay(existing, requested sandbox.ExecutionOperation) error {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.CandidateID != requested.CandidateID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"sandbox execution operation key was already used for different intent")
	}
	return nil
}

func validateStoredSandboxExecutionOperation(execution sandbox.DisabledExecution,
	operation sandbox.ExecutionOperation,
) error {
	if execution.ID != operation.ExecutionID || execution.CandidateID != operation.CandidateID ||
		execution.RunID != operation.RunID || execution.RequestedBy != operation.RequestedBy ||
		!execution.CreatedAt.Equal(operation.CreatedAt) ||
		operation.RequestFingerprint != sandbox.ExecutionRequestFingerprint(execution) {
		return apperror.New(apperror.CodeInternal,
			"stored sandbox execution operation binding is invalid")
	}
	return nil
}

func (s *SQLiteStore) recoverSandboxDisabledExecutionCreate(ctx context.Context,
	operation sandbox.ExecutionOperation, original error,
) (sandbox.Lifecycle, bool, error) {
	existing, found, err := getSandboxExecutionOperation(ctx, s.db, operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return sandbox.Lifecycle{}, false, original
		}
		return sandbox.Lifecycle{}, false, errors.Join(original, err)
	}
	if err := validateSandboxExecutionReplay(existing, operation); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	lifecycle, err := getSandboxLifecycle(ctx, s.db, existing.ExecutionID)
	if err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	if err := validateStoredSandboxExecutionOperation(lifecycle.Execution, existing); err != nil {
		return sandbox.Lifecycle{}, false, err
	}
	lifecycle.Replayed = true
	return lifecycle, true, nil
}

func replaySandboxCancellation(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.CancellationOperation,
) (sandbox.CancellationRequest, bool, error) {
	if err := validateSandboxCancellationReplay(existing, requested); err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	request, found, err := getSandboxCancellation(ctx, tx, existing.ExecutionID)
	if err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	if !found || request.ID != existing.RequestID || !request.RequestedAt.Equal(existing.CreatedAt) {
		return sandbox.CancellationRequest{}, false, apperror.New(apperror.CodeInternal,
			"stored sandbox cancellation operation binding is invalid")
	}
	if err := tx.Commit(); err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	return request, true, nil
}

func validateSandboxCancellationReplay(existing, requested sandbox.CancellationOperation) error {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.ExecutionID != requested.ExecutionID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"sandbox cancellation operation key was already used for different intent")
	}
	return nil
}

func (s *SQLiteStore) recoverSandboxCancellationCreate(ctx context.Context,
	operation sandbox.CancellationOperation, original error,
) (sandbox.CancellationRequest, bool, error) {
	existing, found, err := getSandboxCancellationOperation(ctx, s.db, operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return sandbox.CancellationRequest{}, false, original
		}
		return sandbox.CancellationRequest{}, false, errors.Join(original, err)
	}
	if err := validateSandboxCancellationReplay(existing, operation); err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	request, found, err := getSandboxCancellation(ctx, s.db, existing.ExecutionID)
	if err != nil {
		return sandbox.CancellationRequest{}, false, err
	}
	if !found || request.ID != existing.RequestID ||
		!request.RequestedAt.Equal(existing.CreatedAt) {
		return sandbox.CancellationRequest{}, false, apperror.New(apperror.CodeInternal,
			"stored sandbox cancellation recovery binding is invalid")
	}
	return request, true, nil
}

func replaySandboxCleanup(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.CleanupOperation,
) (sandbox.CleanupResult, bool, error) {
	if err := validateSandboxCleanupReplay(existing, requested); err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	result, found, err := getSandboxCleanupResult(ctx, tx, existing.ExecutionID)
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if !found || result.ID != existing.CleanupID || !result.CompletedAt.Equal(existing.CreatedAt) {
		return sandbox.CleanupResult{}, false, apperror.New(apperror.CodeInternal,
			"stored sandbox cleanup operation binding is invalid")
	}
	if err := tx.Commit(); err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	return result, true, nil
}

func validateSandboxCleanupReplay(existing, requested sandbox.CleanupOperation) error {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.ExecutionID != requested.ExecutionID || existing.RunID != requested.RunID ||
		existing.ReconciledBy != requested.ReconciledBy {
		return apperror.New(apperror.CodeConflict,
			"sandbox cleanup operation key was already used for different intent")
	}
	return nil
}

func (s *SQLiteStore) recoverSandboxCleanupCreate(ctx context.Context,
	operation sandbox.CleanupOperation, original error,
) (sandbox.CleanupResult, bool, error) {
	existing, found, err := getSandboxCleanupOperation(ctx, s.db, operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return sandbox.CleanupResult{}, false, original
		}
		return sandbox.CleanupResult{}, false, errors.Join(original, err)
	}
	if err := validateSandboxCleanupReplay(existing, operation); err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	result, found, err := getSandboxCleanupResult(ctx, s.db, existing.ExecutionID)
	if err != nil {
		return sandbox.CleanupResult{}, false, err
	}
	if !found || result.ID != existing.CleanupID || !result.CompletedAt.Equal(existing.CreatedAt) {
		return sandbox.CleanupResult{}, false, apperror.New(apperror.CodeInternal,
			"stored sandbox cleanup recovery binding is invalid")
	}
	return result, true, nil
}

func sameSandboxExecutionLease(left, right sandbox.ExecutionLease) bool {
	return left.ExecutionID == right.ExecutionID && left.LeaseID == right.LeaseID &&
		left.OwnerID == right.OwnerID && left.Generation == right.Generation
}

func newSandboxLeaseID() string {
	return idgen.New("sandbox-lease")
}

func appendSandboxExecutionPreparedEvent(ctx context.Context, tx *sql.Tx,
	execution sandbox.DisabledExecution, lease sandbox.ExecutionLease,
) error {
	event, err := events.New(execution.RunID, execution.MissionID,
		events.SandboxExecutionPreparedEvent, "sandbox_lifecycle", execution.ID, map[string]any{
			"protocol": execution.ProtocolVersion, "candidate_id": execution.CandidateID,
			"input_artifact_count": execution.InputArtifactCount,
			"input_artifact_bytes": execution.InputArtifactBytes,
			"capture_stdout":       execution.OutputPlan.CaptureStdout,
			"capture_stderr":       execution.OutputPlan.CaptureStderr,
			"output_path_count":    execution.OutputPlan.OutputPathCount,
			"max_output_bytes":     execution.OutputPlan.MaxOutputBytes,
			"lease_generation":     lease.Generation, "backend_enabled": false,
			"execution_authorized": false, "backend_started": false,
			"manifest_content_stored": false, "artifact_content_stored": false,
			"output_paths_stored": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = execution.CreatedAt
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return err
	}
	leaseEvent, err := events.New(execution.RunID, execution.MissionID,
		events.SandboxExecutionLeaseAcquiredEvent, "sandbox_lifecycle", execution.ID,
		map[string]any{"generation": lease.Generation, "expires_at": lease.ExpiresAt,
			"initial": true})
	if err != nil {
		return err
	}
	leaseEvent.CreatedAt = execution.CreatedAt
	_, err = insertRunEventTx(ctx, tx, leaseEvent)
	return err
}

func appendSandboxLeaseEvent(ctx context.Context, tx *sql.Tx,
	execution sandbox.DisabledExecution, lease sandbox.ExecutionLease, eventType string,
	previousGeneration int64,
) error {
	payload := map[string]any{"generation": lease.Generation,
		"previous_generation": previousGeneration, "status": lease.Status}
	if eventType != events.SandboxExecutionLeaseReleasedEvent {
		payload["expires_at"] = lease.ExpiresAt
	}
	event, err := events.New(execution.RunID, execution.MissionID, eventType,
		"sandbox_lifecycle", execution.ID, payload)
	if err != nil {
		return err
	}
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}

func appendSandboxCancellationEvent(ctx context.Context, tx *sql.Tx,
	execution sandbox.DisabledExecution, request sandbox.CancellationRequest,
) error {
	event, err := events.New(execution.RunID, execution.MissionID,
		events.SandboxExecutionCancelRequestedEvent, "sandbox_lifecycle", execution.ID,
		map[string]any{"protocol": request.ProtocolVersion, "backend_started": false})
	if err != nil {
		return err
	}
	event.CreatedAt = request.RequestedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}

func appendSandboxCleanupEvent(ctx context.Context, tx *sql.Tx,
	execution sandbox.DisabledExecution, result sandbox.CleanupResult,
) error {
	event, err := events.New(execution.RunID, execution.MissionID,
		events.SandboxExecutionCleanupCompletedEvent, "sandbox_lifecycle", execution.ID,
		map[string]any{"protocol": result.ProtocolVersion,
			"lease_generation":      result.LeaseGeneration,
			"cancellation_observed": result.CancellationObserved,
			"backend_started":       false, "orphan_detected": false, "orphan_reaped": false,
			"input_artifacts_verified": true, "output_artifact_count": 0,
			"cleanup_complete": true, "outcome": result.Outcome})
	if err != nil {
		return err
	}
	event.CreatedAt = result.CompletedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
