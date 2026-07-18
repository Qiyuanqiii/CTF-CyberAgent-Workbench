package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/scriptprocess"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/toolrun"
)

type toolListRow struct {
	id        string
	status    string
	tool      string
	preview   string
	updatedAt time.Time
}

func (a *App) toolCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("tool subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	switch args[0] {
	case "schema":
		return a.toolSchema(args[1:])
	case "invoke":
		return a.toolInvoke(ctx, args[1:])
	case "list":
		return a.toolList(ctx, args[1:])
	case "show":
		return a.toolShow(ctx, args[1:])
	case "approve":
		return a.toolApprove(ctx, args[1:])
	case "deny":
		return a.toolDeny(ctx, args[1:])
	default:
		return fmt.Errorf("unknown tool subcommand %q", args[0])
	}
}

func (a *App) toolSchema(args []string) error {
	fs := newFlagSet("tool schema", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("usage: cyberagent tool schema [work_item_create|note_create|specialist_delegation_propose|plan_delivery_propose]")
	}
	var value any = toolgateway.AllSupervisorToolDefinitions()
	if fs.NArg() == 1 {
		definition, found := toolgateway.SupervisorToolDefinition(toolgateway.ToolName(fs.Arg(0)))
		if !found {
			return fmt.Errorf("supervisor tool %q was not found", fs.Arg(0))
		}
		value = definition
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, string(encoded))
	return nil
}

func (a *App) toolInvoke(ctx context.Context, args []string) error {
	fs := newFlagSet("tool invoke", a.errOut)
	runID := fs.String("run", "", "Run id")
	operationKey := fs.String("operation-key", "", "stable idempotency key")
	payload := fs.String("payload", "", "JSON tool payload")
	payloadFile := fs.String("payload-file", "", "UTF-8 file containing the JSON tool payload")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"run": true, "operation-key": true, "payload": true, "payload-file": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*runID) == "" || strings.TrimSpace(*operationKey) == "" ||
		(flagWasSet(fs, "payload") == flagWasSet(fs, "payload-file")) {
		return errors.New("usage: cyberagent tool invoke <tool> --run <id> --operation-key <key> (--payload <json> | --payload-file <path>)")
	}
	tool := toolgateway.ToolName(strings.TrimSpace(fs.Arg(0)))
	if _, found := toolgateway.StructuredMemoryToolDefinition(tool); !found {
		return fmt.Errorf("tool %q is not an invocable structured memory tool", tool)
	}
	rawPayload := []byte(*payload)
	if flagWasSet(fs, "payload-file") {
		var err error
		rawPayload, err = readStructuredToolPayload(*payloadFile)
		if err != nil {
			return err
		}
	}
	run, err := a.store.GetRun(ctx, strings.TrimSpace(*runID))
	if err != nil {
		return err
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return errors.New("structured tool Run must have an attached Session")
	}
	mission, err := a.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return err
	}
	root, _, err := a.store.RegisterRootAgent(ctx, run.ID)
	if err != nil {
		return err
	}
	outcome, err := a.newToolGateway().Invoke(ctx, toolgateway.ToolCall{
		Name: tool, Payload: json.RawMessage(rawPayload), OperationKey: *operationKey,
		RunID: run.ID, AgentID: root.ID, SessionID: run.SessionID,
		WorkspaceID: mission.WorkspaceID, RequestedBy: "cli",
	})
	if err != nil {
		return err
	}
	if outcome.Result == nil {
		return errors.New("structured tool invocation did not return a result")
	}
	if !outcome.Decision.Allowed {
		fmt.Fprintf(a.out, "tool %s denied\nrun: %s\nreason: %s\n", tool, run.ID, outcome.Decision.Reason)
		return apperror.New(apperror.CodePolicyDenied,
			"policy denied structured tool invocation: "+outcome.Decision.Reason)
	}
	fmt.Fprintf(a.out, "tool %s %s\nrun: %s\ndecision: %s\n", tool, outcome.Result.Status, run.ID, outcome.Decision.Approval)
	keys := make([]string, 0, len(outcome.Result.Metadata))
	for key := range outcome.Result.Metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(a.out, "%s: %s\n", key, outcome.Result.Metadata[key])
	}
	return nil
}

func readStructuredToolPayload(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("structured tool payload file path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("structured tool payload file %s is a directory", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, toolgateway.MaxStructuredMemoryPayloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > toolgateway.MaxStructuredMemoryPayloadBytes {
		return nil, fmt.Errorf("structured tool payload exceeds %d bytes", toolgateway.MaxStructuredMemoryPayloadBytes)
	}
	if !utf8.Valid(data) {
		return nil, errors.New("structured tool payload file is not valid UTF-8")
	}
	return data, nil
}

func (a *App) toolList(ctx context.Context, args []string) error {
	fs := newFlagSet("tool list", a.errOut)
	sessionID := fs.String("session", "", "session id")
	status := fs.String("status", "", "proposal status")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"session": true, "status": true})); err != nil {
		return err
	}
	if !validToolProposalStatus(strings.TrimSpace(*status)) {
		return fmt.Errorf("invalid tool proposal status %q", strings.TrimSpace(*status))
	}
	gateway := a.newToolGateway()
	runs, err := gateway.ToolRuns().List(ctx, toolrun.ListFilter{SessionID: *sessionID, Status: *status})
	if err != nil {
		return err
	}
	rows := make([]toolListRow, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, toolListRow{
			id: run.ID, status: run.Status, tool: run.ToolName, preview: run.Command, updatedAt: run.UpdatedAt,
		})
	}
	processStatus := scriptprocess.Status(strings.TrimSpace(*status))
	if processStatus == "" || processStatus.Valid() {
		processes, err := gateway.ScriptProcesses().List(ctx, scriptprocess.ListFilter{
			SessionID: *sessionID, Status: processStatus,
		})
		if err != nil {
			return err
		}
		for _, process := range processes {
			rows = append(rows, toolListRow{
				id: process.ID, status: string(process.Status), tool: string(toolgateway.ScriptProcessTool),
				preview: scriptProcessPreview(process), updatedAt: process.UpdatedAt,
			})
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(a.out, "no tool proposals")
		return nil
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].updatedAt.Equal(rows[j].updatedAt) {
			return rows[i].id > rows[j].id
		}
		return rows[i].updatedAt.After(rows[j].updatedAt)
	})
	for _, row := range rows {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\n", row.id, row.status, row.tool, row.preview)
	}
	return nil
}

func validToolProposalStatus(status string) bool {
	switch status {
	case "", toolrun.StatusProposed, toolrun.StatusApproved, toolrun.StatusDenied,
		toolrun.StatusRunning, toolrun.StatusCompleted, toolrun.StatusFailed:
		return true
	default:
		return false
	}
}

func (a *App) toolShow(ctx context.Context, args []string) error {
	fs := newFlagSet("tool show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool show <proposal-id>")
	}
	tool, err := a.proposalTool(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	gateway := a.newToolGateway()
	switch tool {
	case toolgateway.ShellTool:
		run, err := gateway.ToolRuns().Get(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printToolRun(a.out, run)
		return a.printSourceArtifacts(ctx, run.ID)
	case toolgateway.ScriptProcessTool:
		process, err := gateway.ScriptProcesses().Get(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		if err := printScriptProcess(a.out, process); err != nil {
			return err
		}
		return a.printSourceArtifacts(ctx, process.ID)
	case toolgateway.ReplaceFileTool:
		return errors.New("file edit proposals are inspected with `cyberagent edit show`")
	default:
		return fmt.Errorf("unsupported proposal tool %q", tool)
	}
}

func (a *App) toolApprove(ctx context.Context, args []string) error {
	fs := newFlagSet("tool approve", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool approve <proposal-id>")
	}
	return a.reviewToolProposal(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, ProposalID: fs.Arg(0), ReviewedBy: "cli",
	})
}

func (a *App) toolDeny(ctx context.Context, args []string) error {
	fs := newFlagSet("tool deny", a.errOut)
	reason := fs.String("reason", "", "denial reason")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"reason": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent tool deny <proposal-id> [--reason <reason>]")
	}
	return a.reviewToolProposal(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewDeny, ProposalID: fs.Arg(0), ReviewedBy: "cli", Reason: *reason,
	})
}

func (a *App) reviewToolProposal(ctx context.Context, request toolgateway.ReviewRequest) error {
	tool, err := a.proposalTool(ctx, request.ProposalID)
	if err != nil {
		return err
	}
	if tool == toolgateway.ReplaceFileTool {
		return errors.New("file edits use `cyberagent edit review-approve|review-deny`; approved Run-bound edits require a separate `cyberagent edit apply`")
	}
	request.Tool = tool
	outcome, err := a.newToolGateway().Review(ctx, request)
	if err != nil {
		return err
	}
	if outcome.Proposal == nil {
		return errors.New("tool review did not return a proposal")
	}
	label := "tool run"
	if tool == toolgateway.ScriptProcessTool {
		label = "script process"
	}
	fmt.Fprintf(a.out, "%s %s %s\n", label, outcome.Proposal.ID, outcome.Proposal.Status)
	if outcome.Result != nil && strings.TrimSpace(outcome.Result.Stdout) != "" {
		fmt.Fprintln(a.out, outcome.Result.Stdout)
	}
	if outcome.Result != nil && strings.TrimSpace(outcome.Result.Stderr) != "" {
		fmt.Fprintln(a.errOut, outcome.Result.Stderr)
	}
	if outcome.Result != nil {
		keys := make([]string, 0, len(outcome.Result.Metadata))
		for key := range outcome.Result.Metadata {
			if strings.HasPrefix(key, "artifact_") {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(a.out, "%s: %s\n", key, outcome.Result.Metadata[key])
		}
	}
	if request.Action == toolgateway.ReviewDeny && strings.TrimSpace(outcome.Decision.Reason) != "" {
		fmt.Fprintf(a.out, "reason: %s\n", outcome.Decision.Reason)
	}
	return nil
}

func (a *App) printSourceArtifacts(ctx context.Context, sourceID string) error {
	descriptors, err := a.newToolGateway().Artifacts().List(ctx, artifact.ListFilter{SourceID: sourceID})
	if err != nil {
		return err
	}
	for _, descriptor := range descriptors {
		fmt.Fprintf(a.out, "artifact_%s: %s\n", descriptor.Stream, descriptor.ID)
	}
	return nil
}

func (a *App) proposalTool(ctx context.Context, proposalID string) (toolgateway.ToolName, error) {
	record, err := a.store.GetApprovalByProposal(ctx, strings.TrimSpace(proposalID))
	if err != nil {
		return "", err
	}
	return toolgateway.ToolName(record.ToolName), nil
}

func scriptProcessPreview(process scriptprocess.Process) string {
	arguments, _ := json.Marshal(process.Arguments)
	return fmt.Sprintf("%s %s", process.Executable, arguments)
}

func printScriptProcess(out interface{ Write([]byte) (int, error) }, process scriptprocess.Process) error {
	arguments, err := json.Marshal(process.Arguments)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "id: %s\n", process.ID)
	fmt.Fprintf(out, "schema: %s\n", scriptprocess.Schema)
	fmt.Fprintf(out, "status: %s\n", process.Status)
	fmt.Fprintf(out, "tool: %s\n", toolgateway.ScriptProcessTool)
	fmt.Fprintf(out, "run: %s\n", process.RunID)
	fmt.Fprintf(out, "session: %s\n", process.SessionID)
	fmt.Fprintf(out, "workspace: %s\n", process.WorkspaceID)
	fmt.Fprintf(out, "executable: %s\n", process.Executable)
	fmt.Fprintf(out, "arguments: %s\n", arguments)
	fmt.Fprintf(out, "working_directory: %s\n", process.WorkingDirectory)
	fmt.Fprintf(out, "requested_backend: %s\n", process.RequestedBackend)
	fmt.Fprintf(out, "execution_mode: %s\n", process.ExecutionMode)
	fmt.Fprintf(out, "risk: %s\n", process.Risk)
	fmt.Fprintf(out, "policy_reason: %s\n", process.PolicyReason)
	if strings.TrimSpace(process.Stdout) != "" {
		fmt.Fprintf(out, "stdout: %s\n", process.Stdout)
	}
	if strings.TrimSpace(process.Stderr) != "" {
		fmt.Fprintf(out, "stderr: %s\n", process.Stderr)
	}
	fmt.Fprintf(out, "exit_code: %d\n", process.ExitCode)
	fmt.Fprintf(out, "created_at: %s\n", process.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "updated_at: %s\n", process.UpdatedAt.Format(time.RFC3339))
	return nil
}

func printToolRun(out interface{ Write([]byte) (int, error) }, run toolrun.ToolRun) {
	fmt.Fprintf(out, "id: %s\n", run.ID)
	fmt.Fprintf(out, "status: %s\n", run.Status)
	fmt.Fprintf(out, "tool: %s\n", run.ToolName)
	fmt.Fprintf(out, "session: %s\n", run.SessionID)
	fmt.Fprintf(out, "workspace: %s\n", run.WorkspaceID)
	fmt.Fprintf(out, "command: %s\n", run.Command)
	if strings.TrimSpace(run.Risk) != "" {
		fmt.Fprintf(out, "risk: %s\n", run.Risk)
	}
	if strings.TrimSpace(run.PolicyReason) != "" {
		fmt.Fprintf(out, "policy_reason: %s\n", run.PolicyReason)
	}
	if strings.TrimSpace(run.Stdout) != "" {
		fmt.Fprintf(out, "stdout: %s\n", run.Stdout)
	}
	if strings.TrimSpace(run.Stderr) != "" {
		fmt.Fprintf(out, "stderr: %s\n", run.Stderr)
	}
	fmt.Fprintf(out, "exit_code: %d\n", run.ExitCode)
	fmt.Fprintf(out, "created_at: %s\n", run.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "updated_at: %s\n", run.UpdatedAt.Format(time.RFC3339))
}
