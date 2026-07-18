package application

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/runmutation"
)

type RunLifecycleControlStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetRunLifecycleOperation(context.Context, string) (domain.RunLifecycleOperation, bool, error)
	TransitionRunWithLifecycleOperation(context.Context, domain.RunLifecycleOperation) (
		domain.RunLifecycleOperation, domain.Run, bool, error)
}

type RunLifecycleControlService struct {
	store RunLifecycleControlStore
}

type ControlRunLifecycleRequest struct {
	Version      string
	RunID        string
	Action       domain.RunLifecycleAction
	OperationKey string
	RequestedBy  string
}

type ControlRunLifecycleResult struct {
	Operation domain.RunLifecycleOperation
	Run       domain.Run
	Replayed  bool
}

func NewRunLifecycleControlService(
	store RunLifecycleControlStore,
) *RunLifecycleControlService {
	return &RunLifecycleControlService{store: store}
}

func (s *RunLifecycleControlService) Apply(ctx context.Context,
	request ControlRunLifecycleRequest,
) (ControlRunLifecycleResult, error) {
	if s == nil || s.store == nil {
		return ControlRunLifecycleResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Run lifecycle control store is required")
	}
	normalized, expected, applied, err := normalizeRunLifecycleControlRequest(request)
	if err != nil {
		return ControlRunLifecycleResult{}, err
	}
	keyDigest := runmutation.RunLifecycleOperationDigest(normalized.RunID,
		normalized.OperationKey)
	requestFingerprint := runmutation.RunLifecycleRequestFingerprint(normalized.RunID,
		string(normalized.Action), string(expected), normalized.RequestedBy)
	if replay, found, err := s.loadReplay(ctx, keyDigest, requestFingerprint,
		normalized, expected, applied); err != nil || found {
		return replay, err
	}
	if _, err := s.store.GetRun(ctx, normalized.RunID); err != nil {
		return ControlRunLifecycleResult{}, apperror.Normalize(err)
	}
	operation := domain.RunLifecycleOperation{
		ProtocolVersion: domain.RunLifecycleControlProtocolVersion,
		KeyDigest:       keyDigest, RequestFingerprint: requestFingerprint,
		RunID: normalized.RunID, Action: normalized.Action,
		ExpectedStatus: expected, AppliedStatus: applied,
		RequestedBy: normalized.RequestedBy, CreatedAt: time.Now().UTC(),
	}
	stored, run, replayed, err := s.store.TransitionRunWithLifecycleOperation(ctx,
		operation)
	return ControlRunLifecycleResult{Operation: stored, Run: run, Replayed: replayed},
		apperror.Normalize(err)
}

func (s *RunLifecycleControlService) loadReplay(ctx context.Context,
	keyDigest string, requestFingerprint string, request ControlRunLifecycleRequest,
	expected domain.RunStatus, applied domain.RunStatus,
) (ControlRunLifecycleResult, bool, error) {
	existing, found, err := s.store.GetRunLifecycleOperation(ctx, keyDigest)
	if err != nil || !found {
		return ControlRunLifecycleResult{}, found, apperror.Normalize(err)
	}
	if existing.ProtocolVersion != domain.RunLifecycleControlProtocolVersion ||
		existing.RequestFingerprint != requestFingerprint ||
		existing.RunID != request.RunID || existing.Action != request.Action ||
		existing.ExpectedStatus != expected || existing.AppliedStatus != applied ||
		existing.RequestedBy != request.RequestedBy {
		return ControlRunLifecycleResult{}, true, apperror.New(apperror.CodeConflict,
			"Run lifecycle operation key was already used for different intent")
	}
	run, err := s.store.GetRun(ctx, existing.RunID)
	if err != nil {
		return ControlRunLifecycleResult{}, true, apperror.Normalize(err)
	}
	return ControlRunLifecycleResult{Operation: existing, Run: run, Replayed: true},
		true, nil
}

func normalizeRunLifecycleControlRequest(request ControlRunLifecycleRequest) (
	ControlRunLifecycleRequest, domain.RunStatus, domain.RunStatus, error,
) {
	if request.Version != domain.RunLifecycleControlProtocolVersion {
		return ControlRunLifecycleRequest{}, "", "", apperror.New(
			apperror.CodeInvalidArgument, "unsupported Run lifecycle control version")
	}
	if request.RunID != strings.TrimSpace(request.RunID) ||
		!domain.ValidAgentID(request.RunID) || strings.ContainsRune(request.RunID, 0) {
		return ControlRunLifecycleRequest{}, "", "", apperror.New(
			apperror.CodeInvalidArgument, "Run lifecycle control Run id is invalid")
	}
	operationKey, err := domain.NormalizeAgentOperationKey(request.OperationKey)
	if err != nil || operationKey != request.OperationKey || containsSpaceOrControl(operationKey) {
		return ControlRunLifecycleRequest{}, "", "", apperror.New(
			apperror.CodeInvalidArgument, "Run lifecycle idempotency key is invalid")
	}
	requestedBy := strings.TrimSpace(request.RequestedBy)
	if requestedBy != request.RequestedBy || !domain.ValidAgentID(requestedBy) ||
		strings.ContainsRune(requestedBy, 0) {
		return ControlRunLifecycleRequest{}, "", "", apperror.New(
			apperror.CodeInvalidArgument, "Run lifecycle requester is invalid")
	}
	expected, applied, err := request.Action.Transition()
	if err != nil {
		return ControlRunLifecycleRequest{}, "", "", apperror.Wrap(
			apperror.CodeInvalidArgument, "Run lifecycle action is invalid", err)
	}
	request.OperationKey = operationKey
	request.RequestedBy = requestedBy
	return request, expected, applied, nil
}
