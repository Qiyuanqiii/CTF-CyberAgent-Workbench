package repository

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/redact"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

const (
	HistoryProtocolVersion = "repository_history.v1"
	MaxHistoryCommits      = 50
	MaxHistoryBranches     = 64
	MaxHistoryBranchScan   = 1024
	MaxHistoryParentCount  = 1024
	MaxCommitSubjectRunes  = 240
)

type HistoryCommit struct {
	Hash         string    `json:"hash"`
	Subject      string    `json:"subject"`
	ParentCount  int       `json:"parent_count"`
	CommittedAt  time.Time `json:"committed_at"`
	Redacted     bool      `json:"redacted"`
	SubjectBound bool      `json:"subject_bounded"`
}

type HistoryBranch struct {
	Name    string `json:"name"`
	Head    string `json:"head"`
	Current bool   `json:"current"`
}

type History struct {
	ProtocolVersion        string          `json:"protocol_version"`
	WorkspaceID            string          `json:"workspace_id"`
	Kind                   string          `json:"kind"`
	Available              bool            `json:"available"`
	Head                   string          `json:"head"`
	Detached               bool            `json:"detached"`
	Commits                []HistoryCommit `json:"commits"`
	Branches               []HistoryBranch `json:"branches"`
	ReturnedCommitCount    int             `json:"returned_commit_count"`
	ReturnedBranchCount    int             `json:"returned_branch_count"`
	OmittedBranchCount     int             `json:"omitted_branch_count"`
	RedactionCount         int             `json:"redaction_count"`
	Truncated              bool            `json:"truncated"`
	FirstParentOnly        bool            `json:"first_parent_only"`
	ReadOnly               bool            `json:"read_only"`
	RootPathExposed        bool            `json:"root_path_exposed"`
	AuthorIdentityIncluded bool            `json:"author_identity_included"`
	CommitBodyIncluded     bool            `json:"commit_body_included"`
	RemoteConfigIncluded   bool            `json:"remote_config_included"`
	ProcessStarted         bool            `json:"process_started"`
	NetworkUsed            bool            `json:"network_used"`
	HooksExecuted          bool            `json:"hooks_executed"`
}

// InspectHistory returns only bounded local metadata. It follows the first
// parent chain so work is independent of the total commit graph size.
func InspectHistory(ctx context.Context, root string, workspaceID string) (History, error) {
	base := History{ProtocolVersion: HistoryProtocolVersion, WorkspaceID: workspaceID,
		Kind: "none", Commits: []HistoryCommit{}, Branches: []HistoryBranch{},
		FirstParentOnly: true, ReadOnly: true}
	if err := validateIdentity(workspaceID); err != nil {
		return History{}, err
	}
	if err := ctx.Err(); err != nil {
		return History{}, err
	}
	_, repo, available, err := openExactRepository(ctx, root)
	if err != nil {
		return History{}, err
	}
	if !available {
		return base, nil
	}
	base.Kind = "git"
	base.Available = true

	head, err := repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		if err := addHistoryBranches(ctx, repo, nil, &base); err != nil {
			return History{}, err
		}
		return base, nil
	}
	if err != nil {
		return History{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository history HEAD could not be inspected")
	}
	base.Head = shortHash(head.Hash().String())
	base.Detached = !head.Name().IsBranch()
	if err := addHistoryBranches(ctx, repo, head, &base); err != nil {
		return History{}, err
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return History{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository history commit could not be inspected")
	}
	for len(base.Commits) < MaxHistoryCommits {
		if err := ctx.Err(); err != nil {
			return History{}, err
		}
		subject, redactions, redacted, bounded := safeCommitSubject(commit.Message)
		base.RedactionCount += redactions
		base.Truncated = base.Truncated || bounded
		parentCount, parentsBounded := boundedHistoryParentCount(len(commit.ParentHashes))
		base.Truncated = base.Truncated || parentsBounded
		base.Commits = append(base.Commits, HistoryCommit{
			Hash: shortHash(commit.Hash.String()), Subject: subject,
			ParentCount: parentCount, CommittedAt: commit.Committer.When.UTC(),
			Redacted: redacted, SubjectBound: bounded,
		})
		if len(commit.ParentHashes) == 0 {
			break
		}
		if len(base.Commits) == MaxHistoryCommits {
			base.Truncated = true
			break
		}
		commit, err = repo.CommitObject(commit.ParentHashes[0])
		if err != nil {
			return History{}, apperror.New(apperror.CodeFailedPrecondition,
				"repository history parent commit could not be inspected")
		}
	}
	base.ReturnedCommitCount = len(base.Commits)
	return base, nil
}

func addHistoryBranches(ctx context.Context, repo *git.Repository,
	head *plumbing.Reference, result *History,
) error {
	iterator, err := repo.Branches()
	if err != nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"repository local branches could not be inspected")
	}
	branches := make([]HistoryBranch, 0, MaxHistoryBranches)
	scanned := 0
	err = iterator.ForEach(func(reference *plumbing.Reference) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		scanned++
		if scanned > MaxHistoryBranchScan {
			incrementOmittedHistoryBranch(result)
			result.Truncated = true
			return storer.ErrStop
		}
		if reference == nil || reference.Type() != plumbing.HashReference {
			incrementOmittedHistoryBranch(result)
			result.Truncated = true
			return nil
		}
		name, unsafe, redactions := safeReference(reference.Name().Short(), false, 0)
		result.RedactionCount += redactions
		if unsafe {
			incrementOmittedHistoryBranch(result)
			result.Truncated = true
			return nil
		}
		branches = append(branches, HistoryBranch{Name: name,
			Head:    shortHash(reference.Hash().String()),
			Current: head != nil && head.Name() == reference.Name()})
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		return apperror.New(apperror.CodeFailedPrecondition,
			"repository local branches could not be inspected")
	}
	sort.Slice(branches, func(left int, right int) bool {
		return branches[left].Name < branches[right].Name
	})
	if len(branches) > MaxHistoryBranches {
		result.OmittedBranchCount += len(branches) - MaxHistoryBranches
		branches = branches[:MaxHistoryBranches]
		result.Truncated = true
	}
	result.Branches = branches
	result.ReturnedBranchCount = len(branches)
	return nil
}

func boundedHistoryParentCount(count int) (int, bool) {
	if count > MaxHistoryParentCount {
		return MaxHistoryParentCount, true
	}
	return count, false
}

func incrementOmittedHistoryBranch(result *History) {
	if result.OmittedBranchCount < MaxHistoryBranchScan {
		result.OmittedBranchCount++
	}
}

func safeCommitSubject(message string) (string, int, bool, bool) {
	if !utf8.ValidString(message) {
		return "[unavailable subject]", 0, false, true
	}
	message = strings.ReplaceAll(message, "\r\n", "\n")
	if index := strings.IndexByte(message, '\n'); index >= 0 {
		message = message[:index]
	}
	var cleaned strings.Builder
	bounded := false
	for _, current := range strings.TrimSpace(message) {
		if unicode.IsControl(current) {
			cleaned.WriteByte(' ')
			bounded = true
			continue
		}
		cleaned.WriteRune(current)
	}
	message = strings.Join(strings.Fields(cleaned.String()), " ")
	if message == "" {
		message = "[no subject]"
	}
	runes := []rune(message)
	if len(runes) > MaxCommitSubjectRunes {
		message = string(runes[:MaxCommitSubjectRunes])
		bounded = true
	}
	projection := redact.Text(message)
	redactions := 0
	for _, finding := range projection.Findings {
		redactions += finding.Count
	}
	return projection.Text, redactions, projection.Text != message, bounded
}
