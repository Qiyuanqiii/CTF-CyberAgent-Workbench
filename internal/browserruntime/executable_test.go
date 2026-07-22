package browserruntime

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBrowserExecutableDiscoveryBindsFixedPEBytesWithoutPATHOrLaunch(t *testing.T) {
	root := t.TempDir()
	relative := filepath.Join("Google", "Chrome", "Application", "chrome.exe")
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := minimalPEImage(t, "amd64")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	spec := browserExecutableSpec{
		RootID: DiscoveryRootProgramFiles, Product: BrowserProductChrome,
		Channel: BrowserChannelStable, Vendor: "Google",
		Components: []string{"Google", "Chrome", "Application", "chrome.exe"},
	}
	versionProbe := func(string) (string, ExecutableVersionSource, bool) {
		if runtime.GOOS == "windows" {
			return "123.45.67.89", VersionSourceWindowsResource, true
		}
		return "", VersionSourceUnavailable, false
	}
	identities, err := discoverBrowserExecutables([]DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: root},
	}, []browserExecutableSpec{spec}, versionProbe)
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 1 {
		t.Fatalf("expected one browser identity, got %d", len(identities))
	}
	identity := identities[0]
	digest := sha256.Sum256(raw)
	if identity.ExecutableSHA256 != hex.EncodeToString(digest[:]) ||
		identity.ExecutableBytes != int64(len(raw)) || identity.TargetGOARCH != "amd64" ||
		identity.RelativePath != filepath.ToSlash(relative) || identity.PATHLookupUsed ||
		identity.ProcessStartEnabled || identity.ProductLaunchEnabled ||
		identity.PublisherSignatureVerified || identity.LaunchTrustComplete ||
		identity.PathPersistenceAllowed ||
		identity.Authority != (RuntimeAuthority{}) || !identity.MetadataOnly ||
		identity.RawBytesIncluded {
		t.Fatalf("unsafe or incomplete browser executable identity: %#v", identity)
	}
	if err := ValidateBrowserExecutableIdentity(identity); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserExecutableDiscoveryRejectsRegistryEscapeAndUnsafeCandidate(t *testing.T) {
	root := t.TempDir()
	escape := browserExecutableSpec{
		RootID: DiscoveryRootProgramFiles, Product: BrowserProductChrome,
		Channel: BrowserChannelStable, Vendor: "Google",
		Components: []string{"..", "outside.exe"},
	}
	if _, err := discoverBrowserExecutables([]DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: root},
	}, []browserExecutableSpec{escape}, browserExecutableVersion); err == nil {
		t.Fatal("registry traversal unexpectedly passed")
	}

	path := filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	valid := knownSpec(t, DiscoveryRootProgramFiles, BrowserProductChrome,
		BrowserChannelStable)
	if _, err := discoverBrowserExecutables([]DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: root},
	}, []browserExecutableSpec{valid}, browserExecutableVersion); err == nil {
		t.Fatal("directory candidate unexpectedly passed")
	}
}

func TestBrowserExecutableIdentityRejectsTamperingAndByteDrift(t *testing.T) {
	root := t.TempDir()
	spec := knownSpec(t, DiscoveryRootProgramFiles, BrowserProductEdge,
		BrowserChannelStable)
	path := filepath.Join(append([]string{root}, spec.Components...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := minimalPEImage(t, "amd64")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := discoverBrowserExecutables([]DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: root},
	}, []browserExecutableSpec{spec}, browserExecutableVersion)
	if err != nil || len(identities) != 1 {
		t.Fatalf("discover identity: count=%d err=%v", len(identities), err)
	}
	base := identities[0]
	mutations := []func(*BrowserExecutableIdentity){
		func(value *BrowserExecutableIdentity) { value.PATHLookupUsed = true },
		func(value *BrowserExecutableIdentity) { value.ProcessStartEnabled = true },
		func(value *BrowserExecutableIdentity) { value.PublisherSignatureVerified = true },
		func(value *BrowserExecutableIdentity) { value.Authority.NetworkAccess = true },
		func(value *BrowserExecutableIdentity) { value.RelativePath = "unknown.exe" },
		func(value *BrowserExecutableIdentity) { value.ExecutableSHA256 = strings.Repeat("a", 64) },
		func(value *BrowserExecutableIdentity) { value.Fingerprint = strings.Repeat("b", 64) },
	}
	for index, mutate := range mutations {
		candidate := base
		mutate(&candidate)
		if err := ValidateBrowserExecutableIdentity(candidate); err == nil {
			t.Fatalf("identity mutation %d unexpectedly passed", index)
		}
	}

	raw[len(raw)-1] ^= 0xff
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RevalidateBrowserExecutableIdentity(base); err == nil {
		t.Fatal("changed browser executable bytes unexpectedly revalidated")
	}
}

func TestBrowserExecutableDiscoverySkipsMissingCandidatesAndDeduplicatesRoots(t *testing.T) {
	root := t.TempDir()
	spec := knownSpec(t, DiscoveryRootProgramFiles, BrowserProductChromium,
		BrowserChannelStable)
	identities, err := discoverBrowserExecutables([]DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: root},
	}, []browserExecutableSpec{spec}, browserExecutableVersion)
	if err != nil || len(identities) != 0 {
		t.Fatalf("missing browser candidate should be empty: count=%d err=%v", len(identities), err)
	}
	if _, err := discoverBrowserExecutables([]DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: root},
		{ID: DiscoveryRootProgramFiles, Path: root},
	}, nil, browserExecutableVersion); err == nil {
		t.Fatal("duplicate discovery roots unexpectedly passed")
	}
}

func TestInstalledBrowserDiscoverySmoke(t *testing.T) {
	if os.Getenv("CYBERAGENT_BROWSER_DISCOVERY_SMOKE") != "1" {
		t.Skip("set CYBERAGENT_BROWSER_DISCOVERY_SMOKE=1 for local read-only discovery")
	}
	identities, err := DiscoverInstalledBrowsers()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read-only discovery found %d fixed-location browser candidate(s)", len(identities))
	for _, identity := range identities {
		if err := ValidateBrowserExecutableIdentity(identity); err != nil {
			t.Fatal(err)
		}
		if identity.ProcessStartEnabled || identity.ProductLaunchEnabled ||
			identity.PublisherSignatureVerified || identity.LaunchTrustComplete ||
			identity.Authority != (RuntimeAuthority{}) {
			t.Fatalf("local discovery returned an authorizing identity: %#v", identity)
		}
	}
}

func knownSpec(t *testing.T, root DiscoveryRootID, product BrowserProduct,
	channel BrowserChannel,
) browserExecutableSpec {
	t.Helper()
	for _, spec := range knownBrowserExecutableSpecs() {
		if spec.RootID == root && spec.Product == product && spec.Channel == channel {
			return spec
		}
	}
	t.Fatalf("missing fixed browser spec for %s/%s/%s", root, product, channel)
	return browserExecutableSpec{}
}

func minimalPEImage(t *testing.T, arch string) []byte {
	t.Helper()
	raw := make([]byte, 1024)
	copy(raw[:2], "MZ")
	binary.LittleEndian.PutUint32(raw[0x3c:0x40], 0x80)
	copy(raw[0x80:0x84], "PE\x00\x00")
	var machine uint16
	switch arch {
	case "386":
		machine = 0x014c
	case "amd64":
		machine = 0x8664
	case "arm64":
		machine = 0xaa64
	default:
		t.Fatalf("unsupported test PE architecture %q", arch)
	}
	binary.LittleEndian.PutUint16(raw[0x84:0x86], machine)
	binary.LittleEndian.PutUint16(raw[0x86:0x88], 1)
	binary.LittleEndian.PutUint16(raw[0x96:0x98], 0x0002)
	return raw
}
