package application

import (
	"context"
	"errors"
	"strings"

	"cyberagent-workbench/internal/toolgateway"
)

type ScriptProcessService struct {
	runs    *RunService
	gateway *toolgateway.Gateway
}

type CreateScriptProcessRunRequest struct {
	Run          CreateRunRequest
	OperationKey string
	RequestedBy  string
	Process      toolgateway.ScriptProcessProposal
}

func NewScriptProcessService(runStore RunStore, gateway *toolgateway.Gateway) *ScriptProcessService {
	return &ScriptProcessService{runs: NewRunService(runStore), gateway: gateway}
}

func (s *ScriptProcessService) Create(ctx context.Context, request CreateScriptProcessRunRequest) (toolgateway.ScriptRunCreateResult, error) {
	if s == nil || s.runs == nil || s.gateway == nil {
		return toolgateway.ScriptRunCreateResult{}, errors.New("script process service dependencies are required")
	}
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	request.RequestedBy = strings.TrimSpace(request.RequestedBy)
	if request.OperationKey == "" {
		return toolgateway.ScriptRunCreateResult{}, errors.New("script process idempotency key is required")
	}
	if request.RequestedBy == "" {
		request.RequestedBy = "script_process_service"
	}
	prepared, err := s.runs.prepare(ctx, request.Run)
	if err != nil {
		return toolgateway.ScriptRunCreateResult{}, err
	}
	return s.gateway.CreateScriptProcessRun(ctx, toolgateway.ScriptRunCreateRequest{
		OperationKey: request.OperationKey, Mission: prepared.Mission, Run: prepared.Run,
		Mode: prepared.Mode,
		Session: toolgateway.ScriptRunSession{
			ID: prepared.Session.ID, WorkspaceID: prepared.Session.WorkspaceID, Title: prepared.Session.Title,
			Route: prepared.Session.Route, Status: prepared.Session.Status,
			CreatedAt: prepared.Session.CreatedAt, UpdatedAt: prepared.Session.UpdatedAt,
		},
		CreateSession: prepared.CreateSession, InitialEvents: prepared.InitialEvents,
		Call: toolgateway.ToolCall{
			Name: toolgateway.ScriptProcessTool, RunID: prepared.Run.ID, SessionID: prepared.Session.ID,
			WorkspaceID: prepared.Mission.WorkspaceID, RequestedBy: request.RequestedBy,
		},
		Proposal: request.Process,
	})
}
