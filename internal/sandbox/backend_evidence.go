package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	BackendEvidenceProtocolVersion = "sandbox_backend_evidence.v1"
	BackendEvidenceSourceFake      = "in_memory_fake"
	BackendEvidenceTrustSimulation = "simulation_only"
	BackendEvidenceStatusComplete  = "simulation_complete"
	BackendEvidenceStatePass       = "simulated_pass"
)

type BackendEvidenceItem struct {
	Ordinal        int
	Name           string
	EvidenceState  string
	EvidenceDigest string
	Satisfied      bool
	Verified       bool
}

func (item BackendEvidenceItem) Validate() error {
	checks := RequiredBackendChecks()
	if item.Ordinal < 1 || item.Ordinal > len(checks) || item.Name != checks[item.Ordinal-1].Name ||
		item.EvidenceState != BackendEvidenceStatePass || !validDigest(item.EvidenceDigest) ||
		!item.Satisfied || item.Verified {
		return errors.New("sandbox backend evidence must remain simulated, satisfied, and unverified")
	}
	return nil
}

type BackendEvidenceReport struct {
	ProtocolVersion               string
	Source                        string
	TrustClass                    string
	Status                        string
	Backend                       Backend
	ImageDigest                   string
	DaemonCapabilitiesFingerprint string
	MountPlanFingerprint          string
	NetworkPlanFingerprint        string
	SecretPlanFingerprint         string
	ContainerConfigFingerprint    string
	ResourcePlanFingerprint       string
	TerminationPlanFingerprint    string
	OrphanPlanFingerprint         string
	OutputPlanFingerprint         string
	EvidenceFingerprint           string
	Items                         []BackendEvidenceItem
	ProductionVerified            bool
	BackendAvailable              bool
	BackendEnabled                bool
	ExecutionAuthorized           bool
	ArtifactCommitAuthorized      bool
}

func (report BackendEvidenceReport) Validate() error {
	if report.ProtocolVersion != BackendEvidenceProtocolVersion ||
		report.Source != BackendEvidenceSourceFake ||
		report.TrustClass != BackendEvidenceTrustSimulation ||
		report.Status != BackendEvidenceStatusComplete || report.Backend != BackendDocker ||
		!ValidOCIImageDigest(report.ImageDigest) || len(report.Items) != MaxBackendChecks ||
		report.ProductionVerified || report.BackendAvailable || report.BackendEnabled ||
		report.ExecutionAuthorized || report.ArtifactCommitAuthorized {
		return errors.New("sandbox backend evidence must remain a Docker simulation without authority")
	}
	for label, value := range map[string]string{
		"daemon capabilities": report.DaemonCapabilitiesFingerprint,
		"mount plan":          report.MountPlanFingerprint,
		"network plan":        report.NetworkPlanFingerprint,
		"secret plan":         report.SecretPlanFingerprint,
		"container config":    report.ContainerConfigFingerprint,
		"resource plan":       report.ResourcePlanFingerprint,
		"termination plan":    report.TerminationPlanFingerprint,
		"orphan plan":         report.OrphanPlanFingerprint,
		"output plan":         report.OutputPlanFingerprint,
		"evidence":            report.EvidenceFingerprint,
	} {
		if !validDigest(value) {
			return fmt.Errorf("sandbox backend evidence %s fingerprint is invalid", label)
		}
	}
	for index, item := range report.Items {
		if item.Ordinal != index+1 {
			return errors.New("sandbox backend evidence order is invalid")
		}
		if err := item.Validate(); err != nil {
			return err
		}
	}
	if report.EvidenceFingerprint != backendEvidenceFingerprint(report) {
		return errors.New("sandbox backend evidence aggregate fingerprint is invalid")
	}
	return nil
}

type BackendEvidenceProbeRequest struct {
	PreflightID            string
	Backend                Backend
	Manifest               Manifest
	ManifestFingerprint    string
	ThreatModelFingerprint string
	OutputPlanFingerprint  string
	ImageDigest            string
}

type BackendEvidenceClient interface {
	Probe(ctx context.Context, request BackendEvidenceProbeRequest) (BackendEvidenceReport, error)
}

// SimulationBackendClient derives configuration evidence without contacting a daemon.
type SimulationBackendClient struct{}

func NewSimulationBackendClient() SimulationBackendClient {
	return SimulationBackendClient{}
}

func (SimulationBackendClient) Probe(ctx context.Context,
	request BackendEvidenceProbeRequest,
) (BackendEvidenceReport, error) {
	if err := ctx.Err(); err != nil {
		return BackendEvidenceReport{}, err
	}
	if err := validateStoredIdentity("backend evidence preflight id", request.PreflightID); err != nil {
		return BackendEvidenceReport{}, err
	}
	manifest, err := NormalizeManifest(request.Manifest)
	if err != nil {
		return BackendEvidenceReport{}, err
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return BackendEvidenceReport{}, err
	}
	if request.Backend != BackendDocker || manifest.Backend != BackendDocker ||
		request.ManifestFingerprint != manifestFingerprint ||
		!validDigest(request.ThreatModelFingerprint) ||
		!validDigest(request.OutputPlanFingerprint) || !ValidOCIImageDigest(request.ImageDigest) {
		return BackendEvidenceReport{}, errors.New("sandbox fake backend probe binding is invalid")
	}

	report := BackendEvidenceReport{
		ProtocolVersion: BackendEvidenceProtocolVersion,
		Source:          BackendEvidenceSourceFake, TrustClass: BackendEvidenceTrustSimulation,
		Status: BackendEvidenceStatusComplete, Backend: BackendDocker,
		ImageDigest: request.ImageDigest,
		DaemonCapabilitiesFingerprint: fingerprint("sandbox_fake_daemon_capabilities.v1",
			"api=simulation", "rootless=true", "private_mounts=true", "network_policy=true",
			"resource_limits=true", "kill=true", "orphan_reconcile=true"),
		MountPlanFingerprint:   simulatedMountPlanFingerprint(manifest),
		NetworkPlanFingerprint: simulatedNetworkPlanFingerprint(manifest),
		SecretPlanFingerprint:  simulatedSecretPlanFingerprint(manifest),
		ContainerConfigFingerprint: fingerprint("sandbox_fake_container_config.v1", request.ImageDigest,
			"uid=65532", "gid=65532", "cap_drop=all", "no_new_privileges=true", "rootfs=readonly"),
		ResourcePlanFingerprint: fingerprint("sandbox_fake_resource_plan.v1",
			strconv.Itoa(manifest.Resources.CPUQuotaMillis), strconv.FormatInt(manifest.Resources.MemoryBytes, 10),
			strconv.Itoa(manifest.Resources.PIDs), strconv.FormatInt(manifest.Resources.MaxOutputBytes, 10)),
		TerminationPlanFingerprint: fingerprint("sandbox_fake_termination_plan.v1",
			strconv.Itoa(manifest.TimeoutSeconds), strconv.Itoa(manifest.Cancellation.GracePeriodMillis),
			"graceful_then_forced=true"),
		OrphanPlanFingerprint: fingerprint("sandbox_fake_orphan_plan.v1", request.PreflightID,
			"labels_bound=true", "reconcile_before_retry=true"),
		OutputPlanFingerprint: request.OutputPlanFingerprint,
	}

	checks := RequiredBackendChecks()
	report.Items = make([]BackendEvidenceItem, len(checks))
	for index, check := range checks {
		subject := simulatedEvidenceSubject(report, check.Name)
		report.Items[index] = BackendEvidenceItem{
			Ordinal: check.Ordinal, Name: check.Name, EvidenceState: BackendEvidenceStatePass,
			EvidenceDigest: fingerprint("sandbox_backend_evidence_item.v1", request.PreflightID,
				check.Name, subject),
			Satisfied: true, Verified: false,
		}
	}
	report.EvidenceFingerprint = backendEvidenceFingerprint(report)
	return report, report.Validate()
}

func ValidOCIImageDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && len(value) == len("sha256:")+64 &&
		validDigest(strings.TrimPrefix(value, "sha256:"))
}

func simulatedMountPlanFingerprint(manifest Manifest) string {
	parts := []string{"sandbox_fake_mount_plan.v1", strconv.Itoa(len(manifest.Mounts)),
		"propagation=private", "rootfs=readonly", "dedicated_output=true"}
	for _, mount := range manifest.Mounts {
		parts = append(parts, mount.Source, mount.Target, string(mount.Access))
	}
	return fingerprint(parts...)
}

func simulatedNetworkPlanFingerprint(manifest Manifest) string {
	parts := []string{"sandbox_fake_network_plan.v1", manifest.Network.Mode,
		strconv.Itoa(len(manifest.Network.AllowedTargets)), "default_deny=true"}
	parts = append(parts, manifest.Network.AllowedTargets...)
	return fingerprint(parts...)
}

func simulatedSecretPlanFingerprint(manifest Manifest) string {
	parts := []string{"sandbox_fake_secret_plan.v1", strconv.Itoa(manifest.SecretReferenceCount()),
		"ephemeral=true", "environment_persisted=false"}
	for _, binding := range manifest.Environment {
		parts = append(parts, binding.Name, string(binding.Source))
		if binding.Source == EnvironmentSecretRef {
			parts = append(parts, binding.Value)
		}
	}
	return fingerprint(parts...)
}

func simulatedEvidenceSubject(report BackendEvidenceReport, name string) string {
	switch name {
	case "host_path_isolation", "mount_propagation_private", "read_only_rootfs",
		"read_only_inputs", "dedicated_writable_output":
		return report.MountPlanFingerprint
	case "network_default_deny", "exact_network_allowlist":
		return report.NetworkPlanFingerprint
	case "ephemeral_secret_materialization":
		return report.SecretPlanFingerprint
	case "non_root_container_identity":
		return report.ContainerConfigFingerprint
	case "cpu_memory_pid_limits":
		return report.ResourcePlanFingerprint
	case "wall_clock_timeout", "graceful_then_forced_kill":
		return report.TerminationPlanFingerprint
	case "orphan_reconciliation":
		return report.OrphanPlanFingerprint
	case "output_regular_file_only", "output_symlink_special_rejection",
		"atomic_output_artifact_commit":
		return report.OutputPlanFingerprint
	default:
		return report.DaemonCapabilitiesFingerprint
	}
}

func backendEvidenceFingerprint(report BackendEvidenceReport) string {
	parts := []string{BackendEvidenceProtocolVersion, report.Source, report.TrustClass,
		report.Status, string(report.Backend), report.ImageDigest,
		report.DaemonCapabilitiesFingerprint, report.MountPlanFingerprint,
		report.NetworkPlanFingerprint, report.SecretPlanFingerprint,
		report.ContainerConfigFingerprint, report.ResourcePlanFingerprint,
		report.TerminationPlanFingerprint, report.OrphanPlanFingerprint,
		report.OutputPlanFingerprint, strconv.Itoa(len(report.Items)),
		strconv.FormatBool(report.ProductionVerified), strconv.FormatBool(report.BackendAvailable),
		strconv.FormatBool(report.BackendEnabled), strconv.FormatBool(report.ExecutionAuthorized),
		strconv.FormatBool(report.ArtifactCommitAuthorized)}
	for _, item := range report.Items {
		parts = append(parts, strconv.Itoa(item.Ordinal), item.Name, item.EvidenceState,
			item.EvidenceDigest, strconv.FormatBool(item.Satisfied), strconv.FormatBool(item.Verified))
	}
	return fingerprint(parts...)
}

type BackendEvidence struct {
	ID                       string
	PreflightID              string
	ExecutionID              string
	CandidateID              string
	PreparationID            string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ManifestFingerprint      string
	AuthorizationFingerprint string
	PolicyFingerprint        string
	MountBindingFingerprint  string
	InputArtifactDigest      string
	ThreatModelFingerprint   string
	Report                   BackendEvidenceReport
	RequestedBy              string
	CreatedAt                time.Time
	Replayed                 bool
}

func (evidence BackendEvidence) Validate() error {
	for label, value := range map[string]string{
		"backend evidence id": evidence.ID, "backend evidence preflight id": evidence.PreflightID,
		"backend evidence execution id":   evidence.ExecutionID,
		"backend evidence candidate id":   evidence.CandidateID,
		"backend evidence preparation id": evidence.PreparationID,
		"backend evidence Run id":         evidence.RunID, "backend evidence Mission id": evidence.MissionID,
		"backend evidence workspace id": evidence.WorkspaceID,
		"backend evidence requester":    evidence.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	for label, value := range map[string]string{
		"manifest":       evidence.ManifestFingerprint,
		"authorization":  evidence.AuthorizationFingerprint,
		"policy":         evidence.PolicyFingerprint,
		"mount binding":  evidence.MountBindingFingerprint,
		"input Artifact": evidence.InputArtifactDigest,
		"threat model":   evidence.ThreatModelFingerprint,
	} {
		if !validDigest(value) {
			return fmt.Errorf("sandbox backend evidence %s fingerprint is invalid", label)
		}
	}
	if evidence.CreatedAt.IsZero() {
		return errors.New("sandbox backend evidence creation time is required")
	}
	return evidence.Report.Validate()
}

type BackendEvidenceOperation struct {
	KeyDigest          string
	RequestFingerprint string
	EvidenceID         string
	PreflightID        string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (operation BackendEvidenceOperation) Validate() error {
	for label, value := range map[string]string{
		"backend evidence operation evidence id":  operation.EvidenceID,
		"backend evidence operation preflight id": operation.PreflightID,
		"backend evidence operation Run id":       operation.RunID,
		"backend evidence operation requester":    operation.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(operation.KeyDigest) || !validDigest(operation.RequestFingerprint) ||
		operation.CreatedAt.IsZero() {
		return errors.New("sandbox backend evidence operation is invalid")
	}
	return nil
}

func BackendEvidenceRequestFingerprint(evidence BackendEvidence) string {
	return fingerprint("sandbox_backend_evidence_request.v1", evidence.PreflightID,
		evidence.ManifestFingerprint, evidence.AuthorizationFingerprint,
		evidence.PolicyFingerprint, evidence.MountBindingFingerprint,
		evidence.InputArtifactDigest, evidence.ThreatModelFingerprint,
		evidence.Report.OutputPlanFingerprint, evidence.Report.ImageDigest,
		evidence.Report.EvidenceFingerprint, evidence.RequestedBy)
}
