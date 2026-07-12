package app

import (
	"context"
	"errors"
	"fmt"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	reporting "cyberagent-workbench/internal/report"
)

func (a *App) reportCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("report subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	switch args[0] {
	case "show":
		return a.reportShow(ctx, args[1:])
	default:
		return fmt.Errorf("unknown report subcommand %q", args[0])
	}
}

func (a *App) reportShow(ctx context.Context, args []string) error {
	fs := newFlagSet("report show", a.errOut)
	format := fs.String("format", "markdown", "report format: markdown or json")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"format": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent report show <report-id> [--format markdown|json]")
	}
	value, err := application.NewFindingReportService(a.store).Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	return a.renderFindingReport(value, *format)
}

func (a *App) renderFindingReport(value domain.FindingReport, formatValue string) error {
	format, err := reporting.ParseFormat(formatValue)
	if err != nil {
		return err
	}
	encoded, err := reporting.Render(value, format)
	if err != nil {
		return err
	}
	_, err = a.out.Write(encoded)
	return err
}
