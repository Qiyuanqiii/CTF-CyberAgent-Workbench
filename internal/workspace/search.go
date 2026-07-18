package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/redact"
)

const (
	SearchProtocolVersion = "workspace_search.v1"

	MaxSearchQueryRunes  = 128
	MaxSearchDirectories = 128
	MaxSearchEntries     = 1000
	MaxSearchFiles       = 64
	MaxSearchResults     = 50
	// Explorer reads up to UTFMax look-ahead bytes to avoid splitting a rune.
	MaxSearchReadBytes    = MaxSearchFiles * (MaxExplorerReadBytes + utf8.UTFMax)
	MaxSearchSnippetBytes = 512

	explorerStagingPrefix = ".cyberagent-edit-"
)

type SearchResult struct {
	Path             string
	MatchKind        string
	Line             int
	Snippet          string
	ContentTruncated bool
	Provenance       ExplorerProvenance
}

type SearchSnapshot struct {
	ProtocolVersion string
	WorkspaceID     string
	Results         []SearchResult
	ScannedEntries  int
	ScannedFiles    int
	ScannedBytes    int64
	Truncated       bool
	RootPathExposed bool
}

// Search performs one deterministic, bounded scan over redacted Explorer
// projections. It never builds an index and never searches raw file bytes.
func Search(root string, workspaceID string, query string) (SearchSnapshot, error) {
	query, err := normalizeSearchQuery(query)
	if err != nil {
		return SearchSnapshot{}, err
	}
	if workspaceID == "" || workspaceID != strings.TrimSpace(workspaceID) ||
		strings.ContainsRune(workspaceID, 0) {
		return SearchSnapshot{}, apperror.New(apperror.CodeInvalidArgument,
			"workspace identity is invalid")
	}
	rootTarget, rootInfo, err := resolveExplorerTarget(root, ".")
	if err != nil {
		return SearchSnapshot{}, err
	}
	if !rootInfo.IsDir() {
		return SearchSnapshot{}, apperror.New(apperror.CodeFailedPrecondition,
			"workspace root is not a directory")
	}

	snapshot := SearchSnapshot{ProtocolVersion: SearchProtocolVersion,
		WorkspaceID: workspaceID, Results: []SearchResult{}, RootPathExposed: false}
	lowerQuery := strings.ToLower(query)
	queue := []string{"."}
	scannedDirectories := 0
	for len(queue) > 0 {
		if scannedDirectories >= MaxSearchDirectories {
			snapshot.Truncated = true
			break
		}
		relative := queue[0]
		queue = queue[1:]
		scannedDirectories++
		target := rootTarget
		if relative != "." {
			resolved, info, resolveErr := resolveExplorerTarget(rootTarget, relative)
			if resolveErr != nil || !info.IsDir() {
				snapshot.Truncated = true
				continue
			}
			target = resolved
		}
		entries, more, readErr := readSearchDirectory(target,
			MaxSearchEntries-snapshot.ScannedEntries)
		if readErr != nil {
			if relative == "." {
				return SearchSnapshot{}, apperror.New(apperror.CodeFailedPrecondition,
					"workspace root could not be searched")
			}
			snapshot.Truncated = true
			continue
		}
		if more {
			snapshot.Truncated = true
		}
		for _, entry := range entries {
			if snapshot.ScannedEntries >= MaxSearchEntries {
				snapshot.Truncated = true
				return snapshot, nil
			}
			snapshot.ScannedEntries++
			if strings.HasPrefix(entry.Name(), explorerStagingPrefix) ||
				!safeExplorerName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
				snapshot.Truncated = true
				continue
			}
			nameProjection := redact.Text(entry.Name())
			if nameProjection.Text != entry.Name() {
				snapshot.Truncated = true
				continue
			}
			entryPath := entry.Name()
			if relative != "." {
				entryPath = filepath.ToSlash(filepath.Join(filepath.FromSlash(relative), entry.Name()))
			}
			canonical, pathErr := normalizeExplorerPath(entryPath)
			if pathErr != nil || canonical != entryPath {
				snapshot.Truncated = true
				continue
			}
			_, info, resolveErr := resolveExplorerTarget(rootTarget, entryPath)
			if resolveErr != nil {
				snapshot.Truncated = true
				continue
			}
			if info.IsDir() {
				if scannedDirectories+len(queue) >= MaxSearchDirectories {
					snapshot.Truncated = true
					continue
				}
				queue = append(queue, entryPath)
				continue
			}
			if !info.Mode().IsRegular() {
				snapshot.Truncated = true
				continue
			}
			if snapshot.ScannedFiles >= MaxSearchFiles {
				snapshot.Truncated = true
				continue
			}
			file, fileErr := Explore(rootTarget, workspaceID, entryPath)
			if fileErr != nil || file.Kind != "file" {
				snapshot.Truncated = true
				continue
			}
			snapshot.ScannedFiles++
			readBytes := file.TotalBytes
			if readBytes > MaxExplorerReadBytes+utf8.UTFMax {
				readBytes = MaxExplorerReadBytes + utf8.UTFMax
			}
			snapshot.ScannedBytes += readBytes
			filenameMatch := strings.Contains(strings.ToLower(entry.Name()), lowerQuery)
			line, snippet, contentMatch := searchProjectedContent(file.Content, lowerQuery)
			if !filenameMatch && !contentMatch {
				continue
			}
			kind := "content"
			if filenameMatch && contentMatch {
				kind = "filename_and_content"
			} else if filenameMatch {
				kind = "filename"
			}
			snapshot.Results = append(snapshot.Results, SearchResult{Path: entryPath,
				MatchKind: kind, Line: line, Snippet: snippet,
				ContentTruncated: file.Truncated, Provenance: file.Provenance})
			if len(snapshot.Results) >= MaxSearchResults {
				snapshot.Truncated = true
				return snapshot, nil
			}
		}
	}
	return snapshot, nil
}

func normalizeSearchQuery(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > MaxSearchQueryRunes || strings.ContainsRune(value, 0) {
		return "", apperror.New(apperror.CodeInvalidArgument,
			"workspace search query must be normalized, non-empty, and bounded")
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return "", apperror.New(apperror.CodeInvalidArgument,
				"workspace search query cannot contain control characters")
		}
	}
	return value, nil
}

func readSearchDirectory(path string, limit int) ([]os.DirEntry, bool, error) {
	if limit <= 0 {
		return nil, true, nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	more := len(entries) > limit
	if more {
		entries = entries[:limit]
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})
	return entries, more, nil
}

func searchProjectedContent(content string, lowerQuery string) (int, string, bool) {
	// Search one original line at a time. Unicode case mapping can change byte
	// length, so offsets from a lower-cased copy must never slice the source.
	for index, original := range strings.Split(content, "\n") {
		if !strings.Contains(strings.ToLower(original), lowerQuery) {
			continue
		}
		snippet := strings.TrimSpace(original)
		if len([]byte(snippet)) > MaxSearchSnippetBytes {
			projected, err := validExplorerUTF8Prefix([]byte(snippet), MaxSearchSnippetBytes)
			if err == nil {
				snippet = string(projected)
			}
		}
		return index + 1, snippet, true
	}
	return 0, "", false
}
