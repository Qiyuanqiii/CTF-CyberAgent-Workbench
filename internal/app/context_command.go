package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/workspace"
)

func (a *App) contextCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("context subcommand is required")
	}
	switch args[0] {
	case "compact":
		return a.contextCompact(ctx, args[1:])
	case "show":
		return a.contextShow(ctx, args[1:])
	default:
		return fmt.Errorf("unknown context subcommand %q", args[0])
	}
}

func (a *App) contextCompact(ctx context.Context, args []string) error {
	if err := a.ensureStore(); err != nil {
		return err
	}
	fs := newFlagSet("context compact", a.errOut)
	workspaceName := fs.String("workspace", "", "workspace name")
	taskID := fs.String("task", "", "task id")
	var rawMessages repeatedString
	fs.Var(&rawMessages, "message", "message as role: content; repeatable")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"workspace": true, "task": true, "message": true})); err != nil {
		return err
	}
	if strings.TrimSpace(*taskID) == "" {
		return errors.New("usage: cyberagent context compact --task <id> --message \"user: ...\" [--workspace <name>]")
	}
	if len(rawMessages) == 0 {
		return errors.New("context compact requires at least one --message")
	}

	workspaceID := ""
	if strings.TrimSpace(*workspaceName) != "" {
		rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(*workspaceName))
		if err != nil {
			return err
		}
		workspaceID = rec.ID
	}

	messages := make([]contextmgr.Message, 0, len(rawMessages))
	for _, raw := range rawMessages {
		messages = append(messages, contextmgr.ParseMessage(raw))
	}
	manager := contextmgr.NewManager(a.store, contextmgr.DefaultConfig())
	result, err := manager.Compact(ctx, *taskID, workspaceID, messages)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "context compacted\nsummary_id: %d\ntask: %s\nworkspace: %s\nsource_messages: %d\npreserved_messages: %d\nremoved_messages: %d\ntoken_estimate: %d\n",
		result.Summary.ID, result.Summary.TaskID, result.Summary.WorkspaceID, result.Summary.SourceMessageCount,
		result.Summary.PreservedMessageCount, result.RemovedMessages, result.Summary.TokenEstimate)
	return nil
}

func (a *App) contextShow(ctx context.Context, args []string) error {
	if err := a.ensureStore(); err != nil {
		return err
	}
	fs := newFlagSet("context show", a.errOut)
	taskID := fs.String("task", "", "task id")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"task": true})); err != nil {
		return err
	}
	if strings.TrimSpace(*taskID) == "" {
		return errors.New("usage: cyberagent context show --task <id>")
	}
	manager := contextmgr.NewManager(a.store, contextmgr.DefaultConfig())
	summary, ok, err := manager.Latest(ctx, *taskID)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(a.out, "no context summary")
		return nil
	}
	fmt.Fprintf(a.out, "summary_id: %d\ntask: %s\nworkspace: %s\nsource_messages: %d\npreserved_messages: %d\ntoken_estimate: %d\ncreated_at: %s\n\n%s",
		summary.ID, summary.TaskID, summary.WorkspaceID, summary.SourceMessageCount,
		summary.PreservedMessageCount, summary.TokenEstimate, summary.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), summary.Content)
	if !strings.HasSuffix(summary.Content, "\n") {
		fmt.Fprintln(a.out)
	}
	return nil
}

type repeatedString []string

func (r *repeatedString) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedString) Set(value string) error {
	*r = append(*r, value)
	return nil
}
