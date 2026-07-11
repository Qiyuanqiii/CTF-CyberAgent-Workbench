package toolgateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/toolrun"
	"cyberagent-workbench/internal/tools"
)

type ToolRunAdapter struct {
	gateway *Gateway
}

func (g *Gateway) ToolRuns() *ToolRunAdapter {
	return &ToolRunAdapter{gateway: g}
}

func (a *ToolRunAdapter) ProposeShell(ctx context.Context, sessionID string, workspaceID string, command string) (toolrun.ToolRun, error) {
	if a == nil || a.gateway == nil {
		return toolrun.ToolRun{}, errors.New("tool gateway is required")
	}
	outcome, err := a.gateway.Invoke(ctx, ToolCall{
		Name: ShellTool, Arguments: map[string]string{"command": command},
		SessionID: sessionID, WorkspaceID: workspaceID, RequestedBy: "compat_toolrun_adapter",
	})
	if err != nil {
		return toolrun.ToolRun{}, err
	}
	if outcome.Proposal == nil {
		return toolrun.ToolRun{}, fmt.Errorf("shell proposal was not created: %s", outcome.Decision.Reason)
	}
	return a.Get(ctx, outcome.Proposal.ID)
}

func (a *ToolRunAdapter) Approve(ctx context.Context, id string) (toolrun.ToolRun, error) {
	if a == nil || a.gateway == nil {
		return toolrun.ToolRun{}, errors.New("tool gateway is required")
	}
	_, reviewErr := a.gateway.Review(ctx, ReviewRequest{Action: ReviewApprove, Tool: ShellTool, ProposalID: id})
	run, getErr := a.Get(ctx, id)
	return run, errors.Join(reviewErr, getErr)
}

func (a *ToolRunAdapter) ApproveForSession(ctx context.Context, id string, expectedSessionID string,
	reason string, grantedBy string, idempotencyKey string,
) (toolrun.ToolRun, approval.SessionGrant, error) {
	if a == nil || a.gateway == nil || a.gateway.grantStore == nil || a.gateway.legacyTools == nil {
		return toolrun.ToolRun{}, approval.SessionGrant{}, errors.New("tool gateway is required")
	}
	expectedSessionID = strings.TrimSpace(expectedSessionID)
	if expectedSessionID == "" {
		return toolrun.ToolRun{}, approval.SessionGrant{}, errors.New("expected session id is required")
	}
	before, err := a.Get(ctx, id)
	if err != nil {
		return toolrun.ToolRun{}, approval.SessionGrant{}, err
	}
	if before.Status != toolrun.StatusProposed {
		return before, approval.SessionGrant{},
			fmt.Errorf("tool run %s is %s, not %s", before.ID, before.Status, toolrun.StatusProposed)
	}
	if before.ToolName != toolrun.ShellTool || before.SessionID == "" {
		return before, approval.SessionGrant{}, errors.New("session approval requires a Session-bound Shell proposal")
	}
	if before.SessionID != expectedSessionID {
		return before, approval.SessionGrant{}, errors.New("session approval proposal does not belong to the expected Session")
	}
	record, err := a.gateway.store.GetApprovalByProposal(ctx, before.ID)
	if err != nil {
		return before, approval.SessionGrant{}, err
	}
	if record.RunID == "" || record.SessionID != before.SessionID || record.WorkspaceID != before.WorkspaceID ||
		record.ToolName != string(ShellTool) || record.ActionClass != string(ClassShell) ||
		record.RequestFingerprint != approval.ShellFingerprint(before.SessionID, before.WorkspaceID, before.Command) {
		return before, approval.SessionGrant{}, errors.New("session approval proposal does not match its durable approval scope")
	}
	policyDecision := a.gateway.checker.CheckToolCall(tools.Call{
		Name: string(ShellTool), Args: map[string]string{"command": before.Command},
	})
	if !policyDecision.Allowed {
		return before, approval.SessionGrant{}, fmt.Errorf("session approval denied by policy: %s", policyDecision.Reason)
	}
	if record.Status == approval.StatusDenied {
		return before, approval.SessionGrant{}, errors.New("session approval proposal was already denied")
	}
	if record.Status == approval.StatusApproved {
		if record.GrantID == "" {
			return before, approval.SessionGrant{}, errors.New("session approval proposal was approved without a session grant")
		}
		grant, err := a.gateway.grantStore.GetSessionGrant(ctx, record.GrantID)
		if err != nil {
			return before, approval.SessionGrant{}, err
		}
		return a.finishSessionApprovedToolRun(ctx, before, record, grant)
	}
	grantResult, err := a.gateway.grantStore.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: before.SessionID, WorkspaceID: before.WorkspaceID, ToolName: string(ShellTool),
		ActionClass: string(ClassShell), Reason: reason, GrantedBy: grantedBy, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return before, approval.SessionGrant{}, err
	}
	grant := grantResult.Grant
	if _, err := a.gateway.grantStore.AuthorizeApprovalWithSessionGrant(ctx, before.ID, grant.ID); err != nil {
		return before, grant, err
	}
	record, err = a.gateway.store.GetApprovalByProposal(ctx, before.ID)
	if err != nil {
		return before, grant, err
	}
	return a.finishSessionApprovedToolRun(ctx, before, record, grant)
}

func (a *ToolRunAdapter) finishSessionApprovedToolRun(ctx context.Context, before toolrun.ToolRun,
	record approval.Record, grant approval.SessionGrant,
) (toolrun.ToolRun, approval.SessionGrant, error) {
	if grant.Status != approval.GrantActive || grant.ID != record.GrantID || grant.RunID != record.RunID ||
		grant.SessionID != record.SessionID || grant.WorkspaceID != record.WorkspaceID ||
		grant.ToolName != record.ToolName || grant.ActionClass != record.ActionClass {
		return before, grant, errors.New("session grant no longer authorizes this approval scope")
	}
	after, approveErr := a.gateway.legacyTools.Approve(ctx, before.ID)
	if after.ID == "" {
		after = before
	}
	call := ToolCall{
		Name: ShellTool, Arguments: map[string]string{"command": before.Command}, RunID: record.RunID,
		SessionID: before.SessionID, WorkspaceID: before.WorkspaceID, RequestedBy: "session_grant",
	}
	_, projectionErr := a.gateway.outcomeFromToolRun(ctx, call, after, approveErr)
	return after, grant, projectionErr
}

func (a *ToolRunAdapter) Deny(ctx context.Context, id string, reason string) (toolrun.ToolRun, error) {
	if a == nil || a.gateway == nil {
		return toolrun.ToolRun{}, errors.New("tool gateway is required")
	}
	_, reviewErr := a.gateway.Review(ctx, ReviewRequest{
		Action: ReviewDeny, Tool: ShellTool, ProposalID: id, Reason: reason,
	})
	run, getErr := a.Get(ctx, id)
	return run, errors.Join(reviewErr, getErr)
}

func (a *ToolRunAdapter) Get(ctx context.Context, id string) (toolrun.ToolRun, error) {
	if a == nil || a.gateway == nil || a.gateway.legacyTools == nil {
		return toolrun.ToolRun{}, errors.New("tool gateway is required")
	}
	return a.gateway.legacyTools.Get(ctx, id)
}

func (a *ToolRunAdapter) List(ctx context.Context, filter toolrun.ListFilter) ([]toolrun.ToolRun, error) {
	if a == nil || a.gateway == nil || a.gateway.legacyTools == nil {
		return nil, errors.New("tool gateway is required")
	}
	return a.gateway.legacyTools.List(ctx, filter)
}

type FileEditAdapter struct {
	gateway *Gateway
}

func (g *Gateway) FileEdits() *FileEditAdapter {
	return &FileEditAdapter{gateway: g}
}

func (a *FileEditAdapter) Propose(ctx context.Context, proposal fileedit.Proposal) (fileedit.Edit, error) {
	if a == nil || a.gateway == nil {
		return fileedit.Edit{}, errors.New("tool gateway is required")
	}
	outcome, err := a.gateway.Invoke(ctx, ToolCall{
		Name: ReplaceFileTool, Arguments: map[string]string{"path": proposal.Path, "content": proposal.ProposedText},
		SessionID: proposal.SessionID, WorkspaceID: proposal.WorkspaceID, WorkspaceRoot: proposal.WorkspaceRoot,
		RequestedBy: "compat_fileedit_adapter",
	})
	if err != nil {
		return fileedit.Edit{}, err
	}
	if outcome.Proposal == nil {
		return fileedit.Edit{}, fmt.Errorf("policy denied file edit proposal: %s", outcome.Decision.Reason)
	}
	return a.Get(ctx, outcome.Proposal.ID)
}

func (a *FileEditAdapter) Approve(ctx context.Context, id string, workspaceRoot string) (fileedit.Edit, error) {
	if a == nil || a.gateway == nil {
		return fileedit.Edit{}, errors.New("tool gateway is required")
	}
	_, reviewErr := a.gateway.Review(ctx, ReviewRequest{
		Action: ReviewApprove, Tool: ReplaceFileTool, ProposalID: id, WorkspaceRoot: workspaceRoot,
	})
	edit, getErr := a.Get(ctx, id)
	return edit, errors.Join(reviewErr, getErr)
}

func (a *FileEditAdapter) Deny(ctx context.Context, id string, reason string) (fileedit.Edit, error) {
	if a == nil || a.gateway == nil {
		return fileedit.Edit{}, errors.New("tool gateway is required")
	}
	_, reviewErr := a.gateway.Review(ctx, ReviewRequest{
		Action: ReviewDeny, Tool: ReplaceFileTool, ProposalID: id, Reason: reason,
	})
	edit, getErr := a.Get(ctx, id)
	return edit, errors.Join(reviewErr, getErr)
}

func (a *FileEditAdapter) Get(ctx context.Context, id string) (fileedit.Edit, error) {
	if a == nil || a.gateway == nil || a.gateway.legacyEdits == nil {
		return fileedit.Edit{}, errors.New("tool gateway is required")
	}
	return a.gateway.legacyEdits.Get(ctx, id)
}

func (a *FileEditAdapter) List(ctx context.Context, filter fileedit.ListFilter) ([]fileedit.Edit, error) {
	if a == nil || a.gateway == nil || a.gateway.legacyEdits == nil {
		return nil, errors.New("tool gateway is required")
	}
	return a.gateway.legacyEdits.List(ctx, filter)
}
