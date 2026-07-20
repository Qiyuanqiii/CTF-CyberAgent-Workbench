package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestInspectCommitComparisonProjectsExactMetadataOnlyChanges(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Add(path); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.txt", "base\n")
	write("remove.txt", "remove\n")
	base, err := worktree.Commit("base subject\nprivate body", &git.CommitOptions{
		Author: &object.Signature{Name: "Private", Email: "private@example.invalid",
			When: time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
	write("keep.txt", "head\n")
	write("added.txt", "added\n")
	secretPath := "sk-" + strings.Repeat("x", 32) + ".txt"
	write(secretPath, "private\n")
	if _, err := worktree.Remove("remove.txt"); err != nil {
		t.Fatal(err)
	}
	head, err := worktree.Commit("head subject\nprivate body", &git.CommitOptions{
		Author: &object.Signature{Name: "Private", Email: "private@example.invalid",
			When: time.Date(2026, 7, 19, 2, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}

	comparison, err := InspectCommitComparison(t.Context(), root, "workspace-1",
		base.String(), head.String())
	if err != nil {
		t.Fatal(err)
	}
	if comparison.ProtocolVersion != CommitComparisonProtocolVersion ||
		comparison.WorkspaceID != "workspace-1" || comparison.Kind != "git" ||
		!comparison.Available || comparison.BaseObjectID != base.String() ||
		comparison.HeadObjectID != head.String() || comparison.BaseHash != shortHash(base.String()) ||
		comparison.HeadHash != shortHash(head.String()) || comparison.BaseSubject != "base subject" ||
		comparison.HeadSubject != "head subject" || comparison.SameObject ||
		comparison.ChangedFileCount != 4 || comparison.ReturnedChangeCount != 3 ||
		comparison.OmittedChangeCount != 1 || comparison.RedactionCount != 1 ||
		!comparison.Truncated {
		t.Fatalf("unexpected commit comparison: %#v", comparison)
	}
	want := map[string]string{"added.txt": "added", "keep.txt": "modified",
		"remove.txt": "deleted"}
	for _, change := range comparison.Changes {
		if want[change.Path] != change.Change || strings.Contains(change.Path, "sk-") {
			t.Fatalf("unsafe or incorrect comparison change: %#v", change)
		}
		delete(want, change.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing comparison changes: %#v", want)
	}
	if !comparison.MetadataOnly || !comparison.ReadOnly || comparison.RenameInferred ||
		comparison.AncestorRequired || comparison.AuthorityGranted || comparison.RootPathExposed ||
		comparison.AuthorIdentityIncluded || comparison.CommitBodyIncluded ||
		comparison.FileContentIncluded || comparison.PatchIncluded ||
		comparison.RemoteConfigIncluded || comparison.CheckoutPerformed ||
		comparison.ReferenceUpdated || comparison.ProcessStarted || comparison.NetworkUsed ||
		comparison.HooksExecuted {
		t.Fatalf("commit comparison widened privacy or authority: %#v", comparison)
	}

	same, err := InspectCommitComparison(t.Context(), root, "workspace-1",
		head.String(), head.String())
	if err != nil || !same.SameObject || same.ChangedFileCount != 0 || len(same.Changes) != 0 {
		t.Fatalf("same-object comparison diverged: %#v err=%v", same, err)
	}
}

func TestInspectCommitComparisonRejectsInvalidMissingAndCancelledRequests(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	valid := strings.Repeat("a", 40)
	for _, request := range [][2]string{{"", valid}, {valid, strings.Repeat("A", 40)},
		{strings.Repeat("z", 40), valid}} {
		_, err := InspectCommitComparison(t.Context(), root, "workspace-1", request[0], request[1])
		if apperror.CodeOf(err) != apperror.CodeInvalidArgument {
			t.Fatalf("invalid comparison ids=%q returned %v", request, err)
		}
	}
	_, err := InspectCommitComparison(t.Context(), root, "workspace-1", valid, valid)
	if apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("missing comparison object returned %v", err)
	}
	none, err := InspectCommitComparison(t.Context(), t.TempDir(), "workspace-1", valid, valid)
	if err != nil || none.Available || none.Kind != "none" || !none.SameObject ||
		none.BaseObjectID != valid || none.HeadObjectID != valid || !none.MetadataOnly ||
		!none.ReadOnly {
		t.Fatalf("non-repository comparison changed contract: %#v err=%v", none, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectCommitComparison(cancelled, root, "workspace-1", valid, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled comparison returned %v", err)
	}
}
