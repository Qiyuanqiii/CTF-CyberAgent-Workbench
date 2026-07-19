package modelregistry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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

type orderedRouteWriter struct {
	registry *Registry
	values   map[string]string
}

type blockingRouteWriter struct {
	mu            sync.Mutex
	values        []string
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
}

func (w *blockingRouteWriter) SetProviderSetting(_ context.Context, _ string,
	value string,
) error {
	w.mu.Lock()
	ordinal := len(w.values)
	w.values = append(w.values, value)
	w.mu.Unlock()
	if ordinal == 0 {
		close(w.firstEntered)
		<-w.releaseFirst
	} else if ordinal == 1 {
		close(w.secondEntered)
	}
	return nil
}

func (w *orderedRouteWriter) SetProviderSetting(_ context.Context, key string, value string) error {
	if current := w.registry.Router().Resolve("code"); current.Provider != "mock" ||
		current.Model != "mock-code" {
		return errors.New("route changed before durable setting")
	}
	w.values[key] = value
	return nil
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

func TestRegistryBootstrapsSystemCredentialWithoutProjectingIt(t *testing.T) {
	secret := "system-provider-key-0123456789"
	registry, err := newRegistry(func(string) (string, bool) { return "", false },
		func(provider string) (string, bool, error) {
			if provider == "mimo" {
				return secret, true, nil
			}
			return "", false, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := registry.Snapshot()
	var mimo ProviderAvailability
	for _, provider := range snapshot.Providers {
		if provider.Name == "mimo" {
			mimo = provider
		}
	}
	if mimo.Status != ProviderAvailable || mimo.CredentialSource != "system" ||
		!contains(registry.Router().ProviderNames(), "mimo") ||
		strings.Contains(snapshotText(snapshot), secret) {
		t.Fatalf("system credential bootstrap violated its projection boundary: %#v", mimo)
	}
}

func TestRegistryContainsSystemCredentialReadFailureToOneProvider(t *testing.T) {
	registry, err := newRegistry(func(string) (string, bool) { return "", false },
		func(provider string) (string, bool, error) {
			if provider == "mimo" {
				return "", false, errors.New("credential store unavailable")
			}
			return "", false, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := registry.Snapshot()
	for _, provider := range snapshot.Providers {
		if provider.Name == "mimo" &&
			(provider.Status != ProviderInvalidConfiguration ||
				provider.CredentialSource != "system" || !provider.ConfigurationError) {
			t.Fatalf("system credential failure escaped its Provider boundary: %#v", provider)
		}
	}
	if !contains(registry.Router().ProviderNames(), "mock") {
		t.Fatal("system credential failure disabled the local Provider")
	}
}

func TestRegistryProjectsNoCredentialSourceWhenUnconfigured(t *testing.T) {
	registry := New(func(string) (string, bool) { return "", false })
	for _, provider := range registry.Snapshot().Providers {
		if provider.Name != "mock" && provider.CredentialSource != "none" {
			t.Fatalf("unconfigured Provider projected a false credential source: %#v", provider)
		}
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

func TestRegistrySelectRoutePersistsBeforeConcurrentRouterUpdate(t *testing.T) {
	registry := New(nil)
	writer := &orderedRouteWriter{registry: registry, values: map[string]string{}}
	selected, err := registry.SelectRoute(context.Background(), writer,
		"code", "mock", "mock-fast")
	if err != nil {
		t.Fatal(err)
	}
	if writer.values["route.code"] != "mock/mock-fast" || !selected.Available ||
		selected.Name != "code" || selected.Provider != "mock" || selected.Model != "mock-fast" {
		t.Fatalf("unexpected persisted route selection: %#v %#v", writer.values, selected)
	}
	if current := registry.Router().Resolve("code"); current.Provider != "mock" ||
		current.Model != "mock-fast" {
		t.Fatalf("router was not updated: %#v", current)
	}
	if _, err := registry.SelectRoute(context.Background(), writer,
		"unknown", "mock", "mock-fast"); err == nil {
		t.Fatal("unsupported route was accepted")
	}
	if _, err := registry.SelectRoute(context.Background(), writer,
		"code", "missing", "model"); err == nil {
		t.Fatal("unavailable Provider was accepted")
	}
}

func TestRegistrySelectRouteSerializesDurableAndMemoryUpdates(t *testing.T) {
	registry := New(nil)
	writer := &blockingRouteWriter{firstEntered: make(chan struct{}),
		secondEntered: make(chan struct{}), releaseFirst: make(chan struct{})}
	errorsSeen := make(chan error, 2)
	go func() {
		_, err := registry.SelectRoute(context.Background(), writer,
			"code", "mock", "mock-fast")
		errorsSeen <- err
	}()
	<-writer.firstEntered
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		_, err := registry.SelectRoute(context.Background(), writer,
			"code", "mock", "mock-code")
		errorsSeen <- err
	}()
	<-secondStarted
	select {
	case <-writer.secondEntered:
		t.Fatal("second route persistence overtook the first in-memory update")
	case <-time.After(50 * time.Millisecond):
	}
	close(writer.releaseFirst)
	for range 2 {
		if err := <-errorsSeen; err != nil {
			t.Fatal(err)
		}
	}
	writer.mu.Lock()
	values := append([]string(nil), writer.values...)
	writer.mu.Unlock()
	if len(values) != 2 || values[0] != "mock/mock-fast" ||
		values[1] != "mock/mock-code" {
		t.Fatalf("route persistence order=%#v", values)
	}
	if current := registry.Router().Resolve("code"); current.Provider != "mock" ||
		current.Model != "mock-code" {
		t.Fatalf("durable and in-memory order diverged: %#v", current)
	}
}

func TestRegistryDiagnosticReturnsOnlyBoundedConnectivityFacts(t *testing.T) {
	registry := New(nil)
	result, err := registry.Diagnose(context.Background(), "mock", "mock-fast")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != DiagnosticProtocolVersion ||
		result.Status != DiagnosticReachable || result.Outcome != string(llm.OutcomeSuccess) ||
		result.Provider != "mock" || result.Model != "mock-fast" ||
		result.NetworkRequestAttempted || !result.ModelCalled || result.ToolCalled ||
		result.ResponseContentReturned || result.DurationMillis < 0 {
		t.Fatalf("unexpected diagnostic projection: %#v", result)
	}
	if _, err := registry.Diagnose(context.Background(), "mimo", DefaultMimoModel); err == nil {
		t.Fatal("unconfigured Provider diagnostic was accepted")
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
