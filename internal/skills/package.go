package skills

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"slices"
)

const (
	PackageProtocolVersion      = "skill_package.v1"
	PackageManifestPath         = "manifest.json"
	PackageContentPath          = "SKILL.md"
	MaxPackageArchiveBytes      = 64 * 1024
	MaxPackageUncompressedBytes = MaxManifestBytes + MaxContentBytes
	MaxPackageCompressionRatio  = 128
	PackageEntryCount           = 2
)

type PackageTrustClass string

const PackageTrustOperatorInstalledUntrusted PackageTrustClass = "operator_installed_untrusted"

type PackageRiskCode string

const (
	PackageRiskUntrustedInstructions PackageRiskCode = "untrusted_instructions"
	PackageRiskDeclaredToolsOnly     PackageRiskCode = "declared_tools_not_capabilities"
)

// PackagePreview contains bounded metadata only. It deliberately omits the
// Skill body and the source path supplied by the operator.
type PackagePreview struct {
	ProtocolVersion        string
	Manifest               Manifest
	ArchiveSHA256          string
	PackageFingerprint     string
	ArchiveBytes           int
	UncompressedBytes      int
	EntryCount             int
	TrustClass             PackageTrustClass
	RiskCodes              []PackageRiskCode
	ExecutableAssetCount   int
	InstallHookCount       int
	ImportCommandExecution bool
	ImportNetworkAccess    bool
	ImportProviderCalls    bool
	ToolCapabilityGrant    bool
	InstallationAuthorized bool
}

// SkillPackage is an immutable, validated in-memory package. Parsing never
// writes files, executes commands, opens the network, or calls a Provider.
type SkillPackage struct {
	preview PackagePreview
	content []byte
}

func (p *SkillPackage) Preview() PackagePreview {
	if p == nil {
		return PackagePreview{}
	}
	preview := p.preview
	preview.Manifest = cloneManifest(preview.Manifest)
	preview.RiskCodes = slices.Clone(preview.RiskCodes)
	return preview
}

func (p *SkillPackage) Manifest() Manifest {
	if p == nil {
		return Manifest{}
	}
	return cloneManifest(p.preview.Manifest)
}

// contentBytes returns a defensive copy for the trusted package delivery
// path. It remains unexported so callers cannot bypass PackageObjectLoader's
// object identity and descriptor checks.
func (p *SkillPackage) contentBytes() []byte {
	if p == nil {
		return nil
	}
	return bytes.Clone(p.content)
}

// ParsePackage validates the complete deterministic ZIP container before it
// decodes the Skill manifest or body.
func ParsePackage(raw []byte) (*SkillPackage, error) {
	if len(raw) == 0 || len(raw) > MaxPackageArchiveBytes {
		return nil, fmt.Errorf("invalid skill package: archive must contain between 1 and %d bytes", MaxPackageArchiveBytes)
	}
	records, uncompressedBytes, err := inspectPackageContainer(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid skill package: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("invalid skill package: open ZIP: %w", err)
	}
	if len(reader.File) != PackageEntryCount {
		return nil, fmt.Errorf("invalid skill package: ZIP contains %d entries, want %d", len(reader.File), PackageEntryCount)
	}

	manifestRaw, err := readPackageEntry(reader.File[0], records[0], MaxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid skill package: %w", err)
	}
	content, err := readPackageEntry(reader.File[1], records[1], MaxContentBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid skill package: %w", err)
	}
	manifest, err := decodeManifest(manifestRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid skill package: manifest: %w", err)
	}
	if manifest.ContentPath != PackageContentPath {
		return nil, fmt.Errorf("invalid skill package: manifest content_path must be %q", PackageContentPath)
	}
	if err := manifest.Validate(content); err != nil {
		return nil, fmt.Errorf("invalid skill package: manifest: %w", err)
	}

	fingerprint, err := packageFingerprint(manifest, content)
	if err != nil {
		return nil, fmt.Errorf("invalid skill package: canonicalize manifest: %w", err)
	}
	archiveDigest := sha256.Sum256(raw)
	return &SkillPackage{
		preview: PackagePreview{
			ProtocolVersion:    PackageProtocolVersion,
			Manifest:           cloneManifest(manifest),
			ArchiveSHA256:      hex.EncodeToString(archiveDigest[:]),
			PackageFingerprint: fingerprint,
			ArchiveBytes:       len(raw),
			UncompressedBytes:  uncompressedBytes,
			EntryCount:         PackageEntryCount,
			TrustClass:         PackageTrustOperatorInstalledUntrusted,
			RiskCodes: []PackageRiskCode{
				PackageRiskUntrustedInstructions,
				PackageRiskDeclaredToolsOnly,
			},
		},
		content: bytes.Clone(content),
	}, nil
}

type packageEntryRecord struct {
	name              string
	flags             uint16
	method            uint16
	crc32             uint32
	compressedBytes   uint32
	uncompressedBytes uint32
	localOffset       uint32
}

const (
	zipLocalHeaderSignature   = 0x04034b50
	zipCentralHeaderSignature = 0x02014b50
	zipDescriptorSignature    = 0x08074b50
	zipEndHeaderSignature     = 0x06054b50
	zipLocalHeaderBytes       = 30
	zipCentralHeaderBytes     = 46
	zipDescriptorBytes        = 16
	zipEndHeaderBytes         = 22
	zipDataDescriptorFlag     = 1 << 3
	zipDeflateVersion         = 20
)

func inspectPackageContainer(raw []byte) ([]packageEntryRecord, int, error) {
	if len(raw) < zipEndHeaderBytes || binary.LittleEndian.Uint32(raw[:4]) != zipLocalHeaderSignature {
		return nil, 0, errors.New("ZIP must start with a local file header")
	}
	endOffset := len(raw) - zipEndHeaderBytes
	end := raw[endOffset:]
	if binary.LittleEndian.Uint32(end[:4]) != zipEndHeaderSignature {
		return nil, 0, errors.New("ZIP must end with an uncommented end record")
	}
	if binary.LittleEndian.Uint16(end[4:6]) != 0 || binary.LittleEndian.Uint16(end[6:8]) != 0 {
		return nil, 0, errors.New("multi-disk ZIP is not supported")
	}
	if binary.LittleEndian.Uint16(end[8:10]) != PackageEntryCount ||
		binary.LittleEndian.Uint16(end[10:12]) != PackageEntryCount {
		return nil, 0, fmt.Errorf("ZIP must contain exactly %d entries", PackageEntryCount)
	}
	if binary.LittleEndian.Uint16(end[20:22]) != 0 {
		return nil, 0, errors.New("ZIP comments are forbidden")
	}
	centralBytes := uint64(binary.LittleEndian.Uint32(end[12:16]))
	centralOffset := uint64(binary.LittleEndian.Uint32(end[16:20]))
	if centralOffset+centralBytes != uint64(endOffset) || centralOffset >= uint64(endOffset) {
		return nil, 0, errors.New("ZIP contains prefix, gap, ZIP64, or trailing data")
	}

	expectedNames := [...]string{PackageManifestPath, PackageContentPath}
	records := make([]packageEntryRecord, 0, PackageEntryCount)
	cursor := int(centralOffset)
	totalUncompressed := uint64(0)
	for index, expectedName := range expectedNames {
		if cursor+zipCentralHeaderBytes > endOffset ||
			binary.LittleEndian.Uint32(raw[cursor:cursor+4]) != zipCentralHeaderSignature {
			return nil, 0, fmt.Errorf("central directory entry %d is malformed", index)
		}
		header := raw[cursor : cursor+zipCentralHeaderBytes]
		creatorVersion := binary.LittleEndian.Uint16(header[4:6])
		versionNeeded := binary.LittleEndian.Uint16(header[6:8])
		flags := binary.LittleEndian.Uint16(header[8:10])
		method := binary.LittleEndian.Uint16(header[10:12])
		modifiedTime := binary.LittleEndian.Uint16(header[12:14])
		modifiedDate := binary.LittleEndian.Uint16(header[14:16])
		crc := binary.LittleEndian.Uint32(header[16:20])
		compressed := binary.LittleEndian.Uint32(header[20:24])
		uncompressed := binary.LittleEndian.Uint32(header[24:28])
		nameBytes := int(binary.LittleEndian.Uint16(header[28:30]))
		extraBytes := int(binary.LittleEndian.Uint16(header[30:32]))
		commentBytes := int(binary.LittleEndian.Uint16(header[32:34]))
		diskStart := binary.LittleEndian.Uint16(header[34:36])
		internalAttributes := binary.LittleEndian.Uint16(header[36:38])
		externalAttributes := binary.LittleEndian.Uint32(header[38:42])
		localOffset := binary.LittleEndian.Uint32(header[42:46])
		entryEnd := cursor + zipCentralHeaderBytes + nameBytes + extraBytes + commentBytes
		if entryEnd > endOffset {
			return nil, 0, fmt.Errorf("central directory entry %d exceeds its bound", index)
		}
		name := string(raw[cursor+zipCentralHeaderBytes : cursor+zipCentralHeaderBytes+nameBytes])
		if name != expectedName {
			return nil, 0, fmt.Errorf("ZIP entry %d is %q, want %q", index, name, expectedName)
		}
		if diskStart != 0 || internalAttributes != 0 || externalAttributes != 0 {
			return nil, 0, fmt.Errorf("ZIP entry %q has unsupported link, type, disk, or attribute metadata", name)
		}
		if creatorVersion != zipDeflateVersion || versionNeeded != zipDeflateVersion ||
			flags != zipDataDescriptorFlag || method != zip.Deflate {
			return nil, 0, fmt.Errorf("ZIP entry %q must use the fixed Deflate descriptor profile", name)
		}
		if modifiedTime != 0 || modifiedDate != 0 || extraBytes != 0 || commentBytes != 0 {
			return nil, 0, fmt.Errorf("ZIP entry %q contains non-deterministic metadata", name)
		}
		limit := uint32(MaxContentBytes)
		if index == 0 {
			limit = uint32(MaxManifestBytes)
		}
		if uncompressed == 0 || uncompressed > limit || compressed == 0 ||
			uint64(uncompressed) > uint64(compressed)*MaxPackageCompressionRatio {
			return nil, 0, fmt.Errorf("ZIP entry %q exceeds its size or compression-ratio bound", name)
		}
		totalUncompressed += uint64(uncompressed)
		if totalUncompressed > MaxPackageUncompressedBytes {
			return nil, 0, fmt.Errorf("ZIP uncompressed content exceeds %d bytes", MaxPackageUncompressedBytes)
		}
		records = append(records, packageEntryRecord{
			name: name, flags: flags, method: method, crc32: crc,
			compressedBytes: compressed, uncompressedBytes: uncompressed,
			localOffset: localOffset,
		})
		cursor = entryEnd
	}
	if cursor != endOffset {
		return nil, 0, errors.New("central directory contains extra entries or data")
	}

	expectedLocalOffset := uint64(0)
	for index, record := range records {
		if uint64(record.localOffset) != expectedLocalOffset || expectedLocalOffset+zipLocalHeaderBytes > centralOffset {
			return nil, 0, fmt.Errorf("local ZIP entry %d is reordered or out of bounds", index)
		}
		cursor = int(expectedLocalOffset)
		header := raw[cursor : cursor+zipLocalHeaderBytes]
		if binary.LittleEndian.Uint32(header[:4]) != zipLocalHeaderSignature ||
			binary.LittleEndian.Uint16(header[4:6]) != zipDeflateVersion ||
			binary.LittleEndian.Uint16(header[6:8]) != record.flags ||
			binary.LittleEndian.Uint16(header[8:10]) != record.method ||
			binary.LittleEndian.Uint16(header[10:12]) != 0 ||
			binary.LittleEndian.Uint16(header[12:14]) != 0 {
			return nil, 0, fmt.Errorf("local ZIP entry %q does not match its central header", record.name)
		}
		if binary.LittleEndian.Uint32(header[14:18]) != 0 ||
			binary.LittleEndian.Uint32(header[18:22]) != 0 ||
			binary.LittleEndian.Uint32(header[22:26]) != 0 {
			return nil, 0, fmt.Errorf("local ZIP entry %q must defer CRC and sizes to its descriptor", record.name)
		}
		nameBytes := int(binary.LittleEndian.Uint16(header[26:28]))
		extraBytes := int(binary.LittleEndian.Uint16(header[28:30]))
		nameStart := cursor + zipLocalHeaderBytes
		dataStart := nameStart + nameBytes + extraBytes
		dataEnd := uint64(dataStart) + uint64(record.compressedBytes)
		descriptorEnd := dataEnd + zipDescriptorBytes
		if extraBytes != 0 || dataStart > int(centralOffset) || descriptorEnd > centralOffset ||
			string(raw[nameStart:nameStart+nameBytes]) != record.name {
			return nil, 0, fmt.Errorf("local ZIP entry %q has an invalid name, extra field, or data bound", record.name)
		}
		descriptor := raw[int(dataEnd):int(descriptorEnd)]
		if binary.LittleEndian.Uint32(descriptor[:4]) != zipDescriptorSignature ||
			binary.LittleEndian.Uint32(descriptor[4:8]) != record.crc32 ||
			binary.LittleEndian.Uint32(descriptor[8:12]) != record.compressedBytes ||
			binary.LittleEndian.Uint32(descriptor[12:16]) != record.uncompressedBytes {
			return nil, 0, fmt.Errorf("ZIP entry %q has an invalid data descriptor", record.name)
		}
		if err := validatePackageDeflateStream(raw[dataStart:int(dataEnd)], record); err != nil {
			return nil, 0, err
		}
		expectedLocalOffset = descriptorEnd
	}
	if expectedLocalOffset != centralOffset {
		return nil, 0, errors.New("ZIP contains a gap between local entries and the central directory")
	}
	return records, int(totalUncompressed), nil
}

func validatePackageDeflateStream(compressed []byte, record packageEntryRecord) error {
	source := bytes.NewReader(compressed)
	reader := flate.NewReader(source)
	data, readErr := io.ReadAll(io.LimitReader(reader, int64(record.uncompressedBytes)+1))
	closeErr := reader.Close()
	if readErr != nil {
		return fmt.Errorf("ZIP entry %q has an invalid Deflate stream: %w", record.name, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("ZIP entry %q cannot close its Deflate stream: %w", record.name, closeErr)
	}
	if source.Len() != 0 {
		return fmt.Errorf("ZIP entry %q contains data after its Deflate stream", record.name)
	}
	if uint64(len(data)) != uint64(record.uncompressedBytes) || crc32.ChecksumIEEE(data) != record.crc32 {
		return fmt.Errorf("ZIP entry %q does not match its declared size or CRC", record.name)
	}
	return nil
}

func readPackageEntry(file *zip.File, record packageEntryRecord, limit int) ([]byte, error) {
	if file == nil || file.Name != record.name || file.Method != record.method ||
		file.Flags != record.flags || file.CRC32 != record.crc32 ||
		file.CompressedSize64 != uint64(record.compressedBytes) ||
		file.UncompressedSize64 != uint64(record.uncompressedBytes) {
		return nil, fmt.Errorf("ZIP entry %q disagrees with the validated container", record.name)
	}
	if !file.FileInfo().Mode().IsRegular() {
		return nil, fmt.Errorf("ZIP entry %q must be a regular file", record.name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open ZIP entry %q: %w", record.name, err)
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read ZIP entry %q: %w", record.name, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close ZIP entry %q: %w", record.name, closeErr)
	}
	if len(data) == 0 || len(data) > limit || uint64(len(data)) != file.UncompressedSize64 {
		return nil, fmt.Errorf("ZIP entry %q violates its uncompressed-size bound", record.name)
	}
	return data, nil
}

func packageFingerprint(manifest Manifest, content []byte) (string, error) {
	canonicalManifest, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(PackageProtocolVersion))
	_, _ = hash.Write([]byte{0})
	writePackageFrame(hash, canonicalManifest)
	writePackageFrame(hash, content)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writePackageFrame(writer io.Writer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write(value)
}
