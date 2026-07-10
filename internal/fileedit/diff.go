package fileedit

import (
	"fmt"
	"strings"
)

const diffContextLines = 3

// UnifiedDiff emits one bounded unified-style hunk. It keeps generation
// linear in file size so adversarial files cannot trigger quadratic memory use.
func UnifiedDiff(path string, oldText string, newText string) string {
	oldLines := diffLines(oldText)
	newLines := diffLines(newText)
	prefix := commonPrefix(oldLines, newLines)
	suffix := commonSuffix(oldLines[prefix:], newLines[prefix:])
	if oldText != newText && prefix == len(oldLines) && prefix == len(newLines) {
		prefix = 0
		suffix = 0
	}

	contextStart := max(0, prefix-diffContextLines)
	oldChangedEnd := len(oldLines) - suffix
	newChangedEnd := len(newLines) - suffix
	suffixContext := min(diffContextLines, suffix)
	oldEnd := oldChangedEnd + suffixContext
	newEnd := newChangedEnd + suffixContext
	oldCount := oldEnd - contextStart
	newCount := newEnd - contextStart

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n", path)
	fmt.Fprintf(&out, "+++ b/%s\n", path)
	fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", rangeStart(contextStart, oldCount), oldCount, rangeStart(contextStart, newCount), newCount)
	for _, line := range oldLines[contextStart:prefix] {
		fmt.Fprintf(&out, " %s\n", line)
	}
	for _, line := range oldLines[prefix:oldChangedEnd] {
		fmt.Fprintf(&out, "-%s\n", line)
	}
	for _, line := range newLines[prefix:newChangedEnd] {
		fmt.Fprintf(&out, "+%s\n", line)
	}
	for i := 0; i < suffixContext; i++ {
		fmt.Fprintf(&out, " %s\n", oldLines[oldChangedEnd+i])
	}
	return out.String()
}

func redactedChangeDiff(path string) string {
	return fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1,1 +1,1 @@\n-[existing sensitive content omitted]\n+[replacement content retained after redaction]\n", path, path)
}

func diffLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func commonPrefix(a []string, b []string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func commonSuffix(a []string, b []string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return limit
}

func rangeStart(zeroBased int, count int) int {
	if count == 0 {
		return 0
	}
	return zeroBased + 1
}
