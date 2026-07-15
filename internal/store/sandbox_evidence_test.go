package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

func TestSandboxBackendEvidenceAndOutputSimulationLedgersAreImmutableAndPrivate(t *testing.T) {
	ctx := context.Background()
	st, run, root := openSandboxManifestStore(t, ctx)
	service, manifest, preflight := createDockerPreflightStoreFixture(t, ctx, st, run.ID,
		"store-evidence")
	evidence, err := service.RecordSimulatedBackendEvidence(ctx,
		application.RecordSandboxBackendEvidenceRequest{
			PreflightID: preflight.ID, Manifest: manifest,
			ImageDigest:  "sha256:" + strings.Repeat("c", 64),
			OperationKey: "store-evidence-record", RequestedBy: "store_evidence_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	fixture := sandbox.OutputFixture{ProtocolVersion: sandbox.OutputFixtureProtocolVersion,
		Outputs: []sandbox.OutputFixtureItem{
			{Kind: sandbox.OutputKindStdout, FileType: sandbox.OutputFileTypeStream, Content: "ok"},
			{Kind: sandbox.OutputKindStderr, FileType: sandbox.OutputFileTypeStream,
				Content: "TOKEN=sk-123456789012345678901234567890"},
		}}
	simulation, err := service.SimulateOutputTransaction(ctx,
		application.SimulateSandboxOutputRequest{
			EvidenceID: evidence.ID, Manifest: manifest, Fixture: fixture,
			OperationKey: "store-output-simulation", RequestedBy: "store_evidence_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	var evidenceItems, outputItems, artifactCount int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_backend_evidence_items
		WHERE evidence_id = ?`, evidence.ID).Scan(&evidenceItems); err != nil || evidenceItems != 16 {
		t.Fatalf("evidence item count=%d err=%v", evidenceItems, err)
	}
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_output_simulation_items
		WHERE simulation_id = ?`, simulation.ID).Scan(&outputItems); err != nil || outputItems != 2 {
		t.Fatalf("output simulation item count=%d err=%v", outputItems, err)
	}
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_artifacts
		WHERE run_id = ?`, run.ID).Scan(&artifactCount); err != nil || artifactCount != 0 {
		t.Fatalf("simulation created production Artifacts: count=%d err=%v", artifactCount, err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE sandbox_backend_evidence
		SET production_verified = 1 WHERE id = ?`, evidence.ID); err == nil ||
		!strings.Contains(err.Error(), "cannot be updated") {
		t.Fatalf("backend evidence root was mutable: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM sandbox_backend_evidence_items
		WHERE evidence_id = ? AND ordinal = 1`, evidence.ID); err == nil ||
		!strings.Contains(err.Error(), "cannot be deleted") {
		t.Fatalf("backend evidence item was deletable: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE sandbox_output_simulations
		SET production_artifact_count = 1 WHERE id = ?`, simulation.ID); err == nil ||
		!strings.Contains(err.Error(), "cannot be updated") {
		t.Fatalf("output simulation root was mutable: %v", err)
	}

	for _, table := range []string{"sandbox_backend_evidence", "sandbox_backend_evidence_items",
		"sandbox_output_simulations", "sandbox_output_simulation_items"} {
		rows, err := st.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			switch name {
			case "content", "raw_path", "output_path", "workspace_root", "command",
				"arguments_json", "secret_value", "lease_id", "owner_id", "container_id":
				_ = rows.Close()
				t.Fatalf("schema v52 persists private data in %s.%s", table, name)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}

	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range timeline {
		if event.Type != events.SandboxBackendEvidenceRecordedEvent &&
			event.Type != events.SandboxOutputSimulationRecordedEvent {
			continue
		}
		if strings.Contains(event.PayloadJSON, root) || strings.Contains(event.PayloadJSON, "sk-123456") ||
			strings.Contains(event.PayloadJSON, evidence.Report.ImageDigest) ||
			strings.Contains(event.PayloadJSON, simulation.FixtureDigest) ||
			strings.Contains(event.PayloadJSON, preflight.OutputPlan.Slots[0].LocatorFingerprint) {
			t.Fatalf("schema v52 event leaked private data: %#v", event)
		}
	}
}

func TestSandboxBackendEvidenceConcurrentReplayConvergesAcrossStores(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st1, run, _ := openSandboxManifestStoreAt(t, ctx, path)
	t.Cleanup(func() { _ = st1.Close() })
	service1, manifest, preflight := createDockerPreflightStoreFixture(t, ctx, st1, run.ID,
		"concurrent-evidence")
	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	services := []*application.SandboxManifestService{
		service1, application.NewSandboxManifestService(st2, policy.NewDefaultChecker()),
	}
	request := application.RecordSandboxBackendEvidenceRequest{
		PreflightID: preflight.ID, Manifest: manifest,
		ImageDigest:  "sha256:" + strings.Repeat("d", 64),
		OperationKey: "concurrent-evidence-record", RequestedBy: "store_evidence_operator",
	}
	start := make(chan struct{})
	results := make([]sandbox.BackendEvidence, len(services))
	errorsFound := make([]error, len(services))
	var group sync.WaitGroup
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index], errorsFound[index] = services[index].RecordSimulatedBackendEvidence(ctx, request)
		}(index)
	}
	close(start)
	group.Wait()
	if errorsFound[0] != nil || errorsFound[1] != nil || results[0].ID == "" ||
		results[0].ID != results[1].ID || results[0].Replayed == results[1].Replayed {
		t.Fatalf("concurrent backend evidence replay diverged: results=%#v errors=%v",
			results, errorsFound)
	}
	fixture := sandbox.OutputFixture{ProtocolVersion: sandbox.OutputFixtureProtocolVersion,
		Outputs: []sandbox.OutputFixtureItem{
			{Kind: sandbox.OutputKindStdout, FileType: sandbox.OutputFileTypeStream, Content: "out"},
			{Kind: sandbox.OutputKindStderr, FileType: sandbox.OutputFileTypeStream, Content: "err"},
		}}
	outputRequest := application.SimulateSandboxOutputRequest{
		EvidenceID: results[0].ID, Manifest: manifest, Fixture: fixture,
		OperationKey: "concurrent-output-simulation", RequestedBy: "store_evidence_operator",
	}
	outputResults := make([]sandbox.OutputSimulation, len(services))
	outputErrors := make([]error, len(services))
	start = make(chan struct{})
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			outputResults[index], outputErrors[index] = services[index].SimulateOutputTransaction(ctx,
				outputRequest)
		}(index)
	}
	close(start)
	group.Wait()
	if outputErrors[0] != nil || outputErrors[1] != nil || outputResults[0].ID == "" ||
		outputResults[0].ID != outputResults[1].ID ||
		outputResults[0].Replayed == outputResults[1].Replayed {
		t.Fatalf("concurrent output simulation replay diverged: results=%#v errors=%v",
			outputResults, outputErrors)
	}
}

func TestSandboxOutputSimulationLimitFailsClosedInServiceAndSQL(t *testing.T) {
	ctx := context.Background()
	st, run, _ := openSandboxManifestStore(t, ctx)
	service, manifest, preflight := createDockerPreflightStoreFixture(t, ctx, st, run.ID,
		"output-limit")
	evidence, err := service.RecordSimulatedBackendEvidence(ctx,
		application.RecordSandboxBackendEvidenceRequest{
			PreflightID: preflight.ID, Manifest: manifest,
			ImageDigest:  "sha256:" + strings.Repeat("f", 64),
			OperationKey: "output-limit-evidence", RequestedBy: "store_evidence_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	fixture := sandbox.OutputFixture{ProtocolVersion: sandbox.OutputFixtureProtocolVersion,
		Outputs: []sandbox.OutputFixtureItem{
			{Kind: sandbox.OutputKindStdout, FileType: sandbox.OutputFileTypeStream, Content: "out"},
			{Kind: sandbox.OutputKindStderr, FileType: sandbox.OutputFileTypeStream, Content: "err"},
		}}
	var last sandbox.OutputSimulation
	for index := 0; index < sandbox.MaxOutputSimulationsPerEvidence; index++ {
		last, err = service.SimulateOutputTransaction(ctx,
			application.SimulateSandboxOutputRequest{
				EvidenceID: evidence.ID, Manifest: manifest, Fixture: fixture,
				OperationKey: "output-limit-simulation-" + string(rune('a'+index)),
				RequestedBy:  "store_evidence_operator",
			})
		if err != nil {
			t.Fatalf("simulation %d failed: %v", index+1, err)
		}
	}
	_, err = service.SimulateOutputTransaction(ctx, application.SimulateSandboxOutputRequest{
		EvidenceID: evidence.ID, Manifest: manifest, Fixture: fixture,
		OperationKey: "output-limit-simulation-overflow", RequestedBy: "store_evidence_operator",
	})
	if apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("ninth simulation code=%s err=%v", apperror.CodeOf(err), err)
	}

	_, err = st.db.ExecContext(ctx, `INSERT INTO sandbox_output_simulations
		SELECT ?, evidence_id, preflight_id, execution_id, run_id, mission_id, workspace_id,
			protocol_version, status, output_plan_fingerprint, fixture_digest, transaction_digest,
			expected_slot_count, staged_output_count, staged_output_bytes, fake_artifact_count,
			production_artifact_count, all_or_nothing, simulation_only,
			artifact_commit_authorized, backend_enabled, execution_authorized, requested_by, created_at
		FROM sandbox_output_simulations WHERE id = ?`, "sandbox-output-sim-overflow", last.ID)
	if err == nil || !strings.Contains(err.Error(), "sandbox output simulation binding is invalid") {
		t.Fatalf("SQL bypassed the per-evidence simulation limit: %v", err)
	}
}

func TestSchemaV51UpgradeAddsSandboxEvidenceWithoutLosingPreflight(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v51.db")
	st, run, _ := openSandboxManifestStoreAt(t, ctx, path)
	_, _, preflight := createDockerPreflightStoreFixture(t, ctx, st, run.ID, "upgrade-evidence")
	for _, statement := range removeSchemaV52ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("simulate schema v51 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if version, err := st.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v51 did not upgrade to v52: version=%d err=%v", version, err)
	}
	loaded, err := st.GetSandboxDisabledPreflight(ctx, preflight.ID)
	if err != nil || loaded.ID != preflight.ID {
		t.Fatalf("schema v51 preflight was not preserved: %#v err=%v", loaded, err)
	}
	var table string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'sandbox_backend_evidence'`).Scan(&table); err != nil ||
		table != "sandbox_backend_evidence" {
		t.Fatalf("schema v52 evidence ledger is missing: %q err=%v", table, err)
	}
}

func createDockerPreflightStoreFixture(t *testing.T, ctx context.Context, st *SQLiteStore,
	runID, prefix string,
) (*application.SandboxManifestService, sandbox.Manifest, sandbox.DisabledPreflight) {
	t.Helper()
	service := application.NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest := sandboxStoreTestManifest()
	manifest.Backend = sandbox.BackendDocker
	prepared, err := service.Prepare(ctx, application.PrepareSandboxManifestRequest{
		RunID: runID, Manifest: manifest, OperationKey: prefix + "-prepare-operation",
		RequestedBy: "store_evidence_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.RequestApproval(ctx, prepared.Preparation.ID, "store_evidence_operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewApproval(ctx, prepared.Preparation.ID, approval.ActionApprove,
		prefix+"-review-operation", "store_evidence_operator", ""); err != nil {
		t.Fatal(err)
	}
	validated, err := service.ValidateExecutionCandidate(ctx,
		application.ValidateSandboxExecutionCandidateRequest{
			PreparationID: prepared.Preparation.ID, Manifest: manifest, ApprovalID: record.ID,
			OperationKey: prefix + "-candidate-operation", RequestedBy: "store_evidence_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := service.BeginDisabledExecution(ctx, application.BeginSandboxExecutionRequest{
		CandidateID: validated.Candidate.ID, Manifest: manifest,
		OperationKey: prefix + "-begin-operation", RequestedBy: "store_evidence_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := service.PrepareDisabledPreflight(ctx,
		application.PrepareSandboxPreflightRequest{
			ExecutionID: lifecycle.Execution.ID, Manifest: manifest,
			OperationKey: prefix + "-preflight-operation", RequestedBy: "store_evidence_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	return service, manifest, preflight
}

func removeSchemaV52ForTestStatements() []string {
	return append(removeSchemaV53ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_output_simulation_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_output_simulation_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_output_simulation_item_delete_immutable`,
		`DROP TRIGGER trg_sandbox_output_simulation_item_update_immutable`,
		`DROP TRIGGER trg_sandbox_output_simulation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_output_simulation_update_immutable`,
		`DROP TRIGGER trg_sandbox_backend_evidence_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_backend_evidence_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_backend_evidence_item_delete_immutable`,
		`DROP TRIGGER trg_sandbox_backend_evidence_item_update_immutable`,
		`DROP TRIGGER trg_sandbox_backend_evidence_delete_immutable`,
		`DROP TRIGGER trg_sandbox_backend_evidence_update_immutable`,
		`DROP TRIGGER trg_sandbox_output_simulation_operation_insert`,
		`DROP TRIGGER trg_sandbox_output_simulation_item_insert`,
		`DROP TRIGGER trg_sandbox_output_simulation_insert`,
		`DROP TRIGGER trg_sandbox_backend_evidence_operation_insert`,
		`DROP TRIGGER trg_sandbox_backend_evidence_item_insert`,
		`DROP TRIGGER trg_sandbox_backend_evidence_insert`,
		`DROP TABLE sandbox_output_simulation_operations`,
		`DROP TABLE sandbox_output_simulation_items`,
		`DROP INDEX idx_sandbox_output_simulations_run_created`,
		`DROP TABLE sandbox_output_simulations`,
		`DROP TABLE sandbox_backend_evidence_operations`,
		`DROP TABLE sandbox_backend_evidence_items`,
		`DROP INDEX idx_sandbox_backend_evidence_run_created`,
		`DROP TABLE sandbox_backend_evidence`,
		`DELETE FROM schema_migrations WHERE version = 52`,
	}...)
}

func removeSchemaV53ForTestStatements() []string {
	return append(removeSchemaV54ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_observation_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_observation_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_observation_item_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_observation_item_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_observation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_observation_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_observation_operation_insert`,
		`DROP TRIGGER trg_sandbox_docker_observation_item_insert`,
		`DROP TRIGGER trg_sandbox_docker_observation_insert`,
		`DROP TABLE sandbox_docker_observation_operations`,
		`DROP TABLE sandbox_docker_observation_items`,
		`DROP INDEX idx_sandbox_docker_observations_run_created`,
		`DROP TABLE sandbox_docker_observations`,
		`DELETE FROM schema_migrations WHERE version = 53`,
	}...)
}

func removeSchemaV54ForTestStatements() []string {
	return append(removeSchemaV55ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_container_plan_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_step_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_step_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_control_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_control_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_operation_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_step_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_control_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_plan_insert`,
		`DROP TABLE sandbox_docker_container_plan_operations`,
		`DROP TABLE sandbox_docker_container_plan_steps`,
		`DROP TABLE sandbox_docker_container_plan_controls`,
		`DROP INDEX idx_sandbox_docker_container_plans_run_created`,
		`DROP TABLE sandbox_docker_container_plans`,
		`DELETE FROM schema_migrations WHERE version = 54`,
	}...)
}

func removeSchemaV55ForTestStatements() []string {
	return append(removeSchemaV56ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_step_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_step_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_operation_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_step_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_rehearsal_insert`,
		`DROP TABLE sandbox_docker_container_rehearsal_operations`,
		`DROP TABLE sandbox_docker_container_rehearsal_steps`,
		`DROP INDEX idx_sandbox_docker_container_rehearsals_run_created`,
		`DROP TABLE sandbox_docker_container_rehearsals`,
		`DELETE FROM schema_migrations WHERE version = 55`,
	}...)
}

func removeSchemaV56ForTestStatements() []string {
	return append(removeSchemaV57ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_container_attempt_completion_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_completion_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_failure_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_failure_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_cleanup_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_cleanup_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_control_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_control_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_stage_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_stage_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_lease_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_completion_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_failure_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_cleanup_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_control_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_stage_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_lease_update`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_lease_insert`,
		`DROP TRIGGER trg_sandbox_docker_container_attempt_insert`,
		`DROP TABLE sandbox_docker_container_attempt_completions`,
		`DROP TABLE sandbox_docker_container_attempt_failures`,
		`DROP TABLE sandbox_docker_container_attempt_cleanups`,
		`DROP TABLE sandbox_docker_container_attempt_controls`,
		`DROP TABLE sandbox_docker_container_attempt_stages`,
		`DROP TABLE sandbox_docker_container_attempt_leases`,
		`DROP INDEX idx_sandbox_docker_container_attempts_run_created`,
		`DROP TABLE sandbox_docker_container_rehearsal_attempts`,
		`DELETE FROM schema_migrations WHERE version = 56`,
	}...)
}

func removeSchemaV57ForTestStatements() []string {
	return append(removeSchemaV58ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_host_input_staging_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_staging_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_staging_intent_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_staging_intent_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_attempt_completion_requires_host_input_staging`,
		`DROP TRIGGER trg_sandbox_docker_host_input_staging_insert`,
		`DROP TRIGGER trg_sandbox_docker_host_input_staging_intent_insert`,
		`DROP INDEX idx_sandbox_docker_host_input_stagings_run_created`,
		`DROP TABLE sandbox_docker_host_input_stagings`,
		`DROP INDEX idx_sandbox_docker_host_input_staging_intents_run_created`,
		`DROP TABLE sandbox_docker_host_input_staging_intents`,
		`DELETE FROM schema_migrations WHERE version = 57`,
	}...)
}

func removeSchemaV58ForTestStatements() []string {
	return append(removeSchemaV59ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_host_input_requirement_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_requirement_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_attempt_completion_requires_host_input_requirement`,
		`DROP TRIGGER trg_sandbox_docker_attempt_stage_requires_host_input_requirement`,
		`DROP TRIGGER trg_sandbox_docker_host_input_requirement_staging_compatibility`,
		`DROP TRIGGER trg_sandbox_docker_host_input_requirement_insert`,
		`DROP INDEX idx_sandbox_docker_host_input_requirements_run_created`,
		`DROP TABLE sandbox_docker_host_input_requirements`,
		`DROP TRIGGER trg_sandbox_docker_host_input_requirement_legacy_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_requirement_legacy_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_requirement_legacy_insert_immutable`,
		`DROP TABLE sandbox_docker_host_input_requirement_legacy_attempts`,
		`DELETE FROM schema_migrations WHERE version = 58`,
	}...)
}

func removeSchemaV59ForTestStatements() []string {
	return append(removeSchemaV60ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_intent_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_intent_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_requirement_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_requirement_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_attempt_completion_requires_host_input_handoff`,
		`DROP TRIGGER trg_sandbox_docker_attempt_cleanup_requires_host_input_handoff`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_insert`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_intent_insert`,
		`DROP TRIGGER trg_sandbox_docker_attempt_stage_requires_handoff_requirement`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_requirement_insert`,
		`DROP INDEX idx_sandbox_docker_host_input_handoffs_run_created`,
		`DROP TABLE sandbox_docker_host_input_handoffs`,
		`DROP INDEX idx_sandbox_docker_host_input_handoff_intents_run_created`,
		`DROP TABLE sandbox_docker_host_input_handoff_intents`,
		`DROP INDEX idx_sandbox_docker_host_input_handoff_requirements_run_created`,
		`DROP TABLE sandbox_docker_host_input_handoff_requirements`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_legacy_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_legacy_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_host_input_handoff_legacy_insert_immutable`,
		`DROP TABLE sandbox_docker_host_input_handoff_legacy_attempts`,
		`DELETE FROM schema_migrations WHERE version = 59`,
	}...)
}

func removeSchemaV60ForTestStatements() []string {
	return append(removeSchemaV61ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_completion_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_completion_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_item_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_item_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_plan_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_plan_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_operation_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_completion_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_item_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_projection_plan_insert`,
		`DROP TABLE sandbox_docker_runtime_input_projection_operations`,
		`DROP TABLE sandbox_docker_runtime_input_projection_completions`,
		`DROP TABLE sandbox_docker_runtime_input_projection_items`,
		`DROP INDEX idx_sandbox_docker_runtime_input_projection_plans_run_created`,
		`DROP TABLE sandbox_docker_runtime_input_projection_plans`,
		`DELETE FROM schema_migrations WHERE version = 60`,
	}...)
}

func removeSchemaV61ForTestStatements() []string {
	return append(removeSchemaV62ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_result_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_result_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_failure_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_failure_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_intent_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_intent_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_lease_update`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_result_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_failure_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_intent_insert`,
		`DROP TABLE sandbox_docker_runtime_input_application_results`,
		`DROP TABLE sandbox_docker_runtime_input_application_failures`,
		`DROP TABLE sandbox_docker_runtime_input_application_leases`,
		`DROP INDEX idx_sandbox_docker_runtime_input_application_intents_run_created`,
		`DROP TABLE sandbox_docker_runtime_input_application_intents`,
		`DELETE FROM schema_migrations WHERE version = 61`,
	}...)
}

func removeSchemaV62ForTestStatements() []string {
	return []string{
		`DROP TRIGGER trg_sandbox_docker_runtime_input_application_lease_delete_immutable_v62`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_lease_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_result_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_result_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_failure_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_failure_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_intent_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_intent_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_inspection_delete_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_inspection_update_immutable`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_lease_update`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_result_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_failure_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_intent_insert`,
		`DROP TRIGGER trg_sandbox_docker_runtime_input_resource_inspection_insert`,
		`DROP TABLE sandbox_docker_runtime_input_resource_cleanup_results`,
		`DROP TABLE sandbox_docker_runtime_input_resource_cleanup_failures`,
		`DROP TABLE sandbox_docker_runtime_input_resource_cleanup_leases`,
		`DROP INDEX idx_sandbox_docker_runtime_input_resource_cleanup_intents_run_created`,
		`DROP TABLE sandbox_docker_runtime_input_resource_cleanup_intents`,
		`DROP INDEX idx_sandbox_docker_runtime_input_resource_inspections_run_created`,
		`DROP TABLE sandbox_docker_runtime_input_resource_inspections`,
		`DELETE FROM schema_migrations WHERE version = 62`,
	}
}
