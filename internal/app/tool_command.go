package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cyberagent-workbench/internal/scriptprocess"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/toolrun"
)

type toolListRow struct {
	id        string
	status    string
	tool      string
	preview   string
	updatedAt time.Time
}

func (a *App) toolCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("tool subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	switch args[0] {
	case "list":
		return a.toolList(ctx, args[1:])
	case "show":
		return a.toolShow(ctx, args[1:])
	case "approve":
		return a.toolApprove(ctx, args[1:])
	case "deny":
		return a.toolDeny(ctx, args[1:])
	default:
		return fmt.Errorf("unknown tool subcommand %q", args[0])
	}
}

func (a *App) toolList(ctx context.Context, args []string) error {
	fs := newFlagSet("tool list", a.errOut)
	sessionID := fs.String("session", "", "session id")
	status := fs.String("status", "", "proposal status")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"session": true, "status": true})); err != nil {
		return err
	}
	if !validToolProposalStatus(strings.TrimSpace(*status)) {
		return fmt.Errorf("invalid tool proposal status %q", strings.TrimSpace(*status))
	}
	gateway := a.newToolGateway()
	runs, err := gateway.ToolRuns().List(ctx, toolrun.ListFilter{SessionID: *sessionID, Status: *status})
	if err != nil {
		return err
	}
	rows := make([]toolListRow, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, toolListRow{
			id: run.ID, status: run.Status, tool: run.ToolName, preview: run.Command, updatedAt: run.UpdatedAt,
		})
	}
	processStatus := scriptprocess.Status(strings.TrimSpace(*status))
	if processStatus == "" || processStatus.Valid() {
		processes, err := gateway.ScriptProcesses().List(ctx, scriptprocess.ListFilter{
			SessionID: *sessionID, Status: processStatus,
		})
		if err != nil {
			return err
		}
		for _, process := range processes {
			rows = append(rows, toolListRow{
				id: process.ID, status: string(process.Status), tool: string(toolgateway.ScriptProcessTool),
				preview: scriptProcessPreview(process), updatedAt: process.UpdatedAt,
			})
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(a.out, "no tool proposals")
		return nil
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].updatedAt.Equal(rows[j].updatedAt) {
			return rows[i].id > rows[j].id
		}
		return rows[i].updatedAt.After(rows[j].updatedAt)
	})
	for _, row := range rows {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\n", row.id, row.status, row.tool, row.preview)
	}
	return nil
}

func validToolProposalStatus(status string) bool {
	switch status {
	case "", toolrun.StatusProposed, toolrun.StatusApproved, toolrun.StatusDenied,
		toolrun.StatusRunning, toolrun.StatusCompleted, toolrun.StatusFailed:
		return true
	default:
		return false
	}
}

func (a *App) toolShow(ctx context.Context, args []string) error {
	fs := newFlagSet("tool show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool show <proposal-id>")
	}
	tool, err := a.proposalTool(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	gateway := a.newToolGateway()
	switch tool {
	case toolgateway.ShellTool:
		run, err := gateway.ToolRuns().Get(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printToolRun(a.out, run)
		return nil
	case toolgateway.ScriptProcessTool:
		process, err := gateway.ScriptProcesses().Get(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		return printScriptProcess(a.out, process)
	case toolgateway.ReplaceFileTool:
		return errors.New("file edit proposals are inspected with `cyberagent edit show`")
	default:
		return fmt.Errorf("unsupported proposal tool %q", tool)
	}
}

func (a *App) toolApprove(ctx context.Context, args []string) error {
	fs := newFlagSet("tool approve", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool approve <proposal-id>")
	}
	return a.reviewToolProposal(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, ProposalID: fs.Arg(0), ReviewedBy: "cli",
	})
}

func (a *App) toolDeny(ctx context.Context, args []string) error {
	fs := newFlagSet("tool deny", a.errOut)
	reason := fs.String("reason", "", "denial reason")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"reason": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool deny <proposal-id> [--reason <reason>]")
	}
	return a.reviewToolProposal(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewDeny, ProposalID: fs.Arg(0), ReviewedBy: "cli", Reason: *reason,
	})
}

func (a *App) reviewToolProposal(ctx context.Context, request toolgateway.ReviewRequest) error {
	tool, err := a.proposalTool(ctx, request.ProposalID)
	if err != nil {
		return err
	}
	if tool == toolgateway.ReplaceFileTool {
		return errors.New("file edit proposals are reviewed with `cyberagent edit approve|deny`")
	}
	request.Tool = tool
	outcome, err := a.newToolGateway().Review(ctx, request)
	if err != nil {
		return err
	}
	if outcome.Proposal == nil {
		return errors.New("tool review did not return a proposal")
	}
	label := "tool run"
	if tool == toolgateway.ScriptProcessTool {
		label = "script process"
	}
	fmt.Fprintf(a.out, "%s %s %s\n", label, outcome.Proposal.ID, outcome.Proposal.Status)
	if outcome.Result != nil && strings.TrimSpace(outcome.Result.Stdout) != "" {
		fmt.Fprintln(a.out, outcome.Result.Stdout)
	}
	if outcome.Result != nil && strings.TrimSpace(outcome.Result.Stderr) != "" {
		fmt.Fprintln(a.errOut, outcome.Result.Stderr)
	}
	if request.Action == toolgateway.ReviewDeny && strings.TrimSpace(outcome.Decision.Reason) != "" {
		fmt.Fprintf(a.out, "reason: %s\n", outcome.Decision.Reason)
	}
	return nil
}

func (a *App) proposalTool(ctx context.Context, proposalID string) (toolgateway.ToolName, error) {
	record, err := a.store.GetApprovalByProposal(ctx, strings.TrimSpace(proposalID))
	if err != nil {
		return "", err
	}
	return toolgateway.ToolName(record.ToolName), nil
}

func scriptProcessPreview(process scriptprocess.Process) string {
	arguments, _ := json.Marshal(process.Arguments)
	return fmt.Sprintf("%s %s", process.Executable, arguments)
}

func printScriptProcess(out interface{ Write([]byte) (int, error) }, process scriptprocess.Process) error {
	arguments, err := json.Marshal(process.Arguments)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "id: %s\n", process.ID)
	fmt.Fprintf(out, "schema: %s\n", scriptprocess.Schema)
	fmt.Fprintf(out, "status: %s\n", process.Status)
	fmt.Fprintf(out, "tool: %s\n", toolgateway.ScriptProcessTool)
	fmt.Fprintf(out, "run: %s\n", process.RunID)
	fmt.Fprintf(out, "session: %s\n", process.SessionID)
	fmt.Fprintf(out, "workspace: %s\n", process.WorkspaceID)
	fmt.Fprintf(out, "executable: %s\n", process.Executable)
	fmt.Fprintf(out, "arguments: %s\n", arguments)
	fmt.Fprintf(out, "working_directory: %s\n", process.WorkingDirectory)
	fmt.Fprintf(out, "requested_backend: %s\n", process.RequestedBackend)
	fmt.Fprintf(out, "execution_mode: %s\n", process.ExecutionMode)
	fmt.Fprintf(out, "risk: %s\n", process.Risk)
	fmt.Fprintf(out, "policy_reason: %s\n", process.PolicyReason)
	if strings.TrimSpace(process.Stdout) != "" {
		fmt.Fprintf(out, "stdout: %s\n", process.Stdout)
	}
	if strings.TrimSpace(process.Stderr) != "" {
		fmt.Fprintf(out, "stderr: %s\n", process.Stderr)
	}
	fmt.Fprintf(out, "exit_code: %d\n", process.ExitCode)
	fmt.Fprintf(out, "created_at: %s\n", process.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "updated_at: %s\n", process.UpdatedAt.Format(time.RFC3339))
	return nil
}

func printToolRun(out interface{ Write([]byte) (int, error) }, run toolrun.ToolRun) {
	fmt.Fprintf(out, "id: %s\n", run.ID)
	fmt.Fprintf(out, "status: %s\n", run.Status)
	fmt.Fprintf(out, "tool: %s\n", run.ToolName)
	fmt.Fprintf(out, "session: %s\n", run.SessionID)
	fmt.Fprintf(out, "workspace: %s\n", run.WorkspaceID)
	fmt.Fprintf(out, "command: %s\n", run.Command)
	if strings.TrimSpace(run.Risk) != "" {
		fmt.Fprintf(out, "risk: %s\n", run.Risk)
	}
	if strings.TrimSpace(run.PolicyReason) != "" {
		fmt.Fprintf(out, "policy_reason: %s\n", run.PolicyReason)
	}
	if strings.TrimSpace(run.Stdout) != "" {
		fmt.Fprintf(out, "stdout: %s\n", run.Stdout)
	}
	if strings.TrimSpace(run.Stderr) != "" {
		fmt.Fprintf(out, "stderr: %s\n", run.Stderr)
	}
	fmt.Fprintf(out, "exit_code: %d\n", run.ExitCode)
	fmt.Fprintf(out, "created_at: %s\n", run.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "updated_at: %s\n", run.UpdatedAt.Format(time.RFC3339))
}
