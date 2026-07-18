package application

import (
	"context"
	"testing"

	"cyberagent-workbench/internal/modelregistry"
)

type modelRouteSettings map[string]string

func (s modelRouteSettings) SetProviderSetting(_ context.Context, key string, value string) error {
	s[key] = value
	return nil
}

func TestModelControlSelectsPersistedAvailableRoute(t *testing.T) {
	registry := modelregistry.New(nil)
	settings := modelRouteSettings{}
	service := NewModelControlService(registry, settings)
	selected, err := service.SelectRoute(context.Background(), SelectModelRouteRequest{
		Version: modelregistry.RouteControlProtocolVersion,
		Route:   "review", Provider: "mock", Model: "mock-fast",
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings["route.review"] != "mock/mock-fast" || !selected.Available ||
		registry.Router().Resolve("review").Model != "mock-fast" {
		t.Fatalf("unexpected model route selection: %#v %#v", settings, selected)
	}
	if _, err := service.SelectRoute(context.Background(), SelectModelRouteRequest{
		Version: modelregistry.RouteControlProtocolVersion,
		Route:   "review", Provider: "mimo", Model: modelregistry.DefaultMimoModel,
	}); err == nil {
		t.Fatal("unconfigured Provider route was accepted")
	}
}

func TestModelControlRequiresExplicitDiagnosticConfirmation(t *testing.T) {
	service := NewModelControlService(modelregistry.New(nil), modelRouteSettings{})
	if _, err := service.Diagnose(context.Background(), DiagnoseProviderRequest{
		Version:  modelregistry.DiagnosticProtocolVersion,
		Provider: "mock", Model: "mock-fast",
	}); err == nil {
		t.Fatal("diagnostic without explicit confirmation was accepted")
	}
	result, err := service.Diagnose(context.Background(), DiagnoseProviderRequest{
		Version:  modelregistry.DiagnosticProtocolVersion,
		Provider: "mock", Model: "mock-fast", ConfirmDiagnostic: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != modelregistry.DiagnosticReachable ||
		result.ResponseContentReturned || result.ToolCalled {
		t.Fatalf("unexpected diagnostic result: %#v", result)
	}
}
