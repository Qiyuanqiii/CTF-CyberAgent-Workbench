package toolgateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/toolrun"
)

type memoryStore struct {
	mu         sync.Mutex
	runs       map[string]toolrun.ToolRun
	edits      map[string]fileedit.Edit
	approvals  map[string]approval.Record
	operations map[string]string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		runs: map[string]toolrun.ToolRun{}, edits: map[string]fileedit.Edit{},
		approvals: map[string]approval.Record{}, operations: map[string]string{},
	}
}

func (s *memoryStore) SaveToolRun(_ context.Context, run toolrun.ToolRun) (toolrun.ToolRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	return run, nil
}

func (s *memoryStore) GetToolRun(_ context.Context, id string) (toolrun.ToolRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return toolrun.ToolRun{}, errors.New("tool run not found")
	}
	return run, nil
}

func (s *memoryStore) ListToolRuns(_ context.Context, filter toolrun.ListFilter) ([]toolrun.ToolRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var runs []toolrun.ToolRun
	for _, run := range s.runs {
		if filter.SessionID != "" && filter.SessionID != run.SessionID {
			continue
		}
		if filter.Status != "" && filter.Status != run.Status {
			continue
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (s *memoryStore) SaveFileEdit(_ context.Context, edit fileedit.Edit) (fileedit.Edit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edits[edit.ID] = edit
	return edit, nil
}

func (s *memoryStore) GetFileEdit(_ context.Context, id string) (fileedit.Edit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	edit, ok := s.edits[id]
	if !ok {
		return fileedit.Edit{}, errors.New("file edit not found")
	}
	return edit, nil
}

func (s *memoryStore) ListFileEdits(_ context.Context, filter fileedit.ListFilter) ([]fileedit.Edit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var edits []fileedit.Edit
	for _, edit := range s.edits {
		if filter.SessionID != "" && filter.SessionID != edit.SessionID {
			continue
		}
		if filter.WorkspaceID != "" && filter.WorkspaceID != edit.WorkspaceID {
			continue
		}
		if filter.Status != "" && filter.Status != edit.Status {
			continue
		}
		edits = append(edits, edit)
	}
	return edits, nil
}

func (s *memoryStore) EnsureApproval(_ context.Context, proposal approval.Proposal) (approval.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.approvals[proposal.ProposalID]; ok {
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
	s.approvals[proposal.ProposalID] = record
	return record, nil
}

func (s *memoryStore) DecideApproval(_ context.Context, request approval.DecisionRequest) (approval.DecisionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalized, err := request.Normalize()
	if err != nil {
		return approval.DecisionResult{}, err
	}
	if proposalID, ok := s.operations[normalized.IdempotencyKey]; ok {
		if proposalID != normalized.ProposalID {
			return approval.DecisionResult{}, errors.New("idempotency conflict")
		}
		return approval.DecisionResult{Approval: s.approvals[proposalID], Replayed: true}, nil
	}
	record, ok := s.approvals[normalized.ProposalID]
	if !ok {
		return approval.DecisionResult{}, errors.New("approval not found")
	}
	desired, _ := normalized.Action.Status()
	if record.Status != approval.StatusPending && record.Status != desired {
		return approval.DecisionResult{}, errors.New("approval conflict")
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
		s.approvals[normalized.ProposalID] = record
	}
	s.operations[normalized.IdempotencyKey] = normalized.ProposalID
	return approval.DecisionResult{Approval: record, Replayed: replayed}, nil
}

func (s *memoryStore) GetApproval(_ context.Context, id string) (approval.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.approvals {
		if record.ID == id {
			return record, nil
		}
	}
	return approval.Record{}, errors.New("approval not found")
}

func (s *memoryStore) GetApprovalByProposal(_ context.Context, proposalID string) (approval.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.approvals[proposalID]
	if !ok {
		return approval.Record{}, errors.New("approval not found")
	}
	return record, nil
}

func (s *memoryStore) ListApprovals(_ context.Context, filter approval.ListFilter) ([]approval.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []approval.Record
	for _, record := range s.approvals {
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

func TestGatewayExecutesScopedReadsWithRedactionAndLimits(t *testing.T) {
	root := t.TempDir()
	token := "s" + "k-" + strings.Repeat("a", 28)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("DEEPSEEK_API_KEY="+token+"\nmore"), 0o600); err != nil {
		t.Fatal(err)
	}
	gateway := New(nil, policy.NewDefaultChecker())
	outcome, err := gateway.Invoke(context.Background(), ToolCall{
		Name: ReadFileTool, WorkspaceID: "ws-1", WorkspaceRoot: root,
		Arguments: map[string]string{"path": "notes.txt", "max_bytes": "64"}, RequestedBy: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Decision.Approval != ApprovalAutomatic || outcome.Execution == nil || outcome.Result == nil ||
		outcome.Result.Status != StatusCompleted || outcome.Result.MIME != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected read outcome: %#v", outcome)
	}
	if outcome.Call.WorkspaceRoot != "" || strings.Contains(outcome.Result.Stdout, token) ||
		!strings.Contains(outcome.Result.Stdout, "[REDACTED:secret]") {
		t.Fatalf("workspace root or secret escaped the gateway: %#v", outcome)
	}
	truncated, err := gateway.Invoke(context.Background(), ToolCall{
		Name: ReadFileTool, WorkspaceID: "ws-1", WorkspaceRoot: root,
		Arguments: map[string]string{"path": "notes.txt", "max_bytes": "3"},
	})
	if err != nil || truncated.Result == nil || !truncated.Result.Truncated {
		t.Fatalf("read truncation was not projected: %#v err=%v", truncated, err)
	}
	if _, err := gateway.Invoke(context.Background(), ToolCall{
		Name: ReadFileTool, WorkspaceID: "ws-1", WorkspaceRoot: root,
		Arguments: map[string]string{"path": "notes.txt", "max_bytes": "999999"},
	}); err == nil {
		t.Fatal("expected oversized read limit rejection")
	}
}

func TestGatewayShellProposalApprovalAndPolicyDenial(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	gateway := New(store, policy.NewDefaultChecker())
	outcome, err := gateway.Invoke(ctx, ToolCall{
		Name: ShellTool, SessionID: "sess-1", WorkspaceID: "ws-1",
		Arguments: map[string]string{"command": "echo hello"}, RequestedBy: "root",
	})
	if err != nil || outcome.Proposal == nil || outcome.Proposal.Status != StatusProposed ||
		outcome.Decision.Approval != ApprovalPerCall || outcome.Execution != nil {
		t.Fatalf("unexpected shell proposal: %#v err=%v", outcome, err)
	}
	operatorProposal, err := gateway.Invoke(ctx, ToolCall{
		Name: ShellTool, SessionID: "sess-1", WorkspaceID: "ws-1",
		Arguments: map[string]string{"command": "echo decline"},
	})
	if err != nil {
		t.Fatal(err)
	}
	operatorDenied, err := gateway.Review(ctx, ReviewRequest{
		Action: ReviewDeny, Tool: ShellTool, ProposalID: operatorProposal.Proposal.ID,
	})
	if err != nil || operatorDenied.Decision.Reason != "denied by operator" ||
		operatorDenied.Result == nil || operatorDenied.Result.Stderr != "denied by operator" {
		t.Fatalf("operator denial reason was inconsistent: %#v err=%v", operatorDenied, err)
	}
	persistedOperatorDenial, err := store.GetToolRun(ctx, operatorProposal.Proposal.ID)
	if err != nil || persistedOperatorDenial.PolicyReason != "denied by operator" {
		t.Fatalf("operator denial reason was not persisted: %#v err=%v", persistedOperatorDenial, err)
	}
	reviewed, err := gateway.Review(ctx, ReviewRequest{
		Action: ReviewApprove, Tool: ShellTool, ProposalID: outcome.Proposal.ID,
	})
	if err != nil || reviewed.Proposal.Status != StatusCompleted || reviewed.Execution == nil ||
		reviewed.Execution.Backend != "dry_run" || reviewed.Result == nil ||
		!strings.Contains(reviewed.Result.Stdout, "dry run: echo hello") {
		t.Fatalf("unexpected shell review: %#v err=%v", reviewed, err)
	}
	denied, err := gateway.Invoke(ctx, ToolCall{
		Name: ShellTool, SessionID: "sess-1", WorkspaceID: "ws-1",
		Arguments: map[string]string{"command": "masscan 0.0.0.0/0"},
	})
	if err != nil || denied.Proposal == nil || denied.Proposal.Status != StatusDenied ||
		denied.Decision.Allowed || denied.Decision.Approval != ApprovalNever || denied.Result == nil {
		t.Fatalf("dangerous shell was not denied durably: %#v err=%v", denied, err)
	}
	persisted, err := store.GetToolRun(ctx, denied.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	mapped, mapErr := gateway.outcomeFromToolRun(ToolCall{RequestedBy: "test"}, persisted, errors.New("post-persist failure"))
	if mapErr == nil || mapped.Decision.Allowed || mapped.Decision.Approval != ApprovalNever {
		t.Fatalf("persisted denial changed meaning after an operation error: %#v err=%v", mapped, mapErr)
	}
}

func TestGatewayFileEditProposalApprovalAndAdapterCompatibility(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	gateway := New(store, policy.NewDefaultChecker()).WithWorkspaceRootResolver(func(context.Context, string) (string, error) {
		return root, nil
	})
	otherRoot := t.TempDir()
	if _, err := gateway.Invoke(ctx, ToolCall{
		Name: ReplaceFileTool, WorkspaceID: "ws-1", WorkspaceRoot: otherRoot,
		Arguments: map[string]string{"path": "README.md", "content": "wrong root\n"},
	}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched workspace root was not rejected: %v", err)
	}
	outcome, err := gateway.Invoke(ctx, ToolCall{
		Name: ReplaceFileTool, WorkspaceID: "ws-1", WorkspaceRoot: root, SessionID: "sess-1",
		Arguments: map[string]string{"path": "README.md", "content": "new\n"}, RequestedBy: "root",
	})
	if err != nil || outcome.Proposal == nil || outcome.Proposal.Status != StatusProposed ||
		outcome.Decision.Approval != ApprovalPerCall || !strings.Contains(outcome.Proposal.Preview, "+new") {
		t.Fatalf("unexpected file proposal: %#v err=%v", outcome, err)
	}
	before, err := os.ReadFile(path)
	if err != nil || string(before) != "old\n" {
		t.Fatalf("proposal changed workspace: %q err=%v", before, err)
	}
	reviewed, err := gateway.Review(ctx, ReviewRequest{
		Action: ReviewApprove, Tool: ReplaceFileTool, ProposalID: outcome.Proposal.ID,
	})
	if err != nil || reviewed.Proposal.Status != StatusCompleted || reviewed.Execution == nil || reviewed.Result == nil ||
		reviewed.Result.Metadata["path"] != "README.md" {
		t.Fatalf("unexpected file approval: %#v err=%v", reviewed, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != "new\n" {
		t.Fatalf("approved edit was not applied: %q err=%v", after, err)
	}

	adapter := gateway.FileEdits()
	second, err := adapter.Propose(ctx, fileedit.Proposal{
		WorkspaceID: "ws-1", WorkspaceRoot: root, Path: "README.md", ProposedText: "third\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err = adapter.Deny(ctx, second.ID, "not now")
	if err != nil || second.Status != fileedit.StatusDenied || second.Reason != "not now" {
		t.Fatalf("file adapter did not use gateway review: %#v err=%v", second, err)
	}
}

func TestGatewayRejectsUnknownArgumentsAndDangerousFileProposal(t *testing.T) {
	root := t.TempDir()
	gateway := New(newMemoryStore(), policy.NewDefaultChecker())
	if _, err := gateway.Invoke(context.Background(), ToolCall{
		Name: ReadFileTool, WorkspaceID: "ws", WorkspaceRoot: root,
		Arguments: map[string]string{"path": "README.md", "extra": "value"},
	}); err == nil {
		t.Fatal("expected unknown argument rejection")
	}
	denied, err := gateway.Invoke(context.Background(), ToolCall{
		Name: ReplaceFileTool, WorkspaceID: "ws", WorkspaceRoot: root,
		Arguments: map[string]string{"path": "notes.txt", "content": "masscan 0.0.0.0/0"},
	})
	if err != nil || denied.Decision.Allowed || denied.Result == nil || denied.Result.Status != StatusDenied || denied.Proposal != nil {
		t.Fatalf("dangerous file proposal was not denied: %#v err=%v", denied, err)
	}
}

func TestBoundResultTextPreservesUTF8AndHardLimit(t *testing.T) {
	value, truncated := boundResultText(strings.Repeat("界", 100), 64)
	if !truncated || !utf8.ValidString(value) || len([]byte(value)) > 64 || !strings.Contains(value, "[truncated") {
		t.Fatalf("invalid bounded result: bytes=%d valid=%t value=%q", len([]byte(value)), utf8.ValidString(value), value)
	}
	tiny, truncated := boundResultText(strings.Repeat("abcdef", 4), 8)
	if !truncated || len([]byte(tiny)) != 8 {
		t.Fatalf("tiny hard limit was not enforced: bytes=%d value=%q", len([]byte(tiny)), tiny)
	}
}
