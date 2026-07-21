package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/session"
)

const (
	maxTUIFileEdits        = 20
	maxTUIDiffDisplayBytes = 128 * 1024
	maxTUIDiffDisplayLines = 4096
	maxTUIEditPathRunes    = 1024
	maxTUIEditReasonBytes  = 64 * 1024
)

type fileEditContext struct {
	Preview       fileedit.Preview
	DiffLines     []string
	DiffTruncated bool
}

func loadFileEditContext(ctx context.Context, stateStore RunStateStore,
	run domain.Run, sess session.Session,
) ([]fileEditContext, bool, error) {
	previews, err := stateStore.ListFileEditPreviewsPage(ctx, fileedit.ListFilter{
		SessionID: sess.ID, WorkspaceID: sess.WorkspaceID,
	}, 0, maxTUIFileEdits+1)
	if err != nil {
		return nil, false, err
	}
	truncated := len(previews) > maxTUIFileEdits
	if truncated {
		previews = previews[:maxTUIFileEdits]
	}
	out := make([]fileEditContext, 0, len(previews))
	for _, preview := range previews {
		if err := validateTUIFileEditPreview(preview, run, sess); err != nil {
			return nil, false, err
		}
		lines, diffTruncated := boundedTUIDiffLines(preview.Diff)
		preview.Diff = ""
		out = append(out, fileEditContext{
			Preview: preview, DiffLines: lines, DiffTruncated: diffTruncated,
		})
	}
	return out, truncated, nil
}

func validateTUIFileEditPreview(preview fileedit.Preview, run domain.Run,
	sess session.Session,
) error {
	if run.SessionID != sess.ID || preview.SessionID != sess.ID ||
		preview.WorkspaceID != sess.WorkspaceID {
		return errors.New("TUI FileEdit projection is cross-scope")
	}
	if !boundedTUIIdentity(preview.ID) || !boundedTUIIdentity(preview.SessionID) ||
		!boundedTUIIdentity(preview.WorkspaceID) || !fileedit.ValidStatus(preview.Status) {
		return errors.New("TUI FileEdit projection has invalid identity or status")
	}
	if !validTUIEditPath(preview.Path) {
		return errors.New("TUI FileEdit projection has an invalid path")
	}
	if !utf8.ValidString(preview.Diff) || len(preview.Diff) > fileedit.MaxDiffBytes ||
		!utf8.ValidString(preview.Reason) || len(preview.Reason) > maxTUIEditReasonBytes ||
		preview.CreatedAt.IsZero() || preview.UpdatedAt.IsZero() {
		return errors.New("TUI FileEdit projection has invalid or unbounded metadata")
	}
	return nil
}

func boundedTUIIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		utf8.RuneCountInString(value) <= maxTUIEventIdentityRunes &&
		!strings.ContainsRune(value, 0)
}

func validTUIEditPath(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maxTUIEditPathRunes || strings.ContainsRune(value, 0) ||
		filepath.IsAbs(value) {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	return clean != "." && clean != ".." &&
		!strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func boundedTUIDiffLines(value string) ([]string, bool) {
	truncated := false
	if len(value) > maxTUIDiffDisplayBytes {
		value = validUTF8Prefix(value, maxTUIDiffDisplayBytes)
		truncated = true
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	parts := strings.SplitN(value, "\n", maxTUIDiffDisplayLines+1)
	if len(parts) > maxTUIDiffDisplayLines {
		parts = parts[:maxTUIDiffDisplayLines]
		truncated = true
	}
	for index := range parts {
		parts[index] = terminalSafeText(strings.TrimSuffix(parts[index], "\r"))
	}
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if truncated {
		parts = append(parts, "[diff truncated by TUI display bounds]")
	}
	if len(parts) == 0 {
		parts = []string{"[empty diff]"}
	}
	return parts, truncated
}

func validUTF8Prefix(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}

func (m *Model) renderEdits(width int, height int) string {
	label := fmt.Sprintf("File Edits %d", len(m.runContext.FileEdits))
	if m.runContext.FileEditsTruncated {
		label += fmt.Sprintf(" latest=%d", maxTUIFileEdits)
	}
	lines := m.activityHeader(label, activityEdits, width)
	if !m.runContext.Found {
		lines = append(lines, "no Run attached")
		return strings.Join(lines, "\n")
	}
	if len(m.runContext.FileEdits) == 0 {
		lines = append(lines, "none")
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-len(lines)-3)
	m.ensureEditVisible(visible)
	end := min(len(m.runContext.FileEdits), m.editScroll+visible)
	for index := m.editScroll; index < end; index++ {
		preview := m.runContext.FileEdits[index].Preview
		marker := " "
		if index == m.selectedEdit {
			marker = ">"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s %s", marker,
			preview.Status, preview.Path), width))
	}
	selected := m.runContext.FileEdits[m.selectedEdit]
	lines = append(lines, truncate("edit="+selected.Preview.ID+
		" redacted="+fmt.Sprint(selected.Preview.SecretsRedacted), width))
	lines = append(lines, "Enter: read-only diff")
	return strings.Join(windowTop(lines, height+1), "\n")
}

func (m *Model) openSelectedEditDetail() {
	if len(m.runContext.FileEdits) == 0 {
		m.status = "no file edits"
		return
	}
	m.normalizeActivitySelection()
	m.editDetailOpen = true
	m.editDetailScroll = 0
	m.status = "read-only file diff"
}

func (m *Model) scrollEditDetail(delta int) {
	if len(m.runContext.FileEdits) == 0 {
		m.editDetailScroll = 0
		return
	}
	m.editDetailScroll += delta
	if m.editDetailScroll < 0 {
		m.editDetailScroll = 0
	}
	maxScroll := max(0, len(m.runContext.FileEdits[m.selectedEdit].DiffLines)-1)
	if m.editDetailScroll > maxScroll {
		m.editDetailScroll = maxScroll
	}
	m.status = fmt.Sprintf("diff scroll: %d", m.editDetailScroll)
}

func (m *Model) renderEditDetailScreen() string {
	width := max(48, m.width)
	height := max(14, m.height)
	if len(m.runContext.FileEdits) == 0 {
		m.editDetailOpen = false
		return m.Snapshot()
	}
	m.normalizeActivitySelection()
	selected := m.runContext.FileEdits[m.selectedEdit]
	preview := selected.Preview
	header := headerStyle.Width(width).Render(truncate(
		"Prayu  read-only diff  "+preview.Path, width-2))
	meta := []string{
		truncate(fmt.Sprintf("edit=%s status=%s redacted=%t", preview.ID,
			preview.Status, preview.SecretsRedacted), width-4),
		truncate("original="+shortHash(preview.OriginalHash)+
			" proposed="+shortHash(preview.ProposedHash), width-4),
	}
	if strings.TrimSpace(preview.Reason) != "" {
		meta = append(meta, truncate("reason="+singleLine(preview.Reason), width-4))
	}
	bodyHeight := max(4, height-len(meta)-6)
	maxStart := max(0, len(selected.DiffLines)-bodyHeight)
	if m.editDetailScroll > maxStart {
		m.editDetailScroll = maxStart
	}
	end := min(len(selected.DiffLines), m.editDetailScroll+bodyHeight)
	diff := make([]string, 0, end-m.editDetailScroll)
	for _, line := range selected.DiffLines[m.editDetailScroll:end] {
		diff = append(diff, truncate(line, width-4))
	}
	content := append(meta, diff...)
	panel := panelStyle.Width(width).Height(height - 5).Render(strings.Join(content, "\n"))
	statusText := fmt.Sprintf(
		"read-only | lines %d-%d/%d | no approval or write authority",
		m.editDetailScroll+1, end, len(selected.DiffLines))
	status := statusStyle.Width(width).Render(truncate(statusText, max(20, width-2)))
	footer := footerStyle.Width(width).Render(truncate(
		"j/k or PgUp/PgDn scroll | Enter/b/Esc back | Ctrl+C quit", max(20, width-2)))
	return lipgloss.JoinVertical(lipgloss.Left, header, panel, status, footer)
}

func shortHash(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
