package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

var expectedProductAdapterControlIDs = []string{
	"executable_handle_identity",
	"executable_format",
	"target_architecture",
	"provenance_signature",
	"version_allowlist",
	"least_privilege_identity",
	"filesystem_sandbox",
	"network_isolation",
	"environment_scrubbing",
	"cpu_limit",
	"memory_limit",
	"process_count_limit",
	"wall_clock_deadline",
	"process_tree_termination",
	"bounded_stdio_redaction",
	"operator_scope_approval",
	"atomic_result_handoff",
	"durable_intent_recovery",
	"append_only_audit",
	"orphan_rollback_reconciliation",
}

func TestProductAdapterThreatModelBlocksStartUntilEveryControlIsVerified(t *testing.T) {
	model := BuildProductAdapterThreatModel()
	if model.ProtocolVersion != ProductAdapterThreatModelProtocolVersion ||
		model.AdapterKind != "local_analyzer_subprocess" ||
		model.ResultCandidateProtocol != ValidatedResultCandidateProtocolVersion ||
		model.ArtifactCandidateProtocol != AnalyzerArtifactCandidateProtocolVersion ||
		model.RequiredControlCount != len(expectedProductAdapterControlIDs) ||
		model.OpenControlCount != len(expectedProductAdapterControlIDs) ||
		model.ImplementedControlCount != 0 || model.VerifiedControlCount != 0 ||
		!model.AllControlsRequired || !model.DefaultDeny || !model.ReviewRequired ||
		!model.TestConformanceEvidenceOnly || !model.MetadataOnly ||
		model.ProductAdapterPresent || model.ProcessStarterPresent ||
		model.OperatorOverrideAllowed || model.ProductStartAuthorized ||
		model.PersistenceAuthorized || model.ArtifactCommitAuthorized ||
		model.NetworkAuthorized || model.HostFilesystemAuthorized ||
		model.SecretAccessAuthorized {
		t.Fatalf("unsafe or incomplete product adapter threat model: %#v", model)
	}
	if len(model.Controls) != len(expectedProductAdapterControlIDs) {
		t.Fatalf("control count=%d", len(model.Controls))
	}
	for index, control := range model.Controls {
		if control.ID != expectedProductAdapterControlIDs[index] || control.Category == "" ||
			control.Requirement == "" || control.Status != ProductAdapterControlStatusRequired ||
			!control.Required || control.Implemented || control.Verified ||
			!control.BlocksProductStart {
			t.Fatalf("unsafe control %d: %#v", index, control)
		}
	}
	encoded, code := EncodeProductAdapterThreatModel(model)
	if code != "" {
		t.Fatal(code)
	}
	decoded, code := DecodeProductAdapterThreatModel(encoded)
	if code != "" || !reflect.DeepEqual(decoded, model) {
		t.Fatalf("threat model round trip code=%s value=%#v", code, decoded)
	}
	assertExactObjectKeys(t, encoded, []string{"adapter_kind", "all_controls_required",
		"artifact_candidate_protocol", "artifact_commit_authorized", "controls",
		"default_deny", "host_filesystem_authorized", "implemented_control_count",
		"metadata_only", "network_authorized", "open_control_count",
		"operator_override_allowed", "persistence_authorized", "process_starter_present",
		"product_adapter_present", "product_start_authorized", "protocol_version",
		"required_control_count", "result_candidate_protocol", "review_required",
		"secret_access_authorized", "test_conformance_evidence_only",
		"verified_control_count"})
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	var controls []json.RawMessage
	if err := json.Unmarshal(fields["controls"], &controls); err != nil || len(controls) == 0 {
		t.Fatalf("decode controls err=%v count=%d", err, len(controls))
	}
	assertExactObjectKeys(t, controls[0], []string{"blocks_product_start", "category",
		"id", "implemented", "required", "requirement", "status", "verified"})
}

func TestProductAdapterThreatModelRejectsControlAndSchemaDrift(t *testing.T) {
	model := BuildProductAdapterThreatModel()
	mutations := map[string]func(*ProductAdapterThreatModel){
		"implemented": func(value *ProductAdapterThreatModel) {
			value.Controls[0].Implemented = true
		},
		"verified": func(value *ProductAdapterThreatModel) { value.Controls[1].Verified = true },
		"remove": func(value *ProductAdapterThreatModel) {
			value.Controls = value.Controls[:len(value.Controls)-1]
		},
		"reorder": func(value *ProductAdapterThreatModel) {
			value.Controls[0], value.Controls[1] = value.Controls[1], value.Controls[0]
		},
		"override": func(value *ProductAdapterThreatModel) { value.OperatorOverrideAllowed = true },
		"start":    func(value *ProductAdapterThreatModel) { value.ProductStartAuthorized = true },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			value := model
			value.Controls = append([]ProductAdapterControl(nil), model.Controls...)
			mutate(&value)
			if code := ValidateProductAdapterThreatModel(value); code != CodeInvalidResult {
				t.Fatalf("drift code=%s", code)
			}
		})
	}
	encoded, code := EncodeProductAdapterThreatModel(model)
	if code != "" {
		t.Fatal(code)
	}
	text := string(encoded)
	for name, malformed := range map[string]string{
		"future": strings.Replace(text, ProductAdapterThreatModelProtocolVersion,
			"analyzer_product_adapter_threat_model.v2", 1),
		"unknown": strings.Replace(text, `"operator_override_allowed":false`,
			`"operator_override_allowed":false,"force":true`, 1),
		"duplicate": strings.Replace(text, `"product_start_authorized":false`,
			`"product_start_authorized":false,"product_start_authorized":false`, 1),
		"missing false": strings.Replace(text, `,"artifact_commit_authorized":false`, "", 1),
		"control widening": strings.Replace(text, `"implemented":false`,
			`"implemented":false,"command":"run"`, 1),
	} {
		t.Run("schema "+name, func(t *testing.T) {
			if _, got := DecodeProductAdapterThreatModel([]byte(malformed)); got != CodeInvalidResult {
				t.Fatalf("schema drift code=%s", got)
			}
		})
	}
}

func TestProductAdapterThreatModelADRTracksEveryRequiredControl(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source path is unavailable")
	}
	path := filepath.Join(filepath.Dir(source), "..", "..", "docs", "adr",
		"0067-inert-analyzer-result-staging-and-product-adapter-threat-model.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, protocol := range []string{ValidatedResultCandidateProtocolVersion,
		AnalyzerArtifactCandidateProtocolVersion, ProductAdapterThreatModelProtocolVersion} {
		if !strings.Contains(text, protocol) {
			t.Fatalf("ADR is missing protocol %s", protocol)
		}
	}
	for _, id := range expectedProductAdapterControlIDs {
		if !strings.Contains(text, "`"+id+"`") {
			t.Fatalf("ADR is missing product adapter control %s", id)
		}
	}
}

func FuzzProductAdapterThreatModelEnvelope(f *testing.F) {
	model := BuildProductAdapterThreatModel()
	seed, code := EncodeProductAdapterThreatModel(model)
	if code != "" {
		f.Fatal(code)
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, input []byte) {
		decoded, decodeCode := DecodeProductAdapterThreatModel(input)
		if decodeCode != "" {
			return
		}
		encoded, encodeCode := EncodeProductAdapterThreatModel(decoded)
		if encodeCode != "" {
			t.Fatalf("accepted threat model failed re-encode: %s", encodeCode)
		}
		replayed, replayCode := DecodeProductAdapterThreatModel(encoded)
		if replayCode != "" || !reflect.DeepEqual(decoded, replayed) {
			t.Fatalf("accepted threat model was not idempotent: code=%s", replayCode)
		}
	})
}
