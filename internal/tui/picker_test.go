package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolrun"
	"cyberagent-workbench/internal/workspace"
)

func TestPickerSnapshotAndOpenSession(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	if _, err := sessionManager.Create(context.Background(), "ws-demo", "demo", "learn"); err != nil {
		t.Fatal(err)
	}
	picker, err := NewPicker(context.Background(), sessionManager, toolManager, "ws-demo", "new", "script")
	if err != nil {
		t.Fatal(err)
	}
	controller := &fakeActiveCallController{}
	picker.WithActiveCallController(controller)
	snapshot := picker.Snapshot()
	if !strings.Contains(snapshot, "Sessions") || !strings.Contains(snapshot, "demo") {
		t.Fatalf("unexpected picker snapshot:\n%s", snapshot)
	}
	model, err := picker.SelectedSessionModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if model.session.Title != "demo" {
		t.Fatalf("expected selected session, got %#v", model.session)
	}
	if model.activeCalls != controller {
		t.Fatal("picker did not pass its active-call controller to the selected session")
	}
}

func TestPickerCreatesNewSession(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	picker, err := NewPicker(context.Background(), sessionManager, toolManager, "ws-demo", "new title", "script")
	if err != nil {
		t.Fatal(err)
	}
	model, err := picker.NewSessionModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if model.session.Title != "new title" || model.session.Route != "script" || model.session.WorkspaceID != "ws-demo" {
		t.Fatalf("unexpected new session: %#v", model.session)
	}
}

func TestPickerPassesWorkspaceStoreToNewModel(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	workspaceManager := workspace.NewManager(home, st)
	rec, err := workspaceManager.Init(context.Background(), "Demo Workspace")
	if err != nil {
		t.Fatal(err)
	}
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	picker, err := NewPicker(context.Background(), sessionManager, toolManager, rec.ID, "new title", "script", st)
	if err != nil {
		t.Fatal(err)
	}
	model, err := picker.NewSessionModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if model.workspace.Name != rec.Name || model.workspace.RootPath != rec.RootPath {
		t.Fatalf("expected workspace context from picker, got %#v", model.workspace)
	}
}

func TestPickerDefaultsToBoundedRunListAndOpensExactRun(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-run-picker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec, err := workspace.NewManager(home, st).Init(ctx, "Run Picker")
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "inspect the selected Run", Profile: "review", WorkspaceID: rec.ID,
		Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	picker, err := NewPicker(ctx, sessionManager, toolManager, rec.ID, "new", "review", st)
	if err != nil {
		t.Fatal(err)
	}
	controller := &fakeActiveCallController{}
	picker.WithActiveCallController(controller)
	if picker.view != pickerRuns || len(picker.runs) != 1 {
		t.Fatalf("picker did not default to the bounded Run list: %#v", picker)
	}
	pickerProjection := picker.CurrentProjection()
	if pickerProjection.View != "runs" || len(pickerProjection.Runs) != 1 ||
		pickerProjection.Runs[0].RunID != run.ID ||
		pickerProjection.Runs[0].SessionID != run.SessionID ||
		len(pickerProjection.Sessions) != 1 ||
		pickerProjection.Sessions[0].SessionID != run.SessionID ||
		pickerProjection.RunsTruncated || pickerProjection.SessionsTruncated {
		t.Fatalf("picker projection drifted: %#v", pickerProjection)
	}
	pickerProjection.Runs[0].RunID = "mutated"
	if current := picker.CurrentProjection(); current.Runs[0].RunID != run.ID {
		t.Fatal("picker projection leaked mutable internal state")
	}
	snapshot := picker.Snapshot()
	for _, want := range []string{"[Runs] Sessions", run.ID, "created", "inspect the selected Run"} {
		if !strings.Contains(snapshot, want) {
			t.Fatalf("Run picker snapshot missing %q:\n%s", want, snapshot)
		}
	}
	model, err := picker.SelectedRunModel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	projection, found := model.CurrentRunProjection()
	if !found || projection.RunID != run.ID || projection.SessionID != run.SessionID ||
		model.activeCalls != controller {
		t.Fatalf("selected Run resolved incorrectly: projection=%#v found=%t", projection, found)
	}
	picker.ToggleView()
	if picker.view != pickerSessions || !strings.Contains(picker.Snapshot(), "Runs [Sessions]") {
		t.Fatalf("picker did not switch to Sessions:\n%s", picker.Snapshot())
	}
}
