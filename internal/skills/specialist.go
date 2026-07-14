package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/runmutation"
)

const (
	SpecialistContextProtocolVersion    = "specialist_skill_context.v1"
	DefaultSpecialistContextTokenBudget = 1024
	MaxSpecialistContextTokenBudget     = 2048
	MaxSpecialistContextItems           = 1
)

// SpecialistContextAssembly is an in-memory, attempt-bound subset of a Run's
// immutable root Skill selection. Content must never be persisted or copied
// into Run events.
type SpecialistContextAssembly struct {
	ProtocolVersion            string
	ParentSelectionID          string
	ParentSelectionFingerprint string
	RunID                      string
	MissionID                  string
	AgentID                    string
	ParentAgentID              string
	AgentAttemptID             string
	Turn                       int64
	ModeSnapshotID             string
	ModeRevision               int64
	Surface                    domain.ExecutionSurface
	Profile                    domain.Profile
	AssignmentFingerprint      string
	Fingerprint                string
	TokenBudget                int
	TokenUpperBound            int
	ItemCount                  int
	RedactionCount             int
	Items                      []ContextItem
}

type SpecialistContextPreparationRequest struct {
	RunID                      string
	MissionID                  string
	AgentID                    string
	ParentAgentID              string
	AgentAttemptID             string
	Turn                       int64
	ParentSelectionID          string
	ProtocolVersion            string
	ParentSelectionFingerprint string
	ModeSnapshotID             string
	ModeRevision               int64
	Surface                    domain.ExecutionSurface
	Profile                    domain.Profile
	AssignmentFingerprint      string
	ContextFingerprint         string
	ItemCount                  int
	TokenBudget                int
	TokenUpperBound            int
	RedactionCount             int
}

type SpecialistContextPreparation struct {
	ID string
	SpecialistContextPreparationRequest
	PreparedAt time.Time
	Recovered  bool
}

type SpecialistContextCommit struct {
	PreparationID  string
	RunID          string
	AgentAttemptID string
	ModelAttempt   int
	CommittedAt    time.Time
}

// AssembleSpecialistContext derives the smallest compatible child guidance
// subset. The child assignment is provenance only: it cannot select a Skill,
// widen the parent selection, or turn guidance into a capability grant.
func (r *Registry) AssembleSpecialistContext(parent Selection,
	mode domain.RunModeSnapshot, child domain.AgentNode, attempt domain.AgentAttempt,
	tokenBudget int,
) (SpecialistContextAssembly, error) {
	if r == nil {
		return SpecialistContextAssembly{}, errors.New("skill registry is required")
	}
	if err := parent.Validate(); err != nil {
		return SpecialistContextAssembly{}, fmt.Errorf("invalid persisted parent Skill selection: %w", err)
	}
	if err := mode.Validate(); err != nil {
		return SpecialistContextAssembly{}, fmt.Errorf("invalid Run mode snapshot: %w", err)
	}
	if err := child.Validate(); err != nil {
		return SpecialistContextAssembly{}, fmt.Errorf("invalid Specialist Agent: %w", err)
	}
	if err := attempt.Validate(); err != nil {
		return SpecialistContextAssembly{}, fmt.Errorf("invalid Specialist attempt: %w", err)
	}
	if tokenBudget == 0 {
		tokenBudget = DefaultSpecialistContextTokenBudget
	}
	if tokenBudget <= 0 || tokenBudget > MaxSpecialistContextTokenBudget {
		return SpecialistContextAssembly{}, fmt.Errorf(
			"specialist Skill context token budget must be between 1 and %d",
			MaxSpecialistContextTokenBudget)
	}
	if parent.RunID != mode.RunID || parent.MissionID != mode.MissionID ||
		parent.Profile != mode.Profile || child.RunID != mode.RunID ||
		child.Profile != mode.Profile || child.Role != domain.AgentRoleSpecialist ||
		child.Status != domain.AgentRunning || child.ParentID == "" ||
		child.ActiveAttemptID != attempt.ID || attempt.RunID != child.RunID ||
		attempt.AgentID != child.ID || attempt.ParentAgentID != child.ParentID ||
		attempt.Status != domain.AgentAttemptRunning || attempt.Turn != child.TurnsUsed {
		return SpecialistContextAssembly{}, errors.New(
			"specialist Skill context does not match its parent selection, mode, or active attempt")
	}
	if !slices.Contains(child.Skills, "model.chat") {
		return SpecialistContextAssembly{}, errors.New(
			"specialist model execution requires the delegated model.chat capability")
	}

	assignmentFingerprint := SpecialistAssignmentFingerprint(child)
	items := make([]SelectionItem, 0, MaxSpecialistContextItems)
	if selected, found := SpecialistSelectionItem(parent, mode.Surface, mode.Profile); found {
		items = append(items, selected)
	}

	assembly := SpecialistContextAssembly{
		ProtocolVersion:   SpecialistContextProtocolVersion,
		ParentSelectionID: parent.ID, ParentSelectionFingerprint: parent.Fingerprint,
		RunID: parent.RunID, MissionID: parent.MissionID, AgentID: child.ID,
		ParentAgentID: child.ParentID, AgentAttemptID: attempt.ID, Turn: attempt.Turn,
		ModeSnapshotID: mode.ID, ModeRevision: mode.Revision, Surface: mode.Surface,
		Profile: mode.Profile, AssignmentFingerprint: assignmentFingerprint,
		TokenBudget: tokenBudget,
	}
	if len(items) > 0 {
		derived := CloneSelection(parent)
		derived.Items = items
		derived.ItemCount = len(items)
		derived.TokenBudget = tokenBudget
		derived.TokenUpperBound = items[0].TokenUpperBound
		derived.Fingerprint = SelectionFingerprint(derived)
		context, err := r.AssembleContext(derived)
		if err != nil {
			return SpecialistContextAssembly{}, fmt.Errorf(
				"assemble Specialist Skill subset: %w", err)
		}
		assembly.Items = append([]ContextItem(nil), context.Items...)
		assembly.ItemCount = context.ItemCount
		assembly.TokenUpperBound = context.TokenUpperBound
		assembly.RedactionCount = context.RedactionCount
	}
	assembly.Fingerprint = SpecialistContextFingerprint(assembly)
	if err := assembly.Validate(); err != nil {
		return SpecialistContextAssembly{}, err
	}
	return assembly, nil
}

func (a SpecialistContextAssembly) Validate() error {
	if a.ProtocolVersion != SpecialistContextProtocolVersion {
		return fmt.Errorf("unsupported Specialist Skill context protocol %q", a.ProtocolVersion)
	}
	for _, value := range []string{
		a.ParentSelectionID, a.RunID, a.MissionID, a.AgentID, a.ParentAgentID,
		a.AgentAttemptID, a.ModeSnapshotID,
	} {
		if !validSelectionIdentity(value) {
			return errors.New("specialist Skill context identities are invalid")
		}
	}
	if !validSHA256(a.ParentSelectionFingerprint) ||
		!validSHA256(a.AssignmentFingerprint) || !validSHA256(a.Fingerprint) {
		return errors.New("specialist Skill context fingerprints are invalid")
	}
	if a.Turn <= 0 || a.ModeRevision <= 0 || !a.Surface.Valid() {
		return errors.New("specialist Skill context turn, mode revision, or surface is invalid")
	}
	profile, err := domain.ParseProfile(string(a.Profile))
	if err != nil || profile != a.Profile {
		return fmt.Errorf("invalid Specialist Skill context profile %q", a.Profile)
	}
	if a.TokenBudget <= 0 || a.TokenBudget > MaxSpecialistContextTokenBudget ||
		a.ItemCount != len(a.Items) || len(a.Items) > MaxSpecialistContextItems ||
		a.TokenUpperBound < 0 || a.TokenUpperBound > a.TokenBudget ||
		a.RedactionCount < 0 || a.RedactionCount > a.TokenBudget {
		return errors.New("specialist Skill context bounds are invalid")
	}
	if len(a.Items) == 0 {
		if a.TokenUpperBound != 0 || a.RedactionCount != 0 {
			return errors.New("empty Specialist Skill context has nonzero accounting")
		}
	} else {
		item := a.Items[0]
		content := []byte(item.Content)
		digest := sha256.Sum256(content)
		if item.Ordinal != 1 || !validName(item.Name) || !validCoreVersion(item.Version) ||
			!validSHA256(item.SourceSHA256) || !validSHA256(item.DeliveredSHA256) ||
			item.SourceBytes <= 0 || item.SourceBytes > MaxContentBytes ||
			item.SourceTokenUpperBound <= 0 ||
			item.SourceTokenUpperBound > MaxContentTokenUpperBound ||
			item.SourceTokenUpperBound != item.SourceBytes || item.DeliveredBytes <= 0 ||
			item.TokenUpperBound != item.DeliveredBytes ||
			item.TokenUpperBound > item.SourceTokenUpperBound || item.RedactionCount < 0 ||
			validateContextContent(content) != nil || len(content) != item.DeliveredBytes ||
			item.DeliveredSHA256 != hex.EncodeToString(digest[:]) {
			return errors.New("specialist Skill context item is invalid")
		}
		if a.TokenUpperBound != item.TokenUpperBound ||
			a.RedactionCount != item.RedactionCount {
			return errors.New("specialist Skill context accounting is inconsistent")
		}
	}
	if a.Fingerprint != SpecialistContextFingerprint(a) {
		return errors.New("specialist Skill context fingerprint is invalid")
	}
	return nil
}

func (a SpecialistContextAssembly) Preparation() SpecialistContextPreparationRequest {
	return SpecialistContextPreparationRequest{
		RunID: a.RunID, MissionID: a.MissionID, AgentID: a.AgentID,
		ParentAgentID: a.ParentAgentID, AgentAttemptID: a.AgentAttemptID, Turn: a.Turn,
		ParentSelectionID: a.ParentSelectionID, ProtocolVersion: a.ProtocolVersion,
		ParentSelectionFingerprint: a.ParentSelectionFingerprint,
		ModeSnapshotID:             a.ModeSnapshotID, ModeRevision: a.ModeRevision,
		Surface: a.Surface, Profile: a.Profile,
		AssignmentFingerprint: a.AssignmentFingerprint,
		ContextFingerprint:    a.Fingerprint, ItemCount: a.ItemCount,
		TokenBudget: a.TokenBudget, TokenUpperBound: a.TokenUpperBound,
		RedactionCount: a.RedactionCount,
	}
}

func (r SpecialistContextPreparationRequest) Validate() error {
	assembly := SpecialistContextAssembly{
		ProtocolVersion: r.ProtocolVersion, ParentSelectionID: r.ParentSelectionID,
		ParentSelectionFingerprint: r.ParentSelectionFingerprint,
		RunID:                      r.RunID, MissionID: r.MissionID, AgentID: r.AgentID,
		ParentAgentID: r.ParentAgentID, AgentAttemptID: r.AgentAttemptID, Turn: r.Turn,
		ModeSnapshotID: r.ModeSnapshotID, ModeRevision: r.ModeRevision,
		Surface: r.Surface, Profile: r.Profile,
		AssignmentFingerprint: r.AssignmentFingerprint,
		Fingerprint:           r.ContextFingerprint, TokenBudget: r.TokenBudget,
		TokenUpperBound: r.TokenUpperBound, ItemCount: r.ItemCount,
		RedactionCount: r.RedactionCount,
	}
	if assembly.ItemCount < 0 || assembly.ItemCount > MaxSpecialistContextItems {
		return errors.New("specialist Skill context preparation item count is invalid")
	}
	for _, value := range []string{
		assembly.ParentSelectionID, assembly.RunID, assembly.MissionID, assembly.AgentID,
		assembly.ParentAgentID, assembly.AgentAttemptID, assembly.ModeSnapshotID,
	} {
		if !validSelectionIdentity(value) {
			return errors.New("specialist Skill context preparation identities are invalid")
		}
	}
	if assembly.ProtocolVersion != SpecialistContextProtocolVersion ||
		!validSHA256(assembly.ParentSelectionFingerprint) ||
		!validSHA256(assembly.AssignmentFingerprint) ||
		!validSHA256(assembly.Fingerprint) || assembly.Turn <= 0 ||
		assembly.ModeRevision <= 0 || !assembly.Surface.Valid() {
		return errors.New("specialist Skill context preparation provenance is invalid")
	}
	profile, err := domain.ParseProfile(string(assembly.Profile))
	if err != nil || profile != assembly.Profile || assembly.TokenBudget <= 0 ||
		assembly.TokenBudget > MaxSpecialistContextTokenBudget ||
		assembly.TokenUpperBound < 0 || assembly.TokenUpperBound > assembly.TokenBudget ||
		assembly.RedactionCount < 0 || assembly.RedactionCount > assembly.TokenBudget ||
		(assembly.ItemCount == 0 &&
			(assembly.TokenUpperBound != 0 || assembly.RedactionCount != 0)) ||
		(assembly.ItemCount > 0 && assembly.TokenUpperBound == 0) {
		return errors.New("specialist Skill context preparation bounds are invalid")
	}
	return nil
}

func (p SpecialistContextPreparation) Validate() error {
	if !validSelectionIdentity(p.ID) || p.PreparedAt.IsZero() {
		return errors.New("specialist Skill context preparation identity and time are required")
	}
	return p.SpecialistContextPreparationRequest.Validate()
}

func (c SpecialistContextCommit) Validate() error {
	if !validSelectionIdentity(c.PreparationID) || !validSelectionIdentity(c.RunID) ||
		!validSelectionIdentity(c.AgentAttemptID) || c.ModelAttempt <= 0 ||
		c.CommittedAt.IsZero() {
		return errors.New("specialist Skill context commit is invalid")
	}
	return nil
}

func SpecialistContextFingerprint(assembly SpecialistContextAssembly) string {
	parts := []string{
		SpecialistContextProtocolVersion, assembly.ParentSelectionID,
		assembly.ParentSelectionFingerprint, assembly.RunID, assembly.MissionID,
		assembly.AgentID, assembly.ParentAgentID, assembly.AgentAttemptID,
		strconv.FormatInt(assembly.Turn, 10), assembly.ModeSnapshotID,
		strconv.FormatInt(assembly.ModeRevision, 10), string(assembly.Surface),
		string(assembly.Profile), assembly.AssignmentFingerprint,
		strconv.Itoa(assembly.TokenBudget), strconv.Itoa(len(assembly.Items)),
	}
	for _, item := range assembly.Items {
		parts = append(parts, strconv.Itoa(item.Ordinal), item.Name, item.Version,
			item.SourceSHA256, strconv.Itoa(item.SourceBytes),
			strconv.Itoa(item.SourceTokenUpperBound), item.DeliveredSHA256,
			strconv.Itoa(item.DeliveredBytes), strconv.Itoa(item.TokenUpperBound),
			strconv.Itoa(item.RedactionCount))
	}
	return runmutation.Fingerprint(parts...)
}

func SpecialistAssignmentFingerprint(child domain.AgentNode) string {
	parts := []string{
		"specialist_skill_assignment.v1", child.RunID, child.ID, child.ParentID,
		string(child.Profile), strconv.FormatInt(child.TurnLimit, 10),
		strconv.FormatInt(child.TokenLimit, 10), strconv.Itoa(len(child.Skills)),
	}
	parts = append(parts, child.Skills...)
	return runmutation.Fingerprint(parts...)
}

// SpecialistSelectionItem returns at most one item already pinned by the
// parent Run. It never consults child-provided text or introduces a Registry
// entry that the operator did not select before the Run started.
func SpecialistSelectionItem(parent Selection, surface domain.ExecutionSurface,
	profile domain.Profile,
) (SelectionItem, bool) {
	if parent.Profile != profile || !surface.Valid() {
		return SelectionItem{}, false
	}
	selectedName := specialistSkillName(surface, profile)
	if selectedName == "" {
		return SelectionItem{}, false
	}
	for _, item := range parent.Items {
		if item.Name == selectedName {
			selected := item
			selected.Ordinal = 1
			return selected, true
		}
	}
	return SelectionItem{}, false
}

func specialistSkillName(surface domain.ExecutionSurface, profile domain.Profile) string {
	if surface == domain.ExecutionSurfaceCyber {
		if profile == domain.ProfileScript {
			return "script"
		}
		return ""
	}
	switch profile {
	case domain.ProfileCode:
		return "code"
	case domain.ProfileReview:
		return "review"
	case domain.ProfileLearn:
		return "learn"
	case domain.ProfileScript:
		return "script"
	default:
		return ""
	}
}
