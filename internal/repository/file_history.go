package repository

import (
	"context"
	"errors"
	"time"

	"cyberagent-workbench/internal/apperror"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	FileHistoryProtocolVersion = "repository_file_history.v1"
	MaxFileHistoryEntries      = 50
	MaxFileHistoryCommitScan   = 512
)

type FileHistoryEntry struct {
	ObjectID       string    `json:"object_id"`
	Hash           string    `json:"hash"`
	Subject        string    `json:"subject"`
	CommittedAt    time.Time `json:"committed_at"`
	Change         string    `json:"change"`
	PreviousKind   string    `json:"previous_kind"`
	CurrentKind    string    `json:"current_kind"`
	ContentChanged bool      `json:"content_changed"`
	ModeChanged    bool      `json:"mode_changed"`
	Redacted       bool      `json:"redacted"`
	SubjectBounded bool      `json:"subject_bounded"`
}

type FileHistory struct {
	ProtocolVersion        string             `json:"protocol_version"`
	WorkspaceID            string             `json:"workspace_id"`
	Kind                   string             `json:"kind"`
	Available              bool               `json:"available"`
	Head                   string             `json:"head"`
	Path                   string             `json:"path"`
	Entries                []FileHistoryEntry `json:"entries"`
	ScannedCommitCount     int                `json:"scanned_commit_count"`
	ReturnedEntryCount     int                `json:"returned_entry_count"`
	RedactionCount         int                `json:"redaction_count"`
	Observed               bool               `json:"observed"`
	Truncated              bool               `json:"truncated"`
	FirstParentOnly        bool               `json:"first_parent_only"`
	RenameInferred         bool               `json:"rename_inferred"`
	MetadataOnly           bool               `json:"metadata_only"`
	ReadOnly               bool               `json:"read_only"`
	AuthorityGranted       bool               `json:"authority_granted"`
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

type fileHistoryState struct {
	exists bool
	hash   plumbing.Hash
	mode   filemode.FileMode
	kind   string
}

// InspectFileHistory follows one bounded first-parent chain and returns only
// metadata for commits where the exact canonical path changed.
func InspectFileHistory(ctx context.Context, root string, workspaceID string,
	path string,
) (FileHistory, error) {
	base := FileHistory{ProtocolVersion: FileHistoryProtocolVersion,
		WorkspaceID: workspaceID, Kind: "none", Path: path, Entries: []FileHistoryEntry{},
		FirstParentOnly: true, MetadataOnly: true, ReadOnly: true}
	if err := validateIdentity(workspaceID); err != nil {
		return FileHistory{}, err
	}
	canonical, _, safe := safePath(path)
	if !safe || canonical != path {
		return FileHistory{}, apperror.New(apperror.CodeInvalidArgument,
			"repository file history path must be canonical and safe")
	}
	if err := ctx.Err(); err != nil {
		return FileHistory{}, err
	}
	_, repo, available, err := openExactRepository(ctx, root)
	if err != nil {
		return FileHistory{}, err
	}
	if !available {
		return base, nil
	}
	base.Kind = "git"
	base.Available = true
	head, err := repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return base, nil
	}
	if err != nil {
		return FileHistory{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository file history HEAD could not be inspected")
	}
	base.Head = shortHash(head.Hash().String())
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return FileHistory{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository file history commit could not be inspected")
	}
	for base.ScannedCommitCount < MaxFileHistoryCommitScan {
		if err := ctx.Err(); err != nil {
			return FileHistory{}, err
		}
		base.ScannedCommitCount++
		current, err := fileHistoryStateAt(ctx, commit, canonical)
		if err != nil {
			return FileHistory{}, err
		}
		previous := fileHistoryState{}
		var parent *object.Commit
		if len(commit.ParentHashes) > 0 {
			parent, err = repo.CommitObject(commit.ParentHashes[0])
			if err != nil {
				return FileHistory{}, apperror.New(apperror.CodeFailedPrecondition,
					"repository file history parent could not be inspected")
			}
			previous, err = fileHistoryStateAt(ctx, parent, canonical)
			if err != nil {
				return FileHistory{}, err
			}
		}
		if fileHistoryStateChanged(previous, current) {
			base.Observed = true
			entry := projectFileHistoryEntry(commit, previous, current)
			base.RedactionCount += fileHistorySubjectRedactions(commit.Message)
			if len(base.Entries) == MaxFileHistoryEntries {
				base.Truncated = true
				break
			}
			base.Entries = append(base.Entries, entry)
			base.Truncated = base.Truncated || entry.SubjectBounded
		}
		if parent == nil {
			break
		}
		commit = parent
	}
	if base.ScannedCommitCount == MaxFileHistoryCommitScan && len(commit.ParentHashes) > 0 {
		base.Truncated = true
	}
	base.ReturnedEntryCount = len(base.Entries)
	return base, nil
}

func fileHistoryStateAt(ctx context.Context, commit *object.Commit,
	path string,
) (fileHistoryState, error) {
	if err := ctx.Err(); err != nil {
		return fileHistoryState{}, err
	}
	if commit == nil {
		return fileHistoryState{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository file history commit is unavailable")
	}
	tree, err := commit.Tree()
	if err != nil {
		return fileHistoryState{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository file history tree could not be inspected")
	}
	entry, err := tree.FindEntry(path)
	if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
		return fileHistoryState{}, nil
	}
	if err != nil {
		return fileHistoryState{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository file history path could not be inspected")
	}
	kind, supported := commitFileKind(entry.Mode)
	if !supported {
		return fileHistoryState{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository file history path has an unsupported file mode")
	}
	return fileHistoryState{exists: true, hash: entry.Hash, mode: entry.Mode, kind: kind}, nil
}

func fileHistoryStateChanged(previous fileHistoryState, current fileHistoryState) bool {
	return previous.exists != current.exists ||
		(previous.exists && current.exists &&
			(previous.hash != current.hash || previous.mode != current.mode))
}

func projectFileHistoryEntry(commit *object.Commit, previous fileHistoryState,
	current fileHistoryState,
) FileHistoryEntry {
	subject, _, redacted, bounded := safeCommitSubject(commit.Message)
	entry := FileHistoryEntry{ObjectID: commit.Hash.String(), Hash: shortHash(commit.Hash.String()),
		Subject: subject, CommittedAt: commit.Committer.When.UTC(),
		PreviousKind: previous.kind, CurrentKind: current.kind,
		ContentChanged: previous.exists != current.exists || previous.hash != current.hash,
		ModeChanged:    previous.exists != current.exists || previous.mode != current.mode,
		Redacted:       redacted, SubjectBounded: bounded}
	switch {
	case !previous.exists:
		entry.Change = "added"
	case !current.exists:
		entry.Change = "deleted"
	default:
		entry.Change = "modified"
	}
	return entry
}

func fileHistorySubjectRedactions(message string) int {
	_, count, _, _ := safeCommitSubject(message)
	return count
}
