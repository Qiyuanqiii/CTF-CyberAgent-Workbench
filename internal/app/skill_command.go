package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/toolgateway"
)

func (a *App) skillCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("skill subcommand is required")
	}
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		flags := newFlagSet("skill list", a.errOut)
		profileValue := flags.String("profile", "", "filter by compatible Profile")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: cyberagent skill list [--profile code|review|learn|script]")
		}
		var profile domain.Profile
		if strings.TrimSpace(*profileValue) != "" {
			profile, err = domain.ParseProfile(*profileValue)
			if err != nil {
				return err
			}
		}
		for _, manifest := range registry.List(profile) {
			fmt.Fprintf(a.out, "%s@%s\tprofiles=%s\ttools=%s\tcontent=%s\tbytes=%d\ttoken_upper_bound=%d\n",
				manifest.Name, manifest.Version, joinProfiles(manifest.Profiles), joinToolDependencies(manifest.ToolDependencies),
				manifest.ContentPath, manifest.ContentBytes, manifest.ContentTokenUpperBound)
		}
		printSkillBoundary(a)
		return nil
	case "show":
		if len(args) != 2 {
			return errors.New("usage: cyberagent skill show <name>")
		}
		manifest, ok := registry.Get(args[1])
		if !ok {
			return fmt.Errorf("skill %q not found", args[1])
		}
		fmt.Fprintf(a.out, "protocol: %s\nname: %s\nversion: %s\ndescription: %s\nprofiles: %s\ntool_dependencies: %s\ncontent_path: %s\ncontent_sha256: %s\ncontent_bytes: %d\ncontent_token_upper_bound: %d\n",
			manifest.Protocol, manifest.Name, manifest.Version, manifest.Description, joinProfiles(manifest.Profiles),
			joinToolDependencies(manifest.ToolDependencies), manifest.ContentPath, manifest.ContentSHA256,
			manifest.ContentBytes, manifest.ContentTokenUpperBound)
		printSkillBoundary(a)
		return nil
	case "validate":
		if len(args) != 1 {
			return errors.New("usage: cyberagent skill validate")
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "validated %d built-in %s manifests\n", len(registry.List("")), skills.ProtocolVersion)
		printSkillBoundary(a)
		return nil
	case "package":
		if len(args) != 3 || args[1] != "validate" {
			return errors.New("usage: cyberagent skill package validate <package.zip>")
		}
		raw, err := readSkillPackageArchive(args[2])
		if err != nil {
			return err
		}
		validated, err := skills.ParsePackage(raw)
		if err != nil {
			return err
		}
		printSkillPackagePreview(a, validated.Preview())
		return nil
	case "select":
		flags := newFlagSet("skill select", a.errOut)
		tokenBudget := flags.Int("token-budget", skills.DefaultSelectionTokenBudget,
			"conservative aggregate token budget")
		operationKey := flags.String("operation-key", "", "stable idempotency key")
		requestedBy := flags.String("operator", "cli_operator", "operator identity")
		if err := flags.Parse(reorderFlags(args[1:], map[string]bool{
			"token-budget": true, "operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if flags.NArg() < 2 {
			return errors.New("usage: cyberagent skill select <run-id> <name> [name...] --operation-key <stable-key> [--token-budget 4096] [--operator cli_operator]")
		}
		if err := a.ensureStore(); err != nil {
			return err
		}
		result, err := application.NewSkillSelectionService(a.store, registry).Select(ctx,
			application.SelectSkillsRequest{
				RunID: flags.Arg(0), Names: flags.Args()[1:], TokenBudget: *tokenBudget,
				OperationKey: *operationKey, RequestedBy: *requestedBy,
			})
		if err != nil {
			return err
		}
		printSkillSelection(a, result.Selection)
		fmt.Fprintf(a.out, "replayed: %t\n", result.Replayed)
		printSkillBoundary(a)
		return nil
	case "selection":
		if len(args) != 2 {
			return errors.New("usage: cyberagent skill selection <run-id>")
		}
		if err := a.ensureStore(); err != nil {
			return err
		}
		selection, err := application.NewSkillSelectionService(a.store, registry).
			GetForRun(ctx, args[1])
		if err != nil {
			return err
		}
		printSkillSelection(a, selection)
		printSkillBoundary(a)
		return nil
	default:
		return fmt.Errorf("unknown skill subcommand %q", args[0])
	}
}

func readSkillPackageArchive(value string) ([]byte, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return nil, errors.New("invalid skill package path: path is required")
	}
	before, err := os.Lstat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperror.Wrap(apperror.CodeNotFound, "skill package file not found", err)
		}
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, "skill package file cannot be inspected", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, errors.New("invalid skill package path: package must be a non-symlink regular file")
	}
	if before.Size() <= 0 || before.Size() > skills.MaxPackageArchiveBytes {
		return nil, fmt.Errorf("invalid skill package path: archive must contain between 1 and %d bytes", skills.MaxPackageArchiveBytes)
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, "skill package file cannot be opened", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, "skill package file identity cannot be verified", err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, errors.New("invalid skill package path: package changed before it was opened")
	}
	raw, err := io.ReadAll(io.LimitReader(file, skills.MaxPackageArchiveBytes+1))
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, "skill package file cannot be read", err)
	}
	after, err := file.Stat()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, "skill package file identity cannot be reverified", err)
	}
	if !os.SameFile(opened, after) || before.Size() != after.Size() ||
		before.ModTime() != after.ModTime() || int64(len(raw)) != after.Size() {
		return nil, errors.New("invalid skill package path: package changed while it was read")
	}
	if len(raw) == 0 || len(raw) > skills.MaxPackageArchiveBytes {
		return nil, fmt.Errorf("invalid skill package path: archive must contain between 1 and %d bytes", skills.MaxPackageArchiveBytes)
	}
	return raw, nil
}

func printSkillPackagePreview(a *App, preview skills.PackagePreview) {
	fmt.Fprintf(a.out, "package_protocol: %s\nskill_protocol: %s\nskill: %s@%s\nprofiles: %s\ntool_dependencies: %s\ncontent_sha256: %s\ncontent_bytes: %d\ncontent_token_upper_bound: %d\narchive_sha256: %s\npackage_fingerprint: %s\narchive_bytes: %d\nuncompressed_bytes: %d\nentry_count: %d\ntrust_class: %s\nrisk_codes: %s\nexecutable_assets: %d\ninstall_hooks: %d\nimport_command_execution: %t\nimport_network_access: %t\nimport_provider_calls: %t\ntool_capability_grant: %t\ninstallation_authorized: %t\nvalidated: true\n",
		preview.ProtocolVersion, preview.Manifest.Protocol, preview.Manifest.Name,
		preview.Manifest.Version, joinProfiles(preview.Manifest.Profiles),
		joinToolDependencies(preview.Manifest.ToolDependencies), preview.Manifest.ContentSHA256,
		preview.Manifest.ContentBytes, preview.Manifest.ContentTokenUpperBound,
		preview.ArchiveSHA256, preview.PackageFingerprint, preview.ArchiveBytes,
		preview.UncompressedBytes, preview.EntryCount, preview.TrustClass,
		joinPackageRiskCodes(preview.RiskCodes), preview.ExecutableAssetCount,
		preview.InstallHookCount, preview.ImportCommandExecution, preview.ImportNetworkAccess,
		preview.ImportProviderCalls, preview.ToolCapabilityGrant, preview.InstallationAuthorized)
}

func joinPackageRiskCodes(codes []skills.PackageRiskCode) string {
	values := make([]string, len(codes))
	for index, code := range codes {
		values[index] = string(code)
	}
	return strings.Join(values, ",")
}

func printSkillSelection(a *App, selection skills.Selection) {
	fmt.Fprintf(a.out, "selection_id: %s\nrun_id: %s\nmission_id: %s\nprotocol: %s\nprofile: %s\ntoken_budget: %d\ntoken_upper_bound: %d\nitem_count: %d\nselection_fingerprint: %s\nrequested_by: %s\ncreated_at: %s\n",
		selection.ID, selection.RunID, selection.MissionID, selection.ProtocolVersion,
		selection.Profile, selection.TokenBudget, selection.TokenUpperBound,
		selection.ItemCount, selection.Fingerprint, selection.RequestedBy,
		selection.CreatedAt.Format("2006-01-02T15:04:05.000000000Z"))
	for _, item := range selection.Items {
		fmt.Fprintf(a.out, "skill[%d]: %s@%s sha256=%s bytes=%d token_upper_bound=%d\n",
			item.Ordinal, item.Name, item.Version, item.ContentSHA256,
			item.ContentBytes, item.TokenUpperBound)
	}
}

func joinProfiles(profiles []domain.Profile) string {
	values := make([]string, len(profiles))
	for index, profile := range profiles {
		values[index] = string(profile)
	}
	return strings.Join(values, ",")
}

func joinToolDependencies(dependencies []toolgateway.ToolName) string {
	values := make([]string, len(dependencies))
	for index, dependency := range dependencies {
		values[index] = string(dependency)
	}
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
}

func printSkillBoundary(a *App) {
	fmt.Fprintln(a.out, "context_injection: root_selected_and_specialist_minimized")
	fmt.Fprintln(a.out, "tool_capability_grant: disabled")
}
