package toolgateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
)

func TestGatewayApprovalRecoversAcrossStoreRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "approval.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{ID: "ws-approval", Name: "approval", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "verify approval recovery", Profile: "code", WorkspaceID: "ws-approval",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, policy.NewDefaultChecker())
	outcome, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo safe"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-approval", RequestedBy: "integration_test",
	})
	if err != nil || outcome.Proposal == nil {
		t.Fatalf("shell proposal failed: %#v err=%v", outcome, err)
	}
	record, err := st.GetApprovalByProposal(ctx, outcome.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.RunID != run.ID || record.SessionID != run.SessionID || record.Status != approval.StatusPending {
		t.Fatalf("approval was not bound to the run: %#v", record)
	}
	reviewKey := approval.ReviewIdempotencyKey("shell", outcome.Proposal.ID, approval.ActionApprove)
	if _, err := st.DecideApproval(ctx, approval.DecisionRequest{
		ProposalID: outcome.Proposal.ID, IdempotencyKey: reviewKey,
		Action: approval.ActionApprove, ReviewedBy: "integration_operator",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	gateway = toolgateway.New(st, policy.NewDefaultChecker())
	request := toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ShellTool, ProposalID: outcome.Proposal.ID,
		IdempotencyKey: reviewKey, ReviewedBy: "integration_operator",
	}
	recovered, err := gateway.Review(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Proposal == nil || recovered.Proposal.Status != toolgateway.StatusCompleted || recovered.Result == nil || recovered.Result.Status != toolgateway.StatusCompleted {
		t.Fatalf("approval did not converge after restart: %#v", recovered)
	}
	if _, err := gateway.Review(ctx, request); err != nil {
		t.Fatalf("repeated review was not idempotent: %v", err)
	}
	runEvents, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	requested, decided := 0, 0
	for _, event := range runEvents {
		switch event.Type {
		case events.ApprovalRequestedEvent:
			requested++
		case events.ApprovalDecidedEvent:
			decided++
		}
	}
	if requested != 1 || decided != 1 {
		t.Fatalf("approval events were duplicated: requested=%d decided=%d", requested, decided)
	}
}

func TestGatewayUsesRevocableSessionGrantWithoutBypassingPolicy(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "session-grant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{ID: "ws-session-grant", Name: "session-grant", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "session grant integration", Profile: "code", WorkspaceID: "ws-session-grant",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	fileGrant, err := st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "replace_file", ActionClass: "workspace_write",
		GrantedBy: "integration_operator", IdempotencyKey: "file-session-grant",
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, policy.NewDefaultChecker()).WithWorkspaceRootResolver(func(context.Context, string) (string, error) {
		return root, nil
	})
	automatic, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ReplaceFileTool, Arguments: map[string]string{"path": "automatic.txt", "content": "authorized\n"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-session-grant", RequestedBy: "integration_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if automatic.Decision.Approval != toolgateway.ApprovalSession || automatic.Proposal == nil ||
		automatic.Proposal.Status != toolgateway.StatusCompleted {
		t.Fatalf("active grant did not authorize the file edit: %#v", automatic)
	}
	written, err := os.ReadFile(filepath.Join(root, "automatic.txt"))
	if err != nil || string(written) != "authorized\n" {
		t.Fatalf("session-authorized file edit was not applied: %q err=%v", written, err)
	}
	ledger, err := st.GetApprovalByProposal(ctx, automatic.Proposal.ID)
	if err != nil || ledger.GrantID != fileGrant.Grant.ID || ledger.Status != approval.StatusApproved {
		t.Fatalf("session grant was not linked to the approval ledger: %#v err=%v", ledger, err)
	}
	if _, err := st.RevokeSessionGrant(ctx, approval.RevokeGrantRequest{
		GrantID: fileGrant.Grant.ID, RevokedBy: "integration_operator", IdempotencyKey: "revoke-file-session-grant",
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ReplaceFileTool, Arguments: map[string]string{"path": "after-revoke.txt", "content": "pending\n"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-session-grant", RequestedBy: "integration_test",
	})
	if err != nil || pending.Decision.Approval != toolgateway.ApprovalPerCall || pending.Proposal == nil ||
		pending.Proposal.Status != toolgateway.StatusProposed {
		t.Fatalf("revoked grant still authorized a file edit: %#v err=%v", pending, err)
	}
	if _, err := os.Stat(filepath.Join(root, "after-revoke.txt")); !os.IsNotExist(err) {
		t.Fatalf("file changed after grant revocation: %v", err)
	}
	if _, err := st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "shell", ActionClass: "shell",
		GrantedBy: "integration_operator", IdempotencyKey: "shell-session-grant",
	}); err != nil {
		t.Fatal(err)
	}
	denied, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "masscan 0.0.0.0/0"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-session-grant", RequestedBy: "integration_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if denied.Decision.Allowed || denied.Decision.Approval != toolgateway.ApprovalNever || denied.Result == nil ||
		denied.Result.Status != toolgateway.StatusDenied {
		t.Fatalf("session grant bypassed a permanent policy denial: %#v", denied)
	}
}

func TestGatewayEnforcesRunToolCallBudgetBeforeSecondExecution(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("bounded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "gateway-budget.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{ID: "ws-gateway-budget", Name: "gateway-budget", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "one tool call", Profile: "code", WorkspaceID: "ws-gateway-budget",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, policy.NewDefaultChecker()).WithWorkspaceRootResolver(func(context.Context, string) (string, error) {
		return root, nil
	})
	call := toolgateway.ToolCall{
		Name: toolgateway.ReadFileTool, Arguments: map[string]string{"path": "readme.txt"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-gateway-budget", RequestedBy: "integration_test",
	}
	first, err := gateway.Invoke(ctx, call)
	if err != nil || first.Result == nil || first.Result.Stdout != "bounded\n" {
		t.Fatalf("first bounded call failed: %#v err=%v", first, err)
	}
	if _, err := gateway.Invoke(ctx, call); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("second call was not blocked by the Run budget: %v", err)
	}
	usage, err := st.GetToolCallUsage(ctx, run.ID)
	if err != nil || usage.Consumed != 1 || usage.Remaining != 0 {
		t.Fatalf("gateway budget usage is wrong: %#v err=%v", usage, err)
	}
}

func TestGatewayFileApprovalAppliesOnceAfterDurableDecision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "approval.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{ID: "ws-file", Name: "file", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "apply a reviewed file", Profile: "code", WorkspaceID: "ws-file", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, policy.NewDefaultChecker()).WithWorkspaceRootResolver(func(context.Context, string) (string, error) {
		return root, nil
	})
	outcome, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ReplaceFileTool, Arguments: map[string]string{"path": "reviewed.txt", "content": "approved\n"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-file", RequestedBy: "integration_test",
	})
	if err != nil || outcome.Proposal == nil {
		t.Fatalf("file proposal failed: %#v err=%v", outcome, err)
	}
	request := toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ReplaceFileTool, ProposalID: outcome.Proposal.ID,
		WorkspaceRoot: root, IdempotencyKey: "review:file:apply-once", ReviewedBy: "integration_operator",
	}
	if _, err := gateway.Review(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Review(ctx, request); err != nil {
		t.Fatalf("repeated file approval failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "reviewed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "approved\n" {
		t.Fatalf("unexpected applied file: %q", data)
	}
	record, err := st.GetApprovalByProposal(ctx, outcome.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != approval.StatusApproved || record.Version != 2 {
		t.Fatalf("repeated file approval changed the decision: %#v", record)
	}
}
