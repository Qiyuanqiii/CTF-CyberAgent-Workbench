package store

var sessionEvidenceAttachmentStatements = []string{
	`CREATE TABLE session_evidence_attachments (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		source_kind TEXT NOT NULL,
		source_ref TEXT NOT NULL,
		content_sha256 TEXT NOT NULL,
		session_message_id INTEGER NOT NULL UNIQUE,
		attached_by TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_message_id) REFERENCES session_messages(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'session_evidence_attachment.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(source_kind = 'workspace_file'),
		CHECK(source_ref = trim(source_ref) AND length(source_ref) BETWEEN 1 AND 512
			AND instr(source_ref, char(0)) = 0 AND instr(source_ref, char(9)) = 0
			AND instr(source_ref, char(10)) = 0 AND instr(source_ref, char(13)) = 0
			AND source_ref <> '.' AND substr(source_ref, 1, 1) <> '/'
			AND instr(source_ref, '\') = 0 AND instr(source_ref, ':') = 0
			AND instr(source_ref, '//') = 0
			AND instr('/' || source_ref || '/', '/../') = 0
			AND instr('/' || source_ref || '/', '/./') = 0),
		CHECK(length(content_sha256) = 64 AND content_sha256 = lower(content_sha256)
			AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(attached_by = trim(attached_by) AND length(attached_by) BETWEEN 1 AND 256
			AND instr(attached_by, char(0)) = 0),
		CHECK(event_sequence > 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_session_evidence_attachments_run_created
		ON session_evidence_attachments(run_id, created_at, id);`,
	`CREATE TRIGGER trg_session_evidence_attachment_insert
		BEFORE INSERT ON session_evidence_attachments
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN session_messages message ON message.id = NEW.session_message_id
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
				AND run.status IN ('running', 'paused')
				AND mission.workspace_id = NEW.workspace_id
				AND session_record.id = NEW.session_id
				AND session_record.workspace_id = NEW.workspace_id
				AND session_record.status = 'active'
				AND message.session_id = NEW.session_id AND message.role = 'tool'
				AND message.provenance_version = 'context_provenance.v1'
				AND message.source_kind = NEW.source_kind
				AND message.source_ref = NEW.source_ref
				AND message.content_sha256 = NEW.content_sha256
				AND message.instruction_authorized = 0
				AND message.created_at = NEW.created_at
				AND event.type = 'session.evidence_attached'
				AND event.source = 'evidence_attachment'
				AND event.subject_id = NEW.id AND event.created_at = NEW.created_at
				AND json_extract(event.payload_json, '$.session_message_id') = NEW.session_message_id
				AND json_extract(event.payload_json, '$.source_kind') = NEW.source_kind
				AND json_extract(event.payload_json, '$.source_ref') = NEW.source_ref
				AND json_extract(event.payload_json, '$.content_sha256') = NEW.content_sha256
				AND json_extract(event.payload_json, '$.instruction_authorized') = 0
				AND json_extract(event.payload_json, '$.model_called') = 0
				AND json_extract(event.payload_json, '$.tool_called') = 0
		)
		BEGIN SELECT RAISE(ABORT, 'Session evidence attachment binding is invalid'); END;`,
	`CREATE TRIGGER trg_session_evidence_attachment_update_immutable
		BEFORE UPDATE ON session_evidence_attachments BEGIN
			SELECT RAISE(ABORT, 'Session evidence attachments cannot be updated');
		END;`,
	`CREATE TRIGGER trg_session_evidence_attachment_delete_immutable
		BEFORE DELETE ON session_evidence_attachments BEGIN
			SELECT RAISE(ABORT, 'Session evidence attachments cannot be deleted');
		END;`,
}
