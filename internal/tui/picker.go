package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cyberagent-workbench/internal/session"
)

type Picker struct {
	sessionManager *session.Manager
	toolManager    ToolManager
	workspaceStore WorkspaceStore
	activeCalls    ActiveCallController
	sessions       []session.Session
	selected       int
	workspaceID    string
	newTitle       string
	newRoute       string
	status         string
	width          int
	height         int
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
	picker := &Picker{
		sessionManager: sessionManager,
		toolManager:    toolManager,
		workspaceStore: workspaceStore,
		workspaceID:    workspaceID,
		newTitle:       title,
		newRoute:       route,
		status:         "select a session or press n for new",
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
			model, err := p.SelectedSessionModel(context.Background())
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
	header := headerStyle.Width(width).Render("CyberAgent Workbench  sessions")
	panel := panelStyle.Width(width).Height(contentHeight).Render(p.renderSessions(width-4, contentHeight-2))
	status := statusStyle.Width(width).Render(p.status)
	footer := footerStyle.Width(width).Render("Enter open | n new | j/k select | r refresh | q/Esc quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, panel, status, footer)
}

func (p *Picker) Refresh(ctx context.Context) error {
	sessions, err := p.sessionManager.List(ctx)
	if err != nil {
		return err
	}
	p.sessions = sessions
	p.normalizeSelection()
	return nil
}

func (p *Picker) SelectNext() {
	if len(p.sessions) == 0 {
		p.status = "no sessions; press n for new"
		return
	}
	p.selected++
	p.normalizeSelection()
	p.status = "selected " + p.sessions[p.selected].ID
}

func (p *Picker) SelectPrevious() {
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
	lines := []string{"Sessions"}
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
	return strings.Join(lines, "\n")
}

func (p *Picker) normalizeSelection() {
	if len(p.sessions) == 0 {
		p.selected = 0
		return
	}
	if p.selected < 0 {
		p.selected = 0
	}
	if p.selected >= len(p.sessions) {
		p.selected = len(p.sessions) - 1
	}
}

func RunPicker(picker *Picker) error {
	_, err := tea.NewProgram(picker, tea.WithAltScreen()).Run()
	return err
}
