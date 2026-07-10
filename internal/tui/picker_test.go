package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

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
