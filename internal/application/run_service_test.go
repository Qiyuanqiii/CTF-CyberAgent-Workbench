package application_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
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
	if mission.Profile != domain.ProfileReview || run.Status != domain.RunCreated || run.SessionID == "" {
		t.Fatalf("unexpected create result mission=%#v run=%#v", mission, run)
	}
	linkedSession, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if linkedSession.Route != "review" || linkedSession.Title != "review code" {
		t.Fatalf("unexpected auto-created session: %#v", linkedSession)
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
	if len(events) != 5 {
		t.Fatalf("expected create, session attach, prepare, run, pause events; got %#v", events)
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("unexpected event sequence: %#v", events)
		}
	}
}

func TestRunServiceReusesOneActiveSessionOnce(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	existing := session.New("", "existing", "learn")
	if err := st.SaveSession(ctx, existing); err != nil {
		t.Fatal(err)
	}
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "review existing session", Profile: "review", WorkspaceID: "ws-demo",
		SessionID: existing.ID, ModelRoute: "review", Budget: domain.Budget{MaxTurns: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID != existing.ID {
		t.Fatalf("run did not reuse session: %#v", run)
	}
	updated, err := st.GetSession(ctx, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.WorkspaceID != "ws-demo" || updated.Route != "review" {
		t.Fatalf("session binding was not normalized: %#v", updated)
	}
	items, err := service.Events(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Type != events.RunCreatedEvent || items[1].Type != events.SessionAttachedEvent || !strings.Contains(items[1].PayloadJSON, `"created":false`) {
		t.Fatalf("unexpected initial timeline: %#v", items)
	}
	if _, _, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "reuse twice", Profile: "review", SessionID: existing.ID, Budget: domain.Budget{MaxTurns: 5},
	}); err == nil {
		t.Fatal("expected a session to be rejected when already attached to a run")
	}
	runs, err := service.List(ctx, domain.RunFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("failed duplicate attach left partial run state: %#v", runs)
	}
}
