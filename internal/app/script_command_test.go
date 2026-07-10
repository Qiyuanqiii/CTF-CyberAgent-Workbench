package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptNewPrintsWorkspaceRelativeRunPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	stdout, stderr, code := executeTestCommand(t,
		"script", "new", "relative path", "--workspace", "demo", "--language", "python",
	)
	if code != 0 || !strings.Contains(stdout, "script_relative: scripts/") || !strings.Contains(stdout, ".py") {
		t.Fatalf("script new did not expose a runnable relative path: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
}

func TestScriptRunCreatesAuditedDryRunWithoutLocalExecution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	scriptPath := filepath.Join(home, "workspaces", "demo", "scripts", "probe.cmd")
	markerPath := filepath.Join(home, "workspaces", "demo", "outputs", "executed.txt")
	if err := os.WriteFile(scriptPath, []byte("@echo off\r\necho executed>..\\outputs\\executed.txt\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	token := "s" + "k-" + strings.Repeat("a", 28)

	stdout, stderr, code := executeTestCommand(t,
		"script", "run", "scripts/probe.cmd", "--workspace", "demo", "--local", "--token="+token,
	)
	if code != 0 {
		t.Fatalf("script proposal failed: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	runID := runIDPattern.FindString(stdout)
	sessionID := sessionIDPattern.FindString(stdout)
	processID := processIDPattern.FindString(stdout)
	if runID == "" || sessionID == "" || processID == "" ||
		!strings.Contains(stdout, "requested_backend: local") ||
		!strings.Contains(stdout, "status: proposed") ||
		!strings.Contains(stdout, "execution: disabled; approval completes as dry run") {
		t.Fatalf("unexpected script proposal output: %s", stdout)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("--local executed before approval: %v", err)
	}

	shown, stderr, code := executeTestCommand(t, "tool", "show", processID)
	if code != 0 || !strings.Contains(shown, "schema: script_process.v1") ||
		!strings.Contains(shown, "tool: script_process") ||
		!strings.Contains(shown, "requested_backend: local") ||
		!strings.Contains(shown, "execution_mode: disabled") ||
		!strings.Contains(shown, "[REDACTED:secret]") || strings.Contains(shown, token) || strings.Contains(shown, home) {
		t.Fatalf("unexpected structured proposal: code=%d stderr=%s output=%s", code, stderr, shown)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || !strings.Contains(timeline, "run.created") || !strings.Contains(timeline, "session.attached") ||
		!strings.Contains(timeline, "policy.decision") || !strings.Contains(timeline, "tool.proposed") ||
		!strings.Contains(timeline, "approval.requested") {
		t.Fatalf("script proposal was not fully audited: code=%d stderr=%s events=%s", code, stderr, timeline)
	}
	approvals, stderr, code := executeTestCommand(t, "approval", "list", "--run", runID, "--status", "pending")
	approvalID := approvalIDPattern.FindString(approvals)
	if code != 0 || approvalID == "" || !strings.Contains(approvals, processID) || !strings.Contains(approvals, "script_process") {
		t.Fatalf("pending approval was not inspectable: code=%d stderr=%s output=%s", code, stderr, approvals)
	}
	shownApproval, stderr, code := executeTestCommand(t, "approval", "show", approvalID)
	if code != 0 || !strings.Contains(shownApproval, "status: pending") ||
		!strings.Contains(shownApproval, "proposal: "+processID) || !strings.Contains(shownApproval, "run: "+runID) {
		t.Fatalf("approval detail is incomplete: code=%d stderr=%s output=%s", code, stderr, shownApproval)
	}

	approved, stderr, code := executeTestCommand(t, "tool", "approve", processID)
	if code != 0 || !strings.Contains(approved, "completed") || !strings.Contains(approved, "dry run:") {
		t.Fatalf("script dry-run approval failed: code=%d stderr=%s output=%s", code, stderr, approved)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("approval executed the local script: %v", err)
	}
	timeline, stderr, code = executeTestCommand(t, "run", "events", runID)
	if code != 0 || !strings.Contains(timeline, "approval.decided") ||
		!strings.Contains(timeline, "tool.approved") || !strings.Contains(timeline, "tool.completed") {
		t.Fatalf("script approval events missing: code=%d stderr=%s events=%s", code, stderr, timeline)
	}
	approvals, stderr, code = executeTestCommand(t, "approval", "list", "--run", runID, "--status", "approved")
	if code != 0 || !strings.Contains(approvals, approvalID) || !strings.Contains(approvals, processID) {
		t.Fatalf("approved decision was not recoverable: code=%d stderr=%s output=%s", code, stderr, approvals)
	}
}

func TestScriptRunPersistsPolicyDenialWithoutExecution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	scriptPath := filepath.Join(home, "workspaces", "demo", "scripts", "probe.py")
	markerPath := filepath.Join(home, "workspaces", "demo", "outputs", "denied.txt")
	if err := os.WriteFile(scriptPath, []byte("from pathlib import Path\nPath('../outputs/denied.txt').write_text('ran')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := executeTestCommand(t,
		"script", "run", "scripts/probe.py", "--workspace", "demo", "--local", "0.0.0.0/0",
	)
	if code != 5 || !strings.Contains(stderr, "policy denied script run") ||
		!strings.Contains(stdout, "status: denied") || !strings.Contains(stdout, "approval: never") {
		t.Fatalf("unexpected policy denial: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	runID := runIDPattern.FindString(stdout)
	processID := processIDPattern.FindString(stdout)
	if runID == "" || processID == "" {
		t.Fatalf("denied proposal ids missing: %s", stdout)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("policy-denied script executed: %v", err)
	}
	timeline, eventErr, eventCode := executeTestCommand(t, "run", "events", runID)
	if eventCode != 0 || !strings.Contains(timeline, "policy.decision") || !strings.Contains(timeline, "tool.denied") ||
		!strings.Contains(timeline, "approval.requested") || !strings.Contains(timeline, "approval.decided") {
		t.Fatalf("denial events missing: code=%d stderr=%s events=%s", eventCode, eventErr, timeline)
	}
}

func TestScriptRunIdempotentReplayDoesNotDuplicateRunOrBudgetCharge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	scriptPath := filepath.Join(home, "workspaces", "demo", "scripts", "noop.py")
	if err := os.WriteFile(scriptPath, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := []string{
		"script", "run", "scripts/noop.py", "--workspace", "demo",
		"--idempotency-key", "stable-script-request", "alpha",
	}
	first, stderr, code := executeTestCommand(t, command...)
	if code != 0 {
		t.Fatalf("first script request failed: code=%d stderr=%s output=%s", code, stderr, first)
	}
	processID := processIDPattern.FindString(first)
	runID := runIDPattern.FindString(first)
	sessionID := sessionIDPattern.FindString(first)
	if processID == "" || runID == "" || sessionID == "" || !strings.Contains(first, "replayed: false") {
		t.Fatalf("first script request did not expose stable identities: %s", first)
	}

	second, stderr, code := executeTestCommand(t, command...)
	if code != 0 || processIDPattern.FindString(second) != processID || runIDPattern.FindString(second) != runID ||
		sessionIDPattern.FindString(second) != sessionID || !strings.Contains(second, "replayed: true") {
		t.Fatalf("script replay was not idempotent: code=%d stderr=%s output=%s", code, stderr, second)
	}
	listed, stderr, code := executeTestCommand(t, "tool", "list", "--session", sessionID)
	if code != 0 || strings.Count(listed, processID) != 1 || !strings.Contains(listed, "script_process") {
		t.Fatalf("script replay duplicated or hid its proposal: code=%d stderr=%s output=%s", code, stderr, listed)
	}
	usage, stderr, code := executeTestCommand(t, "run", "usage", runID)
	if code != 0 || !strings.Contains(usage, "tool_calls: 1") ||
		!strings.Contains(usage, "tool_call_limit: 100") || !strings.Contains(usage, "tool_calls_remaining: 99") {
		t.Fatalf("script replay duplicated its budget charge: code=%d stderr=%s output=%s", code, stderr, usage)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(timeline, "tool.budget_charged") != 1 ||
		strings.Count(timeline, "tool.proposed") != 1 || strings.Count(timeline, "approval.requested") != 1 {
		t.Fatalf("script replay duplicated audit events: code=%d stderr=%s output=%s", code, stderr, timeline)
	}

	conflict, conflictErr, conflictCode := executeTestCommand(t,
		"script", "run", "scripts/noop.py", "--workspace", "demo",
		"--idempotency-key", "stable-script-request", "different",
	)
	if conflictCode != 4 || !strings.Contains(conflictErr, "idempotency key") {
		t.Fatalf("changed idempotent request was not rejected: code=%d stderr=%s output=%s", conflictCode, conflictErr, conflict)
	}
	runs, stderr, code := executeTestCommand(t, "run", "list")
	if code != 0 || strings.Count(runs, runID) != 1 {
		t.Fatalf("idempotency conflict created another Run: code=%d stderr=%s output=%s", code, stderr, runs)
	}
}

func TestScriptRunRejectsUnscopedAndEscapingPathsBeforeRunCreation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	inside := filepath.Join(home, "workspaces", "demo", "scripts", "probe.py")
	outside := filepath.Join(home, "outside.py")
	if err := os.WriteFile(inside, []byte("print('inside')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("print('outside')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing workspace", args: []string{"script", "run", "scripts/probe.py"}, want: "usage:"},
		{name: "absolute path", args: []string{"script", "run", inside, "--workspace", "demo"}, want: "relative"},
		{name: "workspace escape", args: []string{"script", "run", "../../outside.py", "--workspace", "demo"}, want: "escapes workspace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, stderr, code := executeTestCommand(t, test.args...)
			if code != 2 || !strings.Contains(stderr, test.want) {
				t.Fatalf("unexpected path rejection: code=%d stderr=%s", code, stderr)
			}
		})
	}
	listed, stderr, code := executeTestCommand(t, "run", "list")
	if code != 0 || !strings.Contains(listed, "no runs") {
		t.Fatalf("invalid script paths created audit runs: code=%d stderr=%s output=%s", code, stderr, listed)
	}
}

func TestToolListRejectsUnknownStatus(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	_, stderr, code := executeTestCommand(t, "tool", "list", "--status", "invented")
	if code != 2 || !strings.Contains(stderr, "invalid tool proposal status") {
		t.Fatalf("unknown tool status was not rejected: code=%d stderr=%s", code, stderr)
	}
}
