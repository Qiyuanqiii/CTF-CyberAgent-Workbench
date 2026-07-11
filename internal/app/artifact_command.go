package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/artifact"
)

const defaultArtifactReadBytes = 64 * 1024

func (a *App) artifactCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("artifact subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	switch args[0] {
	case "list":
		return a.artifactList(ctx, args[1:])
	case "show":
		return a.artifactShow(ctx, args[1:])
	case "read":
		return a.artifactRead(ctx, args[1:])
	case "verify":
		return a.artifactVerify(ctx, args[1:])
	default:
		return fmt.Errorf("unknown artifact subcommand %q", args[0])
	}
}

func (a *App) artifactList(ctx context.Context, args []string) error {
	fs := newFlagSet("artifact list", a.errOut)
	runID := fs.String("run", "", "Run id")
	sourceID := fs.String("source", "", "tool proposal or result source id")
	stream := fs.String("stream", "", "stdout or stderr")
	limit := fs.Int("limit", 100, "maximum records")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"run": true, "source": true, "stream": true, "limit": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: cyberagent artifact list [--run <id>] [--source <id>] [--stream stdout|stderr] [--limit <n>]")
	}
	descriptors, err := a.newToolGateway().Artifacts().List(ctx, artifact.ListFilter{
		RunID: *runID, SourceID: *sourceID, Stream: artifact.Stream(strings.TrimSpace(*stream)), Limit: *limit,
	})
	if err != nil {
		return err
	}
	if len(descriptors) == 0 {
		fmt.Fprintln(a.out, "no Run artifacts")
		return nil
	}
	for _, descriptor := range descriptors {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%d\t%s\n", descriptor.ID, descriptor.Stream,
			descriptor.ToolName, descriptor.SourceID, descriptor.SizeBytes, descriptor.SHA256)
	}
	return nil
}

func (a *App) artifactShow(ctx context.Context, args []string) error {
	fs := newFlagSet("artifact show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent artifact show <artifact-id>")
	}
	blob, err := a.newToolGateway().Artifacts().Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	printArtifactDescriptor(a.out, blob.Descriptor)
	return nil
}

func (a *App) artifactRead(ctx context.Context, args []string) error {
	fs := newFlagSet("artifact read", a.errOut)
	maxBytes := fs.Int("max-bytes", defaultArtifactReadBytes, "maximum content bytes to print")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"max-bytes": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent artifact read <artifact-id> [--max-bytes <n>]")
	}
	if *maxBytes <= 0 || *maxBytes > artifact.MaxContentBytes {
		return fmt.Errorf("artifact read max-bytes must be between 1 and %d", artifact.MaxContentBytes)
	}
	blob, err := a.newToolGateway().Artifacts().Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	content, truncated := boundArtifactContent(blob.Content, *maxBytes)
	if _, err := io.WriteString(a.out, content); err != nil {
		return err
	}
	if !strings.HasSuffix(content, "\n") {
		fmt.Fprintln(a.out)
	}
	if truncated {
		fmt.Fprintf(a.out, "[artifact preview truncated: %d of %d bytes]\n", len([]byte(content)), blob.SizeBytes)
	}
	return nil
}

func (a *App) artifactVerify(ctx context.Context, args []string) error {
	fs := newFlagSet("artifact verify", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent artifact verify <artifact-id>")
	}
	blob, err := a.newToolGateway().Artifacts().Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "artifact %s verified\nsha256: %s\nsize_bytes: %d\n", blob.ID, blob.SHA256, blob.SizeBytes)
	return nil
}

func printArtifactDescriptor(out io.Writer, descriptor artifact.Descriptor) {
	fmt.Fprintf(out, "id: %s\n", descriptor.ID)
	fmt.Fprintf(out, "run: %s\n", descriptor.RunID)
	fmt.Fprintf(out, "session: %s\n", descriptor.SessionID)
	fmt.Fprintf(out, "workspace: %s\n", descriptor.WorkspaceID)
	fmt.Fprintf(out, "source: %s\n", descriptor.SourceID)
	fmt.Fprintf(out, "tool: %s\n", descriptor.ToolName)
	fmt.Fprintf(out, "stream: %s\n", descriptor.Stream)
	fmt.Fprintf(out, "kind: %s\n", descriptor.Kind)
	fmt.Fprintf(out, "mime: %s\n", descriptor.MIME)
	fmt.Fprintf(out, "encoding: %s\n", descriptor.Encoding)
	fmt.Fprintf(out, "sha256: %s\n", descriptor.SHA256)
	fmt.Fprintf(out, "size_bytes: %d\n", descriptor.SizeBytes)
	fmt.Fprintf(out, "redacted: %t\n", descriptor.Redacted)
	fmt.Fprintf(out, "created_at: %s\n", descriptor.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

func boundArtifactContent(content string, limit int) (string, bool) {
	if len([]byte(content)) <= limit {
		return content, false
	}
	end := limit
	for end > 0 && !utf8.ValidString(content[:end]) {
		end--
	}
	return content[:end], true
}
