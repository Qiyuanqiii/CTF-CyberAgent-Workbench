# CyberAgent Workbench 进度书

更新时间：2026-07-10

## 一、当前阶段

项目正在从可运行的 v0.1 CLI/TUI 骨架迁移到 V2 Run-centric Runtime。CTF 专用求解能力继续后置，当前先完成主流 AI Agent 工具需要的通用运行时。

当前完成度：

- 整体产品愿景：约 67%。
- v0.1 通用 Agent MVP：约 98%。
- V2 Run-centric Runtime：约 80%。
- 项目骨架和模块边界：约 99%。

V2 的 P0/P1 已完成，P2 第七纵向切片已落地：RunSupervisor 全面走 StreamChat，Anthropic-compatible Provider 支持真实 SSE，流聚合执行 UTF-8、64 KiB、final usage 和取消校验，并把每次调用压缩为最多 32 条无文本 `model.delta`。下一步实现 application-owned 主动取消与实时订阅边界。

## 二、已完成功能

### Agent 与运行时

- CLI 入口、命令分发、版本命令和 Bubble Tea TUI。
- Agent Kernel、Planner、Executor、Critic 与 Task/Event 类型边界。
- 持久化 Session、Message、Task、Event、Artifact 和上下文摘要。
- Codex 风格的长上下文压缩骨架，支持手动和自动压缩。
- `/help`、`/compact`、`/model`、`/workspace`、`/ls`、`/read`、`/write`、`/run` 会话命令。

### 模型层

- Provider 接口、模型路由和可重复测试的 MockProvider。
- Anthropic-compatible Provider，已用环境变量完成 Mimo 连通验证。
- 模型请求进入 Provider 前进行敏感信息脱敏。
- Provider 错误统一分类为 retryable、rate_limited、invalid_response、cancelled、permanent。
- Anthropic-compatible Provider 对 429、408/425、5xx/529、永久 4xx、畸形/空响应和 `Retry-After` 进行类型化处理。
- RunSupervisor 默认最多三次模型尝试，100ms 指数退避且单次最多等待 2s；超长 `Retry-After` 不会被提前重试。
- 每次模型调用持久化连续编号的 `model.started/completed/failed`，取消与重启后继续编号。
- 无效 root lifecycle 输出只触发一次纠错提示；不会把原始坏输出回放给模型、写入 Session 或写入事件。
- transport retry 每个协议阶段独立最多三次，全局 model attempt 编号保持连续；CLI 显示 `protocol_repairs`。
- Router 的 StreamChat 与 Chat 共用模型解析和请求脱敏；RunSupervisor 不再旁路 streaming 接口。
- Anthropic-compatible Provider 解析 `message_start/content_block_delta/message_delta/message_stop/error` SSE，并在 final chunk 返回模型与 usage。
- Stream aggregator 支持跨 chunk UTF-8，拒绝超 64 KiB、缺失 Done/usage、负数或溢出 usage、ToolCall、畸形流和中途取消。

### 工作区、编辑与工具

- 本地工作区创建、查询、目录树和文本读取。
- 拒绝绝对路径、`..` 穿越和符号链接逃逸。
- 文件编辑提案、diff 预览、审批、拒绝、失败和已应用状态持久化。
- 审批前重新解析路径并校验 SHA-256，拒绝覆盖提案后被修改的文件。
- ToolRun 提案与审批状态机；`/run` 当前只创建提案，批准仍为 dry-run。
- Policy Checker 拒绝未授权扫描、公网攻击、凭证窃取和明显破坏性命令。
- Noop、Local 和占位 Docker Runner；Docker 当前只检测并返回明确错误。

### 存储与 Run 架构

- CGO SQLite 驱动 `github.com/mattn/go-sqlite3`。
- checksum 校验的版本化事务 migration，可保留旧库数据原地升级。
- Mission、Run 和 append-only Run Events 持久化。
- schema v3 为非空 `session_id` 建立唯一关联并拒绝引用不存在 Session 的 Run。
- 新 Run 默认在同一事务中创建独立 Session；也可绑定一次现有活跃 Session，并统一工作区和模型路由。
- Session Message、assistant policy、ToolRun 和 FileEdit 状态会在业务写入的同一事务内投影为 Run Event。
- 重复保存不会产生重复事件；跨工作区 ToolRun/FileEdit 会在 Store 边界回滚。
- `apperror` 提供稳定代码、CLI 退出码和未来 HTTP 映射，现有错误文本保持不变。
- schema v4 使用 `legacy_task_runs.task_id` 作为幂等键，`run adapt-task` 可安全重复或并发执行。
- TaskAdapter 在一个事务内创建 Session、Mission、Run、映射和三条初始事件；历史状态不会触发隐式执行。
- 旧 Task Goal 与旧 Event 内容补齐 Store 级脱敏。
- schema v5 持久化 Supervisor phase、next turn、attempt 和脱敏错误。
- schema v6 在同一 checkpoint 中持久化累计 input/output/total tokens 与模型执行毫秒数。
- schema v7 在 Provider 调用前持久化脱敏且不超过 64 KiB 的 pending input，完成后原子清空。
- schema v8 持久化协议修复阶段和脱敏原因，支持 pending/exhausted 状态的进程重启恢复。
- `run step` 每次只执行一个无工具规划 turn；`run checkpoint` 可观察恢复状态。
- 模型调用前写 started checkpoint，完成时原子写消息、策略、用量、事件和下一个 checkpoint。
- 重启会恢复同一 started attempt；已提交完成的 turn 和消息不会重复。
- MaxTurns、MaxTokens、模型执行超时与调用前 cancellation 已执行；剩余 token/时间会传入 Provider 请求上下文。
- `run execute` 提供显式步数上限；`run finish/fail` 在一个事务内推进 Run、Supervisor checkpoint 和事件，重复相同终态命令幂等。
- 模型返回 ToolCall 会失败且不会创建 ToolRun；只有校验通过的结构化 action 能推进 Run，自由文本不能。
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

依赖按功能引入。Bubble Tea、SQLite/CGO 和 `net/http` 已使用；Cobra、Chi、Docker client、Qdrant、JSON Schema、Rust crates 与 React/Vite 等到对应功能开始实现时再加入。

## 四、尚未完成

- application-owned 主动取消句柄、实时内存订阅与 WebSocket 推送；当前已聚合真实 Provider stream，但 CLI/TUI 只在 turn 完成后展示文本。
- Provider 费用预算、工具调用预算和 token-aware Context Builder。
- 结构化 WorkItem、Notes、Findings、Evidence 与 Report。
- 跨进程主动取消和实时 TUI 状态体验。
- OpenAI-compatible 与 Ollama Provider。
- 统一 Tool Gateway 与真实 Docker 隔离。
- Go HTTP/WebSocket API、TypeScript Web UI 和 Rust analyzer 进程。
- MCP Server、插件系统和远程任务能力。
- 通用 Agent 稳定后的 CTF 自动分析与求解流程。

## 五、审计结论

最新审计未发现高严重度问题。主要残余风险：

- `staticcheck ./...` 仅报告 3 个既存 TUI 低严重度问题：`S1008`、`S1011` 和未使用 helper `U1000`；本轮协议修复代码无命中。
- `script run --local` 仍可在词法策略检查后直接本机执行，应接入统一提案、审批和 Run 事件。
- 文件写入已二次解析路径并重新校验哈希，但跨平台 `os.WriteFile` 无法完全消除很小的 TOCTOU 窗口。
- Windows 当前账号不能创建符号链接，真实链接逃逸测试会跳过；运行时仍会解析链接并检查工作区边界。
- 脱敏是启发式安全层，不是完整的 Secrets Manager。
- Docker Runner 还不是真实隔离边界。
- `run start` 只推进生命周期，`run step` 执行一个模型 turn，`run execute` 只执行操作者指定的有限步数。
- pre-call checkpoint 后崩溃可能重发一次无副作用模型请求，但已完成 turn 不会重复；工具调用因此继续禁用。
- Supervisor 历史目前按 20 条消息限制，还不是 token-aware Context Builder。
- MaxCostUSD 与工具调用预算尚未执行，因为 Provider 价格元数据和统一 Tool Gateway 尚未建立。
- 执行时间当前只统计 Provider 模型调用；一次 Provider 调用可能超过剩余 token，但实际用量会完整记账并阻止下一次调用。
- 预算边界停止执行后 Run 保持 `running`，需操作者显式 `finish`、`fail` 或 `cancel`；模型输出不能自行终结 Run。
- 本轮审计已修复 Provider 极端用量导致的累计整数溢出，以及超大 `--max-steps` 触发不受控预分配的问题。
- 严格协议只提供一次自动修复；若 Provider 连续两次不遵循 JSON 契约，本轮失败并等待操作者后续重试，不会无限纠错。
- 持久化 `model.delta` 故意不含文本，因此不能从 SQLite 重放逐 token 内容；未来实时 UI 必须消费 Go 控制的短生命周期订阅，并继续经过脱敏/背压边界。
- `wait` 目前映射为 Run paused 和文本 reason，尚无结构化 dependency/approval 对象。
- 未绑定 Run 的 Session 仍直连 Router；这是迁移兼容路径，新功能不应继续扩展该旁路。
- slash command 尚未消耗 Supervisor turn；后续统一 Tool Gateway 时必须保持审批和事件语义，不能直接启用执行。
- pending input 虽已脱敏并限制大小，仍属于会话内容而非 Secrets Manager；`run checkpoint` 不回显原文。
- Provider 自动重试目前只在 RunSupervisor 内启用；未绑定 Run 的 legacy Session 虽有 typed error，仍不自动重试。
- 退避当前无随机抖动；在开放远程并发 worker 前需增加 jitter，避免同一 Provider 同步重试。
- 超过 2s 的服务端 `Retry-After` 会直接返回限流状态并保留输入，需要后续操作者重试。
- 若进程在 `model.completed` 后、turn 提交前退出，恢复时可能以新的 model attempt 重发一次无副作用请求；前一次 token/耗时已经原子记账，因此不会漏算，但工具调用仍关闭。
- 已发布 migration 的语句和 checksum 不可修改，后续 schema 变化必须新增版本。
- v3 会拒绝一个 Session 关联多个 Run；若旧数据库存在重复关联，应先审计，而不是自动丢弃数据。
- 兼容期仍有普通字符串错误通过 `apperror.Normalize` 分类；新服务必须直接返回 typed error。

## 六、验证基线

每个开发切片至少执行：

```powershell
go test -count=1 ./...
go vet ./...
```

共享状态、并发或存储变更还要运行相关包的 `go test -race`。CLI 行为变更要在隔离的 `CYBERAGENT_HOME` 中完成 smoke test。提交前扫描凭据前缀，确认本地数据库、工作区、环境文件和 API key 未进入 Git。

最新验证已覆盖严格 action、schema v8、Session/Run 统一路径、一次协议修复成功、二次协议失败、跨 chunk UTF-8、接近 64 KiB 的 32 条 delta 上限、流中瞬态重试、超限/坏 UTF-8/缺 Done/缺 usage、delta 幂等与终态一致性，以及流中取消后恢复。独立 CLI smoke 已验证 `stream_events/stream_bytes`、单条 text-free `model.delta` 与匹配的模型终态计数。

本机 Go 已从 1.26.1 升级到 1.26.5；升级前 `govulncheck` 命中 9 条可达标准库漏洞，升级后复扫为 0。协议修复 transport 测试验证全局 attempt `1/2/3` 与阶段内 transport `1/1/2`，Store 也会拒绝与 durable started event 不一致的终态元数据。

## 七、下一开发切片

1. 在 application service 层建立 active-call registry，按 Run/attempt 暴露幂等取消句柄，UI 不直接持有 Provider context。
2. 增加有界实时订阅接口，只发送经过 Go 脱敏和背压控制的临时文本 delta；SQLite 继续只存无文本进度。
3. 让 CLI/TUI 可选择消费实时状态，同时关闭 UI 不取消后台 Run。
4. 为未来 HTTP/WebSocket 固化 stream envelope、慢消费者策略和断线恢复语义。
5. 工具执行和多 Agent 并发继续关闭。

## 八、仓库同步与恢复约定

规范远程仓库：`https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench`。

每次完成一个开发切片后，依次执行功能复核、测试、代码与安全审计、项目记忆更新、Git 提交和 GitHub 推送。PR 由用户主动创建；使用功能分支时生成可直接采用的 Summary、Validation 和 Audit 文本。

长对话恢复时依次阅读：`README.md`、`docs/PROJECT_STATUS.md`、本文件、`docs/TASK_BOOK.md`、`docs/errors.md`、`docs/adr/0001-go-control-plane.md` 和 `docs/adr/0002-run-centric-runtime.md`。
