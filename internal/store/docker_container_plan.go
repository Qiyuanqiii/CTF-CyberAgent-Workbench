package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/sandbox"
)

const dockerContainerPlanSelect = `SELECT id, observation_id, evidence_id,
	output_simulation_id, preflight_id, execution_id, candidate_id, preparation_id,
	run_id, mission_id, workspace_id, protocol_version, source, trust_class, status,
	manifest_fingerprint, authorization_fingerprint, policy_fingerprint,
	mount_binding_fingerprint, input_artifact_digest, threat_model_fingerprint,
	output_plan_fingerprint, observation_fingerprint, authority_fingerprint,
	image_digest, os_type, architecture, container_user, spec_fingerprint,
	command_fingerprint, mount_plan_fingerprint, network_plan_fingerprint,
	secret_plan_fingerprint, container_config_fingerprint, resource_plan_fingerprint,
	termination_plan_fingerprint, label_plan_fingerprint, orphan_plan_fingerprint,
	container_name_fingerprint, plan_fingerprint, read_only_rootfs, no_new_privileges,
	drop_all_capabilities, init_enabled, mount_count, read_only_mount_count,
	writable_mount_count, dedicated_output_mount_count, private_propagation_mount_count,
	environment_count, secret_reference_count, input_artifact_count, output_count,
	network_mode, network_target_count, network_default_deny, exact_network_allowlist,
	network_guard_required, nano_cpus, memory_bytes, pids, max_output_bytes,
	timeout_seconds, grace_period_millis, secrets_ephemeral, secrets_metadata_excluded,
	label_count, reconcile_before_create, remove_on_rollback, export_after_stop,
	remove_after_export, control_count, transaction_protocol_version,
	transaction_source, transaction_status, transaction_fingerprint,
	transaction_step_count, transaction_staged_step_count,
	transaction_committed_step_count, transaction_rollback_step_count,
	transaction_daemon_write_count, transaction_backend_touched, simulation_only,
	production_submitted, production_verified, backend_available, backend_enabled,
	execution_authorized, artifact_commit_authorized, requested_by, created_at
	FROM sandbox_docker_container_plans`

func (s *SQLiteStore) GetDockerContainerPlanOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerContainerPlanOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerContainerPlanOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker container plan operation digest is invalid")
	}
	return getDockerContainerPlanOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateDockerContainerPlan(ctx context.Context,
	plan sandbox.DockerContainerPlan, operation sandbox.DockerContainerPlanOperation,
) (sandbox.DockerContainerPlan, bool, error) {
	if err := validateDockerContainerPlanMutation(plan, operation); err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DockerContainerPlan{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, plan.RunID); err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	if existing, found, err := getDockerContainerPlanOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	} else if found {
		return replayDockerContainerPlan(ctx, tx, existing, operation)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_docker_container_plans
		WHERE observation_id = ?`, plan.ObservationID).Scan(&count); err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	if count >= sandbox.MaxDockerContainerPlansPerObservation {
		return sandbox.DockerContainerPlan{}, false, apperror.New(apperror.CodeResourceExhausted,
			"Docker container plan limit is exhausted for this observation")
	}
	if err := validateDockerContainerPlanCurrentTx(ctx, tx, plan); err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	if err := insertDockerContainerPlanTx(ctx, tx, plan); err != nil {
		_ = tx.Rollback()
		return s.recoverDockerContainerPlanCreate(ctx, operation, err)
	}
	for _, control := range plan.Controls {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_plan_controls
			(plan_id, ordinal, name, state, control_digest, planned, applied, verified)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, plan.ID, control.Ordinal, control.Name,
			control.State, control.ControlDigest, boolInt(control.Planned),
			boolInt(control.Applied), boolInt(control.Verified)); err != nil {
			return sandbox.DockerContainerPlan{}, false, err
		}
	}
	for _, step := range plan.Transaction.Steps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_plan_steps
			(plan_id, ordinal, name, state, step_digest, simulated, production_applied)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, plan.ID, step.Ordinal, step.Name, step.State,
			step.StepDigest, boolInt(step.Simulated), boolInt(step.ProductionApplied)); err != nil {
			return sandbox.DockerContainerPlan{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_plan_operations
		(operation_key_digest, request_fingerprint, plan_id, observation_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.PlanID, operation.ObservationID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverDockerContainerPlanCreate(ctx, operation, err)
	}
	if err := appendDockerContainerPlanEvent(ctx, tx, plan); err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverDockerContainerPlanCreate(ctx, operation, err)
	}
	return plan, false, nil
}

func (s *SQLiteStore) GetDockerContainerPlan(ctx context.Context,
	id string,
) (sandbox.DockerContainerPlan, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeInvalidArgument,
			"Docker container plan id is invalid")
	}
	return getDockerContainerPlan(ctx, s.db, id)
}

func (s *SQLiteStore) ListDockerContainerPlans(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerContainerPlan, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker container plan list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker container plan list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_docker_container_plans
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
	values := make([]sandbox.DockerContainerPlan, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerContainerPlan(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func validateDockerContainerPlanMutation(plan sandbox.DockerContainerPlan,
	operation sandbox.DockerContainerPlanOperation,
) error {
	if err := plan.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "Docker container plan is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container plan operation is invalid", err)
	}
	if operation.PlanID != plan.ID || operation.ObservationID != plan.ObservationID ||
		operation.RunID != plan.RunID || operation.RequestedBy != plan.RequestedBy ||
		!operation.CreatedAt.Equal(plan.CreatedAt) ||
		operation.RequestFingerprint != sandbox.DockerContainerPlanRequestFingerprint(plan) {
		return apperror.New(apperror.CodeConflict,
			"Docker container plan operation does not match its request")
	}
	return nil
}

func validateDockerContainerPlanCurrentTx(ctx context.Context, tx *sql.Tx,
	plan sandbox.DockerContainerPlan,
) error {
	observation, err := getDockerObservation(ctx, tx, plan.ObservationID)
	if err != nil {
		return err
	}
	if err := validateDockerObservationCurrentTx(ctx, tx, observation); err != nil {
		return err
	}
	intent, err := getSandboxManifestIntent(ctx, tx, plan.PreparationID)
	if err != nil {
		return err
	}
	preparation := intent.Preparation
	report := observation.Report
	if report.Status != sandbox.DockerObservationStatusComplete ||
		!report.ObservationComplete || !report.ProductionObserved || report.ProductionVerified ||
		report.BackendAvailable || report.BackendEnabled || report.ExecutionAuthorized ||
		report.ArtifactCommitAuthorized || plan.ObservationID != observation.ID ||
		plan.EvidenceID != observation.EvidenceID ||
		plan.OutputSimulationID != observation.OutputSimulationID ||
		plan.PreflightID != observation.PreflightID || plan.ExecutionID != observation.ExecutionID ||
		plan.CandidateID != observation.CandidateID ||
		plan.PreparationID != observation.PreparationID || plan.RunID != observation.RunID ||
		plan.MissionID != observation.MissionID || plan.WorkspaceID != observation.WorkspaceID ||
		plan.ManifestFingerprint != observation.ManifestFingerprint ||
		plan.AuthorizationFingerprint != observation.AuthorizationFingerprint ||
		plan.PolicyFingerprint != observation.PolicyFingerprint ||
		plan.MountBindingFingerprint != observation.MountBindingFingerprint ||
		plan.InputArtifactDigest != observation.InputArtifactDigest ||
		plan.ThreatModelFingerprint != observation.ThreatModelFingerprint ||
		plan.OutputPlanFingerprint != observation.OutputPlanFingerprint ||
		plan.ObservationFingerprint != report.ObservationFingerprint ||
		plan.AuthorityFingerprint != sandbox.DockerContainerAuthorityFingerprint(observation) ||
		plan.ImageDigest != report.ImageDigest || plan.OSType != report.ImageOSType ||
		plan.Architecture != report.ImageArchitecture || !report.PidsLimitSupported ||
		plan.RequestedBy != observation.RequestedBy ||
		plan.ManifestFingerprint != preparation.ManifestFingerprint ||
		plan.RunID != preparation.RunID || plan.MissionID != preparation.MissionID ||
		plan.WorkspaceID != preparation.WorkspaceID || plan.RequestedBy != preparation.RequestedBy ||
		plan.MountCount != preparation.MountCount || plan.ReadOnlyMountCount != preparation.MountCount-1 ||
		plan.WritableMountCount != preparation.WritableMountCount ||
		preparation.WritableMountCount != 1 || plan.DedicatedOutputMounts != 1 ||
		plan.PrivatePropagationMounts != preparation.MountCount ||
		plan.EnvironmentCount != preparation.EnvironmentCount ||
		plan.SecretReferenceCount != preparation.SecretReferenceCount ||
		plan.InputArtifactCount != preparation.InputArtifactCount ||
		plan.OutputCount != preparation.OutputCount || plan.NetworkMode != preparation.NetworkMode ||
		plan.NetworkTargetCount != preparation.AllowedTargetCount ||
		plan.NanoCPUs != int64(preparation.CPUQuotaMillis)*1_000_000 ||
		plan.MemoryBytes != preparation.MemoryBytes || plan.PIDs != preparation.PIDs ||
		plan.MaxOutputBytes != preparation.MaxOutputBytes ||
		plan.TimeoutSeconds != preparation.TimeoutSeconds ||
		plan.GracePeriodMillis != preparation.GracePeriodMillis {
		return apperror.New(apperror.CodeConflict,
			"Docker container plan does not match the current v48-v53 authority chain")
	}
	return nil
}

func insertDockerContainerPlanTx(ctx context.Context, tx *sql.Tx,
	plan sandbox.DockerContainerPlan,
) error {
	transaction := plan.Transaction
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_plans
		(id, observation_id, evidence_id, output_simulation_id, preflight_id, execution_id,
		candidate_id, preparation_id, run_id, mission_id, workspace_id, protocol_version,
		source, trust_class, status, manifest_fingerprint, authorization_fingerprint,
		policy_fingerprint, mount_binding_fingerprint, input_artifact_digest,
		threat_model_fingerprint, output_plan_fingerprint, observation_fingerprint,
		authority_fingerprint, image_digest, os_type, architecture, container_user,
		spec_fingerprint, command_fingerprint, mount_plan_fingerprint,
		network_plan_fingerprint, secret_plan_fingerprint, container_config_fingerprint,
		resource_plan_fingerprint, termination_plan_fingerprint, label_plan_fingerprint,
		orphan_plan_fingerprint, container_name_fingerprint, plan_fingerprint,
		read_only_rootfs, no_new_privileges, drop_all_capabilities, init_enabled,
		mount_count, read_only_mount_count, writable_mount_count,
		dedicated_output_mount_count, private_propagation_mount_count,
		environment_count, secret_reference_count, input_artifact_count, output_count,
		network_mode, network_target_count, network_default_deny, exact_network_allowlist,
		network_guard_required, nano_cpus, memory_bytes, pids, max_output_bytes,
		timeout_seconds, grace_period_millis, secrets_ephemeral, secrets_metadata_excluded,
		label_count, reconcile_before_create, remove_on_rollback, export_after_stop,
		remove_after_export, control_count, transaction_protocol_version,
		transaction_source, transaction_status, transaction_fingerprint,
		transaction_step_count, transaction_staged_step_count,
		transaction_committed_step_count, transaction_rollback_step_count,
		transaction_daemon_write_count, transaction_backend_touched, simulation_only,
		production_submitted, production_verified, backend_available, backend_enabled,
		execution_authorized, artifact_commit_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.ObservationID, plan.EvidenceID, plan.OutputSimulationID,
		plan.PreflightID, plan.ExecutionID, plan.CandidateID, plan.PreparationID,
		plan.RunID, plan.MissionID, plan.WorkspaceID, plan.ProtocolVersion,
		plan.Source, plan.TrustClass, plan.Status, plan.ManifestFingerprint,
		plan.AuthorizationFingerprint, plan.PolicyFingerprint, plan.MountBindingFingerprint,
		plan.InputArtifactDigest, plan.ThreatModelFingerprint, plan.OutputPlanFingerprint,
		plan.ObservationFingerprint, plan.AuthorityFingerprint, plan.ImageDigest,
		plan.OSType, plan.Architecture, plan.ContainerUser, plan.SpecFingerprint,
		plan.CommandFingerprint, plan.MountPlanFingerprint, plan.NetworkPlanFingerprint,
		plan.SecretPlanFingerprint, plan.ContainerConfigFingerprint,
		plan.ResourcePlanFingerprint, plan.TerminationPlanFingerprint,
		plan.LabelPlanFingerprint, plan.OrphanPlanFingerprint,
		plan.ContainerNameFingerprint, plan.PlanFingerprint, boolInt(plan.ReadOnlyRootFS),
		boolInt(plan.NoNewPrivileges), boolInt(plan.DropAllCapabilities), boolInt(plan.InitEnabled),
		plan.MountCount, plan.ReadOnlyMountCount, plan.WritableMountCount,
		plan.DedicatedOutputMounts, plan.PrivatePropagationMounts, plan.EnvironmentCount,
		plan.SecretReferenceCount, plan.InputArtifactCount, plan.OutputCount,
		plan.NetworkMode, plan.NetworkTargetCount, boolInt(plan.NetworkDefaultDeny),
		boolInt(plan.ExactNetworkAllowlist), boolInt(plan.NetworkGuardRequired),
		plan.NanoCPUs, plan.MemoryBytes, plan.PIDs, plan.MaxOutputBytes,
		plan.TimeoutSeconds, plan.GracePeriodMillis, boolInt(plan.SecretsEphemeral),
		boolInt(plan.SecretsMetadataExcluded), plan.LabelCount,
		boolInt(plan.ReconcileBeforeCreate), boolInt(plan.RemoveOnRollback),
		boolInt(plan.ExportAfterStop), boolInt(plan.RemoveAfterExport), len(plan.Controls),
		transaction.ProtocolVersion, transaction.Source, transaction.Status,
		transaction.TransactionFingerprint, transaction.StepCount,
		transaction.StagedStepCount, transaction.CommittedStepCount,
		transaction.RollbackStepCount, transaction.DaemonWriteCount,
		boolInt(transaction.BackendTouched), boolInt(plan.SimulationOnly),
		boolInt(plan.ProductionSubmitted), boolInt(plan.ProductionVerified),
		boolInt(plan.BackendAvailable), boolInt(plan.BackendEnabled),
		boolInt(plan.ExecutionAuthorized), boolInt(plan.ArtifactCommitAuthorized),
		plan.RequestedBy, ts(plan.CreatedAt))
	return err
}

func getDockerContainerPlan(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerContainerPlan, error) {
	plan, err := scanDockerContainerPlan(queryer.QueryRowContext(ctx,
		dockerContainerPlanSelect+` WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sandbox.DockerContainerPlan{}, apperror.New(apperror.CodeNotFound,
				"Docker container plan not found")
		}
		return sandbox.DockerContainerPlan{}, err
	}
	controls, err := listDockerContainerPlanControls(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerPlan{}, err
	}
	steps, err := listDockerContainerPlanSteps(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerPlan{}, err
	}
	plan.Controls = controls
	plan.Transaction.Steps = steps
	if err := plan.Validate(); err != nil {
		return sandbox.DockerContainerPlan{}, fmt.Errorf("stored Docker container plan is invalid: %w", err)
	}
	return plan, nil
}

func scanDockerContainerPlan(row scanner) (sandbox.DockerContainerPlan, error) {
	var plan sandbox.DockerContainerPlan
	var createdAt string
	var readOnlyRootFS, noNewPrivileges, dropAllCapabilities, initEnabled int
	var networkDefaultDeny, exactNetworkAllowlist, networkGuardRequired int
	var secretsEphemeral, secretsMetadataExcluded int
	var reconcileBeforeCreate, removeOnRollback, exportAfterStop, removeAfterExport int
	var transactionBackendTouched, simulationOnly, productionSubmitted, productionVerified int
	var backendAvailable, backendEnabled, executionAuthorized, artifactCommitAuthorized int
	var controlCount int
	if err := row.Scan(&plan.ID, &plan.ObservationID, &plan.EvidenceID,
		&plan.OutputSimulationID, &plan.PreflightID, &plan.ExecutionID, &plan.CandidateID,
		&plan.PreparationID, &plan.RunID, &plan.MissionID, &plan.WorkspaceID,
		&plan.ProtocolVersion, &plan.Source, &plan.TrustClass, &plan.Status,
		&plan.ManifestFingerprint, &plan.AuthorizationFingerprint, &plan.PolicyFingerprint,
		&plan.MountBindingFingerprint, &plan.InputArtifactDigest,
		&plan.ThreatModelFingerprint, &plan.OutputPlanFingerprint,
		&plan.ObservationFingerprint, &plan.AuthorityFingerprint, &plan.ImageDigest,
		&plan.OSType, &plan.Architecture, &plan.ContainerUser, &plan.SpecFingerprint,
		&plan.CommandFingerprint, &plan.MountPlanFingerprint, &plan.NetworkPlanFingerprint,
		&plan.SecretPlanFingerprint, &plan.ContainerConfigFingerprint,
		&plan.ResourcePlanFingerprint, &plan.TerminationPlanFingerprint,
		&plan.LabelPlanFingerprint, &plan.OrphanPlanFingerprint,
		&plan.ContainerNameFingerprint, &plan.PlanFingerprint, &readOnlyRootFS,
		&noNewPrivileges, &dropAllCapabilities, &initEnabled, &plan.MountCount,
		&plan.ReadOnlyMountCount, &plan.WritableMountCount, &plan.DedicatedOutputMounts,
		&plan.PrivatePropagationMounts, &plan.EnvironmentCount, &plan.SecretReferenceCount,
		&plan.InputArtifactCount, &plan.OutputCount, &plan.NetworkMode,
		&plan.NetworkTargetCount, &networkDefaultDeny, &exactNetworkAllowlist,
		&networkGuardRequired, &plan.NanoCPUs, &plan.MemoryBytes, &plan.PIDs,
		&plan.MaxOutputBytes, &plan.TimeoutSeconds, &plan.GracePeriodMillis,
		&secretsEphemeral, &secretsMetadataExcluded, &plan.LabelCount,
		&reconcileBeforeCreate, &removeOnRollback, &exportAfterStop, &removeAfterExport,
		&controlCount, &plan.Transaction.ProtocolVersion, &plan.Transaction.Source,
		&plan.Transaction.Status, &plan.Transaction.TransactionFingerprint,
		&plan.Transaction.StepCount, &plan.Transaction.StagedStepCount,
		&plan.Transaction.CommittedStepCount, &plan.Transaction.RollbackStepCount,
		&plan.Transaction.DaemonWriteCount, &transactionBackendTouched, &simulationOnly,
		&productionSubmitted, &productionVerified, &backendAvailable, &backendEnabled,
		&executionAuthorized, &artifactCommitAuthorized, &plan.RequestedBy,
		&createdAt); err != nil {
		return sandbox.DockerContainerPlan{}, err
	}
	plan.CreatedAt = parseTS(createdAt)
	plan.ReadOnlyRootFS = readOnlyRootFS != 0
	plan.NoNewPrivileges = noNewPrivileges != 0
	plan.DropAllCapabilities = dropAllCapabilities != 0
	plan.InitEnabled = initEnabled != 0
	plan.NetworkDefaultDeny = networkDefaultDeny != 0
	plan.ExactNetworkAllowlist = exactNetworkAllowlist != 0
	plan.NetworkGuardRequired = networkGuardRequired != 0
	plan.SecretsEphemeral = secretsEphemeral != 0
	plan.SecretsMetadataExcluded = secretsMetadataExcluded != 0
	plan.ReconcileBeforeCreate = reconcileBeforeCreate != 0
	plan.RemoveOnRollback = removeOnRollback != 0
	plan.ExportAfterStop = exportAfterStop != 0
	plan.RemoveAfterExport = removeAfterExport != 0
	plan.Transaction.SpecFingerprint = plan.SpecFingerprint
	plan.Transaction.BackendTouched = transactionBackendTouched != 0
	plan.Transaction.SimulationOnly = simulationOnly != 0
	plan.SimulationOnly = simulationOnly != 0
	plan.ProductionSubmitted = productionSubmitted != 0
	plan.ProductionVerified = productionVerified != 0
	plan.BackendAvailable = backendAvailable != 0
	plan.BackendEnabled = backendEnabled != 0
	plan.ExecutionAuthorized = executionAuthorized != 0
	plan.ArtifactCommitAuthorized = artifactCommitAuthorized != 0
	plan.Transaction.ProductionSubmitted = plan.ProductionSubmitted
	plan.Transaction.ProductionVerified = plan.ProductionVerified
	plan.Transaction.BackendEnabled = plan.BackendEnabled
	plan.Transaction.ExecutionAuthorized = plan.ExecutionAuthorized
	plan.Transaction.ArtifactCommitAuthorized = plan.ArtifactCommitAuthorized
	if controlCount != sandbox.MaxDockerContainerControls ||
		plan.Transaction.StepCount != sandbox.MaxDockerWriteSteps {
		return sandbox.DockerContainerPlan{}, errors.New("stored Docker container plan counts are invalid")
	}
	return plan, nil
}

func listDockerContainerPlanControls(ctx context.Context, queryer sandboxLifecycleQueryer,
	planID string,
) ([]sandbox.DockerContainerControl, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, state, control_digest,
		planned, applied, verified FROM sandbox_docker_container_plan_controls
		WHERE plan_id = ? ORDER BY ordinal`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	controls := make([]sandbox.DockerContainerControl, 0, sandbox.MaxDockerContainerControls)
	for rows.Next() {
		var control sandbox.DockerContainerControl
		var planned, applied, verified int
		if err := rows.Scan(&control.Ordinal, &control.Name, &control.State,
			&control.ControlDigest, &planned, &applied, &verified); err != nil {
			return nil, err
		}
		control.Planned = planned != 0
		control.Applied = applied != 0
		control.Verified = verified != 0
		controls = append(controls, control)
	}
	return controls, rows.Err()
}

func listDockerContainerPlanSteps(ctx context.Context, queryer sandboxLifecycleQueryer,
	planID string,
) ([]sandbox.DockerWriteStep, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, state, step_digest,
		simulated, production_applied FROM sandbox_docker_container_plan_steps
		WHERE plan_id = ? ORDER BY ordinal`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	steps := make([]sandbox.DockerWriteStep, 0, sandbox.MaxDockerWriteSteps)
	for rows.Next() {
		var step sandbox.DockerWriteStep
		var simulated, productionApplied int
		if err := rows.Scan(&step.Ordinal, &step.Name, &step.State, &step.StepDigest,
			&simulated, &productionApplied); err != nil {
			return nil, err
		}
		step.Simulated = simulated != 0
		step.ProductionApplied = productionApplied != 0
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func getDockerContainerPlanOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.DockerContainerPlanOperation, bool, error) {
	var operation sandbox.DockerContainerPlanOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		plan_id, observation_id, run_id, requested_by, created_at
		FROM sandbox_docker_container_plan_operations WHERE operation_key_digest = ?`,
		keyDigest).Scan(&operation.KeyDigest, &operation.RequestFingerprint,
		&operation.PlanID, &operation.ObservationID, &operation.RunID,
		&operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerContainerPlanOperation{}, false, nil
	}
	if err != nil {
		return sandbox.DockerContainerPlanOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.Validate(); err != nil {
		return sandbox.DockerContainerPlanOperation{}, false,
			fmt.Errorf("stored Docker container plan operation is invalid: %w", err)
	}
	return operation, true, nil
}

func replayDockerContainerPlan(ctx context.Context, tx *sql.Tx,
	existing, requested sandbox.DockerContainerPlanOperation,
) (sandbox.DockerContainerPlan, bool, error) {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.ObservationID != requested.ObservationID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return sandbox.DockerContainerPlan{}, false, apperror.New(apperror.CodeConflict,
			"Docker container plan operation key was used for different intent")
	}
	plan, err := getDockerContainerPlan(ctx, tx, existing.PlanID)
	if err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	if existing.RequestFingerprint != sandbox.DockerContainerPlanRequestFingerprint(plan) ||
		existing.ObservationID != plan.ObservationID || existing.RunID != plan.RunID ||
		existing.RequestedBy != plan.RequestedBy || !existing.CreatedAt.Equal(plan.CreatedAt) {
		return sandbox.DockerContainerPlan{}, false, apperror.New(apperror.CodeInternal,
			"stored Docker container plan operation binding is invalid")
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	plan.Replayed = true
	return plan, true, nil
}

func (s *SQLiteStore) recoverDockerContainerPlanCreate(ctx context.Context,
	operation sandbox.DockerContainerPlanOperation, cause error,
) (sandbox.DockerContainerPlan, bool, error) {
	existing, found, err := getDockerContainerPlanOperation(ctx, s.db, operation.KeyDigest)
	if err != nil {
		return sandbox.DockerContainerPlan{}, false, errors.Join(cause, err)
	}
	if !found {
		return sandbox.DockerContainerPlan{}, false, cause
	}
	if existing.KeyDigest != operation.KeyDigest ||
		existing.RequestFingerprint != operation.RequestFingerprint ||
		existing.ObservationID != operation.ObservationID || existing.RunID != operation.RunID ||
		existing.RequestedBy != operation.RequestedBy {
		return sandbox.DockerContainerPlan{}, false, apperror.New(apperror.CodeConflict,
			"Docker container plan operation key was used for different intent")
	}
	plan, err := getDockerContainerPlan(ctx, s.db, existing.PlanID)
	if err != nil {
		return sandbox.DockerContainerPlan{}, false, err
	}
	if existing.RequestFingerprint != sandbox.DockerContainerPlanRequestFingerprint(plan) ||
		existing.ObservationID != plan.ObservationID || existing.RunID != plan.RunID ||
		existing.RequestedBy != plan.RequestedBy || !existing.CreatedAt.Equal(plan.CreatedAt) {
		return sandbox.DockerContainerPlan{}, false, apperror.New(apperror.CodeInternal,
			"recovered Docker container plan operation binding is invalid")
	}
	plan.Replayed = true
	return plan, true, nil
}

func appendDockerContainerPlanEvent(ctx context.Context, tx *sql.Tx,
	plan sandbox.DockerContainerPlan,
) error {
	event, err := events.New(plan.RunID, plan.MissionID,
		events.SandboxDockerContainerPlanRecordedEvent, "sandbox_docker_container_plan", plan.ID,
		map[string]any{
			"protocol_version":           plan.ProtocolVersion,
			"status":                     plan.Status,
			"trust_class":                plan.TrustClass,
			"controls":                   len(plan.Controls),
			"fake_write_steps":           plan.Transaction.CommittedStepCount,
			"mounts":                     plan.MountCount,
			"network_mode":               plan.NetworkMode,
			"network_targets":            plan.NetworkTargetCount,
			"secret_references":          plan.SecretReferenceCount,
			"simulation_only":            true,
			"daemon_writes":              0,
			"production_submitted":       false,
			"production_verified":        false,
			"backend_enabled":            false,
			"execution_authorized":       false,
			"artifact_commit_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = plan.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
