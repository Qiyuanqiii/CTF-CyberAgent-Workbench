package application

import (
	"context"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

type VerificationSnapshotReceiptReviewStore interface {
	VerificationSnapshotReceiptStore
	GetVerificationSnapshotReceipt(context.Context, string) (verification.SnapshotReceipt, error)
	GetVerificationSnapshotReceiptReviewByOperation(context.Context, string) (
		verification.SnapshotReceiptReview, bool, error)
	GetVerificationSnapshotReceiptReviewByReceipt(context.Context, string) (
		verification.SnapshotReceiptReview, bool, error)
	ListVerificationSnapshotReceiptReviews(context.Context, string, int) (
		[]verification.SnapshotReceiptReview, error)
	RecordVerificationSnapshotReceiptReview(context.Context, verification.SnapshotReceiptReview) (
		verification.SnapshotReceiptReview, bool, error)
}

type VerificationSnapshotReceiptReviewService struct {
	store VerificationSnapshotReceiptReviewStore
	now   func() time.Time
}

type RecordVerificationSnapshotReceiptReviewRequest struct {
	Version                     string
	RunID                       string
	ReceiptID                   string
	ReceiptContentSHA256        string
	ReceiptEventSequence        int64
	Decision                    string
	ConfirmNonAuthorizingReview bool
	OperationKey                string
	ReviewedBy                  string
}

type RecordVerificationSnapshotReceiptReviewResult struct {
	Review   verification.SnapshotReceiptReview
	Replayed bool
}

type VerificationSnapshotReceiptReviewInventory struct {
	ProtocolVersion      string
	RunID                string
	SessionID            string
	WorkspaceID          string
	Items                []verification.SnapshotReceiptReview
	Truncated            bool
	MetadataOnly         bool
	ReadOnly             bool
	ReviewNonAuthorizing bool
	SnapshotAccepted     bool
	ResultAccepted       bool
	ResultInferred       bool
	RecordRewritten      bool
	Approval             bool
	AuthorityGranted     bool
	ExecutionStarted     bool
}

func NewVerificationSnapshotReceiptReviewService(
	store VerificationSnapshotReceiptReviewStore,
) *VerificationSnapshotReceiptReviewService {
	return &VerificationSnapshotReceiptReviewService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *VerificationSnapshotReceiptReviewService) Record(ctx context.Context,
	request RecordVerificationSnapshotReceiptReviewRequest,
) (RecordVerificationSnapshotReceiptReviewResult, error) {
	if s == nil || s.store == nil || s.now == nil {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"verification snapshot receipt review store is required")
	}
	originalRunID, originalReceiptID, originalReviewedBy := request.RunID, request.ReceiptID,
		request.ReviewedBy
	originalOperationKey := request.OperationKey
	request.RunID = strings.TrimSpace(request.RunID)
	request.ReceiptID = strings.TrimSpace(request.ReceiptID)
	request.ReviewedBy = strings.TrimSpace(redact.String(request.ReviewedBy))
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	decision := verification.SnapshotReceiptReviewDecision(request.Decision)
	if request.Version != verification.SnapshotReceiptReviewProtocolVersion ||
		originalRunID != request.RunID || originalReceiptID != request.ReceiptID ||
		originalReviewedBy != request.ReviewedBy || !domain.ValidAgentID(request.RunID) ||
		!domain.ValidAgentID(request.ReceiptID) || !domain.ValidAgentID(request.ReviewedBy) ||
		!validSHA256Digest(request.ReceiptContentSHA256) || request.ReceiptEventSequence <= 0 ||
		request.ReceiptEventSequence == math.MaxInt64 ||
		(decision != verification.SnapshotReceiptReviewMetadataConfirmed &&
			decision != verification.SnapshotReceiptReviewMetadataDisputed) ||
		!request.ConfirmNonAuthorizingReview {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification snapshot receipt review protocol, binding, decision, or confirmation is invalid")
	}
	if originalOperationKey != request.OperationKey || !utf8.ValidString(request.OperationKey) {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification snapshot receipt review operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil ||
		containsSpaceOrControl(request.OperationKey) {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification snapshot receipt review operation key is invalid")
	}
	for _, current := range request.ReviewedBy {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(
				apperror.CodeInvalidArgument,
				"verification snapshot receipt review operator identity is invalid")
		}
	}
	keyDigest := runmutation.VerificationSnapshotReceiptReviewOperationDigest(request.RunID,
		request.OperationKey)
	existing, found, err := s.store.GetVerificationSnapshotReceiptReviewByOperation(ctx,
		keyDigest)
	if err != nil {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.Normalize(err)
	}
	if found {
		fingerprint := runmutation.VerificationSnapshotReceiptReviewRequestFingerprint(
			request.RunID, existing.SessionID, existing.WorkspaceID, request.ReceiptID,
			request.ReceiptContentSHA256, request.ReceiptEventSequence, request.Decision,
			request.ReviewedBy)
		if existing.RequestFingerprint != fingerprint || existing.RunID != request.RunID ||
			existing.ReceiptID != request.ReceiptID ||
			existing.ReceiptContentSHA256 != request.ReceiptContentSHA256 ||
			existing.ReceiptEventSequence != request.ReceiptEventSequence ||
			existing.Decision != decision || existing.ReviewedBy != request.ReviewedBy {
			return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(
				apperror.CodeConflict,
				"verification snapshot receipt review operation key was used for different intent")
		}
		return RecordVerificationSnapshotReceiptReviewResult{Review: existing, Replayed: true}, nil
	}
	run, mission, linkedSession, registered, err := loadVerificationCoverageBinding(ctx,
		s.store, request.RunID)
	if err != nil {
		return RecordVerificationSnapshotReceiptReviewResult{}, err
	}
	if linkedSession.Status != session.StatusActive {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt review requires an active Code Session")
	}
	receipt, err := s.store.GetVerificationSnapshotReceipt(ctx, request.ReceiptID)
	if err != nil {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.Normalize(err)
	}
	if err := receipt.Validate(); err != nil || receipt.RunID != run.ID ||
		receipt.SessionID != linkedSession.ID || receipt.WorkspaceID != mission.WorkspaceID ||
		receipt.WorkspaceID != registered.ID ||
		receipt.ContentSHA256 != request.ReceiptContentSHA256 ||
		receipt.EventSequence != request.ReceiptEventSequence {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt review escaped its exact receipt binding")
	}
	if _, found, err := s.store.GetVerificationSnapshotReceiptReviewByReceipt(ctx,
		receipt.ID); err != nil {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.Normalize(err)
	} else if found {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt already has an immutable review")
	}
	now := s.now().UTC()
	if now.Before(receipt.CreatedAt) {
		now = receipt.CreatedAt
	}
	review := verification.SnapshotReceiptReview{
		ID:                 idgen.New("verification-snapshot-receipt-review"),
		ProtocolVersion:    verification.SnapshotReceiptReviewProtocolVersion,
		OperationKeyDigest: keyDigest,
		RequestFingerprint: runmutation.VerificationSnapshotReceiptReviewRequestFingerprint(
			run.ID, linkedSession.ID, registered.ID, receipt.ID, receipt.ContentSHA256,
			receipt.EventSequence, request.Decision, request.ReviewedBy),
		RunID: run.ID, SessionID: linkedSession.ID, WorkspaceID: registered.ID,
		ReceiptID: receipt.ID, ReceiptContentSHA256: receipt.ContentSHA256,
		ReceiptEventSequence: receipt.EventSequence, Decision: decision,
		ReviewedBy: request.ReviewedBy, CreatedAt: now,
	}
	prepared := review
	prepared.EventSequence = review.ReceiptEventSequence + 1
	if err := prepared.Validate(); err != nil {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification snapshot receipt review is invalid", err)
	}
	stored, replayed, err := s.store.RecordVerificationSnapshotReceiptReview(ctx, review)
	if err != nil {
		return RecordVerificationSnapshotReceiptReviewResult{}, apperror.Normalize(err)
	}
	return RecordVerificationSnapshotReceiptReviewResult{Review: stored, Replayed: replayed}, nil
}

func (s *VerificationSnapshotReceiptReviewService) Inventory(ctx context.Context,
	runID string,
) (VerificationSnapshotReceiptReviewInventory, error) {
	if s == nil || s.store == nil {
		return VerificationSnapshotReceiptReviewInventory{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"verification snapshot receipt review store is required")
	}
	run, mission, linkedSession, _, err := loadVerificationCoverageBinding(ctx, s.store, runID)
	if err != nil {
		return VerificationSnapshotReceiptReviewInventory{}, err
	}
	values, err := s.store.ListVerificationSnapshotReceiptReviews(ctx, run.ID,
		verification.MaxSnapshotReceiptReviewHistory+1)
	if err != nil {
		return VerificationSnapshotReceiptReviewInventory{}, apperror.Normalize(err)
	}
	result := VerificationSnapshotReceiptReviewInventory{
		ProtocolVersion: verification.SnapshotReceiptReviewInventoryProtocolVersion,
		RunID:           run.ID, SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		Truncated:    len(values) > verification.MaxSnapshotReceiptReviewHistory,
		MetadataOnly: true, ReadOnly: true, ReviewNonAuthorizing: true,
	}
	if result.Truncated {
		values = values[:verification.MaxSnapshotReceiptReviewHistory]
	}
	result.Items = append([]verification.SnapshotReceiptReview{}, values...)
	for _, value := range result.Items {
		if err := value.Validate(); err != nil || value.RunID != run.ID ||
			value.SessionID != linkedSession.ID || value.WorkspaceID != mission.WorkspaceID {
			return VerificationSnapshotReceiptReviewInventory{}, apperror.New(
				apperror.CodeConflict,
				"verification snapshot receipt review escaped its Run binding")
		}
	}
	return result, nil
}
