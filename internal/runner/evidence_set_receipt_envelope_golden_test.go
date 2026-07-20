package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"testing"
)

const evidenceSetReceiptEnvelopeGoldenProtocol = "runner_evidence_set_receipt_envelope_golden_vectors.v1"

type evidenceSetReceiptEnvelopeGoldenFile struct {
	ProtocolVersion string                                   `json:"protocol_version"`
	Vectors         []evidenceSetReceiptEnvelopeGoldenVector `json:"vectors"`
}

type evidenceSetReceiptEnvelopeGoldenVector struct {
	Name           string `json:"name"`
	EnvelopeBytes  int    `json:"envelope_bytes"`
	EnvelopeSHA256 string `json:"envelope_sha256"`
}

func TestEvidenceSetReceiptGoldenAcceptedEnvelopeVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/evidence_set_receipt_envelope_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var golden evidenceSetReceiptEnvelopeGoldenFile
	if err := decoder.Decode(&golden); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("envelope golden vectors contain trailing JSON: %v", err)
	}
	if golden.ProtocolVersion != evidenceSetReceiptEnvelopeGoldenProtocol ||
		len(golden.Vectors) != 2 {
		t.Fatalf("envelope golden vector file is invalid: %#v", golden)
	}
	seen := make(map[string]struct{}, len(golden.Vectors))
	for _, vector := range golden.Vectors {
		if vector.Name == "" || vector.EnvelopeBytes < 1 || len(vector.EnvelopeSHA256) != 64 {
			t.Fatalf("envelope golden vector is incomplete: %#v", vector)
		}
		if _, duplicate := seen[vector.Name]; duplicate {
			t.Fatalf("duplicate envelope golden vector %q", vector.Name)
		}
		seen[vector.Name] = struct{}{}
		input := evidenceSetGoldenInputForName(t, vector.Name)
		receipt, err := buildEvidenceSetReceipt(input.Exit, input.Runtime, input.Limits,
			input.Cause, input.Timeline, input.Budget)
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := encodeEvidenceSetReceiptEnvelope(receipt)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(envelope)
		actualDigest := hex.EncodeToString(digest[:])
		if len(envelope) != vector.EnvelopeBytes || actualDigest != vector.EnvelopeSHA256 {
			t.Fatalf("envelope vector %q drifted: bytes=%d sha256=%s",
				vector.Name, len(envelope), actualDigest)
		}
		if len(envelope) > MaxEvidenceSetReceiptEnvelopeBytes {
			t.Fatalf("envelope vector %q exceeds the compatibility limit: %d",
				vector.Name, len(envelope))
		}
		if err := validateEvidenceSetReceiptCompatibility(envelope, input.Exit, input.Runtime,
			input.Limits, input.Cause, input.Timeline, input.Budget); err != nil {
			t.Fatalf("accepted envelope vector %q was rejected: %v", vector.Name, err)
		}
		decoded, code := decodeEvidenceSetReceiptEnvelope(envelope)
		if code != "" {
			t.Fatalf("accepted envelope vector %q failed strict decode: %s", vector.Name, code)
		}
		reencoded, err := encodeEvidenceSetReceiptEnvelope(decoded)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(reencoded, envelope) {
			t.Fatalf("accepted envelope vector %q is not byte-stable", vector.Name)
		}
	}
}
