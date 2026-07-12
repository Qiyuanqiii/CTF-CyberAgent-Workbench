package store

import (
	"context"
	"database/sql"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

func insertBoundedAgentMessageTx(ctx context.Context, tx *sql.Tx,
	message *domain.AgentMessage,
) error {
	var pending, history int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_messages
		WHERE recipient_agent_id = ? AND status = ?`, message.RecipientAgentID,
		domain.AgentMessagePending).Scan(&pending); err != nil {
		return err
	}
	if pending >= domain.MaxAgentInboxMessages {
		return apperror.New(apperror.CodeResourceExhausted, "recipient Agent inbox is full")
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_messages
		WHERE recipient_agent_id = ?`, message.RecipientAgentID).Scan(&history); err != nil {
		return err
	}
	if history >= domain.MaxAgentMessageHistory {
		return apperror.New(apperror.CodeResourceExhausted, "recipient Agent message history is full")
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM agent_messages
		WHERE recipient_agent_id = ?`, message.RecipientAgentID).Scan(&message.Sequence); err != nil {
		return err
	}
	if err := message.Validate(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_messages
		(id, run_id, sender_agent_id, recipient_agent_id, sequence, kind, semantic, payload_json,
		status, created_at, consumed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		message.ID, message.RunID, nullableAgentID(message.SenderAgentID), message.RecipientAgentID,
		message.Sequence, message.Kind, message.Semantic, message.PayloadJSON, message.Status,
		ts(message.CreatedAt))
	return err
}
