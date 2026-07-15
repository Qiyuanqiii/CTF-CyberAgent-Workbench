package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/sandbox"
)

const dockerRuntimeInputResourceInspectionSelect = `SELECT id, application_intent_id,
	application_result_id, projection_id, container_plan_id, run_id, protocol_version,
	operation_key_digest, manifest_fingerprint, descriptor_fingerprint, request_fingerprint,
	application_result_fingerprint, endpoint_class, endpoint_fingerprint, status, trust_class,
	target_state, projection_count, owned_volume_count, absent_volume_count,
	foreign_volume_count, foreign_resource_count, daemon_read_count, complete,
	cleanup_eligible, owned_target_never_started, all_owned_volumes_read_only,
	all_owned_volumes_no_copy, container_start_authorized, process_execution_authorized,
	output_export_authorized, artifact_commit_authorized, request_semantic_fingerprint,
	inspection_fingerprint, requested_by, created_at
	FROM sandbox_docker_runtime_input_resource_inspections`

func (s *SQLiteStore) RecordDockerRuntimeInputResourceInspection(ctx context.Context,
	value sandbox.DockerRuntimeInputResourceInspection,
) (sandbox.DockerRuntimeInputResourceInspection, bool, error) {
	if err := value.Validate(); err != nil || value.Replayed {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, apperror.Wrap(
			apperror.CodeInvalidArgument, "Docker runtime input resource inspection is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSandboxManifestWriteLock(ctx, tx, value.RunID); err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, err
	}
	if existing, found, lookupErr := getDockerRuntimeInputResourceInspectionByOperation(
		ctx, tx, value.OperationKeyDigest); lookupErr != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, lookupErr
	} else if found {
		if existing.RequestSemanticFingerprint != value.RequestSemanticFingerprint ||
			existing.ApplicationIntentID != value.ApplicationIntentID ||
			existing.RequestedBy != value.RequestedBy {
			return sandbox.DockerRuntimeInputResourceInspection{}, false, apperror.New(
				apperror.CodeConflict, "Docker runtime input resource inspection operation key changed request")
		}
		if err := tx.Commit(); err != nil {
			return sandbox.DockerRuntimeInputResourceInspection{}, false, err
		}
		existing.Replayed = true
		return existing, true, nil
	}
	application, err := validateDockerRuntimeInputResourceInspectionCurrentTx(ctx, tx, value)
	if err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, err
	}
	if value.CreatedAt.After(time.Now().UTC()) {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource inspection timestamp is in the future")
	}
	if err := insertDockerRuntimeInputResourceInspectionTx(ctx, tx, value); err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, err
	}
	if err := appendDockerRuntimeInputResourceEvent(ctx, tx, value.RunID,
		application.Intent.MissionID, value.ID,
		events.SandboxDockerRuntimeInputResourceInspectedEvent, value.CreatedAt,
		map[string]any{"status": value.Status, "target_state": value.TargetState,
			"projection_count":       value.ProjectionCount,
			"foreign_resource_count": value.ForeignResourceCount,
			"cleanup_eligible":       value.CleanupEligible, "container_started": false,
			"process_executed": false, "execution_authorized": false}); err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, err
	}
	return value, false, nil
}

func validateDockerRuntimeInputResourceInspectionCurrentTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerRuntimeInputResourceInspection,
) (sandbox.DockerRuntimeInputApplicationRecord, error) {
	application, err := getDockerRuntimeInputApplication(ctx, tx, value.ApplicationIntentID)
	if err != nil {
		return sandbox.DockerRuntimeInputApplicationRecord{}, err
	}
	if application.Result == nil || application.Replayed ||
		application.Result.ID != value.ApplicationResultID ||
		application.Intent.ProjectionID != value.ProjectionID ||
		application.Intent.ContainerPlanID != value.ContainerPlanID ||
		application.Intent.RunID != value.RunID ||
		application.Intent.ManifestFingerprint != value.ManifestFingerprint ||
		application.Intent.EndpointClass != value.EndpointClass ||
		application.Intent.EndpointFingerprint != value.EndpointFingerprint ||
		application.Intent.ProjectionCount != value.ProjectionCount ||
		application.Intent.RequestedBy != value.RequestedBy ||
		application.Result.RequestFingerprint != value.RequestFingerprint ||
		application.Result.ResultFingerprint != value.ApplicationResultFingerprint ||
		value.CreatedAt.Before(application.Result.CreatedAt) ||
		!application.Result.TargetContainerPresent || application.Result.ContainerStarted ||
		application.Result.ProcessExecuted || application.Result.OutputExported ||
		application.Result.ProductionExecutionSubmitted || application.Result.ProductionVerified ||
		application.Result.BackendEnabled || application.Result.ExecutionAuthorized ||
		application.Result.ArtifactCommitAuthorized {
		return sandbox.DockerRuntimeInputApplicationRecord{}, apperror.New(
			apperror.CodeConflict, "Docker runtime input resource inspection v61 authority changed")
	}
	return application, nil
}

func insertDockerRuntimeInputResourceInspectionTx(ctx context.Context, tx *sql.Tx,
	value sandbox.DockerRuntimeInputResourceInspection,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_runtime_input_resource_inspections
		(id, application_intent_id, application_result_id, projection_id, container_plan_id,
		run_id, protocol_version, operation_key_digest, manifest_fingerprint,
		descriptor_fingerprint, request_fingerprint, application_result_fingerprint,
		endpoint_class, endpoint_fingerprint, status, trust_class, target_state,
		projection_count, owned_volume_count, absent_volume_count, foreign_volume_count,
		foreign_resource_count, daemon_read_count, complete, cleanup_eligible,
		owned_target_never_started, all_owned_volumes_read_only, all_owned_volumes_no_copy,
		container_start_authorized, process_execution_authorized, output_export_authorized,
		artifact_commit_authorized, request_semantic_fingerprint, inspection_fingerprint,
		requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.ApplicationIntentID,
		value.ApplicationResultID, value.ProjectionID, value.ContainerPlanID, value.RunID,
		value.ProtocolVersion, value.OperationKeyDigest, value.ManifestFingerprint,
		value.DescriptorFingerprint, value.RequestFingerprint,
		value.ApplicationResultFingerprint, value.EndpointClass, value.EndpointFingerprint,
		value.Status, value.TrustClass, value.TargetState, value.ProjectionCount,
		value.OwnedVolumeCount, value.AbsentVolumeCount, value.ForeignVolumeCount,
		value.ForeignResourceCount, value.DaemonReadCount, boolInt(value.Complete),
		boolInt(value.CleanupEligible), boolInt(value.OwnedTargetNeverStarted),
		boolInt(value.AllOwnedVolumesReadOnly), boolInt(value.AllOwnedVolumesNoCopy),
		boolInt(value.ContainerStartAuthorized), boolInt(value.ProcessExecutionAuthorized),
		boolInt(value.OutputExportAuthorized), boolInt(value.ArtifactCommitAuthorized),
		value.RequestSemanticFingerprint, value.InspectionFingerprint, value.RequestedBy,
		ts(value.CreatedAt))
	return err
}

func (s *SQLiteStore) GetDockerRuntimeInputResourceInspection(ctx context.Context,
	id string,
) (sandbox.DockerRuntimeInputResourceInspection, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.New(
			apperror.CodeInvalidArgument, "Docker runtime input resource inspection id is invalid")
	}
	return getDockerRuntimeInputResourceInspection(ctx, s.db, id)
}

func (s *SQLiteStore) GetDockerRuntimeInputResourceInspectionByOperation(ctx context.Context,
	keyDigest string,
) (sandbox.DockerRuntimeInputResourceInspection, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Docker runtime input resource inspection operation digest is invalid")
	}
	return getDockerRuntimeInputResourceInspectionByOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) ListDockerRuntimeInputResourceInspections(ctx context.Context,
	runID string, limit int,
) ([]sandbox.DockerRuntimeInputResourceInspection, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker runtime input resource inspection list Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Docker runtime input resource inspection list limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id
		FROM sandbox_docker_runtime_input_resource_inspections
		WHERE run_id = ? ORDER BY created_at, id LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	values := make([]sandbox.DockerRuntimeInputResourceInspection, 0, len(ids))
	for _, id := range ids {
		value, err := getDockerRuntimeInputResourceInspection(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func getDockerRuntimeInputResourceInspection(ctx context.Context,
	queryer sandboxLifecycleQueryer, id string,
) (sandbox.DockerRuntimeInputResourceInspection, error) {
	value, err := scanDockerRuntimeInputResourceInspection(queryer.QueryRowContext(ctx,
		dockerRuntimeInputResourceInspectionSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputResourceInspection{}, apperror.New(
			apperror.CodeNotFound, "Docker runtime input resource inspection not found")
	}
	return value, err
}

func getDockerRuntimeInputResourceInspectionByOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, keyDigest string,
) (sandbox.DockerRuntimeInputResourceInspection, bool, error) {
	value, err := scanDockerRuntimeInputResourceInspection(queryer.QueryRowContext(ctx,
		dockerRuntimeInputResourceInspectionSelect+` WHERE operation_key_digest = ?`, keyDigest))
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerRuntimeInputResourceInspection{}, false, nil
	}
	return value, err == nil, err
}

func scanDockerRuntimeInputResourceInspection(row scanner) (
	sandbox.DockerRuntimeInputResourceInspection, error,
) {
	var value sandbox.DockerRuntimeInputResourceInspection
	var complete, cleanupEligible, targetNeverStarted, volumesReadOnly, volumesNoCopy int
	var startAuthorized, processAuthorized, outputAuthorized, artifactAuthorized int
	var createdAt string
	if err := row.Scan(&value.ID, &value.ApplicationIntentID, &value.ApplicationResultID,
		&value.ProjectionID, &value.ContainerPlanID, &value.RunID, &value.ProtocolVersion,
		&value.OperationKeyDigest, &value.ManifestFingerprint, &value.DescriptorFingerprint,
		&value.RequestFingerprint, &value.ApplicationResultFingerprint,
		&value.EndpointClass, &value.EndpointFingerprint, &value.Status, &value.TrustClass,
		&value.TargetState, &value.ProjectionCount, &value.OwnedVolumeCount,
		&value.AbsentVolumeCount, &value.ForeignVolumeCount, &value.ForeignResourceCount,
		&value.DaemonReadCount, &complete, &cleanupEligible, &targetNeverStarted,
		&volumesReadOnly, &volumesNoCopy, &startAuthorized, &processAuthorized,
		&outputAuthorized, &artifactAuthorized, &value.RequestSemanticFingerprint,
		&value.InspectionFingerprint, &value.RequestedBy, &createdAt); err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, err
	}
	value.Complete, value.CleanupEligible = complete != 0, cleanupEligible != 0
	value.OwnedTargetNeverStarted = targetNeverStarted != 0
	value.AllOwnedVolumesReadOnly, value.AllOwnedVolumesNoCopy = volumesReadOnly != 0,
		volumesNoCopy != 0
	value.ContainerStartAuthorized, value.ProcessExecutionAuthorized = startAuthorized != 0,
		processAuthorized != 0
	value.OutputExportAuthorized, value.ArtifactCommitAuthorized = outputAuthorized != 0,
		artifactAuthorized != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerRuntimeInputResourceInspection{}, err
	}
	return value, nil
}

func appendDockerRuntimeInputResourceEvent(ctx context.Context, tx *sql.Tx,
	runID, missionID, subjectID, eventType string, createdAt time.Time, payload map[string]any,
) error {
	event, err := events.New(runID, missionID, eventType,
		"sandbox_docker_runtime_input_resources", subjectID, payload)
	if err != nil {
		return err
	}
	event.CreatedAt = createdAt
	_, err = insertRunEventTx(ctx, tx, event)
	return err
}
