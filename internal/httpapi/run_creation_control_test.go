package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
)

func TestRunCreationControlCreatesAndReplaysClosedRun(t *testing.T) {
	fixture := newAPIFixture(t)
	workspaces := fixture.get(t, "/api/v1/workspaces?limit=10")
	var listed []WorkspaceView
	decodeData(t, workspaces, &listed)
	if len(listed) != 1 || listed[0].ID != fixture.workspace.ID ||
		listed[0].Name != fixture.workspace.Name ||
		strings.Contains(workspaces.Body.String(), fixture.workspace.RootPath) ||
		strings.Contains(workspaces.Body.String(), "root_path") {
		t.Fatalf("Workspace projection leaked or drifted: %s", workspaces.Body.String())
	}

	key := "http-run-create-operation-0001"
	body := `{"version":"run_creation.v1","goal":"Create an HTTP parser",` +
		`"workspace_id":"` + fixture.workspace.ID + `","profile":"code",` +
		`"surface":"code","phase":"plan"}`
	first := performControlPathRequest(t, fixture.api, RunCreationControlPath, key,
		strings.NewReader(body))
	var created RunCreationControlView
	decodeDataStatus(t, first, http.StatusAccepted, &created)
	if created.Replayed || created.Run.Status != string(domain.RunCreated) ||
		!created.Run.Config.Interactive || created.Run.Config.ModelRoute != "code" ||
		created.Run.SessionID != created.Session.ID || created.Session.WorkspaceID != fixture.workspace.ID ||
		created.Mission.Scope.NetworkMode != "disabled" ||
		len(created.Mission.Scope.AllowedTargets) != 0 || created.Mode.Phase != "plan" ||
		created.Mode.CapabilityGrant {
		t.Fatalf("controlled Run response drifted: %#v", created)
	}
	replay := performControlPathRequest(t, fixture.api, RunCreationControlPath, key,
		strings.NewReader(body))
	var replayed RunCreationControlView
	decodeDataStatus(t, replay, http.StatusAccepted, &replayed)
	if !replayed.Replayed || replayed.Run.ID != created.Run.ID ||
		replayed.Session.ID != created.Session.ID || replayed.Mission.ID != created.Mission.ID {
		t.Fatalf("HTTP replay changed identity: %#v", replayed)
	}
	changed := strings.Replace(body, "Create an HTTP parser", "Create another parser", 1)
	assertAPIError(t, performControlPathRequest(t, fixture.api, RunCreationControlPath,
		key, strings.NewReader(changed)), http.StatusConflict, "CONFLICT")
}

func TestRunCreationControlFailsClosedAtHTTPBoundary(t *testing.T) {
	fixture := newAPIFixture(t)
	validBody := `{"version":"run_creation.v1","goal":"Boundary test",` +
		`"workspace_id":"` + fixture.workspace.ID + `"}`
	request := func(token, key, contentType, target string, body string) *httptest.ResponseRecorder {
		t.Helper()
		value := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+target,
			strings.NewReader(body))
		value.Host = "127.0.0.1:8765"
		value.RemoteAddr = "127.0.0.1:45000"
		if token != "" {
			value.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			value.Header.Set("Idempotency-Key", key)
		}
		if contentType != "" {
			value.Header.Set("Content-Type", contentType)
		}
		response := httptest.NewRecorder()
		fixture.api.ServeHTTP(response, value)
		return response
	}
	key := "http-run-create-operation-0002"
	assertAPIError(t, request(testAccessToken, key, "application/json",
		RunCreationControlPath, validBody), http.StatusUnauthorized, "POLICY_DENIED")
	assertAPIError(t, request(testControlToken, "", "application/json",
		RunCreationControlPath, validBody), http.StatusBadRequest, "INVALID_ARGUMENT")
	assertAPIError(t, request(testControlToken, key, "text/plain",
		RunCreationControlPath, validBody), http.StatusUnsupportedMediaType, "INVALID_ARGUMENT")
	assertAPIError(t, request(testControlToken, key, "application/json",
		RunCreationControlPath+"?start=true", validBody), http.StatusBadRequest, "INVALID_ARGUMENT")
	duplicate := strings.TrimSuffix(validBody, "}") + `,"goal":"duplicate"}`
	assertAPIError(t, request(testControlToken, key, "application/json",
		RunCreationControlPath, duplicate), http.StatusBadRequest, "INVALID_ARGUMENT")
	unknown := strings.TrimSuffix(validBody, "}") + `,"model":"forbidden"}`
	assertAPIError(t, request(testControlToken, key, "application/json",
		RunCreationControlPath, unknown), http.StatusBadRequest, "INVALID_ARGUMENT")
	invalidUTF8 := strings.Replace(validBody, "Boundary test", string([]byte{'B', 0xff}), 1)
	assertAPIError(t, request(testControlToken, key, "application/json",
		RunCreationControlPath, invalidUTF8), http.StatusBadRequest, "INVALID_ARGUMENT")

	disabled, err := New(fixture.store, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken})
	if err != nil {
		t.Fatal(err)
	}
	disabledRequest := httptest.NewRequest(http.MethodPost,
		"http://127.0.0.1"+RunCreationControlPath, strings.NewReader(validBody))
	disabledRequest.Host = "127.0.0.1:8765"
	disabledRequest.RemoteAddr = "127.0.0.1:45000"
	disabledRequest.Header.Set("Authorization", "Bearer "+testControlToken)
	disabledRequest.Header.Set("Idempotency-Key", key)
	disabledRequest.Header.Set("Content-Type", "application/json")
	disabledResponse := httptest.NewRecorder()
	disabled.ServeHTTP(disabledResponse, disabledRequest)
	assertAPIError(t, disabledResponse, http.StatusNotFound, "NOT_FOUND")
}

func TestRunCreationControlResponseMatchesStrictDTO(t *testing.T) {
	fixture := newAPIFixture(t)
	body := `{"version":"run_creation.v1","goal":"Strict DTO",` +
		`"workspace_id":"` + fixture.workspace.ID + `"}`
	response := performControlPathRequest(t, fixture.api, RunCreationControlPath,
		"http-run-create-operation-0003", strings.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Version   string                 `json:"version"`
		RequestID string                 `json:"request_id"`
		Data      RunCreationControlView `json:"data"`
	}
	decoder := json.NewDecoder(strings.NewReader(response.Body.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || envelope.Version != Version ||
		envelope.RequestID == "" || envelope.Data.Run.ID == "" {
		t.Fatalf("strict response rejected: %#v err=%v", envelope, err)
	}
}
