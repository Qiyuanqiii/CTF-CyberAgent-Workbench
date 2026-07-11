package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MinModelCancellationKeyBytes      = 16
	MaxModelCancellationKeyBytes      = 256
	MaxModelCancellationReasonRunes   = 1024
	MaxModelCancellationIdentityRunes = 256
)

type ModelCancellationStatus string

const (
	ModelCancellationPending  ModelCancellationStatus = "pending"
	ModelCancellationObserved ModelCancellationStatus = "observed"
	ModelCancellationResolved ModelCancellationStatus = "resolved"
)

func (s ModelCancellationStatus) Valid() bool {
	return s == ModelCancellationPending || s == ModelCancellationObserved || s == ModelCancellationResolved
}

type RequestModelCancellation struct {
	RunID          string
	AttemptID      string
	ModelAttempt   int
	IdempotencyKey string
	Reason         string
	RequestedBy    string
}

func (r RequestModelCancellation) Normalize() (RequestModelCancellation, error) {
	originalKey := r.IdempotencyKey
	r.RunID = strings.TrimSpace(r.RunID)
	r.AttemptID = strings.TrimSpace(r.AttemptID)
	r.Reason = strings.TrimSpace(r.Reason)
	r.RequestedBy = strings.TrimSpace(r.RequestedBy)
	for label, value := range map[string]string{
		"Run id": r.RunID, "attempt id": r.AttemptID, "requester": r.RequestedBy,
	} {
		if err := validateModelCancellationText(value, MaxModelCancellationIdentityRunes, false); err != nil {
			return RequestModelCancellation{}, fmt.Errorf("model cancellation %s is invalid: %w", label, err)
		}
	}
	if r.ModelAttempt <= 0 {
		return RequestModelCancellation{}, errors.New("model cancellation model attempt must be positive")
	}
	if r.IdempotencyKey != strings.TrimSpace(originalKey) || !utf8.ValidString(r.IdempotencyKey) ||
		len([]byte(r.IdempotencyKey)) < MinModelCancellationKeyBytes ||
		len([]byte(r.IdempotencyKey)) > MaxModelCancellationKeyBytes {
		return RequestModelCancellation{}, fmt.Errorf("model cancellation idempotency key must be normalized UTF-8 between %d and %d bytes",
			MinModelCancellationKeyBytes, MaxModelCancellationKeyBytes)
	}
	for _, current := range r.IdempotencyKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return RequestModelCancellation{}, errors.New("model cancellation idempotency key cannot contain whitespace or control characters")
		}
	}
	if r.Reason == "" {
		r.Reason = "active model call cancellation requested"
	}
	if err := validateModelCancellationText(r.Reason, MaxModelCancellationReasonRunes, false); err != nil {
		return RequestModelCancellation{}, fmt.Errorf("model cancellation reason is invalid: %w", err)
	}
	return r, nil
}

type ModelCancellation struct {
	ID           string
	RunID        string
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

func (c ModelCancellation) Validate() error {
	for label, value := range map[string]string{
		"id": c.ID, "Run id": c.RunID, "attempt id": c.AttemptID, "requester": c.RequestedBy,
	} {
		if err := validateModelCancellationText(value, MaxModelCancellationIdentityRunes, false); err != nil {
			return fmt.Errorf("model cancellation %s is invalid: %w", label, err)
		}
	}
	if c.ModelAttempt <= 0 || !c.Status.Valid() || c.RequestedAt.IsZero() {
		return errors.New("model cancellation attempt, status, and request time are required")
	}
	if err := validateModelCancellationText(c.Reason, MaxModelCancellationReasonRunes, false); err != nil {
		return fmt.Errorf("model cancellation reason is invalid: %w", err)
	}
	switch c.Status {
	case ModelCancellationPending:
		if c.ObservedAt != nil || c.ResolvedAt != nil || c.Resolution != "" {
			return errors.New("pending model cancellation cannot have observation or resolution data")
		}
	case ModelCancellationObserved:
		if c.ObservedAt == nil || c.ObservedAt.IsZero() || c.ObservedAt.Before(c.RequestedAt) ||
			c.ResolvedAt != nil || c.Resolution != "" {
			return errors.New("observed model cancellation requires only an observation time")
		}
	case ModelCancellationResolved:
		if c.ResolvedAt == nil || c.ResolvedAt.IsZero() || c.ResolvedAt.Before(c.RequestedAt) ||
			strings.TrimSpace(c.Resolution) == "" {
			return errors.New("resolved model cancellation requires a resolution and time")
		}
		if c.ObservedAt != nil && (c.ObservedAt.IsZero() || c.ObservedAt.Before(c.RequestedAt) ||
			c.ResolvedAt.Before(*c.ObservedAt)) {
			return errors.New("resolved model cancellation observation time is inconsistent")
		}
		if err := validateModelCancellationText(c.Resolution, 64, false); err != nil {
			return fmt.Errorf("model cancellation resolution is invalid: %w", err)
		}
	}
	return nil
}

type ModelCancellationResult struct {
	Cancellation ModelCancellation
	Replayed     bool
}

func validateModelCancellationText(value string, maxRunes int, allowEmpty bool) error {
	if !utf8.ValidString(value) || value != strings.TrimSpace(value) || len([]rune(value)) > maxRunes ||
		(!allowEmpty && value == "") {
		return errors.New("value must be normalized and bounded UTF-8")
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return errors.New("value cannot contain control characters")
		}
	}
	return nil
}
