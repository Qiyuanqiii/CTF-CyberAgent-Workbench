package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"cyberagent-workbench/internal/redact"
)

type Outcome string

const (
	OutcomeSuccess         Outcome = "success"
	OutcomeRetryable       Outcome = "retryable"
	OutcomeRateLimited     Outcome = "rate_limited"
	OutcomeInvalidResponse Outcome = "invalid_response"
	OutcomeCancelled       Outcome = "cancelled"
	OutcomePermanent       Outcome = "permanent"
)

func (o Outcome) Valid() bool {
	switch o {
	case OutcomeSuccess, OutcomeRetryable, OutcomeRateLimited, OutcomeInvalidResponse, OutcomeCancelled, OutcomePermanent:
		return true
	default:
		return false
	}
}

func (o Outcome) Retryable() bool {
	return o == OutcomeRetryable || o == OutcomeRateLimited
}

type ProviderError struct {
	Kind       Outcome
	Provider   string
	StatusCode int
	RetryAfter time.Duration
	Message    string
	Cause      error
}

func NewProviderError(kind Outcome, provider string, message string, cause error) *ProviderError {
	if !kind.Valid() || kind == OutcomeSuccess {
		kind = OutcomePermanent
	}
	return &ProviderError{
		Kind: kind, Provider: strings.TrimSpace(provider), Message: redact.String(strings.TrimSpace(message)), Cause: cause,
	}
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Message)
	if message == "" && e.Cause != nil {
		message = redact.String(strings.TrimSpace(e.Cause.Error()))
	}
	if message == "" {
		message = string(e.Kind)
	}
	if strings.TrimSpace(e.Provider) == "" {
		return message
	}
	return fmt.Sprintf("provider %s: %s", e.Provider, message)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NormalizeProviderError(provider string, err error) *ProviderError {
	if err == nil {
		return nil
	}
	var typed *ProviderError
	if errors.As(err, &typed) {
		copy := *typed
		if !copy.Kind.Valid() || copy.Kind == OutcomeSuccess {
			copy.Kind = OutcomePermanent
		}
		if strings.TrimSpace(copy.Provider) == "" {
			copy.Provider = strings.TrimSpace(provider)
		}
		copy.Message = redact.String(strings.TrimSpace(copy.Message))
		if copy.RetryAfter < 0 {
			copy.RetryAfter = 0
		}
		return &copy
	}
	kind := OutcomePermanent
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		kind = OutcomeCancelled
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		kind = OutcomeRetryable
	default:
		var netErr net.Error
		var urlErr *url.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			kind = OutcomeRetryable
		} else if errors.As(err, &urlErr) {
			kind = OutcomeRetryable
		}
	}
	return NewProviderError(kind, provider, err.Error(), err)
}

func ProviderErrorKind(err error) Outcome {
	var typed *ProviderError
	if errors.As(err, &typed) && typed != nil {
		return typed.Kind
	}
	return ""
}

type ModelAttempt struct {
	Number       int
	MaxAttempts  int
	Provider     string
	Model        string
	Outcome      Outcome
	ErrorText    string
	RetryAfter   time.Duration
	Elapsed      time.Duration
	RetryPlanned bool
}

func (a ModelAttempt) ValidateStarted() error {
	if a.Number <= 0 || a.MaxAttempts <= 0 || a.Number > a.MaxAttempts {
		return errors.New("model attempt number must be within its limit")
	}
	if strings.TrimSpace(a.Provider) == "" || strings.TrimSpace(a.Model) == "" {
		return errors.New("model attempt provider and model are required")
	}
	if a.RetryAfter < 0 || a.Elapsed < 0 {
		return errors.New("model attempt durations cannot be negative")
	}
	return nil
}

func (a ModelAttempt) ValidateCompleted() error {
	if err := a.ValidateStarted(); err != nil {
		return err
	}
	if a.Outcome != OutcomeSuccess {
		return errors.New("completed model attempt requires success outcome")
	}
	return nil
}

func (a ModelAttempt) ValidateFailed() error {
	if err := a.ValidateStarted(); err != nil {
		return err
	}
	if !a.Outcome.Valid() || a.Outcome == OutcomeSuccess {
		return errors.New("failed model attempt requires a failure outcome")
	}
	if strings.TrimSpace(a.ErrorText) == "" {
		return errors.New("failed model attempt error is required")
	}
	return nil
}
