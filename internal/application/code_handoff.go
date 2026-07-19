package application

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/operatoraction"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

const (
	CodeHandoffProtocolVersion     = "code_handoff.v1"
	MaxCodeHandoffActionReferences = 20
	MaxCodeHandoffReportReferences = 20
	MaxCodeHandoffVerifyReferences = 20
	maxCodeHandoffSnapshotAttempts = 4
)

type CodeHandoffStore interface {
	OperatorActionCenterStore
	GetRunMode(context.Context, string) (domain.RunModeSnapshot, error)
	LatestRunEventSequence(context.Context, string) (int64, error)
	GetSession(context.Context, string) (session.Session, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	ListPlanDeliveryProposals(context.Context, string, int) ([]domain.PlanDeliveryProposal, error)
	GetPlanDeliveryProposal(context.Context, string) (domain.PlanDeliveryProposal, error)
	GetPlanDeliverySelectionByRun(context.Context, string) (domain.PlanDeliverySelection, bool, error)
	GetWorkItem(context.Context, string) (domain.WorkItem, error)
	GetOperatorSteeringQueueSummary(context.Context, string) (domain.OperatorSteeringQueueSummary, error)
	ListFileEditPreviewsPage(context.Context, fileedit.ListFilter, int, int) ([]fileedit.Preview, error)
	ListVerificationEvidence(context.Context, string, int) ([]verification.Evidence, error)
	ListFindingReportSummariesPage(context.Context, string, int, int) ([]domain.FindingReportSummary, error)
}

type CodeHandoffService struct {
	store CodeHandoffStore
	now   func() time.Time
}

type CodeHandoffPlan struct {
	State             string
	ProposalID        string
	SelectionID       string
	DirectionCount    int
	SelectedDirection int
	ModuleCount       int
	PendingCount      int
	InProgressCount   int
	BlockedCount      int
	CompletedCount    int
	CancelledCount    int
}

type CodeHandoffQueue struct {
	Pending   int
	Prepared  int
	Committed int
	Cancelled int
}

type CodeHandoffChangeSet struct {
	Proposed       int
	Approved       int
	Applied        int
	Denied         int
	Failed         int
	ReturnedCount  int
	TotalDiffBytes int
	Truncated      bool
}

type CodeHandoffVerificationReference struct {
	ID        string
	Outcome   verification.Outcome
	Redacted  bool
	CreatedAt time.Time
}

type CodeHandoffVerification struct {
	PassCount     int
	FailCount     int
	UnknownCount  int
	ReturnedCount int
	Truncated     bool
	References    []CodeHandoffVerificationReference
}

type CodeHandoffActionReference struct {
	ID          string
	Kind        operatoraction.Kind
	State       string
	Destination operatoraction.Destination
	AvailableAt time.Time
	DueAt       *time.Time
}

type CodeHandoffReportReference struct {
	ID           string
	Status       domain.FindingReportStatus
	FindingCount int
	CreatedAt    time.Time
}

type CodeHandoff struct {
	ProtocolVersion           string
	RunID                     string
	MissionID                 string
	SessionID                 string
	WorkspaceID               string
	RunStatus                 domain.RunStatus
	Surface                   domain.ExecutionSurface
	Phase                     domain.ExecutionPhase
	ModeRevision              int64
	GeneratedAt               time.Time
	Plan                      CodeHandoffPlan
	Queue                     CodeHandoffQueue
	ChangeSet                 CodeHandoffChangeSet
	Verification              CodeHandoffVerification
	PendingActionCount        int
	PendingActionsTruncated   bool
	PendingActions            []CodeHandoffActionReference
	ReportReferencesTruncated bool
	ReportReferences          []CodeHandoffReportReference
	Regenerable               bool
	DurableSources            bool
	PrivateBodiesIncluded     bool
	CompositeMutation         bool
	ResumeAuthorized          bool
	ExecutionStarted          bool
}

func NewCodeHandoffService(store CodeHandoffStore) *CodeHandoffService {
	return &CodeHandoffService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *CodeHandoffService) Build(ctx context.Context, runID string) (CodeHandoff, error) {
	if s == nil || s.store == nil || s.now == nil {
		return CodeHandoff{}, apperror.New(apperror.CodeFailedPrecondition,
			"Code handoff store is required")
	}
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) {
		return CodeHandoff{}, apperror.New(apperror.CodeInvalidArgument,
			"Code handoff Run identity is invalid")
	}
	for attempt := 0; attempt < maxCodeHandoffSnapshotAttempts; attempt++ {
		beforeSequence, err := s.store.LatestRunEventSequence(ctx, runID)
		if err != nil {
			return CodeHandoff{}, apperror.Normalize(err)
		}
		result, err := s.buildOnce(ctx, runID)
		if err != nil {
			return CodeHandoff{}, err
		}
		afterSequence, err := s.store.LatestRunEventSequence(ctx, runID)
		if err != nil {
			return CodeHandoff{}, apperror.Normalize(err)
		}
		if beforeSequence == afterSequence {
			return result, nil
		}
	}
	return CodeHandoff{}, apperror.New(apperror.CodeConflict,
		"Code handoff changed during bounded snapshot retries")
}

func (s *CodeHandoffService) buildOnce(ctx context.Context, runID string) (CodeHandoff, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return CodeHandoff{}, apperror.Normalize(err)
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return CodeHandoff{}, apperror.Normalize(err)
	}
	mode, err := s.store.GetRunMode(ctx, run.ID)
	if err != nil {
		return CodeHandoff{}, apperror.Normalize(err)
	}
	if run.ID != runID || mission.ID != run.MissionID || run.SessionID == "" ||
		mission.WorkspaceID == "" || mode.RunID != run.ID ||
		mode.Surface != domain.ExecutionSurfaceCode {
		return CodeHandoff{}, apperror.New(apperror.CodeFailedPrecondition,
			"Code handoff requires an exact Code Run, Session, and Workspace binding")
	}
	linkedSession, err := s.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return CodeHandoff{}, apperror.Normalize(err)
	}
	registered, err := s.store.GetWorkspaceInfo(ctx, mission.WorkspaceID)
	if err != nil {
		return CodeHandoff{}, apperror.Normalize(err)
	}
	if linkedSession.ID != run.SessionID || linkedSession.WorkspaceID != mission.WorkspaceID ||
		registered.ID != mission.WorkspaceID {
		return CodeHandoff{}, apperror.New(apperror.CodeFailedPrecondition,
			"Code handoff requires an exact Code Run, Session, and Workspace binding")
	}
	result := CodeHandoff{
		ProtocolVersion: CodeHandoffProtocolVersion, RunID: run.ID,
		MissionID: run.MissionID, SessionID: run.SessionID,
		WorkspaceID: mission.WorkspaceID, RunStatus: run.Status,
		Surface: mode.Surface, Phase: mode.Phase, ModeRevision: mode.Revision,
		GeneratedAt: s.now().UTC(), Regenerable: true, DurableSources: true,
	}
	if err := s.addPlan(ctx, &result); err != nil {
		return CodeHandoff{}, err
	}
	queue, err := s.store.GetOperatorSteeringQueueSummary(ctx, run.ID)
	if err != nil {
		return CodeHandoff{}, apperror.Normalize(err)
	}
	if queue.RunID != run.ID {
		return CodeHandoff{}, apperror.New(apperror.CodeConflict,
			"Code handoff queue escaped its Run binding")
	}
	result.Queue = CodeHandoffQueue{Pending: queue.Pending, Prepared: queue.Prepared,
		Committed: queue.Committed, Cancelled: queue.Cancelled}
	if err := s.addChangeSet(ctx, run, mission, &result); err != nil {
		return CodeHandoff{}, err
	}
	if err := s.addVerification(ctx, run, mission, &result); err != nil {
		return CodeHandoff{}, err
	}
	if err := s.addActions(ctx, &result); err != nil {
		return CodeHandoff{}, err
	}
	if err := s.addReports(ctx, &result); err != nil {
		return CodeHandoff{}, err
	}
	return result, nil
}

func (s *CodeHandoffService) addPlan(ctx context.Context, result *CodeHandoff) error {
	selection, selected, err := s.store.GetPlanDeliverySelectionByRun(ctx, result.RunID)
	if err != nil {
		return apperror.Normalize(err)
	}
	if !selected {
		proposals, err := s.store.ListPlanDeliveryProposals(ctx, result.RunID, 1)
		if err != nil {
			return apperror.Normalize(err)
		}
		result.Plan.State = "none"
		if len(proposals) == 0 {
			return nil
		}
		proposal := proposals[0]
		if proposal.RunID != result.RunID || proposal.SessionID != result.SessionID ||
			proposal.WorkspaceID != result.WorkspaceID {
			return apperror.New(apperror.CodeConflict,
				"Code handoff Plan proposal escaped its Run binding")
		}
		result.Plan.State = "proposed"
		result.Plan.ProposalID = proposal.ID
		result.Plan.DirectionCount = len(proposal.Spec.Directions)
		return nil
	}
	proposal, err := s.store.GetPlanDeliveryProposal(ctx, selection.ProposalID)
	if err != nil {
		return apperror.Normalize(err)
	}
	if selection.RunID != result.RunID || proposal.RunID != result.RunID ||
		proposal.SessionID != result.SessionID || proposal.WorkspaceID != result.WorkspaceID ||
		selection.DirectionOrdinal < 1 ||
		selection.DirectionOrdinal > len(proposal.Spec.Directions) {
		return apperror.New(apperror.CodeConflict,
			"Code handoff Plan selection escaped its Run binding")
	}
	result.Plan = CodeHandoffPlan{State: "selected", ProposalID: proposal.ID,
		SelectionID: selection.ID, DirectionCount: len(proposal.Spec.Directions),
		SelectedDirection: selection.DirectionOrdinal, ModuleCount: len(selection.Items)}
	for _, selectedItem := range selection.Items {
		item, err := s.store.GetWorkItem(ctx, selectedItem.WorkItemID)
		if err != nil {
			return apperror.Normalize(err)
		}
		if item.RunID != result.RunID {
			return apperror.New(apperror.CodeConflict,
				"Code handoff WorkItem escaped its Run binding")
		}
		switch item.Status {
		case domain.WorkItemPending:
			result.Plan.PendingCount++
		case domain.WorkItemInProgress:
			result.Plan.InProgressCount++
		case domain.WorkItemBlocked:
			result.Plan.BlockedCount++
		case domain.WorkItemCompleted:
			result.Plan.CompletedCount++
		case domain.WorkItemCancelled:
			result.Plan.CancelledCount++
		default:
			return apperror.New(apperror.CodeInternal,
				"Code handoff WorkItem status is invalid")
		}
	}
	return nil
}

func (s *CodeHandoffService) addChangeSet(ctx context.Context, run domain.Run,
	mission domain.Mission, result *CodeHandoff,
) error {
	values, err := s.store.ListFileEditPreviewsPage(ctx, fileedit.ListFilter{
		SessionID: run.SessionID, WorkspaceID: mission.WorkspaceID,
	}, 0, MaxFileEditChangeSetItems+1)
	if err != nil {
		return apperror.Normalize(err)
	}
	truncated := len(values) > MaxFileEditChangeSetItems
	if truncated {
		values = values[:MaxFileEditChangeSetItems]
	}
	changeSet, err := BuildFileEditChangeSet(run, mission, values)
	if err != nil {
		return err
	}
	result.ChangeSet = CodeHandoffChangeSet{
		Proposed: changeSet.Counts.Proposed, Approved: changeSet.Counts.Approved,
		Applied: changeSet.Counts.Applied, Denied: changeSet.Counts.Denied,
		Failed: changeSet.Counts.Failed, ReturnedCount: len(values),
		TotalDiffBytes: changeSet.TotalDiffBytes, Truncated: truncated,
	}
	return nil
}

func (s *CodeHandoffService) addVerification(ctx context.Context, run domain.Run,
	mission domain.Mission, result *CodeHandoff,
) error {
	values, err := s.store.ListVerificationEvidence(ctx, run.ID,
		verification.MaxInventoryItems+1)
	if err != nil {
		return apperror.Normalize(err)
	}
	result.Verification.Truncated = len(values) > verification.MaxInventoryItems
	if result.Verification.Truncated {
		values = values[:verification.MaxInventoryItems]
	}
	result.Verification.ReturnedCount = len(values)
	for index, value := range values {
		if value.RunID != run.ID || value.SessionID != run.SessionID ||
			value.WorkspaceID != mission.WorkspaceID {
			return apperror.New(apperror.CodeConflict,
				"Code handoff verification escaped its Run binding")
		}
		switch value.Outcome {
		case verification.OutcomePass:
			result.Verification.PassCount++
		case verification.OutcomeFail:
			result.Verification.FailCount++
		case verification.OutcomeUnknown:
			result.Verification.UnknownCount++
		}
		if index < MaxCodeHandoffVerifyReferences {
			result.Verification.References = append(result.Verification.References,
				CodeHandoffVerificationReference{ID: value.ID, Outcome: value.Outcome,
					Redacted: value.Redacted, CreatedAt: value.CreatedAt})
		}
	}
	if len(values) > MaxCodeHandoffVerifyReferences {
		result.Verification.Truncated = true
	}
	return nil
}

func (s *CodeHandoffService) addActions(ctx context.Context, result *CodeHandoff) error {
	center, err := NewOperatorActionCenterService(s.store).List(ctx, result.RunID)
	if err != nil {
		return err
	}
	result.PendingActionCount = len(center.Items)
	result.PendingActionsTruncated = center.Truncated ||
		len(center.Items) > MaxCodeHandoffActionReferences
	limit := min(len(center.Items), MaxCodeHandoffActionReferences)
	result.PendingActions = make([]CodeHandoffActionReference, limit)
	for index := 0; index < limit; index++ {
		item := center.Items[index]
		result.PendingActions[index] = CodeHandoffActionReference{
			ID: item.ID, Kind: item.Kind, State: item.State,
			Destination: item.Destination, AvailableAt: item.AvailableAt, DueAt: item.DueAt,
		}
	}
	return nil
}

func (s *CodeHandoffService) addReports(ctx context.Context, result *CodeHandoff) error {
	values, err := s.store.ListFindingReportSummariesPage(ctx, result.RunID, 0,
		MaxCodeHandoffReportReferences+1)
	if err != nil {
		return apperror.Normalize(err)
	}
	result.ReportReferencesTruncated = len(values) > MaxCodeHandoffReportReferences
	if result.ReportReferencesTruncated {
		values = values[:MaxCodeHandoffReportReferences]
	}
	result.ReportReferences = make([]CodeHandoffReportReference, len(values))
	for index, value := range values {
		if value.RunID != result.RunID || !value.Status.Valid() {
			return apperror.New(apperror.CodeConflict,
				"Code handoff report escaped its Run binding")
		}
		result.ReportReferences[index] = CodeHandoffReportReference{
			ID: value.ID, Status: value.Status, FindingCount: value.FindingCount,
			CreatedAt: value.CreatedAt,
		}
	}
	return nil
}
