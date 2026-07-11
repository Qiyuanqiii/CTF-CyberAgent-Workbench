# CyberAgent Workbench V2 任务书

更新时间：2026-07-12

## 目标

在现有 Go 项目上构建可恢复、可审计、可审批、可扩展的通用 AI Agent Workbench。借鉴成熟 Agent 产品的运行体验与公开架构思想，但保持原创实现、Go 单一控制平面和安全优先策略。

CTF 专用求解继续排在最后。前置目标是让代码、审查、学习、脚本和安全分析任务共享同一套 Run、Coordinator、Tool、Memory、Finding 和 Report 基础设施。

## 当前基线

- 旧版 v0.1 通用 Agent 骨架：约 99%。
- V2 Run-centric Runtime：约 99%。
- 完整产品愿景：约 91%。

V2 的 99% 在 P0/P1 基础上完成了可恢复 Supervisor、预算、严格生命周期、Run-bound Session、Provider typed outcome/retry/SSE、有界 `model.delta`、active-call 查询/取消/订阅、Bubble Tea 实时取消、一次可跨重启恢复的协议修复、schema v16 有界结构化记忆工具循环、schema v17 跨进程 Run execution lease/心跳/fencing，以及 schema v18 独立 capability 的跨进程活动模型取消。P3 已加入 schema v9 Work Board、schema v10 Notes、事务关系/事件、`todo`/`note` CLI、可见性规则、8192-token Context Section 选择和 `model.started` 来源审计。P5 已统一工作区读取、Shell、FileEdit 与 workspace-scoped `script_process.v1` 提案入口，并新增 schema v11 持久化幂等审批账本、schema v12 可撤销 Session Grant 与原子工具预算、schema v13 独立脚本进程提案、schema v14 脱敏且来源绑定的 Run 输出 Artifact、schema v15 create-only 结构化工具与幂等账本，以及 schema v16 可恢复 Provider 工具批次。

费用预算、Coordinator 和真实 Sandbox 尚未实现；Run 工具预算、跨进程执行互斥与精确主动取消已落地。

## 执行原则

- 每个阶段必须形成可运行的纵向切片，不做一次性大重写。
- Go 是唯一主控；TypeScript 和 Rust 不得绕过 Go。
- 先单 Agent 恢复，再开放多 Agent 并发。
- 先审计和审批，再启用真实执行。
- SQLite 是状态真源；导出的 JSON/Markdown 只是投影。
- 每个阶段结束后执行进度审查、代码审计、功能复核和文档更新。
- 新功能必须有状态机测试、失败路径测试和 CLI smoke。

## P0：迁移地基

状态：完成

目标：在不破坏现有命令的情况下，为 V2 数据模型和演进建立安全地基。

- [x] ADR 0001：Go 单一控制平面。
- [x] ADR 0002：Run-centric 可恢复运行时。
- [x] V2 架构图、领域模型、状态机和包迁移目标。
- [x] 引入 `schema_migrations` 和有序、事务化、checksum 校验的数据库迁移。
- [x] 为现有数据库增加迁移幂等、失败回滚和旧库升级测试。
- [x] 统一 ID 生成入口、UTC/RFC3339Nano 持久化和 v1 JSON event envelope。
- [x] 定义跨 CLI/API 稳定错误码。
- [x] 建立兼容层原则，禁止一次性移动全部包。

验收标准：旧数据库可升级；重复启动幂等；失败迁移回滚；`go test ./...` 通过。

## P1：Mission、Run 与事件主干

状态：完成

目标：把一次用户目标和一次执行尝试分离，并形成统一活动时间线。

- [x] 定义 `Mission`、`Run`、`RunConfig`、`Scope`、`Budget`。
- [x] 新增 `missions`、`runs`、`run_events` 表和 Store 接口。
- [x] 实现 Run 状态机，并在 Domain 和 Store 两层拒绝非法转换。
- [x] 新增 `run create/list/show/start/pause/resume/cancel/events` CLI。
- [x] 把 Session、Policy、ToolRun、FileEdit 事件事务化投影到统一 Run 时间线。
- [x] 为旧 `agent.Task` 提供事务化、幂等兼容映射。

验收标准：创建 Run 后可退出进程并重新加载；状态和事件顺序一致；取消操作幂等。

## P2：单 Agent RunSupervisor

状态：主体完成；有界 Provider ToolCall 循环与跨进程执行租约已接入

目标：先让一个 Agent 在统一 Supervisor 下可靠启动、暂停、恢复和结束。

- [x] 定义 `RunSupervisor`、`RunHandle`、`LifecycleResult`。
- [x] 在模型调用前后持久化 checkpoint，并保证已完成 turn 不会因重启重复提交。
- [x] schema v17 增加持久化 Run execution lease、心跳续期、generation takeover 与 fencing token；`run execute` 全循环只持有一份租约，旧 worker 不能再提交 checkpoint/model/tool/entity 或消耗工具预算。
- [x] 执行一个无工具 root Agent turn，将消息、策略判定、模型用量和事件原子写入。
- [x] 在模型调用前执行 MaxTurns 与 context cancellation 检查。
- [x] 持久化累计 input/output/total tokens 与模型执行时间，并在调用前执行 MaxTokens 和 TimeoutSeconds。
- [x] 增加有界 `run execute` 循环与显式 `run finish/fail`，原子写入 Run 终态、checkpoint 和事件。
- [x] 实现严格 `root_lifecycle.v1` 的 `continue/finish/wait`，仅允许 Supervisor 原子解释和推进终态/暂停态。
- [x] 把 Run-bound Session chat 接入 Supervisor；未绑定 Run 的旧 Session 保留显式兼容路径。
- [x] 统一 Provider transport/rate-limit/invalid/cancelled/permanent outcome，增加有界退避和 `model.started/completed/failed` 持久化事件。
- [x] 复用 Anthropic-compatible transport 注册独立的 Mimo/DeepSeek 环境 Provider；Key 不进入配置、SQLite 或事件。
- [x] 对无效 `root_lifecycle.v1` 增加恰好一次的有界自动修复；修复阶段、原因、token 用量和四类修复事件可跨重启恢复，且与 transport retry 分开计数。
- [ ] 增加结构化依赖等待和未来 child `agent.finish`。
- [ ] 增加金额预算；turn、token、模型执行时间与 P5 Run 工具调用预算已落地。
- [x] schema v18 增加跨进程主动取消控制：独立 read/control token、精确 Run/Supervisor/model attempt 前置条件、原始幂等键不落库、worker lease-fenced 观察、Provider context 取消、终态解析与 stale attempt `superseded`。
- [x] 增加真实 Provider stream 聚合与 `model.delta`；执行 UTF-8、64 KiB、final usage、取消校验，每次模型调用最多持久化 32 条无文本进度事件。
- [x] Bubble Tea 消费进程内 active-call 元数据，展示调用进度/断线/终态，并通过 `Ctrl+X` 调用审计取消；UI 不持有 Provider context。

验收标准：单 Agent headless Run 只能通过生命周期结果完成；中断后恢复不重复已完成动作。

## P3：结构化 Work Board 与 Notes

状态：主体完成；结构化创建工具与模型自动调度已接入

目标：把计划和长期记忆从聊天文本中拆出来。

- [x] 定义 `WorkItem` 状态、优先级、依赖、Owner、验收条件和合法状态转换。
- [x] 定义 `Note` 分类、标签、Source/Evidence 引用、run/root/owner 可见性、pin 和 archive/restore 生命周期。
- [x] 新增 schema v9 `work_items` 与 schema v10 `notes`/关系表，事务化 `work_item.*` 和 `note.*` 事件。
- [x] 增加 `todo create/list/show/update/start/block/reopen/complete/cancel` CLI，并以 schema v15 注册 create-only `work_item_create` Tool Gateway 工具。
- [x] 增加 `note create/list/show/update/archive/restore` CLI、content-file 边界和乐观版本，并以 schema v15 注册 create-only `note_create` Tool Gateway 工具。
- [x] 对结构化创建工具执行严格 JSON、Run/Session/Workspace 绑定、Policy、Run 工具预算、敏感信息脱敏、原子领域/工具事件和 operation-key 幂等重放。
- [x] 将 Provider ToolCall 适配到 RunSupervisor 的受限工具循环；仅允许 create-only WorkItem/Note，每轮最多 4 个调用、每 turn 最多 4 轮，并可从 pending 批次恢复。
- [x] Supervisor 只加载最多 20 个活跃任务并生成不超过 16 KiB 的脱敏 JSON；活跃项存在时拒绝模型自行 `finish`。
- [x] Context Builder 在 8192-token 预算内选择摘要、Work Board、pin/category 加权的可见 Notes，并在 `model.started` 持久化来源 ID/token 元数据。

验收标准：长会话压缩后任务板与 Notes 不丢失；每个变更可在事件流中重放。

## P4：AgentCoordinator 与受控多 Agent

状态：待开始

目标：建立单一所有者的可寻址 Agent 图，再逐步开放并发。

- [ ] 定义 `AgentNode`、父子关系、角色、Skills、状态和 inbox。
- [ ] 新增 `agent_nodes`、`agent_messages` 表。
- [ ] 实现 Coordinator register/send/wait/finish/cancel/snapshot/restore。
- [ ] root 可创建 Specialist，但受并发数、深度、预算和 Skill 限制。
- [ ] 子 Agent 有独立 Session、WorkItem 和可见 Notes。
- [ ] 子 Agent 通过 `agent.finish` 返回结构化 CompletionReport。
- [ ] 实现图快照、消息唤醒、级联取消和崩溃通知。

验收标准：先通过单 Agent Coordinator 测试，再启用最多两个子 Agent；恢复后父子关系和 inbox 完整。

## P5：统一 Tool Gateway 与审批

状态：进行中；Gateway、持久化逐次审批、Session Grant、工具预算、first-class ScriptProcess、Run 输出 Artifact、create-only 结构化记忆工具与有界 Provider 循环已完成

可复用：`tools`、`policy`、`toolrun`、`fileedit`、`redact`。

- [x] 工作区安全读工具。
- [x] Shell 提案与 dry-run 审批。
- [x] 文件编辑提案、diff、审批和 stale hash 检查。
- [x] 统一 ToolCall/Proposal/Decision/Execution/Result 数据模型，并校验合法状态组合。
- [x] 将 ToolRun 和 FileEdit 接入同一 Approval Service；CLI、Session 与 TUI 保留兼容命令但不再直建 Manager。
- [x] 定义并执行自动允许、每次审批、可撤销会话授权、永久拒绝四类策略。
- [x] 新增 schema v11 `tool_approvals`/`approval_operations`，以请求指纹、不可变幂等键、Run/Session 关联和 `approval.requested/decided` 事件支持重启恢复。
- [x] SQLite 拒绝幽灵审批、提案指纹变化、幂等键意图冲突，以及没有持久化批准事实的 `approved`/`applied`/`completed` 状态。
- [x] 增加 `approval list/show` CLI，并让重复 Shell/FileEdit 审批在兼容状态机中幂等收敛。
- [x] schema v12 持久化可撤销 Session Grant，在精确 Run/Session/Workspace/Tool/ActionClass scope 下消费授权；终态 Run、归档 Session 和永久拒绝不可绕过。
- [x] schema v12 原子执行 Run 级工具调用预算，记录有序 charge 与一次性 exhausted 事件，并通过 `run usage` 提供可审计状态。
- [x] schema v13 以独立 `script_process_proposals` 存储类型化 executable/argv/backend/status；Mission、Session、Run、预算、Policy、Process、Approval 与事件在一个事务提交。
- [x] `script run --idempotency-key` 支持跨重试返回同一对象并拒绝异意图复用；原始 key 不持久化，并发创建只产生一个 Run 和一次预算扣减。
- [x] 通用 `tool list/show/approve/deny` 按审批账本分派 Shell 与 ScriptProcess，脚本审批只收敛为 `execution_mode=disabled` 的 dry-run。
- [x] 所有输出增加大小、MIME、UTF-8、脱敏和 Artifact 规则；schema v14 在 Result 截断前捕获最多 4 MiB 的脱敏正文，记录 SHA-256/来源与 `artifact.created`，并提供 `artifact list/show/read/verify`。
- [x] 移除 `script run --local` 的旁路执行；保留该参数仅记录请求后端，实际只创建 Script Profile Run 与 dry-run 工具提案。
- [x] schema v15 增加 `run_memory` action class、`work_item_create`/`note_create` 严格 schema、`structured_tool_operations` 摘要账本，以及 `tool schema/invoke` CLI；相同意图跨进程并发只创建一个实体。
- [x] schema v16 将 Provider ToolCall arguments 映射为规范化 Gateway payload，以 Run/turn/工具/脱敏意图生成稳定 operation key；模型 call ID 不持久化，并实现有界、可恢复、白名单结构化工具循环。

验收标准：没有工具可以绕过 Scope、Policy、Approval 和 Event Store；旁路执行扫描为零。

## P6：真实 Sandbox

状态：待开始

- [ ] 定义 Sandbox Manifest、Mount、NetworkScope、ResourceLimit。
- [ ] 引入 Docker Go client，实现 per-run 容器生命周期。
- [ ] 本地代码默认只读挂载，输出目录独立可写。
- [ ] 网络默认关闭，后续仅允许显式 allowlist。
- [ ] 支持执行、stdin、超时、取消、日志和 Artifact 导出。
- [ ] 清理幂等，残留容器可检测和回收。
- [ ] 保留 Noop/Local 作为测试与开发后端。

验收标准：容器内不能越界读取宿主目录；取消能终止进程；重启后能识别并处理残留 Sandbox。

## P7：Skills 与 Profiles

状态：待开始

- [ ] 定义 Skill manifest：名称、版本、描述、Profile、工具依赖和内容路径。
- [ ] 实现 Skill Registry、校验、按需加载和 token 预算。
- [ ] 为 `code/review/learn/script` 定义基础 Profile。
- [ ] Skill 内容进入上下文前做版本记录和脱敏。
- [ ] Specialist Agent 只分配完成任务所需的少量 Skills。
- [ ] CTF Skills 保留目录规范但暂不实现求解内容。

验收标准：Skill 可测试、可追踪、可热加载；未分配 Skill 不进入 Agent 上下文。

## P8：Finding、Evidence 与 Report

状态：待开始

- [ ] 定义通用 Finding 类型和验证状态机。
- [ ] 定义不可变 Evidence 引用和内容哈希。
- [ ] 新增 `findings`、`evidence` 表。
- [ ] 创建 Finding 必须引用可复核 Evidence。
- [ ] 使用确定性 fingerprint 去重，模型语义去重仅作辅助。
- [ ] 输出 Markdown、JSON，随后增加 SARIF 和 CI annotations。
- [ ] 报告从 Store 投影，不使用进程全局可变状态。

验收标准：同一根因不会重复计数；报告可从数据库完全重建；证据文件被修改时可检测。

## P9：TUI、Headless、API 与 TypeScript

状态：部分完成；loopback-only `api.v1` 读取面、独立授权的活动调用取消、Go 生成的 OpenAPI 3.1 契约、有界 Run-event SSE 与第一版 Run-aware TUI 已落地

可复用：Bubble Tea Session picker、消息区、工具审批和异步状态。

- [ ] TUI 改为消费统一 Event stream。
- [x] TUI 增加当前 Run 状态、Work Board、Notes、durable ToolRound 与 ToolRun 视图，并提供持久化“批准一次/本会话”Shell 操作。
- [ ] 增加 Run 列表、Agent 图、diff 和 Finding 视图。
- [ ] Headless 模式输出 NDJSON 事件并使用稳定退出码。
- [x] 基于标准库 `net/http` 提供 loopback-only read-first API，覆盖 Run、Session、Event、WorkItem、Note、Artifact metadata 与 Supervisor ToolRound。
- [x] Run detail 提供不含 `lease_id` 的 execution-lease 状态摘要；Run events 同样不暴露 fencing token。
- [x] 提供 Bearer token、Host/remote 回环校验、请求/响应上限、稳定 `api.v1` envelope、typed error 与 scope-bound cursor pagination。
- [x] 提供只读 Run-event SSE：持久化 sequence、Run-bound cursor/`Last-Event-ID`、heartbeat、写 deadline、事件/寿命/并发边界和 server shutdown cancellation；不增加模型正文。
- [x] 提供经过单独审计的主动取消入口；独立 control token 不能读取，read token 不能取消，客户端不能提供 fencing token。除这一精确操作外 HTTP API 保持只读；WebSocket 只在未来确有双向或模型正文需求时再引入。
- [x] 从 Go read DTO 生成确定性 OpenAPI 3.1/JSON Schema，提供鉴权端点、CLI 导出、golden 防漂移与 live-route contract tests；TypeScript 不手写安全规则。
- [ ] Go API 稳定后创建 React/Vite UI。
- [ ] Monaco/xterm.js 只展示 Go 授权的编辑和终端会话。

验收标准：CLI、TUI、CI、Web 对同一 Run 显示一致状态；关闭 UI 不会停止后台 Run。

## P10：Rust 确定性分析器

状态：待开始

- [ ] 固化 Go-Rust v1 JSON envelope、超时、大小限制和错误码。
- [ ] 实现 Analyzer Registry 和子进程管理。
- [ ] 首个分析器从明确需求中选择：archive、binary/object 或 PCAP。
- [ ] Rust 只读输入、写隔离输出，不能读取 API key 和全局配置。
- [ ] Go 验证 JSON Schema、退出码和 Artifact 后才写入事件/证据。

验收标准：Rust 进程崩溃、超时、畸形 JSON 和超大输出均被 Go 安全处理。

## P11：CTF 与安全分析能力

状态：最后阶段

- [ ] 在 Profiles/Skills/Finding/Sandbox 稳定后实现 CTF Mission Profile。
- [ ] 支持 challenge metadata、附件导入、类别识别和 writeup 投影。
- [ ] 将二进制、流量、Web、Crypto 分析映射到受控 Skills/Analyzers。
- [ ] 真实网络操作继续遵守授权 Scope 和审批。
- [ ] CTF 输出复用 Evidence/Finding/Report，不建立第二套运行时。

验收标准：CTF 只是通用运行时的 Profile；删除 CTF 包不会破坏 Agent 核心。

## 每轮交付模板

每次推进结束必须记录：

1. 本轮完成的 Task ID 和行为变化。
2. 当前 V2、v0.1 和完整产品进度。
3. 代码审计发现、风险等级和处置。
4. 单元测试、集成测试、race/vet 和 CLI smoke 结果。
5. 数据迁移、兼容性和安全边界复核。
6. 下一轮唯一推荐切片。
