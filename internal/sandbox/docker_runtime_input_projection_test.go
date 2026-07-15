package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

type runtimeProjectionTestEntry struct {
	name    string
	kind    byte
	content string
}

const runtimeProjectionTestBinding = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

type runtimeProjectionTestBundle struct {
	*bytes.Reader
	report HostInputBundleReport
}

func (bundle *runtimeProjectionTestBundle) Report() HostInputBundleReport {
	return bundle.report
}

func (bundle *runtimeProjectionTestBundle) Close() error { return nil }

func TestCompileDockerRuntimeInputProjectionBundleMapsExactReadOnlyTargets(t *testing.T) {
	manifest := dockerContainerCompilerManifest()
	bundle := newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: "mounts/001/cmd", kind: tar.TypeDir},
		{name: "mounts/001/cmd/main.go", kind: tar.TypeReg, content: "package main\n"},
		{name: "artifacts/001", kind: tar.TypeReg, content: "artifact body\n"},
	}, 1, 1)
	compiled, err := CompileDockerRuntimeInputProjectionBundle(context.Background(),
		manifest, bundle, runtimeProjectionTestBinding)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.ReadOnlyMountCount != 1 || compiled.InputArtifactCount != 1 ||
		compiled.DirectoryRootCount != 1 || compiled.FileRootCount != 0 ||
		len(compiled.Items) != 2 || len(compiled.Archives) != 2 {
		t.Fatalf("unexpected projection compilation: %+v", compiled)
	}
	if compiled.Archives[0].Target != DockerRuntimeArtifactTarget ||
		compiled.Items[0].Kind != DockerRuntimeInputProjectionKindArtifacts ||
		compiled.Archives[1].Target != "/workspace" ||
		compiled.Items[1].Kind != DockerRuntimeInputProjectionKindManifestMount ||
		compiled.Items[1].ManifestMountOrdinal != 1 {
		t.Fatalf("projection targets are not exact and deterministic: %+v", compiled.Archives)
	}
	if strings.Contains(compiled.Items[0].TargetFingerprint, "/") ||
		strings.Contains(compiled.Items[1].TargetFingerprint, "/") {
		t.Fatal("persistable projection item retained a raw target")
	}
	assertRuntimeProjectionArchiveNames(t, compiled.Archives[0].Data, []string{"001"})
	assertRuntimeProjectionArchiveNames(t, compiled.Archives[1].Data,
		[]string{"cmd/", "cmd/main.go"})
	if err := compiled.Validate(); err != nil {
		t.Fatal(err)
	}

	secondBundle := newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: "mounts/001/cmd", kind: tar.TypeDir},
		{name: "mounts/001/cmd/main.go", kind: tar.TypeReg, content: "package main\n"},
		{name: "artifacts/001", kind: tar.TypeReg, content: "artifact body\n"},
	}, 1, 1)
	second, err := CompileDockerRuntimeInputProjectionBundle(context.Background(),
		manifest, secondBundle, runtimeProjectionTestBinding)
	if err != nil || second.ProjectionSetFingerprint != compiled.ProjectionSetFingerprint {
		t.Fatalf("projection compilation is not deterministic: err=%v", err)
	}
	isolatedBundle := newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: "mounts/001/cmd", kind: tar.TypeDir},
		{name: "mounts/001/cmd/main.go", kind: tar.TypeReg, content: "package main\n"},
		{name: "artifacts/001", kind: tar.TypeReg, content: "artifact body\n"},
	}, 1, 1)
	isolated, err := CompileDockerRuntimeInputProjectionBundle(context.Background(),
		manifest, isolatedBundle, strings.Repeat("e", 64))
	if err != nil || isolated.Archives[0].VolumeName == compiled.Archives[0].VolumeName ||
		isolated.ProjectionSetFingerprint == compiled.ProjectionSetFingerprint {
		t.Fatalf("projection volume identity is not isolated by handoff: err=%v", err)
	}
}

func TestCompileDockerRuntimeInputProjectionBundleRejectsUnsafeTarAndFileRoots(t *testing.T) {
	manifest := dockerContainerCompilerManifest()
	tests := []struct {
		name    string
		entries []runtimeProjectionTestEntry
	}{
		{name: "file root", entries: []runtimeProjectionTestEntry{
			{name: "mounts/001", kind: tar.TypeReg, content: "file root"},
			{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
		}},
		{name: "symbolic link", entries: []runtimeProjectionTestEntry{
			{name: "mounts/001", kind: tar.TypeDir},
			{name: "mounts/001/link", kind: tar.TypeSymlink},
			{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
		}},
		{name: "unexpected root", entries: []runtimeProjectionTestEntry{
			{name: "mounts/001", kind: tar.TypeDir},
			{name: "secrets/001", kind: tar.TypeReg, content: "secret"},
			{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
		}},
		{name: "duplicate path", entries: []runtimeProjectionTestEntry{
			{name: "mounts/001", kind: tar.TypeDir},
			{name: "mounts/001/a", kind: tar.TypeReg, content: "one"},
			{name: "mounts/001/a", kind: tar.TypeReg, content: "two"},
			{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
		}},
		{name: "missing parent", entries: []runtimeProjectionTestEntry{
			{name: "mounts/001", kind: tar.TypeDir},
			{name: "mounts/001/missing/file", kind: tar.TypeReg, content: "body"},
			{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := newRuntimeProjectionTestBundle(t, test.entries, 1, 1)
			if _, err := CompileDockerRuntimeInputProjectionBundle(context.Background(),
				manifest, bundle, runtimeProjectionTestBinding); err == nil {
				t.Fatal("unsafe runtime input bundle was accepted")
			}
		})
	}
}

func TestCompileDockerRuntimeInputProjectionBundleRejectsReservedTargetAndCancellation(t *testing.T) {
	manifest := dockerContainerCompilerManifest()
	manifest.Mounts[1].Target = "/cyberagent-input/nested"
	manifest.Command.WorkingDirectory = "/cyberagent-input/nested"
	bundle := newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: "mounts/001/main.go", kind: tar.TypeReg, content: "package main\n"},
		{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
	}, 1, 1)
	if _, err := CompileDockerRuntimeInputProjectionBundle(context.Background(),
		manifest, bundle, runtimeProjectionTestBinding); err == nil {
		t.Fatal("reserved runtime input target was accepted")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	bundle = newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
	}, 1, 1)
	if _, err := CompileDockerRuntimeInputProjectionBundle(cancelled,
		dockerContainerCompilerManifest(), bundle, runtimeProjectionTestBinding); err == nil {
		t.Fatal("cancelled projection compilation was accepted")
	}
}

func TestCompileDockerRuntimeInputProjectionBundleRejectsTrailingTarData(t *testing.T) {
	base := newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
	}, 1, 1)
	data, err := io.ReadAll(base)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("hidden trailing instruction")...)
	report, err := NewHostInputBundleReport(HostInputBundleMeasurements{
		ReadOnlyMountCount:    base.report.ReadOnlyMountCount,
		ArtifactCount:         base.report.ArtifactCount,
		RegularFileCount:      base.report.RegularFileCount,
		DirectoryCount:        base.report.DirectoryCount,
		SourceBytes:           base.report.SourceBytes,
		ArtifactBytes:         base.report.ArtifactBytes,
		BundleBytes:           int64(len(data)),
		SourceSnapshotDigest:  base.report.SourceSnapshotDigest,
		ArtifactPayloadDigest: base.report.ArtifactPayloadDigest,
		BundleDigest:          hashHostInputBytes(data),
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	bundle := &runtimeProjectionTestBundle{Reader: bytes.NewReader(data), report: report}
	if _, err := CompileDockerRuntimeInputProjectionBundle(context.Background(),
		dockerContainerCompilerManifest(), bundle, runtimeProjectionTestBinding); err == nil {
		t.Fatal("runtime input projection accepted trailing tar data")
	}
}

func TestCompileDockerRuntimeInputProjectionBundleAcceptsCanonicalLongPAXPath(t *testing.T) {
	longDirectory := "mounts/001/" + strings.Repeat("a", 120)
	bundle := newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: longDirectory, kind: tar.TypeDir},
		{name: longDirectory + "/main.go", kind: tar.TypeReg, content: "package main\n"},
		{name: "artifacts/001", kind: tar.TypeReg, content: "artifact"},
	}, 1, 1)
	compiled, err := CompileDockerRuntimeInputProjectionBundle(context.Background(),
		dockerContainerCompilerManifest(), bundle, runtimeProjectionTestBinding)
	if err != nil || len(compiled.Items) != 2 {
		t.Fatalf("canonical long PAX path was rejected: %#v err=%v", compiled, err)
	}
}

func newRuntimeProjectionTestBundle(t *testing.T, entries []runtimeProjectionTestEntry,
	readOnlyMounts, artifactCount int,
) *runtimeProjectionTestBundle {
	t.Helper()
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	sourceParts := []string{"sandbox_host_input_source_snapshot.v1", "0"}
	regularFiles, directories := 0, 0
	var sourceBytes, artifactBytes int64
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Typeflag: entry.kind,
			ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(),
			ChangeTime: time.Unix(0, 0).UTC(), Uid: 65532, Gid: 65532,
			Format: tar.FormatPAX}
		switch entry.kind {
		case tar.TypeDir:
			header.Name += "/"
			header.Mode = 0o555
			directories++
		case tar.TypeReg:
			header.Mode = 0o444
			header.Size = int64(len([]byte(entry.content)))
		default:
			header.Mode = 0o777
			header.Linkname = "target"
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.kind == tar.TypeReg {
			if _, err := io.WriteString(writer, entry.content); err != nil {
				t.Fatal(err)
			}
		}
		if strings.HasPrefix(entry.name, "mounts/") {
			digest := fingerprint("sandbox_host_input_directory.v1", entry.name)
			if entry.kind == tar.TypeReg {
				regularFiles++
				sourceBytes += header.Size
				digest = hashHostInputBytes([]byte(entry.content))
			}
			sourceParts = append(sourceParts,
				fingerprint("sandbox_host_input_archive_path.v1", entry.name),
				strconv.Itoa(int(entry.kind)), strconv.FormatInt(header.Size, 10), digest)
		} else if strings.HasPrefix(entry.name, "artifacts/") && entry.kind == tar.TypeReg {
			artifactBytes += header.Size
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sourceParts[1] = strconv.Itoa(regularFiles + directories)
	report, err := NewHostInputBundleReport(HostInputBundleMeasurements{
		ReadOnlyMountCount: readOnlyMounts, ArtifactCount: artifactCount,
		RegularFileCount: regularFiles, DirectoryCount: directories,
		SourceBytes: sourceBytes, ArtifactBytes: artifactBytes,
		BundleBytes: int64(output.Len()), SourceSnapshotDigest: fingerprint(sourceParts...),
		ArtifactPayloadDigest: strings.Repeat("a", 64),
		BundleDigest:          hashHostInputBytes(output.Bytes()),
	}, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return &runtimeProjectionTestBundle{Reader: bytes.NewReader(output.Bytes()), report: report}
}

func assertRuntimeProjectionArchiveNames(t *testing.T, data []byte, expected []string) {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(data))
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	if strings.Join(names, "|") != strings.Join(expected, "|") {
		t.Fatalf("projection archive names = %v, want %v", names, expected)
	}
}
