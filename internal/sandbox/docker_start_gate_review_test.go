package sandbox

import (
	"strings"
	"testing"
	"time"
)

func TestDockerStartGateReviewPinsAllBlockersAndUnimplementedLifecycle(t *testing.T) {
	binding := DockerStartGateReviewBinding{
		CleanupIntentID: "cleanup-intent", CleanupResultID: "cleanup-result",
		ApplicationIntentID: "application-intent", ApplicationResultID: "application-result",
		ProjectionID: "projection", ContainerPlanID: "container-plan",
		PreflightID: "preflight", RunID: "run", MissionID: "mission", WorkspaceID: "workspace",
		ManifestFingerprint:      strings.Repeat("1", 64),
		ThreatModelFingerprint:   strings.Repeat("2", 64),
		CleanupResultFingerprint: strings.Repeat("3", 64), MaxLogBytes: 4096,
	}
	review, err := NewDockerStartGateReview("start-gate-review", strings.Repeat("4", 64),
		"operator", binding, true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != DockerStartGateReviewStatusBlocked ||
		review.Decision != DockerStartGateReviewDecisionDeny || review.StartGatePassed ||
		review.RealDaemonChainVerified || review.StartImplementationPresent ||
		review.ContainerStartAuthorized || review.ProcessExecutionAuthorized ||
		review.OutputExportAuthorized || review.ArtifactCommitAuthorized ||
		len(review.Checks) != MaxBackendChecks || review.BlockerCount != MaxBackendChecks ||
		len(review.Lifecycle.Transitions) != DockerStartGateLifecycleTransitionCount ||
		review.Lifecycle.ImplementationPresent || review.Lifecycle.DaemonMutationEnabled ||
		review.Lifecycle.OutputCommitAuthorized {
		t.Fatalf("start-gate review widened authority: %#v", review)
	}
	for _, check := range review.Checks {
		if check.ProductionVerified || check.SufficientForStart || check.BlockerCode == "" ||
			check.FutureGate == "" {
			t.Fatalf("start-gate check was not blocked: %#v", check)
		}
	}
	for _, transition := range review.Lifecycle.Transitions {
		if transition.Implemented || transition.Authorized || !transition.GenerationFenced {
			t.Fatalf("lifecycle transition gained authority: %#v", transition)
		}
	}
	operation, err := NewDockerStartGateReviewOperation(review.OperationKeyDigest, review)
	if err != nil || operation.RequestFingerprint != DockerStartGateReviewRequestFingerprint(review) {
		t.Fatalf("start-gate operation invalid: %#v err=%v", operation, err)
	}
}

func TestDockerStartGateReviewRejectsAuthorityWidening(t *testing.T) {
	binding := DockerStartGateReviewBinding{
		CleanupIntentID: "cleanup-intent", CleanupResultID: "cleanup-result",
		ApplicationIntentID: "application-intent", ApplicationResultID: "application-result",
		ProjectionID: "projection", ContainerPlanID: "container-plan",
		PreflightID: "preflight", RunID: "run", MissionID: "mission", WorkspaceID: "workspace",
		ManifestFingerprint:      strings.Repeat("1", 64),
		ThreatModelFingerprint:   strings.Repeat("2", 64),
		CleanupResultFingerprint: strings.Repeat("3", 64), MaxLogBytes: 4096,
	}
	review, err := NewDockerStartGateReview("start-gate-review", strings.Repeat("4", 64),
		"operator", binding, true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*DockerStartGateReview)
	}{
		{"gate passed", func(value *DockerStartGateReview) { value.StartGatePassed = true }},
		{"production evidence", func(value *DockerStartGateReview) {
			value.Checks[0].ProductionVerified = true
		}},
		{"implemented transition", func(value *DockerStartGateReview) {
			value.Lifecycle.Transitions[1].Implemented = true
		}},
		{"daemon enabled", func(value *DockerStartGateReview) {
			value.Lifecycle.DaemonMutationEnabled = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := review
			changed.Checks = append([]DockerStartGateCheckReview(nil), review.Checks...)
			changed.Lifecycle.Transitions = append([]DockerProcessLifecycleTransition(nil),
				review.Lifecycle.Transitions...)
			test.mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("tampered start-gate review was accepted")
			}
		})
	}
}
