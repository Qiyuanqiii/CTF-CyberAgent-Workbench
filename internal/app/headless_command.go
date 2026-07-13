package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/headless"
)

const maxHeadlessFollowTimeout = 24 * time.Hour

func (a *App) headlessCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("headless subcommand is required")
	}
	switch args[0] {
	case "events":
		return a.headlessEvents(ctx, args[1:])
	default:
		return fmt.Errorf("unknown headless subcommand %q", args[0])
	}
}

func (a *App) headlessEvents(ctx context.Context, args []string) error {
	fs := newFlagSet("headless events", a.errOut)
	afterSequence := fs.Int64("after-sequence", 0, "resume after this durable event sequence")
	maxEvents := fs.Int("max-events", headless.DefaultMaxEvents, "maximum events emitted")
	follow := fs.Bool("follow", false, "follow until the Run reaches a terminal status")
	pollInterval := fs.Duration("poll-interval", headless.DefaultPollInterval,
		"SQLite polling interval while following")
	timeout := fs.Duration("timeout", 0, "optional follow timeout")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"after-sequence": true, "max-events": true, "follow": false,
		"poll-interval": true, "timeout": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent headless events <run-id> [--after-sequence <n>] [--max-events <n>] [--follow] [--poll-interval <duration>] [--timeout <duration>]")
	}
	if *timeout < 0 || *timeout > maxHeadlessFollowTimeout {
		return apperror.New(apperror.CodeInvalidArgument,
			"headless timeout must be between zero and 24h")
	}
	if *timeout > 0 && !*follow {
		return apperror.New(apperror.CodeInvalidArgument,
			"headless timeout requires --follow")
	}
	request, err := (headless.Request{RunID: fs.Arg(0),
		AfterSequence: *afterSequence, MaxEvents: *maxEvents,
		Follow: *follow, PollInterval: *pollInterval}).Normalize()
	if err != nil {
		return err
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}
	return headless.NewExporter(a.store).Export(ctx, a.out, request)
}
