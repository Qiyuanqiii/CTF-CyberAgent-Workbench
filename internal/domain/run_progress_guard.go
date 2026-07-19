package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RunProgressGuardProtocolVersion = "run_progress_guard.v1"
	RunProgressRepeatThreshold      = 3
	RunProgressStagnantThreshold    = 6
)

type RunProgressGuardStatus string

const (
	RunProgressObserving RunProgressGuardStatus = "observing"
	RunProgressDetected  RunProgressGuardStatus = "livelock_detected"
)

type RunProgressReason string

const (
	RunProgressReasonNone           RunProgressReason = ""
	RunProgressRepeatedAction       RunProgressReason = "repeated_action"
	RunProgressNoObservableProgress RunProgressReason = "no_observable_progress"
)

type RunProgressGuard struct {
	RunID               string
	ProtocolVersion     string
	StateFingerprint    string
	ActionFingerprint   string
	RepeatedActionCount int
	StagnantTurnCount   int
	RepeatThreshold     int
	StagnantThreshold   int
	LastTurn            int
	Status              RunProgressGuardStatus
	Reason              RunProgressReason
	DetectedAt          *time.Time
	UpdatedAt           time.Time
}

func (g RunProgressGuard) Validate() error {
	if !ValidAgentID(g.RunID) || g.ProtocolVersion != RunProgressGuardProtocolVersion {
		return errors.New("Run progress guard identity or protocol is invalid")
	}
	if g.RepeatThreshold != RunProgressRepeatThreshold ||
		g.StagnantThreshold != RunProgressStagnantThreshold ||
		g.RepeatedActionCount < 0 || g.StagnantTurnCount < 0 || g.LastTurn < 0 {
		return errors.New("Run progress guard counters or thresholds are invalid")
	}
	empty := g.StateFingerprint == "" && g.ActionFingerprint == ""
	if empty != (g.RepeatedActionCount == 0 && g.StagnantTurnCount == 0) {
		return errors.New("Run progress guard fingerprints and counters are inconsistent")
	}
	if !empty && (!validLowerHexDigest(g.StateFingerprint) || !validLowerHexDigest(g.ActionFingerprint) || g.LastTurn == 0) {
		return errors.New("Run progress guard fingerprints must be lowercase SHA-256")
	}
	if g.UpdatedAt.IsZero() {
		return errors.New("Run progress guard update time is required")
	}
	switch g.Status {
	case RunProgressObserving:
		if g.Reason != RunProgressReasonNone || g.DetectedAt != nil {
			return errors.New("observing Run progress guard cannot contain detection metadata")
		}
		if g.RepeatedActionCount >= g.RepeatThreshold ||
			g.StagnantTurnCount >= g.StagnantThreshold {
			return errors.New("observing Run progress guard cannot meet a detection threshold")
		}
	case RunProgressDetected:
		if g.DetectedAt == nil || (g.Reason != RunProgressRepeatedAction &&
			g.Reason != RunProgressNoObservableProgress) {
			return errors.New("detected Run progress guard requires a reason and timestamp")
		}
		if g.Reason == RunProgressRepeatedAction && g.RepeatedActionCount < g.RepeatThreshold {
			return errors.New("repeated-action detection did not reach its threshold")
		}
		if g.Reason == RunProgressNoObservableProgress && g.StagnantTurnCount < g.StagnantThreshold {
			return errors.New("no-progress detection did not reach its threshold")
		}
	default:
		return fmt.Errorf("unsupported Run progress guard status %q", g.Status)
	}
	return nil
}

func (g RunProgressGuard) WaitReason() string {
	reason := strings.TrimSpace(string(g.Reason))
	if reason == "" {
		reason = "guard_triggered"
	}
	return "livelock_detected:" + reason + "; explicit operator resume required"
}
