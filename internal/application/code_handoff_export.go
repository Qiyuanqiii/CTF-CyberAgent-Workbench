package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"cyberagent-workbench/internal/apperror"
)

const (
	CodeHandoffExportProtocolVersion = "code_handoff_export.v1"
	CodeHandoffExportFormatJSON      = "json"
	CodeHandoffExportFormatMarkdown  = "markdown"
	MaxCodeHandoffExportBytes        = 256 * 1024
)

type CodeHandoffExport struct {
	ProtocolVersion     string
	Format              string
	Filename            string
	MIMEType            string
	RunID               string
	SourceEventSequence int64
	GeneratedAt         time.Time
	ContentSHA256       string
	ContentBytes        int
	Content             string
	ReadOnly            bool
	DownloadOnly        bool
	PrivateBodies       bool
	ResumeAuthorized    bool
	MutationSupported   bool
	ReportAcceptance    bool
	ExecutionStarted    bool
}

type CodeHandoffExportService struct {
	handoff *CodeHandoffService
}

func NewCodeHandoffExportService(store CodeHandoffStore) *CodeHandoffExportService {
	return &CodeHandoffExportService{handoff: NewCodeHandoffService(store)}
}

func (s *CodeHandoffExportService) Build(ctx context.Context, runID string,
	format string,
) (CodeHandoffExport, error) {
	if s == nil || s.handoff == nil {
		return CodeHandoffExport{}, apperror.New(apperror.CodeFailedPrecondition,
			"Code handoff export service is required")
	}
	if format != strings.TrimSpace(format) ||
		(format != CodeHandoffExportFormatJSON && format != CodeHandoffExportFormatMarkdown) {
		return CodeHandoffExport{}, apperror.New(apperror.CodeInvalidArgument,
			"Code handoff export format must be json or markdown")
	}
	handoff, err := s.handoff.Build(ctx, runID)
	if err != nil {
		return CodeHandoffExport{}, err
	}
	if handoff.SourceEventSequence <= 0 {
		return CodeHandoffExport{}, apperror.New(apperror.CodeConflict,
			"Code handoff export requires a durable source event high-water mark")
	}
	var content []byte
	mimeType := "application/json"
	extension := "json"
	if format == CodeHandoffExportFormatJSON {
		content, err = json.MarshalIndent(handoff, "", "  ")
		if err == nil {
			content = append(content, '\n')
		}
	} else {
		content = []byte(renderCodeHandoffMarkdown(handoff))
		mimeType = "text/markdown; charset=utf-8"
		extension = "md"
	}
	if err != nil {
		return CodeHandoffExport{}, apperror.Wrap(apperror.CodeInternal,
			"Code handoff export could not be encoded", err)
	}
	if len(content) == 0 || len(content) > MaxCodeHandoffExportBytes {
		return CodeHandoffExport{}, apperror.New(apperror.CodeResourceExhausted,
			"Code handoff export exceeds the bounded content limit")
	}
	digest := sha256.Sum256(content)
	return CodeHandoffExport{
		ProtocolVersion: CodeHandoffExportProtocolVersion, Format: format,
		Filename: "cyberagent-code-handoff-" + safeHandoffFilenameID(handoff.RunID) + "." + extension,
		MIMEType: mimeType, RunID: handoff.RunID,
		SourceEventSequence: handoff.SourceEventSequence, GeneratedAt: handoff.GeneratedAt,
		ContentSHA256: hex.EncodeToString(digest[:]), ContentBytes: len(content),
		Content: string(content), ReadOnly: true, DownloadOnly: true,
	}, nil
}

func renderCodeHandoffMarkdown(value CodeHandoff) string {
	var output strings.Builder
	output.WriteString("# CyberAgent Code Handoff\n\n")
	fmt.Fprintf(&output, "- Run: `%s`\n", markdownCell(value.RunID))
	fmt.Fprintf(&output, "- Mission: `%s`\n", markdownCell(value.MissionID))
	fmt.Fprintf(&output, "- Session: `%s`\n", markdownCell(value.SessionID))
	fmt.Fprintf(&output, "- Workspace: `%s`\n", markdownCell(value.WorkspaceID))
	fmt.Fprintf(&output, "- Source event high-water: `%d`\n", value.SourceEventSequence)
	fmt.Fprintf(&output, "- Generated: `%s`\n\n", value.GeneratedAt.UTC().Format(time.RFC3339Nano))

	output.WriteString("## Run State\n\n")
	output.WriteString("| Status | Surface | Phase | Mode revision |\n")
	output.WriteString("| --- | --- | --- | ---: |\n")
	fmt.Fprintf(&output, "| %s | %s | %s | %d |\n\n", markdownCell(string(value.RunStatus)),
		markdownCell(string(value.Surface)), markdownCell(string(value.Phase)), value.ModeRevision)

	output.WriteString("## Delivery\n\n")
	output.WriteString("| Plan | Modules | Pending | In progress | Blocked | Completed | Cancelled |\n")
	output.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	fmt.Fprintf(&output, "| %s | %d | %d | %d | %d | %d | %d |\n\n",
		markdownCell(value.Plan.State), value.Plan.ModuleCount, value.Plan.PendingCount,
		value.Plan.InProgressCount, value.Plan.BlockedCount, value.Plan.CompletedCount,
		value.Plan.CancelledCount)
	fmt.Fprintf(&output, "Queue: %d pending, %d prepared, %d committed, %d cancelled.\n\n",
		value.Queue.Pending, value.Queue.Prepared, value.Queue.Committed, value.Queue.Cancelled)
	fmt.Fprintf(&output, "Change set: %d proposed, %d approved, %d applied, %d denied, %d failed; %d bytes%s.\n\n",
		value.ChangeSet.Proposed, value.ChangeSet.Approved, value.ChangeSet.Applied,
		value.ChangeSet.Denied, value.ChangeSet.Failed, value.ChangeSet.TotalDiffBytes,
		markdownTruncation(value.ChangeSet.Truncated))

	output.WriteString("## Verification\n\n")
	fmt.Fprintf(&output, "Evidence: %d pass, %d fail, %d unknown%s.\n\n",
		value.Verification.PassCount, value.Verification.FailCount,
		value.Verification.UnknownCount, markdownTruncation(value.Verification.Truncated))
	if len(value.Verification.References) > 0 {
		output.WriteString("| Evidence | Outcome | Recorded |\n| --- | --- | --- |\n")
		for _, reference := range value.Verification.References {
			fmt.Fprintf(&output, "| `%s` | %s | %s |\n", markdownCell(reference.ID),
				markdownCell(string(reference.Outcome)), reference.CreatedAt.UTC().Format(time.RFC3339Nano))
		}
		output.WriteByte('\n')
	}
	fmt.Fprintf(&output, "Operator plans: %d%s.\n\n", value.VerificationPlans.ReturnedCount,
		markdownTruncation(value.VerificationPlans.Truncated))
	if len(value.VerificationPlans.References) > 0 {
		output.WriteString("| Plan | Items | SHA-256 | Created |\n| --- | ---: | --- | --- |\n")
		for _, reference := range value.VerificationPlans.References {
			fmt.Fprintf(&output, "| `%s` | %d | `%s` | %s |\n",
				markdownCell(reference.ID), reference.ItemCount, markdownCell(reference.PlanSHA256),
				reference.CreatedAt.UTC().Format(time.RFC3339Nano))
		}
		output.WriteByte('\n')
	}

	output.WriteString("## Pending Actions\n\n")
	if len(value.PendingActions) == 0 {
		output.WriteString("None.\n\n")
	} else {
		output.WriteString("| Action | Kind | State | Destination | Available |\n")
		output.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, action := range value.PendingActions {
			fmt.Fprintf(&output, "| `%s` | %s | %s | %s | %s |\n",
				markdownCell(action.ID), markdownCell(string(action.Kind)), markdownCell(action.State),
				markdownCell(string(action.Destination)), action.AvailableAt.UTC().Format(time.RFC3339Nano))
		}
		output.WriteByte('\n')
	}

	output.WriteString("## Reports\n\n")
	if len(value.ReportReferences) == 0 {
		output.WriteString("None.\n\n")
	} else {
		output.WriteString("| Report | Status | Findings | Created |\n")
		output.WriteString("| --- | --- | ---: | --- |\n")
		for _, report := range value.ReportReferences {
			fmt.Fprintf(&output, "| `%s` | %s | %d | %s |\n", markdownCell(report.ID),
				markdownCell(string(report.Status)), report.FindingCount,
				report.CreatedAt.UTC().Format(time.RFC3339Nano))
		}
		output.WriteByte('\n')
	}
	output.WriteString("## Safety Boundary\n\n")
	output.WriteString("This export is a read-only snapshot. It contains no private bodies, grants no resume or mutation authority, accepts no report, and starts no execution.\n")
	return output.String()
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.ReplaceAll(value, "\n", " ")
}

func markdownTruncation(truncated bool) string {
	if truncated {
		return " (truncated)"
	}
	return ""
}

func safeHandoffFilenameID(value string) string {
	var output strings.Builder
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || current == '-' || current == '_' {
			output.WriteRune(current)
		} else {
			output.WriteByte('-')
		}
		if output.Len() >= 80 {
			break
		}
	}
	if output.Len() == 0 {
		return "run"
	}
	return output.String()
}
