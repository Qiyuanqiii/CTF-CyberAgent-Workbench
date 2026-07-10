package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
)

func (s *SQLiteStore) CreateMissionRun(ctx context.Context, mission domain.Mission, run domain.Run, linkedSession session.Session, createSession bool, initialEvents []events.Event) error {
	mission.Goal = redact.String(mission.Goal)
	linkedSession.Title = redact.String(linkedSession.Title)
	if err := mission.Validate(); err != nil {
		return err
	}
	if err := run.Validate(); err != nil {
		return err
	}
	if err := linkedSession.Validate(); err != nil {
		return err
	}
	if run.Status != domain.RunCreated {
		return errors.New("new run must start in created status")
	}
	if run.MissionID != mission.ID || run.SessionID != linkedSession.ID {
		return errors.New("mission, run, and session identities do not match")
	}
	if mission.WorkspaceID != linkedSession.WorkspaceID {
		return errors.New("mission and session workspaces do not match")
	}
	if len(initialEvents) == 0 {
		return errors.New("initial run events are required")
	}
	for _, event := range initialEvents {
		if event.RunID != run.ID || event.MissionID != mission.ID {
			return errors.New("mission, run, and event identities do not match")
		}
		if err := event.Validate(); err != nil {
			return err
		}
	}
	scopeJSON, err := marshalRedactedJSON(mission.Scope)
	if err != nil {
		return err
	}
	configJSON, err := marshalRedactedJSON(run.Config)
	if err != nil {
		return err
	}
	budgetJSON, err := marshalRedactedJSON(run.Budget)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if createSession {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sessions
			(id, workspace_id, title, route, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, linkedSession.ID, linkedSession.WorkspaceID, linkedSession.Title,
			linkedSession.Route, linkedSession.Status, ts(linkedSession.CreatedAt), ts(linkedSession.UpdatedAt)); err != nil {
			return err
		}
	} else {
		result, err := tx.ExecContext(ctx, `UPDATE sessions SET workspace_id = ?, route = ?, updated_at = ?
			WHERE id = ? AND status = ?`, linkedSession.WorkspaceID, linkedSession.Route,
			ts(linkedSession.UpdatedAt), linkedSession.ID, session.StatusActive)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return errors.New("active run session was not found")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO missions
		(id, goal, profile, workspace_id, scope_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, mission.ID, mission.Goal, mission.Profile, mission.WorkspaceID,
		scopeJSON, ts(mission.CreatedAt), ts(mission.UpdatedAt)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runs
		(id, mission_id, session_id, status, config_json, budget_json, started_at, finished_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.ID, run.MissionID, run.SessionID, run.Status,
		configJSON, budgetJSON, nullableTS(run.StartedAt), nullableTS(run.FinishedAt), ts(run.CreatedAt), ts(run.UpdatedAt)); err != nil {
		return err
	}
	for _, event := range initialEvents {
		if _, err := insertRunEventTx(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetMission(ctx context.Context, id string) (domain.Mission, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, goal, profile, workspace_id, scope_json, created_at, updated_at
		FROM missions WHERE id = ?`, id)
	return scanMission(row)
}

func (s *SQLiteStore) GetRun(ctx context.Context, id string) (domain.Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, id)
	return scanRun(row)
}

func (s *SQLiteStore) ListRuns(ctx context.Context, filter domain.RunFilter) ([]domain.Run, error) {
	query := `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE 1=1`
	var args []any
	if strings.TrimSpace(filter.MissionID) != "" {
		query += ` AND mission_id = ?`
		args = append(args, strings.TrimSpace(filter.MissionID))
	}
	if filter.Status != "" {
		if !domain.ValidRunStatus(filter.Status) {
			return nil, fmt.Errorf("invalid run status %q", filter.Status)
		}
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *SQLiteStore) TransitionRun(ctx context.Context, run domain.Run, expected domain.RunStatus, event events.Event) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if run.Status == expected {
		return errors.New("run transition must change status")
	}
	before := run
	before.Status = expected
	if !before.CanTransition(run.Status) {
		return fmt.Errorf("run cannot transition from %s to %s", expected, run.Status)
	}
	if event.RunID != run.ID || event.MissionID != run.MissionID {
		return errors.New("run and event identities do not match")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	configJSON, err := marshalRedactedJSON(run.Config)
	if err != nil {
		return err
	}
	budgetJSON, err := marshalRedactedJSON(run.Budget)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, config_json = ?, budget_json = ?,
		started_at = ?, finished_at = ?, updated_at = ? WHERE id = ? AND status = ?`,
		run.Status, configJSON, budgetJSON, nullableTS(run.StartedAt), nullableTS(run.FinishedAt),
		ts(run.UpdatedAt), run.ID, expected)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("run %s status changed concurrently or was not found", run.ID)
	}
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListRunEvents(ctx context.Context, runID string) ([]events.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, version, run_id, mission_id, sequence,
		type, source, subject_id, payload_json, created_at FROM run_events WHERE run_id = ? ORDER BY sequence`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []events.Event
	for rows.Next() {
		event, err := scanRunEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func insertRunEventTx(ctx context.Context, tx *sql.Tx, event events.Event) (events.Event, error) {
	if event.Sequence != 0 {
		return events.Event{}, errors.New("run event sequence is assigned by the store")
	}
	event.PayloadJSON = redact.String(event.PayloadJSON)
	if err := event.Validate(); err != nil {
		return events.Event{}, err
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM run_events WHERE run_id = ?`, event.RunID).Scan(&event.Sequence); err != nil {
		return events.Event{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO run_events
		(event_id, version, run_id, mission_id, sequence, type, source, subject_id, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.EventID, event.Version, event.RunID, event.MissionID,
		event.Sequence, event.Type, event.Source, event.SubjectID, event.PayloadJSON, ts(event.CreatedAt))
	if err != nil {
		return events.Event{}, err
	}
	if id, err := result.LastInsertId(); err == nil {
		event.ID = id
	}
	return event, nil
}

func scanMission(row scanner) (domain.Mission, error) {
	var mission domain.Mission
	var profile string
	var scopeJSON string
	var created string
	var updated string
	if err := row.Scan(&mission.ID, &mission.Goal, &profile, &mission.WorkspaceID, &scopeJSON, &created, &updated); err != nil {
		return domain.Mission{}, err
	}
	mission.Profile = domain.Profile(profile)
	if err := json.Unmarshal([]byte(scopeJSON), &mission.Scope); err != nil {
		return domain.Mission{}, fmt.Errorf("decode mission scope: %w", err)
	}
	mission.CreatedAt = parseTS(created)
	mission.UpdatedAt = parseTS(updated)
	return mission, mission.Validate()
}

func scanRun(row scanner) (domain.Run, error) {
	var run domain.Run
	var status string
	var configJSON string
	var budgetJSON string
	var started sql.NullString
	var finished sql.NullString
	var created string
	var updated string
	if err := row.Scan(&run.ID, &run.MissionID, &run.SessionID, &status, &configJSON, &budgetJSON,
		&started, &finished, &created, &updated); err != nil {
		return domain.Run{}, err
	}
	run.Status = domain.RunStatus(status)
	if err := json.Unmarshal([]byte(configJSON), &run.Config); err != nil {
		return domain.Run{}, fmt.Errorf("decode run config: %w", err)
	}
	if err := json.Unmarshal([]byte(budgetJSON), &run.Budget); err != nil {
		return domain.Run{}, fmt.Errorf("decode run budget: %w", err)
	}
	run.StartedAt = parseNullableTS(started)
	run.FinishedAt = parseNullableTS(finished)
	run.CreatedAt = parseTS(created)
	run.UpdatedAt = parseTS(updated)
	return run, run.Validate()
}

func scanRunEvent(row scanner) (events.Event, error) {
	var event events.Event
	var created string
	if err := row.Scan(&event.ID, &event.EventID, &event.Version, &event.RunID, &event.MissionID,
		&event.Sequence, &event.Type, &event.Source, &event.SubjectID, &event.PayloadJSON, &created); err != nil {
		return events.Event{}, err
	}
	event.CreatedAt = parseTS(created)
	return event, event.Validate()
}

func marshalRedactedJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	safe := redact.String(string(data))
	if !json.Valid([]byte(safe)) {
		return "", errors.New("redaction produced invalid JSON")
	}
	return safe, nil
}

func nullableTS(value *time.Time) any {
	if value == nil {
		return nil
	}
	return ts(*value)
}

func parseNullableTS(value sql.NullString) *time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed := parseTS(value.String)
	return &parsed
}
