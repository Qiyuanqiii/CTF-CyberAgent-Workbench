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
