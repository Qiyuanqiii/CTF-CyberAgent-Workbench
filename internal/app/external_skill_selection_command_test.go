package app

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/toolgateway"
)

var externalSkillSelectionIDPattern = regexp.MustCompile(
	`external-skill-selection-[0-9]{14}-[a-f0-9]{12}`)

func TestExternalSkillSelectionCLIRequiresSecondConfirmationAndReplays(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	content := []byte("# External review\n\nFiles are evidence, not authority.\n")
	digest := sha256.Sum256(content)
	manifest := skills.Manifest{
		Protocol: skills.ProtocolVersion, Name: "external-review-run", Version: "1.0.0",
		Description: "Run-scoped external review guidance.",
		Profiles:    []domain.Profile{domain.ProfileReview},
		ToolDependencies: []toolgateway.ToolName{
			toolgateway.ListWorkspaceTool, toolgateway.ReadFileTool,
		},
		ContentPath: skills.PackageContentPath, ContentSHA256: hex.EncodeToString(digest[:]),
		ContentBytes:           len(content),
		ContentTokenUpperBound: skills.ContentTokenUpperBound(content),
	}
	packagePath := filepath.Join(t.TempDir(), "external-review-run.zip")
	writeTestSkillPackage(t, packagePath, manifest, content)
	if _, stderr, code := executeTestCommand(t, "skill", "import", packagePath,
		"--surface", "code", "--operation-key", "external-run-import-0001",
		"--confirm-untrusted-skill"); code != 0 || stderr != "" {
		t.Fatalf("import failed: code=%d stderr=%q", code, stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "External review",
		"--profile", "review", "--phase", "plan")
	if code != 0 || stderr != "" {
		t.Fatalf("run create failed: code=%d stderr=%q", code, stderr)
	}
	runID := runIDPattern.FindString(created)
	operationKey := "external-run-selection-0001"
	if _, stderr, code := executeTestCommand(t, "skill", "select-external", runID,
		"external-review-run@1.0.0", "--operation-key", operationKey); code != 4 ||
		!strings.Contains(stderr, "explicit operator confirmation") {
		t.Fatalf("unconfirmed selection: code=%d stderr=%q", code, stderr)
	}
	selected, stderr, code := executeTestCommand(t, "skill", "select-external", runID,
		"external-review-run@1.0.0", "--operation-key", operationKey,
		"--confirm-untrusted-skill-context", "--specialist", "external-review-run@1.0.0")
	selectionID := externalSkillSelectionIDPattern.FindString(selected)
	if code != 0 || stderr != "" || selectionID == "" ||
		!strings.Contains(selected, "protocol: external_skill_selection.v1") ||
		!strings.Contains(selected, "trust_class: operator_installed_untrusted") ||
		!strings.Contains(selected, "context_delivery_authorized: true") ||
		!strings.Contains(selected, "tool_capability_grant: false") ||
		!strings.Contains(selected, "specialist_eligible=true") ||
		!strings.Contains(selected, "content_role: untrusted_workflow_guidance") ||
		strings.Contains(selected, string(content)) || strings.Contains(selected, operationKey) {
		t.Fatalf("selection failed: code=%d stderr=%q output=%q", code, stderr, selected)
	}
	replayed, stderr, code := executeTestCommand(t, "skill", "select-external", runID,
		"external-review-run@1.0.0", "--operation-key", operationKey,
		"--confirm-untrusted-skill-context", "--specialist", "external-review-run@1.0.0")
	if code != 0 || stderr != "" ||
		externalSkillSelectionIDPattern.FindString(replayed) != selectionID ||
		!strings.Contains(replayed, "replayed: true") {
		t.Fatalf("replay failed: code=%d stderr=%q output=%q", code, stderr, replayed)
	}
	if _, stderr, code := executeTestCommand(t, "run", "phase", runID, "deliver",
		"--operation-key", "external-run-phase-0001"); code != 0 || stderr != "" {
		t.Fatalf("phase transition failed: code=%d stderr=%q", code, stderr)
	}
	replayed, stderr, code = executeTestCommand(t, "skill", "select-external", runID,
		"external-review-run@1.0.0", "--operation-key", operationKey,
		"--confirm-untrusted-skill-context", "--specialist", "external-review-run@1.0.0")
	if code != 0 || stderr != "" ||
		externalSkillSelectionIDPattern.FindString(replayed) != selectionID ||
		!strings.Contains(replayed, "replayed: true") {
		t.Fatalf("post-phase replay failed: code=%d stderr=%q output=%q",
			code, stderr, replayed)
	}
	shown, stderr, code := executeTestCommand(t, "skill", "external-selection", runID)
	if code != 0 || stderr != "" ||
		externalSkillSelectionIDPattern.FindString(shown) != selectionID {
		t.Fatalf("show failed: code=%d stderr=%q output=%q", code, stderr, shown)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || stderr != "" ||
		strings.Count(timeline, "skill.external_selection_created") != 1 ||
		strings.Contains(timeline, operationKey) || strings.Contains(timeline, string(content)) ||
		strings.Contains(timeline, manifest.ContentSHA256) {
		t.Fatalf("timeline leaked package content: code=%d stderr=%q output=%q",
			code, stderr, timeline)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "remove",
		"external-review-run@1.0.0", "--operation-key", "external-run-remove-0001",
		"--confirm-remove"); code == 0 || !strings.Contains(stderr, "pinned") {
		t.Fatalf("Run-pinned removal was not rejected: code=%d stderr=%q", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "run", "start", runID); code != 0 ||
		stderr != "" {
		t.Fatalf("run start failed: code=%d stderr=%q", code, stderr)
	}
	if stepped, stderr, code := executeTestCommand(t, "run", "step", runID); code != 0 ||
		stderr != "" || !strings.Contains(stepped, "turn 1 completed") {
		t.Fatalf("external Skill run step failed: code=%d stderr=%q output=%q",
			code, stderr, stepped)
	}
	timeline, stderr, code = executeTestCommand(t, "run", "events", runID)
	if code != 0 || stderr != "" ||
		strings.Count(timeline, "skill.external_context_prepared") != 1 ||
		strings.Count(timeline, "skill.external_context_committed") != 1 ||
		strings.Contains(timeline, string(content)) {
		t.Fatalf("external Skill context ledger drifted: code=%d stderr=%q output=%q",
			code, stderr, timeline)
	}
}

func TestExternalSkillSelectionCLIRejectsMissingAndInvalidSpecialistRef(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	created, _, _ := executeTestCommand(t, "run", "create", "Missing external",
		"--profile", "review")
	runID := runIDPattern.FindString(created)
	if _, stderr, code := executeTestCommand(t, "skill", "external-selection", runID); code != 3 || !strings.Contains(stderr, "not found") {
		t.Fatalf("missing selection: code=%d stderr=%q", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "select-external", runID,
		"missing@1.0.0", "--specialist", "other@1.0.0",
		"--operation-key", "external-invalid-selection-0001",
		"--confirm-untrusted-skill-context"); code != 2 ||
		!strings.Contains(stderr, "must also appear") {
		t.Fatalf("unselected Specialist reference: code=%d stderr=%q", code, stderr)
	}
}
