package analyzer

import (
	"encoding/json"
	"reflect"
)

const (
	ProductAdapterThreatModelProtocolVersion  = "analyzer_product_adapter_threat_model.v1"
	ProductAdapterControlStatusRequired       = "required_unimplemented"
	MaxProductAdapterThreatModelEnvelopeBytes = 24 * 1024
)

// ProductAdapterControl is one mandatory, independently verified release gate.
type ProductAdapterControl struct {
	ID                 string `json:"id"`
	Category           string `json:"category"`
	Requirement        string `json:"requirement"`
	Status             string `json:"status"`
	Required           bool   `json:"required"`
	Implemented        bool   `json:"implemented"`
	Verified           bool   `json:"verified"`
	BlocksProductStart bool   `json:"blocks_product_start"`
}

// ProductAdapterThreatModel is a default-deny readiness record. Every v1
// control is deliberately open, so this record cannot authorize a process.
type ProductAdapterThreatModel struct {
	ProtocolVersion             string                  `json:"protocol_version"`
	AdapterKind                 string                  `json:"adapter_kind"`
	ResultCandidateProtocol     string                  `json:"result_candidate_protocol"`
	ArtifactCandidateProtocol   string                  `json:"artifact_candidate_protocol"`
	Controls                    []ProductAdapterControl `json:"controls"`
	RequiredControlCount        int                     `json:"required_control_count"`
	ImplementedControlCount     int                     `json:"implemented_control_count"`
	VerifiedControlCount        int                     `json:"verified_control_count"`
	OpenControlCount            int                     `json:"open_control_count"`
	AllControlsRequired         bool                    `json:"all_controls_required"`
	DefaultDeny                 bool                    `json:"default_deny"`
	ReviewRequired              bool                    `json:"review_required"`
	TestConformanceEvidenceOnly bool                    `json:"test_conformance_evidence_only"`
	MetadataOnly                bool                    `json:"metadata_only"`
	ProductAdapterPresent       bool                    `json:"product_adapter_present"`
	ProcessStarterPresent       bool                    `json:"process_starter_present"`
	OperatorOverrideAllowed     bool                    `json:"operator_override_allowed"`
	ProductStartAuthorized      bool                    `json:"product_start_authorized"`
	PersistenceAuthorized       bool                    `json:"persistence_authorized"`
	ArtifactCommitAuthorized    bool                    `json:"artifact_commit_authorized"`
	NetworkAuthorized           bool                    `json:"network_authorized"`
	HostFilesystemAuthorized    bool                    `json:"host_filesystem_authorized"`
	SecretAccessAuthorized      bool                    `json:"secret_access_authorized"`
}

func BuildProductAdapterThreatModel() ProductAdapterThreatModel {
	controls := []ProductAdapterControl{
		productAdapterControl("executable_handle_identity", "executable",
			"execute the same immutable handle that passed identity verification"),
		productAdapterControl("executable_format", "executable",
			"parse and allow only the expected PE or ELF executable format"),
		productAdapterControl("target_architecture", "executable",
			"prove executable architecture matches the reviewed target"),
		productAdapterControl("provenance_signature", "supply_chain",
			"verify trusted provenance, digest, and platform signing policy"),
		productAdapterControl("version_allowlist", "supply_chain",
			"pin an immutable reviewed analyzer version and digest allowlist"),
		productAdapterControl("least_privilege_identity", "isolation",
			"launch under a dedicated non-administrator operating-system identity"),
		productAdapterControl("filesystem_sandbox", "isolation",
			"deny host filesystem access outside explicit read-only inputs and staged output"),
		productAdapterControl("network_isolation", "isolation",
			"enforce no network namespace or socket access for the analyzer"),
		productAdapterControl("environment_scrubbing", "isolation",
			"supply an explicit minimal environment without inherited secrets"),
		productAdapterControl("cpu_limit", "resource",
			"enforce an operating-system CPU quota independent of model deadlines"),
		productAdapterControl("memory_limit", "resource",
			"enforce a hard resident-memory ceiling with deterministic failure"),
		productAdapterControl("process_count_limit", "resource",
			"bound descendants and prevent process-tree expansion"),
		productAdapterControl("wall_clock_deadline", "lifecycle",
			"enforce a monotonic hard deadline with reserved cleanup time"),
		productAdapterControl("process_tree_termination", "lifecycle",
			"terminate and prove reaping of the complete process tree"),
		productAdapterControl("bounded_stdio_redaction", "data",
			"bound stdout and stderr before allocation and redact retained evidence"),
		productAdapterControl("operator_scope_approval", "authority",
			"bind explicit operator approval to analyzer, input scope, digest, and limits"),
		productAdapterControl("atomic_result_handoff", "integrity",
			"publish a validated result with no-replace atomicity and exact digest binding"),
		productAdapterControl("durable_intent_recovery", "recovery",
			"commit write-ahead intent and generation fencing before product launch"),
		productAdapterControl("append_only_audit", "audit",
			"record bounded lifecycle, policy, approval, resource, and result events"),
		productAdapterControl("orphan_rollback_reconciliation", "recovery",
			"detect and reconcile crash residue without deleting foreign resources"),
	}
	return ProductAdapterThreatModel{
		ProtocolVersion:           ProductAdapterThreatModelProtocolVersion,
		AdapterKind:               "local_analyzer_subprocess",
		ResultCandidateProtocol:   ValidatedResultCandidateProtocolVersion,
		ArtifactCandidateProtocol: AnalyzerArtifactCandidateProtocolVersion,
		Controls:                  controls, RequiredControlCount: len(controls), OpenControlCount: len(controls),
		AllControlsRequired: true, DefaultDeny: true, ReviewRequired: true,
		TestConformanceEvidenceOnly: true, MetadataOnly: true,
	}
}

func ValidateProductAdapterThreatModel(value ProductAdapterThreatModel) ErrorCode {
	expected := BuildProductAdapterThreatModel()
	if !reflect.DeepEqual(value, expected) {
		return CodeInvalidResult
	}
	return ""
}

func EncodeProductAdapterThreatModel(value ProductAdapterThreatModel) ([]byte, ErrorCode) {
	if code := ValidateProductAdapterThreatModel(value); code != "" {
		return nil, code
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 || len(encoded) > MaxProductAdapterThreatModelEnvelopeBytes {
		return nil, CodeInternal
	}
	return encoded, ""
}

func DecodeProductAdapterThreatModel(raw []byte) (ProductAdapterThreatModel, ErrorCode) {
	var wire productAdapterThreatModelWire
	if !strictDecode(raw, MaxProductAdapterThreatModelEnvelopeBytes, &wire) || !wire.complete() {
		return ProductAdapterThreatModel{}, CodeInvalidResult
	}
	value := wire.value()
	if code := ValidateProductAdapterThreatModel(value); code != "" {
		return ProductAdapterThreatModel{}, CodeInvalidResult
	}
	return value, ""
}

func productAdapterControl(id, category, requirement string) ProductAdapterControl {
	return ProductAdapterControl{
		ID: id, Category: category, Requirement: requirement,
		Status: ProductAdapterControlStatusRequired, Required: true,
		BlocksProductStart: true,
	}
}

type productAdapterControlWire struct {
	ID                 *string `json:"id"`
	Category           *string `json:"category"`
	Requirement        *string `json:"requirement"`
	Status             *string `json:"status"`
	Required           *bool   `json:"required"`
	Implemented        *bool   `json:"implemented"`
	Verified           *bool   `json:"verified"`
	BlocksProductStart *bool   `json:"blocks_product_start"`
}

func (wire productAdapterControlWire) complete() bool {
	return wire.ID != nil && wire.Category != nil && wire.Requirement != nil &&
		wire.Status != nil && wire.Required != nil && wire.Implemented != nil &&
		wire.Verified != nil && wire.BlocksProductStart != nil
}

func (wire productAdapterControlWire) value() ProductAdapterControl {
	return ProductAdapterControl{
		ID: *wire.ID, Category: *wire.Category, Requirement: *wire.Requirement,
		Status: *wire.Status, Required: *wire.Required, Implemented: *wire.Implemented,
		Verified: *wire.Verified, BlocksProductStart: *wire.BlocksProductStart,
	}
}

type productAdapterThreatModelWire struct {
	ProtocolVersion             *string                      `json:"protocol_version"`
	AdapterKind                 *string                      `json:"adapter_kind"`
	ResultCandidateProtocol     *string                      `json:"result_candidate_protocol"`
	ArtifactCandidateProtocol   *string                      `json:"artifact_candidate_protocol"`
	Controls                    *[]productAdapterControlWire `json:"controls"`
	RequiredControlCount        *int                         `json:"required_control_count"`
	ImplementedControlCount     *int                         `json:"implemented_control_count"`
	VerifiedControlCount        *int                         `json:"verified_control_count"`
	OpenControlCount            *int                         `json:"open_control_count"`
	AllControlsRequired         *bool                        `json:"all_controls_required"`
	DefaultDeny                 *bool                        `json:"default_deny"`
	ReviewRequired              *bool                        `json:"review_required"`
	TestConformanceEvidenceOnly *bool                        `json:"test_conformance_evidence_only"`
	MetadataOnly                *bool                        `json:"metadata_only"`
	ProductAdapterPresent       *bool                        `json:"product_adapter_present"`
	ProcessStarterPresent       *bool                        `json:"process_starter_present"`
	OperatorOverrideAllowed     *bool                        `json:"operator_override_allowed"`
	ProductStartAuthorized      *bool                        `json:"product_start_authorized"`
	PersistenceAuthorized       *bool                        `json:"persistence_authorized"`
	ArtifactCommitAuthorized    *bool                        `json:"artifact_commit_authorized"`
	NetworkAuthorized           *bool                        `json:"network_authorized"`
	HostFilesystemAuthorized    *bool                        `json:"host_filesystem_authorized"`
	SecretAccessAuthorized      *bool                        `json:"secret_access_authorized"`
}

func (wire productAdapterThreatModelWire) complete() bool {
	if wire.ProtocolVersion == nil || wire.AdapterKind == nil ||
		wire.ResultCandidateProtocol == nil || wire.ArtifactCandidateProtocol == nil ||
		wire.Controls == nil || wire.RequiredControlCount == nil ||
		wire.ImplementedControlCount == nil || wire.VerifiedControlCount == nil ||
		wire.OpenControlCount == nil || wire.AllControlsRequired == nil ||
		wire.DefaultDeny == nil || wire.ReviewRequired == nil ||
		wire.TestConformanceEvidenceOnly == nil || wire.MetadataOnly == nil ||
		wire.ProductAdapterPresent == nil || wire.ProcessStarterPresent == nil ||
		wire.OperatorOverrideAllowed == nil || wire.ProductStartAuthorized == nil ||
		wire.PersistenceAuthorized == nil || wire.ArtifactCommitAuthorized == nil ||
		wire.NetworkAuthorized == nil || wire.HostFilesystemAuthorized == nil ||
		wire.SecretAccessAuthorized == nil {
		return false
	}
	for _, control := range *wire.Controls {
		if !control.complete() {
			return false
		}
	}
	return true
}

func (wire productAdapterThreatModelWire) value() ProductAdapterThreatModel {
	controls := make([]ProductAdapterControl, len(*wire.Controls))
	for index, control := range *wire.Controls {
		controls[index] = control.value()
	}
	return ProductAdapterThreatModel{
		ProtocolVersion: *wire.ProtocolVersion, AdapterKind: *wire.AdapterKind,
		ResultCandidateProtocol:   *wire.ResultCandidateProtocol,
		ArtifactCandidateProtocol: *wire.ArtifactCandidateProtocol, Controls: controls,
		RequiredControlCount:    *wire.RequiredControlCount,
		ImplementedControlCount: *wire.ImplementedControlCount,
		VerifiedControlCount:    *wire.VerifiedControlCount,
		OpenControlCount:        *wire.OpenControlCount, AllControlsRequired: *wire.AllControlsRequired,
		DefaultDeny: *wire.DefaultDeny, ReviewRequired: *wire.ReviewRequired,
		TestConformanceEvidenceOnly: *wire.TestConformanceEvidenceOnly,
		MetadataOnly:                *wire.MetadataOnly, ProductAdapterPresent: *wire.ProductAdapterPresent,
		ProcessStarterPresent:    *wire.ProcessStarterPresent,
		OperatorOverrideAllowed:  *wire.OperatorOverrideAllowed,
		ProductStartAuthorized:   *wire.ProductStartAuthorized,
		PersistenceAuthorized:    *wire.PersistenceAuthorized,
		ArtifactCommitAuthorized: *wire.ArtifactCommitAuthorized,
		NetworkAuthorized:        *wire.NetworkAuthorized,
		HostFilesystemAuthorized: *wire.HostFilesystemAuthorized,
		SecretAccessAuthorized:   *wire.SecretAccessAuthorized,
	}
}
