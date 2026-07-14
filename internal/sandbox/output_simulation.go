package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/redact"
)

const (
	OutputFixtureProtocolVersion    = "sandbox_output_fixture.v1"
	OutputSimulationProtocolVersion = "sandbox_output_simulation.v1"
	OutputSimulationStatusCommitted = "simulation_committed"
	OutputFileTypeStream            = "stream"
	OutputFileTypeRegular           = "regular"
	OutputFileTypeSymlink           = "symlink"
	OutputFileTypeSpecial           = "special"
	MaxOutputFixtureBytes           = 32 * 1024 * 1024
	MaxOutputSimulationsPerEvidence = 8
)

type OutputFixtureItem struct {
	Kind     string `json:"kind"`
	FileType string `json:"file_type"`
	Content  string `json:"content"`
}

type OutputFixture struct {
	ProtocolVersion string              `json:"protocol_version"`
	Outputs         []OutputFixtureItem `json:"outputs"`
}

func DecodeOutputFixture(data []byte) (OutputFixture, error) {
	if len(data) == 0 || len(data) > MaxOutputFixtureBytes || !utf8.Valid(data) {
		return OutputFixture{}, errors.New("sandbox output fixture must be bounded UTF-8")
	}
	if err := rejectDuplicateOutputFixtureFields(data); err != nil {
		return OutputFixture{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture OutputFixture
	if err := decoder.Decode(&fixture); err != nil {
		return OutputFixture{}, fmt.Errorf("decode sandbox output fixture: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return OutputFixture{}, errors.New("sandbox output fixture contains trailing data")
	}
	if fixture.ProtocolVersion != OutputFixtureProtocolVersion || len(fixture.Outputs) < 1 ||
		len(fixture.Outputs) > MaxOutputPaths+2 {
		return OutputFixture{}, errors.New("sandbox output fixture protocol or output count is invalid")
	}
	return fixture, nil
}

type StagedOutputDescriptor struct {
	Ordinal                 int
	Kind                    string
	LocatorFingerprint      string
	MIME                    string
	SHA256                  string
	SizeBytes               int64
	Redacted                bool
	FakeArtifactFingerprint string
}

func (descriptor StagedOutputDescriptor) Validate() error {
	if descriptor.Ordinal < 1 || descriptor.Ordinal > MaxOutputPaths+2 ||
		(descriptor.Kind != OutputKindStdout && descriptor.Kind != OutputKindStderr &&
			descriptor.Kind != OutputKindFile) || !validDigest(descriptor.LocatorFingerprint) ||
		!validDigest(descriptor.SHA256) || !validDigest(descriptor.FakeArtifactFingerprint) ||
		descriptor.SizeBytes < 1 || descriptor.SizeBytes > MaxCapturedOutputBytes ||
		descriptor.MIME == "" || len([]byte(descriptor.MIME)) > 256 {
		return errors.New("sandbox staged output descriptor is invalid")
	}
	if _, _, err := mime.ParseMediaType(descriptor.MIME); err != nil {
		return errors.New("sandbox staged output MIME is invalid")
	}
	return nil
}

type StagedOutput struct {
	Descriptor StagedOutputDescriptor
	Content    string
}

func (output StagedOutput) Validate() error {
	if err := output.Descriptor.Validate(); err != nil {
		return err
	}
	if !utf8.ValidString(output.Content) || int64(len([]byte(output.Content))) != output.Descriptor.SizeBytes ||
		digestOutputContent(output.Content) != output.Descriptor.SHA256 {
		return errors.New("sandbox staged output content integrity is invalid")
	}
	return nil
}

type FakeArtifactSink struct {
	failAtOrdinal int
	artifacts     []StagedOutput
}

func NewFakeArtifactSink(failAtOrdinal int) *FakeArtifactSink {
	return &FakeArtifactSink{failAtOrdinal: failAtOrdinal}
}

func (sink *FakeArtifactSink) CommitAtomic(ctx context.Context,
	outputs []StagedOutput,
) ([]string, error) {
	if sink == nil {
		return nil, errors.New("fake Artifact sink is required")
	}
	pending := make([]StagedOutput, len(outputs))
	ids := make([]string, len(outputs))
	for index, output := range outputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := output.Validate(); err != nil {
			return nil, err
		}
		if output.Descriptor.Ordinal != index+1 {
			return nil, errors.New("fake Artifact commit order is invalid")
		}
		if sink.failAtOrdinal == output.Descriptor.Ordinal {
			return nil, fmt.Errorf("fake Artifact commit failed at ordinal %d", sink.failAtOrdinal)
		}
		pending[index] = output
		ids[index] = output.Descriptor.FakeArtifactFingerprint
	}
	sink.artifacts = pending
	return ids, nil
}

func (sink *FakeArtifactSink) Snapshot() []StagedOutput {
	if sink == nil {
		return nil
	}
	return append([]StagedOutput(nil), sink.artifacts...)
}

type OutputSimulationResult struct {
	Descriptors       []StagedOutputDescriptor
	TotalBytes        int64
	FixtureDigest     string
	TransactionDigest string
	FakeCommitCount   int
}

type OutputSimulationHarness interface {
	Simulate(ctx context.Context, plan OutputExportPlan,
		fixture OutputFixture) (OutputSimulationResult, error)
}

type InMemoryOutputHarness struct {
	FailCommitAtOrdinal int
}

func NewInMemoryOutputHarness() InMemoryOutputHarness {
	return InMemoryOutputHarness{}
}

func (harness InMemoryOutputHarness) Simulate(ctx context.Context, plan OutputExportPlan,
	fixture OutputFixture,
) (OutputSimulationResult, error) {
	if err := ctx.Err(); err != nil {
		return OutputSimulationResult{}, err
	}
	if err := plan.Validate(); err != nil {
		return OutputSimulationResult{}, err
	}
	if fixture.ProtocolVersion != OutputFixtureProtocolVersion ||
		len(fixture.Outputs) != len(plan.Slots) {
		return OutputSimulationResult{}, errors.New("sandbox output fixture does not match the export plan")
	}
	staged := make([]StagedOutput, len(plan.Slots))
	var total int64
	fixtureParts := []string{OutputFixtureProtocolVersion, strconv.Itoa(len(fixture.Outputs))}
	for index, slot := range plan.Slots {
		if err := ctx.Err(); err != nil {
			return OutputSimulationResult{}, err
		}
		item := fixture.Outputs[index]
		if item.Kind != slot.Kind {
			return OutputSimulationResult{}, errors.New("sandbox output fixture kind or order changed")
		}
		if slot.Kind == OutputKindFile {
			if item.FileType != OutputFileTypeRegular {
				return OutputSimulationResult{}, errors.New("sandbox output fixture rejected a symlink or special file")
			}
		} else if item.FileType != OutputFileTypeStream {
			return OutputSimulationResult{}, errors.New("sandbox stream fixture file type is invalid")
		}
		if item.Content == "" || !utf8.ValidString(item.Content) {
			return OutputSimulationResult{}, errors.New("sandbox output fixture content must be non-empty UTF-8")
		}
		if int64(len([]byte(item.Content))) > plan.MaxOutputBytes-total {
			return OutputSimulationResult{}, errors.New("sandbox output fixture exceeds the aggregate output limit")
		}
		redacted := redact.String(item.Content)
		redactedChanged := redacted != item.Content
		if redacted == "" || !utf8.ValidString(redacted) {
			return OutputSimulationResult{}, errors.New("sandbox output fixture redaction produced invalid content")
		}
		size := int64(len([]byte(redacted)))
		if size > plan.MaxOutputBytes-total {
			return OutputSimulationResult{}, errors.New("sandbox redacted output exceeds the aggregate output limit")
		}
		total += size
		detectedMIME := http.DetectContentType([]byte(redacted))
		if _, _, err := mime.ParseMediaType(detectedMIME); err != nil {
			return OutputSimulationResult{}, errors.New("sandbox output MIME detection failed")
		}
		descriptor := StagedOutputDescriptor{
			Ordinal: slot.Ordinal, Kind: slot.Kind, LocatorFingerprint: slot.LocatorFingerprint,
			MIME: detectedMIME, SHA256: digestOutputContent(redacted), SizeBytes: size,
			Redacted: redactedChanged,
		}
		descriptor.FakeArtifactFingerprint = fingerprint("sandbox_fake_output_artifact.v1",
			strconv.Itoa(descriptor.Ordinal), descriptor.Kind, descriptor.LocatorFingerprint,
			descriptor.MIME, descriptor.SHA256, strconv.FormatInt(descriptor.SizeBytes, 10),
			strconv.FormatBool(descriptor.Redacted))
		staged[index] = StagedOutput{Descriptor: descriptor, Content: redacted}
		fixtureParts = append(fixtureParts, strconv.Itoa(index+1), item.Kind, item.FileType,
			descriptor.SHA256)
	}
	sink := NewFakeArtifactSink(harness.FailCommitAtOrdinal)
	ids, err := sink.CommitAtomic(ctx, staged)
	if err != nil {
		if len(sink.Snapshot()) != 0 {
			return OutputSimulationResult{}, errors.New("fake Artifact transaction violated atomic rollback")
		}
		return OutputSimulationResult{}, err
	}
	if len(ids) != len(staged) || len(sink.Snapshot()) != len(staged) {
		return OutputSimulationResult{}, errors.New("fake Artifact transaction did not commit every staged output")
	}
	descriptors := make([]StagedOutputDescriptor, len(staged))
	transactionParts := []string{OutputSimulationProtocolVersion, plan.Fingerprint,
		strconv.FormatInt(total, 10), strconv.Itoa(len(staged))}
	for index, output := range staged {
		descriptors[index] = output.Descriptor
		transactionParts = append(transactionParts, output.Descriptor.FakeArtifactFingerprint)
	}
	return OutputSimulationResult{
		Descriptors: descriptors, TotalBytes: total,
		FixtureDigest:     fingerprint(fixtureParts...),
		TransactionDigest: fingerprint(transactionParts...), FakeCommitCount: len(ids),
	}, nil
}

type OutputSimulation struct {
	ID                       string
	EvidenceID               string
	PreflightID              string
	ExecutionID              string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ProtocolVersion          string
	Status                   string
	OutputPlanFingerprint    string
	FixtureDigest            string
	TransactionDigest        string
	ExpectedSlotCount        int
	StagedOutputCount        int
	StagedOutputBytes        int64
	FakeArtifactCount        int
	ProductionArtifactCount  int
	AllOrNothing             bool
	SimulationOnly           bool
	ArtifactCommitAuthorized bool
	BackendEnabled           bool
	ExecutionAuthorized      bool
	Descriptors              []StagedOutputDescriptor
	RequestedBy              string
	CreatedAt                time.Time
	Replayed                 bool
}

func (simulation OutputSimulation) Validate() error {
	for label, value := range map[string]string{
		"output simulation id": simulation.ID, "output simulation evidence id": simulation.EvidenceID,
		"output simulation preflight id": simulation.PreflightID,
		"output simulation execution id": simulation.ExecutionID,
		"output simulation Run id":       simulation.RunID,
		"output simulation Mission id":   simulation.MissionID,
		"output simulation workspace id": simulation.WorkspaceID,
		"output simulation requester":    simulation.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if simulation.ProtocolVersion != OutputSimulationProtocolVersion ||
		simulation.Status != OutputSimulationStatusCommitted ||
		!validDigest(simulation.OutputPlanFingerprint) || !validDigest(simulation.FixtureDigest) ||
		!validDigest(simulation.TransactionDigest) || simulation.ExpectedSlotCount < 1 ||
		simulation.ExpectedSlotCount > MaxOutputPaths+2 ||
		simulation.StagedOutputCount != simulation.ExpectedSlotCount ||
		simulation.StagedOutputCount != len(simulation.Descriptors) ||
		simulation.FakeArtifactCount != simulation.StagedOutputCount ||
		simulation.StagedOutputBytes < 1 || simulation.StagedOutputBytes > MaxCapturedOutputBytes ||
		simulation.ProductionArtifactCount != 0 || !simulation.AllOrNothing || !simulation.SimulationOnly ||
		simulation.ArtifactCommitAuthorized || simulation.BackendEnabled || simulation.ExecutionAuthorized ||
		simulation.CreatedAt.IsZero() {
		return errors.New("sandbox output simulation must remain atomic, simulation-only, and unauthorized")
	}
	var total int64
	for index, descriptor := range simulation.Descriptors {
		if descriptor.Ordinal != index+1 {
			return errors.New("sandbox output simulation descriptor order is invalid")
		}
		if err := descriptor.Validate(); err != nil {
			return err
		}
		total += descriptor.SizeBytes
	}
	if total != simulation.StagedOutputBytes {
		return errors.New("sandbox output simulation byte total is invalid")
	}
	return nil
}

type OutputSimulationOperation struct {
	KeyDigest          string
	RequestFingerprint string
	SimulationID       string
	EvidenceID         string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (operation OutputSimulationOperation) Validate() error {
	for label, value := range map[string]string{
		"output simulation operation simulation id": operation.SimulationID,
		"output simulation operation evidence id":   operation.EvidenceID,
		"output simulation operation Run id":        operation.RunID,
		"output simulation operation requester":     operation.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(operation.KeyDigest) || !validDigest(operation.RequestFingerprint) ||
		operation.CreatedAt.IsZero() {
		return errors.New("sandbox output simulation operation is invalid")
	}
	return nil
}

func OutputSimulationRequestFingerprint(simulation OutputSimulation) string {
	return fingerprint("sandbox_output_simulation_request.v1", simulation.EvidenceID,
		simulation.OutputPlanFingerprint, simulation.FixtureDigest,
		simulation.TransactionDigest, simulation.RequestedBy)
}

func digestOutputContent(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func rejectDuplicateOutputFixtureFields(data []byte) error {
	allowed := map[string]struct{}{
		"protocol_version": {}, "outputs": {}, "kind": {}, "file_type": {}, "content": {},
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 6 {
			return errors.New("sandbox output fixture JSON is too deep")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				fieldToken, err := decoder.Token()
				if err != nil {
					return err
				}
				field, ok := fieldToken.(string)
				if !ok {
					return errors.New("sandbox output fixture field name is invalid")
				}
				if _, exists := seen[field]; exists {
					return fmt.Errorf("sandbox output fixture contains duplicate field %q", field)
				}
				if _, exists := allowed[field]; !exists {
					return fmt.Errorf("sandbox output fixture contains unknown field %q", field)
				}
				seen[field] = struct{}{}
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return errors.New("sandbox output fixture object is not closed")
			}
		case '[':
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return errors.New("sandbox output fixture array is not closed")
			}
		default:
			return errors.New("sandbox output fixture contains an unexpected delimiter")
		}
		return nil
	}
	if err := walk(1); err != nil {
		return fmt.Errorf("inspect sandbox output fixture JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("sandbox output fixture contains trailing data")
	}
	return nil
}
