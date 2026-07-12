package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
)

func (a *App) noteCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("note subcommand is required")
	}
	if err := a.ensureStore(); err != nil {
		return err
	}
	service := application.NewNoteService(a.store)
	switch args[0] {
	case "create":
		return a.noteCreate(ctx, service, args[1:])
	case "list":
		return a.noteList(ctx, service, args[1:])
	case "show", "get":
		return a.noteShow(ctx, service, args[1:])
	case "update":
		return a.noteUpdate(ctx, service, args[1:])
	case "archive":
		return a.noteTransition(ctx, service, domain.NoteArchived, args[1:])
	case "restore":
		return a.noteTransition(ctx, service, domain.NoteActive, args[1:])
	default:
		return fmt.Errorf("unknown note subcommand %q", args[0])
	}
}

func (a *App) noteCreate(ctx context.Context, service *application.NoteService, args []string) error {
	fs := newFlagSet("note create", a.errOut)
	content := fs.String("content", "", "note content")
	contentFile := fs.String("content-file", "", "UTF-8 file containing note content")
	category := fs.String("category", string(domain.NoteObservation), "note category")
	visibility := fs.String("visibility", string(domain.NoteVisibilityRun), "run, root, or owner")
	owner := fs.String("owner", "", "owner label for owner visibility")
	ownerAgent := fs.String("owner-agent", "", "same-Run Agent id that owns the note")
	pinned := fs.Bool("pin", false, "pin note for context selection")
	var tags stringListFlag
	var sources stringListFlag
	var evidence stringListFlag
	fs.Var(&tags, "tag", "note tag; repeat for multiple values")
	fs.Var(&sources, "source", "source reference; repeat for multiple values")
	fs.Var(&evidence, "evidence", "evidence id; repeat for multiple values")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"content": true, "content-file": true, "category": true, "visibility": true,
		"owner": true, "owner-agent": true,
		"pin": false, "tag": true, "source": true, "evidence": true,
	})); err != nil {
		return err
	}
	contentSet := flagWasSet(fs, "content")
	contentFileSet := flagWasSet(fs, "content-file")
	if fs.NArg() < 2 || contentSet == contentFileSet {
		return errors.New(`usage: cyberagent note create <run-id> "title" (--content <text> | --content-file <path>)`)
	}
	value := *content
	if contentFileSet {
		loaded, err := readNoteContent(*contentFile)
		if err != nil {
			return err
		}
		value = loaded
	}
	note, err := service.Create(ctx, application.CreateNoteRequest{
		RunID: fs.Arg(0), Title: strings.Join(fs.Args()[1:], " "), Content: value,
		Category: *category, Visibility: *visibility, Owner: *owner, OwnerAgentID: *ownerAgent,
		Tags:       tags.values,
		SourceRefs: sources.values, EvidenceIDs: evidence.values, Pinned: *pinned,
	})
	if err != nil {
		return err
	}
	printNoteSummary(a.out, "created", note)
	return nil
}

func (a *App) noteList(ctx context.Context, service *application.NoteService, args []string) error {
	fs := newFlagSet("note list", a.errOut)
	statusValue := fs.String("status", "", "comma-separated active or archived statuses")
	categoryValue := fs.String("category", "", "comma-separated note categories")
	visibilityValue := fs.String("visibility", "", "comma-separated note visibilities")
	owner := fs.String("owner", "", "exact owner filter")
	ownerAgent := fs.String("owner-agent", "", "exact owner Agent id filter")
	pinnedValue := fs.String("pinned", "", "true or false")
	limit := fs.Int("limit", 100, "maximum rows")
	var tags stringListFlag
	fs.Var(&tags, "tag", "required tag; repeat to require all tags")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"status": true, "category": true, "visibility": true, "owner": true, "owner-agent": true,
		"pinned": true, "limit": true, "tag": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent note list <run-id> [--status active] [--category decision] [--tag <tag>]")
	}
	if *limit <= 0 || *limit > 500 {
		return errors.New("note list limit must be between 1 and 500")
	}
	statuses, err := parseNoteStatuses(*statusValue)
	if err != nil {
		return err
	}
	categories, err := parseNoteCategories(*categoryValue)
	if err != nil {
		return err
	}
	visibilities, err := parseNoteVisibilities(*visibilityValue)
	if err != nil {
		return err
	}
	pinned, err := parseOptionalBool(*pinnedValue)
	if err != nil {
		return err
	}
	notes, err := service.List(ctx, domain.NoteFilter{
		RunID: fs.Arg(0), Statuses: statuses, Categories: categories, Visibilities: visibilities,
		Owner: *owner, OwnerAgentID: *ownerAgent, Tags: tags.values, Pinned: pinned, Limit: *limit,
	})
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		fmt.Fprintln(a.out, "no notes")
		return nil
	}
	for _, note := range notes {
		pin := "-"
		if note.Pinned {
			pin = "pinned"
		}
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\tv%d\ttags=%d\t%s\n",
			note.ID, note.Status, note.Category, note.Visibility, pin, note.Version, len(note.Tags), note.Title)
	}
	return nil
}

func (a *App) noteShow(ctx context.Context, service *application.NoteService, args []string) error {
	fs := newFlagSet("note show", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent note show <note-id>")
	}
	note, err := service.Get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	printNote(a.out, note)
	return nil
}

func (a *App) noteUpdate(ctx context.Context, service *application.NoteService, args []string) error {
	fs := newFlagSet("note update", a.errOut)
	title := fs.String("title", "", "replacement title")
	content := fs.String("content", "", "replacement content")
	contentFile := fs.String("content-file", "", "UTF-8 file containing replacement content")
	category := fs.String("category", "", "replacement category")
	visibility := fs.String("visibility", "", "replacement visibility")
	owner := fs.String("owner", "", "replacement owner; empty clears it")
	ownerAgent := fs.String("owner-agent", "", "replacement owner Agent id; empty clears it")
	version := fs.Int64("version", 0, "expected version; zero uses the current version")
	pin := fs.Bool("pin", false, "pin the note")
	unpin := fs.Bool("unpin", false, "unpin the note")
	clearTags := fs.Bool("clear-tags", false, "clear all tags")
	clearSources := fs.Bool("clear-sources", false, "clear all sources")
	clearEvidence := fs.Bool("clear-evidence", false, "clear all evidence ids")
	var tags stringListFlag
	var sources stringListFlag
	var evidence stringListFlag
	fs.Var(&tags, "tag", "replacement tag; repeat for multiple values")
	fs.Var(&sources, "source", "replacement source; repeat for multiple values")
	fs.Var(&evidence, "evidence", "replacement evidence id; repeat for multiple values")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"title": true, "content": true, "content-file": true, "category": true, "visibility": true,
		"owner": true, "owner-agent": true, "version": true, "pin": false, "unpin": false,
		"clear-tags": false, "clear-sources": false, "clear-evidence": false,
		"tag": true, "source": true, "evidence": true,
	})); err != nil {
		return err
	}
	if fs.NArg() != 1 || (*pin && *unpin) || (flagWasSet(fs, "content") && flagWasSet(fs, "content-file")) ||
		(tags.set && *clearTags) || (sources.set && *clearSources) || (evidence.set && *clearEvidence) {
		return errors.New("usage: cyberagent note update <note-id> [--content <text> | --content-file <path>] [--version <n>]")
	}
	visited := visitedFlags(fs)
	req := application.UpdateNoteRequest{ID: fs.Arg(0), ExpectedVersion: *version}
	if visited["title"] {
		req.Title = title
	}
	if visited["content"] {
		req.Content = content
	}
	if visited["content-file"] {
		loaded, err := readNoteContent(*contentFile)
		if err != nil {
			return err
		}
		req.Content = &loaded
	}
	if visited["category"] {
		req.Category = category
	}
	if visited["visibility"] {
		req.Visibility = visibility
	}
	if visited["owner"] {
		req.Owner = owner
	}
	if visited["owner-agent"] {
		req.OwnerAgentID = ownerAgent
	}
	if *pin || *unpin {
		value := *pin
		req.Pinned = &value
	}
	if tags.set || *clearTags {
		values := tags.values
		if *clearTags {
			values = []string{}
		}
		req.Tags = &values
	}
	if sources.set || *clearSources {
		values := sources.values
		if *clearSources {
			values = []string{}
		}
		req.SourceRefs = &values
	}
	if evidence.set || *clearEvidence {
		values := evidence.values
		if *clearEvidence {
			values = []string{}
		}
		req.EvidenceIDs = &values
	}
	note, err := service.Update(ctx, req)
	if err != nil {
		return err
	}
	printNoteSummary(a.out, "updated", note)
	return nil
}

func (a *App) noteTransition(ctx context.Context, service *application.NoteService, target domain.NoteStatus, args []string) error {
	name := "archive"
	if target == domain.NoteActive {
		name = "restore"
	}
	fs := newFlagSet("note "+name, a.errOut)
	version := fs.Int64("version", 0, "expected version; zero uses the current version")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"version": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: cyberagent note %s <note-id> [--version <n>]", name)
	}
	note, err := service.Transition(ctx, fs.Arg(0), *version, target)
	if err != nil {
		return err
	}
	printNoteSummary(a.out, name+"d", note)
	return nil
}

func parseNoteStatuses(value string) ([]domain.NoteStatus, error) {
	parts := splitCommaValues(value)
	statuses := make([]domain.NoteStatus, 0, len(parts))
	for _, part := range parts {
		status, err := domain.ParseNoteStatus(part)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func parseNoteCategories(value string) ([]domain.NoteCategory, error) {
	parts := splitCommaValues(value)
	categories := make([]domain.NoteCategory, 0, len(parts))
	for _, part := range parts {
		category, err := domain.ParseNoteCategory(part)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, nil
}

func parseNoteVisibilities(value string) ([]domain.NoteVisibility, error) {
	parts := splitCommaValues(value)
	visibilities := make([]domain.NoteVisibility, 0, len(parts))
	for _, part := range parts {
		visibility, err := domain.ParseNoteVisibility(part)
		if err != nil {
			return nil, err
		}
		visibilities = append(visibilities, visibility)
	}
	return visibilities, nil
}

func splitCommaValues(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func parseOptionalBool(value string) (*bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("invalid note pinned filter %q; must be true or false", value)
	}
	return &parsed, nil
}

func readNoteContent(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("note content file path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("note content file %s is a directory", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, domain.MaxNoteContentBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > domain.MaxNoteContentBytes {
		return "", fmt.Errorf("note content file exceeds %d bytes", domain.MaxNoteContentBytes)
	}
	if !utf8.Valid(data) {
		return "", errors.New("note content file is not valid UTF-8 text")
	}
	return string(data), nil
}

func printNoteSummary(out io.Writer, action string, note domain.Note) {
	fmt.Fprintf(out, "note %s %s\nrun: %s\nstatus: %s\ncategory: %s\nvisibility: %s\npinned: %t\nversion: %d\n",
		note.ID, action, note.RunID, note.Status, note.Category, note.Visibility, note.Pinned, note.Version)
}

func printNote(out io.Writer, note domain.Note) {
	fmt.Fprintf(out, "id: %s\nrun: %s\ntitle: %s\nstatus: %s\ncategory: %s\nvisibility: %s\nowner: %s\nowner_agent: %s\npinned: %t\nversion: %d\ncontent:\n%s\n",
		note.ID, note.RunID, note.Title, note.Status, note.Category, note.Visibility, note.Owner,
		note.OwnerAgentID, note.Pinned, note.Version, note.Content)
	printStringList(out, "tags", note.Tags)
	printStringList(out, "sources", note.SourceRefs)
	printStringList(out, "evidence", note.EvidenceIDs)
	fmt.Fprintf(out, "created_at: %s\nupdated_at: %s\n", note.CreatedAt.Format(time.RFC3339), note.UpdatedAt.Format(time.RFC3339))
	if note.ArchivedAt != nil {
		fmt.Fprintf(out, "archived_at: %s\n", note.ArchivedAt.Format(time.RFC3339))
	}
}
