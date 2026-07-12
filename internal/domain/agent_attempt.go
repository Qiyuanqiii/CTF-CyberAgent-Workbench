package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	AgentAttemptProtocolVersion        = "agent_attempt.v1"
	AgentAttemptFailureProtocolVersion = AgentAttemptFailureVersion
	MaxAgentFailureCodeBytes           = 64
	MaxAgentFailureReasonRunes         = 4096
	MaxAgentFailureReasonBytes         = 8 * 1024
)

type AgentAttemptStatus string

const (
	AgentAttemptRunning     AgentAttemptStatus = "running"
	AgentAttemptContinued   AgentAttemptStatus = "continued"
	AgentAttemptFinished    AgentAttemptStatus = "finished"
	AgentAttemptCrashed     AgentAttemptStatus = "crashed"
	AgentAttemptInterrupted AgentAttemptStatus = "interrupted"
)

type AgentAttemptUsage struct {
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	TotalTokens     int64 `json:"total_tokens"`
	ExecutionMillis int64 `json:"execution_millis"`
}

type AgentAttemptFailure struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type AgentAttempt struct {
	ID                    string
	RunID                 string
	AgentID               string
	ParentAgentID         string
	LeaseID               string
	LeaseGeneration       int64
	Turn                  int64
	Status                AgentAttemptStatus
	Usage                 AgentAttemptUsage
	UsageRecordedAt       *time.Time
	Failure               AgentAttemptFailure
	NotificationMessageID string
	StartedAt             time.Time
	UpdatedAt             time.Time
	FinishedAt            *time.Time
}

type AgentAttemptStart struct {
	AttemptID     string
	RunID         string
	AgentID       string
	ParentAgentID string
	Lease         RunExecutionLease
	StartedAt     time.Time
}

type AgentAttemptRef struct {
	RunID     string
	AgentID   string
	AttemptID string
}

type AgentAttemptFailureRequest struct {
	Ref                   AgentAttemptRef
	Failure               AgentAttemptFailure
	NotificationMessageID string
	FailedAt              time.Time
}

func ValidAgentAttemptStatus(status AgentAttemptStatus) bool {
	switch status {
	case AgentAttemptRunning, AgentAttemptContinued, AgentAttemptFinished,
		AgentAttemptCrashed, AgentAttemptInterrupted:
		return true
	default:
		return false
	}
}

func (s AgentAttemptStatus) Terminal() bool {
	return s != AgentAttemptRunning && ValidAgentAttemptStatus(s)
}

func (u AgentAttemptUsage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 || u.ExecutionMillis < 0 {
		return errors.New("agent attempt usage counters cannot be negative")
	}
	if u.TotalTokens < u.InputTokens || u.TotalTokens < u.OutputTokens {
		return errors.New("agent attempt total tokens cannot be smaller than an input or output counter")
	}
	return nil
}

func NormalizeAgentAttemptFailure(failure AgentAttemptFailure) (AgentAttemptFailure, error) {
	failure.Code = strings.ToLower(strings.TrimSpace(failure.Code))
	failure.Reason = strings.TrimSpace(failure.Reason)
	if failure.Code == "" || len([]byte(failure.Code)) > MaxAgentFailureCodeBytes {
		return AgentAttemptFailure{}, fmt.Errorf("agent attempt failure code must contain between 1 and %d bytes",
			MaxAgentFailureCodeBytes)
	}
	for _, char := range failure.Code {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') &&
			char != '.' && char != '_' && char != '-' {
			return AgentAttemptFailure{}, errors.New("agent attempt failure code contains an unsupported character")
		}
	}
	if failure.Reason == "" || !utf8.ValidString(failure.Reason) || strings.ContainsRune(failure.Reason, 0) ||
		utf8.RuneCountInString(failure.Reason) > MaxAgentFailureReasonRunes ||
		len([]byte(failure.Reason)) > MaxAgentFailureReasonBytes {
		return AgentAttemptFailure{}, fmt.Errorf(
			"agent attempt failure reason must contain between 1 and %d characters within %d bytes",
			MaxAgentFailureReasonRunes, MaxAgentFailureReasonBytes)
	}
	return failure, nil
}

func (a AgentAttempt) Validate() error {
	for _, value := range []string{a.ID, a.RunID, a.AgentID, a.ParentAgentID, a.LeaseID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("agent attempt identities are required and must be normalized")
		}
	}
	if a.AgentID == a.ParentAgentID {
		return errors.New("agent attempt child and parent identities must differ")
	}
	if a.LeaseGeneration <= 0 || a.Turn <= 0 {
		return errors.New("agent attempt lease generation and turn must be positive")
	}
	if !ValidAgentAttemptStatus(a.Status) {
		return fmt.Errorf("invalid Agent attempt status %q", a.Status)
	}
	if err := a.Usage.Validate(); err != nil {
		return err
	}
	if a.StartedAt.IsZero() || a.UpdatedAt.IsZero() || a.UpdatedAt.Before(a.StartedAt) {
		return errors.New("agent attempt timestamps are invalid")
	}
	if a.UsageRecordedAt == nil {
		if a.Usage != (AgentAttemptUsage{}) {
			return errors.New("agent attempt usage counters require a recorded timestamp")
		}
	} else if a.UsageRecordedAt.IsZero() || a.UsageRecordedAt.Before(a.StartedAt) ||
		a.UsageRecordedAt.After(a.UpdatedAt) {
		return errors.New("agent attempt usage timestamp is invalid")
	}
	if a.NotificationMessageID != "" &&
		(!validAgentIdentity(a.NotificationMessageID, false) || strings.ContainsRune(a.NotificationMessageID, 0)) {
		return errors.New("agent attempt notification message identity is invalid")
	}
	if a.Status == AgentAttemptRunning {
		if a.FinishedAt != nil || a.Failure != (AgentAttemptFailure{}) || a.NotificationMessageID != "" {
			return errors.New("running Agent attempt cannot have terminal metadata")
		}
		return nil
	}
	if a.FinishedAt == nil || a.FinishedAt.IsZero() || a.FinishedAt.Before(a.StartedAt) ||
		a.FinishedAt.After(a.UpdatedAt) {
		return errors.New("terminal Agent attempt requires a valid finish time")
	}
	switch a.Status {
	case AgentAttemptContinued:
		if a.UsageRecordedAt == nil || a.Failure != (AgentAttemptFailure{}) || a.NotificationMessageID != "" {
			return errors.New("continued Agent attempt requires usage and no failure or notification")
		}
	case AgentAttemptFinished:
		if a.UsageRecordedAt == nil || a.Failure != (AgentAttemptFailure{}) || a.NotificationMessageID == "" {
			return errors.New("finished Agent attempt requires usage and its completion message")
		}
	case AgentAttemptCrashed:
		normalized, err := NormalizeAgentAttemptFailure(a.Failure)
		if err != nil || normalized != a.Failure || a.NotificationMessageID == "" {
			return errors.New("crashed Agent attempt requires normalized failure and notification metadata")
		}
	case AgentAttemptInterrupted:
		normalized, err := NormalizeAgentAttemptFailure(a.Failure)
		if err != nil || normalized != a.Failure || a.NotificationMessageID != "" {
			return errors.New("interrupted Agent attempt requires normalized failure without a notification")
		}
	}
	return nil
}

func (s AgentAttemptStart) Validate() error {
	for _, value := range []string{s.AttemptID, s.RunID, s.AgentID, s.ParentAgentID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("agent attempt start identities are required and must be normalized")
		}
	}
	if s.AgentID == s.ParentAgentID || s.Lease.RunID != s.RunID {
		return errors.New("agent attempt start relationship is invalid")
	}
	if err := s.Lease.Validate(); err != nil {
		return err
	}
	if s.Lease.Status != RunExecutionLeaseActive || s.StartedAt.IsZero() {
		return errors.New("agent attempt start requires an active lease and timestamp")
	}
	return nil
}

func (r AgentAttemptRef) Validate() error {
	for _, value := range []string{r.RunID, r.AgentID, r.AttemptID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("agent attempt reference identities are required and must be normalized")
		}
	}
	return nil
}

func (r AgentAttemptFailureRequest) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if _, err := NormalizeAgentAttemptFailure(r.Failure); err != nil {
		return err
	}
	if !validAgentIdentity(r.NotificationMessageID, false) ||
		strings.ContainsRune(r.NotificationMessageID, 0) || r.FailedAt.IsZero() {
		return errors.New("agent attempt failure notification and timestamp are required")
	}
	return nil
}
