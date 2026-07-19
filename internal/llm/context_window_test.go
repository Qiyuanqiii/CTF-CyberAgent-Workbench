package llm

import "testing"

func TestDefaultContextWindowIsConservativeAndValid(t *testing.T) {
	window := DefaultContextWindow()
	if err := window.Validate(); err != nil {
		t.Fatal(err)
	}
	if window.WindowTokens != 32*1024 || window.OutputLimit(0) != 1024 ||
		window.OutputLimit(100_000) != 4096 {
		t.Fatalf("unexpected default context window: %#v", window)
	}
	limit, err := window.InputLimit(0)
	if err != nil || limit != 30*1024 {
		t.Fatalf("unexpected default input limit: %d err=%v", limit, err)
	}
}

func TestRouterContextWindowUsesExactOverrideAndGenerationSwap(t *testing.T) {
	ref := ModelRef{Provider: "mock", Model: "mock-code"}
	router := NewDefaultRouter()
	if got := router.ContextWindow(ref); got.Source != "conservative_default" {
		t.Fatalf("unexpected fallback context window: %#v", got)
	}
	override := ContextWindow{
		ProtocolVersion: ContextWindowProtocolVersion, WindowTokens: 64 * 1024,
		SafetyMarginTokens: 2048, DefaultOutputTokens: 2048, MaxOutputTokens: 8192,
		Source: "operator_configured",
	}
	if err := router.SetContextWindow(ref, override); err != nil {
		t.Fatal(err)
	}
	if got := router.ContextWindow(ref); got != override {
		t.Fatalf("context-window override was not retained: %#v", got)
	}

	next := NewDefaultRouter()
	if err := next.SetContextWindow(ref, override); err != nil {
		t.Fatal(err)
	}
	if err := router.ReplaceConfiguration(next); err != nil {
		t.Fatal(err)
	}
	if got := router.ContextWindow(ref); got != override {
		t.Fatalf("context-window override was not swapped atomically: %#v", got)
	}
}

func TestContextWindowRejectsInvalidLimits(t *testing.T) {
	window := DefaultContextWindow()
	window.MaxOutputTokens = window.WindowTokens
	if err := window.Validate(); err == nil {
		t.Fatal("invalid output limit was accepted")
	}
	window = DefaultContextWindow()
	window.Source = "operator\nconfigured"
	if err := window.Validate(); err == nil {
		t.Fatal("unsafe context-window source was accepted")
	}
	if err := new(Router).SetContextWindow(ModelRef{}, DefaultContextWindow()); err == nil {
		t.Fatal("invalid model ref was accepted")
	}
	zeroValueRouter := new(Router)
	if err := zeroValueRouter.SetContextWindow(ModelRef{Provider: "mock", Model: "model"},
		DefaultContextWindow()); err != nil {
		t.Fatalf("zero-value router context-window initialization failed: %v", err)
	}
}
