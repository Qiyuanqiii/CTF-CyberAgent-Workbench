package store

var fileEditApplyStatements = []string{
	`CREATE TABLE file_edit_apply_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		edit_id TEXT NOT NULL UNIQUE,
		path TEXT NOT NULL,
		original_hash TEXT NOT NULL,
		proposed_hash TEXT NOT NULL,
		observed_hash TEXT NOT NULL,
		applied_by TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		FOREIGN KEY(edit_id) REFERENCES file_edits(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'file_edit_apply.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(original_hash = 'missing' OR (length(original_hash) = 64
			AND original_hash = lower(original_hash) AND original_hash NOT GLOB '*[^0-9a-f]*')),
		CHECK(length(proposed_hash) = 64 AND proposed_hash = lower(proposed_hash)
			AND proposed_hash NOT GLOB '*[^0-9a-f]*'),
		CHECK(observed_hash IN (original_hash, proposed_hash)),
		CHECK(event_sequence > 0),
		CHECK(path = trim(path) AND length(path) BETWEEN 1 AND 256 AND instr(path, char(0)) = 0),
		CHECK(applied_by = trim(applied_by) AND length(applied_by) BETWEEN 1 AND 256
			AND instr(applied_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_file_edit_apply_operations_run_created
		ON file_edit_apply_operations(run_id, created_at);`,
	`CREATE TABLE file_edit_apply_results (
		operation_key_digest TEXT PRIMARY KEY,
		status TEXT NOT NULL,
		reason_code TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		completed_at TEXT NOT NULL,
		FOREIGN KEY(operation_key_digest) REFERENCES file_edit_apply_operations(operation_key_digest)
			ON DELETE RESTRICT,
		CHECK(status IN ('applied', 'failed')),
		CHECK((status = 'applied' AND reason_code = '') OR
			(status = 'failed' AND length(reason_code) BETWEEN 1 AND 64)),
		CHECK(reason_code = trim(reason_code) AND instr(reason_code, char(0)) = 0),
		CHECK(event_sequence > 0)
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_file_edit_apply_operation_insert
		BEFORE INSERT ON file_edit_apply_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN file_edits edit ON edit.id = NEW.edit_id
			JOIN tool_approvals approval ON approval.proposal_id = edit.id
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
				AND run.status = 'running' AND session_record.status = 'active'
				AND mission.workspace_id = NEW.workspace_id
				AND edit.session_id = NEW.session_id AND edit.workspace_id = NEW.workspace_id
				AND edit.status = 'approved' AND edit.path = NEW.path
				AND edit.original_hash = NEW.original_hash
				AND edit.proposed_hash = NEW.proposed_hash
				AND NEW.observed_hash IN (edit.original_hash, edit.proposed_hash)
				AND approval.run_id = run.id AND approval.session_id = NEW.session_id
				AND approval.workspace_id = NEW.workspace_id
				AND approval.tool_name = 'replace_file'
				AND approval.action_class = 'workspace_write'
				AND approval.status = 'approved'
				AND event.type = 'file_edit.apply_requested'
				AND event.source = 'file_edit_apply' AND event.subject_id = edit.id
				AND event.created_at = NEW.created_at
				AND json_extract(event.payload_json, '$.operation_key_digest') =
					NEW.operation_key_digest
				AND json_extract(event.payload_json, '$.observed_hash') = NEW.observed_hash
				AND json_extract(event.payload_json, '$.proposed_hash') = NEW.proposed_hash
				AND json_extract(event.payload_json, '$.policy_rechecked') = 1
		)
		BEGIN SELECT RAISE(ABORT, 'FileEdit apply operation binding is invalid'); END;`,
	`CREATE TRIGGER trg_file_edit_apply_operation_update_immutable
		BEFORE UPDATE ON file_edit_apply_operations BEGIN
			SELECT RAISE(ABORT, 'FileEdit apply operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_file_edit_apply_operation_delete_immutable
		BEFORE DELETE ON file_edit_apply_operations BEGIN
			SELECT RAISE(ABORT, 'FileEdit apply operation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_file_edit_apply_result_insert
		BEFORE INSERT ON file_edit_apply_results
		WHEN NOT EXISTS (
			SELECT 1 FROM file_edit_apply_operations operation
			JOIN file_edits edit ON edit.id = operation.edit_id
			JOIN run_events event ON event.run_id = operation.run_id
				AND event.sequence = NEW.event_sequence
			WHERE operation.operation_key_digest = NEW.operation_key_digest
				AND ((NEW.status = 'applied' AND edit.status = 'applied'
					AND edit.proposed_hash = operation.proposed_hash AND NEW.reason_code = '')
					OR (NEW.status = 'failed' AND edit.status = 'failed'
						AND length(NEW.reason_code) BETWEEN 1 AND 64))
				AND event.type = 'file_edit.apply_completed'
				AND event.source = 'file_edit_apply' AND event.subject_id = edit.id
				AND event.created_at = NEW.completed_at
				AND json_extract(event.payload_json, '$.operation_key_digest') =
					NEW.operation_key_digest
				AND json_extract(event.payload_json, '$.status') = NEW.status
				AND json_extract(event.payload_json, '$.reason_code') = NEW.reason_code
		)
		BEGIN SELECT RAISE(ABORT, 'FileEdit apply result binding is invalid'); END;`,
	`CREATE TRIGGER trg_file_edit_apply_result_update_immutable
		BEFORE UPDATE ON file_edit_apply_results BEGIN
			SELECT RAISE(ABORT, 'FileEdit apply result cannot be updated');
		END;`,
	`CREATE TRIGGER trg_file_edit_apply_result_delete_immutable
		BEFORE DELETE ON file_edit_apply_results BEGIN
			SELECT RAISE(ABORT, 'FileEdit apply result cannot be deleted');
		END;`,
}
