package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/session"
)

const (
	maxTUIPickerRuns       = 50
	maxTUIPickerSessions   = 50
	maxTUIPickerGoalBytes  = 64 * 1024
	maxTUIPickerTitleBytes = 16 * 1024
	maxTUIPickerRouteRunes = 256
)

type pickerView string

const (
	pickerRuns     pickerView = "runs"
	pickerSessions pickerView = "sessions"
)

type RunPickerStore interface {
	ListRuns(context.Context, domain.RunFilter) ([]domain.Run, error)
	ListSessionsPage(context.Context, int, int) ([]session.Session, error)
	GetMission(context.Context, string) (domain.Mission, error)
	GetSession(context.Context, string) (session.Session, error)
}

type runPickerItem struct {
	Run     domain.Run
	Mission domain.Mission
}

type Picker struct {
	sessionManager    *session.Manager
	toolManager       ToolManager
	workspaceStore    WorkspaceStore
	runStore          RunPickerStore
	activeCalls       ActiveCallController
	sessions          []session.Session
	runs              []runPickerItem
	selected          int
	selectedRun       int
	runsTruncated     bool
	sessionsTruncated bool
	view              pickerView
	workspaceID       string
	newTitle          string
	newRoute          string
	status            string
	width             int
	height            int
}

func (p *Picker) WithActiveCallController(controller ActiveCallController) *Picker {
	if p != nil {
		p.activeCalls = controller
	}
	return p
}

func NewPicker(ctx context.Context, sessionManager *session.Manager, toolManager ToolManager, workspaceID string, title string, route string, workspaceStores ...WorkspaceStore) (*Picker, error) {
	var workspaceStore WorkspaceStore
	if len(workspaceStores) > 0 {
		workspaceStore = workspaceStores[0]
	}
	runStore, _ := any(workspaceStore).(RunPickerStore)
	view := pickerSessions
	status := "select a Session or press n for new"
	if runStore != nil {
		view = pickerRuns
		status = "select a Run or switch to Sessions"
	}
	picker := &Picker{
		sessionManager: sessionManager,
		toolManager:    toolManager,
		workspaceStore: workspaceStore,
		runStore:       runStore,
		workspaceID:    workspaceID,
		newTitle:       title,
		newRoute:       route,
		status:         status,
		view:           view,
		width:          100,
		height:         28,
	}
	if strings.TrimSpace(picker.newTitle) == "" {
		picker.newTitle = "TUI session"
	}
	if strings.TrimSpace(picker.newRoute) == "" {
		picker.newRoute = "learn"
	}
	if err := picker.Refresh(ctx); err != nil {
		return nil, err
	}
	return picker, nil
}

func (p *Picker) Init() tea.Cmd {
	return nil
}

func (p *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return p, tea.Quit
		case "tab", "left", "h", "right", "l":
			p.ToggleView()
			return p, nil
		case "ctrl+r", "r":
			if err := p.Refresh(context.Background()); err != nil {
				p.status = "error: " + err.Error()
			} else {
				p.status = "refreshed"
			}
			return p, nil
		case "up", "k":
			p.SelectPrevious()
			return p, nil
		case "down", "j":
			p.SelectNext()
			return p, nil
		case "n":
			model, err := p.NewSessionModel(context.Background())
			if err != nil {
				p.status = "error: " + err.Error()
				return p, nil
			}
			return model, model.Init()
		case "enter":
			var model *Model
			var err error
			if p.view == pickerRuns {
				model, err = p.SelectedRunModel(context.Background())
			} else {
				model, err = p.SelectedSessionModel(context.Background())
			}
			if err != nil {
				p.status = "error: " + err.Error()
				return p, nil
			}
			return model, model.Init()
		}
	}
	return p, nil
}

func (p *Picker) View() string {
	return p.Snapshot()
}

func (p *Picker) Snapshot() string {
	width := max(80, p.width)
	height := max(16, p.height)
	contentHeight := max(8, height-6)
	header := headerStyle.Width(width).Render("Prayu  " + string(p.view))
	content := p.renderSessions(width-4, contentHeight-2)
	if p.view == pickerRuns {
		content = p.renderRuns(width-4, contentHeight-2)
	}
	panel := panelStyle.Width(width).Height(contentHeight).Render(content)
	status := statusStyle.Width(width).Render(p.status)
	footer := footerStyle.Width(width).Render(truncate(pickerFooterHelp(width), max(20, width-2)))
	return lipgloss.JoinVertical(lipgloss.Left, header, panel, status, footer)
}

func (p *Picker) Refresh(ctx context.Context) error {
	var sessions []session.Session
	var err error
	p.sessionsTruncated = false
	if p.runStore != nil {
		sessions, err = p.runStore.ListSessionsPage(ctx, 0, maxTUIPickerSessions+1)
	} else {
		sessions, err = p.sessionManager.List(ctx)
	}
	if err != nil {
		return err
	}
	if len(sessions) > maxTUIPickerSessions {
		p.sessionsTruncated = true
		sessions = sessions[:maxTUIPickerSessions]
	}
	for _, sess := range sessions {
		if err := validateTUIPickerSession(sess); err != nil {
			return err
		}
	}
	p.sessions = sessions
	p.runs = nil
	p.runsTruncated = false
	if p.runStore != nil {
		runs, err := p.runStore.ListRuns(ctx, domain.RunFilter{Limit: maxTUIPickerRuns + 1})
		if err != nil {
			return err
		}
		if len(runs) > maxTUIPickerRuns {
			p.runsTruncated = true
			runs = runs[:maxTUIPickerRuns]
		}
		p.runs = make([]runPickerItem, 0, len(runs))
		for _, run := range runs {
			if err := run.Validate(); err != nil || !boundedTUIIdentity(run.ID) ||
				!boundedTUIIdentity(run.MissionID) || !boundedTUIIdentity(run.SessionID) ||
				!validTUIEventText(run.Config.ModelRoute, maxTUIPickerRouteRunes, false) {
				return errors.New("TUI Run picker received an invalid Run")
			}
			mission, err := p.runStore.GetMission(ctx, run.MissionID)
			if err != nil {
				return err
			}
			if err := mission.Validate(); err != nil || mission.ID != run.MissionID ||
				!boundedTUIIdentity(mission.ID) || len(mission.Goal) > maxTUIPickerGoalBytes ||
				!utf8.ValidString(mission.Goal) {
				return errors.New("TUI Run picker received an invalid Mission binding")
			}
			p.runs = append(p.runs, runPickerItem{Run: run, Mission: mission})
		}
	}
	p.normalizeSelection()
	return nil
}

func (p *Picker) ToggleView() {
	if p.runStore == nil {
		p.view = pickerSessions
		p.status = "Run list unavailable; showing Sessions"
		return
	}
	if p.view == pickerRuns {
		p.view = pickerSessions
	} else {
		p.view = pickerRuns
	}
	p.normalizeSelection()
	p.status = "picker view: " + string(p.view)
}

func (p *Picker) SelectNext() {
	if p.view == pickerRuns {
		if len(p.runs) == 0 {
			p.status = "no Runs; switch to Sessions or create one with the CLI"
			return
		}
		p.selectedRun++
		p.normalizeSelection()
		p.status = "selected " + p.runs[p.selectedRun].Run.ID
		return
	}
	if len(p.sessions) == 0 {
		p.status = "no sessions; press n for new"
		return
	}
	p.selected++
	p.normalizeSelection()
	p.status = "selected " + p.sessions[p.selected].ID
}

func (p *Picker) SelectPrevious() {
	if p.view == pickerRuns {
		if len(p.runs) == 0 {
			p.status = "no Runs; switch to Sessions or create one with the CLI"
			return
		}
		p.selectedRun--
		p.normalizeSelection()
		p.status = "selected " + p.runs[p.selectedRun].Run.ID
		return
	}
	if len(p.sessions) == 0 {
		p.status = "no sessions; press n for new"
		return
	}
	p.selected--
	p.normalizeSelection()
	p.status = "selected " + p.sessions[p.selected].ID
}

func (p *Picker) SelectedSessionModel(ctx context.Context) (*Model, error) {
	if len(p.sessions) == 0 {
		return nil, fmt.Errorf("no sessions to open; press n to create one")
	}
	p.normalizeSelection()
	model, err := NewModel(ctx, p.sessions[p.selected], p.sessionManager, p.toolManager, p.workspaceStore)
	if err != nil {
		return nil, err
	}
	return model.WithActiveCallController(p.activeCalls), nil
}

func (p *Picker) SelectedRunModel(ctx context.Context) (*Model, error) {
	if p.runStore == nil {
		return nil, errors.New("Run list is unavailable")
	}
	if len(p.runs) == 0 {
		return nil, errors.New("no Runs to open; switch to Sessions or create a Run with the CLI")
	}
	p.normalizeSelection()
	run := p.runs[p.selectedRun].Run
	sess, err := p.runStore.GetSession(ctx, run.SessionID)
	if err != nil {
		return nil, err
	}
	if sess.ID != run.SessionID {
		return nil, errors.New("selected Run has an inconsistent Session binding")
	}
	model, err := NewModel(ctx, sess, p.sessionManager, p.toolManager, p.workspaceStore)
	if err != nil {
		return nil, err
	}
	if projection, found := model.CurrentRunProjection(); !found || projection.RunID != run.ID {
		return nil, errors.New("selected Run did not resolve to the expected TUI projection")
	}
	return model.WithActiveCallController(p.activeCalls), nil
}

func (p *Picker) NewSessionModel(ctx context.Context) (*Model, error) {
	sess, err := p.sessionManager.Create(ctx, p.workspaceID, p.newTitle, p.newRoute)
	if err != nil {
		return nil, err
	}
	model, err := NewModel(ctx, sess, p.sessionManager, p.toolManager, p.workspaceStore)
	if err != nil {
		return nil, err
	}
	return model.WithActiveCallController(p.activeCalls), nil
}

func (p *Picker) renderSessions(width int, height int) string {
	lines := []string{"Runs [Sessions]"}
	if len(p.sessions) == 0 {
		lines = append(lines, "none")
		lines = append(lines, "")
		lines = append(lines, "Press n to create a new session.")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-1)
	start := 0
	if p.selected >= visible {
		start = p.selected - visible + 1
	}
	end := min(len(p.sessions), start+visible)
	for i := start; i < end; i++ {
		sess := p.sessions[i]
		marker := " "
		if i == p.selected {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s  %s  %s", marker, sess.ID, sess.Route, sess.Title), width))
		if sess.WorkspaceID != "" {
			lines = append(lines, truncate("  workspace: "+sess.WorkspaceID, width))
		}
	}
	if p.sessionsTruncated {
		lines = append(lines, fmt.Sprintf("showing latest %d Sessions", maxTUIPickerSessions))
	}
	return strings.Join(lines, "\n")
}

func pickerFooterHelp(width int) string {
	if width < 96 {
		return "Tab Runs/Sessions | Enter | n new | j/k | r | q/Esc"
	}
	return "Tab/h/l Runs/Sessions | Enter open | n new Session | j/k select | r refresh | q/Esc quit"
}

func validateTUIPickerSession(sess session.Session) error {
	if err := sess.Validate(); err != nil || !boundedTUIIdentity(sess.ID) ||
		(sess.WorkspaceID != "" && !boundedTUIIdentity(sess.WorkspaceID)) ||
		!utf8.ValidString(sess.Title) || len(sess.Title) > maxTUIPickerTitleBytes ||
		!validTUIEventText(sess.Route, maxTUIPickerRouteRunes, false) {
		return errors.New("TUI picker received an invalid or unbounded Session")
	}
	return nil
}

func (p *Picker) renderRuns(width int, height int) string {
	lines := []string{"[Runs] Sessions"}
	if len(p.runs) == 0 {
		lines = append(lines, "none")
		lines = append(lines, "")
		lines = append(lines, "Switch to Sessions or create a Run with the CLI.")
		return strings.Join(lines, "\n")
	}
	visible := max(1, (height-2)/2)
	start := 0
	if p.selectedRun >= visible {
		start = p.selectedRun - visible + 1
	}
	end := min(len(p.runs), start+visible)
	for index := start; index < end; index++ {
		item := p.runs[index]
		marker := " "
		if index == p.selectedRun {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s  %s  %s", marker,
			item.Run.ID, item.Run.Status, item.Mission.Profile), width))
		lines = append(lines, truncate("  "+singleLine(item.Mission.Goal), width))
	}
	if p.runsTruncated {
		lines = append(lines, fmt.Sprintf("showing latest %d Runs", maxTUIPickerRuns))
	}
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (p *Picker) normalizeSelection() {
	if len(p.sessions) == 0 {
		p.selected = 0
	} else {
		if p.selected < 0 {
			p.selected = 0
		}
		if p.selected >= len(p.sessions) {
			p.selected = len(p.sessions) - 1
		}
	}
	if len(p.runs) == 0 {
		p.selectedRun = 0
		return
	}
	if p.selectedRun < 0 {
		p.selectedRun = 0
	}
	if p.selectedRun >= len(p.runs) {
		p.selectedRun = len(p.runs) - 1
	}
}

func RunPicker(picker *Picker) error {
	_, err := tea.NewProgram(picker, tea.WithAltScreen()).Run()
	return err
}
