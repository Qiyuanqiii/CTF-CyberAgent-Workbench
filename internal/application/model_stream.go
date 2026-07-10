package application

import (
	"bytes"
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
)

const (
	modelDeltaFlushBytes    = 2 * 1024
	modelDeltaFlushInterval = 250 * time.Millisecond
)

type modelStreamResult struct {
	Response *llm.ChatResponse
	Events   int
	Bytes    int
}

type modelStreamAggregator struct {
	supervisor *RunSupervisor
	checkpoint domain.SupervisorCheckpoint
	attempt    llm.ModelAttempt
	ref        llm.ModelRef

	output        bytes.Buffer
	pendingChunks int
	pendingBytes  int
	events        int
	durableBytes  int
}

func (s *RunSupervisor) streamModel(ctx context.Context, checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, ref llm.ModelRef, request llm.ChatRequest) (modelStreamResult, error) {
	chunks, err := s.router.StreamChatModelRef(ctx, ref, request)
	if err != nil {
		return modelStreamResult{}, err
	}
	aggregator := &modelStreamAggregator{supervisor: s, checkpoint: checkpoint, attempt: attempt, ref: ref}
	return aggregator.consume(ctx, chunks)
}

func (a *modelStreamAggregator) consume(ctx context.Context, chunks <-chan llm.ChatChunk) (modelStreamResult, error) {
	if chunks == nil {
		return a.result(nil), llm.NewProviderError(llm.OutcomeInvalidResponse, a.ref.Provider, "returned a nil stream", nil)
	}
	ticker := time.NewTicker(modelDeltaFlushInterval)
	defer ticker.Stop()
	for {
		var chunk llm.ChatChunk
		var ok bool
		select {
		case chunk, ok = <-chunks:
		default:
			select {
			case chunk, ok = <-chunks:
			case <-ctx.Done():
				flushErr := a.flush(false)
				return a.result(nil), modelStreamFailure(ctx.Err(), flushErr)
			case <-ticker.C:
				if err := a.flush(false); err != nil {
					return a.result(nil), err
				}
				continue
			}
		}
		if !ok {
			flushErr := a.flush(false)
			if ctx.Err() != nil {
				return a.result(nil), modelStreamFailure(ctx.Err(), flushErr)
			}
			streamErr := llm.NewProviderError(llm.OutcomeInvalidResponse, a.ref.Provider, "stream closed before a final chunk", nil)
			return a.result(nil), modelStreamFailure(streamErr, flushErr)
		}
		if chunk.Err != nil {
			flushErr := a.flush(false)
			if ctx.Err() != nil {
				return a.result(nil), modelStreamFailure(ctx.Err(), flushErr)
			}
			return a.result(nil), modelStreamFailure(llm.NormalizeProviderError(a.ref.Provider, chunk.Err), flushErr)
		}
		if len(chunk.ToolCalls) > 0 {
			flushErr := a.flush(false)
			streamErr := llm.NewProviderError(llm.OutcomeInvalidResponse, a.ref.Provider, "tool calls are disabled in the P2 supervisor foundation", nil)
			return a.result(nil), modelStreamFailure(streamErr, flushErr)
		}
		if err := a.appendText(chunk.Text); err != nil {
			flushErr := a.flush(false)
			return a.result(nil), modelStreamFailure(err, flushErr)
		}
		if !chunk.Done && a.pendingBytes >= modelDeltaFlushBytes {
			if err := a.flush(false); err != nil {
				return a.result(nil), err
			}
		}
		if !chunk.Done {
			continue
		}
		if chunk.Usage == nil {
			flushErr := a.flush(false)
			streamErr := llm.NewProviderError(llm.OutcomeInvalidResponse, a.ref.Provider, "final stream chunk omitted usage", nil)
			return a.result(nil), modelStreamFailure(streamErr, flushErr)
		}
		if err := chunk.Usage.Validate(); err != nil {
			flushErr := a.flush(false)
			streamErr := llm.NewProviderError(llm.OutcomeInvalidResponse, a.ref.Provider, "returned invalid token usage", err)
			return a.result(nil), modelStreamFailure(streamErr, flushErr)
		}
		if !utf8.Valid(a.output.Bytes()) {
			flushErr := a.flush(false)
			streamErr := llm.NewProviderError(llm.OutcomeInvalidResponse, a.ref.Provider, "stream returned invalid UTF-8", nil)
			return a.result(nil), modelStreamFailure(streamErr, flushErr)
		}
		if err := a.flush(true); err != nil {
			return a.result(nil), err
		}
		provider := strings.TrimSpace(chunk.Provider)
		if provider == "" {
			provider = a.ref.Provider
		}
		model := strings.TrimSpace(chunk.Model)
		if model == "" {
			model = a.ref.Model
		}
		response := &llm.ChatResponse{
			Text: a.output.String(), Usage: *chunk.Usage, Provider: provider, Model: model,
		}
		return a.result(response), nil
	}
}

func (a *modelStreamAggregator) appendText(text string) error {
	if text == "" {
		return nil
	}
	if len(text) > llm.MaxModelOutputBytes-a.output.Len() {
		return llm.NewProviderError(llm.OutcomeInvalidResponse, a.ref.Provider, "stream output exceeds 65536 bytes", nil)
	}
	_, _ = a.output.WriteString(text)
	a.pendingChunks++
	a.pendingBytes += len(text)
	return nil
}

func (a *modelStreamAggregator) flush(done bool) error {
	if a.pendingBytes == 0 && !done {
		return nil
	}
	if !done && a.events >= llm.MaxModelDeltaEvents-1 {
		return nil
	}
	if a.events >= llm.MaxModelDeltaEvents {
		return apperror.New(apperror.CodeResourceExhausted, "model delta event limit was exhausted")
	}
	delta := llm.ModelDelta{
		Sequence: a.events + 1, ChunkCount: a.pendingChunks, ByteCount: a.pendingBytes,
		TotalBytes: a.durableBytes + a.pendingBytes, Done: done,
	}
	eventCtx, eventCancel := supervisorModelEventContext(context.Background())
	inserted, err := a.supervisor.store.RecordSupervisorModelDelta(eventCtx, a.checkpoint, a.attempt, delta)
	eventCancel()
	if err != nil {
		return err
	}
	if !inserted {
		return apperror.New(apperror.CodeConflict, "model delta event already exists")
	}
	a.events++
	a.durableBytes = delta.TotalBytes
	a.pendingChunks = 0
	a.pendingBytes = 0
	return nil
}

func (a *modelStreamAggregator) result(response *llm.ChatResponse) modelStreamResult {
	return modelStreamResult{Response: response, Events: a.events, Bytes: a.durableBytes}
}

func modelStreamFailure(primary error, persistence error) error {
	if persistence != nil {
		return persistence
	}
	return primary
}
