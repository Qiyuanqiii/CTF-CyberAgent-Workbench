package application

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/skills"
)

type SkillPackageRegistryStore interface {
	PreparePackageInstallation(context.Context, skills.PackageInstallation,
		skills.PackageInstallOperation) (skills.PackageInstallation,
		*skills.PackageInstallResult, bool, error)
	CompletePackageInstallation(context.Context, skills.PackageInstallResult) (
		skills.InstalledPackage, bool, error)
	GetPackageInstallOperation(context.Context, string) (
		skills.PackageInstallOperation, bool, error)
	GetPackageInstallation(context.Context, string) (skills.PackageInstallation, error)
	GetInstalledPackageByRef(context.Context, string, string) (
		skills.InstalledPackage, bool, error)
	ListInstalledPackages(context.Context, domain.ExecutionSurface, domain.Profile, bool) (
		[]skills.InstalledPackage, error)
	CreatePackageRemoval(context.Context, skills.PackageRemoval,
		skills.PackageRemoveOperation) (skills.PackageRemoval, bool, error)
	GetPackageRemoveOperation(context.Context, string) (
		skills.PackageRemoveOperation, bool, error)
}

type SkillPackageRegistryService struct {
	store    SkillPackageRegistryStore
	objects  skills.PackageObjectStore
	builtins *skills.Registry
}

type ImportSkillPackageRequest struct {
	Raw              []byte
	Surface          domain.ExecutionSurface
	OperationKey     string
	InstalledBy      string
	ConfirmUntrusted bool
}

type ImportSkillPackageResult struct {
	Package          skills.InstalledPackage
	Replayed         bool
	RecoveredPending bool
}

type ListInstalledSkillPackagesRequest struct {
	Surface        domain.ExecutionSurface
	Profile        domain.Profile
	IncludeRemoved bool
}

type RemoveSkillPackageRequest struct {
	Name          string
	Version       string
	OperationKey  string
	RemovedBy     string
	ConfirmRemove bool
}

type RemoveSkillPackageResult struct {
	Removal  skills.PackageRemoval
	Replayed bool
}

func NewSkillPackageRegistryService(store SkillPackageRegistryStore,
	objects skills.PackageObjectStore, builtins *skills.Registry,
) *SkillPackageRegistryService {
	return &SkillPackageRegistryService{store: store, objects: objects, builtins: builtins}
}

func (s *SkillPackageRegistryService) Import(ctx context.Context,
	request ImportSkillPackageRequest,
) (ImportSkillPackageResult, error) {
	if s == nil || s.store == nil || s.objects == nil || s.builtins == nil {
		return ImportSkillPackageResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"Skill package Registry store, object store, and built-in Registry are required")
	}
	if !request.ConfirmUntrusted {
		return ImportSkillPackageResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"untrusted Skill package installation requires explicit operator confirmation")
	}
	surface, err := domain.ParseExecutionSurface(string(request.Surface))
	if err != nil || surface != request.Surface {
		return ImportSkillPackageResult{}, apperror.New(apperror.CodeInvalidArgument,
			"Skill package installation surface is invalid")
	}
	operationKey, err := normalizeSkillPackageOperationKey(request.OperationKey)
	if err != nil {
		return ImportSkillPackageResult{}, err
	}
	installedBy, err := normalizeSkillPackageActor(request.InstalledBy, "cli_operator")
	if err != nil {
		return ImportSkillPackageResult{}, err
	}
	parsed, err := skills.ParsePackage(request.Raw)
	if err != nil {
		return ImportSkillPackageResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Skill package failed strict validation", err)
	}
	preview := parsed.Preview()
	if _, reserved := s.builtins.Get(preview.Manifest.Name); reserved {
		return ImportSkillPackageResult{}, apperror.New(apperror.CodeConflict,
			"external Skill packages cannot use a built-in Skill name")
	}
	keyDigest := runmutation.Fingerprint("skill_package_install_operation.v1", operationKey)
	now := time.Now().UTC()
	candidate, err := skills.NewPackageInstallation(idgen.New("skill-install"), parsed,
		surface, keyDigest, installedBy, now)
	if err != nil {
		return ImportSkillPackageResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Skill package installation intent is invalid", err)
	}
	operation := skills.PackageInstallOperation{
		KeyDigest: keyDigest, RequestFingerprint: candidate.RequestFingerprint,
		InstallationID: candidate.ID, Name: candidate.Name, Version: candidate.Version,
		Surface: candidate.Surface, InstalledBy: candidate.InstalledBy,
		CreatedAt: candidate.CreatedAt,
	}
	if existing, found, lookupErr := s.store.GetPackageInstallOperation(ctx,
		keyDigest); lookupErr != nil {
		return ImportSkillPackageResult{}, apperror.Normalize(lookupErr)
	} else if found {
		if existing.RequestFingerprint != candidate.RequestFingerprint ||
			existing.Name != candidate.Name || existing.Version != candidate.Version ||
			existing.Surface != candidate.Surface || existing.InstalledBy != candidate.InstalledBy {
			return ImportSkillPackageResult{}, apperror.New(apperror.CodeConflict,
				"Skill package installation operation key was already used for different intent")
		}
		candidate, err = s.store.GetPackageInstallation(ctx, existing.InstallationID)
		if err != nil {
			return ImportSkillPackageResult{}, apperror.Normalize(err)
		}
		operation = existing
	}
	prepared, existingResult, preparedReplay, err := s.store.PreparePackageInstallation(
		ctx, candidate, operation)
	if err != nil {
		return ImportSkillPackageResult{}, apperror.Normalize(err)
	}
	descriptor := skills.DescriptorForInstallation(prepared)
	if existingResult != nil {
		stored, found, lookupErr := s.store.GetInstalledPackageByRef(ctx,
			prepared.Name, prepared.Version)
		if lookupErr != nil {
			return ImportSkillPackageResult{}, apperror.Normalize(lookupErr)
		}
		if !found || stored.Installation.ID != prepared.ID {
			return ImportSkillPackageResult{}, apperror.New(apperror.CodeInternal,
				"stored Skill package installation result binding is invalid")
		}
		if stored.Removal != nil {
			return ImportSkillPackageResult{}, apperror.New(apperror.CodeConflict,
				"removed Skill package cannot be reinstalled without an explicit restore protocol")
		}
		receipt, verifyErr := s.objects.Verify(ctx, descriptor)
		if verifyErr != nil {
			return ImportSkillPackageResult{}, apperror.Wrap(
				apperror.CodeFailedPrecondition,
				"installed Skill package object failed readback verification", verifyErr)
		}
		if receiptErr := skills.ValidatePackageObjectReceipt(descriptor, receipt); receiptErr != nil ||
			receipt.ObjectKey != existingResult.ObjectKey {
			return ImportSkillPackageResult{}, apperror.Wrap(
				apperror.CodeFailedPrecondition,
				"installed Skill package object receipt binding is invalid", receiptErr)
		}
		value := stored
		value.Replayed = true
		if err := value.Validate(); err != nil {
			return ImportSkillPackageResult{}, apperror.Wrap(
				apperror.CodeInternal, "installed Skill package binding is invalid", err)
		}
		return ImportSkillPackageResult{Package: value, Replayed: true}, nil
	}
	receipt, err := s.objects.Put(ctx, request.Raw, descriptor)
	if err != nil {
		return ImportSkillPackageResult{}, apperror.Wrap(
			apperror.CodeFailedPrecondition,
			"Skill package object could not be published and verified", err)
	}
	result, err := skills.NewPackageInstallResult(prepared, receipt, time.Now().UTC())
	if err != nil {
		return ImportSkillPackageResult{}, apperror.Wrap(
			apperror.CodeInternal, "Skill package installation result could not be built", err)
	}
	installed, completedReplay, err := s.store.CompletePackageInstallation(ctx, result)
	if err != nil {
		return ImportSkillPackageResult{}, apperror.Normalize(err)
	}
	installed.Replayed = preparedReplay || completedReplay
	return ImportSkillPackageResult{
		Package: installed, Replayed: installed.Replayed,
		RecoveredPending: preparedReplay && !completedReplay,
	}, nil
}

func (s *SkillPackageRegistryService) List(ctx context.Context,
	request ListInstalledSkillPackagesRequest,
) ([]skills.InstalledPackage, error) {
	if s == nil || s.store == nil || s.objects == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Skill package Registry store and object store are required")
	}
	values, err := s.store.ListInstalledPackages(ctx, request.Surface, request.Profile,
		request.IncludeRemoved)
	if err != nil {
		return nil, apperror.Normalize(err)
	}
	for index := range values {
		descriptor := skills.DescriptorForInstallation(values[index].Installation)
		receipt, verifyErr := s.objects.Verify(ctx, descriptor)
		if verifyErr != nil {
			return nil, apperror.Wrap(apperror.CodeFailedPrecondition,
				"installed Skill package object failed readback verification", verifyErr)
		}
		if receiptErr := skills.ValidatePackageObjectReceipt(descriptor, receipt); receiptErr != nil ||
			receipt.ObjectKey != values[index].Result.ObjectKey {
			return nil, apperror.Wrap(apperror.CodeFailedPrecondition,
				"installed Skill package object receipt binding is invalid", receiptErr)
		}
		values[index] = skills.CloneInstalledPackage(values[index])
	}
	return values, nil
}

func (s *SkillPackageRegistryService) Get(ctx context.Context, name, version string,
) (skills.InstalledPackage, error) {
	if s == nil || s.store == nil || s.objects == nil {
		return skills.InstalledPackage{}, apperror.New(apperror.CodeFailedPrecondition,
			"Skill package Registry store and object store are required")
	}
	name, version = strings.TrimSpace(name), strings.TrimSpace(version)
	value, found, err := s.store.GetInstalledPackageByRef(ctx, name, version)
	if err != nil {
		return skills.InstalledPackage{}, apperror.Normalize(err)
	}
	if !found {
		return skills.InstalledPackage{}, apperror.New(
			apperror.CodeNotFound, "installed Skill package was not found")
	}
	descriptor := skills.DescriptorForInstallation(value.Installation)
	receipt, err := s.objects.Verify(ctx, descriptor)
	if err != nil {
		return skills.InstalledPackage{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"installed Skill package object failed readback verification", err)
	}
	if receiptErr := skills.ValidatePackageObjectReceipt(descriptor, receipt); receiptErr != nil ||
		receipt.ObjectKey != value.Result.ObjectKey {
		return skills.InstalledPackage{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"installed Skill package object receipt binding is invalid", receiptErr)
	}
	return skills.CloneInstalledPackage(value), nil
}

func (s *SkillPackageRegistryService) Remove(ctx context.Context,
	request RemoveSkillPackageRequest,
) (RemoveSkillPackageResult, error) {
	if s == nil || s.store == nil {
		return RemoveSkillPackageResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"Skill package Registry store is required")
	}
	if !request.ConfirmRemove {
		return RemoveSkillPackageResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"Skill package removal requires explicit operator confirmation")
	}
	request.Name, request.Version = strings.TrimSpace(request.Name), strings.TrimSpace(request.Version)
	operationKey, err := normalizeSkillPackageOperationKey(request.OperationKey)
	if err != nil {
		return RemoveSkillPackageResult{}, err
	}
	removedBy, err := normalizeSkillPackageActor(request.RemovedBy, "cli_operator")
	if err != nil {
		return RemoveSkillPackageResult{}, err
	}
	installed, found, err := s.store.GetInstalledPackageByRef(ctx, request.Name, request.Version)
	if err != nil {
		return RemoveSkillPackageResult{}, apperror.Normalize(err)
	}
	if !found {
		return RemoveSkillPackageResult{}, apperror.New(
			apperror.CodeNotFound, "installed Skill package was not found")
	}
	keyDigest := runmutation.Fingerprint("skill_package_remove_operation.v1", operationKey)
	candidate, err := skills.NewPackageRemoval(idgen.New("skill-remove"),
		installed.Installation, keyDigest, removedBy, time.Now().UTC())
	if err != nil {
		return RemoveSkillPackageResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "Skill package removal intent is invalid", err)
	}
	if existing, operationFound, lookupErr := s.store.GetPackageRemoveOperation(ctx,
		keyDigest); lookupErr != nil {
		return RemoveSkillPackageResult{}, apperror.Normalize(lookupErr)
	} else if operationFound {
		if existing.RequestFingerprint != candidate.RequestFingerprint ||
			existing.InstallationID != installed.Installation.ID ||
			existing.Name != candidate.Name || existing.Version != candidate.Version ||
			existing.Surface != candidate.Surface || existing.RemovedBy != candidate.RemovedBy {
			return RemoveSkillPackageResult{}, apperror.New(apperror.CodeConflict,
				"Skill package removal operation key was already used for different intent")
		}
		if installed.Removal == nil || installed.Removal.ID != existing.RemovalID {
			return RemoveSkillPackageResult{}, apperror.New(apperror.CodeInternal,
				"stored Skill package removal operation binding is invalid")
		}
		return RemoveSkillPackageResult{Removal: *installed.Removal, Replayed: true}, nil
	}
	if installed.Removal != nil {
		return RemoveSkillPackageResult{}, apperror.New(
			apperror.CodeConflict, "Skill package already has an immutable removal tombstone")
	}
	operation := skills.PackageRemoveOperation{
		KeyDigest: keyDigest, RequestFingerprint: candidate.RequestFingerprint,
		RemovalID: candidate.ID, InstallationID: candidate.InstallationID,
		Name: candidate.Name, Version: candidate.Version, Surface: candidate.Surface,
		RemovedBy: candidate.RemovedBy, CreatedAt: candidate.CreatedAt,
	}
	removal, replayed, err := s.store.CreatePackageRemoval(ctx, candidate, operation)
	return RemoveSkillPackageResult{Removal: removal, Replayed: replayed},
		apperror.Normalize(err)
}

func normalizeSkillPackageOperationKey(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		len(value) < 16 || len(value) > 256 {
		return "", apperror.New(apperror.CodeInvalidArgument,
			"Skill package operation key must contain between 16 and 256 normalized UTF-8 bytes")
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return "", apperror.New(apperror.CodeInvalidArgument,
				"Skill package operation key contains a forbidden control character")
		}
	}
	return value, nil
}

func normalizeSkillPackageActor(value, fallback string) (string, error) {
	value = strings.TrimSpace(redact.String(value))
	if value == "" {
		value = fallback
	}
	if !utf8.ValidString(value) || len([]rune(value)) > 256 || strings.ContainsRune(value, 0) {
		return "", apperror.New(apperror.CodeInvalidArgument,
			"Skill package operator identity is invalid")
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return "", apperror.New(apperror.CodeInvalidArgument,
				"Skill package operator identity contains a forbidden control character")
		}
	}
	return value, nil
}
