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

const dockerHostInputHandoffIntentSelect = `SELECT id, attempt_id, staging_intent_id,
	staging_id, plan_id, run_id, mission_id, workspace_id, protocol_version,
	operation_key_digest, attempt_intent_fingerprint, container_id_fingerprint,
	capture_requirement_fingerprint, handoff_requirement_fingerprint,
	staging_fingerprint, bundle_report_fingerprint, bundle_digest, bundle_bytes,
	authority_fingerprint, spec_fingerprint, plan_fingerprint, prepared_generation,
	intent_fingerprint, requested_by, created_at
	FROM sandbox_docker_host_input_handoff_intents`

const dockerHostInputHandoffSelect = `SELECT id, intent_id, attempt_id, plan_id, run_id,
	protocol_version, lease_generation, handoff_fingerprint, source, trust_class,
	status, endpoint_class, endpoint_fingerprint, request_fingerprint,
	intent_fingerprint, bundle_report_fingerprint, bundle_digest, readback_digest,
	carrier_name_fingerprint, volume_name_fingerprint,
	final_container_id_fingerprint, transport_fingerprint, daemon_read_count,
	daemon_write_count, reconciled_resource_count, daemon_consumed,
	readback_verified, final_mount_read_only, carrier_removed,
	final_container_removed, volume_removed, cleanup_confirmed, container_started,
	process_executed, output_exported, raw_content_retained,
	production_execution_submitted, production_verified, backend_enabled,
	execution_authorized, artifact_commit_authorized, created_at
	FROM sandbox_docker_host_input_handoffs`

func (s *SQLiteStore) PrepareDockerHostInputHandoffIntent(ctx context.Context,
	intent sandbox.DockerHostInputHandoffIntent, expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerHostInputHandoffRecord, bool, error) {
	if intent.Validate() != nil || expected.Validate() != nil ||
		intent.AttemptID != expected.AttemptID || intent.PreparedGeneration != expected.Generation ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff intent binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, intent.AttemptID)
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.Intent.RunID); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if existing, found, err := getDockerHostInputHandoffByOperation(ctx, tx,
		intent.OperationKeyDigest); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	} else if found {
		if existing.Intent.IntentFingerprint != intent.IntentFingerprint ||
			existing.Intent.AttemptID != intent.AttemptID ||
			existing.Intent.RequestedBy != intent.RequestedBy {
			return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
				apperror.CodeConflict, "Docker host input handoff operation changed")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerHostInputHandoffRecord{}, false, err
		}
		existing.Replayed = true
		return existing, true, nil
	}
	if _, found, err := getDockerHostInputHandoffByAttempt(ctx, tx,
		intent.AttemptID); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	} else if found {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeConflict, "Docker attempt already has a different host input handoff")
	}
	if err := validateDockerHostInputHandoffIntentCurrentTx(ctx, tx, intent, attempt); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if intent.CreatedAt.After(time.Now().UTC()) {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff intent timestamp is in the future")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_host_input_handoff_intents
		(id, attempt_id, staging_intent_id, staging_id, plan_id, run_id, mission_id,
		workspace_id, protocol_version, operation_key_digest, attempt_intent_fingerprint,
		container_id_fingerprint, capture_requirement_fingerprint,
		handoff_requirement_fingerprint, staging_fingerprint,
		bundle_report_fingerprint, bundle_digest, bundle_bytes, authority_fingerprint,
		spec_fingerprint, plan_fingerprint, prepared_generation, intent_fingerprint,
		requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		intent.ID, intent.AttemptID, intent.StagingIntentID, intent.StagingID,
		intent.PlanID, intent.RunID, intent.MissionID, intent.WorkspaceID,
		intent.ProtocolVersion, intent.OperationKeyDigest, intent.AttemptIntentFingerprint,
		intent.ContainerIDFingerprint, intent.CaptureRequirementFingerprint,
		intent.HandoffRequirementFingerprint, intent.StagingFingerprint,
		intent.BundleReportFingerprint, intent.BundleDigest, intent.BundleBytes,
		intent.AuthorityFingerprint, intent.SpecFingerprint, intent.PlanFingerprint,
		intent.PreparedGeneration, intent.IntentFingerprint, intent.RequestedBy,
		ts(intent.CreatedAt)); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := appendDockerHostInputHandoffEvent(ctx, tx, intent,
		events.SandboxDockerHostInputHandoffIntentEvent, intent.ID, intent.CreatedAt,
		map[string]any{"lease_generation": intent.PreparedGeneration,
			"bundle_bytes": intent.BundleBytes, "daemon_consumed": false,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	record := sandbox.DockerHostInputHandoffRecord{Intent: intent}
	return record, false, record.Validate()
}

func (s *SQLiteStore) RecordDockerHostInputHandoff(ctx context.Context,
	value sandbox.DockerHostInputHandoff, expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerHostInputHandoffRecord, bool, error) {
	if value.Validate() != nil || expected.Validate() != nil ||
		value.AttemptID != expected.AttemptID || value.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff result binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerHostInputHandoffRecord(ctx, tx, value.IntentID)
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, record.Intent.AttemptID)
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if record.Handoff != nil {
		if record.Handoff.HandoffFingerprint != value.HandoffFingerprint {
			return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
				apperror.CodeConflict, "Docker host input handoff result changed")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerHostInputHandoffRecord{}, false, err
		}
		record.Replayed = true
		return record, true, nil
	}
	if attempt.Cleanup != nil || attempt.Completion != nil ||
		value.IntentID != record.Intent.ID || value.AttemptID != record.Intent.AttemptID ||
		value.PlanID != record.Intent.PlanID || value.RunID != record.Intent.RunID ||
		value.Result.IntentFingerprint != record.Intent.IntentFingerprint ||
		value.Result.BundleReportFingerprint != record.Intent.BundleReportFingerprint ||
		value.Result.BundleDigest != record.Intent.BundleDigest ||
		value.LeaseGeneration < record.Intent.PreparedGeneration {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeConflict, "Docker host input handoff result authority changed")
	}
	if value.CreatedAt.After(time.Now().UTC()) || value.CreatedAt.Before(record.Intent.CreatedAt) {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff timestamp is invalid")
	}
	result := value.Result
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_host_input_handoffs
		(id, intent_id, attempt_id, plan_id, run_id, protocol_version, lease_generation,
		handoff_fingerprint, source, trust_class, status, endpoint_class,
		endpoint_fingerprint, request_fingerprint, intent_fingerprint,
		bundle_report_fingerprint, bundle_digest, readback_digest,
		carrier_name_fingerprint, volume_name_fingerprint,
		final_container_id_fingerprint, transport_fingerprint, daemon_read_count,
		daemon_write_count, reconciled_resource_count, daemon_consumed,
		readback_verified, final_mount_read_only, carrier_removed,
		final_container_removed, volume_removed, cleanup_confirmed, container_started,
		process_executed, output_exported, raw_content_retained,
		production_execution_submitted, production_verified, backend_enabled,
		execution_authorized, artifact_commit_authorized, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.IntentID, value.AttemptID, value.PlanID, value.RunID,
		value.ProtocolVersion, value.LeaseGeneration, value.HandoffFingerprint,
		result.Source, result.TrustClass, result.Status, result.EndpointClass,
		result.EndpointFingerprint, result.RequestFingerprint, result.IntentFingerprint,
		result.BundleReportFingerprint, result.BundleDigest, result.ReadbackDigest,
		result.CarrierNameFingerprint, result.VolumeNameFingerprint,
		result.FinalContainerIDFingerprint, result.TransportFingerprint,
		result.DaemonReadCount, result.DaemonWriteCount, result.ReconciledResourceCount,
		boolInt(result.DaemonConsumed), boolInt(result.ReadbackVerified),
		boolInt(result.FinalMountReadOnly), boolInt(result.CarrierRemoved),
		boolInt(result.FinalContainerRemoved), boolInt(result.VolumeRemoved),
		boolInt(result.CleanupConfirmed), boolInt(result.ContainerStarted),
		boolInt(result.ProcessExecuted), boolInt(result.OutputExported),
		boolInt(result.RawContentRetained), boolInt(result.ProductionExecutionSubmitted),
		boolInt(result.ProductionVerified), boolInt(result.BackendEnabled),
		boolInt(result.ExecutionAuthorized), boolInt(result.ArtifactCommitAuthorized),
		ts(value.CreatedAt)); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := appendDockerHostInputHandoffEvent(ctx, tx, record.Intent,
		events.SandboxDockerHostInputHandoffEvent, value.ID, value.CreatedAt,
		map[string]any{"status": result.Status, "lease_generation": value.LeaseGeneration,
			"daemon_reads": result.DaemonReadCount, "daemon_writes": result.DaemonWriteCount,
			"daemon_consumed": true, "readback_verified": true,
			"final_mount_read_only": true, "cleanup_confirmed": true,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	record.Handoff = &value
	return record, false, record.Validate()
}

func (s *SQLiteStore) GetDockerHostInputHandoff(ctx context.Context,
	id string,
) (sandbox.DockerHostInputHandoffRecord, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerHostInputHandoffRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff id is invalid")
	}
	return getDockerHostInputHandoffRecord(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerHostInputHandoffByAttempt(ctx context.Context,
	attemptID string,
) (sandbox.DockerHostInputHandoffRecord, bool, error) {
	attemptID = strings.TrimSpace(attemptID)
	if !domain.ValidAgentID(attemptID) || strings.ContainsRune(attemptID, 0) {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff attempt id is invalid")
	}
	return getDockerHostInputHandoffByAttempt(ctx, s.db, attemptID)
}

func (s *SQLiteStore) GetDockerHostInputHandoffByOperation(ctx context.Context,
	digest string,
) (sandbox.DockerHostInputHandoffRecord, bool, error) {
	digest = strings.TrimSpace(digest)
	if !validStoreDigest(digest) {
		return sandbox.DockerHostInputHandoffRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff operation digest is invalid")
	}
	return getDockerHostInputHandoffByOperation(ctx, s.db, digest)
}

func (s *SQLiteStore) ListDockerHostInputHandoffs(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerHostInputHandoffRecord, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || limit < 1 || limit > 1000 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker host input handoff list request is invalid")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_docker_host_input_handoff_intents
		WHERE run_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
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
	values := make([]sandbox.DockerHostInputHandoffRecord, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerHostInputHandoffRecord(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerHostInputHandoffByAttempt(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) (sandbox.DockerHostInputHandoffRecord, bool, error) {
	return getDockerHostInputHandoffByColumn(ctx, queryer, "attempt_id", attemptID)
}

func getDockerHostInputHandoffByOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	digest string,
) (sandbox.DockerHostInputHandoffRecord, bool, error) {
	return getDockerHostInputHandoffByColumn(ctx, queryer, "operation_key_digest", digest)
}

func getDockerHostInputHandoffByColumn(ctx context.Context, queryer sandboxLifecycleQueryer,
	column, value string,
) (sandbox.DockerHostInputHandoffRecord, bool, error) {
	if column != "attempt_id" && column != "operation_key_digest" {
		return sandbox.DockerHostInputHandoffRecord{}, false, errors.New("invalid handoff lookup")
	}
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id FROM sandbox_docker_host_input_handoff_intents
		WHERE `+column+` = ?`, value).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputHandoffRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, false, err
	}
	record, err := getDockerHostInputHandoffRecord(ctx, queryer, id)
	return record, err == nil, err
}

func getDockerHostInputHandoffRecord(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerHostInputHandoffRecord, error) {
	intent, err := scanDockerHostInputHandoffIntent(queryer.QueryRowContext(ctx,
		dockerHostInputHandoffIntentSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputHandoffRecord{}, apperror.New(
			apperror.CodeNotFound, "Docker host input handoff not found")
	}
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, err
	}
	handoff, found, err := getDockerHostInputHandoffResult(ctx, queryer, intent.ID)
	if err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, err
	}
	record := sandbox.DockerHostInputHandoffRecord{Intent: intent}
	if found {
		record.Handoff = &handoff
	}
	if err := record.Validate(); err != nil {
		return sandbox.DockerHostInputHandoffRecord{}, fmt.Errorf(
			"stored Docker host input handoff is invalid: %w", err)
	}
	return record, nil
}

func scanDockerHostInputHandoffIntent(row scanner) (sandbox.DockerHostInputHandoffIntent, error) {
	var value sandbox.DockerHostInputHandoffIntent
	var createdAt string
	err := row.Scan(&value.ID, &value.AttemptID, &value.StagingIntentID, &value.StagingID,
		&value.PlanID, &value.RunID, &value.MissionID, &value.WorkspaceID,
		&value.ProtocolVersion, &value.OperationKeyDigest, &value.AttemptIntentFingerprint,
		&value.ContainerIDFingerprint, &value.CaptureRequirementFingerprint,
		&value.HandoffRequirementFingerprint, &value.StagingFingerprint,
		&value.BundleReportFingerprint, &value.BundleDigest, &value.BundleBytes,
		&value.AuthorityFingerprint, &value.SpecFingerprint, &value.PlanFingerprint,
		&value.PreparedGeneration, &value.IntentFingerprint, &value.RequestedBy, &createdAt)
	value.CreatedAt = parseTS(createdAt)
	if err == nil {
		err = value.Validate()
	}
	return value, err
}

func getDockerHostInputHandoffResult(ctx context.Context, queryer sandboxLifecycleQueryer,
	intentID string,
) (sandbox.DockerHostInputHandoff, bool, error) {
	var value sandbox.DockerHostInputHandoff
	var daemonConsumed, readbackVerified, finalMountReadOnly, carrierRemoved int
	var finalRemoved, volumeRemoved, cleanupConfirmed, containerStarted int
	var processExecuted, outputExported, rawContentRetained, productionSubmitted int
	var productionVerified, backendEnabled, executionAuthorized, artifactAuthorized int
	var createdAt string
	err := queryer.QueryRowContext(ctx, dockerHostInputHandoffSelect+` WHERE intent_id = ?`,
		intentID).Scan(&value.ID, &value.IntentID, &value.AttemptID, &value.PlanID,
		&value.RunID, &value.ProtocolVersion, &value.LeaseGeneration,
		&value.HandoffFingerprint, &value.Result.Source, &value.Result.TrustClass,
		&value.Result.Status, &value.Result.EndpointClass, &value.Result.EndpointFingerprint,
		&value.Result.RequestFingerprint, &value.Result.IntentFingerprint,
		&value.Result.BundleReportFingerprint, &value.Result.BundleDigest,
		&value.Result.ReadbackDigest, &value.Result.CarrierNameFingerprint,
		&value.Result.VolumeNameFingerprint, &value.Result.FinalContainerIDFingerprint,
		&value.Result.TransportFingerprint, &value.Result.DaemonReadCount,
		&value.Result.DaemonWriteCount, &value.Result.ReconciledResourceCount,
		&daemonConsumed, &readbackVerified, &finalMountReadOnly, &carrierRemoved,
		&finalRemoved, &volumeRemoved, &cleanupConfirmed, &containerStarted,
		&processExecuted, &outputExported, &rawContentRetained, &productionSubmitted,
		&productionVerified, &backendEnabled, &executionAuthorized, &artifactAuthorized,
		&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputHandoff{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputHandoff{}, false, err
	}
	value.Result.ProtocolVersion = value.ProtocolVersion
	value.Result.DaemonConsumed = daemonConsumed != 0
	value.Result.ReadbackVerified = readbackVerified != 0
	value.Result.FinalMountReadOnly = finalMountReadOnly != 0
	value.Result.CarrierRemoved = carrierRemoved != 0
	value.Result.FinalContainerRemoved = finalRemoved != 0
	value.Result.VolumeRemoved = volumeRemoved != 0
	value.Result.CleanupConfirmed = cleanupConfirmed != 0
	value.Result.ContainerStarted = containerStarted != 0
	value.Result.ProcessExecuted = processExecuted != 0
	value.Result.OutputExported = outputExported != 0
	value.Result.RawContentRetained = rawContentRetained != 0
	value.Result.ProductionExecutionSubmitted = productionSubmitted != 0
	value.Result.ProductionVerified = productionVerified != 0
	value.Result.BackendEnabled = backendEnabled != 0
	value.Result.ExecutionAuthorized = executionAuthorized != 0
	value.Result.ArtifactCommitAuthorized = artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerHostInputHandoff{}, false, err
	}
	return value, true, nil
}

func validateDockerHostInputHandoffIntentCurrentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerHostInputHandoffIntent, attempt sandbox.DockerContainerRehearsalAttempt,
) error {
	if attempt.Stage == nil || attempt.Cleanup != nil || attempt.Completion != nil ||
		attempt.HostInputRequirement == nil || attempt.HostInputHandoffRequirement == nil ||
		!attempt.HostInputRequirement.Required || !attempt.HostInputHandoffRequirement.Required ||
		intent.AttemptID != attempt.Intent.ID || intent.PlanID != attempt.Intent.PlanID ||
		intent.RunID != attempt.Intent.RunID || intent.MissionID != attempt.Intent.MissionID ||
		intent.WorkspaceID != attempt.Intent.WorkspaceID ||
		intent.AttemptIntentFingerprint != attempt.Intent.IntentFingerprint ||
		intent.ContainerIDFingerprint != attempt.Stage.Result.ContainerIDFingerprint ||
		intent.CaptureRequirementFingerprint != attempt.HostInputRequirement.RequirementFingerprint ||
		intent.HandoffRequirementFingerprint != attempt.HostInputHandoffRequirement.RequirementFingerprint ||
		intent.AuthorityFingerprint != attempt.Intent.AuthorityFingerprint ||
		intent.SpecFingerprint != attempt.Intent.SpecFingerprint ||
		intent.PlanFingerprint != attempt.Intent.PlanFingerprint ||
		intent.RequestedBy != attempt.Intent.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Docker host input handoff does not match the current attempt")
	}
	staging, found, err := getDockerHostInputStagingByAttempt(ctx, tx, intent.AttemptID)
	if err != nil {
		return err
	}
	if !found || staging.Staging == nil || staging.Intent.ID != intent.StagingIntentID ||
		staging.Staging.ID != intent.StagingID ||
		staging.Staging.StagingFingerprint != intent.StagingFingerprint ||
		staging.Staging.Report.ReportFingerprint != intent.BundleReportFingerprint ||
		staging.Staging.Report.BundleDigest != intent.BundleDigest ||
		staging.Staging.Report.BundleBytes != intent.BundleBytes {
		return apperror.New(apperror.CodeConflict,
			"Docker host input handoff staging evidence changed")
	}
	plan, err := getDockerContainerPlan(ctx, tx, intent.PlanID)
	if err != nil {
		return err
	}
	if err := validateDockerContainerPlanCurrentTx(ctx, tx, plan); err != nil {
		return err
	}
	if plan.PlanFingerprint != intent.PlanFingerprint ||
		plan.AuthorityFingerprint != intent.AuthorityFingerprint ||
		plan.SpecFingerprint != intent.SpecFingerprint || plan.NetworkMode != "disabled" ||
		plan.EnvironmentCount != 0 || plan.SecretReferenceCount != 0 ||
		plan.RequestedBy != intent.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Docker host input handoff plan authority changed")
	}
	return nil
}

func appendDockerHostInputHandoffEvent(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerHostInputHandoffIntent, eventType, subjectID string,
	createdAt time.Time, payload map[string]any,
) error {
	event, err := events.New(intent.RunID, intent.MissionID, eventType,
		"sandbox_docker_host_input_handoff", subjectID, payload)
	if err != nil {
		return err
	}
	event.CreatedAt = createdAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
