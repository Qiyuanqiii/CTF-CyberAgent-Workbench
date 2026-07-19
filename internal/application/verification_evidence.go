package application

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

type VerificationEvidenceStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetMission(context.Context, string) (domain.Mission, error)
	GetSession(context.Context, string) (session.Session, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	GetVerificationEvidenceByOperation(context.Context, string) (
		verification.Evidence, bool, error)
	ListVerificationEvidence(context.Context, string, int) ([]verification.Evidence, error)
	RecordVerificationEvidence(context.Context, verification.Evidence) (
		verification.Evidence, bool, error)
}

type VerificationEvidenceService struct {
	store VerificationEvidenceStore
	now   func() time.Time
}

type RecordVerificationEvidenceRequest struct {
	Version      string
	RunID        string
	Outcome      string
	Title        string
	Summary      string
	OperationKey string
	RecordedBy   string
}

type RecordVerificationEvidenceResult struct {
	Evidence verification.Evidence
	Replayed bool
}

type VerificationEvidenceInventory struct {
	ProtocolVersion string
	RunID           string
	SessionID       string
	WorkspaceID     string
	Items           []verification.Evidence
	PassCount       int
	FailCount       int
	UnknownCount    int
	Truncated       bool
}

func NewVerificationEvidenceService(store VerificationEvidenceStore) *VerificationEvidenceService {
	return &VerificationEvidenceService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *VerificationEvidenceService) Record(ctx context.Context,
	request RecordVerificationEvidenceRequest,
) (RecordVerificationEvidenceResult, error) {
	if s == nil || s.store == nil || s.now == nil {
		return RecordVerificationEvidenceResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification evidence store is required")
	}
	originalRunID := request.RunID
	originalOutcome := request.Outcome
	originalRecordedBy := request.RecordedBy
	originalTitle := normalizedVerificationText(request.Title)
	originalSummary := normalizedVerificationText(request.Summary)
	request.Title = redact.String(originalTitle)
	request.Summary = redact.String(originalSummary)
	redacted := request.Title != originalTitle || request.Summary != originalSummary
	request.RunID = strings.TrimSpace(request.RunID)
	request.Outcome = strings.TrimSpace(request.Outcome)
	request.RecordedBy = strings.TrimSpace(redact.String(request.RecordedBy))
	originalOperationKey := request.OperationKey
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	if request.Version != verification.EvidenceProtocolVersion ||
		!verification.Outcome(request.Outcome).Valid() ||
		request.RunID == "" || request.RecordedBy == "" || request.OperationKey == "" ||
		!domain.ValidAgentID(request.RunID) || !domain.ValidAgentID(request.RecordedBy) ||
		originalRunID != request.RunID || originalOutcome != request.Outcome ||
		originalRecordedBy != request.RecordedBy {
		return RecordVerificationEvidenceResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification evidence protocol, identity, or outcome is invalid")
	}
	if err := verification.ValidateText(request.Title,
		verification.MaxTitleRunes, false); err != nil {
		return RecordVerificationEvidenceResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification evidence title is invalid", err)
	}
	if err := verification.ValidateText(request.Summary,
		verification.MaxSummaryRunes, true); err != nil {
		return RecordVerificationEvidenceResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification evidence summary is invalid", err)
	}
	if originalOperationKey != request.OperationKey || !utf8.ValidString(request.OperationKey) {
		return RecordVerificationEvidenceResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification evidence operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil ||
		containsSpaceOrControl(request.OperationKey) {
		return RecordVerificationEvidenceResult{}, apperror.New(
			apperror.CodeInvalidArgument, "verification evidence operation key is invalid")
	}
	for _, current := range request.RecordedBy {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return RecordVerificationEvidenceResult{}, apperror.New(
				apperror.CodeInvalidArgument, "verification evidence operator identity is invalid")
		}
	}

	keyDigest := runmutation.VerificationEvidenceOperationDigest(request.RunID,
		request.OperationKey)
	existing, found, err := s.store.GetVerificationEvidenceByOperation(ctx, keyDigest)
	if err != nil {
		return RecordVerificationEvidenceResult{}, apperror.Normalize(err)
	}
	if found {
		fingerprint := runmutation.VerificationEvidenceRequestFingerprint(request.RunID,
			existing.SessionID, existing.WorkspaceID, request.Outcome, request.Title,
			request.Summary, request.RecordedBy)
		if existing.RequestFingerprint != fingerprint || existing.RunID != request.RunID ||
			existing.Outcome != verification.Outcome(request.Outcome) ||
			existing.Title != request.Title || existing.Summary != request.Summary ||
			existing.RecordedBy != request.RecordedBy || existing.Redacted != redacted {
			return RecordVerificationEvidenceResult{}, apperror.New(apperror.CodeConflict,
				"verification evidence operation key was used for different intent")
		}
		return RecordVerificationEvidenceResult{Evidence: existing, Replayed: true}, nil
	}

	run, mission, linkedSession, registered, err := s.loadBinding(ctx, request.RunID, true)
	if err != nil {
		return RecordVerificationEvidenceResult{}, err
	}
	now := s.now().UTC()
	if now.Before(run.CreatedAt) {
		now = run.CreatedAt
	}
	evidence := verification.Evidence{
		ID: idgen.New("verification"), ProtocolVersion: verification.EvidenceProtocolVersion,
		OperationKeyDigest: keyDigest,
		RequestFingerprint: runmutation.VerificationEvidenceRequestFingerprint(run.ID,
			linkedSession.ID, registered.ID, request.Outcome, request.Title,
			request.Summary, request.RecordedBy),
		RunID: run.ID, SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		Outcome: verification.Outcome(request.Outcome), Title: request.Title,
		Summary: request.Summary, SummarySHA256: verification.SummaryDigest(request.Summary),
		Redacted: redacted, RecordedBy: request.RecordedBy, CreatedAt: now,
	}
	prepared := evidence
	prepared.EventSequence = 1
	if err := prepared.Validate(); err != nil {
		return RecordVerificationEvidenceResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification evidence is invalid", err)
	}
	stored, replayed, err := s.store.RecordVerificationEvidence(ctx, evidence)
	if err != nil {
		return RecordVerificationEvidenceResult{}, apperror.Normalize(err)
	}
	return RecordVerificationEvidenceResult{Evidence: stored, Replayed: replayed}, nil
}

func (s *VerificationEvidenceService) Inventory(ctx context.Context,
	runID string,
) (VerificationEvidenceInventory, error) {
	if s == nil || s.store == nil {
		return VerificationEvidenceInventory{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification evidence store is required")
	}
	if runID != strings.TrimSpace(runID) {
		return VerificationEvidenceInventory{}, apperror.New(
			apperror.CodeInvalidArgument, "verification evidence Run identity is invalid")
	}
	run, mission, linkedSession, _, err := s.loadBinding(ctx, runID, false)
	if err != nil {
		return VerificationEvidenceInventory{}, err
	}
	values, err := s.store.ListVerificationEvidence(ctx, run.ID,
		verification.MaxInventoryItems+1)
	if err != nil {
		return VerificationEvidenceInventory{}, apperror.Normalize(err)
	}
	result := VerificationEvidenceInventory{
		ProtocolVersion: verification.InventoryProtocolVersion, RunID: run.ID,
		SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		Truncated: len(values) > verification.MaxInventoryItems,
	}
	if result.Truncated {
		values = values[:verification.MaxInventoryItems]
	}
	result.Items = append([]verification.Evidence{}, values...)
	for _, value := range values {
		if value.RunID != run.ID || value.SessionID != linkedSession.ID ||
			value.WorkspaceID != mission.WorkspaceID {
			return VerificationEvidenceInventory{}, apperror.New(apperror.CodeConflict,
				"verification evidence escaped its Run binding")
		}
		switch value.Outcome {
		case verification.OutcomePass:
			result.PassCount++
		case verification.OutcomeFail:
			result.FailCount++
		case verification.OutcomeUnknown:
			result.UnknownCount++
		}
	}
	return result, nil
}

func (s *VerificationEvidenceService) loadBinding(ctx context.Context, runID string,
	requireActiveSession bool,
) (domain.Run, domain.Mission, session.Session, session.WorkspaceInfo, error) {
	if runID == "" || runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeInvalidArgument,
				"verification evidence Run identity is invalid")
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if run.ID != runID || run.SessionID == "" {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeFailedPrecondition,
				"verification evidence requires a Run-bound Session")
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
	if mission.ID != run.MissionID || mission.WorkspaceID == "" ||
		linkedSession.ID != run.SessionID || linkedSession.WorkspaceID != mission.WorkspaceID ||
		(requireActiveSession && linkedSession.Status != session.StatusActive) {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeConflict,
				"verification evidence Run, Mission, Session, or Workspace binding changed")
	}
	registered, err := s.store.GetWorkspaceInfo(ctx, mission.WorkspaceID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if registered.ID != mission.WorkspaceID {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeConflict,
				"verification evidence registered Workspace identity changed")
	}
	return run, mission, linkedSession, registered, nil
}

func normalizedVerificationText(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
}
