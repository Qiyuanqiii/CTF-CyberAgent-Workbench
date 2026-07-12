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

const LatestSchemaVersion = 27

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

var modelCancellationStatements = []string{
	`CREATE TABLE run_model_cancellations (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		attempt_id TEXT NOT NULL,
		model_attempt INTEGER NOT NULL,
		status TEXT NOT NULL,
		reason TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		requested_at TEXT NOT NULL,
		observed_at TEXT,
		resolved_at TEXT,
		resolution TEXT NOT NULL DEFAULT '',
		UNIQUE(run_id, attempt_id, model_attempt),
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		CHECK(model_attempt > 0),
		CHECK(status IN ('pending', 'observed', 'resolved')),
		CHECK((status = 'pending' AND observed_at IS NULL AND resolved_at IS NULL AND resolution = '')
			OR (status = 'observed' AND observed_at IS NOT NULL AND resolved_at IS NULL AND resolution = '')
			OR (status = 'resolved' AND resolved_at IS NOT NULL AND length(resolution) > 0))
	);`,
	`CREATE INDEX idx_run_model_cancellations_pending
		ON run_model_cancellations(run_id, attempt_id, model_attempt, status);`,
	`CREATE TABLE run_model_cancellation_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		cancellation_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		FOREIGN KEY(cancellation_id) REFERENCES run_model_cancellations(id) ON DELETE CASCADE
	);`,
}

var agentCoordinatorStatements = []string{
	`CREATE TABLE agent_nodes (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		parent_id TEXT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		profile TEXT NOT NULL,
		skills_json TEXT NOT NULL,
		status TEXT NOT NULL,
		depth INTEGER NOT NULL,
		child_limit INTEGER NOT NULL,
		turn_limit INTEGER NOT NULL,
		token_limit INTEGER NOT NULL,
		turns_used INTEGER NOT NULL DEFAULT 0,
		tokens_used INTEGER NOT NULL DEFAULT 0,
		active_attempt_id TEXT NOT NULL DEFAULT '',
		status_reason TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		finished_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id, parent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		UNIQUE(run_id, id),
		UNIQUE(run_id, session_id),
		CHECK(role IN ('root', 'specialist')),
		CHECK(status IN ('ready', 'running', 'waiting', 'completed', 'failed', 'cancelled')),
		CHECK(depth BETWEEN 0 AND 1),
		CHECK(child_limit BETWEEN 0 AND 2),
		CHECK(turn_limit > 0),
		CHECK(token_limit >= 0 AND turns_used >= 0 AND tokens_used >= 0),
		CHECK(version > 0),
		CHECK((role = 'root' AND parent_id IS NULL AND depth = 0)
			OR (role = 'specialist' AND parent_id IS NOT NULL AND depth > 0)),
		CHECK((status = 'running' AND length(trim(active_attempt_id)) > 0)
			OR (status <> 'running' AND active_attempt_id = '')),
		CHECK((status IN ('completed', 'failed', 'cancelled') AND finished_at IS NOT NULL)
			OR (status NOT IN ('completed', 'failed', 'cancelled') AND finished_at IS NULL))
	);`,
	`CREATE UNIQUE INDEX idx_agent_nodes_one_root
		ON agent_nodes(run_id) WHERE parent_id IS NULL;`,
	`CREATE INDEX idx_agent_nodes_run_status
		ON agent_nodes(run_id, status, depth, created_at);`,
	`CREATE TRIGGER trg_agent_root_session_matches_run
		BEFORE INSERT ON agent_nodes
		WHEN NEW.role = 'root' AND NOT EXISTS (
			SELECT 1 FROM runs WHERE id = NEW.run_id AND session_id = NEW.session_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'root agent session does not match run');
		END;`,
	`CREATE TRIGGER trg_agent_child_depth
		BEFORE INSERT ON agent_nodes
		WHEN NEW.parent_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM agent_nodes parent
			WHERE parent.run_id = NEW.run_id AND parent.id = NEW.parent_id
				AND NEW.depth = parent.depth + 1 AND parent.child_limit > (
					SELECT COUNT(*) FROM agent_nodes child
					WHERE child.run_id = NEW.run_id AND child.parent_id = parent.id
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'agent child depth or limit is invalid');
		END;`,
	`CREATE TRIGGER trg_agent_node_limit
		BEFORE INSERT ON agent_nodes
		WHEN (SELECT COUNT(*) FROM agent_nodes WHERE run_id = NEW.run_id) >= 3
		BEGIN
			SELECT RAISE(ABORT, 'agent graph node limit reached');
		END;`,
	`CREATE TABLE agent_messages (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		sender_agent_id TEXT,
		recipient_agent_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		kind TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		consumed_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, sender_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, recipient_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		UNIQUE(recipient_agent_id, sequence),
		CHECK(sender_agent_id IS NULL OR sender_agent_id <> recipient_agent_id),
		CHECK(sequence > 0),
		CHECK(kind IN ('control', 'instruction', 'result', 'notification')),
		CHECK(status IN ('pending', 'consumed')),
		CHECK((status = 'pending' AND consumed_at IS NULL)
			OR (status = 'consumed' AND consumed_at IS NOT NULL))
	);`,
	`CREATE INDEX idx_agent_messages_inbox
		ON agent_messages(recipient_agent_id, status, sequence);`,
	`CREATE TRIGGER trg_agent_inbox_limit
		BEFORE INSERT ON agent_messages
		WHEN (SELECT COUNT(*) FROM agent_messages
			WHERE recipient_agent_id = NEW.recipient_agent_id AND status = 'pending') >= 128
		BEGIN
			SELECT RAISE(ABORT, 'agent inbox limit reached');
		END;`,
	`CREATE TRIGGER trg_agent_message_history_limit
		BEFORE INSERT ON agent_messages
		WHEN (SELECT COUNT(*) FROM agent_messages
			WHERE recipient_agent_id = NEW.recipient_agent_id) >= 4096
		BEGIN
			SELECT RAISE(ABORT, 'agent message history limit reached');
		END;`,
	`CREATE TABLE agent_graph_snapshots (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		root_agent_id TEXT NOT NULL,
		node_count INTEGER NOT NULL,
		pending_message_count INTEGER NOT NULL,
		state_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, root_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		UNIQUE(run_id, version),
		CHECK(version > 0),
		CHECK(protocol_version = 'agent_graph.v1'),
		CHECK(node_count BETWEEN 1 AND 3),
		CHECK(pending_message_count BETWEEN 0 AND 384)
	);`,
	`CREATE INDEX idx_agent_graph_snapshots_latest
		ON agent_graph_snapshots(run_id, version DESC);`,
}

var agentInboxProtocolStatements = []string{
	`ALTER TABLE agent_messages ADD COLUMN semantic TEXT NOT NULL DEFAULT 'message'
		CHECK(semantic IN ('message', 'wake', 'dependency'));`,
	`CREATE TABLE agent_message_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		message_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		FOREIGN KEY(message_id) REFERENCES agent_messages(id) ON DELETE CASCADE,
		CHECK(length(operation_key_digest) = 64),
		CHECK(length(request_fingerprint) = 64)
	);`,
}

var specialistAdmissionStatements = []string{
	`CREATE TABLE agent_admission_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		agent_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		FOREIGN KEY(agent_id) REFERENCES agent_nodes(id) ON DELETE CASCADE,
		CHECK(length(operation_key_digest) = 64),
		CHECK(length(request_fingerprint) = 64)
	);`,
}

var agentMemoryOwnershipStatements = []string{
	`ALTER TABLE work_items ADD COLUMN owner_agent_id TEXT
		REFERENCES agent_nodes(id) ON DELETE RESTRICT
		CHECK(owner_agent_id IS NULL OR length(trim(owner_agent_id)) > 0);`,
	`ALTER TABLE notes ADD COLUMN owner_agent_id TEXT
		REFERENCES agent_nodes(id) ON DELETE RESTRICT
		CHECK(owner_agent_id IS NULL OR length(trim(owner_agent_id)) > 0);`,
	`CREATE INDEX idx_work_items_owner_agent
		ON work_items(run_id, owner_agent_id)
		WHERE owner_agent_id IS NOT NULL;`,
	`CREATE INDEX idx_notes_owner_agent
		ON notes(run_id, owner_agent_id)
		WHERE owner_agent_id IS NOT NULL;`,
	`CREATE TRIGGER trg_work_item_owner_agent_insert
		BEFORE INSERT ON work_items
		WHEN NEW.owner_agent_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM agent_nodes owner
			WHERE owner.id = NEW.owner_agent_id AND owner.run_id = NEW.run_id
				AND owner.status NOT IN ('completed', 'failed', 'cancelled')
		)
		BEGIN
			SELECT RAISE(ABORT, 'work item owner Agent does not belong to Run');
		END;`,
	`CREATE TRIGGER trg_work_item_owner_agent_update
		BEFORE UPDATE OF owner_agent_id, run_id ON work_items
		WHEN NEW.owner_agent_id IS NOT NULL AND (
			NOT EXISTS (
				SELECT 1 FROM agent_nodes owner
				WHERE owner.id = NEW.owner_agent_id AND owner.run_id = NEW.run_id
			) OR (
				COALESCE(OLD.owner_agent_id, '') <> NEW.owner_agent_id AND EXISTS (
					SELECT 1 FROM agent_nodes owner
					WHERE owner.id = NEW.owner_agent_id
						AND owner.status IN ('completed', 'failed', 'cancelled')
				)
			)
		)
		BEGIN
			SELECT RAISE(ABORT, 'work item owner Agent does not belong to Run');
		END;`,
	`CREATE TRIGGER trg_note_owner_agent_insert
		BEFORE INSERT ON notes
		WHEN NEW.owner_agent_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM agent_nodes owner
			WHERE owner.id = NEW.owner_agent_id AND owner.run_id = NEW.run_id
				AND owner.status NOT IN ('completed', 'failed', 'cancelled')
		)
		BEGIN
			SELECT RAISE(ABORT, 'note owner Agent does not belong to Run');
		END;`,
	`CREATE TRIGGER trg_note_owner_agent_update
		BEFORE UPDATE OF owner_agent_id, run_id ON notes
		WHEN NEW.owner_agent_id IS NOT NULL AND (
			NOT EXISTS (
				SELECT 1 FROM agent_nodes owner
				WHERE owner.id = NEW.owner_agent_id AND owner.run_id = NEW.run_id
			) OR (
				COALESCE(OLD.owner_agent_id, '') <> NEW.owner_agent_id AND EXISTS (
					SELECT 1 FROM agent_nodes owner
					WHERE owner.id = NEW.owner_agent_id
						AND owner.status IN ('completed', 'failed', 'cancelled')
				)
			)
		)
		BEGIN
			SELECT RAISE(ABORT, 'note owner Agent does not belong to Run');
		END;`,
}

var agentCompletionReportStatements = []string{
	`CREATE TABLE agent_completion_reports (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL UNIQUE,
		parent_agent_id TEXT NOT NULL,
		attempt_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		outcome TEXT NOT NULL,
		summary TEXT NOT NULL,
		work_item_ids_json TEXT NOT NULL,
		note_ids_json TEXT NOT NULL,
		message_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, parent_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(message_id) REFERENCES agent_messages(id) ON DELETE CASCADE,
		UNIQUE(run_id, id),
		CHECK(agent_id <> parent_agent_id),
		CHECK(length(trim(attempt_id)) > 0),
		CHECK(protocol_version = 'agent_completion.v1'),
		CHECK(outcome IN ('succeeded', 'partial')),
		CHECK(length(trim(summary)) BETWEEN 1 AND 4096),
		CHECK(length(CAST(summary AS BLOB)) <= 8192),
		CHECK(json_valid(work_item_ids_json) AND json_type(work_item_ids_json) = 'array'),
		CHECK(json_valid(note_ids_json) AND json_type(note_ids_json) = 'array'),
		CHECK(length(CAST(work_item_ids_json AS BLOB)) <= 8192),
		CHECK(length(CAST(note_ids_json AS BLOB)) <= 8192)
	);`,
	`CREATE INDEX idx_agent_completion_reports_run_created
		ON agent_completion_reports(run_id, created_at, id);`,
	`CREATE TABLE agent_completion_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		report_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES agent_completion_reports(id) ON DELETE CASCADE,
		CHECK(length(operation_key_digest) = 64),
		CHECK(length(request_fingerprint) = 64)
	);`,
	`CREATE TRIGGER trg_agent_completion_running_child
		BEFORE INSERT ON agent_completion_reports
		WHEN NOT EXISTS (
			SELECT 1 FROM agent_nodes child
			JOIN agent_nodes parent
				ON parent.run_id = child.run_id AND parent.id = child.parent_id
			WHERE child.run_id = NEW.run_id AND child.id = NEW.agent_id
				AND parent.id = NEW.parent_agent_id
				AND child.role = 'specialist' AND parent.role = 'root'
				AND child.status = 'running' AND child.active_attempt_id = NEW.attempt_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'completion report requires the active Specialist attempt');
		END;`,
	`CREATE TRIGGER trg_agent_completion_message_matches
		BEFORE INSERT ON agent_completion_reports
		WHEN NOT EXISTS (
			SELECT 1 FROM agent_messages message
			WHERE message.id = NEW.message_id AND message.run_id = NEW.run_id
				AND message.sender_agent_id = NEW.agent_id
				AND message.recipient_agent_id = NEW.parent_agent_id
				AND message.kind = 'result' AND message.semantic = 'message'
				AND message.status = 'pending'
		)
		BEGIN
			SELECT RAISE(ABORT, 'completion report message does not match its Agent relationship');
		END;`,
	`CREATE TRIGGER trg_agent_completed_requires_report
		BEFORE UPDATE OF status ON agent_nodes
		WHEN OLD.role = 'specialist' AND NEW.status = 'completed' AND OLD.status <> 'completed'
			AND NOT EXISTS (
				SELECT 1 FROM agent_completion_reports report
				WHERE report.run_id = NEW.run_id AND report.agent_id = NEW.id
					AND report.attempt_id = OLD.active_attempt_id
			)
		BEGIN
			SELECT RAISE(ABORT, 'completed Specialist requires a completion report');
		END;`,
	`CREATE TRIGGER trg_agent_completion_immutable
		BEFORE UPDATE ON agent_completion_reports
		BEGIN
			SELECT RAISE(ABORT, 'completion report is immutable');
		END;`,
}

var specialistAttemptStatements = []string{
	`CREATE TABLE agent_attempts (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		parent_agent_id TEXT NOT NULL,
		lease_id TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		turn_number INTEGER NOT NULL,
		status TEXT NOT NULL,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		execution_millis INTEGER NOT NULL DEFAULT 0,
		usage_recorded_at TEXT,
		failure_code TEXT NOT NULL DEFAULT '',
		failure_reason TEXT NOT NULL DEFAULT '',
		notification_message_id TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		finished_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, parent_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		UNIQUE(run_id, id),
		UNIQUE(agent_id, turn_number),
		CHECK(agent_id <> parent_agent_id),
		CHECK(lease_generation > 0 AND turn_number > 0),
		CHECK(status IN ('running', 'continued', 'finished', 'crashed', 'interrupted')),
		CHECK(input_tokens >= 0 AND output_tokens >= 0 AND total_tokens >= 0 AND execution_millis >= 0),
		CHECK(total_tokens >= input_tokens AND total_tokens >= output_tokens),
		CHECK((usage_recorded_at IS NULL AND input_tokens = 0 AND output_tokens = 0
			AND total_tokens = 0 AND execution_millis = 0) OR usage_recorded_at IS NOT NULL),
		CHECK((status = 'running' AND finished_at IS NULL AND failure_code = ''
			AND failure_reason = '' AND notification_message_id = '')
			OR (status = 'continued' AND finished_at IS NOT NULL AND usage_recorded_at IS NOT NULL
				AND failure_code = '' AND failure_reason = '' AND notification_message_id = '')
			OR (status = 'finished' AND finished_at IS NOT NULL AND usage_recorded_at IS NOT NULL
				AND failure_code = '' AND failure_reason = '' AND length(trim(notification_message_id)) > 0)
			OR (status = 'crashed' AND finished_at IS NOT NULL AND length(trim(failure_code)) > 0
				AND length(trim(failure_reason)) > 0 AND length(trim(notification_message_id)) > 0)
			OR (status = 'interrupted' AND finished_at IS NOT NULL AND length(trim(failure_code)) > 0
				AND length(trim(failure_reason)) > 0 AND notification_message_id = ''))
	);`,
	`CREATE UNIQUE INDEX idx_agent_attempts_one_running
		ON agent_attempts(agent_id) WHERE status = 'running';`,
	`CREATE INDEX idx_agent_attempts_run_status
		ON agent_attempts(run_id, status, lease_generation, started_at);`,
	`CREATE TABLE agent_attempt_mutations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		attempt_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES agent_attempts(id) ON DELETE CASCADE,
		UNIQUE(attempt_id, kind),
		CHECK(length(operation_key_digest) = 64),
		CHECK(length(request_fingerprint) = 64),
		CHECK(kind IN ('start', 'usage', 'continue', 'crash'))
	);`,
	`CREATE TRIGGER trg_agent_attempt_running_child
		BEFORE INSERT ON agent_attempts
		WHEN NOT EXISTS (
			SELECT 1 FROM agent_nodes child
			JOIN agent_nodes parent
				ON parent.run_id = child.run_id AND parent.id = child.parent_id
			JOIN runs run ON run.id = child.run_id
			JOIN run_execution_leases lease ON lease.run_id = child.run_id
			JOIN sessions child_session ON child_session.id = child.session_id
			WHERE child.run_id = NEW.run_id AND child.id = NEW.agent_id
				AND parent.id = NEW.parent_agent_id AND child.role = 'specialist'
				AND parent.role = 'root' AND child.status = 'ready'
				AND child.active_attempt_id = '' AND run.status = 'running'
				AND lease.lease_id = NEW.lease_id
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND child_session.status = 'active'
		)
		BEGIN
			SELECT RAISE(ABORT, 'Agent attempt requires a ready Specialist in a running Run');
		END;`,
	`CREATE TRIGGER trg_specialist_running_requires_attempt
		BEFORE UPDATE OF status, active_attempt_id ON agent_nodes
		WHEN NEW.role = 'specialist' AND NEW.status = 'running' AND NOT EXISTS (
			SELECT 1 FROM agent_attempts attempt
			WHERE attempt.run_id = NEW.run_id AND attempt.agent_id = NEW.id
				AND attempt.parent_agent_id = NEW.parent_id
				AND attempt.id = NEW.active_attempt_id AND attempt.status = 'running'
		)
		BEGIN
			SELECT RAISE(ABORT, 'running Specialist requires a matching Agent attempt');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_terminal_child
		BEFORE UPDATE OF status ON agent_attempts
		WHEN OLD.status = 'running' AND NEW.status <> 'running' AND NOT EXISTS (
			SELECT 1 FROM agent_nodes child
			WHERE child.run_id = OLD.run_id AND child.id = OLD.agent_id
				AND child.status = 'running' AND child.active_attempt_id = OLD.id
		)
		BEGIN
			SELECT RAISE(ABORT, 'terminal Agent attempt requires its running Specialist');
		END;`,
	`CREATE TRIGGER trg_specialist_nonrunning_requires_terminal_attempt
		BEFORE UPDATE OF status, active_attempt_id ON agent_nodes
		WHEN OLD.role = 'specialist' AND OLD.status = 'running' AND NEW.status <> 'running'
			AND NOT EXISTS (
				SELECT 1 FROM agent_attempts attempt
				WHERE attempt.run_id = OLD.run_id AND attempt.agent_id = OLD.id
					AND attempt.id = OLD.active_attempt_id AND attempt.status <> 'running'
			)
		BEGIN
			SELECT RAISE(ABORT, 'non-running Specialist requires a terminal Agent attempt');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_identity_immutable
		BEFORE UPDATE OF id, run_id, agent_id, parent_agent_id, lease_id, lease_generation,
			turn_number, started_at ON agent_attempts
		BEGIN
			SELECT RAISE(ABORT, 'Agent attempt identity is immutable');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_terminal_immutable
		BEFORE UPDATE ON agent_attempts
		WHEN OLD.status <> 'running'
		BEGIN
			SELECT RAISE(ABORT, 'terminal Agent attempt is immutable');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_usage_immutable
		BEFORE UPDATE OF input_tokens, output_tokens, total_tokens, execution_millis,
			usage_recorded_at ON agent_attempts
		WHEN OLD.usage_recorded_at IS NOT NULL
		BEGIN
			SELECT RAISE(ABORT, 'Agent attempt usage is immutable');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_usage_requires_lease
		BEFORE UPDATE OF input_tokens, output_tokens, total_tokens, execution_millis,
			usage_recorded_at ON agent_attempts
		WHEN OLD.usage_recorded_at IS NULL AND NEW.usage_recorded_at IS NOT NULL
			AND NOT EXISTS (
				SELECT 1 FROM run_execution_leases lease
				WHERE lease.run_id = OLD.run_id AND lease.lease_id = OLD.lease_id
					AND lease.generation = OLD.lease_generation AND lease.status = 'active'
					AND julianday(lease.expires_at) > julianday('now')
			)
		BEGIN
			SELECT RAISE(ABORT, 'Agent attempt usage requires its active Run lease');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_notification_matches
		BEFORE UPDATE OF status, notification_message_id ON agent_attempts
		WHEN NEW.status IN ('finished', 'crashed') AND NOT EXISTS (
			SELECT 1 FROM agent_messages message
			WHERE message.id = NEW.notification_message_id AND message.run_id = NEW.run_id
				AND message.sender_agent_id = NEW.agent_id
				AND message.recipient_agent_id = NEW.parent_agent_id
				AND message.status = 'pending'
				AND ((NEW.status = 'finished' AND message.kind = 'result')
					OR (NEW.status = 'crashed' AND message.kind = 'notification'))
		)
		BEGIN
			SELECT RAISE(ABORT, 'terminal Agent attempt notification does not match its parent message');
		END;`,
	`CREATE TRIGGER trg_completion_requires_agent_attempt
		BEFORE INSERT ON agent_completion_reports
		WHEN NOT EXISTS (
			SELECT 1 FROM agent_attempts attempt
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			WHERE attempt.id = NEW.attempt_id AND attempt.run_id = NEW.run_id
				AND attempt.agent_id = NEW.agent_id
				AND attempt.parent_agent_id = NEW.parent_agent_id
				AND attempt.status = 'running' AND attempt.usage_recorded_at IS NOT NULL
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
		)
		BEGIN
			SELECT RAISE(ABORT, 'completion report requires a usage-recorded Agent attempt');
		END;`,
}

var rootInboxContextStatements = []string{
	`CREATE TABLE root_inbox_deliveries (
		run_id TEXT NOT NULL,
		root_agent_id TEXT NOT NULL,
		supervisor_attempt_id TEXT NOT NULL,
		turn_number INTEGER NOT NULL,
		message_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		status TEXT NOT NULL,
		prepared_at TEXT NOT NULL,
		resolved_at TEXT,
		PRIMARY KEY(supervisor_attempt_id, message_id),
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, root_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(message_id) REFERENCES agent_messages(id) ON DELETE CASCADE,
		UNIQUE(supervisor_attempt_id, ordinal),
		CHECK(turn_number > 0),
		CHECK(ordinal BETWEEN 1 AND 4),
		CHECK(status IN ('prepared', 'committed', 'superseded')),
		CHECK((status = 'prepared' AND resolved_at IS NULL)
			OR (status IN ('committed', 'superseded') AND resolved_at IS NOT NULL))
	);`,
	`CREATE UNIQUE INDEX idx_root_inbox_one_prepared_message
		ON root_inbox_deliveries(message_id) WHERE status = 'prepared';`,
	`CREATE UNIQUE INDEX idx_root_inbox_one_committed_message
		ON root_inbox_deliveries(message_id) WHERE status = 'committed';`,
	`CREATE INDEX idx_root_inbox_run_status
		ON root_inbox_deliveries(run_id, status, prepared_at);`,
	`CREATE TRIGGER trg_root_inbox_delivery_insert
		BEFORE INSERT ON root_inbox_deliveries
		WHEN NOT EXISTS (
			SELECT 1 FROM run_supervisor_checkpoints checkpoint
			JOIN runs run ON run.id = checkpoint.run_id
			JOIN run_execution_leases lease ON lease.run_id = checkpoint.run_id
			JOIN agent_nodes root
				ON root.run_id = checkpoint.run_id AND root.id = NEW.root_agent_id
			JOIN agent_messages message ON message.id = NEW.message_id
			JOIN agent_nodes sender
				ON sender.run_id = message.run_id AND sender.id = message.sender_agent_id
			WHERE checkpoint.run_id = NEW.run_id
				AND checkpoint.phase = 'turn_started'
				AND checkpoint.attempt_id = NEW.supervisor_attempt_id
				AND checkpoint.next_turn = NEW.turn_number
				AND run.status = 'running'
				AND lease.lease_id = checkpoint.lease_id
				AND lease.generation = checkpoint.lease_generation
				AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND root.role = 'root' AND root.status = 'running'
				AND root.active_attempt_id = checkpoint.attempt_id
				AND message.run_id = NEW.run_id
				AND message.recipient_agent_id = root.id
				AND message.status = 'pending'
				AND sender.role = 'specialist' AND sender.parent_id = root.id
				AND (
					(message.semantic = 'dependency' AND message.kind = 'notification')
					OR (message.semantic = 'message' AND message.kind = 'result' AND EXISTS (
						SELECT 1 FROM agent_completion_reports report
						WHERE report.message_id = message.id AND report.agent_id = sender.id
					))
					OR (message.semantic = 'message' AND message.kind = 'notification' AND EXISTS (
						SELECT 1 FROM agent_attempts attempt
						WHERE attempt.notification_message_id = message.id
							AND attempt.agent_id = sender.id AND attempt.status = 'crashed'
					))
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'root inbox delivery requires an eligible pending child message');
		END;`,
	`CREATE TRIGGER trg_root_inbox_delivery_commit
		BEFORE UPDATE OF status, resolved_at ON root_inbox_deliveries
		WHEN OLD.status = 'prepared' AND NEW.status = 'committed' AND NOT EXISTS (
			SELECT 1 FROM run_supervisor_checkpoints checkpoint
			JOIN runs run ON run.id = checkpoint.run_id
			JOIN run_execution_leases lease ON lease.run_id = checkpoint.run_id
			JOIN agent_nodes root
				ON root.run_id = checkpoint.run_id AND root.id = OLD.root_agent_id
			JOIN agent_messages message ON message.id = OLD.message_id
			WHERE checkpoint.run_id = OLD.run_id
				AND checkpoint.phase = 'turn_started'
				AND checkpoint.attempt_id = OLD.supervisor_attempt_id
				AND checkpoint.next_turn = OLD.turn_number
				AND run.status = 'running'
				AND lease.lease_id = checkpoint.lease_id
				AND lease.generation = checkpoint.lease_generation
				AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND root.status = 'running' AND root.active_attempt_id = checkpoint.attempt_id
				AND message.run_id = OLD.run_id
				AND message.recipient_agent_id = root.id AND message.status = 'pending'
		)
		BEGIN
			SELECT RAISE(ABORT, 'root inbox delivery commit requires its active Supervisor attempt');
		END;`,
	`CREATE TRIGGER trg_root_inbox_delivery_active_supersede
		BEFORE UPDATE OF status ON root_inbox_deliveries
		WHEN OLD.status = 'prepared' AND NEW.status = 'superseded' AND EXISTS (
			SELECT 1 FROM run_supervisor_checkpoints checkpoint
			JOIN runs run ON run.id = checkpoint.run_id
			JOIN agent_nodes root
				ON root.run_id = checkpoint.run_id AND root.id = OLD.root_agent_id
			WHERE checkpoint.run_id = OLD.run_id AND run.status = 'running'
				AND checkpoint.phase = 'turn_started'
				AND checkpoint.attempt_id = OLD.supervisor_attempt_id
				AND root.status = 'running' AND root.active_attempt_id = OLD.supervisor_attempt_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'active root inbox delivery cannot be superseded');
		END;`,
	`CREATE TRIGGER trg_root_inbox_delivery_identity_immutable
		BEFORE UPDATE OF run_id, root_agent_id, supervisor_attempt_id, turn_number,
			message_id, ordinal, prepared_at ON root_inbox_deliveries
		BEGIN
			SELECT RAISE(ABORT, 'root inbox delivery identity is immutable');
		END;`,
	`CREATE TRIGGER trg_root_inbox_delivery_terminal_immutable
		BEFORE UPDATE ON root_inbox_deliveries
		WHEN OLD.status <> 'prepared'
		BEGIN
			SELECT RAISE(ABORT, 'terminal root inbox delivery is immutable');
		END;`,
	`CREATE TRIGGER trg_root_inbox_delivery_prepared_delete
		BEFORE DELETE ON root_inbox_deliveries
		WHEN OLD.status = 'prepared'
		BEGIN
			SELECT RAISE(ABORT, 'prepared root inbox delivery cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_agent_message_prepared_delivery
		BEFORE UPDATE OF status, consumed_at ON agent_messages
		WHEN OLD.status = 'pending' AND NEW.status = 'consumed' AND EXISTS (
			SELECT 1 FROM root_inbox_deliveries delivery
			WHERE delivery.message_id = OLD.id AND delivery.status = 'prepared'
		)
		BEGIN
			SELECT RAISE(ABORT, 'prepared root inbox message must commit through its Supervisor turn');
		END;`,
}

var specialistModelCallStatements = []string{
	`CREATE TABLE specialist_model_calls (
		agent_attempt_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		model_attempt_number INTEGER NOT NULL,
		transport_attempt INTEGER NOT NULL,
		max_attempts INTEGER NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		input_fingerprint TEXT NOT NULL DEFAULT '',
		action_fingerprint TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		outcome TEXT NOT NULL DEFAULT '',
		error_text TEXT NOT NULL DEFAULT '',
		retry_after_millis INTEGER NOT NULL DEFAULT 0,
		retry_planned INTEGER NOT NULL DEFAULT 0,
		elapsed_millis INTEGER NOT NULL DEFAULT 0,
		stream_events INTEGER NOT NULL DEFAULT 0,
		stream_bytes INTEGER NOT NULL DEFAULT 0,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		usage_recorded INTEGER NOT NULL DEFAULT 0,
		action_kind TEXT NOT NULL DEFAULT '',
		report_outcome TEXT NOT NULL DEFAULT '',
		policy_allowed INTEGER NOT NULL DEFAULT -1,
		policy_needs_approval INTEGER NOT NULL DEFAULT 0,
		policy_risk TEXT NOT NULL DEFAULT '',
		policy_reason TEXT NOT NULL DEFAULT '',
		user_message_id INTEGER,
		assistant_message_id INTEGER,
		started_at TEXT NOT NULL,
		finished_at TEXT,
		PRIMARY KEY(agent_attempt_id, model_attempt_number),
		FOREIGN KEY(agent_attempt_id) REFERENCES agent_attempts(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(user_message_id) REFERENCES session_messages(id),
		FOREIGN KEY(assistant_message_id) REFERENCES session_messages(id),
		CHECK(model_attempt_number > 0 AND transport_attempt = model_attempt_number),
		CHECK(max_attempts BETWEEN 1 AND 5 AND model_attempt_number <= max_attempts),
		CHECK(status IN ('started', 'completed', 'failed')),
		CHECK(outcome IN ('', 'success', 'retryable', 'rate_limited', 'invalid_response', 'cancelled', 'permanent')),
		CHECK(retry_after_millis >= 0 AND retry_planned IN (0, 1)),
		CHECK(elapsed_millis >= 0 AND stream_events >= 0 AND stream_events <= 4096),
		CHECK(stream_bytes >= 0 AND stream_bytes <= 65536),
		CHECK(input_tokens >= 0 AND output_tokens >= 0 AND total_tokens >= 0),
		CHECK(total_tokens >= input_tokens AND total_tokens >= output_tokens),
		CHECK(usage_recorded IN (0, 1)),
		CHECK(policy_allowed IN (-1, 0, 1) AND policy_needs_approval IN (0, 1)),
		CHECK((status = 'started' AND outcome = '' AND error_text = ''
			AND input_fingerprint = '' AND action_fingerprint = ''
			AND retry_after_millis = 0 AND retry_planned = 0 AND elapsed_millis = 0
			AND stream_events = 0 AND stream_bytes = 0 AND usage_recorded = 0
			AND input_tokens = 0 AND output_tokens = 0 AND total_tokens = 0
			AND action_kind = '' AND report_outcome = '' AND policy_allowed = -1
			AND policy_needs_approval = 0 AND policy_risk = '' AND policy_reason = ''
			AND user_message_id IS NULL AND assistant_message_id IS NULL AND finished_at IS NULL)
		OR (status = 'completed' AND outcome = 'success' AND error_text = ''
			AND length(input_fingerprint) = 64 AND length(action_fingerprint) = 64
			AND retry_after_millis = 0 AND retry_planned = 0 AND usage_recorded = 1
			AND action_kind IN ('continue', 'finish')
			AND ((action_kind = 'continue' AND report_outcome = '')
				OR (action_kind = 'finish' AND report_outcome IN ('succeeded', 'partial')))
			AND policy_allowed IN (0, 1) AND length(trim(policy_reason)) > 0
			AND ((policy_allowed = 1 AND policy_needs_approval = 0
				AND user_message_id IS NOT NULL AND assistant_message_id IS NOT NULL)
				OR ((policy_allowed = 0 OR policy_needs_approval = 1)
					AND user_message_id IS NULL AND assistant_message_id IS NULL))
			AND finished_at IS NOT NULL)
		OR (status = 'failed' AND outcome IN ('retryable', 'rate_limited',
				'invalid_response', 'cancelled', 'permanent')
			AND input_fingerprint = '' AND action_fingerprint = ''
			AND length(trim(error_text)) > 0 AND action_kind = '' AND report_outcome = ''
			AND policy_allowed = -1 AND policy_needs_approval = 0
			AND policy_risk = '' AND policy_reason = ''
			AND user_message_id IS NULL AND assistant_message_id IS NULL
			AND (usage_recorded = 1 OR (input_tokens = 0 AND output_tokens = 0 AND total_tokens = 0))
			AND (retry_planned = 0 OR (usage_recorded = 0
				AND outcome IN ('retryable', 'rate_limited')
				AND model_attempt_number < max_attempts))
			AND finished_at IS NOT NULL))
	);`,
	`CREATE UNIQUE INDEX idx_specialist_model_one_started
		ON specialist_model_calls(agent_attempt_id) WHERE status = 'started';`,
	`CREATE INDEX idx_specialist_model_agent_started
		ON specialist_model_calls(agent_id, started_at, model_attempt_number);`,
	`CREATE TRIGGER trg_specialist_model_call_sequence
		BEFORE INSERT ON specialist_model_calls
		WHEN NEW.model_attempt_number <> (
			SELECT COUNT(*) + 1 FROM specialist_model_calls
			WHERE agent_attempt_id = NEW.agent_attempt_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model call is not the next attempt');
		END;`,
	`CREATE TRIGGER trg_specialist_model_call_insert
		BEFORE INSERT ON specialist_model_calls
		WHEN NOT EXISTS (
			SELECT 1 FROM agent_attempts attempt
			JOIN agent_nodes child
				ON child.run_id = attempt.run_id AND child.id = attempt.agent_id
			JOIN runs run ON run.id = attempt.run_id
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			WHERE attempt.id = NEW.agent_attempt_id AND attempt.run_id = NEW.run_id
				AND attempt.agent_id = NEW.agent_id AND attempt.status = 'running'
				AND attempt.usage_recorded_at IS NULL
				AND child.role = 'specialist' AND child.status = 'running'
				AND child.active_attempt_id = attempt.id AND run.status = 'running'
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model call requires its active leased attempt');
		END;`,
	`CREATE TRIGGER trg_specialist_model_call_terminal_requires_lease
		BEFORE UPDATE OF status ON specialist_model_calls
		WHEN OLD.status = 'started' AND NEW.status <> 'started' AND NOT EXISTS (
			SELECT 1 FROM agent_attempts attempt
			JOIN agent_nodes child
				ON child.run_id = attempt.run_id AND child.id = attempt.agent_id
			JOIN runs run ON run.id = attempt.run_id
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			WHERE attempt.id = OLD.agent_attempt_id AND attempt.run_id = OLD.run_id
				AND attempt.agent_id = OLD.agent_id AND attempt.status = 'running'
				AND child.role = 'specialist' AND child.status = 'running'
				AND child.active_attempt_id = attempt.id AND run.status = 'running'
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
				AND ((NEW.usage_recorded = 0 AND attempt.usage_recorded_at IS NULL)
					OR (NEW.usage_recorded = 1 AND attempt.usage_recorded_at IS NOT NULL
						AND attempt.input_tokens = NEW.input_tokens
						AND attempt.output_tokens = NEW.output_tokens
						AND attempt.total_tokens = NEW.total_tokens))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model terminal requires its active leased usage state');
		END;`,
	`CREATE TRIGGER trg_specialist_model_call_identity_immutable
		BEFORE UPDATE OF agent_attempt_id, run_id, agent_id, model_attempt_number,
			transport_attempt, max_attempts, provider, model, started_at
		ON specialist_model_calls
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model call identity is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_model_call_terminal_immutable
		BEFORE UPDATE ON specialist_model_calls
		WHEN OLD.status <> 'started'
		BEGIN
			SELECT RAISE(ABORT, 'terminal Specialist model call is immutable');
		END;`,
}

var specialistContextDeliveryStatements = []string{
	`CREATE TABLE specialist_context_deliveries (
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		parent_agent_id TEXT NOT NULL,
		agent_attempt_id TEXT NOT NULL,
		turn_number INTEGER NOT NULL,
		message_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		status TEXT NOT NULL,
		prepared_at TEXT NOT NULL,
		resolved_at TEXT,
		PRIMARY KEY(agent_attempt_id, message_id),
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, parent_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(agent_attempt_id) REFERENCES agent_attempts(id) ON DELETE CASCADE,
		FOREIGN KEY(message_id) REFERENCES agent_messages(id) ON DELETE CASCADE,
		UNIQUE(agent_attempt_id, ordinal),
		CHECK(turn_number > 0),
		CHECK(ordinal BETWEEN 1 AND 4),
		CHECK(status IN ('prepared', 'committed', 'superseded')),
		CHECK((status = 'prepared' AND resolved_at IS NULL)
			OR (status IN ('committed', 'superseded') AND resolved_at IS NOT NULL))
	);`,
	`CREATE UNIQUE INDEX idx_specialist_context_one_prepared_message
		ON specialist_context_deliveries(message_id) WHERE status = 'prepared';`,
	`CREATE UNIQUE INDEX idx_specialist_context_one_committed_message
		ON specialist_context_deliveries(message_id) WHERE status = 'committed';`,
	`CREATE INDEX idx_specialist_context_run_status
		ON specialist_context_deliveries(run_id, status, prepared_at);`,
	`CREATE TRIGGER trg_specialist_context_delivery_insert
		BEFORE INSERT ON specialist_context_deliveries
		WHEN NOT EXISTS (
			SELECT 1 FROM agent_attempts attempt
			JOIN runs run ON run.id = attempt.run_id
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			JOIN agent_nodes child
				ON child.run_id = attempt.run_id AND child.id = attempt.agent_id
			JOIN agent_nodes parent
				ON parent.run_id = attempt.run_id AND parent.id = attempt.parent_agent_id
			JOIN agent_messages message ON message.id = NEW.message_id
			WHERE attempt.id = NEW.agent_attempt_id
				AND attempt.run_id = NEW.run_id AND attempt.agent_id = NEW.agent_id
				AND attempt.parent_agent_id = NEW.parent_agent_id
				AND attempt.turn_number = NEW.turn_number AND attempt.status = 'running'
				AND attempt.usage_recorded_at IS NULL AND run.status = 'running'
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
				AND child.role = 'specialist' AND child.status = 'running'
				AND child.active_attempt_id = attempt.id AND child.parent_id = parent.id
				AND parent.role = 'root' AND parent.status IN ('ready', 'running', 'waiting')
				AND message.run_id = NEW.run_id
				AND message.sender_agent_id = parent.id
				AND message.recipient_agent_id = child.id
				AND message.kind = 'instruction' AND message.semantic = 'message'
				AND message.status = 'pending'
				AND json_valid(message.payload_json)
				AND json_type(message.payload_json) = 'object'
				AND json_extract(message.payload_json, '$.version') = 'specialist_instruction.v1'
				AND json_type(message.payload_json, '$.instruction') = 'text'
				AND length(trim(json_extract(message.payload_json, '$.instruction'))) BETWEEN 1 AND 1200
				AND (SELECT COUNT(*) FROM json_each(message.payload_json)) = 2
				AND NOT EXISTS (
					SELECT 1 FROM json_each(message.payload_json) field
					WHERE field.key NOT IN ('version', 'instruction')
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist context delivery requires an eligible pending parent instruction');
		END;`,
	`CREATE TRIGGER trg_specialist_context_delivery_commit
		BEFORE UPDATE OF status, resolved_at ON specialist_context_deliveries
		WHEN OLD.status = 'prepared' AND NEW.status = 'committed' AND NOT EXISTS (
			SELECT 1 FROM agent_attempts attempt
			JOIN runs run ON run.id = attempt.run_id
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			JOIN agent_nodes child
				ON child.run_id = attempt.run_id AND child.id = attempt.agent_id
			JOIN agent_messages message ON message.id = OLD.message_id
			WHERE attempt.id = OLD.agent_attempt_id
				AND attempt.run_id = OLD.run_id AND attempt.agent_id = OLD.agent_id
				AND attempt.parent_agent_id = OLD.parent_agent_id
				AND attempt.turn_number = OLD.turn_number AND attempt.status = 'running'
				AND attempt.usage_recorded_at IS NOT NULL AND run.status = 'running'
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
				AND child.status = 'running' AND child.active_attempt_id = attempt.id
				AND message.run_id = OLD.run_id
				AND message.sender_agent_id = OLD.parent_agent_id
				AND message.recipient_agent_id = child.id
				AND message.kind = 'instruction' AND message.semantic = 'message'
				AND message.status = 'pending'
				AND json_valid(message.payload_json)
				AND json_type(message.payload_json) = 'object'
				AND json_extract(message.payload_json, '$.version') = 'specialist_instruction.v1'
				AND json_type(message.payload_json, '$.instruction') = 'text'
				AND length(trim(json_extract(message.payload_json, '$.instruction'))) BETWEEN 1 AND 1200
				AND (SELECT COUNT(*) FROM json_each(message.payload_json)) = 2
				AND NOT EXISTS (
					SELECT 1 FROM json_each(message.payload_json) field
					WHERE field.key NOT IN ('version', 'instruction')
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist context commit requires its active usage-recorded attempt');
		END;`,
	`CREATE TRIGGER trg_specialist_context_delivery_active_supersede
		BEFORE UPDATE OF status ON specialist_context_deliveries
		WHEN OLD.status = 'prepared' AND NEW.status = 'superseded' AND EXISTS (
			SELECT 1 FROM agent_attempts attempt
			JOIN runs run ON run.id = attempt.run_id
			JOIN agent_nodes child
				ON child.run_id = attempt.run_id AND child.id = attempt.agent_id
			WHERE attempt.id = OLD.agent_attempt_id AND attempt.run_id = OLD.run_id
				AND attempt.status = 'running' AND run.status = 'running'
				AND child.status = 'running' AND child.active_attempt_id = attempt.id
		)
		BEGIN
			SELECT RAISE(ABORT, 'active Specialist context delivery cannot be superseded');
		END;`,
	`CREATE TRIGGER trg_specialist_context_delivery_identity_immutable
		BEFORE UPDATE OF run_id, agent_id, parent_agent_id, agent_attempt_id, turn_number,
			message_id, ordinal, prepared_at ON specialist_context_deliveries
		BEGIN
			SELECT RAISE(ABORT, 'Specialist context delivery identity is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_context_delivery_terminal_immutable
		BEFORE UPDATE ON specialist_context_deliveries
		WHEN OLD.status <> 'prepared'
		BEGIN
			SELECT RAISE(ABORT, 'terminal Specialist context delivery is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_context_delivery_prepared_delete
		BEFORE DELETE ON specialist_context_deliveries
		WHEN OLD.status = 'prepared'
		BEGIN
			SELECT RAISE(ABORT, 'prepared Specialist context delivery cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_agent_message_prepared_specialist_delivery
		BEFORE UPDATE OF status, consumed_at ON agent_messages
		WHEN OLD.status = 'pending' AND NEW.status = 'consumed' AND EXISTS (
			SELECT 1 FROM specialist_context_deliveries delivery
			WHERE delivery.message_id = OLD.id AND delivery.status = 'prepared'
		)
		BEGIN
			SELECT RAISE(ABORT, 'prepared Specialist instruction must commit through its Agent attempt');
		END;`,
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
