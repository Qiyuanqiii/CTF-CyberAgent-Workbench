package application

import (
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

func TestParseRootActionStrictJSON(t *testing.T) {
	action, err := parseRootAction(`{"version":"root_lifecycle.v1","action":"finish","message":"done","summary":"review complete"}`)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != domain.RootActionFinish || action.Summary != "review complete" {
		t.Fatalf("unexpected action: %#v", action)
	}

	invalid := []string{
		`{"version":"root_lifecycle.v1","action":"continue","message":"ok","extra":true}`,
		`{"version":"root_lifecycle.v1","action":"finish","message":"done"}`,
		"```json\n{\"version\":\"root_lifecycle.v1\",\"action\":\"continue\",\"message\":\"ok\"}\n```",
		`{"version":"root_lifecycle.v1","action":"continue","message":"ok"} {}`,
	}
	for _, raw := range invalid {
		if _, err := parseRootAction(raw); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
			t.Fatalf("invalid action code = %s, want %s: %v", apperror.CodeOf(err), apperror.CodeFailedPrecondition, err)
		}
	}

	oversized := `{"version":"root_lifecycle.v1","action":"continue","message":"` + strings.Repeat("x", maxRootActionJSONBytes) + `"}`
	if _, err := parseRootAction(oversized); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("oversized action code = %s, want %s", apperror.CodeOf(err), apperror.CodeResourceExhausted)
	}
	spacePrefixed := strings.Repeat(" ", maxRootActionJSONBytes) + `{"version":"root_lifecycle.v1","action":"continue","message":"ok"}`
	if _, err := parseRootAction(spacePrefixed); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("space-prefixed action code = %s, want %s", apperror.CodeOf(err), apperror.CodeResourceExhausted)
	}
	largeField := `{"version":"root_lifecycle.v1","action":"continue","message":"` + strings.Repeat("x", 17*1024) + `"}`
	if _, err := parseRootAction(largeField); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("large field code = %s, want %s", apperror.CodeOf(err), apperror.CodeFailedPrecondition)
	}
}
