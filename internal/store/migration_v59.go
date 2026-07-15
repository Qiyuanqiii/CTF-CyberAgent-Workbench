package store

var sandboxDockerHostInputHandoffStatements = []string{
	`CREATE TABLE sandbox_docker_host_input_handoff_legacy_attempts (
		attempt_id TEXT PRIMARY KEY,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT
	) WITHOUT ROWID;`,
	`INSERT INTO sandbox_docker_host_input_handoff_legacy_attempts (attempt_id)
		SELECT id FROM sandbox_docker_container_rehearsal_attempts;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_legacy_insert_immutable
		BEFORE INSERT ON sandbox_docker_host_input_handoff_legacy_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff legacy marker cannot be inserted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_legacy_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_handoff_legacy_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff legacy marker cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_legacy_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_handoff_legacy_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff legacy marker cannot be deleted');
		END;`,
	`CREATE TABLE sandbox_docker_host_input_handoff_requirements (
		attempt_id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		attempt_intent_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		capture_requirement_fingerprint TEXT NOT NULL,
		manifest_fingerprint TEXT NOT NULL,
		mount_binding_fingerprint TEXT NOT NULL,
		input_artifact_digest TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		plan_fingerprint TEXT NOT NULL,
		required INTEGER NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		read_only_mount_count INTEGER NOT NULL,
		input_artifact_count INTEGER NOT NULL,
		requirement_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_host_input_handoff_requirement.v1'),
		CHECK(required IN (0, 1) AND operator_confirmed = required),
		CHECK(read_only_mount_count BETWEEN 0 AND 32 AND (required = 0 OR read_only_mount_count >= 1)),
		CHECK(input_artifact_count BETWEEN 0 AND 16),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(attempt_intent_fingerprint) = 64 AND attempt_intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(capture_requirement_fingerprint) = 64 AND capture_requirement_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_binding_fingerprint) = 64 AND mount_binding_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_fingerprint) = 64 AND plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(requirement_fingerprint) = 64 AND requirement_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(attempt_id = trim(attempt_id) AND length(attempt_id) BETWEEN 1 AND 256 AND instr(attempt_id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256 AND instr(mission_id, char(0)) = 0),
		CHECK(workspace_id = trim(workspace_id) AND length(workspace_id) BETWEEN 1 AND 256 AND instr(workspace_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_sandbox_docker_host_input_handoff_requirements_run_created
		ON sandbox_docker_host_input_handoff_requirements(run_id, created_at, attempt_id);`,
	`CREATE TABLE sandbox_docker_host_input_handoff_intents (
		id TEXT PRIMARY KEY,
		attempt_id TEXT NOT NULL UNIQUE,
		staging_intent_id TEXT NOT NULL UNIQUE,
		staging_id TEXT NOT NULL UNIQUE,
		plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		attempt_intent_fingerprint TEXT NOT NULL,
		container_id_fingerprint TEXT NOT NULL,
		capture_requirement_fingerprint TEXT NOT NULL,
		handoff_requirement_fingerprint TEXT NOT NULL,
		staging_fingerprint TEXT NOT NULL,
		bundle_report_fingerprint TEXT NOT NULL,
		bundle_digest TEXT NOT NULL,
		bundle_bytes INTEGER NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		spec_fingerprint TEXT NOT NULL,
		plan_fingerprint TEXT NOT NULL,
		prepared_generation INTEGER NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(staging_intent_id) REFERENCES sandbox_docker_host_input_staging_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(staging_id) REFERENCES sandbox_docker_host_input_stagings(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_host_input_handoff_intent.v1'),
		CHECK(bundle_bytes BETWEEN 1 AND 41943040 AND prepared_generation >= 1),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(attempt_intent_fingerprint) = 64 AND attempt_intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_id_fingerprint) = 64 AND container_id_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(capture_requirement_fingerprint) = 64 AND capture_requirement_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(handoff_requirement_fingerprint) = 64 AND handoff_requirement_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(staging_fingerprint) = 64 AND staging_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(bundle_report_fingerprint) = 64 AND bundle_report_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(bundle_digest) = 64 AND bundle_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(spec_fingerprint) = 64 AND spec_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_fingerprint) = 64 AND plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(attempt_id = trim(attempt_id) AND length(attempt_id) BETWEEN 1 AND 256 AND instr(attempt_id, char(0)) = 0),
		CHECK(staging_intent_id = trim(staging_intent_id) AND length(staging_intent_id) BETWEEN 1 AND 256 AND instr(staging_intent_id, char(0)) = 0),
		CHECK(staging_id = trim(staging_id) AND length(staging_id) BETWEEN 1 AND 256 AND instr(staging_id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256 AND instr(mission_id, char(0)) = 0),
		CHECK(workspace_id = trim(workspace_id) AND length(workspace_id) BETWEEN 1 AND 256 AND instr(workspace_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_sandbox_docker_host_input_handoff_intents_run_created
		ON sandbox_docker_host_input_handoff_intents(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_host_input_handoffs (
		id TEXT PRIMARY KEY,
		intent_id TEXT NOT NULL UNIQUE,
		attempt_id TEXT NOT NULL UNIQUE,
		plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		handoff_fingerprint TEXT NOT NULL,
		source TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		status TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		bundle_report_fingerprint TEXT NOT NULL,
		bundle_digest TEXT NOT NULL,
		readback_digest TEXT NOT NULL,
		carrier_name_fingerprint TEXT NOT NULL,
		volume_name_fingerprint TEXT NOT NULL,
		final_container_id_fingerprint TEXT NOT NULL,
		transport_fingerprint TEXT NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		daemon_write_count INTEGER NOT NULL,
		reconciled_resource_count INTEGER NOT NULL,
		daemon_consumed INTEGER NOT NULL,
		readback_verified INTEGER NOT NULL,
		final_mount_read_only INTEGER NOT NULL,
		carrier_removed INTEGER NOT NULL,
		final_container_removed INTEGER NOT NULL,
		volume_removed INTEGER NOT NULL,
		cleanup_confirmed INTEGER NOT NULL,
		container_started INTEGER NOT NULL,
		process_executed INTEGER NOT NULL,
		output_exported INTEGER NOT NULL,
		raw_content_retained INTEGER NOT NULL,
		production_execution_submitted INTEGER NOT NULL,
		production_verified INTEGER NOT NULL,
		backend_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_host_input_handoff_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_host_input_handoff.v1'),
		CHECK(source = 'local_docker_volume_carrier'),
		CHECK(trust_class = 'daemon_readback_verified_never_started'),
		CHECK(status = 'daemon_handoff_cleaned'),
		CHECK(endpoint_class = 'local_unix'),
		CHECK(lease_generation >= 1),
		CHECK(daemon_read_count BETWEEN 6 AND 32),
		CHECK(daemon_write_count BETWEEN 7 AND 24),
		CHECK(reconciled_resource_count BETWEEN 0 AND 3),
		CHECK(daemon_consumed = 1 AND readback_verified = 1 AND final_mount_read_only = 1),
		CHECK(carrier_removed = 1 AND final_container_removed = 1 AND volume_removed = 1 AND cleanup_confirmed = 1),
		CHECK(container_started = 0 AND process_executed = 0 AND output_exported = 0 AND raw_content_retained = 0),
		CHECK(production_execution_submitted = 0 AND production_verified = 0 AND backend_enabled = 0
			AND execution_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(readback_digest = bundle_digest),
		CHECK(length(handoff_fingerprint) = 64 AND handoff_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(bundle_report_fingerprint) = 64 AND bundle_report_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(bundle_digest) = 64 AND bundle_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(readback_digest) = 64 AND readback_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(carrier_name_fingerprint) = 64 AND carrier_name_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(volume_name_fingerprint) = 64 AND volume_name_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(final_container_id_fingerprint) = 64 AND final_container_id_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(transport_fingerprint) = 64 AND transport_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(intent_id = trim(intent_id) AND length(intent_id) BETWEEN 1 AND 256 AND instr(intent_id, char(0)) = 0),
		CHECK(attempt_id = trim(attempt_id) AND length(attempt_id) BETWEEN 1 AND 256 AND instr(attempt_id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_sandbox_docker_host_input_handoffs_run_created
		ON sandbox_docker_host_input_handoffs(run_id, created_at, id);`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_requirement_insert
		BEFORE INSERT ON sandbox_docker_host_input_handoff_requirements
		WHEN EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoff_legacy_attempts legacy
			WHERE legacy.attempt_id = NEW.attempt_id)
			OR NOT EXISTS (
				SELECT 1 FROM sandbox_docker_container_rehearsal_attempts attempt
				JOIN sandbox_docker_host_input_requirements capture ON capture.attempt_id = attempt.id
				WHERE attempt.id = NEW.attempt_id AND attempt.plan_id = NEW.plan_id
					AND attempt.run_id = NEW.run_id AND attempt.mission_id = NEW.mission_id
					AND attempt.workspace_id = NEW.workspace_id
					AND attempt.operation_key_digest = NEW.operation_key_digest
					AND attempt.intent_fingerprint = NEW.attempt_intent_fingerprint
					AND attempt.request_fingerprint = NEW.request_fingerprint
					AND capture.requirement_fingerprint = NEW.capture_requirement_fingerprint
					AND (NEW.required = 0 OR capture.required = 1)
					AND attempt.manifest_fingerprint = NEW.manifest_fingerprint
					AND attempt.mount_binding_fingerprint = NEW.mount_binding_fingerprint
					AND attempt.input_artifact_digest = NEW.input_artifact_digest
					AND attempt.authority_fingerprint = NEW.authority_fingerprint
					AND attempt.plan_fingerprint = NEW.plan_fingerprint
					AND attempt.requested_by = NEW.requested_by
					AND attempt.created_at = NEW.created_at
					AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_stages stage
						WHERE stage.attempt_id = attempt.id)
			) BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff requirement authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_attempt_stage_requires_handoff_requirement
		BEFORE INSERT ON sandbox_docker_container_attempt_stages
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoff_requirements requirement
			WHERE requirement.attempt_id = NEW.attempt_id)
			AND NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoff_legacy_attempts legacy
				WHERE legacy.attempt_id = NEW.attempt_id) BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff requirement is not durable before stage');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_intent_insert
		BEFORE INSERT ON sandbox_docker_host_input_handoff_intents
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_container_rehearsal_attempts attempt
			JOIN sandbox_docker_container_attempt_stages stage ON stage.attempt_id = attempt.id
			JOIN sandbox_docker_container_attempt_leases lease ON lease.attempt_id = attempt.id
			JOIN sandbox_docker_host_input_requirements capture ON capture.attempt_id = attempt.id
			JOIN sandbox_docker_host_input_handoff_requirements requirement ON requirement.attempt_id = attempt.id
			JOIN sandbox_docker_host_input_staging_intents staging_intent ON staging_intent.attempt_id = attempt.id
			JOIN sandbox_docker_host_input_stagings staging ON staging.attempt_id = attempt.id
			WHERE attempt.id = NEW.attempt_id AND attempt.plan_id = NEW.plan_id
				AND attempt.run_id = NEW.run_id AND attempt.mission_id = NEW.mission_id
				AND attempt.workspace_id = NEW.workspace_id
				AND staging_intent.id = NEW.staging_intent_id AND staging.id = NEW.staging_id
				AND stage.container_id_fingerprint = NEW.container_id_fingerprint
				AND capture.required = 1 AND requirement.required = 1
				AND capture.requirement_fingerprint = NEW.capture_requirement_fingerprint
				AND requirement.requirement_fingerprint = NEW.handoff_requirement_fingerprint
				AND staging.staging_fingerprint = NEW.staging_fingerprint
				AND staging.report_fingerprint = NEW.bundle_report_fingerprint
				AND staging.bundle_digest = NEW.bundle_digest AND staging.bundle_bytes = NEW.bundle_bytes
				AND attempt.intent_fingerprint = NEW.attempt_intent_fingerprint
				AND attempt.authority_fingerprint = NEW.authority_fingerprint
				AND attempt.spec_fingerprint = NEW.spec_fingerprint
				AND attempt.plan_fingerprint = NEW.plan_fingerprint
				AND attempt.requested_by = NEW.requested_by
				AND lease.generation = NEW.prepared_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_cleanups cleanup
					WHERE cleanup.attempt_id = attempt.id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_completions completion
					WHERE completion.attempt_id = attempt.id)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff intent authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_insert
		BEFORE INSERT ON sandbox_docker_host_input_handoffs
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_host_input_handoff_intents intent
			JOIN sandbox_docker_container_attempt_leases lease ON lease.attempt_id = intent.attempt_id
			WHERE intent.id = NEW.intent_id AND intent.attempt_id = NEW.attempt_id
				AND intent.plan_id = NEW.plan_id AND intent.run_id = NEW.run_id
				AND intent.intent_fingerprint = NEW.intent_fingerprint
				AND intent.bundle_report_fingerprint = NEW.bundle_report_fingerprint
				AND intent.bundle_digest = NEW.bundle_digest
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND NEW.lease_generation >= intent.prepared_generation
				AND julianday(lease.expires_at) > julianday('now')
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_cleanups cleanup
					WHERE cleanup.attempt_id = intent.attempt_id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_completions completion
					WHERE completion.attempt_id = intent.attempt_id)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff lease mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_attempt_cleanup_requires_host_input_handoff
		BEFORE INSERT ON sandbox_docker_container_attempt_cleanups
		WHEN EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoff_requirements requirement
			WHERE requirement.attempt_id = NEW.attempt_id AND requirement.required = 1)
			AND NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoffs handoff
				WHERE handoff.attempt_id = NEW.attempt_id) BEGIN
			SELECT RAISE(ABORT, 'Required Docker host input handoff is incomplete before cleanup');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_attempt_completion_requires_host_input_handoff
		BEFORE INSERT ON sandbox_docker_container_attempt_completions
		WHEN (NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoff_requirements requirement
				WHERE requirement.attempt_id = NEW.attempt_id)
			AND NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoff_legacy_attempts legacy
				WHERE legacy.attempt_id = NEW.attempt_id))
			OR (EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoff_requirements requirement
				WHERE requirement.attempt_id = NEW.attempt_id AND requirement.required = 1)
			AND NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_handoffs handoff
				WHERE handoff.attempt_id = NEW.attempt_id)) BEGIN
			SELECT RAISE(ABORT, 'Required Docker host input handoff is incomplete');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_requirement_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_handoff_requirements BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff requirement cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_requirement_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_handoff_requirements BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff requirement cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_intent_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_handoff_intents BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff intent cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_intent_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_handoff_intents BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff intent cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_handoffs BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_handoff_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_handoffs BEGIN
			SELECT RAISE(ABORT, 'Docker host input handoff cannot be deleted');
		END;`,
}
