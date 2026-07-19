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
	stopMarker     string
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
		stopMarker:     filepath.Join(directory, "stop"),
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

func (c *unixConformanceTree) Terminate(ctx context.Context, stopMarker string) error {
	if stopMarker != c.stopMarker {
		return errors.New("process-tree conformance stop marker mismatch")
	}
	if err := writeConformanceMarker(c.stopMarker); err != nil {
		return err
	}
	return c.waitReaped(ctx)
}

func (c *unixConformanceTree) Kill(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Kill the descendant first, then release the parent so it can wait and
	// reap that child before exiting. Killing the whole group simultaneously
	// can leave an adopted zombie that kill(pid, 0) correctly still observes.
	if err := signalUnixProcess(c.childPID, syscall.SIGKILL); err != nil {
		return err
	}
	if err := writeConformanceMarker(c.stopMarker); err != nil {
		return errors.Join(err,
			signalUnixProcessGroup(c.processGroupID, syscall.SIGKILL))
	}
	if err := c.waitReaped(ctx); err != nil {
		return errors.Join(err,
			signalUnixProcessGroup(c.processGroupID, syscall.SIGKILL))
	}
	return nil
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

func signalUnixProcess(pid int, signal syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(pid, signal)
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
