package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RunLifecycleControlProtocolVersion = "run_lifecycle_control.v1"
	RunExecutionHandoffProtocolVersion = "run_execution_handoff.v1"
	MaxRunExecutionHandoffSteps        = 8
)

type RunLifecycleAction string

const (
	RunLifecycleStart  RunLifecycleAction = "start"
	RunLifecyclePause  RunLifecycleAction = "pause"
	RunLifecycleResume RunLifecycleAction = "resume"
)

func (a RunLifecycleAction) Valid() bool {
	return a == RunLifecycleStart || a == RunLifecyclePause || a == RunLifecycleResume
}

func (a RunLifecycleAction) Transition() (RunStatus, RunStatus, error) {
	switch a {
	case RunLifecycleStart:
		return RunCreated, RunRunning, nil
	case RunLifecyclePause:
		return RunRunning, RunPaused, nil
	case RunLifecycleResume:
		return RunPaused, RunRunning, nil
	default:
		return "", "", fmt.Errorf("unsupported Run lifecycle action %q", a)
	}
}

type RunLifecycleOperation struct {
	ProtocolVersion    string
	KeyDigest          string
	RequestFingerprint string
	RunID              string
	Action             RunLifecycleAction
	ExpectedStatus     RunStatus
	AppliedStatus      RunStatus
	EventSequenceStart int64
	EventSequenceEnd   int64
	RequestedBy        string
	CreatedAt          time.Time
}

func (o RunLifecycleOperation) Validate() error {
	if o.ProtocolVersion != RunLifecycleControlProtocolVersion {
		return fmt.Errorf("unsupported Run lifecycle protocol %q", o.ProtocolVersion)
	}
	if !validLowerHexDigest(o.KeyDigest) || !validLowerHexDigest(o.RequestFingerprint) {
		return errors.New("Run lifecycle operation digests must be lowercase SHA-256")
	}
	if !ValidAgentID(o.RunID) || !ValidAgentID(o.RequestedBy) ||
		strings.ContainsRune(o.RunID, 0) || strings.ContainsRune(o.RequestedBy, 0) {
		return errors.New("Run lifecycle operation identities must be normalized and bounded")
	}
	expected, applied, err := o.Action.Transition()
	if err != nil || o.ExpectedStatus != expected || o.AppliedStatus != applied {
		return errors.New("Run lifecycle operation transition does not match its action")
	}
	wantEvents := int64(1)
	if o.Action == RunLifecycleStart {
		wantEvents = 2
	}
	if o.EventSequenceStart <= 0 ||
		o.EventSequenceEnd-o.EventSequenceStart+1 != wantEvents {
		return errors.New("Run lifecycle operation event range is invalid")
	}
	if o.CreatedAt.IsZero() {
		return errors.New("Run lifecycle operation creation time is required")
	}
	return nil
}

type RunExecutionHandoffOperation struct {
	ID                 string
	ProtocolVersion    string
	KeyDigest          string
	RequestFingerprint string
	RunID              string
	SessionID          string
	RequestedBy        string
	MaxSteps           int
	SelectedCount      int
	EventSequence      int64
	CreatedAt          time.Time
}

func (o RunExecutionHandoffOperation) Validate() error {
	if o.ProtocolVersion != RunExecutionHandoffProtocolVersion {
		return fmt.Errorf("unsupported Run execution handoff protocol %q", o.ProtocolVersion)
	}
	if !validLowerHexDigest(o.KeyDigest) || !validLowerHexDigest(o.RequestFingerprint) {
		return errors.New("Run execution handoff digests must be lowercase SHA-256")
	}
	for _, value := range []string{o.ID, o.RunID, o.SessionID, o.RequestedBy} {
		if !ValidAgentID(value) || strings.ContainsRune(value, 0) {
			return errors.New("Run execution handoff identities must be normalized and bounded")
		}
	}
	if o.MaxSteps <= 0 || o.MaxSteps > MaxRunExecutionHandoffSteps ||
		o.SelectedCount < 0 || o.SelectedCount > o.MaxSteps || o.EventSequence <= 0 {
		return errors.New("Run execution handoff bounds are invalid")
	}
	if o.CreatedAt.IsZero() {
		return errors.New("Run execution handoff creation time is required")
	}
	return nil
}

type RunExecutionHandoffItem struct {
	OperationID     string
	Ordinal         int
	MessageID       string
	MessageSequence int64
	Prepared        bool
}

func (i RunExecutionHandoffItem) Validate() error {
	if !ValidAgentID(i.OperationID) || !ValidAgentID(i.MessageID) ||
		i.Ordinal <= 0 || i.MessageSequence <= 0 {
		return errors.New("Run execution handoff item is invalid")
	}
	return nil
}

type RunExecutionHandoffStatus string

const (
	RunExecutionHandoffCompleted RunExecutionHandoffStatus = "completed"
	RunExecutionHandoffFailed    RunExecutionHandoffStatus = "failed"
)

func (s RunExecutionHandoffStatus) Valid() bool {
	return s == RunExecutionHandoffCompleted || s == RunExecutionHandoffFailed
}

type RunExecutionHandoffResult struct {
	OperationID             string
	Status                  RunExecutionHandoffStatus
	RunStatus               RunStatus
	StopReason              string
	ErrorCode               string
	StepsCompleted          int
	ModelCalled             bool
	ToolCalled              bool
	PendingCount            int
	PreparedCount           int
	CommittedCount          int
	CancelledCount          int
	CompletionEventSequence int64
	LeaseID                 string
	LeaseGeneration         int64
	CompletedAt             time.Time
}

func (r RunExecutionHandoffResult) Validate(selectedCount int) error {
	if !ValidAgentID(r.OperationID) || !r.Status.Valid() || !ValidRunStatus(r.RunStatus) ||
		r.StopReason != strings.TrimSpace(r.StopReason) || r.StopReason == "" ||
		len(r.StopReason) > 64 || strings.ContainsRune(r.StopReason, 0) ||
		r.ErrorCode != strings.TrimSpace(r.ErrorCode) || len(r.ErrorCode) > 64 ||
		strings.ContainsRune(r.ErrorCode, 0) {
		return errors.New("Run execution handoff result metadata is invalid")
	}
	if (r.Status == RunExecutionHandoffCompleted) != (r.ErrorCode == "") {
		return errors.New("Run execution handoff status and error code are inconsistent")
	}
	if r.ToolCalled && !r.ModelCalled {
		return errors.New("Run execution handoff tool calls require a model call")
	}
	if selectedCount < 0 || r.StepsCompleted < 0 || r.StepsCompleted > selectedCount ||
		r.PendingCount < 0 || r.PreparedCount < 0 || r.CommittedCount < 0 ||
		r.CancelledCount < 0 || r.PendingCount+r.PreparedCount+r.CommittedCount+
		r.CancelledCount != selectedCount || r.CompletionEventSequence <= 0 {
		return errors.New("Run execution handoff result counts are invalid")
	}
	if (r.LeaseID == "") != (r.LeaseGeneration == 0) || r.LeaseGeneration < 0 {
		return errors.New("Run execution handoff result lease identity is inconsistent")
	}
	if selectedCount > 0 && r.LeaseID == "" {
		return errors.New("non-empty Run execution handoff result requires a lease")
	}
	if r.CompletedAt.IsZero() {
		return errors.New("Run execution handoff completion time is required")
	}
	return nil
}

type RunExecutionHandoff struct {
	Operation RunExecutionHandoffOperation
	Items     []RunExecutionHandoffItem
	Result    *RunExecutionHandoffResult
}

func (h RunExecutionHandoff) Validate() error {
	if err := h.Operation.Validate(); err != nil {
		return err
	}
	if len(h.Items) != h.Operation.SelectedCount {
		return errors.New("Run execution handoff item count does not match")
	}
	for index, item := range h.Items {
		if err := item.Validate(); err != nil || item.OperationID != h.Operation.ID ||
			item.Ordinal != index+1 {
			return errors.New("Run execution handoff item ordering is invalid")
		}
	}
	if h.Result != nil {
		if h.Result.OperationID != h.Operation.ID {
			return errors.New("Run execution handoff result binding does not match")
		}
		return h.Result.Validate(h.Operation.SelectedCount)
	}
	return nil
}
