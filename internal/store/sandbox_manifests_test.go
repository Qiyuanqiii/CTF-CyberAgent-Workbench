package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
)

func TestSandboxManifestLedgerBindsApprovalAndRemainsImmutable(t *testing.T) {
	ctx := context.Background()
	st, run, _ := openSandboxManifestStore(t, ctx)
	service := application.NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest := sandboxStoreTestManifest()
	manifest.Mounts[0].Access = sandbox.MountReadWrite
	first, err := service.Prepare(ctx, application.PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: manifest, OperationKey: "sandbox-store-first",
		RequestedBy: "store_test",
	})
	if err != nil || first.Validation.ApprovalStatus != sandbox.ApprovalRequired {
		t.Fatalf("approval-required preparation failed: %#v err=%v", first, err)
	}
	approvedAt := time.Now().UTC()
	if _, err := st.db.ExecContext(ctx, `INSERT INTO tool_approvals
		(id, idempotency_key, proposal_id, run_id, session_id, workspace_id,
		tool_name, action_class, mode, status, request_fingerprint, decision_reason,
		requested_by, reviewed_by, version, created_at, updated_at, decided_at)
		VALUES (?, ?, ?, ?, ?, ?, 'sandbox.manifest', 'sandbox_execute', 'per_call',
		'approved', ?, '', 'store_test', 'security_reviewer', 1, ?, ?, ?)`,
		"approval-sandbox-exact", "approval-sandbox-exact-key", "approval-sandbox-exact-proposal",
		run.ID, run.SessionID, "ws-sandbox-store", first.Preparation.AuthorizationFingerprint,
		ts(approvedAt), ts(approvedAt), ts(approvedAt)); err != nil {
		t.Fatal(err)
	}
	bound, err := service.Prepare(ctx, application.PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: manifest, ApprovalID: "approval-sandbox-exact",
		OperationKey: "sandbox-store-approved", RequestedBy: "store_test",
	})
	if err != nil || bound.Validation.ApprovalStatus != sandbox.ApprovalApproved ||
		bound.Validation.ExecutionAuthorized || bound.Validation.BackendEnabled {
		t.Fatalf("exact approval binding widened execution authority: %#v err=%v", bound, err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE sandbox_manifest_preparations
		SET backend = 'docker' WHERE id = ?`, bound.Preparation.ID); err == nil {
		t.Fatal("sandbox preparation was mutable")
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM sandbox_manifest_validations
		WHERE preparation_id = ?`, bound.Preparation.ID); err == nil {
		t.Fatal("sandbox validation was deletable")
	}
	var executionAuthorized int
	if err := st.db.QueryRowContext(ctx, `SELECT execution_authorized
		FROM sandbox_manifest_validations WHERE preparation_id = ?`, bound.Preparation.ID).
		Scan(&executionAuthorized); err != nil || executionAuthorized != 0 {
		t.Fatalf("sandbox validation authorized execution: value=%d err=%v", executionAuthorized, err)
	}
	forgedPreparation := first.Preparation
	forgedPreparation.ID = "sandbox-manifest-forged"
	forgedPreparation.CancellationID = "sandbox-cancel-forged"
	forgedPreparation.PreparedAt = first.Preparation.PreparedAt.Add(time.Second)
	forgedValidation := first.Validation
	forgedValidation.PreparationID = forgedPreparation.ID
	forgedValidation.NeedsApproval = false
	forgedValidation.ApprovalID = ""
	forgedValidation.ApprovalStatus = sandbox.ApprovalNotRequired
	forgedValidation.ValidatedAt = forgedPreparation.PreparedAt
	forgedOperation := sandbox.Operation{
		KeyDigest:     runmutation.Fingerprint("sandbox-test-operation", "forged"),
		PreparationID: forgedPreparation.ID, RunID: forgedPreparation.RunID,
		RequestedBy: forgedPreparation.RequestedBy, CreatedAt: forgedPreparation.PreparedAt,
	}
	forgedOperation.RequestFingerprint = sandbox.IntentRequestFingerprint(forgedPreparation,
		forgedValidation)
	if _, _, err := st.CreateSandboxManifestIntent(ctx, forgedPreparation, forgedValidation,
		forgedOperation); err == nil {
		t.Fatal("schema v48 accepted a write-capable manifest without required approval")
	}
	var forgedCount int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_manifest_preparations
		WHERE id = ?`, forgedPreparation.ID).Scan(&forgedCount); err != nil || forgedCount != 0 {
		t.Fatalf("rejected forged preparation was partially persisted: count=%d err=%v", forgedCount, err)
	}

	rows, err := st.db.QueryContext(ctx, `PRAGMA table_info(sandbox_manifest_preparations)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		switch name {
		case "executable", "arguments_json", "manifest_json", "environment_json",
			"allowed_targets_json", "mounts_json", "workspace_root", "command_text":
			t.Fatalf("schema v48 persists raw sandbox intent in column %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxManifestConcurrentReplayConvergesAcrossStores(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "cyberagent.db")
	st1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st1.Close() })
	root := t.TempDir()
	if err := st1.SaveWorkspace(ctx, WorkspaceRecord{
		ID: "ws-sandbox-race", Name: "sandbox-race", RootPath: root,
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st1).Create(ctx, application.CreateRunRequest{
		Goal: "converge sandbox preparation", Profile: "code", WorkspaceID: "ws-sandbox-race",
		Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	services := []*application.SandboxManifestService{
		application.NewSandboxManifestService(st1, policy.NewDefaultChecker()),
		application.NewSandboxManifestService(st2, policy.NewDefaultChecker()),
	}
	request := application.PrepareSandboxManifestRequest{
		RunID: run.ID, Manifest: sandboxStoreTestManifest(),
		OperationKey: "sandbox-concurrent-one", RequestedBy: "race_test",
	}
	start := make(chan struct{})
	results := make([]sandbox.PreparedIntent, 2)
	errorsFound := make([]error, 2)
	var group sync.WaitGroup
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index], errorsFound[index] = services[index].Prepare(ctx, request)
		}(index)
	}
	close(start)
	group.Wait()
	if errorsFound[0] != nil || errorsFound[1] != nil ||
		results[0].Preparation.ID != results[1].Preparation.ID {
		t.Fatalf("concurrent sandbox replay diverged: results=%#v errors=%v", results, errorsFound)
	}
	if results[0].Replayed == results[1].Replayed {
		t.Fatalf("expected one create and one replay: %#v", results)
	}
	values, err := st1.ListSandboxManifestIntents(ctx, run.ID, 100)
	if err != nil || len(values) != 1 {
		t.Fatalf("concurrent preparation created duplicate rows: %#v err=%v", values, err)
	}
}

func TestSchemaV47UpgradeAddsSandboxManifestLedgerWithoutLosingRun(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v47.db")
	st, run, _ := openSandboxManifestStoreAt(t, ctx, path)
	for _, statement := range removeSchemaV48ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("simulate schema v47 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	loaded, err := st.GetRun(ctx, run.ID)
	if err != nil || loaded.ID != run.ID {
		t.Fatalf("schema v47 Run was not preserved: %#v err=%v", loaded, err)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != 48 {
		t.Fatalf("schema v47 did not upgrade to v48: version=%d err=%v", version, err)
	}
	var table string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'sandbox_manifest_preparations'`).Scan(&table); err != nil ||
		table != "sandbox_manifest_preparations" {
		t.Fatalf("schema v48 sandbox ledger is missing: %q err=%v", table, err)
	}
}

func openSandboxManifestStore(t *testing.T, ctx context.Context,
) (*SQLiteStore, domain.Run, string) {
	t.Helper()
	st, run, root := openSandboxManifestStoreAt(t, ctx, filepath.Join(t.TempDir(), "cyberagent.db"))
	t.Cleanup(func() { _ = st.Close() })
	return st, run, root
}

func openSandboxManifestStoreAt(t *testing.T, ctx context.Context, path string,
) (*SQLiteStore, domain.Run, string) {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := st.SaveWorkspace(ctx, WorkspaceRecord{
		ID: "ws-sandbox-store", Name: "sandbox-store", RootPath: root,
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "persist a sandbox manifest", Profile: "code", WorkspaceID: "ws-sandbox-store",
		Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, run, root
}

func sandboxStoreTestManifest() sandbox.Manifest {
	return sandbox.Manifest{
		ProtocolVersion: sandbox.ManifestProtocolVersion,
		Backend:         sandbox.BackendNoop,
		Command: sandbox.CommandSpec{
			Executable: "go", Arguments: []string{"test", "./..."},
			WorkingDirectory: "/workspace",
		},
		Mounts:  []sandbox.Mount{{Source: ".", Target: "/workspace", Access: sandbox.MountReadOnly}},
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
