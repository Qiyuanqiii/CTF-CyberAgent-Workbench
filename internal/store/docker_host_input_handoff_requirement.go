package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/sandbox"
)

const dockerHostInputHandoffRequirementSelect = `SELECT attempt_id, plan_id, run_id,
	mission_id, workspace_id, protocol_version, operation_key_digest,
	attempt_intent_fingerprint, request_fingerprint, capture_requirement_fingerprint,
	manifest_fingerprint, mount_binding_fingerprint, input_artifact_digest,
	authority_fingerprint, plan_fingerprint, required, operator_confirmed,
	read_only_mount_count, input_artifact_count, requirement_fingerprint,
	requested_by, created_at FROM sandbox_docker_host_input_handoff_requirements`

func insertDockerHostInputHandoffRequirementTx(ctx context.Context, tx *sql.Tx,
	requirement sandbox.DockerHostInputHandoffRequirement,
	intent sandbox.DockerContainerAttemptIntent,
) error {
	if requirement.Validate() != nil || intent.Validate() != nil ||
		requirement.AttemptID != intent.ID || requirement.PlanID != intent.PlanID ||
		requirement.AttemptIntentFingerprint != intent.IntentFingerprint ||
		requirement.OperationKeyDigest != intent.OperationKeyDigest ||
		!requirement.CreatedAt.Equal(intent.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Docker host input handoff requirement binding is invalid")
	}
	capture, found, err := getDockerHostInputRequirementByAttempt(ctx, tx, intent.ID)
	if err != nil {
		return err
	}
	if !found || capture.RequirementFingerprint != requirement.CaptureRequirementFingerprint ||
		(requirement.Required && !capture.Required) {
		return apperror.New(apperror.CodeConflict,
			"Docker host input handoff requirement changed its capture authority")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_docker_host_input_handoff_requirements
		(attempt_id, plan_id, run_id, mission_id, workspace_id, protocol_version,
		operation_key_digest, attempt_intent_fingerprint, request_fingerprint,
		capture_requirement_fingerprint, manifest_fingerprint, mount_binding_fingerprint,
		input_artifact_digest, authority_fingerprint, plan_fingerprint, required,
		operator_confirmed, read_only_mount_count, input_artifact_count,
		requirement_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		requirement.AttemptID, requirement.PlanID, requirement.RunID,
		requirement.MissionID, requirement.WorkspaceID, requirement.ProtocolVersion,
		requirement.OperationKeyDigest, requirement.AttemptIntentFingerprint,
		requirement.RequestFingerprint, requirement.CaptureRequirementFingerprint,
		requirement.ManifestFingerprint, requirement.MountBindingFingerprint,
		requirement.InputArtifactDigest, requirement.AuthorityFingerprint,
		requirement.PlanFingerprint, boolInt(requirement.Required),
		boolInt(requirement.OperatorConfirmed), requirement.ReadOnlyMountCount,
		requirement.InputArtifactCount, requirement.RequirementFingerprint,
		requirement.RequestedBy, ts(requirement.CreatedAt))
	return err
}

func (s *SQLiteStore) GetDockerHostInputHandoffRequirement(ctx context.Context,
	attemptID string,
) (sandbox.DockerHostInputHandoffRequirement, bool, error) {
	attemptID = strings.TrimSpace(attemptID)
	if !domain.ValidAgentID(attemptID) || strings.ContainsRune(attemptID, 0) {
		return sandbox.DockerHostInputHandoffRequirement{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff requirement attempt id is invalid")
	}
	return getDockerHostInputHandoffRequirementByAttempt(ctx, s.db, attemptID)
}

func (s *SQLiteStore) GetDockerHostInputHandoffRequirementByOperation(ctx context.Context,
	operationKeyDigest string,
) (sandbox.DockerHostInputHandoffRequirement, bool, error) {
	operationKeyDigest = strings.TrimSpace(operationKeyDigest)
	if !validStoreDigest(operationKeyDigest) {
		return sandbox.DockerHostInputHandoffRequirement{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input handoff requirement operation digest is invalid")
	}
	return getDockerHostInputHandoffRequirementByOperation(ctx, s.db, operationKeyDigest)
}

func getDockerHostInputHandoffRequirementByAttempt(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) (sandbox.DockerHostInputHandoffRequirement, bool, error) {
	return scanDockerHostInputHandoffRequirementOptional(queryer.QueryRowContext(ctx,
		dockerHostInputHandoffRequirementSelect+` WHERE attempt_id = ?`, attemptID))
}

func getDockerHostInputHandoffRequirementByOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, operationKeyDigest string,
) (sandbox.DockerHostInputHandoffRequirement, bool, error) {
	return scanDockerHostInputHandoffRequirementOptional(queryer.QueryRowContext(ctx,
		dockerHostInputHandoffRequirementSelect+` WHERE operation_key_digest = ?`,
		operationKeyDigest))
}

func scanDockerHostInputHandoffRequirementOptional(row scanner,
) (sandbox.DockerHostInputHandoffRequirement, bool, error) {
	var value sandbox.DockerHostInputHandoffRequirement
	var required, confirmed int
	var createdAt string
	err := row.Scan(&value.AttemptID, &value.PlanID, &value.RunID, &value.MissionID,
		&value.WorkspaceID, &value.ProtocolVersion, &value.OperationKeyDigest,
		&value.AttemptIntentFingerprint, &value.RequestFingerprint,
		&value.CaptureRequirementFingerprint, &value.ManifestFingerprint,
		&value.MountBindingFingerprint, &value.InputArtifactDigest,
		&value.AuthorityFingerprint, &value.PlanFingerprint, &required, &confirmed,
		&value.ReadOnlyMountCount, &value.InputArtifactCount,
		&value.RequirementFingerprint, &value.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputHandoffRequirement{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputHandoffRequirement{}, false, err
	}
	value.Required, value.OperatorConfirmed = required != 0, confirmed != 0
	value.CreatedAt = parseTS(createdAt)
	if err := value.Validate(); err != nil {
		return sandbox.DockerHostInputHandoffRequirement{}, false, err
	}
	return value, true, nil
}
