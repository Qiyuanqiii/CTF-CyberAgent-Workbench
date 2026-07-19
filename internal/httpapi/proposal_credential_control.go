package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/credential"
)

const (
	FileEditProposalSourcePathTemplate   = "/api/v1/runs/{run_id}/file-edit-proposal-source"
	FileEditProposalRecoveryPathTemplate = "/api/v1/runs/{run_id}/file-edit-proposal-recovery/{edit_id}"
	FileEditProposalPathTemplate         = "/api/v1/runs/{run_id}/file-edit-proposals"
	ProviderCredentialsPath              = "/api/v1/models/credentials"
	ProviderCredentialPathTemplate       = "/api/v1/models/credentials/{provider}"
)

type FileEditProposalController interface {
	IssueSource(context.Context, string, string) (
		application.FileEditProposalSource, error)
	ReissueSource(context.Context, string, string, string) (
		application.FileEditProposalSource, error)
	Recover(context.Context, string, string) (
		application.FileEditProposalRecovery, error)
	Propose(context.Context, application.CreateFileEditProposalRequest) (
		application.CreateFileEditProposalResult, error)
}

type ProviderCredentialController interface {
	List(context.Context) ([]application.ProviderCredentialStatus, error)
	Change(context.Context, application.ChangeProviderCredentialRequest) (
		application.ProviderCredentialStatus, error)
}

type FileEditProposalSourceView struct {
	ProtocolVersion string    `json:"protocol_version"`
	RunID           string    `json:"run_id"`
	WorkspaceID     string    `json:"workspace_id"`
	Path            string    `json:"path"`
	Content         string    `json:"content"`
	ContentSHA256   string    `json:"content_sha256"`
	SourceHandle    string    `json:"source_handle"`
	ExpiresAt       time.Time `json:"expires_at"`
	Editable        bool      `json:"editable"`
	FileWrite       bool      `json:"file_write"`
}

type FileEditProposalRequestView struct {
	Version      string `json:"version"`
	SourceHandle string `json:"source_handle"`
	ProposedText string `json:"proposed_text"`
}

type FileEditProposalView struct {
	ProtocolVersion  string              `json:"protocol_version"`
	RunID            string              `json:"run_id"`
	Edit             FileEditPreviewView `json:"edit"`
	Replayed         bool                `json:"replayed"`
	ApprovalRequired bool                `json:"approval_required"`
	FileWritten      bool                `json:"file_written"`
}

type FileEditProposalRecoveryView struct {
	ProtocolVersion      string `json:"protocol_version"`
	RunID                string `json:"run_id"`
	WorkspaceID          string `json:"workspace_id"`
	EditID               string `json:"edit_id"`
	Path                 string `json:"path"`
	OriginalContent      string `json:"original_content"`
	ProposedContent      string `json:"proposed_content"`
	OriginalSHA256       string `json:"original_sha256"`
	ProposedSHA256       string `json:"proposed_sha256"`
	CurrentContentSHA256 string `json:"current_content_sha256"`
	Status               string `json:"status"`
	Stale                bool   `json:"stale"`
	ReviewRequired       bool   `json:"review_required"`
	Editable             bool   `json:"editable"`
	FileWrite            bool   `json:"file_write"`
}

type ProviderCredentialRequestView struct {
	Version string                               `json:"version"`
	Action  application.ProviderCredentialAction `json:"action"`
	Secret  string                               `json:"secret"`
	Confirm bool                                 `json:"confirm"`
}

type ProviderCredentialStatusView struct {
	ProtocolVersion    string `json:"protocol_version"`
	Provider           string `json:"provider"`
	Configured         bool   `json:"configured"`
	StoreKind          string `json:"store_kind"`
	StoreAvailable     bool   `json:"store_available"`
	PlaintextReturned  bool   `json:"plaintext_returned"`
	RestartRequired    bool   `json:"restart_required"`
	RegistryReloaded   bool   `json:"registry_reloaded"`
	RegistryGeneration uint64 `json:"registry_generation"`
}

type ProviderCredentialListView struct {
	ProtocolVersion string                         `json:"protocol_version"`
	Items           []ProviderCredentialStatusView `json:"items"`
}

func matchFileEditProposalControlPath(requestPath string) (string, bool) {
	const prefix = "/api/v1/runs/"
	const suffix = "/file-edit-proposals"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	return runID, runID != "" && !strings.Contains(runID, "/")
}

func matchProviderCredentialControlPath(requestPath string) (string, bool) {
	const prefix = "/api/v1/models/credentials/"
	if !strings.HasPrefix(requestPath, prefix) {
		return "", false
	}
	provider := strings.TrimPrefix(requestPath, prefix)
	return provider, provider != "" && !strings.Contains(provider, "/")
}

func (a *API) serveFileEditProposalControl(writer http.ResponseWriter,
	request *http.Request, requestID string, runID string,
) {
	const label = "File edit proposal"
	if !a.authorizeRunOperation(writer, request, requestID,
		a.fileEditProposalEnabled, label) {
		return
	}
	if err := validatePathIdentity(runID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readStrictControlBody(request, label)
	if err != nil {
		a.writeError(writer, requestID, err, runOperationErrorStatus(err))
		return
	}
	var view FileEditProposalRequestView
	if err := decodeStrictRunOperation(body, &view, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	result, err := a.fileEditProposalController.Propose(request.Context(),
		application.CreateFileEditProposalRequest{Version: view.Version, RunID: runID,
			SourceHandle: view.SourceHandle, ProposedText: view.ProposedText})
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	a.writeSuccessStatus(writer, requestID, FileEditProposalView{
		ProtocolVersion: application.FileEditProposalProtocolVersion, RunID: runID,
		Edit: fileEditView(result.Edit, false), Replayed: result.Replayed,
		ApprovalRequired: true, FileWritten: false,
	}, nil, http.StatusAccepted)
}

func (a *API) serveProviderCredentialControl(writer http.ResponseWriter,
	request *http.Request, requestID string, provider string,
) {
	const label = "Provider credential control"
	if !a.authorizeRunOperation(writer, request, requestID,
		a.providerCredentialEnabled, label) {
		return
	}
	if err := validatePathIdentity(provider); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readStrictControlBody(request, label)
	if err != nil {
		a.writeError(writer, requestID, err, runOperationErrorStatus(err))
		return
	}
	defer clear(body)
	var view ProviderCredentialRequestView
	if err := decodeStrictRunOperation(body, &view, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	clear(body)
	body = nil
	status, err := a.providerCredentialController.Change(request.Context(),
		application.ChangeProviderCredentialRequest{Version: view.Version,
			Provider: provider, Action: view.Action, Secret: view.Secret,
			Confirm: view.Confirm})
	view.Secret = ""
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	a.writeSuccessStatus(writer, requestID, providerCredentialStatusView(status), nil,
		http.StatusAccepted)
}

func (a *API) runFileEditProposalSource(request *http.Request,
	runID string,
) (any, *Page, error) {
	if !a.fileEditProposalEnabled || a.fileEditProposalController == nil {
		return nil, nil, apperror.New(apperror.CodeNotFound,
			"file edit proposal source is unavailable")
	}
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "path", "expected_sha256"); err != nil {
		return nil, nil, err
	}
	paths, found := values["path"]
	if !found || len(paths) != 1 || paths[0] == "" {
		return nil, nil, apperror.New(apperror.CodeInvalidArgument,
			"file edit proposal source path is required")
	}
	var source application.FileEditProposalSource
	var err error
	expected, reissue := singleQueryValue(values, "expected_sha256")
	if reissue {
		source, err = a.fileEditProposalController.ReissueSource(request.Context(),
			runID, paths[0], expected)
	} else {
		source, err = a.fileEditProposalController.IssueSource(request.Context(),
			runID, paths[0])
	}
	if err != nil {
		return nil, nil, err
	}
	return FileEditProposalSourceView{ProtocolVersion: source.ProtocolVersion,
		RunID: source.RunID, WorkspaceID: source.WorkspaceID, Path: source.Path,
		Content: source.Content, ContentSHA256: source.ContentSHA256,
		SourceHandle: source.Handle, ExpiresAt: source.ExpiresAt,
		Editable: source.Editable, FileWrite: false}, nil, nil
}

func (a *API) runFileEditProposalRecovery(request *http.Request, runID string,
	editID string,
) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	if !a.fileEditProposalEnabled || a.fileEditProposalController == nil {
		return nil, nil, apperror.New(apperror.CodeNotFound,
			"file edit proposal recovery is unavailable")
	}
	recovery, err := a.fileEditProposalController.Recover(request.Context(), runID, editID)
	if err != nil {
		return nil, nil, err
	}
	return FileEditProposalRecoveryView{
		ProtocolVersion: recovery.ProtocolVersion, RunID: recovery.RunID,
		WorkspaceID: recovery.WorkspaceID, EditID: recovery.EditID, Path: recovery.Path,
		OriginalContent: recovery.OriginalContent, ProposedContent: recovery.ProposedContent,
		OriginalSHA256: recovery.OriginalSHA256, ProposedSHA256: recovery.ProposedSHA256,
		CurrentContentSHA256: recovery.CurrentContentHash, Status: recovery.Status,
		Stale: recovery.Stale, ReviewRequired: recovery.ReviewRequired,
		Editable: recovery.Editable, FileWrite: false,
	}, nil, nil
}

func (a *API) providerCredentialStatuses(request *http.Request) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	if !a.providerCredentialEnabled || a.providerCredentialController == nil {
		return nil, nil, apperror.New(apperror.CodeNotFound,
			"Provider credential status is unavailable")
	}
	statuses, err := a.providerCredentialController.List(request.Context())
	if err != nil {
		return nil, nil, err
	}
	items := make([]ProviderCredentialStatusView, len(statuses))
	for index, status := range statuses {
		items[index] = providerCredentialStatusView(status)
	}
	return ProviderCredentialListView{ProtocolVersion: credential.ProtocolVersion,
		Items: items}, nil, nil
}

func providerCredentialStatusView(value application.ProviderCredentialStatus) ProviderCredentialStatusView {
	return ProviderCredentialStatusView{ProtocolVersion: value.ProtocolVersion,
		Provider: value.Provider, Configured: value.Configured,
		StoreKind: value.StoreKind, StoreAvailable: value.StoreAvailable,
		PlaintextReturned: value.PlaintextReturned,
		RestartRequired:   value.RestartRequired, RegistryReloaded: value.RegistryReloaded,
		RegistryGeneration: value.RegistryGeneration}
}
