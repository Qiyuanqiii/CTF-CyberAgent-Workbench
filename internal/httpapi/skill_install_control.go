package httpapi

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/skills"
)

const (
	SkillPackageInstallPath         = "/api/v1/skills/packages/install"
	MaxSkillPackageInstallBodyBytes = 96 * 1024
)

type SkillInstallationController interface {
	Import(context.Context, application.ImportSkillPackageRequest) (
		application.ImportSkillPackageResult, error)
}

type SkillPackageInstallRequestView struct {
	Version          string `json:"version"`
	ArchiveBase64    string `json:"archive_base64"`
	Surface          string `json:"surface"`
	ConfirmUntrusted bool   `json:"confirm_untrusted"`
}

type SkillPackageInstallView struct {
	ProtocolVersion            string `json:"protocol_version"`
	Name                       string `json:"name"`
	Version                    string `json:"version"`
	Surface                    string `json:"surface"`
	TrustClass                 string `json:"trust_class"`
	ArchiveSHA256              string `json:"archive_sha256"`
	PackageFingerprint         string `json:"package_fingerprint"`
	Replayed                   bool   `json:"replayed"`
	RecoveredPending           bool   `json:"recovered_pending"`
	ImportCommandExecution     bool   `json:"import_command_execution"`
	ImportNetworkAccess        bool   `json:"import_network_access"`
	ImportProviderCalls        bool   `json:"import_provider_calls"`
	ToolCapabilityGrant        bool   `json:"tool_capability_grant"`
	RunSelectionAuthorized     bool   `json:"run_selection_authorized"`
	ContextInjectionAuthorized bool   `json:"context_injection_authorized"`
}

func (a *API) serveSkillPackageInstallControl(writer http.ResponseWriter,
	request *http.Request, requestID string,
) {
	const label = "Skill package installation"
	if !a.authorizeRunOperation(writer, request, requestID,
		a.skillInstallationEnabled, label) {
		return
	}
	if err := rejectQuery(request.URL.Query()); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validateJSONContentType(request.Header); err != nil {
		a.writeError(writer, requestID, err, http.StatusUnsupportedMediaType)
		return
	}
	operationKey, err := sessionControlIdempotencyKey(request.Header, label)
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readBoundedRequestBody(request, MaxSkillPackageInstallBodyBytes)
	if err != nil {
		a.writeError(writer, requestID, err, runOperationErrorStatus(err))
		return
	}
	if err := rejectDuplicateJSONObjectFields(body, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	var view SkillPackageInstallRequestView
	if err := decodeStrictRunOperation(body, &view, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if view.Version != skills.PackageInstallationProtocolVersion ||
		!view.ConfirmUntrusted || strings.TrimSpace(view.ArchiveBase64) != view.ArchiveBase64 ||
		strings.ContainsAny(view.ArchiveBase64, " \t\r\n") {
		a.writeError(writer, requestID, apperror.New(apperror.CodeInvalidArgument,
			"Skill package installation confirmation or archive encoding is invalid"), 0)
		return
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(view.ArchiveBase64)
	if err != nil || len(raw) == 0 || len(raw) > skills.MaxPackageArchiveBytes {
		a.writeError(writer, requestID, apperror.New(apperror.CodeInvalidArgument,
			"Skill package archive must be canonical bounded base64"), 0)
		return
	}
	surface, err := domain.ParseExecutionSurface(view.Surface)
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	result, err := a.skillInstallationController.Import(request.Context(),
		application.ImportSkillPackageRequest{Raw: raw, Surface: surface,
			OperationKey: operationKey, InstalledBy: "http_operator",
			ConfirmUntrusted: true})
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	installation := result.Package.Installation
	a.writeSuccessStatus(writer, requestID, SkillPackageInstallView{
		ProtocolVersion: skills.PackageInstallationProtocolVersion,
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
	}, nil, http.StatusAccepted)
}
