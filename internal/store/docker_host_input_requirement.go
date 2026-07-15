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

const dockerHostInputRequirementSelect = `SELECT attempt_id, plan_id, run_id, mission_id,
	workspace_id, protocol_version, operation_key_digest, attempt_intent_fingerprint,
	request_fingerprint, manifest_fingerprint, mount_binding_fingerprint,
	input_artifact_digest, authority_fingerprint, plan_fingerprint, required,
	operator_confirmed, read_only_mount_count, input_artifact_count,
	requirement_fingerprint, requested_by, created_at
	FROM sandbox_docker_host_input_requirements`

func insertDockerHostInputRequirementTx(ctx context.Context, tx *sql.Tx,
	requirement sandbox.DockerHostInputRequirement,
	intent sandbox.DockerContainerAttemptIntent,
) error {
	if requirement.Validate() != nil || intent.Validate() != nil ||
		requirement.AttemptID != intent.ID || requirement.PlanID != intent.PlanID ||
		requirement.AttemptIntentFingerprint != intent.IntentFingerprint ||
		requirement.OperationKeyDigest != intent.OperationKeyDigest ||
		!requirement.CreatedAt.Equal(intent.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Docker host input requirement binding is invalid")
	}
	plan, err := getDockerContainerPlan(ctx, tx, intent.PlanID)
	if err != nil {
		return err
	}
	if plan.Validate() != nil || plan.RunID != requirement.RunID ||
		plan.MissionID != requirement.MissionID || plan.WorkspaceID != requirement.WorkspaceID ||
		plan.ManifestFingerprint != requirement.ManifestFingerprint ||
		plan.MountBindingFingerprint != requirement.MountBindingFingerprint ||
		plan.InputArtifactDigest != requirement.InputArtifactDigest ||
		plan.AuthorityFingerprint != requirement.AuthorityFingerprint ||
		plan.PlanFingerprint != requirement.PlanFingerprint ||
		plan.ReadOnlyMountCount != requirement.ReadOnlyMountCount ||
		plan.InputArtifactCount != requirement.InputArtifactCount ||
		plan.RequestedBy != requirement.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Docker host input requirement authority changed")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_docker_host_input_requirements
		(attempt_id, plan_id, run_id, mission_id, workspace_id, protocol_version,
		operation_key_digest, attempt_intent_fingerprint, request_fingerprint,
		manifest_fingerprint, mount_binding_fingerprint, input_artifact_digest,
		authority_fingerprint, plan_fingerprint, required, operator_confirmed,
		read_only_mount_count, input_artifact_count, requirement_fingerprint,
		requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		requirement.AttemptID, requirement.PlanID, requirement.RunID,
		requirement.MissionID, requirement.WorkspaceID, requirement.ProtocolVersion,
		requirement.OperationKeyDigest, requirement.AttemptIntentFingerprint,
		requirement.RequestFingerprint, requirement.ManifestFingerprint,
		requirement.MountBindingFingerprint, requirement.InputArtifactDigest,
		requirement.AuthorityFingerprint, requirement.PlanFingerprint,
		boolInt(requirement.Required), boolInt(requirement.OperatorConfirmed),
		requirement.ReadOnlyMountCount, requirement.InputArtifactCount,
		requirement.RequirementFingerprint, requirement.RequestedBy, ts(requirement.CreatedAt))
	return err
}

func (s *SQLiteStore) GetDockerHostInputRequirement(ctx context.Context,
	attemptID string,
) (sandbox.DockerHostInputRequirement, bool, error) {
	attemptID = strings.TrimSpace(attemptID)
	if !domain.ValidAgentID(attemptID) || strings.ContainsRune(attemptID, 0) {
		return sandbox.DockerHostInputRequirement{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input requirement attempt id is invalid")
	}
	return getDockerHostInputRequirementByAttempt(ctx, s.db, attemptID)
}

func (s *SQLiteStore) GetDockerHostInputRequirementByOperation(ctx context.Context,
	operationKeyDigest string,
) (sandbox.DockerHostInputRequirement, bool, error) {
	operationKeyDigest = strings.TrimSpace(operationKeyDigest)
	if !validStoreDigest(operationKeyDigest) {
		return sandbox.DockerHostInputRequirement{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Docker host input requirement operation digest is invalid")
	}
	return getDockerHostInputRequirementByOperation(ctx, s.db, operationKeyDigest)
}

func getDockerHostInputRequirementByAttempt(ctx context.Context,
	queryer sandboxLifecycleQueryer, attemptID string,
) (sandbox.DockerHostInputRequirement, bool, error) {
	return scanDockerHostInputRequirementOptional(queryer.QueryRowContext(ctx,
		dockerHostInputRequirementSelect+` WHERE attempt_id = ?`, attemptID))
}

func getDockerHostInputRequirementByOperation(ctx context.Context,
	queryer sandboxLifecycleQueryer, operationKeyDigest string,
) (sandbox.DockerHostInputRequirement, bool, error) {
	return scanDockerHostInputRequirementOptional(queryer.QueryRowContext(ctx,
		dockerHostInputRequirementSelect+` WHERE operation_key_digest = ?`,
		operationKeyDigest))
}

func scanDockerHostInputRequirementOptional(row scanner,
) (sandbox.DockerHostInputRequirement, bool, error) {
	var requirement sandbox.DockerHostInputRequirement
	var required, confirmed int
	var createdAt string
	err := row.Scan(&requirement.AttemptID, &requirement.PlanID, &requirement.RunID,
		&requirement.MissionID, &requirement.WorkspaceID, &requirement.ProtocolVersion,
		&requirement.OperationKeyDigest, &requirement.AttemptIntentFingerprint,
		&requirement.RequestFingerprint, &requirement.ManifestFingerprint,
		&requirement.MountBindingFingerprint, &requirement.InputArtifactDigest,
		&requirement.AuthorityFingerprint, &requirement.PlanFingerprint, &required,
		&confirmed, &requirement.ReadOnlyMountCount, &requirement.InputArtifactCount,
		&requirement.RequirementFingerprint, &requirement.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sandbox.DockerHostInputRequirement{}, false, nil
	}
	if err != nil {
		return sandbox.DockerHostInputRequirement{}, false, err
	}
	requirement.Required, requirement.OperatorConfirmed = required != 0, confirmed != 0
	requirement.CreatedAt = parseTS(createdAt)
	if err := requirement.Validate(); err != nil {
		return sandbox.DockerHostInputRequirement{}, false, err
	}
	return requirement, true, nil
}
