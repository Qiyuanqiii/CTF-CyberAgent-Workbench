package browserruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	CDPRequestProtocolVersion = "browser_cdp_request.v1"
	CDPOutcomeProtocolVersion = "browser_cdp_outcome.v1"

	DefaultCDPDeadlineMS       = 5_000
	MaxCDPDeadlineMS           = 30_000
	MaxDOMSnapshotBytes        = 1 * 1024 * 1024
	MaxScreenshotBytes         = 8 * 1024 * 1024
	MaxRequestCaptureBytes     = 2 * 1024 * 1024
	MaxCDPCaptureEntries       = 500
	MaxFakeCDPPayloadBytes     = MaxScreenshotBytes + 1
	MaxCDPOutcomeEnvelopeBytes = 8 * 1024

	DisabledCDPTransportName = "disabled"
	FakeCDPTransportName     = "fake"
)

type CDPAction string

const (
	CDPActionNavigate       CDPAction = "navigate"
	CDPActionDOMSnapshot    CDPAction = "dom_snapshot"
	CDPActionScreenshot     CDPAction = "screenshot"
	CDPActionRequestCapture CDPAction = "request_capture"
)

type CDPRequest struct {
	ProtocolVersion                     string           `json:"protocol_version"`
	RequestID                           string           `json:"request_id"`
	SessionPlanFingerprint              string           `json:"session_plan_fingerprint"`
	ExecutableIdentityFingerprint       string           `json:"executable_identity_fingerprint"`
	ProfileOwnershipFingerprint         string           `json:"profile_ownership_fingerprint"`
	Action                              CDPAction        `json:"action"`
	CanonicalURL                        string           `json:"canonical_url"`
	MaxPayloadBytes                     int              `json:"max_payload_bytes"`
	MaxCaptureEntries                   int              `json:"max_capture_entries"`
	DeadlineMS                          int              `json:"deadline_ms"`
	RedirectRevalidationRequired        bool             `json:"redirect_revalidation_required"`
	ResolvedAddressRevalidationRequired bool             `json:"resolved_address_revalidation_required"`
	ProductExecutionBlocked             bool             `json:"product_execution_blocked"`
	Authority                           RuntimeAuthority `json:"authority"`
	Fingerprint                         string           `json:"fingerprint"`
}

type CDPOutcomeStatus string

const (
	CDPOutcomeSucceeded CDPOutcomeStatus = "succeeded"
	CDPOutcomeFailed    CDPOutcomeStatus = "failed"
	CDPOutcomeTimedOut  CDPOutcomeStatus = "timed_out"
	CDPOutcomeCancelled CDPOutcomeStatus = "cancelled"
	CDPOutcomeDisabled  CDPOutcomeStatus = "disabled"
)

type CDPFailureCode string

const (
	CDPFailureNone          CDPFailureCode = ""
	CDPFailureDisabled      CDPFailureCode = "transport_disabled"
	CDPFailureDeadline      CDPFailureCode = "deadline_exceeded"
	CDPFailureCancelled     CDPFailureCode = "cancelled"
	CDPFailureTransport     CDPFailureCode = "transport_failed"
	CDPFailurePayloadLimit  CDPFailureCode = "payload_limit_exceeded"
	CDPFailureInvalidResult CDPFailureCode = "invalid_result"
)

// CDPOutcome contains bounded metadata only. DOM bytes, screenshot pixels,
// request records, headers, cookies, and response bodies are never retained by
// this pre-product bridge.
type CDPOutcome struct {
	ProtocolVersion         string           `json:"protocol_version"`
	RequestFingerprint      string           `json:"request_fingerprint"`
	RequestID               string           `json:"request_id"`
	Action                  CDPAction        `json:"action"`
	CanonicalURL            string           `json:"canonical_url"`
	Transport               string           `json:"transport"`
	Status                  CDPOutcomeStatus `json:"status"`
	FailureCode             CDPFailureCode   `json:"failure_code"`
	PayloadBytes            int              `json:"payload_bytes"`
	PayloadSHA256           string           `json:"payload_sha256"`
	MediaType               string           `json:"media_type"`
	CapturedRequestCount    int              `json:"captured_request_count"`
	DeadlineMS              int              `json:"deadline_ms"`
	Completed               bool             `json:"completed"`
	DeadlineEnforced        bool             `json:"deadline_enforced"`
	ScopeValidated          bool             `json:"scope_validated"`
	PayloadWithinLimit      bool             `json:"payload_within_limit"`
	MetadataOnly            bool             `json:"metadata_only"`
	Synthetic               bool             `json:"synthetic"`
	RawPayloadIncluded      bool             `json:"raw_payload_included"`
	ProcessStarted          bool             `json:"process_started"`
	NetworkUsed             bool             `json:"network_used"`
	ProfileWritten          bool             `json:"profile_written"`
	RequestMutationUsed     bool             `json:"request_mutation_used"`
	RequestReplayUsed       bool             `json:"request_replay_used"`
	ArtifactCommitted       bool             `json:"artifact_committed"`
	ProductExecutionEnabled bool             `json:"product_execution_enabled"`
	Authority               RuntimeAuthority `json:"authority"`
	Fingerprint             string           `json:"fingerprint"`
}

func BuildCDPRequest(session SessionPlan, executable BrowserExecutableIdentity,
	ownership ProfileOwnershipPlan, requestID string, action CDPAction, rawURL string,
) (CDPRequest, error) {
	if err := ValidateProfileOwnershipPlan(ownership, session, executable); err != nil {
		return CDPRequest{}, err
	}
	if !validPlanIdentity(requestID) {
		return CDPRequest{}, errors.New("browser CDP request id is invalid")
	}
	decision := session.Scope.AuthorizeNavigation(rawURL)
	if !decision.Allowed {
		return CDPRequest{}, fmt.Errorf("browser CDP URL is outside the exact target scope: %s", decision.Code)
	}
	maxPayload, maxEntries, err := cdpActionLimits(session, action)
	if err != nil {
		return CDPRequest{}, err
	}
	request := CDPRequest{
		ProtocolVersion: CDPRequestProtocolVersion, RequestID: requestID,
		SessionPlanFingerprint:        session.Fingerprint,
		ExecutableIdentityFingerprint: executable.Fingerprint,
		ProfileOwnershipFingerprint:   ownership.Fingerprint,
		Action:                        action, CanonicalURL: decision.CanonicalURL,
		MaxPayloadBytes: maxPayload, MaxCaptureEntries: maxEntries,
		DeadlineMS:                          DefaultCDPDeadlineMS,
		RedirectRevalidationRequired:        true,
		ResolvedAddressRevalidationRequired: true,
		ProductExecutionBlocked:             true,
	}
	var fingerprintErr error
	request.Fingerprint, fingerprintErr = cdpRequestFingerprint(request)
	if fingerprintErr != nil {
		return CDPRequest{}, fingerprintErr
	}
	if err := ValidateCDPRequest(request, session, executable, ownership); err != nil {
		return CDPRequest{}, err
	}
	return request, nil
}

func ValidateCDPRequest(request CDPRequest, session SessionPlan,
	executable BrowserExecutableIdentity, ownership ProfileOwnershipPlan,
) error {
	if err := ValidateProfileOwnershipPlan(ownership, session, executable); err != nil {
		return err
	}
	decision := session.Scope.AuthorizeNavigation(request.CanonicalURL)
	maxPayload, maxEntries, err := cdpActionLimits(session, request.Action)
	if request.ProtocolVersion != CDPRequestProtocolVersion || !validPlanIdentity(request.RequestID) ||
		request.SessionPlanFingerprint != session.Fingerprint ||
		request.ExecutableIdentityFingerprint != executable.Fingerprint ||
		request.ProfileOwnershipFingerprint != ownership.Fingerprint || err != nil ||
		!decision.Allowed || decision.CanonicalURL != request.CanonicalURL ||
		request.MaxPayloadBytes != maxPayload || request.MaxCaptureEntries != maxEntries ||
		request.DeadlineMS != DefaultCDPDeadlineMS || request.DeadlineMS > MaxCDPDeadlineMS ||
		!request.RedirectRevalidationRequired ||
		!request.ResolvedAddressRevalidationRequired || !request.ProductExecutionBlocked ||
		request.Authority != (RuntimeAuthority{}) {
		return errors.New("browser CDP request lost a fixed scope, limit, or authority boundary")
	}
	expected, err := cdpRequestFingerprint(request)
	if err != nil || request.Fingerprint != expected {
		return errors.New("browser CDP request fingerprint mismatch")
	}
	return nil
}

func cdpActionLimits(session SessionPlan, action CDPAction) (int, int, error) {
	switch action {
	case CDPActionNavigate:
		return 0, 0, nil
	case CDPActionDOMSnapshot:
		if !session.Features.DOMInspection {
			return 0, 0, errors.New("browser session does not allow DOM inspection")
		}
		return minInt(MaxDOMSnapshotBytes, session.Limits.MaxResponseBytes), 0, nil
	case CDPActionScreenshot:
		if !session.Features.Screenshots {
			return 0, 0, errors.New("browser session does not allow screenshots")
		}
		return minInt(MaxScreenshotBytes, session.Limits.MaxResponseBytes), 0, nil
	case CDPActionRequestCapture:
		if !session.Features.RequestCapture {
			return 0, 0, errors.New("browser session does not allow request capture")
		}
		return minInt(MaxRequestCaptureBytes, session.Limits.MaxResponseBytes),
			minInt(MaxCDPCaptureEntries, session.Limits.MaxRequests), nil
	default:
		return 0, 0, fmt.Errorf("unsupported browser CDP action %q", action)
	}
}

// CDPTransport is package-sealed. Only the inert disabled transport and the
// deterministic in-memory fake transport are admitted in this release.
type CDPTransport interface {
	browserCDPTransport()
	name() string
	exchange(context.Context, CDPRequest) (cdpExchange, error)
}

type DisabledCDPTransport struct{}

func (DisabledCDPTransport) browserCDPTransport() {}
func (DisabledCDPTransport) name() string         { return DisabledCDPTransportName }
func (DisabledCDPTransport) exchange(context.Context, CDPRequest) (cdpExchange, error) {
	return cdpExchange{}, errCDPTransportDisabled
}

type FakeCDPTransportPlan struct {
	Payload              []byte
	MediaType            string
	CapturedRequestCount int
	Delay                time.Duration
	Crash                bool
}

type FakeCDPTransport struct {
	requestFingerprint   string
	payload              []byte
	mediaType            string
	capturedRequestCount int
	delay                time.Duration
	crash                bool
}

func NewFakeCDPTransport(request CDPRequest,
	plan FakeCDPTransportPlan,
) (*FakeCDPTransport, error) {
	if !validSHA256(request.Fingerprint) {
		return nil, errors.New("fake browser CDP transport requires a fingerprinted request")
	}
	if len(plan.Payload) > MaxFakeCDPPayloadBytes {
		return nil, fmt.Errorf("fake browser CDP payload exceeds %d bytes", MaxFakeCDPPayloadBytes)
	}
	if !validBoundedMediaType(plan.MediaType) {
		return nil, errors.New("fake browser CDP media type is invalid")
	}
	if plan.CapturedRequestCount < 0 || plan.CapturedRequestCount > MaxCDPCaptureEntries {
		return nil, errors.New("fake browser CDP request count is outside the fixed bound")
	}
	if plan.Delay < 0 || plan.Delay > time.Duration(MaxCDPDeadlineMS+1000)*time.Millisecond {
		return nil, errors.New("fake browser CDP delay is outside the fixed bound")
	}
	if plan.Crash && (len(plan.Payload) != 0 || plan.MediaType != "" ||
		plan.CapturedRequestCount != 0) {
		return nil, errors.New("fake browser CDP crash cannot include a result")
	}
	return &FakeCDPTransport{
		requestFingerprint: request.Fingerprint,
		payload:            append([]byte(nil), plan.Payload...), mediaType: plan.MediaType,
		capturedRequestCount: plan.CapturedRequestCount,
		delay:                plan.Delay, crash: plan.Crash,
	}, nil
}

func (*FakeCDPTransport) browserCDPTransport() {}
func (*FakeCDPTransport) name() string         { return FakeCDPTransportName }
func (transport *FakeCDPTransport) exchange(ctx context.Context,
	request CDPRequest,
) (cdpExchange, error) {
	if transport.requestFingerprint != request.Fingerprint {
		return cdpExchange{}, errCDPRequestMismatch
	}
	if transport.delay > 0 {
		timer := time.NewTimer(transport.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return cdpExchange{}, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return cdpExchange{}, err
	}
	if transport.crash {
		return cdpExchange{}, errFakeCDPTransportCrash
	}
	return cdpExchange{
		payload: append([]byte(nil), transport.payload...), mediaType: transport.mediaType,
		capturedRequestCount: transport.capturedRequestCount,
	}, nil
}

type CDPBridge struct {
	transport CDPTransport
}

func NewCDPBridge(transport CDPTransport) (*CDPBridge, error) {
	if transport == nil {
		return nil, errors.New("browser CDP transport is required")
	}
	switch value := transport.(type) {
	case DisabledCDPTransport:
	case *FakeCDPTransport:
		if value == nil {
			return nil, errors.New("browser CDP transport is required")
		}
	default:
		return nil, errors.New("browser CDP transport is not admitted by the pre-product bridge")
	}
	return &CDPBridge{transport: transport}, nil
}

// Execute validates every binding, enforces the request deadline, and returns
// metadata only. Neither admitted transport can start a process or use a
// network connection.
func (bridge *CDPBridge) Execute(ctx context.Context, session SessionPlan,
	executable BrowserExecutableIdentity, ownership ProfileOwnershipPlan,
	request CDPRequest,
) (CDPOutcome, error) {
	if bridge == nil || bridge.transport == nil {
		return CDPOutcome{}, errors.New("browser CDP bridge is not initialized")
	}
	if err := ValidateCDPRequest(request, session, executable, ownership); err != nil {
		return CDPOutcome{}, err
	}
	base, err := newCDPOutcome(request, bridge.transport.name())
	if err != nil {
		return CDPOutcome{}, err
	}
	if bridge.transport.name() == DisabledCDPTransportName {
		base.Status = CDPOutcomeDisabled
		base.FailureCode = CDPFailureDisabled
		return checkedCDPOutcome(request, base)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return checkedCDPOutcome(request, contextCDPOutcome(base, err))
	}
	deadlineContext, cancel := context.WithTimeout(ctx,
		time.Duration(request.DeadlineMS)*time.Millisecond)
	defer cancel()
	exchange, transportErr := bridge.transport.exchange(deadlineContext, request)
	if err := ctx.Err(); err != nil {
		return checkedCDPOutcome(request, contextCDPOutcome(base, err))
	}
	if err := deadlineContext.Err(); err != nil {
		return checkedCDPOutcome(request, contextCDPOutcome(base, err))
	}
	if transportErr != nil {
		base.Status = CDPOutcomeFailed
		base.FailureCode = CDPFailureTransport
		return checkedCDPOutcome(request, base)
	}
	return checkedCDPOutcome(request, classifyCDPExchange(request, base, exchange))
}

func ValidateCDPOutcome(request CDPRequest, outcome CDPOutcome) error {
	if outcome.ProtocolVersion != CDPOutcomeProtocolVersion ||
		outcome.RequestFingerprint != request.Fingerprint || outcome.RequestID != request.RequestID ||
		outcome.Action != request.Action || outcome.CanonicalURL != request.CanonicalURL ||
		(outcome.Transport != DisabledCDPTransportName && outcome.Transport != FakeCDPTransportName) ||
		outcome.DeadlineMS != request.DeadlineMS || !outcome.Completed ||
		!outcome.DeadlineEnforced || !outcome.ScopeValidated || !outcome.MetadataOnly ||
		outcome.Synthetic != (outcome.Transport == FakeCDPTransportName) ||
		outcome.RawPayloadIncluded || outcome.ProcessStarted || outcome.NetworkUsed ||
		outcome.ProfileWritten || outcome.RequestMutationUsed || outcome.RequestReplayUsed ||
		outcome.ArtifactCommitted || outcome.ProductExecutionEnabled ||
		outcome.Authority != (RuntimeAuthority{}) || outcome.PayloadBytes < 0 ||
		outcome.PayloadBytes > MaxFakeCDPPayloadBytes ||
		outcome.CapturedRequestCount < 0 || outcome.CapturedRequestCount > MaxCDPCaptureEntries ||
		!validBoundedMediaType(outcome.MediaType) {
		return errors.New("browser CDP outcome lost a metadata-only authority boundary")
	}
	if (outcome.PayloadBytes == 0 && outcome.PayloadSHA256 != "") ||
		(outcome.PayloadBytes > 0 && !validSHA256(outcome.PayloadSHA256)) {
		return errors.New("browser CDP outcome payload digest is invalid")
	}
	withinLimit := outcome.PayloadBytes <= request.MaxPayloadBytes
	if outcome.PayloadWithinLimit != withinLimit {
		return errors.New("browser CDP outcome payload limit metadata is inconsistent")
	}
	noResult := outcome.PayloadBytes == 0 && outcome.PayloadSHA256 == "" &&
		outcome.MediaType == "" && outcome.CapturedRequestCount == 0
	switch outcome.Status {
	case CDPOutcomeSucceeded:
		if outcome.Transport != FakeCDPTransportName || outcome.FailureCode != CDPFailureNone ||
			!outcome.PayloadWithinLimit || !validSuccessfulCDPMetadata(request, outcome) {
			return errors.New("browser CDP success outcome is invalid")
		}
	case CDPOutcomeDisabled:
		if outcome.Transport != DisabledCDPTransportName ||
			outcome.FailureCode != CDPFailureDisabled || !noResult {
			return errors.New("browser CDP disabled outcome is invalid")
		}
	case CDPOutcomeTimedOut:
		if outcome.Transport != FakeCDPTransportName ||
			outcome.FailureCode != CDPFailureDeadline || !noResult {
			return errors.New("browser CDP timeout outcome is invalid")
		}
	case CDPOutcomeCancelled:
		if outcome.Transport != FakeCDPTransportName ||
			outcome.FailureCode != CDPFailureCancelled || !noResult {
			return errors.New("browser CDP cancelled outcome is invalid")
		}
	case CDPOutcomeFailed:
		switch outcome.FailureCode {
		case CDPFailureTransport:
			if outcome.Transport != FakeCDPTransportName || !noResult {
				return errors.New("browser CDP transport failure outcome is invalid")
			}
		case CDPFailurePayloadLimit:
			if outcome.Transport != FakeCDPTransportName || outcome.PayloadWithinLimit ||
				outcome.PayloadBytes <= request.MaxPayloadBytes {
				return errors.New("browser CDP payload limit outcome is invalid")
			}
		case CDPFailureInvalidResult:
			if outcome.Transport != FakeCDPTransportName || !outcome.PayloadWithinLimit {
				return errors.New("browser CDP invalid-result outcome is invalid")
			}
		default:
			return errors.New("browser CDP failure code is invalid")
		}
	default:
		return fmt.Errorf("unsupported browser CDP outcome status %q", outcome.Status)
	}
	expected, err := cdpOutcomeFingerprint(outcome)
	if err != nil || outcome.Fingerprint != expected {
		return errors.New("browser CDP outcome fingerprint mismatch")
	}
	return nil
}

func EncodeCDPOutcome(request CDPRequest, outcome CDPOutcome) ([]byte, error) {
	if err := ValidateCDPOutcome(request, outcome); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(outcome)
	if err != nil || len(raw) == 0 || len(raw) > MaxCDPOutcomeEnvelopeBytes {
		return nil, errors.New("browser CDP outcome envelope exceeds its fixed bound")
	}
	return raw, nil
}

func DecodeCDPOutcome(request CDPRequest, raw []byte) (CDPOutcome, error) {
	if len(raw) == 0 || len(raw) > MaxCDPOutcomeEnvelopeBytes {
		return CDPOutcome{}, errors.New("browser CDP outcome envelope is empty or oversized")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var outcome CDPOutcome
	if err := decoder.Decode(&outcome); err != nil {
		return CDPOutcome{}, errors.New("browser CDP outcome envelope is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CDPOutcome{}, errors.New("browser CDP outcome envelope contains trailing data")
	}
	if err := ValidateCDPOutcome(request, outcome); err != nil {
		return CDPOutcome{}, err
	}
	canonical, err := json.Marshal(outcome)
	if err != nil || !bytes.Equal(raw, canonical) {
		return CDPOutcome{}, errors.New("browser CDP outcome envelope is not canonical")
	}
	return outcome, nil
}

func classifyCDPExchange(request CDPRequest, base CDPOutcome,
	exchange cdpExchange,
) CDPOutcome {
	base.PayloadBytes = len(exchange.payload)
	base.MediaType = exchange.mediaType
	base.CapturedRequestCount = exchange.capturedRequestCount
	if len(exchange.payload) > 0 {
		digest := sha256.Sum256(exchange.payload)
		base.PayloadSHA256 = hex.EncodeToString(digest[:])
	}
	base.PayloadWithinLimit = len(exchange.payload) <= request.MaxPayloadBytes
	if !base.PayloadWithinLimit {
		base.Status = CDPOutcomeFailed
		base.FailureCode = CDPFailurePayloadLimit
		return base
	}
	if !validSuccessfulCDPExchange(request, exchange) {
		base.Status = CDPOutcomeFailed
		base.FailureCode = CDPFailureInvalidResult
		return base
	}
	base.Status = CDPOutcomeSucceeded
	return base
}

func validSuccessfulCDPExchange(request CDPRequest, exchange cdpExchange) bool {
	switch request.Action {
	case CDPActionNavigate:
		return len(exchange.payload) == 0 && exchange.mediaType == "" &&
			exchange.capturedRequestCount == 0
	case CDPActionDOMSnapshot:
		return len(exchange.payload) > 0 &&
			exchange.mediaType == "application/vnd.prayu.dom-snapshot+json" &&
			exchange.capturedRequestCount == 0 && json.Valid(exchange.payload)
	case CDPActionScreenshot:
		return len(exchange.payload) > 0 &&
			(exchange.mediaType == "image/png" || exchange.mediaType == "image/jpeg") &&
			exchange.capturedRequestCount == 0 &&
			validScreenshotPayload(exchange.payload, exchange.mediaType)
	case CDPActionRequestCapture:
		return len(exchange.payload) > 0 &&
			exchange.mediaType == "application/vnd.prayu.request-capture+json" &&
			exchange.capturedRequestCount <= request.MaxCaptureEntries &&
			validRequestCapturePayload(exchange.payload, exchange.capturedRequestCount)
	default:
		return false
	}
}

func validScreenshotPayload(payload []byte, mediaType string) bool {
	switch mediaType {
	case "image/png":
		return len(payload) >= 8 && bytes.Equal(payload[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	case "image/jpeg":
		return len(payload) >= 3 && payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff
	default:
		return false
	}
}

func validRequestCapturePayload(payload []byte, expectedCount int) bool {
	var envelope struct {
		Requests []json.RawMessage `json:"requests"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || envelope.Requests == nil ||
		len(envelope.Requests) != expectedCount {
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return false
	}
	for _, request := range envelope.Requests {
		if len(request) == 0 || len(request) > 16*1024 || !json.Valid(request) {
			return false
		}
	}
	return true
}

func validSuccessfulCDPMetadata(request CDPRequest, outcome CDPOutcome) bool {
	switch request.Action {
	case CDPActionNavigate:
		return outcome.PayloadBytes == 0 && outcome.PayloadSHA256 == "" &&
			outcome.MediaType == "" && outcome.CapturedRequestCount == 0
	case CDPActionDOMSnapshot:
		return outcome.PayloadBytes > 0 && validSHA256(outcome.PayloadSHA256) &&
			outcome.MediaType == "application/vnd.prayu.dom-snapshot+json" &&
			outcome.CapturedRequestCount == 0
	case CDPActionScreenshot:
		return outcome.PayloadBytes > 0 && validSHA256(outcome.PayloadSHA256) &&
			(outcome.MediaType == "image/png" || outcome.MediaType == "image/jpeg") &&
			outcome.CapturedRequestCount == 0
	case CDPActionRequestCapture:
		return outcome.PayloadBytes > 0 && validSHA256(outcome.PayloadSHA256) &&
			outcome.MediaType == "application/vnd.prayu.request-capture+json" &&
			outcome.CapturedRequestCount <= request.MaxCaptureEntries
	default:
		return false
	}
}

func newCDPOutcome(request CDPRequest, transport string) (CDPOutcome, error) {
	if transport != DisabledCDPTransportName && transport != FakeCDPTransportName {
		return CDPOutcome{}, errors.New("browser CDP transport name is invalid")
	}
	return CDPOutcome{
		ProtocolVersion:    CDPOutcomeProtocolVersion,
		RequestFingerprint: request.Fingerprint, RequestID: request.RequestID,
		Action: request.Action, CanonicalURL: request.CanonicalURL, Transport: transport,
		DeadlineMS: request.DeadlineMS, Completed: true, DeadlineEnforced: true,
		ScopeValidated: true, PayloadWithinLimit: true, MetadataOnly: true,
		Synthetic: transport == FakeCDPTransportName,
	}, nil
}

func contextCDPOutcome(base CDPOutcome, err error) CDPOutcome {
	if errors.Is(err, context.DeadlineExceeded) {
		base.Status = CDPOutcomeTimedOut
		base.FailureCode = CDPFailureDeadline
		return base
	}
	base.Status = CDPOutcomeCancelled
	base.FailureCode = CDPFailureCancelled
	return base
}

func checkedCDPOutcome(request CDPRequest, outcome CDPOutcome) (CDPOutcome, error) {
	var err error
	outcome.Fingerprint, err = cdpOutcomeFingerprint(outcome)
	if err != nil {
		return CDPOutcome{}, err
	}
	if err := ValidateCDPOutcome(request, outcome); err != nil {
		return CDPOutcome{}, err
	}
	return outcome, nil
}

func cdpRequestFingerprint(value CDPRequest) (string, error) {
	copyValue := value
	copyValue.Fingerprint = ""
	return fingerprintJSON(copyValue, "browser CDP request")
}

func cdpOutcomeFingerprint(value CDPOutcome) (string, error) {
	copyValue := value
	copyValue.Fingerprint = ""
	return fingerprintJSON(copyValue, "browser CDP outcome")
}

func validBoundedMediaType(value string) bool {
	if len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return false
		}
	}
	return !strings.ContainsAny(value, `\\"'`)
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

type cdpExchange struct {
	payload              []byte
	mediaType            string
	capturedRequestCount int
}

var (
	errCDPTransportDisabled  = errors.New("browser CDP transport is disabled")
	errFakeCDPTransportCrash = errors.New("fake browser CDP transport crashed")
	errCDPRequestMismatch    = errors.New("fake browser CDP request fingerprint mismatch")
)
