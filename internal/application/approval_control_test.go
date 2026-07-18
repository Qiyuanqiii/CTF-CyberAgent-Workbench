package application_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/toolrun"
	"cyberagent-workbench/internal/tools"
)

type approvalRecheckDenyChecker struct{}

func (approvalRecheckDenyChecker) CheckText(string, string) policy.Decision {
	return policy.Decision{Allowed: true, Reason: "proposal fixture allowed"}
}

func (approvalRecheckDenyChecker) CheckToolCall(tools.Call) policy.Decision {
	return policy.Decision{Allowed: false, Risk: "high", Reason: "denied during approval recheck"}
}

func TestApprovalControlApprovesOnlyDryRunAndReplaysDurably(t *testing.T) {
	st, run, gateway := prepareApprovalControlFixture(t)
	outcome, err := gateway.Invoke(t.Context(), toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo bounded approval"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "workspace-approval-control",
		RequestedBy: "approval_control_test",
	})
	if err != nil || outcome.Proposal == nil {
		t.Fatalf("Shell proposal=%#v err=%v", outcome, err)
	}
	record, err := st.GetApprovalByProposal(t.Context(), outcome.Proposal.ID)
	if err != nil || record.Status != approval.StatusPending {
		t.Fatalf("pending approval=%#v err=%v", record, err)
	}
	service := application.NewApprovalControlService(st, gateway, policy.NewDefaultChecker())
	request := application.DecideApprovalControlRequest{
		Version: application.ApprovalControlProtocolVersion, RunID: run.ID,
		ApprovalID: record.ID, Action: application.ApprovalControlApproveOnce,
		OperationKey: "approval-control-approve-0001", ReviewedBy: "desktop_operator",
	}
	result, err := service.Decide(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := st.GetToolRun(t.Context(), record.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.Approval.Status != approval.StatusApproved ||
		result.Approval.GrantID != "" || tool.Status != toolrun.StatusCompleted ||
		tool.Stdout != "dry run: echo bounded approval" {
		t.Fatalf("approval widened authority or omitted dry run: result=%#v tool=%#v", result, tool)
	}
	replay, err := service.Decide(t.Context(), request)
	if err != nil || !replay.Replayed || replay.Approval.ID != result.Approval.ID ||
		replay.Approval.GrantID != "" {
		t.Fatalf("approval replay=%#v err=%v", replay, err)
	}
	if _, err := application.NewRunService(st).Complete(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	terminalReplay, err := service.Decide(t.Context(), request)
	if err != nil || !terminalReplay.Replayed ||
		terminalReplay.Approval.ID != result.Approval.ID {
		t.Fatalf("terminal approval replay=%#v err=%v", terminalReplay, err)
	}
}

func TestApprovalControlRechecksPolicyAndCannotOverridePermanentDenial(t *testing.T) {
	st, run, gateway := prepareApprovalControlFixture(t)
	pending, err := gateway.Invoke(t.Context(), toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo policy changes"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "workspace-approval-control",
		RequestedBy: "approval_control_test",
	})
	if err != nil || pending.Proposal == nil {
		t.Fatalf("pending proposal=%#v err=%v", pending, err)
	}
	record, err := st.GetApprovalByProposal(t.Context(), pending.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewApprovalControlService(st, gateway, approvalRecheckDenyChecker{})
	_, err = service.Decide(t.Context(), application.DecideApprovalControlRequest{
		Version: application.ApprovalControlProtocolVersion, RunID: run.ID,
		ApprovalID: record.ID, Action: application.ApprovalControlApproveOnce,
		OperationKey: "approval-control-policy-recheck-0001", ReviewedBy: "desktop_operator",
	})
	if apperror.CodeOf(err) != apperror.CodePolicyDenied {
		t.Fatalf("Policy recheck code=%s err=%v", apperror.CodeOf(err), err)
	}
	unchanged, err := st.GetApproval(t.Context(), record.ID)
	if err != nil || unchanged.Status != approval.StatusPending {
		t.Fatalf("Policy recheck mutated approval=%#v err=%v", unchanged, err)
	}

	permanent, err := gateway.Invoke(t.Context(), toolgateway.ToolCall{
		Name:      toolgateway.ShellTool,
		Arguments: map[string]string{"command": "masscan 0.0.0.0/0"},
		RunID:     run.ID, SessionID: run.SessionID, WorkspaceID: "workspace-approval-control",
		RequestedBy: "approval_control_test",
	})
	if err != nil || permanent.Proposal == nil {
		t.Fatalf("permanent denial proposal=%#v err=%v", permanent, err)
	}
	denied, err := st.GetApprovalByProposal(t.Context(), permanent.Proposal.ID)
	if err != nil || denied.Status != approval.StatusDenied || denied.Mode != "never" {
		t.Fatalf("permanent approval=%#v err=%v", denied, err)
	}
	_, err = application.NewApprovalControlService(st, gateway, policy.NewDefaultChecker()).Decide(
		t.Context(), application.DecideApprovalControlRequest{
			Version: application.ApprovalControlProtocolVersion, RunID: run.ID,
			ApprovalID: denied.ID, Action: application.ApprovalControlApproveOnce,
			OperationKey: "approval-control-override-never-0001", ReviewedBy: "desktop_operator",
		})
	if apperror.CodeOf(err) != apperror.CodePolicyDenied {
		t.Fatalf("permanent denial override code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func prepareApprovalControlFixture(t *testing.T) (*store.SQLiteStore, domain.Run, *toolgateway.Gateway) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "approval-control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := store.WorkspaceRecord{ID: "workspace-approval-control", Name: "approval-control",
		RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "review a bounded approval", Profile: "code", WorkspaceID: workspace.ID,
		Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = application.NewRunService(st).Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checker := policy.NewDefaultChecker()
	return st, run, toolgateway.New(st, checker)
}
