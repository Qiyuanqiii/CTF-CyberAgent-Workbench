package modelregistry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/redact"
)

const ProtocolVersion = "model_availability.v1"

const ReloadProtocolVersion = "provider_registry_reload.v1"

const (
	DiagnosticProtocolVersion   = "provider_diagnostic.v1"
	RouteControlProtocolVersion = "model_route_control.v1"
	DiagnosticTimeout           = 15 * time.Second
)

const (
	ProviderAvailable            = "available"
	ProviderNotConfigured        = "not_configured"
	ProviderInvalidConfiguration = "invalid_configuration"
)

const (
	ProviderKindLocal               = "local"
	ProviderKindAnthropicCompatible = "anthropic_compatible"
)

const (
	DiagnosticReachable   = "reachable"
	DiagnosticUnreachable = "unreachable"
)

const (
	defaultMimoBaseURL     = "https://token-plan-cn.xiaomimimo.com/anthropic"
	DefaultMimoModel       = "mimo-v2.5-pro"
	defaultDeepSeekBaseURL = "https://api.deepseek.com/anthropic"
	DefaultDeepSeekModel   = "deepseek-v4-flash"
	defaultAnthropicURL    = "https://api.anthropic.com"
	DefaultAnthropicModel  = "claude-3-5-sonnet-latest"
)

var routeNames = []string{"code", "ctf", "learn", "review", "script"}

const (
	maxPublicProviderNameBytes = 128
	maxPublicModelNameBytes    = 256
)

type ProviderAvailability struct {
	Name               string
	Kind               string
	Status             string
	Models             []string
	CredentialSource   string
	NetworkRequired    bool
	ConfigurationError bool
}

type RouteAvailability struct {
	Name      string
	Provider  string
	Model     string
	Available bool
}

type Snapshot struct {
	ProtocolVersion string
	Generation      uint64
	Providers       []ProviderAvailability
	Routes          []RouteAvailability
}

type ReloadResult struct {
	ProtocolVersion string
	Generation      uint64
	Reloaded        bool
}

// DiagnosticResult is intentionally content-free. A diagnostic may make one
// minimal model request, but neither model text nor a raw Provider error crosses
// this boundary.
type DiagnosticResult struct {
	ProtocolVersion         string
	Provider                string
	Model                   string
	Status                  string
	Outcome                 string
	Retryable               bool
	NetworkRequestAttempted bool
	ModelCalled             bool
	ToolCalled              bool
	ResponseContentReturned bool
	DurationMillis          int64
}

type RouteSettingReader interface {
	GetProviderSetting(ctx context.Context, key string) (string, bool, error)
}

type RouteSettingWriter interface {
	SetProviderSetting(ctx context.Context, key string, value string) error
}

type EnvironmentLookup func(string) (string, bool)

type CredentialReader interface {
	Get(context.Context, string) (string, bool, error)
}

type Registry struct {
	mu          sync.RWMutex
	routeMu     sync.Mutex
	router      *llm.Router
	providers   []ProviderAvailability
	available   map[string]struct{}
	lookup      EnvironmentLookup
	credentials credentialLookup
	generation  uint64
}

type anthropicEnvironment struct {
	name           string
	apiKeyEnv      string
	baseURLEnv     string
	modelEnv       string
	defaultBaseURL string
	defaultModel   string
}

func NewFromEnvironment() *Registry {
	return New(os.LookupEnv)
}

// NewFromEnvironmentWithCredentials preserves environment-variable priority
// while allowing the Go control plane to bootstrap secrets from an OS-owned
// credential store. No credential is copied into Registry snapshots.
func NewFromEnvironmentWithCredentials(reader CredentialReader) (*Registry, error) {
	return newRegistry(os.LookupEnv, func(ctx context.Context, provider string) (string, bool, error) {
		if reader == nil {
			return "", false, nil
		}
		return reader.Get(ctx, provider)
	})
}

func New(lookup EnvironmentLookup) *Registry {
	registry, _ := newRegistry(lookup, nil)
	return registry
}

type credentialLookup func(context.Context, string) (string, bool, error)

func newRegistry(lookup EnvironmentLookup,
	credentials credentialLookup,
) (*Registry, error) {
	return buildRegistry(context.Background(), lookup, credentials, false)
}

func buildRegistry(ctx context.Context, lookup EnvironmentLookup,
	credentials credentialLookup, strictCredentialReads bool,
) (*Registry, error) {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	if ctx == nil {
		return nil, errors.New("model registry context is required")
	}
	router := llm.NewDefaultRouter()
	registry := &Registry{
		router: router,
		providers: []ProviderAvailability{{
			Name: "mock", Kind: ProviderKindLocal, Status: ProviderAvailable,
			Models:           []string{"mock-code", "mock-cyber-agent", "mock-fast"},
			CredentialSource: "none", NetworkRequired: false,
		}},
		available: map[string]struct{}{"mock": {}}, lookup: lookup,
		credentials: credentials, generation: 1,
	}
	configs := []anthropicEnvironment{
		{name: "mimo", apiKeyEnv: "MIMO_API_KEY", baseURLEnv: "MIMO_BASE_URL",
			modelEnv: "MIMO_MODEL", defaultBaseURL: defaultMimoBaseURL,
			defaultModel: DefaultMimoModel},
		{name: "deepseek", apiKeyEnv: "DEEPSEEK_API_KEY", baseURLEnv: "DEEPSEEK_BASE_URL",
			modelEnv: "DEEPSEEK_MODEL", defaultBaseURL: defaultDeepSeekBaseURL,
			defaultModel: DefaultDeepSeekModel},
		{name: "anthropic", apiKeyEnv: "CYBERAGENT_ANTHROPIC_API_KEY",
			baseURLEnv: "CYBERAGENT_ANTHROPIC_BASE_URL", modelEnv: "CYBERAGENT_ANTHROPIC_MODEL",
			defaultBaseURL: defaultAnthropicURL, defaultModel: DefaultAnthropicModel},
	}
	for _, config := range configs {
		if err := registry.registerAnthropicEnvironment(ctx, config, lookup,
			credentials, strictCredentialReads); err != nil {
			return nil, err
		}
	}
	sort.Slice(registry.providers, func(i, j int) bool {
		return registry.providers[i].Name < registry.providers[j].Name
	})
	return registry, nil
}

// Reload builds a complete candidate generation before taking the public
// snapshot lock. The final Router replacement is atomic; Provider values
// already captured by active calls remain valid and are never cancelled.
func (r *Registry) Reload(ctx context.Context, reader RouteSettingReader) (ReloadResult, error) {
	if r == nil || r.router == nil {
		return ReloadResult{}, errors.New("model registry is unavailable")
	}
	if ctx == nil || reader == nil {
		return ReloadResult{}, errors.New("model registry reload dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return ReloadResult{}, err
	}
	r.routeMu.Lock()
	defer r.routeMu.Unlock()
	candidate, err := buildRegistry(ctx, r.lookup, r.credentials, true)
	if err != nil {
		return ReloadResult{}, err
	}
	if err := candidate.LoadRouteSettings(ctx, reader); err != nil {
		return ReloadResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReloadResult{}, err
	}

	providers := make([]ProviderAvailability, len(candidate.providers))
	copy(providers, candidate.providers)
	for index := range providers {
		providers[index].Models = append([]string(nil), candidate.providers[index].Models...)
	}
	available := make(map[string]struct{}, len(candidate.available))
	for name := range candidate.available {
		available[name] = struct{}{}
	}
	r.mu.Lock()
	if err := r.router.ReplaceConfiguration(candidate.router); err != nil {
		r.mu.Unlock()
		return ReloadResult{}, err
	}
	r.providers = providers
	r.available = available
	r.generation++
	generation := r.generation
	r.mu.Unlock()
	return ReloadResult{ProtocolVersion: ReloadProtocolVersion,
		Generation: generation, Reloaded: true}, nil
}

func (r *Registry) Generation() uint64 {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.generation
}

func (r *Registry) Router() *llm.Router {
	if r == nil {
		return nil
	}
	return r.router
}

func (r *Registry) LoadRouteSettings(ctx context.Context, reader RouteSettingReader) error {
	if r == nil || r.router == nil {
		return errors.New("model registry is unavailable")
	}
	if reader == nil {
		return errors.New("model route setting reader is required")
	}
	for _, route := range routeNames {
		value, found, err := reader.GetProviderSetting(ctx, "route."+route)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		ref, err := llm.ParseModelRef(value)
		if err != nil || strings.TrimSpace(ref.Provider) != ref.Provider ||
			strings.TrimSpace(ref.Model) != ref.Model ||
			!validAvailabilityIdentifier(ref.Provider, maxPublicProviderNameBytes) ||
			!validAvailabilityIdentifier(ref.Model, maxPublicModelNameBytes) {
			continue
		}
		r.router.SetRoute(route, ref)
	}
	return nil
}

func (r *Registry) SelectRoute(ctx context.Context, writer RouteSettingWriter,
	route string, provider string, model string,
) (RouteAvailability, error) {
	if r == nil || r.router == nil {
		return RouteAvailability{}, errors.New("model registry is unavailable")
	}
	if writer == nil {
		return RouteAvailability{}, errors.New("model route setting writer is required")
	}
	r.routeMu.Lock()
	defer r.routeMu.Unlock()
	route = strings.TrimSpace(route)
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if !containsRoute(route) {
		return RouteAvailability{}, errors.New("model route is not supported")
	}
	if !validAvailabilityIdentifier(provider, maxPublicProviderNameBytes) ||
		!validAvailabilityIdentifier(model, maxPublicModelNameBytes) ||
		!r.providerModelAvailable(provider, model) {
		return RouteAvailability{}, errors.New("selected Provider model is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return RouteAvailability{}, err
	}
	value := provider + "/" + model
	if err := writer.SetProviderSetting(ctx, "route."+route, value); err != nil {
		return RouteAvailability{}, fmt.Errorf("persist model route: %w", err)
	}
	// SetRoute cannot fail after the exact Provider/model validation above. If
	// the process exits between persistence and this update, startup reloads the
	// durable setting before serving requests.
	r.router.SetRoute(route, llm.ModelRef{Provider: provider, Model: model})
	return RouteAvailability{Name: route, Provider: provider, Model: model, Available: true}, nil
}

func (r *Registry) Diagnose(ctx context.Context, provider string,
	model string,
) (DiagnosticResult, error) {
	if r == nil || r.router == nil {
		return DiagnosticResult{}, errors.New("model registry is unavailable")
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if !validAvailabilityIdentifier(provider, maxPublicProviderNameBytes) ||
		!validAvailabilityIdentifier(model, maxPublicModelNameBytes) ||
		!r.providerModelAvailable(provider, model) {
		return DiagnosticResult{}, errors.New("diagnostic Provider model is unavailable")
	}
	networkRequired := r.providerNetworkRequired(provider)
	diagnosticCtx, cancel := context.WithTimeout(ctx, DiagnosticTimeout)
	defer cancel()
	started := time.Now()
	result := DiagnosticResult{
		ProtocolVersion: DiagnosticProtocolVersion,
		Provider:        provider, Model: model, Status: DiagnosticUnreachable,
		NetworkRequestAttempted: networkRequired, ModelCalled: true,
		ToolCalled: false, ResponseContentReturned: false,
	}
	response, err := r.router.ChatModelRef(diagnosticCtx,
		llm.ModelRef{Provider: provider, Model: model}, llm.ChatRequest{
			Model: model,
			Messages: []llm.Message{{Role: "user",
				Content: "Reply with one short acknowledgement for a connectivity diagnostic."}},
			Temperature: 0, MaxTokens: 8,
			Metadata: map[string]string{"purpose": "connectivity_diagnostic"},
		})
	result.DurationMillis = time.Since(started).Milliseconds()
	if result.DurationMillis < 0 {
		result.DurationMillis = 0
	}
	if err != nil {
		outcome := llm.ProviderErrorKind(llm.NormalizeProviderError(provider, err))
		if !outcome.Valid() || outcome == llm.OutcomeSuccess {
			outcome = llm.OutcomePermanent
		}
		result.Outcome = string(outcome)
		result.Retryable = outcome.Retryable()
		return result, nil
	}
	if response == nil || strings.TrimSpace(response.Provider) != provider {
		result.Outcome = string(llm.OutcomeInvalidResponse)
		return result, nil
	}
	result.Status = DiagnosticReachable
	result.Outcome = string(llm.OutcomeSuccess)
	return result, nil
}

func (r *Registry) Snapshot() Snapshot {
	if r == nil || r.router == nil {
		return Snapshot{ProtocolVersion: ProtocolVersion, Providers: []ProviderAvailability{},
			Routes: []RouteAvailability{}}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	providers := make([]ProviderAvailability, len(r.providers))
	for index, provider := range r.providers {
		providers[index] = provider
		providers[index].Models = make([]string, len(provider.Models))
		for modelIndex, model := range provider.Models {
			providers[index].Models[modelIndex] = availabilityIdentifier(model,
				maxPublicModelNameBytes)
		}
	}
	available := make(map[string]struct{}, len(r.available))
	for name := range r.available {
		available[name] = struct{}{}
	}
	routes := r.router.Routes()
	outRoutes := make([]RouteAvailability, 0, len(routeNames))
	for _, name := range routeNames {
		ref := routes[name]
		_, registered := available[ref.Provider]
		provider := availabilityIdentifier(ref.Provider, maxPublicProviderNameBytes)
		model := availabilityIdentifier(ref.Model, maxPublicModelNameBytes)
		outRoutes = append(outRoutes, RouteAvailability{
			Name: name, Provider: provider, Model: model,
			Available: registered && provider == ref.Provider && model == ref.Model,
		})
	}
	return Snapshot{ProtocolVersion: ProtocolVersion, Generation: r.generation,
		Providers: providers, Routes: outRoutes}
}

func (r *Registry) providerModelAvailable(provider string, model string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.available[provider]; !ok {
		return false
	}
	for _, current := range r.providers {
		if current.Name != provider || current.Status != ProviderAvailable {
			continue
		}
		for _, candidate := range current.Models {
			if candidate == model {
				return true
			}
		}
	}
	return false
}

func (r *Registry) providerNetworkRequired(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, current := range r.providers {
		if current.Name == provider {
			return current.NetworkRequired
		}
	}
	return false
}

func containsRoute(route string) bool {
	for _, current := range routeNames {
		if current == route {
			return true
		}
	}
	return false
}

func (r *Registry) registerAnthropicEnvironment(ctx context.Context, config anthropicEnvironment,
	lookup EnvironmentLookup, credentials credentialLookup, strictCredentialReads bool,
) error {
	key, present := lookup(config.apiKeyEnv)
	credentialSource := "none"
	if present {
		credentialSource = "environment"
	}
	if !present && credentials != nil {
		var err error
		key, present, err = credentials(ctx, config.name)
		if err != nil {
			if strictCredentialReads {
				return fmt.Errorf("read system credential for %s: %w", config.name, err)
			}
			r.providers = append(r.providers, ProviderAvailability{
				Name: config.name, Kind: ProviderKindAnthropicCompatible,
				Status: ProviderInvalidConfiguration, Models: []string{},
				CredentialSource: "system", NetworkRequired: true,
				ConfigurationError: true,
			})
			return nil
		}
		if present {
			credentialSource = "system"
		}
	}
	status := ProviderNotConfigured
	models := []string{}
	configurationError := false
	if present && key != "" {
		baseURL := environmentValue(lookup, config.baseURLEnv, config.defaultBaseURL)
		model := environmentValue(lookup, config.modelEnv, config.defaultModel)
		if !validAvailabilityIdentifier(model, maxPublicModelNameBytes) {
			status = ProviderInvalidConfiguration
			configurationError = true
			r.providers = append(r.providers, ProviderAvailability{
				Name: config.name, Kind: ProviderKindAnthropicCompatible, Status: status,
				Models: models, CredentialSource: credentialSource, NetworkRequired: true,
				ConfigurationError: configurationError,
			})
			return nil
		}
		provider, err := llm.NewAnthropicCompatibleProvider(llm.AnthropicCompatibleConfig{
			Name: config.name, BaseURL: baseURL, APIKey: key, DefaultModel: model,
		})
		if err != nil {
			status = ProviderInvalidConfiguration
			configurationError = true
		} else {
			status = ProviderAvailable
			models = []string{strings.TrimSpace(model)}
			r.router.RegisterProvider(provider)
			r.available[config.name] = struct{}{}
		}
	} else if present && key != strings.TrimSpace(key) {
		status = ProviderInvalidConfiguration
		configurationError = true
	}
	r.providers = append(r.providers, ProviderAvailability{
		Name: config.name, Kind: ProviderKindAnthropicCompatible, Status: status,
		Models: models, CredentialSource: credentialSource, NetworkRequired: true,
		ConfigurationError: configurationError,
	})
	return nil
}

func environmentValue(lookup EnvironmentLookup, name string, fallback string) string {
	value, found := lookup(name)
	if !found || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func validAvailabilityIdentifier(value string, maximumBytes int) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len([]byte(value)) > maximumBytes || redact.String(value) != value {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return false
		}
	}
	return true
}

func availabilityIdentifier(value string, maximumBytes int) string {
	if validAvailabilityIdentifier(value, maximumBytes) {
		return value
	}
	if redact.String(value) != value {
		return "redacted"
	}
	return "invalid"
}
