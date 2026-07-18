package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cyberagent-workbench/internal/agent"
	"cyberagent-workbench/internal/session"
)

type Manager struct {
	Home  string
	Store WorkspaceStore
}

type WorkspaceStore interface {
	SaveWorkspace(ctx context.Context, rec session.WorkspaceRecord) error
	GetWorkspaceByName(ctx context.Context, name string) (session.WorkspaceRecord, error)
	ListWorkspaces(ctx context.Context) ([]session.WorkspaceRecord, error)
}

func NewManager(home string, st WorkspaceStore) *Manager {
	return &Manager{Home: home, Store: st}
}

func (m *Manager) Init(ctx context.Context, name string) (session.WorkspaceRecord, error) {
	slug := Slug(name)
	root := filepath.Join(m.Home, "workspaces", slug)
	for _, dir := range []string{"attachments", "scripts", "outputs", "logs", "writeups", filepath.Join("tests", "sample_input")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return session.WorkspaceRecord{}, err
		}
	}
	readme := filepath.Join(root, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		content := fmt.Sprintf("# %s\n\nCyberAgent Workbench local workspace.\n", name)
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return session.WorkspaceRecord{}, err
		}
	}
	rec := session.WorkspaceRecord{
		ID:        "ws-" + slug,
		Name:      slug,
		RootPath:  root,
		CreatedAt: time.Now().UTC(),
	}
	if err := m.Store.SaveWorkspace(ctx, rec); err != nil {
		return session.WorkspaceRecord{}, err
	}
	return rec, nil
}

func (m *Manager) Ensure(ctx context.Context, name string) (session.WorkspaceRecord, error) {
	slug := Slug(name)
	rec, err := m.Store.GetWorkspaceByName(ctx, slug)
	if err == nil {
		return rec, nil
	}
	return m.Init(ctx, slug)
}

func (m *Manager) ScriptPath(rec session.WorkspaceRecord, task agent.Task, ext string) string {
	if ext == "" {
		ext = ".txt"
	}
	name := Slug(task.Goal)
	if len(name) > 40 {
		name = name[:40]
	}
	return filepath.Join(rec.RootPath, "scripts", name+"-"+shortID(task.ID)+ext)
}

func (m *Manager) WriteupPath(rec session.WorkspaceRecord) string {
	return filepath.Join(rec.RootPath, "writeups", "writeup.md")
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func Slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "workspace"
	}
	return value
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}
