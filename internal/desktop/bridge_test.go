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

func TestDesktopBridgeBindsOnlyFourPathlessMethods(t *testing.T) {
	typ := reflect.TypeFor[*DesktopBridge]()
	if typ.NumMethod() != 4 {
		t.Fatalf("exported method count = %d, want 4", typ.NumMethod())
	}
	want := []string{"Bootstrap", "InstallSkillPackage", "PreviewSkillPackage", "SelectSkillPackage"}
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
		!bootstrap.RunCreationEnabled || !bootstrap.SessionMessageEnabled ||
		!bootstrap.RunLifecycleEnabled || !bootstrap.RunExecutionEnabled ||
		bootstrap.PlanDeliveryControlEnabled || bootstrap.ApprovalControlEnabled ||
		bootstrap.ModelControlEnabled || bootstrap.ProviderCredentialEnabled ||
		bootstrap.FileEditReviewEnabled || bootstrap.FileEditProposalEnabled ||
		bootstrap.RunWakeControlEnabled || bootstrap.FileEditApplyEnabled ||
		bootstrap.RunWakeExecutionEnabled || bootstrap.RunWakeWorkerEnabled ||
		bootstrap.ReadOnlyDefault || bootstrap.ProcessExecutionEnabled ||
		bootstrap.ShellExecutionEnabled || bootstrap.DockerExecutionEnabled ||
		bootstrap.SkillInstallationEnabled || bootstrap.EvidenceAttachmentEnabled ||
		bootstrap.RendererPathInputSupported {
		t.Fatalf("unexpected bootstrap: %#v", bootstrap)
	}
	raw, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, string(raw), []string{
		"api_base_url", "api_version", "app_version", "approval_control_enabled",
		"control_enabled", "control_token",
		"docker_execution_enabled", "file_edit_review_enabled", "file_edit_proposal_enabled",
		"model_control_enabled", "provider_credential_enabled",
		"file_edit_apply_enabled",
		"process_execution_enabled", "protocol_version", "read_only_default",
		"plan_delivery_control_enabled", "read_token", "renderer_path_input_supported",
		"run_creation_enabled", "shell_execution_enabled",
		"run_execution_enabled", "run_lifecycle_enabled", "run_wake_control_enabled",
		"run_wake_execution_enabled", "run_wake_worker_enabled",
		"session_message_enabled", "session_steering_control_enabled",
		"skill_installation_enabled", "evidence_attachment_enabled", "ui_digest",
	})
}

func TestDesktopBridgeSeparatesEvidenceAttachmentFromOtherControls(t *testing.T) {
	selector, preview := NewSkillPackagePreviewBoundary()
	bridge, err := NewDesktopBridge(DesktopBridgeConfig{
		ContextProvider: func() context.Context { return context.Background() },
		FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
		ControlToken: testDesktopControlToken, EvidenceAttachmentEnabled: true,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
	})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrap.EvidenceAttachmentEnabled || bootstrap.ControlEnabled ||
		bootstrap.RunCreationEnabled || bootstrap.SessionMessageEnabled ||
		bootstrap.SkillInstallationEnabled || bootstrap.ReadOnlyDefault ||
		bootstrap.ControlToken == "" {
		t.Fatalf("evidence attachment widened another capability: %#v", bootstrap)
	}
}

func TestDesktopBridgeSeparatesModelDiffAndWakeControls(t *testing.T) {
	for _, current := range []struct {
		name  string
		model bool
		diff  bool
		wake  bool
	}{
		{name: "model", model: true},
		{name: "diff", diff: true},
		{name: "wake", wake: true},
	} {
		t.Run(current.name, func(t *testing.T) {
			selector, preview := NewSkillPackagePreviewBoundary()
			bridge, err := NewDesktopBridge(DesktopBridgeConfig{
				ContextProvider: func() context.Context { return context.Background() },
				FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
				ControlToken: testDesktopControlToken, ModelControlEnabled: current.model,
				FileEditReviewEnabled: current.diff, RunWakeControlEnabled: current.wake,
				APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
				Selector: selector, PreviewBridge: preview,
			})
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, err := bridge.Bootstrap()
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap.ModelControlEnabled != current.model ||
				bootstrap.FileEditReviewEnabled != current.diff ||
				bootstrap.RunWakeControlEnabled != current.wake ||
				bootstrap.ControlEnabled || bootstrap.RunCreationEnabled ||
				bootstrap.RunExecutionEnabled || bootstrap.ProcessExecutionEnabled ||
				bootstrap.ShellExecutionEnabled || bootstrap.DockerExecutionEnabled ||
				bootstrap.ReadOnlyDefault {
				t.Fatalf("new control capability widened authority: %#v", bootstrap)
			}
		})
	}
}

func TestDesktopBridgeSeparatesPlanAndApprovalControls(t *testing.T) {
	for _, current := range []struct {
		name     string
		plan     bool
		approval bool
	}{
		{name: "plan", plan: true},
		{name: "approval", approval: true},
	} {
		t.Run(current.name, func(t *testing.T) {
			selector, preview := NewSkillPackagePreviewBoundary()
			bridge, err := NewDesktopBridge(DesktopBridgeConfig{
				ContextProvider: func() context.Context { return context.Background() },
				FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
				ControlToken:               testDesktopControlToken,
				PlanDeliveryControlEnabled: current.plan,
				ApprovalControlEnabled:     current.approval,
				APIVersion:                 "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
				Selector: selector, PreviewBridge: preview,
			})
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, err := bridge.Bootstrap()
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap.PlanDeliveryControlEnabled != current.plan ||
				bootstrap.ApprovalControlEnabled != current.approval ||
				bootstrap.ControlEnabled || bootstrap.RunCreationEnabled ||
				bootstrap.SessionMessageEnabled || bootstrap.RunLifecycleEnabled ||
				bootstrap.RunExecutionEnabled || bootstrap.ReadOnlyDefault {
				t.Fatalf("Plan or approval capability widened authority: %#v", bootstrap)
			}
		})
	}
}

func TestDesktopBridgeSeparatesRunLifecycleAndExecutionCapabilities(t *testing.T) {
	for _, current := range []struct {
		name      string
		lifecycle bool
		execution bool
	}{
		{name: "lifecycle", lifecycle: true},
		{name: "execution", execution: true},
	} {
		t.Run(current.name, func(t *testing.T) {
			selector, preview := NewSkillPackagePreviewBoundary()
			bridge, err := NewDesktopBridge(DesktopBridgeConfig{
				ContextProvider: func() context.Context { return context.Background() },
				FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
				ControlToken:        testDesktopControlToken,
				RunLifecycleEnabled: current.lifecycle, RunExecutionEnabled: current.execution,
				APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
				Selector: selector, PreviewBridge: preview,
			})
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, err := bridge.Bootstrap()
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap.RunLifecycleEnabled != current.lifecycle ||
				bootstrap.RunExecutionEnabled != current.execution ||
				bootstrap.ControlEnabled || bootstrap.RunCreationEnabled ||
				bootstrap.SessionMessageEnabled || bootstrap.SessionSteeringControlEnabled ||
				bootstrap.ReadOnlyDefault {
				t.Fatalf("Run operation capability widened authority: %#v", bootstrap)
			}
		})
	}
}

func TestDesktopBridgeSeparatesSessionSteeringControlFromOtherControls(t *testing.T) {
	selector, preview := NewSkillPackagePreviewBoundary()
	bridge, err := NewDesktopBridge(DesktopBridgeConfig{
		ContextProvider: func() context.Context { return context.Background() },
		FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
		ControlToken: testDesktopControlToken, SessionSteeringControlEnabled: true,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
	})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.ControlEnabled || bootstrap.RunCreationEnabled ||
		bootstrap.SessionMessageEnabled || !bootstrap.SessionSteeringControlEnabled ||
		bootstrap.ReadOnlyDefault || bootstrap.ControlToken == "" {
		t.Fatalf("Session steering cancellation widened another capability: %#v", bootstrap)
	}
}

func TestDesktopBridgeSeparatesSessionMessagesFromOtherControls(t *testing.T) {
	selector, preview := NewSkillPackagePreviewBoundary()
	bridge, err := NewDesktopBridge(DesktopBridgeConfig{
		ContextProvider: func() context.Context { return context.Background() },
		FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
		ControlToken: testDesktopControlToken, SessionMessageEnabled: true,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
	})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.ControlEnabled || bootstrap.RunCreationEnabled ||
		!bootstrap.SessionMessageEnabled || bootstrap.ReadOnlyDefault ||
		bootstrap.ControlToken == "" {
		t.Fatalf("Session message submission widened another capability: %#v", bootstrap)
	}
}

func TestDesktopBridgeSeparatesRunCreationFromExistingRunControls(t *testing.T) {
	selector, preview := NewSkillPackagePreviewBoundary()
	bridge, err := NewDesktopBridge(DesktopBridgeConfig{
		ContextProvider: func() context.Context { return context.Background() },
		FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
		ControlToken: testDesktopControlToken, RunCreationEnabled: true,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
	})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.ControlEnabled || !bootstrap.RunCreationEnabled || bootstrap.ReadOnlyDefault ||
		bootstrap.ControlToken == "" {
		t.Fatalf("Run creation widened another capability: %#v", bootstrap)
	}
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
		{name: "capability without token", change: func(c *DesktopBridgeConfig) { c.RunCreationEnabled = true }},
		{name: "approval without token", change: func(c *DesktopBridgeConfig) { c.ApprovalControlEnabled = true }},
		{name: "evidence without token", change: func(c *DesktopBridgeConfig) { c.EvidenceAttachmentEnabled = true }},
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
		RunControlEnabled: true, RunCreationEnabled: true, SessionMessageEnabled: true,
		RunLifecycleEnabled: true, RunExecutionEnabled: true,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bridge
}
