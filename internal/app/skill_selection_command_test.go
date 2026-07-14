package app

import (
	"regexp"
	"strings"
	"testing"
)

var skillSelectionIDPattern = regexp.MustCompile(`skill-selection-[0-9]{14}-[a-f0-9]{12}`)

func TestSkillSelectionCLIIsPinnedReplayableAndMetadataOnly(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	created, stderr, code := executeTestCommand(t, "run", "create", "Skill CLI", "--profile", "code")
	if code != 0 || stderr != "" {
		t.Fatalf("run create failed: code=%d stderr=%q output=%q", code, stderr, created)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("run id missing: %q", created)
	}
	operationKey := "skill-selection-cli-0001"
	selected, stderr, code := executeTestCommand(t, "skill", "select", runID, "code",
		"--operation-key", operationKey, "--token-budget", "4096")
	selectionID := skillSelectionIDPattern.FindString(selected)
	if code != 0 || stderr != "" || selectionID == "" ||
		!strings.Contains(selected, "protocol: skill_selection.v1") ||
		!strings.Contains(selected, "skill[1]: code@1.1.0") ||
		!strings.Contains(selected, "replayed: false") ||
		!strings.Contains(selected, "context_injection: root_selected_and_specialist_minimized") ||
		!strings.Contains(selected, "tool_capability_grant: disabled") ||
		strings.Contains(selected, "SKILL.md") || strings.Contains(selected, "tool_dependencies") {
		t.Fatalf("Skill selection failed: code=%d stderr=%q output=%q", code, stderr, selected)
	}
	replayed, stderr, code := executeTestCommand(t, "skill", "select", runID, "code",
		"--token-budget", "4096", "--operation-key", operationKey)
	if code != 0 || stderr != "" || skillSelectionIDPattern.FindString(replayed) != selectionID ||
		!strings.Contains(replayed, "replayed: true") {
		t.Fatalf("Skill selection replay failed: code=%d stderr=%q output=%q", code, stderr, replayed)
	}
	shown, stderr, code := executeTestCommand(t, "skill", "selection", runID)
	if code != 0 || stderr != "" || skillSelectionIDPattern.FindString(shown) != selectionID ||
		!strings.Contains(shown, "selection_fingerprint:") {
		t.Fatalf("Skill selection show failed: code=%d stderr=%q output=%q", code, stderr, shown)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || stderr != "" || strings.Count(timeline, "skill.selection_created") != 1 ||
		strings.Contains(timeline, operationKey) || strings.Contains(timeline, "content_sha256") ||
		strings.Contains(timeline, "selection_fingerprint") || strings.Contains(timeline, "SKILL.md") {
		t.Fatalf("Skill selection timeline leaked provenance: code=%d stderr=%q output=%q", code, stderr, timeline)
	}
	if _, stderr, code := executeTestCommand(t, "run", "start", runID); code != 0 || stderr != "" {
		t.Fatalf("Run start after selection failed: code=%d stderr=%q", code, stderr)
	}
	if replayed, stderr, code := executeTestCommand(t, "skill", "select", runID, "code",
		"--operation-key", operationKey); code != 0 || stderr != "" ||
		!strings.Contains(replayed, "replayed: true") {
		t.Fatalf("post-start replay failed: code=%d stderr=%q output=%q", code, stderr, replayed)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "select", runID, "code",
		"--operation-key", "skill-selection-cli-0002"); code != 4 ||
		!strings.Contains(stderr, "before a Run starts") {
		t.Fatalf("post-start replacement was not denied: code=%d stderr=%q", code, stderr)
	}
}

func TestSkillSelectionCLIRejectsProfileBudgetAndMissingSelection(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	created, stderr, code := executeTestCommand(t, "run", "create", "Review Skill CLI", "--profile", "review")
	if code != 0 {
		t.Fatalf("run create failed: code=%d stderr=%q", code, stderr)
	}
	runID := runIDPattern.FindString(created)
	if _, stderr, code := executeTestCommand(t, "skill", "selection", runID); code != 3 ||
		!strings.Contains(stderr, "not found") {
		t.Fatalf("missing selection was unstable: code=%d stderr=%q", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "select", runID, "code",
		"--operation-key", "skill-selection-invalid-0001"); code != 2 ||
		!strings.Contains(stderr, "incompatible") {
		t.Fatalf("profile mismatch was unstable: code=%d stderr=%q", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "select", runID, "review",
		"--operation-key", "skill-selection-invalid-0002", "--token-budget", "1"); code != 2 ||
		!strings.Contains(stderr, "token upper bound") {
		t.Fatalf("token budget mismatch was unstable: code=%d stderr=%q", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "select", runID, "review"); code != 2 ||
		!strings.Contains(stderr, "operation key") {
		t.Fatalf("missing operation key was unstable: code=%d stderr=%q", code, stderr)
	}
}
