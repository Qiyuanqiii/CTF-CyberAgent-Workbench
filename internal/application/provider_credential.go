package application

import (
	"context"
	"sort"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/credential"
	"cyberagent-workbench/internal/modelregistry"
)

type ProviderCredentialAction string

const (
	ProviderCredentialSet    ProviderCredentialAction = "set"
	ProviderCredentialDelete ProviderCredentialAction = "delete"
)

var managedCredentialProviders = map[string]struct{}{
	"anthropic": {}, "deepseek": {}, "mimo": {},
}

type ProviderCredentialStatus struct {
	ProtocolVersion    string
	Provider           string
	Configured         bool
	StoreKind          string
	StoreAvailable     bool
	PlaintextReturned  bool
	RestartRequired    bool
	RegistryReloaded   bool
	RegistryGeneration uint64
}

type ChangeProviderCredentialRequest struct {
	Version  string
	Provider string
	Action   ProviderCredentialAction
	Secret   string
	Confirm  bool
}

// ProviderCredentialService is deliberately status-only. The secret crosses
// the renderer/HTTP boundary once, is handed directly to the OS-owned Store,
// and is never returned, logged, placed in SQLite, or included in an event.
type ProviderCredentialService struct {
	store         credential.Store
	registry      ProviderRegistryReloader
	routeSettings modelregistry.RouteSettingReader
}

type ProviderRegistryReloader interface {
	Reload(context.Context, modelregistry.RouteSettingReader) (modelregistry.ReloadResult, error)
	Generation() uint64
}

func NewProviderCredentialService(store credential.Store) *ProviderCredentialService {
	return &ProviderCredentialService{store: store}
}

func (s *ProviderCredentialService) WithRegistryReload(registry ProviderRegistryReloader,
	settings modelregistry.RouteSettingReader,
) *ProviderCredentialService {
	if s != nil {
		s.registry = registry
		s.routeSettings = settings
	}
	return s
}

func (s *ProviderCredentialService) List(ctx context.Context) (
	[]ProviderCredentialStatus, error,
) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"Provider credential store is required")
	}
	names := make([]string, 0, len(managedCredentialProviders))
	for name := range managedCredentialProviders {
		names = append(names, name)
	}
	sort.Strings(names)
	configuredValues := make([]bool, 0, len(names))
	for _, name := range names {
		configured, err := s.configured(ctx, name)
		if err != nil {
			return nil, err
		}
		configuredValues = append(configuredValues, configured)
	}
	generation := s.registryGeneration()
	values := make([]ProviderCredentialStatus, 0, len(names))
	for index, name := range names {
		values = append(values, s.status(name, configuredValues[index], false, false,
			generation))
	}
	return values, nil
}

func (s *ProviderCredentialService) Change(ctx context.Context,
	request ChangeProviderCredentialRequest,
) (ProviderCredentialStatus, error) {
	if s == nil || s.store == nil {
		return ProviderCredentialStatus{}, apperror.New(
			apperror.CodeFailedPrecondition, "Provider credential store is required")
	}
	originalProvider := request.Provider
	request.Provider = strings.TrimSpace(request.Provider)
	if request.Version != credential.ProtocolVersion || !request.Confirm ||
		originalProvider != request.Provider || !managedProvider(request.Provider) {
		return ProviderCredentialStatus{}, apperror.New(apperror.CodeInvalidArgument,
			"Provider credential change request is invalid")
	}
	if !s.store.Available() {
		return ProviderCredentialStatus{}, apperror.New(apperror.CodeFailedPrecondition,
			"system credential storage is unavailable")
	}
	switch request.Action {
	case ProviderCredentialSet:
		if len([]byte(request.Secret)) < 8 || !credential.ValidSecret(request.Secret) {
			return ProviderCredentialStatus{}, apperror.New(apperror.CodeInvalidArgument,
				"Provider credential must be a normalized bounded secret")
		}
		if err := s.store.Put(ctx, request.Provider, request.Secret); err != nil {
			return ProviderCredentialStatus{}, apperror.Wrap(apperror.CodeUnavailable,
				"system credential storage rejected the Provider credential", err)
		}
	case ProviderCredentialDelete:
		if request.Secret != "" {
			return ProviderCredentialStatus{}, apperror.New(apperror.CodeInvalidArgument,
				"Provider credential deletion cannot contain a secret")
		}
		if err := s.store.Delete(ctx, request.Provider); err != nil {
			return ProviderCredentialStatus{}, apperror.Wrap(apperror.CodeUnavailable,
				"system credential storage could not delete the Provider credential", err)
		}
	default:
		return ProviderCredentialStatus{}, apperror.New(apperror.CodeInvalidArgument,
			"Provider credential action is invalid")
	}
	configured, err := s.store.Configured(ctx, request.Provider)
	if err != nil {
		return ProviderCredentialStatus{}, apperror.Wrap(apperror.CodeUnavailable,
			"system credential status could not be verified", err)
	}
	expected := request.Action == ProviderCredentialSet
	if configured != expected {
		return ProviderCredentialStatus{}, apperror.New(apperror.CodeInternal,
			"system credential change failed final status verification")
	}
	reloaded := false
	if s.registry != nil || s.routeSettings != nil {
		if s.registry == nil || s.routeSettings == nil {
			return ProviderCredentialStatus{}, apperror.New(apperror.CodeFailedPrecondition,
				"Provider Registry reload dependencies are incomplete")
		}
		result, reloadErr := s.registry.Reload(ctx, s.routeSettings)
		if reloadErr != nil {
			return ProviderCredentialStatus{}, apperror.Wrap(apperror.CodeUnavailable,
				"Provider credential changed but Registry reload was not applied", reloadErr)
		}
		if !result.Reloaded || result.ProtocolVersion != modelregistry.ReloadProtocolVersion ||
			result.Generation == 0 {
			return ProviderCredentialStatus{}, apperror.New(apperror.CodeInternal,
				"Provider Registry reload returned an invalid generation")
		}
		generation := s.registry.Generation()
		if generation == 0 || generation < result.Generation {
			return ProviderCredentialStatus{}, apperror.New(apperror.CodeInternal,
				"Provider Registry reload did not retain its generation")
		}
		reloaded = true
	}
	return s.status(request.Provider, configured, !reloaded, reloaded,
		s.registryGeneration()), nil
}

func (s *ProviderCredentialService) configured(ctx context.Context,
	provider string,
) (bool, error) {
	if !s.store.Available() {
		return false, nil
	}
	configured, err := s.store.Configured(ctx, provider)
	if err != nil {
		return false, apperror.Wrap(apperror.CodeUnavailable,
			"system credential status is unavailable", err)
	}
	return configured, nil
}

func (s *ProviderCredentialService) status(provider string, configured bool,
	restartRequired bool, registryReloaded bool, generation uint64,
) ProviderCredentialStatus {
	return ProviderCredentialStatus{ProtocolVersion: credential.ProtocolVersion,
		Provider: provider, Configured: configured, StoreKind: s.store.Kind(),
		StoreAvailable: s.store.Available(), PlaintextReturned: false,
		RestartRequired: restartRequired, RegistryReloaded: registryReloaded,
		RegistryGeneration: generation}
}

func (s *ProviderCredentialService) registryGeneration() uint64 {
	if s == nil || s.registry == nil {
		return 0
	}
	return s.registry.Generation()
}

func managedProvider(value string) bool {
	_, found := managedCredentialProviders[value]
	return found && credential.ValidName(value)
}
