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
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/sandbox"
)

const dockerRuntimeInputApplicationIntentSelect = `SELECT id, projection_id, handoff_id,
	handoff_intent_id, attempt_id, container_plan_id, run_id, mission_id, workspace_id,
	protocol_version, operation_key_digest, manifest_fingerprint, mount_binding_fingerprint,
	input_artifact_digest, authority_fingerprint, spec_fingerprint, container_plan_fingerprint,
	handoff_fingerprint, projection_set_fingerprint, projection_fingerprint, endpoint_class,
	endpoint_fingerprint, projection_count, read_only_mount_count, input_artifact_count,
	total_entry_count, total_content_bytes, total_projection_bytes, operator_confirmed,
	daemon_write_confirmed, container_start_authorized, process_execution_authorized,
	output_export_authorized, artifact_commit_authorized, intent_fingerprint, requested_by,
	created_at FROM sandbox_docker_runtime_input_application_intents`

func (s *SQLiteStore) BeginDockerRuntimeInputApplication(ctx context.Context,
	intent sandbox.DockerRuntimeInputApplicationIntent, ownerID string, ttl time.Duration,
) (sandbox.DockerRuntimeInputApplicationAcquisition, error) {
	if err := intent.Validate(); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input application intent is invalid", err)
	}
	ownerID = strings.TrimSpace(ownerID)
	if !validDockerRuntimeInputApplicationOwner(ownerID) {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application lease owner is invalid")
	}
	if err := sandbox.ValidateDockerRuntimeInputApplicationLeaseTTL(ttl); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if existing, found, lookupErr := getDockerRuntimeInputApplicationByOperation(ctx, tx,
		intent.OperationKeyDigest); lookupErr != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, lookupErr
	} else if found {
		if existing.Intent.IntentFingerprint != intent.IntentFingerprint ||
			existing.Intent.ProjectionID != intent.ProjectionID ||
			existing.Intent.RequestedBy != intent.RequestedBy {
			return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
				apperror.CodeConflict, "Docker runtime input application operation key changed intent")
		}
		return acquireExistingDockerRuntimeInputApplication(ctx, tx, existing, ownerID, ttl)
	}
	if _, found, lookupErr := getDockerRuntimeInputApplicationByProjection(ctx, tx,
		intent.ProjectionID); lookupErr != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, lookupErr
	} else if found {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input projection already has a different application intent")
	}
	if err := validateDockerRuntimeInputApplicationCurrentTx(ctx, tx, intent); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	now := time.Now().UTC()
	if intent.CreatedAt.After(now) {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application timestamp is in the future")
	}
	lease := sandbox.DockerRuntimeInputApplicationLease{IntentID: intent.ID,
		LeaseID: newSandboxLeaseID(), OwnerID: ownerID, Generation: 1,
		Status:     sandbox.DockerRuntimeInputApplicationLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(ttl)}
	if err := lease.Validate(); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if err := insertDockerRuntimeInputApplicationIntentTx(ctx, tx, intent); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_application_leases
		(intent_id, lease_id, owner_id, generation, status, acquired_at, expires_at, released_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`, lease.IntentID, lease.LeaseID, lease.OwnerID,
		lease.Generation, lease.Status, ts(lease.AcquiredAt), ts(lease.ExpiresAt)); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if err := appendDockerRuntimeInputApplicationEvent(ctx, tx, intent,
		events.SandboxDockerRuntimeInputApplicationPreparedEvent, intent.CreatedAt,
		map[string]any{"lease_generation": lease.Generation, "daemon_write_confirmed": true,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	record := sandbox.DockerRuntimeInputApplicationRecord{Intent: intent, Lease: lease}
	return sandbox.DockerRuntimeInputApplicationAcquisition{Record: record}, record.Validate()
}

func (s *SQLiteStore) AcquireDockerRuntimeInputApplication(ctx context.Context,
	intentID, requestedBy, ownerID string, ttl time.Duration,
) (sandbox.DockerRuntimeInputApplicationAcquisition, error) {
	intentID, requestedBy, ownerID = strings.TrimSpace(intentID),
		strings.TrimSpace(requestedBy), strings.TrimSpace(ownerID)
	if !domain.ValidAgentID(intentID) || !domain.ValidAgentID(requestedBy) ||
		!validDockerRuntimeInputApplicationOwner(ownerID) {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application acquisition identity is invalid")
	}
	if err := sandbox.ValidateDockerRuntimeInputApplicationLeaseTTL(ttl); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerRuntimeInputApplication(ctx, tx, intentID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if record.Intent.RequestedBy != requestedBy {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input application requester changed")
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	return acquireExistingDockerRuntimeInputApplication(ctx, tx, record, ownerID, ttl)
}

func acquireExistingDockerRuntimeInputApplication(ctx context.Context, tx *sql.Tx,
	record sandbox.DockerRuntimeInputApplicationRecord, ownerID string, ttl time.Duration,
) (sandbox.DockerRuntimeInputApplicationAcquisition, error) {
	if record.Result != nil {
		if err := tx.Commit(); err != nil {
			return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
		}
		record.Replayed = true
		return sandbox.DockerRuntimeInputApplicationAcquisition{Record: record, Replayed: true}, nil
	}
	if len(record.Failures) >= sandbox.MaxDockerRuntimeInputApplicationFailures {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
			apperror.CodeResourceExhausted, "Docker runtime input application failure ledger is exhausted")
	}
	now := time.Now().UTC()
	if record.Lease.ActiveAt(now) {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, apperror.New(
			apperror.CodeConflict, fmt.Sprintf("Docker runtime input application is leased through %s",
				record.Lease.ExpiresAt.Format(time.RFC3339Nano)))
	}
	previous := record.Lease
	tookOver := previous.Status == sandbox.DockerRuntimeInputApplicationLeaseActive
	next := sandbox.DockerRuntimeInputApplicationLease{IntentID: record.Intent.ID,
		LeaseID: newSandboxLeaseID(), OwnerID: ownerID, Generation: previous.Generation + 1,
		Status:     sandbox.DockerRuntimeInputApplicationLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(ttl)}
	if err := next.Validate(); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_runtime_input_application_leases
		SET lease_id = ?, owner_id = ?, generation = ?, status = 'active', acquired_at = ?,
		expires_at = ?, released_at = NULL WHERE intent_id = ? AND lease_id = ?
		AND generation = ? AND status = ?`, next.LeaseID, next.OwnerID, next.Generation,
		ts(next.AcquiredAt), ts(next.ExpiresAt), previous.IntentID, previous.LeaseID,
		previous.Generation, previous.Status)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if err := requireSingleLeaseUpdate(result,
		"Docker runtime input application lease changed before acquisition"); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	eventType := events.SandboxDockerRuntimeInputApplicationAcquiredEvent
	if tookOver {
		eventType = events.SandboxDockerRuntimeInputApplicationTakenOverEvent
	}
	if err := appendDockerRuntimeInputApplicationEvent(ctx, tx, record.Intent, eventType, now,
		map[string]any{"lease_generation": next.Generation,
			"previous_generation": previous.Generation, "took_over": tookOver,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	record.Lease, record.TookOver = next, tookOver
	if err := record.Validate(); err != nil {
		return sandbox.DockerRuntimeInputApplicationAcquisition{}, err
	}
	return sandbox.DockerRuntimeInputApplicationAcquisition{Record: record, TookOver: tookOver}, nil
}

func validDockerRuntimeInputApplicationOwner(value string) bool {
	return domain.ValidAgentID(value) && !strings.ContainsRune(value, 0) && redact.String(value) == value
}

func (s *SQLiteStore) CompleteDockerRuntimeInputApplication(ctx context.Context,
	result sandbox.DockerRuntimeInputApplicationResult,
	expected sandbox.DockerRuntimeInputApplicationLease,
) (sandbox.DockerRuntimeInputApplicationRecord, bool, error) {
	if result.Validate() != nil || expected.Validate() != nil ||
		result.IntentID != expected.IntentID || result.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerRuntimeInputApplicationLeaseActive {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application completion binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerRuntimeInputApplication(ctx, tx, result.IntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	if record.Result != nil {
		if record.Result.ResultFingerprint != result.ResultFingerprint {
			return sandbox.DockerRuntimeInputApplicationRecord{}, false, apperror.New(
				apperror.CodeConflict, "Docker runtime input application result is already immutable")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
		}
		record.Replayed = true
		return record, true, nil
	}
	if err := requireCurrentDockerRuntimeInputApplicationLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	if result.ProjectionID != record.Intent.ProjectionID ||
		result.ContainerPlanID != record.Intent.ContainerPlanID ||
		result.RunID != record.Intent.RunID ||
		result.EndpointClass != record.Intent.EndpointClass ||
		result.EndpointFingerprint != record.Intent.EndpointFingerprint ||
		result.ProjectionFingerprint != record.Intent.ProjectionFingerprint ||
		result.ProjectionCount != record.Intent.ProjectionCount ||
		result.CreatedAt.Before(expected.AcquiredAt) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, apperror.New(
			apperror.CodeConflict, "Docker runtime input application result changed durable intent")
	}
	if err := insertDockerRuntimeInputApplicationResultTx(ctx, tx, result); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_runtime_input_application_leases
		SET status = 'released', released_at = ? WHERE intent_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(result.CreatedAt),
		expected.IntentID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	if err := requireSingleLeaseUpdate(update,
		"Docker runtime input application lease changed before completion"); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	if err := appendDockerRuntimeInputApplicationEvent(ctx, tx, record.Intent,
		events.SandboxDockerRuntimeInputApplicationCompletedEvent, result.CreatedAt,
		map[string]any{"lease_generation": result.LeaseGeneration,
			"projection_count": result.ProjectionCount, "target_container_present": true,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	releasedAt := result.CreatedAt
	record.Lease.Status, record.Lease.ReleasedAt = sandbox.DockerRuntimeInputApplicationLeaseReleased,
		&releasedAt
	record.Result = &result
	return record, false, record.Validate()
}

func (s *SQLiteStore) RecordDockerRuntimeInputApplicationFailure(ctx context.Context,
	intentID string, expected sandbox.DockerRuntimeInputApplicationLease,
	code string, createdAt time.Time,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	intentID = strings.TrimSpace(intentID)
	if !domain.ValidAgentID(intentID) || expected.Validate() != nil ||
		expected.IntentID != intentID || expected.Status != sandbox.DockerRuntimeInputApplicationLeaseActive {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application failure binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerRuntimeInputApplication(ctx, tx, intentID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Intent.RunID); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if record.Result != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeConflict, "Completed Docker runtime input application cannot fail")
	}
	if len(record.Failures) >= sandbox.MaxDockerRuntimeInputApplicationFailures {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeResourceExhausted, "Docker runtime input application failure ledger is exhausted")
	}
	if err := requireCurrentDockerRuntimeInputApplicationLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	failure, err := sandbox.NewDockerRuntimeInputApplicationFailure(intentID,
		len(record.Failures)+1, expected.Generation, code, createdAt)
	if err != nil || createdAt.Before(expected.AcquiredAt) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input application failure is invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_application_failures
		(intent_id, sequence, generation, protocol_version, code, failure_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, failure.IntentID, failure.Sequence,
		failure.Generation, failure.ProtocolVersion, failure.Code,
		failure.FailureFingerprint, ts(failure.CreatedAt)); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_runtime_input_application_leases
		SET status = 'released', released_at = ? WHERE intent_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(failure.CreatedAt),
		expected.IntentID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if err := requireSingleLeaseUpdate(update,
		"Docker runtime input application lease changed before failure recording"); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if err := appendDockerRuntimeInputApplicationEvent(ctx, tx, record.Intent,
		events.SandboxDockerRuntimeInputApplicationFailedEvent, failure.CreatedAt,
		map[string]any{"lease_generation": failure.Generation, "failure_code": failure.Code,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	releasedAt := failure.CreatedAt
	record.Lease.Status, record.Lease.ReleasedAt = sandbox.DockerRuntimeInputApplicationLeaseReleased,
		&releasedAt
	record.Failures = append(record.Failures, failure)
	return record, record.Validate()
}

func requireCurrentDockerRuntimeInputApplicationLease(current,
	expected sandbox.DockerRuntimeInputApplicationLease, now time.Time,
) error {
	if current.IntentID != expected.IntentID || current.LeaseID != expected.LeaseID ||
		current.OwnerID != expected.OwnerID || current.Generation != expected.Generation ||
		current.Status != expected.Status || !current.AcquiredAt.Equal(expected.AcquiredAt) ||
		!current.ExpiresAt.Equal(expected.ExpiresAt) || !current.ActiveAt(now) {
		return apperror.New(apperror.CodeConflict,
			"Docker runtime input application lease expired or was replaced")
	}
	return nil
}

func (s *SQLiteStore) GetDockerRuntimeInputApplication(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application id is invalid")
	}
	return getDockerRuntimeInputApplication(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerRuntimeInputApplicationByProjection(ctx context.Context,
	projectionID string,
) (sandbox.DockerRuntimeInputApplicationRecord, bool, error) {
	projectionID = strings.TrimSpace(projectionID)
	if !domain.ValidAgentID(projectionID) || strings.ContainsRune(projectionID, 0) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application projection id is invalid")
	}
	return getDockerRuntimeInputApplicationByProjection(ctx, s.db, projectionID)
}

func (s *SQLiteStore) GetDockerRuntimeInputApplicationByOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerRuntimeInputApplicationRecord, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input application operation digest is invalid")
	}
	return getDockerRuntimeInputApplicationByOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) ListDockerRuntimeInputApplications(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputApplicationRecord, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker runtime input application list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker runtime input application list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id
		FROM sandbox_docker_runtime_input_application_intents
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
	values := make([]sandbox.DockerRuntimeInputApplicationRecord, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerRuntimeInputApplication(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func validateDockerRuntimeInputApplicationCurrentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerRuntimeInputApplicationIntent,
) error {
	plan, err := getDockerRuntimeInputProjection(ctx, tx, intent.ProjectionID)
	if err != nil {
		return err
	}
	if intent.HandoffID != plan.HandoffID || intent.HandoffIntentID != plan.HandoffIntentID ||
		intent.AttemptID != plan.AttemptID || intent.ContainerPlanID != plan.ContainerPlanID ||
		intent.RunID != plan.RunID || intent.MissionID != plan.MissionID ||
		intent.WorkspaceID != plan.WorkspaceID ||
		intent.ManifestFingerprint != plan.ManifestFingerprint ||
		intent.MountBindingFingerprint != plan.MountBindingFingerprint ||
		intent.InputArtifactDigest != plan.InputArtifactDigest ||
		intent.AuthorityFingerprint != plan.AuthorityFingerprint ||
		intent.SpecFingerprint != plan.SpecFingerprint ||
		intent.ContainerPlanFingerprint != plan.ContainerPlanFingerprint ||
		intent.HandoffFingerprint != plan.HandoffFingerprint ||
		intent.ProjectionSetFingerprint != plan.ProjectionSetFingerprint ||
		intent.ProjectionFingerprint != plan.ProjectionFingerprint ||
		intent.ProjectionCount != plan.ProjectionCount ||
		intent.ReadOnlyMountCount != plan.ReadOnlyMountCount ||
		intent.InputArtifactCount != plan.InputArtifactCount ||
		intent.TotalEntryCount != plan.TotalEntryCount ||
		intent.TotalContentBytes != plan.TotalContentBytes ||
		intent.TotalProjectionBytes != plan.TotalProjectionBytes ||
		intent.RequestedBy != plan.RequestedBy || intent.CreatedAt.Before(plan.CreatedAt) ||
		plan.DaemonContacted || plan.DaemonApplied || plan.ContainerStarted ||
		plan.ProcessExecuted || plan.OutputExported || plan.ProductionExecutionSubmitted ||
		plan.ProductionVerified || plan.BackendEnabled || plan.ExecutionAuthorized ||
		plan.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker runtime input application v60 projection authority changed")
	}
	return nil
}

func insertDockerRuntimeInputApplicationIntentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerRuntimeInputApplicationIntent,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_application_intents
		(id, projection_id, handoff_id, handoff_intent_id, attempt_id, container_plan_id,
		run_id, mission_id, workspace_id, protocol_version, operation_key_digest,
		manifest_fingerprint, mount_binding_fingerprint, input_artifact_digest,
		authority_fingerprint, spec_fingerprint, container_plan_fingerprint,
		handoff_fingerprint, projection_set_fingerprint, projection_fingerprint,
		endpoint_class, endpoint_fingerprint, projection_count, read_only_mount_count,
		input_artifact_count, total_entry_count, total_content_bytes, total_projection_bytes,
		operator_confirmed, daemon_write_confirmed, container_start_authorized,
		process_execution_authorized, output_export_authorized, artifact_commit_authorized,
		intent_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, intent.ID, intent.ProjectionID,
		intent.HandoffID, intent.HandoffIntentID, intent.AttemptID, intent.ContainerPlanID,
		intent.RunID, intent.MissionID, intent.WorkspaceID, intent.ProtocolVersion,
		intent.OperationKeyDigest, intent.ManifestFingerprint, intent.MountBindingFingerprint,
		intent.InputArtifactDigest, intent.AuthorityFingerprint, intent.SpecFingerprint,
		intent.ContainerPlanFingerprint, intent.HandoffFingerprint,
		intent.ProjectionSetFingerprint, intent.ProjectionFingerprint, intent.EndpointClass,
		intent.EndpointFingerprint, intent.ProjectionCount, intent.ReadOnlyMountCount,
		intent.InputArtifactCount, intent.TotalEntryCount, intent.TotalContentBytes,
		intent.TotalProjectionBytes, boolInt(intent.OperatorConfirmed),
		boolInt(intent.DaemonWriteConfirmed), boolInt(intent.ContainerStartAuthorized),
		boolInt(intent.ProcessExecutionAuthorized), boolInt(intent.OutputExportAuthorized),
		boolInt(intent.ArtifactCommitAuthorized), intent.IntentFingerprint,
		intent.RequestedBy, ts(intent.CreatedAt))
	return err
}

func insertDockerRuntimeInputApplicationResultTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerRuntimeInputApplicationResult,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_application_results
		(id, intent_id, projection_id, container_plan_id, run_id, protocol_version, source,
		status, trust_class, lease_generation, endpoint_class, endpoint_fingerprint,
		request_fingerprint, projection_fingerprint, target_container_fingerprint,
		target_inspection_fingerprint, transport_fingerprint, projection_count,
		volume_created_count, volume_present_count, carrier_created_count,
		carrier_removed_count, readback_verified_count, daemon_read_count, daemon_write_count,
		reconciled_resource_count, all_volumes_read_only, all_volumes_no_copy,
		all_projection_bytes_verified, target_configuration_matched, target_container_present,
		container_started, process_executed, output_exported, production_execution_submitted,
		production_verified, backend_enabled, execution_authorized, artifact_commit_authorized,
		result_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.IntentID,
		value.ProjectionID, value.ContainerPlanID, value.RunID, value.ProtocolVersion,
		value.Source, value.Status, value.TrustClass, value.LeaseGeneration,
		value.EndpointClass, value.EndpointFingerprint, value.RequestFingerprint,
		value.ProjectionFingerprint, value.TargetContainerFingerprint,
		value.TargetInspectionFingerprint, value.TransportFingerprint, value.ProjectionCount,
		value.VolumeCreatedCount, value.VolumePresentCount, value.CarrierCreatedCount,
		value.CarrierRemovedCount, value.ReadbackVerifiedCount, value.DaemonReadCount,
		value.DaemonWriteCount, value.ReconciledResourceCount,
		boolInt(value.AllVolumesReadOnly), boolInt(value.AllVolumesNoCopy),
		boolInt(value.AllProjectionBytesVerified), boolInt(value.TargetConfigurationMatched),
		boolInt(value.TargetContainerPresent), boolInt(value.ContainerStarted),
		boolInt(value.ProcessExecuted), boolInt(value.OutputExported),
		boolInt(value.ProductionExecutionSubmitted), boolInt(value.ProductionVerified),
		boolInt(value.BackendEnabled), boolInt(value.ExecutionAuthorized),
		boolInt(value.ArtifactCommitAuthorized), value.ResultFingerprint, ts(value.CreatedAt))
	return err
}

func getDockerRuntimeInputApplication(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	intent, err := scanDockerRuntimeInputApplicationIntent(queryer.QueryRowContext(ctx,
		dockerRuntimeInputApplicationIntentSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeNotFound, "Docker runtime input application not found")
	}
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	lease, err := scanDockerRuntimeInputApplicationLease(queryer.QueryRowContext(ctx,
		`SELECT intent_id, lease_id, owner_id, generation, status, acquired_at,
		expires_at, released_at FROM sandbox_docker_runtime_input_application_leases
		WHERE intent_id = ?`, id))
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	failures, err := listDockerRuntimeInputApplicationFailures(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	result, found, err := getDockerRuntimeInputApplicationResult(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	record := sandbox.DockerRuntimeInputApplicationRecord{Intent: intent, Lease: lease,
		Failures: failures}
	if found {
		record.Result = &result
	}
	if err := record.Validate(); err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, fmt.Errorf(
			"stored Docker runtime input application is invalid: %w", err)
	}
	return record, nil
}

func getDockerRuntimeInputApplicationByProjection(ctx context.Context,
	queryer sandboxLifecycleQueryer, projectionID string,
) (sandbox.DockerRuntimeInputApplicationRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_runtime_input_application_intents WHERE projection_id = ?`,
		projectionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	value, err := getDockerRuntimeInputApplication(ctx, queryer, id)
	return value, true, err
}

func getDockerRuntimeInputApplicationByOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerRuntimeInputApplicationRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_runtime_input_application_intents WHERE operation_key_digest = ?`,
		keyDigest).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, false, err
	}
	value, err := getDockerRuntimeInputApplication(ctx, queryer, id)
	return value, true, err
}

func scanDockerRuntimeInputApplicationIntent(row scanner) (
	sandbox.DockerRuntimeInputApplicationIntent, error,
) {
	var value sandbox.DockerRuntimeInputApplicationIntent
	var operatorConfirmed, daemonConfirmed, startAuthorized, processAuthorized int
	var outputAuthorized, artifactAuthorized int
	var createdAt string
	if err := row.Scan(&value.ID, &value.ProjectionID, &value.HandoffID,
		&value.HandoffIntentID, &value.AttemptID, &value.ContainerPlanID, &value.RunID,
		&value.MissionID, &value.WorkspaceID, &value.ProtocolVersion,
		&value.OperationKeyDigest, &value.ManifestFingerprint,
		&value.MountBindingFingerprint, &value.InputArtifactDigest,
		&value.AuthorityFingerprint, &value.SpecFingerprint,
		&value.ContainerPlanFingerprint, &value.HandoffFingerprint,
		&value.ProjectionSetFingerprint, &value.ProjectionFingerprint,
		&value.EndpointClass, &value.EndpointFingerprint, &value.ProjectionCount,
		&value.ReadOnlyMountCount, &value.InputArtifactCount, &value.TotalEntryCount,
		&value.TotalContentBytes, &value.TotalProjectionBytes, &operatorConfirmed,
		&daemonConfirmed, &startAuthorized, &processAuthorized, &outputAuthorized,
		&artifactAuthorized, &value.IntentFingerprint, &value.RequestedBy,
		&createdAt); err != nil {
		return sandbox.DockerRuntimeInputApplicationIntent{}, err
	}
	value.OperatorConfirmed, value.DaemonWriteConfirmed = operatorConfirmed != 0,
		daemonConfirmed != 0
	value.ContainerStartAuthorized, value.ProcessExecutionAuthorized = startAuthorized != 0,
		processAuthorized != 0
	value.OutputExportAuthorized, value.ArtifactCommitAuthorized = outputAuthorized != 0,
		artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerRuntimeInputApplicationIntent{}, err
	}
	return value, nil
}

func scanDockerRuntimeInputApplicationLease(row scanner) (
	sandbox.DockerRuntimeInputApplicationLease, error,
) {
	var value sandbox.DockerRuntimeInputApplicationLease
	var acquiredAt, expiresAt string
	var releasedAt sql.NullString
	if err := row.Scan(&value.IntentID, &value.LeaseID, &value.OwnerID, &value.Generation,
		&value.Status, &acquiredAt, &expiresAt, &releasedAt); err != nil {
		return sandbox.DockerRuntimeInputApplicationLease{}, err
	}
	value.AcquiredAt, value.ExpiresAt = parseTS(acquiredAt), parseTS(expiresAt)
	if releasedAt.Valid {
		parsed := parseTS(releasedAt.String)
		value.ReleasedAt = &parsed
	}
	return value, value.Validate()
}

func listDockerRuntimeInputApplicationFailures(ctx context.Context,
	queryer sandboxLifecycleQueryer, intentID string,
) ([]sandbox.DockerRuntimeInputApplicationFailure, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT intent_id, sequence, generation,
		protocol_version, code, failure_fingerprint, created_at
		FROM sandbox_docker_runtime_input_application_failures
		WHERE intent_id = ? ORDER BY sequence`, intentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sandbox.DockerRuntimeInputApplicationFailure, 0,
		sandbox.MaxDockerRuntimeInputApplicationFailures)
	for rows.Next() {
		var value sandbox.DockerRuntimeInputApplicationFailure
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

func getDockerRuntimeInputApplicationResult(ctx context.Context,
	queryer sandboxLifecycleQueryer, intentID string,
) (sandbox.DockerRuntimeInputApplicationResult, bool, error) {
	var value sandbox.DockerRuntimeInputApplicationResult
	var volumesReadOnly, volumesNoCopy, projectionVerified, configMatched, targetPresent int
	var started, executed, exported, submitted, productionVerified int
	var backendEnabled, executionAuthorized, artifactAuthorized int
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT id, intent_id, projection_id,
		container_plan_id, run_id, protocol_version, source, status, trust_class,
		lease_generation, endpoint_class, endpoint_fingerprint, request_fingerprint,
		projection_fingerprint, target_container_fingerprint, target_inspection_fingerprint,
		transport_fingerprint, projection_count, volume_created_count, volume_present_count,
		carrier_created_count, carrier_removed_count, readback_verified_count,
		daemon_read_count, daemon_write_count, reconciled_resource_count,
		all_volumes_read_only, all_volumes_no_copy, all_projection_bytes_verified,
		target_configuration_matched, target_container_present, container_started,
		process_executed, output_exported, production_execution_submitted,
		production_verified, backend_enabled, execution_authorized,
		artifact_commit_authorized, result_fingerprint, created_at
		FROM sandbox_docker_runtime_input_application_results WHERE intent_id = ?`,
		intentID).Scan(&value.ID, &value.IntentID, &value.ProjectionID,
		&value.ContainerPlanID, &value.RunID, &value.ProtocolVersion, &value.Source,
		&value.Status, &value.TrustClass, &value.LeaseGeneration, &value.EndpointClass,
		&value.EndpointFingerprint, &value.RequestFingerprint,
		&value.ProjectionFingerprint, &value.TargetContainerFingerprint,
		&value.TargetInspectionFingerprint, &value.TransportFingerprint,
		&value.ProjectionCount, &value.VolumeCreatedCount, &value.VolumePresentCount,
		&value.CarrierCreatedCount, &value.CarrierRemovedCount,
		&value.ReadbackVerifiedCount, &value.DaemonReadCount, &value.DaemonWriteCount,
		&value.ReconciledResourceCount, &volumesReadOnly, &volumesNoCopy,
		&projectionVerified, &configMatched, &targetPresent, &started, &executed,
		&exported, &submitted, &productionVerified, &backendEnabled,
		&executionAuthorized, &artifactAuthorized, &value.ResultFingerprint,
		&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputApplicationResult{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationResult{}, false, err
	}
	value.AllVolumesReadOnly, value.AllVolumesNoCopy = volumesReadOnly != 0, volumesNoCopy != 0
	value.AllProjectionBytesVerified = projectionVerified != 0
	value.TargetConfigurationMatched, value.TargetContainerPresent = configMatched != 0,
		targetPresent != 0
	value.ContainerStarted, value.ProcessExecuted = started != 0, executed != 0
	value.OutputExported, value.ProductionExecutionSubmitted = exported != 0, submitted != 0
	value.ProductionVerified, value.BackendEnabled = productionVerified != 0, backendEnabled != 0
	value.ExecutionAuthorized, value.ArtifactCommitAuthorized = executionAuthorized != 0,
		artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerRuntimeInputApplicationResult{}, false, err
	}
	return value, true, nil
}

func appendDockerRuntimeInputApplicationEvent(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerRuntimeInputApplicationIntent, eventType string, createdAt time.Time,
	payload map[string]any,
) error {
	event, err := events.New(intent.RunID, intent.MissionID, eventType,
		"sandbox_docker_runtime_input_application", intent.ID, payload)
	if err != nil {
		return err
	}
	event.CreatedAt = createdAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
