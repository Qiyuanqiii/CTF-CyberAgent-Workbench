package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/workspace"
)

func (a *App) sessionCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("session subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	manager := a.newSessionManager()
	switch args[0] {
	case "create":
		return a.sessionCreate(ctx, manager, args[1:])
	case "list":
		return a.sessionList(ctx, manager)
	case "send":
		return a.sessionSend(ctx, manager, args[1:])
	case "history":
		return a.sessionHistory(ctx, manager, args[1:])
	default:
		return fmt.Errorf("unknown session subcommand %q", args[0])
	}
}

func (a *App) newSessionManager() *session.Manager {
	executor := application.NewSessionRunChatExecutor(a.store, a.router, a.checker).WithActiveCalls(a.calls)
	return session.NewManager(a.store, a.router, a.checker).WithRunChatExecutor(executor)
}

func (a *App) sessionCreate(ctx context.Context, manager *session.Manager, args []string) error {
	fs := newFlagSet("session create", a.errOut)
	workspaceName := fs.String("workspace", "", "workspace name")
	title := fs.String("title", "New session", "session title")
	route := fs.String("route", "learn", "model route")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"workspace": true, "title": true, "route": true})); err != nil {
		return err
	}
	workspaceID := ""
	if strings.TrimSpace(*workspaceName) != "" {
		rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(*workspaceName))
		if err != nil {
			return err
		}
		workspaceID = rec.ID
	}
	if fs.NArg() > 0 && *title == "New session" {
		*title = strings.Join(fs.Args(), " ")
	}
	sess, err := manager.Create(ctx, workspaceID, *title, *route)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "session %s created\nroute: %s\nworkspace: %s\ntitle: %s\n", sess.ID, sess.Route, sess.WorkspaceID, sess.Title)
	return nil
}

func (a *App) sessionList(ctx context.Context, manager *session.Manager) error {
	sessions, err := manager.List(ctx)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Fprintln(a.out, "no sessions")
		return nil
	}
	for _, sess := range sessions {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\n", sess.ID, sess.Route, sess.Status, sess.Title)
	}
	return nil
}

func (a *App) sessionSend(ctx context.Context, manager *session.Manager, args []string) error {
	fs := newFlagSet("session send", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New(`usage: cyberagent session send <session-id> "message"`)
	}
	result, err := manager.Send(ctx, fs.Arg(0), strings.Join(fs.Args()[1:], " "))
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, result.Text)
	if result.RunID != "" {
		fmt.Fprintf(a.out, "\n[run %s: action=%s status=%s]\n", result.RunID, result.RunAction, result.RunStatus)
	}
	if result.Compacted {
		fmt.Fprintf(a.out, "\n[context compacted: summary=%d]\n", result.SummaryID)
	}
	return nil
}

func (a *App) sessionHistory(ctx context.Context, manager *session.Manager, args []string) error {
	fs := newFlagSet("session history", a.errOut)
	all := fs.Bool("all", false, "include compacted messages")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"all": false})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent session history <session-id> [--all]")
	}
	messages, err := manager.History(ctx, fs.Arg(0), *all)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		fmt.Fprintln(a.out, "no messages")
		return nil
	}
	for _, msg := range messages {
		marker := ""
		if msg.Compacted {
			marker = " compacted"
		}
		sourceRef := ""
		if msg.Provenance.SourceRef != "" {
			sourceRef = " ref=" + msg.Provenance.SourceRef
		}
		fmt.Fprintf(a.out, "#%d%s %s [%s authorized=%t%s]: %s\n", msg.ID, marker, msg.Role,
			msg.Provenance.SourceKind, msg.Provenance.InstructionAuthorized, sourceRef, msg.Content)
	}
	return nil
}
