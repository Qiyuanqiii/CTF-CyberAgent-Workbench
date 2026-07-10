package store

import (
	"math"
	"testing"

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
