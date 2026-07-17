package skills

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/runmutation"
)

const (
	ExternalSelectionProtocolVersion    = "external_skill_selection.v1"
	DefaultExternalSelectionTokenBudget = 2048
	MaxExternalSelectionTokenBudget     = 4096
	MaxExternalSelectionItems           = 4
)

// ExternalSelectionItem pins one active installation and its verified object.
// Declared tools remain compatibility metadata and never become capabilities.
type ExternalSelectionItem struct {
	SelectionID              string
	Ordinal                  int
	InstallationID           string
	InstallationFingerprint  string
	InstallResultFingerprint string
	Name                     string
	Version                  string
	Surface                  domain.ExecutionSurface
	ContentSHA256            string
	ContentBytes             int
	TokenUpperBound          int
	ArchiveSHA256            string
	ArchiveBytes             int
	PackageFingerprint       string
	ObjectKey                string
	TrustClass               PackageTrustClass
	ToolDependencyCount      int
	SpecialistEligible       bool
}

type ExternalSelection struct {
	ID                        string
	RunID                     string
	MissionID                 string
	ModeSnapshotID            string
	ModeRevision              int64
	ProtocolVersion           string
	Surface                   domain.ExecutionSurface
	Profile                   domain.Profile
	TokenBudget               int
	TokenUpperBound           int
	ItemCount                 int
	Fingerprint               string
	RequestedBy               string
	OperatorConfirmed         bool
	ContextDeliveryAuthorized bool
	ToolCapabilityGrant       bool
	Items                     []ExternalSelectionItem
	CreatedAt                 time.Time
}

type ExternalSelectionOperation struct {
	KeyDigest          string
	RequestFingerprint string
	SelectionID        string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

type ResolveExternalSelectionRequest struct {
	SelectionID    string
	RunID          string
	MissionID      string
	ModeSnapshotID string
	ModeRevision   int64
	Surface        domain.ExecutionSurface
	Profile        domain.Profile
	Packages       []InstalledPackage
	SpecialistRef  string
	TokenBudget    int
	RequestedBy    string
	Confirmed      bool
	CreatedAt      time.Time
}

func ResolveExternalSelection(request ResolveExternalSelectionRequest) (ExternalSelection, error) {
	request.SelectionID = strings.TrimSpace(request.SelectionID)
	request.RunID = strings.TrimSpace(request.RunID)
	request.MissionID = strings.TrimSpace(request.MissionID)
	request.ModeSnapshotID = strings.TrimSpace(request.ModeSnapshotID)
	request.RequestedBy = strings.TrimSpace(request.RequestedBy)
	request.SpecialistRef = strings.TrimSpace(request.SpecialistRef)
	request.CreatedAt = request.CreatedAt.UTC()
	if !request.Confirmed {
		return ExternalSelection{}, errors.New("external Skill context selection requires explicit operator confirmation")
	}
	if len(request.Packages) == 0 || len(request.Packages) > MaxExternalSelectionItems {
		return ExternalSelection{}, fmt.Errorf("external Skill selection requires between 1 and %d packages", MaxExternalSelectionItems)
	}
	if request.TokenBudget <= 0 || request.TokenBudget > MaxExternalSelectionTokenBudget {
		return ExternalSelection{}, fmt.Errorf("external Skill selection token budget must be between 1 and %d", MaxExternalSelectionTokenBudget)
	}
	if !request.Surface.Valid() || request.ModeRevision <= 0 {
		return ExternalSelection{}, errors.New("external Skill selection mode is invalid")
	}
	profile, err := domain.ParseProfile(string(request.Profile))
	if err != nil || profile != request.Profile {
		return ExternalSelection{}, fmt.Errorf("invalid external Skill selection profile %q", request.Profile)
	}
	if request.Surface == domain.ExecutionSurfaceCyber && request.Profile != domain.ProfileScript {
		return ExternalSelection{}, errors.New("cyber external Skills are restricted to the script Profile")
	}

	packages := make([]InstalledPackage, len(request.Packages))
	for index, value := range request.Packages {
		packages[index] = CloneInstalledPackage(value)
	}
	sort.Slice(packages, func(left, right int) bool {
		return FormatInstalledPackageRef(packages[left].Installation.Name,
			packages[left].Installation.Version) < FormatInstalledPackageRef(
			packages[right].Installation.Name, packages[right].Installation.Version)
	})
	items := make([]ExternalSelectionItem, 0, len(packages))
	tokens := 0
	specialistFound := request.SpecialistRef == ""
	previousRef := ""
	for index, installed := range packages {
		if err := installed.Validate(); err != nil {
			return ExternalSelection{}, fmt.Errorf("selected external Skill package is invalid: %w", err)
		}
		installation, result := installed.Installation, installed.Result
		ref := FormatInstalledPackageRef(installation.Name, installation.Version)
		if ref == previousRef {
			return ExternalSelection{}, fmt.Errorf("selected external Skill %q is duplicated", ref)
		}
		if installed.Removal != nil {
			return ExternalSelection{}, fmt.Errorf("selected external Skill %q has been removed", ref)
		}
		if installation.Surface != request.Surface ||
			!containsProfile(installation.Manifest.Profiles, request.Profile) {
			return ExternalSelection{}, fmt.Errorf("selected external Skill %q is incompatible with %s/%s",
				ref, request.Surface, request.Profile)
		}
		specialist := request.SpecialistRef != "" && ref == request.SpecialistRef
		if specialist {
			specialistFound = true
		}
		item := ExternalSelectionItem{
			SelectionID: request.SelectionID, Ordinal: index + 1,
			InstallationID:           installation.ID,
			InstallationFingerprint:  installation.InstallationFingerprint,
			InstallResultFingerprint: result.ResultFingerprint,
			Name:                     installation.Name, Version: installation.Version, Surface: installation.Surface,
			ContentSHA256:   installation.Manifest.ContentSHA256,
			ContentBytes:    installation.Manifest.ContentBytes,
			TokenUpperBound: installation.Manifest.ContentTokenUpperBound,
			ArchiveSHA256:   installation.ArchiveSHA256, ArchiveBytes: installation.ArchiveBytes,
			PackageFingerprint: installation.PackageFingerprint, ObjectKey: result.ObjectKey,
			TrustClass:          installation.TrustClass,
			ToolDependencyCount: len(installation.Manifest.ToolDependencies),
			SpecialistEligible:  specialist,
		}
		if specialist && item.TokenUpperBound > MaxExternalSpecialistTokenBudget {
			return ExternalSelection{}, fmt.Errorf(
				"specialist external Skill %q exceeds the %d token hard limit",
				ref, MaxExternalSpecialistTokenBudget)
		}
		items = append(items, item)
		tokens += item.TokenUpperBound
		previousRef = ref
	}
	if !specialistFound {
		return ExternalSelection{}, fmt.Errorf("specialist external Skill %q is not selected", request.SpecialistRef)
	}
	if tokens > request.TokenBudget {
		return ExternalSelection{}, fmt.Errorf("external Skills require token upper bound %d, budget is %d", tokens, request.TokenBudget)
	}
	selection := ExternalSelection{
		ID: request.SelectionID, RunID: request.RunID, MissionID: request.MissionID,
		ModeSnapshotID: request.ModeSnapshotID, ModeRevision: request.ModeRevision,
		ProtocolVersion: ExternalSelectionProtocolVersion, Surface: request.Surface,
		Profile: request.Profile, TokenBudget: request.TokenBudget,
		TokenUpperBound: tokens, ItemCount: len(items), RequestedBy: request.RequestedBy,
		OperatorConfirmed: true, ContextDeliveryAuthorized: true,
		Items: items, CreatedAt: request.CreatedAt,
	}
	selection.Fingerprint = ExternalSelectionFingerprint(selection)
	if err := selection.Validate(); err != nil {
		return ExternalSelection{}, err
	}
	return CloneExternalSelection(selection), nil
}

func (s ExternalSelection) Validate() error {
	for _, value := range []string{s.ID, s.RunID, s.MissionID, s.ModeSnapshotID, s.RequestedBy} {
		if !validSelectionIdentity(value) {
			return errors.New("external Skill selection identities are invalid")
		}
	}
	if s.ProtocolVersion != ExternalSelectionProtocolVersion || s.ModeRevision <= 0 ||
		!s.Surface.Valid() || !s.OperatorConfirmed || !s.ContextDeliveryAuthorized ||
		s.ToolCapabilityGrant || !validUTC(s.CreatedAt) {
		return errors.New("external Skill selection protocol or capability boundary is invalid")
	}
	profile, err := domain.ParseProfile(string(s.Profile))
	if err != nil || profile != s.Profile ||
		(s.Surface == domain.ExecutionSurfaceCyber && s.Profile != domain.ProfileScript) {
		return errors.New("external Skill selection surface or Profile is invalid")
	}
	if s.TokenBudget <= 0 || s.TokenBudget > MaxExternalSelectionTokenBudget ||
		s.TokenUpperBound <= 0 || s.TokenUpperBound > s.TokenBudget ||
		len(s.Items) == 0 || len(s.Items) > MaxExternalSelectionItems ||
		s.ItemCount != len(s.Items) {
		return errors.New("external Skill selection bounds are invalid")
	}
	total := 0
	previousRef := ""
	specialistCount := 0
	for index, item := range s.Items {
		ref := FormatInstalledPackageRef(item.Name, item.Version)
		wantKey, keyErr := PackageObjectKey(item.ArchiveSHA256)
		if item.SelectionID != s.ID || item.Ordinal != index+1 ||
			!validPackageIdentity(item.InstallationID) ||
			!validSHA256(item.InstallationFingerprint) ||
			!validSHA256(item.InstallResultFingerprint) || !validName(item.Name) ||
			!validCoreVersion(item.Version) || item.Surface != s.Surface ||
			!validSHA256(item.ContentSHA256) || item.ContentBytes <= 0 ||
			item.ContentBytes > MaxContentBytes || item.TokenUpperBound != item.ContentBytes ||
			item.TokenUpperBound > MaxContentTokenUpperBound || keyErr != nil ||
			item.ArchiveBytes <= 0 || item.ArchiveBytes > MaxPackageArchiveBytes ||
			!validSHA256(item.PackageFingerprint) || item.ObjectKey != wantKey ||
			item.TrustClass != PackageTrustOperatorInstalledUntrusted ||
			item.ToolDependencyCount < 0 || item.ToolDependencyCount > MaxToolDependencies ||
			(previousRef != "" && previousRef >= ref) {
			return fmt.Errorf("external Skill selection item %d is invalid", index+1)
		}
		if item.SpecialistEligible {
			if item.TokenUpperBound > MaxExternalSpecialistTokenBudget {
				return fmt.Errorf("external Skill selection item %d exceeds the Specialist hard limit", index+1)
			}
			specialistCount++
		}
		total += item.TokenUpperBound
		previousRef = ref
	}
	if total != s.TokenUpperBound || specialistCount > 1 ||
		!validSHA256(s.Fingerprint) || s.Fingerprint != ExternalSelectionFingerprint(s) {
		return errors.New("external Skill selection accounting or fingerprint is invalid")
	}
	return nil
}

func (o ExternalSelectionOperation) Validate() error {
	if !validSHA256(o.KeyDigest) || !validSHA256(o.RequestFingerprint) ||
		!validSelectionIdentity(o.SelectionID) || !validSelectionIdentity(o.RunID) ||
		!validSelectionIdentity(o.RequestedBy) || !validUTC(o.CreatedAt) {
		return errors.New("external Skill selection operation is invalid")
	}
	return nil
}

func ExternalSelectionFingerprint(s ExternalSelection) string {
	parts := []string{ExternalSelectionProtocolVersion, s.ID, s.RunID, s.MissionID,
		s.ModeSnapshotID, strconv.FormatInt(s.ModeRevision, 10), string(s.Surface),
		string(s.Profile), strconv.Itoa(s.TokenBudget), strconv.Itoa(len(s.Items)),
		s.RequestedBy, s.CreatedAt.UTC().Format(time.RFC3339Nano),
		"operator_confirmed=true", "context_delivery=true", "tool_grant=false"}
	for _, item := range s.Items {
		parts = append(parts, strconv.Itoa(item.Ordinal), item.InstallationID,
			item.InstallationFingerprint, item.InstallResultFingerprint, item.Name,
			item.Version, string(item.Surface), item.ContentSHA256,
			strconv.Itoa(item.ContentBytes), strconv.Itoa(item.TokenUpperBound),
			item.ArchiveSHA256, strconv.Itoa(item.ArchiveBytes), item.PackageFingerprint,
			item.ObjectKey, string(item.TrustClass), strconv.Itoa(item.ToolDependencyCount),
			strconv.FormatBool(item.SpecialistEligible))
	}
	return runmutation.Fingerprint(parts...)
}

func ExternalSelectionRequestFingerprint(s ExternalSelection) string {
	parts := []string{"external_skill_selection_intent.v1", s.RunID, s.MissionID,
		s.ModeSnapshotID, strconv.FormatInt(s.ModeRevision, 10), string(s.Surface),
		string(s.Profile), strconv.Itoa(s.TokenBudget), s.RequestedBy,
		"operator_confirmed=true", "context_delivery=true", "tool_grant=false"}
	for _, item := range s.Items {
		parts = append(parts, item.InstallationID, item.InstallationFingerprint,
			item.InstallResultFingerprint, item.Name, item.Version, item.ContentSHA256,
			item.ArchiveSHA256, item.PackageFingerprint, item.ObjectKey,
			strconv.FormatBool(item.SpecialistEligible))
	}
	return runmutation.Fingerprint(parts...)
}

func CloneExternalSelection(value ExternalSelection) ExternalSelection {
	value.Items = append([]ExternalSelectionItem(nil), value.Items...)
	return value
}

func ExternalSpecialistItem(selection ExternalSelection) (ExternalSelectionItem, bool) {
	for _, item := range selection.Items {
		if item.SpecialistEligible {
			return item, true
		}
	}
	return ExternalSelectionItem{}, false
}
