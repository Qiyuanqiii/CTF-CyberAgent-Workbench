package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/skills"
)

func TestSkillSelectionReplaySurvivesRegistryDrift(t *testing.T) {
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	selection, err := registry.ResolveSelection(skills.ResolveSelectionRequest{
		SelectionID: "skill-selection-existing", RunID: "run-existing",
		MissionID: "mission-existing", Profile: domain.ProfileCode,
		Names: []string{"code"}, TokenBudget: skills.DefaultSelectionTokenBudget,
		RequestedBy: "cli_operator", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	keyDigest := runmutation.Fingerprint("skill_selection_operation.v1",
		selection.RunID, "replay-key-000001")
	operation := skills.SelectionOperation{
		KeyDigest: keyDigest, RequestFingerprint: skills.SelectionRequestFingerprint(selection),
		SelectionID: selection.ID, RunID: selection.RunID,
		RequestedBy: selection.RequestedBy, CreatedAt: selection.CreatedAt,
	}
	store := &skillSelectionReplayStore{
		run: domain.Run{ID: selection.RunID, MissionID: selection.MissionID,
			Status: domain.RunRunning},
		mission:   domain.Mission{ID: selection.MissionID, Profile: selection.Profile},
		selection: selection, operation: operation,
	}

	// An empty Registry represents a later binary where the pinned Skill is no
	// longer resolvable. Exact operation replay must still return stored state.
	service := NewSkillSelectionService(store, &skills.Registry{})
	result, err := service.Select(context.Background(), SelectSkillsRequest{
		RunID: selection.RunID, Names: []string{"code"},
		TokenBudget: selection.TokenBudget, OperationKey: "replay-key-000001",
		RequestedBy: selection.RequestedBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replayed || result.Selection.Fingerprint != selection.Fingerprint ||
		store.createCalls != 0 {
		t.Fatalf("unexpected replay result: %#v create_calls=%d", result, store.createCalls)
	}

	_, err = service.Select(context.Background(), SelectSkillsRequest{
		RunID: selection.RunID, Names: []string{"review"},
		TokenBudget: selection.TokenBudget, OperationKey: "replay-key-000001",
		RequestedBy: selection.RequestedBy,
	})
	if err == nil {
		t.Fatal("changed intent reused an existing operation key")
	}

	store.operation.CreatedAt = store.operation.CreatedAt.Add(time.Second)
	_, err = service.Select(context.Background(), SelectSkillsRequest{
		RunID: selection.RunID, Names: []string{"code"},
		TokenBudget: selection.TokenBudget, OperationKey: "replay-key-000001",
		RequestedBy: selection.RequestedBy,
	})
	if err == nil {
		t.Fatal("drifted operation timestamp replayed stored selection")
	}
}

type skillSelectionReplayStore struct {
	run         domain.Run
	mission     domain.Mission
	selection   skills.Selection
	operation   skills.SelectionOperation
	createCalls int
}

func (s *skillSelectionReplayStore) GetMission(context.Context, string) (domain.Mission, error) {
	return s.mission, nil
}

func (s *skillSelectionReplayStore) GetRun(context.Context, string) (domain.Run, error) {
	return s.run, nil
}

func (s *skillSelectionReplayStore) GetSkillSelection(context.Context, string) (skills.Selection, error) {
	return skills.CloneSelection(s.selection), nil
}

func (s *skillSelectionReplayStore) GetSkillSelectionByRun(context.Context,
	string,
) (skills.Selection, bool, error) {
	return skills.CloneSelection(s.selection), true, nil
}

func (s *skillSelectionReplayStore) GetSkillSelectionOperation(context.Context,
	string,
) (skills.SelectionOperation, bool, error) {
	return s.operation, true, nil
}

func (s *skillSelectionReplayStore) CreateSkillSelection(context.Context,
	skills.Selection, skills.SelectionOperation, events.Event,
) (skills.Selection, bool, error) {
	s.createCalls++
	return skills.Selection{}, false, errors.New("unexpected Skill selection creation")
}
