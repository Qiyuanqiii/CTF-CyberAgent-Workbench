package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
)

const (
	InvocationProtocolVersion           = "analyzer_invocation.v1"
	MaxInvocationCandidateEnvelopeBytes = 8 * 1024
)

// InvocationAuthority is deliberately closed. A candidate records what has
// been reviewed; it cannot grant execution, filesystem, or persistence rights.
type InvocationAuthority struct {
	ProductInvocation bool `json:"product_invocation"`
	ProcessStart      bool `json:"process_start"`
	FileInput         bool `json:"file_input"`
	ResultPersistence bool `json:"result_persistence"`
	ArtifactCommit    bool `json:"artifact_commit"`
}

// InvocationCandidate binds an inline request to one exact Go-owned analyzer
// descriptor. It contains no executable, command, path, environment, or input
// body and cannot start an analyzer.
type InvocationCandidate struct {
	ProtocolVersion  string              `json:"protocol_version"`
	RequestID        string              `json:"request_id"`
	Analyzer         string              `json:"analyzer"`
	Descriptor       Descriptor          `json:"descriptor"`
	DescriptorSHA256 string              `json:"descriptor_sha256"`
	RequestSHA256    string              `json:"request_sha256"`
	InputSHA256      string              `json:"input_sha256"`
	InputBytes       int                 `json:"input_bytes"`
	MediaType        string              `json:"media_type"`
	Limits           Limits              `json:"limits"`
	Capabilities     Capabilities        `json:"capabilities"`
	Authority        InvocationAuthority `json:"authority"`
	Deterministic    bool                `json:"deterministic"`
	MetadataOnly     bool                `json:"metadata_only"`
	InlineInput      bool                `json:"inline_input"`
}

func BuildInvocationCandidate(rawRequest []byte) (InvocationCandidate, ErrorCode) {
	request, code := DecodeRequest(rawRequest)
	if code != "" {
		return InvocationCandidate{}, code
	}
	content, code := decodeContent(request)
	if code != "" {
		return InvocationCandidate{}, code
	}
	descriptor, ok := BuiltinRegistry().Lookup(request.Analyzer)
	if !ok {
		return InvocationCandidate{}, CodeUnsupportedAnalyzer
	}
	descriptorDigest, ok := canonicalSHA256(descriptor)
	if !ok {
		return InvocationCandidate{}, CodeInternal
	}
	requestDigest, ok := canonicalSHA256(request)
	if !ok {
		return InvocationCandidate{}, CodeInternal
	}
	inputDigest := sha256.Sum256(content)
	candidate := InvocationCandidate{
		ProtocolVersion: InvocationProtocolVersion, RequestID: request.RequestID,
		Analyzer: request.Analyzer, Descriptor: descriptor,
		DescriptorSHA256: descriptorDigest, RequestSHA256: requestDigest,
		InputSHA256: hex.EncodeToString(inputDigest[:]), InputBytes: len(content),
		MediaType: request.Input.MediaType, Limits: request.Limits,
		Capabilities: request.Capabilities, Deterministic: descriptor.Deterministic,
		MetadataOnly: request.MetadataOnly && descriptor.MetadataOnly, InlineInput: true,
	}
	if !validateInvocationCandidateStructure(candidate) {
		return InvocationCandidate{}, CodeInternal
	}
	return candidate, ""
}

// ValidateInvocationCandidate is pure and restart-independent: all expected
// state is reconstructed from the fixed registry and the supplied request.
func ValidateInvocationCandidate(candidate InvocationCandidate, rawRequest []byte) ErrorCode {
	expected, code := BuildInvocationCandidate(rawRequest)
	if code != "" {
		return code
	}
	if !reflect.DeepEqual(candidate, expected) {
		return CodeInvalidResult
	}
	return ""
}

func EncodeInvocationCandidate(candidate InvocationCandidate, rawRequest []byte) ([]byte, ErrorCode) {
	if code := ValidateInvocationCandidate(candidate, rawRequest); code != "" {
		return nil, code
	}
	encoded, err := json.Marshal(candidate)
	if err != nil || len(encoded) == 0 || len(encoded) > MaxInvocationCandidateEnvelopeBytes {
		return nil, CodeInternal
	}
	return encoded, ""
}

func DecodeInvocationCandidate(rawCandidate, rawRequest []byte) (InvocationCandidate, ErrorCode) {
	var wire invocationCandidateWire
	if !strictDecode(rawCandidate, MaxInvocationCandidateEnvelopeBytes, &wire) || !wire.complete() {
		return InvocationCandidate{}, CodeInvalidResult
	}
	candidate := wire.value()
	if code := ValidateInvocationCandidate(candidate, rawRequest); code != "" {
		return InvocationCandidate{}, CodeInvalidResult
	}
	return candidate, ""
}

func invocationCandidateSHA256(candidate InvocationCandidate) (string, bool) {
	if !validateInvocationCandidateStructure(candidate) {
		return "", false
	}
	return canonicalSHA256(candidate)
}

func validateInvocationCandidateStructure(candidate InvocationCandidate) bool {
	if candidate.ProtocolVersion != InvocationProtocolVersion ||
		!validRequestID(candidate.RequestID) || candidate.Analyzer == "" ||
		candidate.Descriptor.Analyzer != candidate.Analyzer ||
		!ValidateDescriptor(candidate.Descriptor) ||
		!validDigest(candidate.DescriptorSHA256) ||
		!validDigest(candidate.RequestSHA256) || !validDigest(candidate.InputSHA256) ||
		candidate.InputBytes < 0 || candidate.InputBytes > candidate.Limits.MaxInputBytes ||
		candidate.InputBytes > candidate.Descriptor.Limits.MaxInputBytes ||
		!validMediaType(candidate.MediaType) ||
		!descriptorAcceptsMediaType(candidate.Descriptor, candidate.MediaType) ||
		candidate.Limits.MaxInputBytes < 1 ||
		candidate.Limits.MaxInputBytes > candidate.Descriptor.Limits.MaxInputBytes ||
		candidate.Limits.MaxOutputBytes < MinResultEnvelopeBytes ||
		candidate.Limits.MaxOutputBytes > candidate.Descriptor.Limits.MaxOutputBytes ||
		candidate.Limits.TimeoutMilliseconds < MinTimeoutMilliseconds ||
		candidate.Limits.TimeoutMilliseconds > candidate.Descriptor.Limits.TimeoutMilliseconds ||
		capabilitiesEnabled(candidate.Capabilities) ||
		candidate.Capabilities != candidate.Descriptor.Capabilities ||
		candidate.Authority != (InvocationAuthority{}) ||
		candidate.Descriptor.Authority != (DescriptorAuthority{}) ||
		!candidate.Deterministic || candidate.Deterministic != candidate.Descriptor.Deterministic ||
		!candidate.MetadataOnly || candidate.MetadataOnly != candidate.Descriptor.MetadataOnly ||
		!candidate.InlineInput {
		return false
	}
	descriptorDigest, ok := canonicalSHA256(candidate.Descriptor)
	return ok && descriptorDigest == candidate.DescriptorSHA256
}

func canonicalSHA256(value any) (string, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), true
}

type invocationCandidateWire struct {
	ProtocolVersion  *string                  `json:"protocol_version"`
	RequestID        *string                  `json:"request_id"`
	Analyzer         *string                  `json:"analyzer"`
	Descriptor       *descriptorWire          `json:"descriptor"`
	DescriptorSHA256 *string                  `json:"descriptor_sha256"`
	RequestSHA256    *string                  `json:"request_sha256"`
	InputSHA256      *string                  `json:"input_sha256"`
	InputBytes       *int                     `json:"input_bytes"`
	MediaType        *string                  `json:"media_type"`
	Limits           *limitsWire              `json:"limits"`
	Capabilities     *capabilitiesWire        `json:"capabilities"`
	Authority        *invocationAuthorityWire `json:"authority"`
	Deterministic    *bool                    `json:"deterministic"`
	MetadataOnly     *bool                    `json:"metadata_only"`
	InlineInput      *bool                    `json:"inline_input"`
}

type invocationAuthorityWire struct {
	ProductInvocation *bool `json:"product_invocation"`
	ProcessStart      *bool `json:"process_start"`
	FileInput         *bool `json:"file_input"`
	ResultPersistence *bool `json:"result_persistence"`
	ArtifactCommit    *bool `json:"artifact_commit"`
}

func (wire invocationCandidateWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.RequestID != nil && wire.Analyzer != nil &&
		wire.Descriptor != nil && wire.Descriptor.complete() && wire.DescriptorSHA256 != nil &&
		wire.RequestSHA256 != nil && wire.InputSHA256 != nil && wire.InputBytes != nil &&
		wire.MediaType != nil && wire.Limits != nil &&
		wire.Limits.MaxInputBytes != nil && wire.Limits.MaxOutputBytes != nil &&
		wire.Limits.TimeoutMilliseconds != nil &&
		wire.Capabilities != nil && wire.Capabilities.complete() && wire.Authority != nil &&
		wire.Authority.complete() && wire.Deterministic != nil && wire.MetadataOnly != nil &&
		wire.InlineInput != nil
}

func (wire invocationCandidateWire) value() InvocationCandidate {
	return InvocationCandidate{
		ProtocolVersion: *wire.ProtocolVersion, RequestID: *wire.RequestID,
		Analyzer: *wire.Analyzer, Descriptor: wire.Descriptor.value(),
		DescriptorSHA256: *wire.DescriptorSHA256, RequestSHA256: *wire.RequestSHA256,
		InputSHA256: *wire.InputSHA256, InputBytes: *wire.InputBytes,
		MediaType: *wire.MediaType,
		Limits: Limits{MaxInputBytes: *wire.Limits.MaxInputBytes,
			MaxOutputBytes:      *wire.Limits.MaxOutputBytes,
			TimeoutMilliseconds: *wire.Limits.TimeoutMilliseconds},
		Capabilities: wire.Capabilities.value(), Authority: wire.Authority.value(),
		Deterministic: *wire.Deterministic, MetadataOnly: *wire.MetadataOnly,
		InlineInput: *wire.InlineInput,
	}
}

func (wire invocationAuthorityWire) complete() bool {
	return wire.ProductInvocation != nil && wire.ProcessStart != nil && wire.FileInput != nil &&
		wire.ResultPersistence != nil && wire.ArtifactCommit != nil
}

func (wire invocationAuthorityWire) value() InvocationAuthority {
	return InvocationAuthority{
		ProductInvocation: *wire.ProductInvocation, ProcessStart: *wire.ProcessStart,
		FileInput: *wire.FileInput, ResultPersistence: *wire.ResultPersistence,
		ArtifactCommit: *wire.ArtifactCommit,
	}
}
