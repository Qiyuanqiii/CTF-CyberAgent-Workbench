package contextmgr

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"cyberagent-workbench/internal/redact"
)

const (
	MaxContextSections     = 512
	MaxContextSectionBytes = 128 * 1024
)

type Section struct {
	Kind     string
	SourceID string
	Content  string
	Priority int
}

type Source struct {
	Kind     string
	SourceID string
	Tokens   int
}

type Selection struct {
	Sections        []Section
	IncludedSources []Source
	OmittedSources  []Source
	EstimatedTokens int
	TokenBudget     int
}

func SelectSections(sections []Section, maxTokens int) (Selection, error) {
	if maxTokens <= 0 {
		return Selection{}, errors.New("context token budget must be positive")
	}
	if len(sections) > MaxContextSections {
		return Selection{}, fmt.Errorf("context section list exceeds %d items", MaxContextSections)
	}
	type candidate struct {
		section Section
		tokens  int
		order   int
	}
	candidates := make([]candidate, 0, len(sections))
	seen := make(map[string]struct{}, len(sections))
	for index, section := range sections {
		section.Kind = strings.TrimSpace(section.Kind)
		section.SourceID = strings.TrimSpace(section.SourceID)
		section.Content = strings.TrimSpace(redact.String(section.Content))
		if section.Kind == "" || section.SourceID == "" {
			return Selection{}, errors.New("context section kind and source id are required")
		}
		if section.Priority < 0 || section.Priority > 1000 {
			return Selection{}, fmt.Errorf("context section priority %d is outside 0..1000", section.Priority)
		}
		if len([]byte(section.Content)) > MaxContextSectionBytes {
			return Selection{}, fmt.Errorf("context section %s/%s exceeds %d bytes", section.Kind, section.SourceID, MaxContextSectionBytes)
		}
		if section.Content == "" {
			continue
		}
		key := section.Kind + "\x00" + section.SourceID
		if _, ok := seen[key]; ok {
			return Selection{}, fmt.Errorf("duplicate context source %s/%s", section.Kind, section.SourceID)
		}
		seen[key] = struct{}{}
		candidates = append(candidates, candidate{section: section, tokens: EstimateTokens(section.Content), order: index})
	}
	sort.SliceStable(candidates, func(left int, right int) bool {
		if candidates[left].section.Priority == candidates[right].section.Priority {
			return candidates[left].order < candidates[right].order
		}
		return candidates[left].section.Priority > candidates[right].section.Priority
	})
	selection := Selection{
		Sections: make([]Section, 0, len(candidates)), IncludedSources: make([]Source, 0, len(candidates)),
		OmittedSources: make([]Source, 0), TokenBudget: maxTokens,
	}
	for _, candidate := range candidates {
		source := Source{Kind: candidate.section.Kind, SourceID: candidate.section.SourceID, Tokens: candidate.tokens}
		if candidate.tokens > maxTokens-selection.EstimatedTokens {
			selection.OmittedSources = append(selection.OmittedSources, source)
			continue
		}
		selection.Sections = append(selection.Sections, candidate.section)
		selection.IncludedSources = append(selection.IncludedSources, source)
		selection.EstimatedTokens += candidate.tokens
	}
	return selection, nil
}
