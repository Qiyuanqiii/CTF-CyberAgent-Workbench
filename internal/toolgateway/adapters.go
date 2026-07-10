package toolgateway

import (
	"context"
	"errors"
	"fmt"

	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/toolrun"
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
