package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
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
	return []string{
		`DROP TRIGGER trg_agent_completion_running_child`,
		`DROP TRIGGER trg_agent_completion_message_matches`,
		`DROP TRIGGER trg_agent_completed_requires_report`,
		`DROP TRIGGER trg_agent_completion_immutable`,
		`DROP TABLE agent_completion_operations`,
		`DROP TABLE agent_completion_reports`,
		`DELETE FROM schema_migrations WHERE version = 23`,
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
	if len(items) != 3 || items[0].Type != events.RunCreatedEvent ||
		items[1].Type != events.SessionAttachedEvent || items[2].Type != events.AgentRegisteredEvent {
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
