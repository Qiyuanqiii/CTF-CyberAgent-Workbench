//go:build windows

package runner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsStillActive = 259

type windowsConformanceTree struct {
	job       windows.Handle
	parentPID int
	childPID  int
}

func startPlatformConformanceTree(ctx context.Context, command *exec.Cmd,
	directory string,
) (conformanceTreeController, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	closeJob := true
	defer func() {
		if closeJob {
			_ = windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job,
		windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = windows.TerminateJobObject(job, 137)
		_ = command.Wait()
	}
	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|
			windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false, uint32(command.Process.Pid))
	if err != nil {
		cleanup()
		return nil, err
	}
	err = windows.AssignProcessToJobObject(job, processHandle)
	_ = windows.CloseHandle(processHandle)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := writeConformanceMarker(filepath.Join(directory, "assigned")); err != nil {
		cleanup()
		return nil, err
	}
	childPID, err := waitForConformanceChild(ctx, directory)
	if err != nil {
		cleanup()
		return nil, err
	}
	closeJob = false
	return &windowsConformanceTree{job: job, parentPID: command.Process.Pid,
		childPID: childPID}, nil
}

func (c *windowsConformanceTree) Terminate(ctx context.Context, stopMarker string) error {
	if err := writeConformanceMarker(stopMarker); err != nil {
		return err
	}
	return c.waitReaped(ctx)
}

func (c *windowsConformanceTree) Kill(ctx context.Context) error {
	if err := windows.TerminateJobObject(c.job, 137); err != nil {
		return err
	}
	return c.waitReaped(ctx)
}

func (c *windowsConformanceTree) Inspect(ctx context.Context) (TreeState, error) {
	if err := ctx.Err(); err != nil {
		return TreeState{}, err
	}
	parentRunning, err := windowsProcessAlive(c.parentPID)
	if err != nil {
		return TreeState{}, err
	}
	childRunning, err := windowsProcessAlive(c.childPID)
	if err != nil {
		return TreeState{}, err
	}
	descendants := 0
	if childRunning {
		descendants = 1
	}
	return TreeState{ParentRunning: parentRunning, LiveDescendants: descendants,
		Reaped: !parentRunning && !childRunning}, nil
}

func (c *windowsConformanceTree) Close() error {
	return windows.CloseHandle(c.job)
}

func (c *windowsConformanceTree) waitReaped(ctx context.Context) error {
	for {
		state, err := c.Inspect(ctx)
		if err != nil {
			return err
		}
		if state.Reaped {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func windowsProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		return false, err
	}
	defer windows.CloseHandle(handle)
	exitCode := uint32(windowsStillActive)
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false, err
	}
	return exitCode == windowsStillActive, nil
}
