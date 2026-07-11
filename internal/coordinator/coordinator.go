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
	SendAgentMessage(ctx context.Context, message domain.AgentMessage) (domain.AgentMessage, error)
	ListAgentMessages(ctx context.Context, agentID string, pendingOnly bool, limit int) ([]domain.AgentMessage, error)
	ConsumeAgentMessages(ctx context.Context, agentID string, limit int) ([]domain.AgentMessage, error)
	SnapshotAgentGraph(ctx context.Context, runID string) (domain.AgentGraphSnapshot, error)
	GetLatestAgentGraphSnapshot(ctx context.Context, runID string) (domain.AgentGraphSnapshot, bool, error)
	RestoreAgentGraph(ctx context.Context, runID string) (domain.AgentGraph, error)
}

type Coordinator struct {
	store Store
}

type SendRequest struct {
	RunID            string
	SenderAgentID    string
	RecipientAgentID string
	Kind             domain.AgentMessageKind
	Payload          map[string]any
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

func (c *Coordinator) Send(ctx context.Context, req SendRequest) (domain.AgentMessage, error) {
	if c == nil || c.store == nil {
		return domain.AgentMessage{}, apperror.New(apperror.CodeFailedPrecondition,
			"agent coordinator store is required")
	}
	if req.Payload == nil {
		return domain.AgentMessage{}, apperror.New(apperror.CodeInvalidArgument,
			"agent message payload object is required")
	}
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		return domain.AgentMessage{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"agent message payload cannot be encoded", err)
	}
	message, err := c.store.SendAgentMessage(ctx, domain.AgentMessage{
		ID: idgen.New("agentmsg"), RunID: strings.TrimSpace(req.RunID),
		SenderAgentID:    strings.TrimSpace(req.SenderAgentID),
		RecipientAgentID: strings.TrimSpace(req.RecipientAgentID), Kind: req.Kind,
		PayloadJSON: string(payloadJSON), Status: domain.AgentMessagePending, CreatedAt: time.Now().UTC(),
	})
	return message, apperror.Normalize(err)
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
