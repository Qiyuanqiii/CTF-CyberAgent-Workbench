package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
)

type RunStore interface {
	CreateMissionRun(ctx context.Context, mission domain.Mission, run domain.Run, event events.Event) error
	GetMission(ctx context.Context, id string) (domain.Mission, error)
	GetRun(ctx context.Context, id string) (domain.Run, error)
	ListRuns(ctx context.Context, filter domain.RunFilter) ([]domain.Run, error)
	TransitionRun(ctx context.Context, run domain.Run, expected domain.RunStatus, event events.Event) error
	ListRunEvents(ctx context.Context, runID string) ([]events.Event, error)
}

type RunService struct {
	store RunStore
}

type CreateRunRequest struct {
	Goal        string
	Profile     string
	WorkspaceID string
	SessionID   string
	ModelRoute  string
	Interactive bool
	Budget      domain.Budget
}

func NewRunService(store RunStore) *RunService {
	return &RunService{store: store}
}

func (s *RunService) Create(ctx context.Context, req CreateRunRequest) (domain.Mission, domain.Run, error) {
	if s == nil || s.store == nil {
		return domain.Mission{}, domain.Run{}, errors.New("run store is required")
	}
	goal := redact.String(strings.TrimSpace(req.Goal))
	if goal == "" {
		return domain.Mission{}, domain.Run{}, errors.New("mission goal is required")
	}
	profileValue := strings.TrimSpace(req.Profile)
	if profileValue == "" {
		profileValue = string(domain.ProfileCode)
	}
	profile, err := domain.ParseProfile(profileValue)
	if err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	route := strings.TrimSpace(req.ModelRoute)
	if route == "" {
		route = string(profile)
	}
	budget := req.Budget
	if budget.MaxTurns == 0 {
		budget.MaxTurns = domain.DefaultBudget().MaxTurns
	}
	if err := budget.Validate(); err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	now := time.Now().UTC()
	mission := domain.Mission{
		ID:          idgen.New("mission"),
		Goal:        goal,
		Profile:     profile,
		WorkspaceID: strings.TrimSpace(req.WorkspaceID),
		Scope:       domain.DefaultScope(req.WorkspaceID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	run := domain.Run{
		ID:        idgen.New("run"),
		MissionID: mission.ID,
		SessionID: strings.TrimSpace(req.SessionID),
		Status:    domain.RunCreated,
		Config: domain.RunConfig{
			ModelRoute:  route,
			Interactive: req.Interactive,
		},
		Budget:    budget,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := mission.Validate(); err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	if err := run.Validate(); err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	event, err := events.New(run.ID, mission.ID, events.RunCreatedEvent, "run_service", run.ID, map[string]any{
		"status":       run.Status,
		"profile":      mission.Profile,
		"network_mode": mission.Scope.NetworkMode,
	})
	if err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	if err := s.store.CreateMissionRun(ctx, mission, run, event); err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	return mission, run, nil
}

func (s *RunService) Start(ctx context.Context, id string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status == domain.RunRunning {
		return run, nil
	}
	if run.Status == domain.RunCreated {
		run, err = s.transition(ctx, run, domain.RunPreparing, "start requested")
		if err != nil {
			return domain.Run{}, err
		}
	}
	return s.transition(ctx, run, domain.RunRunning, "run prepared")
}

func (s *RunService) Pause(ctx context.Context, id string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status == domain.RunPaused {
		return run, nil
	}
	return s.transition(ctx, run, domain.RunPaused, "pause requested")
}

func (s *RunService) Resume(ctx context.Context, id string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status == domain.RunRunning {
		return run, nil
	}
	return s.transition(ctx, run, domain.RunRunning, "resume requested")
}

func (s *RunService) Cancel(ctx context.Context, id string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status == domain.RunCancelled {
		return run, nil
	}
	return s.transition(ctx, run, domain.RunCancelled, "cancel requested")
}

func (s *RunService) Complete(ctx context.Context, id string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Run{}, err
	}
	return s.transition(ctx, run, domain.RunCompleted, "run completed")
}

func (s *RunService) Fail(ctx context.Context, id string, reason string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Run{}, err
	}
	return s.transition(ctx, run, domain.RunFailed, redact.String(strings.TrimSpace(reason)))
}

func (s *RunService) Get(ctx context.Context, id string) (domain.Mission, domain.Run, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return domain.Mission{}, domain.Run{}, err
	}
	return mission, run, nil
}

func (s *RunService) List(ctx context.Context, filter domain.RunFilter) ([]domain.Run, error) {
	return s.store.ListRuns(ctx, filter)
}

func (s *RunService) Events(ctx context.Context, runID string) ([]events.Event, error) {
	runID = strings.TrimSpace(runID)
	if _, err := s.store.GetRun(ctx, runID); err != nil {
		return nil, err
	}
	return s.store.ListRunEvents(ctx, runID)
}

func (s *RunService) transition(ctx context.Context, run domain.Run, target domain.RunStatus, reason string) (domain.Run, error) {
	expected := run.Status
	if expected == target {
		return run, nil
	}
	if err := run.Transition(target, time.Now().UTC()); err != nil {
		return domain.Run{}, err
	}
	event, err := events.New(run.ID, run.MissionID, events.RunStatusChangedEvent, "run_service", run.ID, map[string]any{
		"from":   expected,
		"to":     target,
		"reason": redact.String(reason),
	})
	if err != nil {
		return domain.Run{}, err
	}
	if err := s.store.TransitionRun(ctx, run, expected, event); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}
