package runmutation

import (
	"strings"
	"testing"
	"time"
)

func TestOperationFingerprintIsDomainSeparatedAndStable(t *testing.T) {
	first := OperationKeyDigest("note_create", "run-1", "call-1")
	if first != OperationKeyDigest("note_create", "run-1", "call-1") || len(first) != 64 {
		t.Fatalf("operation digest is not stable: %q", first)
	}
	for _, changed := range []string{
		OperationKeyDigest("work_item_create", "run-1", "call-1"),
		OperationKeyDigest("note_create", "run-2", "call-1"),
		OperationKeyDigest("note_create", "run-1", "call-2"),
	} {
		if changed == first {
			t.Fatal("operation digest ignored a domain component")
		}
	}
}

func TestOperationValidation(t *testing.T) {
	operation := Operation{
		KeyDigest: Fingerprint("key"), RequestFingerprint: Fingerprint("request"),
		InvocationID: "toolcall-1", RunID: "run-1", SessionID: "sess-1", WorkspaceID: "ws-1",
		ToolName: "note_create", TargetKind: TargetNote, TargetID: "note-1", RequestedBy: "root",
		CreatedAt: time.Now().UTC(),
	}
	if err := operation.Validate(); err != nil {
		t.Fatal(err)
	}
	operation.KeyDigest = strings.Repeat("z", 64)
	if err := operation.Validate(); err == nil {
		t.Fatal("expected malformed digest rejection")
	}
}
