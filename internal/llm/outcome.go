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
	Number           int
	TransportAttempt int
	MaxAttempts      int
	ProtocolRepair   int
	ToolRound        int
	Provider         string
	Model            string
	Outcome          Outcome
	ErrorText        string
	RetryAfter       time.Duration
	Elapsed          time.Duration
	RetryPlanned     bool
	StreamEvents     int
	StreamBytes      int
	Context          *ModelContextAudit
}

const MaxModelContextSources = 256

type ModelContextSource struct {
	Kind     string `json:"kind"`
	SourceID string `json:"source_id"`
	Tokens   int    `json:"tokens"`
}

type ModelContextAudit struct {
	TokenBudget     int                  `json:"token_budget"`
	EstimatedTokens int                  `json:"estimated_tokens"`
	Included        []ModelContextSource `json:"included"`
	Omitted         []ModelContextSource `json:"omitted,omitempty"`
}

func (a ModelContextAudit) Validate() error {
	if a.TokenBudget <= 0 {
		return errors.New("model context token budget must be positive")
	}
	if a.EstimatedTokens < 0 || a.EstimatedTokens > a.TokenBudget {
		return errors.New("model context token estimate must be within budget")
	}
	if len(a.Included)+len(a.Omitted) > MaxModelContextSources {
		return fmt.Errorf("model context source list exceeds %d items", MaxModelContextSources)
	}
	seen := make(map[string]struct{}, len(a.Included)+len(a.Omitted))
	includedTokens := 0
	for _, group := range [][]ModelContextSource{a.Included, a.Omitted} {
		for _, source := range group {
			if strings.TrimSpace(source.Kind) == "" || strings.TrimSpace(source.SourceID) == "" || source.Tokens <= 0 {
				return errors.New("model context source kind, id, and positive tokens are required")
			}
			if len([]rune(source.Kind)) > 64 || len([]rune(source.SourceID)) > 256 {
				return errors.New("model context source identity is too long")
			}
			key := source.Kind + "\x00" + source.SourceID
			if _, ok := seen[key]; ok {
				return errors.New("model context source identities must be unique")
			}
			seen[key] = struct{}{}
		}
	}
	for _, source := range a.Included {
		if source.Tokens > a.TokenBudget-includedTokens {
			return errors.New("model context included source tokens exceed budget")
		}
		includedTokens += source.Tokens
	}
	if includedTokens != a.EstimatedTokens {
		return errors.New("model context included source tokens do not match estimate")
	}
	return nil
}

func (a ModelAttempt) ValidateStarted() error {
	if a.Number <= 0 || a.MaxAttempts <= 0 {
		return errors.New("model attempt number and transport limit must be positive")
	}
	transportAttempt := a.TransportNumber()
	if transportAttempt <= 0 || transportAttempt > a.MaxAttempts {
		return errors.New("model transport attempt must be within its limit")
	}
	if a.ProtocolRepair < 0 || a.ProtocolRepair > 1 {
		return errors.New("model protocol repair number must be zero or one")
	}
	if a.ToolRound < 0 || a.ToolRound > 4 {
		return errors.New("model tool round must be between zero and four")
	}
	if strings.TrimSpace(a.Provider) == "" || strings.TrimSpace(a.Model) == "" {
		return errors.New("model attempt provider and model are required")
	}
	if a.RetryAfter < 0 || a.Elapsed < 0 {
		return errors.New("model attempt durations cannot be negative")
	}
	if a.StreamEvents < 0 || a.StreamBytes < 0 {
		return errors.New("model stream counters cannot be negative")
	}
	if a.Context != nil {
		if err := a.Context.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (a ModelAttempt) TransportNumber() int {
	if a.TransportAttempt > 0 {
		return a.TransportAttempt
	}
	return a.Number
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
