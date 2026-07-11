package domain

import (
	"strings"
	"testing"
	"time"
)

func TestRunExecutionLeaseValidationAndActivity(t *testing.T) {
	now := time.Now().UTC()
	lease := RunExecutionLease{
		RunID: "run-1", LeaseID: "lease-1", OwnerID: "worker-1", Generation: 1,
		Status: RunExecutionLeaseActive, AcquiredAt: now, RenewedAt: now,
		ExpiresAt: now.Add(time.Second),
	}
	if err := lease.Validate(); err != nil || !lease.ActiveAt(now) || lease.ActiveAt(lease.ExpiresAt) {
		t.Fatalf("unexpected active lease validation: %#v err=%v", lease, err)
	}
	releasedAt := now.Add(500 * time.Millisecond)
	lease.Status = RunExecutionLeaseReleased
	lease.ReleasedAt = &releasedAt
	if err := lease.Validate(); err != nil || lease.ActiveAt(now) {
		t.Fatalf("unexpected released lease validation: %#v err=%v", lease, err)
	}
}

func TestAcquireRunExecutionLeaseRequestRequiresBoundedTTLAndReplayID(t *testing.T) {
	request, err := (AcquireRunExecutionLeaseRequest{
		RunID: " run-1 ", OwnerID: " worker-1 ", LeaseID: " lease-1 ", TTL: time.Second,
	}).Normalize()
	if err != nil || request.RunID != "run-1" || request.OwnerID != "worker-1" || request.LeaseID != "lease-1" {
		t.Fatalf("request was not normalized: %#v err=%v", request, err)
	}
	for name, invalid := range map[string]AcquireRunExecutionLeaseRequest{
		"short TTL": {RunID: "run-1", OwnerID: "worker-1", TTL: MinRunExecutionLeaseTTL - time.Nanosecond},
		"long TTL":  {RunID: "run-1", OwnerID: "worker-1", TTL: MaxRunExecutionLeaseTTL + time.Nanosecond},
		"long replay id": {
			RunID: "run-1", OwnerID: "worker-1", LeaseID: strings.Repeat("x", MaxRunLeaseIdentityRunes+1),
			TTL: time.Second,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := invalid.Normalize(); err == nil {
				t.Fatalf("invalid request was accepted: %#v", invalid)
			}
		})
	}
}
