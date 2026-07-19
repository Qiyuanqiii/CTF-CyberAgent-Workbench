package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cyberagent-workbench/internal/credential"
	"cyberagent-workbench/internal/modelregistry"
)

type providerCredentialReloadFake struct {
	generation         uint64
	advanceAfterReload bool
	err                error
}

func (f *providerCredentialReloadFake) Reload(context.Context,
	modelregistry.RouteSettingReader,
) (modelregistry.ReloadResult, error) {
	if f.err != nil {
		return modelregistry.ReloadResult{}, f.err
	}
	f.generation++
	reloadedGeneration := f.generation
	if f.advanceAfterReload {
		f.generation++
	}
	return modelregistry.ReloadResult{ProtocolVersion: modelregistry.ReloadProtocolVersion,
		Generation: reloadedGeneration, Reloaded: true}, nil
}
func (f *providerCredentialReloadFake) Generation() uint64 { return f.generation }

type providerCredentialRouteSettings struct{}

func (providerCredentialRouteSettings) GetProviderSetting(context.Context,
	string,
) (string, bool, error) {
	return "", false, nil
}

func TestProviderCredentialServiceReturnsStatusOnly(t *testing.T) {
	store := credential.NewMemoryStore()
	service := NewProviderCredentialService(store)
	secret := "temporary-provider-key"
	status, err := service.Change(t.Context(), ChangeProviderCredentialRequest{
		Version: credential.ProtocolVersion, Provider: "mimo",
		Action: ProviderCredentialSet, Secret: secret, Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.PlaintextReturned || !status.RestartRequired ||
		strings.Contains(string(raw), secret) {
		t.Fatalf("credential status exposed secret or wrong state: %s", raw)
	}
	stored, found, err := store.Get(t.Context(), "mimo")
	if err != nil || !found || stored != secret {
		t.Fatal("credential was not handed to the owned store")
	}
	deleted, err := service.Change(t.Context(), ChangeProviderCredentialRequest{
		Version: credential.ProtocolVersion, Provider: "mimo",
		Action: ProviderCredentialDelete, Confirm: true,
	})
	if err != nil || deleted.Configured || deleted.PlaintextReturned {
		t.Fatalf("credential deletion failed closed: %#v err=%v", deleted, err)
	}
}

func TestProviderCredentialServiceReloadsRegistryWithoutRestart(t *testing.T) {
	store := credential.NewMemoryStore()
	reloader := &providerCredentialReloadFake{generation: 1}
	service := NewProviderCredentialService(store).WithRegistryReload(reloader,
		providerCredentialRouteSettings{})
	status, err := service.Change(t.Context(), ChangeProviderCredentialRequest{
		Version: credential.ProtocolVersion, Provider: "deepseek",
		Action: ProviderCredentialSet, Secret: "temporary-provider-key", Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.RestartRequired || !status.RegistryReloaded ||
		status.RegistryGeneration != 2 || reloader.Generation() != 2 {
		t.Fatalf("credential Registry generation was not applied: %#v", status)
	}
	statuses, err := service.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range statuses {
		if current.RegistryReloaded || current.RestartRequired ||
			current.RegistryGeneration != 2 {
			t.Fatalf("status list widened reload authority: %#v", current)
		}
	}
}

func TestProviderCredentialServiceKeepsOldGenerationWhenReloadFails(t *testing.T) {
	store := credential.NewMemoryStore()
	reloader := &providerCredentialReloadFake{generation: 7, err: errors.New("reload failed")}
	service := NewProviderCredentialService(store).WithRegistryReload(reloader,
		providerCredentialRouteSettings{})
	_, err := service.Change(t.Context(), ChangeProviderCredentialRequest{
		Version: credential.ProtocolVersion, Provider: "mimo",
		Action: ProviderCredentialSet, Secret: "temporary-provider-key", Confirm: true,
	})
	if err == nil || reloader.Generation() != 7 {
		t.Fatalf("failed reload changed generation or returned success: generation=%d err=%v",
			reloader.Generation(), err)
	}
	if _, found, getErr := store.Get(t.Context(), "mimo"); getErr != nil || !found {
		t.Fatalf("credential change was not durably visible for a safe retry: found=%t err=%v",
			found, getErr)
	}
}

func TestProviderCredentialServiceAcceptsAConcurrentNewerRegistryGeneration(t *testing.T) {
	store := credential.NewMemoryStore()
	reloader := &providerCredentialReloadFake{generation: 1, advanceAfterReload: true}
	service := NewProviderCredentialService(store).WithRegistryReload(reloader,
		providerCredentialRouteSettings{})
	status, err := service.Change(t.Context(), ChangeProviderCredentialRequest{
		Version: credential.ProtocolVersion, Provider: "anthropic",
		Action: ProviderCredentialSet, Secret: "temporary-provider-key", Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.RegistryReloaded || status.RestartRequired ||
		status.RegistryGeneration != 3 {
		t.Fatalf("newer concurrent generation was misreported: %#v", status)
	}
}

func TestProviderCredentialServiceRejectsUnknownOrUnconfirmedChanges(t *testing.T) {
	service := NewProviderCredentialService(credential.NewMemoryStore())
	for _, request := range []ChangeProviderCredentialRequest{
		{Version: credential.ProtocolVersion, Provider: "unknown", Action: ProviderCredentialSet,
			Secret: "temporary-provider-key", Confirm: true},
		{Version: credential.ProtocolVersion, Provider: "mimo", Action: ProviderCredentialSet,
			Secret: "temporary-provider-key", Confirm: false},
		{Version: credential.ProtocolVersion, Provider: "mimo", Action: ProviderCredentialDelete,
			Secret: "must-not-be-here", Confirm: true},
		{Version: credential.ProtocolVersion, Provider: " mimo", Action: ProviderCredentialSet,
			Secret: "temporary-provider-key", Confirm: true},
	} {
		if _, err := service.Change(t.Context(), request); err == nil {
			t.Fatalf("unsafe credential request was accepted: %#v", request)
		}
	}
}
