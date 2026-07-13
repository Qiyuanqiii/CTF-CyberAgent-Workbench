package application

import (
	"testing"
	"time"
)

func TestRunExecutionLeaseRenewalTimeoutUsesExpirySlack(t *testing.T) {
	tests := []struct {
		name   string
		policy RunExecutionLeasePolicy
		want   time.Duration
	}{
		{
			name:   "default cap",
			policy: DefaultRunExecutionLeasePolicy(),
			want:   2 * time.Second,
		},
		{
			name: "short interval keeps available slack",
			policy: RunExecutionLeasePolicy{
				TTL: 180 * time.Millisecond, RenewInterval: 40 * time.Millisecond,
			},
			want: 140 * time.Millisecond,
		},
		{
			name: "near-expiry interval",
			policy: RunExecutionLeasePolicy{
				TTL: time.Second, RenewInterval: 900 * time.Millisecond,
			},
			want: 100 * time.Millisecond,
		},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			if got := runExecutionLeaseRenewalTimeout(current.policy); got != current.want {
				t.Fatalf("renewal timeout = %s, want %s", got, current.want)
			}
		})
	}
}
