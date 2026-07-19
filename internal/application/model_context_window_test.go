package application

import (
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/llm"
)

func TestConstrainRequestToModelWindowTrimsOldestHistoryAndCapsOutput(t *testing.T) {
	window := llm.ContextWindow{
		ProtocolVersion: llm.ContextWindowProtocolVersion, WindowTokens: 4096,
		SafetyMarginTokens: 128, DefaultOutputTokens: 256, MaxOutputTokens: 512,
		Source: "test",
	}
	request := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "fixed"},
			{Role: "user", Content: strings.Repeat("旧", 900)},
			{Role: "assistant", Content: strings.Repeat("旧", 900)},
			{Role: "user", Content: "current"},
		},
		MaxTokens: 900,
	}
	bounded, plan, err := constrainRequestToModelWindow(request, window,
		modelContextLayout{HistoryStart: 1, HistoryCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	if bounded.MaxTokens != 512 || plan.OutputLimitTokens != 512 ||
		plan.HistoryOmitted != 1 || len(bounded.Messages) != 3 {
		t.Fatalf("unexpected bounded request: plan=%#v messages=%d max=%d",
			plan, len(bounded.Messages), bounded.MaxTokens)
	}
	if bounded.Messages[1].Role != "assistant" ||
		bounded.Metadata["context_history_omitted"] != "1" ||
		bounded.Metadata["context_window_source"] != "test" {
		t.Fatalf("oldest history was not removed deterministically: %#v", bounded)
	}
	if len(request.Messages) != 4 {
		t.Fatal("context fitting mutated the caller request")
	}
}

func TestConstrainRequestToModelWindowRejectsOversizedMandatoryContext(t *testing.T) {
	window := llm.ContextWindow{
		ProtocolVersion: llm.ContextWindowProtocolVersion, WindowTokens: 4096,
		SafetyMarginTokens: 128, DefaultOutputTokens: 256, MaxOutputTokens: 512,
		Source: "test",
	}
	_, _, err := constrainRequestToModelWindow(llm.ChatRequest{
		Messages: []llm.Message{{Role: "system", Content: strings.Repeat("界", 1400)}},
	}, window, modelContextLayout{})
	if apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("oversized mandatory context returned %v", err)
	}
}

func TestEstimateModelRequestTokensCountsToolSchemasAndUnicode(t *testing.T) {
	base := estimateModelRequestTokens(llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	})
	withUnicodeTool := estimateModelRequestTokens(llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
		Tools: []llm.ToolSpec{{Name: "note_create", Description: "记录说明",
			Parameters: []byte(`{"type":"object"}`)}},
	})
	if withUnicodeTool <= base+len([]byte("记录说明")) {
		t.Fatalf("tool and framing tokens were not included: base=%d tool=%d", base, withUnicodeTool)
	}
}
