package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/toolgateway"
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
	case "grant":
		return a.approvalGrantCommand(ctx, args[1:])
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
	fmt.Fprintf(out, "grant: %s\n", record.GrantID)
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

func (a *App) approvalGrantCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("approval grant subcommand is required")
	}
	switch args[0] {
	case "create":
		return a.approvalGrantCreate(ctx, args[1:])
	case "list":
		return a.approvalGrantList(ctx, args[1:])
	case "show":
		return a.approvalGrantShow(ctx, args[1:])
	case "revoke":
		return a.approvalGrantRevoke(ctx, args[1:])
	default:
		return fmt.Errorf("unknown approval grant subcommand %q", args[0])
	}
}

func (a *App) approvalGrantCreate(ctx context.Context, args []string) error {
	fs := newFlagSet("approval grant create", a.errOut)
	sessionID := fs.String("session", "", "Run-bound session id")
	workspaceID := fs.String("workspace", "", "workspace id; defaults to the Run workspace")
	toolName := fs.String("tool", "", "shell or replace_file")
	reason := fs.String("reason", "", "grant reason")
	grantedBy := fs.String("by", "cli_operator", "grantor identity")
	operationKey := fs.String("idempotency-key", "", "stable retry key")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"session": true, "workspace": true, "tool": true, "reason": true, "by": true, "idempotency-key": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 0 || strings.TrimSpace(*sessionID) == "" || strings.TrimSpace(*toolName) == "" {
		return errors.New("usage: cyberagent approval grant create --session <id> --tool shell|replace_file [--reason <text>]")
	}
	tool := toolgateway.ToolName(strings.TrimSpace(*toolName))
	class, ok := toolgateway.ClassForTool(tool)
	if !ok || (tool != toolgateway.ShellTool && tool != toolgateway.ReplaceFileTool) {
		return fmt.Errorf("tool %q does not support session grants", tool)
	}
	key := strings.TrimSpace(*operationKey)
	if key == "" {
		key = idgen.New("grantop")
	}
	result, err := a.store.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: *sessionID, WorkspaceID: *workspaceID, ToolName: string(tool), ActionClass: string(class),
		Reason: *reason, GrantedBy: *grantedBy, IdempotencyKey: key,
	})
	if err != nil {
		return err
	}
	verb := "created"
	if result.Replayed {
		verb = "reused"
	}
	fmt.Fprintf(a.out, "approval grant %s %s\n", result.Grant.ID, verb)
	printSessionGrant(a.out, result.Grant)
	return nil
}

func (a *App) approvalGrantList(ctx context.Context, args []string) error {
	fs := newFlagSet("approval grant list", a.errOut)
	runID := fs.String("run", "", "run id")
	sessionID := fs.String("session", "", "session id")
	toolName := fs.String("tool", "", "tool name")
	status := fs.String("status", "", "active or revoked")
	limit := fs.Int("limit", 100, "maximum records")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"run": true, "session": true, "tool": true, "status": true, "limit": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: cyberagent approval grant list [--run <id>] [--session <id>] [--status active|revoked]")
	}
	grants, err := a.store.ListSessionGrants(ctx, approval.GrantListFilter{
		RunID: *runID, SessionID: *sessionID, ToolName: *toolName,
		Status: approval.GrantStatus(strings.TrimSpace(*status)), Limit: *limit,
	})
	if err != nil {
		return err
	}
	if len(grants) == 0 {
		fmt.Fprintln(a.out, "no approval grants")
		return nil
	}
	for _, grant := range grants {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\n", grant.ID, grant.Status, grant.ToolName, grant.SessionID, grant.RunID)
	}
	return nil
}

func (a *App) approvalGrantShow(ctx context.Context, args []string) error {
	fs := newFlagSet("approval grant show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent approval grant show <grant-id>")
	}
	grant, err := a.store.GetSessionGrant(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	printSessionGrant(a.out, grant)
	return nil
}

func (a *App) approvalGrantRevoke(ctx context.Context, args []string) error {
	fs := newFlagSet("approval grant revoke", a.errOut)
	reason := fs.String("reason", "", "revocation reason")
	revokedBy := fs.String("by", "cli_operator", "revoker identity")
	operationKey := fs.String("idempotency-key", "", "stable retry key")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"reason": true, "by": true, "idempotency-key": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent approval grant revoke <grant-id> [--reason <text>]")
	}
	key := strings.TrimSpace(*operationKey)
	if key == "" {
		key = idgen.New("grantop")
	}
	result, err := a.store.RevokeSessionGrant(ctx, approval.RevokeGrantRequest{
		GrantID: fs.Arg(0), Reason: *reason, RevokedBy: *revokedBy, IdempotencyKey: key,
	})
	if err != nil {
		return err
	}
	verb := "revoked"
	if result.Replayed {
		verb = "already revoked"
	}
	fmt.Fprintf(a.out, "approval grant %s %s\n", result.Grant.ID, verb)
	printSessionGrant(a.out, result.Grant)
	return nil
}

func printSessionGrant(out io.Writer, grant approval.SessionGrant) {
	fmt.Fprintf(out, "id: %s\n", grant.ID)
	fmt.Fprintf(out, "status: %s\n", grant.Status)
	fmt.Fprintf(out, "run: %s\n", grant.RunID)
	fmt.Fprintf(out, "session: %s\n", grant.SessionID)
	fmt.Fprintf(out, "workspace: %s\n", grant.WorkspaceID)
	fmt.Fprintf(out, "tool: %s\n", grant.ToolName)
	fmt.Fprintf(out, "action_class: %s\n", grant.ActionClass)
	fmt.Fprintf(out, "request_fingerprint: %s\n", grant.RequestFingerprint)
	fmt.Fprintf(out, "granted_by: %s\n", grant.GrantedBy)
	if grant.Reason != "" {
		fmt.Fprintf(out, "reason: %s\n", grant.Reason)
	}
	if grant.RevokedBy != "" {
		fmt.Fprintf(out, "revoked_by: %s\n", grant.RevokedBy)
	}
	if grant.RevocationReason != "" {
		fmt.Fprintf(out, "revocation_reason: %s\n", grant.RevocationReason)
	}
	fmt.Fprintf(out, "version: %d\n", grant.Version)
	fmt.Fprintf(out, "created_at: %s\n", grant.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(out, "updated_at: %s\n", grant.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	if grant.RevokedAt != nil {
		fmt.Fprintf(out, "revoked_at: %s\n", grant.RevokedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
}
