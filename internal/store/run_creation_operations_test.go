package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/runmutation"
)

func TestRunCreationOperationLedgerIsImmutable(t *testing.T) {
	ctx := context.Background()
	state, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	workspace := WorkspaceRecord{ID: "workspace-ledger", Name: "ledger",
		RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	key := "run-create-ledger-operation"
	result, err := application.NewControlledRunCreationService(state).Create(ctx,
		application.ControlledRunCreationRequest{Version: domain.RunCreationProtocolVersion,
			Goal: "Ledger immutability", WorkspaceID: workspace.ID,
			OperationKey: key, RequestedBy: "http_control"})
	if err != nil {
		t.Fatal(err)
	}
	digest := runmutation.RunCreationOperationDigest(key)
	for _, statement := range []string{
		`UPDATE run_creation_operations SET requested_by = 'other' WHERE operation_key_digest = ?`,
		`DELETE FROM run_creation_operations WHERE operation_key_digest = ?`,
	} {
		if _, err := state.db.ExecContext(ctx, statement, digest); err == nil {
			t.Fatalf("immutable ledger accepted %q", statement)
		}
	}
	operation, found, err := state.GetRunCreationOperation(ctx, digest)
	if err != nil || !found || operation.RunID != result.Run.ID {
		t.Fatalf("ledger changed after rejected mutation: %#v found=%t err=%v",
			operation, found, err)
	}
}

func TestRunCreationOperationTriggerRejectsNonDefaultBudget(t *testing.T) {
	ctx := context.Background()
	state, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	workspace := WorkspaceRecord{ID: "workspace-ledger-budget", Name: "ledger-budget",
		RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	mission, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "Invalid ledger budget", WorkspaceID: workspace.ID,
			Profile: "code", ModelRoute: "code", Interactive: true,
			Budget: domain.Budget{MaxTurns: 1, MaxToolCalls: 100}, RequestedBy: "http_control"})
	if err != nil {
		t.Fatal(err)
	}
	operationKey := "run-create-invalid-budget"
	fingerprint := runmutation.RunCreationRequestFingerprint(mission.Goal, workspace.ID,
		string(mission.Profile), string(domain.ExecutionSurfaceCode),
		string(domain.ExecutionPhaseDeliver), "http_control")
	if _, err := state.db.ExecContext(ctx, `INSERT INTO run_creation_operations
		(operation_key_digest, request_fingerprint, protocol_version, mission_id,
		run_id, session_id, workspace_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runmutation.RunCreationOperationDigest(operationKey), fingerprint,
		domain.RunCreationProtocolVersion, mission.ID, run.ID, run.SessionID,
		workspace.ID, "http_control", ts(run.CreatedAt)); err == nil {
		t.Fatal("Run creation ledger accepted a non-default budget")
	}
}

func TestRunCreationOperationTriggerRejectsNonInitialGraph(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *SQLiteStore, domain.Mission, domain.Run) error
	}{
		{name: "changed update timestamp", mutate: func(ctx context.Context, state *SQLiteStore,
			_ domain.Mission, run domain.Run,
		) error {
			_, err := state.db.ExecContext(ctx, `UPDATE runs SET updated_at = ? WHERE id = ?`,
				ts(run.UpdatedAt.Add(time.Second)), run.ID)
			return err
		}},
		{name: "extra initial event", mutate: func(ctx context.Context, state *SQLiteStore,
			mission domain.Mission, run domain.Run,
		) error {
			extra, err := events.New(run.ID, mission.ID, "test.extra", "run_creation_test",
				run.ID, map[string]any{"extra": true})
			if err != nil {
				return err
			}
			tx, err := state.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := insertRunEventTx(ctx, tx, extra); err != nil {
				return err
			}
			return tx.Commit()
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			state, err := Open(filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer state.Close()
			workspace := WorkspaceRecord{ID: "workspace-initial-graph", Name: "initial-graph",
				RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
			if err := state.SaveWorkspace(ctx, workspace); err != nil {
				t.Fatal(err)
			}
			mission, run, err := application.NewRunService(state).Create(ctx,
				application.CreateRunRequest{Goal: "Exact initial graph", WorkspaceID: workspace.ID,
					Profile: "code", ModelRoute: "code", Interactive: true,
					Budget: domain.DefaultBudget(), RequestedBy: "http_control"})
			if err != nil {
				t.Fatal(err)
			}
			if err := test.mutate(ctx, state, mission, run); err != nil {
				t.Fatal(err)
			}
			operationKey := "run-create-non-initial-graph"
			fingerprint := runmutation.RunCreationRequestFingerprint(mission.Goal, workspace.ID,
				string(mission.Profile), string(domain.ExecutionSurfaceCode),
				string(domain.ExecutionPhaseDeliver), "http_control")
			if _, err := state.db.ExecContext(ctx, `INSERT INTO run_creation_operations
				(operation_key_digest, request_fingerprint, protocol_version, mission_id,
				run_id, session_id, workspace_id, requested_by, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				runmutation.RunCreationOperationDigest(operationKey), fingerprint,
				domain.RunCreationProtocolVersion, mission.ID, run.ID, run.SessionID,
				workspace.ID, "http_control", ts(run.CreatedAt)); err == nil {
				t.Fatal("Run creation ledger accepted a non-initial graph")
			}
		})
	}
}

func TestSchemaV72UpgradePreservesRunWithoutFabricatingCreationOperation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v71.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	workspace := WorkspaceRecord{ID: "workspace-v72-upgrade", Name: "v72-upgrade",
		RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		_ = state.Close()
		t.Fatal(err)
	}
	key := "run-create-v72-upgrade-operation"
	created, err := application.NewControlledRunCreationService(state).Create(ctx,
		application.ControlledRunCreationRequest{Version: domain.RunCreationProtocolVersion,
			Goal: "Preserve historical Run", WorkspaceID: workspace.ID,
			OperationKey: key, RequestedBy: "http_control"})
	if err != nil {
		_ = state.Close()
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV72ForTestStatements() {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			_ = state.Close()
			t.Fatalf("remove schema v72 with %q: %v", statement, err)
		}
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema version = %d, err=%v", version, err)
	}
	if run, err := upgraded.GetRun(ctx, created.Run.ID); err != nil || run.ID != created.Run.ID {
		t.Fatalf("preserved Run = %#v, err=%v", run, err)
	}
	if operation, found, err := upgraded.GetRunCreationOperation(ctx,
		runmutation.RunCreationOperationDigest(key)); err != nil || found {
		t.Fatalf("v72 migration fabricated operation %#v, found=%t err=%v",
			operation, found, err)
	}
}
