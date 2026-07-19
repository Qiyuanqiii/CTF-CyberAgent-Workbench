package store

var operatorVerificationEvidenceStatements = []string{
	`CREATE TABLE operator_verification_evidence (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		outcome TEXT NOT NULL,
		title TEXT NOT NULL,
		summary TEXT NOT NULL,
		summary_sha256 TEXT NOT NULL,
		redacted INTEGER NOT NULL,
		recorded_by TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'operator_verification_evidence.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(outcome IN ('pass', 'fail', 'unknown')),
		CHECK(title = trim(title) AND length(title) BETWEEN 1 AND 160 AND instr(title, char(0)) = 0),
		CHECK(summary = trim(summary) AND length(summary) BETWEEN 1 AND 2048 AND instr(summary, char(0)) = 0),
		CHECK(length(summary_sha256) = 64 AND summary_sha256 = lower(summary_sha256)
			AND summary_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(redacted IN (0, 1)),
		CHECK(recorded_by = trim(recorded_by) AND length(recorded_by) BETWEEN 1 AND 256
			AND instr(recorded_by, char(0)) = 0),
		CHECK(event_sequence > 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_operator_verification_evidence_run_created
		ON operator_verification_evidence(run_id, created_at DESC, id DESC);`,
	`CREATE TRIGGER trg_operator_verification_evidence_insert
		BEFORE INSERT ON operator_verification_evidence
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
				AND mission.workspace_id = NEW.workspace_id
				AND session_record.id = NEW.session_id
				AND session_record.workspace_id = NEW.workspace_id
				AND session_record.status = 'active'
				AND event.type = 'verification.evidence_recorded'
				AND event.source = 'operator_verification'
				AND event.subject_id = NEW.id AND event.created_at = NEW.created_at
				AND json_extract(event.payload_json, '$.outcome') = NEW.outcome
				AND json_extract(event.payload_json, '$.summary_sha256') = NEW.summary_sha256
				AND json_extract(event.payload_json, '$.redacted') = NEW.redacted
				AND json_extract(event.payload_json, '$.command_executed') = 0
				AND json_extract(event.payload_json, '$.model_assertion') = 0
				AND json_extract(event.payload_json, '$.approval') = 0
				AND json_extract(event.payload_json, '$.authority_granted') = 0
		)
		BEGIN SELECT RAISE(ABORT, 'operator verification evidence binding is invalid'); END;`,
	`CREATE TRIGGER trg_operator_verification_evidence_update_immutable
		BEFORE UPDATE ON operator_verification_evidence BEGIN
			SELECT RAISE(ABORT, 'operator verification evidence cannot be updated');
		END;`,
	`CREATE TRIGGER trg_operator_verification_evidence_delete_immutable
		BEFORE DELETE ON operator_verification_evidence BEGIN
			SELECT RAISE(ABORT, 'operator verification evidence cannot be deleted');
		END;`,
}
