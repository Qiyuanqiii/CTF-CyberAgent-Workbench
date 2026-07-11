package application_test

import (
	"context"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

type executionLeaseTestStore interface {
	AcquireRunExecutionLease(context.Context, domain.AcquireRunExecutionLeaseRequest) (domain.RunExecutionLeaseAcquisition, error)
	ReleaseRunExecutionLease(context.Context, domain.RunExecutionLease) (domain.RunExecutionLease, bool, error)
	GetRunExecutionLease(context.Context, string) (domain.RunExecutionLease, bool, error)
}

func releaseTestRunExecutionLease(t testing.TB, ctx context.Context, st executionLeaseTestStore,
	runID string,
) {
	t.Helper()
	lease, found, err := st.GetRunExecutionLease(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || lease.Status != domain.RunExecutionLeaseActive {
		t.Fatalf("active test execution lease for %s was not found: %#v", runID, lease)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
}

func acquireTestRunExecutionLease(t testing.TB, ctx context.Context, st executionLeaseTestStore,
	runID string,
) domain.RunExecutionLease {
	t.Helper()
	acquired, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: runID, OwnerID: "application-test-worker", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return acquired.Lease
}
