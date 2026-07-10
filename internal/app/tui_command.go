package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolrun"
	"cyberagent-workbench/internal/tui"
	"cyberagent-workbench/internal/workspace"
)

func (a *App) tuiCommand(ctx context.Context, args []string) error {
	if err := a.ensureStore(); err != nil {
		return err
	}
	fs := newFlagSet("tui", a.errOut)
	sessionID := fs.String("session", "", "session id")
	workspaceName := fs.String("workspace", "", "workspace name")
	title := fs.String("title", "TUI session", "session title")
	route := fs.String("route", "learn", "model route")
	printOnly := fs.Bool("print", false, "print one TUI snapshot and exit")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"session":   true,
		"workspace": true,
		"title":     true,
		"route":     true,
		"print":     false,
	})); err != nil {
		return err
	}

	sessionManager := session.NewManager(a.store, a.router, a.checker)
	toolManager := toolrun.NewManager(a.store, a.checker)
	workspaceID := ""
	if strings.TrimSpace(*workspaceName) != "" {
		mgr := workspace.NewManager(a.home, a.store)
		rec, err := mgr.Ensure(ctx, workspace.Slug(*workspaceName))
		if err != nil {
			return err
		}
		workspaceID = rec.ID
	}

	if strings.TrimSpace(*sessionID) != "" {
		sess, err := a.store.GetSession(ctx, strings.TrimSpace(*sessionID))
		if err != nil {
			return err
		}
		model, err := tui.NewModel(ctx, sess, sessionManager, toolManager, a.store)
		if err != nil {
			return err
		}
		if *printOnly {
			fmt.Fprintln(a.out, model.Snapshot())
			return nil
		}
		if !isInteractive() {
			return errors.New("tui requires an interactive terminal; use --print for a snapshot")
		}
		return tui.Run(model)
	}

	if *printOnly {
		sess, err := sessionManager.Create(ctx, workspaceID, *title, *route)
		if err != nil {
			return err
		}
		model, err := tui.NewModel(ctx, sess, sessionManager, toolManager, a.store)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, model.Snapshot())
		return nil
	}

	picker, err := tui.NewPicker(ctx, sessionManager, toolManager, workspaceID, *title, *route, a.store)
	if err != nil {
		return err
	}
	if !isInteractive() {
		fmt.Fprintln(a.out, picker.Snapshot())
		return nil
	}
	return tui.RunPicker(picker)
}

func isInteractive() bool {
	if os.Getenv("CI") != "" {
		return false
	}
	return true
}
