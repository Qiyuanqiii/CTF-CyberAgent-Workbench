package store

import (
	"math"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
)

func TestSupervisorAddCounter(t *testing.T) {
	value, err := supervisorAddCounter(4, 5, "test")
	if err != nil {
		t.Fatalf("add counter: %v", err)
	}
	if value != 9 {
		t.Fatalf("counter = %d, want 9", value)
	}

	if _, err := supervisorAddCounter(math.MaxInt64, 1, "test"); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("overflow code = %s, want %s", apperror.CodeOf(err), apperror.CodeResourceExhausted)
	}
	if _, err := supervisorAddCounter(0, -1, "test"); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("negative code = %s, want %s", apperror.CodeOf(err), apperror.CodeFailedPrecondition)
	}
}

func TestSupervisorModelElapsedMillisChargesStartedAttempt(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    int64
	}{
		{name: "same clock tick", elapsed: 0, want: 1},
		{name: "sub millisecond", elapsed: time.Nanosecond, want: 1},
		{name: "whole milliseconds", elapsed: 2 * time.Millisecond, want: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := supervisorModelElapsedMillis(test.elapsed)
			if err != nil || got != test.want {
				t.Fatalf("elapsed=%s got=%d want=%d err=%v", test.elapsed, got, test.want, err)
			}
		})
	}
	if _, err := supervisorModelElapsedMillis(-time.Nanosecond); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("negative model duration code=%s err=%v", apperror.CodeOf(err), err)
	}
}
