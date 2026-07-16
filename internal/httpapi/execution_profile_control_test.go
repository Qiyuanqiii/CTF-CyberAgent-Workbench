package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
)

func TestRunExecutionProfileControlIsCapabilitySeparatedAndNonAuthorizing(t *testing.T) {
	fixture := newAPIFixture(t)
	_, run, err := application.NewRunService(fixture.store).Create(t.Context(),
		application.CreateRunRequest{
			Goal: "select execution profile through HTTP", Profile: "code",
			Budget: domain.Budget{MaxTurns: 2},
		})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/runs/" + run.ID + "/execution-profile"
	body := `{"profile":"docker","reason":"prefer isolated execution"}`
	key := "http-execution-profile-0001"
	first := performControlPathRequest(t, fixture.api, path, key, strings.NewReader(body))
	if raw := first.Body.String(); strings.Contains(raw, "requested_by") ||
		strings.Contains(raw, "prefer isolated execution") {
		t.Fatalf("HTTP profile response exposed private audit text: %s", raw)
	}
	var selected RunExecutionProfileControlView
	decodeDataStatus(t, first, http.StatusAccepted, &selected)
	profile := selected.ExecutionProfile
	if selected.Replayed || profile.Profile != string(domain.RunExecutionProfileDocker) ||
		profile.Backend != string(domain.ExecutionBackendDocker) ||
		profile.RequiredGate != string(domain.ExecutionGateDockerProductionStart) ||
		profile.ProcessEnabled || profile.ExecutionAuthorized || profile.CapabilityGrant {
		t.Fatalf("HTTP profile selection escaped its boundary: %#v", selected)
	}
	replay := performControlPathRequest(t, fixture.api, path, key, strings.NewReader(body))
	var replayed RunExecutionProfileControlView
	decodeDataStatus(t, replay, http.StatusAccepted, &replayed)
	if !replayed.Replayed || replayed.ExecutionProfile.Revision != profile.Revision {
		t.Fatalf("HTTP profile replay changed result: %#v", replayed)
	}
	readToken := performExecutionProfileControlRequest(t, fixture.api, path,
		testAccessToken, "http-execution-profile-read-token", strings.NewReader(body))
	assertAPIError(t, readToken, http.StatusUnauthorized, "POLICY_DENIED")
	disabled, err := New(fixture.store, Config{AccessToken: testAccessToken})
	if err != nil {
		t.Fatal(err)
	}
	disabledResponse := performExecutionProfileControlRequest(t, disabled, path,
		testControlToken, "http-execution-profile-disabled", strings.NewReader(body))
	assertAPIError(t, disabledResponse, http.StatusNotFound, "NOT_FOUND")
}

func TestRunExecutionProfileControlRejectsMalformedAndRunningRequests(t *testing.T) {
	fixture := newAPIFixture(t)
	path := "/api/v1/runs/" + fixture.run.ID + "/execution-profile"
	running := performControlPathRequest(t, fixture.api, path,
		"http-execution-profile-running", strings.NewReader(`{"profile":"docker"}`))
	assertAPIError(t, running, http.StatusPreconditionFailed, "FAILED_PRECONDITION")
	_, run, err := application.NewRunService(fixture.store).Create(t.Context(),
		application.CreateRunRequest{Goal: "malformed profile target", Profile: "code",
			Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	createdPath := "/api/v1/runs/" + run.ID + "/execution-profile"
	unknown := performControlPathRequest(t, fixture.api, createdPath,
		"http-execution-profile-unknown", strings.NewReader(
			`{"profile":"docker","execution_authorized":true}`))
	assertAPIError(t, unknown, http.StatusBadRequest, "INVALID_ARGUMENT")
	invalid := performControlPathRequest(t, fixture.api, createdPath,
		"http-execution-profile-invalid", strings.NewReader(`{"profile":"full-host"}`))
	assertAPIError(t, invalid, http.StatusBadRequest, "INVALID_ARGUMENT")
	empty := performControlPathRequest(t, fixture.api, createdPath,
		"http-execution-profile-empty", strings.NewReader(""))
	assertAPIError(t, empty, http.StatusBadRequest, "INVALID_ARGUMENT")
	if strings.Contains(strings.ToLower(empty.Body.String()), "cancellation") {
		t.Fatalf("execution-profile parse error leaked cancellation wording: %s", empty.Body.String())
	}
}

func performExecutionProfileControlRequest(t *testing.T, api *API, requestPath string,
	token string, key string, body *strings.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+requestPath, body)
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45000"
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}
