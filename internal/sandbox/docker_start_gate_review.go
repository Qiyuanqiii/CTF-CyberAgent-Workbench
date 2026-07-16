package sandbox

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	DockerStartGateReviewProtocolVersion    = "sandbox_docker_start_gate_review.v1"
	DockerStartGateReviewOperationVersion   = "sandbox_docker_start_gate_review_operation.v1"
	DockerProcessLifecycleProtocolVersion   = "sandbox_docker_process_lifecycle_blueprint.v1"
	DockerStartGateReviewStatusBlocked      = "blocked"
	DockerStartGateReviewDecisionDeny       = "deny_start"
	DockerStartGateReviewTrustDesignOnly    = "design_review_only"
	DockerStartGateReviewedThroughSchema    = 62
	DockerStartGateRequiredCheckCount       = MaxBackendChecks
	DockerStartGateProductionVerifiedChecks = 0
	DockerStartGateLifecycleTransitionCount = 11

	DockerStartGateEvidenceNeverStarted = "never_started_daemon_evidence"
	DockerStartGateEvidenceCompiled     = "compiled_only"
	DockerStartGateEvidenceRequirement  = "requirement_only"
	DockerStartGateEvidenceSimulation   = "simulation_only"

	DockerStartGateFutureRuntime     = "runtime_isolation_gate"
	DockerStartGateFutureNetwork     = "network_secret_gate"
	DockerStartGateFutureTermination = "termination_recovery_gate"
	DockerStartGateFutureOutput      = "output_export_gate"

	DockerProcessOwnershipModel = "per_run_generation_fenced_single_owner"
)

type dockerStartGateCheckSpec struct {
	EvidenceClass  string
	EvidenceSource string
	BlockerCode    string
	FutureGate     string
}

var dockerStartGateCheckSpecs = [...]dockerStartGateCheckSpec{
	{DockerStartGateEvidenceNeverStarted, "v57_v59_descriptor_and_handoff", "running_host_path_isolation_unverified", DockerStartGateFutureRuntime},
	{DockerStartGateEvidenceNeverStarted, "v55_v56_stopped_configuration", "running_mount_propagation_unverified", DockerStartGateFutureRuntime},
	{DockerStartGateEvidenceNeverStarted, "v55_v56_stopped_configuration", "running_rootfs_readonly_unverified", DockerStartGateFutureRuntime},
	{DockerStartGateEvidenceNeverStarted, "v61_readonly_nocopy_mounts", "running_input_readonly_unverified", DockerStartGateFutureRuntime},
	{DockerStartGateEvidenceCompiled, "v54_compiled_output_plan", "writable_output_isolation_unimplemented", DockerStartGateFutureOutput},
	{DockerStartGateEvidenceNeverStarted, "v55_v56_stopped_network_none", "running_network_deny_unverified", DockerStartGateFutureRuntime},
	{DockerStartGateEvidenceCompiled, "v54_empty_allowlist_compiled", "network_allowlist_enforcement_unimplemented", DockerStartGateFutureNetwork},
	{DockerStartGateEvidenceCompiled, "v54_zero_secret_profile", "ephemeral_secret_materialization_unimplemented", DockerStartGateFutureNetwork},
	{DockerStartGateEvidenceNeverStarted, "v55_v56_stopped_configuration", "running_nonroot_identity_unverified", DockerStartGateFutureRuntime},
	{DockerStartGateEvidenceNeverStarted, "v55_v56_stopped_configuration", "running_resource_limits_unverified", DockerStartGateFutureRuntime},
	{DockerStartGateEvidenceRequirement, "v51_requirement_only", "wall_clock_supervision_unimplemented", DockerStartGateFutureTermination},
	{DockerStartGateEvidenceRequirement, "v51_requirement_only", "term_kill_escalation_unimplemented", DockerStartGateFutureTermination},
	{DockerStartGateEvidenceNeverStarted, "v50_v56_v62_never_started_cleanup", "running_orphan_reconciliation_unimplemented", DockerStartGateFutureTermination},
	{DockerStartGateEvidenceSimulation, "v52_simulation_only", "output_regular_file_validation_unimplemented", DockerStartGateFutureOutput},
	{DockerStartGateEvidenceSimulation, "v52_simulation_only", "output_link_special_rejection_unimplemented", DockerStartGateFutureOutput},
	{DockerStartGateEvidenceSimulation, "v52_simulation_only", "atomic_artifact_commit_unimplemented", DockerStartGateFutureOutput},
}

type DockerStartGateCheckReview struct {
	Ordinal            int
	Name               string
	EvidenceClass      string
	EvidenceSource     string
	ProductionVerified bool
	SufficientForStart bool
	BlockerCode        string
	FutureGate         string
	ReviewFingerprint  string
}

func (item DockerStartGateCheckReview) Validate(authorityFingerprint string) error {
	checks := RequiredBackendChecks()
	if item.Ordinal < 1 || item.Ordinal > len(checks) ||
		item.Name != checks[item.Ordinal-1].Name || !validDigest(authorityFingerprint) {
		return errors.New("docker start-gate check identity is invalid")
	}
	spec := dockerStartGateCheckSpecs[item.Ordinal-1]
	if item.EvidenceClass != spec.EvidenceClass || item.EvidenceSource != spec.EvidenceSource ||
		item.ProductionVerified || item.SufficientForStart || item.BlockerCode != spec.BlockerCode ||
		item.FutureGate != spec.FutureGate || item.ReviewFingerprint != fingerprint(
		DockerStartGateReviewProtocolVersion, authorityFingerprint, strconv.Itoa(item.Ordinal),
		item.Name, item.EvidenceClass, item.EvidenceSource,
		strconv.FormatBool(item.ProductionVerified), strconv.FormatBool(item.SufficientForStart),
		item.BlockerCode, item.FutureGate) {
		return errors.New("docker start-gate check must remain a fixed unresolved blocker")
	}
	return nil
}

type dockerLifecycleTransitionSpec struct {
	FromState          string
	ToState            string
	Action             string
	WriteAheadRequired bool
	DaemonMutation     bool
	CancellationFanout bool
}

var dockerLifecycleTransitionSpecs = [...]dockerLifecycleTransitionSpec{
	{"absent", "start_intent_committed", "persist_start_intent", true, false, false},
	{"start_intent_committed", "start_submitted", "fixed_endpoint_start", true, true, false},
	{"start_submitted", "running", "inspect_owned_running", false, false, false},
	{"start_submitted", "orphaned", "reconcile_uncertain_start", false, false, false},
	{"running", "term_requested", "cancellation_fanout_term", true, true, true},
	{"term_requested", "exited", "wait_graceful_exit", false, false, false},
	{"term_requested", "kill_requested", "bounded_grace_expired", true, true, true},
	{"kill_requested", "exited", "wait_forced_exit", false, false, false},
	{"running", "exited", "wait_natural_exit", false, false, false},
	{"running", "orphaned", "lease_loss_marks_orphan", true, false, true},
	{"orphaned", "reconciled", "generation_fenced_reconcile", true, true, true},
}

type DockerProcessLifecycleTransition struct {
	Ordinal               int
	FromState             string
	ToState               string
	Action                string
	WriteAheadRequired    bool
	GenerationFenced      bool
	DaemonMutation        bool
	CancellationFanout    bool
	Implemented           bool
	Authorized            bool
	TransitionFingerprint string
}

func (transition DockerProcessLifecycleTransition) Validate(blueprintAuthority string) error {
	if transition.Ordinal < 1 || transition.Ordinal > len(dockerLifecycleTransitionSpecs) ||
		!validDigest(blueprintAuthority) {
		return errors.New("docker process lifecycle transition identity is invalid")
	}
	spec := dockerLifecycleTransitionSpecs[transition.Ordinal-1]
	if transition.FromState != spec.FromState || transition.ToState != spec.ToState ||
		transition.Action != spec.Action || transition.WriteAheadRequired != spec.WriteAheadRequired ||
		!transition.GenerationFenced || transition.DaemonMutation != spec.DaemonMutation ||
		transition.CancellationFanout != spec.CancellationFanout || transition.Implemented ||
		transition.Authorized || transition.TransitionFingerprint != fingerprint(
		DockerProcessLifecycleProtocolVersion, blueprintAuthority,
		strconv.Itoa(transition.Ordinal), transition.FromState, transition.ToState,
		transition.Action, strconv.FormatBool(transition.WriteAheadRequired),
		strconv.FormatBool(transition.GenerationFenced), strconv.FormatBool(transition.DaemonMutation),
		strconv.FormatBool(transition.CancellationFanout), strconv.FormatBool(transition.Implemented),
		strconv.FormatBool(transition.Authorized)) {
		return errors.New("docker process lifecycle transition differs from the blocked blueprint")
	}
	return nil
}

type DockerProcessLifecycleBlueprint struct {
	ProtocolVersion        string
	OwnershipModel         string
	FixedEndpointRequired  bool
	WriteAheadRequired     bool
	GenerationFenced       bool
	CancellationFanout     bool
	BoundedLogs            bool
	MaxLogBytes            int64
	WaitRequired           bool
	GracefulThenForcedKill bool
	OrphanReconciliation   bool
	ImplementationPresent  bool
	DaemonMutationEnabled  bool
	OutputCommitAuthorized bool
	BlueprintFingerprint   string
	Transitions            []DockerProcessLifecycleTransition
}

func newDockerProcessLifecycleBlueprint(authorityFingerprint string,
	maxLogBytes int64,
) DockerProcessLifecycleBlueprint {
	value := DockerProcessLifecycleBlueprint{
		ProtocolVersion: DockerProcessLifecycleProtocolVersion,
		OwnershipModel:  DockerProcessOwnershipModel, FixedEndpointRequired: true,
		WriteAheadRequired: true, GenerationFenced: true, CancellationFanout: true,
		BoundedLogs: true, MaxLogBytes: maxLogBytes, WaitRequired: true,
		GracefulThenForcedKill: true, OrphanReconciliation: true,
		ImplementationPresent: false, DaemonMutationEnabled: false,
		OutputCommitAuthorized: false,
	}
	value.Transitions = make([]DockerProcessLifecycleTransition,
		len(dockerLifecycleTransitionSpecs))
	for index, spec := range dockerLifecycleTransitionSpecs {
		transition := DockerProcessLifecycleTransition{
			Ordinal: index + 1, FromState: spec.FromState, ToState: spec.ToState,
			Action: spec.Action, WriteAheadRequired: spec.WriteAheadRequired,
			GenerationFenced: true, DaemonMutation: spec.DaemonMutation,
			CancellationFanout: spec.CancellationFanout,
		}
		transition.TransitionFingerprint = fingerprint(
			DockerProcessLifecycleProtocolVersion, authorityFingerprint,
			strconv.Itoa(transition.Ordinal), transition.FromState, transition.ToState,
			transition.Action, strconv.FormatBool(transition.WriteAheadRequired),
			strconv.FormatBool(transition.GenerationFenced), strconv.FormatBool(transition.DaemonMutation),
			strconv.FormatBool(transition.CancellationFanout), strconv.FormatBool(transition.Implemented),
			strconv.FormatBool(transition.Authorized))
		value.Transitions[index] = transition
	}
	value.BlueprintFingerprint = dockerProcessLifecycleBlueprintFingerprint(
		value, authorityFingerprint)
	return value
}

func (value DockerProcessLifecycleBlueprint) Validate(authorityFingerprint string) error {
	if value.ProtocolVersion != DockerProcessLifecycleProtocolVersion ||
		value.OwnershipModel != DockerProcessOwnershipModel || !value.FixedEndpointRequired ||
		!value.WriteAheadRequired || !value.GenerationFenced || !value.CancellationFanout ||
		!value.BoundedLogs || value.MaxLogBytes < 1 || value.MaxLogBytes > MaxCapturedOutputBytes ||
		!value.WaitRequired || !value.GracefulThenForcedKill || !value.OrphanReconciliation ||
		value.ImplementationPresent || value.DaemonMutationEnabled || value.OutputCommitAuthorized ||
		len(value.Transitions) != DockerStartGateLifecycleTransitionCount {
		return errors.New("docker process lifecycle blueprint must remain bounded and unimplemented")
	}
	for index, transition := range value.Transitions {
		if transition.Ordinal != index+1 {
			return errors.New("docker process lifecycle transition order is invalid")
		}
		if err := transition.Validate(authorityFingerprint); err != nil {
			return err
		}
	}
	if value.BlueprintFingerprint != dockerProcessLifecycleBlueprintFingerprint(
		value, authorityFingerprint) {
		return errors.New("docker process lifecycle blueprint fingerprint is invalid")
	}
	return nil
}

func dockerProcessLifecycleBlueprintFingerprint(value DockerProcessLifecycleBlueprint,
	authorityFingerprint string,
) string {
	parts := []string{DockerProcessLifecycleProtocolVersion, authorityFingerprint,
		value.OwnershipModel, strconv.FormatBool(value.FixedEndpointRequired),
		strconv.FormatBool(value.WriteAheadRequired), strconv.FormatBool(value.GenerationFenced),
		strconv.FormatBool(value.CancellationFanout), strconv.FormatBool(value.BoundedLogs),
		strconv.FormatInt(value.MaxLogBytes, 10), strconv.FormatBool(value.WaitRequired),
		strconv.FormatBool(value.GracefulThenForcedKill),
		strconv.FormatBool(value.OrphanReconciliation),
		strconv.FormatBool(value.ImplementationPresent),
		strconv.FormatBool(value.DaemonMutationEnabled),
		strconv.FormatBool(value.OutputCommitAuthorized)}
	for _, transition := range value.Transitions {
		parts = append(parts, transition.TransitionFingerprint)
	}
	return fingerprint(parts...)
}

type DockerStartGateReviewBinding struct {
	CleanupIntentID          string
	CleanupResultID          string
	ApplicationIntentID      string
	ApplicationResultID      string
	ProjectionID             string
	ContainerPlanID          string
	PreflightID              string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ManifestFingerprint      string
	ThreatModelFingerprint   string
	CleanupResultFingerprint string
	MaxLogBytes              int64
}

func (binding DockerStartGateReviewBinding) Validate() error {
	for label, value := range map[string]string{
		"cleanup intent": binding.CleanupIntentID, "cleanup result": binding.CleanupResultID,
		"application intent": binding.ApplicationIntentID,
		"application result": binding.ApplicationResultID, "projection": binding.ProjectionID,
		"container plan": binding.ContainerPlanID, "preflight": binding.PreflightID,
		"Run": binding.RunID, "Mission": binding.MissionID, "workspace": binding.WorkspaceID,
	} {
		if err := validateStoredIdentity("Docker start-gate "+label, value); err != nil {
			return err
		}
	}
	for label, value := range map[string]string{
		"Manifest": binding.ManifestFingerprint, "threat model": binding.ThreatModelFingerprint,
		"cleanup result": binding.CleanupResultFingerprint,
	} {
		if !validDigest(value) {
			return fmt.Errorf("docker start-gate %s fingerprint is invalid", label)
		}
	}
	if binding.MaxLogBytes < 1 || binding.MaxLogBytes > MaxCapturedOutputBytes {
		return errors.New("docker start-gate log bound is invalid")
	}
	return nil
}

type DockerStartGateReview struct {
	ID string
	DockerStartGateReviewBinding
	ProtocolVersion            string
	ReviewedThroughSchema      int
	Status                     string
	Decision                   string
	TrustClass                 string
	OperationKeyDigest         string
	AuthorityFingerprint       string
	EvidenceFingerprint        string
	ReviewFingerprint          string
	OperatorConfirmed          bool
	RealDaemonChainVerified    bool
	RequiredCheckCount         int
	ProductionVerifiedCount    int
	SufficientCheckCount       int
	BlockerCount               int
	StartGatePassed            bool
	StartImplementationPresent bool
	ContainerStartAuthorized   bool
	ProcessExecutionAuthorized bool
	OutputExportAuthorized     bool
	ArtifactCommitAuthorized   bool
	Checks                     []DockerStartGateCheckReview
	Lifecycle                  DockerProcessLifecycleBlueprint
	RequestedBy                string
	CreatedAt                  time.Time
	Replayed                   bool
}

func NewDockerStartGateReview(id, keyDigest, requestedBy string,
	binding DockerStartGateReviewBinding, operatorConfirmed bool,
	createdAt time.Time,
) (DockerStartGateReview, error) {
	if err := validateStoredIdentity("Docker start-gate review id", id); err != nil {
		return DockerStartGateReview{}, err
	}
	if err := validateStoredIdentity("Docker start-gate requester", requestedBy); err != nil {
		return DockerStartGateReview{}, err
	}
	if !validDigest(keyDigest) || createdAt.IsZero() || !operatorConfirmed {
		return DockerStartGateReview{}, errors.New("docker start-gate review confirmation, digest, or timestamp is invalid")
	}
	if err := binding.Validate(); err != nil {
		return DockerStartGateReview{}, err
	}
	authority := dockerStartGateAuthorityFingerprint(binding)
	checks := make([]DockerStartGateCheckReview, len(dockerStartGateCheckSpecs))
	backendChecks := RequiredBackendChecks()
	for index, spec := range dockerStartGateCheckSpecs {
		item := DockerStartGateCheckReview{
			Ordinal: index + 1, Name: backendChecks[index].Name,
			EvidenceClass: spec.EvidenceClass, EvidenceSource: spec.EvidenceSource,
			BlockerCode: spec.BlockerCode, FutureGate: spec.FutureGate,
		}
		item.ReviewFingerprint = fingerprint(DockerStartGateReviewProtocolVersion,
			authority, strconv.Itoa(item.Ordinal), item.Name, item.EvidenceClass,
			item.EvidenceSource, strconv.FormatBool(item.ProductionVerified),
			strconv.FormatBool(item.SufficientForStart), item.BlockerCode, item.FutureGate)
		checks[index] = item
	}
	value := DockerStartGateReview{
		ID: id, DockerStartGateReviewBinding: binding,
		ProtocolVersion:       DockerStartGateReviewProtocolVersion,
		ReviewedThroughSchema: DockerStartGateReviewedThroughSchema,
		Status:                DockerStartGateReviewStatusBlocked, Decision: DockerStartGateReviewDecisionDeny,
		TrustClass: DockerStartGateReviewTrustDesignOnly, OperationKeyDigest: keyDigest,
		AuthorityFingerprint: authority, OperatorConfirmed: true,
		RequiredCheckCount: len(checks), BlockerCount: len(checks), Checks: checks,
		RequestedBy: requestedBy, CreatedAt: createdAt,
	}
	value.EvidenceFingerprint = dockerStartGateEvidenceFingerprint(value)
	value.Lifecycle = newDockerProcessLifecycleBlueprint(authority, binding.MaxLogBytes)
	value.ReviewFingerprint = dockerStartGateReviewFingerprint(value)
	return value, value.Validate()
}

func (value DockerStartGateReview) Validate() error {
	if err := validateStoredIdentity("Docker start-gate review id", value.ID); err != nil {
		return err
	}
	if err := validateStoredIdentity("Docker start-gate requester", value.RequestedBy); err != nil {
		return err
	}
	if err := value.DockerStartGateReviewBinding.Validate(); err != nil {
		return err
	}
	if value.ProtocolVersion != DockerStartGateReviewProtocolVersion ||
		value.ReviewedThroughSchema != DockerStartGateReviewedThroughSchema ||
		value.Status != DockerStartGateReviewStatusBlocked ||
		value.Decision != DockerStartGateReviewDecisionDeny ||
		value.TrustClass != DockerStartGateReviewTrustDesignOnly ||
		!validDigest(value.OperationKeyDigest) || !value.OperatorConfirmed ||
		value.RealDaemonChainVerified || value.RequiredCheckCount != DockerStartGateRequiredCheckCount ||
		value.ProductionVerifiedCount != DockerStartGateProductionVerifiedChecks ||
		value.SufficientCheckCount != 0 || value.BlockerCount != value.RequiredCheckCount ||
		value.StartGatePassed || value.StartImplementationPresent ||
		value.ContainerStartAuthorized || value.ProcessExecutionAuthorized ||
		value.OutputExportAuthorized || value.ArtifactCommitAuthorized ||
		len(value.Checks) != value.RequiredCheckCount || value.CreatedAt.IsZero() {
		return errors.New("docker start-gate review must remain a complete blocked design review")
	}
	if value.AuthorityFingerprint != dockerStartGateAuthorityFingerprint(
		value.DockerStartGateReviewBinding) {
		return errors.New("docker start-gate authority fingerprint is invalid")
	}
	for index, item := range value.Checks {
		if item.Ordinal != index+1 {
			return errors.New("docker start-gate check order is invalid")
		}
		if err := item.Validate(value.AuthorityFingerprint); err != nil {
			return err
		}
	}
	if value.EvidenceFingerprint != dockerStartGateEvidenceFingerprint(value) {
		return errors.New("docker start-gate evidence fingerprint is invalid")
	}
	if err := value.Lifecycle.Validate(value.AuthorityFingerprint); err != nil {
		return err
	}
	if value.ReviewFingerprint != dockerStartGateReviewFingerprint(value) {
		return errors.New("docker start-gate review fingerprint is invalid")
	}
	return nil
}

func dockerStartGateAuthorityFingerprint(binding DockerStartGateReviewBinding) string {
	return fingerprint("sandbox_docker_start_gate_authority.v1", binding.CleanupIntentID,
		binding.CleanupResultID, binding.ApplicationIntentID, binding.ApplicationResultID,
		binding.ProjectionID, binding.ContainerPlanID, binding.PreflightID, binding.RunID,
		binding.MissionID, binding.WorkspaceID, binding.ManifestFingerprint,
		binding.ThreatModelFingerprint, binding.CleanupResultFingerprint,
		strconv.FormatInt(binding.MaxLogBytes, 10))
}

func dockerStartGateEvidenceFingerprint(value DockerStartGateReview) string {
	parts := []string{DockerStartGateReviewProtocolVersion, value.AuthorityFingerprint,
		strconv.Itoa(len(value.Checks))}
	for _, item := range value.Checks {
		parts = append(parts, item.ReviewFingerprint)
	}
	return fingerprint(parts...)
}

func dockerStartGateReviewFingerprint(value DockerStartGateReview) string {
	return fingerprint(DockerStartGateReviewProtocolVersion, value.ID,
		value.AuthorityFingerprint, value.EvidenceFingerprint,
		value.Lifecycle.BlueprintFingerprint, value.Status, value.Decision, value.TrustClass,
		strconv.Itoa(value.ReviewedThroughSchema), strconv.Itoa(value.RequiredCheckCount),
		strconv.Itoa(value.ProductionVerifiedCount), strconv.Itoa(value.SufficientCheckCount),
		strconv.Itoa(value.BlockerCount), strconv.FormatBool(value.OperatorConfirmed),
		strconv.FormatBool(value.RealDaemonChainVerified),
		strconv.FormatBool(value.StartGatePassed),
		strconv.FormatBool(value.StartImplementationPresent),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized), value.RequestedBy,
		value.CreatedAt.UTC().Format(time.RFC3339Nano))
}

type DockerStartGateReviewOperation struct {
	KeyDigest          string
	RequestFingerprint string
	ReviewID           string
	CleanupIntentID    string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func NewDockerStartGateReviewOperation(keyDigest string,
	review DockerStartGateReview,
) (DockerStartGateReviewOperation, error) {
	value := DockerStartGateReviewOperation{
		KeyDigest: keyDigest, ReviewID: review.ID, CleanupIntentID: review.CleanupIntentID,
		RunID: review.RunID, RequestedBy: review.RequestedBy, CreatedAt: review.CreatedAt,
		RequestFingerprint: DockerStartGateReviewRequestFingerprint(review),
	}
	return value, value.Validate()
}

func (value DockerStartGateReviewOperation) Validate() error {
	for label, identity := range map[string]string{
		"review": value.ReviewID, "cleanup intent": value.CleanupIntentID,
		"Run": value.RunID, "requester": value.RequestedBy,
	} {
		if err := validateStoredIdentity("Docker start-gate operation "+label, identity); err != nil {
			return err
		}
	}
	if !validDigest(value.KeyDigest) || !validDigest(value.RequestFingerprint) ||
		value.CreatedAt.IsZero() {
		return errors.New("docker start-gate operation digest or timestamp is invalid")
	}
	return nil
}

func DockerStartGateReviewRequestFingerprint(value DockerStartGateReview) string {
	return fingerprint(DockerStartGateReviewOperationVersion, value.CleanupIntentID,
		value.ManifestFingerprint, value.AuthorityFingerprint, value.EvidenceFingerprint,
		value.Lifecycle.BlueprintFingerprint, value.RequestedBy)
}
