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
		return errors.New("usage: cyberagent run sandbox prepare|list|show")
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
	default:
		return fmt.Errorf("unknown run sandbox subcommand %q", args[0])
	}
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
