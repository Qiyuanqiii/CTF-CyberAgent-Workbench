package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	SpecialistLifecycleVersion = "specialist_lifecycle.v1"
	MaxSpecialistMessageRunes  = 4096
	MaxSpecialistMessageBytes  = 8 * 1024
)

type SpecialistActionKind string

const (
	SpecialistActionContinue SpecialistActionKind = "continue"
	SpecialistActionFinish   SpecialistActionKind = "finish"
)

// SpecialistAction is a model proposal. Go remains responsible for usage,
// policy, lease, and lifecycle transitions after validating this payload.
type SpecialistAction struct {
	Version string               `json:"version"`
	Kind    SpecialistActionKind `json:"action"`
	Message string               `json:"message"`
	Report  *CompletionReport    `json:"report,omitempty"`
}

func NormalizeSpecialistAction(action SpecialistAction) (SpecialistAction, error) {
	action.Version = strings.TrimSpace(action.Version)
	action.Kind = SpecialistActionKind(strings.ToLower(strings.TrimSpace(string(action.Kind))))
	action.Message = strings.TrimSpace(action.Message)
	if action.Report != nil {
		report, err := NormalizeCompletionReport(*action.Report)
		if err != nil {
			return SpecialistAction{}, fmt.Errorf("invalid Specialist completion report: %w", err)
		}
		action.Report = &report
	}
	if err := action.Validate(); err != nil {
		return SpecialistAction{}, err
	}
	return action, nil
}

func (a SpecialistAction) Validate() error {
	if a.Version != SpecialistLifecycleVersion {
		return fmt.Errorf("unsupported Specialist lifecycle version %q", a.Version)
	}
	if !utf8.ValidString(a.Message) || strings.TrimSpace(a.Message) != a.Message ||
		a.Message == "" || strings.ContainsRune(a.Message, 0) ||
		utf8.RuneCountInString(a.Message) > MaxSpecialistMessageRunes ||
		len([]byte(a.Message)) > MaxSpecialistMessageBytes {
		return fmt.Errorf("specialist message must contain between 1 and %d characters within %d bytes",
			MaxSpecialistMessageRunes, MaxSpecialistMessageBytes)
	}
	switch a.Kind {
	case SpecialistActionContinue:
		if a.Report != nil {
			return errors.New("specialist continue action cannot include a completion report")
		}
	case SpecialistActionFinish:
		if a.Report == nil {
			return errors.New("specialist finish action requires a completion report")
		}
		if err := a.Report.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported Specialist action %q", a.Kind)
	}
	return nil
}

func DecodeSpecialistAction(payloadJSON string) (SpecialistAction, error) {
	if len([]byte(payloadJSON)) == 0 || len([]byte(payloadJSON)) > MaxAgentMessagePayloadBytes ||
		!utf8.ValidString(payloadJSON) {
		return SpecialistAction{}, fmt.Errorf("specialist lifecycle payload must be valid UTF-8 within %d bytes",
			MaxAgentMessagePayloadBytes)
	}
	var action SpecialistAction
	if err := decodeStrictAgentPayload(payloadJSON, &action); err != nil {
		return SpecialistAction{}, fmt.Errorf("invalid Specialist lifecycle payload: %w", err)
	}
	action, err := NormalizeSpecialistAction(action)
	if err != nil {
		return SpecialistAction{}, fmt.Errorf("invalid Specialist lifecycle payload: %w", err)
	}
	return action, nil
}
