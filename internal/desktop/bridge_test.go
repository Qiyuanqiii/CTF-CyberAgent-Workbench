package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/apperror"
)

const (
	testDesktopReadToken    = "desktop-read-token-0123456789abcdef"
	testDesktopControlToken = "desktop-control-token-0123456789abc"
	testDesktopUIDigest     = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

type testSkillPackagePicker struct {
	mu      sync.Mutex
	path    string
	err     error
	started chan struct{}
	release chan struct{}
	calls   int
}

func (p *testSkillPackagePicker) OpenSkillPackage(ctx context.Context) (string, error) {
	p.mu.Lock()
	p.calls++
	started := p.started
	release := p.release
	path := p.path
	err := p.err
	p.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return path, err
}

func TestDesktopBridgeBindsOnlyThreePathlessMethods(t *testing.T) {
	typ := reflect.TypeFor[*DesktopBridge]()
	if typ.NumMethod() != 3 {
		t.Fatalf("exported method count = %d, want 3", typ.NumMethod())
	}
	want := []string{"Bootstrap", "PreviewSkillPackage", "SelectSkillPackage"}
	for index, name := range want {
		if method := typ.Method(index); method.Name != name {
			t.Fatalf("method %d = %s, want %s", index, method.Name, name)
		}
	}
	for _, methodName := range []string{"SelectSkillPackage", "PreviewSkillPackage"} {
		method, ok := typ.MethodByName(methodName)
		if !ok {
			t.Fatalf("missing method %s", methodName)
		}
		for index := 1; index < method.Type.NumIn(); index++ {
			if method.Type.In(index).Kind() == reflect.String && methodName == "SelectSkillPackage" {
				t.Fatalf("native selection method accepts a renderer string: %s", method.Type)
			}
		}
	}
}

func TestDesktopBridgeBootstrapsMemoryOnlyClosedAuthority(t *testing.T) {
	bridge := newTestDesktopBridge(t, context.Background(), &testSkillPackagePicker{})
	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.ProtocolVersion != ConnectionBootstrapProtocolVersion ||
		bootstrap.APIBaseURL != DesktopAPIBasePath || bootstrap.APIVersion != "api.v1" ||
		bootstrap.AppVersion != "test" || bootstrap.UIDigest != testDesktopUIDigest ||
		bootstrap.ReadToken != testDesktopReadToken ||
		bootstrap.ControlToken != testDesktopControlToken || !bootstrap.ControlEnabled ||
		bootstrap.ReadOnlyDefault || bootstrap.ProcessExecutionEnabled ||
		bootstrap.ShellExecutionEnabled || bootstrap.DockerExecutionEnabled ||
		bootstrap.SkillInstallationEnabled || bootstrap.RendererPathInputSupported {
		t.Fatalf("unexpected bootstrap: %#v", bootstrap)
	}
	raw, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, string(raw), []string{
		"api_base_url", "api_version", "app_version", "control_enabled", "control_token",
		"docker_execution_enabled", "process_execution_enabled", "protocol_version", "read_only_default",
		"read_token", "renderer_path_input_supported", "shell_execution_enabled",
		"skill_installation_enabled", "ui_digest",
	})
}

func TestDesktopBridgeSelectsAndConsumesNativePackageWithoutPathDisclosure(t *testing.T) {
	packagePath := filepath.Join(t.TempDir(), "PRIVATE_DESKTOP_PACKAGE.zip")
	writeDesktopSkillPackage(t, packagePath, "PRIVATE_DESCRIPTION", []byte("# Desktop\n"))
	picker := &testSkillPackagePicker{path: packagePath}
	bridge := newTestDesktopBridge(t, context.Background(), picker)

	result, err := bridge.SelectSkillPackage()
	if err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != SkillPackageDialogProtocolVersion ||
		result.Status != SkillPackageDialogSelected || result.Selection == nil {
		t.Fatalf("unexpected dialog result: %#v", result)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	if strings.Contains(serialized, packagePath) || strings.Contains(serialized, filepath.Base(packagePath)) ||
		strings.Contains(serialized, "PRIVATE_DESCRIPTION") {
		t.Fatalf("dialog result disclosed native input: %s", serialized)
	}
	assertExactJSONKeys(t, serialized, []string{"protocol_version", "selection", "status"})

	preview, err := bridge.PreviewSkillPackage(result.Selection.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Name != "desktop-review" || !preview.Validated || preview.InstallationAuthorized {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if _, err := bridge.PreviewSkillPackage(result.Selection.Handle); apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("replay error = %v, code = %s", err, apperror.CodeOf(err))
	}
}

func TestDesktopBridgeCancellationAndNativeErrorsRemainPathFree(t *testing.T) {
	cancelled := newTestDesktopBridge(t, context.Background(), &testSkillPackagePicker{})
	result, err := cancelled.SelectSkillPackage()
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != SkillPackageDialogCancelled || result.Selection != nil {
		t.Fatalf("unexpected cancelled result: %#v", result)
	}

	privateError := errors.New(`open C:\PRIVATE\package.zip: access denied`)
	failing := newTestDesktopBridge(t, context.Background(), &testSkillPackagePicker{err: privateError})
	if _, err := failing.SelectSkillPackage(); apperror.CodeOf(err) != apperror.CodeUnavailable ||
		strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), "package.zip") {
		t.Fatalf("native error was not bounded: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped := newTestDesktopBridge(t, ctx, &testSkillPackagePicker{})
	if _, err := stopped.SelectSkillPackage(); apperror.CodeOf(err) != apperror.CodeCancelled {
		t.Fatalf("stopped selection error = %v, code = %s", err, apperror.CodeOf(err))
	}
}

func TestDesktopBridgeRejectsConcurrentNativeDialogs(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	picker := &testSkillPackagePicker{started: started, release: release}
	bridge := newTestDesktopBridge(t, context.Background(), picker)
	firstDone := make(chan error, 1)
	go func() {
		_, err := bridge.SelectSkillPackage()
		firstDone <- err
	}()
	<-started
	if _, err := bridge.SelectSkillPackage(); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("concurrent dialog error = %v, code = %s", err, apperror.CodeOf(err))
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	picker.mu.Lock()
	defer picker.mu.Unlock()
	if picker.calls != 1 {
		t.Fatalf("native dialog calls = %d, want 1", picker.calls)
	}
}

func TestNewDesktopBridgeRejectsInvalidMetadataAndDependencies(t *testing.T) {
	selector, preview := NewSkillPackagePreviewBoundary()
	valid := DesktopBridgeConfig{
		ContextProvider: func() context.Context { return context.Background() },
		FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
	}
	tests := []struct {
		name   string
		change func(*DesktopBridgeConfig)
	}{
		{name: "missing context", change: func(c *DesktopBridgeConfig) { c.ContextProvider = nil }},
		{name: "missing picker", change: func(c *DesktopBridgeConfig) { c.FilePicker = nil }},
		{name: "short token", change: func(c *DesktopBridgeConfig) { c.ReadToken = "short" }},
		{name: "same control token", change: func(c *DesktopBridgeConfig) { c.ControlToken = c.ReadToken }},
		{name: "bad digest", change: func(c *DesktopBridgeConfig) { c.UIDigest = strings.Repeat("g", 64) }},
		{name: "missing version", change: func(c *DesktopBridgeConfig) { c.APIVersion = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.change(&config)
			if _, err := NewDesktopBridge(config); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
				t.Fatalf("error = %v, code = %s", err, apperror.CodeOf(err))
			}
		})
	}
}

func newTestDesktopBridge(t *testing.T, ctx context.Context, picker SkillPackageFilePicker) *DesktopBridge {
	t.Helper()
	selector, preview := NewSkillPackagePreviewBoundary()
	bridge, err := NewDesktopBridge(DesktopBridgeConfig{
		ContextProvider: func() context.Context { return ctx }, FilePicker: picker,
		ReadToken: testDesktopReadToken, ControlToken: testDesktopControlToken,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bridge
}
