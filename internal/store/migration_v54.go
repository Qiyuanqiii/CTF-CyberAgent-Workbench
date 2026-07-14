package store

var sandboxDockerContainerPlanStatements = []string{
	`CREATE TABLE sandbox_docker_container_plans (
		id TEXT PRIMARY KEY,
		observation_id TEXT NOT NULL UNIQUE,
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
		image_digest TEXT NOT NULL,
		os_type TEXT NOT NULL,
		architecture TEXT NOT NULL,
		container_user TEXT NOT NULL,
		spec_fingerprint TEXT NOT NULL,
		command_fingerprint TEXT NOT NULL,
		mount_plan_fingerprint TEXT NOT NULL,
		network_plan_fingerprint TEXT NOT NULL,
		secret_plan_fingerprint TEXT NOT NULL,
		container_config_fingerprint TEXT NOT NULL,
		resource_plan_fingerprint TEXT NOT NULL,
		termination_plan_fingerprint TEXT NOT NULL,
		label_plan_fingerprint TEXT NOT NULL,
		orphan_plan_fingerprint TEXT NOT NULL,
		container_name_fingerprint TEXT NOT NULL,
		plan_fingerprint TEXT NOT NULL,
		read_only_rootfs INTEGER NOT NULL,
		no_new_privileges INTEGER NOT NULL,
		drop_all_capabilities INTEGER NOT NULL,
		init_enabled INTEGER NOT NULL,
		mount_count INTEGER NOT NULL,
		read_only_mount_count INTEGER NOT NULL,
		writable_mount_count INTEGER NOT NULL,
		dedicated_output_mount_count INTEGER NOT NULL,
		private_propagation_mount_count INTEGER NOT NULL,
		environment_count INTEGER NOT NULL,
		secret_reference_count INTEGER NOT NULL,
		input_artifact_count INTEGER NOT NULL,
		output_count INTEGER NOT NULL,
		network_mode TEXT NOT NULL,
		network_target_count INTEGER NOT NULL,
		network_default_deny INTEGER NOT NULL,
		exact_network_allowlist INTEGER NOT NULL,
		network_guard_required INTEGER NOT NULL,
		nano_cpus INTEGER NOT NULL,
		memory_bytes INTEGER NOT NULL,
		pids INTEGER NOT NULL,
		max_output_bytes INTEGER NOT NULL,
		timeout_seconds INTEGER NOT NULL,
		grace_period_millis INTEGER NOT NULL,
		secrets_ephemeral INTEGER NOT NULL,
		secrets_metadata_excluded INTEGER NOT NULL,
		label_count INTEGER NOT NULL,
		reconcile_before_create INTEGER NOT NULL,
		remove_on_rollback INTEGER NOT NULL,
		export_after_stop INTEGER NOT NULL,
		remove_after_export INTEGER NOT NULL,
		control_count INTEGER NOT NULL,
		transaction_protocol_version TEXT NOT NULL,
		transaction_source TEXT NOT NULL,
		transaction_status TEXT NOT NULL,
		transaction_fingerprint TEXT NOT NULL,
		transaction_step_count INTEGER NOT NULL,
		transaction_staged_step_count INTEGER NOT NULL,
		transaction_committed_step_count INTEGER NOT NULL,
		transaction_rollback_step_count INTEGER NOT NULL,
		transaction_daemon_write_count INTEGER NOT NULL,
		transaction_backend_touched INTEGER NOT NULL,
		simulation_only INTEGER NOT NULL,
		production_submitted INTEGER NOT NULL,
		production_verified INTEGER NOT NULL,
		backend_available INTEGER NOT NULL,
		backend_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
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
		CHECK(protocol_version = 'sandbox_docker_container_plan.v1'),
		CHECK(source = 'go_deterministic_compiler' AND trust_class = 'simulation_only'),
		CHECK(status = 'compiled_fake_transaction_committed'),
		CHECK(os_type = 'linux' AND length(architecture) BETWEEN 1 AND 64),
		CHECK(container_user = '65532:65532'),
		CHECK(length(image_digest) = 71 AND substr(image_digest, 1, 7) = 'sha256:'
			AND substr(image_digest, 8) = lower(substr(image_digest, 8))
			AND substr(image_digest, 8) NOT GLOB '*[^0-9a-f]*'),
		CHECK(read_only_rootfs = 1 AND no_new_privileges = 1
			AND drop_all_capabilities = 1 AND init_enabled = 1),
		CHECK(mount_count BETWEEN 1 AND 32 AND read_only_mount_count = mount_count - 1
			AND writable_mount_count = 1 AND dedicated_output_mount_count = 1
			AND private_propagation_mount_count = mount_count),
		CHECK(environment_count BETWEEN 0 AND 64
			AND secret_reference_count BETWEEN 0 AND environment_count),
		CHECK(input_artifact_count BETWEEN 0 AND 16 AND output_count BETWEEN 1 AND 18),
		CHECK(network_mode IN ('disabled', 'allowlist') AND network_default_deny = 1
			AND ((network_mode = 'disabled' AND network_target_count = 0
				AND exact_network_allowlist = 0 AND network_guard_required = 0)
			OR (network_mode = 'allowlist' AND network_target_count BETWEEN 1 AND 32
				AND exact_network_allowlist = 1 AND network_guard_required = 1))),
		CHECK(nano_cpus BETWEEN 1000000 AND 8000000000),
		CHECK(memory_bytes BETWEEN 16777216 AND 8589934592),
		CHECK(pids BETWEEN 1 AND 512 AND max_output_bytes BETWEEN 1 AND 16777216),
		CHECK(timeout_seconds BETWEEN 1 AND 3600 AND grace_period_millis BETWEEN 0 AND 30000),
		CHECK(secrets_ephemeral = 1 AND secrets_metadata_excluded = 1),
		CHECK(label_count = 6 AND reconcile_before_create = 1 AND remove_on_rollback = 1
			AND export_after_stop = 1 AND remove_after_export = 1),
		CHECK(control_count = 16),
		CHECK(transaction_protocol_version = 'sandbox_docker_write_transaction.v1'
			AND transaction_source = 'in_memory_fake' AND transaction_status = 'fake_committed'),
		CHECK(transaction_step_count = 7 AND transaction_staged_step_count = 7
			AND transaction_committed_step_count = 7 AND transaction_rollback_step_count = 0
			AND transaction_daemon_write_count = 0 AND transaction_backend_touched = 0),
		CHECK(simulation_only = 1 AND production_submitted = 0 AND production_verified = 0
			AND backend_available = 0 AND backend_enabled = 0 AND execution_authorized = 0
			AND artifact_commit_authorized = 0),
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
		CHECK(length(command_fingerprint) = 64 AND command_fingerprint = lower(command_fingerprint)
			AND command_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_plan_fingerprint) = 64 AND mount_plan_fingerprint = lower(mount_plan_fingerprint)
			AND mount_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(network_plan_fingerprint) = 64 AND network_plan_fingerprint = lower(network_plan_fingerprint)
			AND network_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(secret_plan_fingerprint) = 64 AND secret_plan_fingerprint = lower(secret_plan_fingerprint)
			AND secret_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_config_fingerprint) = 64 AND container_config_fingerprint = lower(container_config_fingerprint)
			AND container_config_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(resource_plan_fingerprint) = 64 AND resource_plan_fingerprint = lower(resource_plan_fingerprint)
			AND resource_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(termination_plan_fingerprint) = 64 AND termination_plan_fingerprint = lower(termination_plan_fingerprint)
			AND termination_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(label_plan_fingerprint) = 64 AND label_plan_fingerprint = lower(label_plan_fingerprint)
			AND label_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(orphan_plan_fingerprint) = 64 AND orphan_plan_fingerprint = lower(orphan_plan_fingerprint)
			AND orphan_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_name_fingerprint) = 64 AND container_name_fingerprint = lower(container_name_fingerprint)
			AND container_name_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_fingerprint) = 64 AND plan_fingerprint = lower(plan_fingerprint)
			AND plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(transaction_fingerprint) = 64 AND transaction_fingerprint = lower(transaction_fingerprint)
			AND transaction_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
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
	`CREATE INDEX idx_sandbox_docker_container_plans_run_created
		ON sandbox_docker_container_plans(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_container_plan_controls (
		plan_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		name TEXT NOT NULL,
		state TEXT NOT NULL,
		control_digest TEXT NOT NULL,
		planned INTEGER NOT NULL,
		applied INTEGER NOT NULL,
		verified INTEGER NOT NULL,
		PRIMARY KEY(plan_id, ordinal),
		UNIQUE(plan_id, name),
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 16),
		CHECK((ordinal = 1 AND name = 'host_path_isolation')
			OR (ordinal = 2 AND name = 'mount_propagation_private')
			OR (ordinal = 3 AND name = 'read_only_rootfs')
			OR (ordinal = 4 AND name = 'read_only_inputs')
			OR (ordinal = 5 AND name = 'dedicated_writable_output')
			OR (ordinal = 6 AND name = 'network_default_deny')
			OR (ordinal = 7 AND name = 'exact_network_allowlist')
			OR (ordinal = 8 AND name = 'ephemeral_secret_materialization')
			OR (ordinal = 9 AND name = 'non_root_container_identity')
			OR (ordinal = 10 AND name = 'cpu_memory_pid_limits')
			OR (ordinal = 11 AND name = 'wall_clock_timeout')
			OR (ordinal = 12 AND name = 'graceful_then_forced_kill')
			OR (ordinal = 13 AND name = 'orphan_reconciliation')
			OR (ordinal = 14 AND name = 'output_regular_file_only')
			OR (ordinal = 15 AND name = 'output_symlink_special_rejection')
			OR (ordinal = 16 AND name = 'atomic_output_artifact_commit')),
		CHECK(state = 'compiled_not_applied' AND planned = 1 AND applied = 0 AND verified = 0),
		CHECK(length(control_digest) = 64 AND control_digest = lower(control_digest)
			AND control_digest NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_plan_steps (
		plan_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		name TEXT NOT NULL,
		state TEXT NOT NULL,
		step_digest TEXT NOT NULL,
		simulated INTEGER NOT NULL,
		production_applied INTEGER NOT NULL,
		PRIMARY KEY(plan_id, ordinal),
		UNIQUE(plan_id, name),
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 7),
		CHECK((ordinal = 1 AND name = 'reconcile_orphans')
			OR (ordinal = 2 AND name = 'create_container')
			OR (ordinal = 3 AND name = 'start_container')
			OR (ordinal = 4 AND name = 'wait_container')
			OR (ordinal = 5 AND name = 'stop_container')
			OR (ordinal = 6 AND name = 'export_outputs')
			OR (ordinal = 7 AND name = 'remove_container')),
		CHECK(state = 'fake_committed' AND simulated = 1 AND production_applied = 0),
		CHECK(length(step_digest) = 64 AND step_digest = lower(step_digest)
			AND step_digest NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_plan_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		plan_id TEXT NOT NULL UNIQUE,
		observation_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(observation_id) REFERENCES sandbox_docker_observations(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(observation_id = trim(observation_id) AND length(observation_id) BETWEEN 1 AND 256
			AND instr(observation_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_insert
		BEFORE INSERT ON sandbox_docker_container_plans
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_observations observation
			JOIN sandbox_backend_evidence evidence ON evidence.id = observation.evidence_id
			JOIN sandbox_output_simulations simulation ON simulation.id = observation.output_simulation_id
			JOIN sandbox_disabled_preflights preflight ON preflight.id = observation.preflight_id
			JOIN sandbox_disabled_executions execution ON execution.id = observation.execution_id
			JOIN sandbox_execution_candidates candidate ON candidate.id = observation.candidate_id
			JOIN sandbox_manifest_preparations preparation ON preparation.id = observation.preparation_id
			JOIN sandbox_execution_leases sandbox_lease ON sandbox_lease.execution_id = execution.id
			JOIN runs run ON run.id = observation.run_id
			JOIN missions mission ON mission.id = observation.mission_id
			WHERE observation.id = NEW.observation_id AND observation.evidence_id = NEW.evidence_id
				AND observation.output_simulation_id = NEW.output_simulation_id
				AND observation.preflight_id = NEW.preflight_id
				AND observation.execution_id = NEW.execution_id
				AND observation.candidate_id = NEW.candidate_id
				AND observation.preparation_id = NEW.preparation_id
				AND observation.run_id = NEW.run_id AND observation.mission_id = NEW.mission_id
				AND observation.workspace_id = NEW.workspace_id
				AND observation.manifest_fingerprint = NEW.manifest_fingerprint
				AND observation.authorization_fingerprint = NEW.authorization_fingerprint
				AND observation.policy_fingerprint = NEW.policy_fingerprint
				AND observation.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND observation.input_artifact_digest = NEW.input_artifact_digest
				AND observation.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND observation.output_plan_fingerprint = NEW.output_plan_fingerprint
				AND observation.observation_fingerprint = NEW.observation_fingerprint
				AND observation.image_digest = NEW.image_digest
				AND observation.os_type = NEW.os_type AND observation.image_os_type = NEW.os_type
				AND observation.architecture = NEW.architecture
				AND observation.image_architecture = NEW.architecture
				AND observation.status = 'observation_complete'
				AND observation.observation_complete = 1 AND observation.production_observed = 1
				AND observation.production_verified = 0 AND observation.backend_available = 0
				AND observation.backend_enabled = 0 AND observation.execution_authorized = 0
				AND observation.artifact_commit_authorized = 0
				AND observation.pids_limit_supported = 1
				AND observation.requested_by = NEW.requested_by
				AND evidence.id = NEW.evidence_id AND simulation.evidence_id = evidence.id
				AND evidence.preflight_id = NEW.preflight_id AND evidence.execution_id = NEW.execution_id
				AND evidence.candidate_id = NEW.candidate_id
				AND evidence.preparation_id = NEW.preparation_id
				AND evidence.run_id = NEW.run_id AND evidence.mission_id = NEW.mission_id
				AND evidence.workspace_id = NEW.workspace_id
				AND evidence.manifest_fingerprint = NEW.manifest_fingerprint
				AND evidence.authorization_fingerprint = NEW.authorization_fingerprint
				AND evidence.policy_fingerprint = NEW.policy_fingerprint
				AND evidence.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND evidence.input_artifact_digest = NEW.input_artifact_digest
				AND evidence.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND evidence.output_plan_fingerprint = NEW.output_plan_fingerprint
				AND simulation.output_plan_fingerprint = NEW.output_plan_fingerprint
				AND evidence.requested_by = NEW.requested_by
				AND simulation.requested_by = NEW.requested_by
				AND evidence.source = 'in_memory_fake' AND evidence.trust_class = 'simulation_only'
				AND evidence.status = 'simulation_complete' AND evidence.production_verified = 0
				AND evidence.backend_available = 0 AND evidence.backend_enabled = 0
				AND evidence.execution_authorized = 0 AND evidence.artifact_commit_authorized = 0
				AND simulation.status = 'simulation_committed' AND simulation.simulation_only = 1
				AND simulation.production_artifact_count = 0
				AND simulation.backend_enabled = 0 AND simulation.execution_authorized = 0
				AND simulation.artifact_commit_authorized = 0
				AND preflight.backend = 'docker' AND preparation.backend = 'docker'
				AND preflight.manifest_fingerprint = NEW.manifest_fingerprint
				AND preflight.authorization_fingerprint = NEW.authorization_fingerprint
				AND preflight.policy_fingerprint = NEW.policy_fingerprint
				AND preflight.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND preflight.input_artifact_digest = NEW.input_artifact_digest
				AND preflight.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND preflight.output_plan_fingerprint = NEW.output_plan_fingerprint
				AND preflight.requested_by = NEW.requested_by
				AND preflight.backend_available = 0 AND preflight.backend_enabled = 0
				AND preflight.execution_authorized = 0 AND preflight.artifact_commit_authorized = 0
				AND preparation.manifest_fingerprint = NEW.manifest_fingerprint
				AND preparation.run_id = NEW.run_id AND preparation.mission_id = NEW.mission_id
				AND preparation.workspace_id = NEW.workspace_id
				AND preparation.requested_by = NEW.requested_by
				AND preparation.mount_count = NEW.mount_count
				AND preparation.writable_mount_count = 1
				AND NEW.read_only_mount_count = preparation.mount_count - 1
				AND NEW.writable_mount_count = preparation.writable_mount_count
				AND preparation.environment_count = NEW.environment_count
				AND preparation.secret_reference_count = NEW.secret_reference_count
				AND preparation.input_artifact_count = NEW.input_artifact_count
				AND preparation.output_count = NEW.output_count
				AND preparation.network_mode = NEW.network_mode
				AND preparation.allowed_target_count = NEW.network_target_count
				AND NEW.nano_cpus = preparation.cpu_quota_millis * 1000000
				AND preparation.memory_bytes = NEW.memory_bytes AND preparation.pids = NEW.pids
				AND preparation.max_output_bytes = NEW.max_output_bytes
				AND preparation.timeout_seconds = NEW.timeout_seconds
				AND preparation.grace_period_millis = NEW.grace_period_millis
				AND execution.backend_enabled = 0 AND execution.execution_authorized = 0
				AND execution.backend_started = 0 AND candidate.backend_enabled = 0
				AND candidate.execution_authorized = 0 AND sandbox_lease.status = 'released'
				AND run.mission_id = NEW.mission_id AND mission.workspace_id = NEW.workspace_id
				AND run.status IN ('created', 'preparing', 'running', 'waiting_approval', 'paused')
				AND NOT EXISTS (SELECT 1 FROM sandbox_execution_cancellations cancellation
					WHERE cancellation.execution_id = execution.id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_cleanup_results cleanup
					WHERE cleanup.execution_id = execution.id)
				AND NOT EXISTS (SELECT 1 FROM run_execution_leases run_lease
					WHERE run_lease.run_id = NEW.run_id AND run_lease.status = 'active'
						AND julianday(run_lease.expires_at) > julianday('now'))
				AND candidate.tokens_used =
					COALESCE((SELECT SUM(node.tokens_used) FROM agent_nodes node
						WHERE node.run_id = NEW.run_id), 0) +
					COALESCE((SELECT SUM(CASE WHEN call.usage_recorded = 1 THEN call.total_tokens
						ELSE call.reserved_total_tokens END) FROM readonly_fanout_model_calls call
						WHERE call.run_id = NEW.run_id), 0)
				AND candidate.execution_millis_used =
					COALESCE((SELECT checkpoint.execution_millis FROM run_supervisor_checkpoints checkpoint
						WHERE checkpoint.run_id = NEW.run_id), 0) +
					COALESCE((SELECT SUM(call.elapsed_millis) FROM specialist_model_calls call
						WHERE call.run_id = NEW.run_id), 0) +
					COALESCE((SELECT SUM(CASE WHEN call.elapsed_recorded = 1 THEN call.elapsed_millis
						ELSE call.reserved_millis END) FROM readonly_fanout_model_calls call
						WHERE call.run_id = NEW.run_id), 0)
				AND candidate.tool_calls_used = COALESCE((SELECT usage.consumed FROM run_tool_usage usage
					WHERE usage.run_id = NEW.run_id), 0)
				AND (COALESCE(CAST(json_extract(run.budget_json, '$.max_tokens') AS INTEGER), 0) = 0
					OR candidate.tokens_used < CAST(json_extract(run.budget_json, '$.max_tokens') AS INTEGER))
				AND (COALESCE(CAST(json_extract(run.budget_json, '$.timeout_seconds') AS INTEGER), 0) = 0
					OR candidate.execution_millis_used < CAST(json_extract(run.budget_json, '$.timeout_seconds') AS INTEGER) * 1000)
				AND (COALESCE(CAST(json_extract(run.budget_json, '$.max_tool_calls') AS INTEGER), 0) = 0
					OR candidate.tool_calls_used < CAST(json_extract(run.budget_json, '$.max_tool_calls') AS INTEGER))
				AND (SELECT COUNT(*) FROM sandbox_execution_inputs input
					JOIN run_artifacts artifact ON artifact.id = input.artifact_id
					WHERE input.execution_id = execution.id AND artifact.run_id = execution.run_id
						AND artifact.session_id = run.session_id
						AND artifact.workspace_id = execution.workspace_id
						AND artifact.sha256 = input.sha256 AND artifact.size_bytes = input.size_bytes
						AND artifact.mime = input.mime AND artifact.stream = input.stream
						AND artifact.source_id = input.source_id AND artifact.redacted = input.redacted)
					= execution.input_artifact_count
				AND COALESCE((SELECT SUM(input.size_bytes) FROM sandbox_execution_inputs input
					WHERE input.execution_id = execution.id), 0) = execution.input_artifact_bytes
				AND julianday(NEW.created_at) >= julianday(observation.created_at)
				AND (SELECT COUNT(*) FROM sandbox_docker_container_plans existing
					WHERE existing.observation_id = NEW.observation_id) < 1
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker container plan authority binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_control_insert
		BEFORE INSERT ON sandbox_docker_container_plan_controls
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_plans plan
			WHERE plan.id = NEW.plan_id AND plan.control_count = 16)
		BEGIN
			SELECT RAISE(ABORT, 'Docker container plan control binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_step_insert
		BEFORE INSERT ON sandbox_docker_container_plan_steps
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_plans plan
			WHERE plan.id = NEW.plan_id AND plan.transaction_step_count = 7)
		BEGIN
			SELECT RAISE(ABORT, 'Docker container plan step binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_operation_insert
		BEFORE INSERT ON sandbox_docker_container_plan_operations
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_plans plan
			WHERE plan.id = NEW.plan_id AND plan.observation_id = NEW.observation_id
				AND plan.run_id = NEW.run_id AND plan.requested_by = NEW.requested_by
				AND plan.created_at = NEW.created_at
				AND (SELECT COUNT(*) FROM sandbox_docker_container_plan_controls control
					WHERE control.plan_id = plan.id) = plan.control_count
				AND (SELECT COALESCE(SUM(control.planned), 0)
					FROM sandbox_docker_container_plan_controls control
					WHERE control.plan_id = plan.id) = plan.control_count
				AND (SELECT COALESCE(SUM(control.applied + control.verified), 0)
					FROM sandbox_docker_container_plan_controls control
					WHERE control.plan_id = plan.id) = 0
				AND (SELECT COUNT(*) FROM sandbox_docker_container_plan_steps step
					WHERE step.plan_id = plan.id) = plan.transaction_step_count
				AND (SELECT COALESCE(SUM(step.simulated), 0)
					FROM sandbox_docker_container_plan_steps step
					WHERE step.plan_id = plan.id) = plan.transaction_step_count
				AND (SELECT COALESCE(SUM(step.production_applied), 0)
					FROM sandbox_docker_container_plan_steps step
					WHERE step.plan_id = plan.id) = 0)
		BEGIN
			SELECT RAISE(ABORT, 'Docker container plan operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_plans BEGIN
			SELECT RAISE(ABORT, 'Docker container plan cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_plans BEGIN
			SELECT RAISE(ABORT, 'Docker container plan cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_control_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_plan_controls BEGIN
			SELECT RAISE(ABORT, 'Docker container plan control cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_control_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_plan_controls BEGIN
			SELECT RAISE(ABORT, 'Docker container plan control cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_step_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_plan_steps BEGIN
			SELECT RAISE(ABORT, 'Docker container plan step cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_step_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_plan_steps BEGIN
			SELECT RAISE(ABORT, 'Docker container plan step cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_operation_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_plan_operations BEGIN
			SELECT RAISE(ABORT, 'Docker container plan operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_plan_operation_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_plan_operations BEGIN
			SELECT RAISE(ABORT, 'Docker container plan operation cannot be deleted');
		END;`,
}
