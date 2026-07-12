package report

import (
	"fmt"
	"strconv"
	"strings"

	"cyberagent-workbench/internal/domain"
)

func RenderGitHubAnnotations(result GateResult) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	var output strings.Builder
	for _, match := range result.Matches {
		command, err := githubAnnotationCommand(match.Severity)
		if err != nil {
			return nil, err
		}
		properties := []string{
			"file=" + escapeGitHubCommandProperty(match.RelativePath),
		}
		if match.LineStart > 0 {
			properties = append(properties,
				"line="+strconv.Itoa(match.LineStart),
				"endLine="+strconv.Itoa(match.LineEnd))
		}
		title := fmt.Sprintf("CyberAgent %s: %s",
			strings.ToUpper(string(match.Severity)), match.Title)
		properties = append(properties,
			"title="+escapeGitHubCommandProperty(title))
		message := fmt.Sprintf("%s [status=%s; category=%s; finding=%s; fingerprint=%s]",
			match.Detail, match.Status, match.Category, match.FindingID,
			match.Fingerprint)
		output.WriteString("::")
		output.WriteString(command)
		output.WriteByte(' ')
		output.WriteString(strings.Join(properties, ","))
		output.WriteString("::")
		output.WriteString(escapeGitHubCommandData(message))
		output.WriteByte('\n')
	}
	return []byte(output.String()), nil
}

func githubAnnotationCommand(severity domain.FindingSeverity) (string, error) {
	switch severity {
	case domain.FindingSeverityInfo, domain.FindingSeverityLow:
		return "notice", nil
	case domain.FindingSeverityMedium:
		return "warning", nil
	case domain.FindingSeverityHigh, domain.FindingSeverityCritical:
		return "error", nil
	default:
		return "", fmt.Errorf("unsupported GitHub annotation severity %q", severity)
	}
}

func escapeGitHubCommandData(value string) string {
	value = escapeGitHubControlCharacters(value)
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	).Replace(value)
}

func escapeGitHubCommandProperty(value string) string {
	value = escapeGitHubControlCharacters(value)
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		":", "%3A",
		",", "%2C",
	).Replace(value)
}

func escapeGitHubControlCharacters(value string) string {
	var output strings.Builder
	for _, current := range value {
		if (current >= 0 && current < 0x20 && current != '\r' && current != '\n') ||
			current == 0x7f {
			fmt.Fprintf(&output, "\\u%04X", current)
			continue
		}
		output.WriteRune(current)
	}
	return output.String()
}
