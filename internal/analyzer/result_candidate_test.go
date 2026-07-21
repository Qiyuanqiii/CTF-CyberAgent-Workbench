package analyzer

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type resultCandidateTestChain struct {
	raw        []byte
	executable []byte
	rawResult  []byte
	invocation InvocationCandidate
	identity   ExecutableIdentity
	preflight  InvocationPreflight
	outcome    InvocationOutcome
	value      ValidatedResultCandidate
}

func newResultCandidateTestChain(t *testing.T) resultCandidateTestChain {
	t.Helper()
	raw := testRequestJSON(t)
	invocation := mustInvocationCandidate(t, raw)
	executable := []byte("test-only-result-candidate-executable")
	identity, code := BuildExecutableIdentity(invocation, raw, executable)
	if code != "" {
		t.Fatal(code)
	}
	preflight, code := BuildInvocationPreflight(invocation, raw, executable, identity)
	if code != "" {
		t.Fatal(code)
	}
	rawResult, exitCode := Evaluate(raw)
	if exitCode != ExitSuccess {
		t.Fatalf("evaluate exit=%d result=%s", exitCode, rawResult)
	}
	outcome := mustInvoke(t, mustFakeTransport(t, FakeTransportPlan{
		Stdout: rawResult, ExitCode: exitCode,
	}), t.Context(), invocation, raw)
	value, code := BuildValidatedResultCandidate(invocation, raw, executable, identity,
		preflight, outcome, rawResult)
	if code != "" {
		t.Fatal(code)
	}
	return resultCandidateTestChain{
		raw: raw, executable: executable, rawResult: rawResult, invocation: invocation,
		identity: identity, preflight: preflight, outcome: outcome, value: value,
	}
}

func TestValidatedResultCandidateSealsExactChainWithoutArtifactAuthority(t *testing.T) {
	chain := newResultCandidateTestChain(t)
	value := chain.value
	if value.ProtocolVersion != ValidatedResultCandidateProtocolVersion ||
		value.RequestID != chain.invocation.RequestID || value.Analyzer != chain.invocation.Analyzer ||
		value.OutcomeStatus != InvocationSucceeded ||
		value.ResultProtocol != chain.invocation.Descriptor.ResultProtocol ||
		value.ResultBytes != len(chain.rawResult) || !validDigest(value.ResultSHA256) ||
		!value.SourceChainBound || !value.ResultEnvelopeValidated || !value.DeterministicMatch ||
		!value.TestConformanceOnly || !value.MetadataOnly || value.RawResultIncluded ||
		value.ExecutableBytesIncluded || value.PathIncluded || value.PersistenceEnabled ||
		value.ArtifactCommitAuthorized || value.ProductInvocationEnabled ||
		value.Authority != (ResultCandidateAuthority{}) {
		t.Fatalf("unsafe or incomplete validated result candidate: %#v", value)
	}
	artifact := value.Artifact
	if artifact.ProtocolVersion != AnalyzerArtifactCandidateProtocolVersion ||
		artifact.Kind != AnalyzerResultArtifactKind ||
		artifact.MediaType != AnalyzerResultArtifactMediaType ||
		artifact.Encoding != AnalyzerResultArtifactEncoding ||
		artifact.SourceProtocol != value.ResultProtocol || artifact.SizeBytes != value.ResultBytes ||
		artifact.SHA256 != value.ResultSHA256 || !artifact.CandidateOnly || !artifact.MetadataOnly ||
		artifact.ContentIncluded || artifact.PathIncluded || artifact.RunBound ||
		artifact.SessionBound || artifact.WorkspaceBound || artifact.PersistenceEnabled ||
		artifact.CommitAuthorized {
		t.Fatalf("unsafe artifact candidate: %#v", artifact)
	}
	encoded, code := EncodeValidatedResultCandidate(value, chain.invocation, chain.raw,
		chain.executable, chain.identity, chain.preflight, chain.outcome, chain.rawResult)
	if code != "" {
		t.Fatal(code)
	}
	if bytes.Contains(encoded, chain.rawResult) || bytes.Contains(encoded, chain.executable) ||
		bytes.Contains(encoded, []byte("alpha\nbeta")) {
		t.Fatalf("candidate retained private bytes: %s", encoded)
	}
	decoded, code := DecodeValidatedResultCandidate(encoded, chain.invocation, chain.raw,
		chain.executable, chain.identity, chain.preflight, chain.outcome, chain.rawResult)
	if code != "" || !reflect.DeepEqual(decoded, value) {
		t.Fatalf("candidate round trip code=%s value=%#v", code, decoded)
	}
	assertExactObjectKeys(t, encoded, []string{"analyzer", "artifact",
		"artifact_commit_authorized", "authority", "candidate_sha256", "deterministic_match",
		"executable_bytes_included", "executable_identity_sha256", "metadata_only",
		"outcome_sha256", "outcome_status", "path_included", "persistence_enabled",
		"preflight_sha256", "product_invocation_enabled", "protocol_version",
		"raw_result_included", "request_id", "result_bytes", "result_envelope_validated",
		"result_protocol", "result_sha256", "source_chain_bound", "test_conformance_only"})
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	assertExactObjectKeys(t, fields["artifact"], []string{"candidate_only",
		"commit_authorized", "content_included", "encoding", "kind", "media_type",
		"metadata_only", "path_included", "persistence_enabled", "protocol_version",
		"run_bound", "session_bound", "sha256", "size_bytes", "source_protocol",
		"workspace_bound"})
	assertExactObjectKeys(t, fields["authority"], []string{"artifact_commit",
		"external_publish", "persistence", "product_invocation", "run_event_write"})
}

func TestValidatedResultCandidateRejectsDriftFailureAndSchemaWidening(t *testing.T) {
	chain := newResultCandidateTestChain(t)
	mutations := map[string]func(*ValidatedResultCandidate){
		"source": func(value *ValidatedResultCandidate) {
			value.OutcomeSHA256 = strings.Repeat("a", 64)
		},
		"result": func(value *ValidatedResultCandidate) {
			value.ResultSHA256 = strings.Repeat("b", 64)
		},
		"raw":       func(value *ValidatedResultCandidate) { value.RawResultIncluded = true },
		"persist":   func(value *ValidatedResultCandidate) { value.PersistenceEnabled = true },
		"commit":    func(value *ValidatedResultCandidate) { value.Artifact.CommitAuthorized = true },
		"authority": func(value *ValidatedResultCandidate) { value.Authority.RunEventWrite = true },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			value := chain.value
			mutate(&value)
			if code := ValidateValidatedResultCandidate(value, chain.invocation, chain.raw,
				chain.executable, chain.identity, chain.preflight, chain.outcome,
				chain.rawResult); code != CodeInvalidResult {
				t.Fatalf("drift code=%s", code)
			}
		})
	}

	changedResult := append([]byte(nil), chain.rawResult...)
	changedResult[len(changedResult)-1] = ' '
	if _, code := BuildValidatedResultCandidate(chain.invocation, chain.raw, chain.executable,
		chain.identity, chain.preflight, chain.outcome, changedResult); code != CodeInvalidResult {
		t.Fatalf("changed result code=%s", code)
	}
	failedOutcome := chain.outcome
	failedOutcome.Status = InvocationFailed
	failedOutcome.FailureCode = InvocationFailureProcess
	failedOutcome.ResultValidated = false
	if _, code := BuildValidatedResultCandidate(chain.invocation, chain.raw, chain.executable,
		chain.identity, chain.preflight, failedOutcome, chain.rawResult); code != CodeInvalidResult {
		t.Fatalf("failed outcome code=%s", code)
	}

	encoded, code := EncodeValidatedResultCandidate(chain.value, chain.invocation, chain.raw,
		chain.executable, chain.identity, chain.preflight, chain.outcome, chain.rawResult)
	if code != "" {
		t.Fatal(code)
	}
	text := string(encoded)
	for name, malformed := range map[string]string{
		"future": strings.Replace(text, ValidatedResultCandidateProtocolVersion,
			"analyzer_validated_result_candidate.v2", 1),
		"unknown": strings.Replace(text, `"raw_result_included":false`,
			`"raw_result_included":false,"result":"private"`, 1),
		"duplicate": strings.Replace(text, `"persistence_enabled":false`,
			`"persistence_enabled":false,"persistence_enabled":false`, 1),
		"missing false": strings.Replace(text, `,"artifact_commit_authorized":false`, "", 1),
		"nested authority": strings.Replace(text, `"artifact_commit":false`,
			`"artifact_commit":false,"write":true`, 1),
	} {
		t.Run("schema "+name, func(t *testing.T) {
			if _, got := DecodeValidatedResultCandidate([]byte(malformed), chain.invocation,
				chain.raw, chain.executable, chain.identity, chain.preflight, chain.outcome,
				chain.rawResult); got != CodeInvalidResult {
				t.Fatalf("schema drift code=%s", got)
			}
		})
	}
}

func FuzzValidatedResultCandidateEnvelope(f *testing.F) {
	raw, err := json.Marshal(testRequest())
	if err != nil {
		f.Fatal(err)
	}
	invocation, code := BuildInvocationCandidate(raw)
	if code != "" {
		f.Fatal(code)
	}
	executable := []byte("fuzz-result-candidate-executable")
	identity, code := BuildExecutableIdentity(invocation, raw, executable)
	if code != "" {
		f.Fatal(code)
	}
	preflight, code := BuildInvocationPreflight(invocation, raw, executable, identity)
	if code != "" {
		f.Fatal(code)
	}
	rawResult, exitCode := Evaluate(raw)
	transport, err := NewFakeTransport(FakeTransportPlan{Stdout: rawResult, ExitCode: exitCode})
	if err != nil {
		f.Fatal(err)
	}
	bridge, err := NewBridge(transport)
	if err != nil {
		f.Fatal(err)
	}
	outcome, code := bridge.Invoke(f.Context(), invocation, raw)
	if code != "" {
		f.Fatal(code)
	}
	value, code := BuildValidatedResultCandidate(invocation, raw, executable, identity,
		preflight, outcome, rawResult)
	if code != "" {
		f.Fatal(code)
	}
	seed, code := EncodeValidatedResultCandidate(value, invocation, raw, executable, identity,
		preflight, outcome, rawResult)
	if code != "" {
		f.Fatal(code)
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, input []byte) {
		decoded, decodeCode := DecodeValidatedResultCandidate(input, invocation, raw,
			executable, identity, preflight, outcome, rawResult)
		if decodeCode != "" {
			return
		}
		encoded, encodeCode := EncodeValidatedResultCandidate(decoded, invocation, raw,
			executable, identity, preflight, outcome, rawResult)
		if encodeCode != "" {
			t.Fatalf("accepted candidate failed re-encode: %s", encodeCode)
		}
		replayed, replayCode := DecodeValidatedResultCandidate(encoded, invocation, raw,
			executable, identity, preflight, outcome, rawResult)
		if replayCode != "" || !reflect.DeepEqual(decoded, replayed) {
			t.Fatalf("accepted candidate was not idempotent: code=%s", replayCode)
		}
	})
}
