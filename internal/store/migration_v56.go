package store

var sandboxDockerContainerAttemptStatements = []string{
	`CREATE TABLE sandbox_docker_container_rehearsal_attempts (
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
		operation_key_digest TEXT NOT NULL UNIQUE,
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
		request_fingerprint TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		network_mode TEXT NOT NULL,
		environment_count INTEGER NOT NULL,
		secret_reference_count INTEGER NOT NULL,
		intent_fingerprint TEXT NOT NULL,
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
		CHECK(protocol_version = 'sandbox_docker_container_rehearsal_attempt.v1'),
		CHECK(endpoint_class = 'local_unix'),
		CHECK(network_mode = 'disabled' AND environment_count = 0 AND secret_reference_count = 0),
		CHECK(length(image_digest) = 71 AND substr(image_digest, 1, 7) = 'sha256:'
			AND substr(image_digest, 8) = lower(substr(image_digest, 8))
			AND substr(image_digest, 8) NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authorization_fingerprint) = 64 AND authorization_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(policy_fingerprint) = 64 AND policy_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_binding_fingerprint) = 64 AND mount_binding_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(threat_model_fingerprint) = 64 AND threat_model_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(output_plan_fingerprint) = 64 AND output_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(observation_fingerprint) = 64 AND observation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(spec_fingerprint) = 64 AND spec_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_fingerprint) = 64 AND plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_container_attempts_run_created
		ON sandbox_docker_container_rehearsal_attempts(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_container_attempt_leases (
		attempt_id TEXT PRIMARY KEY,
		lease_id TEXT NOT NULL UNIQUE,
		owner_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		status TEXT NOT NULL,
		acquired_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		released_at TEXT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		CHECK(generation >= 1),
		CHECK(status IN ('active', 'released')),
		CHECK((status = 'active' AND released_at IS NULL)
			OR (status = 'released' AND released_at IS NOT NULL)),
		CHECK(lease_id = trim(lease_id) AND length(lease_id) BETWEEN 1 AND 256 AND instr(lease_id, char(0)) = 0),
		CHECK(owner_id = trim(owner_id) AND length(owner_id) BETWEEN 1 AND 256 AND instr(owner_id, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_attempt_stages (
		attempt_id TEXT PRIMARY KEY,
		lease_generation INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		spec_fingerprint TEXT NOT NULL,
		container_id_fingerprint TEXT NOT NULL,
		inspection_fingerprint TEXT NOT NULL,
		control_matrix_fingerprint TEXT NOT NULL,
		stage_fingerprint TEXT NOT NULL,
		control_count INTEGER NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		daemon_write_count INTEGER NOT NULL,
		container_created_now INTEGER NOT NULL,
		existing_container_adopted INTEGER NOT NULL,
		configuration_matched INTEGER NOT NULL,
		container_present INTEGER NOT NULL,
		container_never_started INTEGER NOT NULL,
		process_never_executed INTEGER NOT NULL,
		image_never_pulled INTEGER NOT NULL,
		output_never_exported INTEGER NOT NULL,
		production_execution_submitted INTEGER NOT NULL,
		production_verified INTEGER NOT NULL,
		backend_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		checkpoint_fingerprint TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_container_stage.v1'),
		CHECK(status = 'stopped_container_verified' AND endpoint_class = 'local_unix'),
		CHECK(control_count = 19 AND daemon_read_count = 3),
		CHECK((container_created_now = 1 AND existing_container_adopted = 0 AND daemon_write_count = 1)
			OR (container_created_now = 0 AND existing_container_adopted = 1 AND daemon_write_count = 0)),
		CHECK(configuration_matched = 1 AND container_present = 1),
		CHECK(container_never_started = 1 AND process_never_executed = 1
			AND image_never_pulled = 1 AND output_never_exported = 1),
		CHECK(production_execution_submitted = 0 AND production_verified = 0
			AND backend_enabled = 0 AND execution_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(spec_fingerprint) = 64 AND spec_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_id_fingerprint) = 64 AND container_id_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(inspection_fingerprint) = 64 AND inspection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(control_matrix_fingerprint) = 64 AND control_matrix_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(stage_fingerprint) = 64 AND stage_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(checkpoint_fingerprint) = 64 AND checkpoint_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_attempt_controls (
		attempt_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		name TEXT NOT NULL,
		state TEXT NOT NULL,
		observed INTEGER NOT NULL,
		verified INTEGER NOT NULL,
		execution_evidence INTEGER NOT NULL,
		control_digest TEXT NOT NULL,
		PRIMARY KEY(attempt_id, ordinal),
		UNIQUE(attempt_id, name),
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_attempt_stages(attempt_id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 19),
		CHECK((ordinal = 1 AND name = 'image_digest_exact')
			OR (ordinal = 2 AND name = 'command_and_workdir_exact')
			OR (ordinal = 3 AND name = 'non_root_user')
			OR (ordinal = 4 AND name = 'rootfs_read_only')
			OR (ordinal = 5 AND name = 'no_new_privileges')
			OR (ordinal = 6 AND name = 'capabilities_dropped')
			OR (ordinal = 7 AND name = 'init_enabled')
			OR (ordinal = 8 AND name = 'network_disabled')
			OR (ordinal = 9 AND name = 'environment_empty')
			OR (ordinal = 10 AND name = 'secrets_absent')
			OR (ordinal = 11 AND name = 'mount_configuration_exact_private')
			OR (ordinal = 12 AND name = 'resources_bounded')
			OR (ordinal = 13 AND name = 'restart_disabled')
			OR (ordinal = 14 AND name = 'logging_disabled')
			OR (ordinal = 15 AND name = 'devices_absent')
			OR (ordinal = 16 AND name = 'ports_absent')
			OR (ordinal = 17 AND name = 'attachments_disabled')
			OR (ordinal = 18 AND name = 'authority_labels_exact')
			OR (ordinal = 19 AND name = 'container_never_started')),
		CHECK(state = 'verified' AND observed = 1 AND verified = 1 AND execution_evidence = 0),
		CHECK(length(control_digest) = 64 AND control_digest NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_attempt_cleanups (
		attempt_id TEXT PRIMARY KEY,
		lease_generation INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		endpoint_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		container_id_fingerprint TEXT NOT NULL,
		cleanup_fingerprint TEXT NOT NULL,
		daemon_read_count INTEGER NOT NULL,
		daemon_write_count INTEGER NOT NULL,
		container_removed_now INTEGER NOT NULL,
		container_already_absent INTEGER NOT NULL,
		cleanup_confirmed INTEGER NOT NULL,
		container_never_started INTEGER NOT NULL,
		process_never_executed INTEGER NOT NULL,
		output_never_exported INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		checkpoint_fingerprint TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_container_cleanup.v1'),
		CHECK(status IN ('container_removed', 'container_already_absent') AND endpoint_class = 'local_unix'),
		CHECK(daemon_read_count = 1),
		CHECK((status = 'container_removed' AND container_removed_now = 1
			AND container_already_absent = 0 AND daemon_write_count = 1)
			OR (status = 'container_already_absent' AND container_removed_now = 0
				AND container_already_absent = 1 AND daemon_write_count = 0)),
		CHECK(cleanup_confirmed = 1 AND container_never_started = 1
			AND process_never_executed = 1 AND output_never_exported = 1
			AND execution_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(endpoint_fingerprint) = 64 AND endpoint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_id_fingerprint) = 64 AND container_id_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(cleanup_fingerprint) = 64 AND cleanup_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(checkpoint_fingerprint) = 64 AND checkpoint_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_attempt_failures (
		attempt_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		lease_generation INTEGER NOT NULL,
		phase TEXT NOT NULL,
		code TEXT NOT NULL,
		retryable INTEGER NOT NULL,
		failure_fingerprint TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY(attempt_id, ordinal),
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 16 AND lease_generation >= 1),
		CHECK(phase IN ('stage', 'cleanup', 'completion')),
		CHECK(retryable IN (0, 1)),
		CHECK(code IN ('transport_disabled', 'transport_unsupported', 'connection_failed',
			'invalid_response', 'unsafe_existing_container', 'unsafe_image_profile',
			'create_conflict', 'configuration_mismatch', 'cleanup_failed',
			'context_canceled', 'deadline_exceeded', 'checkpoint_failure')),
		CHECK(length(failure_fingerprint) = 64 AND failure_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_container_attempt_completions (
		attempt_id TEXT PRIMARY KEY,
		rehearsal_id TEXT NOT NULL UNIQUE,
		lease_generation INTEGER NOT NULL,
		completion_fingerprint TEXT NOT NULL,
		completed_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(rehearsal_id) REFERENCES sandbox_docker_container_rehearsals(id) ON DELETE RESTRICT,
		CHECK(lease_generation >= 1),
		CHECK(length(completion_fingerprint) = 64 AND completion_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_insert
		BEFORE INSERT ON sandbox_docker_container_rehearsal_attempts
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
				AND execution.backend_started = 0 AND sandbox_lease.status = 'released'
				AND run.status IN ('created', 'preparing', 'running', 'waiting_approval', 'paused')
				AND NOT EXISTS (SELECT 1 FROM sandbox_execution_cancellations cancellation
					WHERE cancellation.execution_id = execution.id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_cleanup_results cleanup
					WHERE cleanup.execution_id = execution.id)
				AND NOT EXISTS (SELECT 1 FROM run_execution_leases run_lease
					WHERE run_lease.run_id = NEW.run_id AND run_lease.status = 'active'
						AND julianday(run_lease.expires_at) > julianday('now'))
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_rehearsals rehearsal
					WHERE rehearsal.plan_id = NEW.plan_id)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_lease_insert
		BEFORE INSERT ON sandbox_docker_container_attempt_leases
		WHEN NEW.generation != 1 OR NEW.status != 'active' OR NEW.released_at IS NOT NULL
			OR NOT EXISTS (SELECT 1 FROM sandbox_docker_container_rehearsal_attempts attempt
				WHERE attempt.id = NEW.attempt_id) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt initial lease mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_lease_update
		BEFORE UPDATE ON sandbox_docker_container_attempt_leases
		WHEN NOT (
			(OLD.status = 'active' AND NEW.status = 'released'
				AND NEW.attempt_id = OLD.attempt_id AND NEW.lease_id = OLD.lease_id
				AND NEW.owner_id = OLD.owner_id AND NEW.generation = OLD.generation
				AND NEW.acquired_at = OLD.acquired_at AND NEW.expires_at = OLD.expires_at
				AND NEW.released_at IS NOT NULL
				AND julianday(NEW.released_at) >= julianday(OLD.acquired_at))
			OR ((OLD.status = 'released' OR julianday(OLD.expires_at) <= julianday('now'))
				AND NEW.status = 'active' AND NEW.attempt_id = OLD.attempt_id
				AND NEW.generation = OLD.generation + 1 AND NEW.released_at IS NULL
				AND julianday(NEW.expires_at) > julianday(NEW.acquired_at)
				AND julianday(NEW.expires_at) > julianday('now')
				AND ((OLD.status = 'released'
						AND julianday(NEW.acquired_at) >= julianday(OLD.released_at))
					OR (OLD.status = 'active'
						AND julianday(NEW.acquired_at) >= julianday(OLD.expires_at))))
		) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt lease transition mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_stage_insert
		BEFORE INSERT ON sandbox_docker_container_attempt_stages
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_rehearsal_attempts attempt
			JOIN sandbox_docker_container_attempt_leases lease ON lease.attempt_id = attempt.id
			WHERE attempt.id = NEW.attempt_id AND attempt.request_fingerprint = NEW.request_fingerprint
				AND attempt.spec_fingerprint = NEW.spec_fingerprint
				AND attempt.endpoint_class = NEW.endpoint_class
				AND attempt.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND julianday(NEW.recorded_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.recorded_at) >= julianday(attempt.created_at)) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt stage lease mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_control_insert
		BEFORE INSERT ON sandbox_docker_container_attempt_controls
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_stages stage
			WHERE stage.attempt_id = NEW.attempt_id) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt control mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_cleanup_insert
		BEFORE INSERT ON sandbox_docker_container_attempt_cleanups
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_stages stage
			JOIN sandbox_docker_container_attempt_leases lease ON lease.attempt_id = stage.attempt_id
			WHERE stage.attempt_id = NEW.attempt_id
				AND stage.request_fingerprint = NEW.request_fingerprint
				AND stage.container_id_fingerprint = NEW.container_id_fingerprint
				AND stage.endpoint_fingerprint = NEW.endpoint_fingerprint
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND julianday(NEW.recorded_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.recorded_at) >= julianday(stage.recorded_at)) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt cleanup lease mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_failure_insert
		BEFORE INSERT ON sandbox_docker_container_attempt_failures
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_leases lease
			WHERE lease.attempt_id = NEW.attempt_id AND lease.generation = NEW.lease_generation
				AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
				AND NEW.ordinal = 1 + (SELECT COUNT(*)
					FROM sandbox_docker_container_attempt_failures prior
					WHERE prior.attempt_id = NEW.attempt_id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_failures prior
					WHERE prior.attempt_id = NEW.attempt_id
						AND julianday(prior.created_at) > julianday(NEW.created_at))) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt failure lease mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_completion_insert
		BEFORE INSERT ON sandbox_docker_container_attempt_completions
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_container_rehearsal_attempts attempt
			JOIN sandbox_docker_container_attempt_stages stage ON stage.attempt_id = attempt.id
			JOIN sandbox_docker_container_attempt_cleanups cleanup ON cleanup.attempt_id = attempt.id
			JOIN sandbox_docker_container_attempt_leases lease ON lease.attempt_id = attempt.id
			JOIN sandbox_docker_container_rehearsals rehearsal ON rehearsal.id = NEW.rehearsal_id
			WHERE attempt.id = NEW.attempt_id AND rehearsal.plan_id = attempt.plan_id
				AND rehearsal.run_id = attempt.run_id AND rehearsal.requested_by = attempt.requested_by
				AND rehearsal.request_fingerprint = attempt.request_fingerprint
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND julianday(NEW.completed_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.completed_at) >= julianday(cleanup.recorded_at)
				AND (SELECT COUNT(*) FROM sandbox_docker_container_attempt_controls control
					WHERE control.attempt_id = attempt.id) = stage.control_count) BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt completion mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_rehearsal_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_rehearsal_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_lease_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_attempt_leases BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt lease cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_stage_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_attempt_stages BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt stage cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_stage_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_attempt_stages BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt stage cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_control_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_attempt_controls BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt control cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_control_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_attempt_controls BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt control cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_cleanup_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_attempt_cleanups BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt cleanup cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_cleanup_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_attempt_cleanups BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt cleanup cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_failure_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_attempt_failures BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt failure cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_failure_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_attempt_failures BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt failure cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_completion_update_immutable
		BEFORE UPDATE ON sandbox_docker_container_attempt_completions BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt completion cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_container_attempt_completion_delete_immutable
		BEFORE DELETE ON sandbox_docker_container_attempt_completions BEGIN
			SELECT RAISE(ABORT, 'Docker container attempt completion cannot be deleted');
		END;`,
}
