package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

const evidenceSetReceiptGoldenProtocol = "runner_evidence_set_receipt_golden_vectors.v1"

type evidenceSetReceiptGoldenFile struct {
	ProtocolVersion string                           `json:"protocol_version"`
	Vectors         []evidenceSetReceiptGoldenVector `json:"vectors"`
}

type evidenceSetReceiptGoldenVector struct {
	Name            string `json:"name"`
	CanonicalBytes  int    `json:"canonical_bytes"`
	CanonicalSHA256 string `json:"canonical_sha256"`
}

type evidenceSetGoldenInput struct {
	Exit     ExitEvidence
	Runtime  RuntimeEvidence
	Limits   ResourceLimitEvidence
	Cause    TerminationCauseEvidence
	Timeline LifecycleTimelineEvidence
	Budget   DeadlineBudgetEvidence
}

func TestEvidenceSetReceiptGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/evidence_set_receipt_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var golden evidenceSetReceiptGoldenFile
	if err := decoder.Decode(&golden); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("golden vectors contain trailing JSON: %v", err)
	}
	if golden.ProtocolVersion != evidenceSetReceiptGoldenProtocol || len(golden.Vectors) != 2 {
		t.Fatalf("golden vector envelope is invalid: %#v", golden)
	}
	seen := make(map[string]struct{}, len(golden.Vectors))
	for _, vector := range golden.Vectors {
		if _, duplicate := seen[vector.Name]; duplicate {
			t.Fatalf("duplicate golden vector %q", vector.Name)
		}
		seen[vector.Name] = struct{}{}
		input := evidenceSetGoldenInputForName(t, vector.Name)
		canonical, err := json.Marshal(evidenceSetCanonical{
			ProtocolVersion: EvidenceSetReceiptProtocolVersion, ExitEvidence: input.Exit,
			RuntimeEvidence: input.Runtime, ResourceLimitEvidence: input.Limits,
			TerminationCauseEvidence: input.Cause, LifecycleTimeline: input.Timeline,
			DeadlineBudgetEvidence: input.Budget,
		})
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(canonical)
		actualDigest := hex.EncodeToString(digest[:])
		receipt, err := buildEvidenceSetReceipt(input.Exit, input.Runtime, input.Limits,
			input.Cause, input.Timeline, input.Budget)
		if err != nil {
			t.Fatal(err)
		}
		if vector.CanonicalBytes != len(canonical) || vector.CanonicalSHA256 != actualDigest ||
			receipt.CanonicalBytes != vector.CanonicalBytes ||
			receipt.CanonicalSHA256 != vector.CanonicalSHA256 ||
			receipt.RecordProtocols != [6]string{ExitEvidenceProtocolVersion,
				RuntimeEvidenceProtocolVersion, ResourceLimitEvidenceProtocolVersion,
				TerminationCauseEvidenceProtocolVersion, LifecycleTimelineEvidenceProtocolVersion,
				DeadlineBudgetEvidenceProtocolVersion} ||
			receipt.CrossRecordWallClockOrderClaimed || receipt.RawOutputIncluded ||
			receipt.ProcessIdentityIncluded || receipt.OSResourceLimitsVerified ||
			receipt.ProductExecutionEnabled {
			t.Fatalf("golden vector %q drifted: bytes=%d sha256=%s receipt=%#v",
				vector.Name, len(canonical), actualDigest, receipt)
		}
	}
}

func evidenceSetGoldenInputForName(t *testing.T, name string) evidenceSetGoldenInput {
	t.Helper()
	switch name {
	case "normal_empty_exit":
		request, err := (Request{ID: "golden-normal"}).normalize()
		if err != nil {
			t.Fatal(err)
		}
		result := Result{Started: true, ExitCode: 0, StopReason: StopExited, TreeReaped: true}
		return buildGoldenInput(t, request, result,
			goldenExitEvidence(0, goldenOutputEvidence(0, EmptyOutputSHA256),
				goldenOutputEvidence(0, EmptyOutputSHA256)),
			goldenRuntimeEvidence(0, EmptyOutputSHA256, ResourceEvidence{
				WallTimeMilliseconds: 1,
			}))
	case "forced_timeout_bounded_metadata":
		request, err := (Request{ID: "golden-forced", Timeout: 1250 * time.Millisecond,
			TerminationGrace: 250 * time.Millisecond, KillGrace: 500 * time.Millisecond}).normalize()
		if err != nil {
			t.Fatal(err)
		}
		result := Result{Started: true, ExitCode: 137, StopReason: StopTimedOut,
			TimedOut: true, TerminateRequested: true, KillRequested: true, TreeReaped: true}
		return buildGoldenInput(t, request, result,
			goldenExitEvidence(137,
				OutputEvidence{ObservedBytes: 70_000, CapturedBytes: MaxOutputCaptureBytes,
					CapturedPrefixSHA256: strings.Repeat("a", 64), Truncated: true},
				goldenOutputEvidence(7, strings.Repeat("b", 64))),
			goldenRuntimeEvidence(12, strings.Repeat("c", 64), ResourceEvidence{
				WallTimeMilliseconds: 1250, ParentUserCPUTimeMilliseconds: 25,
				ParentSystemCPUTimeMilliseconds: 5, PeakResidentBytes: 4096,
				PeakResidentMeasured: true,
			}))
	default:
		t.Fatalf("unknown evidence-set golden vector %q", name)
		return evidenceSetGoldenInput{}
	}
}

func buildGoldenInput(t *testing.T, request Request, result Result, exit ExitEvidence,
	runtime RuntimeEvidence,
) evidenceSetGoldenInput {
	t.Helper()
	status := ExitStatus{Exited: true, ExitCode: result.ExitCode, Reaped: true}
	limits := ResourceLimitEvidence{ProtocolVersion: ResourceLimitEvidenceProtocolVersion,
		RunTimeoutMilliseconds:       request.Timeout.Milliseconds(),
		TerminationGraceMilliseconds: request.TerminationGrace.Milliseconds(),
		KillGraceMilliseconds:        request.KillGrace.Milliseconds(), WallDeadlineConfigured: true,
		MetadataOnly: true}
	trigger, ok := terminationTrigger(result)
	if !ok {
		t.Fatal("golden result has no termination trigger")
	}
	cause := TerminationCauseEvidence{ProtocolVersion: TerminationCauseEvidenceProtocolVersion,
		ControlTrigger: trigger, FinalMechanism: terminationMechanism(result), Exited: true,
		ExitCode: result.ExitCode, TreeReaped: true, TimedOut: result.TimedOut,
		Cancelled: result.Cancelled, OrphanDetected: result.OrphanDetected,
		TerminateRequested: result.TerminateRequested, TerminateFailed: result.TerminateFailed,
		KillRequested: result.KillRequested, KillFailed: result.KillFailed, MetadataOnly: true}
	timeline, ok := buildLifecycleTimelineEvidence(result)
	if !ok {
		t.Fatal("golden result has no lifecycle timeline")
	}
	budget := buildDeadlineBudgetEvidence(request, result)
	if exit.Stdout.validate() != nil || exit.Stderr.validate() != nil ||
		runtime.Stdin.validate() != nil || runtime.Descriptors.validate() != nil ||
		runtime.Resources.validate() != nil || limits.validate(request) != nil ||
		cause.validate(status, result) != nil || timeline.validate(result) != nil ||
		budget.validate(request, result) != nil {
		t.Fatal("golden evidence fixture violates the Runner contract")
	}
	return evidenceSetGoldenInput{Exit: exit, Runtime: runtime, Limits: limits,
		Cause: cause, Timeline: timeline, Budget: budget}
}

func goldenExitEvidence(exitCode int, stdout OutputEvidence, stderr OutputEvidence) ExitEvidence {
	return ExitEvidence{ProtocolVersion: ExitEvidenceProtocolVersion, Exited: true,
		ExitCode: exitCode, Reaped: true, Stdout: stdout, Stderr: stderr,
		MetadataOnly: true}
}

func goldenOutputEvidence(observed int64, digest string) OutputEvidence {
	return OutputEvidence{ObservedBytes: observed, CapturedBytes: int(observed),
		CapturedPrefixSHA256: digest}
}

func goldenRuntimeEvidence(stdinBytes int64, stdinDigest string,
	resources ResourceEvidence,
) RuntimeEvidence {
	return RuntimeEvidence{ProtocolVersion: RuntimeEvidenceProtocolVersion, TreeReaped: true,
		Stdin: StdinEvidence{BytesProvided: stdinBytes, ContentSHA256: stdinDigest, Closed: true},
		Descriptors: DescriptorEvidence{StandardInputClosed: true,
			StandardOutputCaptured: true, StandardErrorCaptured: true},
		Resources: resources, MetadataOnly: true}
}
