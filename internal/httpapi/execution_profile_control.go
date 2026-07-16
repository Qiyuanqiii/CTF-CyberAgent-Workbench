package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
)

const RunExecutionProfileControlPathTemplate = "/api/v1/runs/{run_id}/execution-profile"

type RunExecutionProfileControlRequestView struct {
	Profile string `json:"profile"`
	Reason  string `json:"reason,omitempty"`
}

type RunExecutionProfileControlView struct {
	ExecutionProfile RunExecutionProfileView `json:"execution_profile"`
	Replayed         bool                    `json:"replayed"`
}

func matchRunExecutionProfileControlPath(requestPath string) (string, bool) {
	const prefix = "/api/v1/runs/"
	const suffix = "/execution-profile"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	if runID == "" || strings.Contains(runID, "/") {
		return "", false
	}
	return runID, true
}

func (a *API) serveRunExecutionProfileControl(writer http.ResponseWriter,
	request *http.Request, requestID string, runID string,
) {
	if !a.controlEnabled {
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
				"Run execution profile endpoint only supports POST"),
			http.StatusMethodNotAllowed)
		return
	}
	if err := validatePathIdentity(runID); err != nil {
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
	operationKey, err := runExecutionProfileIdempotencyKey(request.Header)
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
	var view RunExecutionProfileControlRequestView
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		a.writeError(writer, requestID, apperror.Wrap(apperror.CodeInvalidArgument,
			"Run execution profile body must be one JSON object", err), 0)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	service := application.NewRunExecutionProfileService(a.store)
	result, err := service.Change(request.Context(),
		application.ChangeRunExecutionProfileRequest{
			RunID: runID, Profile: view.Profile, OperationKey: operationKey,
			RequestedBy: "http_control", Reason: view.Reason,
		})
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	a.writeSuccessStatus(writer, requestID, RunExecutionProfileControlView{
		ExecutionProfile: runExecutionProfileView(result.Profile),
		Replayed:         result.Replayed,
	}, nil, http.StatusAccepted)
}

func runExecutionProfileIdempotencyKey(header http.Header) (string, error) {
	values := header.Values("Idempotency-Key")
	if len(values) != 1 {
		return "", apperror.New(apperror.CodeInvalidArgument,
			"Run execution profile requires exactly one Idempotency-Key header")
	}
	value, err := domain.NormalizeAgentOperationKey(values[0])
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return value, nil
}
