# CyberAgent Workbench 进度书

更新时间：2026-07-20

## 一、当前阶段

项目正在从可运行的 v0.1 CLI/TUI 骨架迁移到 V2 Run-centric Runtime。CTF 专用求解能力继续后置，当前先完成主流 AI Agent 工具需要的通用运行时。

从 schema v49 起采用“双指标”，不再使用容易混淆的单一“整体产品愿景”百分比：

- 架构完成度：约 99%；其中 V2 Run-centric 控制平面约 99%。该指标衡量 Go 主控、持久化状态机和模块边界的覆盖程度。
- 产品可用度：完整 Code + Cyber 产品约 95-97%；其中通用 Coding Agent 工作流约 95-96%，Cyber 自动化工作流约 20%。该指标衡量用户现在能够完成多少真实端到端任务。
- 上述数值是依据已测试任务切片给出的工程估算，不是性能基准，也不代表仍被安全关闭的功能已经可用。

V2 的 P0/P1 已完成，P2 已具备稳定的单 Agent 恢复、Provider streaming、主动取消、有界工具循环和跨进程 execution lease。P3-P5 已落地 Work/Note/Context、最多两个核心 child、独立 1/2/4/6 只读 Fan-out、Tool Gateway、审批/Grant、预算、ScriptProcess 与 Artifact。P9/Desktop 已推进到 schema v82 / D1-G7/V6，通用运行时安全面已推进到 H1-H3/R4/C1-C3：除既有 Run/Session/Plan/审批/Workspace/证据/回执/行动中心外，现有 API/Desktop 还支持安全恢复的 Monaco FileEdit、独立多文件汇总、exact-root 只读 Repository、可导航精确文件历史、可分页逐检查项验证下钻、Code Journey、累计上下文记忆、Provider generation reload 和有界 wake worker。R4 运行元数据仍只存在于内部 `NonProductOnly` 测试边界。真实 Local/Docker/Shell/Git 进程、安装脚本/钩子和远程 Skill 分发继续关闭。

P8 已推进到 schema v37 及其只读 CI 投影：v35 把完成的 Fan-out execution 投影为通用 `draft` Finding、不可变 `model_assertion` Evidence 和可重建的 Markdown/JSON Report；v36 增加同 Run 冻结 Artifact Evidence、一次性 operator `validated/rejected` 决定与完整复核；v37 以独立不可变事实完成 `validated -> accepted -> fixed`，并强制修复 Evidence 来自接受后新建且未用于验证的同 Run Artifact。验证、接受和修复始终分离；SARIF、通用 CI gate 与 GitHub Actions annotations 均为同一持久化事实上的 Go 只读投影。

P7 已推进到 schema v47：v39-v40 固定并交付 root Skill，v41-v42 建立 Code/Cyber 与 Plan/Deliver 工作流，v43-v44 固化来源隔离和切片交付门禁，v45-v46 提供 exactly-once 操作者引导及控制，v47 为每个 Specialist Attempt 从父 Run 选择中派生最多一项最小 Skill。Code/Cyber 目录保持分离，child assignment 与模型都不能选择或扩权，Skill 正文不落库。

P6 已完成五条纵向链路：schema v48 定义严格 `sandbox_manifest.v1` 并形成 metadata-only preparation；schema v49 复用通用审批账本，增加显式 request/review、完整 Manifest 重新提交、`os.Root` 工作区挂载解析和不可变 candidate；schema v50 再增加禁用态 `sandbox_execution.v1`、输入 Artifact 完整性快照、metadata-only 输出计划、独立 generation fencing、取消与清理恢复；schema v51 固定 16 项后端威胁检查、禁用握手、未绑定容器身份和 metadata-only 输出导出预检；schema v52 使用无 daemon 的假客户端逐项绑定模拟证据，并用内存假 Artifact sink 验证输出原子事务。原始命令、argv、路径、环境、密钥、目标、Manifest、Artifact 正文、输出 locator、夹具正文和 lease token 不进入公开账本或事件。全部生产检查仍未验证，全部后端/执行/导出/生产 Artifact 提交标志仍为 false，Local/Docker 和真实进程完全关闭。

P6 后续已推进到 schema v62：v53-v54 完成固定端点只读观测与确定性计划，v55-v56 完成 never-started 写演练及恢复，v57-v60 完成 descriptor-safe 捕获、daemon handoff 与输入投影，v61 把投影写入回读核验的只读卷并保留 never-started target，v62 再提供 metadata-only 资源复查和 generation-fenced 精确清理。v62 在任何 DELETE 前检查目标与全部卷，外来碰撞保证零 DELETE，完成后复查全部缺失；它仍没有 start、exec、输出导出或 Artifact 提交能力。

P6 当前已推进到 schema v68：v63 固定阻塞态 start-gate 审查，v64 只记录 `preview|docker|local` 非授权档位，v65 建立 16 项非授权生产证据 receipt，v66 把 collector 调用放进 durable write-ahead attempt 与 generation fencing，v67 增加显式 opt-in 的 Linux 只读 daemon harness，v68 再为精确完成的 v67 receipt 增加一次不可变操作员接纳/拒绝决定。即使接纳，生产验证数仍为零、16 项仍全部阻塞，且不增加 Docker 写入或进程权限。

## 二、已完成功能

### Agent 与运行时

- CLI 入口、命令分发、版本命令和 Run-first Bubble Tea TUI；选择器对 Run/Session 各限最近 50 条，Edits 对最近 20 条 FileEdit 提供 metadata/diff-only 只读详情和 128 KiB/4096 行显示上限。
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
- schema v31 以不可变 `specialist_delegation_reviews/review_operations` 和 `agent.delegation_reviewed` 记录一次 operator approved/rejected；拒绝理由脱敏且不进事件，结果不创建 child 且不授权 admission。
- schema v32 以 `specialist_delegation_applications/application_assignments/application_operations` 将 approved proposal 可恢复地接入现有 admission 与严格父指令；applying 阻止 root/无关 mutation/child scheduler，成功保持 child ready，终态 Run 原子 abort。
- schema v38 以不可变 `specialist_operator_schedule_requests/request_agents/operations/attempts` 为 applied application 增加 operator-only `schedule/continue`；目标只能是其 instructed ready child，pending request 预留目标，相同 key 重放不重复模型调用，新 key 才开始下一次 continuation，过期 schedule 由更高 lease generation 收敛并恢复。
- schema v47 以 `specialist_skill_context.v1` 为 active child Attempt 派生最多一项父选择中的指导；Code/Cyber 映射、独立预算、assignment 指纹、并发幂等 preparation 与首次模型启动原子 commit 全由 Go/SQLite 控制，正文、路径、名称、版本和内容哈希不进入账本或事件。
- schema v33 以独立 `readonly_fanout_plans/files/shards/operations` 支持 `auto/1/2/4/6` 只读分片规划；Go/SQLite 锁定 workspace-list/read 能力指纹，核心 Agent/v32 上限仍为 2；v33 当时不调用 Provider，执行能力现由 schema v34 提供。
- schema v34 以独立 `readonly_fanout_executions/execution_shards/model_calls/findings/execution_operations` 执行已冻结的 v33 计划；调用前重建并核对完整 snapshot，正文只在内存中脱敏后进入 tool-free JSON Provider，首错取消 siblings，lease takeover 只重试未完成 shard，未知调用按预留额度计费。
- schema v35 以 `finding_reports/findings/finding_evidence` 将完成的 v34 execution 确定性投影为通用报告；只合并事实字段完全相同的声明，严重度不同绝不合并，重复声明保留全部 Evidence 并采用最低置信度，Markdown/JSON 重放字节稳定且不调用 Provider。
- schema v36 以独立覆盖层挂接同 Run Artifact Evidence；Store 重读完整 blob 并复核 SHA-256/大小/MIME/stream/tool/source/redacted，SQLite 冻结 Artifact 和验证事实。`validated` 至少需要一份 Artifact，`rejected` 可为零 Evidence；同键并发收敛，第二决定和决定后追加稳定冲突，v35 投影摘要不变。
- schema v37 增加独立 acceptance/remediation/fix 覆盖层；接受冻结 validation 快照，修复 Evidence 通过 Run event sequence 证明 Artifact 晚于接受且不可复用验证 Artifact，fix 冻结有序修复 Evidence 数量与摘要。SARIF 只输出 `validated/accepted` 未解决项，CI 默认门禁同样阻断二者，fixed/rejected 永不阻断。
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
- `--local` 只记录 requested backend；schema v48 仅构造 NoopRunner 做确定性 Manifest 校验，CLI 不构造 Local 执行器，审批前后均只产生 dry-run，不执行宿主机命令。
- schema v48-v56 增加 `sandbox_manifest.v1`、严格 duplicate-aware JSON、共享审批、`sandbox_execution_candidate.v1`、禁用态生命周期与后端/输出预检、仅模拟后端证据、原子内存输出事务、固定本机端点的只读 Docker 元数据观测、确定性容器计划、未启动容器写演练和可恢复 pre-daemon attempt；CLI 可离线校验、准备、审批、重新提交、begin/preflight/evidence/output-simulate/observe/docker-plan/docker-rehearse/docker-attempt-resume/cancel/cleanup。命令、argv、路径、环境值、密钥引用、目标、Manifest JSON、Artifact 正文、输出 locator、夹具正文、原始 daemon/socket/container 身份、operation key 和 lease token 不进入公开账本/事件；全部生产后端执行能力仍为 false。
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
- Noop 与 Local Runner 保持 fail-closed；Docker 仅通过 P6 的固定端点、显式确认、最小接口执行只读观测、never-started 资源准备与精确清理，仍没有容器进程 start/exec 能力。

### 存储与 Run 架构

- CGO SQLite 驱动 `github.com/mattn/go-sqlite3`，当前 schema 版本为 v68。
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

当前仓库的控制平面与全部安全/业务状态机使用 Go；React/Vite read-first 控制台使用 TypeScript/TSX。SQLite 通过 CGO 使用 C 编译链；SQL 嵌入 Go migration；CSS、YAML、JSON 与 Markdown 分别用于界面、配置、契约和文档，PowerShell 只出现在 Windows 使用示例中。Rust 分析器尚未开始。

长期边界：

- Go 是唯一控制平面，负责 Agent、Run、Session、LLM、Secrets、Policy、Workspace、SQLite、Docker 和审计。
- Rust 只做确定性分析工具，通过 stdin/stdout JSON 接收任务并返回结果，不调用 LLM、不管理会话或用户配置。
- TypeScript 只做 React/Web 界面，通过 HTTP/WebSocket 调用 Go，不直接操作 Rust、Docker、Shell、SQLite 或 API key。
- 合法链路是 `TypeScript -> Go -> Rust/Docker/LLM`。

依赖按功能引入。Bubble Tea、SQLite/CGO、`net/http` 与 React/Vite 已使用；OpenAPI/JSON Schema 当前由 Go 标准库反射生成，前端 DTO 再从该快照生成。Cobra、Chi、Docker client、Qdrant 与 Rust crates 尚未引入。

## 四、尚未完成

- 经过生命周期投影和跨 chunk 脱敏的用户可见文本 streaming；当前 TUI 只展示元数据。
- Provider 费用预算，以及最近 Session 消息与结构化记忆共用的统一 token 预算。
- Finding/Evidence/Report 的模型辅助跨来源归并仍未开放；确定性投影、验证/接受/修复生命周期、SARIF/CI，以及 Web 只读视图已完成。
- 经过单独安全设计的用户可见模型文本 streaming；持久化 metadata SSE 与跨进程主动取消已完成。
- OpenAI-compatible 与 Ollama Provider。
- 真实 Docker 隔离与命令执行；Tool Gateway、first-class ScriptProcess、逐次审批、Session Grant、工具预算和输出 Artifact 已完成。
- 通用 HTTP/Web 写入接口与 Rust analyzer 进程；TypeScript 只读 Web UI、本地读取 API、metadata SSE、精确取消控制及 Go 同源托管已完成。
- MCP Server、插件系统和远程任务能力。
- 通用 Agent 稳定后的 CTF 自动分析与求解流程。
- 子 Agent admission、Attempt scheduling、独立 Session/累计预算计费、崩溃恢复、消息唤醒、root/child context projection、WorkItem/Note Owner 外键、一次 lifecycle repair、持久化 schedule 摘要、跨进程 child-call cancellation 和 operator-only 双 child 编排已完成；结构化 dependency waiting、HTTP 与模型自主调度产品面仍未开放。

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
- `script run --local` 的执行旁路已移除；生产代码扫描中不存在 `Runner.Run` 调用。schema v48 后仅有 Noop 校验接线，LocalSandbox 仍是禁用的开发后端。
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

测试覆盖真实双 Provider 屏障并发、最多两个 child、共享 lease generation、父取消双向扇出、首个 child 失败取消 sibling、panic containment、all-terminal 与 round-limit、root+children aggregate token、执行 deadline 确定性分片、模型耗时求和、预算耗尽零 Provider 调用、过期 lease takeover 恢复和投影篡改拒绝。该内部切片完成时全仓 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...` 与 `govulncheck ./...` 全部通过，可达漏洞为 0。其后 schema v38 只增加 operator-only CLI gate；HTTP/OpenAPI/model spawn、普通工具、Shell 或网络入口仍不存在。

Specialist Lifecycle Repair 切片新增 schema v28、`SpecialistProtocolRepair` 领域状态、`specialist_protocol_repairs` 持久化账本，以及 `agent.protocol_repair_requested/started/completed/failed` 事件。每个 AgentAttempt 最多请求一次 repair；`specialist_model_calls.model_attempt_number` 是全局连续序列，`transport_attempt` 在 primary/repair 两个 phase 内分别从 1 开始。无效 primary 的真实 usage 与 pending repair 原子提交，repair usage 继续累加到同一 Attempt 和 child token 总账；transport failure 不重复扣 token，但执行毫秒仍进入累计账本。

Runner 只把固定诊断附加到原可信请求，绝不把原始坏输出放进 repair prompt、Session 或事件。第二次无效响应原子标记 exhausted；预算不足、context cancellation、Attempt crash/interruption 和 stale-worker takeover 会在 Attempt 终态前把 pending repair 标记 aborted。`continue`/`finish` 同时由 Go 事务和 SQLite trigger 要求 repair 已 completed。每次 child turn 还会从 `RunAgentUsage` 复核 Run 剩余 total token 与执行时间，repair 只能使用 primary 后的余额；Provider input-inclusive usage 若单次越线仍会完整记账并立即终止，不伪造或截断账本。

测试覆盖 repair success、第二次 invalid、primary/repair 各自 transport retry、全局与 phase-local 编号、累计 token/执行时间、终态 start/terminal 幂等重放、预算中止、repair 调用中取消、crash abort、Session exact-once、原始坏输出隔离、SQLite 跳号/错 phase/未完成 repair 直接写入拒绝，以及 v27 model ledger 到 v28 的数据保留升级。审计发现并修复四项低风险健壮性问题：终态后 `model.started` 重放的判断顺序、超长 repair reason 截断后的 rune 缓冲、系统时钟回拨时的 resolution 时间，以及 Anthropic-compatible repair 请求可能产生连续 user message。全仓普通测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、生产凭据/运行产物扫描与 `govulncheck` 均通过，当前可达漏洞为 0。隔离真实 CLI smoke 创建 schema v28 runtime、workspace 和 review Run 后已删除全部临时数据。没有新增 CLI/HTTP/OpenAPI/model spawn、child tool、Shell 或网络权限。

Specialist Schedule Control 切片新增 schema v29、`specialist_schedules`/`specialist_schedule_agents`、独立的 `specialist_model_cancellations`/operation ledger，以及 `agent.schedule_started/stopped`。Scheduler 在当前 Run lease 下先写 start，正常、失败与取消路径都以计数和 RunAgentUsage 前后快照终结；新 generation 会在开始下一 schedule 前把旧 running 记录恰好一次收敛为 `abandoned/worker_lost`。持久化对象不向应用返回 lease identity，事件 payload 也不含 lease id/generation 或模型正文。

新的 `/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel` 与 schema v18 root route 共用独立 control token、4 KiB 严格 JSON 和 16-256 字节幂等键边界，但写入独立 child 账本。请求必须精确匹配 running Run、direct Specialist、active AgentAttempt、最新 started model call 与活动 execution lease；worker 先事务写 `model.cancel_observed`，再取消该调用自己的 Go context。模型 terminal 与 cancellation resolution 同事务提交；Attempt crash/interruption/takeover 会把遗留请求解析为 `attempt_terminated`/`worker_lost`，新调用不会继承旧意图。Scheduler 的首错策略仍可能本地取消同轮 sibling，但不会产生伪造的第二条 control request。

定向测试覆盖 schedule completed/replay/immutability、start/stop 事件、过期 lease schedule/Attempt 双恢复及恢复计数、协调器 panic 失败摘要、取消 key 摘要化与意图冲突、观察幂等、模型 terminal/Attempt terminal resolution、系统时钟回拨单调时间、两个 SQLite 连接经 HTTP 到 blocking Specialist Provider 的端到端取消、8 路独立 Store 并发同意图收敛、control/read capability 分离、OpenAPI live route 和 fencing/operation-key 非泄露。并发测试连续 10 轮均只创建一个 cancellation 与一个 request event。审计修复四项低风险问题：schema trigger 将 root `parent_id` 的 `NULL` 误判为空字符串；OpenAPI child route 测试复用了已 running 的 root；取消观察/解析时间在系统时钟回拨时可能早于 request；schedule start 后的协调层 panic 可能误写 completed summary。实现同时把 schedule Store 从普通 SpecialistRunner 最小接口中拆出，用 defer 收口 Provider panic 路径的 watcher/context 资源，并为 child operation ledger 增加不可变 trigger。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff` 与 `govulncheck ./...` 全部通过，可达漏洞为 0。凭据/运行产物扫描无命中；隔离 CLI smoke 仅注册 mock，初始化 workspace，在 schema v29 runtime 创建/list review Run 后删除全部临时数据。

Specialist Delegation Proposal 切片新增 schema v30、严格 `specialist_delegation.v1`、Supervisor-only `specialist_delegation_propose`、独立 `agent_proposal` action class、不可变 proposal/assignment 表、digest-only operation ledger，以及 metadata-only `agent.delegation_proposed`。模型最多建议两个 title/goal/Skill 子集/turn-token 预算；协议 unknown field、trailing JSON、重复目标和越界字段在预算前拒绝。良构调用扣一次工具预算后，Gateway/Store/SQLite 复核 active root、checkpoint、lease generation、Run/Session/Workspace、charged invocation、child 容量、不可转授 delegation capability 和 root 余量。成功只返回 proposal ID/count/status 与 `admission_authorized=false`，不创建 child、Session、预算预留或 schedule；只读 `run delegations/delegation` 可检查脱敏明细。

定向测试覆盖协议漂移、未加 fencing 的调用、危险目标 Policy 永久拒绝、nmap 类待审批文本只形成 proposal、Skill 提权、预算/容量边界、proposal/assignment SQL 不可变、Provider call ID 变化下语义重放、两个 SQLite 连接八路并发只创建一条 proposal、密钥和原始 operation key 非持久化、事件不含标题/目标/lease，以及提案后 Agent 图仍只有 root。健壮性复核把良构但越权的模型提案转换为 `INVALID_ARGUMENT` 工具结果供模型纠正，而租约、存储和内部故障仍中止 turn；同时把提案能力标记为不可转授，并让旧 root 在 Supervisor 同步时惰性获得该能力。提交前审计还发现 operation ledger 初稿误存 `lease_id/generation`，现已移除两列并让 SQLite trigger 直接关联当前 active lease/checkpoint，测试锁定账本无租约身份。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff` 与 `govulncheck ./...` 全部通过，可达漏洞为 0。隔离 CLI smoke 验证 version/provider、workspace、Run、工具 schema、空提案查询和普通 `tool invoke` 旁路拒绝；凭据与运行产物扫描无命中，临时 runtime 已删除。v30 发布边界没有 review/application；v31 已补齐独立 review，但 admission/application 仍只能通过显式 Go policy。

Specialist Delegation Review 切片新增 schema v31、一次性 `approved/rejected` review、digest-only review operation、metadata-only `agent.delegation_reviewed`，以及 `run delegation approve/reject/show` 与带 review 状态的列表。批准只在 Run 仍 running 时成立；拒绝必须有理由并可在 Run 终态后关闭提案。Store 对理由和 reviewer 二次脱敏，严格校验事件无 reason/未知字段，SQLite 绑定 immutable proposal/Run/root 并拒绝 review/operation 更新或删除。相同 key/意图跨两个 SQLite 连接八路收敛为一条 review/event，改意图与第二 key 改判返回 `CONFLICT`。所有结果均为 `admission_authorized=false`、`application_required=true`，Agent 图保持 root-only。审计未发现未解决的高/中风险问题，并修复三项低风险一致性问题：缺失的 JSON 布尔值原可等同显式 `false`，现在事件必须实际携带 non-authorization 字段；系统时钟回拨原可让 review 早于 proposal，现由应用钳位时间且 Store/trigger 二次拒绝；第二 operation key 原可能因 Run 后续终态从稳定冲突漂移为前置条件失败，现先查询既有 review 再复核当前生命周期。并发定向测试连续 10 轮通过，v30 proposal 原地升级保留，真实 CLI 集成测试覆盖 approve/replay/show/list/second-decision conflict。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff` 与 `govulncheck ./...` 全部通过，可达漏洞为 0；凭据和运行产物扫描无命中。

Recoverable Specialist Delegation Application 切片新增 schema v32、`applying/applied/aborted` application、assignment `pending/admitted/instructed` 状态机、摘要 operation 关联，以及 `run delegation apply`。Begin 在同一 writer transaction 复核 approved review 及其 operation、当前 Policy、running Run、ready root、active Session、idle child runtime、Skill 子集、既有 child policy、容量与总预算，并在 Coordinator 写入前保存每个 assignment 的 admission/message operation digest。Agent 或 Message 已提交但 application 回写失败时，同 key 重放会找回原实体后继续；双 Store 八路同时 apply 只形成一条 application、最多两个 child 和每 child 一条指令。applying 期间 root turn、无关 admission/message、schedule 和 direct Attempt 均 fail closed；Run completed/failed/cancelled 与 application abort/计数事件同事务提交。成功只留下 ready child，零 AgentAttempt、零 schedule。独立审计未发现未解决的高/中风险问题，并修复两项低风险完整性缺口：application 起始 assignment/operation 时间线现由 Go 与 SQLite 双重绑定，且 durable schedule 门禁新增直接回归测试。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff`、凭据/运行产物扫描与 `govulncheck ./...` 全部通过，可达漏洞为 0。

Read-only Fan-out Planning 切片新增 schema v33、固定 `readonly_fanout.v1` 能力指纹、`auto/1/2/4/6` 档位解析、工作区 snapshot scanner、确定性 shard、摘要幂等 operation，以及 `run fanout plan/fanouts/fanout show`。扫描器拒绝路径逃逸并跳过 symlink、VCS、依赖/构建目录、二进制和 secret-like 文件，硬限制为 20,000 entries、256 files、128 KiB/file、768 KiB total。Go 与 SQLite 同时要求 running/network-disabled Run、相同 workspace 的 active Session、准确 capability fingerprint 和完整 file/shard 计数；所有 v33 行不可更新或删除。双 Store 八路同 key 收敛为一个 plan/event，连续十轮通过；改意图冲突、Policy 拒绝零状态、原始 key 不落库、v32 原地升级保留，CLI 明确 `execution_authorized=false`，Agent 图保持 root-only 且 Attempt/schedule 为零。独立审计未发现未解决的高/中风险问题，并修复两项低风险边界：规划 writer 原误用了要求 Tool Budget 初始化的 helper，现改为独立 Run 行锁且不伪造工具消费；scope 起点原会解析工作区内 symlink，现明确拒绝并通过 Go `os.Root` 约束读取。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff`、`govulncheck ./...`、凭据/运行产物扫描和隔离真实 CLI smoke 全部通过，可达漏洞为 0。

Bounded Read-only Fan-out Execution 切片新增 schema v34、`ReadOnlyFanoutExecutionService`、严格 `readonly_fanout_report.v1`、执行/分片/模型调用/发现项/摘要幂等账本，以及 `run fanout execute` 与 `run fanout execution`。执行器持有现有 Run execution lease，先重建 v33 manifest，再逐文件通过 Go `os.Root` 复核 regular-file identity、大小和 SHA-256；新增、删除、修改、symlink 漂移或超界 prompt 均在调用前 fail closed。通过复核的正文只在内存中存在，并在构建 JSON payload 前做凭据脱敏；请求没有 ToolSpec，强制 JSON mode，报告 unknown/trailing 字段被拒绝，finding 只能引用本 shard 的 manifest path。核心 delegation 的两个 child 上限、Agent 图、AgentAttempt 和 Specialist schedule 均不变。

Go 按计划档位并发 1/2/4/6 个 worker，首错会取消共享 context 并等待所有 shard 写入 durable terminal state。schema v34 的 private lease generation 会 fence 旧 worker；新 generation 将未决调用标记 `abandoned`、running shard 恢复为 pending，并只重试未完成 shard，单 shard 最多三次恢复尝试。root、Specialist 和 Fan-out 用量由 `RunAgentUsage` 一次性从 SQLite 重建；正常调用使用实际 token/elapsed，结果未知的 failed/cancelled/abandoned 调用使用保守预留量，且有 timeout 时把剩余模型毫秒在待执行 shards 间确定性均分。Specialist schedule 持久化也增加 Fan-out 前后预算分量，防止两种并发模式交替运行时丢失基线。

功能复核覆盖 mock 六路成功、终态同 key 零重复、4096-output 预算拒绝零 Provider 调用、六路 barrier 首错后 1 failed + 5 cancelled、无 started/running 残留、旧 lease fencing、未知调用 612 token/1000ms 预留保留、same-key 不同临时 execution ID 收敛、v33->v34 plan 保留、CLI plan/execute/replay/show/usage/event 和 Agent 图仍 root-only。安全审计确认原始 operation key、工作区绝对根目录和原始源码不落库，Run 事件不包含 goal、manifest 路径、finding 详情或报告正文；v33 manifest 的相对路径以及 v34 严格报告/finding 则按设计留在本地 SQLite，供恢复、展示与审计。模型不能改档位、调用工具、递归 spawn 或写文件。审计阶段修复三项低风险代码问题：执行创建重放原把随机 execution ID 误当意图字段，现按摘要意图收敛；Specialist schedule 原没有 Fan-out 预算列，现保留兼容 subtotal 并单独持久化 Fan-out 分量；终态失败原可在所有 shard 已完成时伪造，现由 Domain 与 Store 双重拒绝。提交前复核另修正了一处关于持久化边界的文档表述。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff`、`govulncheck ./...` 与凭据扫描全部通过，可达漏洞为 0。隔离的真实二进制 CLI smoke 也完成 tier 6 规划与执行、六个终态 shard、零重复重放、统一用量查询及 root-only Agent 图验证。

Deterministic Finding/Evidence/Report Projection 切片新增 schema v35、通用有界领域类型、精确事实 fingerprint、确定性 ID/投影摘要、`building -> generated` 事务门、`report.generated`，以及 `run fanout report`/`report show` 的 Markdown 与 JSON 输出。投影只接受 completed v34 execution；每条源 finding 作为不可变 `model_assertion` Evidence 绑定原 fingerprint 与 shard report digest。Go 仅在严重度、类别、标题、详情、路径和行号完全相同时去重，不会调用模型做语义归并或改写严重度；重复置信度取保守最小值。所有 Finding 初始为 `draft`，不能被介绍成已验证漏洞。

SQLite 在同一 writer transaction 内先创建不可见 building aggregate，插入 Finding/Evidence 后再复核源行数、分组数、严重度计数及连续序号，全部一致才允许 generated；终态三类记录均不可更新或删除。十轮双 Store 八路并发均只产生一份 report/event，v34->v35 原地升级保留 execution 且不合成报告。测试还覆盖严重度分歧不合并、Markdown/文件名注入中和、JSON 重放、零 finding execution、直接 SQL 篡改拒绝和事件正文零泄露。审计未发现未解决的高/中风险问题，并修复三项低风险问题：事务在读 execution 前先抢 Run writer lock，避免跨连接锁升级竞争；排序改为 path/line 优先于摘要；Markdown code span 动态适配反引号并由 SQLite 复核每个 Finding 的 Evidence 序号连续。隔离真实二进制 smoke 完成 plan/execute/report JSON/字节级重放/Markdown 回读，确认只有一个 `report.generated`、零额外 Agent 节点，并在校验临时路径后清理全部运行数据。最终 `go test ./...`、全仓 `go test -race ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、`go mod tidy -diff`、`govulncheck ./...`、gofmt、凭据和文档现状扫描全部通过，可达漏洞为 0。

Artifact-backed Finding Validation 切片新增 schema v36、`finding_artifact_evidence`/operation ledger、`finding_validation_decisions`/operation ledger、`finding.evidence_attached`/`finding.validation_decided`，以及 `report finding attach/validate/reject/verify`。验证是 v35 源投影之外的 additive overlay：原始 Finding 永远保持 `draft`，投影摘要计算显式排除验证覆盖层，因此模型声明与人工结论不会混写。

证据挂接会在 Run writer lock 内重读完整 Artifact blob，复核内容哈希、大小、MIME、stream、tool、source、Run 和脱敏标志，再复制不可变元数据；v36 同时以 SQLite trigger 禁止所有 `run_artifacts` 更新/删除。每个 Finding 最多挂 64 份 Artifact Evidence，只能在决定前追加。`validated` 必须至少有一份证据，`rejected` 可记录零证据的无法复现结论；决定冻结当时有序 Evidence 的数量和摘要。说明/理由在应用与 Store 双重脱敏，原始 operation key、说明、理由和 Artifact 正文均不进入 Run 事件。

功能复核覆盖两 SQLite Store 八路证据/决定并发各只写一条、同键重放、改判冲突、跨 Run Artifact 拒绝、无 Evidence 验证拒绝、零 Evidence 拒绝成功、决定后追加拒绝、原始 key 不落库、事件正文零泄露、v35->v36 数据保留、迁移前已篡改 Artifact 的哈希拒绝，以及真实 CLI 的 report->Artifact->attach->validate->verify->render 全链路。审计未发现未解决的高/中风险问题，并修复一项低风险语义缺口：Evidence 现在明确冻结并展示 Artifact 的 `redacted` 标志，避免把脱敏正文误认为原始字节。全仓普通测试、race、vet、staticcheck、模块校验与 `govulncheck` 已通过，当前可达漏洞为 0。

Trust-aware SARIF and CI Gate 切片不增加数据库迁移，只在 `internal/report` 与 CLI 增加纯读取投影。`report show --format sarif` 生成 OASIS SARIF 2.1.0 Errata 01，包含五个稳定 severity rule、工作区相对且逐段转义的 URI、v35 Finding fingerprint、报告/Run/投影摘要和三类验证计数。只有带 operator `validated` 覆盖层的 Finding 会进入 `results`，Artifact 正文、Evidence note 和 validation reason 均不输出。`report check` 默认以 `validated/high` 阻断并返回现有 `FAILED_PRECONDITION` 退出码 4；`active` 必须显式指定才纳入 draft，`none` 可关闭阻断，rejected 永不匹配。文本与 JSON 输出都会在失败退出前完整写出，便于 CI 保存证据。

本轮审计发现并修复一项中风险兼容性问题：初稿按 OASIS 语义把 draft/rejected 映射为 `result.kind=review/pass`，但 [GitHub Code Scanning 支持列表](https://docs.github.com/en/code-security/reference/code-scanning/sarif-files/sarif-support) 不消费 `kind`，这些结果仍可能显示为告警。最终实现因此收紧为 validated-only `results`，完整状态仍保留在 SQLite、Markdown/JSON 与 SARIF Run 汇总中。后续健壮性复核又修复三项低风险边界：公共 Go `GatePolicy` 现在拒绝可能 fail-open 的非规范化状态值；文本门禁传播输出写入错误；公开 SARIF 移除由完整 Evidence 元数据计算的集合摘要，只保留证据数量。定向测试覆盖确定性、空结果数组、URI/fingerprint、私密叙述隔离、默认/active/none 门禁和 CLI 退出码；真实二进制走完 report->Artifact->validate->SARIF/check，并通过 [OASIS 官方 schema](https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json) 校验。最终全仓普通测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、凭据/运行产物扫描与 `govulncheck` 全部通过，可达漏洞为 0。

Finding Acceptance and Remediation 切片新增 schema v37、`finding_acceptance_decisions`、`finding_remediation_evidence`、`finding_fix_decisions` 及三份 digest-only operation ledger，并提供 `report finding accept/remediation attach/fix`。接受要求已有 `validated` 决定，并冻结其 ID、Artifact Evidence 数量和摘要；它不会把 validation 自动升级为 authorization。修复 Evidence 只能绑定同 Run 且 `artifact.created` 事件序严格晚于 `finding.accepted` 的新 Artifact，验证 Artifact 和接受前输出都被拒绝。fix 至少需要一份修复 Evidence，并冻结有序数量与独立域摘要。所有决定、Evidence、operation 和 Run Artifact 都不可更新或删除，叙述和原始 operation key 不进入事件。

Markdown/JSON 现在重建完整五态生命周期；SARIF 仅输出 `validated/accepted` confirmed-unresolved 结果，使用独立 validation/lifecycle property，fixed 从结果中消失。默认 validated/high CI 门禁同时阻断 accepted，`active` 额外纳入 draft，fixed/rejected 永不匹配。功能复核覆盖 v36 原地升级、两个 SQLite 连接的八路 acceptance/Evidence/fix 收敛、改意图冲突、无证据 fix、验证 Artifact 复用、接受前 Artifact、fix 后追加、投影摘要稳定、事件正文隔离、SQL 不可变和完整 CLI 生命周期；并发收敛连续 10 轮通过。代码审计未发现未解决的高/中风险问题，并修复三项低风险一致性问题：v37 触发器补齐 decision/Evidence 时间顺序，生命周期错误不再误称 validation，SARIF 把 validation 状态与 Finding 当前状态拆为两个字段。最终 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、凭据/运行产物扫描与 `govulncheck` 全部通过，可达漏洞为 0。隔离真实二进制创建 schema v37 runtime，完成 report->validation->acceptance->fresh remediation->fix，确认 fixed、SARIF 零结果、CI gate 通过和三类新事件各一条后清理全部临时数据。

Explicit Operator Specialist Scheduling 切片新增 schema v38、不可变 schedule request/target、digest-only operation、request-to-schedule attempt 映射、`agent.operator_schedule_requested`，以及 `run delegation schedule/continue`。只有 applied application 记录的 operator 可以选择其中一到两个 instructed ready child；Go 在每次未终态执行前重查 Policy，并复用现有 Run lease、两-child scheduler、总预算、取消扇出和模型账本。pending request 会阻止普通 scheduler 抢占目标。双 Store 八路同键调用只形成一条 request、一条 schedule 和每 child 一个 Attempt；重放零重复调用，改意图冲突，新 key 显式推进下一轮，八路收敛测试连续 10 轮通过。注入 schedule-start 失败后 request 保留，过期 running schedule 可在同 request 下以 ordinal 2 恢复并把旧记录收敛为 `abandoned/worker_lost`。定向复核还覆盖操作员越权、Policy 拒绝零状态、原始 key/事件隔离、SQL 不可变、v37 原地升级、真实 CLI schedule/replay/continue/show 与普通工具旁路拒绝。代码审计未发现未解决的高/中风险问题。最终 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、凭据/运行产物扫描与 `govulncheck` 全部通过，可达漏洞为 0。隔离真实二进制仅注册 mock，完成 workspace 初始化、schema v38 Run 创建/列表并清理临时 runtime。

GitHub Actions CI Annotations 切片不增加 schema、Store mutation 或 Provider 调用。`GateResult` 在内存中保留 `json:"-"` 的精确 matched Finding 快照，JSON 继续只输出原有计数；`report check --format github` 直接把同一结果投影为带 workspace-relative file、line/endLine、title、status、category、Finding ID 和 fingerprint 的 workflow commands，并保持既有退出码。info/low 映射 notice，medium 映射 warning，high/critical 映射 error。属性严格转义 `%`、CR/LF、冒号和逗号，正文严格转义 `%` 与 CR/LF，其余 C0/DEL 转成可见 `\u00XX`，模型输出不能注入第二条 command 或操纵终端显示。Artifact 正文、validation/acceptance/remediation Evidence note 和 operator reason 不进入快照。定向测试覆盖 JSON 兼容、exact selection/order、私密叙述隔离、命令/控制字符注入、五种 severity、192 条上限与 193 条拒绝、pass/disabled 零输出、validated CLI failure annotation 和 fixed 零 annotation。审计未发现高/中风险问题，并补上两个低风险公共输出边界：人工构造的 GateResult 现在也受 report-wide Finding 上限约束，残余终端控制字符被可视化。最终 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、凭据/运行产物扫描与 `govulncheck` 全部通过，可达漏洞为 0。隔离真实 binary 只注册 mock，确认 github/unknown format 的 lookup 顺序、workspace 初始化和 Run 创建后清理全部临时数据。

React/Vite Read Console 切片不增加 schema、Go Store mutation、Provider 调用或新的 HTTP capability。`web/` 使用 React 19、Vite 8、TanStack Query、Zustand 和 Lucide；`openapi-typescript` 从受测的 `docs/openapi.json` 生成 DTO，GitHub Actions 会重新生成并阻止 drift。界面覆盖 Run/Session 列表与有界分页、Mission/预算/checkpoint/lease 摘要、WorkItem、Note、Artifact descriptor、Supervisor ToolRound、包含 compacted 状态的 Session message，以及实时 Run event。SSE 明确使用带 Bearer header 的 `fetch` 而非无法加 header 的原生 EventSource，以 frame id 对照 cursor，并通过 `Last-Event-ID` 续传。

安全边界复核确认 read token 只保存在页面内存，不进入 URL、localStorage、sessionStorage、日志或 Query key；断开连接会同时清理 token、选择和 React Query cache。API base 固定为同源 `/api/v1`，路径规范化后必须留在该前缀内，Vite 代理目标只接受 HTTP(S) 回环 URL。浏览器不接触 control token、Artifact 正文、checkpoint pending input、fencing token、Shell、Docker 或 SQLite，也不重写 Go Policy。审计阶段修复五项低风险健壮性/仓库卫生问题：effect abort 时的 reconnect delay 不再产生 detached rejection；400/401/403/404 SSE 停止重试，避免无效轮询；分页 envelope 在运行时强制 data 为 array；SSE 同时校验外层 id、frame cursor、Run ID 与 event/frame sequence；TypeScript 增量构建缓存不会进入 Git。未发现未解决的高/中风险问题。

前端门禁通过 strict TypeScript、11 个 Vitest/Testing Library 测试、生产构建和 `npm audit --audit-level=high`，npm 漏洞为 0，bundle 主脚本约 80 KiB gzip。隔离的 schema v38 runtime 创建了 workspace、Run、WorkItem、Note 和两条 Session message；真实 Vite -> Go 代理 smoke 读取 health、Run 列表并接收 7 帧 SSE。浏览器验证 Run 概览、实时事件和 Session 消息，1440x900 与 390x844 均无横向溢出，控制台 error 为 0。当前进度估计保持为整体愿景约 97%、v0.1 通用 Agent MVP 约 99%、V2 Runtime 约 99%；该切片完成了 P9 第一版 Web 读取面，后续切片已补齐同源托管与 Agent/delegation/Fan-out/Finding 投影，Web 控制操作仍未开放。

首次 Linux GitHub Actions 运行中，TypeScript job 21 秒全绿，Go job 则暴露既有 heartbeat 时间边界：短 `RenewInterval` 同时被用作 SQLite 续租调用的完整 timeout，在双核 runner 高负载下会浪费 `TTL - interval` 的剩余安全窗口并过早取消 Run。实现现按租约失效余量设置续租 timeout，并保留 2 秒上限；长模型调用测试仍必须跨过原始 expiry 后证明租约活跃和第二 worker 被拒绝，只把 TTL/轮询调到跨 runner 可调度尺度。新增内部表测试固定 default/short/near-expiry 三种计算；定向普通测试 10 轮、race 3 轮均通过。该项属于低风险健壮性修复，不改变 schema、lease fencing、owner/generation、事件或 API。

Go Same-Origin Web Hosting 切片不增加 schema、Store mutation、Provider 调用、HTTP API route 或 control capability。新增 `internal/webui`，`api serve --ui-dir` 在打开 Store/listener 前通过 `os.Root` 校验真实目录、`index.html`、`assets/`、软链接、常规文件、允许类型、哈希文件名、256 文件、8 MiB 单资源和 32 MiB bundle 上限，再把内容读成不可变启动快照。HTML 使用 `no-store`；精确命中的哈希资源使用 SHA-256 ETag 和一年 immutable cache；只有明确接受 HTML、无扩展名、无点前缀且最多八段的规范路径可走 SPA fallback，缺失 `/assets` 与扩展名路径始终 404。

HTTP 复核确认 `/api` 命名空间继续使用原 read/control Bearer 边界，UI 静态请求也必须通过 loopback listener/Host/client、request target 和规范路径检查，只允许无 query/body/Authorization 的 GET/HEAD。UI CSP 不含 `unsafe-inline`/`unsafe-eval`，并启用 same-origin opener/resource policy、`nosniff`、frame deny、no-referrer 与受限 Permissions Policy；静态 handler panic 只返回通用文本。为满足 CSP，预算条从 React inline style 改为原生 `progress`。定向测试覆盖 API 鉴权不弱化、reserved typo、外部 Host/client、方法/query/body/token 拒绝、panic 隔离、不可变磁盘快照、HEAD/ETag、fallback、恶意 bundle 与真实 CLI 启停。代码审计未发现未解决的高/中风险问题。

真实 Vite 生产 bundle 由单个 Go 进程在 `127.0.0.1:8765` 提供；HTTP smoke 覆盖匿名 HTML、immutable JS、Bearer API、SSE 数据、SPA Accept 分流、query/token/method 拒绝、伪造 Host、HEAD 与 304。浏览器在 1280 和 375px 视口完成 token 连接、Run 概览、事件流和预算进度条复核，无 document 横向溢出；页面内唯一的 inline style 来自 Codex 浏览器注入层，不属于 bundle。最终 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、OpenAPI drift、strict TypeScript、11 个 Vitest、生产构建、npm audit、凭据/运行产物扫描与 `govulncheck` 全部通过，Go/npm 可达漏洞均为 0。当前进度估计保持为整体愿景约 97%、v0.1 通用 Agent MVP 约 99%、V2 Runtime 约 99%。

Run Runtime Read Projections 切片不增加 schema、HTTP mutation、Provider 调用或 capability。Go 新增五个只读路由：Agent graph、delegation、Fan-out plan/latest execution、Finding report list/detail；OpenAPI 扩展为 24 paths、45 schemas，并重新生成 TypeScript DTO。Store 查询使用有界 `LIMIT/OFFSET`；Fan-out 摘要 SQL 从源头不读取 raw model report、snapshot/input/report digest、error reason、lease ID 或 generation。Finding DTO 只公开断言事实、来源序号、Artifact metadata 和生命周期时间，不公开 Artifact 正文、Evidence note、operator reason/identity 或 Evidence-set digest。旧 Run 没有物化 root 时返回合法空图而不是冲突。

React 增加 Agents、委派、Fan-out、发现四个紧凑 Run 标签页，状态机、权限和脱敏仍完全由 Go 所有。后端投影/隐私/分页测试、OpenAPI forbidden-field/golden/live-route 测试与前端 populated/empty-state 测试通过；前端现有 5 个测试文件共 13 项。审计修复一项低风险数据库漂移边界：Fan-out summary 现在完整复核 pending/running/terminal shard 的 provider/model/error/usage/timestamp 组合，外部篡改不能把状态不兼容的元数据投影到 Web。真实 schema v38 数据在单个 Go 进程上显示一个 root Agent 和一个不执行 Provider 的只读 Fan-out 计划，委派/发现空状态正确；1280 与 390x844 视口无 document 横向溢出、console error 为 0。最终 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、strict TypeScript、生产构建、npm audit 与 `govulncheck` 全部通过，Go/npm 可达漏洞均为 0；代码审计未留下已知高/中风险问题。当前进度估计保持为整体愿景约 97%、v0.1 通用 Agent MVP 约 99%、V2 Runtime 约 99%。

Headless NDJSON 切片不增加 schema、Run mutation、Provider、Tool Gateway、Sandbox、网络或文件能力。新增 `internal/headless` 与 `cyberagent headless events`，以 `headless.v1` 逐行输出 Store 已脱敏的持久化 Run events 和最终 `stream.end`。`--after-sequence` 支持数字 sequence 续传；未来 cursor 在 stdout 前拒绝。每批最多读取 100 条，默认最多 1,000、硬上限 10,000，event identity/type/source/payload 另受 256-rune/1 MiB 投影限制；sequence gap、Run/Mission mismatch、无效 JSON 或超界记录 fail closed。普通 snapshot 不等待；`--follow` 以 50ms-5s 的有界间隔轮询本地 SQLite，终态判断前再探测一条尾事件，避免漏掉同事务中的最后记录。

机器出口现固定为 completed 0、failed 4、cancelled/caller-cancel 7、max-events 8、timeout 9；非终态单次 snapshot 返回 0。所有正常/终态/上限/取消/超时路径都会先写包含 status/reason/count/resume/exit_code 的 `stream.end`，stdout 不混入人类文本，stderr 继续使用既有 `apperror` 文本。审计修复一项低风险投影绑定遗漏：每条事件和后续 Run 刷新现在同时钉住初始 Mission ID，外部 SQL 造成的跨 Mission 漂移会在输出前 fail closed。定向普通/race 测试覆盖顺序、未来 cursor、截断恢复、writer failure、超界 metadata、取消/超时和 follow 期间追加终态事件；follow 与 CLI 各连续 10 轮通过。隔离真实 schema v38 二进制复核 0/4/7/8/9、逐行 JSON、resume 完整性及多次读取前后 timeline 尾序号不变，并清理临时 runtime。代码审计未发现未解决的高/中风险问题。当前进度估计保持整体愿景约 97%、v0.1 通用 Agent MVP 约 99%、V2 Runtime 约 99%。

Event-driven Bubble Tea 切片不增加 schema、Provider 调用、工具执行、网络路由、Sandbox capability 或 Store mutation。TUI 现在按最多 32 条一批轮询同一 SQLite Run event sequence，保留最近 50 条 metadata-only 事件，校验连续序号、Run/Mission 绑定、UTF-8 与字段上限；异步旧结果不会回滚刷新后的 cursor，Run 进入终态后停止轮询。新增 Events、Agents 和 Findings 三个读取视图：Agent 最多展示当前有界图的三个节点及 Completion 摘要，Finding 最多展示十份 Report 摘要及该窗口的 Finding 汇总；事件 payload、完整 Finding/Evidence 与私密操作员叙述不进入终端。

复合刷新在读取 Session message、ToolRun、WorkItem、Note、Agent/Completion 和 Finding summary 前后复核 event tail，若并发 mutation 改变尾序号则最多重试八次，避免界面显示“新 cursor + 旧表格”；超过单批 32 条时直接采用完整稳定快照，避免“更晚表格 + 中间 cursor”。针对性测试在首次 WorkItem 读取后注入事务，确认第二次读取收敛；还覆盖 event gap、跨 Mission、超界 metadata、终态停轮询、旧结果丢弃、50 条窗口、只读标签不能触发批准，以及 C0/DEL/ESC 可视化。新增同一真实 Run 的跨界面 golden：CLI 创建并启动 Run，再比较 CLI JSON、TUI projection、带鉴权回环 HTTP 和 Headless NDJSON 的 Run/Mission/Session/status/event tail/Agent count。审计发现并修复同一项中风险复合快照撕裂的两种表现，以及两项低风险问题：事件触发时不再读取完整 Finding/Evidence，终端控制字符不再原样呈现；未留下已知高/中风险问题。

四项时序敏感 TUI 测试与跨界面 golden 各连续 10 轮通过，随后 race 模式各 3 轮通过；TUI package 覆盖率为 66.5%。最终门禁通过 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、凭据/运行产物扫描、OpenAPI 生成、13 个 Vitest、Vite production build、npm audit 与 `govulncheck`，Go/npm 已知可达漏洞均为 0。隔离真实二进制让 CLI/TUI/Headless 在 event tail 6 对齐并显示一个 root Agent，取消后 TUI 读取终态 tail 8，随后清理全部临时 runtime。当前进度估计仍为整体愿景约 97%、通用 Agent MVP 约 99%、V2 Runtime 约 99%。

Run-first TUI 与只读 Diff 切片不增加 schema、Provider 调用、工具执行、HTTP 路由、Sandbox capability 或写权限。默认选择器现在通过 bounded Store page 分别读取最近 50 条 Run 和 Session，逐条验证 Run/Mission/Session 绑定、UTF-8 与字段上限；`tui --run` 会从 Run 解析 Session，再反向确认 TUI 投影仍是同一 Run。旧的无 Store 测试/兼容入口仍可显示 Session picker，但生产 SQLite 路径不再先加载无界 Session 列表。

Run 稳定复合快照新增最近 20 条 exact-Session/Workspace FileEdit preview。新 SQL 只选择 ID、scope、path、status、diff、hash、reason、redaction 与时间，不读取 `original_text`/`proposed_text`；TUI 内存投影随即清空原始 diff 字段，只保留终端安全的有界行。Edits 标签和全屏 detail 都只读，显示最多 128 KiB/4096 行；`a/g/d` 在该标签无法触发审批，detail 捕获普通按键且只允许滚动、返回或退出。窄侧栏会把当前标签放在首位，保证 `[Edits]` 不被截掉。

审计未发现未解决的高/中风险问题，并修复三项低风险健壮性/可用性问题：生产 picker 的旧 Session 读取改为有界 page，100 列标题改为优先显示 Run/status，窄活动栏保证当前标签可见。定向 Store/TUI/App 测试覆盖分页、非法状态、Run-first 精确打开、scope 校验、正文排除、Diff 字节/行截断、终端控制字符和非 Tools 审批旁路。最终门禁通过 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、`govulncheck`、OpenAPI/TypeScript DTO 一致性、13 个 Vitest、Vite production build、npm audit、凭据/运行产物扫描与真实二进制 smoke；Go/npm 已知可达漏洞均为 0，TUI 覆盖率为 65.9%。隔离 smoke 创建 schema v38 workspace/Run/FileEdit，确认 Run-first picker 与 `--run --print` 后清理全部 runtime。当前进度估计保持为整体愿景约 97%、通用 Agent MVP 约 99%、V2 Runtime 约 99%。

Cross-surface Lifecycle/Pagination Golden 切片不增加 schema、Provider 调用、工具执行、HTTP route、Sandbox capability 或 Store mutation。一个真实 schema v38 SQLite fixture 同时建立 running、paused、completed、failed、cancelled 五类 Run，再扩展到 53 条 Run/Session；CLI、TUI stable projection、带鉴权 HTTP 和 Headless NDJSON 必须逐条同意 Run/Mission/Session/status、完整连续 event sequence/tail、Agent count、terminal 标志、reason 与 0/4/7 outcome exit。TUI picker 的稳定只读投影返回防御性副本，并固定最近 50 条与 truncation；HTTP 以 20/20/13 三页验证 opaque cursor、顺序、无遗漏、无循环及合法空集合；Headless 以每页 2 条验证 max-events 8、suggested resume、无重复/无 gap，以及从 durable tail 恢复时只输出零事件 `stream.end`。

React client 现验证 opaque cursor 只进入 query、bearer 只进入 Authorization；ResourceSidebar 回归覆盖 paused/completed/failed/cancelled 徽标并追加下一页 running Run。前端测试由 13 项增至 15 项。审计未发现未解决的高/中风险代码问题；修复一项低风险文档恢复缺口：README 已引用但仓库缺失的 `docs/PROJECT_MEMORY.md` 现已补齐。最终门禁通过 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、OpenAPI/TypeScript DTO 漂移检查、15 个 Vitest、strict TypeScript、Vite production build、npm audit、凭据扫描与 `govulncheck`；Go/npm 可达漏洞为 0。当前完成度保持整体愿景约 97%、通用 Agent MVP 约 99%、V2 Runtime 约 99%。

## 七、下一开发切片

Go-owned `skill.v1` 第一纵向切片不增加 schema、Store 写入、Provider 调用、prompt 注入、工具执行或权限。`internal/skills` 以内嵌 FS 注册 `code/review/learn/script` 四份元数据，固定 core semantic version、Profile、窄工具前置依赖、slash-relative Markdown 路径、UTF-8 字节数、保守 token 上界和 SHA-256。只读 Registry 没有 Register/Update/Content API，返回切片均为防御性副本；`skill list/show/validate` 不打开 SQLite，也不接受任意外部路径，并明确输出 `context_injection: disabled` 与 `tool_capability_grant: disabled`。

定向测试覆盖严格 JSON unknown/duplicate/trailing、非法 UTF-8、Profile/工具提权、排序与重复、路径逃逸、Windows 路径、根目录/内容 symlink、非普通文件、字节/token/hash 漂移、内部状态漂移、防御性副本、CLI Profile 过滤、稳定 exit code 和零运行数据。审计未发现未解决的高/中风险问题，并修复三类低风险边界：显式拒绝 Go JSON 默认容忍的非法 UTF-8 与重复字段，检查 Registry 根路径 symlink，并按名称稳定执行 Registry 自检。`internal/skills` 语句覆盖率为 86.3%；uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、`govulncheck`、OpenAPI/TypeScript 漂移检查、strict TypeScript、15 项 Vitest、Vite production build、npm audit、凭据/运行产物扫描和真实二进制 CLI smoke 全部通过，Go/npm 已知可达漏洞为 0。

P7 第二纵向切片新增 schema v39 与不可变 `skill_selection.v1`。操作者只能在 `created` Run 上选择 1-8 个与 Mission Profile 兼容的内嵌 Skill；Go 按名称排序并固定 version/content SHA-256/字节数/token 上界，SQLite 再次约束 Run/Mission/Profile、连续 ordinal、总额、唯一 Run 选择和 update/delete 不可变。原始 operation key 只保存域分隔摘要；相同意图在两条 SQLite 连接八路并发时收敛到同一 selection，Run 启动后仍可精确重放，改意图或新选择失败关闭。重放指纹只绑定 Run、Profile、预算、名称和操作者，因此内嵌 Registry 未来升级或移除旧版本时仍能读取已固定结果；selection 自身的独立指纹继续锁定原版本与内容哈希。

`skill select/selection` 是唯一写入与读取入口，模型、HTTP、Tool Gateway 和 child scheduler 均无创建能力。事件只含协议、Profile、数量、预算及 `context_injection=false`/`tool_capability_grant=false`，不含名称、正文、路径、内容哈希、工具依赖或原始 key。定向测试覆盖 Profile/预算/重复名称、低报 token、SQL 不可变、事务回滚、v38 原地升级、Run 启动后重放、Registry 漂移重放、operation 身份/时间戳漂移、重复 JSON 字段拒绝、CLI 稳定退出码和并发收敛；并发用例连续 20 轮通过。最终门禁通过 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、零漏洞 `govulncheck`、OpenAPI/TypeScript 漂移、15 项 Vitest、生产构建、npm audit、凭据/运行产物扫描和隔离真实二进制 schema-v39 选择/重放 smoke。审计未留下高/中风险问题，并修复 CLI 校验原因被隐藏、重放依赖当前 Registry、应用层未独立复核 operation 身份/时间戳、事件重复字段歧义和历史迁移夹具漏删 v39 五项健壮性缺口。

P7 第三纵向切片新增 schema v40 与 root-only `skill_context.v1`。RunSupervisor 每个 turn 只读取持久化的 `skill_selection.v1`，再由内嵌 Registry 逐项复核 name/version/content SHA-256/bytes/token 上界与 Profile；正文先经过通用 secret redactor，再按 selection 的独立预算和稳定名称顺序在内存中组装。四份内置指导由会误导模型的占位文本升级为 `1.1.0` 最小工作流，分别覆盖 scoped code、severity-first review、stepwise learn 和 bounded script。Registry 增加每 Skill 最多 8 个版本的内嵌历史索引：新 selection 只看当前版本，旧 `1.0.0` selection 仍按原 version/hash/bytes 精确恢复，不静默套用新正文，也不接受外部历史路径。

schema v40 只持久化 `root_skill_context_preparations/commits` 元数据。准备绑定 Run/Mission/root Agent/Supervisor attempt/turn/selection 和 context fingerprint；第一次 `model.started` 在同一事务提交 context commit，注入 model-start 失败时 commit 一并回滚。第二条 SQLite Store 会恢复同一 preparation，重建 fingerprint 漂移返回 conflict，选择存在但未准备时模型启动失败关闭。`skill.context_prepared/committed` 事件只含 agent、turn、数量、预算、脱敏计数、root-only 与 capability=false，不含正文、路径、Skill 名称/版本/哈希或工具前置声明。Provider request 的工具列表不因 Skill 变化，Registry 缺失会在调用 Provider 前失败；Specialist 尚不接收 Skill。

功能复核覆盖 deterministic current/archived assembly、secret redaction、内容篡改、Registry 漂移、SQL update/delete 不可变、prepare replay、双 Store 恢复、原子 commit/rollback、缺失 prepare 拒绝、v39 原地升级、root prompt 实际交付、metadata-only 事件/表结构、零工具授权和 fail-closed Registry loss；Store 与 Application 定向用例各连续 10 轮通过。审计未发现未解决的高/中风险问题，并修复四类低风险缺口：历史降级夹具未先卸载 v40、内置正文仍声称“不注入”、新增错误文本不符合 staticcheck 小写约定，以及升级正文会让既有固定版本无法恢复。首次 Linux CI 还暴露一项既有测试时序假设：双核 runner 忙时 API 子进程可能无法在固定 4 秒内输出启动元数据；共享测试 helper 现使用有界 15 秒就绪窗口、进程提前退出立即失败并区分 stdout/stderr，生产启动和 4 秒 shutdown 契约不变。三个 API 启动用例连续 10 轮、生成 token 用例 race 连续 3 轮通过。最终门禁通过 uncached 全仓测试、全仓 race、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、零漏洞 `govulncheck`、OpenAPI/TypeScript 漂移、15 项 Vitest、Vite production build、npm audit、凭据/运行产物扫描和隔离真实二进制 schema-v40 Skill context smoke。

P7 第四纵向切片先完成 Plan/Delivery 的 Go-owned 模式地基，而不是把安全边界交给提示词。schema v41 为每个 Run 原子创建 `run_mode.v1` 快照：工作面固定为 `code|cyber`，阶段为 `plan|deliver`，并绑定 Mission、Profile、不可变 Scope、协议和策略版本。旧 v40 数据与未显式提供新字段的调用方统一回填/默认 `code/deliver`。工作面在一个 Run 内不可变；阶段变更需要摘要化稳定 operation key，只允许 `created` 或无活动 execution lease 的 `paused` Run，相同意图跨进程重放收敛，改意图冲突。

RunSupervisor 在开始 turn 的 SQLite 事务中读取权威模式，并在 Skill 之前加入 Go 生成的模式契约。Plan 只允许分析、WorkItem/Note 与交接，不允许完成；模型 `finish` 使用现有一次协议修复并必须收敛到 `continue/wait`，操作者完成和 Store 最终写入也分别失败关闭。`run create --surface/--phase`、`run mode`、`run phase`、Run list/show、TUI header、HTTP/OpenAPI、生成的 TypeScript DTO 和 React overview 均投影同一 revision。模式明确声明 `capability_grant=false`，没有新增 HTTP 写操作、模型模式切换、Shell、网络、文件写入、Sandbox 执行或 child 权限。

本轮代码与安全审计未发现未解决的高/中风险问题，并在发布前修复八项低风险健壮性缺口：迁移回填 ID 不再拼接可能超长的旧 Run ID；活动租约比较使用 Store/SQLite 当前时间而非调用方时间戳；execution lease 在 Go 与 SQLite 两层阻止阶段变化；模型完成事务、操作者完成事务和 SQLite Run 状态 trigger 都重新检查 Plan；未脱敏模式字段不能穿过 Store；Store 重新计算 operation fingerprint，不能信任上层传值；script-process 幂等意图纳入完整模式 tuple；系统时钟回拨时 transition 时间钳位到上一 revision。测试覆盖领域枚举/快照、迁移回填和不可变 trigger、创建原子性、重放/冲突、双 Store 并发、未来时间戳绕过拒绝、Store 脱敏边界、Plan finish 修复、两条完成路径、CLI 以及 CLI/TUI/HTTP/Web 跨入口一致性。

最终发布门禁通过全仓 `go test ./...`、全仓 `go test -race ./...`、`go vet`、零告警 `staticcheck`、`go mod verify/tidy -diff`、零漏洞 `govulncheck`、OpenAPI/TypeScript 确定性再生成、strict TypeScript、6 个 Vitest 文件 15 项测试、Vite production build、零漏洞 npm audit 和凭据模式扫描。隔离真实二进制 smoke 仅使用 mock Provider，验证 `cyber/plan` 创建、Plan 完成拒绝、显式切换 `deliver`、执行一轮、CLI list/show/mode 与 TUI 一致投影；临时 runtime 已删除，没有调用真实模型、Shell、网络或 Sandbox。

P7 第五纵向切片新增 schema v42 与 Go-owned `plan_delivery.v1`。root 模型只可在活动 Plan turn 中调用 `plan_delivery_propose`，提交恰好三个有界方向；每个方向包含 1-8 个有序模块、验收条件、权衡与只能引用前序模块的依赖。Gateway 在任何预算扣减前执行严格 JSON/UTF-8/大小/重复项校验，再绑定 root Agent、Supervisor attempt、活动 execution lease、Run/Session/Workspace Scope、当前 Plan revision、Policy 和工具预算。提案结果始终声明 selection、phase change、execution 与 capability 均未授权。

操作者只能在 Run 暂停且 lease 释放后，用 `run plan choose <proposal> 1|2|3 --operation-key ...` 选择方向。16-256 字节规范化 key 只以域分隔摘要落库；同一事务创建不可变 selection、所选 WorkItem 依赖图、置顶 decision Note、operation 与元数据事件。两条 SQLite Store 并发同 key/同意图收敛到同一组 ID，改方向或操作者冲突。选择后 Run 仍为 `plan`，必须另行执行 schema v41 的显式 `run phase ... deliver`；该选择不是审批或 capability grant。

CLI 新增 proposal 列表/详情/选择/selection 查询；HTTP/OpenAPI、TypeScript DTO、React overview 与 Bubble Tea Plan 标签只读显示三个方向、选择、WorkItem 映射和是否仍需 Deliver。公共投影不包含原始 key、operation/fingerprint、root/requester 内部身份、lease/fencing 或模型正文。内置 `plan-delivery` Skill 作为第五份 `1.1.0` 指导跨 `code/learn/review/script` Profile 可选，但即使未选择该 Skill，Go 生成的 Plan 协议仍然生效；Skill 不增加工具或权限。

本轮审计未发现未解决的高/中风险问题，并在收口时修复八类低风险完整性与隐私缺口：协议版本前后空白不再被默许；重复依赖不再被静默去重；方向与模块标题在 Go 和 SQLite 双层拒绝重复；operation key 与 operator identity 使用精确规范化语义并拒绝空白/控制字符；`plan_delivery_propose` 在进入 Tool Gateway 和预算扣减前即被限制为 Plan phase，越阶段调用只进入一次有界协议修复；内部 proposal fingerprint 不再进入 Run event 或交接 Note；CLI 展示的模型文本被折叠为单行，不能注入额外终端行；最大合法方向标题和极端文本不再产生无法持久化的交接 Note，稳定编号标题与领域层字节证明保证每个已接受方向都可选择。测试覆盖领域协议、Gateway、proposal->wait->choose 端到端、双 Store 十轮并发、重放/冲突、活动 lease 与过期 revision、v41 原地升级、直接 SQL 篡改、HTTP 隐私、TUI 非 Tools mutation 拒绝和 React 只读渲染。

schema v42 本地发布门禁通过：`go test -count=1 ./...`、全仓 `go test -race -count=1 ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、无差异 `go mod tidy -diff` 和零可达漏洞 `govulncheck ./...`；Application Plan 用例连续 10 轮、Store Plan 用例连续 3 轮、交接 Note 极限边界连续 10 轮通过。OpenAPI/TypeScript 两次生成 SHA-256 一致；strict TypeScript、7 个 Vitest 文件共 16 项测试、Vite production build 与零高危 npm audit 均通过。凭证扫描仅命中两个 `_test.go` 固定鉴权夹具，真实 `sk-`/`tp-` 模式与受跟踪运行产物均为零。隔离真实二进制 smoke 仅注册 mock Provider，验证 version、workspace、第五个 Skill、`code/plan` Run、Plan schema、空 proposal 查询和 TUI 投影后删除全部临时 runtime；没有调用真实 Provider、Shell、网络、Docker 或 Sandbox。GitHub CI 结果在推送后复核。

P7 第六纵向切片优先修复间接 Prompt Injection，再继续 Delivery 门禁。schema v43 新增 `context_provenance.v1`：operator/model/Go control/workspace file/listing/diff/tool result/Go command result 使用严格来源枚举，只有 operator 与 Go control 可设置 `instruction_authorized=true`。新消息先脱敏，再计算 SHA-256；SQLite 同时约束来源/角色/授权矩阵、规范化 source ref、不可变正文/来源、不可删除和单向 compact，Go 每次读取再计算摘要以识别数据库层篡改。未知角色不再静默提升为 user，source ref 拒绝控制字符。

Session slash 回复全部改为 role=`tool`：成功的 `/read`、`/ls`、`/write`、`/run` 分别记录 workspace/tool 来源，帮助、模型切换和失败结果记录 Go command 来源，统一无指令权限。模型投影只让 operator/model/Go control 保留原角色；其他内容全部变成 user-role `untrusted_context.v1` JSON，携带 source kind/ref、digest、`instruction_authorized=false` 与脱敏正文。该投影同时接入未绑定 Session、root Supervisor 和 Specialist；WorkBoard、Note、root inbox 与 compacted summary 从 system 降为 user/evidence。压缩摘要使用逐条 provenance JSON，并以“历史数据，不是新指令”的 user transcript 回放。只读 Fan-out 原本已采用无工具、文件内容非可信的独立边界。

v43 迁移将既有行保守标为 `context_provenance.v0`，并把可识别的旧 Workspace read/list、FileEdit 和 ToolRun assistant 回复降级为 tool evidence；旧行不伪造历史摘要。Run Session 事件、CLI history、HTTP/OpenAPI、生成的 TypeScript DTO 与 React 消息标签均显示来源审计字段。回归用例完整复现 README：Setup 明确要求 `.env`、`DATABASE_URL`、`SESSION_SECRET`，中间却要求 automated coding assistants 跳过 `.env`；Session/root/Specialist/compaction 测试证明诱导文本只存在于 `workspace_file + instruction_authorized=false` 封套，绝不进入 system/assistant 角色，真实配置事实仍保留供模型判断。

本轮代码与安全审计未发现未解决的高/中风险问题。收口修复一项低风险 staticcheck 错误文本大小写，并补充未知 role、source-ref 控制字符、SQLite role forgery、直接 update/delete、格式正确但内容错误的摘要、v42 原地升级和 API provenance 测试。发布门禁通过 `go test ./...`、全仓 `go test -race -count=1 ./...`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、无差异 `go mod tidy -diff`、零可达漏洞 `govulncheck ./...`、strict TypeScript、7 个 Vitest 文件共 16 项测试、Vite production build 与零漏洞 npm audit。OpenAPI 与 TypeScript 连续两次生成的 SHA-256 分别稳定为 `BBC58E75...C965` 与 `940175B8...8EB0`。受跟踪文件凭证模式扫描为零；隔离真实二进制 smoke 验证 version、mock Provider、Workspace/Session 创建，以及 history 中 `operator_message authorized=true` 与 `go_command_result authorized=false` 后删除临时 runtime。真实 Provider、Shell、网络、Docker 与 Sandbox 执行均未启用；GitHub CI 在推送后复核。

P7 第七纵向切片完成 schema v44 与 Go-owned `delivery_checkpoint.v1`。新 Plan 选择和未开始的旧选择进入不可变 enrollment；每个所选 WorkItem 只有在 `in_progress`、Run 暂停、当前阶段为 Deliver 且没有活动 execution lease 时，才能由操作者记录检查点。检查点固定 selection/proposal、方向与模块、验收条件和源投影指纹、当前 mode snapshot/revision、WorkItem version，并要求聚焦验证、Diff 审计、安全审计与交接摘要。最后一个所选模块是确定性的较大边界，额外强制整体验证和健壮性审计；非最终模块反而拒绝这两项，避免边界含义漂移。

一次 SQLite 事务创建 checkpoint、摘要化幂等 operation、置顶 summary Note、Note 关系和两条元数据事件。WorkItem 完成、root `finish` 与显式 Run finalize 都消费同一门禁事实；Go Store 与 SQLite trigger 分别校验当前 Deliver revision、item version、operation 账本和完整 selection。checkpoint、operation、Note 正文以及 tag/source/evidence 关系均不可更新或删除。相同 key/意图在两条 Store 八路并发下收敛，改证据冲突；重启后可以精确重放。迁移遇到已完成或取消切片的旧 selection 时保持 `delivery_gate_enforced=false`，不伪造历史审计，也不影响旧 Run 收尾。

CLI 新增 `run delivery checkpoint/list/show` 唯一写入/检查入口；HTTP/OpenAPI、生成 TypeScript DTO、React Plan 面板和 Bubble Tea Plan 面板只显示是否强制、必需/就绪数量及有界 checkpoint 元数据。公共投影不含验证/审计正文、内部指纹、摘要、operation key 或 requester。模型没有 checkpoint 工具；Policy 对 Shell/Sandbox/process/script 中明显自调用 operator 命令的请求永久拒绝。审计未发现未解决的高/中风险问题，并在收口修复四个低风险边界：固定编号标题保证最大合法模块标题不会撑爆交接 Note；关系 UPDATE trigger 同时检查 OLD/NEW note 归属，不能把其他 Note 的关系搬入；Run event 删除不必要的验收/来源/交接摘要；CLI 将该事实表述为门禁就绪而不是能力授权。

v44 发布门禁覆盖领域协议、应用端到端、两 Store 并发、SQLite 直接篡改/迁移、两条完成路径、真实 CLI 生命周期、Policy、HTTP/TUI/React 只读投影和 OpenAPI 隐私。全仓普通/race 测试、vet/staticcheck、模块一致性、`govulncheck`、strict TypeScript、16 项 Vitest、Vite production build、npm audit、凭据/运行产物扫描与确定性契约生成在提交前全部通过。GitHub Actions run `29280076450` 对提交 `0fa5ee3` 复跑后，Go control plane（1 分 58 秒）与 TypeScript console（20 秒）均成功；真实 Provider、Shell、网络、Docker 和 Sandbox 执行仍关闭。

P7 第八纵向切片完成 schema v45 持久化操作者引导队列。每条消息在 Store 边界脱敏、规范化并计算 SHA-256，单条最多 16 KiB；每个 Run 最多保留 64 条、256 KiB pending 输入。`operator_steering_operations` 只保存域分隔 operation-key 摘要和请求指纹，同 key/同意图在两条 SQLite 连接并发时收敛，同 key/异意图冲突。消息正文、操作者与创建时间不可修改，状态只能从 pending 单向进入 committed/cancelled；Run/Session 绑定和完成门禁同时由 trigger 复核。

Supervisor 在开始 turn 的事务中只取最早消息，并创建绑定精确 attempt/turn/PendingInput 的 prepared delivery。模型或工具轮失败时，旧 delivery 进入 superseded，同一消息在新 attempt 重新准备；成功时 operator user message、assistant reply、delivery commit、queue commit 和 lifecycle action 原子提交，Session 不会提前出现半条历史。队列仍有后续输入时，模型请求 `finish` 或 `wait` 只作为 requested action 审计，effective action 由 Go 改为 `continue`。显式完成和普通 Run 状态更新都拒绝遗留 pending；fail/cancel 会事务化取消 remaining queue。

CLI 新增 `run steer enqueue/list/show`，普通 Run-bound `session send` 在活动 lease、started/failed pending attempt 或已有队列时自动排队。Bubble Tea 主动作 busy 时仍接受普通文本并使用独立完成消息，不能借排队清除 busy/live 状态；斜杠命令仍拒绝并发。HTTP/OpenAPI、React Overview 和 TUI Queue 标签只投影 pending/prepared/committed/cancelled 计数、ID、sequence、status 与时间，不含正文、摘要、requested_by、Session 或 Session-message 关联，也没有 Web mutation。公开 OpenAPI 保持 24 paths，schema 从 53 增至 55。

功能与健壮性审计覆盖双消息顺序、enqueue 精确重放/冲突、原始 key 不落库、首轮失败恢复、finish 延后、两条 Session pair exactly-once、事件隐私、busy-only enqueue、终态取消、双 Store 并发收敛、Session 并发发送、暂停 Run 行为、CLI/TUI/HTTP/React 投影和生成契约。收口修复三项低风险问题：暂停 Run 已有队列时不再在 durable commit 后自动 resume 并可能返回误导性错误；queue event 删除 requester identity；`run steer list` 对未知父 Run 返回 `NOT_FOUND` 而非伪装为空队列。全仓普通/race 测试、vet/staticcheck、模块验证、零可达漏洞 `govulncheck`、strict TypeScript、8 个 Vitest 文件共 17 项测试、Vite production build、零漏洞 npm audit和隔离真实二进制 pending-to-committed smoke 均通过；临时 runtime 已删除，真实 Provider、Shell、网络、Docker 与 Sandbox 未启用。

GitHub Actions run `29310437643` 对发布提交 `022b083` 远端复核成功：Go control plane 用时 3 分 10 秒，TypeScript console 用时 23 秒；两个 Job 的测试、静态检查、漏洞/依赖审计、API 同步与生产构建均通过。

最终 CI 审计发现 `govulncheck-action` 会在同一 Job 已恢复 Go 缓存后再次 checkout 并恢复另一 Go 版本的缓存，扫描虽成功但产生大量 `tar: File exists` 注解。工作流关闭该复合 Action 内部的重复 checkout/cache，保留 Job 前置缓存和漏洞扫描，不降低发布门禁。

P7 第九纵向切片完成 schema v46 操作者引导队列控制。`operator_steering_cancellations` 为每条消息最多记录一份不可变事实；人工取消还必须绑定 `operator_steering_cancellation_operations` 中的域分隔 operation-key 摘要和完整意图指纹。相同 key/意图跨进程重放返回同一事实，改理由、操作者或消息冲突。只有未 prepared 的 pending 消息可由操作者取消；prepared、committed、cancelled 消息仍不可改判，正文和顺序继续不可编辑。Run fail/cancel 在生命周期事务更新终态后，为剩余消息记录 `run_terminal` 事实并关闭 prepared delivery 与 pending message；超长、无效 UTF-8 或含异常控制字符的系统失败原因会先脱敏、修复和按 UTF-8 边界截断，审计文字不能反过来卡死终态。

`run steer drain` 是显式、默认单步且最多 64 步的本地执行操作。它先获得现有 Run execution lease，之后才可唤醒 paused Run；其他 worker 持有租约时返回 conflict 且 Run 保持 paused。新的 steering-only Supervisor begin 只恢复精确 prepared steering 或准备最早 pending 消息，队列为空时不创建默认 Mission turn，普通失败 PendingInput 也不会被 drain 越权恢复。每个实际 turn 仍经过原有 token/turn/time 预算、Policy、Tool Gateway、生命周期协议和 fencing。`session send --operation-key` 为 Run-bound 普通文本提供跨进程稳定重试：无论 Run 是否忙碌都只 enqueue/replay，不同步调用 Provider；重启后及消息 committed 后仍可精确重放，异意图冲突，斜杠命令和未绑定 Session 拒绝该标识。

CLI 新增 `run steer cancel/drain` 与 Session `--operation-key`。HTTP/OpenAPI、React 和 TUI 没有新增 mutation，继续只读显示 schema v45 元数据；模型、Tool Gateway 与 child Agent 没有取消、唤醒或 drain 入口。取消事件只含消息 ID、sequence、kind 和状态，不含原始 key、理由或 requester；原始 key 只形成摘要。定向复核覆盖取消重放/冲突、prepared 拒绝、终态关闭、长失败原因、SQLite 仅有事实而无 operation 的旁路拒绝、两 Store 并发收敛、v45 原地升级、租约冲突不唤醒、队列专用 turn、普通失败 turn 隔离、Session 跨重启/终态重放、显式空 key 和 exactly-once history。审计未发现未解决的高/中风险问题，并在收口前修复三项低风险行为缺口：终态理由原可能超过 cancellation 上限而回滚 Run；paused drain 原先可能先 resume 再因租约冲突失败；CLI 显式传入空 operation key 原可能退化为同步 turn。另按 staticcheck 清理一处等价 options 转换。

本切片最终本地发布门禁通过：多轮定向测试、uncached `go test ./... -count=1`、全仓 `go test -race ./... -count=1`、`go vet ./...`、零告警 `staticcheck ./...`、`go mod verify`、无差异 `go mod tidy -diff` 与零可达漏洞 `govulncheck ./...`。OpenAPI 再生成无漂移并保持 24 paths/55 schemas/零 steering mutation route；strict TypeScript、8 个 Vitest 文件共 17 项测试、Vite production build 与零漏洞 npm audit 均通过。受跟踪凭据模式与运行产物扫描为零，所有变更 Go 文件已格式化且 `git diff --check` 通过。隔离真实二进制跨多个进程完成 Run create/start/pause、Session enqueue/replay、cancel/replay、第二条消息 drain 和最终 `pending=0/committed=1/cancelled=1`，只注册 mock Provider，临时 runtime 已验证路径后删除。GitHub Actions run `29316182580` 对发布提交 `5559f76` 远端复核成功：Go control plane 用时 1 分 51 秒，TypeScript console 用时 19 秒，全部步骤通过。

P7 第十纵向切片完成 schema v47 Specialist Skill 最小化。`specialist_skill_context.v1` 只从父 Run 已固定的 `skill_selection.v1`、当前不可变 `run_mode.v1` 与 Go 内嵌 Registry 派生；child assignment 仅以 Agent/parent/Profile/预算/能力指纹参与来源绑定，不能选择或扩大上下文。Code 工作面只取与 Profile 同名的 `code/review/learn/script`，Cyber 工作面对 Code/Review/Learn 返回空集合，仅 Script Profile 可取得 `script`；`plan-delivery` 永不进入 child。每 Attempt 最多一项，默认预算 1024、硬上限 2048 conservative tokens，且必须已有 delegated `model.chat`。

schema v47 新增不可变 `specialist_skill_context_preparations/commits`。八路并发准备收敛到同一记录，后续重放返回 recovered；首次 `specialist_model_calls.started`、Skill commit、commit event 与 model event 在同一事务中提交，任何后续事件失败都会整笔回滚。选择存在但未准备时模型启动 fail closed；无历史选择的旧 Run 保持 no-Skill 兼容。两张表和事件只保存 Run/Mission/Agent/Attempt/mode、数量、预算、脱敏计数和指纹，不保存正文、路径、Skill 名称、版本或 source/delivered content hash。核心最多两个 child、1/2/4/6 只读 Fan-out、无工具 Specialist、Policy、Scope、审批和总预算均未放宽。

本切片审计未发现未解决的高/中风险问题。收口新增未准备启动拒绝、注入事件失败原子回滚、准备/提交 UPDATE/DELETE 拒绝、事件字段白名单、schema metadata-only、事件顺序、v46 原地升级、Cyber 空集合和 Script-only、正文篡改与缺少 `model.chat` 测试。真实二进制 smoke 还发现并修正 `skill validate` 仍显示 root-only 的旧能力声明。最终本地门禁通过 uncached 全仓普通测试、全仓 race、vet、零告警 staticcheck、模块校验、零可达漏洞 govulncheck、OpenAPI 无漂移、8 个 Vitest 文件 17 项测试、Vite production build、零漏洞 npm audit、凭据/运行产物扫描和隔离 schema-v47 smoke。GitHub Actions run `29321708904` 对提交 `d7e269b` 复核成功：Go control plane 1 分 51 秒，TypeScript console 20 秒。

v47 发布后的远端 CI 复核暴露并修正两项既有并发假设。`ed1a2f1` 让阶段切换重放在其他 worker 已到达目标 phase 时再次读取持久化 operation，避免把成功并发提交误报为重复切换；`fa6dfbd` 将 Provider 取消测试改为等待真实调用入口，并规定已有 durable `model.started` 的 root 模型调用至少计费 1ms，关闭 `999/1000ms` deadline 可重复进入的预算边界。为保持 schema-v47 升级兼容，旧 Specialist 严格重放账本的 `0ms` 解释未被暗中改变。定向普通测试 100 轮、race 30 轮、全仓普通/race、vet、staticcheck、模块校验和 govulncheck 均通过；GitHub Actions run `29325171043` 对 `fa6dfbd` 一次通过，Go 2 分 53 秒、TypeScript 18 秒，无未解决高/中风险问题。

schema v48 Sandbox Manifest 切片已完成：严格解析、规范化指纹、Run/Workspace/Scope/Policy/审批/取消绑定、metadata-only 不可变账本、Policy 拒绝审计、同键/并发恢复、v47 升级、CLI 检查以及 Local/Docker fail-closed 均有测试。审计补强了非规范 CIDR、凭证形态 argv/环境字面量、输出挂载根、SQL 写能力审批门和恢复绑定；未发现未解决高/中风险问题，真实执行仍为零。Uncached 全仓测试、全仓 race、vet、staticcheck、模块校验/tidy diff、govulncheck、凭证/产物扫描、OpenAPI/TypeScript 漂移、17 个前端测试、生产构建、npm audit，以及隔离 schema-v48 CLI Run/preparation/event smoke 全部通过。

schema v49 Sandbox 审批/再校验切片已完成：共享审批请求与显式复核、完整 Manifest 重交、`os.Root` 防逃逸挂载解析、Scope/Policy/批准/预算/lease 双层复核、metadata-only 不可变候选、终态重放和跨 Store 收敛均已落地；候选仍不调用 Local/Docker。下一切片继续 P6，先定义禁用后端上的 lease-fenced lifecycle/cancel/cleanup 协议与输入/输出 Artifact 完整性边界，再独立审计 Docker mount/network/secret/kill/orphan 行为。Rust analyzer 与 CTF 求解仍后置。

schema v49 发布审计未发现未解决的高/中风险问题。收口修复了四类低风险健壮性边界：requester/reviewer 统一脱敏并拒绝控制字符；候选重放再次复核 operation 与候选本体摘要；SQL 直接写入必须匹配 preparation 挂载数量；v48 外部绑定审批不能被误解释为 v49 preparation-owned 批准。挂载循环响应 context 取消，双 SQLite 连接的审批请求与候选创建各连续并发收敛。最终门禁通过 `go test ./... -count=1`、`go test -race ./... -count=1`、`go vet`、零告警 `staticcheck`、`go mod verify`、无差异 `go mod tidy -diff`、零可达漏洞 `govulncheck`、OpenAPI 生成无漂移、strict TypeScript、8 个 Vitest 文件 17 项测试、Vite production build、零漏洞 npm audit、受跟踪凭证/运行产物扫描和隔离真实二进制 `prepare -> request -> approve -> candidate -> event` smoke。真实 Provider、Shell、网络、Local 和 Docker 均未执行。当前双指标保持：架构完成度约 97%，完整产品可用度约 45-50%（通用 Coding Agent 约 40%，Cyber 自动化约 20%）。

README 开发历程已完成顺序修复：项目简介恢复为简洁的中英双语产品说明，新增唯一权威的 `v1 -> v49` 双语 schema 时间线，早期 v1/v2/v3 不再缺失；原有长篇增量文字明确归入按能力阅读的架构详解，不再冒充时间线。新增 `TestREADMEListsEverySchemaVersionInOrder` 将 README 行数和顺序绑定到 `LatestSchemaVersion`。49 个英文里程碑逐项匹配真实 migration 名称，README 本地链接、diff check、定向测试、uncached 全仓测试、全仓 race、`go vet` 和零告警 `staticcheck` 全部通过；本切片未改变 runtime、schema 或安全权限，未发现未解决的高/中风险问题。

README 提交后的 Linux CI 暴露两条既有墙钟测试假设：Supervisor backoff 用 `150ms` 覆盖完整 Store/Provider 入口，慢 runner 会在首次 Provider call 前超时；active lease 也使用 `150ms`，导致第二次互斥断言执行前租约可能自然过期。test-only 修复改为等待持久化 `model.failed` 后显式取消，并先用一分钟 TTL 证明活动互斥，再由共享夹具构造时间轴合法、已经过期的 lease。backoff 普通 100 轮/race 20 轮和五条 takeover 路径普通 30 轮/race 10 轮通过，随后 uncached 全仓普通/race、`go vet`、零告警 `staticcheck` 全部通过。生产代码、schema、租约接管和取消语义均未改变，未发现未解决高/中风险问题。

schema v50 禁用态 Sandbox 生命周期切片已完成：`begin` 在消费 v49 candidate 前再次提交完整 Manifest，并复核 v48-v49 指纹、Run/Mission/Workspace/Scope、当前 Policy/批准、`os.Root` 挂载、总预算和 Run lease；Store 在同一写事务重复检查。每个输入 Artifact 按精确 Run/Session/Workspace、顺序、ID、SHA-256、大小、MIME、stream、source 和脱敏位绑定，总计最多 16 MiB；输出只保存 stdout/stderr 开关、路径数量、字节上限和指纹，不保存路径。独立 Sandbox lease 以 generation fencing 支持 released/expired 接管，初始 lease 仅准备禁用记录并立即释放，旧 generation 不能释放或提交。取消和清理为不可变摘要幂等事实，终态 Run 仍可清理；当前唯一结果 `backend_disabled` 明确后端未启动、无 orphan、输入已复核、输出 Artifact 为零。CLI 新增 `begin/cancel/cleanup/executions/execution-show`，且不输出 lease ID/owner、Manifest、命令、路径或 Artifact 正文。

本轮切片审计补强了 SQL 对 preparation 输入/输出计数和输出上限的直接绑定、清理重放的 lease 释放错误传播，以及取消/清理在提交结果不确定时的 operation-ledger 恢复。测试覆盖协议 fail-closed、完整生命周期与重放、终态 Run 清理、跨 Run Artifact 拒绝、租约活动互斥/续期/过期接管、旧 worker fencing、SQL 不可变、v49 原地升级、事件隐私和 CLI smoke；uncached 全仓普通/race、`go vet`、零告警 `staticcheck`、module verify/tidy diff、`govulncheck`、前端测试/typecheck/OpenAPI drift/生产构建、npm audit、凭证/运行产物/Runner 调用扫描、diff 检查和隔离真实二进制生命周期 smoke 全部通过。GitHub Actions run `29353239789` 对提交 `ff4846a` 的 Go 与 TypeScript 作业分别用时 2 分 6 秒和 25 秒并全部通过，未发现未解决高/中风险问题。双指标更新为架构完成度约 98%，产品可用度仍约 45-50%，因为真实执行没有开放。

schema v51 禁用态 Sandbox 后端/输出预检切片已完成：`preflight` 在 v50 lifecycle 之后再次提交完整 Manifest，重验 v48 preparation、v49 candidate、v50 execution、Scope、Policy、精确审批、`os.Root` 挂载、累计 token/模型时间/工具预算、Run lease 和输入 Artifact 内容。固定威胁模型包含 16 项有序检查，当前全部为 required、unverified、not-probed；握手固定 backend unavailable，容器身份固定未绑定。输出计划只保存 domain-separated locator 指纹和 stdout/stderr/file 类型，固定 all-or-nothing、aggregate hard cap、MIME 检测、普通文件、symlink/special-file 拒绝、脱敏和 retry 前协调，导出与 Artifact commit 均未授权。

v51 根记录、检查、输出槽和 digest-only operation 全部不可变；相同意图在双 SQLite Store 并发下收敛，异键或异意图冲突，取消/清理/活动 lease/预算漂移失败关闭。CLI 新增 `preflight/preflights/preflight-show`，不显示 locator、原始路径、命令、Manifest、容器身份或私有 lease。定向测试、uncached 全仓普通/race、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、strict TypeScript、OpenAPI drift、production build、零漏洞 npm audit、凭证/运行产物/Runner 调用扫描、diff 检查和隔离真实二进制 preflight smoke 均通过。GitHub Actions run `29357134923` 已通过提交 `041f617`，Go 与 TypeScript 作业分别用时 2 分 13 秒和 19 秒。当前未发现高/中风险问题；修复了输出计划指纹未持久化、SQLite 布尔解码引用错误、输入 Artifact 总字节 SQL 绑定缺口和同 execution 异键冲突诊断四项低风险问题。架构完成度保持约 98%，产品可用度保持约 45-50%，因为 v51 只冻结实现清单，没有增加真实执行能力。

schema v52 仅模拟后端证据与输出事务切片已完成。`SimulationBackendClient` 不包含 daemon transport，只接受 Docker Manifest 和规范 OCI 镜像摘要，并把 daemon、挂载、网络、密钥、容器配置、资源、终止、orphan 与输出计划的独立指纹映射到 16 项有序 `simulated_pass`。这些项目只供 harness 断言，全部保持 `verified=false`；根记录固定 `simulation_only`、`production_verified=false`，后端可用、执行授权和 Artifact 提交授权均为 false。`sandbox_output_fixture.v1` 严格拒绝重复/未知字段、无效 UTF-8、槽位错序、超总字节、symlink 和特殊文件，并在 MIME 检测与脱敏后一次性提交到 `FakeArtifactSink`；注入失败或取消必须回滚为零，生产 `run_artifacts` 不写入。

Application 与 SQLite 在 evidence 和 output-simulate 两个边界都重新提交 Manifest，并独立复核 v48-v51 身份、Scope、Policy、精确审批、挂载、累计预算、Run/Sandbox lease、输入 Artifact 和输出计划。新根记录、项目与 digest-only operation 不可变，同意图跨 Store 并发收敛，异意图冲突。CLI 新增 `evidence/evidences/evidence-show/output-simulate/output-simulations/output-simulation-show`；数据库、事件和 CLI 不保存或显示夹具正文、locator、路径、命令、Manifest、密钥、容器 ID、operation digest 或私有 lease。协议、Application、Store、迁移、并发、取消、回滚、事件隐私和 CLI 定向测试已通过；uncached 全仓普通/race 分别用时 120.7 秒和 155.7 秒，vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、strict TypeScript、OpenAPI 无漂移、production build、零漏洞 npm audit、凭证/运行产物/Runner 调用/Markdown 链接扫描、diff 检查和隔离真实二进制完整模拟 smoke 全部通过。smoke 没有 Docker create/start 调用，生产 Artifact 为零。审计未发现未解决高/中风险问题，并补强了脱敏后夹具摘要、完整请求指纹、SQL 预算/输入 Artifact 复核、迁移降级测试顺序与每份 evidence 最多 8 次模拟的 Go/SQL 双层门禁。GitHub Actions run `29362181363` 已通过功能提交 `f48cbb4`，Go 与 TypeScript 作业分别用时 2 分 9 秒和 19 秒。该切片不增加真实执行能力，双指标暂保持架构完成度约 98%、产品可用度约 45-50%。

schema v53 固定端点只读 Docker 生产观测切片已完成。最小 transport 只包含 endpoint、ping、daemon version/info 与精确 OCI digest image inspect；Linux 只拨号 `/var/run/docker.sock`，关闭代理和重定向，只放行四类 GET，Windows 当前明确返回 `transport_unsupported`。它不读取 `DOCKER_HOST`，不接受 TCP/调用方 socket，不调用 Docker CLI，也没有 create/start/run/exec/pull/remove 方法。HTTP 响应有 2 MiB 上限、最终 host/content-type/status 校验与重复 JSON 字段拒绝；确定性 fake transport 覆盖完整、daemon 不可用、镜像不存在、歧义响应和取消。

`run sandbox observe` 需要 `--confirm-readonly-probe`，并绑定同一份 v52 evidence、output simulation 与完整 Manifest。Application 在探测前重验 v48-v52，SQLite 在提交时再次复核当前 Policy/审批/预算/Run 与 Sandbox lease/输入 Artifact/取消/清理；同意图重放不再次探测，跨 Store 并发收敛，每份 simulation 最多 8 条。不可变根、六项 item 与 digest-only operation 只存有界元数据和指纹，不存 daemon ID/名称/root、socket、RepoDigest、Manifest、命令或私有 lease。完整结果的 `production_observed=true` 只表示 daemon/image 元数据被读取；private mount 固定 `not_observable_read_only`，生产验证、后端可用/启用、执行和 Artifact 提交授权仍全部为 false。定向测试与最终本地发布门禁均通过：全仓普通/race 用时 125.4 秒/140.7 秒，vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、strict TypeScript、OpenAPI 无漂移、production build、零漏洞 npm audit、真实测试 key 前缀/运行产物/Docker 写入口/Markdown 链接扫描、diff 检查和隔离真实二进制完整链路 smoke 全绿。smoke 证明未确认探测被拒绝、Windows 仅记录 `transport_unsupported`、重放不重探、生产 Artifact 为零且私有 Docker 字段不进入 CLI。审计修复了不同候选 ID/时间导致同语义并发请求误冲突，以及底层 GET 缺少独立路径/最终请求/严格 JSON media type 白名单两项低风险问题；双 Store 竞态连续 20 次普通、10 次 race 通过。首次 GitHub Actions run `29368112083` 的 TypeScript 作业 22 秒通过，Linux Go 作业则发现 CLI 单测误触 runner 真实 daemon；测试已改用仅进程内注入的确定性 unavailable observer，生产默认路径和 opt-in 真实集成测试不变。GitHub Actions run `29368979988` 已通过修复提交 `fe7b070`，Go 与 TypeScript 作业分别用时 2 分 7 秒和 23 秒。未发现未解决高/中风险问题。双指标暂保持架构完成度约 98%、产品可用度约 45-50%，因为没有新增真实执行能力。

schema v54 确定性 Docker 容器计划与假写事务切片已完成。`sandbox_docker_container_spec.v1` 只接受完整且仍有效的 v53 观测和重新提交的 Manifest；Application 与 SQLite 在计划边界重验 v48-v53 的 Run/Mission/Workspace、Policy、审批、预算、Run/Sandbox lease、输入 Artifact、取消、清理与全部指纹。编译器固定 `65532:65532` 非 root、只读根与输入、唯一可写输出、`rprivate` mount、no-new-privileges、drop-all capabilities、init、网络关闭或 managed-egress 默认拒绝/精确白名单、临时 secret mount、CPU/内存/PID/输出/时间/kill 上限、权威指纹派生标签和 reconcile-before-create/stop-before-export/remove-after-export 顺序。完整 command/argv/path/target/env/secret/label/container identity 只存在内存。

schema v54 新增不可变 metadata-only plan/control/step/operation 表。十六项 control 全部固定 `compiled_not_applied`、`applied=false`、`verified=false`；七步 fake write transaction 只在内存中暂存 reconcile/create/start/wait/stop/export/remove，失败、模拟崩溃或取消提交为零，成功也固定 daemon writes、backend touch、production submission、execution/export 和 Artifact authority 为零。CLI 新增 `docker-plan/docker-plans/docker-plan-show` 并要求 `--confirm-fake-write`；同意图重放不重复假写，双 Store 并发收敛，数据库/事件/CLI 不泄漏私有规格。审计补强 evidence/output simulation requester 连续性、SQL 显式 requester 门禁和 fake snapshot 深拷贝隔离。定向协议、Application、Store、迁移、并发、取消、回滚、SQL 不可变、隐私、v53 原地升级和 CLI 测试通过；最终全仓普通/race 用时 128.2 秒/148.5 秒，vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、strict TypeScript、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/运行产物/Docker 写入口/乱码/Markdown 链接扫描、diff 检查和隔离真实二进制 v54 migration/Workspace smoke 均通过，双 Store 竞态连续 20 次普通、10 次 race 通过。GitHub Actions run `29376503165` 已通过功能提交 `126719f`，Go 与 TypeScript 作业分别用时 2 分 7 秒和 19 秒。双指标暂保持架构完成度约 98%、产品可用度约 45-50%，因为真实执行能力没有变化。

schema v55 有界 Docker 创建/核验/删除演练切片已完成实现。新增与 v53 observer 隔离的 `sandbox_docker_write_transport.v1`，默认构造明确 `transport_disabled`；只有 `run sandbox docker-rehearse --confirm-daemon-write` 和完整当前 v54 计划可在 Linux 选择固定 `/var/run/docker.sock`。底层固定 Docker API `1.40`、关闭代理和重定向，不读取 `DOCKER_HOST`，闭合白名单只允许 exact image inspect、deterministic-name create、exact container inspect 和固定 `v=1` 匿名卷清理的 returned-ID non-forced delete；接口不存在 start、exec、attach、pull、日志、archive、export、网络/卷管理/镜像构建或通用请求。Windows 明确返回 unsupported。

首个 v55 profile 强制网络关闭、环境变量为空且无 secret。Application 在 daemon 前重验 v48-v54 身份、Manifest、Policy、审批、预算、Run/Sandbox lease、输入 Artifact、取消、清理、evidence、simulation、observation 和 plan，transport 再次编译并精确匹配 v54 spec。宿主 mount 逐组件拒绝 symlink，且必须位于可信 workspace 内并指向普通文件或目录。create 前新增只读镜像检查：RepoDigest 必须匹配计划，镜像不得声明 `VOLUME`，防止未启动时仍生成匿名卷。创建请求固定摘要镜像、`65532:65532`、只读根、no-new-privileges、drop-all capabilities、init、network none、资源上限、private bind、无重启与无日志；inspect 还精确复核 attachment、capability、device、port 和 mount。名称冲突只有在旧容器从未启动且配置/labels 完全一致时才回收。取消、失败或 create 响应不确定时，独立五秒 cleanup context 会按 ID/名称重查，只有精确 authority match 才删除，不按返回 ID 盲删。

schema v55 增加不可变 metadata-only rehearsal/五步/operation 表和事件。正常路径记录三次 daemon 读取与 create/remove 两次写，回收一个精确旧演练容器时为三次写；持久化、事件和 CLI 不保存原始 container ID、host path、command/argv、环境值、secret、socket 或完整规格。同 operation 重放在 transport 前返回原结果，两个 SQLite Store 的竞态收敛。类型和 SQL 固定 container never started、process never executed、image never pulled、output never exported、cleanup confirmed，并将生产提交/验证、后端启用、执行和 Artifact 授权固定为 false。严格 fake Engine 测试覆盖成功、镜像声明卷拒绝、精确 stale orphan、危险名称碰撞、已知/未知 create 结果取消 cleanup、拒绝盲删、symlink 和 endpoint closure；Linux opt-in 集成测试只接受已存在且无声明卷的 OCI digest，默认跳过且不会 pull。定向协议、Application、Store、迁移、重放、并发、隐私、SQL、降级/再升级和 CLI 测试已通过。最终全仓普通/race 用时 163.3 秒/168.7 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、strict TypeScript、OpenAPI 无漂移、production build、零漏洞 npm audit、凭证/运行产物/禁止 endpoint/乱码/Markdown 链接扫描、diff 检查和隔离真实二进制 schema-v55 Workspace smoke 均通过。transport 连续 20 次普通与 10 次 race、双 Store 并发 10 次通过。发布前审计修复了 create 已落地但响应取消的 orphan 窗口、失败时盲删返回 ID、镜像声明卷的匿名卷副作用，以及 attachment/device/port/capability 核验不足；当前未发现未解决高/中风险问题。由于本机 Windows 无可用 Docker，默认跳过的 Linux real-daemon 写集成测试未实际执行，该残余缺口不开放 start 或生产权限。GitHub Actions run `29382661971` 已通过功能提交 `69d81d6`，Go 与 TypeScript 作业分别用时 2 分 32 秒和 25 秒。

schema v56 可恢复 Docker rehearsal attempt 切片已完成实现。Application 会在 daemon 变更前持久化不可变 intent，并获取带到期时间、递增 generation 的 SQLite lease；Stage 只创建一次确定性名称容器，或收养未知 create 响应/旧 generation 留下的精确 stopped authority match。stage checkpoint 固定 19 项 ordinal/name 检查，覆盖镜像、命令/工作目录、非 root、只读根、no-new-privileges、capability、init、禁用网络、实际空环境、无 secret、mount 配置、资源、重启、日志、设备、端口、attachment、authority label 和 never-started，且 Go/SQLite 均强制 `execution_evidence=false`。镜像与容器 inspect 现在都拒绝继承环境变量，避免把“Manifest 没传 env”误当作“最终环境为空”。

Cleanup 只会在重新 inspect 后删除 authority、配置、request fingerprint 与 container-ID fingerprint 全部精确匹配的对象；已不存在按幂等成功，同名不匹配对象永久拒绝。失败以最多 16 条有界代码追加并释放 lease；过期或已释放 lease 可由新 generation 接管，旧 generation 连同幂等重放一起被拒绝。`docker-attempts/show/resume` 提供 metadata-only 恢复，resume 必须重交完整 Manifest、保持原 requester/intent 并再次确认写入，不要求保留原始 operation key。v55 rehearsal、operation、v56 completion 与最终 lease release 原子提交；发现孤立 legacy operation 时 fail closed。

定向测试覆盖未知 create 后按 attempt ID 恢复、只 create 一次、already-absent、无关同名保护、release/expiry takeover、stale generation fencing、双 Store 单 lease、stage/cleanup/completion 原子性、不可变 SQL、隐私、v55 升级和 CLI。最终全仓普通/race 用时 178.5 秒/181.3 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、17 项前端测试、strict TypeScript、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/运行产物/禁止 endpoint/增量乱码/Markdown 链接扫描、diff 检查和隔离真实二进制 schema-v56 Workspace smoke 均通过。恢复 transport 普通 20 轮/race 10 轮、双 Store 单 lease 普通 20 轮/race 10 轮、Application 未知 create 恢复 10 轮均通过。审计修复了继承环境证据缺口、过期 completion/failure fencing、失败代码/顺序/16 条耗尽边界、generation 获取时间线、旧 generation 借用新 owner 的重放路径、legacy operation 缺 completion 的静默重放、恢复依赖原 operation key、控制矩阵 SQL 名称映射不足，以及 mount 控制名称可能过度声称 TOCTOU 已解决的问题。当前未发现未解决高/中风险；GitHub Actions run `29388724727` 已通过功能提交 `e1710bb`，Go 与 TypeScript 作业分别用时 2 分 32 秒和 23 秒。本机 Windows 无 Docker，因此 Linux real-daemon harness 仍未执行，但它不能 pull 或 start。

schema v57 descriptor-pinned、kernel-sealed 宿主输入捕获切片已完成实现。`DockerHostInputStager` 默认关闭，CLI 只有在原有 `--confirm-daemon-write` 之外同时给出 `--stage-host-inputs --confirm-host-input-staging` 才启用；Windows 在任何容器创建前明确返回 `staging_unsupported`。Linux 根目录和只读 mount 树使用 `openat2` 的 no-symlink/no-magic-link/beneath/no-cross-device 约束。每个条目先以 `O_PATH` 预检，再只对同一 inode 的普通文件或目录执行内容打开，因此 FIFO/特殊文件在潜在阻塞前拒绝；目录与单文件 mount 均支持，hard link、过深目录、条目与字节超限失败关闭。全部 fd 固定后再次核验 dev/inode/mode/nlink/size/mtime/ctime，再把脱敏元数据与已按 Run/Session/Workspace/摘要/大小/MIME/stream/source/order 复核的 Artifact 内容写入确定性 tar。目录的文件系统相关 inode 大小不进入内容摘要。tar 仅存在于 `memfd`，施加 write/grow/shrink/seal 后重新读取校验摘要，不写工作区或数据库。

Application 在 v56 stage 后、cleanup 前持久化 v57 intent，并绑定 operation digest、attempt intent/request、container-ID 指纹、v54 plan、mount/input/authority/spec 指纹、当前 lease generation 和 requester。SQLite 的 intent/result 表、触发器与事件只保存计数、大小、摘要和安全布尔值；路径、正文、fd、raw container ID 与私有 lease 身份均不落库。pending intent 会在 SQL 层阻止 v56 completion。封装、报告复核或落库失败时先以独立有界 context 清理停止容器，再追加失败并释放租约；新 generation 可从 pending intent 恢复，cleanup 已完成也能提交证据，且不重复 create。CLI 新增 `docker-host-inputs` 与 `docker-host-input-show` metadata-only 查询。

定向测试已覆盖独立确认、默认关闭、自定义 stager 报告复核、rename/replace/delete/symlink/hard-link/FIFO/单文件 mount、有界目录枚举、取消、确定性重放、失败清理、重启/第二代 lease 恢复、stale generation fencing、completion SQL 门禁、不可变 SQL、路径/正文隐私和 v56 原地迁移。最终全仓普通/race 分别用时 155.0 秒/168.1 秒；应用 staging 10 轮、双 Store 独立 ID 并发 20 轮及 race 10 轮通过。vet、零告警 staticcheck、module verify、零可达漏洞 govulncheck、17 项前端测试、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭据/运行产物/进程入口扫描、diff 与隔离真实二进制 schema-v57 smoke 均通过。Linux test binary 交叉编译通过；`openat2`/seal 运行测试由 GitHub Actions 执行。本轮审计补强了公开报告构造器、Artifact payload/字节二次复核、根路径父级 symlink 拒绝、stage→intent 时间顺序、超限错误语义、歧义确认、单文件报告约束、目录摘要稳定性、特殊文件预打开阻塞、有界分批目录读取、分块取消、随机 ID 语义收敛和漏确认不消耗 failure/lease。未发现未解决高/中风险。密封包当前没有交给 Docker daemon，持久事实固定 `daemon_consumed=false`、`execution_evidence=false`，因此 v57 只关闭 descriptor-capture 内的路径偷换，不证明未来容器实际消费了这些字节。架构完成度暂保持约 98%，产品可用度约 45-50%。

首次 GitHub Actions run `29395980413` 仅暴露 Linux 单文件测试夹具与 Manifest 工作目录覆盖规则冲突，生产实现未失败。夹具已改为一个目录 mount 覆盖工作目录并增加独立单文件 mount，真实验证 `directory_count < read_only_mount_count`；run `29396264276` 已通过提交 `8719dff`，Go/Linux 3 分 55 秒，TypeScript 23 秒。

schema v58 durable pre-stage host-input requirement 切片已完成实现。Application 在任何 Docker stage 前派生 `sandbox_docker_host_input_requirement.v1`，并与 v56 attempt、首代 lease 及审计事件在同一 SQLite 事务提交。事实固定 required/confirmed 选择，并绑定 attempt/plan、Run/Mission/Workspace、requester、operation digest、Manifest/request/mount/input/authority/spec 指纹、只读挂载和输入 Artifact 计数；语义指纹排除随机 row ID 与时间戳，使双 Store 独立候选收敛。数据层只保留摘要、计数和布尔值，不保存路径、正文、fd、raw container ID、原始 operation key 或私有 lease 身份。

新 attempt 的恢复以 durable requirement 为唯一依据：`required=true` 时，即使 `docker-attempt-resume` 不再提交 staging flags，也会在 completion 前恢复 v57 捕获；`required=false` 不允许稍后扩成 staging。Go 聚合校验、Application 重放规则和 SQLite insert/completion triggers 三层约束 attempt/plan/authority 一致性、required evidence、false-to-staging 拒绝与不可变性。迁移从 v57 升级时不回填 requirement，避免伪造历史 operator intent；它只在安装禁止新增 trigger 前把既有 attempt ID 写入不可变 legacy 集合，之后所有 stage/staging/completion 必须关联 requirement 或该迁移标记。定向测试覆盖 migration、legacy 标记不可变、事件隐私、SQL 更新/删除、缺失 requirement stage、无证据 completion、false 扩权、随机 candidate 收敛、completed/pending operation replay、崩溃后第二 generation 无 flags 恢复、CLI projection 和纯输出零只读挂载合法性。

最终本地门禁全部通过：全仓普通/race 用时 158.1 秒/168.4 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、生产构建、零漏洞 npm audit、凭据/运行产物/进程入口/乱码/Markdown 链接扫描、diff、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v58 workspace smoke 均为绿色。领域 requirement 50 轮、Store 收敛/缺失 requirement 30 轮、Application pending recovery/防扩权 20 轮及 Store/Application race 各 10 轮通过。审计修复了 pending operation-key 恢复误建候选 attempt、durable requirement 旁不成对显式 flags、迁移后 direct-SQL 新 attempt 缺失 requirement，以及 false requirement 零输入兼容性；未发现未解决高/中风险。Windows 本机没有 Docker，故 v59 Linux real-daemon handoff harness 未运行。

GitHub Actions run `29400696276` 已通过功能提交 `4b570f7`，Go/Linux 2 分 39 秒，TypeScript 23 秒。

本轮架构审计发现 Docker Engine archive 写入会拒绝 read-only rootfs/volume，因此 v58 没有把 v57 密封包直接 PUT 到目标容器，也没有将目标根或输入改为可写。archive、volume、start、exec、pull、build、export 与 Artifact 权限均未增加，`daemon_consumed=false`、`execution_evidence=false` 保持不变。ADR 0018 将下一切片拆为 schema v59：daemon-owned writable carrier、固定目的地上传、daemon readback 摘要核验、carrier 删除、以只读 volume 重建仍未启动目标容器，并要求 crash/retry/cleanup/collision 独立证明。架构完成度暂保持约 98%，产品可用度约 45-50%，因为本切片增强恢复安全而未开放新的终端用户执行能力。

Docker/Local 真实进程执行继续关闭，直到 v51 每项检查都有独立可复核的生产证据，且 Sandbox input handoff、资源、网络、取消与原子 Artifact 导出全部通过单独审计；v52 的模拟通过、v53 的元数据观测、v54 的编译/假写、v55-v56 的未启动容器演练/恢复、v57 的 daemon-unconsumed sealed bundle 和 v58 的 durable requirement 都不能满足该要求。下一切片 schema v59 只实现并审计 daemon-owned immutable handoff，仍不开放 start。

schema v59 daemon-owned immutable host-input handoff 切片已完成实现。Application 在 attempt 创建事务内同时固定 v58 capture requirement 与新的 handoff requirement；只有 daemon write、descriptor staging、handoff 三组确认全部一致时才能启用。v57 bundle 重新捕获为短生命周期 sealed handle 后，Go 先把 handoff intent 绑定到 attempt/plan、停止容器、staging/report/bundle、完整 authority、requester 和当前 generation，再允许底层接触 archive 或 volume。required handoff 在 Go 与 SQLite 双层阻止 cleanup/completion，false requirement 不能重放扩权，v58 既有 attempt 只进入迁移生成且不可修改的 legacy 集合，不伪造历史选择。

Linux transport 继续固定本机 Unix socket 与 Docker API `1.40`，不读取 `DOCKER_HOST`，不接受 caller endpoint。operation allowlist 仅包含 exact image/container inspect、确定性 local volume create/get/non-forced delete、never-started carrier/target create/inspect/delete，以及固定 `/cyberagent-input/bundle.tar` 的 archive PUT/GET。它把精确 v57 tar 包成只读文件写入临时可写 carrier 的 daemon-owned volume，再从 daemon 回读并同时核对长度与 SHA-256；之后删除 carrier 和原 stopped target，短暂创建挂只读 volume 的 target 复核完整配置，最后删除 target 与 volume。Manifest mount 与保留目录相同、互为祖先/后代或根挂载均提前拒绝。重试只清理同 request label/config/fingerprint 的 carrier、volume、原 target 或 final target；foreign collision 永不删除，early bundle/image failure 也以独立 context 清理精确自有对象。start、exec、attach、logs、pull/build、network mutation、arbitrary archive、force volume delete、export 与 Artifact writer 均不存在。

CLI 新增 `--handoff-host-inputs --confirm-host-input-handoff` 以及 `docker-host-input-handoffs`/`docker-host-input-handoff-show`，只投影 generation、状态、bounded daemon count、readback/readonly/cleanup 与摘要元数据。路径、正文、fd、raw container ID、carrier/volume 名、socket、raw operation key 和 private lease 不落库、不进事件。持久结果只能声明 `daemon_consumed/readback_verified/final_mount_read_only/cleanup_confirmed=true`，并固定 container started、process executed、output exported、backend enabled、execution authorized、Artifact commit authorized 为 false。

最终本地门禁通过：全仓普通/race 分别用时 183.1 秒/185.1 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/运行产物/进程入口/乱码/Markdown 链接扫描、diff、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v59 workspace smoke 均为绿色。Sandbox handoff 50 轮、Store 30 轮、Application 20 轮、CLI 10 轮，以及 race 下 Sandbox 20/Store 10/Application 10 轮通过。审计修复了 early validation/image failure 遗留原 stopped target、archive GET media type 未复核、固定输入目录与 Manifest mount 重叠、SQLite endpoint/identity 约束不足、重复读取可变 bundle report，以及 Linux harness 只测内存流未串联真实 `openat2 + memfd seal` 的问题。未发现未解决高/中风险。

本机 Windows 没有 Docker，因此 opt-in `TestDockerHostInputHandoffRealDaemonOptIn` 只完成 Linux 编译，尚未执行真实 daemon。该 harness 使用实际 v57 local bundle provider，只接受已存在、RepoDigest 精确匹配、无声明 `VOLUME` 且继承环境为空的镜像，不 pull、不 start，并断言 target/carrier/volume 全部删除。此残余证据缺口阻止后续 start gate，不影响当前 never-started 协议结论。架构完成度维持约 98%，产品可用度维持约 45-50%；schema v60 接着解决 verified bundle 到 Manifest target 的未应用 runtime projection，start/wait/TERM/KILL/orphan 与输出 Artifact 事务继续后置。

GitHub Actions run `29406403201` 已通过功能提交 `fb1daca`，Go/Linux 作业 2 分 37 秒、TypeScript 作业 28 秒；Linux 全仓测试、vet、govulncheck 与前端完整门禁全部通过。

schema v60 deterministic runtime-input projection plan 切片已完成实现。`PlanDockerRuntimeInputs` 只有在操作者独立确认、v59 handoff 和 v56 attempt 均完成后才工作；它重验完整 v48-v59 Run/Mission/Workspace/Manifest/Policy/审批/预算/Artifact/plan/attempt/handoff authority，重编译同一 v54 container spec，再通过 v57 descriptor-safe provider 重新捕获短生命周期 sealed bundle。冻结的 report view 防止 provider 在解析中改变元数据；report fingerprint、bundle digest/长度、mount/Artifact 计数和 Artifact payload 必须与 durable v57/v59 证据完全一致。

Go 编译器只接受逐字节 canonical 的 v57 PAX tar。它拒绝 symlink/hardlink、device/special entry、遍历、绝对/反斜线/重复路径、缺失父目录、异常 root、非规范 mode/owner/time/PAX、空 Artifact、尾随数据及统计漂移，并将解析结果重新编码后逐字节比较。第一版明确只支持目录 root 的 read-only Manifest mount；每个 root 生成一个相对 tar projection，输入 Artifact 固定投影到 `/cyberagent-input/artifacts`。未来 volume 名仅在内存存在，派生同时包含 handoff fingerprint，因此同一 handoff 重试稳定，而不同 Run 即使 Manifest 与 bundle 完全相同也不会资源碰撞。

schema v60 的 plan/item/completion/operation 四类事实和审计事件在 Run 写锁事务中提交。operator confirmation、连续 ordinal、kind 数量、entry/content/archive aggregate、handoff/attempt/container-plan 绑定、operation replay 和所有 false authority 位由 Go 与 SQLite 双层约束；迁移不为 v59 历史 handoff 伪造 projection。CLI 新增 `docker-runtime-input-plan`、`docker-runtime-input-plans`、`docker-runtime-input-plan-show`，只输出 ID、摘要、计数和权限位；raw target、宿主路径、文件名/正文、volume 名、archive bytes、socket、raw operation key 与 private lease 均不持久化。状态固定 `compiled_not_applied`，本切片没有 Docker transport，不 create volume、不 upload、不重建 container、不 start/exec/export，也不授权 backend/execution/Artifact。

切片审计修复了八项问题：操作者确认最初只在 Application 检查而未持久化；未来 volume identity 未绑定 handoff、会让相同跨 Run 输入同名；item fingerprint 被误设为全库唯一；tar reader 会忽略 end marker 后的尾随数据；代码读取已弃用 `Header.Xattrs`；mount ordinal 缺少重复/越界闭包；计划时间没有同时约束 attempt completion 与 handoff；严格 PAX 白名单最初会误拒绝 Go 为长 canonical 路径生成的 `path` 记录。修复后确认事实进入 plan 指纹和 SQL `CHECK=1`，资源名跨 Run 隔离，content fingerprint 可合法重复，canonical re-encode 关闭尾随通道，Go/SQL 同时约束 ordinal 与时间线，并只接受与 header 完全一致的 canonical PAX path。当前未发现未解决高/中风险。

最终本地发布门禁通过：全仓普通/race 分别用时 198.9 秒/194.0 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/运行产物/乱码/Markdown 链接扫描、diff、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v60 Workspace smoke 均为绿色。编译器 50 轮、Store 30 轮、Application 20 轮、CLI 10 轮，以及关键 Sandbox/Store/Application race 10 轮通过。烟测只注册 mock Provider，在独立 `CYBERAGENT_HOME` 创建 v60 数据库和 Workspace，确认 schema-v60 表后安全删除临时程序与 runtime；没有调用真实模型、Shell、网络或 Docker。架构完成度仍约 98%，产品可用度仍约 45-50%，因为本轮增强的是可审计输入准备而非用户可见执行。下一切片是 schema v61：独立 write-ahead/generation-fenced transport 只应用 v60 volumes 并保持 container never-started；start 生命周期继续后置。

GitHub Actions run `29428011306` 已通过功能提交 `cc92421`：Go/Linux 作业 2 分 48 秒、TypeScript 作业 24 秒；Linux 全仓测试、vet、govulncheck 与前端完整门禁全部通过。

schema v61 recoverable runtime-input application 切片已完成实现。`ApplyDockerRuntimeInputs` 要求操作者分别确认输入应用与 daemon 写入，并在任何 bundle recapture 或 Docker 写入前，把不可变 application intent 与独立 generation lease 原子落库。intent 精确绑定 v60 projection、v59 handoff、v56 attempt、container plan、Run/Mission/Workspace、Manifest/mount/input/authority/spec、固定 endpoint、requester 和摘要化 operation key。Application 随后重新核验完整 v48-v60 链、重编译规格、重新解析唯一 output bind，并通过 descriptor-safe provider 重新捕获 v57 密封包；report、bundle、Artifact payload 与全部 v60 projection aggregate/item 必须逐项一致。

固定 Unix transport 只允许 Docker API `1.40` 的 exact image/container/volume inspect、deterministic local volume 与 never-started carrier/target create/delete，以及固定 `/cyberagent-input` 的 archive PUT/GET。每个 projection 使用独立可写 carrier 写入 volume，再从 daemon 回读并按完整 tar entry、mode、content 语义复核，成功后删除 carrier。最终 target 只保留一个已复核 output bind 和全部 read-only/`NoCopy` 输入卷，并再次核验 image、identity、readonly root、capability、network、environment、resource、port/device/attachment、label、mount 与 never-started 配置。allowlist 没有 start、exec、attach、logs、export、pull/build、network mutation、force volume delete 或任意 endpoint。

重试仅在 container/volume 的完整 request/projection/role/item 标签及配置一致时回收或协调，外来同名资源拒绝且不删除。当前 active generation 才能提交 failure/result；失败以有界代码追加并原子释放，released/expired lease 可由 generation+1 恢复，旧 worker 被 SQL fencing。daemon 工作在 lease 到期前预留两段 cleanup 窗口，失败清理使用独立有界 context，只处理 exact-owned 资源。相同 operation 完成后重放不重新捕获、不访问 daemon。SQLite v61 的 intent（含唯一 operation digest 绑定）、lease、failure、result 与 event，以及 CLI apply/resume/list/show 全部 metadata-only；不额外建立 operation 表，也不保存 target、宿主路径、文件或资源名称、raw ID、archive、socket、raw operation key 或 private lease。成功固定 `volumes_applied_target_never_started`，start/process/export/backend/execution/Artifact 权限仍全部为 false；Windows 明确 unsupported。

定向测试覆盖双确认无副作用、写前 intent、transport 失败与 generation2 恢复、stale worker fencing、完成与语义重放、get/list、v60 原地迁移不伪造 application、SQL 不可变/隐私/事件、foreign volume 保护、readback mismatch exact cleanup、禁止 endpoint、lease cleanup reserve、最终只留 never-started target/volumes，以及 CLI 输出隐私。ADR 0021 固化了此边界。最终本地门禁通过：全仓普通/race 分别用时 197.5 秒/316.8 秒；vet、零告警 staticcheck、module verify、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、仓库隐私/进程/禁止 endpoint/编码/Markdown 扫描、diff、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v61 smoke 均为绿色，Sandbox race 20 轮通过。审计补强逐卷回读上限、到期前 cleanup reserve、未来租约/最短 TTL、resume 双确认、取消后独立失败落账、稳定错误码、v55/v59/v61 最小 transport 接口、Docker `RW`/`NoCopy` 实际证据和 operation digest 格式；未发现未解决高/中风险。本机 Windows 没有 Docker，Linux real-daemon v59/v61 harness 仍需人工环境执行，因此当前不作进程隔离或可启动声明。架构完成度维持约 98%，产品可用度维持约 45-50%。下一切片 schema v62 先实现 retained target/volume 的显式 inspect 与 exact-owned cleanup/reconciliation，继续不开放 start。

GitHub Actions run `29437941378` 已通过 v61 功能提交 `f4aaf7a`：Go/Linux 作业 2 分 37 秒、TypeScript 作业 27 秒；Linux 全仓测试、vet、govulncheck 与前端完整门禁全部通过。

schema v62 retained runtime-input resource lifecycle 切片已完成实现。只读 inspection 要求显式确认，重新核验完整 v48-v61 authority、重编译同一规格并解析 output bind，但不重新 probe/capture v57 输入包。固定本机 Unix transport 只检查 v61 target 与全部确定性 volume，并将 exact complete、partial/absent 或 foreign collision 作为不可变 metadata-only 事实落库。never-started、read-only 与 `NoCopy` 只有在目标和全部卷同时精确存在时才成立；unsafe inspection 会保留审计证据并返回 failed precondition，不产生清理权限。

cleanup 需要独立的 `--confirm-resource-cleanup` 与 `--confirm-daemon-write`。Go 在 transport 前持久化 cleanup intent 与 generation lease；固定 transport 在任何 DELETE 前完成目标和所有卷的全量预检，遇到外来对象保证零 DELETE，否则按已检查 container ID 先删除 target，再非强制删除 exact-owned volumes，最后再次全量检查所有资源缺失。失败代码有界追加并释放 lease，后续 generation 可恢复，stale generation 不能提交；完成后的 operation replay 与 resume 均不访问 daemon。SQLite v62、事件和 CLI 不保存资源名、raw ID、宿主路径、socket、Manifest 正文、raw operation key 或 private lease identity，也没有 start/exec/attach/pull/export/backend/execution/Artifact 能力。

定向测试覆盖确认前零调用、无输入 recapture、写前 intent、失败/generation2 恢复、stale worker、幂等重放、unsafe evidence、部分/缺失资源、foreign collision 零 DELETE、target-before-volume、最终全缺失、SQL 不可变、迁移不伪造事实、事件/CLI 隐私与 Windows 最小接口。审计修复了 partial/unsafe 状态误报 read-only/`NoCopy`、清理事件命名歧义、CLI foreign-collision 显示、未来/越界终态时间戳和 v61/v62 lease 可直接删除问题，并把 Linux opt-in harness 延伸到 v62 inspect/cleanup。ADR 0022 固化该边界。架构完成度维持约 98%，产品可用度维持约 45-50%；下一切片先做 schema-v63 start gate 设计与生产证据复核，不直接开放进程。

v62 最终本地门禁通过：全仓普通/race 分别用时 313.6 秒/329.6 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、仓库隐私/禁止能力/编码/Markdown 扫描、diff、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v62 smoke 均为绿色；Sandbox/Store/Application/CLI 高频回归分别通过 20/15/10/10 轮。GitHub Actions run `29444398815` 已通过功能提交 `d250d32`，Go/Linux 与 TypeScript 作业分别用时 2 分 35 秒和 20 秒。本机仍无法执行 Linux real-daemon opt-in 链，因此 start gate 保持关闭。

随后纯文档 run `29444664401` 在 `npm ci` 已报告零漏洞后遇到 npm registry advisory endpoint 临时故障，并被 GitHub runner 错误保留为 in-progress。CI 的高危依赖审计现增加最多三次有界重试；真实高危结果仍会连续失败并阻断，单次 registry 抖动不再制造假红灯。

schema v63 blocked Docker process start-gate review 切片已完成实现。操作者只能在 v62 精确清理完成后，重新提交完整 Manifest、稳定 operation key，并显式确认设计审查。Application 会复核 v48-v62 全链，但不会重新捕获输入、访问 daemon 或创建进程。v51 的 16 项威胁检查逐项保存固定 evidence class/source、生产阻塞代码和未来独立门；所有检查仍固定为 `production_verified=false` 与 `sufficient_for_start=false`，审查结论只能是 `blocked/deny_start`。

同一事务冻结 11 条未来进程生命周期转换，覆盖 write-ahead start intent、固定端点提交、运行态核验、不确定启动、自然退出、TERM/KILL 升级、取消扇出、租约丢失孤儿化和 generation-fenced 协调。蓝图要求 per-Run 单一 owner、有界日志与 wait，但所有转换均为 `implemented=false`、`authorized=false`。SQLite v63 通过严格映射约束、root authority/completion trigger 和不可变触发器独立锁死结论；迁移不伪造旧审查。Store 支持 Run 写锁下的幂等重放和跨实例收敛，读路径重新校验 binding/check/transition/lifecycle/evidence/review fingerprints。CLI create/list/show 只显示有界元数据，不显示 Manifest、资源名、raw container ID、宿主路径、socket、raw operation key 或私有 owner。ADR 0023 固化该边界。

定向 Sandbox/Store/Application/CLI/迁移/并发/SQL/隐私测试已经通过。架构完成度维持约 98%，产品可用度维持约 45-50%，因为本切片增加的是可审计安全设计而非用户进程执行能力。下一切片为 schema v64 机器生成生产证据账本：先让 Linux opt-in real-daemon harness 产生规范化、可复核、无敏感资源数据的检查结果，再考虑独立 start 生命周期；任何 receipt 本身仍不得授权启动。

桌面端与自定义 Skill 导入在 v63 阶段正式进入计划但尚未实现。Desktop 采用 D0-D3 分段：当前只准备 Wails + 现有 React + Go HTTP/SSE/OpenAPI 的开发/便携只读壳；产品可用度约 65-70% 后开放 Go-owned mutation，约 75-80% 后才制作便携 ZIP 与签名 MSIX，企业 MSI、自定义协议、自启动、服务和自动更新继续独立审计。当时现有 `skill.v1` 与内部 `LoadFS` 只能验证内嵌/测试 FS，不是用户导入入口；计划先做 Markdown-only 有界包校验，再做 content-addressed 原子安装和 CLI，最后接 Desktop 的 Go-owned 上传预览。两份计划分别记录在 `docs/DESKTOP_PLAN.md` 与 `docs/SKILL_PACKAGE_PLAN.md`；该阶段尚未改变 schema v64 的优先级，后续非 schema Skill 校验切片再按最新产品优先级调整。

v63 最终本地门禁通过：最终代码全仓普通/race 分别用时 196.9 秒/212.3 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭据/密钥文件/进程入口/乱码/diff 扫描、Linux sandbox test binary 交叉编译和隔离真实二进制 schema-v63 Workspace/Skill smoke 全部为绿色。Sandbox/Store/Application/CLI 高频回归分别通过 20/15/10/10 轮。审计修复 v63 check/transition 子表读取未即时检查 `Rows.Err` 和内部错误文本未满足 staticcheck 两项低风险问题；未发现未解决高/中风险。Windows 无法执行 Linux real-daemon opt-in 链，故 16 项生产检查保持阻塞，start 继续不存在。GitHub Actions run `29503856229` 已通过提交 `e25a2ab`，Go/Linux 与 TypeScript 作业分别用时 2 分 32 秒和 24 秒。

非 schema 的 `skill_package.v1` 第一切片已完成实现。纯 Go parser 在解压前逐项检查完整 ZIP 结构，只接受按顺序出现的 `manifest.json` 与 `SKILL.md` 两个 Deflate 条目，以及固定 ZIP 2.0 data descriptor、零时间戳、零 extra/comment/属性、无前缀/间隙/尾随数据的确定性 profile。archive 限制为 64 KiB，并固定条目数、压缩/解压大小与压缩比；CRC、本地/中央目录头、descriptor、UTF-8、严格 JSON、content path、字节/token 声明与 SHA-256 全部失败关闭。原始 archive digest 与规范 Manifest/正文的 semantic fingerprint 分离，JSON 空白差异不会改变后者。

`cyberagent skill package validate <package.zip>` 只读取一个有界普通文件，拒绝 symlink/目录/非普通文件并在读取前后复核文件身份；输出仅含有界 Manifest 元数据、digest、`operator_installed_untrusted` 信任类别、风险代码和全部 false 的安装/命令/网络/Provider/工具授权位，不显示正文或源路径。解析和 CLI 均不创建数据库、不落盘、不安装、不执行任何文档内容，也不调用网络、模型、Tool Gateway 或 Sandbox。单元、对抗和 fuzz 测试覆盖截断、前后缀、注释、顺序/额外/大小写碰撞条目、压缩方法、时间与属性、symlink、本地头/descriptor 篡改、未知/重复 JSON 字段、路径/hash/UTF-8 漂移、压缩放大和命令样式正文保持惰性；ADR 0024 固化边界。

本切片没有 migration，schema 仍为 v63；五个内置 Skill 仍是唯一可被 Run 选择的产品 Skill。用户 Registry、`skill import/installed/remove`、外部 Run 选择以及 Desktop/HTTP 上传均未开放。按最新产品优先级，下一切片改为 schema v64 content-addressed 用户 Skill Registry 与不可变安装/卸载账本；原 P6 机器生成 Linux real-daemon 生产证据账本顺延至 schema v65 或以后，v63 的所有 Docker blocker 与 start 禁用状态完全不变。

非 schema 的受保护删除守卫切片完成 Go Policy 加固。Shell 原始命令以及结构化 ScriptProcess/Sandbox executable/argv 会在任何审批前检查递归删除、绝对/`..`/通配目标、环境变量、命令替换、当前用户目录和常见 PowerShell、`cmd`、Python、Node 删除形式；命中后固定为 `critical` 永久拒绝，理由不包含实际主目录，逐次审批和 Session Grant 不能扩权。Policy 工具参数按键排序，避免 Go map 遍历影响安全决策指纹。README、日志、模型说明和 read-only 工具内容仍是非执行证据，不因出现示例命令而被赋予执行语义。

定向审计先发现并修复 Node `require('fs').rmSync(...)` 与参数开头 `../` 的漏判，又修正了 PowerShell `-Force` 被紧凑 Unix flag 逻辑误认成递归删除的误报。Shell Gateway 测试证明永久拒绝无法被操作者批准并且不会产生 dry-run 输出；ScriptProcess 集成测试证明结构化删除意图以 `execution_mode=disabled` 终止。ADR 0025 明确该分类器只是纵深防御，不能识别任意间接脚本、构建钩子或编码载荷；未来真实执行仍必须用 OS/容器把宿主根设为不可见/只读，并通过 Go-owned typed workspace delete 完成受限删除。Local/container process 继续关闭，本切片无 migration、无新工具或文件副作用，schema 保持 v63，下一切片仍为 schema v64 Skill Registry。

本切片最终本地门禁通过：最终全仓普通测试 197.0 秒、全仓 race 222.6 秒，新增三层安全链路 race 20 轮，Policy fuzz 约 40.6 万次，Policy/Gateway/Application 高频回归 100/50/50 轮；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit，以及凭据/运行产物/乱码/Markdown 链接/diff 扫描全部通过。凭据模式只命中 6 个既有合成脱敏夹具；未发现未解决高/中风险。本机额外 Linux 交叉编译因宿主命令策略拒绝清理临时产物而未重复执行，不绕过该保护，推送后的 GitHub Linux CI 作为平台验证。

本切片最终发布门禁通过：最终代码全仓普通/race 分别用时 239.4 秒/226.8 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、20 秒约 2645 万次 parser fuzz、`internal/skills` 78.5% 语句覆盖、parser 100 轮与 CLI 20 轮重复回归、严格 TypeScript、8 个文件 17 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit，以及凭据/运行产物/乱码/Markdown 链接/diff 扫描全部通过。凭据扫描只命中既有脱敏测试中的合成 `sk-123...` 夹具。代码审计固定了 central-directory creator version 和 Deflate 精确耗尽，关闭有效压缩流后的隐藏载荷通道，替换测试中的弃用时间 API，并把文件系统 cause 包在稳定、无源路径回显的类型化 CLI 错误后；当前未发现未解决高/中风险。GitHub Actions run `29512332025` 已通过提交 `55b3fae`，Go/Linux 与 TypeScript 作业分别用时 3 分 4 秒和 20 秒。

schema v64 Run execution profile 切片已完成实现。每个新建 Run 在同一事务中获得 revision 1 的 `preview` 快照，迁移数据也保守回填 `preview` 而不伪造历史 Run event。操作者只能在 `created` 或无活动 execution lease 的 `paused` Run 上，用摘要化稳定 operation key 选择 `preview|docker|local`；同键同意图重放返回原快照，改意图冲突，多 Store 通过 Run 写锁和 revision 再校验收敛。每次真实切换原子写入不可变快照、operation 与 `run.execution_profile_selected` 元数据事件。

三个档位的 backend、approval、filesystem/network、risk 和 required gate 都由 Go 闭合映射，SQLite 以 `CHECK`、绑定 trigger 和 update/delete 禁止再次锁定。`process_enabled`、`execution_authorized` 与 `capability_grant` 对所有档位均为 false。`docker` 仍需独立 `docker_production_start_gate`，`local` 仍需尚未实现的 `local_os_sandbox_gate`；选择不会调用 Runner、接触 Docker、启动宿主进程、批准工具或放宽 child Agent。CLI 增加 show/set，Run detail、OpenAPI 与生成 TypeScript 投影同一快照。

React 控制台新增三段式执行环境控件和可选 distinct control token。两个 bearer 都只驻留页面内存，control token 不进入 URL、请求 body、browser storage 或 read 请求；界面只提交 profile enum 与稳定幂等键，Go/SQLite 再检查 Run 状态、lease 与闭合映射。无 control token、Run 非 created/paused、活动 lease 或当前同档位时按钮禁用。ADR 0026 固化了“选择是意图，不是权限”的边界。

本轮定向测试覆盖 Domain 映射/篡改、schema v64 回填、SQL 不可变、幂等/冲突/lease、CLI、HTTP credential 分离/严格 JSON/OpenAPI live route、前端内存 token 与控件行为。最终代码全仓普通测试 225.9 秒通过；完整 race 196.9 秒通过，HTTP/Store/App v64 路径在最终 DTO 收窄后又通过定向 race。`go vet`、零告警 `staticcheck`、module verify/tidy diff、零可达漏洞 `govulncheck`、严格 TypeScript、9 个文件 21 项前端测试、production build、零漏洞 npm audit、OpenAPI/TypeScript 双次生成 SHA-256、README v1-v64 顺序、Markdown 链接、凭据/运行产物/乱码/diff 扫描和隔离真实 CLI smoke 均为绿色。烟测确认 Preview 默认值、Docker 门禁选择、同键重放、单条审计事件和零进程启动。实现提交 `8378419` 的 GitHub Actions run `29523634340` 也已通过，Go/Linux 用时 3 分钟，TypeScript 用时 26 秒。

代码审计修复三类低风险问题：共用 control body parser 不再返回 cancellation 专用误导文案；六处内部错误文本改为仓库要求的小写形式；HTTP/React profile DTO 删除不需要的 requester/reason 审计叙述，只在 SQLite/Event/CLI 保留。当前无未解决高/中风险。本轮未执行 Linux real-daemon harness，本机留下一个不受 Git 跟踪的系统临时烟测目录。架构完成度仍约 98%，产品可用度仍约 45-50%，因为用户现在可以预先表达执行环境，但真实进程仍关闭。原先预留给 Skill Registry 的 v64 已被本切片占用，Registry 顺延到 v65 或以后。

发布后的真实 production bundle 托管复核又发现一项低风险可用性问题：Vite 8 生成的 `index-D0TcvGy-.css` 使用 URL-safe 哈希且末尾恰好为 `-`，原先“取最后分隔符”的校验会把合法哈希判空，从而让 Go 拒绝整个 UI 目录。校验器现改为向前寻找至少 8 字符的 URL-safe 哈希段，主 bundle 测试直接使用该真实文件名，同时继续拒绝短后缀和非法字符。修复后全仓测试 201.1 秒、Web bundle 20 轮 race、vet/staticcheck 通过；真实二进制加载精确 `web/dist`，桌面与 390x844 移动视口均无横向溢出，UI 可切换 Docker 意图并恢复 Preview，执行与能力位始终为 false。该问题不构成路径或执行权限绕过。

schema v65 非授权 Docker 生产证据账本切片已完成实现，任务 ID 为 `P6-Docker-Production-Evidence-Ledger-v65`。Go 固定 `sandbox_docker_production_evidence.v1`、16 项 v51 probe、suite/environment/evidence/capture 指纹和三种当前平台 receipt。操作者只能对同一身份创建的 v63 阻塞审查提交稳定 operation key 与显式机器采集确认；请求没有 evidence item、结论、JSON 报告、socket、路径、镜像、resource、container ID 或 raw daemon response 字段。Windows 只记录 `unsupported_platform`；Linux 未 opt-in 时记录 `opt_in_required`，设置 `CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1` 后也只记录 `harness_pending`。当前所有分支均为零 daemon、零 Docker CLI、零网络、零进程调用。

SQLite v65 原子保存不可变 aggregate、恰好 16 条 item、摘要化 operation 与 metadata-only `sandbox.docker_production_evidence_captured` 事件，并复核 v63 review/Run/Mission/Workspace/操作者/authority/threat-model 全绑定；每个 Run 最多 32 份，迁移不伪造旧证据。同键同语义重放不再调用 collector，改语义冲突。CLI 新增 `docker-production-evidence-capture/captures/show`，输出只含有界状态、计数、摘要和全部 false 的权限位。即使未来 item 可表示 `production_verified`，`sufficient_for_start`、start/process/output/Artifact authority 仍由 Go 和 SQL 固定为 false。

本轮代码与安全审计发现并修复一个重要的未来接线风险：仅靠 Domain 校验时，自定义 collector 理论上可返回合法的 `capture_complete` 与 `real_daemon_contacted=true`，在尚无写前 harness attempt/lease 时留下真实 daemon 副作用。Application 现在硬拒绝这两种结果，并通过恶意 collector 回归证明拒绝后账本行数不变。SQL operation trigger 还新增 `key_digest = evidence.operation_key_digest` 的直接绑定和篡改回归，避免正常 Go 路径的一致性只停留在 Store 校验；Domain 多字段身份检查改为固定顺序并做 20 轮确定性回归。另修复 11 处 staticcheck 错误文本大小写问题。凭据扫描只命中既有合成脱敏夹具或长测试操作名；新增路径未出现 `os/exec`、`exec.Command`、进程 `.Start`、Docker start endpoint、`docker.sock`、`DOCKER_HOST`、网络请求或删除调用。ADR 0027 固化该边界，当前未发现未解决高/中风险。

最终本地门禁通过：最终代码全仓普通/race 测试分别用时 212.3 秒/213.9 秒，v65 Domain 普通/race 各 20 轮；`go vet`、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、9 个文件 21 项 Vitest、production build、零漏洞 npm audit、OpenAPI/生成 TypeScript 无漂移、README v1-v65 顺序、凭据/禁止能力/运行产物/diff 扫描和隔离真实 CLI schema-v65 Workspace smoke 均为绿色。smoke 临时目录因永久删除守卫拒绝递归清理而留给系统临时目录回收，无需人工操作。架构完成度维持约 98%，产品可用度维持约 45-50%，通用 Coding Agent 约 40%，Cyber 自动化约 20%；本切片提升的是可审计恢复边界，不是最终用户执行能力。

下一切片为 schema v66：在第一次真实 daemon 调用前持久化 evidence-capture attempt、过期 generation lease、固定 Linux endpoint、typed failure 与重启协调；随后才能实现 exact pre-existing digest/no-pull 的 16-probe Linux harness。机器 receipt 仍需独立 evidence acceptance review，之后才考虑 v63 的 start/wait/TERM/KILL/orphan 状态机；output/Artifact 与 Local OS sandbox 继续独立门禁，Skill Registry 顺延到 v66 或以后，预计 v67+。

GitHub Actions run `29532551701` 已通过 v65 实现提交 `e97daf0`：Go/Linux 作业 2 分 47 秒，TypeScript 作业 20 秒；Linux 全仓测试、vet、govulncheck 与前端完整门禁全部通过。

schema v66 可恢复 Docker 生产证据 Attempt 切片已完成实现，任务 ID 为 `P6-Recoverable-Docker-Production-Evidence-Attempt-v66`。Go 在任何 collector 调用前原子创建不可变 attempt、摘要化 operation 与 generation 1 lease；每一代还必须先写入 quiescent reconciliation checkpoint。采集 deadline 同时受固定 30 秒上限和 lease 到期余量约束。失败只持久化有界类型码并释放 lease；released/expired attempt 只能由 generation N+1 接管，reconciliation、failure 和 completion 都必须匹配完整私有 lease identity，旧 worker 无法提交。

成功路径把 v65 aggregate、16 项 item、v65 operation、v66 result、lease release 和 metadata-only 事件放在同一事务。新的 SQL trigger 拒绝任何没有 v66 attempt result 的 v65 evidence operation；历史 v65 receipt 保持可读且迁移不补造 attempt。CLI capture 输出关联 attempt，并增加 `docker-production-evidence-attempts`、`docker-production-evidence-attempt-show` 与显式确认的 `docker-production-evidence-attempt-resume`；公开输出不包含 lease ID/owner、raw error、socket、路径、资源/容器身份或 daemon payload。

定向测试覆盖 collector 内可见的写前顺序、活动租约冲突、释放恢复、到期接管、stale-generation fencing、unsafe daemon-contact failure、generation-two completion、SQL 旁路拒绝、v65 升级、不变性、隐私和 replay 零重采集。ADR 0028 固化边界。v66 reconciliation 只记录零 daemon reads 与零 known resources，它证明 ownership/order 而不证明 Docker 生产资源状态；在 v66 交付时 Windows/Linux collector 仍是 inert implementation，没有运行真实 daemon harness、Docker start 或宿主进程。架构完成度保持约 98%，产品可用度保持约 45-50%，通用 Coding Agent 约 40%，Cyber 自动化约 20%。

v66 最终本地门禁通过全仓普通/race（206.9 秒/230.3 秒）、vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、严格 TypeScript、9 个文件 21 项前端测试、OpenAPI/production build/npm audit、50 份 Markdown 文档链接、凭据/运行产物/乱码/进程入口/diff 扫描和隔离真实二进制 schema-v66 Workspace smoke。Domain/Store/Application/CLI 高频回归为 50/10/5/5 轮，关键 race 为 5/3/3 轮。审计修复 lease 行缺少 DELETE 禁止、direct-SQL release/takeover 时间线过宽，以及 capture list 尾随 `--limit` 无法解析三项问题；当前无未解决高/中风险。宿主受保护删除策略拒绝烟测目录的递归清理，目录留在系统临时根等待自动回收，无需人工操作。

GitHub Actions run `29538732903` 已通过 v66 实现提交 `3e52b7d`：Go/Linux 作业 3 分 33 秒，TypeScript 作业 25 秒；Linux 全仓测试、vet、govulncheck 与前端完整门禁全部通过。

schema v67 Linux 只读 Docker 生产证据 Harness 切片已完成实现，任务 ID 为 `P6-Bounded-Linux-Read-Only-Docker-Evidence-Harness-v67`。Application 只有在 v66 attempt、当前 lease 与零读取 control reconciliation 已持久化后，才写入不可变 harness intent。Linux 还要求 `CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1`；Windows 和未 opt-in 的 Linux 不接触 daemon。固定 transport 先按 `cyberagent.workbench.production-evidence-attempt=<attempt-id>` 精确查询容器并要求 empty scope，再持久化 daemon-aware reconciliation，随后依次 GET `_ping`、`version`、`info` 与精确已存在 digest 的 image inspect。

协议总共最多五次 daemon GET，每次最多四秒且仍受 30 秒 attempt deadline 限制；它忽略 `DOCKER_HOST`、禁用代理/重定向、响应上限 2 MiB，并且接口没有 pull/create/start/exec/remove/delete 方法。重复 JSON、额外 query、label 不匹配、重复 resource ID、foreign/pre-existing owned resource、非 Linux daemon/image、元数据或 digest 不匹配均失败关闭。原始 socket、payload、镜像仓库名、资源 ID、路径、operation key 和私有 lease 不落库；失败事件使用 `not_authorized|not_confirmed|confirmed` 区分 daemon contact 证据强度，不再在已尝试读取时误报 `false`。

v67 的 16 项结果全部固定为 `observed_failed`、`production_verified_count=0` 与 `sufficient_for_start=false`。Go/SQLite 双层绑定 intent、v66 control reconciliation、当前 generation daemon reconciliation、evidence/result/operation，并禁止有 v67 intent 的 attempt 退回 v66 inert result；released/expired 恢复进入 N+1 并重做 daemon-aware reconciliation。迁移不会给历史 v65 receipt 或 in-flight v66 attempt 伪造 harness。定向 Domain/Store/Application/HTTP/CLI 测试已经覆盖精确调用顺序、写前可见性、碰撞与畸形响应、代际绑定、不可变、隐私、兼容升级和 replay 零新调用。ADR 0029 固化边界。

本机为 Windows，本轮没有运行真实 Linux daemon、Docker start、Shell 或宿主进程。架构完成度仍约 98%，产品可用度仍约 45-50%，通用 Coding Agent 约 40%，Cyber 自动化约 20%；本切片把真实只读证据链接入产品，但没有增加终端用户进程执行能力。下一切片为 schema v68 独立 operator evidence acceptance/rejection 账本；接受 v67 receipt 仍必须保持 start blocked。Skill Registry 顺延到该审查之后，预计 v69+。

v67 最终本地门禁通过：全仓普通/race 分别 215.2 秒/233.1 秒，vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、9 个文件 21 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、51 份 Markdown、仓库隐私/产物/乱码/Docker 写入口/diff 扫描、隔离真实二进制 Workspace smoke 和 Linux test binary 交叉编译均为绿色。Sandbox/Store/Application/CLI 高频回归为 50/10/5/5 轮，关键 race 为 10/3/3/3 轮。审计收紧零生产验证、精确 selector、v66 control reconciliation、租约时序、direct-SQL 原子终态和 contact 文案，当前无未解决高/中风险。宿主策略拒绝递归清理，两个烟测目录留在系统临时根等待自动回收，无需人工操作。

GitHub Actions run `29543385038` 已通过 v67 实现提交 `8bc0929`：Go/Linux 作业 2 分 50 秒，TypeScript 作业 24 秒；Linux 全仓测试、vet、govulncheck 与前端完整门禁全部通过。

schema v68 不可变 Docker 生产证据操作员审阅切片已完成实现，任务 ID 为 `P6-Immutable-Docker-Production-Evidence-Review-v68`。Application 只接受 evidence ID、稳定 operation key、`accepted|rejected`、有界原因码、reviewer 和显式确认；接纳只能使用 `metadata_scope_accepted`，拒绝只能使用五个固定原因。请求、表、事件和 CLI 不含自由文本、上传报告、daemon payload、socket、路径、镜像仓库名、容器/资源身份、原始 operation key 或私有 lease。

Go 从 Store 重建精确 v67 source chain，拒绝 v66 inert result、未完成 attempt 或无 harness receipt。SQLite 以延迟外键和互相约束的 trigger 保证摘要化 operation 与 review 必须在同一事务成对提交，并绑定 v63 阻塞审查、v65 receipt 与 16 条 `observed_failed` item、v66 attempt、v67 harness result、Run/Mission/Workspace 及全部指纹。每份 evidence/attempt 只能有一次不可变决定，迁移不伪造历史审阅，同键同语义重放不追加事件或重新接触 daemon，改语义冲突。

即使 `accepted`，v68 也只设置 `receipt_accepted=true`；`production_verified_count=0`、`sufficient_check_count=0`、`blocker_count=16`，start gate、容器启动、进程执行、输出导出和 Artifact 提交仍全部为 false。review 路径没有 Docker transport、模型调用、Shell 或宿主进程。CLI 新增 `docker-production-evidence-review/reviews/review-show`，只输出有界元数据。定向 Domain/Store/Application/迁移/SQL/幂等/隐私/CLI 测试通过，边界见 ADR 0030。

独立审计发现并修复两项中低风险审计健壮性缺口和一项低风险覆盖缺口：review root 现持久化并自校验完整 request fingerprint，SQL 要求 operation 与 root 指纹一致，Store 每次读取/重放还会重新计算并复核语义绑定；新增 review-only、operation update/delete、gate/Mission/Workspace/capture/harness/权限篡改负向矩阵和双 Store 并发收敛；`rejected` 已贯通 Store、事件、Application、CLI、list/show 和重放。当前没有已知未解决高/中风险。

v68 最终本地门禁通过：全仓普通/race 分别 247.9 秒/276.3 秒，vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、9 个文件 21 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、57 份 Markdown/74 条相对链接、用户测试 key/乱码/禁止执行入口/diff 扫描、Linux test binary 交叉编译和隔离真实 CLI schema-v68 smoke 均为绿色。Domain/Store/Application/CLI 高频回归为 50/10/5/5，关键 race 为 10/3/3/3。本机未运行真实 Docker、容器 start、Shell 或宿主进程；一个交叉编译文件和一个 smoke 根留在系统临时目录等待回收，无需人工操作。GitHub Actions run `29552080990` 已通过实现提交 `41583ac`，Go/Linux 用时 2 分 57 秒，TypeScript 用时 24 秒。

架构完成度保持约 98%（V2 约 99%），产品可用度保持约 45-50%，通用 Coding Agent 约 40%，Cyber 自动化约 20%。本切片补齐了审计决策，不增加终端用户执行能力。下一唯一推荐切片转回 P7 schema v69：建立内容寻址的本地用户 Skill Registry，只安装已经通过 `skill_package.v1` 校验的惰性包；导入阶段禁止脚本、钩子、命令、Provider、网络、工具和能力授予，并继续隔离 Code/Cyber 目录。

schema v69 内容寻址惰性用户 Skill Registry 切片已完成实现，任务 ID 为 `P7-Content-Addressed-Inert-User-Skill-Registry-v69`。CLI 新增显式确认的 `skill import`、metadata-only `skill installed/installed show` 和显式确认的 `skill remove`。Go 只接收已通过 ADR 0024 严格 parser 的确定性双文件 ZIP，内置名不可覆盖，Code/Cyber Catalog 分离，Cyber 只接受精确 `script` Profile。外部包固定为 `operator_installed_untrusted`，导入命令、网络、Provider、工具授予、Run 选择和上下文注入全部为 false。

SQLite 新增安装 operation/intent/result 与移除 operation/tombstone 五类不可变表。摘要化 operation 先写，延迟外键和互相约束 trigger 防止任一半单独提交；安装意图在对象发布前持久化，同键重试恢复中断，改意图冲突，两个独立 Store 收敛。Registry 最多保留 64 个历史包身份和同名 8 个版本。移除只追加 tombstone，内容对象继续保留；Go 与 SQL 都拒绝移除已经被 `run_skill_selection_items` 以 name/version/content hash 精确固定的版本，已移除包不能无显式 restore 协议静默重装。

本地对象存储固定在 `$CYBERAGENT_HOME/skill-registry/objects/sha256/<prefix>/<digest>.zip`，接口只有 `Put`/`Verify`。实现使用 `os.Root`、同目录独占临时文件、file sync、原子 hard link 与完整回读；已有或新对象都必须重新匹配普通文件身份、字节数、archive SHA-256、严格 ZIP 结构和语义 package fingerprint。symlink、腐坏、读取中替换、伪造收据和取消全部失败关闭。正文、源路径和 raw operation key 不进入 SQLite、Run event 或 CLI；v69 没有向 Agent 暴露内容读取器。

本轮全仓普通/race 最终用时 259.7 秒/275.3 秒；`go vet`、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、OpenAPI 无漂移、9 个文件 21 项前端测试、Vite production build 与零漏洞 npm audit 全部通过。对象/Store/Application/CLI 定向 race 通过三轮，两个真实 Service 各自生成不同候选 ID/时间的导入与移除收敛通过 20 轮普通和 10 轮 race。首次 Linux CI run `29556933994` 暴露 `os.Root.MkdirAll` 并发准备嵌套目录失败；对象存储现逐级 `Mkdir`，只在 `Lstat` 证明是非 symlink 真目录后接受既有项，12 个独立 Store 的对象回归通过 100 轮普通与 20 轮 race，Linux 测试二进制也已交叉编译。GitHub Actions run `29557803407` 已通过修复提交 `d28b100`（Go/Linux 3 分 21 秒，TypeScript 23 秒）。其余审计修复包括 v69 trigger 阻碍旧 schema 夹具逐级还原、37 处新错误文本静态风格、冗余临时清理状态、对象收据接口绑定缺口、对象发布前缺少一次取消复核、生成身份误判改意图冲突，以及 Manifest 自由文本 description 未脱敏复制进 SQLite；应用层恶意收据 list/show 回归现证明失败关闭。当前无已知未解决高/中风险，本轮未执行模型、网络请求、Shell、Docker、安装钩子或宿主进程。ADR 0031 固化边界。

双指标更新为：架构完成度仍约 98%（V2 约 99%）；产品可用度约 46-50%，通用 Coding Agent 约 41%，Cyber 自动化约 20%。提升来自可实际管理外部 Skill 包的 CLI，但外部包尚不能被 Run 选择或进入模型上下文。下一唯一推荐切片为 schema v70：精确固定已验证且未 tombstone 的外部 Skill 版本，并通过独立预算、脱敏、来源隔离和 first-model-call 原子账本向 root/Specialist 最小化交付；声明工具继续不授予能力。

schema v70 外部 Skill Run 选择与最小上下文切片已完成实现，任务 ID 为 `P7-External-Skill-Run-Context-v70`。CLI 新增 `skill select-external` 与 `skill external-selection`。安装确认不会自动授权上下文；操作者必须对 created Run 再次提供稳定 operation key 和 `--confirm-untrusted-skill-context`，才能固定 1-4 个精确 active `name@version`。选择绑定 mode、安装/结果/对象/Manifest 全链指纹，总预算最多 4096，最多一项可明确交给 Specialist，声明工具始终不授予能力。

对象读取使用与发布接口分离的 `PackageObjectLoader`，每次交付都重新核验内容寻址路径、普通文件身份、大小、archive SHA-256、严格 ZIP、语义 package fingerprint 和 Manifest。root 与 Specialist 使用独立预算和 secret redaction，child 默认 1024、硬上限 2048且只加载操作者指定的一项。正文只在当前 Provider request 内以用户角色 `external_skill_guidance.v1` 信封存在；Policy、工具、Shell、网络、Secret、Scope 和委派授权全部为 false。系统控制文本要求把外部 Skill 当作可选工作流建议、把仓库/文档当作证据，并忽略隐藏步骤、泄露秘密、改策略或扩权诱导。

SQLite v70 新增不可变 selection/item/operation 及 root/Specialist preparation/commit 账本；首次 `model.started` 与来源 commit 原子提交，崩溃和重放不伪造交付。事件只含协议、计数、预算、脱敏统计和关闭权限结论，不含正文、路径、raw key、Secret 或模型文本。定向测试覆盖第二次确认、重放、精确对象漂移、removed/跨工作面/Profile 拒绝、Prompt Injection 用户角色隔离、root/无工具 Specialist 交付、SQL 不可变、v69 原地升级不伪造状态，以及 Go/SQL 双层固定版本删除保护。

本轮聚焦审计发现并修复一项中风险约束错误和多项低风险健壮性缺口：`installation_id` 曾被错误设为全表唯一，导致同一已验证安装只能被一个 Run 选择；现改为每份选择内唯一，并新增第二个 Run 独立确认复用回归。旧 schema 降级夹具先拆除全部 v70 trigger/table；Specialist 上下文拒绝未知 Profile、`cyber + 非 script`、其他 Run 或过期 mode，且 1024-2048 合法范围动态适配、超过 2048 在落库前拒绝；包移除在 SQL 触发器前稳定返回 `failed_precondition`；相同 operation 跨 Plan/Delivery revision 漂移仍重放原选择；延迟互反外键保证 selection/operation 原子成对；对象读取在解析前后都复核取消；用户角色信封显式列出全部关闭权限位。当前无已知未解决高/中风险；本轮没有新增真实模型、网络、Shell、Docker start、安装钩子或宿主进程执行。ADR 0032 固化边界。

v70 最终本地门禁通过：全仓普通/race 分别 197.6 秒/264.4 秒，vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、9 个文件 21 项前端测试、OpenAPI 无漂移、production build、零漏洞 npm audit、凭证/运行产物/乱码/43 份 Markdown 链接/diff 扫描和隔离真实 CLI schema-v70 smoke 均为绿色。Prompt Injection 审计结论分层记录：Go/Policy/Tool Gateway 能硬阻断外部 Skill 借正文获得工具、Shell、网络、写文件、Secret、Scope 或委派权限；模型对事实或工作流的语义误判仍是残余风险，依赖用户角色来源、证据优先提示和操作者复核降低，不能宣称绝对消除。宿主保护策略拒绝递归清理烟测目录，该目录仅位于系统 `%TEMP%` 并等待自动回收，无需人工操作。GitHub Actions run `29566538449` 已通过实现提交 `edc4073`，Go/Linux 3 分 42 秒，TypeScript 21 秒。

双指标更新为：架构完成度仍约 98%（V2 约 99%）；产品可用度约 48-52%，通用 Coding Agent 约 43%，Cyber 自动化约 20%。提升来自外部 Skill 已能真实固定并安全进入 root/Specialist 工作流；HTTP/TUI/Web 仍未投影该来源，真实 Sandbox、完整写工具和 Cyber 工具链仍关闭。下一推荐切片为 schema v71 metadata-only 跨界面来源投影，随后再独立审计 Desktop Go-owned 本地上传预览与 Docker 真实进程状态机。

schema v71 有界外部 Skill 来源与交付只读投影切片已完成实现，任务 ID 为 `P7-Bounded-External-Skill-Provenance-Projection-v71`。Go 新增严格 `external_skill_projection.v1` 类型，SQLite 新增两张只读 view，并通过 Store 将一份选择投影成最多四个条目和一个 Specialist。公开字段仅包含 surface/profile、模式修订、token 预算/上界、名称/版本、固定信任类别、声明工具数量、操作者已确认/上下文已授权的历史事实，以及 root/Specialist preparation/commit 计数；`tool_capability_grant` 固定为 false。

HTTP 新增 `GET /api/v1/runs/{run_id}/external-skills`，并在 Run detail 中可选嵌入同一投影；OpenAPI 扩展为 26 个 path、61 个 schema、23 个只读 GET 和原有三个 control POST。TUI 增加只读 Skills activity，React Run Overview 增加响应式 External Skills panel。三个界面都没有安装、选择、审批、授权或执行控件，且 DTO/类型从结构上排除正文、路径、字节数、所有 hash/digest/fingerprint、选择/安装/模式快照 ID、operation key，以及操作者/请求者/attempt/agent 身份。

定向测试已覆盖投影字段、clone 隔离、空 Run、跨 Run 范围、SQLite view 不可写、v70 原地升级不伪造选择或事件、HTTP endpoint/Run detail、OpenAPI 隐私、TUI 安全显示和 React 无 mutation 渲染。切片审计进一步把四个 preparation/commit 计数子查询都显式绑定 `preparation.run_id = selection.run_id`，减少对间接 ID 约束的依赖并改善查询作用域。本切片只读取元数据，没有打开 Skill 对象、执行正文、调用模型/网络/工具、接触 Docker、启动 Shell/宿主进程或写入新的 Run 事件。

v71 最终本地门禁通过：全仓普通/race 分别用时 227.1 秒/301.1 秒；vet、零告警 staticcheck、module verify/tidy diff、零可达漏洞 govulncheck、OpenAPI/TypeScript 双次生成哈希一致、严格 TypeScript、9 个文件 22 项前端测试、production build、零漏洞 npm audit、用户测试 key/运行产物/生产 `exec.Command`/乱码/54 份 Markdown 与 78 条相对链接/diff 扫描和隔离真实二进制 schema-v71 Workspace smoke 均为绿色。最终健壮性审计补充了有效 Run 无外部选择时 detail 必须省略 `external_skills` 且独立 endpoint 必须返回 404 的负向普通/race 回归（14.7 秒/17.6 秒）。当前无已知未解决高/中风险；未调用真实 Provider、Agent-controlled Shell/宿主进程、Docker、安装钩子或外部网络。smoke 根位于系统临时目录等待正常回收，无需人工操作。GitHub Actions run `29574167659` 已通过实现提交 `3947bea`，Go/Linux 2 分 56 秒，TypeScript 25 秒。

双指标更新为：架构完成度仍约 98%（V2 约 99%）；产品可用度约 49-53%，通用 Coding Agent 约 44%，Cyber 自动化约 20%。提升来自操作者现在能在 CLI、TUI 和 Web 一致核对外部 Skill 的来源、预算与实际交付状态；真实 Sandbox、完整写工具和 Cyber 工具链仍关闭。下一推荐切片是 Desktop D1-A 的 Go-owned 本地包验证预览，只校验并显示风险元数据，不安装、不持久化、不执行、不联网、不调用模型/工具。真正安装 mutation 继续作为后续独立安全审计切片。

非 schema Desktop D1-A 路径隔离 Skill 包预览边界已完成，任务 ID 为 `P7-Desktop-Pathless-Skill-Package-Preview-D1A`，SQLite schema 继续保持 v71。原先只存在于 CLI 内部的普通文件读取逻辑已经抽为 `skills.ReadPackageFile`/`ValidatePackageFile`，CLI 校验与导入共同复用；它拒绝空白改写、symlink、目录、空包与 64 KiB 以上包，在打开前、打开后和读取后复核普通文件身份、大小及修改时间，并在所有错误中隐藏操作者选择的路径。

新增 `internal/desktop` Go 边界，但没有新增桌面二进制或 Wails 依赖。构造器返回两个刻意分离的值：未来原生壳保留一个接收 OS 对话框路径的 Go selector closure；可绑定渲染层的 bridge 只有 `Preview(handle)`，不能接收路径或文件字节。selector 在发放句柄前完成严格包校验并立即丢弃路径/正文，只保存有界 DTO。句柄由 256-bit 随机数生成，URL-safe、五分钟过期、最多同时 16 份并且原子单次消费；取消预览不会消费。DTO 不含文件名/路径、正文、Manifest description/content path/content digest，安装、命令、hook、网络、Provider、工具与 capability authority 全部为 false。

最终全仓普通/race 分别通过于 255.8 秒/314.0 秒；Desktop 普通 100 轮/race 25 轮、Skill 文件边界 100 轮、CLI 包链 10 轮通过，32 方并发读取同一句柄恰好一方成功。`go vet`、零告警 `staticcheck`、module verify/tidy diff、零可达漏洞 `govulncheck`、OpenAPI 无漂移、9 个文件 22 项前端测试、production build、零漏洞 npm audit、60 份 Markdown/79 条相对链接和仓库隐私扫描全绿。代码审计补强了首尾空白路径拒绝、CLI 稳定退出码和 JSON 精确字段白名单；还发现 `govulncheck` 与 `npm ci` 不应在同一工作树并发，因为 Node 依赖原子替换会让 Go `./...` 扫描看到瞬时缺口，门禁已改为串行复跑且全部通过。这是本地测试编排竞态，不是产品缺陷。GitHub Actions run `29578985787` 已通过实现提交 `45c047c`（Go/Linux 3 分 13 秒，TypeScript 27 秒）。当前无已知未解决高/中风险；未调用模型、网络、Shell、宿主进程、Docker 或安装钩子。

D1-A 交付时双指标保持架构完成度约 98%（V2 约 99%）、产品可用度约 49-53%、通用 Coding Agent 约 44%、Cyber 自动化约 20%。该轮只建立未来 Desktop 的安全文件边界，尚无用户可见桌面入口，因此不提高产品可用度；后续 D0-A 已完成下述可见壳。安装、HTTP upload、注册表、安装包、自启动和更新继续后置。

非 schema Desktop D0-A 嵌入式只读 Windows 壳已完成，任务 ID 为 `P9-Embedded-Read-First-Wails-Desktop-D0A`，SQLite schema 保持 v71。项目评估后固定 Wails v2.13.0 稳定版；v3 Alpha 不进入主线。`cmd/cyberagent-desktop` 只在 Windows `desktop` tag 下编译，`web/assets_desktop.go` 把 production Vite bundle 编译进可执行文件，`webui.LoadEmbeddedFS` 对 index 与内容哈希资源执行类型、数量、单项/总大小和普通文件快照校验。

桌面没有监听端口。Wails AssetServer 直接调用既有 `httpapi.API` Handler，适配层 clone 请求、固定 loopback Host/RemoteAddr，并只规范化实机发现的 empty root 和无正文 GET `ContentLength=-1` 两种 Wails v2.13 语义；未知正文不会被清空。普通 Web 继续 SSE，Windows Desktop 因 Wails v2 AssetServer 不支持 response streaming 而改用同一 Run event 表的有界 opaque-cursor polling。

Renderer 的完整原生绑定被反射测试锁死为 `Bootstrap`、`SelectSkillPackage`、`PreviewSkillPackage` 三项。默认只生成内存 read token；显式 `--enable-profile-control` 才生成不同的 control token，且只调用原 schema v64 非授权档位 route。进程、Shell、Docker、Skill 安装和 renderer path input authority 全部为 false。原生 `.zip` picker 串行打开，路径只交给 Go selector；React 只取得五分钟单次句柄和 bounded preview。Token 不写 Local Storage、SQLite、日志、输出或注册表。

产品复核修复了两个真实 Wails 兼容问题：AssetServer 根请求的 URL path 可为空；无正文 GET 会以 `ContentLength=-1` 到达 Go。两者都以窄适配和回归固定。最终 UI 还移除了 Desktop 中“断开后自动重连”的无效按钮，并增加 control/read token 不得相同的前后端复核、modal 初始焦点和 path-free native startup failure dialog。单实例恢复、WebView file/drop denial、默认右键菜单禁用、renderer code integrity 与现有 CSP 均启用。

production-tag Windows binary 已在隔离 schema-v71 home 实机启动；1440x900 主窗口、自动连接、Run/Session 空态、Skill preview modal 和原生 `.zip` 对话框均无 blank、重叠或水平溢出。最终本地全仓普通/race 分别为 205.1 秒/293.9 秒；普通与 desktop-tag vet/staticcheck/零漏洞 govulncheck、module verify/tidy、确定性 OpenAPI、严格 TypeScript、12 个文件 31 项前端测试、production build、零漏洞 npm audit、61 份 Markdown/82 条相对链接、凭据/产物/乱码/禁止执行入口/diff 扫描全绿。Desktop 聚焦包通过 50 轮普通和 10 轮 race。最终忽略的未签名 GUI 为 21,022,208 字节，SHA-256 `6b355cfa72b41d225e62ed58ac24cb9493bbf2a71f4d45120e6f0dbf5308ad0c`。GitHub Actions run `29602281365` 已通过实现提交 `2c0b81c`：Go/Linux 4 分 57 秒、TypeScript 26 秒、新增 Windows Desktop 构建/测试作业 4 分 27 秒。当前无已知未解决高/中风险。D0-A 没有运行 Agent-controlled Shell/Local/Docker、安装 Skill、调用 Provider 或新增网络 Scope；唯一启动的宿主进程是人工可见、隔离数据目录下的待测桌面二进制本身。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 52-56%、通用 Coding Agent 约 46%、Cyber 自动化约 20%。下一推荐切片为 Desktop D0-B：同库 CLI/Desktop 并发、崩溃重开、单实例恢复、event polling 续传、WebView2 缺失诊断和 navigation/origin 自动化；继续只读且不做安装器。边界见 ADR 0034。

非 schema Desktop D0-B 生命周期与事件续传加固已完成，任务 ID 为 `P9-Desktop-Lifecycle-And-Event-Resumption-D0B`，SQLite schema 继续保持 v71。`desktop.ControlPlane` 统一拥有 Desktop Store 与进程内 API，无监听端口；测试覆盖 CLI 独立连接写入、Desktop 同库读取、关闭重开、六路并发打开和幂等关闭。`desktop.Lifecycle` 合并启动前第二实例信号，丢弃其参数/工作目录，只恢复现有窗口，并以专用锁把原生恢复与 Stop 串行化，停止后不可重启。

HTTP 新增只读 `run-event-poll.v1`：返回真实 `run-events.v1` frame、与 SSE 相同的 Run-bound 高水位 cursor、最多 100 条和严格 `has_more`；未知/重复 query、非法 limit、跨 Run cursor 与 `Last-Event-ID` 全部拒绝。React Desktop 改为消费该契约，最多在模块内存保留 16 个 Run、每个 500 帧，组件重挂载从已确认 cursor 恢复，失效 cursor 每次挂载最多回退一次，不再制造 `desktop-poll-*`，也不使用 Local/Session Storage。OpenAPI 当前为 27 个 path、62 个 schema、24 个只读 GET 与原有 3 个 control POST。

secure Desktop 构建必须显式使用 `desktop,production,wv2runtime.error`。WebView2 `94.0.992.31` 以上只读预检在 bundle/数据库之前执行，缺失、过旧或探测失败均返回 bounded path-free `FAILED_PRECONDITION`，不会下载、安装或打开 URL。进程内适配器只接受精确 `http://wails.localhost`，clone 后清除 origin、无条件规范化 `RequestURI` 并固定 loopback；Desktop renderer 阻止外部链接、表单与 popup，普通浏览器不受影响，Wails start-origin 仍是原生绑定权限边界。

本轮最终完整普通/race 分别通过于 256.6 秒/273.5 秒；审计后生命周期 race 又通过 20 轮。普通与 secure Desktop tag 的 vet/staticcheck/零漏洞 govulncheck、module verify/tidy、确定性 OpenAPI->TypeScript、严格 TypeScript、13 个文件 37 项前端测试、production build 和零漏洞 npm audit 均通过。第一次 Desktop-tag 扫描发现 `x/net/html@v0.54.0` 五项新可达漏洞通告；升级到修复版 `x/net@v0.55.0`、解析到 `x/sys@v0.45.0` 后，两条最终扫描均为零。Windows 11 Pro x64 10.0.26200 隔离实机烟测验证最终 19,572,224 字节二进制（SHA-256 `f26ea87f42701a7eba8efa789900ea6953ef3c1533ff95106ec4b8e6b02b1160`）的主实例存活、第二实例让位、强制结束、同库重开和零残留进程。GitHub Actions run `29609621468` 已通过实现提交 `c9b1c66`，Go/Linux、Windows Desktop、TypeScript 分别用时 5 分、4 分 21 秒、23 秒。代码审计修复失效 cursor 可重复回退、来源 `RequestURI` 未无条件规范化、原生 restore/Stop 窄竞态三项低风险问题；无未解决高/中风险。Windows 10 实机矩阵仍待正式发行前补齐；没有运行 Agent-controlled Shell/Local/Docker、Provider、Skill 安装或外部网络。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 53-57%、通用 Coding Agent 约 47%、Cyber 自动化约 20%。下一推荐切片为 Desktop D1-R1：先增加 Go-owned Run 创建与自动 Session 绑定的窄 control route，不调用模型、不扩展 Wails native bridge；Session message/steering 作为随后独立审计。边界见 ADR 0035。

schema v72 / Desktop D1-R1 幂等受控 Run 创建切片已完成实现，任务 ID 为 `P9-Idempotent-Controlled-Run-Creation-v72-D1R1`。Go 新增不可变 `run_creation.v1` operation 账本，只持久化域分隔幂等键摘要、请求指纹和 Mission/Run/Session/Workspace 身份。Application 仅接受已注册 Workspace、1-4096 UTF-8 字节目标和规范 Profile/Surface/Phase，并在一个 immediate SQLite 事务中创建脱敏 Mission、交互式 created Run、活动 Session、revision-one mode、`preview/noop` execution profile、root Agent、精确初始事件及 operation。网络固定 disabled、目标为空、预算固定默认值、model route 固定等于 Profile；SQL trigger 独立复核整条绑定并拒绝 operation 更新/删除。

HTTP 新增 distinct-control `POST /api/v1/runs` 和 read-bearer `GET /api/v1/workspaces`。写入口要求恰好一个 `Idempotency-Key`、严格 Content-Type、零 query、有界 body、无未知/重复字段和无尾随 JSON；相同 key/语义跨重启和跨连接收敛到原对象，不同意图冲突。Workspace DTO 只含 ID、名称和创建时间，不公开 root path。OpenAPI 更新为 28 个 path、65 个 schema、25 个 GET 与 4 个 control POST。

Desktop 新增独立 `--enable-run-creation`，与 `--enable-profile-control` 分别控制 capability；creation-only token 不能访问取消或档位 route，Wails 原生 bridge 仍精确保持三个方法。React 新增 New Run 对话框、Workspace/Profile/Surface/Phase 选择、内存内不确定失败 key 重放、UTF-8 字节预检、请求-响应 Workspace/Profile/模式复核、默认预算/关闭权限校验、Run/Session 刷新与新 Run 选中。token 与 operation key 不进入 URL、Local Storage 或 Session Storage。

切片审计发现并修复多类健壮性问题：creation-only control token 与旧取消/档位 capability 完全拆分；重放会重新计算已存图请求指纹并验证完整初态；历史 schema 夹具先移除 v72 trigger 再拆旧 profile 表，避免降级测试失败和 SQLite 文件锁；v72 trigger 进一步锁定初始 updated_at、root 状态和四条事件总数；HTTP 在 Go JSON 解码前拒绝非法 UTF-8，并改用窄 Store 契约而不继承无关 Run transition 权限；Domain 多字段错误顺序固定；React 同时绑定请求 Goal/Workspace、初始 Run/Session 状态、Mode Profile/Surface/Phase/revision/policy、默认预算和关闭权限，并按 Go 的 4096 UTF-8 字节上限预检多字节目标。当前未调用真实 Provider、Agent-controlled Shell/宿主进程、Docker、Skill 安装钩子或外部网络，也未取得 execution lease。边界见 ADR 0036。

v72 最终本地发布门通过：全仓普通/race 分别 271.5 秒/257.9 秒；普通与 secure-Desktop 测试、vet、零告警 staticcheck、module verify/tidy diff、双路径零可达漏洞 govulncheck、严格 TypeScript、14 个文件 45 项前端测试、Vite 与 Windows production build、零漏洞 npm audit、OpenAPI/TypeScript 双次生成哈希一致、63 份 Markdown/86 条相对链接、凭据/运行产物/生产 `exec.Command`/乱码/diff 扫描和隔离 schema-v72 CLI smoke 均为绿色。一次与四项分析并行的 Desktop-tag 测试只触发 244 秒外层超时且没有失败输出，独立重跑在 267.2 秒明确通过；该编排超时不计为产品失败。当前没有已知未解决高/中风险；烟测根位于系统临时目录并等待正常回收，无需人工操作。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 55-59%、通用 Coding Agent 约 49%、Cyber 自动化约 20%。下一推荐切片为 Desktop D1-S1：复用既有 RunSupervisor、持久化引导队列、预算、Policy、脱敏、operation key 和事件语义，增加现有 Run-bound Session 的 Go-owned message/steering mutation；继续不建立 Desktop-only 执行路径，不启动进程。

非 schema Desktop D1-S1 受控 Session message submission 三切片批次已完成，任务 ID 为 `P9-Controlled-Session-Message-Submission-D1S1`，SQLite schema 保持 v72。第一片新增窄 `SessionMessageSubmissionService` 与 `session_message_submission.v1`：Go 重新加载 Session 和精确绑定 Run，复用 v45-v46 的 UTF-8 上界、脱敏、digest-only 幂等 enqueue 和事件语义。HTTP 新增独立 capability 的 `POST /api/v1/sessions/{session_id}/messages`，严格拒绝 query、非精确 JSON、重复/未知字段、非法 UTF-8、尾随数据、缺失/重复幂等 header 和超限 body；响应只含 Run/Session/queue 元数据与四项 false authority，不包含正文、摘要、operation、requester 或 lease。

第二片新增 Desktop `--enable-session-messages`、bootstrap capability 和同进程 Go Handler 接线；它与档位、Run 创建及取消 capability 独立，Wails bridge 仍精确三个方法。第三片新增 React Session composer：只在 capability 开启、Session 精确绑定且 Run 为 running/paused 时显示，按 16 KiB UTF-8 字节预检；同内容的不确定失败只在内存复用 operation key，编辑换 key，成功清空正文/key，Local/Session Storage 始终为空。界面只显示 `Queued #sequence`，不会从响应回显正文。

本批统一功能门已通过：最终代码全仓普通 Go 直接运行 255.6 秒，Desktop-tag 聚焦包 80.5 秒，补齐普通 `api serve` 接线后的 App/Application/HTTP/Desktop 回归 65.2 秒，15 个前端文件 52 项测试、严格 TypeScript、Vite production build 和 Windows production Desktop build 全绿。OpenAPI/TypeScript 契约已确定性再生成，当前仍为 28 个 path，增加到 67 个 schema、25 个 GET 与 5 个 control POST。本批没有调用 Provider、模型、工具、Shell、Docker、外部网络、宿主执行进程或 execution lease；成功只表示消息已排队/重放，不会启动、恢复或 drain Run。GitHub Actions run `29633205163` 已通过实现提交 `3ecb22a`：Go/Linux 3 分 6 秒、Windows Desktop 1 分 38 秒、TypeScript 25 秒；远端同时通过 vet、govulncheck 与 npm 依赖审计。边界见 ADR 0037。

按用户确认的新节奏，本批是第一个“三切片后统一功能测试”批次；完整 race、vet、staticcheck、govulncheck、依赖和扩展隐私健壮性门延期到下一批三个切片完成、累计六片时执行，并非跳过。双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 57-61%、通用 Coding Agent 约 52%、Cyber 自动化约 20%。下一批依次为 D1-S2 pending-only steering cancellation、D1-L1 operator Run start/pause/resume 和 D1-X1 Go-owned bounded execution handoff；第六片结束后执行完整健壮性门。

非 schema D1-S2 与 schema v73 D1-L1/D1-X1 三切片批次已完成，任务 ID 为 `P9-Run-Control-And-Bounded-Handoff-v73`。D1-S2 增加独立 `session_steering_cancellation.v1`：HTTP/Desktop/React 只能取消精确 Session/Run 下仍为 pending 且未 prepared 的消息；公开 `prepared` 由 Store 只读派生，不可借 UI 改写投递账本。D1-L1 增加不可变摘要幂等 lifecycle operation，执行 `created -> preparing -> running`、`running -> paused` 和 `paused -> running`，暂停前复核 execution lease、Agent、Supervisor turn 与 prepared delivery。延迟重放返回原 operation 与 Run 当前状态，后续合法转换不会让重试失败。

D1-X1 增加 `run_execution_handoff.v1`。准备事务按 sequence 冻结精确 Session 的前 1-8 条 pending 身份，零条时以 `queue_empty` 直接完成且不取得 lease/调用模型；非空批次只复用既有 RunSupervisor、Policy、累计预算、模型/工具账本、execution lease、checkpoint 和事件。执行中新增消息不会进入本批，冻结项若在 delivery 前被取消则跳过；root `finish`/`wait`、选择耗尽或类型化错误终止批次。completion 绑定精确 lease ID/generation、Run 状态、stop/error、steps、四类队列计数和 model/tool intent；旧 generation 或改意图重放冲突。HTTP 响应不含正文、模型输出、工具参数、raw key 或 lease 身份。

Desktop 新增 `--enable-session-steering-control`、`--enable-run-lifecycle` 和 `--enable-run-execution` 三个独立 flag，Wails native bridge 仍精确三个方法。React 增加 pending-only Cancel、Start/Pause/Resume 和 `maxSteps=1..8` 的 Run Queue 控件。每类不确定失败重试 key 只驻留内存，并在消息/动作/步数意图变化时轮换。普通 `api serve` 在存在 distinct control token 时也通过相同 Go route 开放这些能力。

累计六切片完整健壮性门已通过：最终全仓 ordinary 268.2 秒、race 295.3 秒；ordinary/secure-Desktop vet 与零告警 staticcheck、双路径零可达漏洞 govulncheck、module verify/tidy diff、确定性 OpenAPI/TypeScript、16 个文件 66 项前端测试、strict TypeScript、Vite production build、Windows production Desktop build、零漏洞 npm audit、凭据/运行产物/乱码/危险入口扫描均为绿色。最终未签名 GUI 为 20,849,664 字节，SHA-256 `ce3ff2b4609068de996b6362e3a5008c4d2348eae73c48ad0661c4e22739eba5`。

组合审计修复五项有实际回归价值的问题：旧 lifecycle operation 在 Run 已继续推进后被响应 parser 错拒；prepared pending 项曾误显示取消按钮；handoff completion 曾把旧 lease 或改终态意图当重放；`maxSteps` 改变曾复用旧前端 key；一项前端回归读取了错误 mock 参数而形成假阳性。另修复 staticcheck 的未使用测试值和错误文本大小写。首次 ordinary 计时封装不会传播子命令退出码，已改为直接执行并明确通过；这是门禁编排问题，不是产品缺陷。当前无已知未解决高/中风险，边界见 ADR 0038。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 61-65%、通用 Coding Agent 约 56%、Cyber 自动化约 20%。下一批三个切片为 D1-M1 Desktop Go-owned Provider/model 路由、D1-P1 Plan 三选一与显式 Deliver 转换、D1-A1 durable approval queue 决策；后台调度、真实进程、Diff apply 和 Skill 安装继续后置。

非 schema D1-M1/P1/A1 三切片批次已完成，任务 ID 为 `P9-Model-Plan-Approval-Controls-D1MPA1`，SQLite schema 保持 v73。D1-M1 新建统一 `internal/modelregistry`，CLI、普通 API 与 Desktop 使用同一环境 Provider/持久化路由初始化；`model_availability.v1` 只读投影固定四个 Provider 与五条路由的有界状态，不含 API key、Base URL、环境变量名或 raw error，也不做网络探测/模型调用。审计后又增加 secret-like/异常模型标识失败关闭和输出脱敏。

D1-P1 新增 `plan_delivery_control.v1` 两条独立 mutation。方向选择绑定 exact Run/proposal/1..3，复用既有 selection/WorkItem/handoff Note 原子事务并保持 phase 不变；进入 Deliver 必须已有选择，复用 Run mode digest-idempotent operation。两条响应固定 model/tool/execution/capability false。D1-A1 新增最多 100 项的 `approval_queue.v1` 和 `approval_control.v1`：公开队列不含命令、参数、路径、文件内容、指纹、原因或 operation；approve-once 重载 source 并重新执行当前 Policy，只接受 dry-run Shell 或 process-disabled ScriptProcess，replace-file 只能 deny，永久拒绝不能覆盖，且不创建 Grant、写文件或启动真实进程。已提交决策在 Run 终态后仍可同键重放，新终态决策继续拒绝。

Desktop 增加彼此独立的 `--enable-plan-delivery` 和 `--enable-approvals`，Wails native bridge 仍精确三个方法。React 增加 Models 对话框、Plan 选择/Deliver 按钮和 Approvals 页；所有 retry key 只驻留内存，客户端严格复核 Run/proposal/selection/approval/Session/Workspace 绑定与关闭权限位。OpenAPI 当前为 36 个 path、84 个 schema、27 个 GET 和 11 个 control POST。

最终普通功能门通过 310.1 秒全仓 Go、Windows Desktop tag、18 个文件 73 项前端测试、strict TypeScript、Vite production build、Windows production Desktop build 和零漏洞 npm audit。组合审计修复四项低风险问题：模型标识误含密钥时的投影、终态 Run 已提交审批的幂等重试、前端审批 Session/Workspace 身份复核，以及 Models 标题乱码。没有调用 Provider 网络、真实 Shell/Local/Docker 进程、文件写入、Session Grant、安装器或外部 Skill 执行；当前无已知未解决高/中风险，边界见 ADR 0039。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 64-68%、通用 Coding Agent 约 60%、Cyber 自动化约 20%。下一批建议依次完成 D1-M2 显式 Provider 诊断/路由设置、D1-D1 Diff 提案审阅和 D1-Q1 durable wake/retry ownership；该批结束将累计六片并执行 full race/vet/staticcheck/govulncheck/依赖/隐私完整健壮性门。

非 schema D1-M2/D1-D1 与 schema v74/D1-Q1 三切片批次已完成实现，任务 ID 为 `P9-Provider-Diff-Wake-Controls-v74`。D1-M2 增加显式 `provider_diagnostic.v1` 与 `model_route_control.v1`：路由先持久化再更新并发安全 Router；诊断只在操作者动作后发起一次 15 秒、无用户正文、禁用工具的请求，结果不含模型正文、密钥、端点、环境变量名或 raw error。D1-D1 增加精确 Run/Mission/Session/Workspace 的 metadata-only FileEdit 队列、脱敏有界 Diff 与 `approve_intent|deny`；批准意图不写文件，真实 apply 继续独立。

schema v74 增加 digest-idempotent wake schedule/cancel、有界 attempts/backoff/deadline 和 generation-fenced 单 owner。CLI/API/Desktop 只管理意图，公开状态不含 lease owner，后台 loop、模型、工具、execution lease 和自动 Run 转换全部为 false。OpenAPI 更新为 43 个 path、96 个 schema、30 个 GET 和 16 个 control POST。组合审计发现并修复审批两阶段崩溃恢复、Diff 精确读取仍加载正文、过期最终 wake lease 事件顺序、非法 wake 时间线和并发模型路由顺序。本批最终全仓 ordinary/race 为 278.6/296.1 秒；双路径 vet/staticcheck/govulncheck、module verify/tidy、20 文件 80 项前端测试、strict TypeScript、Vite/Windows production build、npm 零漏洞、契约、CLI smoke 与隐私/编码/链接/产物/进程入口扫描均为绿色。未使用真实 Provider key、网络 Provider、Shell、LocalRunner、Docker 或文件 apply，无已知未解决高/中风险。边界见 ADR 0040。

GitHub Actions run `29649564643` 已为实现提交 `37fbfbf` 全绿：TypeScript console 30 秒、Windows Desktop shell 1 分 58 秒、Go control plane 3 分 44 秒。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 67-71%、通用 Coding Agent 约 63%、Cyber 自动化约 20%。下一批建议依次完成 D1-Q2 显式前台 wake 消费、D1-D2 独立 Diff apply 和 D1-B1 inert Skill 安装确认；后台服务、真实宿主/容器进程、安装 hook 和 CTF 自动化继续后置。

schema v75/D1-Q2、schema v76/D1-D2 与非 schema D1-B1 三切片批次已完成实现，任务 ID 为 `P9-Foreground-Wake-File-Apply-Inert-Skill-Install-v76`。D1-Q2 新增 `run_wake_consumer.v1`：只有 CLI 命令或显式 control 请求才能领取一条到期 intent，并通过既有 `run_execution_handoff.v1`、RunSupervisor、Policy、累计预算、取消、checkpoint、模型/工具账本和 execution lease 消费最多八步。prepare、精确 handoff 绑定、结果与事件均持久化；`prepared` generation 不会因租约过期重领，结果未知时不能取消或伪造失败，失败事件的模型/工具事实只能来自持久化 handoff 结果，也不建立后台 goroutine、服务或隐藏轮询。

D1-D2 新增 `file_edit_apply.v1`：Go 精确重载 Run/Mission/Session/Workspace/Edit/Approval，每个 Edit 只允许一个 apply operation，并在最终写入边界重新检查运行 Run、活动 Session、当前 Policy、Workspace 路径和原始哈希；正文先写入同目录临时文件并同步，再经最终路径/哈希复核原子替换，随后验证目标 SHA-256 并持久化幂等结果。HTTP/React 不提交路径或正文，重放不会二次写入。Run-bound Edit 已禁止旧 `edit approve` 旁路。

D1-B1 在路径隔离预览后新增第四个窄 Wails 方法 `InstallSkillPackage` 和独立 HTTP control。Desktop 只提交短期单次确认句柄，HTTP 只接受严格有界 canonical base64；两者复用 schema v69 内容寻址惰性 Registry，并要求明确确认 `operator_installed_untrusted`。安装不执行正文、脚本、钩子、命令、工具、Provider 或网络，不自动选择到 Run，也不授权上下文交付或声明工具。三项 capability 均默认关闭且彼此独立。OpenAPI 更新为 46 个 path、102 个 schema、30 个 GET 和 19 个 control POST，边界见 ADR 0041。

最终普通整合门已通过：全仓 Go 333.1 秒、聚焦 race、Windows Desktop tag、20 个文件 85 项前端测试、strict TypeScript、确定性 OpenAPI/TypeScript、Vite/Windows production build、vet、module verify/tidy、npm 零漏洞、隔离 CLI smoke 和隐私/UTF-8/链接/产物/危险入口扫描均为绿色；未签名 GUI 为 23,107,584 字节，SHA-256 `309b0e556c44960d7739ab1159e61ede632a443a87ff0b006e6151a288a38626`。组合审计修复 prepared wake 过期重领/结算前取消、失败事件模型/工具事实失真、FileEdit prepared 恢复时权限过期、同 Edit 双 operation 与直接截断写入。当前无已知高/中风险；强杀发生在暂存后原子替换前可能留下一个已脱敏隐藏临时文件，作为已知低风险交由 D1-U1 recovery receipt 处理。本批是下一组六切片的前三片，不提前声称完整 race/staticcheck/govulncheck 门。双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 70-74%、通用 Coding Agent 约 66%、Cyber 自动化约 20%。

GitHub Actions run `29655417908` 已为实现提交 `79f07fb` 全绿：TypeScript console 28 秒、Windows Desktop shell 2 分 5 秒、Go control plane 3 分 57 秒；远端同时通过 API 漂移、前端测试/构建/依赖审计、Desktop 构建与边界测试、module verify、全仓 Go、vet 和 govulncheck。

下一批建议依次完成 D1-U1 mutation receipt/recovery 统一呈现、D1-E1 Go-owned 有界只读 Workspace explorer、D1-W1 可复现便携构建诊断与 Windows 兼容清单；届时累计六片，执行全仓 ordinary/race/vet/staticcheck/govulncheck/依赖/隐私完整健壮性门。后台服务、真实宿主/容器进程、签名分发、Rust analyzer 和 CTF 自动化继续后置。

非 schema D1-U1/D1-E1/D1-W1 三切片批次已完成，任务 ID 为 `P9-Receipts-Workspace-Explorer-Portable-Diagnostics-D1UEW1`，SQLite schema 保持 v76。D1-U1 新增严格 `operation_receipt.v1`，统一 FileEdit apply、前台 wake consume 和惰性 Skill install 的 outcome、replay、闭集 retry/recovery 与 cleanup 状态；不返回 operation key/digest、路径/正文、请求者、模型内容或私有 lease。FileEdit 终态/重放只会清理同一目标目录内、保留前缀、超过 15 分钟、普通文件且完整字节匹配批准 proposal SHA-256 的暂存文件；无法确认时保持 durable 结果并返回 `pending_review`。React 将回执与父响应交叉验证，失败 apply 使用告警而非成功呈现。

D1-E1 新增 read-bearer `workspace_explorer.v1` 与 Run Files 页。Go 只接受注册 Workspace 下规范 `/` 相对路径，拒绝遍历、绝对/卷路径、软链接/重定向、控制字符、首尾空白、需要规范化的别名和跨平台歧义名称；目录最多扫描 400、返回 200，文件最多读取 64 KiB 有效 UTF-8，脱敏投影最多 128 KiB。内部暂存和可疑名称不进入列表，root path 不进入 DTO。每份目录/文件投影以 `context_provenance.v1` 固定 `instruction_authorized=false`，TypeScript 还会核对条目必须等于当前父目录与条目名组合，并只以纯文本 `<pre>` 显示。

D1-W1 新增可复现 linker metadata、只读 `cyberagent doctor portable [--json]`、`build-desktop.ps1 -VerifyReproducible` 与 `check-windows-compat.ps1`。构建输出必须位于仓库内且不能穿过 child reparse point；连续两次构建必须 SHA-256 相同，并检查 PE 签名/架构/可执行标志、零 COFF timestamp、metadata/hash、`-trimpath`、Go module 及无 installer/registry/startup/update 边界。首次真实执行暴露 Windows PowerShell 5.1 不支持 `Path.GetRelativePath`，已改用经过前缀验证的兼容实现。自动检查全绿仍不等于正式发布，Windows 10/WebView2/display/launch/recovery 人工矩阵使 `release_ready=false`。

累计六切片完整健壮性门通过：最终全仓 ordinary 294.0 秒、race 338.3 秒；普通/secure-Desktop test 与 vet、零告警 staticcheck、零可达漏洞 govulncheck、module verify/tidy diff、22 个文件 88 项 React 测试、strict TypeScript、确定性 OpenAPI/TypeScript、Vite build、npm 零漏洞、隔离且显式清空真实 Provider 环境的 CLI smoke、凭据/本机路径/产物扫描与真实 Windows 双构建均为绿色。OpenAPI 为 47 个 path、106 个 schema、31 个 GET、19 个 control POST；双构建 SHA-256 为 `33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`，COFF timestamp 为 0。审计修复 release-ready 误报、失败回执成功图标、非规范/跨目录 Explorer 路径权限、OpenAPI 活路由缺 Workspace 夹具、脱敏投影上限、PowerShell 5.1 兼容、输出重解析点和一项既有 Go error 文案；当前无已知未解决高/中风险。未调用真实 Provider、Shell、LocalRunner、Docker、网络攻击、安装器、注册表、自启动或更新。

远端 GitHub Actions run `29658783000` 已通过实现提交 `5f0f397`：Go control plane 5 分 49 秒、TypeScript console 32 秒、Windows Desktop shell 2 分 11 秒。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 74-78%、通用 Coding Agent 约 70%、Cyber 自动化约 20%。下一批建议为 D1-E2 有界 Workspace 搜索、D1-C1 操作者显式把单份非授权 Workspace evidence 附加到既有 Session 队列，以及 D1-U2 可刷新 metadata-only operation receipt history。Windows 10 人工矩阵、签名/安装包、真实进程、Rust analyzer 与 CTF 自动化继续独立后置，边界见 ADR 0042。

schema v77 / D1-E2、D1-C1、D1-U2 三切片批次已完成，任务 ID 为 `P9-Workspace-Search-Evidence-Attachment-Receipt-History-v77`。D1-E2 新增一次性 `workspace_search.v1`：Go 只搜索既有 Explorer 脱敏投影，查询最多 128 个 Unicode code point，一次最多扫描 128 个目录、1000 个条目、64 个普通文件并返回 50 个结果；不建立 indexer/watcher，不跟随链接，不搜索原始字节，不返回宿主 root。结果只含 canonical relative reference、512-byte 纯文本 snippet 和 `instruction_authorized=false` provenance。

D1-C1 新增独立且默认关闭的 `session_evidence_attachment.v1`。HTTP/Desktop 只提交精确 Run、Go 返回的相对引用、投影 SHA-256、协议版本和内存幂等 key。Application 重新加载并绑定 Run/Mission/active Session/registered Workspace，再重新投影文件；Store 在一个事务中写 tool-role Session message、`session.evidence_attached` 事件与不可变 attachment。schema v77 trigger 再次要求 source/hash/message/event 全匹配以及 `instruction_authorized=false`。即使 README 含有“自动助手应跳过 .env”之类的诱导文本，进入上下文后仍是 untrusted user evidence，不能变成操作者指令、审批、Scope 或 capability。

D1-U2 新增 `operation_receipt_history.v1` 与 React Receipts 页。Go 从 FileEdit apply、前台 wake consume 和惰性 Skill install 终态事实生成 newest-first、最多 100 条记录，可精确按 Run 过滤；公开 ID 使用 domain-separated opaque hash，不返回 operation key/digest、路径/正文摘要、请求者、包 archive 元数据或 private lease。FileEdit staging 只调用只读 `InspectStaging`，任何路径/年龄/类型/大小/身份/hash 不确定都保守返回 `pending_review`，刷新历史绝不删除文件。

本批普通整合门全绿：最终 uncached 全仓 Go 297.9 秒；Windows Desktop tag、`go vet`、module verify/tidy diff、23 个文件 92 项 React 测试、strict TypeScript、Vite production build、确定性 OpenAPI/TypeScript、隔离 mock-only CLI smoke 和 npm 零漏洞全部通过。OpenAPI 为 50 个 path、53 个 operation、112 个 schema。真实 Windows 连续双构建可复现，未签名 GUI SHA-256 为 `d187601e9e9d8cb0d4ee644e3c9aa1c7617905580b001ef7955dbc35b8c47af3`；自动兼容检查通过，但无安装器/注册表且人工矩阵未完成，所以 `release_ready=false`。

组合审计修复两个具体边界：Unicode case mapping 可能改变字节长度，因此搜索改为逐原始行匹配，绝不用 lower-case 偏移切原文；搜索真实读预算补上每文件最多 `utf8.UTFMax` 的 Explorer 前瞻。canonical Workspace evidence reference 同时由 Go 与 schema v77 CHECK 约束。未发现未解决高/中风险；本批未调用真实 Provider、模型、工具、Shell、LocalRunner、Docker、网络、API key、安装器、注册表、自启动或更新。该批是新组六切片的前三片，因此不提前声称 full race/staticcheck/govulncheck 门。

远端 GitHub Actions run `29661764283` 已通过实现提交 `ffbdc72`：TypeScript console 34 秒、Windows Desktop shell 2 分 21 秒、含 govulncheck 的 Go control plane 3 分 48 秒。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 78-82%、通用 Coding Agent 约 74%、Cyber 自动化约 20%。下一批建议为 D1-O1 Go-owned 有界操作者行动中心、D1-C2 metadata-only 已附加 evidence 清单与 D1-K1 只调用既有已启用能力的键盘命令面板；累计六片后执行 ordinary/race/vet/staticcheck/govulncheck/依赖/隐私完整健壮性门。Windows 10 人工矩阵、签名/安装包、Provider secret UI、真实进程、Rust analyzer 与 CTF 自动化继续独立后置，边界见 ADR 0043。

非 schema D1-O1、D1-C2、D1-K1 三切片批次已完成，任务 ID 为 `P9-Operator-Actions-Evidence-Inventory-Command-Palette-D1OCK1`，SQLite schema 保持 v77。D1-O1 新增 `operator_action_center.v1`：Go 对精确 Run/Mission/Session/Workspace 聚合 pending steering、approval、FileEdit review/apply readiness 与 due wake，最多返回 100 条闭集 metadata。公开 ID 使用域分离 opaque hash，不返回 source row、operation、requester、message、command、path、Diff、private lease 或 authority；列表不做审批、apply、wake、drain 或执行。

D1-C2 新增 `session_evidence_inventory.v1`：只列 exact Run-bound active Session/Workspace 已持久化附件的 source kind/reference、SHA-256、时间与固定 `instruction_authorized=false`；message ID/body、AttachedBy、event sequence、private operation 和 capability 均留在 Go/Store。React 只能把 Go 发放的 canonical 相对引用交回既有 Files Explorer，Explorer 仍独立复核路径。D1-K1 新增静态闭集 `Ctrl+K` 命令面板，只导航现有 Run 页签或刷新当前 Run 查询，不提交 path/content/approval/operation/capability/process/network/secret，也不写浏览器存储。

累计六切片完整健壮性门全绿：最终 uncached 全仓 ordinary/race 319.6/299.8 秒；普通/secure-Desktop test 与 vet、零告警 staticcheck、零可达漏洞 govulncheck、module verify/tidy、26 个文件 97 项 React 测试、strict TypeScript、确定性 OpenAPI/TypeScript、Vite production build、npm 零漏洞、隔离 mock-only CLI smoke、凭据/本机路径/产物/UTF-8/Markdown 扫描和真实 Windows 可复现双构建全部通过。OpenAPI 为 51 个 path、55 个 operation、116 个 schema，SHA-256 `B9CD79254D9AE09A2DB4BCC6268F04CA8F4ADD6C638E6BAA4DA42FC223A10181`；未签名 GUI SHA-256 `a89b2357a5f1e7376ea8a533356028ccd5ea5eaec388b14d7623343fd041f520`，自动检查通过但 `release_ready=false`。

真实浏览器在 1440x900 与 390x844 复核 Actions、Evidence、`Ctrl+K`、本地 tab scroll、overflow、source navigation 和 SSE。审计发现前端错误要求 `event.v1` 而 Go canonical envelope 是 `v1`，且失败重连只 release lock 没有 cancel response body，可能占满六条同源连接；现由 OpenAPI 导入 Go 常量生成 TypeScript literal `v1`，任何 parse/transport failure 先 cancel reader 再重连。当前无已知未解决高/中风险；未使用真实 Provider/API key、Shell、LocalRunner、Docker、外部网络、安装器、注册表、自启动或更新，边界见 ADR 0044。

远端 GitHub Actions run `29665187925` 已通过实现提交 `1151aaf`：TypeScript console 36 秒、Windows Desktop shell 2 分 23 秒、Go control plane 3 分 35 秒。

双指标更新为架构完成度约 98%（V2 约 99%）、完整产品可用度约 80-84%、通用 Coding Agent 约 77%、Cyber 自动化约 20%。下一批候选为 D1-I1 Go-issued Monaco proposal/Diff editor、D1-M3 Go/OS-owned Provider secret boundary 与 D1-J1 默认关闭的 bounded wake worker；三项均先做独立威胁模型，renderer 不直接写文件/读取明文密钥，worker 不取得 Shell/Local/Docker 权限。Windows 10 人工矩阵、签名/安装包、Rust analyzer 与 CTF 自动化继续独立后置。

非 schema D1-I1/D1-M3/D1-J1 三切片批次已完成，任务 ID 为 `P9-Editor-System-Credentials-Bounded-Wake-D1IMJ1`，SQLite 保持 v77。D1-I1 增加 `file_edit_proposal.v1`：Go 为 exact running Run/active Session/registered Workspace 发放 256-bit、五分钟、单意图 source handle；本地 lazy Monaco/Diff 只接收完整未截断的安全 UTF-8 和 handle，提交只含新文本。Go 重检绑定、当前 hash、secret 和 Policy 后只创建 pending proposal，review/apply 仍独立且 proposal 不写文件。

D1-M3 增加 `provider_credential.v1` 与 Windows Credential Manager system store。只支持 exact `mimo|deepseek|anthropic`，generic credential 上限 2,560 bytes；TypeScript 只能设置/删除并读取 configured/store/restart 状态，固定 `plaintext_returned=false`。明文不进入 SQLite、事件、日志、模型上下文或浏览器存储；非 Windows 无明文 fallback，环境变量优先，修改后当前要求重启。D1-J1 增加 default-off `run_wake_worker.v1`：只有 startup flag + distinct control token 才启动一个 process-lifetime serial owner，每轮最多一条 due intent 和 `max_steps=1`，复用 Foreground Wake Consumer/RunSupervisor/预算/Policy/lease/checkpoint/cancel，不依赖 Tool Runner、Shell、LocalRunner 或 Docker。

最终普通整合门通过：uncached 全仓 Go 327.6 秒、`go vet`、secure Desktop tag、28 文件 102 项 React、strict TypeScript、确定性 OpenAPI/TypeScript、Vite production build、npm 零漏洞和 Windows 可复现双构建均为绿色；未签名 GUI SHA-256 为 `a0e6aa0a3d15ccc39712f8a0a64d7de06e4a6af426e060b6378b1011c93a1cf6`，`release_ready=false`。审计修复 exact provider normalization、Windows credential 长度、单凭证读取失败导致全局不可用、ControlPlane 关闭后 worker 重启、不确定 FileEdit 保存后的 ID/意图漂移、审批后重试误报内部错误、未配置 Provider 的 `models:null` 契约回归、前端密钥测试快照，以及 Monaco CDN/依赖漏洞风险；桌面和 390x844 移动 UI 冒烟通过。当前无已知未解决高/中风险，没有使用真实 API key、Provider 网络、Shell、LocalRunner 或 Docker，边界见 ADR 0045。

GitHub Actions run `29671519260` 已通过实现提交 `ee36405`：TypeScript 42 秒、Windows Desktop 2 分 31 秒、Go control plane 3 分 54 秒。

双指标更新为架构完成度约 99%（V2 约 99%）、完整产品可用度约 84-87%、通用 Coding Agent 约 82%、Cyber 自动化约 20%。下一批固定为 D1-I2 安全编辑恢复、D1-M4 Provider Registry generation reload、D1-J2 metadata-only browser capability + worker health/drain；它会补齐普通 `api serve` 保守隐藏新控件的低风险 discovery 缺口。该批结束累计六片，执行全仓 ordinary/race/vet/staticcheck/govulncheck/依赖/隐私完整门。Windows 10 人工矩阵、签名/安装包、Rust analyzer、xterm 与 CTF 自动化继续独立后置。

非 schema D1-I2/D1-M4/D1-J2 三切片批次已完成，任务 ID 为
`P9-Safe-Editor-Recovery-Provider-Generation-Worker-Health-D1IMJ2`，SQLite schema
保持 v77。D1-I2 增加 hash-bound source handle 换发和
`file_edit_proposal_recovery.v1`：旧 SHA-256 不匹配时拒绝换发，持久化 pending
proposal 只恢复为无句柄、不可编辑、需要独立 review 的 Diff，stale/missing 不会
自动 rebase。D1-M4 增加完整候选 Provider Registry generation 构建和原子切换；
route 或 credential 读取失败保留旧 generation，活跃调用继续使用已捕获 Provider，
成功无需重启且任何响应不返回明文。D1-J2 增加只读 `runtime_capabilities.v1` 与
`run_wake_worker_health.v1`；普通浏览器/Desktop 共用精确 Go capability，worker
固定 1 x 1，只显示 `disabled|ready|running|draining|stopped`，不能运行时启用或安装服务。

累计六切片完整健壮性门全绿：最终 uncached 全仓 ordinary/race 322.9/352.8 秒；
`go vet`、零告警 staticcheck、零可达漏洞 govulncheck、module verify/tidy、secure
Desktop tags、29 个文件 108 项 React 测试、strict TypeScript、确定性
OpenAPI/TypeScript、Vite production build、npm 零漏洞和 Windows 可复现双构建均通过。
OpenAPI 为 57 个 path、61 个 operation、125 个 schema；未签名 GUI SHA-256 为
`30a3d9d19e02f32f8ea976fc071bc6942ed06fba3e7cad937310a78e46e74dfc`，自动检查通过
但 `release_ready=false`。真实浏览器桌面/移动冒烟无横向溢出或 console 错误。

组合审计修复 route 与 Provider 混合 generation、候选凭证读取失败误替换当前 Registry、
并发 reload 误报与列表 generation 不一致、worker 无 control token 构造、并发
`RunOnce`、nil context、missing-file proposal 恢复和恢复对话框失败态。当前无已知未
解决高/中风险；未使用真实 API key、Provider 请求、Shell、LocalRunner、Docker、
攻击流量或外部网络。双指标更新为架构完成度约 99%（V2 约 99%）、完整产品可用度
约 87-90%、通用 Coding Agent 约 84%、Cyber 自动化约 20%，边界见 ADR 0046。

远端 GitHub Actions run `29674460349` 已通过实现提交 `7d5736e`：TypeScript console
38 秒、Windows Desktop shell 2 分 49 秒、Go control plane 3 分 43 秒。

非 schema D1-G1/D1-I3/D1-F1 三切片批次已完成，任务 ID 为
`P9-Repository-ChangeSet-Code-Journey-D1GIF`，SQLite 保持 v77。D1-G1 增加纯 Go
`repository_state.v1`：只检查 exact registered Workspace root，拒绝父仓库、重定向
`.git` 与内部 metadata link，50,000/10,000/200 三层上限，不启动进程、网络、remote
或 hook，也不返回 root/body。D1-I3 增加最多 100 项的 metadata-only
`file_edit_change_set.v1`，明确 per-file review/apply、no batch/atomic mutation 和
partial visible。D1-F1 增加 Code-only 五阶段 Journey，只导航既有 Go 能力，无 API
client 或复合 mutation，Cyber surface 不自动继承。

累计六切片完整门全绿：ordinary 321.7 秒、final-code full race 490.4 秒、repository
10 轮、vet、零告警 staticcheck、module verify、0 reachable/imported-package
govulncheck、secure Desktop、31 文件 114 项 React、strict TypeScript、确定性
OpenAPI/TypeScript、Vite、npm 零漏洞、mock-only CLI 和 Windows 可复现双构建均通过。
OpenAPI 为 59 path/63 operation/129 schema；未签名 GUI SHA-256 为
`145757cb1a8bbafc9080fdc29f4ada69d34b850ca64f702310ea44578ca677a9`，仍为
`release_ready=false`。模块图保留应用未调用且无修复版的 `GO-2026-5932` openpgp
残余。审计补上 nested Git metadata link 拒绝，并修复终态 proposed edit 的零 action
严格解析。桌面/390x844 无页面溢出或 console 错误；无已知高/中风险，未使用真实
key、Provider、Shell、LocalRunner、Docker、hook、攻击流量或外部网络。双指标更新为
架构约 99%（V2 约 99%）、完整产品可用度约 89-92%、通用 Coding Agent 约 88%、
Cyber 自动化约 20%，边界见 ADR 0047。

远端 GitHub Actions run `29678257802` 已通过实现提交 `d69a812`：TypeScript 43 秒、
Go 5 分 32 秒、Windows Desktop 5 分 29 秒。

下一批建议为 D1-G2 有界脱敏 repository Diff、D1-V1 操作者 verification evidence
与 D1-F2 Go-owned resumable Code handoff summary。Windows 10 人工矩阵、签名/安装包、
真实进程、Rust analyzer、xterm 与 CTF 自动化继续独立后置。

schema v78 / D1-G2、D1-V1、D1-F2 三切片批次已完成，任务 ID 为
`P9-Repository-Diff-Verification-Code-Handoff-v78`。D1-G2 增加纯 Go
`repository_diff.v1`：只读取 exact registered Workspace root，最多 50 项、单项
64 KiB patch、总计 512 KiB；HEAD 与工作区正文先脱敏，binary/oversized/linked/
unavailable 使用闭集状态。该路径不启动 `git` 或其他进程，不访问网络、remote/hook，
不返回宿主 root 或原始未脱敏正文，所有 instruction/mutation/authority 位保持 false。

D1-V1 通过独立 capability 增加不可变 `operator_verification_evidence.v1`。操作者只提交
闭集 `pass|fail|unknown`、规范化 title/summary 和内存幂等 key；Go 脱敏并精确绑定
Run/Mission/active Session/registered Workspace。Store 在写事务内再次确认 Session active，
同一事务提交 metadata-only `verification.evidence_recorded` 与 evidence；schema v78 trigger
再次要求事件绑定并把 command/model assertion/approval/authority 固定为 false。inventory
最多 100 项；v77 原地升级不伪造 evidence、operation 或事件。

D1-F2 增加 Code-only `code_handoff.v1`，从持久化 Plan/WorkItem、steering queue、FileEdit
change set、verification、operator actions 与 Finding report references 生成可重建有界摘要。
它不复制消息、验证摘要、Diff、文件/报告正文、operation/requester/lease，也不创建
resume、apply 或复合 mutation。Go 在组装前后读取 Run event high-water，最多四次重试；
持续变化时返回 conflict，绝不返回撕裂快照。

最终 ordinary 整合门全绿：uncached 全仓 Go 308.1 秒（Store 299.8 秒）、Desktop tag、
`go vet`、定向零告警 staticcheck、module verify/tidy、35 文件 120 项 React、strict
TypeScript、确定性 OpenAPI/TypeScript、Vite production build、npm 零漏洞、隐私/UTF-8/
产物/新切片进程网络入口扫描和 Windows 可复现双构建均通过。OpenAPI 为 62 path、67
operation、143 schema，SHA-256
`652707A6D9CA72EBBD86B6FD407A382DFBE85B094927C82AFC3765D2648332B3`；未签名 GUI
SHA-256 `2ab74a47794287bac71877172136f02631b5cc9a44febd930e8ee7b1913ba93f`，自动兼容检查
通过但 `release_ready=false`。

production bundle 浏览器测试在隔离 schema-v78 home 中录入一条 pass evidence，Handoff
随即显示 `1 pass`，非 Git Workspace 的 Repository/Diff 空状态正确；桌面/390px 移动
宽度无页面级横向溢出，console 零错误。组合审计修复两项中风险一致性问题：Session
可能在 Application 预检后、Store 写入前被归档，以及 Handoff 多来源查询可能跨事件变化
产生撕裂；另修复顶层 Diff truncation 少报、TS action/reference/truncation 严格校验、CR
规范化差异和 Verify capability 从 connection 到 API client 的漏传。当前无已知未解决
高/中风险；未使用真实 key、Provider、Shell、LocalRunner、Docker、hook、攻击流量、
安装器、注册表、自启动或外部网络，边界见 ADR 0048。

双指标更新为架构完成度约 99%（V2 约 99%）、完整产品可用度约 92-94%、通用 Coding
Agent 约 92%、Cyber 自动化约 20%。本批是新组六切片的前三片；下一批建议 D1-G3
有界 recent commit/branch history、D1-V2 独立 verification plan/checklist 与 D1-F3
带 high-water/digest 的 Markdown/JSON handoff export。下一批结束执行 ordinary/race/vet/
staticcheck/govulncheck/依赖/隐私/构建完整健壮性门。

远端 GitHub Actions run `29682547524` 已通过实现提交 `cff7489`：TypeScript console
42 秒、Windows Desktop shell 2 分 34 秒、含 vet 与 govulncheck 的 Go control plane
3 分 33 秒。

H1/H2/H3 防死锁与活锁三切片已经完成，任务 ID 为
`P2-Runtime-Deadlock-Livelock-Guards-v79`。H1 为每次启用的 in-process Tool 调用增加
默认 15 秒、最多 5 分钟硬截止时间，区分 124 timeout 与 130 cancellation，并恢复 panic。
内置 read/list 在循环中检查 context，`max_bytes` 不得超过 FS 配额或平台整数边界；Unix
以 nonblocking/CLOEXEC/NOFOLLOW 打开并 fstat，其他平台比较打开前后身份，只允许普通
文件，FIFO/device/socket 不再可能把 Tool 调用永久挂住。

H2 新增进程级 `waitgraph`：Agent、Tool、Retriever、Store、Runner、Model 和 External
节点总计最多 4,096，边最多 8,192；边引用计数、幂等释放，插入前拒绝 self/direct/
indirect cycle。Tool/Retriever/Store/Runner 永久不能反向同步等待 Agent。root Supervisor
注入 Agent identity，Specialist scheduler 包围 parent -> child，Tool Gateway 包围当前
调用并传播唯一 Tool identity。未来 RAG、Store callback、Model adapter 和 Runner 也必须
复用该协议，不能另建互相回调链。

H3/schema v79 新增 `run_progress_guard.v1`。它只保存结构化状态/动作 SHA-256、饱和计数、
阈值、原因和时间，不保存模型正文。连续三次相同 `continue` 检测
`repeated_action`，连续六轮结构化状态不变检测 `no_observable_progress`；检测事件、
Session 消息、checkpoint 与 `running -> paused` 在同一事务提交，原始 `continue` 重放
不会重复写消息。只有检测后真实的 `paused -> running` 事件才能把 guard 恢复为 observing；
v78 原地升级只建表和 trigger，不伪造进展。

本批与前一 v78 批累计六切片，完整健壮性门已通过：最终 uncached 全仓 Go 312 秒
（Store 304.6 秒）、final-code 全仓 race 358 秒、Tool/等待图重复 20 轮、v79 Store 重复
10 轮、普通/secure Desktop test 与 vet、零告警 staticcheck、module verify/tidy、零可达
govulncheck、35 文件 120 项 React、strict TypeScript、确定性 API、Vite production、npm
零漏洞、隔离 mock-only CLI、凭据/产物扫描、Windows 可复现构建和 production bundle
桌面/390x844 浏览器检查。GUI SHA-256 为
`31e0df63d3fbbccac6728ad2322196bee55d57e775a15cc34f752c0632bdc699`，仍为
`release_ready=false`；浏览器无横向溢出或 console error。

组合审计补强 `max_bytes` 整数/OOM 边界、Go/SQL observing 阈值、检测后显式恢复证明、
损坏 guard 读取失败关闭和计数饱和。依赖图仅保留未导入/未调用的 module-only
`GO-2026-5932`。当前启用路径无已知未解决高/中风险；未使用真实 key、Provider、Shell、
LocalRunner、Docker、攻击流量或外部网络。硬超时无法强杀完全忽略 context 的第三方 Go
goroutine；真实进程的句柄/端口/进程树死锁仍属于后续 Runner start/wait/TERM/KILL/
orphan 独立门禁，因为真实执行继续关闭。双指标保持架构约 99%（V2 约 99%）、完整产品
可用度约 92-94%、通用 Coding Agent 约 92%、Cyber 自动化约 20%，边界见 ADR 0049。

远端 GitHub Actions run `29688544340` 已通过实现提交 `2012bfa`：TypeScript console
42 秒、Windows Desktop shell 3 分 13 秒、含 vet 与 govulncheck 的 Go control plane
3 分 54 秒。

D1-G3/D1-V2/D1-F3 三切片产品批次已经完成，schema 升至 v80。纯 Go
`repository_history.v1` 只读取 exact registered Workspace root，最多 50 个 first-parent
commit、64 个 local branch 与 1,024 次 reference scan；主题先规范化/限长/密钥脱敏，
author/email/body/remote/root/process/network/hook 全部不进入协议，重定向或链接 Git
metadata 失败关闭，恶意 parent/omission 计数在 Go 侧饱和。

不可变 `operator_verification_plan.v1` 支持 1-32 项操作者检查清单，事务内复核 active Code
Session/Workspace，并由 schema v80 trigger 绑定 metadata event、item count 和 plan digest。
`guidance_only=true`，command/model assertion/result inference/approval/authority 全部固定
false；计划与 v78 `pass|fail|unknown` 结果分表、分协议，不会把模型或文档要求当成通过。
`code_handoff_export.v1` 将稳定 Handoff 输出为最多 256 KiB Markdown/JSON，携带 source
event high-water、UTF-8 byte count 和 SHA-256；TypeScript 下载前重新验证摘要、格式和
Run/source binding，不能 resume、accept report、apply、mutate 或 execute。

本批 ordinary 集成门通过：final functional uncached 全仓 Go 334.6 秒，审计后 Repository/
Application/Store/HTTP 聚焦回归与 `go vet` 再通过；37 文件 124 项 React、strict
TypeScript、确定性 OpenAPI/TypeScript 与 Vite production build 为绿色。OpenAPI 为
65 path/71 operation/155 schema，SHA-256
`99887F651B563C56C87D19C5624EDD776AFC29AA6095EAB8C685E6767C165E7F`。Chrome 插件对
最终 Go-hosted bundle 复验 Repository/Verify/Handoff，无 root/email 泄漏、无结果推导、
无页面横向溢出、console 零 warning/error。

组合审计修复恰好 50 份计划时的 strict-client truncation 误判、恶意 Git 计数越界、
失败计划修改后误复用旧幂等 key 和 download object URL 释放过早。当前无已知未解决
高/中风险，未使用真实 key、Provider、
Shell、LocalRunner、Docker、hook、攻击流量或外部网络。双指标更新为架构约 99%（V2
约 99%）、完整产品可用度约 93-95%、通用 Coding Agent 约 93%、Cyber 自动化约 20%，
边界见 ADR 0050。

远端 GitHub Actions run `29695882120` 已通过实现提交 `d70d96c`：TypeScript console
43 秒、Windows Desktop shell 2 分 39 秒、含 vet 与 govulncheck 的 Go control plane
3 分 56 秒。

下一批建议 D1-G4 exact-commit changed-file metadata、D1-V3 plan-item/evidence 显式关联
和 R1 Go-owned Runner start/wait/cancel/timeout/process-tree-orphan 非产品 harness。它们是
当前周期后半三片，批末执行全仓 ordinary/race/vet/staticcheck/govulncheck/依赖/隐私/
构建/浏览器完整健壮性门；Local/Docker 产品执行、xterm 输入、Rust analyzer 和 CTF 自动化
继续独立后置。

D1-G4/D1-V3/R1 三切片已经完成，schema 升至 v81，任务 ID 为
`P9-Exact-Commit-Verification-Association-Runner-Lifecycle-v81`。D1-G4 增加纯 Go
`repository_commit_detail.v1`：只接受 exact registered Workspace root 内一个小写 40 位
SHA-1 object，将 commit tree 与 first parent 比较，最多输出 200 条 canonical path 的
added/modified/deleted、content/mode-change metadata。author/email/body/blob/remote/root、
checkout/ref mutation、process/network/hook 均不进入协议；redirected metadata、link、畸形
tree 与缺失 object 失败关闭。

D1-V3 增加不可变 `operator_verification_plan_evidence_association.v1` 与 bounded coverage。
一条 later evidence 只能显式关联一个 earlier plan item，一个 item 可保留多条甚至互相矛盾的
观察。Go、事务 Store 与 schema v81 trigger 重复复核 exact Code Run、active Session、
Workspace、plan/item/evidence、operation/event/digest；command/model/result inference/
approval/authority 全为 false。UI 只显示每项 pass/fail/unknown 和 unobserved，不生成整体 pass。

R1 增加 simulation-only `runner_lifecycle_contract.v1`，覆盖 start/wait、pre-cancel、timeout、
共享 waitgraph、TERM/KILL grace、最终 inspect/reap、partial/invalid start cleanup 与 orphan
cleanup。当前没有 CLI、HTTP、Desktop、Agent、LocalRunner、Docker、`os/exec` 或任何产品
capability 接线，因此不开放真实宿主机/容器执行。

本批与前一 v80 批累计六切片，完整健壮性门全绿：最终 uncached 全仓 Go 509 秒、全仓 race
341 秒、ordinary/secure-Desktop test/vet、零告警 staticcheck、module verify/tidy、零可达
govulncheck、37 文件 127 项 React、strict TypeScript、确定性 OpenAPI/TypeScript、Vite、
npm 零漏洞、隔离 mock-only CLI、隐私/产物检查、Windows 可复现构建和 Chrome 桌面/390x844
复核。OpenAPI 为 68 path/74 operation/163 schema，SHA-256
`CFAD160A85306B2602F95A62298828DB86BDFAAF6D55F47BA468860079C42E8D`；TypeScript schema
SHA-256 `CCA5EF8B86E7F0D494E7B2BAF4FCA92FBE3FCB9C3A54E58D4A3C3B77028D5B73`；未签名 GUI
SHA-256 `77fb4d6fede1c1e3a0c3f3e9d39581e28f7a6880e0e25b222dcf0d3c701d1213`，仍为
`release_ready=false`。

真实 production bundle 录入 plan、pass evidence 与显式 association，刷新后恢复为
`1/1 observed` 和 `1 linked`；桌面/移动无页面级横向溢出，console 零 warning/error。
组合审计替换会静默漏掉 missing subtree object 的 Git walker，修复 v81 downgrade fixture
trigger 清理顺序、脱敏计数饱和、OpenAPI control whitelist，并补齐 partial/invalid Runner
start 清理。当前启用路径无已知未解决高/中风险；依赖图仅保留应用未导入/未调用的
transitive `GO-2026-5932`。未使用真实 key、Provider、Shell、LocalRunner、Docker、hook、
攻击流量或外部网络。双指标更新为架构约 99%（V2 约 99%）、完整产品可用度约 94-96%、
通用 Coding Agent 约 94%、Cyber 自动化约 20%，边界见 ADR 0051。

下一批建议 D1-G5 有界脱敏 exact-commit file preview、D1-V4 把显式 verification coverage
以 metadata-only 方式加入 Handoff/export，以及 R2 仅测试构建可见的 platform process-tree
conformance adapters。三者仍不得开放 raw blob、aggregate pass inference 或产品 Local/Docker
start；人工 Windows 10、签名/安装包、Rust analyzer、xterm 与 CTF 自动化继续独立后置。

本轮插入完成 C1/C2/C3 三个通用上下文硬化切片。C1 将非 ASCII 内容改为按 UTF-8 字节
保守估算 token，并使用饱和加法；C2 新增完整请求级 `model_context_window.v1`，默认总窗口
32,768、余量 1,024、默认输出 1,024、输出上限 4,096，root/Specialist 只可裁剪最旧普通
history，mandatory context 超限时在 Provider 前失败；C3/schema v82 将一次性摘要升级为
`handoff_memory.v1` 累计链，默认超过 8 条消息压缩并保留 4 条，交接最多 4,000 字符/
12 条记录，绑定 predecessor、SHA-256、累计计数、单调 ordinal 与 Session 消息 ID 高水位。旧 v0 摘要可读，并在下次
压缩时以无指令权限证据折叠。

组合审计确认并修复一个原有显著可靠性问题：连续压缩只读取最新摘要但未并入更早摘要，
导致早期决定消失；同时将交接记录独立固定为 12 条、修复零值 Router map 初始化及 v82
追加后的 v81 降级夹具顺序、来源引用脱敏/限长、时钟回拨与摘要落库后崩溃重试。README/仓库/工具/模型正文仍是非可信证据，不自动重写或重载
`AGENTS.md`。uncached 全仓 Go 348.5 秒、changed-package `go vet`、strict TypeScript 与
37 文件 127 项 Vitest 全绿；当前无已知未解决高/中风险，边界见 ADR 0052。未使用真实
Provider/key/Shell/LocalRunner/Docker/hook/攻击流量/外部网络。本轮是新六切片周期前三片，
下一批 D1-G5/D1-V4/R2 后执行完整 race/staticcheck/govulncheck/依赖/隐私/构建门。

## D1-G5/V4/R2 批次：精确提交预览、交接覆盖率与进程树一致性

任务 ID：`P9-Commit-Preview-Handoff-Coverage-Process-Conformance-v82`。本轮不新增
SQLite migration，schema 保持 v82。

D1-G5 完成 `repository_commit_file_preview.v1`：精确绑定注册 Workspace root、40 位
小写 commit object 和 canonical path；只接受 regular/executable UTF-8，原文最多 64 KiB，
脱敏投影最多 128 KiB，并携带投影 SHA-256 与 `instruction_authorized=false`。raw blob、
root、remote、checkout/ref mutation、process、network 和 hook 均保持关闭。

D1-V4 将 `operator_verification_plan_coverage.v1` 加入 Code Handoff 与 Markdown/JSON
导出。最多 100 个 flat item 只公开 plan/item digest、显式 pass/fail/unknown 计数和最新关联
事件序号；private guidance/evidence body 不进入交接，矛盾观察保留，整体 pass/fail/completed
不推断。Go 与 TypeScript 分别复核绑定、总数、重复项、摘要、序号和权限位。

R2 把 Runner backend 标记改名为 `NonProductOnly`，并仅在 `_test.go` 增加 Windows
Job Object 与 Unix process group 适配器。三项真实 OS 用例验证协作终止、TERM 无响应后的
强杀升级，以及 parent 先退出后的 orphan child 清理。测试只启动当前 Go test binary；
CLI/HTTP/Desktop/Agent/Sandbox/Local/Docker 没有接线，也没有新增执行 capability。

本批与 C1/C2/C3 累计六切片，完整健壮性门通过：uncached ordinary Go 380 秒，full race
411.2 秒；审计后受影响包 ordinary/race、ordinary/secure-Desktop test/vet、零告警
staticcheck、module verify/tidy、零可达 govulncheck、37 文件 127 项 Web 测试、strict
TypeScript、确定性 OpenAPI/TypeScript、Vite、npm 零漏洞、隔离 mock-only CLI、隐私/产物/
产品进程入口扫描、Linux runner test binary 交叉编译与 Windows 可复现双构建均通过。
OpenAPI 为 69/75/167；最终 GUI SHA-256 为
`44d54bf9d50b7cd99b89f5089833823ce0337bb0e0158ec16ef6aa9a5b415614`，并保持
`release_ready=false`、无 installer、无 registry write。

组合审计修复 platform-width 计数相加、negative/empty aggregate event fact 与窄 Store
边界 duplicate plan/count 接受。当前启用路径无已知未解决高/中风险；用户测试 Key 未进入
仓库，未调用真实 Provider/Shell/LocalRunner/Docker/hook/攻击流量/外部网络。双指标更新为
架构约 99%，完整产品可用度约 95-97%；通用 Coding Agent 约 95%，Cyber 自动化约 20%。
边界见 ADR 0053。

下一批建议 D1-G6 bounded exact-file history、D1-V5 read-only verification coverage
drill-down、R3 bounded output/exit-evidence contract。它们仍不得开放 raw Git content、结果推断
或产品 process start；Windows 10 人工矩阵、签名安装、Rust analyzer、xterm 与 CTF 继续后置。

## D1-G6/V5/R3 批次：精确文件历史、验证下钻与退出证据

任务 ID：`P9-File-History-Verification-Drilldown-Runner-Exit-Evidence-v82`。本轮不新增
SQLite migration，schema 保持 v82。

D1-G6 完成纯 Go `repository_file_history.v1`：对一个已注册 Workspace root 和 canonical
relative path，从当前 HEAD 沿 first-parent 最多扫描 512 个 commit，返回最多 50 条真实变化。
响应只含 object/time、已脱敏有界 subject、added/modified/deleted、前后 mode 与 content/mode
change；不读 raw blob/patch/author/body/remote/root，不推断 rename，也不 checkout、改 ref、启动
进程、访问网络或 hook。React 的 changed-path 列表可打开精确文件历史，删除项同样可查看。

D1-V5 完成 `operator_verification_plan_item_coverage.v1`：对精确 Run + plan + ordinal 返回
item digest/count 和最多 100 条 association metadata。仅显示显式 pass/fail/unknown、opaque
evidence ID、事件序号与时间；plan/evidence body、操作者身份、整体结论、命令/模型/审批/权限
全部缺席。Store/Application/HTTP/TypeScript 共同复核绑定、摘要、总数、严格降序、唯一性和
截断计数。

R3 在内部 `NonProductOnly` lifecycle 增加 `runner_exit_evidence.v1`。进程树确认 reaped 后，
测试边界才可记录 exit code，以及 stdout/stderr 各自最多 64 MiB observed bytes、64 KiB captured
prefix byte count/SHA-256 和 truncated；raw output 固定不返回。产品 CLI/HTTP/Desktop/Agent/
Sandbox/Local/Docker 没有 starter 或接线。

普通三切片门通过：uncached 全仓 Go 373.3 秒、受影响包 race、`go vet`、受影响包
staticcheck、module verify/tidy、37 文件 127 项 Web 测试、strict TypeScript、确定性
OpenAPI/TypeScript、Vite、npm 零漏洞、Desktop tag、Linux runner-test 交叉编译和 Windows
可复现双构建。OpenAPI 为 71/77/170，SHA-256
`C78A701600F8535A9C2398C12B3AAA7A695A93AD58913010D8904ADEED121625`；TypeScript schema
SHA-256 `977B8EEE7E9A268040453E0ADFB6FFB4C58489D4B90B94177473DC4B882E4740`；最终未签名 GUI
SHA-256 `c96047d7f3ea0afbe3b2f54f1c4ded197a861b29d644cb2edb449c8b3e46b031`，保持
`release_ready=false`。

组合审计修复 Git first-parent 顺序被错误要求 commit clock 单调下降，以及验证 count row 和
截断 association 的重复/不一致校验缺口；当前启用路径无已知未解决高/中权限风险。用户测试
Key 未进入仓库，未调用真实 Provider/Shell/LocalRunner/Docker/hook/攻击流量/外部网络。
双指标保持架构约 99%、完整产品可用度约 95-97%；通用 Coding Agent 约 95%，Cyber 自动化
约 20%。边界见 ADR 0054。

下一批建议 D1-G7 history-to-exact-commit 导航、D1-V6 精确检查项 evidence metadata 的
opaque 有界分页、R4 非产品 stdin/descriptor/resource evidence。该批完成后累计六片，执行
full race/vet/staticcheck/govulncheck/依赖/隐私/构建健壮性门；真实进程、Windows 10 人工
矩阵、签名安装、Rust analyzer、xterm 与 CTF 继续后置。

## D1-G7/V6/R4 批次：历史导航、验证分页与运行元数据

任务 ID：`P9-History-Navigation-Verification-Pagination-Runner-Runtime-Evidence-v82`。
本轮不新增 SQLite migration，schema 保持 v82。

D1-G7 让 `repository_file_history.v1` 的每一条历史记录复用现有 exact-commit detail；仅
regular/executable 条目可进一步复用现有脱敏 preview，deleted/symlink/submodule 不显示预览。
React 只使用 Go 已投影的 Workspace/object/path，不新增 raw blob/patch、checkout/ref mutation、
进程、网络、remote 或 hook。

D1-V6 为 exact Run + plan + ordinal 的 evidence association 增加共享严格 `limit` 与 route-
scoped opaque `cursor`。SQLite 按不可变 event sequence/ID 倒序并多取一行，Go 逐页复核绑定、
摘要、聚合计数、返回数、严格顺序、唯一 association/evidence ID 和显式 outcome；起始 offset
上限为 100,000。React 每次显式加载 25 条旧记录，并对跨页 aggregate、latest event、ID 与
顺序做一致性检查；live projection 变化会要求刷新，不会静默拼接。private body、操作者身份、
aggregate verdict、mutation、command/model、approval 和 authority 继续缺席。

R4 新增内部 `runner_runtime_evidence.v1`。整棵进程树确认 reaped 后，`NonProductOnly` 才可
返回有界 stdin count/SHA-256 且声明 closed/non-inherited/no-raw；stdout/stderr captured 且
extra/inherited descriptors 为零、无名称/路径；资源仅含有界 wall time、parent user/system CPU
和可选 peak resident bytes，不含环境、raw/network telemetry。exit/runtime evidence 使用独立
post-reap timeout，二者全部通过后才原子写入 Result；异常或重复采集变化返回
`StopEvidenceFailed`，不会留下半份证据。真实 OS adapter 仍只在 `_test.go` 启动当前 test
binary，产品 CLI/HTTP/Desktop/Agent/Sandbox/Local/Docker 没有 starter 或接线。

累计六切片完整健壮性门通过：最终 uncached 全仓 ordinary/race 分别 377.3/409.8 秒；普通与
secure Desktop test/vet、全仓 vet/staticcheck、普通和 Desktop-tag govulncheck、module verify/
tidy diff、37 文件 127 项 React、strict TypeScript、确定性 OpenAPI/TypeScript、Vite、npm 零
漏洞、隔离且清空真实 Provider 环境的 mock-only CLI smoke、凭据/产物/生产进程入口扫描、Linux
runner-test 交叉编译和 Windows 可复现双构建全部为绿色。OpenAPI 仍为 71 path / 77 operation /
170 schema；OpenAPI/TypeScript SHA-256 分别为
`7418F7CAEED0BA6A5E69E574215F22CC8AA47458A75FB70FD0679FDEDD332BA1` 与
`A3EE3B6E7E1020924B6AB1140F3EC4176A9550407187B7DDB3AA1E6FA15697CD`；未签名 GUI SHA-256
为 `1d51529b1a6d7d90e121e770faa54c9f4d77b4a96d3c0d920fe091178a299da2`，自动兼容检查通过但
`release_ready=false`。

组合审计未发现启用路径中的未解决高/中风险。泛化 secret 扫描只命中固定测试假值，生产
`exec.Command` 入口仍为零；本轮没有使用真实 Provider/Key、Shell、LocalRunner、Docker、
Git hook、攻击流量、外网、installer 或 registry mutation。双指标保持架构约 99%、完整产品
可用度约 95-97%；通用 Coding Agent 约 95-96%，Cyber 自动化约 20%。边界见 ADR 0055。

下一批候选为 D1-G8 bounded exact-commit comparison、D1-V7 verification pagination keyset/
event-high-water 加固与 R5 非产品 resource-limit/termination-cause evidence。真实 Local/Docker
start、Windows 10 人工矩阵、签名分发、Rust analyzer、xterm、网络授权与 CTF 继续独立设门。

## 八、仓库同步与恢复约定

规范远程仓库：`https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench`。

每三个聚焦切片组成一个交付批次；第三片后统一执行功能复核、普通/聚焦测试、组合差异审查、项目记忆更新、Git 提交、GitHub 推送和 CI 复核。每两个批次即六个切片再执行全仓 race、vet、staticcheck、govulncheck、依赖/隐私与完整构建健壮性门。当前仓库直接开发并推送 `main`；除非用户明确要求，不创建功能分支或 PR。

长对话恢复时依次阅读：`README.md`、`docs/PROJECT_MEMORY.md`、`docs/PROJECT_STATUS.md`、本文件、`docs/TASK_BOOK.md`、`docs/http-api.md`、`docs/errors.md`，再按序阅读 `docs/adr/0001-*.md` 到 `docs/adr/0055-history-navigation-verification-pagination-runner-runtime-evidence.md`。
