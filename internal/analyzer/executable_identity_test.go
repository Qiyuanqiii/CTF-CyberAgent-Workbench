package analyzer

import (
	"bytes"
	"encoding/json"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestExecutableIdentityAndPreflightBindExactBytesWithoutLaunchMetadata(t *testing.T) {
	raw := testRequestJSON(t)
	candidate := mustInvocationCandidate(t, raw)
	executable := []byte("MZ-private-executable-marker")

	identity, code := BuildExecutableIdentity(candidate, raw, executable)
	if code != "" {
		t.Fatal(code)
	}
	if identity.ProtocolVersion != ExecutableIdentityProtocolVersion ||
		identity.Analyzer != candidate.Analyzer ||
		identity.DescriptorSHA256 != candidate.DescriptorSHA256 ||
		identity.RequestProtocol != RequestProtocolVersion ||
		identity.ResultProtocol != ResultProtocolVersion ||
		identity.ErrorProtocol != ErrorProtocolVersion || identity.TargetGOOS != runtime.GOOS ||
		identity.TargetGOARCH != runtime.GOARCH || identity.ExecutableBytes != len(executable) ||
		!validDigest(identity.ExecutableSHA256) || !identity.DescriptorDeterministic ||
		!identity.MetadataOnly || identity.ExecutableBytesIncluded || identity.PathIncluded ||
		identity.CommandIncluded || identity.EnvironmentIncluded ||
		identity.ExecutableFormatVerified || identity.ExecutableSemanticsVerified ||
		identity.ProcessStartEnabled ||
		identity.ProductInvocationEnabled {
		t.Fatalf("unsafe or incomplete executable identity: %#v", identity)
	}
	encodedIdentity, code := EncodeExecutableIdentity(identity, candidate, raw, executable)
	if code != "" {
		t.Fatal(code)
	}
	if bytes.Contains(encodedIdentity, executable) {
		t.Fatalf("identity retained executable bytes: %s", encodedIdentity)
	}
	decodedIdentity, code := DecodeExecutableIdentity(encodedIdentity, candidate, raw, executable)
	if code != "" || !reflect.DeepEqual(decodedIdentity, identity) {
		t.Fatalf("identity round trip failed: code=%s value=%#v", code, decodedIdentity)
	}
	assertExactObjectKeys(t, encodedIdentity, []string{"analyzer", "command_included",
		"descriptor_deterministic", "descriptor_sha256", "environment_included", "error_protocol",
		"executable_bytes", "executable_bytes_included", "executable_format_verified",
		"executable_semantics_verified", "executable_sha256", "metadata_only", "path_included", "process_start_enabled",
		"product_invocation_enabled", "protocol_version", "request_protocol",
		"result_protocol", "target_goarch", "target_goos"})

	preflight, code := BuildInvocationPreflight(candidate, raw, executable, identity)
	if code != "" {
		t.Fatal(code)
	}
	if preflight.ProtocolVersion != InvocationPreflightProtocolVersion ||
		preflight.RequestID != candidate.RequestID || preflight.Analyzer != candidate.Analyzer ||
		preflight.Authority != (InvocationAuthority{}) || !preflight.CandidateValidated ||
		!preflight.DescriptorBound || !preflight.ProtocolsBound || !preflight.PlatformRecorded ||
		!preflight.ExecutableBound || !preflight.TestConformanceOnly || !preflight.MetadataOnly ||
		preflight.ExecutableFormatVerified || preflight.ExecutableSemanticsVerified ||
		preflight.ProcessStartEnabled ||
		preflight.ProductInvocationEnabled {
		t.Fatalf("unsafe or incomplete invocation preflight: %#v", preflight)
	}
	encodedPreflight, code := EncodeInvocationPreflight(preflight, candidate, raw, executable,
		identity)
	if code != "" {
		t.Fatal(code)
	}
	if bytes.Contains(encodedPreflight, executable) {
		t.Fatalf("preflight retained executable bytes: %s", encodedPreflight)
	}
	decodedPreflight, code := DecodeInvocationPreflight(encodedPreflight, candidate, raw,
		executable, decodedIdentity)
	if code != "" || !reflect.DeepEqual(decodedPreflight, preflight) {
		t.Fatalf("preflight round trip failed: code=%s value=%#v", code, decodedPreflight)
	}
	assertExactObjectKeys(t, encodedPreflight, []string{"analyzer", "authority",
		"candidate_sha256", "candidate_validated", "descriptor_bound", "error_protocol",
		"executable_bound", "executable_bytes", "executable_format_verified",
		"executable_identity_sha256", "executable_semantics_verified", "executable_sha256", "metadata_only",
		"platform_recorded", "process_start_enabled", "product_invocation_enabled",
		"protocol_version", "protocols_bound", "request_id", "request_protocol",
		"result_protocol", "target_goarch", "target_goos", "test_conformance_only"})
}

func TestExecutableIdentityAndPreflightRejectDriftAndSchemaWidening(t *testing.T) {
	raw := testRequestJSON(t)
	candidate := mustInvocationCandidate(t, raw)
	executable := []byte("exact-executable")
	identity, code := BuildExecutableIdentity(candidate, raw, executable)
	if code != "" {
		t.Fatal(code)
	}
	preflight, code := BuildInvocationPreflight(candidate, raw, executable, identity)
	if code != "" {
		t.Fatal(code)
	}

	identityMutations := map[string]func(*ExecutableIdentity){
		"digest":    func(value *ExecutableIdentity) { value.ExecutableSHA256 = strings.Repeat("a", 64) },
		"platform":  func(value *ExecutableIdentity) { value.TargetGOOS = "other" },
		"command":   func(value *ExecutableIdentity) { value.CommandIncluded = true },
		"format":    func(value *ExecutableIdentity) { value.ExecutableFormatVerified = true },
		"semantics": func(value *ExecutableIdentity) { value.ExecutableSemanticsVerified = true },
		"authority": func(value *ExecutableIdentity) { value.ProcessStartEnabled = true },
	}
	for name, mutate := range identityMutations {
		t.Run("identity "+name, func(t *testing.T) {
			value := identity
			mutate(&value)
			if got := ValidateExecutableIdentity(value, candidate, raw, executable); got != CodeInvalidResult {
				t.Fatalf("identity drift code = %s", got)
			}
		})
	}
	preflightMutations := map[string]func(*InvocationPreflight){
		"candidate": func(value *InvocationPreflight) { value.CandidateSHA256 = strings.Repeat("b", 64) },
		"identity": func(value *InvocationPreflight) {
			value.ExecutableIdentitySHA256 = strings.Repeat("c", 64)
		},
		"check":     func(value *InvocationPreflight) { value.ProtocolsBound = false },
		"semantics": func(value *InvocationPreflight) { value.ExecutableSemanticsVerified = true },
		"authority": func(value *InvocationPreflight) { value.Authority.ProductInvocation = true },
	}
	for name, mutate := range preflightMutations {
		t.Run("preflight "+name, func(t *testing.T) {
			value := preflight
			mutate(&value)
			if got := ValidateInvocationPreflight(value, candidate, raw, executable,
				identity); got != CodeInvalidResult {
				t.Fatalf("preflight drift code = %s", got)
			}
		})
	}

	encodedIdentity, code := EncodeExecutableIdentity(identity, candidate, raw, executable)
	if code != "" {
		t.Fatal(code)
	}
	identityText := string(encodedIdentity)
	for name, malformed := range map[string]string{
		"future": strings.Replace(identityText, ExecutableIdentityProtocolVersion,
			"analyzer_executable_identity.v2", 1),
		"unknown": strings.Replace(identityText, `"path_included":false`,
			`"path_included":false,"path":"fixture"`, 1),
		"duplicate": strings.Replace(identityText, `"command_included":false`,
			`"command_included":false,"command_included":false`, 1),
		"missing false": strings.Replace(identityText, `,"product_invocation_enabled":false`, "", 1),
	} {
		t.Run("identity schema "+name, func(t *testing.T) {
			if _, got := DecodeExecutableIdentity([]byte(malformed), candidate, raw,
				executable); got != CodeInvalidResult {
				t.Fatalf("identity schema drift code = %s", got)
			}
		})
	}

	encodedPreflight, code := EncodeInvocationPreflight(preflight, candidate, raw, executable,
		identity)
	if code != "" {
		t.Fatal(code)
	}
	preflightText := string(encodedPreflight)
	for name, malformed := range map[string]string{
		"future": strings.Replace(preflightText, InvocationPreflightProtocolVersion,
			"analyzer_invocation_preflight.v2", 1),
		"unknown": strings.Replace(preflightText, `"test_conformance_only":true`,
			`"test_conformance_only":true,"launch":true`, 1),
		"duplicate": strings.Replace(preflightText, `"process_start_enabled":false`,
			`"process_start_enabled":false,"process_start_enabled":false`, 1),
		"missing false": strings.Replace(preflightText,
			`,"executable_format_verified":false`, "", 1),
	} {
		t.Run("preflight schema "+name, func(t *testing.T) {
			if _, got := DecodeInvocationPreflight([]byte(malformed), candidate, raw,
				executable, identity); got != CodeInvalidResult {
				t.Fatalf("preflight schema drift code = %s", got)
			}
		})
	}

	if got := ValidateExecutableIdentity(identity, candidate, raw,
		[]byte("different-executable")); got != CodeInvalidResult {
		t.Fatalf("different executable code = %s", got)
	}
	otherRaw := invocationRequestJSON(t, 500)
	otherCandidate := mustInvocationCandidate(t, otherRaw)
	if got := ValidateInvocationPreflight(preflight, otherCandidate, otherRaw, executable,
		identity); got != CodeInvalidResult {
		t.Fatalf("cross-candidate preflight code = %s", got)
	}
}

func TestExecutableIdentityEnforcesByteBounds(t *testing.T) {
	raw := testRequestJSON(t)
	candidate := mustInvocationCandidate(t, raw)
	if _, code := BuildExecutableIdentity(candidate, raw, nil); code != CodeInvalidContent {
		t.Fatalf("empty executable code = %s", code)
	}
	oversized := make([]byte, MaxAnalyzerExecutableBytes+1)
	if _, code := BuildExecutableIdentity(candidate, raw, oversized); code != CodeInputLimitExceeded {
		t.Fatalf("oversized executable code = %s", code)
	}
}

func FuzzExecutableIdentityEnvelope(f *testing.F) {
	raw, err := json.Marshal(testRequest())
	if err != nil {
		f.Fatal(err)
	}
	candidate, code := BuildInvocationCandidate(raw)
	if code != "" {
		f.Fatal(code)
	}
	executable := []byte("fuzz-executable-identity")
	identity, code := BuildExecutableIdentity(candidate, raw, executable)
	if code != "" {
		f.Fatal(code)
	}
	seed, code := EncodeExecutableIdentity(identity, candidate, raw, executable)
	if code != "" {
		f.Fatal(code)
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, input []byte) {
		decoded, decodeCode := DecodeExecutableIdentity(input, candidate, raw, executable)
		if decodeCode != "" {
			return
		}
		encoded, encodeCode := EncodeExecutableIdentity(decoded, candidate, raw, executable)
		if encodeCode != "" {
			t.Fatalf("accepted identity failed re-encode: %s", encodeCode)
		}
		replayed, replayCode := DecodeExecutableIdentity(encoded, candidate, raw, executable)
		if replayCode != "" || !reflect.DeepEqual(decoded, replayed) {
			t.Fatalf("accepted identity was not idempotent: code=%s", replayCode)
		}
	})
}

func FuzzInvocationPreflightEnvelope(f *testing.F) {
	raw, err := json.Marshal(testRequest())
	if err != nil {
		f.Fatal(err)
	}
	candidate, code := BuildInvocationCandidate(raw)
	if code != "" {
		f.Fatal(code)
	}
	executable := []byte("fuzz-invocation-preflight")
	identity, code := BuildExecutableIdentity(candidate, raw, executable)
	if code != "" {
		f.Fatal(code)
	}
	preflight, code := BuildInvocationPreflight(candidate, raw, executable, identity)
	if code != "" {
		f.Fatal(code)
	}
	seed, code := EncodeInvocationPreflight(preflight, candidate, raw, executable, identity)
	if code != "" {
		f.Fatal(code)
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, input []byte) {
		decoded, decodeCode := DecodeInvocationPreflight(input, candidate, raw, executable,
			identity)
		if decodeCode != "" {
			return
		}
		encoded, encodeCode := EncodeInvocationPreflight(decoded, candidate, raw, executable,
			identity)
		if encodeCode != "" {
			t.Fatalf("accepted preflight failed re-encode: %s", encodeCode)
		}
		replayed, replayCode := DecodeInvocationPreflight(encoded, candidate, raw, executable,
			identity)
		if replayCode != "" || !reflect.DeepEqual(decoded, replayed) {
			t.Fatalf("accepted preflight was not idempotent: code=%s", replayCode)
		}
	})
}
