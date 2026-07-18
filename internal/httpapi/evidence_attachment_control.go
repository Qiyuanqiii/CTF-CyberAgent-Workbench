package httpapi

import (
	"net/http"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
)

const (
	EvidenceAttachmentPathTemplate = "/api/v1/runs/{run_id}/evidence-attachments"
	MaxEvidenceAttachmentBodyBytes = 4 * 1024
)

type EvidenceAttachmentRequestView struct {
	Version       string `json:"version"`
	SourceKind    string `json:"source_kind"`
	SourceRef     string `json:"source_ref"`
	ContentSHA256 string `json:"content_sha256"`
}

type EvidenceAttachmentView struct {
	ProtocolVersion       string `json:"protocol_version"`
	AttachmentID          string `json:"attachment_id"`
	RunID                 string `json:"run_id"`
	SessionID             string `json:"session_id"`
	WorkspaceID           string `json:"workspace_id"`
	SourceKind            string `json:"source_kind"`
	SourceRef             string `json:"source_ref"`
	ContentSHA256         string `json:"content_sha256"`
	SessionMessageID      int64  `json:"session_message_id"`
	InstructionAuthorized bool   `json:"instruction_authorized"`
	Replayed              bool   `json:"replayed"`
	ExecutionStarted      bool   `json:"execution_started"`
	ModelCalled           bool   `json:"model_called"`
	ToolCalled            bool   `json:"tool_called"`
	CapabilityGrant       bool   `json:"capability_grant"`
}

func matchEvidenceAttachmentPath(requestPath string) (string, bool) {
	const prefix = "/api/v1/runs/"
	const suffix = "/evidence-attachments"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	if runID == "" || strings.Contains(runID, "/") {
		return "", false
	}
	return runID, true
}

func (a *API) serveEvidenceAttachmentControl(writer http.ResponseWriter,
	request *http.Request, requestID string, runID string,
) {
	const label = "Evidence attachment"
	if !a.authorizeRunOperation(writer, request, requestID,
		a.evidenceAttachmentEnabled, label) {
		return
	}
	if err := validatePathIdentity(runID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validateJSONContentType(request.Header); err != nil {
		a.writeError(writer, requestID, err, http.StatusUnsupportedMediaType)
		return
	}
	operationKey, err := sessionControlIdempotencyKey(request.Header, label)
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := rejectQuery(request.URL.Query()); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readBoundedRequestBody(request, MaxEvidenceAttachmentBodyBytes)
	if err != nil {
		a.writeError(writer, requestID, err, runOperationErrorStatus(err))
		return
	}
	if err := rejectDuplicateJSONObjectFields(body, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	var view EvidenceAttachmentRequestView
	if err := decodeStrictRunOperation(body, &view, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	result, err := application.NewEvidenceAttachmentService(a.store).Attach(
		request.Context(), application.AttachEvidenceRequest{
			Version: view.Version, RunID: runID, SourceKind: view.SourceKind,
			SourceRef: view.SourceRef, ContentSHA256: view.ContentSHA256,
			OperationKey: operationKey, AttachedBy: "http_run_operator",
		})
	if err != nil {
		a.writeError(writer, requestID, apperror.Normalize(err), 0)
		return
	}
	attachment := result.Attachment
	a.writeSuccessStatus(writer, requestID, EvidenceAttachmentView{
		ProtocolVersion: attachment.ProtocolVersion, AttachmentID: attachment.ID,
		RunID: attachment.RunID, SessionID: attachment.SessionID,
		WorkspaceID: attachment.WorkspaceID, SourceKind: attachment.SourceKind,
		SourceRef: attachment.SourceRef, ContentSHA256: attachment.ContentSHA256,
		SessionMessageID: result.Message.ID, InstructionAuthorized: false,
		Replayed: result.Replayed,
	}, nil, http.StatusAccepted)
}
