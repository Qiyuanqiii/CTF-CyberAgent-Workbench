package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/webui"
)

const apiTokenEnvironment = "CYBERAGENT_API_TOKEN"

const apiControlTokenEnvironment = "CYBERAGENT_API_CONTROL_TOKEN"

func (a *App) apiCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("API subcommand is required")
	}
	switch args[0] {
	case "serve":
		return a.apiServeCommand(ctx, args[1:])
	case "openapi":
		return a.apiOpenAPICommand(args[1:])
	default:
		return fmt.Errorf("unknown API subcommand %q", args[0])
	}
}

func (a *App) apiServeCommand(ctx context.Context, args []string) error {
	fs := newFlagSet("api serve", a.errOut)
	listenAddress := fs.String("listen", httpapi.DefaultListenAddress, "loopback listen address")
	uiDirectory := fs.String("ui-dir", "", "optional built Web UI directory")
	fileEditProposals := fs.Bool("enable-file-edit-proposals", false,
		"enable Go-issued interactive FileEdit proposal sources")
	providerCredentials := fs.Bool("enable-provider-credentials", false,
		"enable OS-owned Provider credential changes")
	wakeWorker := fs.Bool("enable-wake-worker", false,
		"enable the bounded single-owner Run wake worker")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"listen": true, "ui-dir": true,
		"enable-file-edit-proposals": false, "enable-provider-credentials": false,
		"enable-wake-worker": false})); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: cyberagent api serve [--listen <loopback-host:port>] [--ui-dir <built-web-directory>] [explicit capability flags]")
	}

	accessToken := os.Getenv(apiTokenEnvironment)
	controlToken := os.Getenv(apiControlTokenEnvironment)
	if (*fileEditProposals || *providerCredentials || *wakeWorker) && controlToken == "" {
		return errors.New("interactive proposals, Provider credentials, and the wake worker require CYBERAGENT_API_CONTROL_TOKEN")
	}
	generated := accessToken == ""
	if generated {
		var err error
		accessToken, err = httpapi.GenerateAccessToken()
		if err != nil {
			return err
		}
	}
	var uiBundle *webui.Bundle
	if strings.TrimSpace(*uiDirectory) != "" {
		var err error
		uiBundle, err = webui.LoadDirectory(*uiDirectory)
		if err != nil {
			return apperror.Wrap(apperror.CodeInvalidArgument, "invalid Web UI directory", err)
		}
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	lifecycleControl := application.NewRunLifecycleControlService(a.store)
	executionControl := application.NewRunExecutionHandoffService(a.store, a.router,
		a.checker).WithActiveCalls(a.calls)
	planDeliveryControl := application.NewPlanDeliveryControlService(a.store)
	approvalControl := application.NewApprovalControlService(a.store,
		a.newToolGateway(), a.checker)
	modelControl := application.NewModelControlService(a.models, a.store)
	providerCredentialControl := application.NewProviderCredentialService(a.credentials).
		WithRegistryReload(a.models, a.store)
	fileEditReview := application.NewFileEditReviewService(a.store)
	fileEditProposal := application.NewFileEditProposalService(a.store, a.checker)
	fileEditApply := application.NewFileEditApplyService(a.store, a.checker)
	runWakeControl := application.NewRunWakeControlService(a.store)
	runWakeExecution := application.NewForegroundRunWakeConsumer(a.store,
		executionControl)
	var worker *application.RunWakeWorker
	if *wakeWorker {
		createdWorker, workerErr := application.NewRunWakeWorker(
			application.NewRunWakeCoordinator(a.store), runWakeExecution,
			application.RunWakeWorkerConfig{OnError: func(runErr error) {
				fmt.Fprintln(a.errOut, "wake-worker:", runErr)
			}})
		if workerErr != nil {
			return workerErr
		}
		worker = createdWorker
	}
	var workerHealth httpapi.RunWakeWorkerHealthSource
	if worker != nil {
		workerHealth = worker
	}
	builtinSkills, err := skills.BuiltinRegistry()
	if err != nil {
		return err
	}
	skillObjects, err := skills.NewLocalPackageObjectStore(a.home)
	if err != nil {
		return err
	}
	skillInstallation := application.NewSkillPackageRegistryService(a.store,
		skillObjects, builtinSkills)
	api, err := httpapi.New(a.store, httpapi.Config{
		AccessToken: accessToken, ControlToken: controlToken,
		RunControlEnabled: controlToken != "", RunCreationEnabled: controlToken != "",
		SessionMessageEnabled:         controlToken != "",
		SessionSteeringControlEnabled: controlToken != "",
		RunLifecycleEnabled:           controlToken != "",
		RunExecutionEnabled:           controlToken != "",
		PlanDeliveryControlEnabled:    controlToken != "",
		ApprovalControlEnabled:        controlToken != "",
		ModelControlEnabled:           controlToken != "",
		ProviderCredentialEnabled:     *providerCredentials,
		FileEditReviewEnabled:         controlToken != "",
		FileEditProposalEnabled:       *fileEditProposals,
		RunWakeControlEnabled:         controlToken != "",
		FileEditApplyEnabled:          controlToken != "",
		RunWakeExecutionEnabled:       controlToken != "",
		RunWakeWorkerEnabled:          *wakeWorker,
		SkillInstallationEnabled:      controlToken != "",
		EvidenceAttachmentEnabled:     controlToken != "",
		VerificationEvidenceEnabled:   controlToken != "",
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
		SkillInstallationController:   skillInstallation,
		ModelRegistry:                 a.models,
		AppVersion:                    Version,
		UIHandler:                     uiBundle,
	})
	if err != nil {
		return err
	}
	listener, err := httpapi.ListenLoopback(ctx, *listenAddress)
	if err != nil {
		return err
	}
	server, err := httpapi.NewServer(api, log.New(a.errOut, "api: ", log.LstdFlags))
	if err != nil {
		_ = listener.Close()
		return err
	}
	origin := "http://" + listener.Addr().String()
	baseURL := origin + "/api/v1"
	fmt.Fprintf(a.out, "api_url: %s\napi_version: %s\napi_token_generated: %t\napi_control_enabled: %t\n",
		baseURL, httpapi.Version, generated, controlToken != "")
	if uiBundle != nil {
		fmt.Fprintf(a.out, "ui_url: %s/\nui_source: %s\nui_assets: %d\nui_digest: %s\n",
			origin, uiBundle.Source(), uiBundle.AssetCount(), uiBundle.Digest())
	}
	if generated {
		fmt.Fprintf(a.out, "api_token: %s\n", accessToken)
	} else {
		fmt.Fprintf(a.out, "api_token_source: %s\n", apiTokenEnvironment)
	}
	if controlToken != "" {
		fmt.Fprintf(a.out, "api_control_token_source: %s\n", apiControlTokenEnvironment)
	}
	var workerCancel context.CancelFunc
	var workerDone chan struct{}
	if worker != nil {
		workerCtx, cancel := context.WithCancel(ctx)
		workerCancel = cancel
		workerDone = make(chan struct{})
		go func() {
			defer close(workerDone)
			_ = worker.Run(workerCtx)
		}()
	}
	if workerCancel != nil {
		defer func() {
			workerCancel()
			<-workerDone
		}()
	}
	fmt.Fprintf(a.out, "file_edit_proposals_enabled: %t\nprovider_credentials_enabled: %t\nwake_worker_enabled: %t\nwake_worker_concurrency: %d\nwake_worker_max_steps: %d\n",
		*fileEditProposals, *providerCredentials, *wakeWorker,
		application.RunWakeWorkerConcurrency, application.RunWakeWorkerMaxSteps)
	fmt.Fprintln(a.out, "note: the API is loopback-only; control is separately authorized and tokens are not persisted")
	return server.Serve(ctx, listener)
}

func (a *App) apiOpenAPICommand(args []string) error {
	fs := newFlagSet("api openapi", a.errOut)
	output := fs.String("output", "", "optional output file")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"output": true})); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: cyberagent api openapi [--output <path>]")
	}
	document, err := httpapi.GenerateOpenAPI()
	if err != nil {
		return err
	}
	if strings.TrimSpace(*output) == "" {
		_, err = a.out.Write(document)
		return err
	}
	path := filepath.Clean(strings.TrimSpace(*output))
	if err := os.WriteFile(path, document, 0o644); err != nil {
		return fmt.Errorf("write OpenAPI document: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	fmt.Fprintf(a.out, "openapi_written: %s\n", absolute)
	return nil
}
