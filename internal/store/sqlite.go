package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/agent"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolrun"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	db *sql.DB
}

const maxStoreListOffset = 100000

func validateStoreListOffset(offset int) error {
	if offset < 0 || offset > maxStoreListOffset {
		return fmt.Errorf("list offset must be between 0 and %d", maxStoreListOffset)
	}
	return nil
}

type WorkspaceRecord struct {
	ID        string
	Name      string
	RootPath  string
	CreatedAt time.Time
}

type ArtifactRecord struct {
	ID          string
	WorkspaceID string
	TaskID      string
	Path        string
	Kind        string
	CreatedAt   time.Time
}

func Open(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := restrictSQLiteFilePermissions(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &SQLiteStore{db: db}
	if err := s.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func restrictSQLiteFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect sqlite database permissions: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("sqlite database path is not a regular file")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict sqlite database permissions: %w", err)
	}
	return nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_txlock=immediate"
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) Migrate(ctx context.Context) error {
	baseline := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			root_path TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			goal TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			workspace_id TEXT,
			type TEXT NOT NULL,
			message TEXT NOT NULL,
			payload_json TEXT,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			task_id TEXT,
			path TEXT NOT NULL,
			kind TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS provider_setting (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS context_summaries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			workspace_id TEXT,
			content TEXT NOT NULL,
			source_message_count INTEGER NOT NULL,
			preserved_message_count INTEGER NOT NULL,
			token_estimate INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_context_summaries_task_id_created_at
			ON context_summaries(task_id, created_at);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			workspace_id TEXT,
			title TEXT NOT NULL,
			route TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at
			ON sessions(updated_at);`,
		`CREATE TABLE IF NOT EXISTS session_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			token_estimate INTEGER NOT NULL,
			compacted INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_session_messages_session_id_id
			ON session_messages(session_id, id);`,
		`CREATE TABLE IF NOT EXISTS tool_runs (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			workspace_id TEXT,
			tool_name TEXT NOT NULL,
			command TEXT NOT NULL,
			status TEXT NOT NULL,
			risk TEXT,
			policy_reason TEXT,
			stdout TEXT,
			stderr TEXT,
			exit_code INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tool_runs_session_status_updated_at
			ON tool_runs(session_id, status, updated_at);`,
		`CREATE TABLE IF NOT EXISTS file_edits (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			workspace_id TEXT NOT NULL,
			path TEXT NOT NULL,
			status TEXT NOT NULL,
			original_text TEXT NOT NULL,
			proposed_text TEXT NOT NULL,
			diff_text TEXT NOT NULL,
			original_hash TEXT NOT NULL,
			proposed_hash TEXT NOT NULL,
			reason TEXT,
			secrets_redacted INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_file_edits_workspace_status_updated_at
			ON file_edits(workspace_id, status, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_file_edits_session_status_updated_at
			ON file_edits(session_id, status, updated_at);`,
	}
	return s.applyMigrations(ctx, []migration{
		{Version: 1, Name: "v0.1 baseline", Statements: baseline},
		{Version: 2, Name: "run-centric foundation", Statements: runCentricSchemaStatements},
		{Version: 3, Name: "run session projection", Statements: runSessionProjectionStatements},
		{Version: 4, Name: "legacy task run mapping", Statements: legacyTaskRunStatements},
		{Version: 5, Name: "supervisor checkpoints", Statements: supervisorCheckpointStatements},
		{Version: 6, Name: "supervisor budget ledger", Statements: supervisorBudgetStatements},
		{Version: 7, Name: "supervisor pending input", Statements: supervisorPendingInputStatements},
		{Version: 8, Name: "supervisor protocol repair", Statements: supervisorProtocolRepairStatements},
		{Version: 9, Name: "run work board", Statements: workBoardStatements},
		{Version: 10, Name: "run notes", Statements: runNotesStatements},
		{Version: 11, Name: "durable tool approvals", Statements: durableApprovalStatements},
		{Version: 12, Name: "session grants and tool budgets", Statements: sessionGrantAndToolBudgetStatements},
		{Version: 13, Name: "typed script process proposals", Statements: typedScriptProcessStatements},
		{Version: 14, Name: "run tool output artifacts", Statements: runArtifactStatements},
		{Version: 15, Name: "structured memory tool operations", Statements: structuredToolOperationStatements},
		{Version: 16, Name: "supervisor structured tool loop", Statements: supervisorToolLoopStatements},
		{Version: 17, Name: "run execution leases", Statements: runExecutionLeaseStatements},
		{Version: 18, Name: "cross-process model cancellation", Statements: modelCancellationStatements},
		{Version: 19, Name: "single-root agent coordinator", Statements: agentCoordinatorStatements},
		{Version: 20, Name: "idempotent agent inbox protocol", Statements: agentInboxProtocolStatements},
		{Version: 21, Name: "bounded specialist admission", Statements: specialistAdmissionStatements},
		{Version: 22, Name: "agent-owned work memory", Statements: agentMemoryOwnershipStatements},
		{Version: 23, Name: "specialist completion reports", Statements: agentCompletionReportStatements},
		{Version: 24, Name: "leased specialist attempts", Statements: specialistAttemptStatements},
		{Version: 25, Name: "root inbox context delivery", Statements: rootInboxContextStatements},
		{Version: 26, Name: "specialist model call ledger", Statements: specialistModelCallStatements},
		{Version: 27, Name: "specialist context delivery", Statements: specialistContextDeliveryStatements},
		{Version: 28, Name: "specialist protocol repair", Statements: specialistProtocolRepairStatements},
		{Version: 29, Name: "specialist schedule and cancellation control", Statements: specialistScheduleControlStatements},
		{Version: 30, Name: "review-gated specialist delegation proposals", Statements: specialistDelegationProposalStatements},
		{Version: 31, Name: "immutable specialist delegation reviews", Statements: specialistDelegationReviewStatements},
		{Version: 32, Name: "recoverable specialist delegation application", Statements: specialistDelegationApplicationStatements},
		{Version: 33, Name: "immutable read-only fan-out plans", Statements: readOnlyFanoutPlanStatements},
		{Version: 34, Name: "bounded read-only fan-out execution", Statements: readOnlyFanoutExecutionStatements},
		{Version: 35, Name: "deterministic finding report projection", Statements: findingReportStatements},
		{Version: 36, Name: "Artifact-backed finding validation", Statements: findingValidationStatements},
		{Version: 37, Name: "accepted and fixed finding remediation lifecycle", Statements: findingRemediationStatements},
		{Version: 38, Name: "operator-controlled Specialist scheduling", Statements: specialistOperatorScheduleStatements},
		{Version: 39, Name: "immutable Run Skill selection", Statements: skillSelectionStatements},
		{Version: 40, Name: "root Skill context provenance", Statements: rootSkillContextStatements},
		{Version: 41, Name: "immutable Run execution mode", Statements: runModeStatements},
		{Version: 42, Name: "review-gated Plan Delivery workflow", Statements: planDeliveryStatements},
		{Version: 43, Name: "immutable session context provenance", Statements: contextProvenanceStatements},
		{Version: 44, Name: "immutable Delivery checkpoint gates", Statements: deliveryCheckpointStatements},
		{Version: 45, Name: "durable operator steering queue", Statements: operatorSteeringStatements},
		{Version: 46, Name: "operator steering queue controls", Statements: operatorSteeringControlStatements},
		{Version: 47, Name: "minimal Specialist Skill context", Statements: specialistSkillContextStatements},
		{Version: 48, Name: "Go-owned Sandbox Manifest preparation", Statements: sandboxManifestStatements},
		{Version: 49, Name: "sandbox approval and disabled execution candidates", Statements: sandboxExecutionCandidateStatements},
		{Version: 50, Name: "disabled Sandbox lifecycle and Artifact bindings", Statements: sandboxLifecycleStatements},
		{Version: 51, Name: "disabled Sandbox backend and output preflight", Statements: sandboxPreflightStatements},
		{Version: 52, Name: "simulation-only Sandbox backend evidence and output transaction", Statements: sandboxBackendEvidenceStatements},
		{Version: 53, Name: "read-only Docker production observations", Statements: sandboxDockerObservationStatements},
		{Version: 54, Name: "deterministic Docker container plans and fake write transactions", Statements: sandboxDockerContainerPlanStatements},
		{Version: 55, Name: "bounded Docker create inspect remove rehearsals", Statements: sandboxDockerContainerRehearsalStatements},
		{Version: 56, Name: "recoverable Docker rehearsal attempts", Statements: sandboxDockerContainerAttemptStatements},
		{Version: 57, Name: "descriptor sealed Docker host input staging", Statements: sandboxDockerHostInputStagingStatements},
		{Version: 58, Name: "durable pre-stage Docker host input requirement", Statements: sandboxDockerHostInputRequirementStatements},
		{Version: 59, Name: "recoverable Docker daemon host input handoff", Statements: sandboxDockerHostInputHandoffStatements},
	})
}

func (s *SQLiteStore) SaveWorkspace(ctx context.Context, rec WorkspaceRecord) error {
	if rec.ID == "" {
		return errors.New("workspace id is required")
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO workspaces (id, name, root_path, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET root_path=excluded.root_path`,
		rec.ID, rec.Name, rec.RootPath, ts(rec.CreatedAt))
	return err
}

func (s *SQLiteStore) ListWorkspaces(ctx context.Context) ([]WorkspaceRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, root_path, created_at FROM workspaces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkspaceRecord
	for rows.Next() {
		rec, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetWorkspaceByName(ctx context.Context, name string) (WorkspaceRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, root_path, created_at FROM workspaces WHERE name = ?`, name)
	return scanWorkspace(row)
}

func (s *SQLiteStore) GetWorkspaceByID(ctx context.Context, id string) (WorkspaceRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, root_path, created_at FROM workspaces WHERE id = ?`, id)
	return scanWorkspace(row)
}

func (s *SQLiteStore) GetWorkspaceInfo(ctx context.Context, id string) (session.WorkspaceInfo, error) {
	rec, err := s.GetWorkspaceByID(ctx, id)
	if err != nil {
		return session.WorkspaceInfo{}, err
	}
	return session.WorkspaceInfo{
		ID:       rec.ID,
		Name:     rec.Name,
		RootPath: rec.RootPath,
	}, nil
}

func (s *SQLiteStore) SaveTask(ctx context.Context, task agent.Task) error {
	if task.ID == "" {
		return errors.New("task id is required")
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.Status == "" {
		task.Status = agent.StatusPending
	}
	task.Goal = redact.String(task.Goal)
	task.Mode = redact.String(task.Mode)
	_, err := s.db.ExecContext(ctx, `INSERT INTO tasks (id, kind, goal, workspace_id, mode, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status`,
		task.ID, string(task.Kind), task.Goal, task.WorkspaceID, task.Mode, task.Status, ts(task.CreatedAt))
	return err
}

func (s *SQLiteStore) GetTask(ctx context.Context, id string) (agent.Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, kind, goal, workspace_id, mode, status, created_at FROM tasks WHERE id = ?`, id)
	var task agent.Task
	var kind string
	var created string
	if err := row.Scan(&task.ID, &kind, &task.Goal, &task.WorkspaceID, &task.Mode, &task.Status, &created); err != nil {
		return agent.Task{}, err
	}
	task.Kind = agent.TaskKind(kind)
	task.CreatedAt = parseTS(created)
	return task, nil
}

func (s *SQLiteStore) UpdateTaskStatus(ctx context.Context, id string, status string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE tasks SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %q not found", id)
	}
	return nil
}

func (s *SQLiteStore) RecordEvent(ctx context.Context, event agent.Event) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.Message = redact.String(event.Message)
	redactedPayload, err := redactJSONPayload(event.PayloadJSON)
	if err != nil {
		return err
	}
	event.PayloadJSON = redactedPayload
	_, err = s.db.ExecContext(ctx, `INSERT INTO events (task_id, workspace_id, type, message, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		event.TaskID, event.WorkspaceID, event.Type, event.Message, event.PayloadJSON, ts(event.CreatedAt))
	return err
}

func (s *SQLiteStore) ListEventsByTask(ctx context.Context, taskID string) ([]agent.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, workspace_id, type, message, payload_json, created_at
		FROM events WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []agent.Event
	for rows.Next() {
		var event agent.Event
		var created string
		if err := rows.Scan(&event.ID, &event.TaskID, &event.WorkspaceID, &event.Type, &event.Message, &event.PayloadJSON, &created); err != nil {
			return nil, err
		}
		event.CreatedAt = parseTS(created)
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveArtifact(ctx context.Context, rec ArtifactRecord) error {
	if rec.ID == "" {
		return errors.New("artifact id is required")
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO artifacts (id, workspace_id, task_id, path, kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.WorkspaceID, rec.TaskID, rec.Path, rec.Kind, ts(rec.CreatedAt))
	return err
}

func (s *SQLiteStore) SetProviderSetting(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO provider_setting (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, ts(time.Now().UTC()))
	return err
}

func (s *SQLiteStore) GetProviderSetting(ctx context.Context, key string) (string, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM provider_setting WHERE key = ?`, key)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

func (s *SQLiteStore) SaveContextSummary(ctx context.Context, summary contextmgr.Summary) (contextmgr.Summary, error) {
	if strings.TrimSpace(summary.TaskID) == "" {
		return contextmgr.Summary{}, errors.New("task id is required")
	}
	summary.Content = redact.String(summary.Content)
	summary.TokenEstimate = contextmgr.EstimateTokens(summary.Content)
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO context_summaries
		(task_id, workspace_id, content, source_message_count, preserved_message_count, token_estimate, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		summary.TaskID, summary.WorkspaceID, summary.Content, summary.SourceMessageCount,
		summary.PreservedMessageCount, summary.TokenEstimate, ts(summary.CreatedAt))
	if err != nil {
		return contextmgr.Summary{}, err
	}
	id, err := res.LastInsertId()
	if err == nil {
		summary.ID = id
	}
	return summary, nil
}

func (s *SQLiteStore) LatestContextSummary(ctx context.Context, taskID string) (contextmgr.Summary, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, task_id, workspace_id, content, source_message_count,
		preserved_message_count, token_estimate, created_at
		FROM context_summaries WHERE task_id = ? ORDER BY id DESC LIMIT 1`, taskID)
	var summary contextmgr.Summary
	var created string
	if err := row.Scan(&summary.ID, &summary.TaskID, &summary.WorkspaceID, &summary.Content,
		&summary.SourceMessageCount, &summary.PreservedMessageCount, &summary.TokenEstimate, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contextmgr.Summary{}, false, nil
		}
		return contextmgr.Summary{}, false, err
	}
	summary.CreatedAt = parseTS(created)
	return summary, true, nil
}

func (s *SQLiteStore) SaveSession(ctx context.Context, sess session.Session) error {
	if sess.Status == "" {
		sess.Status = session.StatusActive
	}
	if sess.Route == "" {
		sess.Route = "learn"
	}
	if strings.TrimSpace(sess.Title) == "" {
		sess.Title = "New session"
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = time.Now().UTC()
	}
	if err := sess.Validate(); err != nil {
		return err
	}
	sess.Title = redact.String(sess.Title)
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id, workspace_id, title, route, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			workspace_id=excluded.workspace_id,
			title=excluded.title,
			route=excluded.route,
			status=excluded.status,
			updated_at=excluded.updated_at`,
		sess.ID, sess.WorkspaceID, sess.Title, sess.Route, sess.Status, ts(sess.CreatedAt), ts(sess.UpdatedAt))
	return err
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (session.Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, title, route, status, created_at, updated_at FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

func (s *SQLiteStore) ListSessions(ctx context.Context) ([]session.Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, title, route, status, created_at, updated_at FROM sessions ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []session.Session
	for rows.Next() {
		rec, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveSessionMessage(ctx context.Context, message session.Message) (session.Message, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return session.Message{}, err
	}
	defer func() { _ = tx.Rollback() }()
	message, err = saveSessionMessageTx(ctx, tx, message)
	if err != nil {
		return session.Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return session.Message{}, err
	}
	return message, nil
}

func saveSessionMessageTx(ctx context.Context, tx *sql.Tx, message session.Message) (session.Message, error) {
	var err error
	message, err = session.PrepareMessageForStorage(message)
	if err != nil {
		return session.Message{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO session_messages
		(session_id, role, content, provenance_version, source_kind, source_ref, content_sha256,
		instruction_authorized, token_estimate, compacted, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.SessionID, message.Role, message.Content, message.Provenance.Version,
		message.Provenance.SourceKind, message.Provenance.SourceRef, message.Provenance.ContentSHA256,
		boolInt(message.Provenance.InstructionAuthorized), message.TokenEstimate,
		boolInt(message.Compacted), ts(message.CreatedAt))
	if err != nil {
		return session.Message{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return session.Message{}, err
	}
	message.ID = id
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET updated_at = ? WHERE id = ?`, ts(time.Now().UTC()), message.SessionID)
	if err != nil {
		return session.Message{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return session.Message{}, err
	}
	if rows != 1 {
		return session.Message{}, errors.New("session was not found")
	}
	if err := projectSessionMessageTx(ctx, tx, message); err != nil {
		return session.Message{}, err
	}
	return message, nil
}

func (s *SQLiteStore) ListSessionMessages(ctx context.Context, sessionID string, includeCompacted bool) ([]session.Message, error) {
	query := `SELECT id, session_id, role, content, provenance_version, source_kind, source_ref,
		content_sha256, instruction_authorized, token_estimate, compacted, created_at
		FROM session_messages WHERE session_id = ?`
	if !includeCompacted {
		query += ` AND compacted = 0`
	}
	query += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []session.Message
	for rows.Next() {
		msg, err := scanSessionMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) MarkSessionMessagesCompacted(ctx context.Context, sessionID string, throughID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE session_messages SET compacted = 1 WHERE session_id = ? AND id <= ? AND compacted = 0`, sessionID, throughID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteStore) SaveToolRun(ctx context.Context, run toolrun.ToolRun) (toolrun.ToolRun, error) {
	if strings.TrimSpace(run.ID) == "" {
		return toolrun.ToolRun{}, errors.New("tool run id is required")
	}
	if strings.TrimSpace(run.ToolName) == "" {
		return toolrun.ToolRun{}, errors.New("tool name is required")
	}
	if strings.TrimSpace(run.Command) == "" {
		return toolrun.ToolRun{}, errors.New("command is required")
	}
	if strings.TrimSpace(run.Status) == "" {
		run.Status = toolrun.StatusProposed
	}
	if !utf8.ValidString(run.Stdout) || !utf8.ValidString(run.Stderr) ||
		len([]byte(run.Stdout)) > artifact.MaxContentBytes || len([]byte(run.Stderr)) > artifact.MaxContentBytes {
		return toolrun.ToolRun{}, fmt.Errorf("tool output must be valid UTF-8 and at most %d bytes per stream", artifact.MaxContentBytes)
	}
	run.Command = redact.String(run.Command)
	run.PolicyReason = redact.String(run.PolicyReason)
	run.Stdout = redact.String(run.Stdout)
	run.Stderr = redact.String(run.Stderr)
	if len([]byte(run.Stdout)) > artifact.MaxContentBytes || len([]byte(run.Stderr)) > artifact.MaxContentBytes {
		return toolrun.ToolRun{}, fmt.Errorf("redacted tool output exceeds %d bytes per stream", artifact.MaxContentBytes)
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return toolrun.ToolRun{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var previousStatus string
	lookupErr := tx.QueryRowContext(ctx, `SELECT status FROM tool_runs WHERE id = ?`, run.ID).Scan(&previousStatus)
	existed := lookupErr == nil
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return toolrun.ToolRun{}, lookupErr
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tool_runs
		(id, session_id, workspace_id, tool_name, command, status, risk, policy_reason, stdout, stderr, exit_code, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id=excluded.session_id,
			workspace_id=excluded.workspace_id,
			tool_name=excluded.tool_name,
			command=excluded.command,
			status=excluded.status,
			risk=excluded.risk,
			policy_reason=excluded.policy_reason,
			stdout=excluded.stdout,
			stderr=excluded.stderr,
			exit_code=excluded.exit_code,
			updated_at=excluded.updated_at`,
		run.ID, run.SessionID, run.WorkspaceID, run.ToolName, run.Command, run.Status, run.Risk,
		run.PolicyReason, run.Stdout, run.Stderr, run.ExitCode, ts(run.CreatedAt), ts(run.UpdatedAt))
	if err != nil {
		return toolrun.ToolRun{}, err
	}
	if err := projectToolRunTx(ctx, tx, run, previousStatus, existed); err != nil {
		return toolrun.ToolRun{}, err
	}
	if err := syncToolApprovalTx(ctx, tx, run, previousStatus, existed); err != nil {
		return toolrun.ToolRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return toolrun.ToolRun{}, err
	}
	return run, nil
}

func (s *SQLiteStore) GetToolRun(ctx context.Context, id string) (toolrun.ToolRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, session_id, workspace_id, tool_name, command, status, risk,
		policy_reason, stdout, stderr, exit_code, created_at, updated_at FROM tool_runs WHERE id = ?`, id)
	return scanToolRun(row)
}

func (s *SQLiteStore) ListToolRuns(ctx context.Context, filter toolrun.ListFilter) ([]toolrun.ToolRun, error) {
	query := `SELECT id, session_id, workspace_id, tool_name, command, status, risk,
		policy_reason, stdout, stderr, exit_code, created_at, updated_at FROM tool_runs WHERE 1=1`
	var args []any
	if strings.TrimSpace(filter.SessionID) != "" {
		query += ` AND session_id = ?`
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	query += ` ORDER BY updated_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []toolrun.ToolRun
	for rows.Next() {
		run, err := scanToolRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveFileEdit(ctx context.Context, edit fileedit.Edit) (fileedit.Edit, error) {
	if strings.TrimSpace(edit.ID) == "" {
		return fileedit.Edit{}, errors.New("file edit id is required")
	}
	if strings.TrimSpace(edit.WorkspaceID) == "" {
		return fileedit.Edit{}, errors.New("workspace id is required")
	}
	if strings.TrimSpace(edit.Path) == "" {
		return fileedit.Edit{}, errors.New("file edit path is required")
	}
	if strings.TrimSpace(edit.Status) == "" {
		edit.Status = fileedit.StatusProposed
	}
	redactedOriginal := redact.String(edit.OriginalText)
	redactedProposed := redact.String(edit.ProposedText)
	if redactedOriginal != edit.OriginalText || redactedProposed != edit.ProposedText {
		edit.SecretsRedacted = true
	}
	edit.OriginalText = redactedOriginal
	if redactedProposed != edit.ProposedText {
		edit.ProposedHash = fileedit.HashText(redactedProposed)
	}
	edit.ProposedText = redactedProposed
	edit.Diff = redact.String(edit.Diff)
	edit.Reason = redact.String(edit.Reason)
	if edit.CreatedAt.IsZero() {
		edit.CreatedAt = time.Now().UTC()
	}
	if edit.UpdatedAt.IsZero() {
		edit.UpdatedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fileedit.Edit{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var previousStatus string
	lookupErr := tx.QueryRowContext(ctx, `SELECT status FROM file_edits WHERE id = ?`, edit.ID).Scan(&previousStatus)
	existed := lookupErr == nil
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fileedit.Edit{}, lookupErr
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO file_edits
		(id, session_id, workspace_id, path, status, original_text, proposed_text, diff_text,
		 original_hash, proposed_hash, reason, secrets_redacted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id=excluded.session_id,
			workspace_id=excluded.workspace_id,
			path=excluded.path,
			status=excluded.status,
			original_text=excluded.original_text,
			proposed_text=excluded.proposed_text,
			diff_text=excluded.diff_text,
			original_hash=excluded.original_hash,
			proposed_hash=excluded.proposed_hash,
			reason=excluded.reason,
			secrets_redacted=excluded.secrets_redacted,
			updated_at=excluded.updated_at`,
		edit.ID, edit.SessionID, edit.WorkspaceID, edit.Path, edit.Status, edit.OriginalText,
		edit.ProposedText, edit.Diff, edit.OriginalHash, edit.ProposedHash, edit.Reason,
		boolInt(edit.SecretsRedacted), ts(edit.CreatedAt), ts(edit.UpdatedAt))
	if err != nil {
		return fileedit.Edit{}, err
	}
	if err := projectFileEditTx(ctx, tx, edit, previousStatus, existed); err != nil {
		return fileedit.Edit{}, err
	}
	if err := syncFileEditApprovalTx(ctx, tx, edit, previousStatus, existed); err != nil {
		return fileedit.Edit{}, err
	}
	if err := tx.Commit(); err != nil {
		return fileedit.Edit{}, err
	}
	return edit, nil
}

func (s *SQLiteStore) GetFileEdit(ctx context.Context, id string) (fileedit.Edit, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, session_id, workspace_id, path, status, original_text,
		proposed_text, diff_text, original_hash, proposed_hash, reason, secrets_redacted, created_at, updated_at
		FROM file_edits WHERE id = ?`, id)
	return scanFileEdit(row)
}

func (s *SQLiteStore) ListFileEdits(ctx context.Context, filter fileedit.ListFilter) ([]fileedit.Edit, error) {
	query := `SELECT id, session_id, workspace_id, path, status, original_text, proposed_text, diff_text,
		original_hash, proposed_hash, reason, secrets_redacted, created_at, updated_at FROM file_edits WHERE 1=1`
	var args []any
	if strings.TrimSpace(filter.SessionID) != "" {
		query += ` AND session_id = ?`
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	query += ` ORDER BY updated_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var edits []fileedit.Edit
	for rows.Next() {
		edit, err := scanFileEdit(rows)
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit)
	}
	return edits, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWorkspace(row scanner) (WorkspaceRecord, error) {
	var rec WorkspaceRecord
	var created string
	if err := row.Scan(&rec.ID, &rec.Name, &rec.RootPath, &created); err != nil {
		return WorkspaceRecord{}, err
	}
	rec.CreatedAt = parseTS(created)
	return rec, nil
}

func scanSession(row scanner) (session.Session, error) {
	var sess session.Session
	var created string
	var updated string
	if err := row.Scan(&sess.ID, &sess.WorkspaceID, &sess.Title, &sess.Route, &sess.Status, &created, &updated); err != nil {
		return session.Session{}, err
	}
	sess.CreatedAt = parseTS(created)
	sess.UpdatedAt = parseTS(updated)
	return sess, nil
}

func scanSessionMessage(row scanner) (session.Message, error) {
	var msg session.Message
	var compacted int
	var instructionAuthorized int
	var created string
	if err := row.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content,
		&msg.Provenance.Version, &msg.Provenance.SourceKind, &msg.Provenance.SourceRef,
		&msg.Provenance.ContentSHA256, &instructionAuthorized, &msg.TokenEstimate,
		&compacted, &created); err != nil {
		return session.Message{}, err
	}
	msg.Provenance.InstructionAuthorized = instructionAuthorized != 0
	msg.Compacted = compacted != 0
	msg.CreatedAt = parseTS(created)
	if err := session.ValidateStoredMessage(msg); err != nil {
		return session.Message{}, fmt.Errorf("validate stored session message %d: %w", msg.ID, err)
	}
	return msg, nil
}

func scanToolRun(row scanner) (toolrun.ToolRun, error) {
	var run toolrun.ToolRun
	var created string
	var updated string
	if err := row.Scan(&run.ID, &run.SessionID, &run.WorkspaceID, &run.ToolName, &run.Command, &run.Status,
		&run.Risk, &run.PolicyReason, &run.Stdout, &run.Stderr, &run.ExitCode, &created, &updated); err != nil {
		return toolrun.ToolRun{}, err
	}
	run.CreatedAt = parseTS(created)
	run.UpdatedAt = parseTS(updated)
	return run, nil
}

func scanFileEdit(row scanner) (fileedit.Edit, error) {
	var edit fileedit.Edit
	var secretsRedacted int
	var created string
	var updated string
	if err := row.Scan(&edit.ID, &edit.SessionID, &edit.WorkspaceID, &edit.Path, &edit.Status,
		&edit.OriginalText, &edit.ProposedText, &edit.Diff, &edit.OriginalHash, &edit.ProposedHash,
		&edit.Reason, &secretsRedacted, &created, &updated); err != nil {
		return fileedit.Edit{}, err
	}
	edit.SecretsRedacted = secretsRedacted != 0
	edit.CreatedAt = parseTS(created)
	edit.UpdatedAt = parseTS(updated)
	return edit, nil
}

func scanFileEditPreview(row scanner) (fileedit.Preview, error) {
	var preview fileedit.Preview
	var secretsRedacted int
	var created string
	var updated string
	if err := row.Scan(&preview.ID, &preview.SessionID, &preview.WorkspaceID, &preview.Path,
		&preview.Status, &preview.Diff, &preview.OriginalHash, &preview.ProposedHash,
		&preview.Reason, &secretsRedacted, &created, &updated); err != nil {
		return fileedit.Preview{}, err
	}
	preview.SecretsRedacted = secretsRedacted != 0
	preview.CreatedAt = parseTS(created)
	preview.UpdatedAt = parseTS(updated)
	return preview, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func ts(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTS(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
