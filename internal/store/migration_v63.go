package store

var sandboxDockerStartGateReviewStatements = []string{
	`CREATE TABLE sandbox_docker_start_gate_reviews (
		id TEXT PRIMARY KEY,
		cleanup_intent_id TEXT NOT NULL UNIQUE,
		cleanup_result_id TEXT NOT NULL UNIQUE,
		application_intent_id TEXT NOT NULL,
		application_result_id TEXT NOT NULL,
		projection_id TEXT NOT NULL,
		container_plan_id TEXT NOT NULL,
		preflight_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		reviewed_through_schema INTEGER NOT NULL,
		status TEXT NOT NULL,
		decision TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		manifest_fingerprint TEXT NOT NULL,
		threat_model_fingerprint TEXT NOT NULL,
		cleanup_result_fingerprint TEXT NOT NULL,
		max_log_bytes INTEGER NOT NULL,
		authority_fingerprint TEXT NOT NULL,
		evidence_fingerprint TEXT NOT NULL,
		lifecycle_blueprint_fingerprint TEXT NOT NULL,
		review_fingerprint TEXT NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		real_daemon_chain_verified INTEGER NOT NULL,
		required_check_count INTEGER NOT NULL,
		production_verified_count INTEGER NOT NULL,
		sufficient_check_count INTEGER NOT NULL,
		blocker_count INTEGER NOT NULL,
		start_gate_passed INTEGER NOT NULL,
		start_implementation_present INTEGER NOT NULL,
		container_start_authorized INTEGER NOT NULL,
		process_execution_authorized INTEGER NOT NULL,
		output_export_authorized INTEGER NOT NULL,
		artifact_commit_authorized INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(cleanup_intent_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(cleanup_result_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_results(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_intent_id) REFERENCES sandbox_docker_runtime_input_application_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(application_result_id) REFERENCES sandbox_docker_runtime_input_application_results(id) ON DELETE RESTRICT,
		FOREIGN KEY(projection_id) REFERENCES sandbox_docker_runtime_input_projection_completions(projection_id) ON DELETE RESTRICT,
		FOREIGN KEY(container_plan_id) REFERENCES sandbox_docker_container_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY(preflight_id) REFERENCES sandbox_disabled_preflights(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_start_gate_review.v1'),
		CHECK(reviewed_through_schema = 62),
		CHECK(status = 'blocked' AND decision = 'deny_start' AND trust_class = 'design_review_only'),
		CHECK(max_log_bytes BETWEEN 1 AND 67108864),
		CHECK(operator_confirmed = 1 AND real_daemon_chain_verified = 0),
		CHECK(required_check_count = 16 AND production_verified_count = 0
			AND sufficient_check_count = 0 AND blocker_count = required_check_count),
		CHECK(start_gate_passed = 0 AND start_implementation_present = 0
			AND container_start_authorized = 0 AND process_execution_authorized = 0
			AND output_export_authorized = 0 AND artifact_commit_authorized = 0),
		CHECK(length(operation_key_digest) = 64 AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(manifest_fingerprint) = 64 AND manifest_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(threat_model_fingerprint) = 64 AND threat_model_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(cleanup_result_fingerprint) = 64 AND cleanup_result_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(authority_fingerprint) = 64 AND authority_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(evidence_fingerprint) = 64 AND evidence_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(lifecycle_blueprint_fingerprint) = 64 AND lifecycle_blueprint_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(review_fingerprint) = 64 AND review_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE INDEX idx_sandbox_docker_start_gate_reviews_run_created
		ON sandbox_docker_start_gate_reviews(run_id, created_at, id);`,
	`CREATE TABLE sandbox_docker_start_gate_review_checks (
		review_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		name TEXT NOT NULL,
		evidence_class TEXT NOT NULL,
		evidence_source TEXT NOT NULL,
		production_verified INTEGER NOT NULL,
		sufficient_for_start INTEGER NOT NULL,
		blocker_code TEXT NOT NULL,
		future_gate TEXT NOT NULL,
		review_fingerprint TEXT NOT NULL,
		PRIMARY KEY(review_id, ordinal),
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
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
		CHECK(evidence_class = CASE
			WHEN ordinal IN (1, 2, 3, 4, 6, 9, 10, 13) THEN 'never_started_daemon_evidence'
			WHEN ordinal IN (5, 7, 8) THEN 'compiled_only'
			WHEN ordinal IN (11, 12) THEN 'requirement_only'
			WHEN ordinal IN (14, 15, 16) THEN 'simulation_only' END),
		CHECK(evidence_source = CASE ordinal
			WHEN 1 THEN 'v57_v59_descriptor_and_handoff'
			WHEN 2 THEN 'v55_v56_stopped_configuration'
			WHEN 3 THEN 'v55_v56_stopped_configuration'
			WHEN 4 THEN 'v61_readonly_nocopy_mounts'
			WHEN 5 THEN 'v54_compiled_output_plan'
			WHEN 6 THEN 'v55_v56_stopped_network_none'
			WHEN 7 THEN 'v54_empty_allowlist_compiled'
			WHEN 8 THEN 'v54_zero_secret_profile'
			WHEN 9 THEN 'v55_v56_stopped_configuration'
			WHEN 10 THEN 'v55_v56_stopped_configuration'
			WHEN 11 THEN 'v51_requirement_only'
			WHEN 12 THEN 'v51_requirement_only'
			WHEN 13 THEN 'v50_v56_v62_never_started_cleanup'
			WHEN 14 THEN 'v52_simulation_only'
			WHEN 15 THEN 'v52_simulation_only'
			WHEN 16 THEN 'v52_simulation_only' END),
		CHECK(production_verified = 0 AND sufficient_for_start = 0),
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
		CHECK(future_gate = CASE
			WHEN ordinal IN (1, 2, 3, 4, 6, 9, 10) THEN 'runtime_isolation_gate'
			WHEN ordinal IN (7, 8) THEN 'network_secret_gate'
			WHEN ordinal IN (11, 12, 13) THEN 'termination_recovery_gate'
			WHEN ordinal IN (5, 14, 15, 16) THEN 'output_export_gate' END),
		CHECK(length(review_fingerprint) = 64 AND review_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_process_lifecycle_blueprints (
		review_id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		ownership_model TEXT NOT NULL,
		fixed_endpoint_required INTEGER NOT NULL,
		write_ahead_required INTEGER NOT NULL,
		generation_fenced INTEGER NOT NULL,
		cancellation_fanout INTEGER NOT NULL,
		bounded_logs INTEGER NOT NULL,
		max_log_bytes INTEGER NOT NULL,
		wait_required INTEGER NOT NULL,
		graceful_then_forced_kill INTEGER NOT NULL,
		orphan_reconciliation INTEGER NOT NULL,
		implementation_present INTEGER NOT NULL,
		daemon_mutation_enabled INTEGER NOT NULL,
		output_commit_authorized INTEGER NOT NULL,
		blueprint_fingerprint TEXT NOT NULL,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'sandbox_docker_process_lifecycle_blueprint.v1'),
		CHECK(ownership_model = 'per_run_generation_fenced_single_owner'),
		CHECK(fixed_endpoint_required = 1 AND write_ahead_required = 1
			AND generation_fenced = 1 AND cancellation_fanout = 1 AND bounded_logs = 1
			AND wait_required = 1 AND graceful_then_forced_kill = 1
			AND orphan_reconciliation = 1),
		CHECK(max_log_bytes BETWEEN 1 AND 67108864),
		CHECK(implementation_present = 0 AND daemon_mutation_enabled = 0
			AND output_commit_authorized = 0),
		CHECK(length(blueprint_fingerprint) = 64 AND blueprint_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_process_lifecycle_transitions (
		review_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		from_state TEXT NOT NULL,
		to_state TEXT NOT NULL,
		action TEXT NOT NULL,
		write_ahead_required INTEGER NOT NULL,
		generation_fenced INTEGER NOT NULL,
		daemon_mutation INTEGER NOT NULL,
		cancellation_fanout INTEGER NOT NULL,
		implemented INTEGER NOT NULL,
		authorized INTEGER NOT NULL,
		transition_fingerprint TEXT NOT NULL,
		PRIMARY KEY(review_id, ordinal),
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_process_lifecycle_blueprints(review_id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 11),
		CHECK(from_state = CASE ordinal
			WHEN 1 THEN 'absent' WHEN 2 THEN 'start_intent_committed'
			WHEN 3 THEN 'start_submitted' WHEN 4 THEN 'start_submitted'
			WHEN 5 THEN 'running' WHEN 6 THEN 'term_requested'
			WHEN 7 THEN 'term_requested' WHEN 8 THEN 'kill_requested'
			WHEN 9 THEN 'running' WHEN 10 THEN 'running' WHEN 11 THEN 'orphaned' END),
		CHECK(to_state = CASE ordinal
			WHEN 1 THEN 'start_intent_committed' WHEN 2 THEN 'start_submitted'
			WHEN 3 THEN 'running' WHEN 4 THEN 'orphaned'
			WHEN 5 THEN 'term_requested' WHEN 6 THEN 'exited'
			WHEN 7 THEN 'kill_requested' WHEN 8 THEN 'exited'
			WHEN 9 THEN 'exited' WHEN 10 THEN 'orphaned' WHEN 11 THEN 'reconciled' END),
		CHECK(action = CASE ordinal
			WHEN 1 THEN 'persist_start_intent' WHEN 2 THEN 'fixed_endpoint_start'
			WHEN 3 THEN 'inspect_owned_running' WHEN 4 THEN 'reconcile_uncertain_start'
			WHEN 5 THEN 'cancellation_fanout_term' WHEN 6 THEN 'wait_graceful_exit'
			WHEN 7 THEN 'bounded_grace_expired' WHEN 8 THEN 'wait_forced_exit'
			WHEN 9 THEN 'wait_natural_exit' WHEN 10 THEN 'lease_loss_marks_orphan'
			WHEN 11 THEN 'generation_fenced_reconcile' END),
		CHECK(write_ahead_required = CASE WHEN ordinal IN (1, 2, 5, 7, 10, 11) THEN 1 ELSE 0 END),
		CHECK(generation_fenced = 1),
		CHECK(daemon_mutation = CASE WHEN ordinal IN (2, 5, 7, 11) THEN 1 ELSE 0 END),
		CHECK(cancellation_fanout = CASE WHEN ordinal IN (5, 7, 10, 11) THEN 1 ELSE 0 END),
		CHECK(implemented = 0 AND authorized = 0),
		CHECK(length(transition_fingerprint) = 64 AND transition_fingerprint NOT GLOB '*[^0-9a-f]*')
	) WITHOUT ROWID;`,
	`CREATE TABLE sandbox_docker_start_gate_review_operations (
		key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		review_id TEXT NOT NULL UNIQUE,
		cleanup_intent_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(review_id) REFERENCES sandbox_docker_start_gate_reviews(id) ON DELETE RESTRICT,
		FOREIGN KEY(cleanup_intent_id) REFERENCES sandbox_docker_runtime_input_resource_cleanup_intents(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(key_digest) = 64 AND key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256 AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_insert
		BEFORE INSERT ON sandbox_docker_start_gate_reviews
		WHEN NOT EXISTS (
			SELECT 1
			FROM sandbox_docker_runtime_input_resource_cleanup_intents cleanup
			JOIN sandbox_docker_runtime_input_resource_cleanup_results cleanup_result
				ON cleanup_result.intent_id = cleanup.id
			JOIN sandbox_docker_runtime_input_application_intents application
				ON application.id = cleanup.application_intent_id
			JOIN sandbox_docker_runtime_input_application_results application_result
				ON application_result.intent_id = application.id
			JOIN sandbox_docker_runtime_input_projection_plans projection
				ON projection.id = application.projection_id
			JOIN sandbox_docker_container_plans plan ON plan.id = application.container_plan_id
			JOIN sandbox_disabled_preflights preflight ON preflight.id = plan.preflight_id
			WHERE cleanup.id = NEW.cleanup_intent_id
				AND cleanup_result.id = NEW.cleanup_result_id
				AND application.id = NEW.application_intent_id
				AND application_result.id = NEW.application_result_id
				AND projection.id = NEW.projection_id AND plan.id = NEW.container_plan_id
				AND preflight.id = NEW.preflight_id AND application.run_id = NEW.run_id
				AND application.mission_id = NEW.mission_id
				AND application.workspace_id = NEW.workspace_id
				AND cleanup.run_id = NEW.run_id AND cleanup_result.run_id = NEW.run_id
				AND projection.run_id = NEW.run_id AND plan.run_id = NEW.run_id
				AND preflight.run_id = NEW.run_id AND preflight.mission_id = NEW.mission_id
				AND preflight.workspace_id = NEW.workspace_id
				AND cleanup.manifest_fingerprint = NEW.manifest_fingerprint
				AND application.manifest_fingerprint = NEW.manifest_fingerprint
				AND projection.manifest_fingerprint = NEW.manifest_fingerprint
				AND plan.manifest_fingerprint = NEW.manifest_fingerprint
				AND preflight.manifest_fingerprint = NEW.manifest_fingerprint
				AND preflight.threat_model_fingerprint = NEW.threat_model_fingerprint
				AND cleanup_result.result_fingerprint = NEW.cleanup_result_fingerprint
				AND preflight.max_output_bytes = NEW.max_log_bytes
				AND cleanup.requested_by = NEW.requested_by
				AND application.requested_by = NEW.requested_by
				AND projection.requested_by = NEW.requested_by
				AND plan.requested_by = NEW.requested_by
				AND preflight.requested_by = NEW.requested_by
				AND cleanup_result.status = 'exact_owned_resources_absent'
				AND cleanup_result.target_absent = 1 AND cleanup_result.all_volumes_absent = 1
				AND cleanup_result.foreign_resource_detected = 0
				AND cleanup_result.container_start_authorized = 0
				AND cleanup_result.process_execution_authorized = 0
				AND cleanup_result.output_export_authorized = 0
				AND cleanup_result.artifact_commit_authorized = 0
				AND application_result.container_started = 0
				AND application_result.process_executed = 0
				AND application_result.output_exported = 0
				AND application_result.production_verified = 0
				AND application_result.execution_authorized = 0
				AND julianday(NEW.created_at) >= julianday(cleanup_result.created_at)
		) BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review authority mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_operation_insert
		BEFORE INSERT ON sandbox_docker_start_gate_review_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM sandbox_docker_start_gate_reviews review
			JOIN sandbox_docker_process_lifecycle_blueprints blueprint
				ON blueprint.review_id = review.id
			WHERE review.id = NEW.review_id AND review.cleanup_intent_id = NEW.cleanup_intent_id
				AND review.run_id = NEW.run_id AND review.requested_by = NEW.requested_by
				AND review.created_at = NEW.created_at AND review.operation_key_digest = NEW.key_digest
				AND review.lifecycle_blueprint_fingerprint = blueprint.blueprint_fingerprint
				AND (SELECT COUNT(*) FROM sandbox_docker_start_gate_review_checks item
					WHERE item.review_id = review.id) = review.required_check_count
				AND (SELECT COUNT(*) FROM sandbox_docker_process_lifecycle_transitions transition
					WHERE transition.review_id = review.id) = 11
		) BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review operation mismatch');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_update_immutable
		BEFORE UPDATE ON sandbox_docker_start_gate_reviews BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_delete_immutable
		BEFORE DELETE ON sandbox_docker_start_gate_reviews BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_check_update_immutable
		BEFORE UPDATE ON sandbox_docker_start_gate_review_checks BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review check cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_check_delete_immutable
		BEFORE DELETE ON sandbox_docker_start_gate_review_checks BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review check cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_process_lifecycle_blueprint_update_immutable
		BEFORE UPDATE ON sandbox_docker_process_lifecycle_blueprints BEGIN
			SELECT RAISE(ABORT, 'Docker process lifecycle blueprint cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_process_lifecycle_blueprint_delete_immutable
		BEFORE DELETE ON sandbox_docker_process_lifecycle_blueprints BEGIN
			SELECT RAISE(ABORT, 'Docker process lifecycle blueprint cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_process_lifecycle_transition_update_immutable
		BEFORE UPDATE ON sandbox_docker_process_lifecycle_transitions BEGIN
			SELECT RAISE(ABORT, 'Docker process lifecycle transition cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_process_lifecycle_transition_delete_immutable
		BEFORE DELETE ON sandbox_docker_process_lifecycle_transitions BEGIN
			SELECT RAISE(ABORT, 'Docker process lifecycle transition cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_operation_update_immutable
		BEFORE UPDATE ON sandbox_docker_start_gate_review_operations BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_sandbox_docker_start_gate_review_operation_delete_immutable
		BEFORE DELETE ON sandbox_docker_start_gate_review_operations BEGIN
			SELECT RAISE(ABORT, 'Docker start-gate review operation cannot be deleted');
		END;`,
}
