package analyzer

import (
	"reflect"
	"sort"
)

const (
	DescriptorProtocolVersion       = "analyzer_descriptor.v1"
	ArchiveInventoryProtocolVersion = "archive.inventory.v1"
	ArchiveAnalyzerName             = "archive.zip.inventory.v1"
)

type DescriptorLimits struct {
	MaxInputBytes       int `json:"max_input_bytes"`
	MaxOutputBytes      int `json:"max_output_bytes"`
	TimeoutMilliseconds int `json:"timeout_ms"`
}

type DescriptorAuthority struct {
	ProductInvocation bool `json:"product_invocation"`
	ProcessStart      bool `json:"process_start"`
	FileInput         bool `json:"file_input"`
	ArtifactCommit    bool `json:"artifact_commit"`
}

// Descriptor is inert metadata. It deliberately has no executable, command, or path field.
type Descriptor struct {
	ProtocolVersion      string              `json:"protocol_version"`
	Analyzer             string              `json:"analyzer"`
	RequestProtocol      string              `json:"request_protocol"`
	ResultProtocol       string              `json:"result_protocol"`
	AcceptedMediaTypes   []string            `json:"accepted_media_types"`
	Limits               DescriptorLimits    `json:"limits"`
	Capabilities         Capabilities        `json:"capabilities"`
	Authority            DescriptorAuthority `json:"authority"`
	Deterministic        bool                `json:"deterministic"`
	MetadataOnly         bool                `json:"metadata_only"`
	CentralDirectoryOnly bool                `json:"central_directory_only"`
}

// Registry is a fixed catalog of Go-owned descriptors. It exposes no registration API.
type Registry struct {
	descriptors map[string]Descriptor
}

func BuiltinRegistry() Registry {
	fixture := Descriptor{
		ProtocolVersion: DescriptorProtocolVersion, Analyzer: FixtureAnalyzerName,
		RequestProtocol: RequestProtocolVersion, ResultProtocol: ResultProtocolVersion,
		AcceptedMediaTypes: []string{"*/*"}, Limits: builtinDescriptorLimits(),
		Deterministic: true, MetadataOnly: true,
	}
	archive := Descriptor{
		ProtocolVersion: DescriptorProtocolVersion, Analyzer: ArchiveAnalyzerName,
		RequestProtocol: RequestProtocolVersion, ResultProtocol: ArchiveInventoryProtocolVersion,
		AcceptedMediaTypes: []string{"application/zip"}, Limits: builtinDescriptorLimits(),
		Deterministic: true, MetadataOnly: true, CentralDirectoryOnly: true,
	}
	return Registry{descriptors: map[string]Descriptor{
		fixture.Analyzer: fixture,
		archive.Analyzer: archive,
	}}
}

func (registry Registry) Lookup(analyzer string) (Descriptor, bool) {
	descriptor, ok := registry.descriptors[analyzer]
	return cloneDescriptor(descriptor), ok
}

func (registry Registry) List() []Descriptor {
	result := make([]Descriptor, 0, len(registry.descriptors))
	for _, descriptor := range registry.descriptors {
		result = append(result, cloneDescriptor(descriptor))
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Analyzer < result[right].Analyzer
	})
	return result
}

func DecodeDescriptor(raw []byte) (Descriptor, ErrorCode) {
	var wire descriptorWire
	if !strictDecode(raw, MaxResultEnvelopeBytes, &wire) || !wire.complete() {
		return Descriptor{}, CodeInvalidResult
	}
	descriptor := wire.value()
	if !ValidateDescriptor(descriptor) {
		return Descriptor{}, CodeInvalidResult
	}
	return descriptor, ""
}

func ValidateDescriptor(descriptor Descriptor) bool {
	expected, ok := BuiltinRegistry().Lookup(descriptor.Analyzer)
	return ok && reflect.DeepEqual(descriptor, expected)
}

func builtinDescriptorLimits() DescriptorLimits {
	return DescriptorLimits{
		MaxInputBytes: MaxDecodedInputBytes, MaxOutputBytes: MaxResultEnvelopeBytes,
		TimeoutMilliseconds: MaxTimeoutMilliseconds,
	}
}

func cloneDescriptor(descriptor Descriptor) Descriptor {
	descriptor.AcceptedMediaTypes = append([]string(nil), descriptor.AcceptedMediaTypes...)
	return descriptor
}

func descriptorAcceptsMediaType(descriptor Descriptor, mediaType string) bool {
	for _, accepted := range descriptor.AcceptedMediaTypes {
		if accepted == "*/*" || accepted == mediaType {
			return true
		}
	}
	return false
}

type descriptorWire struct {
	ProtocolVersion      *string                  `json:"protocol_version"`
	Analyzer             *string                  `json:"analyzer"`
	RequestProtocol      *string                  `json:"request_protocol"`
	ResultProtocol       *string                  `json:"result_protocol"`
	AcceptedMediaTypes   *[]string                `json:"accepted_media_types"`
	Limits               *descriptorLimitsWire    `json:"limits"`
	Capabilities         *capabilitiesWire        `json:"capabilities"`
	Authority            *descriptorAuthorityWire `json:"authority"`
	Deterministic        *bool                    `json:"deterministic"`
	MetadataOnly         *bool                    `json:"metadata_only"`
	CentralDirectoryOnly *bool                    `json:"central_directory_only"`
}

type descriptorLimitsWire struct {
	MaxInputBytes       *int `json:"max_input_bytes"`
	MaxOutputBytes      *int `json:"max_output_bytes"`
	TimeoutMilliseconds *int `json:"timeout_ms"`
}

type descriptorAuthorityWire struct {
	ProductInvocation *bool `json:"product_invocation"`
	ProcessStart      *bool `json:"process_start"`
	FileInput         *bool `json:"file_input"`
	ArtifactCommit    *bool `json:"artifact_commit"`
}

func (wire descriptorWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.Analyzer != nil && wire.RequestProtocol != nil &&
		wire.ResultProtocol != nil && wire.AcceptedMediaTypes != nil && wire.Limits != nil &&
		wire.Limits.complete() && wire.Capabilities != nil && wire.Capabilities.complete() &&
		wire.Authority != nil && wire.Authority.complete() && wire.Deterministic != nil &&
		wire.MetadataOnly != nil && wire.CentralDirectoryOnly != nil
}

func (wire descriptorWire) value() Descriptor {
	return Descriptor{
		ProtocolVersion: *wire.ProtocolVersion, Analyzer: *wire.Analyzer,
		RequestProtocol: *wire.RequestProtocol, ResultProtocol: *wire.ResultProtocol,
		AcceptedMediaTypes: append([]string(nil), (*wire.AcceptedMediaTypes)...),
		Limits: DescriptorLimits{MaxInputBytes: *wire.Limits.MaxInputBytes,
			MaxOutputBytes:      *wire.Limits.MaxOutputBytes,
			TimeoutMilliseconds: *wire.Limits.TimeoutMilliseconds},
		Capabilities: wire.Capabilities.value(),
		Authority: DescriptorAuthority{ProductInvocation: *wire.Authority.ProductInvocation,
			ProcessStart: *wire.Authority.ProcessStart, FileInput: *wire.Authority.FileInput,
			ArtifactCommit: *wire.Authority.ArtifactCommit},
		Deterministic: *wire.Deterministic, MetadataOnly: *wire.MetadataOnly,
		CentralDirectoryOnly: *wire.CentralDirectoryOnly,
	}
}

func (wire descriptorLimitsWire) complete() bool {
	return wire.MaxInputBytes != nil && wire.MaxOutputBytes != nil &&
		wire.TimeoutMilliseconds != nil
}

func (wire descriptorAuthorityWire) complete() bool {
	return wire.ProductInvocation != nil && wire.ProcessStart != nil &&
		wire.FileInput != nil && wire.ArtifactCommit != nil
}
