package store

var sandboxDockerProductionEvidenceStatements = []string{
	`CREATE TABLE sandbox_docker_production_evidence (
		id TEXT PRIMARY KEY,
		review_id TEXT NOT NULL,
		cleanup_intent_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		review_fingerprint TEXT NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		threat_model_fingerprint TEXT NOT NULL,
		source TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		status TEXT NOT NULL,
		platform_class TEXT NOT NULL,
		endpoint_class TEXT NOT NULL,
		suite_fingerprint TEXT NOT NULL,
		environment_fingerprint TEXT NOT NULL,
		evidence_fingerprint TEXT NOT NULL,
		capture_fingerprint TEXT NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		real_daemon_contacted INTEGER NOT NULL,
		required_check_count INTEGER NOT NULL,
		observed_count INTEGER NOT NULL,
		production_verified_count INTEGER NOT NULL,
		sufficient_check_count INTEGER NOT NULL,
		blocker_count INTEGER NOT NULL,
		start_gate_passed INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(cleanup_intent_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_production_evidence.v1'),
		CHECK(source = 'go_local_collector'
			AND trust_class = 'machine_observation_non_authorizing'),
		CHECK(
			(status = 'unsupported_platform' AND platform_class = 'unsupported'
				AND endpoint_class = 'none' AND real_daemon_contacted = 0
				AND observed_count = 0 AND production_verified_count = 0)
			OR (status IN ('opt_in_required', 'harness_pending') AND platform_class = 'linux'
				AND endpoint_class = 'local_unix' AND real_daemon_contacted = 0
				AND observed_count = 0 AND production_verified_count = 0)
			OR (status = 'capture_complete' AND platform_class = 'linux'
				AND endpoint_class = 'local_unix' AND real_daemon_contacted = 1
				AND observed_count BETWEEN 0 AND 16
				AND production_verified_count BETWEEN 0 AND observed_count)
		),
		CHECK(operator_confirmed = 1),
		CHECK(required_check_count = 16 AND sufficient_check_count = 0
			AND blocker_count = required_check_count),
		CHECK(start_gate_passed = 0 AND container_start_authorized = 0
			AND process_execution_authorized = 0 AND output_export_authorized = 0
			AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(review_fingerprint) = 64 AND review_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(threat_model_fingerprint) = 64 AND threat_model_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(suite_fingerprint) = 64 AND suite_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(environment_fingerprint) = 64 AND environment_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(evidence_fingerprint) = 64 AND evidence_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(capture_fingerprint) = 64 AND capture_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_production_evidence_run_created
		ON sandbox_docker_production_evidence(run_id, created_at, id);`,
	`CREATE INDEX idx_sandbox_docker_production_evidence_review_created
		ON sandbox_docker_production_evidence(review_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_production_evidence_items (
		evidence_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		name TEXT NOT NULL,
		probe_code TEXT NOT NULL,
		state TEXT NOT NULL,
		observed INTEGER NOT NULL,
		production_verified INTEGER NOT NULL,
		sufficient_for_start INTEGER NOT NULL,
		blocker_code TEXT NOT NULL,
		evidence_digest TEXT NOT NULL,
		PRIMARY KEY(evidence_id, ordinal),
		FOREIGN KEY(evidence_id) REFERENCES sandbox_docker_production_evidence(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 16),
		CHECK(name = CASE ordinal
			WHEN 1 THEN 'host_path_isolation'
			WHEN 2 THEN 'mount_propagation_private'
			WHEN 3 THEN 'read_only_rootfs'
			WHEN 4 THEN 'read_only_inputs'
			WHEN 5 THEN 'dedicated_writable_output'
			WHEN 6 THEN 'network_default_deny'
			WHEN 7 THEN 'exact_network_allowlist'
			WHEN 8 THEN 'ephemeral_secret_materialization'
			WHEN 9 THEN 'non_root_container_identity'
			WHEN 10 THEN 'cpu_memory_pid_limits'
			WHEN 11 THEN 'wall_clock_timeout'
			WHEN 12 THEN 'graceful_then_forced_kill'
			WHEN 13 THEN 'orphan_reconciliation'
			WHEN 14 THEN 'output_regular_file_only'
			WHEN 15 THEN 'output_symlink_special_rejection'
			WHEN 16 THEN 'atomic_output_artifact_commit' END),
		CHECK(probe_code = CASE ordinal
			WHEN 1 THEN 'probe_host_path_isolation'
			WHEN 2 THEN 'probe_private_mount_propagation'
			WHEN 3 THEN 'probe_read_only_rootfs'
			WHEN 4 THEN 'probe_read_only_inputs'
			WHEN 5 THEN 'probe_dedicated_writable_output'
			WHEN 6 THEN 'probe_network_default_deny'
			WHEN 7 THEN 'probe_exact_network_allowlist'
			WHEN 8 THEN 'probe_ephemeral_secret_materialization'
			WHEN 9 THEN 'probe_non_root_identity'
			WHEN 10 THEN 'probe_cpu_memory_pid_limits'
			WHEN 11 THEN 'probe_wall_clock_timeout'
			WHEN 12 THEN 'probe_term_kill_escalation'
			WHEN 13 THEN 'probe_orphan_reconciliation'
			WHEN 14 THEN 'probe_output_regular_file_only'
			WHEN 15 THEN 'probe_output_link_special_rejection'
			WHEN 16 THEN 'probe_atomic_artifact_commit' END),
		CHECK((state = 'not_observed' AND observed = 0 AND production_verified = 0)
			OR (state = 'observed_failed' AND observed = 1 AND production_verified = 0)
			OR (state = 'production_verified' AND observed = 1 AND production_verified = 1)),
		CHECK(sufficient_for_start = 0),
		CHECK(blocker_code = CASE ordinal
			WHEN 1 THEN 'running_host_path_isolation_unverified'
			WHEN 2 THEN 'running_mount_propagation_unverified'
			WHEN 3 THEN 'running_rootfs_readonly_unverified'
			WHEN 4 THEN 'running_input_readonly_unverified'
			WHEN 5 THEN 'writable_output_isolation_unimplemented'
			WHEN 6 THEN 'running_network_deny_unverified'
			WHEN 7 THEN 'network_allowlist_enforcement_unimplemented'
			WHEN 8 THEN 'ephemeral_secret_materialization_unimplemented'
			WHEN 9 THEN 'running_nonroot_identity_unverified'
			WHEN 10 THEN 'running_resource_limits_unverified'
			WHEN 11 THEN 'wall_clock_supervision_unimplemented'
			WHEN 12 THEN 'term_kill_escalation_unimplemented'
			WHEN 13 THEN 'running_orphan_reconciliation_unimplemented'
			WHEN 14 THEN 'output_regular_file_validation_unimplemented'
			WHEN 15 THEN 'output_link_special_rejection_unimplemented'
			WHEN 16 THEN 'atomic_artifact_commit_unimplemented' END),
		CHECK(length(evidence_digest) = 64 AND evidence_digest NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_production_evidence_operations (
		key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		evidence_id TEXT NOT NULL UNIQUE,
		review_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(evidence_id) REFERENCES sandbox_docker_production_evidence(id) ON DELETE RESTRICT,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(key_digest) = 64 AND key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_insert
		BEFORE INSERT ON sandbox_docker_production_evidence
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
				AND (SELECT COUNT(*) FROM sandbox_docker_production_evidence existing
					WHERE existing.run_id = NEW.run_id) < 32
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence authority binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_item_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_items
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence evidence
			WHERE evidence.id = NEW.evidence_id
				AND (evidence.status = 'capture_complete' OR NEW.state = 'not_observed')
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence item status is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_operation_insert
		BEFORE INSERT ON sandbox_docker_production_evidence_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_production_evidence evidence
			WHERE evidence.id = NEW.evidence_id
				AND evidence.operation_key_digest = NEW.key_digest
				AND evidence.review_id = NEW.review_id
				AND evidence.run_id = NEW.run_id AND evidence.requested_by = NEW.requested_by
				AND evidence.created_at = NEW.created_at
				AND (SELECT COUNT(*) FROM sandbox_docker_production_evidence_items item
					WHERE item.evidence_id = evidence.id) = evidence.required_check_count
				AND (SELECT COALESCE(SUM(item.observed), 0)
					FROM sandbox_docker_production_evidence_items item
					WHERE item.evidence_id = evidence.id) = evidence.observed_count
				AND (SELECT COALESCE(SUM(item.production_verified), 0)
					FROM sandbox_docker_production_evidence_items item
					WHERE item.evidence_id = evidence.id) = evidence.production_verified_count
				AND (SELECT COALESCE(SUM(item.sufficient_for_start), 0)
					FROM sandbox_docker_production_evidence_items item
					WHERE item.evidence_id = evidence.id) = 0
		)
		BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_item_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_items BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence item cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_item_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_items BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence item cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_operation_update_immutable
		BEFORE UPDATE ON sandbox_docker_production_evidence_operations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_production_evidence_operation_delete_immutable
		BEFORE DELETE ON sandbox_docker_production_evidence_operations BEGIN
			SELECT RAISE(ABORT, 'Docker production evidence operation cannot be deleted');
		END;`,
}
