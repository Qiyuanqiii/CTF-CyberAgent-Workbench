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
	LifecycleProtocolVersion                 = "runner_lifecycle_contract.v1"
	ExitEvidenceProtocolVersion              = "runner_exit_evidence.v1"
	RuntimeEvidenceProtocolVersion           = "runner_runtime_evidence.v1"
	ResourceLimitEvidenceProtocolVersion     = "runner_resource_limit_evidence.v1"
	TerminationCauseEvidenceProtocolVersion  = "runner_termination_cause_evidence.v1"
	LifecycleTimelineEvidenceProtocolVersion = "runner_lifecycle_timeline_evidence.v1"
	DeadlineBudgetEvidenceProtocolVersion    = "runner_deadline_budget_evidence.v1"
	DefaultRunTimeout                        = 30 * time.Second
	MaxRunTimeout                            = 5 * time.Minute
	DefaultTerminationGrace                  = 2 * time.Second
	DefaultKillGrace                         = 2 * time.Second
	MaxControlGrace                          = 10 * time.Second
	MaxOutputCaptureBytes                    = 64 * 1024
	MaxOutputObservedBytes                   = 64 * 1024 * 1024
	MaxStdinProvidedBytes                    = 1024 * 1024
	MaxRuntimeEvidenceMilliseconds           = int64((MaxRunTimeout + 2*MaxControlGrace) / time.Millisecond)
	MaxCPUTimeEvidenceMilliseconds           = int64((24 * time.Hour) / time.Millisecond)
	MaxPeakResidentBytes                     = int64(1 << 40)
	EmptyOutputSHA256                        = "e3b0c44298fc1c149afbf4c8996fb924" +
		"27ae41e4649b934ca495991b7852b855"
)

var (
	ErrHarnessBoundary           = errors.New("runner lifecycle backend is not non-product-only")
	ErrStartFailed               = errors.New("runner process start failed")
	ErrWaitFailed                = errors.New("runner process wait failed")
	ErrTerminateFailed           = errors.New("runner process-tree termination failed")
	ErrKillFailed                = errors.New("runner process-tree kill failed")
	ErrOrphanedProcess           = errors.New("runner process tree was not fully reaped")
	ErrExitEvidence              = errors.New("runner exit evidence is invalid")
	ErrRuntimeEvidence           = errors.New("runner runtime evidence is invalid")
	ErrResourceLimitEvidence     = errors.New("runner resource limit evidence is invalid")
	ErrTerminationCauseEvidence  = errors.New("runner termination cause evidence is invalid")
	ErrLifecycleTimelineEvidence = errors.New("runner lifecycle timeline evidence is invalid")
	ErrDeadlineBudgetEvidence    = errors.New("runner deadline budget evidence is invalid")
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

type ResourceLimitEvidence struct {
	ProtocolVersion              string
	RunTimeoutMilliseconds       int64
	TerminationGraceMilliseconds int64
	KillGraceMilliseconds        int64
	WallDeadlineConfigured       bool
	CPUTimeLimitConfigured       bool
	MemoryLimitConfigured        bool
	OSResourceLimitsVerified     bool
	MetadataOnly                 bool
	ProductExecutionEnabled      bool
}

func (e ResourceLimitEvidence) validate(request Request) error {
	if e.ProtocolVersion != ResourceLimitEvidenceProtocolVersion ||
		e.RunTimeoutMilliseconds != request.Timeout.Milliseconds() ||
		e.TerminationGraceMilliseconds != request.TerminationGrace.Milliseconds() ||
		e.KillGraceMilliseconds != request.KillGrace.Milliseconds() ||
		e.RunTimeoutMilliseconds < 1 ||
		e.RunTimeoutMilliseconds > MaxRunTimeout.Milliseconds() ||
		e.TerminationGraceMilliseconds < 1 ||
		e.TerminationGraceMilliseconds > MaxControlGrace.Milliseconds() ||
		e.KillGraceMilliseconds < 1 ||
		e.KillGraceMilliseconds > MaxControlGrace.Milliseconds() ||
		!e.WallDeadlineConfigured || e.CPUTimeLimitConfigured ||
		e.MemoryLimitConfigured || e.OSResourceLimitsVerified || !e.MetadataOnly ||
		e.ProductExecutionEnabled {
		return errors.New("runner resource limit evidence binding is invalid")
	}
	return nil
}

type TerminationControlTrigger string

const (
	TerminationTriggerProcessExit         TerminationControlTrigger = "process_exit"
	TerminationTriggerCallerCancelled     TerminationControlTrigger = "caller_cancelled"
	TerminationTriggerRunDeadline         TerminationControlTrigger = "run_deadline"
	TerminationTriggerWaitFailure         TerminationControlTrigger = "wait_failure"
	TerminationTriggerOrphanAfterExit     TerminationControlTrigger = "orphan_after_exit"
	TerminationTriggerPartialStartFailure TerminationControlTrigger = "partial_start_failure"
)

type TerminationFinalMechanism string

const (
	TerminationMechanismWait      TerminationFinalMechanism = "wait"
	TerminationMechanismTerminate TerminationFinalMechanism = "terminate"
	TerminationMechanismKill      TerminationFinalMechanism = "kill"
)

type TerminationCauseEvidence struct {
	ProtocolVersion         string
	ControlTrigger          TerminationControlTrigger
	FinalMechanism          TerminationFinalMechanism
	Exited                  bool
	ExitCode                int
	TreeReaped              bool
	TimedOut                bool
	Cancelled               bool
	OrphanDetected          bool
	TerminateRequested      bool
	TerminateFailed         bool
	KillRequested           bool
	KillFailed              bool
	OSCauseInferred         bool
	SignalIdentityIncluded  bool
	MetadataOnly            bool
	ProductExecutionEnabled bool
}

// LifecycleTimelineEvidence is a canonical order of observed control facts.
// Sequence numbers do not claim wall-clock timing or backend call duration.
type LifecycleTimelineEvidence struct {
	ProtocolVersion              string
	ControlTrigger               TerminationControlTrigger
	FinalMechanism               TerminationFinalMechanism
	StartAcceptedSequence        int
	StopTriggerSequence          int
	TerminateRequestedSequence   int
	KillRequestedSequence        int
	TreeReapedSequence           int
	ExitEvidenceSequence         int
	RuntimeEvidenceSequence      int
	EvidenceSetCommittedSequence int
	StepCount                    int
	LogicalSequenceOnly          bool
	WallClockIncluded            bool
	BackendCallTimingInferred    bool
	ProcessIdentityIncluded      bool
	MetadataOnly                 bool
	ProductExecutionEnabled      bool
}

func buildLifecycleTimelineEvidence(result Result) (LifecycleTimelineEvidence, bool) {
	trigger, ok := terminationTrigger(result)
	if !ok || !result.Started || !result.TreeReaped {
		return LifecycleTimelineEvidence{}, false
	}
	sequence := 2
	evidence := LifecycleTimelineEvidence{
		ProtocolVersion: LifecycleTimelineEvidenceProtocolVersion,
		ControlTrigger:  trigger, FinalMechanism: terminationMechanism(result),
		StartAcceptedSequence: 1, StopTriggerSequence: 2, LogicalSequenceOnly: true,
		MetadataOnly: true,
	}
	if result.TerminateRequested {
		sequence++
		evidence.TerminateRequestedSequence = sequence
	}
	if result.KillRequested {
		sequence++
		evidence.KillRequestedSequence = sequence
	}
	sequence++
	evidence.TreeReapedSequence = sequence
	sequence++
	evidence.ExitEvidenceSequence = sequence
	sequence++
	evidence.RuntimeEvidenceSequence = sequence
	sequence++
	evidence.EvidenceSetCommittedSequence = sequence
	evidence.StepCount = sequence
	return evidence, true
}

func (e LifecycleTimelineEvidence) validate(result Result) error {
	expected, ok := buildLifecycleTimelineEvidence(result)
	if !ok || e != expected || !e.LogicalSequenceOnly || e.WallClockIncluded ||
		e.BackendCallTimingInferred || e.ProcessIdentityIncluded || !e.MetadataOnly ||
		e.ProductExecutionEnabled {
		return errors.New("runner lifecycle timeline evidence binding is invalid")
	}
	return nil
}

// DeadlineBudgetEvidence records independent Go context ceilings. It does not
// claim a cumulative wall deadline or operating-system resource enforcement.
type DeadlineBudgetEvidence struct {
	ProtocolVersion                          string
	RunContextBudgetMilliseconds             int64
	TerminateCallContextBudgetMilliseconds   int64
	PostTerminateWaitBudgetMilliseconds      int64
	KillCallContextBudgetMilliseconds        int64
	PostKillWaitBudgetMilliseconds           int64
	TreeInspectionContextBudgetMilliseconds  int64
	ExitEvidenceContextBudgetMilliseconds    int64
	RuntimeEvidenceContextBudgetMilliseconds int64
	RunContextApplied                        bool
	TerminateCallContextApplied              bool
	PostTerminateWaitContextApplied          bool
	KillCallContextApplied                   bool
	PostKillWaitContextApplied               bool
	TreeInspectionContextApplied             bool
	ExitEvidenceContextApplied               bool
	RuntimeEvidenceContextApplied            bool
	GoContextDeadlinesConfigured             bool
	IndependentContextBudgets                bool
	CumulativeWallDeadlineClaimed            bool
	CPUTimeLimitClaimed                      bool
	MemoryLimitClaimed                       bool
	OSResourceLimitsVerified                 bool
	MetadataOnly                             bool
	ProductExecutionEnabled                  bool
}

func buildDeadlineBudgetEvidence(request Request, result Result) DeadlineBudgetEvidence {
	return DeadlineBudgetEvidence{
		ProtocolVersion:                          DeadlineBudgetEvidenceProtocolVersion,
		RunContextBudgetMilliseconds:             request.Timeout.Milliseconds(),
		TerminateCallContextBudgetMilliseconds:   request.TerminationGrace.Milliseconds(),
		PostTerminateWaitBudgetMilliseconds:      request.TerminationGrace.Milliseconds(),
		KillCallContextBudgetMilliseconds:        request.KillGrace.Milliseconds(),
		PostKillWaitBudgetMilliseconds:           request.KillGrace.Milliseconds(),
		TreeInspectionContextBudgetMilliseconds:  request.KillGrace.Milliseconds(),
		ExitEvidenceContextBudgetMilliseconds:    request.KillGrace.Milliseconds(),
		RuntimeEvidenceContextBudgetMilliseconds: request.KillGrace.Milliseconds(),
		RunContextApplied:                        true,
		TerminateCallContextApplied:              result.TerminateRequested,
		PostTerminateWaitContextApplied:          result.TerminateRequested,
		KillCallContextApplied:                   result.KillRequested,
		PostKillWaitContextApplied:               result.KillRequested,
		TreeInspectionContextApplied:             true, ExitEvidenceContextApplied: true,
		RuntimeEvidenceContextApplied: true, GoContextDeadlinesConfigured: true,
		IndependentContextBudgets: true, MetadataOnly: true,
	}
}

func (e DeadlineBudgetEvidence) validate(request Request, result Result) error {
	expected := buildDeadlineBudgetEvidence(request, result)
	if !result.Started || !result.TreeReaped || e != expected ||
		e.RunContextBudgetMilliseconds < 1 ||
		e.RunContextBudgetMilliseconds > MaxRunTimeout.Milliseconds() ||
		e.TerminateCallContextBudgetMilliseconds < 1 ||
		e.TerminateCallContextBudgetMilliseconds > MaxControlGrace.Milliseconds() ||
		e.PostTerminateWaitBudgetMilliseconds < 1 ||
		e.PostTerminateWaitBudgetMilliseconds > MaxControlGrace.Milliseconds() ||
		e.KillCallContextBudgetMilliseconds < 1 ||
		e.KillCallContextBudgetMilliseconds > MaxControlGrace.Milliseconds() ||
		e.PostKillWaitBudgetMilliseconds < 1 ||
		e.PostKillWaitBudgetMilliseconds > MaxControlGrace.Milliseconds() ||
		e.TreeInspectionContextBudgetMilliseconds < 1 ||
		e.TreeInspectionContextBudgetMilliseconds > MaxControlGrace.Milliseconds() ||
		e.ExitEvidenceContextBudgetMilliseconds < 1 ||
		e.ExitEvidenceContextBudgetMilliseconds > MaxControlGrace.Milliseconds() ||
		e.RuntimeEvidenceContextBudgetMilliseconds < 1 ||
		e.RuntimeEvidenceContextBudgetMilliseconds > MaxControlGrace.Milliseconds() ||
		!e.GoContextDeadlinesConfigured || !e.IndependentContextBudgets ||
		e.CumulativeWallDeadlineClaimed || e.CPUTimeLimitClaimed || e.MemoryLimitClaimed ||
		e.OSResourceLimitsVerified || !e.MetadataOnly || e.ProductExecutionEnabled {
		return errors.New("runner deadline budget evidence binding is invalid")
	}
	return nil
}

func (e TerminationCauseEvidence) validate(status ExitStatus, result Result) error {
	trigger, ok := terminationTrigger(result)
	mechanism := terminationMechanism(result)
	if !ok || e.ProtocolVersion != TerminationCauseEvidenceProtocolVersion ||
		e.ControlTrigger != trigger || e.FinalMechanism != mechanism ||
		!e.Exited || !status.Exited || e.ExitCode != status.ExitCode ||
		!e.TreeReaped || e.TreeReaped != status.Reaped || e.TreeReaped != result.TreeReaped ||
		e.TimedOut != result.TimedOut || e.Cancelled != result.Cancelled ||
		e.OrphanDetected != result.OrphanDetected ||
		e.TerminateRequested != result.TerminateRequested ||
		e.TerminateFailed != result.TerminateFailed ||
		e.KillRequested != result.KillRequested || e.KillFailed != result.KillFailed ||
		(result.TerminateFailed && !result.TerminateRequested) ||
		(result.KillFailed && !result.KillRequested) ||
		(mechanism == TerminationMechanismWait &&
			(result.TerminateRequested || result.KillRequested)) ||
		(mechanism == TerminationMechanismTerminate &&
			(!result.TerminateRequested || result.KillRequested)) ||
		(mechanism == TerminationMechanismKill &&
			(!result.TerminateRequested || !result.KillRequested)) ||
		e.OSCauseInferred || e.SignalIdentityIncluded || !e.MetadataOnly ||
		e.ProductExecutionEnabled {
		return errors.New("runner termination cause evidence binding is invalid")
	}
	return nil
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
	ProtocolVersion                    string
	RequestID                          string
	Backend                            string
	Started                            bool
	ExitCode                           int
	StopReason                         StopReason
	Cancelled                          bool
	TimedOut                           bool
	TerminateRequested                 bool
	TerminateFailed                    bool
	KillRequested                      bool
	KillFailed                         bool
	OrphanDetected                     bool
	TreeReaped                         bool
	ExitEvidenceAvailable              bool
	ExitEvidence                       ExitEvidence
	RuntimeEvidenceAvailable           bool
	RuntimeEvidence                    RuntimeEvidence
	ResourceLimitEvidenceAvailable     bool
	ResourceLimitEvidence              ResourceLimitEvidence
	TerminationCauseEvidenceAvailable  bool
	TerminationCauseEvidence           TerminationCauseEvidence
	LifecycleTimelineEvidenceAvailable bool
	LifecycleTimelineEvidence          LifecycleTimelineEvidence
	DeadlineBudgetEvidenceAvailable    bool
	DeadlineBudgetEvidence             DeadlineBudgetEvidence
	OutputTruncated                    bool
	RawOutputIncluded                  bool
	ProductExecutionEnabled            bool
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
				result.StopReason = StopExited
				if evidenceErr := h.collectEvidence(ctx, process, exit,
					normalized, &result); evidenceErr != nil {
					result.StopReason = StopEvidenceFailed
					return result, evidenceErr
				}
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
				request, result); evidenceErr != nil {
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
		request, result); evidenceErr != nil {
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
	status ExitStatus, request Request, result *Result,
) error {
	exitCtx, exitCancel := context.WithTimeout(context.WithoutCancel(parent), request.KillGrace)
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
	runtimeCtx, runtimeCancel := context.WithTimeout(context.WithoutCancel(parent), request.KillGrace)
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
	resourceLimitEvidence := ResourceLimitEvidence{
		ProtocolVersion:              ResourceLimitEvidenceProtocolVersion,
		RunTimeoutMilliseconds:       request.Timeout.Milliseconds(),
		TerminationGraceMilliseconds: request.TerminationGrace.Milliseconds(),
		KillGraceMilliseconds:        request.KillGrace.Milliseconds(),
		WallDeadlineConfigured:       true, MetadataOnly: true,
	}
	if err := resourceLimitEvidence.validate(request); err != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: contract mismatch", ErrResourceLimitEvidence)
	}
	trigger, ok := terminationTrigger(*result)
	if !ok {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: control trigger is unavailable", ErrTerminationCauseEvidence)
	}
	terminationCauseEvidence := TerminationCauseEvidence{
		ProtocolVersion: TerminationCauseEvidenceProtocolVersion,
		ControlTrigger:  trigger, FinalMechanism: terminationMechanism(*result),
		Exited: status.Exited, ExitCode: status.ExitCode, TreeReaped: status.Reaped,
		TimedOut: result.TimedOut, Cancelled: result.Cancelled,
		OrphanDetected:     result.OrphanDetected,
		TerminateRequested: result.TerminateRequested,
		TerminateFailed:    result.TerminateFailed, KillRequested: result.KillRequested,
		KillFailed: result.KillFailed, MetadataOnly: true,
	}
	if err := terminationCauseEvidence.validate(status, *result); err != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: contract mismatch", ErrTerminationCauseEvidence)
	}
	lifecycleTimelineEvidence, ok := buildLifecycleTimelineEvidence(*result)
	if !ok || lifecycleTimelineEvidence.validate(*result) != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: contract mismatch", ErrLifecycleTimelineEvidence)
	}
	deadlineBudgetEvidence := buildDeadlineBudgetEvidence(request, *result)
	if err := deadlineBudgetEvidence.validate(request, *result); err != nil {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: contract mismatch", ErrDeadlineBudgetEvidence)
	}
	if result.ExitEvidenceAvailable && result.ExitEvidence != exitEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrExitEvidence)
	}
	if result.RuntimeEvidenceAvailable && result.RuntimeEvidence != runtimeEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrRuntimeEvidence)
	}
	if result.ResourceLimitEvidenceAvailable &&
		result.ResourceLimitEvidence != resourceLimitEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrResourceLimitEvidence)
	}
	if result.TerminationCauseEvidenceAvailable &&
		result.TerminationCauseEvidence != terminationCauseEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrTerminationCauseEvidence)
	}
	if result.LifecycleTimelineEvidenceAvailable &&
		result.LifecycleTimelineEvidence != lifecycleTimelineEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrLifecycleTimelineEvidence)
	}
	if result.DeadlineBudgetEvidenceAvailable &&
		result.DeadlineBudgetEvidence != deadlineBudgetEvidence {
		result.StopReason = StopEvidenceFailed
		return fmt.Errorf("%w: evidence changed after collection", ErrDeadlineBudgetEvidence)
	}
	result.ExitEvidence = exitEvidence
	result.ExitEvidenceAvailable = true
	result.RuntimeEvidence = runtimeEvidence
	result.RuntimeEvidenceAvailable = true
	result.ResourceLimitEvidence = resourceLimitEvidence
	result.ResourceLimitEvidenceAvailable = true
	result.TerminationCauseEvidence = terminationCauseEvidence
	result.TerminationCauseEvidenceAvailable = true
	result.LifecycleTimelineEvidence = lifecycleTimelineEvidence
	result.LifecycleTimelineEvidenceAvailable = true
	result.DeadlineBudgetEvidence = deadlineBudgetEvidence
	result.DeadlineBudgetEvidenceAvailable = true
	result.OutputTruncated = exitEvidence.Stdout.Truncated || exitEvidence.Stderr.Truncated
	result.RawOutputIncluded = false
	return nil
}

func terminationTrigger(result Result) (TerminationControlTrigger, bool) {
	switch result.StopReason {
	case StopExited:
		return TerminationTriggerProcessExit, !result.Cancelled && !result.TimedOut &&
			!result.OrphanDetected && !result.TerminateRequested && !result.KillRequested
	case StopCancelled:
		return TerminationTriggerCallerCancelled, result.Cancelled && !result.TimedOut
	case StopTimedOut:
		return TerminationTriggerRunDeadline, result.TimedOut && !result.Cancelled
	case StopWaitFailed:
		return TerminationTriggerWaitFailure, !result.TimedOut && !result.Cancelled
	case StopOrphanAfterExit:
		return TerminationTriggerOrphanAfterExit, result.OrphanDetected &&
			!result.Cancelled && !result.TimedOut
	case StopStartFailed:
		return TerminationTriggerPartialStartFailure, result.Started &&
			!result.Cancelled && !result.TimedOut && !result.OrphanDetected
	default:
		return "", false
	}
}

func terminationMechanism(result Result) TerminationFinalMechanism {
	if result.KillRequested {
		return TerminationMechanismKill
	}
	if result.TerminateRequested {
		return TerminationMechanismTerminate
	}
	return TerminationMechanismWait
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
