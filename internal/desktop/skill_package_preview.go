// Package desktop contains Go-owned boundaries intended for a future native
// desktop shell. It does not provide an HTTP API or a second control plane.
package desktop

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
	"sync"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/skills"
)

const (
	SkillPackageSelectionProtocolVersion = "desktop_file_selection.v1"
	SkillPackagePreviewProtocolVersion   = "desktop_skill_package_preview.v1"
	DefaultSkillPackageSelectionTTL      = 5 * time.Minute
	MaxPendingSkillPackageSelections     = 16

	skillPackageSelectionTokenBytes  = 32
	skillPackageSelectionTokenLength = 43
	skillPackageSelectionAttempts    = 8
)

// SkillPackageSelection is the only value a native picker returns to the
// renderer. It intentionally contains neither a path nor file content.
type SkillPackageSelection struct {
	ProtocolVersion string    `json:"protocol_version"`
	Handle          string    `json:"handle"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// SkillPackagePreview is a bounded renderer-safe projection. Manifest
// descriptions, content paths, bodies, and source paths are excluded because
// they are untrusted input rather than UI authority.
type SkillPackagePreview struct {
	ProtocolVersion        string    `json:"protocol_version"`
	PackageProtocol        string    `json:"package_protocol"`
	SkillProtocol          string    `json:"skill_protocol"`
	Name                   string    `json:"name"`
	Version                string    `json:"version"`
	Profiles               []string  `json:"profiles"`
	DeclaredTools          []string  `json:"declared_tools"`
	DeclaredToolCount      int       `json:"declared_tool_count"`
	ContentBytes           int       `json:"content_bytes"`
	ContentTokenUpperBound int       `json:"content_token_upper_bound"`
	ArchiveSHA256          string    `json:"archive_sha256"`
	PackageFingerprint     string    `json:"package_fingerprint"`
	ArchiveBytes           int       `json:"archive_bytes"`
	UncompressedBytes      int       `json:"uncompressed_bytes"`
	EntryCount             int       `json:"entry_count"`
	TrustClass             string    `json:"trust_class"`
	RiskCodes              []string  `json:"risk_codes"`
	ExecutableAssetCount   int       `json:"executable_asset_count"`
	InstallHookCount       int       `json:"install_hook_count"`
	ImportCommandExecution bool      `json:"import_command_execution"`
	ImportNetworkAccess    bool      `json:"import_network_access"`
	ImportProviderCalls    bool      `json:"import_provider_calls"`
	ToolCapabilityGrant    bool      `json:"tool_capability_grant"`
	InstallationAuthorized bool      `json:"installation_authorized"`
	Validated              bool      `json:"validated"`
	ConfirmationHandle     string    `json:"confirmation_handle"`
	ConfirmationExpiresAt  time.Time `json:"confirmation_expires_at"`
}

// NativeSkillPackageSelector is held only by the future Go native-shell
// adapter. The renderer-facing bridge receives no method that accepts a path.
type NativeSkillPackageSelector func(context.Context, string) (SkillPackageSelection, error)

type pendingSkillPackagePreview struct {
	preview   SkillPackagePreview
	raw       []byte
	expiresAt time.Time
}

type pendingSkillPackageInstall struct {
	preview   SkillPackagePreview
	raw       []byte
	expiresAt time.Time
}

type skillPackagePreviewBroker struct {
	mu       sync.Mutex
	now      func() time.Time
	random   io.Reader
	ttl      time.Duration
	limit    int
	pending  map[string]pendingSkillPackagePreview
	installs map[string]pendingSkillPackageInstall
}

// SkillPackagePreviewBridge is safe to bind to a renderer: its only input is
// a short-lived opaque handle issued after native Go validation has completed.
type SkillPackagePreviewBridge struct {
	broker *skillPackagePreviewBroker
}

// NewSkillPackagePreviewBoundary returns two deliberately separate halves.
// A desktop shell keeps the selector in Go and binds only the bridge.
func NewSkillPackagePreviewBoundary() (NativeSkillPackageSelector, *SkillPackagePreviewBridge) {
	return newSkillPackagePreviewBoundary(time.Now, rand.Reader,
		DefaultSkillPackageSelectionTTL, MaxPendingSkillPackageSelections)
}

func newSkillPackagePreviewBoundary(now func() time.Time, random io.Reader, ttl time.Duration,
	limit int,
) (NativeSkillPackageSelector, *SkillPackagePreviewBridge) {
	broker := &skillPackagePreviewBroker{
		now: now, random: random, ttl: ttl, limit: limit,
		pending:  make(map[string]pendingSkillPackagePreview),
		installs: make(map[string]pendingSkillPackageInstall),
	}
	selector := func(ctx context.Context, path string) (SkillPackageSelection, error) {
		if err := ctx.Err(); err != nil {
			return SkillPackageSelection{}, apperror.Normalize(err)
		}
		raw, err := skills.ReadPackageFile(ctx, path)
		if err != nil {
			return SkillPackageSelection{}, apperror.Normalize(err)
		}
		parsed, err := skills.ParsePackage(raw)
		if err != nil {
			return SkillPackageSelection{}, apperror.Normalize(err)
		}
		return broker.issue(ctx, projectSkillPackagePreview(parsed.Preview()), raw)
	}
	return selector, &SkillPackagePreviewBridge{broker: broker}
}

// Preview consumes one selection handle. Re-selection is required for every
// retry so stale renderer state cannot become a durable local-file capability.
func (b *SkillPackagePreviewBridge) Preview(ctx context.Context, handle string) (SkillPackagePreview, error) {
	if b == nil || b.broker == nil {
		return SkillPackagePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop skill package preview bridge is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return SkillPackagePreview{}, apperror.Normalize(err)
	}
	if !validSkillPackageSelectionHandle(handle) {
		return SkillPackagePreview{}, apperror.New(apperror.CodeInvalidArgument,
			"desktop skill package selection handle is invalid")
	}

	now := b.broker.now().UTC()
	b.broker.mu.Lock()
	b.broker.purgeExpired(now)
	pending, found := b.broker.pending[handle]
	if found {
		delete(b.broker.pending, handle)
	}
	if !found {
		b.broker.mu.Unlock()
		return SkillPackagePreview{}, apperror.New(apperror.CodeNotFound,
			"desktop skill package selection handle is unavailable")
	}
	confirmationHandle, err := b.broker.issueTokenLocked()
	if err != nil {
		b.broker.mu.Unlock()
		return SkillPackagePreview{}, err
	}
	expiresAt := now.Add(b.broker.ttl)
	b.broker.installs[confirmationHandle] = pendingSkillPackageInstall{
		preview: cloneSkillPackagePreview(pending.preview),
		raw:     append([]byte(nil), pending.raw...), expiresAt: expiresAt,
	}
	b.broker.mu.Unlock()
	preview := cloneSkillPackagePreview(pending.preview)
	preview.ConfirmationHandle = confirmationHandle
	preview.ConfirmationExpiresAt = expiresAt
	return preview, nil
}

// ConsumeInstall consumes one preview-issued confirmation handle and returns
// validated archive bytes only to Go. The renderer never supplies a path or
// receives package content through this boundary.
func (b *SkillPackagePreviewBridge) ConsumeInstall(ctx context.Context,
	handle string,
) ([]byte, SkillPackagePreview, error) {
	if b == nil || b.broker == nil {
		return nil, SkillPackagePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop skill package installation bridge is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, SkillPackagePreview{}, apperror.Normalize(err)
	}
	if !validSkillPackageSelectionHandle(handle) {
		return nil, SkillPackagePreview{}, apperror.New(apperror.CodeInvalidArgument,
			"desktop skill package confirmation handle is invalid")
	}
	now := b.broker.now().UTC()
	b.broker.mu.Lock()
	b.broker.purgeExpired(now)
	pending, found := b.broker.installs[handle]
	if found {
		delete(b.broker.installs, handle)
	}
	b.broker.mu.Unlock()
	if !found {
		return nil, SkillPackagePreview{}, apperror.New(apperror.CodeNotFound,
			"desktop skill package confirmation handle is unavailable")
	}
	return append([]byte(nil), pending.raw...), cloneSkillPackagePreview(pending.preview), nil
}

func (b *skillPackagePreviewBroker) issue(ctx context.Context,
	preview SkillPackagePreview, raw []byte,
) (SkillPackageSelection, error) {
	if b == nil || b.now == nil || b.random == nil || b.ttl <= 0 || b.limit <= 0 {
		return SkillPackageSelection{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop skill package preview boundary is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return SkillPackageSelection{}, apperror.Normalize(err)
	}
	now := b.now().UTC()
	expiresAt := now.Add(b.ttl)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.purgeExpired(now)
	if len(b.pending)+len(b.installs) >= b.limit {
		return SkillPackageSelection{}, apperror.New(apperror.CodeResourceExhausted,
			"desktop skill package selection capacity is exhausted")
	}
	handle, err := b.issueTokenLocked()
	if err != nil {
		return SkillPackageSelection{}, err
	}
	b.pending[handle] = pendingSkillPackagePreview{
		preview: cloneSkillPackagePreview(preview), raw: append([]byte(nil), raw...),
		expiresAt: expiresAt,
	}
	return SkillPackageSelection{ProtocolVersion: SkillPackageSelectionProtocolVersion,
		Handle: handle, ExpiresAt: expiresAt}, nil
}

func (b *skillPackagePreviewBroker) purgeExpired(now time.Time) {
	for handle, pending := range b.pending {
		if !now.Before(pending.expiresAt) {
			delete(b.pending, handle)
		}
	}
	for handle, pending := range b.installs {
		if !now.Before(pending.expiresAt) {
			delete(b.installs, handle)
		}
	}
}

func (b *skillPackagePreviewBroker) issueTokenLocked() (string, error) {
	for range skillPackageSelectionAttempts {
		var token [skillPackageSelectionTokenBytes]byte
		if _, err := io.ReadFull(b.random, token[:]); err != nil {
			return "", apperror.New(apperror.CodeInternal,
				"desktop skill package selection handle cannot be generated")
		}
		handle := base64.RawURLEncoding.EncodeToString(token[:])
		if _, selected := b.pending[handle]; selected {
			continue
		}
		if _, installable := b.installs[handle]; installable {
			continue
		}
		return handle, nil
	}
	return "", apperror.New(apperror.CodeInternal,
		"desktop skill package selection handle collision limit was reached")
}

func validSkillPackageSelectionHandle(value string) bool {
	if len(value) != skillPackageSelectionTokenLength || strings.TrimSpace(value) != value {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == skillPackageSelectionTokenBytes
}

func projectSkillPackagePreview(value skills.PackagePreview) SkillPackagePreview {
	profiles := make([]string, len(value.Manifest.Profiles))
	for index, profile := range value.Manifest.Profiles {
		profiles[index] = string(profile)
	}
	tools := make([]string, len(value.Manifest.ToolDependencies))
	for index, tool := range value.Manifest.ToolDependencies {
		tools[index] = string(tool)
	}
	risks := make([]string, len(value.RiskCodes))
	for index, risk := range value.RiskCodes {
		risks[index] = string(risk)
	}
	return SkillPackagePreview{
		ProtocolVersion:        SkillPackagePreviewProtocolVersion,
		PackageProtocol:        value.ProtocolVersion,
		SkillProtocol:          value.Manifest.Protocol,
		Name:                   value.Manifest.Name,
		Version:                value.Manifest.Version,
		Profiles:               profiles,
		DeclaredTools:          tools,
		DeclaredToolCount:      len(tools),
		ContentBytes:           value.Manifest.ContentBytes,
		ContentTokenUpperBound: value.Manifest.ContentTokenUpperBound,
		ArchiveSHA256:          value.ArchiveSHA256,
		PackageFingerprint:     value.PackageFingerprint,
		ArchiveBytes:           value.ArchiveBytes,
		UncompressedBytes:      value.UncompressedBytes,
		EntryCount:             value.EntryCount,
		TrustClass:             string(value.TrustClass),
		RiskCodes:              risks,
		ExecutableAssetCount:   value.ExecutableAssetCount,
		InstallHookCount:       value.InstallHookCount,
		ImportCommandExecution: value.ImportCommandExecution,
		ImportNetworkAccess:    value.ImportNetworkAccess,
		ImportProviderCalls:    value.ImportProviderCalls,
		ToolCapabilityGrant:    value.ToolCapabilityGrant,
		InstallationAuthorized: value.InstallationAuthorized,
		Validated:              true,
	}
}

func cloneSkillPackagePreview(value SkillPackagePreview) SkillPackagePreview {
	value.Profiles = append([]string(nil), value.Profiles...)
	value.DeclaredTools = append([]string(nil), value.DeclaredTools...)
	value.RiskCodes = append([]string(nil), value.RiskCodes...)
	return value
}
