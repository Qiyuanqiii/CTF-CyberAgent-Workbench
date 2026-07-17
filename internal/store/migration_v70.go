package store

var externalSkillSelectionStatements = []string{
	`CREATE TABLE run_external_skill_selections (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL UNIQUE,
		mission_id TEXT NOT NULL,
		mode_snapshot_id TEXT NOT NULL,
		mode_revision INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		surface TEXT NOT NULL,
		profile TEXT NOT NULL,
		token_budget INTEGER NOT NULL,
		token_upper_bound INTEGER NOT NULL,
		item_count INTEGER NOT NULL,
		selection_fingerprint TEXT NOT NULL UNIQUE,
		requested_by TEXT NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		context_delivery_authorized INTEGER NOT NULL,
		tool_capability_grant INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(mode_snapshot_id) REFERENCES run_mode_snapshots(id) ON DELETE RESTRICT,
		FOREIGN KEY(id) REFERENCES run_external_skill_selection_operations(selection_id)
			ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
		CHECK(protocol_version = 'external_skill_selection.v1'),
		CHECK(surface IN ('code', 'cyber')),
		CHECK(profile IN ('code', 'review', 'learn', 'script')),
		CHECK(surface != 'cyber' OR profile = 'script'),
		CHECK(mode_revision > 0),
		CHECK(token_budget BETWEEN 1 AND 4096),
		CHECK(token_upper_bound BETWEEN 1 AND token_budget),
		CHECK(item_count BETWEEN 1 AND 4),
		CHECK(operator_confirmed = 1 AND context_delivery_authorized = 1
			AND tool_capability_grant = 0),
		CHECK(length(selection_fingerprint) = 64
			AND selection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256 AND instr(mission_id, char(0)) = 0),
		CHECK(mode_snapshot_id = trim(mode_snapshot_id)
			AND length(mode_snapshot_id) BETWEEN 1 AND 256 AND instr(mode_snapshot_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE TABLE run_external_skill_selection_items (
		selection_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		installation_id TEXT NOT NULL,
		installation_fingerprint TEXT NOT NULL,
		install_result_fingerprint TEXT NOT NULL,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		surface TEXT NOT NULL,
		content_sha256 TEXT NOT NULL,
		content_bytes INTEGER NOT NULL,
		token_upper_bound INTEGER NOT NULL,
		archive_sha256 TEXT NOT NULL,
		archive_bytes INTEGER NOT NULL,
		package_fingerprint TEXT NOT NULL,
		object_key TEXT NOT NULL,
		trust_class TEXT NOT NULL,
		tool_dependency_count INTEGER NOT NULL,
		specialist_eligible INTEGER NOT NULL,
		PRIMARY KEY(selection_id, ordinal),
		UNIQUE(selection_id, installation_id),
		UNIQUE(selection_id, name, version),
		FOREIGN KEY(selection_id) REFERENCES run_external_skill_selections(id) ON DELETE RESTRICT,
		FOREIGN KEY(installation_id) REFERENCES skill_package_installations(id) ON DELETE RESTRICT,
		CHECK(ordinal BETWEEN 1 AND 4),
		CHECK(surface IN ('code', 'cyber')),
		CHECK(name = trim(name) AND length(name) BETWEEN 1 AND 64
			AND name NOT GLOB '*[^a-z0-9-]*' AND name GLOB '[a-z]*' AND substr(name, -1) != '-'),
		CHECK(version = trim(version) AND length(version) BETWEEN 5 AND 29
			AND version NOT GLOB '*[^0-9.]*'),
		CHECK(length(installation_fingerprint) = 64
			AND installation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(install_result_fingerprint) = 64
			AND install_result_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(content_sha256) = 64 AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(content_bytes BETWEEN 1 AND 32768 AND token_upper_bound = content_bytes
			AND token_upper_bound BETWEEN 1 AND 4096),
		CHECK(length(archive_sha256) = 64 AND archive_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(archive_bytes BETWEEN 1 AND 65536),
		CHECK(length(package_fingerprint) = 64
			AND package_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(object_key = 'sha256/' || substr(archive_sha256, 1, 2) || '/' || archive_sha256 || '.zip'),
		CHECK(trust_class = 'operator_installed_untrusted'),
		CHECK(tool_dependency_count BETWEEN 0 AND 8),
		CHECK(specialist_eligible IN (0, 1)
			AND (specialist_eligible = 0 OR token_upper_bound <= 2048)),
		CHECK(installation_id = trim(installation_id)
			AND length(installation_id) BETWEEN 1 AND 256 AND instr(installation_id, char(0)) = 0)
	);`,
	`CREATE TABLE run_external_skill_selection_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL UNIQUE,
		selection_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL UNIQUE,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(selection_id) REFERENCES run_external_skill_selections(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	);`,
	`CREATE TABLE root_external_skill_context_preparations (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		root_agent_id TEXT NOT NULL,
		supervisor_attempt_id TEXT NOT NULL UNIQUE,
		turn_number INTEGER NOT NULL,
		selection_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		surface TEXT NOT NULL,
		profile TEXT NOT NULL,
		selection_fingerprint TEXT NOT NULL,
		context_fingerprint TEXT NOT NULL,
		item_count INTEGER NOT NULL,
		token_budget INTEGER NOT NULL,
		token_upper_bound INTEGER NOT NULL,
		redaction_count INTEGER NOT NULL,
		prepared_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id, root_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE RESTRICT,
		FOREIGN KEY(selection_id) REFERENCES run_external_skill_selections(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'external_skill_context.v1'),
		CHECK(surface IN ('code', 'cyber') AND profile IN ('code', 'review', 'learn', 'script')),
		CHECK(surface != 'cyber' OR profile = 'script'),
		CHECK(turn_number > 0 AND item_count BETWEEN 1 AND 4),
		CHECK(token_budget BETWEEN 1 AND 4096 AND token_upper_bound BETWEEN 1 AND token_budget),
		CHECK(redaction_count BETWEEN 0 AND token_budget),
		CHECK(length(selection_fingerprint) = 64
			AND selection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(context_fingerprint) = 64
			AND context_fingerprint NOT GLOB '*[^0-9a-f]*')
	);`,
	`CREATE TABLE root_external_skill_context_commits (
		preparation_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		supervisor_attempt_id TEXT NOT NULL UNIQUE,
		model_attempt INTEGER NOT NULL,
		committed_at TEXT NOT NULL,
		FOREIGN KEY(preparation_id) REFERENCES root_external_skill_context_preparations(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(model_attempt > 0)
	);`,
	`CREATE TABLE specialist_external_skill_context_preparations (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		parent_agent_id TEXT NOT NULL,
		agent_attempt_id TEXT NOT NULL UNIQUE,
		turn_number INTEGER NOT NULL,
		parent_selection_id TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		parent_selection_fingerprint TEXT NOT NULL,
		mode_snapshot_id TEXT NOT NULL,
		mode_revision INTEGER NOT NULL,
		surface TEXT NOT NULL,
		profile TEXT NOT NULL,
		assignment_fingerprint TEXT NOT NULL,
		context_fingerprint TEXT NOT NULL,
		item_count INTEGER NOT NULL,
		token_budget INTEGER NOT NULL,
		token_upper_bound INTEGER NOT NULL,
		redaction_count INTEGER NOT NULL,
		prepared_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id, agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id, parent_agent_id) REFERENCES agent_nodes(run_id, id) ON DELETE RESTRICT,
		FOREIGN KEY(agent_attempt_id) REFERENCES agent_attempts(id) ON DELETE RESTRICT,
		FOREIGN KEY(parent_selection_id) REFERENCES run_external_skill_selections(id) ON DELETE RESTRICT,
		FOREIGN KEY(mode_snapshot_id) REFERENCES run_mode_snapshots(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'external_specialist_skill_context.v1'),
		CHECK(surface IN ('code', 'cyber') AND profile IN ('code', 'review', 'learn', 'script')),
		CHECK(surface != 'cyber' OR profile = 'script'),
		CHECK(turn_number > 0 AND mode_revision > 0 AND item_count = 1),
		CHECK(token_budget BETWEEN 1 AND 2048 AND token_upper_bound BETWEEN 1 AND token_budget),
		CHECK(redaction_count BETWEEN 0 AND token_budget),
		CHECK(length(parent_selection_fingerprint) = 64
			AND parent_selection_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(assignment_fingerprint) = 64
			AND assignment_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(context_fingerprint) = 64
			AND context_fingerprint NOT GLOB '*[^0-9a-f]*')
	);`,
	`CREATE TABLE specialist_external_skill_context_commits (
		preparation_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		agent_attempt_id TEXT NOT NULL UNIQUE,
		model_attempt INTEGER NOT NULL,
		committed_at TEXT NOT NULL,
		FOREIGN KEY(preparation_id) REFERENCES specialist_external_skill_context_preparations(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(agent_attempt_id) REFERENCES agent_attempts(id) ON DELETE RESTRICT,
		CHECK(model_attempt > 0)
	);`,
	`CREATE INDEX idx_run_external_skill_selections_mission
		ON run_external_skill_selections(mission_id, created_at, id);`,
	`CREATE INDEX idx_root_external_skill_context_run_turn
		ON root_external_skill_context_preparations(run_id, turn_number, prepared_at, id);`,
	`CREATE INDEX idx_specialist_external_skill_context_run_agent_turn
		ON specialist_external_skill_context_preparations(run_id, agent_id, turn_number, prepared_at);`,
	`CREATE TRIGGER trg_run_external_skill_selection_insert
		BEFORE INSERT ON run_external_skill_selections
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN run_mode_snapshots mode ON mode.id = NEW.mode_snapshot_id
			WHERE run.id = NEW.run_id AND run.mission_id = NEW.mission_id
				AND run.status = 'created' AND mission.profile = NEW.profile
				AND mode.run_id = run.id AND mode.mission_id = mission.id
				AND mode.revision = NEW.mode_revision AND mode.surface = NEW.surface
				AND mode.profile = NEW.profile
				AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
					WHERE later.run_id = run.id AND later.revision > mode.revision)
				AND julianday(NEW.created_at) >= julianday(run.created_at))
		BEGIN SELECT RAISE(ABORT, 'external Skill selection Run binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_item_insert
		BEFORE INSERT ON run_external_skill_selection_items
		WHEN NOT EXISTS (
			SELECT 1 FROM run_external_skill_selections selection
			JOIN skill_package_installations installation ON installation.id = NEW.installation_id
			JOIN skill_package_install_results result ON result.installation_id = installation.id
			WHERE selection.id = NEW.selection_id
				AND NEW.ordinal = 1 + (SELECT COUNT(*) FROM run_external_skill_selection_items existing
					WHERE existing.selection_id = NEW.selection_id)
				AND NEW.surface = selection.surface AND installation.surface = selection.surface
				AND installation.installation_fingerprint = NEW.installation_fingerprint
				AND result.result_fingerprint = NEW.install_result_fingerprint
				AND installation.name = NEW.name AND installation.version = NEW.version
				AND installation.content_sha256 = NEW.content_sha256
				AND installation.content_bytes = NEW.content_bytes
				AND installation.content_token_upper_bound = NEW.token_upper_bound
				AND installation.archive_sha256 = NEW.archive_sha256
				AND installation.archive_bytes = NEW.archive_bytes
				AND installation.package_fingerprint = NEW.package_fingerprint
				AND installation.trust_class = NEW.trust_class
				AND json_array_length(installation.tool_dependencies_json) = NEW.tool_dependency_count
				AND EXISTS (SELECT 1 FROM json_each(installation.profiles_json) profile
					WHERE profile.type = 'text' AND profile.value = selection.profile)
				AND result.object_key = NEW.object_key AND result.archive_sha256 = NEW.archive_sha256
				AND result.package_fingerprint = NEW.package_fingerprint
				AND result.object_bytes = NEW.archive_bytes AND result.object_verified = 1
				AND installation.run_selection_authorized = 0
				AND installation.context_injection_authorized = 0
				AND installation.tool_capability_grant = 0
				AND result.run_selection_authorized = 0
				AND result.context_injection_authorized = 0
				AND result.tool_capability_grant = 0
				AND NOT EXISTS (SELECT 1 FROM skill_package_removals removal
					WHERE removal.installation_id = installation.id))
		BEGIN SELECT RAISE(ABORT, 'external Skill selection item binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_operation_insert
		BEFORE INSERT ON run_external_skill_selection_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM run_external_skill_selections selection
			WHERE selection.id = NEW.selection_id AND selection.run_id = NEW.run_id
				AND selection.requested_by = NEW.requested_by
				AND selection.created_at = NEW.created_at
				AND selection.item_count = (SELECT COUNT(*) FROM run_external_skill_selection_items item
					WHERE item.selection_id = selection.id)
				AND selection.token_upper_bound = (SELECT COALESCE(SUM(item.token_upper_bound), 0)
					FROM run_external_skill_selection_items item WHERE item.selection_id = selection.id)
				AND (SELECT COUNT(*) FROM run_external_skill_selection_items item
					WHERE item.selection_id = selection.id AND item.specialist_eligible = 1) <= 1)
		BEGIN SELECT RAISE(ABORT, 'external Skill selection operation binding is invalid'); END;`,
	`CREATE TRIGGER trg_skill_package_removal_external_selection_guard
		BEFORE INSERT ON skill_package_removals
		WHEN EXISTS (SELECT 1 FROM run_external_skill_selection_items item
			WHERE item.installation_id = NEW.installation_id
				AND item.installation_fingerprint = NEW.installation_fingerprint)
		BEGIN SELECT RAISE(ABORT, 'Skill package version is pinned by an external Run selection'); END;`,
	`CREATE TRIGGER trg_root_external_skill_context_preparation_insert
		BEFORE INSERT ON root_external_skill_context_preparations
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN run_supervisor_checkpoints checkpoint ON checkpoint.run_id = run.id
			JOIN agent_nodes root ON root.run_id = run.id AND root.id = NEW.root_agent_id
			JOIN run_external_skill_selections selection ON selection.id = NEW.selection_id
			WHERE run.id = NEW.run_id AND run.mission_id = NEW.mission_id
				AND run.status = 'running' AND mission.profile = NEW.profile
				AND checkpoint.phase = 'turn_started'
				AND checkpoint.attempt_id = NEW.supervisor_attempt_id
				AND checkpoint.next_turn = NEW.turn_number
				AND root.role = 'root' AND root.status = 'running'
				AND root.active_attempt_id = NEW.supervisor_attempt_id
				AND selection.run_id = run.id AND selection.mission_id = mission.id
				AND selection.surface = NEW.surface AND selection.profile = NEW.profile
				AND selection.selection_fingerprint = NEW.selection_fingerprint
				AND selection.item_count = NEW.item_count
				AND selection.token_budget = NEW.token_budget
				AND NEW.token_upper_bound <= selection.token_upper_bound)
		BEGIN SELECT RAISE(ABORT, 'external root Skill context preparation binding is invalid'); END;`,
	`CREATE TRIGGER trg_root_external_skill_context_commit_insert
		BEFORE INSERT ON root_external_skill_context_commits
		WHEN NOT EXISTS (
			SELECT 1 FROM root_external_skill_context_preparations preparation
			JOIN runs run ON run.id = preparation.run_id
			JOIN run_supervisor_checkpoints checkpoint ON checkpoint.run_id = run.id
			JOIN agent_nodes root ON root.run_id = run.id AND root.id = preparation.root_agent_id
			WHERE preparation.id = NEW.preparation_id AND preparation.run_id = NEW.run_id
				AND preparation.supervisor_attempt_id = NEW.supervisor_attempt_id
				AND run.status = 'running' AND checkpoint.phase = 'turn_started'
				AND checkpoint.attempt_id = NEW.supervisor_attempt_id
				AND checkpoint.next_turn = preparation.turn_number
				AND root.role = 'root' AND root.status = 'running'
				AND root.active_attempt_id = NEW.supervisor_attempt_id
				AND julianday(NEW.committed_at) >= julianday(preparation.prepared_at))
		BEGIN SELECT RAISE(ABORT, 'external root Skill context commit binding is invalid'); END;`,
	`CREATE TRIGGER trg_specialist_external_skill_context_preparation_insert
		BEFORE INSERT ON specialist_external_skill_context_preparations
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			JOIN missions mission ON mission.id = run.mission_id
			JOIN agent_attempts attempt ON attempt.id = NEW.agent_attempt_id
			JOIN agent_nodes child ON child.run_id = run.id AND child.id = NEW.agent_id
			JOIN agent_nodes parent ON parent.run_id = run.id AND parent.id = NEW.parent_agent_id
			JOIN run_external_skill_selections selection ON selection.id = NEW.parent_selection_id
			JOIN run_mode_snapshots mode ON mode.id = NEW.mode_snapshot_id
			WHERE run.id = NEW.run_id AND run.mission_id = NEW.mission_id
				AND run.status = 'running' AND mission.profile = NEW.profile
				AND attempt.run_id = run.id AND attempt.agent_id = child.id
				AND attempt.parent_agent_id = parent.id AND attempt.status = 'running'
				AND attempt.turn_number = NEW.turn_number
				AND child.role = 'specialist' AND child.status = 'running'
				AND child.parent_id = parent.id AND child.active_attempt_id = attempt.id
				AND child.profile = NEW.profile AND parent.role = 'root'
				AND parent.status IN ('ready', 'running', 'waiting')
				AND selection.run_id = run.id AND selection.mission_id = mission.id
				AND selection.selection_fingerprint = NEW.parent_selection_fingerprint
				AND selection.surface = NEW.surface AND selection.profile = NEW.profile
				AND mode.run_id = run.id AND mode.mission_id = mission.id
				AND mode.revision = NEW.mode_revision AND mode.surface = NEW.surface
				AND mode.profile = NEW.profile
				AND NOT EXISTS (SELECT 1 FROM run_mode_snapshots later
					WHERE later.run_id = run.id AND later.revision > mode.revision)
				AND EXISTS (SELECT 1 FROM run_external_skill_selection_items item
					WHERE item.selection_id = selection.id AND item.specialist_eligible = 1))
		BEGIN SELECT RAISE(ABORT, 'external Specialist Skill context preparation binding is invalid'); END;`,
	`CREATE TRIGGER trg_specialist_external_skill_context_commit_insert
		BEFORE INSERT ON specialist_external_skill_context_commits
		WHEN NOT EXISTS (
			SELECT 1 FROM specialist_external_skill_context_preparations preparation
			JOIN agent_attempts attempt ON attempt.id = preparation.agent_attempt_id
			JOIN specialist_model_calls model_call ON model_call.agent_attempt_id = attempt.id
				AND model_call.model_attempt_number = NEW.model_attempt
			WHERE preparation.id = NEW.preparation_id AND preparation.run_id = NEW.run_id
				AND preparation.agent_attempt_id = NEW.agent_attempt_id
				AND attempt.status = 'running' AND model_call.status = 'started'
				AND julianday(NEW.committed_at) >= julianday(preparation.prepared_at))
		BEGIN SELECT RAISE(ABORT, 'external Specialist Skill context commit binding is invalid'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_update_immutable BEFORE UPDATE ON run_external_skill_selections
		BEGIN SELECT RAISE(ABORT, 'external Skill selection cannot be updated'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_delete_immutable BEFORE DELETE ON run_external_skill_selections
		BEGIN SELECT RAISE(ABORT, 'external Skill selection cannot be deleted'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_item_update_immutable BEFORE UPDATE ON run_external_skill_selection_items
		BEGIN SELECT RAISE(ABORT, 'external Skill selection item cannot be updated'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_item_delete_immutable BEFORE DELETE ON run_external_skill_selection_items
		BEGIN SELECT RAISE(ABORT, 'external Skill selection item cannot be deleted'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_operation_update_immutable BEFORE UPDATE ON run_external_skill_selection_operations
		BEGIN SELECT RAISE(ABORT, 'external Skill selection operation cannot be updated'); END;`,
	`CREATE TRIGGER trg_run_external_skill_selection_operation_delete_immutable BEFORE DELETE ON run_external_skill_selection_operations
		BEGIN SELECT RAISE(ABORT, 'external Skill selection operation cannot be deleted'); END;`,
	`CREATE TRIGGER trg_root_external_skill_context_preparation_update_immutable BEFORE UPDATE ON root_external_skill_context_preparations
		BEGIN SELECT RAISE(ABORT, 'external root Skill context preparation cannot be updated'); END;`,
	`CREATE TRIGGER trg_root_external_skill_context_preparation_delete_immutable BEFORE DELETE ON root_external_skill_context_preparations
		BEGIN SELECT RAISE(ABORT, 'external root Skill context preparation cannot be deleted'); END;`,
	`CREATE TRIGGER trg_root_external_skill_context_commit_update_immutable BEFORE UPDATE ON root_external_skill_context_commits
		BEGIN SELECT RAISE(ABORT, 'external root Skill context commit cannot be updated'); END;`,
	`CREATE TRIGGER trg_root_external_skill_context_commit_delete_immutable BEFORE DELETE ON root_external_skill_context_commits
		BEGIN SELECT RAISE(ABORT, 'external root Skill context commit cannot be deleted'); END;`,
	`CREATE TRIGGER trg_specialist_external_skill_context_preparation_update_immutable BEFORE UPDATE ON specialist_external_skill_context_preparations
		BEGIN SELECT RAISE(ABORT, 'external Specialist Skill context preparation cannot be updated'); END;`,
	`CREATE TRIGGER trg_specialist_external_skill_context_preparation_delete_immutable BEFORE DELETE ON specialist_external_skill_context_preparations
		BEGIN SELECT RAISE(ABORT, 'external Specialist Skill context preparation cannot be deleted'); END;`,
	`CREATE TRIGGER trg_specialist_external_skill_context_commit_update_immutable BEFORE UPDATE ON specialist_external_skill_context_commits
		BEGIN SELECT RAISE(ABORT, 'external Specialist Skill context commit cannot be updated'); END;`,
	`CREATE TRIGGER trg_specialist_external_skill_context_commit_delete_immutable BEFORE DELETE ON specialist_external_skill_context_commits
		BEGIN SELECT RAISE(ABORT, 'external Specialist Skill context commit cannot be deleted'); END;`,
}
