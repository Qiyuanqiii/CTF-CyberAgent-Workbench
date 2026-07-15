package store

var sandboxDockerContainerRehearsalStatements = []string{
	`CREATE TABLE sandbox_docker_container_rehearsals (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL UNIQUE,
		observation_id TEXT NOT NULL,
		evidence_id TEXT NOT NULL,
		output_simulation_id TEXT NOT NULL,
		preflight_id TEXT NOT NULL,
		execution_id TEXT NOT NULL,
		candidate_id TEXT NOT NULL,
		preparation_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		source TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		status TEXT NOT NULL,
		manifest_fingerprint TEXT NOT NULL,
		authorization_fingerprint TEXT NOT NULL,
		policy_fingerprint TEXT NOT NULL,
		mount_binding_fingerprint TEXT NOT NULL,
		input_artifact_digest TEXT NOT NULL,
		threat_model_fingerprint TEXT NOT NULL,
		output_plan_fingerprint TEXT NOT NULL,
		observation_fingerprint TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		spec_fingerprint TEXT NOT NULL,
		plan_fingerprint TEXT NOT NULL,
		image_digest TEXT NOT NULL,
		network_mode TEXT NOT NULL,
		environment_count INTEGER NOT NULL,
		secret_reference_count INTEGER NOT NULL,
		request_fingerprint TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		container_id_fingerprint TEXT NOT NULL,
		inspection_fingerprint TEXT NOT NULL,
		transport_fingerprint TEXT NOT NULL,
		rehearsal_fingerprint TEXT NOT NULL,
		result_protocol_version TEXT NOT NULL,
		result_status TEXT NOT NULL,
		step_count INTEGER NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		daemon_write_count INTEGER NOT NULL,
		reconciled_container_count INTEGER NOT NULL,
		configuration_matched INTEGER NOT NULL,
		container_created INTEGER NOT NULL,
		container_inspected INTEGER NOT NULL,
		container_removed INTEGER NOT NULL,
		container_never_started INTEGER NOT NULL,
		process_never_executed INTEGER NOT NULL,
		image_never_pulled INTEGER NOT NULL,
		output_never_exported INTEGER NOT NULL,
		cleanup_confirmed INTEGER NOT NULL,
		daemon_reachable INTEGER NOT NULL,
		daemon_write_submitted INTEGER NOT NULL,
		production_execution_submitted INTEGER NOT NULL,
		production_verified INTEGER NOT NULL,
		backend_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(observation_id) REFERENCES sandbox_docker_observations(id) ON DELETE RESTRICT,
		FOREIGN KEY(evidence_id) REFERENCES sandbox_backend_evidence(id) ON DELETE RESTRICT,
		FOREIGN KEY(output_simulation_id) REFERENCES sandbox_output_simulations(id) ON DELETE RESTRICT,
		FOREIGN KEY(preflight_id) REFERENCES sandbox_disabled_preflights(id) ON DELETE RESTRICT,
		FOREIGN KEY(execution_id) REFERENCES sandbox_disabled_executions(id) ON DELETE RESTRICT,
		FOREIGN KEY(candidate_id) REFERENCES sandbox_execution_candidates(id) ON DELETE RESTRICT,
		FOREIGN KEY(preparation_id) REFERENCES sandbox_manifest_preparations(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_container_rehearsal.v1'),
		CHECK(source = 'local_unix_create_inspect_remove'),
		CHECK(trust_class = 'production_daemon_rehearsal_unverified'),
		CHECK(status = 'container_config_rehearsed_removed'),
		CHECK(result_protocol_version = 'sandbox_docker_write_transport.v1'),
		CHECK(result_status = 'create_inspect_remove_complete'),
		CHECK(endpoint_class = 'local_unix'),
		CHECK(network_mode = 'disabled' AND environment_count = 0 AND secret_reference_count = 0),
		CHECK(step_count = 5 AND daemon_read_count = 3),
		CHECK(reconciled_container_count BETWEEN 0 AND 1
			AND daemon_write_count = 2 + reconciled_container_count),
		CHECK(configuration_matched = 1 AND container_created = 1
			AND container_inspected = 1 AND container_removed = 1),
		CHECK(container_never_started = 1 AND process_never_executed = 1
			AND image_never_pulled = 1 AND output_never_exported = 1),
		CHECK(cleanup_confirmed = 1 AND daemon_reachable = 1 AND daemon_write_submitted = 1),
		CHECK(production_execution_submitted = 0 AND production_verified = 0
			AND backend_enabled = 0 AND execution_authorized = 0
			AND artifact_commit_authorized = 0),
		CHECK(length(image_digest) = 71 AND substr(image_digest, 1, 7) = 'sha256:'
			AND substr(image_digest, 8) = lower(substr(image_digest, 8))
			AND substr(image_digest, 8) NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint = lower(manifest_fingerprint)
			AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authorization_fingerprint) = 64 AND authorization_fingerprint = lower(authorization_fingerprint)
			AND authorization_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(policy_fingerprint) = 64 AND policy_fingerprint = lower(policy_fingerprint)
			AND policy_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_binding_fingerprint) = 64 AND mount_binding_fingerprint = lower(mount_binding_fingerprint)
			AND mount_binding_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest = lower(input_artifact_digest)
			AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(threat_model_fingerprint) = 64 AND threat_model_fingerprint = lower(threat_model_fingerprint)
			AND threat_model_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(output_plan_fingerprint) = 64 AND output_plan_fingerprint = lower(output_plan_fingerprint)
			AND output_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(observation_fingerprint) = 64 AND observation_fingerprint = lower(observation_fingerprint)
			AND observation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint = lower(authority_fingerprint)
			AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(spec_fingerprint) = 64 AND spec_fingerprint = lower(spec_fingerprint)
			AND spec_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_fingerprint) = 64 AND plan_fingerprint = lower(plan_fingerprint)
			AND plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint = lower(endpoint_fingerprint)
			AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_id_fingerprint) = 64 AND container_id_fingerprint = lower(container_id_fingerprint)
			AND container_id_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(inspection_fingerprint) = 64 AND inspection_fingerprint = lower(inspection_fingerprint)
			AND inspection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(transport_fingerprint) = 64 AND transport_fingerprint = lower(transport_fingerprint)
			AND transport_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(rehearsal_fingerprint) = 64 AND rehearsal_fingerprint = lower(rehearsal_fingerprint)
			AND rehearsal_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(observation_id = trim(observation_id) AND length(observation_id) BETWEEN 1 AND 256
			AND instr(observation_id, char(0)) = 0),
		CHECK(evidence_id = trim(evidence_id) AND length(evidence_id) BETWEEN 1 AND 256
			AND instr(evidence_id, char(0)) = 0),
		CHECK(output_simulation_id = trim(output_simulation_id)
			AND length(output_simulation_id) BETWEEN 1 AND 256 AND instr(output_simulation_id, char(0)) = 0),
		CHECK(preflight_id = trim(preflight_id) AND length(preflight_id) BETWEEN 1 AND 256
			AND instr(preflight_id, char(0)) = 0),
		CHECK(execution_id = trim(execution_id) AND length(execution_id) BETWEEN 1 AND 256
			AND instr(execution_id, char(0)) = 0),
		CHECK(candidate_id = trim(candidate_id) AND length(candidate_id) BETWEEN 1 AND 256
			AND instr(candidate_id, char(0)) = 0),
		CHECK(preparation_id = trim(preparation_id) AND length(preparation_id) BETWEEN 1 AND 256
			AND instr(preparation_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256
			AND instr(mission_id, char(0)) = 0),
		CHECK(workspace_id = trim(workspace_id) AND length(workspace_id) BETWEEN 1 AND 256
			AND instr(workspace_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_container_rehearsals_run_created
		ON sandbox_docker_container_rehearsals(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_container_rehearsal_steps (
		rehearsal_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		name TEXT NOT NULL,
		state TEXT NOT NULL,
		daemon_reads INTEGER NOT NULL,
		daemon_writes INTEGER NOT NULL,
		production_applied INTEGER NOT NULL,
		step_digest TEXT NOT NULL,
		PRIMARY KEY(rehearsal_id, ordinal),
		UNIQUE(rehearsal_id, name),
		FOREIGN KEY(rehearsal_id) REFERENCES sandbox_docker_container_rehearsals(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 5),
		CHECK((ordinal = 1 AND name = 'verify_image_profile' AND daemon_reads = 1
				AND daemon_writes = 0)
			OR (ordinal = 2 AND name = 'reconcile_container_name' AND daemon_reads = 1
				AND daemon_writes BETWEEN 0 AND 1)
			OR (ordinal = 3 AND name = 'create_container' AND daemon_reads = 0 AND daemon_writes = 1)
			OR (ordinal = 4 AND name = 'inspect_container' AND daemon_reads = 1 AND daemon_writes = 0)
			OR (ordinal = 5 AND name = 'remove_container' AND daemon_reads = 0 AND daemon_writes = 1)),
		CHECK(state = 'completed' AND production_applied = daemon_writes),
		CHECK(length(step_digest) = 64 AND step_digest = lower(step_digest)
			AND step_digest NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_rehearsal_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		rehearsal_id TEXT NOT NULL UNIQUE,
		plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(rehearsal_id) REFERENCES sandbox_docker_container_rehearsals(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(rehearsal_id = trim(rehearsal_id) AND length(rehearsal_id) BETWEEN 1 AND 256
			AND instr(rehearsal_id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_insert
		BEFORE INSERT ON sandbox_docker_container_rehearsals
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_container_plans plan
			JOIN sandbox_docker_observations observation ON observation.id = plan.observation_id
			JOIN sandbox_disabled_executions execution ON execution.id = plan.execution_id
			JOIN sandbox_execution_leases sandbox_lease ON sandbox_lease.execution_id = execution.id
			JOIN runs run ON run.id = plan.run_id
			WHERE plan.id = NEW.plan_id AND plan.observation_id = NEW.observation_id
				AND plan.evidence_id = NEW.evidence_id
				AND plan.output_simulation_id = NEW.output_simulation_id
				AND plan.preflight_id = NEW.preflight_id AND plan.execution_id = NEW.execution_id
				AND plan.candidate_id = NEW.candidate_id AND plan.preparation_id = NEW.preparation_id
				AND plan.run_id = NEW.run_id AND plan.mission_id = NEW.mission_id
				AND plan.workspace_id = NEW.workspace_id
				AND plan.manifest_fingerprint = NEW.manifest_fingerprint
				AND plan.authorization_fingerprint = NEW.authorization_fingerprint
				AND plan.policy_fingerprint = NEW.policy_fingerprint
				AND plan.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND plan.input_artifact_digest = NEW.input_artifact_digest
				AND plan.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND plan.output_plan_fingerprint = NEW.output_plan_fingerprint
				AND plan.observation_fingerprint = NEW.observation_fingerprint
				AND plan.authority_fingerprint = NEW.authority_fingerprint
				AND plan.spec_fingerprint = NEW.spec_fingerprint
				AND plan.plan_fingerprint = NEW.plan_fingerprint
				AND plan.image_digest = NEW.image_digest AND plan.network_mode = 'disabled'
				AND plan.network_target_count = 0 AND plan.environment_count = 0
				AND plan.secret_reference_count = 0 AND plan.simulation_only = 1
				AND plan.production_submitted = 0 AND plan.production_verified = 0
				AND plan.backend_available = 0 AND plan.backend_enabled = 0
				AND plan.execution_authorized = 0 AND plan.artifact_commit_authorized = 0
				AND plan.requested_by = NEW.requested_by
				AND observation.status = 'observation_complete'
				AND observation.observation_complete = 1 AND observation.production_observed = 1
				AND observation.production_verified = 0 AND observation.backend_available = 0
				AND observation.backend_enabled = 0 AND observation.execution_authorized = 0
				AND observation.artifact_commit_authorized = 0
				AND execution.backend_enabled = 0 AND execution.execution_authorized = 0
				AND execution.backend_started = 0
				AND sandbox_lease.status = 'released'
				AND run.status IN ('created', 'preparing', 'running', 'waiting_approval', 'paused')
				AND NOT EXISTS (SELECT 1 FROM sandbox_execution_cancellations cancellation
					WHERE cancellation.execution_id = execution.id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_cleanup_results cleanup
					WHERE cleanup.execution_id = execution.id)
				AND NOT EXISTS (SELECT 1 FROM run_execution_leases run_lease
					WHERE run_lease.run_id = NEW.run_id AND run_lease.status = 'active'
						AND julianday(run_lease.expires_at) > julianday('now'))
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_rehearsals existing
					WHERE existing.plan_id = NEW.plan_id)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_step_insert
		BEFORE INSERT ON sandbox_docker_container_rehearsal_steps
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_rehearsals rehearsal
			WHERE rehearsal.id = NEW.rehearsal_id) BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal step mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_operation_insert
		BEFORE INSERT ON sandbox_docker_container_rehearsal_operations
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_rehearsals rehearsal
			WHERE rehearsal.id = NEW.rehearsal_id AND rehearsal.plan_id = NEW.plan_id
				AND rehearsal.run_id = NEW.run_id AND rehearsal.requested_by = NEW.requested_by
				AND rehearsal.created_at = NEW.created_at
				AND (SELECT COUNT(*) FROM sandbox_docker_container_rehearsal_steps step
					WHERE step.rehearsal_id = rehearsal.id) = rehearsal.step_count
				AND (SELECT COALESCE(SUM(step.daemon_reads), 0)
					FROM sandbox_docker_container_rehearsal_steps step
					WHERE step.rehearsal_id = rehearsal.id) = rehearsal.daemon_read_count
				AND (SELECT COALESCE(SUM(step.daemon_writes), 0)
					FROM sandbox_docker_container_rehearsal_steps step
					WHERE step.rehearsal_id = rehearsal.id) = rehearsal.daemon_write_count
		) BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal operation mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_rehearsals BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_rehearsals BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_step_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_rehearsal_steps BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal step cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_step_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_rehearsal_steps BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal step cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_operation_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_rehearsal_operations BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_rehearsal_operation_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_rehearsal_operations BEGIN
			SELECT RAISE(ABORT, 'Docker container rehearsal operation cannot be deleted');
		END;`,
}
