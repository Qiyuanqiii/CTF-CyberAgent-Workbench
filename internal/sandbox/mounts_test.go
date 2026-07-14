package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMountSourcesBindsRegularFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := mountResolutionManifest("input.txt")
	first, err := ResolveMountSources(context.Background(), root, manifest)
	if err != nil || first.MountCount != 1 || first.RegularFileCount != 1 ||
		first.DirectoryCount != 0 || !validDigest(first.Fingerprint) {
		t.Fatalf("regular mount resolution failed: %#v err=%v", first, err)
	}
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("two-two"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := ResolveMountSources(context.Background(), root, manifest)
	if err != nil || second.Fingerprint == first.Fingerprint {
		t.Fatalf("changed mount source retained its binding fingerprint: %#v err=%v", second, err)
	}
	directory, err := ResolveMountSources(context.Background(), root, mountResolutionManifest("."))
	if err != nil || directory.DirectoryCount != 1 || directory.RegularFileCount != 0 {
		t.Fatalf("directory mount resolution failed: %#v err=%v", directory, err)
	}
}

func TestResolveMountSourcesRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks are unavailable in this environment: %v", err)
	}
	if _, err := ResolveMountSources(context.Background(), root,
		mountResolutionManifest("escape.txt")); err == nil {
		t.Fatal("workspace Root followed a mount symlink outside the workspace")
	}
}

func TestResolveMountSourcesHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResolveMountSources(ctx, t.TempDir(), mountResolutionManifest(".")); err == nil {
		t.Fatal("cancelled mount resolution continued")
	}
}

func mountResolutionManifest(source string) Manifest {
	return Manifest{
		ProtocolVersion: ManifestProtocolVersion, Backend: BackendNoop,
		Command: CommandSpec{Executable: "go", WorkingDirectory: "/workspace"},
		Mounts:  []Mount{{Source: source, Target: "/workspace", Access: MountReadOnly}},
		Network: NetworkScope{Mode: "disabled"},
		Resources: ResourceLimits{CPUQuotaMillis: 1000, MemoryBytes: MinMemoryBytes,
			PIDs: 16, MaxOutputBytes: 1024},
		Output: OutputSpec{CaptureStdout: true}, TimeoutSeconds: 30,
		Cancellation: CancellationSpec{GracePeriodMillis: 100},
	}
}
