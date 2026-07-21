package analyzer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const invocationFailureVectorProtocol = "analyzer_invocation_failure_vectors.v1"

type invocationFailureVectorFile struct {
	ProtocolVersion string                    `json:"protocol_version"`
	Vectors         []invocationFailureVector `json:"vectors"`
}

type invocationFailureVector struct {
	Name            string                `json:"name"`
	Mode            string                `json:"mode"`
	Output          string                `json:"output"`
	ExitCode        int                   `json:"exit_code"`
	TimeoutMS       int                   `json:"timeout_ms"`
	ExpectedStatus  InvocationStatus      `json:"expected_status"`
	ExpectedFailure InvocationFailureCode `json:"expected_failure"`
}

func TestInvocationCandidateBindsCanonicalRequestDescriptorAndInlineInput(t *testing.T) {
	request := testRequest()
	content := []byte("private-inline-value\n")
	request.Input.MediaType = "text/plain"
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString(content)
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	candidate, code := BuildInvocationCandidate(raw)
	if code != "" {
		t.Fatalf("candidate build code = %s", code)
	}
	inputDigest := sha256.Sum256(content)
	if candidate.ProtocolVersion != InvocationProtocolVersion ||
		candidate.RequestID != request.RequestID || candidate.Analyzer != request.Analyzer ||
		candidate.InputSHA256 != hex.EncodeToString(inputDigest[:]) ||
		candidate.InputBytes != len(content) || candidate.MediaType != request.Input.MediaType ||
		candidate.Authority != (InvocationAuthority{}) || capabilitiesEnabled(candidate.Capabilities) ||
		!candidate.Deterministic || !candidate.MetadataOnly || !candidate.InlineInput ||
		candidate.Descriptor.Authority != (DescriptorAuthority{}) {
		t.Fatalf("unsafe or incomplete candidate: %#v", candidate)
	}
	encoded, code := EncodeInvocationCandidate(candidate, raw)
	if code != "" {
		t.Fatalf("candidate encode code = %s", code)
	}
	if bytes.Contains(encoded, content) || bytes.Contains(encoded, []byte(request.Input.ContentBase64)) {
		t.Fatalf("candidate retained inline input: %s", encoded)
	}
	decoded, code := DecodeInvocationCandidate(encoded, raw)
	if code != "" || !reflect.DeepEqual(decoded, candidate) {
		t.Fatalf("candidate round trip failed: code=%s value=%#v", code, decoded)
	}
	assertExactObjectKeys(t, encoded, []string{"analyzer", "authority", "capabilities",
		"descriptor", "descriptor_sha256", "deterministic", "inline_input", "input_bytes",
		"input_sha256", "limits", "media_type", "metadata_only", "protocol_version",
		"request_id", "request_sha256"})

	var indented bytes.Buffer
	if err := json.Indent(&indented, raw, "", "  "); err != nil {
		t.Fatal(err)
	}
	canonicalized, code := BuildInvocationCandidate(indented.Bytes())
	if code != "" || !reflect.DeepEqual(canonicalized, candidate) {
		t.Fatalf("canonical request binding drifted: code=%s value=%#v", code, canonicalized)
	}
}

func TestInvocationCandidateRejectsTamperingAndSchemaDrift(t *testing.T) {
	raw := testRequestJSON(t)
	candidate, code := BuildInvocationCandidate(raw)
	if code != "" {
		t.Fatal(code)
	}
	mutations := map[string]func(*InvocationCandidate){
		"request digest": func(value *InvocationCandidate) { value.RequestSHA256 = strings.Repeat("a", 64) },
		"descriptor": func(value *InvocationCandidate) {
			value.Descriptor.AcceptedMediaTypes[0] = "text/plain"
		},
		"authority": func(value *InvocationCandidate) { value.Authority.ProcessStart = true },
		"limit":     func(value *InvocationCandidate) { value.Limits.TimeoutMilliseconds++ },
		"inline":    func(value *InvocationCandidate) { value.InlineInput = false },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			value := candidate
			value.Descriptor = cloneDescriptor(candidate.Descriptor)
			mutate(&value)
			if got := ValidateInvocationCandidate(value, raw); got != CodeInvalidResult {
				t.Fatalf("tampered candidate code = %s", got)
			}
		})
	}

	encoded, code := EncodeInvocationCandidate(candidate, raw)
	if code != "" {
		t.Fatal(code)
	}
	text := string(encoded)
	for name, malformed := range map[string]string{
		"future": strings.Replace(text, InvocationProtocolVersion, "analyzer_invocation.v2", 1),
		"unknown": strings.Replace(text, `"inline_input":true`,
			`"inline_input":true,"executable":"fixture.exe"`, 1),
		"duplicate": strings.Replace(text, `"process_start":false`,
			`"process_start":false,"process_start":false`, 1),
		"missing false": strings.Replace(text, `,"artifact_commit":false`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, got := DecodeInvocationCandidate([]byte(malformed), raw); got != CodeInvalidResult {
				t.Fatalf("schema drift code = %s", got)
			}
		})
	}
}

func TestBridgeAcceptsStrictSuccessRejectionAndInternalFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		raw := invocationRequestJSON(t, 500)
		candidate := mustInvocationCandidate(t, raw)
		stdout, exitCode := Evaluate(raw)
		transport := mustFakeTransport(t, FakeTransportPlan{Stdout: stdout, ExitCode: exitCode})
		stdout[0] = 'x'
		outcome := mustInvoke(t, transport, context.Background(), candidate, raw)
		if outcome.Status != InvocationSucceeded || outcome.ResultProtocol != ResultProtocolVersion ||
			!outcome.ResultValidated || !outcome.StdoutWithinLimit {
			t.Fatalf("success outcome = %#v", outcome)
		}
		assertOutcomeRoundTrip(t, candidate, outcome)
	})

	t.Run("rejected", func(t *testing.T) {
		request := testRequest()
		request.Analyzer = ArchiveAnalyzerName
		request.Input.MediaType = "application/zip"
		request.Input.ContentBase64 = base64.StdEncoding.EncodeToString([]byte("not a zip"))
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		candidate := mustInvocationCandidate(t, raw)
		stdout, exitCode := Evaluate(raw)
		outcome := mustInvoke(t, mustFakeTransport(t, FakeTransportPlan{
			Stdout: stdout, ExitCode: exitCode,
		}), context.Background(), candidate, raw)
		if outcome.Status != InvocationRejected || outcome.AnalyzerErrorCode != CodeInvalidContent ||
			outcome.ResultProtocol != ErrorProtocolVersion || !outcome.ResultValidated {
			t.Fatalf("rejection outcome = %#v", outcome)
		}
		assertOutcomeRoundTrip(t, candidate, outcome)
	})

	t.Run("internal", func(t *testing.T) {
		raw := invocationRequestJSON(t, 500)
		candidate := mustInvocationCandidate(t, raw)
		outcome := mustInvoke(t, mustFakeTransport(t, FakeTransportPlan{
			Stdout: encodeError(candidate.RequestID, CodeInternal), ExitCode: ExitInternal,
		}), context.Background(), candidate, raw)
		if outcome.Status != InvocationFailed || outcome.FailureCode != InvocationFailureInternal ||
			outcome.AnalyzerErrorCode != CodeInternal || !outcome.ResultValidated {
			t.Fatalf("internal outcome = %#v", outcome)
		}
		assertOutcomeRoundTrip(t, candidate, outcome)
	})
}

func TestBridgeRejectsArchiveResultReplayedAcrossInputs(t *testing.T) {
	request := testRequest()
	request.RequestID = "archive-replay"
	request.Analyzer = ArchiveAnalyzerName
	request.Input.MediaType = "application/zip"
	validZIP := testZIP(t, []testZIPEntry{{name: "valid.txt", content: []byte("valid")}})

	t.Run("success", func(t *testing.T) {
		request.Input.ContentBase64 = base64.StdEncoding.EncodeToString(testZIP(t,
			[]testZIPEntry{{name: "first.txt", content: []byte("first")}}))
		firstRaw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		firstResult, exitCode := Evaluate(firstRaw)
		if exitCode != ExitSuccess {
			t.Fatalf("first archive exit = %d result=%s", exitCode, firstResult)
		}

		request.Input.ContentBase64 = base64.StdEncoding.EncodeToString(validZIP)
		secondRaw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		assertCrossInputReplayRejected(t, secondRaw, firstResult, ExitSuccess)
	})

	t.Run("rejection", func(t *testing.T) {
		request.Input.ContentBase64 = base64.StdEncoding.EncodeToString([]byte("not a zip"))
		invalidRaw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		rejection, exitCode := Evaluate(invalidRaw)
		if exitCode != ExitRejected {
			t.Fatalf("invalid archive exit = %d result=%s", exitCode, rejection)
		}

		request.Input.ContentBase64 = base64.StdEncoding.EncodeToString(validZIP)
		validRaw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		assertCrossInputReplayRejected(t, validRaw, rejection, ExitRejected)
	})
}

func assertCrossInputReplayRejected(t *testing.T, rawRequest, replayedResult []byte, exitCode int) {
	t.Helper()
	candidate := mustInvocationCandidate(t, rawRequest)
	outcome := mustInvoke(t, mustFakeTransport(t, FakeTransportPlan{
		Stdout: replayedResult, ExitCode: exitCode,
	}), context.Background(), candidate, rawRequest)
	if outcome.Status != InvocationFailed ||
		outcome.FailureCode != InvocationFailureInvalidResult || outcome.ResultValidated {
		t.Fatalf("cross-input archive replay outcome = %#v", outcome)
	}
}

func TestDisabledTransportReturnsClosedMetadataWithoutStarting(t *testing.T) {
	raw := invocationRequestJSON(t, 500)
	candidate := mustInvocationCandidate(t, raw)
	bridge, err := NewBridge(DisabledTransport{})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, code := bridge.Invoke(cancelled, candidate, raw)
	if code != "" || outcome.Status != InvocationDisabled ||
		outcome.FailureCode != InvocationFailureDisabled || outcome.Transport != DisabledTransportName ||
		outcome.ExitCode != -1 || outcome.StdoutBytes != 0 || outcome.ResultValidated ||
		outcome.ProductInvocationEnabled || outcome.RawOutputIncluded {
		t.Fatalf("disabled outcome code=%s value=%#v", code, outcome)
	}
	assertOutcomeRoundTrip(t, candidate, outcome)
}

func TestInvocationFailureVectorsAreDeterministicAcrossReconstruction(t *testing.T) {
	vectors := loadInvocationFailureVectors(t)
	if len(vectors.Vectors) != 8 {
		t.Fatalf("failure vector count = %d", len(vectors.Vectors))
	}
	seen := make(map[string]struct{}, len(vectors.Vectors))
	for _, vector := range vectors.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			if vector.Name == "" {
				t.Fatal("vector name is empty")
			}
			if _, duplicate := seen[vector.Name]; duplicate {
				t.Fatalf("duplicate vector %q", vector.Name)
			}
			seen[vector.Name] = struct{}{}
			raw := invocationRequestJSON(t, vector.TimeoutMS)
			candidate := mustInvocationCandidate(t, raw)
			candidateEnvelope, code := EncodeInvocationCandidate(candidate, raw)
			if code != "" {
				t.Fatal(code)
			}
			var previous []byte
			for replay := 0; replay < 2; replay++ {
				reconstructed, code := DecodeInvocationCandidate(candidateEnvelope, raw)
				if code != "" {
					t.Fatalf("reconstruct %d code = %s", replay, code)
				}
				plan, ctx := invocationVectorPlan(t, vector, reconstructed, raw)
				transport := mustFakeTransport(t, plan)
				outcome := mustInvoke(t, transport, ctx, reconstructed, raw)
				if outcome.Status != vector.ExpectedStatus || outcome.FailureCode != vector.ExpectedFailure {
					t.Fatalf("replay %d outcome = %#v", replay, outcome)
				}
				encoded, code := EncodeInvocationOutcome(reconstructed, outcome)
				if code != "" {
					t.Fatalf("replay %d encode code = %s", replay, code)
				}
				decoded, code := DecodeInvocationOutcome(encoded, reconstructed)
				if code != "" || !reflect.DeepEqual(decoded, outcome) {
					t.Fatalf("replay %d outcome round trip failed: code=%s", replay, code)
				}
				if replay > 0 && !bytes.Equal(encoded, previous) {
					t.Fatalf("restart-independent outcome drifted:\nfirst=%s\nagain=%s", previous, encoded)
				}
				previous = append([]byte(nil), encoded...)
			}
		})
	}
}

func TestBridgeRejectsCandidateAndOutcomeDriftBeforeOrAfterTransport(t *testing.T) {
	raw := invocationRequestJSON(t, 500)
	candidate := mustInvocationCandidate(t, raw)
	stdout, exitCode := Evaluate(raw)
	transport := mustFakeTransport(t, FakeTransportPlan{Stdout: stdout, ExitCode: exitCode})
	bridge, err := NewBridge(transport)
	if err != nil {
		t.Fatal(err)
	}
	tampered := candidate
	tampered.Authority.ArtifactCommit = true
	if outcome, code := bridge.Invoke(context.Background(), tampered, raw); code != CodeInvalidResult || outcome != (InvocationOutcome{}) {
		t.Fatalf("tampered invocation code=%s outcome=%#v", code, outcome)
	}

	outcome := mustInvoke(t, transport, context.Background(), candidate, raw)
	encoded, code := EncodeInvocationOutcome(candidate, outcome)
	if code != "" {
		t.Fatal(code)
	}
	text := string(encoded)
	for name, malformed := range map[string]string{
		"future": strings.Replace(text, InvocationOutcomeProtocolVersion,
			"analyzer_invocation_outcome.v2", 1),
		"unknown": strings.Replace(text, `"product_invocation_enabled":false`,
			`"product_invocation_enabled":false,"raw_stdout":"secret"`, 1),
		"duplicate": strings.Replace(text, `"raw_output_included":false`,
			`"raw_output_included":false,"raw_output_included":false`, 1),
		"missing false": strings.Replace(text, `,"product_invocation_enabled":false`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, got := DecodeInvocationOutcome([]byte(malformed), candidate); got != CodeInvalidResult {
				t.Fatalf("outcome drift code = %s", got)
			}
		})
	}
	for name, mutate := range map[string]func(*InvocationOutcome){
		"stdout count": func(value *InvocationOutcome) { value.StdoutBytes = MaxFakeTransportStdoutBytes },
		"limit claim":  func(value *InvocationOutcome) { value.StdoutWithinLimit = false },
		"raw output":   func(value *InvocationOutcome) { value.RawOutputIncluded = true },
		"product start": func(value *InvocationOutcome) {
			value.ProductInvocationEnabled = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := outcome
			mutate(&value)
			if ValidateInvocationOutcome(candidate, value) {
				t.Fatalf("tampered outcome was accepted: %#v", value)
			}
		})
	}
}

func FuzzInvocationCandidateDecoderIsBoundedAndIdempotent(f *testing.F) {
	request := testRequest()
	rawRequest, err := json.Marshal(request)
	if err != nil {
		f.Fatal(err)
	}
	candidate, code := BuildInvocationCandidate(rawRequest)
	if code != "" {
		f.Fatal(code)
	}
	valid, code := EncodeInvocationCandidate(candidate, rawRequest)
	if code != "" {
		f.Fatal(code)
	}
	f.Add(valid)
	f.Add([]byte(`{"protocol_version":"analyzer_invocation.v2"}`))
	f.Add([]byte{0xff, 0x00, '{'})
	f.Fuzz(func(t *testing.T, raw []byte) {
		decoded, decodeCode := DecodeInvocationCandidate(raw, rawRequest)
		if decodeCode != "" {
			return
		}
		encoded, encodeCode := EncodeInvocationCandidate(decoded, rawRequest)
		if encodeCode != "" {
			t.Fatalf("accepted candidate failed re-encoding: %s", encodeCode)
		}
		again, againCode := DecodeInvocationCandidate(encoded, rawRequest)
		if againCode != "" || !reflect.DeepEqual(again, decoded) {
			t.Fatalf("accepted candidate was not idempotent: code=%s", againCode)
		}
	})
}

func FuzzInvocationOutcomeDecoderIsBoundedAndIdempotent(f *testing.F) {
	request := testRequest()
	request.Input.MediaType = "text/plain"
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString([]byte("seed\n"))
	rawRequest, err := json.Marshal(request)
	if err != nil {
		f.Fatal(err)
	}
	candidate, code := BuildInvocationCandidate(rawRequest)
	if code != "" {
		f.Fatal(code)
	}
	stdout, exitCode := Evaluate(rawRequest)
	transport, err := NewFakeTransport(FakeTransportPlan{Stdout: stdout, ExitCode: exitCode})
	if err != nil {
		f.Fatal(err)
	}
	bridge, err := NewBridge(transport)
	if err != nil {
		f.Fatal(err)
	}
	outcome, code := bridge.Invoke(context.Background(), candidate, rawRequest)
	if code != "" {
		f.Fatal(code)
	}
	valid, code := EncodeInvocationOutcome(candidate, outcome)
	if code != "" {
		f.Fatal(code)
	}
	f.Add(valid)
	f.Add([]byte(`{"protocol_version":"analyzer_invocation_outcome.v2"}`))
	f.Add([]byte{0xff, 0x00, '{'})
	f.Fuzz(func(t *testing.T, raw []byte) {
		decoded, decodeCode := DecodeInvocationOutcome(raw, candidate)
		if decodeCode != "" {
			return
		}
		encoded, encodeCode := EncodeInvocationOutcome(candidate, decoded)
		if encodeCode != "" {
			t.Fatalf("accepted outcome failed re-encoding: %s", encodeCode)
		}
		again, againCode := DecodeInvocationOutcome(encoded, candidate)
		if againCode != "" || again != decoded {
			t.Fatalf("accepted outcome was not idempotent: code=%s", againCode)
		}
	})
}

func invocationVectorPlan(t *testing.T, vector invocationFailureVector,
	candidate InvocationCandidate, raw []byte,
) (FakeTransportPlan, context.Context) {
	t.Helper()
	ctx := context.Background()
	plan := FakeTransportPlan{ExitCode: vector.ExitCode}
	valid, exitCode := Evaluate(raw)
	if exitCode != ExitSuccess {
		t.Fatalf("valid fixture output exit = %d", exitCode)
	}
	switch vector.Mode {
	case "response":
	case "crash":
		plan.Crash = true
	case "timeout":
		plan.Delay = time.Duration(candidate.Limits.TimeoutMilliseconds+25) * time.Millisecond
	case "cancelled":
		cancelled, cancel := context.WithCancel(context.Background())
		plan.Delay = 50 * time.Millisecond
		time.AfterFunc(5*time.Millisecond, cancel)
		ctx = cancelled
	default:
		t.Fatalf("unknown vector mode %q", vector.Mode)
	}
	switch vector.Output {
	case "none":
	case "valid":
		plan.Stdout = valid
	case "malformed":
		plan.Stdout = []byte(`{"protocol_version":`)
	case "future":
		plan.Stdout = bytes.Replace(valid, []byte(ResultProtocolVersion),
			[]byte("analyzer_result.v2"), 1)
	case "wrong_analyzer":
		plan.Stdout = bytes.Replace(valid, []byte(FixtureAnalyzerName),
			[]byte(ArchiveAnalyzerName), 1)
	case "oversized":
		plan.Stdout = bytes.Repeat([]byte{'x'}, candidate.Limits.MaxOutputBytes+1)
	default:
		t.Fatalf("unknown vector output %q", vector.Output)
	}
	return plan, ctx
}

func loadInvocationFailureVectors(t *testing.T) invocationFailureVectorFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/invocation_failure_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value invocationFailureVectorFile
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("failure vectors contain trailing JSON: %v", err)
	}
	if value.ProtocolVersion != invocationFailureVectorProtocol {
		t.Fatalf("failure vector protocol = %q", value.ProtocolVersion)
	}
	return value
}

func invocationRequestJSON(t *testing.T, timeoutMS int) []byte {
	t.Helper()
	request := testRequest()
	request.Input.MediaType = "text/plain"
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString([]byte("alpha\nbeta\n"))
	request.Limits.TimeoutMilliseconds = timeoutMS
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustInvocationCandidate(t *testing.T, raw []byte) InvocationCandidate {
	t.Helper()
	candidate, code := BuildInvocationCandidate(raw)
	if code != "" {
		t.Fatalf("candidate code = %s", code)
	}
	return candidate
}

func mustFakeTransport(t *testing.T, plan FakeTransportPlan) *FakeTransport {
	t.Helper()
	transport, err := NewFakeTransport(plan)
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func mustInvoke(t *testing.T, transport Transport, ctx context.Context,
	candidate InvocationCandidate, raw []byte,
) InvocationOutcome {
	t.Helper()
	bridge, err := NewBridge(transport)
	if err != nil {
		t.Fatal(err)
	}
	outcome, code := bridge.Invoke(ctx, candidate, raw)
	if code != "" {
		t.Fatalf("invoke code = %s", code)
	}
	if !ValidateInvocationOutcome(candidate, outcome) {
		t.Fatalf("outcome failed validation: %#v", outcome)
	}
	return outcome
}

func assertOutcomeRoundTrip(t *testing.T, candidate InvocationCandidate,
	outcome InvocationOutcome,
) {
	t.Helper()
	encoded, code := EncodeInvocationOutcome(candidate, outcome)
	if code != "" {
		t.Fatalf("outcome encode code = %s", code)
	}
	decoded, code := DecodeInvocationOutcome(encoded, candidate)
	if code != "" || !reflect.DeepEqual(decoded, outcome) {
		t.Fatalf("outcome round trip code=%s value=%#v", code, decoded)
	}
	if bytes.Contains(encoded, []byte("alpha\nbeta")) {
		t.Fatalf("outcome retained raw output: %s", encoded)
	}
}
