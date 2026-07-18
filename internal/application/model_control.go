package application

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/modelregistry"
)

type ModelControlRegistry interface {
	Snapshot() modelregistry.Snapshot
	SelectRoute(context.Context, modelregistry.RouteSettingWriter, string, string, string) (
		modelregistry.RouteAvailability, error)
	Diagnose(context.Context, string, string) (modelregistry.DiagnosticResult, error)
}

type ModelControlService struct {
	registry ModelControlRegistry
	settings modelregistry.RouteSettingWriter
}

type SelectModelRouteRequest struct {
	Version  string
	Route    string
	Provider string
	Model    string
}

type DiagnoseProviderRequest struct {
	Version           string
	Provider          string
	Model             string
	ConfirmDiagnostic bool
}

func NewModelControlService(registry ModelControlRegistry,
	settings modelregistry.RouteSettingWriter,
) *ModelControlService {
	return &ModelControlService{registry: registry, settings: settings}
}

func (s *ModelControlService) SelectRoute(ctx context.Context,
	request SelectModelRouteRequest,
) (modelregistry.RouteAvailability, error) {
	if s == nil || s.registry == nil || s.settings == nil {
		return modelregistry.RouteAvailability{}, apperror.New(
			apperror.CodeFailedPrecondition, "model control dependencies are required")
	}
	if request.Version != modelregistry.RouteControlProtocolVersion ||
		!validModelControlValue(request.Route, 128) ||
		!validModelControlValue(request.Provider, 128) ||
		!validModelControlValue(request.Model, 256) {
		return modelregistry.RouteAvailability{}, apperror.New(
			apperror.CodeInvalidArgument, "model route control request is invalid")
	}
	snapshot := s.registry.Snapshot()
	if !snapshotContainsRoute(snapshot, request.Route) ||
		!snapshotContainsProviderModel(snapshot, request.Provider, request.Model) {
		return modelregistry.RouteAvailability{}, apperror.New(
			apperror.CodeFailedPrecondition, "selected Provider model is unavailable")
	}
	selected, err := s.registry.SelectRoute(ctx, s.settings, request.Route,
		request.Provider, request.Model)
	if err != nil {
		return modelregistry.RouteAvailability{}, apperror.Wrap(
			apperror.CodeUnavailable, "model route selection could not be persisted", err)
	}
	return selected, nil
}

func (s *ModelControlService) Diagnose(ctx context.Context,
	request DiagnoseProviderRequest,
) (modelregistry.DiagnosticResult, error) {
	if s == nil || s.registry == nil {
		return modelregistry.DiagnosticResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "model control dependencies are required")
	}
	if request.Version != modelregistry.DiagnosticProtocolVersion ||
		!request.ConfirmDiagnostic || !validModelControlValue(request.Provider, 128) ||
		!validModelControlValue(request.Model, 256) {
		return modelregistry.DiagnosticResult{}, apperror.New(
			apperror.CodeInvalidArgument, "Provider diagnostic request is invalid")
	}
	if !snapshotContainsProviderModel(s.registry.Snapshot(), request.Provider, request.Model) {
		return modelregistry.DiagnosticResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "diagnostic Provider model is unavailable")
	}
	result, err := s.registry.Diagnose(ctx, request.Provider, request.Model)
	if err != nil {
		return modelregistry.DiagnosticResult{}, apperror.Normalize(err)
	}
	return result, nil
}

func validModelControlValue(value string, maximumBytes int) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len([]byte(value)) > maximumBytes {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return false
		}
	}
	return true
}

func snapshotContainsRoute(snapshot modelregistry.Snapshot, route string) bool {
	for _, current := range snapshot.Routes {
		if current.Name == route {
			return true
		}
	}
	return false
}

func snapshotContainsProviderModel(snapshot modelregistry.Snapshot,
	provider string, model string,
) bool {
	for _, current := range snapshot.Providers {
		if current.Name != provider || current.Status != modelregistry.ProviderAvailable {
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
