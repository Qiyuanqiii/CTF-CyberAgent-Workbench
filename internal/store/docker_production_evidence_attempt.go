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

const dockerProductionEvidenceAttemptSelect = `SELECT id, review_id, cleanup_intent_id,
	run_id, mission_id, workspace_id, protocol_version, operation_key_digest,
	request_fingerprint, review_fingerprint, authority_fingerprint,
	threat_model_fingerprint, suite_fingerprint, endpoint_class, endpoint_fingerprint,
	capture_timeout_millis, operator_confirmed, real_daemon_contact_authorized,
	container_start_authorized, process_execution_authorized, output_export_authorized,
	artifact_commit_authorized, attempt_fingerprint, requested_by, created_at
	FROM sandbox_docker_production_evidence_attempts`

func (s *SQLiteStore) BeginDockerProductionEvidenceAttempt(ctx context.Context,
	attempt sandbox.DockerProductionEvidenceAttempt, ownerID string, ttl time.Duration,
) (sandbox.DockerProductionEvidenceAttemptAcquisition, error) {
	if err := attempt.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker production evidence attempt is invalid", err)
	}
	ownerID = strings.TrimSpace(ownerID)
	if !validDockerProductionEvidenceAttemptOwner(ownerID) {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence attempt lease owner is invalid")
	}
	if err := sandbox.ValidateDockerProductionEvidenceAttemptLeaseTTL(ttl); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, attempt.RunID); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if existing, found, lookupErr := getDockerProductionEvidenceAttemptByOperation(ctx, tx,
		attempt.OperationKeyDigest); lookupErr != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, lookupErr
	} else if found {
		if existing.Attempt.AttemptFingerprint != attempt.AttemptFingerprint ||
			existing.Attempt.RequestFingerprint != attempt.RequestFingerprint ||
			existing.Attempt.RequestedBy != attempt.RequestedBy {
			return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
				apperror.CodeConflict, "Docker production evidence attempt operation key changed intent")
		}
		return acquireExistingDockerProductionEvidenceAttempt(ctx, tx, existing, ownerID, ttl)
	}
	if _, found, lookupErr := getDockerProductionEvidenceOperation(ctx, tx,
		attempt.OperationKeyDigest); lookupErr != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, lookupErr
	} else if found {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker production evidence operation already completed without this attempt")
	}
	if err := validateDockerProductionEvidenceAttemptCurrentTx(ctx, tx, attempt); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	now := time.Now().UTC()
	if attempt.CreatedAt.After(now) {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence attempt timestamp is in the future")
	}
	lease := sandbox.DockerProductionEvidenceAttemptLease{
		AttemptID: attempt.ID, LeaseID: newSandboxLeaseID(), OwnerID: ownerID,
		Generation: 1, Status: sandbox.DockerProductionEvidenceAttemptLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := lease.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if err := insertDockerProductionEvidenceAttemptTx(ctx, tx, attempt); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_attempt_operations
		(key_digest, request_fingerprint, attempt_id, review_id, run_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, attempt.OperationKeyDigest, attempt.RequestFingerprint,
		attempt.ID, attempt.ReviewID, attempt.RunID, attempt.RequestedBy, ts(attempt.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_attempt_leases
		(attempt_id, lease_id, owner_id, generation, status, acquired_at, expires_at, released_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`, lease.AttemptID, lease.LeaseID, lease.OwnerID,
		lease.Generation, lease.Status, ts(lease.AcquiredAt), ts(lease.ExpiresAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, attempt,
		events.SandboxDockerProductionEvidenceAttemptPreparedEvent, attempt.CreatedAt,
		map[string]any{"lease_generation": lease.Generation,
			"endpoint_class": attempt.EndpointClass, "capture_timeout_millis": attempt.CaptureTimeoutMillis,
			"real_daemon_contact_authorized": false, "container_start_authorized": false,
			"process_execution_authorized": false}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	record := sandbox.DockerProductionEvidenceAttemptRecord{Attempt: attempt, Lease: lease}
	return sandbox.DockerProductionEvidenceAttemptAcquisition{Record: record}, record.Validate()
}

func (s *SQLiteStore) AcquireDockerProductionEvidenceAttempt(ctx context.Context,
	attemptID, requestedBy, ownerID string, ttl time.Duration,
) (sandbox.DockerProductionEvidenceAttemptAcquisition, error) {
	attemptID, requestedBy, ownerID = strings.TrimSpace(attemptID),
		strings.TrimSpace(requestedBy), strings.TrimSpace(ownerID)
	if !domain.ValidAgentID(attemptID) || !domain.ValidAgentID(requestedBy) ||
		!validDockerProductionEvidenceAttemptOwner(ownerID) {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence attempt acquisition identity is invalid")
	}
	if err := sandbox.ValidateDockerProductionEvidenceAttemptLeaseTTL(ttl); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerProductionEvidenceAttempt(ctx, tx, attemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if record.Attempt.RequestedBy != requestedBy {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
			apperror.CodeConflict, "Docker production evidence attempt requester changed")
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Attempt.RunID); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	record, err = getDockerProductionEvidenceAttempt(ctx, tx, attemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	return acquireExistingDockerProductionEvidenceAttempt(ctx, tx, record, ownerID, ttl)
}

func acquireExistingDockerProductionEvidenceAttempt(ctx context.Context, tx *sql.Tx,
	record sandbox.DockerProductionEvidenceAttemptRecord, ownerID string, ttl time.Duration,
) (sandbox.DockerProductionEvidenceAttemptAcquisition, error) {
	if record.Result != nil {
		if err := tx.Commit(); err != nil {
			return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
		}
		record.Replayed = true
		return sandbox.DockerProductionEvidenceAttemptAcquisition{Record: record, Replayed: true}, nil
	}
	if len(record.Failures) >= sandbox.MaxDockerProductionEvidenceAttemptFailures {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
			apperror.CodeResourceExhausted, "Docker production evidence attempt failure ledger is exhausted")
	}
	now := time.Now().UTC()
	if record.Lease.ActiveAt(now) {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, apperror.New(
			apperror.CodeConflict, fmt.Sprintf("Docker production evidence attempt is leased through %s",
				record.Lease.ExpiresAt.Format(time.RFC3339Nano)))
	}
	previous := record.Lease
	tookOver := previous.Status == sandbox.DockerProductionEvidenceAttemptLeaseActive
	next := sandbox.DockerProductionEvidenceAttemptLease{
		AttemptID: record.Attempt.ID, LeaseID: newSandboxLeaseID(), OwnerID: ownerID,
		Generation: previous.Generation + 1, Status: sandbox.DockerProductionEvidenceAttemptLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := next.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_production_evidence_attempt_leases
		SET lease_id = ?, owner_id = ?, generation = ?, status = 'active', acquired_at = ?,
		expires_at = ?, released_at = NULL WHERE attempt_id = ? AND lease_id = ?
		AND generation = ? AND status = ?`, next.LeaseID, next.OwnerID, next.Generation,
		ts(next.AcquiredAt), ts(next.ExpiresAt), previous.AttemptID, previous.LeaseID,
		previous.Generation, previous.Status)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if err := requireSingleLeaseUpdate(result,
		"Docker production evidence attempt lease changed before acquisition"); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	eventType := events.SandboxDockerProductionEvidenceAttemptAcquiredEvent
	if tookOver {
		eventType = events.SandboxDockerProductionEvidenceAttemptTakenOverEvent
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt, eventType, now,
		map[string]any{"lease_generation": next.Generation,
			"previous_generation": previous.Generation, "took_over": tookOver,
			"real_daemon_contact_authorized": false, "container_start_authorized": false,
			"process_execution_authorized": false}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	record.Lease, record.TookOver = next, tookOver
	if err := record.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptAcquisition{}, err
	}
	return sandbox.DockerProductionEvidenceAttemptAcquisition{Record: record, TookOver: tookOver}, nil
}

func validDockerProductionEvidenceAttemptOwner(value string) bool {
	return domain.ValidAgentID(value) && !strings.ContainsRune(value, 0) && redact.String(value) == value
}

func (s *SQLiteStore) RecordDockerProductionEvidenceReconciliation(ctx context.Context,
	value sandbox.DockerProductionEvidenceReconciliation,
	expected sandbox.DockerProductionEvidenceAttemptLease,
) (sandbox.DockerProductionEvidenceAttemptRecord, bool, error) {
	if value.Validate() != nil || expected.Validate() != nil ||
		value.AttemptID != expected.AttemptID || value.Generation != expected.Generation ||
		expected.Status != sandbox.DockerProductionEvidenceAttemptLeaseActive {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence reconciliation binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerProductionEvidenceAttempt(ctx, tx, value.AttemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Attempt.RunID); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	record, err = getDockerProductionEvidenceAttempt(ctx, tx, value.AttemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if record.Result != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeConflict, "Completed Docker production evidence attempt cannot reconcile")
	}
	if err := requireCurrentDockerProductionEvidenceAttemptLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if existing, found := reconciliationForGeneration(record.Reconciliations, value.Generation); found {
		if existing.ReconciliationFingerprint != value.ReconciliationFingerprint {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
				apperror.CodeConflict, "Docker production evidence reconciliation is already immutable")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
		}
		record.Replayed = true
		return record, true, nil
	}
	if value.EndpointClass != record.Attempt.EndpointClass ||
		value.EndpointFingerprint != record.Attempt.EndpointFingerprint ||
		value.CreatedAt.Before(expected.AcquiredAt) || !value.CreatedAt.Before(expected.ExpiresAt) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeConflict, "Docker production evidence reconciliation changed durable intent")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_reconciliations
		(attempt_id, generation, previous_generation, protocol_version, status,
		endpoint_class, endpoint_fingerprint, real_daemon_contacted, daemon_read_count,
		reconciled_resource_count, reconciliation_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.AttemptID, value.Generation,
		value.PreviousGeneration, value.ProtocolVersion, value.Status, value.EndpointClass,
		value.EndpointFingerprint, boolInt(value.RealDaemonContacted), value.DaemonReadCount,
		value.ReconciledResourceCount, value.ReconciliationFingerprint, ts(value.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt,
		events.SandboxDockerProductionEvidenceReconciledEvent, value.CreatedAt,
		map[string]any{"lease_generation": value.Generation,
			"previous_generation": value.PreviousGeneration, "status": value.Status,
			"real_daemon_contacted": false, "daemon_read_count": 0,
			"reconciled_resource_count": 0}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	record.Reconciliations = append(record.Reconciliations, value)
	return record, false, record.Validate()
}

func (s *SQLiteStore) CompleteDockerProductionEvidenceAttempt(ctx context.Context,
	value sandbox.DockerProductionEvidence, operation sandbox.DockerProductionEvidenceOperation,
	result sandbox.DockerProductionEvidenceAttemptResult,
	expected sandbox.DockerProductionEvidenceAttemptLease,
) (sandbox.DockerProductionEvidenceAttemptRecord, sandbox.DockerProductionEvidence, bool, error) {
	if err := validateDockerProductionEvidenceMutation(value, operation); err != nil ||
		result.Validate() != nil || expected.Validate() != nil ||
		result.AttemptID != expected.AttemptID || result.EvidenceID != value.ID ||
		result.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerProductionEvidenceAttemptLeaseActive {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument,
				"Docker production evidence attempt completion is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerProductionEvidenceAttempt(ctx, tx, result.AttemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Attempt.RunID); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	record, err = getDockerProductionEvidenceAttempt(ctx, tx, result.AttemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if record.Result != nil {
		if record.Result.ResultFingerprint != result.ResultFingerprint {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false,
				apperror.New(apperror.CodeConflict,
					"Docker production evidence attempt result is already immutable")
		}
		existing, loadErr := getDockerProductionEvidence(ctx, tx, record.Result.EvidenceID)
		if loadErr != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, loadErr
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
		}
		record.Replayed, existing.Replayed = true, true
		return record, existing, true, nil
	}
	if err := requireCurrentDockerProductionEvidenceAttemptLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	reconciliation, found := reconciliationForGeneration(record.Reconciliations,
		expected.Generation)
	if !found || reconciliation.ReconciliationFingerprint != result.ReconciliationFingerprint ||
		record.Attempt.ReviewID != value.ReviewID || record.Attempt.RunID != value.RunID ||
		record.Attempt.OperationKeyDigest != value.OperationKeyDigest ||
		record.Attempt.RequestFingerprint != operation.RequestFingerprint ||
		record.Attempt.RequestedBy != value.RequestedBy ||
		result.EvidenceCaptureFingerprint != value.CaptureFingerprint ||
		value.CreatedAt.Before(reconciliation.CreatedAt) || !value.CreatedAt.Before(expected.ExpiresAt) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false,
			apperror.New(apperror.CodeConflict,
				"Docker production evidence changed its write-ahead attempt")
	}
	if err := validateDockerProductionEvidenceCurrentTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if err := insertDockerProductionEvidenceTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	for _, item := range value.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_items
			(evidence_id, ordinal, name, probe_code, state, observed,
			production_verified, sufficient_for_start, blocker_code, evidence_digest)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, item.Ordinal, item.Name,
			item.ProbeCode, item.State, boolInt(item.Observed), boolInt(item.ProductionVerified),
			boolInt(item.SufficientForStart), item.BlockerCode, item.EvidenceDigest); err != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_attempt_results
		(attempt_id, evidence_id, protocol_version, status, lease_generation,
		reconciliation_fingerprint, evidence_capture_fingerprint, real_daemon_contacted,
		container_start_authorized, process_execution_authorized, output_export_authorized,
		artifact_commit_authorized, result_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, result.AttemptID,
		result.EvidenceID, result.ProtocolVersion, result.Status, result.LeaseGeneration,
		result.ReconciliationFingerprint, result.EvidenceCaptureFingerprint,
		boolInt(result.RealDaemonContacted), boolInt(result.ContainerStartAuthorized),
		boolInt(result.ProcessExecutionAuthorized), boolInt(result.OutputExportAuthorized),
		boolInt(result.ArtifactCommitAuthorized), result.ResultFingerprint, ts(result.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_operations
		(key_digest, request_fingerprint, evidence_id, review_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.EvidenceID, operation.ReviewID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_production_evidence_attempt_leases
		SET status = 'released', released_at = ? WHERE attempt_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(result.CreatedAt),
		expected.AttemptID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if err := requireSingleLeaseUpdate(update,
		"Docker production evidence attempt lease changed before completion"); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if err := appendDockerProductionEvidenceEvent(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt,
		events.SandboxDockerProductionEvidenceAttemptCompletedEvent, result.CreatedAt,
		map[string]any{"lease_generation": result.LeaseGeneration,
			"evidence_id": result.EvidenceID, "real_daemon_contacted": false,
			"container_start_authorized": false, "process_execution_authorized": false}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, sandbox.DockerProductionEvidence{}, false, err
	}
	releasedAt := result.CreatedAt
	record.Lease.Status, record.Lease.ReleasedAt = sandbox.DockerProductionEvidenceAttemptLeaseReleased,
		&releasedAt
	record.Result = &result
	return record, value, false, record.Validate()
}

func (s *SQLiteStore) RecordDockerProductionEvidenceAttemptFailure(ctx context.Context,
	attemptID string, expected sandbox.DockerProductionEvidenceAttemptLease,
	code string, createdAt time.Time,
) (sandbox.DockerProductionEvidenceAttemptRecord, error) {
	attemptID = strings.TrimSpace(attemptID)
	if !domain.ValidAgentID(attemptID) || expected.Validate() != nil ||
		expected.AttemptID != attemptID ||
		expected.Status != sandbox.DockerProductionEvidenceAttemptLeaseActive {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence attempt failure binding is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerProductionEvidenceAttempt(ctx, tx, attemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Attempt.RunID); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	record, err = getDockerProductionEvidenceAttempt(ctx, tx, attemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	if record.Result != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.New(
			apperror.CodeConflict, "Completed Docker production evidence attempt cannot fail")
	}
	if len(record.Failures) >= sandbox.MaxDockerProductionEvidenceAttemptFailures {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.New(
			apperror.CodeResourceExhausted, "Docker production evidence attempt failure ledger is exhausted")
	}
	if err := requireCurrentDockerProductionEvidenceAttemptLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	if _, found := reconciliationForGeneration(record.Reconciliations, expected.Generation); !found {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence failure requires a durable reconciliation checkpoint")
	}
	failure, err := sandbox.NewDockerProductionEvidenceAttemptFailure(attemptID,
		len(record.Failures)+1, expected.Generation, code, createdAt)
	if err != nil || createdAt.Before(expected.AcquiredAt) || !createdAt.Before(expected.ExpiresAt) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker production evidence attempt failure is invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_attempt_failures
		(attempt_id, sequence, generation, protocol_version, code, failure_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, failure.AttemptID, failure.Sequence,
		failure.Generation, failure.ProtocolVersion, failure.Code,
		failure.FailureFingerprint, ts(failure.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_production_evidence_attempt_leases
		SET status = 'released', released_at = ? WHERE attempt_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(failure.CreatedAt),
		expected.AttemptID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	if err := requireSingleLeaseUpdate(update,
		"Docker production evidence attempt lease changed before failure recording"); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt,
		events.SandboxDockerProductionEvidenceAttemptFailedEvent, failure.CreatedAt,
		map[string]any{"lease_generation": failure.Generation, "failure_code": failure.Code,
			"real_daemon_contacted": false, "container_start_authorized": false,
			"process_execution_authorized": false}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	releasedAt := failure.CreatedAt
	record.Lease.Status, record.Lease.ReleasedAt = sandbox.DockerProductionEvidenceAttemptLeaseReleased,
		&releasedAt
	record.Failures = append(record.Failures, failure)
	return record, record.Validate()
}

func requireCurrentDockerProductionEvidenceAttemptLease(current,
	expected sandbox.DockerProductionEvidenceAttemptLease, now time.Time,
) error {
	if current.AttemptID != expected.AttemptID || current.LeaseID != expected.LeaseID ||
		current.OwnerID != expected.OwnerID || current.Generation != expected.Generation ||
		current.Status != expected.Status || !current.AcquiredAt.Equal(expected.AcquiredAt) ||
		!current.ExpiresAt.Equal(expected.ExpiresAt) || !current.ActiveAt(now) {
		return apperror.New(apperror.CodeConflict,
			"Docker production evidence attempt lease expired or was replaced")
	}
	return nil
}

func reconciliationForGeneration(values []sandbox.DockerProductionEvidenceReconciliation,
	generation int64,
) (sandbox.DockerProductionEvidenceReconciliation, bool) {
	for _, value := range values {
		if value.Generation == generation {
			return value, true
		}
	}
	return sandbox.DockerProductionEvidenceReconciliation{}, false
}

func (s *SQLiteStore) GetDockerProductionEvidenceAttempt(ctx context.Context,
	id string,
) (sandbox.DockerProductionEvidenceAttemptRecord, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence attempt id is invalid")
	}
	return getDockerProductionEvidenceAttempt(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerProductionEvidenceAttemptByOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerProductionEvidenceAttemptRecord, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence attempt operation digest is invalid")
	}
	return getDockerProductionEvidenceAttemptByOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) ListDockerProductionEvidenceAttempts(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerProductionEvidenceAttemptRecord, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker production evidence attempt list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker production evidence attempt list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id
		FROM sandbox_docker_production_evidence_attempts
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
	values := make([]sandbox.DockerProductionEvidenceAttemptRecord, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerProductionEvidenceAttempt(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func insertDockerProductionEvidenceAttemptTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidenceAttempt,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_attempts
		(id, review_id, cleanup_intent_id, run_id, mission_id, workspace_id,
		protocol_version, operation_key_digest, request_fingerprint, review_fingerprint,
		authority_fingerprint, threat_model_fingerprint, suite_fingerprint, endpoint_class,
		endpoint_fingerprint, capture_timeout_millis, operator_confirmed,
		real_daemon_contact_authorized, container_start_authorized,
		process_execution_authorized, output_export_authorized, artifact_commit_authorized,
		attempt_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.ReviewID, value.CleanupIntentID, value.RunID, value.MissionID,
		value.WorkspaceID, value.ProtocolVersion, value.OperationKeyDigest,
		value.RequestFingerprint, value.ReviewFingerprint, value.AuthorityFingerprint,
		value.ThreatModelFingerprint, value.SuiteFingerprint, value.EndpointClass,
		value.EndpointFingerprint, value.CaptureTimeoutMillis, boolInt(value.OperatorConfirmed),
		boolInt(value.RealDaemonContactAuthorized), boolInt(value.ContainerStartAuthorized),
		boolInt(value.ProcessExecutionAuthorized), boolInt(value.OutputExportAuthorized),
		boolInt(value.ArtifactCommitAuthorized), value.AttemptFingerprint,
		value.RequestedBy, ts(value.CreatedAt))
	return err
}

func validateDockerProductionEvidenceAttemptCurrentTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidenceAttempt,
) error {
	review, err := getDockerStartGateReview(ctx, tx, value.ReviewID)
	if err != nil {
		return err
	}
	if review.Replayed || review.Validate() != nil ||
		review.CleanupIntentID != value.CleanupIntentID || review.RunID != value.RunID ||
		review.MissionID != value.MissionID || review.WorkspaceID != value.WorkspaceID ||
		review.ReviewFingerprint != value.ReviewFingerprint ||
		review.AuthorityFingerprint != value.AuthorityFingerprint ||
		review.ThreatModelFingerprint != value.ThreatModelFingerprint ||
		review.RequestedBy != value.RequestedBy || value.CreatedAt.Before(review.CreatedAt) ||
		review.StartGatePassed || review.StartImplementationPresent ||
		review.ContainerStartAuthorized || review.ProcessExecutionAuthorized ||
		review.OutputExportAuthorized || review.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker production evidence attempt v63 review authority changed")
	}
	return nil
}

func getDockerProductionEvidenceAttemptByOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerProductionEvidenceAttemptRecord, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id
		FROM sandbox_docker_production_evidence_attempt_operations WHERE key_digest = ?`,
		keyDigest).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, nil
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	value, err := getDockerProductionEvidenceAttempt(ctx, queryer, id)
	return value, err == nil, err
}

func getDockerProductionEvidenceAttempt(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerProductionEvidenceAttemptRecord, error) {
	attempt, err := scanDockerProductionEvidenceAttempt(queryer.QueryRowContext(ctx,
		dockerProductionEvidenceAttemptSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, apperror.New(
			apperror.CodeNotFound, "Docker production evidence attempt not found")
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	lease, err := getDockerProductionEvidenceAttemptLease(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	reconciliations, err := listDockerProductionEvidenceReconciliations(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	failures, err := listDockerProductionEvidenceAttemptFailures(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	result, found, err := getDockerProductionEvidenceAttemptResult(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, err
	}
	record := sandbox.DockerProductionEvidenceAttemptRecord{Attempt: attempt, Lease: lease,
		Reconciliations: reconciliations, Failures: failures}
	if found {
		record.Result = &result
	}
	return record, record.Validate()
}

func scanDockerProductionEvidenceAttempt(row scanner) (sandbox.DockerProductionEvidenceAttempt, error) {
	var value sandbox.DockerProductionEvidenceAttempt
	var confirmed, daemonAuthorized, startAuthorized, processAuthorized int
	var outputAuthorized, artifactAuthorized int
	var createdAt string
	err := row.Scan(&value.ID, &value.ReviewID, &value.CleanupIntentID, &value.RunID,
		&value.MissionID, &value.WorkspaceID, &value.ProtocolVersion,
		&value.OperationKeyDigest, &value.RequestFingerprint, &value.ReviewFingerprint,
		&value.AuthorityFingerprint, &value.ThreatModelFingerprint, &value.SuiteFingerprint,
		&value.EndpointClass, &value.EndpointFingerprint, &value.CaptureTimeoutMillis,
		&confirmed, &daemonAuthorized, &startAuthorized, &processAuthorized,
		&outputAuthorized, &artifactAuthorized, &value.AttemptFingerprint,
		&value.RequestedBy, &createdAt)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttempt{}, err
	}
	value.OperatorConfirmed, value.RealDaemonContactAuthorized = confirmed != 0, daemonAuthorized != 0
	value.ContainerStartAuthorized, value.ProcessExecutionAuthorized = startAuthorized != 0,
		processAuthorized != 0
	value.OutputExportAuthorized, value.ArtifactCommitAuthorized = outputAuthorized != 0,
		artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	return value, value.Validate()
}

func getDockerProductionEvidenceAttemptLease(ctx context.Context, queryer sandboxLifecycleQueryer,
	attemptID string,
) (sandbox.DockerProductionEvidenceAttemptLease, error) {
	var value sandbox.DockerProductionEvidenceAttemptLease
	var acquiredAt, expiresAt string
	var releasedAt sql.NullString
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, lease_id, owner_id, generation,
		status, acquired_at, expires_at, released_at
		FROM sandbox_docker_production_evidence_attempt_leases WHERE attempt_id = ?`,
		attemptID).Scan(&value.AttemptID, &value.LeaseID, &value.OwnerID, &value.Generation,
		&value.Status, &acquiredAt, &expiresAt, &releasedAt)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptLease{}, err
	}
	value.AcquiredAt, value.ExpiresAt = parseTS(acquiredAt), parseTS(expiresAt)
	if releasedAt.Valid {
		parsed := parseTS(releasedAt.String)
		value.ReleasedAt = &parsed
	}
	return value, value.Validate()
}

func listDockerProductionEvidenceReconciliations(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) ([]sandbox.DockerProductionEvidenceReconciliation, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT attempt_id, generation, previous_generation,
		protocol_version, status, endpoint_class, endpoint_fingerprint,
		real_daemon_contacted, daemon_read_count, reconciled_resource_count,
		reconciliation_fingerprint, created_at
		FROM sandbox_docker_production_evidence_reconciliations
		WHERE attempt_id = ? ORDER BY generation`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sandbox.DockerProductionEvidenceReconciliation, 0)
	for rows.Next() {
		var value sandbox.DockerProductionEvidenceReconciliation
		var contacted int
		var createdAt string
		if err := rows.Scan(&value.AttemptID, &value.Generation, &value.PreviousGeneration,
			&value.ProtocolVersion, &value.Status, &value.EndpointClass,
			&value.EndpointFingerprint, &contacted, &value.DaemonReadCount,
			&value.ReconciledResourceCount, &value.ReconciliationFingerprint,
			&createdAt); err != nil {
			return nil, err
		}
		value.RealDaemonContacted, value.CreatedAt = contacted != 0, parseTS(createdAt)
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func listDockerProductionEvidenceAttemptFailures(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) ([]sandbox.DockerProductionEvidenceAttemptFailure, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT attempt_id, sequence, generation,
		protocol_version, code, failure_fingerprint, created_at
		FROM sandbox_docker_production_evidence_attempt_failures
		WHERE attempt_id = ? ORDER BY sequence`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sandbox.DockerProductionEvidenceAttemptFailure, 0)
	for rows.Next() {
		var value sandbox.DockerProductionEvidenceAttemptFailure
		var createdAt string
		if err := rows.Scan(&value.AttemptID, &value.Sequence, &value.Generation,
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

func getDockerProductionEvidenceAttemptResult(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) (sandbox.DockerProductionEvidenceAttemptResult, bool, error) {
	var value sandbox.DockerProductionEvidenceAttemptResult
	var contacted, startAuthorized, processAuthorized, outputAuthorized, artifactAuthorized int
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, evidence_id, protocol_version,
		status, lease_generation, reconciliation_fingerprint, evidence_capture_fingerprint,
		real_daemon_contacted, container_start_authorized, process_execution_authorized,
		output_export_authorized, artifact_commit_authorized, result_fingerprint, created_at
		FROM sandbox_docker_production_evidence_attempt_results WHERE attempt_id = ?`,
		attemptID).Scan(&value.AttemptID, &value.EvidenceID, &value.ProtocolVersion,
		&value.Status, &value.LeaseGeneration, &value.ReconciliationFingerprint,
		&value.EvidenceCaptureFingerprint, &contacted, &startAuthorized,
		&processAuthorized, &outputAuthorized, &artifactAuthorized,
		&value.ResultFingerprint, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceAttemptResult{}, false, nil
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptResult{}, false, err
	}
	value.RealDaemonContacted, value.ContainerStartAuthorized = contacted != 0,
		startAuthorized != 0
	value.ProcessExecutionAuthorized, value.OutputExportAuthorized = processAuthorized != 0,
		outputAuthorized != 0
	value.ArtifactCommitAuthorized, value.CreatedAt = artifactAuthorized != 0, parseTS(createdAt)
	return value, true, value.Validate()
}

func appendDockerProductionEvidenceAttemptEvent(ctx context.Context, tx *sql.Tx,
	attempt sandbox.DockerProductionEvidenceAttempt, eventType string, createdAt time.Time,
	payload map[string]any,
) error {
	event, err := events.New(attempt.RunID, attempt.MissionID, eventType,
		"sandbox_docker_production_evidence_attempt", attempt.ID, payload)
	if err != nil {
		return err
	}
	event.CreatedAt = createdAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
