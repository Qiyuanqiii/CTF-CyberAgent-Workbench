package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/buildinfo"
)

func (a *App) doctorCommand(_ context.Context, args []string) error {
	if len(args) == 0 || args[0] != "portable" {
		return errors.New("usage: cyberagent doctor portable [--json]")
	}
	flags := newFlagSet("doctor portable", a.errOut)
	jsonOutput := flags.Bool("json", false, "print the portable build diagnostic as JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: cyberagent doctor portable [--json]")
	}
	diagnostic := buildinfo.PortableDiagnostic()
	if *jsonOutput {
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(diagnostic)
	}
	release := diagnostic.Release
	fmt.Fprintf(a.out, "protocol: %s\nversion: %s\nrevision: %s\nsource_date: %s\n",
		diagnostic.ProtocolVersion, release.AppVersion, release.Revision,
		valueOrUnknown(release.SourceDate))
	fmt.Fprintf(a.out, "target: %s/%s\ngo: %s\ncgo: %s\ntrimpath: %t\n",
		release.TargetOS, release.TargetArch, release.GoVersion,
		valueOrUnknown(release.CGOEnabled), release.Trimpath)
	fmt.Fprintf(a.out, "fingerprint: %s\nrelease_ready: %t\nchecks:\n",
		release.BuildFingerprint, diagnostic.ReleaseReady)
	for _, check := range diagnostic.Checks {
		fmt.Fprintf(a.out, "- %s: %s (%s)\n", check.ID, check.Status, check.Detail)
	}
	return nil
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
