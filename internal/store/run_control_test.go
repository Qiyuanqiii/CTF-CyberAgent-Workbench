package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
)

func TestRunLifecycleOperationIsImmutable(t *testing.T) {
	ctx := context.Background()
	state, run := createRunControlTestRun(t, ctx,
		filepath.Join(t.TempDir(), "lifecycle-immutable.db"), false)
	defer state.Close()
	key := "lifecycle-immutable-operation-0001"
	result, err := application.NewRunLifecycleControlService(state).Apply(ctx,
		application.ControlRunLifecycleRequest{
			Version: domain.RunLifecycleControlProtocolVersion, RunID: run.ID,
			Action: domain.RunLifecycleStart, OperationKey: key,
			RequestedBy: "store_test_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	digest := runmutation.RunLifecycleOperationDigest(run.ID, key)
	for _, statement := range []string{
		`UPDATE run_lifecycle_operations SET requested_by = 'other' WHERE operation_key_digest = ?`,
		`DELETE FROM run_lifecycle_operations WHERE operation_key_digest = ?`,
	} {
		if _, err := state.db.ExecContext(ctx, statement, digest); err == nil {
			t.Fatalf("immutable lifecycle ledger accepted %q", statement)
		}
	}
	stored, found, err := state.GetRunLifecycleOperation(ctx, digest)
	if err != nil || !found || stored != result.Operation {
		t.Fatalf("lifecycle ledger changed: stored=%#v found=%t err=%v",
			stored, found, err)
	}
}

func TestRunLifecycleConcurrentRequestsConvergeAcrossStores(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "lifecycle-concurrent.db")
	primary, run := createRunControlTestRun(t, ctx, path, false)
	defer primary.Close()
	secondary, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondary.Close()
	request := application.ControlRunLifecycleRequest{
		Version: domain.RunLifecycleControlProtocolVersion, RunID: run.ID,
		Action: domain.RunLifecycleStart, OperationKey: "lifecycle-concurrent-0001",
		RequestedBy: "store_test_operator",
	}
	services := []*application.RunLifecycleControlService{
		application.NewRunLifecycleControlService(primary),
		application.NewRunLifecycleControlService(secondary),
	}
	results := make([]application.ControlRunLifecycleResult, len(services))
	errorsFound := make([]error, len(services))
	var group sync.WaitGroup
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errorsFound[index] = services[index].Apply(ctx, request)
		}(index)
	}
	group.Wait()
	if errorsFound[0] != nil || errorsFound[1] != nil ||
		results[0].Operation != results[1].Operation ||
		results[0].Replayed == results[1].Replayed ||
		results[0].Run.Status != domain.RunRunning ||
		results[1].Run.Status != domain.RunRunning {
		t.Fatalf("concurrent lifecycle operations diverged: results=%#v errors=%v",
			results, errorsFound)
	}
	timeline, err := primary.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, event := range timeline {
		if event.Type == events.RunStatusChangedEvent &&
			event.Source == "run_lifecycle_control" {
			changed++
		}
	}
	if changed != 2 {
		t.Fatalf("concurrent start appended %d lifecycle events, want 2", changed)
	}
}

func TestRunExecutionHandoffIsImmutableAndRejectsStaleLease(t *testing.T) {
	ctx := context.Background()
	state, run := createRunControlTestRun(t, ctx,
		filepath.Join(t.TempDir(), "handoff-immutable.db"), true)
	defer state.Close()
	queued, err := state.EnqueueOperatorSteering(ctx,
		domain.EnqueueOperatorSteeringRequest{
			RunID: run.ID, SessionID: run.SessionID, Content: "frozen input",
			OperationKey: "handoff-immutable-message-0001",
			RequestedBy:  "store_test_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	key := "handoff-immutable-operation-0001"
	operation := domain.RunExecutionHandoffOperation{
		ID:              idgen.New("run-handoff"),
		ProtocolVersion: domain.RunExecutionHandoffProtocolVersion,
		KeyDigest:       runmutation.RunExecutionHandoffOperationDigest(run.ID, key),
		RequestFingerprint: runmutation.RunExecutionHandoffRequestFingerprint(run.ID,
			"store_test_operator", 1),
		RunID: run.ID, SessionID: run.SessionID,
		RequestedBy: "store_test_operator", MaxSteps: 1, CreatedAt: time.Now().UTC(),
	}
	handoff, replayed, err := state.PrepareRunExecutionHandoff(ctx, operation)
	if err != nil || replayed || len(handoff.Items) != 1 ||
		handoff.Items[0].MessageID != queued.Message.ID {
		t.Fatalf("handoff prepare=%#v replayed=%t err=%v", handoff, replayed, err)
	}
	stale := acquireTestRunExecutionLease(t, ctx, state, run.ID)
	if _, _, err := state.ReleaseRunExecutionLease(ctx, stale); err != nil {
		t.Fatal(err)
	}
	active := acquireTestRunExecutionLease(t, ctx, state, run.ID)
	if _, _, err := state.CompleteRunExecutionHandoff(ctx, operation.ID, stale,
		domain.RunExecutionHandoffFailed, "stale_lease", "conflict", 0,
		false, false); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale lease completion code=%s err=%v", apperror.CodeOf(err), err)
	}
	result, replayed, err := state.CompleteRunExecutionHandoff(ctx, operation.ID, active,
		domain.RunExecutionHandoffFailed, "model_unavailable", "internal", 0,
		false, false)
	if err != nil || replayed || result.OperationID != operation.ID {
		t.Fatalf("active lease completion=%#v replayed=%t err=%v", result, replayed, err)
	}
	if _, _, err := state.CompleteRunExecutionHandoff(ctx, operation.ID, active,
		domain.RunExecutionHandoffFailed, "changed_reason", "internal", 0,
		false, false); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed completion replay code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, replayed, err := state.CompleteRunExecutionHandoff(ctx, operation.ID, active,
		domain.RunExecutionHandoffFailed, "model_unavailable", "internal", 0,
		false, false); err != nil || !replayed {
		t.Fatalf("exact completion replay replayed=%t err=%v", replayed, err)
	}
	mutations := []struct {
		statement string
		argument  any
	}{
		{`UPDATE run_execution_handoff_operations SET requested_by = 'other' WHERE id = ?`, operation.ID},
		{`DELETE FROM run_execution_handoff_operations WHERE id = ?`, operation.ID},
		{`UPDATE run_execution_handoff_items SET ordinal = 2 WHERE operation_id = ?`, operation.ID},
		{`DELETE FROM run_execution_handoff_items WHERE operation_id = ?`, operation.ID},
		{`UPDATE run_execution_handoff_results SET stop_reason = 'other' WHERE operation_id = ?`, operation.ID},
		{`DELETE FROM run_execution_handoff_results WHERE operation_id = ?`, operation.ID},
	}
	for _, mutation := range mutations {
		if _, err := state.db.ExecContext(ctx, mutation.statement,
			mutation.argument); err == nil {
			t.Fatalf("immutable handoff ledger accepted %q", mutation.statement)
		}
	}
	stored, found, err := state.GetRunExecutionHandoff(ctx, operation.KeyDigest)
	if err != nil || !found || stored.Result == nil ||
		stored.Result.CompletionEventSequence != result.CompletionEventSequence {
		t.Fatalf("handoff ledger changed: stored=%#v found=%t err=%v",
			stored, found, err)
	}
}

func TestSchemaV73UpgradePreservesRunWithoutFabricatingControlOperations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v72.db")
	state, run := createRunControlTestRun(t, ctx, path, false)
	for _, statement := range removeSchemaV73ForTestStatements() {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			_ = state.Close()
			t.Fatalf("remove schema v73 with %q: %v", statement, err)
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
	if version, err := upgraded.SchemaVersion(ctx); err != nil ||
		version != LatestSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	if preserved, err := upgraded.GetRun(ctx, run.ID); err != nil ||
		preserved.ID != run.ID {
		t.Fatalf("historical Run was not preserved: run=%#v err=%v", preserved, err)
	}
	digest := runmutation.RunLifecycleOperationDigest(run.ID,
		"historical-operation-that-never-existed")
	if operation, found, err := upgraded.GetRunLifecycleOperation(ctx, digest); err != nil || found {
		t.Fatalf("v73 migration fabricated operation=%#v found=%t err=%v",
			operation, found, err)
	}
}

func createRunControlTestRun(t testing.TB, ctx context.Context, path string,
	start bool,
) (*SQLiteStore, domain.Run) {
	t.Helper()
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "Run control test", Profile: "code",
			Budget: domain.Budget{MaxTurns: 8}})
	if err != nil {
		_ = state.Close()
		t.Fatal(err)
	}
	if start {
		run, err = application.NewRunService(state).Start(ctx, run.ID)
		if err != nil {
			_ = state.Close()
			t.Fatal(err)
		}
	}
	return state, run
}
