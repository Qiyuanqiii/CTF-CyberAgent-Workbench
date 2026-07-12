package coordinator

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
)

type Store interface {
	RegisterRootAgent(ctx context.Context, runID string) (domain.AgentNode, bool, error)
	GetAgentNode(ctx context.Context, id string) (domain.AgentNode, error)
	GetRootAgent(ctx context.Context, runID string) (domain.AgentNode, bool, error)
	ListAgentNodes(ctx context.Context, runID string) ([]domain.AgentNode, error)
	AdmitSpecialist(ctx context.Context, admission domain.SpecialistAdmission,
		operationKey string) (domain.AgentNode, bool, error)
	FinishSpecialist(ctx context.Context, completion domain.AgentCompletion,
		operationKey string) (domain.AgentCompletion, bool, error)
	GetAgentCompletion(ctx context.Context, agentID string) (domain.AgentCompletion, bool, error)
	SendAgentMessage(ctx context.Context, message domain.AgentMessage,
		operationKey string) (domain.AgentMessage, bool, error)
	ListAgentMessages(ctx context.Context, agentID string, pendingOnly bool, limit int) ([]domain.AgentMessage, error)
	ConsumeAgentMessages(ctx context.Context, agentID string, limit int) ([]domain.AgentMessage, error)
	SnapshotAgentGraph(ctx context.Context, runID string) (domain.AgentGraphSnapshot, error)
	GetLatestAgentGraphSnapshot(ctx context.Context, runID string) (domain.AgentGraphSnapshot, bool, error)
	RestoreAgentGraph(ctx context.Context, runID string) (domain.AgentGraph, error)
}

type Coordinator struct {
	store            Store
	specialistPolicy *SpecialistAdmissionPolicy
}

type SendRequest struct {
	RunID            string
	SenderAgentID    string
	RecipientAgentID string
	Kind             domain.AgentMessageKind
	Semantic         domain.AgentMessageSemantic
	Payload          map[string]any
	IdempotencyKey   string
}

type SendResult struct {
	Message  domain.AgentMessage
	Replayed bool
}

func New(store Store) *Coordinator {
	return &Coordinator{store: store}
}

func (c *Coordinator) RegisterRoot(ctx context.Context, runID string) (domain.AgentNode, bool, error) {
	if c == nil || c.store == nil {
		return domain.AgentNode{}, false, apperror.New(apperror.CodeFailedPrecondition,
			"agent coordinator store is required")
	}
	node, created, err := c.store.RegisterRootAgent(ctx, strings.TrimSpace(runID))
	return node, created, apperror.Normalize(err)
}

func (c *Coordinator) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	if c == nil || c.store == nil {
		return SendResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"agent coordinator store is required")
	}
	if req.Payload == nil {
		return SendResult{}, apperror.New(apperror.CodeInvalidArgument,
			"agent message payload object is required")
	}
	operationKey, err := domain.NormalizeAgentOperationKey(req.IdempotencyKey)
	if err != nil {
		return SendResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"agent message idempotency key is invalid", err)
	}
	semantic := req.Semantic
	if semantic == "" {
		semantic = domain.AgentMessageSemanticMessage
	}
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		return SendResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"agent message payload cannot be encoded", err)
	}
	message, replayed, err := c.store.SendAgentMessage(ctx, domain.AgentMessage{
		ID: idgen.New("agentmsg"), RunID: strings.TrimSpace(req.RunID),
		SenderAgentID:    strings.TrimSpace(req.SenderAgentID),
		RecipientAgentID: strings.TrimSpace(req.RecipientAgentID), Kind: req.Kind, Semantic: semantic,
		PayloadJSON: string(payloadJSON), Status: domain.AgentMessagePending, CreatedAt: time.Now().UTC(),
	}, operationKey)
	return SendResult{Message: message, Replayed: replayed}, apperror.Normalize(err)
}

func (c *Coordinator) Inbox(ctx context.Context, agentID string, pendingOnly bool,
	limit int,
) ([]domain.AgentMessage, error) {
	if c == nil || c.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "agent coordinator store is required")
	}
	messages, err := c.store.ListAgentMessages(ctx, strings.TrimSpace(agentID), pendingOnly, limit)
	return messages, apperror.Normalize(err)
}

func (c *Coordinator) Consume(ctx context.Context, agentID string, limit int) ([]domain.AgentMessage, error) {
	if c == nil || c.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "agent coordinator store is required")
	}
	messages, err := c.store.ConsumeAgentMessages(ctx, strings.TrimSpace(agentID), limit)
	return messages, apperror.Normalize(err)
}

func (c *Coordinator) Snapshot(ctx context.Context, runID string) (domain.AgentGraphSnapshot, error) {
	if c == nil || c.store == nil {
		return domain.AgentGraphSnapshot{}, apperror.New(apperror.CodeFailedPrecondition,
			"agent coordinator store is required")
	}
	snapshot, err := c.store.SnapshotAgentGraph(ctx, strings.TrimSpace(runID))
	return snapshot, apperror.Normalize(err)
}

func (c *Coordinator) Restore(ctx context.Context, runID string) (domain.AgentGraph, error) {
	if c == nil || c.store == nil {
		return domain.AgentGraph{}, apperror.New(apperror.CodeFailedPrecondition,
			"agent coordinator store is required")
	}
	graph, err := c.store.RestoreAgentGraph(ctx, strings.TrimSpace(runID))
	return graph, apperror.Normalize(err)
}
