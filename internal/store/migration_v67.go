package store

var sandboxDockerProductionEvidenceHarnessStatements = []string{
	`CREATE TABLE sandbox_docker_production_evidence_harness_intents (
		attempt_id TEXT PRIMARY KEY,
		review_id TEXT NOT NULL,
		container_plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		image_digest TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		label_selector_fingerprint TEXT NOT NULL,
		max_daemon_reads INTEGER NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		readonly_daemon_contact_authorized INTEGER NOT NULL,
		daemon_write_authorized INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_production_evidence_harness_intent.v1'),
		CHECK(length(image_digest) = 71 AND substr(image_digest, 1, 7) = 'sha256:'
			AND substr(image_digest, 8) NOT GLOB '*[^0-9a-f]*'),
		CHECK(endpoint_class = 'local_unix' AND max_daemon_reads = 5),
		CHECK(operator_confirmed = 1 AND readonly_daemon_contact_authorized = 1),
		CHECK(daemon_write_authorized = 0 AND container_start_authorized = 0
			AND process_execution_authorized = 0 AND output_export_authorized = 0
			AND artifact_commit_authorized = 0),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(label_selector_fingerprint) = 64
			AND label_selector_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_production_evidence_harness_reconciliations (
		attempt_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		control_reconciliation_fingerprint TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		inventory_fingerprint TEXT NOT NULL,
		real_daemon_contacted INTEGER NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		owned_resource_count INTEGER NOT NULL,
		reconciliation_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(attempt_id, generation),
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		CHECK(generation >= 1),
		CHECK(protocol_version = 'sandbox_docker_production_evidence_harness_reconciliation.v1'),
		CHECK(status = 'owned_scope_empty' AND endpoint_class = 'local_unix'),
		CHECK(real_daemon_contacted = 1 AND daemon_read_count = 1
			AND owned_resource_count = 0),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(control_reconciliation_fingerprint) = 64
			AND control_reconciliation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(inventory_fingerprint) = 64 AND inventory_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(reconciliation_fingerprint) = 64
			AND reconciliation_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_production_evidence_harness_results (
		attempt_id TEXT PRIMARY KEY,
		evidence_id TEXT NOT NULL UNIQUE,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		reconciliation_fingerprint TEXT NOT NULL,
		evidence_capture_fingerprint TEXT NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		probe_count INTEGER NOT NULL,
		observed_count INTEGER NOT NULL,
		production_verified_count INTEGER NOT NULL,
		real_daemon_contacted INTEGER NOT NULL,
		daemon_write_authorized INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		result_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(evidence_id) REFERENCES sandbox_docker_production_evidence(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_production_evidence_harness_result.v1'),
		CHECK(status = 'evidence_committed' AND lease_generation >= 1),
		CHECK(daemon_read_count = 5 AND probe_count = 16 AND observed_count = 16
			AND production_verified_count = 0),
		CHECK(real_daemon_contacted = 1 AND daemon_write_authorized = 0
			AND container_start_authorized = 0 AND process_execution_authorized = 0
			AND output_export_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(reconciliation_fingerprint) = 64
			AND reconciliation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(evidence_capture_fingerprint) = 64
			AND evidence_capture_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(result_fingerprint) = 64 AND result_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_intent_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_harness_intents
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_attempts attempt
			JOIN sandbox_docker_production_evidence_attempt_leases lease
				ON lease.attempt_id = attempt.id
			JOIN sandbox_docker_production_evidence_reconciliations reconciliation
				ON reconciliation.attempt_id = attempt.id
				AND reconciliation.generation = lease.generation
			JOIN sandbox_docker_start_gate_reviews review ON review.id = attempt.review_id
			JOIN sandbox_docker_container_plans plan ON plan.id = review.container_plan_id
			WHERE attempt.id = NEW.attempt_id AND attempt.review_id = NEW.review_id
				AND attempt.run_id = NEW.run_id AND attempt.requested_by = NEW.requested_by
				AND attempt.endpoint_class = NEW.endpoint_class
				AND attempt.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND review.container_plan_id = NEW.container_plan_id
				AND review.run_id = NEW.run_id AND review.requested_by = NEW.requested_by
				AND plan.run_id = NEW.run_id AND plan.image_digest = NEW.image_digest
				AND plan.requested_by = NEW.requested_by
				AND lease.status = 'active'
				AND julianday(NEW.created_at) >= julianday(reconciliation.created_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness intent authority is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_reconciliation_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_harness_reconciliations
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_harness_intents intent
			JOIN sandbox_docker_production_evidence_attempt_leases lease
				ON lease.attempt_id = intent.attempt_id
			JOIN sandbox_docker_production_evidence_reconciliations control
				ON control.attempt_id = intent.attempt_id
				AND control.generation = lease.generation
			WHERE intent.attempt_id = NEW.attempt_id
				AND intent.intent_fingerprint = NEW.intent_fingerprint
				AND intent.endpoint_class = NEW.endpoint_class
				AND intent.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND control.reconciliation_fingerprint = NEW.control_reconciliation_fingerprint
				AND lease.generation = NEW.generation AND lease.status = 'active'
				AND julianday(NEW.created_at) >= julianday(intent.created_at)
				AND julianday(NEW.created_at) >= julianday(control.created_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness reconciliation authority is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_result_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_harness_results
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_harness_intents intent
			JOIN sandbox_docker_production_evidence_attempts attempt
				ON attempt.id = intent.attempt_id
			JOIN sandbox_docker_production_evidence_attempt_leases lease
				ON lease.attempt_id = intent.attempt_id
			JOIN sandbox_docker_production_evidence_harness_reconciliations reconciliation
				ON reconciliation.attempt_id = intent.attempt_id
				AND reconciliation.generation = lease.generation
			JOIN sandbox_docker_production_evidence evidence ON evidence.id = NEW.evidence_id
			WHERE intent.attempt_id = NEW.attempt_id
				AND intent.intent_fingerprint = NEW.intent_fingerprint
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND reconciliation.reconciliation_fingerprint = NEW.reconciliation_fingerprint
				AND evidence.review_id = attempt.review_id AND evidence.run_id = attempt.run_id
				AND evidence.operation_key_digest = attempt.operation_key_digest
				AND evidence.capture_fingerprint = NEW.evidence_capture_fingerprint
				AND evidence.requested_by = attempt.requested_by
				AND evidence.created_at = NEW.created_at
				AND evidence.status = 'capture_complete' AND evidence.real_daemon_contacted = 1
				AND evidence.required_check_count = 16 AND evidence.observed_count = 16
				AND evidence.production_verified_count = 0
				AND NEW.production_verified_count = 0
				AND evidence.sufficient_check_count = 0 AND evidence.blocker_count = 16
				AND evidence.start_gate_passed = 0
				AND evidence.container_start_authorized = 0
				AND evidence.process_execution_authorized = 0
				AND evidence.output_export_authorized = 0
				AND evidence.artifact_commit_authorized = 0
				AND (SELECT COUNT(*) FROM sandbox_docker_production_evidence_items item
					WHERE item.evidence_id = evidence.id) = 16
				AND julianday(NEW.created_at) >= julianday(reconciliation.created_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness result authority is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_blocks_v66_result
		BEFORE INSERT ON sandbox_docker_production_evidence_attempt_results
		WHEN EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_harness_intents intent
			WHERE intent.attempt_id = NEW.attempt_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness requires a v67 result');
		END;`,
	`DROP TRIGGER trg_sandbox_docker_production_evidence_v66_attempt_required;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_v67_attempt_required
		BEFORE INSERT ON sandbox_docker_production_evidence_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_attempt_operations attempt_operation
			JOIN sandbox_docker_production_evidence_attempt_results result
				ON result.attempt_id = attempt_operation.attempt_id
			WHERE attempt_operation.key_digest = NEW.key_digest
				AND attempt_operation.request_fingerprint = NEW.request_fingerprint
				AND attempt_operation.review_id = NEW.review_id
				AND attempt_operation.run_id = NEW.run_id
				AND attempt_operation.requested_by = NEW.requested_by
				AND result.evidence_id = NEW.evidence_id
				AND result.created_at = NEW.created_at
		)
		AND NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_attempt_operations attempt_operation
			JOIN sandbox_docker_production_evidence_harness_results result
				ON result.attempt_id = attempt_operation.attempt_id
			WHERE attempt_operation.key_digest = NEW.key_digest
				AND attempt_operation.request_fingerprint = NEW.request_fingerprint
				AND attempt_operation.review_id = NEW.review_id
				AND attempt_operation.run_id = NEW.run_id
				AND attempt_operation.requested_by = NEW.requested_by
				AND result.evidence_id = NEW.evidence_id
				AND result.created_at = NEW.created_at
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence requires a write-ahead attempt');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_intent_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_harness_intents BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness intent cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_intent_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_harness_intents BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness intent cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_reconciliation_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_harness_reconciliations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness reconciliation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_reconciliation_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_harness_reconciliations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness reconciliation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_result_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_harness_results BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness result cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_harness_result_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_harness_results BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence harness result cannot be deleted');
		END;`,
}
