package httpapi

import (
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/store"
)

func TestSkillPackageInstallHTTPControlRegistersOnlyInertAuthority(t *testing.T) {
	state, err := store.Open(filepath.Join(t.TempDir(), "skill-install-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	objects, err := skills.NewLocalPackageObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	api, err := New(state, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, SkillInstallationEnabled: true,
		SkillInstallationController: application.NewSkillPackageRegistryService(
			state, objects, registry)})
	if err != nil {
		t.Fatal(err)
	}
	archive := buildOpenAPISkillPackage(t)
	encoded := base64.StdEncoding.EncodeToString(archive)
	body := `{"version":"skill_package_installation.v1","archive_base64":"` +
		encoded + `","surface":"code","confirm_untrusted":true}`
	operationKey := "http-skill-install-0001"
	first := performSessionMessageRequest(t, api, http.MethodPost,
		SkillPackageInstallPath, testControlToken, operationKey, "application/json",
		strings.NewReader(body))
	if first.Code != http.StatusAccepted ||
		!strings.Contains(first.Body.String(), `"trust_class":"operator_installed_untrusted"`) ||
		!strings.Contains(first.Body.String(), `"surface":"code"`) ||
		strings.Contains(first.Body.String(), encoded) {
		t.Fatalf("Skill install status=%d body=%s", first.Code, first.Body.String())
	}
	for _, closed := range []string{
		`"import_command_execution":false`, `"import_network_access":false`,
		`"import_provider_calls":false`, `"tool_capability_grant":false`,
		`"run_selection_authorized":false`, `"context_injection_authorized":false`,
	} {
		if !strings.Contains(first.Body.String(), closed) {
			t.Fatalf("Skill install widened authority, missing %s: %s", closed,
				first.Body.String())
		}
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost,
		SkillPackageInstallPath, testControlToken, operationKey, "application/json",
		strings.NewReader(body))
	if replay.Code != http.StatusAccepted ||
		!strings.Contains(replay.Body.String(), `"replayed":true`) {
		t.Fatalf("Skill install replay status=%d body=%s", replay.Code,
			replay.Body.String())
	}
	unconfirmed := performSessionMessageRequest(t, api, http.MethodPost,
		SkillPackageInstallPath, testControlToken, "http-skill-unconfirmed-0001",
		"application/json", strings.NewReader(strings.Replace(body,
			`"confirm_untrusted":true`, `"confirm_untrusted":false`, 1)))
	assertAPIError(t, unconfirmed, http.StatusBadRequest, "INVALID_ARGUMENT")
	readToken := performSessionMessageRequest(t, api, http.MethodPost,
		SkillPackageInstallPath, testAccessToken, "http-skill-read-token-0001",
		"application/json", strings.NewReader(body))
	assertAPIError(t, readToken, http.StatusUnauthorized, "POLICY_DENIED")
}
