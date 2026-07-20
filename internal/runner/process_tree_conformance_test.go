//go:build windows || aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/waitgraph"
)

const (
	conformanceRoleEnv = "CYBERAGENT_RUNNER_CONFORMANCE_ROLE"
	conformanceDirEnv  = "CYBERAGENT_RUNNER_CONFORMANCE_DIR"
	conformanceModeEnv = "CYBERAGENT_RUNNER_CONFORMANCE_MODE"

	conformanceRoleParent = "parent"
	conformanceRoleChild  = "child"
	conformanceModeHold   = "hold"
	conformanceModeOrphan = "parent-exits"
)

type conformanceTreeController interface {
	Terminate(context.Context, string) error
	Kill(context.Context) error
	Inspect(context.Context) (TreeState, error)
	Close() error
}

type conformanceBackend struct {
	testing         *testing.T
	mode            string
	ignoreTerminate bool
}

type conformanceOutputCollector struct {
	mu       sync.Mutex
	observed int64
	captured []byte
}

func (c *conformanceOutputCollector) Write(value []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	written := len(value)
	if c.observed <= MaxOutputObservedBytes {
		if int64(written) > MaxOutputObservedBytes-c.observed {
			c.observed = MaxOutputObservedBytes + 1
		} else {
			c.observed += int64(written)
		}
	}
	remaining := MaxOutputCaptureBytes - len(c.captured)
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		c.captured = append(c.captured, value...)
	}
	return written, nil
}

func (c *conformanceOutputCollector) Evidence() OutputEvidence {
	c.mu.Lock()
	defer c.mu.Unlock()
	return testOutputEvidence(append([]byte(nil), c.captured...), c.observed)
}

func newPlatformConformanceBackend(t *testing.T, mode string,
	ignoreTerminate bool,
) Backend {
	t.Helper()
	return &conformanceBackend{testing: t, mode: mode, ignoreTerminate: ignoreTerminate}
}

func (b *conformanceBackend) Name() string         { return "os-process-tree-conformance" }
func (b *conformanceBackend) NonProductOnly() bool { return true }

func (b *conformanceBackend) Start(ctx context.Context, request Request) (Process, error) {
	if b == nil || b.testing == nil ||
		(b.mode != conformanceModeHold && b.mode != conformanceModeOrphan) {
		return nil, errors.New("invalid process-tree conformance backend")
	}
	directory := b.testing.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestProcessTreeConformanceHelper$")
	command.Env = conformanceEnvironment(directory, conformanceRoleParent, b.mode)
	command.Stdin = nil
	stdout := &conformanceOutputCollector{}
	stderr := &conformanceOutputCollector{}
	startedAt := time.Now()
	command.Stdout = stdout
	command.Stderr = stderr
	controller, err := startPlatformConformanceTree(ctx, command, directory)
	if err != nil {
		return nil, err
	}
	process := &conformanceProcess{
		identity: request.ID + "-process-tree", command: command, controller: controller,
		stopMarker: filepath.Join(directory, "stop"), ignoreTerminate: b.ignoreTerminate,
		stdout: stdout, stderr: stderr, done: make(chan struct{}), startedAt: startedAt,
	}
	go process.reap()
	b.testing.Cleanup(process.forceCleanup)
	return process, nil
}

type conformanceProcess struct {
	identity        string
	command         *exec.Cmd
	controller      conformanceTreeController
	stopMarker      string
	ignoreTerminate bool
	stdout          *conformanceOutputCollector
	stderr          *conformanceOutputCollector
	done            chan struct{}
	closeOnce       sync.Once

	mu          sync.Mutex
	exitCode    int
	waitErr     error
	startedAt   time.Time
	completedAt time.Time
}

func (p *conformanceProcess) Identity() string { return p.identity }

func (p *conformanceProcess) reap() {
	err := p.command.Wait()
	p.mu.Lock()
	p.completedAt = time.Now()
	if p.command.ProcessState != nil && p.command.ProcessState.Exited() {
		p.exitCode = p.command.ProcessState.ExitCode()
	} else {
		p.waitErr = err
	}
	p.mu.Unlock()
	close(p.done)
}

func (p *conformanceProcess) RuntimeEvidence(ctx context.Context) (RuntimeEvidence, error) {
	select {
	case <-p.done:
	case <-ctx.Done():
		return RuntimeEvidence{}, ctx.Err()
	}
	p.mu.Lock()
	waitErr := p.waitErr
	startedAt, completedAt := p.startedAt, p.completedAt
	processState := p.command.ProcessState
	p.mu.Unlock()
	if waitErr != nil || processState == nil || completedAt.Before(startedAt) {
		return RuntimeEvidence{}, errors.New("process runtime evidence is unavailable")
	}
	state, err := p.controller.Inspect(ctx)
	if err != nil {
		return RuntimeEvidence{}, err
	}
	return RuntimeEvidence{ProtocolVersion: RuntimeEvidenceProtocolVersion,
		TreeReaped: state.Reaped,
		Stdin:      StdinEvidence{ContentSHA256: EmptyOutputSHA256, Closed: true},
		Descriptors: DescriptorEvidence{StandardInputClosed: true,
			StandardOutputCaptured: true, StandardErrorCaptured: true},
		Resources: ResourceEvidence{
			WallTimeMilliseconds:            completedAt.Sub(startedAt).Milliseconds(),
			ParentUserCPUTimeMilliseconds:   processState.UserTime().Milliseconds(),
			ParentSystemCPUTimeMilliseconds: processState.SystemTime().Milliseconds(),
		},
		MetadataOnly: true, EnvironmentIncluded: false,
		DescriptorIdentityIncluded: false, ProductExecutionEnabled: false,
	}, nil
}

func (p *conformanceProcess) Wait(ctx context.Context) (ExitStatus, error) {
	select {
	case <-p.done:
		p.mu.Lock()
		exitCode, waitErr := p.exitCode, p.waitErr
		p.mu.Unlock()
		if waitErr != nil {
			return ExitStatus{}, waitErr
		}
		state, err := p.controller.Inspect(ctx)
		if err != nil {
			return ExitStatus{}, err
		}
		return ExitStatus{Exited: true, ExitCode: exitCode, Reaped: state.Reaped}, nil
	case <-ctx.Done():
		return ExitStatus{}, ctx.Err()
	}
}

func (p *conformanceProcess) ExitEvidence(ctx context.Context) (ExitEvidence, error) {
	select {
	case <-p.done:
	case <-ctx.Done():
		return ExitEvidence{}, ctx.Err()
	}
	p.mu.Lock()
	exitCode, waitErr := p.exitCode, p.waitErr
	p.mu.Unlock()
	if waitErr != nil {
		return ExitEvidence{}, waitErr
	}
	state, err := p.controller.Inspect(ctx)
	if err != nil {
		return ExitEvidence{}, err
	}
	return testExitEvidence(exitCode, state.Reaped, p.stdout.Evidence(),
		p.stderr.Evidence()), nil
}

func (p *conformanceProcess) TerminateTree(ctx context.Context) error {
	if p.ignoreTerminate {
		return nil
	}
	return p.controller.Terminate(ctx, p.stopMarker)
}

func (p *conformanceProcess) KillTree(ctx context.Context) error {
	return p.controller.Kill(ctx)
}

func (p *conformanceProcess) InspectTree(ctx context.Context) (TreeState, error) {
	return p.controller.Inspect(ctx)
}

func (p *conformanceProcess) forceCleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.controller.Kill(ctx)
	select {
	case <-p.done:
	case <-ctx.Done():
	}
	p.closeOnce.Do(func() { _ = p.controller.Close() })
}

func TestProcessTreeConformanceHelper(t *testing.T) {
	role := os.Getenv(conformanceRoleEnv)
	if role == "" {
		return
	}
	directory := os.Getenv(conformanceDirEnv)
	mode := os.Getenv(conformanceModeEnv)
	if directory == "" || (mode != conformanceModeHold && mode != conformanceModeOrphan) {
		t.Fatal("invalid process-tree conformance helper environment")
	}
	switch role {
	case conformanceRoleChild:
		waitForConformanceMarker(t, filepath.Join(directory, "stop"))
	case conformanceRoleParent:
		waitForConformanceMarker(t, filepath.Join(directory, "assigned"))
		if _, err := fmt.Fprint(os.Stdout, "runner-conformance-stdout\n"); err != nil {
			t.Fatalf("write conformance stdout: %v", err)
		}
		if _, err := fmt.Fprint(os.Stderr, "runner-conformance-stderr\n"); err != nil {
			t.Fatalf("write conformance stderr: %v", err)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestProcessTreeConformanceHelper$")
		child.Env = conformanceEnvironment(directory, conformanceRoleChild, mode)
		child.Stdin = nil
		child.Stdout = io.Discard
		child.Stderr = io.Discard
		if err := child.Start(); err != nil {
			t.Fatalf("start conformance child: %v", err)
		}
		if err := os.WriteFile(filepath.Join(directory, "child.pid"),
			[]byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
			t.Fatalf("publish conformance child: %v", err)
		}
		if mode == conformanceModeOrphan {
			return
		}
		waitForConformanceMarker(t, filepath.Join(directory, "stop"))
		if err := child.Wait(); err != nil {
			t.Fatalf("wait conformance child: %v", err)
		}
	default:
		t.Fatal("unknown process-tree conformance helper role")
	}
}

func TestProcessTreeConformanceGracefulTerminationReapsDescendants(t *testing.T) {
	harness, err := NewHarness(newPlatformConformanceBackend(t, conformanceModeHold, false))
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "os-graceful-tree", Timeout: time.Second,
		TerminationGrace: 3 * time.Second, KillGrace: 3 * time.Second,
	})
	if !errors.Is(err, context.DeadlineExceeded) || !result.TimedOut ||
		!result.TerminateRequested || result.TerminateFailed || result.KillRequested ||
		result.OrphanDetected || !result.TreeReaped || result.ProductExecutionEnabled {
		t.Fatalf("graceful process-tree result=%#v err=%v", result, err)
	}
	assertConformanceEvidence(t, result)
}

func TestProcessTreeConformanceForcedKillReapsDescendants(t *testing.T) {
	harness, err := NewHarness(newPlatformConformanceBackend(t, conformanceModeHold, true))
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "os-forced-tree", Timeout: time.Second,
		TerminationGrace: 150 * time.Millisecond, KillGrace: 3 * time.Second,
	})
	if !errors.Is(err, context.DeadlineExceeded) || !result.TimedOut ||
		!result.TerminateRequested || !result.KillRequested || result.KillFailed ||
		result.OrphanDetected || !result.TreeReaped || result.ProductExecutionEnabled {
		t.Fatalf("forced process-tree result=%#v err=%v", result, err)
	}
	assertConformanceEvidence(t, result)
}

func TestProcessTreeConformanceCleansChildAfterParentExit(t *testing.T) {
	harness, err := NewHarness(newPlatformConformanceBackend(t, conformanceModeOrphan, false))
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "os-orphan-tree", Timeout: 5 * time.Second,
		TerminationGrace: 3 * time.Second, KillGrace: 3 * time.Second,
	})
	if !errors.Is(err, ErrOrphanedProcess) || !result.OrphanDetected ||
		result.StopReason != StopOrphanAfterExit || !result.TerminateRequested ||
		result.TerminateFailed || result.KillFailed || !result.TreeReaped ||
		result.ProductExecutionEnabled {
		t.Fatalf("orphan process-tree result=%#v err=%v", result, err)
	}
	assertConformanceEvidence(t, result)
}

func assertConformanceEvidence(t *testing.T, result Result) {
	t.Helper()
	if !result.ExitEvidenceAvailable || !result.RuntimeEvidenceAvailable ||
		!result.ResourceLimitEvidenceAvailable || !result.TerminationCauseEvidenceAvailable ||
		!result.LifecycleTimelineEvidenceAvailable || !result.DeadlineBudgetEvidenceAvailable ||
		!result.EvidenceSetReceiptAvailable ||
		result.RawOutputIncluded || result.OutputTruncated ||
		result.ExitEvidence.ProtocolVersion != ExitEvidenceProtocolVersion ||
		!result.ExitEvidence.MetadataOnly || result.ExitEvidence.RawOutputIncluded ||
		result.ExitEvidence.ProductExecutionEnabled ||
		result.ExitEvidence.Stdout.ObservedBytes == 0 ||
		result.ExitEvidence.Stderr.ObservedBytes == 0 ||
		result.ExitEvidence.Stdout.CapturedBytes != int(result.ExitEvidence.Stdout.ObservedBytes) ||
		result.ExitEvidence.Stderr.CapturedBytes != int(result.ExitEvidence.Stderr.ObservedBytes) ||
		result.RuntimeEvidence.ProtocolVersion != RuntimeEvidenceProtocolVersion ||
		!result.RuntimeEvidence.TreeReaped || !result.RuntimeEvidence.MetadataOnly ||
		result.RuntimeEvidence.EnvironmentIncluded ||
		result.RuntimeEvidence.DescriptorIdentityIncluded ||
		result.RuntimeEvidence.ProductExecutionEnabled ||
		!result.RuntimeEvidence.Stdin.Closed || result.RuntimeEvidence.Stdin.Inherited ||
		result.RuntimeEvidence.Stdin.RawInputIncluded ||
		!result.RuntimeEvidence.Descriptors.StandardInputClosed ||
		!result.RuntimeEvidence.Descriptors.StandardOutputCaptured ||
		!result.RuntimeEvidence.Descriptors.StandardErrorCaptured ||
		result.RuntimeEvidence.Descriptors.ExtraDescriptorCount != 0 ||
		result.RuntimeEvidence.Descriptors.InheritedDescriptorCount != 0 ||
		result.RuntimeEvidence.Resources.RawTelemetryIncluded ||
		result.RuntimeEvidence.Resources.NetworkTelemetryIncluded ||
		result.ResourceLimitEvidence.ProtocolVersion != ResourceLimitEvidenceProtocolVersion ||
		!result.ResourceLimitEvidence.WallDeadlineConfigured ||
		result.ResourceLimitEvidence.CPUTimeLimitConfigured ||
		result.ResourceLimitEvidence.MemoryLimitConfigured ||
		result.ResourceLimitEvidence.OSResourceLimitsVerified ||
		!result.ResourceLimitEvidence.MetadataOnly ||
		result.ResourceLimitEvidence.ProductExecutionEnabled ||
		result.TerminationCauseEvidence.ProtocolVersion != TerminationCauseEvidenceProtocolVersion ||
		result.TerminationCauseEvidence.FinalMechanism != terminationMechanism(result) ||
		result.TerminationCauseEvidence.OSCauseInferred ||
		result.TerminationCauseEvidence.SignalIdentityIncluded ||
		!result.TerminationCauseEvidence.MetadataOnly ||
		result.TerminationCauseEvidence.ProductExecutionEnabled ||
		result.LifecycleTimelineEvidence.ProtocolVersion !=
			LifecycleTimelineEvidenceProtocolVersion ||
		result.LifecycleTimelineEvidence.validate(result) != nil ||
		result.LifecycleTimelineEvidence.WallClockIncluded ||
		result.LifecycleTimelineEvidence.BackendCallTimingInferred ||
		result.LifecycleTimelineEvidence.ProcessIdentityIncluded ||
		result.DeadlineBudgetEvidence.ProtocolVersion != DeadlineBudgetEvidenceProtocolVersion ||
		!result.DeadlineBudgetEvidence.GoContextDeadlinesConfigured ||
		!result.DeadlineBudgetEvidence.IndependentContextBudgets ||
		result.DeadlineBudgetEvidence.CumulativeWallDeadlineClaimed ||
		result.DeadlineBudgetEvidence.CPUTimeLimitClaimed ||
		result.DeadlineBudgetEvidence.MemoryLimitClaimed ||
		result.DeadlineBudgetEvidence.OSResourceLimitsVerified ||
		!result.DeadlineBudgetEvidence.MetadataOnly ||
		result.DeadlineBudgetEvidence.ProductExecutionEnabled ||
		result.EvidenceSetReceipt.validate(result.ExitEvidence, result.RuntimeEvidence,
			result.ResourceLimitEvidence, result.TerminationCauseEvidence,
			result.LifecycleTimelineEvidence, result.DeadlineBudgetEvidence) != nil ||
		result.EvidenceSetReceipt.ProductExecutionEnabled {
		t.Fatalf("process-tree output evidence widened or was incomplete: %#v", result)
	}
}

func conformanceEnvironment(directory string, role string, mode string) []string {
	prefixes := []string{conformanceRoleEnv + "=", conformanceDirEnv + "=",
		conformanceModeEnv + "="}
	result := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		discard := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(value, prefix) {
				discard = true
				break
			}
		}
		if !discard {
			result = append(result, value)
		}
	}
	return append(result, conformanceRoleEnv+"="+role, conformanceDirEnv+"="+directory,
		conformanceModeEnv+"="+mode)
}

func waitForConformanceMarker(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("inspect conformance marker: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for conformance marker %q", filepath.Base(path))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForConformanceChild(ctx context.Context, directory string) (int, error) {
	path := filepath.Join(directory, "child.pid")
	for {
		content, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(content)))
			if parseErr == nil && pid > 0 {
				return pid, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("read conformance child identity: %w", err)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func writeConformanceMarker(path string) error {
	return os.WriteFile(path, []byte("ready\n"), 0o600)
}
