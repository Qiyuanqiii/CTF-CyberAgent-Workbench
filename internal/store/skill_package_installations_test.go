package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/toolgateway"
)

func TestSkillPackageInstallationLifecycleIsAppendOnlyAndPinnedRemovalFails(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "skill-packages.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	installation, operation, result := fixturePackageInstallation(t,
		"external-review", "1.0.0", "install-operation-key-0001", time.Now().UTC().Add(-time.Minute))
	prepared, pendingResult, replayed, err := st.PreparePackageInstallation(ctx,
		installation, operation)
	if err != nil || replayed || pendingResult != nil ||
		prepared.InstallationFingerprint != installation.InstallationFingerprint {
		t.Fatalf("prepare = %#v result=%#v replayed=%t err=%v",
			prepared, pendingResult, replayed, err)
	}
	prepared, pendingResult, replayed, err = st.PreparePackageInstallation(ctx,
		installation, operation)
	if err != nil || !replayed || pendingResult != nil || prepared.ID != installation.ID {
		t.Fatalf("pending replay = %#v result=%#v replayed=%t err=%v",
			prepared, pendingResult, replayed, err)
	}
	installed, completedReplay, err := st.CompletePackageInstallation(ctx, result)
	if err != nil || completedReplay || installed.Installation.ID != installation.ID {
		t.Fatalf("complete = %#v replayed=%t err=%v", installed, completedReplay, err)
	}
	secondCompletion := result
	secondCompletion.CompletedAt = result.CompletedAt.Add(time.Second)
	secondCompletion.ResultFingerprint = skills.PackageInstallResultFingerprint(secondCompletion)
	installed, completedReplay, err = st.CompletePackageInstallation(ctx, secondCompletion)
	if err != nil || !completedReplay ||
		!installed.Result.CompletedAt.Equal(result.CompletedAt) {
		t.Fatalf("completion convergence = %#v replayed=%t err=%v",
			installed, completedReplay, err)
	}
	loaded, found, err := st.GetInstalledPackageByRef(ctx, installation.Name,
		installation.Version)
	if err != nil || !found || loaded.Removal != nil {
		t.Fatalf("installed lookup = %#v found=%t err=%v", loaded, found, err)
	}
	listed, err := st.ListInstalledPackages(ctx, domain.ExecutionSurfaceCode,
		domain.ProfileReview, false)
	if err != nil || len(listed) != 1 || listed[0].Installation.ID != installation.ID {
		t.Fatalf("installed list = %#v err=%v", listed, err)
	}

	removal, removeOperation := fixturePackageRemoval(t, installation,
		"remove-operation-key-0001", result.CompletedAt.Add(time.Second))
	removed, removeReplay, err := st.CreatePackageRemoval(ctx, removal, removeOperation)
	if err != nil || removeReplay || removed.ID != removal.ID {
		t.Fatalf("remove = %#v replayed=%t err=%v", removed, removeReplay, err)
	}
	removed, removeReplay, err = st.CreatePackageRemoval(ctx, removal, removeOperation)
	if err != nil || !removeReplay || removed.ID != removal.ID {
		t.Fatalf("remove replay = %#v replayed=%t err=%v", removed, removeReplay, err)
	}
	listed, err = st.ListInstalledPackages(ctx, "", "", false)
	if err != nil || len(listed) != 0 {
		t.Fatalf("active list after removal = %#v err=%v", listed, err)
	}
	listed, err = st.ListInstalledPackages(ctx, "", "", true)
	if err != nil || len(listed) != 1 || listed[0].Removal == nil {
		t.Fatalf("historical list after removal = %#v err=%v", listed, err)
	}

	for _, mutation := range []struct {
		query string
		arg   string
	}{
		{`UPDATE skill_package_installations SET description = 'changed' WHERE id = ?`, installation.ID},
		{`DELETE FROM skill_package_installations WHERE id = ?`, installation.ID},
		{`UPDATE skill_package_install_operations SET installed_by = 'changed' WHERE installation_id = ?`, installation.ID},
		{`DELETE FROM skill_package_install_results WHERE installation_id = ?`, installation.ID},
		{`UPDATE skill_package_removals SET package_object_retained = 0 WHERE id = ?`, removal.ID},
		{`DELETE FROM skill_package_remove_operations WHERE removal_id = ?`, removal.ID},
	} {
		if _, err := st.db.ExecContext(ctx, mutation.query, mutation.arg); err == nil {
			t.Fatalf("immutable Skill package mutation succeeded: %s", mutation.query)
		}
	}
	assertPackageInstallOperationCannotCommitAlone(t, ctx, st)

	pinned, pinnedOperation, pinnedResult := fixturePackageInstallation(t,
		"pinned-review", "1.0.0", "install-operation-key-0002", time.Now().UTC().Add(-time.Minute))
	if _, _, _, err := st.PreparePackageInstallation(ctx, pinned, pinnedOperation); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompletePackageInstallation(ctx, pinnedResult); err != nil {
		t.Fatal(err)
	}
	createExternalSkillPin(t, ctx, st, pinned)
	pinnedRemoval, pinnedRemoveOperation := fixturePackageRemoval(t, pinned,
		"remove-operation-key-0002", pinnedResult.CompletedAt.Add(time.Second))
	if _, _, err := st.CreatePackageRemoval(ctx, pinnedRemoval,
		pinnedRemoveOperation); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("pinned removal error = %v", err)
	}
	assertPackageRemovalSQLPinGuard(t, ctx, st, pinnedRemoval, pinnedRemoveOperation)
}

func TestSkillPackageInstallationConvergesAcrossIndependentStores(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "skill-packages-concurrent.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	installation, operation, result := fixturePackageInstallation(t,
		"concurrent-review", "1.0.0", "concurrent-install-key", time.Now().UTC().Add(-time.Minute))
	stores := []*SQLiteStore{first, second}
	prepareReplay := make([]bool, len(stores))
	errorsByWorker := make([]error, len(stores))
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, _, prepareReplay[index], errorsByWorker[index] =
				stores[index].PreparePackageInstallation(ctx, installation, operation)
		}(index)
	}
	wait.Wait()
	if errorsByWorker[0] != nil || errorsByWorker[1] != nil ||
		prepareReplay[0] == prepareReplay[1] {
		t.Fatalf("prepare replay=%v errors=%v", prepareReplay, errorsByWorker)
	}
	completionReplay := make([]bool, len(stores))
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, completionReplay[index], errorsByWorker[index] =
				stores[index].CompletePackageInstallation(ctx, result)
		}(index)
	}
	wait.Wait()
	if errorsByWorker[0] != nil || errorsByWorker[1] != nil ||
		completionReplay[0] == completionReplay[1] {
		t.Fatalf("completion replay=%v errors=%v", completionReplay, errorsByWorker)
	}
	for table, want := range map[string]int{
		"skill_package_install_operations": 1,
		"skill_package_installations":      1,
		"skill_package_install_results":    1,
	} {
		var count int
		if err := first.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func TestSchemaV69UpgradeDoesNotFabricateSkillPackageInstallations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "skill-packages-v68.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV69ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			_ = st.Close()
			t.Fatalf("remove schema v69 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v69 upgrade version=%d err=%v", version, err)
	}
	values, err := upgraded.ListInstalledPackages(ctx, "", "", true)
	if err != nil || len(values) != 0 {
		t.Fatalf("schema v69 fabricated installations: %#v err=%v", values, err)
	}
}

func TestSchemaV70UpgradeDoesNotFabricateExternalSkillSelections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "external-skill-selections-v69.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV70ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			_ = st.Close()
			t.Fatalf("remove schema v70 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v70 upgrade version=%d err=%v", version, err)
	}
	var selectionCount, contextCount int
	if err := upgraded.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_external_skill_selections`).Scan(&selectionCount); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM root_external_skill_context_preparations`).Scan(&contextCount); err != nil {
		t.Fatal(err)
	}
	if selectionCount != 0 || contextCount != 0 {
		t.Fatalf("schema v70 fabricated state: selections=%d contexts=%d",
			selectionCount, contextCount)
	}
}

func fixturePackageInstallation(t *testing.T, name, version, operationKey string,
	createdAt time.Time,
) (skills.PackageInstallation, skills.PackageInstallOperation, skills.PackageInstallResult) {
	t.Helper()
	content := []byte("# " + name + "\n")
	contentDigest := sha256.Sum256(content)
	archiveDigest := sha256.Sum256([]byte("archive:" + name + "@" + version))
	operationDigest := runmutation.Fingerprint("skill_package_install_operation.v1", operationKey)
	installation := skills.PackageInstallation{
		ID: idgen.New("skill-install"), ProtocolVersion: skills.PackageInstallationProtocolVersion,
		Name: name, Version: version, Surface: domain.ExecutionSurfaceCode,
		Manifest: skills.Manifest{
			Protocol: skills.ProtocolVersion, Name: name, Version: version,
			Description: "External review workflow.",
			Profiles:    []domain.Profile{domain.ProfileReview},
			ToolDependencies: []toolgateway.ToolName{
				toolgateway.ListWorkspaceTool, toolgateway.ReadFileTool,
			},
			ContentPath:   skills.PackageContentPath,
			ContentSHA256: hex.EncodeToString(contentDigest[:]), ContentBytes: len(content),
			ContentTokenUpperBound: len(content),
		},
		ArchiveSHA256: hex.EncodeToString(archiveDigest[:]),
		PackageFingerprint: runmutation.Fingerprint("package", name, version,
			hex.EncodeToString(contentDigest[:])),
		ArchiveBytes: 512, UncompressedBytes: 256, EntryCount: skills.PackageEntryCount,
		TrustClass: skills.PackageTrustOperatorInstalledUntrusted,
		RiskCodes: []skills.PackageRiskCode{
			skills.PackageRiskUntrustedInstructions,
			skills.PackageRiskDeclaredToolsOnly,
		},
		OperatorConfirmed: true, OperationKeyDigest: operationDigest,
		InstalledBy: "operator", CreatedAt: createdAt.UTC(),
	}
	installation.RequestFingerprint = skills.PackageInstallationIntentFingerprint(installation)
	installation.InstallationFingerprint = skills.PackageInstallationFingerprint(installation)
	if err := installation.Validate(); err != nil {
		t.Fatal(err)
	}
	operation := skills.PackageInstallOperation{
		KeyDigest: operationDigest, RequestFingerprint: installation.RequestFingerprint,
		InstallationID: installation.ID, Name: name, Version: version,
		Surface: installation.Surface, InstalledBy: installation.InstalledBy,
		CreatedAt: installation.CreatedAt,
	}
	objectKey, err := skills.PackageObjectKey(installation.ArchiveSHA256)
	if err != nil {
		t.Fatal(err)
	}
	result, err := skills.NewPackageInstallResult(installation, skills.PackageObjectReceipt{
		Descriptor: skills.DescriptorForInstallation(installation), ObjectKey: objectKey,
	}, installation.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return installation, operation, result
}

func fixturePackageRemoval(t *testing.T, installation skills.PackageInstallation,
	operationKey string, createdAt time.Time,
) (skills.PackageRemoval, skills.PackageRemoveOperation) {
	t.Helper()
	operationDigest := runmutation.Fingerprint("skill_package_remove_operation.v1", operationKey)
	removal, err := skills.NewPackageRemoval(idgen.New("skill-remove"), installation,
		operationDigest, "operator", createdAt)
	if err != nil {
		t.Fatal(err)
	}
	operation := skills.PackageRemoveOperation{
		KeyDigest: operationDigest, RequestFingerprint: removal.RequestFingerprint,
		RemovalID: removal.ID, InstallationID: removal.InstallationID,
		Name: removal.Name, Version: removal.Version, Surface: removal.Surface,
		RemovedBy: removal.RemovedBy, CreatedAt: removal.CreatedAt,
	}
	return removal, operation
}

func createExternalSkillPin(t *testing.T, ctx context.Context, st *SQLiteStore,
	installation skills.PackageInstallation,
) {
	t.Helper()
	mission, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "external Skill pin fixture", Profile: "review",
		Budget: domain.Budget{MaxTurns: 4, MaxTokens: 4096},
	})
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().UTC()
	selection := skills.Selection{
		ID: idgen.New("skill-selection"), RunID: run.ID, MissionID: mission.ID,
		ProtocolVersion: skills.SelectionProtocolVersion, Profile: domain.ProfileReview,
		TokenBudget: 4096, TokenUpperBound: installation.Manifest.ContentBytes,
		ItemCount: 1, RequestedBy: "operator", CreatedAt: createdAt,
	}
	selection.Items = []skills.SelectionItem{{
		SelectionID: selection.ID, Ordinal: 1, Name: installation.Name,
		Version: installation.Version, ContentSHA256: installation.Manifest.ContentSHA256,
		ContentBytes:    installation.Manifest.ContentBytes,
		TokenUpperBound: installation.Manifest.ContentBytes,
	}}
	selection.Fingerprint = skills.SelectionFingerprint(selection)
	operation := skills.SelectionOperation{
		KeyDigest:          runmutation.Fingerprint("skill-selection-pin", run.ID),
		RequestFingerprint: skills.SelectionRequestFingerprint(selection),
		SelectionID:        selection.ID, RunID: run.ID, RequestedBy: selection.RequestedBy,
		CreatedAt: createdAt,
	}
	event, err := events.New(run.ID, mission.ID, events.SkillSelectionCreatedEvent,
		"skills", selection.ID, map[string]any{
			"protocol": selection.ProtocolVersion, "profile": selection.Profile,
			"item_count": selection.ItemCount, "token_budget": selection.TokenBudget,
			"token_upper_bound": selection.TokenUpperBound,
			"context_injection": false, "tool_capability_grant": false,
		})
	if err != nil {
		t.Fatal(err)
	}
	event.CreatedAt = createdAt
	if _, _, err := st.CreateSkillSelection(ctx, selection, operation, event); err != nil {
		t.Fatal(err)
	}
}

func assertPackageInstallOperationCannotCommitAlone(t *testing.T, ctx context.Context,
	st *SQLiteStore,
) {
	t.Helper()
	tx, err := st.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	digest := runmutation.Fingerprint("isolated-install-operation")
	_, err = tx.ExecContext(ctx, `INSERT INTO skill_package_install_operations
		(key_digest, request_fingerprint, installation_id, name, version, surface,
		installed_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, digest, digest,
		"missing-installation", "isolated-package", "1.0.0", "code", "operator",
		ts(time.Now().UTC()))
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("Skill package installation operation committed without its intent")
	}
}

func assertPackageRemovalSQLPinGuard(t *testing.T, ctx context.Context, st *SQLiteStore,
	removal skills.PackageRemoval, operation skills.PackageRemoveOperation,
) {
	t.Helper()
	tx, err := st.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO skill_package_remove_operations
		(key_digest, request_fingerprint, removal_id, installation_id, name, version,
		surface, removed_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.KeyDigest, operation.RequestFingerprint, operation.RemovalID,
		operation.InstallationID, operation.Name, operation.Version, operation.Surface,
		operation.RemovedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO skill_package_removals
		(id, protocol_version, installation_id, installation_fingerprint, name, version,
		surface, content_sha256, archive_sha256, package_fingerprint,
		operation_key_digest, request_fingerprint, package_object_retained,
		historical_recovery_preserved, future_selection_enabled,
		run_selection_authorized, context_injection_authorized, tool_capability_grant,
		removal_fingerprint, removed_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1, 0, 0, 0, 0, ?, ?, ?)`,
		removal.ID, removal.ProtocolVersion, removal.InstallationID,
		removal.InstallationFingerprint, removal.Name, removal.Version, removal.Surface,
		removal.ContentSHA256, removal.ArchiveSHA256, removal.PackageFingerprint,
		removal.OperationKeyDigest, removal.RequestFingerprint, removal.RemovalFingerprint,
		removal.RemovedBy, ts(removal.CreatedAt))
	_ = tx.Rollback()
	if err == nil {
		t.Fatal("direct SQL bypass removed a Run-pinned Skill package")
	}
}

func removeSchemaV69ForTestStatements() []string {
	return append(removeSchemaV70ForTestStatements(), []string{
		`DROP TRIGGER skill_package_removals_no_delete`,
		`DROP TRIGGER skill_package_removals_no_update`,
		`DROP TRIGGER skill_package_remove_operations_no_delete`,
		`DROP TRIGGER skill_package_remove_operations_no_update`,
		`DROP TRIGGER skill_package_install_results_no_delete`,
		`DROP TRIGGER skill_package_install_results_no_update`,
		`DROP TRIGGER skill_package_installations_no_delete`,
		`DROP TRIGGER skill_package_installations_no_update`,
		`DROP TRIGGER skill_package_install_operations_no_delete`,
		`DROP TRIGGER skill_package_install_operations_no_update`,
		`DROP TRIGGER skill_package_removal_insert_guard`,
		`DROP TRIGGER skill_package_install_result_insert_guard`,
		`DROP TRIGGER skill_package_installation_insert_guard`,
		`DROP TABLE skill_package_removals`,
		`DROP TABLE skill_package_remove_operations`,
		`DROP TABLE skill_package_install_results`,
		`DROP TABLE skill_package_installations`,
		`DROP TABLE skill_package_install_operations`,
		`DELETE FROM schema_migrations WHERE version = 69`,
	}...)
}

func removeSchemaV70ForTestStatements() []string {
	return append(removeSchemaV71ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_external_skill_context_commit_delete_immutable`,
		`DROP TRIGGER trg_specialist_external_skill_context_commit_update_immutable`,
		`DROP TRIGGER trg_specialist_external_skill_context_preparation_delete_immutable`,
		`DROP TRIGGER trg_specialist_external_skill_context_preparation_update_immutable`,
		`DROP TRIGGER trg_root_external_skill_context_commit_delete_immutable`,
		`DROP TRIGGER trg_root_external_skill_context_commit_update_immutable`,
		`DROP TRIGGER trg_root_external_skill_context_preparation_delete_immutable`,
		`DROP TRIGGER trg_root_external_skill_context_preparation_update_immutable`,
		`DROP TRIGGER trg_run_external_skill_selection_operation_delete_immutable`,
		`DROP TRIGGER trg_run_external_skill_selection_operation_update_immutable`,
		`DROP TRIGGER trg_run_external_skill_selection_item_delete_immutable`,
		`DROP TRIGGER trg_run_external_skill_selection_item_update_immutable`,
		`DROP TRIGGER trg_run_external_skill_selection_delete_immutable`,
		`DROP TRIGGER trg_run_external_skill_selection_update_immutable`,
		`DROP TRIGGER trg_specialist_external_skill_context_commit_insert`,
		`DROP TRIGGER trg_specialist_external_skill_context_preparation_insert`,
		`DROP TRIGGER trg_root_external_skill_context_commit_insert`,
		`DROP TRIGGER trg_root_external_skill_context_preparation_insert`,
		`DROP TRIGGER trg_skill_package_removal_external_selection_guard`,
		`DROP TRIGGER trg_run_external_skill_selection_operation_insert`,
		`DROP TRIGGER trg_run_external_skill_selection_item_insert`,
		`DROP TRIGGER trg_run_external_skill_selection_insert`,
		`DROP TABLE specialist_external_skill_context_commits`,
		`DROP TABLE specialist_external_skill_context_preparations`,
		`DROP TABLE root_external_skill_context_commits`,
		`DROP TABLE root_external_skill_context_preparations`,
		`DROP TABLE run_external_skill_selection_operations`,
		`DROP TABLE run_external_skill_selection_items`,
		`DROP TABLE run_external_skill_selections`,
		`DELETE FROM schema_migrations WHERE version = 70`,
	}...)
}

func removeSchemaV71ForTestStatements() []string {
	return append(removeSchemaV72ForTestStatements(), []string{
		`DROP VIEW run_external_skill_projection_items`,
		`DROP VIEW run_external_skill_projections`,
		`DELETE FROM schema_migrations WHERE version = 71`,
	}...)

}

func removeSchemaV72ForTestStatements() []string {
	return append(removeSchemaV73ForTestStatements(), []string{
		`DROP TRIGGER trg_run_creation_operation_delete_immutable`,
		`DROP TRIGGER trg_run_creation_operation_update_immutable`,
		`DROP TRIGGER trg_run_creation_operation_insert`,
		`DROP TABLE run_creation_operations`,
		`DELETE FROM schema_migrations WHERE version = 72`,
	}...)
}

func removeSchemaV73ForTestStatements() []string {
	return append(removeSchemaV74ForTestStatements(), []string{
		`DROP TRIGGER trg_run_execution_handoff_result_delete_immutable`,
		`DROP TRIGGER trg_run_execution_handoff_result_update_immutable`,
		`DROP TRIGGER trg_run_execution_handoff_item_delete_immutable`,
		`DROP TRIGGER trg_run_execution_handoff_item_update_immutable`,
		`DROP TRIGGER trg_run_execution_handoff_operation_delete_immutable`,
		`DROP TRIGGER trg_run_execution_handoff_operation_update_immutable`,
		`DROP TRIGGER trg_run_execution_handoff_result_insert`,
		`DROP TRIGGER trg_run_execution_handoff_item_insert`,
		`DROP TRIGGER trg_run_execution_handoff_operation_insert`,
		`DROP TRIGGER trg_run_lifecycle_operation_delete_immutable`,
		`DROP TRIGGER trg_run_lifecycle_operation_update_immutable`,
		`DROP TRIGGER trg_run_lifecycle_operation_insert`,
		`DROP TABLE run_execution_handoff_results`,
		`DROP TABLE run_execution_handoff_items`,
		`DROP TABLE run_execution_handoff_operations`,
		`DROP TABLE run_lifecycle_operations`,
		`DELETE FROM schema_migrations WHERE version = 73`,
	}...)
}

func removeSchemaV74ForTestStatements() []string {
	return append(removeSchemaV75ForTestStatements(), []string{
		`DROP TRIGGER trg_run_wake_operation_delete_immutable`,
		`DROP TRIGGER trg_run_wake_operation_update_immutable`,
		`DROP TRIGGER trg_run_wake_operation_insert`,
		`DROP TRIGGER trg_run_wake_lease_delete_immutable`,
		`DROP TRIGGER trg_run_wake_lease_update`,
		`DROP TRIGGER trg_run_wake_lease_insert`,
		`DROP TRIGGER trg_run_wake_intent_delete_immutable`,
		`DROP TRIGGER trg_run_wake_intent_update`,
		`DROP TRIGGER trg_run_wake_intent_insert`,
		`DROP TABLE run_wake_operations`,
		`DROP TABLE run_wake_leases`,
		`DROP TABLE run_wake_intents`,
		`DELETE FROM schema_migrations WHERE version = 74`,
	}...)
}

func removeSchemaV75ForTestStatements() []string {
	return append(removeSchemaV76ForTestStatements(), []string{
		`DROP TRIGGER trg_run_wake_consumption_delete_immutable`,
		`DROP TRIGGER trg_run_wake_consumption_update`,
		`DROP TRIGGER trg_run_wake_consumption_insert`,
		`DROP TRIGGER trg_run_wake_intent_update`,
		`DROP TRIGGER trg_run_wake_lease_update`,
		`DROP INDEX idx_run_wake_consumptions_intent_created`,
		`DROP TABLE run_wake_consumptions`,
		runWakeOwnershipStatements[8],
		runWakeOwnershipStatements[11],
		`DELETE FROM schema_migrations WHERE version = 75`,
	}...)
}

func removeSchemaV76ForTestStatements() []string {
	return append(removeSchemaV77ForTestStatements(), []string{
		`DROP TRIGGER trg_file_edit_apply_result_delete_immutable`,
		`DROP TRIGGER trg_file_edit_apply_result_update_immutable`,
		`DROP TRIGGER trg_file_edit_apply_result_insert`,
		`DROP TRIGGER trg_file_edit_apply_operation_delete_immutable`,
		`DROP TRIGGER trg_file_edit_apply_operation_update_immutable`,
		`DROP TRIGGER trg_file_edit_apply_operation_insert`,
		`DROP TABLE file_edit_apply_results`,
		`DROP INDEX idx_file_edit_apply_operations_run_created`,
		`DROP TABLE file_edit_apply_operations`,
		`DELETE FROM schema_migrations WHERE version = 76`,
	}...)
}

func removeSchemaV77ForTestStatements() []string {
	return append(removeSchemaV78ForTestStatements(), []string{
		`DROP TRIGGER trg_session_evidence_attachment_delete_immutable`,
		`DROP TRIGGER trg_session_evidence_attachment_update_immutable`,
		`DROP TRIGGER trg_session_evidence_attachment_insert`,
		`DROP INDEX idx_session_evidence_attachments_run_created`,
		`DROP TABLE session_evidence_attachments`,
		`DELETE FROM schema_migrations WHERE version = 77`,
	}...)
}

func removeSchemaV78ForTestStatements() []string {
	return append(removeSchemaV79ForTestStatements(), []string{
		`DROP TRIGGER trg_operator_verification_evidence_delete_immutable`,
		`DROP TRIGGER trg_operator_verification_evidence_update_immutable`,
		`DROP TRIGGER trg_operator_verification_evidence_insert`,
		`DROP INDEX idx_operator_verification_evidence_run_created`,
		`DROP TABLE operator_verification_evidence`,
		`DELETE FROM schema_migrations WHERE version = 78`,
	}...)
}

func removeSchemaV79ForTestStatements() []string {
	return append(removeSchemaV80ForTestStatements(), []string{
		`DROP TRIGGER trg_run_progress_guard_update`,
		`DROP TRIGGER trg_run_progress_guard_insert`,
		`DROP INDEX idx_run_progress_guards_status_updated`,
		`DROP TABLE run_progress_guards`,
		`DELETE FROM schema_migrations WHERE version = 79`,
	}...)
}

func removeSchemaV80ForTestStatements() []string {
	return []string{
		`DROP TRIGGER trg_operator_verification_plan_item_delete_immutable`,
		`DROP TRIGGER trg_operator_verification_plan_item_update_immutable`,
		`DROP TRIGGER trg_operator_verification_plan_delete_immutable`,
		`DROP TRIGGER trg_operator_verification_plan_update_immutable`,
		`DROP TRIGGER trg_operator_verification_plan_item_insert`,
		`DROP TRIGGER trg_operator_verification_plan_insert`,
		`DROP INDEX idx_operator_verification_plans_run_created`,
		`DROP TABLE operator_verification_plan_items`,
		`DROP TABLE operator_verification_plans`,
		`DELETE FROM schema_migrations WHERE version = 80`,
	}
}
