package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var sandboxPreparationIDPattern = regexp.MustCompile(`sandbox-manifest-[0-9]{14}-[a-f0-9]{12}`)

func TestSandboxCLIValidatesPreparesListsAndShowsMetadataOnly(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	template, stderr, code := executeTestCommand(t, "sandbox", "template")
	if code != 0 || stderr != "" || !strings.Contains(template, `"protocol_version": "sandbox_manifest.v1"`) ||
		!strings.Contains(template, `"backend": "noop"`) {
		t.Fatalf("unexpected sandbox template output=%s stderr=%s code=%d", template, stderr, code)
	}
	manifestPath := filepath.Join(t.TempDir(), "sandbox-manifest.json")
	if err := os.WriteFile(manifestPath, []byte(template), 0o600); err != nil {
		t.Fatal(err)
	}
	validated, stderr, code := executeTestCommand(t, "sandbox", "validate", manifestPath)
	if code != 0 || stderr != "" || !strings.Contains(validated, "valid: true") ||
		!strings.Contains(validated, "validator: noop") ||
		!strings.Contains(validated, "execution_authorized: false") {
		t.Fatalf("unexpected sandbox validation output=%s stderr=%s code=%d", validated, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "sandbox-demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "sandbox cli lifecycle",
		"--workspace", "sandbox-demo", "--profile", "code")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("missing Run id: %s", created)
	}
	prepared, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-operation-one")
	if code != 0 || stderr != "" || !strings.Contains(prepared, "policy_allowed: true") ||
		!strings.Contains(prepared, "approval_status: not_required") ||
		!strings.Contains(prepared, "execution_authorized: false") ||
		strings.Contains(prepared, "go test") {
		t.Fatalf("unexpected sandbox preparation output=%s stderr=%s code=%d", prepared, stderr, code)
	}
	preparationID := sandboxPreparationIDPattern.FindString(prepared)
	if preparationID == "" {
		t.Fatalf("missing sandbox preparation id: %s", prepared)
	}
	replayed, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-operation-one")
	if code != 0 || !strings.Contains(replayed, "preparation: "+preparationID) ||
		!strings.Contains(replayed, "replayed: true") {
		t.Fatalf("sandbox CLI replay failed output=%s stderr=%s code=%d", replayed, stderr, code)
	}
	listed, stderr, code := executeTestCommand(t, "run", "sandbox", "list", runID)
	if code != 0 || !strings.Contains(listed, preparationID) ||
		!strings.Contains(listed, "execution_authorized=false") {
		t.Fatalf("sandbox CLI list failed output=%s stderr=%s code=%d", listed, stderr, code)
	}
	shown, stderr, code := executeTestCommand(t, "run", "sandbox", "show", preparationID)
	if code != 0 || !strings.Contains(shown, "manifest_fingerprint:") ||
		!strings.Contains(shown, "backend_enabled: false") || strings.Contains(shown, `"arguments"`) {
		t.Fatalf("sandbox CLI show failed output=%s stderr=%s code=%d", shown, stderr, code)
	}
	if _, stderr, code := executeTestCommand(t, "run", "sandbox", "list", runID,
		"--limit", "-1"); code != 2 || !strings.Contains(stderr, "between 1 and 200") {
		t.Fatalf("sandbox CLI accepted a negative list limit: code=%d stderr=%s", code, stderr)
	}
}

func TestSandboxCLIRejectsAmbiguousManifest(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	manifest := defaultSandboxManifestTemplate()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	malformed := strings.Replace(string(encoded), `"backend":"noop"`,
		`"backend":"noop","backend":"docker"`, 1)
	path := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(path, []byte(malformed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := executeTestCommand(t, "sandbox", "validate", path); code != 2 ||
		!strings.Contains(stderr, "duplicate field") {
		t.Fatalf("ambiguous sandbox manifest returned code=%d stderr=%s", code, stderr)
	}
}
