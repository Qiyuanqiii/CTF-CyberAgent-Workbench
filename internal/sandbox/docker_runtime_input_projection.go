package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DockerRuntimeInputProjectionPlanProtocolVersion = "sandbox_docker_runtime_input_projection_plan.v1"
	DockerRuntimeInputProjectionItemProtocolVersion = "sandbox_docker_runtime_input_projection_item.v1"
	DockerRuntimeInputProjectionOperationVersion    = "sandbox_docker_runtime_input_projection_operation.v1"
	DockerRuntimeInputProjectionStatusCompiled      = "compiled_not_applied"
	DockerRuntimeInputProjectionTrustClass          = "handoff_bound_projection_plan_unapplied"
	DockerRuntimeInputProjectionKindManifestMount   = "manifest_directory_mount"
	DockerRuntimeInputProjectionKindArtifacts       = "input_artifact_directory"
	DockerRuntimeArtifactTarget                     = "/cyberagent-input/artifacts"
	MaxDockerRuntimeInputProjections                = MaxMounts + 1
)

type DockerRuntimeInputProjectionItem struct {
	Ordinal                      int
	ProtocolVersion              string
	Kind                         string
	ManifestMountOrdinal         int
	TargetFingerprint            string
	ArchiveRootFingerprint       string
	VolumeNameFingerprint        string
	EntryCount                   int
	RegularFileCount             int
	DirectoryCount               int
	ContentBytes                 int64
	ProjectionArchiveBytes       int64
	ContentDigest                string
	ProjectionArchiveDigest      string
	RootDirectory                bool
	ReadOnly                     bool
	ExactTarget                  bool
	NoCopy                       bool
	DaemonApplied                bool
	ContainerStarted             bool
	ProcessExecuted              bool
	ProductionExecutionSubmitted bool
	ItemFingerprint              string
}

func (item DockerRuntimeInputProjectionItem) Validate() error {
	if item.Ordinal < 1 || item.Ordinal > MaxDockerRuntimeInputProjections ||
		item.ProtocolVersion != DockerRuntimeInputProjectionItemProtocolVersion ||
		item.EntryCount < 0 || item.EntryCount > MaxHostInputBundleEntries ||
		item.RegularFileCount < 0 || item.DirectoryCount < 0 ||
		item.RegularFileCount+item.DirectoryCount != item.EntryCount ||
		item.ContentBytes < 0 || item.ContentBytes > MaxHostInputSourceBytes+MaxInputArtifactTotalBytes ||
		item.ProjectionArchiveBytes < 1 ||
		item.ProjectionArchiveBytes > MaxHostInputBundleBytes ||
		!item.RootDirectory || !item.ReadOnly || !item.ExactTarget || !item.NoCopy ||
		item.DaemonApplied || item.ContainerStarted || item.ProcessExecuted ||
		item.ProductionExecutionSubmitted {
		return errors.New("docker runtime input projection item widened authority")
	}
	for _, digest := range []string{item.TargetFingerprint, item.ArchiveRootFingerprint,
		item.VolumeNameFingerprint, item.ContentDigest, item.ProjectionArchiveDigest,
		item.ItemFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input projection item digest is invalid")
		}
	}
	switch item.Kind {
	case DockerRuntimeInputProjectionKindManifestMount:
		if item.ManifestMountOrdinal < 1 || item.ManifestMountOrdinal > MaxMounts ||
			item.DirectoryCount < 1 || item.EntryCount < 1 {
			return errors.New("docker runtime directory projection item is invalid")
		}
	case DockerRuntimeInputProjectionKindArtifacts:
		if item.ManifestMountOrdinal != 0 || item.RegularFileCount < 1 ||
			item.DirectoryCount != 0 || item.ContentBytes < 1 {
			return errors.New("docker runtime Artifact projection item is invalid")
		}
	default:
		return errors.New("docker runtime input projection kind is invalid")
	}
	if item.ItemFingerprint != dockerRuntimeInputProjectionItemFingerprint(item) {
		return errors.New("docker runtime input projection item fingerprint is invalid")
	}
	return nil
}

func dockerRuntimeInputProjectionItemFingerprint(item DockerRuntimeInputProjectionItem) string {
	return fingerprint(DockerRuntimeInputProjectionItemProtocolVersion, item.Kind,
		strconv.Itoa(item.Ordinal), strconv.Itoa(item.ManifestMountOrdinal),
		item.TargetFingerprint, item.ArchiveRootFingerprint, item.VolumeNameFingerprint,
		strconv.Itoa(item.EntryCount), strconv.Itoa(item.RegularFileCount),
		strconv.Itoa(item.DirectoryCount), strconv.FormatInt(item.ContentBytes, 10),
		strconv.FormatInt(item.ProjectionArchiveBytes, 10), item.ContentDigest,
		item.ProjectionArchiveDigest, strconv.FormatBool(item.RootDirectory),
		strconv.FormatBool(item.ReadOnly), strconv.FormatBool(item.ExactTarget),
		strconv.FormatBool(item.NoCopy), strconv.FormatBool(item.DaemonApplied),
		strconv.FormatBool(item.ContainerStarted), strconv.FormatBool(item.ProcessExecuted),
		strconv.FormatBool(item.ProductionExecutionSubmitted))
}

type DockerRuntimeInputProjectionArchive struct {
	ItemOrdinal int
	Target      string
	VolumeName  string
	Data        []byte
}

type DockerRuntimeInputProjectionCompilation struct {
	ManifestFingerprint       string
	RuntimeBindingFingerprint string
	BundleReportFingerprint   string
	BundleDigest              string
	BundleBytes               int64
	ReadOnlyMountCount        int
	InputArtifactCount        int
	DirectoryRootCount        int
	FileRootCount             int
	TotalEntryCount           int
	TotalContentBytes         int64
	TotalProjectionBytes      int64
	ProjectionSetFingerprint  string
	Items                     []DockerRuntimeInputProjectionItem
	Archives                  []DockerRuntimeInputProjectionArchive
}

func (value DockerRuntimeInputProjectionCompilation) Validate() error {
	if !validDigest(value.ManifestFingerprint) ||
		!validDigest(value.RuntimeBindingFingerprint) ||
		!validDigest(value.BundleReportFingerprint) || !validDigest(value.BundleDigest) ||
		!validDigest(value.ProjectionSetFingerprint) || value.BundleBytes < 1 ||
		value.BundleBytes > MaxHostInputBundleBytes || value.ReadOnlyMountCount < 1 ||
		value.ReadOnlyMountCount > MaxMounts || value.InputArtifactCount < 0 ||
		value.InputArtifactCount > MaxInputArtifacts ||
		value.DirectoryRootCount != value.ReadOnlyMountCount || value.FileRootCount != 0 ||
		value.TotalEntryCount < value.ReadOnlyMountCount ||
		value.TotalEntryCount > MaxHostInputBundleEntries || value.TotalContentBytes < 0 ||
		value.TotalContentBytes > MaxHostInputSourceBytes+MaxInputArtifactTotalBytes ||
		value.TotalProjectionBytes < 1 ||
		value.TotalProjectionBytes > int64(MaxDockerRuntimeInputProjections)*MaxHostInputBundleBytes ||
		len(value.Items) != len(value.Archives) || len(value.Items) < value.ReadOnlyMountCount ||
		len(value.Items) > MaxDockerRuntimeInputProjections {
		return errors.New("docker runtime input projection compilation is invalid")
	}
	if value.InputArtifactCount == 0 && len(value.Items) != value.ReadOnlyMountCount {
		return errors.New("docker runtime input projection has an unexpected Artifact volume")
	}
	if value.InputArtifactCount > 0 && len(value.Items) != value.ReadOnlyMountCount+1 {
		return errors.New("docker runtime input projection is missing its Artifact volume")
	}
	parts := []string{DockerRuntimeInputProjectionPlanProtocolVersion,
		value.ManifestFingerprint, value.RuntimeBindingFingerprint,
		value.BundleReportFingerprint, value.BundleDigest,
		strconv.FormatInt(value.BundleBytes, 10)}
	var entries int
	var contentBytes, projectionBytes int64
	mountOrdinals := make(map[int]struct{}, value.ReadOnlyMountCount)
	for index, item := range value.Items {
		archive := value.Archives[index]
		if item.Validate() != nil || item.Ordinal != index+1 || archive.ItemOrdinal != item.Ordinal ||
			validateVirtualPath("Docker runtime input projection target", archive.Target) != nil ||
			!validDockerRuntimeInputVolumeName(archive.VolumeName) || len(archive.Data) == 0 ||
			int64(len(archive.Data)) != item.ProjectionArchiveBytes ||
			hashHostInputBytes(archive.Data) != item.ProjectionArchiveDigest ||
			fingerprint("sandbox_docker_runtime_input_target.v1", archive.Target) !=
				item.TargetFingerprint ||
			fingerprint("sandbox_docker_runtime_input_volume_name.v1", archive.VolumeName) !=
				item.VolumeNameFingerprint {
			return errors.New("docker runtime input projection archive is invalid")
		}
		if archive.Target == "/" || (index > 0 && archive.Target <= value.Archives[index-1].Target) {
			return errors.New("docker runtime input projection targets are not strictly ordered")
		}
		if item.Kind == DockerRuntimeInputProjectionKindManifestMount {
			if item.ManifestMountOrdinal > value.ReadOnlyMountCount {
				return errors.New("docker runtime input projection mount ordinal is out of range")
			}
			if _, exists := mountOrdinals[item.ManifestMountOrdinal]; exists {
				return errors.New("docker runtime input projection mount ordinal is duplicated")
			}
			mountOrdinals[item.ManifestMountOrdinal] = struct{}{}
		}
		for previous := 0; previous < index; previous++ {
			other := value.Archives[previous].Target
			if pathWithin(archive.Target, other) || pathWithin(other, archive.Target) {
				return errors.New("docker runtime input projection targets overlap")
			}
		}
		entries += item.EntryCount
		contentBytes += item.ContentBytes
		projectionBytes += item.ProjectionArchiveBytes
		parts = append(parts, item.ItemFingerprint)
	}
	if len(mountOrdinals) != value.ReadOnlyMountCount ||
		entries != value.TotalEntryCount || contentBytes != value.TotalContentBytes ||
		projectionBytes != value.TotalProjectionBytes ||
		fingerprint(parts...) != value.ProjectionSetFingerprint {
		return errors.New("docker runtime input projection aggregate changed")
	}
	return nil
}

type DockerRuntimeInputProjectionPlan struct {
	ID                           string
	HandoffID                    string
	HandoffIntentID              string
	AttemptID                    string
	ContainerPlanID              string
	RunID                        string
	MissionID                    string
	WorkspaceID                  string
	ProtocolVersion              string
	Status                       string
	TrustClass                   string
	OperationKeyDigest           string
	ManifestFingerprint          string
	MountBindingFingerprint      string
	InputArtifactDigest          string
	AuthorityFingerprint         string
	SpecFingerprint              string
	ContainerPlanFingerprint     string
	HandoffFingerprint           string
	HandoffTransportFingerprint  string
	BundleReportFingerprint      string
	BundleDigest                 string
	BundleBytes                  int64
	ReadOnlyMountCount           int
	InputArtifactCount           int
	ProjectionCount              int
	DirectoryRootCount           int
	FileRootCount                int
	TotalEntryCount              int
	TotalContentBytes            int64
	TotalProjectionBytes         int64
	ProjectionSetFingerprint     string
	RequestFingerprint           string
	ProjectionFingerprint        string
	OperatorConfirmed            bool
	ExactTargetBinding           bool
	AllVolumesReadOnly           bool
	AllVolumesNoCopy             bool
	BundleRecaptured             bool
	BundleDigestMatched          bool
	DaemonContacted              bool
	DaemonApplied                bool
	ContainerStarted             bool
	ProcessExecuted              bool
	OutputExported               bool
	ProductionExecutionSubmitted bool
	ProductionVerified           bool
	BackendEnabled               bool
	ExecutionAuthorized          bool
	ArtifactCommitAuthorized     bool
	Items                        []DockerRuntimeInputProjectionItem
	RequestedBy                  string
	CreatedAt                    time.Time
	Replayed                     bool
}

func NewDockerRuntimeInputProjectionPlan(id, operationKeyDigest string,
	attempt DockerContainerRehearsalAttempt, containerPlan DockerContainerPlan,
	handoff DockerHostInputHandoffRecord, compilation DockerRuntimeInputProjectionCompilation,
	operatorConfirmed bool, requestedBy string, now time.Time,
) (DockerRuntimeInputProjectionPlan, error) {
	if attempt.Validate() != nil || containerPlan.Validate() != nil || handoff.Validate() != nil ||
		compilation.Validate() != nil || attempt.Completion == nil || handoff.Handoff == nil ||
		handoff.Intent.AttemptID != attempt.Intent.ID || handoff.Intent.PlanID != containerPlan.ID ||
		handoff.Handoff.AttemptID != attempt.Intent.ID ||
		handoff.Handoff.Result.BundleReportFingerprint != compilation.BundleReportFingerprint ||
		handoff.Handoff.Result.BundleDigest != compilation.BundleDigest ||
		compilation.RuntimeBindingFingerprint != handoff.Handoff.HandoffFingerprint ||
		handoff.Intent.BundleBytes != compilation.BundleBytes ||
		compilation.ManifestFingerprint != containerPlan.ManifestFingerprint ||
		compilation.ReadOnlyMountCount != containerPlan.ReadOnlyMountCount ||
		compilation.InputArtifactCount != containerPlan.InputArtifactCount || !operatorConfirmed ||
		now.Before(attempt.Completion.CompletedAt) || now.Before(handoff.Handoff.CreatedAt) ||
		requestedBy != containerPlan.RequestedBy || requestedBy != attempt.Intent.RequestedBy {
		return DockerRuntimeInputProjectionPlan{}, errors.New("docker runtime input projection authority is invalid")
	}
	value := DockerRuntimeInputProjectionPlan{
		ID: id, HandoffID: handoff.Handoff.ID, HandoffIntentID: handoff.Intent.ID,
		AttemptID: attempt.Intent.ID, ContainerPlanID: containerPlan.ID,
		RunID: containerPlan.RunID, MissionID: containerPlan.MissionID,
		WorkspaceID:                 containerPlan.WorkspaceID,
		ProtocolVersion:             DockerRuntimeInputProjectionPlanProtocolVersion,
		Status:                      DockerRuntimeInputProjectionStatusCompiled,
		TrustClass:                  DockerRuntimeInputProjectionTrustClass,
		OperationKeyDigest:          operationKeyDigest,
		ManifestFingerprint:         compilation.ManifestFingerprint,
		MountBindingFingerprint:     containerPlan.MountBindingFingerprint,
		InputArtifactDigest:         containerPlan.InputArtifactDigest,
		AuthorityFingerprint:        containerPlan.AuthorityFingerprint,
		SpecFingerprint:             containerPlan.SpecFingerprint,
		ContainerPlanFingerprint:    containerPlan.PlanFingerprint,
		HandoffFingerprint:          handoff.Handoff.HandoffFingerprint,
		HandoffTransportFingerprint: handoff.Handoff.Result.TransportFingerprint,
		BundleReportFingerprint:     compilation.BundleReportFingerprint,
		BundleDigest:                compilation.BundleDigest, BundleBytes: compilation.BundleBytes,
		ReadOnlyMountCount:       compilation.ReadOnlyMountCount,
		InputArtifactCount:       compilation.InputArtifactCount,
		ProjectionCount:          len(compilation.Items),
		DirectoryRootCount:       compilation.DirectoryRootCount,
		FileRootCount:            compilation.FileRootCount,
		TotalEntryCount:          compilation.TotalEntryCount,
		TotalContentBytes:        compilation.TotalContentBytes,
		TotalProjectionBytes:     compilation.TotalProjectionBytes,
		ProjectionSetFingerprint: compilation.ProjectionSetFingerprint,
		OperatorConfirmed:        operatorConfirmed,
		ExactTargetBinding:       true, AllVolumesReadOnly: true, AllVolumesNoCopy: true,
		BundleRecaptured: true, BundleDigestMatched: true,
		Items:       append([]DockerRuntimeInputProjectionItem(nil), compilation.Items...),
		RequestedBy: requestedBy, CreatedAt: now.UTC(),
	}
	value.RequestFingerprint = dockerRuntimeInputProjectionRequestFingerprint(value)
	value.ProjectionFingerprint = dockerRuntimeInputProjectionPlanFingerprint(value)
	return value, value.Validate()
}

func (value DockerRuntimeInputProjectionPlan) Validate() error {
	for _, identity := range []string{value.ID, value.HandoffID, value.HandoffIntentID,
		value.AttemptID, value.ContainerPlanID, value.RunID, value.MissionID,
		value.WorkspaceID, value.RequestedBy} {
		if validateStoredIdentity("Docker runtime input projection identity", identity) != nil {
			return errors.New("docker runtime input projection identity is invalid")
		}
	}
	for _, digest := range []string{value.OperationKeyDigest, value.ManifestFingerprint,
		value.MountBindingFingerprint, value.InputArtifactDigest,
		value.AuthorityFingerprint, value.SpecFingerprint, value.ContainerPlanFingerprint,
		value.HandoffFingerprint, value.HandoffTransportFingerprint,
		value.BundleReportFingerprint, value.BundleDigest, value.ProjectionSetFingerprint,
		value.RequestFingerprint, value.ProjectionFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input projection digest is invalid")
		}
	}
	if value.ProtocolVersion != DockerRuntimeInputProjectionPlanProtocolVersion ||
		value.Status != DockerRuntimeInputProjectionStatusCompiled ||
		value.TrustClass != DockerRuntimeInputProjectionTrustClass || value.BundleBytes < 1 ||
		value.BundleBytes > MaxHostInputBundleBytes || value.ReadOnlyMountCount < 1 ||
		value.ReadOnlyMountCount > MaxMounts || value.InputArtifactCount < 0 ||
		value.InputArtifactCount > MaxInputArtifacts || value.ProjectionCount != len(value.Items) ||
		value.ProjectionCount < value.ReadOnlyMountCount ||
		value.ProjectionCount > MaxDockerRuntimeInputProjections ||
		value.DirectoryRootCount != value.ReadOnlyMountCount || value.FileRootCount != 0 ||
		value.TotalEntryCount < value.ReadOnlyMountCount ||
		value.TotalEntryCount > MaxHostInputBundleEntries || value.TotalContentBytes < 0 ||
		value.TotalContentBytes > MaxHostInputSourceBytes+MaxInputArtifactTotalBytes ||
		value.TotalProjectionBytes < 1 ||
		value.TotalProjectionBytes > int64(MaxDockerRuntimeInputProjections)*MaxHostInputBundleBytes ||
		value.ProjectionCount != value.ReadOnlyMountCount+boolCount(value.InputArtifactCount > 0) ||
		value.CreatedAt.IsZero() ||
		!value.OperatorConfirmed || !value.ExactTargetBinding ||
		!value.AllVolumesReadOnly || !value.AllVolumesNoCopy ||
		!value.BundleRecaptured || !value.BundleDigestMatched || value.DaemonContacted ||
		value.DaemonApplied || value.ContainerStarted || value.ProcessExecuted ||
		value.OutputExported || value.ProductionExecutionSubmitted || value.ProductionVerified ||
		value.BackendEnabled || value.ExecutionAuthorized || value.ArtifactCommitAuthorized {
		return errors.New("docker runtime input projection plan widened execution authority")
	}
	parts := []string{DockerRuntimeInputProjectionPlanProtocolVersion,
		value.ManifestFingerprint, value.HandoffFingerprint,
		value.BundleReportFingerprint, value.BundleDigest,
		strconv.FormatInt(value.BundleBytes, 10)}
	var manifestMounts, artifactVolumes, totalEntries int
	var totalContentBytes, totalProjectionBytes int64
	mountOrdinals := make(map[int]struct{}, value.ReadOnlyMountCount)
	for index, item := range value.Items {
		if item.Validate() != nil || item.Ordinal != index+1 {
			return errors.New("docker runtime input projection item sequence is invalid")
		}
		switch item.Kind {
		case DockerRuntimeInputProjectionKindManifestMount:
			if item.ManifestMountOrdinal > value.ReadOnlyMountCount {
				return errors.New("docker runtime input projection mount ordinal is out of range")
			}
			if _, exists := mountOrdinals[item.ManifestMountOrdinal]; exists {
				return errors.New("docker runtime input projection mount ordinal is duplicated")
			}
			mountOrdinals[item.ManifestMountOrdinal] = struct{}{}
			manifestMounts++
		case DockerRuntimeInputProjectionKindArtifacts:
			artifactVolumes++
		}
		totalEntries += item.EntryCount
		totalContentBytes += item.ContentBytes
		totalProjectionBytes += item.ProjectionArchiveBytes
		parts = append(parts, item.ItemFingerprint)
	}
	if manifestMounts != value.ReadOnlyMountCount ||
		len(mountOrdinals) != value.ReadOnlyMountCount ||
		artifactVolumes != boolCount(value.InputArtifactCount > 0) ||
		totalEntries != value.TotalEntryCount ||
		totalContentBytes != value.TotalContentBytes ||
		totalProjectionBytes != value.TotalProjectionBytes ||
		fingerprint(parts...) != value.ProjectionSetFingerprint ||
		value.RequestFingerprint != dockerRuntimeInputProjectionRequestFingerprint(value) ||
		value.ProjectionFingerprint != dockerRuntimeInputProjectionPlanFingerprint(value) {
		return errors.New("docker runtime input projection fingerprint is invalid")
	}
	return nil
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func dockerRuntimeInputProjectionRequestFingerprint(value DockerRuntimeInputProjectionPlan) string {
	return fingerprint(DockerRuntimeInputProjectionPlanProtocolVersion, value.HandoffID,
		value.HandoffIntentID, value.AttemptID, value.ContainerPlanID, value.RunID,
		value.MissionID, value.WorkspaceID, value.OperationKeyDigest,
		value.ManifestFingerprint, value.MountBindingFingerprint, value.InputArtifactDigest,
		value.AuthorityFingerprint, value.SpecFingerprint, value.ContainerPlanFingerprint,
		value.HandoffFingerprint, value.HandoffTransportFingerprint,
		value.BundleReportFingerprint, value.BundleDigest, strconv.FormatInt(value.BundleBytes, 10),
		value.ProjectionSetFingerprint, strconv.FormatBool(value.OperatorConfirmed),
		value.RequestedBy)
}

func dockerRuntimeInputProjectionPlanFingerprint(value DockerRuntimeInputProjectionPlan) string {
	parts := []string{DockerRuntimeInputProjectionPlanProtocolVersion,
		dockerRuntimeInputProjectionRequestFingerprint(value), value.Status, value.TrustClass,
		strconv.Itoa(value.ReadOnlyMountCount), strconv.Itoa(value.InputArtifactCount),
		strconv.Itoa(value.ProjectionCount), strconv.Itoa(value.DirectoryRootCount),
		strconv.Itoa(value.FileRootCount), strconv.Itoa(value.TotalEntryCount),
		strconv.FormatInt(value.TotalContentBytes, 10),
		strconv.FormatInt(value.TotalProjectionBytes, 10), value.ProjectionSetFingerprint,
		strconv.FormatBool(value.OperatorConfirmed),
		strconv.FormatBool(value.ExactTargetBinding), strconv.FormatBool(value.AllVolumesReadOnly),
		strconv.FormatBool(value.AllVolumesNoCopy), strconv.FormatBool(value.BundleRecaptured),
		strconv.FormatBool(value.BundleDigestMatched), strconv.FormatBool(value.DaemonContacted),
		strconv.FormatBool(value.DaemonApplied), strconv.FormatBool(value.ContainerStarted),
		strconv.FormatBool(value.ProcessExecuted), strconv.FormatBool(value.OutputExported),
		strconv.FormatBool(value.ProductionExecutionSubmitted),
		strconv.FormatBool(value.ProductionVerified), strconv.FormatBool(value.BackendEnabled),
		strconv.FormatBool(value.ExecutionAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized)}
	for _, item := range value.Items {
		parts = append(parts, item.ItemFingerprint)
	}
	return fingerprint(parts...)
}

type DockerRuntimeInputProjectionOperation struct {
	KeyDigest          string
	ProjectionID       string
	HandoffID          string
	ContainerPlanID    string
	RunID              string
	RequestFingerprint string
	RequestedBy        string
	CreatedAt          time.Time
}

func NewDockerRuntimeInputProjectionOperation(keyDigest string,
	plan DockerRuntimeInputProjectionPlan,
) (DockerRuntimeInputProjectionOperation, error) {
	value := DockerRuntimeInputProjectionOperation{KeyDigest: keyDigest,
		ProjectionID: plan.ID, HandoffID: plan.HandoffID,
		ContainerPlanID: plan.ContainerPlanID, RunID: plan.RunID,
		RequestFingerprint: plan.RequestFingerprint, RequestedBy: plan.RequestedBy,
		CreatedAt: plan.CreatedAt}
	return value, value.Validate()
}

func (value DockerRuntimeInputProjectionOperation) Validate() error {
	for _, identity := range []string{value.ProjectionID, value.HandoffID,
		value.ContainerPlanID, value.RunID, value.RequestedBy} {
		if validateStoredIdentity("Docker runtime input projection operation identity", identity) != nil {
			return errors.New("docker runtime input projection operation identity is invalid")
		}
	}
	if !validDigest(value.KeyDigest) || !validDigest(value.RequestFingerprint) ||
		value.CreatedAt.IsZero() {
		return errors.New("docker runtime input projection operation is invalid")
	}
	return nil
}

type runtimeInputBundleEntry struct {
	name    string
	kind    byte
	mode    int64
	content []byte
}

func CompileDockerRuntimeInputProjectionBundle(ctx context.Context, manifest Manifest,
	bundle HostInputBundle, runtimeBindingFingerprint string,
) (DockerRuntimeInputProjectionCompilation, error) {
	if err := ctx.Err(); err != nil {
		return DockerRuntimeInputProjectionCompilation{}, err
	}
	if bundle == nil || !validDigest(runtimeBindingFingerprint) {
		return DockerRuntimeInputProjectionCompilation{}, errors.New("docker runtime input projection bundle and binding are required")
	}
	report := bundle.Report()
	if report.Validate() != nil {
		return DockerRuntimeInputProjectionCompilation{}, errors.New("docker runtime input projection bundle report is invalid")
	}
	normalized, err := NormalizeManifest(manifest)
	if err != nil {
		return DockerRuntimeInputProjectionCompilation{}, err
	}
	manifestFingerprint, err := normalized.Fingerprint()
	if err != nil {
		return DockerRuntimeInputProjectionCompilation{}, err
	}
	if report.ReadOnlyMountCount != runtimeReadOnlyMountCount(normalized) ||
		report.ArtifactCount != len(normalized.InputArtifactIDs) {
		return DockerRuntimeInputProjectionCompilation{}, errors.New("docker runtime input projection report does not match the Manifest")
	}
	data, err := readExactHostInputBundle(bundle, report)
	if err != nil {
		return DockerRuntimeInputProjectionCompilation{}, err
	}
	entries, err := parseRuntimeInputBundle(ctx, data, report)
	if err != nil {
		return DockerRuntimeInputProjectionCompilation{}, err
	}

	groups := make(map[string][]runtimeInputBundleEntry, report.ReadOnlyMountCount+1)
	for _, entry := range entries {
		root, rootErr := runtimeInputArchiveRoot(entry.name)
		if rootErr != nil {
			return DockerRuntimeInputProjectionCompilation{}, rootErr
		}
		groups[root] = append(groups[root], entry)
	}
	readOnlyMounts := make([]Mount, 0, report.ReadOnlyMountCount)
	for _, mount := range normalized.Mounts {
		if mount.Access == MountReadOnly {
			readOnlyMounts = append(readOnlyMounts, mount)
		}
	}
	archives := make([]DockerRuntimeInputProjectionArchive, 0,
		report.ReadOnlyMountCount+1)
	items := make([]DockerRuntimeInputProjectionItem, 0, cap(archives))
	for index, mount := range readOnlyMounts {
		root := fmt.Sprintf("mounts/%03d", index+1)
		group := groups[root]
		if len(group) == 0 || group[0].name != root || group[0].kind != tar.TypeDir {
			return DockerRuntimeInputProjectionCompilation{}, errors.New("docker runtime input projection requires directory-root read-only mounts")
		}
		delete(groups, root)
		item, archive, buildErr := buildRuntimeInputProjection(index+1,
			DockerRuntimeInputProjectionKindManifestMount, index+1, mount.Target, root, group,
			manifestFingerprint, runtimeBindingFingerprint, report.BundleDigest)
		if buildErr != nil {
			return DockerRuntimeInputProjectionCompilation{}, buildErr
		}
		items = append(items, item)
		archives = append(archives, archive)
	}
	if report.ArtifactCount > 0 {
		group := groups["artifacts"]
		if len(group) != report.ArtifactCount {
			return DockerRuntimeInputProjectionCompilation{}, errors.New("docker runtime Artifact projection is incomplete")
		}
		delete(groups, "artifacts")
		item, archive, buildErr := buildRuntimeInputProjection(len(items)+1,
			DockerRuntimeInputProjectionKindArtifacts, 0, DockerRuntimeArtifactTarget,
			"artifacts", group, manifestFingerprint, runtimeBindingFingerprint,
			report.BundleDigest)
		if buildErr != nil {
			return DockerRuntimeInputProjectionCompilation{}, buildErr
		}
		items = append(items, item)
		archives = append(archives, archive)
	}
	if len(groups) != 0 {
		return DockerRuntimeInputProjectionCompilation{}, errors.New("docker runtime input bundle contains an unexpected projection root")
	}
	// Rebuild in target order without relying on transient fingerprints as map keys.
	type pair struct {
		item    DockerRuntimeInputProjectionItem
		archive DockerRuntimeInputProjectionArchive
	}
	pairs := make([]pair, len(archives))
	for index := range archives {
		pairs[index] = pair{item: items[index], archive: archives[index]}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].archive.Target < pairs[j].archive.Target })
	items = items[:0]
	archives = archives[:0]
	parts := []string{DockerRuntimeInputProjectionPlanProtocolVersion, manifestFingerprint,
		runtimeBindingFingerprint,
		report.ReportFingerprint, report.BundleDigest, strconv.FormatInt(report.BundleBytes, 10)}
	var totalEntries int
	var totalContent, totalProjection int64
	for index, pair := range pairs {
		pair.item.Ordinal = index + 1
		pair.archive.ItemOrdinal = index + 1
		pair.item.ItemFingerprint = dockerRuntimeInputProjectionItemFingerprint(pair.item)
		items = append(items, pair.item)
		archives = append(archives, pair.archive)
		totalEntries += pair.item.EntryCount
		totalContent += pair.item.ContentBytes
		totalProjection += pair.item.ProjectionArchiveBytes
		parts = append(parts, pair.item.ItemFingerprint)
	}
	value := DockerRuntimeInputProjectionCompilation{
		ManifestFingerprint:       manifestFingerprint,
		RuntimeBindingFingerprint: runtimeBindingFingerprint,
		BundleReportFingerprint:   report.ReportFingerprint,
		BundleDigest:              report.BundleDigest, BundleBytes: report.BundleBytes,
		ReadOnlyMountCount: report.ReadOnlyMountCount, InputArtifactCount: report.ArtifactCount,
		DirectoryRootCount: report.ReadOnlyMountCount, TotalEntryCount: totalEntries,
		TotalContentBytes: totalContent, TotalProjectionBytes: totalProjection,
		ProjectionSetFingerprint: fingerprint(parts...), Items: items, Archives: archives,
	}
	return value, value.Validate()
}

func runtimeReadOnlyMountCount(manifest Manifest) int {
	count := 0
	for _, mount := range manifest.Mounts {
		if mount.Access == MountReadOnly {
			count++
		}
	}
	return count
}

func parseRuntimeInputBundle(ctx context.Context, data []byte,
	report HostInputBundleReport,
) ([]runtimeInputBundleEntry, error) {
	reader := tar.NewReader(bytes.NewReader(data))
	entries := make([]runtimeInputBundleEntry, 0, report.EntryCount)
	seen := make(map[string]byte, report.EntryCount)
	sourceParts := []string{"sandbox_host_input_source_snapshot.v1",
		strconv.Itoa(report.RegularFileCount + report.DirectoryCount)}
	var sourceBytes, artifactBytes int64
	regularFiles, directories, artifacts := 0, 0, 0
	previousMount := ""
	artifactPhase := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || header == nil || len(entries) >= MaxHostInputBundleEntries {
			return nil, errors.New("docker runtime input bundle tar is invalid")
		}
		name := strings.TrimSuffix(header.Name, "/")
		if validateRuntimeInputArchiveName(name) != nil || strings.Contains(name, "\\") ||
			header.Format != tar.FormatPAX ||
			header.Linkname != "" ||
			header.Uid != 65532 || header.Gid != 65532 || header.Uname != "" ||
			header.Gname != "" || header.Devmajor != 0 || header.Devminor != 0 ||
			!runtimeInputPAXRecordsAllowed(header) ||
			!header.ModTime.Equal(time.Unix(0, 0).UTC()) ||
			!header.AccessTime.Equal(time.Unix(0, 0).UTC()) ||
			!header.ChangeTime.Equal(time.Unix(0, 0).UTC()) {
			return nil, errors.New("docker runtime input bundle header is not canonical")
		}
		if _, exists := seen[name]; exists {
			return nil, errors.New("docker runtime input bundle path is duplicated")
		}
		entry := runtimeInputBundleEntry{name: name, kind: header.Typeflag, mode: header.Mode}
		switch header.Typeflag {
		case tar.TypeDir:
			if !strings.HasSuffix(header.Name, "/") || header.Mode != 0o555 || header.Size != 0 {
				return nil, errors.New("docker runtime input directory header is invalid")
			}
			directories++
		case tar.TypeReg:
			if strings.HasSuffix(header.Name, "/") || header.Mode != 0o444 || header.Size < 0 ||
				header.Size > MaxHostInputSourceBytes+MaxInputArtifactTotalBytes {
				return nil, errors.New("docker runtime input file header is invalid")
			}
			content, readErr := io.ReadAll(io.LimitReader(reader, header.Size+1))
			if readErr != nil || int64(len(content)) != header.Size {
				return nil, errors.New("docker runtime input file body is invalid")
			}
			entry.content = content
		default:
			return nil, errors.New("docker runtime input bundle contains a forbidden tar entry")
		}
		if parent := path.Dir(name); parent != "." && parent != "mounts" && parent != "artifacts" {
			if kind, exists := seen[parent]; !exists || kind != tar.TypeDir {
				return nil, errors.New("docker runtime input bundle parent directory is missing")
			}
		}
		root, rootErr := runtimeInputArchiveRoot(name)
		if rootErr != nil {
			return nil, rootErr
		}
		if root == "artifacts" {
			artifactPhase = true
			if header.Typeflag != tar.TypeReg || len(entry.content) == 0 ||
				name != fmt.Sprintf("artifacts/%03d", artifacts+1) {
				return nil, errors.New("docker runtime Artifact sequence is invalid")
			}
			artifacts++
			artifactBytes += int64(len(entry.content))
		} else {
			if artifactPhase || (previousMount != "" && name <= previousMount) {
				return nil, errors.New("docker runtime mount entries are not strictly ordered")
			}
			previousMount = name
			if header.Typeflag == tar.TypeReg {
				regularFiles++
				sourceBytes += int64(len(entry.content))
			}
			digest := fingerprint("sandbox_host_input_directory.v1", name)
			if header.Typeflag == tar.TypeReg {
				digest = hashHostInputBytes(entry.content)
			}
			sourceParts = append(sourceParts,
				fingerprint("sandbox_host_input_archive_path.v1", name),
				strconv.Itoa(int(header.Typeflag)), strconv.FormatInt(header.Size, 10), digest)
		}
		seen[name] = header.Typeflag
		entries = append(entries, entry)
	}
	canonical, err := encodeCanonicalRuntimeInputBundle(entries)
	if err != nil || !bytes.Equal(canonical, data) {
		return nil, errors.New("docker runtime input bundle bytes are not canonical")
	}
	if len(entries) != report.EntryCount || regularFiles != report.RegularFileCount ||
		directories != report.DirectoryCount || artifacts != report.ArtifactCount ||
		sourceBytes != report.SourceBytes || artifactBytes != report.ArtifactBytes ||
		fingerprint(sourceParts...) != report.SourceSnapshotDigest {
		return nil, errors.New("docker runtime input bundle measurements changed")
	}
	return entries, nil
}

func encodeCanonicalRuntimeInputBundle(entries []runtimeInputBundleEntry) ([]byte, error) {
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Typeflag: entry.kind, Mode: entry.mode,
			ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(),
			ChangeTime: time.Unix(0, 0).UTC(), Uid: 65532, Gid: 65532,
			Uname: "", Gname: "", Format: tar.FormatPAX}
		if entry.kind == tar.TypeDir {
			header.Name += "/"
		} else {
			header.Size = int64(len(entry.content))
		}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		if len(entry.content) > 0 {
			if _, err := writer.Write(entry.content); err != nil {
				return nil, err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func runtimeInputPAXRecordsAllowed(header *tar.Header) bool {
	if header == nil {
		return false
	}
	for name, value := range header.PAXRecords {
		switch name {
		case "atime", "ctime":
			if value != "0" && value != "0.000000000" {
				return false
			}
		case "path":
			if value != header.Name {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func runtimeInputArchiveRoot(name string) (string, error) {
	parts := strings.Split(name, "/")
	if len(parts) < 2 {
		return "", errors.New("docker runtime input bundle root is invalid")
	}
	if parts[0] == "artifacts" {
		if len(parts) != 2 {
			return "", errors.New("docker runtime Artifact path is invalid")
		}
		return "artifacts", nil
	}
	if parts[0] != "mounts" || len(parts[1]) != 3 {
		return "", errors.New("docker runtime mount archive root is invalid")
	}
	ordinal, err := strconv.Atoi(parts[1])
	if err != nil || ordinal < 1 || ordinal > MaxMounts ||
		fmt.Sprintf("%03d", ordinal) != parts[1] {
		return "", errors.New("docker runtime mount archive ordinal is invalid")
	}
	return strings.Join(parts[:2], "/"), nil
}

func buildRuntimeInputProjection(ordinal int, kind string, mountOrdinal int, target, root string,
	entries []runtimeInputBundleEntry, manifestFingerprint, runtimeBindingFingerprint,
	bundleDigest string,
) (DockerRuntimeInputProjectionItem, DockerRuntimeInputProjectionArchive, error) {
	if validateVirtualPath("Docker runtime input projection target", target) != nil || target == "/" ||
		(kind == DockerRuntimeInputProjectionKindManifestMount &&
			(pathWithin(target, DockerHostInputCarrierDestination) ||
				pathWithin(DockerHostInputCarrierDestination, target))) {
		return DockerRuntimeInputProjectionItem{}, DockerRuntimeInputProjectionArchive{},
			errors.New("docker runtime input projection target is reserved or invalid")
	}
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	parts := []string{"sandbox_docker_runtime_input_projection_content.v1", kind,
		fingerprint("sandbox_docker_runtime_input_target.v1", target),
		fingerprint("sandbox_docker_runtime_input_archive_root.v1", root)}
	regularFiles, directories := 0, 0
	var contentBytes int64
	for _, entry := range entries {
		relative := strings.TrimPrefix(entry.name, root)
		relative = strings.TrimPrefix(relative, "/")
		if kind == DockerRuntimeInputProjectionKindManifestMount && relative == "" {
			directories++
			parts = append(parts, fingerprint("sandbox_docker_runtime_input_relative_path.v1", "."),
				strconv.Itoa(int(entry.kind)), "0",
				fingerprint("sandbox_host_input_directory.v1", entry.name))
			continue
		}
		if validateRuntimeInputArchiveName(relative) != nil {
			return DockerRuntimeInputProjectionItem{}, DockerRuntimeInputProjectionArchive{},
				errors.New("docker runtime input projection relative path is invalid")
		}
		header := &tar.Header{Name: relative, Typeflag: entry.kind, Mode: entry.mode,
			ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(),
			ChangeTime: time.Unix(0, 0).UTC(), Uid: 65532, Gid: 65532,
			Format: tar.FormatPAX}
		digest := fingerprint("sandbox_host_input_directory.v1", entry.name)
		if entry.kind == tar.TypeDir {
			directories++
			header.Name += "/"
		} else {
			regularFiles++
			header.Size = int64(len(entry.content))
			contentBytes += header.Size
			digest = hashHostInputBytes(entry.content)
		}
		if err := writer.WriteHeader(header); err != nil {
			return DockerRuntimeInputProjectionItem{}, DockerRuntimeInputProjectionArchive{}, err
		}
		if len(entry.content) > 0 {
			if _, err := writer.Write(entry.content); err != nil {
				return DockerRuntimeInputProjectionItem{}, DockerRuntimeInputProjectionArchive{}, err
			}
		}
		parts = append(parts, fingerprint("sandbox_docker_runtime_input_relative_path.v1", relative),
			strconv.Itoa(int(entry.kind)), strconv.FormatInt(header.Size, 10), digest)
	}
	if err := writer.Close(); err != nil || output.Len() == 0 ||
		int64(output.Len()) > MaxHostInputBundleBytes {
		return DockerRuntimeInputProjectionItem{}, DockerRuntimeInputProjectionArchive{},
			errors.New("docker runtime input projection archive is outside bounds")
	}
	volumeName := dockerRuntimeInputProjectionVolumeName(manifestFingerprint,
		runtimeBindingFingerprint, bundleDigest, kind, mountOrdinal, target)
	item := DockerRuntimeInputProjectionItem{
		Ordinal: ordinal, ProtocolVersion: DockerRuntimeInputProjectionItemProtocolVersion,
		Kind: kind, ManifestMountOrdinal: mountOrdinal,
		TargetFingerprint:      fingerprint("sandbox_docker_runtime_input_target.v1", target),
		ArchiveRootFingerprint: fingerprint("sandbox_docker_runtime_input_archive_root.v1", root),
		VolumeNameFingerprint:  fingerprint("sandbox_docker_runtime_input_volume_name.v1", volumeName),
		EntryCount:             len(entries), RegularFileCount: regularFiles, DirectoryCount: directories,
		ContentBytes: contentBytes, ProjectionArchiveBytes: int64(output.Len()),
		ContentDigest: fingerprint(parts...), ProjectionArchiveDigest: hashHostInputBytes(output.Bytes()),
		RootDirectory: true, ReadOnly: true, ExactTarget: true, NoCopy: true,
	}
	item.ItemFingerprint = dockerRuntimeInputProjectionItemFingerprint(item)
	archive := DockerRuntimeInputProjectionArchive{ItemOrdinal: ordinal, Target: target,
		VolumeName: volumeName, Data: append([]byte(nil), output.Bytes()...)}
	return item, archive, item.Validate()
}

func dockerRuntimeInputProjectionVolumeName(manifestFingerprint, runtimeBindingFingerprint,
	bundleDigest, kind string, mountOrdinal int, target string,
) string {
	seed := fingerprint("sandbox_docker_runtime_input_volume.v1", manifestFingerprint,
		runtimeBindingFingerprint, bundleDigest, kind, strconv.Itoa(mountOrdinal), target)
	return "cyberagent-runtime-" + seed[:24]
}

func validDockerRuntimeInputVolumeName(value string) bool {
	const prefix = "cyberagent-runtime-"
	if len(value) != len(prefix)+24 || !strings.HasPrefix(value, prefix) {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil
}

func validateRuntimeInputArchiveName(value string) error {
	if value == "" || !utf8.ValidString(value) || len([]byte(value)) > MaxHostInputPathBytes ||
		strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") ||
		path.Clean(value) != value || value == "." || value == ".." ||
		strings.HasPrefix(value, "../") {
		return errors.New("docker runtime input archive name is invalid")
	}
	return nil
}
