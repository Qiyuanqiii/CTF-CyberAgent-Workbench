package analyzer

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestEvaluateFixtureProducesStrictMetadataOnlyResult(t *testing.T) {
	request := testRequest()
	request.Input.MediaType = "text/plain"
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString([]byte("alpha\nbeta\n"))
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	output, exitCode := EvaluateFixture(raw)
	if exitCode != ExitSuccess {
		t.Fatalf("exit code = %d, output = %s", exitCode, output)
	}
	result, code := DecodeResult(output)
	if code != "" {
		t.Fatalf("result rejected with %q: %s", code, output)
	}
	if result.ProtocolVersion != ResultProtocolVersion || result.RequestID != request.RequestID ||
		result.Analyzer != FixtureAnalyzerName || result.Status != "succeeded" ||
		result.Summary.MediaType != "text/plain" || result.Summary.InputBytes != 11 ||
		result.Summary.SHA256 != "e49c81e2d2f84e259d40e2fb8192f3bcd198b355184845d76d8f58807d0d78ee" ||
		!result.Summary.UTF8 || result.Summary.LineCount != 2 || !result.MetadataOnly ||
		capabilitiesEnabled(result.CapabilitiesUsed) {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertExactObjectKeys(t, output, []string{"analyzer", "capabilities_used", "metadata_only",
		"protocol_version", "request_id", "status", "summary"})
}

func TestEvaluateFixtureRejectsUnsafeOrInvalidRequests(t *testing.T) {
	valid := testRequestJSON(t)
	tests := []struct {
		name      string
		raw       []byte
		code      ErrorCode
		requestID string
	}{
		{name: "empty", raw: nil, code: CodeMalformedEnvelope},
		{name: "trailing", raw: append(append([]byte{}, valid...), []byte(`{}`)...),
			code: CodeMalformedEnvelope},
		{name: "deep nesting", raw: []byte(strings.Repeat("[", maxJSONNestingDepth+2) +
			strings.Repeat("]", maxJSONNestingDepth+2)), code: CodeMalformedEnvelope},
		{name: "unknown", raw: replaceJSON(t, valid, `"metadata_only":true`,
			`"metadata_only":true,"extra":false`), code: CodeMalformedEnvelope},
		{name: "duplicate nested", raw: replaceJSON(t, valid, `"filesystem":false`,
			`"filesystem":false,"filesystem":false`), code: CodeMalformedEnvelope},
		{name: "missing false capability", raw: replaceJSON(t, valid,
			`,"environment":false`, ``), code: CodeInvalidRequest},
		{name: "unsupported protocol", raw: replaceJSON(t, valid, RequestProtocolVersion,
			"analyzer_protocol.v2"), code: CodeUnsupportedProtocol, requestID: "request-1"},
		{name: "invalid request id", raw: replaceJSON(t, valid, "request-1", "private value"),
			code: CodeInvalidRequest},
		{name: "unsupported analyzer", raw: replaceJSON(t, valid, FixtureAnalyzerName,
			"archive.inspect.v1"), code: CodeUnsupportedAnalyzer, requestID: "request-1"},
		{name: "filesystem", raw: replaceJSON(t, valid, `"filesystem":false`,
			`"filesystem":true`), code: CodeCapabilityDenied, requestID: "request-1"},
		{name: "network", raw: replaceJSON(t, valid, `"network":false`,
			`"network":true`), code: CodeCapabilityDenied, requestID: "request-1"},
		{name: "metadata body", raw: replaceJSON(t, valid, `"metadata_only":true`,
			`"metadata_only":false`), code: CodeInvalidRequest, requestID: "request-1"},
		{name: "short timeout", raw: replaceJSON(t, valid, `"timeout_ms":5000`,
			`"timeout_ms":99`), code: CodeInvalidRequest, requestID: "request-1"},
		{name: "invalid media", raw: replaceJSON(t, valid, "application/octet-stream",
			"Application/octet-stream"), code: CodeInvalidRequest, requestID: "request-1"},
		{name: "invalid base64", raw: replaceJSON(t, valid, `"content_base64":""`,
			`"content_base64":"%%%"`), code: CodeInvalidContent, requestID: "request-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, exitCode := EvaluateFixture(test.raw)
			if exitCode != ExitRejected {
				t.Fatalf("exit code = %d, want %d", exitCode, ExitRejected)
			}
			rejection, decodeCode := DecodeError(output)
			if decodeCode != "" || rejection.Code != test.code ||
				rejection.RequestID != test.requestID || rejection.Message != messageFor(test.code) ||
				rejection.Retryable || !rejection.MetadataOnly {
				t.Fatalf("unexpected rejection decode=%q value=%#v output=%s",
					decodeCode, rejection, output)
			}
		})
	}
}

func TestEvaluateFixtureEnforcesInputOutputAndEnvelopeLimits(t *testing.T) {
	request := testRequest()
	request.Limits.MaxInputBytes = 4
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString([]byte("12345"))
	assertFixtureError(t, request, CodeInputLimitExceeded)

	request = testRequest()
	request.RequestID = strings.Repeat("r", MaxRequestIDBytes)
	request.Input.MediaType = "application/" + strings.Repeat("x", 116)
	request.Limits.MaxOutputBytes = MinResultEnvelopeBytes
	assertFixtureError(t, request, CodeOutputLimitExceeded)

	oversized := bytes.Repeat([]byte{' '}, MaxRequestEnvelopeBytes+1)
	output, exitCode := EvaluateFixture(oversized)
	if exitCode != ExitRejected {
		t.Fatalf("oversized request exit = %d", exitCode)
	}
	rejection, code := DecodeError(output)
	if code != "" || rejection.Code != CodeRequestTooLarge || rejection.RequestID != "" {
		t.Fatalf("oversized rejection = %#v, decode = %q", rejection, code)
	}
}

func TestEvaluateFixtureClassifiesBinaryWithoutReturningContent(t *testing.T) {
	request := testRequest()
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString([]byte{0xff, 0x00, 0x0a})
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	output, exitCode := EvaluateFixture(raw)
	if exitCode != ExitSuccess {
		t.Fatalf("exit = %d, output = %s", exitCode, output)
	}
	result, code := DecodeResult(output)
	if code != "" || result.Summary.UTF8 || result.Summary.LineCount != 0 ||
		bytes.Contains(output, []byte(request.Input.ContentBase64)) {
		t.Fatalf("binary metadata result = %#v, code = %q, output = %s", result, code, output)
	}
}

func TestStrictResultAndErrorDecodersFailClosed(t *testing.T) {
	output, exitCode := EvaluateFixture(testRequestJSON(t))
	if exitCode != ExitSuccess {
		t.Fatal("valid fixture request failed")
	}
	resultTests := [][]byte{
		replaceJSON(t, output, `"status":"succeeded"`, `"status":"succeeded","status":"succeeded"`),
		replaceJSON(t, output, `"metadata_only":true`, `"metadata_only":true,"extra":false`),
		replaceJSON(t, output, ResultProtocolVersion, "analyzer_result.v2"),
		append(append([]byte{}, output...), []byte(`{}`)...),
		bytes.Repeat([]byte{' '}, MaxResultEnvelopeBytes+1),
	}
	for index, raw := range resultTests {
		if _, code := DecodeResult(raw); code == "" {
			t.Fatalf("invalid result %d was accepted", index)
		}
	}

	errorOutput := encodeError("request-1", CodeProcessFailed)
	value, code := DecodeError(errorOutput)
	if code != "" || !value.Retryable {
		t.Fatalf("retryable error rejected: %#v code=%q", value, code)
	}
	for index, raw := range [][]byte{
		replaceJSON(t, errorOutput, `"retryable":true`, `"retryable":false`),
		replaceJSON(t, errorOutput, `"message":"analyzer process failed"`,
			`"message":"private process detail"`),
		replaceJSON(t, errorOutput, `"metadata_only":true`,
			`"metadata_only":true,"metadata_only":true`),
	} {
		if _, decodeCode := DecodeError(raw); decodeCode == "" {
			t.Fatalf("invalid error envelope %d was accepted", index)
		}
	}
}

func TestEveryProtocolErrorHasStableMetadata(t *testing.T) {
	codes := []ErrorCode{CodeMalformedEnvelope, CodeRequestTooLarge, CodeUnsupportedProtocol,
		CodeInvalidRequest, CodeCapabilityDenied, CodeUnsupportedAnalyzer,
		CodeInputLimitExceeded, CodeInvalidContent, CodeOutputLimitExceeded,
		CodeResultTooLarge, CodeDeadlineExceeded, CodeProcessFailed, CodeInvalidResult,
		CodeInternal}
	for _, code := range codes {
		if messageFor(code) == "" {
			t.Fatalf("missing stable message for %q", code)
		}
		encoded := encodeError("request-1", code)
		value, decodeCode := DecodeError(encoded)
		if decodeCode != "" || value.Code != code || value.Retryable != retryable(code) {
			t.Fatalf("error %q did not round trip: %#v decode=%q", code, value, decodeCode)
		}
	}
}

func FuzzEvaluateFixtureProducesBoundedStrictEnvelope(f *testing.F) {
	valid, err := json.Marshal(testRequest())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	archiveRequest := testRequest()
	archiveRequest.Analyzer = ArchiveAnalyzerName
	archiveRequest.Input.MediaType = "application/zip"
	archiveRequest.Input.ContentBase64 = base64.StdEncoding.EncodeToString(testZIP(f,
		[]testZIPEntry{{name: "seed.txt", content: []byte("seed")}}))
	archiveValid, err := json.Marshal(archiveRequest)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(archiveValid)
	f.Add([]byte(`{"protocol_version":"analyzer_protocol.v1"}`))
	f.Add([]byte(`{"duplicate":false,"duplicate":true}`))
	f.Add([]byte{0xff, 0x00, '{'})
	f.Fuzz(func(t *testing.T, raw []byte) {
		output, exitCode := EvaluateFixture(raw)
		if len(output) == 0 || len(output) > MaxResultEnvelopeBytes {
			t.Fatalf("output size = %d for exit %d", len(output), exitCode)
		}
		switch exitCode {
		case ExitSuccess:
			if _, fixtureCode := DecodeResult(output); fixtureCode != "" {
				if _, archiveCode := DecodeArchiveInventory(output); archiveCode != "" {
					t.Fatalf("success output failed strict decode: fixture=%s archive=%s output=%s",
						fixtureCode, archiveCode, output)
				}
			}
		case ExitRejected:
			if _, code := DecodeError(output); code != "" {
				t.Fatalf("rejection output failed strict decode: %s output=%s", code, output)
			}
		default:
			t.Fatalf("unexpected exit code %d", exitCode)
		}
	})
}

func testRequest() Request {
	return Request{ProtocolVersion: RequestProtocolVersion, RequestID: "request-1",
		Analyzer: FixtureAnalyzerName,
		Input:    Input{MediaType: "application/octet-stream", ContentBase64: ""},
		Limits: Limits{MaxInputBytes: MaxDecodedInputBytes,
			MaxOutputBytes: 4096, TimeoutMilliseconds: 5000},
		Capabilities: Capabilities{}, MetadataOnly: true}
}

func testRequestJSON(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertFixtureError(t *testing.T, request Request, expected ErrorCode) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	output, exitCode := EvaluateFixture(raw)
	value, code := DecodeError(output)
	if exitCode != ExitRejected || code != "" || value.Code != expected {
		t.Fatalf("exit=%d decode=%q error=%#v output=%s", exitCode, code, value, output)
	}
}

func replaceJSON(t *testing.T, raw []byte, oldValue, newValue string) []byte {
	t.Helper()
	if !bytes.Contains(raw, []byte(oldValue)) {
		t.Fatalf("fixture does not contain %q: %s", oldValue, raw)
	}
	return bytes.Replace(raw, []byte(oldValue), []byte(newValue), 1)
}

func assertExactObjectKeys(t *testing.T, raw []byte, expected []string) {
	t.Helper()
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != len(expected) {
		t.Fatalf("object keys = %v, want %v", value, expected)
	}
	for _, key := range expected {
		if _, ok := value[key]; !ok {
			t.Fatalf("object is missing key %q: %s", key, raw)
		}
	}
}
