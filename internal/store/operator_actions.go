package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/operatoraction"
)

func (s *SQLiteStore) ListOperatorActionRecords(ctx context.Context, runID string,
	sessionID string, workspaceID string, now time.Time, limit int,
) ([]operatoraction.Record, error) {
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) ||
		sessionID != strings.TrimSpace(sessionID) ||
		(sessionID != "" && !domain.ValidAgentID(sessionID)) ||
		workspaceID != strings.TrimSpace(workspaceID) ||
		(workspaceID != "" && !domain.ValidAgentID(workspaceID)) || now.IsZero() {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"operator action query binding is invalid")
	}
	if limit < 1 || limit > operatoraction.MaxItems+1 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"operator action query limit is invalid")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_id, kind, state, run_id,
		session_id, workspace_id, available_at, due_at FROM (
			SELECT id AS source_id, 'steering_pending' AS kind, status AS state,
				run_id, session_id, '' AS workspace_id, created_at AS available_at,
				NULL AS due_at
			FROM operator_steering_messages
			WHERE run_id = ? AND status = 'pending'
			UNION ALL
			SELECT id AS source_id, 'approval_pending' AS kind, status AS state,
				run_id, session_id, workspace_id, created_at AS available_at,
				NULL AS due_at
			FROM tool_approvals
			WHERE run_id = ? AND status = 'pending'
			UNION ALL
			SELECT id AS source_id,
				CASE status WHEN 'proposed' THEN 'file_edit_review'
					ELSE 'file_edit_apply' END AS kind,
				status AS state, ? AS run_id, session_id, workspace_id,
				updated_at AS available_at, NULL AS due_at
			FROM file_edits
			WHERE ? <> '' AND ? <> '' AND session_id = ? AND workspace_id = ?
				AND status IN ('proposed', 'approved')
			UNION ALL
			SELECT id AS source_id, 'wake_due' AS kind, status AS state,
				run_id, session_id, '' AS workspace_id, next_wake_at AS available_at,
				next_wake_at AS due_at
			FROM run_wake_intents
			WHERE run_id = ? AND status = 'queued'
				AND julianday(next_wake_at) <= julianday(?)
		) actions
		ORDER BY CASE kind
			WHEN 'wake_due' THEN 0 WHEN 'approval_pending' THEN 1
			WHEN 'file_edit_review' THEN 2 WHEN 'file_edit_apply' THEN 3 ELSE 4 END,
			available_at DESC, kind, source_id
		LIMIT ?`, runID, runID, runID, sessionID, workspaceID, sessionID, workspaceID,
		runID, ts(now.UTC()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]operatoraction.Record, 0, limit)
	for rows.Next() {
		var value operatoraction.Record
		var kind, available string
		var due sql.NullString
		if err := rows.Scan(&value.SourceID, &kind, &value.State, &value.RunID,
			&value.SessionID, &value.WorkspaceID, &available, &due); err != nil {
			return nil, err
		}
		value.Kind = operatoraction.Kind(kind)
		value.AvailableAt = parseTS(available)
		if due.Valid {
			dueAt := parseTS(due.String)
			value.DueAt = &dueAt
		}
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("stored operator action is invalid: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
