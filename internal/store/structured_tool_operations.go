package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
)

func (s *SQLiteStore) CreateWorkItemToolOperation(ctx context.Context, operation runmutation.Operation,
	item domain.WorkItem, policyEvent events.Event, itemEvent events.Event, toolEvent events.Event,
) (domain.WorkItem, bool, error) {
	item = redactAndNormalizeWorkItem(item)
	operation = normalizeStructuredOperation(operation)
	if err := validateStructuredWorkItemCreate(operation, item, policyEvent, itemEvent, toolEvent); err != nil {
		return domain.WorkItem{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.WorkItem{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireStructuredMutationWriteLockTx(ctx, tx, operation.RunID); err != nil {
		return domain.WorkItem{}, false, err
	}
	if err := requireStructuredMutationExecutionLeaseTx(ctx, tx, operation); err != nil {
		return domain.WorkItem{}, false, err
	}

	existing, found, err := getStructuredOperationByKeyTx(ctx, tx, operation.KeyDigest)
	if err != nil {
		return domain.WorkItem{}, false, err
	}
	if found {
		if err := validateStructuredOperationReplay(existing, operation); err != nil {
			return domain.WorkItem{}, false, err
		}
		stored, err := getWorkItemTx(ctx, tx, existing.TargetID)
		return stored, true, err
	}
	missionID, err := requireStructuredOperationBindingTx(ctx, tx, operation)
	if err != nil {
		return domain.WorkItem{}, false, err
	}
	if itemEvent.MissionID != missionID || toolEvent.MissionID != missionID {
		return domain.WorkItem{}, false, apperror.New(apperror.CodeInvalidArgument,
			"structured WorkItem events do not match the Run mission")
	}
	if err := insertNewWorkItemTx(ctx, tx, item); err != nil {
		return domain.WorkItem{}, false, err
	}
	if err := insertStructuredOperationTx(ctx, tx, operation); err != nil {
		_ = tx.Rollback()
		return s.recoverWorkItemToolOperation(ctx, operation, err)
	}
	if _, err := insertRunEventTx(ctx, tx, policyEvent); err != nil {
		return domain.WorkItem{}, false, err
	}
	if _, err := insertRunEventTx(ctx, tx, itemEvent); err != nil {
		return domain.WorkItem{}, false, err
	}
	if _, err := insertRunEventTx(ctx, tx, toolEvent); err != nil {
		return domain.WorkItem{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.WorkItem{}, false, err
	}
	return item, false, nil
}

func (s *SQLiteStore) CreateNoteToolOperation(ctx context.Context, operation runmutation.Operation,
	note domain.Note, policyEvent events.Event, noteEvent events.Event, toolEvent events.Event,
) (domain.Note, bool, error) {
	note = redactAndNormalizeNote(note)
	operation = normalizeStructuredOperation(operation)
	if err := validateStructuredNoteCreate(operation, note, policyEvent, noteEvent, toolEvent); err != nil {
		return domain.Note{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.Note{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireStructuredMutationWriteLockTx(ctx, tx, operation.RunID); err != nil {
		return domain.Note{}, false, err
	}
	if err := requireStructuredMutationExecutionLeaseTx(ctx, tx, operation); err != nil {
		return domain.Note{}, false, err
	}

	existing, found, err := getStructuredOperationByKeyTx(ctx, tx, operation.KeyDigest)
	if err != nil {
		return domain.Note{}, false, err
	}
	if found {
		if err := validateStructuredOperationReplay(existing, operation); err != nil {
			return domain.Note{}, false, err
		}
		stored, err := getNoteTx(ctx, tx, existing.TargetID)
		return stored, true, err
	}
	missionID, err := requireStructuredOperationBindingTx(ctx, tx, operation)
	if err != nil {
		return domain.Note{}, false, err
	}
	if noteEvent.MissionID != missionID || toolEvent.MissionID != missionID {
		return domain.Note{}, false, apperror.New(apperror.CodeInvalidArgument,
			"structured Note events do not match the Run mission")
	}
	if err := insertNewNoteTx(ctx, tx, note); err != nil {
		return domain.Note{}, false, err
	}
	if err := insertStructuredOperationTx(ctx, tx, operation); err != nil {
		_ = tx.Rollback()
		return s.recoverNoteToolOperation(ctx, operation, err)
	}
	if _, err := insertRunEventTx(ctx, tx, policyEvent); err != nil {
		return domain.Note{}, false, err
	}
	if _, err := insertRunEventTx(ctx, tx, noteEvent); err != nil {
		return domain.Note{}, false, err
	}
	if _, err := insertRunEventTx(ctx, tx, toolEvent); err != nil {
		return domain.Note{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Note{}, false, err
	}
	return note, false, nil
}

func acquireStructuredMutationWriteLockTx(ctx context.Context, tx *sql.Tx, runID string) error {
	var rows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_tool_usage WHERE run_id = ?`, runID).Scan(&rows); err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeFailedPrecondition,
			"structured mutation requires initialized Run tool usage")
	}
	return nil
}

func validateStructuredWorkItemCreate(operation runmutation.Operation, item domain.WorkItem,
	policyEvent events.Event, itemEvent events.Event, toolEvent events.Event,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if operation.TargetKind != runmutation.TargetWorkItem || operation.TargetID != item.ID ||
		operation.RunID != item.RunID || operation.ToolName != "work_item_create" {
		return apperror.New(apperror.CodeInvalidArgument, "structured WorkItem operation does not match its target")
	}
	if err := validateNewWorkItem(item, itemEvent); err != nil {
		return err
	}
	if err := validateStructuredPolicyEvent(operation, policyEvent); err != nil {
		return err
	}
	return validateStructuredToolEvent(operation, toolEvent)
}

func validateStructuredNoteCreate(operation runmutation.Operation, note domain.Note,
	policyEvent events.Event, noteEvent events.Event, toolEvent events.Event,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if operation.TargetKind != runmutation.TargetNote || operation.TargetID != note.ID ||
		operation.RunID != note.RunID || operation.ToolName != "note_create" {
		return apperror.New(apperror.CodeInvalidArgument, "structured Note operation does not match its target")
	}
	if err := validateNewNote(note, noteEvent); err != nil {
		return err
	}
	if err := validateStructuredPolicyEvent(operation, policyEvent); err != nil {
		return err
	}
	return validateStructuredToolEvent(operation, toolEvent)
}

func validateStructuredPolicyEvent(operation runmutation.Operation, event events.Event) error {
	if event.Type != events.PolicyDecisionEvent || event.RunID != operation.RunID ||
		event.SubjectID != operation.InvocationID {
		return apperror.New(apperror.CodeInvalidArgument, "structured mutation Policy event does not match the operation")
	}
	if err := event.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return nil
}

func validateStructuredToolEvent(operation runmutation.Operation, event events.Event) error {
	if event.Type != events.ToolCompletedEvent || event.RunID != operation.RunID ||
		event.SubjectID != operation.InvocationID {
		return apperror.New(apperror.CodeInvalidArgument, "structured mutation completion event does not match the operation")
	}
	if err := event.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return nil
}

func normalizeStructuredOperation(operation runmutation.Operation) runmutation.Operation {
	operation.InvocationID = strings.TrimSpace(operation.InvocationID)
	operation.RunID = strings.TrimSpace(operation.RunID)
	operation.SessionID = strings.TrimSpace(operation.SessionID)
	operation.WorkspaceID = strings.TrimSpace(operation.WorkspaceID)
	operation.LeaseID = strings.TrimSpace(operation.LeaseID)
	operation.ToolName = strings.TrimSpace(operation.ToolName)
	operation.TargetID = strings.TrimSpace(operation.TargetID)
	operation.RequestedBy = strings.TrimSpace(redact.String(operation.RequestedBy))
	return operation
}

func requireStructuredMutationExecutionLeaseTx(ctx context.Context, tx *sql.Tx,
	operation runmutation.Operation,
) error {
	if operation.RequestedBy != "run_supervisor" {
		return nil
	}
	return requireRunExecutionLeaseTx(ctx, tx, operation.RunID, operation.LeaseID,
		operation.LeaseGeneration)
}

func requireStructuredOperationBindingTx(ctx context.Context, tx *sql.Tx, operation runmutation.Operation) (string, error) {
	var missionID, sessionID string
	var workspaceID sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT runs.mission_id, runs.session_id, missions.workspace_id
		FROM runs JOIN missions ON missions.id = runs.mission_id WHERE runs.id = ?`, operation.RunID).
		Scan(&missionID, &sessionID, &workspaceID); err != nil {
		return "", err
	}
	if sessionID != operation.SessionID || workspaceID.String != operation.WorkspaceID {
		return "", apperror.New(apperror.CodeFailedPrecondition,
			"structured mutation scope does not match its Run, Session, and Workspace")
	}
	var invocationCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_tool_calls
		WHERE id = ? AND run_id = ? AND session_id = ? AND workspace_id = ? AND tool_name = ?
			AND action_class = 'run_memory'`,
		operation.InvocationID, operation.RunID, operation.SessionID, operation.WorkspaceID,
		operation.ToolName).Scan(&invocationCount); err != nil {
		return "", err
	}
	if invocationCount != 1 {
		return "", apperror.New(apperror.CodeFailedPrecondition,
			"structured mutation invocation is not backed by the Run tool budget ledger")
	}
	if _, err := mutableRunMissionTx(ctx, tx, operation.RunID); err != nil {
		return "", err
	}
	return missionID, nil
}

func insertStructuredOperationTx(ctx context.Context, tx *sql.Tx, operation runmutation.Operation) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO structured_tool_operations
		(operation_key_digest, request_fingerprint, invocation_id, run_id, session_id, workspace_id,
		 tool_name, target_kind, target_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest, operation.RequestFingerprint,
		operation.InvocationID, operation.RunID, operation.SessionID, operation.WorkspaceID,
		operation.ToolName, operation.TargetKind, operation.TargetID, operation.RequestedBy, ts(operation.CreatedAt))
	return err
}

func validateStructuredOperationReplay(existing runmutation.Operation, request runmutation.Operation) error {
	if existing.KeyDigest != request.KeyDigest || existing.RequestFingerprint != request.RequestFingerprint ||
		existing.RunID != request.RunID || existing.SessionID != request.SessionID ||
		existing.WorkspaceID != request.WorkspaceID || existing.ToolName != request.ToolName ||
		existing.TargetKind != request.TargetKind || existing.RequestedBy != request.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"structured mutation idempotency key was already used for different intent")
	}
	return nil
}

func (s *SQLiteStore) recoverWorkItemToolOperation(ctx context.Context, operation runmutation.Operation,
	original error,
) (domain.WorkItem, bool, error) {
	existing, err := getStructuredOperationByKey(ctx, s.db, operation.KeyDigest)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.WorkItem{}, false, original
		}
		return domain.WorkItem{}, false, errors.Join(original, err)
	}
	if err := validateStructuredOperationReplay(existing, operation); err != nil {
		return domain.WorkItem{}, false, err
	}
	item, err := s.GetWorkItem(ctx, existing.TargetID)
	return item, true, err
}

func (s *SQLiteStore) recoverNoteToolOperation(ctx context.Context, operation runmutation.Operation,
	original error,
) (domain.Note, bool, error) {
	existing, err := getStructuredOperationByKey(ctx, s.db, operation.KeyDigest)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Note{}, false, original
		}
		return domain.Note{}, false, errors.Join(original, err)
	}
	if err := validateStructuredOperationReplay(existing, operation); err != nil {
		return domain.Note{}, false, err
	}
	note, err := s.GetNote(ctx, existing.TargetID)
	return note, true, err
}

func getStructuredOperationByKeyTx(ctx context.Context, tx *sql.Tx, keyDigest string) (runmutation.Operation, bool, error) {
	operation, err := getStructuredOperationByKey(ctx, tx, keyDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return runmutation.Operation{}, false, nil
	}
	return operation, err == nil, err
}

func getStructuredOperationByKey(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (runmutation.Operation, error) {
	var operation runmutation.Operation
	var targetKind string
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint, invocation_id,
		run_id, session_id, workspace_id, tool_name, target_kind, target_id, requested_by, created_at
		FROM structured_tool_operations WHERE operation_key_digest = ?`, strings.TrimSpace(keyDigest)).Scan(
		&operation.KeyDigest, &operation.RequestFingerprint, &operation.InvocationID, &operation.RunID,
		&operation.SessionID, &operation.WorkspaceID, &operation.ToolName, &targetKind, &operation.TargetID,
		&operation.RequestedBy, &createdAt)
	if err != nil {
		return runmutation.Operation{}, err
	}
	operation.TargetKind = runmutation.TargetKind(targetKind)
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.ValidateStored(); err != nil {
		return runmutation.Operation{}, fmt.Errorf("invalid stored structured mutation: %w", err)
	}
	return operation, nil
}
