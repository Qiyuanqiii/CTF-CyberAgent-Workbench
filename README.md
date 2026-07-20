# CyberAgent Workbench

> 本地优先、可恢复、可审计的通用 AI Agent 工作台<br>
> A local-first, resumable, and auditable workbench for general-purpose AI agents.

## 完成度口径 / Progress Metrics

项目从 schema v49 起同时使用两项工程指标，避免把“架构已经搭好”误解为“产品已经完整可用”。这些百分比是基于当前任务书和可验证工作流的工程估算，不是性能基准。

- **架构完成度 / Architecture completion：约 99%**。衡量 Go 控制平面、Run/Session、状态恢复、Policy、审批、预算、事件流、Tool Gateway、Agent 协调、Skills、报告、Sandbox 协议及 Go/TypeScript 边界的覆盖程度；其中 V2 Run-centric Runtime 约 99%。schema v84 已覆盖同步等待环检测、工具硬超时、Run 活锁熔断、模型总上下文闸门、不可变累计交接记忆、验证快照回执历史和不授权的回执元数据复核。
- **产品可用度 / Product usability：约 95-97%**。衡量普通用户能否依靠当前 CLI、TUI、Web 和 Windows Desktop 完成真实端到端工作。通用 Coding Agent 工作流约 95-96%，Cyber 自动化工作流约 20%；Run 工作台现已提供安全恢复的 Monaco 提案/Diff 编辑器、只读仓库状态/脱敏 Diff/本地提交与可导航的精确文件历史、可连续导航的成对 base/head 精确预览、多文件独立审阅、分离的验证计划/结果、可分页的逐检查项证据下钻及 Markdown/JSON 快照下载/回执历史/不授权复核、带摘要和事件高水位的 Code Handoff 导出、Code Journey、可热重载的 Windows 系统凭证和有界 wake worker。真实 Sandbox/宿主进程执行、安装时脚本/钩子、Windows 10 人工发布矩阵和 Cyber 工具链仍未开放或完成。

Starting with schema v49, the project reports two engineering indicators so architectural maturity is not mistaken for end-user completeness. These percentages are roadmap estimates backed by tested workflows, not performance benchmarks.

- **Architecture completion: about 99%.** This covers the Go control plane and its Run/Session recovery, Policy, approval, budget, event, Tool Gateway, Agent coordination, Skills, reporting, Sandbox protocol, and Go/TypeScript boundaries. The V2 run-centric runtime itself is about 99% complete. Schema v84 includes synchronous wait-cycle rejection, hard Tool deadlines, a recoverable Run livelock circuit breaker, an aggregate model-context gate, immutable cumulative handoff memory, verification snapshot receipt history, and non-authorizing receipt-metadata review.
- **Product usability: about 95-97%.** This measures how much real end-to-end work a user can complete through the current CLI, TUI, Web, and Windows Desktop shell. The generic coding-agent workflow is about 95-96% usable and the Cyber automation workflow about 20%. The Run workbench now includes safely recoverable Monaco proposal/Diff editing, read-only repository state/redacted Diffs/local commit and navigable exact-file history, navigable paired exact base/head comparison previews, independent multi-file review, separate verification plans/outcomes with paginated per-check drilldown and Markdown/JSON snapshot downloads/receipt history/non-authorizing review, digest- and event-high-water-bound Code Handoff exports, the Code Journey, hot-reloaded Windows system credentials, and a bounded wake worker. Real Sandbox/host-process execution, install-time scripts/hooks, the manual Windows 10 release matrix, and the Cyber toolchain remain disabled or unfinished.

## 项目简介 / Project Overview

### 中文

CyberAgent Workbench 是一个由 Go 驱动的本地 AI Agent 工作台，面向代码开发、代码审查、安全学习、脚本任务和受控网络安全分析。它把模型调用、长上下文、工作区文件、策略检查、审批、执行预算和事件记录统一到一套可恢复的运行时中，让一次任务即使在程序退出后也能继续，并且每一步都可以追踪和复核。

每个用户目标会被记录为一个 `Mission`，每次可恢复的执行过程则是一个 `Run`。Go 是唯一控制平面，负责模型路由、状态机、SQLite 持久化、安全策略和工具边界；当前 TypeScript read-mostly 控制台与未来的 Rust 确定性分析器都通过 Go 协议接入，不绕过安全控制。

项目当前优先完善通用 Agent 运行时及其受控多 Agent 内核。CTF 将作为后续 Profile 和 Skills 能力接入，而不是另建一套独立运行系统。

### English

CyberAgent Workbench is a local AI agent workbench powered by Go for coding, code review, security learning, scripting, and controlled cybersecurity analysis. It unifies model calls, long-context memory, workspace files, policy checks, approvals, execution budgets, and event history in one resumable and auditable runtime.

Each user objective is stored as a `Mission`, while each resumable execution attempt is a `Run`. Go is the sole control plane for model routing, state machines, SQLite persistence, safety policy, and tool boundaries. The TypeScript console and future deterministic Rust analyzers connect through Go-defined protocols instead of bypassing those controls.

The current priority is the general-purpose Agent runtime and its controlled multi-agent kernel. CTF capabilities will be added later as Profiles and Skills on the same foundation rather than as a separate execution system.

## 开发历程 / Development History

下表是唯一按时间排序的 schema 开发历程，完整保留了早期 `v1`、`v2`、`v3`，并连续列到当前 `v84`。这里的 `vN` 是不可变 SQLite schema/runtime 里程碑，不等同于产品发布版本；后面的架构说明按能力域组织，因此不再承担版本排序职责。

The table below is the canonical chronological schema history. It includes every immutable SQLite schema/runtime milestone from `v1` through the current `v84`. These schema numbers are not product release versions; the architecture notes that follow are grouped by capability instead of chronology.

| Schema | 中文里程碑 | English milestone |
| --- | --- | --- |
| v1 | v0.1 基线存储 | v0.1 baseline |
| v2 | Mission/Run 中心化基础 | run-centric foundation |
| v3 | Run 与 Session 投影 | run session projection |
| v4 | 旧 Task 到 Run 的兼容映射 | legacy task run mapping |
| v5 | Supervisor 检查点 | supervisor checkpoints |
| v6 | Supervisor 预算账本 | supervisor budget ledger |
| v7 | Supervisor 待处理输入 | supervisor pending input |
| v8 | Supervisor 协议修复 | supervisor protocol repair |
| v9 | Run 工作看板 | run work board |
| v10 | Run Notes 结构化记忆 | run notes |
| v11 | 持久化工具审批 | durable tool approvals |
| v12 | Session Grant 与工具预算 | session grants and tool budgets |
| v13 | 类型化脚本进程提案 | typed script process proposals |
| v14 | Run 工具输出 Artifact | run tool output artifacts |
| v15 | 结构化记忆工具操作 | structured memory tool operations |
| v16 | Supervisor 结构化工具循环 | supervisor structured tool loop |
| v17 | Run execution lease | run execution leases |
| v18 | 跨进程模型取消 | cross-process model cancellation |
| v19 | 单 root Agent Coordinator | single-root agent coordinator |
| v20 | 幂等 Agent inbox 协议 | idempotent agent inbox protocol |
| v21 | 有界 Specialist 准入 | bounded specialist admission |
| v22 | Agent 归属的工作记忆 | agent-owned work memory |
| v23 | Specialist 完成报告 | specialist completion reports |
| v24 | 受 lease 保护的 Specialist Attempt | leased specialist attempts |
| v25 | root inbox 上下文交付 | root inbox context delivery |
| v26 | Specialist 模型调用账本 | specialist model call ledger |
| v27 | Specialist 上下文交付 | specialist context delivery |
| v28 | Specialist 协议修复 | specialist protocol repair |
| v29 | Specialist 调度与取消控制 | specialist schedule and cancellation control |
| v30 | 审阅门禁的 Specialist 委派提案 | review-gated specialist delegation proposals |
| v31 | 不可变 Specialist 委派审阅 | immutable specialist delegation reviews |
| v32 | 可恢复 Specialist 委派应用 | recoverable specialist delegation application |
| v33 | 不可变只读 Fan-out 计划 | immutable read-only fan-out plans |
| v34 | 有界只读 Fan-out 执行 | bounded read-only fan-out execution |
| v35 | 确定性 Finding 报告投影 | deterministic finding report projection |
| v36 | Artifact 支撑的 Finding 验证 | Artifact-backed finding validation |
| v37 | Finding 接受、修复生命周期 | accepted and fixed finding remediation lifecycle |
| v38 | 操作者控制的 Specialist 调度 | operator-controlled Specialist scheduling |
| v39 | 不可变 Run Skill 选择 | immutable Run Skill selection |
| v40 | root Skill 上下文来源 | root Skill context provenance |
| v41 | 不可变 Run 执行模式 | immutable Run execution mode |
| v42 | 审阅门禁的 Plan/Delivery 工作流 | review-gated Plan Delivery workflow |
| v43 | 不可变 Session 上下文来源 | immutable session context provenance |
| v44 | 不可变 Delivery 检查点门禁 | immutable Delivery checkpoint gates |
| v45 | 持久化操作者引导队列 | durable operator steering queue |
| v46 | 操作者引导队列控制 | operator steering queue controls |
| v47 | 最小化 Specialist Skill 上下文 | minimal Specialist Skill context |
| v48 | Go 主控 Sandbox Manifest 准备 | Go-owned Sandbox Manifest preparation |
| v49 | Sandbox 审批与禁用执行候选 | sandbox approval and disabled execution candidates |
| v50 | 禁用态 Sandbox 生命周期与 Artifact 绑定 | disabled Sandbox lifecycle and Artifact bindings |
| v51 | Sandbox 后端与输出禁用态预检 | disabled Sandbox backend and output preflight |
| v52 | 仅模拟的 Sandbox 后端证据与输出事务 | simulation-only Sandbox backend evidence and output transaction |
| v53 | 只读 Docker 生产环境观测 | read-only Docker production observation |
| v54 | 确定性 Docker 容器计划与假写事务 | deterministic Docker container plans and fake write transactions |
| v55 | 有界 Docker 创建、核验、删除演练 | bounded Docker create-inspect-remove rehearsals |
| v56 | 可恢复 Docker 演练意图、代际租约与检查矩阵 | recoverable Docker rehearsal intents, generation leases, and control matrix |
| v57 | 描述符固定与内核密封的宿主输入演练 | descriptor-pinned and kernel-sealed host-input rehearsal |
| v58 | daemon stage 前持久化宿主输入要求 | durable pre-stage host-input requirement |
| v59 | daemon 托管、回读核验的不可变宿主输入交接 | daemon-owned, readback-verified immutable host-input handoff |
| v60 | 确定性 Docker 运行时输入投影计划 | deterministic Docker runtime input projection plan |
| v61 | 可恢复 Docker 运行时输入卷应用 | recoverable Docker runtime input application |
| v62 | 保留运行时输入资源检查与精确清理 | retained runtime-input resource inspection and exact cleanup |
| v63 | 阻塞态 Docker 进程启动门设计审查 | blocked Docker process start-gate design review |
| v64 | 不可变 Run 执行环境档位选择 | immutable Run execution profile selection |
| v65 | 非授权 Docker 生产证据捕获账本 | non-authorizing Docker production evidence capture ledger |
| v66 | 可恢复 Docker 生产证据捕获 Attempt | recoverable Docker production-evidence capture attempts |
| v67 | Linux 只读 Docker 生产证据探针 | Linux read-only Docker production-evidence harness |
| v68 | 不可变 Docker 生产证据操作员审阅 | immutable Docker production-evidence operator review |
| v69 | 内容寻址惰性用户 Skill 安装账本 | content-addressed inert user Skill installation ledger |
| v70 | 外部 Skill 的 Run 固定选择与最小化上下文 | external-Skill Run selection and minimized context delivery |
| v71 | 有界外部 Skill 来源与交付只读投影 | bounded read-only external-Skill provenance and delivery projection |
| v72 | 幂等受控 Mission/Run/Session 创建账本 | idempotent controlled Mission/Run/Session creation ledger |
| v73 | 幂等 Run 生命周期与有界执行交接 | idempotent Run lifecycle and bounded execution handoff |
| v74 | 持久化 Run wake 重试意图与单一所有权 | durable Run wake retry intents and single-owner fencing |
| v75 | 显式前台 wake 消费与可恢复执行交接 | explicit foreground wake consumption and recoverable execution handoff |
| v76 | 已批准 FileEdit 的幂等独立 apply | idempotent independent apply for approved FileEdits |
| v77 | 非授权 Session 工作区证据挂载 | non-authorizing Session Workspace evidence attachments |
| v78 | 不可变操作者验证证据 | immutable operator verification evidence |
| v79 | 可恢复的 Run 无进展熔断 | recoverable Run livelock progress guard |
| v80 | 不可变操作者验证计划与检查清单 | immutable operator verification plans and checklists |
| v81 | 验证计划项与人工证据的不可变显式关联 | immutable explicit verification plan-item/evidence associations |
| v82 | 不可变累计上下文交接记忆 | immutable cumulative context handoff memory |
| v83 | 不可变验证快照回执历史 | immutable verification snapshot receipt history |
| v84 | 不可变且不授权的验证快照回执复核 | immutable non-authorizing verification snapshot receipt reviews |

## 上下文窗口与累计记忆 / Context Window And Cumulative Memory

默认模型上下文不是“无限历史”，也不是对远端模型规格的声明。Go 为 root Supervisor 和 Specialist 的每次真实模型请求采用保守的 `model_context_window.v1`：总窗口 32,768 tokens，安全余量 1,024，默认输出 1,024，单次输出上限 4,096。估算覆盖全部消息、ToolCall/ToolResult、工具说明和 JSON Schema；中日韩文字、emoji 等非 ASCII 内容按 UTF-8 字节保守计数。超限时只从最旧的普通 Session history 开始裁剪，系统控制、当前输入、结构化记忆、Skill 和工具 Schema 都不会被静默删除；强制上下文本身仍超限时，会在调用 Provider 前以 `RESOURCE_EXHAUSTED` 失败。Router 支持按精确 Provider/Model 设置窗口，但当前还没有面向用户的 CLI 配置入口。

会话压缩是另一层：活动消息超过 8 条时默认保留最近 4 条，其余内容写入最多 4,000 字符、最多 12 条记录的结构化交接摘要。schema v82 的 `handoff_memory.v1` 通过 previous-summary ID、内容 SHA-256、累计消息计数、单调 ordinal 和 Session 消息 ID 高水位形成 append-only 链；SQLite 拒绝更新、删除和过期分叉，读取时 Go 复核摘要与记录完整性。若进程在“摘要写入”与“消息标记 compacted”之间退出，重试会复用既有摘要并只吸收高水位之后的新消息。旧摘要以 `handoff_memory.v0` 保留，并在下一次压缩时作为无指令权限的历史证据折叠进 v1。

压缩后的用户意图只有在来源明确为 `operator_message` 时才保留 `instruction_authorized=true`；README、仓库文件、工具结果、模型输出和旧摘要仍是 user-role 的非可信证据。当前实现是确定性的提取式交接，不会让模型自动修改或自动重载任意 `AGENTS.md`、README 或项目记忆文件；内置/显式选择的 Skills 和 SQLite 记忆由 Go 每轮重新组装。这样可以恢复工作进度，同时不把文档中的 Prompt Injection 变成控制指令。

The default is a local planning policy, not an unlimited transcript or a claim about a remote model's advertised specification. Each real root-Supervisor and Specialist request uses conservative `model_context_window.v1`: 32,768 total tokens, a 1,024-token safety margin, 1,024 default output tokens, and a 4,096 output cap. The estimate includes messages, tool calls/results, tool descriptions, and JSON schemas; non-ASCII text is conservatively counted by UTF-8 bytes. Only the oldest ordinary Session history may be removed. System control, current input, structured memory, Skills, and tool schemas remain mandatory, and an oversized mandatory request fails before the Provider call. Exact Provider/Model overrides exist in the Router, but there is not yet a user-facing CLI setting.

Conversation compaction is a separate layer. More than eight active messages triggers compaction with the newest four retained. Removed history enters a structured handoff capped at 4,000 characters and 12 retained records. Schema v82 `handoff_memory.v1` forms an append-only chain using the previous-summary ID, content SHA-256, cumulative counts, a monotonic ordinal, and a Session-message ID high-water. A crash between summary insertion and message marking reuses the existing summary and admits only messages beyond that high-water. SQLite rejects update, deletion, and stale forks, while Go verifies content and record integrity on read. Legacy `handoff_memory.v0` rows remain readable and are folded once as non-authoritative historical evidence.

Only provenance-confirmed `operator_message` records may retain `instruction_authorized=true`. Repository files, README text, tool results, model output, and prior summaries remain untrusted user-role evidence. This is deterministic extractive handoff memory; it does not let the model automatically rewrite or reload arbitrary `AGENTS.md`, README, or project-memory files. Go reassembles persisted memory and explicitly selected Skills on every turn without promoting document instructions into control authority.

## 死锁与活锁保护 / Deadlock And Livelock Protection

Go Tool Runtime 默认给每次调用 15 秒硬截止时间，允许在构建期收窄但不得超过 5 分钟；调用方取消、超时和 panic 都会返回稳定结果。内置文件读取会响应取消，并拒绝 FIFO、设备、socket 等非普通文件，避免在特殊文件上永久阻塞。硬截止时间不能强杀拒绝响应 `context` 的第三方 Go goroutine，因此外部 Tool 仍必须遵守取消契约；未来真实 Runner 还必须独立实现进程树终止。

有界进程级等待图记录 Agent、Tool、Retriever、Store、Runner、Model 和外部依赖的同步边，最多保留 4,096 个节点和 8,192 条边。新增边若形成直接或间接环路会立即失败；Tool/Retriever/Store/Runner 反向同步等待 Agent 永久禁止。root Supervisor、Specialist parent/child 和 Tool Gateway 已接入同一图，未来 RAG、Store callback 和 Runner adapter 必须沿用该协议。

schema v79 的 `run_progress_guard.v1` 只持久化结构化状态与动作的 SHA-256、计数和原因，不保存模型正文。连续三次完全相同的 `continue`，或六次没有可观察结构化进展的不同 `continue`，会在提交该 turn 的同一事务内把 Run 转为 `paused/waiting`，记录 `supervisor.livelock_detected`，并要求操作者显式 resume；重启和完成重放不会重复提交消息。真实 Local/Docker 进程仍关闭，因此进程句柄、端口和容器级死锁由后续 start/wait/TERM/KILL/orphan 门禁负责，而不是由 v79 虚假声明已经解决。

The Go Tool Runtime applies a 15-second hard deadline by default. Construction may narrow it but cannot exceed five minutes. Caller cancellation, timeout, and panic return stable results. Built-in file reads are cancellation-aware and reject FIFOs, devices, sockets, and other non-regular files. A deadline cannot forcibly terminate a third-party Go goroutine that ignores its context, so external Tools must still honor cancellation; future real Runners must separately terminate process trees.

A bounded process-wide wait graph tracks synchronous Agent, Tool, Retriever, Store, Runner, Model, and external dependencies, up to 4,096 nodes and 8,192 edges. A direct or indirect cycle is rejected before the edge is installed, and Tool/Retriever/Store/Runner callbacks that synchronously wait on an Agent are always forbidden. The root Supervisor, Specialist parent/child execution, and Tool Gateway share this graph; future RAG, Store callback, and Runner adapters must use the same protocol.

Schema v79 `run_progress_guard.v1` persists only structured-state/action SHA-256 fingerprints, counters, and reason codes, never model text. Three identical `continue` actions or six varying `continue` actions without observable structured progress atomically move the Run to `paused/waiting`, append `supervisor.livelock_detected`, and require an explicit operator resume. Restart and completion replay do not duplicate messages. Real Local/Docker processes remain disabled, so handle, port, and container deadlocks belong to the later start/wait/TERM/KILL/orphan gate rather than being falsely claimed as solved by v79.

## 执行环境档位 / Execution Profiles

schema v64 增加由 Go 主控的 `run_execution_profile.v1`。每个 Run 默认使用 `preview`，操作者只能在 Run 处于 `created` 或 `paused` 且没有活动 execution lease 时，在 `preview`、`docker`、`local` 三档之间显式切换。档位通过追加式 SQLite 快照、摘要化幂等操作和审计事件持久化；CLI 与使用独立 control token 的本地 Web 控制台读取和修改同一份状态。

档位选择只表达操作者意图，不等于执行许可。v64 把 `process_enabled`、`execution_authorized` 和 `capability_grant` 全部固定为 `false`：`preview` 映射到 `NoopRunner`；`docker` 仍要求独立的 Docker production start gate；`local` 仍要求尚未实现的 OS sandbox gate。TypeScript 只能提交档位 ID，不能指定 backend、文件系统范围、网络、审批策略或任何权限位；child Agent 也不能放宽父 Run 的档位。

Schema v64 adds the Go-owned `run_execution_profile.v1`. Every Run defaults to `preview`; an operator may select `preview`, `docker`, or `local` only while the Run is `created` or `paused` and has no active execution lease. Selection is persisted through append-only SQLite snapshots, digest-only idempotency operations, and audit events. The CLI and the local Web console, when connected with the distinct control token, operate on the same state.

Selection records intent, not permission. Schema v64 fixes `process_enabled`, `execution_authorized`, and `capability_grant` to `false`: `preview` maps to `NoopRunner`, `docker` still requires an independent Docker production start gate, and `local` still requires the unimplemented OS-sandbox gate. TypeScript submits only a profile ID and cannot choose the backend, filesystem/network scope, approval policy, or authority bits; child Agents cannot widen the parent Run profile.

实现提交 `8378419` 已通过最终本地全仓测试、race/static/security/frontend 门禁，以及 GitHub Actions run `29523634340`；远端 Go/Linux job 用时 3 分钟，TypeScript job 用时 26 秒。审计未发现未解决的高危或中危问题，且没有运行真实 Docker start 或宿主机进程。

Implementation commit `8378419` passed the final local full-suite, race, static, security, and frontend gates plus GitHub Actions run `29523634340`; the remote Go/Linux job completed in 3 minutes and TypeScript in 26 seconds. The audit found no unresolved high- or medium-severity issue and did not run a real Docker start or host process.

真实生产 bundle 托管复核随后发现并修复一项低风险兼容性缺陷：Vite 8 的 URL-safe 内容哈希可能以 `-` 结尾，旧校验器会把该字符误认作最后一个分隔符并拒绝合法资源。Go 现在向前寻找满足长度与 URL-safe 字符集的哈希段；主 bundle 测试直接使用该真实文件名，并通过 20 轮 race、全仓回归以及桌面/移动端浏览器检查。页面无横向溢出，档位切换后仍显示 `process_enabled=false` 与 `execution_authorized=false`。

A real production-bundle hosting check then found and fixed one low-risk compatibility defect: a Vite 8 URL-safe content hash may end in `-`, which the old validator mistook for the final separator and rejected. Go now searches backward for a bounded URL-safe digest segment; the primary bundle test uses the observed filename and passes 20 race repetitions, the full suite, and desktop/mobile browser checks. The page has no horizontal overflow, and profile changes still report `process_enabled=false` and `execution_authorized=false`.

## Docker 生产证据账本 / Docker Production Evidence Ledger

schema v65 增加不可变的 `sandbox_docker_production_evidence.v1`。Go 固定 v51 的 16 项 probe、suite 指纹、平台分类和证据摘要；CLI `docker-production-evidence-capture/captures/show` 只接受 v63 review、摘要化操作键、同一操作者身份和显式确认，不接受用户上传的结论、JSON 报告、socket、路径、容器 ID 或 daemon 原始响应。Windows 只会记录 `unsupported_platform`，Linux 未显式设置 `CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1` 时只会记录 `opt_in_required`；schema v67 才允许显式 opt-in 的 Linux 路径执行受限只读采集。

该账本是机器采集协议和恢复边界，不是启动许可证。v67 的 `capture_complete` 只表示五次固定 GET 已完成，并把 `production_verified_count` 固定为 `0`；`sufficient_for_start`、`start_gate_passed`、容器启动、进程执行、输出导出和 Artifact 提交仍全部为 `false`。schema v68 已增加独立的证据接纳/拒绝审阅，但接纳只确认这份有界元数据回执的范围，真实 start/wait/TERM/KILL/orphan 状态机仍是后续门禁。

Schema v65 adds immutable `sandbox_docker_production_evidence.v1` records. Go fixes the sixteen v51 probes, suite fingerprint, platform classes, and evidence digests. The `docker-production-evidence-capture/captures/show` CLI accepts only a v63 review, a digest-only operation identity, the same operator, and explicit confirmation; it accepts no user-supplied conclusions, JSON report, socket, path, container ID, or raw daemon response. Windows records only `unsupported_platform`, and Linux records `opt_in_required` unless `CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1` is explicitly set. Only schema v67 permits that opted-in Linux path to perform bounded read-only collection.

This ledger is a machine-capture and recovery protocol, not a start permit. A v67 `capture_complete` receipt means only that five fixed GETs completed; `production_verified_count` is fixed to zero, every check remains insufficient, and container-start, process, output-export, and Artifact-commit authority stay false. Schema v68 adds the independent acceptance/rejection review, but acceptance confirms only the bounded metadata receipt; the real start/wait/TERM/KILL/orphan state machine remains a separate release gate.

在 v65 交付时，采集器仍是零副作用门禁，Application 会拒绝任何 `capture_complete` 或 `real_daemon_contacted=true` 结果；v66 随后补上写前 ownership/recovery 边界，v67 才新增受限只读 daemon 实现。v65 最终本地门禁通过全仓普通/race 测试、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、21 项前端测试、production build、零漏洞 npm audit、OpenAPI 无漂移，以及凭据、禁止进程能力、运行产物和 diff 扫描。未发现未解决的高危或中危问题。

实现提交 `e97daf0` 已通过 GitHub Actions run `29532551701`；远端 Go/Linux job 用时 2 分 47 秒，TypeScript job 用时 20 秒。

At the v65 delivery point, the collector remained a zero-side-effect gate and the Application rejected every `capture_complete` or `real_daemon_contacted=true` result. Schema v66 then supplied durable write-ahead ownership and recovery; schema v67 adds the bounded read-only daemon implementation. The v65 final local gate passed the full ordinary/race suites, vet, zero-warning staticcheck, module verification/tidy diff, zero reachable vulnerabilities, 21 frontend tests, the production build, zero-vulnerability npm audit, OpenAPI drift checks, and credential, process-capability, runtime-artifact, and diff scans. No unresolved high- or medium-severity issue is known.

Implementation commit `e97daf0` passed GitHub Actions run `29532551701`; the remote Go/Linux job completed in 2m47s and TypeScript in 20s.

## Docker 生产证据 Attempt / Docker Production-Evidence Attempts

schema v66 在任何产品 collector 调用之前原子写入不可变 `sandbox_docker_production_evidence_attempt.v1` 意图、摘要化幂等操作和 generation 1 租约。每次执行还必须先持久化当前 generation 的 quiescent reconciliation checkpoint；失败只追加有界类型码并释放租约，已释放或过期的 attempt 只能由下一 generation 恢复，旧 worker 无法提交失败、checkpoint 或 evidence。CLI 增加 `docker-production-evidence-attempts`、`docker-production-evidence-attempt-show` 和 `docker-production-evidence-attempt-resume`，恢复仍要求显式机器采集确认，公开输出不包含 lease ID 或 owner。

新的 SQL 门禁要求每一条新 v65 evidence operation 都绑定一个先存在的 attempt、当前 generation reconciliation 和原子提交的 attempt result；旧 v65 evidence 保持可读且不会被迁移伪造 attempt。attempt 固定本机 Linux Unix endpoint、30 秒采集上限和有界租约，并把容器启动、进程执行、输出导出和 Artifact 提交权限全部固定为 `false`。v66 的 reconciliation 只证明 Go 已在 collector 前写入“零 daemon 读取、零已知资源”的恢复检查点，不代表已核验 Docker 生产资源；v67 在该边界后另行增加 daemon-aware 只读检查点。evidence acceptance、start/wait/TERM/KILL/orphan 和输出提交仍是后续独立门禁。

Schema v66 atomically stores an immutable `sandbox_docker_production_evidence_attempt.v1` intent, digest-only idempotency binding, and generation-one lease before any product collector call. Every execution must first persist a quiescent reconciliation checkpoint for its current generation. Failures append only bounded typed codes and release the lease; a released or expired attempt can resume only under the next generation, and a stale worker cannot commit a failure, checkpoint, or evidence. The CLI adds `docker-production-evidence-attempts`, `docker-production-evidence-attempt-show`, and `docker-production-evidence-attempt-resume`; resume still requires explicit machine-capture confirmation, and public output omits lease IDs and owners.

A new SQL gate requires every new v65 evidence operation to bind a pre-existing attempt, the current-generation reconciliation, and an atomically committed attempt result. Legacy v65 evidence remains readable and receives no fabricated attempt during migration. The attempt fixes the local Linux Unix endpoint, a 30-second capture bound, bounded leases, and false container-start, process, output-export, and Artifact-commit authority. The v66 reconciliation proves only that Go stored a zero-daemon-read, zero-known-resource recovery checkpoint before invoking the collector; schema v67 adds a separate daemon-aware read-only checkpoint behind that boundary. Evidence acceptance, the start/wait/TERM/KILL/orphan lifecycle, and output commit remain independent future gates. ADR 0028 records the v66 boundary.

v66 最终本地门禁在最终功能代码上通过：全仓普通/race 测试分别用时 206.9 秒和 230.3 秒；`go vet`、零告警 `staticcheck`、module verify/tidy diff、零可达漏洞 `govulncheck`、21 项前端测试、OpenAPI 无漂移、TypeScript/Vite production build、零漏洞 npm audit、50 份 Markdown 相对链接，以及凭据、运行产物、乱码、进程/start 入口和 diff 扫描均为绿色。Domain 50 轮、Store 10 轮、Application/CLI 各 5 轮及关键 race 5/3/3 轮通过。隔离真实二进制 smoke 只加载 mock Provider，在独立 `CYBERAGENT_HOME` 完成 v66 迁移和 Workspace 初始化；清理命令被宿主受保护删除策略拒绝后，临时目录留给系统自动回收，无需人工操作。

审计在发布前修复两项问题：v66 lease 行原先缺少不可删除触发器且直接 SQL release/takeover 时间线不够严格，这会破坏未来 collector 的 fencing/recovery 证明；现在 lease 不可删除，release 必须早于到期，下一 generation 不得早于上一 release/expiry。另修复 v65 capture 列表将位置参数后的 `--limit` 误判为多余参数的低风险 CLI 缺陷。当前没有已知未解决高危或中危问题；本轮没有运行真实 Docker daemon harness、容器 start、Shell 或宿主进程。

The final v66 local gate passed on the final functional code: the full ordinary and race suites completed in 206.9s and 230.3s. Vet, zero-warning staticcheck, module verification/tidy diff, zero reachable vulnerabilities, 21 frontend tests, OpenAPI drift, the TypeScript/Vite production build, zero-vulnerability npm audit, 50-file Markdown link validation, and credential, runtime-artifact, encoding, process/start-entry, and diff scans are green. Domain, Store, Application, and CLI stress runs passed at 50/10/5/5 iterations, with critical race repetitions at 5/3/3. An isolated real binary loaded only the mock Provider, migrated a separate `CYBERAGENT_HOME` to v66, and initialized a Workspace. The host protected-delete policy rejected recursive cleanup, so the temporary directory is left to normal OS cleanup and needs no manual action.

The pre-release audit fixed two issues. The v66 lease row originally lacked a delete guard and its direct-SQL release/takeover chronology was too permissive, weakening future collector fencing and recovery evidence; leases are now non-deletable, release must precede expiry, and a new generation cannot predate the previous release or expiry. A low-risk CLI defect that rejected `--limit` after the v65 capture-list positional argument was also fixed. No unresolved high- or medium-severity issue is known. This slice ran no real Docker daemon harness, container start, Shell, or host process.

实现提交 `3e52b7d` 已通过 GitHub Actions run `29538732903`；远端 Go/Linux 作业用时 3 分 33 秒，TypeScript 作业用时 25 秒。<br>
Implementation commit `3e52b7d` passed GitHub Actions run `29538732903`; the remote Go/Linux job completed in 3m33s and TypeScript in 25s.

## Linux 只读 Docker 证据探针 / Linux Read-Only Docker Evidence Harness

schema v67 在 v66 的 attempt、当前 generation lease 和零读取控制检查点全部持久化后，才允许显式 opt-in 的 Linux collector 接触固定 `/var/run/docker.sock`。Go 先写入不可变 harness intent，再用精确 attempt label 执行一次容器清单 GET；只有清单为空时才写入 daemon-aware reconciliation，随后依次执行 `_ping`、`version`、`info` 和精确已存在 image digest 的 inspect。总共最多五次 daemon GET，每次最多四秒，整体仍受 30 秒 attempt deadline 约束；不读取 `DOCKER_HOST`，不 pull，也没有 create/start/exec/remove/delete 方法。

采集结果只保存有界指纹、计数和状态，不保存 socket、daemon payload、容器/资源 ID、镜像仓库名、路径或私有 lease 身份。v67 的 16 项结果全部固定为 `observed_failed`、`production_verified_count=0`、`sufficient_for_start=false`；SQL 与 Go 同时禁止退回 v66 inert result、跨 generation reconciliation、修改终态或授予 daemon write/start/process/output/Artifact 权限。Windows 和未 opt-in 的 Linux 仍不会接触 daemon。本机为 Windows，因此本轮只运行模拟 transport、迁移和全仓门禁，没有执行真实 Docker、容器启动、Shell 或宿主进程。边界见 ADR 0029。

Schema v67 permits an explicitly opted-in Linux collector to contact the fixed `/var/run/docker.sock` only after the v66 attempt, current-generation lease, and zero-read control checkpoint are durable. Go first commits an immutable harness intent, performs one exact attempt-label container-list GET, and commits a daemon-aware reconciliation only if that owned scope is empty. It then calls `_ping`, `version`, `info`, and inspect for the exact already-present image digest. The protocol allows at most five daemon GETs, each bounded to four seconds and still constrained by the attempt's 30-second deadline. It ignores `DOCKER_HOST`, never pulls, and exposes no create/start/exec/remove/delete method.

The receipt stores only bounded fingerprints, counts, and state; it excludes sockets, daemon payloads, container/resource IDs, repository names, paths, and private lease identity. All sixteen v67 results are fixed to `observed_failed`, `production_verified_count=0`, and `sufficient_for_start=false`. Go and SQLite jointly prevent fallback to the v66 inert result, cross-generation reconciliation, terminal mutation, and every daemon-write/start/process/output/Artifact grant. Windows and non-opted-in Linux paths remain daemon-free. This Windows host therefore validates simulated transports, migration, and repository gates only; it runs no real Docker, container start, Shell, or host process. ADR 0029 records the boundary.

v67 最终本地门禁已通过：最终功能代码的全仓普通/race 测试分别用时 215.2 秒和 233.1 秒；`go vet`、零告警 `staticcheck`、module verify/tidy diff、零可达漏洞 `govulncheck`、9 个文件 21 项 Vitest、OpenAPI 生成代码无漂移、Vite production build 和零漏洞 npm audit 均为绿色。Sandbox/Store/Application/CLI 高频回归分别通过 50/10/5/5 轮普通测试及 10/3/3/3 轮 race；51 份 Markdown、仓库凭据/运行产物/乱码/Docker 写入入口和 diff 扫描通过，隔离真实二进制 Workspace smoke 与 Linux sandbox test binary 交叉编译通过。宿主受保护删除策略拒绝递归清理烟测目录，两个目录留在系统临时根等待自动回收，无需人工操作。

发布前审计把生产验证数收紧为只能为零，精确重算 label selector，交叉绑定 v66 control reconciliation，要求 evidence 时间位于租约到期前，并关闭了 direct-SQL 时间戳不一致可造成半终态的路径；CLI 和失败事件也改为只报告有证据支撑的 daemon contact 状态。当前没有已知未解决的高危或中危问题。可选真实 Linux daemon 集成测试仍未在本机执行，因而不会把 v67 receipt 误称为生产验证。

The final v67 local gate is green. Full ordinary and race suites completed in 215.2s and 233.1s; vet, zero-warning staticcheck, module verification/tidy diff, zero reachable vulnerabilities, 21 Vitest cases across nine files, generated OpenAPI drift checks, the Vite production build, and zero-vulnerability npm audit all passed. Sandbox, Store, Application, and CLI repetitions passed at 50/10/5/5 ordinary iterations and 10/3/3/3 race iterations. Validation also covered 51 Markdown files, repository privacy/artifact/encoding/Docker-mutation/diff scans, an isolated real-binary Workspace smoke, and Linux sandbox test-binary cross-compilation. Protected host deletion policy rejected recursive cleanup, so two smoke directories remain under the OS temporary root for normal cleanup and require no manual action.

The pre-release audit forced the production-verified count to exactly zero, recomputed the exact label selector, cross-bound the v66 control reconciliation, required evidence to precede lease expiry, and closed a direct-SQL timestamp mismatch that could otherwise leave a half-terminal record. CLI and failure events now report only evidence-backed daemon-contact state. No unresolved high- or medium-severity issue is known. The optional real Linux daemon integration test was not run on this host, so no v67 receipt is represented as production verification.

实现提交 `8bc0929` 已通过 GitHub Actions run `29543385038`；远端 Go/Linux 作业用时 2 分 50 秒，TypeScript 作业用时 24 秒。<br>
Implementation commit `8bc0929` passed GitHub Actions run `29543385038`; the remote Go/Linux job completed in 2m50s and TypeScript in 24s.

## 不可变生产证据审阅 / Immutable Production-Evidence Review

schema v68 增加不可变 `sandbox_docker_production_evidence_review.v1`。操作者必须对一份精确完成的 v67 harness receipt 显式确认，并在 `accepted|rejected` 中作出一次决定；接纳只能使用固定原因 `metadata_scope_accepted`，拒绝只能使用五个有界原因码，不接受自由文本、上传报告、原始 daemon payload、资源身份、路径或 socket。每份 evidence/attempt 最多一条决定，同一稳定操作键只允许同语义重放。

Store 在同一 SQLite 事务中先写摘要化 operation，再写 review 和 metadata-only Run event；延迟外键与双向 trigger 保证 operation 或 review 任意一半都不能单独提交。Go 与 SQL 重新绑定 v63 阻塞审查、v65 receipt、v66 attempt、v67 harness result、16 条 `observed_failed` item 及全部指纹。迁移不会给旧 receipt 或未完成 attempt 伪造审阅。

即使决定为 `accepted`，它也只设置 `receipt_accepted=true`：`production_verified_count=0`、`sufficient_check_count=0`、`blocker_count=16`，并且 start gate、容器启动、进程执行、输出导出和 Artifact 提交权限继续全部为 `false`。v68 不接触 Docker、不调用模型、不启动 Shell 或宿主进程。CLI 提供 `docker-production-evidence-review/reviews/review-show`；公开输出和事件只含有界元数据。边界见 ADR 0030。

Schema v68 adds immutable `sandbox_docker_production_evidence_review.v1`. An operator must explicitly confirm one exact completed v67 harness receipt and make one `accepted|rejected` decision. Acceptance permits only the fixed `metadata_scope_accepted` reason; rejection permits five bounded reason codes. Free-form narratives, uploaded reports, daemon payloads, resource identities, paths, and sockets are not request or storage fields. Each evidence/attempt receives at most one decision, and a stable operation key replays only identical semantics.

The Store commits a digest-only operation first, then the review and a metadata-only Run event in one SQLite transaction. A deferred foreign key and reciprocal triggers prevent either half from committing alone. Go and SQL rebind the v63 blocked review, v65 receipt, v66 attempt, v67 harness result, all sixteen `observed_failed` items, and the complete fingerprint chain. Migration fabricates no review for legacy receipts or incomplete attempts.

Even `accepted` means only `receipt_accepted=true`: production verification remains zero, sufficient checks remain zero, all sixteen blockers remain, and start-gate, container-start, process, output-export, and Artifact-commit authority remain false. Schema v68 contacts no Docker daemon, calls no model, and starts no Shell or host process. The CLI exposes `docker-production-evidence-review/reviews/review-show` with bounded metadata-only projections. ADR 0030 records this boundary.

v68 最终本地门禁已通过：全仓普通/race 测试分别用时 247.9 秒和 276.3 秒；`go vet`、零告警 `staticcheck`、module verify/tidy diff、零可达漏洞 `govulncheck`、9 个文件 21 项 Vitest、OpenAPI 无漂移、Vite production build 和零漏洞 npm audit 均为绿色。Domain/Store/Application/CLI 高频回归分别通过 50/10/5/5 轮普通测试和 10/3/3/3 轮 race；57 份 Markdown 的 74 条相对链接、用户测试 key、乱码、禁止执行入口和 diff 扫描通过，Linux test binary 交叉编译与隔离真实 CLI schema-v68 smoke 通过。

独立审计发现并修复两项中低风险审计健壮性缺口和一项低风险覆盖缺口：Store 现在读取/重放时重新计算已存 operation/review 语义绑定，SQL 也直接绑定两侧 request fingerprint；负向矩阵覆盖孤立 review、operation 更新/删除、来源/指纹/权限篡改和双 Store 并发收敛；`rejected` 决定已覆盖 Store、事件、Application、CLI、list/show 和重放。当前没有已知未解决的高危或中危问题。本机没有运行真实 Docker、容器启动、Shell 或宿主进程；受保护删除策略让一个交叉编译文件和一个 smoke 根留在系统临时目录等待系统回收，无需人工操作。GitHub Actions run `29552080990` 已通过实现提交 `41583ac`，Go/Linux 用时 2 分 57 秒，TypeScript 用时 24 秒。

GitHub Actions run `29552080990` passed implementation commit `41583ac`; Go/Linux completed in 2m57s and TypeScript in 24s. This remote proof does not widen v68's non-authorizing boundary.

The final v68 local gate is green. Full ordinary and race suites completed in 247.9s and 276.3s. Vet, zero-warning staticcheck, module verification/tidy diff, zero reachable vulnerabilities, 21 Vitest cases across nine files, OpenAPI drift checks, the Vite production build, and zero-vulnerability npm audit passed. Domain, Store, Application, and CLI repetitions passed at 50/10/5/5 ordinary iterations and 10/3/3/3 race iterations. Validation also covered 74 relative links across 57 Markdown files, user test-key exclusion, encoding, forbidden execution entries, diff hygiene, Linux test-binary cross-compilation, and an isolated real-CLI schema-v68 smoke.

The independent audit found and fixed two medium/low audit-robustness gaps and one low-risk coverage gap. Store reads and replays now recompute the stored operation/review semantic binding, while SQL directly binds both request fingerprints. Negative tests cover isolated reviews, operation mutation/deletion, source/fingerprint/authority tampering, and two-Store convergence. Rejected decisions now have Store, event, Application, CLI, list/show, and replay coverage. No unresolved high- or medium-severity issue is known. This host ran no real Docker, container start, Shell, or host process. Protected deletion left one cross-compiled binary and one smoke root under the OS temporary directory for normal cleanup; no manual action is required.

## 惰性用户 Skill Registry / Inert User Skill Registry

schema v69 增加内容寻址、不可变且默认不授权的用户 Skill Registry。`skill import` 只接收已通过 `skill_package.v1` 严格校验的本地 ZIP，并要求显式选择 `code|cyber` 工作面、稳定 operation key 与 `--confirm-untrusted-skill`。Go 先把安装意图和摘要化操作写入 SQLite，再将包发布到 `$CYBERAGENT_HOME/skill-registry/objects/sha256/...`，完整回读摘要、结构与语义指纹后才写入安装结果；同键重试可恢复中断导入，两个 SQLite 连接并发时收敛到同一记录。

外部包固定为 `operator_installed_untrusted`。导入不会执行正文、脚本、钩子或命令，不访问网络，不调用 Provider 或工具，也不授予 Run 选择、上下文注入或工具能力。Code 与 Cyber 用户目录分离；Cyber 第一版只接受精确的 `script` Profile，用户包不能覆盖五个内置 Skill。`skill installed`、`skill installed show` 与 `skill remove` 只投影有界元数据；移除追加不可变 tombstone，不删除内容对象，并在 Go 与 SQL 两层拒绝移除已被 Run 精确固定的版本。

v69 本身只完成安全存放与审计，不会因为安装而自动影响任何 Run。schema v70 在其上增加独立的显式 Run 选择与最小化加载协议；安装确认和 Run 上下文确认是两次不同的操作者决定。边界分别见 ADR 0031 与 ADR 0032。

Schema v69 adds a content-addressed, immutable, and non-authorizing user Skill Registry. `skill import` accepts only a local ZIP that passes strict `skill_package.v1` validation and requires an explicit `code|cyber` surface, stable operation key, and `--confirm-untrusted-skill`. Go commits the installation intent before publishing the archive below `$CYBERAGENT_HOME/skill-registry/objects/sha256/...`; it records completion only after full digest, structure, and semantic readback verification. Same-key retries recover interrupted imports, and concurrent SQLite connections converge on one record.

Every external package remains `operator_installed_untrusted`. Import executes no content, script, hook, or command; accesses no network; calls no Provider or tool; and grants no Run-selection, context-injection, or tool authority. Code and Cyber catalogs remain separate, with Cyber accepting only the exact `script` Profile in this first release. User packages cannot shadow the five built-ins. `skill installed`, `skill installed show`, and `skill remove` expose bounded metadata only. Removal appends an immutable tombstone, retains the object, and is rejected in both Go and SQL for an exact version already pinned by a Run.

Schema v69 itself is storage and audit, so installation alone can never affect a Run. Schema v70 adds a separately explicit Run-selection and minimized-load protocol on top; install confirmation and Run-context confirmation remain two distinct operator decisions. ADR 0031 and ADR 0032 record the two boundaries.

The final v69 local gate passed full ordinary/race suites in 259.7s/275.3s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, OpenAPI drift checks, 21 frontend tests, the production build, and zero-vulnerability npm audit. The audit fixed v69 downgrade-fixture ordering, static error style, redundant temporary cleanup state, forged object-receipt acceptance, cancellation immediately before object publication, concurrent requests with independently generated IDs/timestamps being misclassified as changed intent, and unredacted free-text Manifest descriptions being copied into SQLite metadata. The first Linux CI run `29556933994` then exposed concurrent nested-directory preparation through `os.Root.MkdirAll`; publication now creates and `Lstat`-verifies every directory component, rejecting symlinks, and the 12-Store regression passed 100 ordinary and 20 race repetitions. The real two-Service convergence regression passed 20 ordinary and 10 race repetitions. GitHub Actions run `29557803407` passed fix commit `d28b100` with Go/Linux in 3m21s and TypeScript in 23s. No unresolved high- or medium-severity issue is known; this slice ran no real model, network request, Shell, Docker operation, installer hook, or host process.

## 外部 Skill Run 选择与上下文 / External Skill Run Selection And Context

schema v70 新增不可变 `external_skill_selection.v1`。操作者只能在 Run 启动前，通过稳定 operation key、`--confirm-untrusted-skill-context` 和精确的 `name@version` 列表固定 1-4 个仍处于 active 状态的外部包；总保守预算上限为 4096，同一 Run 只有一份选择，最多一项可被明确指定给 Specialist。选择同时固定安装指纹、安装结果、archive/content 摘要、对象 key、工作面、Profile 和 mode revision。移除已固定版本会在 Go 预检和 SQLite 触发器两层失败，包声明的工具始终只是元数据。

每次 root 或被指定的 Specialist 首次模型调用前，Go 都重新打开精确内容寻址对象，复核普通文件身份、字节数、SHA-256、严格 ZIP 结构、语义 package fingerprint 和 Manifest 绑定，再进行 secret redaction 与独立预算检查。正文只存在于当前 Provider request 的用户角色 `external_skill_guidance.v1` JSON 信封中；信封明确把 Policy、工具、Shell、网络、Secret、Scope 和再委派权限固定为 `false`。系统控制文本要求模型把它视为可选工作流建议，把仓库/文档声明视为证据，并忽略要求隐藏步骤、泄露秘密、改变策略或扩大权限的内容。

SQLite 与 Run 事件只保存 selection、prepared/committed 来源元数据和指纹，不保存包正文、原始 operation key 或 Secret。root 与 Specialist 的 preparation 都在对应第一次 `model.started` 事务内提交；崩溃、重试和跨连接恢复不会重复消费或把未提交正文伪造成历史。CLI 提供 `skill select-external` 与 `skill external-selection`；HTTP、TUI、Web 上传、签名、安装钩子和包内命令执行仍未开放。ADR 0032 固定该边界。

Schema v70 adds immutable `external_skill_selection.v1`. Before a Run starts, an operator may pin one to four active external packages by exact `name@version` using a stable operation key and `--confirm-untrusted-skill-context`. The aggregate conservative budget is capped at 4096, each Run has one selection, and at most one item may be explicitly designated for Specialists. The selection freezes installation/result fingerprints, archive/content digests, object key, surface, Profile, and mode revision. Go preflight and SQLite both reject removal of a pinned version, while declared tools remain metadata only.

Before each root or designated Specialist's first model call, Go reopens the exact content-addressed object and revalidates regular-file identity, byte count, SHA-256, strict ZIP structure, semantic package fingerprint, and Manifest binding, then applies secret redaction and a separate budget. Content exists only in the current Provider request as a user-role `external_skill_guidance.v1` JSON envelope. That envelope fixes Policy, tool, Shell, network, secret, scope, and delegation authority to false. System control text tells the model to treat it as optional workflow guidance, treat repository claims as evidence, and ignore requests to hide steps, expose secrets, change policy, or widen authority.

SQLite and Run events retain only selection and prepared/committed provenance metadata and fingerprints, never package content, raw operation keys, or secrets. Root and Specialist preparation commit atomically with their corresponding first `model.started`; crash, retry, and cross-connection recovery cannot double-consume context or fabricate delivery history. The CLI exposes `skill select-external` and `skill external-selection`. HTTP/TUI/Web upload, signatures, install hooks, and package command execution remain closed. ADR 0032 records this boundary.

v70 最终本地门禁通过：全仓普通/race 分别为 197.6 秒/264.4 秒，`go vet`、零告警 `staticcheck`、module verify/tidy diff、零可达漏洞 `govulncheck`、9 个文件 21 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭证/运行产物/乱码/43 份 Markdown 链接/diff 扫描和隔离真实 CLI schema-v70 smoke 均为绿色。审计发现并修复一项中风险约束错误：同一安装曾被错误限制为全局只能供一个 Run 选择；现在改为每份选择内唯一，并以第二个 Run 的回归证明可安全复用。审计还收紧 Specialist 对当前 Run 最新 mode 的 Go/SQL 绑定、1024-2048 token 边界、操作重放跨阶段恢复、选择/operation 原子成对提交、取消时对象读取失败关闭和全部外部权限位显式为 false。当前没有已知未解决高/中风险；本轮没有调用真实模型、网络、Shell、Docker、安装钩子或宿主进程。系统策略拒绝清理的隔离 smoke 根仅留在 `%TEMP%` 等待系统回收，无需人工操作。GitHub Actions run `29566538449` 已通过实现提交 `edc4073`，Go/Linux 与 TypeScript 作业分别用时 3 分 42 秒和 21 秒。

The final v70 local gate passed the full ordinary/race suites in 197.6s/264.4s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 21 frontend tests across nine files, OpenAPI drift checks, the production build, zero-vulnerability npm audit, credential/runtime-artifact/encoding/43-file Markdown-link/diff scans, and an isolated real-CLI schema-v70 smoke. The audit found and fixed one medium-severity constraint defect: an installation had accidentally been globally limited to one Run; uniqueness is now scoped to each selection and a second-Run regression proves safe reuse. It also tightened latest-mode Go/SQL binding for Specialists, the 1024-2048 token boundary, cross-phase operation replay, atomic selection/operation pairing, cancellation-safe object reads, and explicit false authority fields. No unresolved high/medium issue is known. No real model, network, Shell, Docker, installer hook, or host process ran; the protected cleanup policy left only the isolated smoke root under `%TEMP%` for normal system cleanup, with no manual action required. GitHub Actions run `29566538449` passed implementation commit `edc4073` with Go/Linux in 3m42s and TypeScript in 21s.

## 外部 Skill 只读来源投影 / External Skill Read-Only Provenance

schema v71 通过两个 SQLite 只读 VIEW 从 v70 事实生成 `external_skill_projection.v1`，不回填选择、不追加事件，也不伪造上下文交付。投影最多包含四个名称/版本、信任类别、声明工具数量、单项与总 token 上界、原始 mode revision，以及 root/Specialist 的 prepared/committed 计数。其 Go 类型从结构上排除了包正文、路径、字节大小、所有摘要/指纹、selection/installation/mode snapshot ID、请求者、operation 和 attempt/agent 身份。

同一只读投影现在出现在 Run detail、独立 `GET /api/v1/runs/{run_id}/external-skills`、TUI `Skills` 页和 React Run 概览。HTTP 仍使用 read bearer；TUI 与 Web 没有安装、选择、执行或授权按钮，`tool_capability_grant` 继续为 `false`。v70 原地升级会自动读取既有事实，空 Run 保持无投影，SQLite VIEW 无写触发器且写入尝试失败。Web Skill 上传/安装仍未开放。

Schema v71 derives `external_skill_projection.v1` from v70 facts through two read-only SQLite views. It performs no selection backfill, appends no event, and fabricates no context delivery. The projection contains at most four names and versions, trust class, declared-tool count, per-item and aggregate token bounds, the originating mode revision, and root/Specialist prepared/committed counts. Its Go type structurally excludes package bodies, paths, byte sizes, every digest or fingerprint, selection/installation/mode-snapshot IDs, requester and operation identities, and attempt or Agent identities.

The same projection is now available in Run detail, dedicated `GET /api/v1/runs/{run_id}/external-skills`, the TUI `Skills` view, and the React Run overview. HTTP remains under the read bearer. TUI and Web expose no install, select, execute, or authority control, and `tool_capability_grant` remains false. Existing v70 facts become visible after in-place upgrade, Runs without a selection stay empty, and writes to the SQLite views fail. Web Skill upload and installation remain closed.

v71 最终本地门禁通过：全仓普通/race 分别为 227.1 秒/301.1 秒，`go vet`、零告警 `staticcheck`、module verify/tidy diff、零可达漏洞 `govulncheck`、确定性 OpenAPI/TypeScript 生成、9 个文件 22 项前端测试、production build、零漏洞 npm audit、凭据/运行产物/生产 `exec.Command`/乱码/54 份 Markdown 与 78 条相对链接/diff 扫描和隔离真实 CLI schema-v71 smoke 均为绿色。审计把四个上下文计数查询显式绑定到同一 Run，并增加“有效 Run 无外部选择时 detail 省略字段且 endpoint 返回 404”的普通/race 回归；当前没有已知未解决高/中风险。本轮没有执行真实 Provider、Agent-controlled Shell/宿主进程、Docker、安装钩子或外部网络请求；smoke 根仅留在系统 `%TEMP%` 等待正常回收，无需人工操作。GitHub Actions run `29574167659` 已通过实现提交 `3947bea`，Go/Linux 2 分 56 秒，TypeScript 25 秒。

The final v71 local gate passed the full ordinary/race suites in 227.1s/301.1s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, deterministic OpenAPI/TypeScript generation, 22 frontend tests across nine files, the production build, zero-vulnerability npm audit, credential/runtime-artifact/production-`exec.Command`/encoding/54-file and 78-relative-link Markdown/diff scans, and an isolated real-CLI schema-v71 smoke. The audit explicitly bound all four context-count queries to the same Run and added ordinary/race coverage proving that a valid Run without an external selection omits the detail field and receives a 404 from the dedicated endpoint. No unresolved high/medium issue is known. No real Provider, Agent-controlled Shell/host process, Docker operation, installer hook, or external network request ran; the smoke root remains only under the OS temporary directory for normal cleanup and needs no user action. GitHub Actions run `29574167659` passed implementation commit `3947bea` with Go/Linux in 2m56s and TypeScript in 25s.

## 受控 Run 创建 / Controlled Run Creation

schema v72 完成 Desktop D1-R1 的第一条业务写路径。新的不可变 `run_creation.v1` operation 账本只保存域分隔幂等键摘要、请求指纹和 Mission/Run/Session/Workspace 身份；一次事务会创建新 Mission、交互式 Run、活动 Session、初始 `code|cyber`/`plan|deliver` 模式、`preview/noop` 执行档位、root Agent 与精确初始事件。Workspace 必须已注册，目标在脱敏后持久化且原始输入最多 4096 UTF-8 字节，网络固定禁用、目标列表固定为空、预算固定为默认值，模型路由固定为所选 Profile。

`POST /api/v1/runs` 使用独立 control bearer 和恰好一个 `Idempotency-Key`；严格拒绝未知/重复字段、尾随 JSON、query、错误 Content-Type 和超限 body。相同 key 与相同语义跨重启或并发重放会返回原 Mission/Run/Session，不同语义冲突。`GET /api/v1/workspaces` 只返回 Workspace ID、名称和创建时间，不公开宿主路径。React 的 New Run 对话框只在 `run_creation_enabled` 为真时出现，令牌和失败重试 key 仅驻留内存；Go 响应还会在前端按 Workspace/Profile/模式、默认预算和关闭权限重新校验。

Schema v72 completes the first Desktop D1-R1 business mutation. The immutable `run_creation.v1` operation ledger stores only a domain-separated idempotency-key digest, a request fingerprint, and Mission/Run/Session/Workspace identities. One transaction creates a Mission, interactive Run, active Session, initial `code|cyber` and `plan|deliver` mode, `preview/noop` execution profile, root Agent, and the exact initial event set. The Workspace must already be registered; the goal is redacted before persistence and its raw input is capped at 4096 UTF-8 bytes. Network access and target lists stay disabled, the default budget is fixed, and model routing is fixed to the selected Profile.

`POST /api/v1/runs` requires the distinct control bearer and exactly one `Idempotency-Key`, rejecting unknown or duplicate fields, trailing JSON, query data, the wrong content type, and oversized bodies. Same-key/same-intent replay converges on the original Mission/Run/Session across restart or concurrent SQLite connections; changed intent conflicts. `GET /api/v1/workspaces` exposes only Workspace ID, name, and creation time, never the host path. React shows the New Run dialog only when `run_creation_enabled` is true, retains the bearer and uncertain-failure retry key in memory only, and revalidates the returned Workspace/Profile/mode, default budget, and closed authority contract.

## Run 生命周期与执行交接 / Run Lifecycle and Execution Handoff

schema v73 把 D1-L1 与 D1-X1 固定为两份相互独立的 Go 协议。`run_lifecycle_control.v1` 通过不可变摘要幂等账本执行 `created -> preparing -> running`、`running -> paused` 和 `paused -> running`；暂停要求没有活动 execution lease、活动 Agent、未完成 Supervisor turn 或 prepared steering delivery。跨进程同键请求只追加一次状态事件，延迟重放会返回原操作事实和 Run 当前状态。

`run_execution_handoff.v1` 在取得私有 Run execution lease 之前，按 sequence 冻结同一 Run/Session 最多八条 pending steering 身份。它只调用现有 `RunSupervisor`、Policy、预算、模型账本和工具门禁，逐条处理该冻结集合；执行期间新追加的消息不会越界进入本批，已取消的冻结消息会跳过，结果以 lease generation 围栏持久化。浏览器只能看到计数、状态、stop reason 以及是否发生模型/工具调用，不能看到正文、模型输出、工具参数、operation key 或 lease 身份。

D1-S2 另以非 schema 的 `session_steering_cancellation.v1` 开放精确 pending-only 取消；prepared、committed 或已取消消息均不能借此改写。三个入口分别使用独立 Desktop capability 和同一 distinct control bearer，默认关闭；成功不授予 Shell、Docker、LocalRunner、网络、文件写入、工具审批或 child 调度权限。当前执行交接是用户显式触发的有界批次，不是后台自动 wake/retry scheduler。

Schema v73 fixes D1-L1 and D1-X1 as two independent Go protocols. `run_lifecycle_control.v1` uses an immutable digest-idempotency ledger for `created -> preparing -> running`, `running -> paused`, and `paused -> running`. Pause requires no active execution lease, Agent, unfinished Supervisor turn, or prepared steering delivery. Same-key requests across processes append the transition events once; a delayed replay returns the original operation fact plus the Run's current state.

`run_execution_handoff.v1` freezes at most eight pending steering identities from the exact Run and Session, in sequence order, before acquiring a private Run execution lease. It invokes only the existing `RunSupervisor`, Policy, budget, model ledger, and tool gates. Messages appended during execution cannot enter the frozen batch, a selected message cancelled before delivery is skipped, and the durable result is fenced by lease generation. Browser responses contain only bounded counts, state, stop reason, and model/tool-called booleans, never content, model output, tool arguments, operation keys, or lease identity.

Non-schema D1-S2 separately exposes exact pending-only cancellation through `session_steering_cancellation.v1`; prepared, committed, and already-cancelled messages cannot be rewritten through it. Each operation has an independent Desktop capability behind the same distinct control bearer and defaults off. Success grants no Shell, Docker, LocalRunner, network, file-write, tool-approval, or child-scheduling authority. Execution handoff is currently an explicit bounded operator action, not a background wake/retry scheduler.

## 模型、Plan 与审批控制 / Model, Plan, And Approval Controls

非 schema 的 D1-M1、D1-P1 与 D1-A1 组成第三个三切片产品批次，SQLite 继续保持 v73。D1-M1 把 CLI、HTTP 和 Desktop 统一到同一个 Go `modelregistry.Registry`：启动时装载环境 Provider 和持久化路由，`GET /api/v1/models` 只返回固定 Provider/路由、模型名、可用性和配置状态，不返回 API key、Base URL、环境变量名，也不发起网络探测或模型调用。可疑或类似密钥的模型/路由标识会失败关闭或被脱敏。

D1-P1 增加两个彼此独立的摘要幂等控制：操作者先从已持久化的三项方案中选择一个方向，Go 原子创建其 WorkItems 与交接 Note；随后必须再次显式进入 Deliver。选择不会改变阶段，进入 Deliver 不会恢复 Run、取得 execution lease、调用模型/工具或自动执行。D1-A1 增加最多 100 条的 metadata-only pending approval 队列，以及独立的 `approve_once|deny` 决策。文件替换只能拒绝；Shell 仍只产生既有 dry-run 结果；ScriptProcess 必须保持 process-disabled；当前 Policy 会在批准前重检，永久拒绝不能被审批覆盖，且不会创建 Session Grant、写文件或启动 Shell/Local/Docker 进程。

Non-schema D1-M1, D1-P1, and D1-A1 form the third three-slice product batch while SQLite remains at v73. D1-M1 unifies CLI, HTTP, and Desktop around one Go `modelregistry.Registry`. Startup loads environment-backed Providers and persisted routes, while `GET /api/v1/models` returns only bounded Provider/route names, model identifiers, availability, and configuration status. It exposes no API key, base URL, or environment-variable name and performs no network probe or model call. Suspicious or secret-like model/route identifiers fail closed or are redacted.

D1-P1 adds two independent digest-idempotent controls. The operator first selects one of the persisted three directions, atomically creating its WorkItems and handoff Note, and must then explicitly enter Deliver. Selection does not change phase; entering Deliver does not resume the Run, acquire an execution lease, call a model/tool, or execute work. D1-A1 adds a metadata-only queue of at most 100 pending approvals plus separate `approve_once|deny` decisions. File replacement can only be denied, Shell remains dry-run, ScriptProcess must remain process-disabled, current Policy is rechecked before approval, permanent denials cannot be overridden, and no Session Grant, file write, or Shell/Local/Docker process is created. ADR 0039 records these boundaries.

## Provider 诊断、Diff 审阅与 Wake 意图 / Provider Diagnostics, Diff Review, And Wake Intent

非 schema D1-M2 增加显式 `provider_diagnostic.v1` 和 `model_route_control.v1`。模型路由先写入 SQLite，成功后才更新进程内 Router；诊断只在操作者点击或执行命令后发起一次有界、无用户内容、禁用工具的极小模型请求。公开结果仅包含 Provider/模型、成功状态、是否重试、网络/模型是否调用和耗时，不返回模型正文、API key、Base URL、环境变量名或原始错误。每次显式诊断仍可能产生一次 Provider 请求和少量计费。

D1-D1 为精确 Run/Session/Workspace 增加 metadata-only 文件编辑队列、脱敏有界 Diff 和 `approve_intent|deny` 审阅。在该 v74 交付点，批准意图只把提案推进到 `approved`，不会读取 API key、调用模型或写工作区；真正的 apply 仍是独立 CLI 路径，专属 control capability、当前文件哈希复核和幂等事务由后续 v76 补齐。审批事实先于编辑状态落库，崩溃窗口可通过同向终态安全恢复，相反决定继续失败关闭。

schema v74 增加 `run_wake_control.v1`：操作者可以持久化或取消一个 Run 的有界 wake/retry 意图。SQLite 固定最大尝试次数、退避、截止时间、单一活动 owner 和 generation fencing，旧 owner 不能提交新一代结果。在该 v74 交付点，CLI/API/Desktop 只管理意图；没有后台 goroutine、系统服务、模型调用、工具调用、execution lease 或自动执行权。ADR 0040 记录这组三个边界。

Non-schema D1-M2 adds explicit `provider_diagnostic.v1` and `model_route_control.v1`. A route is persisted to SQLite before the in-memory Router changes. A diagnostic performs one bounded, content-free, tool-disabled model request only after an operator click or command. Its public result contains bounded status metadata only, never model text, API keys, base URLs, environment-variable names, or raw errors. Each explicit diagnostic may still incur one Provider request and a small charge.

D1-D1 adds an exact Run/Session/Workspace metadata-only file-edit queue, bounded redacted Diff, and `approve_intent|deny` review. At this v74 milestone, approval intent advances a proposal to `approved` without writing the workspace; the dedicated control capability, current-file hash recheck, and idempotent transaction arrive later in v76. The durable approval is committed before edit state; a same-outcome retry repairs that crash window, while an opposite decision fails closed.

Schema v74 adds `run_wake_control.v1`, allowing an operator to persist or cancel one bounded wake/retry intent per Run. SQLite enforces maximum attempts, backoff, deadline, one active owner, and generation fencing. At that v74 milestone, CLI/API/Desktop manage intent only: there is no background goroutine, system service, model/tool call, execution lease, or automatic execution authority. ADR 0040 records these boundaries.

本批累计六切片完整健壮性门已通过：最终全仓 ordinary/race 分别为 278.6 秒和 296.1 秒；普通与安全 Desktop tags 下的 vet/staticcheck/govulncheck、module verify/tidy、20 个文件 80 项 React 测试、strict TypeScript、Vite/Windows production build、npm 零漏洞、确定性 OpenAPI、CLI 隔离 smoke、UTF-8/89 条本地 Markdown 链接/凭据变更/运行产物/新增进程入口扫描均为绿色。审计修复了 Diff 精确读取仍加载文件正文、审批两阶段崩溃恢复、过期最终 wake lease 的事件顺序、非法 delay/deadline 组合，以及并发模型路由的持久化/内存乱序；没有已知未解决的高/中风险。本门禁只调用 mock Provider，未使用真实 API key、网络 Provider、Shell、LocalRunner、Docker 或文件 apply。

The cumulative six-slice robustness gate is green on the final code: full ordinary/race suites took 278.6s/296.1s; ordinary and secure-Desktop vet/staticcheck/govulncheck, module verification/tidy, 80 React tests across 20 files, strict TypeScript, Vite/Windows production builds, zero-vulnerability npm audit, deterministic OpenAPI, isolated CLI smoke, and UTF-8/local-link/changed-credential/runtime-artifact/new-process-entry scans all passed. The audit fixed an exact Diff read that still loaded file bodies, two-phase approval crash recovery, event ordering for an expired final wake lease, invalid delay/deadline combinations, and concurrent route persistence/memory reordering. No unresolved high- or medium-severity issue is known. The gate used only the mock Provider and made no live API-key, network-Provider, Shell, LocalRunner, Docker, or file-apply call.

远端 GitHub Actions [run 29649564643](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29649564643) 同样通过：TypeScript console 30 秒、Windows Desktop shell 1 分 58 秒、Go control plane 3 分 44 秒。Remote GitHub Actions run 29649564643 is also green: TypeScript console 30s, Windows Desktop shell 1m58s, and Go control plane 3m44s.

## 显式 Wake、Diff Apply 与惰性 Skill 安装 / Explicit Wake, Diff Apply, And Inert Skill Install

schema v75 / D1-Q2 增加一次一条的显式前台 wake consumer。只有操作者执行 CLI 命令或点击 UI 后，Go 才会领取到期 intent，并通过既有 `run_execution_handoff.v1`、RunSupervisor、Policy、预算、取消和 execution lease 消费最多八步；没有后台 goroutine、系统服务或隐藏轮询。消费准备、Handoff 摘要、结果和事件先后持久化；`prepared` generation 不会因租约过期被重领，也不能在结果未知时被取消或伪造失败。只要失败记录含 handoff ID，模型/工具调用事实必须来自其持久化结果。

schema v76 / D1-D2 把 Diff 审阅与写文件固定为两个独立能力。`file_edit_apply.v1` 只接受精确 Run/Edit 和内存中的幂等 key，重新核对 Run、Mission、活动 Session、Workspace、持久化批准、当前 Policy 及原始/目标 SHA-256；每个 Edit 只能建立一个 apply operation，且真正写入前再次检查运行状态与 Policy。浏览器不能提交路径或正文；写入采用同目录暂存、同步、最终路径/哈希复核和原子替换，写后再核验目标摘要，崩溃重放只恢复同一结果。Run 绑定 Edit 已禁止使用旧 `edit approve` 旁路。

非 schema D1-B1 在 Desktop 原生选择/预览之后增加第四个窄 Wails 方法 `InstallSkillPackage`，并提供独立 HTTP control。Desktop 使用短期单次确认句柄，HTTP 使用严格有界 canonical base64；两者都只调用既有内容寻址惰性 Registry。安装要求明确确认 `operator_installed_untrusted`，不执行正文、脚本、钩子或命令，不联网、不调用 Provider/工具，也不自动选择到 Run 或注入上下文。每项能力均独立默认关闭。

Schema v75 / D1-Q2 adds an explicit one-item foreground wake consumer. Go claims a due intent only after a CLI command or UI click, then routes at most eight steps through the existing `run_execution_handoff.v1`, RunSupervisor, Policy, budgets, cancellation, and execution lease. No background goroutine, service, or hidden polling loop exists. A prepared generation is neither reclaimed after lease expiry nor cancelled while its result is unknown. Any failed consumption carrying a handoff ID derives model/tool-call facts from that handoff's durable result.

Schema v76 / D1-D2 makes Diff review and file writing independently authorized capabilities. `file_edit_apply.v1` accepts only an exact Run/Edit plus a memory-held idempotency key. One Edit admits one apply operation; Go rechecks the running Run, active Session, durable approval, current Policy, Workspace path, and hashes again at the write boundary. Browsers submit neither paths nor file bodies. Same-directory staging, sync, final path/hash verification, atomic replacement, and post-write digest verification make restart recovery converge without a second write. Run-bound edits require `review-approve` followed by `apply`.

Non-schema D1-B1 adds the fourth narrow Wails method, `InstallSkillPackage`, after native selection/preview and exposes a separate HTTP control. Desktop consumes a short-lived one-time confirmation handle, while HTTP accepts strict bounded canonical base64. Both call only the existing content-addressed inert Registry. Installation requires explicit `operator_installed_untrusted` confirmation and executes no content, script, hook, command, network, Provider, or tool; it neither selects the package for a Run nor authorizes context injection. Every capability is independently disabled by default.

## 恢复回执、Workspace Explorer 与便携诊断 / Recovery Receipts, Workspace Explorer, And Portable Diagnostics

非 schema D1-U1、D1-E1 与 D1-W1 完成下一组三切片，SQLite 继续保持 v76。`operation_receipt.v1` 为 FileEdit apply、前台 wake consume 和惰性 Skill install 提供统一、无正文的持久化结果：只说明结果、是否重放、允许使用的重试策略和清理状态，不暴露 operation key、摘要、路径、正文或私有 lease。FileEdit 在终态或重放后只清理同一目标目录内、超过 15 分钟、普通文件且字节精确匹配批准内容摘要的内部暂存文件；任何新鲜、异常或无法确认的候选都保守保留并在回执中要求复核。

`workspace_explorer.v1` 新增 read-bearer `GET /api/v1/workspaces/{workspace_id}/explore` 和 Run 的 Files 页。Go 从注册 Workspace 解析规范 `/` 分隔相对路径，拒绝遍历、绝对/卷路径、软链接、重定向、控制字符和非规范路径；目录只扫描 400 项并返回最多 200 项，文件只读取 64 KiB UTF-8，脱敏后的投影最多 128 KiB。内部暂存文件和可疑名称不进入列表，本机 root 永不返回；每份投影使用 `context_provenance.v1` 标记为 `instruction_authorized=false` 的证据。TypeScript 只能继续使用 Go 返回的精确子路径，并以纯文本 `<pre>` 呈现内容。

`cyberagent doctor portable [--json]`、`scripts/build-desktop.ps1 -VerifyReproducible` 与 `scripts/check-windows-compat.ps1` 增加可复现版本元数据、连续双构建 SHA-256、PE/架构/零 COFF 时间戳、`-trimpath`、模块身份和非安装边界检查。输出目录必须留在仓库且不能穿过重解析点。自动检查通过不等于正式发布：在 Windows 10/WebView2 人工矩阵签署前，`release_ready` 始终为 `false`；项目仍不包含安装器、注册表写入、自启动或自动更新。

Non-schema D1-U1, D1-E1, and D1-W1 complete the next three-slice batch while SQLite remains at v76. `operation_receipt.v1` gives FileEdit apply, foreground wake consumption, and inert Skill installation one content-free durable result model. It exposes only outcome, replay, the closed retry strategy, and cleanup state, never an operation key, digest, path, body, or private lease. FileEdit recovery removes only an old regular internal staging file in the exact target directory whose bytes match the approved proposal digest; fresh or uncertain candidates remain untouched and visible as pending review.

### Workspace 搜索、证据附加与回执历史 / Workspace Search, Evidence Attachment, And Receipt History

schema v77 / D1-E2、D1-C1 与 D1-U2 完成新的三切片。`workspace_search.v1` 只对既有 Explorer 的脱敏投影执行一次确定性搜索，限制 128 个目录、1000 个条目、64 个文件和 50 个结果；不建立索引器、不跟随链接，也不返回宿主根路径。`session_evidence_attachment.v1` 由独立且默认关闭的 capability 控制，操作者只能提交精确 Run、相对引用和投影 SHA-256；Go 重新核对 Run/Mission/Session/Workspace 与当前文件，并在一个事务中写入 tool-role evidence message、事件和不可变附件。Go 与 SQLite 都固定 `instruction_authorized=false`，所以 README 中写给自动助手的诱导文本仍只是证据，不能批准工具或扩大权限。

`operation_receipt_history.v1` 从 FileEdit apply、前台 wake 和惰性 Skill 安装的终态事实生成最多 100 条可刷新回执。公开 DTO 不含 operation key/digest、路径/正文、请求者、包元数据或私有 lease；FileEdit 暂存状态只读检查，任何不确定性都保守显示为 `pending_review`，列表操作绝不删除文件。本批没有调用模型、工具、Shell、Docker、网络或 API key，也没有新增后台 worker。

Schema v77 / D1-E2, D1-C1, and D1-U2 complete the new three-slice batch. `workspace_search.v1` performs one deterministic search over existing redacted Explorer projections, bounded to 128 directories, 1,000 entries, 64 files, and 50 results. It creates no indexer, follows no links, and returns no host root. The independently gated and default-off `session_evidence_attachment.v1` accepts only an exact Run, relative evidence reference, and projected SHA-256. Go rechecks the Run/Mission/Session/Workspace and current file before atomically storing a tool-role evidence message, event, and immutable attachment. Go and SQLite both fix `instruction_authorized=false`, so document text addressed to an automated assistant remains evidence and cannot approve a tool or widen authority.

`operation_receipt_history.v1` derives at most 100 refreshable terminal receipts for FileEdit apply, foreground wake, and inert Skill installation. Public DTOs omit operation keys/digests, paths/bodies, requesters, package metadata, and private leases. FileEdit staging state is inspected read-only; uncertainty is conservatively reported as `pending_review`, and listing never deletes a file. This batch called no model, tool, Shell, Docker, network, or API key and added no background worker.

## 操作者行动中心、证据清单与命令面板 / Operator Actions, Evidence Inventory, And Command Palette

非 schema D1-O1、D1-C2 与 D1-K1 完成下一组三切片，SQLite 继续保持 v77。`operator_action_center.v1` 由 Go 在精确 Run/Mission/Session/Workspace 绑定下聚合 pending steering、审批、Diff 审阅/apply readiness 和到期 wake，最多返回 100 条闭集 metadata；公开 ID 使用域分离的不透明摘要，不返回消息正文、命令、参数、路径、Diff、私有 operation 或 lease 身份，也不会自动批准或执行。`session_evidence_inventory.v1` 只列出已经附加到当前 Run/Session 的 source kind/reference、SHA-256、时间与固定 `instruction_authorized=false`，不重放正文，也不把证据推导成权限。

React 新增 Actions、Evidence 两个 Run 页签和 `Ctrl+K` 命令面板。Actions 只能导航到现有 queue/approvals/diffs/wake 视图，Evidence 只能把 Go 发放的 canonical 相对引用交回既有 Files Explorer，命令面板只导航或刷新当前查询；三者均不创建 renderer 原生文件、进程、网络、审批或 mutation 通道。真实浏览器审计还发现事件协议版本曾错误要求 `event.v1`，失败重连没有取消响应体，可能耗尽同源连接；前端现与 Go 的 canonical `v1` 对齐，OpenAPI 将其生成为字面量类型，解析/传输失败时先取消 reader 再重连。

Non-schema D1-O1, D1-C2, and D1-K1 complete the next three-slice batch while SQLite remains at v77. Go exact-binds the Run/Mission/Session/Workspace and exposes at most 100 closed metadata items through `operator_action_center.v1`, aggregating pending steering, approvals, Diff review/apply readiness, and due wake intent. Domain-separated opaque IDs replace private source identity; message bodies, commands, arguments, paths, Diffs, operations, and leases remain absent, and the projection never approves or executes an action. `session_evidence_inventory.v1` lists only the source kind/reference, SHA-256, attachment time, and fixed `instruction_authorized=false` for evidence already attached to the exact Run/Session. It replays no body and infers no authority.

React adds Actions and Evidence Run tabs plus a `Ctrl+K` command palette. Actions may only navigate to existing queue, approval, Diff, or wake views; Evidence can pass only a Go-issued canonical relative reference back to the existing Files Explorer; palette commands only navigate or refresh current queries. None creates a renderer-native filesystem, process, network, approval, or mutation path. Real-browser auditing also found that the client had required `event.v1` instead of Go's canonical event-envelope version `v1`, then leaked a response body on failed reconnect. The client now shares the generated literal `v1` contract and cancels the reader before reconnecting after any parse or transport failure.

`workspace_explorer.v1` adds read-bearer `GET /api/v1/workspaces/{workspace_id}/explore` and a Files tab. Go resolves canonical slash-separated relative paths from the registered Workspace, rejects traversal, absolute/volume paths, links, redirects, controls, and normalized aliases, scans at most 400 directory entries, returns at most 200, reads at most 64 KiB of UTF-8, and caps the redacted projection at 128 KiB. Internal staging and suspicious names are omitted, the host root is never returned, and every projection is `context_provenance.v1` evidence with `instruction_authorized=false`. TypeScript may navigate only the exact child paths issued by Go and renders file text without Markdown execution.

`cyberagent doctor portable [--json]`, `scripts/build-desktop.ps1 -VerifyReproducible`, and `scripts/check-windows-compat.ps1` now verify reproducible release metadata, consecutive SHA-256 equality, PE architecture, a zero COFF timestamp, `-trimpath`, module identity, and the non-installing boundary. Output must remain inside the repository without traversing a reparse point. Automated success is not release approval: `release_ready` stays false until the manual Windows 10/WebView2 matrix is signed, and no installer, registry write, startup task, or updater exists.

## Windows 桌面端 / Windows Desktop

Desktop D0-A、D0-B 与 D1-R1 至 D1-G11/V10 自动化核心已完成；项目全局 SQLite 当前为 v84，并包含非产品 R8 Runner 证据边界。项目固定 Wails v2.13.0 稳定版，并提供 Windows `cyberagent-desktop` 开发/便携测试二进制。Vite production bundle 在编译期嵌入；现有 Go `api.v1` Handler 直接接入 Wails AssetServer，不监听 TCP 端口，也不建立第二套业务 API。桌面端默认只读，每类 mutation 都由独立显式 flag 开启。本地 Monaco 可安全恢复提案；Repository 状态/脱敏 Diff/历史/精确提交预览/可导航精确文件历史/精确提交比较及可连续导航的成对 base/head 预览、change-set、不可变验证与 snapshot-keyset 逐检查项下钻/快照下载/回执历史/不授权复核、Code Handoff/export 和 Code Journey 复用既有 Go read/control 边界；Windows Credential Manager 修改可热重载 Registry；可选 wake worker 固定单并发/单步，仍无 Shell/Local/Docker。

原生 `.zip` 对话框现已接入 ADR 0033：本地路径只在 Go 内部短暂存在，经过严格 `skill_package.v1` 校验后立即丢弃；React 只能得到五分钟、单次消费的不透明句柄和有界风险元数据。Renderer 绑定面只有 `Bootstrap`、`SelectSkillPackage`、`PreviewSkillPackage`、`InstallSkillPackage` 四个方法；安装方法只消费 Go 发放的确认句柄，不能提交路径、文件字节、命令或权限位。进程、Shell、Docker、网络、Provider、工具和 capability authority 全部固定为 false。

The automated core of Desktop D0-A, D0-B, and D1-R1 through D1-G11/V10 is complete at schema v84, together with the test-only R8 Runner evidence boundary. The project pins stable Wails v2.13.0 and builds a Windows `cyberagent-desktop` development/portable-test executable. Its Vite production bundle is embedded at compile time, and the existing Go `api.v1` Handler is connected directly to the Wails AssetServer without opening a TCP listener or creating a second business API. The shell defaults to read-only and every mutation class has an independent explicit flag. Monaco recovery, Repository state/redacted Diff/history/exact-commit preview/navigable exact-file history/comparison with navigable paired base/head previews, change-set review, immutable verification with snapshot-keyset drilldown/download/receipt history/non-authorizing review, Code Handoff/export, and the Code Journey reuse existing Go boundaries; Windows system-credential changes hot-reload the Registry, and the bounded worker retains no Shell/Local/Docker authority.

The ADR 0033 native `.zip` flow is now visible. A selected path exists briefly inside Go, is strictly validated as `skill_package.v1`, and is then discarded. React receives only a five-minute, single-use opaque handle and bounded risk metadata. The complete renderer binding surface is `Bootstrap`, `SelectSkillPackage`, `PreviewSkillPackage`, and `InstallSkillPackage`; installation consumes only a Go-issued confirmation handle and accepts no path, bytes, command, or authority field. Process, Shell, Docker, network, Provider, tool, and capability authority remain false.

D0-B 把桌面 SQLite/API 生命周期收敛到可测试的 Go `desktop.ControlPlane`，并使用同一 Run-bound 高水位 cursor 新增 `GET /api/v1/runs/{run_id}/events/poll`。普通 Web 仍使用 SSE；Windows renderer 最多在内存保留 16 个 Run、每个 500 帧，重挂载从最后确认 cursor 恢复，失效 cursor 每次挂载最多回退一次，且不写浏览器存储。CLI 与 Desktop 同库写读、关闭重开、六路并发打开、poll/SSE 互续、真实二进制强制结束后重开和第二实例让位均已验证。

D0-B moves Desktop SQLite/API ownership into a testable Go `desktop.ControlPlane` and adds `GET /api/v1/runs/{run_id}/events/poll` using the same Run-bound high-water cursor as SSE. Ordinary Web keeps SSE; the Windows renderer retains at most 16 Runs and 500 frames per Run in module memory, resumes from the last confirmed cursor after remount, resets an invalid cursor at most once per mount, and writes no browser storage. Tests cover concurrent CLI/Desktop access, close/reopen, six simultaneous opens, poll/SSE cursor interchange, a real forced-process restart, and second-instance handoff.

桌面生产构建现在强制使用 `desktop,production,wv2runtime.error` 标签，并在打开数据库前只读检查 WebView2 `94.0.992.31` 或更新版本；缺失或过旧只给出有界提示，不下载、不安装、不打开网页。进程内 API 只接受精确 `http://wails.localhost`，重建 URL/`RequestURI` 并固定 loopback；Wails 原生 binding 仍限于起始 origin，CSP 与 renderer 导航守卫阻止外链、外部表单和弹窗。当前 Windows 11 实机与 Windows CI 已覆盖，Windows 10 实机兼容仍保留为正式发布前矩阵。

Production Desktop builds now require the `desktop,production,wv2runtime.error` tags and perform a read-only WebView2 `94.0.992.31` prerequisite check before opening SQLite. A missing or old runtime yields a bounded diagnostic without downloading, installing, or opening a web page. The in-process API accepts only exact `http://wails.localhost`, rebuilds URL/`RequestURI`, and pins loopback; native bindings remain restricted to the start origin, while CSP and a renderer navigation guard block external links, forms, and popups. Windows 11 and Windows CI are covered; a real Windows 10 compatibility run remains a pre-release matrix item.

本地构建命令为 `powershell -ExecutionPolicy Bypass -File scripts/build-desktop.ps1`，输出 `build/desktop/cyberagent-desktop.exe`。D0-B 最终本地门禁通过 256.6 秒全仓普通测试、273.5 秒全仓 race、双标签 vet/staticcheck、双路径零漏洞 govulncheck、13 个文件 37 项前端测试、生产构建与零漏洞 npm audit。GitHub Actions run `29609621468` 已通过实现提交 `c9b1c66`，Go/Linux、Windows Desktop、TypeScript 分别用时 5 分、4 分 21 秒、23 秒。审计修复了失效 cursor 可能重复回退、来源 `RequestURI` 未无条件规范化和原生 restore/Stop 窄竞态三项低风险问题；漏洞扫描还发现 Desktop 依赖图中 `x/net/html@v0.54.0` 的五项新可达通告，现已升级到修复版 `x/net@v0.55.0` 并复扫为零。最终未签名 GUI 为 19,572,224 字节，SHA-256 `f26ea87f42701a7eba8efa789900ea6953ef3c1533ff95106ec4b8e6b02b1160`；它仍不是安装包，不写注册表、不自启动、不自动更新、不提供终端或真实进程执行。未发现未解决高/中风险，完整边界见 ADR 0034、ADR 0035 与 `docs/DESKTOP_PLAN.md`。

Build locally with `powershell -ExecutionPolicy Bypass -File scripts/build-desktop.ps1`; the result is `build/desktop/cyberagent-desktop.exe`. The final D0-B local gate passed the 256.6-second full ordinary suite, 273.5-second full race suite, ordinary and secure-Desktop vet/staticcheck, both zero-finding govulncheck paths, 37 frontend tests across 13 files, the production build, and zero-vulnerability npm audit. GitHub Actions run `29609621468` passed implementation commit `c9b1c66`; Go/Linux, Windows Desktop, and TypeScript completed in 5m00s, 4m21s, and 23s. The audit fixed three low-risk issues: repeated invalid-cursor fallback, non-canonical source `RequestURI`, and a narrow native restore/Stop race. Vulnerability scanning also found five newly reachable `x/net/html@v0.54.0` advisories in the Desktop dependency graph; upgrading to fixed `x/net@v0.55.0` returned both scans to zero. The unsigned GUI is 19,572,224 bytes with SHA-256 `f26ea87f42701a7eba8efa789900ea6953ef3c1533ff95106ec4b8e6b02b1160`. It remains a non-installer with no registry, startup, update, terminal, or real-process behavior. No unresolved high/medium issue is known. ADR 0034, ADR 0035, and `docs/DESKTOP_PLAN.md` record the boundary.

## 架构能力详解 / Architecture Details

### 中文详解

以下内容按能力域和当前阅读价值组织，不代表 schema 时间顺序；需要核对开发先后时，请以上方 `v1 -> v79` 表为准。

P7 Skills 的第一条纵向链路已经落地。Go 内置并严格校验 `code`、`review`、`learn`、`script` 与跨 Profile 的 `plan-delivery` 五份 `skill.v1` 工作流指导，包括固定版本、兼容 Profile、工具前置声明、相对内容路径、UTF-8、字节数、保守 token 上界和 SHA-256。只读 Registry 与 `skill list/show/validate` 不创建数据库，也不读取任意外部路径；当前内置指导版本为 `1.1.0`，工具依赖绝不直接授予执行权限。

`skill_package.v1` 校验、schema v69 惰性用户 Registry、schema v70 外部 Run 选择/最小上下文、schema v71 三端只读来源投影、Desktop D1-A 路径隔离预览与 D1-B1 确认安装已完成：Go 对只含 `manifest.json` 与 `SKILL.md` 的严格确定性 ZIP 做纯内存校验，并仅在显式确认后发布到内容寻址对象存储；另一次 Run 级确认才能固定精确版本并以非可信用户上下文交付。导入不执行正文，选择不授予声明工具，模型请求之外不持久化正文。HTTP 安装只接受有界 canonical base64，Desktop 只消费 Go 发放的一次性确认句柄；签名分发、远程下载和安装时执行仍未开放，边界见 ADR 0024、ADR 0031 至 ADR 0035，以及 ADR 0041。

非 schema 的受保护删除守卫也已接入 Go Policy。Shell、ScriptProcess 与 Sandbox 的可执行意图若包含递归删除、绝对/越界/通配路径、环境变量或命令替换目标，或常见 PowerShell、`cmd`、Python、Node 删除形式，会在审批前被永久拒绝；逐次审批和 Session Grant 都不能覆盖。README、日志和模型说明仍按无执行权限的证据处理。该守卫是纵深防御而不是宿主 Full Access 的安全证明：真实 Local 与容器进程仍关闭，未来执行必须继续依赖只读/不可见宿主根、隔离输出和类型化工作区删除工具。边界见 ADR 0025。

该切片本地发布门禁已通过最终全仓普通测试、全仓 race、三层安全链路 race 重复测试、约 40.6 万次 Policy fuzz、100/50/50 轮 Policy/Gateway/Application 高频回归、vet、staticcheck、module verify/tidy、govulncheck、17 项前端测试、OpenAPI/TypeScript/生产构建/npm audit，以及凭据、运行产物、乱码、Markdown 链接和 diff 扫描；未发现未解决的高/中风险问题。schema 仍为 v63，架构完成度与产品可用度口径不变。

P7 的第二条纵向链路新增 schema v39 与不可变 `skill_selection.v1`。操作者可在 Run 启动前，用 `skill select` 为其 Profile 固定最多 8 个 Skill 的名称、版本、内容哈希和总保守 token 预算；同一 Run 只能有一份选择，原始 operation key 只以域分隔 SHA-256 摘要落库，并发请求和跨重启重放会收敛到同一结果。选择事件只记录数量与预算，不记录 Skill 正文、路径、工具依赖或原始 key。

P7 的第三条纵向链路新增 schema v40 与 `skill_context.v1`。每个 root Supervisor turn 都只从该 Run 的持久化选择和内嵌 Registry 重新核对名称、版本、哈希、字节数与 Profile，先脱敏，再按独立预算和稳定顺序把正文放入内存中的 Provider request。Registry 以每个 Skill 最多 8 个版本的硬上限内嵌保留 `1.0.0` 历史正文，因此旧 Run 精确恢复原版本，新选择只使用 `1.1.0`。准备与首次 `model.started` 之间使用可恢复的两阶段来源账本；SQLite 和 Run 事件只保存数量、预算、脱敏计数与指纹等元数据，不保存正文、路径、名称或内容哈希。Skill 指导从属于 Policy，不会新增工具、Shell、网络、文件、委派或子 Agent 权限。

schema v41 新增由 Go 主控的不可变 `run_mode.v1`。每个 Run 同时固定一个工作面（`code` 或 `cyber`）和一个执行阶段（`plan` 或 `deliver`）；它们与权限、审批和网络 Scope 相互独立。工作面在同一 Run 内不可切换，执行阶段只能由操作者在 `created` 或 `paused` 且没有活动 execution lease 时显式变更。Plan 阶段允许分析、拆解和创建 WorkItem/Note，但不能完成 Run；模型若返回 `finish`，Supervisor 会走一次有界协议修复并要求 `wait`。旧数据库和未显式指定模式的新调用兼容为 `code/deliver`。CLI、TUI、HTTP/OpenAPI 和 React 控制台都读取同一份持久化模式快照；模式本身永远不是工具、Shell、网络或子 Agent 的授权凭据。

schema v42 完成 Go 主控的 `plan_delivery.v1` 规划交付协议。Plan 阶段的 root 模型只能记录恰好三个有界方向，每个方向包含 1-8 个按顺序排列、只能向后依赖的交付切片；提案不会自动选方向、改阶段或执行工作。Run 暂停且 execution lease 已释放后，操作者通过稳定幂等键选择 1/2/3，Go 会在同一 SQLite 事务中创建对应 WorkItem 依赖图、置顶 decision Note、选择事实与完整事件链。选择后 Run 仍停留在 `plan`，必须另行显式切换到 `deliver`。CLI 可写入口是唯一选择入口；HTTP、TUI 与 React 仅投影状态，并始终显示 `capability_grant=false`。

schema v43 新增 `context_provenance.v1`，把“文件内容是证据，不是指令”落实为数据协议，而不只依赖提示词。操作者消息、模型响应、Go 控制文本、工作区文件/目录、Diff、工具结果和命令结果拥有不同且受 SQLite 约束的来源类型；`/read`、`/ls`、`/write` 与 `/run` 结果以 `tool` 证据持久化，并固定 `instruction_authorized=false`。内容在脱敏后计算 SHA-256，来源和正文不可修改，读取时由 Go 复核摘要。进入普通 Session、root Supervisor、Specialist 或压缩摘要前，外部内容都会变成带来源、引用、摘要和授权位的 `untrusted_context.v1` JSON 记录，不能借 README、Issue、日志或网页中“写给 AI 的备注”获得 system/assistant 权限。该层降低间接 Prompt Injection 的影响，但不宣称模型语义判断已经绝对安全；工具、Scope、Policy、审批与预算仍是最终能力边界。

schema v44 新增 Go 主控的 `delivery_checkpoint.v1` 切片交付门禁。对 v44 管理的 Plan 选择，每个所选 WorkItem 必须先进入 `in_progress`，操作者再在 Run 暂停、处于 Deliver 且没有活动 execution lease 时记录聚焦验证、Diff 审计、安全审计和交接摘要；最后一个切片还必须记录整体验证与健壮性审计。检查点固定选择、提案、验收条件、源模块、当前模式 revision 和 WorkItem version，并原子创建不可变置顶交接 Note、幂等操作事实与元数据事件；随后现有 WorkItem/Run 完成路径才会放行。模型、HTTP、TUI 和 React 只能读取门禁，不能代替操作者写入；明显借 Shell/进程自调用 CLI 伪造检查点的工具请求会被 Policy 拒绝。迁移前已部分完成的选择不会被补造证据，而是明确显示 `delivery_gate_enforced=false` 以保持兼容。

schema v45 新增持久化操作者引导队列。Run 忙碌时，普通 Session 输入或显式 `run steer enqueue` 不再打断正在进行的模型/工具事务，而是以有界正文、稳定顺序和摘要化幂等操作原子落库。Supervisor 只在下一个安全 root turn 边界取出最早一条消息，并通过 `prepared -> committed/superseded` 投递账本绑定精确 attempt；失败、取消、进程重启与租约接管不会提前写入 Session，也不会重复提交。若模型在队列仍有后续消息时请求 `finish` 或 `wait`，Go 会把有效动作延后为 `continue`；Run 完成也受 Go 与 SQLite 双层 pending 门禁。CLI 可查看本机正文，HTTP/OpenAPI、React 和 TUI 只显示有界状态与序号元数据，不公开正文、摘要、操作者身份或 Session 内部关联，也不授予任何新工具能力。

schema v46 完成操作者引导队列控制。操作者可以用摘要化幂等键取消尚未准备投递的 pending 消息；取消事实和操作账本不可修改，已 prepared 的消息仍禁止取消，队列也不支持编辑或重排。`run steer drain` 在持有 Run execution lease 后才会显式唤醒 paused Run，并且只执行真正来自队列的 turn，不会顺手恢复普通失败输入或生成默认 turn。普通 Run-bound `session send --operation-key` 可跨进程安全重试，始终先持久化队列事实而不伪装成同步模型回复。HTTP/OpenAPI、React、TUI、模型和 child Agent 继续没有队列写权限；预算、Policy、生命周期和租约边界全部复用既有 Go 主控链路。

schema v47 完成 Specialist Skill 最小化。每个 child Attempt 只能从父 Run 已固定的 Skill 选择、不可变 Run 模式和 Go 内嵌 Registry 派生最多一份指导；`plan-delivery` 不进入 child，上层 assignment 只参与来源指纹，不能自行选 Skill 或扩权。Code 工作面按 Profile 只取 `code/review/learn/script` 中对应的一项；Cyber 工作面默认为空，仅 Script Profile 可取得窄化的 `script` 指导。独立 1024-token 默认预算和 2048-token 硬上限、`prepared -> committed` 两阶段账本以及首次 `model.started` 原子提交支持重启恢复。SQLite 和事件只保存聚合元数据与指纹，不保存 Skill 正文、路径、名称、版本或内容哈希；核心两个 child、只读 Fan-out、Policy、Scope 与无工具边界均未放宽。

schema v48 新增 Go 主控的 `sandbox_manifest.v1` 意图边界。严格 JSON 协议对后端描述、executable/argv、工作区相对挂载、沙箱工作目录、网络白名单、CPU/内存/PID/输出上限、环境字面量或密钥引用、输入 Artifact、输出、超时和取消宽限执行硬校验；重复字段、未知字段、路径穿越、重叠挂载、通配目标、非规范 CIDR、疑似凭证 argv 和越界资源全部关闭失败。Go 将规范化指纹绑定到非终态 Run、Mission、持久化 Workspace 根、Mission Scope、Policy、可选精确审批和 Go 生成的取消身份。SQLite 只保存不可变 preparation/validation/operation 元数据，命令、参数、路径、环境值、密钥引用、目标和 Manifest JSON 原文不落库、不进事件。Docker/Local、写挂载、网络或密钥引用即使通过 Policy 也要求审批；审批通过仍固定 `backend_enabled=false`、`execution_authorized=false`。`NoopRunner` 只做确定性校验，Local/Docker 继续关闭，不会启动任何宿主机或容器进程。

schema v49 完成第一条审批与再校验闭环。操作者可从 preparation 的精确授权指纹创建通用 `tool_approvals` 请求并显式批准或拒绝；后续候选必须重新提交完整 Manifest，再次匹配 Run/Mission/Workspace/Scope/Policy 和批准，使用 Go `os.Root` 解析且限制挂载源位于工作区内，并在同一 SQLite 写事务复核累计 token/模型时间、工具调用预算和活跃 execution lease。不可变候选与事件只保存指纹、计数和状态，不保存 Manifest、命令、路径、密钥或目标；跨进程同键重放收敛。候选仍固定 `backend_enabled=false`、`execution_authorized=false`，不是执行许可证，也不会调用 Local/Docker。

schema v50 新增禁用态 `sandbox_execution.v1` 生命周期，但仍不执行任何进程。`begin` 再次提交完整 Manifest，并重验 v49 候选、Scope、Policy、审批、挂载、总预算和 Run lease；输入 Artifact 逐项绑定精确 Run/Session/Workspace、SHA-256、大小、MIME、来源和顺序，合计最多 16 MiB。独立 Sandbox lease 使用 generation fencing 支持崩溃接管；取消请求和清理结果不可变、可幂等重放，Run 即使已终止仍可清理。当前清理结果只能是 `backend_disabled`，并证明后端未启动、无孤儿进程、输入已复核、输出 Artifact 为零。私有 SQLite lease/cleanup 表只保存围栏所需的不透明 lease ID 与 worker owner，事件和 CLI 不暴露它们；任何生命周期账本或事件都不保存命令、路径、Manifest 或 Artifact 正文。Local/Docker 继续关闭。
v50 发布门禁已通过全仓普通/race 测试、vet/static/module/vulnerability、前端、凭证与运行产物扫描、真实二进制生命周期 smoke，以及 GitHub Actions run `29353239789`；提交 `ff4846a` 的 Go 与 TypeScript 作业分别耗时 2 分 6 秒和 25 秒，未发现未解决的高/中风险问题。

schema v51 新增禁用态 `sandbox_preflight.v1`。每次预检必须重新提交完整 Manifest，并再次复核 v48 preparation、v49 candidate、v50 lifecycle、Scope、Policy、精确审批、挂载、累计预算、Run lease 和输入 Artifact 完整性。Go 固定一份 16 项 Docker/后端威胁模型，覆盖宿主路径隔离、mount propagation、只读根与输入、独立输出、网络默认拒绝与精确白名单、临时密钥、非 root 身份、CPU/内存/PID/超时/kill、orphan 恢复和原子 Artifact 提交；当前所有项目都只能是 `required=true`、`verified=false`、`not_probed`。输出导出计划只保存不透明 locator 指纹和 stdout/stderr/file 类型，固定 all-or-nothing、总字节硬上限、MIME 检测、普通文件限制、symlink/special-file 拒绝、脱敏和重启前协调。后端握手、容器身份、输出导出、Artifact 提交和执行授权全部保持 false；CLI 不显示 locator、原始路径、命令、Manifest、容器身份或内部 lease。该事实是下一阶段实现清单，不是 Docker 可用性证明或执行许可。
v51 发布门禁已通过全仓普通/race 测试、vet/static/module/vulnerability、前端 17 项测试、严格 TypeScript、OpenAPI 漂移检查、生产构建、npm audit、凭证/运行产物/Runner 调用扫描、diff 检查和隔离真实二进制 preflight smoke；Go/npm 可达漏洞均为 0，未发现未解决的高/中风险问题。GitHub Actions run `29357134923` 已通过提交 `041f617`，Go 与 TypeScript 作业分别用时 2 分 13 秒和 19 秒。

schema v52 新增 `sandbox_backend_evidence.v1` 与 `sandbox_output_simulation.v1`，专门验证后续生产接线所需的协议，而不连接 Docker。内存假客户端把规范 OCI 镜像摘要以及 daemon、挂载、网络、密钥、容器配置、资源、终止、orphan 和输出计划分别绑定到 16 项 `simulated_pass` 证据；它们全部保持 `verified=false`，根记录固定 `simulation_only`、`production_verified=false`，后端可用、执行授权和 Artifact 提交授权也全部为 false。输出夹具经过严格重复字段/UTF-8/槽位顺序、总字节、MIME、普通文件、symlink/特殊文件拒绝和脱敏检查后，只能原子提交到内存假账本；失败或取消回滚为零，生产 `run_artifacts` 始终不变。Application 与 SQLite 在证据和输出两个边界都重验完整 v48-v51 权限链、实时预算、lease 与输入 Artifact。CLI/事件/数据库不保留夹具正文、原始路径、命令、Manifest、密钥、容器 ID 或私有 lease；模拟通过不是 Docker 可用证明，也不是执行许可。
v52 发布门禁已通过定向测试、全仓普通/race 测试、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、严格 TypeScript、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭证/运行产物/Runner 调用/Markdown 链接扫描、diff 检查和隔离真实二进制完整模拟 smoke。审计未发现未解决的高/中风险问题，并补强了脱敏后夹具摘要、完整请求指纹、SQL 预算/输入 Artifact 复核、迁移降级测试顺序和每份 evidence 最多 8 次模拟的双层门禁。GitHub Actions run `29362181363` 已通过提交 `f48cbb4`，Go 与 TypeScript 作业分别用时 2 分 9 秒和 19 秒。真实 Provider、Shell、网络、Local、Docker、生产 Artifact 和 CTF 执行均未启用。

schema v53 新增不可变的 `sandbox_docker_observation.v1`，在仍然没有容器执行能力的前提下观测固定本机 Docker Engine。只读 transport 只暴露 `Ping`、`Version`、`Info` 和按规范 OCI 摘要 `InspectImage`；Linux 只连接 `/var/run/docker.sock` 并只允许四类 GET，请求不能跟随重定向，Windows 当前明确返回 `transport_unsupported`。接口没有 create/start/run/exec/pull/remove，代码不调用 `docker` CLI，也不接受 `DOCKER_HOST`、任意 TCP 地址或调用方 socket。观测必须显式确认并绑定同一份 v52 evidence、output simulation 和完整 Manifest；Application 与 SQLite 再次复核 v48-v52 身份、Policy、审批、预算、lease、输入 Artifact、取消和清理状态。结果只可能是 `observation_complete`、`daemon_unavailable` 或 `image_unavailable`，每份 simulation 最多 8 条；原始 daemon ID、主机名、Docker 根目录、socket、RepoDigest、Manifest 和命令都不落库、不进事件。`production_observed=true` 只表示只读 daemon 与镜像元数据被观测，不表示隔离控制已经验证；私有 mount 支持仍标记为 `not_observable_read_only`，`production_verified`、后端可用/启用、执行与 Artifact 提交授权全部固定为 false。`run sandbox observe|observations|observation-show` 也不会创建、拉取、启动或删除任何容器。
v53 本地发布门禁已通过最终代码的全仓普通/race 测试（125.4 秒/140.7 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、严格 TypeScript、OpenAPI 无漂移、生产构建、零漏洞 npm audit、真实测试 key 前缀/运行产物/Docker 写入口/Markdown 链接扫描、diff 检查和隔离真实二进制完整链路 smoke。smoke 证明未确认探测被拒绝、Windows 只记录 `transport_unsupported`、同意图重放不重探、生产 Artifact 为零且 CLI 不泄漏私有 Docker 字段。审计修复了两项低风险健壮性问题：并发 Application 请求现在即使各自产生不同候选 ID/时间也按语义意图收敛；底层 HTTP 路径白名单与最终 method/path/query/JSON media type 校验阻止未来内部误用。连续 20 次普通和 10 次 race 的双 Store 竞态回归通过。首次 GitHub Actions run `29368112083` 的 TypeScript 作业在 22 秒内通过，但 Linux Go 作业暴露 CLI 单元测试误触 runner 真实 Docker daemon 的环境耦合；生产默认路径未改变，测试现通过仅进程内 observer 注入使用确定性 unavailable fake，真实 daemon 仍只属于显式 CLI 和 opt-in 集成测试。GitHub Actions run `29368979988` 已通过修复提交 `fe7b070`，Go 与 TypeScript 作业分别用时 2 分 7 秒和 23 秒。未发现未解决的高/中风险问题。

schema v54 新增确定性的 `sandbox_docker_container_spec.v1`、不可变 `sandbox_docker_container_plan.v1` 和纯内存 `sandbox_docker_write_transaction.v1`。只有完整且仍有效的 v53 观测可以进入编译器；Application 与 SQLite 会再次复核 v48-v53 的 Manifest、身份、Policy、审批、预算、lease、输入 Artifact、取消和清理链。编译结果固定非 root 身份、只读根与输入、唯一可写输出、`rprivate` mount、网络默认拒绝与精确白名单、临时密钥、CPU/内存/PID/时间/kill 上限、标签化 orphan 身份和停止后导出顺序。完整规格只存在内存；持久化计划、事件和 CLI 不包含命令、参数、路径、网络目标、环境值、密钥引用或容器身份。七步 write transaction 只在假 harness 中暂存，失败、崩溃或取消全部回滚为零；成功也固定 daemon 写入数、后端接触、生产提交、执行授权和 Artifact 提交授权为零。`run sandbox docker-plan|docker-plans|docker-plan-show` 不会创建、启动、停止、导出或删除真实容器。
v54 本地发布门禁已通过最终代码的全仓普通/race 测试（128.2 秒/148.5 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、严格 TypeScript、OpenAPI 无漂移、生产构建、零漏洞 npm audit、测试 key 前缀/运行产物/Docker 写入口/乱码/Markdown 链接扫描、diff 检查和隔离真实二进制 v54 migration/Workspace smoke。审计补强了 v52 evidence/output simulation requester 连续性、SQL v54 显式 requester 约束和假事务快照深拷贝；双 Store 并发回归连续 20 次普通、10 次 race 通过。GitHub Actions run `29376503165` 已通过功能提交 `126719f`，Go 与 TypeScript 作业分别用时 2 分 7 秒和 19 秒。未发现未解决的高/中风险问题，真实 Docker 写入与生产 Artifact 仍为零。

schema v55 新增独立且默认关闭的 `sandbox_docker_write_transport.v1` 与不可变 `sandbox_docker_container_rehearsal.v1`。只有操作者显式确认并重新提交完整、当前有效的 v54 计划后，Go 才能在 Linux 固定 `/var/run/docker.sock` 上执行一次“创建未启动容器、精确核验、删除”演练。首个 profile 强制无网络、无环境变量、无密钥；transport 固定 Docker API `1.40`，不接受 `DOCKER_HOST`、TCP、任意 socket、代理或重定向，闭合白名单只允许精确镜像 inspect、create、容器 inspect 与固定 `v=1` 的非强制 delete，没有 start、exec、attach、pull、日志、导出、卷管理或通用请求。create 前必须确认本地镜像 RepoDigest 与计划一致且没有声明 `VOLUME`；容器必须使用摘要镜像、非 root、只读根、drop-all capabilities、资源上限和 private mount。名称冲突只有在旧容器完整匹配且从未启动时才允许回收；取消、失败或 create 响应不确定时，独立有界 context 会重新 inspect，只有精确 authority match 才删除，绝不按返回 ID 盲删。数据库、事件和 CLI 只保存元数据与指纹，不保存原始容器 ID、宿主路径、命令、环境值、密钥、socket 或完整规格；同操作键重放不会再次访问 daemon，跨 Store 并发收敛。正常路径有三次 daemon 读取与两次真实写入，精确回收一个旧演练容器时为三次写入，但容器进程从不启动，镜像不拉取，输出不导出，生产执行、验证、后端启用、执行授权和 Artifact 授权全部固定为 false。
v55 本地发布门禁已通过最终代码的全仓普通/race 测试（163.3 秒/168.7 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、严格 TypeScript、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭证/运行产物/禁止 endpoint/乱码/Markdown 链接扫描、diff 检查和隔离真实二进制 schema-v55 Workspace smoke。审计修复了两类发布前风险：create 已落地但响应取消时现在会立即按确定性名称回收，失败清理也不会盲删返回 ID；镜像声明 `VOLUME` 会在 create 阶段产生匿名卷，因此现在 create 前拒绝该 profile，delete 固定清理匿名卷，并额外核验 attachment/device/port/capability。transport 普通 20 轮、race 10 轮和双 Store 并发 10 轮回归通过。未发现未解决的高/中风险问题。当前 Windows 本机没有可用 Docker，故默认跳过的 Linux real-daemon 写集成测试未实际执行；该残余验证缺口不开放 start 或生产权限。GitHub Actions run `29382661971` 已通过功能提交 `69d81d6`，Go 与 TypeScript 作业分别用时 2 分 32 秒和 25 秒。

schema v56 在任何 daemon 变更之前先持久化 `sandbox_docker_container_rehearsal_attempt.v1` 意图，并用有过期时间和递增 generation 的 SQLite 租约隔离并发操作者。transport 新增可恢复的 stage/cleanup 两阶段：create 后只核验并保留未启动容器，19 项不可变矩阵记录镜像、命令、非 root、只读根、capability、网络、空环境、mount 配置、资源、端口、设备、attachment、authority 标签和 never-started 等配置事实，但每项都明确 `execution_evidence=false`；stage 落库后才按确定性名称重新 inspect 并非强制删除。进程若在 create 响应、stage 提交、delete 或最终事务之间退出，新 generation 会收养同一 authority 的精确停止容器，或把已不存在的容器视为幂等清理完成，不会重复 create，也不会删除不匹配的同名容器。失败原因以有界代码追加到不可变 ledger 后释放租约；最终 v55 rehearsal、operation、v56 completion 和租约释放在一个事务中提交。镜像和容器 inspect 现在同时拒绝任何继承环境变量，修正了“Manifest 未传环境”不等于“最终容器环境为空”的证据缺口。`docker-attempts` 与 `docker-attempt-show` 可查看恢复状态和检查矩阵；原始容器 ID、宿主路径、socket、命令、环境值、密钥和完整规格仍不落库。start、exec、attach、pull、logs、export、网络、密钥、生产验证、执行授权和 Artifact 授权继续不可达。

v56 本地发布门禁已通过全仓普通/race 测试（178.5 秒/181.3 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、严格 TypeScript、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭据/运行产物/禁止 endpoint/增量乱码/Markdown 链接扫描、diff 检查和隔离真实二进制 schema-v56 Workspace smoke。恢复 transport 普通 20 轮/race 10 轮、双 Store 单租约竞态普通 20 轮/race 10 轮、Application 未知 create 恢复 10 轮全部通过。审计修复了继承环境证据、过期 completion/failure fencing、失败代码/顺序/16 条上限、generation 获取时间线、旧 generation 借用重放、孤立 legacy operation、按 attempt ID 恢复、SQL 控制矩阵映射和 mount TOCTOU 过度表述问题；当前未发现未解决高/中风险。Windows 本机无 Docker，故 Linux real-daemon harness 未执行；宿主 mount TOCTOU 仍留给 v57，二者都不授予 start 或生产权限。GitHub Actions run `29388724727` 已通过功能提交 `e1710bb`，Go 与 TypeScript 作业分别用时 2 分 32 秒和 23 秒。

schema v57 在 v56 容器保持停止状态时增加独立的 `sandbox_docker_host_input_staging.v1` 演练。Linux 实现使用 `openat2` 的 `RESOLVE_NO_SYMLINKS`、`RESOLVE_NO_MAGICLINKS`、`RESOLVE_BENEATH` 与 `RESOLVE_NO_XDEV` 固定工作区根和每个只读输入树。每个条目先以 `O_PATH` 预检，再只对同一 inode 的普通文件或目录执行内容打开，因此 FIFO 和其他特殊文件会在潜在阻塞前失败；目录和单文件 mount 均支持，符号链接、硬链接、跨挂载点、越界、过深或超限输入全部拒绝。完成 descriptor pin 后再次核对 inode、大小、时间戳和链接数，再将规范化只读树与已按 Run/Session/Workspace/摘要复核的 Artifact 组成确定性 tar；目录的文件系统相关 inode 大小不进入内容摘要。包只存在于 `memfd`，随后施加 `F_SEAL_WRITE/GROW/SHRINK/SEAL` 并重新读取核对摘要。SQLite 在当前 v56 attempt、container-ID 指纹、计划、输入摘要和 lease generation 下先保存不可变 intent，再保存不含路径、正文、fd 或原始容器 ID 的计数与摘要；未完成 v57 的 attempt 由 SQL 禁止 completion。`--stage-host-inputs` 还要求独立的 `--confirm-host-input-staging`，失败会先尝试清理停止容器，重启后可由新 generation 恢复且不重复 create。当前密封包尚未交给 Docker daemon，`daemon_consumed=false`、`execution_evidence=false`；因此它只关闭 descriptor-capture 内的路径偷换窗口，不是未来容器实际消费输入的生产证据，也没有开放 start、exec、网络、输出导出或 Artifact 提交。
v57 本地发布门禁已通过最终代码的全仓普通/race 测试（155.0 秒/168.1 秒）、vet、零告警 staticcheck、module verify、零可达漏洞 govulncheck、17 项前端测试、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭据/运行产物/进程入口扫描、diff 检查和隔离真实二进制 schema-v57 smoke。Application staging 连续 10 轮、双 Store 独立 ID 并发 20 轮及 race 10 轮通过，Linux test binary 交叉编译通过。定向覆盖 rename/replace/delete/symlink/hard-link/FIFO/单文件 mount、有界目录枚举、取消、重放、租约接管、迁移和隐私。审计修复了单文件报告/SQL 约束、目录摘要跨文件系统稳定性、特殊文件预打开阻塞、超大目录预分配、读取取消延迟、随机 ID 幂等冲突和漏确认消耗失败账本等问题，未发现未解决高/中风险。v57 发布时记录的低风险恢复缺口现已由 v58 关闭。

schema v58 新增不可变的 `sandbox_docker_host_input_requirement.v1`。对所有新 attempt，Go 会在任何 daemon stage 之前，把“本次是否必须进行 v57 宿主输入捕获”与 attempt、计划、Run/Mission/Workspace、Manifest/mount/input/authority 指纹、只读挂载与输入 Artifact 计数、操作者确认和请求者绑定在同一 SQLite 事务中；该事务同时创建 attempt、首代 lease 与审计事件。`required=true` 时，恢复即使不再提交 staging flags 也必须完成同一份 v57 证据后才能 completion；`required=false` 不能在稍后被扩大为 true。Go 与 SQL 双层拒绝缺少证据的 required completion、错误 attempt/plan 绑定和 false→staging 扩权，事件与查询只暴露摘要和计数，不保存路径、内容、fd、原始容器 ID、operation key 或私有 lease 身份。迁移把既有 v57 attempt ID 放入不可新增/修改/删除的兼容集合，不伪造历史选择，也不允许迁移后的新 attempt 绕过 requirement。v58 仍不把密封包交给 Docker，也不增加 archive、volume、start、exec、pull、build、export 或 Artifact 权限；daemon 托管的固定 volume carrier、回读校验和最终只读挂载被拆到独立的 schema v59 门禁。

schema v59 完成默认关闭、四重显式确认的 daemon 宿主输入交接，但仍不启动容器。Go 在任何 archive/volume 写入前原子持久化 handoff requirement 与写前 intent，绑定 v58 requirement、v57 密封报告、attempt/plan、lease generation、停止容器和完整 authority 指纹。Linux transport 只允许固定本机 Unix socket、Docker API `1.40`、确定性 carrier/volume 名称和固定 `/cyberagent-input/bundle.tar`；它创建 daemon-owned 本地 volume 与永不启动的可写 carrier，上传包后通过 daemon `GET archive` 回读并核对精确长度与 SHA-256，再删除 carrier 和原停止容器，短暂重建挂只读 volume 的目标容器，核验后连同 volume 一并删除。目标根与 Manifest mount 从未放宽为可写；用户 mount 与保留目录树重叠会提前拒绝。精确残留可在重试时收敛，外来同名资源永不删除，失败后台清理也只处理指纹匹配的自有对象。SQLite 要求 required handoff 完成后才允许 cleanup/completion，且所有持久结果继续固定 start/exec/export/backend/execution/Artifact 权限为 false。CLI 新增 handoff flags 及 metadata-only list/show；路径、内容、原始 ID、资源名、socket、operation key 和私有 lease 不落库、不进事件。

schema v60 新增经操作者显式确认、但绝不接触 daemon 的确定性运行时输入投影计划。Go 先重新核验完整 v48-v59 authority 链、重编译同一份 Manifest/v54 spec，再从 descriptor-safe provider 重新捕获短生命周期 v57 密封包；报告、摘要、长度、计数与 Artifact payload 必须和已完成的 v59 handoff 完全一致。严格解析器只接受逐字节 canonical 的 v57 PAX tar，拒绝链接、设备、遍历、重复/缺失父目录、异常 root、空 Artifact、尾随数据和非规范 header。第一版只接受目录 root 的只读 mount，Artifact 固定映射到 `/cyberagent-input/artifacts`；每个 root 生成独立的相对 tar 投影，未来 volume 身份同时绑定 handoff 指纹，以保证重试收敛且不同 Run 不碰撞。SQLite 在单事务中提交 plan/items/completion/operation/event，只保存摘要与计数；raw target、宿主路径、文件名/内容、volume 名和 archive bytes 均不落库。状态固定为 `compiled_not_applied`，daemon contact/apply、start、exec、export、backend、execution 与 Artifact 权限全部为 false。

schema v61 把 v60 的短生命周期投影真正应用到 Docker 本地卷，但流程仍停在“目标容器从未启动”。操作者必须分别确认运行时输入应用和 daemon 写入；Go 会先原子写入不可变 intent 与独立 generation lease，再重新核验 v48-v60、重编译规格、重新解析宿主挂载并重新捕获密封输入。固定 Unix transport 只允许精确 image/container/volume inspect、local volume 与未启动 carrier/target 的 create/delete，以及固定 `/cyberagent-input` 的 archive PUT/GET。每个投影先写入独立卷、经 daemon 回读并做完整 tar 语义核验，再以只读 `NoCopy` 方式挂到最终 target；唯一可写项仍是已复核的 output bind。重试只协调完整 authority 匹配的自有资源，外来碰撞永久拒绝；旧 generation 不能提交，失败会以独立有界 context 清理并释放 lease，语义重放不再次捕获或访问 daemon。SQLite、事件和 CLI 只保存摘要、计数、状态与权限位，不保存 target/path/file/resource 名称、原始 ID、archive、socket、operation key 或私有 lease。成功状态固定为 `volumes_applied_target_never_started`，start、exec、export、backend、execution 与 Artifact 权限继续全部为 false；Windows 明确返回 unsupported，不回退到宿主进程。

schema v62 为 v61 保留的 never-started target 与运行时输入卷增加独立资源生命周期。`docker-runtime-input-resource-inspect` 需要只读探测确认，重新核验完整 v48-v61 authority 与 Manifest，但不会重新捕获输入包；固定本机 Unix transport 只读取目标和全部确定性卷，持久化精确自有、部分/缺失或外来碰撞状态。只有完整目标与全部卷同时匹配时，才证明 never-started、read-only 与 `NoCopy`。清理还要求独立的资源清理和 daemon-write 双确认；Go 在任何 DELETE 前写入不可变 intent 与 generation lease，transport 必须先检查全部资源，发现任一外来对象时保证零 DELETE，否则先按已核验容器 ID 删除目标，再删除精确卷，并重新检查全部缺失。失败会追加有界代码并释放 lease，重启可由新 generation 接管；lease 行不可删除，failure/result 时间必须落在当前活跃租约窗内，完成和语义重放不会再次访问 daemon。SQLite、事件与 CLI 不保存资源名、容器 ID、宿主路径、socket、Manifest、原始操作键或私有租约身份。该切片没有 start、exec、attach、pull、export、backend、execution 或 Artifact 提交入口；Windows 仍明确 unsupported。

schema v63 新增不可变的 `sandbox_docker_start_gate_review.v1`，把 v51 的 16 项威胁检查逐项映射到 v52-v62 已有证据和仍未满足的生产阻塞项。操作者必须重新提交完整 Manifest，并在已完成 v62 精确清理后显式确认设计审查；Go 会复核 v48-v62 绑定，但不会重新捕获输入、访问 daemon 或创建进程。每项检查固定保存有界 evidence class/source、阻塞代码和未来独立门，当前 16 项全部 `production_verified=false`、`sufficient_for_start=false`。同一事务还保存 11 条未来 start/wait/TERM/KILL/orphan 转换蓝图，要求 per-Run generation-fenced 单一所有者、写前意图、固定端点、取消扇出、有界日志和孤儿协调；每条转换都固定 `implemented=false`、`authorized=false`。审查结论只能是 `blocked/deny_start`，real-daemon 链、启动门、进程执行、输出导出和 Artifact 提交均为 false。SQLite 约束、不可变触发器、幂等操作和 CLI `docker-start-gate-review/list/show` 只记录元数据，不保存资源名、原始容器 ID、宿主路径、Manifest 正文或原始操作键。

v63 本地发布门禁已通过最终代码的全仓普通/race 测试（196.9 秒/212.3 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/密钥文件/禁止进程入口/乱码/diff 扫描、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v63 Workspace/Skill smoke；Sandbox/Store/Application/CLI 高频回归分别通过 20/15/10/10 轮。审计补上了 v63 子表迭代错误的即时返回，并修正内部错误文本以满足静态规则；未发现未解决高/中风险。Windows 本机仍不能执行 Linux v59/v61/v62 real-daemon opt-in 链，所以 16 项检查继续全部阻塞，start 不开放。桌面端与自定义 Skill 包仅新增计划文档，没有加入依赖、安装器、导入端点或权限。GitHub Actions run `29503856229` 已通过提交 `e25a2ab`，Go/Linux 与 TypeScript 作业分别用时 2 分 32 秒和 24 秒。

随后非 schema 的 `skill_package.v1` 校验切片通过最终代码全仓普通/race 测试（239.4 秒/226.8 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、20 秒约 2645 万次 parser fuzz、`internal/skills` 78.5% 语句覆盖、100 轮 parser 与 20 轮 CLI 重复回归、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit，以及凭据/运行产物/乱码/Markdown 链接/diff 扫描。审计固定 ZIP creator version 和 Deflate 精确耗尽，关闭有效压缩流后的隐藏载荷通道，移除测试中的弃用 API，并阻止文件系统错误回显包路径；未发现未解决高/中风险。该切片没有 migration、安装器、用户 Registry、Run 选择、执行或新权限。GitHub Actions run `29512332025` 已通过提交 `55b3fae`，Go/Linux 与 TypeScript 作业分别用时 3 分 4 秒和 20 秒。

v62 本地发布门禁已通过最终代码的全仓普通/race 测试（313.6 秒/329.6 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/运行产物/禁止能力/乱码/Markdown 链接扫描、diff 检查、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v62 Workspace smoke；Sandbox/Store/Application/CLI 高频回归分别通过 20/15/10/10 轮。审计修复了 partial/unsafe 证据过度声明、事件命名歧义、CLI foreign-collision 显示、未来或越界终态时间戳，以及 v61/v62 lease 可直接删除问题；当前未发现未解决高/中风险。本机 Windows 没有 Docker，因此 Linux v59/v61/v62 real-daemon opt-in 链只完成编译、尚未实际执行，该证据缺口继续阻止 start gate。GitHub Actions run `29444398815` 已通过功能提交 `d250d32`，Go/Linux 与 TypeScript 作业分别用时 2 分 35 秒和 20 秒。

v61 本地发布门禁已通过最终代码的全仓普通/race 测试（197.5 秒/316.8 秒）、vet、零告警 staticcheck、module verify、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/运行产物/进程入口/禁止 endpoint/乱码/Markdown 链接扫描、diff 检查、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v61 Workspace smoke；Sandbox race 连续 20 轮也通过。审计补强了逐卷回读上限、租约到期前清理窗口、未来租约与最短 TTL 拒绝、resume 的 Go Application 双确认、取消后的独立失败落账与稳定错误码、v55/v59/v61 transport 的最小接口暴露、Docker `RW`/`NoCopy` 真实挂载证据以及 operation digest 格式约束。当前未发现未解决高/中风险。Windows 本机没有 Docker，因此默认跳过的 Linux v59/v61 real-daemon harness 只完成编译、尚未实际执行；该证据缺口继续阻止 start gate。GitHub Actions run `29437941378` 已通过功能提交 `f4aaf7a`，Go/Linux 与 TypeScript 作业分别用时 2 分 37 秒和 27 秒。

v60 本地发布门禁已通过最终代码的全仓普通/race 测试（198.9 秒/194.0 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/运行产物/乱码/Markdown 链接扫描、diff 检查、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v60 Workspace smoke。编译器 50 轮、Store 30 轮、Application 20 轮、CLI 10 轮，以及关键 Sandbox/Store/Application race 10 轮均通过。审计修复了确认事实未持久化、未来 volume 跨 Run 碰撞、item fingerprint 错误全局唯一、tar 尾随数据、弃用 xattr API、mount ordinal 重复/越界、计划时间线缺口和 canonical 长 PAX 路径兼容性；当前未发现未解决高/中风险。Windows 本机仍无 Docker，因此 v59 real-daemon handoff 与现有 v61 volume-application harness 尚需在 Linux 人工环境执行；这不影响 v60 的纯本地编译结论，也不开放 start。GitHub Actions run `29428011306` 已通过功能提交 `cc92421`，Go/Linux 与 TypeScript 作业分别用时 2 分 48 秒和 24 秒。

v59 本地发布门禁已通过最终代码的全仓普通/race 测试（183.1 秒/185.1 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭据/运行产物/禁止 endpoint/乱码/Markdown 链接扫描、diff 检查、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v59 Workspace smoke。transport 普通 50 轮、Store 30 轮、Application 20 轮、CLI 10 轮，以及 transport/Store/Application race 20/10/10 轮通过。审计修复了失败早期遗留原停止容器、archive 回读 media type 未校验、保留交接目录重叠、SQL endpoint/identity 约束不足和测试 harness 未使用真实密封 provider 等问题；当前未发现未解决高/中风险。Windows 本机无 Docker，故默认跳过的 Linux real-daemon handoff harness 只完成编译、未实际执行；该残余验证缺口不允许 start，也不构成生产执行证据。GitHub Actions run `29406403201` 已通过功能提交 `fb1daca`，Go/Linux 与 TypeScript 作业分别用时 2 分 37 秒和 28 秒。

v58 本地发布门禁已通过最终代码的全仓普通/race 测试（158.1 秒/168.4 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭据/运行产物/进程入口/乱码/Markdown 链接扫描、diff 检查、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v58 workspace smoke。领域 requirement 50 轮、Store 候选/缺失 requirement 30 轮、Application 恢复/防扩权 20 轮，以及 Store/Application race 各 10 轮通过。审计修复了 pending operation-key 恢复错误重建候选 attempt、durable requirement 下不成对 flags 被接受、迁移后 direct-SQL 新 attempt 缺失 requirement、false requirement 的零只读挂载兼容性等问题；当前未发现未解决高/中风险。Windows 本机无 Docker，故 v59 所需 Linux real-daemon handoff harness 尚未执行，也没有作出 daemon consumption 声明。

GitHub Actions run `29400696276` 已通过功能提交 `4b570f7`；Go/Linux 与 TypeScript 作业分别用时 2 分 39 秒和 23 秒。

首次 GitHub Actions run `29395980413` 只暴露 Linux 单文件测试夹具错误：夹具把唯一 mount 目标改成文件路径，却保留目录工作目录，Manifest 正确拒绝该状态；生产实现未失败。修复后的测试保留覆盖工作目录的只读目录 mount，并增加独立单文件 mount，从而真实覆盖 `directory_count < read_only_mount_count`。run `29396264276` 已通过提交 `8719dff`，Go/Linux 与 TypeScript 作业分别用时 3 分 55 秒和 23 秒。

v47 发布复核进一步加固了 Supervisor 并发恢复与预算边界：阶段切换重放会在观察到目标状态后重新核对幂等操作，取消测试会等待 Provider 真正进入调用后再触发取消；任何已经持久化 `model.started` 的 root 模型调用至少计入 1ms，避免 `999/1000ms` 的 deadline 边界被重复进入。该修复不改变 schema、不开放新能力，也不重解释旧 Specialist 账本。GitHub Actions run `29325171043` 已通过 Go 与 TypeScript 全部门禁。

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

schema v25 把 direct Specialist child 的 `dependency`、CompletionReport result 和崩溃 notification 接入 root Supervisor 的有界上下文。Go 每轮最多准备 4 条、按持久化 sequence 排序的消息，并以 `prepared -> committed/superseded` 两阶段账本绑定精确 Supervisor attempt；只有用户/助手消息与 lifecycle 成功提交时才原子消费，失败时消息保持 pending，取消、进程重启或 lease 接管会复用同一批次。模型只看到经过严格协议校验、脱敏和截断的类型化内容，不会看到消息 ID、sequence、cursor，也不能声明 sender 或控制消费。公开 spawn 与 child Provider loop 仍关闭。

schema v26 接入第一个仅 Go 内部可显式调用的 Specialist 无工具模型回合。`specialist_lifecycle.v1` 只允许 `continue`，或携带严格 `agent_completion.v1` 的 `finish`；模型不能声明 usage、Agent/Attempt 身份、租约、重试、Policy 结论或工具权限。`specialist_model_calls` 在同一 SQLite 事务中提交模型终态、token/执行时间、Policy 审计、脱敏 child Session 消息与 Attempt usage，避免重放重复计费；Provider 重试、context cancellation、Run lease 心跳/fencing、旧 worker 接管恢复和总历史 64 KiB 上限均由 Go 控制。该 Runner 没有 CLI/HTTP/model spawn 入口，也不提供工具、Shell 或网络权限；自主循环仍关闭，后续 v29 只为已经启动的精确 child call 增加取消控制。

schema v27 把直属 root 的 `specialist_instruction.v1` 指令和 child 自己拥有的活跃 WorkItem/Note 接入 Specialist 上下文。每个 Attempt 最多准备 4 条按 sequence 排序的父指令，`specialist_context_deliveries` 以 `prepared -> committed/superseded` 两阶段账本保证 `continue`/`finish` 成功时原子消费，崩溃、中断和 lease takeover 时保留 pending 并重新投递。上下文使用 4096-token 与 32 KiB 双重上限；模型看不到消息 ID 或 sender 选择，WorkItem/Note owner、消费位置、租约和终态仍完全由 Go/SQLite 控制。来源 ID 与 token 估算进入不含正文的 `model.started` 审计；该上下文层不提供公开 spawn、自动 child 循环或 child 工具。

schema v28 为 Specialist child 增加一次且仅一次的有界生命周期协议修复。全局 `model_attempt` 连续编号，主响应与修复响应各自拥有独立的 transport retry 计数；无效主响应的实际 usage 会先原子计费并创建 `specialist_protocol_repairs` pending 事实，修复成功后第二次 usage 累加到同一 AgentAttempt。修复 prompt 只附加固定诊断，不包含原始坏输出；坏输出也不会进入 Session 或 Run 事件。修复再次无效会标记 exhausted，取消、预算不足和 worker 接管会标记 aborted，只有 completed 修复可以继续或完成 child Attempt。每个 turn 在调用前同时复核 Run 剩余 token 与执行时间，修复只能使用主调用后的剩余额度。该能力仍是 Go 内部 no-tool 路径，不新增公开 spawn、CLI/HTTP 写入口、Shell 或网络权限。

Go 内部 `SpecialistScheduler` 一次调度持有一份 Run execution lease，每轮最多并发两个显式指定的 ready child，并以 32 轮硬上限、全完成/无 ready/首错/父取消/token/执行时间等条件停止。父 context、lease 心跳故障或任一 child 不可继续错误会取消同轮其他 child；调度器等待全部 Attempt 先写入终态，再释放 lease。Run 总预算每轮前后都从 SQLite 复核：root token/执行时间来自 Supervisor checkpoint，child token 同时核对 Agent 投影与 Attempt 账本，全部 child 模型耗时从持久化 model-call ledger 求和。v29 时该调度器只有 Go 内部构造路径；当前 v38 仅通过 operator-only application gate 暴露 CLI，仍没有 HTTP、模型 spawn、普通工具、Shell 或网络权限。

schema v29 为这条内部调度链增加可恢复控制事实。每次 schedule 都在当前 Run lease 下写入 `agent.schedule_started`，并以 completed/failed/cancelled 终态记录目标 child、轮数、已启动 turn、恢复数、停止原因和前后总预算快照；新 generation 接管时，遗留 running schedule 会恰好一次收敛为 `abandoned/worker_lost`。同一迁移还新增独立的 Specialist 模型取消账本：控制 API 只能提交精确绑定 `run_id + agent_id + agent_attempt_id + model_attempt` 的脱敏意图，持有该 Attempt 私有 lease 的 worker 观察后才取消本地 Provider context，模型终态与取消 resolution 原子提交。原始幂等键、模型正文、`lease_id` 和 fencing generation 不进入响应或取消/调度事件。该 POST 只增加取消能力，不增加 child admission、spawn、工具、Shell 或网络权限；scheduler 在目标 child 失败后仍按既有首错策略取消同轮 sibling，但不会为 sibling 伪造远程取消记录。

schema v30 新增严格的 `specialist_delegation.v1` 提案协议。root 可通过 `specialist_delegation_propose` 建议一到两个有界任务，给出标题、目标、父 Skills 子集和 turn/token 预算；Go 会在扣减工具调用预算后复核 active root、Supervisor checkpoint、execution lease、Run/Session/Workspace、剩余 child 容量、不可转授能力和 root 预算余量，再把脱敏后的 proposal、assignments、Policy/领域/工具事件与摘要幂等账本原子提交。相同语义意图跨轮次、重启和多 SQLite 连接只产生一个 proposal；账本不保存原始 operation key、Provider call ID、模型正文、`lease_id` 或 fencing generation，事件同样不含这些字段。proposal 始终是不可变的 `proposed` 状态，`admission_authorized=false`；它不会创建 Agent/Session、预留预算或启动 scheduler，实际 review、admission 与 spawn 仍只属于后续 Go internal/operator policy。`run delegations` 和 `run delegation` 只读显示待审事实。

schema v31 将 operator review 从 proposal 和执行授权中独立出来。`run delegation approve/reject` 只能为一个 proposal 写入一次不可变的 `approved` 或 `rejected` 事实；拒绝必须带理由，理由在 Store 边界脱敏且不进入事件。审阅操作使用摘要幂等账本，相同 key/意图安全重放，改意图或第二次改判返回冲突；批准要求 Run 仍在运行，拒绝则可在终态后留下关闭证据。`agent.delegation_reviewed` 只记录元数据并始终声明 `admission_authorized=false` 与 `application_required=true`，所以批准不会创建 child、Session、预算预留、指令或 schedule，也不能由模型或普通工具入口发起。

schema v32 为 approved proposal 增加显式 `run delegation apply`。Go 在创建 application 时重新检查当前 Policy、review operation、running Run、idle root/child scheduler、active root Session、parent Skills、child policy/capacity 和总预算；随后为一到两个 assignment 逐个幂等接入现有 admission，并发送严格 `specialist_instruction.v1`。application/assignment/operation 表在任何外部写入前持久化内部操作摘要，因此进程若在 Agent 或 Message 已提交后中断，重放会找回同一实体并继续，不会重复或留下不可追溯 child。applying 状态阻止 root turn、无关 admission/message 和 Specialist schedule/Attempt 抢跑；Run 终态会原子标记 application `aborted` 并留下元数据事件。成功只把 child 保持在 ready，不启动 Attempt 或 scheduler；模型、Provider、HTTP 与普通工具仍不能 approve、apply、spawn 或 schedule。该切片的独立审计未留下已知高/中风险问题；最终全仓测试、race、vet、staticcheck、模块校验与漏洞扫描均通过。

schema v33 启动与核心委派彻底分离的只读 Fan-out 路线。`run fanout plan` 支持 `auto/1/2/4/6` 并发档位，但“6”只属于 `readonly_fanout.v1` 的 workspace-list/read 能力包络；`MaxAgentChildren=2`、v30/v32 assignments 与 Specialist scheduler 上限均保持不变。Go 扫描器只接受工作区内普通 UTF-8 源码，跳过 symlink、版本库、依赖/构建目录、二进制及 secret-like 文件，并以最多 20,000 个目录项、256 个文件、单文件 128 KiB、总计 768 KiB 的硬限制生成 SHA-256 manifest 和确定性 shards。plan/file/shard/digest-only operation 在 schema v33 中不可变，相同操作安全重放，事件不含 goal、文件路径或工作区根目录。v33 当时只开放 `plan/list/show` 并报告 `execution_authorized=false`；Provider 并发和模型结果由下面的 v34 执行门补齐，自动代码写入仍未开放。独立审计未留下已知高/中风险问题，全仓测试、race、静态分析、模块校验、漏洞/凭据/产物扫描与真实 CLI smoke 均通过。

schema v34 为这份不可变计划增加显式只读执行门。`run fanout execute` 会在持有 Run execution lease 后重建完整 manifest，并通过 Go `os.Root` 再次核对每个文件的身份、大小和 SHA-256；任何新增、删除、修改或 symlink 漂移都会在零 Provider 调用时失败。通过复核的正文只在内存中存在并先做凭证脱敏，模型收到的请求固定为无工具、JSON-mode、`readonly_fanout_report.v1`，不能申请 Shell、写文件、进程、网络、外部工具或递归 child。1/2/4/6 个 worker 使用 Go 有界并发，首错取消同批 siblings；execution/shard/model-call/finding 与摘要幂等 operation 进入 schema v34 账本，事件仍只含元数据。root、核心 Specialist 和只读 Fan-out 的 token/模型时间统一从 SQLite 复核；未决崩溃调用按预留额度保守计费，新 lease generation 会 fence 旧 worker、标记调用 `abandoned` 并只重试未完成 shard。核心 Agent 图和两 child 上限完全不变，Fan-out 不创建 Agent、Attempt、schedule，也没有自动代码写入。

schema v35 将已完成 execution 的 shard 结果确定性投影为通用 Finding/Evidence/Report。Go 只合并严重度、类别、标题、详情、路径和行号完全一致的声明，绝不让模型二次归并或改写严重度；重复声明保留各自不可变 `model_assertion` Evidence，并采用更保守的最低置信度。`finding_reports` 通过同一 SQLite 事务完成 `building -> generated`，只有源 v34 行、连续序号、数量和严重度统计全部吻合才能终态化，之后 Report/Finding/Evidence 均不可修改或删除。每个 Finding 初始都是 `draft`，不等于已验证漏洞。`run fanout report` 和 `report show` 从持久化事实生成字节稳定的 Markdown/JSON，不调用 Provider，也不增加 Agent、工具或网络权限。

schema v36 为 `draft` Finding 增加独立且不可变的 operator 验证覆盖层。操作者可用 `report finding attach` 把同一 Run 的已冻结 Artifact 挂为 Evidence；Store 会重新读取完整正文并校验大小、SHA-256、MIME、stream、tool、source 和脱敏标志，SQLite 同时拒绝 Artifact 更新或删除。`validated` 必须引用至少一份 Artifact Evidence，`rejected` 可在没有 Artifact 时记录无法复现；每个 Finding 只能做一次决定。操作键只保存域分隔摘要，说明和理由会脱敏且不进入 Run 事件。验证不会修改 v35 原始 Finding 或投影摘要。

schema v37 在验证覆盖层之外新增不可变的 `validated -> accepted -> fixed` 修复生命周期。`report finding accept` 是独立 operator 决定，会冻结准确的 validation ID、Evidence 数量和摘要；它不会把验证自动当成接受。接受后，`report finding remediation attach` 只能引用同一 Run 中在 `finding.accepted` 事件之后创建的新 Artifact，既不能复用验证 Artifact，也不能使用接受前预先生成的输出。`report finding fix` 至少需要一份这样的修复 Evidence，并冻结其有序数量和摘要。三类操作都有摘要幂等账本、元数据事件、跨 SQLite 连接并发收敛和 update/delete 触发器，原始 v35 Finding 与投影摘要始终不变。

schema v38 为 v32 已应用且完成指令投递的核心 child 增加显式 operator 调度门。`run delegation schedule/continue` 只能由原 application operator 发起，只能选择该 application 的一到两个 ready child，并复用既有 `SpecialistScheduler`、Run execution lease、总 token/模型时间预算、同轮取消扇出与最多 32 轮硬上限。每个 operation key 对应一份不可变 request 和一次可恢复执行；相同 key/意图只返回原 schedule，改意图冲突，新 key 才会开始下一次显式 continuation。request/target/operation/attempt 全部进入摘要化、不可变的 SQLite 账本，pending request 会预留目标，普通内部 scheduler 不能抢跑；worker 丢失后同一 request 可在更高 lease generation 下把旧 schedule 收敛为 `abandoned/worker_lost` 并继续。模型、Provider、HTTP 和普通 `tool invoke` 仍不能创建 request、选择 child 或启动 scheduler，核心 child 上限保持两个。

schema v39 为每个 `created` Run 增加一份 Profile 匹配、总预算受限的 Skill 选择及摘要化幂等操作账本。选择项按名称稳定排序，固定 `name/version/content_sha256/content_bytes/token_upper_bound`，SQLite 与 Go 同时校验连续序号、总额、Run/Mission/Profile 绑定和不可变性。相同操作可在 Run 启动后继续重放，但任何新选择或改意图都会失败关闭。模型、HTTP 与 Tool Gateway 没有创建入口。

schema v40 将被选择的内嵌 Skill 受控接入 root Supervisor。每次 turn 都重新组装并验证脱敏后的 `skill_context.v1`，独立于结构化记忆预算；metadata-only 的 `prepared -> committed` 账本绑定 root Agent、Supervisor attempt 和首个模型调用，重启或第二条 SQLite 连接会恢复同一准备。正文只存在于当前 Go 进程的 Provider request，数据库和事件流不保存正文、路径、Skill 名称或哈希。

schema v47 将同一来源原则扩展到 Specialist，但只交付父选择中的最小兼容子集。每个 Attempt 最多一项、默认 1024 token；Code/Cyber 目录严格分离，Cyber 仅允许 Script Profile 的 `script` 指导。child assignment、模型输出和外部文档都不能选择或扩大内置 Skill，首次模型启动与 metadata-only 来源提交保持原子。

当前报告 runtime 提供不写库、不调用 Provider 的 SARIF 2.1.0、CI 门禁和 GitHub Actions annotation 投影。SARIF `results` 只包含尚未解决的人工确认项，即 `validated` 和 `accepted`；`fixed`、`draft`、`rejected` 只进入状态汇总，避免已修复或未经确认的声明成为上传告警。`cyberagentValidationStatus` 保留真实验证结论，`cyberagentFindingStatus` 表示当前生命周期。`report check` 的默认 `validated/high` 策略同时阻断 accepted 未解决项，显式 `active` 还纳入 draft，而 fixed/rejected 永不阻断。`--format github` 从同一内存态 `GateResult` 输出带文件和行号的 `notice/warning/error` workflow commands，严格转义模型生成的 `%`、CR/LF、冒号和逗号，并把其他 C0/DEL 控制字符转成可见文本；它不会输出 Artifact、Evidence note 或 operator reason，也不会改变原有退出码。

当前还提供第一版本地 HTTP 控制面。`cyberagent api serve` 只允许绑定回环地址；`CYBERAGENT_API_TOKEN` 只授权读取，默认关闭的控制入口必须另行设置不同的 `CYBERAGENT_API_CONTROL_TOKEN`。稳定的 `api.v1` envelope 和有界游标分页可读取 Run、Session、Event、WorkItem、Note、Artifact 元数据、Supervisor ToolRound、不含 fencing token 的 execution-lease 摘要和 v64 执行档位。三个 POST 只允许精确取消 root/Specialist 活动模型调用，或选择不授予执行权的 Run 档位；它们不能执行工具、启动进程、改变 Policy、接受客户端 fencing token 或读取 Artifact 正文/checkpoint pending input。API 不提供 CORS 或 WebSocket，也不会持久化任何 API 令牌。

Go 响应 DTO 现在同时生成唯一的 OpenAPI 3.1 契约。`cyberagent api openapi` 可输出确定性的 JSON，`--output docs/openapi.json` 可更新仓库内的受测快照；运行中的服务也会在鉴权后的 `/api/v1/openapi.json` 返回同一份原始文档。契约测试会逐条请求全部公开路由并阻止 DTO、静态文档与 handler 漂移，且明确排除 Artifact 正文、checkpoint pending input、`lease_id` 和 fencing token。

只读 Run Event Stream 也已接入同一控制面。`/api/v1/runs/{run_id}/events/stream` 使用 SSE 推送 SQLite 中已脱敏的持久化事件；每帧 `id` 是与 Run 绑定的不透明 cursor，客户端可通过 `Last-Event-ID` 精确续传。连接具有批量、帧大小、总事件数、寿命、写超时和进程并发上限，心跳不伪造 Run event；服务器关闭会主动取消长连接。SSE 本身不推送模型正文、不接受 token query、也不执行任何写操作。

React/Vite read-mostly 控制台位于 `web/`。它从 `docs/openapi.json` 生成 TypeScript DTO，读取 Workspace、Run、Session、WorkItem、Note、Artifact descriptor、ToolRound 与预算/租约摘要，并使用带 Authorization header 的 `fetch` 消费和续传 SSE。schema v64 增加可选、仅驻留内存的 distinct control token 和 `preview|docker|local` 分段控件；schema v72 复用 Go control route 增加注册 Workspace 内的幂等受控 Run 创建。TypeScript 只提交 Profile/Surface/Phase/Workspace/目标或档位 ID，不能提交 backend、scope、预算、门禁或权限位。生产构建可由 `cyberagent api serve --ui-dir web/dist` 在同一回环 origin 托管；Go 启动时把经过类型、大小、哈希文件名与软链接检查的 bundle 读成不可变内存快照，并对 HTML fallback、静态缓存和 CSP 设定严格边界。两个 token 与创建重试 key 都不进入 URL、localStorage 或 sessionStorage，浏览器也不重复实现 Go Policy。Vite 回环代理仍保留为前端开发路径。

Bubble Tea TUI 现在以 Run-first 选择器启动，可在最近 50 个 Run 与最近 50 个 Session 之间切换，也可用 `cyberagent tui --run <run-id>` 精确打开。Run 内可查看 Work Board、Notes、持久化 Supervisor Tool Rounds、最近 Run Events、Agent 图、Finding 报告摘要、Shell ToolRuns，以及最近 20 条 FileEdit 的只读 Diff 详情。FileEdit 查询不选择原文件正文或完整替换正文，显示内容另受 128 KiB/4096 行上限约束。TUI 按持久化 event sequence 有界追赶更新，钉住 Run/Mission、丢弃陈旧异步结果，并以事件尾前后复核保证 Session、工具和 Run 投影来自稳定读取窗口；终态 Run 会停止轮询。工具区支持“批准一次”和“本会话授权”：后者只创建精确绑定当前 Run、Session、Workspace、Shell 与 ActionClass 的可撤销 Grant，并在推进当前提案前重新检查持久化审批作用域和最新 Policy。所有非 Tools 标签及 Diff 详情均为只读，危险命令仍永久拒绝且不会创建 Grant；终端控制字符会被可视化，中文、组合字符和宽字符按终端单元格安全换行与截断。

`cyberagent headless events` 提供版本化 `headless.v1` NDJSON。它按 SQLite sequence 有界读取与续传同一条脱敏 Run 时间线，可用 `--follow` 等待终态，但不执行 Run、不调用 Provider、也不写入任何状态。每个事件和最终 `stream.end` 都各占一行；完成返回 0，失败返回 4，取消返回 7，事件上限返回 8，follow 超时返回 9。stdout 始终只包含 NDJSON，诊断文本保留在 stderr。

### English details

The following notes are organized by capability and current reading value, not by schema order. Use the canonical `v1 -> v79` table above whenever chronology matters.

The first P7 Skills vertical slice is now in place. Go embeds and strictly validates five `skill.v1` workflow guides for `code`, `review`, `learn`, `script`, and cross-Profile `plan-delivery`, including pinned versions, compatible Profiles, tool prerequisites, relative content paths, UTF-8, byte counts, a conservative token upper bound, and SHA-256. The read-only Registry and `skill list/show/validate` create no database and accept no arbitrary external path. The current built-in guidance version is `1.1.0`, and a declared tool dependency is never a capability grant.

`skill_package.v1` validation, the schema-v69 inert user Registry, schema-v70 external Run selection/minimized context, schema-v71 read-only provenance across HTTP/TUI/Web, and the non-schema pathless Desktop D1-A preview bridge are complete. Go validates a strict deterministic ZIP containing only `manifest.json` and `SKILL.md` in memory, then may publish it to content-addressed storage after explicit install confirmation. A separate Run-level confirmation is required to pin an exact version and deliver it as untrusted user context. Import executes no body, selection grants no declared tool, and content is not persisted outside the current model request. The three read surfaces expose bounded metadata without bodies, paths, digests, or private identities; a future desktop renderer can consume only a one-time opaque preview handle issued by Go. HTTP/Desktop install upload, signed distribution, and install-time execution remain closed; ADR 0024, ADR 0031, ADR 0032, and ADR 0033 record the boundaries.

A non-schema protected-delete guard now runs inside Go Policy as well. Executable Shell, ScriptProcess, and Sandbox intents are permanently denied before approval when they express recursive deletion, absolute/traversing/wildcard targets, environment-variable or command-substitution targets, or common PowerShell, `cmd`, Python, and Node deletion forms. Per-call approval and Session Grants cannot override that result, while README text, logs, and model explanations remain non-executable evidence. This is defense in depth, not proof that host Full Access is safe: real Local/container processes remain disabled, and future execution still requires inaccessible/read-only host roots, isolated output, and a typed workspace deletion tool. ADR 0025 records the boundary.

The slice's local release gate passed the final full ordinary suite, the full race suite, repeated three-layer security race tests, roughly 406,000 Policy fuzz executions, 100/50/50 Policy/Gateway/Application repetitions, vet, staticcheck, module verification/tidy, govulncheck, all 17 frontend tests, OpenAPI/TypeScript/production-build/npm audit checks, and credential/runtime-artifact/encoding/Markdown-link/diff scans. No unresolved high/medium issue is known. Schema remains v63 and the dual progress estimates are unchanged.

The second P7 vertical slice adds schema v39 and immutable `skill_selection.v1`. Before a Run starts, an operator can use `skill select` to pin up to eight Profile-compatible Skills by name, version, content hash, and aggregate conservative token budget. A Run can have only one selection; raw operation keys are stored only as domain-separated SHA-256 digests, and concurrent or post-restart replays converge on the same result. Selection events contain counts and budgets only, never Skill content, paths, tool dependencies, or raw keys.

The third P7 vertical slice adds schema v40 and `skill_context.v1`. Every root Supervisor turn rechecks names, versions, hashes, byte counts, and Profile against the Run's persisted selection and embedded Registry, then redacts and deterministically assembles the text under an independent budget into an in-memory Provider request. The Registry keeps a hard-bounded history of at most eight embedded versions per Skill, so existing Runs recover their exact `1.0.0` body while new selections use `1.1.0`. A recoverable two-phase provenance ledger spans preparation and the first `model.started`; SQLite and Run events retain only aggregate metadata and fingerprints, never text, paths, names, or content hashes. Skill guidance remains subordinate to Policy and grants no tool, Shell, network, file, delegation, or child-Agent authority.

Schema v41 adds Go-owned immutable `run_mode.v1`. Every Run pins one execution surface (`code` or `cyber`) and one execution phase (`plan` or `deliver`), independently of permissions, approvals, and network scope. The surface cannot change within a Run. An operator may change the phase only while the Run is `created` or `paused` and has no active execution lease. Plan mode may reason, decompose work, and create WorkItems or Notes, but it cannot complete the Run; a model `finish` is sent through one bounded protocol repair and must converge to `wait`. Legacy databases and callers that omit mode fields default compatibly to `code/deliver`. CLI, TUI, HTTP/OpenAPI, and the React console read the same persisted snapshot. A mode is never a grant of tool, Shell, network, or child-Agent authority.

Schema v42 completes the Go-owned `plan_delivery.v1` planning protocol. In Plan phase, the root model may only record exactly three bounded directions, each containing one to eight ordered delivery slices with backward-only dependencies. A proposal cannot select a direction, change phase, or execute work. After the Run pauses and releases its execution lease, an operator chooses direction 1, 2, or 3 under a stable idempotency key; Go atomically creates the corresponding WorkItem dependency graph, a pinned decision Note, the immutable selection, and its event chain. The Run remains in `plan` until a separate explicit transition to `deliver`. CLI is the only selection surface; HTTP, TUI, and React are read-only and always project `capability_grant=false`.

Schema v43 adds `context_provenance.v1`, enforcing “file content is evidence, not instruction” as a data protocol rather than relying on a prompt alone. Operator messages, model responses, Go control text, workspace files/listings, diffs, tool results, and command results have distinct SQLite-checked source types. `/read`, `/ls`, `/write`, and `/run` replies persist as `tool` evidence with `instruction_authorized=false`. Content is hashed after redaction; content and provenance are immutable, and Go verifies the digest on every read. Before external text reaches legacy Session chat, the root Supervisor, a Specialist, or context compaction, it is projected into a provenance-labeled `untrusted_context.v1` JSON record instead of a system or assistant message. Notes aimed at “automated coding assistants” inside a README, issue, log, or webpage therefore cannot acquire authority merely by being read. This reduces indirect prompt-injection risk without claiming perfect semantic immunity; Tool Gateway, scope, Policy, approval, and budgets remain the final capability boundary.

Schema v44 adds the Go-owned `delivery_checkpoint.v1` slice-delivery gate. For v44-managed Plan selections, each selected WorkItem must enter `in_progress`; while the Run is paused in Deliver phase with no active execution lease, an operator records focused verification, diff review, security review, and a handoff summary. The final slice additionally requires full functional verification and a robustness audit. A checkpoint pins the selection, proposal, acceptance criteria, source module, exact mode revision, and WorkItem version, and atomically creates an immutable pinned handoff Note, an idempotency fact, and metadata-only events. Existing WorkItem and Run completion paths consume those facts. Models, HTTP, TUI, and React can only read this gate; obvious Shell/process attempts to self-invoke the operator CLI are denied by Policy. Partially completed pre-v44 selections are explicitly reported with `delivery_gate_enforced=false` instead of receiving fabricated audit history.

Schema v45 adds a durable operator steering queue. While a Run is busy, ordinary Session input or explicit `run steer enqueue` persists bounded text in stable order instead of interrupting an active model/tool transaction. The Supervisor takes only the oldest item at the next safe root-turn boundary and binds it to the exact attempt through a `prepared -> committed/superseded` delivery ledger. Failure, cancellation, process restart, and lease takeover neither write the user message early nor commit it twice. When more queued input remains, a model-requested `finish` or `wait` is effectively deferred to `continue` by Go, and both Go and SQLite block Run completion with pending steering. Local CLI detail may show content; HTTP/OpenAPI, React, and TUI expose only bounded status and sequence metadata, omitting content, digests, operator identity, and internal Session links. The queue grants no tool or execution capability.

Schema v46 completes operator steering controls. An operator may cancel an unprepared pending message under a digest-only idempotency key; cancellation facts and operation ledgers are immutable, prepared messages remain non-cancellable, and editing or reordering is still unsupported. `run steer drain` acquires the Run execution lease before explicitly waking a paused Run and executes only genuine queued turns, never an unrelated failed input or a generated default turn. Ordinary Run-bound `session send --operation-key` is retry-safe across processes and always creates or replays a durable queue fact instead of pretending to return a synchronous model response. HTTP/OpenAPI, React, TUI, models, and child Agents retain no queue mutation authority, while existing Go-owned budget, Policy, lifecycle, and lease controls remain in force.

Schema v47 completes Specialist Skill minimization. Each child Attempt may derive at most one guide from the parent Run's pinned Skill selection, immutable Run mode, and Go-embedded Registry. `plan-delivery` never reaches a child, and assignment data contributes only to an immutable provenance fingerprint rather than choosing or widening Skills. The Code surface selects only the matching `code`, `review`, `learn`, or `script` guide; the Cyber surface is empty by default and permits only narrow `script` guidance for the Script Profile. A separate 1,024-token default budget, 2,048-token hard cap, recoverable `prepared -> committed` ledger, and atomic binding to the first `model.started` make delivery restart-safe. SQLite and events retain aggregate metadata and fingerprints only, never Skill bodies, paths, names, versions, or content hashes. The two-child core cap, read-only Fan-out, Policy, Scope, and no-tool child boundary remain unchanged.

Schema v48 adds the Go-owned `sandbox_manifest.v1` intent boundary. Strict JSON validation hard-bounds backend description, executable and ordered argv, workspace-relative mounts, sandbox working directory, exact network allowlist, CPU/memory/PID/output limits, literal or secret-reference environment bindings, input Artifacts, outputs, timeout, and cancellation grace. Duplicate or unknown fields, traversal, overlapping mounts, wildcard targets, non-canonical CIDRs, credential-shaped argv, and unbounded resources fail closed. Go binds the normalized fingerprint to a non-terminal Run, Mission, persisted Workspace root, Mission Scope, Policy decision, optional exact approval, and a generated cancellation identity. SQLite stores immutable preparation/validation/operation metadata only; commands, arguments, paths, environment values, secret references, targets, and Manifest JSON never enter the ledger or events. Docker/Local intent, writable mounts, network, or secret references require approval when Policy allows them, yet even an approved record remains fixed at `backend_enabled=false` and `execution_authorized=false`. `NoopRunner` validates deterministically while Local and Docker remain disabled, so no host or container process can start.

Schema v49 closes the first approval and revalidation loop. An operator can create a standard `tool_approvals` request from a preparation's exact authorization fingerprint and explicitly approve or deny it. A later candidate must resupply the complete Manifest, rematch Run/Mission/Workspace/Scope/Policy and approval, resolve mount sources beneath the workspace through Go `os.Root`, and recheck aggregate token/model-time usage, tool-call budget, and the execution lease inside the same SQLite write transaction. Immutable candidate rows and events retain fingerprints, counts, and status only, never Manifest content, commands, paths, secrets, or targets; cross-process retries converge. A candidate is still fixed at `backend_enabled=false` and `execution_authorized=false`, is not an execution permit, and never calls Local or Docker.

Schema v50 adds the disabled `sandbox_execution.v1` lifecycle without executing a process. `begin` resupplies the full Manifest and revalidates the v49 candidate, Scope, Policy, approval, mounts, aggregate budgets, and Run lease. Each input Artifact is pinned to its exact Run/Session/Workspace, SHA-256, size, MIME, source, and order under a 16 MiB aggregate cap. A separate generation-fenced Sandbox lease supports crash takeover; cancellation and cleanup are immutable and idempotent, and cleanup remains available after the Run becomes terminal. The only current cleanup outcome is `backend_disabled`, proving that no backend started, no orphan existed, inputs were reverified, and no output Artifact was produced. Private SQLite lease/cleanup rows retain only the opaque lease ID and worker owner required for fencing; events and CLI omit both, and no lifecycle ledger or event stores commands, paths, Manifest content, or Artifact bodies. Local and Docker remain disabled.
The v50 release gate passed the full ordinary/race, vet/static/module/vulnerability, frontend, credential/runtime-artifact scan, and real-binary lifecycle-smoke suites. GitHub Actions run `29353239789` passed commit `ff4846a`; its Go and TypeScript jobs completed in 2m6s and 25s, with no unresolved high- or medium-severity issue.

Schema v51 adds disabled `sandbox_preflight.v1`. Every preflight resupplies the full Manifest and rechecks the v48 preparation, v49 candidate, v50 lifecycle, Scope, Policy, exact approval, mounts, cumulative budgets, Run lease, and input-Artifact integrity. Go freezes a 16-item Docker/backend threat model covering host-path isolation, mount propagation, read-only roots and inputs, dedicated output, default-deny/exact-allowlist networking, ephemeral secrets, non-root identity, CPU/memory/PID/time/kill bounds, orphan recovery, and atomic Artifact commit. Every item currently remains `required=true`, `verified=false`, and `not_probed`. The output-export plan retains only opaque locator fingerprints and stream/file kinds while fixing all-or-nothing failure, aggregate byte limits, MIME checks, regular-file-only handling, symlink/special-file rejection, redaction, and restart reconciliation. Backend handshake, container identity, export, Artifact commit, and execution authorization all remain false; CLI output omits locators, raw paths, commands, Manifest content, container identity, and private leases. This fact is an implementation checklist, not evidence that Docker is available or permission to execute.
The v51 release gate passed the full ordinary/race, vet/static/module/vulnerability, 17-test frontend, strict TypeScript, OpenAPI drift, production-build, npm-audit, credential/runtime/Runner scan, diff, and isolated real-binary preflight-smoke suites. Reachable Go/npm vulnerabilities are zero, with no unresolved high- or medium-severity issue. GitHub Actions run `29357134923` passed commit `041f617`; its Go and TypeScript jobs completed in 2m13s and 19s.

Schema v52 adds `sandbox_backend_evidence.v1` and `sandbox_output_simulation.v1` to test the future production protocol without contacting Docker. An in-memory fake client separately binds a canonical OCI image digest and daemon, mount, network, secret, container-configuration, resource, termination, orphan, and output-plan fingerprints to 16 `simulated_pass` items. Every item remains `verified=false`; the root is fixed to `simulation_only` and `production_verified=false`, with backend availability, execution authority, and Artifact-commit authority all false. Strict output fixtures must match ordered slots and pass duplicate-field, UTF-8, aggregate-byte, MIME, regular-file, symlink/special-file rejection, and redaction checks before one atomic in-memory fake commit. Failure or cancellation rolls back to zero and production `run_artifacts` never change. Application and SQLite revalidate the full v48-v51 authority chain, live budgets, leases, and input Artifacts at both boundaries. CLI, events, and persistence retain no fixture body, raw path, command, Manifest, secret, container ID, or private lease. Simulated success is neither Docker availability evidence nor permission to execute.
The v52 release gate passed focused tests, full ordinary/race suites, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime/Runner/Markdown-link scans, diff checks, and an isolated real-binary full simulation smoke. The audit found no unresolved high- or medium-severity issue and strengthened redacted fixture digests, complete request fingerprints, SQL budget/input-Artifact revalidation, migration downgrade ordering, and the dual-layer eight-simulations-per-evidence limit. GitHub Actions run `29362181363` passed commit `f48cbb4`; its Go and TypeScript jobs completed in 2m9s and 19s. No real Provider, Shell, network, Local, Docker, production Artifact, or CTF execution was enabled.

Schema v53 adds immutable `sandbox_docker_observation.v1` facts while keeping container execution unreachable. The transport exposes only `Ping`, `Version`, `Info`, and digest-bound `InspectImage`; Linux uses the fixed `/var/run/docker.sock` endpoint and an allowlist of four GET request shapes, while Windows currently returns `transport_unsupported`. It neither calls the Docker CLI nor accepts `DOCKER_HOST`, arbitrary TCP endpoints, or caller-selected sockets, and its interface has no create/start/run/exec/pull/remove operation. A probe requires explicit CLI confirmation plus matching v52 evidence, output simulation, and full Manifest. Application and SQLite revalidate the complete v48-v52 identity, Policy, approval, budget, lease, input-Artifact, cancellation, and cleanup chain before accepting one of three bounded results: complete observation, unavailable daemon, or unavailable image. At most eight observations bind one simulation. Raw daemon identity, host name, Docker root, socket, RepoDigests, Manifest, and commands remain transient. `production_observed=true` means only that daemon and image metadata were read; private-mount support remains `not_observable_read_only`, and production verification, backend availability/enabling, execution, and Artifact-commit authority are all fixed false. The CLI cannot create, pull, start, execute in, or remove a container.
The v53 local release gate passed the final code's full ordinary/race suites (125.4s/140.7s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, exact test-key-prefix/runtime-artifact/Docker-mutation/Markdown-link scans, diff checks, and an isolated real-binary full-chain smoke. The smoke proves that an unconfirmed probe is rejected, Windows records only `transport_unsupported`, semantic replay does not reprobe, production Artifacts remain zero, and CLI output omits private Docker fields. The audit fixed two low-risk robustness issues: concurrent Application requests now converge by semantic intent even when they independently generate different candidate IDs/timestamps, and the HTTP core enforces its own path allowlist plus final method/path/query and exact JSON-media-type checks. Twenty ordinary and ten race repetitions of the two-Store contention regression pass. The TypeScript job in initial GitHub Actions run `29368112083` passed in 22s, but its Linux Go job exposed that the CLI unit test contacted the runner's real Docker daemon. Production defaults are unchanged; the test now injects a deterministic unavailable fake in process, while real-daemon access remains exclusive to explicit CLI confirmation and the opt-in integration test. GitHub Actions run `29368979988` passed fix commit `fe7b070`; its Go and TypeScript jobs completed in 2m7s and 23s. No unresolved high- or medium-severity issue was found.

Schema v54 adds deterministic `sandbox_docker_container_spec.v1`, immutable `sandbox_docker_container_plan.v1`, and in-memory-only `sandbox_docker_write_transaction.v1`. Only a complete and still-current v53 observation can enter the compiler; Application and SQLite again revalidate the full v48-v53 Manifest, identity, Policy, approval, budget, lease, input-Artifact, cancellation, and cleanup chain. The compiled specification fixes a non-root identity, read-only root and inputs, one writable output mount, `rprivate` propagation, default-deny networking with an exact allowlist, ephemeral secrets, CPU/memory/PID/time/kill limits, labeled orphan identity, and stop-before-export ordering. The full spec remains in memory; durable plans, events, and CLI output omit commands, arguments, paths, network targets, environment values, secret references, and container identity. Seven write steps are staged only by a fake harness, and failure, crash, or cancellation rolls back to zero. Even success fixes daemon writes, backend contact, production submission, execution authority, and Artifact-commit authority to zero. `run sandbox docker-plan|docker-plans|docker-plan-show` cannot mutate a real container.
The v54 local release gate passed the final code's full ordinary/race suites (128.2s/148.5s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, test-key-prefix/runtime-artifact/Docker-write/mojibake/Markdown-link scans, diff checks, and an isolated real-binary v54 migration/Workspace smoke. The audit strengthened v52 evidence/output-simulation requester continuity, explicit SQL v54 requester checks, and deep-copy isolation for fake-transaction snapshots. The two-Store contention regression passed 20 ordinary and 10 race repetitions. GitHub Actions run `29376503165` passed feature commit `126719f`; its Go and TypeScript jobs completed in 2m7s and 19s. No unresolved high- or medium-severity issue was found, and real Docker writes plus production Artifacts remain zero.

Schema v55 adds separate, default-disabled `sandbox_docker_write_transport.v1` and immutable `sandbox_docker_container_rehearsal.v1` protocols. Only an explicit operator confirmation plus a complete, still-current v54 plan can reach one Linux-local `/var/run/docker.sock` create-inspect-remove rehearsal. The initial profile is network-disabled, environment-free, and secret-free. The transport pins Docker API `1.40`, ignores `DOCKER_HOST`, and rejects TCP, caller-selected sockets, proxies, redirects, start, exec, attach, pull, logs, export, volume management, and generic requests. Its closed allowlist permits exact image inspect, create, container inspect, and non-forced delete with fixed `v=1` anonymous-volume cleanup. Before create, the local RepoDigest must match the plan and the image must declare no `VOLUME`. A digest-pinned, non-root, read-only, capability-dropped, resource-bounded container is created but never started; attachment, device, port, capability, configuration, and mount checks must pass before removal. A deterministic-name collision may be reconciled only when the stopped existing container exactly matches every expected configuration and authority label. Cancellation, failure, or an uncertain create response triggers an independent bounded re-inspection and deletes only an exact authority match, never a returned ID blindly. Persistence, events, and CLI retain metadata and fingerprints only, never raw container IDs, host paths, commands, environment values, secrets, socket paths, or full specifications. Operation replay never contacts the daemon again, and concurrent Stores converge. The normal path makes three daemon reads and two real writes, or three writes when removing one exact stale rehearsal container, while process execution, image pulls, output export, production verification, backend enabling, execution authority, and Artifact authority remain fixed false.
The v55 local release gate passed the final code's full ordinary/race suites (163.3s/168.7s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/forbidden-endpoint/mojibake/Markdown-link scans, diff checks, and an isolated real-binary schema-v55 Workspace smoke. The audit fixed two pre-release risks: an uncertain post-create response now triggers immediate deterministic-name reconciliation and failure cleanup never blindly deletes a returned ID; image-declared `VOLUME` side effects are rejected before create, delete has fixed anonymous-volume cleanup, and attachment/device/port/capability inspection is stricter. Twenty ordinary and ten race transport repetitions plus ten two-Store contention repetitions pass. No unresolved high- or medium-severity issue is known. The default-skipped Linux real-daemon write integration test was not executed because this Windows host has no available Docker; this residual verification gap grants no start or production authority. GitHub Actions run `29382661971` passed feature commit `69d81d6`; its Go and TypeScript jobs completed in 2m32s and 25s.

Schema v56 persists `sandbox_docker_container_rehearsal_attempt.v1` before any daemon mutation and fences concurrent operators with expiring, monotonically generated SQLite leases. The transport now has recoverable stage/cleanup phases: create leaves one inspected but never-started container in place, then a 19-item immutable control matrix records exact image, command, non-root, read-only-root, capability, network, empty-environment, mount-configuration, resource, port, device, attachment, authority-label, and never-started facts. Every item explicitly remains `execution_evidence=false`. Only after that checkpoint is durable does cleanup re-inspect the deterministic name and issue a non-forced delete. If a process exits around create response, stage commit, delete, or final commit, a later generation adopts the exact stopped authority match or accepts an already-absent container as idempotent cleanup; it never creates twice or removes a mismatched same-name container. Bounded failure codes are append-only and release the lease, while the legacy rehearsal, operation, v56 completion, and lease release commit atomically. Image and container inspection now reject inherited environment entries, closing the difference between “no Manifest environment” and “an actually empty container environment.” `docker-attempts` and `docker-attempt-show` expose metadata-only recovery state and controls. Raw container IDs, host paths, sockets, commands, environment values, secrets, and full specifications remain non-durable, and start, exec, attach, pull, logs, export, network, secrets, production verification, execution authority, and Artifact authority remain unreachable.

The v56 local release gate passed full ordinary/race suites (178.5s/181.3s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/forbidden-endpoint/new-mojibake/Markdown-link scans, diff checks, and an isolated real-binary schema-v56 Workspace smoke. Recovery transport repetitions passed 20 ordinary/10 race rounds, two-Store single-lease contention passed 20 ordinary/10 race rounds, and uncertain-create Application recovery passed 10 rounds. The audit fixed inherited-environment evidence, expired completion/failure fencing, failure-code ordering and the 16-entry ceiling, generation-acquisition chronology, stale-generation replay, orphaned legacy operations, attempt-ID recovery, SQL control mapping, and mount-TOCTOU overclaiming. No unresolved high- or medium-severity issue is known. The Linux real-daemon harness was not run on this Windows host without Docker, and host-mount TOCTOU remains for v57; neither gap grants start or production authority. GitHub Actions run `29388724727` passed feature commit `e1710bb`; its Go and TypeScript jobs completed in 2m32s and 23s.

Schema v57 adds a separate `sandbox_docker_host_input_staging.v1` rehearsal while the v56 container remains stopped. On Linux, `openat2` pins the workspace root and every read-only source tree with no-symlink, no-magic-link, beneath-only, and no-cross-device resolution. Each entry is preflighted with `O_PATH`, then only a matching regular file or directory is opened for content, so FIFOs and other special files fail before a potentially blocking open. Directory and single-file mounts are supported; symlinks, hard links, nested mounts, traversal, excessive depth, and bounded-resource violations fail closed. After all descriptors are pinned, inode, size, timestamps, and link counts are rechecked before a deterministic tar is assembled from sanitized source entries and exactly revalidated Run/Session/Workspace Artifacts. Filesystem-specific directory inode sizes are excluded from the content digest. The archive exists only in a `memfd`, receives write/grow/shrink/seal kernel seals, and is read back for digest verification. SQLite first stores an immutable intent bound to the current v56 attempt, container-ID fingerprint, plan, input digest, and lease generation, then stores counts and digests without paths, content, file descriptors, or raw container IDs. SQL blocks attempt completion while a v57 intent lacks evidence. `--stage-host-inputs` requires a separate `--confirm-host-input-staging`; failure performs best-effort stopped-container cleanup, and takeover can resume without another create. The sealed bundle is not yet handed to the Docker daemon, so `daemon_consumed=false` and `execution_evidence=false`: v57 closes the descriptor-capture race but is not production proof of bytes consumed by a future container and grants no start, exec, network, export, or Artifact authority.
The v57 local release gate passed the final code's full ordinary/race suites (155.0s/168.1s), vet, zero-warning staticcheck, module verification, zero-finding govulncheck, 17 frontend tests, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/process-entry scans, diff checks, and an isolated real-binary schema-v57 smoke. Ten Application staging rounds, twenty two-Store independent-ID contention rounds, and ten race contention rounds pass; the Linux test binary cross-compiles. Focused coverage includes rename/replacement/deletion/symlink/hard-link/FIFO/single-file mounts, bounded directory enumeration, cancellation, replay, lease takeover, migration, and privacy. The audit fixed single-file report/SQL constraints, cross-filesystem directory-digest stability, special-file pre-open blocking, pre-limit directory allocation, read cancellation latency, random-ID idempotency conflicts, and failure-ledger consumption after omitted confirmation, with no unresolved high- or medium-severity issue. The low-risk recovery gap recorded at the v57 release is now closed by v58.

Schema v58 adds immutable `sandbox_docker_host_input_requirement.v1` facts. For every new attempt, Go persists whether v57 host-input capture is mandatory in the same transaction that creates the attempt, first lease, and audit events, before any daemon stage. The fact binds the attempt and plan to Run/Mission/Workspace, Manifest/mount/input/authority fingerprints, read-only mount and input Artifact counts, operator confirmation, requester, and digest-only operation identity. A required attempt remains required after restart even when resume omits staging flags; it cannot complete without matching v57 evidence. A durable false choice cannot later widen into staging. Go and SQLite both enforce those rules, while events and queries retain only metadata and never paths, content, file descriptors, raw container IDs, raw operation keys, or private lease identities. Migration places preexisting v57 attempt IDs in an immutable compatibility set; it invents no historical choice and cannot become a requirement-free path for post-migration attempts. Schema v58 still does not hand the sealed bundle to Docker and adds no archive, volume, start, exec, pull, build, export, or Artifact authority. A fixed daemon-owned volume carrier, readback verification, and final read-only mount are isolated behind the next schema-v59 gate.

Schema v59 implements the default-disabled, four-confirmation daemon handoff without starting a container. Go commits an immutable handoff requirement with the attempt and a write-ahead intent before archive or volume mutation, binding the v58 requirement, v57 sealed report, attempt/plan, lease generation, stopped-container identity, and complete authority fingerprints. The Linux transport is restricted to the fixed local Unix socket, Docker API `1.40`, deterministic carrier/volume names, and `/cyberagent-input/bundle.tar`. It creates a daemon-owned local volume and never-started writable carrier, uploads the sealed bundle, reads it back through the daemon, verifies exact length and SHA-256, removes the carrier and original stopped target, briefly recreates the target with the volume read-only, verifies it, and removes both target and volume. Neither the target root nor Manifest mounts become writable, and overlapping user mounts are rejected. Exact crash residue converges on retry while foreign same-name resources are never deleted; failure cleanup also removes only fingerprint-matched owned objects. SQLite gates cleanup and completion on a required result, and every persisted claim still fixes start, exec, export, backend, execution, and Artifact authority to false. CLI flags plus metadata-only list/show projections expose no paths, content, raw IDs, resource names, socket, raw operation key, or private lease identity.

Schema v60 adds an explicitly operator-confirmed deterministic runtime-input projection plan that never contacts the daemon. Go revalidates the complete v48-v59 authority chain and recompiles the same Manifest/v54 specification before recapturing a short-lived v57 sealed bundle from the descriptor-safe provider. Its report, digest, length, counts, and Artifact payload must exactly match the completed v59 handoff. A strict parser accepts only byte-for-byte canonical v57 PAX tar data and rejects links, devices, traversal, duplicate or parentless paths, unexpected roots, empty Artifacts, trailing data, and non-canonical headers. The first profile supports directory-root read-only mounts and maps Artifacts only to `/cyberagent-input/artifacts`. Every root becomes one relative tar projection; future volume identity also includes the immutable handoff fingerprint so retries converge without cross-Run collisions. SQLite atomically commits plan, ordered items, completion marker, operation, and event while persisting only digests and bounded counts. Raw targets, host paths, file names/content, volume names, and archive bytes remain transient. Status is fixed to `compiled_not_applied`, with daemon contact/application, start, exec, export, backend, execution, and Artifact authority all false.

Schema v61 applies those transient v60 projections to Docker local volumes while still stopping at an inspected, never-started target. Separate operator and daemon-write confirmations are persisted with an immutable write-ahead intent and an independent generation lease before any Docker mutation. Go then revalidates v48-v60, recompiles the specification, resolves the writable output mount again, and recaptures the exact sealed input. The fixed Unix transport permits only exact image/container/volume inspection, local-volume and never-started carrier/target create/delete, and archive PUT/GET at fixed `/cyberagent-input`. Each projection is written into its own volume and semantically verified through daemon readback before read-only `NoCopy` attachment; the reviewed output bind is the only writable mount. Retry reconciles only complete authority matches, foreign collisions fail closed, stale generations cannot commit, and bounded independent cleanup precedes lease takeover. SQLite, events, and CLI retain only metadata and omit targets, host paths, file/resource names, raw IDs, archives, sockets, operation keys, and private leases. Success is `volumes_applied_target_never_started`; start, exec, export, backend, execution, and Artifact authority remain false. Windows is explicitly unsupported and never falls back to host execution.

Schema v62 adds a separate resource lifecycle for the never-started target and runtime-input volumes retained by v61. `docker-runtime-input-resource-inspect` requires an explicit read-only probe confirmation and revalidates the complete v48-v61 authority plus the resupplied Manifest without recapturing the input bundle. The fixed local-Unix transport reads only the target and every deterministic volume, then persists exact-owned, partial/absent, or foreign-collision state. Never-started, read-only, and `NoCopy` evidence is established only when the exact target and every volume are all present. Cleanup requires separate resource-cleanup and daemon-write confirmations. Go commits an immutable intent and generation lease before any DELETE; the transport preflights every resource first, guarantees zero DELETE after any foreign collision, otherwise removes the target by its inspected ID before exact volumes, and finally rechecks that everything is absent. Bounded failures release the lease for generation-based restart recovery. Lease rows cannot be deleted, failure/result timestamps must remain inside the current active lease window, and completion or semantic replay never contacts the daemon again. SQLite, events, and CLI retain no resource names, container IDs, host paths, sockets, Manifest bodies, raw operation keys, or private lease identities. No start, exec, attach, pull, export, backend, execution, or Artifact-commit surface exists, and Windows remains explicitly unsupported.

Schema v63 adds immutable `sandbox_docker_start_gate_review.v1` records that map every one of v51's sixteen threat checks to existing v52-v62 evidence and an explicit remaining production blocker. After completed v62 exact cleanup, an operator must resupply the complete Manifest and confirm the design review. Go revalidates the v48-v62 bindings without recapturing input, contacting the daemon, or creating a process. Every check stores a bounded evidence class/source, blocker code, and independent future gate, while all sixteen remain `production_verified=false` and `sufficient_for_start=false`. The same transaction stores an eleven-transition blueprint for future start/wait/TERM/KILL/orphan ownership with a per-Run generation-fenced single owner, write-ahead intent, fixed endpoint, cancellation fan-out, bounded logs, and orphan reconciliation; every transition remains `implemented=false` and `authorized=false`. The only review outcome is `blocked/deny_start`, with real-daemon-chain verification, start, process execution, output export, and Artifact commit all false. SQLite constraints, immutable triggers, digest-only idempotency, and metadata-only `docker-start-gate-review/list/show` CLI surfaces persist no resource names, raw container IDs, host paths, Manifest bodies, or raw operation keys.

The v63 local release gate passed the final code's full ordinary/race suites (196.9s/212.3s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift checks, production build, zero-vulnerability npm audit, credential/secret-file/forbidden-process-entry/replacement-character/diff scans, Linux sandbox test-binary cross-compilation, and an isolated real-binary schema-v63 Workspace/Skill smoke. Sandbox/Store/Application/CLI stress repetitions passed 20/15/10/10. The audit added immediate v63 child-row iteration-error handling and corrected internal error strings for the static rules; no unresolved high/medium issue is known. This Windows host still cannot execute the Linux v59/v61/v62 real-daemon opt-in chain, so all sixteen checks remain blocked and start stays absent. Desktop and custom-Skill packaging changed planning documents only; no dependency, installer, import endpoint, or authority was added. GitHub Actions run `29503856229` passed commit `e25a2ab`, with Go/Linux in 2m32s and TypeScript in 24s.

The following non-schema `skill_package.v1` validation slice passed the final code's full ordinary/race suites (239.4s/226.8s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, roughly 26.45 million parser fuzz executions over 20 seconds, 78.5% statement coverage for `internal/skills`, 100 parser and 20 CLI repeated regressions, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift checks, production build, zero-vulnerability npm audit, and credential/runtime-artifact/replacement-character/Markdown-link/diff scans. The audit pinned the ZIP creator version and exact Deflate-stream exhaustion to close hidden post-stream payloads, removed a deprecated test API, and prevented filesystem errors from echoing package paths. No unresolved high/medium issue is known. The slice adds no migration, installer, user Registry, Run selection, execution, or authority. GitHub Actions run `29512332025` passed commit `55b3fae`, with Go/Linux in 3m4s and TypeScript in 20s.

The v62 local release gate passed the final code's full ordinary/race suites (313.6s/329.6s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift checks, production build, zero-vulnerability npm audit, credential/runtime-artifact/forbidden-capability/replacement-character/Markdown-link scans, diff checks, Linux sandbox test-binary cross-compilation, and an isolated real-binary schema-v62 Workspace smoke. Sandbox/Store/Application/CLI stress repetitions passed 20/15/10/10. The audit fixed partial/unsafe evidence overclaiming, ambiguous event names, CLI foreign-collision reporting, future or out-of-window terminal timestamps, and deletable v61/v62 lease rows. No unresolved high- or medium-severity issue is known. This Windows host has no Docker, so the Linux v59/v61/v62 real-daemon opt-in chain was compiled but not executed; that evidence gap continues to block a start gate. GitHub Actions run `29444398815` passed feature commit `d250d32`; its Go/Linux and TypeScript jobs completed in 2m35s and 20s.

The v61 local release gate passed the final code's full ordinary/race suites (197.5s/316.8s), vet, zero-warning staticcheck, module verification, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/process-entry/forbidden-endpoint/replacement-character/Markdown-link scans, diff checks, Linux sandbox test-binary cross-compilation, and an isolated real-binary schema-v61 Workspace smoke. Sandbox race tests also passed 20 repetitions. The audit tightened per-volume readback bounds, pre-expiry cleanup reserve, future-lease and minimum-TTL rejection, dual confirmation at the Go resume boundary, cancellation-safe failure persistence with stable codes, least-capability transport factories for v55/v59/v61, Docker `RW`/`NoCopy` mount evidence, and operation-digest syntax. No unresolved high- or medium-severity issue is known. This Windows host has no Docker, so the default-skipped Linux v59/v61 real-daemon harnesses were compiled but not executed; that evidence gap continues to block a start gate. GitHub Actions run `29437941378` passed feature commit `f4aaf7a`; its Go/Linux and TypeScript jobs completed in 2m37s and 27s.

The v60 local release gate passed the final code's full ordinary/race suites (198.9s/194.0s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/replacement-character/Markdown-link scans, diff checks, Linux sandbox test-binary cross-compilation, and an isolated real-binary schema-v60 Workspace smoke. Compiler tests passed 50 repetitions, Store 30, Application 20, CLI 10, and the critical Sandbox/Store/Application race set 10 repetitions. The audit fixed non-durable confirmation, future volume collisions across Runs, an incorrect global item-fingerprint uniqueness constraint, trailing tar data, a deprecated xattr API, duplicate/out-of-range mount ordinals, plan chronology gaps, and canonical long-PAX-path compatibility. No unresolved high- or medium-severity issue is known. This Windows host still has no Docker, so the v59 handoff and now-implemented v61 application real-daemon harnesses require a Linux operator environment; that gap does not weaken v60's local compilation result or enable start. GitHub Actions run `29428011306` passed feature commit `cc92421`; its Go/Linux and TypeScript jobs completed in 2m48s and 24s.

The v59 local release gate passed the final code's full ordinary/race suites (183.1s/185.1s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/forbidden-endpoint/replacement-character/Markdown-link scans, diff checks, Linux sandbox test-binary cross-compilation, and an isolated real-binary schema-v59 Workspace smoke. Transport tests passed 50 ordinary repetitions, Store 30, Application 20, CLI 10, and transport/Store/Application race repetitions 20/10/10. The audit fixed early-failure leakage of the original stopped container, missing archive-readback media-type validation, reserved handoff-tree overlap, weak SQL endpoint/identity constraints, and an integration harness that had not exercised the real sealed provider. No unresolved high- or medium-severity issue is known. This Windows host has no Docker, so the default-skipped Linux real-daemon handoff harness was compiled but not executed; that residual verification gap grants no start authority and is not production execution evidence. GitHub Actions run `29406403201` passed feature commit `fb1daca`; its Go/Linux and TypeScript jobs completed in 2m37s and 28s.

The v58 local release gate passed the final code's full ordinary/race suites (158.1s/168.4s), vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/process-entry/replacement-character/Markdown-link scans, diff checks, Linux sandbox test-binary cross-compilation, and an isolated real-binary schema-v58 workspace smoke. Requirement domain tests passed 50 repetitions, Store candidate/missing-requirement tests 30, Application recovery/no-widen tests 20, and Store/Application race repetitions 10 each. The audit fixed pending operation-key recovery rebuilding the wrong candidate attempt, unmatched staging flags accepted beside a durable requirement, direct-SQL post-migration attempts lacking a requirement, and zero-read-only-mount compatibility for false requirements. No unresolved high- or medium-severity issue is known. This Windows host has no Docker, so the schema-v59 Linux real-daemon handoff harness has not run and no daemon-consumption claim is made.

GitHub Actions run `29400696276` passed feature commit `4b570f7`; its Go/Linux and TypeScript jobs completed in 2m39s and 23s.

Initial GitHub Actions run `29395980413` exposed only an invalid Linux single-file test fixture: it changed the sole mount target to a file while retaining a directory working directory, which the Manifest correctly rejected; production code did not fail. The corrected fixture retains one read-only directory mount covering the working directory and adds a separate file mount, exercising the intended `directory_count < read_only_mount_count` case. Run `29396264276` passed commit `8719dff`; Go/Linux and TypeScript completed in 3m55s and 23s.

The v47 release audit also hardens Supervisor concurrency recovery and budget accounting. Phase-transition replay rechecks the idempotency operation after observing the target state, cancellation tests wait for the Provider call to begin, and every root model call with a durable `model.started` fact is charged at least 1ms so a `999/1000ms` deadline cannot be entered repeatedly. This changes no schema, grants no capability, and preserves the interpretation of historical Specialist ledgers. GitHub Actions run `29325171043` passed every Go and TypeScript gate.

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

Schema v25 projects direct Specialist-child `dependency`, CompletionReport result, and crash notification messages into bounded root Supervisor context. Go prepares at most four sequence-ordered messages per turn and binds them to the exact Supervisor attempt through a `prepared -> committed/superseded` ledger. Consumption commits atomically only with the successful Session/lifecycle transaction; failure leaves messages pending, while cancellation, process restart, and lease takeover replay the same batch. The model receives only strict, redacted, truncated typed content: message IDs, sequence values, cursors, sender selection, and consumption control remain outside the prompt and under Go ownership. Public spawn and the child Provider loop remain disabled.

Schema v26 connects the first explicitly invoked, Go-internal, no-tool Specialist model turn. `specialist_lifecycle.v1` permits only `continue` or `finish` with a strict `agent_completion.v1` report; the model cannot declare usage, Agent/Attempt identity, leases, retries, Policy outcomes, or tool authority. `specialist_model_calls` atomically commits model terminal state, token and execution-time usage, Policy audit, redacted child Session messages, and Attempt usage so replay cannot charge twice. Provider retry, context cancellation, Run-lease heartbeat/fencing, stale-worker takeover recovery, and a 64 KiB aggregate history bound remain Go-owned. The Runner has no CLI, HTTP, or model-spawn entry and grants no tool, Shell, or network authority. Autonomous loops remain disabled; later schema v29 adds cancellation control only for an already-started exact child call.

Schema v27 injects direct-root `specialist_instruction.v1` messages and active child-owned WorkItems/Notes into Specialist context. Each Attempt prepares at most four sequence-ordered parent instructions. The `specialist_context_deliveries` two-phase ledger consumes them atomically only with successful `continue`/`finish`; crash, interruption, and lease takeover leave them pending for a fresh delivery. Context has independent 4,096-token and 32 KiB bounds. Message IDs and sender choice stay outside the model input, while ownership, consumption, leases, and terminal state remain Go/SQLite-owned. Content-free source IDs and token estimates are recorded in `model.started`; this context layer exposes no public spawn, autonomous child loop, or child tools.

Schema v28 gives a Specialist child exactly one bounded lifecycle-protocol repair. Global `model_attempt` numbers remain contiguous, while primary and repair phases own independent transport-retry counters. An invalid primary response first charges its real usage and atomically creates a pending `specialist_protocol_repairs` fact; successful repair usage accumulates on the same AgentAttempt. The repair request adds only a fixed diagnostic and never replays the raw invalid output, which is also excluded from Session history and Run events. A second invalid response becomes exhausted; cancellation, insufficient budget, and worker takeover become aborted; only a completed repair may continue or finish the child Attempt. Every turn rechecks remaining Run token and execution-time budgets before dispatch, and repair can consume only the post-primary remainder. This remains an internal Go no-tool path with no public spawn, CLI/HTTP write surface, Shell, or network authority.

The Go-internal `SpecialistScheduler` owns one Run execution lease per schedule and runs at most two explicitly selected ready children per round. It has a hard 32-round bound and stops on completion, no-ready state, first child error, parent cancellation, token exhaustion, or execution-time exhaustion. Parent cancellation, lease-heartbeat failure, and the first non-recoverable child error fan out to the other child; all Attempts persist terminal state before the lease is released. Before and after every round, aggregate usage is rebuilt from SQLite: root tokens/time come from the Supervisor checkpoint, child tokens are reconciled between Agent projections and Attempt rows, and all child model-call durations are summed. At schema v29 this was an internal-only construction path; schema v38 exposes it only through the operator application gate and still grants no HTTP, model-spawn, ordinary-tool, Shell, or network authority.

Schema v29 adds recoverable control facts to that internal path. Every schedule records `agent.schedule_started` under the current Run lease, then persists a completed, failed, or cancelled summary containing target children, rounds, started turns, recovered attempts, stop reason, and before/after aggregate usage. A later lease generation converges an orphaned running schedule exactly once to `abandoned/worker_lost`. The same migration adds a separate Specialist cancellation ledger: the control API can submit only a redacted intent bound to the exact `run_id + agent_id + agent_attempt_id + model_attempt`, and only the worker holding that Attempt's private lease may observe it and cancel its local Provider context. Model terminal state and cancellation resolution commit atomically. Raw idempotency keys, model text, `lease_id`, and fencing generations are absent from responses and schedule/cancellation events. This POST grants no admission, spawn, tool, Shell, or network authority. The scheduler's existing first-error policy may still cancel an active sibling locally, but it does not fabricate another remote cancellation request.

Schema v30 adds the strict `specialist_delegation.v1` proposal protocol. Root may use `specialist_delegation_propose` to suggest one or two bounded assignments with titles, goals, parent-Skill subsets, and turn/token budgets. After charging the tool-call budget, Go revalidates the active root, Supervisor checkpoint, execution lease, Run/Session/Workspace binding, remaining child capacity, non-delegable capabilities, and root budget headroom before atomically committing the redacted proposal, assignments, Policy/domain/tool events, and digest-only idempotency fact. Identical semantic intent converges to one proposal across rounds, restarts, and independent SQLite connections. The ledger stores no raw operation key, Provider call ID, model text, `lease_id`, or fencing generation, and those fields are absent from events as well. Every proposal remains immutable and `proposed` with `admission_authorized=false`: it creates no Agent or Session, reserves no budget, and starts no scheduler. Actual review, admission, and spawn remain behind a later Go-internal/operator policy. `run delegations` and `run delegation` are read-only inspection commands.

Schema v31 separates operator review from both the proposal and execution authorization. `run delegation approve/reject` can append exactly one immutable `approved` or `rejected` fact for a proposal. Rejection requires a reason; the Store redacts that reason and events never contain it. A digest-only operation ledger safely replays the same key and intent, while changed intent or a second decision conflicts. Approval requires a still-running Run, whereas rejection may close a proposal after the Run becomes terminal. Metadata-only `agent.delegation_reviewed` always says `admission_authorized=false` and `application_required=true`, so approval creates no child, Session, reservation, instruction, or schedule and has no model or ordinary-tool entry point.

Schema v32 adds explicit `run delegation apply` for an approved proposal. At application creation, Go rechecks current Policy, the review operation, running Run, idle root and child scheduler, active root Session, parent Skills, child policy/capacity, and aggregate budgets. It then idempotently enters the existing admission path for one or two assignments and sends one strict `specialist_instruction.v1` to each child. Application, assignment, and operation rows persist deterministic internal operation digests before either external write, so a restart after Agent or Message commit recovers the same entities without duplicates or untracked children. While applying, root turns, unrelated admissions/messages, and Specialist schedules/Attempts are blocked. Terminal Run transitions atomically mark the application `aborted` with a metadata event. Success leaves children ready and starts no Attempt or scheduler; models, Providers, HTTP clients, and ordinary tools still cannot approve, apply, spawn, or schedule. Its independent audit leaves no known high- or medium-severity issue; final full-repository tests, race detection, vet, static analysis, module verification, and vulnerability scanning all pass.

Schema v33 starts a read-only Fan-out path that is structurally separate from core delegation. `run fanout plan` accepts `auto/1/2/4/6` parallelism tiers, but six exists only inside the `readonly_fanout.v1` workspace-list/read capability envelope; `MaxAgentChildren=2`, v30/v32 assignments, and the Specialist scheduler remain unchanged. The Go scanner accepts only regular UTF-8 source files within the workspace, skips symlinks, VCS, dependency/build directories, binaries, and secret-like files, and builds a SHA-256 manifest plus deterministic shards under hard limits of 20,000 walked entries, 256 files, 128 KiB per file, and 768 KiB total. Schema v33 stores immutable plan/file/shard rows and a digest-only operation ledger. Replays are exact, while events omit the goal, file paths, and workspace root. At the v33 boundary, only `plan/list/show` was exposed and reported `execution_authorized=false`; schema v34 below adds Provider fan-out and model results, while automatic code writes remain disabled. Its independent audit leaves no known high- or medium-severity issue; full tests, race/static analysis, module verification, vulnerability/credential/artifact scans, and an isolated real CLI smoke all pass.

Schema v34 adds an explicit read-only execution gate for that immutable plan. Under a Run execution lease, `run fanout execute` rebuilds the complete manifest and uses Go `os.Root` to recheck every file identity, size, and SHA-256; any added, removed, changed, or symlink-drifted file fails before a Provider call. Verified source text exists only in memory and is credential-redacted before a tool-free JSON-mode `readonly_fanout_report.v1` request. The model cannot request Shell, writes, processes, network, external tools, or recursive children. Go runs the 1/2/4/6 workers concurrently and cancels siblings on the first failure. Schema v34 persists execution, shard, model-call, finding, and digest-only idempotency ledgers while events remain metadata-only. SQLite now reconciles root, core Specialist, and read-only Fan-out token/model-time usage together. An uncertain crash call is conservatively charged at its reservation; a newer lease generation fences the old worker, marks the call `abandoned`, and retries only incomplete shards. Core Agent graph and two-child limits are unchanged: Fan-out creates no Agent, Attempt, or schedule and performs no automatic code write.

Schema v35 deterministically projects completed shard results into the generic Finding/Evidence/Report boundary. Go deduplicates only assertions whose severity, category, title, detail, path, and line range are identical; no model call may merge findings or rewrite severity. Every duplicate retains immutable `model_assertion` Evidence, while the projected confidence uses the conservative minimum. A single SQLite transaction advances `finding_reports` from `building` to `generated` only after source-v34 bindings, contiguous ordinals, counts, and severity totals agree; generated Report, Finding, and Evidence rows are immutable. Every projected Finding starts as `draft`, not as a validated vulnerability. `run fanout report` and `report show` render byte-stable Markdown or JSON from persisted facts without another Provider call or any new Agent, tool, or network authority.

Schema v36 adds a separate immutable operator-validation overlay for draft Findings. `report finding attach` binds a frozen Artifact from the same Run as Evidence after the Store rereads the full blob and verifies size, SHA-256, MIME, stream, tool, source, and redaction metadata; SQLite now rejects Artifact updates and deletes. `validated` requires at least one Artifact Evidence record, while `rejected` may record a failed reproduction without one. Each Finding receives at most one decision. Only domain-separated operation-key digests are persisted, narrative fields are redacted and omitted from Run events, and the original v35 Finding and projection digest never change.

Schema v37 adds an immutable `validated -> accepted -> fixed` remediation lifecycle outside that validation overlay. `report finding accept` is an independent operator decision that freezes the exact validation ID, Evidence count, and digest; validation never implies acceptance. After acceptance, `report finding remediation attach` accepts only a new same-Run Artifact whose durable `artifact.created` event follows `finding.accepted`. Validation Artifacts and outputs created before acceptance cannot be reused. `report finding fix` requires at least one such remediation Evidence record and freezes its ordered count and digest. All three mutations use digest-only idempotency ledgers, metadata-only events, cross-connection convergence, and update/delete triggers while leaving the v35 source Finding and projection digest unchanged.

Schema v38 adds an explicit operator scheduling gate for core children already applied and instructed by v32. `run delegation schedule/continue` must be issued by the original application operator, may select only one or two ready children from that application, and reuses the existing `SpecialistScheduler`, Run execution lease, aggregate token/model-time budget, sibling cancellation fan-out, and 32-round hard limit. One operation key identifies one immutable request and one recoverable execution: identical replay returns the original schedule, changed intent conflicts, and a new key is required for another explicit continuation. Digest-only immutable request, target, operation, and attempt ledgers reserve pending targets from ordinary internal scheduling. After worker loss, the same request may fence the orphaned schedule to `abandoned/worker_lost` under a newer lease generation and recover. Models, Providers, HTTP clients, and ordinary `tool invoke` still cannot create a request, select children, or start the scheduler; the core child limit remains two.

Schema v39 adds one Profile-matched, aggregate-budgeted Skill selection to each `created` Run plus a digest-only idempotency ledger. Items are deterministically name-sorted and pin `name/version/content_sha256/content_bytes/token_upper_bound`; Go and SQLite both enforce contiguous ordinals, accounting, Run/Mission/Profile binding, and immutability. An identical operation can replay after the Run starts, while any new selection or changed intent fails closed. Models, HTTP, and the Tool Gateway have no creation path.

Schema v40 delivers selected embedded Skills to the root Supervisor under a separate budget. Each turn reconstructs and validates a redacted `skill_context.v1`; a metadata-only `prepared -> committed` ledger binds the root Agent, Supervisor attempt, and first model call and converges after restart or through another SQLite connection. Skill bodies live only in the current Go process's Provider request and never enter SQLite or Run events.

The report runtime also provides read-only SARIF 2.1.0, CI-gate, and GitHub Actions annotation projections without a database write or Provider call. SARIF `results` include only confirmed unresolved Findings, meaning `validated` and `accepted`; `fixed`, `draft`, and `rejected` entries remain status counts and cannot become upload alerts. `cyberagentValidationStatus` retains the actual validation decision, while `cyberagentFindingStatus` exposes the current lifecycle. The default `validated/high` gate also blocks accepted unresolved findings, explicit `active` additionally admits drafts, and fixed/rejected findings never block. `--format github` renders file/line-aware `notice/warning/error` workflow commands from the same in-memory `GateResult`, strictly escaping model-produced `%`, CR/LF, colons, and commas and rendering other C0/DEL controls visibly. It emits no Artifact content, Evidence note, or operator reason and preserves the existing gate exit status.

The project also includes its first local HTTP control plane. `cyberagent api serve` binds only to loopback. `CYBERAGENT_API_TOKEN` authorizes reads, while default-disabled control routes require a distinct `CYBERAGENT_API_CONTROL_TOKEN`. Stable `api.v1` envelopes and bounded cursor pagination expose durable metadata without fencing tokens. The three POST routes can only request cancellation of an exact root/Specialist model call or select a non-authorizing Run execution profile; they cannot execute tools, start processes, alter Policy, accept a client fencing token, or read Artifact content/checkpoint pending input. The API has no CORS or WebSocket support and never persists either token.

The Go response DTOs now generate the single OpenAPI 3.1 contract. `cyberagent api openapi` emits deterministic JSON, `--output docs/openapi.json` refreshes the tested repository snapshot, and an authenticated running server returns the same raw document at `/api/v1/openapi.json`. Contract tests exercise every published route and prevent drift among DTOs, the committed document, and handlers while explicitly excluding Artifact content, checkpoint pending input, `lease_id`, and fencing tokens.

A read-only Run Event Stream now shares that control plane. `/api/v1/runs/{run_id}/events/stream` uses SSE to publish redacted durable SQLite events. Every frame id is an opaque Run-bound cursor that supports exact `Last-Event-ID` resume. Batch size, frame size, total events, lifetime, write timeout, and process concurrency are bounded; heartbeats never fabricate Run events, and server shutdown cancels long-lived connections. The stream contains no user-visible model text, accepts no token query, and performs no write operation itself.

The React/Vite read-mostly console lives under `web/`. It generates TypeScript DTOs from `docs/openapi.json`, reads Workspaces, Runs, Sessions, the bounded Agent graph, operator-gated delegations, read-only Fan-out summaries, Finding/Report projections, WorkItems, Notes, Artifact descriptors, ToolRounds, budgets, leases, and schema v64 execution profiles, and consumes resumable SSE with authenticated `fetch`. An optional distinct control bearer enables the profile segmented control and schema-v72 controlled Run creation; TypeScript submits only the Workspace/goal/Profile/surface/phase intent or profile enum and adopts the Go-returned state. A production build can be hosted on the same loopback origin by `cyberagent api serve --ui-dir web/dist`; Go loads the validated bundle into an immutable startup snapshot and enforces bounded HTML fallback, immutable hashed-asset caching, and a strict CSP. Both tokens and uncertain-failure creation keys stay in page memory and never enter a URL, localStorage, or sessionStorage. Browser DTOs omit Workspace roots, Artifact content, private decision narratives, raw Fan-out model reports, digests, and lease/fencing identities. The browser does not duplicate Go policy. The loopback Vite proxy remains available for frontend development.

The Bubble Tea TUI now starts with a Run-first picker that can switch between the latest 50 Runs and latest 50 Sessions; `cyberagent tui --run <run-id>` opens one exact Run. A Run exposes its Work Board, Notes, durable Supervisor Tool Rounds, recent Run Events, Agent graph, bounded Finding-report summaries, Shell ToolRuns, and read-only details for the latest 20 FileEdit diffs. The preview query excludes original and complete replacement file bodies, while displayed diffs are capped at 128 KiB and 4096 lines. The TUI follows durable event sequences under hard bounds, pins the Run and Mission, discards stale asynchronous results, and compares the event tail before and after each composite read so Session, tool, and Run projections come from a stable window; terminal Runs stop polling. Its tool pane supports “approve once” and “approve for this session.” Every non-Tools tab and the diff detail screen are read-only. Session approval remains scoped to the exact Run, Session, Workspace, Shell tool, and ActionClass, while dangerous commands remain permanently denied. Terminal controls are rendered visibly, and wrapping/truncation accounts for CJK, combining characters, and other wide graphemes.

`cyberagent headless events` provides versioned `headless.v1` NDJSON over the same redacted durable Run timeline. It reads and resumes by SQLite sequence under hard bounds and may wait for a terminal Run with `--follow`, but it neither executes the Run nor calls a Provider or writes state. Every event and final `stream.end` record occupies one line. Completed, failed, cancelled, event-limit, and follow-timeout outcomes return stable exit codes 0, 4, 7, 8, and 9 respectively. Stdout remains NDJSON-only while diagnostics stay on stderr.

## 核心能力 / Core Capabilities

- **可恢复运行 / Resumable runs:** durable checkpoints, cross-process execution leases with heartbeat/fencing, exact cross-process active-call cancellation, bounded execution, restart recovery, ordered exactly-once operator steering at safe turn boundaries, graceful terminal cancellation, and explicit lifecycle actions.
- **Agent Coordinator:** stable root identity, atomic Run/Supervisor status projection, bounded durable inboxes, hashed exactly-once send/finish intent, explicit wake/dependency semantics, internal-only two-child admission and concurrent scheduling, durable schedule summaries, shared-lease cancellation fan-out, exact cross-process child-call cancellation, aggregate Run budget reconciliation, lease-fenced child Attempts, crash/takeover recovery, attempt-bound CompletionReports, two-phase exactly-once root and Specialist instruction context, child-owned memory selection, a durable no-tool Specialist model ledger, and one isolated child lifecycle repair; public/model spawn and autonomous scheduling remain disabled.
- **统一模型网关 / Model gateway:** route-based providers, Anthropic-compatible SSE/tool protocol, typed failures, application-owned active-call cancellation, bounded live progress, one lifecycle-protocol repair, and durable model/tool events.
- **长上下文与结构化记忆 / Long-context memory:** persisted sessions, automatic compaction, durable categorized Notes, visibility rules, and token-budgeted source selection.
- **Skill Registry 与最小上下文 / Skill Registry and minimal context:** Go-owned strict `skill.v1` guides, one immutable version/hash-pinned `skill_selection.v1` per Run, full selected guidance for root, and a separately budgeted metadata-audited subset of at most one guide for each Specialist Attempt; no capability grant.
- **规划与交付 / Plan and delivery:** strict three-direction `plan_delivery.v1` proposals, operator-only idempotent selection, atomic projection into existing WorkItems and a pinned handoff Note, and an explicit separate Deliver-phase transition; no capability grant.
- **结构化任务板 / Structured Work Board:** Run-scoped work items, dependency and cycle checks, optimistic versions, transactional events, and bounded Supervisor context.
- **发现项与报告 / Findings and reports:** immutable model-assertion provenance, deterministic exact-fact deduplication, Artifact-backed validation, independent acceptance, fresh remediation Evidence, fixed-state snapshots, stable source projection digests, Go-rendered Markdown/JSON, confirmed-unresolved SARIF 2.1.0, an explicit CI gate, and GitHub Actions annotations from the same GateResult.
- **受控多 Agent 提案 / Controlled multi-Agent proposals:** strict review-gated `specialist_delegation.v1`, at most two assignments, parent-Skill and budget checks, digest-only replay, and no model-controlled admission or spawn.
- **本地工作区 / Local workspace:** scoped file access, safe reads, hashed Run artifacts, reviewable edit proposals, a Go-owned read-only Git status projection, and bounded multi-file change-set summaries that preserve per-file review/apply authority.
- **统一工具网关 / Unified Tool Gateway:** normalized calls, trusted workspace binding, policy decisions, shared review, first-class non-executable `script_process.v1` proposals, a bounded resumable Provider loop for create-only WorkItem/Note tools, bounded UTF-8 results, MIME metadata, and compatibility adapters.
- **Sandbox 生命周期边界 / Sandbox lifecycle boundary:** strict `sandbox_manifest.v1`, shared approval review, exact Manifest resubmission, `os.Root` mount resolution, transactional budget/lease rechecks, immutable schema-v48/v49 intent facts, schema-v50 Artifact-bound cancellation/cleanup with generation fencing, schema-v51 disabled backend/output preflight, schema-v52 simulation-only backend evidence plus atomic fake output transactions, schema-v53 fixed-endpoint read-only Docker observations, schema-v54 deterministic plans, schema-v55 default-disabled create-inspect-remove rehearsals, schema-v56 crash-recoverable daemon intents, schema-v57 descriptor-pinned/kernel-sealed host-input evidence, schema-v58 durable pre-stage requirements, schema-v59 daemon-owned/readback-verified/cleaned handoff evidence, schema-v60 strict metadata-only runtime-input projection plans, schema-v61 recoverable read-only volume application to a retained never-started target, schema-v62 retained-resource inspection plus recoverable exact-owned cleanup, schema-v63 immutable blocked start-gate evidence, schema-v64 non-authorizing profile selection, schema-v65 fixed production-evidence receipts, schema-v66 generation-fenced write-ahead capture attempts, schema-v67 bounded Linux read-only daemon evidence, and schema-v68 immutable non-authorizing receipt review; Local and container-process execution remain disabled.
- **安全与审批 / Safety and approval:** policy checks, secret redaction, automatic low-risk reads, durable per-call approvals, revocable scoped Session grants, atomic Run tool budgets, permanent denial, and dry-run command completion.
- **完整审计链 / Audit trail:** append-only Run events plus a bounded resumable SSE projection for messages, context source provenance, text-free model progress, model calls, Notes, policy decisions, tool proposals, file edits, and content-free Artifact metadata.
- **CLI、Headless 与 TUI / CLI, Headless, and TUI:** a scriptable CLI, resumable bounded `headless.v1` NDJSON with stable outcome exits, and a Run-first Bubble Tea interface with live model progress, audited cancellation, event-driven Run/Agent/Finding views, bounded read-only FileEdit diffs, and scoped once/session approvals.
- **可扩展架构 / Extensible architecture:** Go control plane with a generated OpenAPI 3.1 contract, loopback-only API/SSE, a React/Vite read-mostly console for controlled Run creation plus Run/Agent/delegation/Fan-out/report state and non-authorizing profile selection, separately authorized controls, and planned Docker sandbox and Rust analyzer boundaries.

> [!NOTE]
> 当前版本仍在积极开发中。Provider 只能创建 WorkItem/Note 或记录不具执行权的 Plan/委派提案；双 child 并发必须经过显式 operator review、application 与 v38 schedule request，模型不能自主启动。Web/Desktop 的少量 mutation 均由独立 Go capability 控制，包括精确取消、Run 档位、FileEdit 提案/审阅/apply、凭证设置和有界 wake；真实 Shell/容器命令、更广工具面、模型自主子 Agent 调度和 CTF 自动求解尚未开放。schema v63 的全部启动检查仍不足以启动，schema v64-v66 不授予执行权限；schema v67 仅在 Linux 显式 opt-in 后授予五次固定只读 daemon GET，schema v68 即使接纳该回执也仍不授权写入、启动或进程执行。<br>
> This project is under active development. Providers may create WorkItems/Notes or non-executable Plan/delegation proposals only, and two-child concurrency remains explicitly operator-gated. A small set of Web/Desktop mutations is independently gated by Go, including exact cancellation, Run profiles, FileEdit propose/review/apply, credential setting, and bounded wake consumption. Real Shell/container commands, broader model tools, model-driven child scheduling, and automated CTF solving are not enabled. The schema v63 checks remain insufficient and schemas v64-v66 grant no execution authority. Schema v67 permits only five fixed read-only daemon GETs after explicit Linux opt-in, and a schema v68 receipt acceptance still grants no daemon write, start, or process authority.

**密钥边界 / Secret boundary:** 应用数据库、事件、日志和浏览器存储不会持久化 API key；可选在线 Provider 可从当前进程环境变量读取，Windows Desktop/API 也可在显式 capability 下交给 Windows Credential Manager 保存。Go 只向界面返回配置状态，不回读明文。<br>
API keys never enter the application database, events, logs, or browser storage. Optional live providers may read the current process environment; an explicitly enabled Windows Desktop/API control may instead store them in Windows Credential Manager. Go returns status only and never sends plaintext back to the UI.

## Build Requirements

- Go 1.25 or newer; use the latest patch release for standard-library security fixes
- CGO enabled
- A Windows C compiler toolchain, such as MinGW-w64 GCC

On this machine, validation uses Go 1.26.5. WinLibs MinGW-w64 was installed through winget and Go user env was configured with `CGO_ENABLED=1` plus `CC` pointing at the installed `gcc.exe`.

## Quick Start

```powershell
go run ./cmd/cyberagent version
go run ./cmd/cyberagent provider list
go run ./cmd/cyberagent skill list
go run ./cmd/cyberagent skill list --profile review
go run ./cmd/cyberagent skill show code
go run ./cmd/cyberagent skill validate
go run ./cmd/cyberagent skill package validate <package.zip>
go run ./cmd/cyberagent skill import <package.zip> --surface code --operation-key <stable-key> --confirm-untrusted-skill
go run ./cmd/cyberagent skill installed --surface code
go run ./cmd/cyberagent skill installed show <name>@<version>
go run ./cmd/cyberagent skill select-external <run-id> <name>@<version> --operation-key <stable-key> --confirm-untrusted-skill-context [--specialist <name>@<version>] [--token-budget 2048]
go run ./cmd/cyberagent skill external-selection <run-id>
go run ./cmd/cyberagent skill remove <name>@<version> --operation-key <stable-key> --confirm-remove
go run ./cmd/cyberagent workspace init demo
go run ./cmd/cyberagent workspace tree demo
go run ./cmd/cyberagent workspace read demo README.md
go run ./cmd/cyberagent sandbox template
go run ./cmd/cyberagent sandbox validate configs/sandbox-manifest.example.json
go run ./cmd/cyberagent run create "review this workspace" --workspace demo --profile review --surface code --phase plan --max-tool-calls 100
go run ./cmd/cyberagent run sandbox prepare <run-id> --manifest configs/sandbox-manifest.example.json --operation-key <stable-key>
go run ./cmd/cyberagent run sandbox list <run-id>
go run ./cmd/cyberagent run sandbox show <preparation-id>
go run ./cmd/cyberagent run sandbox request <preparation-id> --operator <operator-id>
go run ./cmd/cyberagent run sandbox review <preparation-id> --decision approve --operation-key <stable-key> --reviewer <operator-id>
go run ./cmd/cyberagent run sandbox candidate <preparation-id> --manifest configs/sandbox-manifest.example.json --approval <approval-id> --operation-key <stable-key>
go run ./cmd/cyberagent run sandbox candidates <run-id>
go run ./cmd/cyberagent run sandbox candidate-show <candidate-id>
go run ./cmd/cyberagent run mode <run-id>
go run ./cmd/cyberagent run execution-profile <run-id>
go run ./cmd/cyberagent run execution-profile set <run-id> docker --operation-key <stable-key> --reason "prefer isolated execution"
go run ./cmd/cyberagent run plans <run-id>
go run ./cmd/cyberagent run plan show <proposal-id>
go run ./cmd/cyberagent run plan choose <proposal-id> 2 --operation-key <stable-key>
go run ./cmd/cyberagent run plan selection <run-id>
go run ./cmd/cyberagent run phase <run-id> deliver --operation-key <stable-key> --reason "plan accepted"
go run ./cmd/cyberagent run delivery checkpoint <work-id> --operation-key <stable-key> --focused "focused tests passed" --diff-audit "diff reviewed" --security-audit "security boundary reviewed" --handoff "slice handoff"
go run ./cmd/cyberagent run delivery checkpoint <final-work-id> --operation-key <stable-key> --focused "focused tests passed" --diff-audit "diff reviewed" --security-audit "security boundary reviewed" --handoff "module handoff" --functional "full suite passed" --robustness "race and failure paths passed"
go run ./cmd/cyberagent run delivery list <run-id>
go run ./cmd/cyberagent run delivery show <checkpoint-id>
go run ./cmd/cyberagent run steer enqueue <run-id> "review the current diff" --operation-key <stable-key>
go run ./cmd/cyberagent run steer cancel <steering-id> --operation-key <stable-key> --reason "requirement withdrawn"
go run ./cmd/cyberagent run steer drain <run-id> --max-steps 1
go run ./cmd/cyberagent run steer list <run-id>
go run ./cmd/cyberagent run steer show <steering-id>
go run ./cmd/cyberagent todo complete <work-id>
go run ./cmd/cyberagent skill select <run-id> review --operation-key <stable-key> --token-budget 4096
go run ./cmd/cyberagent skill selection <run-id>
go run ./cmd/cyberagent session send <run-session-id> "inspect the current workspace"
go run ./cmd/cyberagent session send <run-session-id> "queue this exactly once" --operation-key <stable-key>
go run ./cmd/cyberagent run adapt-task <legacy-task-id>
go run ./cmd/cyberagent run list
go run ./cmd/cyberagent run show <run-id>
go run ./cmd/cyberagent run events <run-id>
go run ./cmd/cyberagent headless events <run-id> --max-events 1000
go run ./cmd/cyberagent headless events <run-id> --after-sequence <n> --follow --timeout 30m
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
go run ./cmd/cyberagent run delegations <run-id>
go run ./cmd/cyberagent run delegation <proposal-id>
go run ./cmd/cyberagent run delegation approve <proposal-id> --operation-key <stable-key> [--reason <text>]
go run ./cmd/cyberagent run delegation reject <proposal-id> --operation-key <stable-key> --reason <text>
go run ./cmd/cyberagent run delegation apply <proposal-id> --operation-key <stable-key> [--operator cli_operator]
go run ./cmd/cyberagent run delegation schedule <proposal-id> --operation-key <stable-key> [--operator cli_operator] [--max-rounds 1] [--agent <agent-id>]
go run ./cmd/cyberagent run delegation continue <proposal-id> --operation-key <new-stable-key> [--operator cli_operator] [--max-rounds 1] [--agent <agent-id>]
go run ./cmd/cyberagent run fanout plan <run-id> "audit source modules" --tier auto --path . --operation-key <stable-key>
go run ./cmd/cyberagent run fanouts <run-id>
go run ./cmd/cyberagent run fanout show <plan-id>
go run ./cmd/cyberagent run fanout execute <plan-id> --operation-key <stable-key> --max-output-tokens 1024
go run ./cmd/cyberagent run fanout execution <execution-id>
go run ./cmd/cyberagent run fanout report <execution-id> --format markdown
go run ./cmd/cyberagent report show <report-id> --format json
go run ./cmd/cyberagent report show <report-id> --format sarif
go run ./cmd/cyberagent report check <report-id>
go run ./cmd/cyberagent report check <report-id> --fail-status active --min-severity medium --format json
go run ./cmd/cyberagent report check <report-id> --min-severity high --format github
go run ./cmd/cyberagent report finding attach <finding-id> <artifact-id> --operation-key <stable-key> --note "reproduction output"
go run ./cmd/cyberagent report finding validate <finding-id> --operation-key <stable-key> --reason "Artifact confirms the finding"
go run ./cmd/cyberagent report finding reject <finding-id> --operation-key <stable-key> --reason "not reproducible"
go run ./cmd/cyberagent report finding accept <finding-id> --operation-key <stable-key> --reason "risk accepted for remediation"
go run ./cmd/cyberagent report finding remediation attach <finding-id> <fresh-artifact-id> --operation-key <stable-key> --note "post-acceptance remediation output"
go run ./cmd/cyberagent report finding fix <finding-id> --operation-key <stable-key> --reason "fresh evidence confirms the correction"
go run ./cmd/cyberagent report finding verify <finding-id>
go run ./cmd/cyberagent tool schema
go run ./cmd/cyberagent tool schema work_item_create
go run ./cmd/cyberagent tool schema specialist_delegation_propose
go run ./cmd/cyberagent tool schema plan_delivery_propose
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
go run ./cmd/cyberagent edit review-approve <run-id> <edit-id>
go run ./cmd/cyberagent edit review-deny <run-id> <edit-id>
go run ./cmd/cyberagent edit apply <run-id> <edit-id> --operation-key <stable-apply-key>
go run ./cmd/cyberagent edit approve <non-run-legacy-edit-id>
go run ./cmd/cyberagent run wake schedule <run-id> --operation-key <stable-key>
go run ./cmd/cyberagent run wake show <run-id>
go run ./cmd/cyberagent run wake cancel <run-id> --operation-key <stable-cancel-key>
go run ./cmd/cyberagent run wake consume <run-id> --max-steps 1
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
go run ./cmd/cyberagent tui --run <run-id>
go run ./cmd/cyberagent tui --run <run-id> --print
go run ./cmd/cyberagent tui --session <session-id>
go run ./cmd/cyberagent tui --session <session-id> --print
go run ./cmd/cyberagent script new "parse pcap http token" --workspace demo
go run ./cmd/cyberagent script run scripts/<script-name>.py --workspace demo --local --idempotency-key <stable-key>
go run ./cmd/cyberagent ctf init baby-web --category web
go run ./cmd/cyberagent context compact --workspace demo --task task-demo --message "user: imported challenge" --message "assistant: summarized plan"
go run ./cmd/cyberagent context show --task task-demo
```

### Web Console

```powershell
# repository root: build once, then run one loopback process
cd web
npm ci
npm run check:api
npm run build
cd ..
$env:CYBERAGENT_API_TOKEN = "<local-read-token>"
$env:CYBERAGENT_API_CONTROL_TOKEN = "<optional-distinct-control-token>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765 --ui-dir web/dist
```

打开 / Open `http://127.0.0.1:8765`. Vite 双进程开发方式、详细边界与检查命令见 [web/README.md](web/README.md)。

控制台当前提供 Workspace 选择和受控 Run 创建、Run/Session 浏览、只读 Repository、Code Journey、多文件独立审阅、脱敏模型可用性、Agent 图、operator-gated delegation、只读 Fan-out 摘要、Finding/Report、Work/Notes/Artifact metadata/ToolRound、可恢复事件流、操作者行动中心、证据清单和 `Ctrl+K`。显式 capability 可分别开放 Session/Run/Plan/审批、FileEdit 提案/审阅/apply、Provider 凭证设置、wake 意图/前台消费/有界 worker、证据附加和惰性 Skill 安装；每条路径都经过 Go/SQLite 的状态、Policy、预算、幂等与事件边界。真实 Git/Shell、Docker、LocalRunner 与模型自主 child 调度仍未开放。<br>
The console covers Workspace and Run/Session operations, read-only Repository state, a Code Journey, independent multi-file review, redacted model availability, Agent/delegation/Fan-out metadata, Findings/Reports, Work/Notes/Artifacts/ToolRounds, resumable events, operator actions, evidence inventory, and `Ctrl+K`. Independent capabilities expose Session/Run/Plan/approval controls, FileEdit propose/review/apply, Provider credential setting, wake intent/foreground consumption/bounded worker, evidence attachment, and inert Skill installation. Every path remains behind Go/SQLite state, Policy, budget, idempotency, and event boundaries. Real Git/Shell, Docker, LocalRunner, and model-driven child scheduling remain unavailable.

Use `CYBERAGENT_HOME` to point runtime data at another directory during tests or experiments.

在 Unix 上，新建的运行目录与 SQLite 数据库分别限制为 `0700` 和 `0600`；Windows 继续使用系统 ACL。LocalRunner 当前显式 fail-closed，不会启动宿主机进程，Noop dry-run 输出也会先脱敏。<br>
On Unix, newly created runtime directories and SQLite databases are restricted to `0700` and `0600`; Windows continues to use system ACLs. LocalRunner is explicitly fail-closed and cannot start a host process, while Noop dry-run output is redacted before display.

`api serve` generates and prints a temporary read token when `CYBERAGENT_API_TOKEN` is absent. Model-call cancellation, execution-profile selection, controlled Run creation, Session enqueue/cancel/evidence attachment, Run lifecycle, bounded execution handoff, Plan/Deliver, approval decisions, model diagnostics/routes, Provider credentials, FileEdit propose/review/apply, wake intent/foreground consumption/worker, and Skill installation remain disabled unless a distinct `CYBERAGENT_API_CONTROL_TOKEN` is supplied and the corresponding Go capability is enabled; environment-provided tokens are validated but never echoed. The optional wake worker is the only background consumer and is hard-capped at one due intent and one Supervisor step per tick; it cannot start Shell/Local/Docker. See [docs/http-api.md](docs/http-api.md) for endpoints, envelopes, pagination, and security boundaries.

### 交互式编辑、系统凭证与有界唤醒 / Interactive Editing, System Credentials, And Bounded Wake

- **FileEdit 提案编辑器：** Go 只为当前 running Run、active Session 和已注册 Workspace 发放五分钟、单次意图的不透明 source handle。Monaco 与 Diff editor 使用本地打包 worker，只编辑经过 secret 检查且完整、未截断、未替换的安全 UTF-8；提交仅含 handle 和新文本。过期句柄仅在当前文件仍匹配旧 SHA-256 时换发，持久化 pending proposal 只能恢复成 `editable=false` 的只读 Diff。创建、review 与 apply 继续使用各自独立 capability。
- **Provider 系统凭证：** Windows 使用 Credential Manager 保存 `mimo`、`deepseek` 或 `anthropic` 密钥。TypeScript 只能提交一次密码输入，响应和状态列表固定 `plaintext_returned=false`；SQLite、事件、日志、模型上下文和前端持久化均不保存密钥。修改后 Go 会先构建完整候选 Registry，再原子切换 generation；失败保留旧 generation，成功时无需重启。非 Windows 平台 fail closed，不使用明文文件回退。
- **有界 wake worker：** `--enable-wake-worker` 默认关闭，必须同时具备 control token。每个进程只有一个串行 owner，每轮最多领取一个已到期 intent，并通过现有 Foreground Wake Consumer、RunSupervisor、预算、Policy、lease、checkpoint 和取消路径执行最多一步。只读 health 展示 `ready|running|draining|stopped`，不返回 token、owner、lease、Run 或私有错误；它没有 Tool Runner、Shell、LocalRunner 或 Docker 依赖。

- **FileEdit proposal editor:** Go issues a five-minute opaque source handle bound to one running Run, its active Session, and a registered Workspace. Locally bundled Monaco workers edit only complete, untruncated, unreplaced safe UTF-8 that passed secret checks. An expired handle rotates only if the current file still matches the old SHA-256; a durable pending proposal recovers only as an `editable=false` Diff. Create, review, and apply retain independent capabilities.
- **Provider system credentials:** Windows Credential Manager stores optional `mimo`, `deepseek`, or `anthropic` secrets. TypeScript can submit one password value, while every response remains status-only with `plaintext_returned=false`; SQLite, events, logs, model context, and frontend persistence never store the key. Go builds a complete candidate Registry and atomically swaps its generation; failure retains the old generation and success requires no restart. Non-Windows platforms fail closed without a plaintext-file fallback.
- **Bounded wake worker:** `--enable-wake-worker` is default-off and requires the control token. One serial owner may claim at most one due intent per tick and execute at most one step through the existing Foreground Wake Consumer, RunSupervisor, budgets, Policy, leases, checkpoints, and cancellation. Read-only health exposes `ready|running|draining|stopped` without token, owner, lease, Run, or private error data. It has no Tool Runner, Shell, LocalRunner, or Docker dependency.

Windows Desktop 与普通 `api serve` 现在都从 Go 取得精确 capability。普通浏览器使用
只读 `runtime_capabilities.v1` 决定是否显示控件，并观察固定 1 x 1 worker 的 health；
该投影不能启用 worker、安装服务或授予任何 mutation。每个控制 route 仍独立验证
control token 与自身 capability，TypeScript 不是安全边界。

Windows Desktop and ordinary `api serve` now receive exact capabilities from Go. The
browser uses read-only `runtime_capabilities.v1` to render controls and observe the
fixed 1 x 1 worker health. That projection cannot enable the worker, install a service,
or grant any mutation; every control route still verifies its distinct token and
capability independently, so TypeScript is not a security boundary.

### 仓库状态、多文件审阅与 Code Journey / Repository State, Change Sets, And Code Journey

- `repository_state.v1` 只检查已注册 Workspace 根目录中的真实 `.git` 目录。Go 不向上发现父仓库、不接受重定向 worktree、不跟随 `.git` 内符号链接、不启动 Git 进程/网络/hook，也不返回宿主根路径、文件正文或 remote 配置；元数据扫描、状态处理和返回条目分别有硬上限。
- `file_edit_change_set.v1` 汇总最多 100 个 exact Run/Session/Workspace FileEdit 的状态和 Diff 字节数。它明确声明 review/apply 独立、非原子、无 batch mutation，并显示 partial apply；每个文件仍使用原有独立 operation、Policy/hash 复核和回执。
- Code-only Journey 把 Scope、Plan、Queue and execute、Review、Verify and report 导航到既有 Go 能力。该 React 组件没有 API client，不创建新的复合 mutation，也不会自动套用到 Cyber 模式。

- `repository_state.v1` inspects only a real `.git` directory at the registered Workspace root. Go performs no parent discovery, redirected-worktree traversal, nested Git-metadata symlink traversal, subprocess, network, or hook execution, and returns no host root, file body, or remote configuration. Metadata scanning, status processing, and returned items are independently bounded.
- `file_edit_change_set.v1` summarizes at most 100 exact Run/Session/Workspace FileEdits and their Diff byte counts. It explicitly preserves independent review/apply, non-atomic partial state, and no batch mutation; every file retains its existing operation, Policy/hash recheck, and receipt.
- The Code-only Journey navigates Scope, Plan, Queue and execute, Review, and Verify and report through existing Go capabilities. The React component has no API client or composite mutation and does not automatically apply to Cyber mode.

### Windows Desktop (through schema v84 / D1-G11, D1-V10)

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-desktop.ps1
# Release-candidate diagnostic build: compile twice and compare SHA-256.
powershell -ExecutionPolicy Bypass -File scripts/build-desktop.ps1 -VerifyReproducible
go run ./cmd/cyberagent doctor portable --json
.\build\desktop\cyberagent-desktop.exe

# Optional: expose only the existing non-authorizing v64 profile selector.
.\build\desktop\cyberagent-desktop.exe --enable-profile-control

# Optional: expose only schema-v72 controlled Run creation.
.\build\desktop\cyberagent-desktop.exe --enable-run-creation

# Optional: expose only durable Session-message submission.
.\build\desktop\cyberagent-desktop.exe --enable-session-messages

# Optional: expose pending-only Session steering cancellation.
.\build\desktop\cyberagent-desktop.exe --enable-session-steering-control

# Optional: expose idempotent Run start/pause/resume.
.\build\desktop\cyberagent-desktop.exe --enable-run-lifecycle

# Optional: expose explicit bounded queue execution through RunSupervisor.
.\build\desktop\cyberagent-desktop.exe --enable-run-execution

# Optional: expose operator Plan selection and explicit Deliver transition.
.\build\desktop\cyberagent-desktop.exe --enable-plan-delivery

# Optional: expose bounded approve-once/deny decisions.
.\build\desktop\cyberagent-desktop.exe --enable-approvals

# Optional: expose explicit Provider diagnostics and persisted routes.
.\build\desktop\cyberagent-desktop.exe --enable-model-control

# Optional: store/delete Provider keys through Windows Credential Manager.
.\build\desktop\cyberagent-desktop.exe --enable-provider-credentials

# Optional: create a pending FileEdit proposal from Go-issued Workspace text.
.\build\desktop\cyberagent-desktop.exe --enable-file-edit-proposals

# Optional: expose review-only Diff decisions.
.\build\desktop\cyberagent-desktop.exe --enable-file-edit-review

# Optional: expose durable wake intent; no background worker is started.
.\build\desktop\cyberagent-desktop.exe --enable-run-wake

# Optional: independently apply an already reviewed Diff.
.\build\desktop\cyberagent-desktop.exe --enable-file-edit-apply

# Optional: consume one due wake through the foreground RunSupervisor path.
.\build\desktop\cyberagent-desktop.exe --enable-run-wake-execution

# Optional: start one default-off, 1 x 1-step wake worker.
.\build\desktop\cyberagent-desktop.exe --enable-wake-worker

# Optional: confirm inert local Skill package registration.
.\build\desktop\cyberagent-desktop.exe --enable-skill-installation

# Optional: attach one exact Workspace file as non-authorizing Session evidence.
.\build\desktop\cyberagent-desktop.exe --enable-evidence-attachments

# Optional: record immutable operator verification evidence.
.\build\desktop\cyberagent-desktop.exe --enable-verification-evidence

# Capabilities may be combined; host/container process execution remains disabled.
.\build\desktop\cyberagent-desktop.exe --enable-file-edit-review --enable-file-edit-apply --enable-run-wake --enable-run-wake-execution
```

The desktop shell embeds the same React production UI and calls the Go API in process; it opens no listening port and needs no manually copied bearer token. It requires Windows 10/11 with WebView2 Evergreen Runtime, fails closed without implicitly downloading it, and resumes event polling from a Run-bound high-water cursor. The default launch is read-only. Explicit flags independently expose each Go control. FileEdit proposal, read-only recovery, review, and apply remain separate; credentials are status-only and reload a complete Registry generation. The optional wake worker is a single process-lifetime 1 x 1-step consumer with bounded health/drain and cannot start Local, Docker, Shell, or another host/container process. The reproducible executable is still an unsigned development/portable-test artifact, not an installer or formal release; automated diagnostics keep `release_ready=false` until the Windows 10/WebView2 manual matrix is signed. See [docs/DESKTOP_PLAN.md](docs/DESKTOP_PLAN.md), [ADR 0034](docs/adr/0034-embedded-read-first-wails-shell.md), and the current read/runtime boundary in [ADR 0055](docs/adr/0055-history-navigation-verification-pagination-runner-runtime-evidence.md).

schema v72 / D1-R1 本地发布门已通过：全仓普通与 race 测试分别用时 271.5 秒和 257.9 秒，普通/安全 Desktop 测试与 vet/staticcheck/govulncheck 均通过；React 为 14 个文件 45 项测试，严格 TypeScript、生产构建、确定性 OpenAPI 生成和 npm 零高危漏洞检查均为绿色。审计修复了旧 schema 降级夹具中的 v72 trigger 拆除顺序、创建与旧 Run 控制权限串联风险、重放初态复核、非法 UTF-8 JSON、初始 root/时间线/事件总数约束，以及响应 Goal/Workspace/模式绑定和 UTF-8 字节边界；当前没有已知未解决高/中风险。<br>
The schema-v72/D1-R1 local release gate passed the full ordinary and race suites in 271.5s and 257.9s, ordinary/secure-Desktop tests plus vet/staticcheck/govulncheck, 45 React tests across 14 files, strict TypeScript, the production build, deterministic OpenAPI generation, and the zero-high-severity npm audit. The audit fixed historical-schema trigger teardown order, capability coupling, initial-state replay validation, invalid UTF-8 JSON, exact root/timeline/event-count binding, response binding for Goal/Workspace/mode, and UTF-8 byte limits. No unresolved high/medium issue is known.

非 schema Desktop D1-S1 采用新的三切片批次交付：第一片增加 Go/Application 与严格 HTTP Session message submission，第二片增加独立 Desktop bootstrap capability，第三片增加 React composer、内存内不确定失败重试和 metadata-only 状态反馈。最终代码的全仓普通 Go 测试直接运行通过（255.6 秒），Desktop-tag 聚焦测试（80.5 秒）、Windows production build、15 个前端文件 52 项测试、严格 TypeScript 与 Vite production build 也通过。完整 race/staticcheck/govulncheck 健壮性门按新的六切片节奏在下一批结束时执行；本批没有调用 Provider、工具、Shell、Docker、外部网络或取得 execution lease。<br>
Non-schema Desktop D1-S1 uses the new three-slice batch cadence: Go/Application plus strict HTTP submission, an independent Desktop bootstrap capability, and a React composer with memory-only uncertain-failure retry and metadata-only feedback. A direct full ordinary Go run on the final code passed in 255.6s, along with focused Desktop-tag tests in 80.5s, the Windows production build, 52 frontend tests across 15 files, strict TypeScript, and the Vite production build. GitHub Actions run `29633205163` passed implementation commit `3ecb22a` across Go/Linux, Windows Desktop, and TypeScript. The full local race/staticcheck robustness gate is scheduled after the next batch reaches six slices; remote CI already passed vet, govulncheck, and dependency audit for this commit. This batch called no Provider, tool, Shell, Docker, external network, or execution lease.

schema v73 / D1-S2、D1-L1、D1-X1 完成第二个三切片批次：pending-only Session 取消、幂等 Run start/pause/resume，以及通过既有 RunSupervisor 的最多八步冻结队列执行。累计六切片完整门禁通过最终代码的全仓普通测试（268.2 秒）和 race（295.3 秒）、普通/secure-Desktop vet 与零告警 staticcheck、双路径零可达漏洞 govulncheck、module verify/tidy diff、16 个文件 66 项前端测试、严格 TypeScript、确定性 OpenAPI/TypeScript、Vite 与 Windows production build、零漏洞 npm audit 和隐私/产物/危险入口扫描。最终未签名 GUI 为 20,849,664 字节，SHA-256 `ce3ff2b4609068de996b6362e3a5008c4d2348eae73c48ad0661c4e22739eba5`。审计修复了延迟生命周期重放、prepared 项误显示可取消、旧 lease/改意图终态重放、`maxSteps` 重试键复用和一处前端测试索引误判；当前无已知未解决高/中风险。<br>
Schema v73 / D1-S2, D1-L1, and D1-X1 complete the second three-slice batch: pending-only Session cancellation, idempotent Run start/pause/resume, and an at-most-eight-step frozen queue handoff through the existing RunSupervisor. The full six-slice gate passed the final code's ordinary suite in 268.2s and race suite in 295.3s, ordinary/secure-Desktop vet and zero-warning staticcheck, both zero-finding govulncheck paths, module verification/tidy diff, 66 frontend tests across 16 files, strict TypeScript, deterministic OpenAPI/TypeScript generation, Vite and Windows production builds, zero-vulnerability npm audit, and privacy/artifact/forbidden-entry scans. The final unsigned GUI is 20,849,664 bytes with SHA-256 `ce3ff2b4609068de996b6362e3a5008c4d2348eae73c48ad0661c4e22739eba5`. The audit fixed delayed lifecycle replay, misleading cancellation for prepared items, stale-lease/changed-intent terminal replay, `maxSteps` retry-key reuse, and a false-positive frontend assertion. No unresolved high/medium issue is known.

非 schema D1-M1、D1-P1、D1-A1 完成第三个三切片批次：统一 Go Provider Registry/脱敏模型可用性、Plan 三选一与显式 Deliver、metadata-only 审批队列与受限 approve-once/deny。最终普通全仓 Go（310.1 秒）、Windows Desktop tag、73 项 React 测试、严格 TypeScript、Vite/Windows production build 和零漏洞 npm audit 均通过；OpenAPI 更新为 36 个 path、84 个 schema、27 个 GET 和 11 个 control POST。本批审计修复模型标识误含密钥时的投影风险、终态 Run 已提交审批的幂等重试、前端审批身份绑定和一处新弹窗编码问题。没有调用 Provider 网络、真实 Shell/Local/Docker 进程、文件写入或 Session Grant；当前无已知未解决高/中风险。<br>
Non-schema D1-M1, D1-P1, and D1-A1 complete the third three-slice batch: one Go Provider Registry with redacted model availability, three-way Plan selection plus explicit Deliver, and a metadata-only approval queue with constrained approve-once/deny. The final ordinary Go suite passed in 310.1 seconds, along with Windows Desktop-tag tests, all 73 React tests, strict TypeScript, Vite/Windows production builds, and the zero-vulnerability npm audit. OpenAPI now contains 36 paths, 84 schemas, 27 GET operations, and 11 control POST operations. The audit closed secret-like model-identifier projection, terminal-Run replay of an already committed approval, frontend approval identity binding, and one new-dialog encoding defect. No Provider network call, real Shell/Local/Docker process, file write, or Session Grant occurred; no unresolved high/medium issue is known.

schema v75/D1-Q2、schema v76/D1-D2 与 D1-B1 三切片普通功能门通过：最终全仓 Go 为 333.1 秒，聚焦 race、Windows Desktop tag、85 项 React 测试、严格 TypeScript、确定性 OpenAPI/TypeScript、Vite/Windows production build、vet、module verify/tidy、npm 零漏洞、隔离 CLI smoke，以及隐私/UTF-8/链接/产物/危险入口扫描均为绿色。未签名 GUI 为 23,107,584 字节，SHA-256 `309b0e556c44960d7739ab1159e61ede632a443a87ff0b006e6151a288a38626`。组合审计修复了 prepared wake 过期重领/中途取消、失败调用事实失真、FileEdit 恢复时权限过期、同 Edit 双 operation 和直接截断写入风险；无已知未解决高/中风险。强杀发生在暂存完成但原子替换前时可能残留一个已脱敏隐藏临时文件，这是已记录低风险并进入下一批 recovery receipt。完整 race/staticcheck/govulncheck 门仍按六切片节奏在下一批完成。<br>
The schema-v75/D1-Q2, schema-v76/D1-D2, and D1-B1 ordinary three-slice gate passed the final 333.1s Go suite, focused race checks, Windows Desktop tags, all 85 React tests, strict TypeScript, deterministic OpenAPI/TypeScript generation, Vite/Windows production builds, vet, module verification/tidy, zero-vulnerability npm audit, isolated CLI smoke, and privacy/UTF-8/link/artifact/forbidden-entry scans. The unsigned GUI is 23,107,584 bytes with SHA-256 `309b0e556c44960d7739ab1159e61ede632a443a87ff0b006e6151a288a38626`. The audit closed prepared-wake lease reclamation/cancellation, false failed-call facts, stale FileEdit recovery authority, duplicate operations per Edit, and direct-truncation write risk. No unresolved high/medium issue is known. A forced process kill between staging and atomic replacement can leave one redacted hidden temporary file; this recorded low risk moves into the next recovery-receipt slice. The full race/staticcheck/govulncheck gate remains scheduled at the next six-slice boundary.

非 schema D1-U1/D1-E1/D1-W1 完成累计六切片门禁：最终全仓普通/race 分别为 294.0 秒和 338.3 秒，普通/secure-Desktop tests 与 vet、零告警 staticcheck、零可达漏洞 govulncheck、module verify/tidy、22 个文件 88 项 React 测试、strict TypeScript、确定性 OpenAPI/TypeScript、Vite build、零漏洞 npm audit、隔离 CLI smoke、隐私/产物扫描和真实 Windows 双构建均为绿色。OpenAPI 现在有 47 个 path、106 个 schema、31 个 GET 和 19 个 control POST。双构建产物 SHA-256 为 `33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`，自动兼容检查全部通过；Windows 10/WebView2 人工矩阵仍使 `release_ready=false`。审计修复了失败回执误显示成功、非规范 Explorer 路径与跨目录条目权限、OpenAPI 活路由夹具、发布就绪误报、PowerShell 5.1 API 不兼容和一项既有 Go 错误文本规范问题；当前无已知未解决高/中风险。<br>
Non-schema D1-U1/D1-E1/D1-W1 complete the cumulative six-slice gate. Full ordinary/race suites passed in 294.0s/338.3s, along with ordinary/secure-Desktop tests and vet, zero-warning staticcheck, zero-finding govulncheck, module verification/tidy, 88 React tests across 22 files, strict TypeScript, deterministic OpenAPI/TypeScript, the Vite build, zero-vulnerability npm audit, isolated CLI smoke, privacy/artifact scans, and a real reproducible Windows double build. OpenAPI now has 47 paths, 106 schemas, 31 GET operations, and 19 control POST operations. The double-build SHA-256 is `33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`; every automated compatibility check passed, while the manual Windows 10/WebView2 matrix correctly keeps `release_ready=false`. Audit fixes cover failed-receipt success presentation, non-canonical Explorer paths and cross-directory entry authority, the OpenAPI live-route fixture, release-readiness overstatement, Windows PowerShell 5.1 compatibility, and one pre-existing Go error-text convention. No unresolved high/medium issue is known.

远端 GitHub Actions [run 29658783000](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29658783000) 已通过提交 `5f0f397`：Go control plane 5 分 49 秒、TypeScript console 32 秒、Windows Desktop shell 2 分 11 秒。Remote GitHub Actions run 29658783000 passed commit `5f0f397`: Go control plane 5m49s, TypeScript console 32s, and Windows Desktop shell 2m11s.

schema v77 / D1-E2/D1-C1/D1-U2 三切片普通功能门通过：最终全仓 Go 297.9 秒、Windows Desktop tag、23 个文件 92 项 React 测试、strict TypeScript、`go vet`、module verify/tidy、确定性 OpenAPI/TypeScript、Vite 与可复现 Windows 双构建、隔离 mock-only CLI smoke、npm 零漏洞均为绿色。OpenAPI 当前为 50 个 path、53 个 operation 和 112 个 schema；未签名 GUI SHA-256 为 `d187601e9e9d8cb0d4ee644e3c9aa1c7617905580b001ef7955dbc35b8c47af3`，自动兼容检查通过但 `release_ready=false`。审计修复了 Unicode 大小写映射偏移、搜索真实读取上限少算 UTF-8 前瞻，并把 canonical evidence reference 约束复制到 SQLite；当前无已知未解决高/中风险。本批是新组六切片的前三片，完整 race/staticcheck/govulncheck 门在下一批结束执行。<br>
The schema-v77 D1-E2/D1-C1/D1-U2 ordinary gate passed the uncached Go suite in 297.9s, Windows Desktop tags, 92 React tests across 23 files, strict TypeScript, `go vet`, module verification/tidy, deterministic OpenAPI/TypeScript, Vite and reproducible Windows double builds, isolated mock-only CLI smoke, and the zero-vulnerability npm audit. OpenAPI now has 50 paths, 53 operations, and 112 schemas. The unsigned GUI SHA-256 is `d187601e9e9d8cb0d4ee644e3c9aa1c7617905580b001ef7955dbc35b8c47af3`; automated compatibility passed while `release_ready=false` remains correct. The audit fixed Unicode case-mapping offsets, accounted for UTF-8 look-ahead in the true search read ceiling, and duplicated canonical evidence-reference constraints in SQLite. No unresolved high/medium issue is known. These are the first three of the next six slices; the full race/staticcheck/govulncheck gate runs after the next batch.

远端 GitHub Actions [run 29661764283](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29661764283) 已通过实现提交 `ffbdc72`：TypeScript console 34 秒、Windows Desktop shell 2 分 21 秒、含 govulncheck 的 Go control plane 3 分 48 秒。Remote GitHub Actions run 29661764283 passed implementation commit `ffbdc72`: TypeScript console 34s, Windows Desktop shell 2m21s, and the Go control plane including govulncheck 3m48s.

非 schema D1-O1/D1-C2/D1-K1 完成累计六切片完整健壮性门：最终全仓 ordinary/race 分别为 319.6 秒和 299.8 秒，普通/secure-Desktop test、vet、零告警 staticcheck、零可达漏洞 govulncheck、module verify/tidy、26 个文件 97 项 React 测试、strict TypeScript、确定性 OpenAPI/TypeScript、Vite build、npm 零漏洞、隔离 mock-only CLI smoke、仓库隐私扫描与真实 Windows 可复现双构建全部通过。OpenAPI 为 51 个 path、55 个 operation、116 个 schema；未签名 GUI SHA-256 为 `a89b2357a5f1e7376ea8a533356028ccd5ea5eaec388b14d7623343fd041f520`，自动检查通过但 `release_ready=false`。真实浏览器桌面/移动视口审计验证 Actions、Evidence 与 `Ctrl+K`，并修复事件版本漂移和失败重连占满连接的问题；当前无已知未解决高/中风险。本批没有调用真实 Provider、Shell、LocalRunner、Docker、网络、API key、安装器、注册表、自启动或更新。<br>
Non-schema D1-O1/D1-C2/D1-K1 complete the cumulative six-slice robustness gate. Full ordinary/race suites passed in 319.6s/299.8s, along with ordinary/secure-Desktop tests, vet, zero-warning staticcheck, zero-finding govulncheck, module verification/tidy, 97 React tests across 26 files, strict TypeScript, deterministic OpenAPI/TypeScript, Vite build, zero-vulnerability npm audit, isolated mock-only CLI smoke, repository privacy scans, and a real reproducible Windows double build. OpenAPI has 51 paths, 55 operations, and 116 schemas. The unsigned GUI SHA-256 is `a89b2357a5f1e7376ea8a533356028ccd5ea5eaec388b14d7623343fd041f520`; automated checks pass while `release_ready=false` remains correct. Real-browser desktop/mobile auditing verified Actions, Evidence, and `Ctrl+K`, and fixed event-version drift plus failed-reconnect connection exhaustion. No unresolved high/medium issue is known. No real Provider, Shell, LocalRunner, Docker, network, API key, installer, registry mutation, startup task, or updater was used.

远端 GitHub Actions [run 29665187925](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29665187925) 已通过实现提交 `1151aaf`：TypeScript console 36 秒、Windows Desktop shell 2 分 23 秒、Go control plane 3 分 35 秒。Remote GitHub Actions run 29665187925 passed implementation commit `1151aaf`: TypeScript console 36s, Windows Desktop shell 2m23s, and Go control plane 3m35s.

非 schema D1-I1/D1-M3/D1-J1 完成本轮三个产品切片：Go 发放 source handle 的本地 Monaco 提案/Diff 编辑器、Windows Credential Manager 的 Provider 凭证边界，以及默认关闭且固定单并发/单步的 wake worker。最终全仓普通 Go 测试 327.6 秒通过，`go vet`、secure Desktop tag、28 个文件 102 项 React 测试、strict TypeScript、确定性 OpenAPI/TypeScript、Vite build 和 npm 零漏洞审计均通过；OpenAPI 当前为 55 个 path、59 个 operation 和 122 个 schema。Windows 可复现双构建 SHA-256 为 `a0e6aa0a3d15ccc39712f8a0a64d7de06e4a6af426e060b6378b1011c93a1cf6`，`release_ready=false`。组合审计修复凭证长度与精确 provider 名校验、单 Provider 凭证读取失败造成全局不可用、关闭后 worker 可重启、编辑提案不确定保存后的 ID/意图漂移、审批后重试误报内部错误、未配置 Provider 的 `models:null` 前端契约回归、前端密钥测试快照以及 Monaco CDN/依赖漏洞风险。桌面和 390x844 移动宽度的本地 UI 冒烟通过。当前无已知未解决高/中风险；本批没有使用真实 API key、Provider 网络、Shell、LocalRunner 或 Docker。完整 race/staticcheck/govulncheck 门按六切片节奏在下一批结束后执行。<br>
Non-schema D1-I1/D1-M3/D1-J1 complete this three-slice product batch: a local Monaco proposal/Diff editor over Go-issued source handles, a Windows Credential Manager boundary for Provider secrets, and a default-off wake worker fixed at one concurrent one-step handoff. The final full ordinary Go suite passed in 327.6s, together with `go vet`, secure Desktop tags, 102 React tests across 28 files, strict TypeScript, deterministic OpenAPI/TypeScript, the Vite build, and a zero-vulnerability npm audit. OpenAPI now has 55 paths, 59 operations, and 122 schemas. The reproducible Windows double build has SHA-256 `a0e6aa0a3d15ccc39712f8a0a64d7de06e4a6af426e060b6378b1011c93a1cf6` and remains `release_ready=false`. The combined audit fixed credential-size and exact-provider validation, whole-app failure from one credential-store read error, worker restart after close, FileEdit ID/intent drift after an uncertain save, incorrect internal errors for post-review replay, a `models:null` frontend contract regression for unconfigured Providers, frontend secret-test snapshotting, and Monaco CDN/dependency risk. Local UI smoke passed at desktop and 390x844 mobile widths. No unresolved high/medium issue is known; no real API key, Provider network, Shell, LocalRunner, or Docker was used. The complete race/staticcheck/govulncheck gate follows after the next batch reaches six slices.

GitHub Actions run `29671519260` 已通过实现提交 `ee36405`：TypeScript、Windows
Desktop 和 Go control plane 分别用时 42 秒、2 分 31 秒和 3 分 54 秒。<br>
GitHub Actions run `29671519260` passed implementation commit `ee36405`: TypeScript,
Windows Desktop, and the Go control plane completed in 42s, 2m31s, and 3m54s.

非 schema D1-I2/D1-M4/D1-J2 完成累计六切片健壮性门。编辑器现在可在旧 SHA-256
仍匹配时换发过期 source handle，并把持久化 pending FileEdit 恢复为无句柄、不可编辑
的只读 Diff；stale 文件不会自动 rebase。Provider credential 修改会先构建完整候选
Registry 并原子切换 generation，失败继续服务旧 generation，活跃调用使用已捕获的旧
Provider，后续调用使用新 Provider。`GET /api/v1/capabilities` 让普通浏览器和 Desktop
共享 Go-owned capability/worker health；worker 只能呈现 `ready|running|draining|stopped`，
不能运行时启用或成为服务。<br>
Non-schema D1-I2/D1-M4/D1-J2 complete the cumulative six-slice robustness gate. An
expired editor source handle rotates only while the old SHA-256 still matches, and a
durable pending FileEdit recovers as a handle-free, non-editable Diff without silently
rebasing stale content. Provider credential changes build a complete candidate
Registry and atomically swap generations; failure keeps serving the old generation,
active calls retain their captured Provider, and later calls use the new Provider.
`GET /api/v1/capabilities` gives ordinary Web and Desktop the same Go-owned capability
and worker-health projection; `ready|running|draining|stopped` is observable without
runtime enablement or service authority.

最终 uncached 全仓 ordinary/race 分别为 322.9 秒和 352.8 秒；`go vet`、零告警
staticcheck、零可达漏洞 govulncheck、module verify/tidy、secure Desktop tags、29 个文件
108 项 React 测试、strict TypeScript、确定性 OpenAPI/TypeScript、Vite、npm 零漏洞和
Windows 可复现双构建全部通过。OpenAPI 为 57 个 path、61 个 operation、125 个 schema；
未签名 GUI SHA-256 为 `30a3d9d19e02f32f8ea976fc071bc6942ed06fba3e7cad937310a78e46e74dfc`，
自动检查通过但 `release_ready=false`。桌面/移动真实浏览器冒烟无横向溢出或 console
错误。审计修复混合 Registry generation、凭证读取失败替换当前 Registry、并发 reload
误报、worker 无 control token 构造/并发 `RunOnce`/nil context、missing-file proposal
恢复和恢复对话框失败态。当前无已知未解决高/中风险；未使用真实 key、Provider 请求、
Shell、LocalRunner、Docker、攻击流量或外部网络。<br>
The final uncached ordinary/race suites passed in 322.9s/352.8s, together with `go
vet`, zero-warning staticcheck, zero-finding govulncheck, module verification/tidy,
secure Desktop tags, 108 React tests across 29 files, strict TypeScript, deterministic
OpenAPI/TypeScript, Vite, zero-vulnerability npm audit, and a reproducible Windows
double build. OpenAPI has 57 paths, 61 operations, and 125 schemas. The unsigned GUI
SHA-256 is `30a3d9d19e02f32f8ea976fc071bc6942ed06fba3e7cad937310a78e46e74dfc`;
automated checks pass while `release_ready=false` remains correct. Real-browser
desktop/mobile smoke found no horizontal overflow or console errors. The audit fixed
mixed Registry generations, credential-read failure replacing the active Registry,
false concurrent-reload failures, worker construction without a control token,
concurrent `RunOnce`, nil context, missing-file proposal recovery, and recovery-dialog
failure handling. No unresolved high/medium issue is known; no real key, Provider
request, Shell, LocalRunner, Docker, attack traffic, or external network was used.

远端 GitHub Actions [run 29674460349](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29674460349) 已通过实现提交 `7d5736e`：TypeScript console 38 秒、Windows Desktop shell 2 分 49 秒、Go control plane 3 分 43 秒。<br>
Remote GitHub Actions run 29674460349 passed implementation commit `7d5736e`: TypeScript console 38s, Windows Desktop shell 2m49s, and Go control plane 3m43s.

非 schema D1-G1/D1-I3/D1-F1 完成新的三切片。`repository_state.v1` 使用纯 Go
读取 exact Workspace 根目录的有界 Git 状态，拒绝父仓库发现、重定向 `.git` 和内部
符号链接，不调用进程、网络、remote 或 hook。`file_edit_change_set.v1` 汇总最多 100
个 FileEdit，但每个文件继续独立 review/apply，明确无批量 mutation、无原子 apply，
并显示 partial 状态。Code-only Journey 只把五个阶段导航到既有 Go 能力，不新增复合
执行接口。SQLite 继续保持 v77。<br>
Non-schema D1-G1/D1-I3/D1-F1 completes the next three-slice batch. Pure-Go
`repository_state.v1` projects bounded Git status only at the exact Workspace root,
rejecting parent discovery, redirected `.git`, nested metadata links, subprocesses,
network, remotes, and hooks. `file_edit_change_set.v1` summarizes at most 100 FileEdits
while preserving per-file review/apply and explicitly denying batch or atomic mutation.
The Code-only Journey navigates five stages through existing Go capabilities and adds
no composite execution endpoint. SQLite remains at v77.

本轮累计六切片完整门通过：最终代码全仓 race 490.4 秒，普通全仓此前为 321.7 秒；
repository 追加 10 轮普通重复，`go vet`、零告警 staticcheck、module verify、0 个可达/
导入包漏洞 govulncheck、secure Desktop tags、31 文件 114 项 React、strict TypeScript、
确定性 OpenAPI/TypeScript、Vite、npm 零漏洞、隔离 mock CLI 和 Windows 可复现双构建
均为绿色。OpenAPI 为 59 path/63 operation/129 schema；未签名 GUI SHA-256 为
`145757cb1a8bbafc9080fdc29f4ada69d34b850ca64f702310ea44578ca677a9`，仍正确保持
`release_ready=false`。模块图保留未调用且无修复版的 `GO-2026-5932` openpgp 维护
风险；应用没有导入或调用该包。审计补上 `.git` 内部链接拒绝，并修复终态 Run 的
proposed FileEdit 可无 action 时前端误拒绝。桌面/390x844 浏览器无页面溢出或 console
错误；无已知未解决高/中风险，未使用真实 key、Provider、Shell、LocalRunner、Docker、
hook、攻击流量或外部网络。<br>
The cumulative six-slice gate passed the final-code full race suite in 490.4s; the
ordinary suite had passed in 321.7s, followed by ten repository repetitions after
audit hardening. Vet, zero-warning staticcheck, module verification, zero reachable or
imported-package govulncheck findings, secure Desktop tags, 114 React tests across 31
files, strict TypeScript, deterministic OpenAPI/TypeScript, Vite, zero-vulnerability
npm audit, isolated mock CLI smoke, and a reproducible Windows double build are green.
OpenAPI has 59 paths, 63 operations, and 129 schemas. The unsigned GUI SHA-256 is
`145757cb1a8bbafc9080fdc29f4ada69d34b850ca64f702310ea44578ca677a9` and correctly
remains `release_ready=false`. The module graph retains uncalled, no-fix
`GO-2026-5932` for openpgp; the application neither imports nor calls that package.
Desktop/390x844 browser checks found no page overflow or console error. No unresolved
high/medium issue is known, and no real key, Provider, Shell, LocalRunner, Docker,
hook, attack traffic, or external network was used.

远端 GitHub Actions [run 29678257802](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29678257802) 已通过实现提交 `d69a812`：TypeScript 43 秒、Go 5 分 32 秒、Windows Desktop 5 分 29 秒。<br>
Remote GitHub Actions run 29678257802 passed implementation commit `d69a812`: TypeScript 43s, Go 5m32s, and Windows Desktop 5m29s.

## 仓库 Diff、验证证据与代码交接 / Repository Diff, Verification, And Code Handoff

schema v78 / D1-G2、D1-V1、D1-F2 完成本轮三个产品切片。纯 Go
`repository_diff.v1` 只在 exact registered Workspace root 生成最多 50 项、单项
64 KiB、总计 512 KiB 的脱敏 patch；链接、超大文件、二进制和不可用内容使用闭集状态，
不启动 Git/宿主进程，不访问网络、remote 或 hook，也不返回宿主 root 或原始未脱敏正文。

独立 control capability 允许操作者记录不可变的 `pass|fail|unknown` 验证观察。Go 和
schema v78 精确绑定 Run/Mission/active Session/Workspace、摘要幂等操作与 metadata-only
事件，并把 command/model assertion/approval/authority 全部固定为 false。验证证据不会
执行命令，也不能授权工具、恢复 Run 或替代 Policy。Code-only `code_handoff.v1` 从
持久化 Plan、队列、Change Set、验证、待办行动和报告引用重建有界摘要；四次事件高水位
重试拒绝持续变化造成的撕裂快照，且不复制正文、Diff、私有 operation/lease 或创建复合
mutation。

最终普通全仓 Go 测试 308.1 秒通过，Store 为 299.8 秒；Desktop tag、`go vet`、定向
零告警 staticcheck、module verify/tidy、35 文件 120 项 React、strict TypeScript、
确定性 OpenAPI/TypeScript、Vite build 与 npm 零漏洞均为绿色。OpenAPI 为 62 path、
67 operation、143 schema，SHA-256 为
`652707A6D9CA72EBBD86B6FD407A382DFBE85B094927C82AFC3765D2648332B3`。Windows
可复现双构建 SHA-256 为
`2ab74a47794287bac71877172136f02631b5cc9a44febd930e8ee7b1913ba93f`，自动检查通过但
`release_ready=false`。真实 production bundle 的浏览器复核已完成 Verify 录入、
Handoff 更新、Repository 空状态、桌面/移动宽度与 console 检查。

组合审计修复了 active Session 校验与事务之间的竞态、跨多来源 Handoff 撕裂、顶层
Diff truncation 少报、前端引用/行动/截断严格解析、CR 规范化差异，以及 Verify capability
从连接状态到 API client 的漏传。当前无已知未解决高/中风险；没有使用真实 key、
Provider、Shell、LocalRunner、Docker、hook、攻击流量、安装器、注册表或外部网络。
边界见 [ADR 0048](docs/adr/0048-bounded-diff-verification-code-handoff.md)。

Schema v78 / D1-G2, D1-V1, and D1-F2 complete this three-slice product batch.
Pure-Go `repository_diff.v1` emits at most 50 secret-redacted patches from the exact
registered Workspace root, bounded to 64 KiB per item and 512 KiB total. Links,
oversized files, binary data, and unavailable content use closed states. It starts no
Git/host process, accesses no network, remote, or hook, and exposes neither the host
root nor raw unredacted bodies.

A separate control capability records immutable operator `pass|fail|unknown`
observations. Go and schema v78 exact-bind the Run, Mission, active Session, Workspace,
digest-idempotent operation, and metadata-only event while fixing command execution,
model assertion, approval, and authority to false. Verification runs no command and
cannot authorize a tool, resume a Run, or replace Policy. Code-only
`code_handoff.v1` regenerates a bounded summary from durable Plan, queue, change-set,
verification, action, and report sources. Four event-high-water retries reject a torn
snapshot; private bodies, Diffs, operations, leases, composite mutation, resume
authority, and execution remain absent.

The final ordinary Go suite passed in 308.1s, including Store in 299.8s. Desktop tags,
vet, targeted zero-warning staticcheck, module verification/tidy, all 120 React tests
across 35 files, strict TypeScript, deterministic OpenAPI/TypeScript, Vite, and the
zero-vulnerability npm audit are green. OpenAPI has 62 paths, 67 operations, and 143
schemas with SHA-256
`652707A6D9CA72EBBD86B6FD407A382DFBE85B094927C82AFC3765D2648332B3`.
The reproducible Windows double build has SHA-256
`2ab74a47794287bac71877172136f02631b5cc9a44febd930e8ee7b1913ba93f`; automated
checks pass while `release_ready=false` remains correct. Production-bundle browser
checks covered Verify recording, Handoff refresh, Repository empty state, responsive
containment, and console diagnostics.

The combined audit closed an active-Session transaction race, torn multi-source
Handoffs, top-level Diff truncation under-reporting, strict-client reference/action/
truncation gaps, CR normalization drift, and missing Verify-capability propagation.
No unresolved high/medium issue is known. No real key, Provider, Shell, LocalRunner,
Docker, hook, attack traffic, installer, registry mutation, or external network was
used. [ADR 0048](docs/adr/0048-bounded-diff-verification-code-handoff.md) records the
boundary.

远端 GitHub Actions [run 29682547524](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29682547524) 已通过实现提交 `cff7489`：TypeScript console 42 秒、Windows Desktop shell 2 分 34 秒、含 `go vet` 与 `govulncheck` 的 Go control plane 3 分 33 秒。<br>
Remote GitHub Actions run 29682547524 passed implementation commit `cff7489`: TypeScript console 42s, Windows Desktop shell 2m34s, and Go control plane including `go vet` and `govulncheck` 3m33s.

## v79 防死锁与活锁加固批次 / v79 Deadlock And Livelock Hardening Batch

本轮三个安全切片分别完成：Tool Runtime 的硬超时、取消、panic 恢复和特殊文件拒绝；Agent/Tool/Retriever/Store/Runner/Model 的有界同步等待图；以及 schema v79 可跨重启恢复的 Run 无进展熔断。等待边使用引用计数并在调用结束后幂等释放，Specialist parent/child 和 Tool Gateway 共用 root Supervisor 注入的身份；低层反向等待 Agent 与任何直接/间接环路都在实际阻塞前失败。

v79 对相同动作三轮、无结构化进展六轮的阈值在 Go 与 SQLite 双层固定。检测、审计事件、Session 消息、checkpoint 和 `running -> paused` 在同一事务提交；原始 `continue` 的完成重放会识别已转换的 `wait`，只有检测之后真实存在的 `paused -> running` 事件才能重置 guard。迁移 v78 -> v79 不伪造任何进展记录，事件只含计数、阈值、原因和摘要元数据，不含模型正文。

累计六切片完整门禁已通过：最终未缓存全仓 Go 测试 312 秒、最终代码全仓 race 358 秒、关键 Tool/等待图 20 轮和 v79 Store 10 轮、普通及 secure-Desktop test/vet、零告警 staticcheck、module verify/tidy、零可达漏洞 govulncheck、35 个前端文件 120 项测试、strict TypeScript、确定性 API、Vite production build、零漏洞 npm audit、隔离 mock-only CLI、仓库凭据/产物扫描、Windows 可复现构建和桌面/移动浏览器检查。未签名 Desktop SHA-256 为 `31e0df63d3fbbccac6728ad2322196bee55d57e775a15cc34f752c0632bdc699`，仍正确保持 `release_ready=false`；浏览器无横向溢出和 console error。

审计补强了 `max_bytes` 的平台整数/OOM 边界、Go/SQL 观察态阈值约束、检测后必须有显式恢复事件、损坏 guard 读取失败关闭和计数饱和。`govulncheck` 仍只报告依赖图中未导入、未调用的 `GO-2026-5932` module-only 残余。当前启用路径未发现未解决高/中风险；没有调用真实 key、Provider、Shell、LocalRunner、Docker、攻击流量或外部网络。架构完成度保持约 99%，产品可用度保持约 92-94%，因为该批次提高运行时可靠性但没有开放新的用户执行权限。边界见 [ADR 0049](docs/adr/0049-deadlock-livelock-runtime-guards.md)。

This three-slice safety batch adds hard Tool deadlines, cancellation, panic recovery,
and special-file rejection; a bounded synchronous wait graph for Agent, Tool,
Retriever, Store, Runner, Model, and external nodes; and schema v79's restart-safe Run
no-progress circuit breaker. Wait edges are reference-counted and idempotently
released. Specialist parent/child execution and the Tool Gateway share the identity
injected by the root Supervisor. Lower-layer callbacks into an Agent and every direct
or indirect cycle fail before blocking.

Go and SQLite fix the v79 thresholds at three identical actions or six turns without
structured progress. Detection, audit events, Session messages, checkpoint, and the
`running -> paused` transition commit atomically. Completion replay recognizes the
original `continue` converted to `wait`, and only a real later `paused -> running`
event may reset a detected guard. Upgrading v78 to v79 fabricates no progress record;
events contain bounded counters, thresholds, reasons, and digest metadata, not model
text.

The cumulative six-slice gate passed the final uncached Go suite in 312s, the
final-code full race suite in 358s, 20 Tool/wait-graph repetitions, ten v79 Store
repetitions, ordinary and secure-Desktop tests/vet, zero-warning staticcheck, module
verification/tidy, zero reachable govulncheck findings, 120 frontend tests across 35
files, strict TypeScript, deterministic API generation, Vite production build, a
zero-vulnerability npm audit, isolated mock-only CLI smoke, repository credential and
artifact scans, reproducible Windows build, and desktop/mobile browser checks. The
unsigned Desktop SHA-256 is
`31e0df63d3fbbccac6728ad2322196bee55d57e775a15cc34f752c0632bdc699` and correctly
remains `release_ready=false`; the browser showed no horizontal overflow or console
error.

The audit hardened platform-integer/OOM handling for `max_bytes`, Go/SQL observing
thresholds, explicit post-detection resume proof, fail-closed corrupt guard reads, and
saturating counters. `govulncheck` retains only module-level `GO-2026-5932`, which the
application neither imports nor calls. No unresolved high/medium issue is known on an
enabled path, and no real key, Provider, Shell, LocalRunner, Docker, attack traffic,
or external network was used. Architecture remains about 99% and product usability
about 92-94% because this batch improves runtime reliability without granting a new
execution capability. [ADR 0049](docs/adr/0049-deadlock-livelock-runtime-guards.md)
records the boundary.

远端 GitHub Actions [run 29688544340](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29688544340) 已通过实现提交 `2012bfa`：TypeScript console 42 秒、Windows Desktop shell 3 分 13 秒、含 `go vet` 与 `govulncheck` 的 Go control plane 3 分 54 秒。<br>
Remote GitHub Actions run 29688544340 passed implementation commit `2012bfa`: TypeScript console 42s, Windows Desktop shell 3m13s, and Go control plane including `go vet` and `govulncheck` 3m54s.

## 仓库历史、验证计划与交接导出 / Repository History, Verification Plans, And Handoff Exports

schema v80 / D1-G3、D1-V2、D1-F3 完成本轮三个产品切片。纯 Go
`repository_history.v1` 只打开注册 Workspace 的 exact root，沿 first-parent 最多读取
50 个本地提交，并最多返回 64 个本地分支；提交主题先规范化、限长和密钥脱敏，作者身份、
邮箱、commit body、remote、宿主 root、进程、网络和 hook 均不进入协议。重定向或链接的
`.git` 继续失败关闭，合并父节点与省略分支计数在 Go 侧饱和。

schema v80 新增不可变 `operator_verification_plan.v1`。操作者可记录最多 32 项的检查清单，
但计划与 v78 的 `pass|fail|unknown` 结果保持独立。Go、事务内 Store 复核和 SQLite trigger
共同绑定 Run/Mission/active Session/Workspace、事件和内容摘要，并把 guidance-only 固定为
true，把 command/model assertion/result inference/approval/authority 固定为 false。文档或
模型写出的要求不会自动变成已执行或已通过的验证事实。

`code_handoff_export.v1` 把同一份稳定 `code_handoff.v1` 导出为最多 256 KiB 的 Markdown
或 JSON。服务端返回源事件高水位、UTF-8 字节数和内容 SHA-256；TypeScript 在下载前重新
验证大小、摘要、格式和 Run/高水位绑定。导出仍是只读下载，不恢复 Run、不接受报告、
不 apply 文件，也不启动任何执行。

本轮 ordinary 集成门通过：最终功能代码的 uncached 全仓 Go 测试为 334.6 秒，审计修正后
Repository/Application/Store/HTTP 聚焦回归再次通过，`go vet ./...` 为绿色；37 个前端文件
124 项测试、strict TypeScript 和 Vite production build 通过。OpenAPI 为 65 path、71
operation、155 schema，SHA-256 为
`99887F651B563C56C87D19C5624EDD776AFC29AA6095EAB8C685E6767C165E7F`。Chrome 插件对
最终 production bundle 的 Repository、Verify 和 Handoff 做了真实复验：无根路径/邮箱
泄漏、无验证结果推导、无页面横向溢出，console 零 warning/error。

组合审计修复了恰好 50 份计划时的前端截断误判、恶意 Git 元数据计数越界、计划修改后
误复用旧幂等 key，以及部分浏览器下载对象 URL 释放过早；当前未发现未解决高/中风险。没有使用真实 key、Provider、
Shell、LocalRunner、Docker、Git hook、攻击流量或外部网络。架构完成度约 99%，完整产品
可用度约 93-95%，通用 Coding Agent 约 93%，Cyber 自动化约 20%。本批是新六切片周期
的前三片；再完成三片后执行完整 race/staticcheck/govulncheck/依赖/隐私/构建健壮性门。
边界见 [ADR 0050](docs/adr/0050-repository-history-verification-plan-handoff-export.md)。

Schema v80 / D1-G3, D1-V2, and D1-F3 complete this three-slice product batch.
Pure-Go `repository_history.v1` opens only the exact registered Workspace root,
follows at most 50 first-parent commits, and returns at most 64 local branches.
Subjects are normalized, bounded, and secret-redacted. Author identities, email,
commit bodies, remotes, host roots, subprocesses, network, and hooks stay outside the
protocol. Redirected or linked Git metadata remains fail-closed, and hostile parent
or omitted-branch counts saturate in Go.

Schema v80 adds immutable `operator_verification_plan.v1` with at most 32
operator-authored checks, deliberately separate from v78 `pass|fail|unknown` results.
Go, the transactional Store recheck, and SQLite triggers bind the Run, Mission,
active Session, Workspace, event, and content digests while fixing guidance-only to
true and command/model assertion/result inference/approval/authority to false.
Requirements written by a model or document never become proof of execution or pass.

`code_handoff_export.v1` renders the same stable `code_handoff.v1` as bounded Markdown
or JSON, capped at 256 KiB. The server returns the source event high-water mark, UTF-8
byte count, and content SHA-256; TypeScript verifies size, digest, format, and Run/source
binding before download. Export cannot resume a Run, accept a report, apply a file, or
start execution.

The ordinary integrated gate passed the uncached full Go suite in 334.6s, followed by
post-audit Repository/Application/Store/HTTP regressions and `go vet ./...`. All 124
frontend tests across 37 files, strict TypeScript, and the Vite production build are
green. OpenAPI has 65 paths, 71 operations, and 155 schemas with SHA-256
`99887F651B563C56C87D19C5624EDD776AFC29AA6095EAB8C685E6767C165E7F`.
Chrome-extension checks against the final production bundle covered Repository,
Verify, and Handoff with no root/email disclosure, inferred verification result,
page-level horizontal overflow, console warning, or console error.

The combined audit fixed exact-limit inventory rejection, hostile Git metadata count
overflow, stale idempotency-key reuse after editing a failed plan intent, and premature
download object-URL revocation. No unresolved high/medium
issue is known. No real key, Provider, Shell, LocalRunner, Docker, Git hook, attack
traffic, or external network was used. Architecture is about 99%, complete-product
usability about 93-95%, generic Coding Agent usability about 93%, and Cyber automation
about 20%. This is the first half of a six-slice cycle; the full race/staticcheck/
govulncheck/dependency/privacy/build gate follows the next three slices.
[ADR 0050](docs/adr/0050-repository-history-verification-plan-handoff-export.md)
records the boundary.

远端 GitHub Actions [run 29695882120](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench/actions/runs/29695882120) 已通过实现提交 `d70d96c`：TypeScript console 43 秒、Windows Desktop shell 2 分 39 秒、含 `go vet` 与 `govulncheck` 的 Go control plane 3 分 56 秒。<br>
Remote GitHub Actions run 29695882120 passed implementation commit `d70d96c`: TypeScript console 43s, Windows Desktop shell 2m39s, and Go control plane including `go vet` and `govulncheck` 3m56s.

## 精确提交、验证关联与 Runner 生命周期 / Exact Commit, Verification Association, And Runner Lifecycle

D1-G4、schema v81 / D1-V3 与 R1 完成当前六切片周期的后三片。纯 Go
`repository_commit_detail.v1` 只接受注册 Workspace root 中一个精确的小写 40 位 SHA-1
对象 ID，将提交树与 first parent 比较，最多返回 200 个 canonical changed path 及
added/modified/deleted、content changed、mode changed 元数据。它不读取 author、email、
commit body、blob body、remote 或宿主 root，不 checkout、不调用进程/网络/hook；重定向
Git metadata、link、畸形 tree 和缺失 object 均失败关闭。

schema v81 新增不可变 `operator_verification_plan_evidence_association.v1`。一条 evidence
只能显式关联到一个更早的 plan item，一个 item 可保留多条甚至相互矛盾的观察。Go、事务
Store 与 SQLite trigger 精确绑定 Code Run、active Session、Workspace、plan/item/evidence、
operation、event 和 digest，并把 command execution、model assertion、result inference、
approval 与 authority 固定为 false。`operator_verification_plan_coverage.v1` 只显示每项
`pass|fail|unknown` 计数和 unobserved 状态，绝不推导整份计划通过。

非 schema `runner_lifecycle_contract.v1` 固化 start/wait/cancel/timeout、TERM/KILL grace、
最终 inspect/reap 和 orphan cleanup，并在 start 前进入共享 wait graph。当前只接受
`SimulationOnly` backend；partial start 或非法 process identity 也必须清理。它没有 CLI、
HTTP、Desktop、Agent、LocalRunner、Docker、`os/exec` 或产品 capability 接线，因此不是
开放真实宿主机/容器执行的后门。

累计六切片完整健壮性门已通过：最终 uncached 全仓 Go 509 秒、全仓 race 341 秒、普通与
secure-Desktop test/vet、零告警 staticcheck、module verify/tidy、零可达 govulncheck、
37 文件 127 项 React、strict TypeScript、确定性 OpenAPI/TypeScript、Vite production、
npm 零漏洞、隔离 mock-only CLI、隐私/产物检查和 Windows 可复现构建均为绿色。OpenAPI
为 68 path、74 operation、163 schema，SHA-256 为
`CFAD160A85306B2602F95A62298828DB86BDFAAF6D55F47BA468860079C42E8D`；生成 TypeScript
schema SHA-256 为 `CCA5EF8B86E7F0D494E7B2BAF4FCA92FBE3FCB9C3A54E58D4A3C3B77028D5B73`。
未签名 GUI SHA-256 为
`77fb4d6fede1c1e3a0c3f3e9d39581e28f7a6880e0e25b222dcf0d3c701d1213`，自动检查通过但
`release_ready=false`。

Chrome 插件在 Go-hosted production bundle 中真实记录 plan、pass evidence 和显式关联，
刷新后仍恢复 `1/1 observed` 与 `1 linked`；桌面及 390x844 移动宽度无页面级横向溢出，
console 零 warning/error。组合审计修复了会静默略过缺失 subtree object 的 Git walker、
v81 降级夹具 trigger 清理顺序、脱敏计数饱和、OpenAPI control 白名单，以及 partial/
invalid Runner start 清理。当前启用路径无已知未解决高/中风险；模块图只保留应用未导入/
未调用的 transitive `GO-2026-5932`。没有使用真实 key、Provider、Shell、LocalRunner、
Docker、hook、攻击流量或外部网络。架构完成度约 99%，完整产品可用度约 94-96%，通用
Coding Agent 约 94%，Cyber 自动化约 20%。边界见
[ADR 0051](docs/adr/0051-exact-commit-verification-association-runner-lifecycle.md)。

D1-G4, schema-v81 D1-V3, and R1 complete the second half of the current six-slice
cycle. Pure-Go `repository_commit_detail.v1` accepts one exact lowercase 40-character
SHA-1 object ID at the registered Workspace root, compares the commit tree with its
first parent, and returns at most 200 canonical changed paths with added, modified,
deleted, content-change, and mode-change metadata. It reads no author, email, commit
body, blob body, remote, or host root and performs no checkout, subprocess, network,
or hook action. Redirected Git metadata, links, malformed trees, and missing objects
fail closed.

Schema v81 adds immutable `operator_verification_plan_evidence_association.v1` facts.
One evidence record can explicitly answer one earlier plan item, while an item may
retain multiple or contradictory observations. Go, transactional Store checks, and
SQLite triggers exact-bind the Code Run, active Session, Workspace, plan, item,
evidence, operation, event, and digest and keep command execution, model assertion,
result inference, approval, and authority false. The bounded
`operator_verification_plan_coverage.v1` reports per-item `pass|fail|unknown` counts
and unobserved state only; it never infers an overall pass.

Non-schema `runner_lifecycle_contract.v1` defines start/wait/cancel/timeout, TERM/KILL
grace, final inspection/reaping, orphan cleanup, and shared wait-graph entry. It accepts
only `SimulationOnly` backends today and cleans partial starts and invalid process
identities. There is no CLI, HTTP, Desktop, Agent, LocalRunner, Docker, `os/exec`, or
product-capability wiring, so this does not enable real host or container execution.

The cumulative six-slice robustness gate passed the final uncached full Go suite in
509s and full race suite in 341s, ordinary and secure-Desktop test/vet, zero-warning
staticcheck, module verification/tidy, zero reachable govulncheck findings, 127 React
tests across 37 files, strict TypeScript, deterministic OpenAPI/TypeScript, Vite,
zero-vulnerability npm audit, isolated mock-only CLI, privacy/artifact checks, and a
reproducible Windows build. OpenAPI has 68 paths, 74 operations, and 163 schemas with
SHA-256 `CFAD160A85306B2602F95A62298828DB86BDFAAF6D55F47BA468860079C42E8D`;
the generated TypeScript schema SHA-256 is
`CCA5EF8B86E7F0D494E7B2BAF4FCA92FBE3FCB9C3A54E58D4A3C3B77028D5B73`. The unsigned
GUI SHA-256 is `77fb4d6fede1c1e3a0c3f3e9d39581e28f7a6880e0e25b222dcf0d3c701d1213`, with
automated checks passing and `release_ready=false`.

Chrome-extension verification against the Go-hosted production bundle recorded a
plan, pass evidence, and their explicit association, then recovered `1/1 observed`
and `1 linked` after reload. Desktop and 390x844 mobile layouts had no page-level
horizontal overflow or console warning/error. The audit replaced a Git walker that
could silently omit missing subtree objects, fixed v81 downgrade-fixture trigger
ordering, saturated redaction counts, constrained the OpenAPI control whitelist, and
cleaned partial/invalid Runner starts. No unresolved high/medium issue is known on an
enabled path. The module graph retains only unimported and uncalled transitive
`GO-2026-5932`. No real key, Provider, Shell, LocalRunner, Docker, hook, attack traffic,
or external network was used. Architecture is about 99%, complete-product usability
about 94-96%, generic Coding Agent usability about 94%, and Cyber automation about
20%. [ADR 0051](docs/adr/0051-exact-commit-verification-association-runner-lifecycle.md)
records the boundary.

## 模型上下文与累计交接记忆批次 / Model Context And Cumulative Handoff Batch

本轮三个运行时切片完成 Unicode/CJK 保守 token 估算、root/Specialist 完整请求的模型窗口
闸门，以及 schema v82 的累计交接记忆。审计确认旧实现连续压缩时只读取最新摘要，却没有把
更早摘要并入新摘要，早期决策会随第二次压缩丢失；v82 现在以 exact predecessor、SHA-256、
累计计数和单调 ordinal 构成不可变链。记录超过 12 条时按权限与进度优先级有界保留，省略
数量仍累计，不会在若干轮后突然保存失败。

默认模型规划窗口为 32K，总输入会计包含工具 Schema，并为输出和安全余量预留空间；只裁掉
最旧普通 history，强制上下文超限则在 Provider 调用前失败。该数字是本地保守策略，不代表
MIMO、DeepSeek、Anthropic 或其他模型的官方窗口。没有自动改写/重载任意 `AGENTS.md` 或
README；项目文件、工具结果和旧摘要仍是非可信证据。

最终 uncached 全仓 Go 测试 348.5 秒通过，`go vet` 通过，TypeScript strict typecheck 与
37 个文件 127 项 Vitest 全绿。组合审计另修复零值 Router 的窗口 map 初始化、v82 追加后
v81 降级夹具的迁移顺序，并把交接记录从共享校验上限分离为明确的 12 条契约；来源引用会脱敏/限长，时钟回拨会钳位，摘要写入后的崩溃重试按消息高水位 exactly-once 恢复。当前无已知未解决高/中风险。
本批是新六切片周期的前三片，因此完整 race/staticcheck/govulncheck/依赖/隐私/构建门在
下一批三片后执行。未调用真实 Provider、API key、Shell、LocalRunner、Docker、hook、攻击
流量或外部网络。边界见
[ADR 0052](docs/adr/0052-conservative-model-context-cumulative-handoff-memory.md)。

This three-slice runtime batch adds conservative Unicode/CJK token estimation, a
complete-request context gate for root and Specialist calls, and schema-v82 cumulative
handoff memory. The audit confirmed that repeated legacy compaction loaded only the
newest summary without incorporating earlier summaries, so early decisions could
disappear after a second compaction. V82 now forms an immutable chain with an exact
predecessor, SHA-256, cumulative counters, and monotonic ordinals. Retention is bounded
to 12 prioritized records while keeping a cumulative omitted count.

The 32K fallback is a local conservative planning policy, not a Provider capability
claim. Tool schemas are included, only oldest ordinary history may be removed, and
mandatory overflow fails before a Provider call. Arbitrary `AGENTS.md`, README, tool,
and model text is not automatically reloaded as control authority.

The uncached full Go suite passed in 348.5 seconds, `go vet` passed, and strict
TypeScript plus all 127 Vitest tests across 37 files passed. Review also fixed
zero-value Router initialization, v81 downgrade-fixture ordering after v82, a
dedicated 12-record handoff cap, redacted/bounded source references, clock rollback
clamping, and message-high-water crash recovery. No unresolved high/medium issue is known. This is
the first half of a six-slice cycle, so the full race/staticcheck/govulncheck/
dependency/privacy/build gate follows the next three slices. No real Provider, key,
Shell, LocalRunner, Docker, hook, attack traffic, or external network was used.
[ADR 0052](docs/adr/0052-conservative-model-context-cumulative-handoff-memory.md)
records the boundary.

## 精确提交预览、交接覆盖率与进程树一致性 / Exact Commit Preview, Handoff Coverage, And Process Conformance

本轮完成 D1-G5、D1-V4 与 R2，SQLite schema 保持 v82。D1-G5 新增
`repository_commit_file_preview.v1`：只在注册 Workspace 的精确根目录中读取一个精确
commit 的 canonical 相对路径，只接受 regular/executable UTF-8 文本，输入上限 64 KiB，
脱敏投影上限 128 KiB。响应携带投影正文 SHA-256 和
`instruction_authorized=false`，不返回 raw blob、宿主机 root、remote，也不 checkout、
更新 ref、启动进程、访问网络或执行 hook。TypeScript 会再次校验对象、路径、字节数、
SHA-256 和全部权限位后才显示内容。

D1-V4 把 `operator_verification_plan_coverage.v1` 作为只读元数据加入
`code_handoff.v1` 及 Markdown/JSON 导出。最多 100 个条目只包含 plan/item 摘要、显式
`pass|fail|unknown` 计数和最新关联序号；计划标题、预期结果、Evidence 摘要与消息正文不进入
Handoff。相互矛盾的观察会保留为 contradiction 计数，系统不会推断整份计划已经通过、失败或
完成。Go 和 TypeScript 都会复核总数、重复项、摘要、事件序号以及固定为 false 的结果权限。

R2 用真实操作系统行为验证 `runner_lifecycle_contract.v1`：Windows 测试使用私有 Job
Object，Unix 测试使用私有 process group，覆盖协作终止、无响应后的强杀升级，以及父进程
先退出后的孤儿 child 清理。具体适配器和 `os/exec` 全部只存在于 `_test.go`，并且只启动
当前 Go 测试二进制；CLI、HTTP、Desktop、Agent、Sandbox、LocalRunner、Docker、审批与
执行档位都无法构造它们。接口名称从 `SimulationOnly` 调整为更准确的
`NonProductOnly`，没有开放任何产品执行权限。

本六切片周期的完整门禁已通过：uncached 全仓 Go 普通测试 380 秒、全仓 race 411.2 秒，
审计后受影响包普通/race 回归、ordinary/secure-Desktop test/vet、零告警
`staticcheck`、module verify/tidy、零可达 `govulncheck`、37 个前端文件 127 项测试、strict
TypeScript、Vite production build、OpenAPI/TypeScript 确定性生成、npm 零漏洞、隔离
mock-only CLI、隐私/产物/产品进程入口扫描和 Windows 可复现双构建全部通过。OpenAPI 为
69 path / 75 operation / 167 schema，SHA-256 为
`C548ADCBFB4BF271009348A36352E987FBF4CA10681F4B9C7CC694543487FDF6`；生成的
TypeScript schema SHA-256 为
`6093BB23C1A413154027FF3283AD2485DC3F44C8763949C1A45B7D066B7BB914`。最终未签名 GUI
SHA-256 为 `44d54bf9d50b7cd99b89f5089833823ce0337bb0e0158ec16ef6aa9a5b415614`，自动检查通过，
但仍正确保持 `release_ready=false`、无安装器、无注册表写入。

审计修复了覆盖计数在 32 位平台上的整数相加、负/零聚合事件事实，以及窄 Store 边界对重复
plan/count 的接受问题；当前启用路径没有已知未解决高/中风险。`govulncheck` 只保留一个应用
未导入、未调用的 required-module 公告。用户测试 Key 没有进入仓库，本轮未调用真实
Provider、Shell、LocalRunner、Docker、Git hook、攻击流量或外部网络。当前工程估算为：
架构完成度约 99%，完整产品可用度约 95-97%，通用 Coding Agent 约 95%，Cyber 自动化约
20%。下一批建议推进 D1-G6 有界文件历史、D1-V5 只读覆盖率下钻和 R3 输出/退出证据契约；
真实 Local/Docker start、xterm 输入、安装签名、Rust analyzer 与 CTF 求解继续独立设门。
边界见 [ADR 0053](docs/adr/0053-commit-preview-handoff-coverage-process-conformance.md)。

This non-schema v82 batch completes D1-G5, D1-V4, and R2. Exact-commit file preview
returns only bounded, secret-redacted regular/executable UTF-8 with projected-content
SHA-256 and no raw blob, root, checkout, mutation, process, network, or hook authority.
Code Handoff and its exports now retain bounded explicit verification coverage and
contradictions without private bodies or an inferred aggregate result. Windows Job
Object and Unix process-group adapters prove real tree termination, forced kill, and
orphan cleanup, but exist only in test files and start only the Go test binary.

The full six-slice gate passed the 380-second ordinary suite, 411.2-second race suite,
post-audit focused ordinary/race regressions, ordinary and secure-Desktop checks,
vet/staticcheck/module/vulnerability gates, all 127 Web tests, strict TypeScript,
deterministic API generation, Vite, zero npm vulnerabilities, isolated mock-only CLI,
privacy/artifact checks, and the reproducible Windows build. No enabled path has a
known unresolved high/medium issue, and no product process authority was added.
[ADR 0053](docs/adr/0053-commit-preview-handoff-coverage-process-conformance.md)
is authoritative.

## 精确文件历史、验证下钻与 Runner 退出证据 / Exact File History, Verification Drilldown, And Runner Exit Evidence

本轮完成 D1-G6、D1-V5 与 R3，SQLite schema 保持 v82。D1-G6 新增纯 Go
`repository_file_history.v1`：在注册 Workspace 的精确根目录中，沿当前 HEAD 最多扫描
512 个 first-parent commit，返回一个 canonical 相对路径最多 50 次变化。投影只包含
commit object/短哈希、脱敏且有界的 subject、时间、added/modified/deleted、文件类型和
content/mode 变化；不推断 rename，不返回 author/body/blob/patch/remote/root，也不 checkout、
更新 ref、启动进程、访问网络或执行 hook。Git 图顺序是事实，提交时间不要求单调。

D1-V5 新增 `operator_verification_plan_item_coverage.v1`，按精确 Run + plan + ordinal
读取一个验证检查项。响应给出 plan/item digest、显式 `pass|fail|unknown` 聚合计数和最多
100 条最新 evidence association 引用；不包含计划指导正文、Evidence 正文、操作者身份或
整体 verdict。Go、SQLite、OpenAPI 和 TypeScript 都复核绑定、摘要、唯一 association/
evidence、严格倒序事件、截断计数及全部 false-authority 字段。React 现可从每个检查项展开
这份只读引用列表，不会因为 Run 级 100 条总览上限而看不到该检查项的第一页。

R3 新增内部 `runner_exit_evidence.v1`。`NonProductOnly` backend 只有在进程树退出并确认
reaped 后，才能返回 stdout/stderr 的观察字节数、最多 64 KiB 前缀长度/SHA-256 和截断位，
以及与 exit code/reaped 精确绑定的退出事实；结果类型没有 raw output 字段，并固定
`product_execution_enabled=false`。Windows Job Object 与 Unix process-group 适配器仍只在
`_test.go` 中启动当前 Go test binary，产品 CLI/HTTP/Desktop/Agent/Sandbox/Local/Docker
依然无法构造 Runner 或启动进程。

普通三切片门禁通过：uncached 全仓 Go 373.3 秒、受影响包 race、全仓 vet、受影响包
zero-warning staticcheck、module verify/tidy、37 文件 127 项 Web 测试、strict TypeScript、
Vite production build、npm 高危漏洞为零、确定性 OpenAPI/TypeScript、Linux runner test
binary 交叉编译及 Windows 可复现双构建均为绿色。OpenAPI 为 71 path / 77 operation /
170 schema；最终未签名 GUI SHA-256 为
`c96047d7f3ea0afbe3b2f54f1c4ded197a861b29d644cb2edb449c8b3e46b031`，并继续保持
`release_ready=false`、无 installer、无 registry write。

组合审计修复了两个中等健壮性问题：前端不再错误假设 first-parent commit 时间单调；
截断后的验证下钻会拒绝重复 evidence 和超过聚合值的 outcome。当前启用路径没有已知未解决
高/中风险，用户测试 Key 未进入 diff，本轮未启用真实 Provider、Shell、LocalRunner、Docker、
Git hook、攻击流量或外部网络。下一批候选是 D1-G7 history-to-commit 导航、D1-V6 精确检查项
evidence 的 opaque pagination，以及 R4 非产品 stdin/descriptor/resource 证据；真实进程执行、
xterm、安装签名、Rust analyzer 和 CTF 求解继续独立设门。边界见
[ADR 0054](docs/adr/0054-file-history-verification-drilldown-runner-exit-evidence.md)。

This non-schema v82 batch completes D1-G6, D1-V5, and R3. The Code workbench can now
follow one exact path through bounded first-parent metadata and drill into one exact
verification item's explicit evidence references. Neither projection exposes private
content, infers an aggregate result, mutates Git/state, or grants authority.

The internal non-product Runner now has a bounded metadata-only stdout/stderr and exit-
evidence contract after verified tree reaping. Real OS adapters remain test-only and
no product process-start route was added. The ordinary integrated gate and post-audit
focused regressions passed; two medium robustness issues were fixed before acceptance,
with no known unresolved high/medium issue on an enabled path.
[ADR 0054](docs/adr/0054-file-history-verification-drilldown-runner-exit-evidence.md)
is authoritative.

## 历史导航、验证分页与 Runner 运行元数据 / History Navigation, Verification Pagination, And Runner Runtime Metadata

本轮完成非 schema 的 D1-G7、D1-V6 与 R4，SQLite 继续保持 v82。精确文件历史的每一行
现在可以复用既有 `repository_commit_detail.v1` 打开对应提交；regular/executable 文件还可
复用既有脱敏预览，删除项、symlink 与 submodule 不提供预览。React 只回传 Go 已投影的
Workspace、object ID 和 canonical path，不新增 raw Git、checkout、ref、process、network 或
hook 能力。

精确验证检查项接口现支持共享的 `limit` 与 route-scoped opaque `cursor`。SQLite 以不可变
association event/ID 倒序分页，最多从 100,000 行窗口起点继续；Go 和 TypeScript 逐页复核
绑定、摘要、总数、顺序、唯一 ID、显式 outcome 和 false-authority 字段。React 每次显式加载
25 条更早引用，并对跨页 aggregate/high-water/事件顺序做一致性检查；这是有界 live projection，
若加载期间事实变化则提示刷新，不把不同快照静默拼接。正文、操作者身份、整体 verdict、命令、
模型、审批和权限仍不进入响应。

R4 新增内部 `runner_runtime_evidence.v1`。进程树确认 reaped 后，`NonProductOnly` backend
只能报告 stdin 字节数/SHA-256 且固定关闭、不继承、无原文；stdout/stderr 固定捕获且无额外/
继承描述符、无名称/路径；资源只含有界 wall time、父进程 user/system CPU 和可选峰值内存，
不含环境或网络/raw telemetry。退出证据与运行证据使用独立 post-reap timeout，全部验证后
原子提交；任一异常都得到 `StopEvidenceFailed`，不会留下半份审计记录。真实 OS 适配器仍只在
`_test.go` 中运行当前 Go test binary，产品入口没有 process starter。

累计六切片完整门禁通过：uncached 全仓 Go/race 分别 377.3/409.8 秒；普通与 secure Desktop
test/vet、全仓 vet/staticcheck、双路径 govulncheck、module verify/tidy、37 文件 127 项 Web、
strict TypeScript、确定性 OpenAPI/TypeScript、Vite、npm 零漏洞、隔离 mock-only CLI、隐私/
进程入口扫描、Linux runner test 交叉编译与 Windows 可复现双构建均为绿色。OpenAPI 仍为
71 path / 77 operation / 170 schema；未签名 GUI SHA-256 为
`1d51529b1a6d7d90e121e770faa54c9f4d77b4a96d3c0d920fe091178a299da2`，继续保持
`release_ready=false`、无 installer、无 registry write。组合审计没有发现启用路径中的未解决
高/中风险，也未使用真实 Provider/Key、Shell、LocalRunner、Docker、hook、攻击流量或外网。
边界见 [ADR 0055](docs/adr/0055-history-navigation-verification-pagination-runner-runtime-evidence.md)。

This non-schema v82 batch completes D1-G7, D1-V6, and R4. File-history rows now reuse
the existing exact-commit and redacted-preview contracts. Exact verification-item
evidence uses bounded route-scoped opaque pagination, while React detects cross-page
live-projection drift instead of silently merging incompatible facts.

The internal Runner now atomically validates metadata-only stdin, descriptor, and
resource evidence after tree reaping. OS process adapters remain test-only and no
product start capability exists. The complete six-slice ordinary/race/static/
vulnerability/dependency/privacy/build gate passed with no known unresolved high or
medium issue on an enabled path. ADR 0055 is authoritative.

## 精确提交比较、快照分页与 Runner 控制证据 / Exact Commit Comparison, Snapshot Pagination, And Runner Control Evidence

本轮完成非 schema 的 D1-G8、D1-V7 与 R5，SQLite 继续保持 v82。D1-G8 新增
`repository_commit_comparison.v1`：操作者可在同一注册 Workspace 内选择任意两个精确的
小写 40 字符本地 commit object，比对其完整树的有界元数据；不要求 ancestor 关系。结果只含
脱敏 subject、时间及 canonical path 的 added/modified/deleted、kind、content/mode-change，
不包含 author/body/blob/patch/remote/root，不推断 rename，也不 checkout、更新 ref、启动进程、
访问网络或执行 hook。React 采用“设置当前提交为基线，再打开另一精确提交”的两步流程。

D1-V7 将精确验证检查项分页从 offset/live projection 改为
event-high-water snapshot + `(event_sequence, association_id)` keyset。第一页冻结该检查项的
聚合计数与事件高水位，后续 opaque cursor 绑定 exact route、snapshot、上一页尾 tuple 和已消费
数量；SQLite/Go 会复核 anchor 的真实排名，伪造、跨条目复用或与快照冲突均失败关闭。第一页后
新增的 association 不会挤动旧分页；读取窗口仍硬限制为 100,000 条，达到窗口时最后一页可用
`page.truncated=true` 明确结束，而不是产生越界 cursor。private body、操作者身份、aggregate
verdict、命令、模型、审批、mutation 和 authority 继续缺席。

R5 在内部 `NonProductOnly` Runner 增加 `runner_resource_limit_evidence.v1` 与
`runner_termination_cause_evidence.v1`。前者只证明规范化 run timeout、TERM/KILL grace 和 wall
deadline 已配置，并明确固定 CPU/memory limit 未配置、OS resource limit 未验证；后者把
process exit、caller cancellation、run deadline、wait failure、orphan-after-exit、partial-start
failure 映射到 wait/terminate/kill 控制结果，且不推断 OS signal/cause。四份 post-reap evidence
全部验证后才原子写入 Result；重复采集漂移失败关闭。真实 OS adapter 仍只在 `_test.go` 中启动
当前 Go test binary，产品入口没有 Runner starter。

三切片集成门通过：uncached `go test -count=1 ./...` 391.1 秒、受影响包 race、全仓 vet 与
零告警 staticcheck、module verify、ordinary/secure Desktop、37 个前端文件 128 项测试、strict
TypeScript、Vite production build、npm 高危漏洞为零、确定性 OpenAPI/TypeScript 与 Windows
可复现双构建均为绿色。OpenAPI 为 72 path / 78 operation / 171 schema，OpenAPI 与生成的
TypeScript SHA-256 分别为
`839C731B4D96B9F60A1EB26A0178D0C6212282C0E1287CFC233E7C6AA9520373` 和
`741D882B9213C7AE8244FA90648D08FC351A547334E44CCCA3319C439D4E4D9F`；未签名 GUI
SHA-256 为 `748411c3b3dfd56768c814fd06b6da7e5e81dcd636ad69b658d862afca313e01`，并继续保持
`release_ready=false`、无 installer、无 registry write。

组合审计未发现启用路径中已知未解决高/中风险，也未使用真实 Provider/Key、Shell、
LocalRunner、Docker、Git hook、攻击流量、外网或产品进程。架构完成度维持约 99%，完整产品
可用度约 95-97%，通用 Coding Agent 约 95-96%，Cyber 自动化约 20%。边界见
[ADR 0056](docs/adr/0056-exact-commit-comparison-keyset-verification-runner-control-evidence.md)。

This non-schema v82 batch completes D1-G8, D1-V7, and R5. Operators can compare any
two exact local commit trees through bounded metadata without an ancestry requirement.
Exact verification-item pagination now freezes an event-high-water snapshot and uses
a rank-checked keyset cursor, so later inserts cannot shift already loaded pages.

The internal non-product Runner records only configured wall-deadline/grace controls
and a Go control-plane termination classification. It explicitly does not claim CPU or
memory enforcement, verified OS resource limits, signal identity, or product execution.
The integrated ordinary/focused-race/static/Web/Desktop/build gate passed with no known
unresolved high or medium issue on an enabled path. ADR 0056 is authoritative.

### D1-G9/V8/R6：比较预览、验证快照与 Runner 时间线证据

本轮不新增 SQLite migration，schema 保持 v82。D1-G9 让精确提交比较中的 regular/
executable 变更行直接复用既有脱敏文件预览：added 只显示 head、deleted 只显示 base、
modified 可分别查看两侧；symlink、submodule 与不存在的一侧继续不可预览。选择状态绑定
Workspace 与精确 object，预览标题显示 Go 实际返回的 hash/path，没有新增 Git route 或权限。

D1-V8 新增逐检查项 `snapshot-export?format=markdown|json`。它冻结当前 association event
高水位，最多输出 100 条显式 evidence 引用，携带 Run/Session/Workspace/plan/item 摘要绑定、
pass/fail/unknown 计数、截断位、内容 SHA-256、字节数、安全文件名与固定 MIME，内容上限
256 KiB。导出不含 private body/操作者身份，不推断 verdict、不审批、不改写、不执行；它是
可重建下载回执，不是持久化验收决定。审计同时修复 snapshot 与既有 Handoff export 对
重复或空白包裹 `format` 的接受，现要求唯一且字节精确的值。

R6 仅在内部 `NonProductOnly` 增加 `runner_lifecycle_timeline_evidence.v1` 与
`runner_deadline_budget_evidence.v1`。前者只记录逻辑控制顺序，不记录墙钟/backend timing/
process identity；后者列出 run、TERM/KILL、post-wait、tree/evidence 的独立 Go context 上限，
不宣称累计墙钟 deadline、CPU/memory quota 或 OS enforcement。六份 post-reap evidence
全部验证后原子提交；真实适配器仍只在测试中启动当前 Go test binary。

本轮也是六切片完整健壮性门：全仓普通/race Go 分别 387.3/395.8 秒，vet、双标签
staticcheck/govulncheck、module verify/tidy、ordinary/secure Desktop、37 文件 129 项 Web、
strict TypeScript、Vite/npm、mock-only CLI、隐私/产品入口扫描、Linux cross-compile 与 Windows
可复现构建全绿。OpenAPI 为 73 path / 79 operation / 172 schema；OpenAPI/TypeScript SHA-256
分别为 `8FF7E6A39132ED46DA828009D6D7A603D05B862AEA734E4D8C3E13838DD8A8AE` 与
`FE852EEC8B561D14BD5C3FD1411B2F1930C8A80F15551D071DD3356236BA3503`，未签名 GUI SHA-256
为 `7aa5c3bf67a0af12e51e396977632e5dcc21c74dc04411d3fec7b6f09719aeef`。无已知未解决高/
中风险，且未使用真实 Provider/Key、Shell、LocalRunner、Docker、hook、攻击流量或产品进程。

This non-schema v82 batch completes D1-G9, D1-V8, and R6. Comparison rows reuse the
existing exact redacted preview for each available base/head regular file. Verification
items can produce deterministic digest-bound Markdown/JSON snapshots over one frozen
high-water, with no private bodies, inferred result, mutation, approval, or execution.
The internal Runner now records only a logical lifecycle sequence and independent Go
context ceilings; it still claims no wall-clock measurement, OS quota, signal identity,
or product process authority. The complete six-slice robustness gate passed after one
low-risk exact-format validation fix. [ADR 0057](docs/adr/0057-comparison-preview-verification-snapshot-runner-timeline-evidence.md)
is authoritative.

### D1-G10/V9/R7：成对比较预览、快照回执历史与 Runner 证据集摘要

本轮将 SQLite 推进到 schema v83。D1-G10 在同一个只读区域并列显示精确 base/head
脱敏文件；选择绑定 Workspace、两个 exact object 与 path，added/deleted 文件明确显示缺失侧，
没有新增 Git route、原始 blob/patch、checkout、进程、网络或 hook 权限。

D1-V9 新增不可变 `operator_verification_plan_item_snapshot_receipt.v1` 历史。显式记录前先
重建当前 Markdown/JSON 快照并核对高水位和 SHA-256；事务加写锁后再次复核活动 Code
Session、计划/检查项摘要、关联计数与截断，再原子写入事件和 metadata-only 回执。表不保存
快照正文且拒绝 update/delete；公开 API 不返回私有操作者身份，并把 snapshot/result accepted、
result inferred、rewrite、approval、authority 与 execution 全部固定为 false。“已记录”不等于
“已验收”，未来如需接受/拒绝必须另建独立审查协议。

R7 用 `runner_evidence_set_receipt.v1` 对六份 post-reap 非产品证据的固定无 map JSON 做
SHA-256，只保留协议清单、字节数和摘要。它不保留规范正文，不声明跨记录墙钟顺序、原始输出、
进程身份、OS 限额或产品执行，测试适配器仍未接入任何产品入口。

本轮普通功能门通过：uncached 全仓 Go `394.1s`、全仓 vet、37 文件 129 项 Web、strict
TypeScript、Vite production build、Desktop 边界测试和 Windows 可复现双构建均为绿色。
OpenAPI 为 74 path / 81 operation / 176 schema；生成哈希为
`7E50A343391F167989E871828B1494F45E3A02581198D5B880C3FC3E795B521D` 与
`A693C4E62D65B7D39A5E5668EA319F57E97613AA61234B0658ABC9CBF80F9334`；未签名 GUI
SHA-256 为 `d5e37e193223a41939598edceb77a92637430b0c87c52233cdafb9c2fda10bb5`，仍保持
`release_ready=false`。审计修复三个低风险契约/验证问题：显式 control DTO、inventory v1
枚举和 snapshot-receipt live-route 有效请求夹具；未留下启用路径中的已知
高/中风险。本轮未运行真实 Provider/Key、Shell、LocalRunner、Docker、hook、攻击流量或产品进程。

This schema-v83 batch completes D1-G10, D1-V9, and R7. Operators can inspect both exact
redacted comparison sides together and append a digest-bound metadata receipt for one
deterministic verification snapshot. Recording remains distinct from accepting a
snapshot or result. The internal Runner binds all six post-reap records to one canonical
digest while retaining no raw canonical body and granting no product execution claim.
The three-slice functional gate passed with no known unresolved high/medium issue on an
enabled path. [ADR 0058](docs/adr/0058-paired-comparison-snapshot-receipts-runner-evidence-set.md)
is authoritative.

### D1-G11/V10/R8：成对预览导航、回执复核与 Runner 黄金向量

本轮将 SQLite 推进到 schema v84，并完成当前六切片周期的后三片。D1-G11 在既有精确
comparison 返回的有界文件集合内增加上一项/下一项导航；每次仍只复用两次
`repository_commit_file_preview.v1`，added/deleted 的缺失侧不发请求。它没有新增 Git
route、raw blob/patch、revision expression、checkout、进程、网络、hook 或权限。

D1-V10 新增不可变
`operator_verification_plan_item_snapshot_receipt_review.v1`。操作者只能对一个精确回执
记录一次 `metadata_confirmed|metadata_disputed`，请求必须重复 receipt ID、内容 SHA-256、
回执事件序号并显式确认“不授权”。Go 与 SQLite 在同一事务复核 Run/活动 Code Session/
Workspace/最新模式/回执/事件绑定并原子追加 event + review；同意图可精确重放，变化意图、
第二次复核、过期 digest/sequence、update/delete 均失败关闭。私有 reviewer identity 不进入
公开 DTO；snapshot/result acceptance、inference、rewrite、approval、authority、execution 和
private-body inclusion 全部固定为 false。确认元数据不等于验证通过或批准执行。

R8 为现有 `runner_evidence_set_receipt.v1` 增加 Windows/Linux 共用黄金向量：正常空输出退出
与有界超时/截断元数据各固定 canonical byte count 和 SHA-256。测试重建六份 typed evidence
并复核协议顺序与所有 false claim，不保留 raw output/env/process identity，也不新增产品 Runner
starter。Windows Desktop CI 现在显式运行同一向量测试。

累计六切片完整健壮性门在最终代码上通过：全仓普通/race Go 分别为 357.6/383.4 秒；新增
Verification/Store/HTTP/Runner 聚焦 race 连续 10 轮通过；普通与 secure-Desktop tests/vet、
零告警 staticcheck、双路径零可达漏洞 govulncheck、module verify/tidy、37 个文件 130 项
React 测试、strict TypeScript、Vite production build、npm 零漏洞和 Windows 可复现双构建均
为绿色。OpenAPI 为 75 path / 83 operation / 180 schema，OpenAPI/TypeScript SHA-256 分别为
`F7978C6BBBA216C20A082520BA2B4885D1B16B3D4F42934E3FE328DE1367B075` 与
`E888605EB32D47CCDA045646D84F94AF1BACEC19EBCB976071F1AA2443225112`；未签名 GUI
SHA-256 为 `3bbf545b5ee07597d32345a8dce4f49f063475d881b164a18abf00fd5ff9bc6f`，自动检查通过但
Windows 10/WebView2 人工矩阵仍使 `release_ready=false`。

组合审计修复四项低风险问题：过长 Go 事件标识导致无关格式化噪声、成功 mutation 后 refetch
失败可能留下旧复核按钮、最大事件序号加一前缺少显式溢出拒绝，以及 v84 trigger 未先接入
旧 schema 降级夹具链。当前启用路径无已知未解决高/中风险；未运行真实 Provider/API key、
Shell、LocalRunner、Docker、hook、攻击流量、外网或产品进程。双指标保持架构约 99%、完整
产品可用度约 95-97%、通用 Coding Agent 约 95-96%、Cyber 自动化约 20%。边界见
[ADR 0059](docs/adr/0059-paired-navigation-receipt-review-runner-golden-vectors.md)。

This schema-v84 batch completes D1-G11, D1-V10, and R8, the second half of the current
six-slice cycle. Paired previews now navigate only within the already bounded exact
comparison result. One exact snapshot receipt may receive one immutable
`metadata_confirmed|metadata_disputed` operator review, but that metadata-only fact
cannot accept a snapshot or result, rewrite history, approve work, grant authority, or
start execution. Cross-platform golden vectors pin the existing non-product Runner
receipt bytes and digest without adding a product starter or retaining raw process data.

The complete ordinary/race, static, vulnerability, module, Web, API-contract, and
reproducible Desktop gates passed after four low-risk fixes. No unresolved high/medium
issue is known on an enabled path. No real Provider/key, Shell, LocalRunner, Docker,
hook, attack traffic, external network, installer, registry mutation, or product
process start was used. ADR 0059 is authoritative.

## Project Memory

Read [docs/PROJECT_MEMORY.md](docs/PROJECT_MEMORY.md), [docs/PROJECT_STATUS.md](docs/PROJECT_STATUS.md), [docs/PROGRESS_BOOK.md](docs/PROGRESS_BOOK.md), [docs/TASK_BOOK.md](docs/TASK_BOOK.md), [docs/http-api.md](docs/http-api.md), [docs/openapi.json](docs/openapi.json), [web/README.md](web/README.md), [docs/errors.md](docs/errors.md), and the chronological [ADR 0001](docs/adr/0001-go-control-plane.md), [ADR 0002](docs/adr/0002-run-centric-runtime.md), [ADR 0003](docs/adr/0003-run-execution-modes.md), [ADR 0004](docs/adr/0004-plan-delivery-workflow.md), [ADR 0005](docs/adr/0005-operator-steering-queue.md), [ADR 0006](docs/adr/0006-operator-steering-controls.md), [ADR 0007](docs/adr/0007-specialist-skill-context.md), [ADR 0008](docs/adr/0008-sandbox-manifest-boundary.md), [ADR 0009](docs/adr/0009-sandbox-approval-candidate.md), [ADR 0010](docs/adr/0010-disabled-sandbox-lifecycle.md), [ADR 0011](docs/adr/0011-disabled-sandbox-preflight.md), [ADR 0012](docs/adr/0012-simulation-only-sandbox-evidence.md), [ADR 0013](docs/adr/0013-read-only-docker-observation.md), [ADR 0014](docs/adr/0014-deterministic-docker-container-plan.md), [ADR 0015](docs/adr/0015-bounded-docker-write-rehearsal.md), [ADR 0016](docs/adr/0016-recoverable-docker-rehearsal-attempt.md), [ADR 0017](docs/adr/0017-descriptor-sealed-host-input-staging.md), [ADR 0018](docs/adr/0018-durable-pre-stage-host-input-requirement.md), [ADR 0019](docs/adr/0019-daemon-owned-host-input-handoff.md), [ADR 0020](docs/adr/0020-deterministic-runtime-input-projection.md), [ADR 0021](docs/adr/0021-recoverable-runtime-input-application.md), [ADR 0022](docs/adr/0022-retained-runtime-input-resource-lifecycle.md), [ADR 0023](docs/adr/0023-blocked-docker-start-gate-review.md), [ADR 0024](docs/adr/0024-strict-inert-skill-package.md), [ADR 0025](docs/adr/0025-protected-delete-command-guard.md), [ADR 0026](docs/adr/0026-run-execution-profile-selection.md), [ADR 0027](docs/adr/0027-non-authorizing-docker-production-evidence-ledger.md), and [ADR 0028](docs/adr/0028-recoverable-docker-production-evidence-attempts.md) when resuming development after a long conversation. They record current progress, language ownership, run architecture, execution mode, Plan/Delivery and steering invariants, Specialist Skill delivery, Sandbox authority boundaries, API and error contracts, audit notes, verified commands, and the recommended next slice.

The latest decisions are [ADR 0029](docs/adr/0029-bounded-linux-read-only-docker-evidence-harness.md), [ADR 0030](docs/adr/0030-immutable-docker-production-evidence-review.md), [ADR 0031](docs/adr/0031-content-addressed-inert-skill-registry.md), [ADR 0032](docs/adr/0032-external-skill-run-context.md), [ADR 0033](docs/adr/0033-pathless-desktop-skill-preview.md), [ADR 0034](docs/adr/0034-embedded-read-first-wails-shell.md), [ADR 0035](docs/adr/0035-desktop-lifecycle-and-event-resumption.md), [ADR 0036](docs/adr/0036-idempotent-controlled-run-creation.md), [ADR 0037](docs/adr/0037-controlled-session-message-submission.md), [ADR 0038](docs/adr/0038-idempotent-run-control-and-bounded-handoff.md), [ADR 0039](docs/adr/0039-model-plan-and-approval-controls.md), [ADR 0040](docs/adr/0040-provider-diff-wake-controls.md), [ADR 0041](docs/adr/0041-explicit-wake-file-apply-and-inert-skill-install.md), [ADR 0042](docs/adr/0042-receipts-explorer-portable-build.md), [ADR 0043](docs/adr/0043-workspace-search-evidence-attachment-receipt-history.md), [ADR 0044](docs/adr/0044-operator-action-center-evidence-inventory-command-palette.md), [ADR 0045](docs/adr/0045-go-issued-editor-system-credentials-bounded-wake-worker.md), [ADR 0046](docs/adr/0046-safe-editor-recovery-provider-generation-worker-health.md), [ADR 0047](docs/adr/0047-read-only-repository-change-set-code-journey.md), [ADR 0048](docs/adr/0048-bounded-diff-verification-code-handoff.md), [ADR 0049](docs/adr/0049-deadlock-livelock-runtime-guards.md), [ADR 0050](docs/adr/0050-repository-history-verification-plan-handoff-export.md), [ADR 0051](docs/adr/0051-exact-commit-verification-association-runner-lifecycle.md), [ADR 0052](docs/adr/0052-conservative-model-context-cumulative-handoff-memory.md), [ADR 0053](docs/adr/0053-commit-preview-handoff-coverage-process-conformance.md), [ADR 0054](docs/adr/0054-file-history-verification-drilldown-runner-exit-evidence.md), [ADR 0055](docs/adr/0055-history-navigation-verification-pagination-runner-runtime-evidence.md), [ADR 0056](docs/adr/0056-exact-commit-comparison-keyset-verification-runner-control-evidence.md), [ADR 0057](docs/adr/0057-comparison-preview-verification-snapshot-runner-timeline-evidence.md), [ADR 0058](docs/adr/0058-paired-comparison-snapshot-receipts-runner-evidence-set.md), and [ADR 0059](docs/adr/0059-paired-navigation-receipt-review-runner-golden-vectors.md).

Windows Desktop D0-A/D0-B 与 D1-R1 至 D1-G11/V10 加 R8 非产品 Runner 证据边界的自动化核心已实现，但仍是未签名开发/便携测试壳，不是安装版或完整工作台；Windows 10 实机发布矩阵仍待完成。分阶段方案见 [docs/DESKTOP_PLAN.md](docs/DESKTOP_PLAN.md)。自定义 Skill 已具备严格 `skill_package.v1` 校验、schema v69 本地惰性 Registry、schema v70 CLI Run 选择/最小上下文、schema v71 三端只读来源投影，以及 HTTP/Desktop 显式确认的惰性安装；签名、远程分发和安装时执行仍未开放。详情见 [docs/SKILL_PACKAGE_PLAN.md](docs/SKILL_PACKAGE_PLAN.md)。

The automated Windows Desktop core now reaches D1-G11/V10 plus the non-product R8 Runner evidence boundary, but it remains an unsigned development/portable-test client rather than an installer or complete workbench; the Windows 10 real-machine release matrix is still pending. See [docs/DESKTOP_PLAN.md](docs/DESKTOP_PLAN.md) for the phased Wails/React/Go plan. Custom Skills now have strict `skill_package.v1` validation, the schema-v69 local inert Registry, schema-v70 CLI Run selection/minimized context, schema-v71 read-only provenance across HTTP/TUI/Web, and explicitly confirmed inert HTTP/Desktop installation. Signatures, remote distribution, and install-time execution remain closed. See [docs/SKILL_PACKAGE_PLAN.md](docs/SKILL_PACKAGE_PLAN.md).

## Repository Workflow

The canonical remote is [Qiyuanqiii/CTF-CyberAgent-Workbench](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench). Work is grouped into three focused slices followed by one integrated functional gate and documentation update. Every second batch, after six slices, adds the full race/static-analysis/vulnerability robustness gate. Each completed batch is reviewed, committed, pushed, and verified in GitHub Actions. This repository currently develops directly on `main`; do not create a feature branch or pull request unless the user explicitly asks for one.

Local runtime databases, workspace data, environment files, API keys, IDE metadata, and build output are excluded from Git.

## 许可证 / License

本项目由仓库所有者以 [Apache License 2.0](LICENSE) 授权。你可以在许可证条件下使用、修改和分发代码；分发时请保留许可证与所需声明。Apache-2.0 同时包含明确的专利许可与商标限制，完整法律条款以仓库 `LICENSE` 文件为准。

This project is licensed by the repository owner under the [Apache License 2.0](LICENSE). You may use, modify, and distribute the code subject to its terms, including preservation of the license and required notices. Apache-2.0 also provides an express patent grant and trademark limitations; the repository `LICENSE` file is the authoritative legal text.

## Development Priority

The current priority is the V2 run-centric runtime. P0 and P1 are complete. P2 supports resumable root Agent turns, cumulative token/model-time accounting, bounded execution and Provider retry loops, strict Supervisor-owned `continue`, `finish`, and `wait` actions, one Run execution path for ordinary CLI/TUI Session chat, real Provider streaming with bounded `model.delta` progress, local and schema v18 cross-process active-call cancellation, Bubble Tea live metadata, durable model events, exactly one restart-safe lifecycle-protocol repair, the schema v16 bounded structured-memory tool loop, and schema v17 execution leases with heartbeat/fencing. P3 includes migration v9 Work Board and v10 Notes. P4 covers the schema-v19 root Coordinator through schema-v34 read-only Fan-out. P5 includes the unified Tool Gateway, durable approvals/Grants, typed processes, Artifacts, and structured tools. P9 now includes authenticated API/event surfaces, controlled Run/Session/lifecycle/bounded execution, explicit Provider diagnostics/routes and generation-safe Windows system-credential controls, Plan/Deliver, constrained approvals, schema-v76 review/apply plus safely recoverable Go-issued Monaco proposals, repository state/redacted Diff/history/exact-commit preview/navigation/comparison with navigable paired base/head previews and multi-file review projections, immutable operator verification/coverage with snapshot-stable exact-item pagination, deterministic snapshot download, immutable metadata-only receipt history and non-authorizing receipt review, a regenerable Code Handoff/export and Code Journey, explicit foreground wake consumption and a default-off 1 x 1 worker, inert Skill installation, receipts, bounded Workspace exploration/search, evidence attachment/inventory, operator actions, command palette, portable-build diagnostics, React/Vite, TUI, Headless, and Windows Desktop through D1-G11/V10. xterm, the Windows 10 release matrix, and true Local/Docker execution remain pending. CTF-specific solving logic stays deferred until the generic runtime is stable.

Navigable paired comparison previews, event-high-water/keyset verification pagination, deterministic snapshot export, immutable record-only receipt history, explicit non-authorizing receipt review, and non-product R8 cross-platform receipt vectors are complete. xterm, signed distribution, and the Windows 10 release matrix remain pending. The current narrow controls include read-only repository state, redacted Diff/history/exact-commit preview/navigable exact-file history/comparison, status-only Provider diagnostics/routes/credentials with atomic generation reload, Go-issued FileEdit proposal/read-only recovery plus independently authorized review/apply, immutable operator verification and metadata-only Handoff/snapshot-paginated exact-item coverage/download/receipt history/review, a regenerable non-mutating Code Handoff/export, foreground wake consumption plus an optional 1 x 1-step worker with bounded health, and explicitly confirmed inert Skill registration. R8 pins two deterministic canonical receipt vectors only inside `NonProductOnly`; wall-clock measurement, raw output, process identity, CPU/memory OS limits, signal identity, and host/container process authority remain false.

P7 includes the Go-owned `skill.v1` Registry, immutable embedded-Skill Run selection, durable metadata-only provenance, bounded root context, schema v41 execution modes, schema v42's three-direction Plan/Delivery proposal and operator selection, schema v43 provenance separation, schema v44 per-slice audit/handoff gates, schema v45-v46 operator steering, schema v47 minimal embedded Specialist Skill delivery, schema v69's inert content-addressed user Registry, schema v70's separately confirmed exact external-Skill Run selection plus redacted root/Specialist context, and schema v71's bounded read-only external-Skill provenance across HTTP/TUI/Web. External content is always user-role untrusted guidance and declared dependencies never become capabilities. P6 now includes schema v48's Sandbox Manifest boundary through schema v68's immutable operator receipt review. Real Local/container-process execution remains disabled until every v51 threat-model item has independently verified and accepted production evidence and the real container/input/output transaction passes its own release gate; v52 simulation, v53 metadata observation, v54 compilation/fake writes, v55-v56 non-started daemon rehearsals, v57-v58 local capture facts, v59 never-started handoff evidence, v60 projection metadata, v61 volume/target preparation, v62 resource cleanup, v63 design review, v65 receipts, v66 quiescent checkpoints, v67 read-only metadata, and v68 receipt acceptance do not satisfy that gate. Skill installation/selection, execution modes, Plan selection, document content, audit assertions, queued input, approvals, Sandbox records, profile selection, capture receipts, attempts, harness receipts, and review decisions do not alter Tool Gateway or Policy authority by themselves.

Schema v64 adds immutable `run_execution_profile.v1` snapshots, idempotent CLI/HTTP selection, and the React segmented control. `preview`, `docker`, and `local` all keep process, execution, and capability authority false; Docker and Local remain blocked by independent production/OS-sandbox gates.

P8 now includes schema v35's deterministic Finding/Evidence/Report projection, schema v36's immutable same-Run Artifact Evidence plus one-time operator `validated/rejected` decisions, schema v37's independent acceptance/fresh-remediation/fix lifecycle, confirmed-unresolved SARIF 2.1.0 export, a typed read-only CI gate, and GitHub Actions workflow-command annotations derived from that same GateResult. Source projection digests remain stable across every lifecycle overlay. Additional CI-platform adapters remain future work.

TUI quick controls: `cyberagent tui` opens the Run-first picker; `Tab` or `h/l` switches between Run and Session lists. In chat, `Tab` switches between input and the activity pane; `h/l` changes among Tools, Plan, Queue, Work, Notes, Rounds, Events, Agents, Findings, and Edits; `j/k` moves the current selection. Plan is read-only and shows the three directions, selected direction, projected WorkItem count, and whether an explicit Deliver transition is still needed. Queue is metadata-only and shows ordered steering state without content or mutation controls. `Enter` opens the selected Edit's read-only diff and `Esc` returns. `a` approves one Shell proposal, `g` approves it and grants the exact current session scope, and `d` denies it only while Tools is active. Plain input entered during a busy model action is queued without clearing live progress; slash commands remain blocked. `PgUp/PgDn` scrolls messages or an open diff, `Ctrl+X` requests cancellation of the current model call, `Ctrl+R` refreshes, and `Esc` quits when idle outside the diff screen. Busy sends cannot be closed accidentally with `Esc`; cancel or wait first. The status line renders provider/model, attempt, chunk/byte progress, steering counts, durable event tail, cancellation, disconnect, and terminal state without exposing raw model text. Attached workspaces render in the side panel with local directory counts for attachments, scripts, outputs, logs, and writeups.

Workspace file reads and model-bound messages pass through heuristic secret redaction for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, and private-key blocks.

File changes, Shell proposals, typed script-process proposals, and create-only structured memory calls enter the same Tool Gateway and Run-event stream. The schema v11 approval ledger is inspectable through `approval list/show`; review idempotency survives restart, while a conflicting reuse of the same key is rejected. Schema v12 Session grants are managed through `approval grant create/list/show/revoke`, and `run usage` exposes durable tool consumption. Schema v13 stores script processes separately from legacy Shell ToolRuns and makes initial Run creation recoverably idempotent. Schema v14 stores redacted full-stream evidence separately from bounded Results and links each Artifact to its exact proposal or invocation. Schema v15 stores only operation-key digests and normalized request fingerprints for WorkItem/Note creation. `edit propose` and session `/write` normally persist a redacted diff without changing the workspace; an explicitly created matching Session grant may authorize and apply the edit immediately, with the grant ID recorded on the approval fact. Shell and script-process approval still produce dry-run output only.

## 可选在线 Provider / Optional Online Providers

统一 Go Registry 可从当前进程环境或受支持的系统凭证存储加载 `mimo`、`deepseek` 与 `anthropic`，CLI、API 和 Desktop 共用初始化；环境变量优先。密钥不会写入仓库文件、SQLite、Run 事件或 TypeScript，系统凭证设置接口也只返回状态。修改后 Go 先完整构建并校验候选 Registry/route，再原子切换 generation；失败保留当前 generation，成功无需重启。模型可用性读取不发起探测，只有显式 `provider test`/桌面诊断会发送一次无用户正文、禁用工具的有界请求。诊断结果不返回模型正文、Base URL、环境变量名或原始错误。Provider 地址必须使用 HTTPS，只有 loopback 本地服务可使用 HTTP；客户端不会跟随重定向，API key 也不能包含空白或控制字符。<br>
The unified Go Registry can load `mimo`, `deepseek`, and `anthropic` from the current process environment or a supported system credential store; environment variables take priority, and CLI, API, and Desktop share initialization. Keys never enter repository files, SQLite, Run events, or TypeScript, and credential controls remain status-only. A change builds and validates a complete candidate Registry plus routes before atomically swapping generations; failure preserves the current generation and success requires no restart. Availability reads perform no probe; only explicit `provider test` or a Desktop diagnostic sends one bounded, content-free, tool-disabled request. Diagnostic output contains no model text, base URL, environment-variable name, or raw error. Provider endpoints must use HTTPS except for loopback-local HTTP services; redirects are not followed, and API keys cannot contain whitespace or control characters.

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
go run ./cmd/cyberagent model set code deepseek/deepseek-v4-flash
go run ./cmd/cyberagent run create "review this workspace" --profile review --route deepseek/deepseek-v4-flash
```

每次 `provider test` 可能产生一次 Provider 请求和少量计费。`model set` 会先持久化 SQLite，成功后才更新当前进程 Router。`run wake schedule/show/cancel` 只管理意图；显式 `run wake consume` 或手动开启的 `--enable-wake-worker` 才能通过现有 Supervisor 消费，worker 每轮固定最多一条 intent/一步且不能启动 Shell/Local/Docker。<br>
Each `provider test` may incur one Provider request and a small charge. `model set` persists SQLite before updating the current process Router. `run wake schedule/show/cancel` manages intent only; an explicit `run wake consume` or manually enabled `--enable-wake-worker` may consume it through the existing Supervisor. The worker is capped at one intent and one step per tick and cannot start Shell/Local/Docker.

`DEEPSEEK_BASE_URL` and `DEEPSEEK_MODEL` are optional; their current defaults are the values shown above. Use `deepseek-v4-pro` explicitly when the higher-capability model is required. See the official [DeepSeek Anthropic API guide](https://api-docs.deepseek.com/guides/anthropic_api) for current compatibility and model details.
