package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"cyberagent-workbench/internal/tools"
)

const ProtectedDeleteReason = "permanent denial: raw command may delete a protected, recursive, or unresolved filesystem target"

var (
	dynamicPathPattern = regexp.MustCompile(`(?i)(?:\$\{?[a-z_][a-z0-9_]*\}?|\$env:[a-z_][a-z0-9_]*|%[a-z_][a-z0-9_]*%|\$\()`)
	parentPathPattern  = regexp.MustCompile(`(?:^|[\s"'=\\/])\.\.(?:[\\/]|$)`)
)

type protectedDeleteGuard struct {
	homeMarkers []string
}

type executableIntent struct {
	Executable  string             `json:"executable"`
	Arguments   []string           `json:"arguments"`
	Command     *executableCommand `json:"command"`
	Environment []environmentValue `json:"environment"`
}

type executableCommand struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments"`
}

type environmentValue struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Value  string `json:"value"`
}

func newProtectedDeleteGuard() protectedDeleteGuard {
	guard := protectedDeleteGuard{}
	if home, err := os.UserHomeDir(); err == nil {
		home = normalizePathMarker(home)
		if home != "" && home != "/" {
			guard.homeMarkers = append(guard.homeMarkers, home)
		}
	}
	return guard
}

func protectedDeleteDecision() Decision {
	return Decision{Allowed: false, Reason: ProtectedDeleteReason, Risk: "critical"}
}

func isShellExecutionContext(context string) bool {
	return strings.EqualFold(strings.TrimSpace(context), "tool_run.shell")
}

func (g protectedDeleteGuard) BlocksToolCall(call tools.Call) bool {
	toolName := strings.ToLower(strings.TrimSpace(call.Name))
	if !isExecutableToolName(toolName) {
		return false
	}

	if toolName == "sandbox.manifest" {
		if intent, ok := decodeExecutableIntent(call.Args["intent"]); ok {
			return g.blocksExecutableIntent(intent)
		}
	}
	if strings.Contains(toolName, "script") || strings.Contains(toolName, "process") {
		if intent, ok := decodeExecutableIntent(call.Args["proposal"]); ok {
			return g.blocksExecutableIntent(intent)
		}
	}

	keys := make([]string, 0, len(call.Args))
	for key := range call.Args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, call.Args[key])
	}
	return g.BlocksRawCommand(strings.Join(parts, " "))
}

func (g protectedDeleteGuard) blocksExecutableIntent(intent executableIntent) bool {
	executable := intent.Executable
	arguments := intent.Arguments
	if intent.Command != nil {
		executable = intent.Command.Executable
		arguments = intent.Command.Arguments
	}
	if !mayInterpretDeletion(executable, arguments) {
		return false
	}
	parts := append([]string{executable}, arguments...)
	for _, binding := range intent.Environment {
		if strings.EqualFold(strings.TrimSpace(binding.Source), "literal") {
			parts = append(parts, binding.Name+"="+binding.Value)
		}
	}
	return g.BlocksRawCommand(strings.Join(parts, " "))
}

func decodeExecutableIntent(payload string) (executableIntent, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return executableIntent{}, false
	}
	var intent executableIntent
	if err := json.Unmarshal([]byte(payload), &intent); err != nil {
		return executableIntent{}, false
	}
	if intent.Command != nil {
		return intent, strings.TrimSpace(intent.Command.Executable) != ""
	}
	return intent, strings.TrimSpace(intent.Executable) != ""
}

func isExecutableToolName(name string) bool {
	return strings.Contains(name, "shell") || strings.Contains(name, "sandbox") ||
		strings.Contains(name, "process") || strings.Contains(name, "script")
}

func mayInterpretDeletion(executable string, arguments []string) bool {
	base := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(executable, `\`, "/")))
	base = filepath.Base(base)
	switch base {
	case "echo", "echo.exe", "printf", "printf.exe":
		return false
	case "rm", "rm.exe", "unlink", "unlink.exe", "rmdir", "rmdir.exe", "del", "del.exe", "erase", "erase.exe",
		"find", "find.exe", "sh", "bash", "dash", "zsh", "fish", "cmd", "cmd.exe", "powershell", "powershell.exe",
		"pwsh", "pwsh.exe", "python", "python.exe", "python3", "python3.exe", "node", "node.exe", "deno", "deno.exe",
		"bun", "bun.exe", "perl", "perl.exe", "ruby", "ruby.exe", "busybox", "env", "env.exe", "xargs", "xargs.exe":
		return true
	}
	for _, argument := range arguments {
		if containsDeletionPrimitive(argument) {
			return true
		}
	}
	return false
}

func (g protectedDeleteGuard) BlocksRawCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || !containsDeletionPrimitive(command) {
		return false
	}
	if containsRecursiveDeletion(command) {
		return true
	}
	return g.containsUnsafeTarget(command)
}

func containsDeletionPrimitive(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{
		"shutil.rmtree", "os.removeall", "os.remove(", "os.unlink(", "filesystem.remove_all",
		"fs.rmsync(", "fs.rm(", "fs.rmdirsync(", "fs.rmdir(", ".rmsync(", ".rm(",
		".rmdirsync(", ".rmdir(", "directory.delete(",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, token := range commandTokens(lower) {
		switch token {
		case "rm", "rm.exe", "unlink", "unlink.exe", "rmdir", "rmdir.exe", "rd", "del", "del.exe", "erase", "erase.exe",
			"remove-item", "ri":
			return true
		case "-delete":
			return true
		}
	}
	return false
}

func containsRecursiveDeletion(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{
		"shutil.rmtree", "os.removeall", "filesystem.remove_all", "fs.rmsync(", "fs.rm(",
		"fs.rmdirsync(", "fs.rmdir(", ".rmsync(", ".rm(", ".rmdirsync(", ".rmdir(",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if strings.Contains(lower, "directory.delete(") && strings.Contains(lower, "true") {
		return true
	}
	tokens := commandTokens(lower)
	for index, token := range tokens {
		switch token {
		case "rm", "rm.exe", "remove-item", "ri":
			if hasRecursiveFlag(tokens[index+1:]) {
				return true
			}
		case "rmdir", "rmdir.exe", "rd", "del", "del.exe", "erase", "erase.exe":
			if hasWindowsRecursiveFlag(tokens[index+1:]) {
				return true
			}
		case "-delete":
			return true
		}
	}
	return false
}

func hasRecursiveFlag(tokens []string) bool {
	for _, token := range tokens {
		if token == "--recursive" || token == "-recurse" {
			return true
		}
		if !strings.HasPrefix(token, "-") || strings.HasPrefix(token, "--") {
			continue
		}
		flag := strings.TrimPrefix(token, "-")
		if flag == "r" || (len(flag) > 1 && strings.HasPrefix("recurse", flag)) ||
			(len(flag) <= 4 && strings.Contains(flag, "r") && onlyCompactRMFlags(flag)) {
			return true
		}
	}
	return false
}

func onlyCompactRMFlags(flag string) bool {
	for _, value := range flag {
		if !strings.ContainsRune("rfivd", value) {
			return false
		}
	}
	return true
}

func hasWindowsRecursiveFlag(tokens []string) bool {
	for _, token := range tokens {
		if token == "/s" || token == "-s" || token == "-recurse" {
			return true
		}
	}
	return false
}

func (g protectedDeleteGuard) containsUnsafeTarget(command string) bool {
	lower := strings.ToLower(command)
	normalized := normalizePathMarker(lower)
	for _, marker := range g.homeMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	if dynamicPathPattern.MatchString(lower) || strings.ContainsRune(lower, '`') || parentPathPattern.MatchString(normalized) ||
		strings.ContainsAny(lower, "*?") {
		return true
	}
	for _, token := range commandTokens(normalized) {
		token = strings.TrimSpace(token)
		if token == "~" || strings.HasPrefix(token, "~/") || strings.HasPrefix(token, "//") {
			return true
		}
		if len(token) >= 3 && ((token[0] >= 'a' && token[0] <= 'z') || (token[0] >= 'A' && token[0] <= 'Z')) &&
			token[1] == ':' && token[2] == '/' {
			return true
		}
		if strings.HasPrefix(token, "/") && !isWindowsCommandSwitch(token) {
			return true
		}
	}
	return false
}

func commandTokens(command string) []string {
	return strings.FieldsFunc(command, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`"'(),;|&=[]{}`, r)
	})
}

func normalizePathMarker(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, `\`, "/")
	return strings.TrimRight(value, "/")
}

func isWindowsCommandSwitch(token string) bool {
	switch strings.ToLower(token) {
	case "/s", "/q", "/f", "/a", "/c", "/d", "/v", "/x", "/y":
		return true
	default:
		return false
	}
}
