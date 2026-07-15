package store

var sandboxDockerRuntimeInputProjectionStatements = []string{
	`CREATE TABLE sandbox_docker_runtime_input_projection_plans (
		id TEXT PRIMARY KEY,
		handoff_id TEXT NOT NULL UNIQUE,
		handoff_intent_id TEXT NOT NULL UNIQUE,
		attempt_id TEXT NOT NULL UNIQUE,
		container_plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		manifest_fingerprint TEXT NOT NULL,
		mount_binding_fingerprint TEXT NOT NULL,
		input_artifact_digest TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		spec_fingerprint TEXT NOT NULL,
		container_plan_fingerprint TEXT NOT NULL,
		handoff_fingerprint TEXT NOT NULL,
		handoff_transport_fingerprint TEXT NOT NULL,
		bundle_report_fingerprint TEXT NOT NULL,
		bundle_digest TEXT NOT NULL,
		bundle_bytes INTEGER NOT NULL,
		read_only_mount_count INTEGER NOT NULL,
		input_artifact_count INTEGER NOT NULL,
		projection_count INTEGER NOT NULL,
		directory_root_count INTEGER NOT NULL,
		file_root_count INTEGER NOT NULL,
		total_entry_count INTEGER NOT NULL,
		total_content_bytes INTEGER NOT NULL,
		total_projection_bytes INTEGER NOT NULL,
		projection_set_fingerprint TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		projection_fingerprint TEXT NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		exact_target_binding INTEGER NOT NULL,
		all_volumes_read_only INTEGER NOT NULL,
		all_volumes_no_copy INTEGER NOT NULL,
		bundle_recaptured INTEGER NOT NULL,
		bundle_digest_matched INTEGER NOT NULL,
		daemon_contacted INTEGER NOT NULL,
		daemon_applied INTEGER NOT NULL,
		container_started INTEGER NOT NULL,
		process_executed INTEGER NOT NULL,
		output_exported INTEGER NOT NULL,
		production_execution_submitted INTEGER NOT NULL,
		production_verified INTEGER NOT NULL,
		backend_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(handoff_id) REFERENCES sandbox_docker_host_input_handoffs(id) ON DELETE RESTRICT,
		FOREIGN KEY(handoff_intent_id) REFERENCES sandbox_docker_host_input_handoff_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(attempt_id) REFERENCES sandbox_docker_container_rehearsal_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_runtime_input_projection_plan.v1'),
		CHECK(status = 'compiled_not_applied'),
		CHECK(trust_class = 'handoff_bound_projection_plan_unapplied'),
		CHECK(bundle_bytes BETWEEN 1 AND 41943040),
		CHECK(read_only_mount_count BETWEEN 1 AND 32),
		CHECK(input_artifact_count BETWEEN 0 AND 16),
		CHECK(projection_count BETWEEN 1 AND 33
			AND projection_count = read_only_mount_count + CASE WHEN input_artifact_count > 0 THEN 1 ELSE 0 END),
		CHECK(directory_root_count = read_only_mount_count AND file_root_count = 0),
		CHECK(total_entry_count BETWEEN read_only_mount_count AND 4096),
		CHECK(total_content_bytes BETWEEN 0 AND 33554432),
		CHECK(total_projection_bytes BETWEEN 1 AND 1384120320),
		CHECK(operator_confirmed = 1),
		CHECK(exact_target_binding = 1 AND all_volumes_read_only = 1 AND all_volumes_no_copy = 1),
		CHECK(bundle_recaptured = 1 AND bundle_digest_matched = 1),
		CHECK(daemon_contacted = 0 AND daemon_applied = 0 AND container_started = 0
			AND process_executed = 0 AND output_exported = 0),
		CHECK(production_execution_submitted = 0 AND production_verified = 0
			AND backend_enabled = 0 AND execution_authorized = 0
			AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(mount_binding_fingerprint) = 64 AND mount_binding_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(input_artifact_digest) = 64 AND input_artifact_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(spec_fingerprint) = 64 AND spec_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(container_plan_fingerprint) = 64 AND container_plan_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(handoff_fingerprint) = 64 AND handoff_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(handoff_transport_fingerprint) = 64 AND handoff_transport_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(bundle_report_fingerprint) = 64 AND bundle_report_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(bundle_digest) = 64 AND bundle_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(projection_set_fingerprint) = 64 AND projection_set_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(projection_fingerprint) = 64 AND projection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(handoff_id = trim(handoff_id) AND length(handoff_id) BETWEEN 1 AND 256 AND instr(handoff_id, char(0)) = 0),
		CHECK(handoff_intent_id = trim(handoff_intent_id) AND length(handoff_intent_id) BETWEEN 1 AND 256 AND instr(handoff_intent_id, char(0)) = 0),
		CHECK(attempt_id = trim(attempt_id) AND length(attempt_id) BETWEEN 1 AND 256 AND instr(attempt_id, char(0)) = 0),
		CHECK(container_plan_id = trim(container_plan_id) AND length(container_plan_id) BETWEEN 1 AND 256 AND instr(container_plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256 AND instr(mission_id, char(0)) = 0),
		CHECK(workspace_id = trim(workspace_id) AND length(workspace_id) BETWEEN 1 AND 256 AND instr(workspace_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_runtime_input_projection_plans_run_created
		ON sandbox_docker_runtime_input_projection_plans(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_runtime_input_projection_items (
		projection_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		kind TEXT NOT NULL,
		manifest_mount_ordinal INTEGER NOT NULL,
		target_fingerprint TEXT NOT NULL,
		archive_root_fingerprint TEXT NOT NULL,
		volume_name_fingerprint TEXT NOT NULL,
		entry_count INTEGER NOT NULL,
		regular_file_count INTEGER NOT NULL,
		directory_count INTEGER NOT NULL,
		content_bytes INTEGER NOT NULL,
		projection_archive_bytes INTEGER NOT NULL,
		content_digest TEXT NOT NULL,
		projection_archive_digest TEXT NOT NULL,
		root_directory INTEGER NOT NULL,
		read_only INTEGER NOT NULL,
		exact_target INTEGER NOT NULL,
		no_copy INTEGER NOT NULL,
		daemon_applied INTEGER NOT NULL,
		container_started INTEGER NOT NULL,
		process_executed INTEGER NOT NULL,
		production_execution_submitted INTEGER NOT NULL,
		item_fingerprint TEXT NOT NULL,
		PRIMARY KEY(projection_id, ordinal),
		UNIQUE(projection_id, kind, manifest_mount_ordinal),
		UNIQUE(projection_id, target_fingerprint),
		UNIQUE(projection_id, volume_name_fingerprint),
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_plans(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_runtime_input_projection_item.v1'),
		CHECK(kind IN ('manifest_directory_mount', 'input_artifact_directory')),
		CHECK(ordinal BETWEEN 1 AND 33),
		CHECK((kind = 'manifest_directory_mount' AND manifest_mount_ordinal BETWEEN 1 AND 32
			AND directory_count >= 1 AND entry_count >= 1)
			OR (kind = 'input_artifact_directory' AND manifest_mount_ordinal = 0
				AND regular_file_count >= 1 AND directory_count = 0 AND content_bytes >= 1)),
		CHECK(entry_count BETWEEN 0 AND 4096
			AND regular_file_count BETWEEN 0 AND entry_count
			AND directory_count BETWEEN 0 AND entry_count
			AND regular_file_count + directory_count = entry_count),
		CHECK(content_bytes BETWEEN 0 AND 33554432),
		CHECK(projection_archive_bytes BETWEEN 1 AND 41943040),
		CHECK(root_directory = 1 AND read_only = 1 AND exact_target = 1 AND no_copy = 1),
		CHECK(daemon_applied = 0 AND container_started = 0 AND process_executed = 0
			AND production_execution_submitted = 0),
		CHECK(length(target_fingerprint) = 64 AND target_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(archive_root_fingerprint) = 64 AND archive_root_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(volume_name_fingerprint) = 64 AND volume_name_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(content_digest) = 64 AND content_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(projection_archive_digest) = 64 AND projection_archive_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(item_fingerprint) = 64 AND item_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_runtime_input_projection_completions (
		projection_id TEXT PRIMARY KEY,
		projection_fingerprint TEXT NOT NULL,
		projection_count INTEGER NOT NULL,
		total_entry_count INTEGER NOT NULL,
		total_content_bytes INTEGER NOT NULL,
		total_projection_bytes INTEGER NOT NULL,
		completed_at TEXT NOT NULL,
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_plans(id) ON DELETE RESTRICT,
		CHECK(projection_count BETWEEN 1 AND 33),
		CHECK(total_entry_count BETWEEN 1 AND 4096),
		CHECK(total_content_bytes BETWEEN 0 AND 33554432),
		CHECK(total_projection_bytes BETWEEN 1 AND 1384120320),
		CHECK(length(projection_fingerprint) = 64 AND projection_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_runtime_input_projection_operations (
		key_digest TEXT PRIMARY KEY,
		projection_id TEXT NOT NULL UNIQUE,
		handoff_id TEXT NOT NULL UNIQUE,
		container_plan_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		request_fingerprint TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_completions(projection_id) ON DELETE RESTRICT,
		FOREIGN KEY(handoff_id) REFERENCES sandbox_docker_host_input_handoffs(id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		CHECK(length(key_digest) = 64 AND key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(projection_id = trim(projection_id) AND length(projection_id) BETWEEN 1 AND 256 AND instr(projection_id, char(0)) = 0),
		CHECK(handoff_id = trim(handoff_id) AND length(handoff_id) BETWEEN 1 AND 256 AND instr(handoff_id, char(0)) = 0),
		CHECK(container_plan_id = trim(container_plan_id) AND length(container_plan_id) BETWEEN 1 AND 256 AND instr(container_plan_id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_plan_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_projection_plans
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_host_input_handoffs handoff
			JOIN sandbox_docker_host_input_handoff_intents intent ON intent.id = handoff.intent_id
			JOIN sandbox_docker_container_rehearsal_attempts attempt ON attempt.id = handoff.attempt_id
			JOIN sandbox_docker_container_attempt_completions completion ON completion.attempt_id = attempt.id
			JOIN sandbox_docker_container_plans plan ON plan.id = attempt.plan_id
			WHERE handoff.id = NEW.handoff_id AND intent.id = NEW.handoff_intent_id
				AND attempt.id = NEW.attempt_id AND plan.id = NEW.container_plan_id
				AND handoff.attempt_id = attempt.id AND handoff.plan_id = plan.id
				AND handoff.run_id = NEW.run_id AND intent.mission_id = NEW.mission_id
				AND intent.workspace_id = NEW.workspace_id
				AND plan.manifest_fingerprint = NEW.manifest_fingerprint
				AND plan.mount_binding_fingerprint = NEW.mount_binding_fingerprint
				AND plan.input_artifact_digest = NEW.input_artifact_digest
				AND plan.authority_fingerprint = NEW.authority_fingerprint
				AND plan.spec_fingerprint = NEW.spec_fingerprint
				AND plan.plan_fingerprint = NEW.container_plan_fingerprint
				AND handoff.handoff_fingerprint = NEW.handoff_fingerprint
				AND handoff.transport_fingerprint = NEW.handoff_transport_fingerprint
				AND handoff.bundle_report_fingerprint = NEW.bundle_report_fingerprint
				AND handoff.bundle_digest = NEW.bundle_digest
				AND intent.bundle_bytes = NEW.bundle_bytes
				AND plan.read_only_mount_count = NEW.read_only_mount_count
				AND plan.input_artifact_count = NEW.input_artifact_count
				AND plan.requested_by = NEW.requested_by
				AND julianday(NEW.created_at) >= julianday(handoff.created_at)
				AND julianday(NEW.created_at) >= julianday(completion.completed_at)
				AND handoff.daemon_consumed = 1 AND handoff.readback_verified = 1
				AND handoff.final_mount_read_only = 1 AND handoff.cleanup_confirmed = 1
				AND handoff.container_started = 0 AND handoff.process_executed = 0
				AND handoff.output_exported = 0 AND handoff.production_execution_submitted = 0
				AND handoff.production_verified = 0 AND handoff.backend_enabled = 0
				AND handoff.execution_authorized = 0 AND handoff.artifact_commit_authorized = 0
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_item_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_projection_items
		WHEN NOT EXISTS (SELECT 1 FROM sandbox_docker_runtime_input_projection_plans plan
			WHERE plan.id = NEW.projection_id AND NEW.ordinal <= plan.projection_count
				AND ((NEW.kind = 'manifest_directory_mount'
						AND NEW.manifest_mount_ordinal <= plan.read_only_mount_count)
					OR (NEW.kind = 'input_artifact_directory'
						AND plan.input_artifact_count > 0)))
			OR EXISTS (SELECT 1 FROM sandbox_docker_runtime_input_projection_completions completion
				WHERE completion.projection_id = NEW.projection_id) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection item authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_completion_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_projection_completions
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_projection_plans plan
			WHERE plan.id = NEW.projection_id
				AND plan.projection_fingerprint = NEW.projection_fingerprint
				AND plan.projection_count = NEW.projection_count
				AND plan.total_entry_count = NEW.total_entry_count
				AND plan.total_content_bytes = NEW.total_content_bytes
				AND plan.total_projection_bytes = NEW.total_projection_bytes
				AND NEW.completed_at = plan.created_at
				AND (SELECT COUNT(*) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id) = plan.projection_count
				AND (SELECT MIN(ordinal) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id) = 1
				AND (SELECT MAX(ordinal) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id) = plan.projection_count
				AND (SELECT COALESCE(SUM(entry_count), 0) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id) = plan.total_entry_count
				AND (SELECT COALESCE(SUM(content_bytes), 0) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id) = plan.total_content_bytes
				AND (SELECT COALESCE(SUM(projection_archive_bytes), 0) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id) = plan.total_projection_bytes
				AND (SELECT COUNT(*) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id AND item.kind = 'manifest_directory_mount') = plan.read_only_mount_count
				AND (SELECT COUNT(*) FROM sandbox_docker_runtime_input_projection_items item
					WHERE item.projection_id = plan.id AND item.kind = 'input_artifact_directory') =
					CASE WHEN plan.input_artifact_count > 0 THEN 1 ELSE 0 END
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection is incomplete');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_operation_insert
		BEFORE INSERT ON sandbox_docker_runtime_input_projection_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_runtime_input_projection_plans plan
			JOIN sandbox_docker_runtime_input_projection_completions completion
				ON completion.projection_id = plan.id
			WHERE plan.id = NEW.projection_id AND plan.handoff_id = NEW.handoff_id
				AND plan.container_plan_id = NEW.container_plan_id AND plan.run_id = NEW.run_id
				AND plan.operation_key_digest = NEW.key_digest
				AND plan.request_fingerprint = NEW.request_fingerprint
				AND plan.requested_by = NEW.requested_by AND plan.created_at = NEW.created_at
		) BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection operation mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_plan_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_projection_plans BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection plan cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_plan_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_projection_plans BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection plan cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_item_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_projection_items BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection item cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_item_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_projection_items BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection item cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_completion_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_projection_completions BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection completion cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_completion_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_projection_completions BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection completion cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_operation_update_immutable
		BEFORE UPDATE ON sandbox_docker_runtime_input_projection_operations BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_runtime_input_projection_operation_delete_immutable
		BEFORE DELETE ON sandbox_docker_runtime_input_projection_operations BEGIN
			SELECT RAISE(ABORT, 'Docker runtime input projection operation cannot be deleted');
		END;`,
}
