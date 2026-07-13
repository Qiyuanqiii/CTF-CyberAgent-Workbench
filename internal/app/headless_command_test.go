package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/headless"
)

func TestHeadlessEventsCLIEmitsResumableNDJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	created, stderr, code := executeTestCommand(t, "run", "create", "headless export")
	if code != 0 {
		t.Fatalf("run create failed: stderr=%s", stderr)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("run id missing: %s", created)
	}

	stdout, stderr, code := executeTestCommand(t, "headless", "events", runID,
		"--max-events", "100")
	if code != 0 || stderr != "" {
		t.Fatalf("headless snapshot failed: code=%d stderr=%s stdout=%s",
			code, stderr, stdout)
	}
	records := decodeHeadlessCLIRecords(t, stdout)
	if len(records) < 2 {
		t.Fatalf("headless snapshot omitted records: %#v", records)
	}
	for index, record := range records[:len(records)-1] {
		if record.Kind != headless.EventRecordKind || record.Version != headless.ProtocolVersion ||
			record.Sequence != int64(index+1) || record.RunID != runID {
			t.Fatalf("headless event record drifted: %#v", record)
		}
	}
	end := records[len(records)-1]
	if end.Kind != headless.EndRecordKind || end.Status != "created" || end.Terminal ||
		end.Reason != "snapshot" || end.ExitCode != 0 ||
		end.EventsEmitted != len(records)-1 || end.LastSequence != int64(len(records)-1) {
		t.Fatalf("headless snapshot end drifted: %#v", end)
	}

	stdout, stderr, code = executeTestCommand(t, "headless", "events", runID,
		"--after-sequence", "1", "--max-events", "100")
	if code != 0 || stderr != "" {
		t.Fatalf("headless resume failed: code=%d stderr=%s", code, stderr)
	}
	resumed := decodeHeadlessCLIRecords(t, stdout)
	if len(resumed) != len(records)-1 || resumed[0].Sequence != 2 ||
		resumed[len(resumed)-1].AfterSequence != 1 {
		t.Fatalf("headless resume drifted: %#v", resumed)
	}
}

func TestHeadlessEventsCLIUsesStableBoundaryAndTerminalExitCodes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	created, stderr, code := executeTestCommand(t, "run", "create", "headless bounds")
	if code != 0 {
		t.Fatal(stderr)
	}
	runID := runIDPattern.FindString(created)

	stdout, stderr, code := executeTestCommand(t, "headless", "events", runID,
		"--max-events", "1")
	if code != 8 || !strings.Contains(stderr, "headless event limit was reached") {
		t.Fatalf("headless limit exit drifted: code=%d stderr=%s stdout=%s",
			code, stderr, stdout)
	}
	limited := decodeHeadlessCLIRecords(t, stdout)
	end := limited[len(limited)-1]
	if len(limited) != 2 || end.Kind != headless.EndRecordKind || !end.HasMore ||
		!end.Truncated || end.ExitCode != 8 || end.SuggestedResume != 1 {
		t.Fatalf("headless limit record drifted: %#v", limited)
	}

	stdout, stderr, code = executeTestCommand(t, "headless", "events", runID,
		"--after-sequence", "999")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "beyond the durable event tail") {
		t.Fatalf("future sequence exit drifted: code=%d stderr=%s stdout=%s",
			code, stderr, stdout)
	}

	if _, stderr, code = executeTestCommand(t, "run", "cancel", runID); code != 0 {
		t.Fatalf("run cancel failed: %s", stderr)
	}
	stdout, stderr, code = executeTestCommand(t, "headless", "events", runID)
	if code != 7 || !strings.Contains(stderr, "cancelled status") {
		t.Fatalf("cancelled Run exit drifted: code=%d stderr=%s stdout=%s",
			code, stderr, stdout)
	}
	cancelled := decodeHeadlessCLIRecords(t, stdout)
	end = cancelled[len(cancelled)-1]
	if end.Status != "cancelled" || !end.Terminal || end.ExitCode != 7 {
		t.Fatalf("cancelled Run end record drifted: %#v", end)
	}
}

func TestHeadlessEventsCLIFollowTimeoutAndFailFastValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	_, stderr, code := executeTestCommand(t, "headless", "events", "run-any",
		"--timeout", "1s")
	if code != 2 || !strings.Contains(stderr, "timeout requires --follow") {
		t.Fatalf("timeout validation drifted: code=%d stderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, "cyberagent.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid headless flags opened runtime state: %v", err)
	}

	created, stderr, code := executeTestCommand(t, "run", "create", "headless follow")
	if code != 0 {
		t.Fatal(stderr)
	}
	runID := runIDPattern.FindString(created)
	stdout, stderr, code := executeTestCommand(t, "headless", "events", runID,
		"--follow", "--timeout", "70ms", "--poll-interval", "50ms")
	if code != 9 || !strings.Contains(stderr, "follow deadline exceeded") {
		t.Fatalf("follow deadline exit drifted: code=%d stderr=%s stdout=%s",
			code, stderr, stdout)
	}
	records := decodeHeadlessCLIRecords(t, stdout)
	end := records[len(records)-1]
	if end.Reason != "deadline_exceeded" || end.ExitCode != 9 || end.Terminal {
		t.Fatalf("follow deadline record drifted: %#v", end)
	}
}

type headlessCLIRecord struct {
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	RunID           string `json:"run_id"`
	Sequence        int64  `json:"sequence"`
	Status          string `json:"status"`
	Terminal        bool   `json:"terminal"`
	Reason          string `json:"reason"`
	AfterSequence   int64  `json:"after_sequence"`
	LastSequence    int64  `json:"last_sequence"`
	EventsEmitted   int    `json:"events_emitted"`
	HasMore         bool   `json:"has_more"`
	Truncated       bool   `json:"truncated"`
	SuggestedResume int64  `json:"suggested_resume_after"`
	ExitCode        int    `json:"exit_code"`
}

func decodeHeadlessCLIRecords(t *testing.T, output string) []headlessCLIRecord {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewBufferString(output))
	var records []headlessCLIRecord
	for {
		var record headlessCLIRecord
		if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode headless NDJSON: %v\n%s", err, output)
		}
		records = append(records, record)
	}
	return records
}
