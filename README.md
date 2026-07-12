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

schema v17 新增持久化 Run execution lease。`run step` 每次持有一份租约，`run execute` 在整个有界循环内复用同一份租约；心跳会在长模型调用期间续期，过期租约只能由新的 generation 接管。Supervisor checkpoint、模型事件、工具轮次、结构化记忆写入与工具预算都在 SQLite 事务内校验同一 fencing token，因此旧 worker 在接管后不能继续提交、扣预算或写实体。租约获取、接管和释放进入不含 token 的 Run 事件流；`run lease` 与 Run detail 只公开状态摘要，不公开内部 `lease_id`。

schema v18 新增跨进程模型取消账本。可选控制 API 只提交精确绑定 `run_id + attempt_id + model_attempt` 的取消意图，持有私有 execution lease 的 worker 轮询并消费，再取消 Go 管理的 Provider context。原始 `Idempotency-Key` 与令牌不落库；请求、worker 观察和模型终态都有审计记录。旧 attempt 的请求不会误伤重试，崩溃遗留请求会在后续 attempt 开始时解析为 `superseded`。

schema v19 落地第一阶段 Agent Coordinator。每个新 Run 在创建事务中获得稳定的 root `AgentNode`；旧数据库会在下一次 Coordinator/Supervisor 操作时惰性补建。root 的 ready/running/waiting/terminal 状态与 Supervisor checkpoint、Run lifecycle、inbox 和 `agent_graph.v1` 恢复快照原子提交，`run graph` 可复核当前投影。inbox 每个 Agent 最多保留 128 条 pending 消息和 4096 条总历史，快照只保留最近 32 份并用 SHA-256 校验消息 payload；v19 初始阶段的 root `child_limit=0`，后续 v21 仅通过内部显式 policy 开放有界 Specialist admission。

schema v20 为 Coordinator inbox 增加可恢复的消息协议。每次发送必须携带 16-256 字节的幂等键，SQLite 只保存域分隔 SHA-256 摘要和脱敏意图指纹；相同键与相同意图返回原消息且不重复事件、快照或状态变化，异意图复用返回 `CONFLICT`。`wake` 只能在 Run 正在运行时把同一 Run 内 waiting Specialist 变回 ready，不能唤醒 root 或隐式恢复暂停的 Run；`dependency` 使用严格 payload schema 并要求 Agent sender。普通 v19 消息和快照保持读取兼容，inbox 仍是 Go 内部边界，尚未注入模型提示词。

schema v21 增加默认关闭、仅 Go 内部可启用的 Specialist admission。显式 policy 最多允许两个 depth-1 child；每个 child 必须使用父 Skills 的非空子集、独立活动 Session 和正数 turn/token 预留，且预留后至少保留一个 root turn。Session、child、root capacity/有效预算、摘要幂等事实、事件和图快照在同一事务提交；并发重试收敛到同一 child。预留额度会从 root 后续 Supervisor turn/model token 上限扣除。Run pause/resume 只联动因 Run 生命周期等待的 child，Run complete/fail/cancel 会终止 child 并归档 Session。child 模型循环、公开 spawn API 和模型自主建 child 仍未开放。

schema v22 为 WorkItem 与 Note 增加可选但受验证的 `owner_agent_id`。Store 和 SQLite trigger 都要求所有者 Agent 真实存在、属于同一 Run，且新分配不能指向终态 Agent；跨 Run 伪造在事务提交前失败。旧 `owner` 标签和历史行保持兼容，owner-only Agent Note 会自动镜像兼容标签，但可见性判断优先使用真实 Agent 身份。Supervisor 与 `tool invoke` 由 Go 控制面注入调用 Agent，模型 JSON schema 不暴露 `owner_agent_id`。CLI、TUI、HTTP/OpenAPI 已支持显示和过滤该字段，v21 数据可原地升级且不丢失旧标签。

schema v23 增加严格的 `agent_completion.v1` CompletionReport 和内部 `agent.finish` 提交路径。报告只接受 `succeeded`/`partial`、有界脱敏摘要以及 child 自己拥有的 WorkItem/父可见 Note 引用；成功报告不能遗留活跃 child WorkItem，partial 必须显式交接全部活跃项。报告、摘要幂等账本、父 inbox result、child 终态、Session 归档、审计事件与图快照在同一事务提交，并绑定精确 active attempt；并发重试只生成一份报告，旧 attempt 不能覆盖结果。child 模型循环、公开 finish/spawn API 和模型自主建 child 仍未开放。

schema v24 增加默认关闭、仅 Go 内部可启用的 Specialist Attempt Runtime。每个 child turn 都绑定当前 Run execution lease 与 generation，开始时原子扣减 turn，模型 usage 只允许记录一次并累计到 child token 预算；`continue`、`finished`、`crashed`、`interrupted` 都是不可变终态。崩溃会把脱敏原因和是否可重试通知父 inbox，预算耗尽时归档 child Session；新 worker 接管过期租约后可恰好一次地回收遗留 Attempt，旧 worker 不能再提交新的 usage、完成或失败状态。Run 暂停或终止会先中断 Attempt 再移动 child 投影，恢复时复核尝试序号、累计 token、CompletionReport 和图快照。公开 CLI/HTTP/model spawn、child 模型循环及新增 Shell/网络权限仍然关闭。

当前还提供第一版本地 HTTP 控制面。`cyberagent api serve` 只允许绑定回环地址；`CYBERAGENT_API_TOKEN` 只授权读取，默认关闭的取消入口必须另行设置不同的 `CYBERAGENT_API_CONTROL_TOKEN`。稳定的 `api.v1` envelope 和有界游标分页可读取 Run、Session、Event、WorkItem、Note、Artifact 元数据、Supervisor ToolRound 与不含 fencing token 的 execution-lease 摘要。唯一写路由只能请求取消当前模型调用，不能执行工具、改变 Policy、接受客户端 fencing token 或读取 Artifact 正文/checkpoint pending input。API 不提供 CORS 或 WebSocket，也不会持久化任何 API 令牌。

Go 响应 DTO 现在同时生成唯一的 OpenAPI 3.1 契约。`cyberagent api openapi` 可输出确定性的 JSON，`--output docs/openapi.json` 可更新仓库内的受测快照；运行中的服务也会在鉴权后的 `/api/v1/openapi.json` 返回同一份原始文档。契约测试会逐条请求全部公开路由并阻止 DTO、静态文档与 handler 漂移，且明确排除 Artifact 正文、checkpoint pending input、`lease_id` 和 fencing token。

只读 Run Event Stream 也已接入同一控制面。`/api/v1/runs/{run_id}/events/stream` 使用 SSE 推送 SQLite 中已脱敏的持久化事件；每帧 `id` 是与 Run 绑定的不透明 cursor，客户端可通过 `Last-Event-ID` 精确续传。连接具有批量、帧大小、总事件数、寿命、写超时和进程并发上限，心跳不伪造 Run event；服务器关闭会主动取消长连接。SSE 本身不推送模型正文、不接受 token query、也不执行任何写操作。

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

Schema v17 adds durable Run execution leases. `run step` holds one lease for its turn, while `run execute` reuses one lease across the bounded loop. A heartbeat renews long model calls, and only a new generation may take over an expired lease. Supervisor checkpoints, model events, tool rounds, structured-memory writes, and tool-budget charges verify the same fencing token inside SQLite transactions, so a stale worker cannot commit, consume budget, or create entities after takeover. Acquisition, takeover, and release are audited without the token; `run lease` and Run detail expose status metadata but never the internal `lease_id`.

Schema v18 adds a cross-process model-cancellation ledger. The optional control API submits only an intent bound to the exact `run_id + attempt_id + model_attempt`; the worker holding the private execution lease polls and consumes it before cancelling the Go-owned Provider context. Raw idempotency keys and tokens are never stored. Request, worker observation, and model terminal state are audited, old requests cannot cancel a retry, and crash-orphaned requests resolve as `superseded` when a later attempt begins.

Schema v19 delivers the first Agent Coordinator slice. Every new Run receives a stable root `AgentNode` in its creation transaction, while older databases register one lazily on the next Coordinator or Supervisor operation. Root ready/running/waiting/terminal state commits atomically with Supervisor checkpoints, Run lifecycle, inbox changes, and bounded `agent_graph.v1` recovery snapshots; `run graph` verifies the live projection. Each inbox is capped at 128 pending and 4,096 historical messages, only the latest 32 snapshots are retained, and pending payloads are integrity-checked by SHA-256. Roots began with `child_limit=0`; schema v21 later permits bounded Specialist admission only through an explicit internal policy.

Schema v20 adds a recoverable Coordinator inbox protocol. Every send requires a 16-256 byte idempotency key, while SQLite stores only a domain-separated SHA-256 digest and a redacted intent fingerprint. Replaying the same key and intent returns the original message without duplicating events, snapshots, or state transitions; changed intent conflicts. `wake` can only move a waiting Specialist in the same running Run back to ready and cannot wake root or implicitly resume a paused Run. `dependency` has a strict payload schema and requires an Agent sender. Ordinary v19 messages and snapshots remain readable, and the inbox is still an internal Go boundary rather than model prompt content.

Schema v21 adds default-disabled, Go-internal Specialist admission. An explicit policy allows at most two depth-one children. Every child must receive a nonempty subset of its parent's Skills, a dedicated active Session, and positive turn/token reservations that leave at least one root turn. The Session, child, root capacity/effective budget, hashed idempotency fact, events, and graph snapshot commit in one transaction, and concurrent retries converge on one child. Reserved capacity reduces later root Supervisor turn and model-token limits. Run pause/resume only affects children waiting for Run lifecycle reasons, while Run completion/failure/cancellation terminates children and archives their Sessions. Child model loops, public spawn APIs, and model-driven child creation remain disabled.

Schema v22 adds optional, validated `owner_agent_id` references to WorkItems and Notes. Both Store checks and SQLite triggers require the Agent to exist in the same Run, and new ownership cannot be assigned to a terminal Agent. Cross-Run spoofing fails before commit. Historical rows and legacy `owner` labels remain readable; owner-only Agent Notes mirror a compatibility label while visibility is evaluated against the real Agent identity. The Go control plane injects the calling Agent for Supervisor and `tool invoke` mutations, while model-facing JSON schemas never expose `owner_agent_id`. CLI, TUI, HTTP/OpenAPI display and filter the binding, and v21 databases upgrade in place without losing legacy labels.

Schema v23 adds the strict `agent_completion.v1` CompletionReport and internal `agent.finish` commit path. Reports accept only `succeeded`/`partial`, a bounded redacted summary, and references to child-owned WorkItems or parent-visible Notes. Successful reports cannot leave active child WorkItems, while partial reports must hand off every active item explicitly. The report, hashed idempotency fact, parent result inbox entry, child terminal state, Session archival, audit events, and graph snapshot commit in one transaction bound to the exact active attempt. Concurrent retries create one report, and stale attempts cannot overwrite it. Child model loops, public finish/spawn APIs, and model-driven child creation remain disabled.

Schema v24 adds a default-disabled, Go-internal Specialist Attempt Runtime. Every child turn is fenced by the current Run execution lease and generation, charges one turn when scheduling begins, records model usage exactly once, and accumulates actual tokens against the child budget. `continued`, `finished`, `crashed`, and `interrupted` attempts are immutable terminal facts. Crashes send a redacted retry decision to the parent inbox and archive the child Session when its budget is exhausted. A replacement worker recovers stale attempts exactly once after lease takeover, while the former worker can no longer commit new usage, completion, or failure state. Run pause or termination interrupts the attempt before moving the child projection, and graph recovery verifies turn order, token totals, CompletionReports, and snapshots. Public CLI/HTTP/model spawn, the child model loop, and any additional Shell or network authority remain disabled.

The project also includes its first local HTTP control plane. `cyberagent api serve` binds only to loopback. `CYBERAGENT_API_TOKEN` authorizes reads, while the default-disabled cancellation route requires a distinct `CYBERAGENT_API_CONTROL_TOKEN`. Stable `api.v1` envelopes and bounded cursor pagination expose durable metadata without fencing tokens. The sole write route can only request cancellation of the current model call: it cannot execute tools, alter Policy, accept a client fencing token, or read Artifact content/checkpoint pending input. The API has no CORS or WebSocket support and never persists either token.

The Go response DTOs now generate the single OpenAPI 3.1 contract. `cyberagent api openapi` emits deterministic JSON, `--output docs/openapi.json` refreshes the tested repository snapshot, and an authenticated running server returns the same raw document at `/api/v1/openapi.json`. Contract tests exercise every published route and prevent drift among DTOs, the committed document, and handlers while explicitly excluding Artifact content, checkpoint pending input, `lease_id`, and fencing tokens.

A read-only Run Event Stream now shares that control plane. `/api/v1/runs/{run_id}/events/stream` uses SSE to publish redacted durable SQLite events. Every frame id is an opaque Run-bound cursor that supports exact `Last-Event-ID` resume. Batch size, frame size, total events, lifetime, write timeout, and process concurrency are bounded; heartbeats never fabricate Run events, and server shutdown cancels long-lived connections. The stream contains no user-visible model text, accepts no token query, and performs no write operation itself.

The Bubble Tea TUI now exposes the current Run state, Work Board, Notes, durable Supervisor Tool Rounds, and Shell ToolRuns. Its tool pane supports “approve once” and “approve for this session.” Session approval creates only a revocable Grant scoped to the exact Run, Session, Workspace, Shell tool, and ActionClass, then rechecks the durable approval scope and current Policy before advancing the proposal. Later safe Shell calls may complete as dry runs, while dangerous commands remain permanently denied and cannot create a Grant. Terminal wrapping and truncation account for CJK, combining characters, and other wide graphemes.

## 核心能力 / Core Capabilities

- **可恢复运行 / Resumable runs:** durable checkpoints, cross-process execution leases with heartbeat/fencing, exact cross-process active-call cancellation, bounded execution, restart recovery, graceful terminal cancellation, and explicit lifecycle actions.
- **Agent Coordinator:** stable root identity, atomic Run/Supervisor status projection, bounded durable inboxes, hashed exactly-once send/finish intent, explicit wake/dependency semantics, internal-only two-child admission, lease-fenced child Attempt scheduling and usage accounting, crash/takeover recovery, attempt-bound CompletionReports, lifecycle cascade, and restart-validated graph snapshots; the child model loop remains disabled.
- **统一模型网关 / Model gateway:** route-based providers, Anthropic-compatible SSE/tool protocol, typed failures, application-owned active-call cancellation, bounded live progress, one lifecycle-protocol repair, and durable model/tool events.
- **长上下文与结构化记忆 / Long-context memory:** persisted sessions, automatic compaction, durable categorized Notes, visibility rules, and token-budgeted source selection.
- **结构化任务板 / Structured Work Board:** Run-scoped work items, dependency and cycle checks, optimistic versions, transactional events, and bounded Supervisor context.
- **本地工作区 / Local workspace:** scoped file access, safe reads, hashed Run artifacts, and reviewable edit proposals.
- **统一工具网关 / Unified Tool Gateway:** normalized calls, trusted workspace binding, policy decisions, shared review, first-class non-executable `script_process.v1` proposals, a bounded resumable Provider loop for create-only WorkItem/Note tools, bounded UTF-8 results, MIME metadata, and compatibility adapters.
- **安全与审批 / Safety and approval:** policy checks, secret redaction, automatic low-risk reads, durable per-call approvals, revocable scoped Session grants, atomic Run tool budgets, permanent denial, and dry-run command completion.
- **完整审计链 / Audit trail:** append-only Run events plus a bounded resumable SSE projection for messages, context source provenance, text-free model progress, model calls, Notes, policy decisions, tool proposals, file edits, and content-free Artifact metadata.
- **CLI 与 TUI / CLI and TUI:** a scriptable CLI plus a Bubble Tea interface with live model progress, audited cancellation, Run memory/tool-round views, and scoped once/session approvals.
- **可扩展架构 / Extensible architecture:** Go control plane with a generated OpenAPI 3.1 contract, loopback-only read API/SSE plus a separately authorized cancellation capability, and planned TypeScript UI, Docker sandbox, and Rust analyzer boundaries.

> [!NOTE]
> 当前版本仍在积极开发中。Provider 只能自动创建 WorkItem/Note；真实 Shell/Docker 执行、更广工具面、子 Agent 并发、Web UI 和 CTF 自动求解尚未开放。<br>
> This project is under active development. Providers may only create WorkItems and Notes; real Shell/Docker execution, broader model tools, concurrent child agents, the Web UI, and automated CTF solving are not enabled yet.

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
go run ./cmd/cyberagent run graph <run-id>
go run ./cmd/cyberagent run lease <run-id>
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
go run ./cmd/cyberagent api openapi
go run ./cmd/cyberagent api openapi --output docs/openapi.json
curl.exe -N -H "Authorization: Bearer $env:CYBERAGENT_API_TOKEN" http://127.0.0.1:8765/api/v1/runs/<run-id>/events/stream
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

`api serve` generates and prints a temporary read token when `CYBERAGENT_API_TOKEN` is absent. Cross-process cancellation remains disabled unless a distinct `CYBERAGENT_API_CONTROL_TOKEN` is supplied; environment-provided tokens are validated but never echoed. See [docs/http-api.md](docs/http-api.md) for endpoints, envelopes, pagination, and security boundaries.

## Project Memory

Read [docs/PROJECT_STATUS.md](docs/PROJECT_STATUS.md), [docs/PROGRESS_BOOK.md](docs/PROGRESS_BOOK.md), [docs/TASK_BOOK.md](docs/TASK_BOOK.md), [docs/http-api.md](docs/http-api.md), [docs/openapi.json](docs/openapi.json), [docs/errors.md](docs/errors.md), [ADR 0001](docs/adr/0001-go-control-plane.md), and [ADR 0002](docs/adr/0002-run-centric-runtime.md) first when resuming development after a long conversation. They record current progress, language ownership, run architecture, API and error contracts, audit notes, verified commands, and the recommended next slice.

## Repository Workflow

The canonical remote is [Qiyuanqiii/CTF-CyberAgent-Workbench](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench). Each completed development slice ends with tests, a focused code/security audit, project-memory updates, a Git commit, and a push to GitHub. This repository currently develops directly on `main`; do not create a feature branch or pull request unless the user explicitly asks for one.

Local runtime databases, workspace data, environment files, API keys, IDE metadata, and build output are excluded from Git.

## 许可证 / License

本项目由仓库所有者以 [Apache License 2.0](LICENSE) 授权。你可以在许可证条件下使用、修改和分发代码；分发时请保留许可证与所需声明。Apache-2.0 同时包含明确的专利许可与商标限制，完整法律条款以仓库 `LICENSE` 文件为准。

This project is licensed by the repository owner under the [Apache License 2.0](LICENSE). You may use, modify, and distribute the code subject to its terms, including preservation of the license and required notices. Apache-2.0 also provides an express patent grant and trademark limitations; the repository `LICENSE` file is the authoritative legal text.

## Development Priority

The current priority is the V2 run-centric runtime. P0 and P1 are complete. P2 supports resumable root Agent turns, cumulative token/model-time accounting, bounded execution and Provider retry loops, strict Supervisor-owned `continue`, `finish`, and `wait` actions, one Run execution path for ordinary CLI/TUI Session chat, real Provider streaming with bounded `model.delta` progress, local and schema v18 cross-process active-call cancellation, Bubble Tea live metadata, durable model events, exactly one restart-safe lifecycle-protocol repair, the schema v16 bounded structured-memory tool loop, and schema v17 execution leases with heartbeat/fencing. P3 includes migration v9 WorkItems, migration v10 Notes, transactional relationships/events, `todo` and `note` CLI lifecycles, root/owner visibility, token-budgeted memory selection, and durable context provenance. P4 now has the schema v19 single-root Coordinator, schema v20 idempotent inbox protocol, schema v21 internal-only Specialist admission, schema v22 same-Run Agent-owned WorkItems/Notes, schema v23 attempt-bound CompletionReports, and schema v24 lease-fenced Specialist Attempt scheduling, usage accounting, crash notification, and takeover recovery; the child model loop and root inbox context projection are still disabled. P5 includes the unified Tool Gateway, trusted workspace scope binding, schema v11 durable per-call approvals, schema v12 revocable Session grants and atomic Run tool-call accounting, schema v13 first-class typed script processes, schema v14 source-bound hashed output Artifacts, schema v15 idempotent structured-memory mutations, and v16 durable Provider tool rounds. P9 now includes the authenticated loopback-only `api.v1` read surface, a distinct cancellation-control capability, token-free lease status, a deterministic Go-generated OpenAPI 3.1 contract, and bounded resumable Run-event SSE; TypeScript remains pending. Real Local/Docker command execution and all non-memory model tools remain disabled. CTF-specific solving logic stays deferred until the generic runtime is stable.

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
