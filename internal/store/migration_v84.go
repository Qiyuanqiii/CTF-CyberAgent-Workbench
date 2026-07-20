package store

var operatorVerificationSnapshotReceiptReviewStatements = []string{
	`CREATE TABLE operator_verification_snapshot_receipt_reviews (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		run_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		receipt_id TEXT NOT NULL UNIQUE,
		receipt_content_sha256 TEXT NOT NULL,
		receipt_event_sequence INTEGER NOT NULL,
		decision TEXT NOT NULL,
		reviewed_by TEXT NOT NULL,
		event_sequence INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		FOREIGN KEY(receipt_id) REFERENCES operator_verification_snapshot_receipts(id)
			ON DELETE RESTRICT,
		CHECK(protocol_version =
			'operator_verification_plan_item_snapshot_receipt_review.v1'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(receipt_content_sha256) = 64
			AND receipt_content_sha256 = lower(receipt_content_sha256)
			AND receipt_content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(receipt_event_sequence > 0),
		CHECK(decision IN ('metadata_confirmed', 'metadata_disputed')),
		CHECK(reviewed_by = trim(reviewed_by) AND length(reviewed_by) BETWEEN 1 AND 256
			AND instr(reviewed_by, char(0)) = 0),
		CHECK(event_sequence > receipt_event_sequence)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_operator_verification_snapshot_receipt_reviews_run_event
		ON operator_verification_snapshot_receipt_reviews(run_id, event_sequence DESC, id DESC);`,
	`CREATE TRIGGER trg_operator_verification_snapshot_receipt_review_insert
		BEFORE INSERT ON operator_verification_snapshot_receipt_reviews
		WHEN NOT EXISTS (
			SELECT 1 FROM operator_verification_snapshot_receipts receipt
			JOIN runs run ON run.id = receipt.run_id
			JOIN missions mission ON mission.id = run.mission_id
			JOIN sessions session_record ON session_record.id = run.session_id
			JOIN run_mode_snapshots mode ON mode.run_id = run.id
			JOIN run_events event ON event.run_id = run.id AND event.sequence = NEW.event_sequence
			WHERE receipt.id = NEW.receipt_id AND receipt.run_id = NEW.run_id
				AND receipt.session_id = NEW.session_id
				AND receipt.workspace_id = NEW.workspace_id
				AND receipt.content_sha256 = NEW.receipt_content_sha256
				AND receipt.event_sequence = NEW.receipt_event_sequence
				AND run.session_id = NEW.session_id
				AND mission.workspace_id = NEW.workspace_id
				AND session_record.workspace_id = NEW.workspace_id
				AND session_record.status = 'active' AND mode.surface = 'code'
				AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
					WHERE later.run_id = mode.run_id AND later.revision > mode.revision)
				AND event.type = 'verification.snapshot_receipt_review_recorded'
				AND event.source = 'operator_verification_snapshot_receipt_review'
				AND event.subject_id = NEW.id AND event.created_at = NEW.created_at
				AND json_extract(event.payload_json, '$.receipt_id') = NEW.receipt_id
				AND json_extract(event.payload_json, '$.receipt_content_sha256') =
					NEW.receipt_content_sha256
				AND json_extract(event.payload_json, '$.receipt_event_sequence') =
					NEW.receipt_event_sequence
				AND json_extract(event.payload_json, '$.decision') = NEW.decision
				AND json_extract(event.payload_json, '$.operator_reviewed') = 1
				AND json_extract(event.payload_json, '$.metadata_only') = 1
				AND json_extract(event.payload_json, '$.review_non_authorizing') = 1
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
		BEGIN SELECT RAISE(ABORT, 'verification snapshot receipt review binding is invalid'); END;`,
	`CREATE TRIGGER trg_operator_verification_snapshot_receipt_review_update_immutable
		BEFORE UPDATE ON operator_verification_snapshot_receipt_reviews BEGIN
			SELECT RAISE(ABORT, 'verification snapshot receipt review cannot be updated');
		END;`,
	`CREATE TRIGGER trg_operator_verification_snapshot_receipt_review_delete_immutable
		BEFORE DELETE ON operator_verification_snapshot_receipt_reviews BEGIN
			SELECT RAISE(ABORT, 'verification snapshot receipt review cannot be deleted');
		END;`,
}
