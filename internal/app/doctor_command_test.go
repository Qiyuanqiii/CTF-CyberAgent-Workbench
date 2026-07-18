package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/buildinfo"
)

func TestDoctorPortableIsReadOnlyAndMachineReadable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"doctor", "portable", "--json"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var diagnostic buildinfo.Diagnostic
	if err := json.Unmarshal(stdout.Bytes(), &diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic.ProtocolVersion != buildinfo.DiagnosticProtocolVersion ||
		diagnostic.Release.BuildFingerprint == "" ||
		diagnostic.Release.InstallerIncluded || diagnostic.Release.RegistryWrites ||
		diagnostic.Release.AutoUpdateEnabled || !diagnostic.Release.ManualWindows10Matrix {
		t.Fatalf("diagnostic=%#v", diagnostic)
	}
	if _, err := os.Stat(filepath.Join(home, "cyberagent.db")); !os.IsNotExist(err) {
		t.Fatalf("doctor created runtime state: %v", err)
	}
	if strings.Contains(stdout.String(), home) {
		t.Fatal("doctor exposed the local home path")
	}
}

func TestDoctorPortableTextIncludesManualMatrixBoundary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"doctor", "portable"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "windows_10_runtime_matrix: manual") ||
		!strings.Contains(stdout.String(), "fingerprint:") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}
