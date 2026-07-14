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
	_, stderr, code = executeTestCommand(t, "run", "steer", "list", "run-missing")
	if code != 3 || !strings.Contains(stderr, "not found") {
		t.Fatalf("missing Run list did not fail closed: stderr=%s code=%d", stderr, code)
	}
}
