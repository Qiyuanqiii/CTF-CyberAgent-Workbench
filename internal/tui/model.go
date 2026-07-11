package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolrun"
)

type Model struct {
	session            session.Session
	sessionManager     *session.Manager
	toolManager        ToolManager
	workspaceStore     WorkspaceStore
	runStateStore      RunStateStore
	sessionToolManager SessionToolManager
	activeCalls        ActiveCallController
	input              textinput.Model
	messages           []session.Message
	toolRuns           []toolrun.ToolRun
	workspace          workspaceContext
	runContext         runContext
	status             string
	busy               bool
	width              int
	height             int
	focus              focusArea
	messageScroll      int
	selectedTool       int
	toolScroll         int
	activityView       activityView
	selectedWorkItem   int
	workItemScroll     int
	selectedNote       int
	noteScroll         int
	selectedRound      int
	roundScroll        int
	live               activeCallView
	liveGeneration     uint64
	liveDiscoverCancel context.CancelFunc
	actionCancel       context.CancelFunc
	liveSubscription   ActiveCallSubscription
	cancelPending      bool
}

type actionDoneMsg struct {
	result actionResult
	err    error
}

type actionResult struct {
	session        session.Session
	messages       []session.Message
	toolRuns       []toolrun.ToolRun
	workspace      workspaceContext
	runContext     runContext
	status         string
	selectedToolID string
}

type WorkspaceStore interface {
	GetWorkspaceByID(ctx context.Context, id string) (store.WorkspaceRecord, error)
}

type ToolManager interface {
	List(context.Context, toolrun.ListFilter) ([]toolrun.ToolRun, error)
	Get(context.Context, string) (toolrun.ToolRun, error)
	Approve(context.Context, string) (toolrun.ToolRun, error)
	Deny(context.Context, string, string) (toolrun.ToolRun, error)
}

type SessionToolManager interface {
	ApproveForSession(ctx context.Context, id string, expectedSessionID string, reason string, grantedBy string,
		idempotencyKey string) (toolrun.ToolRun, approval.SessionGrant, error)
}

type RunStateStore interface {
	GetRunBySession(ctx context.Context, sessionID string) (domain.Run, bool, error)
	ListWorkItems(ctx context.Context, filter domain.WorkItemFilter) ([]domain.WorkItem, error)
	ListNotes(ctx context.Context, filter domain.NoteFilter) ([]domain.Note, error)
	ListRunSupervisorToolRoundsPage(ctx context.Context, runID string, offset int,
		limit int) ([]domain.SupervisorToolRound, error)
	ListSessionGrants(ctx context.Context, filter approval.GrantListFilter) ([]approval.SessionGrant, error)
}

type workspaceContext struct {
	ID       string
	Name     string
	RootPath string
	Items    []workspaceItem
	Error    string
}

type workspaceItem struct {
	Name   string
	Count  int
	Exists bool
	Error  string
}

type runContext struct {
	Found      bool
	Run        domain.Run
	WorkItems  []domain.WorkItem
	Notes      []domain.Note
	ToolRounds []domain.SupervisorToolRound
	Grants     []approval.SessionGrant
}

type activityView string

const (
	activityTools    activityView = "tools"
	activityWork     activityView = "work"
	activityNotes    activityView = "notes"
	activityRounds   activityView = "rounds"
	maxTUIRunItems                = 50
	maxTUIToolRounds              = 20
)

type focusArea string

const (
	focusInput focusArea = "input"
	focusTools focusArea = "tools"
)

func NewModel(ctx context.Context, sess session.Session, sessionManager *session.Manager, toolManager ToolManager, workspaceStores ...WorkspaceStore) (*Model, error) {
	input := textinput.New()
	input.Placeholder = "Message or slash command"
	input.Prompt = ""
	input.CharLimit = 4000
	input.Width = 80
	input.Focus()

	var workspaceStore WorkspaceStore
	var runStateStore RunStateStore
	if len(workspaceStores) > 0 {
		workspaceStore = workspaceStores[0]
		runStateStore, _ = any(workspaceStore).(RunStateStore)
	}
	sessionToolManager, _ := toolManager.(SessionToolManager)
	model := &Model{
		session: sess, sessionManager: sessionManager, toolManager: toolManager,
		workspaceStore: workspaceStore, runStateStore: runStateStore,
		sessionToolManager: sessionToolManager, input: input, status: "ready",
		width: 100, height: 32, focus: focusInput, activityView: activityTools,
	}
	if err := model.Refresh(ctx); err != nil {
		return nil, err
	}
	return model, nil
}

func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case activeCallSubscribedMsg:
		return m, m.handleActiveCallSubscribed(msg)
	case activeCallEventMsg:
		return m, m.handleActiveCallEvent(msg)
	case activeCallCancelDoneMsg:
		m.handleActiveCallCancelDone(msg)
		return m, nil
	case actionDoneMsg:
		m.stopLiveTracking(msg.err)
		m.busy = false
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			return m, nil
		}
		m.applyActionResult(msg.result)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(20, msg.Width-4)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.busy {
				m.status = "action running; Ctrl+X cancels the model call"
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+x":
			if m.activeCalls == nil {
				m.status = "active call control unavailable"
				return m, nil
			}
			if m.cancelPending {
				m.status = "model cancellation already pending"
				return m, nil
			}
			m.cancelPending = true
			m.status = "requesting model cancellation..."
			return m, m.cancelActiveCallCmd(m.liveGeneration, m.session.ID)
		}
		if m.busy {
			m.status = "busy; waiting for current action"
			return m, nil
		}
		switch msg.String() {
		case "ctrl+r":
			return m.startAction("refreshing...", m.refreshCmd("refreshed"))
		case "tab":
			m.ToggleFocus()
			return m, nil
		case "pgup":
			m.ScrollMessages(8)
			return m, nil
		case "pgdown":
			m.ScrollMessages(-8)
			return m, nil
		case "up", "k":
			if m.focus == focusTools {
				m.SelectPreviousActivityItem()
			} else {
				m.ScrollMessages(1)
			}
			return m, nil
		case "down", "j":
			if m.focus == focusTools {
				m.SelectNextActivityItem()
			} else {
				m.ScrollMessages(-1)
			}
			return m, nil
		case "left", "h":
			if m.focus == focusTools {
				m.PreviousActivityView()
				return m, nil
			}
		case "right", "l":
			if m.focus == focusTools {
				m.NextActivityView()
				return m, nil
			}
		case "a":
			if m.focus == focusTools {
				return m.selectedToolAction("approving", m.approveToolCmd)
			}
		case "g":
			if m.focus == focusTools {
				return m.selectedToolAction("authorizing session for", m.approveToolForSessionCmd)
			}
		case "d":
			if m.focus == focusTools {
				return m.selectedToolAction("denying", func(id string) tea.Cmd {
					return m.denyToolCmd(id, "denied from tui")
				})
			}
		case "enter":
			if m.focus == focusTools {
				return m.selectedToolAction("approving", m.approveToolCmd)
			}
			value := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if value == "" {
				return m, nil
			}
			if value == "/quit" || value == "/exit" {
				return m, tea.Quit
			}
			return m.startSubmitAction(statusForInput(value), value)
		}
	}
	if m.focus == focusInput {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m *Model) View() string {
	return m.Snapshot()
}

func (m *Model) Snapshot() string {
	width := max(80, m.width)
	height := max(24, m.height)
	contentHeight := max(8, height-8)
	sideWidth := 34
	messageWidth := width - sideWidth - 6
	if width < 110 {
		messageWidth = width - 4
		sideWidth = width - 4
	}

	headerText := fmt.Sprintf("CyberAgent Workbench  session=%s  route=%s", m.session.ID, m.session.Route)
	if m.runContext.Found {
		headerText += fmt.Sprintf("  run=%s  status=%s", m.runContext.Run.ID, m.runContext.Run.Status)
	}
	header := headerStyle.Width(width).Render(truncate(headerText, max(20, width-2)))
	messages := panelStyle.Width(messageWidth).Height(contentHeight).Render(m.renderMessages(messageWidth-2, contentHeight-2))
	workspaceHeight := min(8, max(6, contentHeight/3))
	toolHeight := max(6, contentHeight-workspaceHeight-1)
	workspace := panelStyle.Width(sideWidth).Height(workspaceHeight).Render(m.renderWorkspace(sideWidth-2, workspaceHeight-2))
	tools := panelStyle.Width(sideWidth).Height(toolHeight).Render(m.renderActivity(sideWidth-2, toolHeight-2))
	sidebar := lipgloss.JoinVertical(lipgloss.Left, workspace, tools)
	body := lipgloss.JoinHorizontal(lipgloss.Top, messages, "  ", sidebar)
	if width < 110 {
		body = lipgloss.JoinVertical(lipgloss.Left, messages, workspace, tools)
	}
	inputPrefix := "> "
	if m.focus == focusTools {
		inputPrefix = "  "
	}
	input := inputStyle.Width(width).Render(inputPrefix + m.input.View())
	status := statusStyle.Width(width).Render(m.statusLine())
	footer := footerStyle.Width(width).Render(footerHelp(width))
	return lipgloss.JoinVertical(lipgloss.Left, header, body, input, status, footer)
}

func (m *Model) Submit(ctx context.Context, input string) error {
	result, err := m.submitAction(ctx, m.session, input)
	if err != nil {
		return err
	}
	m.applyActionResult(result)
	return nil
}

func (m *Model) submitAction(ctx context.Context, sess session.Session, input string) (actionResult, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return m.refreshAction(ctx, sess, m.status, "")
	}
	switch {
	case input == "/approve-session" || strings.HasPrefix(input, "/approve-session "):
		id := strings.TrimSpace(strings.TrimPrefix(input, "/approve-session"))
		return m.approveForSessionAction(ctx, sess, id)
	case input == "/approve" || strings.HasPrefix(input, "/approve "):
		id := strings.TrimSpace(strings.TrimPrefix(input, "/approve"))
		return m.approveAction(ctx, sess, id)
	case input == "/deny" || strings.HasPrefix(input, "/deny "):
		fields := strings.Fields(input)
		if len(fields) < 2 {
			return actionResult{}, fmt.Errorf("usage: /deny <tool-run-id> [reason]")
		}
		reason := strings.TrimSpace(strings.TrimPrefix(input, fields[0]+" "+fields[1]))
		return m.denyAction(ctx, sess, fields[1], reason)
	case input == "/tools":
		return m.refreshAction(ctx, sess, "tool list refreshed", "")
	default:
		result, err := m.sessionManager.Send(ctx, sess.ID, input)
		if err != nil {
			return actionResult{}, err
		}
		status := "sent"
		selectedID := ""
		if result.ToolRunID != "" {
			status = "tool proposed: " + result.ToolRunID
			selectedID = result.ToolRunID
		} else if result.RunID != "" {
			status = fmt.Sprintf("run %s: %s -> %s", result.RunID, result.RunAction, result.RunStatus)
			if result.Compacted {
				status += fmt.Sprintf("; context compacted: summary=%d", result.SummaryID)
			}
		} else if result.Compacted {
			status = fmt.Sprintf("context compacted: summary=%d", result.SummaryID)
		}
		return m.refreshAction(ctx, result.Session, status, selectedID)
	}
}

func (m *Model) ToggleFocus() {
	if m.focus == focusInput {
		m.focus = focusTools
		m.input.Blur()
		m.status = "activity focus: " + string(m.activityView)
		m.normalizeActivitySelection()
		return
	}
	m.focus = focusInput
	m.input.Focus()
	m.status = "input focus"
}

func (m *Model) ScrollMessages(delta int) {
	m.messageScroll += delta
	if m.messageScroll < 0 {
		m.messageScroll = 0
	}
	maxScroll := max(0, len(m.messageLines(80))-1)
	if m.messageScroll > maxScroll {
		m.messageScroll = maxScroll
	}
	m.status = fmt.Sprintf("message scroll: %d", m.messageScroll)
}

func (m *Model) SelectNextTool() {
	if len(m.toolRuns) == 0 {
		m.status = "no tool runs"
		return
	}
	m.selectedTool++
	m.normalizeSelection()
	m.status = "selected " + m.toolRuns[m.selectedTool].ID
}

func (m *Model) SelectPreviousTool() {
	if len(m.toolRuns) == 0 {
		m.status = "no tool runs"
		return
	}
	m.selectedTool--
	m.normalizeSelection()
	m.status = "selected " + m.toolRuns[m.selectedTool].ID
}

func (m *Model) NextActivityView() {
	m.setActivityView(activityViewAt(m.activityView, 1))
}

func (m *Model) PreviousActivityView() {
	m.setActivityView(activityViewAt(m.activityView, -1))
}

func (m *Model) setActivityView(view activityView) {
	switch view {
	case activityTools, activityWork, activityNotes, activityRounds:
		m.activityView = view
	default:
		m.activityView = activityTools
	}
	m.normalizeActivitySelection()
	m.status = "activity view: " + string(m.activityView)
}

func activityViewAt(current activityView, delta int) activityView {
	views := []activityView{activityTools, activityWork, activityNotes, activityRounds}
	index := 0
	for candidate, view := range views {
		if view == current {
			index = candidate
			break
		}
	}
	index = (index + delta) % len(views)
	if index < 0 {
		index += len(views)
	}
	return views[index]
}

func (m *Model) SelectNextActivityItem() {
	switch m.activityView {
	case activityTools:
		m.SelectNextTool()
	case activityWork:
		m.selectedWorkItem++
		m.normalizeActivitySelection()
		m.status = m.selectedWorkItemStatus()
	case activityNotes:
		m.selectedNote++
		m.normalizeActivitySelection()
		m.status = m.selectedNoteStatus()
	case activityRounds:
		m.selectedRound++
		m.normalizeActivitySelection()
		m.status = m.selectedRoundStatus()
	}
}

func (m *Model) SelectPreviousActivityItem() {
	switch m.activityView {
	case activityTools:
		m.SelectPreviousTool()
	case activityWork:
		m.selectedWorkItem--
		m.normalizeActivitySelection()
		m.status = m.selectedWorkItemStatus()
	case activityNotes:
		m.selectedNote--
		m.normalizeActivitySelection()
		m.status = m.selectedNoteStatus()
	case activityRounds:
		m.selectedRound--
		m.normalizeActivitySelection()
		m.status = m.selectedRoundStatus()
	}
}

func (m *Model) ApproveSelectedTool(ctx context.Context) error {
	run, ok := m.selectedToolRun()
	if !ok {
		m.status = "no selected tool run"
		return nil
	}
	if run.Status != toolrun.StatusProposed {
		m.status = fmt.Sprintf("%s is %s", run.ID, run.Status)
		return nil
	}
	result, err := m.approveAction(ctx, m.session, run.ID)
	if err != nil {
		return err
	}
	m.applyActionResult(result)
	return nil
}

func (m *Model) ApproveSelectedToolForSession(ctx context.Context) error {
	run, ok := m.selectedToolRun()
	if !ok {
		m.status = "no selected tool run"
		return nil
	}
	if run.Status != toolrun.StatusProposed {
		m.status = fmt.Sprintf("%s is %s", run.ID, run.Status)
		return nil
	}
	result, err := m.approveForSessionAction(ctx, m.session, run.ID)
	if err != nil {
		return err
	}
	m.applyActionResult(result)
	return nil
}

func (m *Model) DenySelectedTool(ctx context.Context, reason string) error {
	run, ok := m.selectedToolRun()
	if !ok {
		m.status = "no selected tool run"
		return nil
	}
	if run.Status != toolrun.StatusProposed {
		m.status = fmt.Sprintf("%s is %s", run.ID, run.Status)
		return nil
	}
	result, err := m.denyAction(ctx, m.session, run.ID, reason)
	if err != nil {
		return err
	}
	m.applyActionResult(result)
	return nil
}

func (m *Model) Refresh(ctx context.Context) error {
	messages, err := m.sessionManager.History(ctx, m.session.ID, false)
	if err != nil {
		return err
	}
	runs, err := m.toolManager.List(ctx, toolrun.ListFilter{SessionID: m.session.ID})
	if err != nil {
		return err
	}
	runView, err := m.loadRunContext(ctx, m.session)
	if err != nil {
		return err
	}
	m.messages = messages
	m.toolRuns = runs
	m.workspace = m.loadWorkspaceContext(ctx, m.session)
	m.runContext = runView
	m.normalizeActivitySelection()
	return nil
}

func (m *Model) refreshAction(ctx context.Context, sess session.Session, status string, selectedToolID string) (actionResult, error) {
	messages, err := m.sessionManager.History(ctx, sess.ID, false)
	if err != nil {
		return actionResult{}, err
	}
	runs, err := m.toolManager.List(ctx, toolrun.ListFilter{SessionID: sess.ID})
	if err != nil {
		return actionResult{}, err
	}
	runView, err := m.loadRunContext(ctx, sess)
	if err != nil {
		return actionResult{}, err
	}
	return actionResult{
		session:        sess,
		messages:       messages,
		toolRuns:       runs,
		workspace:      m.loadWorkspaceContext(ctx, sess),
		runContext:     runView,
		status:         status,
		selectedToolID: selectedToolID,
	}, nil
}

func (m *Model) approveAction(ctx context.Context, sess session.Session, id string) (actionResult, error) {
	if err := m.requireSessionToolRun(ctx, sess.ID, id); err != nil {
		return actionResult{}, err
	}
	run, err := m.toolManager.Approve(ctx, id)
	if err != nil {
		return actionResult{}, err
	}
	return m.refreshAction(ctx, sess, fmt.Sprintf("approved %s -> %s", run.ID, run.Status), run.ID)
}

func (m *Model) approveForSessionAction(ctx context.Context, sess session.Session,
	id string,
) (actionResult, error) {
	if strings.TrimSpace(id) == "" {
		return actionResult{}, errors.New("usage: /approve-session <tool-run-id>")
	}
	if m.sessionToolManager == nil {
		return actionResult{}, errors.New("session approval is unavailable")
	}
	if err := m.requireSessionToolRun(ctx, sess.ID, id); err != nil {
		return actionResult{}, err
	}
	run, grant, err := m.sessionToolManager.ApproveForSession(ctx, id, sess.ID,
		"approved for this session from TUI", "tui_operator", idgen.New("tuigrantop"))
	if err != nil {
		return actionResult{}, err
	}
	status := fmt.Sprintf("session grant %s active; approved %s -> %s", grant.ID, run.ID, run.Status)
	return m.refreshAction(ctx, sess, status, run.ID)
}

func (m *Model) denyAction(ctx context.Context, sess session.Session, id string, reason string) (actionResult, error) {
	if err := m.requireSessionToolRun(ctx, sess.ID, id); err != nil {
		return actionResult{}, err
	}
	run, err := m.toolManager.Deny(ctx, id, reason)
	if err != nil {
		return actionResult{}, err
	}
	return m.refreshAction(ctx, sess, fmt.Sprintf("denied %s -> %s", run.ID, run.Status), run.ID)
}

func (m *Model) requireSessionToolRun(ctx context.Context, sessionID string, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("tool run id is required")
	}
	run, err := m.toolManager.Get(ctx, id)
	if err != nil {
		return err
	}
	if run.SessionID != strings.TrimSpace(sessionID) {
		return errors.New("tool run does not belong to the current Session")
	}
	return nil
}

func (m *Model) submitCmd(input string) tea.Cmd {
	return m.submitCmdContext(context.Background(), input)
}

func (m *Model) submitCmdContext(ctx context.Context, input string) tea.Cmd {
	sess := m.session
	return func() tea.Msg {
		result, err := m.submitAction(ctx, sess, input)
		return actionDoneMsg{result: result, err: err}
	}
}

func (m *Model) refreshCmd(status string) tea.Cmd {
	sess := m.session
	return func() tea.Msg {
		result, err := m.refreshAction(context.Background(), sess, status, "")
		return actionDoneMsg{result: result, err: err}
	}
}

func (m *Model) approveToolCmd(id string) tea.Cmd {
	sess := m.session
	return func() tea.Msg {
		result, err := m.approveAction(context.Background(), sess, id)
		return actionDoneMsg{result: result, err: err}
	}
}

func (m *Model) approveToolForSessionCmd(id string) tea.Cmd {
	sess := m.session
	return func() tea.Msg {
		result, err := m.approveForSessionAction(context.Background(), sess, id)
		return actionDoneMsg{result: result, err: err}
	}
}

func (m *Model) denyToolCmd(id string, reason string) tea.Cmd {
	sess := m.session
	return func() tea.Msg {
		result, err := m.denyAction(context.Background(), sess, id, reason)
		return actionDoneMsg{result: result, err: err}
	}
}

func (m *Model) startAction(status string, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.busy = true
	m.status = status
	return m, cmd
}

func (m *Model) selectedToolAction(verb string, makeCmd func(string) tea.Cmd) (tea.Model, tea.Cmd) {
	if m.activityView != activityTools {
		m.status = string(m.activityView) + " view is read-only"
		return m, nil
	}
	run, ok := m.selectedToolRun()
	if !ok {
		m.status = "no selected tool run"
		return m, nil
	}
	if run.Status != toolrun.StatusProposed {
		m.status = fmt.Sprintf("%s is %s", run.ID, run.Status)
		return m, nil
	}
	return m.startAction(verb+" "+run.ID+"...", makeCmd(run.ID))
}

func (m *Model) applyActionResult(result actionResult) {
	oldSelection := ""
	if run, ok := m.selectedToolRun(); ok {
		oldSelection = run.ID
	}
	m.session = result.session
	m.messages = result.messages
	m.toolRuns = result.toolRuns
	m.workspace = result.workspace
	m.runContext = result.runContext
	m.status = result.status
	target := result.selectedToolID
	if target == "" {
		target = oldSelection
	}
	m.selectToolByID(target)
	m.normalizeActivitySelection()
}

func (m *Model) renderMessages(width int, height int) string {
	lines := []string{m.messageTitle()}
	messageLines := m.messageLines(width)
	lines = append(lines, windowFromBottom(messageLines, max(0, height-1), m.messageScroll)...)
	return strings.Join(lines, "\n")
}

func (m *Model) messageLines(width int) []string {
	var lines []string
	for _, msg := range m.messages {
		prefix := msg.Role + ": "
		lines = append(lines, wrap(prefix+singleLine(msg.Content), width)...)
	}
	if len(lines) == 0 {
		return []string{"no messages"}
	}
	return lines
}

func (m *Model) renderActivity(width int, height int) string {
	switch m.activityView {
	case activityWork:
		return m.renderWorkItems(width, height)
	case activityNotes:
		return m.renderNotes(width, height)
	case activityRounds:
		return m.renderToolRounds(width, height)
	default:
		return m.renderTools(width, height)
	}
}

func (m *Model) renderTools(width int, height int) string {
	lines := m.activityHeader("Tool Runs", activityTools, width)
	if grant, ok := m.activeGrant(string(toolrun.ShellTool)); ok {
		lines = append(lines, truncate("session grant: "+grant.ID, width))
	}
	if len(m.toolRuns) == 0 {
		lines = append(lines, "none")
		return strings.Join(windowTop(lines, height+1), "\n")
	}
	visible := max(1, height-len(lines))
	m.ensureSelectedVisible(visible)
	end := min(len(m.toolRuns), m.toolScroll+visible)
	for i := m.toolScroll; i < end; i++ {
		run := m.toolRuns[i]
		marker := " "
		if i == m.selectedTool {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s %s", marker, run.Status, run.ID), width))
		lines = append(lines, truncate("  "+run.Command, width))
		if run.Stdout != "" && i == m.selectedTool {
			lines = append(lines, truncate("  "+run.Stdout, width))
		}
	}
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) renderWorkItems(width int, height int) string {
	lines := m.activityHeader(fmt.Sprintf("Work Board %d", len(m.runContext.WorkItems)), activityWork, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	if len(m.runContext.WorkItems) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-2)
	m.ensureWorkItemVisible(visible)
	end := min(len(m.runContext.WorkItems), m.workItemScroll+visible)
	for index := m.workItemScroll; index < end; index++ {
		item := m.runContext.WorkItems[index]
		marker := " "
		if index == m.selectedWorkItem {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s/%s %s", marker, item.Status,
			item.Priority, item.Title), width))
	}
	selected := m.runContext.WorkItems[m.selectedWorkItem]
	detail := fmt.Sprintf("owner=%s deps=%d v=%d", defaultText(selected.Owner, "-"),
		len(selected.Dependencies), selected.Version)
	if selected.BlockedReason != "" {
		detail += " blocked=" + singleLine(selected.BlockedReason)
	}
	lines = append(lines, truncate(detail, width))
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) renderNotes(width int, height int) string {
	lines := m.activityHeader(fmt.Sprintf("Notes %d", len(m.runContext.Notes)), activityNotes, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	if len(m.runContext.Notes) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-3)
	m.ensureNoteVisible(visible)
	end := min(len(m.runContext.Notes), m.noteScroll+visible)
	for index := m.noteScroll; index < end; index++ {
		note := m.runContext.Notes[index]
		marker := " "
		if index == m.selectedNote {
			marker = ">"
		}
		pin := ""
		if note.Pinned {
			pin = "*"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s%s %s/%s %s", marker, pin, note.Category,
			note.Visibility, note.Title), width))
	}
	selected := m.runContext.Notes[m.selectedNote]
	lines = append(lines, truncate("status="+string(selected.Status)+" v="+fmt.Sprint(selected.Version), width))
	lines = append(lines, truncate(singleLine(selected.Content), width))
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) renderToolRounds(width int, height int) string {
	lines := m.activityHeader(fmt.Sprintf("Tool Rounds %d", len(m.runContext.ToolRounds)), activityRounds, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	if len(m.runContext.ToolRounds) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	m.normalizeActivitySelection()
	detailLines := len(m.runContext.ToolRounds[m.selectedRound].Calls)
	visible := max(1, height+1-len(lines)-detailLines)
	m.ensureRoundVisible(visible)
	end := min(len(m.runContext.ToolRounds), m.roundScroll+visible)
	for index := m.roundScroll; index < end; index++ {
		round := m.runContext.ToolRounds[index]
		marker := " "
		if index == m.selectedRound {
			marker = ">"
		}
		status := "pending"
		if round.Complete() {
			status = "complete"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s turn=%d round=%d %s calls=%d", marker,
			round.Turn, round.Round, status, len(round.Calls)), width))
	}
	for _, call := range m.runContext.ToolRounds[m.selectedRound].Calls {
		lines = append(lines, truncate(fmt.Sprintf("  %d %s %s", call.Position, call.ToolName, call.Status), width))
	}
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) renderWorkspace(width int, height int) string {
	lines := []string{"Workspace"}
	if strings.TrimSpace(m.workspace.ID) == "" {
		lines = append(lines, "none attached")
		return strings.Join(windowTop(lines, height+1), "\n")
	}
	lines = append(lines, truncate("id: "+m.workspace.ID, width))
	if m.workspace.Name != "" {
		lines = append(lines, truncate("name: "+m.workspace.Name, width))
	}
	if m.workspace.RootPath != "" {
		lines = append(lines, truncate("root: "+m.workspace.RootPath, width))
	}
	if m.workspace.Error != "" {
		lines = append(lines, truncate("error: "+m.workspace.Error, width))
		return strings.Join(windowTop(lines, height+1), "\n")
	}
	for _, item := range m.workspace.Items {
		label := item.Name + ": missing"
		if item.Exists {
			label = fmt.Sprintf("%s: %d", item.Name, item.Count)
		}
		if item.Error != "" {
			label = item.Name + ": error"
		}
		lines = append(lines, truncate(label, width))
	}
	return strings.Join(windowTop(lines, height+1), "\n")
}

func Run(model *Model) error {
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("63")).Padding(0, 1)
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	inputStyle  = lipgloss.NewStyle().Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(0, 1)
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
)

func (m *Model) messageTitle() string {
	if m.messageScroll == 0 {
		return "Messages"
	}
	return fmt.Sprintf("Messages (scroll %d)", m.messageScroll)
}

func (m *Model) activityHeader(label string, selected activityView, width int) []string {
	if m.focus == focusTools {
		label += " (focused)"
	}
	tabs := make([]string, 0, 4)
	for _, view := range []activityView{activityTools, activityWork, activityNotes, activityRounds} {
		name := activityLabel(view)
		if view == selected {
			name = "[" + name + "]"
		}
		tabs = append(tabs, name)
	}
	return []string{truncate(label, width), truncate(strings.Join(tabs, " "), width)}
}

func activityLabel(view activityView) string {
	switch view {
	case activityWork:
		return "Work"
	case activityNotes:
		return "Notes"
	case activityRounds:
		return "Rounds"
	default:
		return "Tools"
	}
}

func (m *Model) statusLine() string {
	focus := string(m.focus)
	if m.focus == focusTools {
		focus = "activity:" + string(m.activityView)
	}
	parts := []string{m.status, "focus=" + focus}
	if m.busy {
		parts = append(parts, "busy")
	}
	if live := m.live.summary(); live != "" {
		parts = append(parts, live)
	}
	return truncate(strings.Join(parts, " | "), max(20, m.width-2))
}

func footerHelp(width int) string {
	if width < 100 {
		return "Tab | h/l views | j/k | a once | g session | d deny | Ctrl+X | Esc"
	}
	if width < 145 {
		return "Tab focus | h/l views | j/k select | a once | g session | d deny | Ctrl+X cancel | Ctrl+R | Esc"
	}
	return "Tab focus | h/l activity views | j/k select | tools: a approve once, g grant session, d deny | Ctrl+X cancel | Ctrl+R | Esc"
}

func (m *Model) selectedToolRun() (toolrun.ToolRun, bool) {
	if len(m.toolRuns) == 0 {
		return toolrun.ToolRun{}, false
	}
	m.normalizeSelection()
	return m.toolRuns[m.selectedTool], true
}

func (m *Model) loadRunContext(ctx context.Context, sess session.Session) (runContext, error) {
	if m.runStateStore == nil {
		return runContext{}, nil
	}
	run, found, err := m.runStateStore.GetRunBySession(ctx, sess.ID)
	if err != nil || !found {
		return runContext{}, err
	}
	workItems, err := m.runStateStore.ListWorkItems(ctx, domain.WorkItemFilter{
		RunID: run.ID, Limit: maxTUIRunItems,
	})
	if err != nil {
		return runContext{}, err
	}
	notes, err := m.runStateStore.ListNotes(ctx, domain.NoteFilter{
		RunID: run.ID, Limit: maxTUIRunItems,
	})
	if err != nil {
		return runContext{}, err
	}
	rounds, err := m.runStateStore.ListRunSupervisorToolRoundsPage(ctx, run.ID, 0, maxTUIToolRounds)
	if err != nil {
		return runContext{}, err
	}
	grants, err := m.runStateStore.ListSessionGrants(ctx, approval.GrantListFilter{
		RunID: run.ID, SessionID: sess.ID, Status: approval.GrantActive, Limit: maxTUIRunItems,
	})
	if err != nil {
		return runContext{}, err
	}
	return runContext{Found: true, Run: run, WorkItems: workItems, Notes: notes,
		ToolRounds: rounds, Grants: grants}, nil
}

func (m *Model) loadWorkspaceContext(ctx context.Context, sess session.Session) workspaceContext {
	id := strings.TrimSpace(sess.WorkspaceID)
	if id == "" {
		return workspaceContext{}
	}
	ctxView := workspaceContext{ID: id}
	if m.workspaceStore == nil {
		ctxView.Error = "workspace lookup unavailable"
		return ctxView
	}
	rec, err := m.workspaceStore.GetWorkspaceByID(ctx, id)
	if err != nil {
		ctxView.Error = err.Error()
		return ctxView
	}
	ctxView.Name = rec.Name
	ctxView.RootPath = rec.RootPath
	for _, name := range []string{"attachments", "scripts", "outputs", "logs", "writeups"} {
		count, exists, itemErr := countDirEntries(filepath.Join(rec.RootPath, name))
		item := workspaceItem{Name: name, Count: count, Exists: exists}
		if itemErr != nil {
			item.Error = itemErr.Error()
		}
		ctxView.Items = append(ctxView.Items, item)
	}
	return ctxView
}

func (m *Model) normalizeSelection() {
	if len(m.toolRuns) == 0 {
		m.selectedTool = 0
		m.toolScroll = 0
		return
	}
	if m.selectedTool < 0 {
		m.selectedTool = 0
	}
	if m.selectedTool >= len(m.toolRuns) {
		m.selectedTool = len(m.toolRuns) - 1
	}
	if m.toolScroll < 0 {
		m.toolScroll = 0
	}
	if m.toolScroll >= len(m.toolRuns) {
		m.toolScroll = len(m.toolRuns) - 1
	}
}

func (m *Model) normalizeActivitySelection() {
	m.normalizeSelection()
	normalizeListSelection(len(m.runContext.WorkItems), &m.selectedWorkItem, &m.workItemScroll)
	normalizeListSelection(len(m.runContext.Notes), &m.selectedNote, &m.noteScroll)
	normalizeListSelection(len(m.runContext.ToolRounds), &m.selectedRound, &m.roundScroll)
}

func normalizeListSelection(length int, selected *int, scroll *int) {
	if length == 0 {
		*selected = 0
		*scroll = 0
		return
	}
	if *selected < 0 {
		*selected = 0
	}
	if *selected >= length {
		*selected = length - 1
	}
	if *scroll < 0 {
		*scroll = 0
	}
	if *scroll >= length {
		*scroll = length - 1
	}
}

func (m *Model) selectToolByID(id string) {
	if id == "" {
		return
	}
	for i, run := range m.toolRuns {
		if run.ID == id {
			m.selectedTool = i
			return
		}
	}
}

func (m *Model) ensureSelectedVisible(visible int) {
	m.normalizeSelection()
	if visible <= 0 || len(m.toolRuns) == 0 {
		return
	}
	if m.selectedTool < m.toolScroll {
		m.toolScroll = m.selectedTool
	}
	if m.selectedTool >= m.toolScroll+visible {
		m.toolScroll = m.selectedTool - visible + 1
	}
	m.normalizeSelection()
}

func (m *Model) ensureWorkItemVisible(visible int) {
	m.normalizeActivitySelection()
	ensureListSelectionVisible(len(m.runContext.WorkItems), m.selectedWorkItem, visible, &m.workItemScroll)
}

func (m *Model) ensureNoteVisible(visible int) {
	m.normalizeActivitySelection()
	ensureListSelectionVisible(len(m.runContext.Notes), m.selectedNote, visible, &m.noteScroll)
}

func (m *Model) ensureRoundVisible(visible int) {
	m.normalizeActivitySelection()
	ensureListSelectionVisible(len(m.runContext.ToolRounds), m.selectedRound, visible, &m.roundScroll)
}

func ensureListSelectionVisible(length int, selected int, visible int, scroll *int) {
	if length == 0 || visible <= 0 {
		return
	}
	if selected < *scroll {
		*scroll = selected
	}
	if selected >= *scroll+visible {
		*scroll = selected - visible + 1
	}
	if *scroll < 0 {
		*scroll = 0
	}
	if *scroll >= length {
		*scroll = length - 1
	}
}

func (m *Model) activeGrant(toolName string) (approval.SessionGrant, bool) {
	for _, grant := range m.runContext.Grants {
		if grant.Status == approval.GrantActive && grant.ToolName == toolName {
			return grant, true
		}
	}
	return approval.SessionGrant{}, false
}

func (m *Model) selectedWorkItemStatus() string {
	if len(m.runContext.WorkItems) == 0 {
		return "no work items"
	}
	return "selected " + m.runContext.WorkItems[m.selectedWorkItem].ID
}

func (m *Model) selectedNoteStatus() string {
	if len(m.runContext.Notes) == 0 {
		return "no notes"
	}
	return "selected " + m.runContext.Notes[m.selectedNote].ID
}

func (m *Model) selectedRoundStatus() string {
	if len(m.runContext.ToolRounds) == 0 {
		return "no tool rounds"
	}
	round := m.runContext.ToolRounds[m.selectedRound]
	return fmt.Sprintf("selected turn %d round %d", round.Turn, round.Round)
}

func wrap(value string, width int) []string {
	value = strings.TrimSpace(value)
	if width <= 0 || ansi.StringWidth(value) <= width {
		return []string{value}
	}
	return strings.Split(ansi.Wrap(value, width, ""), "\n")
}

func windowFromBottom(lines []string, maxLines int, scroll int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	if scroll < 0 {
		scroll = 0
	}
	end := len(lines) - scroll
	if end > len(lines) {
		end = len(lines)
	}
	if end < maxLines {
		end = maxLines
	}
	start := end - maxLines
	return lines[start:end]
}

func windowTop(lines []string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	return lines[:maxLines]
}

func truncate(value string, width int) string {
	if width <= 0 || ansi.StringWidth(value) <= width {
		return value
	}
	if width <= 3 {
		return ansi.Truncate(value, width, "")
	}
	return ansi.Truncate(value, width, "...")
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func countDirEntries(path string) (int, bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, true, err
	}
	return len(entries), true, nil
}

func statusForInput(input string) string {
	switch {
	case input == "/approve-session" || strings.HasPrefix(input, "/approve-session "):
		return "authorizing session..."
	case input == "/approve" || strings.HasPrefix(input, "/approve "):
		return "approving..."
	case input == "/deny" || strings.HasPrefix(input, "/deny "):
		return "denying..."
	case input == "/tools":
		return "refreshing tools..."
	case strings.HasPrefix(input, "/run "):
		return "proposing tool..."
	case strings.HasPrefix(input, "/"):
		return "working..."
	default:
		return "thinking..."
	}
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
