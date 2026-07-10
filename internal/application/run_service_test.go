package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/store"
)

func TestRunServicePersistsLifecycleAndOrderedEvents(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	service := application.NewRunService(st)
	ctx := context.Background()
	mission, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "review code", Profile: "review", ModelRoute: "review", Budget: domain.Budget{MaxTurns: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mission.Profile != domain.ProfileReview || run.Status != domain.RunCreated {
		t.Fatalf("unexpected create result mission=%#v run=%#v", mission, run)
	}
	run, err = service.Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunRunning || run.StartedAt == nil {
		t.Fatalf("unexpected started run: %#v", run)
	}
	run, err = service.Pause(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunPaused {
		t.Fatalf("unexpected paused run: %#v", run)
	}
	events, err := service.Events(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("expected create, prepare, run, pause events; got %#v", events)
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("unexpected event sequence: %#v", events)
		}
	}
}
