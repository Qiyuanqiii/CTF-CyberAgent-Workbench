package app

import (
	"regexp"
	"strings"
	"testing"
)

var grantIDPattern = regexp.MustCompile(`grant-[0-9]{14}-[a-f0-9]{12}`)

func TestApprovalGrantCLIControlsSessionAuthorizationAndToolBudget(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "grant-cli"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "grant CLI integration",
		"--workspace", "grant-cli", "--max-tool-calls", "4")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	sessionID := sessionIDPattern.FindString(created)
	if runID == "" || sessionID == "" {
		t.Fatalf("run identity missing: %s", created)
	}
	if _, stderr, code := executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("run start failed: %s", stderr)
	}
	grantOutput, stderr, code := executeTestCommand(t, "approval", "grant", "create",
		"--session", sessionID, "--tool", "shell", "--reason", "bounded CLI commands",
		"--idempotency-key", "grant-cli-create")
	if code != 0 {
		t.Fatalf("grant create failed: %s", stderr)
	}
	grantID := grantIDPattern.FindString(grantOutput)
	if grantID == "" || !strings.Contains(grantOutput, "status: active") || !strings.Contains(grantOutput, "tool: shell") {
		t.Fatalf("grant output is incomplete: %s", grantOutput)
	}
	replayed, stderr, code := executeTestCommand(t, "approval", "grant", "create",
		"--session", sessionID, "--tool", "shell", "--reason", "bounded CLI commands",
		"--idempotency-key", "grant-cli-create")
	if code != 0 || !strings.Contains(replayed, "reused") {
		t.Fatalf("grant replay failed: code=%d stderr=%s output=%s", code, stderr, replayed)
	}
	authorized, stderr, code := executeTestCommand(t, "session", "send", sessionID, "/run echo grant-cli")
	if code != 0 || !strings.Contains(authorized, "authorized by an active Session grant") ||
		!strings.Contains(authorized, "completed as a dry run") {
		t.Fatalf("session grant was not used: code=%d stderr=%s output=%s", code, stderr, authorized)
	}
	usage, stderr, code := executeTestCommand(t, "run", "usage", runID)
	if code != 0 || !strings.Contains(usage, "tool_calls: 1") ||
		!strings.Contains(usage, "tool_call_limit: 4") || !strings.Contains(usage, "tool_calls_remaining: 3") {
		t.Fatalf("tool budget was not visible: code=%d stderr=%s output=%s", code, stderr, usage)
	}
	revoked, stderr, code := executeTestCommand(t, "approval", "grant", "revoke", grantID,
		"--reason", "CLI phase complete", "--idempotency-key", "grant-cli-revoke")
	if code != 0 || !strings.Contains(revoked, "status: revoked") || !strings.Contains(revoked, "CLI phase complete") {
		t.Fatalf("grant revoke failed: code=%d stderr=%s output=%s", code, stderr, revoked)
	}
	pending, stderr, code := executeTestCommand(t, "session", "send", sessionID, "/run echo requires-review")
	if code != 0 || !strings.Contains(pending, "proposed") || strings.Contains(pending, "authorized by an active Session grant") {
		t.Fatalf("revoked grant still authorized calls: code=%d stderr=%s output=%s", code, stderr, pending)
	}
	listed, stderr, code := executeTestCommand(t, "approval", "grant", "list", "--run", runID, "--status", "revoked")
	if code != 0 || !strings.Contains(listed, grantID) {
		t.Fatalf("revoked grant was not inspectable: code=%d stderr=%s output=%s", code, stderr, listed)
	}
}
