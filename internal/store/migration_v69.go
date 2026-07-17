package store

var skillPackageInstallationStatements = []string{
	`CREATE TABLE skill_package_install_operations (
		key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL UNIQUE,
		installation_id TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		surface TEXT NOT NULL,
		installed_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(installation_id) REFERENCES skill_package_installations(id)
			ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
		CHECK(length(key_digest) = 64 AND key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(surface IN ('code', 'cyber')),
		CHECK(name = trim(name) AND length(name) BETWEEN 1 AND 64
			AND name NOT GLOB '*[^a-z0-9-]*' AND name GLOB '[a-z]*'
			AND substr(name, -1) != '-'),
		CHECK(version = trim(version) AND length(version) BETWEEN 5 AND 29
			AND version NOT GLOB '*[^0-9.]*'),
		CHECK(installed_by = trim(installed_by) AND length(installed_by) BETWEEN 1 AND 256
			AND instr(installed_by, char(0)) = 0)
	);`,
	`CREATE TABLE skill_package_installations (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		surface TEXT NOT NULL,
		manifest_protocol TEXT NOT NULL,
		description TEXT NOT NULL,
		profiles_json TEXT NOT NULL,
		tool_dependencies_json TEXT NOT NULL,
		content_path TEXT NOT NULL,
		content_sha256 TEXT NOT NULL,
		content_bytes INTEGER NOT NULL,
		content_token_upper_bound INTEGER NOT NULL,
		archive_sha256 TEXT NOT NULL UNIQUE,
		package_fingerprint TEXT NOT NULL UNIQUE,
		archive_bytes INTEGER NOT NULL,
		uncompressed_bytes INTEGER NOT NULL,
		entry_count INTEGER NOT NULL,
		trust_class TEXT NOT NULL,
		risk_codes_json TEXT NOT NULL,
		executable_asset_count INTEGER NOT NULL,
		install_hook_count INTEGER NOT NULL,
		import_command_execution INTEGER NOT NULL,
		import_network_access INTEGER NOT NULL,
		import_provider_calls INTEGER NOT NULL,
		tool_capability_grant INTEGER NOT NULL,
		run_selection_authorized INTEGER NOT NULL,
		context_injection_authorized INTEGER NOT NULL,
		operator_confirmed INTEGER NOT NULL,
		installation_fingerprint TEXT NOT NULL UNIQUE,
		installed_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(operation_key_digest) REFERENCES skill_package_install_operations(key_digest)
			ON DELETE RESTRICT,
		UNIQUE(name, version),
		CHECK(protocol_version = 'skill_package_installation.v1'),
		CHECK(manifest_protocol = 'skill.v1'),
		CHECK(surface IN ('code', 'cyber')),
		CHECK(name = trim(name) AND length(name) BETWEEN 1 AND 64
			AND name NOT GLOB '*[^a-z0-9-]*' AND name GLOB '[a-z]*'
			AND substr(name, -1) != '-'
			AND name NOT IN ('code', 'review', 'learn', 'script', 'plan-delivery')),
		CHECK(version = trim(version) AND length(version) BETWEEN 5 AND 29
			AND version NOT GLOB '*[^0-9.]*'),
		CHECK(description = trim(description) AND length(description) BETWEEN 1 AND 2048
			AND instr(description, char(0)) = 0),
		CHECK(json_valid(profiles_json) AND json_type(profiles_json) = 'array'
			AND json_array_length(profiles_json) BETWEEN 1 AND 4),
		CHECK(json_valid(tool_dependencies_json)
			AND json_type(tool_dependencies_json) = 'array'
			AND json_array_length(tool_dependencies_json) BETWEEN 0 AND 8),
		CHECK(content_path = 'SKILL.md'),
		CHECK(length(content_sha256) = 64 AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(content_bytes BETWEEN 1 AND 32768
			AND content_token_upper_bound BETWEEN 1 AND 4096),
		CHECK(length(archive_sha256) = 64 AND archive_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(package_fingerprint) = 64
			AND package_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(archive_bytes BETWEEN 1 AND 65536
			AND uncompressed_bytes BETWEEN 1 AND 49152 AND entry_count = 2),
		CHECK(trust_class = 'operator_installed_untrusted'),
		CHECK(risk_codes_json = '["untrusted_instructions","declared_tools_not_capabilities"]'),
		CHECK(executable_asset_count = 0 AND install_hook_count = 0
			AND import_command_execution = 0 AND import_network_access = 0
			AND import_provider_calls = 0 AND tool_capability_grant = 0
			AND run_selection_authorized = 0 AND context_injection_authorized = 0
			AND operator_confirmed = 1),
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(installation_fingerprint) = 64
			AND installation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(installed_by = trim(installed_by) AND length(installed_by) BETWEEN 1 AND 256
			AND instr(installed_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER skill_package_installation_insert_guard
		BEFORE INSERT ON skill_package_installations
		BEGIN
			SELECT RAISE(ABORT, 'Skill package installation operation binding mismatch')
			WHERE NOT EXISTS (
				SELECT 1 FROM skill_package_install_operations operation
				WHERE operation.key_digest = NEW.operation_key_digest
					AND operation.request_fingerprint = NEW.request_fingerprint
					AND operation.installation_id = NEW.id
					AND operation.name = NEW.name AND operation.version = NEW.version
					AND operation.surface = NEW.surface
					AND operation.installed_by = NEW.installed_by
					AND operation.created_at = NEW.created_at);
			SELECT RAISE(ABORT, 'Skill package installation Profile metadata is invalid')
			WHERE EXISTS (SELECT 1 FROM json_each(NEW.profiles_json)
				WHERE type != 'text' OR value NOT IN ('code', 'review', 'learn', 'script'));
			SELECT RAISE(ABORT, 'Cyber Skill package must be script-only')
			WHERE NEW.surface = 'cyber' AND NEW.profiles_json != '["script"]';
			SELECT RAISE(ABORT, 'Skill package tool dependency metadata is invalid')
			WHERE EXISTS (SELECT 1 FROM json_each(NEW.tool_dependencies_json)
				WHERE type != 'text'
					OR value NOT IN ('list_workspace', 'read_file', 'replace_file', 'script_process'));
			SELECT RAISE(ABORT, 'Skill package Registry capacity exceeded')
			WHERE (SELECT COUNT(*) FROM skill_package_installations) >= 64;
			SELECT RAISE(ABORT, 'Skill package version history capacity exceeded')
			WHERE (SELECT COUNT(*) FROM skill_package_installations
				WHERE name = NEW.name) >= 8;
		END;`,
	`CREATE TABLE skill_package_install_results (
		installation_id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		installation_fingerprint TEXT NOT NULL UNIQUE,
		object_key TEXT NOT NULL UNIQUE,
		archive_sha256 TEXT NOT NULL UNIQUE,
		package_fingerprint TEXT NOT NULL UNIQUE,
		object_bytes INTEGER NOT NULL,
		object_verified INTEGER NOT NULL,
		run_selection_authorized INTEGER NOT NULL,
		context_injection_authorized INTEGER NOT NULL,
		tool_capability_grant INTEGER NOT NULL,
		result_fingerprint TEXT NOT NULL UNIQUE,
		completed_at TEXT NOT NULL,
		FOREIGN KEY(installation_id) REFERENCES skill_package_installations(id) ON DELETE RESTRICT,
		CHECK(protocol_version = 'skill_package_install_result.v1'),
		CHECK(object_key = 'sha256/' || substr(archive_sha256, 1, 2) || '/' || archive_sha256 || '.zip'),
		CHECK(length(archive_sha256) = 64 AND archive_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(package_fingerprint) = 64
			AND package_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(object_bytes BETWEEN 1 AND 65536 AND object_verified = 1),
		CHECK(run_selection_authorized = 0 AND context_injection_authorized = 0
			AND tool_capability_grant = 0),
		CHECK(length(installation_fingerprint) = 64
			AND installation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(result_fingerprint) = 64
			AND result_fingerprint NOT GLOB '*[^0-9a-f]*')
	);`,
	`CREATE TRIGGER skill_package_install_result_insert_guard
		BEFORE INSERT ON skill_package_install_results
		BEGIN
			SELECT RAISE(ABORT, 'Skill package installation result binding mismatch')
			WHERE NOT EXISTS (
				SELECT 1 FROM skill_package_installations installation
				WHERE installation.id = NEW.installation_id
					AND installation.installation_fingerprint = NEW.installation_fingerprint
					AND installation.archive_sha256 = NEW.archive_sha256
					AND installation.package_fingerprint = NEW.package_fingerprint
					AND installation.archive_bytes = NEW.object_bytes
					AND installation.created_at <= NEW.completed_at);
		END;`,
	`CREATE TABLE skill_package_remove_operations (
		key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL UNIQUE,
		removal_id TEXT NOT NULL UNIQUE,
		installation_id TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		surface TEXT NOT NULL,
		removed_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(removal_id) REFERENCES skill_package_removals(id)
			ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
		FOREIGN KEY(installation_id) REFERENCES skill_package_installations(id) ON DELETE RESTRICT,
		CHECK(length(key_digest) = 64 AND key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(surface IN ('code', 'cyber')),
		CHECK(removed_by = trim(removed_by) AND length(removed_by) BETWEEN 1 AND 256
			AND instr(removed_by, char(0)) = 0)
	);`,
	`CREATE TABLE skill_package_removals (
		id TEXT PRIMARY KEY,
		protocol_version TEXT NOT NULL,
		installation_id TEXT NOT NULL UNIQUE,
		installation_fingerprint TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		surface TEXT NOT NULL,
		content_sha256 TEXT NOT NULL,
		archive_sha256 TEXT NOT NULL,
		package_fingerprint TEXT NOT NULL,
		operation_key_digest TEXT NOT NULL UNIQUE,
		request_fingerprint TEXT NOT NULL UNIQUE,
		package_object_retained INTEGER NOT NULL,
		historical_recovery_preserved INTEGER NOT NULL,
		future_selection_enabled INTEGER NOT NULL,
		run_selection_authorized INTEGER NOT NULL,
		context_injection_authorized INTEGER NOT NULL,
		tool_capability_grant INTEGER NOT NULL,
		removal_fingerprint TEXT NOT NULL UNIQUE,
		removed_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(installation_id) REFERENCES skill_package_installations(id) ON DELETE RESTRICT,
		FOREIGN KEY(operation_key_digest) REFERENCES skill_package_remove_operations(key_digest)
			ON DELETE RESTRICT,
		CHECK(protocol_version = 'skill_package_removal.v1'),
		CHECK(surface IN ('code', 'cyber')),
		CHECK(length(content_sha256) = 64 AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(archive_sha256) = 64 AND archive_sha256 NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(package_fingerprint) = 64
			AND package_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(package_object_retained = 1 AND historical_recovery_preserved = 1
			AND future_selection_enabled = 0 AND run_selection_authorized = 0
			AND context_injection_authorized = 0 AND tool_capability_grant = 0),
		CHECK(length(installation_fingerprint) = 64
			AND installation_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(removal_fingerprint) = 64
			AND removal_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(removed_by = trim(removed_by) AND length(removed_by) BETWEEN 1 AND 256
			AND instr(removed_by, char(0)) = 0)
	);`,
	`CREATE TRIGGER skill_package_removal_insert_guard
		BEFORE INSERT ON skill_package_removals
		BEGIN
			SELECT RAISE(ABORT, 'Skill package removal operation binding mismatch')
			WHERE NOT EXISTS (
				SELECT 1 FROM skill_package_remove_operations operation
				WHERE operation.key_digest = NEW.operation_key_digest
					AND operation.request_fingerprint = NEW.request_fingerprint
					AND operation.removal_id = NEW.id
					AND operation.installation_id = NEW.installation_id
					AND operation.name = NEW.name AND operation.version = NEW.version
					AND operation.surface = NEW.surface
					AND operation.removed_by = NEW.removed_by
					AND operation.created_at = NEW.created_at);
			SELECT RAISE(ABORT, 'Skill package removal installation binding mismatch')
			WHERE NOT EXISTS (
				SELECT 1 FROM skill_package_installations installation
				JOIN skill_package_install_results result
					ON result.installation_id = installation.id
				WHERE installation.id = NEW.installation_id
					AND installation.installation_fingerprint = NEW.installation_fingerprint
					AND installation.name = NEW.name AND installation.version = NEW.version
					AND installation.surface = NEW.surface
					AND installation.content_sha256 = NEW.content_sha256
					AND installation.archive_sha256 = NEW.archive_sha256
					AND installation.package_fingerprint = NEW.package_fingerprint
					AND result.completed_at <= NEW.created_at);
			SELECT RAISE(ABORT, 'Skill package version is pinned by a Run')
			WHERE EXISTS (
				SELECT 1 FROM run_skill_selection_items item
				WHERE item.name = NEW.name AND item.version = NEW.version
					AND item.content_sha256 = NEW.content_sha256);
		END;`,
	`CREATE INDEX idx_skill_package_installations_surface_name
		ON skill_package_installations(surface, name, version);`,
	`CREATE INDEX idx_skill_package_install_results_completed
		ON skill_package_install_results(completed_at, installation_id);`,
	`CREATE INDEX idx_skill_package_removals_created
		ON skill_package_removals(created_at, installation_id);`,
	`CREATE TRIGGER skill_package_install_operations_no_update
		BEFORE UPDATE ON skill_package_install_operations
		BEGIN SELECT RAISE(ABORT, 'Skill package installation operations are immutable'); END;`,
	`CREATE TRIGGER skill_package_install_operations_no_delete
		BEFORE DELETE ON skill_package_install_operations
		BEGIN SELECT RAISE(ABORT, 'Skill package installation operations are immutable'); END;`,
	`CREATE TRIGGER skill_package_installations_no_update
		BEFORE UPDATE ON skill_package_installations
		BEGIN SELECT RAISE(ABORT, 'Skill package installations are immutable'); END;`,
	`CREATE TRIGGER skill_package_installations_no_delete
		BEFORE DELETE ON skill_package_installations
		BEGIN SELECT RAISE(ABORT, 'Skill package installations are immutable'); END;`,
	`CREATE TRIGGER skill_package_install_results_no_update
		BEFORE UPDATE ON skill_package_install_results
		BEGIN SELECT RAISE(ABORT, 'Skill package installation results are immutable'); END;`,
	`CREATE TRIGGER skill_package_install_results_no_delete
		BEFORE DELETE ON skill_package_install_results
		BEGIN SELECT RAISE(ABORT, 'Skill package installation results are immutable'); END;`,
	`CREATE TRIGGER skill_package_remove_operations_no_update
		BEFORE UPDATE ON skill_package_remove_operations
		BEGIN SELECT RAISE(ABORT, 'Skill package removal operations are immutable'); END;`,
	`CREATE TRIGGER skill_package_remove_operations_no_delete
		BEFORE DELETE ON skill_package_remove_operations
		BEGIN SELECT RAISE(ABORT, 'Skill package removal operations are immutable'); END;`,
	`CREATE TRIGGER skill_package_removals_no_update
		BEFORE UPDATE ON skill_package_removals
		BEGIN SELECT RAISE(ABORT, 'Skill package removals are immutable'); END;`,
	`CREATE TRIGGER skill_package_removals_no_delete
		BEFORE DELETE ON skill_package_removals
		BEGIN SELECT RAISE(ABORT, 'Skill package removals are immutable'); END;`,
}
