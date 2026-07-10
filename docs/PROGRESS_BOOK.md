# CyberAgent Workbench 进度书

更新时间：2026-07-10

## 一、当前阶段

项目正在从可运行的 v0.1 CLI/TUI 骨架迁移到 V2 Run-centric Runtime。CTF 专用求解能力继续后置，当前先完成主流 AI Agent 工具需要的通用运行时。

当前完成度：

- 整体产品愿景：约 46%。
- v0.1 通用 Agent MVP：约 97%。
- V2 Run-centric Runtime：约 41%。
- 项目骨架和模块边界：约 99%。

V2 已完成版本化 migration、Mission/Run 状态机、自动 Run/Session 绑定、append-only 事件主干、活动投影和 CLI 生命周期。下一步完成稳定错误码与 legacy Task 适配，不做一次性重写。

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

- 跨 CLI/API 的稳定 typed error codes。
- legacy `agent.Task -> Mission/Run` 幂等兼容适配器。
- 可恢复的 RunSupervisor 和单 root Agent 执行循环。
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
- `run start` 当前只推进生命周期，不会启动模型或命令；执行能力要等 RunSupervisor 落地。
- 已发布 migration 的语句和 checksum 不可修改，后续 schema 变化必须新增版本。
- v3 会拒绝一个 Session 关联多个 Run；若旧数据库存在重复关联，应先审计，而不是自动丢弃数据。

## 六、验证基线

每个开发切片至少执行：

```powershell
go test -count=1 ./...
go vet ./...
```

共享状态、并发或存储变更还要运行相关包的 `go test -race`。CLI 行为变更要在隔离的 `CYBERAGENT_HOME` 中完成 smoke test。提交前扫描凭据前缀，确认本地数据库、工作区、环境文件和 API key 未进入 Git。

最新验证已覆盖 Run create/start/pause/resume/cancel/show/events 生命周期、migration 升级与回滚、自动/现有 Session 绑定、统一事件投影、幂等保存、跨工作区回滚、文件编辑审批、工具 dry-run、上下文压缩、模型路由、TUI 快照和敏感信息脱敏。独立 CLI smoke 在多次进程调用间生成了 14 条连续 Run Event。

## 七、下一开发切片

1. 增加稳定的 typed error codes，保持现有人类可读 CLI 错误不变。
2. 增加 legacy `agent.Task -> Mission/Run` 幂等适配器。
3. 将适配结果写成标准 Run Event，重复转换不得创建重复 Mission 或 Run。
4. 保持 `run start` 仅推进生命周期，多 Agent 并发继续关闭，直到 RunSupervisor 到来。

## 八、仓库同步与恢复约定

规范远程仓库：`https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench`。

每次完成一个开发切片后，依次执行功能复核、测试、代码与安全审计、项目记忆更新、Git 提交和 GitHub 推送。PR 由用户主动创建；使用功能分支时生成可直接采用的 Summary、Validation 和 Audit 文本。

长对话恢复时依次阅读：`README.md`、`docs/PROJECT_STATUS.md`、本文件、`docs/TASK_BOOK.md`、`docs/adr/0001-go-control-plane.md` 和 `docs/adr/0002-run-centric-runtime.md`。
