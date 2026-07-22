//go:build windows

package browserruntime

import "golang.org/x/sys/windows"

func defaultBrowserDiscoveryRoots() []DiscoveryRoot {
	return []DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: knownFolderPath(windows.FOLDERID_ProgramFiles)},
		{ID: DiscoveryRootProgramFilesX86, Path: knownFolderPath(windows.FOLDERID_ProgramFilesX86)},
		{ID: DiscoveryRootLocalAppData, Path: knownFolderPath(windows.FOLDERID_LocalAppData)},
	}
}

func knownFolderPath(id *windows.KNOWNFOLDERID) string {
	path, err := windows.KnownFolderPath(id, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return ""
	}
	return path
}
