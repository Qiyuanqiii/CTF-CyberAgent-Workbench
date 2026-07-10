package app

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

var runIDPattern = regexp.MustCompile(`run-[0-9]{14}-[a-f0-9]{12}`)

func executeTestCommand(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Execute(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestRunCLIEndToEndLifecycle(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "review this workspace", "--workspace", "demo", "--profile", "review", "--max-turns", "12")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" || !strings.Contains(created, "status: created") {
		t.Fatalf("unexpected create output: %s", created)
	}
	initialEvents, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || !strings.Contains(initialEvents, "run.created") {
		t.Fatalf("unexpected initial events output=%s stderr=%s", initialEvents, stderr)
	}
	for _, step := range []struct {
		action string
		status string
	}{
		{"start", "running"},
		{"pause", "paused"},
		{"resume", "running"},
		{"cancel", "cancelled"},
	} {
		stdout, stderr, code := executeTestCommand(t, "run", step.action, runID)
		if code != 0 || !strings.Contains(stdout, step.status) {
			t.Fatalf("run %s failed output=%s stderr=%s", step.action, stdout, stderr)
		}
	}
	shown, stderr, code := executeTestCommand(t, "run", "show", runID)
	if code != 0 || !strings.Contains(shown, "status: cancelled") || !strings.Contains(shown, `"max_turns":12`) {
		t.Fatalf("unexpected show output=%s stderr=%s", shown, stderr)
	}
	eventOutput, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(eventOutput, "run.status_changed") != 5 {
		t.Fatalf("unexpected event timeline output=%s stderr=%s", eventOutput, stderr)
	}
	listed, stderr, code := executeTestCommand(t, "run", "list", "--status", "cancelled")
	if code != 0 || !strings.Contains(listed, runID) {
		t.Fatalf("unexpected list output=%s stderr=%s", listed, stderr)
	}
}
