# CyberAgent Workbench V2 任务书

更新时间：2026-07-19

## 目标

在现有 Go 项目上构建可恢复、可审计、可审批、可扩展的通用 AI Agent Workbench。借鉴成熟 Agent 产品的运行体验与公开架构思想，但保持原创实现、Go 单一控制平面和安全优先策略。

CTF 专用求解继续排在最后。前置目标是让代码、审查、学习、脚本和安全分析任务共享同一套 Run、Coordinator、Tool、Memory、Finding 和 Report 基础设施。

## 当前基线

- 架构完成度：约 99%；其中 V2 Run-centric 控制平面约 99%。
- 产品可用度：完整 Code + Cyber 产品约 93-95%。
- 通用 Coding Agent 工作流可用度：约 93%。
- Cyber 自动化工作流可用度：约 20%。
- 以上是依据可验证任务切片给出的工程估算，不是性能基准；从 schema v49 起停用单一“完整产品愿景”百分比。

V2 的 99% 在 P0/P1 基础上完成了可恢复 Supervisor、预算、严格生命周期、Run-bound Session、Provider typed outcome/retry/SSE、有界 `model.delta`、active-call 查询/取消/订阅、Bubble Tea 实时取消、一次可跨重启恢复的协议修复、schema v16 有界结构化记忆工具循环、schema v17 跨进程 Run execution lease/心跳/fencing，以及 schema v18 独立 capability 的跨进程 root 活动模型取消。P3 已加入 schema v9 Work Board、schema v10 Notes、事务关系/事件、`todo`/`note` CLI、可见性规则、8192-token Context Section 选择和 `model.started` 来源审计。P4 已加入 schema v19 单 root Coordinator、schema v20 摘要幂等 inbox、schema v21 默认关闭的 internal-only Specialist admission、schema v22 same-Run Agent-owned WorkItem/Note、schema v23 attempt-bound CompletionReport、schema v24 lease-fenced Specialist Attempt 调度/usage/崩溃恢复、schema v25 root inbox 两阶段 exactly-once context、schema v26 仅内部显式调用的 no-tool Specialist model turn 与原子模型账本、schema v27 可恢复 parent instruction/child-owned memory context、schema v28 child exactly-once lifecycle repair、schema v29 durable schedule summary/跨进程 child-call cancellation、schema v30 review-gated `specialist_delegation.v1` proposal、schema v31 独立且不授权执行的 operator review fact、schema v32 可恢复 operator application、schema v33 immutable read-only Fan-out plan 与 schema v34 bounded read-only execution，以及最多两个核心 child 的 Go-internal scheduler。核心委派不超过两个 child；Fan-out 可按 1/2/4/6 档运行无工具 JSON worker，共享 root+Specialist+Fan-out 总预算和取消扇出，但不创建 Agent、Attempt、schedule，也不具备写入、Shell、进程、网络或再委派权限。P5 已统一工作区读取、Shell、FileEdit 与 workspace-scoped `script_process.v1` 提案入口，并新增 schema v11 持久化幂等审批账本、schema v12 可撤销 Session Grant 与原子工具预算、schema v13 独立脚本进程提案、schema v14 脱敏且来源绑定的 Run 输出 Artifact、schema v15 create-only 结构化工具与幂等账本、schema v16 可恢复 Provider 工具批次，以及 schema v30 独立 `agent_proposal` 工具类。

P8 已推进到 schema v37：v35 将完成的 Fan-out execution 确定性投影为通用 `draft` Finding、不可变 `model_assertion` Evidence 和 `finding_report.v1` Report；v36 增加同 Run 冻结 Artifact Evidence、一次性 operator `validated/rejected` 决定和复核命令；v37 再以独立事实完成 `validated -> accepted -> fixed`，要求接受后新建且不可复用的 remediation Artifact Evidence。SARIF 只输出 `validated/accepted` 未解决项，默认 validated/high CI 门禁同样阻断二者，fixed/rejected 不再阻断。验证、接受和修复始终是三个不同阶段。

金额预算、HTTP 或模型自主 child 调度和真实 Sandbox 进程执行尚未实现；schema v48 的严格 Sandbox Manifest、schema v49 的精确审批/重新提交/禁用候选、schema v50 的禁用态 Artifact 绑定、独立 fencing、取消与清理恢复、schema v51 的禁用态后端/输出预检、schema v52 的仅模拟后端证据与内存输出事务、schema v53 的固定本机端点只读 Docker 观测、schema v54 的确定性容器计划与假写事务、schema v55 默认关闭的 Docker 创建/核验/删除演练、schema v56 的可恢复预写意图、代际租约和 stage/cleanup 检查点、schema v57 的 descriptor-pinned、kernel-sealed 宿主输入捕获证据、schema v58 的 daemon stage 前持久化捕获要求、schema v59 的 daemon-owned/readback-verified/fully-cleaned 输入交接、schema v60 的严格 runtime-input projection plan、schema v61 的可恢复只读卷应用与 never-started target，以及 schema v62 的保留资源检查与可恢复精确清理已经落地。v55-v62 仍不启动容器进程。operator-only 显式 child schedule/continue、no-tool child turn、最多两个 child 的有界并发、一次 child repair、Coordinator、Run 工具预算、跨进程执行互斥，以及 root/child 精确跨进程主动取消均已落地。

P7 已推进到 schema v71 与非 schema D1-B1，P9/Desktop 产品面已推进到 schema v78 与 D1-G2/V1/F2，通用运行时安全面已推进到 schema v79：除既有 Run/Session/Plan/审批、Workspace/证据/回执/行动中心外，FileEdit 支持安全恢复和多文件独立审阅，Windows Credential Manager 可触发 generation-safe Provider Registry 原子 reload，普通浏览器与 Desktop 共享 metadata-only capability/worker health，并新增 Go-owned 只读仓库状态/脱敏 Diff、不可变操作者验证、可恢复 Code Handoff 与 Code Journey。v79 再增加 Tool 硬超时/特殊文件拒绝、有界同步等待图与可恢复 Run 无进展熔断。各项由独立 capability 或 read-only 契约控制；renderer 不提交 host path、不能回读密钥，worker 不持有 Tool Runner，也不授予通用 Shell、LocalRunner、Docker、Git hook、安装 hook 或子 Agent 能力。

schema v64 已增加 Go-owned `run_execution_profile.v1`：每个 Run 默认 `preview`，操作者可在 `created` 或无活动 lease 的 `paused` 状态选择 `preview|docker|local`。CLI、HTTP/OpenAPI 与 React 使用同一状态机；所有档位仍固定零进程、零执行授权和零 capability。

schema v65 已增加不可变 `sandbox_docker_production_evidence.v1`：Go 固定 16 项机器 probe 和摘要协议，CLI 只接受同一操作者的 v63 阻塞审查、稳定操作键和显式确认。schema v66 再增加 collector 调用前持久化的 attempt、摘要化 operation、过期 generation lease、当前代 quiescent reconciliation、类型化 failure 和原子 result。schema v67 只在 Linux 显式 opt-in 后执行五次固定 GET，schema v68 再增加一次不可变操作员接纳/拒绝决定。所有 start/process/export/Artifact authority 继续为 false。schema v69-v78 与 Desktop D1-A 至 D1-G2/V1/F2 已完成外部 Skill 安全链、桌面恢复、日常 Run 控制、模型/Plan/审批、FileEdit 提案/审阅/apply/恢复/多文件汇总、Provider generation reload、持久化 wake/worker health、惰性 Skill、Workspace evidence、恢复回执、行动中心、键盘导航、只读 Repository/脱敏 Diff、不可变操作者验证、Code Handoff 与 Code Journey。schema v79 完成持久化无进展 guard；SQLite 当前为 v79。

## 执行原则

- 每个阶段必须形成可运行的纵向切片，不做一次性大重写。
- Go 是唯一主控；TypeScript 和 Rust 不得绕过 Go。
- 先单 Agent 恢复，再开放多 Agent 并发。
- 先审计和审批，再启用真实执行。
- SQLite 是状态真源；导出的 JSON/Markdown/SARIF 和 CI 判定只是投影。
- 每三个聚焦切片组成一个交付批次；第三片后统一执行功能复核、普通测试、组合差异审查和文档更新。
- 每两个批次即六个切片执行一次完整健壮性门：全仓 race、vet、staticcheck、govulncheck、依赖/隐私检查与完整功能构建；期间仍需运行受影响包的聚焦回归。
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
- [x] 非 schema 的 ADR 0025 受保护删除守卫在审批前永久拒绝递归、绝对/越界/通配、环境变量/命令替换目标及常见 PowerShell/`cmd`/Python/Node 删除形式；Shell、ScriptProcess、Sandbox 共用 Go Policy，非执行证据不扩权，真实进程仍保持关闭。

验收标准：没有工具可以绕过 Scope、Policy、Approval 和 Event Store；旁路执行扫描为零。

## P6：真实 Sandbox

状态：进行中；schema v48-v63 已完成从 Manifest 到阻塞态 Docker start-gate 设计审查，schema v64 只增加非授权 backend 档位选择，schema v65 增加非授权生产证据账本，schema v66 增加 collector 前写入和可恢复 ownership，schema v67 增加显式 opt-in、固定五次 GET 的 Linux 只读 daemon harness，schema v68 增加不授权执行的 receipt 接纳/拒绝账本；daemon 写入、容器启动和进程执行仍未开放

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
- [x] v59 发布门禁通过全仓普通/race、vet/staticcheck/module/govulncheck、严格 TypeScript、17 项前端测试、OpenAPI/构建/npm audit、仓库扫描、Linux 交叉编译与隔离二进制 smoke；GitHub Actions run `29406403201` 已通过功能提交 `fb1daca`，Go/Linux 2 分 37 秒、TypeScript 28 秒。
- [x] schema v60 单独实现并审计 verified bundle 到 Manifest target 的 runtime input projection plan：显式确认、完整 v48-v59 复核、精确 bundle recapture、逐字节 canonical PAX tar、目录 root/fixed Artifact 映射、handoff-bound 跨 Run 隔离、不可变 aggregate completion、幂等 CLI plan/list/show 与 metadata-only 隐私均已覆盖；状态固定 `compiled_not_applied`，没有 daemon/start/exec/export 权限。
- [x] v60 发布门禁通过全仓普通/race（198.9 秒/194.0 秒）、vet/staticcheck/module/govulncheck、严格 TypeScript、17 项前端测试、OpenAPI/构建/npm audit、仓库扫描、Linux 交叉编译与隔离二进制 smoke；高频编译器/Store/Application/CLI/race 回归通过，审计修复八项持久化、隔离、canonical、ordinal 与时间线问题，未发现未解决高/中风险。GitHub Actions run `29428011306` 已通过功能提交 `cc92421`。
- [x] schema v61 增加独立写前 intent、generation fencing 和固定 transport，把 v60 projection archive 应用到 handoff-bound local volumes，全部只读/no-copy 附加并在 never-started 状态精确 inspect；双确认、完整 v48-v60 复核、daemon 回读、foreign collision、exact cleanup、generation takeover、重放、迁移、SQL 不可变与 metadata-only CLI 已覆盖，决策见 ADR 0021。
- [x] v61 本地发布门禁通过全仓普通/race（197.5 秒/316.8 秒）、vet/staticcheck/module/govulncheck、严格 TypeScript、17 项前端测试、OpenAPI/构建/npm audit、仓库扫描、Linux 交叉编译、隔离二进制 smoke 与 Sandbox race 20 轮；审计补强回读上限、租约时序、resume 确认、取消恢复、transport 能力收窄、真实 mount 证据和 digest 语法，未发现未解决高/中风险。GitHub Actions run `29437941378` 已通过功能提交 `f4aaf7a`；Linux real-daemon v59/v61 harness 仍待人工环境执行。
- [x] schema v62 为 v61 保留的 target/volumes 增加显式 metadata-only inspect 与 exact-owned cleanup/reconciliation 命令。检查不重捕获输入；完整 read-only/`NoCopy` 证明只在 target 与全部卷精确存在时成立。清理要求独立双确认、写前 intent、generation lease、全资源预检、foreign collision 零 DELETE、target 先删、最终全缺失复核，失败可释放并接管；迁移不虚构历史事实，决策见 ADR 0022。
- [x] v62 定向 Sandbox/Store/Application/CLI/迁移/SQL/隐私/平台能力测试通过；审计同时固定 lease 不可删除和终态时间必须位于当前活跃租约窗。Linux opt-in harness 已延伸到 v62 inspection/cleanup，但本机 Windows 无 Docker，真实执行仍待 Linux 人工环境。
- [x] v62 最终发布门禁通过全仓普通/race（313.6 秒/329.6 秒）、vet/staticcheck/module/govulncheck、严格 TypeScript、17 项前端测试、OpenAPI/构建/npm audit、仓库扫描、Linux 交叉编译、隔离二进制 smoke 与四层高频回归；GitHub Actions run `29444398815` 已通过功能提交 `d250d32`，未发现未解决高/中风险。
- [x] schema v63 已完成 design-only start-gate review：16 项 v51 检查固定映射为未验证阻塞项，11 条未来 per-Run start/wait/TERM/KILL/orphan 转换固定为未实现、未授权；结果只能是 `blocked/deny_start`，且无 daemon/input/process/output/Artifact 能力。决策见 ADR 0023。
- [x] v63 定向 Sandbox/Store/Application/CLI/迁移/并发/SQL/隐私测试通过；幂等重放、跨 Store 收敛、迁移不伪造历史、全链复核和 false authority 投影均有覆盖。
- [x] v63 最终发布门禁通过：全仓普通/race（196.9 秒/212.3 秒）、vet/staticcheck/module/govulncheck、TypeScript/OpenAPI/build/npm audit、仓库扫描、Linux 交叉编译、隔离 schema-v63 二进制 smoke 与 Sandbox/Store/Application/CLI 20/15/10/10 轮回归；未发现未解决高/中风险。GitHub Actions run `29503856229` 已通过提交 `e25a2ab`。
- [x] schema v64 建立不可变 Run 执行环境档位：`preview|docker|local` 只记录操作者意图，活动 lease/Run 状态、摘要幂等、SQLite 不可变与三项 false authority 均已固定；Docker/Local 门禁仍未满足。
- [x] v64 最终门禁通过全仓普通/race、定向 profile race、vet/staticcheck/module/govulncheck、TypeScript/21 项 Vitest/build/npm audit、生成契约确定性、README/隐私/产物/编码/diff 扫描和隔离 CLI smoke；审计修复误导错误文案、静态错误样式与浏览器 DTO 过度字段，未发现未解决高/中风险。
- [x] schema v65 建立机器生成、非授权的生产证据捕获账本：固定 suite/environment/check digest、16 项 probe、同一 v63 操作者、不可变聚合/item/operation/event、32 条 Run 上限和幂等重放；不接受手写结论或原始 daemon/resource/path 数据。v65 交付时 collector 只落 unsupported/opt-in/harness-pending receipt，并由 Application 拒绝 complete/real-daemon 结果。
- [x] schema v66 在 collector 调用前建立 durable write-ahead evidence-capture attempt、摘要化 operation、过期 generation lease、固定 Linux endpoint class、30 秒上限、typed failure、当前 generation quiescent reconciliation 与原子 result；释放/过期恢复进入 N+1，stale worker 不能提交。SQL 拒绝无 v66 result 的新 v65 evidence operation，历史 v65 receipt 不回填虚构 attempt。
- [x] v66 CLI 增加 production-evidence attempt list/show/resume，恢复要求新的 `--confirm-machine-capture` 且不暴露 lease identity；定向测试覆盖 collector 内可见写前顺序、活动冲突、释放/过期接管、旧代 fencing、unsafe daemon-contact 失败、generation-two completion、SQL 旁路、迁移、不可变、隐私和不重新采集的重放。当前 checkpoint 只记录零 daemon read/resource，不是生产资源核验；ADR 0028 固化边界。
- [x] v66 最终门禁通过全仓普通/race、vet/staticcheck/module/govulncheck、21 项前端测试、OpenAPI/build/npm audit、仓库隐私/链接/diff 扫描、高频回归和隔离 schema-v66 二进制 smoke；审计补上 lease DELETE 防护、release/takeover 时序约束与 capture list 尾随 `--limit` 兼容性，未发现未解决高/中风险。
- [x] schema v67 在 v66 写前门禁后实现 Linux 只读 harness：不可变 intent 先于 daemon contact；固定本机 Unix endpoint，先按精确 attempt label GET 容器清单并要求 empty scope，再执行 `_ping/version/info/exact-image-inspect`；总共五次 GET、逐调用四秒、整体 30 秒，只接受已存在精确 digest，不 pull，transport 无 mutation 方法。
- [x] v67 daemon-aware reconciliation 绑定当前 generation 与 v66 control checkpoint；释放/过期恢复必须在 N+1 重做检查，持久 intent 不能降级到 v66 inert result。Go/SQL 固定 16 项 `observed_failed`、`production_verified_count=0` 和零 start/process/output/Artifact authority；迁移不虚构 v67 状态，CLI/events 不保存 socket、payload、resource ID、路径或私有 lease。决策见 ADR 0029。
- [x] v67 最终门禁通过全仓普通/race（215.2 秒/233.1 秒）、vet/staticcheck/module/govulncheck、21 项前端测试、OpenAPI/build/npm audit、51 份 Markdown、仓库扫描、隔离二进制 smoke、Linux 交叉编译及 Sandbox/Store/Application/CLI 高频回归；审计关闭零验证、selector、control reconciliation、租约时序、direct-SQL 半终态与 contact 文案问题，未发现未解决高/中风险。实现提交 `8bc0929` 的 GitHub Actions run `29543385038` 已全绿（Go/Linux 2 分 50 秒，TypeScript 24 秒）。
- [x] schema v68 增加独立 operator evidence acceptance/rejection 账本；只接受精确完成的 v67 harness receipt、显式确认、固定 decision/reason，operation/review 原子成对且不可变，迁移不伪造历史。同键同语义重放不追加事件或 daemon 调用，改意图冲突。
- [x] v68 即使 `accepted` 也固定零生产验证、零 sufficient、16 个 blocker，以及 false start/container/process/output/Artifact authority；请求、表、事件和 CLI 不含自由文本、daemon payload、socket、路径、资源身份、raw key 或私有 lease。决策见 ADR 0030。
- [x] v68 最终门禁通过全仓普通/race（247.9 秒/276.3 秒）、vet/staticcheck/module/govulncheck、21 项前端测试、OpenAPI/build/npm audit、57 份 Markdown/74 条相对链接、仓库隐私/编码/禁止执行入口/diff 扫描、Linux 交叉编译、隔离真实 CLI smoke 与四层高频回归。审计修复 request-fingerprint 双层绑定、SQL 负向矩阵、双 Store 收敛和 rejected 全链覆盖，未发现未解决高/中风险。GitHub Actions run `29552080990` 已通过实现提交 `41583ac`（Go/Linux 2 分 57 秒，TypeScript 24 秒）。
- [ ] 本地代码默认只读挂载，输出目录独立可写。
- [ ] 网络默认关闭，后续仅允许显式 allowlist。
- [ ] 支持真实执行、stdin、超时/kill、日志和原子 Output Artifact 导出；v51 只固定要求，v52 只验证输出假事务，v53 只读取元数据，v54 只编译并假写，v55-v56 只操作未启动容器，v57-v58 只固定本地输入，v59 只完成 never-started daemon handoff，v60 只编译 projection，v61 只应用只读卷并保留未启动 target，v62 只检查/清理资源，均未启用进程执行或生产 Artifact。
- [ ] 将 v50 幂等清理扩展为真实运行中容器 orphan 检测/回收，并用独立生产证据逐项验证 v51 检查；v52 的 `simulated_pass`、v53 的 `production_observed`、v54/v60 的 `compiled_not_applied`、v55-v56 的非启动 inspect/recovery、v57-v58 的本地捕获事实、v59 的 handoff、v61 的 never-started target 与 v62 的资源清理都不计入进程隔离验证。
- [x] 保留 Noop/Local 作为测试与开发接口；Local 当前明确禁用，不能作为旁路执行后端。

验收标准：容器内不能越界读取宿主目录；取消能终止进程；重启后能识别并处理残留 Sandbox。

## P7：Skills 与 Profiles

状态：进行中；只读 `skill.v1` Registry、schema v39-v47 内置 Skill/模式/Plan/来源/交付/队列链、非 schema 的 `skill_package.v1` 严格校验、schema v69 内容寻址惰性用户 Registry、schema v70 外部 Skill 的不可变 Run 选择与最小上下文、schema v71 HTTP/TUI/Web metadata-only 来源投影、Desktop D1-A 路径隔离 Go 预览桥、D0-A/D0-B 原生预览与恢复加固，以及 D1-B1 HTTP/Desktop 显式确认的惰性安装均已完成；签名、远程分发和安装时执行继续后置，计划见 `docs/DESKTOP_PLAN.md`

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
- [x] ADR 0024 固定 `skill_package.v1` 与威胁模型：第一版只允许按顺序包含 `manifest.json` + UTF-8 `SKILL.md` 的确定性 ZIP，拒绝路径歧义、链接、特殊文件、重复/大小写碰撞、ZIP bomb、尾随数据、脚本和安装钩子。
- [x] 实现纯 Go 包 parser/validator/fuzzer，以及 metadata-only `skill package validate`；校验阶段不写磁盘、不创建数据库、不联网、不调用模型或工具，并保持全部能力/安装授权位为 false。
- [x] 包校验发布门禁通过最终全仓普通/race、vet/staticcheck/module/govulncheck、约 2645 万次 fuzz、78.5% Skills 覆盖、parser/CLI 高频回归、TypeScript/OpenAPI/build/npm audit 和仓库扫描；审计修复 creator version、Deflate 隐藏尾载荷、弃用测试 API 与错误路径回显，未发现未解决高/中风险。GitHub Actions run `29512332025` 已通过提交 `55b3fae`。
- [x] schema v69 实现 content-addressed 用户 Skill Registry、不可变安装/移除账本、写前意图、原子对象发布、崩溃恢复，以及 `skill import/installed/remove`；同名同版本冲突，已被 Run 固定的版本不可移除，对象与历史仍保留。
- [x] 外部包固定标记为 `operator_installed_untrusted` 并要求显式安装确认；导入阶段命令、钩子、网络、Provider、工具授予、Run 选择和上下文注入全部为 false。
- [x] 自定义 Code/Cyber Catalog 严格分离；Cyber 第一版仅接收精确 `script` Profile，不继承 Code/Review/Learn Catalog，用户包不能覆盖内置名。
- [x] v69 最终门禁通过全仓普通/race（259.7 秒/275.3 秒）、vet/staticcheck/module/govulncheck、21 项前端测试、OpenAPI/build/npm audit、定向三轮 race，以及真实双 Service 独立候选身份 20 轮普通/10 轮 race 收敛。首次 Linux CI run `29556933994` 暴露并发嵌套目录准备失败；改为逐级创建与非 symlink 目录身份复核后，12 Store 回归通过 100 轮普通/20 轮 race，修复提交 `d28b100` 的 run `29557803407` 已全绿。其余审计修复旧 schema 夹具移除顺序、对象收据绑定、发布前取消点、生成 ID/时间误判冲突、Manifest description 凭据脱敏和低风险静态问题，未发现未解决高/中风险；边界见 ADR 0031。
- [x] schema v70 将外部包纳入 Run 的不可变精确版本选择；只允许未 tombstone 且安装结果/对象身份精确绑定的版本，使用第二次显式上下文确认，声明工具不授予能力。
- [x] 以独立 hash/Profile/预算/脱敏/来源账本向 root/Specialist 最小化交付外部 Skill，并保持 Code/Cyber 隔离、最多一个操作者指定的 Specialist、first-model-call 原子绑定和跨重启恢复；正文只在用户角色 Provider request 内存中存在。
- [x] v70 聚焦门禁覆盖 operation 重放、对象漂移、secret redaction、Prompt Injection 用户角色隔离、无工具 Specialist、SQL 不可变、Go/SQL 删除固定保护和 v69 原地升级不伪造状态；边界见 ADR 0032。
- [x] schema v71 向 HTTP/OpenAPI、TUI 和 React 增加 bounded metadata-only 外部 Skill 选择/来源投影；不公开正文、路径、摘要、请求者或操作身份，不增加浏览器写权限。
- [x] Desktop D1-A 增加路径隔离 Go 预览桥：原生 picker 只把路径交给 Go closure；严格校验后仅发放最多 16 个、五分钟过期、单次消费的 256-bit 句柄，renderer DTO 不含路径/正文/description/content path/content digest，且全部安装/执行/联网/模型/工具授权为 false；D0-A 已接通可见入口，D0-B 已加固生命周期/事件恢复，边界见 ADR 0033/0034/0035。
- [x] Desktop D1-B1 在原生预览之后增加 Go-owned 确认安装 mutation；TypeScript 只提交一次性确认句柄，不提交路径或 bytes。HTTP 使用独立 capability、control token、严格 Host/Origin、规范 base64、大小和幂等门禁；两端只写惰性 Registry，不执行包内容或授予 Run/工具权限，边界见 ADR 0041。
- [ ] 签名包、团队 Catalog、URL/Git 安装和 Marketplace 后置；签名只证明来源/完整性，不授予 Policy、Tool 或 Sandbox 权限。
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

状态：部分完成；loopback-only `api.v1`、root/Specialist 取消、Run 档位/创建/Session/生命周期/有界交接、模型诊断/路由/系统凭证、Plan/Deliver、审批、wake 意图/前台消费/有界 worker、FileEdit 提案/恢复/独立审阅/apply/多文件汇总、惰性 Skill、回执、Workspace explorer/search/Repository、非授权证据、行动中心/快捷命令、Code Journey、便携诊断、OpenAPI 3.1、SSE/poll、Headless、TUI、React/Vite 和 Windows Wails 壳已落地；有界 repository Diff、verification/handoff、xterm、Windows 10 实机矩阵和签名分发尚待完成，计划见 `docs/DESKTOP_PLAN.md`。

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
- [x] 提供经过单独审计的主动取消入口；独立 control token 不能读取，read token 不能取消，客户端不能提供 fencing token。schema v64 复用该 capability 增加仅记录操作者意图的档位选择；schema v72 再增加独立可开关、registered-Workspace、default-budget、network-disabled 的幂等 Run 创建。两者都不启动进程、不授予 backend 或 capability；WebSocket 只在未来确有双向或模型正文需求时再引入。
- [x] 从 Go read DTO 生成确定性 OpenAPI 3.1/JSON Schema，提供鉴权端点、CLI 导出、golden 防漂移与 live-route contract tests；TypeScript 不手写安全规则。
- [x] 从 OpenAPI snapshot 生成 React/Vite DTO，提供 Run/Session 列表、详情、Work/Notes/Artifact descriptor/ToolRound、预算/租约和带 Authorization header 的可恢复 SSE 视图；read token 只驻留内存，Vite 代理只允许回环目标。
- [x] 由 Go 在显式 `--ui-dir` 配置下同源托管不可变生产 Web bundle，补齐 route-aware CSP、SPA fallback/类型/大小/软链接边界、缓存语义和真实 CLI/浏览器集成测试；API capability 不变。
- [x] 在 Go DTO/契约先行的前提下增加 Agent graph、delegation/Fan-out 与 Finding/Report 只读 Web 视图；列表有界分页，Fan-out 摘要查询不读取 raw report/digest/lease，生命周期 DTO 不公开私有 operator narrative 或 Artifact 正文。
- [x] 在 Run detail、TUI 与 React 增加 schema v42 Plan/Delivery 只读投影；只显示有界方向/模块、选择与 WorkItem 映射，不公开 operation/lease/requester 内部身份，也不提供选择或阶段切换控件。
- [x] 扩展 CLI/TUI/HTTP/Web/Headless 跨入口契约矩阵：固定 `running/paused/completed/failed/cancelled`、终态退出码与事件尾；以 53 条真实 Run/Session 验证 TUI 50 条截断和 HTTP 20/20/13 opaque-cursor 分页，并固定空集合及 Headless 从尾序号零事件续传。
- [x] schema v64 在 Run detail、OpenAPI 与 React 增加 `preview|docker|local` 分段控件；control token 只驻留内存，TypeScript 只提交 profile ID，Go/SQLite 固定 backend/scope/gate 与全部 false authority，活动 lease 和非 created/paused 状态均拒绝。
- [x] Desktop D1-A Go 边界：原生 selector 与 renderer bridge 分离，后者没有接受路径/文件字节的方法；D0-A 已通过原生 `.zip` 对话框和一次性句柄接入 React/桌面可执行文件。该里程碑当时不提供安装 mutation，后续 D1-B1 已以独立能力补齐惰性安装。
- [x] Desktop D0-A：固定 Wails v2.13.0，建立 `cmd/cyberagent-desktop` Windows build-tag 边界，嵌入 React production bundle，并以无 TCP listener 的进程内 Handler 复用 Go HTTP/OpenAPI；默认只读，显式 flag 也只开放 v64 非授权档位。
- [x] Desktop D0-A 原生安全门：完整 renderer binding 只有 bootstrap/选择/预览三项，TS 不能提交路径或 bytes；内存 token、单实例、CSP、renderer code integrity、WebView file/drop 与 context-menu denial、path-free startup error 和原生 `.zip` 预览通过测试/实机复核，边界见 ADR 0034。
- [x] Desktop D0-A 发布门：全仓普通/race 205.1 秒/293.9 秒，ordinary/desktop-tag vet、staticcheck、零漏洞 govulncheck，31 项前端测试、build/npm audit、61 份 Markdown/82 条链接及隐私扫描通过；Desktop 聚焦 50 轮普通/10 轮 race，最终未签名 GUI SHA-256 为 `6b355cfa72b41d225e62ed58ac24cb9493bbf2a71f4d45120e6f0dbf5308ad0c`。GitHub Actions run `29602281365` 已通过实现提交 `2c0b81c`，Go/Linux、TypeScript、Windows Desktop 分别用时 4 分 57 秒、26 秒、4 分 27 秒。
- [x] Desktop D0-B：完成 CLI/Desktop 同库并发、关闭/崩溃重开、六路并发打开、单实例恢复、poll/SSE 高水位 cursor 互续、WebView2 缺失/过旧/探测失败诊断、secure build tags 和 production navigation/binding-origin 自动化；Windows 11 实机强制结束/重开与第二实例通过，继续只读且不做安装器、注册表、自启动、更新或后台服务。边界见 ADR 0035。
- [x] Desktop D0-B 发布门：最终全仓普通/race 256.6 秒/273.5 秒，ordinary/secure-Desktop vet、零告警 staticcheck、双路径零漏洞 govulncheck、37 项前端测试、production build/npm audit 与最终实机烟测通过；扫描中发现的五项 `x/net/html@v0.54.0` 可达通告已通过升级 `x/net@v0.55.0` 修复。最终未签名 GUI 为 19,572,224 字节，SHA-256 `f26ea87f42701a7eba8efa789900ea6953ef3c1533ff95106ec4b8e6b02b1160`。GitHub Actions run `29609621468` 已通过实现提交 `c9b1c66`，Go/Linux、Windows Desktop、TypeScript 分别用时 5 分、4 分 21 秒、23 秒。
- [ ] 在正式便携/签名发行前完成 Windows 10 x64 实机启动、第二实例、强制结束/重开和 WebView2 缺失路径矩阵。
- [x] Desktop D1-R1 / schema v72：增加 Go-owned Run 创建与自动 Session 绑定的窄 control route；distinct token、严格 body、摘要幂等 operation、registered Workspace/closed Scope/default budget、事务事件、前端回归和创建后选中均已完成；不调用模型、不扩展 Wails native bridge。边界见 ADR 0036。
- [x] Desktop D1-R1 发布门：全仓普通/race 271.5 秒/257.9 秒，普通/secure-Desktop 测试与静态/漏洞检查、45 项前端测试、确定性 OpenAPI、生产构建、依赖/隐私/Markdown/diff 扫描和隔离 schema-v72 smoke 全绿；审计修复迁移 trigger 拆除顺序、capability 串联、初态重放、严格 UTF-8/root/时间线/事件数、窄 Store 接口和前端 Goal/Workspace/字节绑定，未发现未解决高/中风险。
- [x] Desktop D1-S1a：增加窄 Go/Application `session_message_submission.v1`，严格绑定现有 Session/Run，复用 v45-v46 脱敏、幂等 enqueue 和事件，不创建模型/lease/tool 路径。
- [x] Desktop D1-S1b：增加独立 `SessionMessageEnabled` HTTP/Desktop capability 与 `--enable-session-messages`；严格 bearer/header/JSON/UTF-8/body/response 契约，Wails bridge 仍精确三个方法。
- [x] Desktop D1-S1c：增加 React Session composer、16 KiB UTF-8 预检、同内容不确定失败内存重试、metadata-only queue 状态和 capability/Run 状态失败关闭。
- [x] Desktop D1-S1 三切片功能门：最终代码全仓普通 Go 直接运行 255.6 秒、Desktop-tag 聚焦 80.5 秒、最终 Application/HTTP/Desktop 回归、15 个前端文件 52 项测试、严格 TypeScript、Vite 与 Windows production build 通过；未调用 Provider、工具、Shell、Docker、外部网络或 execution lease。边界见 ADR 0037。
- [x] Desktop D1-S2：独立 `session_steering_cancellation.v1` HTTP/UI 已复用 v46 完成 pending-only 取消；公开投影派生 `prepared`，prepared/committed/cancelled 状态不可改写。
- [x] Desktop D1-L1 / schema v73：digest-idempotent operator Run start/pause/resume 已完成；精确状态、quiescence、lease、Agent/Supervisor 与 capability 门独立于消息提交。
- [x] Desktop D1-X1 / schema v73：Go-owned bounded execution handoff 已完成；最多冻结八条 pending 身份并交给既有 RunSupervisor/预算/Policy/lease/model/tool/event 路径，不建立 Desktop-native 执行器。
- [x] 六切片健壮性门：全仓 ordinary/race 268.2 秒/295.3 秒、ordinary/secure-Desktop vet/staticcheck/govulncheck、module/依赖/契约/隐私检查、66 项前端测试、Windows/Vite build 和重启/并发功能测试通过；无已知未解决高/中风险，边界见 ADR 0038。
- [x] Desktop D1-M1：CLI/API/Desktop 已统一使用 Go-owned Provider Registry/持久化模型路由并投影脱敏可用性；API key、Base URL、环境变量名不进入 TypeScript、SQLite 或公开事件，读取不探测网络。
- [x] Desktop D1-P1：独立 digest-idempotent Go controls 已开放 Plan 三选一和显式 Plan-to-Deliver；模型不能代选，选择不能自动切换阶段或执行。
- [x] Desktop D1-A1：独立 capability 已投影现有审批队列并开放 approve-once/deny；永久 Policy 拒绝、文件写入、Session Grant 和 process-disabled 结果不可绕过。
- [x] D1-M1/P1/A1 三切片功能门：最终全仓 ordinary、Windows Desktop tag、73 项前端测试、strict TypeScript、OpenAPI、Vite/Windows production build 与 npm audit 通过；组合审计无已知高/中风险，边界见 ADR 0039。
- [x] Desktop D1-M2：显式 content-free Provider 诊断与 persist-before-memory 模型路由已完成；每次诊断至多一次有界请求，公开结果无模型正文、密钥、端点、环境变量名或原始错误。
- [x] Desktop D1-D1：exact-bound metadata-only FileEdit 队列、脱敏 Diff 与 approve-intent/deny 已完成；批准不写文件，真实 apply 仍为独立能力。
- [x] Desktop D1-Q1 / schema v74：有界 wake/retry 意图、取消、deadline/backoff 与单 owner generation fencing 已完成；没有后台 loop、模型、工具、Run lease 或自动执行。
- [x] Desktop D1-Q2 / schema v75：显式前台 wake consumer 已通过既有 RunSupervisor/handoff/预算/Policy/取消/fencing 消费一条到期 intent；不建立隐藏 worker，未知 in-flight handoff 保持 prepared，过期不重领且结算前不可取消，失败调用事实绑定持久化 handoff 结果。
- [x] Desktop D1-D2 / schema v76：已批准 Diff 的独立 apply 已完成；每个 Edit 只有一个 operation，Go 在原子替换前复核 exact Run/active Session/Workspace/Approval/Policy/当前哈希，HTTP/React 不提交路径或正文，重放不二次写入。
- [x] Desktop D1-B1：第四个窄 Wails 方法与独立 HTTP control 已完成确认式惰性 Skill 安装；不执行正文/脚本/钩子/命令/网络/Provider/工具，不自动选择到 Run。
- [x] Desktop D1-U1：`operation_receipt.v1` 已统一 FileEdit apply、前台 wake consume 与惰性 Skill install 的持久化结果/重放/闭集恢复呈现；FileEdit 仅对 exact-dir/old/regular/hash-matched 内部暂存执行保守清理，React 不推导状态。
- [x] Desktop D1-E1：Go-owned `workspace_explorer.v1` 与 Files 页已完成 canonical 相对路径、link/redirect 拒绝、400/200 目录界限、64 KiB UTF-8/128 KiB 脱敏投影、root/staging 隐私和 `instruction_authorized=false` 来源。
- [x] Desktop D1-W1：只读 portable doctor、可复现 linker metadata、仓库内无 child-reparse 输出、Windows 双构建 SHA-256、PE/零 COFF timestamp/trimpath/module/非安装检查已完成；人工 Windows 10/WebView2 矩阵仍使 release-ready 为 false。
- [x] D1-U1/E1/W1 后累计六片完整健壮性门已通过：ordinary/race 294.0/338.3 秒、普通/secure-Desktop tests/vet、staticcheck、govulncheck、module/依赖/隐私、88 项 React、确定性契约、Vite 与真实 Windows 双构建均为绿色；无已知未解决高/中风险，边界见 ADR 0042。
- [x] GitHub Actions run `29658783000` 已通过实现提交 `5f0f397`：Go control plane 5 分 49 秒、TypeScript console 32 秒、Windows Desktop shell 2 分 11 秒。
- [x] Desktop D1-E2：完成 Go-owned 有界 Workspace filename/redacted-text 搜索；128 directory、1000 entry、64 file、50 result 上限，不建立 indexer 或 renderer host path。
- [x] Desktop D1-C1 / schema v77：完成操作者显式 non-authorizing evidence 附加；精确 Run/Session/Workspace/hash 绑定、原子 message/event/attachment、默认关闭独立 capability，文档文本不能自授权。
- [x] Desktop D1-U2：完成可刷新 metadata-only receipt history；最多 100 条、可按 Run 过滤、公开 opaque ID，暂存状态只读检查且不返回 operation/path/private lease。
- [x] Desktop D1-E2/C1/U2 三切片普通功能门：全仓 Go 297.9 秒、Desktop tag、92 项 React、strict TypeScript、vet/module、确定性契约、Vite/Windows 可复现构建和 npm 零漏洞均通过；无已知未解决高/中风险。
- [x] GitHub Actions run `29661764283` 已通过实现提交 `ffbdc72`：TypeScript 34 秒、Windows Desktop 2 分 21 秒、Go/govulncheck 3 分 48 秒。
- [x] Desktop D1-O1：完成 Go-owned `operator_action_center.v1`；最多 100 条闭集 pending steering/approval/FileEdit/wake metadata，精确 Run/Mission/Session/Workspace 复核，不返回正文、命令、路径、Diff、私有 operation/lease，也不自动处理行动。
- [x] Desktop D1-C2：完成 `session_evidence_inventory.v1`；只列 exact Run/Session 已附加证据的 canonical source/hash/time 与固定 false authority，不返回 message/body/requester/private identity。
- [x] Desktop D1-K1：完成静态闭集 `Ctrl+K` 命令面板；只能导航既有 Run 页或刷新当前查询，不提交路径、正文、审批、operation、capability 或进程请求。
- [x] D1-O1/C2/K1 后累计六片完整健壮性门：ordinary/race 319.6/299.8 秒、普通/secure-Desktop test/vet、staticcheck、govulncheck、module/依赖/隐私、97 项 React、确定性契约、Vite 与真实 Windows 可复现构建均为绿色；真实浏览器审计修复 canonical `v1` 事件版本漂移和失败重连连接泄漏，无已知未解决高/中风险，边界见 ADR 0044。
- [x] GitHub Actions run `29665187925` 已通过实现提交 `1151aaf`：TypeScript 36 秒、Windows Desktop 2 分 23 秒、Go control plane 3 分 35 秒。
- [x] Desktop D1-I1：完成 `file_edit_proposal.v1`、Go-issued 五分钟单意图 source handle、本地 lazy Monaco/Diff 与 pending-only proposal；不提交 host path，不直接写文件。
- [x] Desktop D1-M3：完成 `provider_credential.v1` 与 Windows Credential Manager；只返回配置状态，2,560-byte 上限，非 Windows 无明文回退，环境变量优先，修改后当前要求重启。
- [x] Desktop D1-J1：完成 default-off `run_wake_worker.v1`；control token + startup flag、一个 serial owner、每轮一条 due intent/一步 Supervisor，无 Tool Runner/Shell/Local/Docker。
- [x] D1-I1/M3/J1 三切片普通功能门：Go 327.6 秒、vet、secure Desktop tag、28 文件 102 项 React、strict TypeScript、确定性 API、Vite、npm 零漏洞和可复现 Windows 双构建均通过；SHA-256 `a0e6aa0a3d15ccc39712f8a0a64d7de06e4a6af426e060b6378b1011c93a1cf6`。审计固定不确定 FileEdit 保存的 ID/单意图、审批后重试冲突和 Provider 空模型数组契约；桌面/移动 UI 冒烟通过，边界见 ADR 0045。
- [x] GitHub Actions run `29671519260` 通过实现提交 `ee36405`：TypeScript 42 秒、Windows Desktop 2 分 31 秒、Go control plane 3 分 54 秒。
- [x] Desktop D1-I2：过期 source handle 仅在旧 SHA-256 仍匹配时换发；durable pending proposal 只恢复为无句柄、`editable=false` 的 Diff，stale/missing 不自动 rebase。
- [x] Desktop D1-M4：系统凭证变更构建完整候选 Registry 并原子切换 generation；失败保留旧 generation，活跃调用继续使用已捕获 Provider，成功无需重启且不回传密钥。
- [x] Desktop D1-J2：普通浏览器/Desktop 共用只读 `runtime_capabilities.v1` 与 bounded worker health/drain；不返回 token/owner/lease/Run/private error，不能运行时启用或安装服务。
- [x] D1-I2/M4/J2 后累计六片完整健壮性门：ordinary/race 322.9/352.8 秒、vet、staticcheck、govulncheck、module/依赖、secure Desktop、29 文件 108 项 React、strict TypeScript、确定性契约、Vite/npm、真实浏览器和 Windows 可复现双构建全部通过；OpenAPI 57 path/61 operation/125 schema，GUI SHA-256 `30a3d9d19e02f32f8ea976fc071bc6942ed06fba3e7cad937310a78e46e74dfc`，无已知未解决高/中风险，边界见 ADR 0046。
- [x] GitHub Actions run `29674460349` 已通过实现提交 `7d5736e`：TypeScript 38 秒、Windows Desktop 2 分 49 秒、Go control plane 3 分 43 秒。
- [x] Desktop D1-G1：完成 `repository_state.v1`；exact Workspace root、纯 Go、50,000 metadata/10,000 status/200 output 上限，拒绝父发现、重定向 `.git` 与内部链接，不调用进程/网络/remote/hook。
- [x] Desktop D1-I3：完成 `file_edit_change_set.v1`；最多 100 个 exact-bound FileEdit，metadata-only、独立 review/apply、无 batch/atomic mutation，partial apply 可见。
- [x] Desktop D1-F1：完成 Code-only Journey；Scope/Plan/Queue/Review/Verify 仅导航既有 Go 能力，无 API client、无复合 mutation，Cyber 模式不自动继承。
- [x] D1-G1/I3/F1 后累计六片完整健壮性门：ordinary 321.7 秒、final-code race 490.4 秒、vet/staticcheck/module、0 reachable/imported-package govulncheck、114 React、strict TypeScript、确定性契约、Vite/npm、真实浏览器和 Windows 可复现双构建全部通过；OpenAPI 59/63/129，GUI SHA-256 `145757cb1a8bbafc9080fdc29f4ada69d34b850ca64f702310ea44578ca677a9`，边界见 ADR 0047。
- [x] GitHub Actions run `29678257802` 已通过实现提交 `d69a812`：TypeScript 43 秒、Go 5 分 32 秒、Windows Desktop 5 分 29 秒。
- [x] Desktop D1-G2：完成 `repository_diff.v1`；exact Workspace root、纯 Go、50 项/单项 64 KiB/总计 512 KiB、HEAD/Workspace 双侧脱敏、闭集非文本状态，不调用进程/网络/remote/hook。
- [x] Desktop D1-V1 / schema v78：完成不可变 `operator_verification_evidence.v1`；独立 capability、active Session 事务复核、`pass|fail|unknown`、最多 100 项 inventory，command/model/approval/authority 固定为 false。
- [x] Desktop D1-F2：完成 Code-only `code_handoff.v1`；持久化 Plan/Queue/ChangeSet/Verification/Actions/Reports 有界汇总，四次 event high-water 快照重试，无 private body、resume 或 composite mutation。
- [x] D1-G2/V1/F2 三切片普通功能门：uncached Go 308.1 秒、Desktop tag、vet、定向 staticcheck、module、120 React、strict TypeScript、确定性契约、Vite/npm、真实浏览器、隐私扫描与 Windows 可复现双构建全部通过；OpenAPI 62/67/143，GUI SHA-256 `2ab74a47794287bac71877172136f02631b5cc9a44febd930e8ee7b1913ba93f`，边界见 ADR 0048。
- [x] 远端 CI `29682547524` 已通过实现提交 `cff7489`：TypeScript 42 秒、Windows Desktop 2 分 34 秒、含 vet/govulncheck 的 Go 3 分 33 秒。
- [x] Runtime H1：Tool Registry 默认 15 秒、最大 5 分钟硬超时，区分取消/超时退出并恢复 panic；内置 read/list 响应 context，`max_bytes` 双层受限，只打开普通文件并拒绝 FIFO/device/socket。
- [x] Runtime H2：完成 4,096 node/8,192 edge 有界同步等待图、引用计数幂等释放、直接/间接环拒绝与 Tool/Retriever/Store/Runner 反向 Agent wait 永久拒绝；root Supervisor、Specialist parent/child 和 Tool Gateway 已接入。
- [x] Runtime H3 / schema v79：完成 `run_progress_guard.v1`；相同 `continue` 三轮或无结构化进展六轮时，在同一事务提交消息/事件/checkpoint 并暂停 Run；重放不重复消息，只有后续显式 `paused -> running` 才能重置，v78 升级不伪造记录。
- [x] D1-G2/V1/F2 + H1/H2/H3 累计六片完整健壮性门：final uncached Go 312 秒、final-code race 358 秒、Tool/等待图 20 轮、v79 Store 10 轮、普通/secure Desktop test/vet、零告警 staticcheck、module/tidy、零可达 govulncheck、120 React、strict TypeScript、确定性契约、Vite/npm、mock-only CLI、凭据/产物扫描、真实浏览器与 Windows 可复现构建全部通过；GUI SHA-256 `31e0df63d3fbbccac6728ad2322196bee55d57e775a15cc34f752c0632bdc699`，边界见 ADR 0049。
- [x] 远端 CI `29688544340` 已通过实现提交 `2012bfa`：TypeScript 42 秒、Windows Desktop 3 分 13 秒、含 vet/govulncheck 的 Go 3 分 54 秒。
- [x] Desktop D1-G3：完成纯 Go `repository_history.v1`；exact registered root、50 个 first-parent commit、64 个 local branch、1,024 reference scan，主题脱敏且不返回 author/email/body/remote/root，不调用进程/网络/hook。
- [x] Desktop D1-V2 / schema v80：完成不可变 `operator_verification_plan.v1`；1-32 项操作者 checklist，active Code Session/Workspace/event/digest 精确绑定，与结果分离，command/model/result inference/approval/authority 固定 false。
- [x] Desktop D1-F3：完成 `code_handoff_export.v1`；最多 256 KiB Markdown/JSON、source event high-water、byte count、SHA-256 与前端下载前复核，无 resume/report acceptance/apply/mutation/execution。
- [x] D1-G3/V2/F3 三切片普通功能门：uncached Go 334.6 秒，审计后 Repository/Application/Store/HTTP 聚焦回归、vet、37 文件 124 项 React、strict TypeScript、确定性契约、Vite production build 与 Chrome 插件真实复验均通过；OpenAPI 65/71/155，边界见 ADR 0050。
- [x] 远端 CI `29695882120` 已通过实现提交 `d70d96c`：TypeScript 43 秒、Windows Desktop 2 分 39 秒、含 vet/govulncheck 的 Go 3 分 56 秒。
- [ ] Desktop D1（当前产品可用度约 93-95%）：下一批建议 D1-G4 有界 exact-commit changed-file metadata、D1-V3 操作者显式 plan-item/evidence 关联及 R1 Go-owned Runner start/wait/cancel/timeout/process-tree-orphan 非产品门禁；完成后累计六片并执行完整健壮性门，仍不开放 unrestricted `os/exec`、Docker start 或 xterm 输入。
- [ ] Desktop D2（发布成熟度阶段）：发布便携 ZIP 与签名 MSIX，处理 WebView2 检测、per-user 安装、升级/降级、卸载、用户数据保留、SBOM、哈希与签名；自动更新另设门禁。
- [ ] Desktop D3：企业 MSI、Store、远程环境、自定义协议、文件关联、自启动和后台服务按需独立立项，不在基础桌面端默认启用。
- [x] Monaco 只编辑 Go 授权的有界 source，并只创建待审 FileEdit proposal。
- [ ] xterm.js 只展示未来由 Go 授权的终端会话；当前没有真实进程可连接。

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

每个三切片批次结束必须记录：

1. 本轮完成的 Task ID 和行为变化。
2. 当前 V2、v0.1 和完整产品进度。
3. 组合差异审查发现、风险等级和处置。
4. 单元测试、集成测试、功能构建和必要 smoke 结果；每六片再记录 race/vet/staticcheck/govulncheck 完整门。
5. 数据迁移、兼容性和安全边界复核。
6. 下一轮唯一推荐切片。
