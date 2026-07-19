package verification

import (
	"errors"
	"time"
)

const (
	PlanEvidenceAssociationProtocolVersion = "operator_verification_plan_evidence_association.v1"
	PlanCoverageProtocolVersion            = "operator_verification_plan_coverage.v1"
	MaxCoverageAssociations                = 100
	MaxSafeCoverageCount                   = 1_000_000_000
)

// PlanEvidenceAssociation is a separate immutable operator fact. It links one
// already-recorded observation to one earlier plan item without rewriting either.
type PlanEvidenceAssociation struct {
	ID                    string
	ProtocolVersion       string
	OperationKeyDigest    string
	RequestFingerprint    string
	RunID                 string
	SessionID             string
	WorkspaceID           string
	PlanID                string
	PlanItemOrdinal       int
	PlanItemSHA256        string
	EvidenceID            string
	EvidenceOutcome       Outcome
	EvidenceEventSequence int64
	AssociatedBy          string
	EventSequence         int64
	CreatedAt             time.Time
}

func (a PlanEvidenceAssociation) Validate() error {
	if a.ProtocolVersion != PlanEvidenceAssociationProtocolVersion ||
		!validDigest(a.OperationKeyDigest) || !validDigest(a.RequestFingerprint) ||
		!validDigest(a.PlanItemSHA256) || !a.EvidenceOutcome.Valid() ||
		a.PlanItemOrdinal < 1 || a.PlanItemOrdinal > MaxPlanItems ||
		a.EvidenceEventSequence <= 0 || a.EventSequence <= a.EvidenceEventSequence ||
		a.CreatedAt.IsZero() {
		return errors.New("verification association protocol, digest, outcome, or sequence is invalid")
	}
	for _, value := range []string{a.ID, a.RunID, a.SessionID, a.WorkspaceID,
		a.PlanID, a.EvidenceID, a.AssociatedBy} {
		if !validIdentity(value) {
			return errors.New("verification association identity is invalid")
		}
	}
	return nil
}

type PlanEvidenceAssociationReference struct {
	ID                    string
	PlanID                string
	PlanItemOrdinal       int
	PlanItemSHA256        string
	EvidenceID            string
	EvidenceOutcome       Outcome
	EvidenceEventSequence int64
	AssociationSequence   int64
	CreatedAt             time.Time
}

type PlanItemCoverageCount struct {
	PlanID                         string
	PlanItemOrdinal                int
	PlanItemSHA256                 string
	AssociatedEvidenceCount        int
	PassCount                      int
	FailCount                      int
	UnknownCount                   int
	LatestAssociationEventSequence int64
}

func (c PlanItemCoverageCount) Validate() error {
	if !validIdentity(c.PlanID) || c.PlanItemOrdinal < 1 || c.PlanItemOrdinal > MaxPlanItems ||
		!validDigest(c.PlanItemSHA256) || c.AssociatedEvidenceCount < 0 ||
		c.AssociatedEvidenceCount > MaxSafeCoverageCount || c.PassCount < 0 ||
		c.FailCount < 0 || c.UnknownCount < 0 || c.PassCount > MaxSafeCoverageCount ||
		c.FailCount > MaxSafeCoverageCount || c.UnknownCount > MaxSafeCoverageCount ||
		int64(c.PassCount)+int64(c.FailCount)+int64(c.UnknownCount) !=
			int64(c.AssociatedEvidenceCount) ||
		(c.AssociatedEvidenceCount == 0) != (c.LatestAssociationEventSequence == 0) {
		return errors.New("verification plan coverage count is invalid")
	}
	return nil
}
