package analyzer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinRegistryIsFixedSortedAndCloneIsolated(t *testing.T) {
	registry := BuiltinRegistry()
	descriptors := registry.List()
	if len(descriptors) != 2 || descriptors[0].Analyzer != ArchiveAnalyzerName ||
		descriptors[1].Analyzer != FixtureAnalyzerName {
		t.Fatalf("unexpected descriptor order: %#v", descriptors)
	}
	for _, descriptor := range descriptors {
		if !ValidateDescriptor(descriptor) || capabilitiesEnabled(descriptor.Capabilities) ||
			descriptor.Authority != (DescriptorAuthority{}) || !descriptor.Deterministic ||
			!descriptor.MetadataOnly {
			t.Fatalf("unsafe descriptor: %#v", descriptor)
		}
	}
	if !descriptors[0].CentralDirectoryOnly || descriptors[1].CentralDirectoryOnly {
		t.Fatalf("central-directory flags drifted: %#v", descriptors)
	}
	descriptors[0].AcceptedMediaTypes[0] = "text/plain"
	again, ok := registry.Lookup(ArchiveAnalyzerName)
	if !ok || !reflect.DeepEqual(again.AcceptedMediaTypes, []string{"application/zip"}) {
		t.Fatalf("registry leaked mutable descriptor state: %#v", again)
	}
	if _, ok := registry.Lookup("unknown.v1"); ok {
		t.Fatal("unknown descriptor resolved")
	}
}

func TestDescriptorStrictRoundTripAndFailClosedValidation(t *testing.T) {
	descriptor, _ := BuiltinRegistry().Lookup(ArchiveAnalyzerName)
	raw, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, code := DecodeDescriptor(raw)
	if code != "" || !reflect.DeepEqual(decoded, descriptor) {
		t.Fatalf("descriptor round trip failed: code=%s value=%#v", code, decoded)
	}
	assertExactObjectKeys(t, raw, []string{"accepted_media_types", "analyzer", "authority",
		"capabilities", "central_directory_only", "deterministic", "limits", "metadata_only",
		"protocol_version", "request_protocol", "result_protocol"})

	text := string(raw)
	for name, malformed := range map[string]string{
		"unknown": strings.Replace(text, `"metadata_only":true`,
			`"metadata_only":true,"executable":"analyzer.exe"`, 1),
		"duplicate": strings.Replace(text, `"process_start":false`,
			`"process_start":false,"process_start":false`, 1),
		"missing false": strings.Replace(text, `,"artifact_commit":false`, "", 1),
		"future":        strings.Replace(text, DescriptorProtocolVersion, "analyzer_descriptor.v2", 1),
		"authority": strings.Replace(text, `"product_invocation":false`,
			`"product_invocation":true`, 1),
	} {
		if _, code := DecodeDescriptor([]byte(malformed)); code != CodeInvalidResult {
			t.Fatalf("%s descriptor code = %s", name, code)
		}
	}
}

func TestRegistryMediaTypeContract(t *testing.T) {
	registry := BuiltinRegistry()
	fixture, _ := registry.Lookup(FixtureAnalyzerName)
	archive, _ := registry.Lookup(ArchiveAnalyzerName)
	if !descriptorAcceptsMediaType(fixture, "text/plain") ||
		!descriptorAcceptsMediaType(fixture, "application/octet-stream") ||
		!descriptorAcceptsMediaType(archive, "application/zip") ||
		descriptorAcceptsMediaType(archive, "text/plain") {
		t.Fatal("descriptor media-type boundary drifted")
	}
}
