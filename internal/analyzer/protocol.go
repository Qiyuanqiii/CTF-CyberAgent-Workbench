package analyzer

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	RequestProtocolVersion = "analyzer_protocol.v1"
	ResultProtocolVersion  = "analyzer_result.v1"
	ErrorProtocolVersion   = "analyzer_error.v1"
	FixtureAnalyzerName    = "fixture.digest.v1"

	MaxRequestEnvelopeBytes = 96 * 1024
	MaxDecodedInputBytes    = 64 * 1024
	MinResultEnvelopeBytes  = 512
	MaxResultEnvelopeBytes  = 16 * 1024
	MinTimeoutMilliseconds  = 100
	MaxTimeoutMilliseconds  = 30_000
	MaxRequestIDBytes       = 128
	MaxMediaTypeBytes       = 128
	maxJSONNestingDepth     = 8

	ExitSuccess  = 0
	ExitRejected = 2
	ExitInternal = 3
)

type ErrorCode string

const (
	CodeMalformedEnvelope   ErrorCode = "malformed_envelope"
	CodeRequestTooLarge     ErrorCode = "request_too_large"
	CodeUnsupportedProtocol ErrorCode = "unsupported_protocol"
	CodeInvalidRequest      ErrorCode = "invalid_request"
	CodeCapabilityDenied    ErrorCode = "capability_denied"
	CodeUnsupportedAnalyzer ErrorCode = "unsupported_analyzer"
	CodeInputLimitExceeded  ErrorCode = "input_limit_exceeded"
	CodeInvalidContent      ErrorCode = "invalid_content"
	CodeOutputLimitExceeded ErrorCode = "output_limit_exceeded"
	CodeResultTooLarge      ErrorCode = "result_too_large"
	CodeDeadlineExceeded    ErrorCode = "deadline_exceeded"
	CodeProcessFailed       ErrorCode = "process_failed"
	CodeInvalidResult       ErrorCode = "invalid_result"
	CodeInternal            ErrorCode = "internal_error"
)

type Capabilities struct {
	Filesystem  bool `json:"filesystem"`
	Network     bool `json:"network"`
	Subprocess  bool `json:"subprocess"`
	Environment bool `json:"environment"`
}

type Limits struct {
	MaxInputBytes       int `json:"max_input_bytes"`
	MaxOutputBytes      int `json:"max_output_bytes"`
	TimeoutMilliseconds int `json:"timeout_ms"`
}

type Input struct {
	MediaType     string `json:"media_type"`
	ContentBase64 string `json:"content_base64"`
}

type Request struct {
	ProtocolVersion string       `json:"protocol_version"`
	RequestID       string       `json:"request_id"`
	Analyzer        string       `json:"analyzer"`
	Input           Input        `json:"input"`
	Limits          Limits       `json:"limits"`
	Capabilities    Capabilities `json:"capabilities"`
	MetadataOnly    bool         `json:"metadata_only"`
}

type Summary struct {
	MediaType  string `json:"media_type"`
	InputBytes int    `json:"input_bytes"`
	SHA256     string `json:"sha256"`
	UTF8       bool   `json:"utf8"`
	LineCount  int    `json:"line_count"`
}

type Result struct {
	ProtocolVersion  string       `json:"protocol_version"`
	RequestID        string       `json:"request_id"`
	Analyzer         string       `json:"analyzer"`
	Status           string       `json:"status"`
	Summary          Summary      `json:"summary"`
	MetadataOnly     bool         `json:"metadata_only"`
	CapabilitiesUsed Capabilities `json:"capabilities_used"`
}

type ErrorEnvelope struct {
	ProtocolVersion string    `json:"protocol_version"`
	RequestID       string    `json:"request_id"`
	Code            ErrorCode `json:"code"`
	Retryable       bool      `json:"retryable"`
	Message         string    `json:"message"`
	MetadataOnly    bool      `json:"metadata_only"`
}

func DecodeRequest(raw []byte) (Request, ErrorCode) {
	if len(raw) > MaxRequestEnvelopeBytes {
		return Request{}, CodeRequestTooLarge
	}
	var wire requestWire
	if !strictDecode(raw, MaxRequestEnvelopeBytes, &wire) {
		return Request{}, CodeMalformedEnvelope
	}
	if !wire.complete() {
		return Request{}, CodeInvalidRequest
	}
	request := wire.value()
	return request, ValidateRequest(request)
}

func ValidateRequest(request Request) ErrorCode {
	if request.ProtocolVersion != RequestProtocolVersion {
		return CodeUnsupportedProtocol
	}
	if !validRequestID(request.RequestID) {
		return CodeInvalidRequest
	}
	if request.Analyzer != FixtureAnalyzerName {
		return CodeUnsupportedAnalyzer
	}
	if capabilitiesEnabled(request.Capabilities) {
		return CodeCapabilityDenied
	}
	if !request.MetadataOnly || request.Limits.MaxInputBytes < 1 ||
		request.Limits.MaxInputBytes > MaxDecodedInputBytes ||
		request.Limits.MaxOutputBytes < MinResultEnvelopeBytes ||
		request.Limits.MaxOutputBytes > MaxResultEnvelopeBytes ||
		request.Limits.TimeoutMilliseconds < MinTimeoutMilliseconds ||
		request.Limits.TimeoutMilliseconds > MaxTimeoutMilliseconds ||
		!validMediaType(request.Input.MediaType) {
		return CodeInvalidRequest
	}
	_, code := decodeContent(request)
	return code
}

// EvaluateFixture is a pure protocol reference. It does not start a process or read files.
func EvaluateFixture(raw []byte) ([]byte, int) {
	request, code := DecodeRequest(raw)
	if code != "" {
		return encodeError(safeRequestID(request.RequestID), code), ExitRejected
	}
	content, code := decodeContent(request)
	if code != "" {
		return encodeError(request.RequestID, code), ExitRejected
	}
	digest := sha256.Sum256(content)
	text := utf8.Valid(content)
	result := Result{
		ProtocolVersion: ResultProtocolVersion,
		RequestID:       request.RequestID,
		Analyzer:        request.Analyzer,
		Status:          "succeeded",
		Summary: Summary{
			MediaType: request.Input.MediaType, InputBytes: len(content),
			SHA256: hex.EncodeToString(digest[:]), UTF8: text,
			LineCount: logicalLineCount(content, text),
		},
		MetadataOnly: true,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return encodeError(request.RequestID, CodeInternal), ExitRejected
	}
	if len(encoded) > request.Limits.MaxOutputBytes || len(encoded) > MaxResultEnvelopeBytes {
		return encodeError(request.RequestID, CodeOutputLimitExceeded), ExitRejected
	}
	return encoded, ExitSuccess
}

func DecodeResult(raw []byte) (Result, ErrorCode) {
	var wire resultWire
	if !strictDecode(raw, MaxResultEnvelopeBytes, &wire) {
		if len(raw) > MaxResultEnvelopeBytes {
			return Result{}, CodeResultTooLarge
		}
		return Result{}, CodeInvalidResult
	}
	if !wire.complete() {
		return Result{}, CodeInvalidResult
	}
	result := wire.value()
	if result.ProtocolVersion != ResultProtocolVersion {
		return Result{}, CodeUnsupportedProtocol
	}
	if !validRequestID(result.RequestID) || result.Analyzer != FixtureAnalyzerName ||
		result.Status != "succeeded" || !result.MetadataOnly ||
		capabilitiesEnabled(result.CapabilitiesUsed) ||
		!validMediaType(result.Summary.MediaType) || result.Summary.InputBytes < 0 ||
		result.Summary.InputBytes > MaxDecodedInputBytes || !validDigest(result.Summary.SHA256) ||
		result.Summary.LineCount < 0 || result.Summary.LineCount > result.Summary.InputBytes ||
		(!result.Summary.UTF8 && result.Summary.LineCount != 0) ||
		(result.Summary.UTF8 && result.Summary.InputBytes > 0 && result.Summary.LineCount == 0) ||
		(result.Summary.InputBytes == 0 && result.Summary.LineCount != 0) {
		return Result{}, CodeInvalidResult
	}
	return result, ""
}

func DecodeError(raw []byte) (ErrorEnvelope, ErrorCode) {
	if len(raw) > MaxResultEnvelopeBytes {
		return ErrorEnvelope{}, CodeResultTooLarge
	}
	var wire errorWire
	if !strictDecode(raw, MaxResultEnvelopeBytes, &wire) || !wire.complete() {
		return ErrorEnvelope{}, CodeInvalidResult
	}
	value := wire.value()
	if value.ProtocolVersion != ErrorProtocolVersion ||
		(value.RequestID != "" && !validRequestID(value.RequestID)) ||
		messageFor(value.Code) == "" || value.Message != messageFor(value.Code) ||
		value.Retryable != retryable(value.Code) || !value.MetadataOnly {
		return ErrorEnvelope{}, CodeInvalidResult
	}
	return value, ""
}

func decodeContent(request Request) ([]byte, ErrorCode) {
	maximumEncoded := base64.StdEncoding.EncodedLen(MaxDecodedInputBytes)
	if len(request.Input.ContentBase64) > maximumEncoded {
		return nil, CodeInputLimitExceeded
	}
	content, err := base64.StdEncoding.Strict().DecodeString(request.Input.ContentBase64)
	if err != nil || base64.StdEncoding.EncodeToString(content) != request.Input.ContentBase64 {
		return nil, CodeInvalidContent
	}
	if len(content) > request.Limits.MaxInputBytes || len(content) > MaxDecodedInputBytes {
		return nil, CodeInputLimitExceeded
	}
	return content, ""
}

func encodeError(requestID string, code ErrorCode) []byte {
	encoded, _ := json.Marshal(ErrorEnvelope{
		ProtocolVersion: ErrorProtocolVersion, RequestID: requestID, Code: code,
		Retryable: retryable(code), Message: messageFor(code), MetadataOnly: true,
	})
	return encoded
}

func logicalLineCount(content []byte, text bool) int {
	if !text || len(content) == 0 {
		return 0
	}
	count := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		count++
	}
	return count
}

func capabilitiesEnabled(value Capabilities) bool {
	return value.Filesystem || value.Network || value.Subprocess || value.Environment
}

func validRequestID(value string) bool {
	if len(value) < 1 || len(value) > MaxRequestIDBytes {
		return false
	}
	for _, char := range []byte(value) {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("._:-", rune(char))) {
			return false
		}
	}
	return true
}

func safeRequestID(value string) string {
	if validRequestID(value) {
		return value
	}
	return ""
}

func validMediaType(value string) bool {
	if len(value) < 3 || len(value) > MaxMediaTypeBytes || strings.Count(value, "/") != 1 {
		return false
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, char := range []byte(part) {
			if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
				strings.ContainsRune("!#$&^_.+-", rune(char))) {
				return false
			}
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func messageFor(code ErrorCode) string {
	switch code {
	case CodeMalformedEnvelope:
		return "analyzer request is malformed"
	case CodeRequestTooLarge:
		return "analyzer request exceeds the envelope limit"
	case CodeUnsupportedProtocol:
		return "analyzer protocol is unsupported"
	case CodeInvalidRequest:
		return "analyzer request violates the protocol contract"
	case CodeCapabilityDenied:
		return "requested analyzer capability is disabled"
	case CodeUnsupportedAnalyzer:
		return "analyzer is unsupported"
	case CodeInputLimitExceeded:
		return "analyzer input exceeds its declared limit"
	case CodeInvalidContent:
		return "analyzer input content is invalid"
	case CodeOutputLimitExceeded:
		return "analyzer output exceeds its declared limit"
	case CodeResultTooLarge:
		return "analyzer result exceeds the envelope limit"
	case CodeDeadlineExceeded:
		return "analyzer deadline was exceeded"
	case CodeProcessFailed:
		return "analyzer process failed"
	case CodeInvalidResult:
		return "analyzer result violates the protocol contract"
	case CodeInternal:
		return "analyzer failed internally"
	default:
		return ""
	}
}

func retryable(code ErrorCode) bool {
	return code == CodeDeadlineExceeded || code == CodeProcessFailed
}

func strictDecode(raw []byte, maximum int, target any) bool {
	if len(raw) == 0 || len(raw) > maximum || !utf8.Valid(raw) {
		return false
	}
	structure := json.NewDecoder(bytes.NewReader(raw))
	structure.UseNumber()
	if err := consumeJSONValue(structure, 0); err != nil || requireJSONEOF(structure) != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || requireJSONEOF(decoder) != nil {
		return false
	}
	return true
}

func consumeJSONValue(decoder *json.Decoder, depth int) error {
	if depth > maxJSONNestingDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels", maxJSONNestingDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is invalid")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("JSON object key %q is duplicated", key)
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("JSON object is incomplete")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("JSON array is incomplete")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON contains a trailing value")
		}
		return err
	}
	return nil
}

type requestWire struct {
	ProtocolVersion *string           `json:"protocol_version"`
	RequestID       *string           `json:"request_id"`
	Analyzer        *string           `json:"analyzer"`
	Input           *inputWire        `json:"input"`
	Limits          *limitsWire       `json:"limits"`
	Capabilities    *capabilitiesWire `json:"capabilities"`
	MetadataOnly    *bool             `json:"metadata_only"`
}

type inputWire struct {
	MediaType     *string `json:"media_type"`
	ContentBase64 *string `json:"content_base64"`
}

type limitsWire struct {
	MaxInputBytes       *int `json:"max_input_bytes"`
	MaxOutputBytes      *int `json:"max_output_bytes"`
	TimeoutMilliseconds *int `json:"timeout_ms"`
}

type capabilitiesWire struct {
	Filesystem  *bool `json:"filesystem"`
	Network     *bool `json:"network"`
	Subprocess  *bool `json:"subprocess"`
	Environment *bool `json:"environment"`
}

func (wire requestWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.RequestID != nil && wire.Analyzer != nil &&
		wire.Input != nil && wire.Input.MediaType != nil && wire.Input.ContentBase64 != nil &&
		wire.Limits != nil && wire.Limits.MaxInputBytes != nil &&
		wire.Limits.MaxOutputBytes != nil && wire.Limits.TimeoutMilliseconds != nil &&
		wire.Capabilities != nil && wire.Capabilities.complete() && wire.MetadataOnly != nil
}

func (wire requestWire) value() Request {
	return Request{ProtocolVersion: *wire.ProtocolVersion, RequestID: *wire.RequestID,
		Analyzer: *wire.Analyzer,
		Input:    Input{MediaType: *wire.Input.MediaType, ContentBase64: *wire.Input.ContentBase64},
		Limits: Limits{MaxInputBytes: *wire.Limits.MaxInputBytes,
			MaxOutputBytes:      *wire.Limits.MaxOutputBytes,
			TimeoutMilliseconds: *wire.Limits.TimeoutMilliseconds},
		Capabilities: wire.Capabilities.value(), MetadataOnly: *wire.MetadataOnly}
}

func (wire capabilitiesWire) complete() bool {
	return wire.Filesystem != nil && wire.Network != nil && wire.Subprocess != nil &&
		wire.Environment != nil
}

func (wire capabilitiesWire) value() Capabilities {
	return Capabilities{Filesystem: *wire.Filesystem, Network: *wire.Network,
		Subprocess: *wire.Subprocess, Environment: *wire.Environment}
}

type resultWire struct {
	ProtocolVersion  *string           `json:"protocol_version"`
	RequestID        *string           `json:"request_id"`
	Analyzer         *string           `json:"analyzer"`
	Status           *string           `json:"status"`
	Summary          *summaryWire      `json:"summary"`
	MetadataOnly     *bool             `json:"metadata_only"`
	CapabilitiesUsed *capabilitiesWire `json:"capabilities_used"`
}

type summaryWire struct {
	MediaType  *string `json:"media_type"`
	InputBytes *int    `json:"input_bytes"`
	SHA256     *string `json:"sha256"`
	UTF8       *bool   `json:"utf8"`
	LineCount  *int    `json:"line_count"`
}

func (wire resultWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.RequestID != nil && wire.Analyzer != nil &&
		wire.Status != nil && wire.Summary != nil && wire.Summary.complete() &&
		wire.MetadataOnly != nil && wire.CapabilitiesUsed != nil &&
		wire.CapabilitiesUsed.complete()
}

func (wire summaryWire) complete() bool {
	return wire.MediaType != nil && wire.InputBytes != nil && wire.SHA256 != nil &&
		wire.UTF8 != nil && wire.LineCount != nil
}

func (wire resultWire) value() Result {
	return Result{ProtocolVersion: *wire.ProtocolVersion, RequestID: *wire.RequestID,
		Analyzer: *wire.Analyzer, Status: *wire.Status,
		Summary: Summary{MediaType: *wire.Summary.MediaType,
			InputBytes: *wire.Summary.InputBytes, SHA256: *wire.Summary.SHA256,
			UTF8: *wire.Summary.UTF8, LineCount: *wire.Summary.LineCount},
		MetadataOnly: *wire.MetadataOnly, CapabilitiesUsed: wire.CapabilitiesUsed.value()}
}

type errorWire struct {
	ProtocolVersion *string    `json:"protocol_version"`
	RequestID       *string    `json:"request_id"`
	Code            *ErrorCode `json:"code"`
	Retryable       *bool      `json:"retryable"`
	Message         *string    `json:"message"`
	MetadataOnly    *bool      `json:"metadata_only"`
}

func (wire errorWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.RequestID != nil && wire.Code != nil &&
		wire.Retryable != nil && wire.Message != nil && wire.MetadataOnly != nil
}

func (wire errorWire) value() ErrorEnvelope {
	return ErrorEnvelope{ProtocolVersion: *wire.ProtocolVersion, RequestID: *wire.RequestID,
		Code: *wire.Code, Retryable: *wire.Retryable, Message: *wire.Message,
		MetadataOnly: *wire.MetadataOnly}
}
