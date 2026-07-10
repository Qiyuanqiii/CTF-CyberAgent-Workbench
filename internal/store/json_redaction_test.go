package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactJSONPayloadPreservesNestedJSONString(t *testing.T) {
	token := "s" + "k-" + strings.Repeat("a", 28)
	inner := `{"schema":"script_process.v1","arguments":["--token=` + token + `"]}`
	raw, err := json.Marshal(map[string]any{
		"command": inner,
		"count":   json.Number("9223372036854775807"),
		"nested":  []any{map[string]any{"secret": token}},
	})
	if err != nil {
		t.Fatal(err)
	}
	safe, err := redactJSONPayload(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(safe)) || strings.Contains(safe, token) || !strings.Contains(safe, "[REDACTED:") {
		t.Fatalf("redacted payload is unsafe or invalid: %s", safe)
	}
	var decoded struct {
		Command string      `json:"command"`
		Count   json.Number `json:"count"`
	}
	decoder := json.NewDecoder(strings.NewReader(safe))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(decoded.Command)) || decoded.Count.String() != "9223372036854775807" {
		t.Fatalf("nested JSON or exact number was not preserved: %#v", decoded)
	}
}

func TestRedactJSONPayloadRejectsInvalidJSON(t *testing.T) {
	if _, err := redactJSONPayload(`{"broken":`); err == nil {
		t.Fatal("expected invalid JSON rejection")
	}
}

func TestRedactJSONPayloadRejectsResourceExhaustion(t *testing.T) {
	deep := strings.Repeat("[", maxStoreJSONDepth+2) + "0" + strings.Repeat("]", maxStoreJSONDepth+2)
	if _, err := redactJSONPayload(deep); err == nil || !strings.Contains(err.Error(), "JSON depth") {
		t.Fatalf("expected JSON depth rejection, got %v", err)
	}
	large := `{"value":"` + strings.Repeat("x", maxStoreJSONPayloadBytes) + `"}`
	if _, err := redactJSONPayload(large); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected JSON size rejection, got %v", err)
	}
}
