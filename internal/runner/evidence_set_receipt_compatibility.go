package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

const MaxEvidenceSetReceiptEnvelopeBytes = 8 * 1024

const (
	evidenceSetReceiptRejectMalformedEnvelope   = "malformed_envelope"
	evidenceSetReceiptRejectUnsupportedProtocol = "unsupported_protocol"
	evidenceSetReceiptRejectContractMismatch    = "contract_mismatch"
)

var evidenceSetReceiptEnvelopeFields = [...]string{
	"protocol_version", "record_count", "record_protocols", "canonical_sha256",
	"canonical_bytes", "complete", "metadata_only", "timeline_logical_sequence_only",
	"cross_record_wall_clock_order_claimed", "raw_output_included",
	"process_identity_included", "os_resource_limits_verified", "product_execution_enabled",
}

type evidenceSetReceiptEnvelope struct {
	ProtocolVersion                  string   `json:"protocol_version"`
	RecordCount                      int      `json:"record_count"`
	RecordProtocols                  []string `json:"record_protocols"`
	CanonicalSHA256                  string   `json:"canonical_sha256"`
	CanonicalBytes                   int      `json:"canonical_bytes"`
	Complete                         bool     `json:"complete"`
	MetadataOnly                     bool     `json:"metadata_only"`
	TimelineLogicalSequenceOnly      bool     `json:"timeline_logical_sequence_only"`
	CrossRecordWallClockOrderClaimed bool     `json:"cross_record_wall_clock_order_claimed"`
	RawOutputIncluded                bool     `json:"raw_output_included"`
	ProcessIdentityIncluded          bool     `json:"process_identity_included"`
	OSResourceLimitsVerified         bool     `json:"os_resource_limits_verified"`
	ProductExecutionEnabled          bool     `json:"product_execution_enabled"`
}

func encodeEvidenceSetReceiptEnvelope(receipt EvidenceSetReceipt) ([]byte, error) {
	protocols := make([]string, len(receipt.RecordProtocols))
	copy(protocols, receipt.RecordProtocols[:])
	return json.Marshal(evidenceSetReceiptEnvelope{
		ProtocolVersion: receipt.ProtocolVersion, RecordCount: receipt.RecordCount,
		RecordProtocols: protocols, CanonicalSHA256: receipt.CanonicalSHA256,
		CanonicalBytes: receipt.CanonicalBytes, Complete: receipt.Complete,
		MetadataOnly:                     receipt.MetadataOnly,
		TimelineLogicalSequenceOnly:      receipt.TimelineLogicalSequenceOnly,
		CrossRecordWallClockOrderClaimed: receipt.CrossRecordWallClockOrderClaimed,
		RawOutputIncluded:                receipt.RawOutputIncluded,
		ProcessIdentityIncluded:          receipt.ProcessIdentityIncluded,
		OSResourceLimitsVerified:         receipt.OSResourceLimitsVerified,
		ProductExecutionEnabled:          receipt.ProductExecutionEnabled,
	})
}

func validateEvidenceSetReceiptCompatibility(raw []byte, exit ExitEvidence,
	runtime RuntimeEvidence, limits ResourceLimitEvidence, cause TerminationCauseEvidence,
	timeline LifecycleTimelineEvidence, budget DeadlineBudgetEvidence,
) error {
	value, code := decodeEvidenceSetReceiptEnvelope(raw)
	if code != "" {
		return evidenceSetReceiptCompatibilityError(code)
	}
	expectedProtocols := [6]string{ExitEvidenceProtocolVersion, RuntimeEvidenceProtocolVersion,
		ResourceLimitEvidenceProtocolVersion, TerminationCauseEvidenceProtocolVersion,
		LifecycleTimelineEvidenceProtocolVersion, DeadlineBudgetEvidenceProtocolVersion}
	if value.RecordProtocols != expectedProtocols {
		return evidenceSetReceiptCompatibilityError(evidenceSetReceiptRejectUnsupportedProtocol)
	}
	if err := value.validate(exit, runtime, limits, cause, timeline, budget); err != nil {
		return evidenceSetReceiptCompatibilityError(evidenceSetReceiptRejectContractMismatch)
	}
	return nil
}

func decodeEvidenceSetReceiptEnvelope(raw []byte) (EvidenceSetReceipt, string) {
	if len(raw) == 0 || len(raw) > MaxEvidenceSetReceiptEnvelopeBytes || !utf8.Valid(raw) {
		return EvidenceSetReceipt{}, evidenceSetReceiptRejectMalformedEnvelope
	}
	seen, err := inspectEvidenceSetReceiptEnvelope(raw)
	if err != nil || len(seen) != len(evidenceSetReceiptEnvelopeFields) {
		return EvidenceSetReceipt{}, evidenceSetReceiptRejectMalformedEnvelope
	}
	for _, field := range evidenceSetReceiptEnvelopeFields {
		if _, ok := seen[field]; !ok {
			return EvidenceSetReceipt{}, evidenceSetReceiptRejectMalformedEnvelope
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope evidenceSetReceiptEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return EvidenceSetReceipt{}, evidenceSetReceiptRejectMalformedEnvelope
	}
	if err := requireJSONEOF(decoder); err != nil {
		return EvidenceSetReceipt{}, evidenceSetReceiptRejectMalformedEnvelope
	}
	if envelope.ProtocolVersion != EvidenceSetReceiptProtocolVersion {
		return EvidenceSetReceipt{}, evidenceSetReceiptRejectUnsupportedProtocol
	}
	if len(envelope.RecordProtocols) != 6 {
		return EvidenceSetReceipt{}, evidenceSetReceiptRejectContractMismatch
	}
	var protocols [6]string
	copy(protocols[:], envelope.RecordProtocols)
	return EvidenceSetReceipt{
		ProtocolVersion: envelope.ProtocolVersion, RecordCount: envelope.RecordCount,
		RecordProtocols: protocols, CanonicalSHA256: envelope.CanonicalSHA256,
		CanonicalBytes: envelope.CanonicalBytes, Complete: envelope.Complete,
		MetadataOnly:                     envelope.MetadataOnly,
		TimelineLogicalSequenceOnly:      envelope.TimelineLogicalSequenceOnly,
		CrossRecordWallClockOrderClaimed: envelope.CrossRecordWallClockOrderClaimed,
		RawOutputIncluded:                envelope.RawOutputIncluded,
		ProcessIdentityIncluded:          envelope.ProcessIdentityIncluded,
		OSResourceLimitsVerified:         envelope.OSResourceLimitsVerified,
		ProductExecutionEnabled:          envelope.ProductExecutionEnabled,
	}, ""
}

func inspectEvidenceSetReceiptEnvelope(raw []byte) (map[string]struct{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("receipt envelope must be an object")
	}
	seen := make(map[string]struct{}, len(evidenceSetReceiptEnvelopeFields))
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, err
		}
		field, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("receipt envelope field is invalid")
		}
		if _, duplicate := seen[field]; duplicate {
			return nil, fmt.Errorf("receipt envelope field is duplicated")
		}
		seen[field] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return nil, fmt.Errorf("receipt envelope is incomplete")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return seen, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("receipt envelope contains trailing JSON")
		}
		return err
	}
	return nil
}

func evidenceSetReceiptCompatibilityError(code string) error {
	return fmt.Errorf("%w: %s", ErrEvidenceSetReceipt, code)
}
