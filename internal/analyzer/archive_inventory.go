package analyzer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	MaxArchiveEntries                      = 32
	MaxArchiveEntryNameBytes               = 128
	MaxArchiveTotalNameBytes               = 2 * 1024
	MaxArchiveDeclaredEntryBytes    uint64 = 8 * 1024 * 1024
	MaxArchiveDeclaredTotalBytes    uint64 = 32 * 1024 * 1024
	MaxArchiveCompressionRatioMilli        = 100_000
	MaxArchiveReportedRatioMilli    uint64 = 1_000_000_000
)

type ArchiveRiskCode string

const (
	RiskAbsolutePath       ArchiveRiskCode = "absolute_path"
	RiskBackslashSeparator ArchiveRiskCode = "backslash_separator"
	RiskCompressionRatio   ArchiveRiskCode = "compression_ratio"
	RiskDeclaredEntrySize  ArchiveRiskCode = "declared_entry_size"
	RiskDeclaredTotalSize  ArchiveRiskCode = "declared_total_size"
	RiskDirectoryHasData   ArchiveRiskCode = "directory_has_data"
	RiskDuplicateName      ArchiveRiskCode = "duplicate_name"
	RiskParentTraversal    ArchiveRiskCode = "parent_traversal"
)

type ArchiveInventoryLimits struct {
	MaxEntries               int    `json:"max_entries"`
	MaxEntryNameBytes        int    `json:"max_entry_name_bytes"`
	MaxTotalNameBytes        int    `json:"max_total_name_bytes"`
	MaxDeclaredEntryBytes    uint64 `json:"max_declared_entry_bytes"`
	MaxDeclaredTotalBytes    uint64 `json:"max_declared_total_bytes"`
	MaxCompressionRatioMilli uint64 `json:"max_compression_ratio_milli"`
	MaxReportedRatioMilli    uint64 `json:"max_reported_ratio_milli"`
}

type ArchiveEntry struct {
	Index                 int               `json:"index"`
	Name                  string            `json:"name"`
	Kind                  string            `json:"kind"`
	CompressedBytes       uint64            `json:"compressed_bytes"`
	UncompressedBytes     uint64            `json:"uncompressed_bytes"`
	CompressionRatioMilli uint64            `json:"compression_ratio_milli"`
	DeclaredCRC32         string            `json:"declared_crc32"`
	RiskCodes             []ArchiveRiskCode `json:"risk_codes"`
}

type ArchiveInventory struct {
	ProtocolVersion        string                 `json:"protocol_version"`
	RequestID              string                 `json:"request_id"`
	Analyzer               string                 `json:"analyzer"`
	Status                 string                 `json:"status"`
	Format                 string                 `json:"format"`
	EntryCount             int                    `json:"entry_count"`
	TotalCompressedBytes   uint64                 `json:"total_compressed_bytes"`
	TotalUncompressedBytes uint64                 `json:"total_uncompressed_bytes"`
	Limits                 ArchiveInventoryLimits `json:"limits"`
	Entries                []ArchiveEntry         `json:"entries"`
	RiskEntryCount         int                    `json:"risk_entry_count"`
	RiskCodes              []ArchiveRiskCode      `json:"risk_codes"`
	MetadataOnly           bool                   `json:"metadata_only"`
	CentralDirectoryOnly   bool                   `json:"central_directory_only"`
	EntryContentsRead      bool                   `json:"entry_contents_read"`
	ExtractionPerformed    bool                   `json:"extraction_performed"`
	CapabilitiesUsed       Capabilities           `json:"capabilities_used"`
}

func evaluateArchiveRequest(request Request, content []byte) ([]byte, int) {
	inventory, code := inventoryZIP(request.RequestID, content)
	if code != "" {
		return encodeError(request.RequestID, code), ExitRejected
	}
	encoded, err := json.Marshal(inventory)
	if err != nil {
		return encodeError(request.RequestID, CodeInternal), ExitRejected
	}
	if len(encoded) > request.Limits.MaxOutputBytes || len(encoded) > MaxResultEnvelopeBytes {
		return encodeError(request.RequestID, CodeOutputLimitExceeded), ExitRejected
	}
	return encoded, ExitSuccess
}

func inventoryZIP(requestID string, content []byte) (ArchiveInventory, ErrorCode) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return ArchiveInventory{}, CodeInvalidContent
	}
	if len(reader.File) > MaxArchiveEntries {
		return ArchiveInventory{}, CodeInputLimitExceeded
	}
	entries := make([]ArchiveEntry, 0, len(reader.File))
	for index, file := range reader.File {
		entries = append(entries, ArchiveEntry{
			Index: index, Name: file.Name, Kind: archiveEntryKind(file.Name),
			CompressedBytes: file.CompressedSize64, UncompressedBytes: file.UncompressedSize64,
			CompressionRatioMilli: archiveRatioMilli(file.UncompressedSize64, file.CompressedSize64),
			DeclaredCRC32:         fmt.Sprintf("%08x", file.CRC32), RiskCodes: make([]ArchiveRiskCode, 0),
		})
	}
	return buildArchiveInventory(requestID, entries)
}

func buildArchiveInventory(requestID string, entries []ArchiveEntry) (ArchiveInventory, ErrorCode) {
	if len(entries) > MaxArchiveEntries {
		return ArchiveInventory{}, CodeInputLimitExceeded
	}
	seenNames := make(map[string]struct{}, len(entries))
	aggregateRisks := make(map[ArchiveRiskCode]struct{})
	totalNames := 0
	var totalCompressed, totalUncompressed uint64
	riskEntryCount := 0
	for index := range entries {
		entry := &entries[index]
		if entry.Index != index {
			return ArchiveInventory{}, CodeInputLimitExceeded
		}
		if !validArchiveEntryName(entry.Name) {
			return ArchiveInventory{}, CodeInvalidContent
		}
		if len(entry.Name) > MaxArchiveEntryNameBytes {
			return ArchiveInventory{}, CodeInputLimitExceeded
		}
		totalNames += len(entry.Name)
		if totalNames > MaxArchiveTotalNameBytes {
			return ArchiveInventory{}, CodeInputLimitExceeded
		}
		entry.Kind = archiveEntryKind(entry.Name)
		entry.CompressionRatioMilli = archiveRatioMilli(entry.UncompressedBytes, entry.CompressedBytes)
		if !validCRC32(entry.DeclaredCRC32) {
			return ArchiveInventory{}, CodeInvalidContent
		}
		entry.RiskCodes = archiveEntryRisks(*entry, seenNames)
		if len(entry.RiskCodes) > 0 {
			riskEntryCount++
		}
		for _, risk := range entry.RiskCodes {
			aggregateRisks[risk] = struct{}{}
		}
		seenNames[entry.Name] = struct{}{}
		totalCompressed = saturatingAdd(totalCompressed, entry.CompressedBytes)
		totalUncompressed = saturatingAdd(totalUncompressed, entry.UncompressedBytes)
	}
	if totalUncompressed > MaxArchiveDeclaredTotalBytes {
		aggregateRisks[RiskDeclaredTotalSize] = struct{}{}
	}
	return ArchiveInventory{
		ProtocolVersion: ArchiveInventoryProtocolVersion, RequestID: requestID,
		Analyzer: ArchiveAnalyzerName, Status: "succeeded", Format: "zip",
		EntryCount: len(entries), TotalCompressedBytes: totalCompressed,
		TotalUncompressedBytes: totalUncompressed, Limits: archiveInventoryLimits(),
		Entries: entries, RiskEntryCount: riskEntryCount,
		RiskCodes: sortedArchiveRisks(aggregateRisks), MetadataOnly: true,
		CentralDirectoryOnly: true, EntryContentsRead: false, ExtractionPerformed: false,
	}, ""
}

func DecodeArchiveInventory(raw []byte) (ArchiveInventory, ErrorCode) {
	var wire archiveInventoryWire
	if !strictDecode(raw, MaxResultEnvelopeBytes, &wire) {
		if len(raw) > MaxResultEnvelopeBytes {
			return ArchiveInventory{}, CodeResultTooLarge
		}
		return ArchiveInventory{}, CodeInvalidResult
	}
	if !wire.complete() {
		return ArchiveInventory{}, CodeInvalidResult
	}
	value := wire.value()
	if value.ProtocolVersion != ArchiveInventoryProtocolVersion ||
		!validRequestID(value.RequestID) || value.Analyzer != ArchiveAnalyzerName ||
		value.Status != "succeeded" || value.Format != "zip" || !value.MetadataOnly ||
		!value.CentralDirectoryOnly || value.EntryContentsRead || value.ExtractionPerformed ||
		capabilitiesEnabled(value.CapabilitiesUsed) ||
		value.Limits != archiveInventoryLimits() || value.EntryCount != len(value.Entries) {
		return ArchiveInventory{}, CodeInvalidResult
	}
	expected, code := buildArchiveInventory(value.RequestID, append([]ArchiveEntry(nil), value.Entries...))
	if code != "" || !reflect.DeepEqual(value, expected) {
		return ArchiveInventory{}, CodeInvalidResult
	}
	return value, ""
}

func archiveInventoryLimits() ArchiveInventoryLimits {
	return ArchiveInventoryLimits{
		MaxEntries: MaxArchiveEntries, MaxEntryNameBytes: MaxArchiveEntryNameBytes,
		MaxTotalNameBytes:        MaxArchiveTotalNameBytes,
		MaxDeclaredEntryBytes:    MaxArchiveDeclaredEntryBytes,
		MaxDeclaredTotalBytes:    MaxArchiveDeclaredTotalBytes,
		MaxCompressionRatioMilli: MaxArchiveCompressionRatioMilli,
		MaxReportedRatioMilli:    MaxArchiveReportedRatioMilli,
	}
}

func archiveEntryRisks(entry ArchiveEntry, seen map[string]struct{}) []ArchiveRiskCode {
	risks := make(map[ArchiveRiskCode]struct{})
	if archiveAbsolutePath(entry.Name) {
		risks[RiskAbsolutePath] = struct{}{}
	}
	if strings.Contains(entry.Name, `\`) {
		risks[RiskBackslashSeparator] = struct{}{}
	}
	if archiveParentTraversal(entry.Name) {
		risks[RiskParentTraversal] = struct{}{}
	}
	if _, duplicate := seen[entry.Name]; duplicate {
		risks[RiskDuplicateName] = struct{}{}
	}
	if entry.UncompressedBytes > MaxArchiveDeclaredEntryBytes {
		risks[RiskDeclaredEntrySize] = struct{}{}
	}
	if entry.CompressionRatioMilli > MaxArchiveCompressionRatioMilli {
		risks[RiskCompressionRatio] = struct{}{}
	}
	if entry.Kind == "directory" && (entry.CompressedBytes != 0 || entry.UncompressedBytes != 0) {
		risks[RiskDirectoryHasData] = struct{}{}
	}
	return sortedArchiveRisks(risks)
}

func archiveEntryKind(name string) string {
	if strings.HasSuffix(name, "/") || strings.HasSuffix(name, `\`) {
		return "directory"
	}
	return "file"
}

func validArchiveEntryName(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	for _, value := range []byte(name) {
		if value < 0x20 || value == 0x7f {
			return false
		}
	}
	return true
}

func archiveAbsolutePath(name string) bool {
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return true
	}
	return len(name) >= 2 && ((name[0] >= 'a' && name[0] <= 'z') ||
		(name[0] >= 'A' && name[0] <= 'Z')) && name[1] == ':'
}

func archiveParentTraversal(name string) bool {
	for _, part := range strings.FieldsFunc(name, func(value rune) bool {
		return value == '/' || value == '\\'
	}) {
		if part == ".." {
			return true
		}
	}
	return false
}

func archiveRatioMilli(uncompressed, compressed uint64) uint64 {
	if uncompressed == 0 {
		return 0
	}
	if compressed == 0 {
		return MaxArchiveReportedRatioMilli
	}
	whole, remainder := uncompressed/compressed, uncompressed%compressed
	if whole >= MaxArchiveReportedRatioMilli/1000 {
		return MaxArchiveReportedRatioMilli
	}
	high, low := bits.Mul64(remainder, 1000)
	fraction, _ := bits.Div64(high, low, compressed)
	return whole*1000 + fraction
}

func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

func sortedArchiveRisks(values map[ArchiveRiskCode]struct{}) []ArchiveRiskCode {
	result := make([]ArchiveRiskCode, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func validCRC32(value string) bool {
	if len(value) != 8 || strings.ToLower(value) != value {
		return false
	}
	for _, char := range []byte(value) {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

type archiveInventoryWire struct {
	ProtocolVersion        *string                     `json:"protocol_version"`
	RequestID              *string                     `json:"request_id"`
	Analyzer               *string                     `json:"analyzer"`
	Status                 *string                     `json:"status"`
	Format                 *string                     `json:"format"`
	EntryCount             *int                        `json:"entry_count"`
	TotalCompressedBytes   *uint64                     `json:"total_compressed_bytes"`
	TotalUncompressedBytes *uint64                     `json:"total_uncompressed_bytes"`
	Limits                 *archiveInventoryLimitsWire `json:"limits"`
	Entries                *[]archiveEntryWire         `json:"entries"`
	RiskEntryCount         *int                        `json:"risk_entry_count"`
	RiskCodes              *[]ArchiveRiskCode          `json:"risk_codes"`
	MetadataOnly           *bool                       `json:"metadata_only"`
	CentralDirectoryOnly   *bool                       `json:"central_directory_only"`
	EntryContentsRead      *bool                       `json:"entry_contents_read"`
	ExtractionPerformed    *bool                       `json:"extraction_performed"`
	CapabilitiesUsed       *capabilitiesWire           `json:"capabilities_used"`
}

type archiveInventoryLimitsWire struct {
	MaxEntries               *int    `json:"max_entries"`
	MaxEntryNameBytes        *int    `json:"max_entry_name_bytes"`
	MaxTotalNameBytes        *int    `json:"max_total_name_bytes"`
	MaxDeclaredEntryBytes    *uint64 `json:"max_declared_entry_bytes"`
	MaxDeclaredTotalBytes    *uint64 `json:"max_declared_total_bytes"`
	MaxCompressionRatioMilli *uint64 `json:"max_compression_ratio_milli"`
	MaxReportedRatioMilli    *uint64 `json:"max_reported_ratio_milli"`
}

type archiveEntryWire struct {
	Index                 *int               `json:"index"`
	Name                  *string            `json:"name"`
	Kind                  *string            `json:"kind"`
	CompressedBytes       *uint64            `json:"compressed_bytes"`
	UncompressedBytes     *uint64            `json:"uncompressed_bytes"`
	CompressionRatioMilli *uint64            `json:"compression_ratio_milli"`
	DeclaredCRC32         *string            `json:"declared_crc32"`
	RiskCodes             *[]ArchiveRiskCode `json:"risk_codes"`
}

func (wire archiveInventoryWire) complete() bool {
	if wire.ProtocolVersion == nil || wire.RequestID == nil || wire.Analyzer == nil ||
		wire.Status == nil || wire.Format == nil || wire.EntryCount == nil ||
		wire.TotalCompressedBytes == nil || wire.TotalUncompressedBytes == nil ||
		wire.Limits == nil || !wire.Limits.complete() || wire.Entries == nil ||
		wire.RiskEntryCount == nil || wire.RiskCodes == nil || wire.MetadataOnly == nil ||
		wire.CentralDirectoryOnly == nil || wire.EntryContentsRead == nil ||
		wire.ExtractionPerformed == nil || wire.CapabilitiesUsed == nil ||
		!wire.CapabilitiesUsed.complete() {
		return false
	}
	for _, entry := range *wire.Entries {
		if !entry.complete() {
			return false
		}
	}
	return true
}

func (wire archiveInventoryWire) value() ArchiveInventory {
	entries := make([]ArchiveEntry, len(*wire.Entries))
	for index, entry := range *wire.Entries {
		entries[index] = entry.value()
	}
	return ArchiveInventory{
		ProtocolVersion: *wire.ProtocolVersion, RequestID: *wire.RequestID,
		Analyzer: *wire.Analyzer, Status: *wire.Status, Format: *wire.Format,
		EntryCount: *wire.EntryCount, TotalCompressedBytes: *wire.TotalCompressedBytes,
		TotalUncompressedBytes: *wire.TotalUncompressedBytes,
		Limits:                 wire.Limits.value(), Entries: entries, RiskEntryCount: *wire.RiskEntryCount,
		RiskCodes:    copyArchiveRisks(*wire.RiskCodes),
		MetadataOnly: *wire.MetadataOnly, CentralDirectoryOnly: *wire.CentralDirectoryOnly,
		EntryContentsRead:   *wire.EntryContentsRead,
		ExtractionPerformed: *wire.ExtractionPerformed,
		CapabilitiesUsed:    wire.CapabilitiesUsed.value(),
	}
}

func (wire archiveInventoryLimitsWire) complete() bool {
	return wire.MaxEntries != nil && wire.MaxEntryNameBytes != nil &&
		wire.MaxTotalNameBytes != nil && wire.MaxDeclaredEntryBytes != nil &&
		wire.MaxDeclaredTotalBytes != nil && wire.MaxCompressionRatioMilli != nil &&
		wire.MaxReportedRatioMilli != nil
}

func (wire archiveInventoryLimitsWire) value() ArchiveInventoryLimits {
	return ArchiveInventoryLimits{
		MaxEntries: *wire.MaxEntries, MaxEntryNameBytes: *wire.MaxEntryNameBytes,
		MaxTotalNameBytes:        *wire.MaxTotalNameBytes,
		MaxDeclaredEntryBytes:    *wire.MaxDeclaredEntryBytes,
		MaxDeclaredTotalBytes:    *wire.MaxDeclaredTotalBytes,
		MaxCompressionRatioMilli: *wire.MaxCompressionRatioMilli,
		MaxReportedRatioMilli:    *wire.MaxReportedRatioMilli,
	}
}

func (wire archiveEntryWire) complete() bool {
	return wire.Index != nil && wire.Name != nil && wire.Kind != nil &&
		wire.CompressedBytes != nil && wire.UncompressedBytes != nil &&
		wire.CompressionRatioMilli != nil && wire.DeclaredCRC32 != nil && wire.RiskCodes != nil
}

func (wire archiveEntryWire) value() ArchiveEntry {
	return ArchiveEntry{
		Index: *wire.Index, Name: *wire.Name, Kind: *wire.Kind,
		CompressedBytes: *wire.CompressedBytes, UncompressedBytes: *wire.UncompressedBytes,
		CompressionRatioMilli: *wire.CompressionRatioMilli, DeclaredCRC32: *wire.DeclaredCRC32,
		RiskCodes: copyArchiveRisks(*wire.RiskCodes),
	}
}

func copyArchiveRisks(values []ArchiveRiskCode) []ArchiveRiskCode {
	result := make([]ArchiveRiskCode, len(values))
	copy(result, values)
	return result
}
