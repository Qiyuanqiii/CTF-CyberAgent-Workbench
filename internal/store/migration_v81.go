package store

var operatorVerificationPlanEvidenceAssociationStatements = []string{
	`CREATE TABLE operator_verification_plan_evidence_associations (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		plan_item_ordinal INTEGER NOT NULL,
		plan_item_sha256 TEXT NOT NULL,
		evidence_id TEXT NOT NULL UNIQUE,
		evidence_outcome TEXT NOT NULL,
		evidence_event_sequence INTEGER NOT NULL,
		associated_by TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id, plan_item_ordinal)
			REFERENCES operator_verification_plan_items(plan_id, ordinal) ON DELETE RESTRICT,
		FOREIGN KEY(evidence_id)
			REFERENCES operator_verification_evidence(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'operator_verification_plan_evidence_association.v1'),
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(plan_item_ordinal BETWEEN 1 AND 32),
		CHECK(length(plan_item_sha256) = 64
			AND plan_item_sha256 = lower(plan_item_sha256)
			AND plan_item_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(evidence_outcome IN ('pass', 'fail', 'unknown')),
		CHECK(evidence_event_sequence > 0 AND event_sequence > evidence_event_sequence),
		CHECK(associated_by = trim(associated_by) AND length(associated_by) BETWEEN 1 AND 256
			AND instr(associated_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_operator_verification_associations_run_created
		ON operator_verification_plan_evidence_associations(run_id, created_at DESC, id DESC);`,
	`CREATE INDEX idx_operator_verification_associations_plan_item
		ON operator_verification_plan_evidence_associations(
			plan_id, plan_item_ordinal, event_sequence);`,
	`CREATE TRIGGER trg_operator_verification_association_insert
		BEFORE INSERT ON operator_verification_plan_evidence_associations
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN run_mode_snapshots mode ON mode.run_id = run.id
			JOIN operator_verification_plans plan ON plan.id = NEW.plan_id
			JOIN operator_verification_plan_items item
				ON item.plan_id = plan.id AND item.ordinal = NEW.plan_item_ordinal
			JOIN operator_verification_evidence evidence ON evidence.id = NEW.evidence_id
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
				AND mission.workspace_id = NEW.workspace_id
				AND session_record.workspace_id = NEW.workspace_id
				AND session_record.status = 'active'
				AND mode.surface = 'code'
				AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
					WHERE later.run_id = mode.run_id AND later.revision > mode.revision)
				AND plan.run_id = NEW.run_id AND plan.session_id = NEW.session_id
				AND plan.workspace_id = NEW.workspace_id
				AND item.item_sha256 = NEW.plan_item_sha256
				AND evidence.run_id = NEW.run_id AND evidence.session_id = NEW.session_id
				AND evidence.workspace_id = NEW.workspace_id
				AND evidence.outcome = NEW.evidence_outcome
				AND evidence.event_sequence = NEW.evidence_event_sequence
				AND plan.event_sequence < evidence.event_sequence
				AND event.type = 'verification.plan_evidence_associated'
				AND event.source = 'operator_verification_association'
				AND event.subject_id = NEW.id AND event.created_at = NEW.created_at
				AND json_extract(event.payload_json, '$.plan_id') = NEW.plan_id
				AND json_extract(event.payload_json, '$.plan_item_ordinal') = NEW.plan_item_ordinal
				AND json_extract(event.payload_json, '$.plan_item_sha256') = NEW.plan_item_sha256
				AND json_extract(event.payload_json, '$.evidence_id') = NEW.evidence_id
				AND json_extract(event.payload_json, '$.evidence_outcome') = NEW.evidence_outcome
				AND json_extract(event.payload_json, '$.evidence_event_sequence') =
					NEW.evidence_event_sequence
				AND json_extract(event.payload_json, '$.operator_associated') = 1
				AND json_extract(event.payload_json, '$.command_executed') = 0
				AND json_extract(event.payload_json, '$.model_assertion') = 0
				AND json_extract(event.payload_json, '$.result_inferred') = 0
				AND json_extract(event.payload_json, '$.record_rewritten') = 0
				AND json_extract(event.payload_json, '$.approval') = 0
				AND json_extract(event.payload_json, '$.authority_granted') = 0
		)
		BEGIN SELECT RAISE(ABORT, 'verification association binding is invalid'); END;`,
	`CREATE TRIGGER trg_operator_verification_association_update_immutable
		BEFORE UPDATE ON operator_verification_plan_evidence_associations BEGIN
			SELECT RAISE(ABORT, 'verification association cannot be updated');
		END;`,
	`CREATE TRIGGER trg_operator_verification_association_delete_immutable
		BEFORE DELETE ON operator_verification_plan_evidence_associations BEGIN
			SELECT RAISE(ABORT, 'verification association cannot be deleted');
		END;`,
}
