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
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/tools"
	"cyberagent-workbench/internal/workspace"
)

const Version = "v0.1.0"
const defaultMimoBaseURL = "https://token-plan-cn.xiaomimimo.com/anthropic"

type App struct {
	home    string
	out     io.Writer
	errOut  io.Writer
	store   *store.SQLiteStore
	router  *llm.Router
	checker policy.Checker
	kernel  *agent.Kernel
}

func Execute(args []string, out io.Writer, errOut io.Writer) int {
	app := &App{
		home:    DefaultHome(),
		out:     out,
		errOut:  errOut,
		router:  llm.NewDefaultRouter(),
		checker: policy.NewDefaultChecker(),
	}
	app.registerEnvProviders()
	defer app.Close()

	if err := app.dispatch(context.Background(), args); err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	return 0
}

func (a *App) registerEnvProviders() {
	mimoKey := strings.TrimSpace(os.Getenv("MIMO_API_KEY"))
	if mimoKey != "" {
		baseURL := strings.TrimSpace(os.Getenv("MIMO_BASE_URL"))
		if baseURL == "" {
			baseURL = defaultMimoBaseURL
		}
		model := strings.TrimSpace(os.Getenv("MIMO_MODEL"))
		if model == "" {
			model = "mimo-v2.5-pro"
		}
		provider, err := llm.NewAnthropicCompatibleProvider(llm.AnthropicCompatibleConfig{
			Name:         "mimo",
			BaseURL:      baseURL,
			APIKey:       mimoKey,
			DefaultModel: model,
		})
		if err == nil {
			a.router.RegisterProvider(provider)
		}
	}

	anthropicKey := strings.TrimSpace(os.Getenv("CYBERAGENT_ANTHROPIC_API_KEY"))
	if anthropicKey != "" {
		baseURL := strings.TrimSpace(os.Getenv("CYBERAGENT_ANTHROPIC_BASE_URL"))
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		provider, err := llm.NewAnthropicCompatibleProvider(llm.AnthropicCompatibleConfig{
			Name:         "anthropic",
			BaseURL:      baseURL,
			APIKey:       anthropicKey,
			DefaultModel: strings.TrimSpace(os.Getenv("CYBERAGENT_ANTHROPIC_MODEL")),
		})
		if err == nil {
			a.router.RegisterProvider(provider)
		}
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
	case "context":
		return a.contextCommand(ctx, args[1:])
	case "session":
		return a.sessionCommand(ctx, args[1:])
	case "tool":
		return a.toolCommand(ctx, args[1:])
	case "edit":
		return a.editCommand(ctx, args[1:])
	case "run":
		return a.runCommand(ctx, args[1:])
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
	fmt.Fprintln(a.out, "  cyberagent context compact|show")
	fmt.Fprintln(a.out, "  cyberagent session create|list|send|history")
	fmt.Fprintln(a.out, "  cyberagent tool list|show|approve|deny")
	fmt.Fprintln(a.out, "  cyberagent edit propose|list|show|approve|deny")
	fmt.Fprintln(a.out, "  cyberagent run create|list|show|events|start|pause|resume|cancel")
	fmt.Fprintln(a.out, "  cyberagent tui")
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
	tool := tools.NewListWorkspaceTool(rec.RootPath)
	result, err := tool.Run(ctx, tools.Call{
		Name: "list_workspace",
		Args: map[string]string{
			"path":      path,
			"max_depth": strconv.Itoa(*depth),
		},
	})
	if result.Stdout != "" {
		fmt.Fprintln(a.out, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(a.errOut, result.Stderr)
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
	tool := tools.NewReadFileTool(rec.RootPath)
	result, err := tool.Run(ctx, tools.Call{
		Name: "read_file",
		Args: map[string]string{
			"path":      fs.Arg(1),
			"max_bytes": strconv.Itoa(*maxBytes),
		},
	})
	if result.Stdout != "" {
		fmt.Fprintln(a.out, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(a.errOut, result.Stderr)
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
	fmt.Fprintf(a.out, "script task %s completed\nworkspace: %s\nscript: %s\n", task.ID, rec.RootPath, scriptPath)
	return nil
}

func (a *App) scriptRun(ctx context.Context, args []string) error {
	if err := a.ensureStore(); err != nil {
		return err
	}
	fs := newFlagSet("script run", a.errOut)
	local := fs.Bool("local", false, "execute locally instead of dry run")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"local": false})); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("usage: cyberagent script run <path> [--local] [args...]")
	}
	scriptPath := fs.Arg(0)
	cmd, cmdArgs := commandForScript(scriptPath, fs.Args()[1:])
	decision := a.checker.CheckToolCall(tools.Call{
		Name: "sandbox.run",
		Args: map[string]string{"command": strings.Join(append([]string{cmd}, cmdArgs...), " ")},
	})
	if !decision.Allowed {
		return fmt.Errorf("policy denied script run: %s", decision.Reason)
	}
	var runner sandbox.Runner = sandbox.NewNoopRunner()
	if *local {
		runner = sandbox.NewLocalRunner()
	}
	result, err := runner.Run(ctx, sandbox.RunRequest{Command: cmd, Args: cmdArgs, Timeout: 30 * time.Second})
	if result.Stdout != "" {
		fmt.Fprintln(a.out, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(a.errOut, result.Stderr)
	}
	return err
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
