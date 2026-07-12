package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAgentInboxPayloadProtocolsDecodeStrictly(t *testing.T) {
	instructionJSON, err := json.Marshal(AgentInstructionPayload{
		Version: SpecialistInstructionVersion, Instruction: "review the assigned child work",
	})
	if err != nil {
		t.Fatal(err)
	}
	instruction, err := DecodeAgentInstructionPayload(string(instructionJSON))
	if err != nil || instruction.Instruction != "review the assigned child work" {
		t.Fatalf("Specialist instruction did not decode: payload=%#v err=%v", instruction, err)
	}
	for _, invalid := range []string{
		`{"version":"specialist_instruction.v2","instruction":"review"}`,
		`{"version":"specialist_instruction.v1","instruction":""}`,
		`{"version":"specialist_instruction.v1","instruction":"review","sender_agent_id":"forged"}`,
	} {
		if _, err := DecodeAgentInstructionPayload(invalid); err == nil {
			t.Fatalf("Specialist instruction accepted invalid payload %s", invalid)
		}
	}

	completionJSON, err := json.Marshal(AgentCompletionInboxPayload{
		CompletionReportID: "completion-1", AgentID: "agent-child",
		Report: CompletionReport{
			Version: CompletionReportVersion, Outcome: CompletionSucceeded,
			Summary: "review complete", WorkItemIDs: []string{}, NoteIDs: []string{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	completion, err := DecodeAgentCompletionInboxPayload(string(completionJSON))
	if err != nil || completion.AgentID != "agent-child" ||
		completion.Report.Outcome != CompletionSucceeded {
		t.Fatalf("completion inbox payload did not decode: payload=%#v err=%v", completion, err)
	}
	if _, err := DecodeAgentCompletionInboxPayload(`{"completion_report_id":"completion-1","agent_id":"agent-child","report":{"version":"agent_completion.v1","outcome":"succeeded","summary":"done","work_item_ids":[],"note_ids":[]},"cursor":"forged"}`); err == nil {
		t.Fatal("completion inbox payload accepted a model-controlled cursor")
	}

	failureJSON, err := json.Marshal(AgentAttemptFailurePayload{
		Version: AgentAttemptFailureVersion, AgentID: "agent-child", AttemptID: "attempt-1",
		FailureCode: "worker_lost", Reason: "worker lease expired", RetryScheduled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	failure, err := DecodeAgentAttemptFailurePayload(string(failureJSON))
	if err != nil || failure.AttemptID != "attempt-1" || failure.FailureCode != "worker_lost" {
		t.Fatalf("failure inbox payload did not decode: payload=%#v err=%v", failure, err)
	}
	if _, err := DecodeAgentAttemptFailurePayload(`{"version":"agent_attempt_failure.v1","agent_id":"agent-child","attempt_id":"attempt-1","failure_code":"worker_lost","reason":"lost","retry_scheduled":true,"recovered":false,"sender_agent_id":"forged"}`); err == nil {
		t.Fatal("failure inbox payload accepted a forged sender")
	}
}

func TestSpecialistContextBatchRequiresDirectOrderedPendingInstructions(t *testing.T) {
	now := time.Now().UTC()
	payload, err := json.Marshal(AgentInstructionPayload{
		Version: SpecialistInstructionVersion, Instruction: "inspect owned evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := AgentMessage{
		ID: "message-1", RunID: "run-1", SenderAgentID: "agent-root",
		RecipientAgentID: "agent-child", Sequence: 1, Kind: AgentMessageInstruction,
		Semantic: AgentMessageSemanticMessage, PayloadJSON: string(payload),
		Status: AgentMessagePending, CreatedAt: now,
	}
	batch := SpecialistContextBatch{
		RunID: "run-1", AgentID: "agent-child", ParentAgentID: "agent-root",
		AgentAttemptID: "attempt-child", Turn: 1, Messages: []AgentMessage{message},
		PreparedAt: now,
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("valid Specialist context was rejected: %v", err)
	}
	wrongSender := batch
	wrongSender.Messages = append([]AgentMessage(nil), batch.Messages...)
	wrongSender.Messages[0].SenderAgentID = "agent-sibling"
	if err := wrongSender.Validate(); err == nil {
		t.Fatal("Specialist context accepted a sibling instruction")
	}
	wrongKind := batch
	wrongKind.Messages = append([]AgentMessage(nil), batch.Messages...)
	wrongKind.Messages[0].Kind = AgentMessageNotification
	if err := wrongKind.Validate(); err == nil {
		t.Fatal("Specialist context accepted a non-instruction message")
	}
}

func TestRootInboxContextBatchRequiresBoundedOrderedPendingMessages(t *testing.T) {
	now := time.Now().UTC()
	message := AgentMessage{
		ID: "message-1", RunID: "run-1", SenderAgentID: "agent-child",
		RecipientAgentID: "agent-root", Sequence: 1, Kind: AgentMessageNotification,
		Semantic:    AgentMessageSemanticDependency,
		PayloadJSON: `{"dependency_id":"work-1","state":"satisfied","reason":"done"}`,
		Status:      AgentMessagePending, CreatedAt: now,
	}
	batch := RootInboxContextBatch{
		RunID: "run-1", RootAgentID: "agent-root", SupervisorAttemptID: "attempt-root",
		Turn: 1, Messages: []AgentMessage{message}, PreparedAt: now,
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("valid root inbox context was rejected: %v", err)
	}
	duplicate := batch
	duplicate.Messages = []AgentMessage{message, message}
	duplicate.Messages[1].Sequence = 2
	if err := duplicate.Validate(); err == nil {
		t.Fatal("root inbox context accepted a duplicate message identity")
	}
	consumed := batch
	consumed.Messages = append([]AgentMessage(nil), batch.Messages...)
	consumed.Messages[0].Status = AgentMessageConsumed
	consumedAt := now
	consumed.Messages[0].ConsumedAt = &consumedAt
	if err := consumed.Validate(); err == nil {
		t.Fatal("root inbox context accepted an already consumed message")
	}
}
