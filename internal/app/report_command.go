package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
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
	case "finding":
		return a.reportFindingCommand(ctx, args[1:])
	case "check":
		return a.reportCheck(ctx, args[1:])
	default:
		return fmt.Errorf("unknown report subcommand %q", args[0])
	}
}

func (a *App) reportFindingCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("report finding subcommand is required")
	}
	switch args[0] {
	case "attach":
		return a.reportFindingAttach(ctx, args[1:])
	case "validate":
		return a.reportFindingDecide(ctx, args[1:], domain.FindingStatusValidated)
	case "reject":
		return a.reportFindingDecide(ctx, args[1:], domain.FindingStatusRejected)
	case "verify":
		return a.reportFindingVerify(ctx, args[1:])
	default:
		return fmt.Errorf("unknown report finding subcommand %q", args[0])
	}
}

func (a *App) reportFindingAttach(ctx context.Context, args []string) error {
	fs := newFlagSet("report finding attach", a.errOut)
	operationKey := fs.String("operation-key", "", "stable evidence operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	note := fs.String("note", "", "evidence note")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true, "note": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 2 || strings.TrimSpace(*operationKey) == "" ||
		strings.TrimSpace(*note) == "" {
		return errors.New("usage: cyberagent report finding attach <finding-id> <artifact-id> --operation-key <key> --note <text> [--operator <id>]")
	}
	result, err := application.NewFindingReportService(a.store).AttachArtifactEvidence(ctx,
		application.AttachFindingArtifactEvidenceRequest{
			FindingID: fs.Arg(0), ArtifactID: fs.Arg(1), OperationKey: *operationKey,
			AttachedBy: *operator, Note: *note,
		})
	if err != nil {
		return err
	}
	verb := "attached"
	if result.Replayed {
		verb = "reused"
	}
	fmt.Fprintf(a.out, "finding Artifact Evidence %s %s\n", result.Evidence.ID, verb)
	fmt.Fprintf(a.out, "finding: %s\nartifact: %s\nordinal: %d\nsha256: %s\n",
		result.Evidence.FindingID, result.Evidence.ArtifactID,
		result.Evidence.Ordinal, result.Evidence.ArtifactSHA256)
	return nil
}

func (a *App) reportFindingDecide(ctx context.Context, args []string,
	status domain.FindingStatus,
) error {
	name := "validate"
	if status == domain.FindingStatusRejected {
		name = "reject"
	}
	fs := newFlagSet("report finding "+name, a.errOut)
	operationKey := fs.String("operation-key", "", "stable validation operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	reason := fs.String("reason", "", "validation reason")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true, "reason": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" ||
		strings.TrimSpace(*reason) == "" {
		return fmt.Errorf("usage: cyberagent report finding %s <finding-id> --operation-key <key> --reason <text> [--operator <id>]", name)
	}
	result, err := application.NewFindingReportService(a.store).DecideValidation(ctx,
		application.DecideFindingValidationRequest{
			FindingID: fs.Arg(0), OperationKey: *operationKey, Status: status,
			Reason: *reason, DecidedBy: *operator,
		})
	if err != nil {
		return err
	}
	verb := "recorded"
	if result.Replayed {
		verb = "reused"
	}
	fmt.Fprintf(a.out, "finding validation %s %s\n", result.Validation.ID, verb)
	fmt.Fprintf(a.out, "finding: %s\nstatus: %s\nartifact_evidence_count: %d\nartifact_evidence_digest: %s\n",
		result.Validation.FindingID, result.Validation.Status,
		result.Validation.ArtifactEvidenceCount,
		result.Validation.ArtifactEvidenceDigest)
	return nil
}

func (a *App) reportFindingVerify(ctx context.Context, args []string) error {
	fs := newFlagSet("report finding verify", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent report finding verify <finding-id>")
	}
	result, err := application.NewFindingReportService(a.store).
		VerifyArtifactEvidence(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "finding %s Artifact Evidence verified\n", result.FindingID)
	fmt.Fprintf(a.out, "run: %s\nstatus: %s\nartifact_evidence_count: %d\nartifact_evidence_digest: %s\n",
		result.RunID, result.Status, result.ArtifactEvidenceCount,
		result.ArtifactEvidenceDigest)
	return nil
}

func (a *App) reportShow(ctx context.Context, args []string) error {
	fs := newFlagSet("report show", a.errOut)
	format := fs.String("format", "markdown", "report format: markdown, json, or sarif")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"format": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent report show <report-id> [--format markdown|json|sarif]")
	}
	value, err := application.NewFindingReportService(a.store).Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	return a.renderFindingReport(value, *format)
}

func (a *App) reportCheck(ctx context.Context, args []string) error {
	fs := newFlagSet("report check", a.errOut)
	failStatus := fs.String("fail-status", "validated",
		"finding status gate: validated, active, or none")
	minSeverity := fs.String("min-severity", "high",
		"minimum source severity: info, low, medium, high, or critical")
	format := fs.String("format", "text", "check output format: text or json")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"fail-status": true, "min-severity": true, "format": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent report check <report-id> [--fail-status validated|active|none] [--min-severity info|low|medium|high|critical] [--format text|json]")
	}
	outputFormat := strings.ToLower(strings.TrimSpace(*format))
	if outputFormat != "text" && outputFormat != "json" {
		return errors.New("report check format must be text or json")
	}
	status, err := reporting.ParseGateStatus(*failStatus)
	if err != nil {
		return err
	}
	severity, err := reporting.ParseGateSeverity(*minSeverity)
	if err != nil {
		return err
	}
	value, err := application.NewFindingReportService(a.store).Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	result, err := reporting.EvaluateGate(value, reporting.GatePolicy{
		FailStatus: status, MinSeverity: severity,
	})
	if err != nil {
		return err
	}
	switch outputFormat {
	case "text":
		if _, err := fmt.Fprintf(a.out,
			"report: %s\nrun: %s\nprojection_digest: %s\n",
			result.ReportID, result.RunID, result.ProjectionDigest); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(a.out, "fail_status: %s\nmin_severity: %s\n",
			result.Policy.FailStatus, result.Policy.MinSeverity); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(a.out,
			"findings: %d\ndraft: %d\nvalidated: %d\nrejected: %d\nmatched: %d\npassed: %t\n",
			result.FindingCount, result.DraftCount, result.ValidatedCount,
			result.RejectedCount, result.MatchedCount, result.Passed); err != nil {
			return err
		}
	case "json":
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		if _, err := a.out.Write(append(encoded, '\n')); err != nil {
			return err
		}
	}
	if !result.Passed {
		return apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("report check matched %d %s finding(s) at or above %s severity",
				result.MatchedCount, result.Policy.FailStatus, result.Policy.MinSeverity))
	}
	return nil
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
