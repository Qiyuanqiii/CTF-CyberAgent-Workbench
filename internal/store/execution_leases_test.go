package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

func TestSQLiteRunExecutionLeaseLifecycleFencesStaleSupervisor(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, run := createWorkItemTestRun(t, ctx, st, "lease takeover")
	if _, err := application.NewRunService(st).Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}

	first, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "worker-a", TTL: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "worker-a", TTL: 150 * time.Millisecond,
	}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("same owner entered without an explicit replay token: code=%s err=%v", apperror.CodeOf(err), err)
	}
	replayed, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "worker-a", LeaseID: first.Lease.LeaseID, TTL: 150 * time.Millisecond,
	})
	if err != nil || !replayed.Replayed || replayed.TookOver ||
		replayed.Lease.LeaseID != first.Lease.LeaseID || replayed.Lease.Generation != 1 {
		t.Fatalf("same-owner acquisition did not replay: %#v err=%v", replayed, err)
	}
	first.Lease = replayed.Lease
	if _, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "worker-b", TTL: time.Minute,
	}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("active lease was not exclusive: code=%s err=%v", apperror.CodeOf(err), err)
	}

	turn, err := st.BeginSupervisorTurn(ctx, first.Lease, "")
	if err != nil {
		t.Fatal(err)
	}
	waitForLeaseExpiry(first.Lease)
	second, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "worker-b", TTL: time.Minute,
	})
	if err != nil || !second.TookOver || second.Replayed || second.Lease.Generation != 2 ||
		second.Lease.LeaseID == first.Lease.LeaseID {
		t.Fatalf("expired lease was not fenced by a new generation: %#v err=%v", second, err)
	}
	if _, err := st.BindSupervisorTurnInput(ctx, turn.Checkpoint, "stale input"); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale checkpoint was not fenced: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := st.RenewRunExecutionLease(ctx, first.Lease, time.Minute); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale lease renewed: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, first.Lease); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale lease released its successor: code=%s err=%v", apperror.CodeOf(err), err)
	}

	recovered, err := st.BeginSupervisorTurn(ctx, second.Lease, "recovered input")
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Recovered || recovered.Checkpoint.AttemptID != turn.Checkpoint.AttemptID ||
		recovered.Checkpoint.LeaseID != second.Lease.LeaseID ||
		recovered.Checkpoint.LeaseGeneration != second.Lease.Generation ||
		recovered.Checkpoint.PendingInput != "recovered input" {
		t.Fatalf("takeover did not recover and rebind the pending turn: %#v", recovered.Checkpoint)
	}
	released, wasReplay, err := st.ReleaseRunExecutionLease(ctx, second.Lease)
	if err != nil || wasReplay || released.Status != domain.RunExecutionLeaseReleased {
		t.Fatalf("lease release failed: %#v replay=%v err=%v", released, wasReplay, err)
	}
	_, wasReplay, err = st.ReleaseRunExecutionLease(ctx, second.Lease)
	if err != nil || !wasReplay {
		t.Fatalf("lease release was not idempotent: replay=%v err=%v", wasReplay, err)
	}

	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countRunEventType(timeline, events.RunExecutionLeaseAcquiredEvent) != 1 ||
		countRunEventType(timeline, events.RunExecutionLeaseTakenOverEvent) != 1 ||
		countRunEventType(timeline, events.RunExecutionLeaseReleasedEvent) != 1 {
		t.Fatalf("unexpected lease audit events: %#v", timeline)
	}
	for _, event := range timeline {
		if strings.Contains(event.PayloadJSON, first.Lease.LeaseID) ||
			strings.Contains(event.PayloadJSON, second.Lease.LeaseID) ||
			event.SubjectID == first.Lease.LeaseID || event.SubjectID == second.Lease.LeaseID {
			t.Fatalf("lease event exposed a fencing token: %#v", event)
		}
	}
}

func TestSQLiteRunExecutionLeaseConcurrentAcquisitionHasOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	const contenders = 8
	stores := make([]*SQLiteStore, contenders)
	for index := range stores {
		opened, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		stores[index] = opened
	}
	t.Cleanup(func() {
		for _, st := range stores {
			_ = st.Close()
		}
	})
	ctx := context.Background()
	_, run := createWorkItemTestRun(t, ctx, stores[0], "concurrent lease")

	results := make(chan error, contenders)
	var ready sync.WaitGroup
	ready.Add(contenders)
	start := make(chan struct{})
	for index, st := range stores {
		index, st := index, st
		go func() {
			ready.Done()
			<-start
			_, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
				RunID: run.ID, OwnerID: fmt.Sprintf("worker-%d", index), TTL: time.Minute,
			})
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	successes, conflicts := 0, 0
	for range stores {
		switch code := apperror.CodeOf(<-results); code {
		case "":
			successes++
		case apperror.CodeConflict:
			conflicts++
		default:
			t.Fatalf("unexpected acquisition result code %s", code)
		}
	}
	if successes != 1 || conflicts != contenders-1 {
		t.Fatalf("expected one lease winner, got successes=%d conflicts=%d", successes, conflicts)
	}
	timeline, err := stores[0].ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := countRunEventType(timeline, events.RunExecutionLeaseAcquiredEvent); got != 1 {
		t.Fatalf("expected one acquisition event, got %d", got)
	}
}

func TestSQLiteRunExecutionLeaseRejectsTerminalRunAndSensitiveOwner(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, run := createWorkItemTestRun(t, ctx, st, "lease validation")
	secretOwner := "s" + "k-" + strings.Repeat("a", 32)
	if _, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: secretOwner, TTL: time.Minute,
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("sensitive owner was accepted: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := application.NewRunService(st).Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "worker-terminal", TTL: time.Minute,
	}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("terminal Run acquired a lease: code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestSQLiteSchemaV17RebindsLegacyPendingSupervisorCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v16.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run := createWorkItemTestRun(t, ctx, st, "legacy checkpoint")
	if _, err := application.NewRunService(st).Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	started, err := st.BeginSupervisorTurn(ctx, lease, "legacy input")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	for _, statement := range append(removeSchemaV22ForTestStatements(), []string{
		`DROP TABLE agent_admission_operations`,
		`DELETE FROM schema_migrations WHERE version = 21`,
		`DROP TABLE agent_message_operations`,
		`DELETE FROM schema_migrations WHERE version = 20`,
		`DROP TABLE agent_graph_snapshots`,
		`DROP TABLE agent_messages`,
		`DROP TABLE agent_nodes`,
		`DELETE FROM schema_migrations WHERE version = 19`,
		`DROP TABLE run_model_cancellation_operations`,
		`DROP TABLE run_model_cancellations`,
		`DELETE FROM schema_migrations WHERE version = 18`,
		`DROP TABLE run_execution_leases`,
		`ALTER TABLE run_supervisor_checkpoints DROP COLUMN lease_generation`,
		`ALTER TABLE run_supervisor_checkpoints DROP COLUMN lease_id`,
		`DELETE FROM schema_migrations WHERE version = 17`,
	}...) {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("downgrade v17 fixture with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	legacy, found, err := st.GetSupervisorCheckpoint(ctx, run.ID)
	if err != nil || !found || legacy.LeaseID != "" || legacy.LeaseGeneration != 0 ||
		legacy.AttemptID != started.Checkpoint.AttemptID {
		t.Fatalf("legacy checkpoint was not preserved: %#v found=%v err=%v", legacy, found, err)
	}
	newLease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	recovered, err := st.BeginSupervisorTurn(ctx, newLease, "legacy input")
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Recovered || recovered.Checkpoint.LeaseID != newLease.LeaseID ||
		recovered.Checkpoint.LeaseGeneration != newLease.Generation ||
		recovered.Checkpoint.AttemptID != started.Checkpoint.AttemptID {
		t.Fatalf("legacy checkpoint was not rebound after migration: %#v", recovered.Checkpoint)
	}
}

func waitForLeaseExpiry(lease domain.RunExecutionLease) {
	delay := time.Until(lease.ExpiresAt) + 50*time.Millisecond
	if delay > 0 {
		time.Sleep(delay)
	}
}
