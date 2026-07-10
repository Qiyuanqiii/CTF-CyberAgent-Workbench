package llm

import "errors"

const (
	MaxModelDeltaEvents = 32
	MaxModelOutputBytes = 64 * 1024
)

type ModelDelta struct {
	Sequence   int
	ChunkCount int
	ByteCount  int
	TotalBytes int
	Done       bool
}

func (d ModelDelta) Validate(maxEvents int, maxBytes int) error {
	if d.Sequence <= 0 || d.Sequence > maxEvents {
		return errors.New("model delta sequence is outside its event limit")
	}
	if d.ChunkCount < 0 || d.ByteCount < 0 || d.TotalBytes < 0 || d.ByteCount > d.TotalBytes {
		return errors.New("model delta counters cannot be negative or inconsistent")
	}
	if maxBytes > 0 && d.TotalBytes > maxBytes {
		return errors.New("model delta total exceeds its byte limit")
	}
	if !d.Done && (d.ChunkCount == 0 || d.ByteCount == 0) {
		return errors.New("non-final model delta must contain progress")
	}
	return nil
}

func (u Usage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 {
		return errors.New("model usage counters cannot be negative")
	}
	maxInt := int(^uint(0) >> 1)
	if u.InputTokens > maxInt-u.OutputTokens {
		return errors.New("model usage counters overflow")
	}
	return nil
}
