package sandbox

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHostInputBundleRequestAndReportBindArtifactMetadata(t *testing.T) {
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifest := validManifest()
	content := "redacted evidence"
	artifact := HostInputArtifact{
		Ordinal: 1, ArtifactID: "artifact-one", SHA256: hashHostInputBytes([]byte(content)),
		SizeBytes: int64(len(content)), MIME: "text/plain", Stream: "stdout",
		SourceID: "tool-run-one", Redacted: true, Content: content,
	}
	manifest.InputArtifactIDs = []string{artifact.ArtifactID}
	request := HostInputBundleRequest{WorkspaceRoot: root, Manifest: manifest,
		Artifacts: []HostInputArtifact{artifact}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	if request.ReadOnlyMountCount() != 1 || request.ArtifactBytes() != artifact.SizeBytes ||
		!validDigest(request.ArtifactPayloadDigest()) {
		t.Fatalf("host input request measurements are invalid: %#v", request)
	}
	invalidRoot := request
	invalidRoot.WorkspaceRoot += "\x00escape"
	if invalidRoot.Validate() == nil {
		t.Fatal("host input request accepted a NUL-containing workspace root")
	}
	report, err := NewHostInputBundleReport(HostInputBundleMeasurements{
		ReadOnlyMountCount: request.ReadOnlyMountCount(), ArtifactCount: len(request.Artifacts),
		RegularFileCount: 1, DirectoryCount: 0, SourceBytes: 8,
		ArtifactBytes: request.ArtifactBytes(), BundleBytes: 4096,
		SourceSnapshotDigest:  strings.Repeat("a", 64),
		ArtifactPayloadDigest: request.ArtifactPayloadDigest(),
		BundleDigest:          strings.Repeat("b", 64),
	}, time.Now().UTC())
	if err != nil || report.Validate() != nil || report.SourcePathsRetained ||
		report.RawContentPersisted || report.DaemonConsumed || report.ContainerStarted ||
		report.ProcessExecuted || report.ExecutionEvidence {
		t.Fatalf("host input report widened authority: %#v err=%v", report, err)
	}
	tampered := report
	tampered.ArtifactBytes++
	if tampered.Validate() == nil {
		t.Fatal("host input report accepted tampered measurements")
	}
}

func TestUnavailableDockerHostInputStagerFailsClosed(t *testing.T) {
	stager := NewUnavailableDockerHostInputStager()
	if err := stager.Probe(context.Background(), t.TempDir()); DockerHostInputStagingErrorCode(err) != DockerHostInputStagingErrorDisabled {
		t.Fatalf("unavailable host input stager error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := stager.Stage(ctx, HostInputBundleRequest{}); err == nil ||
		!strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("host input stager ignored cancellation: %v", err)
	}
}
