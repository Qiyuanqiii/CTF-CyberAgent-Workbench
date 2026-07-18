package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

func TestSessionSteeringCancellationControlCancelsOnlyPendingMetadata(t *testing.T) {
	fixture := newAPIFixture(t)
	queued, err := fixture.store.EnqueueOperatorSteering(t.Context(),
		domain.EnqueueOperatorSteeringRequest{
			RunID: fixture.run.ID, SessionID: fixture.run.SessionID,
			Content:      "cancel through HTTP",
			OperationKey: "http-session-steering-queue-0001",
			RequestedBy:  "test_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	beforeHistory, err := fixture.store.ListSessionMessages(t.Context(), fixture.run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.NewReplacer("{session_id}", fixture.run.SessionID,
		"{message_id}", queued.Message.ID).Replace(SessionSteeringCancellationPathTemplate)
	body := `{"version":"session_steering_cancellation.v1",` +
		`"reason":"operator withdrew queued message"}`
	response := performSessionMessageRequest(t, fixture.api, http.MethodPost, path,
		testControlToken, "http-session-steering-cancel-0001", "application/json",
		strings.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data SessionSteeringCancellationView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	view := envelope.Data
	if view.Version != domain.SessionSteeringCancellationProtocolVersion ||
		view.RunID != fixture.run.ID || view.SessionID != fixture.run.SessionID ||
		view.Steering.ID != queued.Message.ID ||
		view.Steering.Status != string(domain.OperatorSteeringCancelled) ||
		view.CancellationID == "" || view.CancellationKind != "operator" || view.Replayed ||
		view.ExecutionStarted || view.ModelCalled || view.ToolCalled || view.CapabilityGrant {
		t.Fatalf("unexpected cancellation response: %#v", view)
	}
	replay := performSessionMessageRequest(t, fixture.api, http.MethodPost, path,
		testControlToken, "http-session-steering-cancel-0001", "application/json",
		strings.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	var replayEnvelope struct {
		Data SessionSteeringCancellationView `json:"data"`
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayEnvelope); err != nil {
		t.Fatal(err)
	}
	if !replayEnvelope.Data.Replayed ||
		replayEnvelope.Data.CancellationID != view.CancellationID {
		t.Fatalf("replay diverged: %#v", replayEnvelope.Data)
	}
	history, err := fixture.store.ListSessionMessages(t.Context(), fixture.run.SessionID, true)
	if err != nil || len(history) != len(beforeHistory) {
		t.Fatalf("cancellation changed Session history: before=%d after=%d err=%v",
			len(beforeHistory), len(history), err)
	}
	lease, found, err := fixture.store.GetRunExecutionLease(t.Context(), fixture.run.ID)
	if err != nil || !found || !lease.ActiveAt(lease.RenewedAt) {
		t.Fatalf("cancellation changed active execution: lease=%#v found=%t err=%v", lease, found, err)
	}
}

func TestSessionSteeringCancellationCapabilityIsIndependent(t *testing.T) {
	fixture := newAPIFixture(t)
	queued, err := fixture.store.EnqueueOperatorSteering(t.Context(),
		domain.EnqueueOperatorSteeringRequest{
			RunID: fixture.run.ID, SessionID: fixture.run.SessionID, Content: "independent",
			OperationKey: "http-session-steering-independent-queue-0001",
			RequestedBy:  "test_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	api, err := New(fixture.store, Config{
		AccessToken: testAccessToken, ControlToken: testControlToken,
		SessionSteeringControlEnabled: true, AppVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.NewReplacer("{session_id}", fixture.run.SessionID,
		"{message_id}", queued.Message.ID).Replace(SessionSteeringCancellationPathTemplate)
	body := `{"version":"session_steering_cancellation.v1","reason":"independent"}`
	accepted := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-session-steering-independent-cancel-0001",
		"application/json", strings.NewReader(body))
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	messagePath := strings.ReplaceAll(SessionMessageControlPathTemplate,
		"{session_id}", fixture.run.SessionID)
	assertAPIError(t, performSessionMessageRequest(t, api, http.MethodPost, messagePath,
		testControlToken, "http-session-steering-sibling-0001", "application/json",
		strings.NewReader(`{"version":"session_message_submission.v1","content":"no"}`)),
		http.StatusNotFound, string(apperror.CodeNotFound))
}

func TestSessionSteeringCancellationRejectsMalformedBoundary(t *testing.T) {
	fixture := newAPIFixture(t)
	path := strings.NewReplacer("{session_id}", fixture.run.SessionID,
		"{message_id}", "steer-missing").Replace(SessionSteeringCancellationPathTemplate)
	valid := `{"version":"session_steering_cancellation.v1","reason":"bounded"}`
	tests := []struct {
		name, method, token, key, contentType, body, suffix string
		status                                              int
	}{
		{name: "read bearer", method: http.MethodPost, token: testAccessToken,
			key: "http-session-steering-invalid-0001", contentType: "application/json",
			body: valid, status: http.StatusUnauthorized},
		{name: "missing key", method: http.MethodPost, token: testControlToken,
			contentType: "application/json", body: valid, status: http.StatusBadRequest},
		{name: "query", method: http.MethodPost, token: testControlToken,
			key: "http-session-steering-invalid-0002", contentType: "application/json",
			body: valid, suffix: "?force=true", status: http.StatusBadRequest},
		{name: "unknown", method: http.MethodPost, token: testControlToken,
			key: "http-session-steering-invalid-0003", contentType: "application/json",
			body:   `{"version":"session_steering_cancellation.v1","reason":"x","force":true}`,
			status: http.StatusBadRequest},
		{name: "duplicate", method: http.MethodPost, token: testControlToken,
			key: "http-session-steering-invalid-0004", contentType: "application/json",
			body:   `{"version":"session_steering_cancellation.v1","reason":"x","reason":"y"}`,
			status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performSessionMessageRequest(t, fixture.api, test.method,
				path+test.suffix, test.token, test.key, test.contentType,
				strings.NewReader(test.body))
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status,
					response.Body.String())
			}
		})
	}
}
