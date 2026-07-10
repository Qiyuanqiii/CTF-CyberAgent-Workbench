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
