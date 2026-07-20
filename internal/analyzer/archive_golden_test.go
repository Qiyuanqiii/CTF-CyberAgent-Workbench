package analyzer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
)

const sharedArchiveGoldenProtocol = "archive_inventory_golden_vectors.v1"

type sharedArchiveGoldenFile struct {
	ProtocolVersion string                      `json:"protocol_version"`
	Contract        sharedArchiveGoldenContract `json:"contract"`
	RiskCodes       []ArchiveRiskCode           `json:"risk_codes"`
	Vectors         []sharedGoldenVector        `json:"vectors"`
}

type sharedArchiveGoldenContract struct {
	DescriptorProtocol       string `json:"descriptor_protocol"`
	RequestProtocol          string `json:"request_protocol"`
	ResultProtocol           string `json:"result_protocol"`
	ErrorProtocol            string `json:"error_protocol"`
	Analyzer                 string `json:"analyzer"`
	MediaType                string `json:"media_type"`
	MaxDecodedInputBytes     int    `json:"max_decoded_input_bytes"`
	MaxResultEnvelopeBytes   int    `json:"max_result_envelope_bytes"`
	MaxEntries               int    `json:"max_entries"`
	MaxEntryNameBytes        int    `json:"max_entry_name_bytes"`
	MaxTotalNameBytes        int    `json:"max_total_name_bytes"`
	MaxDeclaredEntryBytes    uint64 `json:"max_declared_entry_bytes"`
	MaxDeclaredTotalBytes    uint64 `json:"max_declared_total_bytes"`
	MaxCompressionRatioMilli uint64 `json:"max_compression_ratio_milli"`
	MaxReportedRatioMilli    uint64 `json:"max_reported_ratio_milli"`
}

func TestArchiveInventorySharedGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile("../../analyzers/testdata/archive_inventory_v1_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var golden sharedArchiveGoldenFile
	if err := decoder.Decode(&golden); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("shared archive golden file contains trailing JSON: %v", err)
	}
	expectedContract := sharedArchiveGoldenContract{
		DescriptorProtocol: DescriptorProtocolVersion, RequestProtocol: RequestProtocolVersion,
		ResultProtocol: ArchiveInventoryProtocolVersion, ErrorProtocol: ErrorProtocolVersion,
		Analyzer: ArchiveAnalyzerName, MediaType: "application/zip",
		MaxDecodedInputBytes:   MaxDecodedInputBytes,
		MaxResultEnvelopeBytes: MaxResultEnvelopeBytes, MaxEntries: MaxArchiveEntries,
		MaxEntryNameBytes:        MaxArchiveEntryNameBytes,
		MaxTotalNameBytes:        MaxArchiveTotalNameBytes,
		MaxDeclaredEntryBytes:    MaxArchiveDeclaredEntryBytes,
		MaxDeclaredTotalBytes:    MaxArchiveDeclaredTotalBytes,
		MaxCompressionRatioMilli: MaxArchiveCompressionRatioMilli,
		MaxReportedRatioMilli:    MaxArchiveReportedRatioMilli,
	}
	expectedRisks := []ArchiveRiskCode{RiskAbsolutePath, RiskBackslashSeparator,
		RiskCompressionRatio, RiskDeclaredEntrySize, RiskDeclaredTotalSize,
		RiskDirectoryHasData, RiskDuplicateName, RiskParentTraversal}
	if golden.ProtocolVersion != sharedArchiveGoldenProtocol || golden.Contract != expectedContract ||
		!reflect.DeepEqual(golden.RiskCodes, expectedRisks) || len(golden.Vectors) != 5 {
		t.Fatalf("shared archive contract drifted: %#v", golden)
	}
	descriptor, ok := BuiltinRegistry().Lookup(ArchiveAnalyzerName)
	if !ok || descriptor.ResultProtocol != golden.Contract.ResultProtocol ||
		!reflect.DeepEqual(descriptor.AcceptedMediaTypes, []string{golden.Contract.MediaType}) {
		t.Fatalf("archive descriptor drifted: %#v", descriptor)
	}
	seen := make(map[string]struct{}, len(golden.Vectors))
	for _, vector := range golden.Vectors {
		if vector.Name == "" || vector.ExpectedExitCode != ExitSuccess ||
			vector.ExpectedStdoutBytes < 1 || len(vector.ExpectedStdoutSHA256) != 64 {
			t.Fatalf("shared archive vector is incomplete: %#v", vector)
		}
		if _, duplicate := seen[vector.Name]; duplicate {
			t.Fatalf("duplicate shared archive vector %q", vector.Name)
		}
		seen[vector.Name] = struct{}{}
		output, exitCode := Evaluate(vector.Request)
		digest := sha256.Sum256(output)
		actualDigest := hex.EncodeToString(digest[:])
		if exitCode != vector.ExpectedExitCode || len(output) != vector.ExpectedStdoutBytes ||
			actualDigest != vector.ExpectedStdoutSHA256 {
			t.Fatalf("shared archive vector %q drifted: exit=%d bytes=%d sha256=%s output=%s",
				vector.Name, exitCode, len(output), actualDigest, output)
		}
		var actualValue, expectedValue any
		if err := json.Unmarshal(output, &actualValue); err != nil {
			t.Fatalf("shared archive output %q is invalid: %v", vector.Name, err)
		}
		if err := json.Unmarshal(vector.ExpectedStdout, &expectedValue); err != nil {
			t.Fatalf("shared archive expectation %q is invalid: %v", vector.Name, err)
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Fatalf("shared archive semantic output %q drifted: %s", vector.Name, output)
		}
		if _, code := DecodeArchiveInventory(output); code != "" {
			t.Fatalf("shared archive result %q failed Go validation: %s", vector.Name, code)
		}
	}
}
