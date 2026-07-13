package app

import (
	"strings"
	"testing"
)

func TestTUICommandOpensExactRunSnapshot(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "tui-run"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "open this Run in TUI",
		"--workspace", "tui-run", "--profile", "review")
	if code != 0 {
		t.Fatalf("Run creation failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	sessionID := sessionIDPattern.FindString(created)
	if runID == "" || sessionID == "" {
		t.Fatalf("missing Run identities: %s", created)
	}
	snapshot, stderr, code := executeTestCommand(t, "tui", "--run", runID, "--print")
	if code != 0 || stderr != "" || !strings.Contains(snapshot, "run="+runID) ||
		!strings.Contains(snapshot, "status=created") {
		t.Fatalf("exact Run TUI snapshot failed: code=%d stderr=%s snapshot=%s",
			code, stderr, snapshot)
	}
	_, stderr, code = executeTestCommand(t, "tui", "--run", runID,
		"--session", sessionID, "--print")
	if code == 0 || !strings.Contains(stderr, "cannot be used together") {
		t.Fatalf("conflicting TUI identities were accepted: code=%d stderr=%s", code, stderr)
	}
}

func TestCLIHelpListsRunFirstTUISelectors(t *testing.T) {
	stdout, stderr, code := executeTestCommand(t, "help")
	if code != 0 || stderr != "" ||
		!strings.Contains(stdout, "tui [--run <run-id> | --session <session-id>] [--print]") {
		t.Fatalf("Run-first TUI selectors are missing from help: code=%d stderr=%s stdout=%s",
			code, stderr, stdout)
	}
}
