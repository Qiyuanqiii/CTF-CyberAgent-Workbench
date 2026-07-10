# Usage

## Missions and Runs

```powershell
cyberagent run create "review this workspace" --workspace demo --profile review
cyberagent run create "explain this code" --profile learn --max-turns 40 --max-tokens 20000 --timeout 20m
cyberagent run list
cyberagent run list --status paused
cyberagent run show <run-id>
cyberagent run events <run-id>
cyberagent run start <run-id>
cyberagent run pause <run-id>
cyberagent run resume <run-id>
cyberagent run cancel <run-id>
```

A Mission is the stable goal and authorization scope. A Run is one resumable execution attempt. Run state changes and their events are committed together in SQLite. `run start` currently advances the auditable lifecycle from `created` through `preparing` to `running`; the model-driven RunSupervisor execution loop is scheduled for P2 and is not started implicitly in this slice.

Supported profiles are `code`, `review`, `learn`, and `script`. New runs start with network access disabled. Budget flags reject negative values and include maximum turns, tokens, model cost, and wall-clock timeout.

## Workspaces

```powershell
cyberagent workspace init demo
cyberagent workspace list
cyberagent workspace show demo
cyberagent workspace tree demo
cyberagent workspace tree demo scripts --depth 2
cyberagent workspace read demo README.md
```

`workspace tree` and `workspace read` only accept paths relative to the selected workspace. Attempts to read outside the workspace, such as `../outside.txt`, are rejected. Text returned by `workspace read` is passed through secret redaction before printing.

## Script Mode

```powershell
cyberagent script new "parse pcap http token" --workspace demo
cyberagent script run C:\path\to\script.py
cyberagent script run C:\path\to\script.py --local
```

`script run` uses a dry-run sandbox by default. Add `--local` only when you want to execute the script on your machine.

## CTF Mode

```powershell
cyberagent ctf init baby-web --category web
cyberagent ctf analyze baby-web
cyberagent ctf writeup baby-web
```

## Model and Provider Commands

```powershell
cyberagent provider list
cyberagent provider test
cyberagent provider test mimo/mimo-v2.5-pro
cyberagent model list
cyberagent model set script mock/mock-code
```

`provider test` accepts either a route name, such as `learn`, or a direct `provider/model` reference. The optional `mimo` provider is registered from `MIMO_API_KEY`, `MIMO_BASE_URL`, and `MIMO_MODEL`.

## TUI

```powershell
cyberagent tui
cyberagent tui --workspace demo --title "Agent basics" --route learn
cyberagent tui --session <session-id>
cyberagent tui --session <session-id> --print
```

Without `--session`, the TUI opens a session picker. Press `Enter` to open the selected session, `n` to create a new one, `j/k` to move, `r` to refresh, and `q` or `Esc` to quit.

The chat TUI uses the same session and tool approval runtime as the CLI. Normal text sends a session message. Slash commands such as `/run echo hello`, `/model script`, and `/compact` go through the session manager. Tool approvals can be handled in the input box:

```text
/approve <tool-run-id>
/deny <tool-run-id> not needed
```

Keyboard controls:

```text
Tab              switch focus between input and tool runs
Enter            send from input or approve selected proposed tool
PgUp / PgDn      scroll messages
j / k            select next/previous tool when tool runs are focused
a                approve selected proposed tool when tool runs are focused
d                deny selected proposed tool when tool runs are focused
Ctrl+R           refresh session/tool state
Esc              quit
```

`--print` renders one snapshot and exits, which is useful for non-interactive verification.

Message sends, refreshes, and tool approval/deny actions run asynchronously. While one is in flight, the status line shows loading text such as `thinking...`, `proposing tool...`, `refreshing...`, `approving...`, or `denying...`, and additional input is held until the current action finishes.

When a session has an attached workspace, the TUI side panel shows workspace identity, root path, and lightweight counts for `attachments`, `scripts`, `outputs`, `logs`, and `writeups`. This is metadata only; the panel does not read file contents.

## Agent Sessions

```powershell
cyberagent session create --workspace demo --title "Agent basics" --route learn
cyberagent session list
cyberagent session send <session-id> "summarize your current capabilities"
cyberagent session send <session-id> "/help"
cyberagent session send <session-id> "/model script"
cyberagent session send <session-id> "/model mimo/mimo-v2.5-pro"
cyberagent session send <session-id> "/compact"
cyberagent session send <session-id> "/ls ."
cyberagent session send <session-id> "/read README.md"
cyberagent session send <session-id> "/write README.md # Proposed replacement"
cyberagent session send <session-id> "/run echo hello"
cyberagent session history <session-id>
cyberagent session history <session-id> --all
```

Session chat is the main path for generic AI agent features. `/ls`, `/read`, and `/write` require an attached workspace. `/read` responses are redacted before they are persisted in history or sent to a model. `/write <path> <content>` creates a persisted file edit proposal and includes its diff in the assistant response; it never writes before approval. `/run` is dry-run only in v0.1 and records intent without executing commands.

## File Edit Proposals

```powershell
cyberagent edit propose --workspace demo --path README.md --content "# Updated"
cyberagent edit propose --workspace demo --path scripts/main.go --content-file C:\temp\main.go
cyberagent edit list --workspace demo
cyberagent edit list --session <session-id> --status proposed
cyberagent edit show <edit-id>
cyberagent edit approve <edit-id>
cyberagent edit deny <edit-id> --reason "not needed"
```

File edits replace the complete text content of one file. Existing files and new files under an existing workspace directory are supported. Absolute paths, `..` traversal, directory targets, symlink escapes, non-UTF-8 content, missing parent directories, and content over 256 KiB are rejected.

Proposals are stored without modifying the workspace. Approval compares the current file SHA-256 hash with the proposal's original hash, re-resolves the workspace path immediately before writing, and refuses stale changes. Proposed secrets are replaced with redaction markers before persistence and before any approved write. For exact multiline or whitespace-sensitive content, prefer `--content-file`; session `/write` trims the outer message whitespace.

## Tool Proposals

```powershell
cyberagent tool list
cyberagent tool list --session <session-id>
cyberagent tool list --status proposed
cyberagent tool show <tool-run-id>
cyberagent tool approve <tool-run-id>
cyberagent tool deny <tool-run-id> --reason "not needed"
```

`/run` creates a `tool_runs` proposal. Approval completes with dry-run output in v0.1; real command execution stays disabled until approval, sandbox, workspace scoping, and event logging are stricter.

## Context Compaction

```powershell
cyberagent context compact --workspace demo --task task-demo --message "user: imported challenge" --message "assistant: summarized plan"
cyberagent context show --task task-demo
```

`context compact` is the manual v0.1 version of a Codex-style compaction step. It stores a summary in SQLite and reports how many recent messages remain outside the summary.
