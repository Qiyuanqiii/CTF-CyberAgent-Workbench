package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/runmutation"
)

const (
	DeliveryCheckpointProtocolVersion = "delivery_checkpoint.v1"
	MaxDeliveryEvidenceRunes          = 1024
	MaxDeliveryHandoffSummaryRunes    = 2048
)

// DeliveryCheckpoint is an immutable operator attestation for one selected
// Plan/Delivery WorkItem at one exact Deliver-mode and WorkItem revision.
// Go verifies the binding; evidence text remains an operator assertion and is
// never treated as executable model instruction.
type DeliveryCheckpoint struct {
	ID                     string
	RunID                  string
	SelectionID            string
	ProposalID             string
	WorkItemID             string
	DirectionOrdinal       int
	ModuleOrdinal          int
	ModuleCount            int
	ModeSnapshotID         string
	ModeRevision           int64
	WorkItemVersion        int64
	AcceptanceFingerprint  string
	SourceFingerprint      string
	FocusedVerification    string
	DiffAudit              string
	SecurityAudit          string
	FullGateRequired       bool
	FunctionalVerification string
	RobustnessAudit        string
	HandoffNoteID          string
	HandoffDigest          string
	RequestedBy            string
	Version                int64
	CreatedAt              time.Time
}

type DeliveryCheckpointOperation struct {
	KeyDigest          string
	RequestFingerprint string
	CheckpointID       string
	RunID              string
	WorkItemID         string
	RequestedBy        string
	CreatedAt          time.Time
}

func (c DeliveryCheckpoint) Validate() error {
	for label, value := range map[string]string{
		"checkpoint id": c.ID, "Run id": c.RunID,
		"selection id": c.SelectionID, "proposal id": c.ProposalID,
		"WorkItem id": c.WorkItemID, "mode snapshot id": c.ModeSnapshotID,
		"handoff Note id": c.HandoffNoteID, "requester": c.RequestedBy,
	} {
		if !validAgentIdentity(value, false) {
			return fmt.Errorf("delivery checkpoint %s is invalid", label)
		}
	}
	if c.DirectionOrdinal < 1 || c.DirectionOrdinal > PlanDeliveryDirectionCount ||
		c.ModuleOrdinal < 1 || c.ModuleCount < 1 ||
		c.ModuleOrdinal > c.ModuleCount || c.ModuleCount > MaxPlanDeliveryModules ||
		c.ModeRevision <= 0 || c.WorkItemVersion <= 0 || c.Version != 1 ||
		c.CreatedAt.IsZero() {
		return errors.New("delivery checkpoint shape, revision, or timestamp is invalid")
	}
	for label, digest := range map[string]string{
		"acceptance fingerprint": c.AcceptanceFingerprint,
		"source fingerprint":     c.SourceFingerprint,
		"handoff digest":         c.HandoffDigest,
	} {
		if !validLowerHexDigest(digest) {
			return fmt.Errorf("delivery checkpoint %s is invalid", label)
		}
	}
	for label, value := range map[string]string{
		"focused verification": c.FocusedVerification,
		"diff audit":           c.DiffAudit,
		"security audit":       c.SecurityAudit,
	} {
		if err := validateDeliveryEvidence(value, label, MaxDeliveryEvidenceRunes, false); err != nil {
			return err
		}
	}
	if c.FullGateRequired != (c.ModuleOrdinal == c.ModuleCount) {
		return errors.New("delivery checkpoint full gate must be required exactly at the final module boundary")
	}
	if c.FullGateRequired {
		if err := validateDeliveryEvidence(c.FunctionalVerification,
			"functional verification", MaxDeliveryEvidenceRunes, false); err != nil {
			return err
		}
		if err := validateDeliveryEvidence(c.RobustnessAudit,
			"robustness audit", MaxDeliveryEvidenceRunes, false); err != nil {
			return err
		}
	} else if c.FunctionalVerification != "" || c.RobustnessAudit != "" {
		return errors.New("non-boundary Delivery checkpoint cannot contain full-gate evidence")
	}
	return nil
}

func (o DeliveryCheckpointOperation) Validate() error {
	for _, value := range []string{o.CheckpointID, o.RunID, o.WorkItemID, o.RequestedBy} {
		if !validAgentIdentity(value, false) {
			return errors.New("delivery checkpoint operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) || !validLowerHexDigest(o.RequestFingerprint) ||
		o.CreatedAt.IsZero() {
		return errors.New("delivery checkpoint operation digest or timestamp is invalid")
	}
	return nil
}

func NormalizeDeliveryEvidence(value, label string, maxRunes int,
	allowEmpty bool,
) (string, error) {
	value = strings.TrimSpace(value)
	if err := validateDeliveryEvidence(value, label, maxRunes, allowEmpty); err != nil {
		return "", err
	}
	return value, nil
}

func validateDeliveryEvidence(value, label string, maxRunes int,
	allowEmpty bool,
) error {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		(!allowEmpty && value == "") || utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("delivery %s must contain between 1 and %d normalized UTF-8 characters",
			label, maxRunes)
	}
	for _, current := range value {
		if current == 0 || (unicode.IsControl(current) && current != '\n' &&
			current != '\r' && current != '\t') {
			return fmt.Errorf("delivery %s contains a forbidden control character", label)
		}
	}
	return nil
}

func DeliveryAcceptanceFingerprint(criteria []string) string {
	encoded, err := json.Marshal(criteria)
	if err != nil {
		return ""
	}
	return sha256Hex(encoded)
}

func DeliverySourceFingerprint(proposal PlanDeliveryProposal,
	selection PlanDeliverySelection, module PlanDeliveryModule, item WorkItem,
) string {
	acceptance, err := json.Marshal(module.AcceptanceCriteria)
	if err != nil {
		return ""
	}
	moduleDependencies, err := json.Marshal(module.Dependencies)
	if err != nil {
		return ""
	}
	itemDependencies, err := json.Marshal(item.Dependencies)
	if err != nil {
		return ""
	}
	return runmutation.Fingerprint("delivery_source.v1", proposal.ID,
		proposal.Fingerprint, selection.ID, fmt.Sprint(selection.DirectionOrdinal),
		fmt.Sprint(module.Ordinal), item.ID, item.Title, item.Description,
		string(acceptance), string(moduleDependencies), string(itemDependencies))
}

func DeliveryCheckpointRequestFingerprint(c DeliveryCheckpoint) string {
	return runmutation.Fingerprint("delivery_checkpoint_request.v1", c.RunID,
		c.SelectionID, c.ProposalID, c.WorkItemID,
		fmt.Sprint(c.DirectionOrdinal), fmt.Sprint(c.ModuleOrdinal),
		fmt.Sprint(c.ModuleCount), c.ModeSnapshotID, fmt.Sprint(c.ModeRevision),
		fmt.Sprint(c.WorkItemVersion), c.AcceptanceFingerprint,
		c.SourceFingerprint, c.FocusedVerification, c.DiffAudit,
		c.SecurityAudit, fmt.Sprint(c.FullGateRequired),
		c.FunctionalVerification, c.RobustnessAudit, c.HandoffDigest,
		c.RequestedBy)
}

func DeliveryHandoffDigest(title, content string) string {
	return sha256Hex([]byte(title + "\x00" + content))
}

func CloneDeliveryCheckpoints(values []DeliveryCheckpoint) []DeliveryCheckpoint {
	return slices.Clone(values)
}

func DeliveryCheckpointReady(checkpoint DeliveryCheckpoint, item WorkItem,
	mode RunModeSnapshot,
) bool {
	if checkpoint.RunID != item.RunID || checkpoint.WorkItemID != item.ID {
		return false
	}
	switch item.Status {
	case WorkItemCompleted:
		return item.Version == checkpoint.WorkItemVersion+1
	case WorkItemInProgress:
		return item.Version == checkpoint.WorkItemVersion &&
			mode.Phase == ExecutionPhaseDeliver && mode.ID == checkpoint.ModeSnapshotID &&
			mode.Revision == checkpoint.ModeRevision
	default:
		return false
	}
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
