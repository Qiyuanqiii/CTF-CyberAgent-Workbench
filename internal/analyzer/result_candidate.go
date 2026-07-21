package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
)

const (
	ValidatedResultCandidateProtocolVersion  = "analyzer_validated_result_candidate.v1"
	AnalyzerArtifactCandidateProtocolVersion = "analyzer_artifact_candidate.v1"
	MaxValidatedResultCandidateEnvelopeBytes = 8 * 1024

	AnalyzerResultArtifactKind      = "analyzer_result_metadata"
	AnalyzerResultArtifactMediaType = "application/json"
	AnalyzerResultArtifactEncoding  = "utf-8"
)

// ResultCandidateAuthority is deliberately empty. A validated result is still
// only evidence for a later review gate, never permission to persist or publish.
type ResultCandidateAuthority struct {
	ProductInvocation bool `json:"product_invocation"`
	Persistence       bool `json:"persistence"`
	ArtifactCommit    bool `json:"artifact_commit"`
	RunEventWrite     bool `json:"run_event_write"`
	ExternalPublish   bool `json:"external_publish"`
}

// AnalyzerArtifactCandidate describes exact transient result bytes without
// retaining them or binding the candidate to a product Artifact store.
type AnalyzerArtifactCandidate struct {
	ProtocolVersion    string `json:"protocol_version"`
	Kind               string `json:"kind"`
	MediaType          string `json:"media_type"`
	Encoding           string `json:"encoding"`
	SourceProtocol     string `json:"source_protocol"`
	SizeBytes          int    `json:"size_bytes"`
	SHA256             string `json:"sha256"`
	CandidateOnly      bool   `json:"candidate_only"`
	MetadataOnly       bool   `json:"metadata_only"`
	ContentIncluded    bool   `json:"content_included"`
	PathIncluded       bool   `json:"path_included"`
	RunBound           bool   `json:"run_bound"`
	SessionBound       bool   `json:"session_bound"`
	WorkspaceBound     bool   `json:"workspace_bound"`
	PersistenceEnabled bool   `json:"persistence_enabled"`
	CommitAuthorized   bool   `json:"commit_authorized"`
}

// ValidatedResultCandidate seals the complete inert analyzer source chain and
// exact result digest. It contains no result body and grants no product action.
type ValidatedResultCandidate struct {
	ProtocolVersion          string                    `json:"protocol_version"`
	CandidateSHA256          string                    `json:"candidate_sha256"`
	ExecutableIdentitySHA256 string                    `json:"executable_identity_sha256"`
	PreflightSHA256          string                    `json:"preflight_sha256"`
	OutcomeSHA256            string                    `json:"outcome_sha256"`
	RequestID                string                    `json:"request_id"`
	Analyzer                 string                    `json:"analyzer"`
	OutcomeStatus            InvocationStatus          `json:"outcome_status"`
	ResultProtocol           string                    `json:"result_protocol"`
	ResultBytes              int                       `json:"result_bytes"`
	ResultSHA256             string                    `json:"result_sha256"`
	Artifact                 AnalyzerArtifactCandidate `json:"artifact"`
	Authority                ResultCandidateAuthority  `json:"authority"`
	SourceChainBound         bool                      `json:"source_chain_bound"`
	ResultEnvelopeValidated  bool                      `json:"result_envelope_validated"`
	DeterministicMatch       bool                      `json:"deterministic_match"`
	TestConformanceOnly      bool                      `json:"test_conformance_only"`
	MetadataOnly             bool                      `json:"metadata_only"`
	RawResultIncluded        bool                      `json:"raw_result_included"`
	ExecutableBytesIncluded  bool                      `json:"executable_bytes_included"`
	PathIncluded             bool                      `json:"path_included"`
	PersistenceEnabled       bool                      `json:"persistence_enabled"`
	ArtifactCommitAuthorized bool                      `json:"artifact_commit_authorized"`
	ProductInvocationEnabled bool                      `json:"product_invocation_enabled"`
}

func BuildValidatedResultCandidate(invocation InvocationCandidate, rawRequest,
	executable []byte, identity ExecutableIdentity, preflight InvocationPreflight,
	outcome InvocationOutcome, rawResult []byte,
) (ValidatedResultCandidate, ErrorCode) {
	if code := ValidateInvocationPreflight(preflight, invocation, rawRequest, executable,
		identity); code != "" {
		return ValidatedResultCandidate{}, CodeInvalidResult
	}
	if !ValidateInvocationOutcome(invocation, outcome) ||
		outcome.Status != InvocationSucceeded || outcome.ExitCode != ExitSuccess ||
		!outcome.ResultValidated || outcome.ResultProtocol != invocation.Descriptor.ResultProtocol ||
		len(rawResult) == 0 || len(rawResult) > MaxResultEnvelopeBytes ||
		len(rawResult) != outcome.StdoutBytes || !validSuccessfulAnalyzerResult(invocation,
		rawRequest, rawResult) {
		return ValidatedResultCandidate{}, CodeInvalidResult
	}
	resultDigest := sha256.Sum256(rawResult)
	resultSHA256 := hex.EncodeToString(resultDigest[:])
	if resultSHA256 != outcome.StdoutSHA256 {
		return ValidatedResultCandidate{}, CodeInvalidResult
	}
	candidateDigest, ok := invocationCandidateSHA256(invocation)
	if !ok {
		return ValidatedResultCandidate{}, CodeInternal
	}
	identityDigest, ok := canonicalSHA256(identity)
	if !ok {
		return ValidatedResultCandidate{}, CodeInternal
	}
	preflightDigest, ok := canonicalSHA256(preflight)
	if !ok {
		return ValidatedResultCandidate{}, CodeInternal
	}
	outcomeDigest, ok := canonicalSHA256(outcome)
	if !ok {
		return ValidatedResultCandidate{}, CodeInternal
	}
	artifact := AnalyzerArtifactCandidate{
		ProtocolVersion: AnalyzerArtifactCandidateProtocolVersion,
		Kind:            AnalyzerResultArtifactKind, MediaType: AnalyzerResultArtifactMediaType,
		Encoding: AnalyzerResultArtifactEncoding, SourceProtocol: outcome.ResultProtocol,
		SizeBytes: len(rawResult), SHA256: resultSHA256, CandidateOnly: true,
		MetadataOnly: true,
	}
	value := ValidatedResultCandidate{
		ProtocolVersion: ValidatedResultCandidateProtocolVersion,
		CandidateSHA256: candidateDigest, ExecutableIdentitySHA256: identityDigest,
		PreflightSHA256: preflightDigest, OutcomeSHA256: outcomeDigest,
		RequestID: invocation.RequestID, Analyzer: invocation.Analyzer,
		OutcomeStatus: outcome.Status, ResultProtocol: outcome.ResultProtocol,
		ResultBytes: len(rawResult), ResultSHA256: resultSHA256, Artifact: artifact,
		SourceChainBound: true, ResultEnvelopeValidated: true, DeterministicMatch: true,
		TestConformanceOnly: true, MetadataOnly: true,
	}
	if !validateValidatedResultCandidateStructure(value, invocation, identity, preflight,
		outcome) {
		return ValidatedResultCandidate{}, CodeInternal
	}
	return value, ""
}

func ValidateValidatedResultCandidate(value ValidatedResultCandidate,
	invocation InvocationCandidate, rawRequest, executable []byte, identity ExecutableIdentity,
	preflight InvocationPreflight, outcome InvocationOutcome, rawResult []byte,
) ErrorCode {
	expected, code := BuildValidatedResultCandidate(invocation, rawRequest, executable, identity,
		preflight, outcome, rawResult)
	if code != "" {
		return code
	}
	if !reflect.DeepEqual(value, expected) {
		return CodeInvalidResult
	}
	return ""
}

func EncodeValidatedResultCandidate(value ValidatedResultCandidate,
	invocation InvocationCandidate, rawRequest, executable []byte, identity ExecutableIdentity,
	preflight InvocationPreflight, outcome InvocationOutcome, rawResult []byte,
) ([]byte, ErrorCode) {
	if code := ValidateValidatedResultCandidate(value, invocation, rawRequest, executable,
		identity, preflight, outcome, rawResult); code != "" {
		return nil, code
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 || len(encoded) > MaxValidatedResultCandidateEnvelopeBytes {
		return nil, CodeInternal
	}
	return encoded, ""
}

func DecodeValidatedResultCandidate(rawCandidate []byte, invocation InvocationCandidate,
	rawRequest, executable []byte, identity ExecutableIdentity, preflight InvocationPreflight,
	outcome InvocationOutcome, rawResult []byte,
) (ValidatedResultCandidate, ErrorCode) {
	var wire validatedResultCandidateWire
	if !strictDecode(rawCandidate, MaxValidatedResultCandidateEnvelopeBytes, &wire) ||
		!wire.complete() {
		return ValidatedResultCandidate{}, CodeInvalidResult
	}
	value := wire.value()
	if code := ValidateValidatedResultCandidate(value, invocation, rawRequest, executable,
		identity, preflight, outcome, rawResult); code != "" {
		return ValidatedResultCandidate{}, CodeInvalidResult
	}
	return value, ""
}

func validateValidatedResultCandidateStructure(value ValidatedResultCandidate,
	invocation InvocationCandidate, identity ExecutableIdentity, preflight InvocationPreflight,
	outcome InvocationOutcome,
) bool {
	identityDigest, identityOK := canonicalSHA256(identity)
	preflightDigest, preflightOK := canonicalSHA256(preflight)
	outcomeDigest, outcomeOK := canonicalSHA256(outcome)
	return identityOK && preflightOK && outcomeOK &&
		value.ProtocolVersion == ValidatedResultCandidateProtocolVersion &&
		validDigest(value.CandidateSHA256) && value.CandidateSHA256 == preflight.CandidateSHA256 &&
		value.ExecutableIdentitySHA256 == identityDigest &&
		value.ExecutableIdentitySHA256 == preflight.ExecutableIdentitySHA256 &&
		value.PreflightSHA256 == preflightDigest && value.OutcomeSHA256 == outcomeDigest &&
		value.RequestID == invocation.RequestID && value.Analyzer == invocation.Analyzer &&
		value.OutcomeStatus == InvocationSucceeded && value.OutcomeStatus == outcome.Status &&
		value.ResultProtocol == invocation.Descriptor.ResultProtocol &&
		value.ResultProtocol == outcome.ResultProtocol && value.ResultBytes == outcome.StdoutBytes &&
		value.ResultBytes > 0 && value.ResultBytes <= MaxResultEnvelopeBytes &&
		value.ResultSHA256 == outcome.StdoutSHA256 && validDigest(value.ResultSHA256) &&
		validateAnalyzerArtifactCandidate(value.Artifact, value) &&
		value.Authority == (ResultCandidateAuthority{}) && value.SourceChainBound &&
		value.ResultEnvelopeValidated && value.DeterministicMatch && value.TestConformanceOnly &&
		value.MetadataOnly && !value.RawResultIncluded && !value.ExecutableBytesIncluded &&
		!value.PathIncluded && !value.PersistenceEnabled && !value.ArtifactCommitAuthorized &&
		!value.ProductInvocationEnabled
}

func validateAnalyzerArtifactCandidate(artifact AnalyzerArtifactCandidate,
	value ValidatedResultCandidate,
) bool {
	return artifact.ProtocolVersion == AnalyzerArtifactCandidateProtocolVersion &&
		artifact.Kind == AnalyzerResultArtifactKind &&
		artifact.MediaType == AnalyzerResultArtifactMediaType &&
		artifact.Encoding == AnalyzerResultArtifactEncoding &&
		artifact.SourceProtocol == value.ResultProtocol && artifact.SizeBytes == value.ResultBytes &&
		artifact.SHA256 == value.ResultSHA256 && artifact.CandidateOnly && artifact.MetadataOnly &&
		!artifact.ContentIncluded && !artifact.PathIncluded && !artifact.RunBound &&
		!artifact.SessionBound && !artifact.WorkspaceBound && !artifact.PersistenceEnabled &&
		!artifact.CommitAuthorized
}

type resultCandidateAuthorityWire struct {
	ProductInvocation *bool `json:"product_invocation"`
	Persistence       *bool `json:"persistence"`
	ArtifactCommit    *bool `json:"artifact_commit"`
	RunEventWrite     *bool `json:"run_event_write"`
	ExternalPublish   *bool `json:"external_publish"`
}

func (wire resultCandidateAuthorityWire) complete() bool {
	return wire.ProductInvocation != nil && wire.Persistence != nil &&
		wire.ArtifactCommit != nil && wire.RunEventWrite != nil && wire.ExternalPublish != nil
}

func (wire resultCandidateAuthorityWire) value() ResultCandidateAuthority {
	return ResultCandidateAuthority{
		ProductInvocation: *wire.ProductInvocation, Persistence: *wire.Persistence,
		ArtifactCommit: *wire.ArtifactCommit, RunEventWrite: *wire.RunEventWrite,
		ExternalPublish: *wire.ExternalPublish,
	}
}

type analyzerArtifactCandidateWire struct {
	ProtocolVersion    *string `json:"protocol_version"`
	Kind               *string `json:"kind"`
	MediaType          *string `json:"media_type"`
	Encoding           *string `json:"encoding"`
	SourceProtocol     *string `json:"source_protocol"`
	SizeBytes          *int    `json:"size_bytes"`
	SHA256             *string `json:"sha256"`
	CandidateOnly      *bool   `json:"candidate_only"`
	MetadataOnly       *bool   `json:"metadata_only"`
	ContentIncluded    *bool   `json:"content_included"`
	PathIncluded       *bool   `json:"path_included"`
	RunBound           *bool   `json:"run_bound"`
	SessionBound       *bool   `json:"session_bound"`
	WorkspaceBound     *bool   `json:"workspace_bound"`
	PersistenceEnabled *bool   `json:"persistence_enabled"`
	CommitAuthorized   *bool   `json:"commit_authorized"`
}

func (wire analyzerArtifactCandidateWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.Kind != nil && wire.MediaType != nil &&
		wire.Encoding != nil && wire.SourceProtocol != nil && wire.SizeBytes != nil &&
		wire.SHA256 != nil && wire.CandidateOnly != nil && wire.MetadataOnly != nil &&
		wire.ContentIncluded != nil && wire.PathIncluded != nil && wire.RunBound != nil &&
		wire.SessionBound != nil && wire.WorkspaceBound != nil &&
		wire.PersistenceEnabled != nil && wire.CommitAuthorized != nil
}

func (wire analyzerArtifactCandidateWire) value() AnalyzerArtifactCandidate {
	return AnalyzerArtifactCandidate{
		ProtocolVersion: *wire.ProtocolVersion, Kind: *wire.Kind, MediaType: *wire.MediaType,
		Encoding: *wire.Encoding, SourceProtocol: *wire.SourceProtocol,
		SizeBytes: *wire.SizeBytes, SHA256: *wire.SHA256, CandidateOnly: *wire.CandidateOnly,
		MetadataOnly: *wire.MetadataOnly, ContentIncluded: *wire.ContentIncluded,
		PathIncluded: *wire.PathIncluded, RunBound: *wire.RunBound,
		SessionBound: *wire.SessionBound, WorkspaceBound: *wire.WorkspaceBound,
		PersistenceEnabled: *wire.PersistenceEnabled, CommitAuthorized: *wire.CommitAuthorized,
	}
}

type validatedResultCandidateWire struct {
	ProtocolVersion          *string                        `json:"protocol_version"`
	CandidateSHA256          *string                        `json:"candidate_sha256"`
	ExecutableIdentitySHA256 *string                        `json:"executable_identity_sha256"`
	PreflightSHA256          *string                        `json:"preflight_sha256"`
	OutcomeSHA256            *string                        `json:"outcome_sha256"`
	RequestID                *string                        `json:"request_id"`
	Analyzer                 *string                        `json:"analyzer"`
	OutcomeStatus            *InvocationStatus              `json:"outcome_status"`
	ResultProtocol           *string                        `json:"result_protocol"`
	ResultBytes              *int                           `json:"result_bytes"`
	ResultSHA256             *string                        `json:"result_sha256"`
	Artifact                 *analyzerArtifactCandidateWire `json:"artifact"`
	Authority                *resultCandidateAuthorityWire  `json:"authority"`
	SourceChainBound         *bool                          `json:"source_chain_bound"`
	ResultEnvelopeValidated  *bool                          `json:"result_envelope_validated"`
	DeterministicMatch       *bool                          `json:"deterministic_match"`
	TestConformanceOnly      *bool                          `json:"test_conformance_only"`
	MetadataOnly             *bool                          `json:"metadata_only"`
	RawResultIncluded        *bool                          `json:"raw_result_included"`
	ExecutableBytesIncluded  *bool                          `json:"executable_bytes_included"`
	PathIncluded             *bool                          `json:"path_included"`
	PersistenceEnabled       *bool                          `json:"persistence_enabled"`
	ArtifactCommitAuthorized *bool                          `json:"artifact_commit_authorized"`
	ProductInvocationEnabled *bool                          `json:"product_invocation_enabled"`
}

func (wire validatedResultCandidateWire) complete() bool {
	return wire.ProtocolVersion != nil && wire.CandidateSHA256 != nil &&
		wire.ExecutableIdentitySHA256 != nil && wire.PreflightSHA256 != nil &&
		wire.OutcomeSHA256 != nil && wire.RequestID != nil && wire.Analyzer != nil &&
		wire.OutcomeStatus != nil && wire.ResultProtocol != nil && wire.ResultBytes != nil &&
		wire.ResultSHA256 != nil && wire.Artifact != nil && wire.Artifact.complete() &&
		wire.Authority != nil && wire.Authority.complete() && wire.SourceChainBound != nil &&
		wire.ResultEnvelopeValidated != nil && wire.DeterministicMatch != nil &&
		wire.TestConformanceOnly != nil && wire.MetadataOnly != nil &&
		wire.RawResultIncluded != nil && wire.ExecutableBytesIncluded != nil &&
		wire.PathIncluded != nil && wire.PersistenceEnabled != nil &&
		wire.ArtifactCommitAuthorized != nil && wire.ProductInvocationEnabled != nil
}

func (wire validatedResultCandidateWire) value() ValidatedResultCandidate {
	return ValidatedResultCandidate{
		ProtocolVersion: *wire.ProtocolVersion, CandidateSHA256: *wire.CandidateSHA256,
		ExecutableIdentitySHA256: *wire.ExecutableIdentitySHA256,
		PreflightSHA256:          *wire.PreflightSHA256, OutcomeSHA256: *wire.OutcomeSHA256,
		RequestID: *wire.RequestID, Analyzer: *wire.Analyzer,
		OutcomeStatus: *wire.OutcomeStatus, ResultProtocol: *wire.ResultProtocol,
		ResultBytes: *wire.ResultBytes, ResultSHA256: *wire.ResultSHA256,
		Artifact: wire.Artifact.value(), Authority: wire.Authority.value(),
		SourceChainBound:        *wire.SourceChainBound,
		ResultEnvelopeValidated: *wire.ResultEnvelopeValidated,
		DeterministicMatch:      *wire.DeterministicMatch,
		TestConformanceOnly:     *wire.TestConformanceOnly, MetadataOnly: *wire.MetadataOnly,
		RawResultIncluded:       *wire.RawResultIncluded,
		ExecutableBytesIncluded: *wire.ExecutableBytesIncluded,
		PathIncluded:            *wire.PathIncluded, PersistenceEnabled: *wire.PersistenceEnabled,
		ArtifactCommitAuthorized: *wire.ArtifactCommitAuthorized,
		ProductInvocationEnabled: *wire.ProductInvocationEnabled,
	}
}
