package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/coordinator"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

func TestModelCancellationControlUsesDistinctCapabilityAndDurableIdempotency(t *testing.T) {
	fixture := newAPIFixture(t)
	requestBody := `{"attempt_id":"` + fixture.checkpoint.AttemptID + `","model_attempt":1,"reason":"operator stop"}`
	first := fixture.controlRequest(t, http.MethodPost, fixture.run.ID, testControlToken,
		"control-operation-0123456789", "application/json", strings.NewReader(requestBody))
	var created ModelCancellationView
	decodeDataStatus(t, first, http.StatusAccepted, &created)
	if created.ID == "" || created.RunID != fixture.run.ID || created.AttemptID != fixture.checkpoint.AttemptID ||
		created.ModelAttempt != 1 || created.Status != string(domain.ModelCancellationPending) || created.Replayed {
		t.Fatalf("unexpected cancellation response: %#v", created)
	}
	replay := fixture.controlRequest(t, http.MethodPost, fixture.run.ID, testControlToken,
		"control-operation-0123456789", "application/json", strings.NewReader(requestBody))
	var replayed ModelCancellationView
	decodeDataStatus(t, replay, http.StatusAccepted, &replayed)
	if !replayed.Replayed || replayed.ID != created.ID {
		t.Fatalf("HTTP idempotent replay changed cancellation: %#v", replayed)
	}
	changedBody := `{"attempt_id":"` + fixture.checkpoint.AttemptID + `","model_attempt":2}`
	conflict := fixture.controlRequest(t, http.MethodPost, fixture.run.ID, testControlToken,
		"control-operation-0123456789", "application/json", strings.NewReader(changedBody))
	assertAPIError(t, conflict, http.StatusConflict, "CONFLICT")
	stored, err := fixture.store.GetModelCancellation(t.Context(), created.ID)
	if err != nil || stored.ID != created.ID || stored.Reason != "operator stop" {
		t.Fatalf("HTTP cancellation was not persisted: %#v err=%v", stored, err)
	}

	readToken := fixture.controlRequest(t, http.MethodPost, fixture.run.ID, testAccessToken,
		"read-token-operation-012345", "application/json", strings.NewReader(requestBody))
	assertAPIError(t, readToken, http.StatusUnauthorized, "POLICY_DENIED")
	controlRead := fixture.request(t, http.MethodGet, "/api/v1/health", testControlToken,
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, controlRead, http.StatusUnauthorized, "POLICY_DENIED")

	disabled, err := New(fixture.store, Config{AccessToken: testAccessToken})
	if err != nil {
		t.Fatal(err)
	}
	disabledResponse := performControlRequest(t, disabled, http.MethodPost, fixture.run.ID, testControlToken,
		"disabled-operation-01234567", "application/json", strings.NewReader(requestBody))
	assertAPIError(t, disabledResponse, http.StatusNotFound, "NOT_FOUND")
	if _, err := New(fixture.store, Config{AccessToken: testAccessToken, ControlToken: testAccessToken}); err == nil {
		t.Fatal("HTTP API accepted one token for both read and control capabilities")
	}
}

func TestModelCancellationControlRejectsMalformedOrOversizedRequests(t *testing.T) {
	fixture := newAPIFixture(t)
	valid := `{"attempt_id":"` + fixture.checkpoint.AttemptID + `","model_attempt":1}`
	secretField := "sk-" + strings.Repeat("q", 32)
	tests := []struct {
		name        string
		key         string
		contentType string
		body        string
		status      int
		code        string
	}{
		{name: "missing key", contentType: "application/json", body: valid, status: 400, code: "INVALID_ARGUMENT"},
		{name: "short key", key: "short", contentType: "application/json", body: valid, status: 400, code: "INVALID_ARGUMENT"},
		{name: "missing content type", key: "missing-type-0123456789", body: valid, status: 415, code: "INVALID_ARGUMENT"},
		{name: "content type parameters", key: "type-parameters-01234567", contentType: "application/json; charset=utf-8", body: valid, status: 415, code: "INVALID_ARGUMENT"},
		{name: "unknown field", key: "unknown-field-012345678", contentType: "application/json", body: strings.TrimSuffix(valid, "}") + `,"lease_id":"forbidden"}`, status: 400, code: "INVALID_ARGUMENT"},
		{name: "secret-shaped unknown field", key: "secret-field-012345678", contentType: "application/json", body: strings.TrimSuffix(valid, "}") + `,"` + secretField + `":"x"}`, status: 400, code: "INVALID_ARGUMENT"},
		{name: "trailing object", key: "trailing-object-01234567", contentType: "application/json", body: valid + `{}`, status: 400, code: "INVALID_ARGUMENT"},
		{name: "empty body", key: "empty-body-012345678901", contentType: "application/json", body: "", status: 400, code: "INVALID_ARGUMENT"},
		{name: "oversized body", key: "oversized-body-01234567", contentType: "application/json", body: strings.Repeat("x", MaxControlRequestBodyBytes+1), status: 413, code: "RESOURCE_EXHAUSTED"},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			response := fixture.controlRequest(t, http.MethodPost, fixture.run.ID, testControlToken,
				current.key, current.contentType, strings.NewReader(current.body))
			assertAPIError(t, response, current.status, current.code)
			assertSecurityHeaders(t, response)
			if strings.Contains(response.Body.String(), secretField) {
				t.Fatal("control parser error exposed a secret-shaped field")
			}
		})
	}
	wrongMethod := fixture.controlRequest(t, http.MethodGet, fixture.run.ID, testControlToken,
		"wrong-method-012345678", "application/json", strings.NewReader(valid))
	assertAPIError(t, wrongMethod, http.StatusMethodNotAllowed, "INVALID_ARGUMENT")
	if wrongMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("control endpoint omitted Allow header: %#v", wrongMethod.Header())
	}
}

func TestModelCancellationControlStopsProviderAcrossSQLiteConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control-integration.db")
	workerStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer workerStore.Close()
	provider := &httpControlBlockingProvider{entered: make(chan struct{})}
	service := application.NewRunService(workerStore)
	_, run, err := service.Create(t.Context(), application.CreateRunRequest{
		Goal: "cancel through local control API", Profile: "code", ModelRoute: provider.Name() + "/model",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	supervisor := application.NewRunSupervisor(workerStore, router, policy.NewDefaultChecker()).
		WithModelCancellationPollInterval(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stepDone := make(chan error, 1)
	go func() {
		_, err := supervisor.Step(ctx, run.ID)
		stepDone <- err
	}()
	select {
	case <-provider.entered:
	case <-ctx.Done():
		t.Fatal("provider did not start before HTTP cancellation")
	}
	active, found := supervisor.ActiveCall(run.ID)
	if !found {
		t.Fatal("Supervisor did not publish its active call")
	}
	controlStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer controlStore.Close()
	api, err := New(controlStore, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, RunControlEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"attempt_id":"` + active.AttemptID + `","model_attempt":` + strconv.Itoa(active.ModelAttempt) + `}`
	response := performControlRequest(t, api, http.MethodPost, run.ID, testControlToken,
		"end-to-end-control-012345678", "application/json", strings.NewReader(body))
	var accepted ModelCancellationView
	decodeDataStatus(t, response, http.StatusAccepted, &accepted)
	select {
	case stepErr := <-stepDone:
		if apperror.CodeOf(stepErr) != apperror.CodeCancelled {
			t.Fatalf("cancelled Provider returned code=%s err=%v", apperror.CodeOf(stepErr), stepErr)
		}
	case <-ctx.Done():
		t.Fatal("HTTP cancellation did not stop the Provider")
	}
	resolved, err := controlStore.GetModelCancellation(t.Context(), accepted.ID)
	if err != nil || resolved.Status != domain.ModelCancellationResolved || resolved.Resolution != "cancelled" {
		t.Fatalf("HTTP cancellation did not resolve: %#v err=%v", resolved, err)
	}
	if strings.Contains(response.Body.String(), testControlToken) || strings.Contains(response.Body.String(), active.AttemptID+"/model/") {
		t.Fatalf("control response exposed a credential or internal subject: %s", response.Body.String())
	}
}

func TestSpecialistModelCancellationControlStopsExactChildAcrossSQLiteConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specialist-control-integration.db")
	workerStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer workerStore.Close()
	provider := &httpControlBlockingProvider{entered: make(chan struct{})}
	runs := application.NewRunService(workerStore)
	_, run, err := runs.Create(t.Context(), application.CreateRunRequest{
		Goal: "cancel one exact Specialist call through control API", Profile: "code",
		ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = runs.Start(t.Context(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, found, err := workerStore.GetRootAgent(t.Context(), run.ID)
	if err != nil || !found {
		t.Fatalf("Specialist control root missing: found=%t err=%v", found, err)
	}
	coord, err := coordinator.NewWithSpecialistAdmission(workerStore,
		coordinator.SpecialistAdmissionPolicy{
			MaxChildren: 1, MaxTurnsPerChild: 2, MaxTokensPerChild: 64,
		})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := coord.AdmitSpecialist(t.Context(), coordinator.AdmitSpecialistRequest{
		RunID: run.ID, ParentAgentID: root.ID, Title: "cancellable Specialist",
		Skills: []string{"model.chat"}, TurnLimit: 2, TokenLimit: 64,
		IdempotencyKey: "http-specialist-admission-012345",
	})
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	runner := application.NewSpecialistRunner(workerStore, router,
		policy.NewDefaultChecker()).WithModelCancellationPollInterval(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runner.Step(ctx, run.ID, admitted.Agent.ID)
		done <- err
	}()
	select {
	case <-provider.entered:
	case <-ctx.Done():
		t.Fatal("Specialist Provider did not start before control request")
	}
	attempts, err := workerStore.ListAgentAttempts(ctx, admitted.Agent.ID)
	if err != nil || len(attempts) != 1 || attempts[0].Status != domain.AgentAttemptRunning {
		t.Fatalf("active Specialist Attempt missing: attempts=%#v err=%v", attempts, err)
	}
	controlStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer controlStore.Close()
	api, err := New(controlStore, Config{
		AccessToken: testAccessToken, ControlToken: testControlToken, RunControlEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	requestPath := "/api/v1/runs/" + run.ID + "/agents/" + admitted.Agent.ID +
		"/active-call/cancel"
	body := `{"attempt_id":"` + attempts[0].ID + `","model_attempt":1,"reason":"operator child stop"}`
	operationKey := "http-specialist-control-012345"
	response := performControlPathRequest(t, api, requestPath, operationKey,
		strings.NewReader(body))
	var accepted SpecialistModelCancellationView
	decodeDataStatus(t, response, http.StatusAccepted, &accepted)
	if accepted.AgentID != admitted.Agent.ID || accepted.AttemptID != attempts[0].ID ||
		accepted.ModelAttempt != 1 || accepted.Status != string(domain.ModelCancellationPending) {
		t.Fatalf("unexpected Specialist cancellation response: %#v", accepted)
	}
	select {
	case runErr := <-done:
		if apperror.CodeOf(runErr) != apperror.CodeCancelled {
			t.Fatalf("cancelled Specialist returned code=%s err=%v",
				apperror.CodeOf(runErr), runErr)
		}
	case <-ctx.Done():
		t.Fatal("Specialist control request did not stop the Provider")
	}
	resolved, err := controlStore.GetSpecialistModelCancellation(t.Context(), accepted.ID)
	if err != nil || resolved.Status != domain.ModelCancellationResolved ||
		resolved.Resolution != string(llm.OutcomeCancelled) {
		t.Fatalf("Specialist cancellation did not resolve: %#v err=%v", resolved, err)
	}
	if strings.Contains(response.Body.String(), operationKey) ||
		strings.Contains(response.Body.String(), `"lease_id"`) ||
		strings.Contains(response.Body.String(), `"lease_generation"`) {
		t.Fatalf("Specialist control response exposed private data: %s", response.Body.String())
	}
}

type httpControlBlockingProvider struct {
	entered chan struct{}
	once    sync.Once
}

func (*httpControlBlockingProvider) Name() string { return "http-control-test" }

func (p *httpControlBlockingProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: p.Name()}}, nil
}

func (*httpControlBlockingProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, apperror.New(apperror.CodeInternal, "streaming is required")
}

func (p *httpControlBlockingProvider) StreamChat(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	chunks := make(chan llm.ChatChunk)
	p.once.Do(func() { close(p.entered) })
	go func() {
		defer close(chunks)
		<-ctx.Done()
	}()
	return chunks, nil
}

func (*httpControlBlockingProvider) SupportsTools(string) bool    { return false }
func (*httpControlBlockingProvider) SupportsVision(string) bool   { return false }
func (*httpControlBlockingProvider) SupportsJSONMode(string) bool { return true }

func (f *apiFixture) controlRequest(t *testing.T, method string, runID string, token string,
	key string, contentType string, body *strings.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	return performControlRequest(t, f.api, method, runID, token, key, contentType, body)
}

func performControlRequest(t *testing.T, api *API, method string, runID string, token string,
	key string, contentType string, body *strings.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1/api/v1/runs/"+runID+"/active-call/cancel", body)
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45000"
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}

func performControlPathRequest(t *testing.T, api *API, requestPath string, key string,
	body *strings.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+requestPath, body)
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45000"
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}

func decodeDataStatus[T any](t *testing.T, response *httptest.ResponseRecorder, status int, target *T) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("API status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	var envelope apiTestEnvelope
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Version != Version || envelope.RequestID == "" || envelope.Error != nil {
		t.Fatalf("invalid success envelope: %#v", envelope)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatal(err)
	}
}
