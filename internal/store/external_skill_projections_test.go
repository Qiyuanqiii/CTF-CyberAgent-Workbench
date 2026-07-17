package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/skills"
)

func TestExternalSkillProjectionIsBoundedReadOnlyAndMetadataOnly(t *testing.T) {
	ctx := context.Background()
	st, run, installation, selected := createExternalSkillProjectionFixture(t,
		filepath.Join(t.TempDir(), "external-skill-projection.db"))
	defer st.Close()

	projection, found, err := st.GetExternalSkillProjectionByRun(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("load external Skill projection: found=%t value=%#v err=%v",
			found, projection, err)
	}
	if projection.ProtocolVersion != skills.ExternalSkillProjectionProtocolVersion ||
		projection.RunID != run.ID || projection.Surface != domain.ExecutionSurfaceCode ||
		projection.Profile != domain.ProfileReview || projection.ModeRevision != 1 ||
		projection.ItemCount != 1 || len(projection.Items) != 1 ||
		projection.Items[0].Name != installation.Name ||
		projection.Items[0].Version != installation.Version ||
		projection.Items[0].DeclaredToolCount != 2 ||
		!projection.Items[0].SpecialistEligible ||
		projection.RootPreparedCount != 0 || projection.RootCommittedCount != 0 ||
		projection.SpecialistPreparedCount != 0 || projection.SpecialistCommittedCount != 0 ||
		!projection.OperatorConfirmed || !projection.ContextDeliveryAuthorized ||
		projection.ToolCapabilityGrant {
		t.Fatalf("external Skill projection drifted: %#v", projection)
	}

	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		selected.Selection.ID, selected.Selection.MissionID,
		selected.Selection.ModeSnapshotID, selected.Selection.Fingerprint,
		selected.Selection.RequestedBy, installation.ID,
		installation.InstallationFingerprint, installation.RequestFingerprint,
		installation.ArchiveSHA256, installation.Manifest.ContentSHA256,
	} {
		if forbidden != "" && strings.Contains(string(raw), forbidden) {
			t.Fatalf("external Skill projection leaked private value %q: %s", forbidden, raw)
		}
	}
	assertExternalSkillProjectionViewsOmitPrivateColumns(t, st)

	for _, mutation := range []string{
		`UPDATE run_external_skill_projections SET token_budget = 1 WHERE run_id = ?`,
		`DELETE FROM run_external_skill_projection_items WHERE run_id = ?`,
		`INSERT INTO run_external_skill_projections (run_id) VALUES (?)`,
	} {
		if _, err := st.db.ExecContext(ctx, mutation, run.ID); err == nil {
			t.Fatalf("read-only external Skill projection mutation succeeded: %s", mutation)
		}
	}

	clone := skills.CloneExternalSkillProjection(projection)
	clone.Items[0].Name = "changed"
	if projection.Items[0].Name == clone.Items[0].Name {
		t.Fatal("external Skill projection clone aliases its item slice")
	}
	if _, _, err := st.GetExternalSkillProjectionByRun(ctx, "bad\x00id"); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("invalid projection Run id error = %v", err)
	}
}

func TestSchemaV70ExternalSelectionUpgradesToProjectionWithoutFabricatingFacts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "external-skill-projection-upgrade.db")
	st, run, _, _ := createExternalSkillProjectionFixture(t, path)
	_, runWithoutSelection, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{
			Goal: "projection migration must not invent a selection", Profile: "review",
			Budget: domain.Budget{MaxTurns: 2, MaxTokens: 1024},
		})
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var selectionCount int
	if err := st.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_external_skill_selections`).Scan(&selectionCount); err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV71ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("remove schema v71 with %q: %v", statement, err)
		}
	}
	if version, err := st.SchemaVersion(ctx); err != nil || version != 70 {
		t.Fatalf("v70 fixture version=%d err=%v", version, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	projection, found, err := upgraded.GetExternalSkillProjectionByRun(ctx, run.ID)
	if err != nil || !found || projection.ItemCount != 1 {
		t.Fatalf("v70 selection projection was not restored: found=%t value=%#v err=%v",
			found, projection, err)
	}
	if _, found, err := upgraded.GetExternalSkillProjectionByRun(ctx,
		runWithoutSelection.ID); err != nil || found {
		t.Fatalf("v71 migration fabricated a projection: found=%t err=%v", found, err)
	}
	afterEvents, err := upgraded.ListRunEvents(ctx, run.ID)
	if err != nil || len(afterEvents) != len(beforeEvents) {
		t.Fatalf("v71 migration changed Run events: before=%d after=%d err=%v",
			len(beforeEvents), len(afterEvents), err)
	}
	var afterSelectionCount int
	if err := upgraded.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_external_skill_selections`).Scan(&afterSelectionCount); err != nil ||
		afterSelectionCount != selectionCount {
		t.Fatalf("v71 migration changed selections: before=%d after=%d err=%v",
			selectionCount, afterSelectionCount, err)
	}
}

func createExternalSkillProjectionFixture(t *testing.T, path string) (*SQLiteStore,
	domain.Run, skills.PackageInstallation, application.SelectExternalSkillsResult,
) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	installation, operation, result := fixturePackageInstallation(t,
		"projection-review", "1.0.0", "projection-install-operation",
		time.Now().UTC().Add(-time.Minute))
	if _, _, _, err := st.PreparePackageInstallation(ctx, installation, operation); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if _, _, err := st.CompletePackageInstallation(ctx, result); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{
			Goal: "inspect safe external Skill provenance", Profile: "review",
			Budget: domain.Budget{MaxTurns: 4, MaxTokens: 4096},
		})
	if err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	selected, err := application.NewExternalSkillSelectionService(st).Select(ctx,
		application.SelectExternalSkillsRequest{
			RunID: run.ID, PackageRefs: []string{"projection-review@1.0.0"},
			SpecialistRef: "projection-review@1.0.0", TokenBudget: 1024,
			OperationKey: "projection-selection-operation",
			RequestedBy:  "private-projection-operator", ConfirmUntrustedContext: true,
		})
	if err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	return st, run, installation, selected
}

func assertExternalSkillProjectionViewsOmitPrivateColumns(t *testing.T, st *SQLiteStore) {
	t.Helper()
	for _, view := range []string{
		"run_external_skill_projections", "run_external_skill_projection_items",
	} {
		rows, err := st.db.Query(`PRAGMA table_info(` + view + `)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var ordinal, notNull, primaryKey int
			var name, dataType string
			var defaultValue any
			if err := rows.Scan(&ordinal, &name, &dataType, &notNull, &defaultValue,
				&primaryKey); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			lower := strings.ToLower(name)
			for _, forbidden := range []string{
				"selection_id", "installation", "fingerprint", "sha", "digest",
				"object", "path", "content", "requested", "operation", "agent_id",
				"attempt_id", "mission_id", "mode_snapshot_id",
			} {
				if strings.Contains(lower, forbidden) {
					_ = rows.Close()
					t.Fatalf("projection view %s exposes private column %q", view, name)
				}
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
