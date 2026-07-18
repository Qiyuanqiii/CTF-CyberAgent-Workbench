package operationreceipt

import "testing"

func TestSettledReceiptsKeepRecoveryAuthorityClosed(t *testing.T) {
	tests := []struct {
		kind           Kind
		cleanupPending bool
		outcome        string
		cleanup        CleanupState
		recovery       RecoveryAction
	}{
		{KindFileEditApply, false, "applied", CleanupComplete, RecoveryNone},
		{KindFileEditApply, true, "applied", CleanupPendingReview, RecoveryRetryAfterGrace},
		{KindRunWakeConsume, false, "completed", CleanupNotApplicable, RecoveryNone},
		{KindSkillPackageInstall, false, "installed", CleanupNotApplicable, RecoveryNone},
	}
	for _, test := range tests {
		receipt := Settled(test.kind, true, test.cleanupPending)
		if err := receipt.Validate(); err != nil {
			t.Fatalf("%s receipt validation: %v", test.kind, err)
		}
		if receipt.Outcome != test.outcome || receipt.CleanupState != test.cleanup ||
			receipt.RecoveryAction != test.recovery || !receipt.Durable ||
			!receipt.RetrySafe || !receipt.Replayed {
			t.Fatalf("%s receipt=%#v", test.kind, receipt)
		}
	}
	invalid := Settled(KindFileEditApply, false, false)
	invalid.Kind = "shell_execute"
	if err := invalid.Validate(); err == nil {
		t.Fatal("unknown receipt kind was accepted")
	}
	failed := FileEditApply("failed", true, false)
	if err := failed.Validate(); err != nil || failed.Outcome != "failed" {
		t.Fatalf("failed receipt=%#v err=%v", failed, err)
	}
	invalid = FileEditApply("invented", false, false)
	if err := invalid.Validate(); err == nil {
		t.Fatal("unknown FileEdit outcome was accepted")
	}
}
