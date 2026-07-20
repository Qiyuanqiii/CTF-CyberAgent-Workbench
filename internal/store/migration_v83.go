package store

var operatorVerificationSnapshotReceiptStatements = []string{
	`CREATE TABLE operator_verification_snapshot_receipts (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		plan_sha256 TEXT NOT NULL,
		plan_item_ordinal INTEGER NOT NULL,
		plan_item_sha256 TEXT NOT NULL,
		format TEXT NOT NULL,
		snapshot_high_water_event_sequence INTEGER NOT NULL,
		associated_evidence_count INTEGER NOT NULL,
		pass_count INTEGER NOT NULL,
		fail_count INTEGER NOT NULL,
		unknown_count INTEGER NOT NULL,
		returned_association_count INTEGER NOT NULL,
		associations_truncated INTEGER NOT NULL,
		content_sha256 TEXT NOT NULL,
		content_bytes INTEGER NOT NULL,
		recorded_by TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id, plan_item_ordinal)
			REFERENCES operator_verification_plan_items(plan_id, ordinal) ON DELETE RESTRICT,
		CHECK(protocol_version = 'operator_verification_plan_item_snapshot_receipt.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_sha256) = 64 AND plan_sha256 = lower(plan_sha256)
			AND plan_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(plan_item_ordinal BETWEEN 1 AND 32),
		CHECK(length(plan_item_sha256) = 64 AND plan_item_sha256 = lower(plan_item_sha256)
			AND plan_item_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(format IN ('json', 'markdown')),
		CHECK(snapshot_high_water_event_sequence >= 0),
		CHECK(associated_evidence_count >= 0 AND pass_count >= 0 AND fail_count >= 0
			AND unknown_count >= 0 AND associated_evidence_count = pass_count + fail_count + unknown_count),
		CHECK((associated_evidence_count = 0) = (snapshot_high_water_event_sequence = 0)),
		CHECK(returned_association_count = CASE WHEN associated_evidence_count > 100
			THEN 100 ELSE associated_evidence_count END),
		CHECK(associations_truncated IN (0, 1)
			AND associations_truncated = (associated_evidence_count > 100)),
		CHECK(length(content_sha256) = 64 AND content_sha256 = lower(content_sha256)
			AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(content_bytes BETWEEN 1 AND 262144),
		CHECK(recorded_by = trim(recorded_by) AND length(recorded_by) BETWEEN 1 AND 256
			AND instr(recorded_by, char(0)) = 0),
		CHECK(event_sequence > snapshot_high_water_event_sequence)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_operator_verification_snapshot_receipts_run_event
		ON operator_verification_snapshot_receipts(run_id, event_sequence DESC, id DESC);`,
	`CREATE INDEX idx_operator_verification_snapshot_receipts_plan_item
		ON operator_verification_snapshot_receipts(plan_id, plan_item_ordinal, event_sequence DESC);`,
	`CREATE TRIGGER trg_operator_verification_snapshot_receipt_insert
		BEFORE INSERT ON operator_verification_snapshot_receipts
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN run_mode_snapshots mode ON mode.run_id = run.id
			JOIN operator_verification_plans plan ON plan.id = NEW.plan_id
			JOIN operator_verification_plan_items item
				ON item.plan_id = plan.id AND item.ordinal = NEW.plan_item_ordinal
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE run.id = NEW.run_id AND run.session_id = NEW.session_id
				AND mission.workspace_id = NEW.workspace_id
				AND session_record.workspace_id = NEW.workspace_id
				AND session_record.status = 'active' AND mode.surface = 'code'
				AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
					WHERE later.run_id = mode.run_id AND later.revision > mode.revision)
				AND plan.run_id = NEW.run_id AND plan.session_id = NEW.session_id
				AND plan.workspace_id = NEW.workspace_id AND plan.plan_sha256 = NEW.plan_sha256
				AND item.item_sha256 = NEW.plan_item_sha256
				AND NEW.snapshot_high_water_event_sequence = COALESCE((SELECT MAX(association.event_sequence)
					FROM operator_verification_plan_evidence_associations association
					WHERE association.run_id = NEW.run_id AND association.plan_id = NEW.plan_id
						AND association.plan_item_ordinal = NEW.plan_item_ordinal), 0)
				AND NEW.associated_evidence_count = (SELECT COUNT(*)
					FROM operator_verification_plan_evidence_associations association
					WHERE association.run_id = NEW.run_id AND association.plan_id = NEW.plan_id
						AND association.plan_item_ordinal = NEW.plan_item_ordinal)
				AND NEW.pass_count = (SELECT COUNT(*)
					FROM operator_verification_plan_evidence_associations association
					WHERE association.run_id = NEW.run_id AND association.plan_id = NEW.plan_id
						AND association.plan_item_ordinal = NEW.plan_item_ordinal
						AND association.evidence_outcome = 'pass')
				AND NEW.fail_count = (SELECT COUNT(*)
					FROM operator_verification_plan_evidence_associations association
					WHERE association.run_id = NEW.run_id AND association.plan_id = NEW.plan_id
						AND association.plan_item_ordinal = NEW.plan_item_ordinal
						AND association.evidence_outcome = 'fail')
				AND NEW.unknown_count = (SELECT COUNT(*)
					FROM operator_verification_plan_evidence_associations association
					WHERE association.run_id = NEW.run_id AND association.plan_id = NEW.plan_id
						AND association.plan_item_ordinal = NEW.plan_item_ordinal
						AND association.evidence_outcome = 'unknown')
				AND event.type = 'verification.snapshot_receipt_recorded'
				AND event.source = 'operator_verification_snapshot_receipt'
				AND event.subject_id = NEW.id AND event.created_at = NEW.created_at
				AND json_extract(event.payload_json, '$.plan_id') = NEW.plan_id
				AND json_extract(event.payload_json, '$.plan_sha256') = NEW.plan_sha256
				AND json_extract(event.payload_json, '$.plan_item_ordinal') = NEW.plan_item_ordinal
				AND json_extract(event.payload_json, '$.plan_item_sha256') = NEW.plan_item_sha256
				AND json_extract(event.payload_json, '$.format') = NEW.format
				AND json_extract(event.payload_json, '$.snapshot_high_water_event_sequence') =
					NEW.snapshot_high_water_event_sequence
				AND json_extract(event.payload_json, '$.associated_evidence_count') = NEW.associated_evidence_count
				AND json_extract(event.payload_json, '$.pass_count') = NEW.pass_count
				AND json_extract(event.payload_json, '$.fail_count') = NEW.fail_count
				AND json_extract(event.payload_json, '$.unknown_count') = NEW.unknown_count
				AND json_extract(event.payload_json, '$.returned_association_count') =
					NEW.returned_association_count
				AND json_extract(event.payload_json, '$.associations_truncated') = NEW.associations_truncated
				AND json_extract(event.payload_json, '$.content_sha256') = NEW.content_sha256
				AND json_extract(event.payload_json, '$.content_bytes') = NEW.content_bytes
				AND json_extract(event.payload_json, '$.operator_recorded') = 1
				AND json_extract(event.payload_json, '$.metadata_only') = 1
				AND json_extract(event.payload_json, '$.snapshot_accepted') = 0
				AND json_extract(event.payload_json, '$.result_accepted') = 0
				AND json_extract(event.payload_json, '$.result_inferred') = 0
				AND json_extract(event.payload_json, '$.private_bodies_included') = 0
				AND json_extract(event.payload_json, '$.operator_identity_included') = 0
				AND json_extract(event.payload_json, '$.record_rewritten') = 0
				AND json_extract(event.payload_json, '$.approval') = 0
				AND json_extract(event.payload_json, '$.authority_granted') = 0
				AND json_extract(event.payload_json, '$.execution_started') = 0
		)
		BEGIN SELECT RAISE(ABORT, 'verification snapshot receipt binding is invalid'); END;`,
	`CREATE TRIGGER trg_operator_verification_snapshot_receipt_update_immutable
		BEFORE UPDATE ON operator_verification_snapshot_receipts BEGIN
			SELECT RAISE(ABORT, 'verification snapshot receipt cannot be updated');
		END;`,
	`CREATE TRIGGER trg_operator_verification_snapshot_receipt_delete_immutable
		BEFORE DELETE ON operator_verification_snapshot_receipts BEGIN
			SELECT RAISE(ABORT, 'verification snapshot receipt cannot be deleted');
		END;`,
}
