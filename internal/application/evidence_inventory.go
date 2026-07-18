package application

import (
	"context"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/session"
)

type EvidenceInventoryStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetMission(context.Context, string) (domain.Mission, error)
	ListEvidenceAttachments(context.Context, string, int) ([]session.EvidenceAttachment, error)
}

type EvidenceInventoryService struct {
	store EvidenceInventoryStore
}

func NewEvidenceInventoryService(store EvidenceInventoryStore) *EvidenceInventoryService {
	return &EvidenceInventoryService{store: store}
}

func (s *EvidenceInventoryService) List(ctx context.Context,
	runID string,
) (session.EvidenceInventory, error) {
	if s == nil || s.store == nil {
		return session.EvidenceInventory{}, apperror.New(apperror.CodeFailedPrecondition,
			"evidence inventory store is required")
	}
	normalizedRunID := strings.TrimSpace(runID)
	if normalizedRunID != runID || !domain.ValidAgentID(runID) {
		return session.EvidenceInventory{}, apperror.New(apperror.CodeInvalidArgument,
			"evidence inventory Run identity is invalid")
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return session.EvidenceInventory{}, apperror.Normalize(err)
	}
	if run.ID != runID || run.SessionID == "" {
		return session.EvidenceInventory{}, apperror.New(apperror.CodeConflict,
			"evidence inventory Run binding changed")
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return session.EvidenceInventory{}, apperror.Normalize(err)
	}
	if mission.ID != run.MissionID || mission.WorkspaceID == "" {
		return session.EvidenceInventory{}, apperror.New(apperror.CodeConflict,
			"evidence inventory Workspace binding changed")
	}
	attachments, err := s.store.ListEvidenceAttachments(ctx, run.ID,
		session.MaxEvidenceInventoryItems+1)
	if err != nil {
		return session.EvidenceInventory{}, apperror.Normalize(err)
	}
	truncated := len(attachments) > session.MaxEvidenceInventoryItems
	if truncated {
		attachments = attachments[:session.MaxEvidenceInventoryItems]
	}
	items := make([]session.EvidenceInventoryItem, 0, len(attachments))
	for _, attachment := range attachments {
		if err := attachment.Validate(); err != nil {
			return session.EvidenceInventory{}, apperror.Wrap(apperror.CodeInternal,
				"stored evidence attachment is invalid", err)
		}
		if attachment.RunID != run.ID || attachment.SessionID != run.SessionID ||
			attachment.WorkspaceID != mission.WorkspaceID {
			return session.EvidenceInventory{}, apperror.New(apperror.CodeConflict,
				"evidence attachment escaped its requested Run binding")
		}
		items = append(items, session.EvidenceInventoryItem{
			AttachmentID: attachment.ID, RunID: attachment.RunID,
			SessionID: attachment.SessionID, WorkspaceID: attachment.WorkspaceID,
			SourceKind: attachment.SourceKind, SourceRef: attachment.SourceRef,
			ContentSHA256: attachment.ContentSHA256, InstructionAuthorized: false,
			AttachedAt: attachment.CreatedAt,
		})
	}
	inventory := session.EvidenceInventory{
		ProtocolVersion: session.EvidenceInventoryProtocolVersion,
		RunID:           run.ID, Items: items, Truncated: truncated,
	}
	if err := inventory.Validate(); err != nil {
		return session.EvidenceInventory{}, apperror.Wrap(apperror.CodeInternal,
			"evidence inventory projection is invalid", err)
	}
	return inventory, nil
}
