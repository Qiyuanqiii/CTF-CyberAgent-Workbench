package domain

import (
	"strings"
	"testing"
	"time"
)

func TestOperatorSteeringRequestNormalizesContentAndValidatesAuthority(t *testing.T) {
	request, err := (EnqueueOperatorSteeringRequest{
		RunID: "run-1", SessionID: "session-1", Content: "  first\r\nsecond  ",
		OperationKey: "operator-steering-operation-0001", RequestedBy: "operator",
	}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if request.Content != "first\nsecond" {
		t.Fatalf("content was not normalized: %q", request.Content)
	}
	for name, invalid := range map[string]EnqueueOperatorSteeringRequest{
		"empty content": {RunID: "run-1", SessionID: "session-1", Content: " ",
			OperationKey: "operator-steering-operation-0002", RequestedBy: "operator"},
		"oversized content": {RunID: "run-1", SessionID: "session-1",
			Content:      strings.Repeat("x", MaxOperatorSteeringContentBytes+1),
			OperationKey: "operator-steering-operation-0003", RequestedBy: "operator"},
		"short operation": {RunID: "run-1", SessionID: "session-1", Content: "input",
			OperationKey: "short", RequestedBy: "operator"},
		"control requester": {RunID: "run-1", SessionID: "session-1", Content: "input",
			OperationKey: "operator-steering-operation-0004", RequestedBy: "bad\nactor"},
	} {
		if _, err := invalid.Normalize(); err == nil {
			t.Fatalf("%s request unexpectedly validated", name)
		}
	}
}

func TestOperatorSteeringMessageRequiresMonotonicTerminalShape(t *testing.T) {
	now := time.Now().UTC()
	message := OperatorSteeringMessage{
		ID: "steer-1", RunID: "run-1", SessionID: "session-1", Sequence: 1,
		Status: OperatorSteeringPending, Content: "review the current result",
		ContentSHA256: OperatorSteeringContentSHA256("review the current result"),
		RequestedBy:   "operator", CreatedAt: now,
	}
	if err := message.Validate(); err != nil {
		t.Fatal(err)
	}
	message.Status = OperatorSteeringCommitted
	if err := message.Validate(); err == nil {
		t.Fatal("committed message without Session binding unexpectedly validated")
	}
	message.SessionMessageID = 1
	message.CommittedAt = &now
	if err := message.Validate(); err != nil {
		t.Fatal(err)
	}
}
