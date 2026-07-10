package domain

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	MaxWorkItemTitleRunes      = 240
	MaxWorkItemTextRunes       = 8192
	MaxWorkItemOwnerRunes      = 128
	MaxWorkItemAcceptanceItems = 32
	MaxWorkItemDependencies    = 64
)

type WorkItemStatus string

const (
	WorkItemPending    WorkItemStatus = "pending"
	WorkItemInProgress WorkItemStatus = "in_progress"
	WorkItemBlocked    WorkItemStatus = "blocked"
	WorkItemCompleted  WorkItemStatus = "completed"
	WorkItemCancelled  WorkItemStatus = "cancelled"
)

type WorkItemPriority string

const (
	WorkItemPriorityLow      WorkItemPriority = "low"
	WorkItemPriorityNormal   WorkItemPriority = "normal"
	WorkItemPriorityHigh     WorkItemPriority = "high"
	WorkItemPriorityCritical WorkItemPriority = "critical"
)

type WorkItem struct {
	ID                 string
	RunID              string
	Title              string
	Description        string
	Status             WorkItemStatus
	Priority           WorkItemPriority
	Owner              string
	AcceptanceCriteria []string
	Dependencies       []string
	BlockedReason      string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
}

type WorkItemDetails struct {
	Title              string
	Description        string
	Priority           WorkItemPriority
	Owner              string
	AcceptanceCriteria []string
	Dependencies       []string
}

type WorkItemFilter struct {
	RunID    string
	Statuses []WorkItemStatus
	Owner    string
	Limit    int
}

func ParseWorkItemStatus(value string) (WorkItemStatus, error) {
	status := WorkItemStatus(strings.ToLower(strings.TrimSpace(value)))
	if !ValidWorkItemStatus(status) {
		return "", fmt.Errorf("invalid work item status %q", value)
	}
	return status, nil
}

func ValidWorkItemStatus(status WorkItemStatus) bool {
	switch status {
	case WorkItemPending, WorkItemInProgress, WorkItemBlocked, WorkItemCompleted, WorkItemCancelled:
		return true
	default:
		return false
	}
}

func ParseWorkItemPriority(value string) (WorkItemPriority, error) {
	priority := WorkItemPriority(strings.ToLower(strings.TrimSpace(value)))
	if priority == "" {
		return WorkItemPriorityNormal, nil
	}
	if !ValidWorkItemPriority(priority) {
		return "", fmt.Errorf("invalid work item priority %q", value)
	}
	return priority, nil
}

func ValidWorkItemPriority(priority WorkItemPriority) bool {
	switch priority {
	case WorkItemPriorityLow, WorkItemPriorityNormal, WorkItemPriorityHigh, WorkItemPriorityCritical:
		return true
	default:
		return false
	}
}

func (w WorkItem) Terminal() bool {
	return w.Status == WorkItemCompleted || w.Status == WorkItemCancelled
}

func (w WorkItem) CanTransition(to WorkItemStatus) bool {
	if w.Status == to {
		return true
	}
	allowed := map[WorkItemStatus]map[WorkItemStatus]bool{
		WorkItemPending: {
			WorkItemInProgress: true, WorkItemBlocked: true, WorkItemCompleted: true, WorkItemCancelled: true,
		},
		WorkItemInProgress: {
			WorkItemPending: true, WorkItemBlocked: true, WorkItemCompleted: true, WorkItemCancelled: true,
		},
		WorkItemBlocked: {
			WorkItemPending: true, WorkItemInProgress: true, WorkItemCancelled: true,
		},
	}
	return allowed[w.Status][to]
}

func (w *WorkItem) Transition(to WorkItemStatus, reason string, at time.Time) error {
	if w == nil {
		return errors.New("work item is nil")
	}
	if !ValidWorkItemStatus(to) {
		return fmt.Errorf("invalid work item status %q", to)
	}
	if w.Status == to {
		if to == WorkItemBlocked && strings.TrimSpace(reason) != "" && strings.TrimSpace(reason) != w.BlockedReason {
			return errors.New("use a work item update to change an existing blocked reason")
		}
		return nil
	}
	if !w.CanTransition(to) {
		return fmt.Errorf("work item cannot transition from %s to %s", w.Status, to)
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	reason = strings.TrimSpace(reason)
	if to == WorkItemBlocked && reason == "" {
		return errors.New("blocked work item requires a reason")
	}
	if runeCount(reason) > MaxWorkItemTextRunes {
		return fmt.Errorf("work item blocked reason exceeds %d characters", MaxWorkItemTextRunes)
	}
	w.Status = to
	w.UpdatedAt = at
	w.BlockedReason = ""
	w.CompletedAt = nil
	if to == WorkItemBlocked {
		w.BlockedReason = reason
	}
	if to == WorkItemCompleted {
		completed := at
		w.CompletedAt = &completed
	}
	return nil
}

func (w *WorkItem) ApplyDetails(details WorkItemDetails, at time.Time) error {
	if w == nil {
		return errors.New("work item is nil")
	}
	if w.Terminal() {
		return fmt.Errorf("terminal work item %s cannot be updated", w.ID)
	}
	normalized, err := NormalizeWorkItemDetails(w.ID, details)
	if err != nil {
		return err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	w.Title = normalized.Title
	w.Description = normalized.Description
	w.Priority = normalized.Priority
	w.Owner = normalized.Owner
	w.AcceptanceCriteria = normalized.AcceptanceCriteria
	w.Dependencies = normalized.Dependencies
	w.UpdatedAt = at
	return nil
}

func (w WorkItem) Validate() error {
	if strings.TrimSpace(w.ID) == "" {
		return errors.New("work item id is required")
	}
	if strings.TrimSpace(w.RunID) == "" {
		return errors.New("work item run id is required")
	}
	if !ValidWorkItemStatus(w.Status) {
		return fmt.Errorf("invalid work item status %q", w.Status)
	}
	if !ValidWorkItemPriority(w.Priority) {
		return fmt.Errorf("invalid work item priority %q", w.Priority)
	}
	if w.Version <= 0 {
		return errors.New("work item version must be positive")
	}
	if w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() {
		return errors.New("work item timestamps are required")
	}
	if w.UpdatedAt.Before(w.CreatedAt) {
		return errors.New("work item updated_at cannot precede created_at")
	}
	details, err := NormalizeWorkItemDetails(w.ID, WorkItemDetails{
		Title: w.Title, Description: w.Description, Priority: w.Priority, Owner: w.Owner,
		AcceptanceCriteria: w.AcceptanceCriteria, Dependencies: w.Dependencies,
	})
	if err != nil {
		return err
	}
	if details.Title != w.Title || details.Description != w.Description || details.Owner != w.Owner ||
		!slices.Equal(details.AcceptanceCriteria, w.AcceptanceCriteria) || !slices.Equal(details.Dependencies, w.Dependencies) {
		return errors.New("work item text and dependencies must be normalized")
	}
	switch w.Status {
	case WorkItemBlocked:
		if strings.TrimSpace(w.BlockedReason) == "" {
			return errors.New("blocked work item requires a reason")
		}
		if w.CompletedAt != nil {
			return errors.New("blocked work item cannot have completed_at")
		}
	case WorkItemCompleted:
		if w.CompletedAt == nil || w.CompletedAt.IsZero() {
			return errors.New("completed work item requires completed_at")
		}
		if w.CompletedAt.Before(w.CreatedAt) {
			return errors.New("work item completed_at cannot precede created_at")
		}
		if strings.TrimSpace(w.BlockedReason) != "" {
			return errors.New("completed work item cannot have a blocked reason")
		}
	default:
		if strings.TrimSpace(w.BlockedReason) != "" {
			return fmt.Errorf("work item status %s cannot have a blocked reason", w.Status)
		}
		if w.CompletedAt != nil {
			return fmt.Errorf("work item status %s cannot have completed_at", w.Status)
		}
	}
	return nil
}

func NormalizeWorkItemDetails(itemID string, details WorkItemDetails) (WorkItemDetails, error) {
	details.Title = strings.TrimSpace(details.Title)
	details.Description = strings.TrimSpace(details.Description)
	details.Owner = strings.TrimSpace(details.Owner)
	if details.Priority == "" {
		details.Priority = WorkItemPriorityNormal
	}
	if details.Title == "" {
		return WorkItemDetails{}, errors.New("work item title is required")
	}
	if runeCount(details.Title) > MaxWorkItemTitleRunes {
		return WorkItemDetails{}, fmt.Errorf("work item title exceeds %d characters", MaxWorkItemTitleRunes)
	}
	if runeCount(details.Description) > MaxWorkItemTextRunes {
		return WorkItemDetails{}, fmt.Errorf("work item description exceeds %d characters", MaxWorkItemTextRunes)
	}
	if runeCount(details.Owner) > MaxWorkItemOwnerRunes {
		return WorkItemDetails{}, fmt.Errorf("work item owner exceeds %d characters", MaxWorkItemOwnerRunes)
	}
	if !ValidWorkItemPriority(details.Priority) {
		return WorkItemDetails{}, fmt.Errorf("invalid work item priority %q", details.Priority)
	}
	criteria, err := normalizeWorkItemStrings(details.AcceptanceCriteria, MaxWorkItemAcceptanceItems, MaxWorkItemTextRunes, "acceptance criterion")
	if err != nil {
		return WorkItemDetails{}, err
	}
	dependencies, err := normalizeWorkItemStrings(details.Dependencies, MaxWorkItemDependencies, MaxWorkItemOwnerRunes, "dependency")
	if err != nil {
		return WorkItemDetails{}, err
	}
	itemID = strings.TrimSpace(itemID)
	if itemID != "" && slices.Contains(dependencies, itemID) {
		return WorkItemDetails{}, errors.New("work item cannot depend on itself")
	}
	details.AcceptanceCriteria = criteria
	details.Dependencies = dependencies
	return details, nil
}

func normalizeWorkItemStrings(values []string, maxItems int, maxRunes int, label string) ([]string, error) {
	if len(values) > maxItems {
		return nil, fmt.Errorf("work item %s list exceeds %d items", label, maxItems)
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("work item %s cannot be empty", label)
		}
		if runeCount(value) > maxRunes {
			return nil, fmt.Errorf("work item %s exceeds %d characters", label, maxRunes)
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

func runeCount(value string) int {
	return len([]rune(value))
}
