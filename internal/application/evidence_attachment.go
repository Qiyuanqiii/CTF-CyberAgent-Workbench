package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/workspace"
)

type EvidenceAttachmentStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetMission(context.Context, string) (domain.Mission, error)
	GetSession(context.Context, string) (session.Session, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	GetEvidenceAttachment(context.Context, string) (session.EvidenceAttachment,
		session.Message, bool, error)
	AttachEvidence(context.Context, session.EvidenceAttachment, session.Message) (
		session.EvidenceAttachment, session.Message, bool, error)
}

type EvidenceAttachmentService struct {
	store EvidenceAttachmentStore
	now   func() time.Time
}

type AttachEvidenceRequest struct {
	Version       string
	RunID         string
	SourceKind    string
	SourceRef     string
	ContentSHA256 string
	OperationKey  string
	AttachedBy    string
}

type AttachEvidenceResult struct {
	Attachment session.EvidenceAttachment
	Message    session.Message
	Replayed   bool
}

func NewEvidenceAttachmentService(store EvidenceAttachmentStore) *EvidenceAttachmentService {
	return &EvidenceAttachmentService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *EvidenceAttachmentService) Attach(ctx context.Context,
	request AttachEvidenceRequest,
) (AttachEvidenceResult, error) {
	if s == nil || s.store == nil || s.now == nil {
		return AttachEvidenceResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"evidence attachment store is required")
	}
	if request.Version != session.EvidenceAttachmentProtocolVersion ||
		request.SourceKind != session.SourceWorkspaceFile ||
		request.RunID != strings.TrimSpace(request.RunID) ||
		request.AttachedBy != strings.TrimSpace(request.AttachedBy) ||
		!domain.ValidAgentID(request.RunID) || !domain.ValidAgentID(request.AttachedBy) ||
		!validEvidenceDigest(request.ContentSHA256) {
		return AttachEvidenceResult{}, apperror.New(apperror.CodeInvalidArgument,
			"evidence attachment protocol, identity, source, or digest is invalid")
	}
	operationKey, err := domain.NormalizeAgentOperationKey(request.OperationKey)
	if err != nil || containsSpaceOrControl(operationKey) {
		return AttachEvidenceResult{}, apperror.New(apperror.CodeInvalidArgument,
			"evidence attachment idempotency key is invalid")
	}
	keyDigest := runmutation.EvidenceAttachmentOperationDigest(request.RunID, operationKey)

	existing, message, found, err := s.store.GetEvidenceAttachment(ctx, keyDigest)
	if err != nil {
		return AttachEvidenceResult{}, apperror.Normalize(err)
	}
	if found {
		fingerprint := runmutation.EvidenceAttachmentRequestFingerprint(request.RunID,
			existing.WorkspaceID, request.SourceKind, request.SourceRef,
			request.ContentSHA256, request.AttachedBy)
		if existing.OperationKeyDigest != keyDigest ||
			existing.RequestFingerprint != fingerprint || existing.RunID != request.RunID ||
			existing.SourceKind != request.SourceKind || existing.SourceRef != request.SourceRef ||
			existing.ContentSHA256 != request.ContentSHA256 || existing.AttachedBy != request.AttachedBy {
			return AttachEvidenceResult{}, apperror.New(apperror.CodeConflict,
				"evidence attachment operation key was used for different intent")
		}
		return AttachEvidenceResult{Attachment: existing, Message: message,
			Replayed: true}, nil
	}

	run, mission, linkedSession, registered, err := s.loadEvidenceBinding(ctx, request.RunID)
	if err != nil {
		return AttachEvidenceResult{}, err
	}
	fingerprint := runmutation.EvidenceAttachmentRequestFingerprint(run.ID,
		registered.ID, request.SourceKind, request.SourceRef, request.ContentSHA256,
		request.AttachedBy)
	snapshot, err := workspace.Explore(registered.RootPath, registered.ID, request.SourceRef)
	if err != nil {
		return AttachEvidenceResult{}, err
	}
	if snapshot.Kind != "file" || snapshot.Provenance.SourceKind != session.SourceWorkspaceFile ||
		snapshot.Provenance.SourceRef != request.SourceRef ||
		snapshot.Provenance.ContentSHA256 != request.ContentSHA256 ||
		snapshot.Provenance.InstructionAuthorized {
		return AttachEvidenceResult{}, apperror.New(apperror.CodeConflict,
			"Workspace evidence changed after it was projected")
	}
	now := s.now().UTC()
	evidenceMessage := session.NewEvidenceMessage(linkedSession.ID,
		session.SourceWorkspaceFile, snapshot.Provenance.SourceRef, snapshot.Content)
	if evidenceMessage.Provenance.ContentSHA256 != request.ContentSHA256 ||
		evidenceMessage.Provenance.InstructionAuthorized {
		return AttachEvidenceResult{}, apperror.New(apperror.CodeInternal,
			"Workspace evidence projection changed during attachment")
	}
	evidenceMessage.CreatedAt = now
	attachment := session.EvidenceAttachment{
		ID: idgen.New("evidence"), ProtocolVersion: session.EvidenceAttachmentProtocolVersion,
		OperationKeyDigest: keyDigest, RequestFingerprint: fingerprint,
		RunID: run.ID, SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		SourceKind: request.SourceKind, SourceRef: snapshot.Provenance.SourceRef,
		ContentSHA256: request.ContentSHA256, AttachedBy: request.AttachedBy,
		CreatedAt: now,
	}
	stored, storedMessage, replayed, err := s.store.AttachEvidence(ctx, attachment,
		evidenceMessage)
	if err != nil {
		return AttachEvidenceResult{}, apperror.Normalize(err)
	}
	return AttachEvidenceResult{Attachment: stored, Message: storedMessage,
		Replayed: replayed}, nil
}

func (s *EvidenceAttachmentService) loadEvidenceBinding(ctx context.Context,
	runID string,
) (domain.Run, domain.Mission, session.Session, session.WorkspaceInfo, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if run.ID != runID || run.SessionID == "" ||
		(run.Status != domain.RunRunning && run.Status != domain.RunPaused) {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeFailedPrecondition,
				"evidence attachment requires a running or paused Run")
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	linkedSession, err := s.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if linkedSession.ID != run.SessionID || linkedSession.Status != session.StatusActive ||
		mission.ID != run.MissionID || mission.WorkspaceID == "" ||
		linkedSession.WorkspaceID != mission.WorkspaceID {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeConflict,
				"Run, Mission, Session, and Workspace evidence binding changed")
	}
	registered, err := s.store.GetWorkspaceInfo(ctx, mission.WorkspaceID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if registered.ID != mission.WorkspaceID {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeConflict, "registered Workspace identity changed")
	}
	return run, mission, linkedSession, registered, nil
}

func validEvidenceDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
