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

type VerificationAssociationStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetMission(context.Context, string) (domain.Mission, error)
	GetRunMode(context.Context, string) (domain.RunModeSnapshot, error)
	GetSession(context.Context, string) (session.Session, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	GetVerificationPlan(context.Context, string) (verification.Plan, error)
	GetVerificationEvidence(context.Context, string) (verification.Evidence, error)
	GetVerificationPlanEvidenceAssociationByOperation(context.Context, string) (
		verification.PlanEvidenceAssociation, bool, error)
	ListVerificationPlans(context.Context, string, int) ([]verification.Plan, error)
	ListVerificationPlanEvidenceAssociations(context.Context, string, int) (
		[]verification.PlanEvidenceAssociation, error)
	ListVerificationPlanCoverageCounts(context.Context, string, []string) (
		[]verification.PlanItemCoverageCount, error)
	RecordVerificationPlanEvidenceAssociation(context.Context,
		verification.PlanEvidenceAssociation) (verification.PlanEvidenceAssociation, bool, error)
}

type VerificationAssociationService struct {
	store VerificationAssociationStore
	now   func() time.Time
}

type RecordVerificationAssociationRequest struct {
	Version         string
	RunID           string
	PlanID          string
	PlanItemOrdinal int
	EvidenceID      string
	OperationKey    string
	AssociatedBy    string
}

type RecordVerificationAssociationResult struct {
	Association verification.PlanEvidenceAssociation
	Replayed    bool
}

type VerificationPlanItemCoverage struct {
	Ordinal                        int
	ItemSHA256                     string
	AssociatedEvidenceCount        int
	PassCount                      int
	FailCount                      int
	UnknownCount                   int
	LatestAssociationEventSequence int64
}

type VerificationPlanCoverage struct {
	PlanID                  string
	PlanSHA256              string
	ItemCount               int
	ObservedItemCount       int
	AssociatedEvidenceCount int
	Items                   []VerificationPlanItemCoverage
}

type VerificationPlanCoverageInventory struct {
	ProtocolVersion         string
	RunID                   string
	SessionID               string
	WorkspaceID             string
	Plans                   []VerificationPlanCoverage
	PlanCount               int
	PlanItemCount           int
	ObservedPlanItemCount   int
	AssociatedEvidenceCount int
	Associations            []verification.PlanEvidenceAssociationReference
	PlansTruncated          bool
	AssociationsTruncated   bool
	MetadataOnly            bool
	ReadOnly                bool
	ResultInferred          bool
	CommandExecuted         bool
	ModelAssertion          bool
	RecordRewritten         bool
	Approval                bool
	AuthorityGranted        bool
}

func NewVerificationAssociationService(store VerificationAssociationStore) *VerificationAssociationService {
	return &VerificationAssociationService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *VerificationAssociationService) Record(ctx context.Context,
	request RecordVerificationAssociationRequest,
) (RecordVerificationAssociationResult, error) {
	if s == nil || s.store == nil || s.now == nil {
		return RecordVerificationAssociationResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification association store is required")
	}
	originalRunID, originalPlanID, originalEvidenceID := request.RunID, request.PlanID,
		request.EvidenceID
	originalAssociatedBy := request.AssociatedBy
	request.RunID = strings.TrimSpace(request.RunID)
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.EvidenceID = strings.TrimSpace(request.EvidenceID)
	request.AssociatedBy = strings.TrimSpace(redact.String(request.AssociatedBy))
	originalOperationKey := request.OperationKey
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	if request.Version != verification.PlanEvidenceAssociationProtocolVersion ||
		request.PlanItemOrdinal < 1 || request.PlanItemOrdinal > verification.MaxPlanItems ||
		originalRunID != request.RunID || originalPlanID != request.PlanID ||
		originalEvidenceID != request.EvidenceID || originalAssociatedBy != request.AssociatedBy ||
		!domain.ValidAgentID(request.RunID) || !domain.ValidAgentID(request.PlanID) ||
		!domain.ValidAgentID(request.EvidenceID) || !domain.ValidAgentID(request.AssociatedBy) {
		return RecordVerificationAssociationResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification association protocol, identity, or item ordinal is invalid")
	}
	if originalOperationKey != request.OperationKey || !utf8.ValidString(request.OperationKey) {
		return RecordVerificationAssociationResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification association operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil ||
		containsSpaceOrControl(request.OperationKey) {
		return RecordVerificationAssociationResult{}, apperror.New(
			apperror.CodeInvalidArgument, "verification association operation key is invalid")
	}
	for _, current := range request.AssociatedBy {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return RecordVerificationAssociationResult{}, apperror.New(
				apperror.CodeInvalidArgument,
				"verification association operator identity is invalid")
		}
	}
	keyDigest := runmutation.VerificationPlanEvidenceAssociationOperationDigest(
		request.RunID, request.OperationKey)
	existing, found, err := s.store.GetVerificationPlanEvidenceAssociationByOperation(ctx,
		keyDigest)
	if err != nil {
		return RecordVerificationAssociationResult{}, apperror.Normalize(err)
	}
	if found {
		fingerprint := runmutation.VerificationPlanEvidenceAssociationRequestFingerprint(
			existing.RunID, existing.SessionID, existing.WorkspaceID, existing.PlanID,
			existing.PlanItemOrdinal, existing.PlanItemSHA256, existing.EvidenceID,
			string(existing.EvidenceOutcome), existing.EvidenceEventSequence,
			existing.AssociatedBy)
		if existing.OperationKeyDigest != keyDigest || existing.RequestFingerprint != fingerprint ||
			existing.RunID != request.RunID || existing.PlanID != request.PlanID ||
			existing.PlanItemOrdinal != request.PlanItemOrdinal ||
			existing.EvidenceID != request.EvidenceID ||
			existing.AssociatedBy != request.AssociatedBy {
			return RecordVerificationAssociationResult{}, apperror.New(apperror.CodeConflict,
				"verification association operation key was used for different intent")
		}
		return RecordVerificationAssociationResult{Association: existing, Replayed: true}, nil
	}
	run, mission, linkedSession, registered, err := s.loadBinding(ctx, request.RunID, true)
	if err != nil {
		return RecordVerificationAssociationResult{}, err
	}
	plan, err := s.store.GetVerificationPlan(ctx, request.PlanID)
	if err != nil {
		return RecordVerificationAssociationResult{}, apperror.Normalize(err)
	}
	evidence, err := s.store.GetVerificationEvidence(ctx, request.EvidenceID)
	if err != nil {
		return RecordVerificationAssociationResult{}, apperror.Normalize(err)
	}
	if plan.RunID != run.ID || plan.SessionID != linkedSession.ID ||
		plan.WorkspaceID != registered.ID || evidence.RunID != run.ID ||
		evidence.SessionID != linkedSession.ID || evidence.WorkspaceID != registered.ID ||
		evidence.EventSequence <= plan.EventSequence || request.PlanItemOrdinal > len(plan.Items) {
		return RecordVerificationAssociationResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"verification association requires a later observation from the same Code Run")
	}
	item := plan.Items[request.PlanItemOrdinal-1]
	now := s.now().UTC()
	for _, floor := range []time.Time{run.CreatedAt, plan.CreatedAt, evidence.CreatedAt} {
		if now.Before(floor) {
			now = floor
		}
	}
	association := verification.PlanEvidenceAssociation{
		ID:                 idgen.New("verification-association"),
		ProtocolVersion:    verification.PlanEvidenceAssociationProtocolVersion,
		OperationKeyDigest: keyDigest,
		RequestFingerprint: runmutation.VerificationPlanEvidenceAssociationRequestFingerprint(
			run.ID, linkedSession.ID, registered.ID, plan.ID, item.Ordinal, item.ItemSHA256,
			evidence.ID, string(evidence.Outcome), evidence.EventSequence, request.AssociatedBy),
		RunID: run.ID, SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		PlanID: plan.ID, PlanItemOrdinal: item.Ordinal, PlanItemSHA256: item.ItemSHA256,
		EvidenceID: evidence.ID, EvidenceOutcome: evidence.Outcome,
		EvidenceEventSequence: evidence.EventSequence, AssociatedBy: request.AssociatedBy,
		CreatedAt: now,
	}
	prepared := association
	prepared.EventSequence = evidence.EventSequence + 1
	if err := prepared.Validate(); err != nil {
		return RecordVerificationAssociationResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification association is invalid", err)
	}
	stored, replayed, err := s.store.RecordVerificationPlanEvidenceAssociation(ctx, association)
	if err != nil {
		return RecordVerificationAssociationResult{}, apperror.Normalize(err)
	}
	return RecordVerificationAssociationResult{Association: stored, Replayed: replayed}, nil
}

func (s *VerificationAssociationService) Coverage(ctx context.Context,
	runID string,
) (VerificationPlanCoverageInventory, error) {
	if s == nil || s.store == nil {
		return VerificationPlanCoverageInventory{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification association store is required")
	}
	if runID != strings.TrimSpace(runID) {
		return VerificationPlanCoverageInventory{}, apperror.New(
			apperror.CodeInvalidArgument, "verification coverage Run identity is invalid")
	}
	run, mission, linkedSession, _, err := s.loadBinding(ctx, runID, false)
	if err != nil {
		return VerificationPlanCoverageInventory{}, err
	}
	plans, err := s.store.ListVerificationPlans(ctx, run.ID,
		verification.MaxPlanInventoryItems+1)
	if err != nil {
		return VerificationPlanCoverageInventory{}, apperror.Normalize(err)
	}
	result := VerificationPlanCoverageInventory{
		ProtocolVersion: verification.PlanCoverageProtocolVersion, RunID: run.ID,
		SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		PlansTruncated: len(plans) > verification.MaxPlanInventoryItems,
		MetadataOnly:   true, ReadOnly: true,
	}
	if result.PlansTruncated {
		plans = plans[:verification.MaxPlanInventoryItems]
	}
	planIDs := make([]string, len(plans))
	planIndex := make(map[string]int, len(plans))
	itemIndex := make(map[string]map[int]int, len(plans))
	result.Plans = make([]VerificationPlanCoverage, len(plans))
	for index, plan := range plans {
		if plan.RunID != run.ID || plan.SessionID != linkedSession.ID ||
			plan.WorkspaceID != mission.WorkspaceID {
			return VerificationPlanCoverageInventory{}, apperror.New(apperror.CodeConflict,
				"verification coverage plan escaped its Run binding")
		}
		planIDs[index] = plan.ID
		planIndex[plan.ID] = index
		itemIndex[plan.ID] = make(map[int]int, len(plan.Items))
		projected := VerificationPlanCoverage{PlanID: plan.ID,
			PlanSHA256: plan.PlanSHA256, ItemCount: len(plan.Items),
			Items: make([]VerificationPlanItemCoverage, len(plan.Items))}
		for itemPosition, item := range plan.Items {
			itemIndex[plan.ID][item.Ordinal] = itemPosition
			projected.Items[itemPosition] = VerificationPlanItemCoverage{
				Ordinal: item.Ordinal, ItemSHA256: item.ItemSHA256}
		}
		result.PlanItemCount += len(plan.Items)
		result.Plans[index] = projected
	}
	counts, err := s.store.ListVerificationPlanCoverageCounts(ctx, run.ID, planIDs)
	if err != nil {
		return VerificationPlanCoverageInventory{}, apperror.Normalize(err)
	}
	for _, count := range counts {
		planPosition, exists := planIndex[count.PlanID]
		itemPosition, itemExists := itemIndex[count.PlanID][count.PlanItemOrdinal]
		if !exists || !itemExists ||
			result.Plans[planPosition].Items[itemPosition].ItemSHA256 != count.PlanItemSHA256 {
			return VerificationPlanCoverageInventory{}, apperror.New(apperror.CodeConflict,
				"verification coverage escaped its exact plan item binding")
		}
		item := &result.Plans[planPosition].Items[itemPosition]
		item.AssociatedEvidenceCount = count.AssociatedEvidenceCount
		item.PassCount = count.PassCount
		item.FailCount = count.FailCount
		item.UnknownCount = count.UnknownCount
		item.LatestAssociationEventSequence = count.LatestAssociationEventSequence
		if count.AssociatedEvidenceCount > verification.MaxSafeCoverageCount-
			result.Plans[planPosition].AssociatedEvidenceCount ||
			count.AssociatedEvidenceCount > verification.MaxSafeCoverageCount-
				result.AssociatedEvidenceCount {
			return VerificationPlanCoverageInventory{}, apperror.New(
				apperror.CodeResourceExhausted, "verification coverage count exceeds its limit")
		}
		result.Plans[planPosition].ObservedItemCount++
		result.Plans[planPosition].AssociatedEvidenceCount += count.AssociatedEvidenceCount
		result.ObservedPlanItemCount++
		result.AssociatedEvidenceCount += count.AssociatedEvidenceCount
	}
	associations, err := s.store.ListVerificationPlanEvidenceAssociations(ctx, run.ID,
		verification.MaxCoverageAssociations+1)
	if err != nil {
		return VerificationPlanCoverageInventory{}, apperror.Normalize(err)
	}
	result.AssociationsTruncated = len(associations) > verification.MaxCoverageAssociations
	if result.AssociationsTruncated {
		associations = associations[:verification.MaxCoverageAssociations]
	}
	result.Associations = make([]verification.PlanEvidenceAssociationReference,
		len(associations))
	for index, association := range associations {
		if association.RunID != run.ID || association.SessionID != linkedSession.ID ||
			association.WorkspaceID != mission.WorkspaceID {
			return VerificationPlanCoverageInventory{}, apperror.New(apperror.CodeConflict,
				"verification association escaped its Run binding")
		}
		result.Associations[index] = verification.PlanEvidenceAssociationReference{
			ID: association.ID, PlanID: association.PlanID,
			PlanItemOrdinal: association.PlanItemOrdinal,
			PlanItemSHA256:  association.PlanItemSHA256, EvidenceID: association.EvidenceID,
			EvidenceOutcome:       association.EvidenceOutcome,
			EvidenceEventSequence: association.EvidenceEventSequence,
			AssociationSequence:   association.EventSequence, CreatedAt: association.CreatedAt,
		}
	}
	result.PlanCount = len(result.Plans)
	return result, nil
}

func (s *VerificationAssociationService) loadBinding(ctx context.Context, runID string,
	requireActiveSession bool,
) (domain.Run, domain.Mission, session.Session, session.WorkspaceInfo, error) {
	if runID == "" || runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeInvalidArgument,
				"verification association Run identity is invalid")
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
				"verification association requires an exact Code Run, Session, and Workspace binding")
	}
	registered, err := s.store.GetWorkspaceInfo(ctx, mission.WorkspaceID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.Normalize(err)
	}
	if registered.ID != mission.WorkspaceID {
		return domain.Run{}, domain.Mission{}, session.Session{}, session.WorkspaceInfo{},
			apperror.New(apperror.CodeConflict,
				"verification association registered Workspace identity changed")
	}
	return run, mission, linkedSession, registered, nil
}
