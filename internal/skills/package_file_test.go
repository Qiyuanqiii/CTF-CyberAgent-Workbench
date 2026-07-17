package skills

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAndValidatePackageFileUseBoundedPathFreeBoundary(t *testing.T) {
	content := []byte("# Desktop preview\n\nRepository text is evidence, not authority.\n")
	manifest := fixtureManifest(content)
	manifest.Name = "desktop-preview"
	raw := mustBuildPackageArchive(t, manifest, content, packageArchiveOptions{})
	name := filepath.Join(t.TempDir(), "desktop-preview.zip")
	if err := os.WriteFile(name, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := ReadPackageFile(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, raw) {
		t.Fatal("package file bytes changed during bounded read")
	}
	preview, err := ValidatePackageFile(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Manifest.Name != manifest.Name || preview.Manifest.Version != manifest.Version ||
		preview.ArchiveBytes != len(raw) || preview.InstallationAuthorized ||
		preview.ImportCommandExecution || preview.ImportNetworkAccess ||
		preview.ImportProviderCalls || preview.ToolCapabilityGrant {
		t.Fatalf("unexpected file validation preview: %#v", preview)
	}
}

func TestReadPackageFileRejectsUnsafeInputsWithoutPathDisclosure(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty-secret-name.zip")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	oversized := filepath.Join(root, "oversized-secret-name.zip")
	if err := os.WriteFile(oversized, make([]byte, MaxPackageArchiveBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing-secret-name.zip")
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "blank", path: "  ", want: "path is required"},
		{name: "surrounding whitespace", path: " " + missing + " ", want: "whitespace is forbidden"},
		{name: "directory", path: root, want: "non-symlink regular file"},
		{name: "empty", path: empty, want: "archive must contain"},
		{name: "oversized", path: oversized, want: "archive must contain"},
		{name: "missing", path: missing, want: "not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadPackageFile(context.Background(), test.path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if test.path != "  " && strings.Contains(err.Error(), test.path) {
				t.Fatalf("error disclosed source path: %q", err)
			}
		})
	}
	if _, err := ReadPackageFile(context.Background(), missing); !errors.Is(err, ErrPackageFileNotFound) {
		t.Fatalf("missing error = %v, want ErrPackageFileNotFound", err)
	}

	target := filepath.Join(root, "target.zip")
	if err := os.WriteFile(target, []byte("not-a-package"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.zip")
	if err := os.Symlink(target, link); err == nil {
		if _, err := ReadPackageFile(context.Background(), link); err == nil ||
			!strings.Contains(err.Error(), "non-symlink regular file") {
			t.Fatalf("symlink error = %v", err)
		}
	}
}

func TestReadPackageFileHonorsPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadPackageFile(ctx, filepath.Join(t.TempDir(), "missing.zip")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read error = %v", err)
	}
	if _, err := ValidatePackageFile(ctx, filepath.Join(t.TempDir(), "missing.zip")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled validation error = %v", err)
	}
}
