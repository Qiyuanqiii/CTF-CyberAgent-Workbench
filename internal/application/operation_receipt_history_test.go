package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/operationreceipt"
	"cyberagent-workbench/internal/session"
)

type operationReceiptHistoryTestStore struct {
	run              domain.Run
	workspace        session.WorkspaceInfo
	records          []operationreceipt.TerminalRecord
	workspaceLookups int
}

func (s *operationReceiptHistoryTestStore) GetRun(_ context.Context,
	id string,
) (domain.Run, error) {
	if id != s.run.ID {
		return domain.Run{}, errors.New("Run not found")
	}
	return s.run, nil
}

func (s *operationReceiptHistoryTestStore) GetWorkspaceInfo(_ context.Context,
	id string,
) (session.WorkspaceInfo, error) {
	s.workspaceLookups++
	if id != s.workspace.ID {
		return session.WorkspaceInfo{}, errors.New("Workspace not found")
	}
	return s.workspace, nil
}

func (s *operationReceiptHistoryTestStore) ListTerminalOperationRecords(_ context.Context,
	_ string, _ int,
) ([]operationreceipt.TerminalRecord, error) {
	return append([]operationreceipt.TerminalRecord(nil), s.records...), nil
}

func TestOperationReceiptHistoryProjectsOnlyOpaqueTerminalMetadata(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	privateHash := strings.Repeat("a", 64)
	state := &operationReceiptHistoryTestStore{
		run:       domain.Run{ID: "run-history"},
		workspace: session.WorkspaceInfo{ID: "workspace-history", RootPath: t.TempDir()},
		records: []operationreceipt.TerminalRecord{
			{SourceID: "PRIVATE-file-operation", Kind: operationreceipt.KindFileEditApply,
				RunID: "run-history", WorkspaceID: "workspace-history",
				Path: "README.md", ProposedHash: privateHash,
				Outcome: "applied", CompletedAt: now.Add(-time.Minute)},
			{SourceID: "PRIVATE-wake-operation", Kind: operationreceipt.KindRunWakeConsume,
				RunID: "run-history", Outcome: "failed", CompletedAt: now.Add(-2 * time.Minute)},
			{SourceID: "PRIVATE-skill-operation", Kind: operationreceipt.KindSkillPackageInstall,
				Outcome: "installed", CompletedAt: now.Add(-3 * time.Minute)},
		},
	}
	service := NewOperationReceiptHistoryService(state)
	service.now = func() time.Time { return now }

	history, err := service.List(t.Context(), ListOperationReceiptHistoryRequest{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if history.ProtocolVersion != operationreceipt.HistoryProtocolVersion ||
		history.Truncated || len(history.Items) != 3 || state.workspaceLookups != 1 {
		t.Fatalf("unexpected receipt history: %#v", history)
	}
	if history.Items[0].Receipt.Kind != operationreceipt.KindFileEditApply ||
		history.Items[0].Receipt.CleanupState != operationreceipt.CleanupComplete ||
		history.Items[1].Receipt.Outcome != "failed" ||
		history.Items[2].Scope != "skill_registry" || history.Items[2].RunID != "" {
		t.Fatalf("terminal receipt projection drifted: %#v", history.Items)
	}
	for _, item := range history.Items {
		if !strings.HasPrefix(item.ID, "receipt-") || item.Receipt.Replayed {
			t.Fatalf("receipt identity or replay projection is invalid: %#v", item)
		}
	}
	raw, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"PRIVATE", "README.md", privateHash, "workspace-history"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("receipt history disclosed %q: %s", forbidden, serialized)
		}
	}
}

func TestOperationReceiptHistoryRejectsInvalidBoundsAndStoredRecords(t *testing.T) {
	state := &operationReceiptHistoryTestStore{run: domain.Run{ID: "run-history"}}
	service := NewOperationReceiptHistoryService(state)
	if _, err := service.List(t.Context(), ListOperationReceiptHistoryRequest{
		RunID: " run-history",
	}); err == nil {
		t.Fatal("non-normalized Run identity was accepted")
	}
	if _, err := service.List(t.Context(), ListOperationReceiptHistoryRequest{
		Limit: operationreceipt.MaxHistoryItems + 1,
	}); err == nil {
		t.Fatal("unbounded receipt history limit was accepted")
	}
	state.records = []operationreceipt.TerminalRecord{{SourceID: "invalid"}}
	if _, err := service.List(t.Context(), ListOperationReceiptHistoryRequest{}); err == nil {
		t.Fatal("invalid stored terminal record was projected")
	}
	state.records = []operationreceipt.TerminalRecord{{
		SourceID: "wake-other", Kind: operationreceipt.KindRunWakeConsume,
		RunID: "run-other", Outcome: "completed", CompletedAt: time.Now().UTC(),
	}}
	if _, err := service.List(t.Context(), ListOperationReceiptHistoryRequest{
		RunID: "run-history",
	}); err == nil {
		t.Fatal("terminal record escaped its requested Run filter")
	}
}
