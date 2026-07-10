package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/store"
)

func TestWorkspaceInitCreatesExpectedLayout(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	mgr := NewManager(home, st)
	rec, err := mgr.Init(context.Background(), "Demo Workspace")
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{"attachments", "scripts", "outputs", "logs", "writeups", filepath.Join("tests", "sample_input")} {
		path := filepath.Join(rec.RootPath, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", path)
		}
	}
}
