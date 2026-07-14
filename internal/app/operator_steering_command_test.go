package app

import (
	"regexp"
	"strings"
	"testing"
)

var operatorSteeringIDPattern = regexp.MustCompile(
	`steer-[0-9]{14}-[a-f0-9]{12}`)

func TestOperatorSteeringCLIEnqueuesReplaysListsAndShows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	created, stderr, code := executeTestCommand(t, "run", "create",
		"exercise operator steering CLI", "--profile", "review", "--max-turns", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("Run create failed: output=%s stderr=%s code=%d", created, stderr, code)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("Run id missing: %s", created)
	}
	if _, stderr, code = executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("Run start failed: %s", stderr)
	}
	operationKey := "operator-steering-cli-operation-0001"
	queued, stderr, code := executeTestCommand(t, "run", "steer", "enqueue", runID,
		"verify the next safe boundary", "--operation-key", operationKey)
	if code != 0 || stderr != "" || !strings.Contains(queued, "replayed: false") {
		t.Fatalf("steering enqueue failed: output=%s stderr=%s code=%d", queued, stderr, code)
	}
	steeringID := operatorSteeringIDPattern.FindString(queued)
	if steeringID == "" {
		t.Fatalf("steering id missing: %s", queued)
	}
	replayed, stderr, code := executeTestCommand(t, "run", "steer", "enqueue", runID,
		"verify the next safe boundary", "--operation-key", operationKey)
	if code != 0 || stderr != "" || !strings.Contains(replayed, "replayed: true") ||
		!strings.Contains(replayed, steeringID) {
		t.Fatalf("steering replay failed: output=%s stderr=%s code=%d", replayed, stderr, code)
	}
	listed, stderr, code := executeTestCommand(t, "run", "steer", "list", runID)
	if code != 0 || stderr != "" || !strings.Contains(listed, "pending: 1") ||
		!strings.Contains(listed, steeringID) || strings.Contains(listed, "verify the next") {
		t.Fatalf("steering list projection is invalid: output=%s stderr=%s code=%d", listed, stderr, code)
	}
	shown, stderr, code := executeTestCommand(t, "run", "steer", "show", steeringID)
	if code != 0 || stderr != "" || !strings.Contains(shown, "status: pending") ||
		!strings.Contains(shown, "verify the next safe boundary") {
		t.Fatalf("steering show failed: output=%s stderr=%s code=%d", shown, stderr, code)
	}
	cancelled, stderr, code := executeTestCommand(t, "run", "steer", "cancel", steeringID,
		"--operation-key", "operator-steering-cli-cancel-0001",
		"--reason", "operator withdrew this guidance")
	if code != 0 || stderr != "" || !strings.Contains(cancelled, "status: cancelled") ||
		!strings.Contains(cancelled, "kind: operator") ||
		!strings.Contains(cancelled, "replayed: false") {
		t.Fatalf("steering cancellation failed: output=%s stderr=%s code=%d",
			cancelled, stderr, code)
	}
	cancelReplay, stderr, code := executeTestCommand(t, "run", "steer", "cancel", steeringID,
		"--operation-key", "operator-steering-cli-cancel-0001",
		"--reason", "operator withdrew this guidance")
	if code != 0 || stderr != "" || !strings.Contains(cancelReplay, "replayed: true") {
		t.Fatalf("steering cancellation replay failed: output=%s stderr=%s code=%d",
			cancelReplay, stderr, code)
	}
	listed, stderr, code = executeTestCommand(t, "run", "steer", "list", runID)
	if code != 0 || stderr != "" || !strings.Contains(listed, "pending: 0") ||
		!strings.Contains(listed, "cancelled: 1") {
		t.Fatalf("cancelled steering list drifted: output=%s stderr=%s code=%d",
			listed, stderr, code)
	}
	_, stderr, code = executeTestCommand(t, "run", "steer", "list", "run-missing")
	if code != 3 || !strings.Contains(stderr, "not found") {
		t.Fatalf("missing Run list did not fail closed: stderr=%s code=%d", stderr, code)
	}
}

func TestSessionSteeringRetryAndExplicitDrainCLI(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	created, stderr, code := executeTestCommand(t, "run", "create",
		"exercise durable Session steering", "--profile", "review", "--max-turns", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("Run create failed: output=%s stderr=%s code=%d", created, stderr, code)
	}
	runID := runIDPattern.FindString(created)
	sessionID := sessionIDPattern.FindString(created)
	if runID == "" || sessionID == "" {
		t.Fatalf("Run or Session id missing: %s", created)
	}
	if _, stderr, code = executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("Run start failed: %s", stderr)
	}
	if _, stderr, code = executeTestCommand(t, "run", "pause", runID); code != 0 {
		t.Fatalf("Run pause failed: %s", stderr)
	}
	if _, stderr, code = executeTestCommand(t, "session", "send", sessionID,
		"must not become a synchronous turn", "--operation-key", ""); code == 0 ||
		!strings.Contains(stderr, "cannot be blank") {
		t.Fatalf("blank explicit Session retry key did not fail closed: stderr=%s code=%d",
			stderr, code)
	}
	const operationKey = "session-steering-cli-retry-0001"
	queued, stderr, code := executeTestCommand(t, "session", "send", sessionID,
		"queue this ordinary Session input", "--operation-key", operationKey)
	if code != 0 || stderr != "" || !strings.Contains(queued, "status=pending") ||
		!strings.Contains(queued, "replayed=false") || !strings.Contains(queued, "status=paused") {
		t.Fatalf("Session steering queue failed: output=%s stderr=%s code=%d", queued, stderr, code)
	}
	steeringID := operatorSteeringIDPattern.FindString(queued)
	if steeringID == "" {
		t.Fatalf("Session steering id missing: %s", queued)
	}
	replayed, stderr, code := executeTestCommand(t, "session", "send", sessionID,
		"queue this ordinary Session input", "--operation-key", operationKey)
	if code != 0 || stderr != "" || !strings.Contains(replayed, steeringID) ||
		!strings.Contains(replayed, "replayed=true") {
		t.Fatalf("Session steering replay failed: output=%s stderr=%s code=%d",
			replayed, stderr, code)
	}
	drained, stderr, code := executeTestCommand(t, "run", "steer", "drain", runID,
		"--max-steps", "1")
	if code != 0 || stderr != "" || !strings.Contains(drained, "woke: true") ||
		!strings.Contains(drained, "after_pending: 0") ||
		!strings.Contains(drained, "committed: 1") ||
		!strings.Contains(drained, "stop: steering_drained") {
		t.Fatalf("explicit steering drain failed: output=%s stderr=%s code=%d",
			drained, stderr, code)
	}
}
