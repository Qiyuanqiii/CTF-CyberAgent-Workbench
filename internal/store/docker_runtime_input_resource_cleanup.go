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

const dockerRuntimeInputResourceCleanupIntentSelect = `SELECT id, inspection_id,
	application_intent_id, application_result_id, projection_id, container_plan_id, run_id,
	protocol_version, operation_key_digest, manifest_fingerprint, descriptor_fingerprint,
	request_fingerprint, inspection_fingerprint, application_result_fingerprint,
	endpoint_class, endpoint_fingerprint, projection_count, operator_confirmed,
	daemon_write_confirmed, container_start_authorized, process_execution_authorized,
	output_export_authorized, artifact_commit_authorized, intent_fingerprint, requested_by,
	created_at FROM sandbox_docker_runtime_input_resource_cleanup_intents`

func (s *SQLiteStore) BeginDockerRuntimeInputResourceCleanup(ctx context.Context,
	intent sandbox.DockerRuntimeInputResourceCleanupIntent, ownerID string, ttl time.Duration,
) (sandbox.DockerRuntimeInputResourceCleanupAcquisition, error) {
	if err := intent.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup intent is invalid", err)
	}
	ownerID = strings.TrimSpace(ownerID)
	if !validDockerRuntimeInputApplicationOwner(ownerID) {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup lease owner is invalid")
	}
	if err := sandbox.ValidateDockerRuntimeInputResourceCleanupLeaseTTL(ttl); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if existing, found, lookupErr := getDockerRuntimeInputResourceCleanupByOperation(ctx, tx,
		intent.OperationKeyDigest); lookupErr != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, lookupErr
	} else if found {
		if existing.Intent.IntentFingerprint != intent.IntentFingerprint ||
			existing.Intent.InspectionID != intent.InspectionID ||
			existing.Intent.RequestedBy != intent.RequestedBy {
			return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
				apperror.CodeConflict, "Docker runtime input resource cleanup operation key changed intent")
		}
		return acquireExistingDockerRuntimeInputResourceCleanup(ctx, tx, existing, ownerID, ttl)
	}
	if _, found, lookupErr := getDockerRuntimeInputResourceCleanupByApplication(ctx, tx,
		intent.ApplicationIntentID); lookupErr != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, lookupErr
	} else if found {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input application already has a cleanup intent")
	}
	inspection, application, err := validateDockerRuntimeInputResourceCleanupCurrentTx(ctx, tx, intent)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	now := time.Now().UTC()
	if intent.CreatedAt.After(now) {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup timestamp is in the future")
	}
	lease := sandbox.DockerRuntimeInputResourceCleanupLease{IntentID: intent.ID,
		LeaseID: newSandboxLeaseID(), OwnerID: ownerID, Generation: 1,
		Status:     sandbox.DockerRuntimeInputResourceCleanupLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(ttl)}
	if err := lease.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if err := insertDockerRuntimeInputResourceCleanupIntentTx(ctx, tx, intent); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_resource_cleanup_leases
		(intent_id, lease_id, owner_id, generation, status, acquired_at, expires_at, released_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`, lease.IntentID, lease.LeaseID, lease.OwnerID,
		lease.Generation, lease.Status, ts(lease.AcquiredAt), ts(lease.ExpiresAt)); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if err := appendDockerRuntimeInputResourceEvent(ctx, tx, intent.RunID,
		application.Intent.MissionID, intent.ID,
		events.SandboxDockerRuntimeInputResourceCleanupPreparedEvent, intent.CreatedAt,
		map[string]any{"inspection_id": inspection.ID, "lease_generation": lease.Generation,
			"projection_count": intent.ProjectionCount, "daemon_write_confirmed": true,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	record := sandbox.DockerRuntimeInputResourceCleanupRecord{Intent: intent, Lease: lease}
	return sandbox.DockerRuntimeInputResourceCleanupAcquisition{Record: record}, record.Validate()
}

func (s *SQLiteStore) AcquireDockerRuntimeInputResourceCleanup(ctx context.Context,
	intentID, requestedBy, ownerID string, ttl time.Duration,
) (sandbox.DockerRuntimeInputResourceCleanupAcquisition, error) {
	intentID, requestedBy, ownerID = strings.TrimSpace(intentID),
		strings.TrimSpace(requestedBy), strings.TrimSpace(ownerID)
	if !domain.ValidAgentID(intentID) || !domain.ValidAgentID(requestedBy) ||
		!validDockerRuntimeInputApplicationOwner(ownerID) {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup acquisition identity is invalid")
	}
	if err := sandbox.ValidateDockerRuntimeInputResourceCleanupLeaseTTL(ttl); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerRuntimeInputResourceCleanup(ctx, tx, intentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if record.Intent.RequestedBy != requestedBy {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input resource cleanup requester changed")
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	return acquireExistingDockerRuntimeInputResourceCleanup(ctx, tx, record, ownerID, ttl)
}

func acquireExistingDockerRuntimeInputResourceCleanup(ctx context.Context, tx *sql.Tx,
	record sandbox.DockerRuntimeInputResourceCleanupRecord, ownerID string, ttl time.Duration,
) (sandbox.DockerRuntimeInputResourceCleanupAcquisition, error) {
	if record.Result != nil {
		if err := tx.Commit(); err != nil {
			return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
		}
		record.Replayed = true
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{
			Record: record, Replayed: true,
		}, nil
	}
	if len(record.Failures) >= sandbox.MaxDockerRuntimeInputResourceCleanupFailures {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
			apperror.CodeResourceExhausted, "Docker runtime input resource cleanup failure ledger is exhausted")
	}
	now := time.Now().UTC()
	if record.Lease.ActiveAt(now) {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, apperror.New(
			apperror.CodeConflict, fmt.Sprintf("Docker runtime input resource cleanup is leased through %s",
				record.Lease.ExpiresAt.Format(time.RFC3339Nano)))
	}
	previous := record.Lease
	tookOver := previous.Status == sandbox.DockerRuntimeInputResourceCleanupLeaseActive
	next := sandbox.DockerRuntimeInputResourceCleanupLease{IntentID: record.Intent.ID,
		LeaseID: newSandboxLeaseID(), OwnerID: ownerID, Generation: previous.Generation + 1,
		Status:     sandbox.DockerRuntimeInputResourceCleanupLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(ttl)}
	if err := next.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_runtime_input_resource_cleanup_leases
		SET lease_id = ?, owner_id = ?, generation = ?, status = 'active', acquired_at = ?,
		expires_at = ?, released_at = NULL WHERE intent_id = ? AND lease_id = ?
		AND generation = ? AND status = ?`, next.LeaseID, next.OwnerID, next.Generation,
		ts(next.AcquiredAt), ts(next.ExpiresAt), previous.IntentID, previous.LeaseID,
		previous.Generation, previous.Status)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if err := requireSingleLeaseUpdate(result,
		"Docker runtime input resource cleanup lease changed before acquisition"); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	application, err := getDockerRuntimeInputApplication(ctx, tx, record.Intent.ApplicationIntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	eventType := events.SandboxDockerRuntimeInputResourceCleanupAcquiredEvent
	if tookOver {
		eventType = events.SandboxDockerRuntimeInputResourceCleanupTakenOverEvent
	}
	if err := appendDockerRuntimeInputResourceEvent(ctx, tx, record.Intent.RunID,
		application.Intent.MissionID, record.Intent.ID, eventType, now,
		map[string]any{"lease_generation": next.Generation,
			"previous_generation": previous.Generation, "took_over": tookOver,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	record.Lease, record.TookOver = next, tookOver
	if err := record.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupAcquisition{}, err
	}
	return sandbox.DockerRuntimeInputResourceCleanupAcquisition{
		Record: record, TookOver: tookOver,
	}, nil
}

func (s *SQLiteStore) CompleteDockerRuntimeInputResourceCleanup(ctx context.Context,
	result sandbox.DockerRuntimeInputResourceCleanupResult,
	expected sandbox.DockerRuntimeInputResourceCleanupLease,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, bool, error) {
	if result.Validate() != nil || expected.Validate() != nil ||
		result.IntentID != expected.IntentID || result.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerRuntimeInputResourceCleanupLeaseActive {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup completion binding is invalid")
	}
	now := time.Now().UTC()
	if result.CreatedAt.Before(expected.AcquiredAt) || !result.CreatedAt.Before(expected.ExpiresAt) ||
		result.CreatedAt.After(now) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup completion timestamp is outside the active lease")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerRuntimeInputResourceCleanup(ctx, tx, result.IntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	if record.Result != nil {
		if record.Result.ResultFingerprint != result.ResultFingerprint {
			return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, apperror.New(
				apperror.CodeConflict, "Docker runtime input resource cleanup result is already immutable")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
		}
		record.Replayed = true
		return record, true, nil
	}
	if err := requireCurrentDockerRuntimeInputResourceCleanupLease(record.Lease, expected, now); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	if result.InspectionID != record.Intent.InspectionID ||
		result.ApplicationIntentID != record.Intent.ApplicationIntentID ||
		result.ApplicationResultID != record.Intent.ApplicationResultID ||
		result.RunID != record.Intent.RunID || result.EndpointClass != record.Intent.EndpointClass ||
		result.EndpointFingerprint != record.Intent.EndpointFingerprint ||
		result.DescriptorFingerprint != record.Intent.DescriptorFingerprint ||
		result.RequestFingerprint != record.Intent.RequestFingerprint ||
		result.ApplicationResultFingerprint != record.Intent.ApplicationResultFingerprint ||
		result.ProjectionCount != record.Intent.ProjectionCount ||
		result.CreatedAt.Before(expected.AcquiredAt) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, apperror.New(
			apperror.CodeConflict, "Docker runtime input resource cleanup result changed durable intent")
	}
	if err := insertDockerRuntimeInputResourceCleanupResultTx(ctx, tx, result); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_runtime_input_resource_cleanup_leases
		SET status = 'released', released_at = ? WHERE intent_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(result.CreatedAt),
		expected.IntentID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	if err := requireSingleLeaseUpdate(update,
		"Docker runtime input resource cleanup lease changed before completion"); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	application, err := getDockerRuntimeInputApplication(ctx, tx, record.Intent.ApplicationIntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	if err := appendDockerRuntimeInputResourceEvent(ctx, tx, record.Intent.RunID,
		application.Intent.MissionID, record.Intent.ID,
		events.SandboxDockerRuntimeInputResourceCleanupCompletedEvent, result.CreatedAt,
		map[string]any{"lease_generation": result.LeaseGeneration,
			"total_resource_count":        result.TotalResourceCount,
			"delete_attempt_count":        result.DeleteAttemptCount,
			"final_absent_resource_count": result.FinalAbsentResourceCount,
			"container_started":           false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	releasedAt := result.CreatedAt
	record.Lease.Status, record.Lease.ReleasedAt =
		sandbox.DockerRuntimeInputResourceCleanupLeaseReleased, &releasedAt
	record.Result = &result
	return record, false, record.Validate()
}

func (s *SQLiteStore) RecordDockerRuntimeInputResourceCleanupFailure(ctx context.Context,
	intentID string, expected sandbox.DockerRuntimeInputResourceCleanupLease,
	code string, createdAt time.Time,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	intentID = strings.TrimSpace(intentID)
	if !domain.ValidAgentID(intentID) || expected.Validate() != nil ||
		expected.IntentID != intentID ||
		expected.Status != sandbox.DockerRuntimeInputResourceCleanupLeaseActive {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup failure binding is invalid")
	}
	createdAt = createdAt.UTC()
	now := time.Now().UTC()
	if createdAt.Before(expected.AcquiredAt) || !createdAt.Before(expected.ExpiresAt) ||
		createdAt.After(now) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup failure timestamp is outside the active lease")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerRuntimeInputResourceCleanup(ctx, tx, intentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if record.Result != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeConflict, "Completed Docker runtime input resource cleanup cannot fail")
	}
	if len(record.Failures) >= sandbox.MaxDockerRuntimeInputResourceCleanupFailures {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeResourceExhausted, "Docker runtime input resource cleanup failure ledger is exhausted")
	}
	if err := requireCurrentDockerRuntimeInputResourceCleanupLease(record.Lease, expected, now); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	failure, err := sandbox.NewDockerRuntimeInputResourceCleanupFailure(intentID,
		len(record.Failures)+1, expected.Generation, code, createdAt)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup failure is invalid", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sandbox_docker_runtime_input_resource_cleanup_failures
		(intent_id, sequence, generation, protocol_version, code, failure_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, failure.IntentID, failure.Sequence,
		failure.Generation, failure.ProtocolVersion, failure.Code,
		failure.FailureFingerprint, ts(failure.CreatedAt)); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_runtime_input_resource_cleanup_leases
		SET status = 'released', released_at = ? WHERE intent_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(failure.CreatedAt),
		expected.IntentID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if err := requireSingleLeaseUpdate(update,
		"Docker runtime input resource cleanup lease changed before failure recording"); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	application, err := getDockerRuntimeInputApplication(ctx, tx, record.Intent.ApplicationIntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if err := appendDockerRuntimeInputResourceEvent(ctx, tx, record.Intent.RunID,
		application.Intent.MissionID, record.Intent.ID,
		events.SandboxDockerRuntimeInputResourceCleanupFailedEvent, failure.CreatedAt,
		map[string]any{"lease_generation": failure.Generation, "failure_code": failure.Code,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	releasedAt := failure.CreatedAt
	record.Lease.Status, record.Lease.ReleasedAt =
		sandbox.DockerRuntimeInputResourceCleanupLeaseReleased, &releasedAt
	record.Failures = append(record.Failures, failure)
	return record, record.Validate()
}

func requireCurrentDockerRuntimeInputResourceCleanupLease(current,
	expected sandbox.DockerRuntimeInputResourceCleanupLease, now time.Time,
) error {
	if current.IntentID != expected.IntentID || current.LeaseID != expected.LeaseID ||
		current.OwnerID != expected.OwnerID || current.Generation != expected.Generation ||
		current.Status != expected.Status || !current.AcquiredAt.Equal(expected.AcquiredAt) ||
		!current.ExpiresAt.Equal(expected.ExpiresAt) || !current.ActiveAt(now) {
		return apperror.New(apperror.CodeConflict,
			"Docker runtime input resource cleanup lease expired or was replaced")
	}
	return nil
}

func validateDockerRuntimeInputResourceCleanupCurrentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerRuntimeInputResourceCleanupIntent,
) (sandbox.DockerRuntimeInputResourceInspection, sandbox.DockerRuntimeInputApplicationRecord, error) {
	inspection, err := getDockerRuntimeInputResourceInspection(ctx, tx, intent.InspectionID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{},
			sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	application, err := getDockerRuntimeInputApplication(ctx, tx, intent.ApplicationIntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{},
			sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if application.Result == nil || !inspection.CleanupEligible || inspection.ForeignResourceCount != 0 ||
		inspection.ApplicationIntentID != intent.ApplicationIntentID ||
		inspection.ApplicationResultID != intent.ApplicationResultID ||
		inspection.ProjectionID != intent.ProjectionID ||
		inspection.ContainerPlanID != intent.ContainerPlanID || inspection.RunID != intent.RunID ||
		inspection.ManifestFingerprint != intent.ManifestFingerprint ||
		inspection.DescriptorFingerprint != intent.DescriptorFingerprint ||
		inspection.RequestFingerprint != intent.RequestFingerprint ||
		inspection.InspectionFingerprint != intent.InspectionFingerprint ||
		inspection.ApplicationResultFingerprint != intent.ApplicationResultFingerprint ||
		inspection.EndpointClass != intent.EndpointClass ||
		inspection.EndpointFingerprint != intent.EndpointFingerprint ||
		inspection.ProjectionCount != intent.ProjectionCount ||
		inspection.RequestedBy != intent.RequestedBy || intent.CreatedAt.Before(inspection.CreatedAt) ||
		application.Result.ID != intent.ApplicationResultID ||
		application.Result.RequestFingerprint != intent.RequestFingerprint ||
		application.Result.ResultFingerprint != intent.ApplicationResultFingerprint ||
		application.Intent.RequestedBy != intent.RequestedBy {
		return sandbox.DockerRuntimeInputResourceInspection{},
			sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
				apperror.CodeConflict, "Docker runtime input resource cleanup inspection authority changed")
	}
	return inspection, application, nil
}

func insertDockerRuntimeInputResourceCleanupIntentTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerRuntimeInputResourceCleanupIntent,
) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO sandbox_docker_runtime_input_resource_cleanup_intents
		(id, inspection_id, application_intent_id, application_result_id, projection_id,
		container_plan_id, run_id, protocol_version, operation_key_digest, manifest_fingerprint,
		descriptor_fingerprint, request_fingerprint, inspection_fingerprint,
		application_result_fingerprint, endpoint_class, endpoint_fingerprint, projection_count,
		operator_confirmed, daemon_write_confirmed, container_start_authorized,
		process_execution_authorized, output_export_authorized, artifact_commit_authorized,
		intent_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.InspectionID, value.ApplicationIntentID, value.ApplicationResultID,
		value.ProjectionID, value.ContainerPlanID, value.RunID, value.ProtocolVersion,
		value.OperationKeyDigest, value.ManifestFingerprint, value.DescriptorFingerprint,
		value.RequestFingerprint, value.InspectionFingerprint,
		value.ApplicationResultFingerprint, value.EndpointClass, value.EndpointFingerprint,
		value.ProjectionCount, boolInt(value.OperatorConfirmed), boolInt(value.DaemonWriteConfirmed),
		boolInt(value.ContainerStartAuthorized), boolInt(value.ProcessExecutionAuthorized),
		boolInt(value.OutputExportAuthorized), boolInt(value.ArtifactCommitAuthorized),
		value.IntentFingerprint, value.RequestedBy, ts(value.CreatedAt))
	return err
}

func insertDockerRuntimeInputResourceCleanupResultTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerRuntimeInputResourceCleanupResult,
) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO sandbox_docker_runtime_input_resource_cleanup_results
		(id, intent_id, inspection_id, application_intent_id, application_result_id, run_id,
		protocol_version, status, trust_class, lease_generation, endpoint_class,
		endpoint_fingerprint, descriptor_fingerprint, request_fingerprint,
		application_result_fingerprint, projection_count, total_resource_count,
		initial_owned_resource_count, initial_absent_resource_count, delete_attempt_count,
		final_absent_resource_count, daemon_read_count, daemon_write_count, target_absent,
		all_volumes_absent, foreign_resource_detected, container_start_authorized,
		process_execution_authorized, output_export_authorized, artifact_commit_authorized,
		result_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?)`, value.ID, value.IntentID, value.InspectionID,
		value.ApplicationIntentID, value.ApplicationResultID, value.RunID,
		value.ProtocolVersion, value.Status, value.TrustClass, value.LeaseGeneration,
		value.EndpointClass, value.EndpointFingerprint, value.DescriptorFingerprint,
		value.RequestFingerprint, value.ApplicationResultFingerprint, value.ProjectionCount,
		value.TotalResourceCount, value.InitialOwnedResourceCount,
		value.InitialAbsentResourceCount, value.DeleteAttemptCount,
		value.FinalAbsentResourceCount, value.DaemonReadCount, value.DaemonWriteCount,
		boolInt(value.TargetAbsent), boolInt(value.AllVolumesAbsent),
		boolInt(value.ForeignResourceDetected), boolInt(value.ContainerStartAuthorized),
		boolInt(value.ProcessExecutionAuthorized), boolInt(value.OutputExportAuthorized),
		boolInt(value.ArtifactCommitAuthorized), value.ResultFingerprint, ts(value.CreatedAt))
	return err
}

func (s *SQLiteStore) GetDockerRuntimeInputResourceCleanup(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource cleanup id is invalid")
	}
	return getDockerRuntimeInputResourceCleanup(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerRuntimeInputResourceCleanupByOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker runtime input resource cleanup operation digest is invalid")
	}
	return getDockerRuntimeInputResourceCleanupByOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) ListDockerRuntimeInputResourceCleanups(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker runtime input resource cleanup list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker runtime input resource cleanup list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id
		FROM sandbox_docker_runtime_input_resource_cleanup_intents
		WHERE run_id = ? ORDER BY created_at, id LIMIT ?`, runID, limit)
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
	values := make([]sandbox.DockerRuntimeInputResourceCleanupRecord, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerRuntimeInputResourceCleanup(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerRuntimeInputResourceCleanup(ctx context.Context,
	queryer sandboxLifecycleQueryer, id string,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, error) {
	intent, err := scanDockerRuntimeInputResourceCleanupIntent(queryer.QueryRowContext(ctx,
		dockerRuntimeInputResourceCleanupIntentSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, apperror.New(
			apperror.CodeNotFound, "Docker runtime input resource cleanup not found")
	}
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	lease, err := scanDockerRuntimeInputResourceCleanupLease(queryer.QueryRowContext(ctx,
		`SELECT intent_id, lease_id, owner_id, generation, status, acquired_at, expires_at,
		released_at FROM sandbox_docker_runtime_input_resource_cleanup_leases
		WHERE intent_id = ?`, id))
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	failures, err := listDockerRuntimeInputResourceCleanupFailures(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	result, found, err := getDockerRuntimeInputResourceCleanupResult(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, err
	}
	record := sandbox.DockerRuntimeInputResourceCleanupRecord{Intent: intent, Lease: lease,
		Failures: failures}
	if found {
		record.Result = &result
	}
	if err := record.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, fmt.Errorf(
			"stored Docker runtime input resource cleanup is invalid: %w", err)
	}
	return record, nil
}

func getDockerRuntimeInputResourceCleanupByOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_runtime_input_resource_cleanup_intents
		WHERE operation_key_digest = ?`, keyDigest).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	value, err := getDockerRuntimeInputResourceCleanup(ctx, queryer, id)
	return value, err == nil, err
}

func getDockerRuntimeInputResourceCleanupByApplication(ctx context.Context,
	queryer sandboxLifecycleQueryer, applicationIntentID string,
) (sandbox.DockerRuntimeInputResourceCleanupRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_runtime_input_resource_cleanup_intents
		WHERE application_intent_id = ?`, applicationIntentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupRecord{}, false, err
	}
	value, err := getDockerRuntimeInputResourceCleanup(ctx, queryer, id)
	return value, err == nil, err
}

func scanDockerRuntimeInputResourceCleanupIntent(row scanner) (
	sandbox.DockerRuntimeInputResourceCleanupIntent, error,
) {
	var value sandbox.DockerRuntimeInputResourceCleanupIntent
	var operatorConfirmed, daemonConfirmed, startAuthorized, processAuthorized int
	var outputAuthorized, artifactAuthorized int
	var createdAt string
	if err := row.Scan(&value.ID, &value.InspectionID, &value.ApplicationIntentID,
		&value.ApplicationResultID, &value.ProjectionID, &value.ContainerPlanID,
		&value.RunID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.ManifestFingerprint, &value.DescriptorFingerprint,
		&value.RequestFingerprint, &value.InspectionFingerprint,
		&value.ApplicationResultFingerprint, &value.EndpointClass,
		&value.EndpointFingerprint, &value.ProjectionCount, &operatorConfirmed,
		&daemonConfirmed, &startAuthorized, &processAuthorized, &outputAuthorized,
		&artifactAuthorized, &value.IntentFingerprint, &value.RequestedBy,
		&createdAt); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupIntent{}, err
	}
	value.OperatorConfirmed, value.DaemonWriteConfirmed = operatorConfirmed != 0,
		daemonConfirmed != 0
	value.ContainerStartAuthorized, value.ProcessExecutionAuthorized = startAuthorized != 0,
		processAuthorized != 0
	value.OutputExportAuthorized, value.ArtifactCommitAuthorized = outputAuthorized != 0,
		artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupIntent{}, err
	}
	return value, nil
}

func scanDockerRuntimeInputResourceCleanupLease(row scanner) (
	sandbox.DockerRuntimeInputResourceCleanupLease, error,
) {
	var value sandbox.DockerRuntimeInputResourceCleanupLease
	var acquiredAt, expiresAt string
	var releasedAt sql.NullString
	if err := row.Scan(&value.IntentID, &value.LeaseID, &value.OwnerID, &value.Generation,
		&value.Status, &acquiredAt, &expiresAt, &releasedAt); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupLease{}, err
	}
	value.AcquiredAt, value.ExpiresAt = parseTS(acquiredAt), parseTS(expiresAt)
	if releasedAt.Valid {
		parsed := parseTS(releasedAt.String)
		value.ReleasedAt = &parsed
	}
	return value, value.Validate()
}

func listDockerRuntimeInputResourceCleanupFailures(ctx context.Context,
	queryer sandboxLifecycleQueryer, intentID string,
) ([]sandbox.DockerRuntimeInputResourceCleanupFailure, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT intent_id, sequence, generation,
		protocol_version, code, failure_fingerprint, created_at
		FROM sandbox_docker_runtime_input_resource_cleanup_failures
		WHERE intent_id = ? ORDER BY sequence`, intentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sandbox.DockerRuntimeInputResourceCleanupFailure, 0,
		sandbox.MaxDockerRuntimeInputResourceCleanupFailures)
	for rows.Next() {
		var value sandbox.DockerRuntimeInputResourceCleanupFailure
		var createdAt string
		if err := rows.Scan(&value.IntentID, &value.Sequence, &value.Generation,
			&value.ProtocolVersion, &value.Code, &value.FailureFingerprint,
			&createdAt); err != nil {
			return nil, err
		}
		value.CreatedAt = parseTS(createdAt)
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func getDockerRuntimeInputResourceCleanupResult(ctx context.Context,
	queryer sandboxLifecycleQueryer, intentID string,
) (sandbox.DockerRuntimeInputResourceCleanupResult, bool, error) {
	var value sandbox.DockerRuntimeInputResourceCleanupResult
	var targetAbsent, volumesAbsent, foreignDetected int
	var startAuthorized, processAuthorized, outputAuthorized, artifactAuthorized int
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT id, intent_id, inspection_id,
		application_intent_id, application_result_id, run_id, protocol_version, status,
		trust_class, lease_generation, endpoint_class, endpoint_fingerprint,
		descriptor_fingerprint, request_fingerprint, application_result_fingerprint,
		projection_count, total_resource_count, initial_owned_resource_count,
		initial_absent_resource_count, delete_attempt_count, final_absent_resource_count,
		daemon_read_count, daemon_write_count, target_absent, all_volumes_absent,
		foreign_resource_detected, container_start_authorized, process_execution_authorized,
		output_export_authorized, artifact_commit_authorized, result_fingerprint, created_at
		FROM sandbox_docker_runtime_input_resource_cleanup_results WHERE intent_id = ?`,
		intentID).Scan(&value.ID, &value.IntentID, &value.InspectionID,
		&value.ApplicationIntentID, &value.ApplicationResultID, &value.RunID,
		&value.ProtocolVersion, &value.Status, &value.TrustClass, &value.LeaseGeneration,
		&value.EndpointClass, &value.EndpointFingerprint, &value.DescriptorFingerprint,
		&value.RequestFingerprint, &value.ApplicationResultFingerprint,
		&value.ProjectionCount, &value.TotalResourceCount,
		&value.InitialOwnedResourceCount, &value.InitialAbsentResourceCount,
		&value.DeleteAttemptCount, &value.FinalAbsentResourceCount,
		&value.DaemonReadCount, &value.DaemonWriteCount, &targetAbsent, &volumesAbsent,
		&foreignDetected, &startAuthorized, &processAuthorized, &outputAuthorized,
		&artifactAuthorized, &value.ResultFingerprint, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputResourceCleanupResult{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupResult{}, false, err
	}
	value.TargetAbsent, value.AllVolumesAbsent = targetAbsent != 0, volumesAbsent != 0
	value.ForeignResourceDetected = foreignDetected != 0
	value.ContainerStartAuthorized, value.ProcessExecutionAuthorized = startAuthorized != 0,
		processAuthorized != 0
	value.OutputExportAuthorized, value.ArtifactCommitAuthorized = outputAuthorized != 0,
		artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupResult{}, false, err
	}
	return value, true, nil
}
