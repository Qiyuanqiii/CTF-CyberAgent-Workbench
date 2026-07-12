package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

const (
	ModelCancellationPathTemplate           = "/api/v1/runs/{run_id}/active-call/cancel"
	SpecialistModelCancellationPathTemplate = "/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel"
	MaxControlRequestBodyBytes              = 4 * 1024
)

type ModelCancellationRequestView struct {
	AttemptID    string `json:"attempt_id"`
	ModelAttempt int    `json:"model_attempt"`
	Reason       string `json:"reason,omitempty"`
}

type ModelCancellationView struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	AttemptID    string    `json:"attempt_id"`
	ModelAttempt int       `json:"model_attempt"`
	Status       string    `json:"status"`
	RequestedAt  time.Time `json:"requested_at"`
	Replayed     bool      `json:"replayed"`
}

type SpecialistModelCancellationView struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	AgentID      string    `json:"agent_id"`
	AttemptID    string    `json:"attempt_id"`
	ModelAttempt int       `json:"model_attempt"`
	Status       string    `json:"status"`
	RequestedAt  time.Time `json:"requested_at"`
	Replayed     bool      `json:"replayed"`
}

func matchSpecialistModelCancellationPath(requestPath string) (string, string, bool) {
	const prefix = "/api/v1/runs/"
	if !strings.HasPrefix(requestPath, prefix) {
		return "", "", false
	}
	segments := strings.Split(strings.TrimPrefix(requestPath, prefix), "/")
	if len(segments) != 5 || segments[0] == "" || segments[1] != "agents" ||
		segments[2] == "" || segments[3] != "active-call" || segments[4] != "cancel" {
		return "", "", false
	}
	return segments[0], segments[2], true
}

func matchModelCancellationPath(requestPath string) (string, bool) {
	const prefix = "/api/v1/runs/"
	const suffix = "/active-call/cancel"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	if runID == "" || strings.Contains(runID, "/") {
		return "", false
	}
	return runID, true
}

func (a *API) serveModelCancellation(writer http.ResponseWriter, request *http.Request,
	requestID string, runID string,
) {
	a.serveCancellation(writer, request, requestID, runID, "")
}

func (a *API) serveSpecialistModelCancellation(writer http.ResponseWriter,
	request *http.Request, requestID string, runID string, agentID string,
) {
	a.serveCancellation(writer, request, requestID, runID, agentID)
}

func (a *API) serveCancellation(writer http.ResponseWriter, request *http.Request,
	requestID string, runID string, agentID string,
) {
	if !a.controlEnabled {
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeNotFound, "HTTP API endpoint was not found"), http.StatusNotFound)
		return
	}
	if !a.authorized(request, a.controlTokenHash) {
		writer.Header().Set("WWW-Authenticate", `Bearer realm="CyberAgent Control API"`)
		a.writeError(writer, requestID,
			apperror.New(apperror.CodePolicyDenied, "valid control bearer authorization is required"),
			http.StatusUnauthorized)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeInvalidArgument, "model cancellation endpoint only supports POST"),
			http.StatusMethodNotAllowed)
		return
	}
	if err := validatePathIdentity(runID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if agentID != "" {
		if err := validatePathIdentity(agentID); err != nil {
			a.writeError(writer, requestID, err, 0)
			return
		}
	}
	if err := rejectQuery(request.URL.Query()); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validateJSONContentType(request.Header); err != nil {
		a.writeError(writer, requestID, err, http.StatusUnsupportedMediaType)
		return
	}
	key, err := modelCancellationIdempotencyKey(request.Header)
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readBoundedControlBody(request)
	if err != nil {
		status := 0
		if apperror.CodeOf(apperror.Normalize(err)) == apperror.CodeResourceExhausted {
			status = http.StatusRequestEntityTooLarge
		}
		a.writeError(writer, requestID, err, status)
		return
	}
	var view ModelCancellationRequestView
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		a.writeError(writer, requestID,
			apperror.Wrap(apperror.CodeInvalidArgument, "model cancellation body must be one JSON object", err), 0)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if agentID == "" {
		result, err := a.store.RequestSupervisorModelCancellation(request.Context(), domain.RequestModelCancellation{
			RunID: runID, AttemptID: view.AttemptID, ModelAttempt: view.ModelAttempt,
			IdempotencyKey: key, Reason: view.Reason, RequestedBy: "http_control",
		})
		if err != nil {
			a.writeError(writer, requestID, err, 0)
			return
		}
		a.writeSuccessStatus(writer, requestID, modelCancellationView(result), nil,
			http.StatusAccepted)
		return
	}
	result, err := a.store.RequestSpecialistModelCancellation(request.Context(),
		domain.RequestSpecialistModelCancellation{
			RunID: runID, AgentID: agentID, AttemptID: view.AttemptID,
			ModelAttempt: view.ModelAttempt, IdempotencyKey: key,
			Reason: view.Reason, RequestedBy: "http_control",
		})
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	a.writeSuccessStatus(writer, requestID, specialistModelCancellationView(result), nil,
		http.StatusAccepted)
}

func specialistModelCancellationView(
	result domain.SpecialistModelCancellationResult,
) SpecialistModelCancellationView {
	value := result.Cancellation
	return SpecialistModelCancellationView{
		ID: value.ID, RunID: value.RunID, AgentID: value.AgentID,
		AttemptID: value.AttemptID, ModelAttempt: value.ModelAttempt,
		Status: string(value.Status), RequestedAt: value.RequestedAt,
		Replayed: result.Replayed,
	}
}

func validateJSONContentType(header http.Header) error {
	values := header.Values("Content-Type")
	if len(values) != 1 {
		return apperror.New(apperror.CodeInvalidArgument,
			"model cancellation requires one application/json Content-Type")
	}
	mediaType, parameters, err := mime.ParseMediaType(values[0])
	if err != nil || mediaType != "application/json" || len(parameters) != 0 {
		return apperror.New(apperror.CodeInvalidArgument,
			"model cancellation Content-Type must be application/json")
	}
	return nil
}

func modelCancellationIdempotencyKey(header http.Header) (string, error) {
	values := header.Values("Idempotency-Key")
	if len(values) != 1 {
		return "", apperror.New(apperror.CodeInvalidArgument,
			"model cancellation requires exactly one Idempotency-Key header")
	}
	probe := domain.RequestModelCancellation{
		RunID: "validation", AttemptID: "validation", ModelAttempt: 1,
		IdempotencyKey: values[0], RequestedBy: "http_control",
	}
	normalized, err := probe.Normalize()
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return normalized.IdempotencyKey, nil
}

func readBoundedControlBody(request *http.Request) ([]byte, error) {
	if request.Body == nil || request.ContentLength == 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument, "model cancellation JSON body is required")
	}
	if request.ContentLength > MaxControlRequestBodyBytes {
		return nil, apperror.New(apperror.CodeResourceExhausted,
			"model cancellation request body exceeds its limit")
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxControlRequestBodyBytes+1))
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, "read model cancellation request body", err)
	}
	if len(body) == 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument, "model cancellation JSON body is required")
	}
	if len(body) > MaxControlRequestBodyBytes {
		return nil, apperror.New(apperror.CodeResourceExhausted,
			"model cancellation request body exceeds its limit")
	}
	return body, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return apperror.New(apperror.CodeInvalidArgument,
		"model cancellation body must contain exactly one JSON object")
}

func modelCancellationView(result domain.ModelCancellationResult) ModelCancellationView {
	value := result.Cancellation
	return ModelCancellationView{
		ID: value.ID, RunID: value.RunID, AttemptID: value.AttemptID,
		ModelAttempt: value.ModelAttempt, Status: string(value.Status),
		RequestedAt: value.RequestedAt, Replayed: result.Replayed,
	}
}
