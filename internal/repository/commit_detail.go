package repository

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	CommitDetailProtocolVersion = "repository_commit_detail.v1"
	MaxCommitChangedFiles       = 200
	MaxCommitTreeEntries        = 20_000
	MaxCommitTreeDepth          = 128
	MaxCommitOmittedFiles       = MaxCommitTreeEntries * 2
)

type CommitFileChange struct {
	Path           string `json:"path"`
	Change         string `json:"change"`
	PreviousKind   string `json:"previous_kind"`
	CurrentKind    string `json:"current_kind"`
	ContentChanged bool   `json:"content_changed"`
	ModeChanged    bool   `json:"mode_changed"`
}

type CommitDetail struct {
	ProtocolVersion        string             `json:"protocol_version"`
	WorkspaceID            string             `json:"workspace_id"`
	Kind                   string             `json:"kind"`
	Available              bool               `json:"available"`
	ObjectID               string             `json:"object_id"`
	Hash                   string             `json:"hash"`
	Subject                string             `json:"subject"`
	CommittedAt            time.Time          `json:"committed_at"`
	ParentCount            int                `json:"parent_count"`
	Changes                []CommitFileChange `json:"changes"`
	ChangedFileCount       int                `json:"changed_file_count"`
	ReturnedChangeCount    int                `json:"returned_change_count"`
	OmittedChangeCount     int                `json:"omitted_change_count"`
	RedactionCount         int                `json:"redaction_count"`
	Truncated              bool               `json:"truncated"`
	FirstParentOnly        bool               `json:"first_parent_only"`
	ReadOnly               bool               `json:"read_only"`
	RootPathExposed        bool               `json:"root_path_exposed"`
	AuthorIdentityIncluded bool               `json:"author_identity_included"`
	CommitBodyIncluded     bool               `json:"commit_body_included"`
	FileContentIncluded    bool               `json:"file_content_included"`
	PatchIncluded          bool               `json:"patch_included"`
	RemoteConfigIncluded   bool               `json:"remote_config_included"`
	CheckoutPerformed      bool               `json:"checkout_performed"`
	ReferenceUpdated       bool               `json:"reference_updated"`
	ProcessStarted         bool               `json:"process_started"`
	NetworkUsed            bool               `json:"network_used"`
	HooksExecuted          bool               `json:"hooks_executed"`
}

type commitTreeEntry struct {
	hash plumbing.Hash
	mode filemode.FileMode
}

// InspectCommitDetail compares one exact commit object with its first parent.
// It walks bounded tree metadata and never materializes blob or patch content.
func InspectCommitDetail(ctx context.Context, root string, workspaceID string,
	objectID string,
) (CommitDetail, error) {
	base := CommitDetail{ProtocolVersion: CommitDetailProtocolVersion,
		WorkspaceID: workspaceID, Kind: "none", ObjectID: objectID,
		Changes: []CommitFileChange{}, FirstParentOnly: true, ReadOnly: true}
	if err := validateIdentity(workspaceID); err != nil {
		return CommitDetail{}, err
	}
	if !validCommitObjectID(objectID) {
		return CommitDetail{}, apperror.New(apperror.CodeInvalidArgument,
			"repository commit object identity is invalid")
	}
	if err := ctx.Err(); err != nil {
		return CommitDetail{}, err
	}
	_, repo, available, err := openExactRepository(ctx, root)
	if err != nil {
		return CommitDetail{}, err
	}
	if !available {
		return base, nil
	}
	commit, err := repo.CommitObject(plumbing.NewHash(objectID))
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return CommitDetail{}, apperror.New(apperror.CodeNotFound,
				"repository commit was not found")
		}
		return CommitDetail{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit could not be inspected")
	}
	if commit.Hash.String() != objectID {
		return CommitDetail{}, apperror.New(apperror.CodeConflict,
			"repository commit identity changed")
	}
	base.Kind = "git"
	base.Available = true
	base.Hash = shortHash(objectID)
	base.CommittedAt = commit.Committer.When.UTC()
	base.Subject, base.RedactionCount, _, base.Truncated = safeCommitSubject(commit.Message)
	base.ParentCount, _ = boundedHistoryParentCount(len(commit.ParentHashes))
	if len(commit.ParentHashes) > MaxHistoryParentCount {
		base.Truncated = true
	}

	currentTree, err := commit.Tree()
	if err != nil {
		return CommitDetail{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit tree could not be inspected")
	}
	current, err := collectCommitTreeEntries(ctx, currentTree)
	if err != nil {
		return CommitDetail{}, err
	}
	previous := map[string]commitTreeEntry{}
	if len(commit.ParentHashes) > 0 {
		parent, err := repo.CommitObject(commit.ParentHashes[0])
		if err != nil {
			return CommitDetail{}, apperror.New(apperror.CodeFailedPrecondition,
				"repository commit parent could not be inspected")
		}
		parentTree, err := parent.Tree()
		if err != nil {
			return CommitDetail{}, apperror.New(apperror.CodeFailedPrecondition,
				"repository parent tree could not be inspected")
		}
		previous, err = collectCommitTreeEntries(ctx, parentTree)
		if err != nil {
			return CommitDetail{}, err
		}
	}
	projectCommitTreeChanges(previous, current, &base)
	return base, nil
}

func collectCommitTreeEntries(ctx context.Context,
	tree *object.Tree,
) (map[string]commitTreeEntry, error) {
	if tree == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit tree is unavailable")
	}
	entries := make(map[string]commitTreeEntry)
	visited := 0
	ancestry := make(map[plumbing.Hash]struct{})
	if err := collectCommitTree(ctx, tree, "", 0, &visited, ancestry, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func collectCommitTree(ctx context.Context, tree *object.Tree, prefix string, depth int,
	visited *int, ancestry map[plumbing.Hash]struct{}, entries map[string]commitTreeEntry,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tree == nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"repository commit tree is unavailable")
	}
	if depth > MaxCommitTreeDepth {
		return apperror.New(apperror.CodeResourceExhausted,
			"repository commit tree exceeds the depth limit")
	}
	if _, exists := ancestry[tree.Hash]; exists {
		return apperror.New(apperror.CodeConflict,
			"repository commit tree contains a cycle")
	}
	ancestry[tree.Hash] = struct{}{}
	defer delete(ancestry, tree.Hash)
	localNames := make(map[string]struct{}, len(tree.Entries))
	for _, entry := range tree.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		*visited++
		if *visited > MaxCommitTreeEntries {
			return apperror.New(apperror.CodeResourceExhausted,
				"repository commit tree exceeds the inspection limit")
		}
		if !validCommitTreeEntryName(entry.Name) {
			return apperror.New(apperror.CodeFailedPrecondition,
				"repository commit contains an invalid tree path")
		}
		if _, exists := localNames[entry.Name]; exists {
			return apperror.New(apperror.CodeConflict,
				"repository commit contains duplicate tree entries")
		}
		localNames[entry.Name] = struct{}{}
		name := entry.Name
		if prefix != "" {
			name = prefix + "/" + entry.Name
		}
		if entry.Mode == filemode.Dir {
			child, err := tree.Tree(entry.Name)
			if err != nil {
				return apperror.New(apperror.CodeFailedPrecondition,
					"repository commit subtree could not be inspected")
			}
			if err := collectCommitTree(ctx, child, name, depth+1, visited, ancestry, entries); err != nil {
				return err
			}
			continue
		}
		if _, ok := commitFileKind(entry.Mode); !ok {
			return apperror.New(apperror.CodeFailedPrecondition,
				"repository commit contains an unsupported file mode")
		}
		if _, exists := entries[name]; exists {
			return apperror.New(apperror.CodeConflict,
				"repository commit contains duplicate paths")
		}
		entries[name] = commitTreeEntry{hash: entry.Hash, mode: entry.Mode}
	}
	return nil
}

func validCommitTreeEntryName(value string) bool {
	return value != "" && value != "." && value != ".." &&
		!strings.ContainsAny(value, "/\\") && !strings.ContainsRune(value, 0)
}

func projectCommitTreeChanges(previous map[string]commitTreeEntry,
	current map[string]commitTreeEntry, result *CommitDetail,
) {
	paths := make([]string, 0, len(previous)+len(current))
	seen := make(map[string]struct{}, len(previous)+len(current))
	for path := range previous {
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for path := range current {
		if _, exists := seen[path]; !exists {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		before, hadBefore := previous[path]
		after, hasAfter := current[path]
		if hadBefore && hasAfter && before == after {
			continue
		}
		result.ChangedFileCount++
		canonical, redactions, safe := safePath(path)
		incrementCommitRedactions(result, redactions)
		if !safe {
			incrementCommitOmitted(result)
			result.Truncated = true
			continue
		}
		change := CommitFileChange{Path: canonical}
		if hadBefore {
			change.PreviousKind, _ = commitFileKind(before.mode)
		}
		if hasAfter {
			change.CurrentKind, _ = commitFileKind(after.mode)
		}
		switch {
		case !hadBefore:
			change.Change = "added"
			change.ContentChanged = true
			change.ModeChanged = true
		case !hasAfter:
			change.Change = "deleted"
			change.ContentChanged = true
			change.ModeChanged = true
		default:
			change.Change = "modified"
			change.ContentChanged = before.hash != after.hash
			change.ModeChanged = before.mode != after.mode
		}
		if len(result.Changes) >= MaxCommitChangedFiles {
			incrementCommitOmitted(result)
			result.Truncated = true
			continue
		}
		result.Changes = append(result.Changes, change)
	}
	result.ReturnedChangeCount = len(result.Changes)
}

func commitFileKind(mode filemode.FileMode) (string, bool) {
	switch mode {
	case filemode.Regular, filemode.Deprecated:
		return "regular", true
	case filemode.Executable:
		return "executable", true
	case filemode.Symlink:
		return "symlink", true
	case filemode.Submodule:
		return "submodule", true
	default:
		return "", false
	}
}

func validCommitObjectID(value string) bool {
	if len(value) != 40 || value != strings.ToLower(value) {
		return false
	}
	for _, current := range value {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}

func incrementCommitOmitted(result *CommitDetail) {
	if result.OmittedChangeCount < MaxCommitOmittedFiles {
		result.OmittedChangeCount++
	}
}

func incrementCommitRedactions(result *CommitDetail, count int) {
	if count <= 0 || result.RedactionCount >= MaxCommitOmittedFiles {
		return
	}
	if count > MaxCommitOmittedFiles-result.RedactionCount {
		result.RedactionCount = MaxCommitOmittedFiles
		result.Truncated = true
		return
	}
	result.RedactionCount += count
}
