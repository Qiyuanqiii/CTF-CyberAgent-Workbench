package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DockerProductionEvidenceProtocolVersion  = "sandbox_docker_production_evidence.v1"
	DockerProductionEvidenceOperationVersion = "sandbox_docker_production_evidence_operation.v1"
	DockerProductionEvidenceSuiteVersion     = "sandbox_docker_production_evidence_suite.v1"

	DockerProductionEvidenceSourceLocal = "go_local_collector"
	DockerProductionEvidenceTrustClass  = "machine_observation_non_authorizing"

	DockerProductionEvidenceStatusUnsupported = "unsupported_platform"
	DockerProductionEvidenceStatusOptIn       = "opt_in_required"
	DockerProductionEvidenceStatusPending     = "harness_pending"
	DockerProductionEvidenceStatusComplete    = "capture_complete"

	DockerProductionEvidenceStateNotObserved = "not_observed"
	DockerProductionEvidenceStateFailed      = "observed_failed"
	DockerProductionEvidenceStateVerified    = "production_verified"

	DockerProductionEvidencePlatformLinux       = "linux"
	DockerProductionEvidencePlatformUnsupported = "unsupported"
	DockerProductionEvidenceEndpointNone        = "none"

	DockerProductionEvidenceOptInEnv  = "CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE"
	MaxDockerProductionEvidencePerRun = 32
)

var dockerProductionEvidenceProbeCodes = [...]string{
	"probe_host_path_isolation",
	"probe_private_mount_propagation",
	"probe_read_only_rootfs",
	"probe_read_only_inputs",
	"probe_dedicated_writable_output",
	"probe_network_default_deny",
	"probe_exact_network_allowlist",
	"probe_ephemeral_secret_materialization",
	"probe_non_root_identity",
	"probe_cpu_memory_pid_limits",
	"probe_wall_clock_timeout",
	"probe_term_kill_escalation",
	"probe_orphan_reconciliation",
	"probe_output_regular_file_only",
	"probe_output_link_special_rejection",
	"probe_atomic_artifact_commit",
}

func DockerProductionEvidenceSuiteFingerprint() string {
	checks := RequiredBackendChecks()
	parts := []string{DockerProductionEvidenceSuiteVersion, strconv.Itoa(len(checks))}
	for index, check := range checks {
		parts = append(parts, strconv.Itoa(check.Ordinal), check.Name,
			dockerProductionEvidenceProbeCodes[index],
			dockerStartGateCheckSpecs[index].FutureGate)
	}
	return fingerprint(parts...)
}

type DockerProductionEvidenceItem struct {
	Ordinal            int
	Name               string
	ProbeCode          string
	State              string
	Observed           bool
	ProductionVerified bool
	SufficientForStart bool
	BlockerCode        string
	EvidenceDigest     string
}

func (item DockerProductionEvidenceItem) Validate(authorityFingerprint,
	environmentFingerprint string,
) error {
	checks := RequiredBackendChecks()
	if item.Ordinal < 1 || item.Ordinal > len(checks) ||
		item.Name != checks[item.Ordinal-1].Name ||
		item.ProbeCode != dockerProductionEvidenceProbeCodes[item.Ordinal-1] ||
		item.BlockerCode != dockerStartGateCheckSpecs[item.Ordinal-1].BlockerCode ||
		!validDigest(authorityFingerprint) || !validDigest(environmentFingerprint) ||
		item.SufficientForStart {
		return errors.New("docker production evidence item identity is invalid")
	}
	switch item.State {
	case DockerProductionEvidenceStateNotObserved:
		if item.Observed || item.ProductionVerified {
			return errors.New("unobserved Docker production evidence cannot be verified")
		}
	case DockerProductionEvidenceStateFailed:
		if !item.Observed || item.ProductionVerified {
			return errors.New("failed Docker production evidence must be observed and unverified")
		}
	case DockerProductionEvidenceStateVerified:
		if !item.Observed || !item.ProductionVerified {
			return errors.New("verified Docker production evidence must be observed")
		}
	default:
		return errors.New("docker production evidence item state is invalid")
	}
	if item.EvidenceDigest != dockerProductionEvidenceItemDigest(item,
		authorityFingerprint, environmentFingerprint) {
		return errors.New("docker production evidence item digest is invalid")
	}
	return nil
}

func dockerProductionEvidenceItemDigest(item DockerProductionEvidenceItem,
	authorityFingerprint, environmentFingerprint string,
) string {
	return fingerprint(DockerProductionEvidenceSuiteVersion, authorityFingerprint,
		environmentFingerprint, strconv.Itoa(item.Ordinal), item.Name, item.ProbeCode,
		item.State, strconv.FormatBool(item.Observed),
		strconv.FormatBool(item.ProductionVerified),
		strconv.FormatBool(item.SufficientForStart), item.BlockerCode)
}

type DockerProductionEvidenceObservation struct {
	Source                 string
	TrustClass             string
	Status                 string
	PlatformClass          string
	EndpointClass          string
	SuiteFingerprint       string
	EnvironmentFingerprint string
	RealDaemonContacted    bool
	Items                  []DockerProductionEvidenceItem
}

func (value DockerProductionEvidenceObservation) Validate(authorityFingerprint string) error {
	if value.Source != DockerProductionEvidenceSourceLocal ||
		value.TrustClass != DockerProductionEvidenceTrustClass ||
		value.SuiteFingerprint != DockerProductionEvidenceSuiteFingerprint() ||
		!validDigest(authorityFingerprint) || !validDigest(value.EnvironmentFingerprint) ||
		len(value.Items) != MaxBackendChecks {
		return errors.New("docker production evidence observation is invalid")
	}
	switch value.Status {
	case DockerProductionEvidenceStatusUnsupported:
		if value.PlatformClass != DockerProductionEvidencePlatformUnsupported ||
			value.EndpointClass != DockerProductionEvidenceEndpointNone || value.RealDaemonContacted {
			return errors.New("unsupported Docker evidence capture must not contact a daemon")
		}
	case DockerProductionEvidenceStatusOptIn, DockerProductionEvidenceStatusPending:
		if value.PlatformClass != DockerProductionEvidencePlatformLinux ||
			value.EndpointClass != DockerObservationEndpointLocalUnix || value.RealDaemonContacted {
			return errors.New("inactive Docker evidence capture must not contact a daemon")
		}
	case DockerProductionEvidenceStatusComplete:
		if value.PlatformClass != DockerProductionEvidencePlatformLinux ||
			value.EndpointClass != DockerObservationEndpointLocalUnix || !value.RealDaemonContacted {
			return errors.New("complete Docker evidence capture requires the fixed Linux daemon endpoint")
		}
	default:
		return errors.New("docker production evidence capture status is invalid")
	}
	for index, item := range value.Items {
		if item.Ordinal != index+1 {
			return errors.New("docker production evidence item order is invalid")
		}
		if err := item.Validate(authorityFingerprint, value.EnvironmentFingerprint); err != nil {
			return err
		}
		if value.Status != DockerProductionEvidenceStatusComplete &&
			item.State != DockerProductionEvidenceStateNotObserved {
			return errors.New("inactive Docker production evidence cannot contain observations")
		}
	}
	return nil
}

type DockerProductionEvidenceCaptureRequest struct {
	ReviewID             string
	RunID                string
	AuthorityFingerprint string
}

func (request DockerProductionEvidenceCaptureRequest) Validate() error {
	if validateStoredIdentity("Docker production evidence review", request.ReviewID) != nil ||
		validateStoredIdentity("Docker production evidence Run", request.RunID) != nil ||
		!validDigest(request.AuthorityFingerprint) {
		return errors.New("docker production evidence capture request is invalid")
	}
	return nil
}

type DockerProductionEvidenceCollector interface {
	Capture(context.Context, DockerProductionEvidenceCaptureRequest) (
		DockerProductionEvidenceObservation, error)
}

type LocalDockerProductionEvidenceCollector struct {
	platform string
	arch     string
	lookup   func(string) (string, bool)
}

func NewLocalDockerProductionEvidenceCollector() LocalDockerProductionEvidenceCollector {
	return LocalDockerProductionEvidenceCollector{
		platform: runtime.GOOS, arch: runtime.GOARCH, lookup: os.LookupEnv,
	}
}

func (collector LocalDockerProductionEvidenceCollector) Capture(ctx context.Context,
	request DockerProductionEvidenceCaptureRequest,
) (DockerProductionEvidenceObservation, error) {
	if err := ctx.Err(); err != nil {
		return DockerProductionEvidenceObservation{}, err
	}
	if err := request.Validate(); err != nil {
		return DockerProductionEvidenceObservation{}, err
	}
	platformClass := DockerProductionEvidencePlatformUnsupported
	endpointClass := DockerProductionEvidenceEndpointNone
	status := DockerProductionEvidenceStatusUnsupported
	optedIn := false
	if collector.platform == DockerProductionEvidencePlatformLinux {
		platformClass = DockerProductionEvidencePlatformLinux
		endpointClass = DockerObservationEndpointLocalUnix
		status = DockerProductionEvidenceStatusOptIn
		if collector.lookup != nil {
			value, found := collector.lookup(DockerProductionEvidenceOptInEnv)
			optedIn = found && strings.TrimSpace(value) == "1"
		}
		if optedIn {
			status = DockerProductionEvidenceStatusPending
		}
	}
	environmentFingerprint := fingerprint("sandbox_docker_production_environment.v1",
		collector.platform, collector.arch, endpointClass, strconv.FormatBool(optedIn),
		DockerProductionEvidenceSuiteFingerprint())
	items := newUnobservedDockerProductionEvidenceItems(request.AuthorityFingerprint,
		environmentFingerprint)
	observation := DockerProductionEvidenceObservation{
		Source:     DockerProductionEvidenceSourceLocal,
		TrustClass: DockerProductionEvidenceTrustClass, Status: status,
		PlatformClass: platformClass, EndpointClass: endpointClass,
		SuiteFingerprint:       DockerProductionEvidenceSuiteFingerprint(),
		EnvironmentFingerprint: environmentFingerprint, Items: items,
	}
	return observation, observation.Validate(request.AuthorityFingerprint)
}

func newUnobservedDockerProductionEvidenceItems(authorityFingerprint,
	environmentFingerprint string,
) []DockerProductionEvidenceItem {
	checks := RequiredBackendChecks()
	items := make([]DockerProductionEvidenceItem, len(checks))
	for index, check := range checks {
		item := DockerProductionEvidenceItem{
			Ordinal: check.Ordinal, Name: check.Name,
			ProbeCode:   dockerProductionEvidenceProbeCodes[index],
			State:       DockerProductionEvidenceStateNotObserved,
			BlockerCode: dockerStartGateCheckSpecs[index].BlockerCode,
		}
		item.EvidenceDigest = dockerProductionEvidenceItemDigest(item,
			authorityFingerprint, environmentFingerprint)
		items[index] = item
	}
	return items
}

type DockerProductionEvidence struct {
	ID                         string
	ReviewID                   string
	CleanupIntentID            string
	RunID                      string
	MissionID                  string
	WorkspaceID                string
	ProtocolVersion            string
	OperationKeyDigest         string
	ReviewFingerprint          string
	AuthorityFingerprint       string
	ThreatModelFingerprint     string
	Source                     string
	TrustClass                 string
	Status                     string
	PlatformClass              string
	EndpointClass              string
	SuiteFingerprint           string
	EnvironmentFingerprint     string
	EvidenceFingerprint        string
	CaptureFingerprint         string
	OperatorConfirmed          bool
	RealDaemonContacted        bool
	RequiredCheckCount         int
	ObservedCount              int
	ProductionVerifiedCount    int
	SufficientCheckCount       int
	BlockerCount               int
	StartGatePassed            bool
	ContainerStartAuthorized   bool
	ProcessExecutionAuthorized bool
	OutputExportAuthorized     bool
	ArtifactCommitAuthorized   bool
	Items                      []DockerProductionEvidenceItem
	RequestedBy                string
	CreatedAt                  time.Time
	Replayed                   bool
}

func NewDockerProductionEvidence(id, keyDigest, requestedBy string,
	review DockerStartGateReview, observation DockerProductionEvidenceObservation,
	operatorConfirmed bool, createdAt time.Time,
) (DockerProductionEvidence, error) {
	if review.Validate() != nil || observation.Validate(review.AuthorityFingerprint) != nil ||
		validateStoredIdentity("Docker production evidence id", id) != nil ||
		validateStoredIdentity("Docker production evidence requester", requestedBy) != nil ||
		requestedBy != review.RequestedBy || !validDigest(keyDigest) || !operatorConfirmed ||
		createdAt.IsZero() {
		return DockerProductionEvidence{}, errors.New("docker production evidence input is invalid")
	}
	value := DockerProductionEvidence{
		ID: id, ReviewID: review.ID, CleanupIntentID: review.CleanupIntentID,
		RunID: review.RunID, MissionID: review.MissionID, WorkspaceID: review.WorkspaceID,
		ProtocolVersion:    DockerProductionEvidenceProtocolVersion,
		OperationKeyDigest: keyDigest, ReviewFingerprint: review.ReviewFingerprint,
		AuthorityFingerprint:   review.AuthorityFingerprint,
		ThreatModelFingerprint: review.ThreatModelFingerprint,
		Source:                 observation.Source, TrustClass: observation.TrustClass, Status: observation.Status,
		PlatformClass: observation.PlatformClass, EndpointClass: observation.EndpointClass,
		SuiteFingerprint:       observation.SuiteFingerprint,
		EnvironmentFingerprint: observation.EnvironmentFingerprint,
		OperatorConfirmed:      true, RealDaemonContacted: observation.RealDaemonContacted,
		RequiredCheckCount: len(observation.Items), BlockerCount: len(observation.Items),
		Items:       append([]DockerProductionEvidenceItem(nil), observation.Items...),
		RequestedBy: requestedBy, CreatedAt: createdAt.UTC(),
	}
	for _, item := range value.Items {
		if item.Observed {
			value.ObservedCount++
		}
		if item.ProductionVerified {
			value.ProductionVerifiedCount++
		}
	}
	value.EvidenceFingerprint = dockerProductionEvidenceFingerprint(value)
	value.CaptureFingerprint = dockerProductionEvidenceCaptureFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidence) Validate() error {
	identities := [...]struct {
		label string
		value string
	}{
		{label: "id", value: value.ID},
		{label: "review", value: value.ReviewID},
		{label: "cleanup intent", value: value.CleanupIntentID},
		{label: "Run", value: value.RunID},
		{label: "Mission", value: value.MissionID},
		{label: "workspace", value: value.WorkspaceID},
		{label: "requester", value: value.RequestedBy},
	}
	for _, identity := range identities {
		if err := validateStoredIdentity("Docker production evidence "+identity.label,
			identity.value); err != nil {
			return err
		}
	}
	if value.ProtocolVersion != DockerProductionEvidenceProtocolVersion ||
		!validDigest(value.OperationKeyDigest) || !validDigest(value.ReviewFingerprint) ||
		!validDigest(value.AuthorityFingerprint) || !validDigest(value.ThreatModelFingerprint) ||
		!value.OperatorConfirmed || value.RequiredCheckCount != MaxBackendChecks ||
		len(value.Items) != value.RequiredCheckCount || value.SufficientCheckCount != 0 ||
		value.BlockerCount != value.RequiredCheckCount || value.StartGatePassed ||
		value.ContainerStartAuthorized || value.ProcessExecutionAuthorized ||
		value.OutputExportAuthorized || value.ArtifactCommitAuthorized || value.CreatedAt.IsZero() {
		return errors.New("docker production evidence must remain complete and non-authorizing")
	}
	observation := DockerProductionEvidenceObservation{
		Source: value.Source, TrustClass: value.TrustClass, Status: value.Status,
		PlatformClass: value.PlatformClass, EndpointClass: value.EndpointClass,
		SuiteFingerprint:       value.SuiteFingerprint,
		EnvironmentFingerprint: value.EnvironmentFingerprint,
		RealDaemonContacted:    value.RealDaemonContacted, Items: value.Items,
	}
	if err := observation.Validate(value.AuthorityFingerprint); err != nil {
		return err
	}
	observed, verified := 0, 0
	for _, item := range value.Items {
		if item.Observed {
			observed++
		}
		if item.ProductionVerified {
			verified++
		}
	}
	if value.ObservedCount != observed || value.ProductionVerifiedCount != verified ||
		value.EvidenceFingerprint != dockerProductionEvidenceFingerprint(value) ||
		value.CaptureFingerprint != dockerProductionEvidenceCaptureFingerprint(value) {
		return errors.New("docker production evidence aggregate is invalid")
	}
	return nil
}

func dockerProductionEvidenceFingerprint(value DockerProductionEvidence) string {
	parts := []string{DockerProductionEvidenceProtocolVersion, value.AuthorityFingerprint,
		value.SuiteFingerprint, value.EnvironmentFingerprint, value.Status,
		strconv.Itoa(len(value.Items))}
	for _, item := range value.Items {
		parts = append(parts, item.EvidenceDigest)
	}
	return fingerprint(parts...)
}

func dockerProductionEvidenceCaptureFingerprint(value DockerProductionEvidence) string {
	return fingerprint(DockerProductionEvidenceProtocolVersion, value.ID, value.ReviewID,
		value.CleanupIntentID, value.RunID, value.MissionID, value.WorkspaceID,
		value.OperationKeyDigest, value.ReviewFingerprint, value.AuthorityFingerprint,
		value.ThreatModelFingerprint, value.Source, value.TrustClass, value.Status,
		value.PlatformClass, value.EndpointClass, value.SuiteFingerprint,
		value.EnvironmentFingerprint, value.EvidenceFingerprint,
		strconv.FormatBool(value.OperatorConfirmed),
		strconv.FormatBool(value.RealDaemonContacted),
		strconv.Itoa(value.RequiredCheckCount), strconv.Itoa(value.ObservedCount),
		strconv.Itoa(value.ProductionVerifiedCount),
		strconv.Itoa(value.SufficientCheckCount), strconv.Itoa(value.BlockerCount),
		strconv.FormatBool(value.StartGatePassed),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized), value.RequestedBy,
		value.CreatedAt.UTC().Format(time.RFC3339Nano))
}

type DockerProductionEvidenceOperation struct {
	KeyDigest          string
	RequestFingerprint string
	EvidenceID         string
	ReviewID           string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func NewDockerProductionEvidenceOperation(keyDigest string,
	value DockerProductionEvidence,
) (DockerProductionEvidenceOperation, error) {
	operation := DockerProductionEvidenceOperation{
		KeyDigest: keyDigest, EvidenceID: value.ID, ReviewID: value.ReviewID,
		RunID: value.RunID, RequestedBy: value.RequestedBy, CreatedAt: value.CreatedAt,
		RequestFingerprint: DockerProductionEvidenceRequestFingerprint(value),
	}
	return operation, operation.Validate()
}

func (value DockerProductionEvidenceOperation) Validate() error {
	identities := [...]struct {
		label string
		value string
	}{
		{label: "evidence", value: value.EvidenceID},
		{label: "review", value: value.ReviewID},
		{label: "Run", value: value.RunID},
		{label: "requester", value: value.RequestedBy},
	}
	for _, identity := range identities {
		if err := validateStoredIdentity("Docker production evidence operation "+identity.label,
			identity.value); err != nil {
			return err
		}
	}
	if !validDigest(value.KeyDigest) || !validDigest(value.RequestFingerprint) ||
		value.CreatedAt.IsZero() {
		return errors.New("docker production evidence operation digest or timestamp is invalid")
	}
	return nil
}

func DockerProductionEvidenceRequestFingerprint(value DockerProductionEvidence) string {
	return DockerProductionEvidenceCaptureRequestFingerprint(value.ReviewID, value.RunID,
		value.AuthorityFingerprint, value.SuiteFingerprint, value.RequestedBy)
}

func DockerProductionEvidenceCaptureRequestFingerprint(reviewID, runID,
	authorityFingerprint, suiteFingerprint, requestedBy string,
) string {
	return fingerprint(DockerProductionEvidenceOperationVersion, reviewID, runID,
		authorityFingerprint, suiteFingerprint, requestedBy)
}

func DockerProductionEvidenceStatusDescription(status string) string {
	switch status {
	case DockerProductionEvidenceStatusUnsupported:
		return "production evidence capture requires Linux"
	case DockerProductionEvidenceStatusOptIn:
		return fmt.Sprintf("set %s=1 to request the future Linux harness",
			DockerProductionEvidenceOptInEnv)
	case DockerProductionEvidenceStatusPending:
		return "the Linux real-daemon harness is not enabled in this release"
	case DockerProductionEvidenceStatusComplete:
		return "machine capture completed; independent start review is still required"
	default:
		return "unknown capture status"
	}
}
