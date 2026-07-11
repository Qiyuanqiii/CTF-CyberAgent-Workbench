# CyberAgent Workbench

> 本地优先、可恢复、可审计的通用 AI Agent 工作台<br>
> A local-first, resumable, and auditable workbench for general-purpose AI agents.

## 项目简介 / Project Overview

### 中文

CyberAgent Workbench 是一个由 Go 驱动的本地 AI Agent 工作台，面向代码开发、代码审查、安全学习、脚本任务和受控网络安全分析。它把模型调用、长上下文、工作区文件、策略检查、审批、执行预算和事件记录统一到一套可恢复的运行时中，让一次任务即使在程序退出后也能继续，并且每一步都可以追踪和复核。

每个用户目标会被记录为一个 `Mission`，每次可恢复的执行过程则是一个 `Run`。Go 是唯一控制平面，负责模型路由、状态机、SQLite 持久化、安全策略和工具边界；未来的 TypeScript 界面与 Rust 确定性分析器都通过 Go 协议接入，不绕过安全控制。

项目当前优先完善通用单 Agent 运行时。CTF 将作为后续 Profile 和 Skills 能力接入，而不是另建一套独立运行系统。

当前版本已经提供 Run 级结构化 Work Board 与 Notes：工作项负责可执行计划，Note 负责观察、假设、决策、摘要和来源引用，所有变更都与 Run 事件在同一事务提交。Supervisor 使用 8192 token 的独立记忆预算选择摘要、活跃工作项和当前 root 可见 Note，并把来源 ID 与 token 估算记录在 `model.started`；Note 正文不会写入模型事件。仍有活跃工作项时，模型不能自行 `finish`。

统一 Tool Gateway 的第一条纵向链路也已落地。工作区读取、Shell 提案、整文件替换和脚本进程提案现在共享 Go 定义的 `ToolCall -> Decision -> Proposal -> Execution -> Result` 契约；CLI、Session 与 TUI 都通过同一入口。生产路径会用工作区 ID 解析可信根目录，拒绝伪造路径；读取可在低风险策略下自动执行，Shell 与 `script_process.v1` 审批仍只进行 dry-run，文件写入仍要求显式逐次审批。`script run --local` 只记录期望后端，不会执行宿主机命令。

schema v11 新增统一的持久化审批账本。每个 Shell/FileEdit 提案都会在原业务事务中创建 Run/Session 关联的 `tool_approvals` 记录和 `approval.requested` 事件；审批操作使用不可变幂等键摘要先提交 `approval_operations` 与 `approval.decided`，再推进兼容提案，客户端原始 key 不写入数据库。进程在两步之间退出时，重复同一审批会恢复而不会重复决定或绕过策略。SQLite 会拒绝缺少批准事实的 `approved`、`applied` 或 `completed` 状态。

schema v12 新增可撤销 Session Grant 和 Run 级工具调用预算。Grant 精确绑定 Run、Session、Workspace、Tool 与 ActionClass，只有仍处于活动状态的匹配授权才能自动满足逐次审批，撤销后立即失效，且永久 Policy 拒绝始终优先。Run 工具调用以 SQLite 原子计数，首次超限写入一次 `tool.budget_exhausted`，并以稳定的 `RESOURCE_EXHAUSTED` 拒绝后续调用；旧 Run 的 `0` 保持无限制兼容，新建 CLI Run 默认上限为 100。

schema v13 将 `script_process.v1` 提升为独立类型化提案。`script run` 会在一个 SQLite 事务内创建 Mission、Session、Run、初始预算扣减、Policy 决策、Process、Approval 与完整事件链；客户端可用 `--idempotency-key` 安全重试，相同意图返回原有对象，不同意图复用同一键会返回冲突。原始幂等键不会写入数据库，脚本路径与参数在持久化前脱敏，审批仍只生成明确的 dry-run 结果。

schema v14 新增 Run 级工具输出 Artifact。终态 Shell、ScriptProcess、失败的 FileEdit 以及 Run 绑定的工作区读取，会在结果截断前捕获最多 4 MiB 的完整脱敏文本，记录 MIME、UTF-8、SHA-256、字节数、来源调用与 `artifact.created` 事件。常规 Result 仍限制为 128 KiB stdout 和 32 KiB stderr；Artifact 哈希基于脱敏后正文，并可通过 `artifact list/show/read/verify` 检查。

schema v15 新增 create-only 的 `work_item_create` 与 `note_create` 结构化记忆工具。调用必须绑定准确的 Run、Session 与 Workspace，先经过严格 JSON schema、工具预算和 Policy，再把业务记录、`policy.decision`、领域事件、`tool.completed` 与幂等操作账本原子提交。原始 operation key 不落库；相同 key 与相同意图安全重放，不同意图返回冲突；Note 内容和工具参数在输出、事件与持久化边界脱敏。

schema v16 将这两个工具接入 RunSupervisor 的 Provider 工具循环。每个模型轮次最多接受 4 个调用，每个 turn 最多执行 4 个工具轮次；`model.completed` 与 pending 工具批次原子提交，进程中断后会从 SQLite 恢复未完成调用。语义操作键由 Run、turn、工具名和脱敏规范化参数生成，不依赖 Provider 的临时 call ID，因此重发和跨轮重复意图只创建一个实体。Anthropic-compatible Provider 已支持 `tools`、`tool_use`、`tool_result` 与流式工具参数。Policy 拒绝和预算耗尽会作为有界错误结果返回模型；Shell、文件、进程、网络以及更新/删除类模型工具仍不开放。

当前还提供第一版本地只读 HTTP 控制面。`cyberagent api serve` 只允许绑定回环地址，并要求每个请求携带进程级 Bearer token；它通过稳定的 `api.v1` envelope 和有界游标分页读取 Run、Session、Event、WorkItem、Note、Artifact 元数据与 Supervisor ToolRound。API 不提供写入、CORS、WebSocket、Artifact 正文或 checkpoint pending input，也不会持久化访问令牌。它是未来 TypeScript 界面的 Go 安全边界，不是绕过 CLI、Store、Policy 或 Tool Gateway 的旁路。

Bubble Tea TUI 现在可直接查看当前 Run 状态、Work Board、Notes、持久化 Supervisor Tool Rounds 与 Shell ToolRuns。工具区支持“批准一次”和“本会话授权”：后者只创建精确绑定当前 Run、Session、Workspace、Shell 与 ActionClass 的可撤销 Grant，并在推进当前提案前重新检查持久化审批作用域和最新 Policy。后续安全 Shell 可自动完成 dry-run，危险命令仍永久拒绝且不会创建 Grant；中文、组合字符和宽字符按终端单元格安全换行与截断。

### English

CyberAgent Workbench is a local AI agent workbench powered by Go for coding, code review, security learning, scripting, and controlled cybersecurity analysis. It brings model calls, long-context memory, workspace files, policy checks, approvals, execution budgets, and event history into one resumable runtime, so work can continue after a process restart and every action remains inspectable.

Each user objective is stored as a `Mission`, while each resumable execution is a `Run`. Go is the sole control plane for model routing, state machines, SQLite persistence, safety policy, and tool boundaries. Future TypeScript interfaces and deterministic Rust analyzers connect through Go-defined protocols instead of bypassing those controls.

The current priority is the general-purpose single-agent runtime. CTF capabilities will be added later as Profiles and Skills on top of the same foundation rather than as a separate execution system.

The current build includes a structured, Run-scoped Work Board and durable Notes. WorkItems hold executable plans, while Notes hold observations, hypotheses, decisions, summaries, and source references. Every mutation commits with its Run event. The Supervisor selects summaries, active work, and root-visible Notes under a separate 8,192-token memory budget, then records source IDs and token estimates in `model.started` without persisting Note bodies there. Model-driven `finish` remains blocked while active work exists.

The first vertical slice of the unified Tool Gateway is also in place. Workspace reads, shell proposals, whole-file replacements, and script-process proposals now share a Go-owned `ToolCall -> Decision -> Proposal -> Execution -> Result` contract, and the CLI, Session, and TUI use that same boundary. Production calls resolve a trusted root from the workspace ID and reject mismatches. Low-risk reads may execute automatically, while shell and `script_process.v1` approval remain dry-run only and file writes still require explicit per-call approval. `script run --local` records the requested backend but never executes a host command.

Schema v11 adds a unified durable approval ledger. Each Shell/FileEdit proposal creates a Run/Session-bound `tool_approvals` record and `approval.requested` event in the original business transaction. Review operations commit an immutable digest of the client idempotency key, `approval_operations`, and `approval.decided` before advancing the compatibility proposal; the raw client key is not stored. Repeating the same review after a crash resumes convergence without duplicating the decision or bypassing policy, and SQLite rejects `approved`, `applied`, or `completed` states without a persisted approval fact.

Schema v12 adds revocable Session grants and Run-level tool-call budgets. A grant is bound to an exact Run, Session, Workspace, Tool, and ActionClass; only a matching active grant can satisfy per-call review automatically, revocation takes effect immediately, and permanent Policy denial always wins. Tool calls are counted atomically in SQLite. The first rejected over-budget attempt appends one `tool.budget_exhausted` event and later calls return stable `RESOURCE_EXHAUSTED` errors. A zero limit preserves unlimited compatibility for older Runs, while new CLI Runs default to 100 calls.

Schema v13 promotes `script_process.v1` to a first-class typed proposal. `script run` creates its Mission, Session, Run, initial budget charge, Policy decision, Process, Approval, and complete event chain in one SQLite transaction. `--idempotency-key` makes retries recoverable: identical intent returns the original objects, while changed intent under the same key returns a conflict. Raw operation keys are never persisted, script paths and arguments are redacted at the Store boundary, and approval still produces an explicit dry-run result only.

Schema v14 adds Run-scoped tool-output Artifacts. Terminal Shell and ScriptProcess output, failed FileEdit diagnostics, and Run-bound workspace reads are captured before Result truncation as up to 4 MiB of complete redacted text with MIME, UTF-8 encoding, SHA-256, byte count, source linkage, and an `artifact.created` event. Ordinary Results remain capped at 128 KiB stdout and 32 KiB stderr. Artifact hashes cover the redacted content and can be inspected through `artifact list/show/read/verify`.

Schema v15 adds create-only `work_item_create` and `note_create` structured-memory tools. Every call is bound to an exact Run, Session, and Workspace, then passes strict JSON validation, the Run tool budget, and Policy before the business record, `policy.decision`, domain event, `tool.completed`, and idempotency ledger commit atomically. Raw operation keys are never stored; identical intent replays safely, changed intent conflicts, and Note content and tool payloads are redacted at output, event, and persistence boundaries.

Schema v16 connects only those two tools to the RunSupervisor Provider loop. A model response may request at most four calls, and one turn may execute at most four tool rounds. `model.completed` and its pending tool batch commit atomically, so unfinished calls resume from SQLite after a process interruption. Semantic operation keys derive from the Run, turn, tool name, and redacted canonical arguments rather than transient Provider call IDs, making repeated intent converge on one entity. The Anthropic-compatible transport now supports `tools`, `tool_use`, `tool_result`, and streamed tool arguments. Policy denial and budget exhaustion return bounded error results to the model; model-driven Shell, file, process, network, update, and delete tools remain disabled.

The project also includes its first local read-only HTTP control plane. `cyberagent api serve` binds only to a loopback address and requires a process-scoped bearer token on every request. Stable `api.v1` envelopes and bounded cursor pagination expose Run, Session, Event, WorkItem, Note, Artifact metadata, and Supervisor ToolRound state. The API has no writes, CORS, WebSocket, Artifact content, or checkpoint pending input, and never persists its access token. It is the Go security boundary for a future TypeScript UI, not a path around the CLI, Store, Policy, or Tool Gateway.

The Bubble Tea TUI now exposes the current Run state, Work Board, Notes, durable Supervisor Tool Rounds, and Shell ToolRuns. Its tool pane supports “approve once” and “approve for this session.” Session approval creates only a revocable Grant scoped to the exact Run, Session, Workspace, Shell tool, and ActionClass, then rechecks the durable approval scope and current Policy before advancing the proposal. Later safe Shell calls may complete as dry runs, while dangerous commands remain permanently denied and cannot create a Grant. Terminal wrapping and truncation account for CJK, combining characters, and other wide graphemes.

## 核心能力 / Core Capabilities

- **可恢复运行 / Resumable runs:** durable checkpoints, bounded execution, restart recovery, graceful terminal cancellation, and explicit lifecycle actions.
- **统一模型网关 / Model gateway:** route-based providers, Anthropic-compatible SSE/tool protocol, typed failures, application-owned active-call cancellation, bounded live progress, one lifecycle-protocol repair, and durable model/tool events.
- **长上下文与结构化记忆 / Long-context memory:** persisted sessions, automatic compaction, durable categorized Notes, visibility rules, and token-budgeted source selection.
- **结构化任务板 / Structured Work Board:** Run-scoped work items, dependency and cycle checks, optimistic versions, transactional events, and bounded Supervisor context.
- **本地工作区 / Local workspace:** scoped file access, safe reads, hashed Run artifacts, and reviewable edit proposals.
- **统一工具网关 / Unified Tool Gateway:** normalized calls, trusted workspace binding, policy decisions, shared review, first-class non-executable `script_process.v1` proposals, a bounded resumable Provider loop for create-only WorkItem/Note tools, bounded UTF-8 results, MIME metadata, and compatibility adapters.
- **安全与审批 / Safety and approval:** policy checks, secret redaction, automatic low-risk reads, durable per-call approvals, revocable scoped Session grants, atomic Run tool budgets, permanent denial, and dry-run command completion.
- **完整审计链 / Audit trail:** append-only Run events for messages, context source provenance, bounded text-free stream progress, model calls, Notes, policy decisions, tool proposals, file edits, and content-free Artifact metadata.
- **CLI 与 TUI / CLI and TUI:** a scriptable CLI plus a Bubble Tea interface with live model progress, audited cancellation, Run memory/tool-round views, and scoped once/session approvals.
- **可扩展架构 / Extensible architecture:** Go control plane with a loopback-only read API and planned WebSocket, TypeScript UI, Docker sandbox, and Rust analyzer boundaries.

> [!NOTE]
> 当前版本仍在积极开发中。Provider 只能自动创建 WorkItem/Note；真实 Shell/Docker 执行、更广工具面、多 Agent 协作、Web UI 和 CTF 自动求解尚未开放。<br>
> This project is under active development. Providers may only create WorkItems and Notes; real Shell/Docker execution, broader model tools, multi-agent coordination, the Web UI, and automated CTF solving are not enabled yet.

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
go run ./cmd/cyberagent run create "review this workspace" --workspace demo --profile review --max-tool-calls 100
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
go run ./cmd/cyberagent run usage <run-id>
go run ./cmd/cyberagent tool schema
go run ./cmd/cyberagent tool schema work_item_create
go run ./cmd/cyberagent tool invoke work_item_create --run <run-id> --operation-key <stable-key> --payload-file .\work-item.json
go run ./cmd/cyberagent tool invoke note_create --run <run-id> --operation-key <stable-key> --payload-file .\note.json
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
go run ./cmd/cyberagent tool show <proposal-id>
go run ./cmd/cyberagent tool approve <proposal-id>
go run ./cmd/cyberagent artifact list --run <run-id>
go run ./cmd/cyberagent artifact show <artifact-id>
go run ./cmd/cyberagent artifact read <artifact-id> --max-bytes 65536
go run ./cmd/cyberagent artifact verify <artifact-id>
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765
go run ./cmd/cyberagent approval list --run <run-id> --status pending
go run ./cmd/cyberagent approval show <approval-id>
go run ./cmd/cyberagent approval grant create --session <session-id> --tool shell --reason "trusted build commands"
go run ./cmd/cyberagent approval grant list --run <run-id> --status active
go run ./cmd/cyberagent approval grant revoke <grant-id> --reason "command phase complete"
go run ./cmd/cyberagent tui
go run ./cmd/cyberagent tui --session <session-id>
go run ./cmd/cyberagent tui --session <session-id> --print
go run ./cmd/cyberagent script new "parse pcap http token" --workspace demo
go run ./cmd/cyberagent script run scripts/<script-name>.py --workspace demo --local --idempotency-key <stable-key>
go run ./cmd/cyberagent ctf init baby-web --category web
go run ./cmd/cyberagent context compact --workspace demo --task task-demo --message "user: imported challenge" --message "assistant: summarized plan"
go run ./cmd/cyberagent context show --task task-demo
```

Use `CYBERAGENT_HOME` to point runtime data at another directory during tests or experiments.

`api serve` generates and prints a temporary access token when `CYBERAGENT_API_TOKEN` is absent. When the variable is supplied, the CLI validates its exact value but does not echo it. See [docs/http-api.md](docs/http-api.md) for endpoints, envelopes, pagination, and security boundaries.

## Project Memory

Read [docs/PROJECT_STATUS.md](docs/PROJECT_STATUS.md), [docs/PROGRESS_BOOK.md](docs/PROGRESS_BOOK.md), [docs/TASK_BOOK.md](docs/TASK_BOOK.md), [docs/http-api.md](docs/http-api.md), [docs/errors.md](docs/errors.md), [ADR 0001](docs/adr/0001-go-control-plane.md), and [ADR 0002](docs/adr/0002-run-centric-runtime.md) first when resuming development after a long conversation. They record current progress, language ownership, run architecture, API and error contracts, audit notes, verified commands, and the recommended next slice.

## Repository Workflow

The canonical remote is [Qiyuanqiii/CTF-CyberAgent-Workbench](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench). Each completed development slice ends with tests, a focused code/security audit, project-memory updates, a Git commit, and a push to GitHub. This repository currently develops directly on `main`; do not create a feature branch or pull request unless the user explicitly asks for one.

Local runtime databases, workspace data, environment files, API keys, IDE metadata, and build output are excluded from Git.

## Development Priority

The current priority is the V2 run-centric runtime. P0 and P1 are complete. P2 supports resumable root Agent turns, cumulative token/model-time accounting, bounded execution and Provider retry loops, strict Supervisor-owned `continue`, `finish`, and `wait` actions, one Run execution path for ordinary CLI/TUI Session chat, real Provider streaming with bounded `model.delta` progress, application-owned active-call query/cancellation, Bubble Tea live metadata and `Ctrl+X` cancellation, durable model events, exactly one restart-safe lifecycle-protocol repair, and the schema v16 bounded structured-memory tool loop. P3 includes migration v9 WorkItems, migration v10 Notes, transactional relationships/events, `todo` and `note` CLI lifecycles, root/owner visibility, token-budgeted memory selection, and durable context provenance. P5 includes the unified Tool Gateway, trusted workspace scope binding, schema v11 durable per-call approvals, schema v12 revocable Session grants and atomic Run tool-call accounting, schema v13 first-class typed script processes, schema v14 source-bound hashed output Artifacts, schema v15 idempotent structured-memory mutations, and v16 durable Provider tool rounds. P9 now includes the authenticated loopback-only `api.v1` read surface; WebSocket and TypeScript remain pending. Real Local/Docker command execution and all non-memory model tools remain disabled. CTF-specific solving logic stays deferred until the generic runtime is stable.

TUI quick controls: `cyberagent tui` opens a session picker. In chat, `Tab` switches between input and the activity pane; `h/l` changes among Tools, Work, Notes, and Rounds; `j/k` moves the current selection; `a` approves one Shell proposal, `g` approves it and grants the exact current session scope, and `d` denies it. `PgUp/PgDn` scrolls messages, `Ctrl+X` requests cancellation of the current model call, `Ctrl+R` refreshes, and `Esc` quits when idle. Busy sends cannot be closed accidentally with `Esc`; cancel or wait first. The status line renders provider/model, attempt, chunk/byte progress, cancellation, disconnect, and terminal state without exposing raw model text. Attached workspaces render in the side panel with local directory counts for attachments, scripts, outputs, logs, and writeups.

Workspace file reads and model-bound messages pass through heuristic secret redaction for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, and private-key blocks.

File changes, Shell proposals, typed script-process proposals, and create-only structured memory calls enter the same Tool Gateway and Run-event stream. The schema v11 approval ledger is inspectable through `approval list/show`; review idempotency survives restart, while a conflicting reuse of the same key is rejected. Schema v12 Session grants are managed through `approval grant create/list/show/revoke`, and `run usage` exposes durable tool consumption. Schema v13 stores script processes separately from legacy Shell ToolRuns and makes initial Run creation recoverably idempotent. Schema v14 stores redacted full-stream evidence separately from bounded Results and links each Artifact to its exact proposal or invocation. Schema v15 stores only operation-key digests and normalized request fingerprints for WorkItem/Note creation. `edit propose` and session `/write` normally persist a redacted diff without changing the workspace; an explicitly created matching Session grant may authorize and apply the edit immediately, with the grant ID recorded on the approval fact. Shell and script-process approval still produce dry-run output only.

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
