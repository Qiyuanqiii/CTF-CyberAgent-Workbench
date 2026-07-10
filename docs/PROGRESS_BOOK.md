# CyberAgent Workbench 进度书

更新时间：2026-07-10

## 一、当前阶段

项目正在从可运行的 v0.1 CLI/TUI 骨架迁移到 V2 Run-centric Runtime。CTF 专用求解能力继续后置，当前先完成主流 AI Agent 工具需要的通用运行时。

当前完成度：

- 整体产品愿景：约 59%。
- v0.1 通用 Agent MVP：约 98%。
- V2 Run-centric Runtime：约 63%。
- 项目骨架和模块边界：约 99%。

V2 的 P0/P1 已完成，P2 第三纵向切片已落地：严格 `root_lifecycle.v1`、Supervisor-owned `continue/finish/wait`、模型终态原子提交和可恢复等待。下一步将现有 Session chat 统一接入 RunSupervisor，消除普通消息直连 Router 的旁路。

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

- 现有 Session chat 的 RunSupervisor 接入；普通消息当前仍直接调用 Router。
- Provider 限流/可重试错误、流式事件和自动协议修复。
- Provider 费用预算、工具调用预算和 token-aware Context Builder。
- 结构化 WorkItem、Notes、Findings、Evidence 与 Report。
- 真正的流式 token 更新、取消和超时体验。
- OpenAI-compatible 与 Ollama Provider。
- 统一 Tool Gateway 与真实 Docker 隔离。
- Go HTTP/WebSocket API、TypeScript Web UI 和 Rust analyzer 进程。
- MCP Server、插件系统和远程任务能力。
- 通用 Agent 稳定后的 CTF 自动分析与求解流程。

## 五、审计结论

最新审计未发现高严重度问题。主要残余风险：

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
- 严格协议对不遵循 JSON 指令的 Provider 会返回可审计失败；当前不会自动追加修复提示重试。
- `wait` 目前映射为 Run paused 和文本 reason，尚无结构化 dependency/approval 对象。
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

最新验证已覆盖严格 action 解码、mock JSON 模式、continue、原子 finish、wait/park/跨 Store 重启恢复、无效协议不写消息、终态重放幂等、action/event 脱敏，以及原有 schema v6、预算、审批、上下文、TUI 和安全边界。独立 CLI smoke 已验证 action 输出、两次 action 事件、显式终态与 token 预算退出码 8。

## 七、下一开发切片

1. 为 Session Manager 定义不依赖 application 包的 Run chat executor 接口。
2. 普通 Session 消息通过已绑定 Run 的 Supervisor 执行，避免重复 user message 和包循环依赖。
3. 未绑定 Run 的旧 Session 保留明确兼容路径，并记录迁移边界。
4. 统一普通 CLI/TUI chat 与 headless Run 的 policy、budget、action 和事件行为。
5. 工具执行和多 Agent 并发继续关闭。

## 八、仓库同步与恢复约定

规范远程仓库：`https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench`。

每次完成一个开发切片后，依次执行功能复核、测试、代码与安全审计、项目记忆更新、Git 提交和 GitHub 推送。PR 由用户主动创建；使用功能分支时生成可直接采用的 Summary、Validation 和 Audit 文本。

长对话恢复时依次阅读：`README.md`、`docs/PROJECT_STATUS.md`、本文件、`docs/TASK_BOOK.md`、`docs/errors.md`、`docs/adr/0001-go-control-plane.md` 和 `docs/adr/0002-run-centric-runtime.md`。
