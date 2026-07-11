package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactCLIListsReadsAndVerifiesCapturedScriptOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	scriptPath := filepath.Join(home, "workspaces", "demo", "scripts", "artifact.py")
	if err := os.WriteFile(scriptPath, []byte("print('not executed')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	token := "s" + "k-" + strings.Repeat("q", 28)
	created, stderr, code := executeTestCommand(t,
		"script", "run", "scripts/artifact.py", "--workspace", "demo",
		"--idempotency-key", "artifact-cli-test", "--token="+token,
	)
	if code != 0 {
		t.Fatalf("script process creation failed: code=%d stderr=%s output=%s", code, stderr, created)
	}
	processID := processIDPattern.FindString(created)
	runID := runIDPattern.FindString(created)
	if processID == "" || runID == "" {
		t.Fatalf("script process identities are missing: %s", created)
	}
	approved, stderr, code := executeTestCommand(t, "tool", "approve", processID)
	artifactID := artifactIDPattern.FindString(approved)
	if code != 0 || artifactID == "" || !strings.Contains(approved, "artifact_stdout_id:") {
		t.Fatalf("script output artifact was not exposed: code=%d stderr=%s output=%s", code, stderr, approved)
	}

	listed, stderr, code := executeTestCommand(t, "artifact", "list", "--run", runID, "--stream", "stdout")
	if code != 0 || strings.Count(listed, artifactID) != 1 || !strings.Contains(listed, processID) {
		t.Fatalf("artifact list is incomplete: code=%d stderr=%s output=%s", code, stderr, listed)
	}
	shown, stderr, code := executeTestCommand(t, "artifact", "show", artifactID)
	if code != 0 || !strings.Contains(shown, "source: "+processID) ||
		!strings.Contains(shown, "tool: script_process") || !strings.Contains(shown, "redacted: true") ||
		strings.Contains(shown, "dry run:") || strings.Contains(shown, token) {
		t.Fatalf("artifact show leaked content or missed metadata: code=%d stderr=%s output=%s", code, stderr, shown)
	}
	content, stderr, code := executeTestCommand(t, "artifact", "read", artifactID, "--max-bytes", "4194304")
	if code != 0 || !strings.Contains(content, "dry run:") || !strings.Contains(content, "[REDACTED:") ||
		strings.Contains(content, token) {
		t.Fatalf("artifact read was not safely redacted: code=%d stderr=%s output=%s", code, stderr, content)
	}
	preview, stderr, code := executeTestCommand(t, "artifact", "read", artifactID, "--max-bytes", "12")
	if code != 0 || !strings.Contains(preview, "artifact preview truncated") {
		t.Fatalf("artifact read bound was not visible: code=%d stderr=%s output=%s", code, stderr, preview)
	}
	verified, stderr, code := executeTestCommand(t, "artifact", "verify", artifactID)
	if code != 0 || !strings.Contains(verified, "verified") || !strings.Contains(verified, "sha256:") {
		t.Fatalf("artifact verify failed: code=%d stderr=%s output=%s", code, stderr, verified)
	}
	toolDetail, stderr, code := executeTestCommand(t, "tool", "show", processID)
	if code != 0 || !strings.Contains(toolDetail, "artifact_stdout: "+artifactID) {
		t.Fatalf("tool detail did not link its artifact: code=%d stderr=%s output=%s", code, stderr, toolDetail)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(timeline, "artifact.created") != 1 || strings.Contains(timeline, token) {
		t.Fatalf("artifact event is missing or unsafe: code=%d stderr=%s output=%s", code, stderr, timeline)
	}
}

func TestArtifactCLIRejectsInvalidFiltersAndBounds(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	if _, stderr, code := executeTestCommand(t, "artifact", "list", "--stream", "combined"); code != 2 || !strings.Contains(stderr, "invalid artifact stream") {
		t.Fatalf("invalid artifact stream was not rejected: code=%d stderr=%s", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "artifact", "read", "artifact-missing", "--max-bytes", "0"); code != 2 || !strings.Contains(stderr, "max-bytes") {
		t.Fatalf("invalid artifact read bound was not rejected: code=%d stderr=%s", code, stderr)
	}
}
