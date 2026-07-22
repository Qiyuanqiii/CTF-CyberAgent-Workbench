package browserruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileOwnershipPlanDerivesExactPathWithoutFilesystemMutation(t *testing.T) {
	session, executable, ownership, runtimeRoot := profileLifecycleFixture(t)
	if _, err := os.Stat(runtimeRoot); !os.IsNotExist(err) {
		t.Fatalf("ownership planning unexpectedly created its root: %v", err)
	}
	if err := ValidateProfileOwnershipPlan(ownership, session, executable); err != nil {
		t.Fatal(err)
	}
	if ownership.DirectoryName != "profile-"+session.ProfileToken ||
		ownership.DirectoryPath != filepath.Join(runtimeRoot, ownership.DirectoryName) ||
		ownership.OwnerMarkerName != ProfileOwnerMarkerName || ownership.Generation != 1 ||
		!ownership.Disposable || !ownership.ExactPathOnly ||
		!ownership.CollisionCheckRequired || !ownership.RestartRecoveryRequired ||
		!ownership.CleanupRequired || ownership.PersonalProfileAllowed ||
		ownership.ModelOwnsCleanup || !ownership.ApplyBlocked ||
		ownership.Authority != (DirectoryAuthority{}) {
		t.Fatalf("unsafe or incomplete profile ownership plan: %#v", ownership)
	}
	if _, err := os.Stat(ownership.DirectoryPath); !os.IsNotExist(err) {
		t.Fatalf("ownership planning unexpectedly created its directory: %v", err)
	}
}

func TestProfileReconciliationClassifiesCollisionRecoveryAndRefusal(t *testing.T) {
	_, _, ownership, _ := profileLifecycleFixture(t)
	foreign := strings.Repeat("f", 64)
	cases := []struct {
		name       string
		state      ProfileDirectoryState
		owner      string
		generation uint64
		marker     string
		decision   ProfileReconciliationDecision
		check      func(ProfileReconciliationPlan) bool
	}{
		{"absent", ProfileDirectoryAbsent, "", 0, "", ProfileDecisionCreateCandidate,
			func(value ProfileReconciliationPlan) bool { return value.CreateCandidate && !value.CollisionDetected }},
		{"active", ProfileDirectoryOwnedActive, ownership.OwnerToken, ownership.Generation,
			ownership.MarkerPayloadSHA256, ProfileDecisionWaitForOwner,
			func(value ProfileReconciliationPlan) bool {
				return value.CollisionDetected && value.ExactOwnerVerified &&
					value.ProcessQuiescenceRequired
			}},
		{"stale", ProfileDirectoryOwnedStale, ownership.OwnerToken, ownership.Generation,
			ownership.MarkerPayloadSHA256, ProfileDecisionRecoverCandidate,
			func(value ProfileReconciliationPlan) bool {
				return value.RestartRecoveryCandidate && !value.CleanupCandidate
			}},
		{"released", ProfileDirectoryOwnedReleased, ownership.OwnerToken, ownership.Generation,
			ownership.MarkerPayloadSHA256, ProfileDecisionCleanupCandidate,
			func(value ProfileReconciliationPlan) bool { return value.CleanupCandidate }},
		{"foreign", ProfileDirectoryForeign, foreign, 9, foreign, ProfileDecisionRefuseForeign,
			func(value ProfileReconciliationPlan) bool {
				return value.ForeignDirectoryRefused && !value.CleanupCandidate && !value.CreateCandidate
			}},
		{"corrupt", ProfileDirectoryCorrupt, "", 0, "", ProfileDecisionRefuseCorrupt,
			func(value ProfileReconciliationPlan) bool {
				return value.CorruptMarkerRefused && !value.CleanupCandidate && !value.CreateCandidate
			}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			observation, err := BuildProfileDirectoryObservation(ownership, testCase.state,
				testCase.owner, testCase.generation, testCase.marker)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := BuildProfileReconciliationPlan(ownership, observation)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Decision != testCase.decision || !testCase.check(plan) ||
				!plan.FilesystemRecheckRequired || !plan.ApplyBlocked ||
				plan.Authority != (DirectoryAuthority{}) {
				t.Fatalf("unexpected reconciliation plan: %#v", plan)
			}
			if err := ValidateProfileReconciliationPlan(plan, ownership, observation); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProfileCleanupPlanRequiresExactReleasedOwnerAndRemainsBlocked(t *testing.T) {
	_, _, ownership, runtimeRoot := profileLifecycleFixture(t)
	released, err := BuildProfileDirectoryObservation(ownership, ProfileDirectoryOwnedReleased,
		ownership.OwnerToken, ownership.Generation, ownership.MarkerPayloadSHA256)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := BuildProfileCleanupPlan(ownership, released)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProfileCleanupPlan(cleanup, ownership, released); err != nil {
		t.Fatal(err)
	}
	if cleanup.DirectoryPath != ownership.DirectoryPath || !cleanup.ExactOwnerRequired ||
		!cleanup.ReleasedStateRequired || !cleanup.ProcessQuiescenceRequired ||
		!cleanup.FilesystemRecheckRequired || cleanup.RecursiveWildcardAllowed ||
		cleanup.ModelOwnsCleanup || !cleanup.DeleteBlocked ||
		cleanup.Authority != (DirectoryAuthority{}) {
		t.Fatalf("unsafe cleanup plan: %#v", cleanup)
	}
	if _, err := os.Stat(runtimeRoot); !os.IsNotExist(err) {
		t.Fatalf("cleanup planning unexpectedly touched the filesystem: %v", err)
	}

	for _, state := range []ProfileDirectoryState{
		ProfileDirectoryOwnedActive, ProfileDirectoryOwnedStale,
	} {
		observation, buildErr := BuildProfileDirectoryObservation(ownership, state,
			ownership.OwnerToken, ownership.Generation, ownership.MarkerPayloadSHA256)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		if _, buildErr = BuildProfileCleanupPlan(ownership, observation); buildErr == nil {
			t.Fatalf("state %q unexpectedly produced a cleanup candidate", state)
		}
	}
	foreign, err := BuildProfileDirectoryObservation(ownership, ProfileDirectoryForeign,
		strings.Repeat("e", 64), 2, strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildProfileCleanupPlan(ownership, foreign); err == nil {
		t.Fatal("foreign browser directory unexpectedly produced a cleanup candidate")
	}
}

func TestProfileRestartRecoveryAdvancesGenerationAndFencesOldOwner(t *testing.T) {
	session, executable, ownership, runtimeRoot := profileLifecycleFixture(t)
	stale, err := BuildProfileDirectoryObservation(ownership, ProfileDirectoryOwnedStale,
		ownership.OwnerToken, ownership.Generation, ownership.MarkerPayloadSHA256)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := BuildRecoveredProfileOwnershipPlan(ownership, stale, session, executable)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Generation != ownership.Generation+1 ||
		recovered.PreviousOwnershipFingerprint != ownership.Fingerprint ||
		recovered.RecoveryObservationFingerprint != stale.Fingerprint ||
		recovered.DirectoryPath != ownership.DirectoryPath ||
		recovered.OwnerToken == ownership.OwnerToken ||
		recovered.MarkerPayloadSHA256 == ownership.MarkerPayloadSHA256 ||
		recovered.Authority != (DirectoryAuthority{}) || !recovered.ApplyBlocked {
		t.Fatalf("unsafe or incomplete recovered ownership: %#v", recovered)
	}
	if err := ValidateProfileOwnershipPlan(recovered, session, executable); err != nil {
		t.Fatal(err)
	}
	oldOwnerAtRecoveredPath, err := BuildProfileDirectoryObservation(recovered,
		ProfileDirectoryOwnedActive, ownership.OwnerToken, ownership.Generation,
		ownership.MarkerPayloadSHA256)
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := BuildProfileReconciliationPlan(recovered, oldOwnerAtRecoveredPath)
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.Decision != ProfileDecisionRefuseForeign ||
		!reconciliation.ForeignDirectoryRefused || reconciliation.ExactOwnerVerified {
		t.Fatalf("old generation was not fenced: %#v", reconciliation)
	}
	if _, err := os.Stat(runtimeRoot); !os.IsNotExist(err) {
		t.Fatalf("restart recovery planning unexpectedly touched the filesystem: %v", err)
	}
}

func TestProfileLifecycleTamperingFailsClosed(t *testing.T) {
	session, executable, ownership, _ := profileLifecycleFixture(t)
	ownershipMutations := []func(*ProfileOwnershipPlan){
		func(value *ProfileOwnershipPlan) { value.DirectoryPath = filepath.Dir(value.DirectoryPath) },
		func(value *ProfileOwnershipPlan) { value.Generation++ },
		func(value *ProfileOwnershipPlan) { value.ModelOwnsCleanup = true },
		func(value *ProfileOwnershipPlan) { value.Authority.Delete = true },
		func(value *ProfileOwnershipPlan) { value.Fingerprint = strings.Repeat("a", 64) },
	}
	for index, mutate := range ownershipMutations {
		candidate := ownership
		mutate(&candidate)
		if err := ValidateProfileOwnershipPlan(candidate, session, executable); err == nil {
			t.Fatalf("ownership mutation %d unexpectedly passed", index)
		}
	}

	released, err := BuildProfileDirectoryObservation(ownership, ProfileDirectoryOwnedReleased,
		ownership.OwnerToken, ownership.Generation, ownership.MarkerPayloadSHA256)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := BuildProfileCleanupPlan(ownership, released)
	if err != nil {
		t.Fatal(err)
	}
	cleanup.DeleteBlocked = false
	if err := ValidateProfileCleanupPlan(cleanup, ownership, released); err == nil {
		t.Fatal("unblocked cleanup mutation unexpectedly passed")
	}

	observation := released
	observation.FilesystemMutationOccurred = true
	if err := ValidateProfileDirectoryObservation(observation, ownership); err == nil {
		t.Fatal("mutating observation unexpectedly passed")
	}
}

func TestProfileOwnershipRejectsNonDedicatedRoot(t *testing.T) {
	session, executable, _, _ := profileLifecycleFixture(t)
	for _, root := range []string{t.TempDir(), filepath.Join(t.TempDir(), ".."), "relative"} {
		if _, err := BuildProfileOwnershipPlan(session, executable, root); err == nil {
			t.Fatalf("non-dedicated profile root %q unexpectedly passed", root)
		}
	}
}

func profileLifecycleFixture(t *testing.T) (SessionPlan, BrowserExecutableIdentity,
	ProfileOwnershipPlan, string,
) {
	t.Helper()
	installRoot := t.TempDir()
	spec := knownSpec(t, DiscoveryRootProgramFiles, BrowserProductEdge, BrowserChannelStable)
	executablePath := filepath.Join(append([]string{installRoot}, spec.Components...)...)
	if err := os.MkdirAll(filepath.Dir(executablePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executablePath, minimalPEImage(t, "amd64"), 0o600); err != nil {
		t.Fatal(err)
	}
	identities, err := discoverBrowserExecutables([]DiscoveryRoot{
		{ID: DiscoveryRootProgramFiles, Path: installRoot},
	}, []browserExecutableSpec{spec}, browserExecutableVersion)
	if err != nil || len(identities) != 1 {
		t.Fatalf("build executable identity: count=%d err=%v", len(identities), err)
	}
	session, err := BuildSessionPlan(NewSessionPlanRequest{
		SessionID: "browser-profile-session", RunID: "browser-profile-run",
		WorkspaceID: "browser-profile-workspace", ProfileID: ProfileSafeWeb,
		Targets: []string{"https://example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(t.TempDir(), ProfileRuntimeRootName)
	ownership, err := BuildProfileOwnershipPlan(session, identities[0], runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	return session, identities[0], ownership, runtimeRoot
}
