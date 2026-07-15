package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DockerHostInputStagingIntentProtocolVersion       = "sandbox_docker_host_input_staging_intent.v1"
	DockerHostInputStagingProtocolVersion             = "sandbox_docker_host_input_staging.v1"
	HostInputBundleProtocolVersion                    = "sandbox_host_input_bundle.v1"
	HostInputBundleSourceLinux                        = "linux_openat2_memfd_seal"
	HostInputBundleStatusSealed                       = "descriptor_bundle_sealed"
	DockerHostInputStagingStatusComplete              = "host_inputs_descriptor_sealed"
	DockerHostInputStagingTrustClass                  = "local_descriptor_rehearsal_unconsumed"
	DockerHostInputStagingErrorDisabled               = "staging_disabled"
	DockerHostInputStagingErrorUnsupported            = "staging_unsupported"
	DockerHostInputStagingErrorUnsafeSource           = "unsafe_source"
	DockerHostInputStagingErrorSourceChanged          = "source_changed"
	DockerHostInputStagingErrorResourceLimit          = "resource_limit"
	DockerHostInputStagingErrorSealFailed             = "seal_failed"
	MaxHostInputBundleEntries                         = 4096
	MaxHostInputSourceBytes                     int64 = 16 * 1024 * 1024
	MaxHostInputBundleBytes                     int64 = 40 * 1024 * 1024
	MaxHostInputPathBytes                             = 1024
	MaxHostInputWorkspaceRootBytes                    = 4096
)

type HostInputArtifact struct {
	Ordinal    int
	ArtifactID string
	SHA256     string
	SizeBytes  int64
	MIME       string
	Stream     string
	SourceID   string
	Redacted   bool
	Content    string
}

func (artifact HostInputArtifact) Validate() error {
	for label, value := range map[string]string{
		"host input Artifact id": artifact.ArtifactID,
		"host input source id":   artifact.SourceID,
	} {
		if validateStoredIdentity(label, value) != nil {
			return errors.New("host input Artifact identity is invalid")
		}
	}
	if artifact.Ordinal < 1 || artifact.Ordinal > MaxInputArtifacts ||
		!validDigest(artifact.SHA256) || artifact.SizeBytes < 1 ||
		artifact.SizeBytes > MaxInputArtifactTotalBytes ||
		int64(len([]byte(artifact.Content))) != artifact.SizeBytes ||
		!utf8.ValidString(artifact.Content) ||
		hashHostInputBytes([]byte(artifact.Content)) != artifact.SHA256 ||
		(artifact.Stream != "stdout" && artifact.Stream != "stderr") ||
		artifact.MIME == "" || len([]byte(artifact.MIME)) > 256 {
		return errors.New("host input Artifact content or metadata is invalid")
	}
	if _, _, err := mime.ParseMediaType(artifact.MIME); err != nil {
		return errors.New("host input Artifact MIME is invalid")
	}
	return nil
}

type HostInputBundleRequest struct {
	WorkspaceRoot string
	Manifest      Manifest
	Artifacts     []HostInputArtifact
}

func (request HostInputBundleRequest) Validate() error {
	if request.WorkspaceRoot == "" || !utf8.ValidString(request.WorkspaceRoot) ||
		strings.ContainsRune(request.WorkspaceRoot, 0) ||
		len([]byte(request.WorkspaceRoot)) > MaxHostInputWorkspaceRootBytes ||
		!filepath.IsAbs(request.WorkspaceRoot) ||
		filepath.Clean(request.WorkspaceRoot) != request.WorkspaceRoot {
		return errors.New("host input staging workspace root must be an absolute clean path")
	}
	normalized, err := NormalizeManifest(request.Manifest)
	if err != nil {
		return err
	}
	readOnly := 0
	for _, mount := range normalized.Mounts {
		if mount.Access == MountReadOnly {
			readOnly++
		}
	}
	if readOnly == 0 || len(request.Artifacts) != len(normalized.InputArtifactIDs) {
		return errors.New("host input staging requires read-only mounts and exact input Artifacts")
	}
	var artifactBytes int64
	for index, artifact := range request.Artifacts {
		if artifact.Validate() != nil || artifact.Ordinal != index+1 ||
			artifact.ArtifactID != normalized.InputArtifactIDs[index] {
			return errors.New("host input Artifact sequence does not match the Manifest")
		}
		artifactBytes += artifact.SizeBytes
		if artifactBytes > MaxInputArtifactTotalBytes {
			return errors.New("host input Artifacts exceed the aggregate limit")
		}
	}
	return nil
}

func (request HostInputBundleRequest) ReadOnlyMountCount() int {
	count := 0
	for _, mount := range request.Manifest.Mounts {
		if mount.Access == MountReadOnly {
			count++
		}
	}
	return count
}

func (request HostInputBundleRequest) ArtifactBytes() int64 {
	var total int64
	for _, artifact := range request.Artifacts {
		total += artifact.SizeBytes
	}
	return total
}

func (request HostInputBundleRequest) ArtifactPayloadDigest() string {
	return hostInputArtifactPayloadDigest(request.Artifacts)
}

type HostInputBundleMeasurements struct {
	ReadOnlyMountCount    int
	ArtifactCount         int
	RegularFileCount      int
	DirectoryCount        int
	SourceBytes           int64
	ArtifactBytes         int64
	BundleBytes           int64
	SourceSnapshotDigest  string
	ArtifactPayloadDigest string
	BundleDigest          string
}

type HostInputBundleReport struct {
	ProtocolVersion       string
	Source                string
	Status                string
	ReadOnlyMountCount    int
	ArtifactCount         int
	RegularFileCount      int
	DirectoryCount        int
	EntryCount            int
	SourceBytes           int64
	ArtifactBytes         int64
	BundleBytes           int64
	SourceSnapshotDigest  string
	ArtifactPayloadDigest string
	BundleDigest          string
	ReportFingerprint     string
	DescriptorPinned      bool
	SymlinkFree           bool
	KernelSealed          bool
	SourcePathsRetained   bool
	RawContentPersisted   bool
	DaemonConsumed        bool
	ContainerStarted      bool
	ProcessExecuted       bool
	ExecutionEvidence     bool
	CreatedAt             time.Time
}

func NewHostInputBundleReport(measurements HostInputBundleMeasurements,
	createdAt time.Time,
) (HostInputBundleReport, error) {
	report := HostInputBundleReport{
		ProtocolVersion:    HostInputBundleProtocolVersion,
		Source:             HostInputBundleSourceLinux,
		Status:             HostInputBundleStatusSealed,
		ReadOnlyMountCount: measurements.ReadOnlyMountCount,
		ArtifactCount:      measurements.ArtifactCount,
		RegularFileCount:   measurements.RegularFileCount,
		DirectoryCount:     measurements.DirectoryCount,
		EntryCount: measurements.RegularFileCount + measurements.DirectoryCount +
			measurements.ArtifactCount,
		SourceBytes: measurements.SourceBytes, ArtifactBytes: measurements.ArtifactBytes,
		BundleBytes:           measurements.BundleBytes,
		SourceSnapshotDigest:  measurements.SourceSnapshotDigest,
		ArtifactPayloadDigest: measurements.ArtifactPayloadDigest,
		BundleDigest:          measurements.BundleDigest,
		DescriptorPinned:      true, SymlinkFree: true, KernelSealed: true,
		CreatedAt: createdAt.UTC(),
	}
	report.ReportFingerprint = hostInputBundleReportFingerprint(report)
	return report, report.Validate()
}

func (report HostInputBundleReport) Validate() error {
	if report.ProtocolVersion != HostInputBundleProtocolVersion ||
		report.Source != HostInputBundleSourceLinux || report.Status != HostInputBundleStatusSealed ||
		report.ReadOnlyMountCount < 1 || report.ReadOnlyMountCount > MaxMounts ||
		report.ArtifactCount < 0 || report.ArtifactCount > MaxInputArtifacts ||
		report.RegularFileCount < 0 || report.DirectoryCount < 0 ||
		report.RegularFileCount+report.DirectoryCount < report.ReadOnlyMountCount ||
		report.EntryCount != report.RegularFileCount+report.DirectoryCount+report.ArtifactCount ||
		report.EntryCount < report.ReadOnlyMountCount || report.EntryCount > MaxHostInputBundleEntries ||
		report.SourceBytes < 0 || report.SourceBytes > MaxHostInputSourceBytes ||
		report.ArtifactBytes < 0 || report.ArtifactBytes > MaxInputArtifactTotalBytes ||
		report.BundleBytes < 1 || report.BundleBytes > MaxHostInputBundleBytes ||
		!validDigest(report.SourceSnapshotDigest) || !validDigest(report.ArtifactPayloadDigest) ||
		!validDigest(report.BundleDigest) || !validDigest(report.ReportFingerprint) ||
		!report.DescriptorPinned || !report.SymlinkFree || !report.KernelSealed ||
		report.SourcePathsRetained || report.RawContentPersisted || report.DaemonConsumed ||
		report.ContainerStarted || report.ProcessExecuted || report.ExecutionEvidence ||
		report.CreatedAt.IsZero() ||
		report.ReportFingerprint != hostInputBundleReportFingerprint(report) {
		return errors.New("host input bundle report violates the descriptor-only rehearsal boundary")
	}
	return nil
}

func hostInputBundleReportFingerprint(report HostInputBundleReport) string {
	return fingerprint(HostInputBundleProtocolVersion, report.Source, report.Status,
		strconv.Itoa(report.ReadOnlyMountCount), strconv.Itoa(report.ArtifactCount),
		strconv.Itoa(report.RegularFileCount), strconv.Itoa(report.DirectoryCount),
		strconv.Itoa(report.EntryCount), strconv.FormatInt(report.SourceBytes, 10),
		strconv.FormatInt(report.ArtifactBytes, 10), strconv.FormatInt(report.BundleBytes, 10),
		report.SourceSnapshotDigest, report.ArtifactPayloadDigest, report.BundleDigest,
		strconv.FormatBool(report.DescriptorPinned), strconv.FormatBool(report.SymlinkFree),
		strconv.FormatBool(report.KernelSealed), strconv.FormatBool(report.SourcePathsRetained),
		strconv.FormatBool(report.RawContentPersisted), strconv.FormatBool(report.DaemonConsumed),
		strconv.FormatBool(report.ContainerStarted), strconv.FormatBool(report.ProcessExecuted),
		strconv.FormatBool(report.ExecutionEvidence))
}

type DockerHostInputStagingError struct {
	code string
}

func (err *DockerHostInputStagingError) Error() string {
	return "docker host input staging failed: " + err.code
}

func newDockerHostInputStagingError(code string) error {
	return &DockerHostInputStagingError{code: code}
}

func DockerHostInputStagingErrorCode(err error) string {
	var stagingError *DockerHostInputStagingError
	if errors.As(err, &stagingError) {
		return stagingError.code
	}
	return ""
}

type DockerHostInputStager interface {
	Probe(ctx context.Context, workspaceRoot string) error
	Stage(ctx context.Context, request HostInputBundleRequest) (HostInputBundleReport, error)
}

type UnavailableDockerHostInputStager struct {
	code string
}

func NewUnavailableDockerHostInputStager() UnavailableDockerHostInputStager {
	return UnavailableDockerHostInputStager{code: DockerHostInputStagingErrorDisabled}
}

func (stager UnavailableDockerHostInputStager) Probe(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return newDockerHostInputStagingError(stager.normalizedCode())
}

func (stager UnavailableDockerHostInputStager) Stage(ctx context.Context,
	_ HostInputBundleRequest,
) (HostInputBundleReport, error) {
	if err := ctx.Err(); err != nil {
		return HostInputBundleReport{}, err
	}
	return HostInputBundleReport{}, newDockerHostInputStagingError(stager.normalizedCode())
}

func (stager UnavailableDockerHostInputStager) normalizedCode() string {
	if stager.code == DockerHostInputStagingErrorUnsupported {
		return stager.code
	}
	return DockerHostInputStagingErrorDisabled
}

type DockerHostInputStagingIntent struct {
	ID                       string
	AttemptID                string
	PlanID                   string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ProtocolVersion          string
	OperationKeyDigest       string
	AttemptIntentFingerprint string
	RequestFingerprint       string
	ContainerIDFingerprint   string
	ManifestFingerprint      string
	MountBindingFingerprint  string
	InputArtifactDigest      string
	AuthorityFingerprint     string
	SpecFingerprint          string
	PlanFingerprint          string
	ReadOnlyMountCount       int
	InputArtifactCount       int
	PreparedGeneration       int64
	IntentFingerprint        string
	RequestedBy              string
	CreatedAt                time.Time
}

func NewDockerHostInputStagingIntent(id, operationKeyDigest string,
	attempt DockerContainerRehearsalAttempt, plan DockerContainerPlan,
	manifest Manifest, requestedBy string, now time.Time,
) (DockerHostInputStagingIntent, error) {
	requestedBy = strings.TrimSpace(requestedBy)
	normalized, err := NormalizeManifest(manifest)
	manifestFingerprint, fingerprintErr := "", error(nil)
	if err == nil {
		manifestFingerprint, fingerprintErr = normalized.Fingerprint()
	}
	if err != nil || attempt.Validate() != nil || plan.Validate() != nil ||
		attempt.Stage == nil || attempt.Completion != nil || attempt.Intent.PlanID != plan.ID ||
		attempt.Intent.RequestedBy != requestedBy || !attempt.Lease.ActiveAt(now) ||
		fingerprintErr != nil || manifestFingerprint != plan.ManifestFingerprint {
		return DockerHostInputStagingIntent{}, errors.New("docker host input staging intent authority is invalid")
	}
	readOnly := 0
	for _, mount := range normalized.Mounts {
		if mount.Access == MountReadOnly {
			readOnly++
		}
	}
	if readOnly != plan.ReadOnlyMountCount ||
		len(normalized.InputArtifactIDs) != plan.InputArtifactCount {
		return DockerHostInputStagingIntent{}, errors.New(
			"docker host input staging Manifest counts changed")
	}
	intent := DockerHostInputStagingIntent{
		ID: id, AttemptID: attempt.Intent.ID, PlanID: plan.ID, RunID: plan.RunID,
		MissionID: plan.MissionID, WorkspaceID: plan.WorkspaceID,
		ProtocolVersion:          DockerHostInputStagingIntentProtocolVersion,
		OperationKeyDigest:       operationKeyDigest,
		AttemptIntentFingerprint: attempt.Intent.IntentFingerprint,
		RequestFingerprint:       attempt.Intent.RequestFingerprint,
		ContainerIDFingerprint:   attempt.Stage.Result.ContainerIDFingerprint,
		ManifestFingerprint:      plan.ManifestFingerprint,
		MountBindingFingerprint:  plan.MountBindingFingerprint,
		InputArtifactDigest:      plan.InputArtifactDigest,
		AuthorityFingerprint:     plan.AuthorityFingerprint,
		SpecFingerprint:          plan.SpecFingerprint, PlanFingerprint: plan.PlanFingerprint,
		ReadOnlyMountCount: readOnly, InputArtifactCount: len(normalized.InputArtifactIDs),
		PreparedGeneration: attempt.Lease.Generation, RequestedBy: requestedBy, CreatedAt: now,
	}
	intent.IntentFingerprint = dockerHostInputStagingIntentFingerprint(intent)
	return intent, intent.Validate()
}

func (intent DockerHostInputStagingIntent) Validate() error {
	for label, value := range map[string]string{
		"host input staging intent id": intent.ID, "host input staging attempt id": intent.AttemptID,
		"host input staging plan id": intent.PlanID, "host input staging Run id": intent.RunID,
		"host input staging Mission id":   intent.MissionID,
		"host input staging workspace id": intent.WorkspaceID,
		"host input staging requester":    intent.RequestedBy,
	} {
		if validateStoredIdentity(label, value) != nil {
			return errors.New("docker host input staging intent identity is invalid")
		}
	}
	if intent.ProtocolVersion != DockerHostInputStagingIntentProtocolVersion ||
		intent.ReadOnlyMountCount < 1 || intent.ReadOnlyMountCount > MaxMounts ||
		intent.InputArtifactCount < 0 || intent.InputArtifactCount > MaxInputArtifacts ||
		intent.PreparedGeneration < 1 || intent.CreatedAt.IsZero() {
		return errors.New("docker host input staging intent fields are invalid")
	}
	for _, value := range []string{intent.OperationKeyDigest, intent.AttemptIntentFingerprint,
		intent.RequestFingerprint, intent.ContainerIDFingerprint, intent.ManifestFingerprint,
		intent.MountBindingFingerprint, intent.InputArtifactDigest, intent.AuthorityFingerprint,
		intent.SpecFingerprint, intent.PlanFingerprint, intent.IntentFingerprint} {
		if !validDigest(value) {
			return errors.New("docker host input staging intent digest is invalid")
		}
	}
	if intent.IntentFingerprint != dockerHostInputStagingIntentFingerprint(intent) {
		return errors.New("docker host input staging intent fingerprint is invalid")
	}
	return nil
}

func dockerHostInputStagingIntentFingerprint(intent DockerHostInputStagingIntent) string {
	return fingerprint(DockerHostInputStagingIntentProtocolVersion, intent.AttemptID,
		intent.PlanID, intent.RunID, intent.MissionID, intent.WorkspaceID,
		intent.OperationKeyDigest, intent.AttemptIntentFingerprint, intent.RequestFingerprint,
		intent.ContainerIDFingerprint, intent.ManifestFingerprint, intent.MountBindingFingerprint,
		intent.InputArtifactDigest, intent.AuthorityFingerprint, intent.SpecFingerprint,
		intent.PlanFingerprint, strconv.Itoa(intent.ReadOnlyMountCount),
		strconv.Itoa(intent.InputArtifactCount), strconv.FormatInt(intent.PreparedGeneration, 10),
		intent.RequestedBy)
}

type DockerHostInputStaging struct {
	ID                       string
	IntentID                 string
	AttemptID                string
	PlanID                   string
	RunID                    string
	ProtocolVersion          string
	Source                   string
	TrustClass               string
	Status                   string
	LeaseGeneration          int64
	AttemptIntentFingerprint string
	ContainerIDFingerprint   string
	InputArtifactDigest      string
	AuthorityFingerprint     string
	ReadOnlyMountCount       int
	InputArtifactCount       int
	Report                   HostInputBundleReport
	StagingFingerprint       string
	ProductionVerified       bool
	BackendEnabled           bool
	ExecutionAuthorized      bool
	ArtifactCommitAuthorized bool
	CreatedAt                time.Time
}

func NewDockerHostInputStaging(id string, intent DockerHostInputStagingIntent,
	leaseGeneration int64, report HostInputBundleReport, now time.Time,
) (DockerHostInputStaging, error) {
	if intent.Validate() != nil || report.Validate() != nil || leaseGeneration < 1 || now.IsZero() ||
		report.ReadOnlyMountCount != intent.ReadOnlyMountCount ||
		report.ArtifactCount != intent.InputArtifactCount {
		return DockerHostInputStaging{}, errors.New("docker host input staging result authority is invalid")
	}
	value := DockerHostInputStaging{
		ID: id, IntentID: intent.ID, AttemptID: intent.AttemptID, PlanID: intent.PlanID,
		RunID: intent.RunID, ProtocolVersion: DockerHostInputStagingProtocolVersion,
		Source: report.Source, TrustClass: DockerHostInputStagingTrustClass,
		Status: DockerHostInputStagingStatusComplete, LeaseGeneration: leaseGeneration,
		AttemptIntentFingerprint: intent.AttemptIntentFingerprint,
		ContainerIDFingerprint:   intent.ContainerIDFingerprint,
		InputArtifactDigest:      intent.InputArtifactDigest,
		AuthorityFingerprint:     intent.AuthorityFingerprint,
		ReadOnlyMountCount:       intent.ReadOnlyMountCount,
		InputArtifactCount:       intent.InputArtifactCount, Report: report, CreatedAt: now,
	}
	value.StagingFingerprint = dockerHostInputStagingFingerprint(value)
	return value, value.Validate()
}

func (value DockerHostInputStaging) Validate() error {
	for label, identity := range map[string]string{
		"host input staging id": value.ID, "host input staging intent id": value.IntentID,
		"host input staging attempt id": value.AttemptID,
		"host input staging plan id":    value.PlanID, "host input staging Run id": value.RunID,
	} {
		if validateStoredIdentity(label, identity) != nil {
			return errors.New("docker host input staging identity is invalid")
		}
	}
	if value.ProtocolVersion != DockerHostInputStagingProtocolVersion ||
		value.Source != HostInputBundleSourceLinux ||
		value.TrustClass != DockerHostInputStagingTrustClass ||
		value.Status != DockerHostInputStagingStatusComplete || value.LeaseGeneration < 1 ||
		value.ReadOnlyMountCount < 1 || value.ReadOnlyMountCount > MaxMounts ||
		value.InputArtifactCount < 0 || value.InputArtifactCount > MaxInputArtifacts ||
		value.Report.Validate() != nil || value.Report.ReadOnlyMountCount != value.ReadOnlyMountCount ||
		value.Report.ArtifactCount != value.InputArtifactCount || value.ProductionVerified ||
		value.BackendEnabled || value.ExecutionAuthorized || value.ArtifactCommitAuthorized ||
		value.CreatedAt.IsZero() || !validDigest(value.AttemptIntentFingerprint) ||
		!validDigest(value.ContainerIDFingerprint) || !validDigest(value.InputArtifactDigest) ||
		!validDigest(value.AuthorityFingerprint) || !validDigest(value.StagingFingerprint) ||
		value.StagingFingerprint != dockerHostInputStagingFingerprint(value) {
		return errors.New("docker host input staging result violates the non-executing boundary")
	}
	return nil
}

func dockerHostInputStagingFingerprint(value DockerHostInputStaging) string {
	return fingerprint(DockerHostInputStagingProtocolVersion, value.IntentID,
		value.AttemptID, value.PlanID, value.RunID, value.Source, value.TrustClass, value.Status,
		strconv.FormatInt(value.LeaseGeneration, 10), value.AttemptIntentFingerprint,
		value.ContainerIDFingerprint, value.InputArtifactDigest, value.AuthorityFingerprint,
		strconv.Itoa(value.ReadOnlyMountCount), strconv.Itoa(value.InputArtifactCount),
		value.Report.ReportFingerprint, strconv.FormatBool(value.ProductionVerified),
		strconv.FormatBool(value.BackendEnabled), strconv.FormatBool(value.ExecutionAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized))
}

type DockerHostInputStagingRecord struct {
	Intent   DockerHostInputStagingIntent
	Staging  *DockerHostInputStaging
	Replayed bool
}

func (record DockerHostInputStagingRecord) Validate() error {
	if record.Intent.Validate() != nil {
		return errors.New("docker host input staging record intent is invalid")
	}
	if record.Staging != nil {
		if record.Staging.Validate() != nil || record.Staging.IntentID != record.Intent.ID ||
			record.Staging.AttemptID != record.Intent.AttemptID ||
			record.Staging.PlanID != record.Intent.PlanID ||
			record.Staging.RunID != record.Intent.RunID ||
			record.Staging.AttemptIntentFingerprint != record.Intent.AttemptIntentFingerprint ||
			record.Staging.ContainerIDFingerprint != record.Intent.ContainerIDFingerprint ||
			record.Staging.InputArtifactDigest != record.Intent.InputArtifactDigest ||
			record.Staging.AuthorityFingerprint != record.Intent.AuthorityFingerprint {
			return errors.New("docker host input staging record result is invalid")
		}
	}
	return nil
}

func hostInputArtifactPayloadDigest(artifacts []HostInputArtifact) string {
	parts := []string{"sandbox_host_input_artifact_payloads.v1", strconv.Itoa(len(artifacts))}
	for _, artifact := range artifacts {
		parts = append(parts, strconv.Itoa(artifact.Ordinal), artifact.ArtifactID,
			artifact.SHA256, strconv.FormatInt(artifact.SizeBytes, 10), artifact.MIME,
			artifact.Stream, artifact.SourceID, strconv.FormatBool(artifact.Redacted))
	}
	return fingerprint(parts...)
}

func hashHostInputBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
