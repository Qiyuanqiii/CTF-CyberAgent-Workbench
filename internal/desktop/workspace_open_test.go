package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/apperror"
)

type testWorkspaceResolver struct {
	target WorkspaceOpenTarget
	err    error
}

func (r testWorkspaceResolver) ResolveWorkspace(context.Context,
	string) (WorkspaceOpenTarget, error) {
	return r.target, r.err
}

type testWorkspaceLauncher struct {
	mu          sync.Mutex
	descriptors []WorkspaceLauncherDescriptor
	result      NativeWorkspaceOpenResult
	err         error
	started     chan struct{}
	release     chan struct{}
	openCalls   int
	target      WorkspaceOpenTarget
	launcherID  string
}

func (l *testWorkspaceLauncher) List(context.Context) ([]WorkspaceLauncherDescriptor, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]WorkspaceLauncherDescriptor(nil), l.descriptors...), nil
}

func (l *testWorkspaceLauncher) Open(ctx context.Context, target WorkspaceOpenTarget,
	launcherID string) (NativeWorkspaceOpenResult, error) {
	l.mu.Lock()
	l.openCalls++
	l.target = target
	l.launcherID = launcherID
	started := l.started
	release := l.release
	result := l.result
	err := l.err
	l.mu.Unlock()
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
			return NativeWorkspaceOpenResult{}, ctx.Err()
		}
	}
	return result, err
}

func TestWorkspaceLauncherListIsStrictAndPathless(t *testing.T) {
	root := t.TempDir()
	launcher := &testWorkspaceLauncher{descriptors: testWorkspaceLauncherDescriptors()}
	bridge := newWorkspaceDesktopBridge(t, WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: root,
	}, launcher)
	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrap.WorkspaceOpenEnabled || !bootstrap.ReadOnlyDefault ||
		bootstrap.ControlToken != "" || bootstrap.ProcessExecutionEnabled ||
		bootstrap.ShellExecutionEnabled || bootstrap.DockerExecutionEnabled {
		t.Fatalf("workspace opener widened Desktop authority: %#v", bootstrap)
	}

	result, err := bridge.WorkspaceLaunchers("workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != WorkspaceLauncherListProtocolVersion ||
		result.WorkspaceID != "workspace-1" || len(result.Launchers) != 2 ||
		result.RootPathExposed || result.RendererPathInputSupported ||
		result.ArbitraryArgumentsAccepted || result.AgentAuthorityGranted {
		t.Fatalf("unexpected launcher list: %#v", result)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), root) {
		t.Fatalf("launcher list disclosed root: %s", raw)
	}
	assertExactJSONKeys(t, string(raw), []string{
		"agent_authority_granted", "arbitrary_arguments_accepted", "launchers",
		"protocol_version", "renderer_path_input_supported", "root_path_exposed", "workspace_id",
	})
}

func TestOpenWorkspaceRequiresNativeConfirmationAndReturnsPathlessReceipt(t *testing.T) {
	root := t.TempDir()
	launcher := &testWorkspaceLauncher{
		descriptors: testWorkspaceLauncherDescriptors(),
		result: NativeWorkspaceOpenResult{Status: WorkspaceOpenStarted,
			OperatorConfirmed: true, ExternalProcessStarted: true},
	}
	bridge := newWorkspaceDesktopBridge(t, WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: root,
	}, launcher)
	result, err := bridge.OpenWorkspace(WorkspaceOpenRequest{
		ProtocolVersion: WorkspaceOpenProtocolVersion,
		WorkspaceID:     "workspace-1", LauncherID: "file-explorer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != WorkspaceOpenStarted || !result.OperatorConfirmed ||
		!result.ExternalProcessStarted || result.ArbitraryArgumentsAccepted ||
		result.CommandExecuted || result.RootPathExposed || result.AgentAuthorityGranted {
		t.Fatalf("unexpected workspace open result: %#v", result)
	}
	launcher.mu.Lock()
	if launcher.openCalls != 1 || launcher.target.RootPath != root ||
		launcher.launcherID != "file-explorer" {
		t.Fatalf("unexpected native open call: calls=%d target=%#v launcher=%q",
			launcher.openCalls, launcher.target, launcher.launcherID)
	}
	launcher.mu.Unlock()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), root) {
		t.Fatalf("workspace open receipt disclosed root: %s", raw)
	}
	assertExactJSONKeys(t, string(raw), []string{
		"agent_authority_granted", "arbitrary_arguments_accepted", "command_executed",
		"external_process_started", "launcher_id", "operator_confirmed", "protocol_version",
		"root_path_exposed", "status", "workspace_id",
	})
}

func TestOpenWorkspaceCancellationAndUnknownLauncherFailClosed(t *testing.T) {
	launcher := &testWorkspaceLauncher{
		descriptors: testWorkspaceLauncherDescriptors(),
		result:      NativeWorkspaceOpenResult{Status: WorkspaceOpenCancelled},
	}
	bridge := newWorkspaceDesktopBridge(t, WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: t.TempDir(),
	}, launcher)
	cancelled, err := bridge.OpenWorkspace(WorkspaceOpenRequest{
		ProtocolVersion: WorkspaceOpenProtocolVersion,
		WorkspaceID:     "workspace-1", LauncherID: "terminal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != WorkspaceOpenCancelled || cancelled.OperatorConfirmed ||
		cancelled.ExternalProcessStarted {
		t.Fatalf("unexpected cancellation result: %#v", cancelled)
	}
	_, err = bridge.OpenWorkspace(WorkspaceOpenRequest{
		ProtocolVersion: WorkspaceOpenProtocolVersion,
		WorkspaceID:     "workspace-1", LauncherID: "unknown",
	})
	if apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("unknown launcher error = %v, code = %s", err, apperror.CodeOf(err))
	}
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if launcher.openCalls != 1 {
		t.Fatalf("native open calls = %d, want 1", launcher.openCalls)
	}
}

func TestOpenWorkspaceRejectsInvalidNativeReceiptAndRedactsResolverErrors(t *testing.T) {
	launcher := &testWorkspaceLauncher{
		descriptors: testWorkspaceLauncherDescriptors(),
		result: NativeWorkspaceOpenResult{Status: WorkspaceOpenStarted,
			ExternalProcessStarted: true},
	}
	bridge := newWorkspaceDesktopBridge(t, WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: t.TempDir(),
	}, launcher)
	_, err := bridge.OpenWorkspace(WorkspaceOpenRequest{
		ProtocolVersion: WorkspaceOpenProtocolVersion,
		WorkspaceID:     "workspace-1", LauncherID: "terminal",
	})
	if apperror.CodeOf(err) != apperror.CodeInternal {
		t.Fatalf("invalid native receipt error = %v, code = %s", err, apperror.CodeOf(err))
	}

	privatePath := `C:\Users\private\workspace`
	selector, preview := NewSkillPackagePreviewBoundary()
	redacting, createErr := NewDesktopBridge(DesktopBridgeConfig{
		ContextProvider: func() context.Context { return context.Background() },
		FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
		WorkspaceResolver: testWorkspaceResolver{err: errors.New(privatePath)},
		WorkspaceLauncher: launcher,
	})
	if createErr != nil {
		t.Fatal(createErr)
	}
	_, err = redacting.WorkspaceLaunchers("workspace-1")
	if apperror.CodeOf(err) != apperror.CodeUnavailable || strings.Contains(err.Error(), privatePath) {
		t.Fatalf("resolver error was not redacted: %v", err)
	}
}

func TestOpenWorkspaceRejectsConcurrentConfirmations(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	launcher := &testWorkspaceLauncher{
		descriptors: testWorkspaceLauncherDescriptors(), started: started, release: release,
		result: NativeWorkspaceOpenResult{Status: WorkspaceOpenStarted,
			OperatorConfirmed: true, ExternalProcessStarted: true},
	}
	bridge := newWorkspaceDesktopBridge(t, WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: t.TempDir(),
	}, launcher)
	request := WorkspaceOpenRequest{ProtocolVersion: WorkspaceOpenProtocolVersion,
		WorkspaceID: "workspace-1", LauncherID: "terminal"}
	firstDone := make(chan error, 1)
	go func() {
		_, openErr := bridge.OpenWorkspace(request)
		firstDone <- openErr
	}()
	<-started
	if _, err := bridge.OpenWorkspace(request); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("concurrent open error = %v, code = %s", err, apperror.CodeOf(err))
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if launcher.openCalls != 1 {
		t.Fatalf("native open calls = %d, want 1", launcher.openCalls)
	}
}

func TestWorkspaceOpenMethodsRejectInvalidOrDisabledRequests(t *testing.T) {
	disabled := newTestDesktopBridge(t, context.Background(), &testSkillPackagePicker{})
	if _, err := disabled.WorkspaceLaunchers("workspace-1"); apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("disabled launcher list error = %v, code = %s", err, apperror.CodeOf(err))
	}
	if _, err := disabled.OpenWorkspace(WorkspaceOpenRequest{}); apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("disabled open error = %v, code = %s", err, apperror.CodeOf(err))
	}

	launcher := &testWorkspaceLauncher{descriptors: testWorkspaceLauncherDescriptors()}
	bridge := newWorkspaceDesktopBridge(t, WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: t.TempDir(),
	}, launcher)
	if _, err := bridge.WorkspaceLaunchers(" workspace-1"); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("invalid workspace ID error = %v, code = %s", err, apperror.CodeOf(err))
	}
	for _, request := range []WorkspaceOpenRequest{
		{ProtocolVersion: "wrong", WorkspaceID: "workspace-1", LauncherID: "terminal"},
		{ProtocolVersion: WorkspaceOpenProtocolVersion, WorkspaceID: "workspace-1", LauncherID: "Terminal"},
		{ProtocolVersion: WorkspaceOpenProtocolVersion, WorkspaceID: "workspace-1", LauncherID: "bad id"},
	} {
		if _, err := bridge.OpenWorkspace(request); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
			t.Fatalf("invalid request %#v error = %v, code = %s", request, err, apperror.CodeOf(err))
		}
	}
}

func TestWorkspaceOpenRejectsNonCanonicalResolvedRoot(t *testing.T) {
	root := t.TempDir()
	launcher := &testWorkspaceLauncher{descriptors: testWorkspaceLauncherDescriptors()}
	bridge := newWorkspaceDesktopBridge(t, WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: root + string(filepath.Separator) + ".",
	}, launcher)
	_, err := bridge.WorkspaceLaunchers("workspace-1")
	if apperror.CodeOf(err) != apperror.CodeInternal {
		t.Fatalf("non-canonical root error = %v, code = %s", err, apperror.CodeOf(err))
	}
}

func newWorkspaceDesktopBridge(t *testing.T, target WorkspaceOpenTarget,
	launcher NativeWorkspaceLauncher) *DesktopBridge {
	t.Helper()
	selector, preview := NewSkillPackagePreviewBoundary()
	bridge, err := NewDesktopBridge(DesktopBridgeConfig{
		ContextProvider: func() context.Context { return context.Background() },
		FilePicker:      &testSkillPackagePicker{}, ReadToken: testDesktopReadToken,
		APIVersion: "api.v1", AppVersion: "test", UIDigest: testDesktopUIDigest,
		Selector: selector, PreviewBridge: preview,
		WorkspaceResolver: testWorkspaceResolver{target: target}, WorkspaceLauncher: launcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bridge
}

func testWorkspaceLauncherDescriptors() []WorkspaceLauncherDescriptor {
	return []WorkspaceLauncherDescriptor{
		{ID: "file-explorer", Label: "File Explorer", Kind: WorkspaceLauncherFolder},
		{ID: "terminal", Label: "Terminal", Kind: WorkspaceLauncherTerminal},
	}
}
