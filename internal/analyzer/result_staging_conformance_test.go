package analyzer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	testResultStageProtocol = "analyzer_result_stage_test.v1"
	testResultStageMaxBytes = MaxValidatedResultCandidateEnvelopeBytes +
		((MaxResultEnvelopeBytes+2)/3)*4 + 2*1024
)

type testResultStageFault string

const (
	testResultStageNoFault           testResultStageFault = ""
	testResultStageRollbackAfterSync testResultStageFault = "rollback_after_sync"
	testResultStageCrashAfterSync    testResultStageFault = "crash_after_sync"
	testResultStageCrashAfterPublish testResultStageFault = "crash_after_publish"
	testResultStageTruncatedCrash    testResultStageFault = "truncated_crash"
)

var (
	errTestResultStageRolledBack = errors.New("test-only analyzer result stage rolled back")
	errTestResultStageCrash      = errors.New("test-only analyzer result stage crash point")
	errTestResultStageCollision  = errors.New("test-only analyzer result stage collision")
	errTestResultStageMissing    = errors.New("test-only analyzer result stage is missing")
)

type testResultStageInput struct {
	Invocation InvocationCandidate
	RawRequest []byte
	Executable []byte
	Identity   ExecutableIdentity
	Preflight  InvocationPreflight
	Outcome    InvocationOutcome
	RawResult  []byte
	Candidate  ValidatedResultCandidate
}

type testResultStageEnvelope struct {
	ProtocolVersion string          `json:"protocol_version"`
	CandidateSHA256 string          `json:"candidate_sha256"`
	Candidate       json.RawMessage `json:"candidate"`
	ResultBytes     int             `json:"result_bytes"`
	ResultSHA256    string          `json:"result_sha256"`
	ResultBase64    string          `json:"result_base64"`
}

type testResultStageReceipt struct {
	ProtocolVersion          string
	CandidateSHA256          string
	EnvelopeSHA256           string
	ResultBytes              int
	ResultSHA256             string
	Replayed                 bool
	Recovered                bool
	AtomicPublish            bool
	NoReplace                bool
	TestOnly                 bool
	MetadataOnly             bool
	RawResultIncluded        bool
	ProductPersistence       bool
	ArtifactCommitAuthorized bool
	DirectorySyncVerified    bool
}

type testResultStager struct {
	directory string
}

func testStageInput(chain resultCandidateTestChain) testResultStageInput {
	return testResultStageInput{
		Invocation: chain.invocation, RawRequest: append([]byte(nil), chain.raw...),
		Executable: append([]byte(nil), chain.executable...), Identity: chain.identity,
		Preflight: chain.preflight, Outcome: chain.outcome,
		RawResult: append([]byte(nil), chain.rawResult...), Candidate: chain.value,
	}
}

func newTestResultStager(t *testing.T) *testResultStager {
	t.Helper()
	return &testResultStager{directory: t.TempDir()}
}

func (stager *testResultStager) Stage(ctx context.Context, input testResultStageInput,
	fault testResultStageFault,
) (testResultStageReceipt, error) {
	envelope, candidateDigest, resultDigest, err := expectedTestResultStage(input)
	if err != nil {
		return testResultStageReceipt{}, err
	}
	pending, final := stager.paths(candidateDigest)
	if receipt, ok, readErr := stager.readFinal(final, envelope, candidateDigest,
		resultDigest, input); readErr != nil || ok {
		if ok {
			receipt.Replayed = true
		}
		return receipt, readErr
	}
	if err := ctx.Err(); err != nil {
		return testResultStageReceipt{}, err
	}
	file, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return testResultStageReceipt{}, err
	}
	writeBytes := envelope
	if fault == testResultStageTruncatedCrash {
		writeBytes = envelope[:len(envelope)/2]
	}
	written, writeErr := file.Write(writeBytes)
	if writeErr == nil && written != len(writeBytes) {
		writeErr = errors.New("short test-only analyzer result stage write")
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(pending)
		return testResultStageReceipt{}, errors.Join(writeErr, syncErr, closeErr)
	}
	if fault == testResultStageTruncatedCrash || fault == testResultStageCrashAfterSync {
		return testResultStageReceipt{}, errTestResultStageCrash
	}
	if fault == testResultStageRollbackAfterSync {
		if err := os.Remove(pending); err != nil {
			return testResultStageReceipt{}, err
		}
		return testResultStageReceipt{}, errTestResultStageRolledBack
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(pending)
		return testResultStageReceipt{}, err
	}
	if err := os.Link(pending, final); err != nil {
		if receipt, ok, readErr := stager.readFinal(final, envelope, candidateDigest,
			resultDigest, input); readErr == nil && ok {
			if cleanupErr := removeExactTestStageFile(pending, envelope); cleanupErr != nil {
				return testResultStageReceipt{}, cleanupErr
			}
			receipt.Replayed = true
			return receipt, nil
		}
		return testResultStageReceipt{}, err
	}
	if fault == testResultStageCrashAfterPublish {
		return testResultStageReceipt{}, errTestResultStageCrash
	}
	if err := removeExactTestStageFile(pending, envelope); err != nil {
		return testResultStageReceipt{}, err
	}
	return newTestResultStageReceipt(envelope, candidateDigest, resultDigest, input), nil
}

func (stager *testResultStager) Recover(input testResultStageInput,
) (testResultStageReceipt, error) {
	envelope, candidateDigest, resultDigest, err := expectedTestResultStage(input)
	if err != nil {
		return testResultStageReceipt{}, err
	}
	pending, final := stager.paths(candidateDigest)
	if receipt, ok, readErr := stager.readFinal(final, envelope, candidateDigest,
		resultDigest, input); readErr != nil {
		return testResultStageReceipt{}, readErr
	} else if ok {
		if _, err := os.Stat(pending); err == nil {
			if err := removeExactTestStageFile(pending, envelope); err != nil {
				return testResultStageReceipt{}, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return testResultStageReceipt{}, err
		}
		receipt.Recovered = true
		return receipt, nil
	}
	pendingBytes, err := os.ReadFile(pending)
	if errors.Is(err, os.ErrNotExist) {
		return testResultStageReceipt{}, errTestResultStageMissing
	}
	if err != nil {
		return testResultStageReceipt{}, err
	}
	if !bytes.Equal(pendingBytes, envelope) {
		return testResultStageReceipt{}, errTestResultStageCollision
	}
	if err := os.Link(pending, final); err != nil {
		return testResultStageReceipt{}, err
	}
	if err := removeExactTestStageFile(pending, envelope); err != nil {
		return testResultStageReceipt{}, err
	}
	receipt := newTestResultStageReceipt(envelope, candidateDigest, resultDigest, input)
	receipt.Recovered = true
	return receipt, nil
}

func (stager *testResultStager) Rollback(input testResultStageInput) error {
	expected, candidateDigest, _, err := expectedTestResultStage(input)
	if err != nil {
		return err
	}
	pending, _ := stager.paths(candidateDigest)
	content, err := os.ReadFile(pending)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(content) > len(expected) || !bytes.Equal(content, expected[:len(content)]) {
		return errTestResultStageCollision
	}
	return os.Remove(pending)
}

func (stager *testResultStager) paths(candidateDigest string) (string, string) {
	return filepath.Join(stager.directory, candidateDigest+".pending"),
		filepath.Join(stager.directory, candidateDigest+".staged")
}

func (stager *testResultStager) readFinal(path string, expected []byte,
	candidateDigest, resultDigest string, input testResultStageInput,
) (testResultStageReceipt, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return testResultStageReceipt{}, false, nil
	}
	if err != nil {
		return testResultStageReceipt{}, false, err
	}
	if !bytes.Equal(content, expected) {
		return testResultStageReceipt{}, false, errTestResultStageCollision
	}
	return newTestResultStageReceipt(expected, candidateDigest, resultDigest, input), true, nil
}

func expectedTestResultStage(input testResultStageInput) ([]byte, string, string, error) {
	candidateJSON, code := EncodeValidatedResultCandidate(input.Candidate, input.Invocation,
		input.RawRequest, input.Executable, input.Identity, input.Preflight, input.Outcome,
		input.RawResult)
	if code != "" {
		return nil, "", "", errors.New(string(code))
	}
	candidateDigestBytes := sha256.Sum256(candidateJSON)
	candidateDigest := hex.EncodeToString(candidateDigestBytes[:])
	resultDigestBytes := sha256.Sum256(input.RawResult)
	resultDigest := hex.EncodeToString(resultDigestBytes[:])
	value := testResultStageEnvelope{
		ProtocolVersion: testResultStageProtocol, CandidateSHA256: candidateDigest,
		Candidate: append(json.RawMessage(nil), candidateJSON...), ResultBytes: len(input.RawResult),
		ResultSHA256: resultDigest,
		ResultBase64: base64.StdEncoding.EncodeToString(input.RawResult),
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 || len(encoded) > testResultStageMaxBytes {
		return nil, "", "", errors.Join(err, errors.New("test-only result stage exceeds bound"))
	}
	return encoded, candidateDigest, resultDigest, nil
}

func newTestResultStageReceipt(envelope []byte, candidateDigest, resultDigest string,
	input testResultStageInput,
) testResultStageReceipt {
	digest := sha256.Sum256(envelope)
	return testResultStageReceipt{
		ProtocolVersion: testResultStageProtocol, CandidateSHA256: candidateDigest,
		EnvelopeSHA256: hex.EncodeToString(digest[:]), ResultBytes: len(input.RawResult),
		ResultSHA256: resultDigest, AtomicPublish: true, NoReplace: true,
		TestOnly: true, MetadataOnly: true,
	}
}

func removeExactTestStageFile(path string, expected []byte) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, expected) {
		return errTestResultStageCollision
	}
	return os.Remove(path)
}

func assertSafeTestResultStageReceipt(t *testing.T, receipt testResultStageReceipt) {
	t.Helper()
	if receipt.ProtocolVersion != testResultStageProtocol ||
		!validDigest(receipt.CandidateSHA256) || !validDigest(receipt.EnvelopeSHA256) ||
		receipt.ResultBytes <= 0 || !validDigest(receipt.ResultSHA256) ||
		!receipt.AtomicPublish || !receipt.NoReplace || !receipt.TestOnly ||
		!receipt.MetadataOnly || receipt.RawResultIncluded || receipt.ProductPersistence ||
		receipt.ArtifactCommitAuthorized || receipt.DirectorySyncVerified {
		t.Fatalf("unsafe test-only stage receipt: %#v", receipt)
	}
}

func TestAnalyzerResultStagingAtomicSuccessAndReplay(t *testing.T) {
	input := testStageInput(newResultCandidateTestChain(t))
	stager := newTestResultStager(t)
	receipt, err := stager.Stage(t.Context(), input, testResultStageNoFault)
	if err != nil {
		t.Fatal(err)
	}
	assertSafeTestResultStageReceipt(t, receipt)
	pending, final := stager.paths(receipt.CandidateSHA256)
	if _, err := os.Stat(pending); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending file remained: %v", err)
	}
	before, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := stager.Stage(t.Context(), input, testResultStageNoFault)
	if err != nil || !replayed.Replayed || replayed.Recovered {
		t.Fatalf("replay receipt=%#v err=%v", replayed, err)
	}
	after, err := os.ReadFile(final)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("replay rewrote final: err=%v", err)
	}
}

func TestAnalyzerResultStagingRollbackAndCancellation(t *testing.T) {
	input := testStageInput(newResultCandidateTestChain(t))
	stager := newTestResultStager(t)
	if _, err := stager.Stage(t.Context(), input,
		testResultStageRollbackAfterSync); !errors.Is(err, errTestResultStageRolledBack) {
		t.Fatalf("rollback err=%v", err)
	}
	envelope, candidateDigest, _, err := expectedTestResultStage(input)
	if err != nil {
		t.Fatal(err)
	}
	pending, final := stager.paths(candidateDigest)
	for _, path := range []string{pending, final} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rollback retained %s: %v", path, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := stager.Stage(ctx, input, testResultStageNoFault); !errors.Is(err,
		context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
	if _, err := os.Stat(pending); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancel retained pending: %v", err)
	}
	if len(envelope) == 0 {
		t.Fatal("expected non-empty stage envelope")
	}
}

func TestAnalyzerResultStagingCrashRecoveryBeforeAndAfterPublish(t *testing.T) {
	for _, fault := range []testResultStageFault{
		testResultStageCrashAfterSync, testResultStageCrashAfterPublish,
	} {
		t.Run(string(fault), func(t *testing.T) {
			input := testStageInput(newResultCandidateTestChain(t))
			stager := newTestResultStager(t)
			if _, err := stager.Stage(t.Context(), input, fault); !errors.Is(err,
				errTestResultStageCrash) {
				t.Fatalf("crash err=%v", err)
			}
			recovered, err := stager.Recover(input)
			if err != nil || !recovered.Recovered || recovered.Replayed {
				t.Fatalf("recovery receipt=%#v err=%v", recovered, err)
			}
			assertSafeTestResultStageReceipt(t, recovered)
			pending, final := stager.paths(recovered.CandidateSHA256)
			if _, err := os.Stat(pending); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recovery retained pending: %v", err)
			}
			if _, err := os.Stat(final); err != nil {
				t.Fatalf("recovery final missing: %v", err)
			}
		})
	}
}

func TestAnalyzerResultStagingRejectsTruncationAndFinalCollision(t *testing.T) {
	input := testStageInput(newResultCandidateTestChain(t))
	stager := newTestResultStager(t)
	if _, err := stager.Stage(t.Context(), input,
		testResultStageTruncatedCrash); !errors.Is(err, errTestResultStageCrash) {
		t.Fatalf("truncated crash err=%v", err)
	}
	if _, err := stager.Recover(input); !errors.Is(err, errTestResultStageCollision) {
		t.Fatalf("truncated recovery err=%v", err)
	}
	if err := stager.Rollback(input); err != nil {
		t.Fatal(err)
	}
	_, candidateDigest, _, err := expectedTestResultStage(input)
	if err != nil {
		t.Fatal(err)
	}
	_, final := stager.paths(candidateDigest)
	if err := os.WriteFile(final, []byte("foreign-stage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stager.Stage(t.Context(), input,
		testResultStageNoFault); !errors.Is(err, errTestResultStageCollision) {
		t.Fatalf("final collision err=%v", err)
	}
	content, err := os.ReadFile(final)
	if err != nil || string(content) != "foreign-stage" {
		t.Fatalf("foreign final was modified: %q err=%v", content, err)
	}

	t.Run("foreign pending", func(t *testing.T) {
		stager := newTestResultStager(t)
		pending, _ := stager.paths(candidateDigest)
		if err := os.WriteFile(pending, []byte("foreign-pending"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := stager.Rollback(input); !errors.Is(err, errTestResultStageCollision) {
			t.Fatalf("foreign pending rollback err=%v", err)
		}
		content, err := os.ReadFile(pending)
		if err != nil || string(content) != "foreign-pending" {
			t.Fatalf("foreign pending was modified: %q err=%v", content, err)
		}
	})
}
