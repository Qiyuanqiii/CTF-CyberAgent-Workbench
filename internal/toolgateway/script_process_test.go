package toolgateway

import (
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
		Name: ShellTool, SessionID: "sess-1", Arguments: map[string]string{"command": "echo bypass"},
	}, ScriptProcessProposal{Executable: "python"}); err == nil {
		t.Fatal("expected caller-generated command rejection")
	}
	if _, err := gateway.ProposeScriptProcess(t.Context(), ToolCall{
		Name: ReadFileTool, SessionID: "sess-1",
	}, ScriptProcessProposal{Executable: "python"}); err == nil {
		t.Fatal("expected mismatched tool rejection")
	}
}
