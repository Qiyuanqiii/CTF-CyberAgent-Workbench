package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

const httpPlanDeliveryPayload = `{"version":"plan_delivery.v1","directions":[` +
	`{"title":"Conservative","summary":"Keep changes narrow.","tradeoffs":["More sequential work"],"modules":[{"title":"Inspect","objective":"Inspect current boundaries.","acceptance_criteria":["Boundaries recorded"],"dependencies":[]}]},` +
	`{"title":"Balanced","summary":"Deliver a vertical slice.","tradeoffs":["Moderate breadth"],"modules":[{"title":"Implement","objective":"Implement the core path.","acceptance_criteria":["Focused tests pass"],"dependencies":[]}]},` +
	`{"title":"Accelerated","summary":"Prepare independent slices.","tradeoffs":["Higher review load"],"modules":[{"title":"Prepare","objective":"Prepare independent work.","acceptance_criteria":["Work stays bounded"],"dependencies":[]}]}]}`

func TestRunDetailProjectsPlanDeliveryWithoutGrantingControl(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "http-plan-delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "project a bounded plan", Profile: "review", Phase: "plan",
		ModelRoute: "http-plan/model",
		Budget:     domain.Budget{MaxTurns: 4, MaxTokens: 1000, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	provider := &httpPlanProvider{responses: []*llm.ChatResponse{
		{Provider: "http-plan", Model: "model", Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
			ToolCalls: []llm.ToolCall{{ID: "http-plan-call", Name: "plan_delivery_propose",
				Arguments: json.RawMessage(httpPlanDeliveryPayload)}}},
		{Text: httpRootWaitResponse(t), Provider: "http-plan", Model: "model",
			Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}},
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	if _, err := application.NewRunSupervisor(st, router, policy.NewDefaultChecker()).Step(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	api, err := New(st, Config{AccessToken: testAccessToken, AppVersion: "test-version"})
	if err != nil {
		t.Fatal(err)
	}
	read := func() (*RunDetailView, string) {
		response := performRequest(t, api, http.MethodGet, "/api/v1/runs/"+run.ID,
			testAccessToken, "127.0.0.1:8765", "127.0.0.1:45000", nil)
		var detail RunDetailView
		decodeData(t, response, &detail)
		return &detail, response.Body.String()
	}
	detail, _ := read()
	if detail.PlanDelivery == nil || detail.PlanDelivery.Proposal == nil ||
		!detail.PlanDelivery.OperatorChoiceNeeded || detail.PlanDelivery.Selection != nil ||
		detail.PlanDelivery.CapabilityGrant || len(detail.PlanDelivery.Proposal.Directions) != 3 {
		t.Fatalf("pending Plan/Delivery projection is incomplete: %#v", detail.PlanDelivery)
	}
	planJSON, err := json.Marshal(detail.PlanDelivery)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"proposal_fingerprint", "requested_by", "root_agent_id"} {
		if strings.Contains(string(planJSON), forbidden) {
			t.Fatalf("Plan/Delivery HTTP projection exposed %s: %s", forbidden, planJSON)
		}
	}
	result, err := application.NewPlanDeliveryService(st).Select(ctx,
		application.SelectPlanDeliveryDirectionRequest{
			ProposalID: detail.PlanDelivery.Proposal.ID, Direction: 2,
			OperationKey: "http-plan-choice-0001", RequestedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ = read()
	if detail.PlanDelivery == nil || detail.PlanDelivery.Selection == nil ||
		detail.PlanDelivery.Selection.ID != result.Selection.ID ||
		detail.PlanDelivery.Selection.DirectionOrdinal != 2 ||
		len(detail.PlanDelivery.Selection.Items) != 1 ||
		!detail.PlanDelivery.PhaseChangeNeeded || detail.PlanDelivery.OperatorChoiceNeeded ||
		detail.Mode.Phase != string(domain.ExecutionPhasePlan) {
		t.Fatalf("selected Plan/Delivery projection drifted: %#v", detail.PlanDelivery)
	}
}

type httpPlanProvider struct {
	responses []*llm.ChatResponse
}

func (*httpPlanProvider) Name() string { return "http-plan" }

func (*httpPlanProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: "http-plan", Capabilities: []string{"chat", "tools"}}}, nil
}

func (p *httpPlanProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(p.responses) == 0 {
		return nil, errors.New("HTTP Plan provider response queue is empty")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	copy := *response
	copy.ToolCalls = append([]llm.ToolCall{}, response.ToolCalls...)
	return &copy, nil
}

func (p *httpPlanProvider) StreamChat(ctx context.Context, request llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	response, err := p.Chat(ctx, request)
	if err != nil {
		return nil, err
	}
	chunks := make(chan llm.ChatChunk, 2)
	if response.Text != "" {
		chunks <- llm.ChatChunk{Text: response.Text}
	}
	chunks <- llm.FinalChatChunk(response)
	close(chunks)
	return chunks, nil
}

func (*httpPlanProvider) SupportsTools(string) bool    { return true }
func (*httpPlanProvider) SupportsVision(string) bool   { return false }
func (*httpPlanProvider) SupportsJSONMode(string) bool { return true }

func httpRootWaitResponse(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(domain.RootAction{Version: domain.RootLifecycleVersion,
		Kind: domain.RootActionWait, Message: "three directions are ready",
		Reason: "operator direction choice required"})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
