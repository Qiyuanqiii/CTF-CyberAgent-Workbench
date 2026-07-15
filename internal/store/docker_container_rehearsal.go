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

const dockerContainerRehearsalSelect = `SELECT id, plan_id, observation_id, evidence_id,
	output_simulation_id, preflight_id, execution_id, candidate_id, preparation_id,
	run_id, mission_id, workspace_id, protocol_version, source, trust_class, status,
	manifest_fingerprint, authorization_fingerprint, policy_fingerprint,
	mount_binding_fingerprint, input_artifact_digest, threat_model_fingerprint,
	output_plan_fingerprint, observation_fingerprint, authority_fingerprint,
	spec_fingerprint, plan_fingerprint, image_digest, network_mode, environment_count,
	secret_reference_count, request_fingerprint, endpoint_class, endpoint_fingerprint,
	container_id_fingerprint, inspection_fingerprint, transport_fingerprint,
	rehearsal_fingerprint, result_protocol_version, result_status, step_count,
	daemon_read_count, daemon_write_count, reconciled_container_count,
	configuration_matched, container_created, container_inspected, container_removed,
	container_never_started, process_never_executed, image_never_pulled,
	output_never_exported, cleanup_confirmed, daemon_reachable, daemon_write_submitted,
	production_execution_submitted, production_verified, backend_enabled,
	execution_authorized, artifact_commit_authorized, requested_by, created_at
	FROM sandbox_docker_container_rehearsals`

func (s *SQLiteStore) GetDockerContainerRehearsalOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerContainerRehearsalOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerContainerRehearsalOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker rehearsal operation digest is invalid")
	}
	return getDockerContainerRehearsalOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateDockerContainerRehearsal(ctx context.Context,
	rehearsal sandbox.DockerContainerRehearsal,
	operation sandbox.DockerContainerRehearsalOperation,
) (sandbox.DockerContainerRehearsal, bool, error) {
	if err := validateDockerContainerRehearsalMutation(rehearsal, operation); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, rehearsal.RunID); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if existing, found, err := getDockerContainerRehearsalOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	} else if found {
		return replayDockerContainerRehearsal(ctx, tx, existing, operation)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_docker_container_rehearsals
		WHERE plan_id = ?`, rehearsal.PlanID).Scan(&count); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if count >= sandbox.MaxDockerContainerRehearsalsPerPlan {
		return sandbox.DockerContainerRehearsal{}, false, apperror.New(apperror.CodeResourceExhausted,
			"Docker container rehearsal limit is exhausted for this plan")
	}
	if err := validateDockerContainerRehearsalCurrentTx(ctx, tx, rehearsal); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := insertDockerContainerRehearsalTx(ctx, tx, rehearsal); err != nil {
		_ = tx.Rollback()
		return s.recoverDockerContainerRehearsalCreate(ctx, operation, err)
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
		_ = tx.Rollback()
		return s.recoverDockerContainerRehearsalCreate(ctx, operation, err)
	}
	if err := appendDockerContainerRehearsalEvent(ctx, tx, rehearsal); err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverDockerContainerRehearsalCreate(ctx, operation, err)
	}
	return rehearsal, false, nil
}

func (s *SQLiteStore) GetDockerContainerRehearsal(ctx context.Context,
	id string,
) (sandbox.DockerContainerRehearsal, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeInvalidArgument,
			"Docker container rehearsal id is invalid")
	}
	return getDockerContainerRehearsal(ctx, s.db, id)
}

func (s *SQLiteStore) ListDockerContainerRehearsals(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerContainerRehearsal, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker container rehearsal list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker container rehearsal list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_docker_container_rehearsals
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
	values := make([]sandbox.DockerContainerRehearsal, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerContainerRehearsal(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func validateDockerContainerRehearsalMutation(rehearsal sandbox.DockerContainerRehearsal,
	operation sandbox.DockerContainerRehearsalOperation,
) error {
	if err := rehearsal.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container rehearsal is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker container rehearsal operation is invalid", err)
	}
	if operation.RehearsalID != rehearsal.ID || operation.PlanID != rehearsal.PlanID ||
		operation.RunID != rehearsal.RunID || operation.RequestedBy != rehearsal.RequestedBy ||
		!operation.CreatedAt.Equal(rehearsal.CreatedAt) ||
		operation.RequestFingerprint != sandbox.DockerContainerRehearsalRequestFingerprint(rehearsal) {
		return apperror.New(apperror.CodeConflict,
			"Docker container rehearsal operation does not match its request")
	}
	return nil
}

func validateDockerContainerRehearsalCurrentTx(ctx context.Context, tx *sql.Tx,
	rehearsal sandbox.DockerContainerRehearsal,
) error {
	plan, err := getDockerContainerPlan(ctx, tx, rehearsal.PlanID)
	if err != nil {
		return err
	}
	if err := validateDockerContainerPlanCurrentTx(ctx, tx, plan); err != nil {
		return err
	}
	if rehearsal.PlanID != plan.ID || rehearsal.ObservationID != plan.ObservationID ||
		rehearsal.EvidenceID != plan.EvidenceID ||
		rehearsal.OutputSimulationID != plan.OutputSimulationID ||
		rehearsal.PreflightID != plan.PreflightID || rehearsal.ExecutionID != plan.ExecutionID ||
		rehearsal.CandidateID != plan.CandidateID ||
		rehearsal.PreparationID != plan.PreparationID || rehearsal.RunID != plan.RunID ||
		rehearsal.MissionID != plan.MissionID || rehearsal.WorkspaceID != plan.WorkspaceID ||
		rehearsal.ManifestFingerprint != plan.ManifestFingerprint ||
		rehearsal.AuthorizationFingerprint != plan.AuthorizationFingerprint ||
		rehearsal.PolicyFingerprint != plan.PolicyFingerprint ||
		rehearsal.MountBindingFingerprint != plan.MountBindingFingerprint ||
		rehearsal.InputArtifactDigest != plan.InputArtifactDigest ||
		rehearsal.ThreatModelFingerprint != plan.ThreatModelFingerprint ||
		rehearsal.OutputPlanFingerprint != plan.OutputPlanFingerprint ||
		rehearsal.ObservationFingerprint != plan.ObservationFingerprint ||
		rehearsal.AuthorityFingerprint != plan.AuthorityFingerprint ||
		rehearsal.SpecFingerprint != plan.SpecFingerprint ||
		rehearsal.PlanFingerprint != plan.PlanFingerprint || rehearsal.ImageDigest != plan.ImageDigest ||
		plan.NetworkMode != "disabled" || plan.NetworkTargetCount != 0 ||
		plan.EnvironmentCount != 0 || plan.SecretReferenceCount != 0 ||
		rehearsal.NetworkMode != plan.NetworkMode || rehearsal.EnvironmentCount != 0 ||
		rehearsal.SecretReferenceCount != 0 || rehearsal.RequestedBy != plan.RequestedBy ||
		!plan.SimulationOnly || plan.ProductionSubmitted || plan.ProductionVerified ||
		plan.BackendAvailable || plan.BackendEnabled || plan.ExecutionAuthorized ||
		plan.ArtifactCommitAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker container rehearsal does not match the current v48-v54 authority chain")
	}
	return nil
}

func insertDockerContainerRehearsalTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerContainerRehearsal,
) error {
	result := value.Result
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_container_rehearsals
		(id, plan_id, observation_id, evidence_id, output_simulation_id, preflight_id,
		execution_id, candidate_id, preparation_id, run_id, mission_id, workspace_id,
		protocol_version, source, trust_class, status, manifest_fingerprint,
		authorization_fingerprint, policy_fingerprint, mount_binding_fingerprint,
		input_artifact_digest, threat_model_fingerprint, output_plan_fingerprint,
		observation_fingerprint, authority_fingerprint, spec_fingerprint, plan_fingerprint,
		image_digest, network_mode, environment_count, secret_reference_count,
		request_fingerprint, endpoint_class, endpoint_fingerprint,
		container_id_fingerprint, inspection_fingerprint, transport_fingerprint,
		rehearsal_fingerprint, result_protocol_version, result_status, step_count,
		daemon_read_count, daemon_write_count, reconciled_container_count,
		configuration_matched, container_created, container_inspected, container_removed,
		container_never_started, process_never_executed, image_never_pulled,
		output_never_exported, cleanup_confirmed, daemon_reachable, daemon_write_submitted,
		production_execution_submitted, production_verified, backend_enabled,
		execution_authorized, artifact_commit_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.PlanID, value.ObservationID, value.EvidenceID,
		value.OutputSimulationID, value.PreflightID, value.ExecutionID, value.CandidateID,
		value.PreparationID, value.RunID, value.MissionID, value.WorkspaceID,
		value.ProtocolVersion, value.Source, value.TrustClass, value.Status,
		value.ManifestFingerprint, value.AuthorizationFingerprint, value.PolicyFingerprint,
		value.MountBindingFingerprint, value.InputArtifactDigest, value.ThreatModelFingerprint,
		value.OutputPlanFingerprint, value.ObservationFingerprint, value.AuthorityFingerprint,
		value.SpecFingerprint, value.PlanFingerprint, value.ImageDigest, value.NetworkMode,
		value.EnvironmentCount, value.SecretReferenceCount, value.RequestFingerprint,
		value.EndpointClass, value.EndpointFingerprint, value.ContainerIDFingerprint,
		value.InspectionFingerprint, value.TransportFingerprint, value.RehearsalFingerprint,
		result.ProtocolVersion, result.Status, value.StepCount, value.DaemonReadCount,
		value.DaemonWriteCount, value.ReconciledContainerCount,
		boolInt(value.ConfigurationMatched), boolInt(result.ContainerCreated),
		boolInt(result.ContainerInspected), boolInt(result.ContainerRemoved),
		boolInt(value.ContainerNeverStarted), boolInt(value.ProcessNeverExecuted),
		boolInt(value.ImageNeverPulled), boolInt(value.OutputNeverExported),
		boolInt(value.CleanupConfirmed), boolInt(value.DaemonReachable),
		boolInt(value.DaemonWriteSubmitted), boolInt(value.ProductionExecutionSubmitted),
		boolInt(value.ProductionVerified), boolInt(value.BackendEnabled),
		boolInt(value.ExecutionAuthorized), boolInt(value.ArtifactCommitAuthorized),
		value.RequestedBy, ts(value.CreatedAt))
	return err
}

func getDockerContainerRehearsal(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerContainerRehearsal, error) {
	value, err := scanDockerContainerRehearsal(queryer.QueryRowContext(ctx,
		dockerContainerRehearsalSelect+` WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sandbox.DockerContainerRehearsal{}, apperror.New(apperror.CodeNotFound,
				"Docker container rehearsal not found")
		}
		return sandbox.DockerContainerRehearsal{}, err
	}
	steps, err := listDockerContainerRehearsalSteps(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, err
	}
	value.Result.Steps = steps
	if err := value.Validate(); err != nil {
		return sandbox.DockerContainerRehearsal{}, fmt.Errorf(
			"stored Docker container rehearsal is invalid: %w", err)
	}
	return value, nil
}

func scanDockerContainerRehearsal(row scanner) (sandbox.DockerContainerRehearsal, error) {
	var value sandbox.DockerContainerRehearsal
	var resultProtocol, resultStatus, createdAt string
	var configurationMatched, containerCreated, containerInspected, containerRemoved int
	var containerNeverStarted, processNeverExecuted, imageNeverPulled, outputNeverExported int
	var cleanupConfirmed, daemonReachable, daemonWriteSubmitted int
	var productionExecutionSubmitted, productionVerified, backendEnabled int
	var executionAuthorized, artifactCommitAuthorized int
	if err := row.Scan(&value.ID, &value.PlanID, &value.ObservationID, &value.EvidenceID,
		&value.OutputSimulationID, &value.PreflightID, &value.ExecutionID, &value.CandidateID,
		&value.PreparationID, &value.RunID, &value.MissionID, &value.WorkspaceID,
		&value.ProtocolVersion, &value.Source, &value.TrustClass, &value.Status,
		&value.ManifestFingerprint, &value.AuthorizationFingerprint, &value.PolicyFingerprint,
		&value.MountBindingFingerprint, &value.InputArtifactDigest,
		&value.ThreatModelFingerprint, &value.OutputPlanFingerprint,
		&value.ObservationFingerprint, &value.AuthorityFingerprint, &value.SpecFingerprint,
		&value.PlanFingerprint, &value.ImageDigest, &value.NetworkMode,
		&value.EnvironmentCount, &value.SecretReferenceCount, &value.RequestFingerprint,
		&value.EndpointClass, &value.EndpointFingerprint, &value.ContainerIDFingerprint,
		&value.InspectionFingerprint, &value.TransportFingerprint,
		&value.RehearsalFingerprint, &resultProtocol, &resultStatus, &value.StepCount,
		&value.DaemonReadCount, &value.DaemonWriteCount, &value.ReconciledContainerCount,
		&configurationMatched, &containerCreated, &containerInspected, &containerRemoved,
		&containerNeverStarted, &processNeverExecuted, &imageNeverPulled,
		&outputNeverExported, &cleanupConfirmed, &daemonReachable, &daemonWriteSubmitted,
		&productionExecutionSubmitted, &productionVerified, &backendEnabled,
		&executionAuthorized, &artifactCommitAuthorized, &value.RequestedBy,
		&createdAt); err != nil {
		return sandbox.DockerContainerRehearsal{}, err
	}
	value.CreatedAt = parseTS(createdAt)
	value.ConfigurationMatched = configurationMatched != 0
	value.ContainerNeverStarted = containerNeverStarted != 0
	value.ProcessNeverExecuted = processNeverExecuted != 0
	value.ImageNeverPulled = imageNeverPulled != 0
	value.OutputNeverExported = outputNeverExported != 0
	value.CleanupConfirmed = cleanupConfirmed != 0
	value.DaemonReachable = daemonReachable != 0
	value.DaemonWriteSubmitted = daemonWriteSubmitted != 0
	value.ProductionExecutionSubmitted = productionExecutionSubmitted != 0
	value.ProductionVerified = productionVerified != 0
	value.BackendEnabled = backendEnabled != 0
	value.ExecutionAuthorized = executionAuthorized != 0
	value.ArtifactCommitAuthorized = artifactCommitAuthorized != 0
	value.Result = sandbox.DockerContainerWriteResult{
		ProtocolVersion: resultProtocol, Source: value.Source, Status: resultStatus,
		EndpointClass: value.EndpointClass, EndpointFingerprint: value.EndpointFingerprint,
		RequestFingerprint: value.RequestFingerprint, SpecFingerprint: value.SpecFingerprint,
		ContainerIDFingerprint: value.ContainerIDFingerprint,
		InspectionFingerprint:  value.InspectionFingerprint,
		TransportFingerprint:   value.TransportFingerprint, StepCount: value.StepCount,
		DaemonReadCount: value.DaemonReadCount, DaemonWriteCount: value.DaemonWriteCount,
		ReconciledContainerCount: value.ReconciledContainerCount,
		ConfigurationMatched:     value.ConfigurationMatched, ContainerCreated: containerCreated != 0,
		ContainerInspected: containerInspected != 0, ContainerRemoved: containerRemoved != 0,
		ContainerStarted: !value.ContainerNeverStarted,
		ProcessExecuted:  !value.ProcessNeverExecuted, ImagePulled: !value.ImageNeverPulled,
		OutputExported: !value.OutputNeverExported, CleanupConfirmed: value.CleanupConfirmed,
		DaemonReachable:              value.DaemonReachable,
		DaemonWriteSubmitted:         value.DaemonWriteSubmitted,
		ProductionExecutionSubmitted: value.ProductionExecutionSubmitted,
		ProductionVerified:           value.ProductionVerified, BackendEnabled: value.BackendEnabled,
		ExecutionAuthorized:      value.ExecutionAuthorized,
		ArtifactCommitAuthorized: value.ArtifactCommitAuthorized,
	}
	return value, nil
}

func listDockerContainerRehearsalSteps(ctx context.Context, queryer sandboxLifecycleQueryer,
	rehearsalID string,
) ([]sandbox.DockerContainerWriteStep, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, state, daemon_reads,
		daemon_writes, production_applied, step_digest
		FROM sandbox_docker_container_rehearsal_steps
		WHERE rehearsal_id = ? ORDER BY ordinal`, rehearsalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []sandbox.DockerContainerWriteStep
	for rows.Next() {
		var step sandbox.DockerContainerWriteStep
		var applied int
		if err := rows.Scan(&step.Ordinal, &step.Name, &step.State, &step.DaemonReads,
			&step.DaemonWrites, &applied, &step.StepDigest); err != nil {
			return nil, err
		}
		step.ProductionApplied = applied != 0
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func getDockerContainerRehearsalOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerContainerRehearsalOperation, bool, error) {
	var operation sandbox.DockerContainerRehearsalOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		rehearsal_id, plan_id, run_id, requested_by, created_at
		FROM sandbox_docker_container_rehearsal_operations WHERE operation_key_digest = ?`,
		keyDigest).Scan(&operation.KeyDigest, &operation.RequestFingerprint,
		&operation.RehearsalID, &operation.PlanID, &operation.RunID,
		&operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerContainerRehearsalOperation{}, false, nil
	}
	if err != nil {
		return sandbox.DockerContainerRehearsalOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.Validate(); err != nil {
		return sandbox.DockerContainerRehearsalOperation{}, false,
			fmt.Errorf("stored Docker rehearsal operation is invalid: %w", err)
	}
	return operation, true, nil
}

func replayDockerContainerRehearsal(ctx context.Context, queryer sandboxLifecycleQueryer,
	existing, requested sandbox.DockerContainerRehearsalOperation,
) (sandbox.DockerContainerRehearsal, bool, error) {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.PlanID != requested.PlanID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return sandbox.DockerContainerRehearsal{}, false, apperror.New(apperror.CodeConflict,
			"Docker rehearsal operation key was used for different intent")
	}
	value, err := getDockerContainerRehearsal(ctx, queryer, existing.RehearsalID)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, false, err
	}
	if existing.RequestFingerprint != sandbox.DockerContainerRehearsalRequestFingerprint(value) ||
		existing.PlanID != value.PlanID || existing.RunID != value.RunID ||
		existing.RequestedBy != value.RequestedBy || !existing.CreatedAt.Equal(value.CreatedAt) {
		return sandbox.DockerContainerRehearsal{}, false, apperror.New(apperror.CodeInternal,
			"stored Docker rehearsal operation binding is invalid")
	}
	value.Replayed = true
	return value, true, nil
}

func (s *SQLiteStore) recoverDockerContainerRehearsalCreate(ctx context.Context,
	operation sandbox.DockerContainerRehearsalOperation, cause error,
) (sandbox.DockerContainerRehearsal, bool, error) {
	existing, found, err := getDockerContainerRehearsalOperation(ctx, s.db,
		operation.KeyDigest)
	if err != nil {
		return sandbox.DockerContainerRehearsal{}, false, errors.Join(cause, err)
	}
	if !found {
		return sandbox.DockerContainerRehearsal{}, false, cause
	}
	return replayDockerContainerRehearsal(ctx, s.db, existing, operation)
}

func appendDockerContainerRehearsalEvent(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerContainerRehearsal,
) error {
	event, err := events.New(value.RunID, value.MissionID,
		events.SandboxDockerRehearsalRecordedEvent,
		"sandbox_docker_container_rehearsal", value.ID, map[string]any{
			"protocol_version":               value.ProtocolVersion,
			"status":                         value.Status,
			"trust_class":                    value.TrustClass,
			"endpoint_class":                 value.EndpointClass,
			"daemon_reads":                   value.DaemonReadCount,
			"daemon_writes":                  value.DaemonWriteCount,
			"reconciled_containers":          value.ReconciledContainerCount,
			"configuration_matched":          true,
			"container_started":              false,
			"process_executed":               false,
			"image_pulled":                   false,
			"output_exported":                false,
			"cleanup_confirmed":              true,
			"production_execution_submitted": false,
			"production_verified":            false,
			"backend_enabled":                false,
			"execution_authorized":           false,
			"artifact_commit_authorized":     false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = value.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
