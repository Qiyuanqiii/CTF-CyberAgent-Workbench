package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
)

const (
	SessionMessageControlPathTemplate = "/api/v1/sessions/{session_id}/messages"
	MaxSessionMessageRequestBodyBytes = 128 * 1024
)

type SessionMessageControlRequestView struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

type SessionMessageControlView struct {
	Version          string                      `json:"version"`
	RunID            string                      `json:"run_id"`
	SessionID        string                      `json:"session_id"`
	Steering         OperatorSteeringMessageView `json:"steering"`
	Replayed         bool                        `json:"replayed"`
	ExecutionStarted bool                        `json:"execution_started"`
	ModelCalled      bool                        `json:"model_called"`
	ToolCalled       bool                        `json:"tool_called"`
	CapabilityGrant  bool                        `json:"capability_grant"`
}

func matchSessionMessageControlPath(requestPath string) (string, bool) {
	const prefix = "/api/v1/sessions/"
	const suffix = "/messages"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	sessionID := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	if sessionID == "" || strings.Contains(sessionID, "/") {
		return "", false
	}
	return sessionID, true
}

func (a *API) serveSessionMessageControl(writer http.ResponseWriter,
	request *http.Request, requestID string, sessionID string,
) {
	if !a.sessionMessageEnabled {
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeNotFound, "HTTP API endpoint was not found"),
			http.StatusNotFound)
		return
	}
	if !a.authorized(request, a.controlTokenHash) {
		writer.Header().Set("WWW-Authenticate", `Bearer realm="CyberAgent Control API"`)
		a.writeError(writer, requestID,
			apperror.New(apperror.CodePolicyDenied,
				"valid control bearer authorization is required"),
			http.StatusUnauthorized)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeInvalidArgument,
				"Session message endpoint only supports POST"),
			http.StatusMethodNotAllowed)
		return
	}
	if err := validatePathIdentity(sessionID); err != nil {
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
	operationKey, err := sessionMessageIdempotencyKey(request.Header)
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readBoundedRequestBody(request, MaxSessionMessageRequestBodyBytes)
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
			"Session message body must be valid UTF-8 JSON"), 0)
		return
	}
	if err := rejectDuplicateJSONObjectFields(body, "Session message"); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	var view SessionMessageControlRequestView
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		a.writeError(writer, requestID, apperror.Wrap(apperror.CodeInvalidArgument,
			"Session message body must be one JSON object", err), 0)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}

	result, err := application.NewSessionMessageSubmissionService(a.store).Submit(
		request.Context(), application.SubmitSessionMessageRequest{
			Version: view.Version, SessionID: sessionID, Content: view.Content,
			OperationKey: operationKey, RequestedBy: "http_session_operator",
		})
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	a.writeSuccessStatus(writer, requestID, SessionMessageControlView{
		Version: domain.SessionMessageSubmissionProtocolVersion,
		RunID:   result.Run.ID, SessionID: result.Session.ID,
		Steering: operatorSteeringMessageView(result.Message),
		Replayed: result.Replayed,
	}, nil, http.StatusAccepted)
}

func sessionMessageIdempotencyKey(header http.Header) (string, error) {
	return sessionControlIdempotencyKey(header, "Session message")
}
