package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/sandbox"
)

const dockerObservationSelect = `SELECT id, evidence_id, output_simulation_id, preflight_id,
	execution_id, candidate_id, preparation_id, run_id, mission_id, workspace_id,
	manifest_fingerprint, authorization_fingerprint, policy_fingerprint,
	mount_binding_fingerprint, input_artifact_digest, threat_model_fingerprint,
	output_plan_fingerprint,
	protocol_version, source, trust_class, status, endpoint_class, endpoint_fingerprint,
	binding_fingerprint, image_digest, failure_code, daemon_reachable, image_inspected,
	observation_complete, production_observed, production_verified, backend_available,
	backend_enabled, execution_authorized, artifact_commit_authorized, api_version,
	min_api_version, engine_version, os_type, architecture, rootless,
	user_namespace_enabled, private_mount_state, cgroup_version, ncpu, memory_bytes,
	pids_limit_supported, image_os_type, image_architecture, image_size_bytes,
	image_user_state, daemon_identity_fingerprint, capability_fingerprint,
	image_fingerprint, observation_fingerprint, item_count, observed_count,
	verified_count, requested_by, created_at FROM sandbox_docker_observations`

func (s *SQLiteStore) GetDockerObservationOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerObservationOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerObservationOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker observation operation digest is invalid")
	}
	return getDockerObservationOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateDockerObservation(ctx context.Context,
	observation sandbox.DockerObservation, operation sandbox.DockerObservationOperation,
) (sandbox.DockerObservation, bool, error) {
	if err := validateDockerObservationMutation(observation, operation); err != nil {
		return sandbox.DockerObservation{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.DockerObservation{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerObservation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, observation.RunID); err != nil {
		return sandbox.DockerObservation{}, false, err
	}
	if existing, found, err := getDockerObservationOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.DockerObservation{}, false, err
	} else if found {
		return replayDockerObservation(ctx, tx, existing, operation)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_docker_observations
		WHERE output_simulation_id = ?`, observation.OutputSimulationID).Scan(&count); err != nil {
		return sandbox.DockerObservation{}, false, err
	}
	if count >= sandbox.MaxDockerObservationsPerSimulation {
		return sandbox.DockerObservation{}, false, apperror.New(apperror.CodeResourceExhausted,
			"Docker observation limit is exhausted for this output simulation")
	}
	if err := validateDockerObservationCurrentTx(ctx, tx, observation); err != nil {
		return sandbox.DockerObservation{}, false, err
	}
	if err := insertDockerObservationTx(ctx, tx, observation); err != nil {
		_ = tx.Rollback()
		return s.recoverDockerObservationCreate(ctx, operation, err)
	}
	for _, item := range observation.Report.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_observation_items
			(observation_id, ordinal, name, state, evidence_digest, observed, verified)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, observation.ID, item.Ordinal, item.Name,
			item.State, item.EvidenceDigest, boolInt(item.Observed), boolInt(item.Verified)); err != nil {
			return sandbox.DockerObservation{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_observation_operations
		(operation_key_digest, request_fingerprint, observation_id, evidence_id,
		output_simulation_id, run_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest, operation.RequestFingerprint,
		operation.ObservationID, operation.EvidenceID, operation.OutputSimulationID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverDockerObservationCreate(ctx, operation, err)
	}
	if err := appendDockerObservationEvent(ctx, tx, observation); err != nil {
		return sandbox.DockerObservation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverDockerObservationCreate(ctx, operation, err)
	}
	return observation, false, nil
}

func (s *SQLiteStore) GetDockerObservation(ctx context.Context,
	id string,
) (sandbox.DockerObservation, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerObservation{}, apperror.New(apperror.CodeInvalidArgument,
			"Docker observation id is invalid")
	}
	return getDockerObservation(ctx, s.db, id)
}

func (s *SQLiteStore) ListDockerObservations(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerObservation, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker observation list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker observation list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_docker_observations
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
	values := make([]sandbox.DockerObservation, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerObservation(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func validateDockerObservationMutation(observation sandbox.DockerObservation,
	operation sandbox.DockerObservationOperation,
) error {
	if err := observation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "Docker observation is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Docker observation operation is invalid", err)
	}
	if operation.ObservationID != observation.ID || operation.EvidenceID != observation.EvidenceID ||
		operation.OutputSimulationID != observation.OutputSimulationID ||
		operation.RunID != observation.RunID || operation.RequestedBy != observation.RequestedBy ||
		!operation.CreatedAt.Equal(observation.CreatedAt) ||
		operation.RequestFingerprint != sandbox.DockerObservationRequestFingerprint(observation) {
		return apperror.New(apperror.CodeConflict,
			"Docker observation operation does not match its request")
	}
	return nil
}

func validateDockerObservationCurrentTx(ctx context.Context, tx *sql.Tx,
	observation sandbox.DockerObservation,
) error {
	evidence, err := getSandboxBackendEvidence(ctx, tx, observation.EvidenceID)
	if err != nil {
		return err
	}
	preflight, err := validateSandboxBackendEvidenceCurrentTx(ctx, tx, evidence)
	if err != nil {
		return err
	}
	simulation, err := getSandboxOutputSimulation(ctx, tx, observation.OutputSimulationID)
	if err != nil {
		return err
	}
	if err := validateSandboxOutputSimulationCurrent(simulation, evidence, preflight); err != nil {
		return err
	}
	if observation.EvidenceID != evidence.ID ||
		observation.OutputSimulationID != simulation.ID ||
		observation.PreflightID != evidence.PreflightID ||
		observation.ExecutionID != evidence.ExecutionID ||
		observation.CandidateID != evidence.CandidateID ||
		observation.PreparationID != evidence.PreparationID ||
		observation.RunID != evidence.RunID || observation.MissionID != evidence.MissionID ||
		observation.WorkspaceID != evidence.WorkspaceID ||
		observation.ManifestFingerprint != evidence.ManifestFingerprint ||
		observation.AuthorizationFingerprint != evidence.AuthorizationFingerprint ||
		observation.PolicyFingerprint != evidence.PolicyFingerprint ||
		observation.MountBindingFingerprint != evidence.MountBindingFingerprint ||
		observation.InputArtifactDigest != evidence.InputArtifactDigest ||
		observation.ThreatModelFingerprint != evidence.ThreatModelFingerprint ||
		observation.OutputPlanFingerprint != evidence.Report.OutputPlanFingerprint ||
		observation.OutputPlanFingerprint != simulation.OutputPlanFingerprint ||
		observation.Report.ImageDigest != evidence.Report.ImageDigest ||
		observation.Report.BindingFingerprint != sandbox.DockerObservationBindingFingerprint(observation) ||
		observation.RequestedBy != evidence.RequestedBy ||
		observation.RequestedBy != simulation.RequestedBy ||
		evidence.Report.ProductionVerified || evidence.Report.BackendAvailable ||
		evidence.Report.BackendEnabled || evidence.Report.ExecutionAuthorized ||
		evidence.Report.ArtifactCommitAuthorized || !simulation.SimulationOnly ||
		simulation.ProductionArtifactCount != 0 || simulation.ArtifactCommitAuthorized ||
		simulation.BackendEnabled || simulation.ExecutionAuthorized {
		return apperror.New(apperror.CodeConflict,
			"Docker observation does not match the current v48-v52 authority chain")
	}
	return nil
}

func insertDockerObservationTx(ctx context.Context, tx *sql.Tx,
	observation sandbox.DockerObservation,
) error {
	report := observation.Report
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_observations
		(id, evidence_id, output_simulation_id, preflight_id, execution_id, candidate_id,
		preparation_id, run_id, mission_id, workspace_id, manifest_fingerprint,
		authorization_fingerprint, policy_fingerprint, mount_binding_fingerprint,
		input_artifact_digest, threat_model_fingerprint, output_plan_fingerprint,
		protocol_version, source, trust_class,
		status, endpoint_class, endpoint_fingerprint, binding_fingerprint, image_digest,
		failure_code, daemon_reachable, image_inspected, observation_complete,
		production_observed, production_verified, backend_available, backend_enabled,
		execution_authorized, artifact_commit_authorized, api_version, min_api_version,
		engine_version, os_type, architecture, rootless, user_namespace_enabled,
		private_mount_state, cgroup_version, ncpu, memory_bytes, pids_limit_supported,
		image_os_type, image_architecture, image_size_bytes, image_user_state,
		daemon_identity_fingerprint, capability_fingerprint, image_fingerprint,
		observation_fingerprint, item_count, observed_count, verified_count, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?)`,
		observation.ID, observation.EvidenceID, observation.OutputSimulationID,
		observation.PreflightID, observation.ExecutionID, observation.CandidateID,
		observation.PreparationID, observation.RunID, observation.MissionID,
		observation.WorkspaceID, observation.ManifestFingerprint,
		observation.AuthorizationFingerprint, observation.PolicyFingerprint,
		observation.MountBindingFingerprint, observation.InputArtifactDigest,
		observation.ThreatModelFingerprint, observation.OutputPlanFingerprint,
		report.ProtocolVersion, report.Source, report.TrustClass,
		report.Status, report.EndpointClass, report.EndpointFingerprint,
		report.BindingFingerprint, report.ImageDigest, report.FailureCode,
		boolInt(report.DaemonReachable), boolInt(report.ImageInspected),
		boolInt(report.ObservationComplete), boolInt(report.ProductionObserved),
		boolInt(report.ProductionVerified), boolInt(report.BackendAvailable),
		boolInt(report.BackendEnabled), boolInt(report.ExecutionAuthorized),
		boolInt(report.ArtifactCommitAuthorized), report.APIVersion, report.MinAPIVersion,
		report.EngineVersion, report.OSType, report.Architecture, boolInt(report.Rootless),
		boolInt(report.UserNamespaceEnabled), report.PrivateMountState, report.CgroupVersion,
		report.NCPU, report.MemoryBytes, boolInt(report.PidsLimitSupported), report.ImageOSType,
		report.ImageArchitecture, report.ImageSizeBytes, report.ImageUserState,
		report.DaemonIdentityFingerprint, report.CapabilityFingerprint,
		report.ImageFingerprint, report.ObservationFingerprint, len(report.Items),
		countObservedObservationItems(report.Items), 0, observation.RequestedBy,
		ts(observation.CreatedAt))
	return err
}

func countObservedObservationItems(items []sandbox.DockerObservationItem) int {
	count := 0
	for _, item := range items {
		if item.Observed {
			count++
		}
	}
	return count
}

func getDockerObservation(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.DockerObservation, error) {
	observation, err := scanDockerObservation(queryer.QueryRowContext(ctx,
		dockerObservationSelect+` WHERE id = ?`, id))
	if err != nil {
		return sandbox.DockerObservation{}, err
	}
	items, err := listDockerObservationItems(ctx, queryer, id)
	if err != nil {
		return sandbox.DockerObservation{}, err
	}
	observation.Report.Items = items
	if err := observation.Validate(); err != nil {
		return sandbox.DockerObservation{}, apperror.Wrap(apperror.CodeInternal,
			"stored Docker observation is invalid", err)
	}
	return observation, nil
}

func scanDockerObservation(row scanner) (sandbox.DockerObservation, error) {
	var observation sandbox.DockerObservation
	var daemonReachable, imageInspected, observationComplete, productionObserved int
	var productionVerified, backendAvailable, backendEnabled, executionAuthorized int
	var artifactCommitAuthorized, rootless, userNamespace, pidsLimit int
	var itemCount, observedCount, verifiedCount int
	var createdAt string
	err := row.Scan(&observation.ID, &observation.EvidenceID,
		&observation.OutputSimulationID, &observation.PreflightID, &observation.ExecutionID,
		&observation.CandidateID, &observation.PreparationID, &observation.RunID,
		&observation.MissionID, &observation.WorkspaceID,
		&observation.ManifestFingerprint, &observation.AuthorizationFingerprint,
		&observation.PolicyFingerprint, &observation.MountBindingFingerprint,
		&observation.InputArtifactDigest, &observation.ThreatModelFingerprint,
		&observation.OutputPlanFingerprint,
		&observation.Report.ProtocolVersion, &observation.Report.Source,
		&observation.Report.TrustClass, &observation.Report.Status,
		&observation.Report.EndpointClass, &observation.Report.EndpointFingerprint,
		&observation.Report.BindingFingerprint, &observation.Report.ImageDigest,
		&observation.Report.FailureCode, &daemonReachable, &imageInspected,
		&observationComplete, &productionObserved, &productionVerified, &backendAvailable,
		&backendEnabled, &executionAuthorized, &artifactCommitAuthorized,
		&observation.Report.APIVersion, &observation.Report.MinAPIVersion,
		&observation.Report.EngineVersion, &observation.Report.OSType,
		&observation.Report.Architecture, &rootless, &userNamespace,
		&observation.Report.PrivateMountState, &observation.Report.CgroupVersion,
		&observation.Report.NCPU, &observation.Report.MemoryBytes, &pidsLimit,
		&observation.Report.ImageOSType, &observation.Report.ImageArchitecture,
		&observation.Report.ImageSizeBytes, &observation.Report.ImageUserState,
		&observation.Report.DaemonIdentityFingerprint,
		&observation.Report.CapabilityFingerprint, &observation.Report.ImageFingerprint,
		&observation.Report.ObservationFingerprint, &itemCount, &observedCount,
		&verifiedCount, &observation.RequestedBy, &createdAt)
	if err != nil {
		return sandbox.DockerObservation{}, err
	}
	if itemCount != sandbox.MaxDockerObservationItems || verifiedCount != 0 ||
		observedCount < 0 || observedCount > sandbox.MaxDockerObservationItems {
		return sandbox.DockerObservation{}, errors.New("stored Docker observation counts are invalid")
	}
	observation.Report.DaemonReachable = daemonReachable == 1
	observation.Report.ImageInspected = imageInspected == 1
	observation.Report.ObservationComplete = observationComplete == 1
	observation.Report.ProductionObserved = productionObserved == 1
	observation.Report.ProductionVerified = productionVerified == 1
	observation.Report.BackendAvailable = backendAvailable == 1
	observation.Report.BackendEnabled = backendEnabled == 1
	observation.Report.ExecutionAuthorized = executionAuthorized == 1
	observation.Report.ArtifactCommitAuthorized = artifactCommitAuthorized == 1
	observation.Report.Rootless = rootless == 1
	observation.Report.UserNamespaceEnabled = userNamespace == 1
	observation.Report.PidsLimitSupported = pidsLimit == 1
	observation.CreatedAt = parseTS(createdAt)
	return observation, nil
}

func listDockerObservationItems(ctx context.Context, queryer sandboxLifecycleQueryer,
	observationID string,
) ([]sandbox.DockerObservationItem, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, state, evidence_digest,
		observed, verified FROM sandbox_docker_observation_items
		WHERE observation_id = ? ORDER BY ordinal`, observationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []sandbox.DockerObservationItem
	for rows.Next() {
		var item sandbox.DockerObservationItem
		var observed, verified int
		if err := rows.Scan(&item.Ordinal, &item.Name, &item.State, &item.EvidenceDigest,
			&observed, &verified); err != nil {
			return nil, err
		}
		item.Observed = observed == 1
		item.Verified = verified == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func getDockerObservationOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.DockerObservationOperation, bool, error) {
	var operation sandbox.DockerObservationOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		observation_id, evidence_id, output_simulation_id, run_id, requested_by, created_at
		FROM sandbox_docker_observation_operations WHERE operation_key_digest = ?`,
		keyDigest).Scan(&operation.KeyDigest, &operation.RequestFingerprint,
		&operation.ObservationID, &operation.EvidenceID, &operation.OutputSimulationID,
		&operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerObservationOperation{}, false, nil
	}
	if err != nil {
		return sandbox.DockerObservationOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.Validate(); err != nil {
		return sandbox.DockerObservationOperation{}, false, apperror.Wrap(
			apperror.CodeInternal, "stored Docker observation operation is invalid", err)
	}
	return operation, true, nil
}

func replayDockerObservation(ctx context.Context, queryer sandboxLifecycleQueryer,
	existing, requested sandbox.DockerObservationOperation,
) (sandbox.DockerObservation, bool, error) {
	if existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.EvidenceID != requested.EvidenceID ||
		existing.OutputSimulationID != requested.OutputSimulationID ||
		existing.RunID != requested.RunID || existing.RequestedBy != requested.RequestedBy {
		return sandbox.DockerObservation{}, false, apperror.New(apperror.CodeConflict,
			"Docker observation operation key was used for different intent")
	}
	value, err := getDockerObservation(ctx, queryer, existing.ObservationID)
	if err != nil {
		return sandbox.DockerObservation{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func (s *SQLiteStore) recoverDockerObservationCreate(ctx context.Context,
	operation sandbox.DockerObservationOperation, cause error,
) (sandbox.DockerObservation, bool, error) {
	existing, found, err := getDockerObservationOperation(ctx, s.db, operation.KeyDigest)
	if err == nil && found {
		return replayDockerObservation(ctx, s.db, existing, operation)
	}
	if err != nil {
		return sandbox.DockerObservation{}, false, errors.Join(cause, err)
	}
	return sandbox.DockerObservation{}, false, cause
}

func appendDockerObservationEvent(ctx context.Context, tx *sql.Tx,
	observation sandbox.DockerObservation,
) error {
	report := observation.Report
	event, err := events.New(observation.RunID, observation.MissionID,
		events.SandboxDockerObservationRecordedEvent, "sandbox_docker_observation", observation.ID,
		map[string]any{
			"protocol": report.ProtocolVersion, "source": report.Source,
			"trust_class": report.TrustClass, "status": report.Status,
			"endpoint_class": report.EndpointClass, "failure_code": report.FailureCode,
			"daemon_reachable":     report.DaemonReachable,
			"image_inspected":      report.ImageInspected,
			"observation_complete": report.ObservationComplete,
			"production_observed":  report.ProductionObserved,
			"observed_items":       countObservedObservationItems(report.Items),
			"production_verified":  false, "verified_items": 0,
			"backend_available": false, "backend_enabled": false,
			"execution_authorized": false, "artifact_commit_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = observation.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
