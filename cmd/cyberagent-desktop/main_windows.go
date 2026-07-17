//go:build windows && desktop

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"cyberagent-workbench/internal/app"
	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/desktop"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/webui"
	webassets "cyberagent-workbench/web"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	syswindows "golang.org/x/sys/windows"
)

const desktopSingleInstanceID = "e3305a58-3d1e-4e2f-b4ca-d1032a737b96"

type desktopOptions struct {
	profileControl bool
	version        bool
}

type desktopLifecycle struct {
	mu  sync.RWMutex
	ctx context.Context
}

func (l *desktopLifecycle) set(ctx context.Context) {
	l.mu.Lock()
	l.ctx = ctx
	l.mu.Unlock()
}

func (l *desktopLifecycle) clear() {
	l.mu.Lock()
	l.ctx = nil
	l.mu.Unlock()
}

func (l *desktopLifecycle) current() context.Context {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.ctx
}

type nativeSkillPackagePicker struct{}

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
	trusted := request.Clone(request.Context())
	trusted.Host = "127.0.0.1"
	trusted.RemoteAddr = "127.0.0.1:0"
	if trusted.URL != nil && trusted.URL.Path == "" {
		trusted.URL.Path = "/"
	}
	if trusted.RequestURI == "" {
		trusted.RequestURI = trusted.URL.RequestURI()
		if trusted.RequestURI == "" {
			trusted.RequestURI = "/"
		}
	}
	if (trusted.Method == http.MethodGet || trusted.Method == http.MethodHead) &&
		trusted.ContentLength == -1 && len(trusted.TransferEncoding) == 0 && trusted.Body == http.NoBody &&
		trusted.Header.Get("Content-Length") == "" {
		trusted.ContentLength = 0
	}
	h.next.ServeHTTP(writer, trusted)
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
	code := apperror.CodeOf(apperror.Normalize(err))
	return "CyberAgent Workbench could not start.\r\n\r\nError code: " + string(code) +
		"\r\n\r\nLocal data was not deleted or reset. Keep it for diagnosis."
}

func parseDesktopOptions(args []string) (desktopOptions, error) {
	fs := flag.NewFlagSet("cyberagent-desktop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profileControl := fs.Bool("enable-profile-control", false,
		"enable only the non-authorizing Run execution-profile control")
	version := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return desktopOptions{}, err
	}
	if fs.NArg() != 0 {
		return desktopOptions{}, errors.New("cyberagent-desktop accepts no positional arguments")
	}
	return desktopOptions{profileControl: *profileControl, version: *version}, nil
}

func runDesktop(config desktopOptions) error {
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
	if config.profileControl {
		controlToken, err = httpapi.GenerateAccessToken()
		if err != nil {
			return err
		}
	}

	databasePath := filepath.Join(app.DefaultHome(), "cyberagent.db")
	stateStore, err := store.Open(databasePath)
	if err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"desktop data store validation failed", err)
	}
	defer stateStore.Close()
	api, err := httpapi.New(stateStore, httpapi.Config{
		AccessToken: readToken, ControlToken: controlToken, AppVersion: app.Version,
		UIHandler: bundle,
	})
	if err != nil {
		return err
	}

	lifecycle := &desktopLifecycle{}
	selector, preview := desktop.NewSkillPackagePreviewBoundary()
	bridge, err := desktop.NewDesktopBridge(desktop.DesktopBridgeConfig{
		ContextProvider: lifecycle.current, FilePicker: nativeSkillPackagePicker{},
		ReadToken: readToken, ControlToken: controlToken, APIVersion: httpapi.Version,
		AppVersion: app.Version, UIDigest: bundle.Digest(), Selector: selector, PreviewBridge: preview,
	})
	if err != nil {
		return err
	}

	return wails.Run(&options.App{
		Title: "CyberAgent Workbench", Width: 1440, Height: 900, MinWidth: 1024, MinHeight: 680,
		WindowStartState: options.Normal,
		BackgroundColour: options.NewRGB(245, 247, 249),
		AssetServer: &assetserver.Options{
			Handler: inProcessAPIHandler{next: api.Handler()},
		},
		OnStartup: lifecycle.set,
		OnShutdown: func(context.Context) {
			lifecycle.clear()
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
			UniqueId: desktopSingleInstanceID,
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if ctx := lifecycle.current(); ctx != nil {
					runtime.WindowUnminimise(ctx)
					runtime.WindowShow(ctx)
				}
			},
		},
		Windows: &windows.Options{
			Theme: windows.SystemDefault, WebviewIsTransparent: false, WindowIsTranslucent: false,
			DisablePinchZoom: true, IsZoomControlEnabled: true, EnableSwipeGestures: false,
			WebviewDisableRendererCodeIntegrity: false, WindowClassName: "CyberAgentWorkbench",
		},
		Debug: options.Debug{OpenInspectorOnStartup: false},
		ErrorFormatter: func(err error) any {
			normalized := apperror.Normalize(err)
			return desktopBindingError{Code: string(apperror.CodeOf(normalized)), Message: normalized.Error()}
		},
	})
}
