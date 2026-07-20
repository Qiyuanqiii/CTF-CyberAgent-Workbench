package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/waitgraph"
)

const (
	LifecycleProtocolVersion       = "runner_lifecycle_contract.v1"
	ExitEvidenceProtocolVersion    = "runner_exit_evidence.v1"
	RuntimeEvidenceProtocolVersion = "runner_runtime_evidence.v1"
	DefaultRunTimeout              = 30 * time.Second
	MaxRunTimeout                  = 5 * time.Minute
	DefaultTerminationGrace        = 2 * time.Second
	DefaultKillGrace               = 2 * time.Second
	MaxControlGrace                = 10 * time.Second
	MaxOutputCaptureBytes          = 64 * 1024
	MaxOutputObservedBytes         = 64 * 1024 * 1024
	MaxStdinProvidedBytes          = 1024 * 1024
	MaxRuntimeEvidenceMilliseconds = int64((MaxRunTimeout + 2*MaxControlGrace) / time.Millisecond)
	MaxCPUTimeEvidenceMilliseconds = int64((24 * time.Hour) / time.Millisecond)
	MaxPeakResidentBytes           = int64(1 << 40)
	EmptyOutputSHA256              = "e3b0c44298fc1c149afbf4c8996fb924" +
		"27ae41e4649b934ca495991b7852b855"
)

var (
	ErrHarnessBoundary = errors.New("runner lifecycle backend is not non-product-only")
	ErrStartFailed     = errors.New("runner process start failed")
	ErrWaitFailed      = errors.New("runner process wait failed")
	ErrTerminateFailed = errors.New("runner process-tree termination failed")
	ErrKillFailed      = errors.New("runner process-tree kill failed")
	ErrOrphanedProcess = errors.New("runner process tree was not fully reaped")
	ErrExitEvidence    = errors.New("runner exit evidence is invalid")
	ErrRuntimeEvidence = errors.New("runner runtime evidence is invalid")
)

type StopReason string

const (
	StopExited            StopReason = "exited"
	StopCancelled         StopReason = "cancelled"
	StopTimedOut          StopReason = "timed_out"
	StopWaitFailed        StopReason = "wait_failed"
	StopOrphanAfterExit   StopReason = "orphan_after_exit"
	StopStartFailed       StopReason = "start_failed"
	StopDependencyRefused StopReason = "dependency_refused"
	StopEvidenceFailed    StopReason = "evidence_failed"
)

type Request struct {
	ID               string
	Timeout          time.Duration
	TerminationGrace time.Duration
	KillGrace        time.Duration
}

func (r Request) normalize() (Request, error) {
	originalID := r.ID
	r.ID = strings.TrimSpace(r.ID)
	if originalID != r.ID || !validIdentity(r.ID) {
		return Request{}, errors.New("runner lifecycle request identity is invalid")
	}
	if r.Timeout == 0 {
		r.Timeout = DefaultRunTimeout
	}
	if r.TerminationGrace == 0 {
		r.TerminationGrace = DefaultTerminationGrace
	}
	if r.KillGrace == 0 {
		r.KillGrace = DefaultKillGrace
	}
	if r.Timeout < time.Millisecond || r.Timeout > MaxRunTimeout ||
		r.TerminationGrace < time.Millisecond || r.TerminationGrace > MaxControlGrace ||
		r.KillGrace < time.Millisecond || r.KillGrace > MaxControlGrace {
		return Request{}, errors.New("runner lifecycle timeout or grace period is invalid")
	}
	return r, nil
}

type ExitStatus struct {
	Exited   bool
	ExitCode int
	Reaped   bool
}

type OutputEvidence struct {
	ObservedBytes        int64
	CapturedBytes        int
	CapturedPrefixSHA256 string
	Truncated            bool
	RawOutputIncluded    bool
}

func (e OutputEvidence) validate() error {
	if e.ObservedBytes < 0 || e.ObservedBytes > MaxOutputObservedBytes ||
		e.CapturedBytes < 0 || e.CapturedBytes > MaxOutputCaptureBytes ||
		int64(e.CapturedBytes) > e.ObservedBytes || e.RawOutputIncluded ||
		!validSHA256(e.CapturedPrefixSHA256) {
		return errors.New("runner output evidence is invalid")
	}
	expectedCaptured := e.ObservedBytes
	if expectedCaptured > MaxOutputCaptureBytes {
		expectedCaptured = MaxOutputCaptureBytes
	}
	if int64(e.CapturedBytes) != expectedCaptured ||
		e.Truncated != (e.ObservedBytes > MaxOutputCaptureBytes) ||
		(e.CapturedBytes == 0 && e.CapturedPrefixSHA256 != EmptyOutputSHA256) {
		return errors.New("runner output evidence is inconsistent")
	}
	return nil
}

type ExitEvidence struct {
	ProtocolVersion         string
	Exited                  bool
	ExitCode                int
	Reaped                  bool
	Stdout                  OutputEvidence
	Stderr                  OutputEvidence
	MetadataOnly            bool
	RawOutputIncluded       bool
	ProductExecutionEnabled bool
}

type StdinEvidence struct {
	BytesProvided    int64
	ContentSHA256    string
	Closed           bool
	Inherited        bool
	RawInputIncluded bool
}

func (e StdinEvidence) validate() error {
	if e.BytesProvided < 0 || e.BytesProvided > MaxStdinProvidedBytes ||
		!validSHA256(e.ContentSHA256) || !e.Closed || e.Inherited || e.RawInputIncluded ||
		(e.BytesProvided == 0 && e.ContentSHA256 != EmptyOutputSHA256) {
		return errors.New("runner stdin evidence is invalid")
	}
	return nil
}

type DescriptorEvidence struct {
	StandardInputClosed      bool
	StandardOutputCaptured   bool
	StandardErrorCaptured    bool
	ExtraDescriptorCount     int
	InheritedDescriptorCount int
	NamesIncluded            bool
	PathsIncluded            bool
}

func (e DescriptorEvidence) validate() error {
	if !e.StandardInputClosed || !e.StandardOutputCaptured || !e.StandardErrorCaptured ||
		e.ExtraDescriptorCount != 0 || e.InheritedDescriptorCount != 0 ||
		e.NamesIncluded || e.PathsIncluded {
		return errors.New("runner descriptor evidence is invalid")
	}
	return nil
}

type ResourceEvidence struct {
	WallTimeMilliseconds            int64
	ParentUserCPUTimeMilliseconds   int64
	ParentSystemCPUTimeMilliseconds int64
	PeakResidentBytes               int64
	PeakResidentMeasured            bool
	RawTelemetryIncluded            bool
	NetworkTelemetryIncluded        bool
}

func (e ResourceEvidence) validate() error {
	if e.WallTimeMilliseconds < 0 ||
		e.WallTimeMilliseconds > MaxRuntimeEvidenceMilliseconds ||
		e.ParentUserCPUTimeMilliseconds < 0 ||
		e.ParentUserCPUTimeMilliseconds > MaxCPUTimeEvidenceMilliseconds ||
		e.ParentSystemCPUTimeMilliseconds < 0 ||
		e.ParentSystemCPUTimeMilliseconds > MaxCPUTimeEvidenceMilliseconds ||
		e.PeakResidentBytes < 0 || e.PeakResidentBytes > MaxPeakResidentBytes ||
		(e.PeakResidentMeasured != (e.PeakResidentBytes > 0)) ||
		e.RawTelemetryIncluded || e.NetworkTelemetryIncluded {
		return errors.New("runner resource evidence is invalid")
	}
	return nil
}

type RuntimeEvidence struct {
	ProtocolVersion            string
	TreeReaped                 bool
	Stdin                      StdinEvidence
	Descriptors                DescriptorEvidence
	Resources                  ResourceEvidence
	MetadataOnly               bool
	EnvironmentIncluded        bool
	DescriptorIdentityIncluded bool
	ProductExecutionEnabled    bool
}

func (e RuntimeEvidence) validate(status ExitStatus) error {
	if e.ProtocolVersion != RuntimeEvidenceProtocolVersion || !e.TreeReaped ||
		e.TreeReaped != status.Reaped || !e.MetadataOnly || e.EnvironmentIncluded ||
		e.DescriptorIdentityIncluded || e.ProductExecutionEnabled {
		return errors.New("runner runtime evidence binding is invalid")
	}
	if err := e.Stdin.validate(); err != nil {
		return err
	}
	if err := e.Descriptors.validate(); err != nil {
		return err
	}
	return e.Resources.validate()
}

func (e ExitEvidence) validate(status ExitStatus) error {
	if e.ProtocolVersion != ExitEvidenceProtocolVersion || !e.Exited ||
		e.ExitCode != status.ExitCode || e.Reaped != status.Reaped ||
		!e.MetadataOnly || e.RawOutputIncluded || e.ProductExecutionEnabled {
		return errors.New("runner exit evidence binding is invalid")
	}
	if err := e.Stdout.validate(); err != nil {
		return err
	}
	return e.Stderr.validate()
}

func (s ExitStatus) validate() error {
	if !s.Exited {
		return errors.New("runner wait returned without an exited process")
	}
	return nil
}

type TreeState struct {
	ParentRunning   bool
	LiveDescendants int
	Reaped          bool
}

func (s TreeState) validate() error {
	if s.LiveDescendants < 0 || s.LiveDescendants > 1_000_000 {
		return errors.New("runner process-tree count is invalid")
	}
	if s.Reaped && (s.ParentRunning || s.LiveDescendants != 0) {
		return errors.New("runner process-tree reaped state is inconsistent")
	}
	return nil
}

type Process interface {
	Identity() string
	Wait(context.Context) (ExitStatus, error)
	ExitEvidence(context.Context) (ExitEvidence, error)
	RuntimeEvidence(context.Context) (RuntimeEvidence, error)
	TerminateTree(context.Context) error
	KillTree(context.Context) error
	InspectTree(context.Context) (TreeState, error)
}

// Backend is intentionally narrower than a product Runner. It accepts only
// deterministic simulations or test-only conformance adapters, so implementing
// this interface cannot enable Local or Docker execution in a product path.
type Backend interface {
	Name() string
	NonProductOnly() bool
	Start(context.Context, Request) (Process, error)
}

type Result struct {
	ProtocolVersion          string
	RequestID                string
	Backend                  string
	Started                  bool
	ExitCode                 int
	StopReason               StopReason
	Cancelled                bool
	TimedOut                 bool
	TerminateRequested       bool
	TerminateFailed          bool
	KillRequested            bool
	KillFailed               bool
	OrphanDetected           bool
	TreeReaped               bool
	ExitEvidenceAvailable    bool
	ExitEvidence             ExitEvidence
	RuntimeEvidenceAvailable bool
	RuntimeEvidence          RuntimeEvidence
	OutputTruncated          bool
	RawOutputIncluded        bool
	ProductExecutionEnabled  bool
}

type Harness struct {
	backend   Backend
	waitGraph *waitgraph.Graph
}

func NewHarness(backend Backend) (*Harness, error) {
	if backend == nil {
		return nil, ErrHarnessBoundary
	}
	name := backend.Name()
	if name != strings.TrimSpace(name) || !validIdentity(name) ||
		!backend.NonProductOnly() {
		return nil, ErrHarnessBoundary
	}
	return &Harness{backend: backend, waitGraph: waitgraph.Default()}, nil
}

func (h *Harness) WithWaitGraph(graph *waitgraph.Graph) *Harness {
	if h == nil {
		return nil
	}
	copy := *h
	copy.waitGraph = graph
	return &copy
}

func (h *Harness) Run(ctx context.Context, request Request) (Result, error) {
	result := Result{ProtocolVersion: LifecycleProtocolVersion,
		ProductExecutionEnabled: false}
	if h == nil || h.backend == nil || h.waitGraph == nil || !h.backend.NonProductOnly() {
		return result, ErrHarnessBoundary
	}
	backendName := h.backend.Name()
	if backendName != strings.TrimSpace(backendName) || !validIdentity(backendName) {
		return result, ErrHarnessBoundary
	}
	normalized, err := request.normalize()
	if err != nil {
		return result, err
	}
	result.RequestID = normalized.ID
	result.Backend = backendName
	if ctx == nil {
		return result, errors.New("runner lifecycle context is required")
	}
	if err := ctx.Err(); err != nil {
		result.Cancelled = errors.Is(err, context.Canceled)
		result.TimedOut = errors.Is(err, context.DeadlineExceeded)
		if result.TimedOut {
			result.StopReason = StopTimedOut
		} else {
			result.StopReason = StopCancelled
		}
		return result, err
	}
	runnerNode := waitgraph.Runner(normalized.ID)
	runCtx, release, err := waitgraph.Enter(ctx, h.waitGraph,
		waitgraph.External("runner-lifecycle-harness"), runnerNode)
	if err != nil {
		result.StopReason = StopDependencyRefused
		return result, err
	}
	defer release()
	runCtx, cancel := context.WithTimeout(runCtx, normalized.Timeout)
	defer cancel()
	process, err := h.backend.Start(runCtx, normalized)
	if err != nil {
		result.StopReason = StopStartFailed
		if process != nil {
			result.Started = true
			if cleanupErr := h.cleanup(ctx, process, normalized, &result); cleanupErr != nil {
				return result, fmt.Errorf("%w: partial start cleanup failed", errors.Join(
					ErrStartFailed, cleanupErr))
			}
		}
		return result, fmt.Errorf("%w: %v", ErrStartFailed, stableContextError(err))
	}
	if process == nil {
		result.StopReason = StopStartFailed
		return result, fmt.Errorf("%w: missing process handle", ErrStartFailed)
	}
	result.Started = true
	processIdentity := process.Identity()
	if processIdentity != strings.TrimSpace(processIdentity) || !validIdentity(processIdentity) {
		result.StopReason = StopStartFailed
		if cleanupErr := h.cleanup(ctx, process, normalized, &result); cleanupErr != nil {
			return result, fmt.Errorf("%w: invalid process identity cleanup failed", errors.Join(
				ErrStartFailed, cleanupErr))
		}
		return result, fmt.Errorf("%w: invalid process identity", ErrStartFailed)
	}
	exit, waitErr := process.Wait(runCtx)
	if waitErr == nil {
		if err := exit.validate(); err != nil {
			waitErr = err
		} else {
			result.ExitCode = exit.ExitCode
			safe, inspectErr := h.inspectReaped(ctx, process, normalized.KillGrace, &result)
			if inspectErr == nil && safe && exit.Reaped {
				if evidenceErr := h.collectEvidence(ctx, process, exit,
					normalized.KillGrace, &result); evidenceErr != nil {
					result.StopReason = StopEvidenceFailed
					return result, evidenceErr
				}
				result.StopReason = StopExited
				return result, nil
			}
			result.OrphanDetected = true
			result.StopReason = StopOrphanAfterExit
			cleanupErr := h.cleanup(ctx, process, normalized, &result)
			if cleanupErr != nil {
				return result, cleanupErr
			}
			return result, ErrOrphanedProcess
		}
	}
	result.StopReason = StopWaitFailed
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.StopReason = StopTimedOut
	} else if errors.Is(ctx.Err(), context.Canceled) {
		result.Cancelled = true
		result.StopReason = StopCancelled
	} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.StopReason = StopTimedOut
	}
	cleanupErr := h.cleanup(ctx, process, normalized, &result)
	if cleanupErr != nil {
		return result, cleanupErr
	}
	if result.StopReason == StopWaitFailed {
		return result, fmt.Errorf("%w: %v", ErrWaitFailed, stableContextError(waitErr))
	}
	if result.Cancelled {
		return result, context.Canceled
	}
	if result.TimedOut {
		return result, context.DeadlineExceeded
	}
	return result, nil
}

func (h *Harness) cleanup(parent context.Context, process Process, request Request,
	result *Result,
) error {
	base := context.WithoutCancel(parent)
	result.TerminateRequested = true
	terminateCtx, terminateCancel := context.WithTimeout(base, request.TerminationGrace)
	terminateErr := process.TerminateTree(terminateCtx)
	terminateCancel()
	result.TerminateFailed = terminateErr != nil
	waitCtx, waitCancel := context.WithTimeout(base, request.TerminationGrace)
	exit, waitErr := process.Wait(waitCtx)
	waitCancel()
	if waitErr == nil && exit.validate() == nil {
		result.ExitCode = exit.ExitCode
		if safe, inspectErr := h.inspectReaped(base, process, request.KillGrace, result); inspectErr == nil && safe && exit.Reaped {
			if evidenceErr := h.collectEvidence(base, process, exit,
				request.KillGrace, result); evidenceErr != nil {
				return evidenceErr
			}
			if terminateErr != nil {
				return fmt.Errorf("%w: backend returned an error after reaping", ErrTerminateFailed)
			}
			return nil
		}
	}
	result.KillRequested = true
	killCtx, killCancel := context.WithTimeout(base, request.KillGrace)
	killErr := process.KillTree(killCtx)
	killCancel()
	result.KillFailed = killErr != nil
	waitCtx, waitCancel = context.WithTimeout(base, request.KillGrace)
	exit, waitErr = process.Wait(waitCtx)
	waitCancel()
	if waitErr == nil && exit.validate() == nil {
		result.ExitCode = exit.ExitCode
	}
	safe, inspectErr := h.inspectReaped(base, process, request.KillGrace, result)
	if inspectErr != nil || !safe || waitErr != nil || exit.validate() != nil || !exit.Reaped {
		result.OrphanDetected = true
		if killErr != nil {
			return fmt.Errorf("%w: %v", ErrKillFailed, stableContextError(killErr))
		}
		return ErrOrphanedProcess
	}
	if evidenceErr := h.collectEvidence(base, process, exit,
		request.KillGrace, result); evidenceErr != nil {
		return evidenceErr
	}
	if terminateErr != nil {
		return fmt.Errorf("%w: backend returned an error before successful kill", ErrTerminateFailed)
	}
	if killErr != nil {
		return fmt.Errorf("%w: backend returned an error after reaping", ErrKillFailed)
	}
	return nil
}

func (h *Harness) collectEvidence(parent context.Context, process Process,
	status ExitStatus, timeout time.Duration, result *Result,
) error {
	exitCtx, exitCancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	exitEvidence, err := process.ExitEvidence(exitCtx)
	exitCancel()
	if err != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: %v", ErrExitEvidence, stableContextError(err))
	}
	if err := exitEvidence.validate(status); err != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: contract mismatch", ErrExitEvidence)
	}
	runtimeCtx, runtimeCancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	runtimeEvidence, err := process.RuntimeEvidence(runtimeCtx)
	runtimeCancel()
	if err != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: %v", ErrRuntimeEvidence, stableContextError(err))
	}
	if err := runtimeEvidence.validate(status); err != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: contract mismatch", ErrRuntimeEvidence)
	}
	if result.ExitEvidenceAvailable && result.ExitEvidence != exitEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrExitEvidence)
	}
	if result.RuntimeEvidenceAvailable && result.RuntimeEvidence != runtimeEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrRuntimeEvidence)
	}
	result.ExitEvidence = exitEvidence
	result.ExitEvidenceAvailable = true
	result.RuntimeEvidence = runtimeEvidence
	result.RuntimeEvidenceAvailable = true
	result.OutputTruncated = exitEvidence.Stdout.Truncated || exitEvidence.Stderr.Truncated
	result.RawOutputIncluded = false
	return nil
}

func (h *Harness) inspectReaped(parent context.Context, process Process,
	timeout time.Duration, result *Result,
) (bool, error) {
	inspectCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	defer cancel()
	state, err := process.InspectTree(inspectCtx)
	if err != nil {
		return false, err
	}
	if err := state.validate(); err != nil {
		return false, err
	}
	safe := !state.ParentRunning && state.LiveDescendants == 0 && state.Reaped
	result.TreeReaped = safe
	return safe, nil
}

func stableContextError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return errors.New("backend lifecycle operation failed")
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validIdentity(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > 256 || strings.ContainsRune(value, 0) {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return false
		}
	}
	return true
}
