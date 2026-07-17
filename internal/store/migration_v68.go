package store

var sandboxDockerProductionEvidenceReviewStatements = []string{
	`CREATE TABLE sandbox_docker_production_evidence_reviews (
		id TEXT PRIMARY KEY,
		evidence_id TEXT NOT NULL UNIQUE,
		attempt_id TEXT NOT NULL UNIQUE,
		start_gate_review_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		evidence_operation_key_digest TEXT NOT NULL,
		evidence_capture_fingerprint TEXT NOT NULL,
		harness_result_fingerprint TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		threat_model_fingerprint TEXT NOT NULL,
		suite_fingerprint TEXT NOT NULL,
		environment_fingerprint TEXT NOT NULL,
		decision TEXT NOT NULL,
		reason_code TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		receipt_accepted INTEGER NOT NULL,
		real_daemon_contacted INTEGER NOT NULL,
		required_check_count INTEGER NOT NULL,
		observed_count INTEGER NOT NULL,
		production_verified_count INTEGER NOT NULL,
		sufficient_check_count INTEGER NOT NULL,
		blocker_count INTEGER NOT NULL,
		start_gate_passed INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		review_fingerprint TEXT NOT NULL,
		reviewed_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(evidence_id) REFERENCES sandbox_docker_production_evidence(id) ON DELETE RESTRICT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(start_gate_review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_production_evidence_review.v1'),
		CHECK(decision IN ('accepted', 'rejected')),
		CHECK((decision = 'accepted' AND reason_code = 'metadata_scope_accepted'
				AND receipt_accepted = 1)
			OR (decision = 'rejected' AND reason_code IN ('integrity_concern',
				'environment_concern', 'scope_concern', 'insufficient_evidence',
				'operator_rejected') AND receipt_accepted = 0)),
		CHECK(trust_class = 'operator_receipt_review_non_authorizing'),
		CHECK(operator_confirmed = 1 AND real_daemon_contacted = 1),
		CHECK(required_check_count = 16 AND observed_count = 16
			AND production_verified_count = 0 AND sufficient_check_count = 0
			AND blocker_count = 16),
		CHECK(start_gate_passed = 0 AND container_start_authorized = 0
			AND process_execution_authorized = 0 AND output_export_authorized = 0
			AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(evidence_operation_key_digest) = 64
			AND evidence_operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(evidence_capture_fingerprint) = 64
			AND evidence_capture_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(harness_result_fingerprint) = 64
			AND harness_result_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(threat_model_fingerprint) = 64
			AND threat_model_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(suite_fingerprint) = 64 AND suite_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(environment_fingerprint) = 64
			AND environment_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(review_fingerprint) = 64 AND review_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(reviewed_by = trim(reviewed_by) AND length(reviewed_by) BETWEEN 1 AND 256
			AND instr(reviewed_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_production_evidence_reviews_run_created
		ON sandbox_docker_production_evidence_reviews(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_production_evidence_review_operations (
		key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		review_id TEXT NOT NULL UNIQUE,
		evidence_id TEXT NOT NULL UNIQUE,
		attempt_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		reviewed_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_production_evidence_reviews(id)
			ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
		FOREIGN KEY(evidence_id) REFERENCES sandbox_docker_production_evidence(id) ON DELETE RESTRICT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(key_digest) = 64 AND key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(reviewed_by = trim(reviewed_by) AND length(reviewed_by) BETWEEN 1 AND 256
			AND instr(reviewed_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_review_operation_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_review_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence evidence
			JOIN sandbox_docker_production_evidence_harness_results harness
				ON harness.evidence_id = evidence.id
			JOIN sandbox_docker_production_evidence_attempts attempt
				ON attempt.id = harness.attempt_id
			WHERE evidence.id = NEW.evidence_id AND attempt.id = NEW.attempt_id
				AND evidence.run_id = NEW.run_id AND attempt.run_id = NEW.run_id
				AND evidence.status = 'capture_complete' AND evidence.real_daemon_contacted = 1
				AND evidence.required_check_count = 16 AND evidence.observed_count = 16
				AND evidence.production_verified_count = 0
				AND evidence.sufficient_check_count = 0 AND evidence.blocker_count = 16
				AND evidence.start_gate_passed = 0
				AND evidence.container_start_authorized = 0
				AND evidence.process_execution_authorized = 0
				AND evidence.output_export_authorized = 0
				AND evidence.artifact_commit_authorized = 0
				AND harness.evidence_capture_fingerprint = evidence.capture_fingerprint
				AND harness.observed_count = 16 AND harness.production_verified_count = 0
				AND harness.container_start_authorized = 0
				AND harness.process_execution_authorized = 0
				AND harness.output_export_authorized = 0
				AND harness.artifact_commit_authorized = 0
				AND NOT EXISTS (SELECT 1
					FROM sandbox_docker_production_evidence_attempt_results legacy
					WHERE legacy.attempt_id = attempt.id)
				AND julianday(NEW.created_at) >= julianday(harness.created_at)
		) OR EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_reviews existing
			WHERE existing.evidence_id = NEW.evidence_id OR existing.attempt_id = NEW.attempt_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence review requires one unreviewed v67 harness receipt');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_review_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_reviews
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_review_operations operation
			JOIN sandbox_docker_production_evidence evidence ON evidence.id = NEW.evidence_id
			JOIN sandbox_docker_production_evidence_operations evidence_operation
				ON evidence_operation.evidence_id = evidence.id
			JOIN sandbox_docker_production_evidence_harness_results harness
				ON harness.evidence_id = evidence.id
			JOIN sandbox_docker_production_evidence_attempts attempt
				ON attempt.id = harness.attempt_id
			JOIN sandbox_docker_start_gate_reviews gate_review
				ON gate_review.id = evidence.review_id
			WHERE operation.review_id = NEW.id AND operation.key_digest = NEW.operation_key_digest
				AND operation.request_fingerprint = NEW.request_fingerprint
				AND operation.evidence_id = NEW.evidence_id AND operation.attempt_id = NEW.attempt_id
				AND operation.run_id = NEW.run_id AND operation.reviewed_by = NEW.reviewed_by
				AND operation.created_at = NEW.created_at
				AND evidence.id = NEW.evidence_id AND attempt.id = NEW.attempt_id
				AND gate_review.id = NEW.start_gate_review_id
				AND evidence.run_id = NEW.run_id AND evidence.mission_id = NEW.mission_id
				AND evidence.workspace_id = NEW.workspace_id
				AND evidence.operation_key_digest = NEW.evidence_operation_key_digest
				AND evidence_operation.key_digest = NEW.evidence_operation_key_digest
				AND evidence.capture_fingerprint = NEW.evidence_capture_fingerprint
				AND harness.evidence_capture_fingerprint = NEW.evidence_capture_fingerprint
				AND harness.result_fingerprint = NEW.harness_result_fingerprint
				AND evidence.authority_fingerprint = NEW.authority_fingerprint
				AND evidence.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND evidence.suite_fingerprint = NEW.suite_fingerprint
				AND evidence.environment_fingerprint = NEW.environment_fingerprint
				AND attempt.review_id = NEW.start_gate_review_id
				AND attempt.run_id = NEW.run_id AND attempt.mission_id = NEW.mission_id
				AND attempt.workspace_id = NEW.workspace_id
				AND attempt.operation_key_digest = NEW.evidence_operation_key_digest
				AND gate_review.run_id = NEW.run_id AND gate_review.mission_id = NEW.mission_id
				AND gate_review.workspace_id = NEW.workspace_id
				AND gate_review.authority_fingerprint = NEW.authority_fingerprint
				AND gate_review.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND gate_review.status = 'blocked' AND gate_review.decision = 'deny_start'
				AND gate_review.start_gate_passed = 0
				AND gate_review.container_start_authorized = 0
				AND gate_review.process_execution_authorized = 0
				AND gate_review.output_export_authorized = 0
				AND gate_review.artifact_commit_authorized = 0
				AND evidence.status = 'capture_complete' AND evidence.real_daemon_contacted = 1
				AND evidence.required_check_count = 16 AND evidence.observed_count = 16
				AND evidence.production_verified_count = 0
				AND evidence.sufficient_check_count = 0 AND evidence.blocker_count = 16
				AND evidence.start_gate_passed = 0
				AND evidence.container_start_authorized = 0
				AND evidence.process_execution_authorized = 0
				AND evidence.output_export_authorized = 0
				AND evidence.artifact_commit_authorized = 0
				AND harness.real_daemon_contacted = 1 AND harness.daemon_write_authorized = 0
				AND harness.container_start_authorized = 0
				AND harness.process_execution_authorized = 0
				AND harness.output_export_authorized = 0
				AND harness.artifact_commit_authorized = 0
				AND (SELECT COUNT(*) FROM sandbox_docker_production_evidence_items item
					WHERE item.evidence_id = evidence.id) = 16
				AND (SELECT COUNT(*) FROM sandbox_docker_production_evidence_items item
					WHERE item.evidence_id = evidence.id AND item.state = 'observed_failed'
						AND item.observed = 1 AND item.production_verified = 0
						AND item.sufficient_for_start = 0) = 16
				AND NOT EXISTS (SELECT 1
					FROM sandbox_docker_production_evidence_attempt_results legacy
					WHERE legacy.attempt_id = attempt.id)
				AND julianday(NEW.created_at) >= julianday(evidence.created_at)
				AND julianday(NEW.created_at) >= julianday(harness.created_at)
				AND (SELECT COUNT(*) FROM sandbox_docker_production_evidence_reviews existing
					WHERE existing.run_id = NEW.run_id) < 32
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence review authority binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_review_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_reviews BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence review cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_review_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_reviews BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence review cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_review_operation_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_review_operations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence review operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_review_operation_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_review_operations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence review operation cannot be deleted');
		END;`,
}
