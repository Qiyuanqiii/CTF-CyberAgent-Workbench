package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolrun"
)

type Model struct {
	session            session.Session
	sessionManager     *session.Manager
	toolManager        *toolrun.Manager
	workspaceStore     WorkspaceStore
	activeCalls        ActiveCallController
	input              textinput.Model
	messages           []session.Message
	toolRuns           []toolrun.ToolRun
	workspace          workspaceContext
	status             string
	busy               bool
	width              int
	height             int
	focus              focusArea
	messageScroll      int
	selectedTool       int
	toolScroll         int
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
	status         string
	selectedToolID string
}

type WorkspaceStore interface {
	GetWorkspaceByID(ctx context.Context, id string) (store.WorkspaceRecord, error)
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

type focusArea string

const (
	focusInput focusArea = "input"
	focusTools focusArea = "tools"
)

func NewModel(ctx context.Context, sess session.Session, sessionManager *session.Manager, toolManager *toolrun.Manager, workspaceStores ...WorkspaceStore) (*Model, error) {
	input := textinput.New()
	input.Placeholder = "Message or slash command"
	input.Prompt = ""
	input.CharLimit = 4000
	input.Width = 80
	input.Focus()

	var workspaceStore WorkspaceStore
	if len(workspaceStores) > 0 {
		workspaceStore = workspaceStores[0]
	}
	model := &Model{
		session:        sess,
		sessionManager: sessionManager,
		toolManager:    toolManager,
		workspaceStore: workspaceStore,
		input:          input,
		status:         "ready",
		width:          100,
		height:         32,
		focus:          focusInput,
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
				m.SelectPreviousTool()
			} else {
				m.ScrollMessages(1)
			}
			return m, nil
		case "down", "j":
			if m.focus == focusTools {
				m.SelectNextTool()
			} else {
				m.ScrollMessages(-1)
			}
			return m, nil
		case "a":
			if m.focus == focusTools {
				return m.selectedToolAction("approving", m.approveToolCmd)
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

	header := headerStyle.Width(width).Render(fmt.Sprintf("CyberAgent Workbench  session=%s  route=%s", m.session.ID, m.session.Route))
	messages := panelStyle.Width(messageWidth).Height(contentHeight).Render(m.renderMessages(messageWidth-2, contentHeight-2))
	workspaceHeight := min(8, max(6, contentHeight/3))
	toolHeight := max(6, contentHeight-workspaceHeight-1)
	workspace := panelStyle.Width(sideWidth).Height(workspaceHeight).Render(m.renderWorkspace(sideWidth-2, workspaceHeight-2))
	tools := panelStyle.Width(sideWidth).Height(toolHeight).Render(m.renderTools(sideWidth-2, toolHeight-2))
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
	case strings.HasPrefix(input, "/approve "):
		id := strings.TrimSpace(strings.TrimPrefix(input, "/approve "))
		run, err := m.toolManager.Approve(ctx, id)
		if err != nil {
			return actionResult{}, err
		}
		return m.refreshAction(ctx, sess, fmt.Sprintf("approved %s -> %s", run.ID, run.Status), run.ID)
	case strings.HasPrefix(input, "/deny "):
		fields := strings.Fields(input)
		if len(fields) < 2 {
			return actionResult{}, fmt.Errorf("usage: /deny <tool-run-id> [reason]")
		}
		reason := strings.TrimSpace(strings.TrimPrefix(input, fields[0]+" "+fields[1]))
		run, err := m.toolManager.Deny(ctx, fields[1], reason)
		if err != nil {
			return actionResult{}, err
		}
		return m.refreshAction(ctx, sess, fmt.Sprintf("denied %s", run.ID), run.ID)
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
		m.status = "tool focus"
		m.normalizeSelection()
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
	m.messages = messages
	m.toolRuns = runs
	m.workspace = m.loadWorkspaceContext(ctx, m.session)
	m.normalizeSelection()
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
	return actionResult{
		session:        sess,
		messages:       messages,
		toolRuns:       runs,
		workspace:      m.loadWorkspaceContext(ctx, sess),
		status:         status,
		selectedToolID: selectedToolID,
	}, nil
}

func (m *Model) approveAction(ctx context.Context, sess session.Session, id string) (actionResult, error) {
	run, err := m.toolManager.Approve(ctx, id)
	if err != nil {
		return actionResult{}, err
	}
	return m.refreshAction(ctx, sess, fmt.Sprintf("approved %s -> %s", run.ID, run.Status), run.ID)
}

func (m *Model) denyAction(ctx context.Context, sess session.Session, id string, reason string) (actionResult, error) {
	run, err := m.toolManager.Deny(ctx, id, reason)
	if err != nil {
		return actionResult{}, err
	}
	return m.refreshAction(ctx, sess, fmt.Sprintf("denied %s -> %s", run.ID, run.Status), run.ID)
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
	m.status = result.status
	target := result.selectedToolID
	if target == "" {
		target = oldSelection
	}
	m.selectToolByID(target)
	m.normalizeSelection()
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

func (m *Model) renderTools(width int, height int) string {
	lines := []string{m.toolTitle()}
	if len(m.toolRuns) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-1)
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
	return strings.Join(lines, "\n")
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

func (m *Model) toolTitle() string {
	if m.focus == focusTools {
		return "Tool Runs (focused)"
	}
	return "Tool Runs"
}

func (m *Model) statusLine() string {
	parts := []string{m.status, "focus=" + string(m.focus)}
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
		return "Tab | Enter | PgUp/PgDn | j/k tools | Ctrl+X cancel | Ctrl+R | Esc quit"
	}
	if width < 145 {
		return "Tab focus | Enter act | PgUp/PgDn | j/k tools | a/d decision | Ctrl+X cancel | Ctrl+R | Esc quit"
	}
	return "Tab focus | Enter send/approve | PgUp/PgDn scroll | tools: j/k select, a approve, d deny | Ctrl+X cancel call | Ctrl+R refresh | Esc quit"
}

func (m *Model) selectedToolRun() (toolrun.ToolRun, bool) {
	if len(m.toolRuns) == 0 {
		return toolrun.ToolRun{}, false
	}
	m.normalizeSelection()
	return m.toolRuns[m.selectedTool], true
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

func wrap(value string, width int) []string {
	value = strings.TrimSpace(value)
	if width <= 0 {
		return []string{value}
	}
	if len(value) <= width {
		return []string{value}
	}
	var lines []string
	for len(value) > width {
		cut := width
		if idx := strings.LastIndex(value[:width], " "); idx > 10 {
			cut = idx
		}
		lines = append(lines, strings.TrimSpace(value[:cut]))
		value = strings.TrimSpace(value[cut:])
	}
	if value != "" {
		lines = append(lines, value)
	}
	return lines
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
	if width <= 0 || len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
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
	case strings.HasPrefix(input, "/approve "):
		return "approving..."
	case strings.HasPrefix(input, "/deny "):
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
