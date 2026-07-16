package toolgateway

import (
	"context"
	"strings"
	"testing"
)

func TestEncodeScriptProcessProposalIsDeterministicAndNonExecutable(t *testing.T) {
	arguments := []string{"scripts/probe.py", "--name", "demo"}
	payload, err := EncodeScriptProcessProposal(ScriptProcessProposal{
		Executable: " python ", Arguments: arguments, RequestedBackend: " LOCAL ",
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments[0] = "changed.py"
	want := `{"schema":"script_process.v1","executable":"python","arguments":["scripts/probe.py","--name","demo"],"working_directory":".","requested_backend":"local","execution_mode":"disabled"}`
	if payload != want {
		t.Fatalf("unexpected script process payload:\n got: %s\nwant: %s", payload, want)
	}
}

func TestScriptProcessProposalRejectsUnsafeOrUnboundedFields(t *testing.T) {
	tests := []struct {
		name     string
		proposal ScriptProcessProposal
	}{
		{name: "missing executable", proposal: ScriptProcessProposal{}},
		{name: "NUL executable", proposal: ScriptProcessProposal{Executable: "py\x00thon"}},
		{name: "working directory", proposal: ScriptProcessProposal{Executable: "python", WorkingDirectory: "scripts"}},
		{name: "backend", proposal: ScriptProcessProposal{Executable: "python", RequestedBackend: "host"}},
		{name: "too many arguments", proposal: ScriptProcessProposal{Executable: "python", Arguments: make([]string, MaxScriptProcessArguments+1)}},
		{name: "large argument", proposal: ScriptProcessProposal{Executable: "python", Arguments: []string{strings.Repeat("x", MaxScriptProcessArgumentBytes+1)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := EncodeScriptProcessProposal(test.proposal); err == nil {
				t.Fatal("expected script process validation error")
			}
		})
	}
}

func TestScriptProcessGatewayRejectsCallerGeneratedArguments(t *testing.T) {
	gateway := New(newMemoryStore(), nil)
	if _, err := gateway.ProposeScriptProcess(t.Context(), ToolCall{
		Name: ScriptProcessTool, RunID: "run-1", SessionID: "sess-1", WorkspaceID: "ws-1",
		Arguments: map[string]string{"command": "echo bypass"},
	}, ScriptProcessProposal{Executable: "python"}); err == nil {
		t.Fatal("expected caller-generated command rejection")
	}
	if _, err := gateway.ProposeScriptProcess(t.Context(), ToolCall{
		Name: ReadFileTool, RunID: "run-1", SessionID: "sess-1", WorkspaceID: "ws-1",
	}, ScriptProcessProposal{Executable: "python"}); err == nil {
		t.Fatal("expected mismatched tool rejection")
	}
}

func TestScriptProcessGatewayReviewCompletesOnlyAsDryRun(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	gateway := New(store, nil)
	outcome, err := gateway.ProposeScriptProcess(ctx, ToolCall{
		Name: ScriptProcessTool, RunID: "run-1", SessionID: "sess-1", WorkspaceID: "ws-1",
		WorkspaceRoot: t.TempDir(), RequestedBy: "test",
	}, ScriptProcessProposal{Executable: "python", Arguments: []string{"scripts/noop.py"}})
	if err != nil || outcome.Proposal == nil || outcome.Proposal.Tool != ScriptProcessTool ||
		outcome.Proposal.Status != StatusProposed || outcome.Execution != nil {
		t.Fatalf("unexpected script process proposal: %#v err=%v", outcome, err)
	}
	reviewed, err := gateway.Review(ctx, ReviewRequest{
		Action: ReviewApprove, Tool: ScriptProcessTool, ProposalID: outcome.Proposal.ID, ReviewedBy: "operator",
	})
	if err != nil || reviewed.Proposal == nil || reviewed.Proposal.Status != StatusCompleted ||
		reviewed.Execution == nil || reviewed.Execution.Backend != "dry_run" || reviewed.Result == nil ||
		!strings.Contains(reviewed.Result.Stdout, `"execution_mode":"disabled"`) {
		t.Fatalf("script process approval was not a typed dry run: %#v err=%v", reviewed, err)
	}
}

func TestScriptProcessGatewayPermanentlyDeniesProtectedDelete(t *testing.T) {
	store := newMemoryStore()
	gateway := New(store, nil)
	outcome, err := gateway.ProposeScriptProcess(t.Context(), ToolCall{
		Name: ScriptProcessTool, RunID: "run-delete", SessionID: "sess-delete", WorkspaceID: "ws-delete",
		WorkspaceRoot: t.TempDir(), RequestedBy: "test",
	}, ScriptProcessProposal{
		Executable: "python", Arguments: []string{"-c", `import shutil; shutil.rmtree('/workspace')`},
	})
	if err != nil || outcome.Proposal == nil || outcome.Proposal.Status != StatusDenied ||
		outcome.Decision.Allowed || outcome.Decision.Approval != ApprovalNever || outcome.Decision.Risk != "critical" ||
		outcome.Execution != nil {
		t.Fatalf("protected ScriptProcess deletion was not permanently denied: %#v err=%v", outcome, err)
	}
	process, err := store.GetScriptProcess(t.Context(), outcome.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if process.Status != "denied" || process.ExecutionMode != "disabled" || process.Stdout != "" {
		t.Fatalf("denied ScriptProcess acquired execution state: %#v", process)
	}
}
