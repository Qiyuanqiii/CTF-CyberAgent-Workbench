package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

const evidenceSetReceiptCompatibilityVectorProtocol = "runner_evidence_set_receipt_compatibility_rejection_vectors.v1"

type evidenceSetReceiptCompatibilityVectorFile struct {
	ProtocolVersion string                                  `json:"protocol_version"`
	SourceVector    string                                  `json:"source_vector"`
	Vectors         []evidenceSetReceiptCompatibilityVector `json:"vectors"`
}

type evidenceSetReceiptCompatibilityVector struct {
	Name         string          `json:"name"`
	Operation    string          `json:"operation"`
	Field        string          `json:"field"`
	Value        json.RawMessage `json:"value"`
	ExpectedCode string          `json:"expected_code"`
}

func TestEvidenceSetReceiptCompatibilityRejectionVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/evidence_set_receipt_compatibility_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var vectors evidenceSetReceiptCompatibilityVectorFile
	if err := decoder.Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("compatibility vectors contain trailing JSON: %v", err)
	}
	if vectors.ProtocolVersion != evidenceSetReceiptCompatibilityVectorProtocol ||
		vectors.SourceVector != "normal_empty_exit" || len(vectors.Vectors) != 11 {
		t.Fatalf("compatibility vector envelope is invalid: %#v", vectors)
	}
	input := evidenceSetGoldenInputForName(t, vectors.SourceVector)
	receipt, err := buildEvidenceSetReceipt(input.Exit, input.Runtime, input.Limits,
		input.Cause, input.Timeline, input.Budget)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := encodeEvidenceSetReceiptEnvelope(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) == 0 || len(baseline) > MaxEvidenceSetReceiptEnvelopeBytes {
		t.Fatalf("baseline receipt envelope is not bounded: %d", len(baseline))
	}
	if err := validateEvidenceSetReceiptCompatibility(baseline, input.Exit, input.Runtime,
		input.Limits, input.Cause, input.Timeline, input.Budget); err != nil {
		t.Fatalf("valid receipt envelope was rejected: %v", err)
	}
	seen := make(map[string]struct{}, len(vectors.Vectors))
	for _, vector := range vectors.Vectors {
		if vector.Name == "" || vector.ExpectedCode == "" {
			t.Fatalf("compatibility vector is incomplete: %#v", vector)
		}
		if _, duplicate := seen[vector.Name]; duplicate {
			t.Fatalf("duplicate compatibility vector %q", vector.Name)
		}
		seen[vector.Name] = struct{}{}
		payload := mutateEvidenceSetReceiptEnvelope(t, baseline, vector)
		err := validateEvidenceSetReceiptCompatibility(payload, input.Exit, input.Runtime,
			input.Limits, input.Cause, input.Timeline, input.Budget)
		if !errors.Is(err, ErrEvidenceSetReceipt) ||
			!strings.Contains(err.Error(), vector.ExpectedCode) {
			t.Fatalf("vector %q returned %v, want %s", vector.Name, err, vector.ExpectedCode)
		}
	}
}

func mutateEvidenceSetReceiptEnvelope(t *testing.T, baseline []byte,
	vector evidenceSetReceiptCompatibilityVector,
) []byte {
	t.Helper()
	switch vector.Operation {
	case "replace", "add", "remove":
		var document map[string]json.RawMessage
		if err := json.Unmarshal(baseline, &document); err != nil {
			t.Fatal(err)
		}
		if vector.Operation == "remove" {
			delete(document, vector.Field)
		} else {
			if len(vector.Value) == 0 || !json.Valid(vector.Value) {
				t.Fatalf("vector %q has invalid replacement JSON", vector.Name)
			}
			document[vector.Field] = append(json.RawMessage(nil), vector.Value...)
		}
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	case "duplicate":
		if vector.Field == "" || len(vector.Value) == 0 || !json.Valid(vector.Value) ||
			len(baseline) < 2 || baseline[len(baseline)-1] != '}' {
			t.Fatalf("vector %q cannot duplicate a field", vector.Name)
		}
		payload := append([]byte(nil), baseline[:len(baseline)-1]...)
		payload = append(payload, []byte(",\""+vector.Field+"\":")...)
		payload = append(payload, vector.Value...)
		return append(payload, '}')
	case "append":
		if len(vector.Value) == 0 {
			t.Fatalf("vector %q has no trailing payload", vector.Name)
		}
		return append(append([]byte(nil), baseline...), vector.Value...)
	case "append_invalid_utf8":
		return append(append([]byte(nil), baseline...), 0xff)
	case "exceed_size_limit":
		padding := MaxEvidenceSetReceiptEnvelopeBytes - len(baseline) + 1
		if padding < 1 {
			t.Fatalf("vector %q baseline already exceeds the size limit", vector.Name)
		}
		return append(append([]byte(nil), baseline...), bytes.Repeat([]byte{' '}, padding)...)
	default:
		t.Fatalf("vector %q has unknown operation %q", vector.Name, vector.Operation)
		return nil
	}
}
