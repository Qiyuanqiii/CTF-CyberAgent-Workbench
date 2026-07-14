package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/sandbox"
)

const sandboxDisabledPreflightSelect = `SELECT id, execution_id, candidate_id,
	preparation_id, run_id, mission_id, workspace_id, protocol_version, backend,
	manifest_fingerprint, authorization_fingerprint, policy_fingerprint,
	mount_binding_fingerprint, input_artifact_digest, handshake_protocol,
	inspector_name, handshake_status, backend_available, threat_model_fingerprint,
	container_identity_protocol, container_runtime, container_identity_bound,
	container_identity_fingerprint, output_protocol, capture_stdout, capture_stderr,
	output_plan_fingerprint, output_slot_count, max_output_bytes, partial_failure_policy, truncation_policy,
	mime_policy, file_type_policy, restart_policy, raw_paths_stored,
	output_export_enabled, artifact_commit_authorized, backend_enabled,
	execution_authorized, requested_by, created_at FROM sandbox_disabled_preflights`

func (s *SQLiteStore) GetSandboxPreflightOperation(ctx context.Context,
	keyDigest string,
) (sandbox.PreflightOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.PreflightOperation{}, false, apperror.New(apperror.CodeInvalidArgument,
			"sandbox preflight operation digest is invalid")
	}
	return getSandboxPreflightOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateSandboxDisabledPreflight(ctx context.Context,
	preflight sandbox.DisabledPreflight, operation sandbox.PreflightOperation,
) (sandbox.DisabledPreflight, bool, error) {
	if err := validateSandboxPreflightMutation(preflight, operation); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DisabledPreflight{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, preflight.RunID); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if existing, found, err := getSandboxPreflightOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	} else if found {
		return replaySandboxDisabledPreflight(ctx, tx, existing, operation)
	}
	if _, err := getSandboxDisabledPreflightByExecution(ctx, tx,
		preflight.ExecutionID); err == nil {
		return sandbox.DisabledPreflight{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution already has a preflight")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return sandbox.DisabledPreflight{}, false, err
	}
	lifecycle, err := getSandboxLifecycle(ctx, tx, preflight.ExecutionID)
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	candidate, err := getSandboxExecutionCandidate(ctx, tx, preflight.CandidateID)
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	intent, err := getSandboxManifestIntent(ctx, tx, preflight.PreparationID)
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	run, mission, err := getCoordinatorRunTx(ctx, tx, preflight.RunID)
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if err := validateSandboxPreflightCurrentBinding(preflight, lifecycle, candidate,
		intent, run, mission); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if err := validateSandboxCandidateApprovalTx(ctx, tx, run, intent, candidate); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	usage, err := getRunAgentUsageTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	toolCalls, err := getRunToolCallCountTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if usage.TotalTokens != candidate.TokensUsed ||
		usage.TotalExecutionMillis != candidate.ExecutionMillisUsed ||
		toolCalls != candidate.ToolCallsUsed {
		return sandbox.DisabledPreflight{}, false, apperror.New(apperror.CodeConflict,
			"sandbox candidate usage changed before preflight")
	}
	if err := requireSandboxCandidateStoreBudget(run.Budget, usage, toolCalls); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if lease, found, err := getRunExecutionLeaseTx(ctx, tx, run.ID); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	} else if found && lease.ActiveAt(time.Now().UTC()) {
		return sandbox.DisabledPreflight{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox preflight requires a quiescent Run")
	}
	if err := verifySandboxInputBindingsTx(ctx, tx, lifecycle.Execution,
		lifecycle.Inputs, run.SessionID); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if err := insertSandboxDisabledPreflightTx(ctx, tx, preflight); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxDisabledPreflightCreate(ctx, operation, err)
	}
	for _, check := range preflight.Handshake.Checks {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_backend_preflight_checks
			(preflight_id, ordinal, name, required, verified, evidence_state)
			VALUES (?, ?, ?, ?, ?, ?)`, preflight.ID, check.Ordinal, check.Name,
			boolInt(check.Required), boolInt(check.Verified), check.EvidenceState); err != nil {
			return sandbox.DisabledPreflight{}, false, err
		}
	}
	for _, slot := range preflight.OutputPlan.Slots {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_output_export_slots
			(preflight_id, ordinal, kind, locator_fingerprint, regular_file_required,
			symlink_rejected, special_file_rejected, mime_detection_required,
			redaction_required, artifact_commit_authorized)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, preflight.ID, slot.Ordinal, slot.Kind,
			slot.LocatorFingerprint, boolInt(slot.RegularFileRequired),
			boolInt(slot.SymlinkRejected), boolInt(slot.SpecialFileRejected),
			boolInt(slot.MIMEDetectionRequired), boolInt(slot.RedactionRequired),
			boolInt(slot.ArtifactCommitAuthorized)); err != nil {
			return sandbox.DisabledPreflight{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_preflight_operations
		(operation_key_digest, request_fingerprint, preflight_id, execution_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.PreflightID, operation.ExecutionID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxDisabledPreflightCreate(ctx, operation, err)
	}
	if err := appendSandboxPreflightEvent(ctx, tx, preflight); err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxDisabledPreflightCreate(ctx, operation, err)
	}
	return preflight, false, nil
}

func (s *SQLiteStore) GetSandboxDisabledPreflight(ctx context.Context,
	id string,
) (sandbox.DisabledPreflight, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox preflight id is invalid")
	}
	return getSandboxDisabledPreflight(ctx, s.db, id)
}

func (s *SQLiteStore) ListSandboxDisabledPreflights(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DisabledPreflight, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox preflight list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox preflight list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_disabled_preflights
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
	values := make([]sandbox.DisabledPreflight, 0, len(ids))
	for _, id := range ids {
		value, err := getSandboxDisabledPreflight(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func validateSandboxPreflightMutation(preflight sandbox.DisabledPreflight,
	operation sandbox.PreflightOperation,
) error {
	if err := preflight.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox preflight is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox preflight operation is invalid", err)
	}
	if operation.PreflightID != preflight.ID || operation.ExecutionID != preflight.ExecutionID ||
		operation.RunID != preflight.RunID || operation.RequestedBy != preflight.RequestedBy ||
		!operation.CreatedAt.Equal(preflight.CreatedAt) ||
		operation.RequestFingerprint != sandbox.PreflightRequestFingerprint(preflight) {
		return apperror.New(apperror.CodeConflict,
			"sandbox preflight operation does not match its request")
	}
	return nil
}

func validateSandboxPreflightCurrentBinding(preflight sandbox.DisabledPreflight,
	lifecycle sandbox.Lifecycle, candidate sandbox.ExecutionCandidate,
	intent sandbox.PreparedIntent, run domain.Run, mission domain.Mission,
) error {
	execution := lifecycle.Execution
	if lifecycle.Cancellation != nil || lifecycle.Cleanup != nil ||
		lifecycle.Status != sandbox.LifecyclePrepared ||
		lifecycle.Lease.Status != sandbox.ExecutionLeaseReleased {
		return apperror.New(apperror.CodeFailedPrecondition,
			"sandbox execution is not eligible for disabled preflight")
	}
	if run.Terminal() || run.MissionID != mission.ID || mission.WorkspaceID != preflight.WorkspaceID ||
		preflight.ExecutionID != execution.ID || preflight.CandidateID != candidate.ID ||
		preflight.PreparationID != intent.Preparation.ID || preflight.RunID != run.ID ||
		preflight.MissionID != mission.ID || execution.CandidateID != candidate.ID ||
		execution.PreparationID != intent.Preparation.ID || execution.RunID != run.ID ||
		execution.MissionID != mission.ID || execution.WorkspaceID != mission.WorkspaceID ||
		candidate.PreparationID != intent.Preparation.ID || candidate.RunID != run.ID ||
		intent.Preparation.RunID != run.ID || intent.Preparation.MissionID != mission.ID ||
		intent.Preparation.WorkspaceID != mission.WorkspaceID {
		return apperror.New(apperror.CodeConflict,
			"sandbox preflight Run, execution, candidate, or preparation binding changed")
	}
	if preflight.Backend != intent.Preparation.Backend ||
		preflight.ManifestFingerprint != execution.ManifestFingerprint ||
		preflight.ManifestFingerprint != candidate.ManifestFingerprint ||
		preflight.AuthorizationFingerprint != execution.AuthorizationFingerprint ||
		preflight.AuthorizationFingerprint != candidate.AuthorizationFingerprint ||
		preflight.PolicyFingerprint != execution.PolicyFingerprint ||
		preflight.PolicyFingerprint != candidate.PolicyFingerprint ||
		preflight.MountBindingFingerprint != execution.MountBindingFingerprint ||
		preflight.MountBindingFingerprint != candidate.MountBindingFingerprint ||
		preflight.InputArtifactDigest != execution.InputArtifactDigest ||
		preflight.Handshake.ThreatModelFingerprint != sandbox.BackendThreatModelFingerprint(
			preflight.Handshake.Checks) {
		return apperror.New(apperror.CodeConflict,
			"sandbox preflight authority or threat-model binding changed")
	}
	if preflight.OutputPlan.CaptureStdout != execution.OutputPlan.CaptureStdout ||
		preflight.OutputPlan.CaptureStderr != execution.OutputPlan.CaptureStderr ||
		preflight.OutputPlan.SlotCount != execution.OutputPlan.OutputPathCount+
			boolCount(execution.OutputPlan.CaptureStdout, execution.OutputPlan.CaptureStderr) ||
		preflight.OutputPlan.MaxOutputBytes != execution.OutputPlan.MaxOutputBytes {
		return apperror.New(apperror.CodeConflict,
			"sandbox preflight output binding changed")
	}
	return nil
}

func insertSandboxDisabledPreflightTx(ctx context.Context, tx *sql.Tx,
	p sandbox.DisabledPreflight,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_disabled_preflights
		(id, execution_id, candidate_id, preparation_id, run_id, mission_id, workspace_id,
		protocol_version, backend, manifest_fingerprint, authorization_fingerprint,
		policy_fingerprint, mount_binding_fingerprint, input_artifact_digest,
		handshake_protocol, inspector_name, handshake_status, backend_available,
		threat_model_fingerprint, container_identity_protocol, container_runtime,
		container_identity_bound, container_identity_fingerprint, output_protocol,
		capture_stdout, capture_stderr, output_plan_fingerprint, output_slot_count, max_output_bytes,
		partial_failure_policy, truncation_policy, mime_policy, file_type_policy,
		restart_policy, raw_paths_stored, output_export_enabled, artifact_commit_authorized,
		backend_enabled, execution_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ExecutionID, p.CandidateID, p.PreparationID, p.RunID, p.MissionID,
		p.WorkspaceID, p.ProtocolVersion, p.Backend, p.ManifestFingerprint,
		p.AuthorizationFingerprint, p.PolicyFingerprint, p.MountBindingFingerprint,
		p.InputArtifactDigest, p.Handshake.ProtocolVersion, p.Handshake.InspectorName,
		p.Handshake.Status, boolInt(p.Handshake.Available),
		p.Handshake.ThreatModelFingerprint, p.Handshake.ContainerIdentity.ProtocolVersion,
		p.Handshake.ContainerIdentity.Runtime, boolInt(p.Handshake.ContainerIdentity.Bound),
		p.Handshake.ContainerIdentity.Fingerprint, p.OutputPlan.ProtocolVersion,
		boolInt(p.OutputPlan.CaptureStdout), boolInt(p.OutputPlan.CaptureStderr),
		p.OutputPlan.Fingerprint, p.OutputPlan.SlotCount, p.OutputPlan.MaxOutputBytes,
		p.OutputPlan.PartialFailurePolicy, p.OutputPlan.TruncationPolicy,
		p.OutputPlan.MIMEPolicy, p.OutputPlan.FileTypePolicy, p.OutputPlan.RestartPolicy,
		boolInt(p.OutputPlan.RawPathsStored), boolInt(p.OutputPlan.ExportEnabled),
		boolInt(p.OutputPlan.ArtifactCommitAuthorized), boolInt(p.BackendEnabled),
		boolInt(p.ExecutionAuthorized), p.RequestedBy, ts(p.CreatedAt))
	return err
}

func getSandboxDisabledPreflight(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DisabledPreflight, error) {
	preflight, err := scanSandboxDisabledPreflight(queryer.QueryRowContext(ctx,
		sandboxDisabledPreflightSelect+` WHERE id = ?`, id))
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	checks, err := listSandboxPreflightChecks(ctx, queryer, id)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	slots, err := listSandboxOutputExportSlots(ctx, queryer, id)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	preflight.Handshake.Checks = checks
	preflight.OutputPlan.Slots = slots
	if err := preflight.Validate(); err != nil {
		return sandbox.DisabledPreflight{}, apperror.Wrap(apperror.CodeInternal,
			"stored sandbox preflight is invalid", err)
	}
	return preflight, nil
}

func getSandboxDisabledPreflightByExecution(ctx context.Context,
	queryer sandboxLifecycleQueryer, executionID string,
) (sandbox.DisabledPreflight, error) {
	var id string
	if err := queryer.QueryRowContext(ctx, `SELECT id FROM sandbox_disabled_preflights
		WHERE execution_id = ?`, executionID).Scan(&id); err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	return getSandboxDisabledPreflight(ctx, queryer, id)
}

func scanSandboxDisabledPreflight(row scanner) (sandbox.DisabledPreflight, error) {
	var value sandbox.DisabledPreflight
	var backendAvailable, containerBound int
	var captureStdout, captureStderr, rawPathsStored, exportEnabled int
	var artifactCommitAuthorized, backendEnabled, executionAuthorized int
	var createdAt string
	if err := row.Scan(&value.ID, &value.ExecutionID, &value.CandidateID,
		&value.PreparationID, &value.RunID, &value.MissionID, &value.WorkspaceID,
		&value.ProtocolVersion, &value.Backend, &value.ManifestFingerprint,
		&value.AuthorizationFingerprint, &value.PolicyFingerprint,
		&value.MountBindingFingerprint, &value.InputArtifactDigest,
		&value.Handshake.ProtocolVersion, &value.Handshake.InspectorName,
		&value.Handshake.Status, &backendAvailable,
		&value.Handshake.ThreatModelFingerprint,
		&value.Handshake.ContainerIdentity.ProtocolVersion,
		&value.Handshake.ContainerIdentity.Runtime, &containerBound,
		&value.Handshake.ContainerIdentity.Fingerprint,
		&value.OutputPlan.ProtocolVersion, &captureStdout, &captureStderr,
		&value.OutputPlan.Fingerprint, &value.OutputPlan.SlotCount, &value.OutputPlan.MaxOutputBytes,
		&value.OutputPlan.PartialFailurePolicy, &value.OutputPlan.TruncationPolicy,
		&value.OutputPlan.MIMEPolicy, &value.OutputPlan.FileTypePolicy,
		&value.OutputPlan.RestartPolicy, &rawPathsStored, &exportEnabled,
		&artifactCommitAuthorized, &backendEnabled, &executionAuthorized,
		&value.RequestedBy, &createdAt); err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	value.Handshake.Backend = value.Backend
	value.Status = value.Handshake.Status
	value.Handshake.Available = backendAvailable == 1
	value.Handshake.ContainerIdentity.Bound = containerBound == 1
	value.OutputPlan.CaptureStdout = captureStdout == 1
	value.OutputPlan.CaptureStderr = captureStderr == 1
	value.OutputPlan.RawPathsStored = rawPathsStored == 1
	value.OutputPlan.ExportEnabled = exportEnabled == 1
	value.OutputPlan.ArtifactCommitAuthorized = artifactCommitAuthorized == 1
	value.BackendEnabled = backendEnabled == 1
	value.ExecutionAuthorized = executionAuthorized == 1
	value.ArtifactCommitAuthorized = artifactCommitAuthorized == 1
	value.CreatedAt = parseTS(createdAt)
	return value, nil
}

func listSandboxPreflightChecks(ctx context.Context, queryer sandboxLifecycleQueryer,
	preflightID string,
) ([]sandbox.BackendCheck, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, required, verified,
		evidence_state FROM sandbox_backend_preflight_checks
		WHERE preflight_id = ? ORDER BY ordinal`, preflightID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	checks := make([]sandbox.BackendCheck, 0, sandbox.MaxBackendChecks)
	for rows.Next() {
		var check sandbox.BackendCheck
		var required, verified int
		if err := rows.Scan(&check.Ordinal, &check.Name, &required, &verified,
			&check.EvidenceState); err != nil {
			return nil, err
		}
		check.Required, check.Verified = required == 1, verified == 1
		checks = append(checks, check)
	}
	return checks, rows.Err()
}

func listSandboxOutputExportSlots(ctx context.Context, queryer sandboxLifecycleQueryer,
	preflightID string,
) ([]sandbox.OutputExportSlot, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, kind, locator_fingerprint,
		regular_file_required, symlink_rejected, special_file_rejected,
		mime_detection_required, redaction_required, artifact_commit_authorized
		FROM sandbox_output_export_slots WHERE preflight_id = ? ORDER BY ordinal`, preflightID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	slots := make([]sandbox.OutputExportSlot, 0, sandbox.MaxOutputPaths+2)
	for rows.Next() {
		var slot sandbox.OutputExportSlot
		var regular, symlink, special, mimeRequired, redaction, commit int
		if err := rows.Scan(&slot.Ordinal, &slot.Kind, &slot.LocatorFingerprint,
			&regular, &symlink, &special, &mimeRequired, &redaction, &commit); err != nil {
			return nil, err
		}
		slot.RegularFileRequired = regular == 1
		slot.SymlinkRejected = symlink == 1
		slot.SpecialFileRejected = special == 1
		slot.MIMEDetectionRequired = mimeRequired == 1
		slot.RedactionRequired = redaction == 1
		slot.ArtifactCommitAuthorized = commit == 1
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

func getSandboxPreflightOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.PreflightOperation, bool, error) {
	var value sandbox.PreflightOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		preflight_id, execution_id, run_id, requested_by, created_at
		FROM sandbox_preflight_operations WHERE operation_key_digest = ?`, keyDigest).Scan(
		&value.KeyDigest, &value.RequestFingerprint, &value.PreflightID,
		&value.ExecutionID, &value.RunID, &value.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.PreflightOperation{}, false, nil
	}
	if err != nil {
		return sandbox.PreflightOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.PreflightOperation{}, false, apperror.Wrap(apperror.CodeInternal,
			"stored sandbox preflight operation is invalid", err)
	}
	return value, true, nil
}

func replaySandboxDisabledPreflight(ctx context.Context, queryer sandboxLifecycleQueryer,
	existing, requested sandbox.PreflightOperation,
) (sandbox.DisabledPreflight, bool, error) {
	if existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.ExecutionID != requested.ExecutionID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return sandbox.DisabledPreflight{}, false, apperror.New(apperror.CodeConflict,
			"sandbox preflight operation key was reused for different intent")
	}
	value, err := getSandboxDisabledPreflight(ctx, queryer, existing.PreflightID)
	if err != nil {
		return sandbox.DisabledPreflight{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func (s *SQLiteStore) recoverSandboxDisabledPreflightCreate(ctx context.Context,
	operation sandbox.PreflightOperation, cause error,
) (sandbox.DisabledPreflight, bool, error) {
	existing, found, err := getSandboxPreflightOperation(ctx, s.db, operation.KeyDigest)
	if err == nil && found {
		return replaySandboxDisabledPreflight(ctx, s.db, existing, operation)
	}
	if err != nil {
		return sandbox.DisabledPreflight{}, false, errors.Join(cause, err)
	}
	if _, lookupErr := getSandboxDisabledPreflightByExecution(ctx, s.db,
		operation.ExecutionID); lookupErr == nil {
		return sandbox.DisabledPreflight{}, false, apperror.New(apperror.CodeConflict,
			"sandbox execution already has a preflight")
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return sandbox.DisabledPreflight{}, false, errors.Join(cause, lookupErr)
	}
	return sandbox.DisabledPreflight{}, false, cause
}

func appendSandboxPreflightEvent(ctx context.Context, tx *sql.Tx,
	preflight sandbox.DisabledPreflight,
) error {
	event, err := events.New(preflight.RunID, preflight.MissionID,
		events.SandboxPreflightRecordedEvent, "sandbox_preflight", preflight.ID,
		map[string]any{
			"protocol": preflight.ProtocolVersion, "backend": preflight.Backend,
			"status":          preflight.Status,
			"required_checks": len(preflight.Handshake.Checks), "verified_checks": 0,
			"container_identity_bound": false, "backend_available": false,
			"output_slots":           preflight.OutputPlan.SlotCount,
			"max_output_bytes":       preflight.OutputPlan.MaxOutputBytes,
			"partial_failure_policy": preflight.OutputPlan.PartialFailurePolicy,
			"raw_paths_stored":       false, "output_export_enabled": false,
			"artifact_commit_authorized": false, "backend_enabled": false,
			"execution_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = preflight.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}
