//go:build windows && desktop && wv2runtime.error

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"cyberagent-workbench/internal/desktop"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type workspaceLauncherCandidate struct {
	descriptor   desktop.WorkspaceLauncherDescriptor
	executable   string
	passRoot     bool
	allowReparse bool
}

type nativeWorkspaceLauncher struct {
	discover func() ([]workspaceLauncherCandidate, error)
	confirm  func(context.Context, workspaceLauncherCandidate,
		desktop.WorkspaceOpenTarget) (bool, error)
	start func(context.Context, workspaceLauncherCandidate,
		desktop.WorkspaceOpenTarget) error
}

func newNativeWorkspaceLauncher() *nativeWorkspaceLauncher {
	return &nativeWorkspaceLauncher{
		discover: discoverWorkspaceLaunchers,
		confirm:  confirmWorkspaceOpen,
		start:    startWorkspaceLauncher,
	}
}

func (l *nativeWorkspaceLauncher) List(
	ctx context.Context,
) ([]desktop.WorkspaceLauncherDescriptor, error) {
	if l == nil || l.discover == nil || ctx == nil {
		return nil, errors.New("native workspace launcher is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidates, err := l.discover()
	if err != nil {
		return nil, err
	}
	out := make([]desktop.WorkspaceLauncherDescriptor, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.descriptor)
	}
	return out, nil
}

func (l *nativeWorkspaceLauncher) Open(ctx context.Context,
	target desktop.WorkspaceOpenTarget, launcherID string) (desktop.NativeWorkspaceOpenResult, error) {
	if l == nil || l.discover == nil || l.confirm == nil || l.start == nil || ctx == nil {
		return desktop.NativeWorkspaceOpenResult{}, errors.New("native workspace launcher is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	candidates, err := l.discover()
	if err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	candidate, found := findWorkspaceLauncher(candidates, launcherID)
	if !found {
		return desktop.NativeWorkspaceOpenResult{}, errors.New("native workspace launcher was not found")
	}
	if err := validateWorkspaceDirectory(target.RootPath); err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	if err := validateLauncherExecutable(candidate); err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	confirmed, err := l.confirm(ctx, candidate, target)
	if err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	if !confirmed {
		return desktop.NativeWorkspaceOpenResult{Status: desktop.WorkspaceOpenCancelled}, nil
	}
	if err := ctx.Err(); err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	// Revalidate after the native confirmation to reduce replacement races.
	if err := validateWorkspaceDirectory(target.RootPath); err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	if err := validateLauncherExecutable(candidate); err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	if err := l.start(ctx, candidate, target); err != nil {
		return desktop.NativeWorkspaceOpenResult{}, err
	}
	return desktop.NativeWorkspaceOpenResult{
		Status: desktop.WorkspaceOpenStarted, OperatorConfirmed: true,
		ExternalProcessStarted: true,
	}, nil
}

func confirmWorkspaceOpen(ctx context.Context, candidate workspaceLauncherCandidate,
	target desktop.WorkspaceOpenTarget) (bool, error) {
	answer, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:  runtime.QuestionDialog,
		Title: "打开工作区",
		Message: fmt.Sprintf("使用 %s 打开 Workspace \"%s\"？\n\n目录：%s\n应用：%s\n\n"+
			"Prayu 只传递已登记目录，不执行命令。外部应用可能读取该目录内容。",
			candidate.descriptor.Label, target.Name, target.RootPath, candidate.executable),
		Buttons: []string{"打开", "取消"}, DefaultButton: "取消", CancelButton: "取消",
	})
	if err != nil {
		return false, err
	}
	return answer == "打开", nil
}

func startWorkspaceLauncher(ctx context.Context, candidate workspaceLauncherCandidate,
	target desktop.WorkspaceOpenTarget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	command, err := workspaceLauncherCommand(candidate, target)
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	// The external application owns its lifecycle after the confirmed launch.
	_ = command.Process.Release()
	return nil
}

func workspaceLauncherCommand(candidate workspaceLauncherCandidate,
	target desktop.WorkspaceOpenTarget) (*exec.Cmd, error) {
	if err := validateLauncherExecutable(candidate); err != nil {
		return nil, err
	}
	if err := validateWorkspaceDirectory(target.RootPath); err != nil {
		return nil, err
	}
	arguments := []string(nil)
	if candidate.passRoot {
		arguments = []string{target.RootPath}
	}
	command := exec.Command(candidate.executable, arguments...)
	command.Dir = target.RootPath
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	return command, nil
}

func discoverWorkspaceLaunchers() ([]workspaceLauncherCandidate, error) {
	candidates := make(map[string]workspaceLauncherCandidate)
	localAppData, _ := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DEFAULT)
	windowsRoot, _ := windows.KnownFolderPath(windows.FOLDERID_Windows, windows.KF_FLAG_DEFAULT)
	programFiles, _ := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DEFAULT)
	programFilesX86, _ := windows.KnownFolderPath(windows.FOLDERID_ProgramFilesX86, windows.KF_FLAG_DEFAULT)

	addWorkspaceLauncher(candidates, workspaceLauncher("antigravity", "Antigravity",
		desktop.WorkspaceLauncherEditor, filepath.Join(localAppData,
			"Programs", "antigravity", "Antigravity.exe"), true, false))
	addWorkspaceLauncher(candidates, workspaceLauncher("file-explorer", "File Explorer",
		desktop.WorkspaceLauncherFolder, filepath.Join(windowsRoot, "explorer.exe"), true, false))
	addWorkspaceLauncher(candidates, workspaceLauncher("terminal", "Terminal",
		desktop.WorkspaceLauncherTerminal, filepath.Join(localAppData,
			"Microsoft", "WindowsApps", "wt.exe"), false, true))
	if _, exists := candidates["terminal"]; !exists {
		addWorkspaceLauncher(candidates, workspaceLauncher("terminal", "Terminal",
			desktop.WorkspaceLauncherTerminal, filepath.Join(windowsRoot,
				"System32", "WindowsPowerShell", "v1.0", "powershell.exe"), false, false))
	}
	for _, root := range []string{localAppData, programFiles, programFilesX86} {
		addWorkspaceLauncher(candidates, workspaceLauncher("visual-studio-code", "Visual Studio Code",
			desktop.WorkspaceLauncherEditor, filepath.Join(root,
				"Programs", "Microsoft VS Code", "Code.exe"), true, false))
		addWorkspaceLauncher(candidates, workspaceLauncher("visual-studio-code", "Visual Studio Code",
			desktop.WorkspaceLauncherEditor, filepath.Join(root,
				"Microsoft VS Code", "Code.exe"), true, false))
	}
	for _, candidate := range registryWorkspaceLaunchers() {
		addWorkspaceLauncher(candidates, candidate)
	}

	order := map[string]int{
		"antigravity": 0, "file-explorer": 1, "terminal": 2,
		"pycharm": 3, "webstorm": 4, "visual-studio-code": 5,
	}
	out := make([]workspaceLauncherCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate)
	}
	sort.Slice(out, func(left, right int) bool {
		leftOrder, leftKnown := order[out[left].descriptor.ID]
		rightOrder, rightKnown := order[out[right].descriptor.ID]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return out[left].descriptor.ID < out[right].descriptor.ID
	})
	return out, nil
}

func registryWorkspaceLaunchers() []workspaceLauncherCandidate {
	const uninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	type registryRoot struct {
		key  registry.Key
		view uint32
	}
	roots := []registryRoot{
		{registry.CURRENT_USER, registry.WOW64_64KEY},
		{registry.CURRENT_USER, registry.WOW64_32KEY},
		{registry.LOCAL_MACHINE, registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, registry.WOW64_32KEY},
	}
	var out []workspaceLauncherCandidate
	for _, root := range roots {
		key, err := registry.OpenKey(root.key, uninstallPath, registry.READ|root.view)
		if err != nil {
			continue
		}
		names, err := key.ReadSubKeyNames(-1)
		_ = key.Close()
		if err != nil {
			continue
		}
		sort.Strings(names)
		for _, name := range names {
			subkey, openErr := registry.OpenKey(root.key,
				uninstallPath+`\`+name, registry.READ|root.view)
			if openErr != nil {
				continue
			}
			displayName, _, nameErr := subkey.GetStringValue("DisplayName")
			displayIcon, _, iconErr := subkey.GetStringValue("DisplayIcon")
			_ = subkey.Close()
			if nameErr != nil || iconErr != nil {
				continue
			}
			id, label, kind, matched := classifyWorkspaceLauncher(displayName)
			if !matched {
				continue
			}
			executable := parseDisplayIconExecutable(displayIcon)
			if !launcherExecutableMatches(id, executable) {
				continue
			}
			out = append(out, workspaceLauncher(id, label, kind, executable, true, false))
		}
	}
	return out
}

func classifyWorkspaceLauncher(displayName string) (string, string,
	desktop.WorkspaceLauncherKind, bool) {
	value := strings.ToLower(strings.TrimSpace(displayName))
	switch {
	case strings.Contains(value, "antigravity"):
		return "antigravity", "Antigravity", desktop.WorkspaceLauncherEditor, true
	case strings.Contains(value, "pycharm"):
		return "pycharm", "PyCharm", desktop.WorkspaceLauncherEditor, true
	case strings.Contains(value, "webstorm"):
		return "webstorm", "WebStorm", desktop.WorkspaceLauncherEditor, true
	case strings.Contains(value, "visual studio code") || strings.Contains(value, "vscode"):
		return "visual-studio-code", "Visual Studio Code", desktop.WorkspaceLauncherEditor, true
	default:
		return "", "", "", false
	}
}

func parseDisplayIconExecutable(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "%") {
		return ""
	}
	if strings.HasPrefix(value, `"`) {
		if end := strings.Index(value[1:], `"`); end >= 0 {
			return filepath.Clean(value[1 : end+1])
		}
		return ""
	}
	if comma := strings.LastIndex(value, ","); comma >= 0 {
		value = value[:comma]
	}
	return filepath.Clean(strings.TrimSpace(value))
}

func workspaceLauncher(id, label string, kind desktop.WorkspaceLauncherKind,
	executable string, passRoot, allowReparse bool) workspaceLauncherCandidate {
	return workspaceLauncherCandidate{
		descriptor: desktop.WorkspaceLauncherDescriptor{ID: id, Label: label, Kind: kind},
		executable: filepath.Clean(executable), passRoot: passRoot, allowReparse: allowReparse,
	}
}

func addWorkspaceLauncher(target map[string]workspaceLauncherCandidate,
	candidate workspaceLauncherCandidate) {
	if _, exists := target[candidate.descriptor.ID]; exists {
		return
	}
	if err := validateLauncherExecutable(candidate); err != nil {
		return
	}
	target[candidate.descriptor.ID] = candidate
}

func findWorkspaceLauncher(candidates []workspaceLauncherCandidate,
	id string) (workspaceLauncherCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.descriptor.ID == id {
			return candidate, true
		}
	}
	return workspaceLauncherCandidate{}, false
}

func launcherExecutableMatches(id, executable string) bool {
	base := strings.ToLower(filepath.Base(executable))
	switch id {
	case "antigravity":
		return base == "antigravity.exe"
	case "pycharm":
		return base == "pycharm64.exe"
	case "webstorm":
		return base == "webstorm64.exe"
	case "visual-studio-code":
		return base == "code.exe"
	default:
		return false
	}
}

func validateLauncherExecutable(candidate workspaceLauncherCandidate) error {
	path := candidate.executable
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) ||
		!strings.EqualFold(filepath.Ext(path), ".exe") {
		return errors.New("native workspace launcher executable is invalid")
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("native workspace launcher executable is invalid")
	}
	attributes, err := windows.GetFileAttributes(pointer)
	if err != nil || attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 ||
		(!candidate.allowReparse && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0) {
		return errors.New("native workspace launcher executable is unavailable")
	}
	return nil
}

func validateWorkspaceDirectory(root string) error {
	if root == "" || !filepath.IsAbs(root) || strings.ContainsRune(root, 0) {
		return errors.New("registered workspace directory is invalid")
	}
	if root != filepath.Clean(root) {
		return errors.New("registered workspace directory is not canonical")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return errors.New("registered workspace directory is unavailable")
	}
	return nil
}
