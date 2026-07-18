package modelregistry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cyberagent-workbench/internal/llm"
)

type routeSettings map[string]string

func (s routeSettings) GetProviderSetting(_ context.Context, key string) (string, bool, error) {
	value, found := s[key]
	return value, found, nil
}

type failingRouteSettings struct{}

func (failingRouteSettings) GetProviderSetting(context.Context, string) (string, bool, error) {
	return "", false, errors.New("route store unavailable")
}

func TestRegistryBuildsRedactedEnvironmentAvailabilityAndRoutes(t *testing.T) {
	secret := "sk-" + strings.Repeat("z", 48)
	values := map[string]string{
		"MIMO_API_KEY": secret,
		"MIMO_MODEL":   "mimo-test-model",
	}
	registry := New(func(name string) (string, bool) {
		value, found := values[name]
		return value, found
	})
	if err := registry.LoadRouteSettings(context.Background(), routeSettings{
		"route.code": "mimo/mimo-test-model",
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := registry.Snapshot()
	if snapshot.ProtocolVersion != ProtocolVersion || len(snapshot.Providers) != 4 ||
		len(snapshot.Routes) != len(routeNames) {
		t.Fatalf("unexpected registry snapshot: %#v", snapshot)
	}
	var mimo ProviderAvailability
	for _, provider := range snapshot.Providers {
		if provider.Name == "mimo" {
			mimo = provider
		}
	}
	if mimo.Status != ProviderAvailable || mimo.CredentialSource != "environment" ||
		len(mimo.Models) != 1 || mimo.Models[0] != "mimo-test-model" {
		t.Fatalf("unexpected Mimo availability: %#v", mimo)
	}
	for _, route := range snapshot.Routes {
		if route.Name == "code" && (!route.Available || route.Provider != "mimo" ||
			route.Model != "mimo-test-model") {
			t.Fatalf("unexpected code route: %#v", route)
		}
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(snapshotText(snapshot))), secret) {
		t.Fatal("provider snapshot exposed an API key")
	}
}

func TestRegistryMarksInvalidAndUnavailableConfigurationWithoutRegisteringIt(t *testing.T) {
	values := map[string]string{
		"DEEPSEEK_API_KEY": " invalid-key ",
	}
	registry := New(func(name string) (string, bool) {
		value, found := values[name]
		return value, found
	})
	snapshot := registry.Snapshot()
	for _, provider := range snapshot.Providers {
		if provider.Name == "deepseek" &&
			(provider.Status != ProviderInvalidConfiguration || !provider.ConfigurationError) {
			t.Fatalf("invalid provider was not projected safely: %#v", provider)
		}
	}
	if contains(registry.Router().ProviderNames(), "deepseek") {
		t.Fatal("invalid provider was registered")
	}
}

func TestRegistryNeverProjectsSecretLikeModelOrRouteIdentifiers(t *testing.T) {
	secret := "sk-" + strings.Repeat("q", 48)
	registry := New(func(name string) (string, bool) {
		values := map[string]string{
			"MIMO_API_KEY": "provider-key-for-model-redaction-test",
			"MIMO_MODEL":   secret,
		}
		value, found := values[name]
		return value, found
	})
	registry.Router().SetRoute("code", llm.ModelRef{Provider: "mock", Model: secret})
	snapshot := registry.Snapshot()
	if strings.Contains(snapshotText(snapshot), secret) {
		t.Fatal("model availability projected a secret-like model identifier")
	}
	for _, provider := range snapshot.Providers {
		if provider.Name == "mimo" && (provider.Status != ProviderInvalidConfiguration ||
			!provider.ConfigurationError || len(provider.Models) != 0) {
			t.Fatalf("secret-like model configuration was not rejected: %#v", provider)
		}
	}
	for _, route := range snapshot.Routes {
		if route.Name == "code" && (route.Model != "redacted" || route.Available) {
			t.Fatalf("secret-like route was not closed: %#v", route)
		}
	}
}

func TestRegistryRouteSettingFailureIsReturned(t *testing.T) {
	registry := New(nil)
	if err := registry.LoadRouteSettings(context.Background(), failingRouteSettings{}); err == nil {
		t.Fatal("expected route setting failure")
	}
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func snapshotText(snapshot Snapshot) string {
	var builder strings.Builder
	for _, provider := range snapshot.Providers {
		builder.WriteString(provider.Name)
		builder.WriteString(provider.Status)
		builder.WriteString(strings.Join(provider.Models, ","))
	}
	for _, route := range snapshot.Routes {
		builder.WriteString(route.Name)
		builder.WriteString(route.Provider)
		builder.WriteString(route.Model)
	}
	return builder.String()
}
