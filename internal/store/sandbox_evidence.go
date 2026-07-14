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

const sandboxBackendEvidenceSelect = `SELECT id, preflight_id, execution_id, candidate_id,
	preparation_id, run_id, mission_id, workspace_id, protocol_version, source, trust_class,
	status, backend, image_digest, manifest_fingerprint, authorization_fingerprint,
	policy_fingerprint, mount_binding_fingerprint, input_artifact_digest,
	threat_model_fingerprint, daemon_capabilities_fingerprint, mount_plan_fingerprint,
	network_plan_fingerprint, secret_plan_fingerprint, container_config_fingerprint,
	resource_plan_fingerprint, termination_plan_fingerprint, orphan_plan_fingerprint,
	output_plan_fingerprint, evidence_fingerprint, evidence_count, satisfied_count,
	verified_count, production_verified, backend_available, backend_enabled,
	execution_authorized, artifact_commit_authorized, requested_by, created_at
	FROM sandbox_backend_evidence`

const sandboxOutputSimulationSelect = `SELECT id, evidence_id, preflight_id, execution_id,
	run_id, mission_id, workspace_id, protocol_version, status, output_plan_fingerprint,
	fixture_digest, transaction_digest, expected_slot_count, staged_output_count,
	staged_output_bytes, fake_artifact_count, production_artifact_count, all_or_nothing,
	simulation_only, artifact_commit_authorized, backend_enabled, execution_authorized,
	requested_by, created_at FROM sandbox_output_simulations`

func (s *SQLiteStore) GetSandboxBackendEvidenceOperation(ctx context.Context,
	keyDigest string,
) (sandbox.BackendEvidenceOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.BackendEvidenceOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "sandbox backend evidence operation digest is invalid")
	}
	return getSandboxBackendEvidenceOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateSandboxBackendEvidence(ctx context.Context,
	evidence sandbox.BackendEvidence, operation sandbox.BackendEvidenceOperation,
) (sandbox.BackendEvidence, bool, error) {
	if err := validateSandboxBackendEvidenceMutation(evidence, operation); err != nil {
		return sandbox.BackendEvidence{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.BackendEvidence{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.BackendEvidence{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, evidence.RunID); err != nil {
		return sandbox.BackendEvidence{}, false, err
	}
	if existing, found, err := getSandboxBackendEvidenceOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.BackendEvidence{}, false, err
	} else if found {
		return replaySandboxBackendEvidence(ctx, tx, existing, operation)
	}
	if _, err := getSandboxBackendEvidenceByPreflight(ctx, tx, evidence.PreflightID); err == nil {
		return sandbox.BackendEvidence{}, false, apperror.New(apperror.CodeConflict,
			"sandbox preflight already has backend evidence")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return sandbox.BackendEvidence{}, false, err
	}
	if _, err := validateSandboxBackendEvidenceCurrentTx(ctx, tx, evidence); err != nil {
		return sandbox.BackendEvidence{}, false, err
	}
	if err := insertSandboxBackendEvidenceTx(ctx, tx, evidence); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxBackendEvidenceCreate(ctx, operation, err)
	}
	for _, item := range evidence.Report.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_backend_evidence_items
			(evidence_id, ordinal, name, evidence_state, evidence_digest, satisfied, verified)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, evidence.ID, item.Ordinal, item.Name,
			item.EvidenceState, item.EvidenceDigest, boolInt(item.Satisfied),
			boolInt(item.Verified)); err != nil {
			return sandbox.BackendEvidence{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_backend_evidence_operations
		(operation_key_digest, request_fingerprint, evidence_id, preflight_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.EvidenceID, operation.PreflightID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxBackendEvidenceCreate(ctx, operation, err)
	}
	if err := appendSandboxBackendEvidenceEvent(ctx, tx, evidence); err != nil {
		return sandbox.BackendEvidence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxBackendEvidenceCreate(ctx, operation, err)
	}
	return evidence, false, nil
}

func (s *SQLiteStore) GetSandboxBackendEvidence(ctx context.Context,
	id string,
) (sandbox.BackendEvidence, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.BackendEvidence{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox backend evidence id is invalid")
	}
	return getSandboxBackendEvidence(ctx, s.db, id)
}

func (s *SQLiteStore) ListSandboxBackendEvidence(ctx context.Context,
	runID string, limit int,
) ([]sandbox.BackendEvidence, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox backend evidence list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox backend evidence list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_backend_evidence
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
	values := make([]sandbox.BackendEvidence, 0, len(ids))
	for _, id := range ids {
		value, err := getSandboxBackendEvidence(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (s *SQLiteStore) GetSandboxOutputSimulationOperation(ctx context.Context,
	keyDigest string,
) (sandbox.OutputSimulationOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.OutputSimulationOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "sandbox output simulation operation digest is invalid")
	}
	return getSandboxOutputSimulationOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateSandboxOutputSimulation(ctx context.Context,
	simulation sandbox.OutputSimulation, operation sandbox.OutputSimulationOperation,
) (sandbox.OutputSimulation, bool, error) {
	if err := validateSandboxOutputSimulationMutation(simulation, operation); err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return sandbox.OutputSimulation{}, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, simulation.RunID); err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	if existing, found, err := getSandboxOutputSimulationOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return sandbox.OutputSimulation{}, false, err
	} else if found {
		return replaySandboxOutputSimulation(ctx, tx, existing, operation)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_output_simulations
		WHERE evidence_id = ?`, simulation.EvidenceID).Scan(&count); err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	if count >= sandbox.MaxOutputSimulationsPerEvidence {
		return sandbox.OutputSimulation{}, false, apperror.New(apperror.CodeResourceExhausted,
			"sandbox output simulation limit is exhausted for this evidence")
	}
	evidence, err := getSandboxBackendEvidence(ctx, tx, simulation.EvidenceID)
	if err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	preflight, err := validateSandboxBackendEvidenceCurrentTx(ctx, tx, evidence)
	if err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	if err := validateSandboxOutputSimulationCurrent(simulation, evidence, preflight); err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	if err := insertSandboxOutputSimulationTx(ctx, tx, simulation); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxOutputSimulationCreate(ctx, operation, err)
	}
	for _, descriptor := range simulation.Descriptors {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_output_simulation_items
			(simulation_id, ordinal, kind, locator_fingerprint, mime, sha256, size_bytes,
			redacted, fake_artifact_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			simulation.ID, descriptor.Ordinal, descriptor.Kind, descriptor.LocatorFingerprint,
			descriptor.MIME, descriptor.SHA256, descriptor.SizeBytes,
			boolInt(descriptor.Redacted), descriptor.FakeArtifactFingerprint); err != nil {
			return sandbox.OutputSimulation{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_output_simulation_operations
		(operation_key_digest, request_fingerprint, simulation_id, evidence_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.SimulationID, operation.EvidenceID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSandboxOutputSimulationCreate(ctx, operation, err)
	}
	if err := appendSandboxOutputSimulationEvent(ctx, tx, simulation); err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return s.recoverSandboxOutputSimulationCreate(ctx, operation, err)
	}
	return simulation, false, nil
}

func (s *SQLiteStore) GetSandboxOutputSimulation(ctx context.Context,
	id string,
) (sandbox.OutputSimulation, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.OutputSimulation{}, apperror.New(apperror.CodeInvalidArgument,
			"sandbox output simulation id is invalid")
	}
	return getSandboxOutputSimulation(ctx, s.db, id)
}

func (s *SQLiteStore) ListSandboxOutputSimulations(ctx context.Context,
	runID string, limit int,
) ([]sandbox.OutputSimulation, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox output simulation list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"sandbox output simulation list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sandbox_output_simulations
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
	values := make([]sandbox.OutputSimulation, 0, len(ids))
	for _, id := range ids {
		value, err := getSandboxOutputSimulation(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func validateSandboxBackendEvidenceMutation(evidence sandbox.BackendEvidence,
	operation sandbox.BackendEvidenceOperation,
) error {
	if err := evidence.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "sandbox backend evidence is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox backend evidence operation is invalid", err)
	}
	if operation.EvidenceID != evidence.ID || operation.PreflightID != evidence.PreflightID ||
		operation.RunID != evidence.RunID || operation.RequestedBy != evidence.RequestedBy ||
		!operation.CreatedAt.Equal(evidence.CreatedAt) ||
		operation.RequestFingerprint != sandbox.BackendEvidenceRequestFingerprint(evidence) {
		return apperror.New(apperror.CodeConflict,
			"sandbox backend evidence operation does not match its request")
	}
	return nil
}

func validateSandboxOutputSimulationMutation(simulation sandbox.OutputSimulation,
	operation sandbox.OutputSimulationOperation,
) error {
	if err := simulation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "sandbox output simulation is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output simulation operation is invalid", err)
	}
	if operation.SimulationID != simulation.ID || operation.EvidenceID != simulation.EvidenceID ||
		operation.RunID != simulation.RunID || operation.RequestedBy != simulation.RequestedBy ||
		!operation.CreatedAt.Equal(simulation.CreatedAt) ||
		operation.RequestFingerprint != sandbox.OutputSimulationRequestFingerprint(simulation) {
		return apperror.New(apperror.CodeConflict,
			"sandbox output simulation operation does not match its request")
	}
	return nil
}

func validateSandboxBackendEvidenceCurrentTx(ctx context.Context, tx *sql.Tx,
	evidence sandbox.BackendEvidence,
) (sandbox.DisabledPreflight, error) {
	preflight, err := getSandboxDisabledPreflight(ctx, tx, evidence.PreflightID)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	lifecycle, err := getSandboxLifecycle(ctx, tx, preflight.ExecutionID)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	candidate, err := getSandboxExecutionCandidate(ctx, tx, preflight.CandidateID)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	intent, err := getSandboxManifestIntent(ctx, tx, preflight.PreparationID)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	run, mission, err := getCoordinatorRunTx(ctx, tx, preflight.RunID)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	if err := validateSandboxPreflightCurrentBinding(preflight, lifecycle, candidate,
		intent, run, mission); err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	if evidence.PreflightID != preflight.ID || evidence.ExecutionID != preflight.ExecutionID ||
		evidence.CandidateID != preflight.CandidateID ||
		evidence.PreparationID != preflight.PreparationID || evidence.RunID != preflight.RunID ||
		evidence.MissionID != preflight.MissionID || evidence.WorkspaceID != preflight.WorkspaceID ||
		evidence.ManifestFingerprint != preflight.ManifestFingerprint ||
		evidence.AuthorizationFingerprint != preflight.AuthorizationFingerprint ||
		evidence.PolicyFingerprint != preflight.PolicyFingerprint ||
		evidence.MountBindingFingerprint != preflight.MountBindingFingerprint ||
		evidence.InputArtifactDigest != preflight.InputArtifactDigest ||
		evidence.ThreatModelFingerprint != preflight.Handshake.ThreatModelFingerprint ||
		evidence.Report.Backend != preflight.Backend ||
		evidence.Report.OutputPlanFingerprint != preflight.OutputPlan.Fingerprint ||
		evidence.RequestedBy != preflight.RequestedBy {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeConflict,
			"sandbox backend evidence does not match the v48-v51 authority chain")
	}
	if err := validateSandboxCandidateApprovalTx(ctx, tx, run, intent, candidate); err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	usage, err := getRunAgentUsageTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	toolCalls, err := getRunToolCallCountTx(ctx, tx, run.ID)
	if err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	if usage.TotalTokens != candidate.TokensUsed ||
		usage.TotalExecutionMillis != candidate.ExecutionMillisUsed ||
		toolCalls != candidate.ToolCallsUsed {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeConflict,
			"sandbox candidate usage changed before backend evidence")
	}
	if err := requireSandboxCandidateStoreBudget(run.Budget, usage, toolCalls); err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	if lease, found, err := getRunExecutionLeaseTx(ctx, tx, run.ID); err != nil {
		return sandbox.DisabledPreflight{}, err
	} else if found && lease.ActiveAt(time.Now().UTC()) {
		return sandbox.DisabledPreflight{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox evidence requires a quiescent Run")
	}
	if err := verifySandboxInputBindingsTx(ctx, tx, lifecycle.Execution,
		lifecycle.Inputs, run.SessionID); err != nil {
		return sandbox.DisabledPreflight{}, err
	}
	return preflight, nil
}

func validateSandboxOutputSimulationCurrent(simulation sandbox.OutputSimulation,
	evidence sandbox.BackendEvidence, preflight sandbox.DisabledPreflight,
) error {
	if simulation.EvidenceID != evidence.ID || simulation.PreflightID != preflight.ID ||
		simulation.ExecutionID != preflight.ExecutionID || simulation.RunID != preflight.RunID ||
		simulation.MissionID != preflight.MissionID ||
		simulation.WorkspaceID != preflight.WorkspaceID ||
		simulation.OutputPlanFingerprint != preflight.OutputPlan.Fingerprint ||
		simulation.OutputPlanFingerprint != evidence.Report.OutputPlanFingerprint ||
		simulation.ExpectedSlotCount != len(preflight.OutputPlan.Slots) ||
		simulation.RequestedBy != evidence.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"sandbox output simulation does not match its evidence and preflight")
	}
	for index, descriptor := range simulation.Descriptors {
		slot := preflight.OutputPlan.Slots[index]
		if descriptor.Ordinal != slot.Ordinal || descriptor.Kind != slot.Kind ||
			descriptor.LocatorFingerprint != slot.LocatorFingerprint {
			return apperror.New(apperror.CodeConflict,
				"sandbox output simulation descriptor does not match its opaque slot")
		}
	}
	return nil
}

func insertSandboxBackendEvidenceTx(ctx context.Context, tx *sql.Tx,
	evidence sandbox.BackendEvidence,
) error {
	report := evidence.Report
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_backend_evidence
		(id, preflight_id, execution_id, candidate_id, preparation_id, run_id, mission_id,
		workspace_id, protocol_version, source, trust_class, status, backend, image_digest,
		manifest_fingerprint, authorization_fingerprint, policy_fingerprint,
		mount_binding_fingerprint, input_artifact_digest, threat_model_fingerprint,
		daemon_capabilities_fingerprint, mount_plan_fingerprint, network_plan_fingerprint,
		secret_plan_fingerprint, container_config_fingerprint, resource_plan_fingerprint,
		termination_plan_fingerprint, orphan_plan_fingerprint, output_plan_fingerprint,
		evidence_fingerprint, evidence_count, satisfied_count, verified_count,
		production_verified, backend_available, backend_enabled, execution_authorized,
		artifact_commit_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, evidence.ID, evidence.PreflightID,
		evidence.ExecutionID, evidence.CandidateID, evidence.PreparationID, evidence.RunID,
		evidence.MissionID, evidence.WorkspaceID, report.ProtocolVersion, report.Source,
		report.TrustClass, report.Status, report.Backend, report.ImageDigest,
		evidence.ManifestFingerprint, evidence.AuthorizationFingerprint,
		evidence.PolicyFingerprint, evidence.MountBindingFingerprint,
		evidence.InputArtifactDigest, evidence.ThreatModelFingerprint,
		report.DaemonCapabilitiesFingerprint, report.MountPlanFingerprint,
		report.NetworkPlanFingerprint, report.SecretPlanFingerprint,
		report.ContainerConfigFingerprint, report.ResourcePlanFingerprint,
		report.TerminationPlanFingerprint, report.OrphanPlanFingerprint,
		report.OutputPlanFingerprint, report.EvidenceFingerprint, len(report.Items),
		len(report.Items), 0, boolInt(report.ProductionVerified), boolInt(report.BackendAvailable),
		boolInt(report.BackendEnabled), boolInt(report.ExecutionAuthorized),
		boolInt(report.ArtifactCommitAuthorized), evidence.RequestedBy, ts(evidence.CreatedAt))
	return err
}

func insertSandboxOutputSimulationTx(ctx context.Context, tx *sql.Tx,
	simulation sandbox.OutputSimulation,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_output_simulations
		(id, evidence_id, preflight_id, execution_id, run_id, mission_id, workspace_id,
		protocol_version, status, output_plan_fingerprint, fixture_digest, transaction_digest,
		expected_slot_count, staged_output_count, staged_output_bytes, fake_artifact_count,
		production_artifact_count, all_or_nothing, simulation_only, artifact_commit_authorized,
		backend_enabled, execution_authorized, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		simulation.ID, simulation.EvidenceID, simulation.PreflightID, simulation.ExecutionID,
		simulation.RunID, simulation.MissionID, simulation.WorkspaceID,
		simulation.ProtocolVersion, simulation.Status, simulation.OutputPlanFingerprint,
		simulation.FixtureDigest, simulation.TransactionDigest, simulation.ExpectedSlotCount,
		simulation.StagedOutputCount, simulation.StagedOutputBytes,
		simulation.FakeArtifactCount, simulation.ProductionArtifactCount,
		boolInt(simulation.AllOrNothing), boolInt(simulation.SimulationOnly),
		boolInt(simulation.ArtifactCommitAuthorized), boolInt(simulation.BackendEnabled),
		boolInt(simulation.ExecutionAuthorized), simulation.RequestedBy, ts(simulation.CreatedAt))
	return err
}

func getSandboxBackendEvidence(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.BackendEvidence, error) {
	evidence, err := scanSandboxBackendEvidence(queryer.QueryRowContext(ctx,
		sandboxBackendEvidenceSelect+` WHERE id = ?`, id))
	if err != nil {
		return sandbox.BackendEvidence{}, err
	}
	items, err := listSandboxBackendEvidenceItems(ctx, queryer, id)
	if err != nil {
		return sandbox.BackendEvidence{}, err
	}
	evidence.Report.Items = items
	if err := evidence.Validate(); err != nil {
		return sandbox.BackendEvidence{}, apperror.Wrap(apperror.CodeInternal,
			"stored sandbox backend evidence is invalid", err)
	}
	return evidence, nil
}

func getSandboxBackendEvidenceByPreflight(ctx context.Context, queryer sandboxLifecycleQueryer,
	preflightID string,
) (sandbox.BackendEvidence, error) {
	var id string
	if err := queryer.QueryRowContext(ctx, `SELECT id FROM sandbox_backend_evidence
		WHERE preflight_id = ?`, preflightID).Scan(&id); err != nil {
		return sandbox.BackendEvidence{}, err
	}
	return getSandboxBackendEvidence(ctx, queryer, id)
}

func scanSandboxBackendEvidence(row scanner) (sandbox.BackendEvidence, error) {
	var evidence sandbox.BackendEvidence
	var evidenceCount, satisfiedCount, verifiedCount int
	var productionVerified, backendAvailable, backendEnabled, executionAuthorized, commitAuthorized int
	var createdAt string
	err := row.Scan(&evidence.ID, &evidence.PreflightID, &evidence.ExecutionID,
		&evidence.CandidateID, &evidence.PreparationID, &evidence.RunID, &evidence.MissionID,
		&evidence.WorkspaceID, &evidence.Report.ProtocolVersion, &evidence.Report.Source,
		&evidence.Report.TrustClass, &evidence.Report.Status, &evidence.Report.Backend,
		&evidence.Report.ImageDigest, &evidence.ManifestFingerprint,
		&evidence.AuthorizationFingerprint, &evidence.PolicyFingerprint,
		&evidence.MountBindingFingerprint, &evidence.InputArtifactDigest,
		&evidence.ThreatModelFingerprint, &evidence.Report.DaemonCapabilitiesFingerprint,
		&evidence.Report.MountPlanFingerprint, &evidence.Report.NetworkPlanFingerprint,
		&evidence.Report.SecretPlanFingerprint, &evidence.Report.ContainerConfigFingerprint,
		&evidence.Report.ResourcePlanFingerprint, &evidence.Report.TerminationPlanFingerprint,
		&evidence.Report.OrphanPlanFingerprint, &evidence.Report.OutputPlanFingerprint,
		&evidence.Report.EvidenceFingerprint, &evidenceCount, &satisfiedCount, &verifiedCount,
		&productionVerified, &backendAvailable, &backendEnabled, &executionAuthorized,
		&commitAuthorized, &evidence.RequestedBy, &createdAt)
	if err != nil {
		return sandbox.BackendEvidence{}, err
	}
	if evidenceCount != sandbox.MaxBackendChecks || satisfiedCount != sandbox.MaxBackendChecks ||
		verifiedCount != 0 {
		return sandbox.BackendEvidence{}, errors.New("stored sandbox backend evidence counts are invalid")
	}
	evidence.Report.ProductionVerified = productionVerified == 1
	evidence.Report.BackendAvailable = backendAvailable == 1
	evidence.Report.BackendEnabled = backendEnabled == 1
	evidence.Report.ExecutionAuthorized = executionAuthorized == 1
	evidence.Report.ArtifactCommitAuthorized = commitAuthorized == 1
	evidence.CreatedAt = parseTS(createdAt)
	return evidence, nil
}

func listSandboxBackendEvidenceItems(ctx context.Context, queryer sandboxLifecycleQueryer,
	evidenceID string,
) ([]sandbox.BackendEvidenceItem, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, name, evidence_state,
		evidence_digest, satisfied, verified FROM sandbox_backend_evidence_items
		WHERE evidence_id = ? ORDER BY ordinal`, evidenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]sandbox.BackendEvidenceItem, 0, sandbox.MaxBackendChecks)
	for rows.Next() {
		var item sandbox.BackendEvidenceItem
		var satisfied, verified int
		if err := rows.Scan(&item.Ordinal, &item.Name, &item.EvidenceState,
			&item.EvidenceDigest, &satisfied, &verified); err != nil {
			return nil, err
		}
		item.Satisfied, item.Verified = satisfied == 1, verified == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func getSandboxOutputSimulation(ctx context.Context, queryer sandboxLifecycleQueryer,
	id string,
) (sandbox.OutputSimulation, error) {
	simulation, err := scanSandboxOutputSimulation(queryer.QueryRowContext(ctx,
		sandboxOutputSimulationSelect+` WHERE id = ?`, id))
	if err != nil {
		return sandbox.OutputSimulation{}, err
	}
	descriptors, err := listSandboxOutputSimulationItems(ctx, queryer, id)
	if err != nil {
		return sandbox.OutputSimulation{}, err
	}
	simulation.Descriptors = descriptors
	if err := simulation.Validate(); err != nil {
		return sandbox.OutputSimulation{}, apperror.Wrap(apperror.CodeInternal,
			"stored sandbox output simulation is invalid", err)
	}
	return simulation, nil
}

func scanSandboxOutputSimulation(row scanner) (sandbox.OutputSimulation, error) {
	var simulation sandbox.OutputSimulation
	var allOrNothing, simulationOnly, commitAuthorized, backendEnabled, executionAuthorized int
	var createdAt string
	if err := row.Scan(&simulation.ID, &simulation.EvidenceID, &simulation.PreflightID,
		&simulation.ExecutionID, &simulation.RunID, &simulation.MissionID,
		&simulation.WorkspaceID, &simulation.ProtocolVersion, &simulation.Status,
		&simulation.OutputPlanFingerprint, &simulation.FixtureDigest,
		&simulation.TransactionDigest, &simulation.ExpectedSlotCount,
		&simulation.StagedOutputCount, &simulation.StagedOutputBytes,
		&simulation.FakeArtifactCount, &simulation.ProductionArtifactCount,
		&allOrNothing, &simulationOnly, &commitAuthorized, &backendEnabled,
		&executionAuthorized, &simulation.RequestedBy, &createdAt); err != nil {
		return sandbox.OutputSimulation{}, err
	}
	simulation.AllOrNothing = allOrNothing == 1
	simulation.SimulationOnly = simulationOnly == 1
	simulation.ArtifactCommitAuthorized = commitAuthorized == 1
	simulation.BackendEnabled = backendEnabled == 1
	simulation.ExecutionAuthorized = executionAuthorized == 1
	simulation.CreatedAt = parseTS(createdAt)
	return simulation, nil
}

func listSandboxOutputSimulationItems(ctx context.Context, queryer sandboxLifecycleQueryer,
	simulationID string,
) ([]sandbox.StagedOutputDescriptor, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT ordinal, kind, locator_fingerprint,
		mime, sha256, size_bytes, redacted, fake_artifact_fingerprint
		FROM sandbox_output_simulation_items WHERE simulation_id = ? ORDER BY ordinal`, simulationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	descriptors := make([]sandbox.StagedOutputDescriptor, 0, sandbox.MaxOutputPaths+2)
	for rows.Next() {
		var descriptor sandbox.StagedOutputDescriptor
		var redacted int
		if err := rows.Scan(&descriptor.Ordinal, &descriptor.Kind,
			&descriptor.LocatorFingerprint, &descriptor.MIME, &descriptor.SHA256,
			&descriptor.SizeBytes, &redacted, &descriptor.FakeArtifactFingerprint); err != nil {
			return nil, err
		}
		descriptor.Redacted = redacted == 1
		descriptors = append(descriptors, descriptor)
	}
	return descriptors, rows.Err()
}

func getSandboxBackendEvidenceOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.BackendEvidenceOperation, bool, error) {
	var operation sandbox.BackendEvidenceOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		evidence_id, preflight_id, run_id, requested_by, created_at
		FROM sandbox_backend_evidence_operations WHERE operation_key_digest = ?`, keyDigest).Scan(
		&operation.KeyDigest, &operation.RequestFingerprint, &operation.EvidenceID,
		&operation.PreflightID, &operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.BackendEvidenceOperation{}, false, nil
	}
	if err != nil {
		return sandbox.BackendEvidenceOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.Validate(); err != nil {
		return sandbox.BackendEvidenceOperation{}, false, apperror.Wrap(apperror.CodeInternal,
			"stored sandbox backend evidence operation is invalid", err)
	}
	return operation, true, nil
}

func getSandboxOutputSimulationOperation(ctx context.Context, queryer sandboxLifecycleQueryer,
	keyDigest string,
) (sandbox.OutputSimulationOperation, bool, error) {
	var operation sandbox.OutputSimulationOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		simulation_id, evidence_id, run_id, requested_by, created_at
		FROM sandbox_output_simulation_operations WHERE operation_key_digest = ?`, keyDigest).Scan(
		&operation.KeyDigest, &operation.RequestFingerprint, &operation.SimulationID,
		&operation.EvidenceID, &operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.OutputSimulationOperation{}, false, nil
	}
	if err != nil {
		return sandbox.OutputSimulationOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.Validate(); err != nil {
		return sandbox.OutputSimulationOperation{}, false, apperror.Wrap(apperror.CodeInternal,
			"stored sandbox output simulation operation is invalid", err)
	}
	return operation, true, nil
}

func replaySandboxBackendEvidence(ctx context.Context, queryer sandboxLifecycleQueryer,
	existing, requested sandbox.BackendEvidenceOperation,
) (sandbox.BackendEvidence, bool, error) {
	if existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.PreflightID != requested.PreflightID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return sandbox.BackendEvidence{}, false, apperror.New(apperror.CodeConflict,
			"sandbox backend evidence operation key was reused for different intent")
	}
	value, err := getSandboxBackendEvidence(ctx, queryer, existing.EvidenceID)
	if err != nil {
		return sandbox.BackendEvidence{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func replaySandboxOutputSimulation(ctx context.Context, queryer sandboxLifecycleQueryer,
	existing, requested sandbox.OutputSimulationOperation,
) (sandbox.OutputSimulation, bool, error) {
	if existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.EvidenceID != requested.EvidenceID || existing.RunID != requested.RunID ||
		existing.RequestedBy != requested.RequestedBy {
		return sandbox.OutputSimulation{}, false, apperror.New(apperror.CodeConflict,
			"sandbox output simulation operation key was reused for different intent")
	}
	value, err := getSandboxOutputSimulation(ctx, queryer, existing.SimulationID)
	if err != nil {
		return sandbox.OutputSimulation{}, false, err
	}
	value.Replayed = true
	return value, true, nil
}

func (s *SQLiteStore) recoverSandboxBackendEvidenceCreate(ctx context.Context,
	operation sandbox.BackendEvidenceOperation, cause error,
) (sandbox.BackendEvidence, bool, error) {
	existing, found, err := getSandboxBackendEvidenceOperation(ctx, s.db, operation.KeyDigest)
	if err == nil && found {
		return replaySandboxBackendEvidence(ctx, s.db, existing, operation)
	}
	if err != nil {
		return sandbox.BackendEvidence{}, false, errors.Join(cause, err)
	}
	if _, lookupErr := getSandboxBackendEvidenceByPreflight(ctx, s.db,
		operation.PreflightID); lookupErr == nil {
		return sandbox.BackendEvidence{}, false, apperror.New(apperror.CodeConflict,
			"sandbox preflight already has backend evidence")
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return sandbox.BackendEvidence{}, false, errors.Join(cause, lookupErr)
	}
	return sandbox.BackendEvidence{}, false, cause
}

func (s *SQLiteStore) recoverSandboxOutputSimulationCreate(ctx context.Context,
	operation sandbox.OutputSimulationOperation, cause error,
) (sandbox.OutputSimulation, bool, error) {
	existing, found, err := getSandboxOutputSimulationOperation(ctx, s.db, operation.KeyDigest)
	if err == nil && found {
		return replaySandboxOutputSimulation(ctx, s.db, existing, operation)
	}
	if err != nil {
		return sandbox.OutputSimulation{}, false, errors.Join(cause, err)
	}
	return sandbox.OutputSimulation{}, false, cause
}

func appendSandboxBackendEvidenceEvent(ctx context.Context, tx *sql.Tx,
	evidence sandbox.BackendEvidence,
) error {
	event, err := events.New(evidence.RunID, evidence.MissionID,
		events.SandboxBackendEvidenceRecordedEvent, "sandbox_backend_evidence", evidence.ID,
		map[string]any{
			"protocol": evidence.Report.ProtocolVersion, "source": evidence.Report.Source,
			"trust_class": evidence.Report.TrustClass, "status": evidence.Report.Status,
			"backend": evidence.Report.Backend, "evidence_items": len(evidence.Report.Items),
			"simulated_satisfied": len(evidence.Report.Items), "production_verified": 0,
			"backend_available": false, "backend_enabled": false,
			"execution_authorized": false, "artifact_commit_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = evidence.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}

func appendSandboxOutputSimulationEvent(ctx context.Context, tx *sql.Tx,
	simulation sandbox.OutputSimulation,
) error {
	event, err := events.New(simulation.RunID, simulation.MissionID,
		events.SandboxOutputSimulationRecordedEvent, "sandbox_output_simulation", simulation.ID,
		map[string]any{
			"protocol": simulation.ProtocolVersion, "status": simulation.Status,
			"staged_outputs": simulation.StagedOutputCount,
			"staged_bytes":   simulation.StagedOutputBytes,
			"fake_artifacts": simulation.FakeArtifactCount, "production_artifacts": 0,
			"all_or_nothing": true, "simulation_only": true,
			"artifact_commit_authorized": false, "backend_enabled": false,
			"execution_authorized": false,
		})
	if err != nil {
		return err
	}
	event.CreatedAt = simulation.CreatedAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
