package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/verification"
)

func TestVerificationCoverageCursorRoundTripsExactSnapshotKeyset(t *testing.T) {
	path := "/api/v1/runs/run-1/verification-plan-coverage/plan-1/items/1"
	initial, err := parseVerificationCoveragePage(mapValues("limit", "25"), path)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Limit != 25 || initial.Anchor != (application.VerificationCoveragePageAnchor{}) {
		t.Fatalf("initial verification page diverged: %#v", initial)
	}
	detail := application.VerificationPlanItemCoverageDetail{
		AssociationsTruncated:          true,
		SnapshotHighWaterEventSequence: 19,
		NextPageBeforeEventSequence:    15,
		NextPageBeforeAssociationID:    "association-2",
		NextPageConsumed:               2,
	}
	page := verificationCoveragePage(detail, initial)
	if page.Limit != 25 || page.NextCursor == "" || page.Truncated {
		t.Fatalf("verification keyset cursor was not issued: %#v", page)
	}
	continued, err := parseVerificationCoveragePage(
		mapValues("limit", "10", "cursor", page.NextCursor), path)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Limit != 10 || continued.Scope != initial.Scope ||
		continued.Anchor.SnapshotHighWaterEventSequence != 19 ||
		continued.Anchor.BeforeEventSequence != 15 ||
		continued.Anchor.BeforeAssociationID != "association-2" ||
		continued.Anchor.Consumed != 2 {
		t.Fatalf("verification cursor lost its snapshot keyset: %#v", continued)
	}
	if _, err := parseVerificationCoveragePage(
		mapValues("cursor", page.NextCursor), path+"-different"); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("cross-route verification cursor code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestVerificationCoverageCursorRejectsMalformedOrWidenedState(t *testing.T) {
	path := "/api/v1/runs/run-1/verification-plan-coverage/plan-1/items/1"
	scope := pageScope(path, mapValues())
	cases := []string{
		"not-base64!",
		base64.RawURLEncoding.EncodeToString([]byte(
			`{"v":2,"s":"` + scope + `","h":9,"b":7,"i":"association-1","c":1,"x":true}`)),
		base64.RawURLEncoding.EncodeToString([]byte(
			`{"v":2,"s":"` + scope + `","h":9,"b":10,"i":"association-1","c":1}`)),
		base64.RawURLEncoding.EncodeToString([]byte(
			`{"v":2,"s":"` + scope + `","h":9,"b":7,"i":"association-1","c":100000}`)),
		base64.RawURLEncoding.EncodeToString([]byte(
			`{"v":2,"s":"` + scope + `","h":9,"b":7,"i":"association-1","c":1} trailing`)),
		strings.Repeat("a", MaxCursorBytes+1),
	}
	for _, encoded := range cases {
		if _, err := parseVerificationCoveragePage(mapValues("cursor", encoded), path); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
			t.Fatalf("malformed cursor code=%s err=%v cursor=%q",
				apperror.CodeOf(err), err, encoded)
		}
	}
}

func TestVerificationCoveragePageStopsAtBoundedReadWindow(t *testing.T) {
	request := verificationCoveragePageRequest{Limit: 100, Scope: strings.Repeat("a", 32)}
	detail := application.VerificationPlanItemCoverageDetail{
		AssociationsTruncated:          true,
		SnapshotHighWaterEventSequence: 100_100,
		NextPageBeforeEventSequence:    100,
		NextPageBeforeAssociationID:    "association-window-limit",
		NextPageConsumed:               verification.MaxCoveragePageWindow,
	}
	page := verificationCoveragePage(detail, request)
	if page.NextCursor != "" || !page.Truncated || page.Limit != request.Limit {
		t.Fatalf("verification page widened its read window: %#v", page)
	}
}

func mapValues(items ...string) map[string][]string {
	values := make(map[string][]string, len(items)/2)
	for index := 0; index+1 < len(items); index += 2 {
		values[items[index]] = []string{items[index+1]}
	}
	return values
}
