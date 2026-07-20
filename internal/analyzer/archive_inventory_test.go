package analyzer

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

var testArchiveTime = time.Date(2020, 1, 2, 3, 4, 6, 0, time.UTC)

func TestEvaluateZIPInventoryReturnsCentralDirectoryMetadataOnly(t *testing.T) {
	content := testZIP(t, []testZIPEntry{{name: "docs/", content: nil},
		{name: "docs/readme.txt", content: []byte("hello\n")}})
	request := testRequest()
	request.RequestID = "archive-benign"
	request.Analyzer = ArchiveAnalyzerName
	request.Input.MediaType = "application/zip"
	request.Input.ContentBase64 = base64.StdEncoding.EncodeToString(content)
	raw, _ := json.Marshal(request)
	output, exitCode := Evaluate(raw)
	if exitCode != ExitSuccess {
		t.Fatalf("exit=%d output=%s", exitCode, output)
	}
	inventory, code := DecodeArchiveInventory(output)
	if code != "" {
		t.Fatalf("inventory rejected: %s output=%s", code, output)
	}
	if inventory.EntryCount != 2 || inventory.RiskEntryCount != 0 ||
		len(inventory.RiskCodes) != 0 || inventory.EntryContentsRead ||
		inventory.ExtractionPerformed || !inventory.CentralDirectoryOnly ||
		inventory.Entries[0].Kind != "directory" || inventory.Entries[1].Name != "docs/readme.txt" {
		t.Fatalf("unexpected inventory: %#v", inventory)
	}
	if bytes.Contains(output, []byte("hello")) {
		t.Fatalf("archive content leaked into metadata result: %s", output)
	}
	if bytes.Contains(output, []byte(`"crc32"`)) {
		t.Fatalf("archive result overstated a declared checksum: %s", output)
	}
	assertExactObjectKeys(t, output, []string{"analyzer", "capabilities_used",
		"central_directory_only", "entries", "entry_contents_read", "entry_count",
		"extraction_performed", "format", "limits", "metadata_only", "protocol_version",
		"request_id", "risk_codes", "risk_entry_count", "status",
		"total_compressed_bytes", "total_uncompressed_bytes"})
}

func TestZIPInventoryClassifiesPathDuplicateSizeAndRatioRisks(t *testing.T) {
	entries := []ArchiveEntry{
		{Index: 0, Name: "../secret.txt", CompressedBytes: 4, UncompressedBytes: 4,
			DeclaredCRC32: "00000000"},
		{Index: 1, Name: `C:\temp\data.bin`, CompressedBytes: 1,
			UncompressedBytes: MaxArchiveDeclaredEntryBytes + 1, DeclaredCRC32: "00000000"},
		{Index: 2, Name: "../secret.txt", CompressedBytes: 0, UncompressedBytes: 0,
			DeclaredCRC32: "00000000"},
	}
	inventory, code := buildArchiveInventory("archive-risks", entries)
	if code != "" {
		t.Fatal(code)
	}
	expected := []ArchiveRiskCode{RiskAbsolutePath, RiskBackslashSeparator,
		RiskCompressionRatio, RiskDeclaredEntrySize, RiskDuplicateName, RiskParentTraversal}
	if fmt.Sprint(inventory.RiskCodes) != fmt.Sprint(expected) || inventory.RiskEntryCount != 3 {
		t.Fatalf("unexpected risks: %#v", inventory)
	}
}

func TestZIPInventoryHardBoundsAndMalformedArchiveFailClosed(t *testing.T) {
	request := testRequest()
	request.Analyzer = ArchiveAnalyzerName
	request.Input.MediaType = "application/zip"
	for name, content := range map[string][]byte{
		"malformed": []byte("not a zip"),
		"too many":  testZIPCount(t, MaxArchiveEntries+1),
		"long name": testZIP(t, []testZIPEntry{{name: strings.Repeat("a", MaxArchiveEntryNameBytes+1)}}),
	} {
		request.RequestID = strings.ReplaceAll(name, " ", "-")
		request.Input.ContentBase64 = base64.StdEncoding.EncodeToString(content)
		raw, _ := json.Marshal(request)
		output, exitCode := Evaluate(raw)
		if exitCode != ExitRejected {
			t.Fatalf("%s exit=%d output=%s", name, exitCode, output)
		}
		envelope, code := DecodeError(output)
		if code != "" {
			t.Fatalf("%s error envelope invalid: %s", name, code)
		}
		expected := CodeInputLimitExceeded
		if name == "malformed" {
			expected = CodeInvalidContent
		}
		if envelope.Code != expected {
			t.Fatalf("%s code=%s want=%s", name, envelope.Code, expected)
		}
	}
}

func TestDecodeZIPInventoryRejectsMissingFalseAndDerivedDrift(t *testing.T) {
	inventory, code := buildArchiveInventory("archive-strict", []ArchiveEntry{{
		Index: 0, Name: "safe.txt", CompressedBytes: 4, UncompressedBytes: 4,
		DeclaredCRC32: "00000000",
	}})
	if code != "" {
		t.Fatal(code)
	}
	raw, _ := json.Marshal(inventory)
	for name, malformed := range map[string][]byte{
		"missing false":      bytes.Replace(raw, []byte(`,"entry_contents_read":false`), nil, 1),
		"enabled capability": bytes.Replace(raw, []byte(`"network":false`), []byte(`"network":true`), 1),
		"ratio drift": bytes.Replace(raw, []byte(`"compression_ratio_milli":1000`),
			[]byte(`"compression_ratio_milli":1001`), 1),
		"unverified crc alias": bytes.Replace(raw, []byte(`"declared_crc32"`),
			[]byte(`"crc32"`), 1),
		"risk drift": bytes.Replace(raw, []byte(`"risk_codes":[]`),
			[]byte(`"risk_codes":["absolute_path"]`), 1),
	} {
		if _, code := DecodeArchiveInventory(malformed); code != CodeInvalidResult {
			t.Fatalf("%s code=%s json=%s", name, code, malformed)
		}
	}
}

func TestZIPInventorySaturatesHostileSizeMetadata(t *testing.T) {
	entries := []ArchiveEntry{
		{Index: 0, Name: "first.bin", CompressedBytes: 1,
			UncompressedBytes: ^uint64(0), DeclaredCRC32: "00000000"},
		{Index: 1, Name: "second.bin", CompressedBytes: ^uint64(0),
			UncompressedBytes: 1, DeclaredCRC32: "00000000"},
	}
	inventory, code := buildArchiveInventory("archive-overflow", entries)
	if code != "" {
		t.Fatal(code)
	}
	if inventory.TotalCompressedBytes != ^uint64(0) ||
		inventory.TotalUncompressedBytes != ^uint64(0) ||
		archiveRatioMilli(^uint64(0), 1) != MaxArchiveReportedRatioMilli ||
		!containsArchiveRisk(inventory.RiskCodes, RiskDeclaredTotalSize) {
		t.Fatalf("hostile size metadata was not saturated: %#v", inventory)
	}
}

func containsArchiveRisk(values []ArchiveRiskCode, expected ArchiveRiskCode) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type testZIPEntry struct {
	name    string
	content []byte
}

func testZIP(t testing.TB, entries []testZIPEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.Modified = testArchiveTime
		output, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := output.Write(entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func testZIPCount(t testing.TB, count int) []byte {
	t.Helper()
	entries := make([]testZIPEntry, count)
	for index := range entries {
		entries[index].name = fmt.Sprintf("entry-%02d.txt", index)
	}
	return testZIP(t, entries)
}
