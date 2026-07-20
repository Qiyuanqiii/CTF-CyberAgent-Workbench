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

type VerificationSnapshotReceiptStore interface {
	VerificationCoverageDetailStore
	GetVerificationSnapshotReceiptByOperation(context.Context, string) (
		verification.SnapshotReceipt, bool, error)
	ListVerificationSnapshotReceipts(context.Context, string, int) (
		[]verification.SnapshotReceipt, error)
	RecordVerificationSnapshotReceipt(context.Context, verification.SnapshotReceipt) (
		verification.SnapshotReceipt, bool, error)
}

type VerificationSnapshotReceiptService struct {
	store VerificationSnapshotReceiptStore
	now   func() time.Time
}

type RecordVerificationSnapshotReceiptRequest struct {
	Version                        string
	RunID                          string
	PlanID                         string
	PlanItemOrdinal                int
	Format                         string
	SnapshotHighWaterEventSequence int64
	ContentSHA256                  string
	ConfirmMetadataSnapshot        bool
	OperationKey                   string
	RecordedBy                     string
}

type RecordVerificationSnapshotReceiptResult struct {
	Receipt  verification.SnapshotReceipt
	Replayed bool
}

type VerificationSnapshotReceiptInventory struct {
	ProtocolVersion  string
	RunID            string
	SessionID        string
	WorkspaceID      string
	Items            []verification.SnapshotReceipt
	Truncated        bool
	MetadataOnly     bool
	ReadOnly         bool
	SnapshotAccepted bool
	ResultAccepted   bool
	ResultInferred   bool
	RecordRewritten  bool
	Approval         bool
	AuthorityGranted bool
	ExecutionStarted bool
}

func NewVerificationSnapshotReceiptService(
	store VerificationSnapshotReceiptStore,
) *VerificationSnapshotReceiptService {
	return &VerificationSnapshotReceiptService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *VerificationSnapshotReceiptService) Record(ctx context.Context,
	request RecordVerificationSnapshotReceiptRequest,
) (RecordVerificationSnapshotReceiptResult, error) {
	if s == nil || s.store == nil || s.now == nil {
		return RecordVerificationSnapshotReceiptResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification snapshot receipt store is required")
	}
	originalRunID, originalPlanID, originalRecordedBy := request.RunID, request.PlanID,
		request.RecordedBy
	originalOperationKey := request.OperationKey
	request.RunID = strings.TrimSpace(request.RunID)
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.RecordedBy = strings.TrimSpace(redact.String(request.RecordedBy))
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	if request.Version != verification.SnapshotReceiptProtocolVersion ||
		originalRunID != request.RunID || originalPlanID != request.PlanID ||
		originalRecordedBy != request.RecordedBy || !domain.ValidAgentID(request.RunID) ||
		!domain.ValidAgentID(request.PlanID) || !domain.ValidAgentID(request.RecordedBy) ||
		request.PlanItemOrdinal < 1 || request.PlanItemOrdinal > verification.MaxPlanItems ||
		(request.Format != VerificationSnapshotExportFormatJSON &&
			request.Format != VerificationSnapshotExportFormatMarkdown) ||
		request.SnapshotHighWaterEventSequence < 0 ||
		request.SnapshotHighWaterEventSequence == math.MaxInt64 ||
		!validSHA256Digest(request.ContentSHA256) || !request.ConfirmMetadataSnapshot {
		return RecordVerificationSnapshotReceiptResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification snapshot receipt protocol, binding, digest, or confirmation is invalid")
	}
	if originalOperationKey != request.OperationKey || !utf8.ValidString(request.OperationKey) {
		return RecordVerificationSnapshotReceiptResult{}, apperror.New(
			apperror.CodeInvalidArgument,
			"verification snapshot receipt operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil ||
		containsSpaceOrControl(request.OperationKey) {
		return RecordVerificationSnapshotReceiptResult{}, apperror.New(
			apperror.CodeInvalidArgument, "verification snapshot receipt operation key is invalid")
	}
	for _, current := range request.RecordedBy {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return RecordVerificationSnapshotReceiptResult{}, apperror.New(
				apperror.CodeInvalidArgument,
				"verification snapshot receipt operator identity is invalid")
		}
	}
	keyDigest := runmutation.VerificationSnapshotReceiptOperationDigest(request.RunID,
		request.OperationKey)
	existing, found, err := s.store.GetVerificationSnapshotReceiptByOperation(ctx, keyDigest)
	if err != nil {
		return RecordVerificationSnapshotReceiptResult{}, apperror.Normalize(err)
	}
	if found {
		fingerprint := runmutation.VerificationSnapshotReceiptRequestFingerprint(request.RunID,
			existing.SessionID, existing.WorkspaceID, request.PlanID, request.PlanItemOrdinal,
			request.Format, request.SnapshotHighWaterEventSequence, request.ContentSHA256,
			request.RecordedBy)
		if existing.RequestFingerprint != fingerprint || existing.RunID != request.RunID ||
			existing.PlanID != request.PlanID ||
			existing.PlanItemOrdinal != request.PlanItemOrdinal || existing.Format != request.Format ||
			existing.SnapshotHighWaterEventSequence != request.SnapshotHighWaterEventSequence ||
			existing.ContentSHA256 != request.ContentSHA256 ||
			existing.RecordedBy != request.RecordedBy {
			return RecordVerificationSnapshotReceiptResult{}, apperror.New(
				apperror.CodeConflict,
				"verification snapshot receipt operation key was used for different intent")
		}
		return RecordVerificationSnapshotReceiptResult{Receipt: existing, Replayed: true}, nil
	}
	run, mission, linkedSession, registered, err := loadVerificationCoverageBinding(ctx,
		s.store, request.RunID)
	if err != nil {
		return RecordVerificationSnapshotReceiptResult{}, err
	}
	if linkedSession.Status != session.StatusActive {
		return RecordVerificationSnapshotReceiptResult{}, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt requires an active Code Session")
	}
	plan, err := s.store.GetVerificationPlan(ctx, request.PlanID)
	if err != nil {
		return RecordVerificationSnapshotReceiptResult{}, apperror.Normalize(err)
	}
	if err := plan.Validate(); err != nil || plan.RunID != run.ID ||
		plan.SessionID != linkedSession.ID || plan.WorkspaceID != mission.WorkspaceID ||
		request.PlanItemOrdinal > len(plan.Items) {
		return RecordVerificationSnapshotReceiptResult{}, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt escaped its exact plan binding")
	}
	exported, err := NewVerificationSnapshotExportService(s.store).Build(ctx, run.ID,
		plan.ID, request.PlanItemOrdinal, request.Format)
	if err != nil {
		return RecordVerificationSnapshotReceiptResult{}, err
	}
	if exported.SnapshotHighWaterEventSequence != request.SnapshotHighWaterEventSequence ||
		exported.ContentSHA256 != request.ContentSHA256 || !exported.MetadataOnly ||
		!exported.ReadOnly || !exported.DownloadOnly || exported.PrivatePlanBodyIncluded ||
		exported.PrivateEvidenceBodiesIncluded || exported.OperatorIdentityIncluded ||
		exported.ResultInferred || exported.CommandExecuted || exported.ModelAssertion ||
		exported.RecordRewritten || exported.Approval || exported.AuthorityGranted ||
		exported.MutationSupported || exported.ExecutionStarted {
		return RecordVerificationSnapshotReceiptResult{}, apperror.New(apperror.CodeConflict,
			"verification snapshot receipt does not match the current deterministic snapshot")
	}
	now := s.now().UTC()
	if now.Before(run.CreatedAt) {
		now = run.CreatedAt
	}
	if now.Before(plan.CreatedAt) {
		now = plan.CreatedAt
	}
	receipt := verification.SnapshotReceipt{
		ID:                 idgen.New("verification-snapshot-receipt"),
		ProtocolVersion:    verification.SnapshotReceiptProtocolVersion,
		OperationKeyDigest: keyDigest,
		RequestFingerprint: runmutation.VerificationSnapshotReceiptRequestFingerprint(run.ID,
			linkedSession.ID, registered.ID, plan.ID, request.PlanItemOrdinal, request.Format,
			request.SnapshotHighWaterEventSequence, request.ContentSHA256, request.RecordedBy),
		RunID: run.ID, SessionID: linkedSession.ID, WorkspaceID: registered.ID,
		PlanID: plan.ID, PlanSHA256: plan.PlanSHA256,
		PlanItemOrdinal:                request.PlanItemOrdinal,
		PlanItemSHA256:                 plan.Items[request.PlanItemOrdinal-1].ItemSHA256,
		Format:                         request.Format,
		SnapshotHighWaterEventSequence: exported.SnapshotHighWaterEventSequence,
		AssociatedEvidenceCount:        exported.AssociatedEvidenceCount,
		PassCount:                      exported.PassCount, FailCount: exported.FailCount,
		UnknownCount:             exported.UnknownCount,
		ReturnedAssociationCount: exported.ReturnedAssociationCount,
		AssociationsTruncated:    exported.AssociationsTruncated,
		ContentSHA256:            exported.ContentSHA256, ContentBytes: exported.ContentBytes,
		RecordedBy: request.RecordedBy, CreatedAt: now,
	}
	prepared := receipt
	prepared.EventSequence = receipt.SnapshotHighWaterEventSequence + 1
	if err := prepared.Validate(); err != nil {
		return RecordVerificationSnapshotReceiptResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "verification snapshot receipt is invalid", err)
	}
	stored, replayed, err := s.store.RecordVerificationSnapshotReceipt(ctx, receipt)
	if err != nil {
		return RecordVerificationSnapshotReceiptResult{}, apperror.Normalize(err)
	}
	return RecordVerificationSnapshotReceiptResult{Receipt: stored, Replayed: replayed}, nil
}

func (s *VerificationSnapshotReceiptService) Inventory(ctx context.Context,
	runID string,
) (VerificationSnapshotReceiptInventory, error) {
	if s == nil || s.store == nil {
		return VerificationSnapshotReceiptInventory{}, apperror.New(
			apperror.CodeFailedPrecondition, "verification snapshot receipt store is required")
	}
	run, mission, linkedSession, _, err := loadVerificationCoverageBinding(ctx, s.store, runID)
	if err != nil {
		return VerificationSnapshotReceiptInventory{}, err
	}
	values, err := s.store.ListVerificationSnapshotReceipts(ctx, run.ID,
		verification.MaxSnapshotReceiptHistory+1)
	if err != nil {
		return VerificationSnapshotReceiptInventory{}, apperror.Normalize(err)
	}
	result := VerificationSnapshotReceiptInventory{
		ProtocolVersion: verification.SnapshotReceiptInventoryProtocolVersion,
		RunID:           run.ID, SessionID: linkedSession.ID, WorkspaceID: mission.WorkspaceID,
		Truncated:    len(values) > verification.MaxSnapshotReceiptHistory,
		MetadataOnly: true, ReadOnly: true,
	}
	if result.Truncated {
		values = values[:verification.MaxSnapshotReceiptHistory]
	}
	result.Items = append([]verification.SnapshotReceipt{}, values...)
	for _, value := range result.Items {
		if err := value.Validate(); err != nil || value.RunID != run.ID ||
			value.SessionID != linkedSession.ID || value.WorkspaceID != mission.WorkspaceID {
			return VerificationSnapshotReceiptInventory{}, apperror.New(apperror.CodeConflict,
				"verification snapshot receipt escaped its Run binding")
		}
	}
	return result, nil
}
