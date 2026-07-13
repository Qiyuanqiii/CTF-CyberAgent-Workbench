package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
)

const maxWorkItemListLimit = 500

func (s *SQLiteStore) CreateWorkItem(ctx context.Context, item domain.WorkItem, event events.Event) error {
	item = redactAndNormalizeWorkItem(item)
	if err := validateNewWorkItem(item, event); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	missionID, err := mutableRunMissionTx(ctx, tx, item.RunID)
	if err != nil {
		return err
	}
	if event.MissionID != missionID {
		return apperror.New(apperror.CodeInvalidArgument, "work item create event mission does not match the run")
	}
	if err := insertNewWorkItemTx(ctx, tx, item); err != nil {
		return err
	}
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func validateNewWorkItem(item domain.WorkItem, event events.Event) error {
	if err := item.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if item.Version != 1 || item.Status != domain.WorkItemPending {
		return apperror.New(apperror.CodeInvalidArgument, "new work item must be pending at version 1")
	}
	if event.Type != events.WorkItemCreatedEvent || event.SubjectID != item.ID || event.RunID != item.RunID {
		return apperror.New(apperror.CodeInvalidArgument, "work item create event does not match the item")
	}
	if err := event.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	return nil
}

func insertNewWorkItemTx(ctx context.Context, tx *sql.Tx, item domain.WorkItem) error {
	if err := requireAssignableAgentOwnerTx(ctx, tx, item.RunID, item.OwnerAgentID); err != nil {
		return err
	}
	if err := ensureWorkItemDependenciesTx(ctx, tx, item.RunID, item.ID, item.Dependencies); err != nil {
		return err
	}
	acceptanceJSON, err := json.Marshal(item.AcceptanceCriteria)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO work_items
		(id, run_id, title, description, status, priority, owner, owner_agent_id, acceptance_json, blocked_reason,
		 version, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.RunID, item.Title,
		item.Description, item.Status, item.Priority, item.Owner, nullableAgentID(item.OwnerAgentID),
		string(acceptanceJSON), item.BlockedReason,
		item.Version, ts(item.CreatedAt), ts(item.UpdatedAt), nullableTS(item.CompletedAt)); err != nil {
		return err
	}
	return replaceWorkItemDependenciesTx(ctx, tx, item)
}

func (s *SQLiteStore) GetWorkItem(ctx context.Context, id string) (domain.WorkItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.WorkItem{}, apperror.New(apperror.CodeInvalidArgument, "work item id is required")
	}
	item, err := scanWorkItem(s.db.QueryRowContext(ctx, workItemSelect+` WHERE id = ?`, id))
	if err != nil {
		return domain.WorkItem{}, err
	}
	item.Dependencies, err = loadWorkItemDependencies(ctx, s.db, item.RunID, []string{item.ID})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return item, item.Validate()
}

func (s *SQLiteStore) ListWorkItems(ctx context.Context, filter domain.WorkItemFilter) ([]domain.WorkItem, error) {
	filter.RunID = strings.TrimSpace(filter.RunID)
	filter.Owner = strings.TrimSpace(filter.Owner)
	filter.OwnerAgentID = strings.TrimSpace(filter.OwnerAgentID)
	if filter.RunID == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, "work item list run id is required")
	}
	if err := validateStoreListOffset(filter.Offset); err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	query := workItemSelect + ` WHERE run_id = ?`
	args := []any{filter.RunID}
	if len(filter.Statuses) > 0 {
		seen := make(map[domain.WorkItemStatus]struct{}, len(filter.Statuses))
		statuses := make([]domain.WorkItemStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			if !domain.ValidWorkItemStatus(status) {
				return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("invalid work item status %q", status))
			}
			if _, ok := seen[status]; ok {
				continue
			}
			seen[status] = struct{}{}
			statuses = append(statuses, status)
		}
		query += ` AND status IN (` + placeholders(len(statuses)) + `)`
		for _, status := range statuses {
			args = append(args, status)
		}
	}
	if filter.Owner != "" {
		query += ` AND owner = ?`
		args = append(args, filter.Owner)
	}
	if filter.OwnerAgentID != "" {
		if !domain.ValidAgentID(filter.OwnerAgentID) {
			return nil, apperror.New(apperror.CodeInvalidArgument,
				"work item owner Agent filter is invalid")
		}
		query += ` AND owner_agent_id = ?`
		args = append(args, filter.OwnerAgentID)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxWorkItemListLimit {
		return nil, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("work item list limit must be between 1 and %d", maxWorkItemListLimit))
	}
	query += ` ORDER BY CASE priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
		CASE status WHEN 'in_progress' THEN 0 WHEN 'blocked' THEN 1 WHEN 'pending' THEN 2 ELSE 3 END,
		updated_at DESC, id LIMIT ? OFFSET ?`
	args = append(args, limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	items := make([]domain.WorkItem, 0)
	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	ids := make([]string, len(items))
	for index := range items {
		ids[index] = items[index].ID
	}
	dependencies, err := loadWorkItemDependencyMap(ctx, s.db, filter.RunID, ids)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index].Dependencies = dependencies[items[index].ID]
		if err := items[index].Validate(); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *SQLiteStore) UpdateWorkItem(ctx context.Context, item domain.WorkItem, expectedVersion int64, event events.Event) error {
	item = redactAndNormalizeWorkItem(item)
	if err := item.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if expectedVersion <= 0 || item.Version != expectedVersion+1 {
		return apperror.New(apperror.CodeInvalidArgument, "work item update requires the next expected version")
	}
	if event.Type != events.WorkItemChangedEvent || event.SubjectID != item.ID || event.RunID != item.RunID {
		return apperror.New(apperror.CodeInvalidArgument, "work item change event does not match the item")
	}
	if err := event.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getWorkItemTx(ctx, tx, item.ID)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return apperror.New(apperror.CodeConflict, fmt.Sprintf("work item %s changed concurrently: expected version %d, got %d", item.ID, expectedVersion, current.Version))
	}
	if err := validateWorkItemReplacement(current, item); err != nil {
		return err
	}
	missionID, err := mutableRunMissionTx(ctx, tx, item.RunID)
	if err != nil {
		return err
	}
	if event.MissionID != missionID {
		return apperror.New(apperror.CodeInvalidArgument, "work item change event mission does not match the run")
	}
	if err := ensureWorkItemDependenciesTx(ctx, tx, item.RunID, item.ID, item.Dependencies); err != nil {
		return err
	}
	if current.OwnerAgentID != item.OwnerAgentID {
		if err := requireAssignableAgentOwnerTx(ctx, tx, item.RunID, item.OwnerAgentID); err != nil {
			return err
		}
	}
	if !slices.Equal(current.Dependencies, item.Dependencies) {
		if err := rejectWorkItemDependencyCycleTx(ctx, tx, item.RunID, item.ID, item.Dependencies); err != nil {
			return err
		}
	}
	if current.Status != item.Status && (item.Status == domain.WorkItemInProgress || item.Status == domain.WorkItemCompleted) {
		if err := requireCompletedWorkItemDependenciesTx(ctx, tx, item.RunID, item.ID); err != nil {
			return err
		}
	}
	if current.Status != item.Status && item.Status == domain.WorkItemCompleted {
		if err := requireSelectedWorkItemDeliveryCheckpointTx(ctx, tx, current); err != nil {
			return err
		}
	}
	acceptanceJSON, err := json.Marshal(item.AcceptanceCriteria)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE work_items SET title = ?, description = ?, status = ?, priority = ?,
		owner = ?, owner_agent_id = ?, acceptance_json = ?, blocked_reason = ?, version = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND run_id = ? AND version = ?`, item.Title, item.Description, item.Status, item.Priority,
		item.Owner, nullableAgentID(item.OwnerAgentID), string(acceptanceJSON), item.BlockedReason,
		item.Version, ts(item.UpdatedAt), nullableTS(item.CompletedAt),
		item.ID, item.RunID, expectedVersion)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeConflict, fmt.Sprintf("work item %s changed concurrently or was not found", item.ID))
	}
	if !slices.Equal(current.Dependencies, item.Dependencies) {
		if err := replaceWorkItemDependenciesTx(ctx, tx, item); err != nil {
			return err
		}
	}
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

const workItemSelect = `SELECT id, run_id, title, description, status, priority, owner, owner_agent_id, acceptance_json,
	blocked_reason, version, created_at, updated_at, completed_at FROM work_items`

func getWorkItemTx(ctx context.Context, tx *sql.Tx, id string) (domain.WorkItem, error) {
	item, err := scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE id = ?`, strings.TrimSpace(id)))
	if err != nil {
		return domain.WorkItem{}, err
	}
	dependencies, err := loadWorkItemDependencyMap(ctx, tx, item.RunID, []string{item.ID})
	if err != nil {
		return domain.WorkItem{}, err
	}
	item.Dependencies = dependencies[item.ID]
	return item, item.Validate()
}

func scanWorkItem(row scanner) (domain.WorkItem, error) {
	var item domain.WorkItem
	var status string
	var priority string
	var acceptanceJSON string
	var created string
	var updated string
	var completed sql.NullString
	var ownerAgentID sql.NullString
	if err := row.Scan(&item.ID, &item.RunID, &item.Title, &item.Description, &status, &priority, &item.Owner,
		&ownerAgentID, &acceptanceJSON, &item.BlockedReason, &item.Version, &created, &updated, &completed); err != nil {
		return domain.WorkItem{}, err
	}
	item.OwnerAgentID = ownerAgentID.String
	item.Status = domain.WorkItemStatus(status)
	item.Priority = domain.WorkItemPriority(priority)
	if err := json.Unmarshal([]byte(acceptanceJSON), &item.AcceptanceCriteria); err != nil {
		return domain.WorkItem{}, fmt.Errorf("decode work item acceptance criteria: %w", err)
	}
	item.CreatedAt = parseTS(created)
	item.UpdatedAt = parseTS(updated)
	item.CompletedAt = parseNullableTS(completed)
	return item, nil
}

func mutableRunMissionTx(ctx context.Context, tx *sql.Tx, runID string) (string, error) {
	var missionID string
	var status domain.RunStatus
	if err := tx.QueryRowContext(ctx, `SELECT mission_id, status FROM runs WHERE id = ?`, strings.TrimSpace(runID)).Scan(&missionID, &status); err != nil {
		return "", err
	}
	if status == domain.RunCompleted || status == domain.RunFailed || status == domain.RunCancelled {
		return "", apperror.New(apperror.CodeFailedPrecondition, fmt.Sprintf("terminal run %s cannot change its work board", runID))
	}
	return missionID, nil
}

func ensureWorkItemDependenciesTx(ctx context.Context, tx *sql.Tx, runID string, itemID string, dependencies []string) error {
	for _, dependencyID := range dependencies {
		var dependencyRunID string
		err := tx.QueryRowContext(ctx, `SELECT run_id FROM work_items WHERE id = ?`, dependencyID).Scan(&dependencyRunID)
		if errors.Is(err, sql.ErrNoRows) {
			return apperror.New(apperror.CodeFailedPrecondition, fmt.Sprintf("work item dependency %s was not found", dependencyID))
		}
		if err != nil {
			return err
		}
		if dependencyRunID != runID {
			return apperror.New(apperror.CodeFailedPrecondition, fmt.Sprintf("work item dependency %s belongs to another run", dependencyID))
		}
		if dependencyID == itemID {
			return apperror.New(apperror.CodeInvalidArgument, "work item cannot depend on itself")
		}
	}
	return nil
}

func rejectWorkItemDependencyCycleTx(ctx context.Context, tx *sql.Tx, runID string, itemID string, dependencies []string) error {
	for _, dependencyID := range dependencies {
		var cycle int
		err := tx.QueryRowContext(ctx, `WITH RECURSIVE reachable(id) AS (
			SELECT depends_on_id FROM work_item_dependencies WHERE run_id = ? AND work_item_id = ?
			UNION
			SELECT edge.depends_on_id FROM work_item_dependencies edge
			JOIN reachable ON edge.work_item_id = reachable.id WHERE edge.run_id = ?
		) SELECT EXISTS(SELECT 1 FROM reachable WHERE id = ?)`, runID, dependencyID, runID, itemID).Scan(&cycle)
		if err != nil {
			return err
		}
		if cycle != 0 {
			return apperror.New(apperror.CodeFailedPrecondition, fmt.Sprintf("work item dependency %s would create a cycle", dependencyID))
		}
	}
	return nil
}

func requireCompletedWorkItemDependenciesTx(ctx context.Context, tx *sql.Tx, runID string, itemID string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_item_dependencies edge
		JOIN work_items dependency ON dependency.run_id = edge.run_id AND dependency.id = edge.depends_on_id
		WHERE edge.run_id = ? AND edge.work_item_id = ? AND dependency.status <> ?`,
		runID, itemID, domain.WorkItemCompleted).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return apperror.New(apperror.CodeFailedPrecondition, fmt.Sprintf("work item %s has %d incomplete dependencies", itemID, count))
	}
	return nil
}

func replaceWorkItemDependenciesTx(ctx context.Context, tx *sql.Tx, item domain.WorkItem) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_item_dependencies WHERE run_id = ? AND work_item_id = ?`, item.RunID, item.ID); err != nil {
		return err
	}
	for _, dependencyID := range item.Dependencies {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_item_dependencies
			(run_id, work_item_id, depends_on_id, created_at) VALUES (?, ?, ?, ?)`,
			item.RunID, item.ID, dependencyID, ts(item.UpdatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func loadWorkItemDependencies(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string, ids []string) ([]string, error) {
	dependencies, err := loadWorkItemDependencyMap(ctx, queryer, runID, ids)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	return dependencies[ids[0]], nil
}

func loadWorkItemDependencyMap(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string, ids []string) (map[string][]string, error) {
	out := make(map[string][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	query := `SELECT work_item_id, depends_on_id FROM work_item_dependencies
		WHERE run_id = ? AND work_item_id IN (` + placeholders(len(ids)) + `) ORDER BY work_item_id, depends_on_id`
	args := make([]any, 0, len(ids)+1)
	args = append(args, runID)
	for _, id := range ids {
		args = append(args, id)
		out[id] = []string{}
	}
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var itemID string
		var dependencyID string
		if err := rows.Scan(&itemID, &dependencyID); err != nil {
			return nil, err
		}
		out[itemID] = append(out[itemID], dependencyID)
	}
	return out, rows.Err()
}

func validateWorkItemReplacement(current domain.WorkItem, next domain.WorkItem) error {
	if current.ID != next.ID || current.RunID != next.RunID || !current.CreatedAt.Equal(next.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument, "work item identity, run, and created_at are immutable")
	}
	if next.UpdatedAt.Before(current.UpdatedAt) {
		return apperror.New(apperror.CodeInvalidArgument, "work item updated_at cannot move backwards")
	}
	if current.Status != next.Status {
		if !sameWorkItemDetails(current, next) {
			return apperror.New(apperror.CodeInvalidArgument, "work item details and status must be changed separately")
		}
		expected := current
		if err := expected.Transition(next.Status, next.BlockedReason, next.UpdatedAt); err != nil {
			return apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
		}
		expected.Version = next.Version
		if expected.BlockedReason != next.BlockedReason || !equalNullableTime(expected.CompletedAt, next.CompletedAt) {
			return apperror.New(apperror.CodeInvalidArgument, "work item transition markers are inconsistent")
		}
		return nil
	}
	if current.BlockedReason != next.BlockedReason || !equalNullableTime(current.CompletedAt, next.CompletedAt) {
		return apperror.New(apperror.CodeInvalidArgument, "work item status markers cannot change without a transition")
	}
	expected := current
	if err := expected.ApplyDetails(workItemDetails(next), next.UpdatedAt); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition, err.Error(), err)
	}
	return nil
}

func redactAndNormalizeWorkItem(item domain.WorkItem) domain.WorkItem {
	details, err := domain.NormalizeWorkItemDetails(item.ID, domain.WorkItemDetails{
		Title:              redact.String(item.Title),
		Description:        redact.String(item.Description),
		Priority:           item.Priority,
		Owner:              redact.String(item.Owner),
		OwnerAgentID:       item.OwnerAgentID,
		AcceptanceCriteria: redactStrings(item.AcceptanceCriteria),
		Dependencies:       item.Dependencies,
	})
	if err == nil {
		item.Title = details.Title
		item.Description = details.Description
		item.Priority = details.Priority
		item.Owner = details.Owner
		item.OwnerAgentID = details.OwnerAgentID
		item.AcceptanceCriteria = details.AcceptanceCriteria
		item.Dependencies = details.Dependencies
	}
	item.BlockedReason = strings.TrimSpace(redact.String(item.BlockedReason))
	return item
}

func redactStrings(values []string) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = redact.String(value)
	}
	return out
}

func workItemDetails(item domain.WorkItem) domain.WorkItemDetails {
	return domain.WorkItemDetails{
		Title: item.Title, Description: item.Description, Priority: item.Priority, Owner: item.Owner,
		OwnerAgentID:       item.OwnerAgentID,
		AcceptanceCriteria: item.AcceptanceCriteria, Dependencies: item.Dependencies,
	}
}

func sameWorkItemDetails(left domain.WorkItem, right domain.WorkItem) bool {
	return left.Title == right.Title && left.Description == right.Description && left.Priority == right.Priority &&
		left.Owner == right.Owner && left.OwnerAgentID == right.OwnerAgentID &&
		slices.Equal(left.AcceptanceCriteria, right.AcceptanceCriteria) &&
		slices.Equal(left.Dependencies, right.Dependencies)
}

func equalNullableTime(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
