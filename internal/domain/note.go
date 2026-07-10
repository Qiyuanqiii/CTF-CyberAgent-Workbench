package domain

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxNoteTitleRunes    = 240
	MaxNoteContentBytes  = 64 * 1024
	MaxNoteOwnerRunes    = 128
	MaxNoteTags          = 32
	MaxNoteTagRunes      = 64
	MaxNoteSources       = 32
	MaxNoteSourceRunes   = 512
	MaxNoteEvidenceIDs   = 64
	MaxNoteEvidenceRunes = 128
)

type NoteCategory string

const (
	NoteObservation NoteCategory = "observation"
	NoteHypothesis  NoteCategory = "hypothesis"
	NoteDecision    NoteCategory = "decision"
	NoteSummary     NoteCategory = "summary"
	NoteReference   NoteCategory = "reference"
)

type NoteVisibility string

const (
	NoteVisibilityRun   NoteVisibility = "run"
	NoteVisibilityRoot  NoteVisibility = "root"
	NoteVisibilityOwner NoteVisibility = "owner"
)

type NoteStatus string

const (
	NoteActive   NoteStatus = "active"
	NoteArchived NoteStatus = "archived"
)

type Note struct {
	ID          string
	RunID       string
	Title       string
	Content     string
	Category    NoteCategory
	Visibility  NoteVisibility
	Owner       string
	Tags        []string
	SourceRefs  []string
	EvidenceIDs []string
	Status      NoteStatus
	Pinned      bool
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

type NoteDetails struct {
	Title       string
	Content     string
	Category    NoteCategory
	Visibility  NoteVisibility
	Owner       string
	Tags        []string
	SourceRefs  []string
	EvidenceIDs []string
	Pinned      bool
}

type NoteFilter struct {
	RunID        string
	Statuses     []NoteStatus
	Categories   []NoteCategory
	Visibilities []NoteVisibility
	Owner        string
	Viewer       string
	Tags         []string
	Pinned       *bool
	Limit        int
}

func ParseNoteCategory(value string) (NoteCategory, error) {
	category := NoteCategory(strings.ToLower(strings.TrimSpace(value)))
	if category == "" {
		return NoteObservation, nil
	}
	if !ValidNoteCategory(category) {
		return "", fmt.Errorf("invalid note category %q", value)
	}
	return category, nil
}

func ValidNoteCategory(category NoteCategory) bool {
	switch category {
	case NoteObservation, NoteHypothesis, NoteDecision, NoteSummary, NoteReference:
		return true
	default:
		return false
	}
}

func ParseNoteVisibility(value string) (NoteVisibility, error) {
	visibility := NoteVisibility(strings.ToLower(strings.TrimSpace(value)))
	if visibility == "" {
		return NoteVisibilityRun, nil
	}
	if !ValidNoteVisibility(visibility) {
		return "", fmt.Errorf("invalid note visibility %q", value)
	}
	return visibility, nil
}

func ValidNoteVisibility(visibility NoteVisibility) bool {
	switch visibility {
	case NoteVisibilityRun, NoteVisibilityRoot, NoteVisibilityOwner:
		return true
	default:
		return false
	}
}

func ParseNoteStatus(value string) (NoteStatus, error) {
	status := NoteStatus(strings.ToLower(strings.TrimSpace(value)))
	if !ValidNoteStatus(status) {
		return "", fmt.Errorf("invalid note status %q", value)
	}
	return status, nil
}

func ValidNoteStatus(status NoteStatus) bool {
	return status == NoteActive || status == NoteArchived
}

func (n *Note) Transition(to NoteStatus, at time.Time) error {
	if n == nil {
		return errors.New("note is nil")
	}
	if !ValidNoteStatus(to) {
		return fmt.Errorf("invalid note status %q", to)
	}
	if n.Status == to {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	switch {
	case n.Status == NoteActive && to == NoteArchived:
		archived := at
		n.ArchivedAt = &archived
	case n.Status == NoteArchived && to == NoteActive:
		n.ArchivedAt = nil
	default:
		return fmt.Errorf("note cannot transition from %s to %s", n.Status, to)
	}
	n.Status = to
	n.UpdatedAt = at
	return nil
}

func (n *Note) ApplyDetails(details NoteDetails, at time.Time) error {
	if n == nil {
		return errors.New("note is nil")
	}
	if n.Status != NoteActive {
		return fmt.Errorf("archived note %s cannot be updated", n.ID)
	}
	normalized, err := NormalizeNoteDetails(details)
	if err != nil {
		return err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	n.Title = normalized.Title
	n.Content = normalized.Content
	n.Category = normalized.Category
	n.Visibility = normalized.Visibility
	n.Owner = normalized.Owner
	n.Tags = normalized.Tags
	n.SourceRefs = normalized.SourceRefs
	n.EvidenceIDs = normalized.EvidenceIDs
	n.Pinned = normalized.Pinned
	n.UpdatedAt = at
	return nil
}

func (n Note) Validate() error {
	if strings.TrimSpace(n.ID) == "" {
		return errors.New("note id is required")
	}
	if strings.TrimSpace(n.RunID) == "" {
		return errors.New("note run id is required")
	}
	if !ValidNoteStatus(n.Status) {
		return fmt.Errorf("invalid note status %q", n.Status)
	}
	if n.Version <= 0 {
		return errors.New("note version must be positive")
	}
	if n.CreatedAt.IsZero() || n.UpdatedAt.IsZero() {
		return errors.New("note timestamps are required")
	}
	if n.UpdatedAt.Before(n.CreatedAt) {
		return errors.New("note updated_at cannot precede created_at")
	}
	details, err := NormalizeNoteDetails(noteDetails(n))
	if err != nil {
		return err
	}
	if details.Title != n.Title || details.Content != n.Content || details.Category != n.Category ||
		details.Visibility != n.Visibility || details.Owner != n.Owner ||
		!slices.Equal(details.Tags, n.Tags) || !slices.Equal(details.SourceRefs, n.SourceRefs) ||
		!slices.Equal(details.EvidenceIDs, n.EvidenceIDs) {
		return errors.New("note text, tags, and references must be normalized")
	}
	switch n.Status {
	case NoteActive:
		if n.ArchivedAt != nil {
			return errors.New("active note cannot have archived_at")
		}
	case NoteArchived:
		if n.ArchivedAt == nil || n.ArchivedAt.IsZero() {
			return errors.New("archived note requires archived_at")
		}
		if n.ArchivedAt.Before(n.CreatedAt) {
			return errors.New("note archived_at cannot precede created_at")
		}
	}
	return nil
}

func NormalizeNoteDetails(details NoteDetails) (NoteDetails, error) {
	details.Title = strings.TrimSpace(details.Title)
	details.Content = strings.TrimSpace(details.Content)
	details.Owner = strings.TrimSpace(details.Owner)
	if !utf8.ValidString(details.Title) {
		return NoteDetails{}, errors.New("note title must be valid UTF-8")
	}
	if !utf8.ValidString(details.Content) {
		return NoteDetails{}, errors.New("note content must be valid UTF-8")
	}
	if !utf8.ValidString(details.Owner) {
		return NoteDetails{}, errors.New("note owner must be valid UTF-8")
	}
	if details.Category == "" {
		details.Category = NoteObservation
	}
	if details.Visibility == "" {
		details.Visibility = NoteVisibilityRun
	}
	if details.Title == "" {
		return NoteDetails{}, errors.New("note title is required")
	}
	if runeCount(details.Title) > MaxNoteTitleRunes {
		return NoteDetails{}, fmt.Errorf("note title exceeds %d characters", MaxNoteTitleRunes)
	}
	if details.Content == "" {
		return NoteDetails{}, errors.New("note content is required")
	}
	if len([]byte(details.Content)) > MaxNoteContentBytes {
		return NoteDetails{}, fmt.Errorf("note content exceeds %d bytes", MaxNoteContentBytes)
	}
	if !ValidNoteCategory(details.Category) {
		return NoteDetails{}, fmt.Errorf("invalid note category %q", details.Category)
	}
	if !ValidNoteVisibility(details.Visibility) {
		return NoteDetails{}, fmt.Errorf("invalid note visibility %q", details.Visibility)
	}
	if runeCount(details.Owner) > MaxNoteOwnerRunes {
		return NoteDetails{}, fmt.Errorf("note owner exceeds %d characters", MaxNoteOwnerRunes)
	}
	if details.Visibility == NoteVisibilityOwner && details.Owner == "" {
		return NoteDetails{}, errors.New("owner-visible note requires an owner")
	}
	if details.Visibility != NoteVisibilityOwner && details.Owner != "" {
		return NoteDetails{}, errors.New("note owner requires owner visibility")
	}
	var err error
	details.Tags, err = NormalizeNoteTags(details.Tags)
	if err != nil {
		return NoteDetails{}, err
	}
	details.SourceRefs, err = NormalizeNoteSourceRefs(details.SourceRefs)
	if err != nil {
		return NoteDetails{}, err
	}
	details.EvidenceIDs, err = NormalizeNoteEvidenceIDs(details.EvidenceIDs)
	if err != nil {
		return NoteDetails{}, err
	}
	return details, nil
}

func NormalizeNoteTags(values []string) ([]string, error) {
	return normalizeNoteList(values, MaxNoteTags, MaxNoteTagRunes, "tag", true, false)
}

func NormalizeNoteSourceRefs(values []string) ([]string, error) {
	return normalizeNoteList(values, MaxNoteSources, MaxNoteSourceRunes, "source reference", false, false)
}

func NormalizeNoteEvidenceIDs(values []string) ([]string, error) {
	return normalizeNoteList(values, MaxNoteEvidenceIDs, MaxNoteEvidenceRunes, "evidence id", false, true)
}

func normalizeNoteList(values []string, maxItems int, maxRunes int, label string, lower bool, rejectWhitespace bool) ([]string, error) {
	if len(values) > maxItems {
		return nil, fmt.Errorf("note %s list exceeds %d items", label, maxItems)
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !utf8.ValidString(value) {
			return nil, fmt.Errorf("note %s must be valid UTF-8", label)
		}
		value = strings.TrimSpace(value)
		if label != "evidence id" {
			value = strings.Join(strings.Fields(value), " ")
		}
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" {
			return nil, fmt.Errorf("note %s cannot be empty", label)
		}
		if runeCount(value) > maxRunes {
			return nil, fmt.Errorf("note %s exceeds %d characters", label, maxRunes)
		}
		if rejectWhitespace && strings.IndexFunc(value, unicode.IsSpace) >= 0 {
			return nil, fmt.Errorf("note %s cannot contain whitespace", label)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func noteDetails(note Note) NoteDetails {
	return NoteDetails{
		Title: note.Title, Content: note.Content, Category: note.Category, Visibility: note.Visibility,
		Owner: note.Owner, Tags: note.Tags, SourceRefs: note.SourceRefs, EvidenceIDs: note.EvidenceIDs, Pinned: note.Pinned,
	}
}
