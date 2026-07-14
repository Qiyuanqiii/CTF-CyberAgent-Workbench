package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

func TestSandboxExecutionCandidateConcurrentReplayAndImmutability(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "candidate.db")
	st1, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st1.Close() })
	root := t.TempDir()
	if err := st1.SaveWorkspace(ctx, WorkspaceRecord{
		ID: "ws-sandbox-candidate", Name: "sandbox-candidate", RootPath: root,
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st1).Create(ctx, application.CreateRunRequest{
		Goal: "validate a disabled execution candidate", Profile: "code",
		WorkspaceID: "ws-sandbox-candidate",
		Budget:      domain.Budget{MaxTurns: 4, MaxToolCalls: 4, MaxTokens: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := application.NewSandboxManifestService(st1, policy.NewDefaultChecker()).Prepare(ctx,
		application.PrepareSandboxManifestRequest{
			RunID: run.ID, Manifest: sandboxStoreTestManifest(),
			OperationKey: "candidate-concurrent-prepare", RequestedBy: "candidate_store_test",
		})
	if err != nil {
		t.Fatal(err)
	}
	st2, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	services := []*application.SandboxManifestService{
		application.NewSandboxManifestService(st1, policy.NewDefaultChecker()),
		application.NewSandboxManifestService(st2, policy.NewDefaultChecker()),
	}
	request := application.ValidateSandboxExecutionCandidateRequest{
		PreparationID: prepared.Preparation.ID, Manifest: sandboxStoreTestManifest(),
		OperationKey: "candidate-concurrent-validate", RequestedBy: "candidate_store_test",
	}
	start := make(chan struct{})
	results := make([]sandbox.ValidatedExecutionCandidate, len(services))
	errorsFound := make([]error, len(services))
	var group sync.WaitGroup
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index], errorsFound[index] = services[index].ValidateExecutionCandidate(ctx, request)
		}(index)
	}
	close(start)
	group.Wait()
	if errorsFound[0] != nil || errorsFound[1] != nil ||
		results[0].Candidate.ID != results[1].Candidate.ID {
		t.Fatalf("concurrent candidate validation diverged: results=%#v errors=%v", results, errorsFound)
	}
	if results[0].Replayed == results[1].Replayed {
		t.Fatalf("expected one candidate create and one replay: %#v", results)
	}
	candidateID := results[0].Candidate.ID
	if _, err := st1.db.ExecContext(ctx, `UPDATE sandbox_execution_candidates
		SET execution_authorized = 1 WHERE id = ?`, candidateID); err == nil {
		t.Fatal("sandbox execution candidate was mutable or could authorize execution")
	}
	values, err := st1.ListSandboxExecutionCandidates(ctx, run.ID, 100)
	if err != nil || len(values) != 1 || values[0].Candidate.ExecutionAuthorized ||
		values[0].Candidate.BackendEnabled {
		t.Fatalf("stored candidate projection is invalid: %#v err=%v", values, err)
	}
	rows, err := st1.db.QueryContext(ctx, `PRAGMA table_info(sandbox_execution_candidates)`)
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
		case "manifest_json", "command", "arguments_json", "mount_sources_json",
			"workspace_root", "environment_json", "secret_references_json":
			t.Fatalf("schema v49 candidate persists raw intent in column %q", name)
		}
	}
	eventsFound, err := st1.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range eventsFound {
		if strings.Contains(event.PayloadJSON, root) || strings.Contains(event.PayloadJSON, `"executable"`) {
			t.Fatalf("candidate event leaked raw workspace or command data: %#v", event)
		}
	}
}

func TestSchemaV48UpgradeAddsSandboxExecutionCandidates(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v48.db")
	st, run, _ := openSandboxManifestStoreAt(t, ctx, path)
	prepared, err := application.NewSandboxManifestService(st, policy.NewDefaultChecker()).Prepare(ctx,
		application.PrepareSandboxManifestRequest{
			RunID: run.ID, Manifest: sandboxStoreTestManifest(),
			OperationKey: "candidate-v48-prepare", RequestedBy: "upgrade_test",
		})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV49ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("simulate schema v48 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if version, err := st.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v48 did not upgrade to v49: version=%d err=%v", version, err)
	}
	loaded, err := st.GetSandboxManifestIntent(ctx, prepared.Preparation.ID)
	if err != nil || loaded.Preparation.ID != prepared.Preparation.ID {
		t.Fatalf("schema v48 preparation was not preserved: %#v err=%v", loaded, err)
	}
	var table string
	if err := st.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'sandbox_execution_candidates'`).Scan(&table); err != nil ||
		table != "sandbox_execution_candidates" {
		t.Fatalf("schema v49 candidate ledger is missing: %q err=%v", table, err)
	}
}

func TestSandboxApprovalRequestConcurrentReplayAcrossStores(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "approval-race.db")
	st1, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st1.Close() })
	root := t.TempDir()
	if err := st1.SaveWorkspace(ctx, WorkspaceRecord{
		ID: "ws-sandbox-approval-race", Name: "sandbox-approval-race", RootPath: root,
	}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st1).Create(ctx, application.CreateRunRequest{
		Goal: "converge sandbox approval request", Profile: "code",
		WorkspaceID: "ws-sandbox-approval-race",
		Budget:      domain.Budget{MaxTurns: 4, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := sandboxStoreTestManifest()
	manifest.Mounts[0].Access = sandbox.MountReadWrite
	prepared, err := application.NewSandboxManifestService(st1, policy.NewDefaultChecker()).Prepare(ctx,
		application.PrepareSandboxManifestRequest{
			RunID: run.ID, Manifest: manifest, OperationKey: "approval-race-prepare",
			RequestedBy: "approval_race_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	st2, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	services := []*application.SandboxManifestService{
		application.NewSandboxManifestService(st1, policy.NewDefaultChecker()),
		application.NewSandboxManifestService(st2, policy.NewDefaultChecker()),
	}
	start := make(chan struct{})
	ids := make([]string, len(services))
	errorsFound := make([]error, len(services))
	var group sync.WaitGroup
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			record, requestErr := services[index].RequestApproval(ctx,
				prepared.Preparation.ID, "approval_race_operator")
			ids[index], errorsFound[index] = record.ID, requestErr
		}(index)
	}
	close(start)
	group.Wait()
	if errorsFound[0] != nil || errorsFound[1] != nil || ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("concurrent sandbox approval requests diverged: ids=%v errors=%v", ids, errorsFound)
	}
}
