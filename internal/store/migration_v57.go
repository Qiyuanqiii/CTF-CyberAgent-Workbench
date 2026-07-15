package store

var sandboxDockerHostInputStagingStatements = []string{
	`CREATE TABLE sandbox_docker_host_input_staging_intents (
		id TEXT PRIMARY KEY,
		attempt_id TEXT NOT NULL UNIQUE,
		plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		attempt_intent_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		container_id_fingerprint TEXT NOT NULL,
		manifest_fingerprint TEXT NOT NULL,
		mount_binding_fingerprint TEXT NOT NULL,
		input_artifact_digest TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		spec_fingerprint TEXT NOT NULL,
		plan_fingerprint TEXT NOT NULL,
		read_only_mount_count INTEGER NOT NULL,
		input_artifact_count INTEGER NOT NULL,
		prepared_generation INTEGER NOT NULL,
		intent_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_host_input_staging_intent.v1'),
		CHECK(read_only_mount_count BETWEEN 1 AND 32),
		CHECK(input_artifact_count BETWEEN 0 AND 16 AND prepared_generation >= 1),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(attempt_intent_fingerprint) = 64 AND attempt_intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_id_fingerprint) = 64 AND container_id_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_binding_fingerprint) = 64 AND mount_binding_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(spec_fingerprint) = 64 AND spec_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_fingerprint) = 64 AND plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(intent_fingerprint) = 64 AND intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(attempt_id = trim(attempt_id) AND length(attempt_id) BETWEEN 1 AND 256
			AND instr(attempt_id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256 AND instr(plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_sandbox_docker_host_input_staging_intents_run_created
		ON sandbox_docker_host_input_staging_intents(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_host_input_stagings (
		id TEXT PRIMARY KEY,
		intent_id TEXT NOT NULL UNIQUE,
		attempt_id TEXT NOT NULL UNIQUE,
		plan_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		source TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		status TEXT NOT NULL,
		lease_generation INTEGER NOT NULL,
		attempt_intent_fingerprint TEXT NOT NULL,
		container_id_fingerprint TEXT NOT NULL,
		input_artifact_digest TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		read_only_mount_count INTEGER NOT NULL,
		input_artifact_count INTEGER NOT NULL,
		bundle_protocol_version TEXT NOT NULL,
		bundle_status TEXT NOT NULL,
		regular_file_count INTEGER NOT NULL,
		directory_count INTEGER NOT NULL,
		entry_count INTEGER NOT NULL,
		source_bytes INTEGER NOT NULL,
		artifact_bytes INTEGER NOT NULL,
		bundle_bytes INTEGER NOT NULL,
		source_snapshot_digest TEXT NOT NULL,
		artifact_payload_digest TEXT NOT NULL,
		bundle_digest TEXT NOT NULL,
		report_fingerprint TEXT NOT NULL,
		descriptor_pinned INTEGER NOT NULL,
		symlink_free INTEGER NOT NULL,
		kernel_sealed INTEGER NOT NULL,
		source_paths_retained INTEGER NOT NULL,
		raw_content_persisted INTEGER NOT NULL,
		daemon_consumed INTEGER NOT NULL,
		container_started INTEGER NOT NULL,
		process_executed INTEGER NOT NULL,
		execution_evidence INTEGER NOT NULL,
		staging_fingerprint TEXT NOT NULL,
		production_verified INTEGER NOT NULL,
		backend_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		bundle_created_at TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(intent_id) REFERENCES sandbox_docker_host_input_staging_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_host_input_staging.v1'),
		CHECK(source = 'linux_openat2_memfd_seal'),
		CHECK(trust_class = 'local_descriptor_rehearsal_unconsumed'),
		CHECK(status = 'host_inputs_descriptor_sealed'),
		CHECK(lease_generation >= 1),
		CHECK(read_only_mount_count BETWEEN 1 AND 32),
		CHECK(input_artifact_count BETWEEN 0 AND 16),
		CHECK(bundle_protocol_version = 'sandbox_host_input_bundle.v1'),
		CHECK(bundle_status = 'descriptor_bundle_sealed'),
		CHECK(regular_file_count >= 0 AND directory_count >= 0
			AND regular_file_count + directory_count >= read_only_mount_count),
		CHECK(entry_count = regular_file_count + directory_count + input_artifact_count
			AND entry_count BETWEEN read_only_mount_count AND 4096),
		CHECK(source_bytes BETWEEN 0 AND 16777216),
		CHECK(artifact_bytes BETWEEN 0 AND 16777216),
		CHECK(bundle_bytes BETWEEN 1 AND 41943040),
		CHECK(descriptor_pinned = 1 AND symlink_free = 1 AND kernel_sealed = 1),
		CHECK(source_paths_retained = 0 AND raw_content_persisted = 0 AND daemon_consumed = 0),
		CHECK(container_started = 0 AND process_executed = 0 AND execution_evidence = 0),
		CHECK(production_verified = 0 AND backend_enabled = 0
			AND execution_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(attempt_intent_fingerprint) = 64 AND attempt_intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_id_fingerprint) = 64 AND container_id_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(source_snapshot_digest) = 64 AND source_snapshot_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(artifact_payload_digest) = 64 AND artifact_payload_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(bundle_digest) = 64 AND bundle_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(report_fingerprint) = 64 AND report_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(staging_fingerprint) = 64 AND staging_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_sandbox_docker_host_input_stagings_run_created
		ON sandbox_docker_host_input_stagings(run_id, created_at, id);`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_staging_intent_insert
		BEFORE INSERT ON sandbox_docker_host_input_staging_intents
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_container_rehearsal_attempts attempt
			JOIN sandbox_docker_container_plans plan ON plan.id = attempt.plan_id
			JOIN sandbox_docker_container_attempt_stages stage ON stage.attempt_id = attempt.id
			JOIN sandbox_docker_container_attempt_leases lease ON lease.attempt_id = attempt.id
			WHERE attempt.id = NEW.attempt_id AND attempt.plan_id = NEW.plan_id
				AND attempt.run_id = NEW.run_id AND attempt.mission_id = NEW.mission_id
				AND attempt.workspace_id = NEW.workspace_id
				AND attempt.intent_fingerprint = NEW.attempt_intent_fingerprint
				AND attempt.request_fingerprint = NEW.request_fingerprint
				AND stage.container_id_fingerprint = NEW.container_id_fingerprint
				AND attempt.manifest_fingerprint = NEW.manifest_fingerprint
				AND attempt.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND attempt.input_artifact_digest = NEW.input_artifact_digest
				AND attempt.authority_fingerprint = NEW.authority_fingerprint
				AND attempt.spec_fingerprint = NEW.spec_fingerprint
				AND attempt.plan_fingerprint = NEW.plan_fingerprint
				AND attempt.requested_by = NEW.requested_by
				AND plan.read_only_mount_count = NEW.read_only_mount_count
				AND plan.input_artifact_count = NEW.input_artifact_count
				AND lease.generation = NEW.prepared_generation AND lease.status = 'active'
				AND julianday(lease.expires_at) > julianday('now')
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.created_at) >= julianday(stage.recorded_at)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_cleanups cleanup
					WHERE cleanup.attempt_id = attempt.id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_completions completion
					WHERE completion.attempt_id = attempt.id)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging intent authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_staging_insert
		BEFORE INSERT ON sandbox_docker_host_input_stagings
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_host_input_staging_intents intent
			JOIN sandbox_docker_container_rehearsal_attempts attempt ON attempt.id = intent.attempt_id
			JOIN sandbox_docker_container_attempt_stages stage ON stage.attempt_id = attempt.id
			JOIN sandbox_docker_container_attempt_leases lease ON lease.attempt_id = attempt.id
			WHERE intent.id = NEW.intent_id AND intent.attempt_id = NEW.attempt_id
				AND intent.plan_id = NEW.plan_id AND intent.run_id = NEW.run_id
				AND intent.attempt_intent_fingerprint = NEW.attempt_intent_fingerprint
				AND intent.container_id_fingerprint = NEW.container_id_fingerprint
				AND intent.input_artifact_digest = NEW.input_artifact_digest
				AND intent.authority_fingerprint = NEW.authority_fingerprint
				AND intent.read_only_mount_count = NEW.read_only_mount_count
				AND intent.input_artifact_count = NEW.input_artifact_count
				AND lease.generation = NEW.lease_generation AND lease.status = 'active'
				AND NEW.lease_generation >= intent.prepared_generation
				AND julianday(lease.expires_at) > julianday('now')
				AND julianday(NEW.created_at) >= julianday(lease.acquired_at)
				AND julianday(NEW.created_at) >= julianday(intent.created_at)
				AND julianday(NEW.bundle_created_at) <= julianday(NEW.created_at)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_completions completion
					WHERE completion.attempt_id = attempt.id)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging lease mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_attempt_completion_requires_host_input_staging
		BEFORE INSERT ON sandbox_docker_container_attempt_completions
		WHEN EXISTS (SELECT 1 FROM sandbox_docker_host_input_staging_intents intent
			WHERE intent.attempt_id = NEW.attempt_id)
			AND NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_stagings staging
				WHERE staging.attempt_id = NEW.attempt_id) BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging is incomplete');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_staging_intent_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_staging_intents BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging intent cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_staging_intent_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_staging_intents BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging intent cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_staging_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_stagings BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_staging_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_stagings BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging cannot be deleted');
		END;`,
}
