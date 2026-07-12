package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAgentInboxPayloadProtocolsDecodeStrictly(t *testing.T) {
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
