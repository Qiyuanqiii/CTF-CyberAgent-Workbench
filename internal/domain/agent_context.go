package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	RootInboxContextVersion       = "root_inbox_context.v1"
	SpecialistContextVersion      = "specialist_context.v1"
	SpecialistInstructionVersion  = "specialist_instruction.v1"
	AgentAttemptFailureVersion    = "agent_attempt_failure.v1"
	MaxRootInboxContextMessages   = 4
	MaxRootInboxDeliveryHistory   = 256
	MaxRootInboxContextTextRunes  = 1200
	MaxRootInboxContextReferences = 4
	MaxSpecialistContextMessages  = 4
	MaxSpecialistDeliveryHistory  = 256
	MaxSpecialistInstructionRunes = 1200
)

type RootInboxDeliveryStatus string

const (
	RootInboxDeliveryPrepared   RootInboxDeliveryStatus = "prepared"
	RootInboxDeliveryCommitted  RootInboxDeliveryStatus = "committed"
	RootInboxDeliverySuperseded RootInboxDeliveryStatus = "superseded"
)

type AgentCompletionInboxPayload struct {
	CompletionReportID string           `json:"completion_report_id"`
	AgentID            string           `json:"agent_id"`
	Report             CompletionReport `json:"report"`
}

type AgentAttemptFailurePayload struct {
	Version        string `json:"version"`
	AgentID        string `json:"agent_id"`
	AttemptID      string `json:"attempt_id"`
	FailureCode    string `json:"failure_code"`
	Reason         string `json:"reason"`
	RetryScheduled bool   `json:"retry_scheduled"`
	Recovered      bool   `json:"recovered"`
}

// AgentInstructionPayload is the only parent-to-Specialist message body that
// can enter a child model context. Routing identity remains outside the model
// payload and is verified against the durable Agent graph by the Store.
type AgentInstructionPayload struct {
	Version     string `json:"version"`
	Instruction string `json:"instruction"`
}

type RootInboxDelivery struct {
	RunID               string
	RootAgentID         string
	SupervisorAttemptID string
	Turn                int
	MessageID           string
	Ordinal             int
	Status              RootInboxDeliveryStatus
	PreparedAt          time.Time
	ResolvedAt          *time.Time
}

type RootInboxContextBatch struct {
	RunID               string
	RootAgentID         string
	SupervisorAttemptID string
	Turn                int
	Messages            []AgentMessage
	PreparedAt          time.Time
	Recovered           bool
}

type SpecialistContextDelivery struct {
	RunID          string
	AgentID        string
	ParentAgentID  string
	AgentAttemptID string
	Turn           int64
	MessageID      string
	Ordinal        int
	Status         RootInboxDeliveryStatus
	PreparedAt     time.Time
	ResolvedAt     *time.Time
}

type SpecialistContextBatch struct {
	RunID          string
	AgentID        string
	ParentAgentID  string
	AgentAttemptID string
	Turn           int64
	Messages       []AgentMessage
	PreparedAt     time.Time
	Recovered      bool
}

func (p AgentInstructionPayload) Validate() error {
	if p.Version != SpecialistInstructionVersion {
		return fmt.Errorf("unsupported Specialist instruction payload version %q", p.Version)
	}
	if !utf8.ValidString(p.Instruction) || strings.TrimSpace(p.Instruction) == "" {
		return errors.New("specialist instruction is required and must be valid UTF-8")
	}
	if p.Instruction != strings.TrimSpace(p.Instruction) {
		return errors.New("specialist instruction must be normalized")
	}
	if runeCount(p.Instruction) > MaxSpecialistInstructionRunes {
		return fmt.Errorf("specialist instruction exceeds %d characters", MaxSpecialistInstructionRunes)
	}
	return nil
}

func DecodeAgentInstructionPayload(payloadJSON string) (AgentInstructionPayload, error) {
	var payload AgentInstructionPayload
	if err := decodeStrictAgentPayload(payloadJSON, &payload); err != nil {
		return AgentInstructionPayload{}, fmt.Errorf("invalid Specialist instruction payload: %w", err)
	}
	payload.Version = strings.TrimSpace(payload.Version)
	payload.Instruction = strings.TrimSpace(payload.Instruction)
	if err := payload.Validate(); err != nil {
		return AgentInstructionPayload{}, fmt.Errorf("invalid Specialist instruction payload: %w", err)
	}
	return payload, nil
}

func (p AgentCompletionInboxPayload) Validate() error {
	if !validAgentIdentity(p.CompletionReportID, false) ||
		!validAgentIdentity(p.AgentID, false) {
		return errors.New("completion inbox identities are required and must be normalized")
	}
	if err := p.Report.Validate(); err != nil {
		return err
	}
	return nil
}

func DecodeAgentCompletionInboxPayload(payloadJSON string) (AgentCompletionInboxPayload, error) {
	var payload AgentCompletionInboxPayload
	if err := decodeStrictAgentPayload(payloadJSON, &payload); err != nil {
		return AgentCompletionInboxPayload{}, fmt.Errorf("invalid completion inbox payload: %w", err)
	}
	payload.CompletionReportID = strings.TrimSpace(payload.CompletionReportID)
	payload.AgentID = strings.TrimSpace(payload.AgentID)
	report, err := NormalizeCompletionReport(payload.Report)
	if err != nil {
		return AgentCompletionInboxPayload{}, fmt.Errorf("invalid completion inbox payload: %w", err)
	}
	payload.Report = report
	if err := payload.Validate(); err != nil {
		return AgentCompletionInboxPayload{}, fmt.Errorf("invalid completion inbox payload: %w", err)
	}
	return payload, nil
}

func (p AgentAttemptFailurePayload) Validate() error {
	if p.Version != AgentAttemptFailureVersion {
		return fmt.Errorf("unsupported Agent attempt failure payload version %q", p.Version)
	}
	if !validAgentIdentity(p.AgentID, false) || !validAgentIdentity(p.AttemptID, false) {
		return errors.New("agent attempt failure payload identities are invalid")
	}
	normalized, err := NormalizeAgentAttemptFailure(AgentAttemptFailure{
		Code: p.FailureCode, Reason: p.Reason,
	})
	if err != nil || normalized.Code != p.FailureCode || normalized.Reason != p.Reason {
		return errors.New("agent attempt failure payload must contain normalized failure metadata")
	}
	return nil
}

func DecodeAgentAttemptFailurePayload(payloadJSON string) (AgentAttemptFailurePayload, error) {
	var payload AgentAttemptFailurePayload
	if err := decodeStrictAgentPayload(payloadJSON, &payload); err != nil {
		return AgentAttemptFailurePayload{}, fmt.Errorf("invalid Agent attempt failure payload: %w", err)
	}
	payload.Version = strings.TrimSpace(payload.Version)
	payload.AgentID = strings.TrimSpace(payload.AgentID)
	payload.AttemptID = strings.TrimSpace(payload.AttemptID)
	failure, err := NormalizeAgentAttemptFailure(AgentAttemptFailure{
		Code: payload.FailureCode, Reason: payload.Reason,
	})
	if err != nil {
		return AgentAttemptFailurePayload{}, fmt.Errorf("invalid Agent attempt failure payload: %w", err)
	}
	payload.FailureCode = failure.Code
	payload.Reason = failure.Reason
	if err := payload.Validate(); err != nil {
		return AgentAttemptFailurePayload{}, fmt.Errorf("invalid Agent attempt failure payload: %w", err)
	}
	return payload, nil
}

func ValidRootInboxDeliveryStatus(status RootInboxDeliveryStatus) bool {
	return status == RootInboxDeliveryPrepared || status == RootInboxDeliveryCommitted ||
		status == RootInboxDeliverySuperseded
}

func EligibleRootInboxMessage(message AgentMessage) bool {
	if message.Semantic == AgentMessageSemanticDependency {
		return message.Kind == AgentMessageNotification
	}
	if message.Semantic != AgentMessageSemanticMessage {
		return false
	}
	return message.Kind == AgentMessageResult || message.Kind == AgentMessageNotification
}

func EligibleSpecialistContextMessage(message AgentMessage) bool {
	return message.Kind == AgentMessageInstruction &&
		message.Semantic == AgentMessageSemanticMessage
}

func (d RootInboxDelivery) Validate() error {
	for _, value := range []string{
		d.RunID, d.RootAgentID, d.SupervisorAttemptID, d.MessageID,
	} {
		if !validAgentIdentity(value, false) {
			return errors.New("root inbox delivery identities are required and must be normalized")
		}
	}
	if d.Turn <= 0 || d.Ordinal <= 0 || d.Ordinal > MaxRootInboxContextMessages {
		return errors.New("root inbox delivery turn and ordinal are invalid")
	}
	if !ValidRootInboxDeliveryStatus(d.Status) || d.PreparedAt.IsZero() {
		return errors.New("root inbox delivery status and prepared time are required")
	}
	if d.Status == RootInboxDeliveryPrepared {
		if d.ResolvedAt != nil {
			return errors.New("prepared root inbox delivery cannot have a resolution time")
		}
		return nil
	}
	if d.ResolvedAt == nil || d.ResolvedAt.IsZero() || d.ResolvedAt.Before(d.PreparedAt) {
		return errors.New("terminal root inbox delivery requires a valid resolution time")
	}
	return nil
}

func (b RootInboxContextBatch) Validate() error {
	for _, value := range []string{b.RunID, b.RootAgentID, b.SupervisorAttemptID} {
		if !validAgentIdentity(value, false) {
			return errors.New("root inbox context identities are required and must be normalized")
		}
	}
	if b.Turn <= 0 || b.PreparedAt.IsZero() || len(b.Messages) > MaxRootInboxContextMessages {
		return errors.New("root inbox context turn, prepared time, or message count is invalid")
	}
	seen := make([]string, 0, len(b.Messages))
	var previousSequence int64
	for _, message := range b.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
		if message.RunID != b.RunID || message.RecipientAgentID != b.RootAgentID ||
			message.SenderAgentID == "" || message.Status != AgentMessagePending ||
			!EligibleRootInboxMessage(message) || message.Sequence <= previousSequence {
			return errors.New("root inbox context contains an invalid or unordered message")
		}
		seen = append(seen, message.ID)
		previousSequence = message.Sequence
	}
	unique := append([]string(nil), seen...)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	if len(unique) != len(seen) {
		return errors.New("root inbox context contains duplicate messages")
	}
	return nil
}

func (d SpecialistContextDelivery) Validate() error {
	for _, value := range []string{
		d.RunID, d.AgentID, d.ParentAgentID, d.AgentAttemptID, d.MessageID,
	} {
		if !validAgentIdentity(value, false) {
			return errors.New("specialist context delivery identities are required and must be normalized")
		}
	}
	if d.Turn <= 0 || d.Ordinal <= 0 || d.Ordinal > MaxSpecialistContextMessages {
		return errors.New("specialist context delivery turn and ordinal are invalid")
	}
	if !ValidRootInboxDeliveryStatus(d.Status) || d.PreparedAt.IsZero() {
		return errors.New("specialist context delivery status and prepared time are required")
	}
	if d.Status == RootInboxDeliveryPrepared {
		if d.ResolvedAt != nil {
			return errors.New("prepared Specialist context delivery cannot have a resolution time")
		}
		return nil
	}
	if d.ResolvedAt == nil || d.ResolvedAt.IsZero() || d.ResolvedAt.Before(d.PreparedAt) {
		return errors.New("terminal Specialist context delivery requires a valid resolution time")
	}
	return nil
}

func (b SpecialistContextBatch) Validate() error {
	for _, value := range []string{b.RunID, b.AgentID, b.ParentAgentID, b.AgentAttemptID} {
		if !validAgentIdentity(value, false) {
			return errors.New("specialist context identities are required and must be normalized")
		}
	}
	if b.Turn <= 0 || b.PreparedAt.IsZero() || len(b.Messages) > MaxSpecialistContextMessages {
		return errors.New("specialist context turn, prepared time, or message count is invalid")
	}
	seen := make([]string, 0, len(b.Messages))
	var previousSequence int64
	for _, message := range b.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
		if message.RunID != b.RunID || message.RecipientAgentID != b.AgentID ||
			message.SenderAgentID != b.ParentAgentID || message.Status != AgentMessagePending ||
			!EligibleSpecialistContextMessage(message) || message.Sequence <= previousSequence {
			return errors.New("specialist context contains an invalid or unordered message")
		}
		if _, err := DecodeAgentInstructionPayload(message.PayloadJSON); err != nil {
			return err
		}
		seen = append(seen, message.ID)
		previousSequence = message.Sequence
	}
	unique := append([]string(nil), seen...)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	if len(unique) != len(seen) {
		return errors.New("specialist context contains duplicate messages")
	}
	return nil
}
