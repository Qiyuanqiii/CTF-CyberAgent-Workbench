//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package runner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type unixConformanceTree struct {
	processGroupID int
	parentPID      int
	childPID       int
}

func startPlatformConformanceTree(ctx context.Context, command *exec.Cmd,
	directory string,
) (conformanceTreeController, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	controller := &unixConformanceTree{
		processGroupID: command.Process.Pid,
		parentPID:      command.Process.Pid,
	}
	cleanup := func() {
		_ = syscall.Kill(-controller.processGroupID, syscall.SIGKILL)
		_ = command.Wait()
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
	controller.childPID = childPID
	return controller, nil
}

func (c *unixConformanceTree) Terminate(ctx context.Context, _ string) error {
	if err := signalUnixProcessGroup(c.processGroupID, syscall.SIGTERM); err != nil {
		return err
	}
	return c.waitReaped(ctx)
}

func (c *unixConformanceTree) Kill(ctx context.Context) error {
	if err := signalUnixProcessGroup(c.processGroupID, syscall.SIGKILL); err != nil {
		return err
	}
	return c.waitReaped(ctx)
}

func (c *unixConformanceTree) Inspect(ctx context.Context) (TreeState, error) {
	if err := ctx.Err(); err != nil {
		return TreeState{}, err
	}
	parentRunning, err := unixProcessAlive(c.parentPID)
	if err != nil {
		return TreeState{}, err
	}
	childRunning, err := unixProcessAlive(c.childPID)
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

func (c *unixConformanceTree) Close() error { return nil }

func (c *unixConformanceTree) waitReaped(ctx context.Context) error {
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

func signalUnixProcessGroup(processGroupID int, signal syscall.Signal) error {
	err := syscall.Kill(-processGroupID, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func unixProcessAlive(pid int) (bool, error) {
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
