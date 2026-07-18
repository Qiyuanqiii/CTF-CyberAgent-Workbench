package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/operatoraction"
)

type operatorActionCenterTestStore struct {
	run     domain.Run
	mission domain.Mission
	records []operatoraction.Record
}

func (s *operatorActionCenterTestStore) GetRun(_ context.Context,
	id string,
) (domain.Run, error) {
	if id != s.run.ID {
		return domain.Run{}, errors.New("Run not found")
	}
	return s.run, nil
}

func (s *operatorActionCenterTestStore) GetMission(_ context.Context,
	id string,
) (domain.Mission, error) {
	if id != s.mission.ID {
		return domain.Mission{}, errors.New("Mission not found")
	}
	return s.mission, nil
}

func (s *operatorActionCenterTestStore) ListOperatorActionRecords(_ context.Context,
	_, _, _ string, _ time.Time, _ int,
) ([]operatoraction.Record, error) {
	return append([]operatoraction.Record(nil), s.records...), nil
}

func TestOperatorActionCenterProjectsOpaqueBoundedNavigation(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	privateID := "PRIVATE-steering-message"
	state := &operatorActionCenterTestStore{
		run:     domain.Run{ID: "run-actions", MissionID: "mission-actions", SessionID: "session-actions"},
		mission: domain.Mission{ID: "mission-actions", WorkspaceID: "workspace-actions"},
		records: []operatoraction.Record{{SourceID: privateID,
			Kind: operatoraction.KindSteeringPending, State: "pending", RunID: "run-actions",
			SessionID: "session-actions", AvailableAt: now.Add(-time.Minute)}},
	}
	service := NewOperatorActionCenterService(state)
	service.now = func() time.Time { return now }
	center, err := service.List(t.Context(), state.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if center.ProtocolVersion != operatoraction.ProtocolVersion || center.Truncated ||
		len(center.Items) != 1 || center.Items[0].Destination != operatoraction.DestinationQueue ||
		!strings.HasPrefix(center.Items[0].ID, "action-") {
		t.Fatalf("unexpected operator action center: %#v", center)
	}
	raw, err := json.Marshal(center)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), privateID) || strings.Contains(string(raw), "workspace-actions") ||
		strings.Contains(string(raw), "session-actions") {
		t.Fatalf("operator action center leaked private binding metadata: %s", raw)
	}
	if _, err := service.List(t.Context(), " run-actions"); err == nil {
		t.Fatal("non-normalized Run identity was accepted")
	}
}

func TestOperatorActionCenterRejectsCrossRunAndFutureWakeRecords(t *testing.T) {
	now := time.Now().UTC()
	due := now.Add(time.Minute)
	state := &operatorActionCenterTestStore{
		run:     domain.Run{ID: "run-actions", MissionID: "mission-actions", SessionID: "session-actions"},
		mission: domain.Mission{ID: "mission-actions", WorkspaceID: "workspace-actions"},
		records: []operatoraction.Record{{SourceID: "wake-other", Kind: operatoraction.KindWakeDue,
			State: "queued", RunID: "run-actions", SessionID: "session-actions",
			AvailableAt: due, DueAt: &due}},
	}
	service := NewOperatorActionCenterService(state)
	service.now = func() time.Time { return now }
	if _, err := service.List(t.Context(), state.run.ID); err == nil {
		t.Fatal("future wake record was accepted as due")
	}
	state.records[0].DueAt = &now
	state.records[0].RunID = "run-other"
	if _, err := service.List(t.Context(), state.run.ID); err == nil {
		t.Fatal("cross-Run operator action was accepted")
	}
}
