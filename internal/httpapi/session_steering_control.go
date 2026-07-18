package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
)

const (
	SessionSteeringCancellationPathTemplate = "/api/v1/sessions/{session_id}/messages/{message_id}/cancel"
	MaxSessionSteeringCancellationBodyBytes = 4 * 1024
)

type SessionSteeringCancellationRequestView struct {
	Version string `json:"version"`
	Reason  string `json:"reason"`
}

type SessionSteeringCancellationView struct {
	Version          string                      `json:"version"`
	RunID            string                      `json:"run_id"`
	SessionID        string                      `json:"session_id"`
	Steering         OperatorSteeringMessageView `json:"steering"`
	CancellationID   string                      `json:"cancellation_id"`
	CancellationKind string                      `json:"cancellation_kind"`
	Replayed         bool                        `json:"replayed"`
	ExecutionStarted bool                        `json:"execution_started"`
	ModelCalled      bool                        `json:"model_called"`
	ToolCalled       bool                        `json:"tool_called"`
	CapabilityGrant  bool                        `json:"capability_grant"`
}

func matchSessionSteeringCancellationPath(requestPath string) (string, string, bool) {
	const prefix = "/api/v1/sessions/"
	if !strings.HasPrefix(requestPath, prefix) {
		return "", "", false
	}
	segments := strings.Split(strings.TrimPrefix(requestPath, prefix), "/")
	if len(segments) != 4 || segments[0] == "" || segments[1] != "messages" ||
		segments[2] == "" || segments[3] != "cancel" {
		return "", "", false
	}
	return segments[0], segments[2], true
}

func (a *API) serveSessionSteeringCancellation(writer http.ResponseWriter,
	request *http.Request, requestID string, sessionID string, messageID string,
) {
	if !a.sessionSteeringControlEnabled {
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeNotFound, "HTTP API endpoint was not found"),
			http.StatusNotFound)
		return
	}
	if !a.authorized(request, a.controlTokenHash) {
		writer.Header().Set("WWW-Authenticate", `Bearer realm="CyberAgent Control API"`)
		a.writeError(writer, requestID, apperror.New(apperror.CodePolicyDenied,
			"valid control bearer authorization is required"), http.StatusUnauthorized)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		a.writeError(writer, requestID, apperror.New(apperror.CodeInvalidArgument,
			"Session steering cancellation endpoint only supports POST"),
			http.StatusMethodNotAllowed)
		return
	}
	if err := validatePathIdentity(sessionID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validatePathIdentity(messageID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := rejectQuery(request.URL.Query()); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validateJSONContentType(request.Header); err != nil {
		a.writeError(writer, requestID, err, http.StatusUnsupportedMediaType)
		return
	}
	operationKey, err := sessionControlIdempotencyKey(request.Header,
		"Session steering cancellation")
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readBoundedRequestBody(request, MaxSessionSteeringCancellationBodyBytes)
	if err != nil {
		status := 0
		if apperror.CodeOf(apperror.Normalize(err)) == apperror.CodeResourceExhausted {
			status = http.StatusRequestEntityTooLarge
		}
		a.writeError(writer, requestID, err, status)
		return
	}
	if !utf8.Valid(body) {
		a.writeError(writer, requestID, apperror.New(apperror.CodeInvalidArgument,
			"Session steering cancellation body must be valid UTF-8 JSON"), 0)
		return
	}
	if err := rejectDuplicateJSONObjectFields(body, "Session steering cancellation"); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	var view SessionSteeringCancellationRequestView
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		a.writeError(writer, requestID, apperror.Wrap(apperror.CodeInvalidArgument,
			"Session steering cancellation body must be one JSON object", err), 0)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}

	result, err := application.NewSessionSteeringCancellationService(a.store).Cancel(
		request.Context(), application.CancelSessionSteeringRequest{
			Version: view.Version, SessionID: sessionID, MessageID: messageID,
			OperationKey: operationKey, RequestedBy: "http_session_operator",
			Reason: view.Reason,
		})
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	a.writeSuccessStatus(writer, requestID, SessionSteeringCancellationView{
		Version: domain.SessionSteeringCancellationProtocolVersion,
		RunID:   result.Run.ID, SessionID: result.Session.ID,
		Steering:         operatorSteeringMessageView(result.Message),
		CancellationID:   result.Cancellation.ID,
		CancellationKind: string(result.Cancellation.Kind), Replayed: result.Replayed,
	}, nil, http.StatusAccepted)
}

func sessionControlIdempotencyKey(header http.Header, label string) (string, error) {
	values := header.Values("Idempotency-Key")
	if len(values) != 1 {
		return "", apperror.New(apperror.CodeInvalidArgument,
			label+" requires exactly one Idempotency-Key header")
	}
	value, err := domain.NormalizeAgentOperationKey(values[0])
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	for _, current := range value {
		if unicode.IsSpace(current) || unicode.IsControl(current) {
			return "", apperror.New(apperror.CodeInvalidArgument,
				label+" idempotency key cannot contain whitespace or control characters")
		}
	}
	return value, nil
}
