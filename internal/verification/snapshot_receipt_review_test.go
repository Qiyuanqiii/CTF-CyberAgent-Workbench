package verification

import (
	"strings"
	"testing"
	"time"
)

func TestSnapshotReceiptReviewValidationKeepsDecisionNarrow(t *testing.T) {
	value := SnapshotReceiptReview{
		ID: "review-1", ProtocolVersion: SnapshotReceiptReviewProtocolVersion,
		OperationKeyDigest: strings.Repeat("a", 64),
		RequestFingerprint: strings.Repeat("b", 64), RunID: "run-1",
		SessionID: "session-1", WorkspaceID: "workspace-1", ReceiptID: "receipt-1",
		ReceiptContentSHA256: strings.Repeat("c", 64), ReceiptEventSequence: 2,
		Decision: SnapshotReceiptReviewMetadataConfirmed, ReviewedBy: "operator",
		EventSequence: 3, CreatedAt: time.Now().UTC(),
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.Decision = "accepted"
	if err := value.Validate(); err == nil {
		t.Fatal("receipt review accepted a result-acceptance decision")
	}
	value.Decision = SnapshotReceiptReviewMetadataDisputed
	value.EventSequence = value.ReceiptEventSequence
	if err := value.Validate(); err == nil {
		t.Fatal("receipt review accepted a non-causal event sequence")
	}
}
