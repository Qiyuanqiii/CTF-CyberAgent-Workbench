package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillCLIListsShowsAndValidatesBuiltinsWithoutRuntimeState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)

	listed, stderr, code := executeTestCommand(t, "skill", "list")
	if code != 0 || stderr != "" || !strings.Contains(listed, "code@1.1.0") ||
		!strings.Contains(listed, "learn@1.1.0") || !strings.Contains(listed, "review@1.1.0") ||
		!strings.Contains(listed, "plan-delivery@1.1.0") ||
		!strings.Contains(listed, "script@1.1.0") || !strings.Contains(listed, "context_injection: root_selected_only") ||
		!strings.Contains(listed, "tool_capability_grant: disabled") {
		t.Fatalf("unexpected skill list: code=%d stderr=%q output=%q", code, stderr, listed)
	}
	if strings.Index(listed, "code@") > strings.Index(listed, "learn@") ||
		strings.Index(listed, "learn@") > strings.Index(listed, "plan-delivery@") ||
		strings.Index(listed, "plan-delivery@") > strings.Index(listed, "review@") ||
		strings.Index(listed, "review@") > strings.Index(listed, "script@") {
		t.Fatalf("skill list is not deterministic: %q", listed)
	}

	filtered, stderr, code := executeTestCommand(t, "skill", "list", "--profile", "review")
	if code != 0 || stderr != "" || !strings.Contains(filtered, "review@1.1.0") ||
		!strings.Contains(filtered, "plan-delivery@1.1.0") ||
		strings.Contains(filtered, "code@1.1.0") || strings.Contains(filtered, "script@1.1.0") {
		t.Fatalf("unexpected profile filter: code=%d stderr=%q output=%q", code, stderr, filtered)
	}

	shown, stderr, code := executeTestCommand(t, "skill", "show", "code")
	if code != 0 || stderr != "" || !strings.Contains(shown, "protocol: skill.v1") ||
		!strings.Contains(shown, "tool_dependencies: list_workspace,read_file,replace_file") ||
		!strings.Contains(shown, "content_sha256: 279113f9") ||
		strings.Contains(shown, "The current runtime does not inject") {
		t.Fatalf("unexpected skill show: code=%d stderr=%q output=%q", code, stderr, shown)
	}

	validated, stderr, code := executeTestCommand(t, "skill", "validate")
	if code != 0 || stderr != "" || !strings.Contains(validated, "validated 5 built-in skill.v1 manifests") {
		t.Fatalf("unexpected skill validation: code=%d stderr=%q output=%q", code, stderr, validated)
	}
	if _, err := os.Stat(filepath.Join(home, "cyberagent.db")); !os.IsNotExist(err) {
		t.Fatalf("read-only skill commands created runtime state: %v", err)
	}
}

func TestSkillCLIRejectsInvalidProfileNameAndValidationArguments(t *testing.T) {
	if _, stderr, code := executeTestCommand(t, "skill", "list", "--profile", "admin"); code != 2 || !strings.Contains(stderr, "unsupported profile") {
		t.Fatalf("invalid profile was unstable: code=%d stderr=%q", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "show", "missing"); code != 3 || !strings.Contains(stderr, "not found") {
		t.Fatalf("missing skill was unstable: code=%d stderr=%q", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "skill", "validate", "external.json"); code != 2 || !strings.Contains(stderr, "usage:") {
		t.Fatalf("external validation path was accepted: code=%d stderr=%q", code, stderr)
	}
	help, stderr, code := executeTestCommand(t, "help")
	if code != 0 || stderr != "" || !strings.Contains(help, "cyberagent skill list|show|validate") {
		t.Fatalf("skill help is missing: code=%d stderr=%q output=%q", code, stderr, help)
	}
}
