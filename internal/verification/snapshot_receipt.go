package verification

import (
	"errors"
	"time"
)

const (
	SnapshotReceiptProtocolVersion          = "operator_verification_plan_item_snapshot_receipt.v1"
	SnapshotReceiptInventoryProtocolVersion = "operator_verification_plan_item_snapshot_receipt_inventory.v1"
	MaxSnapshotReceiptHistory               = 100
	MaxSnapshotReceiptContentBytes          = 256 * 1024
)

// SnapshotReceipt records that an operator retained one exact deterministic
// metadata snapshot. It is not verification-result acceptance or execution authority.
type SnapshotReceipt struct {
	ID                             string
	ProtocolVersion                string
	OperationKeyDigest             string
	RequestFingerprint             string
	RunID                          string
	SessionID                      string
	WorkspaceID                    string
	PlanID                         string
	PlanSHA256                     string
	PlanItemOrdinal                int
	PlanItemSHA256                 string
	Format                         string
	SnapshotHighWaterEventSequence int64
	AssociatedEvidenceCount        int
	PassCount                      int
	FailCount                      int
	UnknownCount                   int
	ReturnedAssociationCount       int
	AssociationsTruncated          bool
	ContentSHA256                  string
	ContentBytes                   int
	RecordedBy                     string
	EventSequence                  int64
	CreatedAt                      time.Time
}

func (r SnapshotReceipt) Validate() error {
	if r.ProtocolVersion != SnapshotReceiptProtocolVersion ||
		!validDigest(r.OperationKeyDigest) || !validDigest(r.RequestFingerprint) ||
		!validDigest(r.PlanSHA256) || !validDigest(r.PlanItemSHA256) ||
		!validDigest(r.ContentSHA256) ||
		(r.Format != "json" && r.Format != "markdown") ||
		r.PlanItemOrdinal < 1 || r.PlanItemOrdinal > MaxPlanItems ||
		r.SnapshotHighWaterEventSequence < 0 || r.EventSequence <= 0 ||
		r.EventSequence <= r.SnapshotHighWaterEventSequence || r.CreatedAt.IsZero() {
		return errors.New("verification snapshot receipt protocol, digest, format, or sequence is invalid")
	}
	for _, value := range []string{r.ID, r.RunID, r.SessionID, r.WorkspaceID,
		r.PlanID, r.RecordedBy} {
		if !validIdentity(value) {
			return errors.New("verification snapshot receipt identity is invalid")
		}
	}
	if r.AssociatedEvidenceCount < 0 || r.AssociatedEvidenceCount > MaxSafeCoverageCount ||
		r.PassCount < 0 || r.FailCount < 0 || r.UnknownCount < 0 ||
		r.PassCount > MaxSafeCoverageCount || r.FailCount > MaxSafeCoverageCount ||
		r.UnknownCount > MaxSafeCoverageCount ||
		int64(r.PassCount)+int64(r.FailCount)+int64(r.UnknownCount) !=
			int64(r.AssociatedEvidenceCount) ||
		(r.AssociatedEvidenceCount == 0) != (r.SnapshotHighWaterEventSequence == 0) {
		return errors.New("verification snapshot receipt observation counts are invalid")
	}
	returned := r.AssociatedEvidenceCount
	if returned > MaxCoverageAssociations {
		returned = MaxCoverageAssociations
	}
	if r.ReturnedAssociationCount != returned ||
		r.AssociationsTruncated != (r.AssociatedEvidenceCount > MaxCoverageAssociations) ||
		r.ContentBytes < 1 || r.ContentBytes > MaxSnapshotReceiptContentBytes {
		return errors.New("verification snapshot receipt content metadata is invalid")
	}
	return nil
}
