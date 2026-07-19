package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestInspectHistoryProjectsBoundedLocalMetadata(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "tracked.txt")
	commitFile := func(message, content string, when time.Time) plumbing.Hash {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Add("tracked.txt"); err != nil {
			t.Fatal(err)
		}
		hash, err := worktree.Commit(message, &git.CommitOptions{Author: &object.Signature{
			Name: "Private Author", Email: "private@example.invalid", When: when,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	first := commitFile("initial body\nprivate second line", "one\n",
		time.Date(2026, 7, 17, 8, 0, 0, 0, time.FixedZone("test", 3600)))
	secret := "sk-123456789012345678901234567890"
	second := commitFile("rotate "+secret+"\nbody must stay private", "two\n",
		time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC))
	if err := repo.CreateBranch(&config.Branch{Name: "feature-safe"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("feature-safe"), first)); err != nil {
		t.Fatal(err)
	}

	history, err := InspectHistory(context.Background(), root, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if history.ProtocolVersion != HistoryProtocolVersion || history.Kind != "git" ||
		!history.Available || history.Head != shortHash(second.String()) || history.Detached ||
		history.ReturnedCommitCount != 2 || history.ReturnedBranchCount != 2 {
		t.Fatalf("unexpected repository history: %#v", history)
	}
	if history.Commits[0].Hash != shortHash(second.String()) ||
		!strings.Contains(history.Commits[0].Subject, "[REDACTED:") ||
		strings.Contains(history.Commits[0].Subject, secret) ||
		strings.Contains(history.Commits[1].Subject, "private second line") ||
		history.RedactionCount != 1 {
		t.Fatalf("commit subjects were not safely projected: %#v", history.Commits)
	}
	if history.Branches[0].Name != "feature-safe" || history.Branches[1].Name != "master" ||
		!history.Branches[1].Current {
		t.Fatalf("local branches were not deterministically projected: %#v", history.Branches)
	}
	if !history.FirstParentOnly || !history.ReadOnly || history.RootPathExposed ||
		history.AuthorIdentityIncluded || history.CommitBodyIncluded ||
		history.RemoteConfigIncluded || history.ProcessStarted || history.NetworkUsed ||
		history.HooksExecuted {
		t.Fatalf("repository history widened authority or privacy: %#v", history)
	}
	if !history.Commits[1].CommittedAt.Equal(time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("commit time was not normalized to UTC: %s", history.Commits[1].CommittedAt)
	}
}

func TestInspectHistoryStopsAtFirstParentLimit(t *testing.T) {
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "tracked.txt")
	for index := 0; index < MaxHistoryCommits+1; index++ {
		if err := os.WriteFile(path, []byte(strings.Repeat("x", index+1)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Add("tracked.txt"); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Commit("commit", &git.CommitOptions{Author: &object.Signature{
			Name: "test", Email: "test@example.invalid",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := InspectHistory(context.Background(), root, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Commits) != MaxHistoryCommits || !history.Truncated ||
		history.ReturnedCommitCount != MaxHistoryCommits {
		t.Fatalf("commit history was not bounded: count=%d history=%#v",
			len(history.Commits), history)
	}
}

func TestInspectHistoryDoesNotDiscoverParentRepository(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	history, err := InspectHistory(context.Background(), child, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if history.Available || history.Kind != "none" || len(history.Commits) != 0 ||
		len(history.Branches) != 0 {
		t.Fatalf("parent repository was discovered: %#v", history)
	}
}

func TestHistoryMetadataCountersSaturateAtProtocolBounds(t *testing.T) {
	if count, bounded := boundedHistoryParentCount(MaxHistoryParentCount + 1); !bounded ||
		count != MaxHistoryParentCount {
		t.Fatalf("parent count was not bounded: count=%d bounded=%t", count, bounded)
	}
	history := History{OmittedBranchCount: MaxHistoryBranchScan}
	incrementOmittedHistoryBranch(&history)
	if history.OmittedBranchCount != MaxHistoryBranchScan {
		t.Fatalf("omitted branch count exceeded its bound: %d", history.OmittedBranchCount)
	}
}
