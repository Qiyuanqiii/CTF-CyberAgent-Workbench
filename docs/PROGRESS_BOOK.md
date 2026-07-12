# CyberAgent Workbench 进度书

更新时间：2026-07-12

## 一、当前阶段

项目正在从可运行的 v0.1 CLI/TUI 骨架迁移到 V2 Run-centric Runtime。CTF 专用求解能力继续后置，当前先完成主流 AI Agent 工具需要的通用运行时。

当前完成度：

- 整体产品愿景：约 94%。
- v0.1 通用 Agent MVP：约 99%。
- V2 Run-centric Runtime：约 99%。
- 项目骨架和模块边界：约 99%。

V2 的 P0/P1 已完成，P2 已具备稳定的单 Agent 恢复、Provider streaming、进程内主动取消、schema v16 有界工具循环、schema v17 跨进程执行租约/心跳/fencing，以及 schema v18 独立 capability 的跨进程 root 活动模型取消。P3 主体已落地：WorkItem/schema v9、Note/schema v10、事务化关系与事件、完整 `todo`/`note` CLI、可见性、8192-token Context Builder，以及不含正文的持久化上下文来源审计。P4 已完成 schema v19 单 root Coordinator、schema v20 可恢复 inbox、schema v21 internal-only Specialist admission、schema v22 Agent-owned memory、schema v23 CompletionReport、schema v24 Specialist Attempt Runtime、schema v25 root inbox context、schema v26 internal-only no-tool Specialist turn、schema v27 recoverable child context、schema v28 child lifecycle repair、schema v29 durable schedule/cross-process child cancellation、schema v30 review-gated delegation proposal，以及最多两个 child 的 Go-internal 有界并发调度：稳定 Agent 身份、有界幂等 inbox、严格 wake/dependency、最多两个 depth-1 child、独立 Session、Skill/预算预留、同 Run memory ownership、lease-fenced Attempt、累计 usage 计费、崩溃通知、接管恢复、attempt-bound `agent.finish`、root/child 两阶段 exactly-once context、严格 child lifecycle、一次隔离修复、持久化调度摘要、精确 child-call 取消、root 提案与 admission 权限分离、取消扇出及 root+children SQLite 总预算复核均已落地；公开/model admission、spawn 和自主 child scheduling 继续关闭。P5 已落地统一 Tool Gateway、schema v11 持久化幂等逐次审批、schema v12 可撤销 Session Grant 与 Run 工具预算、schema v13 first-class ScriptProcess、schema v14 来源绑定的脱敏输出 Artifact、schema v15 create-only WorkItem/Note 结构化工具、schema v16 可恢复 Provider 工具批次，以及 schema v30 `agent_proposal` class。P9 已新增 loopback-only `api.v1` 读取面、独立授权的 root/Specialist 精确取消 POST、由 Go DTO 生成且受 golden/live-route 测试保护的 OpenAPI 3.1 契约、有界可恢复 Run-event SSE，以及 Run-aware TUI 的 Work/Notes/ToolRounds/Tools 视图和批准一次/本会话操作；真实命令执行、通用 API 写入和执行类模型工具继续关闭。

## 二、已完成功能

### Agent 与运行时

- CLI 入口、命令分发、版本命令和 Bubble Tea TUI。
- Agent Kernel、Planner、Executor、Critic 与 Task/Event 类型边界。
- schema v19 为每个新 Run 原子创建稳定 root `AgentNode`；旧库可惰性补建。root 的 ready/running/waiting/completed/failed/cancelled、turn/token 用量和 active attempt 与 RunSupervisor/RunService 同事务投影。
- `internal/coordinator` 提供 root register、结构化 inbox send/consume、snapshot/restore；`run graph` 验证当前节点与 pending inbox metadata 是否匹配最新 `agent_graph.v1` 快照。
- 图最多 3 个节点、深度 1、每个 inbox 最多 128 条 pending/4096 条总历史、每批消费 32 条、快照保留 32 份；payload 递归脱敏并以 SHA-256 进入快照完整性校验。默认 Coordinator 无 admission capability，显式内部 policy 才能把 root capacity 提升到最多两个 child；Runner/context/scheduler 均只有 Go 内部显式入口，仍没有公开/model spawn。
- schema v20 要求 inbox send 携带 16-256 字节幂等键，只持久化域分隔摘要与脱敏意图指纹；原键重放不重复消息、事件、快照或 wake 状态转换，异意图复用返回稳定冲突。
- `wake` 仅允许 running Run 内的 waiting Specialist 转为 ready，不能唤醒 root 或恢复暂停 Run；`dependency` 要求同 Run Agent sender 和严格枚举 payload。普通 v19 消息/快照可原地升级。
- schema v21 admission 默认关闭；只有带显式 `SpecialistAdmissionPolicy` 的 Go Coordinator 可创建 child，容量硬限制为两个、深度为 1、Skills 必须为父集合子集，且每个 child 有独立活动 Session。
- child turn/token 必须正数预留并给 root 留出协调额度；root 后续 Supervisor budget 使用扣除预留后的有效值。并发跨 Store 重试只生成一个 child，失败事件会回滚 capacity、Session、节点和操作事实。
- Run pause/resume 只恢复因 Run 生命周期进入 waiting 的 child；Run complete/fail/cancel 原子终止 child、归档其 Session 并更新图快照。Go-internal `SpecialistScheduler` 每轮最多并发两个显式 ready child、最多 32 轮；公开 spawn、模型自主 admission 与 autonomous scheduling 仍关闭。
- schema v22 为 WorkItem/Note 增加可选 `owner_agent_id`；Store 与 SQLite trigger 双重拒绝缺失、终态新分配和跨 Run Agent，v21 行与旧 `owner` 标签原地保留。
- schema v23 增加严格 `agent_completion.v1`、`agent_completion_reports` 与摘要幂等账本；`agent.finish` 绑定 active attempt，并原子提交父 result inbox、child 完成、Session 归档、事件和快照。
- schema v24 增加 `agent_attempts`/`agent_attempt_mutations`、turn/usage 预算计费、Run lease fencing、崩溃通知、暂停中断和 takeover recovery；默认 Coordinator 仍不具备 Specialist runtime capability。
- schema v25 增加 `root_inbox_deliveries` 两阶段账本；root 每轮最多准备 4 条 direct-child dependency/result/failure 消息，成功 turn 原子提交消费，失败 supersede 且保留 pending，取消/重启/lease takeover 重放同一 attempt 批次。
- schema v26 增加严格 `specialist_lifecycle.v1`、`specialist_model_calls` 与 `SpecialistRunner`；只允许 no-tool `continue`/`finish`，模型终态、usage、Policy 和脱敏 child Session 消息原子提交，Provider 重试、context cancellation、历史 12 条/64 KiB 总量和 lease takeover 均由 Go 控制。
- schema v27 增加严格 `specialist_instruction.v1` 与 `specialist_context_deliveries`；每个 child Attempt 最多准备 4 条直属父指令，并在 4096-token/32 KiB 双重上限内选择 child-owned active WorkItem/Note。成功 lifecycle 原子消费，crash/interruption/takeover supersede 后重投，模型看不到 message ID 或 owner/sender 控制字段，`model.started` 只记录来源 ID 与 token 估算。
- `SpecialistScheduler` 在一个 Run execution lease 下按稳定 Agent ID 每轮最多启动两个 goroutine；父 context、lease heartbeat loss 或首个 child 错误统一取消同轮其他 child，并等待全部 Attempt 写入终态后才释放 lease。停止条件覆盖 all-terminal、no-ready、round-limit、cancelled、child-error、token-budget 与 execution-budget。
- schema v29 以 `specialist_schedules`/`specialist_schedule_agents` 持久化 schedule started/stopped summary；接管 generation 将旧 running schedule 收敛为 `abandoned/worker_lost`，公开领域对象和事件均不含 lease id/generation。
- schema v29 以独立 `specialist_model_cancellations` 与 digest-only operation ledger 支持跨进程精确 child-call 取消；控制面绑定 Run/Agent/AgentAttempt/model attempt，worker 观察后取消本地 context，终态或 Attempt 回收会原子解析旧请求。
- schema v30 以严格 `specialist_delegation.v1`、`specialist_delegation_proposals/assignments/operations` 和 `agent.delegation_proposed` 支持 root 最多提出两个待审 child 目标；active root/lease/scope/Skill/capacity/budget 全部由 Go/SQLite 复核，proposal 不创建 Agent、Session、预算预留或 schedule。
- `RunAgentUsage` 每轮前后从 SQLite 重建总账：root token/执行时间来自 Supervisor checkpoint，child token 必须在 Agent 投影与 Attempt ledger 间一致，所有 child model-call elapsed 求和；投影漂移返回 `CONFLICT`，剩余 token/毫秒按排序后的 active child 确定性分片。
- root inbox 进入模型前执行持久化协议关联、sender 路由、严格 JSON、脱敏、字段截断和 8192-token 全批次适配；prompt 不含消息 ID、sequence/cursor，模型不能选择 sender 或提交消费位置。
- Supervisor 与 `tool invoke` 从 Go-owned Run/Agent 状态注入所有者，模型 schema 不包含 `owner_agent_id`；CLI/TUI/HTTP/OpenAPI 可显示和过滤 Agent owner。root/Specialist 的 owner-only Notes 按真实 Agent 身份隔离。
- 持久化 Session、Message、Task、Event、Artifact 和上下文摘要。
- Codex 风格的长上下文压缩骨架，支持手动和自动压缩。
- `/help`、`/compact`、`/model`、`/workspace`、`/ls`、`/read`、`/write`、`/run` 会话命令。
- `cyberagent api serve` 提供回环限定、稳定 envelope、有界 cursor pagination 和优雅关闭的本地读取面，覆盖 Run/Session/Event/WorkItem/Note/Artifact metadata/ToolRound，以及不含 fencing token 的 execution-lease 摘要；schema v18 的唯一控制 POST 使用不同 token。
- `cyberagent api openapi` 从 Go read DTO 与显式路由目录生成确定性 OpenAPI 3.1 JSON；支持 stdout/文件导出，运行时 `/api/v1/openapi.json` 在同一鉴权边界返回原始契约。
- `/api/v1/runs/{run_id}/events/stream` 从 SQLite sequence 增量投影已脱敏 Run events，支持 Run-bound cursor/`Last-Event-ID` 恢复、心跳、跨连接写入可见性、慢写 deadline、连接寿命/事件数和进程并发上限。

### 模型层

- Provider 接口、模型路由和可重复测试的 MockProvider。
- Anthropic-compatible Provider，已用环境变量完成 Mimo 与 DeepSeek 连通验证；二者都有独立 Provider 名称和环境变量边界。
- 模型请求进入 Provider 前进行敏感信息脱敏。
- Provider 错误统一分类为 retryable、rate_limited、invalid_response、cancelled、permanent。
- Anthropic-compatible Provider 对 429、408/425、5xx/529、永久 4xx、畸形/空响应和 `Retry-After` 进行类型化处理。
- RunSupervisor 默认最多三次模型尝试，100ms 指数退避且单次最多等待 2s；超长 `Retry-After` 不会被提前重试。
- 每次模型调用持久化连续编号的 `model.started/completed/failed`，取消与重启后继续编号。
- 无效 root lifecycle 输出只触发一次纠错提示；不会把原始坏输出回放给模型、写入 Session 或写入事件。
- transport retry 每个协议阶段独立最多三次，全局 model attempt 编号保持连续；CLI 显示 `protocol_repairs`。
- Router 的 StreamChat 与 Chat 共用模型解析和请求脱敏；RunSupervisor 不再旁路 streaming 接口。
- Anthropic-compatible Provider 解析 `message_start/content_block_delta/message_delta/message_stop/error` SSE，并在 final chunk 返回模型与 usage。
- Stream aggregator 支持跨 chunk UTF-8，拒绝超 64 KiB、缺失 Done/usage、负数或溢出 usage、非 final ToolCall、畸形流和中途取消；final ToolCall 会进入严格类型化工具循环。
- Anthropic-compatible Provider 支持 `tools`、assistant `tool_use`、user `tool_result`、非流式 ToolCall 和 SSE `input_json_delta` 聚合。

### 工作区、编辑与工具

- 本地工作区创建、查询、目录树和文本读取。
- 拒绝绝对路径、`..` 穿越和符号链接逃逸。
- 纯 Go Tool Gateway 定义 `ToolCall/Decision/Proposal/Execution/Result/Outcome`、稳定状态和合法组合校验。
- `read_file`、`list_workspace`、Shell ToolRun 与 FileEdit 现在共享 schema、scope、policy、approval 和结果规范化入口。
- CLI、Session 与 TUI 生产路径通过 Gateway/兼容适配器调用；底层 read tool、ToolRun Manager 和 FileEdit Manager 的构造已集中到 Gateway。
- 生产文件操作按 workspace ID 查询 Store-owned root，并在调用方路径不一致时于文件访问前拒绝。
- 输出执行 UTF-8、MIME、stdout/stderr/preview 硬限制、截断标记和敏感信息脱敏。
- schema v14 在 Result 截断前捕获最多 4 MiB 的脱敏工具 stdout/stderr，持久化 MIME、UTF-8、SHA-256、字节数和精确来源，并原子追加不含正文的 `artifact.created`。
- `artifact list/show/read/verify` 可检索元数据、有界读取脱敏正文并复核哈希；重复审批返回同一 Artifact，不重复事件。
- schema v15 新增 `run_memory` action class 与严格 JSON 的 `work_item_create`/`note_create`。调用绑定持久化 Run/Session/Workspace、经过 Policy 与 Run 工具预算，并把业务实体、允许决定、领域事件、`tool.completed` 和幂等账本原子提交。
- `tool schema [name]` 可导出 Provider-ready schema，`tool invoke <name> --run ... --operation-key ... --payload/--payload-file` 可执行 create-only 调用；CLI 从 Run 推导可信 Session/Workspace，调用方不能伪造审计 requester。
- schema v15 `structured_tool_operations` 只保存原始 operation key 的域分隔 SHA-256 与脱敏规范化意图指纹。相同意图重放返回原实体，异意图冲突，跨 SQLite 连接并发收敛为一个实体；格式错误在预算前拒绝，重放、冲突和 Policy 拒绝按真实调用计入预算。
- schema v16 新增 `run_supervisor_tool_rounds`/`run_supervisor_tool_calls`。模型完成事件与 pending 批次原子提交，每条结果和 round-complete 事件也在一个事务内提交；重启只重放 pending 调用。
- RunSupervisor 仅公布 `work_item_create`/`note_create`，每批最多 4 个调用、每 turn 最多 4 轮。Provider call ID 不落库，语义 operation key 来自 Run/turn/工具/脱敏规范化参数；Policy 拒绝和预算耗尽作为 metadata-only error result 返回模型。
- `script run` 现在要求 workspace 相对文件，并在单个 SQLite 事务中创建 Script Profile Mission/Run/Session、初始预算扣减、`script_process.v1` typed proposal、Approval 及 Policy/Tool events。
- `script run --idempotency-key` 可安全跨进程重试；相同意图返回同一 Mission/Run/Session/Process，异意图复用返回冲突，原始 key 不持久化。
- `--local` 只记录 requested backend；CLI 不再构造 Local/Noop Runner，审批前后均只产生 dry-run，不执行宿主机命令。
- Store 对 JSON payload 先解析、递归脱敏字符串值再编码，嵌套 JSON、转义和 64-bit 数字不会被字符串级正则破坏。
- 文件编辑提案、diff 预览、审批、拒绝、失败和已应用状态持久化。
- 审批前重新解析路径并校验 SHA-256，拒绝覆盖提案后被修改的文件。
- ToolRun 提案与审批状态机；`/run` 当前只创建提案，批准仍为 dry-run。
- schema v11 `tool_approvals` 为 Shell/FileEdit 提案记录 Run、Session、Workspace、Tool、ActionClass、模式、状态和 SHA-256 请求指纹，不重复保存原始命令或文件正文。
- `approval_operations` 以不可变幂等键记录 approve/deny 意图；相同请求可跨重启重放，不同意图复用同一键会被拒绝。
- 提案事务同时提交 `approval.requested`；审批准入先提交 `approval.decided` 再推进兼容状态，崩溃后重试可收敛。
- Store 拒绝幽灵审批、提案指纹变化，以及没有匹配批准事实的 `approved`/`applied`/`completed`；`approval list/show` 可检查账本。
- schema v12 `approval_session_grants` 精确绑定 Run、Session、Workspace、Tool 和 ActionClass；创建/撤销幂等键只保存域分隔摘要，`approval grant create/list/show/revoke` 可跨进程检查与撤销。
- 活动 Grant 只能在 Policy 允许后自动批准匹配提案；终态 Run、归档 Session、跨作用域调用和已撤销 Grant 均不能使用，危险 Shell 永久拒绝不会被覆盖。
- schema v12 `run_tool_usage`/`run_tool_calls` 原子记录有序工具调用；新 CLI Run 默认 100 次，可用 `--max-tool-calls` 调整，`run usage` 展示消费量；Gateway Store 在编译期强制依赖 Grant 与 Budget 接口，新增后端不能静默省略。
- 第一次超预算尝试只追加一条 `tool.budget_exhausted` 并返回稳定 `RESOURCE_EXHAUSTED`；重复拒绝不刷事件，并发调用不会超支。
- Policy Checker 拒绝未授权扫描、公网攻击、凭证窃取和明显破坏性命令。
- Noop、Local 和占位 Docker Runner；Docker 当前只检测并返回明确错误。

### 存储与 Run 架构

- CGO SQLite 驱动 `github.com/mattn/go-sqlite3`，当前 schema 版本为 v27。
- checksum 校验的版本化事务 migration，可保留旧库数据原地升级。
- Mission、Run 和 append-only Run Events 持久化。
- schema v3 为非空 `session_id` 建立唯一关联并拒绝引用不存在 Session 的 Run。
- 新 Run 默认在同一事务中创建独立 Session；也可绑定一次现有活跃 Session，并统一工作区和模型路由。
- Session Message、assistant policy、ToolRun 和 FileEdit 状态会在业务写入的同一事务内投影为 Run Event。
- 重复保存不会产生重复事件；跨工作区 ToolRun/FileEdit 会在 Store 边界回滚。
- `apperror` 提供稳定代码、CLI 退出码和未来 HTTP 映射，现有错误文本保持不变。
- schema v4 使用 `legacy_task_runs.task_id` 作为幂等键，`run adapt-task` 可安全重复或并发执行。
- TaskAdapter 在一个事务内创建 Session、Mission、Run、映射和四条初始事件（含 root `agent.registered`）；历史状态不会触发隐式执行。
- 旧 Task Goal 与旧 Event 内容补齐 Store 级脱敏。
- schema v5 持久化 Supervisor phase、next turn、attempt 和脱敏错误。
- schema v6 在同一 checkpoint 中持久化累计 input/output/total tokens 与模型执行毫秒数。
- schema v7 在 Provider 调用前持久化脱敏且不超过 64 KiB 的 pending input，完成后原子清空。
- schema v8 持久化协议修复阶段和脱敏原因，支持 pending/exhausted 状态的进程重启恢复。
- schema v9 新增 Run-scoped `work_items` 与 `work_item_dependencies`；复合外键拒绝跨 Run 依赖，Store 额外拒绝缺失依赖、自依赖和依赖环。
- WorkItem 具有 pending/in_progress/blocked/completed/cancelled 状态、四级优先级、Owner、验收条件、依赖、阻塞原因、完成时间和乐观版本。
- 工作项创建/更新与 `work_item.created/changed` 在同一事务提交；事件失败会回滚记录，陈旧版本只允许一个并发写入者成功。
- `todo create/list/show/update/start/block/reopen/complete/cancel` 已可用；依赖完成前不能启动或完成下游项，终态 WorkItem 与终态 Run 不可继续修改。
- 每次 Supervisor 调用只注入最多 20 个活跃 WorkItem，使用不超过 16 KiB 的脱敏 `work_board.v1` JSON；completed/cancelled 不进入模型上下文。
- 模型在仍有活跃 WorkItem 时返回 `finish` 会进入现有的一次协议修复，不能绕过工作板完成 Run；显式 `run finish` 保留为操作者覆盖。
- `CompleteSupervisorTurn` 在取得 SQLite 写锁后再次检查活跃任务；模型调用期间由另一进程新建任务时，陈旧 `finish` 会回滚且保留可恢复 checkpoint。
- schema v10 新增 Run-scoped `notes`、`note_tags`、`note_sources` 与 `note_evidence`；v9 数据库可原地升级并保留 WorkItem。
- Note 支持 observation/hypothesis/decision/summary/reference、run/root/owner 可见性、Owner、标签、来源、Evidence ID、pin、active/archive 和乐观版本。
- Note 记录、关系表与 `note.created/changed` 在同一事务提交；事件失败回滚正文和关系，并发陈旧写入只允许一个胜者。
- `note create/list/show/get/update/archive/restore` 已可用；支持 UTF-8 content-file 的实际读取上限、联合标签过滤、pin/unpin、列表清空和显式 `--version`。
- root Supervisor 只查询 run/root/`owner=root` 的 active Notes；其他 Owner 和 archived Notes 不进入模型上下文。
- 通用 Context Section 选择器按优先级在 8192-token 估算预算内选择最新摘要、Work Board 与 Notes，pin 和 decision/summary 类别优先。
- 每条 `model.started` 持久化 included/omitted 的 kind/source_id/tokens 与总预算，不持久化 Note 正文，重启后仍可审计当次模型上下文来源。
- schema v15 新增带 Run/Session/Workspace、预算 invocation、target 与 requester 约束的结构化工具操作事实；SQLite trigger 拒绝跨作用域 invocation 或目标，所有写事务使用 immediate lock 和 5 秒 busy timeout，避免多进程 deferred transaction 升级竞争。
- schema v16 新增 Supervisor 工具 round/call 状态、严格 Store 复核、active-attempt/model-attempt trigger 和 terminal-result 不变量。
- schema v17 新增每 Run 一行的 `run_execution_leases` 与 checkpoint fencing 字段。`run step` 持有单 turn 租约，`run execute` 在整个有界循环复用一份租约；默认 30 秒 TTL、10 秒心跳，过期接管递增 generation。
- Supervisor checkpoint、模型事件、ToolRound/ToolResult、结构化 WorkItem/Note 写入和 Run 工具预算均在各自 SQLite 事务中检查同一 `lease_id + generation`。旧 worker 接管后不能继续提交或消耗预算。
- 同 owner 的普通二次获取也返回冲突；只有显式携带当前 `lease_id` 的获取重试可幂等续用，避免同一 Supervisor 并发共享租约。`run lease` 与 HTTP Run detail 只公开 owner/generation/status/timestamps。
- `run step` 每次执行一个有界 turn，内部可包含最多四轮结构化记忆调用；`run checkpoint` 可观察恢复状态，CLI 结果展示 `tool_rounds`/`tool_calls`。
- 模型调用前写 started checkpoint，完成时原子写消息、策略、用量、事件和下一个 checkpoint。
- 重启会恢复同一 started attempt；已提交完成的 turn 和消息不会重复。
- MaxTurns、MaxTokens、模型执行超时与调用前 cancellation 已执行；剩余 token/时间会传入 Provider 请求上下文。
- `run execute` 提供显式步数上限；`run finish/fail` 在一个事务内推进 Run、Supervisor checkpoint 和事件，重复相同终态命令幂等。
- 模型只可调用 create-only WorkItem/Note；其他 ToolCall 会进入一次无工具协议修复，仍不合规则失败。只有校验通过的结构化 lifecycle action 能推进 Run，自由文本不能。
- root Agent 必须返回严格 JSON action；未知字段、尾随数据、Markdown fence、非法字段组合和超过 64 KiB 的回复都会失败。
- `continue` 回到 idle；`finish` 原子提交 turn 与 completed；`wait` 原子提交 turn 与 paused，`run resume` 后从下一 turn 继续。
- 原始 lifecycle JSON 不进入 Session history；只持久化脱敏后的用户可见 message、summary/reason 与审计事件。
- 即时 CLI 模型回复和持久化回复都经过同一脱敏边界。
- `session.RunChatExecutor` 以无包循环接口连接 Session Manager 和 application 层 RunSupervisor。
- Run-bound Session 的普通 CLI/TUI 消息自动启动 created Run、自动恢复 paused Run，并返回 action/status；终态或 waiting-approval Run 拒绝新 turn。
- pending input 在重启后恢复为同一 attempt；成功时 user/assistant 消息、checkpoint、Run 状态和事件一次事务提交，不会重复写消息。
- 未绑定 Run 的旧 Session 暂时保留 direct Router 兼容路径；slash command 继续作为显式命令路径。
- 自动压缩生成的最新 Session summary 会进入后续 Supervisor 模型上下文。
- 模型终态事件、token 用量与 execution_millis 在一个事务记账；终态重放不会重复事件或重复扣减预算。
- 首次非法协议响应的用量先持久化再检查剩余预算；修复成功只提交一对合法 Session 消息，二次非法响应直接失败且不会第三次调用。
- 修复请求、开始、完成和失败分别写入 `supervisor.protocol_repair_*` 事件；父 context 在响应返回时取消也不会丢失终态或修复 checkpoint。
- 每次模型调用最多持久化 32 条 `model.delta`，只包含 chunk/byte/sequence/done 计数，不包含模型文本；终态必须与 delta 账本一致。
- active-call registry 按 Run/attempt 唯一占位，只有 `model.started` 成功后才对外可见；Provider 所有终态都会清理注册项。
- application API 支持活动调用单项/列表查询、32 槽有界订阅和幂等取消；慢消费者关闭，不反向阻塞模型流。
- 显式主动取消先写脱敏且幂等的 `model.cancel_requested`，再触发 Go 持有的 cancel function；终端 `Ctrl+C`/SIGTERM 也进入 Supervisor 取消与恢复路径。
- ActiveCallInfo 带 Store 绑定的 Session ID；Bubble Tea 发送与订阅命令并行运行，不在 `Update` 中执行阻塞 I/O。
- TUI 状态栏显示 provider/model、attempt、chunk/byte、取消、慢消费者断线和终态；live envelope 仍不含模型文本。
- TUI 活动区从 Go Store 有界加载当前 Run、WorkItems、Notes、Supervisor ToolRounds、Shell ToolRuns 与活动 Grant；`h/l` 切换视图，`j/k` 选择，非 Tools 视图保持只读。
- `a` 执行持久化逐次审批，`g` 创建或复用精确 Run/Session/Workspace/Shell/ActionClass Grant 并推进当前提案；推进前重新验证审批指纹、作用域和当前 Policy，Shell 仍只 dry-run。
- TUI 换行和截断按终端单元格与字素计算，中文和宽字符不会被切成非法 UTF-8 或撑破默认侧栏。
- `Ctrl+X` 优先调用 audit-first active-call API；legacy/尚未激活调用在短时查找后仅取消当前 application request context，不接触 Provider cancel function。
- busy 时 `Esc/Ctrl+C` 只提示等待或 `Ctrl+X`，避免键盘退出直接中断进程内调用；picker 打开和新建会话都会继承同一控制器。
- 限流耗尽后 checkpoint 保留 pending input；无新输入的 `run step` 会继续原请求而不是退回 Mission goal。
- Provider 调用中或退避中的 context 取消会停止重试；调用中取消使用短时审计上下文记录 cancelled 事件和耗时，turn 保持可恢复。
- Run 状态转换与事件写入保持原子性，Store 会拒绝非法或陈旧转换。
- `run create/list/show/events/start/pause/resume/cancel` 已可使用。
- `Run` 是可恢复的执行实例，不是编程语言；Go 负责控制，TypeScript 负责界面，Rust 负责确定性分析。

## 三、语言与架构边界

当前仓库以 Go 为唯一业务实现语言。SQLite 通过 CGO 使用 C 编译链；SQL 嵌入 Go migration；YAML 用于配置，Markdown 用于文档，PowerShell 只出现在 Windows 使用示例中。

长期边界：

- Go 是唯一控制平面，负责 Agent、Run、Session、LLM、Secrets、Policy、Workspace、SQLite、Docker 和审计。
- Rust 只做确定性分析工具，通过 stdin/stdout JSON 接收任务并返回结果，不调用 LLM、不管理会话或用户配置。
- TypeScript 只做 React/Web 界面，通过 HTTP/WebSocket 调用 Go，不直接操作 Rust、Docker、Shell、SQLite 或 API key。
- 合法链路是 `TypeScript -> Go -> Rust/Docker/LLM`。

依赖按功能引入。Bubble Tea、SQLite/CGO 和 `net/http` 已使用；OpenAPI/JSON Schema 当前由标准库反射生成，未增加运行时依赖。Cobra、Chi、Docker client、Qdrant、Rust crates 与 React/Vite 等到对应功能开始实现时再加入。

## 四、尚未完成

- 经过生命周期投影和跨 chunk 脱敏的用户可见文本 streaming；当前 TUI 只展示元数据。
- Provider 费用预算，以及最近 Session 消息与结构化记忆共用的统一 token 预算。
- Findings、Evidence 实体与 Report；WorkItem/Note 基础、create-only Provider 调度循环和 TUI 专用视图已完成。
- 经过单独安全设计的用户可见模型文本 streaming；持久化 metadata SSE 与跨进程主动取消已完成。
- OpenAI-compatible 与 Ollama Provider。
- 真实 Docker 隔离与命令执行；Tool Gateway、first-class ScriptProcess、逐次审批、Session Grant、工具预算和输出 Artifact 已完成。
- 通用 HTTP 写入接口、TypeScript Web UI 和 Rust analyzer 进程；本地读取 API、metadata SSE 与精确取消控制已完成。
- MCP Server、插件系统和远程任务能力。
- 通用 Agent 稳定后的 CTF 自动分析与求解流程。
- 子 Agent admission、Attempt scheduling、独立 Session/累计预算计费、崩溃恢复、消息唤醒、root/child context projection、WorkItem/Note Owner 外键、一次 lifecycle repair 和内部双 child 并发编排已完成；尚缺结构化 dependency waiting、持久化 schedule 摘要、跨进程 child-call cancellation 与公开/自主调度产品面。

## 五、审计结论

最新审计未发现高严重度问题。主要残余风险：

- schema v17 已解决同一 SQLite 数据库上的跨进程 Run 执行互斥和 stale-write fencing，schema v18 用独立 capability 把取消意图持久化并交给持有私有 lease 的 worker 消费；瞬时 registry 与 Provider cancel 函数仍不跨进程暴露。
- execution lease 依赖本机 UTC 时钟与 SQLite 写事务；它不是分布式共识协议。当前 local-first 单机架构适用，未来多主机 worker 需要外部协调存储或数据库时间源。
- `lease_id` 不进入 Run 事件、Gateway Outcome、CLI 或 HTTP DTO；可观测面只包含 owner、generation、状态和时间。租约过期无需人工删锁，新 generation 可接管未完成 checkpoint。
- schema v19 将 root 注册、Supervisor 状态、Run 取消和 graph snapshot 放入同一 SQLite 写事务；不存在“Run 已终态但 root 仍 running”的已提交窗口。快照保存 pending payload 的 SHA-256 和 metadata，不复制正文；直接篡改 inbox 会在 restore 时失败。
- Coordinator inbox 仍是 Go-owned 内部原语，模型和公开 HTTP 不能发送、消费或控制 cursor。schema v25 只把已由 direct-child durable protocol 支撑的 dependency/result/failure 内容以有界只读上下文交给 root；消息身份、顺序和提交留在 Store。

- 本地 HTTP API 只接受 loopback、bodyless GET 和单一 Bearer token，不提供 CORS、Artifact 正文或 checkpoint pending input。token 不持久化；环境提供值不回显，自动生成值只输出到启动终端。
- API 尚无细粒度多用户授权；持有进程 token 等同于获得当前数据库全部已发布读取资源。写路由、WebSocket 与远程监听在单独威胁建模前保持关闭。
- OpenAPI 契约描述 Go 已公开的 17 个只读 GET 和 1 个独立控制 POST，并由 live-route 与 golden 测试阻止漂移；它不包含 Artifact 正文、checkpoint pending input、`lease_id`、fencing token 或 API-key 字段。未来 TypeScript 只能生成 DTO/client，不能把契约当成新的授权层。
- Redocly 推荐规则确认文档符合 OpenAPI 3.1。仓库所有者现已选择 Apache License 2.0，Go 生成器同步输出 `info.license.identifier: Apache-2.0`，当前校验零 warning。
- SSE cursor 只包含 version、持久化 sequence 和 Run scope digest，不含 token、正文或内部租约信息；跨 Run、未知字段、尾随 JSON、重复 header/query 和空 cursor 都在提交 stream headers 前拒绝。cursor 不是授权，所有重连仍必须携带 Bearer token。
- 每条 stream 默认受 32-event batch、2 MiB frame、10,000 events、5 分钟 lifetime 和 2 秒 write deadline 限制，进程默认最多 16 条并发 stream；超限返回稳定 `RESOURCE_EXHAUSTED`。write deadline 上限为 4 秒，低于 5 秒 server shutdown 窗口；shutdown 先取消 request BaseContext，避免慢客户端或长连接阻塞退出。

- 本轮审批审计在发布前修复两项中风险完整性问题：公开 adoption 路径原本可能为不存在的提案创建幽灵审批，策略直接拒绝记录重复保存时可能从 `never` 漂移到 `per_call`；Store 现会验证真实 ToolRun/FileEdit 及指纹，并保留原拒绝模式。
- 健壮性复核进一步修复一项低风险隐私问题：未来客户端提供的原始 review key 不再写入 SQLite，`approval_operations` 只保存域分隔 SHA-256 摘要，幂等重放和冲突检测语义保持不变。
- Approval 的 get/list/adoption 读取入口现与写入模型共享 UTF-8、256-rune identity 和 500-row 列表上限，避免未来 HTTP 控制面放大超大查询参数。
- schema v11 将审批决定与后续文件/ToolRun 状态推进设计为可恢复的两阶段提交；批准后、执行前崩溃会留下可审计的 approved 决定，重复同一键负责继续收敛。当前只允许 FileEdit 产生真实文件副作用，Shell/Script 仍 dry-run。
- 兼容 Session 若先有提案、后绑定 Run，下一次审批读取会在事务内补齐 `run_id` 并追加一次 `approval.bound`，避免历史审批永久脱离 Run 时间线。
- 本轮移除了 `script run --local` 的直接 LocalSandbox 路径；审计同时发现字符串级二次脱敏可破坏事件中的嵌套 JSON 转义，现已改为值级递归脱敏并增加 1 MiB、64 层、100,000 节点限制。
- 本轮审计修复了截断时非法 UTF-8 被误判成功、极小输出上限溢出、持久化拒绝状态在错误路径下映射不一致，以及文件工具信任调用方 workspace root 的问题；均已增加回归测试。
- Gateway 已集中现有工作区读、Shell 提案和 FileEdit 生产入口，仍复用 `tool_runs`/`file_edits` 作为兼容 Proposal 表；统一逐次 Decision 与 Session Grant 已分别由 schema v11/v12 账本承担。
- `staticcheck ./...` 当前零告警；本轮顺带清理了既存 TUI `S1008`、`S1011` 和未使用 helper `U1000`。
- `script run --local` 的执行旁路已移除；生产代码扫描中不再存在 Sandbox Runner 调用，LocalSandbox 仅保留为未接线的开发后端。
- schema v13 已消除旧的 Script Run/ToolRun 两事务窗口；故障注入证明任一 Process/Approval/Event 写入失败都会回滚整套 Mission/Session/Run/预算状态。
- schema v14 将 Artifact 行与 `artifact.created` 置于同一事务；事件写入失败不留孤立证据，终态工具已另行提交时可用同一审批幂等恢复捕获，不重复 Tool/Approval 事件。
- Artifact 哈希针对脱敏后正文；`artifact.created` 仅保存哈希、MIME、大小、stream 和 source ID，不复制输出内容。自动读取保存该次读工具实际返回的内容，不恢复读取参数已主动省略的字节。
- 文件写入已二次解析路径并重新校验哈希，但跨平台 `os.WriteFile` 无法完全消除很小的 TOCTOU 窗口。
- Windows 当前账号不能创建符号链接，真实链接逃逸测试会跳过；运行时仍会解析链接并检查工作区边界。
- 脱敏是启发式安全层，不是完整的 Secrets Manager。
- Docker Runner 还不是真实隔离边界。
- schema v16 模型工具面仍只有 create，不允许更新、完成、取消、归档、文件、Shell、进程或网络动作；这是为了保持可变状态面和安全边界可审计。
- 结构化工具 Policy 目前会保守拒绝包含 `masscan`/`nmap` 等危险动作文本的 Note，即使文字是描述用途；后续需要在不削弱永久拒绝的前提下区分“记录内容”和“执行意图”。
- 相同 operation key 的安全重放仍会消耗一次工具预算，因为预算衡量 invocation attempt；只保证业务实体和成功事件不重复。
- `run start` 只推进生命周期，`run step` 执行一个模型 turn，`run execute` 只执行操作者指定的有限步数。
- pre-call checkpoint 后崩溃可能重发模型请求，但已完成 turn 不会重复；工具响应与 pending 批次原子提交，且相同语义意图依靠 operation key 收敛。最终零工具 `model.completed` 到 turn 提交之间仍可能重发一次最终模型请求。
- 结构化记忆已有 8192-token 估算预算，但最近 Session 历史仍按 20 条消息限制，尚未纳入同一 token 预算。
- MaxCostUSD 尚未执行，因为 Provider 价格元数据缺失；工具调用预算已由 Gateway 原子记入 Run budget。
- 执行时间当前只统计 Provider 模型调用；一次 Provider 调用可能超过剩余 token，但实际用量会完整记账并阻止下一次调用。
- 预算边界停止执行后 Run 保持 `running`，需操作者显式 `finish`、`fail` 或 `cancel`；模型输出不能自行终结 Run。
- 本轮审计已修复 Provider 极端用量导致的累计整数溢出，以及超大 `--max-steps` 触发不受控预分配的问题。
- 严格协议只提供一次自动修复；若 Provider 连续两次不遵循 JSON 契约，本轮失败并等待操作者后续重试，不会无限纠错。
- 持久化 `model.delta` 故意不含文本，因此不能从 SQLite 重放逐 token 内容；未来实时 UI 必须消费 Go 控制的短生命周期订阅，并继续经过脱敏/背压边界。
- active-call 订阅只存在于当前 Go 进程，不可重放且不含文本；32 槽缓冲耗尽时通道关闭，消费者必须通过 `Dropped()` 区分慢消费断开。
- application 主动取消采用 audit-first：SQLite 无法写入取消请求时不会静默发出不可审计信号；终端父 context 取消仍会走现有 `model.failed(cancelled)` 恢复路径。
- 取消请求可能与已经返回的 Provider 响应竞争；结果中的 `Signaled=false` 明确表示请求已审计但没有再影响已结束调用。
- TUI live 状态是进程内临时视图，关闭/断线后仍必须以 SQLite Run events 为准；跨进程取消通过 schema v18 账本恢复，不把 transient registry 变成远程对象。
- `Ctrl+X` 在找不到 active registry 项时会取消当前 application request context，这是 legacy/预激活兜底，不会伪造 `model.cancel_requested`。
- `wait` 目前映射为 Run paused 和文本 reason，尚无结构化 dependency/approval 对象。
- 未绑定 Run 的 Session 仍直连 Router；这是迁移兼容路径，新功能不应继续扩展该旁路。
- slash command 仍不消耗 Supervisor turn，但 `/ls`、`/read`、`/write`、`/run` 已统一进入 Tool Gateway；未来模型工具调用仍需使用同一审批和事件语义。
- pending input 虽已脱敏并限制大小，仍属于会话内容而非 Secrets Manager；`run checkpoint` 不回显原文。
- Provider 自动重试目前只在 RunSupervisor 内启用；未绑定 Run 的 legacy Session 虽有 typed error，仍不自动重试。
- 退避当前无随机抖动；在开放远程并发 worker 前需增加 jitter，避免同一 Provider 同步重试。
- 超过 2s 的服务端 `Retry-After` 会直接返回限流状态并保留输入，需要后续操作者重试。
- 若进程在 `model.completed` 后、turn 提交前退出，恢复时可能以新的 model attempt 重发一次无副作用请求；前一次 token/耗时已经原子记账，因此不会漏算，但工具调用仍关闭。
- 已发布 migration 的语句和 checksum 不可修改，后续 schema 变化必须新增版本。
- v3 会拒绝一个 Session 关联多个 Run；若旧数据库存在重复关联，应先审计，而不是自动丢弃数据。
- 兼容期仍有普通字符串错误通过 `apperror.Normalize` 分类；新服务必须直接返回 typed error。
- Work Board 与 Notes 可由 CLI/application service、Gateway 或 RunSupervisor 的 create-only Provider 工具修改；模型更新和归档仍未开放。
- Supervisor 的 Work Board 是每次调用前最多 20 项、16 KiB 的候选快照，并作为一个 Context Section 参与 token 选择；超出项保留在 SQLite，后续仍需相关性查询或显式加载协议。
- 显式操作者 `run finish` 可以覆盖仍有活跃 WorkItem 的模型完成门禁；该命令是人工终结边界，报告层后续应明确记录未完成项。
- WorkItem ID 在全库唯一、依赖在同 Run 内；AgentNode 身份表现已存在，但 Owner 仍是受长度约束的兼容标签，等待 Specialist admission 后迁移为 Agent 外键。
- Note Owner 同样还是受长度约束的身份标签；`root` 是当前 Supervisor 的保留查看者名称，下一阶段再绑定稳定 AgentNode ID。
- Context Builder 的 8192 token 是启发式估算预算，只覆盖摘要/Work Board/Notes；最近 20 条 Session 消息仍使用既有数量上限，Provider usage 才是最终计费依据。
- Supervisor 最多查询 100 条当前可见 active Notes；更多记录保留在 SQLite，但本轮不会进入候选集，后续需要查询相关性或显式加载协议。
- Evidence ID 当前是结构化引用而不是外键，因为 Evidence 实体尚未落地；P6 报告阶段必须补引用完整性和失效投影。
- 模型可读取选中的 Note，并通过 `note_create` 安全新增 metadata-tracked Note；更新和归档仍只属于操作者命令。

## 六、验证基线

每个开发切片至少执行：

```powershell
go test -count=1 ./...
go vet ./...
```

共享状态、并发或存储变更还要运行相关包的 `go test -race`。CLI 行为变更要在隔离的 `CYBERAGENT_HOME` 中完成 smoke test。提交前扫描凭据前缀，确认本地数据库、工作区、环境文件和 API key 未进入 Git。

最新验证在 active-call 基线上新增 Work Board 覆盖：领域状态机、migration v9、旧库升级、复合外键、依赖环/缺失/未完成门禁、事件回滚、并发版本胜者唯一性、CLI 全生命周期、终态不可变、脱敏上下文、16 KiB 上限、终态项排除，以及活跃任务下模型 `finish` 的协议修复。

本机 Go 已从 1.26.1 升级到 1.26.5；升级前 `govulncheck` 命中 9 条可达标准库漏洞，升级后复扫为 0。协议修复 transport 测试验证全局 attempt `1/2/3` 与阶段内 transport `1/1/2`，Store 也会拒绝与 durable started event 不一致的终态元数据。

本轮最终发布门已通过：`go test -count=1 ./...`、`go vet ./...`、全仓库 `go test -race -count=1 ./...`、`staticcheck ./...` 和 `govulncheck ./...`；后者报告 0 条可达漏洞。仓库扫描返回 `NO_CREDENTIAL_PATTERN_IN_REPO` 与 `NO_RUNTIME_OR_SECRET_ARTIFACTS_IN_REPO`。

Notes/Context Builder 切片新增验证：schema v9->v10 数据保留、分类/可见性/Owner、关系复合外键、标签联合过滤、事件原子回滚、并发版本、content-file 大小与 UTF-8、CLI archive/restore、root 可见 Note 选择、token 预算优先级、敏感文本隔离，以及 `model.started` 仅保存来源元数据。隔离 smoke 核对 1 条 `note.created`、3 条 `note.changed` 和最终 version 4。

发布前人工审计额外修复了三项低风险一致性问题：所有 Note 文本/关系字段现在显式拒绝非法 UTF-8；Store 拒绝负数列表上限；`note.changed` 的 `changed_fields` 只记录规范化后真正变化的字段。三项均有回归测试。

DeepSeek Provider 切片新增 `DEEPSEEK_API_KEY`/`BASE_URL`/`MODEL` 独立注册和无 Key 不注册测试。真实 `deepseek-v4-flash` smoke 同时通过普通 Messages 请求与 RunSupervisor SSE 路径，并产生不含文本的 `model.delta` 持久化进度。

该增量最终门通过 `go test -count=1 ./...`、`go vet ./...`、`go test -race -count=1 ./internal/app ./internal/llm`、`staticcheck ./...` 与 `govulncheck ./...`；可达漏洞为 0，真实 DeepSeek Key 与其他凭据模式未进入仓库。

Tool Gateway 第一纵向切片新增领域不变量、精确参数 schema、automatic/per-call/never 决策、可信 workspace root 绑定、共享 ToolRun/FileEdit review service、UTF-8/MIME/截断/脱敏结果，以及 CLI/Session/TUI 兼容迁移。测试覆盖正常读取、密钥脱敏、输出上限、合法多字节字符跨界、非法 UTF-8 位于截断点、路径伪造、危险 Shell/FileEdit 拒绝、dry-run 审批、文件落盘和适配器兼容。

本轮最终发布门通过 `go test -count=1 ./...`、`go vet ./...`、全仓库 `go test -race -count=1 ./...`、`staticcheck ./...` 与 `govulncheck ./...`；可达漏洞为 0。隔离 CLI smoke 验证 workspace read、Shell dry-run、FileEdit approve 和危险 `masscan 0.0.0.0/0` 提案拒绝，未执行危险命令。仓库扫描返回 `NO_CREDENTIAL_PATTERN_IN_REPO` 与 `NO_RUNTIME_OR_SECRET_ARTIFACTS_IN_REPO`。

Script Gateway 切片新增 workspace/path 前置校验、`script_process.v1` schema、executable/argv/backend/disabled-mode 上限、自动 Script Profile Run、Policy/Tool event 投影，以及 `--local` 零副作用测试。含 token-shaped 参数的 JSON 在 ToolRun 与 Run Event 中均脱敏且保持可解析；危险参数持久化 `tool.denied`。

该切片最终门再次通过 `go test -count=1 ./...`、`go vet ./...`、全仓库 `go test -race -count=1 ./...`、`staticcheck ./...` 和 `govulncheck ./...`，可达漏洞为 0。临时构建的真实 CLI 二进制 smoke 返回危险请求退出码 5，审批前后均未创建脚本标记文件；扫描返回 `NO_PRODUCTION_SANDBOX_RUNNER_CALLS`、`NO_CREDENTIAL_PATTERN_IN_REPO` 和 `NO_RUNTIME_OR_SECRET_ARTIFACTS_IN_REPO`。

Durable Approval 切片新增 schema v10->v11 原地升级、Run/Session 绑定与后绑定 adoption、请求指纹、不可变 operation-key 摘要、同键重放/异意图冲突、无批准状态门禁、幽灵提案与指纹篡改拒绝、策略 `never` 幂等保存、Shell/FileEdit 重复审批、并发 approve/deny 单胜者、事件失败事务回滚、读取边界，以及批准后跨 Store 重启继续收敛测试。TUI 测试也改为只通过 Gateway 审批，不再直调 legacy Manager。

该切片发布门通过 `go test -count=1 ./...`、`go vet ./...`、全仓库 `go test -race -count=1 ./...`、`staticcheck ./...` 和 `govulncheck ./...`；可达漏洞为 0。隔离 CLI smoke 验证 pending/approved 查询、审批详情、dry-run 完成和 `approval.requested/decided`。扫描返回 `NO_CREDENTIAL_PATTERN_IN_REPO`、`NO_PRODUCTION_APPROVAL_MANAGER_BYPASS`、`NO_PRODUCTION_SANDBOX_RUNNER_IMPORTS` 与 `NO_RUNTIME_OR_SECRET_ARTIFACTS_IN_REPO`。

Session Grant/Tool Budget 切片新增 schema v11->v12 原地升级、Grant 创建/复用/撤销幂等操作、精确五维 scope、终态 Run/归档 Session 门禁、`tool_approvals.grant_id` 审计关联、Policy 永久拒绝优先、自动 FileEdit 应用与 Shell dry-run，以及 `approval grant`/`run usage` CLI。工具预算测试覆盖有序计数、零值兼容、跨作用域拒绝、并发饱和不超支和一次性 `tool.budget_exhausted`。

本轮健壮性审计未发现高严重度问题，并补齐一项低风险可观测性缺口：预算首次拒绝现在会持久化唯一耗尽事件，后续拒绝不会刷事件。最终门通过 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`staticcheck ./...` 与 `govulncheck ./...`，可达漏洞为 0。隔离真实二进制 smoke 验证 Grant 自动授权、撤销后恢复逐次审批、危险 `masscan` 仍永久拒绝和预算计数到达上限；扫描返回零凭据模式、零运行产物和零生产 Sandbox 调用。

Typed ScriptProcess 切片新增 schema v13、独立 `internal/scriptprocess` 状态机、Gateway 可选能力边界、原子 Run/Process 创建、SHA-256 operation-key 摘要、通用 Tool CLI 分派，以及 dry-run-only 审批。测试覆盖 12 路并发幂等收敛、异意图冲突、单 Run 多 Process、Run/Session 绑定伪造、审批台账绕过、事件失败全事务回滚、v12 Run/Grant 原地升级、危险参数永久拒绝、密钥脱敏与零宿主机副作用。

本轮代码审计修复三项一致性问题：`ReviewRequest` 的规范化层漏放行 ScriptProcess；Session Store 被无关脚本原子接口污染；v13 初稿误把 `run_id` 设为唯一并缺少 Process Run/Session 复合绑定校验。修复后脚本能力作为 Gateway 可选接口接入，一个 Run 可拥有多个 Process，伪造跨 Run 绑定会整笔回滚。

该切片最终门通过 `go test -count=1 ./...`、全仓库 `go test -race -count=1 ./...`、`go vet ./...`、`staticcheck ./...` 与 `govulncheck ./...`；可达漏洞为 0。隔离真实二进制 smoke 验证同键重放、异意图退出码 4、危险参数退出码 5、一次预算扣减、typed show/list/approval、密钥脱敏和零标记文件。扫描返回 `NO_USER_TEST_KEYS_IN_REPO`、`NO_CREDENTIAL_PATTERN_IN_REPO`、`NO_TRACKED_RUNTIME_OR_SECRET_ARTIFACTS` 与 `NO_PRODUCTION_SANDBOX_RUNNER_CALLS`。

Run Artifact 切片新增 schema v14、`internal/artifact` 领域边界、SQLite 原子捕获、Gateway 截断前投影、自动读取 invocation 来源、Artifact CLI 与 Tool CLI 反向链接。测试覆盖 4 MiB/UTF-8/MIME 边界、密钥二次脱敏、Result 截断前完整捕获、跨 Run 来源拒绝、事件正文隔离、哈希篡改检测、捕获故障恢复、重放幂等、v13->v14 保留升级和 Policy 拒绝零 Artifact。

本轮审计未发现高严重度问题，并修复两项低风险健壮性问题：Go error 文案首字母不符合约定，以及自定义 Store 返回非法 UTF-8 终态输出时可使 Artifact 捕获失败。Gateway 现在会先规范 UTF-8，再脱敏、哈希和截断。发布门通过全仓库 `go test -race -count=1 ./...`、`go vet ./...`、`staticcheck ./...` 与 `govulncheck ./...`，可达漏洞为 0；真实二进制 smoke 验证单次工具预算、唯一 `artifact.created`、稳定 Artifact ID 和无密钥泄露。

Structured Memory Tool 切片新增 schema v15、`internal/runmutation`、`run_memory` Gateway action class、strict Provider-ready schemas、WorkItem/Note create executor、摘要幂等账本与 `tool schema/invoke` CLI。Store 测试覆盖同键重放、异意图冲突、事件故障全回滚、v14->v15 数据保留、8 路跨 Store 并发收敛、精确 Run/Session/Workspace/invocation 绑定、密钥脱敏和事件正文隔离。Gateway 测试证明非法 JSON、未知字段、非法枚举与伪密钥依赖在预算前拒绝且错误不回显密钥；Policy 拒绝不会调用 executor。

本轮人工健壮性审计修复三项问题：依赖 ID 初稿格式过宽，可能把伪密钥形态的缺失依赖回显；严格 JSON/枚举错误可能包含敏感字段值；跨进程 SQLite deferred 读后写事务可能直接返回 `database is locked`。现已收紧到真实 `work-时间戳[-随机值]` 格式、对解析错误统一脱敏，并通过 `_txlock=immediate` 配合已有 busy timeout 串行化写事务；只读 Grant 查询不再开启写锁事务。

该切片发布门通过 `go test -count=1 ./...`、全仓库 `go test -race -count=1 ./...`、`go vet ./...`、`staticcheck ./...` 与 `govulncheck ./...`；可达漏洞为 0。两项跨 Store 并发测试连续 10 轮通过。真实二进制 smoke 验证 WorkItem 创建/重放、异意图退出码 4、Note 脱敏、危险内容退出码 5、`tool_calls: 5`、唯一领域/完成事件，以及原始 key/伪密钥不进入事件流；临时运行目录已清理。

Supervisor Tool Loop 切片新增 schema v16、持久化 tool round/call、Anthropic-compatible 工具协议、final-stream ToolCall 聚合、RunSupervisor 白名单循环、metadata-only 结果和 CLI 轮次计数。单批最多 4 调用、单 turn 最多 4 轮；模型事件与批次原子提交，pending 结果可跨 Store 重启恢复。语义键不采用 Provider 临时 ID，同一意图跨失败 attempt、不同 Provider ID 和后续轮次只创建一个实体。

本轮审计未发现高严重度问题，并修复四项健壮性问题：应用层与 Store 的 JSON 字段顺序不一致、跨轮次本地 call ID 冲突、并发恢复时 `replayed` 元数据使结果不稳定，以及协议修复仍向 Provider 暴露工具。Store 现在独立复核严格 typed payload，结果去除竞争时序字段，repair 请求工具列表为空。发布门通过 uncached 全仓测试、全仓 `-race`、`go vet`、`staticcheck` 和 `govulncheck`，可达漏洞为 0；双 SQLite 连接并发结果只产生一个 result/round-complete 事件。隔离真实二进制 mock smoke 导出两个 schema，并完成 `tool_rounds: 0`/`tool_calls: 0` 的 Run turn；凭据扫描仅命中脱敏单元测试的合成 fixture，未发现用户测试 key。

Local Read API 切片新增 `internal/httpapi`、`cyberagent api serve`、Store 级 offset/page 查询和 metadata-only Artifact lookup。`api.v1` 覆盖 Run/Mission/checkpoint/tool usage、Session/messages、Run events、WorkItems、Notes、Artifacts 和历史 Supervisor ToolRounds；列表使用 route/filter-bound opaque cursor，每页最多 100，100,000 行窗口到达边界时显式标记 `truncated`。

本轮代码与安全审计未发现高严重度问题，并修复三项低风险健壮性问题：Windows 上预取消 context 仍可能完成 bind、环境 token 被静默 trim 且校验错误误报为内部错误，以及游标窗口边界可能返回下一次必然无效的 cursor。现已在 listen 前检查 context、按原字节执行 typed token 校验且不回显环境 token，并以 `page.truncated` 表示硬边界。真实 SQLite 集成测试覆盖所有资源族、分页、父资源 404、脱敏、Artifact 正文隔离、checkpoint input 隔离、Host/remote/token/method/body 防线、内部错误隐藏、32 路并发读取、优雅关闭和 CLI token 零持久化。

该切片发布门通过 `go test -count=1 ./...`、全仓库 `go test -race -count=1 ./...`、`go vet ./...`、`staticcheck ./...` 与 `govulncheck ./...`；可达漏洞为 0。真实二进制隔离 smoke 验证 `v0.1.0`、`api.v1`、schema v16、正确 token 200、错误 token 401、POST 405、无 CORS、环境 token 不回显且关闭进程后未出现在 SQLite；临时进程和运行目录已清理。

Run-aware TUI 切片新增当前 Run 头部、Tools/Work/Notes/Rounds 四视图、`a` 逐次批准、`g` 本会话 Grant、活动授权提示和终端单元格感知的 Unicode 布局。会话授权由 Go Tool Gateway 校验持久化审批指纹与精确作用域，在创建 Grant 前重新执行当前 Policy；已授权但尚未推进的提案可恢复，Grant 永远不能覆盖永久拒绝。

本轮审计未发现未解决的高/中风险问题，并修复四项健壮性/作用域缺口：异常审批记录可能先留下活动 Grant、TUI 手工输入其他 Session 的 ToolRun ID 时键盘与 slash command 的作用域语义不一致、状态栏无法区分四个活动视图、默认窄侧栏把标题与标签挤在同一行。适配器现要求调用方声明期望 Session，TUI 的批准一次、本会话授权和拒绝都先验证当前 Session。真实 SQLite 测试覆盖跨 Session 三类操作拒绝且零状态变化、Grant 关联、撤销后恢复、后续安全 Shell 自动 dry-run、危险命令永久拒绝、Policy 拒绝时零 Grant、pending ToolRound、中文 WorkItem/Note 和 `g` 键异步状态。发布门通过 `go test -count=1 ./...`、全仓库 `go test -race -count=1 ./...`、`go vet ./...`、`staticcheck ./...` 与 `govulncheck ./...`；可达漏洞为 0。

Run Execution Lease 切片新增 schema v17、`RunExecutionLease` 领域对象、SQLite acquisition/renew/release/takeover、RunSupervisor 心跳和 fencing token 贯穿。审计修复了六个并发/安全问题：结构化实体虽被 fence 但预算可能先扣、持久化幂等记录误要求瞬时租约、同 owner 隐式重入可并发共享租约、takeover update 未检查影响行数、lease token 可能经事件/Outcome/API 泄露，以及租约必需校验一度误放在会验证无 token 安全返回值的通用 `ToolCall.Validate`。入站 Gateway 现在负责租约门禁，Outcome 继续安全清空 token。现有测试覆盖双连接八路竞争唯一胜者、显式 acquisition replay、过期 generation 接管、旧 checkpoint/续租/释放拒绝、v16 pending checkpoint 原地升级、长模型调用跨原 TTL 心跳、两 turn 共用一份 Execute 租约，以及 stale structured call 零预算/零实体/零工具事件。发布门通过全仓 uncached 测试、全仓 `-race`、`go vet`、`staticcheck` 和 `govulncheck`，可达漏洞为 0。

OpenAPI Contract 初始切片新增 `internal/httpapi/openapi.go`、`cyberagent api openapi`、鉴权后的原始 `/api/v1/openapi.json` 与 `docs/openapi.json`。响应 schema 从 Go DTO 的 JSON tag 反射生成，路径、filter、分页、枚举、Bearer security scheme 和标准错误响应由 Go 路由目录定义。当时契约为严格只读的 16 个 GET path；schema v18 后续按同一生成器增加独立 control operation。CLI 导出不打开 SQLite、不读取 token。测试覆盖字节级确定性、golden 防漂移、全部路由真实 SQLite 命中、鉴权/query 拒绝、media type、operation ID 唯一性，以及敏感内部字段排除。

该切片发布门通过 `go test -count=1 ./...`、全仓库 `go test -race -count=1 ./...`、`go vet ./...`、零告警 `staticcheck ./...` 与 `govulncheck ./...`，可达漏洞为 0。初始 Redocly license advisory 已由所有者后续选择的 Apache-2.0 元数据解决。隔离真实二进制 smoke 导出当时的 73,354-byte、16-path、23-schema 契约，确认未创建 `CYBERAGENT_HOME`；凭据、运行数据和 OpenAPI 内部字段扫描均为零命中，临时目录已安全清理。

Run Event Stream 切片新增 sequence-based Store 查询、`run-events.v1` SSE envelope、Run-bound opaque cursor、`Last-Event-ID` 恢复、heartbeat、逐帧 write deadline、每连接事件/时间边界、进程连接槽位和 Server BaseContext shutdown cancellation。HTTP/OpenAPI contract 现包含 17 paths 与 24 schemas。测试覆盖前三页精确续传、跨 Run/损坏/重复 cursor 拒绝、heartbeat 零持久化、第二 SQLite 连接追加可见、并发槽超限与释放、慢 writer deadline，以及一分钟 stream 在 server shutdown 时立即结束。

该切片发布门通过 `go test -count=1 ./...`、全仓库 `go test -race -count=1 ./...`、`go vet ./...`、零告警 `staticcheck ./...` 与 `govulncheck ./...`，可达漏洞为 0。Redocly 接受当时的 17-path/24-schema 契约；原 license warning 已在 Apache-2.0 选择后消除。隔离真实二进制在 loopback Bearer 边界推送 2 个持久化 frame，curl 的 1 秒超时按预期断开；stream 不含 token、`lease_id` 或 pending input，SQLite 未持久化 API token。凭据、运行数据和 OpenAPI 内部字段扫描均为零命中，临时进程与目录已清理。

Cross-Process Model Cancellation 切片新增 schema v18、`run_model_cancellations`、一对一摘要幂等操作账本、`model.cancel_observed`、Supervisor 100 ms polling，以及 `/api/v1/runs/{run_id}/active-call/cancel`。read/control token 完全分离且不能相同；控制默认关闭。API 只接受 4 KiB 内的单个严格 JSON 对象和 16-256 字节 operation key，精确校验 Run、Supervisor attempt、最新 model attempt 与活动 execution lease，但客户端永远不能提交 fencing token。

本轮审计未发现未解决的高/中风险问题，并在完成前修复五个竞态/健壮性缺口：旧 `model.started` 可能被误认为当前调用、崩溃遗留请求可能永久 pending、内部 requester 可能携带敏感材料、多个 key 可为同一目标无限制造 operation alias，以及持久化时间顺序缺少领域复核。现在后续 attempt 会原子解析旧请求为 `superseded`，目标与 operation key 一对一，Store 拒绝敏感 requester，观察动作先通过 lease-fenced 事务再 signal Provider context，模型终态与 cancellation `resolved` 原子提交。

测试覆盖 HTTP 到阻塞 Provider 的双 SQLite 连接端到端取消、read/control capability 互斥、默认关闭、202 原键重放、409 变更意图/换键、stale lease/latest-attempt 拒绝、secret redaction、原始 key 零持久化、严格 body/content-type/大小边界和 OpenAPI live route。发布门通过全仓测试、全仓 `-race`、`go vet`、零告警 `staticcheck` 与 `govulncheck`，可达漏洞为 0。OpenAPI 当前为 18 paths、26 schemas、2 security schemes，并带 Apache-2.0 metadata；Redocly 零 warning。隔离真实二进制报告 schema v18，read health 为 200，read-token POST 与 control-token GET 均为 401，授权取消缺失 Run 为 404；输出与关闭后的 SQLite 均无两个测试 token，临时进程和目录已清理。

Single-Root Agent Coordinator 切片新增 schema v19、`AgentNode`/inbox/`agent_graph.v1` 领域对象、`internal/coordinator` 服务、`agent_nodes`、`agent_messages`、`agent_graph_snapshots` 和 `run graph`。新 Run 在创建事务中获得一个稳定 root；v18 旧库不做不可审计的批量身份猜测，而是在下一次 register/Supervisor 操作中惰性补建。Supervisor begin、continue、wait、finish、failure/finalize 以及普通 Run pause/resume/cancel 都与 Agent 状态和快照同事务提交。

本轮审计未发现未解决的高/中风险问题。实现阶段补强了五项健壮性边界：Coordinator 写入口先获取 SQLite writer reservation，避免并发 register/inbox sequence 的 deferred read-to-write 竞争；快照不复制 inbox payload，而是保存 SHA-256、消息 ID、sequence、kind 与时间，restore 会拒绝正文篡改；历史快照每 Run 只保留最新 32 份，inbox pending/总历史和消费批次都有数据库硬上限；register 不再在 graph 校验前修正已有 root，状态漂移会失败而不是被新快照覆盖；inbox JSON key 现在限制为 64-byte ASCII 协议字段并拒绝密钥形态，补上通用 value-redaction 不处理 map key 的边界。测试覆盖 concurrent registration、重启恢复、wait/resume 身份连续、cancel 级联、child insert hard denial、key/value secret handling、exactly-once consume、v18 原地升级、tamper detection 与 snapshot retention。最终 race 复核还修正了三项既有测试时序假设：Provider 总 deadline 不再被误当成全部模型执行时间，mid-stream cancellation 会等待首个 delta 持久化后再主动取消，API CLI 多行启动输出会等待最后一项元数据后再断言。发布门通过全仓测试、全仓 `-race`、`go vet`、零告警 `staticcheck` 与 `govulncheck`，可达漏洞为 0；隔离 CLI smoke 创建 Run 并由 `run graph` 恢复一个 ready root。当前 root `child_limit=0`，无 child spawn、无模型 inbox 注入、无新 Shell 权限。

Idempotent Agent Inbox Protocol 切片新增 schema v20、`agent_message_operations` 摘要账本和 `message`/`wake`/`dependency` 语义。Store 在同一 SQLite writer transaction 内完成重放判定、消息、操作事实、审计事件、可选 Specialist 状态转换和图快照；判断重放发生在当前收件状态检查之前，因此成功 wake 后的网络重试仍返回原消息。快照只为非普通语义增加字段，v19 普通消息快照保持字节兼容。

本轮代码与安全审计未发现未解决的高/中风险问题。原始幂等键不进入数据库、事件、快照或错误；指纹排除随机 message ID/时间并基于脱敏规范化 payload；同键异意图稳定冲突。严格 payload decoder 拒绝未知字段、非法 dependency 状态、wake/kind 错配、senderless dependency 和 root wake。测试覆盖正常重放、冲突、wake exactly-once、事件/快照零重复、原键零泄漏、v19->v20 数据/快照兼容和旧迁移夹具。发布门通过 `go test -count=1 ./...`、全仓 `go test -race -count=1 ./...`、`go vet ./...`、零告警 `staticcheck ./...` 与 `govulncheck ./...`；可达漏洞为 0。Specialist admission、child model execution、模型 inbox 注入和任何新 Shell/网络权限仍未开放。

Bounded Specialist Admission 切片新增 schema v21、`SpecialistAdmissionPolicy`、`agent_admission_operations` 摘要账本、独立 child Session 与原子 root budget reservation。默认 `coordinator.New` 不具备 admission capability；显式内部构造器仍受最多两个 child、深度 1、父 Skills 子集、每 child policy 上限和 root 保留额度约束。预留后的有效 Run budget 从 `BeginSupervisorTurn` 返回给 root 模型请求，Agent graph 同步展示相同 root 上限。

本轮代码与安全审计未发现未解决的高/中风险问题。Store 在一个 writer transaction 内完成重放、容量/预算/Skill 复核、root version fencing、Session、child、摘要 operation、两类事件与快照；强制事件失败测试证明全部回滚。两条 SQLite 连接并发使用同一意图只创建一个 child/Session/operation。pause/resume 使用明确 status reason，避免把依赖等待误唤醒；Run 终态级联 child 并归档 Session。全仓普通测试、全仓 race、`go vet`、零告警 `staticcheck` 与 `govulncheck` 均通过，可达漏洞为 0。该 v21 切片结束时 CompletionReport 尚未实现，后续已由 schema v23 落地；child 模型循环、公开 API/CLI spawn 和新工具权限仍关闭。

Agent-Owned Work Memory 切片新增 schema v22，在 WorkItem/Note 上增加 nullable `owner_agent_id`、同 Run 外键触发器和索引。应用与 Store 同时验证真实 Agent、Run 归属与非终态新分配；旧标签、旧行和 v21 数据不被重写。owner-only Agent Note 在缺少旧标签时镜像 Agent ID，以兼容既有 v10 CHECK 与旧客户端，同时 `ViewerAgentID` 按 root/Specialist 身份执行真正的私有可见性。CLI 增加 `--owner-agent`，TUI 显示 Agent owner，HTTP/OpenAPI 增加字段与过滤器。

本轮代码审计未发现未解决的高/中风险问题，并在定向测试中修复一项旧 schema 兼容缺口：Agent-only owner Note 原本会被 v10 的 owner-label CHECK 拒绝，现通过确定性兼容标签收敛，无需高风险重建 Notes 表。Supervisor structured-memory scope 强制携带合法 Agent 与 execution lease，策略/工具事件记录 Agent ID；模型 JSON schema 明确拒绝 `owner_agent_id`，避免模型伪造控制面身份。测试覆盖同 Run root/Specialist visibility、跨 Run Store/trigger 拒绝、Agent 重新分配、可见性变化不丢所有权、v21->v22 数据保留、CLI/HTTP 过滤和 Supervisor 自动绑定。发布门通过全仓 uncached tests、全仓 `-race`、`go vet`、零告警 `staticcheck` 和 `govulncheck`，可达漏洞为 0；隔离二进制 smoke 验证 v22 Run/root、Agent-owned todo/private Note 过滤和包含 `owner_agent_id` 的 OpenAPI，并清理全部临时数据。

Specialist CompletionReport 切片新增 schema v23、严格 `agent_completion.v1`、`agent_completion_reports`、摘要型 `agent_completion_operations` 和内部 Coordinator `FinishSpecialist`。协议必须显式携带版本，只允许 `succeeded`/`partial`，摘要限制为 4096 rune/8 KiB，WorkItem 与 Note 引用分别最多 16 项。Store 只接受 running Specialist 的精确 active attempt 和直接 root parent；成功报告不能留下活跃 child WorkItem，partial 必须交接全部活跃项，Note 必须由 child 拥有、处于 active 且对 parent 可见。

本轮代码与安全审计未发现未解决的高/中风险问题。Store 先验证原始 summary 上限再脱敏并复验，避免超大敏感字符串脱敏缩短后绕过；报告、父 result inbox、child terminal、Session archive、摘要 operation、三类元数据事件和图快照在一个 writer transaction 内提交。SQLite trigger 拒绝修改已提交报告。强制事件失败测试证明零残留，两条 SQLite 连接并发完成只生成一份报告/消息/operation；同键异意图冲突，不同键旧 attempt 失败，原始 key 与原始敏感摘要不落库。restore 会拒绝 completed child 缺失报告的篡改状态。发布门通过全仓 uncached tests、全仓 `-race`、`go vet`、零告警 `staticcheck` 与 `govulncheck`，可达漏洞为 0；仓库凭证模式和运行产物扫描为零，隔离 CLI smoke 创建 schema v23 Run 后清理临时目录。公开/model finish 与 spawn、child model loop、child usage scheduler 和真实 Shell/网络权限仍关闭。

Specialist Attempt Runtime 切片新增 schema v24、`agent_attempt.v1`、`agent_attempts`、摘要型 `agent_attempt_mutations` 和默认关闭的 Coordinator runtime capability。每次调度绑定当前 Run execution lease/generation 并立即计入一个 turn；模型 usage 只允许记录一次并累计到 child token 预算。Attempt 可收敛为 `continued`、`finished`、`crashed` 或 `interrupted`，终态不可修改。CompletionReport 现在必须绑定当前租约且已记录 usage 的 Attempt。崩溃原因在 Store 边界脱敏后写入父 notification，预算耗尽时 child 失败并归档 Session；新 lease generation 会恰好一次回收旧 worker 遗留的 running Attempt。

本轮代码与安全审计未发现未解决的高/中风险问题，并修复两项防御性缺口：终态 Attempt 的无变化锁定 UPDATE 会被不可变触发器拒绝，导致成功操作无法重放，锁定点现移到所属 Agent 行；v24 初稿只在 Go Store 校验 lease，现已把 Attempt 创建、usage 和 CompletionReport 的 active lease/generation/expiry 条件下沉到 SQLite trigger，直接写库也不能伪造或复用过期租约。Store 在同一 writer transaction 内提交 Attempt、预算、child、Session、父消息、摘要 operation、元数据事件与图快照。Run pause/wait/terminal 先中断 Attempt 再移动 child，restore 会校验尝试序号、累计 token、active projection 和 CompletionReport。测试覆盖 usage/continue/crash 重放、预算耗尽、强制事件失败零残留、双 Store 并发唯一调度、伪造租约直接写入拒绝、过期租约接管、旧 worker fencing、恰好一次恢复、暂停/恢复新 Attempt 和 v23 原地升级。发布门通过全仓 uncached tests、全仓 `-race`、`go vet`、零告警 `staticcheck` 与 `govulncheck`，可达漏洞为 0；生产代码凭据模式与受跟踪运行产物扫描为零。隔离真实二进制 smoke 完成 version、mock Provider、工作区初始化、schema v24 数据库打开、Run 创建/列举，并安全清理临时运行目录。公开 CLI/HTTP/model spawn、child 模型循环、root inbox context 和真实 Shell/网络权限仍关闭。

Root Inbox Context 切片新增 schema v25、严格的 completion/failure inbox payload decoder、`root_inbox_deliveries` 和 `agent.inbox_context_prepared/committed/superseded` 事件。Go 只选择 direct Specialist child 发出的 dependency、与 immutable CompletionReport 对应的 result、与 crashed Attempt 对应的 failure notification；每轮按持久化 sequence 最多绑定 4 条。`prepared` 批次绑定精确 Supervisor attempt 与 turn，成功 lifecycle 事务先提交 delivery 再消费消息，失败或 Run 状态离开 running 时 supersede 且消息继续 pending；取消、进程重启和 lease generation takeover 保留同一 attempt 与同一批次。

本轮代码与安全审计未发现未解决的高/中风险问题。模型收到的是有界、脱敏、截断的 `root_inbox_context.v1` 类型化 JSON，且系统提示明确把 payload 当作不可信任务状态；消息 ID、sequence、cursor 和消费控制不进入 prompt，sender 由 Store 路由关系确定。手工 consume 不能抢占 running root，SQLite trigger 拒绝未由 CompletionReport/crashed Attempt 支撑的 result/failure delivery，图快照包含 prepared delivery 元数据并保持 v24 兼容。测试覆盖三种协议、顺序与 4 条上限、伪造字段、并发双 Store prepare、事件失败回滚、原子 completion 回滚、失败 supersede、取消后进程重启重放、lease takeover/stale fencing、exactly-once commit、v24 原地升级和 prompt 凭据脱敏。发布门通过全仓 uncached tests、全仓 `-race`、`go vet`、零告警 `staticcheck` 与 `govulncheck`，当前代码可达漏洞为 0。隔离真实二进制只注册 mock Provider，初始化工作区，创建 schema v25 review Run，并由 `run graph` 恢复 ready root 后清理临时目录。公开 CLI/HTTP/model spawn、child Provider loop 和真实 Shell/网络权限仍关闭。

schema v25 完成后的独立全项目审计覆盖依赖图、所有 Go 包、race、静态分析、SQLite、LLM 网络出口、HTTP loopback 入口、Shell/Sandbox、工作区与审批文件写入、凭据/运行产物，以及真实 CLI/TUI/OpenAPI 链路。审计发现并修复四类健壮性问题：Bubble Tea 间接引入的 `x/sys v0.38.0` 含不可达但已知的 Windows 包级漏洞，已定点升级到无告警的 `v0.44.0`；未接入生产路径的 LocalRunner 仍具备宿主机 `exec` 实现，现改为明确 fail-closed，Noop/Docker 同时补上取消处理且 Noop 显示会脱敏；Anthropic-compatible Provider 现只接受 HTTPS 或 exact-loopback HTTP，拒绝 URL credentials/query/fragment、异常 API key 与全部重定向，防止 `x-api-key` 被转发；新 Unix runtime 目录和 SQLite 文件权限分别收紧为 `0700`/`0600`。

新增测试把 Sandbox statement coverage 从 13.2% 提升到 72.0%，`toolbudget` 从 0% 提升到 100%，并覆盖 Provider 非安全配置、跨域重定向零触达、预取消 Runner 和 Unix 私有权限。最终全仓 uncached tests、全仓 `-race`、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff` 与 `govulncheck` 全部通过，漏洞为 0。完整隔离二进制 smoke 覆盖 version/provider/model、workspace read/tree、script `--local` disabled proposal、learn、CTF 骨架、WorkItem/Note、Run step/checkpoint/events/usage/graph/lease/pause/resume/cancel、Run-bound Session send、TUI snapshot 和 OpenAPI 导出，并清理临时 runtime。残余风险仍是：Policy/Prompt 注入防御为规则型骨架，文件审批无法消除拥有同一主机权限的外部进程在最终写入瞬间制造 TOCTOU，真实 Sandbox、自主/并发 child model loop、金额预算和 TypeScript/Rust 层尚未实现。

Internal No-Tool Specialist Turn 切片新增 schema v26、严格 `specialist_lifecycle.v1`、`specialist_model_calls` 和显式 Go-internal `SpecialistRunner`。模型只可提议 `continue` 或携带 `agent_completion.v1` 的 `finish`，不能提交 usage、Agent/Attempt 身份、lease、重试、Policy 结论或工具调用。Store 将模型终态、token/执行时间、Policy 事件、允许输出的脱敏 child Session 消息和 Attempt usage 在同一 writer transaction 提交；invalid response 也会先记 usage 再崩溃，transport retry 不重复扣 token。RunSupervisor 与 SpecialistRunner 现共用同一 execution-lease 心跳/fencing helper。

本轮健壮性审计未发现未解决的高/中风险问题。实现过程中主动修正了四项边界：child Session 不再整表加载，而由 SQLite 只取最近 12 条；进入 Provider 前历史正文再受 64 KiB 聚合上限约束；完成调用保存脱敏 input/action 指纹，异意图终态重放返回冲突；SQLite trigger 强制模型 attempt 连续，并让 `started -> terminal` 直接写入再次校验 active lease 与 Attempt usage 一致性。测试覆盖 continue、CompletionReport finish、预算耗尽终止、retry 后单次计费、严格 JSON、模型工具调用拒绝、危险 `masscan 0.0.0.0/0` Policy 拒绝、context cancellation、lease release、过期 worker takeover、Session 持久化、终态不可变、异意图重放、跳号/伪造 usage/过期 lease 直接写库拒绝和 v25->v26 升级；没有新增 CLI/HTTP/OpenAPI 路由或 Shell/网络权限。全仓普通测试、`-race`、`go vet`、`staticcheck`、`go mod verify/tidy -diff`、凭据模式扫描与 `govulncheck` 均通过，当前可达漏洞为 0；隔离真实二进制只列出 mock Provider，初始化工作区，打开 schema v26，创建/列举 review Run 后删除临时 runtime。

Recoverable Specialist Child Context 切片新增 schema v27、严格 `specialist_instruction.v1`、`specialist_context_deliveries` 和 4096-token/32 KiB bounded context builder。Store 只准备直属 root -> child 的 instruction/message；WorkItem/Note 只按 child `owner_agent_id`、active 状态和 child 可见性读取。消息 ID 仅作为 `model.started` provenance，不进入 prompt；WorkItem/Note ID 保留给 CompletionReport 引用。`continue` 与 `finish` 在原生命周期事务中先提交 delivery 再消费消息，crash/interruption/takeover 在 Attempt 终态后 supersede，pending 指令由下一 Attempt 重新绑定。prepared delivery 同时进入 agent graph snapshot/restore 校验。

本轮健壮性审计未发现未解决的高/中风险问题，并修正一项既有低风险边界：运行中的 Specialist 原可进入通用 manual consume，现提前返回稳定 `FAILED_PRECONDITION`，SQLite prepared-delivery trigger 继续作为第二道防线。测试覆盖严格 payload/未知字段拒绝、错误 sender/kind、最多 4 条有序指令、child-only memory、token/byte omission、无正文 provenance、prepare/replay、manual consume 拒绝、continue/finish exact-once commit、事件故障全事务回滚、crash 保留、active supersede 拒绝、expired-lease takeover 重投、graph restore、malformed direct-SQL 拒绝和 v26->v27 升级。全仓普通测试、全仓 `-race`、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、凭据模式扫描与 `govulncheck` 均通过，可达漏洞为 0；隔离真实二进制仅注册 mock Provider，初始化 workspace，创建/列举 review Run 并生成 schema v27 SQLite runtime 后删除全部临时数据。没有新增 CLI/HTTP/OpenAPI 路由、公开/model spawn、Shell/网络权限或 child 工具。

Bounded Specialist Scheduler 切片新增 Go-internal `SpecialistScheduler` 与持久化聚合 `RunAgentUsage` 复核，不增加 schema 版本。调度器在整个 bounded schedule 中复用一份 Run lease，每轮按稳定 Agent ID 最多并发两个 direct ready child，最多 32 轮；all-terminal、no-ready、round-limit、首错、父取消、token 和 execution budget 均为显式停止条件。父 context、heartbeat cancellation 或首个不可继续 child error 会 fan out 到同轮其他 child；所有 goroutine 完成 Attempt crash/continue/finish 写入后才返回并释放 lease。旧 generation 遗留 Attempt 在新调度开始时恰好一次恢复。scheduler goroutine 边界会把自定义 Provider/runtime panic 收敛为不含 panic payload 的 `INTERNAL`，已开始的 Attempt 先持久化 crash，再取消 sibling，避免进程崩溃或留下当前 lease 的 running child。

总预算不采用进程内猜测：每轮前后以只读 SQLite 事务核对 root checkpoint 与 root Agent token、Specialist Agent token 与全部 Attempt token，并累加所有 `specialist_model_calls.elapsed_millis`；任一投影不一致立即 `CONFLICT`。剩余 total token 和模型毫秒按实际参与 child 数确定性均分，调度后再次复核。审计确认一个已知低风险硬边界：Provider `MaxTokens` 只限制输出，最终 usage 还包含输入，因此单 round 可能被 Provider 上报推过总 token 线；实现会完整持久化实际 usage 并立即停止后续 round，不通过截断账本掩盖超支。

测试覆盖真实双 Provider 屏障并发、最多两个 child、共享 lease generation、父取消双向扇出、首个 child 失败取消 sibling、panic containment、all-terminal 与 round-limit、root+children aggregate token、执行 deadline 确定性分片、模型耗时求和、预算耗尽零 Provider 调用、过期 lease takeover 恢复和投影篡改拒绝。全仓 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...` 与 `govulncheck ./...` 全部通过，可达漏洞为 0。`rg` 复核确认 `NewSpecialistScheduler` 仅存在于内部实现与测试，没有 CLI/HTTP/OpenAPI/model spawn、工具、Shell 或网络入口。

Specialist Lifecycle Repair 切片新增 schema v28、`SpecialistProtocolRepair` 领域状态、`specialist_protocol_repairs` 持久化账本，以及 `agent.protocol_repair_requested/started/completed/failed` 事件。每个 AgentAttempt 最多请求一次 repair；`specialist_model_calls.model_attempt_number` 是全局连续序列，`transport_attempt` 在 primary/repair 两个 phase 内分别从 1 开始。无效 primary 的真实 usage 与 pending repair 原子提交，repair usage 继续累加到同一 Attempt 和 child token 总账；transport failure 不重复扣 token，但执行毫秒仍进入累计账本。

Runner 只把固定诊断附加到原可信请求，绝不把原始坏输出放进 repair prompt、Session 或事件。第二次无效响应原子标记 exhausted；预算不足、context cancellation、Attempt crash/interruption 和 stale-worker takeover 会在 Attempt 终态前把 pending repair 标记 aborted。`continue`/`finish` 同时由 Go 事务和 SQLite trigger 要求 repair 已 completed。每次 child turn 还会从 `RunAgentUsage` 复核 Run 剩余 total token 与执行时间，repair 只能使用 primary 后的余额；Provider input-inclusive usage 若单次越线仍会完整记账并立即终止，不伪造或截断账本。

测试覆盖 repair success、第二次 invalid、primary/repair 各自 transport retry、全局与 phase-local 编号、累计 token/执行时间、终态 start/terminal 幂等重放、预算中止、repair 调用中取消、crash abort、Session exact-once、原始坏输出隔离、SQLite 跳号/错 phase/未完成 repair 直接写入拒绝，以及 v27 model ledger 到 v28 的数据保留升级。审计发现并修复四项低风险健壮性问题：终态后 `model.started` 重放的判断顺序、超长 repair reason 截断后的 rune 缓冲、系统时钟回拨时的 resolution 时间，以及 Anthropic-compatible repair 请求可能产生连续 user message。全仓普通测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、生产凭据/运行产物扫描与 `govulncheck` 均通过，当前可达漏洞为 0。隔离真实 CLI smoke 创建 schema v28 runtime、workspace 和 review Run 后已删除全部临时数据。没有新增 CLI/HTTP/OpenAPI/model spawn、child tool、Shell 或网络权限。

Specialist Schedule Control 切片新增 schema v29、`specialist_schedules`/`specialist_schedule_agents`、独立的 `specialist_model_cancellations`/operation ledger，以及 `agent.schedule_started/stopped`。Scheduler 在当前 Run lease 下先写 start，正常、失败与取消路径都以计数和 RunAgentUsage 前后快照终结；新 generation 会在开始下一 schedule 前把旧 running 记录恰好一次收敛为 `abandoned/worker_lost`。持久化对象不向应用返回 lease identity，事件 payload 也不含 lease id/generation 或模型正文。

新的 `/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel` 与 schema v18 root route 共用独立 control token、4 KiB 严格 JSON 和 16-256 字节幂等键边界，但写入独立 child 账本。请求必须精确匹配 running Run、direct Specialist、active AgentAttempt、最新 started model call 与活动 execution lease；worker 先事务写 `model.cancel_observed`，再取消该调用自己的 Go context。模型 terminal 与 cancellation resolution 同事务提交；Attempt crash/interruption/takeover 会把遗留请求解析为 `attempt_terminated`/`worker_lost`，新调用不会继承旧意图。Scheduler 的首错策略仍可能本地取消同轮 sibling，但不会产生伪造的第二条 control request。

定向测试覆盖 schedule completed/replay/immutability、start/stop 事件、过期 lease schedule/Attempt 双恢复及恢复计数、协调器 panic 失败摘要、取消 key 摘要化与意图冲突、观察幂等、模型 terminal/Attempt terminal resolution、系统时钟回拨单调时间、两个 SQLite 连接经 HTTP 到 blocking Specialist Provider 的端到端取消、8 路独立 Store 并发同意图收敛、control/read capability 分离、OpenAPI live route 和 fencing/operation-key 非泄露。并发测试连续 10 轮均只创建一个 cancellation 与一个 request event。审计修复四项低风险问题：schema trigger 将 root `parent_id` 的 `NULL` 误判为空字符串；OpenAPI child route 测试复用了已 running 的 root；取消观察/解析时间在系统时钟回拨时可能早于 request；schedule start 后的协调层 panic 可能误写 completed summary。实现同时把 schedule Store 从普通 SpecialistRunner 最小接口中拆出，用 defer 收口 Provider panic 路径的 watcher/context 资源，并为 child operation ledger 增加不可变 trigger。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff` 与 `govulncheck ./...` 全部通过，可达漏洞为 0。凭据/运行产物扫描无命中；隔离 CLI smoke 仅注册 mock，初始化 workspace，在 schema v29 runtime 创建/list review Run 后删除全部临时数据。

Specialist Delegation Proposal 切片新增 schema v30、严格 `specialist_delegation.v1`、Supervisor-only `specialist_delegation_propose`、独立 `agent_proposal` action class、不可变 proposal/assignment 表、digest-only operation ledger，以及 metadata-only `agent.delegation_proposed`。模型最多建议两个 title/goal/Skill 子集/turn-token 预算；协议 unknown field、trailing JSON、重复目标和越界字段在预算前拒绝。良构调用扣一次工具预算后，Gateway/Store/SQLite 复核 active root、checkpoint、lease generation、Run/Session/Workspace、charged invocation、child 容量、不可转授 delegation capability 和 root 余量。成功只返回 proposal ID/count/status 与 `admission_authorized=false`，不创建 child、Session、预算预留或 schedule；只读 `run delegations/delegation` 可检查脱敏明细。

定向测试覆盖协议漂移、未加 fencing 的调用、危险目标 Policy 永久拒绝、nmap 类待审批文本只形成 proposal、Skill 提权、预算/容量边界、proposal/assignment SQL 不可变、Provider call ID 变化下语义重放、两个 SQLite 连接八路并发只创建一条 proposal、密钥和原始 operation key 非持久化、事件不含标题/目标/lease，以及提案后 Agent 图仍只有 root。健壮性复核把良构但越权的模型提案转换为 `INVALID_ARGUMENT` 工具结果供模型纠正，而租约、存储和内部故障仍中止 turn；同时把提案能力标记为不可转授，并让旧 root 在 Supervisor 同步时惰性获得该能力。提交前审计还发现 operation ledger 初稿误存 `lease_id/generation`，现已移除两列并让 SQLite trigger 直接关联当前 active lease/checkpoint，测试锁定账本无租约身份。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff` 与 `govulncheck ./...` 全部通过，可达漏洞为 0。隔离 CLI smoke 验证 version/provider、workspace、Run、工具 schema、空提案查询和普通 `tool invoke` 旁路拒绝；凭据与运行产物扫描无命中，临时 runtime 已删除。当前未发现未解决的高/中风险问题；已知功能边界是 proposal 尚无 review/application 状态，任何 admission 仍只能通过原有显式 Go policy。

## 七、下一开发切片

1. 为 schema v30 proposal 增加独立 operator/internal review 与可恢复 application：批准时重新复核 Policy、capacity、budget、Skills，幂等接入现有 admission，并向 child 写入严格父指令；拒绝、过期与中断恢复不能留下幽灵 child，模型仍不能直接 admission 或 spawn。
2. React/Vite 后续从 `docs/openapi.json` 生成 client/DTO，并通过带 Authorization header 的 fetch 消费 SSE，不把 token 放入 URL、不重复实现 Go Policy。
3. Docker/Local 真实执行继续关闭，直到 Sandbox manifest、资源、网络、取消与证据导出全部通过审计。

## 八、仓库同步与恢复约定

规范远程仓库：`https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench`。

每次完成一个开发切片后，依次执行功能复核、测试、代码与安全审计、项目记忆更新、Git 提交和 GitHub 推送。当前仓库直接开发并推送 `main`；除非用户明确要求，不创建功能分支或 PR。

长对话恢复时依次阅读：`README.md`、`docs/PROJECT_STATUS.md`、本文件、`docs/TASK_BOOK.md`、`docs/http-api.md`、`docs/errors.md`、`docs/adr/0001-go-control-plane.md` 和 `docs/adr/0002-run-centric-runtime.md`。
