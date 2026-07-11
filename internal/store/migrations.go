package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const LatestSchemaVersion = 17

type migration struct {
	Version    int
	Name       string
	Statements []string
}

type appliedMigration struct {
	Name     string
	Checksum string
}

var runCentricSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS missions (
		id TEXT PRIMARY KEY,
		goal TEXT NOT NULL,
		profile TEXT NOT NULL,
		workspace_id TEXT,
		scope_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_missions_updated_at
		ON missions(updated_at);`,
	`CREATE TABLE IF NOT EXISTS runs (
		id TEXT PRIMARY KEY,
		mission_id TEXT NOT NULL,
		session_id TEXT,
		status TEXT NOT NULL,
		config_json TEXT NOT NULL,
		budget_json TEXT NOT NULL,
		started_at TEXT,
		finished_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(mission_id) REFERENCES missions(id)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_runs_mission_created_at
		ON runs(mission_id, created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runs_status_updated_at
		ON runs(status, updated_at);`,
	`CREATE TABLE IF NOT EXISTS run_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		version TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		subject_id TEXT,
		payload_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id),
		FOREIGN KEY(mission_id) REFERENCES missions(id),
		UNIQUE(run_id, sequence)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_run_events_run_sequence
		ON run_events(run_id, sequence);`,
}

var runSessionProjectionStatements = []string{
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_session_id_unique
		ON runs(session_id)
		WHERE session_id IS NOT NULL AND session_id <> '';`,
	`CREATE TRIGGER IF NOT EXISTS trg_runs_session_insert
		BEFORE INSERT ON runs
		WHEN NEW.session_id IS NOT NULL AND NEW.session_id <> ''
			AND NOT EXISTS (SELECT 1 FROM sessions WHERE id = NEW.session_id)
		BEGIN
			SELECT RAISE(ABORT, 'run session does not exist');
		END;`,
	`CREATE TRIGGER IF NOT EXISTS trg_runs_session_update
		BEFORE UPDATE OF session_id ON runs
		WHEN NEW.session_id IS NOT NULL AND NEW.session_id <> ''
			AND NOT EXISTS (SELECT 1 FROM sessions WHERE id = NEW.session_id)
		BEGIN
			SELECT RAISE(ABORT, 'run session does not exist');
		END;`,
}

var legacyTaskRunStatements = []string{
	`CREATE TABLE IF NOT EXISTS legacy_task_runs (
		task_id TEXT PRIMARY KEY,
		mission_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		FOREIGN KEY(task_id) REFERENCES tasks(id),
		FOREIGN KEY(mission_id) REFERENCES missions(id),
		FOREIGN KEY(run_id) REFERENCES runs(id)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_legacy_task_runs_run_id
		ON legacy_task_runs(run_id);`,
}

var supervisorCheckpointStatements = []string{
	`CREATE TABLE IF NOT EXISTS run_supervisor_checkpoints (
		run_id TEXT PRIMARY KEY,
		next_turn INTEGER NOT NULL,
		phase TEXT NOT NULL,
		attempt_id TEXT,
		last_error TEXT,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id),
		CHECK(next_turn > 0)
	);`,
}

var supervisorBudgetStatements = []string{
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN execution_millis INTEGER NOT NULL DEFAULT 0;`,
}

var supervisorPendingInputStatements = []string{
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN pending_input TEXT NOT NULL DEFAULT '';`,
}

var supervisorProtocolRepairStatements = []string{
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN repair_phase TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN repair_reason TEXT NOT NULL DEFAULT '';`,
}

var workBoardStatements = []string{
	`CREATE TABLE work_items (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		priority TEXT NOT NULL,
		owner TEXT NOT NULL DEFAULT '',
		acceptance_json TEXT NOT NULL DEFAULT '[]',
		blocked_reason TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		completed_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		UNIQUE(run_id, id),
		CHECK(status IN ('pending', 'in_progress', 'blocked', 'completed', 'cancelled')),
		CHECK(priority IN ('low', 'normal', 'high', 'critical')),
		CHECK(version > 0),
		CHECK((status = 'blocked' AND length(trim(blocked_reason)) > 0) OR (status <> 'blocked' AND blocked_reason = '')),
		CHECK((status = 'completed' AND completed_at IS NOT NULL) OR (status <> 'completed' AND completed_at IS NULL))
	);`,
	`CREATE INDEX idx_work_items_run_status_priority
		ON work_items(run_id, status, priority, updated_at);`,
	`CREATE TABLE work_item_dependencies (
		run_id TEXT NOT NULL,
		work_item_id TEXT NOT NULL,
		depends_on_id TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(run_id, work_item_id, depends_on_id),
		FOREIGN KEY(run_id, work_item_id) REFERENCES work_items(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, depends_on_id) REFERENCES work_items(run_id, id) ON DELETE RESTRICT,
		CHECK(work_item_id <> depends_on_id)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_work_item_dependencies_target
		ON work_item_dependencies(run_id, depends_on_id, work_item_id);`,
}

var runNotesStatements = []string{
	`CREATE TABLE notes (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		category TEXT NOT NULL,
		visibility TEXT NOT NULL,
		owner TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		pinned INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		archived_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		UNIQUE(run_id, id),
		CHECK(category IN ('observation', 'hypothesis', 'decision', 'summary', 'reference')),
		CHECK(visibility IN ('run', 'root', 'owner')),
		CHECK(status IN ('active', 'archived')),
		CHECK(pinned IN (0, 1)),
		CHECK(version > 0),
		CHECK((visibility = 'owner' AND length(trim(owner)) > 0) OR (visibility <> 'owner' AND owner = '')),
		CHECK((status = 'archived' AND archived_at IS NOT NULL) OR (status = 'active' AND archived_at IS NULL))
	);`,
	`CREATE INDEX idx_notes_run_status_pinned
		ON notes(run_id, status, pinned, updated_at);`,
	`CREATE INDEX idx_notes_run_category_visibility
		ON notes(run_id, category, visibility, updated_at);`,
	`CREATE TABLE note_tags (
		run_id TEXT NOT NULL,
		note_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(run_id, note_id, tag),
		FOREIGN KEY(run_id, note_id) REFERENCES notes(run_id, id) ON DELETE CASCADE
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_note_tags_lookup ON note_tags(run_id, tag, note_id);`,
	`CREATE TABLE note_sources (
		run_id TEXT NOT NULL,
		note_id TEXT NOT NULL,
		source_ref TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(run_id, note_id, source_ref),
		FOREIGN KEY(run_id, note_id) REFERENCES notes(run_id, id) ON DELETE CASCADE
	) WITHOUT ROWID;`,
	`CREATE TABLE note_evidence (
		run_id TEXT NOT NULL,
		note_id TEXT NOT NULL,
		evidence_id TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(run_id, note_id, evidence_id),
		FOREIGN KEY(run_id, note_id) REFERENCES notes(run_id, id) ON DELETE CASCADE
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_note_evidence_lookup ON note_evidence(run_id, evidence_id, note_id);`,
}

var durableApprovalStatements = []string{
	`CREATE TABLE tool_approvals (
		id TEXT PRIMARY KEY,
		idempotency_key TEXT NOT NULL UNIQUE,
		proposal_id TEXT NOT NULL UNIQUE,
		run_id TEXT,
		session_id TEXT NOT NULL DEFAULT '',
		workspace_id TEXT NOT NULL DEFAULT '',
		tool_name TEXT NOT NULL,
		action_class TEXT NOT NULL,
		mode TEXT NOT NULL,
		status TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		decision_reason TEXT NOT NULL DEFAULT '',
		requested_by TEXT NOT NULL DEFAULT '',
		reviewed_by TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		decided_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		CHECK(mode IN ('automatic', 'per_call', 'session', 'never')),
		CHECK(status IN ('pending', 'approved', 'denied')),
		CHECK(version > 0),
		CHECK(length(request_fingerprint) = 64),
		CHECK((status = 'pending' AND decided_at IS NULL AND reviewed_by = '') OR
			(status <> 'pending' AND decided_at IS NOT NULL AND length(trim(reviewed_by)) > 0))
	);`,
	`CREATE INDEX idx_tool_approvals_run_status_updated_at
		ON tool_approvals(run_id, status, updated_at);`,
	`CREATE INDEX idx_tool_approvals_session_status_updated_at
		ON tool_approvals(session_id, status, updated_at);`,
	`CREATE TABLE approval_operations (
		idempotency_key TEXT PRIMARY KEY,
		approval_id TEXT NOT NULL,
		action TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		result_status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(approval_id) REFERENCES tool_approvals(id) ON DELETE CASCADE,
		CHECK(action IN ('approve', 'deny')),
		CHECK(result_status IN ('approved', 'denied')),
		CHECK(length(idempotency_key) = 64),
		CHECK(length(request_fingerprint) = 64)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_approval_operations_approval_created_at
		ON approval_operations(approval_id, created_at);`,
}

var sessionGrantAndToolBudgetStatements = []string{
	`CREATE TABLE approval_session_grants (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT '',
		tool_name TEXT NOT NULL,
		action_class TEXT NOT NULL,
		status TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		revocation_reason TEXT NOT NULL DEFAULT '',
		granted_by TEXT NOT NULL,
		revoked_by TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		revoked_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		CHECK(status IN ('active', 'revoked')),
		CHECK(version > 0),
		CHECK(length(request_fingerprint) = 64),
		CHECK((status = 'active' AND revoked_at IS NULL AND revoked_by = '' AND revocation_reason = '') OR
			(status = 'revoked' AND revoked_at IS NOT NULL AND length(trim(revoked_by)) > 0))
	);`,
	`CREATE UNIQUE INDEX idx_approval_session_grants_active_scope
		ON approval_session_grants(session_id, workspace_id, tool_name, action_class)
		WHERE status = 'active';`,
	`CREATE INDEX idx_approval_session_grants_run_status_updated_at
		ON approval_session_grants(run_id, status, updated_at);`,
	`CREATE TABLE approval_grant_operations (
		operation_key TEXT PRIMARY KEY,
		grant_id TEXT NOT NULL,
		action TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		result_status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(grant_id) REFERENCES approval_session_grants(id) ON DELETE CASCADE,
		CHECK(action IN ('grant', 'revoke')),
		CHECK(result_status IN ('active', 'revoked')),
		CHECK(length(operation_key) = 64),
		CHECK(length(request_fingerprint) = 64)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_approval_grant_operations_grant_created_at
		ON approval_grant_operations(grant_id, created_at);`,
	`ALTER TABLE tool_approvals ADD COLUMN grant_id TEXT REFERENCES approval_session_grants(id);`,
	`CREATE INDEX idx_tool_approvals_grant_id ON tool_approvals(grant_id);`,
	`CREATE TABLE run_tool_usage (
		run_id TEXT PRIMARY KEY,
		consumed INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL,
		exhausted_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		CHECK(consumed >= 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE run_tool_calls (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		workspace_id TEXT NOT NULL DEFAULT '',
		tool_name TEXT NOT NULL,
		action_class TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		UNIQUE(run_id, sequence),
		CHECK(sequence > 0)
	);`,
	`CREATE INDEX idx_run_tool_calls_run_created_at
		ON run_tool_calls(run_id, created_at);`,
}

var typedScriptProcessStatements = []string{
	`CREATE TABLE script_process_proposals (
		id TEXT PRIMARY KEY,
		operation_key_digest TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		executable TEXT NOT NULL,
		arguments_json TEXT NOT NULL,
		working_directory TEXT NOT NULL,
		requested_backend TEXT NOT NULL,
		execution_mode TEXT NOT NULL,
		status TEXT NOT NULL,
		risk TEXT NOT NULL,
		policy_reason TEXT NOT NULL,
		stdout TEXT NOT NULL DEFAULT '',
		stderr TEXT NOT NULL DEFAULT '',
		exit_code INTEGER NOT NULL DEFAULT 0,
		request_fingerprint TEXT NOT NULL,
		approval_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		CHECK(length(operation_key_digest) = 64),
		CHECK(length(request_fingerprint) = 64),
		CHECK(length(approval_fingerprint) = 64),
		CHECK(working_directory = '.'),
		CHECK(execution_mode = 'disabled'),
		CHECK(requested_backend IN ('sandbox', 'local')),
		CHECK(risk IN ('low', 'medium', 'high', 'critical')),
		CHECK(status IN ('proposed', 'approved', 'denied', 'completed', 'failed')),
		CHECK(json_valid(arguments_json)),
		CHECK(version > 0)
	);`,
	`CREATE INDEX idx_script_process_run_status_updated_at
		ON script_process_proposals(run_id, status, updated_at);`,
	`CREATE INDEX idx_script_process_session_status_updated_at
		ON script_process_proposals(session_id, status, updated_at);`,
	`CREATE INDEX idx_script_process_workspace_status_updated_at
		ON script_process_proposals(workspace_id, status, updated_at);`,
}

var runArtifactStatements = []string{
	`CREATE TABLE run_artifacts (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT '',
		source_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		stream TEXT NOT NULL,
		kind TEXT NOT NULL,
		mime TEXT NOT NULL,
		encoding TEXT NOT NULL,
		sha256 TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		content TEXT NOT NULL,
		redacted INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		UNIQUE(run_id, source_id, stream),
		CHECK(stream IN ('stdout', 'stderr')),
		CHECK(kind = 'tool_output'),
		CHECK(encoding = 'utf-8'),
		CHECK(length(sha256) = 64),
		CHECK(size_bytes > 0 AND size_bytes <= 4194304),
		CHECK(redacted IN (0, 1))
	);`,
	`CREATE INDEX idx_run_artifacts_run_created_at
		ON run_artifacts(run_id, created_at, id);`,
	`CREATE INDEX idx_run_artifacts_source_stream
		ON run_artifacts(source_id, stream, created_at);`,
}

var structuredToolOperationStatements = []string{
	`CREATE TABLE structured_tool_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		invocation_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT '',
		tool_name TEXT NOT NULL,
		target_kind TEXT NOT NULL,
		target_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		FOREIGN KEY(invocation_id) REFERENCES run_tool_calls(id) ON DELETE RESTRICT,
		UNIQUE(tool_name, target_id),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK((target_kind = 'work_item' AND tool_name = 'work_item_create')
			OR (target_kind = 'note' AND tool_name = 'note_create'))
	);`,
	`CREATE INDEX idx_structured_tool_operations_run_created_at
		ON structured_tool_operations(run_id, created_at, operation_key_digest);`,
	`CREATE TRIGGER trg_structured_tool_operation_invocation_scope
		BEFORE INSERT ON structured_tool_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM run_tool_calls
			WHERE id = NEW.invocation_id AND run_id = NEW.run_id AND session_id = NEW.session_id
				AND workspace_id = NEW.workspace_id AND tool_name = NEW.tool_name
				AND action_class = 'run_memory'
		)
		BEGIN
			SELECT RAISE(ABORT, 'structured tool invocation scope mismatch');
		END;`,
	`CREATE TRIGGER trg_structured_tool_operation_work_item_target
		BEFORE INSERT ON structured_tool_operations
		WHEN NEW.target_kind = 'work_item'
			AND NOT EXISTS (SELECT 1 FROM work_items WHERE id = NEW.target_id AND run_id = NEW.run_id)
		BEGIN
			SELECT RAISE(ABORT, 'structured tool WorkItem target mismatch');
		END;`,
	`CREATE TRIGGER trg_structured_tool_operation_note_target
		BEFORE INSERT ON structured_tool_operations
		WHEN NEW.target_kind = 'note'
			AND NOT EXISTS (SELECT 1 FROM notes WHERE id = NEW.target_id AND run_id = NEW.run_id)
		BEGIN
			SELECT RAISE(ABORT, 'structured tool Note target mismatch');
		END;`,
}

var supervisorToolLoopStatements = []string{
	`CREATE TABLE run_supervisor_tool_rounds (
		run_id TEXT NOT NULL,
		turn INTEGER NOT NULL,
		attempt_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		model_attempt INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		completed_at TEXT,
		PRIMARY KEY(run_id, turn, attempt_id, round),
		UNIQUE(run_id, turn, attempt_id, model_attempt),
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		CHECK(turn > 0),
		CHECK(round BETWEEN 1 AND 4),
		CHECK(model_attempt > 0)
	);`,
	`CREATE TABLE run_supervisor_tool_calls (
		run_id TEXT NOT NULL,
		turn INTEGER NOT NULL,
		attempt_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		position INTEGER NOT NULL,
		model_attempt INTEGER NOT NULL,
		call_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		status TEXT NOT NULL,
		result_json TEXT NOT NULL DEFAULT '',
		error_code TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		completed_at TEXT,
		PRIMARY KEY(run_id, turn, attempt_id, round, position),
		UNIQUE(run_id, turn, attempt_id, call_id),
		FOREIGN KEY(run_id, turn, attempt_id, round)
			REFERENCES run_supervisor_tool_rounds(run_id, turn, attempt_id, round) ON DELETE CASCADE,
		CHECK(position BETWEEN 1 AND 4),
		CHECK(model_attempt > 0),
		CHECK(tool_name IN ('work_item_create', 'note_create')),
		CHECK(status IN ('pending', 'completed', 'denied', 'failed')),
		CHECK((status = 'pending' AND result_json = '' AND error_code = '' AND completed_at IS NULL)
			OR (status = 'completed' AND length(result_json) > 0 AND error_code = '' AND completed_at IS NOT NULL)
			OR (status IN ('denied', 'failed') AND length(result_json) > 0 AND length(error_code) > 0
				AND completed_at IS NOT NULL))
	);`,
	`CREATE INDEX idx_run_supervisor_tool_calls_pending
		ON run_supervisor_tool_calls(run_id, turn, attempt_id, status, round, position);`,
	`CREATE TRIGGER trg_supervisor_tool_round_active_attempt
		BEFORE INSERT ON run_supervisor_tool_rounds
		WHEN NOT EXISTS (
			SELECT 1 FROM run_supervisor_checkpoints
			WHERE run_id = NEW.run_id AND next_turn = NEW.turn AND attempt_id = NEW.attempt_id
				AND phase = 'turn_started'
		)
		BEGIN
			SELECT RAISE(ABORT, 'supervisor tool round is not bound to the active turn');
		END;`,
	`CREATE TRIGGER trg_supervisor_tool_round_completed_model
		BEFORE INSERT ON run_supervisor_tool_rounds
		WHEN NOT EXISTS (
			SELECT 1 FROM run_events
			WHERE run_id = NEW.run_id AND type = 'model.completed' AND source = 'model_gateway'
				AND subject_id = NEW.attempt_id || '/model/' || NEW.model_attempt
		)
		BEGIN
			SELECT RAISE(ABORT, 'supervisor tool round requires a completed model attempt');
		END;`,
	`CREATE TRIGGER trg_supervisor_tool_call_model_attempt
		BEFORE INSERT ON run_supervisor_tool_calls
		WHEN NOT EXISTS (
			SELECT 1 FROM run_supervisor_tool_rounds
			WHERE run_id = NEW.run_id AND turn = NEW.turn AND attempt_id = NEW.attempt_id
				AND round = NEW.round AND model_attempt = NEW.model_attempt
		)
		BEGIN
			SELECT RAISE(ABORT, 'supervisor tool call model attempt mismatch');
		END;`,
	`CREATE TRIGGER trg_supervisor_tool_round_completion
		BEFORE UPDATE OF completed_at ON run_supervisor_tool_rounds
		WHEN NEW.completed_at IS NOT NULL AND EXISTS (
			SELECT 1 FROM run_supervisor_tool_calls
			WHERE run_id = NEW.run_id AND turn = NEW.turn AND attempt_id = NEW.attempt_id
				AND round = NEW.round AND status = 'pending'
		)
		BEGIN
			SELECT RAISE(ABORT, 'supervisor tool round still has pending calls');
		END;`,
}

var runExecutionLeaseStatements = []string{
	`CREATE TABLE run_execution_leases (
		run_id TEXT PRIMARY KEY,
		lease_id TEXT NOT NULL UNIQUE,
		owner_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		status TEXT NOT NULL,
		acquired_at TEXT NOT NULL,
		renewed_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		released_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		CHECK(generation > 0),
		CHECK(status IN ('active', 'released')),
		CHECK((status = 'active' AND released_at IS NULL)
			OR (status = 'released' AND released_at IS NOT NULL))
	);`,
	`CREATE INDEX idx_run_execution_leases_status_expires
		ON run_execution_leases(status, expires_at);`,
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN lease_id TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE run_supervisor_checkpoints ADD COLUMN lease_generation INTEGER NOT NULL DEFAULT 0
		CHECK(lease_generation >= 0 AND ((lease_id = '' AND lease_generation = 0)
			OR (lease_id <> '' AND lease_generation > 0)));`,
}

func (s *SQLiteStore) applyMigrations(ctx context.Context, migrations []migration) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite store is not open")
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	);`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := s.loadAppliedMigrations(ctx)
	if err != nil {
		return err
	}
	if err := validateMigrationPlan(migrations, applied); err != nil {
		return err
	}
	for _, item := range migrations {
		if _, ok := applied[item.Version]; ok {
			continue
		}
		if err := s.applyMigration(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) applyMigration(ctx context.Context, item migration) error {
	if item.Version <= 0 || strings.TrimSpace(item.Name) == "" || len(item.Statements) == 0 {
		return fmt.Errorf("invalid migration version=%d name=%q", item.Version, item.Name)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", item.Version, err)
	}
	defer func() { _ = tx.Rollback() }()
	for index, stmt := range item.Statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration %d %q statement %d: %w", item.Version, item.Name, index+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, checksum, applied_at)
		VALUES (?, ?, ?, ?)`, item.Version, item.Name, migrationChecksum(item), ts(time.Now().UTC())); err != nil {
		return fmt.Errorf("record migration %d: %w", item.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", item.Version, err)
	}
	return nil
}

func (s *SQLiteStore) loadAppliedMigrations(ctx context.Context) (map[int]appliedMigration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[int]appliedMigration{}
	for rows.Next() {
		var version int
		var item appliedMigration
		if err := rows.Scan(&version, &item.Name, &item.Checksum); err != nil {
			return nil, err
		}
		applied[version] = item
	}
	return applied, rows.Err()
}

func validateMigrationPlan(migrations []migration, applied map[int]appliedMigration) error {
	if len(migrations) != LatestSchemaVersion {
		return fmt.Errorf("latest schema version is %d but migration plan has %d entries", LatestSchemaVersion, len(migrations))
	}
	known := make(map[int]migration, len(migrations))
	for index, item := range migrations {
		expectedVersion := index + 1
		if item.Version != expectedVersion {
			return fmt.Errorf("migration plan must be contiguous: expected version %d, got %d", expectedVersion, item.Version)
		}
		if _, exists := known[item.Version]; exists {
			return fmt.Errorf("duplicate migration version %d", item.Version)
		}
		known[item.Version] = item
	}
	for version, recorded := range applied {
		item, ok := known[version]
		if !ok {
			return fmt.Errorf("database schema version %d is newer or unknown", version)
		}
		if recorded.Name != item.Name || recorded.Checksum != migrationChecksum(item) {
			return fmt.Errorf("migration %d checksum or name mismatch", version)
		}
	}
	for version := 1; version <= len(migrations); version++ {
		if _, ok := applied[version]; ok {
			continue
		}
		for later := version + 1; later <= len(migrations); later++ {
			if _, ok := applied[later]; ok {
				return fmt.Errorf("migration history has a gap at version %d", version)
			}
		}
		break
	}
	return nil
}

func migrationChecksum(item migration) string {
	sum := sha256.Sum256([]byte(strings.Join(item.Statements, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (s *SQLiteStore) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
