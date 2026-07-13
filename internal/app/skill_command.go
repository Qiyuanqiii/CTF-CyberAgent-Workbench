package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/toolgateway"
)

func (a *App) skillCommand(_ context.Context, args []string) error {
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
	default:
		return fmt.Errorf("unknown skill subcommand %q", args[0])
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
	fmt.Fprintln(a.out, "context_injection: disabled")
	fmt.Fprintln(a.out, "tool_capability_grant: disabled")
}
