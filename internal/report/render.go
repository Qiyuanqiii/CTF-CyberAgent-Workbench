package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"

	"cyberagent-workbench/internal/domain"
)

type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

func ParseFormat(value string) (Format, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "md" {
		value = string(FormatMarkdown)
	}
	format := Format(value)
	if format != FormatMarkdown && format != FormatJSON {
		return "", errors.New("report format must be markdown or json")
	}
	return format, nil
}

func Render(value domain.FindingReport, format Format) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	switch format {
	case FormatJSON:
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(encoded, '\n'), nil
	case FormatMarkdown:
		return []byte(renderMarkdown(value)), nil
	default:
		return nil, errors.New("report format is invalid")
	}
}

func renderMarkdown(value domain.FindingReport) string {
	var output strings.Builder
	fmt.Fprintf(&output, "# %s\n\n", markdownInline(value.Title))
	fmt.Fprintf(&output, "- Report: `%s`\n", value.ID)
	fmt.Fprintf(&output, "- Run: `%s`\n", value.RunID)
	fmt.Fprintf(&output, "- Source: `%s/%s`\n", value.SourceKind, value.SourceID)
	fmt.Fprintf(&output, "- Protocol: `%s`\n", value.ProtocolVersion)
	fmt.Fprintf(&output, "- Status: `%s`\n", value.Status)
	fmt.Fprintf(&output, "- Findings: %d\n", value.FindingCount)
	fmt.Fprintf(&output, "- Evidence records: %d\n", value.EvidenceCount)
	fmt.Fprintf(&output, "- Projection digest: `%s`\n\n", value.ProjectionDigest)
	output.WriteString("## Severity Summary\n\n")
	output.WriteString("| Severity | Count |\n| --- | ---: |\n")
	fmt.Fprintf(&output, "| Critical | %d |\n", value.Severity.Critical)
	fmt.Fprintf(&output, "| High | %d |\n", value.Severity.High)
	fmt.Fprintf(&output, "| Medium | %d |\n", value.Severity.Medium)
	fmt.Fprintf(&output, "| Low | %d |\n", value.Severity.Low)
	fmt.Fprintf(&output, "| Info | %d |\n\n", value.Severity.Info)
	output.WriteString("## Findings\n\n")
	if len(value.Findings) == 0 {
		output.WriteString("No draft findings were projected from this execution.\n")
		return output.String()
	}
	for _, finding := range value.Findings {
		fmt.Fprintf(&output, "### F-%03d: %s\n\n", finding.Ordinal,
			markdownInline(finding.Title))
		fmt.Fprintf(&output, "- Status: `%s`\n", finding.Status)
		fmt.Fprintf(&output, "- Severity: `%s`\n", finding.Severity)
		fmt.Fprintf(&output, "- Category: %s\n", markdownInline(finding.Category))
		fmt.Fprintf(&output, "- Location: %s\n", markdownCodeSpan(fmt.Sprintf("%s:%d-%d",
			finding.RelativePath, finding.LineStart, finding.LineEnd)))
		fmt.Fprintf(&output, "- Confidence: %d%%\n", finding.Confidence)
		fmt.Fprintf(&output, "- Fingerprint: `%s`\n\n", finding.Fingerprint)
		output.WriteString("Detail:\n\n")
		output.WriteString(markdownQuote(finding.Detail))
		output.WriteString("\nEvidence:\n\n")
		for _, evidence := range finding.Evidence {
			fmt.Fprintf(&output, "- E-%03d: `%s`, shard %d finding %d, confidence %d%%, digest `%s`\n",
				evidence.Ordinal, evidence.Kind, evidence.SourceShard,
				evidence.SourceOrdinal, evidence.Confidence, evidence.SourceDigest)
		}
		output.WriteString("\n")
	}
	return output.String()
}

func markdownInline(value string) string {
	value = html.EscapeString(strings.Join(strings.Fields(value), " "))
	replacer := strings.NewReplacer(
		"\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_",
		"[", "\\[", "]", "\\]", "#", "\\#", "|", "\\|",
		"<", "&lt;", ">", "&gt;",
	)
	return replacer.Replace(value)
}

func markdownCodeSpan(value string) string {
	longest := 0
	current := 0
	for _, character := range value {
		if character == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	delimiter := strings.Repeat("`", longest+1)
	return delimiter + " " + value + " " + delimiter
}

func markdownQuote(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = "> " + markdownInline(lines[index])
	}
	return strings.Join(lines, "\n") + "\n"
}
