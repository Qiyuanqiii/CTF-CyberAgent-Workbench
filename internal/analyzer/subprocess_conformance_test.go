//go:build windows || aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package analyzer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	analyzerConformanceBinaryEnv = "CYBERAGENT_ANALYZER_CONFORMANCE_BINARY"
	analyzerHelperModeEnv        = "CYBERAGENT_ANALYZER_HELPER_MODE"
	analyzerHelperDirEnv         = "CYBERAGENT_ANALYZER_HELPER_DIR"

	analyzerHelperMalformed            = "malformed-output"
	analyzerHelperFuture               = "future-output"
	analyzerHelperWrong                = "wrong-analyzer"
	analyzerHelperStderr               = "stderr-private"
	analyzerHelperTree                 = "tree-hold"
	analyzerHelperTreeIgnore           = "tree-ignore-terminate"
	analyzerHelperOrphan               = "tree-parent-exits"
	analyzerHelperChild                = "tree-child"
	subprocessConformanceTransportName = "subprocess_conformance_test"

	maxConformanceStderrCaptureBytes = 4 * 1024
	conformanceTerminateGrace        = 200 * time.Millisecond
	conformanceCleanupTimeout        = 5 * time.Second
)

type conformanceTreeState struct {
	ParentRunning  bool
	ChildRunning   bool
	TreeReaped     bool
	OrphanDetected bool
}

type conformanceProcessController interface {
	Terminate(context.Context) error
	Kill(context.Context) error
	State(context.Context) (conformanceTreeState, error)
	SetChildPID(int)
	SetStopMarker(string)
	Close() error
}

type conformanceStreamEvidence struct {
	ObservedBytes      int
	CapturedBytes      int
	CapturedSHA256     string
	Truncated          bool
	RawContentIncluded bool
}

type subprocessConformanceEvidence struct {
	ExecutableIdentitySHA256 string
	PreflightSHA256          string
	Started                  bool
	TerminateRequested       bool
	KillRequested            bool
	TreeReaped               bool
	OrphanDetected           bool
	Stdout                   conformanceStreamEvidence
	Stderr                   conformanceStreamEvidence
	TestConformanceOnly      bool
	RawStderrIncluded        bool
	ProductInvocationEnabled bool
}

type boundedConformanceCollector struct {
	mu       sync.Mutex
	limit    int
	observed int
	captured []byte
}

func newBoundedConformanceCollector(limit int) *boundedConformanceCollector {
	return &boundedConformanceCollector{limit: limit}
}

func (collector *boundedConformanceCollector) Write(value []byte) (int, error) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	written := len(value)
	if collector.observed <= collector.limit {
		if written > collector.limit+1-collector.observed {
			collector.observed = collector.limit + 1
		} else {
			collector.observed += written
		}
	}
	remaining := collector.limit + 1 - len(collector.captured)
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		collector.captured = append(collector.captured, value...)
	}
	return written, nil
}

func (collector *boundedConformanceCollector) bytes() []byte {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return append([]byte(nil), collector.captured...)
}

func (collector *boundedConformanceCollector) evidence() conformanceStreamEvidence {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	digest := ""
	if len(collector.captured) > 0 {
		sum := sha256.Sum256(collector.captured)
		digest = hex.EncodeToString(sum[:])
	}
	return conformanceStreamEvidence{
		ObservedBytes: collector.observed, CapturedBytes: len(collector.captured),
		CapturedSHA256: digest, Truncated: collector.observed > collector.limit,
	}
}

type subprocessConformanceSpec struct {
	executable      string
	arguments       []string
	environment     []string
	directory       string
	holdStdin       bool
	crashAfterStart bool
	identity        ExecutableIdentity
	preflight       InvocationPreflight
	executableData  []byte
}

type subprocessConformanceTransport struct {
	spec    subprocessConformanceSpec
	started chan struct{}

	mu       sync.Mutex
	evidence subprocessConformanceEvidence
	claimed  bool
}

func newRustSubprocessConformanceTransport(t *testing.T, candidate InvocationCandidate,
	rawRequest []byte, holdStdin, crashAfterStart bool,
) *subprocessConformanceTransport {
	t.Helper()
	executable := os.Getenv(analyzerConformanceBinaryEnv)
	if executable == "" {
		t.Skipf("%s is not set; Rust subprocess conformance is an explicit CI/local gate",
			analyzerConformanceBinaryEnv)
	}
	return newSubprocessConformanceTransport(t, executable, nil, nil, candidate, rawRequest,
		holdStdin, crashAfterStart)
}

func newHelperSubprocessConformanceTransport(t *testing.T, mode string,
	candidate InvocationCandidate, rawRequest []byte,
) *subprocessConformanceTransport {
	t.Helper()
	directory := t.TempDir()
	environment := []string{analyzerHelperModeEnv + "=" + mode,
		analyzerHelperDirEnv + "=" + directory}
	return newSubprocessConformanceTransport(t, os.Args[0],
		[]string{"-test.run=^TestAnalyzerSubprocessConformanceHelper$"}, environment,
		candidate, rawRequest, false, false)
}

func newSubprocessConformanceTransport(t *testing.T, executable string, arguments,
	environment []string, candidate InvocationCandidate, rawRequest []byte,
	holdStdin, crashAfterStart bool,
) *subprocessConformanceTransport {
	t.Helper()
	absolute, err := filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("conformance executable must be a regular file: %v", err)
	}
	executableData, err := os.ReadFile(absolute)
	if err != nil {
		t.Fatal(err)
	}
	identity, code := BuildExecutableIdentity(candidate, rawRequest, executableData)
	if code != "" {
		t.Fatalf("build executable identity: %s", code)
	}
	preflight, code := BuildInvocationPreflight(candidate, rawRequest, executableData, identity)
	if code != "" {
		t.Fatalf("build invocation preflight: %s", code)
	}
	directory := t.TempDir()
	return &subprocessConformanceTransport{
		spec: subprocessConformanceSpec{
			executable: absolute, arguments: append([]string(nil), arguments...),
			environment: append([]string(nil), environment...), directory: directory,
			holdStdin: holdStdin, crashAfterStart: crashAfterStart,
			identity: identity, preflight: preflight,
			executableData: executableData,
		},
		started: make(chan struct{}),
	}
}

func (*subprocessConformanceTransport) analyzerTransport() {}
func (*subprocessConformanceTransport) name() string {
	return subprocessConformanceTransportName
}

func (transport *subprocessConformanceTransport) exchange(ctx context.Context,
	candidate InvocationCandidate, rawRequest []byte,
) (transportExchange, error) {
	if transport == nil {
		return transportExchange{}, errors.New("nil subprocess conformance transport")
	}
	transport.mu.Lock()
	if transport.claimed {
		transport.mu.Unlock()
		return transportExchange{}, errors.New("subprocess conformance transport is single use")
	}
	transport.claimed = true
	transport.mu.Unlock()
	currentExecutable, err := os.ReadFile(transport.spec.executable)
	if err != nil || !bytes.Equal(currentExecutable, transport.spec.executableData) {
		return transportExchange{}, errors.New("conformance executable changed after preflight")
	}
	if code := ValidateExecutableIdentity(transport.spec.identity, candidate, rawRequest,
		currentExecutable); code != "" {
		return transportExchange{}, fmt.Errorf("executable identity validation failed: %s", code)
	}
	if code := ValidateInvocationPreflight(transport.spec.preflight, candidate, rawRequest,
		currentExecutable, transport.spec.identity); code != "" {
		return transportExchange{}, fmt.Errorf("invocation preflight validation failed: %s", code)
	}

	command := exec.Command(transport.spec.executable, transport.spec.arguments...)
	command.Env = isolatedConformanceEnvironment(transport.spec.environment)
	command.Dir = transport.spec.directory
	stdin, err := command.StdinPipe()
	if err != nil {
		return transportExchange{}, err
	}
	stdout := newBoundedConformanceCollector(MaxFakeTransportStdoutBytes - 1)
	stderr := newBoundedConformanceCollector(maxConformanceStderrCaptureBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	controller, err := startSubprocessConformanceProcess(command)
	if err != nil {
		_ = stdin.Close()
		return transportExchange{}, err
	}
	transport.markStarted()
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	if transport.spec.crashAfterStart {
		transport.markKillRequested()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), conformanceCleanupTimeout)
		_ = controller.Kill(cleanupCtx)
		cancel()
	}
	if !transport.spec.holdStdin {
		writeDone := make(chan error, 1)
		go func() {
			_, writeErr := stdin.Write(rawRequest)
			writeDone <- errors.Join(writeErr, stdin.Close())
		}()
		select {
		case <-writeDone:
		case <-ctx.Done():
			return transport.finishCancelled(ctx.Err(), controller, stdin, wait,
				stdout, stderr)
		case <-wait:
			_ = stdin.Close()
			return transport.finishExited(command, controller, stdout, stderr)
		}
	}

	select {
	case <-ctx.Done():
		return transport.finishCancelled(ctx.Err(), controller, stdin, wait,
			stdout, stderr)
	case <-wait:
		_ = stdin.Close()
		return transport.finishExited(command, controller, stdout, stderr)
	}
}

func (transport *subprocessConformanceTransport) finishExited(command *exec.Cmd,
	controller conformanceProcessController, stdout,
	stderr *boundedConformanceCollector,
) (transportExchange, error) {
	defer controller.Close()
	state := conformanceTreeState{}
	inspectCtx, cancel := context.WithTimeout(context.Background(), conformanceCleanupTimeout)
	if observed, err := controller.State(inspectCtx); err == nil {
		state = observed
	}
	cancel()
	transport.recordEvidence(stdout, stderr, state)
	if command.ProcessState == nil {
		return transportExchange{}, errors.New("conformance process state is unavailable")
	}
	exitCode := command.ProcessState.ExitCode()
	if exitCode < 0 {
		return transportExchange{}, errors.New("conformance process terminated without an exit code")
	}
	return transportExchange{stdout: stdout.bytes(), exitCode: exitCode}, nil
}

func (transport *subprocessConformanceTransport) finishCancelled(cause error,
	controller conformanceProcessController, stdin io.Closer, wait <-chan error,
	stdout, stderr *boundedConformanceCollector,
) (transportExchange, error) {
	transport.markTerminateRequested()
	terminateCtx, terminateCancel := context.WithTimeout(context.Background(),
		conformanceTerminateGrace)
	_ = controller.Terminate(terminateCtx)
	terminateCancel()

	timer := time.NewTimer(conformanceTerminateGrace)
	defer timer.Stop()
	select {
	case <-wait:
	case <-timer.C:
		transport.markKillRequested()
		killCtx, killCancel := context.WithTimeout(context.Background(),
			conformanceCleanupTimeout)
		_ = controller.Kill(killCtx)
		killCancel()
		cleanupTimer := time.NewTimer(conformanceCleanupTimeout)
		select {
		case <-wait:
		case <-cleanupTimer.C:
		}
		if !cleanupTimer.Stop() {
			select {
			case <-cleanupTimer.C:
			default:
			}
		}
	}
	_ = stdin.Close()
	state := conformanceTreeState{}
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(),
		conformanceCleanupTimeout)
	if observed, err := controller.State(inspectCtx); err == nil {
		state = observed
	}
	inspectCancel()
	transport.recordEvidence(stdout, stderr, state)
	_ = controller.Close()
	return transportExchange{}, cause
}

func (transport *subprocessConformanceTransport) markStarted() {
	transport.mu.Lock()
	if !transport.evidence.Started {
		transport.evidence.Started = true
		close(transport.started)
	}
	transport.mu.Unlock()
}

func (transport *subprocessConformanceTransport) markTerminateRequested() {
	transport.mu.Lock()
	transport.evidence.TerminateRequested = true
	transport.mu.Unlock()
}

func (transport *subprocessConformanceTransport) markKillRequested() {
	transport.mu.Lock()
	transport.evidence.KillRequested = true
	transport.mu.Unlock()
}

func (transport *subprocessConformanceTransport) recordEvidence(
	stdout, stderr *boundedConformanceCollector,
	state conformanceTreeState,
) {
	identityDigest, _ := canonicalSHA256(transport.spec.identity)
	preflightDigest, _ := canonicalSHA256(transport.spec.preflight)
	transport.mu.Lock()
	transport.evidence.ExecutableIdentitySHA256 = identityDigest
	transport.evidence.PreflightSHA256 = preflightDigest
	transport.evidence.TreeReaped = state.TreeReaped
	transport.evidence.OrphanDetected = state.OrphanDetected
	transport.evidence.Stdout = stdout.evidence()
	transport.evidence.Stderr = stderr.evidence()
	transport.evidence.TestConformanceOnly = true
	transport.mu.Unlock()
}

func (transport *subprocessConformanceTransport) Evidence() subprocessConformanceEvidence {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.evidence
}

type subprocessConformanceBridge struct {
	bridge *Bridge
}

func (bridge *subprocessConformanceBridge) Invoke(ctx context.Context,
	candidate InvocationCandidate, rawRequest []byte,
) (InvocationOutcome, ErrorCode) {
	return bridge.bridge.invoke(ctx, candidate, rawRequest,
		validateSubprocessConformanceOutcome)
}

func validateSubprocessConformanceOutcome(candidate InvocationCandidate,
	outcome InvocationOutcome,
) bool {
	return validateInvocationOutcome(candidate, outcome, func(name string) bool {
		return name == subprocessConformanceTransportName
	})
}

func conformanceBridge(t *testing.T,
	transport *subprocessConformanceTransport,
) *subprocessConformanceBridge {
	t.Helper()
	if _, err := NewBridge(transport); err == nil {
		t.Fatal("product bridge admitted a subprocess conformance transport")
	}
	return &subprocessConformanceBridge{bridge: &Bridge{transport: transport}}
}

func isolatedConformanceEnvironment(values []string) []string {
	// exec.Cmd uses nil to mean "inherit the parent environment". Always
	// allocate, including for the intentionally empty Rust fixture environment.
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func TestSubprocessConformanceEmptyEnvironmentDoesNotMeanInherit(t *testing.T) {
	isolated := isolatedConformanceEnvironment(nil)
	if isolated == nil || len(isolated) != 0 {
		t.Fatalf("isolated empty environment = %#v", isolated)
	}
	source := []string{"ONLY=fixture"}
	isolated = isolatedConformanceEnvironment(source)
	source[0] = "MUTATED=yes"
	if len(isolated) != 1 || isolated[0] != "ONLY=fixture" {
		t.Fatalf("isolated environment was not copied: %#v", isolated)
	}
}

func TestSubprocessConformanceOutcomeCannotCrossProductCodec(t *testing.T) {
	raw := invocationRequestJSON(t, 5000)
	candidate := mustInvocationCandidate(t, raw)
	transport := newHelperSubprocessConformanceTransport(t, analyzerHelperStderr,
		candidate, raw)
	outcome, code := conformanceBridge(t, transport).Invoke(t.Context(), candidate, raw)
	if code != "" || !validateSubprocessConformanceOutcome(candidate, outcome) {
		t.Fatalf("test-only outcome=%#v code=%s", outcome, code)
	}
	if ValidateInvocationOutcome(candidate, outcome) {
		t.Fatal("product validator admitted a test-only subprocess outcome")
	}
	if _, code = EncodeInvocationOutcome(candidate, outcome); code != CodeInvalidResult {
		t.Fatalf("product encode code=%s, want %s", code, CodeInvalidResult)
	}
	encoded, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	if _, code = DecodeInvocationOutcome(encoded, candidate); code != CodeInvalidResult {
		t.Fatalf("product decode code=%s, want %s", code, CodeInvalidResult)
	}
}

func TestSubprocessConformanceRustFixtureSuccessAndRejection(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		raw := invocationRequestJSON(t, 1000)
		candidate := mustInvocationCandidate(t, raw)
		transport := newRustSubprocessConformanceTransport(t, candidate, raw, false, false)
		outcome, code := conformanceBridge(t, transport).Invoke(t.Context(), candidate, raw)
		if code != "" || outcome.Status != InvocationSucceeded ||
			outcome.Transport != subprocessConformanceTransportName || !outcome.ResultValidated ||
			outcome.ProductInvocationEnabled || outcome.RawOutputIncluded {
			t.Fatalf("Rust success outcome=%#v code=%s", outcome, code)
		}
		assertSubprocessConformanceEvidence(t, transport.Evidence(), false, false)
	})

	t.Run("runtime rejection", func(t *testing.T) {
		raw := archiveInvocationRequestJSON(t, []byte("not a zip"), 1000)
		candidate := mustInvocationCandidate(t, raw)
		transport := newRustSubprocessConformanceTransport(t, candidate, raw, false, false)
		outcome, code := conformanceBridge(t, transport).Invoke(t.Context(), candidate, raw)
		if code != "" || outcome.Status != InvocationRejected ||
			outcome.AnalyzerErrorCode != CodeInvalidContent || !outcome.ResultValidated {
			t.Fatalf("Rust rejection outcome=%#v code=%s", outcome, code)
		}
		assertSubprocessConformanceEvidence(t, transport.Evidence(), false, false)
	})
}

func archiveInvocationRequestJSON(t *testing.T, content []byte, timeoutMS int) []byte {
	t.Helper()
	request := testRequest()
	request.Analyzer = ArchiveAnalyzerName
	request.Input.MediaType = "application/zip"
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString(content)
	request.Limits.TimeoutMilliseconds = timeoutMS
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSubprocessConformanceRustFixtureTimeoutCancellationAndCrash(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		raw := invocationRequestJSON(t, MinTimeoutMilliseconds)
		candidate := mustInvocationCandidate(t, raw)
		transport := newRustSubprocessConformanceTransport(t, candidate, raw, true, false)
		outcome, code := conformanceBridge(t, transport).Invoke(t.Context(), candidate, raw)
		if code != "" || outcome.Status != InvocationTimedOut ||
			outcome.FailureCode != InvocationFailureDeadline {
			t.Fatalf("Rust timeout outcome=%#v code=%s", outcome, code)
		}
		assertSubprocessConformanceEvidence(t, transport.Evidence(), true,
			conformanceCancellationRequiresKill())
	})

	t.Run("in flight cancellation", func(t *testing.T) {
		raw := invocationRequestJSON(t, 5000)
		candidate := mustInvocationCandidate(t, raw)
		transport := newRustSubprocessConformanceTransport(t, candidate, raw, true, false)
		bridge := conformanceBridge(t, transport)
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan InvocationOutcome, 1)
		codes := make(chan ErrorCode, 1)
		go func() {
			outcome, code := bridge.Invoke(ctx, candidate, raw)
			result <- outcome
			codes <- code
		}()
		select {
		case <-transport.started:
			cancel()
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("Rust conformance process did not start")
		}
		outcome, code := <-result, <-codes
		if code != "" || outcome.Status != InvocationCancelled ||
			outcome.FailureCode != InvocationFailureCancelled {
			t.Fatalf("Rust cancellation outcome=%#v code=%s", outcome, code)
		}
		assertSubprocessConformanceEvidence(t, transport.Evidence(), true,
			conformanceCancellationRequiresKill())
	})

	t.Run("forced crash", func(t *testing.T) {
		raw := invocationRequestJSON(t, 1000)
		candidate := mustInvocationCandidate(t, raw)
		transport := newRustSubprocessConformanceTransport(t, candidate, raw, true, true)
		outcome, code := conformanceBridge(t, transport).Invoke(t.Context(), candidate, raw)
		if code != "" || outcome.Status != InvocationFailed ||
			outcome.FailureCode != InvocationFailureProcess {
			t.Fatalf("Rust crash outcome=%#v code=%s", outcome, code)
		}
		assertSubprocessConformanceEvidence(t, transport.Evidence(), false, true)
	})
}

func TestSubprocessConformanceAdverseOutputAndStderrPrivacy(t *testing.T) {
	// Race instrumentation substantially slows a freshly spawned Go test
	// helper on Windows; keep this protocol test below the global 30s bound
	// without conflating helper startup with the dedicated timeout vectors.
	raw := invocationRequestJSON(t, 5000)
	candidate := mustInvocationCandidate(t, raw)
	for _, mode := range []string{analyzerHelperMalformed, analyzerHelperFuture,
		analyzerHelperWrong} {
		t.Run(mode, func(t *testing.T) {
			transport := newHelperSubprocessConformanceTransport(t, mode, candidate, raw)
			outcome, code := conformanceBridge(t, transport).Invoke(t.Context(), candidate, raw)
			if code != "" || outcome.Status != InvocationFailed ||
				outcome.FailureCode != InvocationFailureInvalidResult ||
				outcome.RawOutputIncluded || outcome.ProductInvocationEnabled {
				t.Fatalf("adverse output outcome=%#v code=%s", outcome, code)
			}
			assertSubprocessConformanceEvidence(t, transport.Evidence(), false, false)
		})
	}

	t.Run("stderr metadata only", func(t *testing.T) {
		transport := newHelperSubprocessConformanceTransport(t, analyzerHelperStderr,
			candidate, raw)
		outcome, code := conformanceBridge(t, transport).Invoke(t.Context(), candidate, raw)
		evidence := transport.Evidence()
		if code != "" || outcome.Status != InvocationSucceeded ||
			evidence.Stderr.ObservedBytes == 0 || evidence.Stderr.CapturedSHA256 == "" ||
			evidence.RawStderrIncluded || outcome.RawOutputIncluded {
			t.Fatalf("stderr privacy outcome=%#v evidence=%#v code=%s", outcome, evidence, code)
		}
		encoded, err := json.Marshal(struct {
			Outcome  InvocationOutcome             `json:"outcome"`
			Evidence subprocessConformanceEvidence `json:"evidence"`
		}{Outcome: outcome, Evidence: evidence})
		if err != nil || bytes.Contains(encoded, []byte("private-stderr-marker")) {
			t.Fatalf("stderr leaked into test evidence: err=%v envelope=%s", err, encoded)
		}
	})
}

func assertSubprocessConformanceEvidence(t *testing.T,
	evidence subprocessConformanceEvidence, terminateRequested, killRequested bool,
) {
	t.Helper()
	if !evidence.Started || !validDigest(evidence.ExecutableIdentitySHA256) ||
		!validDigest(evidence.PreflightSHA256) ||
		evidence.TerminateRequested != terminateRequested || evidence.KillRequested != killRequested ||
		!evidence.TreeReaped || evidence.OrphanDetected || !evidence.TestConformanceOnly ||
		evidence.RawStderrIncluded || evidence.ProductInvocationEnabled ||
		evidence.Stdout.RawContentIncluded || evidence.Stderr.RawContentIncluded {
		t.Fatalf("unsafe or incomplete subprocess conformance evidence: %#v", evidence)
	}
}

func TestAnalyzerProcessEntryPointsRemainTestOnly(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve analyzer package directory")
	}
	directory := filepath.Dir(currentFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]struct{}{
		"C": {}, "os": {}, "os/exec": {}, "syscall": {},
		"golang.org/x/sys/unix": {}, "golang.org/x/sys/windows": {},
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, name),
			nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, imported := range parsed.Imports {
			path, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				t.Fatal(unquoteErr)
			}
			if _, denied := forbidden[path]; denied {
				t.Fatalf("production analyzer file %s imports process package %q", name, path)
			}
		}
	}
}

func TestAnalyzerSubprocessConformanceHelper(t *testing.T) {
	mode := os.Getenv(analyzerHelperModeEnv)
	if mode == "" {
		return
	}
	directory := os.Getenv(analyzerHelperDirEnv)
	if directory == "" {
		t.Fatal("analyzer helper directory is required")
	}
	if mode == analyzerHelperChild {
		waitForAnalyzerMarker(t, filepath.Join(directory, "stop"))
		os.Exit(0)
	}
	if mode == analyzerHelperTree || mode == analyzerHelperTreeIgnore ||
		mode == analyzerHelperOrphan {
		runAnalyzerTreeHelper(t, mode, directory)
		os.Exit(0)
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, MaxRequestEnvelopeBytes+1))
	if err != nil {
		t.Fatal(err)
	}
	output, _ := Evaluate(raw)
	switch mode {
	case analyzerHelperMalformed:
		output = []byte(`{"protocol_version":`)
	case analyzerHelperFuture:
		output = bytes.Replace(output, []byte(ResultProtocolVersion),
			[]byte("analyzer_result.v2"), 1)
	case analyzerHelperWrong:
		output = bytes.Replace(output, []byte(FixtureAnalyzerName),
			[]byte("fixture.wrong.v1"), 1)
	case analyzerHelperStderr:
		if _, err := fmt.Fprint(os.Stderr, "private-stderr-marker\n"); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown analyzer helper mode %q", mode)
	}
	if _, err := os.Stdout.Write(output); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func runAnalyzerTreeHelper(t *testing.T, mode, directory string) {
	waitForAnalyzerMarker(t, filepath.Join(directory, "assigned"))
	child := exec.Command(os.Args[0], "-test.run=^TestAnalyzerSubprocessConformanceHelper$")
	child.Env = []string{analyzerHelperModeEnv + "=" + analyzerHelperChild,
		analyzerHelperDirEnv + "=" + directory}
	child.Stdin = nil
	child.Stdout = io.Discard
	child.Stderr = io.Discard
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "child.pid"),
		[]byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatal(err)
	}
	if mode == analyzerHelperOrphan {
		return
	}
	if mode == analyzerHelperTreeIgnore {
		ignoreAnalyzerTerminateSignals()
	}
	if err := writeAnalyzerMarker(filepath.Join(directory, "tree.ready")); err != nil {
		t.Fatal(err)
	}
	waitForAnalyzerMarker(t, filepath.Join(directory, "stop"))
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
}

func waitForAnalyzerMarker(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", filepath.Base(path))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForAnalyzerChildPID(ctx context.Context, directory string) (int, error) {
	for {
		content, err := os.ReadFile(filepath.Join(directory, "child.pid"))
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(content)))
			if parseErr == nil && pid > 0 {
				return pid, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func writeAnalyzerMarker(path string) error {
	return os.WriteFile(path, []byte("ready\n"), 0o600)
}

type runningAnalyzerConformanceTree struct {
	controller conformanceProcessController
	directory  string
	done       chan struct{}
	closeOnce  sync.Once

	waitMu  sync.Mutex
	waitErr error
}

func startAnalyzerConformanceTree(t *testing.T, mode string) *runningAnalyzerConformanceTree {
	t.Helper()
	directory := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestAnalyzerSubprocessConformanceHelper$")
	command.Env = []string{analyzerHelperModeEnv + "=" + mode,
		analyzerHelperDirEnv + "=" + directory}
	command.Dir = directory
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	controller, err := startSubprocessConformanceProcess(command)
	if err != nil {
		t.Fatal(err)
	}
	controller.SetStopMarker(filepath.Join(directory, "stop"))
	tree := &runningAnalyzerConformanceTree{
		controller: controller, directory: directory, done: make(chan struct{}),
	}
	go func() {
		err := command.Wait()
		tree.waitMu.Lock()
		tree.waitErr = err
		tree.waitMu.Unlock()
		close(tree.done)
	}()
	t.Cleanup(func() { tree.cleanup() })
	if err := writeAnalyzerMarker(filepath.Join(directory, "assigned")); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	childPID, err := waitForAnalyzerChildPID(waitCtx, directory)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	controller.SetChildPID(childPID)
	if mode != analyzerHelperOrphan {
		waitForAnalyzerMarker(t, filepath.Join(directory, "tree.ready"))
	}
	return tree
}

func (tree *runningAnalyzerConformanceTree) waitForExit(ctx context.Context) error {
	select {
	case <-tree.done:
		tree.waitMu.Lock()
		defer tree.waitMu.Unlock()
		return tree.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (tree *runningAnalyzerConformanceTree) waitForState(ctx context.Context,
	predicate func(conformanceTreeState) bool,
) (conformanceTreeState, error) {
	for {
		state, err := tree.controller.State(ctx)
		if err != nil {
			return conformanceTreeState{}, err
		}
		if predicate(state) {
			return state, nil
		}
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (tree *runningAnalyzerConformanceTree) cleanup() {
	tree.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), conformanceCleanupTimeout)
		_ = tree.controller.Kill(ctx)
		cancel()
		select {
		case <-tree.done:
		case <-time.After(conformanceCleanupTimeout):
		}
		_ = tree.controller.Close()
	})
}

func TestSubprocessConformanceProcessTreeGracefulAndForcedReap(t *testing.T) {
	t.Run("graceful descendant reap", func(t *testing.T) {
		tree := startAnalyzerConformanceTree(t, analyzerHelperTree)
		if err := writeAnalyzerMarker(filepath.Join(tree.directory, "stop")); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(t.Context(), conformanceCleanupTimeout)
		if err := tree.waitForExit(waitCtx); err != nil {
			cancel()
			t.Fatal(err)
		}
		state, err := tree.waitForState(waitCtx,
			func(value conformanceTreeState) bool { return value.TreeReaped })
		cancel()
		if err != nil || !state.TreeReaped || state.OrphanDetected {
			t.Fatalf("graceful tree state=%#v err=%v", state, err)
		}
	})

	t.Run("terminate then hard stop", func(t *testing.T) {
		tree := startAnalyzerConformanceTree(t, analyzerHelperTreeIgnore)
		terminateCtx, terminateCancel := context.WithTimeout(t.Context(),
			conformanceTerminateGrace)
		if err := tree.controller.Terminate(terminateCtx); err != nil &&
			!errors.Is(err, context.DeadlineExceeded) {
			terminateCancel()
			t.Fatal(err)
		}
		terminateCancel()
		time.Sleep(conformanceTerminateGrace)
		killCtx, killCancel := context.WithTimeout(t.Context(), conformanceCleanupTimeout)
		if err := tree.controller.Kill(killCtx); err != nil {
			killCancel()
			t.Fatal(err)
		}
		if err := tree.waitForExit(killCtx); err != nil {
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				killCancel()
				t.Fatal(err)
			}
		}
		state, err := tree.waitForState(killCtx,
			func(value conformanceTreeState) bool { return value.TreeReaped })
		killCancel()
		if err != nil || !state.TreeReaped || state.OrphanDetected {
			t.Fatalf("forced tree state=%#v err=%v", state, err)
		}
	})
}

func TestSubprocessConformanceDetectsAndCleansOrphan(t *testing.T) {
	tree := startAnalyzerConformanceTree(t, analyzerHelperOrphan)
	waitCtx, waitCancel := context.WithTimeout(t.Context(), conformanceCleanupTimeout)
	if err := tree.waitForExit(waitCtx); err != nil {
		waitCancel()
		t.Fatal(err)
	}
	state, err := tree.waitForState(waitCtx,
		func(value conformanceTreeState) bool { return value.OrphanDetected })
	waitCancel()
	if err != nil || !state.OrphanDetected || state.ParentRunning || !state.ChildRunning {
		t.Fatalf("orphan state=%#v err=%v", state, err)
	}

	terminateCtx, terminateCancel := context.WithTimeout(t.Context(),
		conformanceTerminateGrace)
	_ = tree.controller.Terminate(terminateCtx)
	state, err = tree.waitForState(terminateCtx,
		func(value conformanceTreeState) bool { return value.TreeReaped })
	terminateCancel()
	if err != nil {
		killCtx, killCancel := context.WithTimeout(t.Context(), conformanceCleanupTimeout)
		if killErr := tree.controller.Kill(killCtx); killErr != nil {
			killCancel()
			t.Fatal(killErr)
		}
		state, err = tree.waitForState(killCtx,
			func(value conformanceTreeState) bool { return value.TreeReaped })
		killCancel()
	}
	if err != nil || !state.TreeReaped || state.OrphanDetected {
		t.Fatalf("orphan cleanup state=%#v err=%v", state, err)
	}
}
