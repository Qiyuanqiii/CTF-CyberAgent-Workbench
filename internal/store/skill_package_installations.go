package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/skills"
)

const skillPackageInstallationSelect = `SELECT id, protocol_version,
	operation_key_digest, request_fingerprint, name, version, surface,
	manifest_protocol, description, profiles_json, tool_dependencies_json,
	content_path, content_sha256, content_bytes, content_token_upper_bound,
	archive_sha256, package_fingerprint, archive_bytes, uncompressed_bytes,
	entry_count, trust_class, risk_codes_json, executable_asset_count,
	install_hook_count, import_command_execution, import_network_access,
	import_provider_calls, tool_capability_grant, run_selection_authorized,
	context_injection_authorized, operator_confirmed, installation_fingerprint,
	installed_by, created_at FROM skill_package_installations`

type skillPackageQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *SQLiteStore) PreparePackageInstallation(ctx context.Context,
	installation skills.PackageInstallation, operation skills.PackageInstallOperation,
) (skills.PackageInstallation, *skills.PackageInstallResult, bool, error) {
	installation = skills.ClonePackageInstallation(installation)
	if err := validatePackageInstallMutation(installation, operation); err != nil {
		return skills.PackageInstallation{}, nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return skills.PackageInstallation{}, nil, false, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return skills.PackageInstallation{}, nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, found, lookupErr := getPackageInstallOperation(ctx, tx,
		operation.KeyDigest); lookupErr != nil {
		return skills.PackageInstallation{}, nil, false, lookupErr
	} else if found {
		if !samePackageInstallOperation(existing, operation) {
			return skills.PackageInstallation{}, nil, false, apperror.New(
				apperror.CodeConflict,
				"Skill package installation operation key was already used for different intent")
		}
		stored, err := getPackageInstallation(ctx, tx, existing.InstallationID)
		if err != nil {
			return skills.PackageInstallation{}, nil, false, err
		}
		if err := validateStoredPackageInstallBinding(existing, stored); err != nil {
			return skills.PackageInstallation{}, nil, false, err
		}
		result, found, err := getPackageInstallResult(ctx, tx, stored.ID)
		if err != nil {
			return skills.PackageInstallation{}, nil, false, err
		}
		if found {
			if err := validatePackageInstallResultBinding(stored, result); err != nil {
				return skills.PackageInstallation{}, nil, false, err
			}
		}
		if err := tx.Commit(); err != nil {
			return skills.PackageInstallation{}, nil, false, err
		}
		if !found {
			return stored, nil, true, nil
		}
		return stored, &result, true, nil
	}
	if existing, found, lookupErr := getPackageInstallationByRef(ctx, tx,
		installation.Name, installation.Version); lookupErr != nil {
		return skills.PackageInstallation{}, nil, false, lookupErr
	} else if found {
		return skills.PackageInstallation{}, nil, false, apperror.New(
			apperror.CodeConflict,
			"Skill package name and version already have an immutable installation: "+
				skills.FormatInstalledPackageRef(existing.Name, existing.Version))
	}
	if installation.CreatedAt.After(time.Now().UTC()) {
		return skills.PackageInstallation{}, nil, false, apperror.New(
			apperror.CodeInvalidArgument, "Skill package installation timestamp is in the future")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO skill_package_install_operations
		(key_digest, request_fingerprint, installation_id, name, version, surface,
		installed_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.InstallationID, operation.Name,
		operation.Version, operation.Surface, operation.InstalledBy,
		ts(operation.CreatedAt)); err != nil {
		return skills.PackageInstallation{}, nil, false, err
	}
	profilesJSON, dependenciesJSON, risksJSON, err := packageInstallationJSON(installation)
	if err != nil {
		return skills.PackageInstallation{}, nil, false, err
	}
	manifest := installation.Manifest
	if _, err := tx.ExecContext(ctx, `INSERT INTO skill_package_installations
		(id, protocol_version, operation_key_digest, request_fingerprint, name, version,
		surface, manifest_protocol, description, profiles_json, tool_dependencies_json,
		content_path, content_sha256, content_bytes, content_token_upper_bound,
		archive_sha256, package_fingerprint, archive_bytes, uncompressed_bytes,
		entry_count, trust_class, risk_codes_json, executable_asset_count,
		install_hook_count, import_command_execution, import_network_access,
		import_provider_calls, tool_capability_grant, run_selection_authorized,
		context_injection_authorized, operator_confirmed, installation_fingerprint,
		installed_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, installation.ID, installation.ProtocolVersion,
		installation.OperationKeyDigest, installation.RequestFingerprint, installation.Name,
		installation.Version, installation.Surface, manifest.Protocol, manifest.Description,
		profilesJSON, dependenciesJSON, manifest.ContentPath, manifest.ContentSHA256,
		manifest.ContentBytes, manifest.ContentTokenUpperBound, installation.ArchiveSHA256,
		installation.PackageFingerprint, installation.ArchiveBytes,
		installation.UncompressedBytes, installation.EntryCount, installation.TrustClass,
		risksJSON, installation.ExecutableAssetCount, installation.InstallHookCount,
		boolInt(installation.ImportCommandExecution), boolInt(installation.ImportNetworkAccess),
		boolInt(installation.ImportProviderCalls), boolInt(installation.ToolCapabilityGrant),
		boolInt(installation.RunSelectionAuthorized),
		boolInt(installation.ContextInjectionAuthorized), boolInt(installation.OperatorConfirmed),
		installation.InstallationFingerprint, installation.InstalledBy,
		ts(installation.CreatedAt)); err != nil {
		return skills.PackageInstallation{}, nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return skills.PackageInstallation{}, nil, false, err
	}
	return skills.ClonePackageInstallation(installation), nil, false, nil
}

func (s *SQLiteStore) CompletePackageInstallation(ctx context.Context,
	result skills.PackageInstallResult,
) (skills.InstalledPackage, bool, error) {
	if err := result.Validate(); err != nil {
		return skills.InstalledPackage{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "Skill package installation result is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return skills.InstalledPackage{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	installation, err := getPackageInstallation(ctx, tx, result.InstallationID)
	if err != nil {
		return skills.InstalledPackage{}, false, err
	}
	if err := validatePackageInstallResultBinding(installation, result); err != nil {
		return skills.InstalledPackage{}, false, err
	}
	if existing, found, lookupErr := getPackageInstallResult(ctx, tx,
		result.InstallationID); lookupErr != nil {
		return skills.InstalledPackage{}, false, lookupErr
	} else if found {
		if existing.InstallationID != result.InstallationID ||
			existing.InstallationFingerprint != result.InstallationFingerprint ||
			existing.ObjectKey != result.ObjectKey ||
			existing.ArchiveSHA256 != result.ArchiveSHA256 ||
			existing.PackageFingerprint != result.PackageFingerprint ||
			existing.ObjectBytes != result.ObjectBytes {
			return skills.InstalledPackage{}, false, apperror.New(
				apperror.CodeConflict, "Skill package installation already has a different result")
		}
		if err := tx.Commit(); err != nil {
			return skills.InstalledPackage{}, false, err
		}
		value := skills.InstalledPackage{Installation: installation, Result: existing, Replayed: true}
		return value, true, value.Validate()
	}
	if result.CompletedAt.After(time.Now().UTC()) || result.CompletedAt.Before(installation.CreatedAt) {
		return skills.InstalledPackage{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Skill package installation completion time is invalid")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO skill_package_install_results
		(installation_id, protocol_version, installation_fingerprint, object_key,
		archive_sha256, package_fingerprint, object_bytes, object_verified,
		run_selection_authorized, context_injection_authorized, tool_capability_grant,
		result_fingerprint, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.InstallationID, result.ProtocolVersion, result.InstallationFingerprint,
		result.ObjectKey, result.ArchiveSHA256, result.PackageFingerprint,
		result.ObjectBytes, boolInt(result.ObjectVerified),
		boolInt(result.RunSelectionAuthorized), boolInt(result.ContextInjectionAuthorized),
		boolInt(result.ToolCapabilityGrant), result.ResultFingerprint,
		ts(result.CompletedAt)); err != nil {
		return skills.InstalledPackage{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return skills.InstalledPackage{}, false, err
	}
	value := skills.InstalledPackage{Installation: installation, Result: result}
	return value, false, value.Validate()
}

func (s *SQLiteStore) GetPackageInstallOperation(ctx context.Context,
	keyDigest string,
) (skills.PackageInstallOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return skills.PackageInstallOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Skill package installation operation digest is invalid")
	}
	return getPackageInstallOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) GetPackageInstallation(ctx context.Context,
	id string,
) (skills.PackageInstallation, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 256 || strings.ContainsRune(id, 0) {
		return skills.PackageInstallation{}, apperror.New(
			apperror.CodeInvalidArgument, "Skill package installation id is invalid")
	}
	return getPackageInstallation(ctx, s.db, id)
}

func (s *SQLiteStore) GetInstalledPackageByRef(ctx context.Context, name, version string,
) (skills.InstalledPackage, bool, error) {
	name, version = strings.TrimSpace(name), strings.TrimSpace(version)
	installation, found, err := getPackageInstallationByRef(ctx, s.db, name, version)
	if err != nil || !found {
		return skills.InstalledPackage{}, false, err
	}
	value, err := getInstalledPackage(ctx, s.db, installation.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return skills.InstalledPackage{}, false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) ListInstalledPackages(ctx context.Context,
	surface domain.ExecutionSurface, profile domain.Profile, includeRemoved bool,
) ([]skills.InstalledPackage, error) {
	if surface != "" && !surface.Valid() {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"installed Skill package surface is invalid")
	}
	if profile != "" {
		parsed, err := domain.ParseProfile(string(profile))
		if err != nil || parsed != profile {
			return nil, apperror.New(apperror.CodeInvalidArgument,
				"installed Skill package Profile is invalid")
		}
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT installation.id FROM skill_package_installations installation
		JOIN skill_package_install_results result ON result.installation_id = installation.id
		LEFT JOIN skill_package_removals removal ON removal.installation_id = installation.id
		WHERE (? = '' OR installation.surface = ?)
			AND (? = '' OR EXISTS (SELECT 1 FROM json_each(installation.profiles_json)
				WHERE value = ?))
			AND (? = 1 OR removal.id IS NULL)
		ORDER BY installation.surface, installation.name, installation.version`
	rows, err := tx.QueryContext(ctx, query, surface, surface, profile, profile, boolInt(includeRemoved))
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
		if len(ids) > skills.MaxInstalledPackageIdentities {
			_ = rows.Close()
			return nil, apperror.New(apperror.CodeInternal,
				"installed Skill package Registry exceeds its bound")
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	values := make([]skills.InstalledPackage, 0, len(ids))
	for _, id := range ids {
		value, err := getInstalledPackage(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		values = append(values, skills.CloneInstalledPackage(value))
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *SQLiteStore) CreatePackageRemoval(ctx context.Context,
	removal skills.PackageRemoval, operation skills.PackageRemoveOperation,
) (skills.PackageRemoval, bool, error) {
	if err := validatePackageRemovalMutation(removal, operation); err != nil {
		return skills.PackageRemoval{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return skills.PackageRemoval{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, found, lookupErr := getPackageRemoveOperation(ctx, tx,
		operation.KeyDigest); lookupErr != nil {
		return skills.PackageRemoval{}, false, lookupErr
	} else if found {
		if !samePackageRemoveOperation(existing, operation) {
			return skills.PackageRemoval{}, false, apperror.New(
				apperror.CodeConflict,
				"Skill package removal operation key was already used for different intent")
		}
		stored, err := getPackageRemoval(ctx, tx, existing.RemovalID)
		if err != nil {
			return skills.PackageRemoval{}, false, err
		}
		if err := validateStoredPackageRemovalBinding(existing, stored); err != nil {
			return skills.PackageRemoval{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return skills.PackageRemoval{}, false, err
		}
		return stored, true, nil
	}
	installed, err := getInstalledPackage(ctx, tx, removal.InstallationID)
	if err != nil {
		return skills.PackageRemoval{}, false, err
	}
	if installed.Removal != nil {
		return skills.PackageRemoval{}, false, apperror.New(
			apperror.CodeConflict, "Skill package already has an immutable removal tombstone")
	}
	if err := validatePackageRemovalInstallationBinding(installed, removal); err != nil {
		return skills.PackageRemoval{}, false, err
	}
	var pinned int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM run_skill_selection_items
		WHERE name = ? AND version = ? AND content_sha256 = ?)
		OR EXISTS (
			SELECT 1 FROM run_external_skill_selection_items
			WHERE installation_id = ? AND installation_fingerprint = ?)`, removal.Name,
		removal.Version, removal.ContentSHA256, removal.InstallationID,
		removal.InstallationFingerprint).Scan(&pinned); err != nil {
		return skills.PackageRemoval{}, false, err
	}
	if pinned != 0 {
		return skills.PackageRemoval{}, false, apperror.New(
			apperror.CodeFailedPrecondition, "Skill package version is pinned by a Run")
	}
	if removal.CreatedAt.After(time.Now().UTC()) ||
		removal.CreatedAt.Before(installed.Result.CompletedAt) {
		return skills.PackageRemoval{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Skill package removal timestamp is invalid")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO skill_package_remove_operations
		(key_digest, request_fingerprint, removal_id, installation_id, name, version,
		surface, removed_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.KeyDigest, operation.RequestFingerprint, operation.RemovalID,
		operation.InstallationID, operation.Name, operation.Version, operation.Surface,
		operation.RemovedBy, ts(operation.CreatedAt)); err != nil {
		return skills.PackageRemoval{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO skill_package_removals
		(id, protocol_version, installation_id, installation_fingerprint, name, version,
		surface, content_sha256, archive_sha256, package_fingerprint,
		operation_key_digest, request_fingerprint, package_object_retained,
		historical_recovery_preserved, future_selection_enabled,
		run_selection_authorized, context_injection_authorized, tool_capability_grant,
		removal_fingerprint, removed_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		removal.ID, removal.ProtocolVersion, removal.InstallationID,
		removal.InstallationFingerprint, removal.Name, removal.Version, removal.Surface,
		removal.ContentSHA256, removal.ArchiveSHA256, removal.PackageFingerprint,
		removal.OperationKeyDigest, removal.RequestFingerprint,
		boolInt(removal.PackageObjectRetained), boolInt(removal.HistoricalRecoveryPreserved),
		boolInt(removal.FutureSelectionEnabled), boolInt(removal.RunSelectionAuthorized),
		boolInt(removal.ContextInjectionAuthorized), boolInt(removal.ToolCapabilityGrant),
		removal.RemovalFingerprint, removal.RemovedBy, ts(removal.CreatedAt)); err != nil {
		return skills.PackageRemoval{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return skills.PackageRemoval{}, false, err
	}
	return removal, false, nil
}

func (s *SQLiteStore) GetPackageRemoveOperation(ctx context.Context,
	keyDigest string,
) (skills.PackageRemoveOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return skills.PackageRemoveOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Skill package removal operation digest is invalid")
	}
	return getPackageRemoveOperation(ctx, s.db, keyDigest)
}

func getPackageInstallOperation(ctx context.Context, queryer skillPackageQueryer,
	keyDigest string,
) (skills.PackageInstallOperation, bool, error) {
	var value skills.PackageInstallOperation
	var created string
	err := queryer.QueryRowContext(ctx, `SELECT key_digest, request_fingerprint,
		installation_id, name, version, surface, installed_by, created_at
		FROM skill_package_install_operations WHERE key_digest = ?`, keyDigest).Scan(
		&value.KeyDigest, &value.RequestFingerprint, &value.InstallationID, &value.Name,
		&value.Version, &value.Surface, &value.InstalledBy, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return skills.PackageInstallOperation{}, false, nil
	}
	if err != nil {
		return skills.PackageInstallOperation{}, false, err
	}
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return skills.PackageInstallOperation{}, false, apperror.Wrap(
			apperror.CodeInternal, "stored Skill package installation operation is invalid", err)
	}
	return value, true, nil
}

func getPackageInstallation(ctx context.Context, queryer skillPackageQueryer,
	id string,
) (skills.PackageInstallation, error) {
	return scanPackageInstallation(queryer.QueryRowContext(ctx,
		skillPackageInstallationSelect+` WHERE id = ?`, id))
}

func getPackageInstallationByRef(ctx context.Context, queryer skillPackageQueryer,
	name, version string,
) (skills.PackageInstallation, bool, error) {
	value, err := scanPackageInstallation(queryer.QueryRowContext(ctx,
		skillPackageInstallationSelect+` WHERE name = ? AND version = ?`, name, version))
	if errors.Is(err, sql.ErrNoRows) {
		return skills.PackageInstallation{}, false, nil
	}
	return value, err == nil, err
}

type packageRowScanner interface {
	Scan(...any) error
}

func scanPackageInstallation(row packageRowScanner) (skills.PackageInstallation, error) {
	var value skills.PackageInstallation
	var profilesJSON, dependenciesJSON, risksJSON, created string
	var commandExecution, networkAccess, providerCalls, toolGrant, runSelection int
	var contextInjection, confirmed int
	err := row.Scan(&value.ID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.RequestFingerprint, &value.Name, &value.Version, &value.Surface,
		&value.Manifest.Protocol, &value.Manifest.Description, &profilesJSON,
		&dependenciesJSON, &value.Manifest.ContentPath, &value.Manifest.ContentSHA256,
		&value.Manifest.ContentBytes, &value.Manifest.ContentTokenUpperBound,
		&value.ArchiveSHA256, &value.PackageFingerprint, &value.ArchiveBytes,
		&value.UncompressedBytes, &value.EntryCount, &value.TrustClass, &risksJSON,
		&value.ExecutableAssetCount, &value.InstallHookCount, &commandExecution,
		&networkAccess, &providerCalls, &toolGrant, &runSelection, &contextInjection,
		&confirmed, &value.InstallationFingerprint, &value.InstalledBy, &created)
	if err != nil {
		return skills.PackageInstallation{}, err
	}
	if !storeBools(commandExecution, networkAccess, providerCalls, toolGrant,
		runSelection, contextInjection, confirmed) {
		return skills.PackageInstallation{}, apperror.New(
			apperror.CodeInternal, "stored Skill package installation booleans are invalid")
	}
	if err := decodeCanonicalPackageJSON(profilesJSON, &value.Manifest.Profiles, true); err != nil {
		return skills.PackageInstallation{}, err
	}
	if err := decodeCanonicalPackageJSON(dependenciesJSON,
		&value.Manifest.ToolDependencies, true); err != nil {
		return skills.PackageInstallation{}, err
	}
	if err := decodeCanonicalPackageJSON(risksJSON, &value.RiskCodes, false); err != nil {
		return skills.PackageInstallation{}, err
	}
	value.Manifest.Name = value.Name
	value.Manifest.Version = value.Version
	value.ImportCommandExecution = commandExecution == 1
	value.ImportNetworkAccess = networkAccess == 1
	value.ImportProviderCalls = providerCalls == 1
	value.ToolCapabilityGrant = toolGrant == 1
	value.RunSelectionAuthorized = runSelection == 1
	value.ContextInjectionAuthorized = contextInjection == 1
	value.OperatorConfirmed = confirmed == 1
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return skills.PackageInstallation{}, apperror.Wrap(
			apperror.CodeInternal, "stored Skill package installation is invalid", err)
	}
	return skills.ClonePackageInstallation(value), nil
}

func getPackageInstallResult(ctx context.Context, queryer skillPackageQueryer,
	installationID string,
) (skills.PackageInstallResult, bool, error) {
	var value skills.PackageInstallResult
	var verified, runSelection, contextInjection, toolGrant int
	var completed string
	err := queryer.QueryRowContext(ctx, `SELECT protocol_version, installation_id,
		installation_fingerprint, object_key, archive_sha256, package_fingerprint,
		object_bytes, object_verified, run_selection_authorized,
		context_injection_authorized, tool_capability_grant, result_fingerprint,
		completed_at FROM skill_package_install_results WHERE installation_id = ?`,
		installationID).Scan(&value.ProtocolVersion, &value.InstallationID,
		&value.InstallationFingerprint, &value.ObjectKey, &value.ArchiveSHA256,
		&value.PackageFingerprint, &value.ObjectBytes, &verified, &runSelection,
		&contextInjection, &toolGrant, &value.ResultFingerprint, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return skills.PackageInstallResult{}, false, nil
	}
	if err != nil {
		return skills.PackageInstallResult{}, false, err
	}
	if !storeBools(verified, runSelection, contextInjection, toolGrant) {
		return skills.PackageInstallResult{}, false, apperror.New(
			apperror.CodeInternal, "stored Skill package installation result booleans are invalid")
	}
	value.ObjectVerified = verified == 1
	value.RunSelectionAuthorized = runSelection == 1
	value.ContextInjectionAuthorized = contextInjection == 1
	value.ToolCapabilityGrant = toolGrant == 1
	value.CompletedAt = parseTS(completed)
	if err := value.Validate(); err != nil {
		return skills.PackageInstallResult{}, false, apperror.Wrap(
			apperror.CodeInternal, "stored Skill package installation result is invalid", err)
	}
	return value, true, nil
}

func getPackageRemoveOperation(ctx context.Context, queryer skillPackageQueryer,
	keyDigest string,
) (skills.PackageRemoveOperation, bool, error) {
	var value skills.PackageRemoveOperation
	var created string
	err := queryer.QueryRowContext(ctx, `SELECT key_digest, request_fingerprint,
		removal_id, installation_id, name, version, surface, removed_by, created_at
		FROM skill_package_remove_operations WHERE key_digest = ?`, keyDigest).Scan(
		&value.KeyDigest, &value.RequestFingerprint, &value.RemovalID,
		&value.InstallationID, &value.Name, &value.Version, &value.Surface,
		&value.RemovedBy, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return skills.PackageRemoveOperation{}, false, nil
	}
	if err != nil {
		return skills.PackageRemoveOperation{}, false, err
	}
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return skills.PackageRemoveOperation{}, false, apperror.Wrap(
			apperror.CodeInternal, "stored Skill package removal operation is invalid", err)
	}
	return value, true, nil
}

func getPackageRemoval(ctx context.Context, queryer skillPackageQueryer,
	id string,
) (skills.PackageRemoval, error) {
	var value skills.PackageRemoval
	var retained, recovery, future, runSelection, contextInjection, toolGrant int
	var created string
	err := queryer.QueryRowContext(ctx, `SELECT id, protocol_version, installation_id,
		installation_fingerprint, name, version, surface, content_sha256,
		archive_sha256, package_fingerprint, operation_key_digest,
		request_fingerprint, package_object_retained, historical_recovery_preserved,
		future_selection_enabled, run_selection_authorized,
		context_injection_authorized, tool_capability_grant, removal_fingerprint,
		removed_by, created_at FROM skill_package_removals WHERE id = ?`, id).Scan(
		&value.ID, &value.ProtocolVersion, &value.InstallationID,
		&value.InstallationFingerprint, &value.Name, &value.Version, &value.Surface,
		&value.ContentSHA256, &value.ArchiveSHA256, &value.PackageFingerprint,
		&value.OperationKeyDigest, &value.RequestFingerprint, &retained, &recovery,
		&future, &runSelection, &contextInjection, &toolGrant, &value.RemovalFingerprint,
		&value.RemovedBy, &created)
	if err != nil {
		return skills.PackageRemoval{}, err
	}
	if !storeBools(retained, recovery, future, runSelection, contextInjection, toolGrant) {
		return skills.PackageRemoval{}, apperror.New(
			apperror.CodeInternal, "stored Skill package removal booleans are invalid")
	}
	value.PackageObjectRetained = retained == 1
	value.HistoricalRecoveryPreserved = recovery == 1
	value.FutureSelectionEnabled = future == 1
	value.RunSelectionAuthorized = runSelection == 1
	value.ContextInjectionAuthorized = contextInjection == 1
	value.ToolCapabilityGrant = toolGrant == 1
	value.CreatedAt = parseTS(created)
	if err := value.Validate(); err != nil {
		return skills.PackageRemoval{}, apperror.Wrap(
			apperror.CodeInternal, "stored Skill package removal is invalid", err)
	}
	return value, nil
}

func getPackageRemovalByInstallation(ctx context.Context, queryer skillPackageQueryer,
	installationID string,
) (skills.PackageRemoval, bool, error) {
	var id string
	err := queryer.QueryRowContext(ctx, `SELECT id FROM skill_package_removals
		WHERE installation_id = ?`, installationID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return skills.PackageRemoval{}, false, nil
	}
	if err != nil {
		return skills.PackageRemoval{}, false, err
	}
	value, err := getPackageRemoval(ctx, queryer, id)
	return value, err == nil, err
}

func getInstalledPackage(ctx context.Context, queryer skillPackageQueryer,
	installationID string,
) (skills.InstalledPackage, error) {
	installation, err := getPackageInstallation(ctx, queryer, installationID)
	if err != nil {
		return skills.InstalledPackage{}, err
	}
	result, found, err := getPackageInstallResult(ctx, queryer, installationID)
	if err != nil {
		return skills.InstalledPackage{}, err
	}
	if !found {
		return skills.InstalledPackage{}, sql.ErrNoRows
	}
	removal, removed, err := getPackageRemovalByInstallation(ctx, queryer, installationID)
	if err != nil {
		return skills.InstalledPackage{}, err
	}
	value := skills.InstalledPackage{Installation: installation, Result: result}
	if removed {
		value.Removal = &removal
	}
	if err := value.Validate(); err != nil {
		return skills.InstalledPackage{}, apperror.Wrap(
			apperror.CodeInternal, "stored installed Skill package is invalid", err)
	}
	return value, nil
}

func validatePackageInstallMutation(installation skills.PackageInstallation,
	operation skills.PackageInstallOperation,
) error {
	if err := installation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Skill package installation is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Skill package installation operation is invalid", err)
	}
	if installation.OperationKeyDigest != operation.KeyDigest ||
		installation.RequestFingerprint != operation.RequestFingerprint ||
		installation.ID != operation.InstallationID || installation.Name != operation.Name ||
		installation.Version != operation.Version || installation.Surface != operation.Surface ||
		installation.InstalledBy != operation.InstalledBy ||
		!installation.CreatedAt.Equal(operation.CreatedAt) {
		return apperror.New(apperror.CodeConflict,
			"Skill package installation operation does not match its intent")
	}
	return nil
}

func validatePackageInstallResultBinding(installation skills.PackageInstallation,
	result skills.PackageInstallResult,
) error {
	if result.InstallationID != installation.ID ||
		result.InstallationFingerprint != installation.InstallationFingerprint ||
		result.ArchiveSHA256 != installation.ArchiveSHA256 ||
		result.PackageFingerprint != installation.PackageFingerprint ||
		result.ObjectBytes != installation.ArchiveBytes {
		return apperror.New(apperror.CodeConflict,
			"Skill package installation result does not match its intent")
	}
	return nil
}

func validatePackageRemovalMutation(removal skills.PackageRemoval,
	operation skills.PackageRemoveOperation,
) error {
	if err := removal.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "Skill package removal is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Skill package removal operation is invalid", err)
	}
	if removal.OperationKeyDigest != operation.KeyDigest ||
		removal.RequestFingerprint != operation.RequestFingerprint ||
		removal.ID != operation.RemovalID || removal.InstallationID != operation.InstallationID ||
		removal.Name != operation.Name || removal.Version != operation.Version ||
		removal.Surface != operation.Surface || removal.RemovedBy != operation.RemovedBy ||
		!removal.CreatedAt.Equal(operation.CreatedAt) {
		return apperror.New(apperror.CodeConflict,
			"Skill package removal operation does not match its tombstone")
	}
	return nil
}

func validatePackageRemovalInstallationBinding(installed skills.InstalledPackage,
	removal skills.PackageRemoval,
) error {
	copy := installed
	copy.Removal = &removal
	if err := copy.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeConflict,
			"Skill package removal does not match its installation", err)
	}
	return nil
}

func validateStoredPackageInstallBinding(operation skills.PackageInstallOperation,
	installation skills.PackageInstallation,
) error {
	if installation.OperationKeyDigest != operation.KeyDigest ||
		installation.RequestFingerprint != operation.RequestFingerprint ||
		installation.ID != operation.InstallationID || installation.Name != operation.Name ||
		installation.Version != operation.Version || installation.Surface != operation.Surface ||
		installation.InstalledBy != operation.InstalledBy ||
		!installation.CreatedAt.Equal(operation.CreatedAt) {
		return apperror.New(apperror.CodeInternal,
			"stored Skill package installation operation binding is invalid")
	}
	return nil
}

func validateStoredPackageRemovalBinding(operation skills.PackageRemoveOperation,
	removal skills.PackageRemoval,
) error {
	if removal.OperationKeyDigest != operation.KeyDigest ||
		removal.RequestFingerprint != operation.RequestFingerprint ||
		removal.ID != operation.RemovalID || removal.InstallationID != operation.InstallationID ||
		removal.Name != operation.Name || removal.Version != operation.Version ||
		removal.Surface != operation.Surface || removal.RemovedBy != operation.RemovedBy ||
		!removal.CreatedAt.Equal(operation.CreatedAt) {
		return apperror.New(apperror.CodeInternal,
			"stored Skill package removal operation binding is invalid")
	}
	return nil
}

func samePackageInstallOperation(left, right skills.PackageInstallOperation) bool {
	return left.KeyDigest == right.KeyDigest &&
		left.RequestFingerprint == right.RequestFingerprint &&
		left.Name == right.Name && left.Version == right.Version &&
		left.Surface == right.Surface && left.InstalledBy == right.InstalledBy
}

func samePackageRemoveOperation(left, right skills.PackageRemoveOperation) bool {
	return left.KeyDigest == right.KeyDigest &&
		left.RequestFingerprint == right.RequestFingerprint &&
		left.InstallationID == right.InstallationID && left.Name == right.Name &&
		left.Version == right.Version && left.Surface == right.Surface &&
		left.RemovedBy == right.RemovedBy
}

func packageInstallationJSON(value skills.PackageInstallation) (string, string, string, error) {
	profiles, err := json.Marshal(value.Manifest.Profiles)
	if err != nil {
		return "", "", "", err
	}
	dependencies, err := json.Marshal(value.Manifest.ToolDependencies)
	if err != nil {
		return "", "", "", err
	}
	risks, err := json.Marshal(value.RiskCodes)
	if err != nil {
		return "", "", "", err
	}
	return string(profiles), string(dependencies), string(risks), nil
}

func decodeCanonicalPackageJSON[T ~string](raw string, target *[]T, requireSorted bool) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		return apperror.New(apperror.CodeInternal,
			"stored Skill package metadata JSON is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apperror.New(apperror.CodeInternal,
			"stored Skill package metadata JSON has trailing data")
	}
	canonical, err := json.Marshal(*target)
	if err != nil || string(canonical) != raw {
		return apperror.New(apperror.CodeInternal,
			"stored Skill package metadata JSON is not canonical")
	}
	if requireSorted && len(*target) > 1 {
		for index := 1; index < len(*target); index++ {
			if string((*target)[index-1]) >= string((*target)[index]) {
				return apperror.New(apperror.CodeInternal,
					"stored Skill package metadata is not sorted and unique")
			}
		}
	}
	return nil
}

func storeBools(values ...int) bool {
	return !slices.ContainsFunc(values, func(value int) bool { return value != 0 && value != 1 })
}
