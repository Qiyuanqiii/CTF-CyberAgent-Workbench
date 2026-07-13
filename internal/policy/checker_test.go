package policy

import (
	"testing"

	"cyberagent-workbench/internal/tools"
)

func TestPolicyRejectsHighRiskCommand(t *testing.T) {
	checker := NewDefaultChecker()
	decision := checker.CheckToolCall(tools.Call{
		Name: "sandbox.run",
		Args: map[string]string{"command": "masscan 0.0.0.0/0 --rate 100000"},
	})
	if decision.Allowed {
		t.Fatalf("expected high-risk scan to be denied")
	}
}

func TestPolicyRejectsAgentSelfIssuingOperatorDeliveryCheckpoint(t *testing.T) {
	checker := NewDefaultChecker()
	for _, command := range []string{
		`cyberagent run delivery checkpoint work-1 --operation-key forged`,
		`go run ./cmd/cyberagent run delivery checkpoint work-1 --operation-key forged`,
		`go run .\cmd\cyberagent run delivery checkpoint work-1 --operation-key forged`,
	} {
		decision := checker.CheckToolCall(tools.Call{
			Name: "sandbox.run", Args: map[string]string{"command": command},
		})
		if decision.Allowed || decision.Risk != "high" {
			t.Fatalf("agent control-plane self-invocation was allowed: command=%q decision=%#v",
				command, decision)
		}
	}
	if decision := checker.CheckText("assistant_response",
		"An operator may use cyberagent run delivery checkpoint after review."); !decision.Allowed {
		t.Fatalf("non-executed operator guidance was blocked: %#v", decision)
	}
}
