package repository

import (
	"context"
	"errors"
	"time"

	"cyberagent-workbench/internal/apperror"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const CommitComparisonProtocolVersion = "repository_commit_comparison.v1"

type CommitComparison struct {
	ProtocolVersion        string             `json:"protocol_version"`
	WorkspaceID            string             `json:"workspace_id"`
	Kind                   string             `json:"kind"`
	Available              bool               `json:"available"`
	BaseObjectID           string             `json:"base_object_id"`
	BaseHash               string             `json:"base_hash"`
	BaseSubject            string             `json:"base_subject"`
	BaseCommittedAt        time.Time          `json:"base_committed_at"`
	BaseRedacted           bool               `json:"base_redacted"`
	BaseSubjectBounded     bool               `json:"base_subject_bounded"`
	HeadObjectID           string             `json:"head_object_id"`
	HeadHash               string             `json:"head_hash"`
	HeadSubject            string             `json:"head_subject"`
	HeadCommittedAt        time.Time          `json:"head_committed_at"`
	HeadRedacted           bool               `json:"head_redacted"`
	HeadSubjectBounded     bool               `json:"head_subject_bounded"`
	SameObject             bool               `json:"same_object"`
	Changes                []CommitFileChange `json:"changes"`
	ChangedFileCount       int                `json:"changed_file_count"`
	ReturnedChangeCount    int                `json:"returned_change_count"`
	OmittedChangeCount     int                `json:"omitted_change_count"`
	RedactionCount         int                `json:"redaction_count"`
	Truncated              bool               `json:"truncated"`
	MetadataOnly           bool               `json:"metadata_only"`
	ReadOnly               bool               `json:"read_only"`
	RenameInferred         bool               `json:"rename_inferred"`
	AncestorRequired       bool               `json:"ancestor_required"`
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

// InspectCommitComparison compares two exact local commit trees without
// materializing file content or requiring an ancestor relationship.
func InspectCommitComparison(ctx context.Context, root string, workspaceID string,
	baseObjectID string, headObjectID string,
) (CommitComparison, error) {
	result := CommitComparison{ProtocolVersion: CommitComparisonProtocolVersion,
		WorkspaceID: workspaceID, Kind: "none", BaseObjectID: baseObjectID,
		HeadObjectID: headObjectID, SameObject: baseObjectID == headObjectID,
		Changes: []CommitFileChange{}, MetadataOnly: true, ReadOnly: true}
	if err := validateIdentity(workspaceID); err != nil {
		return CommitComparison{}, err
	}
	if !validCommitObjectID(baseObjectID) || !validCommitObjectID(headObjectID) {
		return CommitComparison{}, apperror.New(apperror.CodeInvalidArgument,
			"repository comparison object identity is invalid")
	}
	if err := ctx.Err(); err != nil {
		return CommitComparison{}, err
	}
	_, repo, available, err := openExactRepository(ctx, root)
	if err != nil {
		return CommitComparison{}, err
	}
	if !available {
		return result, nil
	}
	loadCommit := func(objectID string) (*object.Commit, error) {
		commit, loadErr := repo.CommitObject(plumbing.NewHash(objectID))
		if loadErr != nil {
			if errors.Is(loadErr, plumbing.ErrObjectNotFound) {
				return nil, apperror.New(apperror.CodeNotFound,
					"repository comparison commit was not found")
			}
			return nil, apperror.New(apperror.CodeFailedPrecondition,
				"repository comparison commit could not be inspected")
		}
		if commit.Hash.String() != objectID {
			return nil, apperror.New(apperror.CodeConflict,
				"repository comparison commit identity changed")
		}
		return commit, nil
	}
	baseCommit, err := loadCommit(baseObjectID)
	if err != nil {
		return CommitComparison{}, err
	}
	headCommit, err := loadCommit(headObjectID)
	if err != nil {
		return CommitComparison{}, err
	}
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return CommitComparison{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository comparison base tree could not be inspected")
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return CommitComparison{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository comparison head tree could not be inspected")
	}
	baseEntries, err := collectCommitTreeEntries(ctx, baseTree)
	if err != nil {
		return CommitComparison{}, err
	}
	headEntries, err := collectCommitTreeEntries(ctx, headTree)
	if err != nil {
		return CommitComparison{}, err
	}

	result.Kind = "git"
	result.Available = true
	result.BaseHash = shortHash(baseObjectID)
	result.BaseCommittedAt = baseCommit.Committer.When.UTC()
	baseRedactions := 0
	result.BaseSubject, baseRedactions, result.BaseRedacted,
		result.BaseSubjectBounded = safeCommitSubject(baseCommit.Message)
	result.HeadHash = shortHash(headObjectID)
	result.HeadCommittedAt = headCommit.Committer.When.UTC()
	headRedactions := 0
	result.HeadSubject, headRedactions, result.HeadRedacted,
		result.HeadSubjectBounded = safeCommitSubject(headCommit.Message)

	projection := CommitDetail{Changes: []CommitFileChange{},
		RedactionCount: baseRedactions + headRedactions,
		Truncated:      result.BaseSubjectBounded || result.HeadSubjectBounded}
	projectCommitTreeChanges(baseEntries, headEntries, &projection)
	result.Changes = projection.Changes
	result.ChangedFileCount = projection.ChangedFileCount
	result.ReturnedChangeCount = projection.ReturnedChangeCount
	result.OmittedChangeCount = projection.OmittedChangeCount
	result.RedactionCount = projection.RedactionCount
	result.Truncated = projection.Truncated
	return result, nil
}
