package toolbudget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxIdentityRunes = 256

type ChargeRequest struct {
	RunID       string
	SessionID   string
	WorkspaceID string
	ToolName    string
	ActionClass string
}

func (r ChargeRequest) Normalize() (ChargeRequest, error) {
	r.RunID = strings.TrimSpace(r.RunID)
	r.SessionID = strings.TrimSpace(r.SessionID)
	r.WorkspaceID = strings.TrimSpace(r.WorkspaceID)
	r.ToolName = strings.TrimSpace(r.ToolName)
	r.ActionClass = strings.TrimSpace(r.ActionClass)
	for label, value := range map[string]string{
		"run id": r.RunID, "session id": r.SessionID, "workspace id": r.WorkspaceID,
		"tool name": r.ToolName, "action class": r.ActionClass,
	} {
		if !utf8.ValidString(value) || len([]rune(value)) > MaxIdentityRunes {
			return ChargeRequest{}, fmt.Errorf("tool budget %s must be bounded UTF-8", label)
		}
	}
	if r.ToolName == "" || r.ActionClass == "" {
		return ChargeRequest{}, errors.New("tool budget tool name and action class are required")
	}
	return r, nil
}

type Usage struct {
	RunID       string
	Consumed    int64
	Limit       int64
	Remaining   int64
	LastCharge  string
	LastUpdated time.Time
	ExhaustedAt *time.Time
	Tracked     bool
}

type Store interface {
	ChargeToolCall(ctx context.Context, request ChargeRequest) (Usage, error)
	GetToolCallUsage(ctx context.Context, runID string) (Usage, error)
}
