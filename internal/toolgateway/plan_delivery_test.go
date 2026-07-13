package toolgateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
)

type planDeliveryExecutorStub struct {
	calls     int
	lastScope PlanDeliveryContext
	lastSpec  domain.PlanDeliverySpec
}

func (s *planDeliveryExecutorStub) ProposePlan(_ context.Context,
	scope PlanDeliveryContext, spec domain.PlanDeliverySpec,
) (PlanDeliveryResult, error) {
	s.calls++
	s.lastScope = scope
	s.lastSpec = spec
	return PlanDeliveryResult{
		ProposalID: "plan-proposal-1", Status: domain.PlanDeliveryProposalProposed,
		DirectionCount: len(spec.Directions), Version: 1,
	}, nil
}

const gatewayPlanDeliveryPayload = `{"version":"plan_delivery.v1","directions":[` +
	`{"title":"A","summary":"A","tradeoffs":["A"],"modules":[{"title":"A","objective":"A","acceptance_criteria":["A"],"dependencies":[]}]},` +
	`{"title":"B","summary":"B","tradeoffs":["B"],"modules":[{"title":"B","objective":"B","acceptance_criteria":["B"],"dependencies":[]}]},` +
	`{"title":"C","summary":"C","tradeoffs":["C"],"modules":[{"title":"C","objective":"C","acceptance_criteria":["C"],"dependencies":[]}]}]}`

func TestPlanDeliveryDefinitionIsPlanOnlyAndPayloadIsStrict(t *testing.T) {
	if len(SupervisorToolDefinitions()) != 3 || len(PlanPhaseSupervisorToolDefinitions()) != 4 {
		t.Fatal("Plan/Delivery tool leaked into the ordinary Supervisor tool set")
	}
	definition, found := SupervisorToolDefinition(PlanDeliveryProposeTool)
	if !found || definition.Class != ClassAgentProposal ||
		definition.Approval != ApprovalAutomatic || !json.Valid(definition.InputSchema) ||
		!strings.Contains(string(definition.InputSchema), `"const":"plan_delivery.v1"`) {
		t.Fatalf("invalid Plan/Delivery definition: %#v", definition)
	}
	canonical, err := NormalizeSupervisorToolPayload(PlanDeliveryProposeTool,
		json.RawMessage(gatewayPlanDeliveryPayload))
	if err != nil || !json.Valid(canonical) {
		t.Fatalf("valid Plan/Delivery payload failed: %s err=%v", canonical, err)
	}
	for _, payload := range []json.RawMessage{
		json.RawMessage(`{"version":"plan_delivery.v1","directions":[]}`),
		json.RawMessage(`{"version":"plan_delivery.v1","directions":[],"authority":true}`),
		json.RawMessage(gatewayPlanDeliveryPayload + ` {}`),
	} {
		if _, err := NormalizeSupervisorToolPayload(PlanDeliveryProposeTool, payload); err == nil {
			t.Fatalf("invalid Plan/Delivery payload was accepted: %s", payload)
		}
	}
}

func TestPlanDeliveryGatewayRequiresFencedRootAndCreatesOnlyProposal(t *testing.T) {
	tracked := newTrackedStructuredStore()
	executor := &planDeliveryExecutorStub{}
	gateway := New(tracked, policy.NewDefaultChecker()).
		WithPlanDeliveryExecutor(executor)
	call := ToolCall{
		Name:         PlanDeliveryProposeTool,
		Payload:      json.RawMessage(gatewayPlanDeliveryPayload),
		OperationKey: "plan-operation", RunID: "run-1", AgentID: "agent-root",
		SessionID: "session-1", WorkspaceID: "workspace-1",
		RequestedBy: "run_supervisor", LeaseID: "lease-1", LeaseGeneration: 1,
	}
	outcome, err := gateway.Invoke(t.Context(), call)
	if err != nil || outcome.Result == nil || outcome.Result.Status != StatusCompleted ||
		outcome.Execution == nil || outcome.Execution.Backend != "plan_proposal" ||
		outcome.Result.Metadata["proposal_id"] != "plan-proposal-1" ||
		outcome.Result.Metadata["selection_authorized"] != "false" ||
		outcome.Result.Metadata["phase_change_authorized"] != "false" ||
		outcome.Result.Metadata["execution_authorized"] != "false" || executor.calls != 1 {
		t.Fatalf("unexpected Plan/Delivery outcome: %#v err=%v", outcome, err)
	}
	unfenced := call
	unfenced.RequestedBy = "cli"
	unfenced.LeaseID = ""
	unfenced.LeaseGeneration = 0
	if _, err := gateway.Invoke(t.Context(), unfenced); err == nil {
		t.Fatal("unfenced Plan/Delivery proposal was accepted")
	}
	if tracked.chargeCount() != 1 {
		t.Fatalf("unfenced proposal reached budget ledger: %d", tracked.chargeCount())
	}
}

func TestPlanDeliveryGatewayPolicyDenialNeverInvokesExecutor(t *testing.T) {
	tracked := newTrackedStructuredStore()
	executor := &planDeliveryExecutorStub{}
	gateway := New(tracked, policy.NewDefaultChecker()).
		WithPlanDeliveryExecutor(executor)
	payload := strings.Replace(gatewayPlanDeliveryPayload, `"objective":"A"`,
		`"objective":"masscan 0.0.0.0/0"`, 1)
	outcome, err := gateway.Invoke(t.Context(), ToolCall{
		Name: PlanDeliveryProposeTool, Payload: json.RawMessage(payload),
		OperationKey: "denied-plan", RunID: "run-1", AgentID: "agent-root",
		SessionID: "session-1", WorkspaceID: "workspace-1",
		RequestedBy: "run_supervisor", LeaseID: "lease-1", LeaseGeneration: 1,
	})
	if err != nil || outcome.Result == nil || outcome.Result.Status != StatusDenied ||
		outcome.Decision.Allowed || executor.calls != 0 || tracked.chargeCount() != 1 {
		t.Fatalf("Plan/Delivery Policy denial drifted: %#v err=%v calls=%d charges=%d",
			outcome, err, executor.calls, tracked.chargeCount())
	}
}
