//go:build windows && desktop && wv2runtime.error

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"cyberagent-workbench/internal/desktop"

	"golang.org/x/sys/windows"
)

func TestNativeWorkspaceLauncherCancellationAndConfirmedStart(t *testing.T) {
	root := t.TempDir()
	executable := createFakeExecutable(t, root, "editor.exe")
	candidate := workspaceLauncher("editor", "Editor", desktop.WorkspaceLauncherEditor,
		executable, true, false)
	target := desktop.WorkspaceOpenTarget{ID: "workspace-1", Name: "demo", RootPath: root}
	confirmed := false
	starts := 0
	launcher := &nativeWorkspaceLauncher{
		discover: func() ([]workspaceLauncherCandidate, error) {
			return []workspaceLauncherCandidate{candidate}, nil
		},
		confirm: func(_ context.Context, got workspaceLauncherCandidate,
			gotTarget desktop.WorkspaceOpenTarget) (bool, error) {
			if got.descriptor.ID != candidate.descriptor.ID || gotTarget != target {
				t.Fatalf("unexpected confirmation input: %#v %#v", got, gotTarget)
			}
			return confirmed, nil
		},
		start: func(_ context.Context, got workspaceLauncherCandidate,
			gotTarget desktop.WorkspaceOpenTarget) error {
			starts++
			if got.executable != executable || gotTarget != target {
				t.Fatalf("unexpected start input: %#v %#v", got, gotTarget)
			}
			return nil
		},
	}

	cancelled, err := launcher.Open(context.Background(), target, "editor")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != desktop.WorkspaceOpenCancelled ||
		cancelled.OperatorConfirmed || cancelled.ExternalProcessStarted || starts != 0 {
		t.Fatalf("unexpected cancelled result: %#v starts=%d", cancelled, starts)
	}

	confirmed = true
	started, err := launcher.Open(context.Background(), target, "editor")
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != desktop.WorkspaceOpenStarted || !started.OperatorConfirmed ||
		!started.ExternalProcessStarted || starts != 1 {
		t.Fatalf("unexpected started result: %#v starts=%d", started, starts)
	}
}

func TestNativeWorkspaceLauncherFailsBeforeConfirmationForInvalidInputs(t *testing.T) {
	root := t.TempDir()
	confirmations := 0
	launcher := &nativeWorkspaceLauncher{
		discover: func() ([]workspaceLauncherCandidate, error) {
			return []workspaceLauncherCandidate{workspaceLauncher("editor", "Editor",
				desktop.WorkspaceLauncherEditor, filepath.Join(root, "missing.exe"), true, false)}, nil
		},
		confirm: func(context.Context, workspaceLauncherCandidate,
			desktop.WorkspaceOpenTarget) (bool, error) {
			confirmations++
			return true, nil
		},
		start: func(context.Context, workspaceLauncherCandidate,
			desktop.WorkspaceOpenTarget) error {
			t.Fatal("start must not run")
			return nil
		},
	}
	_, err := launcher.Open(context.Background(), desktop.WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: root,
	}, "editor")
	if err == nil || confirmations != 0 {
		t.Fatalf("invalid executable error = %v confirmations=%d", err, confirmations)
	}

	launcher.discover = func() ([]workspaceLauncherCandidate, error) {
		executable := createFakeExecutable(t, root, "editor.exe")
		return []workspaceLauncherCandidate{workspaceLauncher("editor", "Editor",
			desktop.WorkspaceLauncherEditor, executable, true, false)}, nil
	}
	_, err = launcher.Open(context.Background(), desktop.WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: filepath.Join(root, "missing"),
	}, "editor")
	if err == nil || confirmations != 0 {
		t.Fatalf("invalid root error = %v confirmations=%d", err, confirmations)
	}
}

func TestWorkspaceLauncherCommandUsesOnlyFixedExecutableAndRegisteredRoot(t *testing.T) {
	root := t.TempDir()
	executable := createFakeExecutable(t, root, "editor.exe")
	target := desktop.WorkspaceOpenTarget{ID: "workspace-1", Name: "demo", RootPath: root}
	editor := workspaceLauncher("editor", "Editor", desktop.WorkspaceLauncherEditor,
		executable, true, false)
	command, err := workspaceLauncherCommand(editor, target)
	if err != nil {
		t.Fatal(err)
	}
	if command.Path != executable || command.Dir != root ||
		!reflect.DeepEqual(command.Args, []string{executable, root}) || command.SysProcAttr == nil ||
		command.SysProcAttr.CreationFlags != windows.CREATE_NEW_PROCESS_GROUP {
		t.Fatalf("unexpected editor command: path=%q dir=%q args=%#v attr=%#v",
			command.Path, command.Dir, command.Args, command.SysProcAttr)
	}

	terminal := workspaceLauncher("terminal", "Terminal", desktop.WorkspaceLauncherTerminal,
		executable, false, false)
	command, err = workspaceLauncherCommand(terminal, target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(command.Args, []string{executable}) {
		t.Fatalf("terminal received arguments: %#v", command.Args)
	}

	nonCanonical := target
	nonCanonical.RootPath = root + string(filepath.Separator) + "."
	if _, err := workspaceLauncherCommand(editor, nonCanonical); err == nil {
		t.Fatal("non-canonical registered root was accepted")
	}
}

func TestDisplayIconParsingAndLauncherClassificationAreBounded(t *testing.T) {
	for _, current := range []struct {
		input string
		want  string
	}{
		{input: `"D:\JetBrains\bin\pycharm64.exe",0`,
			want: `D:\JetBrains\bin\pycharm64.exe`},
		{input: `D:\WebStorm\bin\webstorm64.exe,0`,
			want: `D:\WebStorm\bin\webstorm64.exe`},
		{input: `%LOCALAPPDATA%\Programs\Code.exe`, want: ""},
		{input: `"D:\broken.exe`, want: ""},
	} {
		if got := parseDisplayIconExecutable(current.input); got != current.want {
			t.Fatalf("parseDisplayIconExecutable(%q) = %q, want %q",
				current.input, got, current.want)
		}
	}
	for _, current := range []struct {
		name string
		id   string
		ok   bool
	}{
		{name: "JetBrains PyCharm 2025.1", id: "pycharm", ok: true},
		{name: "JetBrains WebStorm 2025.3", id: "webstorm", ok: true},
		{name: "Microsoft Visual Studio Code", id: "visual-studio-code", ok: true},
		{name: "Unknown Editor", ok: false},
	} {
		id, _, _, ok := classifyWorkspaceLauncher(current.name)
		if id != current.id || ok != current.ok {
			t.Fatalf("classifyWorkspaceLauncher(%q) = (%q, %v)", current.name, id, ok)
		}
	}
}

func TestDiscoverWorkspaceLaunchersIncludesExplorerAndUniqueIDs(t *testing.T) {
	launchers, err := discoverWorkspaceLaunchers()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, len(launchers))
	foundExplorer := false
	for _, launcher := range launchers {
		id := launcher.descriptor.ID
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate launcher identifier %q", id)
		}
		seen[id] = struct{}{}
		if id == "file-explorer" {
			foundExplorer = true
			if launcher.descriptor.Kind != desktop.WorkspaceLauncherFolder || !launcher.passRoot {
				t.Fatalf("unexpected File Explorer descriptor: %#v", launcher)
			}
		}
	}
	if !foundExplorer {
		t.Fatal("File Explorer was not discovered from the Windows known folder")
	}
}

func TestNativeWorkspaceLauncherPropagatesConfirmationFailureWithoutStarting(t *testing.T) {
	root := t.TempDir()
	executable := createFakeExecutable(t, root, "editor.exe")
	want := errors.New("dialog failed")
	starts := 0
	launcher := &nativeWorkspaceLauncher{
		discover: func() ([]workspaceLauncherCandidate, error) {
			return []workspaceLauncherCandidate{workspaceLauncher("editor", "Editor",
				desktop.WorkspaceLauncherEditor, executable, true, false)}, nil
		},
		confirm: func(context.Context, workspaceLauncherCandidate,
			desktop.WorkspaceOpenTarget) (bool, error) {
			return false, want
		},
		start: func(context.Context, workspaceLauncherCandidate,
			desktop.WorkspaceOpenTarget) error {
			starts++
			return nil
		},
	}
	_, err := launcher.Open(context.Background(), desktop.WorkspaceOpenTarget{
		ID: "workspace-1", Name: "demo", RootPath: root,
	}, "editor")
	if !errors.Is(err, want) || starts != 0 {
		t.Fatalf("confirmation error = %v starts=%d", err, starts)
	}
}

func createFakeExecutable(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("not executed"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
