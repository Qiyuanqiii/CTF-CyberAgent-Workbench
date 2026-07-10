package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/toolrun"
)

type toolRunManager interface {
	Get(context.Context, string) (toolrun.ToolRun, error)
	List(context.Context, toolrun.ListFilter) ([]toolrun.ToolRun, error)
	Approve(context.Context, string) (toolrun.ToolRun, error)
	Deny(context.Context, string, string) (toolrun.ToolRun, error)
}

func (a *App) toolCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("tool subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	manager := a.newToolGateway().ToolRuns()
	switch args[0] {
	case "list":
		return a.toolList(ctx, manager, args[1:])
	case "show":
		return a.toolShow(ctx, manager, args[1:])
	case "approve":
		return a.toolApprove(ctx, manager, args[1:])
	case "deny":
		return a.toolDeny(ctx, manager, args[1:])
	default:
		return fmt.Errorf("unknown tool subcommand %q", args[0])
	}
}

func (a *App) toolList(ctx context.Context, manager toolRunManager, args []string) error {
	fs := newFlagSet("tool list", a.errOut)
	sessionID := fs.String("session", "", "session id")
	status := fs.String("status", "", "tool run status")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"session": true, "status": true})); err != nil {
		return err
	}
	runs, err := manager.List(ctx, toolrun.ListFilter{SessionID: *sessionID, Status: *status})
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Fprintln(a.out, "no tool runs")
		return nil
	}
	for _, run := range runs {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\n", run.ID, run.Status, run.ToolName, run.Command)
	}
	return nil
}

func (a *App) toolShow(ctx context.Context, manager toolRunManager, args []string) error {
	fs := newFlagSet("tool show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool show <tool-run-id>")
	}
	run, err := manager.Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	printToolRun(a.out, run)
	return nil
}

func (a *App) toolApprove(ctx context.Context, manager toolRunManager, args []string) error {
	fs := newFlagSet("tool approve", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool approve <tool-run-id>")
	}
	run, err := manager.Approve(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "tool run %s %s\n", run.ID, run.Status)
	if strings.TrimSpace(run.Stdout) != "" {
		fmt.Fprintln(a.out, run.Stdout)
	}
	if strings.TrimSpace(run.Stderr) != "" {
		fmt.Fprintln(a.errOut, run.Stderr)
	}
	return nil
}

func (a *App) toolDeny(ctx context.Context, manager toolRunManager, args []string) error {
	fs := newFlagSet("tool deny", a.errOut)
	reason := fs.String("reason", "", "denial reason")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"reason": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool deny <tool-run-id> [--reason <reason>]")
	}
	run, err := manager.Deny(ctx, fs.Arg(0), *reason)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "tool run %s %s\n", run.ID, run.Status)
	if strings.TrimSpace(run.PolicyReason) != "" {
		fmt.Fprintf(a.out, "reason: %s\n", run.PolicyReason)
	}
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
	fmt.Fprintf(out, "created_at: %s\n", run.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(out, "updated_at: %s\n", run.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}
