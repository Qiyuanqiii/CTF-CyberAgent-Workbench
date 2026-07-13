package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/skills"
)

func TestSkillSelectionIsImmutableIdempotentAndMetadataOnly(t *testing.T) {
	st, run := createSkillSelectionRun(t, filepath.Join(t.TempDir(), "skill-selection.db"), "code")
	ctx := context.Background()
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewSkillSelectionService(st, registry)
	request := application.SelectSkillsRequest{
		RunID: run.ID, Names: []string{"code"}, TokenBudget: 4096,
		OperationKey: "skill-selection-store-0001", RequestedBy: "operator",
	}
	created, err := service.Select(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Replayed || created.Selection.ItemCount != 1 ||
		created.Selection.Items[0].Name != "code" ||
		created.Selection.TokenUpperBound != 398 {
		t.Fatalf("created selection drifted: %#v", created)
	}
	loaded, found, err := st.GetSkillSelectionByRun(ctx, run.ID)
	if err != nil || !found || loaded.Fingerprint != created.Selection.Fingerprint {
		t.Fatalf("stored selection drifted: found=%t value=%#v err=%v", found, loaded, err)
	}
	replayed, err := service.Select(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Selection.ID != created.Selection.ID {
		t.Fatalf("selection replay failed: %#v err=%v", replayed, err)
	}
	changed := request
	changed.TokenBudget = 2048
	if _, err := service.Select(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed intent error = %v", err)
	}
	secondKey := request
	secondKey.OperationKey = "skill-selection-store-0002"
	if _, err := service.Select(ctx, secondKey); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("second immutable selection error = %v", err)
	}

	for _, mutation := range []string{
		`UPDATE run_skill_selections SET token_budget = token_budget + 1 WHERE id = ?`,
		`DELETE FROM run_skill_selection_items WHERE selection_id = ?`,
		`DELETE FROM run_skill_selection_operations WHERE selection_id = ?`,
		`DELETE FROM run_skill_selections WHERE id = ?`,
	} {
		if _, err := st.db.ExecContext(ctx, mutation, created.Selection.ID); err == nil {
			t.Fatalf("immutable Skill selection mutation succeeded: %s", mutation)
		}
	}
	eventList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	selectionEvents := 0
	for _, event := range eventList {
		if event.Type != events.SkillSelectionCreatedEvent {
			continue
		}
		selectionEvents++
		for _, forbidden := range []string{"content_sha256", "selection_fingerprint", "SKILL.md", "tool_dependencies", request.OperationKey} {
			if strings.Contains(event.PayloadJSON, forbidden) {
				t.Fatalf("Skill selection event leaked %q: %s", forbidden, event.PayloadJSON)
			}
		}
		if !strings.Contains(event.PayloadJSON, `"context_injection":false`) ||
			!strings.Contains(event.PayloadJSON, `"tool_capability_grant":false`) {
			t.Fatalf("Skill selection event omitted closed boundaries: %s", event.PayloadJSON)
		}
	}
	if selectionEvents != 1 {
		t.Fatalf("Skill selection event count = %d", selectionEvents)
	}
	var rawKeyMatches int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_skill_selection_operations
		WHERE operation_key_digest = ? OR request_fingerprint = ?`, request.OperationKey,
		request.OperationKey).Scan(&rawKeyMatches); err != nil || rawKeyMatches != 0 {
		t.Fatalf("raw operation key persisted: matches=%d err=%v", rawKeyMatches, err)
	}
}

func TestSkillSelectionConvergesAcrossStoresAndReplaysAfterStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-selection-concurrent.db")
	first, run := createSkillSelectionRun(t, path, "code")
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	services := []*application.SkillSelectionService{
		application.NewSkillSelectionService(first, registry),
		application.NewSkillSelectionService(second, registry),
	}
	request := application.SelectSkillsRequest{
		RunID: run.ID, Names: []string{"code"}, TokenBudget: 4096,
		OperationKey: "skill-selection-concurrent-0001", RequestedBy: "operator",
	}
	type result struct {
		value application.SelectSkillsResult
		err   error
	}
	results := make(chan result, 8)
	var wait sync.WaitGroup
	for index := range 8 {
		wait.Add(1)
		go func(service *application.SkillSelectionService) {
			defer wait.Done()
			value, err := service.Select(context.Background(), request)
			results <- result{value: value, err: err}
		}(services[index%len(services)])
	}
	wait.Wait()
	close(results)
	ids := map[string]bool{}
	for current := range results {
		if current.err != nil {
			t.Fatal(current.err)
		}
		ids[current.value.Selection.ID] = true
	}
	if len(ids) != 1 {
		t.Fatalf("concurrent Skill selection IDs = %#v", ids)
	}
	for table, want := range map[string]int{
		"run_skill_selections": 1, "run_skill_selection_items": 1,
		"run_skill_selection_operations": 1,
	} {
		var count int
		if err := first.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d want=%d err=%v", table, count, want, err)
		}
	}
	if _, err := application.NewRunService(first).Start(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	replayed, err := services[0].Select(context.Background(), request)
	if err != nil || !replayed.Replayed {
		t.Fatalf("post-start replay failed: %#v err=%v", replayed, err)
	}
	request.OperationKey = "skill-selection-concurrent-0002"
	if _, err := services[0].Select(context.Background(), request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("post-start new selection error = %v", err)
	}
}

func TestSkillSelectionEventFailureRollsBackAndV38UpgradesCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-selection-migration.db")
	st, run := createSkillSelectionRun(t, path, "code")
	ctx := context.Background()
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveSelection(skills.ResolveSelectionRequest{
		SelectionID: idgen.New("skill-selection"), RunID: run.ID, MissionID: run.MissionID,
		Profile: domain.ProfileCode, Names: []string{"code"}, TokenBudget: 4096,
		RequestedBy: "operator", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation := skills.SelectionOperation{
		KeyDigest:          runmutation.Fingerprint("skill_selection_operation.v1", run.ID, "rollback-operation-key"),
		RequestFingerprint: skills.SelectionRequestFingerprint(selection), SelectionID: selection.ID,
		RunID: run.ID, RequestedBy: selection.RequestedBy, CreatedAt: selection.CreatedAt,
	}
	event, err := events.New(run.ID, run.MissionID, events.SkillSelectionCreatedEvent,
		"skills", selection.ID, map[string]any{
			"protocol": selection.ProtocolVersion, "profile": selection.Profile,
			"item_count": selection.ItemCount, "token_budget": selection.TokenBudget,
			"token_upper_bound": selection.TokenUpperBound,
			"context_injection": false, "tool_capability_grant": false,
		})
	if err != nil {
		t.Fatal(err)
	}
	event.CreatedAt = selection.CreatedAt
	duplicateFieldEvent := event
	duplicateFieldEvent.PayloadJSON = strings.TrimSuffix(event.PayloadJSON, "}") +
		`,"context_injection":false}`
	if err := validateSkillSelectionEvent(duplicateFieldEvent, selection); err == nil ||
		!strings.Contains(err.Error(), "duplicate field") {
		t.Fatalf("duplicate Skill selection event validation error = %v", err)
	}
	if _, _, err := st.CreateSkillSelection(ctx, selection, operation,
		duplicateFieldEvent); err == nil {
		t.Fatal("duplicate Skill selection event field was persisted")
	}
	if _, found, err := st.GetSkillSelectionByRun(ctx, run.ID); err != nil || found {
		t.Fatalf("ambiguous event left Skill selection: found=%t err=%v", found, err)
	}
	existingEvents, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || len(existingEvents) == 0 {
		t.Fatalf("initial Run event is missing: %v", err)
	}
	event.EventID = existingEvents[0].EventID
	if _, _, err := st.CreateSkillSelection(ctx, selection, operation, event); err == nil {
		t.Fatal("duplicate event id unexpectedly committed Skill selection")
	}
	if _, found, err := st.GetSkillSelectionByRun(ctx, run.ID); err != nil || found {
		t.Fatalf("failed transaction left Skill selection: found=%t err=%v", found, err)
	}

	for _, statement := range removeSchemaV39ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v38 did not upgrade: version=%d err=%v", version, err)
	}
	if _, err := upgraded.GetRun(ctx, run.ID); err != nil {
		t.Fatalf("v38 Run was not preserved: %v", err)
	}
	if _, found, err := upgraded.GetSkillSelectionByRun(ctx, run.ID); err != nil || found {
		t.Fatalf("migration synthesized a Skill selection: found=%t err=%v", found, err)
	}
}

func createSkillSelectionRun(t *testing.T, path string, profile string) (*SQLiteStore, domain.Run) {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, run, err := application.NewRunService(st).Create(context.Background(), application.CreateRunRequest{
		Goal: "Skill selection fixture", Profile: profile,
		Budget: domain.Budget{MaxTurns: 8, MaxTokens: 8192},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, run
}
