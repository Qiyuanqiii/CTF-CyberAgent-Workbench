package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/workspace"
)

func (a *App) runCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("run subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	service := application.NewRunService(a.store)
	switch args[0] {
	case "create":
		return a.runCreate(ctx, service, args[1:])
	case "adapt-task":
		return a.runAdaptTask(ctx, args[1:])
	case "list":
		return a.runList(ctx, service, args[1:])
	case "show":
		return a.runShow(ctx, service, args[1:])
	case "events":
		return a.runEvents(ctx, service, args[1:])
	case "step":
		return a.runSupervisorStep(ctx, args[1:])
	case "execute":
		return a.runSupervisorExecute(ctx, args[1:])
	case "checkpoint":
		return a.runSupervisorCheckpoint(ctx, args[1:])
	case "finish":
		return a.runSupervisorFinalize(ctx, application.LifecycleOutcomeCompleted, args[1:])
	case "fail":
		return a.runSupervisorFinalize(ctx, application.LifecycleOutcomeFailed, args[1:])
	case "start":
		return a.runTransition(ctx, service, "start", args[1:])
	case "pause":
		return a.runTransition(ctx, service, "pause", args[1:])
	case "resume":
		return a.runTransition(ctx, service, "resume", args[1:])
	case "cancel":
		return a.runTransition(ctx, service, "cancel", args[1:])
	default:
		return fmt.Errorf("unknown run subcommand %q", args[0])
	}
}

func (a *App) runSupervisorStep(ctx context.Context, args []string) error {
	fs := newFlagSet("run step", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run step <run-id>")
	}
	result, err := application.NewRunSupervisor(a.store, a.router, a.checker).Step(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run %s turn %d completed\nattempt: %s\nrecovered: %t\nmodel_attempts: %d\nmodel_outcome: %s\naction: %s\nrun_status: %s\nprovider: %s\nmodel: %s\nusage: input=%d output=%d total=%d\ncumulative_tokens: %d\nexecution_millis: %d\nnext_turn: %d\nresponse: %s\n",
		result.Handle.RunID, result.Turn, result.AttemptID, result.Recovered, result.ModelAttempts, result.ModelOutcome, result.Action.Kind, result.RunStatus,
		result.Provider, result.Model,
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.TotalTokens,
		result.Checkpoint.TotalTokens, result.Checkpoint.ExecutionMillis, result.Checkpoint.NextTurn, result.Text)
	return nil
}

func (a *App) runSupervisorExecute(ctx context.Context, args []string) error {
	fs := newFlagSet("run execute", a.errOut)
	maxSteps := fs.Int("max-steps", 1, "maximum supervised turns in this invocation")
	finish := fs.Bool("finish", false, "finalize the run as completed after the step limit")
	summary := fs.String("summary", "", "completion summary used with --finish")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"max-steps": true, "finish": false, "summary": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 || *maxSteps <= 0 {
		return errors.New("usage: cyberagent run execute <run-id> [--max-steps <n>] [--finish] [--summary <text>]")
	}
	supervisor := application.NewRunSupervisor(a.store, a.router, a.checker)
	result, err := supervisor.Execute(ctx, fs.Arg(0), *maxSteps)
	for _, step := range result.Steps {
		fmt.Fprintf(a.out, "turn %d\t%s\t%s/%s\tattempts=%d\ttokens=%d\tnext=%d\n",
			step.Turn, step.Action.Kind, step.Provider, step.Model, step.ModelAttempts, step.Usage.TotalTokens, step.Checkpoint.NextTurn)
	}
	if err != nil {
		fmt.Fprintf(a.out, "execution stopped: %s\n", result.StopReason)
		return err
	}
	if *finish {
		if result.RunStatus == domain.RunPaused || result.RunStatus == domain.RunWaitingApproval {
			return apperror.New(apperror.CodeFailedPrecondition, "cannot finalize a waiting run with --finish; resume it or use run fail")
		}
		completionSummary := strings.TrimSpace(*summary)
		if completionSummary == "" {
			completionSummary = fmt.Sprintf("operator finalized after %d supervised turn(s)", len(result.Steps))
		}
		finalized, err := supervisor.Finalize(ctx, fs.Arg(0), application.LifecycleOutcomeCompleted, completionSummary)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "run %s finalized: %s\n", finalized.Run.ID, finalized.Run.Status)
		return nil
	}
	fmt.Fprintf(a.out, "execution stopped: %s\nrun_status: %s\n", result.StopReason, result.RunStatus)
	return nil
}

func (a *App) runSupervisorFinalize(ctx context.Context, outcome application.LifecycleOutcome, args []string) error {
	name := "finish"
	flagName := "summary"
	if outcome == application.LifecycleOutcomeFailed {
		name = "fail"
		flagName = "reason"
	}
	fs := newFlagSet("run "+name, a.errOut)
	text := fs.String(flagName, "", flagName+" text")
	if err := fs.Parse(reorderFlags(args, map[string]bool{flagName: true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: cyberagent run %s <run-id> [--%s <text>]", name, flagName)
	}
	result, err := application.NewRunSupervisor(a.store, a.router, a.checker).Finalize(ctx, fs.Arg(0), outcome, *text)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run %s finalized: %s\nphase: %s\nturns_completed: %d\ntotal_tokens: %d\nexecution_millis: %d\n",
		result.Run.ID, result.Run.Status, result.Checkpoint.Phase, result.Checkpoint.NextTurn-1,
		result.Checkpoint.TotalTokens, result.Checkpoint.ExecutionMillis)
	return nil
}

func (a *App) runSupervisorCheckpoint(ctx context.Context, args []string) error {
	fs := newFlagSet("run checkpoint", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run checkpoint <run-id>")
	}
	checkpoint, ok, err := application.NewRunSupervisor(a.store, a.router, a.checker).Checkpoint(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintf(a.out, "run %s has no supervisor checkpoint\n", fs.Arg(0))
		return nil
	}
	fmt.Fprintf(a.out, "run: %s\nphase: %s\nnext_turn: %d\nattempt: %s\nlast_error: %s\ninput_tokens: %d\noutput_tokens: %d\ntotal_tokens: %d\nexecution_millis: %d\nupdated_at: %s\n",
		checkpoint.RunID, checkpoint.Phase, checkpoint.NextTurn, checkpoint.AttemptID,
		checkpoint.LastError, checkpoint.InputTokens, checkpoint.OutputTokens,
		checkpoint.TotalTokens, checkpoint.ExecutionMillis, checkpoint.UpdatedAt.Format(time.RFC3339))
	return nil
}

func (a *App) runAdaptTask(ctx context.Context, args []string) error {
	fs := newFlagSet("run adapt-task", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run adapt-task <task-id>")
	}
	result, err := application.NewTaskAdapter(a.store).Adapt(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	action := "reused"
	if result.Created {
		action = "adapted"
	}
	fmt.Fprintf(a.out, "task %s %s\nmission: %s\nrun: %s\nsession: %s\nstatus: %s\nprofile: %s\n",
		result.Source.ID, action, result.Mission.ID, result.Run.ID, result.Run.SessionID, result.Run.Status, result.Mission.Profile)
	return nil
}

func (a *App) runCreate(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run create", a.errOut)
	workspaceName := fs.String("workspace", "", "workspace name")
	profile := fs.String("profile", string(domain.ProfileCode), "mission profile")
	route := fs.String("route", "", "model route")
	sessionID := fs.String("session", "", "existing session id")
	interactive := fs.Bool("interactive", false, "mark run as interactive")
	maxTurns := fs.Int("max-turns", domain.DefaultBudget().MaxTurns, "maximum agent turns")
	maxTokens := fs.Int64("max-tokens", 0, "maximum model tokens; zero means unset")
	maxCostUSD := fs.Float64("max-cost-usd", 0, "maximum model cost in USD; zero means unset")
	timeout := fs.Duration("timeout", 0, "run timeout; zero means unset")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"workspace":    true,
		"profile":      true,
		"route":        true,
		"session":      true,
		"interactive":  false,
		"max-turns":    true,
		"max-tokens":   true,
		"max-cost-usd": true,
		"timeout":      true,
	})); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New(`usage: cyberagent run create "goal" [--workspace <name>] [--profile code|review|learn|script]`)
	}
	workspaceID := ""
	if strings.TrimSpace(*workspaceName) != "" {
		rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(*workspaceName))
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
		if workspaceID != "" && sess.WorkspaceID != "" && workspaceID != sess.WorkspaceID {
			return errors.New("session and requested workspace do not match")
		}
		if workspaceID == "" {
			workspaceID = sess.WorkspaceID
		}
	}
	mission, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal:        strings.Join(fs.Args(), " "),
		Profile:     *profile,
		WorkspaceID: workspaceID,
		SessionID:   *sessionID,
		ModelRoute:  *route,
		Interactive: *interactive,
		Budget: domain.Budget{
			MaxTurns:       *maxTurns,
			MaxTokens:      *maxTokens,
			MaxCostUSD:     *maxCostUSD,
			TimeoutSeconds: int64(timeout.Seconds()),
		},
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run %s created\nmission: %s\nsession: %s\nstatus: %s\nprofile: %s\nworkspace: %s\nroute: %s\n",
		run.ID, mission.ID, run.SessionID, run.Status, mission.Profile, mission.WorkspaceID, run.Config.ModelRoute)
	return nil
}

func (a *App) runList(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run list", a.errOut)
	statusValue := fs.String("status", "", "run status")
	missionID := fs.String("mission", "", "mission id")
	limit := fs.Int("limit", 100, "maximum rows")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"status": true, "mission": true, "limit": true})); err != nil {
		return err
	}
	status := domain.RunStatus(strings.TrimSpace(*statusValue))
	if status != "" && !domain.ValidRunStatus(status) {
		return fmt.Errorf("invalid run status %q", status)
	}
	if *limit <= 0 || *limit > 1000 {
		return errors.New("run list limit must be between 1 and 1000")
	}
	runs, err := service.List(ctx, domain.RunFilter{MissionID: *missionID, Status: status, Limit: *limit})
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Fprintln(a.out, "no runs")
		return nil
	}
	for _, run := range runs {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\n", run.ID, run.Status, run.MissionID, run.Config.ModelRoute, run.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func (a *App) runShow(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run show <run-id>")
	}
	mission, run, err := service.Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	scope, _ := json.Marshal(mission.Scope)
	budget, _ := json.Marshal(run.Budget)
	fmt.Fprintf(a.out, "id: %s\nmission: %s\nstatus: %s\ngoal: %s\nprofile: %s\nworkspace: %s\nsession: %s\nroute: %s\ninteractive: %t\nscope: %s\nbudget: %s\ncreated_at: %s\nupdated_at: %s\n",
		run.ID, mission.ID, run.Status, mission.Goal, mission.Profile, mission.WorkspaceID, run.SessionID,
		run.Config.ModelRoute, run.Config.Interactive, scope, budget, run.CreatedAt.Format(time.RFC3339), run.UpdatedAt.Format(time.RFC3339))
	if run.StartedAt != nil {
		fmt.Fprintf(a.out, "started_at: %s\n", run.StartedAt.Format(time.RFC3339))
	}
	if run.FinishedAt != nil {
		fmt.Fprintf(a.out, "finished_at: %s\n", run.FinishedAt.Format(time.RFC3339))
	}
	return nil
}

func (a *App) runEvents(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run events", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run events <run-id>")
	}
	items, err := service.Events(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(a.out, "no run events")
		return nil
	}
	for _, event := range items {
		fmt.Fprintf(a.out, "#%d\t%s\t%s\t%s\t%s\n", event.Sequence, event.CreatedAt.Format(time.RFC3339), event.Type, event.Source, event.PayloadJSON)
	}
	return nil
}

func (a *App) runTransition(ctx context.Context, service *application.RunService, action string, args []string) error {
	fs := newFlagSet("run "+action, a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: cyberagent run %s <run-id>", action)
	}
	var run domain.Run
	var err error
	switch action {
	case "start":
		run, err = service.Start(ctx, fs.Arg(0))
	case "pause":
		run, err = service.Pause(ctx, fs.Arg(0))
	case "resume":
		run, err = service.Resume(ctx, fs.Arg(0))
	case "cancel":
		run, err = service.Cancel(ctx, fs.Arg(0))
	default:
		return fmt.Errorf("unknown run transition %q", action)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run %s %s\n", run.ID, run.Status)
	if action == "start" {
		fmt.Fprintln(a.out, "note: lifecycle is running; use `cyberagent run step <run-id>` for one supervised turn")
	}
	return nil
}
