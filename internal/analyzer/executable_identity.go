package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"runtime"
)

const (
	ExecutableIdentityProtocolVersion   = "analyzer_executable_identity.v1"
	InvocationPreflightProtocolVersion  = "analyzer_invocation_preflight.v1"
	MaxAnalyzerExecutableBytes          = 32 * 1024 * 1024
	MaxExecutableIdentityEnvelopeBytes  = 2 * 1024
	MaxInvocationPreflightEnvelopeBytes = 3 * 1024
)

// ExecutableIdentity binds one descriptor to exact executable bytes for one
// target platform. It is inert metadata: paths, commands, environments, raw
// bytes, persistence, and process-start authority are deliberately absent.
type ExecutableIdentity struct {
	ProtocolVersion             string `json:"protocol_version"`
	Analyzer                    string `json:"analyzer"`
	DescriptorSHA256            string `json:"descriptor_sha256"`
	RequestProtocol             string `json:"request_protocol"`
	ResultProtocol              string `json:"result_protocol"`
	ErrorProtocol               string `json:"error_protocol"`
	TargetGOOS                  string `json:"target_goos"`
	TargetGOARCH                string `json:"target_goarch"`
	ExecutableBytes             int    `json:"executable_bytes"`
	ExecutableSHA256            string `json:"executable_sha256"`
	DescriptorDeterministic     bool   `json:"descriptor_deterministic"`
	MetadataOnly                bool   `json:"metadata_only"`
	ExecutableBytesIncluded     bool   `json:"executable_bytes_included"`
	PathIncluded                bool   `json:"path_included"`
	CommandIncluded             bool   `json:"command_included"`
	EnvironmentIncluded         bool   `json:"environment_included"`
	ExecutableFormatVerified    bool   `json:"executable_format_verified"`
	ExecutableSemanticsVerified bool   `json:"executable_semantics_verified"`
	ProcessStartEnabled         bool   `json:"process_start_enabled"`
	ProductInvocationEnabled    bool   `json:"product_invocation_enabled"`
}

// InvocationPreflight proves that a candidate and executable identity were
// reconstructed together. Passing this metadata check is sufficient only for
// the separately compiled test conformance harness, never for product launch.
type InvocationPreflight struct {
	ProtocolVersion             string              `json:"protocol_version"`
	CandidateSHA256             string              `json:"candidate_sha256"`
	ExecutableIdentitySHA256    string              `json:"executable_identity_sha256"`
	RequestID                   string              `json:"request_id"`
	Analyzer                    string              `json:"analyzer"`
	RequestProtocol             string              `json:"request_protocol"`
	ResultProtocol              string              `json:"result_protocol"`
	ErrorProtocol               string              `json:"error_protocol"`
	TargetGOOS                  string              `json:"target_goos"`
	TargetGOARCH                string              `json:"target_goarch"`
	ExecutableBytes             int                 `json:"executable_bytes"`
	ExecutableSHA256            string              `json:"executable_sha256"`
	Authority                   InvocationAuthority `json:"authority"`
	CandidateValidated          bool                `json:"candidate_validated"`
	DescriptorBound             bool                `json:"descriptor_bound"`
	ProtocolsBound              bool                `json:"protocols_bound"`
	PlatformRecorded            bool                `json:"platform_recorded"`
	ExecutableBound             bool                `json:"executable_bound"`
	TestConformanceOnly         bool                `json:"test_conformance_only"`
	MetadataOnly                bool                `json:"metadata_only"`
	ExecutableFormatVerified    bool                `json:"executable_format_verified"`
	ExecutableSemanticsVerified bool                `json:"executable_semantics_verified"`
	ProcessStartEnabled         bool                `json:"process_start_enabled"`
	ProductInvocationEnabled    bool                `json:"product_invocation_enabled"`
}

func BuildExecutableIdentity(candidate InvocationCandidate, rawRequest,
	executable []byte,
) (ExecutableIdentity, ErrorCode) {
	if code := ValidateInvocationCandidate(candidate, rawRequest); code != "" {
		return ExecutableIdentity{}, CodeInvalidResult
	}
	if len(executable) == 0 {
		return ExecutableIdentity{}, CodeInvalidContent
	}
	if len(executable) > MaxAnalyzerExecutableBytes {
		return ExecutableIdentity{}, CodeInputLimitExceeded
	}
	digest := sha256.Sum256(executable)
	identity := ExecutableIdentity{
		ProtocolVersion: ExecutableIdentityProtocolVersion,
		Analyzer:        candidate.Analyzer, DescriptorSHA256: candidate.DescriptorSHA256,
		RequestProtocol: candidate.Descriptor.RequestProtocol,
		ResultProtocol:  candidate.Descriptor.ResultProtocol, ErrorProtocol: ErrorProtocolVersion,
		TargetGOOS: runtime.GOOS, TargetGOARCH: runtime.GOARCH,
		ExecutableBytes: len(executable), ExecutableSHA256: hex.EncodeToString(digest[:]),
		DescriptorDeterministic: candidate.Deterministic, MetadataOnly: true,
	}
	if !validateExecutableIdentityStructure(candidate, identity) {
		return ExecutableIdentity{}, CodeInternal
	}
	return identity, ""
}

func ValidateExecutableIdentity(identity ExecutableIdentity, candidate InvocationCandidate,
	rawRequest, executable []byte,
) ErrorCode {
	expected, code := BuildExecutableIdentity(candidate, rawRequest, executable)
	if code != "" {
		return code
	}
	if !reflect.DeepEqual(identity, expected) {
		return CodeInvalidResult
	}
	return ""
}

func EncodeExecutableIdentity(identity ExecutableIdentity, candidate InvocationCandidate,
	rawRequest, executable []byte,
) ([]byte, ErrorCode) {
	if code := ValidateExecutableIdentity(identity, candidate, rawRequest, executable); code != "" {
		return nil, code
	}
	encoded, err := json.Marshal(identity)
	if err != nil || len(encoded) == 0 || len(encoded) > MaxExecutableIdentityEnvelopeBytes {
		return nil, CodeInternal
	}
	return encoded, ""
}

func DecodeExecutableIdentity(rawIdentity []byte, candidate InvocationCandidate,
	rawRequest, executable []byte,
) (ExecutableIdentity, ErrorCode) {
	var wire executableIdentityWire
	if !strictDecode(rawIdentity, MaxExecutableIdentityEnvelopeBytes, &wire) || !wire.complete() {
		return ExecutableIdentity{}, CodeInvalidResult
	}
	identity := wire.value()
	if code := ValidateExecutableIdentity(identity, candidate, rawRequest, executable); code != "" {
		return ExecutableIdentity{}, CodeInvalidResult
	}
	return identity, ""
}

func BuildInvocationPreflight(candidate InvocationCandidate, rawRequest, executable []byte,
	identity ExecutableIdentity,
) (InvocationPreflight, ErrorCode) {
	if code := ValidateExecutableIdentity(identity, candidate, rawRequest, executable); code != "" {
		return InvocationPreflight{}, CodeInvalidResult
	}
	candidateDigest, ok := invocationCandidateSHA256(candidate)
	if !ok {
		return InvocationPreflight{}, CodeInternal
	}
	identityDigest, ok := canonicalSHA256(identity)
	if !ok {
		return InvocationPreflight{}, CodeInternal
	}
	preflight := InvocationPreflight{
		ProtocolVersion: InvocationPreflightProtocolVersion,
		CandidateSHA256: candidateDigest, ExecutableIdentitySHA256: identityDigest,
		RequestID: candidate.RequestID, Analyzer: candidate.Analyzer,
		RequestProtocol: identity.RequestProtocol, ResultProtocol: identity.ResultProtocol,
		ErrorProtocol: identity.ErrorProtocol, TargetGOOS: identity.TargetGOOS,
		TargetGOARCH: identity.TargetGOARCH, ExecutableBytes: identity.ExecutableBytes,
		ExecutableSHA256: identity.ExecutableSHA256, CandidateValidated: true,
		DescriptorBound: true, ProtocolsBound: true, PlatformRecorded: true,
		ExecutableBound: true, TestConformanceOnly: true, MetadataOnly: true,
	}
	if !validateInvocationPreflightStructure(candidate, identity, preflight) {
		return InvocationPreflight{}, CodeInternal
	}
	return preflight, ""
}

func ValidateInvocationPreflight(preflight InvocationPreflight, candidate InvocationCandidate,
	rawRequest, executable []byte, identity ExecutableIdentity,
) ErrorCode {
	expected, code := BuildInvocationPreflight(candidate, rawRequest, executable, identity)
	if code != "" {
		return code
	}
	if !reflect.DeepEqual(preflight, expected) {
		return CodeInvalidResult
	}
	return ""
}

func EncodeInvocationPreflight(preflight InvocationPreflight, candidate InvocationCandidate,
	rawRequest, executable []byte, identity ExecutableIdentity,
) ([]byte, ErrorCode) {
	if code := ValidateInvocationPreflight(preflight, candidate, rawRequest, executable,
		identity); code != "" {
		return nil, code
	}
	encoded, err := json.Marshal(preflight)
	if err != nil || len(encoded) == 0 || len(encoded) > MaxInvocationPreflightEnvelopeBytes {
		return nil, CodeInternal
	}
	return encoded, ""
}

func DecodeInvocationPreflight(rawPreflight []byte, candidate InvocationCandidate,
	rawRequest, executable []byte, identity ExecutableIdentity,
) (InvocationPreflight, ErrorCode) {
	var wire invocationPreflightWire
	if !strictDecode(rawPreflight, MaxInvocationPreflightEnvelopeBytes, &wire) ||
		!wire.complete() {
		return InvocationPreflight{}, CodeInvalidResult
	}
	preflight := wire.value()
	if code := ValidateInvocationPreflight(preflight, candidate, rawRequest, executable,
		identity); code != "" {
		return InvocationPreflight{}, CodeInvalidResult
	}
	return preflight, ""
}

func validateExecutableIdentityStructure(candidate InvocationCandidate,
	identity ExecutableIdentity,
) bool {
	return identity.ProtocolVersion == ExecutableIdentityProtocolVersion &&
		identity.Analyzer == candidate.Analyzer &&
		identity.DescriptorSHA256 == candidate.DescriptorSHA256 &&
		identity.RequestProtocol == candidate.Descriptor.RequestProtocol &&
		identity.ResultProtocol == candidate.Descriptor.ResultProtocol &&
		identity.ErrorProtocol == ErrorProtocolVersion && identity.TargetGOOS == runtime.GOOS &&
		identity.TargetGOARCH == runtime.GOARCH && identity.ExecutableBytes > 0 &&
		identity.ExecutableBytes <= MaxAnalyzerExecutableBytes &&
		validDigest(identity.ExecutableSHA256) && identity.DescriptorDeterministic &&
		identity.DescriptorDeterministic == candidate.Deterministic && identity.MetadataOnly &&
		!identity.ExecutableBytesIncluded && !identity.PathIncluded && !identity.CommandIncluded &&
		!identity.EnvironmentIncluded && !identity.ExecutableFormatVerified &&
		!identity.ExecutableSemanticsVerified &&
		!identity.ProcessStartEnabled && !identity.ProductInvocationEnabled
}

func validateInvocationPreflightStructure(candidate InvocationCandidate,
	identity ExecutableIdentity, preflight InvocationPreflight,
) bool {
	candidateDigest, candidateOK := invocationCandidateSHA256(candidate)
	identityDigest, identityOK := canonicalSHA256(identity)
	return candidateOK && identityOK &&
		preflight.ProtocolVersion == InvocationPreflightProtocolVersion &&
		preflight.CandidateSHA256 == candidateDigest &&
		preflight.ExecutableIdentitySHA256 == identityDigest &&
		preflight.RequestID == candidate.RequestID && preflight.Analyzer == candidate.Analyzer &&
		preflight.RequestProtocol == identity.RequestProtocol &&
		preflight.ResultProtocol == identity.ResultProtocol &&
		preflight.ErrorProtocol == identity.ErrorProtocol &&
		preflight.TargetGOOS == identity.TargetGOOS &&
		preflight.TargetGOARCH == identity.TargetGOARCH &&
		preflight.ExecutableBytes == identity.ExecutableBytes &&
		preflight.ExecutableSHA256 == identity.ExecutableSHA256 &&
		preflight.Authority == (InvocationAuthority{}) && preflight.CandidateValidated &&
		preflight.DescriptorBound && preflight.ProtocolsBound && preflight.PlatformRecorded &&
		preflight.ExecutableBound && preflight.TestConformanceOnly && preflight.MetadataOnly &&
		!preflight.ExecutableFormatVerified && !preflight.ExecutableSemanticsVerified &&
		!preflight.ProcessStartEnabled &&
		!preflight.ProductInvocationEnabled
}

type executableIdentityWire struct {
	ProtocolVersion             *string `json:"protocol_version"`
	Analyzer                    *string `json:"analyzer"`
	DescriptorSHA256            *string `json:"descriptor_sha256"`
	RequestProtocol             *string `json:"request_protocol"`
	ResultProtocol              *string `json:"result_protocol"`
	ErrorProtocol               *string `json:"error_protocol"`
	TargetGOOS                  *string `json:"target_goos"`
	TargetGOARCH                *string `json:"target_goarch"`
	ExecutableBytes             *int    `json:"executable_bytes"`
	ExecutableSHA256            *string `json:"executable_sha256"`
	DescriptorDeterministic     *bool   `json:"descriptor_deterministic"`
	MetadataOnly                *bool   `json:"metadata_only"`
	ExecutableBytesIncluded     *bool   `json:"executable_bytes_included"`
	PathIncluded                *bool   `json:"path_included"`
	CommandIncluded             *bool   `json:"command_included"`
	EnvironmentIncluded         *bool   `json:"environment_included"`
	ExecutableFormatVerified    *bool   `json:"executable_format_verified"`
	ExecutableSemanticsVerified *bool   `json:"executable_semantics_verified"`
	ProcessStartEnabled         *bool   `json:"process_start_enabled"`
	ProductInvocationEnabled    *bool   `json:"product_invocation_enabled"`
}

func (wire executableIdentityWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.Analyzer != nil &&
		wire.DescriptorSHA256 != nil && wire.RequestProtocol != nil &&
		wire.ResultProtocol != nil && wire.ErrorProtocol != nil && wire.TargetGOOS != nil &&
		wire.TargetGOARCH != nil && wire.ExecutableBytes != nil && wire.ExecutableSHA256 != nil &&
		wire.DescriptorDeterministic != nil && wire.MetadataOnly != nil &&
		wire.ExecutableBytesIncluded != nil && wire.PathIncluded != nil &&
		wire.CommandIncluded != nil && wire.EnvironmentIncluded != nil &&
		wire.ExecutableFormatVerified != nil && wire.ExecutableSemanticsVerified != nil &&
		wire.ProcessStartEnabled != nil &&
		wire.ProductInvocationEnabled != nil
}

func (wire executableIdentityWire) value() ExecutableIdentity {
	return ExecutableIdentity{
		ProtocolVersion: *wire.ProtocolVersion, Analyzer: *wire.Analyzer,
		DescriptorSHA256: *wire.DescriptorSHA256, RequestProtocol: *wire.RequestProtocol,
		ResultProtocol: *wire.ResultProtocol, ErrorProtocol: *wire.ErrorProtocol,
		TargetGOOS: *wire.TargetGOOS, TargetGOARCH: *wire.TargetGOARCH,
		ExecutableBytes: *wire.ExecutableBytes, ExecutableSHA256: *wire.ExecutableSHA256,
		DescriptorDeterministic: *wire.DescriptorDeterministic,
		MetadataOnly:            *wire.MetadataOnly,
		ExecutableBytesIncluded: *wire.ExecutableBytesIncluded, PathIncluded: *wire.PathIncluded,
		CommandIncluded: *wire.CommandIncluded, EnvironmentIncluded: *wire.EnvironmentIncluded,
		ExecutableFormatVerified:    *wire.ExecutableFormatVerified,
		ExecutableSemanticsVerified: *wire.ExecutableSemanticsVerified,
		ProcessStartEnabled:         *wire.ProcessStartEnabled,
		ProductInvocationEnabled:    *wire.ProductInvocationEnabled,
	}
}

type invocationPreflightWire struct {
	ProtocolVersion             *string                  `json:"protocol_version"`
	CandidateSHA256             *string                  `json:"candidate_sha256"`
	ExecutableIdentitySHA256    *string                  `json:"executable_identity_sha256"`
	RequestID                   *string                  `json:"request_id"`
	Analyzer                    *string                  `json:"analyzer"`
	RequestProtocol             *string                  `json:"request_protocol"`
	ResultProtocol              *string                  `json:"result_protocol"`
	ErrorProtocol               *string                  `json:"error_protocol"`
	TargetGOOS                  *string                  `json:"target_goos"`
	TargetGOARCH                *string                  `json:"target_goarch"`
	ExecutableBytes             *int                     `json:"executable_bytes"`
	ExecutableSHA256            *string                  `json:"executable_sha256"`
	Authority                   *invocationAuthorityWire `json:"authority"`
	CandidateValidated          *bool                    `json:"candidate_validated"`
	DescriptorBound             *bool                    `json:"descriptor_bound"`
	ProtocolsBound              *bool                    `json:"protocols_bound"`
	PlatformRecorded            *bool                    `json:"platform_recorded"`
	ExecutableBound             *bool                    `json:"executable_bound"`
	TestConformanceOnly         *bool                    `json:"test_conformance_only"`
	MetadataOnly                *bool                    `json:"metadata_only"`
	ExecutableFormatVerified    *bool                    `json:"executable_format_verified"`
	ExecutableSemanticsVerified *bool                    `json:"executable_semantics_verified"`
	ProcessStartEnabled         *bool                    `json:"process_start_enabled"`
	ProductInvocationEnabled    *bool                    `json:"product_invocation_enabled"`
}

func (wire invocationPreflightWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.CandidateSHA256 != nil &&
		wire.ExecutableIdentitySHA256 != nil && wire.RequestID != nil && wire.Analyzer != nil &&
		wire.RequestProtocol != nil && wire.ResultProtocol != nil && wire.ErrorProtocol != nil &&
		wire.TargetGOOS != nil && wire.TargetGOARCH != nil && wire.ExecutableBytes != nil &&
		wire.ExecutableSHA256 != nil && wire.Authority != nil && wire.Authority.complete() &&
		wire.CandidateValidated != nil && wire.DescriptorBound != nil &&
		wire.ProtocolsBound != nil && wire.PlatformRecorded != nil && wire.ExecutableBound != nil &&
		wire.TestConformanceOnly != nil && wire.MetadataOnly != nil &&
		wire.ExecutableFormatVerified != nil && wire.ExecutableSemanticsVerified != nil &&
		wire.ProcessStartEnabled != nil &&
		wire.ProductInvocationEnabled != nil
}

func (wire invocationPreflightWire) value() InvocationPreflight {
	return InvocationPreflight{
		ProtocolVersion: *wire.ProtocolVersion, CandidateSHA256: *wire.CandidateSHA256,
		ExecutableIdentitySHA256: *wire.ExecutableIdentitySHA256,
		RequestID:                *wire.RequestID, Analyzer: *wire.Analyzer,
		RequestProtocol: *wire.RequestProtocol, ResultProtocol: *wire.ResultProtocol,
		ErrorProtocol: *wire.ErrorProtocol, TargetGOOS: *wire.TargetGOOS,
		TargetGOARCH: *wire.TargetGOARCH, ExecutableBytes: *wire.ExecutableBytes,
		ExecutableSHA256: *wire.ExecutableSHA256, Authority: wire.Authority.value(),
		CandidateValidated: *wire.CandidateValidated, DescriptorBound: *wire.DescriptorBound,
		ProtocolsBound: *wire.ProtocolsBound, PlatformRecorded: *wire.PlatformRecorded,
		ExecutableBound: *wire.ExecutableBound, TestConformanceOnly: *wire.TestConformanceOnly,
		MetadataOnly:                *wire.MetadataOnly,
		ExecutableFormatVerified:    *wire.ExecutableFormatVerified,
		ExecutableSemanticsVerified: *wire.ExecutableSemanticsVerified,
		ProcessStartEnabled:         *wire.ProcessStartEnabled,
		ProductInvocationEnabled:    *wire.ProductInvocationEnabled,
	}
}
