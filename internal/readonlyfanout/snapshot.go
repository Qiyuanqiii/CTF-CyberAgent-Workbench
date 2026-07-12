package readonlyfanout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/domain"
)

type BuildPlanRequest struct {
	PlanID        string
	RunID         string
	WorkspaceID   string
	WorkspaceRoot string
	ScopePath     string
	Goal          string
	Tier          domain.ReadOnlyFanoutTier
	RequestedBy   string
	CreatedAt     time.Time
}

type snapshotCandidate struct {
	relativePath string
	sizeBytes    int64
	digest       string
}

var skippedDirectories = map[string]struct{}{
	".git": {}, ".hg": {}, ".svn": {}, ".cache": {}, ".next": {},
	"node_modules": {}, "vendor": {}, "target": {}, "dist": {}, "build": {},
}

var supportedExtensions = map[string]struct{}{
	".c": {}, ".cc": {}, ".cfg": {}, ".conf": {}, ".cpp": {}, ".cs": {},
	".css": {}, ".go": {}, ".h": {}, ".hpp": {}, ".html": {}, ".ini": {},
	".java": {}, ".js": {}, ".jsx": {}, ".json": {}, ".kt": {}, ".lock": {},
	".md": {}, ".mod": {}, ".php": {}, ".proto": {}, ".py": {}, ".rb": {},
	".rs": {}, ".scss": {}, ".sh": {}, ".sql": {}, ".sum": {}, ".swift": {},
	".toml": {}, ".ts": {}, ".tsx": {}, ".txt": {}, ".xml": {}, ".yaml": {},
	".yml": {}, ".zig": {},
}

var supportedBasenames = map[string]struct{}{
	"dockerfile": {}, "license": {}, "makefile": {}, "readme": {},
}

var sensitiveBasenames = map[string]struct{}{
	".netrc": {}, ".npmrc": {}, ".pypirc": {}, "credentials": {},
	"credentials.json": {}, "id_dsa": {}, "id_ed25519": {}, "id_rsa": {},
	"secrets.json": {},
}

var sensitiveExtensions = map[string]struct{}{
	".jks": {}, ".kdbx": {}, ".key": {}, ".p12": {}, ".pem": {}, ".pfx": {},
}

func BuildPlan(ctx context.Context, request BuildPlanRequest) (domain.ReadOnlyFanoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request.WorkspaceRoot = strings.TrimSpace(request.WorkspaceRoot)
	request.ScopePath = strings.TrimSpace(request.ScopePath)
	if request.ScopePath == "" {
		request.ScopePath = "."
	}
	if request.CreatedAt.IsZero() {
		request.CreatedAt = time.Now().UTC()
	} else {
		request.CreatedAt = request.CreatedAt.UTC()
	}
	root, scope, err := resolveSnapshotScope(request.WorkspaceRoot, request.ScopePath)
	if err != nil {
		return domain.ReadOnlyFanoutPlan{}, err
	}
	candidates, excluded, err := collectSnapshotCandidates(ctx, root, scope)
	if err != nil {
		return domain.ReadOnlyFanoutPlan{}, err
	}
	if len(candidates) == 0 {
		return domain.ReadOnlyFanoutPlan{}, errors.New(
			"read-only fan-out scope contains no eligible UTF-8 source files")
	}
	parallelism, err := domain.ResolveReadOnlyFanoutParallelism(request.Tier,
		len(candidates))
	if err != nil {
		return domain.ReadOnlyFanoutPlan{}, err
	}
	files, shardBytes, shardFiles := partitionCandidates(request.PlanID, candidates,
		parallelism)
	snapshotDigest, err := domain.ReadOnlyFanoutSnapshotDigest(files)
	if err != nil {
		return domain.ReadOnlyFanoutPlan{}, err
	}
	capabilityFingerprint, err := domain.ReadOnlyFanoutCapabilityFingerprint(
		domain.DefaultReadOnlyFanoutCapabilities())
	if err != nil {
		return domain.ReadOnlyFanoutPlan{}, err
	}
	shards := make([]domain.ReadOnlyFanoutShard, parallelism)
	for index := range shards {
		ordinal := index + 1
		digest, err := domain.ReadOnlyFanoutShardDigest(ordinal, files)
		if err != nil {
			return domain.ReadOnlyFanoutPlan{}, err
		}
		shards[index] = domain.ReadOnlyFanoutShard{
			PlanID: request.PlanID, Ordinal: ordinal,
			Status:    domain.ReadOnlyFanoutShardPending,
			FileCount: shardFiles[index], TotalBytes: shardBytes[index],
			InputDigest: digest, Version: 1,
			CreatedAt: request.CreatedAt, UpdatedAt: request.CreatedAt,
		}
	}
	var totalBytes int64
	for _, file := range files {
		totalBytes += file.SizeBytes
	}
	plan := domain.ReadOnlyFanoutPlan{
		ID: request.PlanID, RunID: request.RunID, WorkspaceID: request.WorkspaceID,
		ScopePath: request.ScopePath, Goal: request.Goal,
		ProtocolVersion: domain.ReadOnlyFanoutProtocolVersion,
		RequestedTier:   request.Tier, EffectiveParallelism: parallelism,
		Status:                domain.ReadOnlyFanoutPlanned,
		CapabilityFingerprint: capabilityFingerprint, SnapshotDigest: snapshotDigest,
		FileCount: len(files), TotalBytes: totalBytes, ExcludedCount: excluded,
		ShardCount: parallelism, RequestedBy: request.RequestedBy,
		Version: 1, CreatedAt: request.CreatedAt, UpdatedAt: request.CreatedAt,
		Files: files, Shards: shards,
	}
	if err := plan.Validate(); err != nil {
		return domain.ReadOnlyFanoutPlan{}, err
	}
	return plan, nil
}

func resolveSnapshotScope(workspaceRoot, scopePath string) (string, string, error) {
	if workspaceRoot == "" {
		return "", "", errors.New("read-only fan-out workspace root is required")
	}
	if filepath.IsAbs(scopePath) || strings.Contains(scopePath, "\\") {
		return "", "", errors.New("read-only fan-out scope must be a slash-separated workspace-relative path")
	}
	cleanScope := path.Clean(scopePath)
	if cleanScope != scopePath || cleanScope == ".." || strings.HasPrefix(cleanScope, "../") {
		return "", "", errors.New("read-only fan-out scope escapes the workspace")
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return "", "", err
	}
	if !rootInfo.IsDir() {
		return "", "", errors.New("read-only fan-out workspace root is not a directory")
	}
	scopeCandidate, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(cleanScope)))
	if err != nil {
		return "", "", err
	}
	scope, err := filepath.EvalSymlinks(scopeCandidate)
	if err != nil {
		return "", "", err
	}
	if !sameCanonicalPath(scopeCandidate, scope) {
		return "", "", errors.New("read-only fan-out scope cannot traverse a symlink")
	}
	relative, err := filepath.Rel(root, scope)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", "", errors.New("read-only fan-out scope escapes the workspace")
	}
	scopeInfo, err := os.Stat(scope)
	if err != nil {
		return "", "", err
	}
	if !scopeInfo.IsDir() {
		return "", "", errors.New("read-only fan-out scope is not a directory")
	}
	return root, scope, nil
}

func collectSnapshotCandidates(ctx context.Context, root, scope string,
) ([]snapshotCandidate, int, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, 0, err
	}
	defer rootHandle.Close()
	candidates := make([]snapshotCandidate, 0, 64)
	excluded := 0
	entries := 0
	err = filepath.WalkDir(scope, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current == scope {
			return nil
		}
		entries++
		if entries > domain.MaxReadOnlyFanoutWalkEntries {
			return fmt.Errorf("read-only fan-out scope exceeds %d filesystem entries",
				domain.MaxReadOnlyFanoutWalkEntries)
		}
		name := strings.ToLower(entry.Name())
		if entry.IsDir() {
			if _, skip := skippedDirectories[name]; skip {
				excluded++
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			excluded++
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rootRelative := filepath.Clean(relative)
		info, err := rootHandle.Lstat(rootRelative)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || sensitiveFile(name) || !supportedSourceFile(name) ||
			info.Size() > domain.MaxReadOnlyFanoutFileBytes {
			excluded++
			return nil
		}
		content, err := rootHandle.ReadFile(rootRelative)
		if err != nil {
			return err
		}
		if !utf8.Valid(content) || containsNUL(content) {
			excluded++
			return nil
		}
		relative = filepath.ToSlash(relative)
		if len(candidates) >= domain.MaxReadOnlyFanoutFiles ||
			snapshotBytes(candidates)+int64(len(content)) > domain.MaxReadOnlyFanoutTotalBytes {
			excluded++
			return nil
		}
		digest := sha256.Sum256(content)
		candidates = append(candidates, snapshotCandidate{
			relativePath: relative, sizeBytes: int64(len(content)),
			digest: hex.EncodeToString(digest[:]),
		})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	slices.SortFunc(candidates, func(left, right snapshotCandidate) int {
		return strings.Compare(left.relativePath, right.relativePath)
	})
	return candidates, excluded, nil
}

func sameCanonicalPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if strings.EqualFold(left, right) {
		return true
	}
	return left == right
}

func partitionCandidates(planID string, candidates []snapshotCandidate,
	parallelism int,
) ([]domain.ReadOnlyFanoutFile, []int64, []int) {
	files := make([]domain.ReadOnlyFanoutFile, len(candidates))
	shardBytes := make([]int64, parallelism)
	shardFiles := make([]int, parallelism)
	for index, candidate := range candidates {
		shardIndex := 0
		for current := 1; current < parallelism; current++ {
			if shardBytes[current] < shardBytes[shardIndex] ||
				(shardBytes[current] == shardBytes[shardIndex] &&
					shardFiles[current] < shardFiles[shardIndex]) {
				shardIndex = current
			}
		}
		files[index] = domain.ReadOnlyFanoutFile{
			PlanID: planID, Ordinal: index + 1, ShardOrdinal: shardIndex + 1,
			RelativePath: candidate.relativePath, SizeBytes: candidate.sizeBytes,
			ContentSHA256: candidate.digest,
		}
		shardBytes[shardIndex] += candidate.sizeBytes
		shardFiles[shardIndex]++
	}
	return files, shardBytes, shardFiles
}

func supportedSourceFile(name string) bool {
	if _, allowed := supportedBasenames[name]; allowed {
		return true
	}
	_, allowed := supportedExtensions[strings.ToLower(filepath.Ext(name))]
	return allowed
}

func sensitiveFile(name string) bool {
	if name == ".env" || strings.HasPrefix(name, ".env.") {
		return true
	}
	if _, sensitive := sensitiveBasenames[name]; sensitive {
		return true
	}
	_, sensitive := sensitiveExtensions[strings.ToLower(filepath.Ext(name))]
	return sensitive
}

func snapshotBytes(candidates []snapshotCandidate) int64 {
	var total int64
	for _, candidate := range candidates {
		total += candidate.sizeBytes
	}
	return total
}

func containsNUL(content []byte) bool {
	for _, current := range content {
		if current == 0 {
			return true
		}
	}
	return false
}
