package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const RootLifecycleVersion = "root_lifecycle.v1"

const maxRootActionFieldRunes = 16 * 1024

type RootActionKind string

const (
	RootActionContinue RootActionKind = "continue"
	RootActionFinish   RootActionKind = "finish"
	RootActionWait     RootActionKind = "wait"
)

type RootAction struct {
	Version string         `json:"version"`
	Kind    RootActionKind `json:"action"`
	Message string         `json:"message"`
	Summary string         `json:"summary,omitempty"`
	Reason  string         `json:"reason,omitempty"`
}

func (a RootAction) Validate() error {
	if strings.TrimSpace(a.Version) != RootLifecycleVersion {
		return fmt.Errorf("root action version must be %q", RootLifecycleVersion)
	}
	if strings.TrimSpace(a.Message) == "" {
		return errors.New("root action message is required")
	}
	if !utf8.ValidString(string(a.Kind)) {
		return errors.New("root action kind must be valid UTF-8")
	}
	if utf8.RuneCountInString(string(a.Kind)) > 32 {
		return errors.New("root action kind exceeds 32 characters")
	}
	for name, value := range map[string]string{
		"message": a.Message,
		"summary": a.Summary,
		"reason":  a.Reason,
	} {
		if !utf8.ValidString(value) {
			return fmt.Errorf("root action %s must be valid UTF-8", name)
		}
		if utf8.RuneCountInString(value) > maxRootActionFieldRunes {
			return fmt.Errorf("root action %s exceeds %d characters", name, maxRootActionFieldRunes)
		}
	}
	switch a.Kind {
	case RootActionContinue:
		if strings.TrimSpace(a.Summary) != "" || strings.TrimSpace(a.Reason) != "" {
			return errors.New("continue action cannot include summary or reason")
		}
	case RootActionFinish:
		if strings.TrimSpace(a.Summary) == "" {
			return errors.New("finish action summary is required")
		}
		if strings.TrimSpace(a.Reason) != "" {
			return errors.New("finish action cannot include reason")
		}
	case RootActionWait:
		if strings.TrimSpace(a.Reason) == "" {
			return errors.New("wait action reason is required")
		}
		if strings.TrimSpace(a.Summary) != "" {
			return errors.New("wait action cannot include summary")
		}
	default:
		return fmt.Errorf("unsupported root action %q", a.Kind)
	}
	return nil
}
