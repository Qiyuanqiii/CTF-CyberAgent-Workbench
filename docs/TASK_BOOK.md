# CyberAgent Workbench V2 任务书

更新时间：2026-07-15

## 目标

在现有 Go 项目上构建可恢复、可审计、可审批、可扩展的通用 AI Agent Workbench。借鉴成熟 Agent 产品的运行体验与公开架构思想，但保持原创实现、Go 单一控制平面和安全优先策略。

CTF 专用求解继续排在最后。前置目标是让代码、审查、学习、脚本和安全分析任务共享同一套 Run、Coordinator、Tool、Memory、Finding 和 Report 基础设施。

## 当前基线

- 架构完成度：约 98%；其中 V2 Run-centric 控制平面约 99%。
- 产品可用度：完整 Code + Cyber 产品约 45-50%。
- 通用 Coding Agent 工作流可用度：约 40%。
- Cyber 自动化工作流可用度：约 20%。
- 以上是依据可验证任务切片给出的工程估算，不是性能基准；从 schema v49 起停用单一“完整产品愿景”百分比。

V2 的 99% 在 P0/P1 基础上完成了可恢复 Supervisor、预算、严格生命周期、Run-bound Session、Provider typed outcome/retry/SSE、有界 `model.delta`、active-call 查询/取消/订阅、Bubble Tea 实时取消、一次可跨重启恢复的协议修复、schema v16 有界结构化记忆工具循环、schema v17 跨进程 Run execution lease/心跳/fencing，以及 schema v18 独立 capability 的跨进程 root 活动模型取消。P3 已加入 schema v9 Work Board、schema v10 Notes、事务关系/事件、`todo`/`note` CLI、可见性规则、8192-token Context Section 选择和 `model.started` 来源审计。P4 已加入 schema v19 单 root Coordinator、schema v20 摘要幂等 inbox、schema v21 默认关闭的 internal-only Specialist admission、schema v22 same-Run Agent-owned WorkItem/Note、schema v23 attempt-bound CompletionReport、schema v24 lease-fenced Specialist Attempt 调度/usage/崩溃恢复、schema v25 root inbox 两阶段 exactly-once context、schema v26 仅内部显式调用的 no-tool Specialist model turn 与原子模型账本、schema v27 可恢复 parent instruction/child-owned memory context、schema v28 child exactly-once lifecycle repair、schema v29 durable schedule summary/跨进程 child-call cancellation、schema v30 review-gated `specialist_delegation.v1` proposal、schema v31 独立且不授权执行的 operator review fact、schema v32 可恢复 operator application、schema v33 immutable read-only Fan-out plan 与 schema v34 bounded read-only execution，以及最多两个核心 child 的 Go-internal scheduler。核心委派不超过两个 child；Fan-out 可按 1/2/4/6 档运行无工具 JSON worker，共享 root+Specialist+Fan-out 总预算和取消扇出，但不创建 Agent、Attempt、schedule，也不具备写入、Shell、进程、网络或再委派权限。P5 已统一工作区读取、Shell、FileEdit 与 workspace-scoped `script_process.v1` 提案入口，并新增 schema v11 持久化幂等审批账本、schema v12 可撤销 Session Grant 与原子工具预算、schema v13 独立脚本进程提案、schema v14 脱敏且来源绑定的 Run 输出 Artifact、schema v15 create-only 结构化工具与幂等账本、schema v16 可恢复 Provider 工具批次，以及 schema v30 独立 `agent_proposal` 工具类。

P8 已推进到 schema v37：v35 将完成的 Fan-out execution 确定性投影为通用 `draft` Finding、不可变 `model_assertion` Evidence 和 `finding_report.v1` Report；v36 增加同 Run 冻结 Artifact Evidence、一次性 operator `validated/rejected` 决定和复核命令；v37 再以独立事实完成 `validated -> accepted -> fixed`，要求接受后新建且不可复用的 remediation Artifact Evidence。SARIF 只输出 `validated/accepted` 未解决项，默认 validated/high CI 门禁同样阻断二者，fixed/rejected 不再阻断。验证、接受和修复始终是三个不同阶段。

金额预算、HTTP 或模型自主 child 调度和真实 Sandbox 进程执行尚未实现；schema v48 的严格 Sandbox Manifest、schema v49 的精确审批/重新提交/禁用候选、schema v50 的禁用态 Artifact 绑定、独立 fencing、取消与清理恢复、schema v51 的禁用态后端/输出预检、schema v52 的仅模拟后端证据与内存输出事务、schema v53 的固定本机端点只读 Docker 观测、schema v54 的确定性容器计划与假写事务、schema v55 默认关闭的 Docker 创建/核验/删除演练、schema v56 的可恢复预写意图、代际租约和 stage/cleanup 检查点、schema v57 的 descriptor-pinned、kernel-sealed 宿主输入捕获证据、schema v58 的 daemon stage 前持久化捕获要求，以及 schema v59 的 daemon-owned、readback-verified、fully-cleaned 输入交接已经落地。v55-v59 仍不启动容器进程。operator-only 显式 child schedule/continue、no-tool child turn、最多两个 child 的有界并发、一次 child repair、Coordinator、Run 工具预算、跨进程执行互斥，以及 root/child 精确跨进程主动取消均已落地。

P7 已推进到 schema v47：schema v41 为每个 Run 固定 `code|cyber` 工作面与 `plan|deliver` 阶段；schema v42 增加严格三方向 `plan_delivery.v1` 提案与操作者幂等选择；schema v43 将操作者指令与非可信证据分离；schema v44 固化逐切片验证、审计和交接门禁；schema v45-v46 提供安全 turn 边界的 exactly-once 操作者队列、pending-only 取消和显式 drain；schema v47 再从父 Run 固定选择中为每个 Specialist Attempt 派生最多一项、Code/Cyber 分离、metadata-only 可恢复的 Skill 指导。模式、提案、选择、文档、审计声明、排队、取消和 Skill 指导本身都不授予工具、网络、Shell、写文件或子 Agent 能力。

## 执行原则

- 每个阶段必须形成可运行的纵向切片，不做一次性大重写。
- Go 是唯一主控；TypeScript 和 Rust 不得绕过 Go。
- 先单 Agent 恢复，再开放多 Agent 并发。
- 先审计和审批，再启用真实执行。
- SQLite 是状态真源；导出的 JSON/Markdown/SARIF 和 CI 判定只是投影。
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
- [ ] 增加结构化依赖等待；schema v23 已完成 child `agent.finish` 的持久化、幂等和父 inbox 回传。
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

状态：进行中；schema v32 核心委派保持最多两个 child，schema v33-v37 已完成独立只读 Fan-out、通用报告、验证、接受/修复、SARIF 与 CI 门，显式核心调度入口仍待后续切片

目标：建立单一所有者的可寻址 Agent 图，再逐步开放并发。

- [x] 定义 `AgentNode`、父子关系、角色、Skills、状态和有界 inbox；root 当前 `child_limit=0`。
- [x] schema v19 新增 `agent_nodes`、`agent_messages` 与有界 `agent_graph_snapshots` 表。
- [x] 完成单 root register/send/consume/wait/finish/cancel/snapshot/restore；Supervisor 与 RunService 在原事务内投影状态，Specialist 版本继续关闭。
- [x] schema v20 增加不落原始 key 的消息幂等账本、严格 `wake`/`dependency` payload、重放前置判定和 v19 消息/快照兼容升级。
- [x] schema v21 允许显式启用的 internal Coordinator 创建 Specialist；最多两个、深度 1、父 Skills 子集、正数子预算，并保留 root 协调额度。默认 Coordinator 仍关闭 admission。
- [x] 子 Agent admission 原子创建独立 Session，并在 Run 终态归档。
- [x] schema v22 为 WorkItem/Note 增加可选且同 Run 校验的 Agent identity；Supervisor/CLI 工具由 Go 注入调用 Agent，Note viewer 按 root/Specialist 身份隔离，旧 `owner` 标签与 v21 数据保持兼容。
- [x] schema v23 通过内部 `agent.finish` 返回严格 `agent_completion.v1` CompletionReport；报告绑定 active attempt，原子提交报告/父 result inbox/child 终态/Session 归档/事件/快照，并以摘要幂等账本收敛并发重试。
- [x] schema v24 通过默认关闭的内部 runtime 持久化 Specialist Attempt；开始、usage、continue、crash 均使用摘要幂等账本，turn 在调度时扣减，usage 恰好一次累计到 child token 预算。
- [x] schema v24 将 Attempt 绑定 Run execution lease/generation；接管后旧 worker 的 usage/finish/crash 全部被 fencing，新 worker 恰好一次恢复遗留 Attempt 并向父 inbox 写脱敏崩溃通知。
- [x] Run pause/wait/complete/fail/cancel 会先把 running Attempt 终结为 `interrupted`，再移动 child；graph restore 复核 Attempt turn 序列、累计 token、active projection 与 CompletionReport。
- [x] 实现有界图快照与 waiting Specialist 的显式消息唤醒；root/暂停 Run 不可被隐式唤醒。
- [x] 实现 Run pause/resume 与 Specialist 的原因感知联动，以及 complete/fail/cancel 的终态级联和 child Session 归档。
- [x] 实现 Specialist 崩溃通知、预算耗尽终止和 child Session 归档。
- [x] schema v25 让 root 通过最多 4 条、两阶段提交的 context projection 消费 direct-child dependency/result/notification inbox；失败不消费，取消/重启/lease takeover 重放同一批，模型看不到消息 ID、sequence、cursor，也不能伪造 sender。
- [x] schema v26 将一个内部 Specialist no-tool 模型 turn 接入现有 Provider、Run lease、预算、Policy 与 context cancellation 边界；`specialist_lifecycle.v1` 只允许 `continue` 或带 CompletionReport 的 `finish`，模型不能控制 usage/租约/重试/身份/权限。
- [x] schema v26 新增 `specialist_model_calls` 原子账本：模型终态、token/执行时间、Policy 结论、脱敏 child Session 消息与 Attempt usage 同事务提交；重试、取消、无效协议、工具调用拒绝和 lease takeover 均可审计且不重复计费。
- [x] schema v27 将直属 root 的严格 `specialist_instruction.v1`、child-owned active WorkItem/Note 与 bounded inbox 接入 SpecialistRunner；`specialist_context_deliveries` 两阶段账本绑定 AgentAttempt，成功 `continue`/`finish` 原子消费，crash/interruption/takeover supersede 后保持 pending，sender、owner、消费和 context provenance 均由 Go/SQLite 控制。
- [x] 在 Go 内部增加最多两个 child 的有界调度、取消扇出、32-round/生命周期/错误停止条件，以及每轮前后的 root+children token/执行时间 SQLite 总账复核；并发屏障、父取消、首错扇出、预算、篡改和 lease takeover 恢复测试通过。v38 后仅经 operator application gate 开放 CLI，仍未开放 HTTP/model spawn。
- [x] schema v28 为 child 无效 lifecycle 增加恰好一次有界 repair；repair 与 transport retry 分开计数，usage 累加到同一 Attempt，原始坏输出不进入 Session、事件或 repair prompt，预算/取消/takeover 会终结 pending repair。
- [x] schema v29 为内部 scheduler 增加 lease-fenced start/stop summary；正常/失败/取消写入目标、轮数、turn、恢复数、停止原因和前后总预算，后续 generation 将遗留 running schedule 恰好一次收敛为 `abandoned/worker_lost`。
- [x] schema v29 增加独立 Specialist model cancellation/operation 账本与 control API；请求精确绑定 Run/Agent/AgentAttempt/model attempt，worker 持私有 lease 观察后取消本地 Provider context，终态原子解析，原始 key、模型正文和 fencing token 不进入响应/事件。
- [x] schema v30 定义受限 `specialist_delegation.v1` proposal：root 最多提出两个有界目标、Skill 子集和预算建议；Go 在 active root/lease/scope/capacity/budget 复核后原子持久化脱敏且不可变的 proposal/assignments/digest-only operation，模型不能直接 admission 或 spawn。
- [x] schema v31 为 proposal 增加独立 operator review fact：一次性 approved/rejected、拒绝理由脱敏且不进事件、摘要幂等重放、改意图/第二次改判冲突，结果始终不授予 admission。
- [x] schema v32 为 approved proposal 增加可恢复 application：重新复核 Policy/review operation/Session/idle runtime/容量/预算/parent Skills，以 assignment 级 deterministic operation 幂等接入 admission 并投递严格父指令；Agent/Message 提交后的中断可恢复，终态 Run 原子 abort，root/无关 mutation/scheduler 无法抢跑。
- [x] schema v33 增加独立 planning-only `readonly_fanout.v1`：档位为 `auto/1/2/4/6`，固定 workspace-list/read 能力包络，生成不可变 snapshot manifest/确定性 shards/digest-only operation；不创建 Agent、不调用 Provider、不开放执行。
- [x] schema v34 增加 lease-fenced 只读执行状态机：执行前重建 manifest 并逐文件复核 identity/size/hash，最多六个无工具 JSON worker，共享 root+Specialist+Fan-out Run 总预算与取消扇出；execution/shard/model-call/finding/operation 可恢复持久化，未知崩溃调用按预留额度计费，不具备写入/网络/再委派权限。
- [x] schema v35 将 v34 shard findings 确定性汇总、精确事实去重并投影到 P8 通用 Finding/Evidence/Report；支持 Markdown/JSON，不由模型修改事实或严重度。
- [x] schema v38 增加显式 operator child `schedule/continue`：只接受原 application operator 和其中已 instructed 的一到两个 ready child，以不可变 request/target/operation/attempt 账本复用现有两-child scheduler、总预算、取消与 lease takeover；模型、HTTP 和普通工具不能启动或扩权。

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
- [x] schema v30 增加 Supervisor-only `specialist_delegation_propose` 与独立 `agent_proposal` class；严格协议在预算前校验，语义越权作为有界错误回传，成功结果明确 `admission_authorized=false`，CLI 仅提供只读 proposal 检查。

验收标准：没有工具可以绕过 Scope、Policy、Approval 和 Event Store；旁路执行扫描为零。

## P6：真实 Sandbox

状态：进行中；schema v48 Manifest、v49 审批候选、v50 禁用生命周期、v51 禁用后端/输出预检、v52 仅模拟证据/输出事务、v53 固定端点只读 Docker 观测、v54 确定性容器计划/假写事务、v55 未启动容器写演练和 v56 崩溃可恢复 attempt 已完成，真实进程执行未开放

- [x] 定义严格 `sandbox_manifest.v1`、Mount、NetworkScope、ResourceLimit、环境、输入/输出、超时与取消宽限，并提供 Noop 校验和 CLI 检查。
- [x] schema v48 持久化 metadata-only preparation/validation/operation，精确绑定 Run/Mission/Workspace/Scope/Policy/可选审批；摘要幂等重放与跨 Store 并发收敛。
- [x] 在 Go 类型、SQLite 与 CLI 固定 `backend_enabled=false`、`execution_authorized=false`；Local/Docker 继续关闭失败，审批不能绕过永久 Policy 拒绝或启动进程。
- [x] schema v49 增加精确 Sandbox 审批请求、操作者批准/拒绝和重新提交 Manifest 的再校验桥；用 `os.Root` 解析工作区源路径，在事务中复核预算/lease，只持久化不可执行候选。
- [x] schema v50 增加不可变禁用 execution、精确输入 Artifact 快照、metadata-only 输出计划、独立 generation-fenced Sandbox lease、取消事实和终态 Run 可恢复清理；全部后端能力仍为 false。
- [x] `run sandbox begin|cancel|cleanup|executions|execution-show` 接入同一 Go 服务；CLI 不显示 lease token/owner、Manifest、命令、路径或 Artifact 正文。
- [x] 证明 generation takeover、旧 worker 拒绝、初始 lease 崩溃恢复、跨 Run Artifact 拒绝、输入哈希复核、不可变 SQL、v49 原地升级和幂等重放。
- [x] schema v51 固定 16 项 Docker/后端威胁模型、禁用握手、未绑定容器身份和 metadata-only 输出导出计划；所有检查保持 required/unverified/not-probed，全部执行与 Artifact 提交能力保持 false。
- [x] `run sandbox preflight|preflights|preflight-show` 重新提交 Manifest 并复核 v48-v50 权限链；CLI/事件不显示 locator、原始路径、命令、Manifest、容器身份、operation digest 或内部 lease。
- [x] 证明 v51 同键跨 Store 收敛、异键/异意图冲突、取消态拒绝、SQL 不可变、v50 原地升级、事件隐私，以及 all-or-nothing/总字节/MIME/普通文件/symlink/特殊文件/重启协调策略固定。
- [x] schema v52 增加仅内存 `SimulationBackendClient`，把 OCI 镜像摘要与 daemon、挂载、网络、密钥、容器配置、资源、终止、orphan 和输出计划分别绑定到 16 项 `simulated_pass` 证据；全部保持 `verified=false`、`production_verified=false` 和零执行授权。
- [x] schema v52 增加严格 `sandbox_output_fixture.v1`、MIME/总字节/普通文件/symlink/特殊文件/脱敏检查与原子 `FakeArtifactSink`；注入失败或取消回滚为零，生产 `run_artifacts` 不发生写入。
- [x] `run sandbox evidence|evidences|evidence-show|output-simulate|output-simulations|output-simulation-show` 接入同一 Go 服务，并在两个新边界重验完整 v48-v51 权限链、预算、lease 与输入 Artifact；数据库、事件和 CLI 不保留夹具正文、路径、命令、Manifest、密钥、容器 ID 或私有 lease。
- [x] 证明 v52 同意图跨 Store 并发收敛、异意图冲突、取消恢复、假事务回滚、SQL 不可变、v51 原地升级、事件/CLI 隐私和每个 evidence 最多 8 次模拟；模拟结果不能升级为生产验证。
- [x] schema v53 增加最小 `DockerReadOnlyTransport`，Linux 只连接固定 `/var/run/docker.sock` 并只允许 `/_ping`、`/version`、`/info` 和精确镜像摘要 inspect 四类 GET；Windows 明确记录 `transport_unsupported`，不读取 `DOCKER_HOST`、不调用 Docker CLI，也不暴露 create/start/run/exec/pull/remove。
- [x] `run sandbox observe|observations|observation-show` 要求显式 `--confirm-readonly-probe`，绑定同一 v52 evidence、output simulation 和完整 Manifest，再在 Go/SQLite 重验 v48-v52 权限链；三类结果均保持零生产验证、零后端/执行/Artifact 授权。
- [x] 证明 v53 完整/daemon 不可用/镜像不可用状态、重复 JSON/重定向/非固定端点拒绝、context 取消、无 mutation 方法、同意图不重探、跨 Store 收敛、取消后拒绝、SQL 不可变、v52 原地升级、事件/CLI 隐私和每个 simulation 最多 8 次观测；private mount 明确保持 `not_observable_read_only`。
- [x] v53 最终本地发布门禁通过普通/race、静态分析、依赖/漏洞、前端、仓库扫描、diff 与真实二进制完整链路 smoke；修复并发语义收敛与 HTTP 内层白名单两项低风险问题，未确认探测不落库，Windows 只记录受限 unsupported 结果，生产 Artifact 保持为零。
- [x] 修复 Linux CI 暴露的 CLI 单测环境耦合：测试仅进程内注入确定性 unavailable observer，生产 CLI 默认固定端点与 opt-in 真实 daemon 集成测试保持不变；GitHub Actions run `29368979988` 已通过修复提交 `fe7b070`。
- [x] schema v54 定义确定性 Docker container-spec 编译器，在任何真实 daemon 写调用前固定 non-root、只读根/输入、唯一可写输出、private mount、network default-deny/精确 allowlist、临时 secret、resource/time/kill、orphan 和停止后导出约束。
- [x] schema v54 增加纯内存七步 fake write transaction 与失败/崩溃/取消零提交证明；Application/SQLite 重验 v48-v53，计划、事件和 CLI 不保留命令、路径、目标、环境值、secret 引用或容器身份，所有生产能力固定为 false。
- [x] `run sandbox docker-plan|docker-plans|docker-plan-show` 要求显式 `--confirm-fake-write`，同意图重放不重复假写，跨 Store 收敛；真实 Docker 写 API、生产 Artifact 和执行授权仍不可达。
- [x] schema v55 引入与 v53 observer 隔离、默认关闭的最小 Go Docker 写 transport；Linux 固定本机 socket 和 API `1.40`，闭合白名单只有精确 image/container inspect、create 和固定 `v=1` 匿名卷清理的 non-forced delete，不接受环境端点、TCP、任意 socket、代理、重定向、pull、start、exec、attach、日志、导出、卷管理或通用请求。
- [x] `run sandbox docker-rehearse|docker-rehearsals|docker-rehearsal-show` 要求显式 `--confirm-daemon-write` 和精确当前 v54 计划，只接受无网络/无环境/无密钥 profile；create 前核对本地 RepoDigest 并拒绝镜像声明 `VOLUME`，再创建摘要镜像的未启动容器，精确核验后删除。
- [x] v55 重新核验 v48-v54 全链，拒绝 symlink/越界/非普通 mount 源和不完全匹配的名称碰撞；仅精确且未启动的旧演练容器可回收，失败、取消或 create 响应不确定时用独立 context 重新 inspect 且绝不盲删，同意图重放不访问 daemon，双 Store 并发收敛，持久化/事件/CLI 不保留原始容器 ID、宿主路径、命令、环境值、密钥、socket 或完整规格。
- [x] v55 固定 `container_never_started`、`process_never_executed`、`image_never_pulled`、`output_never_exported` 为 true，并固定生产执行、验证、后端启用、执行授权和 Artifact 授权为 false；提供默认跳过、只接受已存在摘要且禁止 pull 的 Linux opt-in 集成测试。
- [x] v55 最终本地发布门禁通过全仓普通/race、静态分析、模块/漏洞、17 项前端测试、OpenAPI/构建/audit、仓库隐私扫描、diff 和隔离真实二进制 smoke；审计修复未知 create 结果回收、禁止盲删、镜像声明卷副作用及 attachment/device/port/capability 核验，未发现未解决高/中风险；GitHub Actions run `29382661971` 通过。
- [x] schema v56 在首个 daemon 写入前持久化不可变 attempt/intent，并用有界、递增 generation 的 SQLite lease 隔离 acquire/release/takeover；stage、cleanup、failure 和 completion 只接受当前未过期 owner/generation，旧 generation 即使重放同一检查点也会拒绝。
- [x] v56 将 transport 拆为可恢复 Stage/Cleanup：未知 create 结果或旧 generation 留下的精确 stopped authority match 会被收养而不再次 create；cleanup 只删除 authority、配置、request 和 container-ID 指纹全部匹配的对象，already-absent 幂等成功，同名不匹配对象绝不删除。
- [x] v56 落库 19 项固定 ordinal/name 的不可变检查矩阵，Go 与 SQLite 同时强制 `execution_evidence=false`；镜像与容器 inspect 都必须证明继承环境为空，原始 ID、宿主路径、命令、环境值、密钥、socket、完整规格、operation key 和私有 lease owner 不进入公开账本。
- [x] `docker-attempts|docker-attempt-show|docker-attempt-resume` 支持 metadata-only 查询和按持久化 attempt ID 恢复；恢复必须完整重交 Manifest、再次确认 daemon 写入、保持 requester/intent 完全一致，不依赖调用方保存原始 operation key。
- [x] v56 定向测试覆盖未知 create 后恢复、单次 create、already-absent、无关同名保护、释放/过期接管、旧 generation fencing、双 Store 竞态、原子 v55/v56 completion、隐私、不可变 SQL、v55 升级和 CLI。全仓普通/race、vet/staticcheck、模块/漏洞、17 项前端测试、OpenAPI/构建/audit、仓库隐私/链接/编码扫描、diff 和隔离真实二进制 smoke 均通过；高频 transport/Store/Application 恢复回归通过。GitHub Actions run `29388724727` 已通过功能提交 `e1710bb`，Go 与 TypeScript 作业分别用时 2 分 32 秒和 23 秒。
- [x] schema v57 增加独立宿主输入捕获门禁：Linux 使用 `openat2` 固定工作区根与只读树，`RESOLVE_NO_XDEV` 禁止跨挂载点，`O_PATH` 在内容打开前拒绝 FIFO/特殊文件；目录和单文件 mount 均支持，symlink/magic-link/hard-link 与资源越界失败关闭。重新核对 descriptor 后生成确定性 tar，在 sealed `memfd` 中复读校验，并把 metadata-only evidence 绑定当前 v56 attempt、计划、container-ID 指纹、输入摘要和 lease generation。
- [x] v57 的 intent/result、事件与 CLI 保持路径/正文/fd/raw container ID/私有 lease 不落库；SQL 禁止 pending intent 绕过 completion，失败先清理停止容器，接管恢复不重复 create。随机 row ID 不参与语义指纹，跨 Store 独立重试收敛；漏交恢复确认在 acquire 前拒绝且不消耗 failure slot。定向测试覆盖 rename/replace/delete/symlink/hard-link/FIFO/单文件、有界目录枚举、取消、重放、generation fencing、迁移和隐私；全仓普通/race、静态/漏洞、前端与隔离二进制门禁通过。
- [x] v57 GitHub Actions run `29396264276` 已通过提交 `8719dff`，Go/Linux 3 分 55 秒、TypeScript 23 秒。首次 run `29395980413` 仅暴露单文件测试夹具未覆盖目录工作路径，修复为合法目录+单文件混合 mount 后 Linux 运行测试通过，生产实现未变。
- [x] schema v58 在同一事务中创建 attempt、首代 lease、审计事件与不可变 `sandbox_docker_host_input_requirement.v1`，并在任何 daemon stage 前固定是否必须执行 v57 捕获。事实绑定 attempt/plan、Run/Mission/Workspace、requester、operation digest、Manifest/mount/input/authority 指纹和输入计数；路径、正文、fd、raw container ID、原始 operation key 与私有 lease 不落库。
- [x] v58 恢复以持久事实为准：required attempt 即使不重交 staging flags 也会完成 v57 捕获，false requirement 不能随后扩权；Go/SQL 双层阻止 required 无证据 completion、false requirement staging、跨 attempt/plan 复用、更新或删除。legacy v57 attempt 不回填虚构意图并保留兼容行为。迁移、不可变性、隐私、并发收敛、operation replay、崩溃恢复与 CLI 定向测试已覆盖。
- [x] v58 设计审计确认 Docker archive 写入与只读目标不兼容，因此没有通过放宽 `ReadonlyRootfs` 或输入可写性来“解决”交接；本切片不新增 archive、volume、start、exec、pull、build、export 或 Artifact 权限，v57 事实继续固定 `daemon_consumed=false`、`execution_evidence=false`。决策记录见 ADR 0018。
- [x] v58 发布门禁通过全仓普通/race（158.1 秒/168.4 秒）、vet/staticcheck/module/govulncheck、严格 TypeScript、17 项前端测试、OpenAPI/构建/npm audit、仓库隐私/产物/进程/编码/链接扫描、Linux 交叉编译与隔离真实二进制 smoke。高频 domain/Store/Application/race 回归通过；审计修复 pending operation-key 恢复候选错误、不成对 flags、迁移后 direct-SQL 缺失 requirement 和 false requirement 零输入兼容性，未发现未解决高/中风险。
- [x] v58 GitHub Actions run `29400696276` 已通过功能提交 `4b570f7`，Go/Linux 2 分 39 秒、TypeScript 23 秒。
- [x] schema v59 单独实现 daemon-owned local-volume carrier：四重显式确认、不可变 requirement、写前 intent、固定 `/cyberagent-input/bundle.tar`、精确 daemon 回读长度/摘要、删除可写 carrier、只读 volume 目标复核和全部资源删除。Manifest mount 保留区、exact crash residue、foreign collision、early-failure cleanup、第二代 lease 恢复、迁移/不可变/隐私与 Go/SQL completion gate 已覆盖；start/exec/export 权限仍为 false，决策见 ADR 0019。
- [x] v59 增加 Linux opt-in real-daemon handoff harness；只接受已存在且满足 v55 profile 的精确 digest，不 pull、不 start，并断言 target/carrier/volume 最终全部不存在。本机 Windows 只完成 Linux test binary 交叉编译，真实运行仍待 Linux 环境。
- [ ] schema v60 单独设计并审计 verified bundle 到 Manifest target 的 runtime input projection，以及 per-run start/wait/TERM/KILL/orphan 生命周期；不得把 v59 handoff 当作进程隔离证据。
- [ ] 本地代码默认只读挂载，输出目录独立可写。
- [ ] 网络默认关闭，后续仅允许显式 allowlist。
- [ ] 支持真实执行、stdin、超时/kill、日志和原子 Output Artifact 导出；v51 只固定要求，v52 只验证输出假事务，v53 只读取元数据，v54 只编译并假写，v55-v56 只操作未启动容器，v57-v58 只固定本地输入，v59 只完成 never-started daemon handoff，均未启用进程执行或生产 Artifact。
- [ ] 将 v50 幂等清理扩展为真实运行中容器 orphan 检测/回收，并用独立生产证据逐项验证 v51 检查；v52 的 `simulated_pass`、v53 的 `production_observed`、v54 的 `compiled_not_applied`、v55-v56 的非启动 inspect/recovery、v57-v58 的本地捕获事实与 v59 的 never-started handoff 都不计入进程隔离验证。
- [x] 保留 Noop/Local 作为测试与开发接口；Local 当前明确禁用，不能作为旁路执行后端。

验收标准：容器内不能越界读取宿主目录；取消能终止进程；重启后能识别并处理残留 Sandbox。

## P7：Skills 与 Profiles

状态：进行中；只读 `skill.v1` Registry、schema v39 Run 选择、schema v40 root 上下文交付、schema v41 Go-owned Run 模式、schema v42 Plan 提案/操作者选择、schema v43 来源隔离、schema v44 Delivery 检查点门禁、schema v45-v46 操作者引导队列与控制，以及 schema v47 Specialist 最小 Skill 上下文已完成

- [x] 定义有界 `skill.v1` manifest：名称、版本、描述、Profile、工具依赖、内容路径、字节数、保守 token 上界与 SHA-256。
- [x] 实现内嵌只读 Skill Registry、严格 JSON/UTF-8/路径/校验和验证，以及 `skill list/show/validate`；命令不创建运行数据库。
- [x] 为 `code/review/learn/script` 注册 `1.1.0` 最小工作流指导和窄工具前置声明；声明不授予能力。
- [x] schema v39 实现 Run 级不可变 `skill_selection.v1`：Profile 匹配、版本/内容哈希固定、总 token 预算、摘要化幂等操作、并发收敛与元数据来源事件。
- [x] schema v40 按持久化选择从内嵌 Registry 受控加载正文，并在每次 root turn 准备时复核版本、哈希、字节数与 Profile。
- [x] Skill 内容进入 root 上下文前脱敏并使用独立预算；准备/首次模型调用以 metadata-only 两阶段账本绑定，正文不落库。
- [x] 以内嵌、每 Skill 最多 8 个版本的有界历史索引保留旧 Run 精确恢复；新选择只解析当前版本，外部路径不能注入历史。
- [x] schema v41 增加不可变 `run_mode.v1`：`code|cyber` 工作面、`plan|deliver` 阶段、摘要幂等变更、活动租约门禁、Plan 完成拒绝、v40 `code/deliver` 兼容回填，以及 CLI/TUI/HTTP/Web 一致投影；模式不授予权限。
- [x] schema v42 增加跨 Profile `plan-delivery` 内置 Skill 与严格 `plan_delivery.v1`：Plan root 只能提出恰好三个有界方向，操作者在暂停且无租约时幂等选择 1/2/3，并在同一事务投影 WorkItem 依赖图、置顶 decision Note 与元数据事件；CLI 是唯一选择入口，HTTP/TUI/Web 只读，选择不切阶段或授予能力。
- [x] schema v43 增加 `context_provenance.v1`：新 Session 消息持久化严格来源、授权位与脱敏正文摘要；`/read`、目录、Diff、工具和命令输出固定为无指令权限的 tool evidence；普通 Session、root、Specialist 与压缩摘要统一使用 untrusted JSON 投影；v42 历史保守迁移，SQLite 不可变与 Go 摘要复核共同防篡改。
- [x] schema v44 增加不可变 `delivery_checkpoint.v1`：绑定被选 WorkItem、验收条件、源模块、Deliver revision 与 WorkItem version；完成前要求聚焦验证、Diff/安全审计和交接 Note，最终模块再要求整体验证与健壮性审计；Go/SQLite 双层完成门禁，CLI 是唯一写入口，HTTP/TUI/Web 只读。
- [x] schema v45 增加持久化操作者引导队列：Run 忙碌时按序接收追加要求，在安全 turn 边界投递且不打断活动工具调用；失败/重启重新准备、Session 消息原子提交、后续输入延后 finish/wait，HTTP/Web 只读且不公开正文或内部身份。
- [x] schema v46 增加 pending-only 操作者取消账本、明确 idle/paused wake/drain 策略和普通 Session 跨进程幂等标识；禁止编辑、重排或取消 prepared 消息，模型/child/HTTP 不获得写入权限。
- [x] schema v47 为每个 Specialist Attempt 从父 Run 固定选择派生最多一项 Skill；Code/Cyber 目录分离，`plan-delivery` 保持 root-only，assignment 不能选择或扩权，metadata-only 两阶段来源账本与首次模型调用原子绑定。
- [ ] CTF Skills 保留目录规范但暂不实现求解内容。

验收标准：Skill 可测试、版本固定、来源可追踪；未分配 Skill 不进入 Agent 上下文，声明的工具依赖不产生授权。

## P8：Finding、Evidence 与 Report

状态：进行中；schema v35 已完成不可变 draft 投影，schema v36 已完成冻结 Artifact Evidence 与一次性 operator 验证/拒绝，schema v37 已完成独立 acceptance、fresh remediation Evidence 与 fix；当前 runtime 已完成 confirmed-unresolved SARIF、通用 CI gate 与同 GateResult 派生的 GitHub Actions annotations，其他平台 adapter 可后续增加

- [x] 定义通用 Finding 类型、`draft/validating/validated/accepted/fixed/rejected` 状态枚举；v35 创建不可变 `draft`，v36 开放一次性 `draft -> validated|rejected`，v37 以独立不可变事实开放 `validated -> accepted -> fixed`。
- [x] 定义不可变 Evidence 引用、源 finding fingerprint 与源 report digest。
- [x] 新增 `finding_reports`、`findings`、`finding_evidence` 表及 `building -> generated` 完整性门。
- [x] 创建 Finding 必须引用可复核的 v34 `model_assertion` Evidence。
- [x] 使用确定性 fingerprint 做精确事实去重；严重度不同绝不合并，重复置信度取保守最小值。
- [x] 输出字节稳定的 Markdown、JSON。
- [x] 增加同 Run Artifact-backed Evidence：重新校验完整 blob、冻结 Artifact 更新/删除、记录脱敏标志，并以不可变摘要操作账本收敛并发重放。
- [x] 增加一次性 operator `validated/rejected` 决定；validated 至少需要一份 Artifact Evidence，决定后禁止追加 Evidence，原 v35 投影摘要保持不变。
- [x] 增加 `report finding attach/validate/reject/accept/remediation attach/fix/verify` 与 Markdown/JSON 完整生命周期覆盖层。
- [x] 增加只读 SARIF 2.1.0：只输出 `validated/accepted` confirmed-unresolved Finding，稳定 rule/相对 URI/fingerprint，草稿、已修复和拒绝项不进入 `results`。
- [x] 增加 `report check` 通用 CI gate：默认 `validated/high` 同时阻断 accepted，只有显式 `active` 才纳入 draft，fixed/rejected 永不阻断。
- [x] 增加独立 `accepted/fixed` 生命周期：接受冻结 validation 快照；修复 Evidence 必须来自同 Run、接受事件之后的新 Artifact 且不能复用验证 Artifact；fix 冻结修复 Evidence 集合。
- [x] 增加 GitHub Actions 平台 CI annotations：`report check --format github` 复用同一 GateResult、严格 workflow-command 转义、保留退出码且不输出私密 lifecycle 叙述；其他平台按独立 renderer 后续扩展。
- [x] 报告从 Store 投影，不使用进程全局可变状态，也不发起额外 Provider 调用。

验收标准：同一根因不会重复计数；报告可从数据库完全重建；证据文件被修改时可检测。

## P9：TUI、Headless、API 与 TypeScript

状态：部分完成；loopback-only `api.v1` 读取面、独立授权的 root/Specialist 活动调用取消、Go 生成的 OpenAPI 3.1 契约、有界 Run-event SSE、`headless.v1` NDJSON 与稳定终态退出码、持久事件驱动的 Run-first TUI、React/Vite Run/Agent/delegation/Fan-out/Finding 只读控制台及显式 Go 同源生产托管已落地

可复用：Bubble Tea Session picker、消息区、工具审批和异步状态。

- [x] TUI 以同一持久化 Run event sequence 驱动有界轮询和复合投影刷新；严格绑定 Run/Mission、拒绝 gap/超界记录、丢弃陈旧结果、终态停止，并以前后 event tail 复核避免跨表撕裂快照。
- [x] TUI 增加当前 Run 状态、Work Board、Notes、durable ToolRound 与 ToolRun 视图，并提供持久化“批准一次/本会话”Shell 操作。
- [x] TUI 增加最近 Events、Agent 图/Completion 与有界 Finding 报告摘要只读视图；所有审批快捷键仍只在 Tools 页生效。
- [x] 增加最近 50 条的 Run-first Run/Session 双选择器、`tui --run` 精确打开，以及最近 20 条 FileEdit 的 metadata/diff-only 只读详情；查询不选择原文/替换正文，显示另受 128 KiB/4096 行上限约束，完整 Finding/Evidence 详情继续由现有 CLI/Web 按需读取。
- [x] Headless 模式输出版本化、有界、sequence 可恢复的 NDJSON Run events 与最终 `stream.end`；stdout 只含 JSON，完成/失败/取消/上限/超时使用稳定退出码 0/4/7/8/9，不新增执行状态机。
- [x] 基于标准库 `net/http` 提供 loopback-only read-first API，覆盖 Run、Session、Event、WorkItem、Note、Artifact metadata 与 Supervisor ToolRound。
- [x] Run detail 提供不含 `lease_id` 的 execution-lease 状态摘要；Run events 同样不暴露 fencing token。
- [x] 提供 Bearer token、Host/remote 回环校验、请求/响应上限、稳定 `api.v1` envelope、typed error 与 scope-bound cursor pagination。
- [x] 提供只读 Run-event SSE：持久化 sequence、Run-bound cursor/`Last-Event-ID`、heartbeat、写 deadline、事件/寿命/并发边界和 server shutdown cancellation；不增加模型正文。
- [x] 提供经过单独审计的主动取消入口；独立 control token 不能读取，read token 不能取消，客户端不能提供 fencing token。除这一精确操作外 HTTP API 保持只读；WebSocket 只在未来确有双向或模型正文需求时再引入。
- [x] 从 Go read DTO 生成确定性 OpenAPI 3.1/JSON Schema，提供鉴权端点、CLI 导出、golden 防漂移与 live-route contract tests；TypeScript 不手写安全规则。
- [x] 从 OpenAPI snapshot 生成 React/Vite DTO，提供 Run/Session 列表、详情、Work/Notes/Artifact descriptor/ToolRound、预算/租约和带 Authorization header 的可恢复 SSE 视图；read token 只驻留内存，Vite 代理只允许回环目标。
- [x] 由 Go 在显式 `--ui-dir` 配置下同源托管不可变生产 Web bundle，补齐 route-aware CSP、SPA fallback/类型/大小/软链接边界、缓存语义和真实 CLI/浏览器集成测试；API capability 不变。
- [x] 在 Go DTO/契约先行的前提下增加 Agent graph、delegation/Fan-out 与 Finding/Report 只读 Web 视图；列表有界分页，Fan-out 摘要查询不读取 raw report/digest/lease，生命周期 DTO 不公开私有 operator narrative 或 Artifact 正文。
- [x] 在 Run detail、TUI 与 React 增加 schema v42 Plan/Delivery 只读投影；只显示有界方向/模块、选择与 WorkItem 映射，不公开 operation/lease/requester 内部身份，也不提供选择或阶段切换控件。
- [x] 扩展 CLI/TUI/HTTP/Web/Headless 跨入口契约矩阵：固定 `running/paused/completed/failed/cancelled`、终态退出码与事件尾；以 53 条真实 Run/Session 验证 TUI 50 条截断和 HTTP 20/20/13 opaque-cursor 分页，并固定空集合及 Headless 从尾序号零事件续传。
- [ ] Monaco/xterm.js 只展示 Go 授权的编辑和终端会话。

验收标准：CLI、TUI、CI、Web 对同一 Run 显示一致状态；关闭 UI 不会停止后台 Run。当前 golden 已固定五类 Run lifecycle、Run/Mission/Session/status、完整 event sequence/tail、Agent count、Headless 0/4/7 终态退出、TUI 截断、HTTP cursor、空页和零事件续传语义；前端测试同时固定终态徽标、opaque cursor 追加与 bearer 不进入 URL。

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
