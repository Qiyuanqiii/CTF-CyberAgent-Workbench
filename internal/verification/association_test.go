package verification

import (
	"strings"
	"testing"
	"time"
)

func TestPlanEvidenceAssociationValidation(t *testing.T) {
	digest := strings.Repeat("a", 64)
	value := PlanEvidenceAssociation{
		ID: "association-1", ProtocolVersion: PlanEvidenceAssociationProtocolVersion,
		OperationKeyDigest: digest, RequestFingerprint: digest, RunID: "run-1",
		SessionID: "session-1", WorkspaceID: "workspace-1", PlanID: "plan-1",
		PlanItemOrdinal: 1, PlanItemSHA256: digest, EvidenceID: "evidence-1",
		EvidenceOutcome: OutcomePass, EvidenceEventSequence: 2, AssociatedBy: "operator",
		EventSequence: 3, CreatedAt: time.Now().UTC(),
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.EventSequence = value.EvidenceEventSequence
	if err := value.Validate(); err == nil {
		t.Fatal("association accepted a non-causal event sequence")
	}
}

func TestPlanItemCoverageCountRejectsInferredOrOverflowingCounts(t *testing.T) {
	digest := strings.Repeat("b", 64)
	value := PlanItemCoverageCount{PlanID: "plan-1", PlanItemOrdinal: 1,
		PlanItemSHA256: digest, AssociatedEvidenceCount: 2, PassCount: 1,
		UnknownCount: 1, LatestAssociationEventSequence: 4}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.PassCount = 2
	if err := value.Validate(); err == nil {
		t.Fatal("coverage accepted outcome counts that exceed explicit associations")
	}
	value = PlanItemCoverageCount{PlanID: "plan-1", PlanItemOrdinal: 1,
		PlanItemSHA256: digest, AssociatedEvidenceCount: 1, PassCount: 1,
		LatestAssociationEventSequence: -1}
	if err := value.Validate(); err == nil {
		t.Fatal("coverage accepted a negative latest event sequence")
	}
	value = PlanItemCoverageCount{PlanID: "plan-1", PlanItemOrdinal: 1,
		PlanItemSHA256: digest}
	if err := value.Validate(); err == nil {
		t.Fatal("coverage accepted an unobserved aggregate row")
	}
	value = PlanItemCoverageCount{PlanID: "plan-1", PlanItemOrdinal: 1,
		PlanItemSHA256: digest, AssociatedEvidenceCount: MaxSafeCoverageCount + 1,
		PassCount: MaxSafeCoverageCount + 1, LatestAssociationEventSequence: 4}
	if err := value.Validate(); err == nil {
		t.Fatal("coverage accepted a protocol count overflow")
	}
}
