package toolgateway

import (
	"errors"
	"fmt"
	"maps"
	"mime"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/redact"
)

const (
	MaxCallArguments          = 32
	MaxArgumentNameRunes      = 64
	MaxArgumentValueBytes     = 256 * 1024
	MaxCommandBytes           = 16 * 1024
	MaxResultStdoutBytes      = 128 * 1024
	MaxResultStderrBytes      = 32 * 1024
	MaxProposalPreviewBytes   = 64 * 1024
	MaxDecisionReasonRunes    = 2048
	MaxReviewReasonRunes      = 2048
	MaxToolIdentityRunes      = 256
	MaxWorkspaceRootPathRunes = 4096
	MaxMIMEBytes              = 256
	MaxExecutionBackendRunes  = 128
)

type ToolName string

const (
	ReadFileTool      ToolName = "read_file"
	ListWorkspaceTool ToolName = "list_workspace"
	ShellTool         ToolName = "shell"
	ReplaceFileTool   ToolName = "replace_file"
)

func (n ToolName) Valid() bool {
	switch n {
	case ReadFileTool, ListWorkspaceTool, ShellTool, ReplaceFileTool:
		return true
	default:
		return false
	}
}

type ActionClass string

const (
	ClassWorkspaceRead  ActionClass = "workspace_read"
	ClassWorkspaceWrite ActionClass = "workspace_write"
	ClassShell          ActionClass = "shell"
)

func (c ActionClass) Valid() bool {
	switch c {
	case ClassWorkspaceRead, ClassWorkspaceWrite, ClassShell:
		return true
	default:
		return false
	}
}

func ClassForTool(name ToolName) (ActionClass, bool) {
	switch name {
	case ReadFileTool, ListWorkspaceTool:
		return ClassWorkspaceRead, true
	case ReplaceFileTool:
		return ClassWorkspaceWrite, true
	case ShellTool:
		return ClassShell, true
	default:
		return "", false
	}
}

type ApprovalMode string

const (
	ApprovalAutomatic ApprovalMode = "automatic"
	ApprovalPerCall   ApprovalMode = "per_call"
	ApprovalSession   ApprovalMode = "session"
	ApprovalNever     ApprovalMode = "never"
)

func (m ApprovalMode) Valid() bool {
	switch m {
	case ApprovalAutomatic, ApprovalPerCall, ApprovalSession, ApprovalNever:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusProposed  Status = "proposed"
	StatusApproved  Status = "approved"
	StatusDenied    Status = "denied"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

func (s Status) Valid() bool {
	switch s {
	case StatusProposed, StatusApproved, StatusDenied, StatusRunning, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

type ToolCall struct {
	Name          ToolName          `json:"name"`
	Arguments     map[string]string `json:"arguments"`
	RunID         string            `json:"run_id,omitempty"`
	SessionID     string            `json:"session_id,omitempty"`
	WorkspaceID   string            `json:"workspace_id,omitempty"`
	WorkspaceRoot string            `json:"-"`
	RequestedBy   string            `json:"requested_by,omitempty"`
}

func NormalizeToolCall(call ToolCall) (ToolCall, error) {
	call.Name = ToolName(strings.TrimSpace(string(call.Name)))
	call.RunID = strings.TrimSpace(call.RunID)
	call.SessionID = strings.TrimSpace(call.SessionID)
	call.WorkspaceID = strings.TrimSpace(call.WorkspaceID)
	call.WorkspaceRoot = strings.TrimSpace(call.WorkspaceRoot)
	call.RequestedBy = strings.TrimSpace(redact.String(call.RequestedBy))
	if !call.Name.Valid() {
		return ToolCall{}, fmt.Errorf("unsupported tool %q", call.Name)
	}
	for label, value := range map[string]string{
		"run id": call.RunID, "session id": call.SessionID, "workspace id": call.WorkspaceID, "requester": call.RequestedBy,
	} {
		if !utf8.ValidString(value) {
			return ToolCall{}, fmt.Errorf("tool %s must be valid UTF-8", label)
		}
		if len([]rune(value)) > MaxToolIdentityRunes {
			return ToolCall{}, fmt.Errorf("tool %s exceeds %d characters", label, MaxToolIdentityRunes)
		}
	}
	if !utf8.ValidString(call.WorkspaceRoot) {
		return ToolCall{}, errors.New("tool workspace root must be valid UTF-8")
	}
	if len([]rune(call.WorkspaceRoot)) > MaxWorkspaceRootPathRunes {
		return ToolCall{}, fmt.Errorf("tool workspace root exceeds %d characters", MaxWorkspaceRootPathRunes)
	}
	if len(call.Arguments) > MaxCallArguments {
		return ToolCall{}, fmt.Errorf("tool argument list exceeds %d items", MaxCallArguments)
	}
	arguments := make(map[string]string, len(call.Arguments))
	for rawName, value := range call.Arguments {
		name := strings.TrimSpace(rawName)
		if !validArgumentName(name) {
			return ToolCall{}, fmt.Errorf("invalid tool argument name %q", rawName)
		}
		if _, exists := arguments[name]; exists {
			return ToolCall{}, fmt.Errorf("duplicate tool argument %q", name)
		}
		if !utf8.ValidString(value) {
			return ToolCall{}, fmt.Errorf("tool argument %q must be valid UTF-8", name)
		}
		if len([]byte(value)) > MaxArgumentValueBytes {
			return ToolCall{}, fmt.Errorf("tool argument %q exceeds %d bytes", name, MaxArgumentValueBytes)
		}
		arguments[name] = value
	}
	call.Arguments = arguments
	return call, nil
}

func (c ToolCall) Validate() error {
	normalized, err := NormalizeToolCall(c)
	if err != nil {
		return err
	}
	if normalized.Name != c.Name || normalized.RunID != c.RunID || normalized.SessionID != c.SessionID ||
		normalized.WorkspaceID != c.WorkspaceID || normalized.WorkspaceRoot != c.WorkspaceRoot || normalized.RequestedBy != c.RequestedBy ||
		!maps.Equal(normalized.Arguments, c.Arguments) {
		return errors.New("tool call must be normalized")
	}
	return nil
}

type Decision struct {
	Allowed  bool         `json:"allowed"`
	Approval ApprovalMode `json:"approval"`
	Risk     string       `json:"risk"`
	Reason   string       `json:"reason"`
}

func NormalizeDecision(decision Decision) (Decision, error) {
	decision.Risk = strings.ToLower(strings.TrimSpace(decision.Risk))
	decision.Reason = strings.TrimSpace(redact.String(decision.Reason))
	if !decision.Approval.Valid() {
		return Decision{}, fmt.Errorf("invalid approval mode %q", decision.Approval)
	}
	if decision.Allowed && decision.Approval == ApprovalNever {
		return Decision{}, errors.New("allowed decision cannot use never approval")
	}
	if !decision.Allowed && decision.Approval != ApprovalNever {
		return Decision{}, errors.New("denied decision must use never approval")
	}
	if decision.Risk == "" {
		return Decision{}, errors.New("tool decision risk is required")
	}
	switch decision.Risk {
	case "low", "medium", "high", "critical":
	default:
		return Decision{}, fmt.Errorf("invalid tool decision risk %q", decision.Risk)
	}
	if decision.Reason == "" {
		return Decision{}, errors.New("tool decision reason is required")
	}
	if len([]rune(decision.Reason)) > MaxDecisionReasonRunes {
		return Decision{}, fmt.Errorf("tool decision reason exceeds %d characters", MaxDecisionReasonRunes)
	}
	return decision, nil
}

func (d Decision) Validate() error {
	normalized, err := NormalizeDecision(d)
	if err != nil {
		return err
	}
	if normalized != d {
		return errors.New("tool decision must be normalized")
	}
	return nil
}

type Proposal struct {
	ID        string      `json:"id"`
	Tool      ToolName    `json:"tool"`
	Class     ActionClass `json:"class"`
	Status    Status      `json:"status"`
	Preview   string      `json:"preview,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func (p Proposal) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ID) != p.ID || !utf8.ValidString(p.ID) || len([]rune(p.ID)) > MaxToolIdentityRunes {
		return errors.New("tool proposal id is required and bounded")
	}
	if !p.Tool.Valid() || !p.Class.Valid() || !p.Status.Valid() {
		return errors.New("tool proposal tool, class, and status are required")
	}
	class, ok := ClassForTool(p.Tool)
	if !ok || class != p.Class {
		return errors.New("tool proposal class does not match its tool")
	}
	if p.Status != StatusProposed && p.Status != StatusApproved && p.Status != StatusDenied &&
		p.Status != StatusCompleted && p.Status != StatusFailed {
		return fmt.Errorf("invalid tool proposal status %q", p.Status)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() || p.UpdatedAt.Before(p.CreatedAt) {
		return errors.New("tool proposal timestamps are invalid")
	}
	if !utf8.ValidString(p.Preview) || len([]byte(p.Preview)) > MaxProposalPreviewBytes {
		return errors.New("tool proposal preview must be bounded UTF-8")
	}
	return nil
}

type Execution struct {
	Backend     string     `json:"backend"`
	Status      Status     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (e Execution) Validate() error {
	if strings.TrimSpace(e.Backend) == "" || strings.TrimSpace(e.Backend) != e.Backend ||
		!utf8.ValidString(e.Backend) || len([]rune(e.Backend)) > MaxExecutionBackendRunes {
		return errors.New("tool execution backend must be normalized and bounded UTF-8")
	}
	if e.Status != StatusRunning && e.Status != StatusCompleted && e.Status != StatusFailed {
		return fmt.Errorf("invalid tool execution status %q", e.Status)
	}
	if e.StartedAt.IsZero() {
		return errors.New("tool execution started_at is required")
	}
	if e.Status == StatusRunning && e.CompletedAt != nil {
		return errors.New("running tool execution cannot be completed")
	}
	if e.Status != StatusRunning && (e.CompletedAt == nil || e.CompletedAt.Before(e.StartedAt)) {
		return errors.New("terminal tool execution requires a valid completion time")
	}
	return nil
}

type Result struct {
	Status      Status            `json:"status"`
	Stdout      string            `json:"stdout,omitempty"`
	Stderr      string            `json:"stderr,omitempty"`
	ExitCode    int               `json:"exit_code"`
	MIME        string            `json:"mime"`
	Truncated   bool              `json:"truncated"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CompletedAt time.Time         `json:"completed_at"`
}

func (r Result) Validate() error {
	if r.Status != StatusCompleted && r.Status != StatusDenied && r.Status != StatusFailed {
		return fmt.Errorf("invalid tool result status %q", r.Status)
	}
	if !utf8.ValidString(r.Stdout) || !utf8.ValidString(r.Stderr) ||
		len([]byte(r.Stdout)) > MaxResultStdoutBytes || len([]byte(r.Stderr)) > MaxResultStderrBytes {
		return errors.New("tool result output must be bounded UTF-8")
	}
	if strings.TrimSpace(r.MIME) == "" || strings.TrimSpace(r.MIME) != r.MIME || len([]byte(r.MIME)) > MaxMIMEBytes || r.CompletedAt.IsZero() {
		return errors.New("tool result MIME and completion time are required")
	}
	if _, _, err := mime.ParseMediaType(r.MIME); err != nil {
		return fmt.Errorf("invalid tool result MIME %q: %w", r.MIME, err)
	}
	if len(r.Metadata) > MaxCallArguments {
		return fmt.Errorf("tool result metadata exceeds %d items", MaxCallArguments)
	}
	for key, value := range r.Metadata {
		if !validArgumentName(key) || !utf8.ValidString(value) || len([]byte(value)) > MaxArgumentValueBytes {
			return fmt.Errorf("invalid tool result metadata %q", key)
		}
	}
	return nil
}

type Outcome struct {
	Call      ToolCall   `json:"call"`
	Decision  Decision   `json:"decision"`
	Proposal  *Proposal  `json:"proposal,omitempty"`
	Execution *Execution `json:"execution,omitempty"`
	Result    *Result    `json:"result,omitempty"`
}

func (o Outcome) Validate() error {
	if err := o.Call.Validate(); err != nil {
		return err
	}
	if err := o.Decision.Validate(); err != nil {
		return err
	}
	if o.Proposal != nil {
		if err := o.Proposal.Validate(); err != nil {
			return err
		}
		if o.Proposal.Tool != o.Call.Name {
			return errors.New("tool proposal does not match the call")
		}
	}
	if o.Execution != nil {
		if err := o.Execution.Validate(); err != nil {
			return err
		}
	}
	if o.Result != nil {
		if err := o.Result.Validate(); err != nil {
			return err
		}
	}
	if o.Proposal == nil && o.Execution == nil && o.Result == nil {
		return errors.New("tool outcome requires a proposal, execution, or result")
	}
	if !o.Decision.Allowed {
		if o.Execution != nil || o.Result == nil || o.Result.Status != StatusDenied {
			return errors.New("denied tool outcome requires a denied result and no execution")
		}
		if o.Proposal != nil && o.Proposal.Status != StatusDenied {
			return errors.New("denied tool outcome cannot retain an active proposal")
		}
	}
	if o.Execution != nil {
		if !o.Decision.Allowed {
			return errors.New("denied tool outcome cannot include execution")
		}
		if o.Execution.Status == StatusRunning {
			if o.Result != nil {
				return errors.New("running tool outcome cannot include a result")
			}
		} else if o.Result == nil || o.Result.Status != o.Execution.Status {
			return errors.New("terminal tool execution and result statuses must match")
		}
	}
	if o.Result != nil && o.Result.Status != StatusDenied && o.Execution == nil {
		return errors.New("terminal tool result requires execution metadata")
	}
	if o.Result != nil && o.Result.Status == StatusDenied && o.Decision.Allowed {
		return errors.New("denied tool result requires a denied decision")
	}
	if o.Proposal != nil {
		switch o.Proposal.Status {
		case StatusProposed:
			if o.Execution != nil || o.Result != nil {
				return errors.New("proposed tool outcome cannot include execution or result")
			}
		case StatusApproved:
			if o.Result != nil {
				return errors.New("approved tool outcome cannot include a terminal result")
			}
		case StatusDenied:
			if o.Decision.Allowed || o.Result == nil || o.Result.Status != StatusDenied {
				return errors.New("denied proposal requires a denied decision and result")
			}
		case StatusCompleted, StatusFailed:
			if o.Execution == nil || o.Result == nil || o.Execution.Status != o.Proposal.Status || o.Result.Status != o.Proposal.Status {
				return errors.New("terminal proposal, execution, and result statuses must match")
			}
		}
	}
	return nil
}

type ReviewAction string

const (
	ReviewApprove ReviewAction = "approve"
	ReviewDeny    ReviewAction = "deny"
)

type ReviewRequest struct {
	Action        ReviewAction `json:"action"`
	Tool          ToolName     `json:"tool"`
	ProposalID    string       `json:"proposal_id"`
	WorkspaceRoot string       `json:"-"`
	Reason        string       `json:"reason,omitempty"`
}

func NormalizeReviewRequest(request ReviewRequest) (ReviewRequest, error) {
	request.Action = ReviewAction(strings.TrimSpace(string(request.Action)))
	request.Tool = ToolName(strings.TrimSpace(string(request.Tool)))
	request.ProposalID = strings.TrimSpace(request.ProposalID)
	request.WorkspaceRoot = strings.TrimSpace(request.WorkspaceRoot)
	request.Reason = strings.TrimSpace(redact.String(request.Reason))
	if request.Action != ReviewApprove && request.Action != ReviewDeny {
		return ReviewRequest{}, fmt.Errorf("invalid tool review action %q", request.Action)
	}
	if request.Tool != ShellTool && request.Tool != ReplaceFileTool {
		return ReviewRequest{}, fmt.Errorf("tool %q does not support proposal review", request.Tool)
	}
	if request.ProposalID == "" || len([]rune(request.ProposalID)) > MaxToolIdentityRunes {
		return ReviewRequest{}, errors.New("tool review proposal id is required and bounded")
	}
	if !utf8.ValidString(request.WorkspaceRoot) || len([]rune(request.WorkspaceRoot)) > MaxWorkspaceRootPathRunes {
		return ReviewRequest{}, errors.New("tool review workspace root must be bounded UTF-8")
	}
	if !utf8.ValidString(request.Reason) || len([]rune(request.Reason)) > MaxReviewReasonRunes {
		return ReviewRequest{}, fmt.Errorf("tool review reason exceeds %d characters", MaxReviewReasonRunes)
	}
	if request.Action == ReviewApprove && request.Reason != "" {
		return ReviewRequest{}, errors.New("tool approval cannot include a denial reason")
	}
	if request.Tool == ShellTool && request.WorkspaceRoot != "" {
		return ReviewRequest{}, errors.New("shell review cannot include a workspace root")
	}
	if request.Action == ReviewDeny && request.WorkspaceRoot != "" {
		return ReviewRequest{}, errors.New("tool denial cannot include a workspace root")
	}
	return request, nil
}

func validArgumentName(value string) bool {
	if value == "" || len(value) > MaxArgumentNameRunes || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, current := range []byte(value) {
		if (current >= 'a' && current <= 'z') || (current >= '0' && current <= '9') || current == '_' {
			continue
		}
		return false
	}
	return true
}
