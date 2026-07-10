package contextmgr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/redact"
)

type Message struct {
	Role      string
	Content   string
	CreatedAt time.Time
}

type Summary struct {
	ID                    int64
	TaskID                string
	WorkspaceID           string
	Content               string
	SourceMessageCount    int
	PreservedMessageCount int
	TokenEstimate         int
	CreatedAt             time.Time
}

type Result struct {
	Compacted       bool
	Summary         Summary
	Preserved       []Message
	RemovedMessages int
}

type Config struct {
	MaxMessagesBeforeCompact int
	PreserveRecentMessages   int
	MaxSummaryChars          int
	MaxLineChars             int
}

type SummaryStore interface {
	SaveContextSummary(ctx context.Context, summary Summary) (Summary, error)
	LatestContextSummary(ctx context.Context, taskID string) (Summary, bool, error)
}

type Manager struct {
	store  SummaryStore
	config Config
}

func NewManager(store SummaryStore, config Config) *Manager {
	return &Manager{store: store, config: config.withDefaults()}
}

func DefaultConfig() Config {
	return Config{
		MaxMessagesBeforeCompact: 8,
		PreserveRecentMessages:   4,
		MaxSummaryChars:          4000,
		MaxLineChars:             220,
	}
}

func (c Config) withDefaults() Config {
	defaults := DefaultConfig()
	if c.MaxMessagesBeforeCompact <= 0 {
		c.MaxMessagesBeforeCompact = defaults.MaxMessagesBeforeCompact
	}
	if c.PreserveRecentMessages <= 0 {
		c.PreserveRecentMessages = defaults.PreserveRecentMessages
	}
	if c.MaxSummaryChars <= 0 {
		c.MaxSummaryChars = defaults.MaxSummaryChars
	}
	if c.MaxLineChars <= 0 {
		c.MaxLineChars = defaults.MaxLineChars
	}
	if c.PreserveRecentMessages > c.MaxMessagesBeforeCompact {
		c.PreserveRecentMessages = c.MaxMessagesBeforeCompact
	}
	return c
}

func (m *Manager) ShouldCompact(messages []Message) bool {
	return len(messages) > m.config.MaxMessagesBeforeCompact
}

func (m *Manager) MaybeCompact(ctx context.Context, taskID string, workspaceID string, messages []Message) (Result, error) {
	if !m.ShouldCompact(messages) {
		return Result{Compacted: false, Preserved: redactMessages(messages)}, nil
	}
	return m.Compact(ctx, taskID, workspaceID, messages)
}

func (m *Manager) Compact(ctx context.Context, taskID string, workspaceID string, messages []Message) (Result, error) {
	if strings.TrimSpace(taskID) == "" {
		return Result{}, errors.New("task id is required")
	}
	if len(messages) == 0 {
		return Result{}, errors.New("at least one message is required")
	}

	preserveCount := m.config.PreserveRecentMessages
	if preserveCount > len(messages) {
		preserveCount = len(messages)
	}
	if preserveCount == len(messages) && len(messages) > 0 {
		preserveCount = len(messages) - 1
	}
	removeCount := len(messages) - preserveCount
	older := messages[:removeCount]
	preserved := redactMessages(messages[removeCount:])

	content := m.buildSummary(taskID, workspaceID, older, preserved, len(messages))
	summary := Summary{
		TaskID:                taskID,
		WorkspaceID:           workspaceID,
		Content:               content,
		SourceMessageCount:    len(messages),
		PreservedMessageCount: len(preserved),
		TokenEstimate:         EstimateTokens(content),
		CreatedAt:             time.Now().UTC(),
	}

	if m.store != nil {
		saved, err := m.store.SaveContextSummary(ctx, summary)
		if err != nil {
			return Result{}, err
		}
		summary = saved
	}

	return Result{
		Compacted:       true,
		Summary:         summary,
		Preserved:       preserved,
		RemovedMessages: removeCount,
	}, nil
}

func (m *Manager) Latest(ctx context.Context, taskID string) (Summary, bool, error) {
	if m.store == nil {
		return Summary{}, false, errors.New("summary store is not configured")
	}
	return m.store.LatestContextSummary(ctx, taskID)
}

func (m *Manager) BuildPrompt(system string, summary Summary, recent []Message) []Message {
	out := make([]Message, 0, len(recent)+2)
	if strings.TrimSpace(system) != "" {
		out = append(out, Message{Role: "system", Content: system})
	}
	if strings.TrimSpace(summary.Content) != "" {
		out = append(out, Message{
			Role:    "system",
			Content: "Compacted prior context:\n" + redact.String(summary.Content),
		})
	}
	out = append(out, redactMessages(recent)...)
	return out
}

func (m *Manager) buildSummary(taskID string, workspaceID string, older []Message, preserved []Message, total int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Context summary for task %s", taskID)
	if workspaceID != "" {
		fmt.Fprintf(&b, " in workspace %s", workspaceID)
	}
	fmt.Fprintln(&b, ".")
	fmt.Fprintf(&b, "Source messages: %d. Removed into summary: %d. Recent messages preserved outside summary: %d.\n", total, len(older), len(preserved))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Condensed prior conversation:")
	for i, msg := range older {
		role := normalizeRole(msg.Role)
		line := collapseWhitespace(redact.String(msg.Content))
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, "- %02d %s: %s\n", i+1, role, trimRunes(line, m.config.MaxLineChars))
	}
	content := redact.String(b.String())
	return trimRunes(content, m.config.MaxSummaryChars)
}

func ParseMessage(raw string) Message {
	raw = strings.TrimSpace(raw)
	role := "user"
	content := raw
	if before, after, ok := strings.Cut(raw, ":"); ok {
		candidate := strings.ToLower(strings.TrimSpace(before))
		if isKnownRole(candidate) {
			role = candidate
			content = strings.TrimSpace(after)
		}
	}
	return Message{Role: role, Content: redact.String(content), CreatedAt: time.Now().UTC()}
}

func EstimateTokens(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	words := len(strings.Fields(value))
	chars := utf8.RuneCountInString(value)
	byChars := chars / 4
	if chars%4 != 0 {
		byChars++
	}
	if words > byChars {
		return words
	}
	return byChars
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if isKnownRole(role) {
		return role
	}
	return "user"
}

func isKnownRole(role string) bool {
	switch role {
	case "system", "user", "assistant", "tool":
		return true
	default:
		return false
	}
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func trimRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func copyMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}

func redactMessages(messages []Message) []Message {
	out := copyMessages(messages)
	for i := range out {
		out[i].Content = redact.String(out[i].Content)
	}
	return out
}
