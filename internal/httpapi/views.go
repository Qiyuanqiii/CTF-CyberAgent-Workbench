package httpapi

import (
	"encoding/json"
	"time"

	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolbudget"
)

type ScopeView struct {
	WorkspaceID    string   `json:"workspace_id,omitempty"`
	NetworkMode    string   `json:"network_mode"`
	AllowedTargets []string `json:"allowed_targets,omitempty"`
}

type MissionView struct {
	ID          string    `json:"id"`
	Goal        string    `json:"goal"`
	Profile     string    `json:"profile"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	Scope       ScopeView `json:"scope"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RunConfigView struct {
	ModelRoute  string `json:"model_route"`
	Interactive bool   `json:"interactive"`
}

type BudgetView struct {
	MaxTurns       int     `json:"max_turns"`
	MaxTokens      int64   `json:"max_tokens,omitempty"`
	MaxToolCalls   int64   `json:"max_tool_calls,omitempty"`
	MaxCostUSD     float64 `json:"max_cost_usd,omitempty"`
	TimeoutSeconds int64   `json:"timeout_seconds,omitempty"`
}

type RunView struct {
	ID         string        `json:"id"`
	MissionID  string        `json:"mission_id"`
	SessionID  string        `json:"session_id,omitempty"`
	Status     string        `json:"status"`
	Config     RunConfigView `json:"config"`
	Budget     BudgetView    `json:"budget"`
	StartedAt  *time.Time    `json:"started_at,omitempty"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type SupervisorCheckpointView struct {
	RunID           string    `json:"run_id"`
	NextTurn        int       `json:"next_turn"`
	Phase           string    `json:"phase"`
	AttemptID       string    `json:"attempt_id,omitempty"`
	RepairPhase     string    `json:"repair_phase,omitempty"`
	RepairReason    string    `json:"repair_reason,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	ExecutionMillis int64     `json:"execution_millis"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ToolUsageView struct {
	Consumed    int64      `json:"consumed"`
	Limit       int64      `json:"limit"`
	Remaining   int64      `json:"remaining"`
	ExhaustedAt *time.Time `json:"exhausted_at,omitempty"`
}

type RunExecutionLeaseView struct {
	OwnerID    string     `json:"owner_id"`
	Generation int64      `json:"generation"`
	Status     string     `json:"status"`
	Active     bool       `json:"active"`
	AcquiredAt time.Time  `json:"acquired_at"`
	RenewedAt  time.Time  `json:"renewed_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

type RunDetailView struct {
	Run        RunView                   `json:"run"`
	Mission    MissionView               `json:"mission"`
	Checkpoint *SupervisorCheckpointView `json:"checkpoint,omitempty"`
	Lease      *RunExecutionLeaseView    `json:"execution_lease,omitempty"`
	ToolUsage  ToolUsageView             `json:"tool_usage"`
}

type SessionView struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	Title       string    `json:"title"`
	Route       string    `json:"route"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SessionDetailView struct {
	Session SessionView `json:"session"`
	Run     *RunView    `json:"run,omitempty"`
}

type MessageView struct {
	ID            int64     `json:"id"`
	SessionID     string    `json:"session_id"`
	Role          string    `json:"role"`
	Content       string    `json:"content"`
	TokenEstimate int       `json:"token_estimate"`
	Compacted     bool      `json:"compacted"`
	CreatedAt     time.Time `json:"created_at"`
}

type EventView struct {
	EventID   string          `json:"event_id"`
	Version   string          `json:"version"`
	RunID     string          `json:"run_id"`
	MissionID string          `json:"mission_id"`
	Sequence  int64           `json:"sequence"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	SubjectID string          `json:"subject_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type WorkItemView struct {
	ID                 string     `json:"id"`
	RunID              string     `json:"run_id"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Status             string     `json:"status"`
	Priority           string     `json:"priority"`
	Owner              string     `json:"owner,omitempty"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
	Dependencies       []string   `json:"dependencies"`
	BlockedReason      string     `json:"blocked_reason,omitempty"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

type NoteView struct {
	ID          string     `json:"id"`
	RunID       string     `json:"run_id"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Category    string     `json:"category"`
	Visibility  string     `json:"visibility"`
	Owner       string     `json:"owner,omitempty"`
	Tags        []string   `json:"tags"`
	SourceRefs  []string   `json:"source_refs"`
	EvidenceIDs []string   `json:"evidence_ids"`
	Status      string     `json:"status"`
	Pinned      bool       `json:"pinned"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

type ArtifactView struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	SessionID   string    `json:"session_id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	SourceID    string    `json:"source_id"`
	ToolName    string    `json:"tool_name"`
	Stream      string    `json:"stream"`
	Kind        string    `json:"kind"`
	MIME        string    `json:"mime"`
	Encoding    string    `json:"encoding"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int64     `json:"size_bytes"`
	Redacted    bool      `json:"redacted"`
	CreatedAt   time.Time `json:"created_at"`
}

type SupervisorToolCallView struct {
	Position     int             `json:"position"`
	ModelAttempt int             `json:"model_attempt"`
	CallID       string          `json:"call_id"`
	ToolName     string          `json:"tool_name"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorCode    string          `json:"error_code,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

type SupervisorToolRoundView struct {
	RunID        string                   `json:"run_id"`
	Turn         int                      `json:"turn"`
	AttemptID    string                   `json:"attempt_id"`
	Round        int                      `json:"round"`
	ModelAttempt int                      `json:"model_attempt"`
	Calls        []SupervisorToolCallView `json:"calls"`
	CreatedAt    time.Time                `json:"created_at"`
	CompletedAt  *time.Time               `json:"completed_at,omitempty"`
}

func missionView(value domain.Mission) MissionView {
	return MissionView{
		ID: value.ID, Goal: value.Goal, Profile: string(value.Profile), WorkspaceID: value.WorkspaceID,
		Scope: ScopeView{WorkspaceID: value.Scope.WorkspaceID, NetworkMode: value.Scope.NetworkMode,
			AllowedTargets: append([]string{}, value.Scope.AllowedTargets...)},
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func runView(value domain.Run) RunView {
	return RunView{
		ID: value.ID, MissionID: value.MissionID, SessionID: value.SessionID, Status: string(value.Status),
		Config: RunConfigView{ModelRoute: value.Config.ModelRoute, Interactive: value.Config.Interactive},
		Budget: BudgetView{MaxTurns: value.Budget.MaxTurns, MaxTokens: value.Budget.MaxTokens,
			MaxToolCalls: value.Budget.MaxToolCalls, MaxCostUSD: value.Budget.MaxCostUSD,
			TimeoutSeconds: value.Budget.TimeoutSeconds},
		StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func checkpointView(value domain.SupervisorCheckpoint) SupervisorCheckpointView {
	return SupervisorCheckpointView{
		RunID: value.RunID, NextTurn: value.NextTurn, Phase: string(value.Phase), AttemptID: value.AttemptID,
		RepairPhase: string(value.RepairPhase), RepairReason: value.RepairReason, LastError: value.LastError,
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens, TotalTokens: value.TotalTokens,
		ExecutionMillis: value.ExecutionMillis, UpdatedAt: value.UpdatedAt,
	}
}

func toolUsageView(value toolbudget.Usage) ToolUsageView {
	return ToolUsageView{Consumed: value.Consumed, Limit: value.Limit,
		Remaining: value.Remaining, ExhaustedAt: value.ExhaustedAt}
}

func runExecutionLeaseView(value domain.RunExecutionLease, now time.Time) RunExecutionLeaseView {
	return RunExecutionLeaseView{
		OwnerID: value.OwnerID, Generation: value.Generation, Status: string(value.Status),
		Active: value.ActiveAt(now), AcquiredAt: value.AcquiredAt, RenewedAt: value.RenewedAt,
		ExpiresAt: value.ExpiresAt, ReleasedAt: value.ReleasedAt,
	}
}

func sessionView(value session.Session) SessionView {
	return SessionView{ID: value.ID, WorkspaceID: value.WorkspaceID, Title: value.Title, Route: value.Route,
		Status: value.Status, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func messageView(value session.Message) MessageView {
	return MessageView{ID: value.ID, SessionID: value.SessionID, Role: value.Role, Content: value.Content,
		TokenEstimate: value.TokenEstimate, Compacted: value.Compacted, CreatedAt: value.CreatedAt}
}

func eventView(value events.Event) EventView {
	return EventView{EventID: value.EventID, Version: value.Version, RunID: value.RunID,
		MissionID: value.MissionID, Sequence: value.Sequence, Type: value.Type, Source: value.Source,
		SubjectID: value.SubjectID, Payload: json.RawMessage(value.PayloadJSON), CreatedAt: value.CreatedAt}
}

func workItemView(value domain.WorkItem) WorkItemView {
	return WorkItemView{ID: value.ID, RunID: value.RunID, Title: value.Title, Description: value.Description,
		Status: string(value.Status), Priority: string(value.Priority), Owner: value.Owner,
		AcceptanceCriteria: append([]string{}, value.AcceptanceCriteria...),
		Dependencies:       append([]string{}, value.Dependencies...), BlockedReason: value.BlockedReason,
		Version: value.Version, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		CompletedAt: value.CompletedAt}
}

func noteView(value domain.Note) NoteView {
	return NoteView{ID: value.ID, RunID: value.RunID, Title: value.Title, Content: value.Content,
		Category: string(value.Category), Visibility: string(value.Visibility), Owner: value.Owner,
		Tags: append([]string{}, value.Tags...), SourceRefs: append([]string{}, value.SourceRefs...),
		EvidenceIDs: append([]string{}, value.EvidenceIDs...), Status: string(value.Status),
		Pinned: value.Pinned, Version: value.Version, CreatedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt, ArchivedAt: value.ArchivedAt}
}

func artifactView(value artifact.Descriptor) ArtifactView {
	return ArtifactView{ID: value.ID, RunID: value.RunID, SessionID: value.SessionID,
		WorkspaceID: value.WorkspaceID, SourceID: value.SourceID, ToolName: value.ToolName,
		Stream: string(value.Stream), Kind: value.Kind, MIME: value.MIME, Encoding: value.Encoding,
		SHA256: value.SHA256, SizeBytes: value.SizeBytes, Redacted: value.Redacted, CreatedAt: value.CreatedAt}
}

func supervisorToolRoundView(value domain.SupervisorToolRound) SupervisorToolRoundView {
	calls := make([]SupervisorToolCallView, len(value.Calls))
	for index, call := range value.Calls {
		calls[index] = SupervisorToolCallView{
			Position: call.Position, ModelAttempt: call.ModelAttempt, CallID: call.CallID,
			ToolName: call.ToolName, Payload: json.RawMessage(call.PayloadJSON), Status: string(call.Status),
			Result: json.RawMessage(call.ResultJSON), ErrorCode: call.ErrorCode,
			CreatedAt: call.CreatedAt, CompletedAt: call.CompletedAt,
		}
	}
	return SupervisorToolRoundView{RunID: value.RunID, Turn: value.Turn, AttemptID: value.AttemptID,
		Round: value.Round, ModelAttempt: value.ModelAttempt, Calls: calls,
		CreatedAt: value.CreatedAt, CompletedAt: value.CompletedAt}
}
