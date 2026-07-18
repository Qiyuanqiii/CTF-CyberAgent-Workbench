package store

var runWakeConsumptionStatements = []string{
	`CREATE TABLE run_wake_consumptions (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		intent_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		lease_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		owner_id TEXT NOT NULL,
		handoff_operation_key_digest TEXT NOT NULL UNIQUE,
		max_steps INTEGER NOT NULL,
		status TEXT NOT NULL,
		handoff_operation_id TEXT,
		stop_reason TEXT NOT NULL,
		error_code TEXT NOT NULL,
		prepared_event_sequence INTEGER NOT NULL,
		completion_event_sequence INTEGER,
		created_at TEXT NOT NULL,
		completed_at TEXT,
		FOREIGN KEY(intent_id) REFERENCES run_wake_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(lease_id) REFERENCES run_wake_leases(id) ON DELETE RESTRICT,
		FOREIGN KEY(handoff_operation_id) REFERENCES run_execution_handoff_operations(id)
			ON DELETE RESTRICT,
		UNIQUE(intent_id, generation),
		CHECK(protocol_version = 'run_wake_consumption.v1'),
		CHECK(status IN ('prepared', 'completed', 'failed')),
		CHECK(generation BETWEEN 1 AND 8 AND max_steps BETWEEN 1 AND 8),
		CHECK(length(handoff_operation_key_digest) = 64
			AND handoff_operation_key_digest = lower(handoff_operation_key_digest)
			AND handoff_operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(prepared_event_sequence > 0),
		CHECK((status = 'prepared' AND handoff_operation_id IS NULL
			AND stop_reason = '' AND error_code = ''
			AND completion_event_sequence IS NULL AND completed_at IS NULL)
			OR (status = 'completed' AND handoff_operation_id IS NOT NULL
				AND length(stop_reason) BETWEEN 1 AND 64 AND error_code = ''
				AND completion_event_sequence > 0 AND completed_at IS NOT NULL)
			OR (status = 'failed' AND length(stop_reason) BETWEEN 1 AND 64
				AND length(error_code) BETWEEN 1 AND 64
				AND completion_event_sequence > 0 AND completed_at IS NOT NULL)),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(owner_id = trim(owner_id) AND length(owner_id) BETWEEN 1 AND 256
			AND instr(owner_id, char(0)) = 0),
		CHECK(stop_reason = trim(stop_reason) AND instr(stop_reason, char(0)) = 0),
		CHECK(error_code = trim(error_code) AND instr(error_code, char(0)) = 0)
	);`,
	`CREATE INDEX idx_run_wake_consumptions_intent_created
		ON run_wake_consumptions(intent_id, generation DESC);`,
	`CREATE TRIGGER trg_run_wake_consumption_insert
		BEFORE INSERT ON run_wake_consumptions
		WHEN NEW.status <> 'prepared' OR NEW.handoff_operation_id IS NOT NULL
			OR NEW.stop_reason <> '' OR NEW.error_code <> ''
			OR NEW.completion_event_sequence IS NOT NULL OR NEW.completed_at IS NOT NULL
			OR NOT EXISTS (
				SELECT 1 FROM run_wake_intents intent
				JOIN run_wake_leases lease ON lease.id = intent.active_lease_id
				JOIN run_events event ON event.run_id = intent.run_id
					AND event.sequence = NEW.prepared_event_sequence
				WHERE intent.id = NEW.intent_id AND intent.run_id = NEW.run_id
					AND intent.session_id = NEW.session_id AND intent.status = 'leased'
					AND intent.attempt_count = NEW.generation
					AND lease.id = NEW.lease_id AND lease.generation = NEW.generation
					AND lease.owner_id = NEW.owner_id AND lease.status = 'active'
					AND event.type = 'run.wake_handoff_prepared'
					AND event.source = 'run_wake_consumer' AND event.subject_id = NEW.id
					AND event.created_at = NEW.created_at
					AND json_extract(event.payload_json, '$.intent_id') = NEW.intent_id
					AND json_extract(event.payload_json, '$.generation') = NEW.generation
					AND json_extract(event.payload_json, '$.max_steps') = NEW.max_steps
					AND json_extract(event.payload_json, '$.handoff_key_digest') =
						NEW.handoff_operation_key_digest
			)
		BEGIN SELECT RAISE(ABORT, 'Run wake consumption preparation is invalid'); END;`,
	`CREATE TRIGGER trg_run_wake_consumption_update
		BEFORE UPDATE ON run_wake_consumptions
		WHEN NEW.id <> OLD.id OR NEW.protocol_version <> OLD.protocol_version
			OR NEW.intent_id <> OLD.intent_id OR NEW.run_id <> OLD.run_id
			OR NEW.session_id <> OLD.session_id OR NEW.lease_id <> OLD.lease_id
			OR NEW.generation <> OLD.generation OR NEW.owner_id <> OLD.owner_id
			OR NEW.handoff_operation_key_digest <> OLD.handoff_operation_key_digest
			OR NEW.max_steps <> OLD.max_steps
			OR NEW.prepared_event_sequence <> OLD.prepared_event_sequence
			OR NEW.created_at <> OLD.created_at OR OLD.status <> 'prepared'
			OR NEW.status NOT IN ('completed', 'failed')
			OR NEW.completion_event_sequence IS NULL OR NEW.completed_at IS NULL
			OR NOT EXISTS (
				SELECT 1 FROM run_events event
				WHERE event.run_id = OLD.run_id
					AND event.sequence = NEW.completion_event_sequence
					AND event.created_at = NEW.completed_at
					AND ((NEW.status = 'completed'
						AND event.type = 'run.wake_completed'
						AND event.source = 'run_wake_consumer'
						AND event.subject_id = OLD.id
						AND json_extract(event.payload_json, '$.generation') = OLD.generation
						AND json_extract(event.payload_json, '$.handoff_operation_id') =
							NEW.handoff_operation_id
						AND json_extract(event.payload_json, '$.stop_reason') = NEW.stop_reason
						AND json_extract(event.payload_json, '$.model_called') =
							(SELECT result.model_called FROM run_execution_handoff_results result
								WHERE result.operation_id = NEW.handoff_operation_id)
						AND json_extract(event.payload_json, '$.tool_called') =
							(SELECT result.tool_called FROM run_execution_handoff_results result
								WHERE result.operation_id = NEW.handoff_operation_id))
					OR (NEW.status = 'failed'
						AND event.type IN ('run.wake_retried', 'run.wake_exhausted')
						AND event.source = 'run_wake_coordinator'
						AND event.subject_id = OLD.intent_id
						AND json_extract(event.payload_json, '$.generation') = OLD.generation
						AND json_extract(event.payload_json, '$.stop_reason') = NEW.stop_reason
						AND json_extract(event.payload_json, '$.error_code') = NEW.error_code
						AND json_extract(event.payload_json, '$.handoff_operation_id') =
							coalesce(NEW.handoff_operation_id, '')
						AND json_extract(event.payload_json, '$.execution_started') =
							CASE WHEN NEW.handoff_operation_id IS NULL THEN 0 ELSE 1 END
						AND json_extract(event.payload_json, '$.model_called') = coalesce(
							(SELECT result.model_called FROM run_execution_handoff_results result
								WHERE result.operation_id = NEW.handoff_operation_id), 0)
						AND json_extract(event.payload_json, '$.tool_called') = coalesce(
							(SELECT result.tool_called FROM run_execution_handoff_results result
								WHERE result.operation_id = NEW.handoff_operation_id), 0)))
			)
			OR (NEW.status = 'completed' AND NOT EXISTS (
				SELECT 1 FROM run_execution_handoff_operations operation
				JOIN run_execution_handoff_results result ON result.operation_id = operation.id
				WHERE operation.id = NEW.handoff_operation_id
					AND operation.operation_key_digest = OLD.handoff_operation_key_digest
					AND operation.run_id = OLD.run_id AND operation.session_id = OLD.session_id
					AND operation.requested_by = 'run_wake_consumer'
					AND operation.max_steps = OLD.max_steps AND result.status = 'completed'
					AND result.stop_reason = NEW.stop_reason AND NEW.error_code = ''
			))
			OR (NEW.status = 'failed' AND (NEW.error_code = '' OR
				(NEW.handoff_operation_id IS NOT NULL AND NOT EXISTS (
					SELECT 1 FROM run_execution_handoff_operations operation
					JOIN run_execution_handoff_results result
						ON result.operation_id = operation.id
					WHERE operation.id = NEW.handoff_operation_id
						AND operation.operation_key_digest = OLD.handoff_operation_key_digest
						AND operation.run_id = OLD.run_id
						AND operation.session_id = OLD.session_id
						AND operation.requested_by = 'run_wake_consumer'
						AND operation.max_steps = OLD.max_steps
						AND result.status = 'failed'
						AND result.stop_reason = NEW.stop_reason
						AND result.error_code = NEW.error_code
				))))
		BEGIN SELECT RAISE(ABORT, 'Run wake consumption result is invalid'); END;`,
	`CREATE TRIGGER trg_run_wake_consumption_delete_immutable
		BEFORE DELETE ON run_wake_consumptions BEGIN
			SELECT RAISE(ABORT, 'Run wake consumption cannot be deleted');
		END;`,
	`DROP TRIGGER trg_run_wake_intent_update;`,
	`CREATE TRIGGER trg_run_wake_intent_update
		BEFORE UPDATE ON run_wake_intents
		WHEN NEW.id <> OLD.id OR NEW.protocol_version <> OLD.protocol_version
			OR NEW.run_id <> OLD.run_id OR NEW.session_id <> OLD.session_id
			OR NEW.max_attempts <> OLD.max_attempts
			OR NEW.initial_delay_seconds <> OLD.initial_delay_seconds
			OR NEW.base_backoff_seconds <> OLD.base_backoff_seconds
			OR NEW.max_backoff_seconds <> OLD.max_backoff_seconds
			OR NEW.max_elapsed_seconds <> OLD.max_elapsed_seconds
			OR NEW.deadline_at <> OLD.deadline_at OR NEW.created_at <> OLD.created_at
			OR NEW.execution_enabled <> 0 OR NEW.background_loop_enabled <> 0
			OR julianday(NEW.updated_at) < julianday(OLD.updated_at)
			OR NOT (
				(OLD.status = 'queued' AND NEW.status = 'leased'
					AND NEW.attempt_count = OLD.attempt_count + 1
					AND NEW.active_lease_id IS NOT NULL AND NEW.cancelled_at IS NULL
					AND NEW.next_wake_at = OLD.next_wake_at
					AND EXISTS (SELECT 1 FROM run_events event
						WHERE event.run_id = OLD.run_id AND event.type = 'run.wake_claimed'
							AND event.source = 'run_wake_coordinator' AND event.subject_id = OLD.id
							AND event.created_at = NEW.updated_at
							AND json_extract(event.payload_json, '$.generation') = NEW.attempt_count))
				OR (OLD.status = 'leased' AND NEW.status = 'queued'
					AND NEW.attempt_count = OLD.attempt_count AND NEW.active_lease_id IS NULL
					AND NEW.cancelled_at IS NULL
					AND julianday(NEW.next_wake_at) >= julianday(NEW.updated_at)
					AND EXISTS (SELECT 1 FROM run_events event
						WHERE event.run_id = OLD.run_id AND event.type = 'run.wake_retried'
							AND event.source = 'run_wake_coordinator' AND event.subject_id = OLD.id
							AND event.created_at = NEW.updated_at
							AND json_extract(event.payload_json, '$.generation') = NEW.attempt_count))
				OR (OLD.status IN ('queued', 'leased') AND NEW.status = 'exhausted'
					AND NEW.attempt_count = OLD.attempt_count AND NEW.active_lease_id IS NULL
					AND NEW.cancelled_at IS NULL
					AND EXISTS (SELECT 1 FROM run_events event
						WHERE event.run_id = OLD.run_id AND event.type = 'run.wake_exhausted'
							AND event.source = 'run_wake_coordinator' AND event.subject_id = OLD.id
							AND event.created_at = NEW.updated_at))
				OR (OLD.status = 'leased' AND NEW.status = 'exhausted'
					AND NEW.attempt_count = OLD.attempt_count AND NEW.active_lease_id IS NULL
					AND NEW.cancelled_at IS NULL AND NEW.next_wake_at = NEW.deadline_at
					AND EXISTS (SELECT 1 FROM run_wake_consumptions consumption
						JOIN run_events event ON event.run_id = OLD.run_id
							AND event.sequence = consumption.completion_event_sequence
						WHERE consumption.intent_id = OLD.id
							AND consumption.generation = OLD.attempt_count
							AND consumption.status = 'completed'
							AND event.type = 'run.wake_completed'
							AND event.source = 'run_wake_consumer'
							AND event.subject_id = consumption.id
							AND event.created_at = NEW.updated_at))
				OR (OLD.status IN ('queued', 'leased') AND NEW.status = 'cancelled'
					AND NEW.attempt_count = OLD.attempt_count AND NEW.active_lease_id IS NULL
					AND NEW.cancelled_at = NEW.updated_at
					AND (OLD.status <> 'leased' OR NOT EXISTS (
						SELECT 1 FROM run_wake_consumptions consumption
						WHERE consumption.intent_id = OLD.id
							AND consumption.generation = OLD.attempt_count
							AND consumption.lease_id = OLD.active_lease_id
							AND consumption.status = 'prepared'))
					AND EXISTS (SELECT 1 FROM run_events event
						WHERE event.run_id = OLD.run_id AND event.type = 'run.wake_cancelled'
							AND event.source = 'run_wake_control' AND event.subject_id = OLD.id
							AND event.created_at = NEW.updated_at))
			)
		BEGIN SELECT RAISE(ABORT, 'Run wake intent transition is invalid'); END;`,
	`DROP TRIGGER trg_run_wake_lease_update;`,
	`CREATE TRIGGER trg_run_wake_lease_update
		BEFORE UPDATE ON run_wake_leases
		WHEN NEW.id <> OLD.id OR NEW.intent_id <> OLD.intent_id
			OR NEW.generation <> OLD.generation OR NEW.owner_id <> OLD.owner_id
			OR NEW.acquired_at <> OLD.acquired_at OR NEW.expires_at <> OLD.expires_at
			OR OLD.status <> 'active' OR NEW.status NOT IN ('released', 'revoked', 'expired')
			OR NEW.ended_at IS NULL OR julianday(NEW.ended_at) < julianday(OLD.acquired_at)
			OR NOT EXISTS (
				SELECT 1 FROM run_wake_intents intent
				JOIN run_events event ON event.run_id = intent.run_id
				WHERE intent.id = OLD.intent_id AND event.created_at = NEW.ended_at
					AND ((NEW.status = 'released' AND event.type IN
						('run.wake_retried', 'run.wake_exhausted')
						AND event.source = 'run_wake_coordinator' AND event.subject_id = intent.id)
						OR (NEW.status = 'released' AND event.type = 'run.wake_completed'
							AND event.source = 'run_wake_consumer'
							AND EXISTS (SELECT 1 FROM run_wake_consumptions consumption
								WHERE consumption.intent_id = intent.id
									AND consumption.lease_id = OLD.id
									AND consumption.status = 'completed'
									AND event.subject_id = consumption.id))
						OR (NEW.status = 'revoked' AND event.type = 'run.wake_cancelled'
							AND event.source = 'run_wake_control' AND event.subject_id = intent.id)
						OR (NEW.status = 'expired' AND event.type IN
							('run.wake_retried', 'run.wake_exhausted')
							AND event.source = 'run_wake_coordinator' AND event.subject_id = intent.id))
			)
		BEGIN SELECT RAISE(ABORT, 'Run wake lease transition is invalid'); END;`,
}
