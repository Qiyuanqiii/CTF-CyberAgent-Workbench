# CyberAgent Workbench

CyberAgent Workbench is a local-first Go skeleton for a general coding and cyber learning agent runtime.

The project is transitioning to a run-centric architecture: a `Mission` is the stable user objective, while a `Run` is one resumable execution attempt. `Run` is a domain concept implemented and controlled by Go, not a programming language.

The v0.1 scaffold focuses on stable boundaries:

- CLI-first workflow through `cyberagent`
- deterministic mock LLM provider
- Codex-style context compaction scaffold
- persisted agent sessions with slash commands
- versioned SQLite migrations plus Mission/Run lifecycle and append-only run events
- automatic Run/Session binding with transactional Session, Policy, ToolRun, and FileEdit timeline projection
- stable typed errors with CLI exit-code and future HTTP-status mappings
- idempotent legacy Task-to-Run adaptation through `run adapt-task`
- resumable no-tool RunSupervisor turns with durable pre/post checkpoints
- persisted cumulative token/model-time budgets with per-call remaining-token limits
- bounded `run execute` loops and atomic operator-controlled Run completion/failure
- strict `root_lifecycle.v1` actions with Supervisor-owned `continue`, `finish`, and resumable `wait`
- ordinary Run-bound Session chat routed through the same RunSupervisor, budgets, policy, and event stream
- durable, redacted pending user input with restart recovery and exactly-once Session message commit
- typed Provider outcomes with bounded, cancellation-aware retry and `Retry-After` handling
- durable `model.started`, `model.completed`, and `model.failed` events with per-attempt execution accounting
- workspace-scoped list/read commands for safe file context
- secret redaction before file context, session storage, context summaries, tool runs, and provider calls
- tool proposal and approval flow for session `/run`
- persisted file edit proposals with safe diff preview, stale-file detection, and explicit approval
- Bubble Tea terminal UI shell with session picker, loading states, and workspace context
- model router and future provider extension points
- local workspace layout under `~/.cyberagent-workbench`
- SQLite event store via `github.com/mattn/go-sqlite3`
- safety policy checks for risky cyber actions
- no-op, local, and placeholder Docker sandbox runners

API keys are never stored by the application. Optional live providers read keys from the current process environment only.

## Build Requirements

- Go
- CGO enabled
- A Windows C compiler toolchain, such as MinGW-w64 GCC

On this machine, WinLibs MinGW-w64 was installed through winget and Go user env was configured with `CGO_ENABLED=1` plus `CC` pointing at the installed `gcc.exe`.

## Quick Start

```powershell
go run ./cmd/cyberagent version
go run ./cmd/cyberagent provider list
go run ./cmd/cyberagent workspace init demo
go run ./cmd/cyberagent workspace tree demo
go run ./cmd/cyberagent workspace read demo README.md
go run ./cmd/cyberagent run create "review this workspace" --workspace demo --profile review
go run ./cmd/cyberagent session send <run-session-id> "inspect the current workspace"
go run ./cmd/cyberagent run adapt-task <legacy-task-id>
go run ./cmd/cyberagent run list
go run ./cmd/cyberagent run show <run-id>
go run ./cmd/cyberagent run events <run-id>
go run ./cmd/cyberagent run start <run-id>
go run ./cmd/cyberagent run step <run-id>
go run ./cmd/cyberagent run execute <run-id> --max-steps 3
go run ./cmd/cyberagent run execute <run-id> --max-steps 3 --finish --summary "planning complete"
go run ./cmd/cyberagent run finish <run-id> --summary "review complete"
go run ./cmd/cyberagent run fail <run-id> --reason "blocked by provider"
go run ./cmd/cyberagent run checkpoint <run-id>
go run ./cmd/cyberagent session create --workspace demo --title "Agent basics" --route learn
go run ./cmd/cyberagent session send <session-id> "/ls ."
go run ./cmd/cyberagent session send <session-id> "/read README.md"
go run ./cmd/cyberagent session send <session-id> "/write README.md # Proposed replacement"
go run ./cmd/cyberagent session send <session-id> "/run echo hello"
go run ./cmd/cyberagent edit list --workspace demo --status proposed
go run ./cmd/cyberagent edit show <edit-id>
go run ./cmd/cyberagent edit approve <edit-id>
go run ./cmd/cyberagent tool list --session <session-id>
go run ./cmd/cyberagent tool approve <tool-run-id>
go run ./cmd/cyberagent tui
go run ./cmd/cyberagent tui --session <session-id>
go run ./cmd/cyberagent tui --session <session-id> --print
go run ./cmd/cyberagent script new "parse pcap http token" --workspace demo
go run ./cmd/cyberagent ctf init baby-web --category web
go run ./cmd/cyberagent context compact --workspace demo --task task-demo --message "user: imported challenge" --message "assistant: summarized plan"
go run ./cmd/cyberagent context show --task task-demo
```

Use `CYBERAGENT_HOME` to point runtime data at another directory during tests or experiments.

## Project Memory

Read [docs/PROJECT_STATUS.md](docs/PROJECT_STATUS.md), [docs/PROGRESS_BOOK.md](docs/PROGRESS_BOOK.md), [docs/TASK_BOOK.md](docs/TASK_BOOK.md), [docs/errors.md](docs/errors.md), [ADR 0001](docs/adr/0001-go-control-plane.md), and [ADR 0002](docs/adr/0002-run-centric-runtime.md) first when resuming development after a long conversation. They record current progress, language ownership, run architecture, error contract, audit notes, verified commands, and the recommended next slice.

## Repository Workflow

The canonical remote is [Qiyuanqiii/CTF-CyberAgent-Workbench](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench). Each completed development slice ends with tests, a focused code/security audit, project-memory updates, a Git commit, and a push to GitHub. Pull requests remain user-controlled; the repository template records the expected summary, validation, and audit evidence.

Local runtime databases, workspace data, environment files, API keys, IDE metadata, and build output are excluded from Git.

## Development Priority

The current priority is the V2 run-centric runtime. P0 and P1 are complete. P2 now supports resumable no-tool root Agent turns, cumulative token/model-time accounting, bounded execution and Provider retry loops, strict Supervisor-owned `continue`, `finish`, and `wait` actions, one Run execution path for ordinary CLI/TUI Session chat, and normalized model attempt events. Next comes bounded lifecycle-protocol repair and a real `model.delta` streaming/cancellation path before structured work items/notes or multi-agent coordination. CTF-specific solving logic stays deferred until the generic runtime is stable.

TUI quick controls: `cyberagent tui` opens a session picker. In chat, `Tab` switches focus, `PgUp/PgDn` scroll messages, `j/k` select tool runs, `a` approves, `d` denies, `Ctrl+R` refreshes, and `Esc` quits. Slow sends, refreshes, and tool approvals run through async commands with visible status text such as `thinking...`, `proposing tool...`, or `approving...`. Attached workspaces render in the side panel with local directory counts for attachments, scripts, outputs, logs, and writeups.

Workspace file reads and model-bound messages pass through heuristic secret redaction for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, and private-key blocks.

File changes use a separate approval boundary. `edit propose` and session `/write` persist a redacted diff without changing the workspace. `edit approve` re-resolves the workspace path and compares the current file hash with the proposal before writing, so stale proposals do not overwrite newer user changes.

## Optional Mimo Provider

The CLI registers a `mimo` Anthropic-compatible provider only when `MIMO_API_KEY` exists in the current environment. Secrets are not written to repo files or SQLite.

```powershell
$env:MIMO_API_KEY = "<token-plan-key>"
$env:MIMO_BASE_URL = "https://token-plan-cn.xiaomimimo.com/anthropic"
$env:MIMO_MODEL = "mimo-v2.5-pro"
go run ./cmd/cyberagent provider list
go run ./cmd/cyberagent provider test mimo/mimo-v2.5-pro
```
