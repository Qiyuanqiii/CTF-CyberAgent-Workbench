package application

import (
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
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
