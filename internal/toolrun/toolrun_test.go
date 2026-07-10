package toolrun

import (
	"context"
	"sort"
	"strings"
	"testing"

	"cyberagent-workbench/internal/policy"
)

type memoryStore struct {
	runs map[string]ToolRun
}

func newMemoryStore() *memoryStore {
	return &memoryStore{runs: map[string]ToolRun{}}
}

func (m *memoryStore) SaveToolRun(ctx context.Context, run ToolRun) (ToolRun, error) {
	m.runs[run.ID] = run
	return run, nil
}

func (m *memoryStore) GetToolRun(ctx context.Context, id string) (ToolRun, error) {
	run, ok := m.runs[id]
	if !ok {
		return ToolRun{}, errNotFound(id)
	}
	return run, nil
}

func (m *memoryStore) ListToolRuns(ctx context.Context, filter ListFilter) ([]ToolRun, error) {
	var out []ToolRun
	for _, run := range m.runs {
		if filter.SessionID != "" && run.SessionID != filter.SessionID {
			continue
		}
		if filter.Status != "" && run.Status != filter.Status {
			continue
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

type errNotFound string

func (e errNotFound) Error() string { return "not found: " + string(e) }

func TestApproveCompletesDryRun(t *testing.T) {
	store := newMemoryStore()
	manager := NewManager(store, policy.NewDefaultChecker())
	run, err := manager.ProposeShell(context.Background(), "sess-1", "ws-1", "echo hello")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusProposed {
		t.Fatalf("expected proposed, got %s", run.Status)
	}
	run, err = manager.Approve(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusCompleted || run.Stdout != "dry run: echo hello" || run.ExitCode != 0 {
		t.Fatalf("unexpected approved run: %#v", run)
	}
}

func TestDangerousProposalIsDenied(t *testing.T) {
	store := newMemoryStore()
	manager := NewManager(store, policy.NewDefaultChecker())
	run, err := manager.ProposeShell(context.Background(), "sess-1", "ws-1", "masscan 0.0.0.0/0")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusDenied {
		t.Fatalf("expected denied, got %#v", run)
	}
}

func TestProposalRedactsCommandSecrets(t *testing.T) {
	store := newMemoryStore()
	manager := NewManager(store, policy.NewDefaultChecker())
	mimoToken := "t" + "p-" + strings.Repeat("a", 40)
	run, err := manager.ProposeShell(context.Background(), "sess-1", "ws-1", "echo "+mimoToken)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(run.Command, mimoToken[:11]) {
		t.Fatalf("secret stored in command: %#v", run)
	}
	if !strings.Contains(run.Command, "[REDACTED:mimo-token]") {
		t.Fatalf("expected redacted command, got %q", run.Command)
	}
}
