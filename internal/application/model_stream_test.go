package application_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

func TestRunSupervisorAggregatesSplitUTF8Stream(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := &scriptedStreamProvider{name: "stream-test"}
	text := rootActionResponse(domain.RootActionContinue, "你好 streamed", "", "")
	boundary := strings.Index(text, "你") + 1
	usage := llm.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5}
	provider.streams = [][]llm.ChatChunk{{
		{Text: text[:boundary]},
		{Text: text[boundary : boundary+1]},
		{Text: text[boundary+1:]},
		finalStreamChunk(provider.name, "model", usage),
	}}
	run, supervisor := newStreamSupervisor(t, st, provider, domain.Budget{MaxTurns: 2})
	result, err := supervisor.Step(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if provider.chatCalls != 0 || provider.streamCalls != 1 || result.Text != "你好 streamed" ||
		result.StreamEvents != 1 || result.StreamBytes != len(text) || result.Checkpoint.TotalTokens != 5 {
		t.Fatalf("split UTF-8 stream was not aggregated: provider=%#v result=%#v", provider, result)
	}
}

func TestRunSupervisorBoundsModelDeltaEventsWithoutPersistingText(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := &scriptedStreamProvider{name: "stream-test"}
	message := "DELTA_SECRET_MARKER" + strings.Repeat("x", llm.MaxModelOutputBytes-300)
	text := rootActionResponse(domain.RootActionContinue, message, "", "")
	if len(text) > llm.MaxModelOutputBytes || len(text) <= (llm.MaxModelDeltaEvents-1)*2048 {
		t.Fatalf("test response length %d does not exercise the event ceiling", len(text))
	}
	repairedText := rootActionResponse(domain.RootActionContinue, "bounded repair", "", "")
	provider.streams = [][]llm.ChatChunk{
		chunkedStream(text, 2048, provider.name, "model", llm.Usage{InputTokens: 1, OutputTokens: 10, TotalTokens: 11}),
		chunkedStream(repairedText, 2048, provider.name, "model", llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}),
	}
	run, supervisor := newStreamSupervisor(t, st, provider, domain.Budget{MaxTurns: 2, MaxTokens: 100})
	result, err := supervisor.Step(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProtocolRepairs != 1 || result.StreamEvents != llm.MaxModelDeltaEvents+1 ||
		result.StreamBytes != len(text)+len(repairedText) || result.Text != "bounded repair" {
		t.Fatalf("stream event ceiling was not exact: %#v", result)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	deltaCount := 0
	finalCount := 0
	firstAttemptCount := 0
	for _, item := range items {
		if item.Type != events.ModelDeltaEvent {
			continue
		}
		deltaCount++
		if strings.Contains(item.PayloadJSON, `"model_attempt":1`) {
			firstAttemptCount++
		}
		if strings.Contains(item.PayloadJSON, "DELTA_SECRET_MARKER") || strings.Contains(item.PayloadJSON, `"text"`) {
			t.Fatalf("model delta persisted streamed text: %s", item.PayloadJSON)
		}
		if strings.Contains(item.PayloadJSON, `"done":true`) {
			finalCount++
		}
	}
	if deltaCount != llm.MaxModelDeltaEvents+1 || firstAttemptCount != llm.MaxModelDeltaEvents || finalCount != 2 {
		t.Fatalf("unexpected bounded delta stream count=%d first=%d final=%d", deltaCount, firstAttemptCount, finalCount)
	}
}

func TestRunSupervisorRetriesRetryableMidStreamFailure(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := &scriptedStreamProvider{name: "stream-test", streams: [][]llm.ChatChunk{
		{{Err: llm.NewProviderError(llm.OutcomeRetryable, "stream-test", "stream reset", nil)}},
		chunkedStream(rootActionResponse(domain.RootActionContinue, "stream recovered", "", ""), 8, "stream-test", "model", llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}),
	}}
	run, supervisor := newStreamSupervisor(t, st, provider, domain.Budget{MaxTurns: 2})
	supervisor.WithModelRetryPolicy(application.ModelRetryPolicy{MaxAttempts: 2})
	result, err := supervisor.Step(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if provider.streamCalls != 2 || provider.chatCalls != 0 || result.ModelAttempts != 2 ||
		result.ModelOutcome != llm.OutcomeSuccess || result.StreamEvents != 1 {
		t.Fatalf("retryable stream failure did not recover: provider=%#v result=%#v", provider, result)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelStartedEvent) != 2 || countEventType(items, events.ModelFailedEvent) != 1 ||
		countEventType(items, events.ModelCompletedEvent) != 1 {
		t.Fatalf("unexpected stream retry events: %#v", items)
	}
}

func TestRunSupervisorRejectsMalformedStreamBoundaries(t *testing.T) {
	usage := llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}
	tests := []struct {
		name       string
		chunks     []llm.ChatChunk
		wantEvents int
	}{
		{name: "oversized", chunks: []llm.ChatChunk{{Text: strings.Repeat("x", llm.MaxModelOutputBytes+1)}}},
		{name: "invalid utf8", chunks: []llm.ChatChunk{{Text: string([]byte{0xff, 0xfe})}, finalStreamChunk("stream-test", "model", usage)}, wantEvents: 1},
		{name: "missing usage", chunks: []llm.ChatChunk{{Text: `{"version":"root_lifecycle.v1"}`}, {Done: true}}, wantEvents: 1},
		{name: "missing done", chunks: []llm.ChatChunk{{Text: `{"version":"root_lifecycle.v1"}`}}, wantEvents: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			provider := &scriptedStreamProvider{name: "stream-test", streams: [][]llm.ChatChunk{test.chunks}}
			run, supervisor := newStreamSupervisor(t, st, provider, domain.Budget{MaxTurns: 2})
			result, err := supervisor.Step(context.Background(), run.ID)
			if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || result.ModelOutcome != llm.OutcomeInvalidResponse ||
				result.ProtocolRepairs != 0 || result.StreamEvents != test.wantEvents || result.Checkpoint.Phase != domain.SupervisorTurnFailed {
				t.Fatalf("malformed stream was not rejected: result=%#v code=%s err=%v", result, apperror.CodeOf(err), err)
			}
			messages, listErr := st.ListSessionMessages(context.Background(), run.SessionID, true)
			if listErr != nil || len(messages) != 0 {
				t.Fatalf("malformed stream wrote messages=%#v err=%v", messages, listErr)
			}
		})
	}
}

func TestRunSupervisorResumesAfterMidStreamCancellation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := &cancelThenStreamProvider{name: "cancel-stream"}
	run, supervisor := newStreamSupervisor(t, st, provider, domain.Budget{MaxTurns: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	first, err := supervisor.Step(ctx, run.ID)
	if apperror.CodeOf(err) != apperror.CodeDeadlineExceeded || first.Checkpoint.Phase != domain.SupervisorTurnStarted ||
		first.StreamEvents != 1 || first.StreamBytes != len("partial stream") || first.ModelOutcome != llm.OutcomeCancelled {
		t.Fatalf("mid-stream cancellation was not recoverable: result=%#v code=%s err=%v", first, apperror.CodeOf(err), err)
	}
	resumed, err := supervisor.Step(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Recovered || resumed.ModelAttempts != 2 || resumed.Text != "resumed stream" || provider.calls != 2 {
		t.Fatalf("cancelled stream did not resume: provider=%#v result=%#v", provider, resumed)
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.ModelDeltaEvent) != 2 || countEventType(items, events.ModelFailedEvent) != 1 ||
		countEventType(items, events.ModelCompletedEvent) != 1 || countEventType(items, events.AgentTurnStartedEvent) != 1 {
		t.Fatalf("cancel/resume stream events were inconsistent: %#v", items)
	}
}

func TestSupervisorModelDeltaLedgerIsOrderedAndIdempotent(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run := newStartedRunForProvider(t, st, "stream-test", domain.Budget{MaxTurns: 2})
	turn, err := st.BeginSupervisorTurn(context.Background(), run.ID, "delta ledger")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "stream-test", Model: "model"}
	inserted, err := st.RecordSupervisorModelStarted(context.Background(), turn.Checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("model start inserted=%t err=%v", inserted, err)
	}
	delta := llm.ModelDelta{Sequence: 1, ChunkCount: 2, ByteCount: 10, TotalBytes: 10, Done: true}
	inserted, err = st.RecordSupervisorModelDelta(context.Background(), turn.Checkpoint, attempt, delta)
	if err != nil || !inserted {
		t.Fatalf("delta inserted=%t err=%v", inserted, err)
	}
	inserted, err = st.RecordSupervisorModelDelta(context.Background(), turn.Checkpoint, attempt, delta)
	if err != nil || inserted {
		t.Fatalf("delta replay inserted=%t err=%v", inserted, err)
	}
	mismatch := delta
	mismatch.ByteCount = 9
	if _, err := st.RecordSupervisorModelDelta(context.Background(), turn.Checkpoint, attempt, mismatch); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("mismatched delta replay code=%s err=%v", apperror.CodeOf(err), err)
	}
	terminal := attempt
	terminal.Outcome = llm.OutcomeSuccess
	if _, err := st.RecordSupervisorModelCompleted(context.Background(), turn.Checkpoint, terminal, llm.ChatResponse{}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("terminal accepted wrong stream counters code=%s err=%v", apperror.CodeOf(err), err)
	}
	terminal.StreamEvents = 1
	terminal.StreamBytes = 10
	if _, err := st.RecordSupervisorModelCompleted(context.Background(), turn.Checkpoint, terminal, llm.ChatResponse{}); err != nil {
		t.Fatal(err)
	}
}

type scriptedStreamProvider struct {
	name        string
	streams     [][]llm.ChatChunk
	startupErrs []error
	requests    []llm.ChatRequest
	streamCalls int
	chatCalls   int
}

func (p *scriptedStreamProvider) Name() string { return p.name }

func (p *scriptedStreamProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: p.name}}, nil
}

func (p *scriptedStreamProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	p.chatCalls++
	return nil, apperror.New(apperror.CodeInternal, "non-streaming Chat must not be used by RunSupervisor")
}

func (p *scriptedStreamProvider) StreamChat(ctx context.Context, request llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index := p.streamCalls
	p.streamCalls++
	p.requests = append(p.requests, request)
	if index < len(p.startupErrs) && p.startupErrs[index] != nil {
		return nil, p.startupErrs[index]
	}
	if index >= len(p.streams) {
		return nil, apperror.New(apperror.CodeFailedPrecondition, "scripted stream exhausted")
	}
	chunks := make(chan llm.ChatChunk, len(p.streams[index]))
	for _, chunk := range p.streams[index] {
		chunks <- chunk
	}
	close(chunks)
	return chunks, nil
}

func (*scriptedStreamProvider) SupportsTools(string) bool    { return false }
func (*scriptedStreamProvider) SupportsVision(string) bool   { return false }
func (*scriptedStreamProvider) SupportsJSONMode(string) bool { return true }

type cancelThenStreamProvider struct {
	name  string
	mu    sync.Mutex
	calls int
}

func (p *cancelThenStreamProvider) Name() string { return p.name }

func (p *cancelThenStreamProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: p.name}}, nil
}

func (p *cancelThenStreamProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, apperror.New(apperror.CodeInternal, "non-streaming Chat must not be used by RunSupervisor")
}

func (p *cancelThenStreamProvider) StreamChat(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	p.mu.Lock()
	index := p.calls
	p.calls++
	p.mu.Unlock()
	if index > 0 {
		text := rootActionResponse(domain.RootActionContinue, "resumed stream", "", "")
		chunks := make(chan llm.ChatChunk, 2)
		chunks <- llm.ChatChunk{Text: text}
		chunks <- finalStreamChunk(p.name, "model", llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2})
		close(chunks)
		return chunks, nil
	}
	chunks := make(chan llm.ChatChunk, 1)
	chunks <- llm.ChatChunk{Text: "partial stream"}
	go func() {
		<-ctx.Done()
		close(chunks)
	}()
	return chunks, nil
}

func (*cancelThenStreamProvider) SupportsTools(string) bool    { return false }
func (*cancelThenStreamProvider) SupportsVision(string) bool   { return false }
func (*cancelThenStreamProvider) SupportsJSONMode(string) bool { return true }

func newStreamSupervisor(t *testing.T, st *store.SQLiteStore, provider llm.Provider, budget domain.Budget) (domain.Run, *application.RunSupervisor) {
	t.Helper()
	run := newStartedRunForProvider(t, st, provider.Name(), budget)
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	return run, application.NewRunSupervisor(st, router, policy.NewDefaultChecker())
}

func finalStreamChunk(provider string, model string, usage llm.Usage) llm.ChatChunk {
	return llm.ChatChunk{Done: true, Usage: &usage, Provider: provider, Model: model}
}

func chunkedStream(text string, size int, provider string, model string, usage llm.Usage) []llm.ChatChunk {
	chunks := make([]llm.ChatChunk, 0, len(text)/size+2)
	for len(text) > 0 {
		length := min(size, len(text))
		chunks = append(chunks, llm.ChatChunk{Text: text[:length]})
		text = text[length:]
	}
	return append(chunks, finalStreamChunk(provider, model, usage))
}
