package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const LatestSchemaVersion = 2

type migration struct {
	Version    int
	Name       string
	Statements []string
}

type appliedMigration struct {
	Name     string
	Checksum string
}

var runCentricSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS missions (
		id TEXT PRIMARY KEY,
		goal TEXT NOT NULL,
		profile TEXT NOT NULL,
		workspace_id TEXT,
		scope_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_missions_updated_at
		ON missions(updated_at);`,
	`CREATE TABLE IF NOT EXISTS runs (
		id TEXT PRIMARY KEY,
		mission_id TEXT NOT NULL,
		session_id TEXT,
		status TEXT NOT NULL,
		config_json TEXT NOT NULL,
		budget_json TEXT NOT NULL,
		started_at TEXT,
		finished_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(mission_id) REFERENCES missions(id)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_runs_mission_created_at
		ON runs(mission_id, created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_runs_status_updated_at
		ON runs(status, updated_at);`,
	`CREATE TABLE IF NOT EXISTS run_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		version TEXT NOT NULL,
		run_id TEXT NOT NULL,
		mission_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		subject_id TEXT,
		payload_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(run_id) REFERENCES runs(id),
		FOREIGN KEY(mission_id) REFERENCES missions(id),
		UNIQUE(run_id, sequence)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_run_events_run_sequence
		ON run_events(run_id, sequence);`,
}

func (s *SQLiteStore) applyMigrations(ctx context.Context, migrations []migration) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite store is not open")
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	);`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := s.loadAppliedMigrations(ctx)
	if err != nil {
		return err
	}
	if err := validateMigrationPlan(migrations, applied); err != nil {
		return err
	}
	for _, item := range migrations {
		if _, ok := applied[item.Version]; ok {
			continue
		}
		if err := s.applyMigration(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) applyMigration(ctx context.Context, item migration) error {
	if item.Version <= 0 || strings.TrimSpace(item.Name) == "" || len(item.Statements) == 0 {
		return fmt.Errorf("invalid migration version=%d name=%q", item.Version, item.Name)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", item.Version, err)
	}
	defer func() { _ = tx.Rollback() }()
	for index, stmt := range item.Statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration %d %q statement %d: %w", item.Version, item.Name, index+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, checksum, applied_at)
		VALUES (?, ?, ?, ?)`, item.Version, item.Name, migrationChecksum(item), ts(time.Now().UTC())); err != nil {
		return fmt.Errorf("record migration %d: %w", item.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", item.Version, err)
	}
	return nil
}

func (s *SQLiteStore) loadAppliedMigrations(ctx context.Context) (map[int]appliedMigration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[int]appliedMigration{}
	for rows.Next() {
		var version int
		var item appliedMigration
		if err := rows.Scan(&version, &item.Name, &item.Checksum); err != nil {
			return nil, err
		}
		applied[version] = item
	}
	return applied, rows.Err()
}

func validateMigrationPlan(migrations []migration, applied map[int]appliedMigration) error {
	if len(migrations) != LatestSchemaVersion {
		return fmt.Errorf("latest schema version is %d but migration plan has %d entries", LatestSchemaVersion, len(migrations))
	}
	known := make(map[int]migration, len(migrations))
	for index, item := range migrations {
		expectedVersion := index + 1
		if item.Version != expectedVersion {
			return fmt.Errorf("migration plan must be contiguous: expected version %d, got %d", expectedVersion, item.Version)
		}
		if _, exists := known[item.Version]; exists {
			return fmt.Errorf("duplicate migration version %d", item.Version)
		}
		known[item.Version] = item
	}
	for version, recorded := range applied {
		item, ok := known[version]
		if !ok {
			return fmt.Errorf("database schema version %d is newer or unknown", version)
		}
		if recorded.Name != item.Name || recorded.Checksum != migrationChecksum(item) {
			return fmt.Errorf("migration %d checksum or name mismatch", version)
		}
	}
	for version := 1; version <= len(migrations); version++ {
		if _, ok := applied[version]; ok {
			continue
		}
		for later := version + 1; later <= len(migrations); later++ {
			if _, ok := applied[later]; ok {
				return fmt.Errorf("migration history has a gap at version %d", version)
			}
		}
		break
	}
	return nil
}

func migrationChecksum(item migration) string {
	sum := sha256.Sum256([]byte(strings.Join(item.Statements, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (s *SQLiteStore) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
