package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxSpecialistRepairReasonRunes = 1024
	MaxSpecialistRepairReasonBytes = 4 * 1024
)

type SpecialistProtocolRepairStatus string

const (
	SpecialistRepairPending   SpecialistProtocolRepairStatus = "pending"
	SpecialistRepairCompleted SpecialistProtocolRepairStatus = "completed"
	SpecialistRepairExhausted SpecialistProtocolRepairStatus = "exhausted"
	SpecialistRepairAborted   SpecialistProtocolRepairStatus = "aborted"
)

type SpecialistProtocolRepair struct {
	AgentAttemptID        string
	RunID                 string
	AgentID               string
	Status                SpecialistProtocolRepairStatus
	Reason                string
	RequestedModelAttempt int
	ResolvedModelAttempt  int
	RequestedAt           time.Time
	ResolvedAt            *time.Time
}

func ValidSpecialistProtocolRepairStatus(status SpecialistProtocolRepairStatus) bool {
	switch status {
	case SpecialistRepairPending, SpecialistRepairCompleted,
		SpecialistRepairExhausted, SpecialistRepairAborted:
		return true
	default:
		return false
	}
}

func (r SpecialistProtocolRepair) Validate() error {
	for _, value := range []string{r.AgentAttemptID, r.RunID, r.AgentID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("specialist protocol repair identities are required and must be normalized")
		}
	}
	if !ValidSpecialistProtocolRepairStatus(r.Status) {
		return fmt.Errorf("invalid Specialist protocol repair status %q", r.Status)
	}
	if !utf8.ValidString(r.Reason) || strings.TrimSpace(r.Reason) != r.Reason ||
		r.Reason == "" || strings.ContainsRune(r.Reason, 0) ||
		utf8.RuneCountInString(r.Reason) > MaxSpecialistRepairReasonRunes ||
		len([]byte(r.Reason)) > MaxSpecialistRepairReasonBytes {
		return errors.New("specialist protocol repair reason is invalid or too large")
	}
	if r.RequestedModelAttempt <= 0 || r.RequestedAt.IsZero() {
		return errors.New("specialist protocol repair request metadata is invalid")
	}
	if r.Status == SpecialistRepairPending {
		if r.ResolvedModelAttempt != 0 || r.ResolvedAt != nil {
			return errors.New("pending specialist protocol repair cannot be resolved")
		}
		return nil
	}
	if r.ResolvedAt == nil || r.ResolvedAt.IsZero() || r.ResolvedAt.Before(r.RequestedAt) {
		return errors.New("terminal specialist protocol repair requires a valid resolution time")
	}
	if r.Status == SpecialistRepairAborted {
		if r.ResolvedModelAttempt != 0 {
			return errors.New("aborted Specialist protocol repair cannot name a model attempt")
		}
		return nil
	}
	if r.ResolvedModelAttempt <= r.RequestedModelAttempt {
		return errors.New("resolved Specialist protocol repair requires a later model attempt")
	}
	return nil
}
