package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"cyberagent-workbench/internal/application"
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

	sessionManager := a.newSessionManager()
	activeCalls := &tuiActiveCallController{supervisor: a.newRunSupervisor()}
	toolManager := a.newToolGateway().ToolRuns()
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
		model.WithActiveCallController(activeCalls)
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
		model.WithActiveCallController(activeCalls)
		fmt.Fprintln(a.out, model.Snapshot())
		return nil
	}

	picker, err := tui.NewPicker(ctx, sessionManager, toolManager, workspaceID, *title, *route, a.store)
	if err != nil {
		return err
	}
	picker.WithActiveCallController(activeCalls)
	if !isInteractive() {
		fmt.Fprintln(a.out, picker.Snapshot())
		return nil
	}
	return tui.RunPicker(picker)
}

type tuiActiveCallController struct {
	supervisor *application.RunSupervisor
}

func (c *tuiActiveCallController) ActiveCallForSession(sessionID string) (application.ActiveCallInfo, bool) {
	if c == nil || c.supervisor == nil {
		return application.ActiveCallInfo{}, false
	}
	return c.supervisor.ActiveCallForSession(sessionID)
}

func (c *tuiActiveCallController) SubscribeActiveCall(runID string) (tui.ActiveCallSubscription, error) {
	if c == nil || c.supervisor == nil {
		return nil, errors.New("active call controller is unavailable")
	}
	return c.supervisor.SubscribeActiveCall(runID)
}

func (c *tuiActiveCallController) CancelActiveCall(ctx context.Context, request application.ActiveCallCancelRequest) (application.ActiveCallCancelResult, error) {
	if c == nil || c.supervisor == nil {
		return application.ActiveCallCancelResult{}, errors.New("active call controller is unavailable")
	}
	return c.supervisor.CancelActiveCall(ctx, request)
}

func isInteractive() bool {
	return os.Getenv("CI") == ""
}
