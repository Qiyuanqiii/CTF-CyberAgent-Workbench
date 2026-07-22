//go:build windows

package browserruntime

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	versionDLL                 = windows.NewLazySystemDLL("version.dll")
	getFileVersionInfoSizeProc = versionDLL.NewProc("GetFileVersionInfoSizeW")
	getFileVersionInfoProc     = versionDLL.NewProc("GetFileVersionInfoW")
	verQueryValueProc          = versionDLL.NewProc("VerQueryValueW")
)

type fixedFileInfo struct {
	Signature        uint32
	StructureVersion uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
	FileFlagsMask    uint32
	FileFlags        uint32
	FileOS           uint32
	FileType         uint32
	FileSubtype      uint32
	FileDateMS       uint32
	FileDateLS       uint32
}

func browserExecutableVersion(path string) (string, ExecutableVersionSource, bool) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", VersionSourceUnavailable, false
	}
	size, _, _ := getFileVersionInfoSizeProc.Call(uintptr(unsafe.Pointer(pathPointer)), 0)
	if size == 0 || size > 16*1024*1024 {
		return "", VersionSourceUnavailable, false
	}
	buffer := make([]byte, int(size))
	ok, _, _ := getFileVersionInfoProc.Call(uintptr(unsafe.Pointer(pathPointer)), 0, size,
		uintptr(unsafe.Pointer(&buffer[0])))
	if ok == 0 {
		return "", VersionSourceUnavailable, false
	}
	rootPointer, err := windows.UTF16PtrFromString("\\")
	if err != nil {
		return "", VersionSourceUnavailable, false
	}
	var value unsafe.Pointer
	var valueBytes uint32
	ok, _, _ = verQueryValueProc.Call(uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(rootPointer)), uintptr(unsafe.Pointer(&value)),
		uintptr(unsafe.Pointer(&valueBytes)))
	if ok == 0 || value == nil || uintptr(valueBytes) < unsafe.Sizeof(fixedFileInfo{}) {
		return "", VersionSourceUnavailable, false
	}
	info := (*fixedFileInfo)(value)
	if info.Signature != 0xfeef04bd {
		return "", VersionSourceUnavailable, false
	}
	version := fmt.Sprintf("%d.%d.%d.%d", info.FileVersionMS>>16,
		info.FileVersionMS&0xffff, info.FileVersionLS>>16, info.FileVersionLS&0xffff)
	return version, VersionSourceWindowsResource, true
}
