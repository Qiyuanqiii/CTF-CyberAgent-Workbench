package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/skills"
)

func TestRootSkillContextPreparationIsReplayableImmutableAndAtomicallyCommitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "root-skill-context.db")
	st, turn, selection, assembly := createRootSkillContextTurn(t, path)
	ctx := context.Background()
	request := assembly.Preparation(turn.Agent.ID, turn.Checkpoint.AttemptID,
		turn.Checkpoint.NextTurn)
	prepared, err := st.PrepareRootSkillContext(ctx, turn.Checkpoint, request)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Recovered || prepared.SelectionID != selection.ID || prepared.ItemCount != 1 {
		t.Fatalf("unexpected root Skill context preparation: %#v", prepared)
	}
	replayed, err := st.PrepareRootSkillContext(ctx, turn.Checkpoint, request)
	if err != nil || !replayed.Recovered || replayed.ID != prepared.ID {
		t.Fatalf("root Skill context replay drifted: %#v err=%v", replayed, err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	crossProcessReplay, err := second.PrepareRootSkillContext(ctx, turn.Checkpoint, request)
	if err != nil || !crossProcessReplay.Recovered || crossProcessReplay.ID != prepared.ID {
		t.Fatalf("cross-Store root Skill context replay drifted: %#v err=%v",
			crossProcessReplay, err)
	}
	changed := request
	changed.ContextFingerprint = strings.Repeat("a", 64)
	if changed.ContextFingerprint == request.ContextFingerprint {
		changed.ContextFingerprint = strings.Repeat("b", 64)
	}
	if _, err := st.PrepareRootSkillContext(ctx, turn.Checkpoint, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("reconstructed context drift error = %v", err)
	}

	if _, err := st.db.ExecContext(ctx, `CREATE TRIGGER fail_skill_context_model_start
		BEFORE INSERT ON run_events WHEN NEW.type = 'model.started'
		BEGIN SELECT RAISE(ABORT, 'injected model start failure'); END;`); err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test", Model: "model",
	}
	if _, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt); err == nil {
		t.Fatal("injected model start failure unexpectedly committed")
	}
	assertTableCount(t, st, "root_skill_context_commits", 0)
	if _, err := st.db.ExecContext(ctx, `DROP TRIGGER fail_skill_context_model_start`); err != nil {
		t.Fatal(err)
	}
	inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("model start did not atomically commit Skill context: inserted=%t err=%v", inserted, err)
	}
	inserted, err = st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt)
	if err != nil || inserted {
		t.Fatalf("model start replay drifted: inserted=%t err=%v", inserted, err)
	}
	assertTableCount(t, st, "root_skill_context_preparations", 1)
	assertTableCount(t, st, "root_skill_context_commits", 1)

	for _, mutation := range []string{
		`UPDATE root_skill_context_preparations SET token_budget = token_budget + 1 WHERE id = ?`,
		`DELETE FROM root_skill_context_preparations WHERE id = ?`,
		`UPDATE root_skill_context_commits SET model_attempt = model_attempt + 1 WHERE preparation_id = ?`,
		`DELETE FROM root_skill_context_commits WHERE preparation_id = ?`,
	} {
		if _, err := st.db.ExecContext(ctx, mutation, prepared.ID); err == nil {
			t.Fatalf("immutable root Skill context mutation succeeded: %s", mutation)
		}
	}

	items, err := st.ListRunEvents(ctx, turn.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	preparedIndex, committedIndex, modelIndex := -1, -1, -1
	for index, item := range items {
		switch item.Type {
		case events.SkillContextPreparedEvent:
			preparedIndex = index
			assertRootSkillContextEventIsMetadataOnly(t, item, assembly, false)
		case events.SkillContextCommittedEvent:
			committedIndex = index
			assertRootSkillContextEventIsMetadataOnly(t, item, assembly, true)
		case events.ModelStartedEvent:
			modelIndex = index
		}
	}
	if preparedIndex < 0 || committedIndex <= preparedIndex || modelIndex <= committedIndex {
		t.Fatalf("root Skill context event order drifted: prepared=%d committed=%d model=%d",
			preparedIndex, committedIndex, modelIndex)
	}
	assertRootSkillContextSchemaIsMetadataOnly(t, st)
}

func TestSelectedRunCannotStartModelWithoutPreparedRootSkillContext(t *testing.T) {
	st, turn, _, _ := createRootSkillContextTurn(t,
		filepath.Join(t.TempDir(), "missing-root-skill-context.db"))
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "test", Model: "model",
	}
	if _, err := st.RecordSupervisorModelStarted(context.Background(), turn.Checkpoint, attempt); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("unprepared selected model start error = %v", err)
	}
	assertTableCount(t, st, "root_skill_context_commits", 0)
	eventList, err := st.ListRunEvents(context.Background(), turn.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range eventList {
		if item.Type == events.ModelStartedEvent || item.Type == events.SkillContextCommittedEvent {
			t.Fatalf("unprepared selected Run persisted %s", item.Type)
		}
	}
}

func TestSchemaV39SkillSelectionSurvivesRootContextMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v39-root-skill-context.db")
	st, run := createSkillSelectionRun(t, path, "code")
	ctx := context.Background()
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := application.NewSkillSelectionService(st, registry).Select(ctx,
		application.SelectSkillsRequest{
			RunID: run.ID, Names: []string{"code"}, TokenBudget: 4096,
			OperationKey: "skill-context-v39-upgrade", RequestedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV40ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("downgrade v40 fixture with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v39 did not upgrade: version=%d err=%v", version, err)
	}
	loaded, found, err := upgraded.GetSkillSelectionByRun(ctx, run.ID)
	if err != nil || !found || loaded.Fingerprint != selected.Selection.Fingerprint {
		t.Fatalf("v39 Skill selection was not preserved: found=%t selection=%#v err=%v",
			found, loaded, err)
	}
	assertTableCount(t, upgraded, "root_skill_context_preparations", 0)
	assertTableCount(t, upgraded, "root_skill_context_commits", 0)
}

func createRootSkillContextTurn(t *testing.T, path string) (*SQLiteStore,
	domain.SupervisorTurn, skills.Selection, skills.ContextAssembly,
) {
	t.Helper()
	st, run := createSkillSelectionRun(t, path, "code")
	ctx := context.Background()
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := application.NewSkillSelectionService(st, registry).Select(ctx,
		application.SelectSkillsRequest{
			RunID: run.ID, Names: []string{"code"}, TokenBudget: 4096,
			OperationKey: "root-skill-context-selection", RequestedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewRunService(st).Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx,
		acquireTestRunExecutionLease(t, ctx, st, run.ID), "deliver selected Skill")
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := registry.AssembleContext(selected.Selection)
	if err != nil {
		t.Fatal(err)
	}
	return st, turn, selected.Selection, assembly
}

func assertTableCount(t *testing.T, st *SQLiteStore, table string, want int) {
	t.Helper()
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s count=%d want=%d", table, count, want)
	}
}

func assertRootSkillContextEventIsMetadataOnly(t *testing.T, event events.Event,
	assembly skills.ContextAssembly, committed bool,
) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	wantFields := 9
	if committed {
		wantFields++
	}
	if len(payload) != wantFields || payload["protocol"] != skills.ContextProtocolVersion ||
		payload["item_count"] != float64(assembly.ItemCount) ||
		payload["token_budget"] != float64(assembly.TokenBudget) ||
		payload["token_upper_bound"] != float64(assembly.TokenUpperBound) ||
		payload["redaction_count"] != float64(assembly.RedactionCount) ||
		payload["root_only"] != true || payload["tool_capability_grant"] != false {
		t.Fatalf("unsafe or incomplete root Skill context event: %#v", payload)
	}
	for _, forbidden := range []string{
		"name", "version", "path", "content", "hash", "sha", "selection_id",
		assembly.Items[0].Name, assembly.Items[0].Version, assembly.Items[0].Content,
	} {
		if forbidden != "" && strings.Contains(event.PayloadJSON, forbidden) {
			t.Fatalf("root Skill context event exposed %q: %s", forbidden, event.PayloadJSON)
		}
	}
}

func assertRootSkillContextSchemaIsMetadataOnly(t *testing.T, st *SQLiteStore) {
	t.Helper()
	for _, table := range []string{"root_skill_context_preparations", "root_skill_context_commits"} {
		rows, err := st.db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var ordinal int
			var name, dataType string
			var notNull, primaryKey int
			var defaultValue any
			if err := rows.Scan(&ordinal, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			lower := strings.ToLower(name)
			for _, forbidden := range []string{"content", "path", "skill_name", "skill_version"} {
				if strings.Contains(lower, forbidden) {
					rows.Close()
					t.Fatalf("%s contains forbidden durable column %q", table, name)
				}
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
