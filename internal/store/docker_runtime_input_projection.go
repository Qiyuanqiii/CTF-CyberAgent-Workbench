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

const dockerRuntimeInputProjectionSelect = `SELECT plan.id, plan.handoff_id,
	plan.handoff_intent_id, plan.attempt_id, plan.container_plan_id, plan.run_id,
	plan.mission_id, plan.workspace_id, plan.protocol_version, plan.status,
	plan.trust_class, plan.operation_key_digest, plan.manifest_fingerprint,
	plan.mount_binding_fingerprint, plan.input_artifact_digest,
	plan.authority_fingerprint, plan.spec_fingerprint,
	plan.container_plan_fingerprint, plan.handoff_fingerprint,
	plan.handoff_transport_fingerprint, plan.bundle_report_fingerprint,
	plan.bundle_digest, plan.bundle_bytes, plan.read_only_mount_count,
	plan.input_artifact_count, plan.projection_count, plan.directory_root_count,
	plan.file_root_count, plan.total_entry_count, plan.total_content_bytes,
	plan.total_projection_bytes, plan.projection_set_fingerprint,
	plan.request_fingerprint, plan.projection_fingerprint,
	plan.operator_confirmed,
	plan.exact_target_binding, plan.all_volumes_read_only,
	plan.all_volumes_no_copy, plan.bundle_recaptured, plan.bundle_digest_matched,
	plan.daemon_contacted, plan.daemon_applied, plan.container_started,
	plan.process_executed, plan.output_exported,
	plan.production_execution_submitted, plan.production_verified,
	plan.backend_enabled, plan.execution_authorized,
	plan.artifact_commit_authorized, plan.requested_by, plan.created_at
	FROM sandbox_docker_runtime_input_projection_plans plan
	JOIN sandbox_docker_runtime_input_projection_completions completion
		ON completion.projection_id = plan.id`

func (s *SQLiteStore) GetDockerRuntimeInputProjectionOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerRuntimeInputProjectionOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerRuntimeInputProjectionOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker runtime input projection operation digest is invalid")
	}
	return getDockerRuntimeInputProjectionOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateDockerRuntimeInputProjectionPlan(ctx context.Context,
	plan sandbox.DockerRuntimeInputProjectionPlan,
	operation sandbox.DockerRuntimeInputProjectionOperation,
) (sandbox.DockerRuntimeInputProjectionPlan, bool, error) {
	if err := validateDockerRuntimeInputProjectionMutation(plan, operation); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, plan.RunID); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	if existing, found, err := getDockerRuntimeInputProjectionOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	} else if found {
		return replayDockerRuntimeInputProjection(ctx, tx, existing, operation)
	}
	if _, found, err := getDockerRuntimeInputProjectionByHandoff(ctx, tx,
		plan.HandoffID); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	} else if found {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker host input handoff already has a different runtime projection plan")
	}
	if err := validateDockerRuntimeInputProjectionCurrentTx(ctx, tx, plan); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	if plan.CreatedAt.After(time.Now().UTC()) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker runtime input projection timestamp is in the future")
	}
	if err := insertDockerRuntimeInputProjectionPlanTx(ctx, tx, plan); err != nil {
		_ = tx.Rollback()
		return s.recoverDockerRuntimeInputProjectionCreate(ctx, operation, err)
	}
	for _, item := range plan.Items {
		if err := insertDockerRuntimeInputProjectionItemTx(ctx, tx, plan.ID, item); err != nil {
			return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO
		sandbox_docker_runtime_input_projection_completions
		(projection_id, projection_fingerprint, projection_count, total_entry_count,
		total_content_bytes, total_projection_bytes, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, plan.ID, plan.ProjectionFingerprint,
		plan.ProjectionCount, plan.TotalEntryCount, plan.TotalContentBytes,
		plan.TotalProjectionBytes, ts(plan.CreatedAt)); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO
		sandbox_docker_runtime_input_projection_operations
		(key_digest, projection_id, handoff_id, container_plan_id, run_id,
		request_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest, plan.ID,
		plan.HandoffID, plan.ContainerPlanID, plan.RunID, plan.RequestFingerprint,
		plan.RequestedBy, ts(plan.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverDockerRuntimeInputProjectionCreate(ctx, operation, err)
	}
	if err := appendDockerRuntimeInputProjectionEvent(ctx, tx, plan); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverDockerRuntimeInputProjectionCreate(ctx, operation, err)
	}
	return plan, false, nil
}

func (s *SQLiteStore) GetDockerRuntimeInputProjectionPlan(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputProjectionPlan, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input projection id is invalid")
	}
	return getDockerRuntimeInputProjection(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerRuntimeInputProjectionPlanByHandoff(ctx context.Context,
	handoffID string,
) (sandbox.DockerRuntimeInputProjectionPlan, bool, error) {
	handoffID = strings.TrimSpace(handoffID)
	if !domain.ValidAgentID(handoffID) || strings.ContainsRune(handoffID, 0) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker runtime input projection handoff id is invalid")
	}
	return getDockerRuntimeInputProjectionByHandoff(ctx, s.db, handoffID)
}

func (s *SQLiteStore) ListDockerRuntimeInputProjectionPlans(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputProjectionPlan, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) ||
		limit < 1 || limit > 1000 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker runtime input projection list request is invalid")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT plan.id
		FROM sandbox_docker_runtime_input_projection_plans plan
		JOIN sandbox_docker_runtime_input_projection_completions completion
			ON completion.projection_id = plan.id
		WHERE plan.run_id = ? ORDER BY plan.created_at DESC, plan.id DESC LIMIT ?`,
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
	values := make([]sandbox.DockerRuntimeInputProjectionPlan, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerRuntimeInputProjection(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func validateDockerRuntimeInputProjectionMutation(
	plan sandbox.DockerRuntimeInputProjectionPlan,
	operation sandbox.DockerRuntimeInputProjectionOperation,
) error {
	if err := plan.Validate(); err != nil || plan.Replayed {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker runtime input projection plan is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker runtime input projection operation is invalid", err)
	}
	if operation.KeyDigest != plan.OperationKeyDigest ||
		operation.ProjectionID != plan.ID || operation.HandoffID != plan.HandoffID ||
		operation.ContainerPlanID != plan.ContainerPlanID || operation.RunID != plan.RunID ||
		operation.RequestFingerprint != plan.RequestFingerprint ||
		operation.RequestedBy != plan.RequestedBy ||
		!operation.CreatedAt.Equal(plan.CreatedAt) {
		return apperror.New(apperror.CodeConflict,
			"Docker runtime input projection operation does not match its request")
	}
	return nil
}

func validateDockerRuntimeInputProjectionCurrentTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerRuntimeInputProjectionPlan,
) error {
	handoff, err := getDockerHostInputHandoffRecord(ctx, tx, value.HandoffIntentID)
	if err != nil {
		return err
	}
	attempt, err := getDockerContainerRehearsalAttempt(ctx, tx, value.AttemptID)
	if err != nil {
		return err
	}
	containerPlan, err := getDockerContainerPlan(ctx, tx, value.ContainerPlanID)
	if err != nil {
		return err
	}
	if err := validateDockerContainerPlanCurrentTx(ctx, tx, containerPlan); err != nil {
		return err
	}
	if handoff.Handoff == nil || attempt.Completion == nil ||
		attempt.Status != sandbox.DockerContainerAttemptStatusCompleted ||
		handoff.Intent.ID != value.HandoffIntentID ||
		handoff.Handoff.ID != value.HandoffID ||
		handoff.Intent.AttemptID != value.AttemptID ||
		handoff.Intent.PlanID != value.ContainerPlanID ||
		attempt.Intent.PlanID != value.ContainerPlanID ||
		containerPlan.RunID != value.RunID || containerPlan.MissionID != value.MissionID ||
		containerPlan.WorkspaceID != value.WorkspaceID ||
		containerPlan.ManifestFingerprint != value.ManifestFingerprint ||
		containerPlan.MountBindingFingerprint != value.MountBindingFingerprint ||
		containerPlan.InputArtifactDigest != value.InputArtifactDigest ||
		containerPlan.AuthorityFingerprint != value.AuthorityFingerprint ||
		containerPlan.SpecFingerprint != value.SpecFingerprint ||
		containerPlan.PlanFingerprint != value.ContainerPlanFingerprint ||
		handoff.Handoff.HandoffFingerprint != value.HandoffFingerprint ||
		handoff.Handoff.Result.TransportFingerprint != value.HandoffTransportFingerprint ||
		handoff.Handoff.Result.BundleReportFingerprint != value.BundleReportFingerprint ||
		handoff.Handoff.Result.BundleDigest != value.BundleDigest ||
		handoff.Intent.BundleBytes != value.BundleBytes ||
		containerPlan.ReadOnlyMountCount != value.ReadOnlyMountCount ||
		containerPlan.InputArtifactCount != value.InputArtifactCount ||
		containerPlan.RequestedBy != value.RequestedBy ||
		attempt.Intent.RequestedBy != value.RequestedBy ||
		value.CreatedAt.Before(attempt.Completion.CompletedAt) ||
		value.CreatedAt.Before(handoff.Handoff.CreatedAt) ||
		!handoff.Handoff.Result.DaemonConsumed ||
		!handoff.Handoff.Result.ReadbackVerified ||
		!handoff.Handoff.Result.FinalMountReadOnly ||
		!handoff.Handoff.Result.CleanupConfirmed ||
		handoff.Handoff.Result.ContainerStarted ||
		handoff.Handoff.Result.ProcessExecuted ||
		handoff.Handoff.Result.OutputExported ||
		handoff.Handoff.Result.ProductionExecutionSubmitted ||
		handoff.Handoff.Result.ProductionVerified ||
		handoff.Handoff.Result.BackendEnabled ||
		handoff.Handoff.Result.ExecutionAuthorized ||
		handoff.Handoff.Result.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker runtime input projection authority chain changed")
	}
	return nil
}

func insertDockerRuntimeInputProjectionPlanTx(ctx context.Context, tx *sql.Tx,
	plan sandbox.DockerRuntimeInputProjectionPlan,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_projection_plans
		(id, handoff_id, handoff_intent_id, attempt_id, container_plan_id, run_id,
		mission_id, workspace_id, protocol_version, status, trust_class,
		operation_key_digest, manifest_fingerprint, mount_binding_fingerprint,
		input_artifact_digest, authority_fingerprint, spec_fingerprint,
		container_plan_fingerprint, handoff_fingerprint, handoff_transport_fingerprint,
		bundle_report_fingerprint, bundle_digest, bundle_bytes, read_only_mount_count,
		input_artifact_count, projection_count, directory_root_count, file_root_count,
		total_entry_count, total_content_bytes, total_projection_bytes,
		projection_set_fingerprint, request_fingerprint, projection_fingerprint,
		operator_confirmed, exact_target_binding, all_volumes_read_only, all_volumes_no_copy,
		bundle_recaptured, bundle_digest_matched, daemon_contacted, daemon_applied,
		container_started, process_executed, output_exported,
		production_execution_submitted, production_verified, backend_enabled,
		execution_authorized, artifact_commit_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?)`,
		plan.ID, plan.HandoffID, plan.HandoffIntentID, plan.AttemptID,
		plan.ContainerPlanID, plan.RunID, plan.MissionID, plan.WorkspaceID,
		plan.ProtocolVersion, plan.Status, plan.TrustClass, plan.OperationKeyDigest,
		plan.ManifestFingerprint, plan.MountBindingFingerprint,
		plan.InputArtifactDigest, plan.AuthorityFingerprint, plan.SpecFingerprint,
		plan.ContainerPlanFingerprint, plan.HandoffFingerprint,
		plan.HandoffTransportFingerprint, plan.BundleReportFingerprint,
		plan.BundleDigest, plan.BundleBytes, plan.ReadOnlyMountCount,
		plan.InputArtifactCount, plan.ProjectionCount, plan.DirectoryRootCount,
		plan.FileRootCount, plan.TotalEntryCount, plan.TotalContentBytes,
		plan.TotalProjectionBytes, plan.ProjectionSetFingerprint,
		plan.RequestFingerprint, plan.ProjectionFingerprint,
		boolInt(plan.OperatorConfirmed),
		boolInt(plan.ExactTargetBinding), boolInt(plan.AllVolumesReadOnly),
		boolInt(plan.AllVolumesNoCopy), boolInt(plan.BundleRecaptured),
		boolInt(plan.BundleDigestMatched), boolInt(plan.DaemonContacted),
		boolInt(plan.DaemonApplied), boolInt(plan.ContainerStarted),
		boolInt(plan.ProcessExecuted), boolInt(plan.OutputExported),
		boolInt(plan.ProductionExecutionSubmitted), boolInt(plan.ProductionVerified),
		boolInt(plan.BackendEnabled), boolInt(plan.ExecutionAuthorized),
		boolInt(plan.ArtifactCommitAuthorized), plan.RequestedBy, ts(plan.CreatedAt))
	return err
}

func insertDockerRuntimeInputProjectionItemTx(ctx context.Context, tx *sql.Tx,
	projectionID string, item sandbox.DockerRuntimeInputProjectionItem,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_projection_items
		(projection_id, ordinal, protocol_version, kind, manifest_mount_ordinal,
		target_fingerprint, archive_root_fingerprint, volume_name_fingerprint,
		entry_count, regular_file_count, directory_count, content_bytes,
		projection_archive_bytes, content_digest, projection_archive_digest,
		root_directory, read_only, exact_target, no_copy, daemon_applied,
		container_started, process_executed, production_execution_submitted,
		item_fingerprint)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		projectionID, item.Ordinal, item.ProtocolVersion, item.Kind,
		item.ManifestMountOrdinal, item.TargetFingerprint, item.ArchiveRootFingerprint,
		item.VolumeNameFingerprint, item.EntryCount, item.RegularFileCount,
		item.DirectoryCount, item.ContentBytes, item.ProjectionArchiveBytes,
		item.ContentDigest, item.ProjectionArchiveDigest, boolInt(item.RootDirectory),
		boolInt(item.ReadOnly), boolInt(item.ExactTarget), boolInt(item.NoCopy),
		boolInt(item.DaemonApplied), boolInt(item.ContainerStarted),
		boolInt(item.ProcessExecuted), boolInt(item.ProductionExecutionSubmitted),
		item.ItemFingerprint)
	return err
}

func getDockerRuntimeInputProjection(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerRuntimeInputProjectionPlan, error) {
	value, err := scanDockerRuntimeInputProjection(queryer.QueryRowContext(ctx,
		dockerRuntimeInputProjectionSelect+` WHERE plan.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, apperror.New(
			apperror.CodeNotFound, "Docker runtime input projection plan not found")
	}
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, err
	}
	items, err := listDockerRuntimeInputProjectionItems(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, err
	}
	value.Items = items
	if err := value.Validate(); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, fmt.Errorf(
			"stored Docker runtime input projection plan is invalid: %w", err)
	}
	return value, nil
}

func getDockerRuntimeInputProjectionByHandoff(ctx context.Context,
	queryer sandboxLifecycleQueryer, handoffID string,
) (sandbox.DockerRuntimeInputProjectionPlan, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT plan.id
		FROM sandbox_docker_runtime_input_projection_plans plan
		JOIN sandbox_docker_runtime_input_projection_completions completion
			ON completion.projection_id = plan.id
		WHERE plan.handoff_id = ?`, handoffID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	value, err := getDockerRuntimeInputProjection(ctx, queryer, id)
	return value, err == nil, err
}

func scanDockerRuntimeInputProjection(row scanner) (
	sandbox.DockerRuntimeInputProjectionPlan, error,
) {
	var value sandbox.DockerRuntimeInputProjectionPlan
	var operatorConfirmed, exactTarget, readOnly, noCopy, recaptured, digestMatched int
	var daemonContacted, daemonApplied, started, executed, exported int
	var productionSubmitted, productionVerified, backendEnabled int
	var executionAuthorized, artifactAuthorized int
	var createdAt string
	err := row.Scan(&value.ID, &value.HandoffID, &value.HandoffIntentID,
		&value.AttemptID, &value.ContainerPlanID, &value.RunID, &value.MissionID,
		&value.WorkspaceID, &value.ProtocolVersion, &value.Status, &value.TrustClass,
		&value.OperationKeyDigest, &value.ManifestFingerprint,
		&value.MountBindingFingerprint, &value.InputArtifactDigest,
		&value.AuthorityFingerprint, &value.SpecFingerprint,
		&value.ContainerPlanFingerprint, &value.HandoffFingerprint,
		&value.HandoffTransportFingerprint, &value.BundleReportFingerprint,
		&value.BundleDigest, &value.BundleBytes, &value.ReadOnlyMountCount,
		&value.InputArtifactCount, &value.ProjectionCount, &value.DirectoryRootCount,
		&value.FileRootCount, &value.TotalEntryCount, &value.TotalContentBytes,
		&value.TotalProjectionBytes, &value.ProjectionSetFingerprint,
		&value.RequestFingerprint, &value.ProjectionFingerprint, &operatorConfirmed, &exactTarget,
		&readOnly, &noCopy, &recaptured, &digestMatched, &daemonContacted,
		&daemonApplied, &started, &executed, &exported, &productionSubmitted,
		&productionVerified, &backendEnabled, &executionAuthorized,
		&artifactAuthorized, &value.RequestedBy, &createdAt)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, err
	}
	value.CreatedAt = parseTS(createdAt)
	value.OperatorConfirmed = operatorConfirmed != 0
	value.ExactTargetBinding = exactTarget != 0
	value.AllVolumesReadOnly = readOnly != 0
	value.AllVolumesNoCopy = noCopy != 0
	value.BundleRecaptured = recaptured != 0
	value.BundleDigestMatched = digestMatched != 0
	value.DaemonContacted = daemonContacted != 0
	value.DaemonApplied = daemonApplied != 0
	value.ContainerStarted = started != 0
	value.ProcessExecuted = executed != 0
	value.OutputExported = exported != 0
	value.ProductionExecutionSubmitted = productionSubmitted != 0
	value.ProductionVerified = productionVerified != 0
	value.BackendEnabled = backendEnabled != 0
	value.ExecutionAuthorized = executionAuthorized != 0
	value.ArtifactCommitAuthorized = artifactAuthorized != 0
	return value, nil
}

func listDockerRuntimeInputProjectionItems(ctx context.Context,
	queryer sandboxLifecycleQueryer, projectionID string,
) ([]sandbox.DockerRuntimeInputProjectionItem, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, protocol_version, kind,
		manifest_mount_ordinal, target_fingerprint, archive_root_fingerprint,
		volume_name_fingerprint, entry_count, regular_file_count, directory_count,
		content_bytes, projection_archive_bytes, content_digest,
		projection_archive_digest, root_directory, read_only, exact_target, no_copy,
		daemon_applied, container_started, process_executed,
		production_execution_submitted, item_fingerprint
		FROM sandbox_docker_runtime_input_projection_items
		WHERE projection_id = ? ORDER BY ordinal`, projectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]sandbox.DockerRuntimeInputProjectionItem, 0,
		sandbox.MaxDockerRuntimeInputProjections)
	for rows.Next() {
		var item sandbox.DockerRuntimeInputProjectionItem
		var rootDirectory, readOnly, exactTarget, noCopy int
		var daemonApplied, started, executed, productionSubmitted int
		if err := rows.Scan(&item.Ordinal, &item.ProtocolVersion, &item.Kind,
			&item.ManifestMountOrdinal, &item.TargetFingerprint,
			&item.ArchiveRootFingerprint, &item.VolumeNameFingerprint,
			&item.EntryCount, &item.RegularFileCount, &item.DirectoryCount,
			&item.ContentBytes, &item.ProjectionArchiveBytes, &item.ContentDigest,
			&item.ProjectionArchiveDigest, &rootDirectory, &readOnly, &exactTarget,
			&noCopy, &daemonApplied, &started, &executed, &productionSubmitted,
			&item.ItemFingerprint); err != nil {
			return nil, err
		}
		item.RootDirectory = rootDirectory != 0
		item.ReadOnly = readOnly != 0
		item.ExactTarget = exactTarget != 0
		item.NoCopy = noCopy != 0
		item.DaemonApplied = daemonApplied != 0
		item.ContainerStarted = started != 0
		item.ProcessExecuted = executed != 0
		item.ProductionExecutionSubmitted = productionSubmitted != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func getDockerRuntimeInputProjectionOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerRuntimeInputProjectionOperation, bool, error) {
	var value sandbox.DockerRuntimeInputProjectionOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT key_digest, projection_id, handoff_id,
		container_plan_id, run_id, request_fingerprint, requested_by, created_at
		FROM sandbox_docker_runtime_input_projection_operations WHERE key_digest = ?`,
		keyDigest).Scan(&value.KeyDigest, &value.ProjectionID, &value.HandoffID,
		&value.ContainerPlanID, &value.RunID, &value.RequestFingerprint,
		&value.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputProjectionOperation{}, false, nil
	}
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionOperation{}, false, err
	}
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerRuntimeInputProjectionOperation{}, false, fmt.Errorf(
			"stored Docker runtime input projection operation is invalid: %w", err)
	}
	return value, true, nil
}

func replayDockerRuntimeInputProjection(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.DockerRuntimeInputProjectionOperation,
) (sandbox.DockerRuntimeInputProjectionPlan, bool, error) {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.HandoffID != requested.HandoffID ||
		existing.ContainerPlanID != requested.ContainerPlanID ||
		existing.RunID != requested.RunID || existing.RequestedBy != requested.RequestedBy {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker runtime input projection operation key was used for different intent")
	}
	value, err := getDockerRuntimeInputProjection(ctx, tx, existing.ProjectionID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	if !dockerRuntimeInputProjectionOperationMatchesPlan(existing, value) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.New(
			apperror.CodeInternal,
			"stored Docker runtime input projection operation binding is invalid")
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func (s *SQLiteStore) recoverDockerRuntimeInputProjectionCreate(ctx context.Context,
	operation sandbox.DockerRuntimeInputProjectionOperation, cause error,
) (sandbox.DockerRuntimeInputProjectionPlan, bool, error) {
	existing, found, err := getDockerRuntimeInputProjectionOperation(ctx, s.db,
		operation.KeyDigest)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, errors.Join(cause, err)
	}
	if !found {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, cause
	}
	if existing.RequestFingerprint != operation.RequestFingerprint ||
		existing.HandoffID != operation.HandoffID ||
		existing.ContainerPlanID != operation.ContainerPlanID ||
		existing.RunID != operation.RunID || existing.RequestedBy != operation.RequestedBy {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.New(
			apperror.CodeConflict,
			"Docker runtime input projection operation key was used for different intent")
	}
	value, err := getDockerRuntimeInputProjection(ctx, s.db, existing.ProjectionID)
	if err != nil {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, err
	}
	if !dockerRuntimeInputProjectionOperationMatchesPlan(existing, value) {
		return sandbox.DockerRuntimeInputProjectionPlan{}, false, apperror.New(
			apperror.CodeInternal,
			"recovered Docker runtime input projection operation binding is invalid")
	}
	value.Replayed = true
	return value, true, nil
}

func dockerRuntimeInputProjectionOperationMatchesPlan(
	operation sandbox.DockerRuntimeInputProjectionOperation,
	plan sandbox.DockerRuntimeInputProjectionPlan,
) bool {
	return operation.KeyDigest == plan.OperationKeyDigest &&
		operation.ProjectionID == plan.ID && operation.HandoffID == plan.HandoffID &&
		operation.ContainerPlanID == plan.ContainerPlanID && operation.RunID == plan.RunID &&
		operation.RequestFingerprint == plan.RequestFingerprint &&
		operation.RequestedBy == plan.RequestedBy &&
		operation.CreatedAt.Equal(plan.CreatedAt)
}

func appendDockerRuntimeInputProjectionEvent(ctx context.Context, tx *sql.Tx,
	plan sandbox.DockerRuntimeInputProjectionPlan,
) error {
	event, err := events.New(plan.RunID, plan.MissionID,
		events.SandboxDockerRuntimeInputProjectionEvent,
		"sandbox_docker_runtime_input_projection", plan.ID, map[string]any{
			"protocol_version":               plan.ProtocolVersion,
			"status":                         plan.Status,
			"trust_class":                    plan.TrustClass,
			"read_only_mounts":               plan.ReadOnlyMountCount,
			"input_artifacts":                plan.InputArtifactCount,
			"projections":                    plan.ProjectionCount,
			"directory_roots":                plan.DirectoryRootCount,
			"file_roots":                     plan.FileRootCount,
			"entries":                        plan.TotalEntryCount,
			"content_bytes":                  plan.TotalContentBytes,
			"projection_bytes":               plan.TotalProjectionBytes,
			"operator_confirmed":             true,
			"exact_target_binding":           true,
			"all_volumes_read_only":          true,
			"all_volumes_no_copy":            true,
			"bundle_recaptured":              true,
			"bundle_digest_matched":          true,
			"daemon_contacted":               false,
			"daemon_applied":                 false,
			"container_started":              false,
			"process_executed":               false,
			"production_execution_submitted": false,
			"backend_enabled":                false,
			"execution_authorized":           false,
			"artifact_commit_authorized":     false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = plan.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
