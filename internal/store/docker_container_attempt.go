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

const dockerContainerAttemptIntentSelect = `SELECT id, plan_id, observation_id, evidence_id,
	output_simulation_id, preflight_id, execution_id, candidate_id, preparation_id,
	run_id, mission_id, workspace_id, protocol_version, operation_key_digest,
	manifest_fingerprint, authorization_fingerprint, policy_fingerprint,
	mount_binding_fingerprint, input_artifact_digest, threat_model_fingerprint,
	output_plan_fingerprint, observation_fingerprint, authority_fingerprint,
	spec_fingerprint, plan_fingerprint, image_digest, request_fingerprint,
	endpoint_class, endpoint_fingerprint, network_mode, environment_count,
	secret_reference_count, intent_fingerprint, requested_by, created_at
	FROM sandbox_docker_container_rehearsal_attempts`

func (s *SQLiteStore) BeginDockerContainerRehearsalAttempt(ctx context.Context,
	intent sandbox.DockerContainerAttemptIntent, ownerID string, ttl time.Duration,
) (sandbox.DockerContainerAttemptAcquisition, error) {
	if err := intent.Validate(); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker container attempt intent is invalid", err)
	}
	ownerID = strings.TrimSpace(ownerID)
	if !domain.ValidAgentID(ownerID) || strings.ContainsRune(ownerID, 0) ||
		redact.String(ownerID) != ownerID {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt lease owner is invalid")
	}
	if err := sandbox.ValidateDockerContainerAttemptLeaseTTL(ttl); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, intent.RunID); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	existing, found, err := getDockerContainerAttemptByOperation(ctx, tx,
		intent.OperationKeyDigest)
	if err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if found {
		if existing.Intent.IntentFingerprint != intent.IntentFingerprint ||
			existing.Intent.PlanID != intent.PlanID ||
			existing.Intent.RequestedBy != intent.RequestedBy {
			return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(
				apperror.CodeConflict, "Docker attempt operation key was used for different intent")
		}
		return acquireExistingDockerContainerAttempt(ctx, tx, existing, ownerID, ttl)
	}
	if existing, found, err = getDockerContainerAttemptByPlan(ctx, tx, intent.PlanID); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	} else if found {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker container plan already has a different durable attempt")
	}
	if err := validateDockerContainerAttemptIntentCurrentTx(ctx, tx, intent); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	now := time.Now().UTC()
	if intent.CreatedAt.After(now) {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt timestamp is in the future")
	}
	lease := sandbox.DockerContainerAttemptLease{AttemptID: intent.ID,
		LeaseID: newSandboxLeaseID(), OwnerID: ownerID, Generation: 1,
		Status: sandbox.DockerContainerAttemptLeaseActive, AcquiredAt: now,
		ExpiresAt: now.Add(ttl)}
	if err := lease.Validate(); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if err := insertDockerContainerAttemptIntentTx(ctx, tx, intent); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_attempt_leases
		(attempt_id, lease_id, owner_id, generation, status, acquired_at, expires_at,
		released_at) VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`, lease.AttemptID, lease.LeaseID,
		lease.OwnerID, lease.Generation, lease.Status, ts(lease.AcquiredAt),
		ts(lease.ExpiresAt)); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if err := appendDockerContainerAttemptEvent(ctx, tx, intent,
		events.SandboxDockerAttemptPreparedEvent, intent.CreatedAt, map[string]any{
			"status":           sandbox.DockerContainerAttemptStatusPrepared,
			"lease_generation": 1, "container_started": false,
			"process_executed": false, "execution_authorized": false,
		}); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	attempt := sandbox.DockerContainerRehearsalAttempt{Intent: intent,
		Status: sandbox.DockerContainerAttemptStatusPrepared, Lease: lease}
	return sandbox.DockerContainerAttemptAcquisition{Attempt: attempt}, attempt.Validate()
}

func (s *SQLiteStore) AcquireDockerContainerRehearsalAttempt(ctx context.Context,
	attemptID, requestedBy, ownerID string, ttl time.Duration,
) (sandbox.DockerContainerAttemptAcquisition, error) {
	attemptID, requestedBy, ownerID = strings.TrimSpace(attemptID),
		strings.TrimSpace(requestedBy), strings.TrimSpace(ownerID)
	if !domain.ValidAgentID(attemptID) || !domain.ValidAgentID(requestedBy) ||
		!domain.ValidAgentID(ownerID) || strings.ContainsRune(attemptID+requestedBy+ownerID, 0) ||
		redact.String(ownerID) != ownerID {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt acquisition identity is invalid")
	}
	if err := sandbox.ValidateDockerContainerAttemptLeaseTTL(ttl); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, attemptID)
	if err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if attempt.Intent.RequestedBy != requestedBy {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker container attempt requester changed")
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.Intent.RunID); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	return acquireExistingDockerContainerAttempt(ctx, tx, attempt, ownerID, ttl)
}

func acquireExistingDockerContainerAttempt(ctx context.Context, tx *sql.Tx,
	attempt sandbox.DockerContainerRehearsalAttempt, ownerID string, ttl time.Duration,
) (sandbox.DockerContainerAttemptAcquisition, error) {
	if attempt.Completion != nil {
		if err := tx.Commit(); err != nil {
			return sandbox.DockerContainerAttemptAcquisition{}, err
		}
		attempt.Replayed = true
		return sandbox.DockerContainerAttemptAcquisition{Attempt: attempt, Replayed: true}, nil
	}
	if len(attempt.Failures) >= sandbox.MaxDockerContainerAttemptFailures {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(
			apperror.CodeResourceExhausted,
			"Docker container attempt failure ledger is exhausted")
	}
	now := time.Now().UTC()
	if attempt.Lease.ActiveAt(now) {
		return sandbox.DockerContainerAttemptAcquisition{}, apperror.New(apperror.CodeConflict,
			fmt.Sprintf("Docker container attempt is leased through %s",
				attempt.Lease.ExpiresAt.Format(time.RFC3339Nano)))
	}
	previous := attempt.Lease
	tookOver := previous.Status == sandbox.DockerContainerAttemptLeaseActive
	next := sandbox.DockerContainerAttemptLease{AttemptID: attempt.Intent.ID,
		LeaseID: newSandboxLeaseID(), OwnerID: ownerID, Generation: previous.Generation + 1,
		Status: sandbox.DockerContainerAttemptLeaseActive, AcquiredAt: now,
		ExpiresAt: now.Add(ttl)}
	if err := next.Validate(); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_container_attempt_leases
		SET lease_id = ?, owner_id = ?, generation = ?, status = 'active', acquired_at = ?,
		expires_at = ?, released_at = NULL WHERE attempt_id = ? AND lease_id = ?
		AND generation = ? AND status = ?`, next.LeaseID, next.OwnerID, next.Generation,
		ts(next.AcquiredAt), ts(next.ExpiresAt), previous.AttemptID, previous.LeaseID,
		previous.Generation, previous.Status)
	if err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if err := requireSingleLeaseUpdate(result,
		"Docker container attempt lease changed before acquisition"); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	eventType := events.SandboxDockerAttemptAcquiredEvent
	if tookOver {
		eventType = events.SandboxDockerAttemptTakenOverEvent
	}
	if err := appendDockerContainerAttemptEvent(ctx, tx, attempt.Intent, eventType, now,
		map[string]any{"lease_generation": next.Generation,
			"previous_generation": previous.Generation, "took_over": tookOver,
			"container_started": false, "process_executed": false,
			"execution_authorized": false}); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	attempt.Lease, attempt.TookOver = next, tookOver
	if err := attempt.Validate(); err != nil {
		return sandbox.DockerContainerAttemptAcquisition{}, err
	}
	return sandbox.DockerContainerAttemptAcquisition{Attempt: attempt, TookOver: tookOver}, nil
}

func (s *SQLiteStore) RecordDockerContainerAttemptStage(ctx context.Context,
	stage sandbox.DockerContainerAttemptStage, expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerContainerRehearsalAttempt, bool, error) {
	if stage.Validate() != nil || expected.Validate() != nil ||
		stage.AttemptID != expected.AttemptID ||
		stage.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerContainerRehearsalAttempt{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt stage binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, stage.AttemptID)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.Intent.RunID); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if attempt.Stage != nil {
		if attempt.Stage.CheckpointFingerprint != stage.CheckpointFingerprint {
			return sandbox.DockerContainerRehearsalAttempt{}, false, apperror.New(
				apperror.CodeConflict, "Docker container attempt stage is already immutable")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerContainerRehearsalAttempt{}, false, err
		}
		return attempt, true, nil
	}
	if stage.Result.RequestFingerprint != attempt.Intent.RequestFingerprint ||
		stage.Result.SpecFingerprint != attempt.Intent.SpecFingerprint ||
		stage.Result.EndpointFingerprint != attempt.Intent.EndpointFingerprint ||
		stage.RecordedAt.Before(expected.AcquiredAt) {
		return sandbox.DockerContainerRehearsalAttempt{}, false, apperror.New(
			apperror.CodeConflict, "Docker container attempt stage changed intent")
	}
	result := stage.Result
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_attempt_stages
		(attempt_id, lease_generation, protocol_version, status, endpoint_class,
		endpoint_fingerprint, request_fingerprint, spec_fingerprint,
		container_id_fingerprint, inspection_fingerprint, control_matrix_fingerprint,
		stage_fingerprint, control_count, daemon_read_count, daemon_write_count,
		container_created_now, existing_container_adopted, configuration_matched,
		container_present, container_never_started, process_never_executed,
		image_never_pulled, output_never_exported, production_execution_submitted,
		production_verified, backend_enabled, execution_authorized,
		artifact_commit_authorized, checkpoint_fingerprint, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?)`, stage.AttemptID, stage.LeaseGeneration,
		result.ProtocolVersion, result.Status, result.EndpointClass, result.EndpointFingerprint,
		result.RequestFingerprint, result.SpecFingerprint, result.ContainerIDFingerprint,
		result.InspectionFingerprint, result.ControlMatrixFingerprint, result.StageFingerprint,
		result.ControlCount, result.DaemonReadCount, result.DaemonWriteCount,
		boolInt(result.ContainerCreatedNow), boolInt(result.ExistingContainerAdopted),
		boolInt(result.ConfigurationMatched), boolInt(result.ContainerPresent),
		boolInt(!result.ContainerStarted), boolInt(!result.ProcessExecuted),
		boolInt(!result.ImagePulled), boolInt(!result.OutputExported),
		boolInt(result.ProductionExecutionSubmitted), boolInt(result.ProductionVerified),
		boolInt(result.BackendEnabled), boolInt(result.ExecutionAuthorized),
		boolInt(result.ArtifactCommitAuthorized), stage.CheckpointFingerprint,
		ts(stage.RecordedAt)); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	for _, control := range result.Controls {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_attempt_controls
			(attempt_id, ordinal, name, state, observed, verified, execution_evidence,
			control_digest) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, stage.AttemptID,
			control.Ordinal, control.Name, control.State, boolInt(control.Observed),
			boolInt(control.Verified), boolInt(control.ExecutionEvidence),
			control.ControlDigest); err != nil {
			return sandbox.DockerContainerRehearsalAttempt{}, false, err
		}
	}
	if err := appendDockerContainerAttemptEvent(ctx, tx, attempt.Intent,
		events.SandboxDockerAttemptStagedEvent, stage.RecordedAt, map[string]any{
			"status":           sandbox.DockerContainerAttemptStatusStaged,
			"lease_generation": stage.LeaseGeneration, "control_count": result.ControlCount,
			"container_created_now":      result.ContainerCreatedNow,
			"existing_container_adopted": result.ExistingContainerAdopted,
			"container_started":          false, "process_executed": false,
			"execution_authorized": false,
		}); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	value, err := getDockerContainerRehearsalAttempt(ctx, s.db, stage.AttemptID)
	return value, false, err
}

func (s *SQLiteStore) RecordDockerContainerAttemptCleanup(ctx context.Context,
	cleanup sandbox.DockerContainerAttemptCleanup, expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerContainerRehearsalAttempt, bool, error) {
	if cleanup.Validate() != nil || expected.Validate() != nil ||
		cleanup.AttemptID != expected.AttemptID ||
		cleanup.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerContainerRehearsalAttempt{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt cleanup binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, cleanup.AttemptID)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.Intent.RunID); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if attempt.Cleanup != nil {
		if attempt.Cleanup.CheckpointFingerprint != cleanup.CheckpointFingerprint {
			return sandbox.DockerContainerRehearsalAttempt{}, false, apperror.New(
				apperror.CodeConflict, "Docker container attempt cleanup is already immutable")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerContainerRehearsalAttempt{}, false, err
		}
		return attempt, true, nil
	}
	if attempt.Stage == nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, apperror.New(
			apperror.CodeFailedPrecondition, "Docker container attempt has no durable stage")
	}
	result := cleanup.Result
	if result.RequestFingerprint != attempt.Intent.RequestFingerprint ||
		result.EndpointFingerprint != attempt.Intent.EndpointFingerprint ||
		result.ContainerIDFingerprint != attempt.Stage.Result.ContainerIDFingerprint ||
		cleanup.RecordedAt.Before(expected.AcquiredAt) ||
		cleanup.RecordedAt.Before(attempt.Stage.RecordedAt) {
		return sandbox.DockerContainerRehearsalAttempt{}, false, apperror.New(
			apperror.CodeConflict, "Docker container attempt cleanup changed intent")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_attempt_cleanups
		(attempt_id, lease_generation, protocol_version, status, endpoint_class,
		endpoint_fingerprint, request_fingerprint, container_id_fingerprint,
		cleanup_fingerprint, daemon_read_count, daemon_write_count, container_removed_now,
		container_already_absent, cleanup_confirmed, container_never_started,
		process_never_executed, output_never_exported, execution_authorized,
		artifact_commit_authorized, checkpoint_fingerprint, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cleanup.AttemptID, cleanup.LeaseGeneration, result.ProtocolVersion, result.Status,
		result.EndpointClass, result.EndpointFingerprint, result.RequestFingerprint,
		result.ContainerIDFingerprint, result.CleanupFingerprint, result.DaemonReadCount,
		result.DaemonWriteCount, boolInt(result.ContainerRemovedNow),
		boolInt(result.ContainerAlreadyAbsent), boolInt(result.CleanupConfirmed),
		boolInt(!result.ContainerStarted), boolInt(!result.ProcessExecuted),
		boolInt(!result.OutputExported), boolInt(result.ExecutionAuthorized),
		boolInt(result.ArtifactCommitAuthorized), cleanup.CheckpointFingerprint,
		ts(cleanup.RecordedAt)); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if err := appendDockerContainerAttemptEvent(ctx, tx, attempt.Intent,
		events.SandboxDockerAttemptCleanupEvent, cleanup.RecordedAt, map[string]any{
			"status":                   sandbox.DockerContainerAttemptStatusCleaned,
			"lease_generation":         cleanup.LeaseGeneration,
			"container_removed_now":    result.ContainerRemovedNow,
			"container_already_absent": result.ContainerAlreadyAbsent,
			"container_started":        false, "process_executed": false,
			"execution_authorized": false,
		}); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	value, err := getDockerContainerRehearsalAttempt(ctx, s.db, cleanup.AttemptID)
	return value, false, err
}

func (s *SQLiteStore) FailDockerContainerRehearsalAttempt(ctx context.Context,
	failure sandbox.DockerContainerAttemptFailure, expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerContainerRehearsalAttempt, error) {
	if failure.Validate() != nil || expected.Validate() != nil ||
		failure.AttemptID != expected.AttemptID ||
		failure.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerContainerRehearsalAttempt{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt failure binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, failure.AttemptID)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.Intent.RunID); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	if attempt.Completion != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, apperror.New(
			apperror.CodeFailedPrecondition, "Docker container attempt is already complete")
	}
	if len(attempt.Failures) >= sandbox.MaxDockerContainerAttemptFailures {
		return sandbox.DockerContainerRehearsalAttempt{}, apperror.New(
			apperror.CodeResourceExhausted, "Docker container attempt failure ledger is exhausted")
	}
	if failure.Ordinal != len(attempt.Failures)+1 ||
		failure.CreatedAt.Before(expected.AcquiredAt) ||
		(len(attempt.Failures) > 0 &&
			failure.CreatedAt.Before(attempt.Failures[len(attempt.Failures)-1].CreatedAt)) {
		return sandbox.DockerContainerRehearsalAttempt{}, apperror.New(
			apperror.CodeConflict, "Docker container attempt failure sequence changed")
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_attempt_failures
		(attempt_id, ordinal, lease_generation, phase, code, retryable,
		failure_fingerprint, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, failure.AttemptID,
		failure.Ordinal, failure.LeaseGeneration, failure.Phase, failure.Code,
		boolInt(failure.Retryable), failure.FailureFingerprint, ts(failure.CreatedAt)); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	released := failure.CreatedAt
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_container_attempt_leases
		SET status = 'released', released_at = ? WHERE attempt_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(released),
		expected.AttemptID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	if err := requireSingleLeaseUpdate(result,
		"Docker container attempt lease changed before failure release"); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	if err := appendDockerContainerAttemptEvent(ctx, tx, attempt.Intent,
		events.SandboxDockerAttemptFailedEvent, failure.CreatedAt, map[string]any{
			"phase": failure.Phase, "code": failure.Code,
			"retryable": failure.Retryable, "lease_generation": failure.LeaseGeneration,
			"container_started": false, "process_executed": false,
			"execution_authorized": false,
		}); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	return getDockerContainerRehearsalAttempt(ctx, s.db, failure.AttemptID)
}

func (s *SQLiteStore) CompleteDockerContainerRehearsalAttempt(ctx context.Context,
	completion sandbox.DockerContainerAttemptCompletion,
	rehearsal sandbox.DockerContainerRehearsal,
	operation sandbox.DockerContainerRehearsalOperation,
	expected sandbox.DockerContainerAttemptLease,
) (sandbox.DockerContainerRehearsal, bool, error) {
	if completion.Validate() != nil || expected.Validate() != nil ||
		completion.AttemptID != expected.AttemptID ||
		completion.LeaseGeneration != expected.Generation ||
		completion.RehearsalID != rehearsal.ID ||
		!completion.CompletedAt.Equal(rehearsal.CreatedAt) ||
		expected.Status != sandbox.DockerContainerAttemptLeaseActive {
		return sandbox.DockerContainerRehearsal{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt completion binding is invalid")
	}
	if err := validateDockerContainerRehearsalMutation(rehearsal, operation); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, completion.AttemptID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.Intent.RunID); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if attempt.Completion != nil {
		if attempt.Completion.CompletionFingerprint != completion.CompletionFingerprint {
			return sandbox.DockerContainerRehearsal{}, false, apperror.New(
				apperror.CodeConflict, "Docker container attempt completion is already immutable")
		}
		value, err := getDockerContainerRehearsal(ctx, tx, attempt.Completion.RehearsalID)
		if err != nil {
			return sandbox.DockerContainerRehearsal{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerContainerRehearsal{}, false, err
		}
		value.Replayed = true
		return value, true, nil
	}
	if attempt.Stage == nil || attempt.Cleanup == nil {
		return sandbox.DockerContainerRehearsal{}, false, apperror.New(
			apperror.CodeFailedPrecondition, "Docker container attempt cleanup is not durable")
	}
	if err := requireCurrentDockerContainerAttemptLease(attempt.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if rehearsal.PlanID != attempt.Intent.PlanID || rehearsal.RunID != attempt.Intent.RunID ||
		rehearsal.RequestedBy != attempt.Intent.RequestedBy ||
		rehearsal.RequestFingerprint != attempt.Intent.RequestFingerprint ||
		rehearsal.SpecFingerprint != attempt.Intent.SpecFingerprint ||
		rehearsal.ContainerIDFingerprint != attempt.Stage.Result.ContainerIDFingerprint ||
		rehearsal.InspectionFingerprint != attempt.Stage.Result.InspectionFingerprint ||
		!rehearsal.CleanupConfirmed || !attempt.Cleanup.Result.CleanupConfirmed ||
		completion.CompletedAt.Before(expected.AcquiredAt) ||
		completion.CompletedAt.Before(attempt.Cleanup.RecordedAt) {
		return sandbox.DockerContainerRehearsal{}, false, apperror.New(
			apperror.CodeConflict, "Docker container rehearsal does not match its durable attempt")
	}
	if err := validateDockerContainerRehearsalCurrentTx(ctx, tx, rehearsal); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if _, found, err := getDockerContainerRehearsalOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	} else if found {
		return sandbox.DockerContainerRehearsal{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker rehearsal exists without its atomic attempt completion")
	}
	if err := insertDockerContainerRehearsalTx(ctx, tx, rehearsal); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	for _, step := range rehearsal.Result.Steps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_rehearsal_steps
			(rehearsal_id, ordinal, name, state, daemon_reads, daemon_writes,
			production_applied, step_digest) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, rehearsal.ID,
			step.Ordinal, step.Name, step.State, step.DaemonReads, step.DaemonWrites,
			boolInt(step.ProductionApplied), step.StepDigest); err != nil {
			return sandbox.DockerContainerRehearsal{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_rehearsal_operations
		(operation_key_digest, request_fingerprint, rehearsal_id, plan_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.RehearsalID, operation.PlanID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := appendDockerContainerRehearsalEvent(ctx, tx, rehearsal); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_attempt_completions
		(attempt_id, rehearsal_id, lease_generation, completion_fingerprint, completed_at)
		VALUES (?, ?, ?, ?, ?)`, completion.AttemptID, completion.RehearsalID,
		completion.LeaseGeneration, completion.CompletionFingerprint,
		ts(completion.CompletedAt)); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_container_attempt_leases
		SET status = 'released', released_at = ? WHERE attempt_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(completion.CompletedAt),
		expected.AttemptID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := requireSingleLeaseUpdate(result,
		"Docker container attempt lease changed before completion"); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := appendDockerContainerAttemptEvent(ctx, tx, attempt.Intent,
		events.SandboxDockerAttemptCompletedEvent, completion.CompletedAt, map[string]any{
			"status":            sandbox.DockerContainerAttemptStatusCompleted,
			"lease_generation":  completion.LeaseGeneration,
			"control_count":     attempt.Stage.Result.ControlCount,
			"container_started": false, "process_executed": false,
			"production_verified": false, "execution_authorized": false,
		}); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	return rehearsal, false, nil
}

func (s *SQLiteStore) GetDockerContainerRehearsalAttempt(ctx context.Context,
	id string,
) (sandbox.DockerContainerRehearsalAttempt, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerContainerRehearsalAttempt{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker container attempt id is invalid")
	}
	return getDockerContainerRehearsalAttempt(ctx, s.db, id)
}

func (s *SQLiteStore) ListDockerContainerRehearsalAttempts(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerContainerRehearsalAttempt, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker container attempt list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker container attempt list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id
		FROM sandbox_docker_container_rehearsal_attempts
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
	values := make([]sandbox.DockerContainerRehearsalAttempt, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerContainerRehearsalAttempt(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerContainerAttemptByOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.DockerContainerRehearsalAttempt, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_container_rehearsal_attempts WHERE operation_key_digest = ?`,
		keyDigest).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerContainerRehearsalAttempt{}, false, nil
	}
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	value, err := getDockerContainerRehearsalAttempt(ctx, queryer, id)
	return value, err == nil, err
}

func getDockerContainerAttemptByPlan(ctx context.Context, queryer sandboxLifecycleQueryer,
	planID string,
) (sandbox.DockerContainerRehearsalAttempt, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id
		FROM sandbox_docker_container_rehearsal_attempts WHERE plan_id = ?`, planID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerContainerRehearsalAttempt{}, false, nil
	}
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, false, err
	}
	value, err := getDockerContainerRehearsalAttempt(ctx, queryer, id)
	return value, err == nil, err
}

func getDockerContainerRehearsalAttempt(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerContainerRehearsalAttempt, error) {
	intent, err := scanDockerContainerAttemptIntent(queryer.QueryRowContext(ctx,
		dockerContainerAttemptIntentSelect+` WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sandbox.DockerContainerRehearsalAttempt{}, apperror.New(
				apperror.CodeNotFound, "Docker container rehearsal attempt not found")
		}
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	lease, err := getDockerContainerAttemptLease(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	stage, found, err := getDockerContainerAttemptStage(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	cleanup, cleanupFound, err := getDockerContainerAttemptCleanup(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	failures, err := listDockerContainerAttemptFailures(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	completion, completionFound, err := getDockerContainerAttemptCompletion(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, err
	}
	attempt := sandbox.DockerContainerRehearsalAttempt{Intent: intent,
		Status: sandbox.DockerContainerAttemptStatusPrepared, Lease: lease, Failures: failures}
	if found {
		attempt.Stage = &stage
		attempt.Status = sandbox.DockerContainerAttemptStatusStaged
	}
	if cleanupFound {
		attempt.Cleanup = &cleanup
		attempt.Status = sandbox.DockerContainerAttemptStatusCleaned
	}
	if completionFound {
		attempt.Completion = &completion
		attempt.Status = sandbox.DockerContainerAttemptStatusCompleted
	}
	if err := attempt.Validate(); err != nil {
		return sandbox.DockerContainerRehearsalAttempt{}, fmt.Errorf(
			"stored Docker container rehearsal attempt is invalid: %w", err)
	}
	return attempt, nil
}

func scanDockerContainerAttemptIntent(row scanner) (sandbox.DockerContainerAttemptIntent, error) {
	var intent sandbox.DockerContainerAttemptIntent
	var createdAt string
	err := row.Scan(&intent.ID, &intent.PlanID, &intent.ObservationID, &intent.EvidenceID,
		&intent.OutputSimulationID, &intent.PreflightID, &intent.ExecutionID,
		&intent.CandidateID, &intent.PreparationID, &intent.RunID, &intent.MissionID,
		&intent.WorkspaceID, &intent.ProtocolVersion, &intent.OperationKeyDigest,
		&intent.ManifestFingerprint, &intent.AuthorizationFingerprint,
		&intent.PolicyFingerprint, &intent.MountBindingFingerprint,
		&intent.InputArtifactDigest, &intent.ThreatModelFingerprint,
		&intent.OutputPlanFingerprint, &intent.ObservationFingerprint,
		&intent.AuthorityFingerprint, &intent.SpecFingerprint, &intent.PlanFingerprint,
		&intent.ImageDigest, &intent.RequestFingerprint, &intent.EndpointClass,
		&intent.EndpointFingerprint, &intent.NetworkMode, &intent.EnvironmentCount,
		&intent.SecretReferenceCount, &intent.IntentFingerprint, &intent.RequestedBy,
		&createdAt)
	intent.CreatedAt = parseTS(createdAt)
	if err == nil {
		err = intent.Validate()
	}
	return intent, err
}

func getDockerContainerAttemptLease(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) (sandbox.DockerContainerAttemptLease, error) {
	var lease sandbox.DockerContainerAttemptLease
	var acquiredAt, expiresAt string
	var releasedAt sql.NullString
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, lease_id, owner_id, generation,
		status, acquired_at, expires_at, released_at
		FROM sandbox_docker_container_attempt_leases WHERE attempt_id = ?`, attemptID).Scan(
		&lease.AttemptID, &lease.LeaseID, &lease.OwnerID, &lease.Generation, &lease.Status,
		&acquiredAt, &expiresAt, &releasedAt)
	if err != nil {
		return sandbox.DockerContainerAttemptLease{}, err
	}
	lease.AcquiredAt, lease.ExpiresAt = parseTS(acquiredAt), parseTS(expiresAt)
	if releasedAt.Valid {
		value := parseTS(releasedAt.String)
		lease.ReleasedAt = &value
	}
	if err := lease.Validate(); err != nil {
		return sandbox.DockerContainerAttemptLease{}, err
	}
	return lease, nil
}

func getDockerContainerAttemptStage(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) (sandbox.DockerContainerAttemptStage, bool, error) {
	var stage sandbox.DockerContainerAttemptStage
	var recordedAt string
	var createdNow, adopted, matched, present int
	var neverStarted, neverExecuted, neverPulled, neverExported int
	var productionSubmitted, productionVerified, backendEnabled, executionAuthorized int
	var artifactAuthorized int
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, lease_generation,
		protocol_version, status, endpoint_class, endpoint_fingerprint, request_fingerprint,
		spec_fingerprint, container_id_fingerprint, inspection_fingerprint,
		control_matrix_fingerprint, stage_fingerprint, control_count, daemon_read_count,
		daemon_write_count, container_created_now, existing_container_adopted,
		configuration_matched, container_present, container_never_started,
		process_never_executed, image_never_pulled, output_never_exported,
		production_execution_submitted, production_verified, backend_enabled,
		execution_authorized, artifact_commit_authorized, checkpoint_fingerprint, recorded_at
		FROM sandbox_docker_container_attempt_stages WHERE attempt_id = ?`, attemptID).Scan(
		&stage.AttemptID, &stage.LeaseGeneration, &stage.Result.ProtocolVersion,
		&stage.Result.Status, &stage.Result.EndpointClass, &stage.Result.EndpointFingerprint,
		&stage.Result.RequestFingerprint, &stage.Result.SpecFingerprint,
		&stage.Result.ContainerIDFingerprint, &stage.Result.InspectionFingerprint,
		&stage.Result.ControlMatrixFingerprint, &stage.Result.StageFingerprint,
		&stage.Result.ControlCount, &stage.Result.DaemonReadCount,
		&stage.Result.DaemonWriteCount, &createdNow, &adopted, &matched, &present,
		&neverStarted, &neverExecuted, &neverPulled, &neverExported,
		&productionSubmitted, &productionVerified, &backendEnabled,
		&executionAuthorized, &artifactAuthorized, &stage.CheckpointFingerprint,
		&recordedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerContainerAttemptStage{}, false, nil
	}
	if err != nil {
		return sandbox.DockerContainerAttemptStage{}, false, err
	}
	stage.Result.ContainerCreatedNow = createdNow != 0
	stage.Result.ExistingContainerAdopted = adopted != 0
	stage.Result.ConfigurationMatched = matched != 0
	stage.Result.ContainerPresent = present != 0
	stage.Result.ContainerStarted = neverStarted == 0
	stage.Result.ProcessExecuted = neverExecuted == 0
	stage.Result.ImagePulled = neverPulled == 0
	stage.Result.OutputExported = neverExported == 0
	stage.Result.ProductionExecutionSubmitted = productionSubmitted != 0
	stage.Result.ProductionVerified = productionVerified != 0
	stage.Result.BackendEnabled = backendEnabled != 0
	stage.Result.ExecutionAuthorized = executionAuthorized != 0
	stage.Result.ArtifactCommitAuthorized = artifactAuthorized != 0
	stage.RecordedAt = parseTS(recordedAt)
	stage.Result.Controls, err = listDockerContainerAttemptControls(ctx, queryer, attemptID)
	if err != nil {
		return sandbox.DockerContainerAttemptStage{}, false, err
	}
	if err := stage.Validate(); err != nil {
		return sandbox.DockerContainerAttemptStage{}, false, err
	}
	return stage, true, nil
}

func listDockerContainerAttemptControls(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) ([]sandbox.DockerContainerVerifiedControl, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, state, observed, verified,
		execution_evidence, control_digest FROM sandbox_docker_container_attempt_controls
		WHERE attempt_id = ? ORDER BY ordinal`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []sandbox.DockerContainerVerifiedControl
	for rows.Next() {
		var value sandbox.DockerContainerVerifiedControl
		var observed, verified, executionEvidence int
		if err := rows.Scan(&value.Ordinal, &value.Name, &value.State, &observed, &verified,
			&executionEvidence, &value.ControlDigest); err != nil {
			return nil, err
		}
		value.Observed, value.Verified = observed != 0, verified != 0
		value.ExecutionEvidence = executionEvidence != 0
		values = append(values, value)
	}
	return values, rows.Err()
}

func getDockerContainerAttemptCleanup(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) (sandbox.DockerContainerAttemptCleanup, bool, error) {
	var cleanup sandbox.DockerContainerAttemptCleanup
	var recordedAt string
	var removedNow, alreadyAbsent, confirmed int
	var neverStarted, neverExecuted, neverExported, executionAuthorized, artifactAuthorized int
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, lease_generation,
		protocol_version, status, endpoint_class, endpoint_fingerprint, request_fingerprint,
		container_id_fingerprint, cleanup_fingerprint, daemon_read_count, daemon_write_count,
		container_removed_now, container_already_absent, cleanup_confirmed,
		container_never_started, process_never_executed, output_never_exported,
		execution_authorized, artifact_commit_authorized, checkpoint_fingerprint, recorded_at
		FROM sandbox_docker_container_attempt_cleanups WHERE attempt_id = ?`, attemptID).Scan(
		&cleanup.AttemptID, &cleanup.LeaseGeneration, &cleanup.Result.ProtocolVersion,
		&cleanup.Result.Status, &cleanup.Result.EndpointClass,
		&cleanup.Result.EndpointFingerprint, &cleanup.Result.RequestFingerprint,
		&cleanup.Result.ContainerIDFingerprint, &cleanup.Result.CleanupFingerprint,
		&cleanup.Result.DaemonReadCount, &cleanup.Result.DaemonWriteCount, &removedNow,
		&alreadyAbsent, &confirmed, &neverStarted, &neverExecuted, &neverExported,
		&executionAuthorized, &artifactAuthorized, &cleanup.CheckpointFingerprint,
		&recordedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerContainerAttemptCleanup{}, false, nil
	}
	if err != nil {
		return sandbox.DockerContainerAttemptCleanup{}, false, err
	}
	cleanup.Result.ContainerRemovedNow = removedNow != 0
	cleanup.Result.ContainerAlreadyAbsent = alreadyAbsent != 0
	cleanup.Result.CleanupConfirmed = confirmed != 0
	cleanup.Result.ContainerStarted = neverStarted == 0
	cleanup.Result.ProcessExecuted = neverExecuted == 0
	cleanup.Result.OutputExported = neverExported == 0
	cleanup.Result.ExecutionAuthorized = executionAuthorized != 0
	cleanup.Result.ArtifactCommitAuthorized = artifactAuthorized != 0
	cleanup.RecordedAt = parseTS(recordedAt)
	if err := cleanup.Validate(); err != nil {
		return sandbox.DockerContainerAttemptCleanup{}, false, err
	}
	return cleanup, true, nil
}

func listDockerContainerAttemptFailures(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) ([]sandbox.DockerContainerAttemptFailure, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT attempt_id, ordinal, lease_generation,
		phase, code, retryable, failure_fingerprint, created_at
		FROM sandbox_docker_container_attempt_failures
		WHERE attempt_id = ? ORDER BY ordinal`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []sandbox.DockerContainerAttemptFailure
	for rows.Next() {
		var value sandbox.DockerContainerAttemptFailure
		var retryable int
		var createdAt string
		if err := rows.Scan(&value.AttemptID, &value.Ordinal, &value.LeaseGeneration,
			&value.Phase, &value.Code, &retryable, &value.FailureFingerprint,
			&createdAt); err != nil {
			return nil, err
		}
		value.Retryable, value.CreatedAt = retryable != 0, parseTS(createdAt)
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func getDockerContainerAttemptCompletion(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) (sandbox.DockerContainerAttemptCompletion, bool, error) {
	var value sandbox.DockerContainerAttemptCompletion
	var completedAt string
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, rehearsal_id, lease_generation,
		completion_fingerprint, completed_at
		FROM sandbox_docker_container_attempt_completions WHERE attempt_id = ?`, attemptID).Scan(
		&value.AttemptID, &value.RehearsalID, &value.LeaseGeneration,
		&value.CompletionFingerprint, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerContainerAttemptCompletion{}, false, nil
	}
	if err != nil {
		return sandbox.DockerContainerAttemptCompletion{}, false, err
	}
	value.CompletedAt = parseTS(completedAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerContainerAttemptCompletion{}, false, err
	}
	return value, true, nil
}

func insertDockerContainerAttemptIntentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerContainerAttemptIntent,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_rehearsal_attempts
		(id, plan_id, observation_id, evidence_id, output_simulation_id, preflight_id,
		execution_id, candidate_id, preparation_id, run_id, mission_id, workspace_id,
		protocol_version, operation_key_digest, manifest_fingerprint,
		authorization_fingerprint, policy_fingerprint, mount_binding_fingerprint,
		input_artifact_digest, threat_model_fingerprint, output_plan_fingerprint,
		observation_fingerprint, authority_fingerprint, spec_fingerprint, plan_fingerprint,
		image_digest, request_fingerprint, endpoint_class, endpoint_fingerprint,
		network_mode, environment_count, secret_reference_count, intent_fingerprint,
		requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, intent.ID, intent.PlanID, intent.ObservationID,
		intent.EvidenceID, intent.OutputSimulationID, intent.PreflightID, intent.ExecutionID,
		intent.CandidateID, intent.PreparationID, intent.RunID, intent.MissionID,
		intent.WorkspaceID, intent.ProtocolVersion, intent.OperationKeyDigest,
		intent.ManifestFingerprint, intent.AuthorizationFingerprint, intent.PolicyFingerprint,
		intent.MountBindingFingerprint, intent.InputArtifactDigest,
		intent.ThreatModelFingerprint, intent.OutputPlanFingerprint,
		intent.ObservationFingerprint, intent.AuthorityFingerprint, intent.SpecFingerprint,
		intent.PlanFingerprint, intent.ImageDigest, intent.RequestFingerprint,
		intent.EndpointClass, intent.EndpointFingerprint, intent.NetworkMode,
		intent.EnvironmentCount, intent.SecretReferenceCount, intent.IntentFingerprint,
		intent.RequestedBy, ts(intent.CreatedAt))
	return err
}

func validateDockerContainerAttemptIntentCurrentTx(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerContainerAttemptIntent,
) error {
	plan, err := getDockerContainerPlan(ctx, tx, intent.PlanID)
	if err != nil {
		return err
	}
	if err := validateDockerContainerPlanCurrentTx(ctx, tx, plan); err != nil {
		return err
	}
	if intent.ObservationID != plan.ObservationID || intent.EvidenceID != plan.EvidenceID ||
		intent.OutputSimulationID != plan.OutputSimulationID ||
		intent.PreflightID != plan.PreflightID || intent.ExecutionID != plan.ExecutionID ||
		intent.CandidateID != plan.CandidateID || intent.PreparationID != plan.PreparationID ||
		intent.RunID != plan.RunID || intent.MissionID != plan.MissionID ||
		intent.WorkspaceID != plan.WorkspaceID ||
		intent.ManifestFingerprint != plan.ManifestFingerprint ||
		intent.AuthorizationFingerprint != plan.AuthorizationFingerprint ||
		intent.PolicyFingerprint != plan.PolicyFingerprint ||
		intent.MountBindingFingerprint != plan.MountBindingFingerprint ||
		intent.InputArtifactDigest != plan.InputArtifactDigest ||
		intent.ThreatModelFingerprint != plan.ThreatModelFingerprint ||
		intent.OutputPlanFingerprint != plan.OutputPlanFingerprint ||
		intent.ObservationFingerprint != plan.ObservationFingerprint ||
		intent.AuthorityFingerprint != plan.AuthorityFingerprint ||
		intent.SpecFingerprint != plan.SpecFingerprint ||
		intent.PlanFingerprint != plan.PlanFingerprint || intent.ImageDigest != plan.ImageDigest ||
		intent.NetworkMode != "disabled" || intent.EnvironmentCount != 0 ||
		intent.SecretReferenceCount != 0 || intent.RequestedBy != plan.RequestedBy ||
		!plan.SimulationOnly || plan.ProductionSubmitted || plan.ProductionVerified ||
		plan.BackendAvailable || plan.BackendEnabled || plan.ExecutionAuthorized ||
		plan.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker container attempt does not match the current v48-v54 authority chain")
	}
	return nil
}

func requireCurrentDockerContainerAttemptLease(current,
	expected sandbox.DockerContainerAttemptLease, now time.Time,
) error {
	if current.AttemptID != expected.AttemptID || current.LeaseID != expected.LeaseID ||
		current.OwnerID != expected.OwnerID || current.Generation != expected.Generation ||
		current.Status != expected.Status || !current.AcquiredAt.Equal(expected.AcquiredAt) ||
		!current.ExpiresAt.Equal(expected.ExpiresAt) || !current.ActiveAt(now) {
		return apperror.New(apperror.CodeConflict,
			"Docker container attempt lease expired or was replaced")
	}
	return nil
}

func appendDockerContainerAttemptEvent(ctx context.Context, tx *sql.Tx,
	intent sandbox.DockerContainerAttemptIntent, eventType string, createdAt time.Time,
	payload map[string]any,
) error {
	event, err := events.New(intent.RunID, intent.MissionID, eventType,
		"sandbox_docker_container_attempt", intent.ID, payload)
	if err != nil {
		return err
	}
	event.CreatedAt = createdAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
