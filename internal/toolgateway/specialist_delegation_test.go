package toolgateway

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
)

type specialistDelegationExecutorStub struct {
	mu        sync.Mutex
	calls     int
	lastScope SpecialistDelegationContext
	lastSpec  domain.SpecialistDelegationSpec
}

func (s *specialistDelegationExecutorStub) ProposeSpecialists(_ context.Context,
	scope SpecialistDelegationContext, spec domain.SpecialistDelegationSpec,
) (SpecialistDelegationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastScope = scope
	s.lastSpec = spec
	return SpecialistDelegationResult{
		ProposalID: "delegation-1", Status: domain.SpecialistDelegationProposed,
		AssignmentCount: len(spec.Assignments), Version: 1,
	}, nil
}

func TestSpecialistDelegationDefinitionAndPayloadAreStrict(t *testing.T) {
	definitions := SupervisorToolDefinitions()
	if len(definitions) != 3 {
		t.Fatalf("unexpected Supervisor tool definitions: %#v", definitions)
	}
	definition, found := SupervisorToolDefinition(SpecialistDelegationProposeTool)
	if !found || definition.Class != ClassAgentProposal || definition.Approval != ApprovalAutomatic ||
		!json.Valid(definition.InputSchema) || !strings.Contains(string(definition.InputSchema),
		`"const":"specialist_delegation.v1"`) {
		t.Fatalf("invalid Specialist delegation tool definition: %#v", definition)
	}
	valid := json.RawMessage(`{"version":"specialist_delegation.v1","assignments":[{"title":"A","goal":"B","skills":["work_item_create","model.chat"],"turn_limit":2,"token_limit":128}]}`)
	canonical, err := NormalizeSupervisorToolPayload(SpecialistDelegationProposeTool, valid)
	if err != nil || !json.Valid(canonical) || !strings.Contains(string(canonical),
		`"skills":["model.chat","work_item_create"]`) {
		t.Fatalf("delegation payload was not canonicalized: %s err=%v", canonical, err)
	}
	invalid := []json.RawMessage{
		json.RawMessage(`{"version":"specialist_delegation.v1","assignments":[],"spawn":true}`),
		json.RawMessage(`{"version":"specialist_delegation.v1","assignments":[]} {}`),
		json.RawMessage(`{"version":"specialist_delegation.v2","assignments":[]}`),
	}
	for _, payload := range invalid {
		if _, err := NormalizeSupervisorToolPayload(SpecialistDelegationProposeTool, payload); err == nil {
			t.Fatalf("invalid delegation payload was accepted: %s", payload)
		}
	}
}

func TestSpecialistDelegationRequiresFencedRootAndOnlyCreatesProposal(t *testing.T) {
	store := newTrackedStructuredStore()
	executor := &specialistDelegationExecutorStub{}
	gateway := New(store, policy.NewDefaultChecker()).WithSpecialistDelegationExecutor(executor)
	token := "s" + "k-" + strings.Repeat("q", 28)
	payload := json.RawMessage(`{"version":"specialist_delegation.v1","assignments":[{"title":"Network review","goal":"Use nmap only after scoped operator review; token=` + token + `","skills":["model.chat"],"turn_limit":2,"token_limit":128}]}`)
	call := ToolCall{
		Name: SpecialistDelegationProposeTool, Payload: payload,
		OperationKey: "delegation-operation", RunID: "run-1", AgentID: "agent-root",
		SessionID: "sess-1", WorkspaceID: "ws-1", RequestedBy: "run_supervisor",
		LeaseID: "lease-1", LeaseGeneration: 1,
	}
	outcome, err := gateway.Invoke(t.Context(), call)
	if err != nil || outcome.Result == nil || outcome.Result.Status != StatusCompleted ||
		outcome.Result.Metadata["proposal_id"] != "delegation-1" ||
		outcome.Result.Metadata["admission_authorized"] != "false" ||
		outcome.Execution == nil || outcome.Execution.Backend != "agent_proposal" {
		t.Fatalf("unexpected delegation proposal outcome: %#v err=%v", outcome, err)
	}
	if outcome.Call.OperationKey != "" || outcome.Call.LeaseID != "" ||
		strings.Contains(string(outcome.Call.Payload), token) ||
		!strings.Contains(string(outcome.Call.Payload), "[REDACTED:") {
		t.Fatalf("delegation outcome exposed control or secret data: %#v", outcome.Call)
	}
	executor.mu.Lock()
	if executor.calls != 1 || executor.lastScope.InvocationID == "" ||
		executor.lastScope.PolicyDecision.Approval != ApprovalAutomatic ||
		strings.Contains(executor.lastSpec.Assignments[0].Goal, token) {
		t.Fatalf("delegation executor received an unsafe scope: %#v %#v",
			executor.lastScope, executor.lastSpec)
	}
	executor.mu.Unlock()

	unfenced := call
	unfenced.RequestedBy = "cli"
	unfenced.LeaseID = ""
	unfenced.LeaseGeneration = 0
	if _, err := gateway.Invoke(t.Context(), unfenced); err == nil {
		t.Fatal("unfenced delegation proposal was accepted")
	}
	if store.chargeCount() != 1 {
		t.Fatalf("unfenced proposal reached the budget ledger: %d", store.chargeCount())
	}
}

func TestSpecialistDelegationPolicyDenialNeverInvokesExecutor(t *testing.T) {
	store := newTrackedStructuredStore()
	executor := &specialistDelegationExecutorStub{}
	gateway := New(store, policy.NewDefaultChecker()).WithSpecialistDelegationExecutor(executor)
	payload := json.RawMessage(`{"version":"specialist_delegation.v1","assignments":[{"title":"Unsafe","goal":"masscan 0.0.0.0/0","skills":["model.chat"],"turn_limit":1,"token_limit":32}]}`)
	outcome, err := gateway.Invoke(t.Context(), ToolCall{
		Name: SpecialistDelegationProposeTool, Payload: payload,
		OperationKey: "denied-delegation", RunID: "run-1", AgentID: "agent-root",
		SessionID: "sess-1", WorkspaceID: "ws-1", RequestedBy: "run_supervisor",
		LeaseID: "lease-1", LeaseGeneration: 1,
	})
	if err != nil || outcome.Result == nil || outcome.Result.Status != StatusDenied ||
		outcome.Decision.Allowed {
		t.Fatalf("delegation Policy denial was unstable: %#v err=%v", outcome, err)
	}
	executor.mu.Lock()
	calls := executor.calls
	executor.mu.Unlock()
	if calls != 0 || store.chargeCount() != 1 {
		t.Fatalf("denied delegation invoked executor=%d charges=%d", calls, store.chargeCount())
	}
}
