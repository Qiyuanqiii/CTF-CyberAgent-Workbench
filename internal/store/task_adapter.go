package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/agent"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
)

func (s *SQLiteStore) FindTaskRunLink(ctx context.Context, taskID string) (agent.TaskRunLink, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT task_id, mission_id, run_id, created_at
		FROM legacy_task_runs WHERE task_id = ?`, strings.TrimSpace(taskID))
	link, err := scanTaskRunLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agent.TaskRunLink{}, false, nil
	}
	if err != nil {
		return agent.TaskRunLink{}, false, err
	}
	return link, true, nil
}

func (s *SQLiteStore) CreateTaskMissionRun(ctx context.Context, source agent.Task,
	mission domain.Mission, run domain.Run, mode domain.RunModeSnapshot,
	linkedSession session.Session, initialEvents []events.Event,
) (agent.TaskRunLink, bool, error) {
	if strings.TrimSpace(source.ID) == "" {
		return agent.TaskRunLink{}, false, errors.New("source task id is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return agent.TaskRunLink{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	if link, ok, err := findTaskRunLinkTx(ctx, tx, source.ID); err != nil {
		return agent.TaskRunLink{}, false, err
	} else if ok {
		return link, false, nil
	}
	var storedID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE id = ?`, source.ID).Scan(&storedID); err != nil {
		return agent.TaskRunLink{}, false, err
	}
	if err := createMissionRunTx(ctx, tx, mission, run, mode, linkedSession, true, initialEvents); err != nil {
		_ = tx.Rollback()
		if existing, ok, findErr := s.FindTaskRunLink(ctx, source.ID); findErr == nil && ok {
			return existing, false, nil
		}
		return agent.TaskRunLink{}, false, err
	}
	link := agent.TaskRunLink{
		TaskID:    source.ID,
		MissionID: mission.ID,
		RunID:     run.ID,
		CreatedAt: time.Now().UTC(),
	}
	if err := link.Validate(); err != nil {
		return agent.TaskRunLink{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO legacy_task_runs (task_id, mission_id, run_id, created_at)
		VALUES (?, ?, ?, ?)`, link.TaskID, link.MissionID, link.RunID, ts(link.CreatedAt)); err != nil {
		_ = tx.Rollback()
		if existing, ok, findErr := s.FindTaskRunLink(ctx, source.ID); findErr == nil && ok {
			return existing, false, nil
		}
		return agent.TaskRunLink{}, false, err
	}
	if err := tx.Commit(); err != nil {
		if existing, ok, findErr := s.FindTaskRunLink(ctx, source.ID); findErr == nil && ok {
			return existing, false, nil
		}
		return agent.TaskRunLink{}, false, err
	}
	return link, true, nil
}

func findTaskRunLinkTx(ctx context.Context, tx *sql.Tx, taskID string) (agent.TaskRunLink, bool, error) {
	link, err := scanTaskRunLink(tx.QueryRowContext(ctx, `SELECT task_id, mission_id, run_id, created_at
		FROM legacy_task_runs WHERE task_id = ?`, strings.TrimSpace(taskID)))
	if errors.Is(err, sql.ErrNoRows) {
		return agent.TaskRunLink{}, false, nil
	}
	if err != nil {
		return agent.TaskRunLink{}, false, err
	}
	return link, true, nil
}

func scanTaskRunLink(row scanner) (agent.TaskRunLink, error) {
	var link agent.TaskRunLink
	var created string
	if err := row.Scan(&link.TaskID, &link.MissionID, &link.RunID, &created); err != nil {
		return agent.TaskRunLink{}, err
	}
	link.CreatedAt = parseTS(created)
	return link, link.Validate()
}
