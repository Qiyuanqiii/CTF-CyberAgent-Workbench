//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package analyzer

import (
	"context"
	"errors"
	"os/exec"
	"os/signal"
	"syscall"
)

type unixSubprocessConformanceController struct {
	processGroupID int
	parentPID      int
	childPID       int
	stopMarker     string
}

func startSubprocessConformanceProcess(command *exec.Cmd) (conformanceProcessController, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &unixSubprocessConformanceController{
		processGroupID: command.Process.Pid, parentPID: command.Process.Pid,
	}, nil
}

func (controller *unixSubprocessConformanceController) Terminate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return signalAnalyzerProcessGroup(controller.processGroupID, syscall.SIGTERM)
}

func (controller *unixSubprocessConformanceController) Kill(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if controller.childPID > 0 && controller.stopMarker != "" {
		if err := signalAnalyzerProcess(controller.childPID, syscall.SIGKILL); err != nil {
			return err
		}
		return writeAnalyzerMarker(controller.stopMarker)
	}
	return signalAnalyzerProcessGroup(controller.processGroupID, syscall.SIGKILL)
}

func (controller *unixSubprocessConformanceController) State(ctx context.Context) (
	conformanceTreeState, error,
) {
	if err := ctx.Err(); err != nil {
		return conformanceTreeState{}, err
	}
	parent, err := analyzerUnixProcessAlive(controller.parentPID)
	if err != nil {
		return conformanceTreeState{}, err
	}
	child, err := analyzerUnixProcessAlive(controller.childPID)
	if err != nil {
		return conformanceTreeState{}, err
	}
	return conformanceTreeState{ParentRunning: parent, ChildRunning: child,
		TreeReaped: !parent && !child, OrphanDetected: !parent && child}, nil
}

func (controller *unixSubprocessConformanceController) SetChildPID(pid int) {
	controller.childPID = pid
}

func (controller *unixSubprocessConformanceController) SetStopMarker(path string) {
	controller.stopMarker = path
}

func (*unixSubprocessConformanceController) Close() error { return nil }

func signalAnalyzerProcessGroup(processGroupID int, value syscall.Signal) error {
	err := syscall.Kill(-processGroupID, value)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func signalAnalyzerProcess(pid int, value syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(pid, value)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func analyzerUnixProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}

func ignoreAnalyzerTerminateSignals() {
	signal.Ignore(syscall.SIGTERM)
}

func conformanceCancellationRequiresKill() bool { return false }
