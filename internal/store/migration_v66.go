package store

var sandboxDockerProductionEvidenceAttemptStatements = []string{
	`CREATE TABLE sandbox_docker_production_evidence_attempts (
		id TEXT PRIMARY KEY,
		review_id TEXT NOT NULL,
		cleanup_intent_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL,
		review_fingerprint TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		threat_model_fingerprint TEXT NOT NULL,
		suite_fingerprint TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		capture_timeout_millis INTEGER NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		real_daemon_contact_authorized INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		attempt_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(cleanup_intent_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_production_evidence_attempt.v1'),
		CHECK(endpoint_class = 'local_unix'),
		CHECK(capture_timeout_millis BETWEEN 1000 AND 120000),
		CHECK(operator_confirmed = 1 AND real_daemon_contact_authorized = 0),
		CHECK(container_start_authorized = 0 AND process_execution_authorized = 0
			AND output_export_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(review_fingerprint) = 64 AND review_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(threat_model_fingerprint) = 64 AND threat_model_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(suite_fingerprint) = 64 AND suite_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(attempt_fingerprint) = 64 AND attempt_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_production_evidence_attempts_run_created
		ON sandbox_docker_production_evidence_attempts(run_id, created_at, id);`,
	`CREATE INDEX idx_sandbox_docker_production_evidence_attempts_review_created
		ON sandbox_docker_production_evidence_attempts(review_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_production_evidence_attempt_operations (
		key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		attempt_id TEXT NOT NULL UNIQUE,
		review_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(key_digest) = 64 AND key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_production_evidence_attempt_leases (
		attempt_id TEXT PRIMARY KEY,
		lease_id TEXT NOT NULL UNIQUE,
		owner_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		status TEXT NOT NULL,
		acquired_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		released_at TEXT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		CHECK(generation >= 1),
		CHECK(status IN ('active', 'released')),
		CHECK(julianday(expires_at) > julianday(acquired_at)),
		CHECK((status = 'active' AND released_at IS NULL)
			OR (status = 'released' AND released_at IS NOT NULL
				AND julianday(released_at) >= julianday(acquired_at))),
		CHECK(lease_id = trim(lease_id) AND length(lease_id) BETWEEN 1 AND 256
			AND instr(lease_id, char(0)) = 0),
		CHECK(owner_id = trim(owner_id) AND length(owner_id) BETWEEN 1 AND 256
			AND instr(owner_id, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_production_evidence_reconciliations (
		attempt_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		previous_generation INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		real_daemon_contacted INTEGER NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		reconciled_resource_count INTEGER NOT NULL,
		reconciliation_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(attempt_id, generation),
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		CHECK(generation >= 1 AND previous_generation >= 0),
		CHECK(protocol_version = 'sandbox_docker_production_evidence_reconciliation.v1'),
		CHECK((generation = 1 AND previous_generation = 0
				AND status = 'initial_generation_quiescent')
			OR (generation > 1 AND previous_generation = generation - 1
				AND status = 'restart_generation_quiescent')),
		CHECK(endpoint_class = 'local_unix'),
		CHECK(real_daemon_contacted = 0 AND daemon_read_count = 0
			AND reconciled_resource_count = 0),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(reconciliation_fingerprint) = 64
			AND reconciliation_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_production_evidence_attempt_failures (
		attempt_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		generation INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		code TEXT NOT NULL,
		failure_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(attempt_id, sequence),
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		CHECK(sequence BETWEEN 1 AND 16 AND generation >= 1),
		CHECK(protocol_version = 'sandbox_docker_production_evidence_attempt_failure.v1'),
		CHECK(code IN ('collector_failed', 'invalid_observation', 'unsafe_daemon_contact',
			'context_canceled', 'deadline_exceeded', 'persistence_failed')),
		CHECK(length(failure_fingerprint) = 64 AND failure_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_production_evidence_attempt_results (
		attempt_id TEXT PRIMARY KEY,
		evidence_id TEXT NOT NULL UNIQUE,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		reconciliation_fingerprint TEXT NOT NULL,
		evidence_capture_fingerprint TEXT NOT NULL,
		real_daemon_contacted INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		result_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_production_evidence_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(evidence_id) REFERENCES sandbox_docker_production_evidence(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_production_evidence_attempt_result.v1'),
		CHECK(status = 'evidence_committed' AND lease_generation >= 1),
		CHECK(real_daemon_contacted = 0 AND container_start_authorized = 0
			AND process_execution_authorized = 0 AND output_export_authorized = 0
			AND artifact_commit_authorized = 0),
		CHECK(length(reconciliation_fingerprint) = 64
			AND reconciliation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(evidence_capture_fingerprint) = 64
			AND evidence_capture_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(result_fingerprint) = 64 AND result_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_attempts
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_start_gate_reviews review
			JOIN runs run ON run.id = review.run_id
			JOIN missions mission ON mission.id = review.mission_id
			WHERE review.id = NEW.review_id AND review.cleanup_intent_id = NEW.cleanup_intent_id
				AND review.run_id = NEW.run_id AND review.mission_id = NEW.mission_id
				AND review.workspace_id = NEW.workspace_id
				AND review.review_fingerprint = NEW.review_fingerprint
				AND review.authority_fingerprint = NEW.authority_fingerprint
				AND review.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND review.requested_by = NEW.requested_by
				AND review.status = 'blocked' AND review.decision = 'deny_start'
				AND review.start_gate_passed = 0 AND review.start_implementation_present = 0
				AND review.container_start_authorized = 0
				AND review.process_execution_authorized = 0
				AND review.output_export_authorized = 0
				AND review.artifact_commit_authorized = 0
				AND run.mission_id = NEW.mission_id AND mission.workspace_id = NEW.workspace_id
				AND julianday(NEW.created_at) >= julianday(review.created_at)
				AND (SELECT COUNT(*) FROM sandbox_docker_production_evidence_attempts existing
					WHERE existing.run_id = NEW.run_id) < 32
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt authority binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_operation_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_attempt_operations
		WHEN EXISTS (SELECT 1 FROM sandbox_docker_production_evidence_operations legacy
				WHERE legacy.key_digest = NEW.key_digest)
			OR NOT EXISTS (
				SELECT 1 FROM sandbox_docker_production_evidence_attempts attempt
				WHERE attempt.id = NEW.attempt_id
					AND attempt.operation_key_digest = NEW.key_digest
					AND attempt.request_fingerprint = NEW.request_fingerprint
					AND attempt.review_id = NEW.review_id AND attempt.run_id = NEW.run_id
					AND attempt.requested_by = NEW.requested_by
					AND attempt.created_at = NEW.created_at)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_reconciliation_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_reconciliations
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_attempts attempt
			JOIN sandbox_docker_production_evidence_attempt_leases lease
				ON lease.attempt_id = attempt.id
			WHERE attempt.id = NEW.attempt_id AND attempt.endpoint_class = NEW.endpoint_class
				AND attempt.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND lease.generation = NEW.generation AND lease.status = 'active'
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence reconciliation lease binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_failure_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_attempt_failures
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_attempt_leases lease
			JOIN sandbox_docker_production_evidence_reconciliations reconciliation
				ON reconciliation.attempt_id = lease.attempt_id
				AND reconciliation.generation = lease.generation
			WHERE lease.attempt_id = NEW.attempt_id AND lease.generation = NEW.generation
				AND lease.status = 'active'
				AND NEW.sequence = 1 + (SELECT COUNT(*)
					FROM sandbox_docker_production_evidence_attempt_failures prior
					WHERE prior.attempt_id = NEW.attempt_id)
				AND julianday(NEW.created_at) >= julianday(reconciliation.created_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt failure binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_result_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_attempt_results
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence_attempts attempt
			JOIN sandbox_docker_production_evidence_attempt_leases lease
				ON lease.attempt_id = attempt.id
			JOIN sandbox_docker_production_evidence_reconciliations reconciliation
				ON reconciliation.attempt_id = attempt.id
				AND reconciliation.generation = lease.generation
			JOIN sandbox_docker_production_evidence evidence
				ON evidence.id = NEW.evidence_id
			WHERE attempt.id = NEW.attempt_id AND lease.status = 'active'
				AND lease.generation = NEW.lease_generation
				AND reconciliation.reconciliation_fingerprint = NEW.reconciliation_fingerprint
				AND evidence.review_id = attempt.review_id AND evidence.run_id = attempt.run_id
				AND evidence.operation_key_digest = attempt.operation_key_digest
				AND evidence.capture_fingerprint = NEW.evidence_capture_fingerprint
				AND evidence.requested_by = attempt.requested_by
				AND evidence.real_daemon_contacted = 0
				AND julianday(NEW.created_at) >= julianday(reconciliation.created_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt result binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_v66_attempt_required
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
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence requires a write-ahead attempt');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_lease_update
		BEFORE UPDATE ON sandbox_docker_production_evidence_attempt_leases
		WHEN NOT (
			OLD.attempt_id = NEW.attempt_id AND (
				(OLD.lease_id = NEW.lease_id AND OLD.owner_id = NEW.owner_id
					AND OLD.generation = NEW.generation AND OLD.status = 'active'
					AND NEW.status = 'released' AND OLD.acquired_at = NEW.acquired_at
					AND OLD.expires_at = NEW.expires_at AND NEW.released_at IS NOT NULL
					AND julianday(NEW.released_at) < julianday(OLD.expires_at))
				OR
				(NEW.generation = OLD.generation + 1 AND NEW.status = 'active'
					AND NEW.released_at IS NULL AND NEW.lease_id <> OLD.lease_id
					AND julianday(NEW.expires_at) > julianday(NEW.acquired_at)
					AND ((OLD.status = 'released'
							AND julianday(NEW.acquired_at) >= julianday(OLD.released_at))
						OR (OLD.status = 'active'
							AND julianday(OLD.expires_at) <= julianday(NEW.acquired_at))))
			)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt lease transition is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_lease_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_attempt_leases BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt lease cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_operation_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_attempt_operations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_operation_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_attempt_operations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt operation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_reconciliation_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_reconciliations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence reconciliation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_reconciliation_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_reconciliations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence reconciliation cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_failure_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_attempt_failures BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt failure cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_failure_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_attempt_failures BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt failure cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_result_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_attempt_results BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt result cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_attempt_result_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_attempt_results BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence attempt result cannot be deleted');
		END;`,
}
