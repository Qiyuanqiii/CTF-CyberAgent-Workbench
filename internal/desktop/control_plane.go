package desktop

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/credential"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/modelregistry"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
)

// ControlPlane owns the Desktop process' SQLite connection and in-process API.
// It does not listen on a socket and it adds no renderer authority beyond the
// tokens explicitly supplied in ControlPlaneConfig.
type ControlPlane struct {
	stateStore     *store.SQLiteStore
	handler        http.Handler
	closeOnce      sync.Once
	closeErr       error
	skillInstaller *application.SkillPackageRegistryService
	wakeWorker     *application.RunWakeWorker
	workerMu       sync.Mutex
	workerCancel   context.CancelFunc
	workerDone     chan struct{}
	closed         bool
}

type ControlPlaneConfig struct {
	DatabasePath                  string
	HomePath                      string
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
	AppVersion                    string
	UIHandler                     http.Handler
	CredentialStore               credential.Store
	OnWakeWorkerError             func(error)
}

func OpenControlPlane(config ControlPlaneConfig) (*ControlPlane, error) {
	if strings.TrimSpace(config.DatabasePath) == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop database path is required")
	}
	stateStore, err := store.Open(config.DatabasePath)
	if err != nil {
		return nil, err
	}
	credentialStore := config.CredentialStore
	if credentialStore == nil {
		credentialStore = credential.NewSystemStore()
	}
	models := modelregistry.NewFromEnvironment()
	if credentialStore.Available() {
		models, err = modelregistry.NewFromEnvironmentWithCredentials(credentialStore)
		if err != nil {
			_ = stateStore.Close()
			return nil, err
		}
	}
	if err := models.LoadRouteSettings(context.Background(), stateStore); err != nil {
		_ = stateStore.Close()
		return nil, err
	}
	checker := policy.NewDefaultChecker()
	lifecycleControl := application.NewRunLifecycleControlService(stateStore)
	executionControl := application.NewRunExecutionHandoffService(stateStore,
		models.Router(), checker).WithActiveCalls(
		application.NewActiveCallRegistry())
	planDeliveryControl := application.NewPlanDeliveryControlService(stateStore)
	approvalControl := application.NewApprovalControlService(stateStore,
		toolgateway.New(stateStore, checker), checker)
	modelControl := application.NewModelControlService(models, stateStore)
	providerCredentialControl := application.NewProviderCredentialService(credentialStore).
		WithRegistryReload(models, stateStore)
	fileEditReview := application.NewFileEditReviewService(stateStore)
	fileEditProposal := application.NewFileEditProposalService(stateStore, checker)
	fileEditApply := application.NewFileEditApplyService(stateStore, checker)
	runWakeControl := application.NewRunWakeControlService(stateStore)
	runWakeExecution := application.NewForegroundRunWakeConsumer(stateStore,
		executionControl)
	var wakeWorker *application.RunWakeWorker
	if config.RunWakeWorkerEnabled {
		wakeWorker, err = application.NewRunWakeWorker(
			application.NewRunWakeCoordinator(stateStore), runWakeExecution,
			application.RunWakeWorkerConfig{OnError: config.OnWakeWorkerError})
		if err != nil {
			_ = stateStore.Close()
			return nil, err
		}
	}
	var workerHealth httpapi.RunWakeWorkerHealthSource
	if wakeWorker != nil {
		workerHealth = wakeWorker
	}
	var skillInstaller *application.SkillPackageRegistryService
	if config.SkillInstallationEnabled {
		home := strings.TrimSpace(config.HomePath)
		if home == "" {
			home = filepath.Dir(config.DatabasePath)
		}
		objects, objectErr := skills.NewLocalPackageObjectStore(home)
		if objectErr != nil {
			_ = stateStore.Close()
			return nil, objectErr
		}
		registry, registryErr := skills.BuiltinRegistry()
		if registryErr != nil {
			_ = stateStore.Close()
			return nil, registryErr
		}
		skillInstaller = application.NewSkillPackageRegistryService(stateStore,
			objects, registry)
	}
	api, err := httpapi.New(stateStore, httpapi.Config{
		AccessToken: config.ReadToken, ControlToken: config.ControlToken,
		RunControlEnabled:             config.RunControlEnabled,
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
		SkillInstallationEnabled:      config.SkillInstallationEnabled,
		EvidenceAttachmentEnabled:     config.EvidenceAttachmentEnabled,
		VerificationEvidenceEnabled:   config.VerificationEvidenceEnabled,
		RunLifecycleController:        lifecycleControl,
		RunExecutionController:        executionControl,
		PlanDeliveryController:        planDeliveryControl,
		ApprovalController:            approvalControl,
		ModelControlController:        modelControl,
		ProviderCredentialController:  providerCredentialControl,
		FileEditReviewController:      fileEditReview,
		FileEditProposalController:    fileEditProposal,
		RunWakeController:             runWakeControl,
		FileEditApplyController:       fileEditApply,
		RunWakeExecutionController:    runWakeExecution,
		RunWakeWorkerHealthSource:     workerHealth,
		SkillInstallationController:   skillInstaller,
		ModelRegistry:                 models,
		AppVersion:                    config.AppVersion, UIHandler: config.UIHandler,
	})
	if err != nil {
		_ = stateStore.Close()
		return nil, err
	}
	return &ControlPlane{stateStore: stateStore, handler: api.Handler(),
		skillInstaller: skillInstaller, wakeWorker: wakeWorker}, nil
}

func (c *ControlPlane) Handler() http.Handler {
	if c == nil {
		return nil
	}
	return c.handler
}

func (c *ControlPlane) SkillInstaller() SkillPackageInstaller {
	if c == nil {
		return nil
	}
	return c.skillInstaller
}

// ResolveWorkspace keeps the registered root inside the Go control plane. The
// renderer selects only an opaque Workspace ID and never receives RootPath.
func (c *ControlPlane) ResolveWorkspace(ctx context.Context,
	workspaceID string) (WorkspaceOpenTarget, error) {
	if c == nil || c.stateStore == nil {
		return WorkspaceOpenTarget{}, apperror.New(apperror.CodeFailedPrecondition,
			"desktop workspace resolver is unavailable")
	}
	if ctx == nil || !validWorkspaceIdentity(workspaceID) {
		return WorkspaceOpenTarget{}, apperror.New(apperror.CodeInvalidArgument,
			"desktop workspace identifier is invalid")
	}
	record, err := c.stateStore.GetWorkspaceByID(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceOpenTarget{}, apperror.New(apperror.CodeNotFound,
			"desktop workspace was not found")
	}
	if err != nil {
		return WorkspaceOpenTarget{}, apperror.New(apperror.CodeUnavailable,
			"desktop workspace lookup failed")
	}
	return WorkspaceOpenTarget{
		ID: record.ID, Name: record.Name, RootPath: filepath.Clean(record.RootPath),
	}, nil
}

func (c *ControlPlane) StartWakeWorker(parent context.Context) error {
	if c == nil {
		return errors.New("desktop control plane is unavailable")
	}
	if parent == nil {
		return errors.New("desktop wake worker context is required")
	}
	c.workerMu.Lock()
	defer c.workerMu.Unlock()
	if c.closed {
		return errors.New("desktop control plane is closed")
	}
	if c.wakeWorker == nil {
		return nil
	}
	if c.workerDone != nil {
		return errors.New("desktop wake worker is already started")
	}
	ctx, cancel := context.WithCancel(parent)
	c.workerCancel = cancel
	c.workerDone = make(chan struct{})
	go func(done chan struct{}) {
		defer close(done)
		_ = c.wakeWorker.Run(ctx)
	}(c.workerDone)
	return nil
}

func (c *ControlPlane) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.workerMu.Lock()
		c.closed = true
		cancel, done := c.workerCancel, c.workerDone
		c.workerCancel = nil
		c.workerDone = nil
		c.workerMu.Unlock()
		if cancel != nil {
			cancel()
		}
		if done != nil {
			<-done
		}
		if c.stateStore == nil {
			c.closeErr = errors.New("desktop control plane store is unavailable")
			return
		}
		c.closeErr = c.stateStore.Close()
	})
	return c.closeErr
}
