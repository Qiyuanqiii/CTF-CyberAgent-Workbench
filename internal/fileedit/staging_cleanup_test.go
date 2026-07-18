package fileedit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupStaleStagingRemovesOnlyMatchingOldInternalFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	stale := filepath.Join(root, stagingFilePrefix+"stale")
	fresh := filepath.Join(root, stagingFilePrefix+"fresh")
	foreign := filepath.Join(root, stagingFilePrefix+"foreign")
	for path, content := range map[string]string{
		stale: "replacement\n", fresh: "replacement\n", foreign: "different\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := now.Add(-StagingCleanupGrace - time.Minute)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(foreign, old, old); err != nil {
		t.Fatal(err)
	}

	result, err := CleanupStaleStaging(root, "target.txt", HashText("replacement\n"), now)
	if err != nil || result.Removed != 1 || !result.Pending {
		t.Fatalf("first cleanup=%#v err=%v", result, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("matching stale staging remains: %v", err)
	}
	for _, path := range []string{fresh, foreign} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected staging %s: %v", filepath.Base(path), err)
		}
	}

	if err := os.Chtimes(fresh, old, old); err != nil {
		t.Fatal(err)
	}
	result, err = CleanupStaleStaging(root, "target.txt", HashText("replacement\n"), now)
	if err != nil || result.Removed != 1 || result.Pending {
		t.Fatalf("second cleanup=%#v err=%v", result, err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign staging content was removed: %v", err)
	}
}
