package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
)

const maxStoreReadPageLimit = 101

func validateStoreReadPage(offset int, limit int) error {
	if err := validateStoreListOffset(offset); err != nil {
		return err
	}
	if limit <= 0 || limit > maxStoreReadPageLimit {
		return fmt.Errorf("read page limit must be between 1 and %d", maxStoreReadPageLimit)
	}
	return nil
}

func (s *SQLiteStore) ListSessionsPage(ctx context.Context, offset int, limit int) ([]session.Session, error) {
	if err := validateStoreReadPage(offset, limit); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, title, route, status, created_at, updated_at
		FROM sessions ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]session.Session, 0, limit)
	for rows.Next() {
		record, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListSessionMessagesPage(ctx context.Context, sessionID string,
	includeCompacted bool, offset int, limit int,
) ([]session.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if err := validateStoreReadPage(offset, limit); err != nil {
		return nil, err
	}
	query := `SELECT id, session_id, role, content, token_estimate, compacted, created_at
		FROM session_messages WHERE session_id = ?`
	if !includeCompacted {
		query += ` AND compacted = 0`
	}
	query += ` ORDER BY id LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]session.Message, 0, limit)
	for rows.Next() {
		message, err := scanSessionMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListRecentSessionMessages(ctx context.Context, sessionID string,
	includeCompacted bool, limit int,
) ([]session.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if err := validateStoreReadPage(0, limit); err != nil {
		return nil, err
	}
	query := `SELECT id, session_id, role, content, token_estimate, compacted, created_at
		FROM session_messages WHERE session_id = ?`
	if !includeCompacted {
		query += ` AND compacted = 0`
	}
	query += ` ORDER BY id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]session.Message, 0, limit)
	for rows.Next() {
		message, err := scanSessionMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out, nil
}

func (s *SQLiteStore) ListRunEventsPage(ctx context.Context, runID string,
	offset int, limit int,
) ([]events.Event, error) {
	var err error
	if runID, err = validateReadRunID(runID); err != nil {
		return nil, err
	}
	if err := validateStoreReadPage(offset, limit); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, version, run_id, mission_id, sequence,
		type, source, subject_id, payload_json, created_at FROM run_events
		WHERE run_id = ? ORDER BY sequence LIMIT ? OFFSET ?`, runID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]events.Event, 0, limit)
	for rows.Next() {
		event, err := scanRunEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListRunEventsAfterSequence(ctx context.Context, runID string,
	afterSequence int64, limit int,
) ([]events.Event, error) {
	var err error
	if runID, err = validateReadRunID(runID); err != nil {
		return nil, err
	}
	if afterSequence < 0 {
		return nil, errors.New("event sequence cursor cannot be negative")
	}
	if err := validateStoreReadPage(0, limit); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, version, run_id, mission_id, sequence,
		type, source, subject_id, payload_json, created_at FROM run_events
		WHERE run_id = ? AND sequence > ? ORDER BY sequence LIMIT ?`, runID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]events.Event, 0, limit)
	for rows.Next() {
		event, err := scanRunEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func validateReadRunID(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || !utf8.ValidString(runID) || len([]rune(runID)) > 256 {
		return "", errors.New("run id is required and bounded")
	}
	return runID, nil
}

func (s *SQLiteStore) GetRunArtifactDescriptor(ctx context.Context, id string) (artifact.Descriptor, error) {
	id = strings.TrimSpace(id)
	if id == "" || !utf8.ValidString(id) || len([]rune(id)) > artifact.MaxIdentityRunes {
		return artifact.Descriptor{}, errors.New("artifact id is required and bounded")
	}
	return scanRunArtifactDescriptor(s.db.QueryRowContext(ctx, runArtifactDescriptorSelect+` WHERE id = ?`, id))
}
