package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
)

func (a *App) runWake(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent run wake schedule|show|cancel|consume")
	}
	service := application.NewRunWakeControlService(a.store)
	switch args[0] {
	case "schedule":
		fs := newFlagSet("run wake schedule", a.errOut)
		operationKey := fs.String("operation-key", "", "stable wake operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		maxAttempts := fs.Int("max-attempts", 3, "maximum ownership attempts")
		initialDelay := fs.Int("initial-delay-seconds", 0, "delay before first claim")
		baseBackoff := fs.Int("base-backoff-seconds", 5, "initial retry backoff")
		maxBackoff := fs.Int("max-backoff-seconds", 60, "maximum retry backoff")
		maxElapsed := fs.Int("max-elapsed-seconds", 300, "total wake intent lifetime")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"operation-key": true, "operator": true, "max-attempts": true,
			"initial-delay-seconds": true, "base-backoff-seconds": true,
			"max-backoff-seconds": true, "max-elapsed-seconds": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run wake schedule <run-id> --operation-key <key> [bounded retry options]")
		}
		result, err := service.Schedule(ctx, application.ScheduleRunWakeRequest{
			Version: domain.RunWakeControlProtocolVersion, RunID: fs.Arg(0),
			OperationKey: *operationKey, RequestedBy: *operator,
			MaxAttempts: *maxAttempts, InitialDelaySeconds: *initialDelay,
			BaseBackoffSeconds: *baseBackoff, MaxBackoffSeconds: *maxBackoff,
			MaxElapsedSeconds: *maxElapsed,
		})
		if err != nil {
			return err
		}
		printRunWakeIntent(a.out, result.Intent)
		fmt.Fprintf(a.out, "replayed: %t\n", result.Replayed)
		return nil
	case "show":
		fs := newFlagSet("run wake show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run wake show <run-id>")
		}
		intent, found, err := service.Get(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		if !found {
			fmt.Fprintln(a.out, "no Run wake intent")
			return nil
		}
		printRunWakeIntent(a.out, intent)
		return nil
	case "cancel":
		fs := newFlagSet("run wake cancel", a.errOut)
		operationKey := fs.String("operation-key", "", "stable cancellation operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run wake cancel <run-id> --operation-key <key>")
		}
		result, err := service.Cancel(ctx, application.CancelRunWakeRequest{
			Version: domain.RunWakeControlProtocolVersion, RunID: fs.Arg(0),
			OperationKey: *operationKey, RequestedBy: *operator,
		})
		if err != nil {
			return err
		}
		printRunWakeIntent(a.out, result.Intent)
		fmt.Fprintf(a.out, "replayed: %t\n", result.Replayed)
		return nil
	case "consume":
		fs := newFlagSet("run wake consume", a.errOut)
		operator := fs.String("operator", "cli_foreground", "foreground owner identity")
		maxSteps := fs.Int("max-steps", 1, "bounded Run Supervisor handoff steps")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"operator": true, "max-steps": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run wake consume <run-id> [--max-steps 1..8] [--operator <id>]")
		}
		handoff := application.NewRunExecutionHandoffService(a.store, a.router,
			a.checker).WithActiveCalls(a.calls)
		result, err := application.NewForegroundRunWakeConsumer(a.store, handoff).
			Consume(ctx, application.ConsumeRunWakeRequest{
				Version: domain.RunWakeConsumerProtocolVersion, RunID: fs.Arg(0),
				OwnerID: *operator, MaxSteps: *maxSteps,
			})
		if result.Intent.ID != "" {
			printRunWakeIntent(a.out, result.Intent)
			fmt.Fprintf(a.out, "consumer_protocol: %s\nconsumption_status: %s\nreplayed: %t\nbackground_loop_enabled: false\n",
				domain.RunWakeConsumerProtocolVersion, result.Consumption.Status,
				result.Replayed)
			if result.Handoff.Result != nil {
				fmt.Fprintf(a.out, "handoff_status: %s\nstop_reason: %s\nmodel_called: %t\ntool_called: %t\n",
					result.Handoff.Result.Status, result.Handoff.Result.StopReason,
					result.Handoff.Result.ModelCalled, result.Handoff.Result.ToolCalled)
			}
		}
		return err
	default:
		return fmt.Errorf("unknown run wake subcommand %q", args[0])
	}
}

func printRunWakeIntent(out interface{ Write([]byte) (int, error) }, intent domain.RunWakeIntent) {
	cancelledAt := ""
	if intent.CancelledAt != nil {
		cancelledAt = intent.CancelledAt.Format(time.RFC3339Nano)
	}
	fmt.Fprintf(out, "protocol: %s\nintent: %s\nrun: %s\nsession: %s\nstatus: %s\nattempts: %d/%d\nnext_wake_at: %s\ndeadline_at: %s\ncancelled_at: %s\nexecution_enabled: false\nbackground_loop_enabled: false\n",
		intent.ProtocolVersion, intent.ID, intent.RunID, intent.SessionID, intent.Status,
		intent.AttemptCount, intent.MaxAttempts, intent.NextWakeAt.Format(time.RFC3339Nano),
		intent.DeadlineAt.Format(time.RFC3339Nano), cancelledAt)
}
