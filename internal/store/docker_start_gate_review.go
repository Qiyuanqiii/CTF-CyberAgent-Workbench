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

const dockerStartGateReviewSelect = `SELECT review.id, review.cleanup_intent_id,
	review.cleanup_result_id, review.application_intent_id, review.application_result_id,
	review.projection_id, review.container_plan_id, review.preflight_id, review.run_id,
	review.mission_id, review.workspace_id, review.protocol_version,
	review.reviewed_through_schema, review.status, review.decision, review.trust_class,
	review.operation_key_digest, review.manifest_fingerprint,
	review.threat_model_fingerprint, review.cleanup_result_fingerprint,
	review.max_log_bytes, review.authority_fingerprint, review.evidence_fingerprint,
	review.lifecycle_blueprint_fingerprint, review.review_fingerprint,
	review.operator_confirmed, review.real_daemon_chain_verified,
	review.required_check_count, review.production_verified_count,
	review.sufficient_check_count, review.blocker_count, review.start_gate_passed,
	review.start_implementation_present, review.container_start_authorized,
	review.process_execution_authorized, review.output_export_authorized,
	review.artifact_commit_authorized, review.requested_by, review.created_at
	FROM sandbox_docker_start_gate_reviews review
	JOIN sandbox_docker_start_gate_review_operations operation ON operation.review_id = review.id`

func (s *SQLiteStore) GetDockerStartGateReviewOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerStartGateReviewOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerStartGateReviewOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker start-gate review operation digest is invalid")
	}
	return getDockerStartGateReviewOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateDockerStartGateReview(ctx context.Context,
	review sandbox.DockerStartGateReview, operation sandbox.DockerStartGateReviewOperation,
) (sandbox.DockerStartGateReview, bool, error) {
	if err := validateDockerStartGateReviewMutation(review, operation); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DockerStartGateReview{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, review.RunID); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	if existing, found, lookupErr := getDockerStartGateReviewOperation(ctx, tx,
		operation.KeyDigest); lookupErr != nil {
		return sandbox.DockerStartGateReview{}, false, lookupErr
	} else if found {
		return replayDockerStartGateReview(ctx, tx, existing, operation)
	}
	if _, found, lookupErr := getDockerStartGateReviewByCleanup(ctx, tx,
		review.CleanupIntentID); lookupErr != nil {
		return sandbox.DockerStartGateReview{}, false, lookupErr
	} else if found {
		return sandbox.DockerStartGateReview{}, false, apperror.New(
			apperror.CodeConflict, "Docker resource cleanup already has a start-gate review")
	}
	if err := validateDockerStartGateReviewCurrentTx(ctx, tx, review); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	if review.CreatedAt.After(time.Now().UTC()) {
		return sandbox.DockerStartGateReview{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker start-gate review timestamp is in the future")
	}
	if err := insertDockerStartGateReviewTx(ctx, tx, review); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	for _, item := range review.Checks {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_start_gate_review_checks
			(review_id, ordinal, name, evidence_class, evidence_source,
			production_verified, sufficient_for_start, blocker_code, future_gate,
			review_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			review.ID, item.Ordinal, item.Name, item.EvidenceClass, item.EvidenceSource,
			boolInt(item.ProductionVerified), boolInt(item.SufficientForStart), item.BlockerCode,
			item.FutureGate, item.ReviewFingerprint); err != nil {
			return sandbox.DockerStartGateReview{}, false, err
		}
	}
	lifecycle := review.Lifecycle
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_process_lifecycle_blueprints
		(review_id, protocol_version, ownership_model, fixed_endpoint_required,
		write_ahead_required, generation_fenced, cancellation_fanout, bounded_logs,
		max_log_bytes, wait_required, graceful_then_forced_kill, orphan_reconciliation,
		implementation_present, daemon_mutation_enabled, output_commit_authorized,
		blueprint_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		review.ID, lifecycle.ProtocolVersion, lifecycle.OwnershipModel,
		boolInt(lifecycle.FixedEndpointRequired), boolInt(lifecycle.WriteAheadRequired),
		boolInt(lifecycle.GenerationFenced), boolInt(lifecycle.CancellationFanout),
		boolInt(lifecycle.BoundedLogs), lifecycle.MaxLogBytes, boolInt(lifecycle.WaitRequired),
		boolInt(lifecycle.GracefulThenForcedKill), boolInt(lifecycle.OrphanReconciliation),
		boolInt(lifecycle.ImplementationPresent), boolInt(lifecycle.DaemonMutationEnabled),
		boolInt(lifecycle.OutputCommitAuthorized), lifecycle.BlueprintFingerprint); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	for _, transition := range lifecycle.Transitions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_process_lifecycle_transitions
			(review_id, ordinal, from_state, to_state, action, write_ahead_required,
			generation_fenced, daemon_mutation, cancellation_fanout, implemented,
			authorized, transition_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			review.ID, transition.Ordinal, transition.FromState, transition.ToState,
			transition.Action, boolInt(transition.WriteAheadRequired),
			boolInt(transition.GenerationFenced), boolInt(transition.DaemonMutation),
			boolInt(transition.CancellationFanout), boolInt(transition.Implemented),
			boolInt(transition.Authorized), transition.TransitionFingerprint); err != nil {
			return sandbox.DockerStartGateReview{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_start_gate_review_operations
		(key_digest, request_fingerprint, review_id, cleanup_intent_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.ReviewID, operation.CleanupIntentID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	if err := appendDockerStartGateReviewEvent(ctx, tx, review); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	return review, false, nil
}

func validateDockerStartGateReviewMutation(review sandbox.DockerStartGateReview,
	operation sandbox.DockerStartGateReviewOperation,
) error {
	if err := review.Validate(); err != nil || review.Replayed {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker start-gate review is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker start-gate review operation is invalid", err)
	}
	if operation.KeyDigest != review.OperationKeyDigest || operation.ReviewID != review.ID ||
		operation.CleanupIntentID != review.CleanupIntentID || operation.RunID != review.RunID ||
		operation.RequestedBy != review.RequestedBy || !operation.CreatedAt.Equal(review.CreatedAt) ||
		operation.RequestFingerprint != sandbox.DockerStartGateReviewRequestFingerprint(review) {
		return apperror.New(apperror.CodeConflict,
			"Docker start-gate review operation does not match its review")
	}
	return nil
}

func validateDockerStartGateReviewCurrentTx(ctx context.Context, tx *sql.Tx,
	review sandbox.DockerStartGateReview,
) error {
	cleanup, err := getDockerRuntimeInputResourceCleanup(ctx, tx, review.CleanupIntentID)
	if err != nil {
		return err
	}
	if cleanup.Result == nil || cleanup.Replayed || cleanup.Result.Validate() != nil ||
		cleanup.Result.ID != review.CleanupResultID ||
		cleanup.Intent.ApplicationIntentID != review.ApplicationIntentID ||
		cleanup.Intent.ProjectionID != review.ProjectionID ||
		cleanup.Intent.ContainerPlanID != review.ContainerPlanID ||
		cleanup.Intent.RunID != review.RunID || cleanup.Intent.RequestedBy != review.RequestedBy ||
		cleanup.Intent.ManifestFingerprint != review.ManifestFingerprint ||
		cleanup.Result.ResultFingerprint != review.CleanupResultFingerprint ||
		!cleanup.Result.TargetAbsent || !cleanup.Result.AllVolumesAbsent ||
		cleanup.Result.ForeignResourceDetected || cleanup.Result.ContainerStartAuthorized ||
		cleanup.Result.ProcessExecutionAuthorized || cleanup.Result.OutputExportAuthorized ||
		cleanup.Result.ArtifactCommitAuthorized || review.CreatedAt.Before(cleanup.Result.CreatedAt) {
		return apperror.New(apperror.CodeConflict,
			"Docker start-gate review v62 cleanup authority changed")
	}
	application, err := getDockerRuntimeInputApplication(ctx, tx, review.ApplicationIntentID)
	if err != nil {
		return err
	}
	if application.Result == nil || application.Replayed ||
		application.Result.ID != review.ApplicationResultID ||
		application.Intent.ProjectionID != review.ProjectionID ||
		application.Intent.ContainerPlanID != review.ContainerPlanID ||
		application.Intent.RunID != review.RunID || application.Intent.MissionID != review.MissionID ||
		application.Intent.WorkspaceID != review.WorkspaceID ||
		application.Intent.ManifestFingerprint != review.ManifestFingerprint ||
		application.Intent.RequestedBy != review.RequestedBy ||
		application.Result.ContainerStarted || application.Result.ProcessExecuted ||
		application.Result.OutputExported || application.Result.ProductionExecutionSubmitted ||
		application.Result.ProductionVerified || application.Result.BackendEnabled ||
		application.Result.ExecutionAuthorized || application.Result.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker start-gate review v61 application authority changed")
	}
	projection, err := getDockerRuntimeInputProjection(ctx, tx, review.ProjectionID)
	if err != nil {
		return err
	}
	plan, err := getDockerContainerPlan(ctx, tx, review.ContainerPlanID)
	if err != nil {
		return err
	}
	preflight, err := getSandboxDisabledPreflight(ctx, tx, review.PreflightID)
	if err != nil {
		return err
	}
	if projection.ContainerPlanID != plan.ID || projection.RunID != review.RunID ||
		projection.MissionID != review.MissionID || projection.WorkspaceID != review.WorkspaceID ||
		projection.ManifestFingerprint != review.ManifestFingerprint ||
		projection.RequestedBy != review.RequestedBy || projection.ContainerStarted ||
		projection.ProcessExecuted || projection.OutputExported || projection.ProductionVerified ||
		projection.BackendEnabled || projection.ExecutionAuthorized ||
		projection.ArtifactCommitAuthorized || plan.PreflightID != preflight.ID ||
		plan.RunID != review.RunID || plan.MissionID != review.MissionID ||
		plan.WorkspaceID != review.WorkspaceID ||
		plan.ManifestFingerprint != review.ManifestFingerprint || plan.RequestedBy != review.RequestedBy ||
		plan.ProductionVerified || plan.BackendEnabled || plan.ExecutionAuthorized ||
		plan.ArtifactCommitAuthorized || preflight.RunID != review.RunID ||
		preflight.MissionID != review.MissionID || preflight.WorkspaceID != review.WorkspaceID ||
		preflight.ManifestFingerprint != review.ManifestFingerprint ||
		preflight.RequestedBy != review.RequestedBy ||
		preflight.Handshake.ThreatModelFingerprint != review.ThreatModelFingerprint ||
		preflight.OutputPlan.MaxOutputBytes != review.MaxLogBytes || preflight.BackendEnabled ||
		preflight.ExecutionAuthorized || preflight.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker start-gate review v51-v60 authority changed")
	}
	return nil
}

func insertDockerStartGateReviewTx(ctx context.Context, tx *sql.Tx,
	review sandbox.DockerStartGateReview,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_start_gate_reviews
		(id, cleanup_intent_id, cleanup_result_id, application_intent_id,
		application_result_id, projection_id, container_plan_id, preflight_id, run_id,
		mission_id, workspace_id, protocol_version, reviewed_through_schema, status,
		decision, trust_class, operation_key_digest, manifest_fingerprint,
		threat_model_fingerprint, cleanup_result_fingerprint, max_log_bytes,
		authority_fingerprint, evidence_fingerprint, lifecycle_blueprint_fingerprint,
		review_fingerprint, operator_confirmed, real_daemon_chain_verified,
		required_check_count, production_verified_count, sufficient_check_count,
		blocker_count, start_gate_passed, start_implementation_present,
		container_start_authorized, process_execution_authorized,
		output_export_authorized, artifact_commit_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, review.ID, review.CleanupIntentID,
		review.CleanupResultID, review.ApplicationIntentID, review.ApplicationResultID,
		review.ProjectionID, review.ContainerPlanID, review.PreflightID, review.RunID,
		review.MissionID, review.WorkspaceID, review.ProtocolVersion,
		review.ReviewedThroughSchema, review.Status, review.Decision, review.TrustClass,
		review.OperationKeyDigest, review.ManifestFingerprint, review.ThreatModelFingerprint,
		review.CleanupResultFingerprint, review.MaxLogBytes, review.AuthorityFingerprint,
		review.EvidenceFingerprint, review.Lifecycle.BlueprintFingerprint,
		review.ReviewFingerprint, boolInt(review.OperatorConfirmed),
		boolInt(review.RealDaemonChainVerified), review.RequiredCheckCount,
		review.ProductionVerifiedCount, review.SufficientCheckCount, review.BlockerCount,
		boolInt(review.StartGatePassed), boolInt(review.StartImplementationPresent),
		boolInt(review.ContainerStartAuthorized), boolInt(review.ProcessExecutionAuthorized),
		boolInt(review.OutputExportAuthorized), boolInt(review.ArtifactCommitAuthorized),
		review.RequestedBy, ts(review.CreatedAt))
	return err
}

func (s *SQLiteStore) GetDockerStartGateReview(ctx context.Context,
	id string,
) (sandbox.DockerStartGateReview, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerStartGateReview{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker start-gate review id is invalid")
	}
	return getDockerStartGateReview(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerStartGateReviewByCleanup(ctx context.Context,
	cleanupIntentID string,
) (sandbox.DockerStartGateReview, bool, error) {
	cleanupIntentID = strings.TrimSpace(cleanupIntentID)
	if !domain.ValidAgentID(cleanupIntentID) || strings.ContainsRune(cleanupIntentID, 0) {
		return sandbox.DockerStartGateReview{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker start-gate cleanup intent id is invalid")
	}
	return getDockerStartGateReviewByCleanup(ctx, s.db, cleanupIntentID)
}

func (s *SQLiteStore) ListDockerStartGateReviews(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerStartGateReview, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker start-gate review list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker start-gate review list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT review.id
		FROM sandbox_docker_start_gate_reviews review
		JOIN sandbox_docker_start_gate_review_operations operation ON operation.review_id = review.id
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
	values := make([]sandbox.DockerStartGateReview, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerStartGateReview(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerStartGateReview(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerStartGateReview, error) {
	value, err := scanDockerStartGateReviewRoot(queryer.QueryRowContext(ctx,
		dockerStartGateReviewSelect+` WHERE review.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerStartGateReview{}, apperror.New(
			apperror.CodeNotFound, "Docker start-gate review not found")
	}
	if err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	return loadDockerStartGateReviewChildren(ctx, queryer, value)
}

func getDockerStartGateReviewByCleanup(ctx context.Context, queryer sandboxLifecycleQueryer,
	cleanupIntentID string,
) (sandbox.DockerStartGateReview, bool, error) {
	value, err := scanDockerStartGateReviewRoot(queryer.QueryRowContext(ctx,
		dockerStartGateReviewSelect+` WHERE review.cleanup_intent_id = ?`, cleanupIntentID))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerStartGateReview{}, false, nil
	}
	if err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	value, err = loadDockerStartGateReviewChildren(ctx, queryer, value)
	return value, err == nil, err
}

func scanDockerStartGateReviewRoot(row scanner) (sandbox.DockerStartGateReview, error) {
	var value sandbox.DockerStartGateReview
	var operatorConfirmed, realDaemonVerified, gatePassed, implementationPresent int
	var startAuthorized, processAuthorized, outputAuthorized, artifactAuthorized int
	var createdAt string
	err := row.Scan(&value.ID, &value.CleanupIntentID, &value.CleanupResultID,
		&value.ApplicationIntentID, &value.ApplicationResultID, &value.ProjectionID,
		&value.ContainerPlanID, &value.PreflightID, &value.RunID, &value.MissionID,
		&value.WorkspaceID, &value.ProtocolVersion, &value.ReviewedThroughSchema,
		&value.Status, &value.Decision, &value.TrustClass, &value.OperationKeyDigest,
		&value.ManifestFingerprint, &value.ThreatModelFingerprint,
		&value.CleanupResultFingerprint, &value.MaxLogBytes, &value.AuthorityFingerprint,
		&value.EvidenceFingerprint, &value.Lifecycle.BlueprintFingerprint,
		&value.ReviewFingerprint, &operatorConfirmed, &realDaemonVerified,
		&value.RequiredCheckCount, &value.ProductionVerifiedCount,
		&value.SufficientCheckCount, &value.BlockerCount, &gatePassed,
		&implementationPresent, &startAuthorized, &processAuthorized, &outputAuthorized,
		&artifactAuthorized, &value.RequestedBy, &createdAt)
	if err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	value.OperatorConfirmed, value.RealDaemonChainVerified = operatorConfirmed != 0,
		realDaemonVerified != 0
	value.StartGatePassed, value.StartImplementationPresent = gatePassed != 0,
		implementationPresent != 0
	value.ContainerStartAuthorized, value.ProcessExecutionAuthorized = startAuthorized != 0,
		processAuthorized != 0
	value.OutputExportAuthorized, value.ArtifactCommitAuthorized = outputAuthorized != 0,
		artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	return value, nil
}

func loadDockerStartGateReviewChildren(ctx context.Context, queryer sandboxLifecycleQueryer,
	value sandbox.DockerStartGateReview,
) (sandbox.DockerStartGateReview, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, evidence_class,
		evidence_source, production_verified, sufficient_for_start, blocker_code,
		future_gate, review_fingerprint FROM sandbox_docker_start_gate_review_checks
		WHERE review_id = ? ORDER BY ordinal`, value.ID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	for rows.Next() {
		var item sandbox.DockerStartGateCheckReview
		var verified, sufficient int
		if err := rows.Scan(&item.Ordinal, &item.Name, &item.EvidenceClass,
			&item.EvidenceSource, &verified, &sufficient, &item.BlockerCode,
			&item.FutureGate, &item.ReviewFingerprint); err != nil {
			_ = rows.Close()
			return sandbox.DockerStartGateReview{}, err
		}
		item.ProductionVerified, item.SufficientForStart = verified != 0, sufficient != 0
		value.Checks = append(value.Checks, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return sandbox.DockerStartGateReview{}, err
	}
	if err := rows.Close(); err != nil {
		return sandbox.DockerStartGateReview{}, err
	}

	var fixedEndpoint, writeAhead, generationFenced, cancellationFanout, boundedLogs int
	var waitRequired, gracefulKill, orphanReconciliation int
	var implementationPresent, daemonEnabled, outputAuthorized int
	if err := queryer.QueryRowContext(ctx, `SELECT protocol_version, ownership_model,
		fixed_endpoint_required, write_ahead_required, generation_fenced,
		cancellation_fanout, bounded_logs, max_log_bytes, wait_required,
		graceful_then_forced_kill, orphan_reconciliation, implementation_present,
		daemon_mutation_enabled, output_commit_authorized, blueprint_fingerprint
		FROM sandbox_docker_process_lifecycle_blueprints WHERE review_id = ?`, value.ID).Scan(
		&value.Lifecycle.ProtocolVersion, &value.Lifecycle.OwnershipModel, &fixedEndpoint,
		&writeAhead, &generationFenced, &cancellationFanout, &boundedLogs,
		&value.Lifecycle.MaxLogBytes, &waitRequired, &gracefulKill, &orphanReconciliation,
		&implementationPresent, &daemonEnabled, &outputAuthorized,
		&value.Lifecycle.BlueprintFingerprint); err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	value.Lifecycle.FixedEndpointRequired, value.Lifecycle.WriteAheadRequired =
		fixedEndpoint != 0, writeAhead != 0
	value.Lifecycle.GenerationFenced, value.Lifecycle.CancellationFanout =
		generationFenced != 0, cancellationFanout != 0
	value.Lifecycle.BoundedLogs, value.Lifecycle.WaitRequired = boundedLogs != 0,
		waitRequired != 0
	value.Lifecycle.GracefulThenForcedKill, value.Lifecycle.OrphanReconciliation =
		gracefulKill != 0, orphanReconciliation != 0
	value.Lifecycle.ImplementationPresent, value.Lifecycle.DaemonMutationEnabled =
		implementationPresent != 0, daemonEnabled != 0
	value.Lifecycle.OutputCommitAuthorized = outputAuthorized != 0

	transitionRows, err := queryer.QueryContext(ctx, `SELECT ordinal, from_state, to_state,
		action, write_ahead_required, generation_fenced, daemon_mutation,
		cancellation_fanout, implemented, authorized, transition_fingerprint
		FROM sandbox_docker_process_lifecycle_transitions WHERE review_id = ? ORDER BY ordinal`,
		value.ID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	for transitionRows.Next() {
		var transition sandbox.DockerProcessLifecycleTransition
		var write, fenced, mutation, fanout, implemented, authorized int
		if err := transitionRows.Scan(&transition.Ordinal, &transition.FromState,
			&transition.ToState, &transition.Action, &write, &fenced, &mutation,
			&fanout, &implemented, &authorized, &transition.TransitionFingerprint); err != nil {
			_ = transitionRows.Close()
			return sandbox.DockerStartGateReview{}, err
		}
		transition.WriteAheadRequired, transition.GenerationFenced = write != 0, fenced != 0
		transition.DaemonMutation, transition.CancellationFanout = mutation != 0, fanout != 0
		transition.Implemented, transition.Authorized = implemented != 0, authorized != 0
		value.Lifecycle.Transitions = append(value.Lifecycle.Transitions, transition)
	}
	if err := transitionRows.Err(); err != nil {
		_ = transitionRows.Close()
		return sandbox.DockerStartGateReview{}, err
	}
	if err := transitionRows.Close(); err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	if err := value.Validate(); err != nil {
		return sandbox.DockerStartGateReview{}, err
	}
	return value, nil
}

func getDockerStartGateReviewOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerStartGateReviewOperation, bool, error) {
	var value sandbox.DockerStartGateReviewOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT key_digest, request_fingerprint,
		review_id, cleanup_intent_id, run_id, requested_by, created_at
		FROM sandbox_docker_start_gate_review_operations WHERE key_digest = ?`, keyDigest).Scan(
		&value.KeyDigest, &value.RequestFingerprint, &value.ReviewID,
		&value.CleanupIntentID, &value.RunID, &value.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerStartGateReviewOperation{}, false, nil
	}
	if err != nil {
		return sandbox.DockerStartGateReviewOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerStartGateReviewOperation{}, false, err
	}
	return value, true, nil
}

func replayDockerStartGateReview(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.DockerStartGateReviewOperation,
) (sandbox.DockerStartGateReview, bool, error) {
	if existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.CleanupIntentID != requested.CleanupIntentID ||
		existing.RunID != requested.RunID || existing.RequestedBy != requested.RequestedBy {
		return sandbox.DockerStartGateReview{}, false, apperror.New(
			apperror.CodeConflict, "Docker start-gate review operation key changed request")
	}
	value, err := getDockerStartGateReview(ctx, tx, existing.ReviewID)
	if err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerStartGateReview{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func appendDockerStartGateReviewEvent(ctx context.Context, tx *sql.Tx,
	review sandbox.DockerStartGateReview,
) error {
	event, err := events.New(review.RunID, review.MissionID,
		events.SandboxDockerStartGateReviewedEvent, "sandbox_docker_start_gate_review",
		review.ID, map[string]any{
			"status": review.Status, "decision": review.Decision,
			"reviewed_through_schema":   review.ReviewedThroughSchema,
			"required_check_count":      review.RequiredCheckCount,
			"production_verified_count": review.ProductionVerifiedCount,
			"blocker_count":             review.BlockerCount, "start_gate_passed": false,
			"start_implementation_present": false, "container_start_authorized": false,
			"process_execution_authorized": false, "artifact_commit_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = review.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
