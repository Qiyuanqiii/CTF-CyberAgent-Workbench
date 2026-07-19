package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestInspectProjectsBoundedReadOnlyStatus(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("tracked.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("initial", &git.CommitOptions{Author: &object.Signature{
		Name: "test", Email: "test@example.invalid",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracked, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := Inspect(context.Background(), root, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.ProtocolVersion != ProtocolVersion || state.Kind != "git" || !state.Available ||
		state.Clean || state.Branch != "master" || len(state.Head) != 12 {
		t.Fatalf("unexpected repository state: %+v", state)
	}
	if state.WorktreeCount != 1 || state.UntrackedCount != 1 || state.StagedCount != 0 ||
		len(state.Changes) != 2 {
		t.Fatalf("unexpected change projection: %+v", state)
	}
	if !state.ReadOnly || state.RootPathExposed || state.ContentIncluded ||
		state.RemoteConfigIncluded || state.ProcessStarted || state.NetworkUsed ||
		state.HooksExecuted {
		t.Fatalf("repository projection widened authority: %+v", state)
	}
}

func TestInspectDoesNotDiscoverParentRepository(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	state, err := Inspect(context.Background(), child, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Available || state.Kind != "none" || len(state.Changes) != 0 {
		t.Fatalf("parent repository was discovered: %+v", state)
	}
}

func TestInspectRejectsRedirectedGitMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(context.Background(), root, "workspace-1"); err == nil {
		t.Fatal("expected redirected metadata to be rejected")
	}
}

func TestInspectRejectsNestedRedirectedGitMetadata(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-head")
	if err := os.WriteFile(outside, []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(root, ".git", "HEAD")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, head); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	_, err := Inspect(context.Background(), root, "workspace-1")
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("expected redirected metadata rejection, got %v", err)
	}
}

func TestInspectOmitsSecretLookingPaths(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	secretPath := "sk-123456789012345678901234.txt"
	if err := os.WriteFile(filepath.Join(root, secretPath), []byte("not a secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := Inspect(context.Background(), root, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Changes) != 0 || state.RedactionCount != 1 || !state.Truncated ||
		state.UntrackedCount != 1 {
		t.Fatalf("secret-looking path was not safely omitted: %+v", state)
	}
}
