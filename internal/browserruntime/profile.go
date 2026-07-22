package browserruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	ProfileProtocolVersion = "browser_profile.v1"

	ProfileSafeWeb         ProfileID = "safe-web"
	ProfileCTFLab          ProfileID = "ctf-lab"
	ProfileCTFInstrumented ProfileID = "ctf-instrumented"
)

type ProfileID string

type Surface string

const (
	SurfaceCode  Surface = "code"
	SurfaceCyber Surface = "cyber"
)

type RiskTier string

const (
	RiskTierModerate RiskTier = "moderate"
	RiskTierElevated RiskTier = "elevated"
	RiskTierHigh     RiskTier = "high"
)

type ProfileLimits struct {
	MaxOrigins       int `json:"max_origins"`
	MaxURLBytes      int `json:"max_url_bytes"`
	MaxRequests      int `json:"max_requests"`
	MaxResponseBytes int `json:"max_response_bytes"`
	MaxDownloadBytes int `json:"max_download_bytes"`
	TimeoutMS        int `json:"timeout_ms"`
}

type NetworkControls struct {
	AllowPublicTargets    bool `json:"allow_public_targets"`
	AllowLoopbackTargets  bool `json:"allow_loopback_targets"`
	AllowPrivateTargets   bool `json:"allow_private_targets"`
	AllowProxy            bool `json:"allow_proxy"`
	RequireExactOrigins   bool `json:"require_exact_origins"`
	RevalidateRedirects   bool `json:"revalidate_redirects"`
	RevalidateResolvedIPs bool `json:"revalidate_resolved_ips"`
	BlockLinkLocal        bool `json:"block_link_local"`
	BlockCloudMetadata    bool `json:"block_cloud_metadata"`
	DefaultDeny           bool `json:"default_deny"`
}

type ToolControls struct {
	DOMInspection       bool `json:"dom_inspection"`
	Screenshots         bool `json:"screenshots"`
	RequestCapture      bool `json:"request_capture"`
	RequestInterception bool `json:"request_interception"`
	RequestMutation     bool `json:"request_mutation"`
	RequestReplay       bool `json:"request_replay"`
	CookieEditing       bool `json:"cookie_editing"`
}

type SecurityControls struct {
	PreserveSameOriginByDefault bool `json:"preserve_same_origin_by_default"`
	PreserveCSPByDefault        bool `json:"preserve_csp_by_default"`
	VerifyTLSByDefault          bool `json:"verify_tls_by_default"`
	MayRelaxOriginPolicy        bool `json:"may_relax_origin_policy"`
	MayRelaxMixedContent        bool `json:"may_relax_mixed_content"`
	MayRelaxCertificateErrors   bool `json:"may_relax_certificate_errors"`
}

type IsolationControls struct {
	DisposableProfileRequired bool `json:"disposable_profile_required"`
	ContainerRequired         bool `json:"container_required"`
	PersonalProfileForbidden  bool `json:"personal_profile_forbidden"`
	ExtensionsForbidden       bool `json:"extensions_forbidden"`
	PasswordStoreForbidden    bool `json:"password_store_forbidden"`
	HostFilesystemForbidden   bool `json:"host_filesystem_forbidden"`
}

// RuntimeAuthority is deliberately false in every descriptor in this batch.
// A profile describes a requested operating envelope; it is not a launch grant.
type RuntimeAuthority struct {
	ProcessStart    bool `json:"process_start"`
	NetworkAccess   bool `json:"network_access"`
	ProfileWrite    bool `json:"profile_write"`
	RequestMutation bool `json:"request_mutation"`
	RequestReplay   bool `json:"request_replay"`
	ArtifactCommit  bool `json:"artifact_commit"`
}

type ProfileDescriptor struct {
	ProtocolVersion  string            `json:"protocol_version"`
	ID               ProfileID         `json:"id"`
	Surface          Surface           `json:"surface"`
	RiskTier         RiskTier          `json:"risk_tier"`
	ApprovalRequired bool              `json:"approval_required"`
	EvidenceClass    string            `json:"evidence_class"`
	Limits           ProfileLimits     `json:"limits"`
	Network          NetworkControls   `json:"network"`
	Tools            ToolControls      `json:"tools"`
	Security         SecurityControls  `json:"security"`
	Isolation        IsolationControls `json:"isolation"`
	Authority        RuntimeAuthority  `json:"authority"`
}

type Registry struct {
	profiles map[ProfileID]ProfileDescriptor
}

func BuiltinRegistry() Registry {
	baseLimits := ProfileLimits{
		MaxOrigins: 8, MaxURLBytes: 4096, MaxRequests: 500,
		MaxResponseBytes: 8 << 20, MaxDownloadBytes: 16 << 20,
		TimeoutMS: 5 * 60 * 1000,
	}
	baseNetwork := NetworkControls{
		AllowPublicTargets: true, AllowProxy: true, RequireExactOrigins: true,
		RevalidateRedirects: true, RevalidateResolvedIPs: true,
		BlockLinkLocal: true, BlockCloudMetadata: true, DefaultDeny: true,
	}
	baseTools := ToolControls{DOMInspection: true, Screenshots: true, RequestCapture: true}
	baseSecurity := SecurityControls{
		PreserveSameOriginByDefault: true, PreserveCSPByDefault: true,
		VerifyTLSByDefault: true,
	}
	baseIsolation := IsolationControls{
		DisposableProfileRequired: true, PersonalProfileForbidden: true,
		ExtensionsForbidden: true, PasswordStoreForbidden: true,
		HostFilesystemForbidden: true,
	}

	safe := ProfileDescriptor{
		ProtocolVersion: ProfileProtocolVersion, ID: ProfileSafeWeb,
		Surface: SurfaceCode, RiskTier: RiskTierModerate, EvidenceClass: "normal",
		Limits: baseLimits, Network: baseNetwork, Tools: baseTools,
		Security: baseSecurity, Isolation: baseIsolation,
	}
	safe.Network.AllowLoopbackTargets = true

	lab := safe
	lab.ID = ProfileCTFLab
	lab.Surface = SurfaceCyber
	lab.RiskTier = RiskTierElevated
	lab.ApprovalRequired = true
	lab.Network.AllowPrivateTargets = true
	lab.Tools.RequestInterception = true
	lab.Tools.RequestMutation = true
	lab.Tools.RequestReplay = true
	lab.Tools.CookieEditing = true

	instrumented := lab
	instrumented.ID = ProfileCTFInstrumented
	instrumented.RiskTier = RiskTierHigh
	instrumented.EvidenceClass = "instrumented"
	instrumented.Security.MayRelaxOriginPolicy = true
	instrumented.Security.MayRelaxMixedContent = true
	instrumented.Security.MayRelaxCertificateErrors = true
	instrumented.Isolation.ContainerRequired = true

	return Registry{profiles: map[ProfileID]ProfileDescriptor{
		safe.ID: safe, lab.ID: lab, instrumented.ID: instrumented,
	}}
}

func ParseProfileID(value string) (ProfileID, error) {
	id := ProfileID(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := BuiltinRegistry().Lookup(id); !ok {
		return "", fmt.Errorf("unsupported browser profile %q", value)
	}
	return id, nil
}

func (registry Registry) Lookup(id ProfileID) (ProfileDescriptor, bool) {
	profile, ok := registry.profiles[id]
	return profile, ok
}

func (registry Registry) List() []ProfileDescriptor {
	profiles := make([]ProfileDescriptor, 0, len(registry.profiles))
	for _, profile := range registry.profiles {
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(left, right int) bool { return profiles[left].ID < profiles[right].ID })
	return profiles
}

func ValidateProfileDescriptor(profile ProfileDescriptor) error {
	expected, ok := BuiltinRegistry().Lookup(profile.ID)
	if !ok || !reflect.DeepEqual(profile, expected) {
		return errorsForProfile(profile.ID)
	}
	if profile.Authority != (RuntimeAuthority{}) {
		return fmt.Errorf("browser profile %q cannot grant runtime authority", profile.ID)
	}
	return nil
}

func ProfileFingerprint(profile ProfileDescriptor) (string, error) {
	if err := ValidateProfileDescriptor(profile); err != nil {
		return "", err
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("encode browser profile: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func errorsForProfile(id ProfileID) error {
	return fmt.Errorf("browser profile %q does not match the fixed Go registry", id)
}
