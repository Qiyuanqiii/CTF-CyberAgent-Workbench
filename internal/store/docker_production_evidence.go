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

const dockerProductionEvidenceSelect = `SELECT evidence.id, evidence.review_id,
	evidence.cleanup_intent_id, evidence.run_id, evidence.mission_id,
	evidence.workspace_id, evidence.protocol_version, evidence.operation_key_digest,
	evidence.review_fingerprint, evidence.authority_fingerprint,
	evidence.threat_model_fingerprint, evidence.source, evidence.trust_class,
	evidence.status, evidence.platform_class, evidence.endpoint_class,
	evidence.suite_fingerprint, evidence.environment_fingerprint,
	evidence.evidence_fingerprint, evidence.capture_fingerprint,
	evidence.operator_confirmed, evidence.real_daemon_contacted,
	evidence.required_check_count, evidence.observed_count,
	evidence.production_verified_count, evidence.sufficient_check_count,
	evidence.blocker_count, evidence.start_gate_passed,
	evidence.container_start_authorized, evidence.process_execution_authorized,
	evidence.output_export_authorized, evidence.artifact_commit_authorized,
	evidence.requested_by, evidence.created_at
	FROM sandbox_docker_production_evidence evidence
	JOIN sandbox_docker_production_evidence_operations operation
		ON operation.evidence_id = evidence.id`

func (s *SQLiteStore) GetDockerProductionEvidenceOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerProductionEvidenceOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerProductionEvidenceOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence operation digest is invalid")
	}
	return getDockerProductionEvidenceOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateDockerProductionEvidence(ctx context.Context,
	value sandbox.DockerProductionEvidence,
	operation sandbox.DockerProductionEvidenceOperation,
) (sandbox.DockerProductionEvidence, bool, error) {
	if err := validateDockerProductionEvidenceMutation(value, operation); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DockerProductionEvidence{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, value.RunID); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	if existing, found, lookupErr := getDockerProductionEvidenceOperation(ctx, tx,
		operation.KeyDigest); lookupErr != nil {
		return sandbox.DockerProductionEvidence{}, false, lookupErr
	} else if found {
		return replayDockerProductionEvidence(ctx, tx, existing, operation)
	}
	if err := validateDockerProductionEvidenceCurrentTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	if value.CreatedAt.After(time.Now().UTC()) {
		return sandbox.DockerProductionEvidence{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence timestamp is in the future")
	}
	if err := insertDockerProductionEvidenceTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	for _, item := range value.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_items
			(evidence_id, ordinal, name, probe_code, state, observed,
			production_verified, sufficient_for_start, blocker_code, evidence_digest)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, item.Ordinal, item.Name,
			item.ProbeCode, item.State, boolInt(item.Observed),
			boolInt(item.ProductionVerified), boolInt(item.SufficientForStart),
			item.BlockerCode, item.EvidenceDigest); err != nil {
			return sandbox.DockerProductionEvidence{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_operations
		(key_digest, request_fingerprint, evidence_id, review_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.EvidenceID, operation.ReviewID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	if err := appendDockerProductionEvidenceEvent(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	return value, false, nil
}

func validateDockerProductionEvidenceMutation(value sandbox.DockerProductionEvidence,
	operation sandbox.DockerProductionEvidenceOperation,
) error {
	if err := value.Validate(); err != nil || value.Replayed {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker production evidence is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker production evidence operation is invalid", err)
	}
	if operation.KeyDigest != value.OperationKeyDigest || operation.EvidenceID != value.ID ||
		operation.ReviewID != value.ReviewID || operation.RunID != value.RunID ||
		operation.RequestedBy != value.RequestedBy || !operation.CreatedAt.Equal(value.CreatedAt) ||
		operation.RequestFingerprint != sandbox.DockerProductionEvidenceRequestFingerprint(value) {
		return apperror.New(apperror.CodeConflict,
			"Docker production evidence operation does not match its capture")
	}
	return nil
}

func validateDockerProductionEvidenceCurrentTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidence,
) error {
	review, err := getDockerStartGateReview(ctx, tx, value.ReviewID)
	if err != nil {
		return err
	}
	if review.Replayed || review.Validate() != nil || review.CleanupIntentID != value.CleanupIntentID ||
		review.RunID != value.RunID || review.MissionID != value.MissionID ||
		review.WorkspaceID != value.WorkspaceID || review.ReviewFingerprint != value.ReviewFingerprint ||
		review.AuthorityFingerprint != value.AuthorityFingerprint ||
		review.ThreatModelFingerprint != value.ThreatModelFingerprint ||
		review.RequestedBy != value.RequestedBy || value.CreatedAt.Before(review.CreatedAt) ||
		review.StartGatePassed || review.StartImplementationPresent ||
		review.ContainerStartAuthorized || review.ProcessExecutionAuthorized ||
		review.OutputExportAuthorized || review.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker production evidence v63 review authority changed")
	}
	return nil
}

func insertDockerProductionEvidenceTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidence,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence
		(id, review_id, cleanup_intent_id, run_id, mission_id, workspace_id,
		protocol_version, operation_key_digest, review_fingerprint,
		authority_fingerprint, threat_model_fingerprint, source, trust_class, status,
		platform_class, endpoint_class, suite_fingerprint, environment_fingerprint,
		evidence_fingerprint, capture_fingerprint, operator_confirmed,
		real_daemon_contacted, required_check_count, observed_count,
		production_verified_count, sufficient_check_count, blocker_count,
		start_gate_passed, container_start_authorized, process_execution_authorized,
		output_export_authorized, artifact_commit_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.ReviewID, value.CleanupIntentID,
		value.RunID, value.MissionID, value.WorkspaceID, value.ProtocolVersion,
		value.OperationKeyDigest, value.ReviewFingerprint, value.AuthorityFingerprint,
		value.ThreatModelFingerprint, value.Source, value.TrustClass, value.Status,
		value.PlatformClass, value.EndpointClass, value.SuiteFingerprint,
		value.EnvironmentFingerprint, value.EvidenceFingerprint, value.CaptureFingerprint,
		boolInt(value.OperatorConfirmed), boolInt(value.RealDaemonContacted),
		value.RequiredCheckCount, value.ObservedCount, value.ProductionVerifiedCount,
		value.SufficientCheckCount, value.BlockerCount, boolInt(value.StartGatePassed),
		boolInt(value.ContainerStartAuthorized), boolInt(value.ProcessExecutionAuthorized),
		boolInt(value.OutputExportAuthorized), boolInt(value.ArtifactCommitAuthorized),
		value.RequestedBy, ts(value.CreatedAt))
	return err
}

func (s *SQLiteStore) GetDockerProductionEvidence(ctx context.Context,
	id string,
) (sandbox.DockerProductionEvidence, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence id is invalid")
	}
	return getDockerProductionEvidence(ctx, s.db, id)
}

func (s *SQLiteStore) ListDockerProductionEvidence(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerProductionEvidence, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker production evidence list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker production evidence list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT evidence.id
		FROM sandbox_docker_production_evidence evidence
		JOIN sandbox_docker_production_evidence_operations operation
			ON operation.evidence_id = evidence.id
		WHERE evidence.run_id = ? ORDER BY evidence.created_at, evidence.id LIMIT ?`,
		runID, limit)
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
	values := make([]sandbox.DockerProductionEvidence, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerProductionEvidence(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerProductionEvidence(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerProductionEvidence, error) {
	value, err := scanDockerProductionEvidenceRoot(queryer.QueryRowContext(ctx,
		dockerProductionEvidenceSelect+` WHERE evidence.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidence{}, apperror.New(
			apperror.CodeNotFound, "Docker production evidence not found")
	}
	if err != nil {
		return sandbox.DockerProductionEvidence{}, err
	}
	return loadDockerProductionEvidenceItems(ctx, queryer, value)
}

func scanDockerProductionEvidenceRoot(row scanner) (sandbox.DockerProductionEvidence, error) {
	var value sandbox.DockerProductionEvidence
	var confirmed, daemonContacted, gatePassed int
	var startAuthorized, processAuthorized, outputAuthorized, artifactAuthorized int
	var createdAt string
	err := row.Scan(&value.ID, &value.ReviewID, &value.CleanupIntentID, &value.RunID,
		&value.MissionID, &value.WorkspaceID, &value.ProtocolVersion,
		&value.OperationKeyDigest, &value.ReviewFingerprint, &value.AuthorityFingerprint,
		&value.ThreatModelFingerprint, &value.Source, &value.TrustClass, &value.Status,
		&value.PlatformClass, &value.EndpointClass, &value.SuiteFingerprint,
		&value.EnvironmentFingerprint, &value.EvidenceFingerprint,
		&value.CaptureFingerprint, &confirmed, &daemonContacted,
		&value.RequiredCheckCount, &value.ObservedCount, &value.ProductionVerifiedCount,
		&value.SufficientCheckCount, &value.BlockerCount, &gatePassed, &startAuthorized,
		&processAuthorized, &outputAuthorized, &artifactAuthorized, &value.RequestedBy,
		&createdAt)
	if err != nil {
		return sandbox.DockerProductionEvidence{}, err
	}
	value.OperatorConfirmed, value.RealDaemonContacted = confirmed != 0, daemonContacted != 0
	value.StartGatePassed, value.ContainerStartAuthorized = gatePassed != 0,
		startAuthorized != 0
	value.ProcessExecutionAuthorized, value.OutputExportAuthorized = processAuthorized != 0,
		outputAuthorized != 0
	value.ArtifactCommitAuthorized = artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	return value, nil
}

func loadDockerProductionEvidenceItems(ctx context.Context, queryer sandboxLifecycleQueryer,
	value sandbox.DockerProductionEvidence,
) (sandbox.DockerProductionEvidence, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, probe_code, state,
		observed, production_verified, sufficient_for_start, blocker_code, evidence_digest
		FROM sandbox_docker_production_evidence_items WHERE evidence_id = ? ORDER BY ordinal`,
		value.ID)
	if err != nil {
		return sandbox.DockerProductionEvidence{}, err
	}
	for rows.Next() {
		var item sandbox.DockerProductionEvidenceItem
		var observed, verified, sufficient int
		if err := rows.Scan(&item.Ordinal, &item.Name, &item.ProbeCode, &item.State,
			&observed, &verified, &sufficient, &item.BlockerCode,
			&item.EvidenceDigest); err != nil {
			_ = rows.Close()
			return sandbox.DockerProductionEvidence{}, err
		}
		item.Observed, item.ProductionVerified = observed != 0, verified != 0
		item.SufficientForStart = sufficient != 0
		value.Items = append(value.Items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return sandbox.DockerProductionEvidence{}, err
	}
	if err := rows.Close(); err != nil {
		return sandbox.DockerProductionEvidence{}, err
	}
	if err := value.Validate(); err != nil {
		return sandbox.DockerProductionEvidence{}, err
	}
	return value, nil
}

func getDockerProductionEvidenceOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerProductionEvidenceOperation, bool, error) {
	var value sandbox.DockerProductionEvidenceOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT key_digest, request_fingerprint,
		evidence_id, review_id, run_id, requested_by, created_at
		FROM sandbox_docker_production_evidence_operations WHERE key_digest = ?`,
		keyDigest).Scan(&value.KeyDigest, &value.RequestFingerprint, &value.EvidenceID,
		&value.ReviewID, &value.RunID, &value.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceOperation{}, false, nil
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceOperation{}, false, err
	}
	return value, true, nil
}

func replayDockerProductionEvidence(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.DockerProductionEvidenceOperation,
) (sandbox.DockerProductionEvidence, bool, error) {
	if existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.ReviewID != requested.ReviewID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return sandbox.DockerProductionEvidence{}, false, apperror.New(
			apperror.CodeConflict, "Docker production evidence operation key changed request")
	}
	value, err := getDockerProductionEvidence(ctx, tx, existing.EvidenceID)
	if err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidence{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func appendDockerProductionEvidenceEvent(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidence,
) error {
	event, err := events.New(value.RunID, value.MissionID,
		events.SandboxDockerProductionEvidenceCapturedEvent,
		"sandbox_docker_production_evidence", value.ID, map[string]any{
			"status": value.Status, "platform_class": value.PlatformClass,
			"required_check_count":      value.RequiredCheckCount,
			"observed_count":            value.ObservedCount,
			"production_verified_count": value.ProductionVerifiedCount,
			"sufficient_check_count":    0, "blocker_count": value.BlockerCount,
			"real_daemon_contacted": value.RealDaemonContacted,
			"start_gate_passed":     false, "container_start_authorized": false,
			"process_execution_authorized": false, "output_export_authorized": false,
			"artifact_commit_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = value.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
