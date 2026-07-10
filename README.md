# CyberAgent Workbench

> 本地优先、可恢复、可审计的通用 AI Agent 工作台<br>
> A local-first, resumable, and auditable workbench for general-purpose AI agents.

## 项目简介 / Project Overview

### 中文

CyberAgent Workbench 是一个由 Go 驱动的本地 AI Agent 工作台，面向代码开发、代码审查、安全学习、脚本任务和受控网络安全分析。它把模型调用、长上下文、工作区文件、策略检查、审批、执行预算和事件记录统一到一套可恢复的运行时中，让一次任务即使在程序退出后也能继续，并且每一步都可以追踪和复核。

每个用户目标会被记录为一个 `Mission`，每次可恢复的执行过程则是一个 `Run`。Go 是唯一控制平面，负责模型路由、状态机、SQLite 持久化、安全策略和工具边界；未来的 TypeScript 界面与 Rust 确定性分析器都通过 Go 协议接入，不绕过安全控制。

项目当前优先完善通用单 Agent 运行时。CTF 将作为后续 Profile 和 Skills 能力接入，而不是另建一套独立运行系统。

当前版本已经提供 Run 级结构化 Work Board 与 Notes：工作项负责可执行计划，Note 负责观察、假设、决策、摘要和来源引用，所有变更都与 Run 事件在同一事务提交。Supervisor 使用 8192 token 的独立记忆预算选择摘要、活跃工作项和当前 root 可见 Note，并把来源 ID 与 token 估算记录在 `model.started`；Note 正文不会写入模型事件。仍有活跃工作项时，模型不能自行 `finish`。

统一 Tool Gateway 的第一条纵向链路也已落地。工作区读取、Shell 提案和整文件替换现在共享 Go 定义的 `ToolCall -> Decision -> Proposal -> Execution -> Result` 契约；CLI、Session 与 TUI 都通过同一入口。生产路径会用工作区 ID 解析可信根目录，拒绝伪造路径；读取可在低风险策略下自动执行，Shell 仍只进行 dry-run，文件写入仍要求显式逐次审批。

### English

CyberAgent Workbench is a local AI agent workbench powered by Go for coding, code review, security learning, scripting, and controlled cybersecurity analysis. It brings model calls, long-context memory, workspace files, policy checks, approvals, execution budgets, and event history into one resumable runtime, so work can continue after a process restart and every action remains inspectable.

Each user objective is stored as a `Mission`, while each resumable execution is a `Run`. Go is the sole control plane for model routing, state machines, SQLite persistence, safety policy, and tool boundaries. Future TypeScript interfaces and deterministic Rust analyzers connect through Go-defined protocols instead of bypassing those controls.

The current priority is the general-purpose single-agent runtime. CTF capabilities will be added later as Profiles and Skills on top of the same foundation rather than as a separate execution system.

The current build includes a structured, Run-scoped Work Board and durable Notes. WorkItems hold executable plans, while Notes hold observations, hypotheses, decisions, summaries, and source references. Every mutation commits with its Run event. The Supervisor selects summaries, active work, and root-visible Notes under a separate 8,192-token memory budget, then records source IDs and token estimates in `model.started` without persisting Note bodies there. Model-driven `finish` remains blocked while active work exists.

The first vertical slice of the unified Tool Gateway is also in place. Workspace reads, shell proposals, and whole-file replacements now share a Go-owned `ToolCall -> Decision -> Proposal -> Execution -> Result` contract, and the CLI, Session, and TUI use that same boundary. Production calls resolve a trusted root from the workspace ID and reject mismatches. Low-risk reads may execute automatically, while shell approval remains a dry run and file writes still require explicit per-call approval.

## 核心能力 / Core Capabilities

- **可恢复运行 / Resumable runs:** durable checkpoints, bounded execution, restart recovery, graceful terminal cancellation, and explicit lifecycle actions.
- **统一模型网关 / Model gateway:** route-based providers, Anthropic-compatible SSE streaming, typed failures, application-owned active-call cancellation, bounded live progress, one lifecycle-protocol repair, and durable model events.
- **长上下文与结构化记忆 / Long-context memory:** persisted sessions, automatic compaction, durable categorized Notes, visibility rules, and token-budgeted source selection.
- **结构化任务板 / Structured Work Board:** Run-scoped work items, dependency and cycle checks, optimistic versions, transactional events, and bounded Supervisor context.
- **本地工作区 / Local workspace:** scoped file access, safe reads, persistent artifacts, and reviewable edit proposals.
- **统一工具网关 / Unified Tool Gateway:** normalized calls, trusted workspace binding, policy decisions, shared shell/file approval, bounded UTF-8 results, MIME metadata, and compatibility adapters.
- **安全与审批 / Safety and approval:** policy checks, secret redaction, automatic low-risk reads, per-call write/shell approval, permanent denial, and dry-run command completion.
- **完整审计链 / Audit trail:** append-only Run events for messages, context source provenance, bounded text-free stream progress, model calls, Notes, policy decisions, tool proposals, and file edits.
- **CLI 与 TUI / CLI and TUI:** a scriptable CLI plus a Bubble Tea interface with live model progress and audited cancellation.
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
go run ./cmd/cyberagent todo create <run-id> "inspect parser" --priority high --acceptance "tests pass"
go run ./cmd/cyberagent todo create <run-id> "write tests" --depends-on <work-id>
go run ./cmd/cyberagent todo list <run-id> --status pending,blocked
go run ./cmd/cyberagent todo show <work-id>
go run ./cmd/cyberagent todo block <work-id> --reason "waiting for fixture"
go run ./cmd/cyberagent todo reopen <work-id>
go run ./cmd/cyberagent todo complete <work-id>
go run ./cmd/cyberagent note create <run-id> "parser decision" --content "Use strict JSON" --category decision --pin
go run ./cmd/cyberagent note create <run-id> "test evidence" --content-file .\note.txt --source docs/spec.md --evidence evidence-1
go run ./cmd/cyberagent note list <run-id> --status active --tag parser
go run ./cmd/cyberagent note show <note-id>
go run ./cmd/cyberagent note update <note-id> --visibility root --version 1
go run ./cmd/cyberagent note archive <note-id>
go run ./cmd/cyberagent note restore <note-id>
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

The current priority is the V2 run-centric runtime. P0 and P1 are complete. P2 supports resumable no-tool root Agent turns, cumulative token/model-time accounting, bounded execution and Provider retry loops, strict Supervisor-owned `continue`, `finish`, and `wait` actions, one Run execution path for ordinary CLI/TUI Session chat, real Provider streaming with bounded `model.delta` progress, application-owned active-call query/cancellation, Bubble Tea live metadata and `Ctrl+X` cancellation, durable model events, and exactly one restart-safe lifecycle-protocol repair. P3 includes migration v9 WorkItems, migration v10 Notes, transactional relationships/events, `todo` and `note` CLI lifecycles, root/owner visibility, token-budgeted memory selection, and durable context provenance. P5 has started with the unified Tool Gateway domain model, trusted workspace scope binding, shared approval adapters, output limits, and production-path migration. Real Local/Docker command execution remains disabled. CTF-specific solving logic stays deferred until the generic runtime is stable.

TUI quick controls: `cyberagent tui` opens a session picker. In chat, `Tab` switches focus, `PgUp/PgDn` scroll messages, `j/k` select tool runs, `a` approves, `d` denies, `Ctrl+X` requests cancellation of the current model call, `Ctrl+R` refreshes, and `Esc` quits when idle. Busy sends cannot be closed accidentally with `Esc`; cancel or wait first. The status line renders provider/model, attempt, chunk/byte progress, cancellation, disconnect, and terminal state without exposing raw model text. Attached workspaces render in the side panel with local directory counts for attachments, scripts, outputs, logs, and writeups.

Workspace file reads and model-bound messages pass through heuristic secret redaction for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, and private-key blocks.

File changes and Shell proposals now enter the same Tool Gateway approval service while retaining their existing SQLite records and Run-event projections. `edit propose` and session `/write` persist a redacted diff without changing the workspace. `edit approve` resolves the Store-owned workspace root again and compares the current file hash with the proposal before writing, so a forged root or stale proposal cannot overwrite newer user changes. Shell approval still produces dry-run output only.

## 可选在线 Provider / Optional Online Providers

CLI 只在对应 API key 存在于当前进程环境时注册 `mimo` 或 `deepseek` Anthropic-compatible Provider。密钥不会写入仓库文件、SQLite 或 Run 事件。<br>
The CLI registers the `mimo` and `deepseek` Anthropic-compatible providers only when their API keys exist in the current process environment. Keys are never written to repository files, SQLite, or Run events.

```powershell
$env:MIMO_API_KEY = "<token-plan-key>"
$env:MIMO_BASE_URL = "https://token-plan-cn.xiaomimimo.com/anthropic"
$env:MIMO_MODEL = "mimo-v2.5-pro"
go run ./cmd/cyberagent provider list
go run ./cmd/cyberagent provider test mimo/mimo-v2.5-pro

$env:DEEPSEEK_API_KEY = "<deepseek-key>"
$env:DEEPSEEK_BASE_URL = "https://api.deepseek.com/anthropic"
$env:DEEPSEEK_MODEL = "deepseek-v4-flash"
go run ./cmd/cyberagent provider list
go run ./cmd/cyberagent provider test deepseek/deepseek-v4-flash
go run ./cmd/cyberagent run create "review this workspace" --profile review --route deepseek/deepseek-v4-flash
```

`DEEPSEEK_BASE_URL` and `DEEPSEEK_MODEL` are optional; their current defaults are the values shown above. Use `deepseek-v4-pro` explicitly when the higher-capability model is required. See the official [DeepSeek Anthropic API guide](https://api-docs.deepseek.com/guides/anthropic_api) for current compatibility and model details.
