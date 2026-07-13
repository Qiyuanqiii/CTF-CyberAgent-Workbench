package tui

import "cyberagent-workbench/internal/domain"

// PickerRunProjection is the bounded Run identity shown by the TUI picker.
type PickerRunProjection struct {
	RunID     string
	MissionID string
	SessionID string
	Status    domain.RunStatus
}

// PickerSessionProjection is the bounded Session identity shown by the TUI picker.
type PickerSessionProjection struct {
	SessionID string
	Status    string
}

// PickerProjection exposes a stable read contract without leaking picker internals.
type PickerProjection struct {
	View              string
	Runs              []PickerRunProjection
	Sessions          []PickerSessionProjection
	RunsTruncated     bool
	SessionsTruncated bool
}

// CurrentProjection returns a defensive copy of the picker's bounded read state.
func (p *Picker) CurrentProjection() PickerProjection {
	if p == nil {
		return PickerProjection{}
	}
	projection := PickerProjection{
		View:              string(p.view),
		Runs:              make([]PickerRunProjection, len(p.runs)),
		Sessions:          make([]PickerSessionProjection, len(p.sessions)),
		RunsTruncated:     p.runsTruncated,
		SessionsTruncated: p.sessionsTruncated,
	}
	for index, item := range p.runs {
		projection.Runs[index] = PickerRunProjection{
			RunID: item.Run.ID, MissionID: item.Run.MissionID,
			SessionID: item.Run.SessionID, Status: item.Run.Status,
		}
	}
	for index, item := range p.sessions {
		projection.Sessions[index] = PickerSessionProjection{
			SessionID: item.ID, Status: item.Status,
		}
	}
	return projection
}
