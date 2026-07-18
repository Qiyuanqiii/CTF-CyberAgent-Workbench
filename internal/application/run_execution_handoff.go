package application

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/runmutation"
)

type RunExecutionHandoffStore interface {
	SessionRunStore
	GetRunExecutionHandoff(context.Context, string) (domain.RunExecutionHandoff, bool, error)
	PrepareRunExecutionHandoff(context.Context, domain.RunExecutionHandoffOperation) (
		domain.RunExecutionHandoff, bool, error)
	CompleteRunExecutionHandoff(context.Context, string, domain.RunExecutionLease,
		domain.RunExecutionHandoffStatus, string, string, int, bool, bool) (
		domain.RunExecutionHandoffResult, bool, error)
}

type RunExecutionHandoffService struct {
	store      RunExecutionHandoffStore
	supervisor *RunSupervisor
}

type ExecuteRunHandoffRequest struct {
	Version      string
	RunID        string
	MaxSteps     int
	OperationKey string
	RequestedBy  string
}

type ExecuteRunHandoffResult struct {
	Handoff   domain.RunExecutionHandoff
	Execution ExecutionResult
	Replayed  bool
}

func NewRunExecutionHandoffService(store RunExecutionHandoffStore,
	router *llm.Router, checker policy.Checker,
) *RunExecutionHandoffService {
	return &RunExecutionHandoffService{
		store: store, supervisor: NewRunSupervisor(store, router, checker),
	}
}

func (s *RunExecutionHandoffService) WithActiveCalls(
	registry *ActiveCallRegistry,
) *RunExecutionHandoffService {
	if s != nil && s.supervisor != nil {
		s.supervisor.WithActiveCalls(registry)
	}
	return s
}

func (s *RunExecutionHandoffService) Execute(ctx context.Context,
	request ExecuteRunHandoffRequest,
) (ExecuteRunHandoffResult, error) {
	if s == nil || s.store == nil || s.supervisor == nil {
		return ExecuteRunHandoffResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"Run execution handoff dependencies are required")
	}
	normalized, err := normalizeRunExecutionHandoffRequest(request)
	if err != nil {
		return ExecuteRunHandoffResult{}, err
	}
	keyDigest := runmutation.RunExecutionHandoffOperationDigest(normalized.RunID,
		normalized.OperationKey)
	requestFingerprint := runmutation.RunExecutionHandoffRequestFingerprint(
		normalized.RunID, normalized.RequestedBy, normalized.MaxSteps)
	handoff, found, err := s.store.GetRunExecutionHandoff(ctx, keyDigest)
	if err != nil {
		return ExecuteRunHandoffResult{}, apperror.Normalize(err)
	}
	replayedOperation := found
	if found {
		if err := validateRunExecutionHandoffReplay(handoff, normalized,
			keyDigest, requestFingerprint); err != nil {
			return ExecuteRunHandoffResult{}, err
		}
		if handoff.Result != nil {
			return ExecuteRunHandoffResult{Handoff: handoff,
				Execution: executionResultFromHandoff(handoff), Replayed: true}, nil
		}
	} else {
		run, err := s.store.GetRun(ctx, normalized.RunID)
		if err != nil {
			return ExecuteRunHandoffResult{}, apperror.Normalize(err)
		}
		operation := domain.RunExecutionHandoffOperation{
			ID:              idgen.New("run-handoff"),
			ProtocolVersion: domain.RunExecutionHandoffProtocolVersion,
			KeyDigest:       keyDigest, RequestFingerprint: requestFingerprint,
			RunID: run.ID, SessionID: run.SessionID, RequestedBy: normalized.RequestedBy,
			MaxSteps: normalized.MaxSteps, CreatedAt: time.Now().UTC(),
		}
		handoff, _, err = s.store.PrepareRunExecutionHandoff(ctx, operation)
		if err != nil {
			return ExecuteRunHandoffResult{}, apperror.Normalize(err)
		}
		if handoff.Result != nil {
			return ExecuteRunHandoffResult{Handoff: handoff,
				Execution: executionResultFromHandoff(handoff)}, nil
		}
	}

	execution := ExecutionResult{RunID: handoff.Operation.RunID,
		Steps: make([]LifecycleResult, 0), RunStatus: domain.RunRunning}
	err = s.supervisor.withRunExecutionLease(ctx, handoff.Operation.RunID,
		func(leaseCtx context.Context, lease domain.RunExecutionLease) error {
			return s.executeSelectionWithLease(leaseCtx, lease, &handoff, &execution)
		})
	if err != nil {
		return ExecuteRunHandoffResult{Handoff: handoff, Execution: execution,
			Replayed: replayedOperation}, apperror.Normalize(err)
	}
	stored, storedFound, err := s.store.GetRunExecutionHandoff(ctx, keyDigest)
	if err != nil || !storedFound || stored.Result == nil {
		if err == nil {
			err = apperror.New(apperror.CodeInternal,
				"Run execution handoff completion was not persisted")
		}
		return ExecuteRunHandoffResult{Handoff: handoff, Execution: execution,
			Replayed: replayedOperation}, apperror.Normalize(err)
	}
	return ExecuteRunHandoffResult{Handoff: stored, Execution: execution,
		Replayed: replayedOperation}, nil
}

func (s *RunExecutionHandoffService) executeSelectionWithLease(ctx context.Context,
	lease domain.RunExecutionLease, handoff *domain.RunExecutionHandoff,
	execution *ExecutionResult,
) error {
	var executionErr error
	for _, item := range handoff.Items {
		message, err := s.store.GetOperatorSteering(ctx, item.MessageID)
		if err != nil {
			executionErr = apperror.Normalize(err)
			break
		}
		if message.RunID != handoff.Operation.RunID ||
			message.SessionID != handoff.Operation.SessionID {
			executionErr = apperror.New(apperror.CodeConflict,
				"Run execution handoff message binding changed")
			break
		}
		if message.Status != domain.OperatorSteeringPending {
			continue
		}
		step, err := s.supervisor.stepSteeringMessageWithLease(ctx, lease,
			message.ID)
		if step.Turn > 0 {
			execution.Steps = append(execution.Steps, step)
			execution.RunStatus = step.RunStatus
		}
		if err != nil {
			latest, lookupErr := s.store.GetOperatorSteering(ctx, item.MessageID)
			if lookupErr == nil && latest.Status == domain.OperatorSteeringCancelled &&
				apperror.CodeOf(apperror.Normalize(err)) == apperror.CodeFailedPrecondition {
				continue
			}
			executionErr = apperror.Normalize(err)
			break
		}
		if step.Action.Kind == domain.RootActionFinish {
			execution.StopReason = "root_finish"
			break
		}
		if step.Action.Kind == domain.RootActionWait {
			execution.StopReason = "root_wait"
			break
		}
	}
	run, runErr := s.store.GetRun(ctx, handoff.Operation.RunID)
	if runErr == nil {
		execution.RunStatus = run.Status
	} else if executionErr == nil {
		executionErr = apperror.Normalize(runErr)
	}
	status := domain.RunExecutionHandoffCompleted
	errorCode := ""
	if executionErr != nil {
		status = domain.RunExecutionHandoffFailed
		errorCode = strings.ToLower(string(apperror.CodeOf(executionErr)))
	}
	if execution.StopReason == "" {
		if executionErr != nil {
			execution.StopReason = errorCode
		} else {
			execution.StopReason = "selection_drained"
		}
	}
	completeCtx, cancelComplete := context.WithTimeout(context.WithoutCancel(ctx),
		2*time.Second)
	defer cancelComplete()
	modelCalled := false
	toolCalled := false
	for _, step := range execution.Steps {
		modelCalled = modelCalled || step.ModelAttempts > 0
		toolCalled = toolCalled || step.ToolCalls > 0
	}
	result, _, completeErr := s.store.CompleteRunExecutionHandoff(completeCtx,
		handoff.Operation.ID, lease, status, execution.StopReason, errorCode,
		len(execution.Steps), modelCalled, toolCalled)
	if completeErr != nil {
		return apperror.Normalize(completeErr)
	}
	handoff.Result = &result
	return nil
}

func normalizeRunExecutionHandoffRequest(request ExecuteRunHandoffRequest) (
	ExecuteRunHandoffRequest, error,
) {
	if request.Version != domain.RunExecutionHandoffProtocolVersion {
		return ExecuteRunHandoffRequest{}, apperror.New(apperror.CodeInvalidArgument,
			"unsupported Run execution handoff version")
	}
	if request.RunID != strings.TrimSpace(request.RunID) ||
		!domain.ValidAgentID(request.RunID) || strings.ContainsRune(request.RunID, 0) {
		return ExecuteRunHandoffRequest{}, apperror.New(apperror.CodeInvalidArgument,
			"Run execution handoff Run id is invalid")
	}
	if request.MaxSteps <= 0 || request.MaxSteps > domain.MaxRunExecutionHandoffSteps {
		return ExecuteRunHandoffRequest{}, apperror.New(apperror.CodeInvalidArgument,
			"Run execution handoff step limit is invalid")
	}
	operationKey, err := domain.NormalizeAgentOperationKey(request.OperationKey)
	if err != nil || operationKey != request.OperationKey || containsSpaceOrControl(operationKey) {
		return ExecuteRunHandoffRequest{}, apperror.New(apperror.CodeInvalidArgument,
			"Run execution handoff idempotency key is invalid")
	}
	requestedBy := strings.TrimSpace(request.RequestedBy)
	if requestedBy != request.RequestedBy || !domain.ValidAgentID(requestedBy) ||
		strings.ContainsRune(requestedBy, 0) {
		return ExecuteRunHandoffRequest{}, apperror.New(apperror.CodeInvalidArgument,
			"Run execution handoff requester is invalid")
	}
	request.OperationKey = operationKey
	request.RequestedBy = requestedBy
	return request, nil
}

func validateRunExecutionHandoffReplay(handoff domain.RunExecutionHandoff,
	request ExecuteRunHandoffRequest, keyDigest string, requestFingerprint string,
) error {
	operation := handoff.Operation
	if operation.ProtocolVersion != domain.RunExecutionHandoffProtocolVersion ||
		operation.KeyDigest != keyDigest ||
		operation.RequestFingerprint != requestFingerprint ||
		operation.RunID != request.RunID || operation.RequestedBy != request.RequestedBy ||
		operation.MaxSteps != request.MaxSteps {
		return apperror.New(apperror.CodeConflict,
			"Run execution handoff key was already used for different intent")
	}
	return nil
}

func executionResultFromHandoff(handoff domain.RunExecutionHandoff) ExecutionResult {
	result := ExecutionResult{RunID: handoff.Operation.RunID,
		Steps: make([]LifecycleResult, 0)}
	if handoff.Result != nil {
		result.RunStatus = handoff.Result.RunStatus
		result.StopReason = handoff.Result.StopReason
	}
	return result
}
