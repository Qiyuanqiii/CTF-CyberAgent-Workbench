package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/session"
)

func TestSQLiteUpgradesV18AndLazilyRegistersExistingRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v18.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "upgrade coordinator", Profile: "review", Budget: domain.Budget{MaxTurns: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE agent_graph_snapshots`,
		`DROP TABLE agent_messages`,
		`DROP TABLE agent_nodes`,
		`DELETE FROM run_events WHERE type LIKE 'agent.%'`,
		`DELETE FROM schema_migrations WHERE version = 19`,
	} {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare v18 schema with %q: %v", statement, err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
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
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("v18 database did not upgrade to v19: version=%d err=%v", version, err)
	}
	if _, found, err := st.GetRootAgent(ctx, run.ID); err != nil || found {
		t.Fatalf("migration unexpectedly invented a root outside a Run transaction: found=%t err=%v", found, err)
	}
	root, created, err := st.RegisterRootAgent(ctx, run.ID)
	if err != nil || !created || root.Status != domain.AgentReady {
		t.Fatalf("lazy root registration failed: root=%#v created=%t err=%v", root, created, err)
	}
	repeated, created, err := st.RegisterRootAgent(ctx, run.ID)
	if err != nil || created || repeated.ID != root.ID {
		t.Fatalf("lazy registration was not idempotent: root=%#v created=%t err=%v", repeated, created, err)
	}
	graph, err := st.RestoreAgentGraph(ctx, run.ID)
	if err != nil || graph.RootAgentID != root.ID || graph.LatestSnapshot.Version != 1 {
		t.Fatalf("upgraded graph is not recoverable: graph=%#v err=%v", graph, err)
	}
}

func TestAgentGraphSnapshotDetectsInboxTamperingAndRetainsBoundedHistory(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "coordinator.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "snapshot integrity", Profile: "code", Budget: domain.Budget{MaxTurns: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, found, err := st.GetRootAgent(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("root agent was not created: found=%t err=%v", found, err)
	}
	message, err := st.SendAgentMessage(ctx, domain.AgentMessage{
		ID: idgen.New("agentmsg"), RunID: run.ID, RecipientAgentID: root.ID,
		Kind: domain.AgentMessageInstruction, PayloadJSON: `{"goal":"inspect"}`,
		Status: domain.AgentMessagePending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RestoreAgentGraph(ctx, run.ID); err != nil {
		t.Fatalf("valid graph did not restore: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE agent_messages SET payload_json = ? WHERE id = ?`,
		`{"goal":"tampered"}`, message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RestoreAgentGraph(ctx, run.ID); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered inbox was not rejected: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE agent_messages SET payload_json = ? WHERE id = ?`,
		message.PayloadJSON, message.ID); err != nil {
		t.Fatal(err)
	}
	for range domain.MaxAgentGraphSnapshots + 5 {
		if _, err := st.SnapshotAgentGraph(ctx, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var minimum, maximum int64
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(version), MAX(version)
		FROM agent_graph_snapshots WHERE run_id = ?`, run.ID).Scan(&count, &minimum, &maximum); err != nil {
		t.Fatal(err)
	}
	if count != domain.MaxAgentGraphSnapshots || maximum-minimum != int64(domain.MaxAgentGraphSnapshots-1) {
		t.Fatalf("snapshot retention is not bounded: count=%d min=%d max=%d", count, minimum, maximum)
	}
}

func TestAgentCoordinatorSchemaKeepsChildCreationDisabled(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "coordinator.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "no child execution", Profile: "code", Budget: domain.Budget{MaxTurns: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, found, err := st.GetRootAgent(ctx, run.ID)
	if err != nil || !found || root.ChildLimit != 0 {
		t.Fatalf("unexpected root: root=%#v found=%t err=%v", root, found, err)
	}
	childSession := session.New("", "blocked child", "code")
	if err := st.SaveSession(ctx, childSession); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = st.db.ExecContext(ctx, `INSERT INTO agent_nodes
		(id, run_id, parent_id, session_id, role, profile, skills_json, status, depth, child_limit,
		turn_limit, token_limit, turns_used, tokens_used, active_attempt_id, status_reason, version,
		created_at, updated_at, finished_at)
		VALUES (?, ?, ?, ?, 'specialist', 'code', '["model.chat"]', 'ready', 1, 0,
		1, 0, 0, 0, '', '', 1, ?, ?, NULL)`, idgen.New("agent"), run.ID, root.ID,
		childSession.ID, ts(now), ts(now))
	if err == nil || !strings.Contains(err.Error(), "child depth or limit") {
		t.Fatalf("schema allowed a child while root child_limit=0: %v", err)
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_nodes WHERE run_id = ?`, run.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("blocked child insert changed graph size to %d", count)
	}
}
