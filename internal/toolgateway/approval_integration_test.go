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
	"cyberagent-workbench/internal/tools"
)

type sessionReviewDenyChecker struct{}

func (sessionReviewDenyChecker) CheckText(string, string) policy.Decision {
	return policy.Decision{Allowed: true, Reason: "proposal allowed"}
}

func (sessionReviewDenyChecker) CheckToolCall(tools.Call) policy.Decision {
	return policy.Decision{Allowed: false, Reason: "current policy denied session approval", Risk: "high"}
}

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

func TestToolRunAdapterApprovesCurrentAndFutureSafeShellsForSession(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "adapter-session-grant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{
		ID: "ws-adapter-session", Name: "adapter-session", RootPath: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "approve safe shell commands for this session", Profile: "code", WorkspaceID: "ws-adapter-session",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, policy.NewDefaultChecker())
	call := func(command string) (toolgateway.Outcome, error) {
		return gateway.Invoke(ctx, toolgateway.ToolCall{
			Name: toolgateway.ShellTool, Arguments: map[string]string{"command": command},
			RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-adapter-session",
			RequestedBy: "integration_test",
		})
	}

	first, err := call("echo first")
	if err != nil || first.Proposal == nil || first.Proposal.Status != toolgateway.StatusProposed {
		t.Fatalf("first shell was not proposed: %#v err=%v", first, err)
	}
	if _, _, err := gateway.ToolRuns().ApproveForSession(ctx, first.Proposal.ID, "sess-other",
		"wrong scope", "integration_operator", "wrong-session-grant"); err == nil {
		t.Fatal("expected mismatched Session scope to be rejected")
	}
	grants, err := st.ListSessionGrants(ctx, approval.GrantListFilter{RunID: run.ID, Status: approval.GrantActive})
	if err != nil || len(grants) != 0 {
		t.Fatalf("scope mismatch left an active grant: %#v err=%v", grants, err)
	}
	after, grant, err := gateway.ToolRuns().ApproveForSession(ctx, first.Proposal.ID, run.SessionID,
		"trusted commands in this session", "integration_operator", "adapter-session-grant")
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "completed" || after.Stdout != "dry run: echo first" ||
		grant.RunID != run.ID || grant.SessionID != run.SessionID || grant.WorkspaceID != "ws-adapter-session" ||
		grant.ToolName != "shell" || grant.ActionClass != "shell" || grant.Status != approval.GrantActive {
		t.Fatalf("session approval scope or dry-run result is wrong: run=%#v grant=%#v", after, grant)
	}
	ledger, err := st.GetApprovalByProposal(ctx, first.Proposal.ID)
	if err != nil || ledger.Status != approval.StatusApproved || ledger.GrantID != grant.ID {
		t.Fatalf("current proposal was not linked to its grant: %#v err=%v", ledger, err)
	}

	automatic, err := call("echo second")
	if err != nil || automatic.Decision.Approval != toolgateway.ApprovalSession ||
		automatic.Proposal == nil || automatic.Proposal.Status != toolgateway.StatusCompleted ||
		automatic.Result == nil || automatic.Result.Stdout != "dry run: echo second" {
		t.Fatalf("future safe shell did not use the active grant: %#v err=%v", automatic, err)
	}

	if _, err := st.RevokeSessionGrant(ctx, approval.RevokeGrantRequest{
		GrantID: grant.ID, Reason: "exercise recovery", RevokedBy: "integration_operator",
		IdempotencyKey: "revoke-adapter-session-grant",
	}); err != nil {
		t.Fatal(err)
	}
	recoverable, err := call("echo recover")
	if err != nil || recoverable.Proposal == nil || recoverable.Proposal.Status != toolgateway.StatusProposed {
		t.Fatalf("recoverable shell was not proposed: %#v err=%v", recoverable, err)
	}
	recoveryGrant, err := st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, WorkspaceID: "ws-adapter-session", ToolName: "shell", ActionClass: "shell",
		Reason: "resume an authorized proposal", GrantedBy: "integration_operator",
		IdempotencyKey: "adapter-recovery-session-grant",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthorizeApprovalWithSessionGrant(ctx, recoverable.Proposal.ID, recoveryGrant.Grant.ID); err != nil {
		t.Fatal(err)
	}
	recovered, recoveredGrant, err := gateway.ToolRuns().ApproveForSession(ctx, recoverable.Proposal.ID, run.SessionID,
		"trusted commands in this session", "integration_operator", "unused-recovery-operation")
	if err != nil || recovered.Status != "completed" || recoveredGrant.ID != recoveryGrant.Grant.ID {
		t.Fatalf("authorized proposal did not recover: run=%#v grant=%#v err=%v", recovered, recoveredGrant, err)
	}

	denied, err := call("masscan 0.0.0.0/0")
	if err != nil || denied.Decision.Allowed || denied.Decision.Approval != toolgateway.ApprovalNever ||
		denied.Result == nil || denied.Result.Status != toolgateway.StatusDenied {
		t.Fatalf("active session grant bypassed permanent policy denial: %#v err=%v", denied, err)
	}
}

func TestToolRunAdapterRechecksPolicyBeforeCreatingSessionGrant(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "adapter-policy-recheck.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{
		ID: "ws-adapter-policy", Name: "adapter-policy", RootPath: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "recheck session approval policy", Profile: "code", WorkspaceID: "ws-adapter-policy",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, sessionReviewDenyChecker{})
	proposal, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo policy"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-adapter-policy",
		RequestedBy: "integration_test",
	})
	if err != nil || proposal.Proposal == nil || proposal.Proposal.Status != toolgateway.StatusProposed {
		t.Fatalf("shell proposal setup failed: %#v err=%v", proposal, err)
	}
	if _, _, err := gateway.ToolRuns().ApproveForSession(ctx, proposal.Proposal.ID, run.SessionID,
		"should be denied", "integration_operator", "policy-recheck-grant"); err == nil {
		t.Fatal("expected current policy to reject session approval")
	}
	grants, err := st.ListSessionGrants(ctx, approval.GrantListFilter{
		RunID: run.ID, SessionID: run.SessionID, Status: approval.GrantActive,
	})
	if err != nil || len(grants) != 0 {
		t.Fatalf("policy denial left an active grant: %#v err=%v", grants, err)
	}
	ledger, err := st.GetApprovalByProposal(ctx, proposal.Proposal.ID)
	if err != nil || ledger.Status != approval.StatusPending || ledger.GrantID != "" {
		t.Fatalf("policy denial changed the pending approval: %#v err=%v", ledger, err)
	}
}
