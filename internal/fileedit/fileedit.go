package fileedit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/tools"
)

const (
	MaxContentBytes = 256 * 1024
	// MaxDiffBytes bounds a persisted unified diff generated from two bounded
	// file versions, including per-line prefixes and headers.
	MaxDiffBytes = 4*MaxContentBytes + 16*1024
)

const (
	StatusProposed = "proposed"
	StatusApproved = "approved"
	StatusApplied  = "applied"
	StatusDenied   = "denied"
	StatusFailed   = "failed"
)

const missingHash = "missing"

type Edit struct {
	ID              string
	SessionID       string
	WorkspaceID     string
	Path            string
	Status          string
	OriginalText    string
	ProposedText    string
	Diff            string
	OriginalHash    string
	ProposedHash    string
	Reason          string
	SecretsRedacted bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Preview is the read-only FileEdit projection used by operator surfaces.
// It deliberately excludes the original and proposed file bodies.
type Preview struct {
	ID              string
	SessionID       string
	WorkspaceID     string
	Path            string
	Status          string
	Diff            string
	OriginalHash    string
	ProposedHash    string
	Reason          string
	SecretsRedacted bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func ValidStatus(status string) bool {
	switch status {
	case StatusProposed, StatusApproved, StatusApplied, StatusDenied, StatusFailed:
		return true
	default:
		return false
	}
}

type Proposal struct {
	SessionID     string
	WorkspaceID   string
	WorkspaceRoot string
	Path          string
	ProposedText  string
}

type ListFilter struct {
	SessionID   string
	WorkspaceID string
	Status      string
}

type Store interface {
	SaveFileEdit(ctx context.Context, edit Edit) (Edit, error)
	GetFileEdit(ctx context.Context, id string) (Edit, error)
	ListFileEdits(ctx context.Context, filter ListFilter) ([]Edit, error)
}

type Manager struct {
	store Store
}

func NewManager(store Store) *Manager {
	return &Manager{store: store}
}

func (m *Manager) Propose(ctx context.Context, proposal Proposal) (Edit, error) {
	if m == nil || m.store == nil {
		return Edit{}, errors.New("file edit store is required")
	}
	proposal.WorkspaceID = strings.TrimSpace(proposal.WorkspaceID)
	proposal.WorkspaceRoot = strings.TrimSpace(proposal.WorkspaceRoot)
	if proposal.WorkspaceID == "" {
		return Edit{}, errors.New("workspace id is required")
	}
	if proposal.WorkspaceRoot == "" {
		return Edit{}, errors.New("workspace root is required")
	}
	if len([]byte(proposal.ProposedText)) > MaxContentBytes {
		return Edit{}, fmt.Errorf("proposed content exceeds %d bytes", MaxContentBytes)
	}
	if !utf8.ValidString(proposal.ProposedText) {
		return Edit{}, errors.New("proposed content is not valid UTF-8 text")
	}

	relPath, err := normalizePath(proposal.Path)
	if err != nil {
		return Edit{}, err
	}
	fs := tools.NewWorkspaceFS(proposal.WorkspaceRoot)
	target, err := fs.ResolveForWrite(relPath)
	if err != nil {
		return Edit{}, err
	}
	original, exists, err := readCurrentText(target)
	if err != nil {
		return Edit{}, err
	}
	proposed := redact.String(proposal.ProposedText)
	secretsRedacted := proposed != proposal.ProposedText
	if exists && original == proposed {
		return Edit{}, errors.New("proposed content does not change the file")
	}

	originalPreview := redact.String(original)
	diff := UnifiedDiff(relPath, originalPreview, proposed)
	if exists && original != proposed && originalPreview == proposed {
		diff = redactedChangeDiff(relPath)
	}
	now := time.Now().UTC()
	edit := Edit{
		ID:              newID("edit"),
		SessionID:       strings.TrimSpace(proposal.SessionID),
		WorkspaceID:     proposal.WorkspaceID,
		Path:            relPath,
		Status:          StatusProposed,
		OriginalText:    originalPreview,
		ProposedText:    proposed,
		Diff:            diff,
		OriginalHash:    contentHash(original, exists),
		ProposedHash:    contentHash(proposed, true),
		SecretsRedacted: secretsRedacted || originalPreview != original,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return m.store.SaveFileEdit(ctx, edit)
}

func (m *Manager) Approve(ctx context.Context, id string, workspaceRoot string) (Edit, error) {
	edit, err := m.store.GetFileEdit(ctx, strings.TrimSpace(id))
	if err != nil {
		return Edit{}, err
	}
	if edit.Status == StatusApplied {
		return edit, nil
	}
	if edit.Status != StatusProposed && edit.Status != StatusApproved {
		return Edit{}, fmt.Errorf("file edit %s is %s, not %s", edit.ID, edit.Status, StatusProposed)
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return Edit{}, errors.New("workspace root is required")
	}
	if contentHash(edit.ProposedText, true) != edit.ProposedHash {
		return m.fail(ctx, edit, errors.New("stored proposed content failed integrity validation"))
	}

	target, err := tools.NewWorkspaceFS(workspaceRoot).ResolveForWrite(edit.Path)
	if err != nil {
		return m.fail(ctx, edit, err)
	}
	current, exists, err := readCurrentText(target)
	if err != nil {
		return m.fail(ctx, edit, err)
	}
	currentHash := contentHash(current, exists)
	if currentHash == edit.ProposedHash {
		edit.Status = StatusApplied
		edit.Reason = ""
		edit.UpdatedAt = time.Now().UTC()
		return m.store.SaveFileEdit(ctx, edit)
	}
	if currentHash != edit.OriginalHash {
		return m.fail(ctx, edit, errors.New("workspace file changed after the proposal; refusing to overwrite"))
	}

	edit.Status = StatusApproved
	edit.Reason = ""
	edit.UpdatedAt = time.Now().UTC()
	edit, err = m.store.SaveFileEdit(ctx, edit)
	if err != nil {
		return Edit{}, err
	}
	writeTarget, err := tools.NewWorkspaceFS(workspaceRoot).ResolveForWrite(edit.Path)
	if err != nil {
		return m.fail(ctx, edit, err)
	}
	if writeTarget != target {
		return m.fail(ctx, edit, errors.New("workspace path changed during approval; refusing to write"))
	}
	latest, latestExists, err := readCurrentText(writeTarget)
	if err != nil {
		return m.fail(ctx, edit, err)
	}
	if contentHash(latest, latestExists) != edit.OriginalHash {
		return m.fail(ctx, edit, errors.New("workspace file changed during approval; refusing to overwrite"))
	}
	target = writeTarget
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(target); statErr == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return m.fail(ctx, edit, statErr)
	}
	if err := os.WriteFile(target, []byte(edit.ProposedText), mode); err != nil {
		return m.fail(ctx, edit, err)
	}

	written, writtenExists, err := readCurrentText(target)
	if err != nil {
		return m.fail(ctx, edit, err)
	}
	if contentHash(written, writtenExists) != edit.ProposedHash {
		return m.fail(ctx, edit, errors.New("written file failed integrity validation"))
	}
	edit.Status = StatusApplied
	edit.Reason = ""
	edit.UpdatedAt = time.Now().UTC()
	return m.store.SaveFileEdit(ctx, edit)
}

func (m *Manager) Deny(ctx context.Context, id string, reason string) (Edit, error) {
	edit, err := m.store.GetFileEdit(ctx, strings.TrimSpace(id))
	if err != nil {
		return Edit{}, err
	}
	if edit.Status == StatusDenied {
		return edit, nil
	}
	if edit.Status != StatusProposed {
		return Edit{}, fmt.Errorf("file edit %s is %s, not %s", edit.ID, edit.Status, StatusProposed)
	}
	edit.Status = StatusDenied
	edit.Reason = redact.String(strings.TrimSpace(reason))
	edit.UpdatedAt = time.Now().UTC()
	return m.store.SaveFileEdit(ctx, edit)
}

func (m *Manager) Get(ctx context.Context, id string) (Edit, error) {
	return m.store.GetFileEdit(ctx, strings.TrimSpace(id))
}

func (m *Manager) List(ctx context.Context, filter ListFilter) ([]Edit, error) {
	return m.store.ListFileEdits(ctx, filter)
}

func (m *Manager) fail(ctx context.Context, edit Edit, cause error) (Edit, error) {
	edit.Status = StatusFailed
	edit.Reason = redact.String(cause.Error())
	edit.UpdatedAt = time.Now().UTC()
	saved, saveErr := m.store.SaveFileEdit(ctx, edit)
	if saveErr != nil {
		return Edit{}, errors.Join(cause, saveErr)
	}
	return saved, cause
}

func normalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "", errors.New("file path is required")
	}
	if filepath.IsAbs(path) {
		return "", errors.New("path must be relative to the workspace")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return filepath.ToSlash(clean), nil
}

func readCurrentText(path string) (string, bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("%s is a directory", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxContentBytes+1))
	if err != nil {
		return "", false, err
	}
	if len(data) > MaxContentBytes {
		return "", false, fmt.Errorf("file exceeds %d bytes", MaxContentBytes)
	}
	if !utf8.Valid(data) {
		return "", false, errors.New("file is not valid UTF-8 text")
	}
	return string(data), true, nil
}

func contentHash(content string, exists bool) string {
	if !exists {
		return missingHash
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func HashText(content string) string {
	return contentHash(content, true)
}

func newID(prefix string) string {
	return idgen.New(prefix)
}
