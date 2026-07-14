package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/tools"
)

func TestSandboxManifestServicePreparesReplaysAndRejectsChangedIntent(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest := sandboxManifestTestFixture()
	request := PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: manifest, OperationKey: "sandbox-operation-one",
		RequestedBy: "test_operator",
	}
	first, err := service.Prepare(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || !first.Validation.PolicyAllowed || first.Validation.NeedsApproval ||
		first.Validation.ApprovalStatus != sandbox.ApprovalNotRequired ||
		first.Validation.BackendEnabled || first.Validation.ExecutionAuthorized ||
		first.Preparation.CancellationID == "" {
		t.Fatalf("unexpected sandbox preparation: %#v", first)
	}
	replayed, err := service.Prepare(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Preparation.ID != first.Preparation.ID ||
		replayed.Preparation.CancellationID != first.Preparation.CancellationID {
		t.Fatalf("sandbox replay did not converge: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Manifest.Command.Arguments = []string{"test", "./internal/..."}
	if _, err := service.Prepare(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed sandbox intent reused an operation key: %v", err)
	}
	if _, err := NewRunService(st).Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	terminalReplay, err := service.Prepare(ctx, request)
	if err != nil || !terminalReplay.Replayed ||
		terminalReplay.Preparation.ID != first.Preparation.ID {
		t.Fatalf("terminal Run replay did not return the immutable preparation: %#v err=%v",
			terminalReplay, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("manifest preparation created workspace side effects: %#v", entries)
	}
}

func TestSandboxManifestServicePersistsPolicyDenialWithoutRawIntent(t *testing.T) {
	ctx := context.Background()
	st, run, _ := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest := sandboxManifestTestFixture()
	secretMarker := "masscan-secret-command-marker"
	manifest.Command.Executable = "masscan"
	manifest.Command.Arguments = []string{"0.0.0.0/0", secretMarker}
	result, err := service.Prepare(ctx, PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: manifest, OperationKey: "sandbox-denied-one",
		RequestedBy: "test_operator",
	})
	if apperror.CodeOf(err) != apperror.CodePolicyDenied || result.Preparation.ID == "" ||
		result.Validation.PolicyAllowed || result.Validation.ApprovalStatus != sandbox.ApprovalNotApplicable {
		t.Fatalf("policy denial was not durably recorded: %#v err=%v", result, err)
	}
	stored, err := service.Get(ctx, result.Preparation.ID)
	if err != nil || stored.Preparation.ID != result.Preparation.ID {
		t.Fatalf("denied preparation was not readable: %#v err=%v", stored, err)
	}
	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(event.PayloadJSON, secretMarker) || strings.Contains(event.PayloadJSON, "0.0.0.0/0") ||
			strings.Contains(event.PayloadJSON, `"executable"`) {
			t.Fatalf("sandbox event leaked raw intent: %#v", event)
		}
	}
}

func TestSandboxManifestServiceRefusesScopeWideningAndMarksApprovalBoundary(t *testing.T) {
	ctx := context.Background()
	st, run, _ := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	widened := sandboxManifestTestFixture()
	widened.Network = sandbox.NetworkScope{Mode: "allowlist", AllowedTargets: []string{"example.com"}}
	if _, err := service.Prepare(ctx, PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: widened, OperationKey: "sandbox-widen-one",
	}); apperror.CodeOf(err) != apperror.CodePolicyDenied {
		t.Fatalf("Mission scope widening was not denied: %v", err)
	}
	values, err := service.List(ctx, run.ID, 100)
	if err != nil || len(values) != 0 {
		t.Fatalf("scope widening should not persist a preparation: %#v err=%v", values, err)
	}

	writable := sandboxManifestTestFixture()
	writable.Mounts[0].Access = sandbox.MountReadWrite
	prepared, err := service.Prepare(ctx, PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: writable, OperationKey: "sandbox-write-one",
	})
	if err != nil || !prepared.Validation.PolicyAllowed || !prepared.Validation.NeedsApproval ||
		prepared.Validation.ApprovalStatus != sandbox.ApprovalRequired ||
		prepared.Validation.ExecutionAuthorized {
		t.Fatalf("write capability approval boundary is invalid: %#v err=%v", prepared, err)
	}
}

func TestSandboxManifestServiceConservativelyRequiresApprovalForHighRiskPolicy(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, sandboxHighRiskChecker{})
	prepared, err := service.Prepare(ctx, PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: sandboxManifestTestFixture(),
		OperationKey: "sandbox-high-risk-policy", RequestedBy: "test_operator",
	})
	if err != nil || !prepared.Validation.NeedsApproval ||
		prepared.Validation.ApprovalStatus != sandbox.ApprovalRequired ||
		prepared.Validation.Risk != "high" {
		t.Fatalf("high-risk allowed Policy did not require approval: %#v err=%v", prepared, err)
	}
	if _, err := validateSandboxWorkspaceBinding(sandbox.WorkspaceBinding{
		ID: "ws-sandbox", RootPath: root + " ",
	}); err == nil {
		t.Fatal("sandbox workspace binding silently trimmed the persisted root")
	}
	if _, err := normalizeSandboxMissionScope(domain.Scope{
		WorkspaceID: "ws-sandbox", NetworkMode: " disabled ",
	}); err == nil {
		t.Fatal("sandbox Mission scope silently trimmed persisted policy fields")
	}
}

func newSandboxManifestTestRuntime(t *testing.T, ctx context.Context,
) (*store.SQLiteStore, domain.Run, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	root := t.TempDir()
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{
		ID: "ws-sandbox", Name: "sandbox", RootPath: root,
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := NewRunService(st).Create(ctx, CreateRunRequest{
		Goal: "validate a bounded sandbox manifest", Profile: "code",
		WorkspaceID: "ws-sandbox", Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, run, root
}

func sandboxManifestTestFixture() sandbox.Manifest {
	return sandbox.Manifest{
		ProtocolVersion: sandbox.ManifestProtocolVersion,
		Backend:         sandbox.BackendNoop,
		Command: sandbox.CommandSpec{
			Executable: "go", Arguments: []string{"test", "./..."},
			WorkingDirectory: "/workspace",
		},
		Mounts: []sandbox.Mount{{
			Source: ".", Target: "/workspace", Access: sandbox.MountReadOnly,
		}},
		Network: sandbox.NetworkScope{Mode: "disabled"},
		Resources: sandbox.ResourceLimits{
			CPUQuotaMillis: 1000, MemoryBytes: 256 * 1024 * 1024,
			PIDs: 64, MaxOutputBytes: 4 * 1024 * 1024,
		},
		Output:         sandbox.OutputSpec{CaptureStdout: true, CaptureStderr: true},
		TimeoutSeconds: 300,
		Cancellation:   sandbox.CancellationSpec{GracePeriodMillis: 2000},
	}
}

type sandboxHighRiskChecker struct{}

func (sandboxHighRiskChecker) CheckText(context string, text string) policy.Decision {
	return policy.Decision{Allowed: true, Reason: "custom high-risk decision", Risk: "high"}
}

func (sandboxHighRiskChecker) CheckToolCall(call tools.Call) policy.Decision {
	return policy.Decision{Allowed: true, Reason: "custom high-risk decision", Risk: "high"}
}
