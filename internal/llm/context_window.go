package llm

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ContextWindowProtocolVersion = "model_context_window.v1"
	DefaultContextWindowTokens   = 32 * 1024
	DefaultContextSafetyTokens   = 1024
	DefaultContextOutputTokens   = 1024
	DefaultContextMaxOutput      = 4096
	MaxContextWindowTokens       = 2 * 1024 * 1024
)

// ContextWindow is a conservative local planning limit. It is not a claim
// about a remote Provider's advertised capacity.
type ContextWindow struct {
	ProtocolVersion     string
	WindowTokens        int
	SafetyMarginTokens  int
	DefaultOutputTokens int
	MaxOutputTokens     int
	Source              string
}

func DefaultContextWindow() ContextWindow {
	return ContextWindow{
		ProtocolVersion: ContextWindowProtocolVersion,
		WindowTokens:    DefaultContextWindowTokens, SafetyMarginTokens: DefaultContextSafetyTokens,
		DefaultOutputTokens: DefaultContextOutputTokens, MaxOutputTokens: DefaultContextMaxOutput,
		Source: "conservative_default",
	}
}

func (w ContextWindow) Validate() error {
	if w.ProtocolVersion != ContextWindowProtocolVersion {
		return errors.New("unsupported model context-window protocol")
	}
	if w.WindowTokens < 4096 || w.WindowTokens > MaxContextWindowTokens {
		return fmt.Errorf("model context window must be between 4096 and %d tokens", MaxContextWindowTokens)
	}
	if w.SafetyMarginTokens < 128 || w.SafetyMarginTokens >= w.WindowTokens {
		return errors.New("model context safety margin is invalid")
	}
	if w.DefaultOutputTokens < 1 || w.MaxOutputTokens < w.DefaultOutputTokens ||
		w.MaxOutputTokens >= w.WindowTokens-w.SafetyMarginTokens {
		return errors.New("model context output limits are invalid")
	}
	if strings.TrimSpace(w.Source) == "" || strings.TrimSpace(w.Source) != w.Source ||
		len(w.Source) > 64 {
		return errors.New("model context-window source is invalid")
	}
	for _, current := range w.Source {
		if (current < 'a' || current > 'z') && (current < '0' || current > '9') &&
			current != '_' && current != '-' && current != '.' {
			return errors.New("model context-window source is invalid")
		}
	}
	return nil
}

func (w ContextWindow) OutputLimit(requested int) int {
	if err := w.Validate(); err != nil {
		w = DefaultContextWindow()
	}
	if requested <= 0 {
		return w.DefaultOutputTokens
	}
	if requested > w.MaxOutputTokens {
		return w.MaxOutputTokens
	}
	return requested
}

func (w ContextWindow) InputLimit(outputTokens int) (int, error) {
	if err := w.Validate(); err != nil {
		return 0, err
	}
	outputTokens = w.OutputLimit(outputTokens)
	limit := w.WindowTokens - w.SafetyMarginTokens - outputTokens
	if limit <= 0 {
		return 0, errors.New("model context window leaves no input capacity")
	}
	return limit, nil
}

func contextWindowKey(ref ModelRef) (string, bool) {
	provider := strings.TrimSpace(ref.Provider)
	model := strings.TrimSpace(ref.Model)
	if provider == "" || model == "" || provider != ref.Provider || model != ref.Model {
		return "", false
	}
	return provider + "\x00" + model, true
}
