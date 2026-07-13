package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/tools"
)

type PlanDeliveryContext struct {
	InvocationID    string
	OperationKey    string
	RunID           string
	RootAgentID     string
	SessionID       string
	WorkspaceID     string
	LeaseID         string
	LeaseGeneration int64
	RequestedBy     string
	PolicyDecision  Decision
}

func (c PlanDeliveryContext) Validate() error {
	for label, value := range map[string]string{
		"invocation id": c.InvocationID, "operation key": c.OperationKey,
		"run id": c.RunID, "root agent id": c.RootAgentID,
		"session id": c.SessionID, "workspace id": c.WorkspaceID,
		"lease id": c.LeaseID, "requester": c.RequestedBy,
	} {
		if !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
			len([]rune(value)) > MaxToolIdentityRunes {
			return fmt.Errorf("Plan/Delivery %s must be normalized and bounded UTF-8", label)
		}
	}
	if c.InvocationID == "" || c.OperationKey == "" || c.RunID == "" ||
		c.RootAgentID == "" || c.SessionID == "" || c.LeaseID == "" ||
		c.RequestedBy != "run_supervisor" || c.LeaseGeneration <= 0 {
		return errors.New("Plan/Delivery proposal requires a fenced root Supervisor scope")
	}
	if !domain.ValidAgentID(c.RootAgentID) {
		return errors.New("Plan/Delivery root Agent identity is invalid")
	}
	if err := c.PolicyDecision.Validate(); err != nil {
		return err
	}
	if !c.PolicyDecision.Allowed || c.PolicyDecision.Approval != ApprovalAutomatic {
		return errors.New("Plan/Delivery proposal creation requires an automatic allowed decision")
	}
	return nil
}

type PlanDeliveryResult struct {
	ProposalID     string
	Status         domain.PlanDeliveryProposalStatus
	DirectionCount int
	Version        int64
	Replayed       bool
}

func (r PlanDeliveryResult) Validate() error {
	if !domain.ValidAgentID(r.ProposalID) ||
		r.Status != domain.PlanDeliveryProposalProposed ||
		r.DirectionCount != domain.PlanDeliveryDirectionCount || r.Version != 1 {
		return errors.New("Plan/Delivery result is invalid")
	}
	return nil
}

type PlanDeliveryExecutor interface {
	ProposePlan(ctx context.Context, scope PlanDeliveryContext,
		spec domain.PlanDeliverySpec) (PlanDeliveryResult, error)
}

var planDeliveryDefinition = ToolDefinition{
	Name: PlanDeliveryProposeTool, Class: ClassAgentProposal,
	Approval:    ApprovalAutomatic,
	Description: "Record exactly three bounded Plan/Delivery directions for operator choice without selecting one, changing phase, or executing work.",
	InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["version","directions"],"properties":{"version":{"const":"plan_delivery.v1"},"directions":{"type":"array","minItems":3,"maxItems":3,"items":{"type":"object","additionalProperties":false,"required":["title","summary","tradeoffs","modules"],"properties":{"title":{"type":"string","minLength":1,"maxLength":240},"summary":{"type":"string","minLength":1,"maxLength":1200},"tradeoffs":{"type":"array","minItems":1,"maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":512}},"modules":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"object","additionalProperties":false,"required":["title","objective","acceptance_criteria","dependencies"],"properties":{"title":{"type":"string","minLength":1,"maxLength":240},"objective":{"type":"string","minLength":1,"maxLength":2400},"acceptance_criteria":{"type":"array","minItems":1,"maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":512}},"dependencies":{"type":"array","maxItems":7,"uniqueItems":true,"items":{"type":"integer","minimum":1,"maximum":7}}}}}}}}}}`),
}

func PlanPhaseSupervisorToolDefinitions() []ToolDefinition {
	definitions := SupervisorToolDefinitions()
	definition := planDeliveryDefinition
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	return append(definitions, definition)
}

func AllSupervisorToolDefinitions() []ToolDefinition {
	return PlanPhaseSupervisorToolDefinitions()
}

func normalizePlanDeliveryPayload(payload json.RawMessage) (domain.PlanDeliverySpec,
	json.RawMessage, error,
) {
	spec, err := domain.DecodePlanDeliverySpec(payload)
	if err != nil {
		return domain.PlanDeliverySpec{}, nil, err
	}
	for directionIndex := range spec.Directions {
		direction := &spec.Directions[directionIndex]
		direction.Title = redact.String(direction.Title)
		direction.Summary = redact.String(direction.Summary)
		for index := range direction.Tradeoffs {
			direction.Tradeoffs[index] = redact.String(direction.Tradeoffs[index])
		}
		for moduleIndex := range direction.Modules {
			module := &direction.Modules[moduleIndex]
			module.Title = redact.String(module.Title)
			module.Objective = redact.String(module.Objective)
			for index := range module.AcceptanceCriteria {
				module.AcceptanceCriteria[index] = redact.String(
					module.AcceptanceCriteria[index])
			}
		}
	}
	spec, err = domain.NormalizePlanDeliverySpec(spec)
	if err != nil {
		return domain.PlanDeliverySpec{}, nil,
			fmt.Errorf("redacted Plan/Delivery payload is invalid: %w", err)
	}
	canonical, err := json.Marshal(spec)
	if err != nil {
		return domain.PlanDeliverySpec{}, nil, err
	}
	return spec, canonical, nil
}

func (g *Gateway) WithPlanDeliveryExecutor(executor PlanDeliveryExecutor) *Gateway {
	if g != nil {
		g.planDeliveryProposals = executor
	}
	return g
}

func (g *Gateway) invokePlanDelivery(ctx context.Context, call ToolCall) (Outcome, error) {
	spec, canonical, err := normalizePlanDeliveryPayload(call.Payload)
	if err != nil {
		return Outcome{}, err
	}
	call.Payload = canonical
	policyDecision := g.checker.CheckToolCall(tools.Call{
		Name: string(call.Name), Args: map[string]string{"payload": string(canonical)},
	})
	if !policyDecision.Allowed {
		if err := g.recordPlanDeliveryPolicyDecision(ctx, call, policyDecision); err != nil {
			return Outcome{}, err
		}
		return deniedOutcome(call, policyDecision)
	}
	if policyDecision.NeedsApproval {
		policyDecision.Reason = "proposal recorded for mandatory operator choice: " +
			policyDecision.Reason
	}
	decision, err := gatewayDecision(policyDecision, ApprovalAutomatic, "low")
	if err != nil {
		return Outcome{}, err
	}
	scope := PlanDeliveryContext{
		InvocationID: call.InvocationID, OperationKey: call.OperationKey,
		RunID: call.RunID, RootAgentID: call.AgentID, SessionID: call.SessionID,
		WorkspaceID: call.WorkspaceID, LeaseID: call.LeaseID,
		LeaseGeneration: call.LeaseGeneration, RequestedBy: call.RequestedBy,
		PolicyDecision: decision,
	}
	if err := scope.Validate(); err != nil {
		return Outcome{}, err
	}
	started := time.Now().UTC()
	result, err := g.planDeliveryProposals.ProposePlan(ctx, scope, spec)
	if err != nil {
		return Outcome{}, err
	}
	if err := result.Validate(); err != nil {
		return Outcome{}, err
	}
	completed := time.Now().UTC()
	outcome := Outcome{
		Call: safeToolCall(call), Decision: decision,
		Execution: &Execution{Backend: "plan_proposal", Status: StatusCompleted,
			StartedAt: started, CompletedAt: &completed},
		Result: &Result{
			Status: StatusCompleted, ExitCode: 0, MIME: "application/json",
			CompletedAt: completed,
			Metadata: map[string]string{
				"proposal_id": result.ProposalID, "status": string(result.Status),
				"direction_count":      strconv.Itoa(result.DirectionCount),
				"version":              strconv.FormatInt(result.Version, 10),
				"selection_authorized": "false", "phase_change_authorized": "false",
				"execution_authorized": "false",
				"replayed":             strconv.FormatBool(result.Replayed),
			},
		},
	}
	return validateOutcome(outcome, nil)
}

func (g *Gateway) recordPlanDeliveryPolicyDecision(ctx context.Context, call ToolCall,
	decision policy.Decision,
) error {
	if g == nil || g.policyRecorder == nil {
		return errors.New("Plan/Delivery policy decision recorder is required")
	}
	return g.policyRecorder.RecordPolicyDecision(ctx, policy.DecisionRecord{
		SessionID: call.SessionID, SubjectID: call.InvocationID,
		Context: "tool_run." + string(call.Name), Decision: decision,
	})
}
