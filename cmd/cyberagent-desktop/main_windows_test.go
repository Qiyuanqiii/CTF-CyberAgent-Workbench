//go:build windows && desktop

package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
)

func TestDesktopOptionsDefaultToReadOnlyAndRequireExplicitProfileControl(t *testing.T) {
	defaults, err := parseDesktopOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.profileControl || defaults.version {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}
	enabled, err := parseDesktopOptions([]string{"--enable-profile-control"})
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.profileControl || enabled.version {
		t.Fatalf("profile control was not explicit: %#v", enabled)
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

func TestInProcessAPIHandlerPinsLoopbackBoundaryWithoutMutatingRequest(t *testing.T) {
	var receivedHost string
	var receivedRemote string
	var receivedPath string
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedHost = request.Host
		receivedRemote = request.RemoteAddr
		receivedPath = request.URL.Path
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := inProcessAPIHandler{next: next}
	request := httptest.NewRequest(http.MethodGet, "https://untrusted.example/api/v1/health", nil)
	request.Host = "untrusted.example"
	request.RemoteAddr = "203.0.113.10:443"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || receivedHost != "127.0.0.1" ||
		receivedRemote != "127.0.0.1:0" || receivedPath != "/api/v1/health" {
		t.Fatalf("unexpected in-process projection: status=%d host=%q remote=%q path=%q",
			response.Code, receivedHost, receivedRemote, receivedPath)
	}
	if request.Host != "untrusted.example" || request.RemoteAddr != "203.0.113.10:443" {
		t.Fatalf("original request was mutated: host=%q remote=%q", request.Host, request.RemoteAddr)
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
