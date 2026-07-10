package toolgateway

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/toolrun"
	"cyberagent-workbench/internal/tools"
)

const maxWorkspaceListDepth = 32

type Store interface {
	toolrun.Store
	fileedit.Store
}

type Gateway struct {
	store                 Store
	checker               policy.Checker
	workspaceRootResolver WorkspaceRootResolver
	legacyTools           *toolrun.Manager
	legacyEdits           *fileedit.Manager
}

type WorkspaceRootResolver func(ctx context.Context, workspaceID string) (string, error)

func New(store Store, checker policy.Checker) *Gateway {
	if checker == nil {
		fallback := policy.NewDefaultChecker()
		checker = fallback
	}
	return &Gateway{
		store: store, checker: checker,
		legacyTools: toolrun.NewManager(store, checker), legacyEdits: fileedit.NewManager(store),
	}
}

func (g *Gateway) WithWorkspaceRootResolver(resolver WorkspaceRootResolver) *Gateway {
	if g != nil {
		g.workspaceRootResolver = resolver
	}
	return g
}

func (g *Gateway) Invoke(ctx context.Context, call ToolCall) (Outcome, error) {
	if g == nil {
		return Outcome{}, errors.New("tool gateway is required")
	}
	normalized, err := NormalizeToolCall(call)
	if err != nil {
		return Outcome{}, err
	}
	if err := validateToolArguments(normalized); err != nil {
		return Outcome{}, err
	}
	switch normalized.Name {
	case ReadFileTool, ListWorkspaceTool:
		return g.invokeWorkspaceRead(ctx, normalized)
	case ShellTool:
		return g.invokeShellProposal(ctx, normalized)
	case ReplaceFileTool:
		return g.invokeFileEditProposal(ctx, normalized)
	default:
		return Outcome{}, fmt.Errorf("unsupported tool %q", normalized.Name)
	}
}

func (g *Gateway) Review(ctx context.Context, request ReviewRequest) (Outcome, error) {
	if g == nil || g.store == nil {
		return Outcome{}, errors.New("tool gateway store is required")
	}
	normalized, err := NormalizeReviewRequest(request)
	if err != nil {
		return Outcome{}, err
	}
	switch normalized.Tool {
	case ShellTool:
		return g.reviewShell(ctx, normalized)
	case ReplaceFileTool:
		return g.reviewFileEdit(ctx, normalized)
	default:
		return Outcome{}, fmt.Errorf("tool %q does not support review", normalized.Tool)
	}
}

func (g *Gateway) invokeWorkspaceRead(ctx context.Context, call ToolCall) (Outcome, error) {
	if call.WorkspaceID == "" {
		return Outcome{}, errors.New("workspace read requires a workspace id")
	}
	root, err := g.bindWorkspaceRoot(ctx, call.WorkspaceID, call.WorkspaceRoot)
	if err != nil {
		return Outcome{}, err
	}
	call.WorkspaceRoot = root
	toolCall := tools.Call{Name: string(call.Name), Args: cloneArguments(call.Arguments), WorkingDir: call.WorkspaceRoot}
	policyDecision := g.checker.CheckToolCall(toolCall)
	if !policyDecision.Allowed {
		return deniedOutcome(call, policyDecision)
	}
	if policyDecision.NeedsApproval {
		policyDecision.Allowed = false
		policyDecision.Risk = defaultRisk(policyDecision.Risk, "medium")
		policyDecision.Reason = "scoped read required approval and was not executed: " + policyDecision.Reason
		return deniedOutcome(call, policyDecision)
	}
	decision, err := gatewayDecision(policyDecision, ApprovalAutomatic, "low")
	if err != nil {
		return Outcome{}, err
	}
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(call.WorkspaceRoot))
	registry.Register(tools.NewListWorkspaceTool(call.WorkspaceRoot))
	started := time.Now().UTC()
	legacyResult, runErr := registry.Run(ctx, toolCall)
	completed := time.Now().UTC()
	status := StatusCompleted
	if runErr != nil {
		status = StatusFailed
		if strings.TrimSpace(legacyResult.Stderr) == "" {
			legacyResult.Stderr = runErr.Error()
		}
	}
	stdout, stdoutTruncated := boundResultText(legacyResult.Stdout, MaxResultStdoutBytes)
	stderr, stderrTruncated := boundResultText(legacyResult.Stderr, MaxResultStderrBytes)
	mime := strings.TrimSpace(legacyResult.MIME)
	if mime == "" {
		mime = "text/plain; charset=utf-8"
	}
	outcome := Outcome{
		Call: safeToolCall(call), Decision: decision,
		Execution: &Execution{Backend: "workspace_fs", Status: status, StartedAt: started, CompletedAt: &completed},
		Result: &Result{
			Status: status, Stdout: stdout, Stderr: stderr, ExitCode: legacyResult.ExitCode, MIME: mime,
			Truncated: legacyResult.Truncated || stdoutTruncated || stderrTruncated, CompletedAt: completed,
		},
	}
	return validateOutcome(outcome, runErr)
}

func (g *Gateway) invokeShellProposal(ctx context.Context, call ToolCall) (Outcome, error) {
	if g.store == nil {
		return Outcome{}, errors.New("tool gateway store is required")
	}
	if call.SessionID == "" {
		return Outcome{}, errors.New("shell proposal requires a session id")
	}
	run, err := g.legacyTools.ProposeShell(ctx, call.SessionID, call.WorkspaceID, call.Arguments["command"])
	if err != nil {
		return Outcome{}, err
	}
	return g.outcomeFromToolRun(call, run, nil)
}

func (g *Gateway) invokeFileEditProposal(ctx context.Context, call ToolCall) (Outcome, error) {
	if g.store == nil {
		return Outcome{}, errors.New("tool gateway store is required")
	}
	if call.WorkspaceID == "" {
		return Outcome{}, errors.New("file edit proposal requires a workspace id")
	}
	root, err := g.bindWorkspaceRoot(ctx, call.WorkspaceID, call.WorkspaceRoot)
	if err != nil {
		return Outcome{}, err
	}
	call.WorkspaceRoot = root
	policyDecision := g.checker.CheckToolCall(tools.Call{
		Name: string(call.Name), Args: cloneArguments(call.Arguments), WorkingDir: call.WorkspaceRoot,
	})
	if !policyDecision.Allowed {
		return deniedOutcome(call, policyDecision)
	}
	edit, err := g.legacyEdits.Propose(ctx, fileedit.Proposal{
		SessionID: call.SessionID, WorkspaceID: call.WorkspaceID, WorkspaceRoot: call.WorkspaceRoot,
		Path: call.Arguments["path"], ProposedText: call.Arguments["content"],
	})
	if err != nil {
		return Outcome{}, err
	}
	return g.outcomeFromFileEdit(call, edit, nil)
}

func (g *Gateway) reviewShell(ctx context.Context, request ReviewRequest) (Outcome, error) {
	before, err := g.legacyTools.Get(ctx, request.ProposalID)
	if err != nil {
		return Outcome{}, err
	}
	var after toolrun.ToolRun
	if request.Action == ReviewApprove {
		after, err = g.legacyTools.Approve(ctx, request.ProposalID)
	} else {
		reason := request.Reason
		if reason == "" {
			reason = "denied by operator"
		}
		after, err = g.legacyTools.Deny(ctx, request.ProposalID, reason)
	}
	if after.ID == "" {
		after = before
	}
	call := ToolCall{
		Name: ShellTool, Arguments: map[string]string{"command": before.Command},
		SessionID: before.SessionID, WorkspaceID: before.WorkspaceID, RequestedBy: "approval_service",
	}
	return g.outcomeFromToolRun(call, after, err)
}

func (g *Gateway) reviewFileEdit(ctx context.Context, request ReviewRequest) (Outcome, error) {
	before, err := g.legacyEdits.Get(ctx, request.ProposalID)
	if err != nil {
		return Outcome{}, err
	}
	var after fileedit.Edit
	if request.Action == ReviewApprove {
		root, bindErr := g.bindWorkspaceRoot(ctx, before.WorkspaceID, request.WorkspaceRoot)
		if bindErr != nil {
			return Outcome{}, bindErr
		}
		after, err = g.legacyEdits.Approve(ctx, request.ProposalID, root)
	} else {
		reason := request.Reason
		if reason == "" {
			reason = "denied by operator"
		}
		after, err = g.legacyEdits.Deny(ctx, request.ProposalID, reason)
	}
	if after.ID == "" {
		after = before
	}
	call := ToolCall{
		Name: ReplaceFileTool, Arguments: map[string]string{"path": before.Path, "content": before.ProposedText},
		SessionID: before.SessionID, WorkspaceID: before.WorkspaceID, RequestedBy: "approval_service",
	}
	return g.outcomeFromFileEdit(call, after, err)
}

func (g *Gateway) outcomeFromToolRun(call ToolCall, run toolrun.ToolRun, operationErr error) (Outcome, error) {
	if run.ID == "" {
		return Outcome{}, errors.Join(operationErr, errors.New("tool run record is required"))
	}
	call.Name = ShellTool
	call.SessionID = run.SessionID
	call.WorkspaceID = run.WorkspaceID
	call.Arguments = map[string]string{"command": run.Command}
	call.WorkspaceRoot = ""
	call, err := NormalizeToolCall(call)
	if err != nil {
		return Outcome{}, errors.Join(operationErr, err)
	}
	allowed := run.Status != toolrun.StatusDenied
	mode := ApprovalPerCall
	if run.Status == toolrun.StatusDenied {
		mode = ApprovalNever
	}
	risk := defaultRisk(run.Risk, "medium")
	reason := strings.TrimSpace(run.PolicyReason)
	if run.Status == toolrun.StatusDenied && (reason == "" || strings.HasPrefix(strings.ToLower(reason), "allowed")) {
		reason = "denied by operator"
	}
	if reason == "" {
		reason = "shell commands require explicit per-call approval"
	}
	decision, err := NormalizeDecision(Decision{Allowed: allowed, Approval: mode, Risk: risk, Reason: reason})
	if err != nil {
		return Outcome{}, errors.Join(operationErr, err)
	}
	status, ok := gatewayToolRunStatus(run.Status)
	if !ok {
		return Outcome{}, errors.Join(operationErr, fmt.Errorf("unsupported tool run status %q", run.Status))
	}
	proposalStatus := status
	if status == StatusRunning {
		proposalStatus = StatusApproved
	}
	proposal := &Proposal{
		ID: run.ID, Tool: ShellTool, Class: ClassShell, Status: proposalStatus,
		Preview: boundedPreview(run.Command), CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}
	outcome := Outcome{Call: safeToolCall(call), Decision: decision, Proposal: proposal}
	if status == StatusCompleted || status == StatusFailed {
		completed := run.UpdatedAt
		started := completed
		stdout, stdoutTruncated := boundResultText(run.Stdout, MaxResultStdoutBytes)
		stderr, stderrTruncated := boundResultText(run.Stderr, MaxResultStderrBytes)
		outcome.Execution = &Execution{Backend: "dry_run", Status: status, StartedAt: started, CompletedAt: &completed}
		outcome.Result = &Result{
			Status: status, Stdout: stdout, Stderr: stderr, ExitCode: run.ExitCode,
			MIME: "text/plain; charset=utf-8", Truncated: stdoutTruncated || stderrTruncated, CompletedAt: completed,
		}
	} else if status == StatusRunning {
		outcome.Execution = &Execution{Backend: "sandbox_pending", Status: StatusRunning, StartedAt: run.UpdatedAt}
	} else if status == StatusDenied {
		outcome.Result = deniedResult(decision.Reason, run.UpdatedAt)
	}
	return validateOutcome(outcome, operationErr)
}

func (g *Gateway) outcomeFromFileEdit(call ToolCall, edit fileedit.Edit, operationErr error) (Outcome, error) {
	if edit.ID == "" {
		return Outcome{}, errors.Join(operationErr, errors.New("file edit record is required"))
	}
	call.Name = ReplaceFileTool
	call.SessionID = edit.SessionID
	call.WorkspaceID = edit.WorkspaceID
	call.Arguments = map[string]string{"path": edit.Path, "content": edit.ProposedText}
	call.WorkspaceRoot = ""
	call, err := NormalizeToolCall(call)
	if err != nil {
		return Outcome{}, errors.Join(operationErr, err)
	}
	status, ok := gatewayFileEditStatus(edit.Status)
	if !ok {
		return Outcome{}, errors.Join(operationErr, fmt.Errorf("unsupported file edit status %q", edit.Status))
	}
	allowed := status != StatusDenied
	approval := ApprovalPerCall
	reason := "workspace file replacements require explicit per-call approval"
	if !allowed {
		approval = ApprovalNever
		if strings.TrimSpace(edit.Reason) != "" {
			reason = edit.Reason
		}
	}
	decision, err := NormalizeDecision(Decision{Allowed: allowed, Approval: approval, Risk: "medium", Reason: reason})
	if err != nil {
		return Outcome{}, errors.Join(operationErr, err)
	}
	proposal := &Proposal{
		ID: edit.ID, Tool: ReplaceFileTool, Class: ClassWorkspaceWrite, Status: status,
		Preview: boundedPreview(edit.Diff), CreatedAt: edit.CreatedAt, UpdatedAt: edit.UpdatedAt,
	}
	outcome := Outcome{Call: safeToolCall(call), Decision: decision, Proposal: proposal}
	if status == StatusCompleted || status == StatusFailed {
		completed := edit.UpdatedAt
		started := completed
		outcome.Execution = &Execution{Backend: "workspace_write", Status: status, StartedAt: started, CompletedAt: &completed}
		stderr := ""
		exitCode := 0
		if status == StatusFailed {
			stderr = edit.Reason
			exitCode = 1
		}
		stderr, truncated := boundResultText(stderr, MaxResultStderrBytes)
		outcome.Result = &Result{
			Status: status, Stderr: stderr, ExitCode: exitCode, MIME: "text/plain; charset=utf-8", Truncated: truncated,
			Metadata: map[string]string{"path": redact.String(edit.Path)}, CompletedAt: completed,
		}
	} else if status == StatusDenied {
		outcome.Result = deniedResult(edit.Reason, edit.UpdatedAt)
	}
	return validateOutcome(outcome, operationErr)
}

func deniedOutcome(call ToolCall, source policy.Decision) (Outcome, error) {
	decision, err := gatewayDecision(source, ApprovalNever, "high")
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{Call: safeToolCall(call), Decision: decision, Result: deniedResult(decision.Reason, time.Now().UTC())}
	return validateOutcome(outcome, nil)
}

func deniedResult(reason string, at time.Time) *Result {
	stderr, truncated := boundResultText(reason, MaxResultStderrBytes)
	return &Result{
		Status: StatusDenied, Stderr: stderr, ExitCode: 126,
		MIME: "text/plain; charset=utf-8", Truncated: truncated, CompletedAt: at,
	}
}

func gatewayDecision(source policy.Decision, mode ApprovalMode, fallbackRisk string) (Decision, error) {
	if !source.Allowed {
		mode = ApprovalNever
	}
	reason := strings.TrimSpace(source.Reason)
	if reason == "" {
		reason = "tool call evaluated by policy"
	}
	return NormalizeDecision(Decision{
		Allowed: source.Allowed, Approval: mode, Risk: defaultRisk(source.Risk, fallbackRisk), Reason: reason,
	})
}

func validateToolArguments(call ToolCall) error {
	var required []string
	var optional []string
	switch call.Name {
	case ReadFileTool:
		required = []string{"path"}
		optional = []string{"max_bytes"}
	case ListWorkspaceTool:
		optional = []string{"path", "max_depth"}
	case ShellTool:
		required = []string{"command"}
	case ReplaceFileTool:
		required = []string{"path", "content"}
	default:
		return fmt.Errorf("unsupported tool %q", call.Name)
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, name := range append(required, optional...) {
		allowed[name] = struct{}{}
	}
	for name := range call.Arguments {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("tool %s does not accept argument %q", call.Name, name)
		}
	}
	for _, name := range required {
		value, ok := call.Arguments[name]
		if !ok || (name != "content" && strings.TrimSpace(value) == "") {
			return fmt.Errorf("tool %s requires argument %q", call.Name, name)
		}
	}
	if call.Name == ShellTool && len([]byte(strings.TrimSpace(call.Arguments["command"]))) > MaxCommandBytes {
		return fmt.Errorf("shell command exceeds %d bytes", MaxCommandBytes)
	}
	if value := strings.TrimSpace(call.Arguments["max_bytes"]); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > MaxResultStdoutBytes {
			return fmt.Errorf("read_file max_bytes must be between 1 and %d", MaxResultStdoutBytes)
		}
	}
	if value := strings.TrimSpace(call.Arguments["max_depth"]); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 || parsed > maxWorkspaceListDepth {
			return fmt.Errorf("list_workspace max_depth must be between 0 and %d", maxWorkspaceListDepth)
		}
	}
	return nil
}

func gatewayToolRunStatus(status string) (Status, bool) {
	switch status {
	case toolrun.StatusProposed:
		return StatusProposed, true
	case toolrun.StatusApproved:
		return StatusApproved, true
	case toolrun.StatusDenied:
		return StatusDenied, true
	case toolrun.StatusRunning:
		return StatusRunning, true
	case toolrun.StatusCompleted:
		return StatusCompleted, true
	case toolrun.StatusFailed:
		return StatusFailed, true
	default:
		return "", false
	}
}

func gatewayFileEditStatus(status string) (Status, bool) {
	switch status {
	case fileedit.StatusProposed:
		return StatusProposed, true
	case fileedit.StatusApproved:
		return StatusApproved, true
	case fileedit.StatusApplied:
		return StatusCompleted, true
	case fileedit.StatusDenied:
		return StatusDenied, true
	case fileedit.StatusFailed:
		return StatusFailed, true
	default:
		return "", false
	}
}

func safeToolCall(call ToolCall) ToolCall {
	call.WorkspaceRoot = ""
	call.Arguments = cloneArguments(call.Arguments)
	for name, value := range call.Arguments {
		call.Arguments[name] = redact.String(value)
	}
	call.RequestedBy = redact.String(call.RequestedBy)
	return call
}

func cloneArguments(arguments map[string]string) map[string]string {
	out := make(map[string]string, len(arguments))
	for name, value := range arguments {
		out[name] = value
	}
	return out
}

func boundResultText(value string, limit int) (string, bool) {
	value = redact.String(strings.ToValidUTF8(value, "?"))
	if limit <= 0 {
		return "", value != ""
	}
	if len([]byte(value)) <= limit {
		return value, false
	}
	const marker = "\n[truncated by tool gateway]\n"
	if limit <= len(marker) {
		return marker[:limit], true
	}
	contentLimit := max(0, limit-len(marker))
	data := []byte(value)
	data = data[:contentLimit]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data) + marker, true
}

func boundedPreview(value string) string {
	value, _ = boundResultText(value, MaxProposalPreviewBytes)
	return value
}

func defaultRisk(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func validateOutcome(outcome Outcome, operationErr error) (Outcome, error) {
	if err := outcome.Validate(); err != nil {
		return Outcome{}, errors.Join(operationErr, err)
	}
	return outcome, operationErr
}

func (g *Gateway) bindWorkspaceRoot(ctx context.Context, workspaceID string, provided string) (string, error) {
	provided = strings.TrimSpace(provided)
	if g.workspaceRootResolver == nil {
		if provided == "" {
			return "", errors.New("workspace root is required")
		}
		return provided, nil
	}
	expected, err := g.workspaceRootResolver(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return "", errors.New("resolved workspace root is empty")
	}
	if provided == "" {
		return expected, nil
	}
	expectedInfo, err := os.Stat(expected)
	if err != nil {
		return "", errors.New("resolved workspace root is unavailable")
	}
	providedInfo, err := os.Stat(provided)
	if err != nil {
		return "", errors.New("provided workspace root is unavailable")
	}
	if !expectedInfo.IsDir() || !providedInfo.IsDir() || !os.SameFile(expectedInfo, providedInfo) {
		return "", errors.New("provided workspace root does not match the workspace record")
	}
	return expected, nil
}
