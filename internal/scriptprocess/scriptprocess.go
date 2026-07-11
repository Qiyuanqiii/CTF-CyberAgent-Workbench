package scriptprocess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/artifact"
)

const (
	Schema                  = "script_process.v1"
	ExecutionDisabled       = "disabled"
	BackendSandbox          = "sandbox"
	BackendLocal            = "local"
	MaxArguments            = 128
	MaxExecutableBytes      = 4096
	MaxArgumentBytes        = 8192
	MaxIdentityRunes        = 256
	MaxPolicyReasonRunes    = 2048
	MaxStdoutBytes          = artifact.MaxContentBytes
	MaxStderrBytes          = artifact.MaxContentBytes
	MaxEncodedProposalBytes = 64 * 1024
)

type Status string

const (
	StatusProposed  Status = "proposed"
	StatusApproved  Status = "approved"
	StatusDenied    Status = "denied"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

func (s Status) Valid() bool {
	switch s {
	case StatusProposed, StatusApproved, StatusDenied, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

type Proposal struct {
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments"`
	WorkingDirectory string   `json:"working_directory"`
	RequestedBackend string   `json:"requested_backend"`
}

type Envelope struct {
	Schema           string   `json:"schema"`
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments"`
	WorkingDirectory string   `json:"working_directory"`
	RequestedBackend string   `json:"requested_backend"`
	ExecutionMode    string   `json:"execution_mode"`
}

func NormalizeProposal(proposal Proposal) (Proposal, error) {
	proposal.Executable = strings.TrimSpace(proposal.Executable)
	proposal.WorkingDirectory = strings.TrimSpace(proposal.WorkingDirectory)
	proposal.RequestedBackend = strings.ToLower(strings.TrimSpace(proposal.RequestedBackend))
	if proposal.WorkingDirectory == "" {
		proposal.WorkingDirectory = "."
	}
	if proposal.RequestedBackend == "" {
		proposal.RequestedBackend = BackendSandbox
	}
	if proposal.Executable == "" {
		return Proposal{}, errors.New("script process executable is required")
	}
	if !utf8.ValidString(proposal.Executable) || strings.ContainsRune(proposal.Executable, 0) ||
		len([]byte(proposal.Executable)) > MaxExecutableBytes {
		return Proposal{}, fmt.Errorf("script process executable must be valid UTF-8 without NUL and at most %d bytes", MaxExecutableBytes)
	}
	if proposal.WorkingDirectory != "." {
		return Proposal{}, errors.New("script process working directory must be the workspace root")
	}
	if proposal.RequestedBackend != BackendSandbox && proposal.RequestedBackend != BackendLocal {
		return Proposal{}, fmt.Errorf("unsupported script process backend %q", proposal.RequestedBackend)
	}
	if len(proposal.Arguments) > MaxArguments {
		return Proposal{}, fmt.Errorf("script process arguments exceed %d items", MaxArguments)
	}
	arguments := make([]string, len(proposal.Arguments))
	for index, argument := range proposal.Arguments {
		if !utf8.ValidString(argument) || strings.ContainsRune(argument, 0) || len([]byte(argument)) > MaxArgumentBytes {
			return Proposal{}, fmt.Errorf("script process argument %d must be valid UTF-8 without NUL and at most %d bytes", index, MaxArgumentBytes)
		}
		arguments[index] = argument
	}
	proposal.Arguments = arguments
	return proposal, nil
}

func EncodeProposal(proposal Proposal) (string, error) {
	normalized, err := NormalizeProposal(proposal)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(Envelope{
		Schema: Schema, Executable: normalized.Executable, Arguments: normalized.Arguments,
		WorkingDirectory: normalized.WorkingDirectory, RequestedBackend: normalized.RequestedBackend,
		ExecutionMode: ExecutionDisabled,
	})
	if err != nil {
		return "", err
	}
	if len(payload) > MaxEncodedProposalBytes {
		return "", fmt.Errorf("script process proposal exceeds %d bytes", MaxEncodedProposalBytes)
	}
	return string(payload), nil
}

type Process struct {
	ID                  string
	OperationKeyDigest  string
	RunID               string
	SessionID           string
	WorkspaceID         string
	Executable          string
	Arguments           []string
	WorkingDirectory    string
	RequestedBackend    string
	ExecutionMode       string
	Status              Status
	Risk                string
	PolicyReason        string
	Stdout              string
	Stderr              string
	ExitCode            int
	RequestFingerprint  string
	ApprovalFingerprint string
	RequestedBy         string
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (p Process) Proposal() Proposal {
	return Proposal{
		Executable: p.Executable, Arguments: append([]string(nil), p.Arguments...),
		WorkingDirectory: p.WorkingDirectory, RequestedBackend: p.RequestedBackend,
	}
}

func (p Process) Validate() error {
	for label, value := range map[string]string{
		"id": p.ID, "run id": p.RunID, "session id": p.SessionID, "workspace id": p.WorkspaceID,
		"requested by": p.RequestedBy,
	} {
		if strings.TrimSpace(value) != value || !utf8.ValidString(value) || len([]rune(value)) > MaxIdentityRunes {
			return fmt.Errorf("script process %s must be normalized and bounded UTF-8", label)
		}
	}
	if p.ID == "" || p.RunID == "" || p.SessionID == "" || p.WorkspaceID == "" || p.RequestedBy == "" {
		return errors.New("script process identity, Run, Session, Workspace, and requester are required")
	}
	if !validFingerprint(p.OperationKeyDigest) || !validFingerprint(p.RequestFingerprint) || !validFingerprint(p.ApprovalFingerprint) {
		return errors.New("script process operation and request fingerprints must be SHA-256 hex digests")
	}
	normalizedProposal, err := NormalizeProposal(p.Proposal())
	if err != nil {
		return err
	}
	if normalizedProposal.Executable != p.Executable || normalizedProposal.WorkingDirectory != p.WorkingDirectory ||
		normalizedProposal.RequestedBackend != p.RequestedBackend || !slices.Equal(normalizedProposal.Arguments, p.Arguments) {
		return errors.New("script process proposal fields must be normalized")
	}
	if p.ExecutionMode != ExecutionDisabled {
		return errors.New("script process execution mode must remain disabled")
	}
	if !p.Status.Valid() {
		return fmt.Errorf("invalid script process status %q", p.Status)
	}
	normalizedRisk := strings.ToLower(strings.TrimSpace(p.Risk))
	if normalizedRisk == "" || normalizedRisk != p.Risk {
		return errors.New("script process risk is required")
	}
	switch normalizedRisk {
	case "low", "medium", "high", "critical":
	default:
		return fmt.Errorf("invalid script process risk %q", p.Risk)
	}
	if strings.TrimSpace(p.PolicyReason) == "" || strings.TrimSpace(p.PolicyReason) != p.PolicyReason ||
		!utf8.ValidString(p.PolicyReason) || len([]rune(p.PolicyReason)) > MaxPolicyReasonRunes {
		return errors.New("script process policy reason is required and bounded")
	}
	if !utf8.ValidString(p.Stdout) || !utf8.ValidString(p.Stderr) ||
		len([]byte(p.Stdout)) > MaxStdoutBytes || len([]byte(p.Stderr)) > MaxStderrBytes {
		return errors.New("script process output must be bounded UTF-8")
	}
	if p.Status == StatusDenied && p.Stdout != "" {
		return errors.New("denied script process cannot have stdout")
	}
	if p.Status != StatusCompleted && p.Status != StatusFailed && (p.Stdout != "" || p.Stderr != "" || p.ExitCode != 0) {
		return errors.New("non-terminal script process cannot have execution output")
	}
	if p.Version <= 0 || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() || p.UpdatedAt.Before(p.CreatedAt) {
		return errors.New("script process version and timestamps are invalid")
	}
	return nil
}

type ListFilter struct {
	RunID     string
	SessionID string
	Status    Status
	Limit     int
}

type Store interface {
	SaveScriptProcess(ctx context.Context, process Process) (Process, error)
	GetScriptProcess(ctx context.Context, id string) (Process, error)
	ListScriptProcesses(ctx context.Context, filter ListFilter) ([]Process, error)
}

type Manager struct {
	store Store
}

func NewManager(store Store) *Manager {
	return &Manager{store: store}
}

func (m *Manager) Get(ctx context.Context, id string) (Process, error) {
	if m == nil || m.store == nil {
		return Process{}, errors.New("script process store is required")
	}
	return m.store.GetScriptProcess(ctx, strings.TrimSpace(id))
}

func (m *Manager) List(ctx context.Context, filter ListFilter) ([]Process, error) {
	if m == nil || m.store == nil {
		return nil, errors.New("script process store is required")
	}
	return m.store.ListScriptProcesses(ctx, filter)
}

func (m *Manager) Approve(ctx context.Context, id string) (Process, error) {
	process, err := m.Get(ctx, id)
	if err != nil {
		return Process{}, err
	}
	if process.Status == StatusCompleted {
		return process, nil
	}
	if process.Status != StatusProposed && process.Status != StatusApproved {
		return Process{}, fmt.Errorf("script process %s is %s, not proposed", process.ID, process.Status)
	}
	if process.Status == StatusProposed {
		process.Status = StatusApproved
		process.Version++
		process.UpdatedAt = time.Now().UTC()
		process, err = m.store.SaveScriptProcess(ctx, process)
		if err != nil {
			return Process{}, err
		}
	}
	payload, err := EncodeProposal(process.Proposal())
	if err != nil {
		return Process{}, err
	}
	process.Status = StatusCompleted
	process.Stdout = "dry run: " + payload
	process.Stderr = ""
	process.ExitCode = 0
	process.Version++
	process.UpdatedAt = time.Now().UTC()
	return m.store.SaveScriptProcess(ctx, process)
}

func (m *Manager) Deny(ctx context.Context, id string, reason string) (Process, error) {
	process, err := m.Get(ctx, id)
	if err != nil {
		return Process{}, err
	}
	if process.Status == StatusDenied {
		return process, nil
	}
	if process.Status != StatusProposed {
		return Process{}, fmt.Errorf("script process %s is %s, not proposed", process.ID, process.Status)
	}
	process.Status = StatusDenied
	if strings.TrimSpace(reason) != "" {
		process.PolicyReason = strings.TrimSpace(reason)
	}
	process.Version++
	process.UpdatedAt = time.Now().UTC()
	return m.store.SaveScriptProcess(ctx, process)
}

func OperationKeyDigest(key string) string {
	return Fingerprint("script_process_operation_key.v1", strings.TrimSpace(key))
}

func Fingerprint(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		value := []byte(part)
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{'|'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validFingerprint(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
