package verification

import (
	"strings"
	"testing"
	"time"
)

func TestSnapshotReceiptValidationKeepsAcceptanceAndContentOutOfTheRecord(t *testing.T) {
	digest := strings.Repeat("a", 64)
	value := SnapshotReceipt{
		ID: "snapshot-receipt-1", ProtocolVersion: SnapshotReceiptProtocolVersion,
		OperationKeyDigest: digest, RequestFingerprint: digest, RunID: "run-1",
		SessionID: "session-1", WorkspaceID: "workspace-1", PlanID: "plan-1",
		PlanSHA256: digest, PlanItemOrdinal: 1, PlanItemSHA256: digest, Format: "json",
		SnapshotHighWaterEventSequence: 7, AssociatedEvidenceCount: 1, PassCount: 1,
		ReturnedAssociationCount: 1, ContentSHA256: digest, ContentBytes: 128,
		RecordedBy: "operator", EventSequence: 8, CreatedAt: time.Now().UTC(),
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := value
	changed.ReturnedAssociationCount = 0
	if err := changed.Validate(); err == nil {
		t.Fatal("receipt accepted inconsistent returned association count")
	}
	changed = value
	changed.EventSequence = changed.SnapshotHighWaterEventSequence
	if err := changed.Validate(); err == nil {
		t.Fatal("receipt accepted a non-causal event sequence")
	}
}
