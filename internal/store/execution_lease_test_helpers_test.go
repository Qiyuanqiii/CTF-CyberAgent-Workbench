package store

import (
	"context"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func acquireTestRunExecutionLease(t testing.TB, ctx context.Context, st *SQLiteStore,
	runID string,
) domain.RunExecutionLease {
	t.Helper()
	acquired, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: runID, OwnerID: "store-test-worker", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return acquired.Lease
}

func expireTestRunExecutionLease(t testing.TB, ctx context.Context, st *SQLiteStore,
	lease domain.RunExecutionLease,
) {
	t.Helper()
	expiresAt := lease.RenewedAt.Add(time.Nanosecond)
	if delay := time.Until(expiresAt); delay > 0 {
		time.Sleep(delay)
	}
	result, err := st.db.ExecContext(ctx, `UPDATE run_execution_leases SET expires_at = ?
		WHERE run_id = ? AND lease_id = ? AND generation = ? AND status = ?`,
		ts(expiresAt), lease.RunID, lease.LeaseID, lease.Generation, domain.RunExecutionLeaseActive)
	if err != nil {
		t.Fatalf("expire test run execution lease: %v", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("read expired test lease rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expire test run execution lease rows=%d, want 1", rows)
	}
}
