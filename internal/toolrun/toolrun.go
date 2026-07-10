package toolrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
)

const (
	StatusProposed  = "proposed"
	StatusApproved  = "approved"
	StatusDenied    = "denied"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const ShellTool = "shell"

type ToolRun struct {
	ID           string
	SessionID    string
	WorkspaceID  string
	ToolName     string
	Command      string
	Status       string
	Risk         string
	PolicyReason string
	Stdout       string
	Stderr       string
	ExitCode     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ListFilter struct {
	SessionID string
	Status    string
}

type Store interface {
	SaveToolRun(ctx context.Context, run ToolRun) (ToolRun, error)
	GetToolRun(ctx context.Context, id string) (ToolRun, error)
	ListToolRuns(ctx context.Context, filter ListFilter) ([]ToolRun, error)
}

type Manager struct {
	store   Store
	checker policy.Checker
}

func NewManager(store Store, checker policy.Checker) *Manager {
	return &Manager{store: store, checker: checker}
}

func (m *Manager) ProposeShell(ctx context.Context, sessionID string, workspaceID string, command string) (ToolRun, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return ToolRun{}, errors.New("command is required")
	}
	decision := m.checker.CheckText("tool_run.shell", command)
	redactedCommand := redact.String(command)
	status := StatusProposed
	if !decision.Allowed {
		status = StatusDenied
	}
	now := time.Now().UTC()
	run := ToolRun{
		ID:           newID("tool"),
		SessionID:    sessionID,
		WorkspaceID:  workspaceID,
		ToolName:     ShellTool,
		Command:      redactedCommand,
		Status:       status,
		Risk:         decision.Risk,
		PolicyReason: redact.String(decision.Reason),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return m.store.SaveToolRun(ctx, run)
}

func (m *Manager) Approve(ctx context.Context, id string) (ToolRun, error) {
	run, err := m.store.GetToolRun(ctx, id)
	if err != nil {
		return ToolRun{}, err
	}
	if run.Status == StatusCompleted {
		return run, nil
	}
	if run.Status != StatusProposed && run.Status != StatusApproved {
		return ToolRun{}, fmt.Errorf("tool run %s is %s, not %s", run.ID, run.Status, StatusProposed)
	}
	if run.Status == StatusProposed {
		run.Status = StatusApproved
		run.UpdatedAt = time.Now().UTC()
		run, err = m.store.SaveToolRun(ctx, run)
		if err != nil {
			return ToolRun{}, err
		}
	}

	// v0.1 approval intentionally uses a dry-run completion instead of executing.
	run.Status = StatusCompleted
	run.Stdout = "dry run: " + run.Command
	run.Stderr = ""
	run.ExitCode = 0
	run.UpdatedAt = time.Now().UTC()
	return m.store.SaveToolRun(ctx, run)
}

func (m *Manager) Deny(ctx context.Context, id string, reason string) (ToolRun, error) {
	run, err := m.store.GetToolRun(ctx, id)
	if err != nil {
		return ToolRun{}, err
	}
	if run.Status == StatusDenied {
		return run, nil
	}
	if run.Status != StatusProposed {
		return ToolRun{}, fmt.Errorf("tool run %s is %s, not %s", run.ID, run.Status, StatusProposed)
	}
	run.Status = StatusDenied
	if strings.TrimSpace(reason) != "" {
		run.PolicyReason = strings.TrimSpace(reason)
	}
	run.UpdatedAt = time.Now().UTC()
	return m.store.SaveToolRun(ctx, run)
}

func (m *Manager) List(ctx context.Context, filter ListFilter) ([]ToolRun, error) {
	return m.store.ListToolRuns(ctx, filter)
}

func (m *Manager) Get(ctx context.Context, id string) (ToolRun, error) {
	return m.store.GetToolRun(ctx, id)
}

func newID(prefix string) string {
	return idgen.New(prefix)
}
