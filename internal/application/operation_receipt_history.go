package application

import (
	"context"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/operationreceipt"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
)

const DefaultOperationReceiptHistoryLimit = 50

type OperationReceiptHistoryStore interface {
	GetRun(context.Context, string) (domain.Run, error)
	GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error)
	ListTerminalOperationRecords(context.Context, string, int) (
		[]operationreceipt.TerminalRecord, error)
}

type OperationReceiptHistoryService struct {
	store OperationReceiptHistoryStore
	now   func() time.Time
}

type ListOperationReceiptHistoryRequest struct {
	RunID string
	Limit int
}

func NewOperationReceiptHistoryService(
	store OperationReceiptHistoryStore,
) *OperationReceiptHistoryService {
	return &OperationReceiptHistoryService{store: store,
		now: func() time.Time { return time.Now().UTC() }}
}

func (s *OperationReceiptHistoryService) List(ctx context.Context,
	request ListOperationReceiptHistoryRequest,
) (operationreceipt.History, error) {
	if s == nil || s.store == nil || s.now == nil {
		return operationreceipt.History{}, apperror.New(apperror.CodeFailedPrecondition,
			"operation receipt history store is required")
	}
	runID := strings.TrimSpace(request.RunID)
	if runID != request.RunID || (runID != "" && !domain.ValidAgentID(runID)) {
		return operationreceipt.History{}, apperror.New(apperror.CodeInvalidArgument,
			"operation receipt history Run identity is invalid")
	}
	limit := request.Limit
	if limit == 0 {
		limit = DefaultOperationReceiptHistoryLimit
	}
	if limit < 1 || limit > operationreceipt.MaxHistoryItems {
		return operationreceipt.History{}, apperror.New(apperror.CodeInvalidArgument,
			"operation receipt history limit is invalid")
	}
	if runID != "" {
		run, err := s.store.GetRun(ctx, runID)
		if err != nil {
			return operationreceipt.History{}, apperror.Normalize(err)
		}
		if run.ID != runID {
			return operationreceipt.History{}, apperror.New(apperror.CodeConflict,
				"operation receipt history Run lookup changed")
		}
	}
	records, err := s.store.ListTerminalOperationRecords(ctx, runID, limit+1)
	if err != nil {
		return operationreceipt.History{}, apperror.Normalize(err)
	}
	truncated := len(records) > limit
	if truncated {
		records = records[:limit]
	}
	items := make([]operationreceipt.HistoryItem, 0, len(records))
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return operationreceipt.History{}, apperror.Wrap(apperror.CodeInternal,
				"terminal operation receipt is invalid", err)
		}
		if runID != "" && record.RunID != runID {
			return operationreceipt.History{}, apperror.New(apperror.CodeConflict,
				"terminal operation receipt escaped the requested Run filter")
		}
		var receipt operationreceipt.Receipt
		scope := "run"
		switch record.Kind {
		case operationreceipt.KindFileEditApply:
			pending := true
			workspace, lookupErr := s.store.GetWorkspaceInfo(ctx, record.WorkspaceID)
			if lookupErr == nil && workspace.ID == record.WorkspaceID {
				inspection, inspectErr := fileedit.InspectStaging(workspace.RootPath,
					record.Path, record.ProposedHash, s.now().UTC())
				if inspectErr == nil {
					pending = inspection.Pending
				}
			}
			receipt = operationreceipt.FileEditApply(record.Outcome, false, pending)
		case operationreceipt.KindRunWakeConsume:
			receipt = operationreceipt.RunWakeConsume(record.Outcome, false)
		case operationreceipt.KindSkillPackageInstall:
			receipt = operationreceipt.Settled(record.Kind, false, false)
			scope = "skill_registry"
		default:
			return operationreceipt.History{}, apperror.New(apperror.CodeInternal,
				"terminal operation receipt kind is unsupported")
		}
		if err := receipt.Validate(); err != nil {
			return operationreceipt.History{}, apperror.Wrap(apperror.CodeInternal,
				"operation receipt projection is invalid", err)
		}
		identity := runmutation.Fingerprint("operation_receipt_history_item.v1",
			string(record.Kind), record.SourceID)
		items = append(items, operationreceipt.HistoryItem{
			ID: "receipt-" + identity[:24], Scope: scope, RunID: record.RunID,
			CompletedAt: record.CompletedAt, Receipt: receipt,
		})
	}
	return operationreceipt.History{ProtocolVersion: operationreceipt.HistoryProtocolVersion,
		Items: items, Truncated: truncated}, nil
}
