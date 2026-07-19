package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/modelregistry"
	"cyberagent-workbench/internal/store"
)

func TestModelAvailabilityIsRedactedAndDoesNotProbeProviders(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "model-availability.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SetProviderSetting(context.Background(), "route.code", "mimo/mimo-http-test"); err != nil {
		t.Fatal(err)
	}
	secret := "provider-test-key-" + strings.Repeat("x", 40)
	values := map[string]string{"MIMO_API_KEY": secret, "MIMO_MODEL": "mimo-http-test"}
	models := modelregistry.New(func(name string) (string, bool) {
		value, found := values[name]
		return value, found
	})
	if err := models.LoadRouteSettings(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	api, err := New(st, Config{AccessToken: testAccessToken, ModelRegistry: models,
		AppVersion: "model-test"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	request.Host = "127.0.0.1"
	request.RemoteAddr = "127.0.0.1:45000"
	request.Header.Set("Authorization", "Bearer "+testAccessToken)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("model availability status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) ||
		strings.Contains(response.Body.String(), "MIMO_API_KEY") ||
		strings.Contains(response.Body.String(), "token-plan-cn") ||
		strings.Contains(response.Body.String(), "base_url") ||
		strings.Contains(response.Body.String(), `"models":null`) {
		t.Fatalf("model availability exposed private configuration: %s", response.Body.String())
	}
	var envelope struct {
		Data ModelAvailabilityView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ProtocolVersion != modelregistry.ProtocolVersion ||
		len(envelope.Data.Providers) != 4 || len(envelope.Data.Routes) != 5 {
		t.Fatalf("unexpected model availability: %#v", envelope.Data)
	}
	var foundMimo, foundCode bool
	for _, provider := range envelope.Data.Providers {
		if provider.Name == "mimo" {
			foundMimo = provider.Status == modelregistry.ProviderAvailable &&
				len(provider.Models) == 1 && provider.Models[0] == "mimo-http-test"
		}
	}
	for _, route := range envelope.Data.Routes {
		if route.Name == "code" {
			foundCode = route.Available && route.Provider == "mimo" &&
				route.Model == "mimo-http-test"
		}
	}
	if !foundMimo || !foundCode {
		t.Fatalf("configured provider or route missing: %#v", envelope.Data)
	}
}
