package application

import (
	"context"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/session"
)

type SessionSteeringCancellationStore interface {
	GetSession(context.Context, string) (session.Session, error)
	GetRunBySession(context.Context, string) (domain.Run, bool, error)
	GetOperatorSteering(context.Context, string) (domain.OperatorSteeringMessage, error)
	CancelOperatorSteering(context.Context,
		domain.CancelOperatorSteeringRequest) (domain.OperatorSteeringCancellationResult, error)
}

type SessionSteeringCancellationService struct {
	store SessionSteeringCancellationStore
}

type CancelSessionSteeringRequest struct {
	Version      string
	SessionID    string
	MessageID    string
	OperationKey string
	RequestedBy  string
	Reason       string
}

type CancelSessionSteeringResult struct {
	Run          domain.Run
	Session      session.Session
	Message      domain.OperatorSteeringMessage
	Cancellation domain.OperatorSteeringCancellation
	Replayed     bool
}

func NewSessionSteeringCancellationService(
	store SessionSteeringCancellationStore,
) *SessionSteeringCancellationService {
	return &SessionSteeringCancellationService{store: store}
}

func (s *SessionSteeringCancellationService) Cancel(ctx context.Context,
	request CancelSessionSteeringRequest,
) (CancelSessionSteeringResult, error) {
	if s == nil || s.store == nil {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Session steering cancellation store is required")
	}
	if request.Version != domain.SessionSteeringCancellationProtocolVersion {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeInvalidArgument, "unsupported Session steering cancellation version")
	}
	if request.SessionID != strings.TrimSpace(request.SessionID) ||
		!domain.ValidAgentID(request.SessionID) ||
		request.MessageID != strings.TrimSpace(request.MessageID) ||
		!domain.ValidAgentID(request.MessageID) {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeInvalidArgument, "Session steering cancellation identity is invalid")
	}
	operationKey, err := domain.NormalizeAgentOperationKey(request.OperationKey)
	if err != nil || containsSpaceOrControl(operationKey) {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeInvalidArgument, "Session steering cancellation idempotency key is invalid")
	}
	requestedBy := strings.TrimSpace(request.RequestedBy)
	if requestedBy != request.RequestedBy || !domain.ValidAgentID(requestedBy) {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeInvalidArgument, "Session steering cancellation requester is invalid")
	}
	reason, err := domain.NormalizeOperatorSteeringCancellationReason(request.Reason)
	if err != nil {
		return CancelSessionSteeringResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}

	linkedSession, err := s.store.GetSession(ctx, request.SessionID)
	if err != nil {
		return CancelSessionSteeringResult{}, apperror.Normalize(err)
	}
	run, found, err := s.store.GetRunBySession(ctx, linkedSession.ID)
	if err != nil {
		return CancelSessionSteeringResult{}, apperror.Normalize(err)
	}
	if !found {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Session is not bound to a Run")
	}
	if linkedSession.ID != request.SessionID || run.SessionID != linkedSession.ID {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeConflict, "Session steering Run binding changed")
	}
	message, err := s.store.GetOperatorSteering(ctx, request.MessageID)
	if err != nil {
		return CancelSessionSteeringResult{}, apperror.Normalize(err)
	}
	if message.RunID != run.ID || message.SessionID != linkedSession.ID {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeConflict, "Session steering message binding does not match")
	}

	result, err := s.store.CancelOperatorSteering(ctx,
		domain.CancelOperatorSteeringRequest{
			MessageID: message.ID, OperationKey: operationKey,
			RequestedBy: requestedBy, Reason: reason,
		})
	if err != nil {
		return CancelSessionSteeringResult{}, apperror.Normalize(err)
	}
	if result.Message.ID != message.ID || result.Message.RunID != run.ID ||
		result.Message.SessionID != linkedSession.ID ||
		result.Cancellation.MessageID != message.ID ||
		result.Cancellation.RunID != run.ID {
		return CancelSessionSteeringResult{}, apperror.New(
			apperror.CodeConflict, "cancelled Session steering binding changed")
	}
	return CancelSessionSteeringResult{
		Run: run, Session: linkedSession, Message: result.Message,
		Cancellation: result.Cancellation, Replayed: result.Replayed,
	}, nil
}
