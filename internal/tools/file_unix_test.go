//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadFileToolRejectsFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "blocked.pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	result, err := NewReadFileTool(root).Run(context.Background(), Call{
		Args: map[string]string{"path": "blocked.pipe"},
	})
	if err == nil || !strings.Contains(result.Stderr, "regular file") {
		t.Fatalf("FIFO was not rejected: result=%#v err=%v", result, err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("FIFO rejection blocked for %s", time.Since(started))
	}
}
