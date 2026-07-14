package store

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

func TestREADMEListsEverySchemaVersionInOrder(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate README history test")
	}
	readmePath := filepath.Join(filepath.Dir(filename), "..", "..", "README.md")
	contents, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	rows := regexp.MustCompile(`(?m)^\| v([0-9]+) \|`).FindAllSubmatch(contents, -1)
	if len(rows) != LatestSchemaVersion {
		t.Fatalf("README schema history has %d rows, want %d", len(rows), LatestSchemaVersion)
	}
	for index, row := range rows {
		version, err := strconv.Atoi(string(row[1]))
		if err != nil {
			t.Fatalf("parse README schema row %d: %v", index+1, err)
		}
		if want := index + 1; version != want {
			t.Fatalf("README schema history row %d is v%d, want v%d", index+1, version, want)
		}
	}
}
