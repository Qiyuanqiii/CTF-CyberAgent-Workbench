package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
)

const maxNoteListLimit = 500

func (s *SQLiteStore) CreateNote(ctx context.Context, note domain.Note, event events.Event) error {
	note = redactAndNormalizeNote(note)
	if err := validateNewNote(note, event); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	missionID, err := mutableRunMissionTx(ctx, tx, note.RunID)
	if err != nil {
		return err
	}
	if event.MissionID != missionID {
		return apperror.New(apperror.CodeInvalidArgument, "note create event mission does not match the run")
	}
	if err := insertNewNoteTx(ctx, tx, note); err != nil {
		return err
	}
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func validateNewNote(note domain.Note, event events.Event) error {
	if err := note.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if note.Version != 1 || note.Status != domain.NoteActive {
		return apperror.New(apperror.CodeInvalidArgument, "new note must be active at version 1")
	}
	if event.Type != events.NoteCreatedEvent || event.SubjectID != note.ID || event.RunID != note.RunID {
		return apperror.New(apperror.CodeInvalidArgument, "note create event does not match the note")
	}
	if err := event.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return nil
}

func insertNewNoteTx(ctx context.Context, tx *sql.Tx, note domain.Note) error {
	if err := requireAssignableAgentOwnerTx(ctx, tx, note.RunID, note.OwnerAgentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notes
		(id, run_id, title, content, category, visibility, owner, owner_agent_id, status, pinned, version, created_at, updated_at, archived_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, note.ID, note.RunID, note.Title, note.Content,
		note.Category, note.Visibility, note.Owner, nullableAgentID(note.OwnerAgentID), note.Status,
		boolInt(note.Pinned), note.Version,
		ts(note.CreatedAt), ts(note.UpdatedAt), nullableTS(note.ArchivedAt)); err != nil {
		return err
	}
	return replaceNoteRelationsTx(ctx, tx, note)
}

func (s *SQLiteStore) GetNote(ctx context.Context, id string) (domain.Note, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Note{}, apperror.New(apperror.CodeInvalidArgument, "note id is required")
	}
	note, err := scanNote(s.db.QueryRowContext(ctx, noteSelect+` WHERE id = ?`, id))
	if err != nil {
		return domain.Note{}, err
	}
	relations, err := loadNoteRelations(ctx, s.db, note.RunID, []string{note.ID})
	if err != nil {
		return domain.Note{}, err
	}
	applyNoteRelations(&note, relations)
	return note, note.Validate()
}

func (s *SQLiteStore) ListNotes(ctx context.Context, filter domain.NoteFilter) ([]domain.Note, error) {
	filter.RunID = strings.TrimSpace(filter.RunID)
	filter.Owner = strings.TrimSpace(filter.Owner)
	filter.Viewer = strings.TrimSpace(filter.Viewer)
	filter.OwnerAgentID = strings.TrimSpace(filter.OwnerAgentID)
	filter.ViewerAgentID = strings.TrimSpace(filter.ViewerAgentID)
	if filter.RunID == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, "note list run id is required")
	}
	if err := validateStoreListOffset(filter.Offset); err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	query := noteSelect + ` WHERE run_id = ?`
	args := []any{filter.RunID}
	if len(filter.Statuses) > 0 {
		values, err := uniqueNoteStatuses(filter.Statuses)
		if err != nil {
			return nil, err
		}
		query += ` AND status IN (` + placeholders(len(values)) + `)`
		for _, value := range values {
			args = append(args, value)
		}
	}
	if len(filter.Categories) > 0 {
		values, err := uniqueNoteCategories(filter.Categories)
		if err != nil {
			return nil, err
		}
		query += ` AND category IN (` + placeholders(len(values)) + `)`
		for _, value := range values {
			args = append(args, value)
		}
	}
	if len(filter.Visibilities) > 0 {
		values, err := uniqueNoteVisibilities(filter.Visibilities)
		if err != nil {
			return nil, err
		}
		query += ` AND visibility IN (` + placeholders(len(values)) + `)`
		for _, value := range values {
			args = append(args, value)
		}
	}
	if filter.Owner != "" {
		query += ` AND owner = ?`
		args = append(args, filter.Owner)
	}
	if filter.OwnerAgentID != "" {
		if !domain.ValidAgentID(filter.OwnerAgentID) {
			return nil, apperror.New(apperror.CodeInvalidArgument, "note owner Agent filter is invalid")
		}
		query += ` AND owner_agent_id = ?`
		args = append(args, filter.OwnerAgentID)
	}
	if filter.Viewer != "" || filter.ViewerAgentID != "" {
		if len(filter.Visibilities) > 0 || filter.Owner != "" || filter.OwnerAgentID != "" {
			return nil, apperror.New(apperror.CodeInvalidArgument, "note viewer cannot be combined with visibility or owner filters")
		}
		if len([]rune(filter.Viewer)) > domain.MaxNoteOwnerRunes {
			return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("note viewer exceeds %d characters", domain.MaxNoteOwnerRunes))
		}
		viewerIsRoot := filter.Viewer == "root"
		ownerChecks := make([]string, 0, 2)
		ownerArgs := make([]any, 0, 2)
		if filter.Viewer != "" {
			ownerChecks = append(ownerChecks, "owner = ?")
			ownerArgs = append(ownerArgs, filter.Viewer)
		}
		if filter.ViewerAgentID != "" {
			if !domain.ValidAgentID(filter.ViewerAgentID) {
				return nil, apperror.New(apperror.CodeInvalidArgument, "note viewer Agent identity is invalid")
			}
			viewerAgent, err := s.noteViewerAgent(ctx, filter.RunID, filter.ViewerAgentID)
			if err != nil {
				return nil, err
			}
			viewerIsRoot = viewerIsRoot || viewerAgent.Role == domain.AgentRoleRoot
			ownerChecks = append(ownerChecks, "owner_agent_id = ?")
			ownerArgs = append(ownerArgs, viewerAgent.ID)
		}
		ownerMatch := "(" + strings.Join(ownerChecks, " OR ") + ")"
		if viewerIsRoot {
			query += ` AND (visibility IN (?, ?) OR (visibility = ? AND ` + ownerMatch + `))`
			args = append(args, domain.NoteVisibilityRun, domain.NoteVisibilityRoot, domain.NoteVisibilityOwner)
		} else {
			query += ` AND (visibility = ? OR (visibility = ? AND ` + ownerMatch + `))`
			args = append(args, domain.NoteVisibilityRun, domain.NoteVisibilityOwner)
		}
		args = append(args, ownerArgs...)
	}
	if filter.Pinned != nil {
		query += ` AND pinned = ?`
		args = append(args, boolInt(*filter.Pinned))
	}
	tags, err := domain.NormalizeNoteTags(filter.Tags)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	for _, tag := range tags {
		query += ` AND EXISTS (SELECT 1 FROM note_tags filter_tag
			WHERE filter_tag.run_id = notes.run_id AND filter_tag.note_id = notes.id AND filter_tag.tag = ?)`
		args = append(args, tag)
	}
	limit := filter.Limit
	if limit < 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("note list limit must be between 1 and %d", maxNoteListLimit))
	}
	if limit == 0 {
		limit = 100
	}
	if limit > maxNoteListLimit {
		return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("note list limit must be between 1 and %d", maxNoteListLimit))
	}
	query += ` ORDER BY pinned DESC,
		CASE category WHEN 'decision' THEN 0 WHEN 'summary' THEN 1 WHEN 'observation' THEN 2 WHEN 'hypothesis' THEN 3 ELSE 4 END,
		updated_at DESC, id LIMIT ? OFFSET ?`
	args = append(args, limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	notes := make([]domain.Note, 0)
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	ids := make([]string, len(notes))
	for index := range notes {
		ids[index] = notes[index].ID
	}
	relations, err := loadNoteRelations(ctx, s.db, filter.RunID, ids)
	if err != nil {
		return nil, err
	}
	for index := range notes {
		applyNoteRelations(&notes[index], relations)
		if err := notes[index].Validate(); err != nil {
			return nil, err
		}
	}
	return notes, nil
}

func (s *SQLiteStore) UpdateNote(ctx context.Context, note domain.Note, expectedVersion int64, event events.Event) error {
	note = redactAndNormalizeNote(note)
	if err := note.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if expectedVersion <= 0 || note.Version != expectedVersion+1 {
		return apperror.New(apperror.CodeInvalidArgument, "note update requires the next expected version")
	}
	if event.Type != events.NoteChangedEvent || event.SubjectID != note.ID || event.RunID != note.RunID {
		return apperror.New(apperror.CodeInvalidArgument, "note change event does not match the note")
	}
	if err := event.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getNoteTx(ctx, tx, note.ID)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return apperror.New(apperror.CodeConflict,
			fmt.Sprintf("note %s changed concurrently: expected version %d, got %d", note.ID, expectedVersion, current.Version))
	}
	if err := validateNoteReplacement(current, note); err != nil {
		return err
	}
	missionID, err := mutableRunMissionTx(ctx, tx, note.RunID)
	if err != nil {
		return err
	}
	if event.MissionID != missionID {
		return apperror.New(apperror.CodeInvalidArgument, "note change event mission does not match the run")
	}
	if current.OwnerAgentID != note.OwnerAgentID {
		if err := requireAssignableAgentOwnerTx(ctx, tx, note.RunID, note.OwnerAgentID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE notes SET title = ?, content = ?, category = ?, visibility = ?,
		owner = ?, owner_agent_id = ?, status = ?, pinned = ?, version = ?, updated_at = ?, archived_at = ?
		WHERE id = ? AND run_id = ? AND version = ?`, note.Title, note.Content, note.Category, note.Visibility,
		note.Owner, nullableAgentID(note.OwnerAgentID), note.Status, boolInt(note.Pinned), note.Version,
		ts(note.UpdatedAt), nullableTS(note.ArchivedAt),
		note.ID, note.RunID, expectedVersion)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeConflict, fmt.Sprintf("note %s changed concurrently or was not found", note.ID))
	}
	if !sameNoteDetails(current, note) {
		if err := replaceNoteRelationsTx(ctx, tx, note); err != nil {
			return err
		}
	}
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

const noteSelect = `SELECT id, run_id, title, content, category, visibility, owner, owner_agent_id, status, pinned,
	version, created_at, updated_at, archived_at FROM notes`

func getNoteTx(ctx context.Context, tx *sql.Tx, id string) (domain.Note, error) {
	note, err := scanNote(tx.QueryRowContext(ctx, noteSelect+` WHERE id = ?`, strings.TrimSpace(id)))
	if err != nil {
		return domain.Note{}, err
	}
	relations, err := loadNoteRelations(ctx, tx, note.RunID, []string{note.ID})
	if err != nil {
		return domain.Note{}, err
	}
	applyNoteRelations(&note, relations)
	return note, note.Validate()
}

func scanNote(row scanner) (domain.Note, error) {
	var note domain.Note
	var category string
	var visibility string
	var status string
	var pinned int
	var created string
	var updated string
	var archived sql.NullString
	var ownerAgentID sql.NullString
	if err := row.Scan(&note.ID, &note.RunID, &note.Title, &note.Content, &category, &visibility,
		&note.Owner, &ownerAgentID, &status, &pinned, &note.Version, &created, &updated, &archived); err != nil {
		return domain.Note{}, err
	}
	note.OwnerAgentID = ownerAgentID.String
	note.Category = domain.NoteCategory(category)
	note.Visibility = domain.NoteVisibility(visibility)
	note.Status = domain.NoteStatus(status)
	note.Pinned = pinned != 0
	note.CreatedAt = parseTS(created)
	note.UpdatedAt = parseTS(updated)
	note.ArchivedAt = parseNullableTS(archived)
	return note, nil
}

type noteRelationSet struct {
	tags     map[string][]string
	sources  map[string][]string
	evidence map[string][]string
}

func loadNoteRelations(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string, ids []string) (noteRelationSet, error) {
	set := noteRelationSet{
		tags: make(map[string][]string, len(ids)), sources: make(map[string][]string, len(ids)), evidence: make(map[string][]string, len(ids)),
	}
	for _, id := range ids {
		set.tags[id] = []string{}
		set.sources[id] = []string{}
		set.evidence[id] = []string{}
	}
	if len(ids) == 0 {
		return set, nil
	}
	var err error
	set.tags, err = loadNoteRelationValues(ctx, queryer, "note_tags", "tag", runID, ids, set.tags)
	if err != nil {
		return noteRelationSet{}, err
	}
	set.sources, err = loadNoteRelationValues(ctx, queryer, "note_sources", "source_ref", runID, ids, set.sources)
	if err != nil {
		return noteRelationSet{}, err
	}
	set.evidence, err = loadNoteRelationValues(ctx, queryer, "note_evidence", "evidence_id", runID, ids, set.evidence)
	if err != nil {
		return noteRelationSet{}, err
	}
	return set, nil
}

func loadNoteRelationValues(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, table string, valueColumn string, runID string, ids []string, out map[string][]string) (map[string][]string, error) {
	allowed := map[string]string{
		"note_tags": "tag", "note_sources": "source_ref", "note_evidence": "evidence_id",
	}
	if allowed[table] != valueColumn {
		return nil, apperror.New(apperror.CodeInternal, "invalid note relation query")
	}
	query := `SELECT note_id, ` + valueColumn + ` FROM ` + table + `
		WHERE run_id = ? AND note_id IN (` + placeholders(len(ids)) + `) ORDER BY note_id, ` + valueColumn
	args := make([]any, 0, len(ids)+1)
	args = append(args, runID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var noteID string
		var value string
		if err := rows.Scan(&noteID, &value); err != nil {
			return nil, err
		}
		out[noteID] = append(out[noteID], value)
	}
	return out, rows.Err()
}

func applyNoteRelations(note *domain.Note, relations noteRelationSet) {
	if note == nil {
		return
	}
	note.Tags = relations.tags[note.ID]
	note.SourceRefs = relations.sources[note.ID]
	note.EvidenceIDs = relations.evidence[note.ID]
}

func replaceNoteRelationsTx(ctx context.Context, tx *sql.Tx, note domain.Note) error {
	for _, table := range []string{"note_tags", "note_sources", "note_evidence"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE run_id = ? AND note_id = ?`, note.RunID, note.ID); err != nil {
			return err
		}
	}
	for _, tag := range note.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_tags (run_id, note_id, tag, created_at) VALUES (?, ?, ?, ?)`,
			note.RunID, note.ID, tag, ts(note.UpdatedAt)); err != nil {
			return err
		}
	}
	for _, source := range note.SourceRefs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_sources (run_id, note_id, source_ref, created_at) VALUES (?, ?, ?, ?)`,
			note.RunID, note.ID, source, ts(note.UpdatedAt)); err != nil {
			return err
		}
	}
	for _, evidenceID := range note.EvidenceIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_evidence (run_id, note_id, evidence_id, created_at) VALUES (?, ?, ?, ?)`,
			note.RunID, note.ID, evidenceID, ts(note.UpdatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func validateNoteReplacement(current domain.Note, next domain.Note) error {
	if current.ID != next.ID || current.RunID != next.RunID || !current.CreatedAt.Equal(next.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument, "note identity, run, and created_at are immutable")
	}
	if next.UpdatedAt.Before(current.UpdatedAt) {
		return apperror.New(apperror.CodeInvalidArgument, "note updated_at cannot move backwards")
	}
	if current.Status != next.Status {
		if !sameNoteDetails(current, next) {
			return apperror.New(apperror.CodeInvalidArgument, "note details and status must be changed separately")
		}
		expected := current
		if err := expected.Transition(next.Status, next.UpdatedAt); err != nil {
			return apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
		}
		if !equalNullableTime(expected.ArchivedAt, next.ArchivedAt) {
			return apperror.New(apperror.CodeInvalidArgument, "note transition timestamp is inconsistent")
		}
		return nil
	}
	if !equalNullableTime(current.ArchivedAt, next.ArchivedAt) {
		return apperror.New(apperror.CodeInvalidArgument, "note archived_at cannot change without a transition")
	}
	expected := current
	if err := expected.ApplyDetails(domain.NoteDetails{
		Title: next.Title, Content: next.Content, Category: next.Category, Visibility: next.Visibility,
		Owner: next.Owner, OwnerAgentID: next.OwnerAgentID, Tags: next.Tags,
		SourceRefs: next.SourceRefs, EvidenceIDs: next.EvidenceIDs, Pinned: next.Pinned,
	}, next.UpdatedAt); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
	}
	return nil
}

func sameNoteDetails(left domain.Note, right domain.Note) bool {
	return left.Title == right.Title && left.Content == right.Content && left.Category == right.Category &&
		left.Visibility == right.Visibility && left.Owner == right.Owner &&
		left.OwnerAgentID == right.OwnerAgentID && left.Pinned == right.Pinned &&
		slices.Equal(left.Tags, right.Tags) && slices.Equal(left.SourceRefs, right.SourceRefs) &&
		slices.Equal(left.EvidenceIDs, right.EvidenceIDs)
}

func redactAndNormalizeNote(note domain.Note) domain.Note {
	details, err := domain.NormalizeNoteDetails(domain.NoteDetails{
		Title: redact.String(note.Title), Content: redact.String(note.Content), Category: note.Category,
		Visibility: note.Visibility, Owner: redact.String(note.Owner), OwnerAgentID: note.OwnerAgentID,
		Tags:       redactStrings(note.Tags),
		SourceRefs: redactStrings(note.SourceRefs), EvidenceIDs: redactStrings(note.EvidenceIDs), Pinned: note.Pinned,
	})
	if err == nil {
		note.Title = details.Title
		note.Content = details.Content
		note.Category = details.Category
		note.Visibility = details.Visibility
		note.Owner = details.Owner
		note.OwnerAgentID = details.OwnerAgentID
		note.Tags = details.Tags
		note.SourceRefs = details.SourceRefs
		note.EvidenceIDs = details.EvidenceIDs
		note.Pinned = details.Pinned
	}
	return note
}

func uniqueNoteStatuses(values []domain.NoteStatus) ([]domain.NoteStatus, error) {
	seen := make(map[domain.NoteStatus]struct{}, len(values))
	out := make([]domain.NoteStatus, 0, len(values))
	for _, value := range values {
		if !domain.ValidNoteStatus(value) {
			return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("invalid note status %q", value))
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out, nil
}

func uniqueNoteCategories(values []domain.NoteCategory) ([]domain.NoteCategory, error) {
	seen := make(map[domain.NoteCategory]struct{}, len(values))
	out := make([]domain.NoteCategory, 0, len(values))
	for _, value := range values {
		if !domain.ValidNoteCategory(value) {
			return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("invalid note category %q", value))
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out, nil
}

func uniqueNoteVisibilities(values []domain.NoteVisibility) ([]domain.NoteVisibility, error) {
	seen := make(map[domain.NoteVisibility]struct{}, len(values))
	out := make([]domain.NoteVisibility, 0, len(values))
	for _, value := range values {
		if !domain.ValidNoteVisibility(value) {
			return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("invalid note visibility %q", value))
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out, nil
}
