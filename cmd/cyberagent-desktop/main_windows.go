//go:build windows && desktop && wv2runtime.error

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cyberagent-workbench/internal/app"
	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/desktop"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/webui"
	webassets "cyberagent-workbench/web"

	"github.com/wailsapp/go-webview2/webviewloader"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	syswindows "golang.org/x/sys/windows"
)

const (
	desktopSingleInstanceID       = "e3305a58-3d1e-4e2f-b4ca-d1032a737b96"
	minimumWebView2RuntimeVersion = "94.0.992.31"
)

var errWebView2RuntimeRequired = errors.New("required WebView2 Runtime is unavailable")

type webView2RuntimeProbe struct {
	detect  func(string) (string, error)
	compare func(string, string) (int, error)
}

type desktopOptions struct {
	profileControl         bool
	runCreation            bool
	sessionMessages        bool
	sessionSteeringControl bool
	runLifecycle           bool
	runExecution           bool
	planDeliveryControl    bool
	approvalControl        bool
	modelControl           bool
	providerCredentials    bool
	fileEditReview         bool
	fileEditProposals      bool
	runWakeControl         bool
	fileEditApply          bool
	runWakeExecution       bool
	runWakeWorker          bool
	skillInstallation      bool
	evidenceAttachment     bool
	verificationEvidence   bool
	version                bool
}

type nativeSkillPackagePicker struct{}

type wailsWindowRestorer struct{}

func (wailsWindowRestorer) Unminimise(ctx context.Context) { runtime.WindowUnminimise(ctx) }
func (wailsWindowRestorer) Show(ctx context.Context)       { runtime.WindowShow(ctx) }

func secondInstanceHandler(lifecycle *desktop.Lifecycle) func(options.SecondInstanceData) {
	return func(_ options.SecondInstanceData) {
		lifecycle.RequestRestore()
	}
}

func (nativeSkillPackagePicker) OpenSkillPackage(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", errors.New("desktop lifecycle is unavailable")
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: "Select CyberAgent Skill package",
		Filters: []runtime.FileFilter{
			{DisplayName: "CyberAgent Skill package (*.zip)", Pattern: "*.zip"},
		},
		ShowHiddenFiles:      false,
		CanCreateDirectories: false,
		ResolvesAliases:      false,
	})
}

type inProcessAPIHandler struct {
	next http.Handler
}

func (h inProcessAPIHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if h.next == nil || request == nil || request.URL == nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if !trustedDesktopRendererOrigin(request) {
		http.Error(writer, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	trusted := request.Clone(request.Context())
	trusted.Host = "127.0.0.1"
	trusted.RemoteAddr = "127.0.0.1:0"
	trusted.URL.Scheme = ""
	trusted.URL.Host = ""
	if trusted.URL != nil && trusted.URL.Path == "" {
		trusted.URL.Path = "/"
	}
	trusted.RequestURI = trusted.URL.RequestURI()
	if trusted.RequestURI == "" {
		trusted.RequestURI = "/"
	}
	if (trusted.Method == http.MethodGet || trusted.Method == http.MethodHead) &&
		trusted.ContentLength == -1 && len(trusted.TransferEncoding) == 0 && trusted.Body == http.NoBody &&
		trusted.Header.Get("Content-Length") == "" {
		trusted.ContentLength = 0
	}
	h.next.ServeHTTP(writer, trusted)
}

func trustedDesktopRendererOrigin(request *http.Request) bool {
	if request == nil || request.URL == nil || request.URL.User != nil || request.URL.Fragment != "" ||
		request.URL.Opaque != "" {
		return false
	}
	return strings.EqualFold(request.URL.Scheme, "http") &&
		strings.EqualFold(request.URL.Hostname(), "wails.localhost") && request.URL.Port() == ""
}

type desktopBindingError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	config, err := parseDesktopOptions(os.Args[1:])
	if err != nil {
		reportDesktopStartupFailure(err)
		os.Exit(2)
	}
	if config.version {
		fmt.Fprintf(os.Stdout, "CyberAgent Workbench desktop %s\n", app.Version)
		return
	}
	if err := runDesktop(config); err != nil {
		reportDesktopStartupFailure(err)
		os.Exit(1)
	}
}

func reportDesktopStartupFailure(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	message, messageErr := syswindows.UTF16PtrFromString(desktopStartupFailureMessage(err))
	title, titleErr := syswindows.UTF16PtrFromString("CyberAgent Workbench")
	if messageErr != nil || titleErr != nil {
		return
	}
	_, _ = syswindows.MessageBox(0, message, title,
		syswindows.MB_OK|syswindows.MB_ICONERROR|syswindows.MB_SETFOREGROUND)
}

func desktopStartupFailureMessage(err error) string {
	if errors.Is(err, errWebView2RuntimeRequired) {
		return "CyberAgent Workbench requires Microsoft Edge WebView2 Runtime " +
			minimumWebView2RuntimeVersion + " or newer.\r\n\r\n" +
			"Install or update it through a trusted Windows software channel, then reopen the app.\r\n\r\n" +
			"No download or installation was started."
	}
	code := apperror.CodeOf(apperror.Normalize(err))
	return "CyberAgent Workbench could not start.\r\n\r\nError code: " + string(code) +
		"\r\n\r\nLocal data was not deleted or reset. Keep it for diagnosis."
}

func parseDesktopOptions(args []string) (desktopOptions, error) {
	fs := flag.NewFlagSet("cyberagent-desktop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profileControl := fs.Bool("enable-profile-control", false,
		"enable only the non-authorizing Run execution-profile control")
	runCreation := fs.Bool("enable-run-creation", false,
		"enable idempotent workspace-bound Run creation")
	sessionMessages := fs.Bool("enable-session-messages", false,
		"enable idempotent Run-bound Session message submission")
	sessionSteeringControl := fs.Bool("enable-session-steering-control", false,
		"enable pending-only Run-bound Session steering cancellation")
	runLifecycle := fs.Bool("enable-run-lifecycle", false,
		"enable idempotent Run start, pause, and resume control")
	runExecution := fs.Bool("enable-run-execution", false,
		"enable bounded queued Run execution through the Go Supervisor")
	planDeliveryControl := fs.Bool("enable-plan-delivery", false,
		"enable operator Plan direction selection and explicit Deliver transition")
	approvalControl := fs.Bool("enable-approvals", false,
		"enable bounded approve-once and deny decisions for durable approvals")
	modelControl := fs.Bool("enable-model-control", false,
		"enable persisted model route selection and explicit connectivity diagnostics")
	providerCredentials := fs.Bool("enable-provider-credentials", false,
		"enable OS-owned Provider credential changes")
	fileEditReview := fs.Bool("enable-file-edit-review", false,
		"enable review-only file edit approval or denial without applying files")
	fileEditProposals := fs.Bool("enable-file-edit-proposals", false,
		"enable Go-issued interactive FileEdit proposal sources")
	runWakeControl := fs.Bool("enable-run-wake", false,
		"enable durable bounded Run wake intent scheduling and cancellation")
	fileEditApply := fs.Bool("enable-file-edit-apply", false,
		"enable independently authorized approved FileEdit application")
	runWakeExecution := fs.Bool("enable-run-wake-execution", false,
		"enable explicitly launched foreground wake execution")
	runWakeWorker := fs.Bool("enable-wake-worker", false,
		"enable the bounded single-owner Run wake worker")
	skillInstallation := fs.Bool("enable-skill-installation", false,
		"enable confirmed inert Skill package installation")
	evidenceAttachment := fs.Bool("enable-evidence-attachments", false,
		"enable idempotent non-authorizing Workspace evidence attachment")
	verificationEvidence := fs.Bool("enable-verification-evidence", false,
		"enable immutable operator verification evidence recording")
	version := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return desktopOptions{}, err
	}
	if fs.NArg() != 0 {
		return desktopOptions{}, errors.New("cyberagent-desktop accepts no positional arguments")
	}
	return desktopOptions{profileControl: *profileControl, runCreation: *runCreation,
		sessionMessages:        *sessionMessages,
		sessionSteeringControl: *sessionSteeringControl,
		runLifecycle:           *runLifecycle,
		runExecution:           *runExecution,
		planDeliveryControl:    *planDeliveryControl,
		approvalControl:        *approvalControl,
		modelControl:           *modelControl,
		providerCredentials:    *providerCredentials,
		fileEditReview:         *fileEditReview,
		fileEditProposals:      *fileEditProposals,
		runWakeControl:         *runWakeControl,
		fileEditApply:          *fileEditApply,
		runWakeExecution:       *runWakeExecution,
		runWakeWorker:          *runWakeWorker,
		skillInstallation:      *skillInstallation,
		evidenceAttachment:     *evidenceAttachment,
		verificationEvidence:   *verificationEvidence,
		version:                *version}, nil
}

func runDesktop(config desktopOptions) error {
	if err := requireWebView2Runtime(webView2RuntimeProbe{
		detect:  webviewloader.GetAvailableCoreWebView2BrowserVersionString,
		compare: webviewloader.CompareBrowserVersions,
	}); err != nil {
		return err
	}
	bundle, err := webui.LoadEmbeddedFS(webassets.Files, "dist")
	if err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"embedded Desktop UI validation failed", err)
	}
	readToken, err := httpapi.GenerateAccessToken()
	if err != nil {
		return err
	}
	controlToken := ""
	if config.profileControl || config.runCreation || config.sessionMessages ||
		config.sessionSteeringControl || config.runLifecycle || config.runExecution ||
		config.planDeliveryControl || config.approvalControl || config.modelControl ||
		config.providerCredentials || config.fileEditReview || config.fileEditProposals ||
		config.runWakeControl || config.fileEditApply || config.runWakeExecution ||
		config.runWakeWorker || config.skillInstallation || config.evidenceAttachment ||
		config.verificationEvidence {
		controlToken, err = httpapi.GenerateAccessToken()
		if err != nil {
			return err
		}
	}

	homePath := app.DefaultHome()
	databasePath := filepath.Join(homePath, "cyberagent.db")
	controlPlane, err := desktop.OpenControlPlane(desktop.ControlPlaneConfig{
		DatabasePath: databasePath, HomePath: homePath, ReadToken: readToken,
		ControlToken:      controlToken,
		RunControlEnabled: config.profileControl, RunCreationEnabled: config.runCreation,
		SessionMessageEnabled:         config.sessionMessages,
		SessionSteeringControlEnabled: config.sessionSteeringControl,
		RunLifecycleEnabled:           config.runLifecycle,
		RunExecutionEnabled:           config.runExecution,
		PlanDeliveryControlEnabled:    config.planDeliveryControl,
		ApprovalControlEnabled:        config.approvalControl,
		ModelControlEnabled:           config.modelControl,
		ProviderCredentialEnabled:     config.providerCredentials,
		FileEditReviewEnabled:         config.fileEditReview,
		FileEditProposalEnabled:       config.fileEditProposals,
		RunWakeControlEnabled:         config.runWakeControl,
		FileEditApplyEnabled:          config.fileEditApply,
		RunWakeExecutionEnabled:       config.runWakeExecution,
		RunWakeWorkerEnabled:          config.runWakeWorker,
		SkillInstallationEnabled:      config.skillInstallation,
		EvidenceAttachmentEnabled:     config.evidenceAttachment,
		VerificationEvidenceEnabled:   config.verificationEvidence,
		AppVersion:                    app.Version, UIHandler: bundle,
		OnWakeWorkerError: func(runErr error) {
			fmt.Fprintln(os.Stderr, "wake-worker:", runErr)
		},
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"desktop data store validation failed", err)
	}
	defer controlPlane.Close()
	if err := controlPlane.StartWakeWorker(context.Background()); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"desktop wake worker could not start", err)
	}

	lifecycle := desktop.NewLifecycle(wailsWindowRestorer{})
	selector, preview := desktop.NewSkillPackagePreviewBoundary()
	bridge, err := desktop.NewDesktopBridge(desktop.DesktopBridgeConfig{
		ContextProvider: lifecycle.Context, FilePicker: nativeSkillPackagePicker{},
		ReadToken: readToken, ControlToken: controlToken, APIVersion: httpapi.Version,
		RunControlEnabled: config.profileControl, RunCreationEnabled: config.runCreation,
		SessionMessageEnabled:         config.sessionMessages,
		SessionSteeringControlEnabled: config.sessionSteeringControl,
		RunLifecycleEnabled:           config.runLifecycle,
		RunExecutionEnabled:           config.runExecution,
		PlanDeliveryControlEnabled:    config.planDeliveryControl,
		ApprovalControlEnabled:        config.approvalControl,
		ModelControlEnabled:           config.modelControl,
		ProviderCredentialEnabled:     config.providerCredentials,
		FileEditReviewEnabled:         config.fileEditReview,
		FileEditProposalEnabled:       config.fileEditProposals,
		RunWakeControlEnabled:         config.runWakeControl,
		FileEditApplyEnabled:          config.fileEditApply,
		RunWakeExecutionEnabled:       config.runWakeExecution,
		RunWakeWorkerEnabled:          config.runWakeWorker,
		SkillInstallationEnabled:      config.skillInstallation,
		EvidenceAttachmentEnabled:     config.evidenceAttachment,
		VerificationEvidenceEnabled:   config.verificationEvidence,
		AppVersion:                    app.Version, UIDigest: bundle.Digest(), Selector: selector,
		PreviewBridge: preview, SkillInstaller: controlPlane.SkillInstaller(),
	})
	if err != nil {
		return err
	}

	return wails.Run(&options.App{
		Title: "CyberAgent Workbench", Width: 1440, Height: 900, MinWidth: 1024, MinHeight: 680,
		WindowStartState: options.Normal,
		BackgroundColour: options.NewRGB(245, 247, 249),
		AssetServer: &assetserver.Options{
			Handler: inProcessAPIHandler{next: controlPlane.Handler()},
		},
		OnStartup: lifecycle.Start,
		OnShutdown: func(context.Context) {
			lifecycle.Stop()
		},
		Bind:                     []interface{}{bridge},
		EnableDefaultContextMenu: false,
		// Keep OS anti-phishing cloud submission disabled for a local-first app.
		EnableFraudulentWebsiteDetection: false,
		BindingsAllowedOrigins:           "",
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: false, DisableWebViewDrop: true,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               desktopSingleInstanceID,
			OnSecondInstanceLaunch: secondInstanceHandler(lifecycle),
		},
		Windows: &windows.Options{
			Theme: windows.SystemDefault, WebviewIsTransparent: false, WindowIsTranslucent: false,
			DisablePinchZoom: true, IsZoomControlEnabled: true, EnableSwipeGestures: false,
			WebviewDisableRendererCodeIntegrity: false, WindowClassName: "CyberAgentWorkbench",
			Messages: desktopWebView2Messages(),
		},
		Debug: options.Debug{OpenInspectorOnStartup: false},
		ErrorFormatter: func(err error) any {
			normalized := apperror.Normalize(err)
			return desktopBindingError{Code: string(apperror.CodeOf(normalized)), Message: normalized.Error()}
		},
	})
}

func requireWebView2Runtime(probe webView2RuntimeProbe) error {
	if probe.detect == nil || probe.compare == nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"desktop WebView2 prerequisite check is unavailable", errWebView2RuntimeRequired)
	}
	version, err := probe.detect("")
	if err != nil || strings.TrimSpace(version) == "" {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"desktop WebView2 prerequisite is not satisfied", errWebView2RuntimeRequired)
	}
	comparison, err := probe.compare(version, minimumWebView2RuntimeVersion)
	if err != nil || comparison < 0 {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"desktop WebView2 prerequisite is not satisfied", errWebView2RuntimeRequired)
	}
	return nil
}

func desktopWebView2Messages() *windows.Messages {
	return &windows.Messages{
		InstallationRequired: "Microsoft Edge WebView2 Runtime is required. Use a trusted Windows software channel, then reopen CyberAgent Workbench.",
		UpdateRequired:       "Microsoft Edge WebView2 Runtime must be updated through a trusted Windows software channel before CyberAgent Workbench can start.",
		MissingRequirements:  "CyberAgent Workbench prerequisite",
		Webview2NotInstalled: "Microsoft Edge WebView2 Runtime is unavailable.",
		Error:                "CyberAgent Workbench prerequisite",
		FailedToInstall:      "Microsoft Edge WebView2 Runtime remains unavailable.",
		DownloadPage:         "",
		PressOKToInstall:     "",
		ContactAdmin:         "Microsoft Edge WebView2 Runtime is required. Contact your administrator or use a trusted Windows software channel.",
		InvalidFixedWebview2: "The configured WebView2 Runtime does not meet the required version.",
		WebView2ProcessCrash: "The WebView2 process stopped. Reopen CyberAgent Workbench; local data was not reset.",
	}
}
