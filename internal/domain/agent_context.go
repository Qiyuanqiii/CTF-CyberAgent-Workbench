package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	RootInboxContextVersion       = "root_inbox_context.v1"
	AgentAttemptFailureVersion    = "agent_attempt_failure.v1"
	MaxRootInboxContextMessages   = 4
	MaxRootInboxDeliveryHistory   = 256
	MaxRootInboxContextTextRunes  = 1200
	MaxRootInboxContextReferences = 4
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
