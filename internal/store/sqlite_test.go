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
	if len(items) != 2 || items[0].Type != events.RunCreatedEvent || items[1].Type != events.SessionAttachedEvent {
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
		events.SessionMessageEvent,
		events.PolicyDecisionEvent,
		events.ToolProposedEvent,
		events.ToolApprovedEvent,
		events.ToolCompletedEvent,
		events.FileEditProposedEvent,
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
