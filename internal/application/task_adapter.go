package application

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/agent"
	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
)

type TaskAdapterStore interface {
	GetTask(ctx context.Context, id string) (agent.Task, error)
	FindTaskRunLink(ctx context.Context, taskID string) (agent.TaskRunLink, bool, error)
	CreateTaskMissionRun(ctx context.Context, source agent.Task, mission domain.Mission, run domain.Run, linkedSession session.Session, initialEvents []events.Event) (agent.TaskRunLink, bool, error)
	GetMission(ctx context.Context, id string) (domain.Mission, error)
	GetRun(ctx context.Context, id string) (domain.Run, error)
}

type TaskAdapter struct {
	store TaskAdapterStore
}

type AdaptTaskResult struct {
	Source  agent.Task
	Link    agent.TaskRunLink
	Mission domain.Mission
	Run     domain.Run
	Created bool
}

func NewTaskAdapter(store TaskAdapterStore) *TaskAdapter {
	return &TaskAdapter{store: store}
}

func (a *TaskAdapter) Adapt(ctx context.Context, taskID string) (AdaptTaskResult, error) {
	if a == nil || a.store == nil {
		return AdaptTaskResult{}, apperror.New(apperror.CodeFailedPrecondition, "task adapter store is required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return AdaptTaskResult{}, apperror.New(apperror.CodeInvalidArgument, "task id is required")
	}
	source, err := a.store.GetTask(ctx, taskID)
	if err != nil {
		return AdaptTaskResult{}, err
	}
	if link, ok, err := a.store.FindTaskRunLink(ctx, source.ID); err != nil {
		return AdaptTaskResult{}, err
	} else if ok {
		return a.loadExisting(ctx, source, link)
	}

	profile, err := legacyTaskProfile(source.Kind)
	if err != nil {
		return AdaptTaskResult{}, err
	}
	now := time.Now().UTC()
	goal := redact.String(strings.TrimSpace(source.Goal))
	if goal == "" {
		return AdaptTaskResult{}, apperror.New(apperror.CodeFailedPrecondition, "legacy task goal is required")
	}
	linkedSession := session.New(source.WorkspaceID, goal, string(profile))
	linkedSession.CreatedAt = now
	linkedSession.UpdatedAt = now
	mission := domain.Mission{
		ID:          idgen.New("mission"),
		Goal:        goal,
		Profile:     profile,
		WorkspaceID: strings.TrimSpace(source.WorkspaceID),
		Scope:       domain.DefaultScope(source.WorkspaceID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	run := domain.Run{
		ID:        idgen.New("run"),
		MissionID: mission.ID,
		SessionID: linkedSession.ID,
		Status:    domain.RunCreated,
		Config: domain.RunConfig{
			ModelRoute:  string(profile),
			Interactive: false,
		},
		Budget:    domain.DefaultBudget(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	initialEvents, err := legacyTaskEvents(source, mission, run, linkedSession)
	if err != nil {
		return AdaptTaskResult{}, err
	}
	link, created, err := a.store.CreateTaskMissionRun(ctx, source, mission, run, linkedSession, initialEvents)
	if err != nil {
		return AdaptTaskResult{}, err
	}
	if !created {
		return a.loadExisting(ctx, source, link)
	}
	return AdaptTaskResult{Source: source, Link: link, Mission: mission, Run: run, Created: true}, nil
}

func (a *TaskAdapter) loadExisting(ctx context.Context, source agent.Task, link agent.TaskRunLink) (AdaptTaskResult, error) {
	mission, err := a.store.GetMission(ctx, link.MissionID)
	if err != nil {
		return AdaptTaskResult{}, apperror.Wrap(apperror.CodeFailedPrecondition, "legacy task mission mapping could not be loaded", err)
	}
	run, err := a.store.GetRun(ctx, link.RunID)
	if err != nil {
		return AdaptTaskResult{}, apperror.Wrap(apperror.CodeFailedPrecondition, "legacy task run mapping could not be loaded", err)
	}
	return AdaptTaskResult{Source: source, Link: link, Mission: mission, Run: run, Created: false}, nil
}

func legacyTaskProfile(kind agent.TaskKind) (domain.Profile, error) {
	switch kind {
	case agent.TaskScript:
		return domain.ProfileScript, nil
	case agent.TaskCode:
		return domain.ProfileCode, nil
	case agent.TaskLearn:
		return domain.ProfileLearn, nil
	case agent.TaskReview:
		return domain.ProfileReview, nil
	case agent.TaskCTF:
		return domain.ProfileReview, nil
	default:
		return "", apperror.New(apperror.CodeFailedPrecondition, "legacy task kind is not supported")
	}
}

func legacyTaskEvents(source agent.Task, mission domain.Mission, run domain.Run, linkedSession session.Session) ([]events.Event, error) {
	created, err := events.New(run.ID, mission.ID, events.RunCreatedEvent, "task_adapter", run.ID, map[string]any{
		"status":         run.Status,
		"profile":        mission.Profile,
		"network_mode":   mission.Scope.NetworkMode,
		"session_id":     run.SessionID,
		"legacy_task_id": source.ID,
	})
	if err != nil {
		return nil, err
	}
	attached, err := events.New(run.ID, mission.ID, events.SessionAttachedEvent, "task_adapter", linkedSession.ID, map[string]any{
		"created":      true,
		"route":        linkedSession.Route,
		"workspace_id": linkedSession.WorkspaceID,
	})
	if err != nil {
		return nil, err
	}
	adapted, err := events.New(run.ID, mission.ID, events.LegacyTaskAdaptedEvent, "task_adapter", source.ID, map[string]any{
		"task_id":        source.ID,
		"task_kind":      source.Kind,
		"task_mode":      source.Mode,
		"task_status":    source.Status,
		"mapped_profile": mission.Profile,
	})
	if err != nil {
		return nil, err
	}
	return []events.Event{created, attached, adapted}, nil
}
