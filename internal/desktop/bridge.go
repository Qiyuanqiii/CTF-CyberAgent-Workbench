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
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/operationreceipt"
)

const (
	ConnectionBootstrapProtocolVersion = "desktop_connection_bootstrap.v1"
	SkillPackageDialogProtocolVersion  = "desktop_skill_package_dialog.v1"
	SkillPackageInstallProtocolVersion = "desktop_skill_package_install.v1"
	DesktopAPIBasePath                 = "/api/v1"

	desktopTokenMinBytes = 32
	desktopTokenMaxBytes = 512
)

// ConnectionBootstrap is delivered only through the same-origin native
// binding. Tokens stay in renderer memory and are never written to browser
// storage, SQLite, logs, command output, or the Windows registry.
type ConnectionBootstrap struct {
	ProtocolVersion               string `json:"protocol_version"`
	APIBaseURL                    string `json:"api_base_url"`
	APIVersion                    string `json:"api_version"`
	AppVersion                    string `json:"app_version"`
	UIDigest                      string `json:"ui_digest"`
	ReadToken                     string `json:"read_token"`
	ControlToken                  string `json:"control_token"`
	ControlEnabled                bool   `json:"control_enabled"`
	RunCreationEnabled            bool   `json:"run_creation_enabled"`
	SessionMessageEnabled         bool   `json:"session_message_enabled"`
	SessionSteeringControlEnabled bool   `json:"session_steering_control_enabled"`
	RunLifecycleEnabled           bool   `json:"run_lifecycle_enabled"`
	RunExecutionEnabled           bool   `json:"run_execution_enabled"`
	PlanDeliveryControlEnabled    bool   `json:"plan_delivery_control_enabled"`
	ApprovalControlEnabled        bool   `json:"approval_control_enabled"`
	ModelControlEnabled           bool   `json:"model_control_enabled"`
	ProviderCredentialEnabled     bool   `json:"provider_credential_enabled"`
	FileEditReviewEnabled         bool   `json:"file_edit_review_enabled"`
	FileEditProposalEnabled       bool   `json:"file_edit_proposal_enabled"`
	RunWakeControlEnabled         bool   `json:"run_wake_control_enabled"`
	FileEditApplyEnabled          bool   `json:"file_edit_apply_enabled"`
	RunWakeExecutionEnabled       bool   `json:"run_wake_execution_enabled"`
	RunWakeWorkerEnabled          bool   `json:"run_wake_worker_enabled"`
	ReadOnlyDefault               bool   `json:"read_only_default"`
	ProcessExecutionEnabled       bool   `json:"process_execution_enabled"`
	ShellExecutionEnabled         bool   `json:"shell_execution_enabled"`
	DockerExecutionEnabled        bool   `json:"docker_execution_enabled"`
	SkillInstallationEnabled      bool   `json:"skill_installation_enabled"`
	EvidenceAttachmentEnabled     bool   `json:"evidence_attachment_enabled"`
	VerificationEvidenceEnabled   bool   `json:"verification_evidence_enabled"`
	RendererPathInputSupported    bool   `json:"renderer_path_input_supported"`
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

type SkillPackageInstaller interface {
	Import(context.Context, application.ImportSkillPackageRequest) (
		application.ImportSkillPackageResult, error)
}

type SkillPackageInstallRequest struct {
	ProtocolVersion    string `json:"protocol_version"`
	ConfirmationHandle string `json:"confirmation_handle"`
	Surface            string `json:"surface"`
	OperationKey       string `json:"operation_key"`
	ConfirmUntrusted   bool   `json:"confirm_untrusted"`
}

type SkillPackageInstallResult struct {
	ProtocolVersion            string                   `json:"protocol_version"`
	Name                       string                   `json:"name"`
	Version                    string                   `json:"version"`
	Surface                    string                   `json:"surface"`
	TrustClass                 string                   `json:"trust_class"`
	ArchiveSHA256              string                   `json:"archive_sha256"`
	PackageFingerprint         string                   `json:"package_fingerprint"`
	Replayed                   bool                     `json:"replayed"`
	RecoveredPending           bool                     `json:"recovered_pending"`
	ImportCommandExecution     bool                     `json:"import_command_execution"`
	ImportNetworkAccess        bool                     `json:"import_network_access"`
	ImportProviderCalls        bool                     `json:"import_provider_calls"`
	ToolCapabilityGrant        bool                     `json:"tool_capability_grant"`
	RunSelectionAuthorized     bool                     `json:"run_selection_authorized"`
	ContextInjectionAuthorized bool                     `json:"context_injection_authorized"`
	Receipt                    operationreceipt.Receipt `json:"receipt"`
}

type DesktopBridgeConfig struct {
	ContextProvider               func() context.Context
	FilePicker                    SkillPackageFilePicker
	ReadToken                     string
	ControlToken                  string
	RunControlEnabled             bool
	RunCreationEnabled            bool
	SessionMessageEnabled         bool
	SessionSteeringControlEnabled bool
	RunLifecycleEnabled           bool
	RunExecutionEnabled           bool
	PlanDeliveryControlEnabled    bool
	ApprovalControlEnabled        bool
	ModelControlEnabled           bool
	ProviderCredentialEnabled     bool
	FileEditReviewEnabled         bool
	FileEditProposalEnabled       bool
	RunWakeControlEnabled         bool
	FileEditApplyEnabled          bool
	RunWakeExecutionEnabled       bool
	RunWakeWorkerEnabled          bool
	SkillInstallationEnabled      bool
	EvidenceAttachmentEnabled     bool
	VerificationEvidenceEnabled   bool
	APIVersion                    string
	AppVersion                    string
	UIDigest                      string
	Selector                      NativeSkillPackageSelector
	PreviewBridge                 *SkillPackagePreviewBridge
	SkillInstaller                SkillPackageInstaller
}

// DesktopBridge is the complete renderer binding surface for D0-A. Keep this
// type deliberately small: Wails binds every exported method.
type DesktopBridge struct {
	contextProvider func() context.Context
	filePicker      SkillPackageFilePicker
	selector        NativeSkillPackageSelector
	previewBridge   *SkillPackagePreviewBridge
	skillInstaller  SkillPackageInstaller
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
	controlEnabled := config.RunControlEnabled || config.RunCreationEnabled ||
		config.SessionMessageEnabled ||
		config.SessionSteeringControlEnabled || config.RunLifecycleEnabled ||
		config.RunExecutionEnabled || config.PlanDeliveryControlEnabled ||
		config.ApprovalControlEnabled || config.ModelControlEnabled ||
		config.ProviderCredentialEnabled || config.FileEditReviewEnabled ||
		config.FileEditProposalEnabled || config.RunWakeControlEnabled
	controlEnabled = controlEnabled || config.FileEditApplyEnabled ||
		config.RunWakeExecutionEnabled || config.RunWakeWorkerEnabled ||
		config.SkillInstallationEnabled ||
		config.EvidenceAttachmentEnabled || config.VerificationEvidenceEnabled
	if controlEnabled && config.ControlToken == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop control capabilities require a control token")
	}
	if config.ControlToken != "" && !controlEnabled {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop control token requires an enabled control capability")
	}
	if config.SkillInstallationEnabled && config.SkillInstaller == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop Skill installation requires the Go Registry installer")
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
		skillInstaller:  config.SkillInstaller,
		bootstrap: ConnectionBootstrap{
			ProtocolVersion: ConnectionBootstrapProtocolVersion,
			APIBaseURL:      DesktopAPIBasePath, APIVersion: apiVersion, AppVersion: appVersion,
			UIDigest: config.UIDigest, ReadToken: config.ReadToken, ControlToken: config.ControlToken,
			ControlEnabled:                config.RunControlEnabled,
			RunCreationEnabled:            config.RunCreationEnabled,
			SessionMessageEnabled:         config.SessionMessageEnabled,
			SessionSteeringControlEnabled: config.SessionSteeringControlEnabled,
			RunLifecycleEnabled:           config.RunLifecycleEnabled,
			RunExecutionEnabled:           config.RunExecutionEnabled,
			PlanDeliveryControlEnabled:    config.PlanDeliveryControlEnabled,
			ApprovalControlEnabled:        config.ApprovalControlEnabled,
			ModelControlEnabled:           config.ModelControlEnabled,
			ProviderCredentialEnabled:     config.ProviderCredentialEnabled,
			FileEditReviewEnabled:         config.FileEditReviewEnabled,
			FileEditProposalEnabled:       config.FileEditProposalEnabled,
			RunWakeControlEnabled:         config.RunWakeControlEnabled,
			FileEditApplyEnabled:          config.FileEditApplyEnabled,
			RunWakeExecutionEnabled:       config.RunWakeExecutionEnabled,
			RunWakeWorkerEnabled:          config.RunWakeWorkerEnabled,
			ReadOnlyDefault:               !controlEnabled,
			ProcessExecutionEnabled:       false, ShellExecutionEnabled: false, DockerExecutionEnabled: false,
			SkillInstallationEnabled:    config.SkillInstallationEnabled,
			EvidenceAttachmentEnabled:   config.EvidenceAttachmentEnabled,
			VerificationEvidenceEnabled: config.VerificationEvidenceEnabled,
			RendererPathInputSupported:  false,
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

// InstallSkillPackage consumes a short-lived preview confirmation. Package
// bytes remain in Go and flow only through the inert content-addressed
// Registry; this method cannot execute scripts, hooks, commands, tools,
// provider calls, or network requests.
func (b *DesktopBridge) InstallSkillPackage(
	request SkillPackageInstallRequest,
) (SkillPackageInstallResult, error) {
	if b == nil || !b.bootstrap.SkillInstallationEnabled || b.skillInstaller == nil ||
		b.previewBridge == nil {
		return SkillPackageInstallResult{}, apperror.New(apperror.CodeNotFound,
			"desktop Skill installation is disabled")
	}
	if request.ProtocolVersion != SkillPackageInstallProtocolVersion ||
		!request.ConfirmUntrusted {
		return SkillPackageInstallResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop Skill installation requires explicit untrusted-content confirmation")
	}
	surface, err := domain.ParseExecutionSurface(request.Surface)
	if err != nil {
		return SkillPackageInstallResult{}, apperror.Normalize(err)
	}
	ctx, err := b.lifecycleContext()
	if err != nil {
		return SkillPackageInstallResult{}, err
	}
	raw, preview, err := b.previewBridge.ConsumeInstall(ctx,
		request.ConfirmationHandle)
	if err != nil {
		return SkillPackageInstallResult{}, err
	}
	result, err := b.skillInstaller.Import(ctx, application.ImportSkillPackageRequest{
		Raw: raw, Surface: surface, OperationKey: request.OperationKey,
		InstalledBy: "desktop_operator", ConfirmUntrusted: true,
	})
	if err != nil {
		return SkillPackageInstallResult{}, apperror.Normalize(err)
	}
	installation := result.Package.Installation
	if installation.Name != preview.Name || installation.Version != preview.Version ||
		installation.ArchiveSHA256 != preview.ArchiveSHA256 ||
		installation.PackageFingerprint != preview.PackageFingerprint ||
		installation.Surface != surface {
		return SkillPackageInstallResult{}, apperror.New(apperror.CodeInternal,
			"desktop Skill installation result violated its preview binding")
	}
	return SkillPackageInstallResult{
		ProtocolVersion: SkillPackageInstallProtocolVersion,
		Name:            installation.Name, Version: installation.Version,
		Surface: string(installation.Surface), TrustClass: string(installation.TrustClass),
		ArchiveSHA256:      installation.ArchiveSHA256,
		PackageFingerprint: installation.PackageFingerprint,
		Replayed:           result.Replayed, RecoveredPending: result.RecoveredPending,
		ImportCommandExecution:     installation.ImportCommandExecution,
		ImportNetworkAccess:        installation.ImportNetworkAccess,
		ImportProviderCalls:        installation.ImportProviderCalls,
		ToolCapabilityGrant:        installation.ToolCapabilityGrant,
		RunSelectionAuthorized:     installation.RunSelectionAuthorized,
		ContextInjectionAuthorized: installation.ContextInjectionAuthorized,
		Receipt: operationreceipt.Settled(operationreceipt.KindSkillPackageInstall,
			result.Replayed, false),
	}, nil
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
