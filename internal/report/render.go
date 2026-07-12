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
	FormatSARIF    Format = "sarif"
)

func ParseFormat(value string) (Format, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "md" {
		value = string(FormatMarkdown)
	}
	format := Format(value)
	if format != FormatMarkdown && format != FormatJSON && format != FormatSARIF {
		return "", errors.New("report format must be markdown, json, or sarif")
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
	case FormatSARIF:
		return renderSARIF(value)
	default:
		return nil, errors.New("report format is invalid")
	}
}

func renderMarkdown(value domain.FindingReport) string {
	var output strings.Builder
	validationCounts := map[domain.FindingStatus]int{
		domain.FindingStatusDraft:     0,
		domain.FindingStatusValidated: 0,
		domain.FindingStatusAccepted:  0,
		domain.FindingStatusFixed:     0,
		domain.FindingStatusRejected:  0,
	}
	artifactEvidenceCount := 0
	remediationEvidenceCount := 0
	for _, finding := range value.Findings {
		validationCounts[finding.EffectiveStatus()]++
		artifactEvidenceCount += len(finding.ArtifactEvidence)
		remediationEvidenceCount += len(finding.RemediationEvidence)
	}
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
	output.WriteString("## Finding Lifecycle Summary\n\n")
	fmt.Fprintf(&output, "- Draft: %d\n", validationCounts[domain.FindingStatusDraft])
	fmt.Fprintf(&output, "- Validated: %d\n", validationCounts[domain.FindingStatusValidated])
	fmt.Fprintf(&output, "- Accepted: %d\n", validationCounts[domain.FindingStatusAccepted])
	fmt.Fprintf(&output, "- Fixed: %d\n", validationCounts[domain.FindingStatusFixed])
	fmt.Fprintf(&output, "- Rejected: %d\n", validationCounts[domain.FindingStatusRejected])
	fmt.Fprintf(&output, "- Validation Artifact Evidence records: %d\n",
		artifactEvidenceCount)
	fmt.Fprintf(&output, "- Remediation Artifact Evidence records: %d\n\n",
		remediationEvidenceCount)
	output.WriteString("## Findings\n\n")
	if len(value.Findings) == 0 {
		output.WriteString("No draft findings were projected from this execution.\n")
		return output.String()
	}
	for _, finding := range value.Findings {
		fmt.Fprintf(&output, "### F-%03d: %s\n\n", finding.Ordinal,
			markdownInline(finding.Title))
		fmt.Fprintf(&output, "- Status: `%s`\n", finding.EffectiveStatus())
		fmt.Fprintf(&output, "- Severity: `%s`\n", finding.Severity)
		fmt.Fprintf(&output, "- Category: %s\n", markdownInline(finding.Category))
		fmt.Fprintf(&output, "- Location: %s\n", markdownCodeSpan(fmt.Sprintf("%s:%d-%d",
			finding.RelativePath, finding.LineStart, finding.LineEnd)))
		fmt.Fprintf(&output, "- Confidence: %d%%\n", finding.Confidence)
		fmt.Fprintf(&output, "- Fingerprint: `%s`\n\n", finding.Fingerprint)
		output.WriteString("Detail:\n\n")
		output.WriteString(markdownQuote(finding.Detail))
		output.WriteString("\nModel assertion evidence:\n\n")
		for _, evidence := range finding.Evidence {
			fmt.Fprintf(&output, "- E-%03d: `%s`, shard %d finding %d, confidence %d%%, digest `%s`\n",
				evidence.Ordinal, evidence.Kind, evidence.SourceShard,
				evidence.SourceOrdinal, evidence.Confidence, evidence.SourceDigest)
		}
		output.WriteString("\nValidation Artifact Evidence:\n\n")
		if len(finding.ArtifactEvidence) == 0 {
			output.WriteString("- None attached.\n")
		}
		for _, evidence := range finding.ArtifactEvidence {
			fmt.Fprintf(&output,
				"- AE-%03d: Artifact `%s`, `%s`, %d bytes, `%s`, tool `%s`, redacted `%t`, SHA-256 `%s`\n",
				evidence.Ordinal, evidence.ArtifactID, evidence.ArtifactStream,
				evidence.ArtifactSize, evidence.ArtifactMIME, evidence.ArtifactTool,
				evidence.ArtifactRedacted, evidence.ArtifactSHA256)
			fmt.Fprintf(&output, "  Attached by %s: %s\n",
				markdownInline(evidence.AttachedBy), markdownInline(evidence.Note))
		}
		if finding.Validation != nil {
			output.WriteString("\nValidation decision:\n\n")
			fmt.Fprintf(&output, "- Decision: `%s` from `%s`\n",
				finding.Validation.Status, finding.Validation.FromStatus)
			fmt.Fprintf(&output, "- Decided by: %s\n",
				markdownInline(finding.Validation.DecidedBy))
			fmt.Fprintf(&output, "- Artifact Evidence snapshot: %d records, digest `%s`\n",
				finding.Validation.ArtifactEvidenceCount,
				finding.Validation.ArtifactEvidenceDigest)
			output.WriteString("- Reason:\n\n")
			output.WriteString(markdownQuote(finding.Validation.Reason))
		}
		if finding.Acceptance != nil {
			output.WriteString("\nAcceptance decision:\n\n")
			fmt.Fprintf(&output, "- Decision: `%s` from `%s`\n",
				finding.Acceptance.Status, finding.Acceptance.FromStatus)
			fmt.Fprintf(&output, "- Decided by: %s\n",
				markdownInline(finding.Acceptance.DecidedBy))
			fmt.Fprintf(&output, "- Validation snapshot: `%s`, %d records, digest `%s`\n",
				finding.Acceptance.ValidationID,
				finding.Acceptance.ValidationArtifactEvidenceCount,
				finding.Acceptance.ValidationArtifactEvidenceDigest)
			output.WriteString("- Reason:\n\n")
			output.WriteString(markdownQuote(finding.Acceptance.Reason))
		}
		output.WriteString("\nRemediation Artifact Evidence:\n\n")
		if len(finding.RemediationEvidence) == 0 {
			output.WriteString("- None attached.\n")
		}
		for _, evidence := range finding.RemediationEvidence {
			fmt.Fprintf(&output,
				"- RE-%03d: Artifact `%s`, `%s`, %d bytes, `%s`, tool `%s`, redacted `%t`, SHA-256 `%s`\n",
				evidence.Ordinal, evidence.ArtifactID, evidence.ArtifactStream,
				evidence.ArtifactSize, evidence.ArtifactMIME, evidence.ArtifactTool,
				evidence.ArtifactRedacted, evidence.ArtifactSHA256)
			fmt.Fprintf(&output, "  Attached by %s: %s\n",
				markdownInline(evidence.AttachedBy), markdownInline(evidence.Note))
		}
		if finding.Fix != nil {
			output.WriteString("\nFix decision:\n\n")
			fmt.Fprintf(&output, "- Decision: `%s` from `%s`\n",
				finding.Fix.Status, finding.Fix.FromStatus)
			fmt.Fprintf(&output, "- Decided by: %s\n",
				markdownInline(finding.Fix.DecidedBy))
			fmt.Fprintf(&output, "- Remediation Evidence snapshot: %d records, digest `%s`\n",
				finding.Fix.RemediationEvidenceCount,
				finding.Fix.RemediationEvidenceDigest)
			output.WriteString("- Reason:\n\n")
			output.WriteString(markdownQuote(finding.Fix.Reason))
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
