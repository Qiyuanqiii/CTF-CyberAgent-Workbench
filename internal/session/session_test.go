package session

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/toolbudget"
	"cyberagent-workbench/internal/toolrun"
)

type memorySessionStore struct {
	sessions   map[string]Session
	messages   []Message
	summaries  []contextmgr.Summary
	fileEdits  map[string]fileedit.Edit
	toolRuns   map[string]toolrun.ToolRun
	approvals  map[string]approval.Record
	workspaces map[string]WorkspaceInfo
	nextMsgID  int64
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{
		sessions:   map[string]Session{},
		fileEdits:  map[string]fileedit.Edit{},
		toolRuns:   map[string]toolrun.ToolRun{},
		approvals:  map[string]approval.Record{},
		workspaces: map[string]WorkspaceInfo{},
	}
}

func (m *memorySessionStore) SaveSession(ctx context.Context, session Session) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *memorySessionStore) GetSession(ctx context.Context, id string) (Session, error) {
	session, ok := m.sessions[id]
	if !ok {
		return Session{}, errNotFound("session")
	}
	return session, nil
}

func (m *memorySessionStore) ListSessions(ctx context.Context) ([]Session, error) {
	out := make([]Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		out = append(out, session)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memorySessionStore) GetWorkspaceInfo(ctx context.Context, id string) (WorkspaceInfo, error) {
	workspace, ok := m.workspaces[id]
	if !ok {
		return WorkspaceInfo{}, errNotFound("workspace")
	}
	return workspace, nil
}

func (m *memorySessionStore) SaveSessionMessage(ctx context.Context, message Message) (Message, error) {
	m.nextMsgID++
	message.ID = m.nextMsgID
	m.messages = append(m.messages, message)
	return message, nil
}

func (m *memorySessionStore) ListSessionMessages(ctx context.Context, sessionID string, includeCompacted bool) ([]Message, error) {
	var out []Message
	for _, message := range m.messages {
		if message.SessionID != sessionID {
			continue
		}
		if message.Compacted && !includeCompacted {
			continue
		}
		out = append(out, message)
	}
	return out, nil
}

func (m *memorySessionStore) MarkSessionMessagesCompacted(ctx context.Context, sessionID string, throughID int64) (int64, error) {
	var count int64
	for i := range m.messages {
		if m.messages[i].SessionID == sessionID && m.messages[i].ID <= throughID && !m.messages[i].Compacted {
			m.messages[i].Compacted = true
			count++
		}
	}
	return count, nil
}

func (m *memorySessionStore) SaveContextSummary(ctx context.Context, summary contextmgr.Summary) (contextmgr.Summary, error) {
	summary.ID = int64(len(m.summaries) + 1)
	m.summaries = append(m.summaries, summary)
	return summary, nil
}

func (m *memorySessionStore) LatestContextSummary(ctx context.Context, taskID string) (contextmgr.Summary, bool, error) {
	for i := len(m.summaries) - 1; i >= 0; i-- {
		if m.summaries[i].TaskID == taskID {
			return m.summaries[i], true, nil
		}
	}
	return contextmgr.Summary{}, false, nil
}

func (m *memorySessionStore) SaveFileEdit(ctx context.Context, edit fileedit.Edit) (fileedit.Edit, error) {
	m.fileEdits[edit.ID] = edit
	return edit, nil
}

func (m *memorySessionStore) GetFileEdit(ctx context.Context, id string) (fileedit.Edit, error) {
	edit, ok := m.fileEdits[id]
	if !ok {
		return fileedit.Edit{}, errNotFound("file edit")
	}
	return edit, nil
}

func (m *memorySessionStore) ListFileEdits(ctx context.Context, filter fileedit.ListFilter) ([]fileedit.Edit, error) {
	var out []fileedit.Edit
	for _, edit := range m.fileEdits {
		if filter.SessionID != "" && edit.SessionID != filter.SessionID {
			continue
		}
		if filter.WorkspaceID != "" && edit.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if filter.Status != "" && edit.Status != filter.Status {
			continue
		}
		out = append(out, edit)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memorySessionStore) SaveToolRun(ctx context.Context, run toolrun.ToolRun) (toolrun.ToolRun, error) {
	m.toolRuns[run.ID] = run
	return run, nil
}

func (m *memorySessionStore) GetToolRun(ctx context.Context, id string) (toolrun.ToolRun, error) {
	run, ok := m.toolRuns[id]
	if !ok {
		return toolrun.ToolRun{}, errNotFound("tool run")
	}
	return run, nil
}

func (m *memorySessionStore) ListToolRuns(ctx context.Context, filter toolrun.ListFilter) ([]toolrun.ToolRun, error) {
	var out []toolrun.ToolRun
	for _, run := range m.toolRuns {
		if filter.SessionID != "" && run.SessionID != filter.SessionID {
			continue
		}
		if filter.Status != "" && run.Status != filter.Status {
			continue
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memorySessionStore) EnsureApproval(ctx context.Context, proposal approval.Proposal) (approval.Record, error) {
	if record, ok := m.approvals[proposal.ProposalID]; ok {
		return record, nil
	}
	now := proposal.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated := proposal.UpdatedAt
	if updated.IsZero() {
		updated = now
	}
	record := approval.Record{
		ID: "approval-" + proposal.ProposalID, IdempotencyKey: proposal.IdempotencyKey, ProposalID: proposal.ProposalID,
		SessionID: proposal.SessionID, WorkspaceID: proposal.WorkspaceID, ToolName: proposal.ToolName,
		ActionClass: proposal.ActionClass, Mode: proposal.Mode, Status: proposal.Status,
		RequestFingerprint: proposal.RequestFingerprint, DecisionReason: proposal.DecisionReason,
		RequestedBy: proposal.RequestedBy, ReviewedBy: proposal.ReviewedBy, Version: 1,
		CreatedAt: now, UpdatedAt: updated, DecidedAt: proposal.DecidedAt,
	}
	m.approvals[proposal.ProposalID] = record
	return record, nil
}

func (m *memorySessionStore) DecideApproval(ctx context.Context, request approval.DecisionRequest) (approval.DecisionResult, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return approval.DecisionResult{}, err
	}
	record, ok := m.approvals[normalized.ProposalID]
	if !ok {
		return approval.DecisionResult{}, errNotFound("approval")
	}
	desired, _ := normalized.Action.Status()
	if record.Status != approval.StatusPending && record.Status != desired {
		return approval.DecisionResult{}, errNotFound("compatible approval")
	}
	replayed := record.Status == desired
	if !replayed {
		now := time.Now().UTC()
		record.Status = desired
		record.DecisionReason = normalized.Reason
		record.ReviewedBy = normalized.ReviewedBy
		record.DecidedAt = &now
		record.UpdatedAt = now
		record.Version++
		m.approvals[normalized.ProposalID] = record
	}
	return approval.DecisionResult{Approval: record, Replayed: replayed}, nil
}

func (m *memorySessionStore) GetApproval(ctx context.Context, id string) (approval.Record, error) {
	for _, record := range m.approvals {
		if record.ID == id {
			return record, nil
		}
	}
	return approval.Record{}, errNotFound("approval")
}

func (m *memorySessionStore) GetApprovalByProposal(ctx context.Context, proposalID string) (approval.Record, error) {
	record, ok := m.approvals[proposalID]
	if !ok {
		return approval.Record{}, errNotFound("approval")
	}
	return record, nil
}

func (m *memorySessionStore) ListApprovals(ctx context.Context, filter approval.ListFilter) ([]approval.Record, error) {
	var records []approval.Record
	for _, record := range m.approvals {
		if filter.RunID != "" && record.RunID != filter.RunID ||
			filter.SessionID != "" && record.SessionID != filter.SessionID ||
			filter.Status != "" && record.Status != filter.Status ||
			filter.ToolName != "" && record.ToolName != filter.ToolName {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (m *memorySessionStore) CreateSessionGrant(context.Context, approval.CreateGrantRequest) (approval.GrantResult, error) {
	return approval.GrantResult{}, errNotFound("session grant")
}

func (m *memorySessionStore) RevokeSessionGrant(context.Context, approval.RevokeGrantRequest) (approval.GrantResult, error) {
	return approval.GrantResult{}, errNotFound("session grant")
}

func (m *memorySessionStore) AuthorizeApprovalWithSessionGrant(context.Context, string, string) (approval.DecisionResult, error) {
	return approval.DecisionResult{}, errNotFound("session grant")
}

func (m *memorySessionStore) FindActiveSessionGrant(context.Context, approval.GrantQuery) (approval.SessionGrant, bool, error) {
	return approval.SessionGrant{}, false, nil
}

func (m *memorySessionStore) GetSessionGrant(context.Context, string) (approval.SessionGrant, error) {
	return approval.SessionGrant{}, errNotFound("session grant")
}

func (m *memorySessionStore) ListSessionGrants(context.Context, approval.GrantListFilter) ([]approval.SessionGrant, error) {
	return nil, nil
}

func (m *memorySessionStore) ChargeToolCall(context.Context, toolbudget.ChargeRequest) (toolbudget.Usage, error) {
	return toolbudget.Usage{Remaining: -1}, nil
}

func (m *memorySessionStore) GetToolCallUsage(context.Context, string) (toolbudget.Usage, error) {
	return toolbudget.Usage{Remaining: -1}, nil
}

func (m *memorySessionStore) RecordPolicyDecision(context.Context, policy.DecisionRecord) error {
	return nil
}

func (m *memorySessionStore) CaptureToolOutput(context.Context, artifact.CaptureRequest) ([]artifact.Descriptor, error) {
	return nil, errNotFound("Run artifact")
}

func (m *memorySessionStore) GetRunArtifact(context.Context, string) (artifact.Blob, error) {
	return artifact.Blob{}, errNotFound("Run artifact")
}

func (m *memorySessionStore) ListRunArtifacts(context.Context, artifact.ListFilter) ([]artifact.Descriptor, error) {
	return nil, nil
}

type errNotFound string

func (e errNotFound) Error() string { return string(e) + " not found" }

func TestSessionSendPersistsUserAndAssistantMessages(t *testing.T) {
	store := newMemorySessionStore()
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Send(context.Background(), sess.ID, "explain this code")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text == "" {
		t.Fatal("expected assistant text")
	}
	history, err := manager.History(context.Background(), sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("unexpected history: %#v", history)
	}
}

func TestSessionSlashModelUpdatesRoute(t *testing.T) {
	store := newMemorySessionStore()
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Send(context.Background(), sess.ID, "/model script")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Command {
		t.Fatal("expected slash command result")
	}
	updated, err := store.GetSession(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Route != "script" {
		t.Fatalf("expected script route, got %s", updated.Route)
	}
}

func TestSessionSlashModelAcceptsDirectModelRef(t *testing.T) {
	store := newMemorySessionStore()
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Send(context.Background(), sess.ID, "/model mock/mock-code")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "mock/mock-code") {
		t.Fatalf("expected direct model ref in response, got %q", result.Text)
	}
	result, err = manager.Send(context.Background(), sess.ID, "write code")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "[mock-code]") {
		t.Fatalf("expected direct model ref to be used, got %q", result.Text)
	}
}

func TestSessionRunCreatesToolProposal(t *testing.T) {
	store := newMemorySessionStore()
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Send(context.Background(), sess.ID, "/run echo hello")
	if err != nil {
		t.Fatal(err)
	}
	if result.ToolRunID == "" {
		t.Fatal("expected tool run id")
	}
	run, err := store.GetToolRun(context.Background(), result.ToolRunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != toolrun.StatusProposed || run.Command != "echo hello" || run.SessionID != sess.ID {
		t.Fatalf("unexpected tool run: %#v", run)
	}
}

func TestSessionRunDeniedByPolicy(t *testing.T) {
	store := newMemorySessionStore()
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Send(context.Background(), sess.ID, "/run masscan 0.0.0.0/0")
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.GetToolRun(context.Background(), result.ToolRunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != toolrun.StatusDenied {
		t.Fatalf("expected denied tool run, got %#v", run)
	}
}

func TestSessionWriteCreatesFileEditProposal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("# Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newMemorySessionStore()
	store.workspaces["ws-demo"] = WorkspaceInfo{ID: "ws-demo", Name: "demo", RootPath: root}
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Send(context.Background(), sess.ID, "/write README.md # Updated")
	if err != nil {
		t.Fatal(err)
	}
	if result.FileEditID == "" || !strings.Contains(result.Text, "-# Old") || !strings.Contains(result.Text, "+# Updated") {
		t.Fatalf("unexpected write proposal result: %#v", result)
	}
	edit, err := store.GetFileEdit(context.Background(), result.FileEditID)
	if err != nil {
		t.Fatal(err)
	}
	if edit.Status != fileedit.StatusProposed || edit.SessionID != sess.ID {
		t.Fatalf("unexpected file edit: %#v", edit)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Old\n" {
		t.Fatalf("proposal wrote before approval: %q", data)
	}
}

func TestSessionWorkspaceSlashListAndReadAreScoped(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mimoToken := "t" + "p-" + strings.Repeat("a", 40)
	if err := os.WriteFile(filepath.Join(root, "env.txt"), []byte("MIMO_API_KEY="+mimoToken+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "example.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemorySessionStore()
	store.workspaces["ws-demo"] = WorkspaceInfo{ID: "ws-demo", Name: "demo", RootPath: root}
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}

	result, err := manager.Send(context.Background(), sess.ID, "/ls .")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "README.md") || !strings.Contains(result.Text, "scripts") {
		t.Fatalf("unexpected /ls response: %q", result.Text)
	}

	result, err = manager.Send(context.Background(), sess.ID, "/read README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "# Demo") {
		t.Fatalf("unexpected /read response: %q", result.Text)
	}

	result, err = manager.Send(context.Background(), sess.ID, "/read ../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "escapes workspace") {
		t.Fatalf("expected scoped read denial, got %q", result.Text)
	}

	result, err = manager.Send(context.Background(), sess.ID, "/read env.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Text, mimoToken[:11]) || !strings.Contains(result.Text, "[REDACTED:secret]") {
		t.Fatalf("expected redacted /read response, got %q", result.Text)
	}
	history, err := manager.History(context.Background(), sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range history {
		if strings.Contains(msg.Content, mimoToken[:11]) {
			t.Fatalf("secret persisted in history: %#v", msg)
		}
	}
}

func TestSessionAutoCompactsLongHistory(t *testing.T) {
	store := newMemorySessionStore()
	manager := NewManager(store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	sess, err := manager.Create(context.Background(), "", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	var last SendResult
	for i := 0; i < 5; i++ {
		last, err = manager.Send(context.Background(), sess.ID, "message")
		if err != nil {
			t.Fatal(err)
		}
	}
	if !last.Compacted || last.SummaryID == 0 {
		t.Fatalf("expected automatic compaction, got %#v", last)
	}
	active, err := manager.History(context.Background(), sess.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) >= 10 {
		t.Fatalf("expected compacted history to shrink active messages, got %d", len(active))
	}
}
