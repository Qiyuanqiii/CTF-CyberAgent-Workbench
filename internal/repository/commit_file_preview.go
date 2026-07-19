package repository

import (
	"context"
	"errors"
	"io"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/workspace"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	CommitFilePreviewProtocolVersion   = "repository_commit_file_preview.v1"
	CommitFilePreviewSourceKind        = "repository_commit_file"
	MaxCommitFilePreviewBytes          = workspace.MaxExplorerReadBytes
	MaxCommitFilePreviewProjectedBytes = workspace.MaxExplorerProjectedBytes
)

type CommitFilePreviewProvenance struct {
	Version               string `json:"version"`
	SourceKind            string `json:"source_kind"`
	SourceRef             string `json:"source_ref"`
	ContentSHA256         string `json:"content_sha256"`
	InstructionAuthorized bool   `json:"instruction_authorized"`
}

// CommitFilePreview is a bounded, redacted projection of one regular file in
// one exact commit tree. It carries no checkout, mutation, or execution power.
type CommitFilePreview struct {
	ProtocolVersion         string                      `json:"protocol_version"`
	WorkspaceID             string                      `json:"workspace_id"`
	ObjectID                string                      `json:"object_id"`
	Hash                    string                      `json:"hash"`
	Path                    string                      `json:"path"`
	Kind                    string                      `json:"kind"`
	Content                 string                      `json:"content"`
	TotalBytes              int64                       `json:"total_bytes"`
	ReturnedBytes           int                         `json:"returned_bytes"`
	RedactionCount          int                         `json:"redaction_count"`
	Redacted                bool                        `json:"redacted"`
	Provenance              CommitFilePreviewProvenance `json:"provenance"`
	ReadOnly                bool                        `json:"read_only"`
	MutationSupported       bool                        `json:"mutation_supported"`
	AuthorityGranted        bool                        `json:"authority_granted"`
	RootPathExposed         bool                        `json:"root_path_exposed"`
	RawBlobIncluded         bool                        `json:"raw_blob_included"`
	RedactedContentIncluded bool                        `json:"redacted_content_included"`
	RemoteConfigIncluded    bool                        `json:"remote_config_included"`
	CheckoutPerformed       bool                        `json:"checkout_performed"`
	ReferenceUpdated        bool                        `json:"reference_updated"`
	ProcessStarted          bool                        `json:"process_started"`
	NetworkUsed             bool                        `json:"network_used"`
	HooksExecuted           bool                        `json:"hooks_executed"`
}

func InspectCommitFilePreview(ctx context.Context, root string, workspaceID string,
	objectID string, path string,
) (CommitFilePreview, error) {
	if err := validateIdentity(workspaceID); err != nil {
		return CommitFilePreview{}, err
	}
	if !validCommitObjectID(objectID) {
		return CommitFilePreview{}, apperror.New(apperror.CodeInvalidArgument,
			"repository commit object identity is invalid")
	}
	canonical, _, safe := safePath(path)
	if !safe || canonical != path {
		return CommitFilePreview{}, apperror.New(apperror.CodeInvalidArgument,
			"repository commit file path must be canonical and safe")
	}
	if err := ctx.Err(); err != nil {
		return CommitFilePreview{}, err
	}
	_, repo, available, err := openExactRepository(ctx, root)
	if err != nil {
		return CommitFilePreview{}, err
	}
	if !available {
		return CommitFilePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository is unavailable at the registered Workspace root")
	}
	commit, err := repo.CommitObject(plumbing.NewHash(objectID))
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return CommitFilePreview{}, apperror.New(apperror.CodeNotFound,
				"repository commit was not found")
		}
		return CommitFilePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit could not be inspected")
	}
	if commit.Hash.String() != objectID {
		return CommitFilePreview{}, apperror.New(apperror.CodeConflict,
			"repository commit identity changed")
	}
	tree, err := commit.Tree()
	if err != nil {
		return CommitFilePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit tree could not be inspected")
	}
	file, err := tree.File(canonical)
	if errors.Is(err, object.ErrFileNotFound) {
		return CommitFilePreview{}, apperror.New(apperror.CodeNotFound,
			"repository commit file was not found")
	}
	if err != nil {
		return CommitFilePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit file could not be inspected")
	}
	kind, supported := commitFileKind(file.Mode)
	if !supported || (kind != "regular" && kind != "executable") || !file.Mode.IsRegular() {
		return CommitFilePreview{}, apperror.New(apperror.CodePolicyDenied,
			"repository commit preview accepts regular files only")
	}
	if file.Size < 0 || file.Size > MaxCommitFilePreviewBytes {
		return CommitFilePreview{}, apperror.New(apperror.CodeResourceExhausted,
			"repository commit file exceeds the preview limit")
	}
	reader, err := file.Reader()
	if err != nil {
		return CommitFilePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit file could not be opened")
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, MaxCommitFilePreviewBytes+1))
	if err != nil {
		return CommitFilePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit file could not be read")
	}
	if len(data) > MaxCommitFilePreviewBytes {
		return CommitFilePreview{}, apperror.New(apperror.CodeResourceExhausted,
			"repository commit file exceeds the preview limit")
	}
	if !safeDiffText(data) {
		return CommitFilePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"repository commit preview accepts UTF-8 text only")
	}
	projection := redact.Text(string(data))
	if len([]byte(projection.Text)) > MaxCommitFilePreviewProjectedBytes {
		return CommitFilePreview{}, apperror.New(apperror.CodeResourceExhausted,
			"repository commit redacted projection exceeds the preview limit")
	}
	redactionCount := diffFindingCount(projection.Findings)
	digest := session.ContentSHA256(projection.Text)
	return CommitFilePreview{
		ProtocolVersion: CommitFilePreviewProtocolVersion, WorkspaceID: workspaceID,
		ObjectID: objectID, Hash: shortHash(objectID), Path: canonical, Kind: kind,
		Content: projection.Text, TotalBytes: file.Size, ReturnedBytes: len([]byte(projection.Text)),
		RedactionCount: redactionCount, Redacted: projection.Text != string(data),
		Provenance: CommitFilePreviewProvenance{
			Version: session.ContextProvenanceVersion, SourceKind: CommitFilePreviewSourceKind,
			SourceRef: canonical, ContentSHA256: digest,
		},
		ReadOnly: true, RedactedContentIncluded: true,
	}, nil
}
