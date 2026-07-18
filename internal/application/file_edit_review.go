package application

import (
	"context"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
)

const FileEditReviewProtocolVersion = "file_edit_review.v1"

type FileEditReviewAction string

const (
	FileEditApproveIntent FileEditReviewAction = "approve_intent"
	FileEditDeny          FileEditReviewAction = "deny"
)

type FileEditReviewStore interface {
	fileedit.Store
	GetRun(context.Context, string) (domain.Run, error)
	GetMission(context.Context, string) (domain.Mission, error)
	GetApprovalByProposal(context.Context, string) (approval.Record, error)
	DecideApproval(context.Context, approval.DecisionRequest) (approval.DecisionResult, error)
}

type FileEditReviewService struct {
	store   FileEditReviewStore
	manager *fileedit.Manager
}

type ReviewFileEditRequest struct {
	Version string
	RunID   string
	EditID  string
	Action  FileEditReviewAction
}

type ReviewFileEditResult struct {
	Edit     fileedit.Edit
	Action   FileEditReviewAction
	Replayed bool
}

func NewFileEditReviewService(store FileEditReviewStore) *FileEditReviewService {
	return &FileEditReviewService{store: store, manager: fileedit.NewManager(store)}
}

func (s *FileEditReviewService) Review(ctx context.Context,
	request ReviewFileEditRequest,
) (ReviewFileEditResult, error) {
	if s == nil || s.store == nil || s.manager == nil {
		return ReviewFileEditResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "file edit review dependencies are required")
	}
	if request.Version != FileEditReviewProtocolVersion ||
		!validControlIdentity(request.RunID) || !validControlIdentity(request.EditID) ||
		(request.Action != FileEditApproveIntent && request.Action != FileEditDeny) {
		return ReviewFileEditResult{}, apperror.New(
			apperror.CodeInvalidArgument, "file edit review request is invalid")
	}
	run, err := s.store.GetRun(ctx, request.RunID)
	if err != nil {
		return ReviewFileEditResult{}, apperror.Normalize(err)
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return ReviewFileEditResult{}, apperror.Normalize(err)
	}
	edit, err := s.store.GetFileEdit(ctx, request.EditID)
	if err != nil {
		return ReviewFileEditResult{}, apperror.Normalize(err)
	}
	if run.SessionID == "" || edit.SessionID != run.SessionID ||
		mission.WorkspaceID == "" || edit.WorkspaceID != mission.WorkspaceID {
		return ReviewFileEditResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"file edit does not belong to the requested Run")
	}
	expected := fileedit.StatusDenied
	if request.Action == FileEditApproveIntent {
		expected = fileedit.StatusApproved
	}
	if edit.Status != fileedit.StatusProposed && edit.Status != expected {
		return ReviewFileEditResult{}, apperror.New(apperror.CodeConflict,
			"file edit was already decided with a different outcome")
	}
	replayed := edit.Status == expected
	var reviewed fileedit.Edit
	if edit.Status == fileedit.StatusProposed {
		record, approvalErr := s.store.GetApprovalByProposal(ctx, edit.ID)
		if approvalErr != nil {
			return ReviewFileEditResult{}, apperror.Normalize(approvalErr)
		}
		if record.RunID != run.ID || record.SessionID != run.SessionID ||
			record.WorkspaceID != mission.WorkspaceID || record.ProposalID != edit.ID ||
			record.ToolName != "replace_file" || record.ActionClass != "workspace_write" {
			return ReviewFileEditResult{}, apperror.New(apperror.CodeFailedPrecondition,
				"file edit approval binding is invalid")
		}
		approvalAction := approval.ActionDeny
		expectedApproval := approval.StatusDenied
		reason := "denied by operator"
		if request.Action == FileEditApproveIntent {
			approvalAction = approval.ActionApprove
			expectedApproval = approval.StatusApproved
			reason = ""
		}
		switch record.Status {
		case approval.StatusPending:
			if run.Terminal() {
				return ReviewFileEditResult{}, apperror.New(apperror.CodeFailedPrecondition,
					"terminal Run file edits cannot be changed")
			}
			decision, decisionErr := s.store.DecideApproval(ctx, approval.DecisionRequest{
				ProposalID: edit.ID,
				IdempotencyKey: approval.ReviewIdempotencyKey("replace_file", edit.ID,
					approvalAction),
				Action: approvalAction, Reason: reason, ReviewedBy: "file_edit_review",
			})
			if decisionErr != nil {
				return ReviewFileEditResult{}, apperror.Normalize(decisionErr)
			}
			if decision.Approval.ProposalID != edit.ID ||
				decision.Approval.Status != expectedApproval ||
				decision.Approval.RunID != run.ID {
				return ReviewFileEditResult{}, apperror.New(apperror.CodeInternal,
					"file edit approval decision violated its exact binding")
			}
			replayed = decision.Replayed
		case expectedApproval:
			// The approval transaction committed before the edit-state transaction.
			// Finish the same decision without asking the operator a second time.
			replayed = true
		default:
			return ReviewFileEditResult{}, apperror.New(apperror.CodeConflict,
				"file edit approval was already decided with a different outcome")
		}
	}
	if request.Action == FileEditApproveIntent {
		reviewed, err = s.manager.ApproveIntent(ctx, edit.ID)
	} else {
		reviewed, err = s.manager.Deny(ctx, edit.ID, "denied by operator")
	}
	if err != nil {
		return ReviewFileEditResult{}, apperror.Normalize(err)
	}
	if reviewed.ID != edit.ID || reviewed.SessionID != run.SessionID ||
		reviewed.WorkspaceID != mission.WorkspaceID || reviewed.Status != expected ||
		strings.TrimSpace(reviewed.Path) == "" {
		return ReviewFileEditResult{}, apperror.New(apperror.CodeInternal,
			"file edit review result violated its exact binding")
	}
	return ReviewFileEditResult{Edit: reviewed, Action: request.Action,
		Replayed: replayed}, nil
}

func validControlIdentity(value string) bool {
	return value == strings.TrimSpace(value) && domain.ValidAgentID(value) &&
		!strings.ContainsRune(value, 0)
}
