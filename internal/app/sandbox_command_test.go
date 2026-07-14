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
var sandboxCandidateIDPattern = regexp.MustCompile(`sandbox-candidate-[0-9]{14}-[a-f0-9]{12}`)
var sandboxExecutionIDPattern = regexp.MustCompile(`sandbox-execution-[0-9]{14}-[a-f0-9]{12}`)
var sandboxPreflightIDPattern = regexp.MustCompile(`sandbox-preflight-[0-9]{14}-[a-f0-9]{12}`)

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
	candidate, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate", preparationID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-candidate-one")
	if code != 0 || stderr != "" || !strings.Contains(candidate, "budget_checked: true") ||
		!strings.Contains(candidate, "lease_quiescent: true") ||
		!strings.Contains(candidate, "execution_authorized: false") {
		t.Fatalf("sandbox CLI candidate failed output=%s stderr=%s code=%d", candidate, stderr, code)
	}
	candidateID := sandboxCandidateIDPattern.FindString(candidate)
	if candidateID == "" {
		t.Fatalf("missing sandbox candidate id: %s", candidate)
	}
	candidates, stderr, code := executeTestCommand(t, "run", "sandbox", "candidates", runID)
	if code != 0 || stderr != "" || !strings.Contains(candidates, candidateID) ||
		!strings.Contains(candidates, "execution_authorized=false") {
		t.Fatalf("sandbox CLI candidate list failed output=%s stderr=%s code=%d", candidates, stderr, code)
	}
	candidateShown, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate-show", candidateID)
	if code != 0 || stderr != "" || !strings.Contains(candidateShown, "mount_binding_fingerprint:") ||
		!strings.Contains(candidateShown, "backend_enabled: false") {
		t.Fatalf("sandbox CLI candidate show failed output=%s stderr=%s code=%d", candidateShown, stderr, code)
	}
	begun, stderr, code := executeTestCommand(t, "run", "sandbox", "begin", candidateID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-begin-operation")
	if code != 0 || stderr != "" || !strings.Contains(begun, "status: prepared") ||
		!strings.Contains(begun, "lease_status: released") ||
		!strings.Contains(begun, "backend_started: false") || strings.Contains(begun, "go test") ||
		strings.Contains(begun, "lease_id") || strings.Contains(begun, "owner_id") {
		t.Fatalf("sandbox CLI begin failed output=%s stderr=%s code=%d", begun, stderr, code)
	}
	executionID := sandboxExecutionIDPattern.FindString(begun)
	if executionID == "" {
		t.Fatalf("missing sandbox execution id: %s", begun)
	}
	beginReplay, stderr, code := executeTestCommand(t, "run", "sandbox", "begin", candidateID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-begin-operation")
	if code != 0 || stderr != "" || !strings.Contains(beginReplay, "execution: "+executionID) ||
		!strings.Contains(beginReplay, "replayed: true") {
		t.Fatalf("sandbox CLI begin replay failed output=%s stderr=%s code=%d", beginReplay, stderr, code)
	}
	executions, stderr, code := executeTestCommand(t, "run", "sandbox", "executions", runID)
	if code != 0 || stderr != "" || !strings.Contains(executions, executionID) ||
		!strings.Contains(executions, "backend_started=false") {
		t.Fatalf("sandbox CLI execution list failed output=%s stderr=%s code=%d", executions, stderr, code)
	}
	preflight, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight", executionID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-preflight-operation")
	if code != 0 || stderr != "" || !strings.Contains(preflight, "status: backend_disabled") ||
		!strings.Contains(preflight, "required_checks: 16") ||
		!strings.Contains(preflight, "verified_checks: 0") ||
		!strings.Contains(preflight, "partial_failure_policy: all_or_nothing") ||
		!strings.Contains(preflight, "artifact_commit_authorized: false") ||
		strings.Contains(preflight, "locator_fingerprint") ||
		strings.Contains(preflight, "container_identity_fingerprint") ||
		strings.Contains(preflight, "go test") {
		t.Fatalf("unexpected sandbox preflight output=%s stderr=%s code=%d", preflight, stderr, code)
	}
	preflightID := sandboxPreflightIDPattern.FindString(preflight)
	if preflightID == "" {
		t.Fatalf("missing sandbox preflight id: %s", preflight)
	}
	preflightReplay, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight", executionID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-preflight-operation")
	if code != 0 || stderr != "" || !strings.Contains(preflightReplay, "preflight: "+preflightID) ||
		!strings.Contains(preflightReplay, "replayed: true") {
		t.Fatalf("sandbox preflight replay failed output=%s stderr=%s code=%d", preflightReplay, stderr, code)
	}
	preflights, stderr, code := executeTestCommand(t, "run", "sandbox", "preflights", runID)
	if code != 0 || stderr != "" || !strings.Contains(preflights, preflightID) ||
		!strings.Contains(preflights, "backend_enabled=false") {
		t.Fatalf("sandbox preflight list failed output=%s stderr=%s code=%d", preflights, stderr, code)
	}
	preflightShown, stderr, code := executeTestCommand(t, "run", "sandbox", "preflight-show", preflightID)
	if code != 0 || stderr != "" || !strings.Contains(preflightShown, "network_default_deny") ||
		!strings.Contains(preflightShown, "kind=stdout") ||
		strings.Contains(preflightShown, "locator_fingerprint") {
		t.Fatalf("sandbox preflight show failed output=%s stderr=%s code=%d", preflightShown, stderr, code)
	}
	cancelled, stderr, code := executeTestCommand(t, "run", "sandbox", "cancel", executionID,
		"--operation-key", "sandbox-cli-cancel-operation")
	if code != 0 || stderr != "" || !strings.Contains(cancelled, "status: cancel_requested") ||
		!strings.Contains(cancelled, "cancellation_requested: true") {
		t.Fatalf("sandbox CLI cancellation failed output=%s stderr=%s code=%d", cancelled, stderr, code)
	}
	cleaned, stderr, code := executeTestCommand(t, "run", "sandbox", "cleanup", executionID,
		"--operation-key", "sandbox-cli-cleanup-operation")
	if code != 0 || stderr != "" || !strings.Contains(cleaned, "status: cleaned") ||
		!strings.Contains(cleaned, "cleanup_outcome: backend_disabled") ||
		!strings.Contains(cleaned, "input_artifacts_verified: true") ||
		!strings.Contains(cleaned, "output_artifacts: 0") {
		t.Fatalf("sandbox CLI cleanup failed output=%s stderr=%s code=%d", cleaned, stderr, code)
	}
	executionShown, stderr, code := executeTestCommand(t, "run", "sandbox", "execution-show", executionID)
	if code != 0 || stderr != "" || !strings.Contains(executionShown, "cleanup_complete: true") ||
		strings.Contains(executionShown, "lease_id") || strings.Contains(executionShown, "owner_id") {
		t.Fatalf("sandbox CLI execution show failed output=%s stderr=%s code=%d", executionShown, stderr, code)
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

func TestSandboxCLIApprovalRequestReviewAndDisabledCandidate(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	manifest := defaultSandboxManifestTemplate()
	manifest.Mounts[0].Access = "read_write"
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "sandbox-write-manifest.json")
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "sandbox-approval-demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "sandbox approval lifecycle",
		"--workspace", "sandbox-approval-demo", "--profile", "code")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	prepared, stderr, code := executeTestCommand(t, "run", "sandbox", "prepare", runID,
		"--manifest", manifestPath, "--operation-key", "sandbox-cli-approval-prepare")
	if code != 0 || stderr != "" || !strings.Contains(prepared, "approval_status: required") {
		t.Fatalf("approval preparation failed output=%s stderr=%s code=%d", prepared, stderr, code)
	}
	preparationID := sandboxPreparationIDPattern.FindString(prepared)
	requested, stderr, code := executeTestCommand(t, "run", "sandbox", "request", preparationID,
		"--operator", "sandbox_cli_operator")
	if code != 0 || stderr != "" || !strings.Contains(requested, "status: pending") ||
		!strings.Contains(requested, "tool: sandbox.manifest") {
		t.Fatalf("approval request failed output=%s stderr=%s code=%d", requested, stderr, code)
	}
	approvalID := approvalIDPattern.FindString(requested)
	if approvalID == "" {
		t.Fatalf("approval request did not return an id: %s", requested)
	}
	reviewed, stderr, code := executeTestCommand(t, "run", "sandbox", "review", preparationID,
		"--decision", "approve", "--operation-key", "sandbox-cli-approval-review",
		"--reviewer", "sandbox_security_operator")
	if code != 0 || stderr != "" || !strings.Contains(reviewed, "status: approved") {
		t.Fatalf("approval review failed output=%s stderr=%s code=%d", reviewed, stderr, code)
	}
	candidate, stderr, code := executeTestCommand(t, "run", "sandbox", "candidate", preparationID,
		"--manifest", manifestPath, "--approval", approvalID,
		"--operation-key", "sandbox-cli-approved-candidate",
		"--operator", "sandbox_cli_operator")
	if code != 0 || stderr != "" || !strings.Contains(candidate, "approval_status: approved") ||
		!strings.Contains(candidate, "backend_enabled: false") ||
		!strings.Contains(candidate, "execution_authorized: false") {
		t.Fatalf("approved disabled candidate failed output=%s stderr=%s code=%d", candidate, stderr, code)
	}
}
