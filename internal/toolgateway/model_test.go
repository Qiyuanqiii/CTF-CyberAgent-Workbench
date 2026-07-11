package toolgateway

import (
	"strings"
	"testing"
	"time"
)

func TestApprovalModesAndToolClassesAreStable(t *testing.T) {
	for _, mode := range []ApprovalMode{ApprovalAutomatic, ApprovalPerCall, ApprovalSession, ApprovalNever} {
		if !mode.Valid() {
			t.Fatalf("approval mode %q is not valid", mode)
		}
	}
	tests := map[ToolName]ActionClass{
		ReadFileTool: ClassWorkspaceRead, ListWorkspaceTool: ClassWorkspaceRead,
		ReplaceFileTool: ClassWorkspaceWrite, ShellTool: ClassShell,
		ScriptProcessTool: ClassProcess, WorkItemCreateTool: ClassRunMemory, NoteCreateTool: ClassRunMemory,
	}
	for name, want := range tests {
		got, ok := ClassForTool(name)
		if !ok || got != want {
			t.Fatalf("ClassForTool(%q) = %q, %t; want %q", name, got, ok, want)
		}
	}
}

func TestToolCallNormalizationAndBounds(t *testing.T) {
	arguments := map[string]string{"path": " README.md "}
	call, err := NormalizeToolCall(ToolCall{
		Name: ReadFileTool, Arguments: arguments, WorkspaceID: " ws-demo ", WorkspaceRoot: " C:/workspace ", RequestedBy: " root ",
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments["path"] = "changed"
	if call.WorkspaceID != "ws-demo" || call.RequestedBy != "root" || call.Arguments["path"] != " README.md " {
		t.Fatalf("tool call was not normalized or cloned: %#v", call)
	}
	if err := call.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeToolCall(ToolCall{Name: "unknown", Arguments: map[string]string{}}); err == nil {
		t.Fatal("expected unsupported tool rejection")
	}
	if _, err := NormalizeToolCall(ToolCall{Name: ReadFileTool, Arguments: map[string]string{"Bad-Key": "value"}}); err == nil {
		t.Fatal("expected invalid argument name rejection")
	}
	if _, err := NormalizeToolCall(ToolCall{Name: ReadFileTool, Arguments: map[string]string{"path": string([]byte{0xff})}}); err == nil {
		t.Fatal("expected invalid UTF-8 argument rejection")
	}
	if _, err := NormalizeToolCall(ToolCall{
		Name: ReplaceFileTool, Arguments: map[string]string{"path": "x", "content": strings.Repeat("x", MaxArgumentValueBytes+1)},
	}); err == nil {
		t.Fatal("expected oversized argument rejection")
	}
}

func TestDecisionAndOutcomeInvariants(t *testing.T) {
	if _, err := NormalizeDecision(Decision{Allowed: true, Approval: ApprovalNever, Risk: "low", Reason: "bad"}); err == nil {
		t.Fatal("expected allowed/never inconsistency rejection")
	}
	if _, err := NormalizeDecision(Decision{Allowed: false, Approval: ApprovalPerCall, Risk: "high", Reason: "bad"}); err == nil {
		t.Fatal("expected denied/per-call inconsistency rejection")
	}
	decision, err := NormalizeDecision(Decision{
		Allowed: true, Approval: ApprovalAutomatic, Risk: " LOW ", Reason: " allowed ",
	})
	if err != nil || decision.Risk != "low" || decision.Reason != "allowed" {
		t.Fatalf("unexpected normalized decision: %#v err=%v", decision, err)
	}
	now := time.Now().UTC()
	outcome := Outcome{
		Call:      ToolCall{Name: ReadFileTool, Arguments: map[string]string{"path": "README.md"}, WorkspaceID: "ws"},
		Decision:  decision,
		Execution: &Execution{Backend: "workspace_fs", Status: StatusCompleted, StartedAt: now, CompletedAt: &now},
		Result: &Result{
			Status: StatusCompleted, MIME: "text/plain; charset=utf-8", CompletedAt: now,
		},
	}
	if err := outcome.Validate(); err != nil {
		t.Fatal(err)
	}
	broken := outcome
	broken.Execution = nil
	if err := broken.Validate(); err == nil {
		t.Fatal("expected terminal result without execution rejection")
	}
	broken = outcome
	broken.Result = &Result{Status: StatusDenied, MIME: "text/plain; charset=utf-8", CompletedAt: now}
	broken.Execution = nil
	if err := broken.Validate(); err == nil {
		t.Fatal("expected allowed decision with denied result rejection")
	}
	broken = outcome
	broken.Result = &Result{Status: StatusCompleted, MIME: "not a media type;", CompletedAt: now}
	if err := broken.Validate(); err == nil {
		t.Fatal("expected invalid MIME rejection")
	}
}

func TestReviewRequestValidation(t *testing.T) {
	request, err := NormalizeReviewRequest(ReviewRequest{
		Action: ReviewDeny, Tool: ShellTool, ProposalID: " tool-1 ", Reason: " no ",
	})
	if err != nil || request.ProposalID != "tool-1" || request.Reason != "no" {
		t.Fatalf("unexpected review request: %#v err=%v", request, err)
	}
	if _, err := NormalizeReviewRequest(ReviewRequest{
		Action: ReviewApprove, Tool: ShellTool, ProposalID: "tool-1", Reason: "not allowed",
	}); err == nil {
		t.Fatal("expected approval reason rejection")
	}
	if _, err := NormalizeReviewRequest(ReviewRequest{
		Action: ReviewApprove, Tool: ReadFileTool, ProposalID: "read-1",
	}); err == nil {
		t.Fatal("expected read review rejection")
	}
	if _, err := NormalizeReviewRequest(ReviewRequest{
		Action: ReviewApprove, Tool: ReplaceFileTool, ProposalID: "edit-1",
	}); err != nil {
		t.Fatalf("server-resolved file approval should not require a caller root: %v", err)
	}
	if _, err := NormalizeReviewRequest(ReviewRequest{
		Action: ReviewDeny, Tool: ReplaceFileTool, ProposalID: "edit-1", WorkspaceRoot: "C:/workspace",
	}); err == nil {
		t.Fatal("expected denial workspace root rejection")
	}
}
