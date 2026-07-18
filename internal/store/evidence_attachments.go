package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
)

const evidenceAttachmentSelect = `SELECT id, protocol_version, operation_key_digest,
	request_fingerprint, run_id, session_id, workspace_id, source_kind, source_ref,
	content_sha256, session_message_id, attached_by, event_sequence, created_at
	FROM session_evidence_attachments`

func (s *SQLiteStore) GetEvidenceAttachment(ctx context.Context,
	keyDigest string,
) (session.EvidenceAttachment, session.Message, bool, error) {
	if !validStoreDigest(keyDigest) {
		return session.EvidenceAttachment{}, session.Message{}, false,
			apperror.New(apperror.CodeInvalidArgument,
				"evidence attachment operation digest is invalid")
	}
	attachment, err := scanEvidenceAttachment(s.db.QueryRowContext(ctx,
		evidenceAttachmentSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return session.EvidenceAttachment{}, session.Message{}, false, nil
	}
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	message, err := getSessionMessageByID(ctx, s.db, attachment.SessionMessageID)
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	if err := validateEvidenceMessageBinding(attachment, message); err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	return attachment, message, true, nil
}

func (s *SQLiteStore) AttachEvidence(ctx context.Context,
	attachment session.EvidenceAttachment, message session.Message,
) (session.EvidenceAttachment, session.Message, bool, error) {
	if attachment.SessionMessageID != 0 || attachment.EventSequence != 0 ||
		message.ID != 0 || message.SessionID != attachment.SessionID ||
		message.Role != "tool" || message.Provenance.Version != session.ContextProvenanceVersion ||
		message.Provenance.SourceKind != attachment.SourceKind ||
		message.Provenance.SourceRef != attachment.SourceRef ||
		message.Provenance.ContentSHA256 != attachment.ContentSHA256 ||
		message.Provenance.InstructionAuthorized ||
		!message.CreatedAt.Equal(attachment.CreatedAt) {
		return session.EvidenceAttachment{}, session.Message{}, false,
			apperror.New(apperror.CodeInvalidArgument,
				"evidence attachment and Session message binding is invalid")
	}
	prepared := attachment
	prepared.SessionMessageID = 1
	prepared.EventSequence = 1
	if err := prepared.Validate(); err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument,
				"evidence attachment is invalid", err)
	}
	preparedMessage, err := session.PrepareMessageForStorage(message)
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false,
			apperror.Wrap(apperror.CodeInvalidArgument,
				"evidence Session message is invalid", err)
	}
	if preparedMessage.Provenance.ContentSHA256 != attachment.ContentSHA256 {
		return session.EvidenceAttachment{}, session.Message{}, false,
			apperror.New(apperror.CodeInvalidArgument,
				"evidence Session message digest is invalid")
	}
	message = preparedMessage

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`,
		attachment.RunID)
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	if rows != 1 {
		return session.EvidenceAttachment{}, session.Message{}, false,
			apperror.New(apperror.CodeNotFound, "evidence attachment Run was not found")
	}
	if existing, found, lookupErr := getEvidenceAttachmentTx(ctx, tx,
		attachment.OperationKeyDigest); lookupErr != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, lookupErr
	} else if found {
		if !sameEvidenceAttachmentIntent(existing, attachment) {
			return session.EvidenceAttachment{}, session.Message{}, false,
				apperror.New(apperror.CodeConflict,
					"evidence attachment operation key was used for different intent")
		}
		storedMessage, messageErr := getSessionMessageByID(ctx, tx,
			existing.SessionMessageID)
		if messageErr != nil {
			return session.EvidenceAttachment{}, session.Message{}, false, messageErr
		}
		if bindErr := validateEvidenceMessageBinding(existing, storedMessage); bindErr != nil {
			return session.EvidenceAttachment{}, session.Message{}, false, bindErr
		}
		if err := tx.Commit(); err != nil {
			return session.EvidenceAttachment{}, session.Message{}, false, err
		}
		return existing, storedMessage, true, nil
	}

	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status,
		config_json, budget_json, started_at, finished_at, created_at, updated_at
		FROM runs WHERE id = ?`, attachment.RunID))
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	var workspaceID, sessionWorkspaceID, sessionStatus string
	if err := tx.QueryRowContext(ctx, `SELECT mission.workspace_id,
		session_record.workspace_id, session_record.status
		FROM missions mission JOIN sessions session_record ON session_record.id = ?
		WHERE mission.id = ?`, attachment.SessionID, run.MissionID).Scan(
		&workspaceID, &sessionWorkspaceID, &sessionStatus); err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	if run.SessionID != attachment.SessionID ||
		(run.Status != domain.RunRunning && run.Status != domain.RunPaused) ||
		workspaceID != attachment.WorkspaceID || sessionWorkspaceID != attachment.WorkspaceID ||
		sessionStatus != session.StatusActive {
		return session.EvidenceAttachment{}, session.Message{}, false,
			apperror.New(apperror.CodeConflict,
				"evidence attachment Run, Session, or Workspace binding changed")
	}
	storedMessage, err := saveSessionMessageTx(ctx, tx, message)
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	attachment.SessionMessageID = storedMessage.ID
	event, err := events.New(run.ID, run.MissionID, events.SessionEvidenceAttachedEvent,
		"evidence_attachment", attachment.ID, map[string]any{
			"session_message_id":     storedMessage.ID,
			"source_kind":            attachment.SourceKind,
			"source_ref":             attachment.SourceRef,
			"content_sha256":         attachment.ContentSHA256,
			"instruction_authorized": false,
			"model_called":           false,
			"tool_called":            false,
		})
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	event.CreatedAt = attachment.CreatedAt
	event, err = insertRunEventTx(ctx, tx, event)
	if err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	attachment.EventSequence = event.Sequence
	if err := attachment.Validate(); err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_evidence_attachments
		(id, protocol_version, operation_key_digest, request_fingerprint, run_id,
		session_id, workspace_id, source_kind, source_ref, content_sha256,
		session_message_id, attached_by, event_sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, attachment.ID,
		attachment.ProtocolVersion, attachment.OperationKeyDigest,
		attachment.RequestFingerprint, attachment.RunID, attachment.SessionID,
		attachment.WorkspaceID, attachment.SourceKind, attachment.SourceRef,
		attachment.ContentSHA256, attachment.SessionMessageID, attachment.AttachedBy,
		attachment.EventSequence, ts(attachment.CreatedAt)); err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return session.EvidenceAttachment{}, session.Message{}, false, err
	}
	return attachment, storedMessage, false, nil
}

func scanEvidenceAttachment(row scanner) (session.EvidenceAttachment, error) {
	var value session.EvidenceAttachment
	var created string
	if err := row.Scan(&value.ID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.RequestFingerprint, &value.RunID, &value.SessionID, &value.WorkspaceID,
		&value.SourceKind, &value.SourceRef, &value.ContentSHA256,
		&value.SessionMessageID, &value.AttachedBy, &value.EventSequence, &created); err != nil {
		return session.EvidenceAttachment{}, err
	}
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return session.EvidenceAttachment{}, fmt.Errorf(
			"stored evidence attachment is invalid: %w", err)
	}
	return value, nil
}

func getEvidenceAttachmentTx(ctx context.Context, tx *sql.Tx,
	keyDigest string,
) (session.EvidenceAttachment, bool, error) {
	value, err := scanEvidenceAttachment(tx.QueryRowContext(ctx,
		evidenceAttachmentSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return session.EvidenceAttachment{}, false, nil
	}
	return value, err == nil, err
}

func getSessionMessageByID(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64,
) (session.Message, error) {
	return scanSessionMessage(queryer.QueryRowContext(ctx, `SELECT id, session_id, role,
		content, provenance_version, source_kind, source_ref, content_sha256,
		instruction_authorized, token_estimate, compacted, created_at
		FROM session_messages WHERE id = ?`, id))
}

func sameEvidenceAttachmentIntent(left session.EvidenceAttachment,
	right session.EvidenceAttachment,
) bool {
	return left.ProtocolVersion == right.ProtocolVersion &&
		left.OperationKeyDigest == right.OperationKeyDigest &&
		left.RequestFingerprint == right.RequestFingerprint &&
		left.RunID == right.RunID && left.SessionID == right.SessionID &&
		left.WorkspaceID == right.WorkspaceID && left.SourceKind == right.SourceKind &&
		left.SourceRef == right.SourceRef && left.ContentSHA256 == right.ContentSHA256 &&
		left.AttachedBy == right.AttachedBy
}

func validateEvidenceMessageBinding(attachment session.EvidenceAttachment,
	message session.Message,
) error {
	if message.ID != attachment.SessionMessageID || message.SessionID != attachment.SessionID ||
		message.Role != "tool" || message.Provenance.Version != session.ContextProvenanceVersion ||
		message.Provenance.SourceKind != attachment.SourceKind ||
		message.Provenance.SourceRef != attachment.SourceRef ||
		message.Provenance.ContentSHA256 != attachment.ContentSHA256 ||
		message.Provenance.InstructionAuthorized || !message.CreatedAt.Equal(attachment.CreatedAt) {
		return errors.New("stored evidence attachment Session message binding is invalid")
	}
	return session.ValidateStoredMessage(message)
}
