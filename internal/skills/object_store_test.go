package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/domain"
)

func TestLocalPackageObjectStorePublishesAndVerifiesOneImmutableObject(t *testing.T) {
	raw, descriptor := objectStoreFixture(t)
	home := t.TempDir()
	const workers = 12
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			objects, err := NewLocalPackageObjectStore(home)
			if err != nil {
				results <- err
				return
			}
			receipt, putErr := objects.Put(context.Background(), raw, descriptor)
			if putErr == nil && receipt.ObjectKey == "" {
				putErr = os.ErrInvalid
			}
			results <- putErr
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	objects, err := NewLocalPackageObjectStore(home)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := objects.Verify(context.Background(), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(home, filepath.FromSlash(PackageObjectRoot),
		"sha256", descriptor.ArchiveSHA256[:2]))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(filepath.FromSlash(receipt.ObjectKey)) {
		t.Fatalf("content-addressed directory entries = %v", entries)
	}
}

func TestLocalPackageObjectStoreRejectsSymlinkDirectory(t *testing.T) {
	raw, descriptor := objectStoreFixture(t)
	home := t.TempDir()
	redirect := filepath.Join(home, "redirect")
	if err := os.Mkdir(redirect, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("redirect", filepath.Join(home, "skill-registry")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	objects, err := NewLocalPackageObjectStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := objects.Put(context.Background(), raw, descriptor); err == nil ||
		!strings.Contains(err.Error(), "directory") {
		t.Fatalf("symlink directory error = %v", err)
	}
	entries, err := os.ReadDir(redirect)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink redirect received object-store entries: %v", entries)
	}
}

func TestLocalPackageObjectStoreRejectsCorruptionAndCancelledWrites(t *testing.T) {
	raw, descriptor := objectStoreFixture(t)
	home := t.TempDir()
	objects, err := NewLocalPackageObjectStore(home)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := objects.Put(cancelled, raw, descriptor); err == nil {
		t.Fatal("cancelled object write succeeded")
	}
	if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(PackageObjectRoot))); !os.IsNotExist(err) {
		t.Fatalf("cancelled write created the object tree: %v", err)
	}
	receipt, err := objects.Put(context.Background(), raw, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(home, filepath.FromSlash(PackageObjectRoot),
		filepath.FromSlash(receipt.ObjectKey))
	changed := append([]byte(nil), raw...)
	changed[len(changed)/2] ^= 0xff
	if err := os.WriteFile(objectPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := objects.Verify(context.Background(), descriptor); err == nil ||
		!strings.Contains(err.Error(), "digest") {
		t.Fatalf("corrupt object error = %v", err)
	}
	if _, err := objects.Put(context.Background(), raw, descriptor); err == nil {
		t.Fatal("Put silently replaced a corrupt published object")
	}
}

func TestLocalPackageObjectStoreRejectsSymlinkObject(t *testing.T) {
	raw, descriptor := objectStoreFixture(t)
	home := t.TempDir()
	objects, err := NewLocalPackageObjectStore(home)
	if err != nil {
		t.Fatal(err)
	}
	objectKey, err := PackageObjectKey(descriptor.ArchiveSHA256)
	if err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(home, filepath.FromSlash(PackageObjectRoot),
		filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "outside.zip")
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, objectPath); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if _, err := objects.Verify(context.Background(), descriptor); err == nil ||
		!strings.Contains(err.Error(), "identity") {
		t.Fatalf("symlink object error = %v", err)
	}
}

func objectStoreFixture(t *testing.T) ([]byte, PackageObjectDescriptor) {
	t.Helper()
	content := []byte("# Object store package\n")
	manifest := fixtureManifest(content)
	manifest.Name = "object-store-package"
	manifest.Profiles = []domain.Profile{domain.ProfileReview}
	raw := mustBuildPackageArchive(t, manifest, content, packageArchiveOptions{})
	parsed, err := ParsePackage(raw)
	if err != nil {
		t.Fatal(err)
	}
	preview := parsed.Preview()
	return raw, PackageObjectDescriptor{
		ProtocolVersion:    PackageObjectProtocolVersion,
		ArchiveSHA256:      preview.ArchiveSHA256,
		PackageFingerprint: preview.PackageFingerprint,
		ArchiveBytes:       preview.ArchiveBytes,
	}
}
