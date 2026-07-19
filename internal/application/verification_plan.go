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

type VerificationPlanStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetMission(context.Context, string) (domain.Mission, error)
	GetRunMode(context.Context, string) (domain.RunModeSnapshot, error)
	GetSession(context.Context, string) (session.Session, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	GetVerificationPlanByOperation(context.Context, string) (verification.Plan, bool, error)
	ListVerificationPlans(context.Context, string, int) ([]verification.Plan, error)
	RecordVerificationPlan(context.Context, verification.Plan) (verification.Plan, bool, error)
}

type VerificationPlanService struct {
	store VerificationPlanStore
	now   func() time.Time
}

type VerificationPlanItemRequest struct {
	Title               string
	ExpectedObservation string
}

type RecordVerificationPlanRequest struct {
	Version      string
	RunID        string
	Title        string
	Summary      string
	Items        []VerificationPlanItemRequest
	OperationKey string
	AuthoredBy   string
}

type RecordVerificationPlanResult struct {
	Plan     verification.Plan
	Replayed bool
}

type VerificationPlanInventory struct {
	ProtocolVersion string
	RunID           string
	SessionID       string
	WorkspaceID     string
	Items           []verification.Plan
	Truncated       bool
}

func NewVerificationPlanService(store VerificationPlanStore) *VerificationPlanService {
	return &VerificationPlanService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *VerificationPlanService) Record(ctx context.Context,
	request RecordVerificationPlanRequest,
) (RecordVerificationPlanResult, error) {
	if s == nil || s.store == nil || s.now == nil {
		return RecordVerificationPlanResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification plan store is required")
	}
	originalRunID, originalAuthoredBy := request.RunID, request.AuthoredBy
	request.RunID = strings.TrimSpace(request.RunID)
	request.AuthoredBy = strings.TrimSpace(redact.String(request.AuthoredBy))
	originalOperationKey := request.OperationKey
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	if request.Version != verification.PlanProtocolVersion ||
		request.RunID == "" || request.AuthoredBy == "" || request.OperationKey == "" ||
		!domain.ValidAgentID(request.RunID) || !domain.ValidAgentID(request.AuthoredBy) ||
		originalRunID != request.RunID || originalAuthoredBy != request.AuthoredBy ||
		len(request.Items) < 1 || len(request.Items) > verification.MaxPlanItems {
		return RecordVerificationPlanResult{}, apperror.New(apperror.CodeInvalidArgument,
			"verification plan protocol, identity, or item count is invalid")
	}
	if originalOperationKey != request.OperationKey || !utf8.ValidString(request.OperationKey) {
		return RecordVerificationPlanResult{}, apperror.New(apperror.CodeInvalidArgument,
			"verification plan operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil ||
		containsSpaceOrControl(request.OperationKey) {
		return RecordVerificationPlanResult{}, apperror.New(apperror.CodeInvalidArgument,
			"verification plan operation key is invalid")
	}
	for _, current := range request.AuthoredBy {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return RecordVerificationPlanResult{}, apperror.New(apperror.CodeInvalidArgument,
				"verification plan operator identity is invalid")
		}
	}

	originalTitle := normalizedVerificationText(request.Title)
	originalSummary := normalizedVerificationText(request.Summary)
	request.Title = redact.String(originalTitle)
	request.Summary = redact.String(originalSummary)
	redacted := request.Title != originalTitle || request.Summary != originalSummary
	if err := verification.ValidateText(request.Title, verification.MaxTitleRunes, false); err != nil {
		return RecordVerificationPlanResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"verification plan title is invalid", err)
	}
	if err := verification.ValidateText(request.Summary, verification.MaxSummaryRunes, true); err != nil {
		return RecordVerificationPlanResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"verification plan summary is invalid", err)
	}
	items := make([]verification.PlanItem, len(request.Items))
	for index, item := range request.Items {
		originalItemTitle := normalizedVerificationText(item.Title)
		originalExpected := normalizedVerificationText(item.ExpectedObservation)
		item.Title = redact.String(originalItemTitle)
		item.ExpectedObservation = redact.String(originalExpected)
		itemRedacted := item.Title != originalItemTitle ||
			item.ExpectedObservation != originalExpected
		if err := verification.ValidateText(item.Title,
			verification.MaxPlanItemTitleRunes, false); err != nil {
			return RecordVerificationPlanResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
				"verification plan item title is invalid", err)
		}
		if err := verification.ValidateText(item.ExpectedObservation,
			verification.MaxExpectedObservationRunes, true); err != nil {
			return RecordVerificationPlanResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
				"verification plan expected observation is invalid", err)
		}
		items[index] = verification.PlanItem{Ordinal: index + 1, Title: item.Title,
			ExpectedObservation: item.ExpectedObservation,
			ItemSHA256:          verification.PlanItemDigest(item.Title, item.ExpectedObservation),
			Redacted:            itemRedacted}
		redacted = redacted || itemRedacted
	}
	planSHA := verification.PlanDigest(request.Title, request.Summary, items)
	keyDigest := runmutation.VerificationPlanOperationDigest(request.RunID,
		request.OperationKey)
	existing, found, err := s.store.GetVerificationPlanByOperation(ctx, keyDigest)
	if err != nil {
		return RecordVerificationPlanResult{}, apperror.Normalize(err)
	}
	if found {
		fingerprint := runmutation.VerificationPlanRequestFingerprint(request.RunID,
			existing.SessionID, existing.WorkspaceID, planSHA, request.AuthoredBy)
		if existing.RequestFingerprint != fingerprint || existing.RunID != request.RunID ||
			existing.Title != request.Title || existing.Summary != request.Summary ||
			existing.PlanSHA256 != planSHA || existing.AuthoredBy != request.AuthoredBy ||
			existing.Redacted != redacted || !sameVerificationPlanItems(existing.Items, items) {
			return RecordVerificationPlanResult{}, apperror.New(apperror.CodeConflict,
				"verification plan operation key was used for different intent")
		}
		return RecordVerificationPlanResult{Plan: existing, Replayed: true}, nil
	}
	run, mission, linkedSession, registered, err := s.loadBinding(ctx, request.RunID, true)
	if err != nil {
		return RecordVerificationPlanResult{}, err
	}
	now := s.now().UTC()
	if now.Before(run.CreatedAt) {
		now = run.CreatedAt
	}
	plan := verification.Plan{
		ID: idgen.New("verification-plan"), ProtocolVersion: verification.PlanProtocolVersion,
		OperationKeyDigest: keyDigest,
		RequestFingerprint: runmutation.VerificationPlanRequestFingerprint(run.ID,
			linkedSession.ID, registered.ID, planSHA, request.AuthoredBy),
		RunID: run.ID, SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		Title: request.Title, Summary: request.Summary, PlanSHA256: planSHA,
		Redacted: redacted, AuthoredBy: request.AuthoredBy, CreatedAt: now, Items: items,
	}
	prepared := plan
	prepared.EventSequence = 1
	if err := prepared.Validate(); err != nil {
		return RecordVerificationPlanResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"verification plan is invalid", err)
	}
	stored, replayed, err := s.store.RecordVerificationPlan(ctx, plan)
	if err != nil {
		return RecordVerificationPlanResult{}, apperror.Normalize(err)
	}
	return RecordVerificationPlanResult{Plan: stored, Replayed: replayed}, nil
}

func (s *VerificationPlanService) Inventory(ctx context.Context,
	runID string,
) (VerificationPlanInventory, error) {
	if s == nil || s.store == nil {
		return VerificationPlanInventory{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification plan store is required")
	}
	if runID != strings.TrimSpace(runID) {
		return VerificationPlanInventory{}, apperror.New(apperror.CodeInvalidArgument,
			"verification plan Run identity is invalid")
	}
	run, mission, linkedSession, _, err := s.loadBinding(ctx, runID, false)
	if err != nil {
		return VerificationPlanInventory{}, err
	}
	values, err := s.store.ListVerificationPlans(ctx, run.ID,
		verification.MaxPlanInventoryItems+1)
	if err != nil {
		return VerificationPlanInventory{}, apperror.Normalize(err)
	}
	result := VerificationPlanInventory{
		ProtocolVersion: verification.PlanInventoryProtocolVersion, RunID: run.ID,
		SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		Truncated: len(values) > verification.MaxPlanInventoryItems,
	}
	if result.Truncated {
		values = values[:verification.MaxPlanInventoryItems]
	}
	result.Items = append([]verification.Plan{}, values...)
	for _, value := range result.Items {
		if value.RunID != run.ID || value.SessionID != linkedSession.ID ||
			value.WorkspaceID != mission.WorkspaceID {
			return VerificationPlanInventory{}, apperror.New(apperror.CodeConflict,
				"verification plan escaped its Run binding")
		}
	}
	return result, nil
}

func (s *VerificationPlanService) loadBinding(ctx context.Context, runID string,
	requireActiveSession bool,
) (domain.Run, domain.Mission, session.Session, session.WorkspaceInfo, error) {
	if runID == "" || runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeInvalidArgument,
				"verification plan Run identity is invalid")
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	mode, err := s.store.GetRunMode(ctx, run.ID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	linkedSession, err := s.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if run.ID != runID || run.SessionID == "" || mission.ID != run.MissionID ||
		mission.WorkspaceID == "" || mode.RunID != run.ID ||
		mode.Surface != domain.ExecutionSurfaceCode || linkedSession.ID != run.SessionID ||
		linkedSession.WorkspaceID != mission.WorkspaceID ||
		(requireActiveSession && linkedSession.Status != session.StatusActive) {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeConflict,
				"verification plan requires an exact Code Run, Session, and Workspace binding")
	}
	registered, err := s.store.GetWorkspaceInfo(ctx, mission.WorkspaceID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if registered.ID != mission.WorkspaceID {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeConflict,
				"verification plan registered Workspace identity changed")
	}
	return run, mission, linkedSession, registered, nil
}

func sameVerificationPlanItems(left []verification.PlanItem,
	right []verification.PlanItem,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
