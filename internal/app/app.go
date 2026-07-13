package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cyberagent-workbench/internal/agent"
	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/tools"
	"cyberagent-workbench/internal/workspace"
)

const Version = "v0.1.0"

const (
	defaultMimoBaseURL     = "https://token-plan-cn.xiaomimimo.com/anthropic"
	defaultMimoModel       = "mimo-v2.5-pro"
	defaultDeepSeekBaseURL = "https://api.deepseek.com/anthropic"
	defaultDeepSeekModel   = "deepseek-v4-flash"
	defaultAnthropicURL    = "https://api.anthropic.com"
)

type envAnthropicProviderConfig struct {
	name           string
	apiKeyEnv      string
	baseURLEnv     string
	modelEnv       string
	defaultBaseURL string
	defaultModel   string
}

type App struct {
	home    string
	out     io.Writer
	errOut  io.Writer
	store   *store.SQLiteStore
	router  *llm.Router
	checker policy.Checker
	kernel  *agent.Kernel
	calls   *application.ActiveCallRegistry
}

func Execute(args []string, out io.Writer, errOut io.Writer) int {
	return ExecuteContext(context.Background(), args, out, errOut)
}

func ExecuteContext(ctx context.Context, args []string, out io.Writer, errOut io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	app := &App{
		home:    DefaultHome(),
		out:     out,
		errOut:  errOut,
		router:  llm.NewDefaultRouter(),
		checker: policy.NewDefaultChecker(),
		calls:   application.NewActiveCallRegistry(),
	}
	app.registerEnvProviders()
	defer app.Close()

	if err := app.dispatch(ctx, args); err != nil {
		classified := apperror.Normalize(err)
		fmt.Fprintln(errOut, "error:", classified)
		return apperror.ExitCode(classified)
	}
	return 0
}

func (a *App) newRunSupervisor() *application.RunSupervisor {
	return application.NewRunSupervisor(a.store, a.router, a.checker).WithActiveCalls(a.calls)
}

func (a *App) newToolGateway() *toolgateway.Gateway {
	return toolgateway.New(a.store, a.checker).
		WithStructuredMemoryExecutor(application.NewStructuredMemoryToolExecutor(a.store)).
		WithSpecialistDelegationExecutor(application.NewSpecialistDelegationToolExecutor(a.store)).
		WithPlanDeliveryExecutor(application.NewPlanDeliveryToolExecutor(a.store)).
		WithWorkspaceRootResolver(func(ctx context.Context, workspaceID string) (string, error) {
			rec, err := a.store.GetWorkspaceByID(ctx, workspaceID)
			return rec.RootPath, err
		})
}

func (a *App) registerEnvProviders() {
	configs := []envAnthropicProviderConfig{
		{
			name: "mimo", apiKeyEnv: "MIMO_API_KEY", baseURLEnv: "MIMO_BASE_URL", modelEnv: "MIMO_MODEL",
			defaultBaseURL: defaultMimoBaseURL, defaultModel: defaultMimoModel,
		},
		{
			name: "deepseek", apiKeyEnv: "DEEPSEEK_API_KEY", baseURLEnv: "DEEPSEEK_BASE_URL", modelEnv: "DEEPSEEK_MODEL",
			defaultBaseURL: defaultDeepSeekBaseURL, defaultModel: defaultDeepSeekModel,
		},
		{
			name: "anthropic", apiKeyEnv: "CYBERAGENT_ANTHROPIC_API_KEY", baseURLEnv: "CYBERAGENT_ANTHROPIC_BASE_URL",
			modelEnv: "CYBERAGENT_ANTHROPIC_MODEL", defaultBaseURL: defaultAnthropicURL,
		},
	}
	for _, config := range configs {
		a.registerEnvAnthropicProvider(config)
	}
}

func (a *App) registerEnvAnthropicProvider(config envAnthropicProviderConfig) {
	apiKey := strings.TrimSpace(os.Getenv(config.apiKeyEnv))
	if apiKey == "" {
		return
	}
	baseURL := strings.TrimSpace(os.Getenv(config.baseURLEnv))
	if baseURL == "" {
		baseURL = config.defaultBaseURL
	}
	model := strings.TrimSpace(os.Getenv(config.modelEnv))
	if model == "" {
		model = config.defaultModel
	}
	provider, err := llm.NewAnthropicCompatibleProvider(llm.AnthropicCompatibleConfig{
		Name: config.name, BaseURL: baseURL, APIKey: apiKey, DefaultModel: model,
	})
	if err == nil {
		a.router.RegisterProvider(provider)
	}
}

func DefaultHome() string {
	if home := strings.TrimSpace(os.Getenv("CYBERAGENT_HOME")); home != "" {
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return ".cyberagent-workbench"
	}
	return filepath.Join(userHome, ".cyberagent-workbench")
}

func (a *App) Close() {
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "version":
		fmt.Fprintf(a.out, "CyberAgent Workbench cyberagent %s\n", Version)
		return nil
	case "workspace":
		return a.workspaceCommand(ctx, args[1:])
	case "script":
		return a.scriptCommand(ctx, args[1:])
	case "ctf":
		return a.ctfCommand(ctx, args[1:])
	case "learn":
		return a.learnCommand(ctx, args[1:])
	case "provider":
		return a.providerCommand(ctx, args[1:])
	case "model":
		return a.modelCommand(ctx, args[1:])
	case "skill":
		return a.skillCommand(ctx, args[1:])
	case "context":
		return a.contextCommand(ctx, args[1:])
	case "session":
		return a.sessionCommand(ctx, args[1:])
	case "tool":
		return a.toolCommand(ctx, args[1:])
	case "edit":
		return a.editCommand(ctx, args[1:])
	case "approval":
		return a.approvalCommand(ctx, args[1:])
	case "artifact":
		return a.artifactCommand(ctx, args[1:])
	case "report":
		return a.reportCommand(ctx, args[1:])
	case "api":
		return a.apiCommand(ctx, args[1:])
	case "headless":
		return a.headlessCommand(ctx, args[1:])
	case "run":
		return a.runCommand(ctx, args[1:])
	case "todo":
		return a.todoCommand(ctx, args[1:])
	case "note":
		return a.noteCommand(ctx, args[1:])
	case "tui":
		return a.tuiCommand(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (a *App) printHelp() {
	fmt.Fprintln(a.out, "CyberAgent Workbench")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Usage:")
	fmt.Fprintln(a.out, "  cyberagent version")
	fmt.Fprintln(a.out, "  cyberagent workspace init|list|show|tree|read")
	fmt.Fprintln(a.out, "  cyberagent script new|run")
	fmt.Fprintln(a.out, "  cyberagent ctf init|analyze|writeup")
	fmt.Fprintln(a.out, "  cyberagent learn ask")
	fmt.Fprintln(a.out, "  cyberagent provider list|test")
	fmt.Fprintln(a.out, "  cyberagent model list|set")
	fmt.Fprintln(a.out, "  cyberagent skill list|show|validate|select|selection")
	fmt.Fprintln(a.out, "  cyberagent context compact|show")
	fmt.Fprintln(a.out, "  cyberagent session create|list|send|history")
	fmt.Fprintln(a.out, "  cyberagent tool schema|invoke|list|show|approve|deny")
	fmt.Fprintln(a.out, "  cyberagent edit propose|list|show|approve|deny")
	fmt.Fprintln(a.out, "  cyberagent approval list|show|grant")
	fmt.Fprintln(a.out, "  cyberagent artifact list|show|read|verify")
	fmt.Fprintln(a.out, "  cyberagent report show|finding|check")
	fmt.Fprintln(a.out, "  cyberagent report finding attach|validate|reject|accept|remediation|fix|verify")
	fmt.Fprintln(a.out, "  cyberagent api serve|openapi")
	fmt.Fprintln(a.out, "  cyberagent headless events")
	fmt.Fprintln(a.out, "  cyberagent run create|adapt-task|list|show|mode|phase|events|usage|start|step|execute|checkpoint|graph|lease|finish|fail|pause|resume|cancel|delegations|delegation|plans|plan|fanouts|fanout")
	fmt.Fprintln(a.out, "  cyberagent run plan show|choose|selection")
	fmt.Fprintln(a.out, "  cyberagent run fanout plan|execute|show|execution|report")
	fmt.Fprintln(a.out, "  cyberagent todo create|list|show|update|start|block|reopen|complete|cancel")
	fmt.Fprintln(a.out, "  cyberagent note create|list|show|update|archive|restore")
	fmt.Fprintln(a.out, "  cyberagent tui [--run <run-id> | --session <session-id>] [--print]")
}

func (a *App) ensureStore() error {
	if a.store != nil {
		return nil
	}
	dbPath := filepath.Join(a.home, "cyberagent.db")
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	a.store = st
	a.loadRouteSettings(context.Background())
	a.kernel = agent.NewKernel(st, a.router, a.checker)
	return nil
}

func (a *App) loadRouteSettings(ctx context.Context) {
	for _, route := range []string{"ctf", "script", "learn", "code", "review"} {
		value, ok, err := a.store.GetProviderSetting(ctx, "route."+route)
		if err != nil || !ok {
			continue
		}
		ref, err := llm.ParseModelRef(value)
		if err == nil {
			a.router.SetRoute(route, ref)
		}
	}
}

func (a *App) workspaceManager() (*workspace.Manager, error) {
	if err := a.ensureStore(); err != nil {
		return nil, err
	}
	return workspace.NewManager(a.home, a.store), nil
}

func (a *App) workspaceCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("workspace subcommand is required")
	}
	mgr, err := a.workspaceManager()
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		fs := newFlagSet("workspace init", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent workspace init <name>")
		}
		rec, err := mgr.Init(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "workspace %s initialized at %s\n", rec.Name, rec.RootPath)
		return nil
	case "list":
		recs, err := a.store.ListWorkspaces(ctx)
		if err != nil {
			return err
		}
		if len(recs) == 0 {
			fmt.Fprintln(a.out, "no workspaces")
			return nil
		}
		for _, rec := range recs {
			fmt.Fprintf(a.out, "%s\t%s\n", rec.Name, rec.RootPath)
		}
		return nil
	case "show":
		fs := newFlagSet("workspace show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent workspace show <name>")
		}
		rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(fs.Arg(0)))
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "name: %s\nid: %s\npath: %s\ncreated_at: %s\n", rec.Name, rec.ID, rec.RootPath, rec.CreatedAt.Format(time.RFC3339))
		return nil
	case "tree":
		return a.workspaceTree(ctx, args[1:])
	case "read":
		return a.workspaceRead(ctx, args[1:])
	default:
		return fmt.Errorf("unknown workspace subcommand %q", args[0])
	}
}

func (a *App) workspaceTree(ctx context.Context, args []string) error {
	fs := newFlagSet("workspace tree", a.errOut)
	depth := fs.Int("depth", 2, "maximum recursive depth")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"depth": true})); err != nil {
		return err
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		return errors.New("usage: cyberagent workspace tree <workspace> [path] [--depth <n>]")
	}
	rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(fs.Arg(0)))
	if err != nil {
		return err
	}
	path := "."
	if fs.NArg() == 2 {
		path = fs.Arg(1)
	}
	outcome, err := a.newToolGateway().Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ListWorkspaceTool, WorkspaceID: rec.ID, WorkspaceRoot: rec.RootPath, RequestedBy: "cli",
		Arguments: map[string]string{
			"path":      path,
			"max_depth": strconv.Itoa(*depth),
		},
	})
	if outcome.Result != nil && outcome.Result.Stdout != "" {
		fmt.Fprintln(a.out, outcome.Result.Stdout)
	}
	if outcome.Result != nil && outcome.Result.Stderr != "" {
		fmt.Fprintln(a.errOut, outcome.Result.Stderr)
	}
	if err == nil && !outcome.Decision.Allowed {
		return apperror.New(apperror.CodePolicyDenied, "policy denied workspace list: "+outcome.Decision.Reason)
	}
	return err
}

func (a *App) workspaceRead(ctx context.Context, args []string) error {
	fs := newFlagSet("workspace read", a.errOut)
	maxBytes := fs.Int("max-bytes", tools.DefaultMaxReadBytes, "maximum bytes to print")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"max-bytes": true})); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: cyberagent workspace read <workspace> <path> [--max-bytes <n>]")
	}
	rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(fs.Arg(0)))
	if err != nil {
		return err
	}
	outcome, err := a.newToolGateway().Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ReadFileTool, WorkspaceID: rec.ID, WorkspaceRoot: rec.RootPath, RequestedBy: "cli",
		Arguments: map[string]string{
			"path":      fs.Arg(1),
			"max_bytes": strconv.Itoa(*maxBytes),
		},
	})
	if outcome.Result != nil && outcome.Result.Stdout != "" {
		fmt.Fprintln(a.out, outcome.Result.Stdout)
	}
	if outcome.Result != nil && outcome.Result.Stderr != "" {
		fmt.Fprintln(a.errOut, outcome.Result.Stderr)
	}
	if err == nil && !outcome.Decision.Allowed {
		return apperror.New(apperror.CodePolicyDenied, "policy denied workspace read: "+outcome.Decision.Reason)
	}
	return err
}

func (a *App) scriptCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("script subcommand is required")
	}
	switch args[0] {
	case "new":
		return a.scriptNew(ctx, args[1:])
	case "run":
		return a.scriptRun(ctx, args[1:])
	default:
		return fmt.Errorf("unknown script subcommand %q", args[0])
	}
}

func (a *App) scriptNew(ctx context.Context, args []string) error {
	mgr, err := a.workspaceManager()
	if err != nil {
		return err
	}
	fs := newFlagSet("script new", a.errOut)
	workspaceName := fs.String("workspace", "", "workspace name")
	language := fs.String("language", "python", "script language")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"workspace": true, "language": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New(`usage: cyberagent script new "goal" --workspace <name>`)
	}
	goal := fs.Arg(0)
	wsName := *workspaceName
	if wsName == "" {
		wsName = goal
	}
	rec, err := mgr.Ensure(ctx, wsName)
	if err != nil {
		return err
	}
	task := agent.Task{
		ID:          agent.NewID("task"),
		Kind:        agent.TaskScript,
		Goal:        goal,
		WorkspaceID: rec.ID,
		Mode:        *language,
		Status:      agent.StatusPending,
		CreatedAt:   time.Now().UTC(),
	}
	if err := a.store.SaveTask(ctx, task); err != nil {
		return err
	}
	if err := a.kernel.Step(ctx, task.ID); err != nil {
		return err
	}
	scriptPath := mgr.ScriptPath(rec, task, scriptExt(*language))
	if err := os.WriteFile(scriptPath, []byte(scriptTemplate(goal, *language)), 0o644); err != nil {
		return err
	}
	if err := a.store.SaveArtifact(ctx, store.ArtifactRecord{
		ID:          agent.NewID("artifact"),
		WorkspaceID: rec.ID,
		TaskID:      task.ID,
		Path:        scriptPath,
		Kind:        "script",
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}
	relativeScriptPath, err := filepath.Rel(rec.RootPath, scriptPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "script task %s completed\nworkspace: %s\nscript: %s\nscript_relative: %s\n",
		task.ID, rec.RootPath, scriptPath, filepath.ToSlash(relativeScriptPath))
	return nil
}

func (a *App) scriptRun(ctx context.Context, args []string) error {
	if err := a.ensureStore(); err != nil {
		return err
	}
	fs := newFlagSet("script run", a.errOut)
	workspaceName := fs.String("workspace", "", "workspace containing the script")
	local := fs.Bool("local", false, "record a local backend request; execution remains disabled")
	operationKey := fs.String("idempotency-key", "", "stable retry key; generated when omitted")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"workspace": true, "local": false, "idempotency-key": true,
	})); err != nil {
		return err
	}
	if fs.NArg() < 1 || strings.TrimSpace(*workspaceName) == "" {
		return errors.New("usage: cyberagent script run <workspace-relative-path> --workspace <name> [--local] [args...]")
	}
	rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(*workspaceName))
	if err != nil {
		return err
	}
	rawPath := strings.TrimSpace(fs.Arg(0))
	if _, err := tools.NewWorkspaceFS(rec.RootPath).ResolveFileForRead(rawPath); err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return apperror.Wrap(apperror.CodeInvalidArgument, "invalid script path: "+err.Error(), err)
	}
	scriptPath := filepath.ToSlash(filepath.Clean(rawPath))
	cmd, cmdArgs := commandForScript(scriptPath, fs.Args()[1:])
	requestedBackend := "sandbox"
	if *local {
		requestedBackend = "local"
	}
	processProposal := toolgateway.ScriptProcessProposal{
		Executable: cmd, Arguments: cmdArgs, WorkingDirectory: ".", RequestedBackend: requestedBackend,
	}
	if _, err := toolgateway.EncodeScriptProcessProposal(processProposal); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	key := strings.TrimSpace(*operationKey)
	if key == "" {
		key = idgen.New("scriptop")
	}
	gateway := a.newToolGateway()
	result, err := application.NewScriptProcessService(a.store, gateway).Create(ctx, application.CreateScriptProcessRunRequest{
		Run: application.CreateRunRequest{
			Goal: "Review execution of workspace script " + scriptPath, Profile: string(domain.ProfileScript),
			WorkspaceID: rec.ID, Interactive: true, Budget: domain.DefaultBudget(),
		},
		OperationKey: key, RequestedBy: "script_cli", Process: processProposal,
	})
	if err != nil {
		return err
	}
	if result.Outcome.Proposal == nil {
		return errors.New("script run did not create a tool proposal")
	}
	fmt.Fprintf(a.out, "script process proposal %s\nmission: %s\nrun: %s\nsession: %s\nworkspace: %s\nscript: %s\nrequested_backend: %s\nstatus: %s\napproval: %s\nreplayed: %t\nidempotency_key: %s\nexecution: disabled; approval completes as dry run\n",
		result.Process.ID, result.Mission.ID, result.Run.ID, result.Run.SessionID, rec.ID, scriptPath,
		requestedBackend, result.Outcome.Proposal.Status, result.Outcome.Decision.Approval,
		result.Replayed, redact.String(key))
	if !result.Outcome.Decision.Allowed {
		return apperror.New(apperror.CodePolicyDenied, "policy denied script run: "+result.Outcome.Decision.Reason)
	}
	return nil
}

func (a *App) ctfCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("ctf subcommand is required")
	}
	switch args[0] {
	case "init":
		return a.ctfInit(ctx, args[1:])
	case "analyze":
		return a.ctfAnalyze(ctx, args[1:])
	case "writeup":
		return a.ctfWriteup(ctx, args[1:])
	default:
		return fmt.Errorf("unknown ctf subcommand %q", args[0])
	}
}

func (a *App) ctfInit(ctx context.Context, args []string) error {
	mgr, err := a.workspaceManager()
	if err != nil {
		return err
	}
	fs := newFlagSet("ctf init", a.errOut)
	category := fs.String("category", "misc", "challenge category")
	scope := fs.String("scope", "local", "authorized target scope")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"category": true, "scope": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent ctf init <name> --category <category>")
	}
	rec, err := mgr.Init(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	task := agent.Task{
		ID:          agent.NewID("task"),
		Kind:        agent.TaskCTF,
		Goal:        fmt.Sprintf("Initialize %s challenge in category %s with scope %s", rec.Name, *category, *scope),
		WorkspaceID: rec.ID,
		Mode:        *category,
		Status:      agent.StatusPending,
		CreatedAt:   time.Now().UTC(),
	}
	if err := a.store.SaveTask(ctx, task); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "ctf workspace %s initialized\ncategory: %s\nscope: %s\npath: %s\n", rec.Name, *category, *scope, rec.RootPath)
	return nil
}

func (a *App) ctfAnalyze(ctx context.Context, args []string) error {
	return a.ctfStep(ctx, args, "analyze")
}

func (a *App) ctfWriteup(ctx context.Context, args []string) error {
	mgr, err := a.workspaceManager()
	if err != nil {
		return err
	}
	fs := newFlagSet("ctf writeup", a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cyberagent ctf writeup <workspace>")
	}
	rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(fs.Arg(0)))
	if err != nil {
		return err
	}
	path := mgr.WriteupPath(rec)
	content := fmt.Sprintf("# %s Writeup\n\nStatus: mock writeup scaffold.\n\n- Record observations in attachments/ and logs/.\n- Keep exploit steps scoped to authorized CTF targets.\n", rec.Name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "writeup scaffold: %s\n", path)
	return nil
}

func (a *App) ctfStep(ctx context.Context, args []string, action string) error {
	if err := a.ensureStore(); err != nil {
		return err
	}
	fs := newFlagSet("ctf "+action, a.errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: cyberagent ctf %s <workspace>", action)
	}
	rec, err := a.store.GetWorkspaceByName(ctx, workspace.Slug(fs.Arg(0)))
	if err != nil {
		return err
	}
	task := agent.Task{
		ID:          agent.NewID("task"),
		Kind:        agent.TaskCTF,
		Goal:        action + " CTF workspace " + rec.Name,
		WorkspaceID: rec.ID,
		Mode:        action,
		Status:      agent.StatusPending,
		CreatedAt:   time.Now().UTC(),
	}
	if err := a.store.SaveTask(ctx, task); err != nil {
		return err
	}
	if err := a.kernel.Step(ctx, task.ID); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "ctf %s task %s completed\n", action, task.ID)
	return nil
}

func (a *App) learnCommand(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "ask" {
		return errors.New(`usage: cyberagent learn ask "question"`)
	}
	fs := newFlagSet("learn ask", a.errOut)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New(`usage: cyberagent learn ask "question"`)
	}
	resp, err := a.router.Chat(ctx, "learn", llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: fs.Arg(0)}},
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, resp.Text)
	return nil
}

func (a *App) providerCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("provider subcommand is required")
	}
	switch args[0] {
	case "list":
		for _, name := range a.router.ProviderNames() {
			fmt.Fprintln(a.out, name)
		}
		return nil
	case "test":
		route := "learn"
		if len(args) > 1 {
			route = args[1]
		}
		req := llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "provider health check"}}}
		var resp *llm.ChatResponse
		var err error
		if strings.Contains(route, "/") {
			ref, parseErr := llm.ParseModelRef(route)
			if parseErr != nil {
				return parseErr
			}
			resp, err = a.router.ChatModelRef(ctx, ref, req)
		} else {
			resp, err = a.router.Chat(ctx, route, req)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "provider: %s\nmodel: %s\nresponse: %s\n", resp.Provider, resp.Model, resp.Text)
		return nil
	default:
		return fmt.Errorf("unknown provider subcommand %q", args[0])
	}
}

func (a *App) modelCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("model subcommand is required")
	}
	switch args[0] {
	case "list":
		if err := a.ensureStore(); err != nil {
			return err
		}
		models, err := a.router.ListModels(ctx)
		if err != nil {
			return err
		}
		for _, model := range models {
			fmt.Fprintf(a.out, "%s/%s\t%s\t%s\n", model.Provider, model.ID, model.DisplayName, strings.Join(model.Capabilities, ","))
		}
		routes := a.router.Routes()
		names := make([]string, 0, len(routes))
		for name := range routes {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintln(a.out, "routes:")
		for _, name := range names {
			ref := routes[name]
			fmt.Fprintf(a.out, "%s -> %s/%s\n", name, ref.Provider, ref.Model)
		}
		return nil
	case "set":
		if err := a.ensureStore(); err != nil {
			return err
		}
		fs := newFlagSet("model set", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 2 {
			return errors.New("usage: cyberagent model set <route> <provider/model>")
		}
		ref, err := llm.ParseModelRef(fs.Arg(1))
		if err != nil {
			return err
		}
		a.router.SetRoute(fs.Arg(0), ref)
		if err := a.store.SetProviderSetting(ctx, "route."+fs.Arg(0), fs.Arg(1)); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "route %s set to %s/%s\n", fs.Arg(0), ref.Provider, ref.Model)
		return nil
	default:
		return fmt.Errorf("unknown model subcommand %q", args[0])
	}
}

func newFlagSet(name string, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	return fs
}

func scriptExt(language string) string {
	switch strings.ToLower(language) {
	case "python", "py":
		return ".py"
	case "bash", "sh":
		return ".sh"
	case "go":
		return ".go"
	case "node", "javascript", "js":
		return ".js"
	default:
		return ".txt"
	}
}

func scriptTemplate(goal string, language string) string {
	switch strings.ToLower(language) {
	case "bash", "sh":
		return "#!/usr/bin/env bash\nset -euo pipefail\n\n# Goal: " + sanitizeLine(goal) + "\necho \"mock script scaffold\"\n"
	case "go":
		return "package main\n\nimport \"fmt\"\n\nfunc main() {\n\t// Goal: " + sanitizeLine(goal) + "\n\tfmt.Println(\"mock script scaffold\")\n}\n"
	case "node", "javascript", "js":
		return "// Goal: " + sanitizeLine(goal) + "\nconsole.log(\"mock script scaffold\");\n"
	default:
		return "#!/usr/bin/env python3\n\"\"\"Mock script scaffold generated by CyberAgent Workbench.\"\"\"\n\nGOAL = " + fmt.Sprintf("%q", goal) + "\n\nif __name__ == \"__main__\":\n    print(\"mock script scaffold\")\n    print(f\"goal: {GOAL}\")\n"
	}
}

func sanitizeLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func commandForScript(path string, rest []string) (string, []string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py":
		return "python", append([]string{path}, rest...)
	case ".js":
		return "node", append([]string{path}, rest...)
	case ".sh":
		return "bash", append([]string{path}, rest...)
	default:
		return path, rest
	}
}

func reorderFlags(args []string, takesValue map[string]bool) []string {
	flags := make([]string, 0, len(args))
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			name := strings.TrimLeft(arg, "-")
			if idx := strings.Index(name, "="); idx >= 0 {
				name = name[:idx]
			}
			takes, ok := takesValue[name]
			if !ok {
				rest = append(rest, arg)
				continue
			}
			flags = append(flags, arg)
			if takes && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		rest = append(rest, arg)
	}
	return append(flags, rest...)
}
