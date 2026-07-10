package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"cyberagent-workbench/internal/approval"
)

func (a *App) approvalCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("approval subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	switch args[0] {
	case "list":
		return a.approvalList(ctx, args[1:])
	case "show":
		return a.approvalShow(ctx, args[1:])
	default:
		return fmt.Errorf("unknown approval subcommand %q", args[0])
	}
}

func (a *App) approvalList(ctx context.Context, args []string) error {
	fs := newFlagSet("approval list", a.errOut)
	runID := fs.String("run", "", "run id")
	sessionID := fs.String("session", "", "session id")
	status := fs.String("status", "", "pending, approved, or denied")
	toolName := fs.String("tool", "", "tool name")
	limit := fs.Int("limit", 100, "maximum records")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"run": true, "session": true, "status": true, "tool": true, "limit": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: cyberagent approval list [--run <id>] [--session <id>] [--status <status>] [--tool <name>] [--limit <n>]")
	}
	records, err := a.store.ListApprovals(ctx, approval.ListFilter{
		RunID: *runID, SessionID: *sessionID, Status: approval.Status(strings.TrimSpace(*status)),
		ToolName: *toolName, Limit: *limit,
	})
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Fprintln(a.out, "no approvals")
		return nil
	}
	for _, record := range records {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\n", record.ID, record.Status, record.ToolName, record.ProposalID, record.RunID)
	}
	return nil
}

func (a *App) approvalShow(ctx context.Context, args []string) error {
	fs := newFlagSet("approval show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent approval show <approval-id>")
	}
	record, err := a.store.GetApproval(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	printApproval(a.out, record)
	return nil
}

func printApproval(out io.Writer, record approval.Record) {
	fmt.Fprintf(out, "id: %s\n", record.ID)
	fmt.Fprintf(out, "status: %s\n", record.Status)
	fmt.Fprintf(out, "proposal: %s\n", record.ProposalID)
	fmt.Fprintf(out, "run: %s\n", record.RunID)
	fmt.Fprintf(out, "session: %s\n", record.SessionID)
	fmt.Fprintf(out, "workspace: %s\n", record.WorkspaceID)
	fmt.Fprintf(out, "tool: %s\n", record.ToolName)
	fmt.Fprintf(out, "action_class: %s\n", record.ActionClass)
	fmt.Fprintf(out, "mode: %s\n", record.Mode)
	fmt.Fprintf(out, "request_fingerprint: %s\n", record.RequestFingerprint)
	fmt.Fprintf(out, "requested_by: %s\n", record.RequestedBy)
	fmt.Fprintf(out, "reviewed_by: %s\n", record.ReviewedBy)
	if record.DecisionReason != "" {
		fmt.Fprintf(out, "reason: %s\n", record.DecisionReason)
	}
	fmt.Fprintf(out, "version: %d\n", record.Version)
	fmt.Fprintf(out, "created_at: %s\n", record.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(out, "updated_at: %s\n", record.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	if record.DecidedAt != nil {
		fmt.Fprintf(out, "decided_at: %s\n", record.DecidedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
}
