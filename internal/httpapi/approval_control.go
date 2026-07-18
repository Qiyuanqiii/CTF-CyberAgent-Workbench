package httpapi

import (
	"context"
	"net/http"
	"strings"

	"cyberagent-workbench/internal/application"
)

const ApprovalDecisionControlPathTemplate = "/api/v1/runs/{run_id}/approvals/{approval_id}/decision"

type ApprovalController interface {
	Decide(context.Context, application.DecideApprovalControlRequest) (
		application.DecideApprovalControlResult, error)
}

type ApprovalDecisionControlRequestView struct {
	Version string                            `json:"version"`
	Action  application.ApprovalControlAction `json:"action"`
	Reason  string                            `json:"reason,omitempty"`
}

type ApprovalDecisionControlView struct {
	Version                 string                            `json:"version"`
	RunID                   string                            `json:"run_id"`
	ApprovalID              string                            `json:"approval_id"`
	ProposalID              string                            `json:"proposal_id"`
	ToolName                string                            `json:"tool_name"`
	Action                  application.ApprovalControlAction `json:"action"`
	Status                  string                            `json:"status"`
	Replayed                bool                              `json:"replayed"`
	ProcessExecutionEnabled bool                              `json:"process_execution_enabled"`
	ShellExecutionEnabled   bool                              `json:"shell_execution_enabled"`
	DockerExecutionEnabled  bool                              `json:"docker_execution_enabled"`
	WorkspaceWriteApplied   bool                              `json:"workspace_write_applied"`
	SessionGrantCreated     bool                              `json:"session_grant_created"`
	CapabilityGrant         bool                              `json:"capability_grant"`
}

func matchApprovalDecisionControlPath(requestPath string) (string, string, bool) {
	const prefix = "/api/v1/runs/"
	const middle = "/approvals/"
	const suffix = "/decision"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", "", false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	parts := strings.Split(value, middle)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" ||
		strings.Contains(parts[0], "/") || strings.Contains(parts[1], "/") {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (a *API) serveApprovalDecisionControl(writer http.ResponseWriter,
	request *http.Request, requestID string, runID string, approvalID string,
) {
	if !a.authorizeRunOperation(writer, request, requestID,
		a.approvalControlEnabled, "Approval decision") {
		return
	}
	if err := validatePathIdentity(runID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validatePathIdentity(approvalID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validateJSONContentType(request.Header); err != nil {
		a.writeError(writer, requestID, err, http.StatusUnsupportedMediaType)
		return
	}
	operationKey, body, err := a.readRunOperationRequest(request, "Approval decision control")
	if err != nil {
		a.writeError(writer, requestID, err, runOperationErrorStatus(err))
		return
	}
	var view ApprovalDecisionControlRequestView
	if err := decodeStrictRunOperation(body, &view, "Approval decision control"); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	result, err := a.approvalController.Decide(request.Context(),
		application.DecideApprovalControlRequest{
			Version: view.Version, RunID: runID, ApprovalID: approvalID,
			Action: view.Action, OperationKey: operationKey,
			ReviewedBy: "http_approval_operator", Reason: view.Reason,
		})
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	a.writeSuccessStatus(writer, requestID, ApprovalDecisionControlView{
		Version: application.ApprovalControlProtocolVersion,
		RunID:   result.Approval.RunID, ApprovalID: result.Approval.ID,
		ProposalID: result.Approval.ProposalID, ToolName: result.Approval.ToolName,
		Action: result.Action, Status: string(result.Approval.Status),
		Replayed: result.Replayed,
	}, nil, http.StatusAccepted)
}
