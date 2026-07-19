package store

var operatorVerificationPlanStatements = []string{
	`CREATE TABLE operator_verification_plans (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		title TEXT NOT NULL,
		summary TEXT NOT NULL,
		plan_sha256 TEXT NOT NULL,
		redacted INTEGER NOT NULL,
		authored_by TEXT NOT NULL,
		item_count INTEGER NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'operator_verification_plan.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(title = trim(title) AND length(title) BETWEEN 1 AND 160 AND instr(title, char(0)) = 0),
		CHECK(summary = trim(summary) AND length(summary) BETWEEN 1 AND 2048 AND instr(summary, char(0)) = 0),
		CHECK(length(plan_sha256) = 64 AND plan_sha256 = lower(plan_sha256)
			AND plan_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(redacted IN (0, 1)),
		CHECK(authored_by = trim(authored_by) AND length(authored_by) BETWEEN 1 AND 256
			AND instr(authored_by, char(0)) = 0),
		CHECK(item_count BETWEEN 1 AND 32),
		CHECK(event_sequence > 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE operator_verification_plan_items (
		plan_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		title TEXT NOT NULL,
		expected_observation TEXT NOT NULL,
		item_sha256 TEXT NOT NULL,
		redacted INTEGER NOT NULL,
		PRIMARY KEY(plan_id, ordinal),
		FOREIGN KEY(plan_id) REFERENCES operator_verification_plans(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 32),
		CHECK(title = trim(title) AND length(title) BETWEEN 1 AND 160 AND instr(title, char(0)) = 0),
		CHECK(expected_observation = trim(expected_observation)
			AND length(expected_observation) BETWEEN 1 AND 1024
			AND instr(expected_observation, char(0)) = 0),
		CHECK(length(item_sha256) = 64 AND item_sha256 = lower(item_sha256)
			AND item_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(redacted IN (0, 1))
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_operator_verification_plans_run_created
		ON operator_verification_plans(run_id, created_at DESC, id DESC);`,
	`CREATE TRIGGER trg_operator_verification_plan_insert
		BEFORE INSERT ON operator_verification_plans
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN run_mode_snapshots mode ON mode.run_id = run.id
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
				AND mission.workspace_id = NEW.workspace_id
				AND session_record.id = NEW.session_id
				AND session_record.workspace_id = NEW.workspace_id
				AND session_record.status = 'active' AND mode.surface = 'code'
				AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
					WHERE later.run_id = mode.run_id AND later.revision > mode.revision)
				AND event.type = 'verification.plan_recorded'
				AND event.source = 'operator_verification_plan'
				AND event.subject_id = NEW.id AND event.created_at = NEW.created_at
				AND json_extract(event.payload_json, '$.plan_sha256') = NEW.plan_sha256
				AND json_extract(event.payload_json, '$.item_count') = NEW.item_count
				AND json_extract(event.payload_json, '$.redacted') = NEW.redacted
				AND json_extract(event.payload_json, '$.guidance_only') = 1
				AND json_extract(event.payload_json, '$.command_executed') = 0
				AND json_extract(event.payload_json, '$.model_assertion') = 0
				AND json_extract(event.payload_json, '$.result_inferred') = 0
				AND json_extract(event.payload_json, '$.approval') = 0
				AND json_extract(event.payload_json, '$.authority_granted') = 0
		)
		BEGIN SELECT RAISE(ABORT, 'operator verification plan binding is invalid'); END;`,
	`CREATE TRIGGER trg_operator_verification_plan_item_insert
		BEFORE INSERT ON operator_verification_plan_items
		WHEN NOT EXISTS (
			SELECT 1 FROM operator_verification_plans plan
			WHERE plan.id = NEW.plan_id AND NEW.ordinal <= plan.item_count
		)
		BEGIN SELECT RAISE(ABORT, 'operator verification plan item binding is invalid'); END;`,
	`CREATE TRIGGER trg_operator_verification_plan_update_immutable
		BEFORE UPDATE ON operator_verification_plans BEGIN
			SELECT RAISE(ABORT, 'operator verification plan cannot be updated');
		END;`,
	`CREATE TRIGGER trg_operator_verification_plan_delete_immutable
		BEFORE DELETE ON operator_verification_plans BEGIN
			SELECT RAISE(ABORT, 'operator verification plan cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_operator_verification_plan_item_update_immutable
		BEFORE UPDATE ON operator_verification_plan_items BEGIN
			SELECT RAISE(ABORT, 'operator verification plan item cannot be updated');
		END;`,
	`CREATE TRIGGER trg_operator_verification_plan_item_delete_immutable
		BEFORE DELETE ON operator_verification_plan_items BEGIN
			SELECT RAISE(ABORT, 'operator verification plan item cannot be deleted');
		END;`,
}
