//go:build windows

package credential

import (
	"context"
	"errors"
	"runtime"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credentialTypeGeneric         = 1
	credentialPersistLocalMachine = 2
	credentialTargetPrefix        = "CyberAgentWorkbench/provider/"
	errorNotFound                 = syscall.Errno(1168)
)

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsStore struct{}

func newSystemStore() Store          { return windowsStore{} }
func (windowsStore) Kind() string    { return "windows_credential_manager" }
func (windowsStore) Available() bool { return true }

func (windowsStore) Put(ctx context.Context, name string, secret string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ValidName(name) || !ValidSecret(secret) {
		return errors.New("credential name or secret is invalid")
	}
	target, err := windows.UTF16PtrFromString(credentialTargetPrefix + name)
	if err != nil {
		return err
	}
	username, err := windows.UTF16PtrFromString("CyberAgent Workbench")
	if err != nil {
		return err
	}
	blob := []byte(secret)
	defer zeroBytes(blob)
	credential := windowsCredential{Type: credentialTypeGeneric, TargetName: target,
		CredentialBlobSize: uint32(len(blob)), CredentialBlob: &blob[0],
		Persist: credentialPersistLocalMachine, UserName: username}
	result, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0)
	runtime.KeepAlive(blob)
	runtime.KeepAlive(credential)
	runtime.KeepAlive(target)
	runtime.KeepAlive(username)
	if result == 0 {
		return windowsCallError("CredWriteW", callErr)
	}
	return nil
}

func (windowsStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ValidName(name) {
		return errors.New("credential name is invalid")
	}
	target, err := windows.UTF16PtrFromString(credentialTargetPrefix + name)
	if err != nil {
		return err
	}
	result, _, callErr := procCredDeleteW.Call(uintptr(unsafe.Pointer(target)),
		credentialTypeGeneric, 0)
	runtime.KeepAlive(target)
	if result == 0 && !errors.Is(callErr, errorNotFound) {
		return windowsCallError("CredDeleteW", callErr)
	}
	return nil
}

func (windowsStore) Get(ctx context.Context, name string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !ValidName(name) {
		return "", false, errors.New("credential name is invalid")
	}
	raw, found, err := readWindowsCredential(name)
	if err != nil || !found {
		return "", found, err
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(raw)))
	if raw.CredentialBlobSize == 0 || raw.CredentialBlob == nil ||
		int(raw.CredentialBlobSize) > MaxSecretBytes {
		return "", false, errors.New("stored credential has an invalid size")
	}
	blob := unsafe.Slice(raw.CredentialBlob, int(raw.CredentialBlobSize))
	copyValue := append([]byte(nil), blob...)
	defer zeroBytes(copyValue)
	if !utf8.Valid(copyValue) {
		return "", false, errors.New("stored credential is not UTF-8")
	}
	value := string(copyValue)
	if !ValidSecret(value) {
		return "", false, errors.New("stored credential is invalid")
	}
	return value, true, nil
}

func (s windowsStore) Configured(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !ValidName(name) {
		return false, errors.New("credential name is invalid")
	}
	raw, found, err := readWindowsCredential(name)
	if err != nil || !found {
		return found, err
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(raw)))
	if raw.CredentialBlobSize == 0 || raw.CredentialBlob == nil ||
		int(raw.CredentialBlobSize) > MaxSecretBytes {
		return false, errors.New("stored credential has an invalid size")
	}
	return true, nil
}

// readWindowsCredential returns an OS-owned allocation. Callers must release
// a non-nil result with CredFree and must not retain pointers into its BLOB.
func readWindowsCredential(name string) (*windowsCredential, bool, error) {
	target, err := windows.UTF16PtrFromString(credentialTargetPrefix + name)
	if err != nil {
		return nil, false, err
	}
	var raw *windowsCredential
	result, _, callErr := procCredReadW.Call(uintptr(unsafe.Pointer(target)),
		credentialTypeGeneric, 0, uintptr(unsafe.Pointer(&raw)))
	runtime.KeepAlive(target)
	if result == 0 {
		if errors.Is(callErr, errorNotFound) {
			return nil, false, nil
		}
		return nil, false, windowsCallError("CredReadW", callErr)
	}
	if raw == nil {
		return nil, false, errors.New("CredReadW returned an empty credential")
	}
	return raw, true, nil
}

func windowsCallError(operation string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return errors.New(operation + " failed")
	}
	return errors.New(operation + " failed: " + err.Error())
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
