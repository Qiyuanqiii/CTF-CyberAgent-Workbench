package contextmgr

import (
	"context"
	"strings"
	"testing"
)

type memoryStore struct {
	summaries []Summary
}

func (m *memoryStore) SaveContextSummary(ctx context.Context, summary Summary) (Summary, error) {
	summary.ID = int64(len(m.summaries) + 1)
	m.summaries = append(m.summaries, summary)
	return summary, nil
}

func (m *memoryStore) LatestContextSummary(ctx context.Context, taskID string) (Summary, bool, error) {
	for i := len(m.summaries) - 1; i >= 0; i-- {
		if m.summaries[i].TaskID == taskID {
			return m.summaries[i], true, nil
		}
	}
	return Summary{}, false, nil
}

func TestCompactPreservesRecentMessagesAndStoresSummary(t *testing.T) {
	store := &memoryStore{}
	manager := NewManager(store, Config{MaxMessagesBeforeCompact: 4, PreserveRecentMessages: 2, MaxSummaryChars: 1000, MaxLineChars: 120})
	messages := []Message{
		{Role: "user", Content: "imported a web challenge"},
		{Role: "assistant", Content: "classified it as Flask session work"},
		{Role: "tool", Content: "read app.py"},
		{Role: "user", Content: "asked for exploit plan"},
		{Role: "assistant", Content: "proposed scoped cookie signing check"},
	}

	result, err := manager.Compact(context.Background(), "task-1", "ws-demo", messages)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compacted {
		t.Fatal("expected compaction")
	}
	if result.RemovedMessages != 3 {
		t.Fatalf("expected 3 removed messages, got %d", result.RemovedMessages)
	}
	if len(result.Preserved) != 2 || result.Preserved[0].Content != "asked for exploit plan" {
		t.Fatalf("unexpected preserved messages: %#v", result.Preserved)
	}
	if !strings.Contains(result.Summary.Content, "Flask session") {
		t.Fatalf("summary missed older context: %s", result.Summary.Content)
	}
	latest, ok, err := manager.Latest(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || latest.ID != result.Summary.ID {
		t.Fatalf("latest summary not stored")
	}
}

func TestMaybeCompactSkipsBelowThreshold(t *testing.T) {
	manager := NewManager(nil, Config{MaxMessagesBeforeCompact: 3, PreserveRecentMessages: 1})
	result, err := manager.MaybeCompact(context.Background(), "task-1", "", []Message{
		{Role: "user", Content: "short"},
		{Role: "assistant", Content: "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Compacted {
		t.Fatal("did not expect compaction below threshold")
	}
	if len(result.Preserved) != 2 {
		t.Fatalf("expected messages to be preserved, got %d", len(result.Preserved))
	}
}

func TestCompactSmallConversationStillWritesSummary(t *testing.T) {
	manager := NewManager(nil, DefaultConfig())
	result, err := manager.Compact(context.Background(), "task-small", "", []Message{
		{Role: "user", Content: "short but important"},
		{Role: "assistant", Content: "acknowledged"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedMessages != 1 {
		t.Fatalf("expected one message to be summarized, got %d", result.RemovedMessages)
	}
	if !strings.Contains(result.Summary.Content, "short but important") {
		t.Fatalf("expected content in summary: %s", result.Summary.Content)
	}
}

func TestParseMessageRecognizesRoles(t *testing.T) {
	msg := ParseMessage("assistant: hello")
	if msg.Role != "assistant" || msg.Content != "hello" {
		t.Fatalf("unexpected parsed message: %#v", msg)
	}
	unknown := ParseMessage("operator: run it")
	if unknown.Role != "user" || unknown.Content != "operator: run it" {
		t.Fatalf("unknown prefix should stay in content: %#v", unknown)
	}
}

func TestCompactAndPromptRedactSecrets(t *testing.T) {
	manager := NewManager(nil, Config{MaxMessagesBeforeCompact: 2, PreserveRecentMessages: 1, MaxSummaryChars: 1000, MaxLineChars: 240})
	mimoToken := "t" + "p-" + strings.Repeat("a", 40)
	openAIToken := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz123456"
	openAIPrefix := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz"
	messages := []Message{
		{Role: "user", Content: "MIMO_API_KEY=" + mimoToken},
		{Role: "assistant", Content: "stored observation"},
		{Role: "user", Content: "OPENAI_API_KEY=" + openAIToken},
	}
	result, err := manager.Compact(context.Background(), "task-redact", "ws-demo", messages)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{result.Summary.Content, result.Preserved[0].Content} {
		if strings.Contains(value, mimoToken[:11]) || strings.Contains(value, openAIPrefix) {
			t.Fatalf("secret was not redacted: %q", value)
		}
		if !strings.Contains(value, "[REDACTED:secret]") {
			t.Fatalf("expected redaction marker in %q", value)
		}
	}
	prompt := manager.BuildPrompt("system", Summary{Content: "TOKEN=verysecretvalue"}, messages)
	for _, msg := range prompt {
		if strings.Contains(msg.Content, "verysecretvalue") || strings.Contains(msg.Content, mimoToken[:11]) {
			t.Fatalf("secret reached prompt: %#v", prompt)
		}
	}
}
