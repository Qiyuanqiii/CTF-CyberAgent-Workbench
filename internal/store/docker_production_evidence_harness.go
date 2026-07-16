package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/sandbox"
)

func (s *SQLiteStore) PrepareDockerProductionEvidenceHarnessIntent(ctx context.Context,
	value sandbox.DockerProductionEvidenceHarnessIntent,
	expected sandbox.DockerProductionEvidenceAttemptLease,
) (sandbox.DockerProductionEvidenceAttemptRecord, bool, error) {
	if value.Validate() != nil || expected.Validate() != nil ||
		value.AttemptID != expected.AttemptID ||
		expected.Status != sandbox.DockerProductionEvidenceAttemptLeaseActive {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker production evidence harness intent binding is invalid")
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
	if _, completed := record.CompletedEvidenceID(); completed {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeConflict,
			"Completed Docker production evidence attempt cannot prepare a harness")
	}
	if err := requireCurrentDockerProductionEvidenceAttemptLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	control, found := reconciliationForGeneration(record.Reconciliations,
		expected.Generation)
	if !found {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence harness requires the durable control reconciliation")
	}
	if record.HarnessIntent != nil {
		if record.HarnessIntent.IntentFingerprint != value.IntentFingerprint {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
				apperror.CodeConflict,
				"Docker production evidence harness intent is already immutable")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
		}
		record.Replayed = true
		return record, true, nil
	}
	if value.ReviewID != record.Attempt.ReviewID ||
		value.RunID != record.Attempt.RunID ||
		value.EndpointClass != record.Attempt.EndpointClass ||
		value.EndpointFingerprint != record.Attempt.EndpointFingerprint ||
		value.RequestedBy != record.Attempt.RequestedBy ||
		value.CreatedAt.Before(control.CreatedAt) ||
		!value.CreatedAt.Before(expected.ExpiresAt) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence harness changed its write-ahead attempt")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_harness_intents
		(attempt_id, review_id, container_plan_id, run_id, protocol_version, image_digest,
		endpoint_class, endpoint_fingerprint, label_selector_fingerprint, max_daemon_reads,
		operator_confirmed, readonly_daemon_contact_authorized, daemon_write_authorized,
		container_start_authorized, process_execution_authorized, output_export_authorized,
		artifact_commit_authorized, intent_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.AttemptID, value.ReviewID, value.ContainerPlanID, value.RunID,
		value.ProtocolVersion, value.ImageDigest, value.EndpointClass,
		value.EndpointFingerprint, value.LabelSelectorFingerprint, value.MaxDaemonReads,
		boolInt(value.OperatorConfirmed), boolInt(value.ReadOnlyDaemonContactAuthorized),
		boolInt(value.DaemonWriteAuthorized), boolInt(value.ContainerStartAuthorized),
		boolInt(value.ProcessExecutionAuthorized), boolInt(value.OutputExportAuthorized),
		boolInt(value.ArtifactCommitAuthorized), value.IntentFingerprint,
		value.RequestedBy, ts(value.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt,
		events.SandboxDockerProductionEvidenceHarnessPreparedEvent, value.CreatedAt,
		map[string]any{
			"lease_generation":                    expected.Generation,
			"endpoint_class":                      value.EndpointClass,
			"max_daemon_reads":                    value.MaxDaemonReads,
			"read_only_daemon_contact_authorized": true,
			"daemon_write_authorized":             false,
			"container_start_authorized":          false,
			"process_execution_authorized":        false,
		}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	record.HarnessIntent = &value
	return record, false, record.Validate()
}

func (s *SQLiteStore) RecordDockerProductionEvidenceHarnessReconciliation(
	ctx context.Context,
	value sandbox.DockerProductionEvidenceHarnessReconciliation,
	expected sandbox.DockerProductionEvidenceAttemptLease,
) (sandbox.DockerProductionEvidenceAttemptRecord, bool, error) {
	if value.Validate() != nil || expected.Validate() != nil ||
		value.AttemptID != expected.AttemptID || value.Generation != expected.Generation ||
		expected.Status != sandbox.DockerProductionEvidenceAttemptLeaseActive {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker production evidence harness reconciliation binding is invalid")
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
	if _, completed := record.CompletedEvidenceID(); completed {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeConflict,
			"Completed Docker production evidence attempt cannot reconcile a harness")
	}
	if err := requireCurrentDockerProductionEvidenceAttemptLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if record.HarnessIntent == nil ||
		record.HarnessIntent.IntentFingerprint != value.IntentFingerprint {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"Docker production evidence harness reconciliation requires its durable intent")
	}
	control, found := reconciliationForGeneration(record.Reconciliations,
		expected.Generation)
	if !found || control.ReconciliationFingerprint !=
		value.ControlReconciliationFingerprint {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence harness control reconciliation changed")
	}
	if existing, found := harnessReconciliationForGeneration(
		record.HarnessReconciliations, value.Generation); found {
		if existing.ReconciliationFingerprint != value.ReconciliationFingerprint {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
				apperror.CodeConflict,
				"Docker production evidence harness reconciliation is already immutable")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
		}
		record.Replayed = true
		return record, true, nil
	}
	if value.EndpointClass != record.HarnessIntent.EndpointClass ||
		value.EndpointFingerprint != record.HarnessIntent.EndpointFingerprint ||
		value.CreatedAt.Before(control.CreatedAt) ||
		value.CreatedAt.Before(record.HarnessIntent.CreatedAt) ||
		!value.CreatedAt.Before(expected.ExpiresAt) {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence harness reconciliation changed durable intent")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_harness_reconciliations
		(attempt_id, generation, protocol_version, status, intent_fingerprint,
		control_reconciliation_fingerprint, endpoint_class, endpoint_fingerprint,
		inventory_fingerprint, real_daemon_contacted, daemon_read_count,
		owned_resource_count, reconciliation_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.AttemptID,
		value.Generation, value.ProtocolVersion, value.Status, value.IntentFingerprint,
		value.ControlReconciliationFingerprint, value.EndpointClass,
		value.EndpointFingerprint, value.InventoryFingerprint,
		boolInt(value.RealDaemonContacted), value.DaemonReadCount,
		value.OwnedResourceCount, value.ReconciliationFingerprint,
		ts(value.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt,
		events.SandboxDockerProductionEvidenceHarnessReconciledEvent, value.CreatedAt,
		map[string]any{
			"lease_generation":           value.Generation,
			"status":                     value.Status,
			"real_daemon_contacted":      true,
			"daemon_read_count":          value.DaemonReadCount,
			"owned_resource_count":       0,
			"daemon_write_authorized":    false,
			"container_start_authorized": false,
		}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{}, false, err
	}
	record.HarnessReconciliations = append(record.HarnessReconciliations, value)
	return record, false, record.Validate()
}

func (s *SQLiteStore) CompleteDockerProductionEvidenceHarnessAttempt(ctx context.Context,
	value sandbox.DockerProductionEvidence,
	operation sandbox.DockerProductionEvidenceOperation,
	result sandbox.DockerProductionEvidenceHarnessResult,
	expected sandbox.DockerProductionEvidenceAttemptLease,
) (sandbox.DockerProductionEvidenceAttemptRecord, sandbox.DockerProductionEvidence, bool, error) {
	mutationErr := validateDockerProductionEvidenceMutation(value, operation)
	if mutationErr != nil || result.Validate() != nil || expected.Validate() != nil ||
		result.AttemptID != expected.AttemptID || result.EvidenceID != value.ID ||
		result.LeaseGeneration != expected.Generation ||
		expected.Status != sandbox.DockerProductionEvidenceAttemptLeaseActive {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, apperror.Wrap(
				apperror.CodeInvalidArgument,
				"Docker production evidence harness completion is invalid", mutationErr)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := getDockerProductionEvidenceAttempt(ctx, tx, result.AttemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if err := acquireSandboxManifestWriteLock(ctx, tx, record.Attempt.RunID); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	record, err = getDockerProductionEvidenceAttempt(ctx, tx, result.AttemptID)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if record.HarnessResult != nil {
		if record.HarnessResult.ResultFingerprint != result.ResultFingerprint {
			return sandbox.DockerProductionEvidenceAttemptRecord{},
				sandbox.DockerProductionEvidence{}, false, apperror.New(
					apperror.CodeConflict,
					"Docker production evidence harness result is already immutable")
		}
		existing, loadErr := getDockerProductionEvidence(ctx, tx,
			record.HarnessResult.EvidenceID)
		if loadErr != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{},
				sandbox.DockerProductionEvidence{}, false, loadErr
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{},
				sandbox.DockerProductionEvidence{}, false, err
		}
		record.Replayed, existing.Replayed = true, true
		return record, existing, true, nil
	}
	if record.Result != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, apperror.New(
				apperror.CodeConflict,
				"Docker production evidence attempt already has an inert result")
	}
	if err := requireCurrentDockerProductionEvidenceAttemptLease(record.Lease, expected,
		time.Now().UTC()); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if record.HarnessIntent == nil ||
		record.HarnessIntent.IntentFingerprint != result.IntentFingerprint {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, apperror.New(
				apperror.CodeFailedPrecondition,
				"Docker production evidence harness result requires its durable intent")
	}
	reconciliation, found := harnessReconciliationForGeneration(
		record.HarnessReconciliations, expected.Generation)
	if !found || reconciliation.ReconciliationFingerprint !=
		result.ReconciliationFingerprint || record.Attempt.ReviewID != value.ReviewID ||
		record.Attempt.RunID != value.RunID ||
		record.Attempt.OperationKeyDigest != value.OperationKeyDigest ||
		record.Attempt.RequestFingerprint != operation.RequestFingerprint ||
		record.Attempt.RequestedBy != value.RequestedBy ||
		result.EvidenceCaptureFingerprint != value.CaptureFingerprint ||
		value.CreatedAt.Before(reconciliation.CreatedAt) ||
		!value.CreatedAt.Before(expected.ExpiresAt) {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, apperror.New(
				apperror.CodeConflict,
				"Docker production evidence changed its harness checkpoint")
	}
	if err := validateDockerProductionEvidenceCurrentTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if err := insertDockerProductionEvidenceTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	for _, item := range value.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_items
			(evidence_id, ordinal, name, probe_code, state, observed,
			production_verified, sufficient_for_start, blocker_code, evidence_digest)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, item.Ordinal, item.Name,
			item.ProbeCode, item.State, boolInt(item.Observed),
			boolInt(item.ProductionVerified), boolInt(item.SufficientForStart),
			item.BlockerCode, item.EvidenceDigest); err != nil {
			return sandbox.DockerProductionEvidenceAttemptRecord{},
				sandbox.DockerProductionEvidence{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_harness_results
		(attempt_id, evidence_id, protocol_version, status, lease_generation,
		intent_fingerprint, reconciliation_fingerprint, evidence_capture_fingerprint,
		daemon_read_count, probe_count, observed_count, production_verified_count,
		real_daemon_contacted, daemon_write_authorized, container_start_authorized,
		process_execution_authorized, output_export_authorized,
		artifact_commit_authorized, result_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.AttemptID, result.EvidenceID, result.ProtocolVersion, result.Status,
		result.LeaseGeneration, result.IntentFingerprint,
		result.ReconciliationFingerprint, result.EvidenceCaptureFingerprint,
		result.DaemonReadCount, result.ProbeCount, result.ObservedCount,
		result.ProductionVerifiedCount, boolInt(result.RealDaemonContacted),
		boolInt(result.DaemonWriteAuthorized), boolInt(result.ContainerStartAuthorized),
		boolInt(result.ProcessExecutionAuthorized), boolInt(result.OutputExportAuthorized),
		boolInt(result.ArtifactCommitAuthorized), result.ResultFingerprint,
		ts(result.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_operations
		(key_digest, request_fingerprint, evidence_id, review_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.EvidenceID, operation.ReviewID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE sandbox_docker_production_evidence_attempt_leases
		SET status = 'released', released_at = ? WHERE attempt_id = ? AND lease_id = ?
		AND owner_id = ? AND generation = ? AND status = 'active'`, ts(result.CreatedAt),
		expected.AttemptID, expected.LeaseID, expected.OwnerID, expected.Generation)
	if err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if err := requireSingleLeaseUpdate(update,
		"Docker production evidence harness lease changed before completion"); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if err := appendDockerProductionEvidenceEvent(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt,
		events.SandboxDockerProductionEvidenceHarnessCompletedEvent, result.CreatedAt,
		map[string]any{
			"lease_generation":             result.LeaseGeneration,
			"evidence_id":                  result.EvidenceID,
			"daemon_read_count":            result.DaemonReadCount,
			"probe_count":                  result.ProbeCount,
			"observed_count":               result.ObservedCount,
			"production_verified_count":    result.ProductionVerifiedCount,
			"real_daemon_contacted":        true,
			"daemon_write_authorized":      false,
			"container_start_authorized":   false,
			"process_execution_authorized": false,
		}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if err := appendDockerProductionEvidenceAttemptEvent(ctx, tx, record.Attempt,
		events.SandboxDockerProductionEvidenceAttemptCompletedEvent, result.CreatedAt,
		map[string]any{
			"lease_generation":             result.LeaseGeneration,
			"evidence_id":                  result.EvidenceID,
			"real_daemon_contacted":        true,
			"container_start_authorized":   false,
			"process_execution_authorized": false,
		}); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceAttemptRecord{},
			sandbox.DockerProductionEvidence{}, false, err
	}
	releasedAt := result.CreatedAt
	record.Lease.Status = sandbox.DockerProductionEvidenceAttemptLeaseReleased
	record.Lease.ReleasedAt = &releasedAt
	record.HarnessResult = &result
	return record, value, false, record.Validate()
}

func harnessReconciliationForGeneration(
	values []sandbox.DockerProductionEvidenceHarnessReconciliation, generation int64,
) (sandbox.DockerProductionEvidenceHarnessReconciliation, bool) {
	for _, value := range values {
		if value.Generation == generation {
			return value, true
		}
	}
	return sandbox.DockerProductionEvidenceHarnessReconciliation{}, false
}

func getDockerProductionEvidenceHarnessIntent(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) (sandbox.DockerProductionEvidenceHarnessIntent, bool, error) {
	var value sandbox.DockerProductionEvidenceHarnessIntent
	var confirmed, readAuthorized, writeAuthorized, startAuthorized int
	var processAuthorized, outputAuthorized, artifactAuthorized int
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, review_id, container_plan_id,
		run_id, protocol_version, image_digest, endpoint_class, endpoint_fingerprint,
		label_selector_fingerprint, max_daemon_reads, operator_confirmed,
		readonly_daemon_contact_authorized, daemon_write_authorized,
		container_start_authorized, process_execution_authorized,
		output_export_authorized, artifact_commit_authorized, intent_fingerprint,
		requested_by, created_at
		FROM sandbox_docker_production_evidence_harness_intents WHERE attempt_id = ?`,
		attemptID).Scan(&value.AttemptID, &value.ReviewID, &value.ContainerPlanID,
		&value.RunID, &value.ProtocolVersion, &value.ImageDigest, &value.EndpointClass,
		&value.EndpointFingerprint, &value.LabelSelectorFingerprint,
		&value.MaxDaemonReads, &confirmed, &readAuthorized, &writeAuthorized,
		&startAuthorized, &processAuthorized, &outputAuthorized, &artifactAuthorized,
		&value.IntentFingerprint, &value.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceHarnessIntent{}, false, nil
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceHarnessIntent{}, false, err
	}
	value.OperatorConfirmed = confirmed != 0
	value.ReadOnlyDaemonContactAuthorized = readAuthorized != 0
	value.DaemonWriteAuthorized = writeAuthorized != 0
	value.ContainerStartAuthorized = startAuthorized != 0
	value.ProcessExecutionAuthorized = processAuthorized != 0
	value.OutputExportAuthorized = outputAuthorized != 0
	value.ArtifactCommitAuthorized = artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	return value, true, value.Validate()
}

func listDockerProductionEvidenceHarnessReconciliations(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) ([]sandbox.DockerProductionEvidenceHarnessReconciliation, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT attempt_id, generation,
		protocol_version, status, intent_fingerprint,
		control_reconciliation_fingerprint, endpoint_class, endpoint_fingerprint,
		inventory_fingerprint, real_daemon_contacted, daemon_read_count,
		owned_resource_count, reconciliation_fingerprint, created_at
		FROM sandbox_docker_production_evidence_harness_reconciliations
		WHERE attempt_id = ? ORDER BY generation`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sandbox.DockerProductionEvidenceHarnessReconciliation, 0)
	for rows.Next() {
		var value sandbox.DockerProductionEvidenceHarnessReconciliation
		var contacted int
		var createdAt string
		if err := rows.Scan(&value.AttemptID, &value.Generation,
			&value.ProtocolVersion, &value.Status, &value.IntentFingerprint,
			&value.ControlReconciliationFingerprint, &value.EndpointClass,
			&value.EndpointFingerprint, &value.InventoryFingerprint, &contacted,
			&value.DaemonReadCount, &value.OwnedResourceCount,
			&value.ReconciliationFingerprint, &createdAt); err != nil {
			return nil, err
		}
		value.RealDaemonContacted = contacted != 0
		value.CreatedAt = parseTS(createdAt)
		if err := value.Validate(); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func getDockerProductionEvidenceHarnessResult(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) (sandbox.DockerProductionEvidenceHarnessResult, bool, error) {
	var value sandbox.DockerProductionEvidenceHarnessResult
	var contacted, writeAuthorized, startAuthorized, processAuthorized int
	var outputAuthorized, artifactAuthorized int
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT attempt_id, evidence_id,
		protocol_version, status, lease_generation, intent_fingerprint,
		reconciliation_fingerprint, evidence_capture_fingerprint, daemon_read_count,
		probe_count, observed_count, production_verified_count, real_daemon_contacted,
		daemon_write_authorized, container_start_authorized,
		process_execution_authorized, output_export_authorized,
		artifact_commit_authorized, result_fingerprint, created_at
		FROM sandbox_docker_production_evidence_harness_results WHERE attempt_id = ?`,
		attemptID).Scan(&value.AttemptID, &value.EvidenceID, &value.ProtocolVersion,
		&value.Status, &value.LeaseGeneration, &value.IntentFingerprint,
		&value.ReconciliationFingerprint, &value.EvidenceCaptureFingerprint,
		&value.DaemonReadCount, &value.ProbeCount, &value.ObservedCount,
		&value.ProductionVerifiedCount, &contacted, &writeAuthorized,
		&startAuthorized, &processAuthorized, &outputAuthorized, &artifactAuthorized,
		&value.ResultFingerprint, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceHarnessResult{}, false, nil
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceHarnessResult{}, false, err
	}
	value.RealDaemonContacted = contacted != 0
	value.DaemonWriteAuthorized = writeAuthorized != 0
	value.ContainerStartAuthorized = startAuthorized != 0
	value.ProcessExecutionAuthorized = processAuthorized != 0
	value.OutputExportAuthorized = outputAuthorized != 0
	value.ArtifactCommitAuthorized = artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	return value, true, value.Validate()
}
