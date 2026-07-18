package httpapi

import (
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/operatoraction"
)

func TestOperatorActionCenterExposesOpaqueReadOnlyNavigation(t *testing.T) {
	fixture := newAPIFixture(t)
	privateContent := "PRIVATE steering content must not cross HTTP"
	privateOperationKey := "PRIVATE-operator-action-key-0001"
	if _, err := fixture.store.EnqueueOperatorSteering(t.Context(),
		domain.EnqueueOperatorSteeringRequest{RunID: fixture.run.ID,
			SessionID: fixture.run.SessionID, Content: privateContent,
			OperationKey: privateOperationKey, RequestedBy: "PRIVATE-operator"}); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/runs/" + fixture.run.ID + "/operator-actions"
	response := fixture.get(t, path)
	var center OperatorActionCenterView
	decodeData(t, response, &center)
	if center.ProtocolVersion != operatoraction.ProtocolVersion ||
		center.RunID != fixture.run.ID || center.GeneratedAt.IsZero() || len(center.Items) == 0 {
		t.Fatalf("unexpected operator action center: %#v", center)
	}
	found := false
	for _, item := range center.Items {
		if item.Kind == operatoraction.KindSteeringPending {
			found = item.Destination == operatoraction.DestinationQueue &&
				strings.HasPrefix(item.ID, "action-")
		}
	}
	if !found {
		t.Fatalf("pending steering action was not projected: %#v", center.Items)
	}
	for _, forbidden := range []string{privateContent, privateOperationKey,
		"PRIVATE-operator", "content_sha256", "workspace_id", "session_id"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("operator action center exposed %q: %s", forbidden,
				response.Body.String())
		}
	}
}
