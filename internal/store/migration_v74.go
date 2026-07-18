package store

var runWakeOwnershipStatements = []string{
	`CREATE TABLE run_wake_intents (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		status TEXT NOT NULL,
		max_attempts INTEGER NOT NULL,
		attempt_count INTEGER NOT NULL,
		initial_delay_seconds INTEGER NOT NULL,
		base_backoff_seconds INTEGER NOT NULL,
		max_backoff_seconds INTEGER NOT NULL,
		max_elapsed_seconds INTEGER NOT NULL,
		next_wake_at TEXT NOT NULL,
		deadline_at TEXT NOT NULL,
		active_lease_id TEXT,
		execution_enabled INTEGER NOT NULL,
		background_loop_enabled INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		cancelled_at TEXT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(active_lease_id) REFERENCES run_wake_leases(id)
			DEFERRABLE INITIALLY DEFERRED,
		CHECK(protocol_version = 'run_wake_intent.v1'),
		CHECK(status IN ('queued', 'leased', 'cancelled', 'exhausted')),
		CHECK(max_attempts BETWEEN 1 AND 8 AND attempt_count BETWEEN 0 AND max_attempts),
		CHECK(initial_delay_seconds BETWEEN 0 AND 3600),
		CHECK(base_backoff_seconds BETWEEN 5 AND 21600),
		CHECK(max_backoff_seconds BETWEEN base_backoff_seconds AND 21600),
		CHECK(max_elapsed_seconds BETWEEN 60 AND 86400),
		CHECK(julianday(next_wake_at) >= julianday(created_at)
			AND julianday(next_wake_at) <= julianday(deadline_at)
			AND julianday(deadline_at) > julianday(created_at)),
		CHECK((status = 'leased') = (active_lease_id IS NOT NULL)),
		CHECK((status = 'cancelled') = (cancelled_at IS NOT NULL)),
		CHECK(execution_enabled = 0 AND background_loop_enabled = 0),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(session_id = trim(session_id) AND length(session_id) BETWEEN 1 AND 256
			AND instr(session_id, char(0)) = 0)
	);`,
	`CREATE UNIQUE INDEX idx_run_wake_intents_active_run
		ON run_wake_intents(run_id) WHERE status IN ('queued', 'leased');`,
	`CREATE INDEX idx_run_wake_intents_due
		ON run_wake_intents(status, next_wake_at, created_at);`,
	`CREATE TABLE run_wake_leases (
		id TEXT PRIMARY KEY,
		intent_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		owner_id TEXT NOT NULL,
		status TEXT NOT NULL,
		acquired_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		ended_at TEXT,
		FOREIGN KEY(intent_id) REFERENCES run_wake_intents(id) ON DELETE RESTRICT,
		UNIQUE(intent_id, generation),
		CHECK(generation BETWEEN 1 AND 8),
		CHECK(status IN ('active', 'released', 'revoked', 'expired')),
		CHECK(julianday(expires_at) > julianday(acquired_at)
			AND julianday(expires_at) <= julianday(acquired_at, '+30 seconds')),
		CHECK((status = 'active') = (ended_at IS NULL)),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(owner_id = trim(owner_id) AND length(owner_id) BETWEEN 1 AND 256
			AND instr(owner_id, char(0)) = 0)
	);`,
	`CREATE UNIQUE INDEX idx_run_wake_leases_active_intent
		ON run_wake_leases(intent_id) WHERE status = 'active';`,
	`CREATE TABLE run_wake_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		action TEXT NOT NULL,
		intent_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(intent_id) REFERENCES run_wake_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'run_wake_control.v1'),
		CHECK(action IN ('schedule', 'cancel')),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(event_sequence > 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_run_wake_operations_run_created
		ON run_wake_operations(run_id, created_at);`,
	`CREATE TRIGGER trg_run_wake_intent_insert
		BEFORE INSERT ON run_wake_intents
		WHEN NEW.status <> 'queued' OR NEW.attempt_count <> 0 OR NEW.active_lease_id IS NOT NULL
			OR NEW.cancelled_at IS NOT NULL OR NEW.execution_enabled <> 0
			OR NEW.background_loop_enabled <> 0
			OR NOT EXISTS (
				SELECT 1 FROM runs run
				JOIN sessions session_record ON session_record.id = run.session_id
				JOIN run_events event ON event.run_id = run.id
				WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
					AND run.status = 'running' AND session_record.status = 'active'
					AND event.type = 'run.wake_scheduled'
					AND event.source = 'run_wake_control' AND event.subject_id = NEW.id
					AND event.created_at = NEW.created_at
					AND json_extract(event.payload_json, '$.max_attempts') = NEW.max_attempts
					AND json_extract(event.payload_json, '$.initial_delay_seconds') = NEW.initial_delay_seconds
					AND json_extract(event.payload_json, '$.base_backoff_seconds') = NEW.base_backoff_seconds
					AND json_extract(event.payload_json, '$.max_backoff_seconds') = NEW.max_backoff_seconds
					AND json_extract(event.payload_json, '$.max_elapsed_seconds') = NEW.max_elapsed_seconds
					AND json_extract(event.payload_json, '$.execution_enabled') = 0
					AND json_extract(event.payload_json, '$.background_loop_enabled') = 0
					AND EXISTS (SELECT 1 FROM operator_steering_messages message
						WHERE message.run_id = run.id AND message.session_id = run.session_id
							AND message.status = 'pending')
					AND NOT EXISTS (SELECT 1 FROM run_execution_leases execution_lease
						WHERE execution_lease.run_id = run.id AND execution_lease.status = 'active'
							AND julianday(execution_lease.expires_at) > julianday(NEW.created_at))
			)
		BEGIN SELECT RAISE(ABORT, 'Run wake intent initial binding is invalid'); END;`,
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
					AND NEW.cancelled_at IS NULL AND julianday(NEW.next_wake_at) >= julianday(NEW.updated_at)
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
				OR (OLD.status IN ('queued', 'leased') AND NEW.status = 'cancelled'
					AND NEW.attempt_count = OLD.attempt_count AND NEW.active_lease_id IS NULL
					AND NEW.cancelled_at = NEW.updated_at
					AND EXISTS (SELECT 1 FROM run_events event
						WHERE event.run_id = OLD.run_id AND event.type = 'run.wake_cancelled'
							AND event.source = 'run_wake_control' AND event.subject_id = OLD.id
							AND event.created_at = NEW.updated_at))
			)
		BEGIN SELECT RAISE(ABORT, 'Run wake intent transition is invalid'); END;`,
	`CREATE TRIGGER trg_run_wake_intent_delete_immutable
		BEFORE DELETE ON run_wake_intents BEGIN
			SELECT RAISE(ABORT, 'Run wake intent cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_run_wake_lease_insert
		BEFORE INSERT ON run_wake_leases
		WHEN NEW.status <> 'active' OR NEW.ended_at IS NOT NULL OR NOT EXISTS (
			SELECT 1 FROM run_wake_intents intent
			WHERE intent.id = NEW.intent_id AND intent.status = 'leased'
				AND intent.active_lease_id = NEW.id AND intent.attempt_count = NEW.generation
				AND julianday(NEW.acquired_at) = julianday(intent.updated_at)
				AND julianday(NEW.expires_at) <= julianday(intent.deadline_at)
		)
		BEGIN SELECT RAISE(ABORT, 'Run wake lease binding is invalid'); END;`,
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
				WHERE intent.id = OLD.intent_id AND event.subject_id = intent.id
					AND event.source IN ('run_wake_control', 'run_wake_coordinator')
					AND event.created_at = NEW.ended_at
					AND ((NEW.status = 'released' AND event.type IN
						('run.wake_retried', 'run.wake_exhausted'))
						OR (NEW.status = 'revoked' AND event.type = 'run.wake_cancelled')
						OR (NEW.status = 'expired' AND event.type IN
							('run.wake_retried', 'run.wake_exhausted')))
			)
		BEGIN SELECT RAISE(ABORT, 'Run wake lease transition is invalid'); END;`,
	`CREATE TRIGGER trg_run_wake_lease_delete_immutable
		BEFORE DELETE ON run_wake_leases BEGIN
			SELECT RAISE(ABORT, 'Run wake lease cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_run_wake_operation_insert
		BEFORE INSERT ON run_wake_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM run_wake_intents intent
			JOIN run_events event ON event.run_id = intent.run_id
				AND event.sequence = NEW.event_sequence
			WHERE intent.id = NEW.intent_id AND intent.run_id = NEW.run_id
				AND event.source = 'run_wake_control' AND event.subject_id = intent.id
				AND event.created_at = NEW.created_at
				AND ((NEW.action = 'schedule' AND event.type = 'run.wake_scheduled'
					AND intent.created_at = NEW.created_at)
					OR (NEW.action = 'cancel' AND event.type = 'run.wake_cancelled'
						AND intent.status = 'cancelled' AND intent.cancelled_at = NEW.created_at))
		)
		BEGIN SELECT RAISE(ABORT, 'Run wake operation binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_wake_operation_update_immutable
		BEFORE UPDATE ON run_wake_operations BEGIN
			SELECT RAISE(ABORT, 'Run wake operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_wake_operation_delete_immutable
		BEFORE DELETE ON run_wake_operations BEGIN
			SELECT RAISE(ABORT, 'Run wake operation cannot be deleted');
		END;`,
}
