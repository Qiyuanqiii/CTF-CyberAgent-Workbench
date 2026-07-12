package application

import (
	"encoding/json"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/redact"
)

const (
	maxSpecialistContextTokens      = 4096
	maxSpecialistContextSourceBytes = 28 * 1024
	maxSpecialistGoalRunes          = 4096
	maxSpecialistWorkItems          = 20
	maxSpecialistNotes              = 50
)

type specialistContextEnvelope struct {
	Version            string                         `json:"version"`
	Mission            specialistMissionContext       `json:"mission"`
	ParentInstructions []specialistInstructionContext `json:"parent_instructions"`
	WorkItems          []specialistWorkItemContext    `json:"work_items"`
	Notes              []specialistNoteContext        `json:"notes"`
}

type specialistMissionContext struct {
	Goal            string         `json:"goal"`
	Profile         domain.Profile `json:"profile"`
	Skills          []string       `json:"skills"`
	Turn            int64          `json:"turn"`
	RemainingTurns  int64          `json:"remaining_turns"`
	RemainingTokens int64          `json:"remaining_tokens"`
	NetworkMode     string         `json:"network_mode"`
}

type specialistInstructionContext struct {
	Instruction string `json:"instruction"`
}

type specialistWorkItemContext struct {
	ID                 string                  `json:"id"`
	Status             domain.WorkItemStatus   `json:"status"`
	Priority           domain.WorkItemPriority `json:"priority"`
	Title              string                  `json:"title"`
	Description        string                  `json:"description,omitempty"`
	AcceptanceCriteria []string                `json:"acceptance_criteria,omitempty"`
	Dependencies       []string                `json:"dependencies,omitempty"`
	BlockedReason      string                  `json:"blocked_reason,omitempty"`
	Version            int64                   `json:"item_version"`
}

type specialistNoteContext struct {
	ID          string                `json:"id"`
	Title       string                `json:"title"`
	Content     string                `json:"content"`
	Category    domain.NoteCategory   `json:"category"`
	Visibility  domain.NoteVisibility `json:"visibility"`
	Tags        []string              `json:"tags,omitempty"`
	SourceRefs  []string              `json:"source_refs,omitempty"`
	EvidenceIDs []string              `json:"evidence_ids,omitempty"`
	Pinned      bool                  `json:"pinned"`
	Version     int64                 `json:"note_version"`
}

func specialistTurnInput(mission domain.Mission, child domain.AgentNode,
	attempt domain.AgentAttempt, messages []domain.AgentMessage, workItems []domain.WorkItem,
	notes []domain.Note,
) (string, contextmgr.Selection, error) {
	if mission.ID == "" || child.ID == "" || child.RunID != attempt.RunID ||
		child.ID != attempt.AgentID || child.ParentID != attempt.ParentAgentID {
		return "", contextmgr.Selection{}, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist context identities do not match the active attempt")
	}
	if len(messages) > domain.MaxSpecialistContextMessages || len(workItems) > maxSpecialistWorkItems ||
		len(notes) > maxSpecialistNotes {
		return "", contextmgr.Selection{}, apperror.New(apperror.CodeResourceExhausted,
			"Specialist context source count exceeds its bound")
	}
	missionRecord := specialistMissionContext{
		Goal:    truncateWorkBoardText(redact.String(mission.Goal), maxSpecialistGoalRunes),
		Profile: child.Profile, Skills: append([]string(nil), child.Skills...), Turn: attempt.Turn,
		RemainingTurns:  max(int64(0), child.TurnLimit-attempt.Turn),
		RemainingTokens: max(int64(0), child.TokenLimit-child.TokensUsed),
		NetworkMode:     strings.TrimSpace(mission.Scope.NetworkMode),
	}
	sections := make([]contextmgr.Section, 0, 1+len(messages)+len(workItems)+len(notes))
	missionContent, err := marshalSpecialistContextRecord(missionRecord)
	if err != nil {
		return "", contextmgr.Selection{}, err
	}
	sections = append(sections, contextmgr.Section{
		Kind: "specialist_mission", SourceID: mission.ID, Content: missionContent, Priority: 1000,
	})
	mandatory := map[string]struct{}{specialistContextSourceKey("specialist_mission", mission.ID): {}}
	instructions := make(map[string]specialistInstructionContext, len(messages))
	for _, message := range messages {
		if message.RunID != child.RunID || message.RecipientAgentID != child.ID ||
			message.SenderAgentID != child.ParentID || message.Status != domain.AgentMessagePending ||
			!domain.EligibleSpecialistContextMessage(message) {
			return "", contextmgr.Selection{}, apperror.New(apperror.CodeFailedPrecondition,
				"Specialist context contains an ineligible parent instruction")
		}
		payload, err := domain.DecodeAgentInstructionPayload(message.PayloadJSON)
		if err != nil {
			return "", contextmgr.Selection{}, err
		}
		record := specialistInstructionContext{
			Instruction: truncateWorkBoardText(redact.String(payload.Instruction),
				domain.MaxSpecialistInstructionRunes),
		}
		content, err := marshalSpecialistContextRecord(record)
		if err != nil {
			return "", contextmgr.Selection{}, err
		}
		sections = append(sections, contextmgr.Section{
			Kind: "parent_instruction", SourceID: message.ID, Content: content, Priority: 990,
		})
		key := specialistContextSourceKey("parent_instruction", message.ID)
		mandatory[key] = struct{}{}
		instructions[key] = record
	}
	workRecords := make(map[string]specialistWorkItemContext, len(workItems))
	for _, item := range workItems {
		if item.RunID != child.RunID || item.OwnerAgentID != child.ID || item.Terminal() {
			return "", contextmgr.Selection{}, apperror.New(apperror.CodeFailedPrecondition,
				"Specialist context contains work not owned by the child")
		}
		record := specialistWorkItemContext{
			ID: item.ID, Status: item.Status, Priority: item.Priority,
			Title:              truncateWorkBoardText(redact.String(item.Title), 240),
			Description:        truncateWorkBoardText(redact.String(item.Description), 800),
			AcceptanceCriteria: boundedWorkBoardStrings(item.AcceptanceCriteria, 6, 320),
			Dependencies:       boundedWorkBoardStrings(item.Dependencies, 12, 128),
			BlockedReason:      truncateWorkBoardText(redact.String(item.BlockedReason), 480),
			Version:            item.Version,
		}
		content, err := marshalSpecialistContextRecord(record)
		if err != nil {
			return "", contextmgr.Selection{}, err
		}
		sections = append(sections, contextmgr.Section{
			Kind: "child_work_item", SourceID: item.ID, Content: content,
			Priority: specialistWorkItemPriority(item),
		})
		workRecords[specialistContextSourceKey("child_work_item", item.ID)] = record
	}
	noteRecords := make(map[string]specialistNoteContext, len(notes))
	for _, note := range notes {
		if note.RunID != child.RunID || note.OwnerAgentID != child.ID ||
			note.Status != domain.NoteActive ||
			(note.Visibility != domain.NoteVisibilityRun &&
				note.Visibility != domain.NoteVisibilityOwner) {
			return "", contextmgr.Selection{}, apperror.New(apperror.CodeFailedPrecondition,
				"Specialist context contains a Note not owned by or visible to the child")
		}
		record := specialistNoteContext{
			ID: note.ID, Title: truncateWorkBoardText(redact.String(note.Title), 240),
			Content:  truncateWorkBoardText(redact.String(note.Content), 1600),
			Category: note.Category, Visibility: note.Visibility,
			Tags:        boundedWorkBoardStrings(note.Tags, 12, 64),
			SourceRefs:  boundedWorkBoardStrings(note.SourceRefs, 8, 256),
			EvidenceIDs: boundedWorkBoardStrings(note.EvidenceIDs, 12, 128),
			Pinned:      note.Pinned, Version: note.Version,
		}
		content, err := marshalSpecialistContextRecord(record)
		if err != nil {
			return "", contextmgr.Selection{}, err
		}
		sections = append(sections, contextmgr.Section{
			Kind: "child_note", SourceID: note.ID, Content: content,
			Priority: supervisorNotePriority(note),
		})
		noteRecords[specialistContextSourceKey("child_note", note.ID)] = record
	}
	selection, err := contextmgr.SelectSections(sections, maxSpecialistContextTokens)
	if err != nil {
		return "", contextmgr.Selection{}, err
	}
	for key := range mandatory {
		kind, sourceID, _ := strings.Cut(key, "\x00")
		if !containsContextSource(selection.IncludedSources, kind, sourceID) {
			return "", contextmgr.Selection{}, apperror.New(apperror.CodeResourceExhausted,
				"bounded Specialist context did not fit its mandatory parent instructions")
		}
	}
	selection, err = boundSpecialistContextSourceBytes(selection, mandatory,
		maxSpecialistContextSourceBytes)
	if err != nil {
		return "", contextmgr.Selection{}, err
	}
	included := make(map[string]struct{}, len(selection.IncludedSources))
	for _, source := range selection.IncludedSources {
		included[specialistContextSourceKey(source.Kind, source.SourceID)] = struct{}{}
	}
	envelope := specialistContextEnvelope{
		Version: domain.SpecialistContextVersion, Mission: missionRecord,
		ParentInstructions: []specialistInstructionContext{},
		WorkItems:          []specialistWorkItemContext{}, Notes: []specialistNoteContext{},
	}
	for _, message := range messages {
		key := specialistContextSourceKey("parent_instruction", message.ID)
		if _, ok := included[key]; ok {
			envelope.ParentInstructions = append(envelope.ParentInstructions, instructions[key])
		}
	}
	for _, item := range workItems {
		key := specialistContextSourceKey("child_work_item", item.ID)
		if _, ok := included[key]; ok {
			envelope.WorkItems = append(envelope.WorkItems, workRecords[key])
		}
	}
	for _, note := range notes {
		key := specialistContextSourceKey("child_note", note.ID)
		if _, ok := included[key]; ok {
			envelope.Notes = append(envelope.Notes, noteRecords[key])
		}
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", contextmgr.Selection{}, err
	}
	if len(encoded) == 0 || len(encoded) > maxSpecialistInputBytes {
		return "", contextmgr.Selection{}, apperror.New(apperror.CodeResourceExhausted,
			"Specialist turn input exceeds its bounded context")
	}
	return string(encoded), selection, nil
}

func marshalSpecialistContextRecord(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(encoded) == 0 {
		return "", fmt.Errorf("specialist context record is empty")
	}
	return string(encoded), nil
}

func specialistContextSourceKey(kind string, sourceID string) string {
	return kind + "\x00" + sourceID
}

func boundSpecialistContextSourceBytes(selection contextmgr.Selection,
	mandatory map[string]struct{}, maxBytes int,
) (contextmgr.Selection, error) {
	bounded := contextmgr.Selection{
		Sections:        make([]contextmgr.Section, 0, len(selection.Sections)),
		IncludedSources: make([]contextmgr.Source, 0, len(selection.IncludedSources)),
		OmittedSources:  append([]contextmgr.Source(nil), selection.OmittedSources...),
		TokenBudget:     selection.TokenBudget,
	}
	usedBytes := 0
	for index, section := range selection.Sections {
		source := selection.IncludedSources[index]
		key := specialistContextSourceKey(source.Kind, source.SourceID)
		sectionBytes := len([]byte(section.Content))
		if sectionBytes > maxBytes-usedBytes {
			if _, required := mandatory[key]; required {
				return contextmgr.Selection{}, apperror.New(apperror.CodeResourceExhausted,
					"mandatory Specialist context exceeds its byte bound")
			}
			bounded.OmittedSources = append(bounded.OmittedSources, source)
			continue
		}
		bounded.Sections = append(bounded.Sections, section)
		bounded.IncludedSources = append(bounded.IncludedSources, source)
		bounded.EstimatedTokens += source.Tokens
		usedBytes += sectionBytes
	}
	return bounded, nil
}

func specialistWorkItemPriority(item domain.WorkItem) int {
	priority := 800
	switch item.Priority {
	case domain.WorkItemPriorityCritical:
		priority += 120
	case domain.WorkItemPriorityHigh:
		priority += 80
	case domain.WorkItemPriorityNormal:
		priority += 40
	}
	switch item.Status {
	case domain.WorkItemInProgress:
		priority += 30
	case domain.WorkItemBlocked:
		priority += 20
	}
	return priority
}
