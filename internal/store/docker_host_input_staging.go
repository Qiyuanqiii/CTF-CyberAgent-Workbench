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

const dockerHostInputStagingIntentSelect = `SELECT id, attempt_id, plan_id, run_id,
	mission_id, workspace_id, protocol_version, operation_key_digest,
	attempt_intent_fingerprint, request_fingerprint, container_id_fingerprint,
	manifest_fingerprint, mount_binding_fingerprint, input_artifact_digest,
	authority_fingerprint, spec_fingerprint, plan_fingerprint, read_only_mount_count,
	input_artifact_count, prepared_generation, intent_fingerprint, requested_by, created_at
	FROM sandbox_docker_host_input_staging_intents`

const dockerHostInputStagingSelect = `SELECT id, intent_id, attempt_id, plan_id, run_id,
	protocol_version, source, trust_class, status, lease_generation,
	attempt_intent_fingerprint, container_id_fingerprint, input_artifact_digest,
	authority_fingerprint, read_only_mount_count, input_artifact_count,
	bundle_protocol_version, source, bundle_status, regular_file_count, directory_count,
	entry_count, source_bytes, artifact_bytes, bundle_bytes, source_snapshot_digest,
	artifact_payload_digest, bundle_digest, report_fingerprint, descriptor_pinned,
	symlink_free, kernel_sealed, source_paths_retained, raw_content_persisted,
	daemon_consumed, container_started, process_executed, execution_evidence,
	staging_fingerprint, production_verified, backend_enabled, execution_authorized,
	artifact_commit_authorized, bundle_created_at, created_at
	FROM sandbox_docker_host_input_stagings`

func (s *SQLiteStore) PrepareDockerHostInputStagingIntent(ctx context.Context,
	intent sandbox.DockerHostInputStagingIntent,
	expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	if intent.Validate() != nil || expected.Validate() != nil ||
		intent.AttemptID != expected.AttemptID || intent.PreparedGeneration != expected.Generation ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging intent binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, intent.AttemptID)
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.Intent.RunID); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if existing, found, err := getDockerHostInputStagingByOperation(ctx, tx,
		intent.OperationKeyDigest); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	} else if found {
		if existing.Intent.IntentFingerprint != intent.IntentFingerprint ||
			existing.Intent.AttemptID != intent.AttemptID ||
			existing.Intent.RequestedBy != intent.RequestedBy {
			return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
				apperror.CodeConflict, "Docker host input staging operation changed")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerHostInputStagingRecord{}, false, err
		}
		existing.Replayed = true
		return existing, true, nil
	}
	if _, found, err := getDockerHostInputStagingByAttempt(ctx, tx,
		intent.AttemptID); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	} else if found {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeConflict, "Docker container attempt already has a different host input staging intent")
	}
	if err := validateDockerHostInputStagingIntentCurrentTx(ctx, tx, intent, attempt); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if intent.CreatedAt.After(time.Now().UTC()) {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging intent timestamp is in the future")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_host_input_staging_intents
		(id, attempt_id, plan_id, run_id, mission_id, workspace_id, protocol_version,
		operation_key_digest, attempt_intent_fingerprint, request_fingerprint,
		container_id_fingerprint, manifest_fingerprint, mount_binding_fingerprint,
		input_artifact_digest, authority_fingerprint, spec_fingerprint, plan_fingerprint,
		read_only_mount_count, input_artifact_count, prepared_generation,
		intent_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		intent.ID, intent.AttemptID, intent.PlanID, intent.RunID, intent.MissionID,
		intent.WorkspaceID, intent.ProtocolVersion, intent.OperationKeyDigest,
		intent.AttemptIntentFingerprint, intent.RequestFingerprint,
		intent.ContainerIDFingerprint, intent.ManifestFingerprint,
		intent.MountBindingFingerprint, intent.InputArtifactDigest,
		intent.AuthorityFingerprint, intent.SpecFingerprint, intent.PlanFingerprint,
		intent.ReadOnlyMountCount, intent.InputArtifactCount, intent.PreparedGeneration,
		intent.IntentFingerprint, intent.RequestedBy, ts(intent.CreatedAt)); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := appendDockerHostInputStagingEvent(ctx, tx, intent,
		events.SandboxDockerHostInputIntentEvent, intent.ID, intent.CreatedAt, map[string]any{
			"status":                sandbox.DockerContainerAttemptStatusStaged,
			"lease_generation":      intent.PreparedGeneration,
			"read_only_mount_count": intent.ReadOnlyMountCount,
			"input_artifact_count":  intent.InputArtifactCount,
			"container_started":     false, "process_executed": false,
			"daemon_consumed": false, "execution_authorized": false,
		}); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	record := sandbox.DockerHostInputStagingRecord{Intent: intent}
	return record, false, record.Validate()
}

func (s *SQLiteStore) RecordDockerHostInputStaging(ctx context.Context,
	value sandbox.DockerHostInputStaging,
	expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	if value.Validate() != nil || expected.Validate() != nil ||
		value.AttemptID != expected.AttemptID || value.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging result binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerHostInputStagingRecord(ctx, tx, value.IntentID)
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, record.Intent.AttemptID)
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if record.Staging != nil {
		if record.Staging.StagingFingerprint != value.StagingFingerprint {
			return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
				apperror.CodeConflict, "Docker host input staging result changed")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerHostInputStagingRecord{}, false, err
		}
		record.Replayed = true
		return record, true, nil
	}
	if err := validateDockerHostInputStagingResultCurrentTx(ctx, tx, value,
		record.Intent, attempt); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	now := time.Now().UTC()
	if value.CreatedAt.After(now) || value.Report.CreatedAt.After(value.CreatedAt) {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging timestamp is invalid")
	}
	report := value.Report
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_host_input_stagings
		(id, intent_id, attempt_id, plan_id, run_id, protocol_version, source, trust_class,
		status, lease_generation, attempt_intent_fingerprint, container_id_fingerprint,
		input_artifact_digest, authority_fingerprint, read_only_mount_count,
		input_artifact_count, bundle_protocol_version, bundle_status, regular_file_count,
		directory_count, entry_count, source_bytes, artifact_bytes, bundle_bytes,
		source_snapshot_digest, artifact_payload_digest, bundle_digest, report_fingerprint,
		descriptor_pinned, symlink_free, kernel_sealed, source_paths_retained,
		raw_content_persisted, daemon_consumed, container_started, process_executed,
		execution_evidence, staging_fingerprint, production_verified, backend_enabled,
		execution_authorized, artifact_commit_authorized, bundle_created_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID,
		value.IntentID, value.AttemptID, value.PlanID, value.RunID, value.ProtocolVersion,
		value.Source, value.TrustClass, value.Status, value.LeaseGeneration,
		value.AttemptIntentFingerprint, value.ContainerIDFingerprint,
		value.InputArtifactDigest, value.AuthorityFingerprint, value.ReadOnlyMountCount,
		value.InputArtifactCount, report.ProtocolVersion, report.Status,
		report.RegularFileCount, report.DirectoryCount, report.EntryCount,
		report.SourceBytes, report.ArtifactBytes, report.BundleBytes,
		report.SourceSnapshotDigest, report.ArtifactPayloadDigest, report.BundleDigest,
		report.ReportFingerprint, boolInt(report.DescriptorPinned), boolInt(report.SymlinkFree),
		boolInt(report.KernelSealed), boolInt(report.SourcePathsRetained),
		boolInt(report.RawContentPersisted), boolInt(report.DaemonConsumed),
		boolInt(report.ContainerStarted), boolInt(report.ProcessExecuted),
		boolInt(report.ExecutionEvidence), value.StagingFingerprint,
		boolInt(value.ProductionVerified), boolInt(value.BackendEnabled),
		boolInt(value.ExecutionAuthorized), boolInt(value.ArtifactCommitAuthorized),
		ts(report.CreatedAt), ts(value.CreatedAt)); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := appendDockerHostInputStagingEvent(ctx, tx, record.Intent,
		events.SandboxDockerHostInputStagedEvent, value.ID, value.CreatedAt, map[string]any{
			"status": value.Status, "lease_generation": value.LeaseGeneration,
			"read_only_mount_count": value.ReadOnlyMountCount,
			"input_artifact_count":  value.InputArtifactCount,
			"entry_count":           report.EntryCount, "source_bytes": report.SourceBytes,
			"artifact_bytes": report.ArtifactBytes, "bundle_bytes": report.BundleBytes,
			"descriptor_pinned": report.DescriptorPinned, "kernel_sealed": report.KernelSealed,
			"daemon_consumed": false, "container_started": false,
			"process_executed": false, "execution_evidence": false,
			"execution_authorized": false,
		}); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	record.Staging = &value
	return record, false, record.Validate()
}

func (s *SQLiteStore) GetDockerHostInputStaging(ctx context.Context,
	id string,
) (sandbox.DockerHostInputStagingRecord, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerHostInputStagingRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging id is invalid")
	}
	return getDockerHostInputStagingRecord(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerHostInputStagingByAttempt(ctx context.Context,
	attemptID string,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	attemptID = strings.TrimSpace(attemptID)
	if !domain.ValidAgentID(attemptID) || strings.ContainsRune(attemptID, 0) {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging attempt id is invalid")
	}
	return getDockerHostInputStagingByAttempt(ctx, s.db, attemptID)
}

func (s *SQLiteStore) GetDockerHostInputStagingByOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging operation digest is invalid")
	}
	return getDockerHostInputStagingByOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) GetDockerHostInputStagingByPlan(ctx context.Context,
	planID string,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	planID = strings.TrimSpace(planID)
	if !domain.ValidAgentID(planID) || strings.ContainsRune(planID, 0) {
		return sandbox.DockerHostInputStagingRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input staging plan id is invalid")
	}
	return getDockerHostInputStagingByPlan(ctx, s.db, planID)
}

func (s *SQLiteStore) ListDockerHostInputStagings(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerHostInputStagingRecord, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker host input staging list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker host input staging list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id
		FROM sandbox_docker_host_input_staging_intents
		WHERE run_id = ? ORDER BY created_at, id LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	values := make([]sandbox.DockerHostInputStagingRecord, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerHostInputStagingRecord(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerHostInputStagingByOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_host_input_staging_intents WHERE operation_key_digest = ?`,
		keyDigest).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputStagingRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	value, err := getDockerHostInputStagingRecord(ctx, queryer, id)
	return value, err == nil, err
}

func getDockerHostInputStagingByAttempt(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_host_input_staging_intents WHERE attempt_id = ?`,
		attemptID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputStagingRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	value, err := getDockerHostInputStagingRecord(ctx, queryer, id)
	return value, err == nil, err
}

func getDockerHostInputStagingByPlan(ctx context.Context,
	queryer sandboxLifecycleQueryer, planID string,
) (sandbox.DockerHostInputStagingRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_host_input_staging_intents WHERE plan_id = ?
		ORDER BY created_at DESC, id DESC LIMIT 1`, planID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputStagingRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, false, err
	}
	value, err := getDockerHostInputStagingRecord(ctx, queryer, id)
	return value, err == nil, err
}

func getDockerHostInputStagingRecord(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerHostInputStagingRecord, error) {
	intent, err := scanDockerHostInputStagingIntent(queryer.QueryRowContext(ctx,
		dockerHostInputStagingIntentSelect+` WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sandbox.DockerHostInputStagingRecord{}, apperror.New(
				apperror.CodeNotFound, "Docker host input staging not found")
		}
		return sandbox.DockerHostInputStagingRecord{}, err
	}
	staging, found, err := getDockerHostInputStagingResult(ctx, queryer, intent.ID)
	if err != nil {
		return sandbox.DockerHostInputStagingRecord{}, err
	}
	record := sandbox.DockerHostInputStagingRecord{Intent: intent}
	if found {
		record.Staging = &staging
	}
	if err := record.Validate(); err != nil {
		return sandbox.DockerHostInputStagingRecord{}, fmt.Errorf(
			"stored Docker host input staging is invalid: %w", err)
	}
	return record, nil
}

func scanDockerHostInputStagingIntent(row scanner) (sandbox.DockerHostInputStagingIntent, error) {
	var intent sandbox.DockerHostInputStagingIntent
	var createdAt string
	err := row.Scan(&intent.ID, &intent.AttemptID, &intent.PlanID, &intent.RunID,
		&intent.MissionID, &intent.WorkspaceID, &intent.ProtocolVersion,
		&intent.OperationKeyDigest, &intent.AttemptIntentFingerprint,
		&intent.RequestFingerprint, &intent.ContainerIDFingerprint,
		&intent.ManifestFingerprint, &intent.MountBindingFingerprint,
		&intent.InputArtifactDigest, &intent.AuthorityFingerprint,
		&intent.SpecFingerprint, &intent.PlanFingerprint, &intent.ReadOnlyMountCount,
		&intent.InputArtifactCount, &intent.PreparedGeneration,
		&intent.IntentFingerprint, &intent.RequestedBy, &createdAt)
	intent.CreatedAt = parseTS(createdAt)
	if err == nil {
		err = intent.Validate()
	}
	return intent, err
}

func getDockerHostInputStagingResult(ctx context.Context, queryer sandboxLifecycleQueryer,
	intentID string,
) (sandbox.DockerHostInputStaging, bool, error) {
	var value sandbox.DockerHostInputStaging
	var bundleCreatedAt, createdAt string
	var descriptorPinned, symlinkFree, kernelSealed, sourcePathsRetained int
	var rawContentPersisted, daemonConsumed, containerStarted, processExecuted int
	var executionEvidence, productionVerified, backendEnabled, executionAuthorized int
	var artifactAuthorized int
	err := queryer.QueryRowContext(ctx, dockerHostInputStagingSelect+` WHERE intent_id = ?`,
		intentID).Scan(&value.ID, &value.IntentID, &value.AttemptID, &value.PlanID,
		&value.RunID, &value.ProtocolVersion, &value.Source, &value.TrustClass,
		&value.Status, &value.LeaseGeneration, &value.AttemptIntentFingerprint,
		&value.ContainerIDFingerprint, &value.InputArtifactDigest,
		&value.AuthorityFingerprint, &value.ReadOnlyMountCount,
		&value.InputArtifactCount, &value.Report.ProtocolVersion, &value.Report.Source,
		&value.Report.Status, &value.Report.RegularFileCount,
		&value.Report.DirectoryCount, &value.Report.EntryCount, &value.Report.SourceBytes,
		&value.Report.ArtifactBytes, &value.Report.BundleBytes,
		&value.Report.SourceSnapshotDigest, &value.Report.ArtifactPayloadDigest,
		&value.Report.BundleDigest, &value.Report.ReportFingerprint, &descriptorPinned,
		&symlinkFree, &kernelSealed, &sourcePathsRetained, &rawContentPersisted,
		&daemonConsumed, &containerStarted, &processExecuted, &executionEvidence,
		&value.StagingFingerprint, &productionVerified, &backendEnabled,
		&executionAuthorized, &artifactAuthorized, &bundleCreatedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputStaging{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputStaging{}, false, err
	}
	value.Report.ReadOnlyMountCount = value.ReadOnlyMountCount
	value.Report.ArtifactCount = value.InputArtifactCount
	value.Report.DescriptorPinned = descriptorPinned != 0
	value.Report.SymlinkFree = symlinkFree != 0
	value.Report.KernelSealed = kernelSealed != 0
	value.Report.SourcePathsRetained = sourcePathsRetained != 0
	value.Report.RawContentPersisted = rawContentPersisted != 0
	value.Report.DaemonConsumed = daemonConsumed != 0
	value.Report.ContainerStarted = containerStarted != 0
	value.Report.ProcessExecuted = processExecuted != 0
	value.Report.ExecutionEvidence = executionEvidence != 0
	value.Report.CreatedAt = parseTS(bundleCreatedAt)
	value.ProductionVerified = productionVerified != 0
	value.BackendEnabled = backendEnabled != 0
	value.ExecutionAuthorized = executionAuthorized != 0
	value.ArtifactCommitAuthorized = artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerHostInputStaging{}, false, err
	}
	return value, true, nil
}

func validateDockerHostInputStagingIntentCurrentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerHostInputStagingIntent,
	attempt sandbox.DockerContainerRehearsalAttempt,
) error {
	if attempt.Stage == nil || attempt.Cleanup != nil || attempt.Completion != nil ||
		attempt.Intent.ID != intent.AttemptID || attempt.Intent.PlanID != intent.PlanID ||
		attempt.Intent.RunID != intent.RunID || attempt.Intent.MissionID != intent.MissionID ||
		attempt.Intent.WorkspaceID != intent.WorkspaceID ||
		attempt.Intent.IntentFingerprint != intent.AttemptIntentFingerprint ||
		attempt.Intent.RequestFingerprint != intent.RequestFingerprint ||
		attempt.Stage.Result.ContainerIDFingerprint != intent.ContainerIDFingerprint ||
		attempt.Intent.ManifestFingerprint != intent.ManifestFingerprint ||
		attempt.Intent.MountBindingFingerprint != intent.MountBindingFingerprint ||
		attempt.Intent.InputArtifactDigest != intent.InputArtifactDigest ||
		attempt.Intent.AuthorityFingerprint != intent.AuthorityFingerprint ||
		attempt.Intent.SpecFingerprint != intent.SpecFingerprint ||
		attempt.Intent.PlanFingerprint != intent.PlanFingerprint ||
		attempt.Intent.RequestedBy != intent.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Docker host input staging does not match the current v56 attempt")
	}
	plan, err := getDockerContainerPlan(ctx, tx, intent.PlanID)
	if err != nil {
		return err
	}
	if err := validateDockerContainerPlanCurrentTx(ctx, tx, plan); err != nil {
		return err
	}
	if plan.RunID != intent.RunID || plan.MissionID != intent.MissionID ||
		plan.WorkspaceID != intent.WorkspaceID ||
		plan.ManifestFingerprint != intent.ManifestFingerprint ||
		plan.MountBindingFingerprint != intent.MountBindingFingerprint ||
		plan.InputArtifactDigest != intent.InputArtifactDigest ||
		plan.AuthorityFingerprint != intent.AuthorityFingerprint ||
		plan.SpecFingerprint != intent.SpecFingerprint ||
		plan.PlanFingerprint != intent.PlanFingerprint ||
		plan.ReadOnlyMountCount != intent.ReadOnlyMountCount ||
		plan.InputArtifactCount != intent.InputArtifactCount || plan.NetworkMode != "disabled" ||
		plan.EnvironmentCount != 0 || plan.SecretReferenceCount != 0 ||
		plan.RequestedBy != intent.RequestedBy || !plan.SimulationOnly ||
		plan.ProductionSubmitted || plan.ProductionVerified || plan.BackendAvailable ||
		plan.BackendEnabled || plan.ExecutionAuthorized || plan.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker host input staging plan authority changed")
	}
	return nil
}

func validateDockerHostInputStagingResultCurrentTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerHostInputStaging, intent sandbox.DockerHostInputStagingIntent,
	attempt sandbox.DockerContainerRehearsalAttempt,
) error {
	if attempt.Stage == nil || attempt.Completion != nil || value.IntentID != intent.ID ||
		value.AttemptID != intent.AttemptID || value.PlanID != intent.PlanID ||
		value.RunID != intent.RunID || value.AttemptIntentFingerprint != intent.AttemptIntentFingerprint ||
		value.ContainerIDFingerprint != intent.ContainerIDFingerprint ||
		value.InputArtifactDigest != intent.InputArtifactDigest ||
		value.AuthorityFingerprint != intent.AuthorityFingerprint ||
		value.ReadOnlyMountCount != intent.ReadOnlyMountCount ||
		value.InputArtifactCount != intent.InputArtifactCount ||
		value.LeaseGeneration < intent.PreparedGeneration {
		return apperror.New(apperror.CodeConflict,
			"Docker host input staging result authority changed")
	}
	return validateDockerHostInputStagingIntentPlanCurrentTx(ctx, tx, intent)
}

func validateDockerHostInputStagingIntentPlanCurrentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerHostInputStagingIntent,
) error {
	plan, err := getDockerContainerPlan(ctx, tx, intent.PlanID)
	if err != nil {
		return err
	}
	if err := validateDockerContainerPlanCurrentTx(ctx, tx, plan); err != nil {
		return err
	}
	if plan.PlanFingerprint != intent.PlanFingerprint ||
		plan.InputArtifactDigest != intent.InputArtifactDigest ||
		plan.MountBindingFingerprint != intent.MountBindingFingerprint ||
		plan.AuthorityFingerprint != intent.AuthorityFingerprint ||
		plan.SpecFingerprint != intent.SpecFingerprint || plan.RequestedBy != intent.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Docker host input staging plan changed before commit")
	}
	return nil
}

func appendDockerHostInputStagingEvent(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerHostInputStagingIntent, eventType, subjectID string,
	createdAt time.Time, payload map[string]any,
) error {
	event, err := events.New(intent.RunID, intent.MissionID, eventType,
		"sandbox_docker_host_input_staging", subjectID, payload)
	if err != nil {
		return err
	}
	event.CreatedAt = createdAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
