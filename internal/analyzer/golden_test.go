package analyzer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
)

const sharedGoldenProtocol = "analyzer_protocol_golden_vectors.v1"

type sharedGoldenFile struct {
	ProtocolVersion string               `json:"protocol_version"`
	Contract        sharedGoldenContract `json:"contract"`
	ErrorCodes      []ErrorCode          `json:"error_codes"`
	Vectors         []sharedGoldenVector `json:"vectors"`
}

type sharedGoldenContract struct {
	RequestProtocol         string `json:"request_protocol"`
	ResultProtocol          string `json:"result_protocol"`
	ErrorProtocol           string `json:"error_protocol"`
	FixtureAnalyzer         string `json:"fixture_analyzer"`
	MaxRequestEnvelopeBytes int    `json:"max_request_envelope_bytes"`
	MaxDecodedInputBytes    int    `json:"max_decoded_input_bytes"`
	MinResultEnvelopeBytes  int    `json:"min_result_envelope_bytes"`
	MaxResultEnvelopeBytes  int    `json:"max_result_envelope_bytes"`
	MinTimeoutMilliseconds  int    `json:"min_timeout_ms"`
	MaxTimeoutMilliseconds  int    `json:"max_timeout_ms"`
	MaxRequestIDBytes       int    `json:"max_request_id_bytes"`
	MaxMediaTypeBytes       int    `json:"max_media_type_bytes"`
	ExitSuccess             int    `json:"exit_success"`
	ExitRejected            int    `json:"exit_rejected"`
	ExitInternal            int    `json:"exit_internal"`
}

type sharedGoldenVector struct {
	Name                 string          `json:"name"`
	Request              json.RawMessage `json:"request"`
	ExpectedExitCode     int             `json:"expected_exit_code"`
	ExpectedStdout       json.RawMessage `json:"expected_stdout"`
	ExpectedStdoutBytes  int             `json:"expected_stdout_bytes"`
	ExpectedStdoutSHA256 string          `json:"expected_stdout_sha256"`
}

func TestAnalyzerProtocolSharedGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile("../../analyzers/testdata/analyzer_protocol_v1_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var golden sharedGoldenFile
	if err := decoder.Decode(&golden); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("shared golden file contains trailing JSON: %v", err)
	}
	if golden.ProtocolVersion != sharedGoldenProtocol || len(golden.Vectors) != 5 {
		t.Fatalf("shared golden envelope is invalid: %#v", golden)
	}
	expectedContract := sharedGoldenContract{RequestProtocol: RequestProtocolVersion,
		ResultProtocol: ResultProtocolVersion, ErrorProtocol: ErrorProtocolVersion,
		FixtureAnalyzer:         FixtureAnalyzerName,
		MaxRequestEnvelopeBytes: MaxRequestEnvelopeBytes,
		MaxDecodedInputBytes:    MaxDecodedInputBytes,
		MinResultEnvelopeBytes:  MinResultEnvelopeBytes,
		MaxResultEnvelopeBytes:  MaxResultEnvelopeBytes,
		MinTimeoutMilliseconds:  MinTimeoutMilliseconds,
		MaxTimeoutMilliseconds:  MaxTimeoutMilliseconds,
		MaxRequestIDBytes:       MaxRequestIDBytes, MaxMediaTypeBytes: MaxMediaTypeBytes,
		ExitSuccess: ExitSuccess, ExitRejected: ExitRejected, ExitInternal: ExitInternal}
	if golden.Contract != expectedContract {
		t.Fatalf("shared golden contract drifted: %#v", golden.Contract)
	}
	expectedCodes := []ErrorCode{CodeMalformedEnvelope, CodeRequestTooLarge,
		CodeUnsupportedProtocol, CodeInvalidRequest, CodeCapabilityDenied,
		CodeUnsupportedAnalyzer, CodeInputLimitExceeded, CodeInvalidContent,
		CodeOutputLimitExceeded, CodeResultTooLarge, CodeDeadlineExceeded,
		CodeProcessFailed, CodeInvalidResult, CodeInternal}
	if !reflect.DeepEqual(golden.ErrorCodes, expectedCodes) {
		t.Fatalf("shared golden error codes drifted: %v", golden.ErrorCodes)
	}
	seen := make(map[string]struct{}, len(golden.Vectors))
	for _, vector := range golden.Vectors {
		if vector.Name == "" || vector.ExpectedStdoutBytes < 1 ||
			len(vector.ExpectedStdoutSHA256) != 64 {
			t.Fatalf("shared golden vector is incomplete: %#v", vector)
		}
		if _, duplicate := seen[vector.Name]; duplicate {
			t.Fatalf("duplicate shared golden vector %q", vector.Name)
		}
		seen[vector.Name] = struct{}{}
		output, exitCode := EvaluateFixture(vector.Request)
		digest := sha256.Sum256(output)
		actualDigest := hex.EncodeToString(digest[:])
		if exitCode != vector.ExpectedExitCode || len(output) != vector.ExpectedStdoutBytes ||
			actualDigest != vector.ExpectedStdoutSHA256 {
			t.Fatalf("shared golden vector %q drifted: exit=%d bytes=%d sha256=%s output=%s",
				vector.Name, exitCode, len(output), actualDigest, output)
		}
		var actualValue, expectedValue any
		if err := json.Unmarshal(output, &actualValue); err != nil {
			t.Fatalf("shared golden output %q is invalid: %v", vector.Name, err)
		}
		if err := json.Unmarshal(vector.ExpectedStdout, &expectedValue); err != nil {
			t.Fatalf("shared golden expectation %q is invalid: %v", vector.Name, err)
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Fatalf("shared golden semantic output %q drifted: %s", vector.Name, output)
		}
		if exitCode == ExitSuccess {
			if _, code := DecodeResult(output); code != "" {
				t.Fatalf("shared result %q failed Go validation: %s", vector.Name, code)
			}
		} else if value, code := DecodeError(output); code != "" || value.RequestID == "" {
			t.Fatalf("shared error %q failed Go validation: code=%s value=%#v",
				vector.Name, code, value)
		}
	}
}
