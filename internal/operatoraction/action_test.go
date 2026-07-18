package operatoraction

import (
	"testing"
	"time"
)

func TestActionCenterRejectsDestinationAndWakeAuthorityDrift(t *testing.T) {
	now := time.Now().UTC()
	valid := Item{ID: "action-valid", Kind: KindApprovalPending, State: "pending",
		Destination: DestinationApprovals, AvailableAt: now}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	wrongDestination := valid
	wrongDestination.Destination = DestinationDiffs
	if err := wrongDestination.Validate(); err == nil {
		t.Fatal("mismatched operator action destination was accepted")
	}
	wakeWithoutDue := Item{ID: "action-wake", Kind: KindWakeDue, State: "queued",
		Destination: DestinationWake, AvailableAt: now}
	if err := wakeWithoutDue.Validate(); err == nil {
		t.Fatal("wake action without a due time was accepted")
	}
}
