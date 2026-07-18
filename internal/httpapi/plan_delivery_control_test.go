package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

func TestPlanDeliveryHTTPControlsRequireExplicitSeparateOperations(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "http-plan-control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runs := application.NewRunService(st)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "control a bounded Plan", Profile: "review", Phase: "plan",
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
		{Provider: "http-plan", Model: "model",
			Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
			ToolCalls: []llm.ToolCall{{ID: "http-plan-control-call",
				Name: "plan_delivery_propose", Arguments: json.RawMessage(httpPlanDeliveryPayload)}}},
		{Text: httpRootWaitResponse(t), Provider: "http-plan", Model: "model",
			Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}},
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	if _, err := application.NewRunSupervisor(st, router,
		policy.NewDefaultChecker()).Step(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	proposals, err := st.ListPlanDeliveryProposals(ctx, run.ID, 2)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("Plan proposals=%#v err=%v", proposals, err)
	}
	controller := application.NewPlanDeliveryControlService(st)
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		PlanDeliveryControlEnabled: true, PlanDeliveryController: controller,
		AppVersion: "plan-control-test"})
	if err != nil {
		t.Fatal(err)
	}
	directionBody := `{"version":"plan_delivery_control.v1","proposal_id":"` +
		proposals[0].ID + `","direction":2}`
	direction := planControlRequest(t, api,
		"/api/v1/runs/"+run.ID+"/plan/direction", directionBody,
		"http-plan-direction-0001")
	if direction.Code != http.StatusAccepted {
		t.Fatalf("direction status=%d body=%s", direction.Code, direction.Body.String())
	}
	var directionEnvelope struct {
		Data PlanDirectionControlView `json:"data"`
	}
	if err := json.Unmarshal(direction.Body.Bytes(), &directionEnvelope); err != nil {
		t.Fatal(err)
	}
	if directionEnvelope.Data.Direction != 2 || directionEnvelope.Data.WorkItemCount != 1 ||
		directionEnvelope.Data.PhaseChanged || directionEnvelope.Data.ExecutionStarted ||
		directionEnvelope.Data.ModelCalled || directionEnvelope.Data.ToolCalled ||
		directionEnvelope.Data.CapabilityGrant {
		t.Fatalf("direction response widened authority: %#v", directionEnvelope.Data)
	}
	mode, err := st.GetRunMode(ctx, run.ID)
	if err != nil || mode.Phase != domain.ExecutionPhasePlan {
		t.Fatalf("direction changed phase: %#v err=%v", mode, err)
	}
	deliver := planControlRequest(t, api,
		"/api/v1/runs/"+run.ID+"/plan/deliver",
		`{"version":"plan_delivery_control.v1"}`, "http-plan-deliver-0001")
	if deliver.Code != http.StatusAccepted {
		t.Fatalf("Deliver status=%d body=%s", deliver.Code, deliver.Body.String())
	}
	var deliverEnvelope struct {
		Data PlanDeliveryTransitionControlView `json:"data"`
	}
	if err := json.Unmarshal(deliver.Body.Bytes(), &deliverEnvelope); err != nil {
		t.Fatal(err)
	}
	if deliverEnvelope.Data.AppliedMode.Phase != "deliver" ||
		deliverEnvelope.Data.CurrentMode.Phase != "deliver" ||
		deliverEnvelope.Data.ExecutionStarted || deliverEnvelope.Data.ModelCalled ||
		deliverEnvelope.Data.ToolCalled || deliverEnvelope.Data.CapabilityGrant {
		t.Fatalf("Deliver response widened authority: %#v", deliverEnvelope.Data)
	}
	replay := planControlRequest(t, api,
		"/api/v1/runs/"+run.ID+"/plan/deliver",
		`{"version":"plan_delivery_control.v1"}`, "http-plan-deliver-0001")
	if replay.Code != http.StatusAccepted ||
		!bytes.Contains(replay.Body.Bytes(), []byte(`"replayed":true`)) {
		t.Fatalf("Deliver replay status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func TestPlanDeliveryHTTPControlCapabilityIsIndependent(t *testing.T) {
	fixture := newAPIFixture(t)
	request := planControlRequest(t, fixture.api,
		"/api/v1/runs/"+fixture.run.ID+"/plan/deliver",
		`{"version":"plan_delivery_control.v1"}`, "http-plan-disabled-0001")
	assertAPIError(t, request, http.StatusNotFound, "NOT_FOUND")
}

func planControlRequest(t *testing.T, api *API, path string, body string,
	operationKey string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path,
		bytes.NewBufferString(body))
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45000"
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", operationKey)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}
