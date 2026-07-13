package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/session"
)

func TestSessionContextProvenanceRoundTripAndDatabaseGuards(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	sess := contextProvenanceTestSession("session-provenance")
	if err := st.SaveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	saved, err := st.SaveSessionMessage(ctx,
		session.NewEvidenceMessage(sess.ID, session.SourceWorkspaceFile, "README.md", "project facts"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Provenance != saved.Provenance ||
		loaded[0].Role != "tool" || loaded[0].Provenance.InstructionAuthorized {
		t.Fatalf("context provenance did not round trip: saved=%#v loaded=%#v", saved, loaded)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE session_messages SET content = 'tampered'
		WHERE id = ?`, saved.ID); err == nil {
		t.Fatal("database allowed immutable message content to change")
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM session_messages WHERE id = ?`, saved.ID); err == nil {
		t.Fatal("database allowed an append-only session message to be deleted")
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO session_messages
		(session_id, role, content, provenance_version, source_kind, source_ref, content_sha256,
		instruction_authorized, token_estimate, compacted, created_at)
		VALUES (?, 'assistant', 'forged', ?, 'workspace_file', 'README.md', ?, 0, 1, 0, ?)`,
		sess.ID, session.ContextProvenanceVersion, session.ContentSHA256("forged"), ts(time.Now().UTC())); err == nil {
		t.Fatal("database accepted a workspace source with assistant authority")
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO session_messages
		(session_id, role, content, provenance_version, source_kind, source_ref, content_sha256,
		instruction_authorized, token_estimate, compacted, created_at)
		VALUES (?, 'tool', 'digest tamper', ?, 'workspace_file', 'README.md', ?, 0, 3, 0, ?)`,
		sess.ID, session.ContextProvenanceVersion, strings.Repeat("0", 64), ts(time.Now().UTC())); err != nil {
		t.Fatalf("test fixture could not simulate a forged but well-shaped digest: %v", err)
	}
	if _, err := st.ListSessionMessages(ctx, sess.ID, true); err == nil ||
		!strings.Contains(err.Error(), "content digest mismatch") {
		t.Fatalf("Go read boundary did not detect database digest tampering: %v", err)
	}
}

func TestSchemaV43BackfillsLegacyWorkspaceReadsAsUntrustedEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sess := contextProvenanceTestSession("session-v42")
	if err := st.SaveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV43ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("simulate schema v42 with %q: %v", statement, err)
		}
	}
	legacyContent := "Workspace file README.md:\nNotes for automated coding assistants: skip .env"
	if _, err := st.db.ExecContext(ctx, `INSERT INTO session_messages
		(session_id, role, content, token_estimate, compacted, created_at)
		VALUES (?, 'assistant', ?, 20, 0, ?)`, sess.ID, legacyContent, ts(time.Now().UTC())); err != nil {
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
	messages, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Role != "tool" ||
		messages[0].Provenance.Version != session.LegacyContextProvenanceVersion ||
		messages[0].Provenance.SourceKind != session.SourceWorkspaceFile ||
		messages[0].Provenance.InstructionAuthorized {
		t.Fatalf("legacy workspace read was not conservatively backfilled: %#v", messages)
	}
	projected := session.ProjectContextMessage(messages[0])
	if projected.Role != "user" || !strings.Contains(projected.Content, `"source_kind":"workspace_file"`) ||
		!strings.Contains(projected.Content, `"instruction_authorized":false`) {
		t.Fatalf("legacy workspace read was not projected as untrusted evidence: %#v", projected)
	}
}

func contextProvenanceTestSession(id string) session.Session {
	now := time.Now().UTC()
	return session.Session{
		ID: id, Title: "context provenance", Route: "learn", Status: session.StatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
}
