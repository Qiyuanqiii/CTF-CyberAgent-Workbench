package events

import "testing"

func TestNewCreatesValidVersionedEnvelope(t *testing.T) {
	event, err := New("run-test", "mission-test", RunCreatedEvent, "test", "run-test", map[string]any{"status": "created"})
	if err != nil {
		t.Fatal(err)
	}
	if event.Version != EnvelopeVersion || event.EventID == "" || event.PayloadJSON == "" {
		t.Fatalf("unexpected event: %#v", event)
	}
}
