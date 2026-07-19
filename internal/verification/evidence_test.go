package verification

import (
	"strings"
	"testing"
	"time"
)

func TestEvidenceValidatesClosedOutcomeAndDigest(t *testing.T) {
	summary := "go test ./... passed"
	value := Evidence{
		ID: "verification-1", ProtocolVersion: EvidenceProtocolVersion,
		OperationKeyDigest: strings.Repeat("a", 64),
		RequestFingerprint: strings.Repeat("b", 64), RunID: "run-1",
		SessionID: "session-1", WorkspaceID: "workspace-1", Outcome: OutcomePass,
		Title: "Full test suite", Summary: summary, SummarySHA256: SummaryDigest(summary),
		RecordedBy: "operator", EventSequence: 1, CreatedAt: time.Now().UTC(),
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.Outcome = "skipped"
	if err := value.Validate(); err == nil {
		t.Fatal("open verification outcome was accepted")
	}
}

func TestValidateTextRejectsCarriageReturn(t *testing.T) {
	for _, value := range []string{"line one\rline two", "line one\r\nline two"} {
		if err := ValidateText(value, MaxSummaryRunes, true); err == nil {
			t.Fatalf("verification summary accepted carriage return %q", value)
		}
	}
	if err := ValidateText("line one\nline two\tverified", MaxSummaryRunes, true); err != nil {
		t.Fatalf("verification summary rejected supported multiline text: %v", err)
	}
}
