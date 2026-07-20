import type {
  CodeHandoffView,
  VerificationSnapshotReceiptReviewView,
  VerificationSnapshotReceiptView,
} from "../api/types";

export type ReceiptReviewFacts = CodeHandoffView["verification_snapshot_receipt_reviews"];
export type ReceiptReviewNavigationTarget = ReceiptReviewFacts["references"][number];

export function matchesReceiptReviewTarget(target: ReceiptReviewNavigationTarget,
  review: VerificationSnapshotReceiptReviewView,
): boolean {
  return review.id === target.id && review.receipt_id === target.receipt_id &&
    review.receipt_content_sha256 === target.receipt_content_sha256 &&
    review.receipt_event_sequence === target.receipt_event_sequence &&
    review.decision === target.decision &&
    review.review_event_sequence === target.review_event_sequence &&
    review.reviewed_at === target.reviewed_at;
}

export function matchesReceiptTarget(target: ReceiptReviewNavigationTarget,
  receipt: VerificationSnapshotReceiptView,
): boolean {
  return receipt.id === target.receipt_id &&
    receipt.content_sha256 === target.receipt_content_sha256 &&
    receipt.receipt_event_sequence === target.receipt_event_sequence;
}

export function receiptReviewTargetKey(target: ReceiptReviewNavigationTarget): string {
  return JSON.stringify([target.id, target.receipt_id, target.receipt_content_sha256,
    target.receipt_event_sequence, target.decision, target.review_event_sequence,
    target.reviewed_at]);
}
