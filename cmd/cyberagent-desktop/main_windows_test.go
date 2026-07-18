//go:build windows && desktop && wv2runtime.error

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/desktop"

	"github.com/wailsapp/wails/v2/pkg/options"
)

type testWindowRestorer struct {
	unminimised int
	shown       int
}

func (r *testWindowRestorer) Unminimise(context.Context) { r.unminimised++ }
func (r *testWindowRestorer) Show(context.Context)       { r.shown++ }

func TestDesktopOptionsDefaultToReadOnlyAndRequireExplicitCapabilities(t *testing.T) {
	defaults, err := parseDesktopOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if defaults != (desktopOptions{}) {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}
	for _, current := range []struct {
		flag string
		want desktopOptions
	}{
		{flag: "--enable-profile-control", want: desktopOptions{profileControl: true}},
		{flag: "--enable-run-creation", want: desktopOptions{runCreation: true}},
		{flag: "--enable-session-messages", want: desktopOptions{sessionMessages: true}},
		{flag: "--enable-session-steering-control", want: desktopOptions{sessionSteeringControl: true}},
		{flag: "--enable-run-lifecycle", want: desktopOptions{runLifecycle: true}},
		{flag: "--enable-run-execution", want: desktopOptions{runExecution: true}},
		{flag: "--enable-plan-delivery", want: desktopOptions{planDeliveryControl: true}},
		{flag: "--enable-approvals", want: desktopOptions{approvalControl: true}},
		{flag: "--enable-model-control", want: desktopOptions{modelControl: true}},
		{flag: "--enable-file-edit-review", want: desktopOptions{fileEditReview: true}},
		{flag: "--enable-run-wake", want: desktopOptions{runWakeControl: true}},
		{flag: "--enable-file-edit-apply", want: desktopOptions{fileEditApply: true}},
		{flag: "--enable-run-wake-execution", want: desktopOptions{runWakeExecution: true}},
		{flag: "--enable-skill-installation", want: desktopOptions{skillInstallation: true}},
	} {
		parsed, err := parseDesktopOptions([]string{current.flag})
		if err != nil {
			t.Fatal(err)
		}
		if parsed != current.want {
			t.Fatalf("%s was not independently explicit: %#v", current.flag, parsed)
		}
	}
	if _, err := parseDesktopOptions([]string{"unexpected"}); err == nil {
		t.Fatal("desktop positional argument was accepted")
	}
}

func TestDesktopStartupFailureMessageIsBoundedAndPathFree(t *testing.T) {
	private := apperror.Wrap(apperror.CodeFailedPrecondition, "database validation failed",
		errors.New(`C:\PRIVATE\cyberagent.db`))
	message := desktopStartupFailureMessage(private)
	if !strings.Contains(message, string(apperror.CodeFailedPrecondition)) ||
		strings.Contains(message, "PRIVATE") || strings.Contains(message, "cyberagent.db") ||
		len(message) > 256 {
		t.Fatalf("unsafe startup failure message: %q", message)
	}
}

func TestWebView2PrerequisiteFailsClosedWithoutStartingAnInstaller(t *testing.T) {
	tests := []struct {
		name    string
		detect  func(string) (string, error)
		compare func(string, string) (int, error)
	}{
		{name: "missing", detect: func(string) (string, error) { return "", nil },
			compare: func(string, string) (int, error) { return 0, nil }},
		{name: "probe error", detect: func(string) (string, error) { return "", errors.New(`C:\PRIVATE`) },
			compare: func(string, string) (int, error) { return 0, nil }},
		{name: "old", detect: func(string) (string, error) { return "93.0.1.0", nil },
			compare: func(string, string) (int, error) { return -1, nil }},
		{name: "invalid", detect: func(string) (string, error) { return "invalid", nil },
			compare: func(string, string) (int, error) { return 0, errors.New("invalid version") }},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			err := requireWebView2Runtime(webView2RuntimeProbe{
				detect: current.detect, compare: current.compare,
			})
			if !errors.Is(err, errWebView2RuntimeRequired) ||
				apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
				t.Fatalf("prerequisite error = %v", err)
			}
			message := desktopStartupFailureMessage(err)
			if !strings.Contains(message, minimumWebView2RuntimeVersion) ||
				strings.Contains(message, "PRIVATE") || strings.Contains(strings.ToLower(message), "http") ||
				len(message) > 320 {
				t.Fatalf("unsafe WebView2 message: %q", message)
			}
		})
	}

	if err := requireWebView2Runtime(webView2RuntimeProbe{
		detect:  func(string) (string, error) { return "120.0.1.2", nil },
		compare: func(string, string) (int, error) { return 1, nil },
	}); err != nil {
		t.Fatalf("current WebView2 runtime was rejected: %v", err)
	}

	messages := desktopWebView2Messages()
	all := strings.ToLower(strings.Join([]string{
		messages.InstallationRequired, messages.UpdateRequired, messages.MissingRequirements,
		messages.Webview2NotInstalled, messages.Error, messages.FailedToInstall,
		messages.DownloadPage, messages.PressOKToInstall, messages.ContactAdmin,
		messages.InvalidFixedWebview2, messages.WebView2ProcessCrash,
	}, " "))
	if strings.Contains(all, "http://") || strings.Contains(all, "https://") ||
		strings.Contains(all, "silently") || strings.Contains(all, "press ok") ||
		messages.DownloadPage != "" || messages.PressOKToInstall != "" {
		t.Fatalf("WebView2 messages can trigger or direct an implicit installer: %q", all)
	}
}

func TestSecondInstanceHandlerIgnoresArgumentsAndRecoversAfterStartup(t *testing.T) {
	restorer := &testWindowRestorer{}
	lifecycle := desktop.NewLifecycle(restorer)
	handler := secondInstanceHandler(lifecycle)
	handler(options.SecondInstanceData{
		Args: []string{"--secret", `C:\PRIVATE\workspace`}, WorkingDirectory: `C:\PRIVATE`,
	})
	if restorer.unminimised != 0 || restorer.shown != 0 {
		t.Fatal("second instance restored the window before startup")
	}
	lifecycle.Start(context.Background())
	if restorer.unminimised != 1 || restorer.shown != 1 {
		t.Fatalf("second instance recovery count = %d/%d", restorer.unminimised, restorer.shown)
	}
	lifecycle.Stop()
}

func TestInProcessAPIHandlerPinsLoopbackBoundaryWithoutMutatingRequest(t *testing.T) {
	var receivedHost string
	var receivedRemote string
	var receivedPath string
	var receivedScheme string
	var receivedURLHost string
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedHost = request.Host
		receivedRemote = request.RemoteAddr
		receivedPath = request.URL.Path
		receivedScheme = request.URL.Scheme
		receivedURLHost = request.URL.Host
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := inProcessAPIHandler{next: next}
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/v1/health", nil)
	request.Host = "wails.localhost"
	request.RemoteAddr = "203.0.113.10:443"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || receivedHost != "127.0.0.1" ||
		receivedRemote != "127.0.0.1:0" || receivedPath != "/api/v1/health" ||
		receivedScheme != "" || receivedURLHost != "" {
		t.Fatalf("unexpected in-process projection: status=%d host=%q remote=%q path=%q",
			response.Code, receivedHost, receivedRemote, receivedPath)
	}
	if request.Host != "wails.localhost" || request.RemoteAddr != "203.0.113.10:443" ||
		request.URL.Scheme != "http" || request.URL.Host != "wails.localhost" {
		t.Fatalf("original request was mutated: host=%q remote=%q", request.Host, request.RemoteAddr)
	}
}

func TestInProcessAPIHandlerRejectsNonRendererOrigins(t *testing.T) {
	for _, target := range []string{
		"https://wails.localhost/api/v1/health",
		"http://wails.localhost:80/api/v1/health",
		"http://user@wails.localhost/api/v1/health",
		"http://untrusted.example/api/v1/health",
	} {
		t.Run(target, func(t *testing.T) {
			called := false
			handler := inProcessAPIHandler{next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			})}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
			if response.Code != http.StatusForbidden || called {
				t.Fatalf("origin %q reached API: status=%d called=%t", target, response.Code, called)
			}
		})
	}
}

func TestInProcessAPIHandlerRejectsNonCanonicalRendererURL(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "fragment", mutate: func(request *http.Request) { request.URL.Fragment = "fragment" }},
		{name: "opaque", mutate: func(request *http.Request) {
			request.URL.Opaque = "//wails.localhost/api/v1/health"
		}},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			called := false
			handler := inProcessAPIHandler{next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			})}
			request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/v1/health", nil)
			current.mutate(request)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || called {
				t.Fatalf("non-canonical renderer URL reached API: status=%d called=%t",
					response.Code, called)
			}
		})
	}
}

func TestInProcessAPIHandlerFailsClosedWithoutTarget(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/", nil)
	response := httptest.NewRecorder()
	inProcessAPIHandler{}.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestInProcessAPIHandlerFailsClosedWithoutURL(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/", nil)
	request.URL = nil
	response := httptest.NewRecorder()
	inProcessAPIHandler{next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("nil URL request reached the API")
	})}.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestInProcessAPIHandlerRebuildsMissingRequestURIFromURL(t *testing.T) {
	var requestURI string
	handler := inProcessAPIHandler{next: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestURI = request.RequestURI
		writer.WriteHeader(http.StatusNoContent)
	})}
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/v1/health?probe=one", nil)
	request.RequestURI = ""
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || requestURI != "/api/v1/health?probe=one" {
		t.Fatalf("status=%d request_uri=%q", response.Code, requestURI)
	}
	if request.RequestURI != "" {
		t.Fatal("source request URI was mutated")
	}
}

func TestInProcessAPIHandlerCanonicalizesMismatchedRequestURI(t *testing.T) {
	var requestURI string
	handler := inProcessAPIHandler{next: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestURI = request.RequestURI
		writer.WriteHeader(http.StatusNoContent)
	})}
	request := httptest.NewRequest(http.MethodGet,
		"http://wails.localhost/api/v1/health?probe=one", nil)
	request.RequestURI = "http://untrusted.example/private?secret=true"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || requestURI != "/api/v1/health?probe=one" {
		t.Fatalf("status=%d request_uri=%q", response.Code, requestURI)
	}
	if request.RequestURI != "http://untrusted.example/private?secret=true" {
		t.Fatal("source request URI was mutated")
	}
}

func TestInProcessAPIHandlerCanonicalizesOnlyTheWailsEmptyRoot(t *testing.T) {
	var path string
	var requestURI string
	var contentLength int64
	handler := inProcessAPIHandler{next: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		requestURI = request.RequestURI
		contentLength = request.ContentLength
		writer.WriteHeader(http.StatusNoContent)
	})}
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/", nil)
	request.URL.Path = ""
	request.RequestURI = ""
	request.ContentLength = -1
	request.Body = http.NoBody
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || path != "/" || requestURI != "/" || contentLength != 0 {
		t.Fatalf("status=%d path=%q request_uri=%q content_length=%d",
			response.Code, path, requestURI, contentLength)
	}
	if request.URL.Path != "" || request.RequestURI != "" || request.ContentLength != -1 {
		t.Fatal("Wails source request was mutated")
	}
}

func TestInProcessAPIHandlerDoesNotEraseUnknownRequestBodies(t *testing.T) {
	var contentLength int64
	handler := inProcessAPIHandler{next: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		contentLength = request.ContentLength
		writer.WriteHeader(http.StatusNoContent)
	})}
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/v1/health", nil)
	request.ContentLength = -1
	request.Body = io.NopCloser(strings.NewReader("unexpected"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || contentLength != -1 {
		t.Fatalf("status=%d content_length=%d", response.Code, contentLength)
	}
}
