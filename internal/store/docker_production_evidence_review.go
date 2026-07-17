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

const dockerProductionEvidenceReviewSelect = `SELECT review.id, review.evidence_id,
	review.attempt_id, review.start_gate_review_id, review.run_id, review.mission_id,
	review.workspace_id, review.protocol_version, review.operation_key_digest,
	review.request_fingerprint, review.evidence_operation_key_digest,
	review.evidence_capture_fingerprint,
	review.harness_result_fingerprint, review.authority_fingerprint,
	review.threat_model_fingerprint, review.suite_fingerprint,
	review.environment_fingerprint, review.decision, review.reason_code,
	review.trust_class, review.operator_confirmed, review.receipt_accepted,
	review.real_daemon_contacted, review.required_check_count, review.observed_count,
	review.production_verified_count, review.sufficient_check_count, review.blocker_count,
	review.start_gate_passed, review.container_start_authorized,
	review.process_execution_authorized, review.output_export_authorized,
	review.artifact_commit_authorized, review.review_fingerprint, review.reviewed_by,
	review.created_at
	FROM sandbox_docker_production_evidence_reviews review
	JOIN sandbox_docker_production_evidence_review_operations operation
		ON operation.review_id = review.id`

func (s *SQLiteStore) GetDockerProductionEvidenceReviewOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerProductionEvidenceReviewOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerProductionEvidenceReviewOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker production evidence review operation digest is invalid")
	}
	return getDockerProductionEvidenceReviewOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateDockerProductionEvidenceReview(ctx context.Context,
	value sandbox.DockerProductionEvidenceReview,
	operation sandbox.DockerProductionEvidenceReviewOperation,
) (sandbox.DockerProductionEvidenceReview, bool, error) {
	if err := validateDockerProductionEvidenceReviewMutation(value, operation); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, value.RunID); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if existing, found, lookupErr := getDockerProductionEvidenceReviewOperation(ctx, tx,
		operation.KeyDigest); lookupErr != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, lookupErr
	} else if found {
		return replayDockerProductionEvidenceReview(ctx, tx, existing, operation)
	}
	if existing, found, lookupErr := getDockerProductionEvidenceReviewByEvidence(ctx, tx,
		value.EvidenceID); lookupErr != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, lookupErr
	} else if found {
		return sandbox.DockerProductionEvidenceReview{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence already has an immutable review: "+existing.ID)
	}
	if err := validateDockerProductionEvidenceReviewCurrentTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if value.CreatedAt.After(time.Now().UTC()) {
		return sandbox.DockerProductionEvidenceReview{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker production evidence review timestamp is in the future")
	}
	// The deferred review foreign key lets the operation be written first. The review
	// insert trigger then requires that operation, so neither half can commit alone.
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_review_operations
		(key_digest, request_fingerprint, review_id, evidence_id, attempt_id, run_id,
		reviewed_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.ReviewID, operation.EvidenceID,
		operation.AttemptID, operation.RunID, operation.ReviewedBy,
		ts(operation.CreatedAt)); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if err := insertDockerProductionEvidenceReviewTx(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if err := appendDockerProductionEvidenceReviewEvent(ctx, tx, value); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	return value, false, nil
}

func insertDockerProductionEvidenceReviewTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidenceReview,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_reviews
		(id, evidence_id, attempt_id, start_gate_review_id, run_id, mission_id,
		workspace_id, protocol_version, operation_key_digest,
		request_fingerprint, evidence_operation_key_digest, evidence_capture_fingerprint,
		harness_result_fingerprint, authority_fingerprint, threat_model_fingerprint,
		suite_fingerprint, environment_fingerprint, decision, reason_code, trust_class,
		operator_confirmed, receipt_accepted, real_daemon_contacted,
		required_check_count, observed_count, production_verified_count,
		sufficient_check_count, blocker_count, start_gate_passed,
		container_start_authorized, process_execution_authorized,
		output_export_authorized, artifact_commit_authorized, review_fingerprint,
		reviewed_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.EvidenceID, value.AttemptID,
		value.StartGateReviewID, value.RunID, value.MissionID, value.WorkspaceID,
		value.ProtocolVersion, value.OperationKeyDigest, value.RequestFingerprint,
		value.EvidenceOperationKeyDigest,
		value.EvidenceCaptureFingerprint, value.HarnessResultFingerprint,
		value.AuthorityFingerprint, value.ThreatModelFingerprint, value.SuiteFingerprint,
		value.EnvironmentFingerprint, value.Decision, value.ReasonCode, value.TrustClass,
		boolInt(value.OperatorConfirmed), boolInt(value.ReceiptAccepted),
		boolInt(value.RealDaemonContacted), value.RequiredCheckCount, value.ObservedCount,
		value.ProductionVerifiedCount, value.SufficientCheckCount, value.BlockerCount,
		boolInt(value.StartGatePassed), boolInt(value.ContainerStartAuthorized),
		boolInt(value.ProcessExecutionAuthorized), boolInt(value.OutputExportAuthorized),
		boolInt(value.ArtifactCommitAuthorized), value.ReviewFingerprint,
		value.ReviewedBy, ts(value.CreatedAt))
	return err
}

func validateDockerProductionEvidenceReviewMutation(
	value sandbox.DockerProductionEvidenceReview,
	operation sandbox.DockerProductionEvidenceReviewOperation,
) error {
	if err := value.Validate(); err != nil || value.Replayed {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker production evidence review is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker production evidence review operation is invalid", err)
	}
	if operation.KeyDigest != value.OperationKeyDigest || operation.ReviewID != value.ID ||
		operation.EvidenceID != value.EvidenceID || operation.AttemptID != value.AttemptID ||
		operation.RunID != value.RunID || operation.ReviewedBy != value.ReviewedBy ||
		!operation.CreatedAt.Equal(value.CreatedAt) ||
		operation.RequestFingerprint != value.RequestFingerprint ||
		value.RequestFingerprint != sandbox.DockerProductionEvidenceReviewRequestFingerprint(value) {
		return apperror.New(apperror.CodeConflict,
			"Docker production evidence review operation does not match its decision")
	}
	return nil
}

func validateDockerProductionEvidenceReviewCurrentTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidenceReview,
) error {
	evidence, err := getDockerProductionEvidence(ctx, tx, value.EvidenceID)
	if err != nil {
		return err
	}
	attempt, err := getDockerProductionEvidenceAttempt(ctx, tx, value.AttemptID)
	if err != nil {
		return err
	}
	if evidence.Replayed || attempt.Replayed || evidence.Validate() != nil ||
		attempt.Validate() != nil || attempt.Result != nil || attempt.HarnessResult == nil ||
		attempt.HarnessResult.Validate() != nil ||
		attempt.HarnessResult.EvidenceID != evidence.ID ||
		attempt.HarnessResult.EvidenceCaptureFingerprint != evidence.CaptureFingerprint ||
		evidence.ID != value.EvidenceID || attempt.Attempt.ID != value.AttemptID ||
		evidence.ReviewID != value.StartGateReviewID ||
		evidence.RunID != value.RunID || evidence.MissionID != value.MissionID ||
		evidence.WorkspaceID != value.WorkspaceID ||
		evidence.OperationKeyDigest != value.EvidenceOperationKeyDigest ||
		evidence.CaptureFingerprint != value.EvidenceCaptureFingerprint ||
		attempt.HarnessResult.ResultFingerprint != value.HarnessResultFingerprint ||
		evidence.AuthorityFingerprint != value.AuthorityFingerprint ||
		evidence.ThreatModelFingerprint != value.ThreatModelFingerprint ||
		evidence.SuiteFingerprint != value.SuiteFingerprint ||
		evidence.EnvironmentFingerprint != value.EnvironmentFingerprint ||
		evidence.Status != sandbox.DockerProductionEvidenceStatusComplete ||
		!evidence.RealDaemonContacted || evidence.RequiredCheckCount != sandbox.MaxBackendChecks ||
		evidence.ObservedCount != sandbox.MaxBackendChecks ||
		evidence.ProductionVerifiedCount != 0 || evidence.SufficientCheckCount != 0 ||
		evidence.BlockerCount != sandbox.MaxBackendChecks || evidence.StartGatePassed ||
		evidence.ContainerStartAuthorized || evidence.ProcessExecutionAuthorized ||
		evidence.OutputExportAuthorized || evidence.ArtifactCommitAuthorized ||
		value.CreatedAt.Before(evidence.CreatedAt) ||
		value.CreatedAt.Before(attempt.HarnessResult.CreatedAt) {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Docker production evidence review requires the exact completed v67 harness receipt")
	}
	return nil
}

func (s *SQLiteStore) GetDockerProductionEvidenceReview(ctx context.Context,
	id string,
) (sandbox.DockerProductionEvidenceReview, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker production evidence review id is invalid")
	}
	return getDockerProductionEvidenceReview(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerProductionEvidenceReviewByEvidence(ctx context.Context,
	evidenceID string,
) (sandbox.DockerProductionEvidenceReview, bool, error) {
	evidenceID = strings.TrimSpace(evidenceID)
	if !domain.ValidAgentID(evidenceID) || strings.ContainsRune(evidenceID, 0) {
		return sandbox.DockerProductionEvidenceReview{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker production evidence review evidence id is invalid")
	}
	return getDockerProductionEvidenceReviewByEvidence(ctx, s.db, evidenceID)
}

func (s *SQLiteStore) ListDockerProductionEvidenceReviews(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerProductionEvidenceReview, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker production evidence review list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker production evidence review list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT review.id
		FROM sandbox_docker_production_evidence_reviews review
		JOIN sandbox_docker_production_evidence_review_operations operation
			ON operation.review_id = review.id
		WHERE review.run_id = ? ORDER BY review.created_at, review.id LIMIT ?`, runID, limit)
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
	values := make([]sandbox.DockerProductionEvidenceReview, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerProductionEvidenceReview(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerProductionEvidenceReview(ctx context.Context,
	queryer sandboxLifecycleQueryer, id string,
) (sandbox.DockerProductionEvidenceReview, error) {
	value, err := scanDockerProductionEvidenceReview(queryer.QueryRowContext(ctx,
		dockerProductionEvidenceReviewSelect+` WHERE review.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeNotFound, "Docker production evidence review not found")
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, err
	}
	if err := value.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, err
	}
	operation, found, err := getDockerProductionEvidenceReviewOperation(ctx, queryer,
		value.OperationKeyDigest)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, err
	}
	if !found {
		return sandbox.DockerProductionEvidenceReview{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"stored Docker production evidence review operation is missing")
	}
	if err := validateStoredDockerProductionEvidenceReviewOperation(value, operation); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, err
	}
	return value, nil
}

func getDockerProductionEvidenceReviewByEvidence(ctx context.Context,
	queryer sandboxLifecycleQueryer, evidenceID string,
) (sandbox.DockerProductionEvidenceReview, bool, error) {
	value, err := scanDockerProductionEvidenceReview(queryer.QueryRowContext(ctx,
		dockerProductionEvidenceReviewSelect+` WHERE review.evidence_id = ?`, evidenceID))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceReview{}, false, nil
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if err := value.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	operation, found, err := getDockerProductionEvidenceReviewOperation(ctx, queryer,
		value.OperationKeyDigest)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if !found {
		return sandbox.DockerProductionEvidenceReview{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"stored Docker production evidence review operation is missing")
	}
	if err := validateStoredDockerProductionEvidenceReviewOperation(value, operation); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	return value, true, nil
}

func scanDockerProductionEvidenceReview(row scanner) (
	sandbox.DockerProductionEvidenceReview, error,
) {
	var value sandbox.DockerProductionEvidenceReview
	var confirmed, accepted, contacted, gatePassed int
	var startAuthorized, processAuthorized, outputAuthorized, artifactAuthorized int
	var createdAt string
	err := row.Scan(&value.ID, &value.EvidenceID, &value.AttemptID,
		&value.StartGateReviewID, &value.RunID, &value.MissionID, &value.WorkspaceID,
		&value.ProtocolVersion, &value.OperationKeyDigest, &value.RequestFingerprint,
		&value.EvidenceOperationKeyDigest, &value.EvidenceCaptureFingerprint,
		&value.HarnessResultFingerprint, &value.AuthorityFingerprint,
		&value.ThreatModelFingerprint, &value.SuiteFingerprint,
		&value.EnvironmentFingerprint, &value.Decision, &value.ReasonCode,
		&value.TrustClass, &confirmed, &accepted, &contacted,
		&value.RequiredCheckCount, &value.ObservedCount, &value.ProductionVerifiedCount,
		&value.SufficientCheckCount, &value.BlockerCount, &gatePassed, &startAuthorized,
		&processAuthorized, &outputAuthorized, &artifactAuthorized,
		&value.ReviewFingerprint, &value.ReviewedBy, &createdAt)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, err
	}
	value.OperatorConfirmed, value.ReceiptAccepted = confirmed != 0, accepted != 0
	value.RealDaemonContacted, value.StartGatePassed = contacted != 0, gatePassed != 0
	value.ContainerStartAuthorized, value.ProcessExecutionAuthorized =
		startAuthorized != 0, processAuthorized != 0
	value.OutputExportAuthorized, value.ArtifactCommitAuthorized =
		outputAuthorized != 0, artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	return value, nil
}

func getDockerProductionEvidenceReviewOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerProductionEvidenceReviewOperation, bool, error) {
	var value sandbox.DockerProductionEvidenceReviewOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT key_digest, request_fingerprint,
		review_id, evidence_id, attempt_id, run_id, reviewed_by, created_at
		FROM sandbox_docker_production_evidence_review_operations WHERE key_digest = ?`,
		keyDigest).Scan(&value.KeyDigest, &value.RequestFingerprint, &value.ReviewID,
		&value.EvidenceID, &value.AttemptID, &value.RunID, &value.ReviewedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerProductionEvidenceReviewOperation{}, false, nil
	}
	if err != nil {
		return sandbox.DockerProductionEvidenceReviewOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerProductionEvidenceReviewOperation{}, false, err
	}
	return value, true, nil
}

func replayDockerProductionEvidenceReview(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.DockerProductionEvidenceReviewOperation,
) (sandbox.DockerProductionEvidenceReview, bool, error) {
	value, err := getDockerProductionEvidenceReview(ctx, tx, existing.ReviewID)
	if err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if err := validateStoredDockerProductionEvidenceReviewOperation(value, existing); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	if existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.EvidenceID != requested.EvidenceID || existing.AttemptID != requested.AttemptID ||
		existing.RunID != requested.RunID || existing.ReviewedBy != requested.ReviewedBy {
		return sandbox.DockerProductionEvidenceReview{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker production evidence review operation key changed request")
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerProductionEvidenceReview{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func validateStoredDockerProductionEvidenceReviewOperation(
	value sandbox.DockerProductionEvidenceReview,
	operation sandbox.DockerProductionEvidenceReviewOperation,
) error {
	if operation.KeyDigest != value.OperationKeyDigest ||
		operation.RequestFingerprint != value.RequestFingerprint ||
		value.RequestFingerprint != sandbox.DockerProductionEvidenceReviewRequestFingerprint(value) ||
		operation.ReviewID != value.ID || operation.EvidenceID != value.EvidenceID ||
		operation.AttemptID != value.AttemptID || operation.RunID != value.RunID ||
		operation.ReviewedBy != value.ReviewedBy || !operation.CreatedAt.Equal(value.CreatedAt) {
		return apperror.New(apperror.CodeFailedPrecondition,
			"stored Docker production evidence review operation binding is invalid")
	}
	return nil
}

func appendDockerProductionEvidenceReviewEvent(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerProductionEvidenceReview,
) error {
	event, err := events.New(value.RunID, value.MissionID,
		events.SandboxDockerProductionEvidenceReviewedEvent,
		"sandbox_docker_production_evidence_review", value.ID, map[string]any{
			"evidence_id": value.EvidenceID, "attempt_id": value.AttemptID,
			"decision": value.Decision, "reason_code": value.ReasonCode,
			"receipt_accepted":          value.ReceiptAccepted,
			"required_check_count":      value.RequiredCheckCount,
			"observed_count":            value.ObservedCount,
			"production_verified_count": 0, "sufficient_check_count": 0,
			"blocker_count": value.BlockerCount, "start_gate_passed": false,
			"container_start_authorized":   false,
			"process_execution_authorized": false,
			"output_export_authorized":     false, "artifact_commit_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = value.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
