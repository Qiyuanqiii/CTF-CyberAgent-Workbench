package modelregistry

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/redact"
)

const ProtocolVersion = "model_availability.v1"

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
	Providers       []ProviderAvailability
	Routes          []RouteAvailability
}

type RouteSettingReader interface {
	GetProviderSetting(ctx context.Context, key string) (string, bool, error)
}

type EnvironmentLookup func(string) (string, bool)

type Registry struct {
	mu        sync.RWMutex
	router    *llm.Router
	providers []ProviderAvailability
	available map[string]struct{}
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

func New(lookup EnvironmentLookup) *Registry {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	router := llm.NewDefaultRouter()
	registry := &Registry{
		router: router,
		providers: []ProviderAvailability{{
			Name: "mock", Kind: ProviderKindLocal, Status: ProviderAvailable,
			Models:           []string{"mock-code", "mock-cyber-agent", "mock-fast"},
			CredentialSource: "none", NetworkRequired: false,
		}},
		available: map[string]struct{}{"mock": {}},
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
		registry.registerAnthropicEnvironment(config, lookup)
	}
	sort.Slice(registry.providers, func(i, j int) bool {
		return registry.providers[i].Name < registry.providers[j].Name
	})
	return registry
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

func (r *Registry) Snapshot() Snapshot {
	if r == nil || r.router == nil {
		return Snapshot{ProtocolVersion: ProtocolVersion, Providers: []ProviderAvailability{},
			Routes: []RouteAvailability{}}
	}
	r.mu.RLock()
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
	r.mu.RUnlock()
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
	return Snapshot{ProtocolVersion: ProtocolVersion, Providers: providers, Routes: outRoutes}
}

func (r *Registry) registerAnthropicEnvironment(config anthropicEnvironment,
	lookup EnvironmentLookup,
) {
	key, present := lookup(config.apiKeyEnv)
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
				Models: models, CredentialSource: "environment", NetworkRequired: true,
				ConfigurationError: configurationError,
			})
			return
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
		Models: models, CredentialSource: "environment", NetworkRequired: true,
		ConfigurationError: configurationError,
	})
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
