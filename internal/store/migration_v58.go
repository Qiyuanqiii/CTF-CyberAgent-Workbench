package store

var sandboxDockerHostInputRequirementStatements = []string{
	`CREATE TABLE sandbox_docker_host_input_requirement_legacy_attempts (
		attempt_id TEXT PRIMARY KEY,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		CHECK(attempt_id = trim(attempt_id) AND length(attempt_id) BETWEEN 1 AND 256
			AND instr(attempt_id, char(0)) = 0)
	) WITHOUT ROWID;`,
	`INSERT INTO sandbox_docker_host_input_requirement_legacy_attempts (attempt_id)
		SELECT id FROM sandbox_docker_container_rehearsal_attempts;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_requirement_legacy_insert_immutable
		BEFORE INSERT ON sandbox_docker_host_input_requirement_legacy_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker host input legacy marker cannot be inserted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_requirement_legacy_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_requirement_legacy_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker host input legacy marker cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_requirement_legacy_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_requirement_legacy_attempts BEGIN
			SELECT RAISE(ABORT, 'Docker host input legacy marker cannot be deleted');
		END;`,
	`CREATE TABLE sandbox_docker_host_input_requirements (
		attempt_id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		attempt_intent_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
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
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_host_input_requirement.v1'),
		CHECK(required IN (0, 1) AND operator_confirmed = required),
		CHECK(read_only_mount_count BETWEEN 0 AND 32
			AND (required = 0 OR read_only_mount_count >= 1)),
		CHECK(input_artifact_count BETWEEN 0 AND 16),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(attempt_intent_fingerprint) = 64 AND attempt_intent_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_binding_fingerprint) = 64 AND mount_binding_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(plan_fingerprint) = 64 AND plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(requirement_fingerprint) = 64 AND requirement_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(attempt_id = trim(attempt_id) AND length(attempt_id) BETWEEN 1 AND 256
			AND instr(attempt_id, char(0)) = 0),
		CHECK(plan_id = trim(plan_id) AND length(plan_id) BETWEEN 1 AND 256
			AND instr(plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256
			AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256
			AND instr(mission_id, char(0)) = 0),
		CHECK(workspace_id = trim(workspace_id) AND length(workspace_id) BETWEEN 1 AND 256
			AND instr(workspace_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE INDEX idx_sandbox_docker_host_input_requirements_run_created
		ON sandbox_docker_host_input_requirements(run_id, created_at, attempt_id);`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_requirement_insert
		BEFORE INSERT ON sandbox_docker_host_input_requirements
		WHEN EXISTS (SELECT 1 FROM sandbox_docker_host_input_requirement_legacy_attempts legacy
			WHERE legacy.attempt_id = NEW.attempt_id)
			OR NOT EXISTS (
			SELECT 1 FROM sandbox_docker_container_rehearsal_attempts attempt
			JOIN sandbox_docker_container_plans plan ON plan.id = attempt.plan_id
			WHERE attempt.id = NEW.attempt_id AND attempt.plan_id = NEW.plan_id
				AND attempt.run_id = NEW.run_id AND attempt.mission_id = NEW.mission_id
				AND attempt.workspace_id = NEW.workspace_id
				AND attempt.operation_key_digest = NEW.operation_key_digest
				AND attempt.intent_fingerprint = NEW.attempt_intent_fingerprint
				AND attempt.request_fingerprint = NEW.request_fingerprint
				AND attempt.manifest_fingerprint = NEW.manifest_fingerprint
				AND attempt.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND attempt.input_artifact_digest = NEW.input_artifact_digest
				AND attempt.authority_fingerprint = NEW.authority_fingerprint
				AND attempt.plan_fingerprint = NEW.plan_fingerprint
				AND attempt.requested_by = NEW.requested_by
				AND plan.read_only_mount_count = NEW.read_only_mount_count
				AND plan.input_artifact_count = NEW.input_artifact_count
				AND attempt.created_at = NEW.created_at
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_stages stage
					WHERE stage.attempt_id = attempt.id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_cleanups cleanup
					WHERE cleanup.attempt_id = attempt.id)
				AND NOT EXISTS (SELECT 1 FROM sandbox_docker_container_attempt_completions completion
					WHERE completion.attempt_id = attempt.id)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker host input requirement authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_requirement_staging_compatibility
		BEFORE INSERT ON sandbox_docker_host_input_staging_intents
		WHEN (NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_requirements requirement
				WHERE requirement.attempt_id = NEW.attempt_id)
			AND NOT EXISTS (
				SELECT 1 FROM sandbox_docker_host_input_requirement_legacy_attempts legacy
				WHERE legacy.attempt_id = NEW.attempt_id))
			OR EXISTS (
			SELECT 1 FROM sandbox_docker_host_input_requirements requirement
			WHERE requirement.attempt_id = NEW.attempt_id
				AND (requirement.required = 0 OR requirement.plan_id != NEW.plan_id
					OR requirement.run_id != NEW.run_id
					OR requirement.attempt_intent_fingerprint != NEW.attempt_intent_fingerprint
					OR requirement.request_fingerprint != NEW.request_fingerprint
					OR requirement.manifest_fingerprint != NEW.manifest_fingerprint
					OR requirement.mount_binding_fingerprint != NEW.mount_binding_fingerprint
					OR requirement.input_artifact_digest != NEW.input_artifact_digest
					OR requirement.authority_fingerprint != NEW.authority_fingerprint
					OR requirement.plan_fingerprint != NEW.plan_fingerprint
					OR requirement.read_only_mount_count != NEW.read_only_mount_count
					OR requirement.input_artifact_count != NEW.input_artifact_count
					OR requirement.requested_by != NEW.requested_by)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker host input staging conflicts with the durable requirement');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_attempt_stage_requires_host_input_requirement
		BEFORE INSERT ON sandbox_docker_container_attempt_stages
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_requirements requirement
			WHERE requirement.attempt_id = NEW.attempt_id)
			AND NOT EXISTS (
				SELECT 1 FROM sandbox_docker_host_input_requirement_legacy_attempts legacy
				WHERE legacy.attempt_id = NEW.attempt_id) BEGIN
			SELECT RAISE(ABORT, 'Docker host input requirement is not durable before stage');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_attempt_completion_requires_host_input_requirement
		BEFORE INSERT ON sandbox_docker_container_attempt_completions
		WHEN (NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_requirements requirement
				WHERE requirement.attempt_id = NEW.attempt_id)
			AND NOT EXISTS (
				SELECT 1 FROM sandbox_docker_host_input_requirement_legacy_attempts legacy
				WHERE legacy.attempt_id = NEW.attempt_id))
			OR (EXISTS (SELECT 1 FROM sandbox_docker_host_input_requirements requirement
				WHERE requirement.attempt_id = NEW.attempt_id AND requirement.required = 1)
			AND NOT EXISTS (SELECT 1 FROM sandbox_docker_host_input_stagings staging
				WHERE staging.attempt_id = NEW.attempt_id)) BEGIN
			SELECT RAISE(ABORT, 'Required Docker host input staging is incomplete');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_requirement_update_immutable
		BEFORE UPDATE ON sandbox_docker_host_input_requirements BEGIN
			SELECT RAISE(ABORT, 'Docker host input requirement cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_host_input_requirement_delete_immutable
		BEFORE DELETE ON sandbox_docker_host_input_requirements BEGIN
			SELECT RAISE(ABORT, 'Docker host input requirement cannot be deleted');
		END;`,
}
