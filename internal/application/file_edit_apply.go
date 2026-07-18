package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/tools"
)

type FileEditApplyStore interface {
	fileedit.Store
	GetRun(context.Context, string) (domain.Run, error)
	GetMission(context.Context, string) (domain.Mission, error)
	GetSession(context.Context, string) (session.Session, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	GetApprovalByProposal(context.Context, string) (approval.Record, error)
	RecordPolicyDecision(context.Context, policy.DecisionRecord) error
	GetFileEditApplyOperation(context.Context, string) (
		fileedit.ApplyOperation, *fileedit.ApplyResult, bool, error)
	PrepareFileEditApply(context.Context, fileedit.ApplyOperation) (
		fileedit.ApplyOperation, *fileedit.ApplyResult, bool, error)
	CompleteFileEditApply(context.Context, fileedit.ApplyResult) (
		fileedit.ApplyResult, bool, error)
}

type FileEditApplyService struct {
	store   FileEditApplyStore
	manager *fileedit.Manager
	checker policy.Checker
	now     func() time.Time
}

type ApplyFileEditRequest struct {
	Version      string
	RunID        string
	EditID       string
	OperationKey string
	AppliedBy    string
}

type ApplyFileEditResult struct {
	Operation      fileedit.ApplyOperation
	Result         fileedit.ApplyResult
	Edit           fileedit.Edit
	StagingCleanup fileedit.StagingCleanupResult
	Replayed       bool
	FileWritten    bool
}

func NewFileEditApplyService(store FileEditApplyStore,
	checker policy.Checker,
) *FileEditApplyService {
	return &FileEditApplyService{store: store, manager: fileedit.NewManager(store),
		checker: checker, now: func() time.Time { return time.Now().UTC() }}
}

func (s *FileEditApplyService) Apply(ctx context.Context,
	request ApplyFileEditRequest,
) (ApplyFileEditResult, error) {
	if s == nil || s.store == nil || s.manager == nil || s.checker == nil || s.now == nil {
		return ApplyFileEditResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"FileEdit apply dependencies are required")
	}
	normalized, err := normalizeFileEditApplyRequest(request)
	if err != nil {
		return ApplyFileEditResult{}, err
	}
	keyDigest := runmutation.FileEditApplyOperationDigest(normalized.RunID,
		normalized.EditID, normalized.OperationKey)
	fingerprint := runmutation.FileEditApplyRequestFingerprint(normalized.RunID,
		normalized.EditID, normalized.AppliedBy)
	operation, storedResult, found, err := s.store.GetFileEditApplyOperation(ctx,
		keyDigest)
	if err != nil {
		return ApplyFileEditResult{}, apperror.Normalize(err)
	}
	preparedReplay := found
	if found {
		if operation.ProtocolVersion != fileedit.FileEditApplyProtocolVersion ||
			operation.KeyDigest != keyDigest || operation.RequestFingerprint != fingerprint ||
			operation.RunID != normalized.RunID || operation.EditID != normalized.EditID ||
			operation.AppliedBy != normalized.AppliedBy {
			return ApplyFileEditResult{}, apperror.New(apperror.CodeConflict,
				"FileEdit apply operation key was used for different intent")
		}
		edit, lookupErr := s.store.GetFileEdit(ctx, operation.EditID)
		if lookupErr != nil {
			return ApplyFileEditResult{}, apperror.Normalize(lookupErr)
		}
		if storedResult != nil {
			cleanup := s.cleanupStaging(ctx, operation)
			return ApplyFileEditResult{Operation: operation, Result: *storedResult,
				Edit: edit, StagingCleanup: cleanup, Replayed: true}, nil
		}
	} else {
		binding, bindingErr := s.loadBinding(ctx, normalized.RunID, normalized.EditID,
			true)
		if bindingErr != nil {
			return ApplyFileEditResult{}, bindingErr
		}
		if policyErr := s.checkCurrentPolicy(ctx, binding); policyErr != nil {
			return ApplyFileEditResult{}, policyErr
		}
		observedHash, hashErr := fileedit.CurrentHash(binding.workspace.RootPath,
			binding.edit.Path)
		if hashErr != nil {
			return ApplyFileEditResult{}, apperror.Normalize(hashErr)
		}
		if observedHash != binding.edit.OriginalHash &&
			observedHash != binding.edit.ProposedHash {
			return ApplyFileEditResult{}, apperror.New(apperror.CodeConflict,
				"workspace file changed after review; refusing to apply")
		}
		operation = fileedit.ApplyOperation{
			ProtocolVersion: fileedit.FileEditApplyProtocolVersion,
			KeyDigest:       keyDigest, RequestFingerprint: fingerprint,
			RunID: binding.run.ID, SessionID: binding.run.SessionID,
			WorkspaceID: binding.edit.WorkspaceID, EditID: binding.edit.ID,
			Path: binding.edit.Path, OriginalHash: binding.edit.OriginalHash,
			ProposedHash: binding.edit.ProposedHash, ObservedHash: observedHash,
			AppliedBy: normalized.AppliedBy, CreatedAt: s.now().UTC(),
		}
		operation, storedResult, preparedReplay, err = s.store.PrepareFileEditApply(ctx,
			operation)
		if err != nil {
			return ApplyFileEditResult{}, apperror.Normalize(err)
		}
		if storedResult != nil {
			edit, lookupErr := s.store.GetFileEdit(ctx, operation.EditID)
			cleanup := fileedit.StagingCleanupResult{}
			if lookupErr == nil {
				cleanup = s.cleanupStaging(ctx, operation)
			}
			return ApplyFileEditResult{Operation: operation, Result: *storedResult,
				Edit: edit, StagingCleanup: cleanup, Replayed: true}, apperror.Normalize(lookupErr)
		}
	}

	binding, err := s.loadOperationBinding(ctx, operation)
	if err != nil {
		return ApplyFileEditResult{}, err
	}
	currentHash, err := fileedit.CurrentHash(binding.workspace.RootPath, operation.Path)
	if err != nil {
		return ApplyFileEditResult{}, apperror.Normalize(err)
	}
	if currentHash != operation.OriginalHash && currentHash != operation.ProposedHash {
		return ApplyFileEditResult{}, apperror.New(apperror.CodeConflict,
			"workspace file no longer matches the prepared FileEdit apply operation")
	}
	fileWritten := binding.edit.Status == fileedit.StatusApproved &&
		currentHash == operation.OriginalHash
	if fileWritten {
		if binding.run.Status != domain.RunRunning ||
			binding.session.Status != session.StatusActive {
			return ApplyFileEditResult{}, apperror.New(apperror.CodeFailedPrecondition,
				"FileEdit apply recovery requires a running Run and active Session before writing")
		}
		// Preparation is durable, but execution authority is not. Recheck Policy at
		// the final write boundary so a restart cannot revive stale permission.
		if policyErr := s.checkCurrentPolicy(ctx, binding); policyErr != nil {
			return ApplyFileEditResult{}, policyErr
		}
	}
	applied := binding.edit
	var applyErr error
	switch binding.edit.Status {
	case fileedit.StatusApproved:
		applied, applyErr = s.manager.Approve(ctx, binding.edit.ID,
			binding.workspace.RootPath)
	case fileedit.StatusApplied, fileedit.StatusFailed:
		// Recover the durable result after a process interruption.
	default:
		return ApplyFileEditResult{}, apperror.New(apperror.CodeConflict,
			"FileEdit is not in an applicable state")
	}
	status := fileedit.ApplyCompleted
	reasonCode := ""
	if applyErr != nil || applied.Status == fileedit.StatusFailed {
		status = fileedit.ApplyFailed
		reasonCode = "file_edit_apply_failed"
	}
	if status == fileedit.ApplyCompleted {
		writtenHash, hashErr := fileedit.CurrentHash(binding.workspace.RootPath,
			operation.Path)
		if hashErr != nil || writtenHash != operation.ProposedHash ||
			applied.Status != fileedit.StatusApplied {
			if hashErr == nil {
				hashErr = errors.New("applied FileEdit failed final hash verification")
			}
			return ApplyFileEditResult{}, apperror.Normalize(hashErr)
		}
	}
	result, completionReplay, completionErr := s.store.CompleteFileEditApply(ctx,
		fileedit.ApplyResult{OperationKeyDigest: operation.KeyDigest, Status: status,
			ReasonCode: reasonCode, CompletedAt: s.now().UTC()})
	if completionErr != nil {
		if applyErr != nil {
			return ApplyFileEditResult{}, apperror.Normalize(errors.Join(applyErr,
				completionErr))
		}
		return ApplyFileEditResult{}, apperror.Normalize(completionErr)
	}
	cleanup := s.cleanupStaging(ctx, operation)
	value := ApplyFileEditResult{Operation: operation, Result: result, Edit: applied,
		StagingCleanup: cleanup, Replayed: preparedReplay || completionReplay,
		FileWritten: fileWritten}
	if applyErr != nil {
		return value, apperror.Normalize(applyErr)
	}
	return value, nil
}

func (s *FileEditApplyService) cleanupStaging(ctx context.Context,
	operation fileedit.ApplyOperation,
) fileedit.StagingCleanupResult {
	binding, err := s.loadOperationBinding(ctx, operation)
	if err != nil {
		return fileedit.StagingCleanupResult{Pending: true}
	}
	result, err := fileedit.CleanupStaleStaging(binding.workspace.RootPath,
		operation.Path, operation.ProposedHash, s.now().UTC())
	if err != nil {
		return fileedit.StagingCleanupResult{Pending: true}
	}
	return result
}

type fileEditApplyBinding struct {
	run       domain.Run
	mission   domain.Mission
	session   session.Session
	workspace session.WorkspaceInfo
	edit      fileedit.Edit
	approval  approval.Record
}

func (s *FileEditApplyService) loadBinding(ctx context.Context, runID string,
	editID string, requireRunning bool,
) (fileEditApplyBinding, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fileEditApplyBinding{}, apperror.Normalize(err)
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return fileEditApplyBinding{}, apperror.Normalize(err)
	}
	linkedSession, err := s.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return fileEditApplyBinding{}, apperror.Normalize(err)
	}
	edit, err := s.store.GetFileEdit(ctx, editID)
	if err != nil {
		return fileEditApplyBinding{}, apperror.Normalize(err)
	}
	workspace, err := s.store.GetWorkspaceInfo(ctx, mission.WorkspaceID)
	if err != nil {
		return fileEditApplyBinding{}, apperror.Normalize(err)
	}
	record, err := s.store.GetApprovalByProposal(ctx, edit.ID)
	if err != nil {
		return fileEditApplyBinding{}, apperror.Normalize(err)
	}
	if run.SessionID == "" || mission.WorkspaceID == "" ||
		linkedSession.ID != run.SessionID ||
		linkedSession.WorkspaceID != mission.WorkspaceID ||
		edit.SessionID != run.SessionID || edit.WorkspaceID != mission.WorkspaceID ||
		workspace.ID != mission.WorkspaceID || record.RunID != run.ID ||
		record.SessionID != run.SessionID || record.WorkspaceID != mission.WorkspaceID ||
		record.ProposalID != edit.ID || record.ToolName != "replace_file" ||
		record.ActionClass != "workspace_write" || record.Status != approval.StatusApproved {
		return fileEditApplyBinding{}, apperror.New(apperror.CodeFailedPrecondition,
			"FileEdit apply binding or approval is invalid")
	}
	if requireRunning && (run.Status != domain.RunRunning ||
		linkedSession.Status != session.StatusActive ||
		edit.Status != fileedit.StatusApproved) {
		return fileEditApplyBinding{}, apperror.New(apperror.CodeFailedPrecondition,
			"new FileEdit apply requires a running Run, active Session, and approved edit")
	}
	return fileEditApplyBinding{run: run, mission: mission, session: linkedSession,
		workspace: workspace, edit: edit, approval: record}, nil
}

func (s *FileEditApplyService) checkCurrentPolicy(ctx context.Context,
	binding fileEditApplyBinding,
) error {
	decision := s.checker.CheckToolCall(tools.Call{Name: "replace_file",
		Args: map[string]string{
			"run_id": binding.run.ID, "workspace_id": binding.edit.WorkspaceID,
			"path": binding.edit.Path, "original_hash": binding.edit.OriginalHash,
			"proposed_hash": binding.edit.ProposedHash,
		}})
	if err := s.store.RecordPolicyDecision(ctx, policy.DecisionRecord{
		SessionID: binding.run.SessionID, SubjectID: binding.edit.ID,
		Context: "file_edit_apply", Decision: decision,
	}); err != nil {
		return apperror.Normalize(err)
	}
	if !decision.Allowed {
		return apperror.New(apperror.CodePolicyDenied,
			"FileEdit apply was denied by current Policy")
	}
	return nil
}

func (s *FileEditApplyService) loadOperationBinding(ctx context.Context,
	operation fileedit.ApplyOperation,
) (fileEditApplyBinding, error) {
	binding, err := s.loadBinding(ctx, operation.RunID, operation.EditID, false)
	if err != nil {
		return fileEditApplyBinding{}, err
	}
	if binding.run.SessionID != operation.SessionID ||
		binding.edit.WorkspaceID != operation.WorkspaceID ||
		binding.edit.Path != operation.Path ||
		binding.edit.OriginalHash != operation.OriginalHash ||
		binding.edit.ProposedHash != operation.ProposedHash {
		return fileEditApplyBinding{}, apperror.New(apperror.CodeConflict,
			"prepared FileEdit apply binding changed")
	}
	return binding, nil
}

func normalizeFileEditApplyRequest(request ApplyFileEditRequest) (
	ApplyFileEditRequest, error,
) {
	if request.Version != fileedit.FileEditApplyProtocolVersion ||
		!validControlIdentity(request.RunID) || !validControlIdentity(request.EditID) ||
		!validControlIdentity(request.AppliedBy) {
		return ApplyFileEditRequest{}, apperror.New(apperror.CodeInvalidArgument,
			"FileEdit apply request is invalid")
	}
	key, err := domain.NormalizeAgentOperationKey(request.OperationKey)
	if err != nil || key != request.OperationKey || containsSpaceOrControl(key) {
		return ApplyFileEditRequest{}, apperror.New(apperror.CodeInvalidArgument,
			"FileEdit apply idempotency key is invalid")
	}
	request.OperationKey = key
	request.AppliedBy = strings.TrimSpace(request.AppliedBy)
	return request, nil
}
