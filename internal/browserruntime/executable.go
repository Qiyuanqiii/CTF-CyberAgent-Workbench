package browserruntime

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
)

const (
	BrowserExecutableIdentityProtocolVersion = "browser_executable_identity.v1"
	MinBrowserExecutableBytes                = 512
	MaxBrowserExecutableBytes                = 512 * 1024 * 1024
	maxPEHeaderOffset                        = 1024 * 1024
)

type BrowserProduct string

const (
	BrowserProductChrome   BrowserProduct = "chrome"
	BrowserProductEdge     BrowserProduct = "edge"
	BrowserProductChromium BrowserProduct = "chromium"
)

type BrowserChannel string

const (
	BrowserChannelStable BrowserChannel = "stable"
	BrowserChannelBeta   BrowserChannel = "beta"
	BrowserChannelDev    BrowserChannel = "dev"
	BrowserChannelCanary BrowserChannel = "canary"
)

type DiscoveryRootID string

const (
	DiscoveryRootProgramFiles    DiscoveryRootID = "program_files"
	DiscoveryRootProgramFilesX86 DiscoveryRootID = "program_files_x86"
	DiscoveryRootLocalAppData    DiscoveryRootID = "local_app_data"
)

type ExecutableVersionSource string

const (
	VersionSourceUnavailable     ExecutableVersionSource = "unavailable"
	VersionSourceWindowsResource ExecutableVersionSource = "windows_version_resource"
)

// DiscoveryRoot identifies one OS-provided installation root. Product code
// obtains these roots without consulting PATH; callers cannot add executable
// names or relative paths to the fixed candidate registry.
type DiscoveryRoot struct {
	ID   DiscoveryRootID
	Path string
}

// BrowserExecutableIdentity is read-only discovery metadata. It contains no
// command line, environment, writable profile, or process-start grant.
type BrowserExecutableIdentity struct {
	ProtocolVersion            string                  `json:"protocol_version"`
	Product                    BrowserProduct          `json:"product"`
	Channel                    BrowserChannel          `json:"channel"`
	Vendor                     string                  `json:"vendor"`
	RootID                     DiscoveryRootID         `json:"root_id"`
	CanonicalPath              string                  `json:"canonical_path"`
	RelativePath               string                  `json:"relative_path"`
	HostGOOS                   string                  `json:"host_goos"`
	HostGOARCH                 string                  `json:"host_goarch"`
	TargetGOARCH               string                  `json:"target_goarch"`
	ExecutableBytes            int64                   `json:"executable_bytes"`
	ExecutableSHA256           string                  `json:"executable_sha256"`
	Version                    string                  `json:"version,omitempty"`
	VersionSource              ExecutableVersionSource `json:"version_source"`
	VersionVerified            bool                    `json:"version_verified"`
	PEFormatVerified           bool                    `json:"pe_format_verified"`
	PublisherSignatureVerified bool                    `json:"publisher_signature_verified"`
	LaunchTrustComplete        bool                    `json:"launch_trust_complete"`
	RegularFileVerified        bool                    `json:"regular_file_verified"`
	SymlinkRejected            bool                    `json:"symlink_rejected"`
	MetadataOnly               bool                    `json:"metadata_only"`
	RawBytesIncluded           bool                    `json:"raw_bytes_included"`
	PathPersistenceAllowed     bool                    `json:"path_persistence_allowed"`
	PATHLookupUsed             bool                    `json:"path_lookup_used"`
	ProcessStartEnabled        bool                    `json:"process_start_enabled"`
	ProductLaunchEnabled       bool                    `json:"product_launch_enabled"`
	Authority                  RuntimeAuthority        `json:"authority"`
	Fingerprint                string                  `json:"fingerprint"`
}

type browserExecutableSpec struct {
	RootID     DiscoveryRootID
	Product    BrowserProduct
	Channel    BrowserChannel
	Vendor     string
	Components []string
}

type executableVersionProbe func(string) (string, ExecutableVersionSource, bool)

// DiscoverInstalledBrowsers inspects only fixed browser locations below
// OS-provided installation roots. Missing roots and candidates are ordinary;
// an existing but unsafe candidate fails the discovery closed.
func DiscoverInstalledBrowsers() ([]BrowserExecutableIdentity, error) {
	return discoverBrowserExecutables(defaultBrowserDiscoveryRoots(), knownBrowserExecutableSpecs(),
		browserExecutableVersion)
}

func discoverBrowserExecutables(roots []DiscoveryRoot, specs []browserExecutableSpec,
	versionProbe executableVersionProbe,
) ([]BrowserExecutableIdentity, error) {
	if len(roots) > 3 {
		return nil, errors.New("browser discovery root count exceeds the fixed bound")
	}
	if versionProbe == nil {
		return nil, errors.New("browser executable version probe is required")
	}
	resolvedRoots := make(map[DiscoveryRootID]string, len(roots))
	for _, root := range roots {
		if !validDiscoveryRootID(root.ID) {
			return nil, fmt.Errorf("unsupported browser discovery root %q", root.ID)
		}
		if _, exists := resolvedRoots[root.ID]; exists {
			return nil, fmt.Errorf("duplicate browser discovery root %q", root.ID)
		}
		canonical, exists, err := inspectDiscoveryRoot(root.Path)
		if err != nil {
			return nil, fmt.Errorf("inspect browser discovery root %q: %w", root.ID, err)
		}
		if exists {
			resolvedRoots[root.ID] = canonical
		}
	}

	identities := make([]BrowserExecutableIdentity, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		root, ok := resolvedRoots[spec.RootID]
		if !ok {
			continue
		}
		if !validExecutableSpec(spec) {
			return nil, errors.New("browser executable registry contains an invalid candidate")
		}
		identity, exists, err := inspectBrowserExecutable(root, spec, versionProbe)
		if err != nil {
			return nil, fmt.Errorf("inspect %s %s browser candidate: %w",
				spec.Product, spec.Channel, err)
		}
		if !exists {
			continue
		}
		key := identity.CanonicalPath
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(left, right int) bool {
		if identities[left].Product != identities[right].Product {
			return identities[left].Product < identities[right].Product
		}
		if identities[left].Channel != identities[right].Channel {
			return identities[left].Channel < identities[right].Channel
		}
		return identities[left].CanonicalPath < identities[right].CanonicalPath
	})
	return identities, nil
}

func ValidateBrowserExecutableIdentity(identity BrowserExecutableIdentity) error {
	if identity.ProtocolVersion != BrowserExecutableIdentityProtocolVersion ||
		!validProductChannel(identity.Product, identity.Channel) ||
		identity.Vendor != browserVendor(identity.Product) ||
		!validDiscoveryRootID(identity.RootID) || !filepath.IsAbs(identity.CanonicalPath) ||
		filepath.Clean(identity.CanonicalPath) != identity.CanonicalPath ||
		!validRelativeBrowserPath(identity.RelativePath) ||
		identity.HostGOOS == "" || identity.HostGOARCH == "" ||
		!validBrowserTargetArch(identity.TargetGOARCH) ||
		identity.ExecutableBytes < MinBrowserExecutableBytes ||
		identity.ExecutableBytes > MaxBrowserExecutableBytes ||
		!validSHA256(identity.ExecutableSHA256) {
		return errors.New("browser executable identity contains invalid metadata")
	}
	if !matchesKnownExecutableSpec(identity) {
		return errors.New("browser executable identity is outside the fixed candidate registry")
	}
	if !validVersionMetadata(identity.Version, identity.VersionSource, identity.VersionVerified) {
		return errors.New("browser executable identity contains invalid version metadata")
	}
	if !identity.PEFormatVerified || identity.PublisherSignatureVerified ||
		identity.LaunchTrustComplete || !identity.RegularFileVerified || !identity.SymlinkRejected ||
		!identity.MetadataOnly || identity.RawBytesIncluded || identity.PATHLookupUsed ||
		identity.PathPersistenceAllowed ||
		identity.ProcessStartEnabled || identity.ProductLaunchEnabled ||
		identity.Authority != (RuntimeAuthority{}) {
		return errors.New("browser executable identity grants authority or lost a verification boundary")
	}
	root, ok := executableIdentityRoot(identity)
	if !ok || !pathWithinRoot(root, identity.CanonicalPath) {
		return errors.New("browser executable identity path escaped its root")
	}
	expected, err := browserExecutableIdentityFingerprint(identity)
	if err != nil || identity.Fingerprint != expected {
		return errors.New("browser executable identity fingerprint mismatch")
	}
	return nil
}

// RevalidateBrowserExecutableIdentity rereads the exact admitted path and
// rejects byte, file-type, path, version, or platform drift. It still does not
// authorize or start a process.
func RevalidateBrowserExecutableIdentity(identity BrowserExecutableIdentity) error {
	if err := ValidateBrowserExecutableIdentity(identity); err != nil {
		return err
	}
	root, ok := executableIdentityRoot(identity)
	if !ok {
		return errors.New("browser executable identity root cannot be reconstructed")
	}
	spec := browserExecutableSpec{
		RootID: identity.RootID, Product: identity.Product, Channel: identity.Channel,
		Vendor: identity.Vendor, Components: strings.Split(identity.RelativePath, "/"),
	}
	rebuilt, exists, err := inspectBrowserExecutable(root, spec, browserExecutableVersion)
	if err != nil {
		return err
	}
	if !exists || !reflect.DeepEqual(identity, rebuilt) {
		return errors.New("browser executable identity no longer matches the admitted file")
	}
	return nil
}

func inspectDiscoveryRoot(raw string) (string, bool, error) {
	if raw == "" {
		return "", false, nil
	}
	absolute, err := filepath.Abs(raw)
	if err != nil || filepath.Clean(absolute) != absolute {
		return "", false, errors.New("discovery root is not canonical")
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("discovery root is unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", false, errors.New("discovery root is not a real directory")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil || !samePath(absolute, resolved) {
		return "", false, errors.New("discovery root contains filesystem indirection")
	}
	return absolute, true, nil
}

func inspectBrowserExecutable(root string, spec browserExecutableSpec,
	versionProbe executableVersionProbe,
) (BrowserExecutableIdentity, bool, error) {
	relative := filepath.Join(spec.Components...)
	candidate := filepath.Join(root, relative)
	if !pathWithinRoot(root, candidate) {
		return BrowserExecutableIdentity{}, false, errors.New("candidate escaped its discovery root")
	}
	info, err := os.Lstat(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return BrowserExecutableIdentity{}, false, nil
	}
	if err != nil {
		return BrowserExecutableIdentity{}, false, errors.New("candidate metadata is unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return BrowserExecutableIdentity{}, false, errors.New("candidate is not a non-link regular file")
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil || !samePath(candidate, resolved) || !pathWithinRoot(root, resolved) {
		return BrowserExecutableIdentity{}, false, errors.New("candidate contains filesystem indirection")
	}
	file, err := os.Open(candidate)
	if err != nil {
		return BrowserExecutableIdentity{}, false, errors.New("candidate cannot be opened read-only")
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() < MinBrowserExecutableBytes ||
		before.Size() > MaxBrowserExecutableBytes {
		return BrowserExecutableIdentity{}, false, errors.New("candidate size or file type is invalid")
	}
	targetArch, err := inspectPEArchitecture(file, before.Size())
	if err != nil {
		return BrowserExecutableIdentity{}, false, err
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, MaxBrowserExecutableBytes+1))
	if err != nil || written != before.Size() {
		return BrowserExecutableIdentity{}, false, errors.New("candidate bytes changed while hashing")
	}
	after, err := file.Stat()
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return BrowserExecutableIdentity{}, false, errors.New("candidate metadata changed while hashing")
	}
	version, source, verified := versionProbe(candidate)
	if !validVersionMetadata(version, source, verified) {
		return BrowserExecutableIdentity{}, false, errors.New("candidate version metadata is invalid")
	}
	pathAfter, err := os.Lstat(candidate)
	if err != nil || pathAfter.Mode()&os.ModeSymlink != 0 || !pathAfter.Mode().IsRegular() ||
		!os.SameFile(before, pathAfter) || pathAfter.Size() != before.Size() ||
		!pathAfter.ModTime().Equal(before.ModTime()) {
		return BrowserExecutableIdentity{}, false, errors.New("candidate path changed during identity inspection")
	}
	identity := BrowserExecutableIdentity{
		ProtocolVersion: BrowserExecutableIdentityProtocolVersion,
		Product:         spec.Product, Channel: spec.Channel, Vendor: spec.Vendor, RootID: spec.RootID,
		CanonicalPath: candidate, RelativePath: filepath.ToSlash(relative),
		HostGOOS: runtime.GOOS, HostGOARCH: runtime.GOARCH, TargetGOARCH: targetArch,
		ExecutableBytes: before.Size(), ExecutableSHA256: hex.EncodeToString(hasher.Sum(nil)),
		Version: version, VersionSource: source, VersionVerified: verified,
		PEFormatVerified: true, RegularFileVerified: true, SymlinkRejected: true,
		MetadataOnly: true,
	}
	identity.Fingerprint, err = browserExecutableIdentityFingerprint(identity)
	if err != nil {
		return BrowserExecutableIdentity{}, false, err
	}
	if err := ValidateBrowserExecutableIdentity(identity); err != nil {
		return BrowserExecutableIdentity{}, false, err
	}
	return identity, true, nil
}

func inspectPEArchitecture(file *os.File, size int64) (string, error) {
	var dos [64]byte
	if _, err := file.ReadAt(dos[:], 0); err != nil || string(dos[:2]) != "MZ" {
		return "", errors.New("candidate is not a bounded PE image")
	}
	offset := int64(binary.LittleEndian.Uint32(dos[0x3c:0x40]))
	if offset < int64(len(dos)) || offset > maxPEHeaderOffset || offset+24 > size {
		return "", errors.New("candidate PE header offset is invalid")
	}
	var header [24]byte
	if _, err := file.ReadAt(header[:], offset); err != nil || string(header[:4]) != "PE\x00\x00" ||
		binary.LittleEndian.Uint16(header[6:8]) == 0 ||
		binary.LittleEndian.Uint16(header[22:24])&0x0002 == 0 {
		return "", errors.New("candidate PE header is invalid")
	}
	switch binary.LittleEndian.Uint16(header[4:6]) {
	case 0x014c:
		return "386", nil
	case 0x8664:
		return "amd64", nil
	case 0xaa64:
		return "arm64", nil
	default:
		return "", errors.New("candidate PE architecture is unsupported")
	}
}

func knownBrowserExecutableSpecs() []browserExecutableSpec {
	roots := []DiscoveryRootID{
		DiscoveryRootProgramFiles, DiscoveryRootProgramFilesX86, DiscoveryRootLocalAppData,
	}
	base := []browserExecutableSpec{
		{Product: BrowserProductChrome, Channel: BrowserChannelStable, Vendor: "Google",
			Components: []string{"Google", "Chrome", "Application", "chrome.exe"}},
		{Product: BrowserProductChrome, Channel: BrowserChannelBeta, Vendor: "Google",
			Components: []string{"Google", "Chrome Beta", "Application", "chrome.exe"}},
		{Product: BrowserProductChrome, Channel: BrowserChannelDev, Vendor: "Google",
			Components: []string{"Google", "Chrome Dev", "Application", "chrome.exe"}},
		{Product: BrowserProductChrome, Channel: BrowserChannelCanary, Vendor: "Google",
			Components: []string{"Google", "Chrome SxS", "Application", "chrome.exe"}},
		{Product: BrowserProductEdge, Channel: BrowserChannelStable, Vendor: "Microsoft",
			Components: []string{"Microsoft", "Edge", "Application", "msedge.exe"}},
		{Product: BrowserProductEdge, Channel: BrowserChannelBeta, Vendor: "Microsoft",
			Components: []string{"Microsoft", "Edge Beta", "Application", "msedge.exe"}},
		{Product: BrowserProductEdge, Channel: BrowserChannelDev, Vendor: "Microsoft",
			Components: []string{"Microsoft", "Edge Dev", "Application", "msedge.exe"}},
		{Product: BrowserProductEdge, Channel: BrowserChannelCanary, Vendor: "Microsoft",
			Components: []string{"Microsoft", "Edge SxS", "Application", "msedge.exe"}},
		{Product: BrowserProductChromium, Channel: BrowserChannelStable, Vendor: "Chromium",
			Components: []string{"Chromium", "Application", "chrome.exe"}},
	}
	result := make([]browserExecutableSpec, 0, len(roots)*len(base))
	for _, root := range roots {
		for _, candidate := range base {
			candidate.RootID = root
			candidate.Components = append([]string(nil), candidate.Components...)
			result = append(result, candidate)
		}
	}
	return result
}

func validExecutableSpec(spec browserExecutableSpec) bool {
	if !validDiscoveryRootID(spec.RootID) || !validProductChannel(spec.Product, spec.Channel) ||
		spec.Vendor != browserVendor(spec.Product) || len(spec.Components) < 2 ||
		len(spec.Components) > 6 {
		return false
	}
	for _, component := range spec.Components {
		if component == "" || component == "." || component == ".." ||
			strings.ContainsAny(component, `/\\`) {
			return false
		}
	}
	return strings.HasSuffix(strings.ToLower(spec.Components[len(spec.Components)-1]), ".exe")
}

func matchesKnownExecutableSpec(identity BrowserExecutableIdentity) bool {
	for _, spec := range knownBrowserExecutableSpecs() {
		if spec.RootID == identity.RootID && spec.Product == identity.Product &&
			spec.Channel == identity.Channel && spec.Vendor == identity.Vendor &&
			filepath.ToSlash(filepath.Join(spec.Components...)) == identity.RelativePath {
			return true
		}
	}
	return false
}

func executableIdentityRoot(identity BrowserExecutableIdentity) (string, bool) {
	root := identity.CanonicalPath
	components := strings.Split(identity.RelativePath, "/")
	for range components {
		root = filepath.Dir(root)
	}
	root = filepath.Clean(root)
	relative, err := filepath.Rel(root, identity.CanonicalPath)
	if err != nil || filepath.ToSlash(relative) != identity.RelativePath {
		return "", false
	}
	return root, true
}

func validDiscoveryRootID(value DiscoveryRootID) bool {
	return value == DiscoveryRootProgramFiles || value == DiscoveryRootProgramFilesX86 ||
		value == DiscoveryRootLocalAppData
}

func validProductChannel(product BrowserProduct, channel BrowserChannel) bool {
	if product != BrowserProductChrome && product != BrowserProductEdge &&
		product != BrowserProductChromium {
		return false
	}
	if product == BrowserProductChromium {
		return channel == BrowserChannelStable
	}
	return channel == BrowserChannelStable || channel == BrowserChannelBeta ||
		channel == BrowserChannelDev || channel == BrowserChannelCanary
}

func browserVendor(product BrowserProduct) string {
	switch product {
	case BrowserProductChrome:
		return "Google"
	case BrowserProductEdge:
		return "Microsoft"
	case BrowserProductChromium:
		return "Chromium"
	default:
		return ""
	}
}

func validBrowserTargetArch(value string) bool {
	return value == "386" || value == "amd64" || value == "arm64"
}

func validVersionMetadata(version string, source ExecutableVersionSource, verified bool) bool {
	if !verified {
		return version == "" && source == VersionSourceUnavailable
	}
	if source != VersionSourceWindowsResource || version == "" || len(version) > 64 {
		return false
	}
	parts := strings.Split(version, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 5 {
			return false
		}
		for _, current := range part {
			if current < '0' || current > '9' {
				return false
			}
		}
	}
	return true
}

func validRelativeBrowserPath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || filepath.IsAbs(filepath.FromSlash(value)) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func pathWithinRoot(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func samePath(left string, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func browserExecutableIdentityFingerprint(identity BrowserExecutableIdentity) (string, error) {
	copyValue := identity
	copyValue.Fingerprint = ""
	raw, err := json.Marshal(copyValue)
	if err != nil {
		return "", fmt.Errorf("encode browser executable identity: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
