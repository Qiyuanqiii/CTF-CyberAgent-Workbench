package application

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
)

type ControlledRunCreationStore interface {
	GetMission(context.Context, string) (domain.Mission, error)
	GetRun(context.Context, string) (domain.Run, error)
	GetRunMode(context.Context, string) (domain.RunModeSnapshot, error)
	GetSession(context.Context, string) (session.Session, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	GetRunCreationOperation(context.Context, string) (domain.RunCreationOperation, bool, error)
	CreateMissionRunWithOperation(context.Context, domain.Mission, domain.Run,
		domain.RunModeSnapshot, session.Session, []events.Event,
		domain.RunCreationOperation) (domain.RunCreationOperation, bool, error)
}

type ControlledRunCreationService struct {
	store ControlledRunCreationStore
}

type ControlledRunCreationRequest struct {
	Version      string
	Goal         string
	WorkspaceID  string
	Profile      string
	Surface      string
	Phase        string
	OperationKey string
	RequestedBy  string
}

type ControlledRunCreationResult struct {
	Mission  domain.Mission
	Run      domain.Run
	Session  session.Session
	Mode     domain.RunModeSnapshot
	Replayed bool
}

type normalizedControlledRunCreationRequest struct {
	Goal         string
	WorkspaceID  string
	Profile      domain.Profile
	Surface      domain.ExecutionSurface
	Phase        domain.ExecutionPhase
	OperationKey string
	RequestedBy  string
}

func NewControlledRunCreationService(store ControlledRunCreationStore) *ControlledRunCreationService {
	return &ControlledRunCreationService{store: store}
}

func (s *ControlledRunCreationService) Create(ctx context.Context,
	request ControlledRunCreationRequest,
) (ControlledRunCreationResult, error) {
	if s == nil || s.store == nil {
		return ControlledRunCreationResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "controlled Run creation store is required")
	}
	normalized, err := normalizeControlledRunCreationRequest(request)
	if err != nil {
		return ControlledRunCreationResult{}, err
	}
	keyDigest := runmutation.RunCreationOperationDigest(normalized.OperationKey)
	requestFingerprint := runmutation.RunCreationRequestFingerprint(normalized.Goal,
		normalized.WorkspaceID, string(normalized.Profile), string(normalized.Surface),
		string(normalized.Phase), normalized.RequestedBy)

	if existing, found, err := s.store.GetRunCreationOperation(ctx, keyDigest); err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	} else if found {
		if err := validateControlledRunCreationIntent(existing, requestFingerprint,
			normalized.WorkspaceID, normalized.RequestedBy); err != nil {
			return ControlledRunCreationResult{}, err
		}
		return s.loadResult(ctx, existing, true)
	}

	workspace, err := s.store.GetWorkspaceInfo(ctx, normalized.WorkspaceID)
	if err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	}
	if workspace.ID != normalized.WorkspaceID || strings.TrimSpace(workspace.Name) == "" ||
		strings.TrimSpace(workspace.RootPath) == "" {
		return ControlledRunCreationResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "controlled Run workspace record is invalid")
	}

	prepared, err := prepareRun(ctx, CreateRunRequest{
		Goal: normalized.Goal, Profile: string(normalized.Profile),
		Surface: string(normalized.Surface), Phase: string(normalized.Phase),
		WorkspaceID: normalized.WorkspaceID, ModelRoute: string(normalized.Profile),
		Interactive: true, Budget: domain.DefaultBudget(), RequestedBy: normalized.RequestedBy,
	}, nil)
	if err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	}
	operation := domain.RunCreationOperation{
		ProtocolVersion: domain.RunCreationProtocolVersion,
		KeyDigest:       keyDigest, RequestFingerprint: requestFingerprint,
		MissionID: prepared.Mission.ID, RunID: prepared.Run.ID,
		SessionID: prepared.Session.ID, WorkspaceID: normalized.WorkspaceID,
		RequestedBy: normalized.RequestedBy, CreatedAt: prepared.Run.CreatedAt,
	}
	stored, replayed, err := s.store.CreateMissionRunWithOperation(ctx,
		prepared.Mission, prepared.Run, prepared.Mode, prepared.Session,
		prepared.InitialEvents, operation)
	if err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	}
	if err := validateControlledRunCreationIntent(stored, requestFingerprint,
		normalized.WorkspaceID, normalized.RequestedBy); err != nil {
		return ControlledRunCreationResult{}, err
	}
	if replayed {
		return s.loadResult(ctx, stored, true)
	}
	if stored.MissionID != prepared.Mission.ID || stored.RunID != prepared.Run.ID ||
		stored.SessionID != prepared.Session.ID {
		return ControlledRunCreationResult{}, apperror.New(
			apperror.CodeConflict, "controlled Run creation identity changed during commit")
	}
	return ControlledRunCreationResult{Mission: prepared.Mission, Run: prepared.Run,
		Session: prepared.Session, Mode: prepared.Mode}, nil
}

func (s *ControlledRunCreationService) loadResult(ctx context.Context,
	operation domain.RunCreationOperation, replayed bool,
) (ControlledRunCreationResult, error) {
	mission, err := s.store.GetMission(ctx, operation.MissionID)
	if err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	}
	run, err := s.store.GetRun(ctx, operation.RunID)
	if err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	}
	linkedSession, err := s.store.GetSession(ctx, operation.SessionID)
	if err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	}
	mode, err := s.store.GetRunMode(ctx, operation.RunID)
	if err != nil {
		return ControlledRunCreationResult{}, apperror.Normalize(err)
	}
	if run.MissionID != mission.ID || run.SessionID != linkedSession.ID ||
		mission.ID != operation.MissionID || mission.WorkspaceID != operation.WorkspaceID ||
		linkedSession.WorkspaceID != operation.WorkspaceID || mode.RunID != run.ID ||
		mode.MissionID != mission.ID || mode.Profile != mission.Profile ||
		mode.Revision != 1 || mode.RequestedBy != operation.RequestedBy ||
		mode.Scope.WorkspaceID != operation.WorkspaceID ||
		mode.Scope.NetworkMode != "disabled" || len(mode.Scope.AllowedTargets) != 0 ||
		mission.Scope.WorkspaceID != operation.WorkspaceID ||
		mission.Scope.NetworkMode != "disabled" || len(mission.Scope.AllowedTargets) != 0 ||
		run.Status != domain.RunCreated || run.StartedAt != nil || run.FinishedAt != nil ||
		linkedSession.Status != session.StatusActive || linkedSession.Title != mission.Goal ||
		!run.Config.Interactive || run.Config.ModelRoute != string(mission.Profile) ||
		run.Budget != domain.DefaultBudget() || linkedSession.Route != string(mission.Profile) ||
		!mission.CreatedAt.Equal(operation.CreatedAt) ||
		!mission.UpdatedAt.Equal(operation.CreatedAt) ||
		!run.CreatedAt.Equal(operation.CreatedAt) || !run.UpdatedAt.Equal(operation.CreatedAt) ||
		!linkedSession.CreatedAt.Equal(operation.CreatedAt) ||
		!linkedSession.UpdatedAt.Equal(operation.CreatedAt) ||
		!mode.CreatedAt.Equal(operation.CreatedAt) ||
		operation.RequestFingerprint != runmutation.RunCreationRequestFingerprint(
			mission.Goal, mission.WorkspaceID, string(mission.Profile), string(mode.Surface),
			string(mode.Phase), operation.RequestedBy) {
		return ControlledRunCreationResult{}, apperror.New(
			apperror.CodeConflict, "stored controlled Run creation binding is inconsistent")
	}
	return ControlledRunCreationResult{Mission: mission, Run: run,
		Session: linkedSession, Mode: mode, Replayed: replayed}, nil
}

func normalizeControlledRunCreationRequest(request ControlledRunCreationRequest) (
	normalizedControlledRunCreationRequest, error,
) {
	if request.Version != domain.RunCreationProtocolVersion {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "unsupported controlled Run creation version")
	}
	if !utf8.ValidString(request.Goal) || strings.ContainsRune(request.Goal, 0) {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run goal must be valid UTF-8 without NUL bytes")
	}
	rawGoal := strings.TrimSpace(request.Goal)
	if rawGoal == "" || len([]byte(rawGoal)) > domain.MaxRunCreationGoalBytes {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run goal must contain between 1 and 4096 bytes")
	}
	goal := redact.String(rawGoal)
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	if workspaceID != request.WorkspaceID || !domain.ValidAgentID(workspaceID) ||
		strings.ContainsRune(workspaceID, 0) {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run workspace id is invalid")
	}
	profileValue := strings.TrimSpace(request.Profile)
	if profileValue == "" {
		profileValue = string(domain.ProfileCode)
	}
	profile, err := domain.ParseProfile(profileValue)
	if err != nil {
		return normalizedControlledRunCreationRequest{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Run profile is invalid", err)
	}
	if request.Profile != "" && request.Profile != string(profile) {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run profile must use its canonical value")
	}
	surfaceValue := strings.TrimSpace(request.Surface)
	if surfaceValue == "" {
		surfaceValue = string(domain.ExecutionSurfaceCode)
	}
	surface, err := domain.ParseExecutionSurface(surfaceValue)
	if err != nil {
		return normalizedControlledRunCreationRequest{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Run surface is invalid", err)
	}
	if request.Surface != "" && request.Surface != string(surface) {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run surface must use its canonical value")
	}
	phaseValue := strings.TrimSpace(request.Phase)
	if phaseValue == "" {
		phaseValue = string(domain.ExecutionPhaseDeliver)
	}
	phase, err := domain.ParseExecutionPhase(phaseValue)
	if err != nil {
		return normalizedControlledRunCreationRequest{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Run phase is invalid", err)
	}
	if request.Phase != "" && request.Phase != string(phase) {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run phase must use its canonical value")
	}
	operationKey, err := domain.NormalizeAgentOperationKey(request.OperationKey)
	if err != nil || containsSpaceOrControl(operationKey) {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run creation idempotency key is invalid")
	}
	requestedBy := redact.String(strings.TrimSpace(request.RequestedBy))
	if requestedBy == "" {
		requestedBy = "http_control"
	}
	if !domain.ValidAgentID(requestedBy) || strings.ContainsRune(requestedBy, 0) {
		return normalizedControlledRunCreationRequest{}, apperror.New(
			apperror.CodeInvalidArgument, "Run creation requester is invalid")
	}
	return normalizedControlledRunCreationRequest{Goal: goal, WorkspaceID: workspaceID,
		Profile: profile, Surface: surface, Phase: phase, OperationKey: operationKey,
		RequestedBy: requestedBy}, nil
}

func validateControlledRunCreationIntent(operation domain.RunCreationOperation,
	requestFingerprint string, workspaceID string, requestedBy string,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeConflict,
			"stored Run creation operation is invalid", err)
	}
	if operation.RequestFingerprint != requestFingerprint ||
		operation.WorkspaceID != workspaceID || operation.RequestedBy != requestedBy {
		return apperror.New(apperror.CodeConflict,
			"Run creation idempotency key was already used for a different request")
	}
	return nil
}

func containsSpaceOrControl(value string) bool {
	for _, current := range value {
		if unicode.IsSpace(current) || unicode.IsControl(current) {
			return true
		}
	}
	return false
}
