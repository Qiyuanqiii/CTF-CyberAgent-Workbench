package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/coordinator"
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
	case "mode":
		return a.runMode(ctx, service, args[1:])
	case "phase":
		return a.runPhase(ctx, service, args[1:])
	case "events":
		return a.runEvents(ctx, service, args[1:])
	case "step":
		return a.runSupervisorStep(ctx, args[1:])
	case "execute":
		return a.runSupervisorExecute(ctx, args[1:])
	case "checkpoint":
		return a.runSupervisorCheckpoint(ctx, args[1:])
	case "graph":
		return a.runAgentGraph(ctx, args[1:])
	case "delegations":
		return a.runDelegations(ctx, args[1:])
	case "delegation":
		return a.runDelegation(ctx, args[1:])
	case "plans":
		return a.runPlanDeliveryProposals(ctx, args[1:])
	case "plan":
		return a.runPlanDelivery(ctx, args[1:])
	case "delivery":
		return a.runDeliveryCheckpoint(ctx, args[1:])
	case "fanouts":
		return a.runFanouts(ctx, args[1:])
	case "fanout":
		return a.runFanout(ctx, args[1:])
	case "lease":
		return a.runExecutionLease(ctx, service, args[1:])
	case "usage":
		return a.runUsage(ctx, service, args[1:])
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

func (a *App) runDeliveryCheckpoint(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent run delivery checkpoint|list|show")
	}
	switch args[0] {
	case "checkpoint":
		return a.runDeliveryCheckpointRecord(ctx, args[1:])
	case "list":
		return a.runDeliveryCheckpointList(ctx, args[1:])
	case "show":
		return a.runDeliveryCheckpointShow(ctx, args[1:])
	default:
		return fmt.Errorf("unknown run delivery subcommand %q", args[0])
	}
}

func (a *App) runDeliveryCheckpointRecord(ctx context.Context, args []string) error {
	fs := newFlagSet("run delivery checkpoint", a.errOut)
	operationKey := fs.String("operation-key", "", "stable Delivery checkpoint operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	focused := fs.String("focused", "", "focused verification evidence")
	diffAudit := fs.String("diff-audit", "", "diff audit evidence")
	securityAudit := fs.String("security-audit", "", "security audit evidence")
	functional := fs.String("functional", "", "final-boundary functional verification evidence")
	robustness := fs.String("robustness", "", "final-boundary robustness audit evidence")
	handoff := fs.String("handoff", "", "compact durable handoff summary")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true, "focused": true,
		"diff-audit": true, "security-audit": true, "functional": true,
		"robustness": true, "handoff": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
		return errors.New("usage: cyberagent run delivery checkpoint <work-id> --operation-key <key> --focused <evidence> --diff-audit <evidence> --security-audit <evidence> --handoff <summary> [--functional <evidence> --robustness <evidence>] [--operator <id>]")
	}
	result, err := application.NewDeliveryCheckpointService(a.store).Record(ctx,
		application.RecordDeliveryCheckpointRequest{
			WorkItemID: fs.Arg(0), OperationKey: *operationKey,
			RequestedBy: *operator, FocusedVerification: *focused,
			DiffAudit: *diffAudit, SecurityAudit: *securityAudit,
			FunctionalVerification: *functional, RobustnessAudit: *robustness,
			HandoffSummary: *handoff,
		})
	if err != nil {
		return err
	}
	printDeliveryCheckpoint(a, result.Checkpoint, false)
	fmt.Fprintf(a.out, "handoff_note: %s\nreplayed: %t\ncompletion_gate_ready: true\nnext: cyberagent todo complete %s --version %d\n",
		result.Note.ID, result.Replayed, result.Checkpoint.WorkItemID,
		result.Checkpoint.WorkItemVersion)
	return nil
}

func (a *App) runDeliveryCheckpointList(ctx context.Context, args []string) error {
	fs := newFlagSet("run delivery list", a.errOut)
	limit := fs.Int("limit", 100, "maximum Delivery checkpoints")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"limit": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run delivery list <run-id> [--limit <n>]")
	}
	values, err := application.NewDeliveryCheckpointService(a.store).List(ctx,
		fs.Arg(0), *limit)
	if err != nil {
		return err
	}
	if len(values) == 0 {
		fmt.Fprintln(a.out, "no Delivery checkpoints")
		return nil
	}
	for _, value := range values {
		fmt.Fprintf(a.out, "%s\twork_item=%s\tslice=%d/%d\tmode_revision=%d\twork_item_version=%d\tfull_gate=%t\thandoff_note=%s\tcreated_at=%s\n",
			value.ID, value.WorkItemID, value.ModuleOrdinal, value.ModuleCount,
			value.ModeRevision, value.WorkItemVersion, value.FullGateRequired,
			value.HandoffNoteID, value.CreatedAt.Format(time.RFC3339Nano))
	}
	return nil
}

func (a *App) runDeliveryCheckpointShow(ctx context.Context, args []string) error {
	fs := newFlagSet("run delivery show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run delivery show <checkpoint-id>")
	}
	value, err := application.NewDeliveryCheckpointService(a.store).Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	printDeliveryCheckpoint(a, value, true)
	return nil
}

func printDeliveryCheckpoint(a *App, value domain.DeliveryCheckpoint,
	includeEvidence bool,
) {
	fmt.Fprintf(a.out, "checkpoint: %s\nrun: %s\nselection: %s\nproposal: %s\nwork_item: %s\ndirection: %d\nslice: %d/%d\nmode_snapshot: %s\nmode_revision: %d\nwork_item_version: %d\nacceptance_fingerprint: %s\nsource_fingerprint: %s\nfull_gate_required: %t\nhandoff_note: %s\nhandoff_digest: %s\nrequested_by: %s\nversion: %d\ncreated_at: %s\n",
		value.ID, value.RunID, value.SelectionID, value.ProposalID,
		value.WorkItemID, value.DirectionOrdinal, value.ModuleOrdinal,
		value.ModuleCount, value.ModeSnapshotID, value.ModeRevision,
		value.WorkItemVersion, value.AcceptanceFingerprint,
		value.SourceFingerprint, value.FullGateRequired, value.HandoffNoteID,
		value.HandoffDigest, value.RequestedBy, value.Version,
		value.CreatedAt.Format(time.RFC3339Nano))
	if includeEvidence {
		fmt.Fprintf(a.out, "focused_verification: %s\ndiff_audit: %s\nsecurity_audit: %s\n",
			planDeliveryCLIText(value.FocusedVerification),
			planDeliveryCLIText(value.DiffAudit),
			planDeliveryCLIText(value.SecurityAudit))
		if value.FullGateRequired {
			fmt.Fprintf(a.out, "functional_verification: %s\nrobustness_audit: %s\n",
				planDeliveryCLIText(value.FunctionalVerification),
				planDeliveryCLIText(value.RobustnessAudit))
		}
	}
}

func (a *App) runPlanDelivery(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent run plan show|choose|selection")
	}
	switch args[0] {
	case "show":
		return a.runPlanDeliveryShow(ctx, args[1:])
	case "choose":
		return a.runPlanDeliveryChoose(ctx, args[1:])
	case "selection":
		return a.runPlanDeliverySelection(ctx, args[1:])
	default:
		return fmt.Errorf("unknown run plan subcommand %q", args[0])
	}
}

func (a *App) runPlanDeliveryProposals(ctx context.Context, args []string) error {
	fs := newFlagSet("run plans", a.errOut)
	limit := fs.Int("limit", 20, "maximum Plan/Delivery proposals")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"limit": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run plans <run-id> [--limit <n>]")
	}
	proposals, err := application.NewPlanDeliveryService(a.store).
		ListProposals(ctx, fs.Arg(0), *limit)
	if err != nil {
		return err
	}
	if len(proposals) == 0 {
		fmt.Fprintln(a.out, "no Plan/Delivery proposals")
		return nil
	}
	selection, selected, err := application.NewPlanDeliveryService(a.store).
		SelectionForRun(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	for _, proposal := range proposals {
		selectedDirection := 0
		if selected && selection.ProposalID == proposal.ID {
			selectedDirection = selection.DirectionOrdinal
		}
		fmt.Fprintf(a.out, "%s\tstatus=%s\tdirections=%d\tmode_revision=%d\tselected_direction=%d\tfingerprint=%s\tcreated_at=%s\n",
			proposal.ID, proposal.Status, len(proposal.Spec.Directions),
			proposal.ModeRevision, selectedDirection, proposal.Fingerprint,
			proposal.CreatedAt.Format(time.RFC3339Nano))
	}
	return nil
}

func (a *App) runPlanDeliveryShow(ctx context.Context, args []string) error {
	fs := newFlagSet("run plan show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run plan show <proposal-id>")
	}
	proposal, err := application.NewPlanDeliveryService(a.store).
		GetProposal(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	printPlanDeliveryProposal(a, proposal)
	return nil
}

func (a *App) runPlanDeliveryChoose(ctx context.Context, args []string) error {
	fs := newFlagSet("run plan choose", a.errOut)
	operationKey := fs.String("operation-key", "", "stable direction choice operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 2 || strings.TrimSpace(*operationKey) == "" {
		return errors.New("usage: cyberagent run plan choose <proposal-id> 1|2|3 --operation-key <key> [--operator <id>]")
	}
	direction, err := strconv.Atoi(fs.Arg(1))
	if err != nil {
		return errors.New("Plan/Delivery direction must be 1, 2, or 3")
	}
	result, err := application.NewPlanDeliveryService(a.store).Select(ctx,
		application.SelectPlanDeliveryDirectionRequest{
			ProposalID: fs.Arg(0), Direction: direction,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
	if err != nil {
		return err
	}
	printPlanDeliverySelection(a, result.Selection)
	for _, item := range result.WorkItems {
		fmt.Fprintf(a.out, "work_item[%d]: %s title=%s dependencies=%s\n",
			indexPlanDeliveryWorkItem(result.Selection, item.ID), item.ID,
			item.Title, strings.Join(item.Dependencies, ","))
	}
	fmt.Fprintf(a.out, "handoff_note: %s\nreplayed: %t\nphase_changed: false\ncapability_grant: false\n",
		result.Note.ID, result.Replayed)
	fmt.Fprintf(a.out, "next: cyberagent run phase %s deliver --operation-key <key> --operator %s --reason \"accepted direction %d\"\n",
		result.Selection.RunID, result.Selection.RequestedBy,
		result.Selection.DirectionOrdinal)
	return nil
}

func (a *App) runPlanDeliverySelection(ctx context.Context, args []string) error {
	fs := newFlagSet("run plan selection", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run plan selection <run-id>")
	}
	selection, found, err := application.NewPlanDeliveryService(a.store).
		SelectionForRun(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(a.out, "no Plan/Delivery direction selected")
		return nil
	}
	printPlanDeliverySelection(a, selection)
	return nil
}

func printPlanDeliveryProposal(a *App, proposal domain.PlanDeliveryProposal) {
	fmt.Fprintf(a.out, "proposal: %s\nrun: %s\nprotocol: %s\nstatus: %s\nmode_revision: %d\ndirection_count: %d\nfingerprint: %s\noperator_choice_required: true\nselection_authorized: false\nphase_change_authorized: false\nexecution_authorized: false\n",
		proposal.ID, proposal.RunID, proposal.Spec.Version, proposal.Status,
		proposal.ModeRevision, len(proposal.Spec.Directions), proposal.Fingerprint)
	for _, direction := range proposal.Spec.Directions {
		fmt.Fprintf(a.out, "\ndirection %d: %s\nsummary: %s\ntradeoffs: %s\n",
			direction.Ordinal, planDeliveryCLIText(direction.Title),
			planDeliveryCLIText(direction.Summary),
			planDeliveryCLIText(strings.Join(direction.Tradeoffs, " | ")))
		for _, module := range direction.Modules {
			dependencies := make([]string, len(module.Dependencies))
			for index, dependency := range module.Dependencies {
				dependencies[index] = strconv.Itoa(dependency)
			}
			fmt.Fprintf(a.out, "  slice %d: %s\n    objective: %s\n    acceptance: %s\n    depends_on: %s\n",
				module.Ordinal, planDeliveryCLIText(module.Title),
				planDeliveryCLIText(module.Objective),
				planDeliveryCLIText(strings.Join(module.AcceptanceCriteria, " | ")),
				strings.Join(dependencies, ","))
		}
	}
}

func planDeliveryCLIText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func printPlanDeliverySelection(a *App, selection domain.PlanDeliverySelection) {
	fmt.Fprintf(a.out, "selection: %s\nproposal: %s\nrun: %s\ndirection: %d\nmodule_count: %d\nnote: %s\nrequested_by: %s\ncreated_at: %s\n",
		selection.ID, selection.ProposalID, selection.RunID,
		selection.DirectionOrdinal, len(selection.Items), selection.NoteID,
		selection.RequestedBy, selection.CreatedAt.Format(time.RFC3339Nano))
	for _, item := range selection.Items {
		fmt.Fprintf(a.out, "selected_slice[%d]: module=%d work_item=%s\n",
			item.Ordinal, item.ModuleOrdinal, item.WorkItemID)
	}
}

func indexPlanDeliveryWorkItem(selection domain.PlanDeliverySelection,
	workItemID string,
) int {
	for _, item := range selection.Items {
		if item.WorkItemID == workItemID {
			return item.Ordinal
		}
	}
	return 0
}

func (a *App) runFanouts(ctx context.Context, args []string) error {
	fs := newFlagSet("run fanouts", a.errOut)
	limit := fs.Int("limit", 20, "maximum read-only fan-out plans")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"limit": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run fanouts <run-id> [--limit <n>]")
	}
	plans, err := a.store.ListReadOnlyFanoutPlans(ctx, fs.Arg(0), *limit)
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		fmt.Fprintln(a.out, "no read-only fan-out plans")
		return nil
	}
	for _, plan := range plans {
		fmt.Fprintf(a.out, "%s\tstatus=%s\ttier=%s\tparallelism=%d\tfiles=%d\tshards=%d\texecution_authorized=false\tcreated_at=%s\n",
			plan.ID, plan.Status, plan.RequestedTier, plan.EffectiveParallelism,
			plan.FileCount, plan.ShardCount, plan.CreatedAt.Format(time.RFC3339Nano))
	}
	return nil
}

func (a *App) runFanout(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent run fanout plan|execute|show|execution|report")
	}
	switch args[0] {
	case "plan":
		return a.runFanoutPlan(ctx, args[1:])
	case "execute":
		return a.runFanoutExecute(ctx, args[1:])
	case "show":
		return a.runFanoutShow(ctx, args[1:])
	case "execution":
		return a.runFanoutExecutionShow(ctx, args[1:])
	case "report":
		return a.runFanoutReport(ctx, args[1:])
	default:
		return a.runFanoutShow(ctx, args)
	}
}

func (a *App) runFanoutReport(ctx context.Context, args []string) error {
	fs := newFlagSet("run fanout report", a.errOut)
	format := fs.String("format", "markdown", "report format: markdown, json, or sarif")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"format": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run fanout report <execution-id> [--format markdown|json|sarif]")
	}
	value, _, err := application.NewFindingReportService(a.store).
		GenerateReadOnlyFanout(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	return a.renderFindingReport(value, *format)
}

func (a *App) runFanoutExecute(ctx context.Context, args []string) error {
	fs := newFlagSet("run fanout execute", a.errOut)
	operationKey := fs.String("operation-key", "", "stable execution operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	maxOutputTokens := fs.Int("max-output-tokens", 1024,
		"maximum output tokens reserved for each shard")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true, "max-output-tokens": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
		return errors.New("usage: cyberagent run fanout execute <plan-id> --operation-key <key> [--operator <id>] [--max-output-tokens <128..4096>]")
	}
	result, err := application.NewReadOnlyFanoutExecutionService(a.store, a.router,
		a.checker).Execute(ctx, application.ExecuteReadOnlyFanoutRequest{
		PlanID: fs.Arg(0), OperationKey: *operationKey, RequestedBy: *operator,
		MaxOutputTokensPerShard: *maxOutputTokens,
	})
	if result.Execution.ID != "" {
		a.printReadOnlyFanoutExecution(result.Execution, result.Replayed,
			result.Recovered, &result.UsageAfter)
	}
	return err
}

func (a *App) runFanoutExecutionShow(ctx context.Context, args []string) error {
	fs := newFlagSet("run fanout execution", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run fanout execution <execution-id>")
	}
	execution, err := a.store.GetReadOnlyFanoutExecution(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	a.printReadOnlyFanoutExecution(execution, false, false, nil)
	return nil
}

func (a *App) printReadOnlyFanoutExecution(execution domain.ReadOnlyFanoutExecution,
	replayed bool, recovered bool, usage *domain.RunAgentUsage,
) {
	fmt.Fprintf(a.out, "fanout_execution: %s\nplan: %s\nrun: %s\nstatus: %s\nparallelism: %d\nmax_output_tokens_per_shard: %d\nsnapshot_digest: %s\ncapability: workspace_readonly\nshell: false\nfile_write: false\nprocess: false\nnetwork: false\nexternal_tools: false\nchild_spawn: false\nreplayed: %t\nrecovered: %t\n",
		execution.ID, execution.PlanID, execution.RunID, execution.Status,
		execution.Parallelism, execution.MaxOutputTokensPerShard,
		execution.SnapshotDigest, replayed, recovered)
	if execution.StopCode != "" {
		fmt.Fprintf(a.out, "stop_code: %s\n", execution.StopCode)
	}
	for _, shard := range execution.Shards {
		fmt.Fprintf(a.out, "shard_%d: status=%s attempts=%d provider=%s model=%s tokens=%d elapsed_millis=%d findings=%d",
			shard.Ordinal, shard.Status, shard.AttemptCount, shard.Provider, shard.Model,
			shard.TotalTokens, shard.ElapsedMillis, shard.FindingCount)
		if shard.ErrorCode != "" {
			fmt.Fprintf(a.out, " error_code=%s", shard.ErrorCode)
		}
		fmt.Fprintln(a.out)
		if shard.ReportJSON != "" {
			var report domain.ReadOnlyFanoutReport
			if json.Unmarshal([]byte(shard.ReportJSON), &report) == nil {
				fmt.Fprintf(a.out, "  summary: %s\n", report.Summary)
				for _, finding := range report.Findings {
					fmt.Fprintf(a.out, "  finding: severity=%s path=%s line=%d-%d title=%s\n",
						finding.Severity, finding.Path, finding.LineStart,
						finding.LineEnd, finding.Title)
				}
			}
		}
	}
	if usage != nil && usage.RunID != "" {
		fmt.Fprintf(a.out, "run_total_tokens: %d\nrun_readonly_fanout_tokens: %d\nrun_total_execution_millis: %d\nrun_readonly_fanout_millis: %d\n",
			usage.TotalTokens, usage.ReadOnlyFanoutTokens,
			usage.TotalExecutionMillis, usage.ReadOnlyFanoutMillis)
	}
}

func (a *App) runFanoutPlan(ctx context.Context, args []string) error {
	fs := newFlagSet("run fanout plan", a.errOut)
	tier := fs.String("tier", "auto", "parallelism cap: auto, 1, 2, 4, or 6")
	scopePath := fs.String("path", ".", "workspace-relative directory scope")
	operationKey := fs.String("operation-key", "", "stable planning operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"tier": true, "path": true, "operation-key": true, "operator": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 2 || strings.TrimSpace(*operationKey) == "" {
		return errors.New("usage: cyberagent run fanout plan <run-id> <goal> --operation-key <key> [--tier auto|1|2|4|6] [--path <dir>] [--operator <id>]")
	}
	result, err := application.NewReadOnlyFanoutPlanService(a.store, a.checker).Create(ctx,
		application.CreateReadOnlyFanoutPlanRequest{
			RunID: fs.Arg(0), Goal: fs.Arg(1), ScopePath: *scopePath, Tier: *tier,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
	if err != nil {
		return err
	}
	plan := result.Plan
	fmt.Fprintf(a.out, "fanout_plan: %s\nrun: %s\nstatus: %s\nprotocol: %s\nrequested_tier: %s\neffective_parallelism: %d\nfiles: %d\nexcluded: %d\nshards: %d\nsnapshot_digest: %s\ncapability: workspace_readonly\nshell: false\nfile_write: false\nnetwork: false\nchild_spawn: false\nexecution_authorized: false\nreplayed: %t\n",
		plan.ID, plan.RunID, plan.Status, plan.ProtocolVersion, plan.RequestedTier,
		plan.EffectiveParallelism, plan.FileCount, plan.ExcludedCount, plan.ShardCount,
		plan.SnapshotDigest, result.Replayed)
	return nil
}

func (a *App) runFanoutShow(ctx context.Context, args []string) error {
	fs := newFlagSet("run fanout show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run fanout show <plan-id>")
	}
	plan, err := a.store.GetReadOnlyFanoutPlan(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "fanout_plan: %s\nrun: %s\nworkspace: %s\nstatus: %s\nprotocol: %s\ngoal: %s\nscope: %s\nrequested_tier: %s\neffective_parallelism: %d\nfiles: %d\ntotal_bytes: %d\nexcluded: %d\nshards: %d\nsnapshot_digest: %s\ncapability_fingerprint: %s\ncapability: workspace_readonly\nexecution_authorized: false\ncreated_at: %s\n",
		plan.ID, plan.RunID, plan.WorkspaceID, plan.Status, plan.ProtocolVersion,
		plan.Goal, plan.ScopePath, plan.RequestedTier, plan.EffectiveParallelism,
		plan.FileCount, plan.TotalBytes, plan.ExcludedCount, plan.ShardCount,
		plan.SnapshotDigest, plan.CapabilityFingerprint,
		plan.CreatedAt.Format(time.RFC3339Nano))
	for _, shard := range plan.Shards {
		fmt.Fprintf(a.out, "shard_%d: status=%s files=%d bytes=%d digest=%s\n",
			shard.Ordinal, shard.Status, shard.FileCount, shard.TotalBytes,
			shard.InputDigest)
		for _, file := range plan.Files {
			if file.ShardOrdinal == shard.Ordinal {
				fmt.Fprintf(a.out, "  %d. %s bytes=%d sha256=%s\n", file.Ordinal,
					file.RelativePath, file.SizeBytes, file.ContentSHA256)
			}
		}
	}
	return nil
}

func (a *App) runDelegations(ctx context.Context, args []string) error {
	fs := newFlagSet("run delegations", a.errOut)
	limit := fs.Int("limit", 20, "maximum proposals")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"limit": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run delegations <run-id> [--limit <n>]")
	}
	proposals, err := a.store.ListSpecialistDelegationProposals(ctx, fs.Arg(0), *limit)
	if err != nil {
		return err
	}
	if len(proposals) == 0 {
		fmt.Fprintln(a.out, "no Specialist delegation proposals")
		return nil
	}
	for _, proposal := range proposals {
		reviewStatus := "pending"
		if review, found, err := a.store.GetSpecialistDelegationReviewByProposal(ctx,
			proposal.ID); err != nil {
			return err
		} else if found {
			reviewStatus = string(review.Decision)
		}
		applicationStatus := "none"
		if applied, found, err := a.store.GetSpecialistDelegationApplicationByProposal(ctx,
			proposal.ID); err != nil {
			return err
		} else if found {
			applicationStatus = string(applied.Status)
		}
		fmt.Fprintf(a.out, "%s\tstatus=%s\treview=%s\tapplication=%s\tassignments=%d\troot=%s\tcreated_at=%s\n",
			proposal.ID, proposal.Status, reviewStatus, applicationStatus,
			len(proposal.Spec.Assignments), proposal.RootAgentID,
			proposal.CreatedAt.Format(time.RFC3339Nano))
	}
	return nil
}

func (a *App) runDelegation(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent run delegation [show] <proposal-id> | approve|reject|apply|schedule|continue <proposal-id> --operation-key <key>")
	}
	switch args[0] {
	case "approve", "reject":
		return a.runDelegationReview(ctx, args[0], args[1:])
	case "apply":
		return a.runDelegationApply(ctx, args[1:])
	case "schedule", "continue":
		return a.runDelegationSchedule(ctx, args[0], args[1:])
	case "show":
		return a.runDelegationShow(ctx, args[1:])
	default:
		return a.runDelegationShow(ctx, args)
	}
}

func (a *App) runDelegationShow(ctx context.Context, args []string) error {
	fs := newFlagSet("run delegation", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run delegation <proposal-id>")
	}
	proposal, err := a.store.GetSpecialistDelegationProposal(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "proposal: %s\nrun: %s\nroot_agent: %s\nstatus: %s\nprotocol: %s\nassignments: %d\nproposal_admission_authorized: false\noperator_review_required: true\ncreated_at: %s\n",
		proposal.ID, proposal.RunID, proposal.RootAgentID, proposal.Status,
		proposal.Spec.Version, len(proposal.Spec.Assignments),
		proposal.CreatedAt.Format(time.RFC3339Nano))
	for _, assignment := range proposal.Spec.Assignments {
		fmt.Fprintf(a.out, "%d. %s\n   goal: %s\n   skills: %s\n   budget: turns=%d tokens=%d\n",
			assignment.Ordinal, assignment.Title, assignment.Goal,
			strings.Join(assignment.Skills, ","), assignment.TurnLimit,
			assignment.TokenLimit)
	}
	if review, found, err := a.store.GetSpecialistDelegationReviewByProposal(ctx,
		proposal.ID); err != nil {
		return err
	} else if !found {
		fmt.Fprintln(a.out, "review: pending")
	} else {
		fmt.Fprintf(a.out, "review: %s\nreview_id: %s\nreviewed_by: %s\nreview_reason: %s\nreviewed_at: %s\napplication_required: true\n",
			review.Decision, review.ID, review.ReviewedBy, review.Reason,
			review.CreatedAt.Format(time.RFC3339Nano))
	}
	if applied, found, err := a.store.GetSpecialistDelegationApplicationByProposal(ctx,
		proposal.ID); err != nil {
		return err
	} else if !found {
		fmt.Fprintln(a.out, "application: none")
	} else {
		fmt.Fprintf(a.out, "application: %s\napplication_id: %s\napplication_version: %d\napplication_stop_code: %s\n",
			applied.Status, applied.ID, applied.Version, applied.StopCode)
		for _, assignment := range applied.Assignments {
			fmt.Fprintf(a.out, "application_assignment_%d: status=%s agent=%s message=%s\n",
				assignment.Ordinal, assignment.Status, assignment.AgentID, assignment.MessageID)
		}
		request, requested, err := a.store.
			GetLatestSpecialistOperatorScheduleRequestByApplication(ctx, applied.ID)
		if err != nil {
			return err
		}
		if !requested {
			fmt.Fprintln(a.out, "scheduling_requested: false\nscheduling_started: false")
			return nil
		}
		fmt.Fprintf(a.out, "scheduling_requested: true\nschedule_request_id: %s\nschedule_requested_by: %s\nschedule_agents: %s\nschedule_max_rounds: %d\n",
			request.ID, request.RequestedBy, strings.Join(request.AgentIDs, ","),
			request.MaxRounds)
		schedule, attempt, started, err := a.store.
			GetLatestSpecialistOperatorScheduleAttempt(ctx, request.ID)
		if err != nil {
			return err
		}
		if !started {
			fmt.Fprintln(a.out, "scheduling_started: false\nschedule_status: pending")
			return nil
		}
		fmt.Fprintf(a.out, "scheduling_started: true\nschedule_id: %s\nschedule_status: %s\nschedule_attempt_ordinal: %d\nschedule_rounds_completed: %d\nschedule_turns_started: %d\n",
			schedule.ID, schedule.Status, attempt.Ordinal, schedule.RoundsCompleted,
			schedule.TurnsStarted)
	}
	return nil
}

func (a *App) runDelegationReview(ctx context.Context, action string, args []string) error {
	fs := newFlagSet("run delegation "+action, a.errOut)
	operationKey := fs.String("operation-key", "", "stable review operation key")
	reviewer := fs.String("reviewer", "cli_operator", "reviewer identity")
	reason := fs.String("reason", "", "redacted review reason")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "reviewer": true, "reason": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
		return fmt.Errorf("usage: cyberagent run delegation %s <proposal-id> --operation-key <key> [--reviewer <id>] [--reason <text>]", action)
	}
	decision := domain.SpecialistDelegationApproved
	if action == "reject" {
		decision = domain.SpecialistDelegationRejected
	}
	result, err := application.NewSpecialistDelegationReviewService(a.store).Review(ctx,
		application.ReviewSpecialistDelegationRequest{
			ProposalID: fs.Arg(0), OperationKey: *operationKey, Decision: decision,
			Reason: *reason, ReviewedBy: *reviewer,
		})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "review: %s\nproposal: %s\ndecision: %s\nreviewed_by: %s\nadmission_authorized: false\napplication_required: true\nreplayed: %t\n",
		result.Review.ID, result.Review.ProposalID, result.Review.Decision,
		result.Review.ReviewedBy, result.Replayed)
	return nil
}

func (a *App) runDelegationApply(ctx context.Context, args []string) error {
	fs := newFlagSet("run delegation apply", a.errOut)
	operationKey := fs.String("operation-key", "", "stable application operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
		return errors.New("usage: cyberagent run delegation apply <proposal-id> --operation-key <key> [--operator <id>]")
	}
	service, err := application.NewDefaultSpecialistDelegationApplicationService(a.store, a.checker)
	if err != nil {
		return err
	}
	result, err := service.Apply(ctx, application.ApplySpecialistDelegationRequest{
		ProposalID: fs.Arg(0), OperationKey: *operationKey, RequestedBy: *operator,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "application: %s\nproposal: %s\nstatus: %s\nassignments: %d\nadmission_authorized: true\nscheduling_started: false\nreplayed: %t\nrecovered: %t\n",
		result.Application.ID, result.Application.ProposalID, result.Application.Status,
		result.Application.AssignmentCount, result.Replayed, result.Recovered)
	for _, assignment := range result.Application.Assignments {
		fmt.Fprintf(a.out, "%d. status=%s agent=%s message=%s\n", assignment.Ordinal,
			assignment.Status, assignment.AgentID, assignment.MessageID)
	}
	return nil
}

func (a *App) runDelegationSchedule(ctx context.Context, action string, args []string) error {
	fs := newFlagSet("run delegation "+action, a.errOut)
	operationKey := fs.String("operation-key", "", "stable schedule operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	maxRounds := fs.Int("max-rounds", 1, "bounded schedule rounds")
	var agentIDs stringListFlag
	fs.Var(&agentIDs, "agent", "instructed child Agent id (repeatable)")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true, "max-rounds": true, "agent": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
		return fmt.Errorf("usage: cyberagent run delegation %s <proposal-id> --operation-key <key> [--operator <id>] [--max-rounds <n>] [--agent <id>]", action)
	}
	result, err := application.NewSpecialistOperatorScheduleService(
		a.store, a.router, a.checker).Execute(ctx,
		application.ExecuteSpecialistOperatorScheduleRequest{
			ProposalID: fs.Arg(0), AgentIDs: agentIDs.values, MaxRounds: *maxRounds,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
	if result.Request.ID != "" {
		printSpecialistOperatorScheduleResult(a.out, result)
	}
	return err
}

func printSpecialistOperatorScheduleResult(out interface {
	Write([]byte) (int, error)
}, result application.ExecuteSpecialistOperatorScheduleResult) {
	status := "pending"
	if result.Schedule.ID != "" {
		status = string(result.Schedule.Status)
	}
	fmt.Fprintf(out, "schedule_request: %s\nproposal: %s\nrequested_by: %s\nagents: %s\nmax_rounds: %d\noperator_controlled: true\nschedule: %s\nstatus: %s\nattempt_ordinal: %d\nrounds_completed: %d\nturns_started: %d\nreplayed: %t\nrecovered: %t\n",
		result.Request.ID, result.Request.ProposalID, result.Request.RequestedBy,
		strings.Join(result.Request.AgentIDs, ","), result.Request.MaxRounds,
		result.Schedule.ID, status, result.Attempt.Ordinal,
		result.Schedule.RoundsCompleted, result.Schedule.TurnsStarted,
		result.Replayed, result.Recovered)
}

func (a *App) runExecutionLease(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run lease", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run lease <run-id>")
	}
	_, run, err := service.Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	lease, found, err := a.store.GetRunExecutionLease(ctx, run.ID)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintf(a.out, "run %s has no execution lease\n", run.ID)
		return nil
	}
	now := time.Now().UTC()
	fmt.Fprintf(a.out, "run: %s\nowner: %s\ngeneration: %d\nstatus: %s\nactive: %t\nacquired_at: %s\nrenewed_at: %s\nexpires_at: %s\n",
		lease.RunID, lease.OwnerID, lease.Generation, lease.Status, lease.ActiveAt(now),
		lease.AcquiredAt.Format(time.RFC3339Nano), lease.RenewedAt.Format(time.RFC3339Nano),
		lease.ExpiresAt.Format(time.RFC3339Nano))
	if lease.ReleasedAt != nil {
		fmt.Fprintf(a.out, "released_at: %s\n", lease.ReleasedAt.Format(time.RFC3339Nano))
	}
	return nil
}

func (a *App) runUsage(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run usage", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run usage <run-id>")
	}
	_, run, err := service.Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	toolUsage, err := a.store.GetToolCallUsage(ctx, run.ID)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run: %s\nstatus: %s\ntool_calls: %d\ntool_call_limit: %d\ntool_calls_remaining: %d\n",
		run.ID, run.Status, toolUsage.Consumed, toolUsage.Limit, toolUsage.Remaining)
	if toolUsage.ExhaustedAt != nil {
		fmt.Fprintf(a.out, "tool_budget_exhausted_at: %s\n", toolUsage.ExhaustedAt.Format(time.RFC3339))
	}
	agentUsage, err := a.store.GetRunAgentUsage(ctx, run.ID)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "agent_root_tokens: %d\nagent_specialist_tokens: %d\nagent_readonly_fanout_tokens: %d\nagent_total_tokens: %d\nagent_root_execution_millis: %d\nagent_specialist_execution_millis: %d\nagent_readonly_fanout_millis: %d\nagent_total_execution_millis: %d\n",
		agentUsage.RootTokens, agentUsage.SpecialistTokens,
		agentUsage.ReadOnlyFanoutTokens, agentUsage.TotalTokens,
		agentUsage.RootExecutionMillis, agentUsage.SpecialistExecutionMillis,
		agentUsage.ReadOnlyFanoutMillis, agentUsage.TotalExecutionMillis)
	if checkpoint, ok, err := a.newRunSupervisor().Checkpoint(ctx, run.ID); err != nil {
		return err
	} else if ok {
		fmt.Fprintf(a.out, "turns_completed: %d\ninput_tokens: %d\noutput_tokens: %d\ntotal_tokens: %d\nexecution_millis: %d\n",
			checkpoint.NextTurn-1, checkpoint.InputTokens, checkpoint.OutputTokens,
			checkpoint.TotalTokens, checkpoint.ExecutionMillis)
	}
	return nil
}

func (a *App) runSupervisorStep(ctx context.Context, args []string) error {
	fs := newFlagSet("run step", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run step <run-id>")
	}
	result, err := a.newRunSupervisor().Step(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run %s turn %d completed\nagent: %s\nattempt: %s\nrecovered: %t\nmodel_attempts: %d\nprotocol_repairs: %d\ntool_rounds: %d\ntool_calls: %d\nstream_events: %d\nstream_bytes: %d\nmodel_outcome: %s\naction: %s\nrun_status: %s\nprovider: %s\nmodel: %s\nusage: input=%d output=%d total=%d\ncumulative_tokens: %d\nexecution_millis: %d\nnext_turn: %d\nresponse: %s\n",
		result.Handle.RunID, result.Turn, result.AgentID, result.AttemptID, result.Recovered, result.ModelAttempts,
		result.ProtocolRepairs, result.ToolRounds, result.ToolCalls, result.StreamEvents, result.StreamBytes,
		result.ModelOutcome, result.Action.Kind, result.RunStatus,
		result.Provider, result.Model,
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.TotalTokens,
		result.Checkpoint.TotalTokens, result.Checkpoint.ExecutionMillis, result.Checkpoint.NextTurn, result.Text)
	return nil
}

func (a *App) runAgentGraph(ctx context.Context, args []string) error {
	fs := newFlagSet("run graph", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run graph <run-id>")
	}
	service := coordinator.New(a.store)
	if _, _, err := service.RegisterRoot(ctx, fs.Arg(0)); err != nil {
		return err
	}
	graph, err := service.Restore(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run: %s\nroot_agent: %s\nnodes: %d\npending_messages: %d\nsnapshot_version: %d\nsnapshot_protocol: %s\n",
		graph.RunID, graph.RootAgentID, len(graph.Nodes), len(graph.PendingMessages),
		graph.LatestSnapshot.Version, graph.LatestSnapshot.ProtocolVersion)
	for _, node := range graph.Nodes {
		fmt.Fprintf(a.out, "%s\trole=%s\tstatus=%s\tprofile=%s\tdepth=%d\tchildren=%d\tturns=%d/%d\ttokens=%d/%d\tversion=%d\n",
			node.ID, node.Role, node.Status, node.Profile, node.Depth, node.ChildLimit,
			node.TurnsUsed, node.TurnLimit, node.TokensUsed, node.TokenLimit, node.Version)
	}
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
	supervisor := a.newRunSupervisor()
	result, err := supervisor.Execute(ctx, fs.Arg(0), *maxSteps)
	for _, step := range result.Steps {
		fmt.Fprintf(a.out, "turn %d\t%s\t%s/%s\tattempts=%d\trepairs=%d\ttool_rounds=%d\ttool_calls=%d\tstream_events=%d\ttokens=%d\tnext=%d\n",
			step.Turn, step.Action.Kind, step.Provider, step.Model, step.ModelAttempts, step.ProtocolRepairs,
			step.ToolRounds, step.ToolCalls, step.StreamEvents, step.Usage.TotalTokens, step.Checkpoint.NextTurn)
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
	result, err := a.newRunSupervisor().Finalize(ctx, fs.Arg(0), outcome, *text)
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
	checkpoint, ok, err := a.newRunSupervisor().Checkpoint(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintf(a.out, "run %s has no supervisor checkpoint\n", fs.Arg(0))
		return nil
	}
	toolUsage, err := a.store.GetToolCallUsage(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run: %s\nphase: %s\nnext_turn: %d\nattempt: %s\nrepair_phase: %s\nrepair_reason: %s\nlast_error: %s\ninput_tokens: %d\noutput_tokens: %d\ntotal_tokens: %d\ntool_calls: %d\ntool_call_limit: %d\ntool_calls_remaining: %d\nexecution_millis: %d\nupdated_at: %s\n",
		checkpoint.RunID, checkpoint.Phase, checkpoint.NextTurn, checkpoint.AttemptID,
		checkpoint.RepairPhase, checkpoint.RepairReason, checkpoint.LastError, checkpoint.InputTokens, checkpoint.OutputTokens,
		checkpoint.TotalTokens, toolUsage.Consumed, toolUsage.Limit, toolUsage.Remaining,
		checkpoint.ExecutionMillis, checkpoint.UpdatedAt.Format(time.RFC3339))
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
	surface := fs.String("surface", string(domain.ExecutionSurfaceCode), "execution surface: code or cyber")
	phase := fs.String("phase", string(domain.ExecutionPhaseDeliver), "execution phase: plan or deliver")
	route := fs.String("route", "", "model route")
	sessionID := fs.String("session", "", "existing session id")
	interactive := fs.Bool("interactive", false, "mark run as interactive")
	maxTurns := fs.Int("max-turns", domain.DefaultBudget().MaxTurns, "maximum agent turns")
	maxTokens := fs.Int64("max-tokens", 0, "maximum model tokens; zero means unset")
	maxToolCalls := fs.Int64("max-tool-calls", domain.DefaultBudget().MaxToolCalls, "maximum tool calls; zero means unlimited")
	maxCostUSD := fs.Float64("max-cost-usd", 0, "maximum model cost in USD; zero means unset")
	timeout := fs.Duration("timeout", 0, "run timeout; zero means unset")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"workspace":      true,
		"profile":        true,
		"surface":        true,
		"phase":          true,
		"route":          true,
		"session":        true,
		"interactive":    false,
		"max-turns":      true,
		"max-tokens":     true,
		"max-tool-calls": true,
		"max-cost-usd":   true,
		"timeout":        true,
	})); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New(`usage: cyberagent run create "goal" [--workspace <name>] [--profile code|review|learn|script] [--surface code|cyber] [--phase plan|deliver]`)
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
		Surface:     *surface,
		Phase:       *phase,
		WorkspaceID: workspaceID,
		SessionID:   *sessionID,
		ModelRoute:  *route,
		Interactive: *interactive,
		Budget: domain.Budget{
			MaxTurns:       *maxTurns,
			MaxTokens:      *maxTokens,
			MaxToolCalls:   *maxToolCalls,
			MaxCostUSD:     *maxCostUSD,
			TimeoutSeconds: int64(timeout.Seconds()),
		},
	})
	if err != nil {
		return err
	}
	mode, err := service.Mode(ctx, run.ID)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run %s created\nmission: %s\nsession: %s\nstatus: %s\nprofile: %s\nsurface: %s\nphase: %s\nmode_revision: %d\nworkspace: %s\nroute: %s\n",
		run.ID, mission.ID, run.SessionID, run.Status, mission.Profile, mode.Surface,
		mode.Phase, mode.Revision, mission.WorkspaceID, run.Config.ModelRoute)
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
		mode, err := service.Mode(ctx, run.ID)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "%s\t%s\t%s/%s\t%s\t%s\t%s\n", run.ID, run.Status,
			mode.Surface, mode.Phase, run.MissionID, run.Config.ModelRoute,
			run.UpdatedAt.Format(time.RFC3339))
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
	mode, err := service.Mode(ctx, run.ID)
	if err != nil {
		return err
	}
	scope, _ := json.Marshal(mission.Scope)
	budget, _ := json.Marshal(run.Budget)
	fmt.Fprintf(a.out, "id: %s\nmission: %s\nstatus: %s\ngoal: %s\nprofile: %s\nsurface: %s\nphase: %s\nmode_revision: %d\nmode_policy: %s\nworkspace: %s\nsession: %s\nroute: %s\ninteractive: %t\nscope: %s\nbudget: %s\ncreated_at: %s\nupdated_at: %s\n",
		run.ID, mission.ID, run.Status, mission.Goal, mission.Profile, mode.Surface,
		mode.Phase, mode.Revision, mode.PolicyVersion, mission.WorkspaceID, run.SessionID,
		run.Config.ModelRoute, run.Config.Interactive, scope, budget, run.CreatedAt.Format(time.RFC3339), run.UpdatedAt.Format(time.RFC3339))
	if run.StartedAt != nil {
		fmt.Fprintf(a.out, "started_at: %s\n", run.StartedAt.Format(time.RFC3339))
	}
	if run.FinishedAt != nil {
		fmt.Fprintf(a.out, "finished_at: %s\n", run.FinishedAt.Format(time.RFC3339))
	}
	return nil
}

func (a *App) runMode(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run mode", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent run mode <run-id>")
	}
	mode, err := service.Mode(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run: %s\nmission: %s\nprotocol: %s\nrevision: %d\nsurface: %s\nphase: %s\nprofile: %s\npolicy: %s\nnetwork_mode: %s\nallowed_target_count: %d\nrequested_by: %s\nreason: %s\ncreated_at: %s\ncapability_grant: false\n",
		mode.RunID, mode.MissionID, mode.ProtocolVersion, mode.Revision, mode.Surface,
		mode.Phase, mode.Profile, mode.PolicyVersion, mode.Scope.NetworkMode,
		len(mode.Scope.AllowedTargets), mode.RequestedBy, mode.Reason,
		mode.CreatedAt.Format(time.RFC3339Nano))
	return nil
}

func (a *App) runPhase(ctx context.Context, service *application.RunService, args []string) error {
	fs := newFlagSet("run phase", a.errOut)
	operationKey := fs.String("operation-key", "", "stable phase transition operation key")
	operator := fs.String("operator", "cli_operator", "operator identity")
	reason := fs.String("reason", "", "redacted transition reason")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"operation-key": true, "operator": true, "reason": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 2 || strings.TrimSpace(*operationKey) == "" {
		return errors.New("usage: cyberagent run phase <run-id> plan|deliver --operation-key <key> [--operator <id>] [--reason <text>]")
	}
	result, err := service.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: fs.Arg(0), Phase: fs.Arg(1), OperationKey: *operationKey,
		RequestedBy: *operator, Reason: *reason,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "run: %s\nsurface: %s\nphase: %s\nrevision: %d\npolicy: %s\nrequested_by: %s\nreplayed: %t\ncapability_grant: false\n",
		result.Mode.RunID, result.Mode.Surface, result.Mode.Phase, result.Mode.Revision,
		result.Mode.PolicyVersion, result.Mode.RequestedBy, result.Replayed)
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
