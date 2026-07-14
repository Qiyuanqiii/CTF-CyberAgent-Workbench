package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/agent"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolrun"
)

func TestOpenCreatesPrivateSQLitePathOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions are controlled by ACLs")
	}
	directory := filepath.Join(t.TempDir(), "private", "runtime")
	path := filepath.Join(directory, "cyberagent.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	databaseInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 || databaseInfo.Mode().Perm() != 0o600 {
		t.Fatalf("runtime path permissions are not private: dir=%o db=%o",
			directoryInfo.Mode().Perm(), databaseInfo.Mode().Perm())
	}
}

func TestSQLiteStoreVersionedMigrationsAreIdempotent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	version, err := st.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != LatestSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", LatestSchemaVersion, version)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != LatestSchemaVersion {
		t.Fatalf("expected %d migration records, got %d", LatestSchemaVersion, count)
	}
	rows, err := st.db.QueryContext(ctx, `PRAGMA table_info(run_supervisor_checkpoints)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !columns["repair_phase"] || !columns["repair_reason"] {
		t.Fatalf("schema v8 protocol repair columns are missing: %#v", columns)
	}
	var workItemTable string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'work_items'`).Scan(&workItemTable); err != nil {
		t.Fatal(err)
	}
	if workItemTable != "work_items" {
		t.Fatalf("schema v9 work board table is missing: %q", workItemTable)
	}
	var notesTable string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'notes'`).Scan(&notesTable); err != nil {
		t.Fatal(err)
	}
	if notesTable != "notes" {
		t.Fatalf("schema v10 notes table is missing: %q", notesTable)
	}
	for _, name := range []string{"tool_approvals", "approval_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v11 approval table is missing: %q", table)
		}
	}
	for _, name := range []string{"approval_session_grants", "approval_grant_operations", "run_tool_usage", "run_tool_calls"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v12 table is missing: %q", table)
		}
	}
	var scriptProcessTable string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'script_process_proposals'`).Scan(&scriptProcessTable); err != nil {
		t.Fatal(err)
	}
	if scriptProcessTable != "script_process_proposals" {
		t.Fatalf("schema v13 typed script process table is missing: %q", scriptProcessTable)
	}
	var runArtifactTable string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'run_artifacts'`).Scan(&runArtifactTable); err != nil {
		t.Fatal(err)
	}
	if runArtifactTable != "run_artifacts" {
		t.Fatalf("schema v14 Run artifact table is missing: %q", runArtifactTable)
	}
	var structuredOperationTable string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'structured_tool_operations'`).Scan(&structuredOperationTable); err != nil {
		t.Fatal(err)
	}
	if structuredOperationTable != "structured_tool_operations" {
		t.Fatalf("schema v15 structured tool operation table is missing: %q", structuredOperationTable)
	}
	for _, name := range []string{"run_supervisor_tool_rounds", "run_supervisor_tool_calls"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v16 supervisor tool table is missing: %q", table)
		}
	}
	var executionLeaseTable string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'run_execution_leases'`).Scan(&executionLeaseTable); err != nil {
		t.Fatal(err)
	}
	if executionLeaseTable != "run_execution_leases" {
		t.Fatalf("schema v17 execution lease table is missing: %q", executionLeaseTable)
	}
	for _, name := range []string{"run_model_cancellations", "run_model_cancellation_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v18 model cancellation table is missing: %q", table)
		}
	}
	for _, name := range []string{"specialist_schedules", "specialist_schedule_agents",
		"specialist_model_cancellations", "specialist_model_cancellation_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v29 Specialist control table is missing: %q", table)
		}
	}
	for _, name := range []string{"specialist_delegation_proposals",
		"specialist_delegation_assignments", "specialist_delegation_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v30 Specialist delegation table is missing: %q", table)
		}
	}
	for _, name := range []string{"specialist_delegation_reviews",
		"specialist_delegation_review_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v31 Specialist delegation review table is missing: %q", table)
		}
	}
	for _, name := range []string{"specialist_delegation_applications",
		"specialist_delegation_application_assignments",
		"specialist_delegation_application_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v32 Specialist delegation application table is missing: %q", table)
		}
	}
	for _, name := range []string{"specialist_operator_schedule_requests",
		"specialist_operator_schedule_request_agents",
		"specialist_operator_schedule_operations",
		"specialist_operator_schedule_attempts"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v38 Specialist operator schedule table is missing: %q", table)
		}
	}
	for _, name := range []string{"readonly_fanout_plans", "readonly_fanout_files",
		"readonly_fanout_shards", "readonly_fanout_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v33 read-only fan-out table is missing: %q", table)
		}
	}
	for _, name := range []string{"readonly_fanout_executions",
		"readonly_fanout_execution_shards", "readonly_fanout_model_calls",
		"readonly_fanout_findings", "readonly_fanout_execution_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v34 read-only fan-out execution table is missing: %q", table)
		}
	}
	for _, name := range []string{"finding_reports", "findings", "finding_evidence"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v35 finding report table is missing: %q", table)
		}
	}
	for _, name := range []string{"finding_artifact_evidence",
		"finding_artifact_evidence_operations", "finding_validation_decisions",
		"finding_validation_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v36 finding validation table is missing: %q", table)
		}
	}
	for _, name := range []string{"trg_run_artifact_update_immutable",
		"trg_run_artifact_delete_immutable", "trg_finding_artifact_evidence_insert",
		"trg_finding_validation_insert"} {
		var trigger string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'trigger' AND name = ?`, name).Scan(&trigger); err != nil {
			t.Fatal(err)
		}
		if trigger != name {
			t.Fatalf("schema v36 validation trigger is missing: %q", trigger)
		}
	}
	for _, name := range []string{"finding_acceptance_decisions",
		"finding_acceptance_operations", "finding_remediation_evidence",
		"finding_remediation_evidence_operations", "finding_fix_decisions",
		"finding_fix_operations"} {
		var table string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != name {
			t.Fatalf("schema v37 finding remediation table is missing: %q", table)
		}
	}
	for _, name := range []string{"trg_finding_acceptance_insert",
		"trg_finding_remediation_evidence_insert", "trg_finding_fix_insert"} {
		var trigger string
		if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'trigger' AND name = ?`, name).Scan(&trigger); err != nil {
			t.Fatal(err)
		}
		if trigger != name {
			t.Fatalf("schema v37 finding remediation trigger is missing: %q", trigger)
		}
	}
	for _, column := range []string{"lease_id", "lease_generation"} {
		var count int
		if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('run_supervisor_checkpoints')
			WHERE name = ?`, column).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("schema v17 checkpoint column %s is missing", column)
		}
	}
	var grantColumn int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('tool_approvals') WHERE name = 'grant_id'`).Scan(&grantColumn); err != nil {
		t.Fatal(err)
	}
	if grantColumn != 1 {
		t.Fatal("schema v12 tool_approvals.grant_id column is missing")
	}
}

func TestSQLiteStoreUpgradesLegacyDatabaseWithoutLosingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		root_path TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, root_path, created_at) VALUES (?, ?, ?, ?)`,
		"ws-legacy", "legacy", `C:\legacy`, ts(created)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	rec, err := st.GetWorkspaceByName(ctx, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "ws-legacy" || rec.RootPath != `C:\legacy` {
		t.Fatalf("legacy data changed during migration: %#v", rec)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != LatestSchemaVersion {
		t.Fatalf("expected upgraded version %d, got %d", LatestSchemaVersion, version)
	}
}

func TestSQLiteStoreUpgradesSchemaV8ToLatestWithoutLosingRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v8.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "preserve this v8 run", Profile: "code", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	removeSchemaV12ForTest(t, st, ctx)
	for _, table := range []string{"approval_operations", "tool_approvals"} {
		if _, err := st.db.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 11`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"note_evidence", "note_sources", "note_tags", "notes"} {
		if _, err := st.db.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 10`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE work_item_dependencies`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE work_items`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 9`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loaded, err := st.GetRun(ctx, run.ID)
	if err != nil || loaded.ID != run.ID || loaded.MissionID != run.MissionID {
		t.Fatalf("v8 run was not preserved: %#v err=%v", loaded, err)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("v8 database did not upgrade to latest: version=%d err=%v", version, err)
	}
	item, err := application.NewWorkItemService(st).Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "new v9 item",
	})
	if err != nil || item.RunID != run.ID {
		t.Fatalf("upgraded work board is unusable: %#v err=%v", item, err)
	}
}

func TestSQLiteStoreUpgradesSchemaV9ToNotesWithoutLosingWorkItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v9.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mission, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "preserve v9 work board", Profile: "code", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	workItem, err := application.NewWorkItemService(st).Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "preserved work item",
	})
	if err != nil {
		t.Fatal(err)
	}
	removeSchemaV12ForTest(t, st, ctx)
	for _, table := range []string{"approval_operations", "tool_approvals"} {
		if _, err := st.db.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 11`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"note_evidence", "note_sources", "note_tags", "notes"} {
		if _, err := st.db.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 10`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loaded, err := st.GetWorkItem(ctx, workItem.ID)
	if err != nil || loaded.ID != workItem.ID || loaded.RunID != run.ID {
		t.Fatalf("v9 work item was not preserved: %#v err=%v", loaded, err)
	}
	note := newNoteTest(run.ID, "new v10 note", "migration is usable")
	if err := st.CreateNote(ctx, note, newNoteCreatedEvent(t, mission.ID, note)); err != nil {
		t.Fatalf("upgraded note store is unusable: %v", err)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("v9 database did not upgrade to v10: version=%d err=%v", version, err)
	}
}

func removeSchemaV12ForTest(t *testing.T, st *SQLiteStore, ctx context.Context) {
	t.Helper()
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range append(removeSchemaV22ForTestStatements(), []string{
		`DROP TABLE agent_admission_operations`,
		`DELETE FROM schema_migrations WHERE version = 21`,
		`DROP TABLE agent_message_operations`,
		`DELETE FROM schema_migrations WHERE version = 20`,
		`DROP TABLE agent_graph_snapshots`,
		`DROP TABLE agent_messages`,
		`DROP TABLE agent_nodes`,
		`DELETE FROM schema_migrations WHERE version = 19`,
		`DROP TABLE run_model_cancellation_operations`,
		`DROP TABLE run_model_cancellations`,
		`DELETE FROM schema_migrations WHERE version = 18`,
		`DROP TABLE run_execution_leases`,
		`ALTER TABLE run_supervisor_checkpoints DROP COLUMN lease_generation`,
		`ALTER TABLE run_supervisor_checkpoints DROP COLUMN lease_id`,
		`DELETE FROM schema_migrations WHERE version = 17`,
		`DROP TABLE run_supervisor_tool_calls`,
		`DROP TABLE run_supervisor_tool_rounds`,
		`DELETE FROM schema_migrations WHERE version = 16`,
		`DROP TABLE structured_tool_operations`,
		`DELETE FROM schema_migrations WHERE version = 15`,
		`DROP TABLE run_artifacts`,
		`DELETE FROM schema_migrations WHERE version = 14`,
		`DROP TABLE script_process_proposals`,
		`DELETE FROM schema_migrations WHERE version = 13`,
		`DROP INDEX idx_tool_approvals_grant_id`,
		`ALTER TABLE tool_approvals DROP COLUMN grant_id`,
		`DROP TABLE run_tool_calls`,
		`DROP TABLE run_tool_usage`,
		`DROP TABLE approval_grant_operations`,
		`DROP TABLE approval_session_grants`,
		`DELETE FROM schema_migrations WHERE version = 12`,
	}...) {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("remove schema v12 with %q: %v", statement, err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
}

func removeSchemaV16ForTest(t *testing.T, st *SQLiteStore, ctx context.Context) {
	t.Helper()
	for _, statement := range append(removeSchemaV22ForTestStatements(), []string{
		`DROP TABLE agent_admission_operations`,
		`DELETE FROM schema_migrations WHERE version = 21`,
		`DROP TABLE agent_message_operations`,
		`DELETE FROM schema_migrations WHERE version = 20`,
		`DROP TABLE agent_graph_snapshots`,
		`DROP TABLE agent_messages`,
		`DROP TABLE agent_nodes`,
		`DELETE FROM schema_migrations WHERE version = 19`,
		`DROP TABLE run_model_cancellation_operations`,
		`DROP TABLE run_model_cancellations`,
		`DELETE FROM schema_migrations WHERE version = 18`,
		`DROP TABLE run_execution_leases`,
		`ALTER TABLE run_supervisor_checkpoints DROP COLUMN lease_generation`,
		`ALTER TABLE run_supervisor_checkpoints DROP COLUMN lease_id`,
		`DELETE FROM schema_migrations WHERE version = 17`,
		`DROP TABLE run_supervisor_tool_calls`,
		`DROP TABLE run_supervisor_tool_rounds`,
		`DELETE FROM schema_migrations WHERE version = 16`,
	}...) {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("remove schema v16 with %q: %v", statement, err)
		}
	}
}

func removeSchemaV22ForTestStatements() []string {
	return append(removeSchemaV23ForTestStatements(), []string{
		`DROP TRIGGER trg_work_item_owner_agent_insert`,
		`DROP TRIGGER trg_work_item_owner_agent_update`,
		`DROP TRIGGER trg_note_owner_agent_insert`,
		`DROP TRIGGER trg_note_owner_agent_update`,
		`DROP INDEX idx_work_items_owner_agent`,
		`DROP INDEX idx_notes_owner_agent`,
		`ALTER TABLE work_items DROP COLUMN owner_agent_id`,
		`ALTER TABLE notes DROP COLUMN owner_agent_id`,
		`DELETE FROM schema_migrations WHERE version = 22`,
	}...)
}

func removeSchemaV23ForTestStatements() []string {
	return append(removeSchemaV24ForTestStatements(), []string{
		`DROP TRIGGER trg_agent_completion_running_child`,
		`DROP TRIGGER trg_agent_completion_message_matches`,
		`DROP TRIGGER trg_agent_completed_requires_report`,
		`DROP TRIGGER trg_agent_completion_immutable`,
		`DROP TABLE agent_completion_operations`,
		`DROP TABLE agent_completion_reports`,
		`DELETE FROM schema_migrations WHERE version = 23`,
	}...)
}

func removeSchemaV24ForTestStatements() []string {
	return append(removeSchemaV25ForTestStatements(), []string{
		`DROP TRIGGER trg_agent_attempt_running_child`,
		`DROP TRIGGER trg_specialist_running_requires_attempt`,
		`DROP TRIGGER trg_agent_attempt_terminal_child`,
		`DROP TRIGGER trg_specialist_nonrunning_requires_terminal_attempt`,
		`DROP TRIGGER trg_agent_attempt_identity_immutable`,
		`DROP TRIGGER trg_agent_attempt_terminal_immutable`,
		`DROP TRIGGER trg_agent_attempt_usage_immutable`,
		`DROP TRIGGER trg_agent_attempt_usage_requires_lease`,
		`DROP TRIGGER trg_agent_attempt_notification_matches`,
		`DROP TRIGGER trg_completion_requires_agent_attempt`,
		`DROP TABLE agent_attempt_mutations`,
		`DROP TABLE agent_attempts`,
		`DELETE FROM schema_migrations WHERE version = 24`,
	}...)
}

func removeSchemaV25ForTestStatements() []string {
	return append(removeSchemaV26ForTestStatements(), []string{
		`DROP TRIGGER trg_root_inbox_delivery_insert`,
		`DROP TRIGGER trg_root_inbox_delivery_commit`,
		`DROP TRIGGER trg_root_inbox_delivery_active_supersede`,
		`DROP TRIGGER trg_root_inbox_delivery_identity_immutable`,
		`DROP TRIGGER trg_root_inbox_delivery_terminal_immutable`,
		`DROP TRIGGER trg_root_inbox_delivery_prepared_delete`,
		`DROP TRIGGER trg_agent_message_prepared_delivery`,
		`DROP TABLE root_inbox_deliveries`,
		`DELETE FROM schema_migrations WHERE version = 25`,
	}...)
}

func removeSchemaV26ForTestStatements() []string {
	return append(removeSchemaV27ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_model_call_sequence`,
		`DROP TRIGGER trg_specialist_model_call_insert`,
		`DROP TRIGGER trg_specialist_model_call_terminal_requires_lease`,
		`DROP TRIGGER trg_specialist_model_call_identity_immutable`,
		`DROP TRIGGER trg_specialist_model_call_terminal_immutable`,
		`DROP TABLE specialist_model_calls`,
		`DELETE FROM schema_migrations WHERE version = 26`,
	}...)
}

func removeSchemaV27ForTestStatements() []string {
	return append(removeSchemaV28ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_context_delivery_insert`,
		`DROP TRIGGER trg_specialist_context_delivery_commit`,
		`DROP TRIGGER trg_specialist_context_delivery_active_supersede`,
		`DROP TRIGGER trg_specialist_context_delivery_identity_immutable`,
		`DROP TRIGGER trg_specialist_context_delivery_terminal_immutable`,
		`DROP TRIGGER trg_specialist_context_delivery_prepared_delete`,
		`DROP TRIGGER trg_agent_message_prepared_specialist_delivery`,
		`DROP TABLE specialist_context_deliveries`,
		`DELETE FROM schema_migrations WHERE version = 27`,
	}...)
}

func removeSchemaV28ForTestStatements() []string {
	statements := append(removeSchemaV29ForTestStatements(), []string{
		`DROP TRIGGER trg_agent_attempt_repair_resolution`,
		`DROP TRIGGER trg_agent_attempt_usage_monotonic`,
		`DROP TRIGGER trg_agent_attempt_usage_requires_lease`,
		`DROP TRIGGER trg_specialist_repair_insert`,
		`DROP TRIGGER trg_specialist_repair_resolve`,
		`DROP TRIGGER trg_specialist_repair_identity_immutable`,
		`DROP TRIGGER trg_specialist_repair_terminal_immutable`,
		`DROP TRIGGER trg_specialist_model_call_sequence`,
		`DROP TRIGGER trg_specialist_model_call_phase_sequence`,
		`DROP TRIGGER trg_specialist_model_call_insert`,
		`DROP TRIGGER trg_specialist_model_call_terminal_requires_lease`,
		`DROP TRIGGER trg_specialist_model_call_identity_immutable`,
		`DROP TRIGGER trg_specialist_model_call_terminal_immutable`,
		`DROP INDEX idx_specialist_model_one_started`,
		`DROP INDEX idx_specialist_model_agent_started`,
		`DROP INDEX idx_specialist_repair_run_status`,
		`DROP TABLE specialist_protocol_repairs`,
		`ALTER TABLE specialist_model_calls RENAME TO specialist_model_calls_v28`,
		specialistModelCallStatements[0],
		`INSERT INTO specialist_model_calls
			(agent_attempt_id, run_id, agent_id, model_attempt_number, transport_attempt,
			max_attempts, provider, model, input_fingerprint, action_fingerprint,
			status, outcome, error_text, retry_after_millis, retry_planned, elapsed_millis,
			stream_events, stream_bytes, input_tokens, output_tokens, total_tokens,
			usage_recorded, action_kind, report_outcome, policy_allowed, policy_needs_approval,
			policy_risk, policy_reason, user_message_id, assistant_message_id, started_at, finished_at)
		SELECT agent_attempt_id, run_id, agent_id, model_attempt_number, transport_attempt,
			max_attempts, provider, model, input_fingerprint, action_fingerprint,
			status, outcome, error_text, retry_after_millis, retry_planned, elapsed_millis,
			stream_events, stream_bytes, input_tokens, output_tokens, total_tokens,
			usage_recorded, action_kind, report_outcome, policy_allowed, policy_needs_approval,
			policy_risk, policy_reason, user_message_id, assistant_message_id, started_at, finished_at
		FROM specialist_model_calls_v28 WHERE protocol_repair = 0`,
		`DROP TABLE specialist_model_calls_v28`,
	}...)
	statements = append(statements, specialistModelCallStatements[1:]...)
	statements = append(statements,
		`CREATE TRIGGER trg_agent_attempt_usage_immutable
			BEFORE UPDATE OF input_tokens, output_tokens, total_tokens, execution_millis,
				usage_recorded_at ON agent_attempts
			WHEN OLD.usage_recorded_at IS NOT NULL
			BEGIN
				SELECT RAISE(ABORT, 'Agent attempt usage is immutable');
			END`,
		`CREATE TRIGGER trg_agent_attempt_usage_requires_lease
			BEFORE UPDATE OF input_tokens, output_tokens, total_tokens, execution_millis,
				usage_recorded_at ON agent_attempts
			WHEN OLD.usage_recorded_at IS NULL AND NEW.usage_recorded_at IS NOT NULL
				AND NOT EXISTS (
					SELECT 1 FROM run_execution_leases lease
					WHERE lease.run_id = OLD.run_id AND lease.lease_id = OLD.lease_id
						AND lease.generation = OLD.lease_generation AND lease.status = 'active'
						AND julianday(lease.expires_at) > julianday('now')
				)
			BEGIN
				SELECT RAISE(ABORT, 'Agent attempt usage requires its active Run lease');
			END`,
		`DELETE FROM schema_migrations WHERE version = 28`,
	)
	return statements
}

func removeSchemaV29ForTestStatements() []string {
	return append(removeSchemaV30ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_model_cancellation_operation_immutable`,
		`DROP TRIGGER trg_specialist_model_cancellation_terminal_immutable`,
		`DROP TRIGGER trg_specialist_model_cancellation_identity_immutable`,
		`DROP TRIGGER trg_specialist_model_cancellation_resolve`,
		`DROP TRIGGER trg_specialist_model_cancellation_observe`,
		`DROP TRIGGER trg_specialist_model_cancellation_transition`,
		`DROP TRIGGER trg_specialist_model_cancellation_insert`,
		`DROP TABLE specialist_model_cancellation_operations`,
		`DROP TABLE specialist_model_cancellations`,
		`DROP TRIGGER trg_specialist_schedule_agent_immutable`,
		`DROP TRIGGER trg_specialist_schedule_terminal_immutable`,
		`DROP TRIGGER trg_specialist_schedule_identity_immutable`,
		`DROP TRIGGER trg_specialist_schedule_terminal`,
		`DROP TRIGGER trg_specialist_schedule_agent_insert`,
		`DROP TRIGGER trg_specialist_schedule_insert`,
		`DROP TABLE specialist_schedule_agents`,
		`DROP TABLE specialist_schedules`,
		`DELETE FROM schema_migrations WHERE version = 29`,
	}...)
}

func removeSchemaV30ForTestStatements() []string {
	return append(removeSchemaV31ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_delegation_operation_immutable`,
		`DROP TRIGGER trg_specialist_delegation_assignment_immutable`,
		`DROP TRIGGER trg_specialist_delegation_proposal_immutable`,
		`DROP TRIGGER trg_specialist_delegation_operation_insert`,
		`DROP TRIGGER trg_specialist_non_delegable_capability`,
		`DROP TRIGGER trg_specialist_delegation_assignment_insert`,
		`DROP TRIGGER trg_specialist_delegation_proposal_insert`,
		`DROP TABLE specialist_delegation_operations`,
		`DROP TABLE specialist_delegation_assignments`,
		`DROP TABLE specialist_delegation_proposals`,
		`DROP TRIGGER trg_supervisor_tool_call_model_attempt`,
		`DROP TRIGGER trg_supervisor_tool_round_completion`,
		`DROP INDEX idx_run_supervisor_tool_calls_pending`,
		`ALTER TABLE run_supervisor_tool_calls RENAME TO run_supervisor_tool_calls_v30`,
		supervisorToolLoopStatements[1],
		`INSERT INTO run_supervisor_tool_calls
			(run_id, turn, attempt_id, round, position, model_attempt, call_id, tool_name,
			payload_json, status, result_json, error_code, created_at, completed_at)
			SELECT run_id, turn, attempt_id, round, position, model_attempt, call_id, tool_name,
			payload_json, status, result_json, error_code, created_at, completed_at
			FROM run_supervisor_tool_calls_v30`,
		`DROP TABLE run_supervisor_tool_calls_v30`,
		supervisorToolLoopStatements[2],
		supervisorToolLoopStatements[5],
		supervisorToolLoopStatements[6],
		`DELETE FROM schema_migrations WHERE version = 30`,
	}...)
}

func removeSchemaV31ForTestStatements() []string {
	return append(removeSchemaV32ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_delegation_review_operation_delete_immutable`,
		`DROP TRIGGER trg_specialist_delegation_review_operation_immutable`,
		`DROP TRIGGER trg_specialist_delegation_review_delete_immutable`,
		`DROP TRIGGER trg_specialist_delegation_review_immutable`,
		`DROP TRIGGER trg_specialist_delegation_review_operation_insert`,
		`DROP TRIGGER trg_specialist_delegation_review_insert`,
		`DROP TABLE specialist_delegation_review_operations`,
		`DROP TABLE specialist_delegation_reviews`,
		`DELETE FROM schema_migrations WHERE version = 31`,
	}...)
}

func removeSchemaV32ForTestStatements() []string {
	return append(removeSchemaV33ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_delegation_application_operation_delete_immutable`,
		`DROP TRIGGER trg_specialist_delegation_application_operation_immutable`,
		`DROP TRIGGER trg_specialist_delegation_application_assignment_delete_immutable`,
		`DROP TRIGGER trg_specialist_delegation_application_delete_immutable`,
		`DROP TRIGGER trg_specialist_delegation_application_transition`,
		`DROP TRIGGER trg_specialist_delegation_application_complete`,
		`DROP TRIGGER trg_specialist_delegation_application_assignment_instructed`,
		`DROP TRIGGER trg_specialist_delegation_application_assignment_admitted`,
		`DROP TRIGGER trg_specialist_delegation_application_assignment_transition`,
		`DROP TRIGGER trg_specialist_delegation_application_operation_insert`,
		`DROP TRIGGER trg_specialist_delegation_application_assignment_insert`,
		`DROP TRIGGER trg_specialist_delegation_application_insert`,
		`DROP TABLE specialist_delegation_application_operations`,
		`DROP TABLE specialist_delegation_application_assignments`,
		`DROP TABLE specialist_delegation_applications`,
		`DELETE FROM schema_migrations WHERE version = 32`,
	}...)
}

func removeSchemaV33ForTestStatements() []string {
	return append(removeSchemaV34ForTestStatements(), []string{
		`DROP TRIGGER trg_readonly_fanout_operation_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_operation_immutable`,
		`DROP TRIGGER trg_readonly_fanout_shard_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_shard_immutable`,
		`DROP TRIGGER trg_readonly_fanout_file_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_file_immutable`,
		`DROP TRIGGER trg_readonly_fanout_plan_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_plan_immutable`,
		`DROP TRIGGER trg_readonly_fanout_operation_insert`,
		`DROP TRIGGER trg_readonly_fanout_shard_insert`,
		`DROP TRIGGER trg_readonly_fanout_file_insert`,
		`DROP TRIGGER trg_readonly_fanout_plan_insert`,
		`DROP TABLE readonly_fanout_operations`,
		`DROP TABLE readonly_fanout_shards`,
		`DROP TABLE readonly_fanout_files`,
		`DROP TABLE readonly_fanout_plans`,
		`DELETE FROM schema_migrations WHERE version = 33`,
	}...)
}

func removeSchemaV34ForTestStatements() []string {
	return append(removeSchemaV35ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_schedule_readonly_usage_insert`,
		`DROP TRIGGER trg_readonly_fanout_execution_operation_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_execution_operation_immutable`,
		`DROP TRIGGER trg_readonly_fanout_finding_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_finding_immutable`,
		`DROP TRIGGER trg_readonly_fanout_model_call_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_execution_shard_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_execution_delete_immutable`,
		`DROP TRIGGER trg_readonly_fanout_finding_insert`,
		`DROP TRIGGER trg_readonly_fanout_model_call_transition`,
		`DROP TRIGGER trg_readonly_fanout_model_call_insert`,
		`DROP TRIGGER trg_readonly_fanout_execution_shard_transition`,
		`DROP TRIGGER trg_readonly_fanout_execution_transition`,
		`DROP TRIGGER trg_readonly_fanout_execution_operation_insert`,
		`DROP TRIGGER trg_readonly_fanout_execution_shard_insert`,
		`DROP TRIGGER trg_readonly_fanout_execution_insert`,
		`DROP TABLE readonly_fanout_execution_operations`,
		`DROP TABLE readonly_fanout_findings`,
		`DROP TABLE readonly_fanout_model_calls`,
		`DROP TABLE readonly_fanout_execution_shards`,
		`DROP TABLE readonly_fanout_executions`,
		`ALTER TABLE specialist_schedules DROP COLUMN after_readonly_execution_millis`,
		`ALTER TABLE specialist_schedules DROP COLUMN after_readonly_tokens`,
		`ALTER TABLE specialist_schedules DROP COLUMN before_readonly_execution_millis`,
		`ALTER TABLE specialist_schedules DROP COLUMN before_readonly_tokens`,
		`DELETE FROM schema_migrations WHERE version = 34`,
	}...)
}

func removeSchemaV35ForTestStatements() []string {
	return append(removeSchemaV36ForTestStatements(), []string{
		`DROP TRIGGER trg_finding_evidence_delete_immutable`,
		`DROP TRIGGER trg_finding_evidence_update_immutable`,
		`DROP TRIGGER trg_finding_delete_immutable`,
		`DROP TRIGGER trg_finding_update_immutable`,
		`DROP TRIGGER trg_finding_report_delete_immutable`,
		`DROP TRIGGER trg_finding_report_generate`,
		`DROP TRIGGER trg_finding_evidence_insert`,
		`DROP TRIGGER trg_finding_insert`,
		`DROP TRIGGER trg_finding_report_insert`,
		`DROP TABLE finding_evidence`,
		`DROP TABLE findings`,
		`DROP TABLE finding_reports`,
		`DELETE FROM schema_migrations WHERE version = 35`,
	}...)
}

func removeSchemaV36ForTestStatements() []string {
	return append(removeSchemaV37ForTestStatements(), []string{
		`DROP TRIGGER trg_finding_validation_operation_delete_immutable`,
		`DROP TRIGGER trg_finding_validation_operation_update_immutable`,
		`DROP TRIGGER trg_finding_validation_delete_immutable`,
		`DROP TRIGGER trg_finding_validation_update_immutable`,
		`DROP TRIGGER trg_finding_artifact_evidence_operation_delete_immutable`,
		`DROP TRIGGER trg_finding_artifact_evidence_operation_update_immutable`,
		`DROP TRIGGER trg_finding_artifact_evidence_delete_immutable`,
		`DROP TRIGGER trg_finding_artifact_evidence_update_immutable`,
		`DROP TRIGGER trg_run_artifact_delete_immutable`,
		`DROP TRIGGER trg_run_artifact_update_immutable`,
		`DROP TRIGGER trg_finding_validation_operation_insert`,
		`DROP TRIGGER trg_finding_validation_insert`,
		`DROP TRIGGER trg_finding_artifact_evidence_operation_insert`,
		`DROP TRIGGER trg_finding_artifact_evidence_insert`,
		`DROP TABLE finding_validation_operations`,
		`DROP TABLE finding_validation_decisions`,
		`DROP TABLE finding_artifact_evidence_operations`,
		`DROP TABLE finding_artifact_evidence`,
		`DELETE FROM schema_migrations WHERE version = 36`,
	}...)
}

func removeSchemaV37ForTestStatements() []string {
	return append(removeSchemaV38ForTestStatements(), []string{
		`DROP TRIGGER trg_finding_fix_operation_delete_immutable`,
		`DROP TRIGGER trg_finding_fix_operation_update_immutable`,
		`DROP TRIGGER trg_finding_fix_delete_immutable`,
		`DROP TRIGGER trg_finding_fix_update_immutable`,
		`DROP TRIGGER trg_finding_remediation_evidence_operation_delete_immutable`,
		`DROP TRIGGER trg_finding_remediation_evidence_operation_update_immutable`,
		`DROP TRIGGER trg_finding_remediation_evidence_delete_immutable`,
		`DROP TRIGGER trg_finding_remediation_evidence_update_immutable`,
		`DROP TRIGGER trg_finding_acceptance_operation_delete_immutable`,
		`DROP TRIGGER trg_finding_acceptance_operation_update_immutable`,
		`DROP TRIGGER trg_finding_acceptance_delete_immutable`,
		`DROP TRIGGER trg_finding_acceptance_update_immutable`,
		`DROP TRIGGER trg_finding_fix_operation_insert`,
		`DROP TRIGGER trg_finding_fix_insert`,
		`DROP TRIGGER trg_finding_remediation_evidence_operation_insert`,
		`DROP TRIGGER trg_finding_remediation_evidence_insert`,
		`DROP TRIGGER trg_finding_acceptance_operation_insert`,
		`DROP TRIGGER trg_finding_acceptance_insert`,
		`DROP TABLE finding_fix_operations`,
		`DROP TABLE finding_fix_decisions`,
		`DROP TABLE finding_remediation_evidence_operations`,
		`DROP TABLE finding_remediation_evidence`,
		`DROP TABLE finding_acceptance_operations`,
		`DROP TABLE finding_acceptance_decisions`,
		`DELETE FROM schema_migrations WHERE version = 37`,
	}...)
}

func removeSchemaV38ForTestStatements() []string {
	return append(removeSchemaV39ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_operator_schedule_attempt_delete_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_attempt_update_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_operation_delete_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_operation_update_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_request_agent_delete_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_request_agent_update_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_request_delete_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_request_update_immutable`,
		`DROP TRIGGER trg_specialist_operator_schedule_attempt_insert`,
		`DROP TRIGGER trg_specialist_operator_schedule_operation_insert`,
		`DROP TRIGGER trg_specialist_operator_schedule_request_agent_insert`,
		`DROP TRIGGER trg_specialist_operator_schedule_request_insert`,
		`DROP INDEX idx_specialist_operator_schedule_request_agent`,
		`DROP INDEX idx_specialist_operator_schedule_request_run`,
		`DROP INDEX idx_specialist_operator_schedule_request_application`,
		`DROP TABLE specialist_operator_schedule_attempts`,
		`DROP TABLE specialist_operator_schedule_operations`,
		`DROP TABLE specialist_operator_schedule_request_agents`,
		`DROP TABLE specialist_operator_schedule_requests`,
		`DELETE FROM schema_migrations WHERE version = 38`,
	}...)
}

func removeSchemaV39ForTestStatements() []string {
	return append(removeSchemaV40ForTestStatements(), []string{
		`DROP TRIGGER trg_run_skill_selection_operation_delete_immutable`,
		`DROP TRIGGER trg_run_skill_selection_operation_update_immutable`,
		`DROP TRIGGER trg_run_skill_selection_item_delete_immutable`,
		`DROP TRIGGER trg_run_skill_selection_item_update_immutable`,
		`DROP TRIGGER trg_run_skill_selection_delete_immutable`,
		`DROP TRIGGER trg_run_skill_selection_update_immutable`,
		`DROP TRIGGER trg_run_skill_selection_operation_insert`,
		`DROP TRIGGER trg_run_skill_selection_item_insert`,
		`DROP TRIGGER trg_run_skill_selection_insert`,
		`DROP INDEX idx_run_skill_selections_mission`,
		`DROP TABLE run_skill_selection_operations`,
		`DROP TABLE run_skill_selection_items`,
		`DROP TABLE run_skill_selections`,
		`DELETE FROM schema_migrations WHERE version = 39`,
	}...)
}

func removeSchemaV40ForTestStatements() []string {
	return append(removeSchemaV41ForTestStatements(), []string{
		`DROP TRIGGER trg_root_skill_context_commit_delete_immutable`,
		`DROP TRIGGER trg_root_skill_context_commit_update_immutable`,
		`DROP TRIGGER trg_root_skill_context_preparation_delete_immutable`,
		`DROP TRIGGER trg_root_skill_context_preparation_update_immutable`,
		`DROP TRIGGER trg_root_skill_context_commit_insert`,
		`DROP TRIGGER trg_root_skill_context_preparation_insert`,
		`DROP INDEX idx_root_skill_context_run_turn`,
		`DROP TABLE root_skill_context_commits`,
		`DROP TABLE root_skill_context_preparations`,
		`DELETE FROM schema_migrations WHERE version = 40`,
	}...)
}

func removeSchemaV41ForTestStatements() []string {
	return append(removeSchemaV42ForTestStatements(), []string{
		`DROP TRIGGER trg_run_mode_plan_completion_guard`,
		`DROP TRIGGER trg_run_mode_operation_delete_immutable`,
		`DROP TRIGGER trg_run_mode_operation_update_immutable`,
		`DROP TRIGGER trg_run_mode_snapshot_delete_immutable`,
		`DROP TRIGGER trg_run_mode_snapshot_update_immutable`,
		`DROP TRIGGER trg_run_mode_operation_insert`,
		`DROP TRIGGER trg_run_mode_snapshot_insert`,
		`DROP TABLE run_mode_operations`,
		`DROP INDEX idx_run_mode_snapshots_run_revision`,
		`DROP TABLE run_mode_snapshots`,
		`DELETE FROM schema_migrations WHERE version = 41`,
	}...)
}

func removeSchemaV42ForTestStatements() []string {
	return append(removeSchemaV43ForTestStatements(), []string{
		`DROP TRIGGER trg_plan_delivery_selection_operation_delete_immutable`,
		`DROP TRIGGER trg_plan_delivery_selection_operation_update_immutable`,
		`DROP TRIGGER trg_plan_delivery_selection_item_delete_immutable`,
		`DROP TRIGGER trg_plan_delivery_selection_item_update_immutable`,
		`DROP TRIGGER trg_plan_delivery_selection_delete_immutable`,
		`DROP TRIGGER trg_plan_delivery_selection_update_immutable`,
		`DROP TRIGGER trg_plan_delivery_proposal_operation_delete_immutable`,
		`DROP TRIGGER trg_plan_delivery_proposal_operation_update_immutable`,
		`DROP TRIGGER trg_plan_delivery_module_delete_immutable`,
		`DROP TRIGGER trg_plan_delivery_module_update_immutable`,
		`DROP TRIGGER trg_plan_delivery_direction_delete_immutable`,
		`DROP TRIGGER trg_plan_delivery_direction_update_immutable`,
		`DROP TRIGGER trg_plan_delivery_proposal_delete_immutable`,
		`DROP TRIGGER trg_plan_delivery_proposal_update_immutable`,
		`DROP TRIGGER trg_plan_delivery_selection_operation_insert`,
		`DROP TRIGGER trg_plan_delivery_selection_item_insert`,
		`DROP TRIGGER trg_plan_delivery_selection_insert`,
		`DROP TRIGGER trg_plan_delivery_proposal_operation_insert`,
		`DROP TRIGGER trg_plan_delivery_module_insert`,
		`DROP TRIGGER trg_plan_delivery_direction_insert`,
		`DROP TRIGGER trg_plan_delivery_proposal_insert`,
		`DROP TABLE plan_delivery_selection_operations`,
		`DROP TABLE plan_delivery_selection_items`,
		`DROP TABLE plan_delivery_selections`,
		`DROP TABLE plan_delivery_proposal_operations`,
		`DROP TABLE plan_delivery_modules`,
		`DROP TABLE plan_delivery_directions`,
		`DROP INDEX idx_plan_delivery_proposals_run_created`,
		`DROP TABLE plan_delivery_proposals`,
		`DROP TRIGGER trg_supervisor_tool_call_model_attempt`,
		`DROP TRIGGER trg_supervisor_tool_round_completion`,
		`DROP INDEX idx_run_supervisor_tool_calls_pending`,
		`ALTER TABLE run_supervisor_tool_calls RENAME TO run_supervisor_tool_calls_v42`,
		`CREATE TABLE run_supervisor_tool_calls (
			run_id TEXT NOT NULL,
			turn INTEGER NOT NULL,
			attempt_id TEXT NOT NULL,
			round INTEGER NOT NULL,
			position INTEGER NOT NULL,
			model_attempt INTEGER NOT NULL,
			call_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			status TEXT NOT NULL,
			result_json TEXT NOT NULL DEFAULT '',
			error_code TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			completed_at TEXT,
			PRIMARY KEY(run_id, turn, attempt_id, round, position),
			UNIQUE(run_id, turn, attempt_id, call_id),
			FOREIGN KEY(run_id, turn, attempt_id, round)
				REFERENCES run_supervisor_tool_rounds(run_id, turn, attempt_id, round) ON DELETE CASCADE,
			CHECK(position BETWEEN 1 AND 4),
			CHECK(model_attempt > 0),
			CHECK(tool_name IN ('work_item_create', 'note_create', 'specialist_delegation_propose')),
			CHECK(status IN ('pending', 'completed', 'denied', 'failed')),
			CHECK((status = 'pending' AND result_json = '' AND error_code = '' AND completed_at IS NULL)
				OR (status = 'completed' AND length(result_json) > 0 AND error_code = '' AND completed_at IS NOT NULL)
				OR (status IN ('denied', 'failed') AND length(result_json) > 0 AND length(error_code) > 0
					AND completed_at IS NOT NULL))
		)`,
		`INSERT INTO run_supervisor_tool_calls
			(run_id, turn, attempt_id, round, position, model_attempt, call_id, tool_name,
			payload_json, status, result_json, error_code, created_at, completed_at)
			SELECT run_id, turn, attempt_id, round, position, model_attempt, call_id, tool_name,
			payload_json, status, result_json, error_code, created_at, completed_at
			FROM run_supervisor_tool_calls_v42`,
		`DROP TABLE run_supervisor_tool_calls_v42`,
		`CREATE INDEX idx_run_supervisor_tool_calls_pending
			ON run_supervisor_tool_calls(run_id, turn, attempt_id, status, round, position)`,
		`CREATE TRIGGER trg_supervisor_tool_call_model_attempt
			BEFORE INSERT ON run_supervisor_tool_calls
			WHEN NOT EXISTS (
				SELECT 1 FROM run_supervisor_tool_rounds
				WHERE run_id = NEW.run_id AND turn = NEW.turn AND attempt_id = NEW.attempt_id
					AND round = NEW.round AND model_attempt = NEW.model_attempt
			)
			BEGIN
				SELECT RAISE(ABORT, 'supervisor tool call model attempt mismatch');
			END`,
		`CREATE TRIGGER trg_supervisor_tool_round_completion
			BEFORE UPDATE OF completed_at ON run_supervisor_tool_rounds
			WHEN NEW.completed_at IS NOT NULL AND EXISTS (
				SELECT 1 FROM run_supervisor_tool_calls
				WHERE run_id = NEW.run_id AND turn = NEW.turn AND attempt_id = NEW.attempt_id
					AND round = NEW.round AND status = 'pending'
			)
			BEGIN
				SELECT RAISE(ABORT, 'supervisor tool round still has pending calls');
			END`,
		`DELETE FROM schema_migrations WHERE version = 42`,
	}...)
}

func removeSchemaV43ForTestStatements() []string {
	return append(removeSchemaV44ForTestStatements(), []string{
		`DROP TRIGGER trg_session_message_delete_immutable`,
		`DROP TRIGGER trg_session_message_compaction_monotonic`,
		`DROP TRIGGER trg_session_message_provenance_update_immutable`,
		`DROP TRIGGER trg_session_message_provenance_insert`,
		`DROP INDEX idx_session_messages_source_kind`,
		`ALTER TABLE session_messages DROP COLUMN instruction_authorized`,
		`ALTER TABLE session_messages DROP COLUMN content_sha256`,
		`ALTER TABLE session_messages DROP COLUMN source_ref`,
		`ALTER TABLE session_messages DROP COLUMN source_kind`,
		`ALTER TABLE session_messages DROP COLUMN provenance_version`,
		`DELETE FROM schema_migrations WHERE version = 43`,
	}...)
}

func removeSchemaV44ForTestStatements() []string {
	return append(removeSchemaV45ForTestStatements(), []string{
		`DROP TRIGGER trg_delivery_run_completion_guard`,
		`DROP TRIGGER trg_delivery_work_item_completion_guard`,
		`DROP TRIGGER trg_delivery_handoff_note_evidence_delete_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_evidence_update_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_evidence_insert_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_source_delete_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_source_update_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_source_insert_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_tag_delete_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_tag_update_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_tag_insert_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_delete_immutable`,
		`DROP TRIGGER trg_delivery_handoff_note_update_immutable`,
		`DROP TRIGGER trg_delivery_checkpoint_operation_delete_immutable`,
		`DROP TRIGGER trg_delivery_checkpoint_operation_update_immutable`,
		`DROP TRIGGER trg_delivery_checkpoint_delete_immutable`,
		`DROP TRIGGER trg_delivery_checkpoint_update_immutable`,
		`DROP TRIGGER trg_delivery_checkpoint_operation_insert`,
		`DROP TRIGGER trg_delivery_checkpoint_insert`,
		`DROP TRIGGER trg_delivery_gate_enrollment_delete_immutable`,
		`DROP TRIGGER trg_delivery_gate_enrollment_update_immutable`,
		`DROP TRIGGER trg_delivery_gate_enrollment_insert`,
		`DROP TRIGGER trg_delivery_gate_selection_enroll`,
		`DROP TABLE delivery_checkpoint_operations`,
		`DROP INDEX idx_delivery_checkpoints_run_module`,
		`DROP TABLE delivery_checkpoints`,
		`DROP TABLE delivery_gate_enrollments`,
		`DELETE FROM schema_migrations WHERE version = 44`,
	}...)
}

func removeSchemaV45ForTestStatements() []string {
	return append(removeSchemaV46ForTestStatements(), []string{
		`DROP TRIGGER trg_operator_steering_run_completion_guard`,
		`DROP TRIGGER trg_operator_steering_delivery_delete_immutable`,
		`DROP TRIGGER trg_operator_steering_delivery_update_monotonic`,
		`DROP TRIGGER trg_operator_steering_delivery_insert`,
		`DROP TRIGGER trg_operator_steering_operation_delete_immutable`,
		`DROP TRIGGER trg_operator_steering_operation_update_immutable`,
		`DROP TRIGGER trg_operator_steering_operation_insert`,
		`DROP TRIGGER trg_operator_steering_delete_immutable`,
		`DROP TRIGGER trg_operator_steering_commit_binding`,
		`DROP TRIGGER trg_operator_steering_update_monotonic`,
		`DROP TRIGGER trg_operator_steering_insert_binding`,
		`DROP INDEX idx_operator_steering_one_committed`,
		`DROP INDEX idx_operator_steering_one_prepared`,
		`DROP INDEX idx_operator_steering_deliveries_run_turn`,
		`DROP TABLE operator_steering_deliveries`,
		`DROP TABLE operator_steering_operations`,
		`DROP INDEX idx_operator_steering_run_status_sequence`,
		`DROP TABLE operator_steering_messages`,
		`DELETE FROM schema_migrations WHERE version = 45`,
	}...)
}

func removeSchemaV46ForTestStatements() []string {
	return append(removeSchemaV47ForTestStatements(), []string{
		`DROP TRIGGER trg_operator_steering_update_monotonic`,
		`DROP TRIGGER trg_operator_steering_cancellation_operation_delete_immutable`,
		`DROP TRIGGER trg_operator_steering_cancellation_operation_update_immutable`,
		`DROP TRIGGER trg_operator_steering_cancellation_operation_insert`,
		`DROP TRIGGER trg_operator_steering_cancellation_delete_immutable`,
		`DROP TRIGGER trg_operator_steering_cancellation_update_immutable`,
		`DROP TRIGGER trg_operator_steering_cancellation_insert`,
		`DROP TABLE operator_steering_cancellation_operations`,
		`DROP INDEX idx_operator_steering_cancellations_run_created`,
		`DROP TABLE operator_steering_cancellations`,
		`CREATE TRIGGER trg_operator_steering_update_monotonic
			BEFORE UPDATE ON operator_steering_messages
			WHEN NEW.id IS NOT OLD.id OR NEW.run_id IS NOT OLD.run_id
				OR NEW.session_id IS NOT OLD.session_id OR NEW.sequence IS NOT OLD.sequence
				OR NEW.content IS NOT OLD.content OR NEW.content_sha256 IS NOT OLD.content_sha256
				OR NEW.requested_by IS NOT OLD.requested_by OR NEW.created_at IS NOT OLD.created_at
				OR OLD.status != 'pending' OR NEW.status NOT IN ('committed', 'cancelled')
			BEGIN
				SELECT RAISE(ABORT, 'operator steering content is immutable and status is monotonic');
			END`,
		`DELETE FROM schema_migrations WHERE version = 46`,
	}...)
}

func removeSchemaV47ForTestStatements() []string {
	return append(removeSchemaV48ForTestStatements(), []string{
		`DROP TRIGGER trg_specialist_skill_context_commit_delete_immutable`,
		`DROP TRIGGER trg_specialist_skill_context_commit_update_immutable`,
		`DROP TRIGGER trg_specialist_skill_context_preparation_delete_immutable`,
		`DROP TRIGGER trg_specialist_skill_context_preparation_update_immutable`,
		`DROP TRIGGER trg_specialist_skill_context_commit_insert`,
		`DROP TRIGGER trg_specialist_skill_context_preparation_insert`,
		`DROP TABLE specialist_skill_context_commits`,
		`DROP INDEX idx_specialist_skill_context_run_agent_turn`,
		`DROP TABLE specialist_skill_context_preparations`,
		`DELETE FROM schema_migrations WHERE version = 47`,
	}...)
}

func removeSchemaV48ForTestStatements() []string {
	return append(removeSchemaV49ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_manifest_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_manifest_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_manifest_validation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_manifest_validation_update_immutable`,
		`DROP TRIGGER trg_sandbox_manifest_preparation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_manifest_preparation_update_immutable`,
		`DROP TRIGGER trg_sandbox_manifest_operation_insert`,
		`DROP TRIGGER trg_sandbox_manifest_validation_insert`,
		`DROP TRIGGER trg_sandbox_manifest_preparation_insert`,
		`DROP TABLE sandbox_manifest_operations`,
		`DROP TABLE sandbox_manifest_validations`,
		`DROP INDEX idx_sandbox_manifest_preparations_run_prepared`,
		`DROP TABLE sandbox_manifest_preparations`,
		`DELETE FROM schema_migrations WHERE version = 48`,
	}...)
}

func removeSchemaV49ForTestStatements() []string {
	return append(removeSchemaV50ForTestStatements(), []string{
		`DROP TRIGGER trg_sandbox_execution_candidate_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_execution_candidate_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_execution_candidate_delete_immutable`,
		`DROP TRIGGER trg_sandbox_execution_candidate_update_immutable`,
		`DROP TRIGGER trg_sandbox_execution_candidate_operation_insert`,
		`DROP TRIGGER trg_sandbox_execution_candidate_insert`,
		`DROP TABLE sandbox_execution_candidate_operations`,
		`DROP INDEX idx_sandbox_execution_candidates_run_validated`,
		`DROP TABLE sandbox_execution_candidates`,
		`DELETE FROM schema_migrations WHERE version = 49`,
	}...)
}

func removeSchemaV50ForTestStatements() []string {
	return []string{
		`DROP TRIGGER trg_sandbox_cleanup_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_cleanup_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_cleanup_result_delete_immutable`,
		`DROP TRIGGER trg_sandbox_cleanup_result_update_immutable`,
		`DROP TRIGGER trg_sandbox_execution_cancellation_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_execution_cancellation_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_execution_cancellation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_execution_cancellation_update_immutable`,
		`DROP TRIGGER trg_sandbox_execution_operation_delete_immutable`,
		`DROP TRIGGER trg_sandbox_execution_operation_update_immutable`,
		`DROP TRIGGER trg_sandbox_execution_input_delete_immutable`,
		`DROP TRIGGER trg_sandbox_execution_input_update_immutable`,
		`DROP TRIGGER trg_sandbox_disabled_execution_delete_immutable`,
		`DROP TRIGGER trg_sandbox_disabled_execution_update_immutable`,
		`DROP TRIGGER trg_sandbox_cleanup_operation_insert`,
		`DROP TRIGGER trg_sandbox_cleanup_result_insert`,
		`DROP TRIGGER trg_sandbox_execution_cancellation_operation_insert`,
		`DROP TRIGGER trg_sandbox_execution_cancellation_insert`,
		`DROP TRIGGER trg_sandbox_execution_operation_insert`,
		`DROP TRIGGER trg_sandbox_execution_lease_update`,
		`DROP TRIGGER trg_sandbox_execution_lease_insert`,
		`DROP TRIGGER trg_sandbox_execution_input_insert`,
		`DROP TRIGGER trg_sandbox_disabled_execution_insert`,
		`DROP TABLE sandbox_cleanup_operations`,
		`DROP TABLE sandbox_cleanup_results`,
		`DROP TABLE sandbox_execution_cancellation_operations`,
		`DROP TABLE sandbox_execution_cancellations`,
		`DROP TABLE sandbox_execution_operations`,
		`DROP INDEX idx_sandbox_execution_leases_status_expires`,
		`DROP TABLE sandbox_execution_leases`,
		`DROP TABLE sandbox_execution_inputs`,
		`DROP INDEX idx_sandbox_disabled_executions_run_created`,
		`DROP TABLE sandbox_disabled_executions`,
		`DELETE FROM schema_migrations WHERE version = 50`,
	}
}

func TestSQLiteUpgradesV21MemoryRowsToOptionalAgentOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	_, run := createWorkItemTestRun(t, ctx, st, "v21 memory ownership upgrade")
	workService := application.NewWorkItemService(st)
	legacyWork, err := workService.Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "legacy work", Owner: "legacy-worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	noteService := application.NewNoteService(st)
	legacyNote, err := noteService.Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "legacy note", Content: "preserved",
		Visibility: string(domain.NoteVisibilityOwner), Owner: "legacy-writer",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV22ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("simulate schema v21 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v21 did not upgrade to %d: version=%d err=%v", LatestSchemaVersion, version, err)
	}
	loadedWork, err := st.GetWorkItem(ctx, legacyWork.ID)
	if err != nil || loadedWork.Owner != "legacy-worker" || loadedWork.OwnerAgentID != "" {
		t.Fatalf("legacy WorkItem ownership was not preserved: %#v err=%v", loadedWork, err)
	}
	loadedNote, err := st.GetNote(ctx, legacyNote.ID)
	if err != nil || loadedNote.Owner != "legacy-writer" || loadedNote.OwnerAgentID != "" ||
		loadedNote.Content != "preserved" {
		t.Fatalf("legacy Note ownership was not preserved: %#v err=%v", loadedNote, err)
	}
	root, _, err := st.RegisterRootAgent(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	rootID := root.ID
	updatedWork, err := application.NewWorkItemService(st).Update(ctx, application.UpdateWorkItemRequest{
		ID: loadedWork.ID, ExpectedVersion: loadedWork.Version, OwnerAgentID: &rootID,
	})
	if err != nil || updatedWork.OwnerAgentID != root.ID {
		t.Fatalf("upgraded WorkItem cannot adopt Agent ownership: %#v err=%v", updatedWork, err)
	}
	updatedNote, err := application.NewNoteService(st).Update(ctx, application.UpdateNoteRequest{
		ID: loadedNote.ID, ExpectedVersion: loadedNote.Version, OwnerAgentID: &rootID,
	})
	if err != nil || updatedNote.OwnerAgentID != root.ID {
		t.Fatalf("upgraded Note cannot adopt Agent ownership: %#v err=%v", updatedNote, err)
	}
	var migrationCount int
	if err := st.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version = 22`).Scan(&migrationCount); err != nil || migrationCount != 1 {
		t.Fatalf("schema v22 migration ledger is inconsistent: count=%d err=%v", migrationCount, err)
	}
}

func TestSQLiteMigrationFailureRollsBackSchemaAndVersion(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	item := migration{
		Version: LatestSchemaVersion + 1,
		Name:    "rollback probe",
		Statements: []string{
			`CREATE TABLE rollback_probe (id INTEGER PRIMARY KEY);`,
			`THIS IS NOT VALID SQL`,
		},
	}
	if err := st.applyMigration(ctx, item); err == nil {
		t.Fatal("expected migration failure")
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rollback_probe'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration left rollback_probe table behind")
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != LatestSchemaVersion {
		t.Fatalf("failed migration changed schema version to %d", version)
	}
}

func TestSQLiteStoreRejectsIllegalRunTransitionAtPersistenceBoundary(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "test persistence transition", Profile: "code", Budget: domain.Budget{MaxTurns: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	malicious := run
	malicious.Status = domain.RunCompleted
	malicious.StartedAt = &now
	malicious.FinishedAt = &now
	malicious.UpdatedAt = now
	event, err := events.New(run.ID, run.MissionID, events.RunStatusChangedEvent, "test", run.ID, map[string]any{
		"from": domain.RunCreated, "to": domain.RunCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionRun(ctx, malicious, domain.RunCreated, event); err == nil {
		t.Fatal("expected persistence boundary to reject illegal transition")
	}
	loaded, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.RunCreated {
		t.Fatalf("illegal transition changed stored status: %#v", loaded)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].Type != events.RunCreatedEvent ||
		items[1].Type != events.SessionAttachedEvent || items[2].Type != events.RunModeSelectedEvent ||
		items[3].Type != events.AgentRegisteredEvent {
		t.Fatalf("illegal transition appended an event: %#v", items)
	}
}

func TestSQLiteStoreProjectsRunActivityAtomically(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "project activity", Profile: "code", WorkspaceID: "ws-projection", Budget: domain.Budget{MaxTurns: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveSessionMessage(ctx, session.NewMessage(run.SessionID, "user", "inspect the project")); err != nil {
		t.Fatal(err)
	}

	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	tool, err := toolManager.ProposeShell(ctx, run.SessionID, "ws-projection", "echo hello")
	if err != nil {
		t.Fatal(err)
	}
	approveStoredProposal(t, st, tool.ID, "projection-tool")
	tool, err = toolManager.Approve(ctx, tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveToolRun(ctx, tool); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	edit := fileedit.Edit{
		ID: "edit-projection", SessionID: run.SessionID, WorkspaceID: "ws-projection", Path: "notes.txt",
		Status: fileedit.StatusProposed, ProposedText: "hello", Diff: "+hello", OriginalHash: "missing",
		ProposedHash: fileedit.HashText("hello"), CreatedAt: now, UpdatedAt: now,
	}
	edit, err = st.SaveFileEdit(ctx, edit)
	if err != nil {
		t.Fatal(err)
	}
	approveStoredProposal(t, st, edit.ID, "projection-edit")
	edit.Status = fileedit.StatusApproved
	edit.UpdatedAt = time.Now().UTC()
	edit, err = st.SaveFileEdit(ctx, edit)
	if err != nil {
		t.Fatal(err)
	}
	edit.Status = fileedit.StatusApplied
	edit.UpdatedAt = time.Now().UTC()
	edit, err = st.SaveFileEdit(ctx, edit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveFileEdit(ctx, edit); err != nil {
		t.Fatal(err)
	}

	if err := st.RecordPolicyDecision(ctx, policy.DecisionRecord{
		SessionID: run.SessionID,
		SubjectID: run.SessionID,
		Context:   "assistant_response",
		Decision:  policy.Decision{Allowed: true, Reason: "allowed in test"},
	}); err != nil {
		t.Fatal(err)
	}

	items, err := service.Events(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := []string{
		events.RunCreatedEvent,
		events.SessionAttachedEvent,
		events.RunModeSelectedEvent,
		events.AgentRegisteredEvent,
		events.SessionMessageEvent,
		events.PolicyDecisionEvent,
		events.ToolProposedEvent,
		events.ApprovalRequestedEvent,
		events.ApprovalDecidedEvent,
		events.ToolApprovedEvent,
		events.ToolCompletedEvent,
		events.FileEditProposedEvent,
		events.ApprovalRequestedEvent,
		events.ApprovalDecidedEvent,
		events.FileEditApprovedEvent,
		events.FileEditAppliedEvent,
		events.PolicyDecisionEvent,
	}
	if len(items) != len(wantTypes) {
		t.Fatalf("unexpected projected timeline length: %#v", items)
	}
	for index, want := range wantTypes {
		if items[index].Sequence != int64(index+1) || items[index].Type != want {
			t.Fatalf("unexpected projected event at %d: %#v", index, items[index])
		}
	}

	bad := toolrun.ToolRun{
		ID: "tool-invalid", SessionID: run.SessionID, WorkspaceID: "ws-projection", ToolName: toolrun.ShellTool,
		Command: "echo invalid", Status: "unknown", CreatedAt: now, UpdatedAt: now,
	}
	if _, err := st.SaveToolRun(ctx, bad); err == nil {
		t.Fatal("expected invalid tool status to roll back")
	}
	if _, err := st.GetToolRun(ctx, bad.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("invalid tool run was persisted: %v", err)
	}
	crossWorkspace := bad
	crossWorkspace.ID = "tool-cross-workspace"
	crossWorkspace.Status = toolrun.StatusProposed
	crossWorkspace.WorkspaceID = "ws-other"
	if _, err := st.SaveToolRun(ctx, crossWorkspace); err == nil {
		t.Fatal("expected cross-workspace activity to be rejected")
	}
	if _, err := st.GetToolRun(ctx, crossWorkspace.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-workspace tool run was persisted: %v", err)
	}
}

func TestSQLiteStoreTaskAndEvents(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	task := agent.Task{
		ID:          "task-test",
		Kind:        agent.TaskScript,
		Goal:        "build parser",
		WorkspaceID: "ws-demo",
		Mode:        "python",
		Status:      agent.StatusPending,
		CreatedAt:   time.Now().UTC(),
	}
	if err := st.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEvent(ctx, agent.Event{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Type: "test.event", Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListEventsByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "test.event" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if err := st.UpdateTaskStatus(ctx, task.ID, agent.StatusCompleted); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != agent.StatusCompleted {
		t.Fatalf("expected completed, got %s", loaded.Status)
	}
}

func TestSQLiteStoreContextSummary(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	saved, err := st.SaveContextSummary(ctx, contextmgr.Summary{
		TaskID:                "task-context",
		WorkspaceID:           "ws-demo",
		Content:               "summary",
		SourceMessageCount:    5,
		PreservedMessageCount: 2,
		TokenEstimate:         10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == 0 {
		t.Fatal("expected summary id")
	}
	latest, ok, err := st.LatestContextSummary(ctx, "task-context")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || latest.Content != "summary" || latest.WorkspaceID != "ws-demo" {
		t.Fatalf("unexpected latest summary: %#v", latest)
	}
}

func TestSQLiteStoreRedactsSensitiveContent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	mimoToken := "t" + "p-" + strings.Repeat("a", 40)
	openAIToken := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz123456"
	openAIPrefix := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz"
	task := agent.Task{
		ID: "task-secret", Kind: agent.TaskScript, Goal: "inspect " + mimoToken, Mode: "python",
		Status: agent.StatusPending, CreatedAt: time.Now().UTC(),
	}
	if err := st.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	loadedTask, err := st.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(loadedTask.Goal, mimoToken[:11]) {
		t.Fatalf("secret stored in legacy task: %#v", loadedTask)
	}
	if err := st.RecordEvent(ctx, agent.Event{
		TaskID: task.ID, Type: "test.secret", Message: "observed " + mimoToken,
		PayloadJSON: `{"token":"` + mimoToken + `"}`,
	}); err != nil {
		t.Fatal(err)
	}
	legacyEvents, err := st.ListEventsByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacyEvents) != 1 || strings.Contains(legacyEvents[0].Message+legacyEvents[0].PayloadJSON, mimoToken[:11]) {
		t.Fatalf("secret stored in legacy event: %#v", legacyEvents)
	}
	saved, err := st.SaveContextSummary(ctx, contextmgr.Summary{
		TaskID:  "task-secret",
		Content: "OPENAI_API_KEY=" + openAIToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(saved.Content, openAIPrefix) {
		t.Fatalf("secret stored in summary: %#v", saved)
	}

	sess := session.Session{
		ID:        "sess-secret",
		Title:     "secret",
		Route:     "learn",
		Status:    session.StatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := st.SaveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	msg, err := st.SaveSessionMessage(ctx, session.Message{
		SessionID: sess.ID,
		Role:      "user",
		Content:   "MIMO_API_KEY=" + mimoToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.Content, mimoToken[:11]) {
		t.Fatalf("secret stored in message: %#v", msg)
	}

	run, err := st.SaveToolRun(ctx, toolrun.ToolRun{
		ID:       "tool-secret",
		ToolName: toolrun.ShellTool,
		Command:  "echo " + mimoToken,
		Status:   toolrun.StatusProposed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(run.Command, mimoToken[:11]) {
		t.Fatalf("secret stored in tool run: %#v", run)
	}

	rawEditContent := "MIMO_API_KEY=" + mimoToken + "\n"
	edit, err := st.SaveFileEdit(ctx, fileedit.Edit{
		ID:           "edit-secret",
		WorkspaceID:  "ws-demo",
		Path:         "env.txt",
		Status:       fileedit.StatusProposed,
		ProposedText: rawEditContent,
		ProposedHash: fileedit.HashText(rawEditContent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(edit.ProposedText, mimoToken[:11]) || !edit.SecretsRedacted {
		t.Fatalf("secret stored in file edit: %#v", edit)
	}
	if edit.ProposedHash != fileedit.HashText(edit.ProposedText) {
		t.Fatalf("redacted file edit hash was not updated: %#v", edit)
	}
}

func TestSQLiteStoreSessionsAndMessages(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	sess := session.Session{
		ID:          "sess-test",
		WorkspaceID: "ws-demo",
		Title:       "demo",
		Route:       "learn",
		Status:      session.StatusActive,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := st.SaveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	msg, err := st.SaveSessionMessage(ctx, session.NewMessage(sess.ID, "user", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID == 0 {
		t.Fatal("expected message id")
	}
	messages, err := st.ListSessionMessages(ctx, sess.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if _, err := st.MarkSessionMessagesCompacted(ctx, sess.ID, msg.ID); err != nil {
		t.Fatal(err)
	}
	active, err := st.ListSessionMessages(ctx, sess.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active messages, got %#v", active)
	}
	all, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || !all[0].Compacted {
		t.Fatalf("expected compacted message in all history: %#v", all)
	}
}

func TestSQLiteStoreToolRuns(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	run := toolrun.ToolRun{
		ID:          "tool-test",
		SessionID:   "sess-test",
		WorkspaceID: "ws-demo",
		ToolName:    toolrun.ShellTool,
		Command:     "echo hello",
		Status:      toolrun.StatusProposed,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if _, err := st.SaveToolRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.GetToolRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Command != "echo hello" || loaded.Status != toolrun.StatusProposed {
		t.Fatalf("unexpected loaded run: %#v", loaded)
	}
	approveStoredProposal(t, st, loaded.ID, "store-tool")
	loaded.Status = toolrun.StatusCompleted
	loaded.Stdout = "dry run: echo hello"
	if _, err := st.SaveToolRun(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListToolRuns(ctx, toolrun.ListFilter{SessionID: "sess-test", Status: toolrun.StatusCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Stdout == "" {
		t.Fatalf("unexpected runs: %#v", runs)
	}
}

func TestSQLiteStoreFileEdits(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	edit := fileedit.Edit{
		ID:           "edit-test",
		SessionID:    "sess-test",
		WorkspaceID:  "ws-demo",
		Path:         "README.md",
		Status:       fileedit.StatusProposed,
		OriginalText: "old\n",
		ProposedText: "new\n",
		Diff:         "-old\n+new\n",
		OriginalHash: "old-hash",
		ProposedHash: "new-hash",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if _, err := st.SaveFileEdit(ctx, edit); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.GetFileEdit(ctx, edit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Path != "README.md" || loaded.Status != fileedit.StatusProposed {
		t.Fatalf("unexpected file edit: %#v", loaded)
	}
	approveStoredProposal(t, st, loaded.ID, "store-edit")
	loaded.Status = fileedit.StatusApplied
	if _, err := st.SaveFileEdit(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	edits, err := st.ListFileEdits(ctx, fileedit.ListFilter{WorkspaceID: "ws-demo", Status: fileedit.StatusApplied})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].ID != edit.ID {
		t.Fatalf("unexpected file edit list: %#v", edits)
	}
}

func approveStoredProposal(t *testing.T, st *SQLiteStore, proposalID string, suffix string) approval.Record {
	t.Helper()
	result, err := st.DecideApproval(context.Background(), approval.DecisionRequest{
		ProposalID: proposalID, IdempotencyKey: "test-approval:" + suffix,
		Action: approval.ActionApprove, ReviewedBy: "store_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.Approval
}
