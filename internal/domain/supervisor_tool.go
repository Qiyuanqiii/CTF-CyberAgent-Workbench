package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxSupervisorToolRounds        = 4
	MaxSupervisorToolCallsPerRound = 4
	MaxSupervisorToolPayloadBytes  = 96 * 1024
	MaxSupervisorToolResultBytes   = 16 * 1024
	MaxSupervisorToolIdentityRunes = 256
)

type SupervisorToolCallStatus string

const (
	SupervisorToolPending   SupervisorToolCallStatus = "pending"
	SupervisorToolCompleted SupervisorToolCallStatus = "completed"
	SupervisorToolDenied    SupervisorToolCallStatus = "denied"
	SupervisorToolFailed    SupervisorToolCallStatus = "failed"
)

func (s SupervisorToolCallStatus) Valid() bool {
	switch s {
	case SupervisorToolPending, SupervisorToolCompleted, SupervisorToolDenied, SupervisorToolFailed:
		return true
	default:
		return false
	}
}

func (s SupervisorToolCallStatus) Terminal() bool {
	return s == SupervisorToolCompleted || s == SupervisorToolDenied || s == SupervisorToolFailed
}

type SupervisorToolCall struct {
	RunID        string
	Turn         int
	AttemptID    string
	Round        int
	Position     int
	ModelAttempt int
	CallID       string
	ToolName     string
	PayloadJSON  string
	Status       SupervisorToolCallStatus
	ResultJSON   string
	ErrorCode    string
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

func (c SupervisorToolCall) Validate() error {
	for label, value := range map[string]string{
		"run id": c.RunID, "attempt id": c.AttemptID, "call id": c.CallID, "tool name": c.ToolName,
		"error code": c.ErrorCode,
	} {
		if !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
			len([]rune(value)) > MaxSupervisorToolIdentityRunes {
			return fmt.Errorf("supervisor tool %s must be normalized and bounded UTF-8", label)
		}
	}
	if c.RunID == "" || c.AttemptID == "" || c.CallID == "" || c.ToolName == "" {
		return errors.New("supervisor tool Run, attempt, call, and tool identities are required")
	}
	if c.Turn <= 0 || c.Round <= 0 || c.Round > MaxSupervisorToolRounds ||
		c.Position <= 0 || c.Position > MaxSupervisorToolCallsPerRound || c.ModelAttempt <= 0 {
		return errors.New("supervisor tool turn, round, position, and model attempt are invalid")
	}
	if c.ToolName != "work_item_create" && c.ToolName != "note_create" {
		return fmt.Errorf("unsupported supervisor tool %q", c.ToolName)
	}
	if len(c.PayloadJSON) == 0 || len(c.PayloadJSON) > MaxSupervisorToolPayloadBytes ||
		!utf8.ValidString(c.PayloadJSON) || !json.Valid([]byte(c.PayloadJSON)) {
		return errors.New("supervisor tool payload must be bounded valid UTF-8 JSON")
	}
	if !c.Status.Valid() {
		return fmt.Errorf("invalid supervisor tool status %q", c.Status)
	}
	if c.CreatedAt.IsZero() {
		return errors.New("supervisor tool creation time is required")
	}
	if c.Status == SupervisorToolPending {
		if c.ResultJSON != "" || c.ErrorCode != "" || c.CompletedAt != nil {
			return errors.New("pending supervisor tool call cannot have a result")
		}
		return nil
	}
	if len(c.ResultJSON) == 0 || len(c.ResultJSON) > MaxSupervisorToolResultBytes ||
		!utf8.ValidString(c.ResultJSON) || !json.Valid([]byte(c.ResultJSON)) || c.CompletedAt == nil ||
		c.CompletedAt.IsZero() {
		return errors.New("terminal supervisor tool call requires bounded JSON and a completion time")
	}
	if c.Status == SupervisorToolCompleted && c.ErrorCode != "" {
		return errors.New("completed supervisor tool call cannot have an error code")
	}
	if c.Status != SupervisorToolCompleted && c.ErrorCode == "" {
		return errors.New("denied or failed supervisor tool call requires an error code")
	}
	return nil
}

type SupervisorToolRound struct {
	RunID        string
	Turn         int
	AttemptID    string
	Round        int
	ModelAttempt int
	Calls        []SupervisorToolCall
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

func (r SupervisorToolRound) Validate() error {
	if strings.TrimSpace(r.RunID) == "" || strings.TrimSpace(r.AttemptID) == "" ||
		r.RunID != strings.TrimSpace(r.RunID) || r.AttemptID != strings.TrimSpace(r.AttemptID) {
		return errors.New("supervisor tool round Run and attempt identities are required and normalized")
	}
	if r.Turn <= 0 || r.Round <= 0 || r.Round > MaxSupervisorToolRounds || r.ModelAttempt <= 0 {
		return errors.New("supervisor tool round counters are invalid")
	}
	if len(r.Calls) == 0 || len(r.Calls) > MaxSupervisorToolCallsPerRound || r.CreatedAt.IsZero() {
		return errors.New("supervisor tool round requires a bounded call list and creation time")
	}
	complete := true
	seen := make(map[string]struct{}, len(r.Calls))
	for index, call := range r.Calls {
		if err := call.Validate(); err != nil {
			return err
		}
		if call.RunID != r.RunID || call.Turn != r.Turn || call.AttemptID != r.AttemptID ||
			call.Round != r.Round || call.Position != index+1 || call.ModelAttempt != r.ModelAttempt {
			return errors.New("supervisor tool call does not match its round")
		}
		if _, exists := seen[call.CallID]; exists {
			return errors.New("supervisor tool call ids must be unique")
		}
		seen[call.CallID] = struct{}{}
		complete = complete && call.Status.Terminal()
	}
	if complete != (r.CompletedAt != nil) {
		return errors.New("supervisor tool round completion does not match its calls")
	}
	if r.CompletedAt != nil && r.CompletedAt.IsZero() {
		return errors.New("supervisor tool round completion time cannot be zero")
	}
	return nil
}

func (r SupervisorToolRound) Complete() bool {
	return r.CompletedAt != nil
}

type SupervisorToolResult struct {
	CallID      string
	Status      SupervisorToolCallStatus
	ResultJSON  string
	ErrorCode   string
	CompletedAt time.Time
}

func (r SupervisorToolResult) Validate() error {
	if strings.TrimSpace(r.CallID) == "" || r.CallID != strings.TrimSpace(r.CallID) ||
		len([]rune(r.CallID)) > MaxSupervisorToolIdentityRunes || !utf8.ValidString(r.CallID) {
		return errors.New("supervisor tool result call id is invalid")
	}
	if !r.Status.Terminal() {
		return errors.New("supervisor tool result must be terminal")
	}
	if len(r.ResultJSON) == 0 || len(r.ResultJSON) > MaxSupervisorToolResultBytes ||
		!utf8.ValidString(r.ResultJSON) || !json.Valid([]byte(r.ResultJSON)) || r.CompletedAt.IsZero() {
		return errors.New("supervisor tool result requires bounded JSON and a completion time")
	}
	if !utf8.ValidString(r.ErrorCode) || strings.TrimSpace(r.ErrorCode) != r.ErrorCode ||
		len([]rune(r.ErrorCode)) > MaxSupervisorToolIdentityRunes {
		return errors.New("supervisor tool result error code is invalid")
	}
	if r.Status == SupervisorToolCompleted && r.ErrorCode != "" {
		return errors.New("completed supervisor tool result cannot have an error code")
	}
	if r.Status != SupervisorToolCompleted && r.ErrorCode == "" {
		return errors.New("denied or failed supervisor tool result requires an error code")
	}
	return nil
}
