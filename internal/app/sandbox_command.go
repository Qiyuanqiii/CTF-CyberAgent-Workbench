package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/sandbox"
)

func (a *App) sandboxCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent sandbox validate|template")
	}
	switch args[0] {
	case "validate":
		fs := newFlagSet("sandbox validate", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent sandbox validate <manifest.json>")
		}
		manifest, err := readSandboxManifest(fs.Arg(0))
		if err != nil {
			return err
		}
		validated, err := sandbox.NewNoopRunner().ValidateManifest(ctx, manifest)
		if err != nil {
			return err
		}
		fingerprint, err := validated.Fingerprint()
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "valid: true\nprotocol: %s\nbackend: %s\nmanifest_fingerprint: %s\narguments: %d\nmounts: %d\nwritable_mounts: %d\nenvironment_bindings: %d\nsecret_references: %d\nnetwork_mode: %s\nallowed_targets: %d\ninput_artifacts: %d\noutputs: %d\ntimeout_seconds: %d\nvalidator: noop\nbackend_enabled: false\nexecution_authorized: false\n",
			validated.ProtocolVersion, validated.Backend, fingerprint,
			len(validated.Command.Arguments), len(validated.Mounts), validated.WritableMountCount(),
			len(validated.Environment), validated.SecretReferenceCount(),
			validated.Network.Mode, len(validated.Network.AllowedTargets),
			len(validated.InputArtifactIDs), validated.OutputCount(), validated.TimeoutSeconds)
		return nil
	case "template":
		fs := newFlagSet("sandbox template", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("usage: cyberagent sandbox template")
		}
		encoded, err := json.MarshalIndent(defaultSandboxManifestTemplate(), "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, string(encoded))
		return nil
	default:
		return fmt.Errorf("unknown sandbox subcommand %q", args[0])
	}
}

func (a *App) runSandboxManifest(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent run sandbox prepare|list|show|request|review|candidate|candidates|candidate-show|begin|cancel|cleanup|executions|execution-show")
	}
	service := application.NewSandboxManifestService(a.store, a.checker)
	switch args[0] {
	case "prepare":
		fs := newFlagSet("run sandbox prepare", a.errOut)
		manifestPath := fs.String("manifest", "", "sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable sandbox preparation operation key")
		approvalID := fs.String("approval", "", "exact sandbox approval identity")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "approval": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox prepare <run-id> --manifest <manifest.json> --operation-key <key> [--approval <id>] [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		result, prepareErr := service.Prepare(ctx, application.PrepareSandboxManifestRequest{
			RunID: fs.Arg(0), Manifest: manifest, ApprovalID: *approvalID,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
		if result.Preparation.ID != "" {
			printSandboxManifestIntent(a, result)
		}
		return prepareErr
	case "list":
		fs := newFlagSet("run sandbox list", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox preparations")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox list <run-id> [--limit <n>]")
		}
		values, err := service.List(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox manifest preparations")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tbackend=%s\tpolicy_allowed=%t\tapproval=%s\texecution_authorized=false\tprepared_at=%s\n",
				value.Preparation.ID, value.Preparation.Backend,
				value.Validation.PolicyAllowed, value.Validation.ApprovalStatus,
				value.Preparation.PreparedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "show":
		fs := newFlagSet("run sandbox show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox show <preparation-id>")
		}
		value, err := service.Get(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxManifestIntent(a, value)
		return nil
	case "request":
		fs := newFlagSet("run sandbox request", a.errOut)
		operator := fs.String("operator", "cli_operator", "approval requester identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"operator": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox request <preparation-id> [--operator <id>]")
		}
		record, err := service.RequestApproval(ctx, fs.Arg(0), *operator)
		if err != nil {
			return err
		}
		printApproval(a.out, record)
		return nil
	case "review":
		fs := newFlagSet("run sandbox review", a.errOut)
		decision := fs.String("decision", "", "approve or deny")
		operationKey := fs.String("operation-key", "", "stable approval review operation key")
		reviewer := fs.String("reviewer", "cli_operator", "reviewer identity")
		reason := fs.String("reason", "", "required denial reason")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"decision": true, "operation-key": true, "reviewer": true, "reason": true,
		})); err != nil {
			return err
		}
		action := approval.Action(strings.TrimSpace(*decision))
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" ||
			(action != approval.ActionApprove && action != approval.ActionDeny) {
			return errors.New("usage: cyberagent run sandbox review <preparation-id> --decision approve|deny --operation-key <key> [--reviewer <id>] [--reason <text>]")
		}
		result, err := service.ReviewApproval(ctx, fs.Arg(0), action, *operationKey,
			*reviewer, *reason)
		if err != nil {
			return err
		}
		printApproval(a.out, result.Approval)
		fmt.Fprintf(a.out, "replayed: %t\n", result.Replayed)
		return nil
	case "candidate":
		fs := newFlagSet("run sandbox candidate", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable candidate validation operation key")
		approvalID := fs.String("approval", "", "exact approved sandbox approval identity")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "approval": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox candidate <preparation-id> --manifest <manifest.json> --operation-key <key> [--approval <id>] [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		result, err := service.ValidateExecutionCandidate(ctx,
			application.ValidateSandboxExecutionCandidateRequest{
				PreparationID: fs.Arg(0), Manifest: manifest, ApprovalID: *approvalID,
				OperationKey: *operationKey, RequestedBy: *operator,
			})
		if err != nil {
			return err
		}
		printSandboxExecutionCandidate(a, result)
		return nil
	case "candidates":
		fs := newFlagSet("run sandbox candidates", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox execution candidates")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox candidates <run-id> [--limit <n>]")
		}
		values, err := service.ListExecutionCandidates(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox execution candidates")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tpreparation=%s\tapproval=%s\tbackend_enabled=false\texecution_authorized=false\tvalidated_at=%s\n",
				value.Candidate.ID, value.Candidate.PreparationID, value.Candidate.ApprovalStatus,
				value.Candidate.ValidatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "candidate-show":
		fs := newFlagSet("run sandbox candidate-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox candidate-show <candidate-id>")
		}
		value, err := service.GetExecutionCandidate(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxExecutionCandidate(a, value)
		return nil
	case "begin":
		fs := newFlagSet("run sandbox begin", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable lifecycle creation operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox begin <candidate-id> --manifest <manifest.json> --operation-key <key> [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		value, err := service.BeginDisabledExecution(ctx, application.BeginSandboxExecutionRequest{
			CandidateID: fs.Arg(0), Manifest: manifest,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	case "cancel":
		fs := newFlagSet("run sandbox cancel", a.errOut)
		operationKey := fs.String("operation-key", "", "stable cancellation operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox cancel <execution-id> --operation-key <key> [--operator <id>]")
		}
		value, err := service.CancelDisabledExecution(ctx, application.CancelSandboxExecutionRequest{
			ExecutionID: fs.Arg(0), OperationKey: *operationKey, RequestedBy: *operator,
		})
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	case "cleanup":
		fs := newFlagSet("run sandbox cleanup", a.errOut)
		operationKey := fs.String("operation-key", "", "stable cleanup operation key")
		operator := fs.String("operator", "cli_operator", "reconciler identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox cleanup <execution-id> --operation-key <key> [--operator <id>]")
		}
		value, err := service.CleanupDisabledExecution(ctx, application.CleanupSandboxExecutionRequest{
			ExecutionID: fs.Arg(0), OperationKey: *operationKey, ReconciledBy: *operator,
		})
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	case "executions":
		fs := newFlagSet("run sandbox executions", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox executions")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox executions <run-id> [--limit <n>]")
		}
		values, err := service.ListDisabledExecutions(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox executions")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tcandidate=%s\tstatus=%s\tlease_generation=%d\tlease_status=%s\tbackend_enabled=false\texecution_authorized=false\tbackend_started=false\tcreated_at=%s\n",
				value.Execution.ID, value.Execution.CandidateID, value.Status,
				value.Lease.Generation, value.Lease.Status,
				value.Execution.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "execution-show":
		fs := newFlagSet("run sandbox execution-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox execution-show <execution-id>")
		}
		value, err := service.GetDisabledExecution(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	default:
		return fmt.Errorf("unknown run sandbox subcommand %q", args[0])
	}
}

func printSandboxLifecycle(a *App, value sandbox.Lifecycle) {
	execution := value.Execution
	fmt.Fprintf(a.out, "execution: %s\ncandidate: %s\npreparation: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nstatus: %s\nmanifest_fingerprint: %s\nauthorization_fingerprint: %s\npolicy_fingerprint: %s\nmount_binding_fingerprint: %s\ninput_artifacts: %d\ninput_artifact_bytes: %d\ninput_artifact_digest: %s\ncapture_stdout: %t\ncapture_stderr: %t\noutput_paths: %d\nmax_output_bytes: %d\noutput_plan_fingerprint: %s\nlease_generation: %d\nlease_status: %s\ncancellation_requested: %t\ncleanup_complete: %t\nbackend_enabled: false\nexecution_authorized: false\nbackend_started: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		execution.ID, execution.CandidateID, execution.PreparationID, execution.RunID,
		execution.MissionID, execution.WorkspaceID, execution.ProtocolVersion, value.Status,
		execution.ManifestFingerprint, execution.AuthorizationFingerprint,
		execution.PolicyFingerprint, execution.MountBindingFingerprint,
		execution.InputArtifactCount, execution.InputArtifactBytes,
		execution.InputArtifactDigest, execution.OutputPlan.CaptureStdout,
		execution.OutputPlan.CaptureStderr, execution.OutputPlan.OutputPathCount,
		execution.OutputPlan.MaxOutputBytes, execution.OutputPlan.Fingerprint,
		value.Lease.Generation, value.Lease.Status, value.Cancellation != nil,
		value.Cleanup != nil, execution.RequestedBy,
		execution.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	if value.Cleanup != nil {
		fmt.Fprintf(a.out, "cleanup_protocol: %s\ncleanup_outcome: %s\ncleanup_lease_generation: %d\ncancellation_observed: %t\ninput_artifacts_verified: %t\noutput_artifacts: %d\norphan_detected: false\norphan_reaped: false\ncleaned_at: %s\n",
			value.Cleanup.ProtocolVersion, value.Cleanup.Outcome,
			value.Cleanup.LeaseGeneration, value.Cleanup.CancellationObserved,
			value.Cleanup.InputArtifactsVerified, value.Cleanup.OutputArtifactCount,
			value.Cleanup.CompletedAt.Format(timeFormatRFC3339Nano))
	}
}

func printSandboxExecutionCandidate(a *App, value sandbox.ValidatedExecutionCandidate) {
	candidate := value.Candidate
	fmt.Fprintf(a.out, "candidate: %s\npreparation: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nmanifest_fingerprint: %s\nauthorization_fingerprint: %s\nmount_binding_fingerprint: %s\napproval: %s\napproval_status: %s\nmounts: %d\nregular_file_mounts: %d\ndirectory_mounts: %d\ntokens_used: %d\nexecution_millis_used: %d\ntool_calls_used: %d\nbudget_checked: true\nlease_quiescent: true\nbackend_enabled: false\nexecution_authorized: false\nrequested_by: %s\nvalidated_at: %s\nreplayed: %t\n",
		candidate.ID, candidate.PreparationID, candidate.RunID, candidate.MissionID,
		candidate.WorkspaceID, candidate.ProtocolVersion, candidate.ManifestFingerprint,
		candidate.AuthorizationFingerprint, candidate.MountBindingFingerprint,
		candidate.ApprovalID, candidate.ApprovalStatus, candidate.MountCount,
		candidate.RegularFileMountCount, candidate.DirectoryMountCount,
		candidate.TokensUsed, candidate.ExecutionMillisUsed, candidate.ToolCallsUsed,
		candidate.RequestedBy, candidate.ValidatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func printSandboxManifestIntent(a *App, value sandbox.PreparedIntent) {
	preparation := value.Preparation
	validation := value.Validation
	fmt.Fprintf(a.out, "preparation: %s\nrun: %s\nmission: %s\nworkspace: %s\ncancellation: %s\nprotocol: %s\nbackend: %s\nmanifest_fingerprint: %s\nauthorization_fingerprint: %s\narguments: %d\nmounts: %d\nwritable_mounts: %d\nenvironment_bindings: %d\nsecret_references: %d\nnetwork_mode: %s\nallowed_targets: %d\ninput_artifacts: %d\noutputs: %d\ntimeout_seconds: %d\npolicy_allowed: %t\nneeds_approval: %t\nrisk: %s\napproval_status: %s\nvalidator: %s\nbackend_enabled: false\nexecution_authorized: false\nrequested_by: %s\nreplayed: %t\n",
		preparation.ID, preparation.RunID, preparation.MissionID, preparation.WorkspaceID,
		preparation.CancellationID, preparation.ProtocolVersion, preparation.Backend,
		preparation.ManifestFingerprint, preparation.AuthorizationFingerprint,
		preparation.CommandArgumentCount, preparation.MountCount, preparation.WritableMountCount,
		preparation.EnvironmentCount,
		preparation.SecretReferenceCount, preparation.NetworkMode,
		preparation.AllowedTargetCount, preparation.InputArtifactCount,
		preparation.OutputCount, preparation.TimeoutSeconds,
		validation.PolicyAllowed, validation.NeedsApproval, validation.Risk,
		validation.ApprovalStatus, validation.ValidatorName, preparation.RequestedBy,
		value.Replayed)
}

func readSandboxManifest(path string) (sandbox.Manifest, error) {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return sandbox.Manifest{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, sandbox.MaxManifestBytes+1))
	if err != nil {
		return sandbox.Manifest{}, err
	}
	manifest, err := sandbox.DecodeManifest(data)
	if err != nil {
		return sandbox.Manifest{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest is invalid: "+err.Error(), err)
	}
	return manifest, nil
}

func defaultSandboxManifestTemplate() sandbox.Manifest {
	return sandbox.Manifest{
		ProtocolVersion: sandbox.ManifestProtocolVersion,
		Backend:         sandbox.BackendNoop,
		Command: sandbox.CommandSpec{
			Executable: "go", Arguments: []string{"test", "./..."},
			WorkingDirectory: "/workspace",
		},
		Mounts: []sandbox.Mount{{
			Source: ".", Target: "/workspace", Access: sandbox.MountReadOnly,
		}},
		Network: sandbox.NetworkScope{Mode: "disabled"},
		Resources: sandbox.ResourceLimits{
			CPUQuotaMillis: 1000, MemoryBytes: 256 * 1024 * 1024,
			PIDs: 64, MaxOutputBytes: 4 * 1024 * 1024,
		},
		Output:         sandbox.OutputSpec{CaptureStdout: true, CaptureStderr: true},
		TimeoutSeconds: 300,
		Cancellation:   sandbox.CancellationSpec{GracePeriodMillis: 2000},
	}
}
