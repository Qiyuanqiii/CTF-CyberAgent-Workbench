package store

var sandboxDockerRuntimeInputResourceStatements = []string{
	`CREATE TABLE sandbox_docker_runtime_input_resource_inspections (
		id TEXT PRIMARY KEY,
		application_intent_id TEXT NOT NULL,
		application_result_id TEXT NOT NULL,
		projection_id TEXT NOT NULL,
		container_plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		manifest_fingerprint TEXT NOT NULL,
		descriptor_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		application_result_fingerprint TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		status TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		target_state TEXT NOT NULL,
		projection_count INTEGER NOT NULL,
		owned_volume_count INTEGER NOT NULL,
		absent_volume_count INTEGER NOT NULL,
		foreign_volume_count INTEGER NOT NULL,
		foreign_resource_count INTEGER NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		complete INTEGER NOT NULL,
		cleanup_eligible INTEGER NOT NULL,
		owned_target_never_started INTEGER NOT NULL,
		all_owned_volumes_read_only INTEGER NOT NULL,
		all_owned_volumes_no_copy INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		request_semantic_fingerprint TEXT NOT NULL,
		inspection_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(application_intent_id) REFERENCES sandbox_docker_runtime_input_application_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_result_id) REFERENCES sandbox_docker_runtime_input_application_results(id) ON DELETE RESTRICT,
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_completions(projection_id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_runtime_input_resource_inspection.v1'),
		CHECK(endpoint_class = 'local_unix'),
		CHECK(status IN ('exact_owned_resources_present', 'exact_owned_resources_partial_or_absent', 'unsafe_resource_collision')),
		CHECK(trust_class IN ('exact_owned_readonly_never_started', 'partial_exact_owned_or_absent', 'foreign_or_changed_resource_detected')),
		CHECK(target_state IN ('exact_owned_present', 'absent', 'foreign_or_changed')),
		CHECK(projection_count BETWEEN 1 AND 33),
		CHECK(owned_volume_count >= 0 AND absent_volume_count >= 0 AND foreign_volume_count >= 0
			AND owned_volume_count + absent_volume_count + foreign_volume_count = projection_count),
		CHECK(foreign_resource_count = foreign_volume_count
			+ CASE WHEN target_state = 'foreign_or_changed' THEN 1 ELSE 0 END),
		CHECK(daemon_read_count = projection_count + 1 AND daemon_read_count <= 256),
		CHECK(complete = CASE WHEN target_state = 'exact_owned_present'
			AND owned_volume_count = projection_count AND foreign_resource_count = 0 THEN 1 ELSE 0 END),
		CHECK(cleanup_eligible = CASE WHEN foreign_resource_count = 0 THEN 1 ELSE 0 END),
		CHECK(owned_target_never_started = CASE WHEN target_state = 'exact_owned_present' THEN 1 ELSE 0 END),
		CHECK(all_owned_volumes_read_only = complete
			AND all_owned_volumes_no_copy = complete),
		CHECK((foreign_resource_count > 0 AND status = 'unsafe_resource_collision'
			AND trust_class = 'foreign_or_changed_resource_detected') OR
			(foreign_resource_count = 0 AND complete = 1 AND status = 'exact_owned_resources_present'
			AND trust_class = 'exact_owned_readonly_never_started') OR
			(foreign_resource_count = 0 AND complete = 0
			AND status = 'exact_owned_resources_partial_or_absent'
			AND trust_class = 'partial_exact_owned_or_absent')),
		CHECK(container_start_authorized = 0 AND process_execution_authorized = 0
			AND output_export_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(descriptor_fingerprint) = 64 AND descriptor_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(application_result_fingerprint) = 64 AND application_result_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_semantic_fingerprint) = 64 AND request_semantic_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(inspection_fingerprint) = 64 AND inspection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_runtime_input_resource_inspections_run_created
		ON sandbox_docker_runtime_input_resource_inspections(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_runtime_input_resource_cleanup_intents (
		id TEXT PRIMARY KEY,
		inspection_id TEXT NOT NULL UNIQUE,
		application_intent_id TEXT NOT NULL UNIQUE,
		application_result_id TEXT NOT NULL,
		projection_id TEXT NOT NULL,
		container_plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		manifest_fingerprint TEXT NOT NULL,
		descriptor_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		inspection_fingerprint TEXT NOT NULL,
		application_result_fingerprint TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		projection_count INTEGER NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		daemon_write_confirmed INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(inspection_id) REFERENCES sandbox_docker_runtime_input_resource_inspections(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_intent_id) REFERENCES sandbox_docker_runtime_input_application_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_result_id) REFERENCES sandbox_docker_runtime_input_application_results(id) ON DELETE RESTRICT,
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_completions(projection_id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_runtime_input_resource_cleanup_intent.v1'),
		CHECK(endpoint_class = 'local_unix' AND projection_count BETWEEN 1 AND 33),
		CHECK(operator_confirmed = 1 AND daemon_write_confirmed = 1),
		CHECK(container_start_authorized = 0 AND process_execution_authorized = 0
			AND output_export_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(descriptor_fingerprint) = 64 AND descriptor_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(inspection_fingerprint) = 64 AND inspection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(application_result_fingerprint) = 64 AND application_result_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_runtime_input_resource_cleanup_intents_run_created
		ON sandbox_docker_runtime_input_resource_cleanup_intents(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_runtime_input_resource_cleanup_leases (
		intent_id TEXT PRIMARY KEY,
		lease_id TEXT NOT NULL UNIQUE,
		owner_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		status TEXT NOT NULL,
		acquired_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		released_at TEXT,
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_intents(id) ON DELETE RESTRICT,
		CHECK(generation >= 1),
		CHECK(status IN ('active', 'released')),
		CHECK(julianday(expires_at) > julianday(acquired_at)),
		CHECK((status = 'active' AND released_at IS NULL) OR
			(status = 'released' AND released_at IS NOT NULL
			AND julianday(released_at) >= julianday(acquired_at))),
		CHECK(lease_id = trim(lease_id) AND length(lease_id) BETWEEN 1 AND 256 AND instr(lease_id, char(0)) = 0),
		CHECK(owner_id = trim(owner_id) AND length(owner_id) BETWEEN 1 AND 256 AND instr(owner_id, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_runtime_input_resource_cleanup_failures (
		intent_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		generation INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		code TEXT NOT NULL,
		failure_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(intent_id, sequence),
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_intents(id) ON DELETE RESTRICT,
		CHECK(sequence BETWEEN 1 AND 16 AND generation >= 1),
		CHECK(protocol_version = 'sandbox_docker_runtime_input_resource_cleanup_failure.v1'),
		CHECK(code IN ('resource_lifecycle_disabled', 'resource_lifecycle_unsupported',
			'connection_failed', 'invalid_response', 'unsafe_resource_collision',
			'resource_cleanup_failed', 'context_canceled', 'deadline_exceeded')),
		CHECK(length(failure_fingerprint) = 64 AND failure_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_runtime_input_resource_cleanup_results (
		id TEXT PRIMARY KEY,
		intent_id TEXT NOT NULL UNIQUE,
		inspection_id TEXT NOT NULL UNIQUE,
		application_intent_id TEXT NOT NULL UNIQUE,
		application_result_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		descriptor_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		application_result_fingerprint TEXT NOT NULL,
		projection_count INTEGER NOT NULL,
		total_resource_count INTEGER NOT NULL,
		initial_owned_resource_count INTEGER NOT NULL,
		initial_absent_resource_count INTEGER NOT NULL,
		delete_attempt_count INTEGER NOT NULL,
		final_absent_resource_count INTEGER NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		daemon_write_count INTEGER NOT NULL,
		target_absent INTEGER NOT NULL,
		all_volumes_absent INTEGER NOT NULL,
		foreign_resource_detected INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		result_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(inspection_id) REFERENCES sandbox_docker_runtime_input_resource_inspections(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_intent_id) REFERENCES sandbox_docker_runtime_input_application_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_result_id) REFERENCES sandbox_docker_runtime_input_application_results(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_runtime_input_resource_cleanup_result.v1'),
		CHECK(status = 'exact_owned_resources_absent'
			AND trust_class = 'exact_owned_cleanup_reverified'),
		CHECK(lease_generation >= 1 AND endpoint_class = 'local_unix'),
		CHECK(projection_count BETWEEN 1 AND 33 AND total_resource_count = projection_count + 1),
		CHECK(initial_owned_resource_count >= 0 AND initial_absent_resource_count >= 0
			AND initial_owned_resource_count + initial_absent_resource_count = total_resource_count),
		CHECK(delete_attempt_count = initial_owned_resource_count
			AND final_absent_resource_count = total_resource_count),
		CHECK(daemon_read_count = 2 * total_resource_count AND daemon_read_count <= 256),
		CHECK(daemon_write_count = delete_attempt_count AND daemon_write_count <= 34),
		CHECK(target_absent = 1 AND all_volumes_absent = 1 AND foreign_resource_detected = 0),
		CHECK(container_start_authorized = 0 AND process_execution_authorized = 0
			AND output_export_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(descriptor_fingerprint) = 64 AND descriptor_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(application_result_fingerprint) = 64 AND application_result_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(result_fingerprint) = 64 AND result_fingerprint NOT GLOB '*[^0-9a-f]*')
	);`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_inspection_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_resource_inspections
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_application_intents intent
			JOIN sandbox_docker_runtime_input_application_results result ON result.intent_id = intent.id
			WHERE intent.id = NEW.application_intent_id AND result.id = NEW.application_result_id
				AND intent.projection_id = NEW.projection_id
				AND intent.container_plan_id = NEW.container_plan_id AND intent.run_id = NEW.run_id
				AND intent.manifest_fingerprint = NEW.manifest_fingerprint
				AND intent.endpoint_class = NEW.endpoint_class
				AND intent.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND intent.projection_count = NEW.projection_count
				AND intent.requested_by = NEW.requested_by
				AND result.request_fingerprint = NEW.request_fingerprint
				AND result.result_fingerprint = NEW.application_result_fingerprint
				AND result.target_container_present = 1 AND result.container_started = 0
				AND result.process_executed = 0 AND result.output_exported = 0
				AND julianday(NEW.created_at) >= julianday(result.created_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource inspection authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_intent_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_resource_cleanup_intents
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_resource_inspections inspection
			WHERE inspection.id = NEW.inspection_id
				AND inspection.application_intent_id = NEW.application_intent_id
				AND inspection.application_result_id = NEW.application_result_id
				AND inspection.projection_id = NEW.projection_id
				AND inspection.container_plan_id = NEW.container_plan_id
				AND inspection.run_id = NEW.run_id
				AND inspection.manifest_fingerprint = NEW.manifest_fingerprint
				AND inspection.descriptor_fingerprint = NEW.descriptor_fingerprint
				AND inspection.request_fingerprint = NEW.request_fingerprint
				AND inspection.inspection_fingerprint = NEW.inspection_fingerprint
				AND inspection.application_result_fingerprint = NEW.application_result_fingerprint
				AND inspection.endpoint_class = NEW.endpoint_class
				AND inspection.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND inspection.projection_count = NEW.projection_count
				AND inspection.cleanup_eligible = 1 AND inspection.foreign_resource_count = 0
				AND inspection.requested_by = NEW.requested_by
				AND julianday(NEW.created_at) >= julianday(inspection.created_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_failure_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_resource_cleanup_failures
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_resource_cleanup_leases lease
			WHERE lease.intent_id = NEW.intent_id AND lease.generation = NEW.generation
				AND lease.status = 'active' AND NEW.sequence = 1 + (
					SELECT COUNT(*) FROM sandbox_docker_runtime_input_resource_cleanup_failures prior
					WHERE prior.intent_id = NEW.intent_id)
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup failure mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_result_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_resource_cleanup_results
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_resource_cleanup_intents intent
			JOIN sandbox_docker_runtime_input_resource_cleanup_leases lease ON lease.intent_id = intent.id
			WHERE intent.id = NEW.intent_id AND intent.inspection_id = NEW.inspection_id
				AND intent.application_intent_id = NEW.application_intent_id
				AND intent.application_result_id = NEW.application_result_id
				AND intent.run_id = NEW.run_id AND intent.endpoint_class = NEW.endpoint_class
				AND intent.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND intent.descriptor_fingerprint = NEW.descriptor_fingerprint
				AND intent.request_fingerprint = NEW.request_fingerprint
				AND intent.application_result_fingerprint = NEW.application_result_fingerprint
				AND intent.projection_count = NEW.projection_count
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.created_at) < julianday(lease.expires_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup result mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_lease_update
		BEFORE UPDATE ON sandbox_docker_runtime_input_resource_cleanup_leases
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
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup lease transition is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_lease_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_resource_cleanup_leases BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup lease cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_application_lease_delete_immutable_v62
		BEFORE DELETE ON sandbox_docker_runtime_input_application_leases BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input application lease cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_inspection_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_resource_inspections BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource inspection cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_inspection_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_resource_inspections BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource inspection cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_intent_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_resource_cleanup_intents BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup intent cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_intent_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_resource_cleanup_intents BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup intent cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_failure_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_resource_cleanup_failures BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup failure cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_failure_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_resource_cleanup_failures BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup failure cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_result_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_resource_cleanup_results BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup result cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_resource_cleanup_result_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_resource_cleanup_results BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input resource cleanup result cannot be deleted');
		END;`,
}
