package idgen

import (
	"strings"
	"testing"
)

func TestNewUsesPrefixAndProducesDistinctIDs(t *testing.T) {
	first := New("run")
	second := New("run")
	if !strings.HasPrefix(first, "run-") || !strings.HasPrefix(second, "run-") {
		t.Fatalf("unexpected ids: %q %q", first, second)
	}
	if first == second {
		t.Fatalf("expected distinct ids, got %q", first)
	}
}
