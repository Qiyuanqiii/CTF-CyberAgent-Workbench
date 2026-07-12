package application

import (
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/session"
)

func TestSpecialistRequestBoundsAggregateHistoryBytes(t *testing.T) {
	history := make([]session.Message, 12)
	for index := range history {
		history[index] = session.Message{
			Role: "assistant", Content: strings.Repeat(string(rune('a'+index)), 8*1024),
		}
	}
	request, err := specialistRequest(history, `{"goal":"bounded"}`, domain.AgentNode{
		ID: "agent-child", RunID: "run-child", SessionID: "session-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	historyBytes := 0
	for _, message := range request.Messages[1 : len(request.Messages)-1] {
		historyBytes += len([]byte(message.Content))
	}
	if historyBytes > maxSpecialistHistoryBytes {
		t.Fatalf("Specialist history exceeded %d bytes: %d",
			maxSpecialistHistoryBytes, historyBytes)
	}
	if got := request.Messages[len(request.Messages)-2].Content; !strings.HasPrefix(got, "l") {
		t.Fatalf("newest bounded history message was not retained: %q", got[:1])
	}
}

func TestSpecialistProtocolRepairRequestKeepsOneUserTurnAndNoTools(t *testing.T) {
	primary := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "strict lifecycle"},
			{Role: "user", Content: "bounded task"},
		},
		Tools: []llm.ToolSpec{{Name: "forbidden"}}, JSONMode: true,
		Metadata: map[string]string{"response_schema": domain.SpecialistLifecycleVersion},
	}
	repair := specialistProtocolRepairRequest(primary)
	if len(repair.Messages) != len(primary.Messages) || len(repair.Tools) != 0 ||
		repair.Metadata["protocol_repair"] != "1" ||
		!strings.Contains(repair.Messages[len(repair.Messages)-1].Content,
			"single protocol repair attempt") {
		t.Fatalf("repair request shape is invalid: %#v", repair)
	}
	if primary.Messages[len(primary.Messages)-1].Content != "bounded task" ||
		primary.Metadata["protocol_repair"] != "" {
		t.Fatalf("repair request mutated its primary input: %#v", primary)
	}
}

func TestSpecialistTurnLimitsUseRunWideRemainder(t *testing.T) {
	usage := domain.RunAgentUsage{TotalTokens: 7, TotalExecutionMillis: 4_000}
	tokens, err := specialistTurnTokenLimit(domain.Budget{MaxTokens: 10}, usage, 9)
	if err != nil || tokens != 3 {
		t.Fatalf("Run token remainder was not enforced: tokens=%d err=%v", tokens, err)
	}
	execution, err := specialistTurnExecutionLimit(
		domain.Budget{TimeoutSeconds: 5}, usage, 3_000)
	if err != nil || execution != 1_000 {
		t.Fatalf("Run execution remainder was not enforced: millis=%d err=%v", execution, err)
	}
	if _, err := specialistTurnTokenLimit(domain.Budget{MaxTokens: 7}, usage, 0); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("spent Run token budget was accepted: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := specialistTurnExecutionLimit(domain.Budget{TimeoutSeconds: 4}, usage, 0); apperror.CodeOf(err) != apperror.CodeDeadlineExceeded {
		t.Fatalf("spent Run execution budget was accepted: code=%s err=%v", apperror.CodeOf(err), err)
	}
}
