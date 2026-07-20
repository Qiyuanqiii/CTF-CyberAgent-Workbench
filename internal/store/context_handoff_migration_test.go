package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/contextmgr"
)

func removeSchemaV82ForTestStatements() []string {
	return append(removeSchemaV83ForTestStatements(), []string{
		`DROP TRIGGER trg_context_summaries_delete_immutable`,
		`DROP TRIGGER trg_context_summaries_update_immutable`,
		`DROP TRIGGER trg_context_summaries_v1_insert`,
		`DROP INDEX idx_context_summaries_previous_v1`,
		`ALTER TABLE context_summaries DROP COLUMN compacted_message_count`,
		`ALTER TABLE context_summaries DROP COLUMN content_sha256`,
		`ALTER TABLE context_summaries DROP COLUMN previous_summary_id`,
		`ALTER TABLE context_summaries DROP COLUMN protocol_version`,
		`DELETE FROM schema_migrations WHERE version = 82`,
	}...)
}

func TestSchemaV82UpgradePreservesAndFoldsLegacySummary(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "context-handoff-v81.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV82ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			_ = st.Close()
			t.Fatalf("remove v82 with %q: %v", statement, err)
		}
	}
	createdAt := time.Now().UTC().Add(-time.Minute)
	result, err := st.db.ExecContext(ctx, `INSERT INTO context_summaries
		(task_id, workspace_id, content, source_message_count,
		 preserved_message_count, token_estimate, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "task-legacy-handoff", "ws-demo",
		"legacy objective and completed work", 5, 2, 8, ts(createdAt))
	if err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	legacyID, err := result.LastInsertId()
	if err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	legacy, ok, err := st.LatestContextSummary(ctx, "task-legacy-handoff")
	if err != nil || !ok {
		t.Fatalf("load migrated legacy summary: ok=%t err=%v", ok, err)
	}
	if legacy.ID != legacyID || legacy.ProtocolVersion != contextmgr.LegacyHandoffProtocolVersion ||
		legacy.Content != "legacy objective and completed work" {
		t.Fatalf("unexpected migrated legacy summary: %#v", legacy)
	}

	manager := contextmgr.NewManager(st, contextmgr.Config{
		MaxMessagesBeforeCompact: 2, PreserveRecentMessages: 1,
		MaxSummaryChars: 3000, MaxLineChars: 240,
	})
	folded, err := manager.Compact(ctx, "task-legacy-handoff", "ws-demo", []contextmgr.Message{
		{Role: "assistant", Content: "continued after restart"},
		{Role: "user", Content: "next objective", SourceKind: "operator_message", InstructionAuthorized: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if folded.Summary.PreviousSummaryID != legacyID ||
		folded.Summary.ProtocolVersion != contextmgr.HandoffMemoryProtocolVersion ||
		folded.Summary.CompactedMessageCount != 4 ||
		!strings.Contains(folded.Summary.Content, "legacy objective and completed work") ||
		!strings.Contains(folded.Summary.Content, "continued after restart") {
		t.Fatalf("legacy summary was not folded into cumulative handoff: %#v", folded.Summary)
	}
	rolledBack, err := st.SaveContextSummary(ctx, contextmgr.Summary{
		TaskID: "task-legacy-handoff", WorkspaceID: "ws-demo",
		PreviousSummaryID: folded.Summary.ID, Content: "clock rollback append",
		SourceMessageCount: 6, PreservedMessageCount: 1,
		CreatedAt: createdAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.CreatedAt.Before(folded.Summary.CreatedAt) {
		t.Fatalf("clock rollback broke handoff chronology: previous=%s next=%s",
			folded.Summary.CreatedAt, rolledBack.CreatedAt)
	}

	if _, err := st.db.ExecContext(ctx, `UPDATE context_summaries SET content = 'tampered' WHERE id = ?`,
		folded.Summary.ID); err == nil {
		t.Fatal("v82 allowed a handoff summary update")
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM context_summaries WHERE id = ?`,
		folded.Summary.ID); err == nil {
		t.Fatal("v82 allowed a handoff summary deletion")
	}
	_, err = st.SaveContextSummary(ctx, contextmgr.Summary{
		TaskID: "task-legacy-handoff", WorkspaceID: "ws-demo",
		PreviousSummaryID: legacyID, Content: "stale branch",
		SourceMessageCount: 2, PreservedMessageCount: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale handoff append error = %v", err)
	}
}
