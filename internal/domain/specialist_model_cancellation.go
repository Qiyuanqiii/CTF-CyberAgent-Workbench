package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type RequestSpecialistModelCancellation struct {
	RunID          string
	AgentID        string
	AttemptID      string
	ModelAttempt   int
	IdempotencyKey string
	Reason         string
	RequestedBy    string
}

func (r RequestSpecialistModelCancellation) Normalize() (RequestSpecialistModelCancellation, error) {
	originalKey := r.IdempotencyKey
	r.RunID = strings.TrimSpace(r.RunID)
	r.AgentID = strings.TrimSpace(r.AgentID)
	r.AttemptID = strings.TrimSpace(r.AttemptID)
	r.Reason = strings.TrimSpace(r.Reason)
	r.RequestedBy = strings.TrimSpace(r.RequestedBy)
	for label, value := range map[string]string{
		"Run id": r.RunID, "Agent id": r.AgentID, "attempt id": r.AttemptID,
		"requester": r.RequestedBy,
	} {
		if err := validateModelCancellationText(value, MaxModelCancellationIdentityRunes, false); err != nil {
			return RequestSpecialistModelCancellation{},
				fmt.Errorf("specialist model cancellation %s is invalid: %w", label, err)
		}
	}
	if r.ModelAttempt <= 0 {
		return RequestSpecialistModelCancellation{},
			errors.New("specialist model cancellation model attempt must be positive")
	}
	if r.IdempotencyKey != strings.TrimSpace(originalKey) || !utf8.ValidString(r.IdempotencyKey) ||
		len([]byte(r.IdempotencyKey)) < MinModelCancellationKeyBytes ||
		len([]byte(r.IdempotencyKey)) > MaxModelCancellationKeyBytes {
		return RequestSpecialistModelCancellation{}, fmt.Errorf(
			"specialist model cancellation idempotency key must be normalized UTF-8 between %d and %d bytes",
			MinModelCancellationKeyBytes, MaxModelCancellationKeyBytes)
	}
	for _, current := range r.IdempotencyKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return RequestSpecialistModelCancellation{},
				errors.New("specialist model cancellation idempotency key cannot contain whitespace or control characters")
		}
	}
	if r.Reason == "" {
		r.Reason = "active Specialist model call cancellation requested"
	}
	if err := validateModelCancellationText(r.Reason, MaxModelCancellationReasonRunes, false); err != nil {
		return RequestSpecialistModelCancellation{},
			fmt.Errorf("specialist model cancellation reason is invalid: %w", err)
	}
	return r, nil
}

type SpecialistModelCancellation struct {
	ID           string
	RunID        string
	AgentID      string
	AttemptID    string
	ModelAttempt int
	Status       ModelCancellationStatus
	Reason       string
	RequestedBy  string
	RequestedAt  time.Time
	ObservedAt   *time.Time
	ResolvedAt   *time.Time
	Resolution   string
}

func (c SpecialistModelCancellation) Validate() error {
	base := ModelCancellation{
		ID: c.ID, RunID: c.RunID, AttemptID: c.AttemptID, ModelAttempt: c.ModelAttempt,
		Status: c.Status, Reason: c.Reason, RequestedBy: c.RequestedBy,
		RequestedAt: c.RequestedAt, ObservedAt: c.ObservedAt, ResolvedAt: c.ResolvedAt,
		Resolution: c.Resolution,
	}
	if err := base.Validate(); err != nil {
		return fmt.Errorf("specialist model cancellation is invalid: %w", err)
	}
	if err := validateModelCancellationText(c.AgentID, MaxModelCancellationIdentityRunes, false); err != nil {
		return fmt.Errorf("specialist model cancellation Agent id is invalid: %w", err)
	}
	return nil
}

type SpecialistModelCancellationResult struct {
	Cancellation SpecialistModelCancellation
	Replayed     bool
}
