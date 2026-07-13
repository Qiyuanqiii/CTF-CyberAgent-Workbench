# CyberAgent Workbench

> 本地优先、可恢复、可审计的通用 AI Agent 工作台<br>
> A local-first, resumable, and auditable workbench for general-purpose AI agents.

## 项目简介 / Project Overview

### 中文

CyberAgent Workbench 是一个由 Go 驱动的本地 AI Agent 工作台，面向代码开发、代码审查、安全学习、脚本任务和受控网络安全分析。它把模型调用、长上下文、工作区文件、策略检查、审批、执行预算和事件记录统一到一套可恢复的运行时中，让一次任务即使在程序退出后也能继续，并且每一步都可以追踪和复核。

每个用户目标会被记录为一个 `Mission`，每次可恢复的执行过程则是一个 `Run`。Go 是唯一控制平面，负责模型路由、状态机、SQLite 持久化、安全策略和工具边界；当前 TypeScript 只读控制台与未来的 Rust 确定性分析器都通过 Go 协议接入，不绕过安全控制。

项目当前优先完善通用 Agent 运行时及其受控多 Agent 内核。CTF 将作为后续 Profile 和 Skills 能力接入，而不是另建一套独立运行系统。

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

当前报告 runtime 提供不写库、不调用 Provider 的 SARIF 2.1.0、CI 门禁和 GitHub Actions annotation 投影。SARIF `results` 只包含尚未解决的人工确认项，即 `validated` 和 `accepted`；`fixed`、`draft`、`rejected` 只进入状态汇总，避免已修复或未经确认的声明成为上传告警。`cyberagentValidationStatus` 保留真实验证结论，`cyberagentFindingStatus` 表示当前生命周期。`report check` 的默认 `validated/high` 策略同时阻断 accepted 未解决项，显式 `active` 还纳入 draft，而 fixed/rejected 永不阻断。`--format github` 从同一内存态 `GateResult` 输出带文件和行号的 `notice/warning/error` workflow commands，严格转义模型生成的 `%`、CR/LF、冒号和逗号，并把其他 C0/DEL 控制字符转成可见文本；它不会输出 Artifact、Evidence note 或 operator reason，也不会改变原有退出码。

当前还提供第一版本地 HTTP 控制面。`cyberagent api serve` 只允许绑定回环地址；`CYBERAGENT_API_TOKEN` 只授权读取，默认关闭的取消入口必须另行设置不同的 `CYBERAGENT_API_CONTROL_TOKEN`。稳定的 `api.v1` envelope 和有界游标分页可读取 Run、Session、Event、WorkItem、Note、Artifact 元数据、Supervisor ToolRound 与不含 fencing token 的 execution-lease 摘要。两个 POST 只分别请求取消精确的 root 或 Specialist 活动模型调用，不能执行工具、改变 Policy、接受客户端 fencing token 或读取 Artifact 正文/checkpoint pending input。API 不提供 CORS 或 WebSocket，也不会持久化任何 API 令牌。

Go 响应 DTO 现在同时生成唯一的 OpenAPI 3.1 契约。`cyberagent api openapi` 可输出确定性的 JSON，`--output docs/openapi.json` 可更新仓库内的受测快照；运行中的服务也会在鉴权后的 `/api/v1/openapi.json` 返回同一份原始文档。契约测试会逐条请求全部公开路由并阻止 DTO、静态文档与 handler 漂移，且明确排除 Artifact 正文、checkpoint pending input、`lease_id` 和 fencing token。

只读 Run Event Stream 也已接入同一控制面。`/api/v1/runs/{run_id}/events/stream` 使用 SSE 推送 SQLite 中已脱敏的持久化事件；每帧 `id` 是与 Run 绑定的不透明 cursor，客户端可通过 `Last-Event-ID` 精确续传。连接具有批量、帧大小、总事件数、寿命、写超时和进程并发上限，心跳不伪造 Run event；服务器关闭会主动取消长连接。SSE 本身不推送模型正文、不接受 token query、也不执行任何写操作。

首个 React/Vite 只读控制台位于 `web/`。它从 `docs/openapi.json` 生成 TypeScript DTO，读取 Run、Session、WorkItem、Note、Artifact descriptor、ToolRound 与预算/租约摘要，并使用带 Authorization header 的 `fetch` 消费和续传 SSE。生产构建可由 `cyberagent api serve --ui-dir web/dist` 在同一回环 origin 托管；Go 启动时把经过类型、大小、哈希文件名与软链接检查的 bundle 读成不可变内存快照，并对 HTML fallback、静态缓存和 CSP 设定严格边界。read token 只驻留页面内存，不进入 URL、localStorage 或 sessionStorage；浏览器不持有 control token，也不重复实现 Go Policy。Vite 回环代理仍保留为前端开发路径。

Bubble Tea TUI 现在可直接查看当前 Run 状态、Work Board、Notes、持久化 Supervisor Tool Rounds 与 Shell ToolRuns。工具区支持“批准一次”和“本会话授权”：后者只创建精确绑定当前 Run、Session、Workspace、Shell 与 ActionClass 的可撤销 Grant，并在推进当前提案前重新检查持久化审批作用域和最新 Policy。后续安全 Shell 可自动完成 dry-run，危险命令仍永久拒绝且不会创建 Grant；中文、组合字符和宽字符按终端单元格安全换行与截断。

`cyberagent headless events` 提供版本化 `headless.v1` NDJSON。它按 SQLite sequence 有界读取与续传同一条脱敏 Run 时间线，可用 `--follow` 等待终态，但不执行 Run、不调用 Provider、也不写入任何状态。每个事件和最终 `stream.end` 都各占一行；完成返回 0，失败返回 4，取消返回 7，事件上限返回 8，follow 超时返回 9。stdout 始终只包含 NDJSON，诊断文本保留在 stderr。

### English

CyberAgent Workbench is a local AI agent workbench powered by Go for coding, code review, security learning, scripting, and controlled cybersecurity analysis. It brings model calls, long-context memory, workspace files, policy checks, approvals, execution budgets, and event history into one resumable runtime, so work can continue after a process restart and every action remains inspectable.

Each user objective is stored as a `Mission`, while each resumable execution is a `Run`. Go is the sole control plane for model routing, state machines, SQLite persistence, safety policy, and tool boundaries. The current read-only TypeScript console and future deterministic Rust analyzers connect through Go-defined protocols instead of bypassing those controls.

The current priority is the general-purpose Agent runtime and its controlled multi-agent kernel. CTF capabilities will be added later as Profiles and Skills on top of the same foundation rather than as a separate execution system.

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

The report runtime also provides read-only SARIF 2.1.0, CI-gate, and GitHub Actions annotation projections without a database write or Provider call. SARIF `results` include only confirmed unresolved Findings, meaning `validated` and `accepted`; `fixed`, `draft`, and `rejected` entries remain status counts and cannot become upload alerts. `cyberagentValidationStatus` retains the actual validation decision, while `cyberagentFindingStatus` exposes the current lifecycle. The default `validated/high` gate also blocks accepted unresolved findings, explicit `active` additionally admits drafts, and fixed/rejected findings never block. `--format github` renders file/line-aware `notice/warning/error` workflow commands from the same in-memory `GateResult`, strictly escaping model-produced `%`, CR/LF, colons, and commas and rendering other C0/DEL controls visibly. It emits no Artifact content, Evidence note, or operator reason and preserves the existing gate exit status.

The project also includes its first local HTTP control plane. `cyberagent api serve` binds only to loopback. `CYBERAGENT_API_TOKEN` authorizes reads, while the default-disabled cancellation routes require a distinct `CYBERAGENT_API_CONTROL_TOKEN`. Stable `api.v1` envelopes and bounded cursor pagination expose durable metadata without fencing tokens. The two POST routes can only request cancellation of an exact root or Specialist model call: they cannot execute tools, alter Policy, accept a client fencing token, or read Artifact content/checkpoint pending input. The API has no CORS or WebSocket support and never persists either token.

The Go response DTOs now generate the single OpenAPI 3.1 contract. `cyberagent api openapi` emits deterministic JSON, `--output docs/openapi.json` refreshes the tested repository snapshot, and an authenticated running server returns the same raw document at `/api/v1/openapi.json`. Contract tests exercise every published route and prevent drift among DTOs, the committed document, and handlers while explicitly excluding Artifact content, checkpoint pending input, `lease_id`, and fencing tokens.

A read-only Run Event Stream now shares that control plane. `/api/v1/runs/{run_id}/events/stream` uses SSE to publish redacted durable SQLite events. Every frame id is an opaque Run-bound cursor that supports exact `Last-Event-ID` resume. Batch size, frame size, total events, lifetime, write timeout, and process concurrency are bounded; heartbeats never fabricate Run events, and server shutdown cancels long-lived connections. The stream contains no user-visible model text, accepts no token query, and performs no write operation itself.

The React/Vite read console lives under `web/`. It generates TypeScript DTOs from `docs/openapi.json`, reads Runs, Sessions, the bounded Agent graph, operator-gated delegations, read-only Fan-out plans/execution summaries, Finding/Report projections, WorkItems, Notes, Artifact descriptors, ToolRounds, budgets, and leases, and consumes resumable SSE with authenticated `fetch`. A production build can be hosted on the same loopback origin by `cyberagent api serve --ui-dir web/dist`; Go loads the validated bundle into an immutable startup snapshot and enforces bounded HTML fallback, immutable hashed-asset caching, and a strict CSP. The read token stays in page memory and never enters a URL, localStorage, or sessionStorage. Browser DTOs omit Artifact content, private decision narratives, raw Fan-out model reports, digests, and lease/fencing identities. The browser has no control token and does not duplicate Go policy. The loopback Vite proxy remains available for frontend development.

The Bubble Tea TUI now exposes the current Run state, Work Board, Notes, durable Supervisor Tool Rounds, and Shell ToolRuns. Its tool pane supports “approve once” and “approve for this session.” Session approval creates only a revocable Grant scoped to the exact Run, Session, Workspace, Shell tool, and ActionClass, then rechecks the durable approval scope and current Policy before advancing the proposal. Later safe Shell calls may complete as dry runs, while dangerous commands remain permanently denied and cannot create a Grant. Terminal wrapping and truncation account for CJK, combining characters, and other wide graphemes.

`cyberagent headless events` provides versioned `headless.v1` NDJSON over the same redacted durable Run timeline. It reads and resumes by SQLite sequence under hard bounds and may wait for a terminal Run with `--follow`, but it neither executes the Run nor calls a Provider or writes state. Every event and final `stream.end` record occupies one line. Completed, failed, cancelled, event-limit, and follow-timeout outcomes return stable exit codes 0, 4, 7, 8, and 9 respectively. Stdout remains NDJSON-only while diagnostics stay on stderr.

## 核心能力 / Core Capabilities

- **可恢复运行 / Resumable runs:** durable checkpoints, cross-process execution leases with heartbeat/fencing, exact cross-process active-call cancellation, bounded execution, restart recovery, graceful terminal cancellation, and explicit lifecycle actions.
- **Agent Coordinator:** stable root identity, atomic Run/Supervisor status projection, bounded durable inboxes, hashed exactly-once send/finish intent, explicit wake/dependency semantics, internal-only two-child admission and concurrent scheduling, durable schedule summaries, shared-lease cancellation fan-out, exact cross-process child-call cancellation, aggregate Run budget reconciliation, lease-fenced child Attempts, crash/takeover recovery, attempt-bound CompletionReports, two-phase exactly-once root and Specialist instruction context, child-owned memory selection, a durable no-tool Specialist model ledger, and one isolated child lifecycle repair; public/model spawn and autonomous scheduling remain disabled.
- **统一模型网关 / Model gateway:** route-based providers, Anthropic-compatible SSE/tool protocol, typed failures, application-owned active-call cancellation, bounded live progress, one lifecycle-protocol repair, and durable model/tool events.
- **长上下文与结构化记忆 / Long-context memory:** persisted sessions, automatic compaction, durable categorized Notes, visibility rules, and token-budgeted source selection.
- **结构化任务板 / Structured Work Board:** Run-scoped work items, dependency and cycle checks, optimistic versions, transactional events, and bounded Supervisor context.
- **发现项与报告 / Findings and reports:** immutable model-assertion provenance, deterministic exact-fact deduplication, Artifact-backed validation, independent acceptance, fresh remediation Evidence, fixed-state snapshots, stable source projection digests, Go-rendered Markdown/JSON, confirmed-unresolved SARIF 2.1.0, an explicit CI gate, and GitHub Actions annotations from the same GateResult.
- **受控多 Agent 提案 / Controlled multi-Agent proposals:** strict review-gated `specialist_delegation.v1`, at most two assignments, parent-Skill and budget checks, digest-only replay, and no model-controlled admission or spawn.
- **本地工作区 / Local workspace:** scoped file access, safe reads, hashed Run artifacts, and reviewable edit proposals.
- **统一工具网关 / Unified Tool Gateway:** normalized calls, trusted workspace binding, policy decisions, shared review, first-class non-executable `script_process.v1` proposals, a bounded resumable Provider loop for create-only WorkItem/Note tools, bounded UTF-8 results, MIME metadata, and compatibility adapters.
- **安全与审批 / Safety and approval:** policy checks, secret redaction, automatic low-risk reads, durable per-call approvals, revocable scoped Session grants, atomic Run tool budgets, permanent denial, and dry-run command completion.
- **完整审计链 / Audit trail:** append-only Run events plus a bounded resumable SSE projection for messages, context source provenance, text-free model progress, model calls, Notes, policy decisions, tool proposals, file edits, and content-free Artifact metadata.
- **CLI、Headless 与 TUI / CLI, Headless, and TUI:** a scriptable CLI, resumable bounded `headless.v1` NDJSON with stable outcome exits, and a Bubble Tea interface with live model progress, audited cancellation, Run memory/tool-round views, and scoped once/session approvals.
- **可扩展架构 / Extensible architecture:** Go control plane with a generated OpenAPI 3.1 contract, loopback-only read API/SSE, a React/Vite read console for Run/Agent/delegation/Fan-out/report state, separately authorized cancellation, and planned Docker sandbox and Rust analyzer boundaries.

> [!NOTE]
> 当前版本仍在积极开发中。Provider 只能自动创建 WorkItem/Note；双 child 并发必须经过显式 operator review、application 与 v38 schedule request，模型不能自主启动。Web 目前只读；真实 Shell/Docker 执行、更广工具面、通用 HTTP 写入、Web 控制操作、模型自主子 Agent 调度和 CTF 自动求解尚未开放。<br>
> This project is under active development. Providers may only create WorkItems and Notes, and two-child concurrency remains explicitly operator-gated. The Web console is read-only; real Shell/Docker execution, broader model tools, general HTTP mutations, Web control actions, model-driven child scheduling, and automated CTF solving are not enabled yet.

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

### Web Console

```powershell
# repository root: build once, then run one loopback process
cd web
npm ci
npm run check:api
npm run build
cd ..
$env:CYBERAGENT_API_TOKEN = "<local-read-token>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765 --ui-dir web/dist
```

打开 / Open `http://127.0.0.1:8765`. Vite 双进程开发方式、详细边界与检查命令见 [web/README.md](web/README.md)。

控制台当前提供 Run/Session 浏览、Agent 图、operator-gated delegation、只读 Fan-out 执行摘要、Finding/Report、Work/Notes/Artifact metadata/ToolRound 与可恢复事件流。所有状态机和脱敏规则仍由 Go 定义，Web 不提供执行或审批写操作。<br>
The console currently covers Run/Session browsing, the Agent graph, operator-gated delegations, read-only Fan-out execution summaries, Findings/Reports, Work/Notes/Artifact metadata/ToolRounds, and resumable events. Go still owns every state machine and redaction rule; the Web UI exposes no execution or approval mutation.

Use `CYBERAGENT_HOME` to point runtime data at another directory during tests or experiments.

在 Unix 上，新建的运行目录与 SQLite 数据库分别限制为 `0700` 和 `0600`；Windows 继续使用系统 ACL。LocalRunner 当前显式 fail-closed，不会启动宿主机进程，Noop dry-run 输出也会先脱敏。<br>
On Unix, newly created runtime directories and SQLite databases are restricted to `0700` and `0600`; Windows continues to use system ACLs. LocalRunner is explicitly fail-closed and cannot start a host process, while Noop dry-run output is redacted before display.

`api serve` generates and prints a temporary read token when `CYBERAGENT_API_TOKEN` is absent. Cross-process cancellation remains disabled unless a distinct `CYBERAGENT_API_CONTROL_TOKEN` is supplied; environment-provided tokens are validated but never echoed. See [docs/http-api.md](docs/http-api.md) for endpoints, envelopes, pagination, and security boundaries.

## Project Memory

Read [docs/PROJECT_STATUS.md](docs/PROJECT_STATUS.md), [docs/PROGRESS_BOOK.md](docs/PROGRESS_BOOK.md), [docs/TASK_BOOK.md](docs/TASK_BOOK.md), [docs/http-api.md](docs/http-api.md), [docs/openapi.json](docs/openapi.json), [web/README.md](web/README.md), [docs/errors.md](docs/errors.md), [ADR 0001](docs/adr/0001-go-control-plane.md), and [ADR 0002](docs/adr/0002-run-centric-runtime.md) first when resuming development after a long conversation. They record current progress, language ownership, run architecture, API and error contracts, audit notes, verified commands, and the recommended next slice.

## Repository Workflow

The canonical remote is [Qiyuanqiii/CTF-CyberAgent-Workbench](https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench). Each completed development slice ends with tests, a focused code/security audit, project-memory updates, a Git commit, and a push to GitHub. This repository currently develops directly on `main`; do not create a feature branch or pull request unless the user explicitly asks for one.

Local runtime databases, workspace data, environment files, API keys, IDE metadata, and build output are excluded from Git.

## 许可证 / License

本项目由仓库所有者以 [Apache License 2.0](LICENSE) 授权。你可以在许可证条件下使用、修改和分发代码；分发时请保留许可证与所需声明。Apache-2.0 同时包含明确的专利许可与商标限制，完整法律条款以仓库 `LICENSE` 文件为准。

This project is licensed by the repository owner under the [Apache License 2.0](LICENSE). You may use, modify, and distribute the code subject to its terms, including preservation of the license and required notices. Apache-2.0 also provides an express patent grant and trademark limitations; the repository `LICENSE` file is the authoritative legal text.

## Development Priority

The current priority is the V2 run-centric runtime. P0 and P1 are complete. P2 supports resumable root Agent turns, cumulative token/model-time accounting, bounded execution and Provider retry loops, strict Supervisor-owned `continue`, `finish`, and `wait` actions, one Run execution path for ordinary CLI/TUI Session chat, real Provider streaming with bounded `model.delta` progress, local and schema v18 cross-process active-call cancellation, Bubble Tea live metadata, durable model events, exactly one restart-safe lifecycle-protocol repair, the schema v16 bounded structured-memory tool loop, and schema v17 execution leases with heartbeat/fencing. P3 includes migration v9 WorkItems, migration v10 Notes, transactional relationships/events, `todo` and `note` CLI lifecycles, root/owner visibility, token-budgeted memory selection, and durable context provenance. P4 now has the schema v19 single-root Coordinator, schema v20 idempotent inbox protocol, schema v21 internal-only Specialist admission, schema v22 same-Run Agent-owned WorkItems/Notes, schema v23 attempt-bound CompletionReports, schema v24 lease-fenced Specialist Attempt scheduling/usage/crash recovery, schema v25 two-phase exactly-once root inbox context, schema v26's opt-in internal no-tool Specialist turn with an atomic model/usage/Policy ledger, schema v27's recoverable parent-instruction plus child-owned-memory context, schema v28's exactly-once child lifecycle repair, schema v29 durable schedule summaries plus exact cross-process child-call cancellation, schema v30 review-gated root delegation proposals, schema v31 immutable operator review facts, schema v32 recoverable operator application around the Go-internal two-child scheduler, schema v33 immutable read-only Fan-out plans, and schema v34 lease-fenced tool-free execution for up to six read-only shards. Core delegation remains capped at two; Fan-out is a separate read-only analysis pool with no Agent admission, tools, network, or writes. P5 includes the unified Tool Gateway, trusted workspace scope binding, schema v11 durable per-call approvals, schema v12 revocable Session grants and atomic Run tool-call accounting, schema v13 first-class typed script processes, schema v14 source-bound hashed output Artifacts, schema v15 idempotent structured-memory mutations, v16 durable Provider tool rounds, and the v30 `agent_proposal` tool class. P9 now includes the authenticated loopback-only `api.v1` read surface, distinct root/child cancellation-control operations, token-free lease status, a deterministic Go-generated OpenAPI 3.1 contract, bounded resumable Run-event SSE, generated React/Vite Run/Agent/delegation/Fan-out/Finding views, explicit same-process production bundle hosting, and bounded resumable `headless.v1` NDJSON with stable outcome exits. TUI Event-stream unification, Monaco/xterm surfaces, and Web control mutations remain pending. Real Local/Docker command execution and all executing model tools remain disabled. CTF-specific solving logic stays deferred until the generic runtime is stable.

P8 now includes schema v35's deterministic Finding/Evidence/Report projection, schema v36's immutable same-Run Artifact Evidence plus one-time operator `validated/rejected` decisions, schema v37's independent acceptance/fresh-remediation/fix lifecycle, confirmed-unresolved SARIF 2.1.0 export, a typed read-only CI gate, and GitHub Actions workflow-command annotations derived from that same GateResult. Source projection digests remain stable across every lifecycle overlay. Additional CI-platform adapters remain future work.

TUI quick controls: `cyberagent tui` opens a session picker. In chat, `Tab` switches between input and the activity pane; `h/l` changes among Tools, Work, Notes, and Rounds; `j/k` moves the current selection; `a` approves one Shell proposal, `g` approves it and grants the exact current session scope, and `d` denies it. `PgUp/PgDn` scrolls messages, `Ctrl+X` requests cancellation of the current model call, `Ctrl+R` refreshes, and `Esc` quits when idle. Busy sends cannot be closed accidentally with `Esc`; cancel or wait first. The status line renders provider/model, attempt, chunk/byte progress, cancellation, disconnect, and terminal state without exposing raw model text. Attached workspaces render in the side panel with local directory counts for attachments, scripts, outputs, logs, and writeups.

Workspace file reads and model-bound messages pass through heuristic secret redaction for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, and private-key blocks.

File changes, Shell proposals, typed script-process proposals, and create-only structured memory calls enter the same Tool Gateway and Run-event stream. The schema v11 approval ledger is inspectable through `approval list/show`; review idempotency survives restart, while a conflicting reuse of the same key is rejected. Schema v12 Session grants are managed through `approval grant create/list/show/revoke`, and `run usage` exposes durable tool consumption. Schema v13 stores script processes separately from legacy Shell ToolRuns and makes initial Run creation recoverably idempotent. Schema v14 stores redacted full-stream evidence separately from bounded Results and links each Artifact to its exact proposal or invocation. Schema v15 stores only operation-key digests and normalized request fingerprints for WorkItem/Note creation. `edit propose` and session `/write` normally persist a redacted diff without changing the workspace; an explicitly created matching Session grant may authorize and apply the edit immediately, with the grant ID recorded on the approval fact. Shell and script-process approval still produce dry-run output only.

## 可选在线 Provider / Optional Online Providers

CLI 只在对应 API key 存在于当前进程环境时注册 `mimo` 或 `deepseek` Anthropic-compatible Provider。密钥不会写入仓库文件、SQLite 或 Run 事件。Provider 地址必须使用 HTTPS，只有 loopback 本地服务可使用 HTTP；客户端不会跟随重定向，API key 也不能包含空白或控制字符。<br>
The CLI registers the `mimo` and `deepseek` Anthropic-compatible providers only when their API keys exist in the current process environment. Keys are never written to repository files, SQLite, or Run events. Provider endpoints must use HTTPS except for loopback-local HTTP services; redirects are not followed, and API keys cannot contain whitespace or control characters.

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
