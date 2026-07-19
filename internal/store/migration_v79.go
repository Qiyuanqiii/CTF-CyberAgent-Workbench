package store

var runProgressGuardStatements = []string{
	`CREATE TABLE run_progress_guards (
		run_id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		state_fingerprint TEXT NOT NULL,
		action_fingerprint TEXT NOT NULL,
		repeated_action_count INTEGER NOT NULL,
		stagnant_turn_count INTEGER NOT NULL,
		repeat_threshold INTEGER NOT NULL,
		stagnant_threshold INTEGER NOT NULL,
		last_turn INTEGER NOT NULL,
		status TEXT NOT NULL,
		reason_code TEXT NOT NULL,
		detected_at TEXT,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE,
		CHECK(protocol_version = 'run_progress_guard.v1'),
		CHECK((state_fingerprint = '' AND action_fingerprint = ''
			AND repeated_action_count = 0 AND stagnant_turn_count = 0)
			OR (length(state_fingerprint) = 64 AND state_fingerprint = lower(state_fingerprint)
				AND state_fingerprint NOT GLOB '*[^0-9a-f]*'
				AND length(action_fingerprint) = 64 AND action_fingerprint = lower(action_fingerprint)
				AND action_fingerprint NOT GLOB '*[^0-9a-f]*'
				AND repeated_action_count > 0 AND stagnant_turn_count > 0 AND last_turn > 0)),
		CHECK(repeat_threshold = 3 AND stagnant_threshold = 6),
		CHECK(last_turn >= 0),
		CHECK((status = 'observing' AND reason_code = '' AND detected_at IS NULL
				AND repeated_action_count < repeat_threshold
				AND stagnant_turn_count < stagnant_threshold)
			OR (status = 'livelock_detected'
				AND reason_code IN ('repeated_action', 'no_observable_progress')
				AND detected_at IS NOT NULL
				AND ((reason_code = 'repeated_action' AND repeated_action_count >= repeat_threshold)
					OR (reason_code = 'no_observable_progress' AND stagnant_turn_count >= stagnant_threshold))))
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_run_progress_guards_status_updated
		ON run_progress_guards(status, updated_at DESC, run_id);`,
	`CREATE TRIGGER trg_run_progress_guard_insert
		BEFORE INSERT ON run_progress_guards
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN run_supervisor_checkpoints checkpoint ON checkpoint.run_id = run.id
			WHERE run.id = NEW.run_id AND run.status = 'running'
				AND checkpoint.phase = 'turn_started' AND checkpoint.next_turn = NEW.last_turn
		)
		BEGIN SELECT RAISE(ABORT, 'Run progress guard insert binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_progress_guard_update
		BEFORE UPDATE ON run_progress_guards
		WHEN OLD.run_id != NEW.run_id OR NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN run_supervisor_checkpoints checkpoint ON checkpoint.run_id = run.id
			WHERE run.id = NEW.run_id AND run.status = 'running'
				AND checkpoint.phase = 'turn_started' AND checkpoint.next_turn = NEW.last_turn
		) OR (OLD.status = 'livelock_detected' AND NEW.status = 'observing'
			AND NOT EXISTS (
				SELECT 1 FROM run_events event
				WHERE event.run_id = NEW.run_id AND event.type = 'run.status_changed'
					AND json_extract(event.payload_json, '$.from') = 'paused'
					AND json_extract(event.payload_json, '$.to') = 'running'
					AND julianday(event.created_at) >= julianday(OLD.detected_at)
			))
		BEGIN SELECT RAISE(ABORT, 'Run progress guard update binding is invalid'); END;`,
}
