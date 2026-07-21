package analyzer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	InvocationOutcomeProtocolVersion  = "analyzer_invocation_outcome.v1"
	MaxInvocationOutcomeEnvelopeBytes = 4 * 1024
	MaxFakeTransportStdoutBytes       = MaxResultEnvelopeBytes + 1

	DisabledTransportName = "disabled"
	FakeTransportName     = "fake"
)

type InvocationStatus string

const (
	InvocationSucceeded InvocationStatus = "succeeded"
	InvocationRejected  InvocationStatus = "rejected"
	InvocationFailed    InvocationStatus = "failed"
	InvocationTimedOut  InvocationStatus = "timed_out"
	InvocationCancelled InvocationStatus = "cancelled"
	InvocationDisabled  InvocationStatus = "disabled"
)

type InvocationFailureCode string

const (
	InvocationFailureNone          InvocationFailureCode = ""
	InvocationFailureDisabled      InvocationFailureCode = "transport_disabled"
	InvocationFailureDeadline      InvocationFailureCode = "deadline_exceeded"
	InvocationFailureCancelled     InvocationFailureCode = "cancelled"
	InvocationFailureProcess       InvocationFailureCode = "process_failed"
	InvocationFailureOutputLimit   InvocationFailureCode = "output_limit_exceeded"
	InvocationFailureInvalidResult InvocationFailureCode = "invalid_result"
	InvocationFailureInternal      InvocationFailureCode = "internal_error"
)

// InvocationOutcome contains bounded metadata only. Raw stdout and process
// identity are never retained by this pre-product bridge.
type InvocationOutcome struct {
	ProtocolVersion          string                `json:"protocol_version"`
	CandidateSHA256          string                `json:"candidate_sha256"`
	RequestID                string                `json:"request_id"`
	Analyzer                 string                `json:"analyzer"`
	Transport                string                `json:"transport"`
	Status                   InvocationStatus      `json:"status"`
	FailureCode              InvocationFailureCode `json:"failure_code"`
	AnalyzerErrorCode        ErrorCode             `json:"analyzer_error_code"`
	ExitCode                 int                   `json:"exit_code"`
	StdoutBytes              int                   `json:"stdout_bytes"`
	StdoutSHA256             string                `json:"stdout_sha256"`
	ResultProtocol           string                `json:"result_protocol"`
	DeadlineMilliseconds     int                   `json:"deadline_ms"`
	Completed                bool                  `json:"completed"`
	DeadlineEnforced         bool                  `json:"deadline_enforced"`
	StdoutWithinLimit        bool                  `json:"stdout_within_limit"`
	ResultValidated          bool                  `json:"result_validated"`
	MetadataOnly             bool                  `json:"metadata_only"`
	RawOutputIncluded        bool                  `json:"raw_output_included"`
	ProductInvocationEnabled bool                  `json:"product_invocation_enabled"`
}

// Transport is sealed to this package. Only the inert DisabledTransport and
// deterministic FakeTransport exist in non-test builds in this release.
type Transport interface {
	analyzerTransport()
	name() string
	exchange(context.Context, InvocationCandidate, []byte) (transportExchange, error)
}

type DisabledTransport struct{}

func (DisabledTransport) analyzerTransport() {}
func (DisabledTransport) name() string       { return DisabledTransportName }
func (DisabledTransport) exchange(context.Context, InvocationCandidate, []byte) (transportExchange, error) {
	return transportExchange{}, errTransportDisabled
}

type FakeTransportPlan struct {
	Stdout   []byte
	ExitCode int
	Delay    time.Duration
	Crash    bool
}

type FakeTransport struct {
	stdout   []byte
	exitCode int
	delay    time.Duration
	crash    bool
}

func NewFakeTransport(plan FakeTransportPlan) (*FakeTransport, error) {
	if len(plan.Stdout) > MaxFakeTransportStdoutBytes {
		return nil, fmt.Errorf("fake analyzer stdout exceeds %d bytes", MaxFakeTransportStdoutBytes)
	}
	if plan.ExitCode < 0 || plan.ExitCode > 255 {
		return nil, errors.New("fake analyzer exit code must be between 0 and 255")
	}
	if plan.Delay < 0 || plan.Delay > time.Duration(MaxTimeoutMilliseconds+1000)*time.Millisecond {
		return nil, errors.New("fake analyzer delay is outside the test-only bound")
	}
	if plan.Crash && (len(plan.Stdout) != 0 || plan.ExitCode != 0) {
		return nil, errors.New("fake analyzer crash cannot include stdout or an exit code")
	}
	return &FakeTransport{
		stdout: append([]byte(nil), plan.Stdout...), exitCode: plan.ExitCode,
		delay: plan.Delay, crash: plan.Crash,
	}, nil
}

func (*FakeTransport) analyzerTransport() {}
func (*FakeTransport) name() string       { return FakeTransportName }
func (transport *FakeTransport) exchange(ctx context.Context, _ InvocationCandidate,
	_ []byte,
) (transportExchange, error) {
	if transport.delay > 0 {
		timer := time.NewTimer(transport.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return transportExchange{}, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return transportExchange{}, err
	}
	if transport.crash {
		return transportExchange{}, errFakeTransportCrash
	}
	return transportExchange{
		stdout: append([]byte(nil), transport.stdout...), exitCode: transport.exitCode,
	}, nil
}

type Bridge struct {
	transport Transport
}

func NewBridge(transport Transport) (*Bridge, error) {
	if transport == nil {
		return nil, errors.New("analyzer transport is required")
	}
	switch value := transport.(type) {
	case DisabledTransport:
	case *FakeTransport:
		if value == nil {
			return nil, errors.New("analyzer transport is required")
		}
	default:
		return nil, errors.New("analyzer transport is not admitted by the pre-product bridge")
	}
	return &Bridge{transport: transport}, nil
}

// Invoke revalidates the complete candidate, canonicalizes stdin, applies the
// declared deadline, and returns metadata only. Neither admitted transport can
// start a real process.
func (bridge *Bridge) Invoke(ctx context.Context, candidate InvocationCandidate,
	rawRequest []byte,
) (InvocationOutcome, ErrorCode) {
	return bridge.invoke(ctx, candidate, rawRequest, ValidateInvocationOutcome)
}

type invocationOutcomeValidator func(InvocationCandidate, InvocationOutcome) bool

func (bridge *Bridge) invoke(ctx context.Context, candidate InvocationCandidate,
	rawRequest []byte, validate invocationOutcomeValidator,
) (InvocationOutcome, ErrorCode) {
	if bridge == nil || bridge.transport == nil {
		return InvocationOutcome{}, CodeInternal
	}
	if validate == nil {
		return InvocationOutcome{}, CodeInternal
	}
	if code := ValidateInvocationCandidate(candidate, rawRequest); code != "" {
		return InvocationOutcome{}, CodeInvalidResult
	}
	request, code := DecodeRequest(rawRequest)
	if code != "" {
		return InvocationOutcome{}, CodeInvalidResult
	}
	canonicalRequest, err := json.Marshal(request)
	if err != nil {
		return InvocationOutcome{}, CodeInternal
	}
	base, ok := newInvocationOutcome(candidate, bridge.transport.name())
	if !ok {
		return InvocationOutcome{}, CodeInternal
	}
	if bridge.transport.name() == DisabledTransportName {
		base.Status = InvocationDisabled
		base.FailureCode = InvocationFailureDisabled
		return checkedInvocationOutcome(candidate, base, validate)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		outcome := contextFailureOutcome(base, err)
		return checkedInvocationOutcome(candidate, outcome, validate)
	}
	deadlineCtx, cancel := context.WithTimeout(ctx,
		time.Duration(candidate.Limits.TimeoutMilliseconds)*time.Millisecond)
	defer cancel()
	exchange, transportErr := bridge.transport.exchange(deadlineCtx, candidate, canonicalRequest)
	if err := ctx.Err(); err != nil {
		outcome := contextFailureOutcome(base, err)
		return checkedInvocationOutcome(candidate, outcome, validate)
	}
	if err := deadlineCtx.Err(); err != nil {
		outcome := contextFailureOutcome(base, err)
		return checkedInvocationOutcome(candidate, outcome, validate)
	}
	if errors.Is(transportErr, errTransportDisabled) {
		base.Status = InvocationDisabled
		base.FailureCode = InvocationFailureDisabled
		return checkedInvocationOutcome(candidate, base, validate)
	}
	if transportErr != nil {
		base.Status = InvocationFailed
		base.FailureCode = InvocationFailureProcess
		return checkedInvocationOutcome(candidate, base, validate)
	}
	outcome := classifyInvocationExchange(candidate, base, canonicalRequest, exchange)
	return checkedInvocationOutcome(candidate, outcome, validate)
}

func EncodeInvocationOutcome(candidate InvocationCandidate,
	outcome InvocationOutcome,
) ([]byte, ErrorCode) {
	if !ValidateInvocationOutcome(candidate, outcome) {
		return nil, CodeInvalidResult
	}
	encoded, err := json.Marshal(outcome)
	if err != nil || len(encoded) == 0 || len(encoded) > MaxInvocationOutcomeEnvelopeBytes {
		return nil, CodeInternal
	}
	return encoded, ""
}

func DecodeInvocationOutcome(raw []byte,
	candidate InvocationCandidate,
) (InvocationOutcome, ErrorCode) {
	var wire invocationOutcomeWire
	if !strictDecode(raw, MaxInvocationOutcomeEnvelopeBytes, &wire) || !wire.complete() {
		return InvocationOutcome{}, CodeInvalidResult
	}
	outcome := wire.value()
	if !ValidateInvocationOutcome(candidate, outcome) {
		return InvocationOutcome{}, CodeInvalidResult
	}
	return outcome, ""
}

func ValidateInvocationOutcome(candidate InvocationCandidate, outcome InvocationOutcome) bool {
	return validateInvocationOutcome(candidate, outcome, isProductInvocationExecutionTransport)
}

func validateInvocationOutcome(candidate InvocationCandidate, outcome InvocationOutcome,
	isExecutionTransport func(string) bool,
) bool {
	candidateDigest, ok := invocationCandidateSHA256(candidate)
	if !ok || isExecutionTransport == nil ||
		outcome.ProtocolVersion != InvocationOutcomeProtocolVersion ||
		outcome.CandidateSHA256 != candidateDigest || outcome.RequestID != candidate.RequestID ||
		outcome.Analyzer != candidate.Analyzer ||
		(outcome.Transport != DisabledTransportName &&
			!isExecutionTransport(outcome.Transport)) ||
		outcome.DeadlineMilliseconds != candidate.Limits.TimeoutMilliseconds ||
		!outcome.Completed || !outcome.DeadlineEnforced || !outcome.MetadataOnly ||
		outcome.RawOutputIncluded || outcome.ProductInvocationEnabled ||
		outcome.StdoutBytes < 0 || outcome.StdoutBytes > MaxFakeTransportStdoutBytes ||
		(outcome.StdoutBytes == 0 && outcome.StdoutSHA256 != "") ||
		(outcome.StdoutBytes > 0 && !validDigest(outcome.StdoutSHA256)) {
		return false
	}
	expectedWithinLimit := outcome.StdoutBytes <= candidate.Limits.MaxOutputBytes &&
		outcome.StdoutBytes <= candidate.Descriptor.Limits.MaxOutputBytes &&
		outcome.StdoutBytes <= MaxResultEnvelopeBytes
	if outcome.StdoutWithinLimit != expectedWithinLimit {
		return false
	}
	noOutput := outcome.ExitCode == -1 && outcome.StdoutBytes == 0 &&
		outcome.StdoutSHA256 == "" && outcome.ResultProtocol == "" &&
		outcome.AnalyzerErrorCode == "" && !outcome.ResultValidated
	switch outcome.Status {
	case InvocationSucceeded:
		return isExecutionTransport(outcome.Transport) && outcome.FailureCode == "" &&
			outcome.AnalyzerErrorCode == "" && outcome.ExitCode == ExitSuccess &&
			outcome.StdoutBytes > 0 && outcome.StdoutWithinLimit && outcome.ResultValidated &&
			outcome.ResultProtocol == candidate.Descriptor.ResultProtocol
	case InvocationRejected:
		return isExecutionTransport(outcome.Transport) && outcome.FailureCode == "" &&
			validAnalyzerRejectionCode(outcome.AnalyzerErrorCode) &&
			outcome.ExitCode == ExitRejected && outcome.StdoutBytes > 0 &&
			outcome.StdoutWithinLimit && outcome.ResultValidated &&
			outcome.ResultProtocol == ErrorProtocolVersion
	case InvocationTimedOut:
		return isExecutionTransport(outcome.Transport) &&
			outcome.FailureCode == InvocationFailureDeadline && outcome.StdoutWithinLimit && noOutput
	case InvocationCancelled:
		return isExecutionTransport(outcome.Transport) &&
			outcome.FailureCode == InvocationFailureCancelled && outcome.StdoutWithinLimit && noOutput
	case InvocationDisabled:
		return outcome.Transport == DisabledTransportName &&
			outcome.FailureCode == InvocationFailureDisabled && outcome.StdoutWithinLimit && noOutput
	case InvocationFailed:
		return validateFailedInvocationOutcome(candidate, outcome, noOutput,
			isExecutionTransport)
	default:
		return false
	}
}

func validateFailedInvocationOutcome(candidate InvocationCandidate, outcome InvocationOutcome,
	noOutput bool, isExecutionTransport func(string) bool,
) bool {
	if isExecutionTransport == nil || !isExecutionTransport(outcome.Transport) {
		return false
	}
	switch outcome.FailureCode {
	case InvocationFailureProcess:
		if noOutput {
			return outcome.StdoutWithinLimit
		}
		return outcome.ExitCode >= 0 && outcome.ExitCode != ExitSuccess &&
			outcome.ExitCode != ExitRejected && outcome.ExitCode != ExitInternal &&
			outcome.AnalyzerErrorCode == "" && outcome.ResultProtocol == "" &&
			outcome.StdoutWithinLimit && !outcome.ResultValidated
	case InvocationFailureOutputLimit:
		return outcome.ExitCode >= 0 && outcome.StdoutBytes > candidate.Limits.MaxOutputBytes &&
			!outcome.StdoutWithinLimit && !outcome.ResultValidated &&
			outcome.AnalyzerErrorCode == "" && outcome.ResultProtocol == ""
	case InvocationFailureInvalidResult:
		return (outcome.ExitCode == ExitSuccess || outcome.ExitCode == ExitRejected ||
			outcome.ExitCode == ExitInternal) && outcome.StdoutWithinLimit &&
			!outcome.ResultValidated && outcome.AnalyzerErrorCode == "" &&
			outcome.ResultProtocol == ""
	case InvocationFailureInternal:
		return outcome.ExitCode == ExitInternal && outcome.StdoutBytes > 0 &&
			outcome.StdoutWithinLimit && outcome.ResultValidated &&
			outcome.AnalyzerErrorCode == CodeInternal &&
			outcome.ResultProtocol == ErrorProtocolVersion
	default:
		return false
	}
}

func isProductInvocationExecutionTransport(name string) bool {
	return name == FakeTransportName
}

func classifyInvocationExchange(candidate InvocationCandidate, base InvocationOutcome,
	rawRequest []byte, exchange transportExchange,
) InvocationOutcome {
	base.ExitCode = exchange.exitCode
	base.StdoutBytes = len(exchange.stdout)
	if len(exchange.stdout) > 0 {
		digest := sha256.Sum256(exchange.stdout)
		base.StdoutSHA256 = hex.EncodeToString(digest[:])
	}
	if len(exchange.stdout) > candidate.Limits.MaxOutputBytes ||
		len(exchange.stdout) > candidate.Descriptor.Limits.MaxOutputBytes ||
		len(exchange.stdout) > MaxResultEnvelopeBytes {
		base.Status = InvocationFailed
		base.FailureCode = InvocationFailureOutputLimit
		base.StdoutWithinLimit = false
		return base
	}
	base.StdoutWithinLimit = true
	switch exchange.exitCode {
	case ExitSuccess:
		if !validSuccessfulAnalyzerResult(candidate, rawRequest, exchange.stdout) {
			base.Status = InvocationFailed
			base.FailureCode = InvocationFailureInvalidResult
			return base
		}
		base.Status = InvocationSucceeded
		base.ResultProtocol = candidate.Descriptor.ResultProtocol
		base.ResultValidated = true
	case ExitRejected:
		rejection, code := DecodeError(exchange.stdout)
		if code != "" || rejection.RequestID != candidate.RequestID ||
			!validAnalyzerRejectionCode(rejection.Code) ||
			!matchesDeterministicEvaluation(rawRequest, exchange.stdout, ExitRejected) {
			base.Status = InvocationFailed
			base.FailureCode = InvocationFailureInvalidResult
			return base
		}
		base.Status = InvocationRejected
		base.AnalyzerErrorCode = rejection.Code
		base.ResultProtocol = ErrorProtocolVersion
		base.ResultValidated = true
	case ExitInternal:
		rejection, code := DecodeError(exchange.stdout)
		if code != "" || rejection.RequestID != candidate.RequestID || rejection.Code != CodeInternal {
			base.Status = InvocationFailed
			base.FailureCode = InvocationFailureInvalidResult
			return base
		}
		base.Status = InvocationFailed
		base.FailureCode = InvocationFailureInternal
		base.AnalyzerErrorCode = rejection.Code
		base.ResultProtocol = ErrorProtocolVersion
		base.ResultValidated = true
	default:
		base.Status = InvocationFailed
		base.FailureCode = InvocationFailureProcess
	}
	return base
}

func validSuccessfulAnalyzerResult(candidate InvocationCandidate, rawRequest, rawResult []byte) bool {
	validProtocol := false
	switch candidate.Descriptor.ResultProtocol {
	case ResultProtocolVersion:
		result, code := DecodeResult(rawResult)
		validProtocol = code == "" && result.RequestID == candidate.RequestID &&
			result.Analyzer == candidate.Analyzer && result.Summary.MediaType == candidate.MediaType &&
			result.Summary.InputBytes == candidate.InputBytes &&
			result.Summary.SHA256 == candidate.InputSHA256
	case ArchiveInventoryProtocolVersion:
		result, code := DecodeArchiveInventory(rawResult)
		validProtocol = code == "" && result.RequestID == candidate.RequestID &&
			result.Analyzer == candidate.Analyzer
	default:
		return false
	}
	if !validProtocol {
		return false
	}
	return matchesDeterministicEvaluation(rawRequest, rawResult, ExitSuccess)
}

func matchesDeterministicEvaluation(rawRequest, rawResult []byte, expectedExit int) bool {
	expected, exitCode := Evaluate(rawRequest)
	return exitCode == expectedExit && bytes.Equal(rawResult, expected)
}

func validAnalyzerRejectionCode(code ErrorCode) bool {
	return code == CodeInputLimitExceeded || code == CodeInvalidContent ||
		code == CodeOutputLimitExceeded
}

func newInvocationOutcome(candidate InvocationCandidate, transport string) (InvocationOutcome, bool) {
	digest, ok := invocationCandidateSHA256(candidate)
	if !ok {
		return InvocationOutcome{}, false
	}
	return InvocationOutcome{
		ProtocolVersion: InvocationOutcomeProtocolVersion, CandidateSHA256: digest,
		RequestID: candidate.RequestID, Analyzer: candidate.Analyzer, Transport: transport,
		ExitCode: -1, DeadlineMilliseconds: candidate.Limits.TimeoutMilliseconds,
		Completed: true, DeadlineEnforced: true, StdoutWithinLimit: true, MetadataOnly: true,
	}, true
}

func contextFailureOutcome(base InvocationOutcome, err error) InvocationOutcome {
	if errors.Is(err, context.DeadlineExceeded) {
		base.Status = InvocationTimedOut
		base.FailureCode = InvocationFailureDeadline
		return base
	}
	base.Status = InvocationCancelled
	base.FailureCode = InvocationFailureCancelled
	return base
}

func checkedInvocationOutcome(candidate InvocationCandidate,
	outcome InvocationOutcome, validate invocationOutcomeValidator,
) (InvocationOutcome, ErrorCode) {
	if validate == nil || !validate(candidate, outcome) {
		return InvocationOutcome{}, CodeInternal
	}
	return outcome, ""
}

type transportExchange struct {
	stdout   []byte
	exitCode int
}

var (
	errTransportDisabled  = errors.New("analyzer transport is disabled")
	errFakeTransportCrash = errors.New("fake analyzer transport crashed")
)

type invocationOutcomeWire struct {
	ProtocolVersion          *string                `json:"protocol_version"`
	CandidateSHA256          *string                `json:"candidate_sha256"`
	RequestID                *string                `json:"request_id"`
	Analyzer                 *string                `json:"analyzer"`
	Transport                *string                `json:"transport"`
	Status                   *InvocationStatus      `json:"status"`
	FailureCode              *InvocationFailureCode `json:"failure_code"`
	AnalyzerErrorCode        *ErrorCode             `json:"analyzer_error_code"`
	ExitCode                 *int                   `json:"exit_code"`
	StdoutBytes              *int                   `json:"stdout_bytes"`
	StdoutSHA256             *string                `json:"stdout_sha256"`
	ResultProtocol           *string                `json:"result_protocol"`
	DeadlineMilliseconds     *int                   `json:"deadline_ms"`
	Completed                *bool                  `json:"completed"`
	DeadlineEnforced         *bool                  `json:"deadline_enforced"`
	StdoutWithinLimit        *bool                  `json:"stdout_within_limit"`
	ResultValidated          *bool                  `json:"result_validated"`
	MetadataOnly             *bool                  `json:"metadata_only"`
	RawOutputIncluded        *bool                  `json:"raw_output_included"`
	ProductInvocationEnabled *bool                  `json:"product_invocation_enabled"`
}

func (wire invocationOutcomeWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.CandidateSHA256 != nil &&
		wire.RequestID != nil && wire.Analyzer != nil && wire.Transport != nil &&
		wire.Status != nil && wire.FailureCode != nil && wire.AnalyzerErrorCode != nil &&
		wire.ExitCode != nil && wire.StdoutBytes != nil && wire.StdoutSHA256 != nil &&
		wire.ResultProtocol != nil && wire.DeadlineMilliseconds != nil &&
		wire.Completed != nil && wire.DeadlineEnforced != nil &&
		wire.StdoutWithinLimit != nil && wire.ResultValidated != nil &&
		wire.MetadataOnly != nil && wire.RawOutputIncluded != nil &&
		wire.ProductInvocationEnabled != nil
}

func (wire invocationOutcomeWire) value() InvocationOutcome {
	return InvocationOutcome{
		ProtocolVersion: *wire.ProtocolVersion, CandidateSHA256: *wire.CandidateSHA256,
		RequestID: *wire.RequestID, Analyzer: *wire.Analyzer, Transport: *wire.Transport,
		Status: *wire.Status, FailureCode: *wire.FailureCode,
		AnalyzerErrorCode: *wire.AnalyzerErrorCode, ExitCode: *wire.ExitCode,
		StdoutBytes: *wire.StdoutBytes, StdoutSHA256: *wire.StdoutSHA256,
		ResultProtocol: *wire.ResultProtocol, DeadlineMilliseconds: *wire.DeadlineMilliseconds,
		Completed: *wire.Completed, DeadlineEnforced: *wire.DeadlineEnforced,
		StdoutWithinLimit: *wire.StdoutWithinLimit, ResultValidated: *wire.ResultValidated,
		MetadataOnly: *wire.MetadataOnly, RawOutputIncluded: *wire.RawOutputIncluded,
		ProductInvocationEnabled: *wire.ProductInvocationEnabled,
	}
}
