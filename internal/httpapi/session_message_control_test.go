package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

func TestSessionMessageControlQueuesRedactedMetadataWithoutStartingExecution(t *testing.T) {
	fixture := newAPIFixture(t)
	requestPath := "/api/v1/sessions/" + fixture.run.SessionID + "/messages"
	operationKey := "http-session-message-submit-0001"
	secret := "sk-" + strings.Repeat("m", 32)
	body := `{"version":"session_message_submission.v1","content":"review token=` +
		secret + `"}`
	beforeHistory, err := fixture.store.ListSessionMessages(t.Context(), fixture.run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := fixture.store.ListRunEvents(t.Context(), fixture.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	response := performControlPathRequest(t, fixture.api, requestPath, operationKey,
		strings.NewReader(body))
	var view SessionMessageControlView
	decodeDataStatus(t, response, http.StatusAccepted, &view)
	if view.Version != domain.SessionMessageSubmissionProtocolVersion ||
		view.RunID != fixture.run.ID || view.SessionID != fixture.run.SessionID ||
		view.Steering.ID == "" || view.Steering.Sequence != 1 ||
		view.Steering.Status != string(domain.OperatorSteeringPending) || view.Replayed ||
		view.ExecutionStarted || view.ModelCalled || view.ToolCalled || view.CapabilityGrant {
		t.Fatalf("unexpected Session message control response: %#v", view)
	}
	serialized := response.Body.String()
	for _, forbidden := range []string{secret, operationKey, `"content"`, `"content_sha256"`,
		`"requested_by"`, `"session_message_id"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("Session message response disclosed %q: %s", forbidden, serialized)
		}
	}
	afterHistory, err := fixture.store.ListSessionMessages(t.Context(), fixture.run.SessionID, true)
	if err != nil || len(afterHistory) != len(beforeHistory) {
		t.Fatalf("Session history changed before delivery: before=%d after=%d err=%v",
			len(beforeHistory), len(afterHistory), err)
	}
	afterEvents, err := fixture.store.ListRunEvents(t.Context(), fixture.run.ID)
	if err != nil || countRunEvents(afterEvents, events.OperatorSteeringQueuedEvent) !=
		countRunEvents(beforeEvents, events.OperatorSteeringQueuedEvent)+1 {
		t.Fatalf("queued event was not appended exactly once: err=%v", err)
	}
	persisted, err := fixture.store.GetOperatorSteering(t.Context(), view.Steering.ID)
	if err != nil || strings.Contains(persisted.Content, secret) ||
		!strings.Contains(persisted.Content, "[REDACTED:") {
		t.Fatalf("persisted content was not redacted: message=%#v err=%v", persisted, err)
	}

	replayed := performControlPathRequest(t, fixture.api, requestPath, operationKey,
		strings.NewReader(body))
	var replayView SessionMessageControlView
	decodeDataStatus(t, replayed, http.StatusAccepted, &replayView)
	if !replayView.Replayed || replayView.Steering.ID != view.Steering.ID ||
		replayView.Steering.Sequence != view.Steering.Sequence {
		t.Fatalf("idempotent replay diverged: first=%#v replay=%#v", view, replayView)
	}
	changed := performControlPathRequest(t, fixture.api, requestPath, operationKey,
		strings.NewReader(`{"version":"session_message_submission.v1","content":"changed"}`))
	assertAPIError(t, changed, http.StatusConflict, string(apperror.CodeConflict))
}

func TestSessionMessageControlCapabilityAndBearerAreIndependent(t *testing.T) {
	fixture := newAPIFixture(t)
	requestPath := "/api/v1/sessions/" + fixture.run.SessionID + "/messages"
	body := `{"version":"session_message_submission.v1","content":"capability test"}`
	sessionOnly, err := New(fixture.store, Config{
		AccessToken: testAccessToken, ControlToken: testControlToken,
		SessionMessageEnabled: true, AppVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted := performControlPathRequest(t, sessionOnly, requestPath,
		"http-session-capability-0001", strings.NewReader(body))
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("Session-only capability status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	profile := performControlPathRequest(t, sessionOnly,
		"/api/v1/runs/"+fixture.run.ID+"/execution-profile",
		"http-session-capability-0002", strings.NewReader(`{"profile":"preview"}`))
	assertAPIError(t, profile, http.StatusNotFound, string(apperror.CodeNotFound))
	creation := performControlPathRequest(t, sessionOnly, "/api/v1/runs",
		"http-session-capability-0003",
		strings.NewReader(`{"version":"run_creation.v1","goal":"blocked",`+
			`"workspace_id":"`+fixture.workspace.ID+`"}`))
	assertAPIError(t, creation, http.StatusNotFound, string(apperror.CodeNotFound))

	readBearer := performSessionMessageRequest(t, sessionOnly, http.MethodPost, requestPath,
		testAccessToken, "http-session-capability-0004", "application/json", strings.NewReader(body))
	assertAPIError(t, readBearer, http.StatusUnauthorized, string(apperror.CodePolicyDenied))
	controlRead := fixture.request(t, http.MethodGet, requestPath, testControlToken,
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, controlRead, http.StatusUnauthorized, string(apperror.CodePolicyDenied))

	disabled, err := New(fixture.store, Config{
		AccessToken: testAccessToken, ControlToken: testControlToken, AppVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	disabledResponse := performControlPathRequest(t, disabled, requestPath,
		"http-session-capability-0005", strings.NewReader(body))
	assertAPIError(t, disabledResponse, http.StatusNotFound, string(apperror.CodeNotFound))
	if _, err := New(fixture.store, Config{
		AccessToken: testAccessToken, SessionMessageEnabled: true,
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("capability without token error=%v code=%s", err, apperror.CodeOf(err))
	}
}

func TestSessionMessageControlRejectsMalformedRequests(t *testing.T) {
	fixture := newAPIFixture(t)
	requestPath := "/api/v1/sessions/" + fixture.run.SessionID + "/messages"
	valid := `{"version":"session_message_submission.v1","content":"valid"}`
	tests := []struct {
		name        string
		path        string
		key         string
		contentType string
		body        []byte
		status      int
	}{
		{name: "query", path: requestPath + "?wake=true", key: "http-session-invalid-0001", contentType: "application/json", body: []byte(valid), status: http.StatusBadRequest},
		{name: "missing key", path: requestPath, contentType: "application/json", body: []byte(valid), status: http.StatusBadRequest},
		{name: "short key", path: requestPath, key: "short", contentType: "application/json", body: []byte(valid), status: http.StatusBadRequest},
		{name: "key whitespace", path: requestPath, key: "http-session invalid-0002", contentType: "application/json", body: []byte(valid), status: http.StatusBadRequest},
		{name: "content type", path: requestPath, key: "http-session-invalid-0003", contentType: "application/json; charset=utf-8", body: []byte(valid), status: http.StatusUnsupportedMediaType},
		{name: "invalid utf8", path: requestPath, key: "http-session-invalid-0004", contentType: "application/json", body: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}, status: http.StatusBadRequest},
		{name: "duplicate", path: requestPath, key: "http-session-invalid-0005", contentType: "application/json", body: []byte(`{"version":"session_message_submission.v1","content":"one","content":"two"}`), status: http.StatusBadRequest},
		{name: "unknown", path: requestPath, key: "http-session-invalid-0006", contentType: "application/json", body: []byte(`{"version":"session_message_submission.v1","content":"one","wake":true}`), status: http.StatusBadRequest},
		{name: "trailing", path: requestPath, key: "http-session-invalid-0007", contentType: "application/json", body: []byte(valid + `{}`), status: http.StatusBadRequest},
		{name: "wrong version", path: requestPath, key: "http-session-invalid-0008", contentType: "application/json", body: []byte(`{"version":"other.v1","content":"one"}`), status: http.StatusBadRequest},
		{name: "empty", path: requestPath, key: "http-session-invalid-0009", contentType: "application/json", body: []byte(`{"version":"session_message_submission.v1","content":""}`), status: http.StatusBadRequest},
		{name: "content limit", path: requestPath, key: "http-session-invalid-0010", contentType: "application/json", body: []byte(`{"version":"session_message_submission.v1","content":"` + strings.Repeat("a", domain.MaxOperatorSteeringContentBytes+1) + `"}`), status: http.StatusBadRequest},
		{name: "body limit", path: requestPath, key: "http-session-invalid-0011", contentType: "application/json", body: bytes.Repeat([]byte("x"), MaxSessionMessageRequestBodyBytes+1), status: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performSessionMessageRequest(t, fixture.api, http.MethodPost,
				test.path, testControlToken, test.key, test.contentType, bytes.NewReader(test.body))
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			assertSecurityHeaders(t, response)
		})
	}

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+requestPath,
		strings.NewReader(valid))
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45000"
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	request.Header.Add("Idempotency-Key", "http-session-duplicate-key-0001")
	request.Header.Add("Idempotency-Key", "http-session-duplicate-key-0002")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.api.ServeHTTP(response, request)
	assertAPIError(t, response, http.StatusBadRequest, string(apperror.CodeInvalidArgument))
}

func performSessionMessageRequest(t *testing.T, api *API, method string, requestPath string,
	token string, key string, contentType string, body io.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1"+requestPath, body)
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

func countRunEvents(values []events.Event, eventType string) int {
	count := 0
	for _, value := range values {
		if value.Type == eventType {
			count++
		}
	}
	return count
}

func TestSessionMessageControlResponseJSONHasExactClosedAuthorityShape(t *testing.T) {
	value := SessionMessageControlView{
		Version: domain.SessionMessageSubmissionProtocolVersion,
		RunID:   "run-1", SessionID: "sess-1",
		Steering: OperatorSteeringMessageView{ID: "steer-1", Sequence: 1, Status: "pending"},
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	want := []string{"capability_grant", "execution_started", "model_called", "replayed",
		"run_id", "session_id", "steering", "tool_called", "version"}
	if len(fields) != len(want) {
		t.Fatalf("response fields=%v", fields)
	}
	for _, key := range want {
		if _, found := fields[key]; !found {
			t.Fatalf("response omitted %q", key)
		}
	}
}
