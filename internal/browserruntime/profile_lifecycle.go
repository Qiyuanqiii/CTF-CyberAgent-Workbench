package browserruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	ProfileOwnershipProtocolVersion      = "browser_profile_ownership.v1"
	ProfileObservationProtocolVersion    = "browser_profile_observation.v1"
	ProfileReconciliationProtocolVersion = "browser_profile_reconciliation.v1"
	ProfileCleanupProtocolVersion        = "browser_profile_cleanup.v1"
	ProfileRuntimeRootName               = "browser-profiles"
	ProfileOwnerMarkerName               = ".prayu-browser-owner.json"
	ProfileRuntimeRootID                 = "prayu_browser_profiles"
	MaxProfileOwnershipGeneration        = 1_000_000
)

type DirectoryAuthority struct {
	Create bool `json:"create"`
	Read   bool `json:"read"`
	Write  bool `json:"write"`
	Rename bool `json:"rename"`
	Delete bool `json:"delete"`
}

// ProfileOwnershipPlan binds one disposable profile name to a SessionPlan and
// executable identity. It is a path plan, not a filesystem operation.
type ProfileOwnershipPlan struct {
	ProtocolVersion                string             `json:"protocol_version"`
	SessionPlanFingerprint         string             `json:"session_plan_fingerprint"`
	ExecutableIdentityFingerprint  string             `json:"executable_identity_fingerprint"`
	ProfileToken                   string             `json:"profile_token"`
	RootID                         string             `json:"root_id"`
	RootPath                       string             `json:"root_path"`
	DirectoryName                  string             `json:"directory_name"`
	DirectoryPath                  string             `json:"directory_path"`
	OwnerMarkerName                string             `json:"owner_marker_name"`
	OwnerToken                     string             `json:"owner_token"`
	MarkerPayloadSHA256            string             `json:"marker_payload_sha256"`
	Generation                     uint64             `json:"generation"`
	PreviousOwnershipFingerprint   string             `json:"previous_ownership_fingerprint,omitempty"`
	RecoveryObservationFingerprint string             `json:"recovery_observation_fingerprint,omitempty"`
	Disposable                     bool               `json:"disposable"`
	ExactPathOnly                  bool               `json:"exact_path_only"`
	CollisionCheckRequired         bool               `json:"collision_check_required"`
	RestartRecoveryRequired        bool               `json:"restart_recovery_required"`
	CleanupRequired                bool               `json:"cleanup_required"`
	PersonalProfileAllowed         bool               `json:"personal_profile_allowed"`
	ModelOwnsCleanup               bool               `json:"model_owns_cleanup"`
	ApplyBlocked                   bool               `json:"apply_blocked"`
	Authority                      DirectoryAuthority `json:"authority"`
	Fingerprint                    string             `json:"fingerprint"`
}

type ProfileDirectoryState string

const (
	ProfileDirectoryAbsent        ProfileDirectoryState = "absent"
	ProfileDirectoryOwnedActive   ProfileDirectoryState = "owned_active"
	ProfileDirectoryOwnedStale    ProfileDirectoryState = "owned_stale"
	ProfileDirectoryOwnedReleased ProfileDirectoryState = "owned_released"
	ProfileDirectoryForeign       ProfileDirectoryState = "foreign"
	ProfileDirectoryCorrupt       ProfileDirectoryState = "corrupt"
)

// ProfileDirectoryObservation is bounded metadata supplied by a future
// filesystem adapter. It cannot create, mutate, recover, or delete a directory.
type ProfileDirectoryObservation struct {
	ProtocolVersion            string                `json:"protocol_version"`
	OwnershipPlanFingerprint   string                `json:"ownership_plan_fingerprint"`
	DirectoryPath              string                `json:"directory_path"`
	State                      ProfileDirectoryState `json:"state"`
	ObservedOwnerToken         string                `json:"observed_owner_token,omitempty"`
	ObservedGeneration         uint64                `json:"observed_generation"`
	ObservedMarkerSHA256       string                `json:"observed_marker_sha256,omitempty"`
	MetadataOnly               bool                  `json:"metadata_only"`
	FilesystemMutationOccurred bool                  `json:"filesystem_mutation_occurred"`
	Fingerprint                string                `json:"fingerprint"`
}

type ProfileReconciliationDecision string

const (
	ProfileDecisionCreateCandidate  ProfileReconciliationDecision = "create_candidate"
	ProfileDecisionWaitForOwner     ProfileReconciliationDecision = "wait_for_active_owner"
	ProfileDecisionRecoverCandidate ProfileReconciliationDecision = "recover_stale_owner_candidate"
	ProfileDecisionCleanupCandidate ProfileReconciliationDecision = "cleanup_released_owner_candidate"
	ProfileDecisionRefuseForeign    ProfileReconciliationDecision = "refuse_foreign_owner"
	ProfileDecisionRefuseCorrupt    ProfileReconciliationDecision = "refuse_corrupt_marker"
)

type ProfileReconciliationPlan struct {
	ProtocolVersion           string                        `json:"protocol_version"`
	OwnershipPlanFingerprint  string                        `json:"ownership_plan_fingerprint"`
	ObservationFingerprint    string                        `json:"observation_fingerprint"`
	DirectoryPath             string                        `json:"directory_path"`
	Decision                  ProfileReconciliationDecision `json:"decision"`
	CollisionDetected         bool                          `json:"collision_detected"`
	ExactOwnerVerified        bool                          `json:"exact_owner_verified"`
	RestartRecoveryCandidate  bool                          `json:"restart_recovery_candidate"`
	CreateCandidate           bool                          `json:"create_candidate"`
	CleanupCandidate          bool                          `json:"cleanup_candidate"`
	ForeignDirectoryRefused   bool                          `json:"foreign_directory_refused"`
	CorruptMarkerRefused      bool                          `json:"corrupt_marker_refused"`
	ProcessQuiescenceRequired bool                          `json:"process_quiescence_required"`
	FilesystemRecheckRequired bool                          `json:"filesystem_recheck_required"`
	ApplyBlocked              bool                          `json:"apply_blocked"`
	Authority                 DirectoryAuthority            `json:"authority"`
	Fingerprint               string                        `json:"fingerprint"`
}

// ProfileCleanupPlan names exactly one released, owned directory. It is still
// blocked and cannot be interpreted as permission to remove that directory.
type ProfileCleanupPlan struct {
	ProtocolVersion           string             `json:"protocol_version"`
	OwnershipPlanFingerprint  string             `json:"ownership_plan_fingerprint"`
	ObservationFingerprint    string             `json:"observation_fingerprint"`
	DirectoryPath             string             `json:"directory_path"`
	OwnerToken                string             `json:"owner_token"`
	Generation                uint64             `json:"generation"`
	MarkerSHA256              string             `json:"marker_sha256"`
	ExactOwnerRequired        bool               `json:"exact_owner_required"`
	ReleasedStateRequired     bool               `json:"released_state_required"`
	ProcessQuiescenceRequired bool               `json:"process_quiescence_required"`
	FilesystemRecheckRequired bool               `json:"filesystem_recheck_required"`
	RecursiveWildcardAllowed  bool               `json:"recursive_wildcard_allowed"`
	ModelOwnsCleanup          bool               `json:"model_owns_cleanup"`
	DeleteBlocked             bool               `json:"delete_blocked"`
	Authority                 DirectoryAuthority `json:"authority"`
	Fingerprint               string             `json:"fingerprint"`
}

func BuildProfileOwnershipPlan(session SessionPlan, executable BrowserExecutableIdentity,
	runtimeRoot string,
) (ProfileOwnershipPlan, error) {
	if err := session.Validate(); err != nil {
		return ProfileOwnershipPlan{}, fmt.Errorf("validate browser session plan: %w", err)
	}
	if err := ValidateBrowserExecutableIdentity(executable); err != nil {
		return ProfileOwnershipPlan{}, fmt.Errorf("validate browser executable identity: %w", err)
	}
	if !validProfileRuntimeRoot(runtimeRoot) {
		return ProfileOwnershipPlan{}, errors.New("browser profile runtime root must be a canonical dedicated directory")
	}
	directoryName := "profile-" + session.ProfileToken
	directoryPath := filepath.Join(runtimeRoot, directoryName)
	if !pathWithinRoot(runtimeRoot, directoryPath) || filepath.Base(directoryPath) != directoryName {
		return ProfileOwnershipPlan{}, errors.New("browser profile directory escaped its runtime root")
	}
	plan := ProfileOwnershipPlan{
		ProtocolVersion:               ProfileOwnershipProtocolVersion,
		SessionPlanFingerprint:        session.Fingerprint,
		ExecutableIdentityFingerprint: executable.Fingerprint,
		ProfileToken:                  session.ProfileToken, RootID: ProfileRuntimeRootID,
		RootPath: runtimeRoot, DirectoryName: directoryName, DirectoryPath: directoryPath,
		OwnerMarkerName: ProfileOwnerMarkerName, Generation: 1,
		Disposable: true, ExactPathOnly: true, CollisionCheckRequired: true,
		RestartRecoveryRequired: true, CleanupRequired: true, ApplyBlocked: true,
	}
	plan.OwnerToken = profileOwnerToken(plan)
	plan.MarkerPayloadSHA256 = profileMarkerPayloadSHA256(plan)
	var err error
	plan.Fingerprint, err = profileOwnershipFingerprint(plan)
	if err != nil {
		return ProfileOwnershipPlan{}, err
	}
	if err := ValidateProfileOwnershipPlan(plan, session, executable); err != nil {
		return ProfileOwnershipPlan{}, err
	}
	return plan, nil
}

func ValidateProfileOwnershipPlan(plan ProfileOwnershipPlan, session SessionPlan,
	executable BrowserExecutableIdentity,
) error {
	if err := session.Validate(); err != nil {
		return err
	}
	if err := ValidateBrowserExecutableIdentity(executable); err != nil {
		return err
	}
	if err := validateProfileOwnershipStructure(plan); err != nil {
		return err
	}
	if plan.SessionPlanFingerprint != session.Fingerprint ||
		plan.ExecutableIdentityFingerprint != executable.Fingerprint ||
		plan.ProfileToken != session.ProfileToken {
		return errors.New("browser profile ownership plan does not match its session or executable")
	}
	return nil
}

// BuildRecoveredProfileOwnershipPlan advances an exact stale owner to a new
// generation. The path stays fixed, while the owner token and marker digest
// change so a previous worker cannot commit as the recovered owner.
func BuildRecoveredProfileOwnershipPlan(previous ProfileOwnershipPlan,
	stale ProfileDirectoryObservation, session SessionPlan,
	executable BrowserExecutableIdentity,
) (ProfileOwnershipPlan, error) {
	if err := ValidateProfileOwnershipPlan(previous, session, executable); err != nil {
		return ProfileOwnershipPlan{}, err
	}
	if err := ValidateProfileDirectoryObservation(stale, previous); err != nil {
		return ProfileOwnershipPlan{}, err
	}
	if stale.State != ProfileDirectoryOwnedStale ||
		stale.ObservedOwnerToken != previous.OwnerToken ||
		stale.ObservedGeneration != previous.Generation ||
		stale.ObservedMarkerSHA256 != previous.MarkerPayloadSHA256 ||
		previous.Generation >= MaxProfileOwnershipGeneration {
		return ProfileOwnershipPlan{}, errors.New("browser profile recovery requires an exact stale owner below the generation bound")
	}
	recovered := previous
	recovered.Generation++
	recovered.PreviousOwnershipFingerprint = previous.Fingerprint
	recovered.RecoveryObservationFingerprint = stale.Fingerprint
	recovered.OwnerToken = ""
	recovered.MarkerPayloadSHA256 = ""
	recovered.Fingerprint = ""
	recovered.OwnerToken = profileOwnerToken(recovered)
	recovered.MarkerPayloadSHA256 = profileMarkerPayloadSHA256(recovered)
	var err error
	recovered.Fingerprint, err = profileOwnershipFingerprint(recovered)
	if err != nil {
		return ProfileOwnershipPlan{}, err
	}
	if err := ValidateProfileOwnershipPlan(recovered, session, executable); err != nil {
		return ProfileOwnershipPlan{}, err
	}
	return recovered, nil
}

func BuildProfileDirectoryObservation(ownership ProfileOwnershipPlan,
	state ProfileDirectoryState, observedOwnerToken string, observedGeneration uint64,
	observedMarkerSHA256 string,
) (ProfileDirectoryObservation, error) {
	observation := ProfileDirectoryObservation{
		ProtocolVersion:          ProfileObservationProtocolVersion,
		OwnershipPlanFingerprint: ownership.Fingerprint,
		DirectoryPath:            ownership.DirectoryPath, State: state,
		ObservedOwnerToken: observedOwnerToken, ObservedGeneration: observedGeneration,
		ObservedMarkerSHA256: observedMarkerSHA256, MetadataOnly: true,
	}
	var err error
	observation.Fingerprint, err = profileObservationFingerprint(observation)
	if err != nil {
		return ProfileDirectoryObservation{}, err
	}
	if err := ValidateProfileDirectoryObservation(observation, ownership); err != nil {
		return ProfileDirectoryObservation{}, err
	}
	return observation, nil
}

func ValidateProfileDirectoryObservation(observation ProfileDirectoryObservation,
	ownership ProfileOwnershipPlan,
) error {
	if err := validateProfileOwnershipStructure(ownership); err != nil {
		return err
	}
	if observation.ProtocolVersion != ProfileObservationProtocolVersion ||
		observation.OwnershipPlanFingerprint != ownership.Fingerprint ||
		observation.DirectoryPath != ownership.DirectoryPath || !observation.MetadataOnly ||
		observation.FilesystemMutationOccurred {
		return errors.New("browser profile observation is not inert or ownership-bound")
	}
	switch observation.State {
	case ProfileDirectoryAbsent:
		if observation.ObservedOwnerToken != "" || observation.ObservedGeneration != 0 ||
			observation.ObservedMarkerSHA256 != "" {
			return errors.New("absent browser profile observation contains owner metadata")
		}
	case ProfileDirectoryOwnedActive, ProfileDirectoryOwnedStale,
		ProfileDirectoryOwnedReleased, ProfileDirectoryForeign:
		if !validSHA256(observation.ObservedOwnerToken) || observation.ObservedGeneration == 0 ||
			!validSHA256(observation.ObservedMarkerSHA256) {
			return errors.New("browser profile owner observation is incomplete")
		}
	case ProfileDirectoryCorrupt:
		if observation.ObservedOwnerToken != "" || observation.ObservedGeneration != 0 ||
			observation.ObservedMarkerSHA256 != "" {
			return errors.New("corrupt browser profile observation must not trust marker metadata")
		}
	default:
		return fmt.Errorf("unsupported browser profile directory state %q", observation.State)
	}
	expected, err := profileObservationFingerprint(observation)
	if err != nil || observation.Fingerprint != expected {
		return errors.New("browser profile observation fingerprint mismatch")
	}
	return nil
}

func BuildProfileReconciliationPlan(ownership ProfileOwnershipPlan,
	observation ProfileDirectoryObservation,
) (ProfileReconciliationPlan, error) {
	if err := ValidateProfileDirectoryObservation(observation, ownership); err != nil {
		return ProfileReconciliationPlan{}, err
	}
	plan, err := buildProfileReconciliationPlanUnchecked(ownership, observation)
	if err != nil {
		return ProfileReconciliationPlan{}, err
	}
	if err := ValidateProfileReconciliationPlan(plan, ownership, observation); err != nil {
		return ProfileReconciliationPlan{}, err
	}
	return plan, nil
}

func ValidateProfileReconciliationPlan(plan ProfileReconciliationPlan,
	ownership ProfileOwnershipPlan, observation ProfileDirectoryObservation,
) error {
	if err := ValidateProfileDirectoryObservation(observation, ownership); err != nil {
		return err
	}
	rebuilt, err := buildProfileReconciliationPlanUnchecked(ownership, observation)
	if err != nil || !reflect.DeepEqual(plan, rebuilt) {
		return errors.New("browser profile reconciliation plan does not match its observation")
	}
	return nil
}

func BuildProfileCleanupPlan(ownership ProfileOwnershipPlan,
	observation ProfileDirectoryObservation,
) (ProfileCleanupPlan, error) {
	if err := ValidateProfileDirectoryObservation(observation, ownership); err != nil {
		return ProfileCleanupPlan{}, err
	}
	if observation.State != ProfileDirectoryOwnedReleased ||
		observation.ObservedOwnerToken != ownership.OwnerToken ||
		observation.ObservedGeneration != ownership.Generation ||
		observation.ObservedMarkerSHA256 != ownership.MarkerPayloadSHA256 {
		return ProfileCleanupPlan{}, errors.New("only an exact released browser profile may produce a cleanup candidate")
	}
	plan := ProfileCleanupPlan{
		ProtocolVersion:          ProfileCleanupProtocolVersion,
		OwnershipPlanFingerprint: ownership.Fingerprint,
		ObservationFingerprint:   observation.Fingerprint,
		DirectoryPath:            ownership.DirectoryPath, OwnerToken: ownership.OwnerToken,
		Generation: ownership.Generation, MarkerSHA256: ownership.MarkerPayloadSHA256,
		ExactOwnerRequired: true, ReleasedStateRequired: true,
		ProcessQuiescenceRequired: true, FilesystemRecheckRequired: true,
		DeleteBlocked: true,
	}
	var err error
	plan.Fingerprint, err = profileCleanupFingerprint(plan)
	if err != nil {
		return ProfileCleanupPlan{}, err
	}
	if err := ValidateProfileCleanupPlan(plan, ownership, observation); err != nil {
		return ProfileCleanupPlan{}, err
	}
	return plan, nil
}

func ValidateProfileCleanupPlan(plan ProfileCleanupPlan, ownership ProfileOwnershipPlan,
	observation ProfileDirectoryObservation,
) error {
	if err := ValidateProfileDirectoryObservation(observation, ownership); err != nil {
		return err
	}
	if observation.State != ProfileDirectoryOwnedReleased ||
		observation.ObservedOwnerToken != ownership.OwnerToken ||
		observation.ObservedGeneration != ownership.Generation ||
		observation.ObservedMarkerSHA256 != ownership.MarkerPayloadSHA256 ||
		plan.ProtocolVersion != ProfileCleanupProtocolVersion ||
		plan.OwnershipPlanFingerprint != ownership.Fingerprint ||
		plan.ObservationFingerprint != observation.Fingerprint ||
		plan.DirectoryPath != ownership.DirectoryPath || plan.OwnerToken != ownership.OwnerToken ||
		plan.Generation != ownership.Generation || plan.MarkerSHA256 != ownership.MarkerPayloadSHA256 ||
		!plan.ExactOwnerRequired || !plan.ReleasedStateRequired ||
		!plan.ProcessQuiescenceRequired || !plan.FilesystemRecheckRequired ||
		plan.RecursiveWildcardAllowed || plan.ModelOwnsCleanup || !plan.DeleteBlocked ||
		plan.Authority != (DirectoryAuthority{}) {
		return errors.New("browser profile cleanup plan lost an exact blocked boundary")
	}
	expected, err := profileCleanupFingerprint(plan)
	if err != nil || plan.Fingerprint != expected {
		return errors.New("browser profile cleanup fingerprint mismatch")
	}
	return nil
}

func buildProfileReconciliationPlanUnchecked(ownership ProfileOwnershipPlan,
	observation ProfileDirectoryObservation,
) (ProfileReconciliationPlan, error) {
	exactOwner := observation.ObservedOwnerToken == ownership.OwnerToken &&
		observation.ObservedGeneration == ownership.Generation &&
		observation.ObservedMarkerSHA256 == ownership.MarkerPayloadSHA256
	plan := ProfileReconciliationPlan{
		ProtocolVersion:          ProfileReconciliationProtocolVersion,
		OwnershipPlanFingerprint: ownership.Fingerprint,
		ObservationFingerprint:   observation.Fingerprint,
		DirectoryPath:            ownership.DirectoryPath, ExactOwnerVerified: exactOwner,
		FilesystemRecheckRequired: true, ApplyBlocked: true,
	}
	switch observation.State {
	case ProfileDirectoryAbsent:
		plan.Decision, plan.CreateCandidate = ProfileDecisionCreateCandidate, true
	case ProfileDirectoryOwnedActive:
		plan.CollisionDetected = true
		if exactOwner {
			plan.Decision, plan.ProcessQuiescenceRequired = ProfileDecisionWaitForOwner, true
		} else {
			plan.Decision, plan.ForeignDirectoryRefused = ProfileDecisionRefuseForeign, true
		}
	case ProfileDirectoryOwnedStale:
		plan.CollisionDetected = true
		if exactOwner {
			plan.Decision = ProfileDecisionRecoverCandidate
			plan.RestartRecoveryCandidate, plan.ProcessQuiescenceRequired = true, true
		} else {
			plan.Decision, plan.ForeignDirectoryRefused = ProfileDecisionRefuseForeign, true
		}
	case ProfileDirectoryOwnedReleased:
		plan.CollisionDetected = true
		if exactOwner {
			plan.Decision = ProfileDecisionCleanupCandidate
			plan.CleanupCandidate, plan.ProcessQuiescenceRequired = true, true
		} else {
			plan.Decision, plan.ForeignDirectoryRefused = ProfileDecisionRefuseForeign, true
		}
	case ProfileDirectoryForeign:
		plan.CollisionDetected = true
		plan.Decision, plan.ForeignDirectoryRefused = ProfileDecisionRefuseForeign, true
	case ProfileDirectoryCorrupt:
		plan.CollisionDetected = true
		plan.Decision, plan.CorruptMarkerRefused = ProfileDecisionRefuseCorrupt, true
	default:
		return ProfileReconciliationPlan{}, errors.New("unsupported browser profile observation")
	}
	var err error
	plan.Fingerprint, err = profileReconciliationFingerprint(plan)
	return plan, err
}

func validateProfileOwnershipStructure(plan ProfileOwnershipPlan) error {
	if plan.ProtocolVersion != ProfileOwnershipProtocolVersion ||
		!validSHA256(plan.SessionPlanFingerprint) ||
		!validSHA256(plan.ExecutableIdentityFingerprint) || !validSHA256(plan.ProfileToken) ||
		plan.RootID != ProfileRuntimeRootID || !validProfileRuntimeRoot(plan.RootPath) ||
		plan.DirectoryName != "profile-"+plan.ProfileToken ||
		plan.DirectoryPath != filepath.Join(plan.RootPath, plan.DirectoryName) ||
		!pathWithinRoot(plan.RootPath, plan.DirectoryPath) ||
		plan.OwnerMarkerName != ProfileOwnerMarkerName || plan.Generation == 0 ||
		plan.Generation > MaxProfileOwnershipGeneration ||
		!plan.Disposable || !plan.ExactPathOnly || !plan.CollisionCheckRequired ||
		!plan.RestartRecoveryRequired || !plan.CleanupRequired ||
		plan.PersonalProfileAllowed || plan.ModelOwnsCleanup || !plan.ApplyBlocked ||
		plan.Authority != (DirectoryAuthority{}) {
		return errors.New("browser profile ownership plan lost a fixed safety boundary")
	}
	if plan.Generation == 1 {
		if plan.PreviousOwnershipFingerprint != "" || plan.RecoveryObservationFingerprint != "" {
			return errors.New("initial browser profile ownership contains recovery ancestry")
		}
	} else if !validSHA256(plan.PreviousOwnershipFingerprint) ||
		!validSHA256(plan.RecoveryObservationFingerprint) {
		return errors.New("recovered browser profile ownership lacks bounded ancestry")
	}
	if plan.OwnerToken != profileOwnerToken(plan) ||
		plan.MarkerPayloadSHA256 != profileMarkerPayloadSHA256(plan) {
		return errors.New("browser profile ownership token or marker mismatch")
	}
	expected, err := profileOwnershipFingerprint(plan)
	if err != nil || plan.Fingerprint != expected {
		return errors.New("browser profile ownership fingerprint mismatch")
	}
	return nil
}

func validProfileRuntimeRoot(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value &&
		filepath.Base(value) == ProfileRuntimeRootName
}

func profileOwnerToken(plan ProfileOwnershipPlan) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"browser-profile-owner.v1", plan.SessionPlanFingerprint,
		plan.ExecutableIdentityFingerprint, plan.ProfileToken, plan.RootID,
		plan.RootPath, plan.DirectoryPath, fmt.Sprint(plan.Generation),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func profileMarkerPayloadSHA256(plan ProfileOwnershipPlan) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"browser-profile-marker.v1", plan.OwnerToken, plan.SessionPlanFingerprint,
		plan.ExecutableIdentityFingerprint, plan.ProfileToken,
		fmt.Sprint(plan.Generation),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func profileOwnershipFingerprint(value ProfileOwnershipPlan) (string, error) {
	copyValue := value
	copyValue.Fingerprint = ""
	return fingerprintJSON(copyValue, "browser profile ownership")
}

func profileObservationFingerprint(value ProfileDirectoryObservation) (string, error) {
	copyValue := value
	copyValue.Fingerprint = ""
	return fingerprintJSON(copyValue, "browser profile observation")
}

func profileReconciliationFingerprint(value ProfileReconciliationPlan) (string, error) {
	copyValue := value
	copyValue.Fingerprint = ""
	return fingerprintJSON(copyValue, "browser profile reconciliation")
}

func profileCleanupFingerprint(value ProfileCleanupPlan) (string, error) {
	copyValue := value
	copyValue.Fingerprint = ""
	return fingerprintJSON(copyValue, "browser profile cleanup")
}

func fingerprintJSON(value any, label string) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", label, err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
