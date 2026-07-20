package verification

import (
	"errors"
	"time"
)

const (
	SnapshotReceiptReviewProtocolVersion          = "operator_verification_plan_item_snapshot_receipt_review.v1"
	SnapshotReceiptReviewInventoryProtocolVersion = "operator_verification_plan_item_snapshot_receipt_review_inventory.v1"
	MaxSnapshotReceiptReviewHistory               = 100
)

type SnapshotReceiptReviewDecision string

const (
	SnapshotReceiptReviewMetadataConfirmed SnapshotReceiptReviewDecision = "metadata_confirmed"
	SnapshotReceiptReviewMetadataDisputed  SnapshotReceiptReviewDecision = "metadata_disputed"
)

// SnapshotReceiptReview records one operator decision about receipt metadata only.
// It never accepts a verification result or grants approval, authority, or execution.
type SnapshotReceiptReview struct {
	ID                   string
	ProtocolVersion      string
	OperationKeyDigest   string
	RequestFingerprint   string
	RunID                string
	SessionID            string
	WorkspaceID          string
	ReceiptID            string
	ReceiptContentSHA256 string
	ReceiptEventSequence int64
	Decision             SnapshotReceiptReviewDecision
	ReviewedBy           string
	EventSequence        int64
	CreatedAt            time.Time
}

func (r SnapshotReceiptReview) Validate() error {
	if r.ProtocolVersion != SnapshotReceiptReviewProtocolVersion ||
		!validDigest(r.OperationKeyDigest) || !validDigest(r.RequestFingerprint) ||
		!validDigest(r.ReceiptContentSHA256) || r.ReceiptEventSequence <= 0 ||
		r.EventSequence <= r.ReceiptEventSequence || r.CreatedAt.IsZero() ||
		(r.Decision != SnapshotReceiptReviewMetadataConfirmed &&
			r.Decision != SnapshotReceiptReviewMetadataDisputed) {
		return errors.New("verification snapshot receipt review protocol or binding is invalid")
	}
	for _, value := range []string{r.ID, r.RunID, r.SessionID, r.WorkspaceID,
		r.ReceiptID, r.ReviewedBy} {
		if !validIdentity(value) {
			return errors.New("verification snapshot receipt review identity is invalid")
		}
	}
	return nil
}
