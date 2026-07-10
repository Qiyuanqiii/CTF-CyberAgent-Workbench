# CyberAgent Workbench

> 本地优先、可恢复、可审计的通用 AI Agent 工作台<br>
> A local-first, resumable, and auditable workbench for general-purpose AI agents.

## 项目简介 / Project Overview

### 中文

CyberAgent Workbench 是一个由 Go 驱动的本地 AI Agent 工作台，面向代码开发、代码审查、安全学习、脚本任务和受控网络安全分析。它把模型调用、长上下文、工作区文件、策略检查、审批、执行预算和事件记录统一到一套可恢复的运行时中，让一次任务即使在程序退出后也能继续，并且每一步都可以追踪和复核。

每个用户目标会被记录为一个 `Mission`，每次可恢复的执行过程则是一个 `Run`。Go 是唯一控制平面，负责模型路由、状态机、SQLite 持久化、安全策略和工具边界；未来的 TypeScript 界面与 Rust 确定性分析器都通过 Go 协议接入，不绕过安全控制。

项目当前优先完善通用单 Agent 运行时。CTF 将作为后续 Profile 和 Skills 能力接入，而不是另建一套独立运行系统。

### English

CyberAgent Workbench is a local AI agent workbench powered by Go for coding, code review, security learning, scripting, and controlled cybersecurity analysis. It brings model calls, long-context memory, workspace files, policy checks, approvals, execution budgets, and event history into one resumable runtime, so work can continue after a process restart and every action remains inspectable.

Each user objective is stored as a `Mission`, while each resumable execution is a `Run`. Go is the sole control plane for model routing, state machines, SQLite persistence, safety policy, and tool boundaries. Future TypeScript interfaces and deterministic Rust analyzers connect through Go-defined protocols instead of bypassing those controls.

The current priority is the general-purpose single-agent runtime. CTF capabilities will be added later as Profiles and Skills on top of the same foundation rather than as a separate execution system.

## 核心能力 / Core Capabilities

- **可恢复运行 / Resumable runs:** durable checkpoints, bounded execution, restart recovery, graceful terminal cancellation, and explicit lifecycle actions.
- **统一模型网关 / Model gateway:** route-based providers, Anthropic-compatible SSE streaming, typed failures, application-owned active-call cancellation, bounded live progress, one lifecycle-protocol repair, and durable model events.
- **长上下文管理 / Long-context memory:** persisted sessions and automatic context compaction inspired by modern coding agents.
- **本地工作区 / Local workspace:** scoped file access, safe reads, persistent artifacts, and reviewable edit proposals.
- **安全与审批 / Safety and approval:** policy checks, secret redaction, dry-run tool proposals, and explicit approval boundaries.
- **完整审计链 / Audit trail:** append-only Run events for messages, bounded text-free stream progress, model calls, policy decisions, tool proposals, and file edits.
- **CLI 与 TUI / CLI and TUI:** a scriptable `cyberagent` CLI plus a Bubble Tea terminal interface.
- **可扩展架构 / Extensible architecture:** Go control plane with planned HTTP/WebSocket, TypeScript UI, Docker sandbox, and Rust analyzer boundaries.

> [!NOTE]
> 当前版本仍在积极开发中。真实工具自动执行、多 Agent 协作、Web UI 和 CTF 自动求解尚未开放。<br>
> This project is under active development. Autonomous tool execution, multi-agent coordination, the Web UI, and automated CTF solving are not enabled yet.

**密钥边界 / Secret boundary:** 应用不会持久化 API key；可选在线 Provider 只从当前进程环境变量读取密钥。<br>
The application never persists API keys; optional live providers read them only from the current process environment.

## Build Requirements

- Go 1.25 or newer; use the latest patch release for standard-library security fixes
- CGO enabled
- A Windows C compiler toolchain, such as MinGW-w64 GCC

On this machine, validation uses Go 1.26.5. WinLibs MinGW-w64 was installed through winget and Go user env was configured with `CGO_ENABLED=1` plus `CC` pointing at the installed `gcc.exe`.

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

The current priority is the V2 run-centric runtime. P0 and P1 are complete. P2 now supports resumable no-tool root Agent turns, cumulative token/model-time accounting, bounded execution and Provider retry loops, strict Supervisor-owned `continue`, `finish`, and `wait` actions, one Run execution path for ordinary CLI/TUI Session chat, real Provider streaming with bounded `model.delta` progress, application-owned active-call query/cancellation, bounded metadata-only live subscribers, durable model events, and exactly one restart-safe lifecycle-protocol repair. Next comes TUI consumption of that live control boundary before structured work items/notes or multi-agent coordination. CTF-specific solving logic stays deferred until the generic runtime is stable.

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
