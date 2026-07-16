package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBundleLoadsImmutableSnapshotAndServesBoundedRoutes(t *testing.T) {
	directory := writeTestBundle(t)
	bundle, err := LoadDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.AssetCount() != 2 || len(bundle.Digest()) != 64 || bundle.Source() == "" {
		t.Fatalf("unexpected bundle metadata: source=%q assets=%d digest=%q",
			bundle.Source(), bundle.AssetCount(), bundle.Digest())
	}

	index := requestBundle(t, bundle, http.MethodGet, "/", "", "")
	if index.Code != http.StatusOK || index.Body.String() != testIndexBody ||
		index.Header().Get("Cache-Control") != "no-store" ||
		index.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		index.Header().Get("ETag") == "" {
		t.Fatalf("unexpected index response: status=%d headers=%#v body=%q",
			index.Code, index.Header(), index.Body.String())
	}

	assetPath := "/assets/index-AbCd1234.js"
	asset := requestBundle(t, bundle, http.MethodGet, assetPath, "", "")
	if asset.Code != http.StatusOK || asset.Body.String() != testScriptBody ||
		asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		asset.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected asset response: status=%d headers=%#v body=%q",
			asset.Code, asset.Header(), asset.Body.String())
	}
	head := requestBundle(t, bundle, http.MethodHead, assetPath, "", "")
	if head.Code != http.StatusOK || head.Body.Len() != 0 ||
		head.Header().Get("Content-Length") != asset.Header().Get("Content-Length") {
		t.Fatalf("unexpected HEAD response: status=%d headers=%#v body=%q",
			head.Code, head.Header(), head.Body.String())
	}
	notModified := requestBundle(t, bundle, http.MethodGet, assetPath, "", asset.Header().Get("ETag"))
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("conditional asset response: status=%d body=%q", notModified.Code, notModified.Body.String())
	}

	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchanged := requestBundle(t, bundle, http.MethodGet, "/", "", "")
	if unchanged.Body.String() != testIndexBody {
		t.Fatalf("loaded bundle changed after disk mutation: %q", unchanged.Body.String())
	}
}

func TestBundleSPAFallbackAndUnknownAssetsFailClosed(t *testing.T) {
	bundle, err := LoadDirectory(writeTestBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		method string
		path   string
		accept string
		status int
	}{
		{name: "navigation", method: http.MethodGet, path: "/runs/run-1", accept: "text/html", status: http.StatusOK},
		{name: "xhtml", method: http.MethodGet, path: "/sessions/one", accept: "application/xhtml+xml", status: http.StatusOK},
		{name: "zero quality", method: http.MethodGet, path: "/runs/run-1", accept: "text/html;q=0.0", status: http.StatusNotFound},
		{name: "no accept", method: http.MethodGet, path: "/runs/run-1", status: http.StatusNotFound},
		{name: "missing asset", method: http.MethodGet, path: "/assets/missing-AbCd1234.js", accept: "text/html", status: http.StatusNotFound},
		{name: "extension", method: http.MethodGet, path: "/secrets.txt", accept: "text/html", status: http.StatusNotFound},
		{name: "dot segment", method: http.MethodGet, path: "/.hidden", accept: "text/html", status: http.StatusNotFound},
		{name: "deep route", method: http.MethodGet, path: "/1/2/3/4/5/6/7/8/9", accept: "text/html", status: http.StatusNotFound},
		{name: "write", method: http.MethodPost, path: "/", status: http.StatusMethodNotAllowed},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			response := requestBundle(t, bundle, current.method, current.path, current.accept, "")
			if response.Code != current.status {
				t.Fatalf("status=%d want=%d body=%q", response.Code, current.status, response.Body.String())
			}
		})
	}
}

func TestBundleRejectsUnsafeOrMalformedTrees(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, directory string)
	}{
		{name: "invalid index UTF-8", setup: func(t *testing.T, directory string) {
			writeBundleFile(t, directory, "index.html", []byte{0xff})
			writeBundleFile(t, directory, "assets/index-AbCd1234.js", []byte("ok"))
		}},
		{name: "unhashed asset", setup: func(t *testing.T, directory string) {
			writeBundleFile(t, directory, "index.html", []byte(testIndexBody))
			writeBundleFile(t, directory, "assets/index.js", []byte("ok"))
		}},
		{name: "unsupported source map", setup: func(t *testing.T, directory string) {
			writeBundleFile(t, directory, "index.html", []byte(testIndexBody))
			writeBundleFile(t, directory, "assets/index-AbCd1234.map", []byte("{}"))
		}},
		{name: "oversized index", setup: func(t *testing.T, directory string) {
			writeBundleFile(t, directory, "index.html", []byte("x"))
			if err := os.Truncate(filepath.Join(directory, "index.html"), MaxIndexBytes+1); err != nil {
				t.Fatal(err)
			}
			writeBundleFile(t, directory, "assets/index-AbCd1234.js", []byte("ok"))
		}},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			directory := t.TempDir()
			current.setup(t, directory)
			if bundle, err := LoadDirectory(directory); err == nil || bundle != nil {
				t.Fatalf("unsafe bundle loaded: %#v", bundle)
			}
		})
	}

	if bundle, err := LoadDirectory(""); err == nil || bundle != nil {
		t.Fatal("empty UI directory was accepted")
	}
}

func TestBundleRejectsAssetSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ordinary Windows test users may not have symbolic-link privilege")
	}
	directory := t.TempDir()
	writeBundleFile(t, directory, "index.html", []byte(testIndexBody))
	writeBundleFile(t, directory, "real-AbCd1234.js", []byte("outside assets"))
	if err := os.MkdirAll(filepath.Join(directory, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "real-AbCd1234.js"),
		filepath.Join(directory, "assets", "index-AbCd1234.js")); err != nil {
		t.Fatal(err)
	}
	if bundle, err := LoadDirectory(directory); err == nil || bundle != nil {
		t.Fatal("Web UI asset symlink was accepted")
	}
}

const testIndexBody = "<!doctype html><script type=\"module\" src=\"/assets/index-AbCd1234.js\"></script>"
const testScriptBody = "document.body.dataset.ready = 'yes';"

func writeTestBundle(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeBundleFile(t, directory, "index.html", []byte(testIndexBody))
	writeBundleFile(t, directory, "assets/index-AbCd1234.js", []byte(testScriptBody))
	writeBundleFile(t, directory, "assets/index-D0TcvGy-.css", []byte("body { color: black; }"))
	return directory
}

func writeBundleFile(t *testing.T, directory string, name string, body []byte) {
	t.Helper()
	filePath := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func requestBundle(t *testing.T, bundle *Bundle, method string, requestPath string,
	accept string, etag string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1"+requestPath, nil)
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response := httptest.NewRecorder()
	bundle.ServeHTTP(response, request)
	return response
}

func TestAssetNameDigestValidation(t *testing.T) {
	for _, name := range []string{"index-AbCd1234.js", "chunk.12345678.css", "logo-A_B_C_D1.png",
		"index-D0TcvGy-.css"} {
		if !assetNameHasDigest(name, strings.ToLower(filepath.Ext(name))) {
			t.Fatalf("valid hashed asset name rejected: %s", name)
		}
	}
	for _, name := range []string{"index.js", "index-short.js", "index-short-.js", "index-bad!.js"} {
		if assetNameHasDigest(name, strings.ToLower(filepath.Ext(name))) {
			t.Fatalf("invalid hashed asset name accepted: %s", name)
		}
	}
}
