# CyberAgent Workbench V2 任务书

更新时间：2026-07-10

## 目标

在现有 Go 项目上构建可恢复、可审计、可审批、可扩展的通用 AI Agent Workbench。借鉴成熟 Agent 产品的运行体验与公开架构思想，但保持原创实现、Go 单一控制平面和安全优先策略。

CTF 专用求解继续排在最后。前置目标是让代码、审查、学习、脚本和安全分析任务共享同一套 Run、Coordinator、Tool、Memory、Finding 和 Report 基础设施。

## 当前基线

- 旧版 v0.1 通用 Agent 骨架：约 98%。
- V2 Run-centric Runtime：约 88%。
- 完整产品愿景：约 72%。

V2 的 88% 在 P0/P1 基础上完成了 schema v8 Supervisor checkpoint、单次无工具 root Agent turn、崩溃恢复、累计 token/模型执行时间预算、有界执行循环、严格 `continue/finish/wait` root 生命周期协议、Run-bound Session chat 统一路径、Provider typed outcome、有限 transport 重试、Anthropic-compatible SSE 流聚合、有界 `model.delta` 账本、进程内 active-call 查询/取消/订阅、Bubble Tea 实时元数据/取消体验、模型终态事件，以及一次可跨重启恢复的协议自动修复。P3 已加入 schema v9 Work Board、依赖图、事务事件、`todo` CLI 和 Supervisor 活跃任务上下文。费用/工具预算、Notes、跨进程控制、Coordinator 和真实 Sandbox 尚未实现。

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

状态：进行中

目标：先让一个 Agent 在统一 Supervisor 下可靠启动、暂停、恢复和结束。

- [x] 定义 `RunSupervisor`、`RunHandle`、`LifecycleResult`。
- [x] 在模型调用前后持久化 checkpoint，并保证已完成 turn 不会因重启重复提交。
- [x] 执行一个无工具 root Agent turn，将消息、策略判定、模型用量和事件原子写入。
- [x] 在模型调用前执行 MaxTurns 与 context cancellation 检查。
- [x] 持久化累计 input/output/total tokens 与模型执行时间，并在调用前执行 MaxTokens 和 TimeoutSeconds。
- [x] 增加有界 `run execute` 循环与显式 `run finish/fail`，原子写入 Run 终态、checkpoint 和事件。
- [x] 实现严格 `root_lifecycle.v1` 的 `continue/finish/wait`，仅允许 Supervisor 原子解释和推进终态/暂停态。
- [x] 把 Run-bound Session chat 接入 Supervisor；未绑定 Run 的旧 Session 保留显式兼容路径。
- [x] 统一 Provider transport/rate-limit/invalid/cancelled/permanent outcome，增加有界退避和 `model.started/completed/failed` 持久化事件。
- [x] 对无效 `root_lifecycle.v1` 增加恰好一次的有界自动修复；修复阶段、原因、token 用量和四类修复事件可跨重启恢复，且与 transport retry 分开计数。
- [ ] 增加结构化依赖等待和未来 child `agent.finish`。
- [ ] 增加金额和工具调用预算；turn、token 和模型执行时间预算已落地。
- [ ] 增加跨进程主动取消控制；进程内 active-call registry、幂等审计取消、context 取消、终端信号取消和进程重启恢复已落地，跨进程入口等待 Go API。
- [x] 增加真实 Provider stream 聚合与 `model.delta`；执行 UTF-8、64 KiB、final usage、取消校验，每次模型调用最多持久化 32 条无文本进度事件。
- [x] Bubble Tea 消费进程内 active-call 元数据，展示调用进度/断线/终态，并通过 `Ctrl+X` 调用审计取消；UI 不持有 Provider context。

验收标准：单 Agent headless Run 只能通过生命周期结果完成；中断后恢复不重复已完成动作。

## P3：结构化 Work Board 与 Notes

状态：进行中

目标：把计划和长期记忆从聊天文本中拆出来。

- [x] 定义 `WorkItem` 状态、优先级、依赖、Owner、验收条件和合法状态转换。
- [ ] 定义 `Note` 分类、标签、Evidence 引用和 Agent 可见性。
- [x] 新增 schema v9 `work_items`、同 Run 依赖表和事务化 `work_item.created/changed` 事件；`notes` 表待后续 migration。
- [x] 增加 `todo create/list/show/update/start/block/reopen/complete/cancel` CLI；模型工具提案待统一 Tool Gateway。
- [ ] 增加 `note create/list/get/update` 工具与 CLI。
- [x] Supervisor 只加载最多 20 个活跃任务并生成不超过 16 KiB 的脱敏 JSON；活跃项存在时拒绝模型自行 `finish`。
- [ ] Context Builder 增加按需 Notes、token-aware 选择和来源引用。

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

状态：部分完成

可复用：`tools`、`policy`、`toolrun`、`fileedit`、`redact`。

- [x] 工作区安全读工具。
- [x] Shell 提案与 dry-run 审批。
- [x] 文件编辑提案、diff、审批和 stale hash 检查。
- [ ] 统一 ToolCall/Proposal/Decision/Execution/Result 数据模型。
- [ ] 将 ToolRun 和 FileEdit 接入同一 Approval Service。
- [ ] 定义自动允许、每次审批、会话授权、永久拒绝四类策略。
- [ ] 所有输出增加大小、MIME、UTF-8、脱敏和 Artifact 规则。
- [ ] 移除 `script run --local` 的旁路执行。

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

状态：部分完成

可复用：Bubble Tea Session picker、消息区、工具审批和异步状态。

- [ ] TUI 改为消费统一 Event stream。
- [ ] 增加 Run 列表、Agent 图、Work Board、Notes、diff 和 Finding 视图。
- [ ] Headless 模式输出 NDJSON 事件并使用稳定退出码。
- [ ] 基于 `net/http`/chi 提供本地 HTTP 与 WebSocket API。
- [ ] 生成 OpenAPI/JSON Schema，TypeScript 不手写安全规则。
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
