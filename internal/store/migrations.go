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

const LatestSchemaVersion = 39

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

var specialistProtocolRepairStatements = []string{
	`DROP TRIGGER trg_specialist_model_call_sequence;`,
	`DROP TRIGGER trg_specialist_model_call_insert;`,
	`DROP TRIGGER trg_specialist_model_call_terminal_requires_lease;`,
	`DROP TRIGGER trg_specialist_model_call_identity_immutable;`,
	`DROP TRIGGER trg_specialist_model_call_terminal_immutable;`,
	`DROP INDEX idx_specialist_model_one_started;`,
	`DROP INDEX idx_specialist_model_agent_started;`,
	`ALTER TABLE specialist_model_calls RENAME TO specialist_model_calls_v27;`,
	`CREATE TABLE specialist_model_calls (
		agent_attempt_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		model_attempt_number INTEGER NOT NULL,
		transport_attempt INTEGER NOT NULL,
		max_attempts INTEGER NOT NULL,
		protocol_repair INTEGER NOT NULL DEFAULT 0,
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
		CHECK(model_attempt_number > 0 AND transport_attempt > 0),
		CHECK(max_attempts BETWEEN 1 AND 5 AND transport_attempt <= max_attempts),
		CHECK(protocol_repair IN (0, 1)),
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
				AND transport_attempt < max_attempts))
			AND finished_at IS NOT NULL))
	);`,
	`INSERT INTO specialist_model_calls
		(agent_attempt_id, run_id, agent_id, model_attempt_number, transport_attempt,
		max_attempts, protocol_repair, provider, model, input_fingerprint, action_fingerprint,
		status, outcome, error_text, retry_after_millis, retry_planned, elapsed_millis,
		stream_events, stream_bytes, input_tokens, output_tokens, total_tokens,
		usage_recorded, action_kind, report_outcome, policy_allowed, policy_needs_approval,
		policy_risk, policy_reason, user_message_id, assistant_message_id, started_at, finished_at)
	SELECT agent_attempt_id, run_id, agent_id, model_attempt_number, transport_attempt,
		max_attempts, 0, provider, model, input_fingerprint, action_fingerprint,
		status, outcome, error_text, retry_after_millis, retry_planned, elapsed_millis,
		stream_events, stream_bytes, input_tokens, output_tokens, total_tokens,
		usage_recorded, action_kind, report_outcome, policy_allowed, policy_needs_approval,
		policy_risk, policy_reason, user_message_id, assistant_message_id, started_at, finished_at
	FROM specialist_model_calls_v27;`,
	`DROP TABLE specialist_model_calls_v27;`,
	`CREATE TABLE specialist_protocol_repairs (
		agent_attempt_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		status TEXT NOT NULL,
		reason TEXT NOT NULL,
		requested_model_attempt INTEGER NOT NULL,
		resolved_model_attempt INTEGER,
		requested_at TEXT NOT NULL,
		resolved_at TEXT,
		FOREIGN KEY(agent_attempt_id) REFERENCES agent_attempts(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		FOREIGN KEY(agent_attempt_id, requested_model_attempt)
			REFERENCES specialist_model_calls(agent_attempt_id, model_attempt_number),
		FOREIGN KEY(agent_attempt_id, resolved_model_attempt)
			REFERENCES specialist_model_calls(agent_attempt_id, model_attempt_number),
		CHECK(status IN ('pending', 'completed', 'exhausted', 'aborted')),
		CHECK(length(trim(reason)) BETWEEN 1 AND 1024),
		CHECK(length(CAST(reason AS BLOB)) <= 4096),
		CHECK(requested_model_attempt > 0),
		CHECK((status = 'pending' AND resolved_model_attempt IS NULL AND resolved_at IS NULL)
			OR (status IN ('completed', 'exhausted')
				AND resolved_model_attempt > requested_model_attempt AND resolved_at IS NOT NULL)
			OR (status = 'aborted' AND resolved_model_attempt IS NULL AND resolved_at IS NOT NULL))
	);`,
	`CREATE UNIQUE INDEX idx_specialist_model_one_started
		ON specialist_model_calls(agent_attempt_id) WHERE status = 'started';`,
	`CREATE INDEX idx_specialist_model_agent_started
		ON specialist_model_calls(agent_id, started_at, model_attempt_number);`,
	`CREATE INDEX idx_specialist_repair_run_status
		ON specialist_protocol_repairs(run_id, status, requested_at);`,
	`CREATE TRIGGER trg_specialist_model_call_sequence
		BEFORE INSERT ON specialist_model_calls
		WHEN NEW.model_attempt_number <> (
			SELECT COUNT(*) + 1 FROM specialist_model_calls
			WHERE agent_attempt_id = NEW.agent_attempt_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model call is not the next attempt');
		END;`,
	`CREATE TRIGGER trg_specialist_model_call_phase_sequence
		BEFORE INSERT ON specialist_model_calls
		WHEN NEW.transport_attempt <> (
			SELECT COUNT(*) + 1 FROM specialist_model_calls
			WHERE agent_attempt_id = NEW.agent_attempt_id
				AND protocol_repair = NEW.protocol_repair
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model call is not the next phase transport attempt');
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
				AND child.role = 'specialist' AND child.status = 'running'
				AND child.active_attempt_id = attempt.id AND run.status = 'running'
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
				AND ((NEW.protocol_repair = 0 AND attempt.usage_recorded_at IS NULL
					AND NOT EXISTS (SELECT 1 FROM specialist_protocol_repairs repair
						WHERE repair.agent_attempt_id = attempt.id))
					OR (NEW.protocol_repair = 1 AND attempt.usage_recorded_at IS NOT NULL
						AND EXISTS (SELECT 1 FROM specialist_protocol_repairs repair
							WHERE repair.agent_attempt_id = attempt.id AND repair.status = 'pending')))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model call requires its active leased protocol phase');
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
				AND (NEW.usage_recorded = 0 OR (
					attempt.usage_recorded_at IS NOT NULL
					AND attempt.input_tokens = NEW.input_tokens + COALESCE((
						SELECT SUM(call.input_tokens) FROM specialist_model_calls call
						WHERE call.agent_attempt_id = OLD.agent_attempt_id
							AND call.model_attempt_number <> OLD.model_attempt_number
							AND call.status <> 'started' AND call.usage_recorded = 1), 0)
					AND attempt.output_tokens = NEW.output_tokens + COALESCE((
						SELECT SUM(call.output_tokens) FROM specialist_model_calls call
						WHERE call.agent_attempt_id = OLD.agent_attempt_id
							AND call.model_attempt_number <> OLD.model_attempt_number
							AND call.status <> 'started' AND call.usage_recorded = 1), 0)
					AND attempt.total_tokens = NEW.total_tokens + COALESCE((
						SELECT SUM(call.total_tokens) FROM specialist_model_calls call
						WHERE call.agent_attempt_id = OLD.agent_attempt_id
							AND call.model_attempt_number <> OLD.model_attempt_number
							AND call.status <> 'started' AND call.usage_recorded = 1), 0)
					AND attempt.execution_millis = NEW.elapsed_millis + COALESCE((
						SELECT SUM(call.elapsed_millis) FROM specialist_model_calls call
						WHERE call.agent_attempt_id = OLD.agent_attempt_id
							AND call.model_attempt_number <> OLD.model_attempt_number
							AND call.status <> 'started'), 0)))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model terminal requires its active leased cumulative usage');
		END;`,
	`CREATE TRIGGER trg_specialist_model_call_identity_immutable
		BEFORE UPDATE OF agent_attempt_id, run_id, agent_id, model_attempt_number,
			transport_attempt, max_attempts, protocol_repair, provider, model, started_at
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
	`CREATE TRIGGER trg_specialist_repair_insert
		BEFORE INSERT ON specialist_protocol_repairs
		WHEN NOT EXISTS (
			SELECT 1 FROM agent_attempts attempt
			JOIN specialist_model_calls call
				ON call.agent_attempt_id = attempt.id
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			WHERE attempt.id = NEW.agent_attempt_id AND attempt.run_id = NEW.run_id
				AND attempt.agent_id = NEW.agent_id AND attempt.status = 'running'
				AND attempt.usage_recorded_at IS NOT NULL
				AND call.model_attempt_number = NEW.requested_model_attempt
				AND call.protocol_repair = 0 AND call.status = 'failed'
				AND call.outcome = 'invalid_response' AND call.usage_recorded = 1
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist repair requires a charged primary protocol failure');
		END;`,
	`CREATE TRIGGER trg_specialist_repair_resolve
		BEFORE UPDATE OF status, resolved_model_attempt, resolved_at
		ON specialist_protocol_repairs
		WHEN OLD.status <> 'pending' OR NOT (
			(NEW.status = 'completed' AND NEW.resolved_at IS NOT NULL AND EXISTS (
				SELECT 1 FROM specialist_model_calls call
				WHERE call.agent_attempt_id = OLD.agent_attempt_id
					AND call.model_attempt_number = NEW.resolved_model_attempt
					AND call.protocol_repair = 1 AND call.status = 'completed'
					AND call.outcome = 'success' AND call.usage_recorded = 1))
			OR (NEW.status = 'exhausted' AND NEW.resolved_at IS NOT NULL AND EXISTS (
				SELECT 1 FROM specialist_model_calls call
				WHERE call.agent_attempt_id = OLD.agent_attempt_id
					AND call.model_attempt_number = NEW.resolved_model_attempt
					AND call.protocol_repair = 1 AND call.status = 'failed'
					AND call.outcome = 'invalid_response' AND call.usage_recorded = 1))
			OR (NEW.status = 'aborted' AND NEW.resolved_model_attempt IS NULL
				AND NEW.resolved_at IS NOT NULL AND EXISTS (
					SELECT 1 FROM agent_attempts attempt
					JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
					WHERE attempt.id = OLD.agent_attempt_id AND attempt.status = 'running'
						AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
						AND ((lease.lease_id = attempt.lease_id
							AND lease.generation = attempt.lease_generation)
							OR lease.generation > attempt.lease_generation)))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist repair resolution is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_repair_identity_immutable
		BEFORE UPDATE OF agent_attempt_id, run_id, agent_id, reason,
			requested_model_attempt, requested_at ON specialist_protocol_repairs
		BEGIN
			SELECT RAISE(ABORT, 'Specialist repair identity is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_repair_terminal_immutable
		BEFORE UPDATE ON specialist_protocol_repairs
		WHEN OLD.status <> 'pending'
		BEGIN
			SELECT RAISE(ABORT, 'terminal Specialist repair is immutable');
		END;`,
	`DROP TRIGGER trg_agent_attempt_usage_immutable;`,
	`DROP TRIGGER trg_agent_attempt_usage_requires_lease;`,
	`CREATE TRIGGER trg_agent_attempt_usage_monotonic
		BEFORE UPDATE OF input_tokens, output_tokens, total_tokens, execution_millis,
			usage_recorded_at ON agent_attempts
		WHEN OLD.usage_recorded_at IS NOT NULL AND (
			NEW.usage_recorded_at IS NULL OR NEW.usage_recorded_at <> OLD.usage_recorded_at
			OR NEW.input_tokens < OLD.input_tokens OR NEW.output_tokens < OLD.output_tokens
			OR NEW.total_tokens < OLD.total_tokens OR NEW.execution_millis < OLD.execution_millis)
		BEGIN
			SELECT RAISE(ABORT, 'Agent attempt usage must grow monotonically');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_usage_requires_lease
		BEFORE UPDATE OF input_tokens, output_tokens, total_tokens, execution_millis,
			usage_recorded_at ON agent_attempts
		WHEN NOT EXISTS (
			SELECT 1 FROM run_execution_leases lease
			WHERE lease.run_id = OLD.run_id AND lease.lease_id = OLD.lease_id
				AND lease.generation = OLD.lease_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
		)
		BEGIN
			SELECT RAISE(ABORT, 'Agent attempt usage requires its active Run lease');
		END;`,
	`CREATE TRIGGER trg_agent_attempt_repair_resolution
		BEFORE UPDATE OF status ON agent_attempts
		WHEN OLD.status = 'running' AND NEW.status <> 'running' AND (
			(NEW.status IN ('continued', 'finished') AND EXISTS (
				SELECT 1 FROM specialist_protocol_repairs repair
				WHERE repair.agent_attempt_id = OLD.id AND repair.status <> 'completed'))
			OR (NEW.status IN ('crashed', 'interrupted') AND EXISTS (
				SELECT 1 FROM specialist_protocol_repairs repair
				WHERE repair.agent_attempt_id = OLD.id AND repair.status = 'pending')))
		BEGIN
			SELECT RAISE(ABORT, 'Agent attempt requires a resolved Specialist repair');
		END;`,
}

var specialistScheduleControlStatements = []string{
	`CREATE TABLE specialist_schedules (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		lease_id TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		max_rounds INTEGER NOT NULL,
		status TEXT NOT NULL,
		stop_reason TEXT NOT NULL DEFAULT '',
		rounds_completed INTEGER NOT NULL DEFAULT 0,
		turns_started INTEGER NOT NULL DEFAULT 0,
		recovered_attempts INTEGER NOT NULL DEFAULT 0,
		before_root_tokens INTEGER NOT NULL,
		before_specialist_tokens INTEGER NOT NULL,
		before_total_tokens INTEGER NOT NULL,
		before_root_execution_millis INTEGER NOT NULL,
		before_specialist_execution_millis INTEGER NOT NULL,
		before_total_execution_millis INTEGER NOT NULL,
		after_root_tokens INTEGER NOT NULL,
		after_specialist_tokens INTEGER NOT NULL,
		after_total_tokens INTEGER NOT NULL,
		after_root_execution_millis INTEGER NOT NULL,
		after_specialist_execution_millis INTEGER NOT NULL,
		after_total_execution_millis INTEGER NOT NULL,
		error_code TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		finished_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		CHECK(lease_generation > 0 AND max_rounds BETWEEN 1 AND 32),
		CHECK(status IN ('running', 'completed', 'failed', 'cancelled', 'abandoned')),
		CHECK(rounds_completed BETWEEN 0 AND max_rounds
			AND turns_started >= 0 AND recovered_attempts >= 0),
		CHECK(before_root_tokens >= 0 AND before_specialist_tokens >= 0
			AND before_total_tokens = before_root_tokens + before_specialist_tokens),
		CHECK(before_root_execution_millis >= 0 AND before_specialist_execution_millis >= 0
			AND before_total_execution_millis = before_root_execution_millis + before_specialist_execution_millis),
		CHECK(after_root_tokens >= 0 AND after_specialist_tokens >= 0
			AND after_total_tokens = after_root_tokens + after_specialist_tokens),
		CHECK(after_root_execution_millis >= 0 AND after_specialist_execution_millis >= 0
			AND after_total_execution_millis = after_root_execution_millis + after_specialist_execution_millis),
		CHECK(length(stop_reason) <= 64 AND length(error_code) <= 64),
		CHECK((status = 'running' AND stop_reason = '' AND error_code = ''
			AND rounds_completed = 0 AND turns_started = 0 AND recovered_attempts = 0
			AND before_root_tokens = after_root_tokens
			AND before_specialist_tokens = after_specialist_tokens
			AND before_total_tokens = after_total_tokens
			AND before_root_execution_millis = after_root_execution_millis
			AND before_specialist_execution_millis = after_specialist_execution_millis
			AND before_total_execution_millis = after_total_execution_millis
			AND finished_at IS NULL)
			OR (status <> 'running' AND length(trim(stop_reason)) BETWEEN 1 AND 64
				AND finished_at IS NOT NULL))
	);`,
	`CREATE TABLE specialist_schedule_agents (
		schedule_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		PRIMARY KEY(schedule_id, ordinal),
		UNIQUE(schedule_id, agent_id),
		FOREIGN KEY(schedule_id) REFERENCES specialist_schedules(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		CHECK(ordinal BETWEEN 1 AND 2)
	);`,
	`CREATE UNIQUE INDEX idx_specialist_schedule_one_running
		ON specialist_schedules(run_id) WHERE status = 'running';`,
	`CREATE INDEX idx_specialist_schedule_run_started
		ON specialist_schedules(run_id, started_at, id);`,
	`CREATE TRIGGER trg_specialist_schedule_insert
		BEFORE INSERT ON specialist_schedules
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN run_execution_leases lease ON lease.run_id = run.id
			WHERE run.id = NEW.run_id AND run.status = 'running'
				AND lease.lease_id = NEW.lease_id
				AND lease.generation = NEW.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist schedule requires its active Run lease');
		END;`,
	`CREATE TRIGGER trg_specialist_schedule_agent_insert
		BEFORE INSERT ON specialist_schedule_agents
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_schedules schedule
			JOIN agent_nodes child ON child.run_id = schedule.run_id
				AND child.id = NEW.agent_id
			JOIN agent_nodes parent ON parent.run_id = child.run_id
				AND parent.id = child.parent_id
			WHERE schedule.id = NEW.schedule_id AND schedule.run_id = NEW.run_id
				AND schedule.status = 'running' AND child.role = 'specialist'
				AND parent.role = 'root' AND parent.parent_id IS NULL
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist schedule target must be a direct child');
		END;`,
	`CREATE TRIGGER trg_specialist_schedule_terminal
		BEFORE UPDATE OF status, stop_reason, rounds_completed, turns_started,
			recovered_attempts, after_root_tokens, after_specialist_tokens,
			after_total_tokens, after_root_execution_millis,
			after_specialist_execution_millis, after_total_execution_millis,
			error_code, finished_at ON specialist_schedules
		WHEN OLD.status <> 'running' OR NEW.status = 'running' OR NOT EXISTS (
			SELECT 1 FROM run_execution_leases lease
			WHERE lease.run_id = OLD.run_id AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND ((lease.lease_id = OLD.lease_id
					AND lease.generation = OLD.lease_generation)
					OR (NEW.status = 'abandoned'
						AND lease.generation > OLD.lease_generation))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist schedule terminal transition is not fenced');
		END;`,
	`CREATE TRIGGER trg_specialist_schedule_identity_immutable
		BEFORE UPDATE OF id, run_id, lease_id, lease_generation, max_rounds,
			before_root_tokens, before_specialist_tokens, before_total_tokens,
			before_root_execution_millis, before_specialist_execution_millis,
			before_total_execution_millis, started_at ON specialist_schedules
		BEGIN
			SELECT RAISE(ABORT, 'Specialist schedule identity is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_schedule_terminal_immutable
		BEFORE UPDATE ON specialist_schedules
		WHEN OLD.status <> 'running'
		BEGIN
			SELECT RAISE(ABORT, 'terminal Specialist schedule is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_schedule_agent_immutable
		BEFORE UPDATE ON specialist_schedule_agents
		BEGIN
			SELECT RAISE(ABORT, 'Specialist schedule target is immutable');
		END;`,
	`CREATE TABLE specialist_model_cancellations (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		agent_attempt_id TEXT NOT NULL,
		model_attempt INTEGER NOT NULL,
		status TEXT NOT NULL,
		reason TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		requested_at TEXT NOT NULL,
		observed_at TEXT,
		resolved_at TEXT,
		resolution TEXT NOT NULL DEFAULT '',
		FOREIGN KEY(agent_attempt_id, model_attempt)
			REFERENCES specialist_model_calls(agent_attempt_id, model_attempt_number) ON DELETE CASCADE,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		UNIQUE(run_id, agent_id, agent_attempt_id, model_attempt),
		CHECK(model_attempt > 0),
		CHECK(status IN ('pending', 'observed', 'resolved')),
		CHECK(length(trim(reason)) BETWEEN 1 AND 1024),
		CHECK(length(CAST(reason AS BLOB)) <= 4096),
		CHECK(length(trim(requested_by)) BETWEEN 1 AND 256),
		CHECK((status = 'pending' AND observed_at IS NULL AND resolved_at IS NULL AND resolution = '')
			OR (status = 'observed' AND observed_at IS NOT NULL
				AND resolved_at IS NULL AND resolution = '')
			OR (status = 'resolved' AND resolved_at IS NOT NULL
				AND length(trim(resolution)) BETWEEN 1 AND 64))
	);`,
	`CREATE INDEX idx_specialist_model_cancellations_pending
		ON specialist_model_cancellations(run_id, agent_id, agent_attempt_id,
			model_attempt, status);`,
	`CREATE TABLE specialist_model_cancellation_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		cancellation_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		FOREIGN KEY(cancellation_id) REFERENCES specialist_model_cancellations(id) ON DELETE CASCADE
	);`,
	`CREATE TRIGGER trg_specialist_model_cancellation_operation_immutable
		BEFORE UPDATE ON specialist_model_cancellation_operations
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model cancellation operation is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_model_cancellation_insert
		BEFORE INSERT ON specialist_model_cancellations
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_model_calls call
			JOIN agent_attempts attempt ON attempt.id = call.agent_attempt_id
			JOIN agent_nodes child ON child.run_id = attempt.run_id
				AND child.id = attempt.agent_id
			JOIN runs run ON run.id = attempt.run_id
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			WHERE call.agent_attempt_id = NEW.agent_attempt_id
				AND call.model_attempt_number = NEW.model_attempt
				AND call.run_id = NEW.run_id AND call.agent_id = NEW.agent_id
				AND call.status = 'started' AND attempt.run_id = NEW.run_id
				AND attempt.agent_id = NEW.agent_id AND attempt.status = 'running'
				AND child.role = 'specialist' AND child.status = 'running'
				AND child.active_attempt_id = attempt.id AND run.status = 'running'
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
				AND NOT EXISTS (SELECT 1 FROM specialist_model_calls later
					WHERE later.agent_attempt_id = call.agent_attempt_id
						AND later.model_attempt_number > call.model_attempt_number)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model cancellation requires the latest active call');
		END;`,
	`CREATE TRIGGER trg_specialist_model_cancellation_transition
		BEFORE UPDATE OF status, observed_at, resolved_at, resolution
		ON specialist_model_cancellations
		WHEN NOT ((OLD.status = 'pending' AND NEW.status IN ('observed', 'resolved'))
			OR (OLD.status = 'observed' AND NEW.status = 'resolved'))
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model cancellation transition is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_model_cancellation_observe
		BEFORE UPDATE OF status ON specialist_model_cancellations
		WHEN NEW.status = 'observed' AND NOT EXISTS (
			SELECT 1 FROM specialist_model_calls call
			JOIN agent_attempts attempt ON attempt.id = call.agent_attempt_id
			JOIN agent_nodes child ON child.run_id = attempt.run_id AND child.id = attempt.agent_id
			JOIN runs run ON run.id = attempt.run_id
			JOIN run_execution_leases lease ON lease.run_id = attempt.run_id
			WHERE call.agent_attempt_id = OLD.agent_attempt_id
				AND call.model_attempt_number = OLD.model_attempt
				AND call.status = 'started' AND attempt.status = 'running'
				AND child.status = 'running' AND child.active_attempt_id = attempt.id
				AND run.status = 'running'
				AND lease.lease_id = attempt.lease_id
				AND lease.generation = attempt.lease_generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model cancellation observation is not fenced');
		END;`,
	`CREATE TRIGGER trg_specialist_model_cancellation_resolve
		BEFORE UPDATE OF status ON specialist_model_cancellations
		WHEN NEW.status = 'resolved' AND NOT (
			EXISTS (SELECT 1 FROM specialist_model_calls call
				WHERE call.agent_attempt_id = OLD.agent_attempt_id
					AND call.model_attempt_number = OLD.model_attempt
					AND call.status <> 'started' AND call.outcome = NEW.resolution)
			OR (NEW.resolution IN ('attempt_terminated', 'worker_lost', 'superseded')
				AND EXISTS (SELECT 1 FROM agent_attempts attempt
					WHERE attempt.id = OLD.agent_attempt_id AND attempt.status <> 'running'))
			OR (NEW.resolution = 'superseded' AND EXISTS (
				SELECT 1 FROM specialist_model_calls later
				WHERE later.agent_attempt_id = OLD.agent_attempt_id
					AND later.model_attempt_number > OLD.model_attempt))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model cancellation resolution is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_model_cancellation_identity_immutable
		BEFORE UPDATE OF id, run_id, agent_id, agent_attempt_id, model_attempt,
			reason, requested_by, requested_at ON specialist_model_cancellations
		BEGIN
			SELECT RAISE(ABORT, 'Specialist model cancellation identity is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_model_cancellation_terminal_immutable
		BEFORE UPDATE ON specialist_model_cancellations
		WHEN OLD.status = 'resolved'
		BEGIN
			SELECT RAISE(ABORT, 'resolved Specialist model cancellation is immutable');
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

var specialistDelegationProposalStatements = []string{
	`DROP TRIGGER trg_supervisor_tool_call_model_attempt;`,
	`DROP TRIGGER trg_supervisor_tool_round_completion;`,
	`DROP INDEX idx_run_supervisor_tool_calls_pending;`,
	`ALTER TABLE run_supervisor_tool_calls RENAME TO run_supervisor_tool_calls_v29;`,
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
		CHECK(tool_name IN ('work_item_create', 'note_create', 'specialist_delegation_propose')),
		CHECK(status IN ('pending', 'completed', 'denied', 'failed')),
		CHECK((status = 'pending' AND result_json = '' AND error_code = '' AND completed_at IS NULL)
			OR (status = 'completed' AND length(result_json) > 0 AND error_code = '' AND completed_at IS NOT NULL)
			OR (status IN ('denied', 'failed') AND length(result_json) > 0 AND length(error_code) > 0
				AND completed_at IS NOT NULL))
	);`,
	`INSERT INTO run_supervisor_tool_calls
		(run_id, turn, attempt_id, round, position, model_attempt, call_id, tool_name,
		payload_json, status, result_json, error_code, created_at, completed_at)
		SELECT run_id, turn, attempt_id, round, position, model_attempt, call_id, tool_name,
		payload_json, status, result_json, error_code, created_at, completed_at
		FROM run_supervisor_tool_calls_v29;`,
	`DROP TABLE run_supervisor_tool_calls_v29;`,
	`CREATE INDEX idx_run_supervisor_tool_calls_pending
		ON run_supervisor_tool_calls(run_id, turn, attempt_id, status, round, position);`,
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
	`CREATE TABLE specialist_delegation_proposals (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		root_agent_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT '',
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		assignment_count INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, root_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		CHECK(protocol_version = 'specialist_delegation.v1'),
		CHECK(status = 'proposed'),
		CHECK(assignment_count BETWEEN 1 AND 2),
		CHECK(length(trim(requested_by)) BETWEEN 1 AND 256),
		CHECK(version = 1)
	);`,
	`CREATE INDEX idx_specialist_delegation_proposals_run_created
		ON specialist_delegation_proposals(run_id, created_at, id);`,
	`CREATE TABLE specialist_delegation_assignments (
		proposal_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		title TEXT NOT NULL,
		goal TEXT NOT NULL,
		skills_json TEXT NOT NULL,
		turn_limit INTEGER NOT NULL,
		token_limit INTEGER NOT NULL,
		PRIMARY KEY(proposal_id, ordinal),
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE CASCADE,
		CHECK(ordinal BETWEEN 1 AND 2),
		CHECK(title = trim(title) AND length(title) BETWEEN 1 AND 256),
		CHECK(length(CAST(title AS BLOB)) <= 1024),
		CHECK(goal = trim(goal) AND length(goal) BETWEEN 1 AND 1200),
		CHECK(length(CAST(goal AS BLOB)) <= 4800),
		CHECK(json_valid(skills_json) AND json_type(skills_json) = 'array'),
		CHECK(json_array_length(skills_json) BETWEEN 1 AND 16),
		CHECK(turn_limit > 0),
		CHECK(token_limit > 0)
	);`,
	`CREATE TABLE specialist_delegation_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		invocation_id TEXT NOT NULL UNIQUE,
		proposal_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT '',
		root_agent_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(invocation_id) REFERENCES run_tool_calls(id) ON DELETE RESTRICT,
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE CASCADE,
		FOREIGN KEY(run_id, root_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE CASCADE,
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = 'run_supervisor')
	);`,
	`CREATE TRIGGER trg_specialist_delegation_proposal_insert
		BEFORE INSERT ON specialist_delegation_proposals
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN agent_nodes root ON root.run_id = run.id AND root.id = NEW.root_agent_id
			WHERE run.id = NEW.run_id AND run.status = 'running'
				AND run.session_id = NEW.session_id
				AND COALESCE(mission.workspace_id, '') = NEW.workspace_id
				AND root.role = 'root' AND root.parent_id IS NULL AND root.depth = 0
				AND root.status = 'running' AND root.active_attempt_id <> ''
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation proposal requires the active root Agent');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_assignment_insert
		BEFORE INSERT ON specialist_delegation_assignments
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_proposals proposal
			JOIN agent_nodes root ON root.run_id = proposal.run_id AND root.id = proposal.root_agent_id
			WHERE proposal.id = NEW.proposal_id AND NEW.ordinal <= proposal.assignment_count
				AND NOT EXISTS (
					SELECT 1 FROM json_each(NEW.skills_json) requested
					WHERE requested.type <> 'text'
						OR requested.value <> lower(trim(requested.value))
						OR length(requested.value) NOT BETWEEN 1 AND 96
						OR requested.value GLOB '*[^a-z0-9._-]*'
						OR requested.value = 'specialist_delegation_propose'
						OR NOT EXISTS (
							SELECT 1 FROM json_each(root.skills_json) allowed
							WHERE allowed.type = 'text' AND allowed.value = requested.value
						)
						OR EXISTS (
							SELECT 1 FROM json_each(NEW.skills_json) later
							WHERE CAST(later.key AS INTEGER) > CAST(requested.key AS INTEGER)
								AND requested.value >= later.value
						)
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation assignment exceeds root capability');
		END;`,
	`CREATE TRIGGER trg_specialist_non_delegable_capability
		BEFORE INSERT ON agent_nodes
		WHEN NEW.role = 'specialist' AND EXISTS (
			SELECT 1 FROM json_each(NEW.skills_json) skill
			WHERE skill.type = 'text' AND skill.value = 'specialist_delegation_propose'
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist includes a non-delegable control capability');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_operation_insert
		BEFORE INSERT ON specialist_delegation_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_proposals proposal
			JOIN runs run ON run.id = proposal.run_id
			JOIN agent_nodes root ON root.run_id = proposal.run_id AND root.id = proposal.root_agent_id
			JOIN run_supervisor_checkpoints checkpoint ON checkpoint.run_id = proposal.run_id
			JOIN run_execution_leases lease ON lease.run_id = proposal.run_id
			JOIN run_tool_calls invocation ON invocation.id = NEW.invocation_id
			WHERE proposal.id = NEW.proposal_id AND proposal.run_id = NEW.run_id
				AND proposal.root_agent_id = NEW.root_agent_id
				AND proposal.session_id = NEW.session_id
				AND proposal.workspace_id = NEW.workspace_id
				AND proposal.requested_by = NEW.requested_by
				AND run.status = 'running' AND root.status = 'running'
				AND EXISTS (SELECT 1 FROM json_each(root.skills_json) root_skill
					WHERE root_skill.type = 'text'
						AND root_skill.value = 'specialist_delegation_propose')
				AND root.active_attempt_id = checkpoint.attempt_id
				AND checkpoint.phase = 'turn_started'
				AND checkpoint.lease_id = lease.lease_id
				AND checkpoint.lease_generation = lease.generation
				AND lease.status = 'active' AND julianday(lease.expires_at) > julianday('now')
				AND invocation.run_id = NEW.run_id AND invocation.session_id = NEW.session_id
				AND invocation.workspace_id = NEW.workspace_id
				AND invocation.tool_name = 'specialist_delegation_propose'
				AND invocation.action_class = 'agent_proposal'
				AND (SELECT COUNT(*) FROM specialist_delegation_assignments assignment
					WHERE assignment.proposal_id = proposal.id) = proposal.assignment_count
				AND (SELECT COUNT(*) FROM agent_nodes child
					WHERE child.run_id = proposal.run_id AND child.parent_id = root.id)
					+ proposal.assignment_count <= 2
				AND COALESCE((SELECT SUM(assignment.turn_limit)
					FROM specialist_delegation_assignments assignment
					WHERE assignment.proposal_id = proposal.id), 0)
					<= root.turn_limit - root.turns_used - 2
				AND (root.token_limit = 0 OR COALESCE((SELECT SUM(assignment.token_limit)
					FROM specialist_delegation_assignments assignment
					WHERE assignment.proposal_id = proposal.id), 0)
					< root.token_limit - root.tokens_used)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation operation is not authorized or exceeds capacity');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_proposal_immutable
		BEFORE UPDATE ON specialist_delegation_proposals
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation proposal is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_assignment_immutable
		BEFORE UPDATE ON specialist_delegation_assignments
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation assignment is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_operation_immutable
		BEFORE UPDATE ON specialist_delegation_operations
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation operation is immutable');
		END;`,
}

var specialistDelegationReviewStatements = []string{
	`CREATE TABLE specialist_delegation_reviews (
		id TEXT PRIMARY KEY,
		proposal_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		root_agent_id TEXT NOT NULL,
		decision TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		reviewed_by TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id, root_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE RESTRICT,
		CHECK(decision IN ('approved', 'rejected')),
		CHECK(reason = trim(reason) AND instr(reason, char(0)) = 0
			AND length(reason) <= 2048 AND length(CAST(reason AS BLOB)) <= 8192),
		CHECK(decision = 'approved' OR length(reason) > 0),
		CHECK(reviewed_by = trim(reviewed_by) AND length(reviewed_by) BETWEEN 1 AND 256
			AND instr(reviewed_by, char(0)) = 0),
		CHECK(version = 1)
	);`,
	`CREATE INDEX idx_specialist_delegation_reviews_run_created
		ON specialist_delegation_reviews(run_id, created_at, id);`,
	`CREATE TABLE specialist_delegation_review_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		review_id TEXT NOT NULL UNIQUE,
		proposal_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		reviewed_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(review_id) REFERENCES specialist_delegation_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(reviewed_by = trim(reviewed_by) AND length(reviewed_by) BETWEEN 1 AND 256
			AND instr(reviewed_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER trg_specialist_delegation_review_insert
		BEFORE INSERT ON specialist_delegation_reviews
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_proposals proposal
			JOIN runs run ON run.id = proposal.run_id
			JOIN agent_nodes root ON root.run_id = proposal.run_id
				AND root.id = proposal.root_agent_id
			WHERE proposal.id = NEW.proposal_id AND proposal.run_id = NEW.run_id
				AND proposal.root_agent_id = NEW.root_agent_id
				AND proposal.status = 'proposed'
				AND julianday(NEW.created_at) >= julianday(proposal.created_at)
				AND root.role = 'root' AND root.parent_id IS NULL
				AND (NEW.decision = 'rejected' OR run.status = 'running')
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation review binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_review_operation_insert
		BEFORE INSERT ON specialist_delegation_review_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_reviews review
			WHERE review.id = NEW.review_id AND review.proposal_id = NEW.proposal_id
				AND review.run_id = NEW.run_id AND review.reviewed_by = NEW.reviewed_by
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation review operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_review_immutable
		BEFORE UPDATE ON specialist_delegation_reviews
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation review is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_review_delete_immutable
		BEFORE DELETE ON specialist_delegation_reviews
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation review cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_review_operation_immutable
		BEFORE UPDATE ON specialist_delegation_review_operations
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation review operation is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_review_operation_delete_immutable
		BEFORE DELETE ON specialist_delegation_review_operations
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation review operation cannot be deleted');
		END;`,
}

var specialistDelegationApplicationStatements = []string{
	`CREATE TABLE specialist_delegation_applications (
		id TEXT PRIMARY KEY,
		review_id TEXT NOT NULL UNIQUE,
		proposal_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		root_agent_id TEXT NOT NULL,
		status TEXT NOT NULL,
		assignment_count INTEGER NOT NULL,
		policy_fingerprint TEXT NOT NULL,
		max_children INTEGER NOT NULL,
		max_turns_per_child INTEGER NOT NULL,
		max_tokens_per_child INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		stop_code TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		completed_at TEXT,
		FOREIGN KEY(review_id) REFERENCES specialist_delegation_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id, root_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE RESTRICT,
		CHECK(status IN ('applying', 'applied', 'aborted')),
		CHECK(assignment_count BETWEEN 1 AND 2),
		CHECK(length(policy_fingerprint) = 64 AND policy_fingerprint = lower(policy_fingerprint)
			AND policy_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(max_children BETWEEN 1 AND 2),
		CHECK(max_turns_per_child > 0),
		CHECK(max_tokens_per_child > 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0),
		CHECK((status = 'applying' AND stop_code = '' AND version = 1 AND completed_at IS NULL)
			OR (status = 'applied' AND stop_code = '' AND version = 2 AND completed_at IS NOT NULL)
			OR (status = 'aborted' AND length(stop_code) > 0 AND version = 2 AND completed_at IS NOT NULL)),
		CHECK(julianday(updated_at) >= julianday(created_at)),
		CHECK(completed_at IS NULL OR julianday(completed_at) = julianday(updated_at))
	);`,
	`CREATE UNIQUE INDEX idx_specialist_delegation_applications_running
		ON specialist_delegation_applications(run_id) WHERE status = 'applying';`,
	`CREATE INDEX idx_specialist_delegation_applications_run_created
		ON specialist_delegation_applications(run_id, created_at, id);`,
	`CREATE TABLE specialist_delegation_application_assignments (
		application_id TEXT NOT NULL,
		proposal_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		status TEXT NOT NULL,
		admission_operation_digest TEXT NOT NULL UNIQUE,
		instruction_operation_digest TEXT NOT NULL UNIQUE,
		agent_id TEXT UNIQUE,
		message_id TEXT UNIQUE,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY(application_id, ordinal),
		FOREIGN KEY(application_id) REFERENCES specialist_delegation_applications(id) ON DELETE RESTRICT,
		FOREIGN KEY(proposal_id, ordinal) REFERENCES specialist_delegation_assignments(proposal_id, ordinal) ON DELETE RESTRICT,
		FOREIGN KEY(agent_id) REFERENCES agent_nodes(id) ON DELETE RESTRICT,
		FOREIGN KEY(message_id) REFERENCES agent_messages(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 2),
		CHECK(status IN ('pending', 'admitted', 'instructed')),
		CHECK(length(admission_operation_digest) = 64
			AND admission_operation_digest = lower(admission_operation_digest)
			AND admission_operation_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(instruction_operation_digest) = 64
			AND instruction_operation_digest = lower(instruction_operation_digest)
			AND instruction_operation_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK((status = 'pending' AND agent_id IS NULL AND message_id IS NULL AND version = 1)
			OR (status = 'admitted' AND agent_id IS NOT NULL AND message_id IS NULL AND version = 2)
			OR (status = 'instructed' AND agent_id IS NOT NULL AND message_id IS NOT NULL AND version = 3)),
		CHECK(julianday(updated_at) >= julianday(created_at))
	);`,
	`CREATE TABLE specialist_delegation_application_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		application_id TEXT NOT NULL UNIQUE,
		review_id TEXT NOT NULL UNIQUE,
		proposal_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(application_id) REFERENCES specialist_delegation_applications(id) ON DELETE RESTRICT,
		FOREIGN KEY(review_id) REFERENCES specialist_delegation_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER trg_specialist_delegation_application_insert
		BEFORE INSERT ON specialist_delegation_applications
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_reviews review
			JOIN specialist_delegation_review_operations review_operation
				ON review_operation.review_id = review.id AND review_operation.proposal_id = review.proposal_id
			JOIN specialist_delegation_proposals proposal ON proposal.id = review.proposal_id
			JOIN runs run ON run.id = proposal.run_id
			JOIN agent_nodes root ON root.run_id = proposal.run_id AND root.id = proposal.root_agent_id
			JOIN sessions root_session ON root_session.id = root.session_id
			WHERE review.id = NEW.review_id AND review.proposal_id = NEW.proposal_id
				AND review.run_id = NEW.run_id AND review.root_agent_id = NEW.root_agent_id
				AND review.decision = 'approved' AND review.reviewed_by = NEW.requested_by
				AND proposal.run_id = NEW.run_id AND proposal.root_agent_id = NEW.root_agent_id
				AND proposal.status = 'proposed' AND proposal.assignment_count = NEW.assignment_count
				AND run.status = 'running' AND root.role = 'root' AND root.parent_id IS NULL
				AND root.status = 'ready' AND root.active_attempt_id = ''
				AND root_session.status = 'active'
				AND root.child_limit IN (0, NEW.max_children)
				AND julianday(NEW.created_at) >= julianday(review.created_at)
				AND (SELECT COUNT(*) FROM agent_nodes child
					WHERE child.run_id = NEW.run_id AND child.parent_id = NEW.root_agent_id)
					+ NEW.assignment_count <= NEW.max_children
				AND NOT EXISTS (
					SELECT 1 FROM specialist_delegation_assignments assignment
					WHERE assignment.proposal_id = NEW.proposal_id
						AND (assignment.turn_limit > NEW.max_turns_per_child
							OR assignment.token_limit > NEW.max_tokens_per_child
							OR EXISTS (
								SELECT 1 FROM json_each(assignment.skills_json) requested
								WHERE requested.type != 'text'
									OR requested.value = 'specialist_delegation_propose'
									OR NOT EXISTS (SELECT 1 FROM json_each(root.skills_json) available
										WHERE available.type = 'text' AND available.value = requested.value))))
				AND root.turns_used
					+ COALESCE((SELECT SUM(child.turn_limit) FROM agent_nodes child
						WHERE child.run_id = NEW.run_id AND child.parent_id = NEW.root_agent_id), 0)
					+ (SELECT SUM(assignment.turn_limit) FROM specialist_delegation_assignments assignment
						WHERE assignment.proposal_id = NEW.proposal_id)
					< json_extract(run.budget_json, '$.max_turns')
				AND (COALESCE(json_extract(run.budget_json, '$.max_tokens'), 0) = 0
					OR root.tokens_used
						+ COALESCE((SELECT SUM(child.token_limit) FROM agent_nodes child
							WHERE child.run_id = NEW.run_id AND child.parent_id = NEW.root_agent_id), 0)
						+ (SELECT SUM(assignment.token_limit) FROM specialist_delegation_assignments assignment
							WHERE assignment.proposal_id = NEW.proposal_id)
						< json_extract(run.budget_json, '$.max_tokens'))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_assignment_insert
		BEFORE INSERT ON specialist_delegation_application_assignments
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_applications application
			JOIN specialist_delegation_assignments assignment
				ON assignment.proposal_id = application.proposal_id AND assignment.ordinal = NEW.ordinal
			WHERE application.id = NEW.application_id
				AND application.proposal_id = NEW.proposal_id
				AND application.status = 'applying'
				AND NEW.status = 'pending' AND NEW.agent_id IS NULL AND NEW.message_id IS NULL
				AND NEW.created_at = application.created_at
				AND NEW.updated_at = application.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application assignment binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_operation_insert
		BEFORE INSERT ON specialist_delegation_application_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_applications application
			WHERE application.id = NEW.application_id AND application.review_id = NEW.review_id
				AND application.proposal_id = NEW.proposal_id AND application.run_id = NEW.run_id
				AND application.requested_by = NEW.requested_by
				AND NEW.created_at = application.created_at
				AND (SELECT COUNT(*) FROM specialist_delegation_application_assignments assignment
					WHERE assignment.application_id = application.id) = application.assignment_count
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_assignment_transition
		BEFORE UPDATE ON specialist_delegation_application_assignments
		WHEN NEW.application_id != OLD.application_id OR NEW.proposal_id != OLD.proposal_id
			OR NEW.ordinal != OLD.ordinal
			OR NEW.admission_operation_digest != OLD.admission_operation_digest
			OR NEW.instruction_operation_digest != OLD.instruction_operation_digest
			OR NEW.created_at != OLD.created_at
			OR julianday(NEW.updated_at) < julianday(OLD.updated_at)
			OR NOT ((OLD.status = 'pending' AND NEW.status = 'admitted'
					AND NEW.agent_id IS NOT NULL AND NEW.message_id IS NULL
					AND NEW.version = OLD.version + 1)
				OR (OLD.status = 'admitted' AND NEW.status = 'instructed'
					AND NEW.agent_id = OLD.agent_id AND NEW.message_id IS NOT NULL
					AND NEW.version = OLD.version + 1))
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application assignment transition is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_assignment_admitted
		BEFORE UPDATE ON specialist_delegation_application_assignments
		WHEN NEW.status = 'admitted' AND NOT EXISTS (
			SELECT 1 FROM specialist_delegation_applications application
			JOIN specialist_delegation_application_operations application_operation
				ON application_operation.application_id = application.id
				AND application_operation.proposal_id = application.proposal_id
			JOIN specialist_delegation_assignments proposed
				ON proposed.proposal_id = application.proposal_id AND proposed.ordinal = NEW.ordinal
			JOIN agent_nodes child ON child.id = NEW.agent_id
			JOIN sessions child_session ON child_session.id = child.session_id
			JOIN agent_admission_operations admission
				ON admission.agent_id = child.id
				AND admission.operation_key_digest = NEW.admission_operation_digest
			WHERE application.id = NEW.application_id AND application.status = 'applying'
				AND child.run_id = application.run_id AND child.parent_id = application.root_agent_id
				AND child.role = 'specialist' AND child.depth = 1
				AND child.turn_limit = proposed.turn_limit AND child.token_limit = proposed.token_limit
				AND child.skills_json = proposed.skills_json AND child_session.title = proposed.title
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation admitted Agent binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_assignment_instructed
		BEFORE UPDATE ON specialist_delegation_application_assignments
		WHEN NEW.status = 'instructed' AND NOT EXISTS (
			SELECT 1 FROM specialist_delegation_applications application
			JOIN specialist_delegation_application_operations application_operation
				ON application_operation.application_id = application.id
				AND application_operation.proposal_id = application.proposal_id
			JOIN specialist_delegation_assignments proposed
				ON proposed.proposal_id = application.proposal_id AND proposed.ordinal = NEW.ordinal
			JOIN agent_messages message ON message.id = NEW.message_id
			JOIN agent_message_operations operation ON operation.message_id = message.id
				AND operation.operation_key_digest = NEW.instruction_operation_digest
			WHERE application.id = NEW.application_id AND application.status = 'applying'
				AND message.run_id = application.run_id
				AND message.sender_agent_id = application.root_agent_id
				AND message.recipient_agent_id = NEW.agent_id
				AND message.kind = 'instruction' AND message.semantic = 'message'
				AND message.status = 'pending' AND json_valid(message.payload_json)
				AND json_type(message.payload_json) = 'object'
				AND json_extract(message.payload_json, '$.version') = 'specialist_instruction.v1'
				AND json_extract(message.payload_json, '$.instruction') = proposed.goal
				AND (SELECT COUNT(*) FROM json_each(message.payload_json)) = 2
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation instruction binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_complete
		BEFORE UPDATE ON specialist_delegation_applications
		WHEN NEW.status = 'applied' AND (
			NOT EXISTS (SELECT 1 FROM specialist_delegation_application_operations operation
				WHERE operation.application_id = NEW.id
					AND operation.proposal_id = NEW.proposal_id)
			OR
			(SELECT COUNT(*) FROM specialist_delegation_application_assignments assignment
			WHERE assignment.application_id = NEW.id AND assignment.status = 'instructed')
				!= OLD.assignment_count
			OR (NEW.completed_at IS NOT NULL AND julianday(NEW.completed_at) <
				(SELECT MAX(julianday(assignment.updated_at))
				FROM specialist_delegation_application_assignments assignment
				WHERE assignment.application_id = NEW.id))
		)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application is incomplete');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_transition
		BEFORE UPDATE ON specialist_delegation_applications
		WHEN NEW.id != OLD.id OR NEW.review_id != OLD.review_id OR NEW.proposal_id != OLD.proposal_id
			OR NEW.run_id != OLD.run_id OR NEW.root_agent_id != OLD.root_agent_id
			OR NEW.assignment_count != OLD.assignment_count
			OR NEW.policy_fingerprint != OLD.policy_fingerprint
			OR NEW.max_children != OLD.max_children
			OR NEW.max_turns_per_child != OLD.max_turns_per_child
			OR NEW.max_tokens_per_child != OLD.max_tokens_per_child
			OR NEW.requested_by != OLD.requested_by OR NEW.created_at != OLD.created_at
			OR OLD.status != 'applying' OR NEW.status NOT IN ('applied', 'aborted')
			OR NEW.version != OLD.version + 1 OR NEW.completed_at IS NULL
			OR NEW.updated_at != NEW.completed_at
			OR julianday(NEW.completed_at) <
				(SELECT MAX(julianday(assignment.updated_at))
				FROM specialist_delegation_application_assignments assignment
				WHERE assignment.application_id = NEW.id)
			OR (NEW.status = 'applied' AND NEW.stop_code != '')
			OR (NEW.status = 'aborted' AND length(NEW.stop_code) = 0)
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application transition is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_delete_immutable
		BEFORE DELETE ON specialist_delegation_applications
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_assignment_delete_immutable
		BEFORE DELETE ON specialist_delegation_application_assignments
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application assignment cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_operation_immutable
		BEFORE UPDATE ON specialist_delegation_application_operations
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application operation is immutable');
		END;`,
	`CREATE TRIGGER trg_specialist_delegation_application_operation_delete_immutable
		BEFORE DELETE ON specialist_delegation_application_operations
		BEGIN
			SELECT RAISE(ABORT, 'Specialist delegation application operation cannot be deleted');
		END;`,
}

var readOnlyFanoutPlanStatements = []string{
	`CREATE TABLE readonly_fanout_plans (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		scope_path TEXT NOT NULL,
		goal TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		requested_tier TEXT NOT NULL,
		effective_parallelism INTEGER NOT NULL,
		status TEXT NOT NULL,
		capability_fingerprint TEXT NOT NULL,
		snapshot_digest TEXT NOT NULL,
		file_count INTEGER NOT NULL,
		total_bytes INTEGER NOT NULL,
		excluded_count INTEGER NOT NULL,
		shard_count INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'readonly_fanout.v1'),
		CHECK(requested_tier IN ('auto', '1', '2', '4', '6')),
		CHECK(effective_parallelism BETWEEN 1 AND 6),
		CHECK(status = 'planned'),
		CHECK(capability_fingerprint = '735ca1ca0e0cdf09773b15aa7113e328744c6fde267f469c3f414648baf9e47b'
			AND length(capability_fingerprint) = 64
			AND capability_fingerprint = lower(capability_fingerprint)
			AND capability_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(snapshot_digest) = 64 AND snapshot_digest = lower(snapshot_digest)
			AND snapshot_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(file_count BETWEEN 1 AND 256),
		CHECK(total_bytes BETWEEN 0 AND 786432),
		CHECK(excluded_count BETWEEN 0 AND 20000),
		CHECK(shard_count BETWEEN 1 AND 6 AND shard_count = effective_parallelism),
		CHECK(scope_path = trim(scope_path) AND length(scope_path) BETWEEN 1 AND 2048
			AND instr(scope_path, char(0)) = 0 AND instr(scope_path, char(92)) = 0
			AND substr(scope_path, 1, 1) <> '/' AND scope_path <> '..'
			AND scope_path NOT LIKE '../%' AND instr('/' || scope_path || '/', '/../') = 0),
		CHECK(goal = trim(goal) AND length(goal) BETWEEN 1 AND 4096
			AND length(CAST(goal AS BLOB)) <= 16384 AND instr(goal, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0),
		CHECK(version = 1 AND updated_at = created_at)
	);`,
	`CREATE INDEX idx_readonly_fanout_plans_run_created
		ON readonly_fanout_plans(run_id, created_at, id);`,
	`CREATE TABLE readonly_fanout_files (
		plan_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		shard_ordinal INTEGER NOT NULL,
		relative_path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		content_sha256 TEXT NOT NULL,
		PRIMARY KEY(plan_id, ordinal),
		UNIQUE(plan_id, relative_path),
		FOREIGN KEY(plan_id) REFERENCES readonly_fanout_plans(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 256),
		CHECK(shard_ordinal BETWEEN 1 AND 6),
		CHECK(size_bytes BETWEEN 0 AND 131072),
		CHECK(length(content_sha256) = 64 AND content_sha256 = lower(content_sha256)
			AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(relative_path = trim(relative_path) AND length(relative_path) BETWEEN 1 AND 2048
			AND instr(relative_path, char(0)) = 0 AND instr(relative_path, char(92)) = 0
			AND substr(relative_path, 1, 1) <> '/' AND relative_path NOT IN ('.', '..')
			AND relative_path NOT LIKE '../%'
			AND instr('/' || relative_path || '/', '/../') = 0)
	);`,
	`CREATE INDEX idx_readonly_fanout_files_shard
		ON readonly_fanout_files(plan_id, shard_ordinal, ordinal);`,
	`CREATE TABLE readonly_fanout_shards (
		plan_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		status TEXT NOT NULL,
		file_count INTEGER NOT NULL,
		total_bytes INTEGER NOT NULL,
		input_digest TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY(plan_id, ordinal),
		FOREIGN KEY(plan_id) REFERENCES readonly_fanout_plans(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 6),
		CHECK(status = 'pending'),
		CHECK(file_count BETWEEN 1 AND 256),
		CHECK(total_bytes BETWEEN 0 AND 786432),
		CHECK(length(input_digest) = 64 AND input_digest = lower(input_digest)
			AND input_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(version = 1 AND updated_at = created_at)
	);`,
	`CREATE TABLE readonly_fanout_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(plan_id) REFERENCES readonly_fanout_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER trg_readonly_fanout_plan_insert
		BEFORE INSERT ON readonly_fanout_plans
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions active_session ON active_session.id = run.session_id
			JOIN workspaces workspace ON workspace.id = NEW.workspace_id
			WHERE run.id = NEW.run_id AND run.status = 'running'
				AND mission.workspace_id = NEW.workspace_id
				AND json_valid(mission.scope_json)
				AND json_extract(mission.scope_json, '$.workspace_id') = NEW.workspace_id
				AND json_extract(mission.scope_json, '$.network_mode') = 'disabled'
				AND active_session.status = 'active'
				AND active_session.workspace_id = NEW.workspace_id
				AND julianday(NEW.created_at) >= julianday(run.created_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out plan binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_file_insert
		BEFORE INSERT ON readonly_fanout_files
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_plans plan
			WHERE plan.id = NEW.plan_id AND plan.status = 'planned'
				AND NEW.ordinal <= plan.file_count
				AND NEW.shard_ordinal <= plan.shard_count
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out file binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_shard_insert
		BEFORE INSERT ON readonly_fanout_shards
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_plans plan
			WHERE plan.id = NEW.plan_id AND plan.status = 'planned'
				AND NEW.ordinal <= plan.shard_count
				AND NEW.created_at = plan.created_at AND NEW.updated_at = plan.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out shard binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_operation_insert
		BEFORE INSERT ON readonly_fanout_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_plans plan
			WHERE plan.id = NEW.plan_id AND plan.run_id = NEW.run_id
				AND plan.workspace_id = NEW.workspace_id
				AND plan.requested_by = NEW.requested_by
				AND plan.created_at = NEW.created_at
				AND (SELECT COUNT(*) FROM readonly_fanout_files file
					WHERE file.plan_id = plan.id) = plan.file_count
				AND (SELECT COALESCE(SUM(file.size_bytes), 0)
					FROM readonly_fanout_files file WHERE file.plan_id = plan.id) = plan.total_bytes
				AND (SELECT COUNT(*) FROM readonly_fanout_shards shard
					WHERE shard.plan_id = plan.id) = plan.shard_count
				AND NOT EXISTS (
					SELECT 1 FROM readonly_fanout_shards shard
					WHERE shard.plan_id = plan.id AND (
						shard.file_count != (SELECT COUNT(*) FROM readonly_fanout_files file
							WHERE file.plan_id = plan.id AND file.shard_ordinal = shard.ordinal)
						OR shard.total_bytes != (SELECT COALESCE(SUM(file.size_bytes), 0)
							FROM readonly_fanout_files file
							WHERE file.plan_id = plan.id AND file.shard_ordinal = shard.ordinal)))
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_plan_immutable
		BEFORE UPDATE ON readonly_fanout_plans
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out plan is immutable');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_plan_delete_immutable
		BEFORE DELETE ON readonly_fanout_plans
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out plan cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_file_immutable
		BEFORE UPDATE ON readonly_fanout_files
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out file is immutable');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_file_delete_immutable
		BEFORE DELETE ON readonly_fanout_files
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out file cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_shard_immutable
		BEFORE UPDATE ON readonly_fanout_shards
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out shard is immutable');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_shard_delete_immutable
		BEFORE DELETE ON readonly_fanout_shards
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out shard cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_operation_immutable
		BEFORE UPDATE ON readonly_fanout_operations
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out operation is immutable');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_operation_delete_immutable
		BEFORE DELETE ON readonly_fanout_operations
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out operation cannot be deleted');
		END;`,
}

var readOnlyFanoutExecutionStatements = []string{
	`ALTER TABLE specialist_schedules ADD COLUMN before_readonly_tokens
		INTEGER NOT NULL DEFAULT 0 CHECK(before_readonly_tokens >= 0);`,
	`ALTER TABLE specialist_schedules ADD COLUMN before_readonly_execution_millis
		INTEGER NOT NULL DEFAULT 0 CHECK(before_readonly_execution_millis >= 0);`,
	`ALTER TABLE specialist_schedules ADD COLUMN after_readonly_tokens
		INTEGER NOT NULL DEFAULT 0 CHECK(after_readonly_tokens >= 0);`,
	`ALTER TABLE specialist_schedules ADD COLUMN after_readonly_execution_millis
		INTEGER NOT NULL DEFAULT 0 CHECK(after_readonly_execution_millis >= 0);`,
	`CREATE TRIGGER trg_specialist_schedule_readonly_usage_insert
		BEFORE INSERT ON specialist_schedules
		WHEN NEW.status = 'running' AND
			(NEW.before_readonly_tokens != NEW.after_readonly_tokens
			 OR NEW.before_readonly_execution_millis != NEW.after_readonly_execution_millis)
		BEGIN
			SELECT RAISE(ABORT, 'running Specialist schedule read-only usage must start equal');
		END;`,
	`CREATE TABLE readonly_fanout_executions (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		status TEXT NOT NULL,
		parallelism INTEGER NOT NULL,
		max_output_tokens_per_shard INTEGER NOT NULL,
		snapshot_digest TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		stop_code TEXT NOT NULL DEFAULT '',
		lease_id TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		version INTEGER NOT NULL,
		started_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		finished_at TEXT,
		FOREIGN KEY(plan_id) REFERENCES readonly_fanout_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(status IN ('running', 'completed', 'failed', 'cancelled')),
		CHECK(parallelism BETWEEN 1 AND 6),
		CHECK(max_output_tokens_per_shard BETWEEN 128 AND 4096),
		CHECK(length(snapshot_digest) = 64 AND snapshot_digest = lower(snapshot_digest)
			AND snapshot_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0),
		CHECK(length(stop_code) <= 256 AND instr(stop_code, char(0)) = 0),
		CHECK(lease_id = trim(lease_id) AND length(lease_id) BETWEEN 1 AND 256
			AND instr(lease_id, char(0)) = 0 AND lease_generation > 0),
		CHECK(version > 0 AND julianday(updated_at) >= julianday(started_at)),
		CHECK((status = 'running' AND finished_at IS NULL AND stop_code = '') OR
			(status = 'completed' AND finished_at IS NOT NULL AND stop_code = ''
				AND updated_at = finished_at) OR
			(status IN ('failed', 'cancelled') AND finished_at IS NOT NULL
				AND length(stop_code) > 0 AND updated_at = finished_at))
	);`,
	`CREATE INDEX idx_readonly_fanout_executions_run_started
		ON readonly_fanout_executions(run_id, started_at, id);`,
	`CREATE TABLE readonly_fanout_execution_shards (
		execution_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		status TEXT NOT NULL,
		input_digest TEXT NOT NULL,
		attempt_count INTEGER NOT NULL,
		current_attempt INTEGER NOT NULL,
		provider TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		elapsed_millis INTEGER NOT NULL DEFAULT 0,
		report_json TEXT NOT NULL DEFAULT '',
		report_digest TEXT NOT NULL DEFAULT '',
		finding_count INTEGER NOT NULL DEFAULT 0,
		error_code TEXT NOT NULL DEFAULT '',
		error_reason TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		started_at TEXT,
		finished_at TEXT,
		PRIMARY KEY(execution_id, ordinal),
		FOREIGN KEY(execution_id) REFERENCES readonly_fanout_executions(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id) REFERENCES readonly_fanout_plans(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 6),
		CHECK(status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
		CHECK(length(input_digest) = 64 AND input_digest = lower(input_digest)
			AND input_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(attempt_count BETWEEN 0 AND 3 AND current_attempt BETWEEN 0 AND attempt_count),
		CHECK(length(provider) <= 256 AND length(model) <= 256
			AND instr(provider, char(0)) = 0 AND instr(model, char(0)) = 0),
		CHECK(input_tokens >= 0 AND output_tokens >= 0 AND total_tokens >= 0
			AND total_tokens >= input_tokens + output_tokens AND elapsed_millis >= 0),
		CHECK(length(CAST(report_json AS BLOB)) <= 131072),
		CHECK((report_digest = '') OR (length(report_digest) = 64
			AND report_digest = lower(report_digest)
			AND report_digest NOT GLOB '*[^0-9a-f]*')),
		CHECK(finding_count BETWEEN 0 AND 32),
		CHECK(length(error_code) <= 256 AND instr(error_code, char(0)) = 0
			AND length(CAST(error_reason AS BLOB)) <= 8192
			AND instr(error_reason, char(0)) = 0),
		CHECK(version > 0 AND julianday(updated_at) >= julianday(created_at)),
		CHECK((status = 'pending' AND current_attempt = 0 AND provider = '' AND model = ''
			AND input_tokens = 0 AND output_tokens = 0 AND total_tokens = 0
			AND elapsed_millis = 0 AND report_json = '' AND report_digest = ''
			AND finding_count = 0 AND error_code = '' AND error_reason = ''
			AND started_at IS NULL AND finished_at IS NULL) OR
			(status = 'running' AND attempt_count > 0 AND current_attempt = attempt_count
				AND provider = '' AND model = '' AND input_tokens = 0 AND output_tokens = 0
				AND total_tokens = 0 AND elapsed_millis = 0 AND report_json = ''
				AND report_digest = '' AND finding_count = 0 AND error_code = ''
				AND error_reason = '' AND started_at IS NOT NULL AND finished_at IS NULL) OR
			(status = 'completed' AND attempt_count > 0 AND current_attempt = attempt_count
				AND length(provider) > 0 AND length(model) > 0 AND json_valid(report_json)
				AND length(report_digest) = 64 AND finding_count >= 0
				AND error_code = '' AND error_reason = '' AND started_at IS NOT NULL
				AND finished_at IS NOT NULL AND updated_at = finished_at) OR
			(status = 'failed' AND attempt_count > 0 AND current_attempt = attempt_count
				AND length(provider) > 0 AND length(model) > 0 AND report_json = ''
				AND report_digest = '' AND finding_count = 0 AND length(error_code) > 0
				AND length(error_reason) > 0 AND started_at IS NOT NULL
				AND finished_at IS NOT NULL AND updated_at = finished_at) OR
			(status = 'cancelled' AND report_json = '' AND report_digest = ''
				AND finding_count = 0 AND length(error_code) > 0 AND length(error_reason) > 0
				AND finished_at IS NOT NULL AND updated_at = finished_at AND
				((attempt_count > 0 AND current_attempt = attempt_count
					AND length(provider) > 0 AND length(model) > 0 AND started_at IS NOT NULL) OR
				 (current_attempt = 0 AND provider = '' AND model = '' AND input_tokens = 0
					AND output_tokens = 0 AND total_tokens = 0 AND elapsed_millis = 0
					AND started_at IS NULL))))
	);`,
	`CREATE INDEX idx_readonly_fanout_execution_shards_status
		ON readonly_fanout_execution_shards(execution_id, status, ordinal);`,
	`CREATE TABLE readonly_fanout_model_calls (
		execution_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		shard_ordinal INTEGER NOT NULL,
		attempt_number INTEGER NOT NULL,
		lease_id TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		status TEXT NOT NULL,
		outcome TEXT NOT NULL DEFAULT '',
		input_fingerprint TEXT NOT NULL,
		response_digest TEXT NOT NULL DEFAULT '',
		reserved_input_tokens INTEGER NOT NULL,
		reserved_output_tokens INTEGER NOT NULL,
		reserved_total_tokens INTEGER NOT NULL,
		reserved_millis INTEGER NOT NULL,
		usage_recorded INTEGER NOT NULL DEFAULT 0,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		elapsed_recorded INTEGER NOT NULL DEFAULT 0,
		elapsed_millis INTEGER NOT NULL DEFAULT 0,
		error_code TEXT NOT NULL DEFAULT '',
		error_reason TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT,
		PRIMARY KEY(execution_id, shard_ordinal, attempt_number),
		FOREIGN KEY(execution_id, shard_ordinal)
			REFERENCES readonly_fanout_execution_shards(execution_id, ordinal) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id) REFERENCES readonly_fanout_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(shard_ordinal BETWEEN 1 AND 6 AND attempt_number > 0),
		CHECK(lease_id = trim(lease_id) AND length(lease_id) BETWEEN 1 AND 256
			AND instr(lease_id, char(0)) = 0 AND lease_generation > 0),
		CHECK(provider = trim(provider) AND model = trim(model)
			AND length(provider) BETWEEN 1 AND 256 AND length(model) BETWEEN 1 AND 256
			AND instr(provider, char(0)) = 0 AND instr(model, char(0)) = 0),
		CHECK(status IN ('started', 'completed', 'failed', 'cancelled', 'abandoned')),
		CHECK(length(input_fingerprint) = 64 AND input_fingerprint = lower(input_fingerprint)
			AND input_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK((response_digest = '') OR (length(response_digest) = 64
			AND response_digest = lower(response_digest)
			AND response_digest NOT GLOB '*[^0-9a-f]*')),
		CHECK(reserved_input_tokens >= 0 AND reserved_output_tokens BETWEEN 128 AND 4096
			AND reserved_total_tokens = reserved_input_tokens + reserved_output_tokens
			AND reserved_millis >= 0),
		CHECK(usage_recorded IN (0, 1) AND input_tokens >= 0 AND output_tokens >= 0
			AND total_tokens >= 0 AND total_tokens >= input_tokens + output_tokens),
		CHECK(elapsed_recorded IN (0, 1) AND elapsed_millis >= 0),
		CHECK(length(error_code) <= 256 AND instr(error_code, char(0)) = 0
			AND length(CAST(error_reason AS BLOB)) <= 8192
			AND instr(error_reason, char(0)) = 0),
		CHECK(version > 0),
		CHECK((status = 'started' AND outcome = '' AND response_digest = ''
			AND usage_recorded = 0 AND input_tokens = 0 AND output_tokens = 0
			AND total_tokens = 0 AND elapsed_recorded = 0 AND elapsed_millis = 0
			AND error_code = '' AND error_reason = '' AND finished_at IS NULL) OR
			(status = 'completed' AND outcome = 'success' AND length(response_digest) = 64
				AND usage_recorded = 1 AND elapsed_recorded = 1 AND error_code = ''
				AND error_reason = '' AND finished_at IS NOT NULL) OR
			(status IN ('failed', 'cancelled', 'abandoned') AND length(outcome) > 0
				AND response_digest = '' AND length(error_code) > 0
				AND length(error_reason) > 0 AND finished_at IS NOT NULL))
	);`,
	`CREATE INDEX idx_readonly_fanout_model_calls_run_status
		ON readonly_fanout_model_calls(run_id, status, execution_id, shard_ordinal);`,
	`CREATE TABLE readonly_fanout_findings (
		execution_id TEXT NOT NULL,
		shard_ordinal INTEGER NOT NULL,
		ordinal INTEGER NOT NULL,
		fingerprint TEXT NOT NULL,
		severity TEXT NOT NULL,
		category TEXT NOT NULL,
		title TEXT NOT NULL,
		detail TEXT NOT NULL,
		relative_path TEXT NOT NULL,
		line_start INTEGER NOT NULL,
		line_end INTEGER NOT NULL,
		confidence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(execution_id, shard_ordinal, ordinal),
		UNIQUE(execution_id, fingerprint),
		FOREIGN KEY(execution_id, shard_ordinal)
			REFERENCES readonly_fanout_execution_shards(execution_id, ordinal) ON DELETE RESTRICT,
		CHECK(shard_ordinal BETWEEN 1 AND 6 AND ordinal BETWEEN 1 AND 32),
		CHECK(length(fingerprint) = 64 AND fingerprint = lower(fingerprint)
			AND fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(severity IN ('info', 'low', 'medium', 'high', 'critical')),
		CHECK(category = trim(category) AND length(category) BETWEEN 1 AND 64),
		CHECK(title = trim(title) AND length(title) BETWEEN 1 AND 256),
		CHECK(detail = trim(detail) AND length(detail) BETWEEN 1 AND 2048),
		CHECK(relative_path = trim(relative_path) AND length(relative_path) BETWEEN 1 AND 2048
			AND instr(relative_path, char(0)) = 0 AND instr(relative_path, char(92)) = 0
			AND substr(relative_path, 1, 1) <> '/' AND relative_path NOT IN ('.', '..')
			AND relative_path NOT LIKE '../%'
			AND instr('/' || relative_path || '/', '/../') = 0),
		CHECK(line_start >= 0 AND line_end >= line_start AND confidence BETWEEN 0 AND 100)
	);`,
	`CREATE INDEX idx_readonly_fanout_findings_severity
		ON readonly_fanout_findings(execution_id, severity, shard_ordinal, ordinal);`,
	`CREATE TABLE readonly_fanout_execution_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		execution_id TEXT NOT NULL UNIQUE,
		plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(execution_id) REFERENCES readonly_fanout_executions(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id) REFERENCES readonly_fanout_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER trg_readonly_fanout_execution_insert
		BEFORE INSERT ON readonly_fanout_executions
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_plans plan
			JOIN runs run ON run.id = plan.run_id
			JOIN run_execution_leases lease ON lease.run_id = run.id
			WHERE plan.id = NEW.plan_id AND plan.run_id = NEW.run_id
				AND plan.workspace_id = NEW.workspace_id AND plan.status = 'planned'
				AND plan.effective_parallelism = NEW.parallelism
				AND plan.snapshot_digest = NEW.snapshot_digest
				AND plan.requested_by = NEW.requested_by AND run.status = 'running'
				AND lease.lease_id = NEW.lease_id
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday(NEW.started_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_shard_insert
		BEFORE INSERT ON readonly_fanout_execution_shards
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_executions execution
			JOIN readonly_fanout_shards plan_shard
				ON plan_shard.plan_id = execution.plan_id AND plan_shard.ordinal = NEW.ordinal
			WHERE execution.id = NEW.execution_id AND execution.plan_id = NEW.plan_id
				AND execution.status = 'running' AND NEW.ordinal <= execution.parallelism
				AND plan_shard.input_digest = NEW.input_digest
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution shard binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_operation_insert
		BEFORE INSERT ON readonly_fanout_execution_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_executions execution
			WHERE execution.id = NEW.execution_id AND execution.plan_id = NEW.plan_id
				AND execution.run_id = NEW.run_id AND execution.requested_by = NEW.requested_by
				AND execution.started_at = NEW.created_at
				AND (SELECT COUNT(*) FROM readonly_fanout_execution_shards shard
					WHERE shard.execution_id = execution.id) = execution.parallelism
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_transition
		BEFORE UPDATE ON readonly_fanout_executions
		WHEN OLD.status != 'running'
			OR NEW.id != OLD.id OR NEW.plan_id != OLD.plan_id OR NEW.run_id != OLD.run_id
			OR NEW.workspace_id != OLD.workspace_id OR NEW.parallelism != OLD.parallelism
			OR NEW.max_output_tokens_per_shard != OLD.max_output_tokens_per_shard
			OR NEW.snapshot_digest != OLD.snapshot_digest OR NEW.requested_by != OLD.requested_by
			OR NEW.started_at != OLD.started_at OR NEW.version != OLD.version + 1
			OR julianday(NEW.updated_at) < julianday(OLD.updated_at)
			OR (NEW.status = 'running' AND
				(NEW.finished_at IS NOT NULL OR NEW.stop_code != ''
				 OR NEW.lease_generation <= OLD.lease_generation
				 OR NEW.lease_id = OLD.lease_id))
			OR (NEW.status IN ('completed', 'failed', 'cancelled') AND
				(NEW.lease_id != OLD.lease_id OR NEW.lease_generation != OLD.lease_generation
				 OR NEW.finished_at IS NULL OR NEW.updated_at != NEW.finished_at))
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution transition is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_shard_transition
		BEFORE UPDATE ON readonly_fanout_execution_shards
		WHEN OLD.status IN ('completed', 'failed', 'cancelled')
			OR NEW.execution_id != OLD.execution_id OR NEW.plan_id != OLD.plan_id
			OR NEW.ordinal != OLD.ordinal OR NEW.input_digest != OLD.input_digest
			OR NEW.created_at != OLD.created_at OR NEW.version != OLD.version + 1
			OR julianday(NEW.updated_at) < julianday(OLD.updated_at)
			OR (OLD.status = 'pending' AND NEW.status NOT IN ('running', 'cancelled'))
			OR (OLD.status = 'running' AND NEW.status NOT IN
				('pending', 'completed', 'failed', 'cancelled'))
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution shard transition is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_model_call_insert
		BEFORE INSERT ON readonly_fanout_model_calls
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_executions execution
			JOIN readonly_fanout_execution_shards shard
				ON shard.execution_id = execution.id AND shard.ordinal = NEW.shard_ordinal
			JOIN run_execution_leases lease ON lease.run_id = execution.run_id
			WHERE execution.id = NEW.execution_id AND execution.plan_id = NEW.plan_id
				AND execution.run_id = NEW.run_id AND execution.status = 'running'
				AND execution.lease_id = NEW.lease_id
				AND execution.lease_generation = NEW.lease_generation
				AND shard.status = 'running' AND shard.current_attempt = NEW.attempt_number
				AND lease.lease_id = NEW.lease_id
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out model call binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_model_call_transition
		BEFORE UPDATE ON readonly_fanout_model_calls
		WHEN OLD.status != 'started' OR NEW.status = 'started'
			OR NEW.execution_id != OLD.execution_id OR NEW.plan_id != OLD.plan_id
			OR NEW.run_id != OLD.run_id OR NEW.shard_ordinal != OLD.shard_ordinal
			OR NEW.attempt_number != OLD.attempt_number OR NEW.lease_id != OLD.lease_id
			OR NEW.lease_generation != OLD.lease_generation OR NEW.provider != OLD.provider
			OR NEW.model != OLD.model OR NEW.input_fingerprint != OLD.input_fingerprint
			OR NEW.reserved_input_tokens != OLD.reserved_input_tokens
			OR NEW.reserved_output_tokens != OLD.reserved_output_tokens
			OR NEW.reserved_total_tokens != OLD.reserved_total_tokens
			OR NEW.reserved_millis != OLD.reserved_millis OR NEW.started_at != OLD.started_at
			OR NEW.version != OLD.version + 1
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out model call transition is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_finding_insert
		BEFORE INSERT ON readonly_fanout_findings
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_execution_shards shard
			JOIN readonly_fanout_files file ON file.plan_id = shard.plan_id
				AND file.shard_ordinal = shard.ordinal
			WHERE shard.execution_id = NEW.execution_id
				AND shard.ordinal = NEW.shard_ordinal AND shard.status = 'completed'
				AND file.relative_path = NEW.relative_path
		)
		BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out finding binding is invalid');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_delete_immutable
		BEFORE DELETE ON readonly_fanout_executions BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_shard_delete_immutable
		BEFORE DELETE ON readonly_fanout_execution_shards BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution shard cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_model_call_delete_immutable
		BEFORE DELETE ON readonly_fanout_model_calls BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out model call cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_finding_immutable
		BEFORE UPDATE ON readonly_fanout_findings BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out finding is immutable');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_finding_delete_immutable
		BEFORE DELETE ON readonly_fanout_findings BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out finding cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_operation_immutable
		BEFORE UPDATE ON readonly_fanout_execution_operations BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution operation is immutable');
		END;`,
	`CREATE TRIGGER trg_readonly_fanout_execution_operation_delete_immutable
		BEFORE DELETE ON readonly_fanout_execution_operations BEGIN
			SELECT RAISE(ABORT, 'read-only fan-out execution operation cannot be deleted');
		END;`,
}

var findingReportStatements = []string{
	`CREATE TABLE finding_reports (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		source_kind TEXT NOT NULL,
		source_id TEXT NOT NULL UNIQUE,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		title TEXT NOT NULL,
		projection_digest TEXT NOT NULL DEFAULT '',
		finding_count INTEGER NOT NULL DEFAULT 0,
		evidence_count INTEGER NOT NULL DEFAULT 0,
		info_count INTEGER NOT NULL DEFAULT 0,
		low_count INTEGER NOT NULL DEFAULT 0,
		medium_count INTEGER NOT NULL DEFAULT 0,
		high_count INTEGER NOT NULL DEFAULT 0,
		critical_count INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(source_id) REFERENCES readonly_fanout_executions(id) ON DELETE RESTRICT,
		CHECK(source_kind = 'readonly_fanout_execution'),
		CHECK(protocol_version = 'finding_report.v1'),
		CHECK(status IN ('building', 'generated')),
		CHECK(title = trim(title) AND length(title) BETWEEN 1 AND 256
			AND length(CAST(title AS BLOB)) <= 1024 AND instr(title, char(0)) = 0),
		CHECK((projection_digest = '') OR (length(projection_digest) = 64
			AND projection_digest = lower(projection_digest)
			AND projection_digest NOT GLOB '*[^0-9a-f]*')),
		CHECK(finding_count BETWEEN 0 AND 192 AND evidence_count BETWEEN 0 AND 192),
		CHECK(info_count >= 0 AND low_count >= 0 AND medium_count >= 0
			AND high_count >= 0 AND critical_count >= 0
			AND info_count + low_count + medium_count + high_count + critical_count
				= finding_count),
		CHECK((status = 'building' AND projection_digest = '' AND finding_count = 0
			AND evidence_count = 0 AND version = 1) OR
			(status = 'generated' AND length(projection_digest) = 64 AND version = 2))
	);`,
	`CREATE INDEX idx_finding_reports_run_created
		ON finding_reports(run_id, created_at, id);`,
	`CREATE TABLE findings (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		fingerprint TEXT NOT NULL,
		status TEXT NOT NULL,
		severity TEXT NOT NULL,
		category TEXT NOT NULL,
		title TEXT NOT NULL,
		detail TEXT NOT NULL,
		relative_path TEXT NOT NULL,
		line_start INTEGER NOT NULL,
		line_end INTEGER NOT NULL,
		confidence INTEGER NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES finding_reports(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		UNIQUE(report_id, ordinal),
		UNIQUE(report_id, fingerprint),
		CHECK(ordinal BETWEEN 1 AND 192),
		CHECK(length(fingerprint) = 64 AND fingerprint = lower(fingerprint)
			AND fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(status = 'draft'),
		CHECK(severity IN ('info', 'low', 'medium', 'high', 'critical')),
		CHECK(category = trim(category) AND length(category) BETWEEN 1 AND 64
			AND length(CAST(category AS BLOB)) <= 256 AND instr(category, char(0)) = 0),
		CHECK(title = trim(title) AND length(title) BETWEEN 1 AND 256
			AND length(CAST(title AS BLOB)) <= 1024 AND instr(title, char(0)) = 0),
		CHECK(detail = trim(detail) AND length(detail) BETWEEN 1 AND 2048
			AND length(CAST(detail AS BLOB)) <= 8192 AND instr(detail, char(0)) = 0),
		CHECK(relative_path = trim(relative_path)
			AND length(relative_path) BETWEEN 1 AND 2048
			AND instr(relative_path, char(0)) = 0 AND instr(relative_path, char(92)) = 0
			AND substr(relative_path, 1, 1) <> '/' AND relative_path NOT IN ('.', '..')
			AND relative_path NOT LIKE '../%'
			AND instr('/' || relative_path || '/', '/../') = 0),
		CHECK(line_start >= 0 AND line_end >= line_start AND confidence BETWEEN 0 AND 100),
		CHECK(version = 1)
	);`,
	`CREATE INDEX idx_findings_report_severity
		ON findings(report_id, severity, ordinal);`,
	`CREATE TABLE finding_evidence (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		finding_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		kind TEXT NOT NULL,
		source_kind TEXT NOT NULL,
		source_id TEXT NOT NULL,
		source_shard INTEGER NOT NULL,
		source_ordinal INTEGER NOT NULL,
		source_fingerprint TEXT NOT NULL,
		source_digest TEXT NOT NULL,
		relative_path TEXT NOT NULL,
		line_start INTEGER NOT NULL,
		line_end INTEGER NOT NULL,
		confidence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES finding_reports(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(source_id, source_shard, source_ordinal)
			REFERENCES readonly_fanout_findings(execution_id, shard_ordinal, ordinal)
			ON DELETE RESTRICT,
		UNIQUE(finding_id, ordinal),
		UNIQUE(source_kind, source_id, source_shard, source_ordinal),
		CHECK(ordinal BETWEEN 1 AND 192),
		CHECK(kind = 'model_assertion'),
		CHECK(source_kind = 'readonly_fanout_finding'),
		CHECK(source_shard BETWEEN 1 AND 6 AND source_ordinal BETWEEN 1 AND 32),
		CHECK(length(source_fingerprint) = 64
			AND source_fingerprint = lower(source_fingerprint)
			AND source_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(source_digest) = 64 AND source_digest = lower(source_digest)
			AND source_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(relative_path = trim(relative_path)
			AND length(relative_path) BETWEEN 1 AND 2048
			AND instr(relative_path, char(0)) = 0 AND instr(relative_path, char(92)) = 0
			AND substr(relative_path, 1, 1) <> '/' AND relative_path NOT IN ('.', '..')
			AND relative_path NOT LIKE '../%'
			AND instr('/' || relative_path || '/', '/../') = 0),
		CHECK(line_start >= 0 AND line_end >= line_start AND confidence BETWEEN 0 AND 100)
	);`,
	`CREATE INDEX idx_finding_evidence_report_finding
		ON finding_evidence(report_id, finding_id, ordinal);`,
	`CREATE TRIGGER trg_finding_report_insert
		BEFORE INSERT ON finding_reports
		WHEN NOT EXISTS (
			SELECT 1 FROM readonly_fanout_executions execution
			WHERE execution.id = NEW.source_id AND execution.run_id = NEW.run_id
				AND execution.status = 'completed' AND execution.finished_at = NEW.created_at
				AND NEW.status = 'building' AND NEW.version = 1
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding report source binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_insert
		BEFORE INSERT ON findings
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_reports report
			JOIN readonly_fanout_findings source ON source.execution_id = report.source_id
			WHERE report.id = NEW.report_id AND report.run_id = NEW.run_id
				AND report.status = 'building' AND source.severity = NEW.severity
				AND source.category = NEW.category AND source.title = NEW.title
				AND source.detail = NEW.detail AND source.relative_path = NEW.relative_path
				AND source.line_start = NEW.line_start AND source.line_end = NEW.line_end
				AND NEW.confidence = (
					SELECT MIN(candidate.confidence)
					FROM readonly_fanout_findings candidate
					WHERE candidate.execution_id = report.source_id
						AND candidate.severity = NEW.severity
						AND candidate.category = NEW.category
						AND candidate.title = NEW.title
						AND candidate.detail = NEW.detail
						AND candidate.relative_path = NEW.relative_path
						AND candidate.line_start = NEW.line_start
						AND candidate.line_end = NEW.line_end
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding source projection is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_evidence_insert
		BEFORE INSERT ON finding_evidence
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_reports report
			JOIN findings finding ON finding.report_id = report.id
			JOIN readonly_fanout_findings source
				ON source.execution_id = report.source_id
				AND source.shard_ordinal = NEW.source_shard
				AND source.ordinal = NEW.source_ordinal
			JOIN readonly_fanout_execution_shards shard
				ON shard.execution_id = source.execution_id
				AND shard.ordinal = source.shard_ordinal
			WHERE report.id = NEW.report_id AND report.run_id = NEW.run_id
				AND report.status = 'building' AND finding.id = NEW.finding_id
				AND finding.run_id = NEW.run_id AND NEW.source_id = report.source_id
				AND NEW.source_fingerprint = source.fingerprint
				AND NEW.source_digest = shard.report_digest
				AND NEW.relative_path = source.relative_path
				AND NEW.line_start = source.line_start AND NEW.line_end = source.line_end
				AND NEW.confidence = source.confidence
				AND finding.severity = source.severity
				AND finding.category = source.category AND finding.title = source.title
				AND finding.detail = source.detail
				AND finding.relative_path = source.relative_path
				AND finding.line_start = source.line_start
				AND finding.line_end = source.line_end
				AND finding.confidence <= source.confidence
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding evidence source binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_report_generate
		BEFORE UPDATE ON finding_reports
		WHEN OLD.status != 'building' OR NEW.status != 'generated'
			OR NEW.id != OLD.id OR NEW.run_id != OLD.run_id
			OR NEW.source_kind != OLD.source_kind OR NEW.source_id != OLD.source_id
			OR NEW.protocol_version != OLD.protocol_version OR NEW.title != OLD.title
			OR NEW.created_at != OLD.created_at OR NEW.version != 2
			OR NEW.evidence_count != (SELECT COUNT(*)
				FROM readonly_fanout_findings source
				WHERE source.execution_id = OLD.source_id)
			OR NEW.finding_count != (SELECT COUNT(*) FROM (
				SELECT 1 FROM readonly_fanout_findings source
				WHERE source.execution_id = OLD.source_id
				GROUP BY source.severity, source.category, source.title, source.detail,
					source.relative_path, source.line_start, source.line_end
			))
			OR NEW.finding_count != (SELECT COUNT(*) FROM findings
				WHERE report_id = OLD.id)
			OR NEW.evidence_count != (SELECT COUNT(*) FROM finding_evidence
				WHERE report_id = OLD.id)
			OR NEW.info_count != (SELECT COUNT(*) FROM findings
				WHERE report_id = OLD.id AND severity = 'info')
			OR NEW.low_count != (SELECT COUNT(*) FROM findings
				WHERE report_id = OLD.id AND severity = 'low')
			OR NEW.medium_count != (SELECT COUNT(*) FROM findings
				WHERE report_id = OLD.id AND severity = 'medium')
			OR NEW.high_count != (SELECT COUNT(*) FROM findings
				WHERE report_id = OLD.id AND severity = 'high')
			OR NEW.critical_count != (SELECT COUNT(*) FROM findings
				WHERE report_id = OLD.id AND severity = 'critical')
			OR EXISTS (SELECT 1 FROM findings finding
				WHERE finding.report_id = OLD.id AND NOT EXISTS (
					SELECT 1 FROM finding_evidence evidence
					WHERE evidence.finding_id = finding.id))
			OR EXISTS (SELECT 1 FROM findings finding
				WHERE finding.report_id = OLD.id AND (
					(SELECT MIN(evidence.ordinal) FROM finding_evidence evidence
						WHERE evidence.finding_id = finding.id) != 1
					OR (SELECT MAX(evidence.ordinal) FROM finding_evidence evidence
						WHERE evidence.finding_id = finding.id) !=
						(SELECT COUNT(*) FROM finding_evidence evidence
							WHERE evidence.finding_id = finding.id)))
			OR (NEW.finding_count > 0 AND (SELECT MIN(ordinal) FROM findings
				WHERE report_id = OLD.id) != 1)
			OR (NEW.finding_count > 0 AND (SELECT MAX(ordinal) FROM findings
				WHERE report_id = OLD.id) != NEW.finding_count)
		BEGIN
			SELECT RAISE(ABORT, 'finding report generation is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_report_delete_immutable
		BEFORE DELETE ON finding_reports BEGIN
			SELECT RAISE(ABORT, 'finding report cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_update_immutable
		BEFORE UPDATE ON findings BEGIN
			SELECT RAISE(ABORT, 'finding cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_delete_immutable
		BEFORE DELETE ON findings BEGIN
			SELECT RAISE(ABORT, 'finding cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_evidence_update_immutable
		BEFORE UPDATE ON finding_evidence BEGIN
			SELECT RAISE(ABORT, 'finding evidence cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_evidence_delete_immutable
		BEFORE DELETE ON finding_evidence BEGIN
			SELECT RAISE(ABORT, 'finding evidence cannot be deleted');
		END;`,
}

var findingValidationStatements = []string{
	`CREATE TABLE finding_artifact_evidence (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		finding_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		artifact_id TEXT NOT NULL,
		artifact_sha256 TEXT NOT NULL,
		artifact_size_bytes INTEGER NOT NULL,
		artifact_mime TEXT NOT NULL,
		artifact_stream TEXT NOT NULL,
		artifact_tool TEXT NOT NULL,
		artifact_source_id TEXT NOT NULL,
		artifact_redacted INTEGER NOT NULL,
		attached_by TEXT NOT NULL,
		note TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES finding_reports(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(artifact_id) REFERENCES run_artifacts(id) ON DELETE RESTRICT,
		UNIQUE(finding_id, ordinal),
		UNIQUE(finding_id, artifact_id),
		CHECK(ordinal BETWEEN 1 AND 64),
		CHECK(length(artifact_sha256) = 64
			AND artifact_sha256 = lower(artifact_sha256)
			AND artifact_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(artifact_size_bytes BETWEEN 1 AND 4194304),
		CHECK(artifact_mime = trim(artifact_mime)
			AND length(artifact_mime) BETWEEN 1 AND 256
			AND instr(artifact_mime, char(0)) = 0),
		CHECK(artifact_stream IN ('stdout', 'stderr')),
		CHECK(artifact_redacted IN (0, 1)),
		CHECK(artifact_tool = trim(artifact_tool)
			AND length(artifact_tool) BETWEEN 1 AND 256
			AND instr(artifact_tool, char(0)) = 0),
		CHECK(artifact_source_id = trim(artifact_source_id)
			AND length(artifact_source_id) BETWEEN 1 AND 256
			AND instr(artifact_source_id, char(0)) = 0),
		CHECK(attached_by = trim(attached_by) AND length(attached_by) BETWEEN 1 AND 256
			AND instr(attached_by, char(0)) = 0),
		CHECK(note = trim(note) AND length(note) BETWEEN 1 AND 2048
			AND length(CAST(note AS BLOB)) <= 8192 AND instr(note, char(0)) = 0)
	);`,
	`CREATE INDEX idx_finding_artifact_evidence_report_finding
		ON finding_artifact_evidence(report_id, finding_id, ordinal);`,
	`CREATE TABLE finding_artifact_evidence_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		evidence_id TEXT NOT NULL UNIQUE,
		finding_id TEXT NOT NULL,
		artifact_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		attached_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(evidence_id) REFERENCES finding_artifact_evidence(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(artifact_id) REFERENCES run_artifacts(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(attached_by = trim(attached_by) AND length(attached_by) BETWEEN 1 AND 256
			AND instr(attached_by, char(0)) = 0)
	);`,
	`CREATE TABLE finding_validation_decisions (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		finding_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		from_status TEXT NOT NULL,
		status TEXT NOT NULL,
		decided_by TEXT NOT NULL,
		reason TEXT NOT NULL,
		artifact_evidence_count INTEGER NOT NULL,
		artifact_evidence_digest TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES finding_reports(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(from_status = 'draft'),
		CHECK(status IN ('validated', 'rejected')),
		CHECK(decided_by = trim(decided_by) AND length(decided_by) BETWEEN 1 AND 256
			AND instr(decided_by, char(0)) = 0),
		CHECK(reason = trim(reason) AND length(reason) BETWEEN 1 AND 2048
			AND length(CAST(reason AS BLOB)) <= 8192 AND instr(reason, char(0)) = 0),
		CHECK(artifact_evidence_count BETWEEN 0 AND 64),
		CHECK(length(artifact_evidence_digest) = 64
			AND artifact_evidence_digest = lower(artifact_evidence_digest)
			AND artifact_evidence_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(status != 'validated' OR artifact_evidence_count > 0),
		CHECK(version = 1)
	);`,
	`CREATE INDEX idx_finding_validation_run_created
		ON finding_validation_decisions(run_id, created_at, id);`,
	`CREATE TABLE finding_validation_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		validation_id TEXT NOT NULL UNIQUE,
		finding_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		status TEXT NOT NULL,
		decided_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(validation_id) REFERENCES finding_validation_decisions(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(status IN ('validated', 'rejected')),
		CHECK(decided_by = trim(decided_by) AND length(decided_by) BETWEEN 1 AND 256
			AND instr(decided_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER trg_finding_artifact_evidence_insert
		BEFORE INSERT ON finding_artifact_evidence
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_reports report
			JOIN findings finding ON finding.report_id = report.id
			JOIN run_artifacts artifact ON artifact.id = NEW.artifact_id
			WHERE report.id = NEW.report_id AND report.run_id = NEW.run_id
				AND report.status = 'generated' AND finding.id = NEW.finding_id
				AND finding.run_id = NEW.run_id AND finding.status = 'draft'
				AND artifact.run_id = NEW.run_id
				AND artifact.sha256 = NEW.artifact_sha256
				AND artifact.size_bytes = NEW.artifact_size_bytes
				AND artifact.mime = NEW.artifact_mime
				AND artifact.stream = NEW.artifact_stream
				AND artifact.tool_name = NEW.artifact_tool
				AND artifact.source_id = NEW.artifact_source_id
				AND artifact.redacted = NEW.artifact_redacted
				AND artifact.created_at <= NEW.created_at
				AND NEW.ordinal = 1 + (SELECT COUNT(*)
					FROM finding_artifact_evidence existing
					WHERE existing.finding_id = NEW.finding_id)
				AND NOT EXISTS (SELECT 1 FROM finding_validation_decisions decision
					WHERE decision.finding_id = NEW.finding_id)
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding Artifact Evidence binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_artifact_evidence_operation_insert
		BEFORE INSERT ON finding_artifact_evidence_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_artifact_evidence evidence
			WHERE evidence.id = NEW.evidence_id
				AND evidence.finding_id = NEW.finding_id
				AND evidence.artifact_id = NEW.artifact_id
				AND evidence.run_id = NEW.run_id
				AND evidence.attached_by = NEW.attached_by
				AND evidence.created_at = NEW.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding Artifact Evidence operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_validation_insert
		BEFORE INSERT ON finding_validation_decisions
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_reports report
			JOIN findings finding ON finding.report_id = report.id
			WHERE report.id = NEW.report_id AND report.run_id = NEW.run_id
				AND report.status = 'generated' AND finding.id = NEW.finding_id
				AND finding.run_id = NEW.run_id AND finding.status = NEW.from_status
				AND NEW.artifact_evidence_count = (SELECT COUNT(*)
					FROM finding_artifact_evidence evidence
					WHERE evidence.finding_id = NEW.finding_id)
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding validation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_validation_operation_insert
		BEFORE INSERT ON finding_validation_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_validation_decisions decision
			WHERE decision.id = NEW.validation_id
				AND decision.finding_id = NEW.finding_id
				AND decision.run_id = NEW.run_id
				AND decision.status = NEW.status
				AND decision.decided_by = NEW.decided_by
				AND decision.created_at = NEW.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding validation operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_run_artifact_update_immutable
		BEFORE UPDATE ON run_artifacts BEGIN
			SELECT RAISE(ABORT, 'run Artifact cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_artifact_delete_immutable
		BEFORE DELETE ON run_artifacts BEGIN
			SELECT RAISE(ABORT, 'run Artifact cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_artifact_evidence_update_immutable
		BEFORE UPDATE ON finding_artifact_evidence BEGIN
			SELECT RAISE(ABORT, 'finding Artifact Evidence cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_artifact_evidence_delete_immutable
		BEFORE DELETE ON finding_artifact_evidence BEGIN
			SELECT RAISE(ABORT, 'finding Artifact Evidence cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_artifact_evidence_operation_update_immutable
		BEFORE UPDATE ON finding_artifact_evidence_operations BEGIN
			SELECT RAISE(ABORT, 'finding Artifact Evidence operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_artifact_evidence_operation_delete_immutable
		BEFORE DELETE ON finding_artifact_evidence_operations BEGIN
			SELECT RAISE(ABORT, 'finding Artifact Evidence operation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_validation_update_immutable
		BEFORE UPDATE ON finding_validation_decisions BEGIN
			SELECT RAISE(ABORT, 'finding validation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_validation_delete_immutable
		BEFORE DELETE ON finding_validation_decisions BEGIN
			SELECT RAISE(ABORT, 'finding validation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_validation_operation_update_immutable
		BEFORE UPDATE ON finding_validation_operations BEGIN
			SELECT RAISE(ABORT, 'finding validation operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_validation_operation_delete_immutable
		BEFORE DELETE ON finding_validation_operations BEGIN
			SELECT RAISE(ABORT, 'finding validation operation cannot be deleted');
		END;`,
}

var findingRemediationStatements = []string{
	`CREATE TABLE finding_acceptance_decisions (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		finding_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		validation_id TEXT NOT NULL UNIQUE,
		from_status TEXT NOT NULL,
		status TEXT NOT NULL,
		validation_artifact_evidence_count INTEGER NOT NULL,
		validation_artifact_evidence_digest TEXT NOT NULL,
		decided_by TEXT NOT NULL,
		reason TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES finding_reports(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(validation_id) REFERENCES finding_validation_decisions(id) ON DELETE RESTRICT,
		CHECK(from_status = 'validated'),
		CHECK(status = 'accepted'),
		CHECK(validation_artifact_evidence_count BETWEEN 1 AND 64),
		CHECK(length(validation_artifact_evidence_digest) = 64
			AND validation_artifact_evidence_digest = lower(validation_artifact_evidence_digest)
			AND validation_artifact_evidence_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(decided_by = trim(decided_by) AND length(decided_by) BETWEEN 1 AND 256
			AND instr(decided_by, char(0)) = 0),
		CHECK(reason = trim(reason) AND length(reason) BETWEEN 1 AND 2048
			AND length(CAST(reason AS BLOB)) <= 8192 AND instr(reason, char(0)) = 0),
		CHECK(version = 1)
	);`,
	`CREATE INDEX idx_finding_acceptance_run_created
		ON finding_acceptance_decisions(run_id, created_at, id);`,
	`CREATE TABLE finding_acceptance_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		acceptance_id TEXT NOT NULL UNIQUE,
		validation_id TEXT NOT NULL,
		finding_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		decided_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(acceptance_id) REFERENCES finding_acceptance_decisions(id) ON DELETE RESTRICT,
		FOREIGN KEY(validation_id) REFERENCES finding_validation_decisions(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(decided_by = trim(decided_by) AND length(decided_by) BETWEEN 1 AND 256
			AND instr(decided_by, char(0)) = 0)
	);`,
	`CREATE TABLE finding_remediation_evidence (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		finding_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		acceptance_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		artifact_id TEXT NOT NULL,
		artifact_sha256 TEXT NOT NULL,
		artifact_size_bytes INTEGER NOT NULL,
		artifact_mime TEXT NOT NULL,
		artifact_stream TEXT NOT NULL,
		artifact_tool TEXT NOT NULL,
		artifact_source_id TEXT NOT NULL,
		artifact_redacted INTEGER NOT NULL,
		attached_by TEXT NOT NULL,
		note TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES finding_reports(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(acceptance_id) REFERENCES finding_acceptance_decisions(id) ON DELETE RESTRICT,
		FOREIGN KEY(artifact_id) REFERENCES run_artifacts(id) ON DELETE RESTRICT,
		UNIQUE(finding_id, ordinal),
		UNIQUE(finding_id, artifact_id),
		CHECK(ordinal BETWEEN 1 AND 64),
		CHECK(length(artifact_sha256) = 64
			AND artifact_sha256 = lower(artifact_sha256)
			AND artifact_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(artifact_size_bytes BETWEEN 1 AND 4194304),
		CHECK(artifact_mime = trim(artifact_mime)
			AND length(artifact_mime) BETWEEN 1 AND 256
			AND instr(artifact_mime, char(0)) = 0),
		CHECK(artifact_stream IN ('stdout', 'stderr')),
		CHECK(artifact_redacted IN (0, 1)),
		CHECK(artifact_tool = trim(artifact_tool)
			AND length(artifact_tool) BETWEEN 1 AND 256
			AND instr(artifact_tool, char(0)) = 0),
		CHECK(artifact_source_id = trim(artifact_source_id)
			AND length(artifact_source_id) BETWEEN 1 AND 256
			AND instr(artifact_source_id, char(0)) = 0),
		CHECK(attached_by = trim(attached_by) AND length(attached_by) BETWEEN 1 AND 256
			AND instr(attached_by, char(0)) = 0),
		CHECK(note = trim(note) AND length(note) BETWEEN 1 AND 2048
			AND length(CAST(note AS BLOB)) <= 8192 AND instr(note, char(0)) = 0)
	);`,
	`CREATE INDEX idx_finding_remediation_evidence_report_finding
		ON finding_remediation_evidence(report_id, finding_id, ordinal);`,
	`CREATE TABLE finding_remediation_evidence_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		evidence_id TEXT NOT NULL UNIQUE,
		acceptance_id TEXT NOT NULL,
		finding_id TEXT NOT NULL,
		artifact_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		attached_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(evidence_id) REFERENCES finding_remediation_evidence(id) ON DELETE RESTRICT,
		FOREIGN KEY(acceptance_id) REFERENCES finding_acceptance_decisions(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(artifact_id) REFERENCES run_artifacts(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(attached_by = trim(attached_by) AND length(attached_by) BETWEEN 1 AND 256
			AND instr(attached_by, char(0)) = 0)
	);`,
	`CREATE TABLE finding_fix_decisions (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		finding_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		acceptance_id TEXT NOT NULL UNIQUE,
		from_status TEXT NOT NULL,
		status TEXT NOT NULL,
		remediation_evidence_count INTEGER NOT NULL,
		remediation_evidence_digest TEXT NOT NULL,
		decided_by TEXT NOT NULL,
		reason TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES finding_reports(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(acceptance_id) REFERENCES finding_acceptance_decisions(id) ON DELETE RESTRICT,
		CHECK(from_status = 'accepted'),
		CHECK(status = 'fixed'),
		CHECK(remediation_evidence_count BETWEEN 1 AND 64),
		CHECK(length(remediation_evidence_digest) = 64
			AND remediation_evidence_digest = lower(remediation_evidence_digest)
			AND remediation_evidence_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(decided_by = trim(decided_by) AND length(decided_by) BETWEEN 1 AND 256
			AND instr(decided_by, char(0)) = 0),
		CHECK(reason = trim(reason) AND length(reason) BETWEEN 1 AND 2048
			AND length(CAST(reason AS BLOB)) <= 8192 AND instr(reason, char(0)) = 0),
		CHECK(version = 1)
	);`,
	`CREATE INDEX idx_finding_fix_run_created
		ON finding_fix_decisions(run_id, created_at, id);`,
	`CREATE TABLE finding_fix_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		fix_id TEXT NOT NULL UNIQUE,
		acceptance_id TEXT NOT NULL,
		finding_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		decided_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(fix_id) REFERENCES finding_fix_decisions(id) ON DELETE RESTRICT,
		FOREIGN KEY(acceptance_id) REFERENCES finding_acceptance_decisions(id) ON DELETE RESTRICT,
		FOREIGN KEY(finding_id) REFERENCES findings(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(decided_by = trim(decided_by) AND length(decided_by) BETWEEN 1 AND 256
			AND instr(decided_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER trg_finding_acceptance_insert
		BEFORE INSERT ON finding_acceptance_decisions
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_reports report
			JOIN findings finding ON finding.report_id = report.id
			JOIN finding_validation_decisions validation
				ON validation.finding_id = finding.id
			WHERE report.id = NEW.report_id AND report.run_id = NEW.run_id
				AND report.status = 'generated' AND finding.id = NEW.finding_id
				AND finding.run_id = NEW.run_id AND validation.id = NEW.validation_id
				AND validation.status = NEW.from_status
				AND julianday(NEW.created_at) >= julianday(validation.created_at)
				AND NEW.validation_artifact_evidence_count =
					validation.artifact_evidence_count
				AND NEW.validation_artifact_evidence_digest =
					validation.artifact_evidence_digest
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding acceptance binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_acceptance_operation_insert
		BEFORE INSERT ON finding_acceptance_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_acceptance_decisions acceptance
			WHERE acceptance.id = NEW.acceptance_id
				AND acceptance.validation_id = NEW.validation_id
				AND acceptance.finding_id = NEW.finding_id
				AND acceptance.run_id = NEW.run_id
				AND acceptance.decided_by = NEW.decided_by
				AND acceptance.created_at = NEW.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding acceptance operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_remediation_evidence_insert
		BEFORE INSERT ON finding_remediation_evidence
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_acceptance_decisions acceptance
			JOIN finding_reports report ON report.id = acceptance.report_id
			JOIN findings finding ON finding.id = acceptance.finding_id
			JOIN run_artifacts artifact ON artifact.id = NEW.artifact_id
			WHERE acceptance.id = NEW.acceptance_id
				AND acceptance.finding_id = NEW.finding_id
				AND acceptance.run_id = NEW.run_id
				AND report.id = NEW.report_id AND report.run_id = NEW.run_id
				AND report.status = 'generated' AND finding.run_id = NEW.run_id
				AND artifact.run_id = NEW.run_id
				AND artifact.sha256 = NEW.artifact_sha256
				AND artifact.size_bytes = NEW.artifact_size_bytes
				AND artifact.mime = NEW.artifact_mime
				AND artifact.stream = NEW.artifact_stream
				AND artifact.tool_name = NEW.artifact_tool
				AND artifact.source_id = NEW.artifact_source_id
				AND artifact.redacted = NEW.artifact_redacted
				AND julianday(NEW.created_at) >= julianday(acceptance.created_at)
				AND julianday(NEW.created_at) >= julianday(artifact.created_at)
				AND NEW.ordinal = 1 + (SELECT COUNT(*)
					FROM finding_remediation_evidence existing
					WHERE existing.finding_id = NEW.finding_id)
				AND NOT EXISTS (SELECT 1 FROM finding_artifact_evidence validation_evidence
					WHERE validation_evidence.finding_id = NEW.finding_id
						AND validation_evidence.artifact_id = NEW.artifact_id)
				AND NOT EXISTS (SELECT 1 FROM finding_fix_decisions fix
					WHERE fix.finding_id = NEW.finding_id)
				AND (SELECT sequence FROM run_events
					WHERE run_id = NEW.run_id AND type = 'artifact.created'
						AND subject_id = NEW.artifact_id
					ORDER BY sequence DESC LIMIT 1) >
					(SELECT sequence FROM run_events
					WHERE run_id = NEW.run_id AND type = 'finding.accepted'
						AND subject_id = NEW.acceptance_id
					ORDER BY sequence DESC LIMIT 1)
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding remediation Evidence binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_remediation_evidence_operation_insert
		BEFORE INSERT ON finding_remediation_evidence_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_remediation_evidence evidence
			WHERE evidence.id = NEW.evidence_id
				AND evidence.acceptance_id = NEW.acceptance_id
				AND evidence.finding_id = NEW.finding_id
				AND evidence.artifact_id = NEW.artifact_id
				AND evidence.run_id = NEW.run_id
				AND evidence.attached_by = NEW.attached_by
				AND evidence.created_at = NEW.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding remediation Evidence operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_fix_insert
		BEFORE INSERT ON finding_fix_decisions
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_acceptance_decisions acceptance
			JOIN finding_reports report ON report.id = acceptance.report_id
			JOIN findings finding ON finding.id = acceptance.finding_id
			WHERE acceptance.id = NEW.acceptance_id
				AND acceptance.finding_id = NEW.finding_id
				AND acceptance.run_id = NEW.run_id
				AND report.id = NEW.report_id AND report.run_id = NEW.run_id
				AND report.status = 'generated' AND finding.run_id = NEW.run_id
				AND acceptance.status = NEW.from_status
				AND julianday(NEW.created_at) >= julianday(acceptance.created_at)
				AND julianday(NEW.created_at) >= (SELECT MAX(julianday(evidence.created_at))
					FROM finding_remediation_evidence evidence
					WHERE evidence.finding_id = NEW.finding_id)
				AND NEW.remediation_evidence_count = (SELECT COUNT(*)
					FROM finding_remediation_evidence evidence
					WHERE evidence.finding_id = NEW.finding_id)
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding fix binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_fix_operation_insert
		BEFORE INSERT ON finding_fix_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM finding_fix_decisions fix
			WHERE fix.id = NEW.fix_id
				AND fix.acceptance_id = NEW.acceptance_id
				AND fix.finding_id = NEW.finding_id
				AND fix.run_id = NEW.run_id
				AND fix.decided_by = NEW.decided_by
				AND fix.created_at = NEW.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'finding fix operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_finding_acceptance_update_immutable
		BEFORE UPDATE ON finding_acceptance_decisions BEGIN
			SELECT RAISE(ABORT, 'finding acceptance cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_acceptance_delete_immutable
		BEFORE DELETE ON finding_acceptance_decisions BEGIN
			SELECT RAISE(ABORT, 'finding acceptance cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_acceptance_operation_update_immutable
		BEFORE UPDATE ON finding_acceptance_operations BEGIN
			SELECT RAISE(ABORT, 'finding acceptance operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_acceptance_operation_delete_immutable
		BEFORE DELETE ON finding_acceptance_operations BEGIN
			SELECT RAISE(ABORT, 'finding acceptance operation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_remediation_evidence_update_immutable
		BEFORE UPDATE ON finding_remediation_evidence BEGIN
			SELECT RAISE(ABORT, 'finding remediation Evidence cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_remediation_evidence_delete_immutable
		BEFORE DELETE ON finding_remediation_evidence BEGIN
			SELECT RAISE(ABORT, 'finding remediation Evidence cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_remediation_evidence_operation_update_immutable
		BEFORE UPDATE ON finding_remediation_evidence_operations BEGIN
			SELECT RAISE(ABORT, 'finding remediation Evidence operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_remediation_evidence_operation_delete_immutable
		BEFORE DELETE ON finding_remediation_evidence_operations BEGIN
			SELECT RAISE(ABORT, 'finding remediation Evidence operation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_fix_update_immutable
		BEFORE UPDATE ON finding_fix_decisions BEGIN
			SELECT RAISE(ABORT, 'finding fix cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_fix_delete_immutable
		BEFORE DELETE ON finding_fix_decisions BEGIN
			SELECT RAISE(ABORT, 'finding fix cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_finding_fix_operation_update_immutable
		BEFORE UPDATE ON finding_fix_operations BEGIN
			SELECT RAISE(ABORT, 'finding fix operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_finding_fix_operation_delete_immutable
		BEFORE DELETE ON finding_fix_operations BEGIN
			SELECT RAISE(ABORT, 'finding fix operation cannot be deleted');
		END;`,
}

var specialistOperatorScheduleStatements = []string{
	`CREATE TABLE specialist_operator_schedule_requests (
		id TEXT PRIMARY KEY,
		application_id TEXT NOT NULL,
		proposal_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		root_agent_id TEXT NOT NULL,
		max_rounds INTEGER NOT NULL,
		agent_count INTEGER NOT NULL,
		policy_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(application_id) REFERENCES specialist_delegation_applications(id) ON DELETE RESTRICT,
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(root_agent_id) REFERENCES agent_nodes(id) ON DELETE RESTRICT,
		CHECK(max_rounds BETWEEN 1 AND 32),
		CHECK(agent_count BETWEEN 1 AND 2),
		CHECK(length(policy_fingerprint) = 64
			AND policy_fingerprint = lower(policy_fingerprint)
			AND policy_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by)
			AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_specialist_operator_schedule_request_application
		ON specialist_operator_schedule_requests(application_id, created_at, id);`,
	`CREATE INDEX idx_specialist_operator_schedule_request_run
		ON specialist_operator_schedule_requests(run_id, created_at, id);`,
	`CREATE TABLE specialist_operator_schedule_request_agents (
		request_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		PRIMARY KEY(request_id, ordinal),
		UNIQUE(request_id, agent_id),
		FOREIGN KEY(request_id) REFERENCES specialist_operator_schedule_requests(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(agent_id) REFERENCES agent_nodes(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 2)
	);`,
	`CREATE INDEX idx_specialist_operator_schedule_request_agent
		ON specialist_operator_schedule_request_agents(run_id, agent_id, request_id);`,
	`CREATE TABLE specialist_operator_schedule_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		request_id TEXT NOT NULL UNIQUE,
		application_id TEXT NOT NULL,
		proposal_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(request_id) REFERENCES specialist_operator_schedule_requests(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_id) REFERENCES specialist_delegation_applications(id) ON DELETE RESTRICT,
		FOREIGN KEY(proposal_id) REFERENCES specialist_delegation_proposals(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by)
			AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE TABLE specialist_operator_schedule_attempts (
		request_id TEXT NOT NULL,
		schedule_id TEXT NOT NULL UNIQUE,
		ordinal INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(request_id, ordinal),
		FOREIGN KEY(request_id) REFERENCES specialist_operator_schedule_requests(id) ON DELETE RESTRICT,
		FOREIGN KEY(schedule_id) REFERENCES specialist_schedules(id) ON DELETE RESTRICT,
		CHECK(ordinal > 0)
	);`,
	`CREATE TRIGGER trg_specialist_operator_schedule_request_insert
		BEFORE INSERT ON specialist_operator_schedule_requests
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_delegation_applications application
			JOIN specialist_delegation_proposals proposal
				ON proposal.id = application.proposal_id
			JOIN runs run ON run.id = application.run_id
			JOIN agent_nodes root ON root.id = application.root_agent_id
			WHERE application.id = NEW.application_id
				AND application.proposal_id = NEW.proposal_id
				AND application.run_id = NEW.run_id
				AND application.root_agent_id = NEW.root_agent_id
				AND application.requested_by = NEW.requested_by
				AND application.status = 'applied'
				AND application.completed_at IS NOT NULL
				AND julianday(NEW.created_at) >= julianday(application.completed_at)
				AND proposal.run_id = NEW.run_id
				AND proposal.root_agent_id = NEW.root_agent_id
				AND run.status = 'running'
				AND root.run_id = NEW.run_id AND root.role = 'root'
				AND root.status = 'ready' AND root.active_attempt_id = ''
				AND NOT EXISTS (SELECT 1 FROM specialist_schedules schedule
					WHERE schedule.run_id = NEW.run_id AND schedule.status = 'running')
		)
		BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule request binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_request_agent_insert
		BEFORE INSERT ON specialist_operator_schedule_request_agents
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_operator_schedule_requests request
			JOIN specialist_delegation_application_assignments assignment
				ON assignment.application_id = request.application_id
			JOIN agent_nodes agent ON agent.id = assignment.agent_id
			WHERE request.id = NEW.request_id AND request.run_id = NEW.run_id
				AND assignment.status = 'instructed'
				AND assignment.agent_id = NEW.agent_id
				AND agent.run_id = NEW.run_id AND agent.role = 'specialist'
				AND agent.parent_id = request.root_agent_id
				AND agent.status = 'ready' AND agent.active_attempt_id = ''
				AND NEW.ordinal = 1 + (SELECT COUNT(*)
					FROM specialist_operator_schedule_request_agents existing
					WHERE existing.request_id = NEW.request_id)
				AND NOT EXISTS (
					SELECT 1 FROM specialist_operator_schedule_request_agents reserved
					WHERE reserved.run_id = NEW.run_id
						AND reserved.agent_id = NEW.agent_id
						AND reserved.request_id != NEW.request_id
						AND (
							NOT EXISTS (SELECT 1 FROM specialist_operator_schedule_attempts attempt
								WHERE attempt.request_id = reserved.request_id)
							OR COALESCE((SELECT schedule.status
								FROM specialist_operator_schedule_attempts attempt
								JOIN specialist_schedules schedule ON schedule.id = attempt.schedule_id
								WHERE attempt.request_id = reserved.request_id
								ORDER BY attempt.ordinal DESC LIMIT 1), '')
								IN ('running', 'abandoned')
						)
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule Agent binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_operation_insert
		BEFORE INSERT ON specialist_operator_schedule_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_operator_schedule_requests request
			WHERE request.id = NEW.request_id
				AND request.application_id = NEW.application_id
				AND request.proposal_id = NEW.proposal_id
				AND request.run_id = NEW.run_id
				AND request.requested_by = NEW.requested_by
				AND request.created_at = NEW.created_at
				AND request.agent_count = (SELECT COUNT(*)
					FROM specialist_operator_schedule_request_agents agent
					WHERE agent.request_id = request.id)
		)
		BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_attempt_insert
		BEFORE INSERT ON specialist_operator_schedule_attempts
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_operator_schedule_requests request
			JOIN specialist_operator_schedule_operations operation
				ON operation.request_id = request.id
			JOIN specialist_schedules schedule ON schedule.id = NEW.schedule_id
			WHERE request.id = NEW.request_id
				AND schedule.run_id = request.run_id
				AND schedule.max_rounds = request.max_rounds
				AND schedule.status = 'running'
				AND schedule.started_at = NEW.created_at
				AND NEW.ordinal = 1 + (SELECT COUNT(*)
					FROM specialist_operator_schedule_attempts existing
					WHERE existing.request_id = NEW.request_id)
				AND (NEW.ordinal = 1 OR (SELECT previous.status
					FROM specialist_operator_schedule_attempts prior
					JOIN specialist_schedules previous ON previous.id = prior.schedule_id
					WHERE prior.request_id = NEW.request_id
					ORDER BY prior.ordinal DESC LIMIT 1) = 'abandoned')
				AND request.agent_count = (SELECT COUNT(*)
					FROM specialist_schedule_agents target
					WHERE target.schedule_id = NEW.schedule_id)
				AND NOT EXISTS (
					SELECT 1 FROM specialist_operator_schedule_request_agents requested
					LEFT JOIN specialist_schedule_agents target
						ON target.schedule_id = NEW.schedule_id
						AND target.agent_id = requested.agent_id
					WHERE requested.request_id = NEW.request_id
						AND target.agent_id IS NULL)
				AND NOT EXISTS (
					SELECT 1 FROM specialist_schedule_agents target
					LEFT JOIN specialist_operator_schedule_request_agents requested
						ON requested.request_id = NEW.request_id
						AND requested.agent_id = target.agent_id
					WHERE target.schedule_id = NEW.schedule_id
						AND requested.agent_id IS NULL)
		)
		BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule attempt binding is invalid');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_request_update_immutable
		BEFORE UPDATE ON specialist_operator_schedule_requests BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule request cannot be updated');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_request_delete_immutable
		BEFORE DELETE ON specialist_operator_schedule_requests BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule request cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_request_agent_update_immutable
		BEFORE UPDATE ON specialist_operator_schedule_request_agents BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule Agent cannot be updated');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_request_agent_delete_immutable
		BEFORE DELETE ON specialist_operator_schedule_request_agents BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule Agent cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_operation_update_immutable
		BEFORE UPDATE ON specialist_operator_schedule_operations BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_operation_delete_immutable
		BEFORE DELETE ON specialist_operator_schedule_operations BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule operation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_attempt_update_immutable
		BEFORE UPDATE ON specialist_operator_schedule_attempts BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule attempt cannot be updated');
		END;`,
	`CREATE TRIGGER trg_specialist_operator_schedule_attempt_delete_immutable
		BEFORE DELETE ON specialist_operator_schedule_attempts BEGIN
			SELECT RAISE(ABORT, 'specialist operator schedule attempt cannot be deleted');
		END;`,
}

var skillSelectionStatements = []string{
	`CREATE TABLE run_skill_selections (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL UNIQUE,
		mission_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		profile TEXT NOT NULL,
		token_budget INTEGER NOT NULL,
		token_upper_bound INTEGER NOT NULL,
		item_count INTEGER NOT NULL,
		selection_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'skill_selection.v1'),
		CHECK(profile IN ('code', 'review', 'learn', 'script')),
		CHECK(token_budget BETWEEN 1 AND 8192),
		CHECK(token_upper_bound BETWEEN 1 AND token_budget),
		CHECK(item_count BETWEEN 1 AND 8),
		CHECK(length(selection_fingerprint) = 64
			AND selection_fingerprint = lower(selection_fingerprint)
			AND selection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256 AND instr(mission_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_run_skill_selections_mission
		ON run_skill_selections(mission_id, created_at, id);`,
	`CREATE TABLE run_skill_selection_items (
		selection_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		content_sha256 TEXT NOT NULL,
		content_bytes INTEGER NOT NULL,
		token_upper_bound INTEGER NOT NULL,
		PRIMARY KEY(selection_id, ordinal),
		UNIQUE(selection_id, name),
		FOREIGN KEY(selection_id) REFERENCES run_skill_selections(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 8),
		CHECK(length(name) BETWEEN 1 AND 64 AND name = lower(name)
			AND substr(name, 1, 1) GLOB '[a-z]' AND substr(name, -1, 1) != '-'
			AND name NOT GLOB '*[^a-z0-9-]*'),
		CHECK(length(version) BETWEEN 5 AND 29 AND version = trim(version)),
		CHECK(length(content_sha256) = 64 AND content_sha256 = lower(content_sha256)
			AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(content_bytes BETWEEN 1 AND 4096),
		CHECK(token_upper_bound BETWEEN 1 AND 4096),
		CHECK(token_upper_bound = content_bytes)
	);`,
	`CREATE TABLE run_skill_selection_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		selection_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL UNIQUE,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(selection_id) REFERENCES run_skill_selections(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER trg_run_skill_selection_insert
		BEFORE INSERT ON run_skill_selections
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			WHERE run.id = NEW.run_id AND run.mission_id = NEW.mission_id
				AND run.status = 'created' AND mission.profile = NEW.profile
				AND julianday(NEW.created_at) >= julianday(run.created_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Skill selection Run binding is invalid');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_item_insert
		BEFORE INSERT ON run_skill_selection_items
		WHEN NOT EXISTS (
			SELECT 1 FROM run_skill_selections selection
			WHERE selection.id = NEW.selection_id
				AND NEW.ordinal = 1 + (SELECT COUNT(*) FROM run_skill_selection_items existing
					WHERE existing.selection_id = NEW.selection_id)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Skill selection item binding is invalid');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_operation_insert
		BEFORE INSERT ON run_skill_selection_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM run_skill_selections selection
			WHERE selection.id = NEW.selection_id AND selection.run_id = NEW.run_id
				AND selection.requested_by = NEW.requested_by
				AND selection.created_at = NEW.created_at
				AND selection.item_count = (SELECT COUNT(*) FROM run_skill_selection_items item
					WHERE item.selection_id = selection.id)
				AND selection.token_upper_bound = (SELECT COALESCE(SUM(item.token_upper_bound), 0)
					FROM run_skill_selection_items item WHERE item.selection_id = selection.id)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Skill selection operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_update_immutable
		BEFORE UPDATE ON run_skill_selections BEGIN
			SELECT RAISE(ABORT, 'Skill selection cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_delete_immutable
		BEFORE DELETE ON run_skill_selections BEGIN
			SELECT RAISE(ABORT, 'Skill selection cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_item_update_immutable
		BEFORE UPDATE ON run_skill_selection_items BEGIN
			SELECT RAISE(ABORT, 'Skill selection item cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_item_delete_immutable
		BEFORE DELETE ON run_skill_selection_items BEGIN
			SELECT RAISE(ABORT, 'Skill selection item cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_operation_update_immutable
		BEFORE UPDATE ON run_skill_selection_operations BEGIN
			SELECT RAISE(ABORT, 'Skill selection operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_skill_selection_operation_delete_immutable
		BEFORE DELETE ON run_skill_selection_operations BEGIN
			SELECT RAISE(ABORT, 'Skill selection operation cannot be deleted');
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
