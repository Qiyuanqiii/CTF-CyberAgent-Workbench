package store

var runExecutionProfileStatements = []string{
	`CREATE TABLE run_execution_profile_snapshots (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		protocol_version TEXT NOT NULL,
		profile TEXT NOT NULL,
		backend TEXT NOT NULL,
		approval_policy TEXT NOT NULL,
		filesystem_scope TEXT NOT NULL,
		network_scope TEXT NOT NULL,
		risk_tier TEXT NOT NULL,
		required_gate TEXT NOT NULL,
		policy_version TEXT NOT NULL,
		process_enabled INTEGER NOT NULL,
		execution_authorized INTEGER NOT NULL,
		capability_grant INTEGER NOT NULL,
		requested_by TEXT NOT NULL,
		reason TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT,
		UNIQUE(run_id, revision),
		CHECK(revision > 0),
		CHECK(protocol_version = 'run_execution_profile.v1'),
		CHECK(policy_version = 'execution_profile_policy.v1'),
		CHECK(process_enabled = 0 AND execution_authorized = 0 AND capability_grant = 0),
		CHECK(network_scope = 'disabled'),
		CHECK(
			(profile = 'preview' AND backend = 'noop' AND approval_policy = 'none'
				AND filesystem_scope = 'none' AND risk_tier = 'minimal'
				AND required_gate = 'none')
			OR (profile = 'docker' AND backend = 'docker' AND approval_policy = 'always'
				AND filesystem_scope = 'workspace' AND risk_tier = 'elevated'
				AND required_gate = 'docker_production_start_gate')
			OR (profile = 'local' AND backend = 'local' AND approval_policy = 'always'
				AND filesystem_scope = 'workspace' AND risk_tier = 'high'
				AND required_gate = 'local_os_sandbox_gate')
		),
		CHECK(id = trim(id) AND length(id) BETWEEN 1 AND 256 AND instr(id, char(0)) = 0),
		CHECK(run_id = trim(run_id) AND length(run_id) BETWEEN 1 AND 256 AND instr(run_id, char(0)) = 0),
		CHECK(mission_id = trim(mission_id) AND length(mission_id) BETWEEN 1 AND 256
			AND instr(mission_id, char(0)) = 0),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0),
		CHECK(reason = trim(reason) AND length(reason) BETWEEN 1 AND 1024
			AND instr(reason, char(0)) = 0)
	);`,
	`CREATE INDEX idx_run_execution_profile_snapshots_run_revision
		ON run_execution_profile_snapshots(run_id, revision DESC);`,
	`CREATE TABLE run_execution_profile_operations (
		operation_key_digest TEXT PRIMARY KEY,
		request_fingerprint TEXT NOT NULL,
		snapshot_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(snapshot_id) REFERENCES run_execution_profile_snapshots(id) ON DELETE RESTRICT,
		FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE RESTRICT,
		CHECK(length(operation_key_digest) = 64
			AND operation_key_digest = lower(operation_key_digest)
			AND operation_key_digest NOT GLOB '*[^0-9a-f]*'),
		CHECK(length(request_fingerprint) = 64
			AND request_fingerprint = lower(request_fingerprint)
			AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
		CHECK(requested_by = trim(requested_by) AND length(requested_by) BETWEEN 1 AND 256
			AND instr(requested_by, char(0)) = 0)
	) WITHOUT ROWID;`,
	`INSERT INTO run_execution_profile_snapshots
		(id, run_id, mission_id, revision, protocol_version, profile, backend,
		approval_policy, filesystem_scope, network_scope, risk_tier, required_gate,
		policy_version, process_enabled, execution_authorized, capability_grant,
		requested_by, reason, created_at)
		SELECT printf('run-exec-v64-%016x', run.rowid), run.id, run.mission_id, 1,
			'run_execution_profile.v1', 'preview', 'noop', 'none', 'none', 'disabled',
			'minimal', 'none', 'execution_profile_policy.v1', 0, 0, 0,
			'schema_v64', 'legacy compatibility default', run.created_at
		FROM runs run;`,
	`CREATE TRIGGER trg_run_execution_profile_snapshot_insert
		BEFORE INSERT ON run_execution_profile_snapshots
		WHEN NOT EXISTS (
			SELECT 1 FROM runs run
			WHERE run.id = NEW.run_id AND run.mission_id = NEW.mission_id
				AND julianday(NEW.created_at) >= julianday(run.created_at)
				AND (
					(NEW.revision = 1 AND run.status = 'created' AND NOT EXISTS (
						SELECT 1 FROM run_execution_profile_snapshots existing
						WHERE existing.run_id = NEW.run_id
					))
					OR
					(NEW.revision > 1 AND run.status IN ('created', 'paused')
					AND NOT EXISTS (
						SELECT 1 FROM run_execution_leases lease
						WHERE lease.run_id = NEW.run_id AND lease.status = 'active'
							AND julianday(lease.expires_at) > julianday('now')
					) AND EXISTS (
						SELECT 1 FROM run_execution_profile_snapshots previous
						WHERE previous.run_id = NEW.run_id
							AND previous.revision = NEW.revision - 1
							AND previous.protocol_version = NEW.protocol_version
							AND previous.profile != NEW.profile
							AND previous.policy_version = NEW.policy_version
							AND julianday(NEW.created_at) >= julianday(previous.created_at)
					))
				)
		)
		BEGIN
			SELECT RAISE(ABORT, 'Run execution profile binding or transition is invalid');
		END;`,
	`CREATE TRIGGER trg_run_execution_profile_operation_insert
		BEFORE INSERT ON run_execution_profile_operations
		WHEN NOT EXISTS (
			SELECT 1 FROM run_execution_profile_snapshots snapshot
			WHERE snapshot.id = NEW.snapshot_id AND snapshot.run_id = NEW.run_id
				AND snapshot.requested_by = NEW.requested_by
				AND snapshot.created_at = NEW.created_at AND snapshot.revision > 1
		)
		BEGIN
			SELECT RAISE(ABORT, 'Run execution profile operation binding is invalid');
		END;`,
	`CREATE TRIGGER trg_run_execution_profile_snapshot_update_immutable
		BEFORE UPDATE ON run_execution_profile_snapshots BEGIN
			SELECT RAISE(ABORT, 'Run execution profile snapshot cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_execution_profile_snapshot_delete_immutable
		BEFORE DELETE ON run_execution_profile_snapshots BEGIN
			SELECT RAISE(ABORT, 'Run execution profile snapshot cannot be deleted');
		END;`,
	`CREATE TRIGGER trg_run_execution_profile_operation_update_immutable
		BEFORE UPDATE ON run_execution_profile_operations BEGIN
			SELECT RAISE(ABORT, 'Run execution profile operation cannot be updated');
		END;`,
	`CREATE TRIGGER trg_run_execution_profile_operation_delete_immutable
		BEFORE DELETE ON run_execution_profile_operations BEGIN
			SELECT RAISE(ABORT, 'Run execution profile operation cannot be deleted');
		END;`,
}
