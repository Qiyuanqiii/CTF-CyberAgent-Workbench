package application

import (
	"context"
	"sort"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/credential"
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
	ProtocolVersion   string
	Provider          string
	Configured        bool
	StoreKind         string
	StoreAvailable    bool
	PlaintextReturned bool
	RestartRequired   bool
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
	store credential.Store
}

func NewProviderCredentialService(store credential.Store) *ProviderCredentialService {
	return &ProviderCredentialService{store: store}
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
	values := make([]ProviderCredentialStatus, 0, len(names))
	for _, name := range names {
		configured, err := s.configured(ctx, name)
		if err != nil {
			return nil, err
		}
		values = append(values, s.status(name, configured, false))
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
	return s.status(request.Provider, configured, true), nil
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
	restartRequired bool,
) ProviderCredentialStatus {
	return ProviderCredentialStatus{ProtocolVersion: credential.ProtocolVersion,
		Provider: provider, Configured: configured, StoreKind: s.store.Kind(),
		StoreAvailable: s.store.Available(), PlaintextReturned: false,
		RestartRequired: restartRequired}
}

func managedProvider(value string) bool {
	_, found := managedCredentialProviders[value]
	return found && credential.ValidName(value)
}
