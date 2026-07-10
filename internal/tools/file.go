package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/redact"
)

const (
	DefaultMaxReadBytes   = 128 * 1024
	DefaultMaxListEntries = 200
)

type WorkspaceFS struct {
	Root           string
	MaxReadBytes   int
	MaxListEntries int
}

func NewWorkspaceFS(root string) WorkspaceFS {
	return WorkspaceFS{
		Root:           root,
		MaxReadBytes:   DefaultMaxReadBytes,
		MaxListEntries: DefaultMaxListEntries,
	}
}

type ReadFileTool struct {
	FS WorkspaceFS
}

func NewReadFileTool(root string) ReadFileTool {
	return ReadFileTool{FS: NewWorkspaceFS(root)}
}

func (ReadFileTool) Name() string {
	return "read_file"
}

func (ReadFileTool) Schema() Schema {
	return Schema{
		Description: "Read a scoped local file.",
		Parameters:  map[string]string{"path": "Path to read"},
	}
}

func (t ReadFileTool) Run(ctx context.Context, call Call) (Result, error) {
	_ = ctx
	fs := t.FS.withFallback(call.WorkingDir)
	path, err := fs.resolveExistingFile(call.Args["path"])
	if err != nil {
		return Result{Stderr: err.Error(), ExitCode: 1}, err
	}
	limit := fs.MaxReadBytes
	if value := strings.TrimSpace(call.Args["max_bytes"]); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return Result{Stderr: "max_bytes must be a positive integer", ExitCode: 1}, errors.New("max_bytes must be a positive integer")
		}
		limit = parsed
	}
	text, err := readTextLimited(path, limit)
	if err != nil {
		return Result{Stderr: err.Error(), ExitCode: 1}, err
	}
	return Result{Stdout: text, ExitCode: 0}, nil
}

type ListWorkspaceTool struct {
	FS WorkspaceFS
}

func NewListWorkspaceTool(root string) ListWorkspaceTool {
	return ListWorkspaceTool{FS: NewWorkspaceFS(root)}
}

func (ListWorkspaceTool) Name() string {
	return "list_workspace"
}

func (ListWorkspaceTool) Schema() Schema {
	return Schema{
		Description: "List files under a workspace-scoped path.",
		Parameters: map[string]string{
			"path":      "Relative workspace path to list",
			"max_depth": "Maximum recursive depth, default 2",
		},
	}
}

func (t ListWorkspaceTool) Run(ctx context.Context, call Call) (Result, error) {
	_ = ctx
	fs := t.FS.withFallback(call.WorkingDir)
	path := strings.TrimSpace(call.Args["path"])
	if path == "" {
		path = "."
	}
	depth := 2
	if value := strings.TrimSpace(call.Args["max_depth"]); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return Result{Stderr: "max_depth must be a non-negative integer", ExitCode: 1}, errors.New("max_depth must be a non-negative integer")
		}
		depth = parsed
	}
	out, err := fs.list(path, depth)
	if err != nil {
		return Result{Stderr: err.Error(), ExitCode: 1}, err
	}
	return Result{Stdout: out, ExitCode: 0}, nil
}

func (fs WorkspaceFS) withFallback(root string) WorkspaceFS {
	if strings.TrimSpace(fs.Root) == "" {
		fs.Root = root
	}
	if fs.MaxReadBytes <= 0 {
		fs.MaxReadBytes = DefaultMaxReadBytes
	}
	if fs.MaxListEntries <= 0 {
		fs.MaxListEntries = DefaultMaxListEntries
	}
	return fs
}

func (fs WorkspaceFS) resolveExistingFile(requested string) (string, error) {
	path, err := fs.resolveExisting(requested)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", requested)
	}
	return path, nil
}

// ResolveForWrite returns a workspace-scoped file path. Existing symlinks are
// resolved, while new files require an existing parent directory.
func (fs WorkspaceFS) ResolveForWrite(requested string) (string, error) {
	fs = fs.withFallback("")
	if strings.TrimSpace(fs.Root) == "" {
		return "", errors.New("workspace root is required")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" || requested == "." {
		return "", errors.New("file path is required")
	}
	if filepath.IsAbs(requested) {
		return "", errors.New("path must be relative to the workspace")
	}

	root, err := filepath.Abs(fs.Root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(root, requested))
	if err != nil {
		return "", err
	}
	if !withinRoot(root, candidate) {
		return "", fmt.Errorf("path escapes workspace: %s", requested)
	}

	if _, err := os.Lstat(candidate); err == nil {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", err
		}
		if !withinRoot(root, resolved) {
			return "", fmt.Errorf("path escapes workspace: %s", requested)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory", requested)
		}
		return resolved, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	parent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return "", fmt.Errorf("parent directory must already exist: %w", err)
	}
	if !withinRoot(root, parent) {
		return "", fmt.Errorf("path escapes workspace: %s", requested)
	}
	return filepath.Join(parent, filepath.Base(candidate)), nil
}

func (fs WorkspaceFS) resolveExistingDir(requested string) (string, error) {
	path, err := fs.resolveExisting(requested)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", requested)
	}
	return path, nil
}

func (fs WorkspaceFS) resolveExisting(requested string) (string, error) {
	if strings.TrimSpace(fs.Root) == "" {
		return "", errors.New("workspace root is required")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "."
	}
	if filepath.IsAbs(requested) {
		return "", errors.New("path must be relative to the workspace")
	}
	root, err := filepath.Abs(fs.Root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, requested)
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !withinRoot(root, candidate) {
		return "", fmt.Errorf("path escapes workspace: %s", requested)
	}
	return candidate, nil
}

func (fs WorkspaceFS) list(requested string, maxDepth int) (string, error) {
	base, err := fs.resolveExistingDir(requested)
	if err != nil {
		return "", err
	}
	var lines []string
	limit := fs.MaxListEntries
	err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		depth := pathDepth(rel)
		if depth > maxDepth {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			name += string(os.PathSeparator)
		}
		lines = append(lines, strings.Repeat("  ", max(0, depth-1))+name)
		if len(lines) >= limit {
			lines = append(lines, fmt.Sprintf("... truncated at %d entries", limit))
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "(empty)", nil
	}
	return strings.Join(lines, "\n"), nil
}

func readTextLimited(path string, maxBytes int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return "", err
	}
	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	if !utf8.Valid(data) {
		return "", errors.New("file is not valid UTF-8 text")
	}
	text := string(data)
	if truncated {
		text += fmt.Sprintf("\n[truncated at %d bytes]\n", maxBytes)
	}
	return redact.String(text), nil
}

func withinRoot(root string, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func pathDepth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return 1 + strings.Count(rel, string(os.PathSeparator))
}
