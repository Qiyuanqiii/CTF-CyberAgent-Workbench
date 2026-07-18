package store

import (
	"context"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/operationreceipt"
)

func (s *SQLiteStore) ListTerminalOperationRecords(ctx context.Context,
	runID string, limit int,
) ([]operationreceipt.TerminalRecord, error) {
	if runID != strings.TrimSpace(runID) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"operation receipt history Run identity is invalid")
	}
	if limit < 1 || limit > operationreceipt.MaxHistoryItems+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"operation receipt history limit is invalid")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_id, kind, run_id, workspace_id,
		path, proposed_hash, outcome, completed_at FROM (
			SELECT operation.operation_key_digest AS source_id,
				'file_edit_apply' AS kind, operation.run_id AS run_id,
				operation.workspace_id AS workspace_id, operation.path AS path,
				operation.proposed_hash AS proposed_hash, result.status AS outcome,
				result.completed_at AS completed_at
			FROM file_edit_apply_operations operation
			JOIN file_edit_apply_results result
				ON result.operation_key_digest = operation.operation_key_digest
			UNION ALL
			SELECT consumption.id AS source_id, 'run_wake_consume' AS kind,
				consumption.run_id AS run_id, '' AS workspace_id, '' AS path,
				'' AS proposed_hash, consumption.status AS outcome,
				consumption.completed_at AS completed_at
			FROM run_wake_consumptions consumption
			WHERE consumption.status IN ('completed', 'failed')
			UNION ALL
			SELECT result.installation_id AS source_id, 'skill_package_install' AS kind,
				'' AS run_id, '' AS workspace_id, '' AS path, '' AS proposed_hash,
				'installed' AS outcome, result.completed_at AS completed_at
			FROM skill_package_install_results result
		) terminal
		WHERE (? = '' OR terminal.run_id = ?)
		ORDER BY terminal.completed_at DESC, terminal.kind, terminal.source_id
		LIMIT ?`, runID, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]operationreceipt.TerminalRecord, 0, limit)
	for rows.Next() {
		var value operationreceipt.TerminalRecord
		var kind, completed string
		if err := rows.Scan(&value.SourceID, &kind, &value.RunID, &value.WorkspaceID,
			&value.Path, &value.ProposedHash, &value.Outcome, &completed); err != nil {
			return nil, err
		}
		value.Kind = operationreceipt.Kind(kind)
		value.CompletedAt = parseTS(completed)
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("stored terminal operation receipt is invalid: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
