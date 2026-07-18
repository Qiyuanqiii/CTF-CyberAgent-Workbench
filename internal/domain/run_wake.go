package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RunWakeIntentProtocolVersion      = "run_wake_intent.v1"
	RunWakeControlProtocolVersion     = "run_wake_control.v1"
	RunWakeConsumerProtocolVersion    = "run_wake_consumer.v1"
	RunWakeConsumptionProtocolVersion = "run_wake_consumption.v1"
	MaxRunWakeAttempts                = 8
	MaxRunWakeInitialDelaySeconds     = 3600
	MinRunWakeBackoffSeconds          = 5
	MaxRunWakeBackoffSeconds          = 6 * 60 * 60
	MinRunWakeElapsedSeconds          = 60
	MaxRunWakeElapsedSeconds          = 24 * 60 * 60
	RunWakeLeaseSeconds               = 30
)

type RunWakeStatus string

const (
	RunWakeQueued    RunWakeStatus = "queued"
	RunWakeLeased    RunWakeStatus = "leased"
	RunWakeCancelled RunWakeStatus = "cancelled"
	RunWakeExhausted RunWakeStatus = "exhausted"
	RunWakeCompleted RunWakeStatus = "completed"
)

func (s RunWakeStatus) Valid() bool {
	switch s {
	case RunWakeQueued, RunWakeLeased, RunWakeCancelled, RunWakeExhausted, RunWakeCompleted:
		return true
	default:
		return false
	}
}

type RunWakeConsumptionStatus string

const (
	RunWakeConsumptionPrepared  RunWakeConsumptionStatus = "prepared"
	RunWakeConsumptionCompleted RunWakeConsumptionStatus = "completed"
	RunWakeConsumptionFailed    RunWakeConsumptionStatus = "failed"
)

func (s RunWakeConsumptionStatus) Valid() bool {
	return s == RunWakeConsumptionPrepared || s == RunWakeConsumptionCompleted ||
		s == RunWakeConsumptionFailed
}

// RunWakeConsumption binds one wake generation to exactly one bounded
// RunExecutionHandoff. It contains ownership metadata for store fencing, but
// is never projected to browser clients verbatim.
type RunWakeConsumption struct {
	ID                        string
	ProtocolVersion           string
	IntentID                  string
	RunID                     string
	SessionID                 string
	LeaseID                   string
	Generation                int
	OwnerID                   string
	HandoffOperationKeyDigest string
	MaxSteps                  int
	Status                    RunWakeConsumptionStatus
	HandoffOperationID        string
	StopReason                string
	ErrorCode                 string
	PreparedEventSequence     int64
	CompletionEventSequence   int64
	CreatedAt                 time.Time
	CompletedAt               *time.Time
}

func (c RunWakeConsumption) Validate() error {
	if c.ProtocolVersion != RunWakeConsumptionProtocolVersion || !c.Status.Valid() {
		return errors.New("Run wake consumption protocol or status is invalid")
	}
	for _, value := range []string{c.ID, c.IntentID, c.RunID, c.SessionID, c.LeaseID, c.OwnerID} {
		if !ValidAgentID(value) || strings.ContainsRune(value, 0) {
			return errors.New("Run wake consumption identities must be normalized and bounded")
		}
	}
	if !validLowerHexDigest(c.HandoffOperationKeyDigest) || c.Generation < 1 ||
		c.Generation > MaxRunWakeAttempts || c.MaxSteps < 1 ||
		c.MaxSteps > MaxRunExecutionHandoffSteps || c.PreparedEventSequence <= 0 ||
		c.CreatedAt.IsZero() {
		return errors.New("Run wake consumption bounds are invalid")
	}
	if c.Status == RunWakeConsumptionPrepared {
		if c.HandoffOperationID != "" || c.StopReason != "" || c.ErrorCode != "" ||
			c.CompletionEventSequence != 0 || c.CompletedAt != nil {
			return errors.New("prepared Run wake consumption cannot contain a result")
		}
		return nil
	}
	if c.CompletedAt == nil || c.CompletedAt.Before(c.CreatedAt) ||
		c.CompletionEventSequence <= 0 || strings.TrimSpace(c.StopReason) == "" ||
		len(c.StopReason) > 64 || strings.ContainsRune(c.StopReason, 0) ||
		len(c.ErrorCode) > 64 || strings.ContainsRune(c.ErrorCode, 0) {
		return errors.New("terminal Run wake consumption result is invalid")
	}
	if c.Status == RunWakeConsumptionCompleted {
		if c.HandoffOperationID == "" || c.ErrorCode != "" {
			return errors.New("completed Run wake consumption requires a successful handoff")
		}
	} else if c.ErrorCode == "" {
		return errors.New("failed Run wake consumption requires an error code")
	}
	if c.HandoffOperationID != "" &&
		(!ValidAgentID(c.HandoffOperationID) || strings.ContainsRune(c.HandoffOperationID, 0)) {
		return errors.New("Run wake handoff operation identity is invalid")
	}
	return nil
}

type RunWakeIntent struct {
	ID                    string
	ProtocolVersion       string
	RunID                 string
	SessionID             string
	Status                RunWakeStatus
	MaxAttempts           int
	AttemptCount          int
	InitialDelaySeconds   int
	BaseBackoffSeconds    int
	MaxBackoffSeconds     int
	MaxElapsedSeconds     int
	NextWakeAt            time.Time
	DeadlineAt            time.Time
	ActiveLeaseID         string
	ExecutionEnabled      bool
	BackgroundLoopEnabled bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CancelledAt           *time.Time
}

func (i RunWakeIntent) Validate() error {
	if i.ProtocolVersion != RunWakeIntentProtocolVersion || !i.Status.Valid() {
		return errors.New("Run wake intent protocol or status is invalid")
	}
	for _, value := range []string{i.ID, i.RunID, i.SessionID} {
		if !ValidAgentID(value) || strings.ContainsRune(value, 0) {
			return errors.New("Run wake intent identities must be normalized and bounded")
		}
	}
	if i.MaxAttempts < 1 || i.MaxAttempts > MaxRunWakeAttempts ||
		i.AttemptCount < 0 || i.AttemptCount > i.MaxAttempts ||
		i.InitialDelaySeconds < 0 || i.InitialDelaySeconds > MaxRunWakeInitialDelaySeconds ||
		i.BaseBackoffSeconds < MinRunWakeBackoffSeconds ||
		i.BaseBackoffSeconds > MaxRunWakeBackoffSeconds ||
		i.MaxBackoffSeconds < i.BaseBackoffSeconds ||
		i.MaxBackoffSeconds > MaxRunWakeBackoffSeconds ||
		i.MaxElapsedSeconds < MinRunWakeElapsedSeconds ||
		i.MaxElapsedSeconds > MaxRunWakeElapsedSeconds {
		return errors.New("Run wake intent bounds are invalid")
	}
	if i.CreatedAt.IsZero() || i.UpdatedAt.Before(i.CreatedAt) ||
		i.NextWakeAt.Before(i.CreatedAt) || i.NextWakeAt.After(i.DeadlineAt) ||
		!i.DeadlineAt.Equal(i.CreatedAt.Add(time.Duration(i.MaxElapsedSeconds)*time.Second)) {
		return errors.New("Run wake intent timeline is invalid")
	}
	if (i.Status == RunWakeLeased) != (i.ActiveLeaseID != "") ||
		(i.ActiveLeaseID != "" && (!ValidAgentID(i.ActiveLeaseID) ||
			strings.ContainsRune(i.ActiveLeaseID, 0))) {
		return errors.New("Run wake intent active lease binding is invalid")
	}
	if i.ExecutionEnabled || i.BackgroundLoopEnabled {
		return errors.New("Run wake intent cannot enable background execution")
	}
	if i.Status == RunWakeCancelled {
		if i.CancelledAt == nil || i.CancelledAt.Before(i.CreatedAt) ||
			!i.CancelledAt.Equal(i.UpdatedAt) {
			return errors.New("cancelled Run wake intent requires its exact cancellation time")
		}
	} else if i.CancelledAt != nil {
		return errors.New("non-cancelled Run wake intent cannot have a cancellation time")
	}
	return nil
}

type RunWakeAction string

const (
	RunWakeSchedule RunWakeAction = "schedule"
	RunWakeCancel   RunWakeAction = "cancel"
)

func (a RunWakeAction) Valid() bool {
	return a == RunWakeSchedule || a == RunWakeCancel
}

type RunWakeOperation struct {
	ProtocolVersion    string
	KeyDigest          string
	RequestFingerprint string
	Action             RunWakeAction
	IntentID           string
	RunID              string
	RequestedBy        string
	EventSequence      int64
	CreatedAt          time.Time
}

func (o RunWakeOperation) Validate() error {
	if o.ProtocolVersion != RunWakeControlProtocolVersion || !o.Action.Valid() ||
		!validLowerHexDigest(o.KeyDigest) || !validLowerHexDigest(o.RequestFingerprint) {
		return errors.New("Run wake operation protocol, action, or digest is invalid")
	}
	for _, value := range []string{o.IntentID, o.RunID, o.RequestedBy} {
		if !ValidAgentID(value) || strings.ContainsRune(value, 0) {
			return errors.New("Run wake operation identities must be normalized and bounded")
		}
	}
	if o.EventSequence <= 0 || o.CreatedAt.IsZero() {
		return errors.New("Run wake operation event sequence and creation time are required")
	}
	return nil
}

type RunWakeLeaseStatus string

const (
	RunWakeLeaseActive   RunWakeLeaseStatus = "active"
	RunWakeLeaseReleased RunWakeLeaseStatus = "released"
	RunWakeLeaseRevoked  RunWakeLeaseStatus = "revoked"
	RunWakeLeaseExpired  RunWakeLeaseStatus = "expired"
)

func (s RunWakeLeaseStatus) Valid() bool {
	switch s {
	case RunWakeLeaseActive, RunWakeLeaseReleased, RunWakeLeaseRevoked, RunWakeLeaseExpired:
		return true
	default:
		return false
	}
}

type RunWakeLease struct {
	ID         string
	IntentID   string
	Generation int
	OwnerID    string
	Status     RunWakeLeaseStatus
	AcquiredAt time.Time
	ExpiresAt  time.Time
	EndedAt    *time.Time
}

func (l RunWakeLease) Validate() error {
	for _, value := range []string{l.ID, l.IntentID, l.OwnerID} {
		if !ValidAgentID(value) || strings.ContainsRune(value, 0) {
			return errors.New("Run wake lease identities must be normalized and bounded")
		}
	}
	if !l.Status.Valid() || l.Generation < 1 || l.Generation > MaxRunWakeAttempts ||
		l.AcquiredAt.IsZero() || !l.ExpiresAt.After(l.AcquiredAt) ||
		l.ExpiresAt.After(l.AcquiredAt.Add(RunWakeLeaseSeconds*time.Second)) {
		return errors.New("Run wake lease bounds are invalid")
	}
	if l.Status == RunWakeLeaseActive {
		if l.EndedAt != nil {
			return errors.New("active Run wake lease cannot have an end time")
		}
	} else if l.EndedAt == nil || l.EndedAt.Before(l.AcquiredAt) {
		return fmt.Errorf("%s Run wake lease requires an end time", l.Status)
	}
	return nil
}
