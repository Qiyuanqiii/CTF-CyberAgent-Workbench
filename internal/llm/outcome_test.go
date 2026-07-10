package llm

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNormalizeProviderErrorClassifiesFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Outcome
	}{
		{name: "cancelled", err: context.Canceled, want: OutcomeCancelled},
		{name: "deadline", err: context.DeadlineExceeded, want: OutcomeCancelled},
		{name: "network", err: &url.Error{Op: "Post", URL: "https://provider.invalid", Err: errors.New("connection reset")}, want: OutcomeRetryable},
		{name: "permanent", err: errors.New("invalid provider configuration"), want: OutcomePermanent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeProviderError("test", test.err)
			if got.Kind != test.want || got.Provider != "test" || !errors.Is(got, test.err) {
				t.Fatalf("unexpected normalized error: %#v", got)
			}
		})
	}
}

func TestProviderErrorPreservesTypedMetadataAndRedactsMessage(t *testing.T) {
	token := "t" + "p-" + strings.Repeat("a", 40)
	original := NewProviderError(OutcomeRateLimited, "", "MIMO_API_KEY="+token, nil)
	original.StatusCode = 429
	original.RetryAfter = 3 * time.Second
	normalized := NormalizeProviderError("mimo", original)
	if normalized.Kind != OutcomeRateLimited || normalized.Provider != "mimo" || normalized.StatusCode != 429 || normalized.RetryAfter != 3*time.Second {
		t.Fatalf("typed metadata changed: %#v", normalized)
	}
	if strings.Contains(normalized.Error(), token[:12]) || !strings.Contains(normalized.Error(), "[REDACTED:") {
		t.Fatalf("provider error was not redacted: %q", normalized.Error())
	}
}

func TestNormalizeProviderErrorDowngradesInvalidTypedKind(t *testing.T) {
	for _, kind := range []Outcome{"unknown", OutcomeSuccess} {
		normalized := NormalizeProviderError("test", &ProviderError{Kind: kind, Message: "bad kind"})
		if normalized.Kind != OutcomePermanent {
			t.Fatalf("kind %q normalized to %q", kind, normalized.Kind)
		}
	}
}

func TestModelAttemptValidation(t *testing.T) {
	base := ModelAttempt{Number: 1, MaxAttempts: 3, Provider: "test", Model: "model"}
	if err := base.ValidateStarted(); err != nil {
		t.Fatal(err)
	}
	completed := base
	completed.Outcome = OutcomeSuccess
	if err := completed.ValidateCompleted(); err != nil {
		t.Fatal(err)
	}
	failed := base
	failed.Outcome = OutcomeRetryable
	failed.ErrorText = "temporary"
	if err := failed.ValidateFailed(); err != nil {
		t.Fatal(err)
	}
	failed.ErrorText = ""
	if err := failed.ValidateFailed(); err == nil {
		t.Fatal("failed attempt accepted an empty error")
	}
	repair := ModelAttempt{
		Number: 4, TransportAttempt: 1, MaxAttempts: 3, ProtocolRepair: 1, Provider: "test", Model: "model",
	}
	if err := repair.ValidateStarted(); err != nil {
		t.Fatalf("global attempt number should be independent from transport limit: %v", err)
	}
	repair.ProtocolRepair = 2
	if err := repair.ValidateStarted(); err == nil {
		t.Fatal("model attempt accepted an unbounded protocol repair number")
	}
	stream := base
	stream.StreamEvents = -1
	if err := stream.ValidateStarted(); err == nil {
		t.Fatal("model attempt accepted negative stream counters")
	}
}
