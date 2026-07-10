package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/toolbudget"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/toolrun"
)

const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

type Session struct {
	ID          string
	WorkspaceID string
	Title       string
	Route       string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func New(workspaceID string, title string, route string) Session {
	if strings.TrimSpace(route) == "" {
		route = "learn"
	}
	if strings.TrimSpace(title) == "" {
		title = "New session"
	}
	now := time.Now().UTC()
	return Session{
		ID:          newID("sess"),
		WorkspaceID: strings.TrimSpace(workspaceID),
		Title:       strings.TrimSpace(title),
		Route:       strings.TrimSpace(route),
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (s Session) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("session id is required")
	}
	if strings.TrimSpace(s.Title) == "" {
		return errors.New("session title is required")
	}
	if strings.TrimSpace(s.Route) == "" {
		return errors.New("session route is required")
	}
	if s.Status != StatusActive && s.Status != StatusArchived {
		return fmt.Errorf("invalid session status %q", s.Status)
	}
	if s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() {
		return errors.New("session timestamps are required")
	}
	return nil
}

type Message struct {
	ID            int64
	SessionID     string
	Role          string
	Content       string
	TokenEstimate int
	Compacted     bool
	CreatedAt     time.Time
}

type WorkspaceInfo struct {
	ID       string
	Name     string
	RootPath string
}

type Store interface {
	fileedit.Store
	toolrun.Store
	approval.Store
	approval.GrantStore
	toolbudget.Store

	SaveSession(ctx context.Context, session Session) error
	GetSession(ctx context.Context, id string) (Session, error)
	ListSessions(ctx context.Context) ([]Session, error)
	GetWorkspaceInfo(ctx context.Context, id string) (WorkspaceInfo, error)
	SaveSessionMessage(ctx context.Context, message Message) (Message, error)
	ListSessionMessages(ctx context.Context, sessionID string, includeCompacted bool) ([]Message, error)
	MarkSessionMessagesCompacted(ctx context.Context, sessionID string, throughID int64) (int64, error)
	SaveContextSummary(ctx context.Context, summary contextmgr.Summary) (contextmgr.Summary, error)
	LatestContextSummary(ctx context.Context, taskID string) (contextmgr.Summary, bool, error)
}

type Manager struct {
	store      Store
	router     *llm.Router
	checker    policy.Checker
	runChat    RunChatExecutor
	contextMgr *contextmgr.Manager
	gateway    *toolgateway.Gateway
	fileEdits  *toolgateway.FileEditAdapter
	toolRuns   *toolgateway.ToolRunAdapter
}

type SendResult struct {
	Session      Session
	UserMessage  Message
	ReplyMessage Message
	Text         string
	Command      bool
	Compacted    bool
	SummaryID    int64
	FileEditID   string
	ToolRunID    string
	RunID        string
	RunAction    string
	RunStatus    string
}

type RunChatResult struct {
	RunID        string
	UserMessage  Message
	ReplyMessage Message
	Text         string
	Action       string
	RunStatus    string
}

type TurnMessages struct {
	User      Message
	Assistant Message
}

type RunChatExecutor interface {
	ExecuteSessionTurn(ctx context.Context, sess Session, input string) (RunChatResult, bool, error)
}

func NewManager(store Store, router *llm.Router, checker policy.Checker) *Manager {
	gateway := toolgateway.New(store, checker).WithWorkspaceRootResolver(func(ctx context.Context, workspaceID string) (string, error) {
		workspace, err := store.GetWorkspaceInfo(ctx, workspaceID)
		return workspace.RootPath, err
	})
	return &Manager{
		store:      store,
		router:     router,
		checker:    checker,
		contextMgr: contextmgr.NewManager(store, contextmgr.DefaultConfig()),
		gateway:    gateway,
		fileEdits:  gateway.FileEdits(),
		toolRuns:   gateway.ToolRuns(),
	}
}

func (m *Manager) WithRunChatExecutor(executor RunChatExecutor) *Manager {
	if m != nil {
		m.runChat = executor
	}
	return m
}

func (m *Manager) Create(ctx context.Context, workspaceID string, title string, route string) (Session, error) {
	session := New(workspaceID, title, route)
	if err := m.store.SaveSession(ctx, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (m *Manager) Send(ctx context.Context, sessionID string, input string) (SendResult, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return SendResult{}, errors.New("message is required")
	}
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return SendResult{}, err
	}
	if sess.Status != StatusActive {
		return SendResult{}, fmt.Errorf("session %s is not active", sess.ID)
	}

	if strings.HasPrefix(input, "/") {
		userMsg, err := m.store.SaveSessionMessage(ctx, NewMessage(sess.ID, "user", input))
		if err != nil {
			return SendResult{}, err
		}
		return m.handleSlash(ctx, sess, userMsg, input)
	}
	if m.runChat != nil {
		runResult, handled, err := m.runChat.ExecuteSessionTurn(ctx, sess, input)
		if err != nil {
			return SendResult{}, err
		}
		if handled {
			compacted, summaryID, err := m.compactAfterTurn(ctx, sess)
			if err != nil {
				return SendResult{}, err
			}
			return SendResult{
				Session: sess, UserMessage: runResult.UserMessage, ReplyMessage: runResult.ReplyMessage,
				Text: runResult.Text, Compacted: compacted, SummaryID: summaryID,
				RunID: runResult.RunID, RunAction: runResult.Action, RunStatus: runResult.RunStatus,
			}, nil
		}
	}
	userMsg, err := m.store.SaveSessionMessage(ctx, NewMessage(sess.ID, "user", input))
	if err != nil {
		return SendResult{}, err
	}
	return m.chat(ctx, sess, userMsg)
}

func (m *Manager) History(ctx context.Context, sessionID string, includeCompacted bool) ([]Message, error) {
	return m.store.ListSessionMessages(ctx, sessionID, includeCompacted)
}

func (m *Manager) List(ctx context.Context) ([]Session, error) {
	return m.store.ListSessions(ctx)
}

func (m *Manager) handleSlash(ctx context.Context, sess Session, userMsg Message, input string) (SendResult, error) {
	fields := strings.Fields(input)
	command := strings.TrimPrefix(fields[0], "/")
	args := fields[1:]

	var text string
	var compacted bool
	var summaryID int64
	var fileEditID string
	var toolRunID string
	switch command {
	case "help":
		text = "Commands: /help, /compact, /model, /model <route>, /workspace, /ls [path], /read <path>, /write <path> <content>, /run <command>. /write and /run require explicit approval unless an active scoped Session grant applies."
	case "compact":
		result, err := m.compactActiveMessages(ctx, sess)
		if err != nil {
			return SendResult{}, err
		}
		compacted = result.Compacted
		summaryID = result.Summary.ID
		text = fmt.Sprintf("Context compacted: summary=%d removed=%d preserved=%d", result.Summary.ID, result.RemovedMessages, len(result.Preserved))
	case "model":
		if len(args) > 0 {
			sess.Route = args[0]
			sess.UpdatedAt = time.Now().UTC()
			if err := m.store.SaveSession(ctx, sess); err != nil {
				return SendResult{}, err
			}
		}
		ref, err := m.resolveModelRef(sess.Route)
		if err != nil {
			return SendResult{}, err
		}
		text = fmt.Sprintf("Session route: %s -> %s/%s", sess.Route, ref.Provider, ref.Model)
	case "workspace":
		if sess.WorkspaceID == "" {
			text = "No workspace is attached to this session."
		} else {
			text = "Workspace: " + sess.WorkspaceID
		}
	case "ls":
		text = m.handleWorkspaceList(ctx, sess, strings.Join(args, " "))
	case "read":
		text = m.handleWorkspaceRead(ctx, sess, strings.Join(args, " "))
	case "write":
		path, content, ok := parseWriteCommand(input)
		if !ok {
			text = "Usage: /write <workspace-relative-path> <replacement-content>. This creates a file edit proposal and does not write immediately."
			break
		}
		edit, response := m.handleWorkspaceWrite(ctx, sess, path, content)
		text = response
		fileEditID = edit.ID
	case "run":
		if len(args) == 0 {
			text = "Usage: /run <command>. v0.1 creates a tool proposal and does not execute directly from session chat."
			break
		}
		requested := strings.Join(args, " ")
		run, err := m.toolRuns.ProposeShell(ctx, sess.ID, sess.WorkspaceID, requested)
		if err != nil {
			return SendResult{}, err
		}
		toolRunID = run.ID
		if run.Status == toolrun.StatusDenied {
			text = fmt.Sprintf("Tool run %s denied by policy: %s", run.ID, run.PolicyReason)
			break
		}
		if run.Status == toolrun.StatusCompleted {
			text = fmt.Sprintf("Tool run %s authorized by an active Session grant and completed as a dry run: %s", run.ID, requested)
			break
		}
		text = fmt.Sprintf("Tool run %s proposed: %s. Review with `cyberagent tool show %s`, approve with `cyberagent tool approve %s`, or deny with `cyberagent tool deny %s`.", run.ID, requested, run.ID, run.ID, run.ID)
	default:
		text = "Unknown slash command. Try /help."
	}

	reply, err := m.store.SaveSessionMessage(ctx, NewMessage(sess.ID, "assistant", text))
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{
		Session:      sess,
		UserMessage:  userMsg,
		ReplyMessage: reply,
		Text:         text,
		Command:      true,
		Compacted:    compacted,
		SummaryID:    summaryID,
		FileEditID:   fileEditID,
		ToolRunID:    toolRunID,
	}, nil
}

func (m *Manager) handleWorkspaceList(ctx context.Context, sess Session, path string) string {
	workspace, ok, err := m.workspaceInfo(ctx, sess)
	if err != nil {
		return "Workspace lookup failed: " + err.Error()
	}
	if !ok {
		return "No workspace is attached to this session."
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	outcome, err := m.gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ListWorkspaceTool, SessionID: sess.ID, WorkspaceID: workspace.ID,
		WorkspaceRoot: workspace.RootPath, RequestedBy: "session_slash_command",
		Arguments: map[string]string{
			"path":      path,
			"max_depth": "2",
		},
	})
	if err != nil {
		if outcome.Result != nil && strings.TrimSpace(outcome.Result.Stderr) != "" {
			return "Workspace list failed: " + outcome.Result.Stderr
		}
		return "Workspace list failed: " + err.Error()
	}
	if !outcome.Decision.Allowed {
		return "Workspace list denied by policy: " + outcome.Decision.Reason
	}
	if outcome.Result == nil {
		return "Workspace list failed: tool gateway returned no result"
	}
	return fmt.Sprintf("Workspace list %s:\n%s", path, outcome.Result.Stdout)
}

func (m *Manager) handleWorkspaceRead(ctx context.Context, sess Session, path string) string {
	workspace, ok, err := m.workspaceInfo(ctx, sess)
	if err != nil {
		return "Workspace lookup failed: " + err.Error()
	}
	if !ok {
		return "No workspace is attached to this session."
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "Usage: /read <workspace-relative-path>"
	}
	outcome, err := m.gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ReadFileTool, SessionID: sess.ID, WorkspaceID: workspace.ID,
		WorkspaceRoot: workspace.RootPath, RequestedBy: "session_slash_command",
		Arguments: map[string]string{
			"path":      path,
			"max_bytes": "16384",
		},
	})
	if err != nil {
		if outcome.Result != nil && strings.TrimSpace(outcome.Result.Stderr) != "" {
			return "Workspace read failed: " + outcome.Result.Stderr
		}
		return "Workspace read failed: " + err.Error()
	}
	if !outcome.Decision.Allowed {
		return "Workspace read denied by policy: " + outcome.Decision.Reason
	}
	if outcome.Result == nil {
		return "Workspace read failed: tool gateway returned no result"
	}
	return fmt.Sprintf("Workspace file %s:\n%s", path, outcome.Result.Stdout)
}

func (m *Manager) handleWorkspaceWrite(ctx context.Context, sess Session, path string, content string) (fileedit.Edit, string) {
	workspace, ok, err := m.workspaceInfo(ctx, sess)
	if err != nil {
		return fileedit.Edit{}, "Workspace lookup failed: " + err.Error()
	}
	if !ok {
		return fileedit.Edit{}, "No workspace is attached to this session."
	}
	edit, err := m.fileEdits.Propose(ctx, fileedit.Proposal{
		SessionID:     sess.ID,
		WorkspaceID:   workspace.ID,
		WorkspaceRoot: workspace.RootPath,
		Path:          path,
		ProposedText:  content,
	})
	if err != nil {
		return fileedit.Edit{}, "File edit proposal failed: " + err.Error()
	}
	if edit.Status == fileedit.StatusApplied {
		response := fmt.Sprintf("File edit %s for %s was authorized by an active Session grant and applied.", edit.ID, edit.Path)
		if edit.SecretsRedacted {
			response += " Sensitive values were redacted before the edit was stored."
		}
		return edit, response
	}
	response := fmt.Sprintf("File edit %s proposed for %s. Review with `cyberagent edit show %s`, approve with `cyberagent edit approve %s`, or deny with `cyberagent edit deny %s`.\n\n%s",
		edit.ID, edit.Path, edit.ID, edit.ID, edit.ID, edit.Diff)
	if edit.SecretsRedacted {
		response += "\nSensitive values were redacted before the proposal was stored."
	}
	return edit, response
}

func parseWriteCommand(input string) (string, string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/write"))
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), parts[1], true
}

func (m *Manager) workspaceInfo(ctx context.Context, sess Session) (WorkspaceInfo, bool, error) {
	if strings.TrimSpace(sess.WorkspaceID) == "" {
		return WorkspaceInfo{}, false, nil
	}
	workspace, err := m.store.GetWorkspaceInfo(ctx, sess.WorkspaceID)
	if err != nil {
		return WorkspaceInfo{}, false, err
	}
	return workspace, true, nil
}

func (m *Manager) chat(ctx context.Context, sess Session, userMsg Message) (SendResult, error) {
	active, err := m.store.ListSessionMessages(ctx, sess.ID, false)
	if err != nil {
		return SendResult{}, err
	}
	summary, _, err := m.store.LatestContextSummary(ctx, sess.ID)
	if err != nil {
		return SendResult{}, err
	}
	prompt := m.contextMgr.BuildPrompt(systemPrompt(sess), summary, toContextMessages(active))
	req := llm.ChatRequest{
		Messages: toLLMMessages(prompt),
		Metadata: map[string]string{
			"session_id":   sess.ID,
			"workspace_id": sess.WorkspaceID,
		},
	}
	resp, err := m.chatRoute(ctx, sess.Route, req)
	if err != nil {
		return SendResult{}, err
	}
	decision := m.checker.CheckText("assistant_response", resp.Text)
	if recorder, ok := m.store.(policy.DecisionRecorder); ok {
		if err := recorder.RecordPolicyDecision(ctx, policy.DecisionRecord{
			SessionID: sess.ID,
			SubjectID: sess.ID,
			Context:   "assistant_response",
			Decision:  decision,
		}); err != nil {
			return SendResult{}, err
		}
	}
	if !decision.Allowed {
		return SendResult{}, fmt.Errorf("policy denied assistant response: %s", decision.Reason)
	}
	reply, err := m.store.SaveSessionMessage(ctx, NewMessage(sess.ID, "assistant", resp.Text))
	if err != nil {
		return SendResult{}, err
	}

	compacted, summaryID, err := m.compactAfterTurn(ctx, sess)
	if err != nil {
		return SendResult{}, err
	}

	return SendResult{
		Session:      sess,
		UserMessage:  userMsg,
		ReplyMessage: reply,
		Text:         resp.Text,
		Compacted:    compacted,
		SummaryID:    summaryID,
	}, nil
}

func (m *Manager) compactAfterTurn(ctx context.Context, sess Session) (bool, int64, error) {
	active, err := m.store.ListSessionMessages(ctx, sess.ID, false)
	if err != nil {
		return false, 0, err
	}
	result, err := m.contextMgr.MaybeCompact(ctx, sess.ID, sess.WorkspaceID, toContextMessages(active))
	if err != nil {
		return false, 0, err
	}
	if !result.Compacted || result.RemovedMessages <= 0 {
		return result.Compacted, 0, nil
	}
	through := active[result.RemovedMessages-1].ID
	if _, err := m.store.MarkSessionMessagesCompacted(ctx, sess.ID, through); err != nil {
		return false, 0, err
	}
	return true, result.Summary.ID, nil
}

func (m *Manager) compactActiveMessages(ctx context.Context, sess Session) (contextmgr.Result, error) {
	active, err := m.store.ListSessionMessages(ctx, sess.ID, false)
	if err != nil {
		return contextmgr.Result{}, err
	}
	result, err := m.contextMgr.Compact(ctx, sess.ID, sess.WorkspaceID, toContextMessages(active))
	if err != nil {
		return contextmgr.Result{}, err
	}
	if result.RemovedMessages > 0 {
		through := active[result.RemovedMessages-1].ID
		if _, err := m.store.MarkSessionMessagesCompacted(ctx, sess.ID, through); err != nil {
			return contextmgr.Result{}, err
		}
	}
	return result, nil
}

func (m *Manager) chatRoute(ctx context.Context, route string, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if strings.Contains(route, "/") {
		ref, err := llm.ParseModelRef(route)
		if err != nil {
			return nil, err
		}
		return m.router.ChatModelRef(ctx, ref, req)
	}
	return m.router.Chat(ctx, route, req)
}

func (m *Manager) resolveModelRef(route string) (llm.ModelRef, error) {
	if strings.Contains(route, "/") {
		return llm.ParseModelRef(route)
	}
	return m.router.Resolve(route), nil
}

func NewMessage(sessionID string, role string, content string) Message {
	content = redact.String(content)
	return Message{
		SessionID:     sessionID,
		Role:          normalizeRole(role),
		Content:       content,
		TokenEstimate: contextmgr.EstimateTokens(content),
		CreatedAt:     time.Now().UTC(),
	}
}

func systemPrompt(sess Session) string {
	return "You are CyberAgent Workbench, a local-first coding agent. Prefer safe, scoped, auditable actions. CTF-specific solving is deferred unless explicitly requested."
}

func toContextMessages(messages []Message) []contextmgr.Message {
	out := make([]contextmgr.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, contextmgr.Message{Role: msg.Role, Content: msg.Content, CreatedAt: msg.CreatedAt})
	}
	return out
}

func toLLMMessages(messages []contextmgr.Message) []llm.Message {
	out := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	return out
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system", "assistant", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "user"
	}
}

func newID(prefix string) string {
	return idgen.New(prefix)
}
