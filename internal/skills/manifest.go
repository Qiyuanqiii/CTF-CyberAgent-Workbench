package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/toolgateway"
)

const (
	ProtocolVersion           = "skill.v1"
	MaxManifestBytes          = 16 * 1024
	MaxContentBytes           = 32 * 1024
	MaxContentTokenUpperBound = 4096
	MaxSkills                 = 64
	MaxNameBytes              = 64
	MaxDescriptionRunes       = 512
	MaxProfiles               = 4
	MaxToolDependencies       = 8
	MaxContentPathBytes       = 256
	MaxContentPathDepth       = 8
)

// Manifest is metadata only. ToolDependencies declare prerequisites; they do
// not grant a Skill or an Agent permission to invoke those tools.
type Manifest struct {
	Protocol               string                 `json:"protocol"`
	Name                   string                 `json:"name"`
	Version                string                 `json:"version"`
	Description            string                 `json:"description"`
	Profiles               []domain.Profile       `json:"profiles"`
	ToolDependencies       []toolgateway.ToolName `json:"tool_dependencies"`
	ContentPath            string                 `json:"content_path"`
	ContentSHA256          string                 `json:"content_sha256"`
	ContentBytes           int                    `json:"content_bytes"`
	ContentTokenUpperBound int                    `json:"content_token_upper_bound"`
}

func (m Manifest) Validate(content []byte) error {
	if m.Protocol != ProtocolVersion {
		return fmt.Errorf("unsupported skill protocol %q", m.Protocol)
	}
	if !validName(m.Name) {
		return fmt.Errorf("invalid skill name %q", m.Name)
	}
	if !validCoreVersion(m.Version) {
		return fmt.Errorf("invalid skill version %q", m.Version)
	}
	if err := validateDescription(m.Description); err != nil {
		return err
	}
	if err := validateProfiles(m.Profiles); err != nil {
		return err
	}
	if err := validateToolDependencies(m.Profiles, m.ToolDependencies); err != nil {
		return err
	}
	if err := validateContentPath(m.ContentPath); err != nil {
		return err
	}
	if len(content) == 0 || len(content) > MaxContentBytes || !utf8.Valid(content) {
		return fmt.Errorf("skill content must be valid UTF-8 between 1 and %d bytes", MaxContentBytes)
	}
	for _, current := range string(content) {
		if current == 0 || (unicode.IsControl(current) && current != '\n' && current != '\r' && current != '\t') {
			return errors.New("skill content contains a forbidden control character")
		}
	}
	if m.ContentBytes != len(content) {
		return fmt.Errorf("skill content_bytes is %d, want %d", m.ContentBytes, len(content))
	}
	tokenUpperBound := ContentTokenUpperBound(content)
	if tokenUpperBound > MaxContentTokenUpperBound {
		return fmt.Errorf("skill content token upper bound exceeds %d", MaxContentTokenUpperBound)
	}
	if m.ContentTokenUpperBound != tokenUpperBound {
		return fmt.Errorf("skill content_token_upper_bound is %d, want %d", m.ContentTokenUpperBound, tokenUpperBound)
	}
	if !validSHA256(m.ContentSHA256) {
		return errors.New("skill content_sha256 must be 64 lowercase hexadecimal characters")
	}
	digest := sha256.Sum256(content)
	if m.ContentSHA256 != hex.EncodeToString(digest[:]) {
		return errors.New("skill content_sha256 does not match content")
	}
	return nil
}

// ContentTokenUpperBound uses UTF-8 bytes as a deterministic conservative
// accounting bound until model-specific tokenizers are introduced.
func ContentTokenUpperBound(content []byte) int {
	return len(content)
}

func validName(value string) bool {
	if value == "" || len(value) > MaxNameBytes || !utf8.ValidString(value) || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, current := range []byte(value) {
		if (current >= 'a' && current <= 'z') || (current >= '0' && current <= '9') || current == '-' {
			continue
		}
		return false
	}
	return value[len(value)-1] != '-'
}

func validCoreVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 9 || (len(part) > 1 && part[0] == '0') {
			return false
		}
		for _, current := range []byte(part) {
			if current < '0' || current > '9' {
				return false
			}
		}
	}
	return true
}

func validateDescription(value string) error {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > MaxDescriptionRunes {
		return fmt.Errorf("skill description must contain between 1 and %d normalized UTF-8 characters", MaxDescriptionRunes)
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return errors.New("skill description cannot contain control characters")
		}
	}
	return nil
}

func validateProfiles(profiles []domain.Profile) error {
	if len(profiles) == 0 || len(profiles) > MaxProfiles {
		return fmt.Errorf("skill profiles must contain between 1 and %d entries", MaxProfiles)
	}
	previous := ""
	for _, profile := range profiles {
		parsed, err := domain.ParseProfile(string(profile))
		if err != nil || parsed != profile {
			return fmt.Errorf("invalid skill profile %q", profile)
		}
		if previous != "" && previous >= string(profile) {
			return errors.New("skill profiles must be unique and sorted")
		}
		previous = string(profile)
	}
	return nil
}

func validateToolDependencies(profiles []domain.Profile, dependencies []toolgateway.ToolName) error {
	if dependencies == nil {
		return errors.New("skill tool_dependencies is required")
	}
	if len(dependencies) > MaxToolDependencies {
		return fmt.Errorf("skill tool_dependencies exceeds %d entries", MaxToolDependencies)
	}
	previous := ""
	for _, dependency := range dependencies {
		if !dependency.Valid() {
			return fmt.Errorf("unsupported skill tool dependency %q", dependency)
		}
		if previous != "" && previous >= string(dependency) {
			return errors.New("skill tool_dependencies must be unique and sorted")
		}
		for _, profile := range profiles {
			if !toolCompatibleWithProfile(profile, dependency) {
				return fmt.Errorf("skill tool dependency %q is incompatible with profile %q", dependency, profile)
			}
		}
		previous = string(dependency)
	}
	return nil
}

func toolCompatibleWithProfile(profile domain.Profile, dependency toolgateway.ToolName) bool {
	readOnly := dependency == toolgateway.ListWorkspaceTool || dependency == toolgateway.ReadFileTool
	switch profile {
	case domain.ProfileCode:
		return readOnly || dependency == toolgateway.ReplaceFileTool
	case domain.ProfileReview, domain.ProfileLearn:
		return readOnly
	case domain.ProfileScript:
		return readOnly || dependency == toolgateway.ScriptProcessTool
	default:
		return false
	}
}

func validateContentPath(value string) error {
	if value == "" || len(value) > MaxContentPathBytes || !utf8.ValidString(value) || strings.Contains(value, "\\") ||
		!fs.ValidPath(value) || value == "." || !strings.HasSuffix(value, ".md") {
		return errors.New("skill content_path must be a relative slash-separated Markdown path")
	}
	if len(strings.Split(value, "/")) > MaxContentPathDepth {
		return fmt.Errorf("skill content_path exceeds %d path segments", MaxContentPathDepth)
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.Profiles = slices.Clone(manifest.Profiles)
	manifest.ToolDependencies = slices.Clone(manifest.ToolDependencies)
	return manifest
}
