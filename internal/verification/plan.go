package verification

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

const (
	PlanProtocolVersion          = "operator_verification_plan.v1"
	PlanInventoryProtocolVersion = "operator_verification_plan_inventory.v1"
	MaxPlanItems                 = 32
	MaxPlanInventoryItems        = 50
	MaxPlanItemTitleRunes        = 160
	MaxExpectedObservationRunes  = 1024
)

type PlanItem struct {
	Ordinal             int
	Title               string
	ExpectedObservation string
	ItemSHA256          string
	Redacted            bool
}

// Plan is immutable operator guidance. It deliberately has no result, pass,
// completion, command, model assertion, approval, or execution field.
type Plan struct {
	ID                 string
	ProtocolVersion    string
	OperationKeyDigest string
	RequestFingerprint string
	RunID              string
	SessionID          string
	WorkspaceID        string
	Title              string
	Summary            string
	PlanSHA256         string
	Redacted           bool
	AuthoredBy         string
	EventSequence      int64
	CreatedAt          time.Time
	Items              []PlanItem
}

func (p Plan) Validate() error {
	if p.ProtocolVersion != PlanProtocolVersion || !validDigest(p.OperationKeyDigest) ||
		!validDigest(p.RequestFingerprint) || !validDigest(p.PlanSHA256) ||
		p.EventSequence <= 0 || p.CreatedAt.IsZero() || len(p.Items) < 1 ||
		len(p.Items) > MaxPlanItems {
		return errors.New("verification plan protocol, digest, or durable binding is invalid")
	}
	for _, value := range []string{p.ID, p.RunID, p.SessionID, p.WorkspaceID, p.AuthoredBy} {
		if !validIdentity(value) {
			return errors.New("verification plan identity is invalid")
		}
	}
	if err := ValidateText(p.Title, MaxTitleRunes, false); err != nil {
		return errors.New("verification plan title is invalid")
	}
	if err := ValidateText(p.Summary, MaxSummaryRunes, true); err != nil {
		return errors.New("verification plan summary is invalid")
	}
	redacted := false
	for index, item := range p.Items {
		if item.Ordinal != index+1 ||
			ValidateText(item.Title, MaxPlanItemTitleRunes, false) != nil ||
			ValidateText(item.ExpectedObservation, MaxExpectedObservationRunes, true) != nil ||
			!validDigest(item.ItemSHA256) ||
			item.ItemSHA256 != PlanItemDigest(item.Title, item.ExpectedObservation) {
			return errors.New("verification plan item is invalid")
		}
		redacted = redacted || item.Redacted
	}
	if (redacted && !p.Redacted) || p.PlanSHA256 != PlanDigest(p.Title, p.Summary, p.Items) {
		return errors.New("verification plan content digest or redaction state is invalid")
	}
	return nil
}

func PlanItemDigest(title string, expectedObservation string) string {
	return digestPlanFields("operator_verification_plan_item.v1", title, expectedObservation)
}

func PlanDigest(title string, summary string, items []PlanItem) string {
	values := make([]string, 0, 3+len(items)*4)
	values = append(values, PlanProtocolVersion, title, summary)
	for _, item := range items {
		values = append(values, strconv.Itoa(item.Ordinal), item.Title,
			item.ExpectedObservation, item.ItemSHA256)
	}
	return digestPlanFields(values...)
}

func digestPlanFields(values ...string) string {
	hash := sha256.New()
	var size [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(size[:], uint64(len([]byte(value))))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
