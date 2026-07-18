package buildinfo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPortableDiagnosticUsesReproducibleContentFreeMetadata(t *testing.T) {
	originalVersion, originalRevision := Version, Revision
	originalEpoch, originalModified, originalCGO := SourceDateEpoch, Modified, CGOEnabled
	t.Cleanup(func() {
		Version, Revision = originalVersion, originalRevision
		SourceDateEpoch, Modified, CGOEnabled = originalEpoch, originalModified, originalCGO
	})
	Version = "v9.9.9"
	Revision = strings.Repeat("a", 40)
	SourceDateEpoch = "1700000000"
	Modified = "false"
	CGOEnabled = "1"

	first := PortableDiagnostic()
	second := PortableDiagnostic()
	if first.ProtocolVersion != DiagnosticProtocolVersion ||
		first.Release.AppVersion != Version || first.Release.Revision != Revision ||
		first.Release.SourceDateEpoch != 1700000000 || first.Release.SourceDate == "" ||
		first.Release.Modified || !first.Release.ModifiedKnown ||
		first.Release.CGOEnabled != "1" || first.Release.BuildFingerprint == "" ||
		first.Release.BuildFingerprint != second.Release.BuildFingerprint ||
		first.Release.InstallerIncluded || first.Release.RegistryWrites ||
		first.Release.AutoUpdateEnabled || !first.Release.ManualWindows10Matrix {
		t.Fatalf("diagnostic=%#v", first)
	}
	if first.ReleaseReady {
		t.Fatal("release must remain unready until the manual Windows 10 matrix is signed")
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"root_path", "api_key", "registry_key", "C:\\"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, encoded)
		}
	}
}
