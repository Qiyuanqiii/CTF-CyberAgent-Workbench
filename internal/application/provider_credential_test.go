package application

import (
	"encoding/json"
	"strings"
	"testing"

	"cyberagent-workbench/internal/credential"
)

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
