package desktop

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
)

const (
	ConnectionBootstrapProtocolVersion = "desktop_connection_bootstrap.v1"
	SkillPackageDialogProtocolVersion  = "desktop_skill_package_dialog.v1"
	DesktopAPIBasePath                 = "/api/v1"

	desktopTokenMinBytes = 32
	desktopTokenMaxBytes = 512
)

// ConnectionBootstrap is delivered only through the same-origin native
// binding. Tokens stay in renderer memory and are never written to browser
// storage, SQLite, logs, command output, or the Windows registry.
type ConnectionBootstrap struct {
	ProtocolVersion            string `json:"protocol_version"`
	APIBaseURL                 string `json:"api_base_url"`
	APIVersion                 string `json:"api_version"`
	AppVersion                 string `json:"app_version"`
	UIDigest                   string `json:"ui_digest"`
	ReadToken                  string `json:"read_token"`
	ControlToken               string `json:"control_token"`
	ControlEnabled             bool   `json:"control_enabled"`
	ReadOnlyDefault            bool   `json:"read_only_default"`
	ProcessExecutionEnabled    bool   `json:"process_execution_enabled"`
	ShellExecutionEnabled      bool   `json:"shell_execution_enabled"`
	DockerExecutionEnabled     bool   `json:"docker_execution_enabled"`
	SkillInstallationEnabled   bool   `json:"skill_installation_enabled"`
	RendererPathInputSupported bool   `json:"renderer_path_input_supported"`
}

type SkillPackageDialogStatus string

const (
	SkillPackageDialogSelected  SkillPackageDialogStatus = "selected"
	SkillPackageDialogCancelled SkillPackageDialogStatus = "cancelled"
)

// SkillPackageDialogResult contains either an opaque selection handle or an
// explicit cancellation result. It cannot represent a local path.
type SkillPackageDialogResult struct {
	ProtocolVersion string                   `json:"protocol_version"`
	Status          SkillPackageDialogStatus `json:"status"`
	Selection       *SkillPackageSelection   `json:"selection"`
}

// SkillPackageFilePicker is implemented by the native shell. The selected
// path crosses this interface only inside Go and is immediately validated by
// the pathless preview boundary.
type SkillPackageFilePicker interface {
	OpenSkillPackage(context.Context) (string, error)
}

type DesktopBridgeConfig struct {
	ContextProvider func() context.Context
	FilePicker      SkillPackageFilePicker
	ReadToken       string
	ControlToken    string
	APIVersion      string
	AppVersion      string
	UIDigest        string
	Selector        NativeSkillPackageSelector
	PreviewBridge   *SkillPackagePreviewBridge
}

// DesktopBridge is the complete renderer binding surface for D0-A. Keep this
// type deliberately small: Wails binds every exported method.
type DesktopBridge struct {
	contextProvider func() context.Context
	filePicker      SkillPackageFilePicker
	selector        NativeSkillPackageSelector
	previewBridge   *SkillPackagePreviewBridge
	bootstrap       ConnectionBootstrap
	dialogActive    atomic.Bool
}

func NewDesktopBridge(config DesktopBridgeConfig) (*DesktopBridge, error) {
	if config.ContextProvider == nil || config.FilePicker == nil || config.Selector == nil ||
		config.PreviewBridge == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop bridge dependencies are required")
	}
	if !validDesktopToken(config.ReadToken) ||
		(config.ControlToken != "" && !validDesktopToken(config.ControlToken)) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop bridge tokens must be normalized bounded values")
	}
	readHash := sha256.Sum256([]byte(config.ReadToken))
	controlHash := sha256.Sum256([]byte(config.ControlToken))
	if config.ControlToken != "" && subtle.ConstantTimeCompare(readHash[:], controlHash[:]) == 1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop read and control tokens must be distinct")
	}
	apiVersion := strings.TrimSpace(config.APIVersion)
	appVersion := strings.TrimSpace(config.AppVersion)
	if apiVersion == "" || len(apiVersion) > 64 || appVersion == "" || len(appVersion) > 64 ||
		!validSHA256(config.UIDigest) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop bridge version and UI digest metadata are invalid")
	}
	return &DesktopBridge{
		contextProvider: config.ContextProvider,
		filePicker:      config.FilePicker,
		selector:        config.Selector,
		previewBridge:   config.PreviewBridge,
		bootstrap: ConnectionBootstrap{
			ProtocolVersion: ConnectionBootstrapProtocolVersion,
			APIBaseURL:      DesktopAPIBasePath, APIVersion: apiVersion, AppVersion: appVersion,
			UIDigest: config.UIDigest, ReadToken: config.ReadToken, ControlToken: config.ControlToken,
			ControlEnabled: config.ControlToken != "", ReadOnlyDefault: config.ControlToken == "",
			ProcessExecutionEnabled: false, ShellExecutionEnabled: false, DockerExecutionEnabled: false,
			SkillInstallationEnabled: false, RendererPathInputSupported: false,
		},
	}, nil
}

// Bootstrap returns same-origin in-memory connection material. It performs no
// file, process, network, model, Docker, or database operation.
func (b *DesktopBridge) Bootstrap() (ConnectionBootstrap, error) {
	if b == nil {
		return ConnectionBootstrap{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop bridge is unavailable")
	}
	return b.bootstrap, nil
}

// SelectSkillPackage opens the native dialog and returns only an opaque handle.
// Concurrent dialog requests fail closed rather than queuing more OS dialogs.
func (b *DesktopBridge) SelectSkillPackage() (SkillPackageDialogResult, error) {
	if b == nil || b.contextProvider == nil || b.filePicker == nil || b.selector == nil {
		return SkillPackageDialogResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop skill package dialog is unavailable")
	}
	if !b.dialogActive.CompareAndSwap(false, true) {
		return SkillPackageDialogResult{}, apperror.New(apperror.CodeResourceExhausted,
			"desktop skill package dialog is already active")
	}
	defer b.dialogActive.Store(false)

	ctx, err := b.lifecycleContext()
	if err != nil {
		return SkillPackageDialogResult{}, err
	}
	selectedPath, err := b.filePicker.OpenSkillPackage(ctx)
	if err != nil {
		return SkillPackageDialogResult{}, apperror.New(apperror.CodeUnavailable,
			"native skill package dialog failed")
	}
	if err := ctx.Err(); err != nil {
		return SkillPackageDialogResult{}, apperror.Normalize(err)
	}
	if selectedPath == "" {
		return SkillPackageDialogResult{
			ProtocolVersion: SkillPackageDialogProtocolVersion,
			Status:          SkillPackageDialogCancelled,
			Selection:       nil,
		}, nil
	}
	selection, err := b.selector(ctx, selectedPath)
	if err != nil {
		return SkillPackageDialogResult{}, apperror.Normalize(err)
	}
	return SkillPackageDialogResult{
		ProtocolVersion: SkillPackageDialogProtocolVersion,
		Status:          SkillPackageDialogSelected,
		Selection:       &selection,
	}, nil
}

// PreviewSkillPackage consumes a native-issued handle. No renderer-provided
// path, file bytes, URL, command, or installation request is accepted.
func (b *DesktopBridge) PreviewSkillPackage(handle string) (SkillPackagePreview, error) {
	if b == nil || b.previewBridge == nil {
		return SkillPackagePreview{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop skill package preview is unavailable")
	}
	ctx, err := b.lifecycleContext()
	if err != nil {
		return SkillPackagePreview{}, err
	}
	return b.previewBridge.Preview(ctx, handle)
}

func (b *DesktopBridge) lifecycleContext() (context.Context, error) {
	ctx := b.contextProvider()
	if ctx == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"desktop lifecycle is not ready")
	}
	if err := ctx.Err(); err != nil {
		return nil, apperror.Normalize(err)
	}
	return ctx, nil
}

func validDesktopToken(value string) bool {
	if value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len([]byte(value)) < desktopTokenMinBytes || len([]byte(value)) > desktopTokenMaxBytes {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size
}
