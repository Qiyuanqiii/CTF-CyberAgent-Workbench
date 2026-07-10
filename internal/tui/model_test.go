package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolrun"
	"cyberagent-workbench/internal/workspace"
)

func TestModelSubmitCreatesAndApprovesToolRun(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	sess, err := sessionManager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	model, err := NewModel(context.Background(), sess, sessionManager, toolManager)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Submit(context.Background(), "/run echo hello"); err != nil {
		t.Fatal(err)
	}
	if len(model.toolRuns) != 1 || model.toolRuns[0].Status != toolrun.StatusProposed {
		t.Fatalf("unexpected tool runs: %#v", model.toolRuns)
	}
	if err := model.Submit(context.Background(), "/approve "+model.toolRuns[0].ID); err != nil {
		t.Fatal(err)
	}
	if model.toolRuns[0].Status != toolrun.StatusCompleted {
		t.Fatalf("expected completed tool run, got %#v", model.toolRuns[0])
	}
	if !strings.Contains(model.Snapshot(), "dry run: echo hello") {
		t.Fatalf("snapshot did not render approved output:\n%s", model.Snapshot())
	}
}

func TestModelSelectsAndApprovesFocusedTool(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	sess, err := sessionManager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	model, err := NewModel(context.Background(), sess, sessionManager, toolManager)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Submit(context.Background(), "/run echo first"); err != nil {
		t.Fatal(err)
	}
	if err := model.Submit(context.Background(), "/run echo second"); err != nil {
		t.Fatal(err)
	}
	model.ToggleFocus()
	if model.focus != focusTools {
		t.Fatalf("expected tool focus")
	}
	model.SelectNextTool()
	if model.selectedTool != 1 {
		t.Fatalf("expected second tool selected, got %d", model.selectedTool)
	}
	selectedCommand := model.toolRuns[model.selectedTool].Command
	if err := model.ApproveSelectedTool(context.Background()); err != nil {
		t.Fatal(err)
	}
	var completed bool
	for _, run := range model.toolRuns {
		if run.Command == selectedCommand && run.Status == toolrun.StatusCompleted {
			completed = true
		}
	}
	if !completed {
		t.Fatalf("expected selected tool to complete, got %#v", model.toolRuns)
	}
	snapshot := model.Snapshot()
	if !strings.Contains(snapshot, "Tool Runs (focused)") {
		t.Fatalf("snapshot did not show tool focus:\n%s", snapshot)
	}
	if !strings.Contains(snapshot, "> completed") {
		t.Fatalf("snapshot did not show selected completed tool:\n%s", snapshot)
	}
}

func TestModelDeniesSelectedTool(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	sess, err := sessionManager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	model, err := NewModel(context.Background(), sess, sessionManager, toolManager)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Submit(context.Background(), "/run echo nope"); err != nil {
		t.Fatal(err)
	}
	if err := model.DenySelectedTool(context.Background(), "not needed"); err != nil {
		t.Fatal(err)
	}
	if model.toolRuns[0].Status != toolrun.StatusDenied {
		t.Fatalf("expected denied tool, got %#v", model.toolRuns[0])
	}
}

func TestModelEnterSubmitsAsyncAction(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	sess, err := sessionManager.Create(context.Background(), "ws-demo", "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	model, err := NewModel(context.Background(), sess, sessionManager, toolManager)
	if err != nil {
		t.Fatal(err)
	}

	model.input.SetValue("/run echo async")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*Model)
	if cmd == nil {
		t.Fatal("expected async submit command")
	}
	if !model.busy {
		t.Fatal("expected model to enter busy state")
	}
	if !strings.Contains(model.statusLine(), "proposing tool") || !strings.Contains(model.statusLine(), "busy") {
		t.Fatalf("unexpected busy status: %s", model.statusLine())
	}

	msg := cmd()
	done, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("expected actionDoneMsg, got %T", msg)
	}
	updated, cmd = model.Update(done)
	model = updated.(*Model)
	if cmd != nil {
		t.Fatalf("expected no follow-up command, got %#v", cmd)
	}
	if model.busy {
		t.Fatal("expected busy state to clear")
	}
	if len(model.toolRuns) != 1 || model.toolRuns[0].Status != toolrun.StatusProposed {
		t.Fatalf("unexpected tool runs after async submit: %#v", model.toolRuns)
	}
	if !strings.Contains(model.status, "tool proposed") {
		t.Fatalf("unexpected status after async submit: %s", model.status)
	}
}

func TestModelRendersWorkspaceContext(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(rec.RootPath, "scripts", "example.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	sess, err := sessionManager.Create(context.Background(), rec.ID, "demo", "learn")
	if err != nil {
		t.Fatal(err)
	}
	model, err := NewModel(context.Background(), sess, sessionManager, toolManager, st)
	if err != nil {
		t.Fatal(err)
	}

	model.width = 140
	snapshot := model.Snapshot()
	for _, want := range []string{"Workspace", "name: demo-workspace", "scripts: 1"} {
		if !strings.Contains(snapshot, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, snapshot)
		}
	}
}
