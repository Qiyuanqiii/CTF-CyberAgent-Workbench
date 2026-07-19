package contextmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/redact"
)

const (
	HandoffMemoryProtocolVersion = "handoff_memory.v1"
	LegacyHandoffProtocolVersion = "handoff_memory.v0"
	MaxHandoffMemoryRecords      = 12
	MaxHandoffMemoryChars        = 4000
	MaxHandoffRecordChars        = 1000
	MaxHandoffSourceRefChars     = 256
)

type handoffMemoryEnvelope struct {
	Version                string                `json:"version"`
	TaskID                 string                `json:"task_id"`
	WorkspaceID            string                `json:"workspace_id,omitempty"`
	PreviousSummaryID      int64                 `json:"previous_summary_id,omitempty"`
	CompactedMessageCount  int                   `json:"compacted_message_count"`
	PreservedMessageCount  int                   `json:"preserved_message_count"`
	SourceThroughMessageID int64                 `json:"source_through_message_id,omitempty"`
	LastOrdinal            int                   `json:"last_ordinal"`
	RecordsOmitted         int                   `json:"records_omitted"`
	Records                []handoffMemoryRecord `json:"records"`
}

type handoffMemoryRecord struct {
	Ordinal               int    `json:"ordinal"`
	Category              string `json:"category"`
	Role                  string `json:"role"`
	SourceKind            string `json:"source_kind"`
	SourceRef             string `json:"source_ref,omitempty"`
	SourceMessageID       int64  `json:"source_message_id,omitempty"`
	SourceContentSHA256   string `json:"source_content_sha256,omitempty"`
	ContentSHA256         string `json:"content_sha256"`
	InstructionAuthorized bool   `json:"instruction_authorized"`
	Content               string `json:"content"`
}

func buildHandoffMemory(taskID string, workspaceID string, previous Summary,
	hasPrevious bool, older []Message, preservedCount int, config Config,
) (string, int, int, error) {
	records, previousOmitted, compactedCount, lastOrdinal, sourceThrough :=
		previousHandoffRecords(taskID, workspaceID, previous, hasPrevious, config.MaxLineChars)
	newOlder, nextSourceThrough, err := newHandoffMessages(older, sourceThrough)
	if err != nil {
		return "", 0, 0, err
	}
	for _, message := range newOlder {
		role := normalizeRole(message.Role)
		content := trimRunes(collapseWhitespace(redact.String(message.Content)), config.MaxLineChars)
		if content == "" {
			continue
		}
		lastOrdinal = saturatingTokenAdd(lastOrdinal, 1)
		sourceKind, instructionAuthorized := summaryMessageAuthority(message, role)
		sourceKind = normalizeHandoffSourceKind(sourceKind)
		instructionAuthorized = validHandoffAuthority(role, sourceKind, instructionAuthorized)
		sourceContentDigest := strings.TrimSpace(message.ContentSHA256)
		if !validHandoffDigest(sourceContentDigest) {
			sourceContentDigest = ""
		}
		records = append(records, handoffMemoryRecord{
			Ordinal:  lastOrdinal,
			Category: handoffRecordCategory(role, sourceKind, instructionAuthorized),
			Role:     role, SourceKind: sourceKind,
			SourceRef:           sanitizeHandoffSourceRef(message.SourceRef),
			SourceMessageID:     message.SourceMessageID,
			SourceContentSHA256: sourceContentDigest, ContentSHA256: handoffContentSHA256(content),
			InstructionAuthorized: instructionAuthorized, Content: content,
		})
	}
	compactedCount = saturatingTokenAdd(compactedCount, len(newOlder))
	envelope := handoffMemoryEnvelope{
		Version: HandoffMemoryProtocolVersion, TaskID: taskID, WorkspaceID: workspaceID,
		CompactedMessageCount: compactedCount, PreservedMessageCount: preservedCount,
		SourceThroughMessageID: nextSourceThrough,
		LastOrdinal:            lastOrdinal, RecordsOmitted: previousOmitted,
		Records: []handoffMemoryRecord{},
	}
	if hasPrevious {
		envelope.PreviousSummaryID = previous.ID
	}
	content, err := fitHandoffRecords(envelope, records, config.MaxSummaryChars)
	if err != nil {
		return "", 0, 0, err
	}
	return content, compactedCount, len(newOlder), nil
}

func previousHandoffRecords(taskID string, workspaceID string, previous Summary,
	hasPrevious bool, maxLineChars int,
) ([]handoffMemoryRecord, int, int, int, int64) {
	if !hasPrevious {
		return []handoffMemoryRecord{}, 0, 0, 0, 0
	}
	if previous.ProtocolVersion == HandoffMemoryProtocolVersion &&
		previous.ContentSHA256 == handoffContentSHA256(previous.Content) {
		var envelope handoffMemoryEnvelope
		if json.Unmarshal([]byte(previous.Content), &envelope) == nil &&
			validateHandoffEnvelope(envelope, taskID, workspaceID) == nil {
			lastOrdinal := envelope.LastOrdinal
			for index := range envelope.Records {
				record := &envelope.Records[index]
				record.Content = trimRunes(collapseWhitespace(redact.String(record.Content)),
					maxLineChars)
				record.SourceKind = normalizeHandoffSourceKind(record.SourceKind)
				record.SourceRef = sanitizeHandoffSourceRef(record.SourceRef)
				record.InstructionAuthorized = validHandoffAuthority(record.Role,
					record.SourceKind, record.InstructionAuthorized)
				record.Category = handoffRecordCategory(record.Role, record.SourceKind,
					record.InstructionAuthorized)
				record.ContentSHA256 = handoffContentSHA256(record.Content)
			}
			return envelope.Records, envelope.RecordsOmitted,
				envelope.CompactedMessageCount, lastOrdinal, envelope.SourceThroughMessageID
		}
	}
	content := trimRunes(collapseWhitespace(redact.String(previous.Content)), maxLineChars)
	compacted := previous.SourceMessageCount - previous.PreservedMessageCount
	if compacted < 1 && content != "" {
		compacted = 1
	}
	if content == "" {
		return []handoffMemoryRecord{}, 0, compacted, 0, 0
	}
	return []handoffMemoryRecord{{
		Ordinal: 1, Category: "prior_handoff", Role: "tool",
		SourceKind: "compacted_transcript", ContentSHA256: handoffContentSHA256(content),
		InstructionAuthorized: false, Content: content,
	}}, 0, compacted, 1, 0
}

func newHandoffMessages(messages []Message, previousThrough int64) ([]Message, int64, error) {
	if previousThrough < 0 {
		return nil, 0, errors.New("handoff source message high-water is invalid")
	}
	out := make([]Message, 0, len(messages))
	lastPositiveID := int64(0)
	nextThrough := previousThrough
	for _, message := range messages {
		if message.SourceMessageID < 0 {
			return nil, 0, errors.New("handoff source message id is invalid")
		}
		if message.SourceMessageID > 0 {
			if message.SourceMessageID <= lastPositiveID {
				return nil, 0, errors.New("handoff source message ids are not increasing")
			}
			lastPositiveID = message.SourceMessageID
			if message.SourceMessageID <= previousThrough {
				continue
			}
			nextThrough = message.SourceMessageID
		}
		out = append(out, message)
	}
	return out, nextThrough, nil
}

func fitHandoffRecords(envelope handoffMemoryEnvelope, records []handoffMemoryRecord,
	maxRunes int,
) (string, error) {
	if maxRunes < 512 {
		return "", errors.New("handoff memory requires at least 512 summary characters")
	}
	baseOmitted := envelope.RecordsOmitted
	candidates := append([]handoffMemoryRecord(nil), records...)
	sort.SliceStable(candidates, func(left int, right int) bool {
		leftPriority := handoffRetentionPriority(candidates[left])
		rightPriority := handoffRetentionPriority(candidates[right])
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		return candidates[left].Ordinal > candidates[right].Ordinal
	})
	selected := make([]handoffMemoryRecord, 0, len(candidates))
	for _, candidate := range candidates {
		trial := append(append([]handoffMemoryRecord(nil), selected...), candidate)
		if len(trial) > MaxHandoffMemoryRecords {
			continue
		}
		sort.SliceStable(trial, func(left int, right int) bool {
			return trial[left].Ordinal < trial[right].Ordinal
		})
		trialEnvelope := envelope
		trialEnvelope.Records = trial
		trialEnvelope.RecordsOmitted = saturatingTokenAdd(baseOmitted,
			len(records)-len(trial))
		encoded, err := json.Marshal(trialEnvelope)
		if err != nil {
			return "", err
		}
		if utf8.RuneCount(encoded) <= maxRunes {
			selected = trial
		}
	}
	sort.SliceStable(selected, func(left int, right int) bool {
		return selected[left].Ordinal < selected[right].Ordinal
	})
	envelope.Records = selected
	envelope.RecordsOmitted = saturatingTokenAdd(baseOmitted, len(records)-len(selected))
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	if utf8.RuneCount(encoded) > maxRunes {
		return "", errors.New("handoff memory envelope exceeds its character limit")
	}
	return string(encoded), nil
}

func handoffRetentionPriority(record handoffMemoryRecord) int {
	switch {
	case record.InstructionAuthorized && record.Category == "operator_intent":
		return 4
	case record.Category == "assistant_progress":
		return 3
	case record.Category == "go_control":
		return 2
	default:
		return 1
	}
}

func handoffRecordCategory(role string, sourceKind string, authorized bool) string {
	switch {
	case sourceKind == "compacted_transcript":
		return "prior_handoff"
	case authorized && role == "user" && sourceKind == "operator_message":
		return "operator_intent"
	case authorized && role == "system" && sourceKind == "go_control":
		return "go_control"
	case role == "assistant":
		return "assistant_progress"
	default:
		return "external_evidence"
	}
}

func validHandoffAuthority(role string, sourceKind string, authorized bool) bool {
	if !authorized {
		return false
	}
	return (role == "user" && sourceKind == "operator_message") ||
		(role == "system" && sourceKind == "go_control")
}

func validateHandoffEnvelope(envelope handoffMemoryEnvelope, taskID string,
	workspaceID string,
) error {
	if envelope.Version != HandoffMemoryProtocolVersion || envelope.TaskID != taskID ||
		envelope.WorkspaceID != workspaceID || envelope.PreviousSummaryID < 0 ||
		envelope.SourceThroughMessageID < 0 ||
		envelope.CompactedMessageCount < 0 || envelope.PreservedMessageCount < 0 ||
		envelope.LastOrdinal < 0 || envelope.LastOrdinal > envelope.CompactedMessageCount ||
		envelope.RecordsOmitted < 0 || len(envelope.Records) > MaxHandoffMemoryRecords ||
		envelope.LastOrdinal != len(envelope.Records)+envelope.RecordsOmitted ||
		envelope.CompactedMessageCount < len(envelope.Records)+envelope.RecordsOmitted {
		return errors.New("handoff memory envelope metadata is invalid")
	}
	previousOrdinal := 0
	previousSourceMessageID := int64(0)
	for _, record := range envelope.Records {
		if record.Ordinal <= previousOrdinal || record.Ordinal > envelope.LastOrdinal ||
			!isKnownRole(record.Role) ||
			record.SourceKind != normalizeHandoffSourceKind(record.SourceKind) ||
			record.SourceRef != sanitizeHandoffSourceRef(record.SourceRef) ||
			record.SourceMessageID < 0 ||
			(record.SourceMessageID > 0 && (record.SourceMessageID <= previousSourceMessageID ||
				record.SourceMessageID > envelope.SourceThroughMessageID)) ||
			record.Content == "" || utf8.RuneCountInString(record.Content) > MaxHandoffRecordChars ||
			!utf8.ValidString(record.Content) ||
			record.ContentSHA256 != handoffContentSHA256(record.Content) ||
			(record.SourceContentSHA256 != "" && !validHandoffDigest(record.SourceContentSHA256)) ||
			record.InstructionAuthorized != validHandoffAuthority(record.Role,
				record.SourceKind, record.InstructionAuthorized) ||
			record.Category != handoffRecordCategory(record.Role, record.SourceKind,
				record.InstructionAuthorized) {
			return errors.New("handoff memory record is invalid")
		}
		previousOrdinal = record.Ordinal
		if record.SourceMessageID > 0 {
			previousSourceMessageID = record.SourceMessageID
		}
	}
	return nil
}

func PrepareSummaryForStorage(summary Summary) (Summary, error) {
	summary.TaskID = strings.TrimSpace(summary.TaskID)
	summary.WorkspaceID = strings.TrimSpace(summary.WorkspaceID)
	if summary.TaskID == "" {
		return Summary{}, errors.New("task id is required")
	}
	if summary.SourceMessageCount < 0 || summary.PreservedMessageCount < 0 ||
		summary.PreservedMessageCount > summary.SourceMessageCount {
		return Summary{}, errors.New("handoff summary source counters are invalid")
	}
	summary.Content = strings.TrimSpace(redact.String(summary.Content))
	if summary.Content == "" || !utf8.ValidString(summary.Content) {
		return Summary{}, errors.New("handoff summary content must be nonempty UTF-8")
	}
	if summary.ProtocolVersion == "" {
		compacted := summary.SourceMessageCount - summary.PreservedMessageCount
		if compacted < 1 {
			compacted = 1
		}
		recordContent := trimRunes(summary.Content, 1000)
		record := handoffMemoryRecord{
			Ordinal: 1, Category: "prior_handoff", Role: "tool",
			SourceKind: "compacted_transcript", ContentSHA256: handoffContentSHA256(recordContent),
			InstructionAuthorized: false, Content: recordContent,
		}
		envelope := handoffMemoryEnvelope{
			Version: HandoffMemoryProtocolVersion, TaskID: summary.TaskID,
			WorkspaceID: summary.WorkspaceID, PreviousSummaryID: summary.PreviousSummaryID,
			CompactedMessageCount: compacted,
			PreservedMessageCount: summary.PreservedMessageCount,
			LastOrdinal:           1,
			Records:               []handoffMemoryRecord{record},
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return Summary{}, err
		}
		summary.Content = string(encoded)
		summary.CompactedMessageCount = compacted
		summary.SourceMessageCount = saturatingTokenAdd(compacted,
			summary.PreservedMessageCount)
		summary.ProtocolVersion = HandoffMemoryProtocolVersion
	}
	if summary.ProtocolVersion != HandoffMemoryProtocolVersion || summary.PreviousSummaryID < 0 ||
		summary.CompactedMessageCount < 1 || summary.PreservedMessageCount < 0 ||
		utf8.RuneCountInString(summary.Content) > MaxHandoffMemoryChars ||
		summary.SourceMessageCount != saturatingTokenAdd(summary.CompactedMessageCount,
			summary.PreservedMessageCount) {
		return Summary{}, errors.New("handoff summary counters are invalid")
	}
	var envelope handoffMemoryEnvelope
	if err := json.Unmarshal([]byte(summary.Content), &envelope); err != nil {
		return Summary{}, fmt.Errorf("decode handoff summary: %w", err)
	}
	if err := validateHandoffEnvelope(envelope, summary.TaskID, summary.WorkspaceID); err != nil {
		return Summary{}, err
	}
	if envelope.PreviousSummaryID != summary.PreviousSummaryID ||
		envelope.CompactedMessageCount != summary.CompactedMessageCount ||
		envelope.PreservedMessageCount != summary.PreservedMessageCount {
		return Summary{}, errors.New("handoff summary envelope binding is invalid")
	}
	expectedDigest := handoffContentSHA256(summary.Content)
	if summary.ContentSHA256 != "" && summary.ContentSHA256 != expectedDigest {
		return Summary{}, errors.New("handoff summary content digest does not match")
	}
	summary.ContentSHA256 = expectedDigest
	summary.TokenEstimate = EstimateTokens(summary.Content)
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now().UTC()
	}
	return summary, nil
}

func ValidateStoredSummary(summary Summary) error {
	if summary.ProtocolVersion == LegacyHandoffProtocolVersion {
		if strings.TrimSpace(summary.TaskID) == "" || summary.Content == "" ||
			!utf8.ValidString(summary.Content) || summary.SourceMessageCount < 0 ||
			summary.PreservedMessageCount < 0 ||
			summary.PreservedMessageCount > summary.SourceMessageCount {
			return errors.New("legacy context summary is invalid")
		}
		return nil
	}
	_, err := PrepareSummaryForStorage(summary)
	return err
}

func handoffContentSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func validHandoffDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizeHandoffSourceKind(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return "legacy_unclassified"
	}
	for _, current := range value {
		if (current < 'a' || current > 'z') && (current < '0' || current > '9') &&
			current != '_' {
			return "legacy_unclassified"
		}
	}
	return value
}

func sanitizeHandoffSourceRef(value string) string {
	return trimRunes(collapseWhitespace(redact.String(value)), MaxHandoffSourceRefChars)
}
