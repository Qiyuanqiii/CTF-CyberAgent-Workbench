package sandbox

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"
)

const (
	DockerProductionEvidenceHarnessIntentProtocolVersion         = "sandbox_docker_production_evidence_harness_intent.v1"
	DockerProductionEvidenceHarnessReconciliationProtocolVersion = "sandbox_docker_production_evidence_harness_reconciliation.v1"
	DockerProductionEvidenceHarnessResultProtocolVersion         = "sandbox_docker_production_evidence_harness_result.v1"

	DockerProductionEvidenceHarnessReconciliationEmpty = "owned_scope_empty"
	DockerProductionEvidenceHarnessResultCommitted     = "evidence_committed"

	DockerProductionEvidenceHarnessLabelKey = "cyberagent.workbench.production-evidence-attempt"

	DockerProductionEvidenceHarnessReconciliationReads = 1
	DockerProductionEvidenceHarnessCaptureReads        = 4
	DockerProductionEvidenceHarnessMaxDaemonReads      = DockerProductionEvidenceHarnessReconciliationReads +
		DockerProductionEvidenceHarnessCaptureReads
	MaxDockerProductionEvidenceHarnessResources = 256
)

type DockerProductionEvidenceHarnessIntent struct {
	AttemptID                       string
	ReviewID                        string
	ContainerPlanID                 string
	RunID                           string
	ProtocolVersion                 string
	ImageDigest                     string
	EndpointClass                   string
	EndpointFingerprint             string
	LabelSelectorFingerprint        string
	MaxDaemonReads                  int
	OperatorConfirmed               bool
	ReadOnlyDaemonContactAuthorized bool
	DaemonWriteAuthorized           bool
	ContainerStartAuthorized        bool
	ProcessExecutionAuthorized      bool
	OutputExportAuthorized          bool
	ArtifactCommitAuthorized        bool
	IntentFingerprint               string
	RequestedBy                     string
	CreatedAt                       time.Time
}

func NewDockerProductionEvidenceHarnessIntent(attempt DockerProductionEvidenceAttempt,
	review DockerStartGateReview, plan DockerContainerPlan, now time.Time,
) (DockerProductionEvidenceHarnessIntent, error) {
	if attempt.Validate() != nil || review.Validate() != nil || plan.Validate() != nil ||
		attempt.ReviewID != review.ID || attempt.RunID != review.RunID ||
		review.ContainerPlanID != plan.ID || plan.RunID != attempt.RunID ||
		plan.MissionID != attempt.MissionID || plan.WorkspaceID != attempt.WorkspaceID ||
		plan.RequestedBy != attempt.RequestedBy || review.RequestedBy != attempt.RequestedBy ||
		now.Before(attempt.CreatedAt) {
		return DockerProductionEvidenceHarnessIntent{}, errors.New(
			"docker production evidence harness authority is invalid")
	}
	value := DockerProductionEvidenceHarnessIntent{
		AttemptID: attempt.ID, ReviewID: review.ID, ContainerPlanID: plan.ID,
		RunID: attempt.RunID, ProtocolVersion: DockerProductionEvidenceHarnessIntentProtocolVersion,
		ImageDigest: plan.ImageDigest, EndpointClass: attempt.EndpointClass,
		EndpointFingerprint: attempt.EndpointFingerprint,
		LabelSelectorFingerprint: fingerprint(
			"sandbox_docker_production_evidence_harness_label.v1",
			DockerProductionEvidenceHarnessLabelKey, attempt.ID),
		MaxDaemonReads:    DockerProductionEvidenceHarnessMaxDaemonReads,
		OperatorConfirmed: true, ReadOnlyDaemonContactAuthorized: true,
		RequestedBy: attempt.RequestedBy, CreatedAt: now.UTC(),
	}
	value.IntentFingerprint = dockerProductionEvidenceHarnessIntentFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidenceHarnessIntent) Validate() error {
	for _, identity := range []string{value.AttemptID, value.ReviewID,
		value.ContainerPlanID, value.RunID, value.RequestedBy} {
		if validateStoredIdentity("Docker production evidence harness identity", identity) != nil {
			return errors.New("docker production evidence harness identity is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	if err != nil || value.ProtocolVersion != DockerProductionEvidenceHarnessIntentProtocolVersion ||
		!ValidOCIImageDigest(value.ImageDigest) ||
		value.EndpointClass != DockerObservationEndpointLocalUnix ||
		value.EndpointFingerprint != endpoint.Fingerprint ||
		value.LabelSelectorFingerprint != fingerprint(
			"sandbox_docker_production_evidence_harness_label.v1",
			DockerProductionEvidenceHarnessLabelKey, value.AttemptID) ||
		value.MaxDaemonReads != DockerProductionEvidenceHarnessMaxDaemonReads ||
		!value.OperatorConfirmed || !value.ReadOnlyDaemonContactAuthorized ||
		value.DaemonWriteAuthorized || value.ContainerStartAuthorized ||
		value.ProcessExecutionAuthorized || value.OutputExportAuthorized ||
		value.ArtifactCommitAuthorized || !validDigest(value.IntentFingerprint) ||
		value.CreatedAt.IsZero() ||
		value.IntentFingerprint != dockerProductionEvidenceHarnessIntentFingerprint(value) {
		return errors.New("docker production evidence harness widened authority")
	}
	return nil
}

func dockerProductionEvidenceHarnessIntentFingerprint(
	value DockerProductionEvidenceHarnessIntent,
) string {
	return fingerprint(DockerProductionEvidenceHarnessIntentProtocolVersion,
		value.AttemptID, value.ReviewID, value.ContainerPlanID, value.RunID,
		value.ImageDigest, value.EndpointClass, value.EndpointFingerprint,
		value.LabelSelectorFingerprint, strconv.Itoa(value.MaxDaemonReads),
		strconv.FormatBool(value.OperatorConfirmed),
		strconv.FormatBool(value.ReadOnlyDaemonContactAuthorized),
		strconv.FormatBool(value.DaemonWriteAuthorized),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized), value.RequestedBy)
}

type DockerProductionEvidenceHarnessInventory struct {
	EndpointClass        string
	EndpointFingerprint  string
	RealDaemonContacted  bool
	DaemonReadCount      int
	OwnedResourceCount   int
	InventoryFingerprint string
}

func NewDockerProductionEvidenceHarnessInventory(endpoint DockerObservationEndpoint,
	ownedResourceFingerprints []string,
) (DockerProductionEvidenceHarnessInventory, error) {
	if endpoint.Validate() != nil || endpoint.Class != DockerObservationEndpointLocalUnix ||
		len(ownedResourceFingerprints) > MaxDockerProductionEvidenceHarnessResources {
		return DockerProductionEvidenceHarnessInventory{}, errors.New(
			"docker production evidence harness inventory is invalid")
	}
	ownedResourceFingerprints = append([]string(nil), ownedResourceFingerprints...)
	sort.Strings(ownedResourceFingerprints)
	parts := []string{"sandbox_docker_production_evidence_harness_inventory.v1",
		endpoint.Fingerprint, strconv.Itoa(len(ownedResourceFingerprints))}
	for index, value := range ownedResourceFingerprints {
		if !validDigest(value) {
			return DockerProductionEvidenceHarnessInventory{}, errors.New(
				"docker production evidence harness inventory digest is invalid")
		}
		if index > 0 && value == ownedResourceFingerprints[index-1] {
			return DockerProductionEvidenceHarnessInventory{}, errors.New(
				"docker production evidence harness inventory contains duplicate resources")
		}
		parts = append(parts, value)
	}
	value := DockerProductionEvidenceHarnessInventory{
		EndpointClass: endpoint.Class, EndpointFingerprint: endpoint.Fingerprint,
		RealDaemonContacted:  true,
		DaemonReadCount:      DockerProductionEvidenceHarnessReconciliationReads,
		OwnedResourceCount:   len(ownedResourceFingerprints),
		InventoryFingerprint: fingerprint(parts...),
	}
	return value, value.Validate()
}

func (value DockerProductionEvidenceHarnessInventory) Validate() error {
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	if err != nil || value.EndpointClass != DockerObservationEndpointLocalUnix ||
		value.EndpointFingerprint != endpoint.Fingerprint || !value.RealDaemonContacted ||
		value.DaemonReadCount != DockerProductionEvidenceHarnessReconciliationReads ||
		value.OwnedResourceCount < 0 ||
		value.OwnedResourceCount > MaxDockerProductionEvidenceHarnessResources ||
		!validDigest(value.InventoryFingerprint) {
		return errors.New("docker production evidence harness inventory is invalid")
	}
	return nil
}

type DockerProductionEvidenceHarnessReconciliation struct {
	AttemptID                        string
	Generation                       int64
	ProtocolVersion                  string
	Status                           string
	IntentFingerprint                string
	ControlReconciliationFingerprint string
	EndpointClass                    string
	EndpointFingerprint              string
	InventoryFingerprint             string
	RealDaemonContacted              bool
	DaemonReadCount                  int
	OwnedResourceCount               int
	ReconciliationFingerprint        string
	CreatedAt                        time.Time
}

func NewDockerProductionEvidenceHarnessReconciliation(
	intent DockerProductionEvidenceHarnessIntent,
	lease DockerProductionEvidenceAttemptLease,
	control DockerProductionEvidenceReconciliation,
	inventory DockerProductionEvidenceHarnessInventory,
	now time.Time,
) (DockerProductionEvidenceHarnessReconciliation, error) {
	if intent.Validate() != nil || lease.Validate() != nil || control.Validate() != nil ||
		inventory.Validate() != nil || lease.AttemptID != intent.AttemptID ||
		lease.Status != DockerProductionEvidenceAttemptLeaseActive ||
		control.AttemptID != intent.AttemptID || control.Generation != lease.Generation ||
		inventory.EndpointClass != intent.EndpointClass ||
		inventory.EndpointFingerprint != intent.EndpointFingerprint ||
		inventory.OwnedResourceCount != 0 || now.Before(control.CreatedAt) ||
		!now.Before(lease.ExpiresAt) {
		return DockerProductionEvidenceHarnessReconciliation{}, errors.New(
			"docker production evidence harness reconciliation authority is invalid")
	}
	value := DockerProductionEvidenceHarnessReconciliation{
		AttemptID: intent.AttemptID, Generation: lease.Generation,
		ProtocolVersion:                  DockerProductionEvidenceHarnessReconciliationProtocolVersion,
		Status:                           DockerProductionEvidenceHarnessReconciliationEmpty,
		IntentFingerprint:                intent.IntentFingerprint,
		ControlReconciliationFingerprint: control.ReconciliationFingerprint,
		EndpointClass:                    intent.EndpointClass, EndpointFingerprint: intent.EndpointFingerprint,
		InventoryFingerprint: inventory.InventoryFingerprint,
		RealDaemonContacted:  true, DaemonReadCount: inventory.DaemonReadCount,
		CreatedAt: now.UTC(),
	}
	value.ReconciliationFingerprint =
		dockerProductionEvidenceHarnessReconciliationFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidenceHarnessReconciliation) Validate() error {
	endpoint, endpointErr := NewDockerObservationEndpoint(value.EndpointClass)
	if validateStoredIdentity("Docker production evidence harness reconciliation attempt",
		value.AttemptID) != nil || value.Generation < 1 ||
		value.ProtocolVersion != DockerProductionEvidenceHarnessReconciliationProtocolVersion ||
		value.Status != DockerProductionEvidenceHarnessReconciliationEmpty ||
		!validDigest(value.IntentFingerprint) ||
		!validDigest(value.ControlReconciliationFingerprint) ||
		value.EndpointClass != DockerObservationEndpointLocalUnix ||
		endpointErr != nil || value.EndpointFingerprint != endpoint.Fingerprint ||
		!validDigest(value.InventoryFingerprint) ||
		!value.RealDaemonContacted ||
		value.DaemonReadCount != DockerProductionEvidenceHarnessReconciliationReads ||
		value.OwnedResourceCount != 0 || !validDigest(value.ReconciliationFingerprint) ||
		value.CreatedAt.IsZero() ||
		value.ReconciliationFingerprint !=
			dockerProductionEvidenceHarnessReconciliationFingerprint(value) {
		return errors.New("docker production evidence harness reconciliation is invalid")
	}
	return nil
}

func dockerProductionEvidenceHarnessReconciliationFingerprint(
	value DockerProductionEvidenceHarnessReconciliation,
) string {
	return fingerprint(DockerProductionEvidenceHarnessReconciliationProtocolVersion,
		value.AttemptID, strconv.FormatInt(value.Generation, 10), value.Status,
		value.IntentFingerprint, value.ControlReconciliationFingerprint,
		value.EndpointClass, value.EndpointFingerprint, value.InventoryFingerprint,
		strconv.FormatBool(value.RealDaemonContacted),
		strconv.Itoa(value.DaemonReadCount), strconv.Itoa(value.OwnedResourceCount))
}

type DockerProductionEvidenceHarnessResult struct {
	AttemptID                  string
	EvidenceID                 string
	ProtocolVersion            string
	Status                     string
	LeaseGeneration            int64
	IntentFingerprint          string
	ReconciliationFingerprint  string
	EvidenceCaptureFingerprint string
	DaemonReadCount            int
	ProbeCount                 int
	ObservedCount              int
	ProductionVerifiedCount    int
	RealDaemonContacted        bool
	DaemonWriteAuthorized      bool
	ContainerStartAuthorized   bool
	ProcessExecutionAuthorized bool
	OutputExportAuthorized     bool
	ArtifactCommitAuthorized   bool
	ResultFingerprint          string
	CreatedAt                  time.Time
}

func NewDockerProductionEvidenceHarnessResult(
	intent DockerProductionEvidenceHarnessIntent,
	lease DockerProductionEvidenceAttemptLease,
	reconciliation DockerProductionEvidenceHarnessReconciliation,
	evidence DockerProductionEvidence,
) (DockerProductionEvidenceHarnessResult, error) {
	if intent.Validate() != nil || lease.Validate() != nil ||
		reconciliation.Validate() != nil || evidence.Validate() != nil ||
		lease.AttemptID != intent.AttemptID ||
		lease.Status != DockerProductionEvidenceAttemptLeaseActive ||
		reconciliation.AttemptID != intent.AttemptID ||
		reconciliation.Generation != lease.Generation ||
		reconciliation.IntentFingerprint != intent.IntentFingerprint ||
		evidence.ReviewID != intent.ReviewID || evidence.RunID != intent.RunID ||
		!evidence.RealDaemonContacted ||
		evidence.Status != DockerProductionEvidenceStatusComplete ||
		evidence.RequiredCheckCount != MaxBackendChecks ||
		evidence.ObservedCount != MaxBackendChecks ||
		evidence.ProductionVerifiedCount != 0 ||
		evidence.CreatedAt.Before(reconciliation.CreatedAt) ||
		!evidence.CreatedAt.Before(lease.ExpiresAt) {
		return DockerProductionEvidenceHarnessResult{}, errors.New(
			"docker production evidence harness result authority is invalid")
	}
	value := DockerProductionEvidenceHarnessResult{
		AttemptID: intent.AttemptID, EvidenceID: evidence.ID,
		ProtocolVersion: DockerProductionEvidenceHarnessResultProtocolVersion,
		Status:          DockerProductionEvidenceHarnessResultCommitted,
		LeaseGeneration: lease.Generation, IntentFingerprint: intent.IntentFingerprint,
		ReconciliationFingerprint:  reconciliation.ReconciliationFingerprint,
		EvidenceCaptureFingerprint: evidence.CaptureFingerprint,
		DaemonReadCount:            DockerProductionEvidenceHarnessMaxDaemonReads,
		ProbeCount:                 MaxBackendChecks, ObservedCount: evidence.ObservedCount,
		ProductionVerifiedCount: evidence.ProductionVerifiedCount,
		RealDaemonContacted:     true, CreatedAt: evidence.CreatedAt,
	}
	value.ResultFingerprint = dockerProductionEvidenceHarnessResultFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidenceHarnessResult) Validate() error {
	if validateStoredIdentity("Docker production evidence harness result attempt",
		value.AttemptID) != nil ||
		validateStoredIdentity("Docker production evidence harness result evidence",
			value.EvidenceID) != nil ||
		value.ProtocolVersion != DockerProductionEvidenceHarnessResultProtocolVersion ||
		value.Status != DockerProductionEvidenceHarnessResultCommitted ||
		value.LeaseGeneration < 1 || !validDigest(value.IntentFingerprint) ||
		!validDigest(value.ReconciliationFingerprint) ||
		!validDigest(value.EvidenceCaptureFingerprint) ||
		value.DaemonReadCount != DockerProductionEvidenceHarnessMaxDaemonReads ||
		value.ProbeCount != MaxBackendChecks || value.ObservedCount != MaxBackendChecks ||
		value.ProductionVerifiedCount != 0 || !value.RealDaemonContacted ||
		value.DaemonWriteAuthorized || value.ContainerStartAuthorized ||
		value.ProcessExecutionAuthorized || value.OutputExportAuthorized ||
		value.ArtifactCommitAuthorized || !validDigest(value.ResultFingerprint) ||
		value.CreatedAt.IsZero() ||
		value.ResultFingerprint != dockerProductionEvidenceHarnessResultFingerprint(value) {
		return errors.New("docker production evidence harness result widened authority")
	}
	return nil
}

func dockerProductionEvidenceHarnessResultFingerprint(
	value DockerProductionEvidenceHarnessResult,
) string {
	return fingerprint(DockerProductionEvidenceHarnessResultProtocolVersion,
		value.AttemptID, value.EvidenceID, value.Status,
		strconv.FormatInt(value.LeaseGeneration, 10), value.IntentFingerprint,
		value.ReconciliationFingerprint, value.EvidenceCaptureFingerprint,
		strconv.Itoa(value.DaemonReadCount), strconv.Itoa(value.ProbeCount),
		strconv.Itoa(value.ObservedCount), strconv.Itoa(value.ProductionVerifiedCount),
		strconv.FormatBool(value.RealDaemonContacted),
		strconv.FormatBool(value.DaemonWriteAuthorized),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized))
}

type DockerProductionEvidenceHarnessRequest struct {
	DockerProductionEvidenceCaptureRequest
	ImageDigest                      string
	IntentFingerprint                string
	ControlReconciliationFingerprint string
}

func (request DockerProductionEvidenceHarnessRequest) Validate() error {
	if request.DockerProductionEvidenceCaptureRequest.Validate() != nil ||
		!ValidOCIImageDigest(request.ImageDigest) ||
		!validDigest(request.IntentFingerprint) ||
		!validDigest(request.ControlReconciliationFingerprint) {
		return errors.New("docker production evidence harness request is invalid")
	}
	return nil
}

type DockerProductionEvidenceHarnessCaptureRequest struct {
	DockerProductionEvidenceHarnessRequest
	HarnessReconciliationFingerprint string
}

func (request DockerProductionEvidenceHarnessCaptureRequest) Validate() error {
	if request.DockerProductionEvidenceHarnessRequest.Validate() != nil ||
		!validDigest(request.HarnessReconciliationFingerprint) {
		return errors.New("docker production evidence harness capture request is invalid")
	}
	return nil
}

type DockerProductionEvidenceHarness interface {
	DockerProductionEvidenceCollector
	HarnessEnabled() bool
	ReconcileHarness(context.Context, DockerProductionEvidenceHarnessRequest) (
		DockerProductionEvidenceHarnessInventory, error)
	CaptureHarness(context.Context, DockerProductionEvidenceHarnessCaptureRequest) (
		DockerProductionEvidenceObservation, error)
}

type DockerProductionEvidenceHarnessTransport interface {
	DockerReadOnlyTransport
	ListProductionEvidenceResources(context.Context, string) (
		DockerProductionEvidenceHarnessInventory, error)
}
