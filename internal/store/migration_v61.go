package store

var sandboxDockerRuntimeInputApplicationStatements = []string{
	`CREATE TABLE sandbox_docker_runtime_input_application_intents (
		id TEXT PRIMARY KEY,
		projection_id TEXT NOT NULL UNIQUE,
		handoff_id TEXT NOT NULL,
		handoff_intent_id TEXT NOT NULL,
		attempt_id TEXT NOT NULL,
		container_plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		manifest_fingerprint TEXT NOT NULL,
		mount_binding_fingerprint TEXT NOT NULL,
		input_artifact_digest TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		spec_fingerprint TEXT NOT NULL,
		container_plan_fingerprint TEXT NOT NULL,
		handoff_fingerprint TEXT NOT NULL,
		projection_set_fingerprint TEXT NOT NULL,
		projection_fingerprint TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		projection_count INTEGER NOT NULL,
		read_only_mount_count INTEGER NOT NULL,
		input_artifact_count INTEGER NOT NULL,
		total_entry_count INTEGER NOT NULL,
		total_content_bytes INTEGER NOT NULL,
		total_projection_bytes INTEGER NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		daemon_write_confirmed INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_completions(projection_id) ON DELETE RESTRICT,
		FOREIGN KEY(handoff_id) REFERENCES sandbox_docker_host_input_handoffs(id) ON DELETE RESTRICT,
		FOREIGN KEY(handoff_intent_id) REFERENCES sandbox_docker_host_input_handoff_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_runtime_input_application_intent.v1'),
		CHECK(endpoint_class = 'local_unix'),
		CHECK(projection_count BETWEEN 1 AND 33
			AND projection_count = read_only_mount_count + CASE WHEN input_artifact_count > 0 THEN 1 ELSE 0 END),
		CHECK(read_only_mount_count BETWEEN 1 AND 32),
		CHECK(input_artifact_count BETWEEN 0 AND 16),
		CHECK(total_entry_count BETWEEN read_only_mount_count AND 4096),
		CHECK(total_content_bytes BETWEEN 0 AND 33554432),
		CHECK(total_projection_bytes BETWEEN 1 AND 1384120320),
		CHECK(operator_confirmed = 1 AND daemon_write_confirmed = 1),
		CHECK(container_start_authorized = 0 AND process_execution_authorized = 0
			AND output_export_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_binding_fingerprint) = 64 AND mount_binding_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(spec_fingerprint) = 64 AND spec_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_plan_fingerprint) = 64 AND container_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(handoff_fingerprint) = 64 AND handoff_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(projection_set_fingerprint) = 64 AND projection_set_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(projection_fingerprint) = 64 AND projection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_runtime_input_application_intents_run_created
		ON sandbox_docker_runtime_input_application_intents(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_runtime_input_application_leases (
		intent_id TEXT PRIMARY KEY,
		lease_id TEXT NOT NULL UNIQUE,
		owner_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		status TEXT NOT NULL,
		acquired_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		released_at TEXT,
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_runtime_input_application_intents(id) ON DELETE RESTRICT,
		CHECK(generation >= 1),
		CHECK(status IN ('active', 'released')),
		CHECK(julianday(expires_at) > julianday(acquired_at)),
		CHECK((status = 'active' AND released_at IS NULL)
			OR (status = 'released' AND released_at IS NOT NULL AND julianday(released_at) >= julianday(acquired_at))),
		CHECK(lease_id = trim(lease_id) AND length(lease_id) BETWEEN 1 AND 256 AND instr(lease_id, char(0)) = 0),
		CHECK(owner_id = trim(owner_id) AND length(owner_id) BETWEEN 1 AND 256 AND instr(owner_id, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_runtime_input_application_failures (
		intent_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		generation INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		code TEXT NOT NULL,
		failure_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(intent_id, sequence),
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_runtime_input_application_intents(id) ON DELETE RESTRICT,
		CHECK(sequence BETWEEN 1 AND 16 AND generation >= 1),
		CHECK(protocol_version = 'sandbox_docker_runtime_input_application_failure.v1'),
		CHECK(code IN ('application_disabled', 'application_unsupported', 'connection_failed',
			'invalid_response', 'unsafe_resource_collision', 'projection_readback_mismatch',
			'target_configuration_mismatch', 'application_cleanup_failed',
			'context_canceled', 'deadline_exceeded')),
		CHECK(length(failure_fingerprint) = 64 AND failure_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_runtime_input_application_results (
		id TEXT PRIMARY KEY,
		intent_id TEXT NOT NULL UNIQUE,
		projection_id TEXT NOT NULL UNIQUE,
		container_plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		source TEXT NOT NULL,
		status TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		projection_fingerprint TEXT NOT NULL,
		target_container_fingerprint TEXT NOT NULL,
		target_inspection_fingerprint TEXT NOT NULL,
		transport_fingerprint TEXT NOT NULL,
		projection_count INTEGER NOT NULL,
		volume_created_count INTEGER NOT NULL,
		volume_present_count INTEGER NOT NULL,
		carrier_created_count INTEGER NOT NULL,
		carrier_removed_count INTEGER NOT NULL,
		readback_verified_count INTEGER NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		daemon_write_count INTEGER NOT NULL,
		reconciled_resource_count INTEGER NOT NULL,
		all_volumes_read_only INTEGER NOT NULL,
		all_volumes_no_copy INTEGER NOT NULL,
		all_projection_bytes_verified INTEGER NOT NULL,
		target_configuration_matched INTEGER NOT NULL,
		target_container_present INTEGER NOT NULL,
		container_started INTEGER NOT NULL,
		process_executed INTEGER NOT NULL,
		output_exported INTEGER NOT NULL,
		production_execution_submitted INTEGER NOT NULL,
		production_verified INTEGER NOT NULL,
		backend_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		result_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_runtime_input_application_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_completions(projection_id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_runtime_input_application_result.v1'),
		CHECK(source = 'local_unix_projection_volume_application'),
		CHECK(status = 'volumes_applied_target_never_started'),
		CHECK(trust_class = 'daemon_projection_readback_verified_never_started'),
		CHECK(lease_generation >= 1 AND endpoint_class = 'local_unix'),
		CHECK(projection_count BETWEEN 1 AND 33),
		CHECK(volume_created_count = projection_count AND volume_present_count = projection_count
			AND carrier_created_count = projection_count AND carrier_removed_count = projection_count
			AND readback_verified_count = projection_count),
		CHECK(daemon_read_count = 3 + 5 * projection_count),
		CHECK(reconciled_resource_count BETWEEN 0 AND 1 + 2 * projection_count),
		CHECK(daemon_write_count = 1 + 4 * projection_count + reconciled_resource_count),
		CHECK(all_volumes_read_only = 1 AND all_volumes_no_copy = 1
			AND all_projection_bytes_verified = 1 AND target_configuration_matched = 1
			AND target_container_present = 1),
		CHECK(container_started = 0 AND process_executed = 0 AND output_exported = 0
			AND production_execution_submitted = 0 AND production_verified = 0
			AND backend_enabled = 0 AND execution_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(projection_fingerprint) = 64 AND projection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(target_container_fingerprint) = 64 AND target_container_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(target_inspection_fingerprint) = 64 AND target_inspection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(transport_fingerprint) = 64 AND transport_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(result_fingerprint) = 64 AND result_fingerprint NOT GLOB '*[^0-9a-f]*')
	);`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_intent_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_application_intents
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_projection_plans plan
			JOIN sandbox_docker_runtime_input_projection_completions completion
				ON completion.projection_id = plan.id
			WHERE plan.id = NEW.projection_id AND plan.handoff_id = NEW.handoff_id
				AND plan.handoff_intent_id = NEW.handoff_intent_id AND plan.attempt_id = NEW.attempt_id
				AND plan.container_plan_id = NEW.container_plan_id AND plan.run_id = NEW.run_id
				AND plan.mission_id = NEW.mission_id AND plan.workspace_id = NEW.workspace_id
				AND plan.manifest_fingerprint = NEW.manifest_fingerprint
				AND plan.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND plan.input_artifact_digest = NEW.input_artifact_digest
				AND plan.authority_fingerprint = NEW.authority_fingerprint
				AND plan.spec_fingerprint = NEW.spec_fingerprint
				AND plan.container_plan_fingerprint = NEW.container_plan_fingerprint
				AND plan.handoff_fingerprint = NEW.handoff_fingerprint
				AND plan.projection_set_fingerprint = NEW.projection_set_fingerprint
				AND plan.projection_fingerprint = NEW.projection_fingerprint
				AND plan.projection_count = NEW.projection_count
				AND plan.read_only_mount_count = NEW.read_only_mount_count
				AND plan.input_artifact_count = NEW.input_artifact_count
				AND plan.total_entry_count = NEW.total_entry_count
				AND plan.total_content_bytes = NEW.total_content_bytes
				AND plan.total_projection_bytes = NEW.total_projection_bytes
				AND plan.requested_by = NEW.requested_by
				AND julianday(NEW.created_at) >= julianday(plan.created_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_failure_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_application_failures
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_application_leases lease
			WHERE lease.intent_id = NEW.intent_id AND lease.generation = NEW.generation
				AND lease.status = 'active' AND NEW.sequence = 1 + (
					SELECT COUNT(*) FROM sandbox_docker_runtime_input_application_failures prior
					WHERE prior.intent_id = NEW.intent_id)
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application failure mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_result_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_application_results
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_application_intents intent
			JOIN sandbox_docker_runtime_input_application_leases lease ON lease.intent_id = intent.id
			WHERE intent.id = NEW.intent_id AND intent.projection_id = NEW.projection_id
				AND intent.container_plan_id = NEW.container_plan_id AND intent.run_id = NEW.run_id
				AND intent.endpoint_class = NEW.endpoint_class
				AND intent.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND intent.projection_fingerprint = NEW.projection_fingerprint
				AND intent.projection_count = NEW.projection_count
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application result mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_lease_update
		BEFORE UPDATE ON sandbox_docker_runtime_input_application_leases
		WHEN NOT (
			OLD.intent_id = NEW.intent_id AND (
				(OLD.lease_id = NEW.lease_id AND OLD.owner_id = NEW.owner_id
					AND OLD.generation = NEW.generation AND OLD.status = 'active'
					AND NEW.status = 'released' AND OLD.acquired_at = NEW.acquired_at
					AND OLD.expires_at = NEW.expires_at AND NEW.released_at IS NOT NULL)
				OR
				(NEW.generation = OLD.generation + 1 AND NEW.status = 'active'
					AND NEW.released_at IS NULL AND NEW.lease_id <> OLD.lease_id
					AND julianday(NEW.expires_at) > julianday(NEW.acquired_at)
					AND (OLD.status = 'released'
						OR julianday(OLD.expires_at) <= julianday(NEW.acquired_at)))
			)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application lease transition is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_intent_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_application_intents BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application intent cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_intent_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_application_intents BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application intent cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_failure_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_application_failures BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application failure cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_failure_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_application_failures BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application failure cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_result_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_application_results BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application result cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_result_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_application_results BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application result cannot be deleted');
		END;`,
}
