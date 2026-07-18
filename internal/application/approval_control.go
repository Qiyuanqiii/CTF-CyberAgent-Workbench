package application

import (
	"context"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/scriptprocess"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/toolrun"
	"cyberagent-workbench/internal/tools"
)

const (
	ApprovalControlProtocolVersion = "approval_control.v1"
	ApprovalQueueProtocolVersion   = "approval_queue.v1"
	MaxApprovalQueueItems          = 100
)

type ApprovalControlAction string

const (
	ApprovalControlApproveOnce ApprovalControlAction = "approve_once"
	ApprovalControlDeny        ApprovalControlAction = "deny"
)

type ApprovalControlStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetApproval(context.Context, string) (approval.Record, error)
	ListApprovals(context.Context, approval.ListFilter) ([]approval.Record, error)
	GetToolRun(context.Context, string) (toolrun.ToolRun, error)
	GetScriptProcess(context.Context, string) (scriptprocess.Process, error)
}

type ApprovalReviewer interface {
	Review(context.Context, toolgateway.ReviewRequest) (toolgateway.Outcome, error)
}

type ApprovalControlService struct {
	store    ApprovalControlStore
	reviewer ApprovalReviewer
	checker  policy.Checker
}

type DecideApprovalControlRequest struct {
	Version      string
	RunID        string
	ApprovalID   string
	Action       ApprovalControlAction
	OperationKey string
	ReviewedBy   string
	Reason       string
}

type DecideApprovalControlResult struct {
	Approval approval.Record
	Action   ApprovalControlAction
	Replayed bool
}

func NewApprovalControlService(store ApprovalControlStore, reviewer ApprovalReviewer,
	checker policy.Checker,
) *ApprovalControlService {
	return &ApprovalControlService{store: store, reviewer: reviewer, checker: checker}
}

func (s *ApprovalControlService) Decide(ctx context.Context,
	request DecideApprovalControlRequest,
) (DecideApprovalControlResult, error) {
	if s == nil || s.store == nil || s.reviewer == nil || s.checker == nil {
		return DecideApprovalControlResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "approval control dependencies are required")
	}
	if err := normalizeApprovalControlRequest(&request); err != nil {
		return DecideApprovalControlResult{}, err
	}
	run, err := s.store.GetRun(ctx, request.RunID)
	if err != nil {
		return DecideApprovalControlResult{}, apperror.Normalize(err)
	}
	record, err := s.store.GetApproval(ctx, request.ApprovalID)
	if err != nil {
		return DecideApprovalControlResult{}, apperror.Normalize(err)
	}
	if record.RunID != run.ID || record.GrantID != "" {
		return DecideApprovalControlResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"approval is not an ungranted request for the requested Run")
	}
	if record.Mode == string(toolgateway.ApprovalNever) {
		return DecideApprovalControlResult{}, apperror.New(
			apperror.CodePolicyDenied, "permanent Policy denial cannot be overridden")
	}
	expected := approval.StatusDenied
	reviewAction := toolgateway.ReviewDeny
	if request.Action == ApprovalControlApproveOnce {
		expected = approval.StatusApproved
		reviewAction = toolgateway.ReviewApprove
	}
	if record.Status != approval.StatusPending && record.Status != expected {
		return DecideApprovalControlResult{}, apperror.New(apperror.CodeConflict,
			"approval was already decided with a different outcome")
	}
	if record.Status == approval.StatusPending && run.Terminal() {
		return DecideApprovalControlResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "terminal Run approvals cannot be changed")
	}
	if request.Action == ApprovalControlApproveOnce && record.Status == approval.StatusPending {
		if err := s.recheckApprovalSource(ctx, record); err != nil {
			return DecideApprovalControlResult{}, err
		}
	}
	if !approvalActionSupported(record, request.Action) {
		return DecideApprovalControlResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"approval action is not available through this control surface")
	}
	replayed := record.Status == expected
	_, err = s.reviewer.Review(ctx, toolgateway.ReviewRequest{
		Action: reviewAction, Tool: toolgateway.ToolName(record.ToolName),
		ProposalID: record.ProposalID, IdempotencyKey: request.OperationKey,
		ReviewedBy: request.ReviewedBy, Reason: request.Reason,
	})
	if err != nil {
		return DecideApprovalControlResult{}, apperror.Normalize(err)
	}
	stored, err := s.store.GetApproval(ctx, record.ID)
	if err != nil {
		return DecideApprovalControlResult{}, apperror.Normalize(err)
	}
	if stored.RunID != run.ID || stored.ProposalID != record.ProposalID ||
		stored.ToolName != record.ToolName || stored.ActionClass != record.ActionClass ||
		stored.Status != expected || stored.GrantID != "" {
		return DecideApprovalControlResult{}, apperror.New(apperror.CodeInternal,
			"approval decision result violated its exact binding")
	}
	return DecideApprovalControlResult{Approval: stored, Action: request.Action,
		Replayed: replayed}, nil
}

func (s *ApprovalControlService) recheckApprovalSource(ctx context.Context,
	record approval.Record,
) error {
	var decision policy.Decision
	switch record.ToolName {
	case string(toolgateway.ShellTool):
		proposal, err := s.store.GetToolRun(ctx, record.ProposalID)
		if err != nil {
			return apperror.Normalize(err)
		}
		if proposal.ID != record.ProposalID || proposal.SessionID != record.SessionID ||
			proposal.WorkspaceID != record.WorkspaceID || proposal.Status != toolrun.StatusProposed {
			return apperror.New(apperror.CodeFailedPrecondition,
				"Shell approval source changed or is no longer pending")
		}
		decision = s.checker.CheckToolCall(tools.Call{Name: string(toolgateway.ShellTool),
			Args: map[string]string{"command": proposal.Command}})
	case string(toolgateway.ScriptProcessTool):
		proposal, err := s.store.GetScriptProcess(ctx, record.ProposalID)
		if err != nil {
			return apperror.Normalize(err)
		}
		if proposal.ID != record.ProposalID || proposal.RunID != record.RunID ||
			proposal.SessionID != record.SessionID || proposal.WorkspaceID != record.WorkspaceID ||
			proposal.Status != scriptprocess.StatusProposed ||
			proposal.ExecutionMode != scriptprocess.ExecutionDisabled {
			return apperror.New(apperror.CodeFailedPrecondition,
				"ScriptProcess approval source changed or is not process-disabled")
		}
		payload, err := scriptprocess.EncodeProposal(proposal.Proposal())
		if err != nil {
			return apperror.Wrap(apperror.CodeFailedPrecondition,
				"ScriptProcess approval source is invalid", err)
		}
		decision = s.checker.CheckToolCall(tools.Call{Name: string(toolgateway.ScriptProcessTool),
			Args: map[string]string{"proposal": payload}})
	default:
		return apperror.New(apperror.CodeFailedPrecondition,
			"approve-once is limited to dry-run Shell and ScriptProcess proposals")
	}
	if !decision.Allowed {
		return apperror.New(apperror.CodePolicyDenied,
			"approval source is permanently denied by current Policy")
	}
	return nil
}

func ApprovalDecisionActions(record approval.Record, runTerminal bool) []ApprovalControlAction {
	if runTerminal || record.Status != approval.StatusPending || record.GrantID != "" ||
		record.Mode == string(toolgateway.ApprovalNever) {
		return []ApprovalControlAction{}
	}
	switch record.ToolName {
	case string(toolgateway.ShellTool), string(toolgateway.ScriptProcessTool):
		return []ApprovalControlAction{ApprovalControlApproveOnce, ApprovalControlDeny}
	case string(toolgateway.ReplaceFileTool):
		return []ApprovalControlAction{ApprovalControlDeny}
	default:
		return []ApprovalControlAction{}
	}
}

func approvalActionSupported(record approval.Record, action ApprovalControlAction) bool {
	for _, supported := range ApprovalDecisionActions(record, false) {
		if supported == action {
			return true
		}
	}
	// Idempotent replay is still constrained to the same tool classes.
	if record.Status != approval.StatusPending {
		switch record.ToolName {
		case string(toolgateway.ShellTool), string(toolgateway.ScriptProcessTool):
			return action == ApprovalControlApproveOnce || action == ApprovalControlDeny
		case string(toolgateway.ReplaceFileTool):
			return action == ApprovalControlDeny
		}
	}
	return false
}

func normalizeApprovalControlRequest(request *DecideApprovalControlRequest) error {
	if request == nil {
		return apperror.New(apperror.CodeInvalidArgument,
			"approval control request is required")
	}
	if request.Version != ApprovalControlProtocolVersion {
		return apperror.New(apperror.CodeInvalidArgument,
			"unsupported approval control version")
	}
	for label, value := range map[string]string{
		"Run id": request.RunID, "approval id": request.ApprovalID,
		"reviewer": request.ReviewedBy,
	} {
		if value != strings.TrimSpace(value) || !domain.ValidAgentID(value) ||
			strings.ContainsRune(value, 0) {
			return apperror.New(apperror.CodeInvalidArgument,
				"approval control "+label+" is invalid")
		}
	}
	operationKey, err := domain.NormalizeAgentOperationKey(request.OperationKey)
	if err != nil || operationKey != request.OperationKey || containsSpaceOrControl(operationKey) {
		return apperror.New(apperror.CodeInvalidArgument,
			"approval control idempotency key is invalid")
	}
	if request.Action != ApprovalControlApproveOnce && request.Action != ApprovalControlDeny {
		return apperror.New(apperror.CodeInvalidArgument,
			"approval control action must be approve_once or deny")
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Action == ApprovalControlApproveOnce && request.Reason != "" {
		return apperror.New(apperror.CodeInvalidArgument,
			"approve-once cannot include a denial reason")
	}
	if request.Action == ApprovalControlDeny && request.Reason == "" {
		request.Reason = "denied by operator"
	}
	if _, err := (approval.DecisionRequest{ProposalID: request.ApprovalID,
		IdempotencyKey: operationKey, Action: approval.ActionDeny,
		Reason: request.Reason, ReviewedBy: request.ReviewedBy}).Normalize(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"approval control request is invalid", err)
	}
	request.OperationKey = operationKey
	return nil
}
