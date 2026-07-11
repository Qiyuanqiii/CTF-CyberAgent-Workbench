package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
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
	toolManager := toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns()
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
	toolManager := toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns()
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
	toolManager := toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns()
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
	toolManager := toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns()
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
	toolManager := toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns()
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

func TestModelRendersRunWorkNotesAndSupervisorRounds(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-run-state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	workspaceManager := workspace.NewManager(t.TempDir(), st)
	rec, err := workspaceManager.Init(ctx, "TUI Run State")
	if err != nil {
		t.Fatal(err)
	}
	runService := application.NewRunService(st)
	_, run, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "inspect durable run state", Profile: "code", WorkspaceID: rec.ID,
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runService.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewWorkItemService(st).Create(ctx, application.CreateWorkItemRequest{
		RunID: run.ID, Title: "检查中文工作项", Description: "render it", Priority: "high", Owner: "root",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewNoteService(st).Create(ctx, application.CreateNoteRequest{
		RunID: run.ID, Title: "持久化观察", Content: "这是一条可恢复的中文记录", Category: "observation",
		Visibility: "root", Pinned: true,
	}); err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, run.ID, "record one pending tool round")
	if err != nil {
		t.Fatal(err)
	}
	rawPayload := json.RawMessage(`{"title":"round note","content":"pending"}`)
	normalizedPayload, err := toolgateway.NormalizeStructuredMemoryPayload(toolgateway.NoteCreateTool, rawPayload)
	if err != nil {
		t.Fatal(err)
	}
	operationKey := runmutation.SupervisorToolOperationKey(run.ID, turn.Checkpoint.NextTurn,
		string(toolgateway.NoteCreateTool), string(normalizedPayload))
	callID, err := runmutation.SupervisorToolCallID(operationKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test", Model: "model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt); err != nil || !inserted {
		t.Fatalf("model start failed: inserted=%t err=%v", inserted, err)
	}
	completed := attempt
	completed.Outcome = llm.OutcomeSuccess
	if _, err := st.RecordSupervisorModelCompleted(ctx, turn.Checkpoint, completed, llm.ChatResponse{
		Provider: "test", Model: "model", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		ToolCalls: []llm.ToolCall{{ID: callID, Name: string(toolgateway.NoteCreateTool), Arguments: rawPayload}},
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	model, err := NewModel(ctx, sess, sessionManager,
		toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns(), st)
	if err != nil {
		t.Fatal(err)
	}
	model.width = 180
	model.height = 40
	model.ToggleFocus()
	if snapshot := model.Snapshot(); !strings.Contains(snapshot, "run="+run.ID) ||
		!strings.Contains(snapshot, "focus=activity:tools") {
		t.Fatalf("Run header or activity focus is missing:\n%s", snapshot)
	}
	for view, wants := range map[activityView][]string{
		activityWork:   {"Work Board 1", "检查中文工作项", "owner=root"},
		activityNotes:  {"Notes 1", "持久化观察", "这是一条可恢复的中文记录"},
		activityRounds: {"Tool Rounds 1", "pending", "note_create"},
	} {
		model.setActivityView(view)
		snapshot := model.Snapshot()
		if !utf8.ValidString(snapshot) {
			t.Fatalf("%s snapshot contains invalid UTF-8", view)
		}
		for _, want := range wants {
			if !strings.Contains(snapshot, want) {
				t.Fatalf("%s snapshot missing %q:\n%s", view, want, snapshot)
			}
		}
	}
	if got := truncate("你好世界", 3); ansi.StringWidth(got) > 3 || !utf8.ValidString(got) {
		t.Fatalf("Unicode truncation is unsafe: %q", got)
	}
}

func TestModelApprovesSafeShellsForSessionWithoutBypassingPolicy(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-session-grant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	workspaceManager := workspace.NewManager(t.TempDir(), st)
	rec, err := workspaceManager.Init(ctx, "TUI Session Grant")
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "approve safe shell commands", Profile: "code", WorkspaceID: rec.ID,
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	model, err := NewModel(ctx, sess, sessionManager,
		toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns(), st)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Submit(ctx, "/approve-session"); err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("missing proposal id was not rejected: %v", err)
	}
	for _, input := range []string{"/approve", "/deny"} {
		if err := model.Submit(ctx, input); err == nil {
			t.Fatalf("malformed approval command %q was not rejected", input)
		}
	}
	if err := model.Submit(ctx, "/run echo first"); err != nil {
		t.Fatal(err)
	}
	model.ToggleFocus()
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(*Model)
	if cmd == nil || !model.busy {
		t.Fatalf("g did not start session approval: busy=%t cmd=%#v", model.busy, cmd)
	}
	updated, cmd = model.Update(cmd())
	model = updated.(*Model)
	if cmd != nil || model.busy {
		t.Fatalf("session approval did not settle: busy=%t cmd=%#v", model.busy, cmd)
	}
	if len(model.runContext.Grants) != 1 || model.runContext.Grants[0].Status != approval.GrantActive ||
		model.toolRuns[model.selectedTool].Status != toolrun.StatusCompleted {
		t.Fatalf("TUI did not activate and apply the session grant: %#v %#v", model.runContext.Grants, model.toolRuns)
	}
	if err := model.Submit(ctx, "/run echo second"); err != nil {
		t.Fatal(err)
	}
	if err := model.Submit(ctx, "/run masscan 0.0.0.0/0"); err != nil {
		t.Fatal(err)
	}
	var safeCompleted, dangerousDenied bool
	for _, run := range model.toolRuns {
		switch run.Command {
		case "echo second":
			safeCompleted = run.Status == toolrun.StatusCompleted && run.Stdout == "dry run: echo second"
		case "masscan 0.0.0.0/0":
			dangerousDenied = run.Status == toolrun.StatusDenied
		}
	}
	if !safeCompleted || !dangerousDenied {
		t.Fatalf("session grant behavior is wrong: %#v", model.toolRuns)
	}
	model.width = 150
	if snapshot := model.Snapshot(); !strings.Contains(snapshot, "session grant:") {
		t.Fatalf("active session grant is not visible:\n%s", snapshot)
	}

	_, otherRun, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "separate approval scope", Profile: "code", WorkspaceID: rec.ID,
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := st.GetSession(ctx, otherRun.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	otherProposal, err := sessionManager.Send(ctx, otherSession.ID, "/run echo other-session")
	if err != nil || otherProposal.ToolRunID == "" {
		t.Fatalf("other Session proposal failed: %#v err=%v", otherProposal, err)
	}
	for _, input := range []string{
		"/approve " + otherProposal.ToolRunID,
		"/approve-session " + otherProposal.ToolRunID,
		"/deny " + otherProposal.ToolRunID + " wrong scope",
	} {
		if err := model.Submit(ctx, input); err == nil || !strings.Contains(err.Error(), "current Session") {
			t.Fatalf("cross-Session action %q was not rejected: %v", input, err)
		}
	}
	storedOther, err := st.GetToolRun(ctx, otherProposal.ToolRunID)
	if err != nil || storedOther.Status != toolrun.StatusProposed {
		t.Fatalf("cross-Session action changed the proposal: %#v err=%v", storedOther, err)
	}
	otherGrants, err := st.ListSessionGrants(ctx, approval.GrantListFilter{
		RunID: otherRun.ID, SessionID: otherRun.SessionID, Status: approval.GrantActive,
	})
	if err != nil || len(otherGrants) != 0 {
		t.Fatalf("cross-Session action created a grant: %#v err=%v", otherGrants, err)
	}
}
