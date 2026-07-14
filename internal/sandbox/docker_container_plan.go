package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DockerContainerSpecProtocolVersion    = "sandbox_docker_container_spec.v1"
	DockerContainerPlanProtocolVersion    = "sandbox_docker_container_plan.v1"
	DockerWriteTransactionProtocolVersion = "sandbox_docker_write_transaction.v1"
	DockerContainerPlanSourceCompiler     = "go_deterministic_compiler"
	DockerContainerPlanTrustSimulation    = "simulation_only"
	DockerContainerPlanStatusCompiled     = "compiled_fake_transaction_committed"
	DockerWriteTransactionSourceFake      = "in_memory_fake"
	DockerWriteTransactionStatusCommitted = "fake_committed"
	DockerContainerFixedUser              = "65532:65532"
	DockerMountPropagationPrivate         = "rprivate"
	DockerNetworkDriverNone               = "none"
	DockerNetworkDriverManagedEgress      = "cyberagent_managed_egress"
	DockerSecretDeliveryEphemeralPrivate  = "ephemeral_private_bind"
	DockerSecretMountTarget               = "/run/cyberagent/secrets"
	DockerInputArtifactMountTarget        = "/run/cyberagent/inputs"
	DockerTerminationSignalGraceful       = "SIGTERM"
	DockerTerminationSignalForced         = "SIGKILL"
	DockerContainerControlStateCompiled   = "compiled_not_applied"
	DockerWriteStepStateCommitted         = "fake_committed"
	MaxDockerContainerControls            = MaxBackendChecks
	MaxDockerWriteSteps                   = 7
	MaxDockerContainerPlansPerObservation = 1
)

var dockerWriteStepNames = [...]string{
	"reconcile_orphans",
	"create_container",
	"start_container",
	"wait_container",
	"stop_container",
	"export_outputs",
	"remove_container",
}

type DockerContainerMountSpec struct {
	Ordinal         int
	Source          string
	Target          string
	Access          MountAccess
	Propagation     string
	DedicatedOutput bool
	InputReadOnly   bool
}

func (mount DockerContainerMountSpec) Validate() error {
	if mount.Ordinal < 1 || mount.Ordinal > MaxMounts ||
		validateWorkspacePath("Docker container mount source", mount.Source) != nil ||
		validateVirtualPath("Docker container mount target", mount.Target) != nil ||
		!mount.Access.Valid() || mount.Propagation != DockerMountPropagationPrivate {
		return errors.New("docker container mount specification is invalid")
	}
	if mount.DedicatedOutput != (mount.Access == MountReadWrite) ||
		mount.InputReadOnly != (mount.Access == MountReadOnly) {
		return errors.New("docker container mount access role is invalid")
	}
	return nil
}

type DockerContainerEnvironmentSpec struct {
	Name            string
	Source          EnvironmentSource
	LiteralValue    string
	SecretReference string
	SecretPath      string
}

func (binding DockerContainerEnvironmentSpec) Validate() error {
	if !environmentNamePattern.MatchString(binding.Name) || !binding.Source.Valid() {
		return errors.New("docker container environment binding is invalid")
	}
	switch binding.Source {
	case EnvironmentLiteral:
		if binding.SecretReference != "" || binding.SecretPath != "" ||
			validateBoundedText("Docker environment literal", binding.LiteralValue, 4096, true) != nil {
			return errors.New("docker literal environment binding is invalid")
		}
	case EnvironmentSecretRef:
		if binding.LiteralValue != "" || validateIdentity("Docker secret reference", binding.SecretReference) != nil ||
			validateVirtualPath("Docker secret path", binding.SecretPath) != nil ||
			!pathWithin(binding.SecretPath, DockerSecretMountTarget) || binding.SecretPath == DockerSecretMountTarget {
			return errors.New("docker secret environment binding is invalid")
		}
	}
	return nil
}

type DockerContainerNetworkSpec struct {
	Mode           string
	Driver         string
	AllowedTargets []string
	DefaultDeny    bool
	ExactAllowlist bool
	GuardRequired  bool
}

func (network DockerContainerNetworkSpec) Validate() error {
	if !network.DefaultDeny {
		return errors.New("docker container network must remain default-deny")
	}
	for index, target := range network.AllowedTargets {
		normalized, err := NormalizeAllowedTarget(target)
		if err != nil || normalized != target || (index > 0 && network.AllowedTargets[index-1] >= target) {
			return errors.New("docker container network target set is invalid")
		}
	}
	switch network.Mode {
	case "disabled":
		if network.Driver != DockerNetworkDriverNone || len(network.AllowedTargets) != 0 ||
			network.ExactAllowlist || network.GuardRequired {
			return errors.New("disabled Docker network plan is invalid")
		}
	case "allowlist":
		if network.Driver != DockerNetworkDriverManagedEgress || len(network.AllowedTargets) < 1 ||
			len(network.AllowedTargets) > MaxNetworkTargets || !network.ExactAllowlist || !network.GuardRequired {
			return errors.New("allowlisted Docker network plan requires a managed default-deny guard")
		}
	default:
		return errors.New("docker container network mode is invalid")
	}
	return nil
}

type DockerContainerResourceSpec struct {
	NanoCPUs       int64
	MemoryBytes    int64
	PIDs           int
	MaxOutputBytes int64
}

func (resources DockerContainerResourceSpec) Validate() error {
	if resources.NanoCPUs < 1_000_000 || resources.NanoCPUs > int64(MaxCPUQuotaMillis)*1_000_000 ||
		resources.MemoryBytes < MinMemoryBytes || resources.MemoryBytes > MaxMemoryBytes ||
		resources.PIDs < 1 || resources.PIDs > MaxPIDs ||
		resources.MaxOutputBytes < 1 || resources.MaxOutputBytes > MaxCapturedOutputBytes {
		return errors.New("docker container resource plan is outside protocol bounds")
	}
	return nil
}

type DockerContainerTerminationSpec struct {
	TimeoutSeconds    int
	GracePeriodMillis int
	GracefulSignal    string
	ForcedSignal      string
	ExportAfterStop   bool
	RemoveAfterExport bool
}

func (termination DockerContainerTerminationSpec) Validate() error {
	if termination.TimeoutSeconds < 1 || termination.TimeoutSeconds > MaxTimeoutSeconds ||
		termination.GracePeriodMillis < 0 || termination.GracePeriodMillis > MaxCancellationGraceMS ||
		termination.GracefulSignal != DockerTerminationSignalGraceful ||
		termination.ForcedSignal != DockerTerminationSignalForced ||
		!termination.ExportAfterStop || !termination.RemoveAfterExport {
		return errors.New("docker container termination plan is invalid")
	}
	return nil
}

type DockerContainerLabel struct {
	Name  string
	Value string
}

func (label DockerContainerLabel) Validate() error {
	if validateBoundedText("Docker label name", label.Name, 128, false) != nil ||
		validateBoundedText("Docker label value", label.Value, 512, false) != nil ||
		!strings.HasPrefix(label.Name, "io.cyberagent.") {
		return errors.New("docker container label is invalid")
	}
	return nil
}

type DockerContainerSpec struct {
	ProtocolVersion            string
	ObservationID              string
	RunID                      string
	ExecutionID                string
	ImageDigest                string
	OSType                     string
	Architecture               string
	AuthorityFingerprint       string
	ManifestFingerprint        string
	InputArtifactDigest        string
	OutputPlanFingerprint      string
	Executable                 string
	Arguments                  []string
	WorkingDirectory           string
	User                       string
	ReadOnlyRootFS             bool
	NoNewPrivileges            bool
	DropAllCapabilities        bool
	InitEnabled                bool
	Mounts                     []DockerContainerMountSpec
	Environment                []DockerContainerEnvironmentSpec
	Network                    DockerContainerNetworkSpec
	Resources                  DockerContainerResourceSpec
	Termination                DockerContainerTerminationSpec
	InputArtifactCount         int
	OutputCount                int
	InputArtifactsReadOnly     bool
	SecretMountTarget          string
	SecretsEphemeral           bool
	SecretsMetadataExcluded    bool
	Labels                     []DockerContainerLabel
	ContainerName              string
	ReconcileBeforeCreate      bool
	RemoveOnRollback           bool
	CommandFingerprint         string
	MountPlanFingerprint       string
	NetworkPlanFingerprint     string
	SecretPlanFingerprint      string
	ContainerConfigFingerprint string
	ResourcePlanFingerprint    string
	TerminationPlanFingerprint string
	LabelPlanFingerprint       string
	OrphanPlanFingerprint      string
	SpecFingerprint            string
}

func CompileDockerContainerSpec(ctx context.Context, observation DockerObservation,
	manifest Manifest,
) (DockerContainerSpec, error) {
	if err := ctx.Err(); err != nil {
		return DockerContainerSpec{}, err
	}
	if err := observation.Validate(); err != nil {
		return DockerContainerSpec{}, fmt.Errorf("validate Docker observation: %w", err)
	}
	report := observation.Report
	if report.Status != DockerObservationStatusComplete || !report.ObservationComplete ||
		!report.ProductionObserved || report.ProductionVerified || report.BackendAvailable ||
		report.BackendEnabled || report.ExecutionAuthorized || report.ArtifactCommitAuthorized {
		return DockerContainerSpec{}, errors.New("docker container compilation requires a complete non-authorizing v53 observation")
	}
	normalized, err := NormalizeManifest(manifest)
	if err != nil {
		return DockerContainerSpec{}, err
	}
	manifestFingerprint, err := normalized.Fingerprint()
	if err != nil {
		return DockerContainerSpec{}, err
	}
	if normalized.Backend != BackendDocker || manifestFingerprint != observation.ManifestFingerprint ||
		report.OSType != "linux" || report.ImageOSType != "linux" ||
		report.Architecture != report.ImageArchitecture || !report.PidsLimitSupported {
		return DockerContainerSpec{}, errors.New("docker observation cannot satisfy the bounded Linux container compiler")
	}
	if report.NCPU > 0 && int64(normalized.Resources.CPUQuotaMillis) > int64(report.NCPU)*1000 {
		return DockerContainerSpec{}, errors.New("docker CPU plan exceeds observed daemon capacity")
	}
	if report.MemoryBytes > 0 && normalized.Resources.MemoryBytes > report.MemoryBytes {
		return DockerContainerSpec{}, errors.New("docker memory plan exceeds observed daemon capacity")
	}
	if normalized.WritableMountCount() != 1 {
		return DockerContainerSpec{}, errors.New("docker container compiler requires exactly one dedicated writable output mount")
	}

	spec := DockerContainerSpec{
		ProtocolVersion: DockerContainerSpecProtocolVersion,
		ObservationID:   observation.ID, RunID: observation.RunID, ExecutionID: observation.ExecutionID,
		ImageDigest: report.ImageDigest, OSType: report.ImageOSType,
		Architecture:         report.ImageArchitecture,
		AuthorityFingerprint: DockerContainerAuthorityFingerprint(observation),
		ManifestFingerprint:  manifestFingerprint, InputArtifactDigest: observation.InputArtifactDigest,
		OutputPlanFingerprint: observation.OutputPlanFingerprint,
		Executable:            normalized.Command.Executable,
		Arguments:             append([]string(nil), normalized.Command.Arguments...),
		WorkingDirectory:      normalized.Command.WorkingDirectory,
		User:                  DockerContainerFixedUser, ReadOnlyRootFS: true, NoNewPrivileges: true,
		DropAllCapabilities: true, InitEnabled: true,
		InputArtifactCount: len(normalized.InputArtifactIDs), InputArtifactsReadOnly: true,
		OutputCount:      normalized.OutputCount(),
		SecretsEphemeral: true, SecretsMetadataExcluded: true,
		ReconcileBeforeCreate: true, RemoveOnRollback: true,
		Resources: DockerContainerResourceSpec{
			NanoCPUs:    int64(normalized.Resources.CPUQuotaMillis) * 1_000_000,
			MemoryBytes: normalized.Resources.MemoryBytes, PIDs: normalized.Resources.PIDs,
			MaxOutputBytes: normalized.Resources.MaxOutputBytes,
		},
		Termination: DockerContainerTerminationSpec{
			TimeoutSeconds:    normalized.TimeoutSeconds,
			GracePeriodMillis: normalized.Cancellation.GracePeriodMillis,
			GracefulSignal:    DockerTerminationSignalGraceful, ForcedSignal: DockerTerminationSignalForced,
			ExportAfterStop: true, RemoveAfterExport: true,
		},
	}
	if normalized.SecretReferenceCount() > 0 {
		spec.SecretMountTarget = DockerSecretMountTarget
	}

	spec.Mounts = make([]DockerContainerMountSpec, len(normalized.Mounts))
	outputTarget := ""
	for index, mount := range normalized.Mounts {
		if err := ctx.Err(); err != nil {
			return DockerContainerSpec{}, err
		}
		compiled := DockerContainerMountSpec{
			Ordinal: index + 1, Source: mount.Source, Target: mount.Target, Access: mount.Access,
			Propagation:     DockerMountPropagationPrivate,
			DedicatedOutput: mount.Access == MountReadWrite,
			InputReadOnly:   mount.Access == MountReadOnly,
		}
		if compiled.DedicatedOutput {
			outputTarget = compiled.Target
		}
		spec.Mounts[index] = compiled
	}
	if outputTarget == "" || pathWithin(spec.WorkingDirectory, outputTarget) {
		return DockerContainerSpec{}, errors.New("docker working directory must remain outside the dedicated output mount")
	}
	for _, outputPath := range normalized.Output.Paths {
		if outputPath == outputTarget || !pathWithin(outputPath, outputTarget) {
			return DockerContainerSpec{}, errors.New("docker file output escaped the dedicated output mount")
		}
	}

	spec.Environment = make([]DockerContainerEnvironmentSpec, len(normalized.Environment))
	secretOrdinal := 0
	for index, binding := range normalized.Environment {
		compiled := DockerContainerEnvironmentSpec{Name: binding.Name, Source: binding.Source}
		if binding.Source == EnvironmentLiteral {
			compiled.LiteralValue = binding.Value
		} else {
			secretOrdinal++
			compiled.SecretReference = binding.Value
			compiled.SecretPath = fmt.Sprintf("%s/%03d", DockerSecretMountTarget, secretOrdinal)
		}
		spec.Environment[index] = compiled
	}

	spec.Network = DockerContainerNetworkSpec{
		Mode:           normalized.Network.Mode,
		AllowedTargets: append([]string(nil), normalized.Network.AllowedTargets...),
		DefaultDeny:    true,
	}
	if normalized.Network.Mode == "disabled" {
		spec.Network.Driver = DockerNetworkDriverNone
	} else {
		spec.Network.Driver = DockerNetworkDriverManagedEgress
		spec.Network.ExactAllowlist = true
		spec.Network.GuardRequired = true
	}

	spec.ContainerName = "cyberagent-" + spec.AuthorityFingerprint[:24]
	spec.Labels = []DockerContainerLabel{
		{Name: "io.cyberagent.authority", Value: spec.AuthorityFingerprint},
		{Name: "io.cyberagent.execution", Value: observation.ExecutionID},
		{Name: "io.cyberagent.managed", Value: "true"},
		{Name: "io.cyberagent.observation", Value: observation.ID},
		{Name: "io.cyberagent.protocol", Value: DockerContainerSpecProtocolVersion},
		{Name: "io.cyberagent.run", Value: observation.RunID},
	}
	sort.Slice(spec.Labels, func(i, j int) bool { return spec.Labels[i].Name < spec.Labels[j].Name })
	finalizeDockerContainerSpec(&spec)
	if err := spec.Validate(); err != nil {
		return DockerContainerSpec{}, err
	}
	return spec, nil
}

func DockerContainerAuthorityFingerprint(observation DockerObservation) string {
	return fingerprint("sandbox_docker_container_authority.v1", observation.ID,
		observation.EvidenceID, observation.OutputSimulationID, observation.PreflightID,
		observation.ExecutionID, observation.CandidateID, observation.PreparationID,
		observation.RunID, observation.MissionID, observation.WorkspaceID,
		observation.ManifestFingerprint, observation.AuthorizationFingerprint,
		observation.PolicyFingerprint, observation.MountBindingFingerprint,
		observation.InputArtifactDigest, observation.ThreatModelFingerprint,
		observation.OutputPlanFingerprint, observation.Report.ObservationFingerprint)
}

func finalizeDockerContainerSpec(spec *DockerContainerSpec) {
	spec.CommandFingerprint = dockerContainerCommandFingerprint(*spec)
	spec.MountPlanFingerprint = dockerContainerMountPlanFingerprint(*spec)
	spec.NetworkPlanFingerprint = dockerContainerNetworkPlanFingerprint(*spec)
	spec.SecretPlanFingerprint = dockerContainerSecretPlanFingerprint(*spec)
	spec.ContainerConfigFingerprint = dockerContainerConfigFingerprint(*spec)
	spec.ResourcePlanFingerprint = dockerContainerResourcePlanFingerprint(*spec)
	spec.TerminationPlanFingerprint = dockerContainerTerminationPlanFingerprint(*spec)
	spec.LabelPlanFingerprint = dockerContainerLabelPlanFingerprint(*spec)
	spec.OrphanPlanFingerprint = dockerContainerOrphanPlanFingerprint(*spec)
	spec.SpecFingerprint = dockerContainerSpecFingerprint(*spec)
}

func (spec DockerContainerSpec) Validate() error {
	for label, value := range map[string]string{
		"Docker spec observation id": spec.ObservationID, "Docker spec Run id": spec.RunID,
		"Docker spec execution id": spec.ExecutionID,
	} {
		if validateStoredIdentity(label, value) != nil {
			return errors.New("docker container specification identity is invalid")
		}
	}
	if spec.ProtocolVersion != DockerContainerSpecProtocolVersion ||
		!ValidOCIImageDigest(spec.ImageDigest) || spec.OSType != "linux" ||
		validateBoundedText("Docker architecture", spec.Architecture, 64, false) != nil ||
		!validDigest(spec.AuthorityFingerprint) || !validDigest(spec.ManifestFingerprint) ||
		!validDigest(spec.InputArtifactDigest) || !validDigest(spec.OutputPlanFingerprint) ||
		validateCommand(CommandSpec{Executable: spec.Executable, Arguments: spec.Arguments,
			WorkingDirectory: spec.WorkingDirectory}) != nil || spec.User != DockerContainerFixedUser ||
		!spec.ReadOnlyRootFS || !spec.NoNewPrivileges || !spec.DropAllCapabilities || !spec.InitEnabled ||
		len(spec.Mounts) < 1 || len(spec.Mounts) > MaxMounts ||
		spec.InputArtifactCount < 0 || spec.InputArtifactCount > MaxInputArtifacts ||
		spec.OutputCount < 1 || spec.OutputCount > MaxOutputPaths+2 ||
		!spec.InputArtifactsReadOnly || !spec.SecretsEphemeral || !spec.SecretsMetadataExcluded ||
		!spec.ReconcileBeforeCreate || !spec.RemoveOnRollback {
		return errors.New("docker container specification violates a fixed security invariant")
	}
	writable := 0
	outputTarget := ""
	workdirReadOnly := false
	for index, mount := range spec.Mounts {
		if mount.Ordinal != index+1 || mount.Validate() != nil {
			return errors.New("docker container mount sequence is invalid")
		}
		if mount.DedicatedOutput {
			writable++
			outputTarget = mount.Target
		} else if pathWithin(spec.WorkingDirectory, mount.Target) {
			workdirReadOnly = true
		}
	}
	if writable != 1 || outputTarget == "" || !workdirReadOnly ||
		pathWithin(spec.WorkingDirectory, outputTarget) {
		return errors.New("docker container specification must isolate one writable output mount")
	}
	secretCount := 0
	previousName := ""
	for _, binding := range spec.Environment {
		if binding.Validate() != nil || (previousName != "" && previousName >= binding.Name) {
			return errors.New("docker environment binding sequence is invalid")
		}
		previousName = binding.Name
		if binding.Source == EnvironmentSecretRef {
			secretCount++
			if binding.SecretPath != fmt.Sprintf("%s/%03d", DockerSecretMountTarget, secretCount) {
				return errors.New("docker secret path sequence is invalid")
			}
		}
	}
	if (secretCount == 0 && spec.SecretMountTarget != "") ||
		(secretCount > 0 && spec.SecretMountTarget != DockerSecretMountTarget) ||
		spec.Network.Validate() != nil || spec.Resources.Validate() != nil ||
		spec.Termination.Validate() != nil || len(spec.Labels) != 6 ||
		validateBoundedText("Docker container name", spec.ContainerName, 63, false) != nil ||
		!strings.HasPrefix(spec.ContainerName, "cyberagent-") {
		return errors.New("docker container configuration is invalid")
	}
	expectedLabels := map[string]string{
		"io.cyberagent.authority":   spec.AuthorityFingerprint,
		"io.cyberagent.execution":   spec.ExecutionID,
		"io.cyberagent.managed":     "true",
		"io.cyberagent.observation": spec.ObservationID,
		"io.cyberagent.protocol":    DockerContainerSpecProtocolVersion,
		"io.cyberagent.run":         spec.RunID,
	}
	if spec.ContainerName != "cyberagent-"+spec.AuthorityFingerprint[:24] {
		return errors.New("docker container name is not authority-derived")
	}
	previousName = ""
	for _, label := range spec.Labels {
		if label.Validate() != nil || (previousName != "" && previousName >= label.Name) {
			return errors.New("docker label sequence is invalid")
		}
		if expectedLabels[label.Name] != label.Value {
			return errors.New("docker label authority binding is invalid")
		}
		previousName = label.Name
	}
	checks := []struct{ actual, expected string }{
		{spec.CommandFingerprint, dockerContainerCommandFingerprint(spec)},
		{spec.MountPlanFingerprint, dockerContainerMountPlanFingerprint(spec)},
		{spec.NetworkPlanFingerprint, dockerContainerNetworkPlanFingerprint(spec)},
		{spec.SecretPlanFingerprint, dockerContainerSecretPlanFingerprint(spec)},
		{spec.ContainerConfigFingerprint, dockerContainerConfigFingerprint(spec)},
		{spec.ResourcePlanFingerprint, dockerContainerResourcePlanFingerprint(spec)},
		{spec.TerminationPlanFingerprint, dockerContainerTerminationPlanFingerprint(spec)},
		{spec.LabelPlanFingerprint, dockerContainerLabelPlanFingerprint(spec)},
		{spec.OrphanPlanFingerprint, dockerContainerOrphanPlanFingerprint(spec)},
		{spec.SpecFingerprint, dockerContainerSpecFingerprint(spec)},
	}
	for _, check := range checks {
		if !validDigest(check.actual) || check.actual != check.expected {
			return errors.New("docker container specification fingerprint is invalid")
		}
	}
	return nil
}

func dockerContainerCommandFingerprint(spec DockerContainerSpec) string {
	parts := []string{"sandbox_docker_command.v1", spec.Executable, spec.WorkingDirectory,
		strconv.Itoa(len(spec.Arguments))}
	parts = append(parts, spec.Arguments...)
	return fingerprint(parts...)
}

func dockerContainerMountPlanFingerprint(spec DockerContainerSpec) string {
	parts := []string{"sandbox_docker_mount_plan.v1", strconv.Itoa(len(spec.Mounts)),
		strconv.Itoa(spec.InputArtifactCount), spec.InputArtifactDigest,
		DockerInputArtifactMountTarget, strconv.FormatBool(spec.InputArtifactsReadOnly)}
	for _, mount := range spec.Mounts {
		parts = append(parts, strconv.Itoa(mount.Ordinal), mount.Source, mount.Target,
			string(mount.Access), mount.Propagation, strconv.FormatBool(mount.DedicatedOutput),
			strconv.FormatBool(mount.InputReadOnly))
	}
	return fingerprint(parts...)
}

func dockerContainerNetworkPlanFingerprint(spec DockerContainerSpec) string {
	parts := []string{"sandbox_docker_network_plan.v1", spec.Network.Mode,
		spec.Network.Driver, strconv.FormatBool(spec.Network.DefaultDeny),
		strconv.FormatBool(spec.Network.ExactAllowlist), strconv.FormatBool(spec.Network.GuardRequired),
		strconv.Itoa(len(spec.Network.AllowedTargets))}
	parts = append(parts, spec.Network.AllowedTargets...)
	return fingerprint(parts...)
}

func dockerContainerSecretPlanFingerprint(spec DockerContainerSpec) string {
	parts := []string{"sandbox_docker_secret_plan.v1", DockerSecretDeliveryEphemeralPrivate,
		spec.SecretMountTarget, strconv.FormatBool(spec.SecretsEphemeral),
		strconv.FormatBool(spec.SecretsMetadataExcluded), strconv.Itoa(len(spec.Environment))}
	for _, binding := range spec.Environment {
		parts = append(parts, binding.Name, string(binding.Source), binding.LiteralValue,
			binding.SecretReference, binding.SecretPath)
	}
	return fingerprint(parts...)
}

func dockerContainerConfigFingerprint(spec DockerContainerSpec) string {
	return fingerprint("sandbox_docker_container_config.v1", spec.ImageDigest, spec.OSType,
		spec.Architecture, spec.User, strconv.FormatBool(spec.ReadOnlyRootFS),
		strconv.FormatBool(spec.NoNewPrivileges), strconv.FormatBool(spec.DropAllCapabilities),
		strconv.FormatBool(spec.InitEnabled), spec.CommandFingerprint)
}

func dockerContainerResourcePlanFingerprint(spec DockerContainerSpec) string {
	return fingerprint("sandbox_docker_resource_plan.v1",
		strconv.FormatInt(spec.Resources.NanoCPUs, 10),
		strconv.FormatInt(spec.Resources.MemoryBytes, 10), strconv.Itoa(spec.Resources.PIDs),
		strconv.FormatInt(spec.Resources.MaxOutputBytes, 10))
}

func dockerContainerTerminationPlanFingerprint(spec DockerContainerSpec) string {
	return fingerprint("sandbox_docker_termination_plan.v1",
		strconv.Itoa(spec.Termination.TimeoutSeconds), strconv.Itoa(spec.Termination.GracePeriodMillis),
		spec.Termination.GracefulSignal, spec.Termination.ForcedSignal,
		strconv.FormatBool(spec.Termination.ExportAfterStop),
		strconv.FormatBool(spec.Termination.RemoveAfterExport))
}

func dockerContainerLabelPlanFingerprint(spec DockerContainerSpec) string {
	parts := []string{"sandbox_docker_label_plan.v1", spec.ContainerName,
		strconv.Itoa(len(spec.Labels))}
	for _, label := range spec.Labels {
		parts = append(parts, label.Name, label.Value)
	}
	return fingerprint(parts...)
}

func dockerContainerOrphanPlanFingerprint(spec DockerContainerSpec) string {
	return fingerprint("sandbox_docker_orphan_plan.v1", spec.ContainerName,
		spec.LabelPlanFingerprint, strconv.FormatBool(spec.ReconcileBeforeCreate),
		strconv.FormatBool(spec.RemoveOnRollback), strconv.FormatBool(spec.Termination.RemoveAfterExport))
}

func dockerContainerSpecFingerprint(spec DockerContainerSpec) string {
	return fingerprint(DockerContainerSpecProtocolVersion, spec.ObservationID, spec.RunID,
		spec.ExecutionID, spec.AuthorityFingerprint, spec.ManifestFingerprint,
		spec.InputArtifactDigest, spec.OutputPlanFingerprint, spec.CommandFingerprint,
		strconv.Itoa(spec.OutputCount),
		spec.MountPlanFingerprint, spec.NetworkPlanFingerprint, spec.SecretPlanFingerprint,
		spec.ContainerConfigFingerprint, spec.ResourcePlanFingerprint,
		spec.TerminationPlanFingerprint, spec.LabelPlanFingerprint,
		spec.OrphanPlanFingerprint)
}

type DockerWriteStep struct {
	Ordinal           int
	Name              string
	State             string
	StepDigest        string
	Simulated         bool
	ProductionApplied bool
}

func (step DockerWriteStep) Validate(specFingerprint string) error {
	if step.Ordinal < 1 || step.Ordinal > len(dockerWriteStepNames) ||
		step.Name != dockerWriteStepNames[step.Ordinal-1] ||
		step.State != DockerWriteStepStateCommitted || !step.Simulated ||
		step.ProductionApplied || step.StepDigest != fingerprint("sandbox_docker_fake_write_step.v1",
		specFingerprint, strconv.Itoa(step.Ordinal), step.Name, step.State) {
		return errors.New("docker fake write step is invalid")
	}
	return nil
}

type DockerWriteSimulation struct {
	ProtocolVersion          string
	Source                   string
	Status                   string
	SpecFingerprint          string
	TransactionFingerprint   string
	StepCount                int
	StagedStepCount          int
	CommittedStepCount       int
	RollbackStepCount        int
	DaemonWriteCount         int
	BackendTouched           bool
	SimulationOnly           bool
	ProductionSubmitted      bool
	ProductionVerified       bool
	BackendEnabled           bool
	ExecutionAuthorized      bool
	ArtifactCommitAuthorized bool
	Steps                    []DockerWriteStep
}

func (simulation DockerWriteSimulation) Validate() error {
	if simulation.ProtocolVersion != DockerWriteTransactionProtocolVersion ||
		simulation.Source != DockerWriteTransactionSourceFake ||
		simulation.Status != DockerWriteTransactionStatusCommitted ||
		!validDigest(simulation.SpecFingerprint) ||
		simulation.StepCount != len(dockerWriteStepNames) ||
		simulation.StepCount != len(simulation.Steps) ||
		simulation.StagedStepCount != simulation.StepCount ||
		simulation.CommittedStepCount != simulation.StepCount || simulation.RollbackStepCount != 0 ||
		simulation.DaemonWriteCount != 0 || simulation.BackendTouched || !simulation.SimulationOnly ||
		simulation.ProductionSubmitted || simulation.ProductionVerified || simulation.BackendEnabled ||
		simulation.ExecutionAuthorized || simulation.ArtifactCommitAuthorized {
		return errors.New("docker write transaction must remain a fully committed in-memory simulation")
	}
	for index, step := range simulation.Steps {
		if step.Ordinal != index+1 || step.Validate(simulation.SpecFingerprint) != nil {
			return errors.New("docker write transaction step order is invalid")
		}
	}
	if simulation.TransactionFingerprint != dockerWriteTransactionFingerprint(simulation) {
		return errors.New("docker write transaction fingerprint is invalid")
	}
	return nil
}

func dockerWriteTransactionFingerprint(simulation DockerWriteSimulation) string {
	parts := []string{DockerWriteTransactionProtocolVersion, simulation.Source,
		simulation.Status, simulation.SpecFingerprint, strconv.Itoa(simulation.StepCount),
		strconv.Itoa(simulation.StagedStepCount), strconv.Itoa(simulation.CommittedStepCount),
		strconv.Itoa(simulation.RollbackStepCount), strconv.Itoa(simulation.DaemonWriteCount),
		strconv.FormatBool(simulation.BackendTouched), strconv.FormatBool(simulation.SimulationOnly),
		strconv.FormatBool(simulation.ProductionSubmitted), strconv.FormatBool(simulation.ProductionVerified),
		strconv.FormatBool(simulation.BackendEnabled), strconv.FormatBool(simulation.ExecutionAuthorized),
		strconv.FormatBool(simulation.ArtifactCommitAuthorized)}
	for _, step := range simulation.Steps {
		parts = append(parts, step.StepDigest)
	}
	return fingerprint(parts...)
}

type DockerContainerTransactionHarness interface {
	Simulate(ctx context.Context, spec DockerContainerSpec) (DockerWriteSimulation, error)
}

type InMemoryDockerWriteTransaction struct {
	FailAtOrdinal  int
	CrashAtOrdinal int
	beforeStep     func(int)
	mu             sync.Mutex
	committed      []DockerWriteSimulation
}

func NewInMemoryDockerWriteTransaction() *InMemoryDockerWriteTransaction {
	return &InMemoryDockerWriteTransaction{}
}

func (transaction *InMemoryDockerWriteTransaction) Simulate(ctx context.Context,
	spec DockerContainerSpec,
) (DockerWriteSimulation, error) {
	if transaction == nil {
		return DockerWriteSimulation{}, errors.New("docker fake write transaction is required")
	}
	if err := ctx.Err(); err != nil {
		return DockerWriteSimulation{}, err
	}
	if err := spec.Validate(); err != nil {
		return DockerWriteSimulation{}, err
	}
	staged := make([]DockerWriteStep, 0, len(dockerWriteStepNames))
	for index, name := range dockerWriteStepNames {
		ordinal := index + 1
		if transaction.beforeStep != nil {
			transaction.beforeStep(ordinal)
		}
		if err := ctx.Err(); err != nil {
			return DockerWriteSimulation{}, err
		}
		if transaction.CrashAtOrdinal == ordinal {
			return DockerWriteSimulation{}, fmt.Errorf("fake Docker writer crashed at step %d", ordinal)
		}
		if transaction.FailAtOrdinal == ordinal {
			return DockerWriteSimulation{}, fmt.Errorf("fake Docker writer failed at step %d", ordinal)
		}
		step := DockerWriteStep{Ordinal: ordinal, Name: name,
			State: DockerWriteStepStateCommitted, Simulated: true}
		step.StepDigest = fingerprint("sandbox_docker_fake_write_step.v1",
			spec.SpecFingerprint, strconv.Itoa(step.Ordinal), step.Name, step.State)
		staged = append(staged, step)
	}
	result := DockerWriteSimulation{
		ProtocolVersion: DockerWriteTransactionProtocolVersion,
		Source:          DockerWriteTransactionSourceFake, Status: DockerWriteTransactionStatusCommitted,
		SpecFingerprint: spec.SpecFingerprint, StepCount: len(staged),
		StagedStepCount: len(staged), CommittedStepCount: len(staged),
		SimulationOnly: true, Steps: append([]DockerWriteStep(nil), staged...),
	}
	result.TransactionFingerprint = dockerWriteTransactionFingerprint(result)
	if err := result.Validate(); err != nil {
		return DockerWriteSimulation{}, err
	}
	transaction.mu.Lock()
	transaction.committed = append(transaction.committed, result)
	transaction.mu.Unlock()
	return result, nil
}

func (transaction *InMemoryDockerWriteTransaction) Snapshot() []DockerWriteSimulation {
	if transaction == nil {
		return nil
	}
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	snapshot := make([]DockerWriteSimulation, len(transaction.committed))
	for index, simulation := range transaction.committed {
		snapshot[index] = simulation
		snapshot[index].Steps = append([]DockerWriteStep(nil), simulation.Steps...)
	}
	return snapshot
}

type DockerContainerControl struct {
	Ordinal       int
	Name          string
	State         string
	ControlDigest string
	Planned       bool
	Applied       bool
	Verified      bool
}

func (control DockerContainerControl) Validate(authorityFingerprint, subject string) error {
	checks := RequiredBackendChecks()
	if control.Ordinal < 1 || control.Ordinal > len(checks) ||
		control.Name != checks[control.Ordinal-1].Name ||
		control.State != DockerContainerControlStateCompiled || !control.Planned ||
		control.Applied || control.Verified ||
		control.ControlDigest != fingerprint("sandbox_docker_container_control.v1",
			authorityFingerprint, control.Name, subject) {
		return errors.New("docker container control plan is invalid")
	}
	return nil
}

type DockerContainerPlan struct {
	ID                         string
	ObservationID              string
	EvidenceID                 string
	OutputSimulationID         string
	PreflightID                string
	ExecutionID                string
	CandidateID                string
	PreparationID              string
	RunID                      string
	MissionID                  string
	WorkspaceID                string
	ProtocolVersion            string
	Source                     string
	TrustClass                 string
	Status                     string
	ManifestFingerprint        string
	AuthorizationFingerprint   string
	PolicyFingerprint          string
	MountBindingFingerprint    string
	InputArtifactDigest        string
	ThreatModelFingerprint     string
	OutputPlanFingerprint      string
	ObservationFingerprint     string
	AuthorityFingerprint       string
	ImageDigest                string
	OSType                     string
	Architecture               string
	ContainerUser              string
	SpecFingerprint            string
	CommandFingerprint         string
	MountPlanFingerprint       string
	NetworkPlanFingerprint     string
	SecretPlanFingerprint      string
	ContainerConfigFingerprint string
	ResourcePlanFingerprint    string
	TerminationPlanFingerprint string
	LabelPlanFingerprint       string
	OrphanPlanFingerprint      string
	ContainerNameFingerprint   string
	PlanFingerprint            string
	ReadOnlyRootFS             bool
	NoNewPrivileges            bool
	DropAllCapabilities        bool
	InitEnabled                bool
	MountCount                 int
	ReadOnlyMountCount         int
	WritableMountCount         int
	DedicatedOutputMounts      int
	PrivatePropagationMounts   int
	EnvironmentCount           int
	SecretReferenceCount       int
	InputArtifactCount         int
	OutputCount                int
	NetworkMode                string
	NetworkTargetCount         int
	NetworkDefaultDeny         bool
	ExactNetworkAllowlist      bool
	NetworkGuardRequired       bool
	NanoCPUs                   int64
	MemoryBytes                int64
	PIDs                       int
	MaxOutputBytes             int64
	TimeoutSeconds             int
	GracePeriodMillis          int
	SecretsEphemeral           bool
	SecretsMetadataExcluded    bool
	LabelCount                 int
	ReconcileBeforeCreate      bool
	RemoveOnRollback           bool
	ExportAfterStop            bool
	RemoveAfterExport          bool
	Controls                   []DockerContainerControl
	Transaction                DockerWriteSimulation
	SimulationOnly             bool
	ProductionSubmitted        bool
	ProductionVerified         bool
	BackendAvailable           bool
	BackendEnabled             bool
	ExecutionAuthorized        bool
	ArtifactCommitAuthorized   bool
	RequestedBy                string
	CreatedAt                  time.Time
	Replayed                   bool
}

func NewDockerContainerPlan(id string, observation DockerObservation, spec DockerContainerSpec,
	transaction DockerWriteSimulation, requestedBy string, createdAt time.Time,
) (DockerContainerPlan, error) {
	if err := observation.Validate(); err != nil {
		return DockerContainerPlan{}, err
	}
	if err := spec.Validate(); err != nil {
		return DockerContainerPlan{}, err
	}
	if spec.ObservationID != observation.ID || spec.RunID != observation.RunID ||
		spec.ExecutionID != observation.ExecutionID ||
		spec.AuthorityFingerprint != DockerContainerAuthorityFingerprint(observation) ||
		spec.ManifestFingerprint != observation.ManifestFingerprint ||
		spec.InputArtifactDigest != observation.InputArtifactDigest ||
		spec.OutputPlanFingerprint != observation.OutputPlanFingerprint ||
		spec.ImageDigest != observation.Report.ImageDigest || requestedBy != observation.RequestedBy {
		return DockerContainerPlan{}, errors.New("docker container specification does not bind the observation authority")
	}
	if err := transaction.Validate(); err != nil || transaction.SpecFingerprint != spec.SpecFingerprint {
		return DockerContainerPlan{}, errors.New("docker fake transaction does not bind the compiled specification")
	}
	readOnlyMounts := 0
	for _, mount := range spec.Mounts {
		if mount.InputReadOnly {
			readOnlyMounts++
		}
	}
	secretCount := 0
	for _, binding := range spec.Environment {
		if binding.Source == EnvironmentSecretRef {
			secretCount++
		}
	}
	plan := DockerContainerPlan{
		ID: id, ObservationID: observation.ID, EvidenceID: observation.EvidenceID,
		OutputSimulationID: observation.OutputSimulationID, PreflightID: observation.PreflightID,
		ExecutionID: observation.ExecutionID, CandidateID: observation.CandidateID,
		PreparationID: observation.PreparationID, RunID: observation.RunID,
		MissionID: observation.MissionID, WorkspaceID: observation.WorkspaceID,
		ProtocolVersion: DockerContainerPlanProtocolVersion,
		Source:          DockerContainerPlanSourceCompiler, TrustClass: DockerContainerPlanTrustSimulation,
		Status:                   DockerContainerPlanStatusCompiled,
		ManifestFingerprint:      observation.ManifestFingerprint,
		AuthorizationFingerprint: observation.AuthorizationFingerprint,
		PolicyFingerprint:        observation.PolicyFingerprint,
		MountBindingFingerprint:  observation.MountBindingFingerprint,
		InputArtifactDigest:      observation.InputArtifactDigest,
		ThreatModelFingerprint:   observation.ThreatModelFingerprint,
		OutputPlanFingerprint:    observation.OutputPlanFingerprint,
		ObservationFingerprint:   observation.Report.ObservationFingerprint,
		AuthorityFingerprint:     spec.AuthorityFingerprint, ImageDigest: spec.ImageDigest,
		OSType: spec.OSType, Architecture: spec.Architecture, ContainerUser: spec.User,
		SpecFingerprint: spec.SpecFingerprint, CommandFingerprint: spec.CommandFingerprint,
		MountPlanFingerprint:       spec.MountPlanFingerprint,
		NetworkPlanFingerprint:     spec.NetworkPlanFingerprint,
		SecretPlanFingerprint:      spec.SecretPlanFingerprint,
		ContainerConfigFingerprint: spec.ContainerConfigFingerprint,
		ResourcePlanFingerprint:    spec.ResourcePlanFingerprint,
		TerminationPlanFingerprint: spec.TerminationPlanFingerprint,
		LabelPlanFingerprint:       spec.LabelPlanFingerprint,
		OrphanPlanFingerprint:      spec.OrphanPlanFingerprint,
		ContainerNameFingerprint:   fingerprint("sandbox_docker_container_name.v1", spec.ContainerName),
		ReadOnlyRootFS:             spec.ReadOnlyRootFS, NoNewPrivileges: spec.NoNewPrivileges,
		DropAllCapabilities: spec.DropAllCapabilities, InitEnabled: spec.InitEnabled,
		MountCount: len(spec.Mounts), ReadOnlyMountCount: readOnlyMounts,
		WritableMountCount:    len(spec.Mounts) - readOnlyMounts,
		DedicatedOutputMounts: 1, PrivatePropagationMounts: len(spec.Mounts),
		EnvironmentCount: len(spec.Environment), SecretReferenceCount: secretCount,
		InputArtifactCount: spec.InputArtifactCount,
		NetworkMode:        spec.Network.Mode, NetworkTargetCount: len(spec.Network.AllowedTargets),
		NetworkDefaultDeny:    spec.Network.DefaultDeny,
		ExactNetworkAllowlist: spec.Network.ExactAllowlist,
		NetworkGuardRequired:  spec.Network.GuardRequired,
		NanoCPUs:              spec.Resources.NanoCPUs, MemoryBytes: spec.Resources.MemoryBytes,
		PIDs: spec.Resources.PIDs, MaxOutputBytes: spec.Resources.MaxOutputBytes,
		TimeoutSeconds:          spec.Termination.TimeoutSeconds,
		GracePeriodMillis:       spec.Termination.GracePeriodMillis,
		SecretsEphemeral:        spec.SecretsEphemeral,
		SecretsMetadataExcluded: spec.SecretsMetadataExcluded,
		LabelCount:              len(spec.Labels), ReconcileBeforeCreate: spec.ReconcileBeforeCreate,
		RemoveOnRollback: spec.RemoveOnRollback, ExportAfterStop: spec.Termination.ExportAfterStop,
		RemoveAfterExport: spec.Termination.RemoveAfterExport,
		Transaction:       transaction, SimulationOnly: true,
		RequestedBy: requestedBy, CreatedAt: createdAt,
	}
	plan.OutputCount = spec.OutputCount
	plan.Controls = dockerContainerControls(plan)
	plan.PlanFingerprint = dockerContainerPlanFingerprint(plan)
	return plan, plan.Validate()
}

func (plan DockerContainerPlan) Validate() error {
	for label, value := range map[string]string{
		"Docker plan id": plan.ID, "Docker plan observation id": plan.ObservationID,
		"Docker plan evidence id":          plan.EvidenceID,
		"Docker plan output simulation id": plan.OutputSimulationID,
		"Docker plan preflight id":         plan.PreflightID,
		"Docker plan execution id":         plan.ExecutionID, "Docker plan candidate id": plan.CandidateID,
		"Docker plan preparation id": plan.PreparationID, "Docker plan Run id": plan.RunID,
		"Docker plan Mission id": plan.MissionID, "Docker plan workspace id": plan.WorkspaceID,
		"Docker plan requester": plan.RequestedBy,
	} {
		if validateStoredIdentity(label, value) != nil {
			return errors.New("docker container plan identity is invalid")
		}
	}
	if plan.ProtocolVersion != DockerContainerPlanProtocolVersion ||
		plan.Source != DockerContainerPlanSourceCompiler ||
		plan.TrustClass != DockerContainerPlanTrustSimulation ||
		plan.Status != DockerContainerPlanStatusCompiled || !ValidOCIImageDigest(plan.ImageDigest) ||
		plan.OSType != "linux" || validateBoundedText("Docker plan architecture", plan.Architecture, 64, false) != nil ||
		plan.ContainerUser != DockerContainerFixedUser || !plan.ReadOnlyRootFS ||
		!plan.NoNewPrivileges || !plan.DropAllCapabilities || !plan.InitEnabled ||
		plan.MountCount < 1 || plan.MountCount > MaxMounts ||
		plan.ReadOnlyMountCount != plan.MountCount-1 || plan.WritableMountCount != 1 ||
		plan.DedicatedOutputMounts != 1 || plan.PrivatePropagationMounts != plan.MountCount ||
		plan.EnvironmentCount < 0 || plan.EnvironmentCount > MaxEnvironmentBindings ||
		plan.SecretReferenceCount < 0 || plan.SecretReferenceCount > plan.EnvironmentCount ||
		plan.InputArtifactCount < 0 || plan.InputArtifactCount > MaxInputArtifacts ||
		plan.OutputCount < 1 || plan.OutputCount > MaxOutputPaths+2 || !plan.NetworkDefaultDeny ||
		plan.NetworkTargetCount < 0 || plan.NetworkTargetCount > MaxNetworkTargets ||
		plan.ResourcesInvalid() || plan.TimeoutSeconds < 1 || plan.TimeoutSeconds > MaxTimeoutSeconds ||
		plan.GracePeriodMillis < 0 || plan.GracePeriodMillis > MaxCancellationGraceMS ||
		!plan.SecretsEphemeral || !plan.SecretsMetadataExcluded || plan.LabelCount != 6 ||
		!plan.ReconcileBeforeCreate || !plan.RemoveOnRollback || !plan.ExportAfterStop ||
		!plan.RemoveAfterExport || !plan.SimulationOnly || plan.ProductionSubmitted ||
		plan.ProductionVerified || plan.BackendAvailable || plan.BackendEnabled ||
		plan.ExecutionAuthorized || plan.ArtifactCommitAuthorized || plan.CreatedAt.IsZero() {
		return errors.New("docker container plan violates a fixed non-authorizing invariant")
	}
	if (plan.NetworkMode == "disabled" && (plan.NetworkTargetCount != 0 ||
		plan.ExactNetworkAllowlist || plan.NetworkGuardRequired)) ||
		(plan.NetworkMode == "allowlist" && (plan.NetworkTargetCount < 1 ||
			!plan.ExactNetworkAllowlist || !plan.NetworkGuardRequired)) ||
		(plan.NetworkMode != "disabled" && plan.NetworkMode != "allowlist") {
		return errors.New("docker container plan network summary is invalid")
	}
	for _, value := range []string{plan.ManifestFingerprint, plan.AuthorizationFingerprint,
		plan.PolicyFingerprint, plan.MountBindingFingerprint, plan.InputArtifactDigest,
		plan.ThreatModelFingerprint, plan.OutputPlanFingerprint, plan.ObservationFingerprint,
		plan.AuthorityFingerprint, plan.SpecFingerprint, plan.CommandFingerprint,
		plan.MountPlanFingerprint, plan.NetworkPlanFingerprint, plan.SecretPlanFingerprint,
		plan.ContainerConfigFingerprint, plan.ResourcePlanFingerprint,
		plan.TerminationPlanFingerprint, plan.LabelPlanFingerprint,
		plan.OrphanPlanFingerprint, plan.ContainerNameFingerprint, plan.PlanFingerprint} {
		if !validDigest(value) {
			return errors.New("docker container plan fingerprint is invalid")
		}
	}
	if err := plan.Transaction.Validate(); err != nil ||
		plan.Transaction.SpecFingerprint != plan.SpecFingerprint ||
		len(plan.Controls) != MaxDockerContainerControls {
		return errors.New("docker container plan transaction or control set is invalid")
	}
	for index, control := range plan.Controls {
		if control.Ordinal != index+1 || control.Validate(plan.AuthorityFingerprint,
			dockerContainerControlSubject(plan, control.Name)) != nil {
			return errors.New("docker container plan control sequence is invalid")
		}
	}
	if plan.PlanFingerprint != dockerContainerPlanFingerprint(plan) {
		return errors.New("docker container plan aggregate fingerprint is invalid")
	}
	return nil
}

func (plan DockerContainerPlan) ResourcesInvalid() bool {
	return DockerContainerResourceSpec{NanoCPUs: plan.NanoCPUs, MemoryBytes: plan.MemoryBytes,
		PIDs: plan.PIDs, MaxOutputBytes: plan.MaxOutputBytes}.Validate() != nil
}

func dockerContainerControls(plan DockerContainerPlan) []DockerContainerControl {
	checks := RequiredBackendChecks()
	controls := make([]DockerContainerControl, len(checks))
	for index, check := range checks {
		subject := dockerContainerControlSubject(plan, check.Name)
		controls[index] = DockerContainerControl{
			Ordinal: check.Ordinal, Name: check.Name, State: DockerContainerControlStateCompiled,
			ControlDigest: fingerprint("sandbox_docker_container_control.v1",
				plan.AuthorityFingerprint, check.Name, subject), Planned: true,
		}
	}
	return controls
}

func dockerContainerControlSubject(plan DockerContainerPlan, name string) string {
	switch name {
	case "host_path_isolation", "mount_propagation_private", "read_only_rootfs",
		"read_only_inputs", "dedicated_writable_output":
		return plan.MountPlanFingerprint
	case "network_default_deny", "exact_network_allowlist":
		return plan.NetworkPlanFingerprint
	case "ephemeral_secret_materialization":
		return plan.SecretPlanFingerprint
	case "non_root_container_identity":
		return plan.ContainerConfigFingerprint
	case "cpu_memory_pid_limits":
		return plan.ResourcePlanFingerprint
	case "wall_clock_timeout", "graceful_then_forced_kill":
		return plan.TerminationPlanFingerprint
	case "orphan_reconciliation":
		return plan.OrphanPlanFingerprint
	case "output_regular_file_only", "output_symlink_special_rejection":
		return plan.OutputPlanFingerprint
	case "atomic_output_artifact_commit":
		return plan.Transaction.TransactionFingerprint
	default:
		return plan.SpecFingerprint
	}
}

func dockerContainerPlanFingerprint(plan DockerContainerPlan) string {
	parts := []string{DockerContainerPlanProtocolVersion, plan.Source, plan.TrustClass,
		plan.Status, plan.ObservationID, plan.EvidenceID, plan.OutputSimulationID,
		plan.PreflightID, plan.ExecutionID, plan.CandidateID, plan.PreparationID,
		plan.RunID, plan.MissionID, plan.WorkspaceID, plan.ManifestFingerprint,
		plan.AuthorizationFingerprint, plan.PolicyFingerprint, plan.MountBindingFingerprint,
		plan.InputArtifactDigest, plan.ThreatModelFingerprint, plan.OutputPlanFingerprint,
		plan.ObservationFingerprint, plan.AuthorityFingerprint, plan.ImageDigest,
		plan.OSType, plan.Architecture, plan.ContainerUser, plan.SpecFingerprint,
		plan.CommandFingerprint, plan.MountPlanFingerprint, plan.NetworkPlanFingerprint,
		plan.SecretPlanFingerprint, plan.ContainerConfigFingerprint,
		plan.ResourcePlanFingerprint, plan.TerminationPlanFingerprint,
		plan.LabelPlanFingerprint, plan.OrphanPlanFingerprint, plan.ContainerNameFingerprint,
		strconv.FormatBool(plan.ReadOnlyRootFS), strconv.FormatBool(plan.NoNewPrivileges),
		strconv.FormatBool(plan.DropAllCapabilities), strconv.FormatBool(plan.InitEnabled),
		strconv.Itoa(plan.MountCount), strconv.Itoa(plan.ReadOnlyMountCount),
		strconv.Itoa(plan.WritableMountCount), strconv.Itoa(plan.DedicatedOutputMounts),
		strconv.Itoa(plan.PrivatePropagationMounts), strconv.Itoa(plan.EnvironmentCount),
		strconv.Itoa(plan.SecretReferenceCount), strconv.Itoa(plan.InputArtifactCount),
		strconv.Itoa(plan.OutputCount), plan.NetworkMode, strconv.Itoa(plan.NetworkTargetCount),
		strconv.FormatBool(plan.NetworkDefaultDeny), strconv.FormatBool(plan.ExactNetworkAllowlist),
		strconv.FormatBool(plan.NetworkGuardRequired), strconv.FormatInt(plan.NanoCPUs, 10),
		strconv.FormatInt(plan.MemoryBytes, 10), strconv.Itoa(plan.PIDs),
		strconv.FormatInt(plan.MaxOutputBytes, 10), strconv.Itoa(plan.TimeoutSeconds),
		strconv.Itoa(plan.GracePeriodMillis), strconv.FormatBool(plan.SecretsEphemeral),
		strconv.FormatBool(plan.SecretsMetadataExcluded), strconv.Itoa(plan.LabelCount),
		strconv.FormatBool(plan.ReconcileBeforeCreate), strconv.FormatBool(plan.RemoveOnRollback),
		strconv.FormatBool(plan.ExportAfterStop), strconv.FormatBool(plan.RemoveAfterExport),
		plan.Transaction.TransactionFingerprint, strconv.Itoa(len(plan.Controls)),
		strconv.FormatBool(plan.SimulationOnly), strconv.FormatBool(plan.ProductionSubmitted),
		strconv.FormatBool(plan.ProductionVerified), strconv.FormatBool(plan.BackendAvailable),
		strconv.FormatBool(plan.BackendEnabled), strconv.FormatBool(plan.ExecutionAuthorized),
		strconv.FormatBool(plan.ArtifactCommitAuthorized), plan.RequestedBy}
	for _, control := range plan.Controls {
		parts = append(parts, control.ControlDigest)
	}
	return fingerprint(parts...)
}

type DockerContainerPlanOperation struct {
	KeyDigest          string
	RequestFingerprint string
	PlanID             string
	ObservationID      string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (operation DockerContainerPlanOperation) Validate() error {
	for label, value := range map[string]string{
		"Docker plan operation plan id":        operation.PlanID,
		"Docker plan operation observation id": operation.ObservationID,
		"Docker plan operation Run id":         operation.RunID,
		"Docker plan operation requester":      operation.RequestedBy,
	} {
		if validateStoredIdentity(label, value) != nil {
			return errors.New("docker container plan operation identity is invalid")
		}
	}
	if !validDigest(operation.KeyDigest) || !validDigest(operation.RequestFingerprint) ||
		operation.CreatedAt.IsZero() {
		return errors.New("docker container plan operation is invalid")
	}
	return nil
}

func DockerContainerPlanRequestFingerprint(plan DockerContainerPlan) string {
	return fingerprint("sandbox_docker_container_plan_request.v1", plan.ObservationID,
		plan.ManifestFingerprint, plan.AuthorityFingerprint, plan.SpecFingerprint,
		plan.Transaction.TransactionFingerprint, plan.PlanFingerprint, plan.RequestedBy)
}
