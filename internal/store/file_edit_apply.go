package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/fileedit"
)

const fileEditApplyOperationSelect = `SELECT protocol_version, operation_key_digest,
	request_fingerprint, run_id, session_id, workspace_id, edit_id, path,
	original_hash, proposed_hash, observed_hash, applied_by, event_sequence, created_at
	FROM file_edit_apply_operations `

func (s *SQLiteStore) GetFileEditApplyOperation(ctx context.Context,
	keyDigest string,
) (fileedit.ApplyOperation, *fileedit.ApplyResult, bool, error) {
	operation, err := scanFileEditApplyOperation(s.db.QueryRowContext(ctx,
		fileEditApplyOperationSelect+`WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return fileedit.ApplyOperation{}, nil, false, nil
	}
	if err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	}
	result, found, err := getFileEditApplyResult(ctx, s.db, keyDigest)
	if err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	}
	if found {
		return operation, &result, true, nil
	}
	return operation, nil, true, nil
}

func (s *SQLiteStore) PrepareFileEditApply(ctx context.Context,
	operation fileedit.ApplyOperation,
) (fileedit.ApplyOperation, *fileedit.ApplyResult, bool, error) {
	if operation.ProtocolVersion != fileedit.FileEditApplyProtocolVersion ||
		operation.EventSequence != 0 || operation.CreatedAt.IsZero() {
		return fileedit.ApplyOperation{}, nil, false,
			errors.New("FileEdit apply operation preparation is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, found, err := getFileEditApplyOperationTx(ctx, tx, operation.KeyDigest)
	if err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	}
	if found {
		if !sameFileEditApplyOperation(existing, operation) {
			return fileedit.ApplyOperation{}, nil, false,
				errors.New("FileEdit apply operation key was used for different intent")
		}
		result, resultFound, err := getFileEditApplyResult(ctx, tx,
			operation.KeyDigest)
		if err != nil {
			return fileedit.ApplyOperation{}, nil, false, err
		}
		if resultFound {
			return existing, &result, true, nil
		}
		return existing, nil, true, nil
	}
	if _, editFound, err := getFileEditApplyOperationByEditTx(ctx, tx,
		operation.EditID); err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	} else if editFound {
		return fileedit.ApplyOperation{}, nil, false, apperror.New(
			apperror.CodeConflict, "FileEdit already has a prepared apply operation")
	}
	event, err := appendRunWakeEventTx(ctx, tx, operation.RunID,
		events.FileEditApplyRequestedEvent, "file_edit_apply", operation.EditID,
		map[string]any{
			"operation_key_digest": operation.KeyDigest,
			"observed_hash":        operation.ObservedHash,
			"proposed_hash":        operation.ProposedHash,
			"policy_rechecked":     true,
		}, operation.CreatedAt)
	if err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	}
	operation.EventSequence = event.Sequence
	if err := operation.Validate(); err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO file_edit_apply_operations
		(operation_key_digest, request_fingerprint, protocol_version, run_id,
		 session_id, workspace_id, edit_id, path, original_hash, proposed_hash,
		 observed_hash, applied_by, event_sequence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.ProtocolVersion, operation.RunID,
		operation.SessionID, operation.WorkspaceID, operation.EditID, operation.Path,
		operation.OriginalHash, operation.ProposedHash, operation.ObservedHash,
		operation.AppliedBy, operation.EventSequence, ts(operation.CreatedAt)); err != nil {
		return fileedit.ApplyOperation{}, nil, false, normalizeFileEditApplyStoreError(err)
	}
	if err := tx.Commit(); err != nil {
		return fileedit.ApplyOperation{}, nil, false, err
	}
	return operation, nil, false, nil
}

func (s *SQLiteStore) CompleteFileEditApply(ctx context.Context,
	result fileedit.ApplyResult,
) (fileedit.ApplyResult, bool, error) {
	if result.EventSequence != 0 || result.CompletedAt.IsZero() ||
		(result.Status != fileedit.ApplyCompleted && result.Status != fileedit.ApplyFailed) {
		return fileedit.ApplyResult{}, false,
			errors.New("FileEdit apply completion is invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fileedit.ApplyResult{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	operation, found, err := getFileEditApplyOperationTx(ctx, tx,
		result.OperationKeyDigest)
	if err != nil {
		return fileedit.ApplyResult{}, false, err
	}
	if !found {
		return fileedit.ApplyResult{}, false,
			errors.New("FileEdit apply operation was not prepared")
	}
	if existing, resultFound, err := getFileEditApplyResult(ctx, tx,
		result.OperationKeyDigest); err != nil {
		return fileedit.ApplyResult{}, false, err
	} else if resultFound {
		if existing.Status != result.Status || existing.ReasonCode != result.ReasonCode {
			return fileedit.ApplyResult{}, false,
				errors.New("FileEdit apply operation already has a different result")
		}
		return existing, true, nil
	}
	event, err := appendRunWakeEventTx(ctx, tx, operation.RunID,
		events.FileEditApplyCompletedEvent, "file_edit_apply", operation.EditID,
		map[string]any{
			"operation_key_digest": operation.KeyDigest,
			"status":               string(result.Status), "reason_code": result.ReasonCode,
		}, result.CompletedAt)
	if err != nil {
		return fileedit.ApplyResult{}, false, err
	}
	result.EventSequence = event.Sequence
	if err := result.Validate(); err != nil {
		return fileedit.ApplyResult{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO file_edit_apply_results
		(operation_key_digest, status, reason_code, event_sequence, completed_at)
		VALUES (?, ?, ?, ?, ?)`, result.OperationKeyDigest, result.Status,
		result.ReasonCode, result.EventSequence, ts(result.CompletedAt)); err != nil {
		return fileedit.ApplyResult{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return fileedit.ApplyResult{}, false, err
	}
	return result, false, nil
}

func scanFileEditApplyOperation(row scanner) (fileedit.ApplyOperation, error) {
	var value fileedit.ApplyOperation
	var createdAt string
	if err := row.Scan(&value.ProtocolVersion, &value.KeyDigest,
		&value.RequestFingerprint, &value.RunID, &value.SessionID, &value.WorkspaceID,
		&value.EditID, &value.Path, &value.OriginalHash, &value.ProposedHash,
		&value.ObservedHash, &value.AppliedBy, &value.EventSequence, &createdAt); err != nil {
		return fileedit.ApplyOperation{}, err
	}
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return fileedit.ApplyOperation{}, fmt.Errorf(
			"stored FileEdit apply operation is invalid: %w", err)
	}
	return value, nil
}

func scanFileEditApplyResult(row scanner) (fileedit.ApplyResult, error) {
	var value fileedit.ApplyResult
	var status, completedAt string
	if err := row.Scan(&value.OperationKeyDigest, &status, &value.ReasonCode,
		&value.EventSequence, &completedAt); err != nil {
		return fileedit.ApplyResult{}, err
	}
	value.Status = fileedit.ApplyStatus(status)
	value.CompletedAt = parseTS(completedAt)
	if err := value.Validate(); err != nil {
		return fileedit.ApplyResult{}, fmt.Errorf(
			"stored FileEdit apply result is invalid: %w", err)
	}
	return value, nil
}

func getFileEditApplyOperationTx(ctx context.Context, tx *sql.Tx, keyDigest string,
) (fileedit.ApplyOperation, bool, error) {
	value, err := scanFileEditApplyOperation(tx.QueryRowContext(ctx,
		fileEditApplyOperationSelect+`WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return fileedit.ApplyOperation{}, false, nil
	}
	return value, err == nil, err
}

func getFileEditApplyOperationByEditTx(ctx context.Context, tx *sql.Tx, editID string,
) (fileedit.ApplyOperation, bool, error) {
	value, err := scanFileEditApplyOperation(tx.QueryRowContext(ctx,
		fileEditApplyOperationSelect+`WHERE edit_id = ?`, editID))
	if errors.Is(err, sql.ErrNoRows) {
		return fileedit.ApplyOperation{}, false, nil
	}
	return value, err == nil, err
}

func normalizeFileEditApplyStoreError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
		return apperror.Wrap(apperror.CodeConflict,
			"FileEdit already has a prepared apply operation", err)
	}
	return err
}

type fileEditApplyQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getFileEditApplyResult(ctx context.Context, queryer fileEditApplyQueryer,
	keyDigest string,
) (fileedit.ApplyResult, bool, error) {
	value, err := scanFileEditApplyResult(queryer.QueryRowContext(ctx, `SELECT
		operation_key_digest, status, reason_code, event_sequence, completed_at
		FROM file_edit_apply_results WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return fileedit.ApplyResult{}, false, nil
	}
	return value, err == nil, err
}

func sameFileEditApplyOperation(stored fileedit.ApplyOperation,
	requested fileedit.ApplyOperation,
) bool {
	return stored.ProtocolVersion == requested.ProtocolVersion &&
		stored.KeyDigest == requested.KeyDigest &&
		stored.RequestFingerprint == requested.RequestFingerprint &&
		stored.RunID == requested.RunID && stored.SessionID == requested.SessionID &&
		stored.WorkspaceID == requested.WorkspaceID && stored.EditID == requested.EditID &&
		stored.Path == requested.Path && stored.OriginalHash == requested.OriginalHash &&
		stored.ProposedHash == requested.ProposedHash &&
		stored.ObservedHash == requested.ObservedHash && stored.AppliedBy == requested.AppliedBy
}
