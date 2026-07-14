package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	PreflightProtocolVersion         = "sandbox_preflight.v1"
	BackendHandshakeProtocolVersion  = "sandbox_backend_handshake.v1"
	ContainerIdentityProtocolVersion = "sandbox_container_identity.v1"
	OutputExportProtocolVersion      = "sandbox_output_export_plan.v1"
	PreflightStatusBackendDisabled   = "backend_disabled"
	DisabledBackendInspectorName     = "disabled"
	BackendCheckEvidenceNotProbed    = "not_probed"
	MaxBackendChecks                 = 16

	OutputKindStdout = "stdout"
	OutputKindStderr = "stderr"
	OutputKindFile   = "file"

	OutputPartialFailureAllOrNothing = "all_or_nothing"
	OutputTruncationAggregateHardCap = "aggregate_hard_limit"
	OutputMIMEDetectAndValidate      = "detect_and_validate"
	OutputFileRegularNoLinks         = "regular_file_no_symlink_or_special"
	OutputRestartReconcile           = "reconcile_before_retry"
)

var requiredBackendCheckNames = [...]string{
	"host_path_isolation",
	"mount_propagation_private",
	"read_only_rootfs",
	"read_only_inputs",
	"dedicated_writable_output",
	"network_default_deny",
	"exact_network_allowlist",
	"ephemeral_secret_materialization",
	"non_root_container_identity",
	"cpu_memory_pid_limits",
	"wall_clock_timeout",
	"graceful_then_forced_kill",
	"orphan_reconciliation",
	"output_regular_file_only",
	"output_symlink_special_rejection",
	"atomic_output_artifact_commit",
}

type BackendCheck struct {
	Ordinal       int
	Name          string
	Required      bool
	Verified      bool
	EvidenceState string
}

func RequiredBackendChecks() []BackendCheck {
	checks := make([]BackendCheck, len(requiredBackendCheckNames))
	for index, name := range requiredBackendCheckNames {
		checks[index] = BackendCheck{
			Ordinal: index + 1, Name: name, Required: true,
			Verified: false, EvidenceState: BackendCheckEvidenceNotProbed,
		}
	}
	return checks
}

func (c BackendCheck) Validate() error {
	if c.Ordinal < 1 || c.Ordinal > len(requiredBackendCheckNames) ||
		c.Name != requiredBackendCheckNames[c.Ordinal-1] || !c.Required || c.Verified ||
		c.EvidenceState != BackendCheckEvidenceNotProbed {
		return errors.New("sandbox backend check must remain required and unverified")
	}
	return nil
}

func BackendThreatModelFingerprint(checks []BackendCheck) string {
	parts := []string{BackendHandshakeProtocolVersion, strconv.Itoa(len(checks))}
	for _, check := range checks {
		parts = append(parts, strconv.Itoa(check.Ordinal), check.Name,
			strconv.FormatBool(check.Required), strconv.FormatBool(check.Verified),
			check.EvidenceState)
	}
	return fingerprint(parts...)
}

type ContainerIdentity struct {
	ProtocolVersion string
	Runtime         string
	Bound           bool
	Fingerprint     string
}

func DisabledContainerIdentity() ContainerIdentity {
	return ContainerIdentity{
		ProtocolVersion: ContainerIdentityProtocolVersion,
		Runtime:         "none",
		Bound:           false,
		Fingerprint:     "",
	}
}

func (i ContainerIdentity) Validate() error {
	if i.ProtocolVersion != ContainerIdentityProtocolVersion || i.Runtime != "none" ||
		i.Bound || i.Fingerprint != "" {
		return errors.New("sandbox container identity must remain unbound while the backend is disabled")
	}
	return nil
}

type BackendHandshake struct {
	ProtocolVersion        string
	Backend                Backend
	InspectorName          string
	Status                 string
	Available              bool
	ThreatModelFingerprint string
	Checks                 []BackendCheck
	ContainerIdentity      ContainerIdentity
}

func (h BackendHandshake) Validate() error {
	if h.ProtocolVersion != BackendHandshakeProtocolVersion || !h.Backend.Valid() ||
		h.InspectorName != DisabledBackendInspectorName ||
		h.Status != PreflightStatusBackendDisabled || h.Available {
		return errors.New("sandbox backend handshake must remain disabled")
	}
	if len(h.Checks) != len(requiredBackendCheckNames) || len(h.Checks) > MaxBackendChecks {
		return errors.New("sandbox backend handshake check set is incomplete")
	}
	for _, check := range h.Checks {
		if err := check.Validate(); err != nil {
			return err
		}
	}
	if h.ThreatModelFingerprint != BackendThreatModelFingerprint(h.Checks) {
		return errors.New("sandbox backend threat-model fingerprint is invalid")
	}
	return h.ContainerIdentity.Validate()
}

type BackendInspector interface {
	Inspect(ctx context.Context, backend Backend) (BackendHandshake, error)
}

type DisabledBackendInspector struct{}

func NewDisabledBackendInspector() DisabledBackendInspector {
	return DisabledBackendInspector{}
}

func (DisabledBackendInspector) Inspect(ctx context.Context, backend Backend) (BackendHandshake, error) {
	if err := ctx.Err(); err != nil {
		return BackendHandshake{}, err
	}
	if !backend.Valid() {
		return BackendHandshake{}, fmt.Errorf("unsupported sandbox backend %q", backend)
	}
	checks := RequiredBackendChecks()
	return BackendHandshake{
		ProtocolVersion: BackendHandshakeProtocolVersion,
		Backend:         backend, InspectorName: DisabledBackendInspectorName,
		Status: PreflightStatusBackendDisabled, Available: false,
		ThreatModelFingerprint: BackendThreatModelFingerprint(checks), Checks: checks,
		ContainerIdentity: DisabledContainerIdentity(),
	}, nil
}

type OutputExportSlot struct {
	Ordinal                  int
	Kind                     string
	LocatorFingerprint       string
	RegularFileRequired      bool
	SymlinkRejected          bool
	SpecialFileRejected      bool
	MIMEDetectionRequired    bool
	RedactionRequired        bool
	ArtifactCommitAuthorized bool
}

func (s OutputExportSlot) Validate() error {
	if s.Ordinal < 1 || s.Ordinal > MaxOutputPaths+2 || !validDigest(s.LocatorFingerprint) ||
		!s.MIMEDetectionRequired || !s.RedactionRequired || s.ArtifactCommitAuthorized {
		return errors.New("sandbox output export slot is outside protocol bounds")
	}
	switch s.Kind {
	case OutputKindStdout, OutputKindStderr:
		if s.RegularFileRequired || s.SymlinkRejected || s.SpecialFileRejected {
			return errors.New("sandbox stream output cannot claim file checks")
		}
	case OutputKindFile:
		if !s.RegularFileRequired || !s.SymlinkRejected || !s.SpecialFileRejected {
			return errors.New("sandbox file output must reject links and special files")
		}
	default:
		return fmt.Errorf("sandbox output kind %q is invalid", s.Kind)
	}
	return nil
}

type OutputExportPlan struct {
	ProtocolVersion          string
	CaptureStdout            bool
	CaptureStderr            bool
	SlotCount                int
	MaxOutputBytes           int64
	PartialFailurePolicy     string
	TruncationPolicy         string
	MIMEPolicy               string
	FileTypePolicy           string
	RestartPolicy            string
	RawPathsStored           bool
	ExportEnabled            bool
	ArtifactCommitAuthorized bool
	Fingerprint              string
	Slots                    []OutputExportSlot
}

func NewOutputExportPlan(manifest Manifest) (OutputExportPlan, error) {
	normalized, err := NormalizeManifest(manifest)
	if err != nil {
		return OutputExportPlan{}, err
	}
	slots := make([]OutputExportSlot, 0, normalized.OutputCount())
	appendSlot := func(kind, locator string, file bool) {
		slots = append(slots, OutputExportSlot{
			Ordinal: len(slots) + 1, Kind: kind,
			LocatorFingerprint:       fingerprint("sandbox_output_locator.v1", kind, locator),
			RegularFileRequired:      file,
			SymlinkRejected:          file,
			SpecialFileRejected:      file,
			MIMEDetectionRequired:    true,
			RedactionRequired:        true,
			ArtifactCommitAuthorized: false,
		})
	}
	if normalized.Output.CaptureStdout {
		appendSlot(OutputKindStdout, OutputKindStdout, false)
	}
	if normalized.Output.CaptureStderr {
		appendSlot(OutputKindStderr, OutputKindStderr, false)
	}
	for _, outputPath := range normalized.Output.Paths {
		appendSlot(OutputKindFile, outputPath, true)
	}
	plan := OutputExportPlan{
		ProtocolVersion: OutputExportProtocolVersion,
		CaptureStdout:   normalized.Output.CaptureStdout, CaptureStderr: normalized.Output.CaptureStderr,
		SlotCount: len(slots), MaxOutputBytes: normalized.Resources.MaxOutputBytes,
		PartialFailurePolicy: OutputPartialFailureAllOrNothing,
		TruncationPolicy:     OutputTruncationAggregateHardCap,
		MIMEPolicy:           OutputMIMEDetectAndValidate, FileTypePolicy: OutputFileRegularNoLinks,
		RestartPolicy: OutputRestartReconcile, RawPathsStored: false,
		ExportEnabled: false, ArtifactCommitAuthorized: false, Slots: slots,
	}
	plan.Fingerprint = outputExportPlanFingerprint(plan)
	return plan, plan.Validate()
}

func (p OutputExportPlan) Validate() error {
	if p.ProtocolVersion != OutputExportProtocolVersion || p.SlotCount != len(p.Slots) ||
		p.SlotCount < 1 || p.SlotCount > MaxOutputPaths+2 ||
		p.MaxOutputBytes < 1 || p.MaxOutputBytes > MaxCapturedOutputBytes ||
		p.PartialFailurePolicy != OutputPartialFailureAllOrNothing ||
		p.TruncationPolicy != OutputTruncationAggregateHardCap ||
		p.MIMEPolicy != OutputMIMEDetectAndValidate || p.FileTypePolicy != OutputFileRegularNoLinks ||
		p.RestartPolicy != OutputRestartReconcile || p.RawPathsStored || p.ExportEnabled ||
		p.ArtifactCommitAuthorized {
		return errors.New("sandbox output export plan must remain bounded and disabled")
	}
	stdout, stderr := false, false
	for index, slot := range p.Slots {
		if slot.Ordinal != index+1 {
			return errors.New("sandbox output export slot order is invalid")
		}
		if err := slot.Validate(); err != nil {
			return err
		}
		switch slot.Kind {
		case OutputKindStdout:
			if stdout {
				return errors.New("sandbox output export plan duplicates stdout")
			}
			stdout = true
		case OutputKindStderr:
			if stderr {
				return errors.New("sandbox output export plan duplicates stderr")
			}
			stderr = true
		}
	}
	if stdout != p.CaptureStdout || stderr != p.CaptureStderr ||
		p.Fingerprint != outputExportPlanFingerprint(p) {
		return errors.New("sandbox output export plan fingerprint or stream binding is invalid")
	}
	return nil
}

func outputExportPlanFingerprint(plan OutputExportPlan) string {
	parts := []string{
		OutputExportProtocolVersion,
		strconv.FormatBool(plan.CaptureStdout), strconv.FormatBool(plan.CaptureStderr),
		strconv.Itoa(plan.SlotCount), strconv.FormatInt(plan.MaxOutputBytes, 10),
		plan.PartialFailurePolicy, plan.TruncationPolicy, plan.MIMEPolicy,
		plan.FileTypePolicy, plan.RestartPolicy,
		strconv.FormatBool(plan.RawPathsStored), strconv.FormatBool(plan.ExportEnabled),
		strconv.FormatBool(plan.ArtifactCommitAuthorized),
	}
	for _, slot := range plan.Slots {
		parts = append(parts, strconv.Itoa(slot.Ordinal), slot.Kind, slot.LocatorFingerprint,
			strconv.FormatBool(slot.RegularFileRequired), strconv.FormatBool(slot.SymlinkRejected),
			strconv.FormatBool(slot.SpecialFileRejected),
			strconv.FormatBool(slot.MIMEDetectionRequired), strconv.FormatBool(slot.RedactionRequired),
			strconv.FormatBool(slot.ArtifactCommitAuthorized))
	}
	return fingerprint(parts...)
}

type DisabledPreflight struct {
	ID                       string
	ExecutionID              string
	CandidateID              string
	PreparationID            string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ProtocolVersion          string
	Backend                  Backend
	ManifestFingerprint      string
	AuthorizationFingerprint string
	PolicyFingerprint        string
	MountBindingFingerprint  string
	InputArtifactDigest      string
	Handshake                BackendHandshake
	OutputPlan               OutputExportPlan
	Status                   string
	BackendEnabled           bool
	ExecutionAuthorized      bool
	ArtifactCommitAuthorized bool
	RequestedBy              string
	CreatedAt                time.Time
	Replayed                 bool
}

func (p DisabledPreflight) Validate() error {
	for label, value := range map[string]string{
		"preflight id": p.ID, "preflight execution id": p.ExecutionID,
		"preflight candidate id": p.CandidateID, "preflight preparation id": p.PreparationID,
		"preflight Run id": p.RunID, "preflight Mission id": p.MissionID,
		"preflight workspace id": p.WorkspaceID, "preflight requester": p.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if p.ProtocolVersion != PreflightProtocolVersion || !p.Backend.Valid() ||
		p.Handshake.Backend != p.Backend || p.Status != PreflightStatusBackendDisabled ||
		p.BackendEnabled || p.ExecutionAuthorized || p.ArtifactCommitAuthorized || p.CreatedAt.IsZero() {
		return errors.New("sandbox preflight must remain disabled")
	}
	for label, value := range map[string]string{
		"manifest": p.ManifestFingerprint, "authorization": p.AuthorizationFingerprint,
		"policy": p.PolicyFingerprint, "mount binding": p.MountBindingFingerprint,
		"input Artifact": p.InputArtifactDigest,
	} {
		if !validDigest(value) {
			return fmt.Errorf("sandbox preflight %s fingerprint is invalid", label)
		}
	}
	if err := p.Handshake.Validate(); err != nil {
		return err
	}
	return p.OutputPlan.Validate()
}

type PreflightOperation struct {
	KeyDigest          string
	RequestFingerprint string
	PreflightID        string
	ExecutionID        string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (o PreflightOperation) Validate() error {
	for label, value := range map[string]string{
		"preflight operation preflight id": o.PreflightID,
		"preflight operation execution id": o.ExecutionID,
		"preflight operation Run id":       o.RunID,
		"preflight operation requester":    o.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("sandbox preflight operation digests or timestamp are invalid")
	}
	return nil
}

func PreflightRequestFingerprint(preflight DisabledPreflight) string {
	return fingerprint("sandbox_preflight_request.v1", preflight.ExecutionID,
		preflight.ManifestFingerprint, string(preflight.Backend),
		preflight.Handshake.ThreatModelFingerprint, preflight.OutputPlan.Fingerprint,
		preflight.RequestedBy)
}
