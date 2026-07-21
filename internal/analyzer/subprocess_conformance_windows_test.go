//go:build windows

package analyzer

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const analyzerWindowsStillActive = 259

type windowsSubprocessConformanceController struct {
	job       windows.Handle
	parentPID int
	childPID  int
}

func startSubprocessConformanceProcess(command *exec.Cmd) (conformanceProcessController, error) {
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
	closeJob = false
	return &windowsSubprocessConformanceController{
		job: job, parentPID: command.Process.Pid,
	}, nil
}

func (controller *windowsSubprocessConformanceController) Terminate(ctx context.Context) error {
	// Windows has no portable SIGTERM equivalent for a hidden process group.
	// The bounded common harness records this attempt, waits the grace period,
	// then terminates the Job Object as its explicit hard-stop fallback.
	return ctx.Err()
}

func (controller *windowsSubprocessConformanceController) Kill(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := windows.TerminateJobObject(controller.job, 137); err != nil {
		return err
	}
	return controller.waitReaped(ctx)
}

func (controller *windowsSubprocessConformanceController) State(ctx context.Context) (
	conformanceTreeState, error,
) {
	if err := ctx.Err(); err != nil {
		return conformanceTreeState{}, err
	}
	parent, err := analyzerWindowsProcessAlive(controller.parentPID)
	if err != nil {
		return conformanceTreeState{}, err
	}
	child, err := analyzerWindowsProcessAlive(controller.childPID)
	if err != nil {
		return conformanceTreeState{}, err
	}
	return conformanceTreeState{ParentRunning: parent, ChildRunning: child,
		TreeReaped: !parent && !child, OrphanDetected: !parent && child}, nil
}

func (controller *windowsSubprocessConformanceController) SetChildPID(pid int) {
	controller.childPID = pid
}

func (controller *windowsSubprocessConformanceController) SetStopMarker(path string) {
	_ = path
}

func (controller *windowsSubprocessConformanceController) Close() error {
	return windows.CloseHandle(controller.job)
}

func (controller *windowsSubprocessConformanceController) waitReaped(ctx context.Context) error {
	for {
		state, err := controller.State(ctx)
		if err != nil {
			return err
		}
		if state.TreeReaped {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func analyzerWindowsProcessAlive(pid int) (bool, error) {
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
	exitCode := uint32(analyzerWindowsStillActive)
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false, err
	}
	return exitCode == analyzerWindowsStillActive, nil
}

func ignoreAnalyzerTerminateSignals() {}

func conformanceCancellationRequiresKill() bool { return true }
