package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/waitgraph"
)

type simulationBackend struct {
	name           string
	process        *simulationProcess
	startErr       error
	startCount     int
	nonProductOnly bool
}

func (b *simulationBackend) Name() string {
	if b.name != "" {
		return b.name
	}
	return "deterministic-harness"
}
func (b *simulationBackend) NonProductOnly() bool { return b.nonProductOnly }
func (b *simulationBackend) Start(ctx context.Context, _ Request) (Process, error) {
	b.startCount++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.startErr != nil {
		return b.process, b.startErr
	}
	return b.process, nil
}

type simulationProcess struct {
	mu                 sync.Mutex
	done               chan struct{}
	doneOnce           sync.Once
	running            bool
	descendants        int
	reaped             bool
	exitCode           int
	exitOnTerminate    bool
	exitOnKill         bool
	terminateErr       error
	killErr            error
	terminateCount     int
	killCount          int
	waitCount          int
	inspectCount       int
	identity           string
	stdoutEvidence     OutputEvidence
	stderrEvidence     OutputEvidence
	evidenceErr        error
	runtimeEvidence    RuntimeEvidence
	runtimeEvidenceErr error
}

func newSimulationProcess() *simulationProcess {
	return &simulationProcess{done: make(chan struct{}), running: true,
		exitOnKill: true, exitCode: 137,
		stdoutEvidence:  testOutputEvidence(nil, 0),
		stderrEvidence:  testOutputEvidence(nil, 0),
		runtimeEvidence: testRuntimeEvidence()}
}

func (p *simulationProcess) Identity() string {
	if p.identity != "" {
		return p.identity
	}
	return "simulated-process-tree"
}

func (p *simulationProcess) Wait(ctx context.Context) (ExitStatus, error) {
	p.mu.Lock()
	p.waitCount++
	done := p.done
	p.mu.Unlock()
	select {
	case <-done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return ExitStatus{Exited: true, ExitCode: p.exitCode, Reaped: p.reaped}, nil
	case <-ctx.Done():
		return ExitStatus{}, ctx.Err()
	}
}

func (p *simulationProcess) ExitEvidence(ctx context.Context) (ExitEvidence, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ExitEvidence{}, err
	}
	if p.running || !p.reaped {
		return ExitEvidence{}, errors.New("simulated process tree is not reaped")
	}
	if p.evidenceErr != nil {
		return ExitEvidence{}, p.evidenceErr
	}
	return testExitEvidence(p.exitCode, p.reaped, p.stdoutEvidence, p.stderrEvidence), nil
}

func (p *simulationProcess) RuntimeEvidence(ctx context.Context) (RuntimeEvidence, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return RuntimeEvidence{}, err
	}
	if p.running || !p.reaped {
		return RuntimeEvidence{}, errors.New("simulated process tree is not reaped")
	}
	if p.runtimeEvidenceErr != nil {
		return RuntimeEvidence{}, p.runtimeEvidenceErr
	}
	return p.runtimeEvidence, nil
}

func (p *simulationProcess) TerminateTree(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.terminateCount++
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.terminateErr != nil {
		return p.terminateErr
	}
	if p.exitOnTerminate {
		p.finishLocked(143, true)
	}
	return nil
}

func (p *simulationProcess) KillTree(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.killCount++
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.killErr != nil {
		return p.killErr
	}
	if p.exitOnKill {
		p.finishLocked(137, true)
	}
	return nil
}

func (p *simulationProcess) InspectTree(ctx context.Context) (TreeState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCount++
	if err := ctx.Err(); err != nil {
		return TreeState{}, err
	}
	return TreeState{ParentRunning: p.running, LiveDescendants: p.descendants,
		Reaped: p.reaped}, nil
}

func (p *simulationProcess) finish(exitCode int, reaped bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finishLocked(exitCode, reaped)
}

func (p *simulationProcess) finishLocked(exitCode int, reaped bool) {
	p.running = false
	p.exitCode = exitCode
	p.reaped = reaped
	if reaped {
		p.descendants = 0
	}
	p.doneOnce.Do(func() { close(p.done) })
}

func testOutputEvidence(captured []byte, observed int64) OutputEvidence {
	digest := sha256.Sum256(captured)
	return OutputEvidence{ObservedBytes: observed, CapturedBytes: len(captured),
		CapturedPrefixSHA256: hex.EncodeToString(digest[:]),
		Truncated:            observed > int64(len(captured)), RawOutputIncluded: false}
}

func testExitEvidence(exitCode int, reaped bool, stdout OutputEvidence,
	stderr OutputEvidence,
) ExitEvidence {
	return ExitEvidence{ProtocolVersion: ExitEvidenceProtocolVersion, Exited: true,
		ExitCode: exitCode, Reaped: reaped, Stdout: stdout, Stderr: stderr,
		MetadataOnly: true, RawOutputIncluded: false, ProductExecutionEnabled: false}
}

func testRuntimeEvidence() RuntimeEvidence {
	return RuntimeEvidence{ProtocolVersion: RuntimeEvidenceProtocolVersion, TreeReaped: true,
		Stdin: StdinEvidence{ContentSHA256: EmptyOutputSHA256, Closed: true},
		Descriptors: DescriptorEvidence{StandardInputClosed: true,
			StandardOutputCaptured: true, StandardErrorCaptured: true},
		Resources: ResourceEvidence{WallTimeMilliseconds: 1}, MetadataOnly: true,
		EnvironmentIncluded: false, DescriptorIdentityIncluded: false,
		ProductExecutionEnabled: false}
}

func TestLifecycleHarnessNormalExitRequiresReapedTree(t *testing.T) {
	process := newSimulationProcess()
	process.finish(0, true)
	backend := &simulationBackend{process: process, nonProductOnly: true}
	harness, err := NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{ID: "normal-exit"})
	if err != nil || !result.Started || result.StopReason != StopExited || result.ExitCode != 0 ||
		!result.TreeReaped || result.TerminateRequested || result.KillRequested ||
		!result.ExitEvidenceAvailable || !result.RuntimeEvidenceAvailable ||
		result.RawOutputIncluded ||
		result.ProductExecutionEnabled {
		t.Fatalf("normal lifecycle result=%#v err=%v", result, err)
	}
}

func TestLifecycleHarnessRejectsRuntimeEvidenceThatWidensMetadataBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimeEvidence)
	}{
		{name: "inherited descriptor", mutate: func(e *RuntimeEvidence) {
			e.Descriptors.InheritedDescriptorCount = 1
		}},
		{name: "raw stdin", mutate: func(e *RuntimeEvidence) {
			e.Stdin.RawInputIncluded = true
		}},
		{name: "negative resource", mutate: func(e *RuntimeEvidence) {
			e.Resources.WallTimeMilliseconds = -1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			process := newSimulationProcess()
			test.mutate(&process.runtimeEvidence)
			process.finish(0, true)
			harness, err := NewHarness(&simulationBackend{process: process, nonProductOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(),
				Request{ID: "invalid-runtime-evidence"})
			if !errors.Is(err, ErrRuntimeEvidence) || result.StopReason != StopEvidenceFailed ||
				!result.TreeReaped || result.ExitEvidenceAvailable ||
				result.RuntimeEvidenceAvailable || result.ProductExecutionEnabled {
				t.Fatalf("invalid runtime evidence result=%#v err=%v", result, err)
			}
		})
	}
}

func TestLifecycleHarnessRequiresStableRepeatedEvidence(t *testing.T) {
	process := newSimulationProcess()
	process.finish(0, true)
	harness, err := NewHarness(&simulationBackend{process: process, nonProductOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	result := Result{}
	status := ExitStatus{Exited: true, ExitCode: 0, Reaped: true}
	if err := harness.collectEvidence(t.Context(), process, status, time.Second, &result); err != nil {
		t.Fatal(err)
	}
	if err := harness.collectEvidence(t.Context(), process, status, time.Second, &result); err != nil {
		t.Fatalf("stable repeated evidence: %v", err)
	}
	original := result.RuntimeEvidence
	process.mu.Lock()
	process.runtimeEvidence.Resources.WallTimeMilliseconds++
	process.mu.Unlock()
	if err := harness.collectEvidence(t.Context(), process, status, time.Second, &result); !errors.Is(err, ErrRuntimeEvidence) || result.StopReason != StopEvidenceFailed ||
		result.RuntimeEvidence != original {
		t.Fatalf("changed repeated evidence result=%#v err=%v", result, err)
	}
}

func TestLifecycleHarnessBoundsOutputAndRejectsInvalidExitEvidence(t *testing.T) {
	process := newSimulationProcess()
	prefix := bytes.Repeat([]byte("x"), MaxOutputCaptureBytes)
	process.stdoutEvidence = testOutputEvidence(prefix, MaxOutputCaptureBytes+17)
	process.stderrEvidence = testOutputEvidence([]byte("bounded stderr\n"), 15)
	process.finish(17, true)
	harness, err := NewHarness(&simulationBackend{process: process, nonProductOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(),
		Request{ID: "bounded-output-evidence"})
	if err != nil || !result.ExitEvidenceAvailable || !result.OutputTruncated ||
		result.RawOutputIncluded || result.ExitEvidence.ExitCode != 17 ||
		result.ExitEvidence.Stdout.ObservedBytes != MaxOutputCaptureBytes+17 ||
		result.ExitEvidence.Stdout.CapturedBytes != MaxOutputCaptureBytes ||
		!result.ExitEvidence.Stdout.Truncated ||
		result.ExitEvidence.Stdout.CapturedPrefixSHA256 == EmptyOutputSHA256 {
		t.Fatalf("bounded output evidence result=%#v err=%v", result, err)
	}

	invalid := newSimulationProcess()
	invalid.stdoutEvidence.RawOutputIncluded = true
	invalid.finish(0, true)
	harness, err = NewHarness(&simulationBackend{process: invalid, nonProductOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err = harness.WithWaitGraph(waitgraph.New()).Run(t.Context(),
		Request{ID: "invalid-output-evidence"})
	if !errors.Is(err, ErrExitEvidence) || result.StopReason != StopEvidenceFailed ||
		!result.TreeReaped || result.OrphanDetected || result.TerminateRequested ||
		result.KillRequested || result.ExitEvidenceAvailable || result.RawOutputIncluded {
		t.Fatalf("invalid output evidence result=%#v err=%v", result, err)
	}
}

func TestLifecycleHarnessTimeoutEscalatesTerminateToKillAndReapsTree(t *testing.T) {
	process := newSimulationProcess()
	backend := &simulationBackend{process: process, nonProductOnly: true}
	harness, err := NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "timeout-kill", Timeout: 5 * time.Millisecond,
		TerminationGrace: 5 * time.Millisecond, KillGrace: 20 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) || !result.TimedOut || result.Cancelled ||
		!result.TerminateRequested || !result.KillRequested || !result.TreeReaped ||
		result.OrphanDetected || process.terminateCount != 1 || process.killCount != 1 {
		t.Fatalf("timeout lifecycle result=%#v process=%#v err=%v", result, process, err)
	}
}

func TestLifecycleHarnessCancellationUsesIndependentCleanupContext(t *testing.T) {
	process := newSimulationProcess()
	process.exitOnTerminate = true
	backend := &simulationBackend{process: process, nonProductOnly: true}
	harness, err := NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(ctx, Request{
		ID: "cancel-cleanup", Timeout: time.Second,
		TerminationGrace: 20 * time.Millisecond, KillGrace: 20 * time.Millisecond,
	})
	if !errors.Is(err, context.Canceled) || !result.Cancelled || result.TimedOut ||
		!result.TerminateRequested || result.KillRequested || !result.TreeReaped ||
		process.terminateCount != 1 {
		t.Fatalf("cancel lifecycle result=%#v process=%#v err=%v", result, process, err)
	}
}

func TestLifecycleHarnessFlagsAndCleansDescendantsAfterParentExit(t *testing.T) {
	process := newSimulationProcess()
	process.mu.Lock()
	process.descendants = 2
	process.finishLocked(0, false)
	process.mu.Unlock()
	backend := &simulationBackend{process: process, nonProductOnly: true}
	harness, err := NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "orphan-cleanup", TerminationGrace: 5 * time.Millisecond,
		KillGrace: 20 * time.Millisecond,
	})
	if !errors.Is(err, ErrOrphanedProcess) || !result.OrphanDetected ||
		result.StopReason != StopOrphanAfterExit || !result.KillRequested || !result.TreeReaped ||
		process.killCount != 1 {
		t.Fatalf("orphan cleanup result=%#v process=%#v err=%v", result, process, err)
	}
}

func TestLifecycleHarnessFailsClosedWhenKillLeavesLiveTree(t *testing.T) {
	process := newSimulationProcess()
	process.exitOnKill = false
	backend := &simulationBackend{process: process, nonProductOnly: true}
	harness, err := NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "stuck-tree", Timeout: 5 * time.Millisecond,
		TerminationGrace: 5 * time.Millisecond, KillGrace: 5 * time.Millisecond,
	})
	if !errors.Is(err, ErrOrphanedProcess) || !result.OrphanDetected || result.TreeReaped ||
		!result.KillRequested {
		t.Fatalf("stuck lifecycle result=%#v err=%v", result, err)
	}
}

func TestLifecycleHarnessNeverStartsAcrossClosedBoundaries(t *testing.T) {
	process := newSimulationProcess()
	backend := &simulationBackend{process: process, nonProductOnly: false}
	if _, err := NewHarness(backend); !errors.Is(err, ErrHarnessBoundary) {
		t.Fatalf("product-like backend was accepted: %v", err)
	}
	backend.nonProductOnly = true
	harness, err := NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(cancelled, Request{ID: "pre-cancel"})
	if !errors.Is(err, context.Canceled) || result.Started || backend.startCount != 0 {
		t.Fatalf("pre-cancelled harness started: result=%#v count=%d err=%v",
			result, backend.startCount, err)
	}
	graph := waitgraph.New()
	release, err := graph.Acquire(waitgraph.Runner("cycle-target"),
		waitgraph.External("runner-lifecycle-harness"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result, err = harness.WithWaitGraph(graph).Run(t.Context(), Request{ID: "cycle-target"})
	if !errors.Is(err, waitgraph.ErrCycle) || result.Started ||
		result.StopReason != StopDependencyRefused || backend.startCount != 0 {
		t.Fatalf("wait cycle reached backend: result=%#v count=%d err=%v",
			result, backend.startCount, err)
	}
}

func TestLifecycleHarnessCleansPartialStartAndInvalidIdentity(t *testing.T) {
	partial := newSimulationProcess()
	partial.exitOnTerminate = true
	backend := &simulationBackend{process: partial, nonProductOnly: true,
		startErr: errors.New("backend start error")}
	harness, err := NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "partial-start", TerminationGrace: 20 * time.Millisecond,
		KillGrace: 20 * time.Millisecond,
	})
	if !errors.Is(err, ErrStartFailed) || !result.Started || !result.TerminateRequested ||
		!result.TreeReaped || partial.terminateCount != 1 {
		t.Fatalf("partial start leaked: result=%#v process=%#v err=%v", result, partial, err)
	}

	invalid := newSimulationProcess()
	invalid.identity = " invalid-process "
	invalid.exitOnTerminate = true
	backend = &simulationBackend{process: invalid, nonProductOnly: true}
	harness, err = NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err = harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{
		ID: "invalid-identity", TerminationGrace: 20 * time.Millisecond,
		KillGrace: 20 * time.Millisecond,
	})
	if !errors.Is(err, ErrStartFailed) || !result.Started || !result.TerminateRequested ||
		!result.TreeReaped || invalid.terminateCount != 1 {
		t.Fatalf("invalid process identity leaked: result=%#v process=%#v err=%v",
			result, invalid, err)
	}

	backend = &simulationBackend{name: " invalid-backend ", nonProductOnly: true}
	if _, err := NewHarness(backend); !errors.Is(err, ErrHarnessBoundary) {
		t.Fatalf("non-normalized backend identity was accepted: %v", err)
	}
	backend = &simulationBackend{process: newSimulationProcess(), nonProductOnly: true}
	harness, err = NewHarness(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err = harness.WithWaitGraph(waitgraph.New()).Run(t.Context(), Request{ID: " invalid "})
	if err == nil || result.Started || backend.startCount != 0 {
		t.Fatalf("non-normalized request reached backend: result=%#v count=%d err=%v",
			result, backend.startCount, err)
	}
}
