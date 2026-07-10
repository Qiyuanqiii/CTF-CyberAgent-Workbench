package events

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/idgen"
)

const EnvelopeVersion = "v1"

const (
	RunCreatedEvent       = "run.created"
	RunStatusChangedEvent = "run.status_changed"
)

type Event struct {
	ID          int64
	EventID     string
	Version     string
	RunID       string
	MissionID   string
	Sequence    int64
	Type        string
	Source      string
	SubjectID   string
	PayloadJSON string
	CreatedAt   time.Time
}

func New(runID string, missionID string, eventType string, source string, subjectID string, payload any) (Event, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	event := Event{
		EventID:     idgen.New("evt"),
		Version:     EnvelopeVersion,
		RunID:       strings.TrimSpace(runID),
		MissionID:   strings.TrimSpace(missionID),
		Type:        strings.TrimSpace(eventType),
		Source:      strings.TrimSpace(source),
		SubjectID:   strings.TrimSpace(subjectID),
		PayloadJSON: string(encoded),
		CreatedAt:   time.Now().UTC(),
	}
	return event, event.Validate()
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.RunID) == "" || strings.TrimSpace(e.MissionID) == "" {
		return errors.New("event id, run id, and mission id are required")
	}
	if e.Version != EnvelopeVersion {
		return errors.New("unsupported event envelope version")
	}
	if strings.TrimSpace(e.Type) == "" || strings.TrimSpace(e.Source) == "" {
		return errors.New("event type and source are required")
	}
	if !json.Valid([]byte(e.PayloadJSON)) {
		return errors.New("event payload must be valid JSON")
	}
	if e.CreatedAt.IsZero() {
		return errors.New("event timestamp is required")
	}
	return nil
}
