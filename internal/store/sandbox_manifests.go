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
	"cyberagent-workbench/internal/sandbox"
)

const sandboxManifestIntentSelect = `SELECT
	preparation.id, preparation.run_id, preparation.mission_id, preparation.workspace_id,
	preparation.cancellation_id, preparation.protocol_version, preparation.backend,
	preparation.manifest_fingerprint, preparation.authorization_fingerprint,
	preparation.workspace_fingerprint, preparation.scope_fingerprint,
	preparation.command_argument_count, preparation.mount_count, preparation.writable_mount_count,
	preparation.environment_count,
	preparation.secret_reference_count, preparation.network_mode, preparation.allowed_target_count,
	preparation.input_artifact_count, preparation.output_count, preparation.timeout_seconds,
	preparation.grace_period_millis, preparation.cpu_quota_millis, preparation.memory_bytes,
	preparation.pids, preparation.max_output_bytes, preparation.requested_by, preparation.prepared_at,
	validation.preparation_id, validation.run_id, validation.protocol_version,
	validation.policy_allowed, validation.needs_approval, validation.risk,
	validation.policy_fingerprint, validation.approval_id, validation.approval_status,
	validation.validator_name, validation.backend_enabled, validation.execution_authorized,
	validation.validated_at
	FROM sandbox_manifest_preparations preparation
	JOIN sandbox_manifest_validations validation ON validation.preparation_id = preparation.id`

type sandboxManifestQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *SQLiteStore) GetSandboxWorkspace(ctx context.Context,
	id string,
) (sandbox.WorkspaceBinding, error) {
	record, err := s.GetWorkspaceByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return sandbox.WorkspaceBinding{}, err
	}
	return sandbox.WorkspaceBinding{ID: record.ID, RootPath: record.RootPath}, nil
}

func (s *SQLiteStore) GetSandboxManifestIntent(ctx context.Context,
	id string,
) (sandbox.PreparedIntent, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox manifest preparation id is invalid")
	}
	return getSandboxManifestIntent(ctx, s.db, id)
}

func (s *SQLiteStore) GetSandboxManifestOperation(ctx context.Context,
	keyDigest string,
) (sandbox.Operation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.Operation{}, false, apperror.New(apperror.CodeInvalidArgument,
			"sandbox manifest operation digest is invalid")
	}
	return getSandboxManifestOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) ListSandboxManifestIntents(ctx context.Context, runID string,
	limit int,
) ([]sandbox.PreparedIntent, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox manifest Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 0 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox manifest list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, sandboxManifestIntentSelect+`
		WHERE preparation.run_id = ? ORDER BY preparation.prepared_at, preparation.id LIMIT ?`,
		runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sandbox.PreparedIntent, 0)
	for rows.Next() {
		value, err := scanSandboxManifestIntent(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) CreateSandboxManifestIntent(ctx context.Context,
	preparation sandbox.Preparation, validation sandbox.Validation,
	operation sandbox.Operation,
) (sandbox.PreparedIntent, bool, error) {
	if err := validateSandboxManifestMutation(preparation, validation, operation); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, preparation.RunID); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	if existing, found, err := getSandboxManifestOperation(ctx, tx, operation.KeyDigest); err != nil {
		return sandbox.PreparedIntent{}, false, err
	} else if found {
		return replaySandboxManifestIntent(ctx, tx, existing, operation)
	}
	run, mission, err := getCoordinatorRunTx(ctx, tx, preparation.RunID)
	if err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	if run.Terminal() || run.MissionID != preparation.MissionID || mission.ID != preparation.MissionID ||
		mission.WorkspaceID != preparation.WorkspaceID {
		return sandbox.PreparedIntent{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest requires a non-terminal Run with an exact Mission and workspace binding")
	}
	var workspaceCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspaces WHERE id = ?`,
		preparation.WorkspaceID).Scan(&workspaceCount); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	if workspaceCount != 1 {
		return sandbox.PreparedIntent{}, false, apperror.New(apperror.CodeNotFound,
			"sandbox manifest workspace was not found")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_manifest_preparations
		(id, run_id, mission_id, workspace_id, cancellation_id, protocol_version, backend,
		manifest_fingerprint, authorization_fingerprint, workspace_fingerprint, scope_fingerprint,
		command_argument_count, mount_count, writable_mount_count, environment_count, secret_reference_count,
		network_mode, allowed_target_count, input_artifact_count, output_count, timeout_seconds,
		grace_period_millis, cpu_quota_millis, memory_bytes, pids, max_output_bytes,
		requested_by, prepared_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		preparation.ID, preparation.RunID, preparation.MissionID, preparation.WorkspaceID,
		preparation.CancellationID, preparation.ProtocolVersion, preparation.Backend,
		preparation.ManifestFingerprint, preparation.AuthorizationFingerprint,
		preparation.WorkspaceFingerprint, preparation.ScopeFingerprint,
		preparation.CommandArgumentCount, preparation.MountCount, preparation.WritableMountCount,
		preparation.EnvironmentCount,
		preparation.SecretReferenceCount, preparation.NetworkMode, preparation.AllowedTargetCount,
		preparation.InputArtifactCount, preparation.OutputCount, preparation.TimeoutSeconds,
		preparation.GracePeriodMillis, preparation.CPUQuotaMillis, preparation.MemoryBytes,
		preparation.PIDs, preparation.MaxOutputBytes, preparation.RequestedBy,
		ts(preparation.PreparedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxManifestCreate(ctx, operation, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_manifest_validations
		(preparation_id, run_id, protocol_version, policy_allowed, needs_approval, risk,
		policy_fingerprint, approval_id, approval_status, validator_name, backend_enabled,
		execution_authorized, validated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, validation.PreparationID,
		validation.RunID, validation.ProtocolVersion, boolInt(validation.PolicyAllowed),
		boolInt(validation.NeedsApproval), validation.Risk, validation.PolicyFingerprint,
		validation.ApprovalID, validation.ApprovalStatus, validation.ValidatorName,
		boolInt(validation.BackendEnabled), boolInt(validation.ExecutionAuthorized),
		ts(validation.ValidatedAt)); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_manifest_operations
		(operation_key_digest, request_fingerprint, preparation_id, run_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, operation.KeyDigest, operation.RequestFingerprint,
		operation.PreparationID, operation.RunID, operation.RequestedBy,
		ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxManifestCreate(ctx, operation, err)
	}
	if err := appendSandboxManifestEvents(ctx, tx, preparation, validation); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxManifestCreate(ctx, operation, err)
	}
	return sandbox.PreparedIntent{Preparation: preparation, Validation: validation}, false, nil
}

func validateSandboxManifestMutation(preparation sandbox.Preparation,
	validation sandbox.Validation, operation sandbox.Operation,
) error {
	if err := preparation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest preparation is invalid", err)
	}
	if err := validation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest validation is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest operation is invalid", err)
	}
	if validation.PreparationID != preparation.ID || validation.RunID != preparation.RunID ||
		operation.PreparationID != preparation.ID || operation.RunID != preparation.RunID ||
		operation.RequestedBy != preparation.RequestedBy ||
		!validation.ValidatedAt.Equal(preparation.PreparedAt) ||
		!operation.CreatedAt.Equal(preparation.PreparedAt) ||
		operation.RequestFingerprint != sandbox.IntentRequestFingerprint(preparation, validation) {
		return apperror.New(apperror.CodeInvalidArgument,
			"sandbox manifest preparation, validation, and operation bindings do not match")
	}
	return nil
}

func appendSandboxManifestEvents(ctx context.Context, tx *sql.Tx,
	preparation sandbox.Preparation, validation sandbox.Validation,
) error {
	preparedEvent, err := events.New(preparation.RunID, preparation.MissionID,
		events.SandboxManifestPreparedEvent, "sandbox_manifest", preparation.ID, map[string]any{
			"protocol": preparation.ProtocolVersion, "backend": preparation.Backend,
			"command_argument_count":  preparation.CommandArgumentCount,
			"mount_count":             preparation.MountCount,
			"writable_mount_count":    preparation.WritableMountCount,
			"environment_count":       preparation.EnvironmentCount,
			"secret_reference_count":  preparation.SecretReferenceCount,
			"network_mode":            preparation.NetworkMode,
			"allowed_target_count":    preparation.AllowedTargetCount,
			"input_artifact_count":    preparation.InputArtifactCount,
			"output_count":            preparation.OutputCount,
			"timeout_seconds":         preparation.TimeoutSeconds,
			"cancellation_bound":      true,
			"manifest_content_stored": false,
			"execution_started":       false,
		})
	if err != nil {
		return err
	}
	preparedEvent.CreatedAt = preparation.PreparedAt
	if _, err := insertRunEventTx(ctx, tx, preparedEvent); err != nil {
		return err
	}
	validatedEvent, err := events.New(preparation.RunID, preparation.MissionID,
		events.SandboxManifestValidatedEvent, "sandbox_manifest", preparation.ID, map[string]any{
			"protocol": validation.ProtocolVersion, "validator": validation.ValidatorName,
			"policy_allowed":       validation.PolicyAllowed,
			"needs_approval":       validation.NeedsApproval,
			"risk":                 validation.Risk,
			"approval_status":      validation.ApprovalStatus,
			"approval_bound":       validation.ApprovalID != "",
			"backend_enabled":      false,
			"execution_authorized": false,
		})
	if err != nil {
		return err
	}
	validatedEvent.CreatedAt = validation.ValidatedAt
	_, err = insertRunEventTx(ctx, tx, validatedEvent)
	return err
}

func acquireSandboxManifestWriteLock(ctx context.Context, tx *sql.Tx, runID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`, runID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return apperror.New(apperror.CodeNotFound, "sandbox manifest Run was not found")
	}
	return nil
}

func replaySandboxManifestIntent(ctx context.Context, tx *sql.Tx,
	existing sandbox.Operation, requested sandbox.Operation,
) (sandbox.PreparedIntent, bool, error) {
	if err := validateSandboxManifestReplay(existing, requested); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	stored, err := getSandboxManifestIntent(ctx, tx, existing.PreparationID)
	if err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	if existing.RequestFingerprint != sandbox.IntentRequestFingerprint(stored.Preparation,
		stored.Validation) || existing.RunID != stored.Preparation.RunID ||
		existing.RequestedBy != stored.Preparation.RequestedBy {
		return sandbox.PreparedIntent{}, false, apperror.New(apperror.CodeInternal,
			"stored sandbox manifest operation binding is invalid")
	}
	if err := tx.Commit(); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	stored.Replayed = true
	return stored, true, nil
}

func validateSandboxManifestReplay(existing, requested sandbox.Operation) error {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.RunID != requested.RunID || existing.RequestedBy != requested.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"sandbox manifest operation key was already used for different intent")
	}
	return nil
}

func (s *SQLiteStore) recoverSandboxManifestCreate(ctx context.Context,
	operation sandbox.Operation, original error,
) (sandbox.PreparedIntent, bool, error) {
	existing, found, err := getSandboxManifestOperation(ctx, s.db, operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return sandbox.PreparedIntent{}, false, original
		}
		return sandbox.PreparedIntent{}, false, errors.Join(original, err)
	}
	if err := validateSandboxManifestReplay(existing, operation); err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	stored, err := getSandboxManifestIntent(ctx, s.db, existing.PreparationID)
	if err != nil {
		return sandbox.PreparedIntent{}, false, err
	}
	if existing.RequestFingerprint != sandbox.IntentRequestFingerprint(stored.Preparation,
		stored.Validation) || existing.RunID != stored.Preparation.RunID ||
		existing.RequestedBy != stored.Preparation.RequestedBy {
		return sandbox.PreparedIntent{}, false, apperror.New(apperror.CodeInternal,
			"recovered sandbox manifest operation binding is invalid")
	}
	stored.Replayed = true
	return stored, true, nil
}

func getSandboxManifestIntent(ctx context.Context, queryer sandboxManifestQueryer,
	id string,
) (sandbox.PreparedIntent, error) {
	return scanSandboxManifestIntent(queryer.QueryRowContext(ctx,
		sandboxManifestIntentSelect+` WHERE preparation.id = ?`, id))
}

func scanSandboxManifestIntent(scanner interface{ Scan(...any) error }) (sandbox.PreparedIntent, error) {
	var value sandbox.PreparedIntent
	var preparedAt, validatedAt string
	var policyAllowed, needsApproval, backendEnabled, executionAuthorized int
	if err := scanner.Scan(
		&value.Preparation.ID, &value.Preparation.RunID, &value.Preparation.MissionID,
		&value.Preparation.WorkspaceID, &value.Preparation.CancellationID,
		&value.Preparation.ProtocolVersion, &value.Preparation.Backend,
		&value.Preparation.ManifestFingerprint, &value.Preparation.AuthorizationFingerprint,
		&value.Preparation.WorkspaceFingerprint, &value.Preparation.ScopeFingerprint,
		&value.Preparation.CommandArgumentCount, &value.Preparation.MountCount,
		&value.Preparation.WritableMountCount,
		&value.Preparation.EnvironmentCount, &value.Preparation.SecretReferenceCount,
		&value.Preparation.NetworkMode, &value.Preparation.AllowedTargetCount,
		&value.Preparation.InputArtifactCount, &value.Preparation.OutputCount,
		&value.Preparation.TimeoutSeconds, &value.Preparation.GracePeriodMillis,
		&value.Preparation.CPUQuotaMillis, &value.Preparation.MemoryBytes,
		&value.Preparation.PIDs, &value.Preparation.MaxOutputBytes,
		&value.Preparation.RequestedBy, &preparedAt,
		&value.Validation.PreparationID, &value.Validation.RunID,
		&value.Validation.ProtocolVersion, &policyAllowed, &needsApproval,
		&value.Validation.Risk, &value.Validation.PolicyFingerprint,
		&value.Validation.ApprovalID, &value.Validation.ApprovalStatus,
		&value.Validation.ValidatorName, &backendEnabled, &executionAuthorized,
		&validatedAt,
	); err != nil {
		return sandbox.PreparedIntent{}, err
	}
	value.Preparation.PreparedAt = parseTS(preparedAt)
	value.Validation.PolicyAllowed = policyAllowed != 0
	value.Validation.NeedsApproval = needsApproval != 0
	value.Validation.BackendEnabled = backendEnabled != 0
	value.Validation.ExecutionAuthorized = executionAuthorized != 0
	value.Validation.ValidatedAt = parseTS(validatedAt)
	if err := value.Preparation.Validate(); err != nil {
		return sandbox.PreparedIntent{}, fmt.Errorf("stored sandbox preparation is invalid: %w", err)
	}
	if err := value.Validation.Validate(); err != nil {
		return sandbox.PreparedIntent{}, fmt.Errorf("stored sandbox validation is invalid: %w", err)
	}
	if value.Validation.PreparationID != value.Preparation.ID ||
		value.Validation.RunID != value.Preparation.RunID ||
		!value.Validation.ValidatedAt.Equal(value.Preparation.PreparedAt) {
		return sandbox.PreparedIntent{}, errors.New("stored sandbox manifest intent binding is invalid")
	}
	return value, nil
}

func getSandboxManifestOperation(ctx context.Context, queryer sandboxManifestQueryer,
	keyDigest string,
) (sandbox.Operation, bool, error) {
	var operation sandbox.Operation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		preparation_id, run_id, requested_by, created_at FROM sandbox_manifest_operations
		WHERE operation_key_digest = ?`, keyDigest).Scan(&operation.KeyDigest,
		&operation.RequestFingerprint, &operation.PreparationID, &operation.RunID,
		&operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.Operation{}, false, nil
	}
	if err != nil {
		return sandbox.Operation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}
