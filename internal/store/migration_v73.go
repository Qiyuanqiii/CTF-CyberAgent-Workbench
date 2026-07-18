package store

var controlledRunOperationsStatements = []string{
	`CREATE TABLE run_lifecycle_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		run_id TEXT NOT NULL,
		action TEXT NOT NULL,
		expected_status TEXT NOT NULL,
		applied_status TEXT NOT NULL,
		event_sequence_start INTEGER NOT NULL,
		event_sequence_end INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'run_lifecycle_control.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK((action = 'start' AND expected_status = 'created' AND applied_status = 'running'
			AND event_sequence_end = event_sequence_start + 1)
			OR (action = 'pause' AND expected_status = 'running' AND applied_status = 'paused'
				AND event_sequence_end = event_sequence_start)
			OR (action = 'resume' AND expected_status = 'paused' AND applied_status = 'running'
				AND event_sequence_end = event_sequence_start)),
		CHECK(event_sequence_start > 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_run_lifecycle_operations_run_created
		ON run_lifecycle_operations(run_id, created_at);`,
	`CREATE TRIGGER trg_run_lifecycle_operation_insert
		BEFORE INSERT ON run_lifecycle_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			WHERE run.id = NEW.run_id AND run.status = NEW.applied_status
				AND NOT EXISTS (SELECT 1 FROM run_execution_leases lease
					WHERE lease.run_id = run.id AND lease.status = 'active'
						AND julianday(lease.expires_at) > julianday(NEW.created_at))
				AND ((NEW.action = 'start'
					AND EXISTS (SELECT 1 FROM run_events first_event
						WHERE first_event.run_id = run.id AND first_event.sequence = NEW.event_sequence_start
							AND first_event.type = 'run.status_changed'
							AND first_event.source = 'run_lifecycle_control'
							AND json_extract(first_event.payload_json, '$.from') = 'created'
							AND json_extract(first_event.payload_json, '$.to') = 'preparing'
							AND json_extract(first_event.payload_json, '$.action') = NEW.action
							AND first_event.created_at = NEW.created_at)
					AND EXISTS (SELECT 1 FROM run_events second_event
						WHERE second_event.run_id = run.id AND second_event.sequence = NEW.event_sequence_end
							AND second_event.type = 'run.status_changed'
							AND second_event.source = 'run_lifecycle_control'
							AND json_extract(second_event.payload_json, '$.from') = 'preparing'
							AND json_extract(second_event.payload_json, '$.to') = 'running'
							AND json_extract(second_event.payload_json, '$.action') = NEW.action
							AND second_event.created_at = NEW.created_at))
				OR (NEW.action IN ('pause', 'resume')
					AND EXISTS (SELECT 1 FROM run_events event
						WHERE event.run_id = run.id AND event.sequence = NEW.event_sequence_start
							AND event.type = 'run.status_changed'
							AND event.source = 'run_lifecycle_control'
							AND json_extract(event.payload_json, '$.from') = NEW.expected_status
							AND json_extract(event.payload_json, '$.to') = NEW.applied_status
							AND json_extract(event.payload_json, '$.action') = NEW.action
							AND event.created_at = NEW.created_at)))
		)
		BEGIN SELECT RAISE(ABORT, 'Run lifecycle operation binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_lifecycle_operation_update_immutable
		BEFORE UPDATE ON run_lifecycle_operations BEGIN
			SELECT RAISE(ABORT, 'Run lifecycle operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_lifecycle_operation_delete_immutable
		BEFORE DELETE ON run_lifecycle_operations BEGIN
			SELECT RAISE(ABORT, 'Run lifecycle operation cannot be deleted');
		END;`,
	`CREATE TABLE run_execution_handoff_operations (
		id TEXT PRIMARY KEY,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		max_steps INTEGER NOT NULL,
		selected_count INTEGER NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'run_execution_handoff.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(max_steps BETWEEN 1 AND 8 AND selected_count BETWEEN 0 AND max_steps),
		CHECK(event_sequence > 0),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(session_id = trim(session_id) AND length(session_id) BETWEEN 1 AND 256
			AND instr(session_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_run_execution_handoff_operations_run_created
		ON run_execution_handoff_operations(run_id, created_at);`,
	`CREATE TABLE run_execution_handoff_items (
		operation_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		message_id TEXT NOT NULL,
		message_sequence INTEGER NOT NULL,
		prepared INTEGER NOT NULL,
		PRIMARY KEY(operation_id, ordinal),
		FOREIGN KEY(operation_id) REFERENCES run_execution_handoff_operations(id) ON DELETE RESTRICT,
		FOREIGN KEY(message_id) REFERENCES operator_steering_messages(id) ON DELETE RESTRICT,
		CHECK(ordinal > 0 AND message_sequence > 0 AND prepared IN (0, 1))
	) WITHOUT ROWID;`,
	`CREATE TABLE run_execution_handoff_results (
		operation_id TEXT PRIMARY KEY,
		status TEXT NOT NULL,
		run_status TEXT NOT NULL,
		stop_reason TEXT NOT NULL,
		error_code TEXT NOT NULL,
		steps_completed INTEGER NOT NULL,
		model_called INTEGER NOT NULL,
		tool_called INTEGER NOT NULL,
		pending_count INTEGER NOT NULL,
		prepared_count INTEGER NOT NULL,
		committed_count INTEGER NOT NULL,
		cancelled_count INTEGER NOT NULL,
		completion_event_sequence INTEGER NOT NULL,
		lease_id TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		completed_at TEXT NOT NULL,
		FOREIGN KEY(operation_id) REFERENCES run_execution_handoff_operations(id) ON DELETE RESTRICT,
		CHECK(status IN ('completed', 'failed')),
		CHECK(run_status IN ('created', 'preparing', 'running', 'waiting_approval', 'paused',
			'completed', 'failed', 'cancelled')),
		CHECK(stop_reason = trim(stop_reason) AND length(stop_reason) BETWEEN 1 AND 64
			AND instr(stop_reason, char(0)) = 0),
		CHECK(error_code = trim(error_code) AND length(error_code) <= 64 AND instr(error_code, char(0)) = 0),
		CHECK((status = 'completed' AND error_code = '') OR (status = 'failed' AND error_code <> '')),
		CHECK(steps_completed >= 0 AND model_called IN (0, 1) AND tool_called IN (0, 1)
			AND tool_called <= model_called AND pending_count >= 0 AND prepared_count >= 0
			AND committed_count >= 0 AND cancelled_count >= 0),
		CHECK(completion_event_sequence > 0),
		CHECK((lease_id = '' AND lease_generation = 0) OR
			(lease_id = trim(lease_id) AND length(lease_id) BETWEEN 1 AND 256
				AND instr(lease_id, char(0)) = 0 AND lease_generation > 0))
	);`,
	`CREATE TRIGGER trg_run_execution_handoff_operation_insert
		BEFORE INSERT ON run_execution_handoff_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
				AND run.status = 'running' AND session_record.status = 'active'
				AND event.type = 'run.execution_handoff_requested'
				AND event.source = 'run_execution_handoff' AND event.subject_id = NEW.id
				AND json_extract(event.payload_json, '$.max_steps') = NEW.max_steps
				AND json_extract(event.payload_json, '$.selected_count') = NEW.selected_count
				AND event.created_at = NEW.created_at
				AND NOT EXISTS (SELECT 1 FROM run_execution_leases lease
					WHERE lease.run_id = run.id AND lease.status = 'active'
						AND julianday(lease.expires_at) > julianday(NEW.created_at))
		)
		BEGIN SELECT RAISE(ABORT, 'Run execution handoff operation binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_execution_handoff_item_insert
		BEFORE INSERT ON run_execution_handoff_items
		WHEN NOT EXISTS (
			SELECT 1 FROM run_execution_handoff_operations operation
			JOIN operator_steering_messages message ON message.id = NEW.message_id
			WHERE operation.id = NEW.operation_id AND NEW.ordinal <= operation.selected_count
				AND message.run_id = operation.run_id AND message.session_id = operation.session_id
				AND message.sequence = NEW.message_sequence AND message.status = 'pending'
				AND NEW.prepared = EXISTS (SELECT 1 FROM operator_steering_deliveries delivery
					WHERE delivery.message_id = message.id AND delivery.status = 'prepared')
		)
		BEGIN SELECT RAISE(ABORT, 'Run execution handoff item binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_execution_handoff_result_insert
		BEFORE INSERT ON run_execution_handoff_results
		WHEN NOT EXISTS (
			SELECT 1 FROM run_execution_handoff_operations operation
			JOIN runs run ON run.id = operation.run_id
			JOIN run_events event ON event.run_id = run.id
				AND event.sequence = NEW.completion_event_sequence
			WHERE operation.id = NEW.operation_id AND run.status = NEW.run_status
				AND NEW.steps_completed <= operation.selected_count
				AND NEW.pending_count + NEW.prepared_count + NEW.committed_count +
					NEW.cancelled_count = operation.selected_count
				AND (SELECT COUNT(*) FROM run_execution_handoff_items item
					WHERE item.operation_id = operation.id) = operation.selected_count
				AND NEW.pending_count = (SELECT COUNT(*) FROM run_execution_handoff_items item
					JOIN operator_steering_messages message ON message.id = item.message_id
					WHERE item.operation_id = operation.id AND message.status = 'pending'
						AND NOT EXISTS (SELECT 1 FROM operator_steering_deliveries delivery
							WHERE delivery.message_id = message.id AND delivery.status = 'prepared'))
				AND NEW.prepared_count = (SELECT COUNT(*) FROM run_execution_handoff_items item
					JOIN operator_steering_messages message ON message.id = item.message_id
					WHERE item.operation_id = operation.id AND message.status = 'pending'
						AND EXISTS (SELECT 1 FROM operator_steering_deliveries delivery
							WHERE delivery.message_id = message.id AND delivery.status = 'prepared'))
				AND NEW.committed_count = (SELECT COUNT(*) FROM run_execution_handoff_items item
					JOIN operator_steering_messages message ON message.id = item.message_id
					WHERE item.operation_id = operation.id AND message.status = 'committed')
				AND NEW.cancelled_count = (SELECT COUNT(*) FROM run_execution_handoff_items item
					JOIN operator_steering_messages message ON message.id = item.message_id
					WHERE item.operation_id = operation.id AND message.status = 'cancelled')
				AND event.type = 'run.execution_handoff_completed'
				AND event.source = 'run_execution_handoff' AND event.subject_id = operation.id
				AND json_extract(event.payload_json, '$.status') = NEW.status
				AND json_extract(event.payload_json, '$.stop_reason') = NEW.stop_reason
				AND json_extract(event.payload_json, '$.error_code') = NEW.error_code
				AND json_extract(event.payload_json, '$.steps_completed') = NEW.steps_completed
				AND json_extract(event.payload_json, '$.selected_count') = operation.selected_count
				AND json_extract(event.payload_json, '$.pending_count') = NEW.pending_count
				AND json_extract(event.payload_json, '$.prepared_count') = NEW.prepared_count
				AND json_extract(event.payload_json, '$.committed_count') = NEW.committed_count
				AND json_extract(event.payload_json, '$.cancelled_count') = NEW.cancelled_count
				AND json_extract(event.payload_json, '$.model_called') = NEW.model_called
				AND json_extract(event.payload_json, '$.tool_called') = NEW.tool_called
				AND event.created_at = NEW.completed_at
				AND ((operation.selected_count = 0 AND NEW.lease_id = '' AND NEW.lease_generation = 0)
					OR (operation.selected_count > 0 AND EXISTS (
						SELECT 1 FROM run_execution_leases lease
						WHERE lease.run_id = run.id AND lease.lease_id = NEW.lease_id
							AND lease.generation = NEW.lease_generation AND lease.status = 'active'
							AND julianday(lease.expires_at) > julianday(NEW.completed_at))))
		)
		BEGIN SELECT RAISE(ABORT, 'Run execution handoff result binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_execution_handoff_operation_update_immutable
		BEFORE UPDATE ON run_execution_handoff_operations BEGIN
			SELECT RAISE(ABORT, 'Run execution handoff operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_execution_handoff_operation_delete_immutable
		BEFORE DELETE ON run_execution_handoff_operations BEGIN
			SELECT RAISE(ABORT, 'Run execution handoff operation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_run_execution_handoff_item_update_immutable
		BEFORE UPDATE ON run_execution_handoff_items BEGIN
			SELECT RAISE(ABORT, 'Run execution handoff item cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_execution_handoff_item_delete_immutable
		BEFORE DELETE ON run_execution_handoff_items BEGIN
			SELECT RAISE(ABORT, 'Run execution handoff item cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_run_execution_handoff_result_update_immutable
		BEFORE UPDATE ON run_execution_handoff_results BEGIN
			SELECT RAISE(ABORT, 'Run execution handoff result cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_execution_handoff_result_delete_immutable
		BEFORE DELETE ON run_execution_handoff_results BEGIN
			SELECT RAISE(ABORT, 'Run execution handoff result cannot be deleted');
		END;`,
}
