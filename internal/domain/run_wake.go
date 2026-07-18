package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RunWakeIntentProtocolVersion  = "run_wake_intent.v1"
	RunWakeControlProtocolVersion = "run_wake_control.v1"
	MaxRunWakeAttempts            = 8
	MaxRunWakeInitialDelaySeconds = 3600
	MinRunWakeBackoffSeconds      = 5
	MaxRunWakeBackoffSeconds      = 6 * 60 * 60
	MinRunWakeElapsedSeconds      = 60
	MaxRunWakeElapsedSeconds      = 24 * 60 * 60
	RunWakeLeaseSeconds           = 30
)

type RunWakeStatus string

const (
	RunWakeQueued    RunWakeStatus = "queued"
	RunWakeLeased    RunWakeStatus = "leased"
	RunWakeCancelled RunWakeStatus = "cancelled"
	RunWakeExhausted RunWakeStatus = "exhausted"
)

func (s RunWakeStatus) Valid() bool {
	switch s {
	case RunWakeQueued, RunWakeLeased, RunWakeCancelled, RunWakeExhausted:
		return true
	default:
		return false
	}
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
