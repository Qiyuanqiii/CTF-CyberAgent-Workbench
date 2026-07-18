package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

func TestRunLifecycleControlHTTPIsIdempotentAndMetadataOnly(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-lifecycle-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, run, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "HTTP lifecycle", Profile: "code",
			Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	controller := application.NewRunLifecycleControlService(st)
	api, err := New(st, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, AppVersion: "test",
		RunLifecycleEnabled: true, RunLifecycleController: controller})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(RunLifecycleControlPathTemplate, "{run_id}", run.ID)
	body := `{"version":"run_lifecycle_control.v1","action":"start"}`
	response := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-run-lifecycle-start-0001", "application/json",
		strings.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data RunLifecycleControlView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	view := envelope.Data
	if view.Version != domain.RunLifecycleControlProtocolVersion ||
		view.Run.ID != run.ID || view.Run.Status != string(domain.RunRunning) ||
		view.Action != "start" || view.ExpectedStatus != "created" ||
		view.AppliedStatus != "running" || view.EventSequenceEnd != view.EventSequenceStart+1 ||
		view.Replayed || view.ExecutionStarted || view.ModelCalled || view.ToolCalled ||
		view.CapabilityGrant {
		t.Fatalf("unexpected lifecycle response: %#v", view)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-run-lifecycle-start-0001", "application/json",
		strings.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.Replayed || envelope.Data.EventSequenceStart != view.EventSequenceStart {
		t.Fatalf("lifecycle replay diverged: %#v", envelope.Data)
	}
}

func TestRunExecutionControlHTTPUsesBoundedSupervisorAndDurableMetadata(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-execute-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runs := application.NewRunService(st)
	_, created, err := runs.Create(t.Context(), application.CreateRunRequest{
		Goal: "HTTP bounded execution", Profile: "code", ModelRoute: "mock/mock-code",
		Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Start(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	secretContent := "execute selected content that must not appear in response"
	for index, content := range []string{secretContent, "leave this later input queued"} {
		if _, err := st.EnqueueOperatorSteering(t.Context(),
			domain.EnqueueOperatorSteeringRequest{
				RunID: run.ID, SessionID: run.SessionID, Content: content,
				OperationKey: "http-run-execution-queue-000" + string(rune('1'+index)),
				RequestedBy:  "test_operator",
			}); err != nil {
			t.Fatal(err)
		}
	}
	controller := application.NewRunExecutionHandoffService(st, llm.NewDefaultRouter(),
		policy.NewDefaultChecker())
	api, err := New(st, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, AppVersion: "test",
		RunExecutionEnabled: true, RunExecutionController: controller})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(RunExecutionControlPathTemplate, "{run_id}", run.ID)
	body := `{"version":"run_execution_handoff.v1","max_steps":1}`
	response := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-run-execution-0001", "application/json",
		strings.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secretContent) ||
		strings.Contains(response.Body.String(), "lease-") {
		t.Fatalf("execution response leaked private material: %s", response.Body.String())
	}
	var envelope struct {
		Data RunExecutionControlView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	view := envelope.Data
	if view.Version != domain.RunExecutionHandoffProtocolVersion ||
		view.RunID != run.ID || view.SessionID != run.SessionID || view.MaxSteps != 1 ||
		view.SelectedCount != 1 || view.Status != "completed" ||
		view.StepsCompleted != 1 || view.CommittedCount != 1 ||
		!view.ExecutionStarted || !view.ModelCalled || view.ToolCalled ||
		view.CapabilityGrant || view.Replayed {
		t.Fatalf("unexpected execution response: %#v", view)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-run-execution-0001", "application/json",
		strings.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.Replayed || envelope.Data.OperationID != view.OperationID ||
		envelope.Data.CompletionEventSequence != view.CompletionEventSequence ||
		envelope.Data.ModelCalled != view.ModelCalled {
		t.Fatalf("execution replay diverged: %#v", envelope.Data)
	}
	summary, err := st.GetOperatorSteeringQueueSummary(t.Context(), run.ID)
	if err != nil || summary.Pending != 1 || summary.Committed != 1 {
		t.Fatalf("bounded execution crossed queue boundary: summary=%#v err=%v", summary, err)
	}
}

func TestRunOperationControlsRejectDisabledUnauthorizedAndMalformedRequests(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-operation-boundary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, run, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "HTTP boundaries", Profile: "code",
			Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(RunLifecycleControlPathTemplate, "{run_id}", run.ID)
	body := `{"version":"run_lifecycle_control.v1","action":"start"}`
	disabled, err := New(st, Config{AccessToken: testAccessToken, AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	assertAPIError(t, performSessionMessageRequest(t, disabled, http.MethodPost, path,
		testControlToken, "run-control-disabled-0001", "application/json",
		strings.NewReader(body)), http.StatusNotFound, string(apperror.CodeNotFound))
	controller := application.NewRunLifecycleControlService(st)
	enabled, err := New(st, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, AppVersion: "test",
		RunLifecycleEnabled: true, RunLifecycleController: controller})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, token, key, contentType, body string
		status                              int
	}{
		{name: "read token", token: testAccessToken, key: "run-control-bad-0001",
			contentType: "application/json", body: body, status: http.StatusUnauthorized},
		{name: "missing key", token: testControlToken, contentType: "application/json",
			body: body, status: http.StatusBadRequest},
		{name: "content type", token: testControlToken, key: "run-control-bad-0002",
			contentType: "text/plain", body: body, status: http.StatusUnsupportedMediaType},
		{name: "duplicate", token: testControlToken, key: "run-control-bad-0003",
			contentType: "application/json",
			body:        `{"version":"run_lifecycle_control.v1","action":"start","action":"pause"}`,
			status:      http.StatusBadRequest},
		{name: "unknown", token: testControlToken, key: "run-control-bad-0004",
			contentType: "application/json",
			body:        `{"version":"run_lifecycle_control.v1","action":"start","force":true}`,
			status:      http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performSessionMessageRequest(t, enabled, http.MethodPost, path,
				test.token, test.key, test.contentType, strings.NewReader(test.body))
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status,
					response.Body.String())
			}
		})
	}
	if _, err := New(st, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, RunExecutionEnabled: true,
		AppVersion: "test"}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("missing execution controller code=%s err=%v", apperror.CodeOf(err), err)
	}
}
