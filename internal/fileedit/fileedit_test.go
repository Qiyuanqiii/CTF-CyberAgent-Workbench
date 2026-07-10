package fileedit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryStore struct {
	edits map[string]Edit
}

func newMemoryStore() *memoryStore {
	return &memoryStore{edits: map[string]Edit{}}
}

func (s *memoryStore) SaveFileEdit(ctx context.Context, edit Edit) (Edit, error) {
	s.edits[edit.ID] = edit
	return edit, nil
}

func (s *memoryStore) GetFileEdit(ctx context.Context, id string) (Edit, error) {
	edit, ok := s.edits[id]
	if !ok {
		return Edit{}, errors.New("edit not found")
	}
	return edit, nil
}

func (s *memoryStore) ListFileEdits(ctx context.Context, filter ListFilter) ([]Edit, error) {
	var edits []Edit
	for _, edit := range s.edits {
		if filter.WorkspaceID != "" && edit.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if filter.SessionID != "" && edit.SessionID != filter.SessionID {
			continue
		}
		if filter.Status != "" && edit.Status != filter.Status {
			continue
		}
		edits = append(edits, edit)
	}
	return edits, nil
}

func TestManagerProposeAndApproveExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("# Old\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(newMemoryStore())
	edit, err := manager.Propose(context.Background(), Proposal{
		WorkspaceID: "ws-demo", WorkspaceRoot: root, Path: "README.md", ProposedText: "# New\n\nBody\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if edit.Status != StatusProposed || !strings.Contains(edit.Diff, "-# Old") || !strings.Contains(edit.Diff, "+# New") {
		t.Fatalf("unexpected proposal: %#v", edit)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != "# Old\n\nBody\n" {
		t.Fatalf("proposal changed file: %q", before)
	}
	applied, err := manager.Approve(context.Background(), edit.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != StatusApplied {
		t.Fatalf("expected applied status, got %#v", applied)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "# New\n\nBody\n" {
		t.Fatalf("unexpected applied content: %q", after)
	}
}

func TestManagerCreatesNewFileUnderExistingDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(newMemoryStore())
	edit, err := manager.Propose(context.Background(), Proposal{
		WorkspaceID: "ws-demo", WorkspaceRoot: root, Path: "scripts/new.go", ProposedText: "package main\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(edit.Diff, "+package main") {
		t.Fatalf("unexpected creation diff: %s", edit.Diff)
	}
	if _, err := manager.Approve(context.Background(), edit.ID, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "scripts", "new.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main\n" {
		t.Fatalf("unexpected new file: %q", data)
	}
}

func TestManagerRejectsTraversalAndStaleProposal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(newMemoryStore())
	if _, err := manager.Propose(context.Background(), Proposal{
		WorkspaceID: "ws-demo", WorkspaceRoot: root, Path: "../outside.txt", ProposedText: "bad\n",
	}); err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
	edit, err := manager.Propose(context.Background(), Proposal{
		WorkspaceID: "ws-demo", WorkspaceRoot: root, Path: "notes.txt", ProposedText: "new\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failed, err := manager.Approve(context.Background(), edit.ID, root)
	if err == nil || !strings.Contains(err.Error(), "changed after") {
		t.Fatalf("expected stale proposal failure, got edit=%#v err=%v", failed, err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("expected failed status, got %#v", failed)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "changed elsewhere\n" {
		t.Fatalf("stale approval overwrote file: %q", data)
	}
}

func TestManagerRedactsSecretsBeforePersistenceAndWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "outputs"), 0o755); err != nil {
		t.Fatal(err)
	}
	token := "t" + "p-" + strings.Repeat("a", 40)
	manager := NewManager(newMemoryStore())
	edit, err := manager.Propose(context.Background(), Proposal{
		WorkspaceID: "ws-demo", WorkspaceRoot: root, Path: "outputs/env.txt", ProposedText: "MIMO_API_KEY=" + token + "\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !edit.SecretsRedacted || strings.Contains(edit.ProposedText, token[:11]) {
		t.Fatalf("secret was not redacted: %#v", edit)
	}
	if _, err := manager.Approve(context.Background(), edit.ID, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "outputs", "env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token[:11]) || !strings.Contains(string(data), "[REDACTED:secret]") {
		t.Fatalf("secret reached workspace file: %q", data)
	}
}

func TestManagerShowsSafeDiffWhenRedactedPreviewsMatch(t *testing.T) {
	root := t.TempDir()
	token := "t" + "p-" + strings.Repeat("b", 40)
	raw := "MIMO_API_KEY=" + token + "\n"
	path := filepath.Join(root, "env.txt")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(newMemoryStore())
	edit, err := manager.Propose(context.Background(), Proposal{
		WorkspaceID: "ws-demo", WorkspaceRoot: root, Path: "env.txt", ProposedText: "MIMO_API_KEY=[REDACTED:secret]\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(edit.Diff, "existing sensitive content omitted") || strings.Contains(edit.Diff, token[:11]) {
		t.Fatalf("unexpected safe redaction diff: %s", edit.Diff)
	}
}

func TestUnifiedDiffHandlesEmptyFile(t *testing.T) {
	diff := UnifiedDiff("new.txt", "", "hello\n")
	if !strings.Contains(diff, "@@ -0,0 +1,1 @@") || !strings.Contains(diff, "+hello") {
		t.Fatalf("unexpected diff: %s", diff)
	}
}
