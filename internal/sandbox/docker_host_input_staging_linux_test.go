//go:build linux

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxDockerHostInputStagerSealsDeterministicBundle(t *testing.T) {
	root, request := linuxHostInputFixture(t)
	stager := &linuxDockerHostInputStager{}
	if err := stager.Probe(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	first, err := stager.Stage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := stager.Stage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.BundleDigest != second.BundleDigest ||
		first.SourceSnapshotDigest != second.SourceSnapshotDigest ||
		first.ArtifactPayloadDigest != request.ArtifactPayloadDigest() ||
		first.ReportFingerprint != second.ReportFingerprint || !first.DescriptorPinned ||
		!first.SymlinkFree || !first.KernelSealed || first.DaemonConsumed ||
		first.ContainerStarted || first.ProcessExecuted || first.ExecutionEvidence {
		t.Fatalf("Linux host input bundle is not deterministic and bounded: first=%#v second=%#v",
			first, second)
	}
}

func TestLinuxDockerHostInputStagerRejectsSymlink(t *testing.T) {
	root, request := linuxHostInputFixture(t)
	if err := os.Symlink(filepath.Join(root, "outside"),
		filepath.Join(root, "src", "linked")); err != nil {
		t.Fatal(err)
	}
	_, err := (&linuxDockerHostInputStager{}).Stage(context.Background(), request)
	if DockerHostInputStagingErrorCode(err) != DockerHostInputStagingErrorUnsafeSource {
		t.Fatalf("Linux host input stager accepted a symlink: %v", err)
	}
}

func TestLinuxDockerHostInputStagerSupportsSingleFileMount(t *testing.T) {
	_, request := linuxHostInputFixture(t)
	request.Manifest.Mounts = []Mount{
		{Source: "src/nested", Target: "/workspace", Access: MountReadOnly},
		{Source: "src/main.txt", Target: "/inputs/main.txt", Access: MountReadOnly},
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	report, err := (&linuxDockerHostInputStager{}).Stage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReadOnlyMountCount != 2 || report.RegularFileCount != 2 ||
		report.DirectoryCount != 1 || report.EntryCount != 4 {
		t.Fatalf("single-file host input mount measurements are invalid: %#v", report)
	}
}

func TestLinuxDockerHostInputStagerRejectsFIFOWithoutBlocking(t *testing.T) {
	root, request := linuxHostInputFixture(t)
	if err := unix.Mkfifo(filepath.Join(root, "src", "blocked.pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (&linuxDockerHostInputStager{}).Stage(context.Background(), request)
	if DockerHostInputStagingErrorCode(err) != DockerHostInputStagingErrorUnsafeSource {
		t.Fatalf("Linux host input stager accepted a FIFO: %v", err)
	}
}

func TestLinuxDockerHostInputStagerRejectsHardLink(t *testing.T) {
	root, request := linuxHostInputFixture(t)
	if err := os.Link(filepath.Join(root, "src", "main.txt"),
		filepath.Join(root, "src", "main-copy.txt")); err != nil {
		t.Fatal(err)
	}
	_, err := (&linuxDockerHostInputStager{}).Stage(context.Background(), request)
	if DockerHostInputStagingErrorCode(err) != DockerHostInputStagingErrorUnsafeSource {
		t.Fatalf("Linux host input stager accepted a hard link: %v", err)
	}
}

func TestLinuxDockerHostInputDirectoryEnumerationIsBounded(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	directory, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	_, err = readBoundedHostInputDirectory(context.Background(), directory, 1)
	if DockerHostInputStagingErrorCode(err) != DockerHostInputStagingErrorResourceLimit {
		t.Fatalf("host input directory enumeration exceeded its bound: %v", err)
	}
}

func TestLinuxDockerHostInputStagerObservesCancellationAfterPin(t *testing.T) {
	_, request := linuxHostInputFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	stager := &linuxDockerHostInputStager{afterPin: cancel}
	if _, err := stager.Stage(ctx, request); err != context.Canceled {
		t.Fatalf("host input staging ignored cancellation after pin: %v", err)
	}
}

func TestLinuxDockerHostInputStagerRejectsSymlinkedWorkspaceRoot(t *testing.T) {
	root, request := linuxHostInputFixture(t)
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	request.WorkspaceRoot = link
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	_, err := (&linuxDockerHostInputStager{}).Stage(context.Background(), request)
	if DockerHostInputStagingErrorCode(err) != DockerHostInputStagingErrorUnsafeSource {
		t.Fatalf("Linux host input stager accepted a symlinked workspace root: %v", err)
	}
}

func TestLinuxDockerHostInputStagerDetectsMutationAfterDescriptorPin(t *testing.T) {
	tests := map[string]func(string) error{
		"overwrite": func(path string) error {
			return os.WriteFile(path, []byte("changed payload"), 0o644)
		},
		"rename and replace": func(path string) error {
			if err := os.Rename(path, path+".moved"); err != nil {
				return err
			}
			return os.WriteFile(path, []byte("replacement"), 0o644)
		},
		"delete": os.Remove,
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root, request := linuxHostInputFixture(t)
			path := filepath.Join(root, "src", "main.txt")
			var mutationErr error
			stager := &linuxDockerHostInputStager{afterPin: func() {
				mutationErr = mutate(path)
			}}
			_, err := stager.Stage(context.Background(), request)
			if mutationErr != nil {
				t.Fatal(mutationErr)
			}
			if DockerHostInputStagingErrorCode(err) != DockerHostInputStagingErrorSourceChanged {
				t.Fatalf("descriptor-pinned source mutation was accepted: %v", err)
			}
		})
	}
}

func linuxHostInputFixture(t *testing.T) (string, HostInputBundleRequest) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.txt"),
		[]byte("source payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "nested", "child.txt"),
		[]byte("child payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := validManifest()
	manifest.Mounts = []Mount{{Source: "src", Target: "/workspace", Access: MountReadOnly}}
	content := "artifact payload"
	artifact := HostInputArtifact{Ordinal: 1, ArtifactID: "artifact-one",
		SHA256: hashHostInputBytes([]byte(content)), SizeBytes: int64(len(content)),
		MIME: "text/plain", Stream: "stdout", SourceID: "tool-run-one",
		Redacted: true, Content: content}
	manifest.InputArtifactIDs = []string{artifact.ArtifactID}
	request := HostInputBundleRequest{WorkspaceRoot: root, Manifest: manifest,
		Artifacts: []HostInputArtifact{artifact}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(root, "/") {
		t.Fatalf("Linux fixture root is not absolute: %q", root)
	}
	return root, request
}
