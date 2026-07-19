package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"

	git "github.com/go-git/go-git/v5"
)

func TestInspectFileHistoryReturnsBoundedExactPathMetadata(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "internal", "check.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	commit := func(message string) {
		t.Helper()
		if _, err := worktree.Add("internal/check.go"); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Commit(message, &git.CommitOptions{Author: testSignature()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte("package internal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commit("add check")
	if err := os.WriteFile(path, []byte("package internal\n\nconst Checked = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commit("SESSION_SECRET=history-subject-secret")
	if _, err := worktree.Remove("internal/check.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("remove check", &git.CommitOptions{Author: testSignature()}); err != nil {
		t.Fatal(err)
	}

	history, err := InspectFileHistory(t.Context(), root, "workspace-1", "internal/check.go")
	if err != nil {
		t.Fatal(err)
	}
	if history.ProtocolVersion != FileHistoryProtocolVersion || history.WorkspaceID != "workspace-1" ||
		history.Kind != "git" || !history.Available || history.Head == "" ||
		history.Path != "internal/check.go" || history.ScannedCommitCount != 3 ||
		history.ReturnedEntryCount != 3 || len(history.Entries) != 3 || !history.Observed ||
		history.RedactionCount != 1 || !history.FirstParentOnly || history.RenameInferred ||
		!history.MetadataOnly || !history.ReadOnly || history.AuthorityGranted ||
		history.RootPathExposed || history.AuthorIdentityIncluded || history.CommitBodyIncluded ||
		history.FileContentIncluded || history.PatchIncluded || history.RemoteConfigIncluded ||
		history.CheckoutPerformed || history.ReferenceUpdated || history.ProcessStarted ||
		history.NetworkUsed || history.HooksExecuted {
		t.Fatalf("unexpected file history: %#v", history)
	}
	if history.Entries[0].Change != "deleted" || history.Entries[0].PreviousKind != "regular" ||
		history.Entries[0].CurrentKind != "" || !history.Entries[0].ContentChanged ||
		history.Entries[1].Change != "modified" || !history.Entries[1].Redacted ||
		!strings.Contains(history.Entries[1].Subject, "[REDACTED:secret]") ||
		history.Entries[2].Change != "added" || history.Entries[2].CurrentKind != "regular" {
		t.Fatalf("unexpected file history entries: %#v", history.Entries)
	}
}

func TestInspectFileHistoryRejectsUnsafePathAndBoundsScan(t *testing.T) {
	if _, err := InspectFileHistory(t.Context(), t.TempDir(), "workspace-1", "../secret"); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("unsafe path returned %v", err)
	}
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	history, err := InspectFileHistory(t.Context(), root, "workspace-1", "missing.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !history.Available || history.Observed || history.ReturnedEntryCount != 0 ||
		len(history.Entries) != 0 || !history.MetadataOnly || !history.ReadOnly {
		t.Fatalf("unexpected empty history: %#v", history)
	}
}
