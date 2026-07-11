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

func TestSupervisorToolIdentityIsDeterministicAndTurnScoped(t *testing.T) {
	first := SupervisorToolOperationKey("run-1", 2, "note_create", `{"content":"x"}`)
	replayed := SupervisorToolOperationKey("run-1", 2, "note_create", `{"content":"x"}`)
	otherTurn := SupervisorToolOperationKey("run-1", 3, "note_create", `{"content":"x"}`)
	callID, err := SupervisorToolCallID(first, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first != replayed || first == otherTurn || len(callID) != len("toolu_")+24 {
		t.Fatalf("unexpected supervisor tool identity: first=%s replayed=%s other=%s call=%s",
			first, replayed, otherTurn, callID)
	}
	secondRoundID, err := SupervisorToolCallID(first, 2)
	if err != nil || secondRoundID == callID {
		t.Fatalf("supervisor tool call id did not separate rounds: first=%s second=%s err=%v",
			callID, secondRoundID, err)
	}
	if _, err := SupervisorToolCallID("raw-provider-id", 1); err == nil {
		t.Fatal("non-digest supervisor tool operation key was accepted")
	}
}
