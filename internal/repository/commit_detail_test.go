package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestInspectCommitDetailProjectsExactBoundedTreeMetadata(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.txt", "one\n")
	write("remove.txt", "gone\n")
	first, err := worktree.Commit("initial subject\nprivate body", &git.CommitOptions{
		Author: &object.Signature{Name: "Private Author", Email: "private@example.invalid",
			When: time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	write("keep.txt", "two\n")
	write("added.txt", "new\n")
	secretPath := "sk-" + strings.Repeat("x", 32) + ".txt"
	write(secretPath, "must not be returned\n")
	if _, err := worktree.Remove("remove.txt"); err != nil {
		t.Fatal(err)
	}
	second, err := worktree.Commit("bounded detail\nbody remains private", &git.CommitOptions{
		Author: &object.Signature{Name: "Another Author", Email: "another@example.invalid",
			When: time.Date(2026, 7, 19, 10, 30, 0, 0, time.FixedZone("test", 3600))},
	})
	if err != nil {
		t.Fatal(err)
	}

	detail, err := InspectCommitDetail(context.Background(), root, "workspace-1", second.String())
	if err != nil {
		t.Fatal(err)
	}
	if detail.ProtocolVersion != CommitDetailProtocolVersion || detail.Kind != "git" ||
		!detail.Available || detail.ObjectID != second.String() || detail.Hash != shortHash(second.String()) ||
		detail.ParentCount != 1 || detail.ChangedFileCount != 4 ||
		detail.ReturnedChangeCount != 3 || detail.OmittedChangeCount != 1 ||
		detail.RedactionCount != 1 || !detail.Truncated {
		t.Fatalf("unexpected exact commit detail: %#v", detail)
	}
	if !detail.CommittedAt.Equal(time.Date(2026, 7, 19, 9, 30, 0, 0, time.UTC)) ||
		detail.Subject != "bounded detail" {
		t.Fatalf("commit metadata was not normalized: %#v", detail)
	}
	want := map[string]string{"added.txt": "added", "keep.txt": "modified", "remove.txt": "deleted"}
	for _, change := range detail.Changes {
		if want[change.Path] != change.Change || !change.ContentChanged ||
			(change.PreviousKind == "" && change.Change != "added") ||
			(change.CurrentKind == "" && change.Change != "deleted") ||
			strings.Contains(change.Path, "sk-") {
			t.Fatalf("unsafe or incorrect changed-file metadata: %#v", change)
		}
		delete(want, change.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing changed-file metadata: %#v", want)
	}
	if !detail.FirstParentOnly || !detail.ReadOnly || detail.RootPathExposed ||
		detail.AuthorIdentityIncluded || detail.CommitBodyIncluded || detail.FileContentIncluded ||
		detail.PatchIncluded || detail.RemoteConfigIncluded || detail.CheckoutPerformed ||
		detail.ReferenceUpdated || detail.ProcessStarted || detail.NetworkUsed || detail.HooksExecuted {
		t.Fatalf("commit detail widened privacy or authority: %#v", detail)
	}

	rootDetail, err := InspectCommitDetail(context.Background(), root, "workspace-1", first.String())
	if err != nil || rootDetail.ParentCount != 0 || rootDetail.ChangedFileCount != 2 {
		t.Fatalf("root commit was not compared with an empty tree: %#v err=%v", rootDetail, err)
	}
	history, err := InspectHistory(context.Background(), root, "workspace-1")
	if err != nil || history.Commits[0].ObjectID != second.String() ||
		history.Commits[1].ObjectID != first.String() {
		t.Fatalf("history did not expose exact immutable object ids: %#v err=%v", history, err)
	}
}

func TestInspectCommitDetailFailsClosedWhenSubtreeObjectIsMissing(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "value.txt"), []byte("value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("nested/value.txt"); err != nil {
		t.Fatal(err)
	}
	commitID, err := worktree.Commit("nested tree", &git.CommitOptions{Author: &object.Signature{
		Name: "Test", Email: "test@example.invalid", When: time.Now().UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(commitID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	nested, err := tree.Tree("nested")
	if err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(root, ".git", "objects", nested.Hash.String()[:2], nested.Hash.String()[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatalf("remove loose subtree object: %v", err)
	}
	_, err = InspectCommitDetail(context.Background(), root, "workspace-1", commitID.String())
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("missing subtree did not fail closed: %v", err)
	}
}

func TestInspectCommitDetailRejectsInvalidOrMissingObject(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", strings.Repeat("a", 39), strings.Repeat("A", 40),
		strings.Repeat("z", 40)} {
		_, err := InspectCommitDetail(context.Background(), root, "workspace-1", value)
		if apperror.CodeOf(err) != apperror.CodeInvalidArgument {
			t.Fatalf("invalid object id %q returned %v", value, err)
		}
	}
	_, err := InspectCommitDetail(context.Background(), root, "workspace-1", strings.Repeat("a", 40))
	if apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("missing exact object returned %v", err)
	}
	none, err := InspectCommitDetail(context.Background(), t.TempDir(), "workspace-1",
		strings.Repeat("a", 40))
	if err != nil || none.Available || none.Kind != "none" || none.ObjectID != strings.Repeat("a", 40) {
		t.Fatalf("non-repository projection changed contract: %#v err=%v", none, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectCommitDetail(cancelled, root, "workspace-1", strings.Repeat("a", 40)); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled detail returned %v", err)
	}
}

func TestProjectCommitTreeChangesCapsReturnedAndOmittedCounts(t *testing.T) {
	previous := make(map[string]commitTreeEntry, MaxCommitChangedFiles+1)
	current := make(map[string]commitTreeEntry, MaxCommitChangedFiles+1)
	for index := 0; index < MaxCommitChangedFiles+1; index++ {
		path := fmt.Sprintf("file-%03d.txt", index)
		previous[path] = commitTreeEntry{hash: plumbing.NewHash(strings.Repeat("a", 40)),
			mode: filemode.Regular}
		current[path] = commitTreeEntry{hash: plumbing.NewHash(strings.Repeat("b", 40)),
			mode: filemode.Regular}
	}
	result := CommitDetail{Changes: []CommitFileChange{}}
	projectCommitTreeChanges(previous, current, &result)
	if result.ChangedFileCount != MaxCommitChangedFiles+1 ||
		result.ReturnedChangeCount != MaxCommitChangedFiles || result.OmittedChangeCount != 1 ||
		!result.Truncated {
		t.Fatalf("commit change projection was not bounded: %#v", result)
	}
	result.OmittedChangeCount = MaxCommitOmittedFiles
	incrementCommitOmitted(&result)
	if result.OmittedChangeCount != MaxCommitOmittedFiles {
		t.Fatalf("omitted counter exceeded protocol bound: %d", result.OmittedChangeCount)
	}
	result.RedactionCount = MaxCommitOmittedFiles - 1
	incrementCommitRedactions(&result, 10)
	if result.RedactionCount != MaxCommitOmittedFiles || !result.Truncated {
		t.Fatalf("redaction counter exceeded protocol bound: %#v", result)
	}
}
