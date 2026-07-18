# CyberAgent Web Console

## 中文

schema v42 的 Run 概览显示三个 Plan 方向、已选方向、切片数量与是否仍需显式切换到 Deliver。D1-P1 在独立 capability 下增加“方向三选一”和“进入 Deliver”两个显式操作；选择不改阶段，Deliver 不启动/恢复执行，`capability_grant` 始终为 `false`。

schema v45-v46 的 Run 概览显示操作者引导队列的 pending/prepared/committed/cancelled 计数和有界元数据。D1-S1/S2 通过两个独立 capability 增加现有 Run-bound Session 的 enqueue/replay 和 pending-only 取消；`prepared=true` 时 UI 不提供取消。schema v73 再增加独立的 Run start/pause/resume 和最多八条冻结输入的显式 RunSupervisor 交接。浏览器不接收消息/模型正文、工具参数、operation 或 lease 身份，也不能编辑、重排或后台唤醒队列。

schema v71 的 External Skills 面板只读显示 Run 固定选择的 surface/profile、版本、信任类别、token 上界、声明工具数量以及 root/Specialist 准备/提交计数。它不接收 Skill 正文、路径、字节数、hash/digest/fingerprint、安装/选择 ID、operation key 或操作者/agent 身份，也没有 Run 选择、授权或执行按钮。D1-B1 仅在 Desktop 原生预览后提供另一次显式惰性安装确认；它不自动改变该面板对应的 Run 选择。

这是 CyberAgent Workbench 的本地 read-mostly 运维界面。React/Vite 只消费 Go 生成的 OpenAPI 3.1 DTO 和 `api.v1`，不会重新实现 Policy、审批、工作区权限、Shell、Docker、模型路由或 SQLite 逻辑。当前窄 mutation 包括非授权档位、受控 Run 创建、Session 排队/pending 取消、schema-v73 Run 生命周期/有界 Supervisor 交接、Plan/Deliver、受限审批、Diff 审阅/独立 apply、wake 意图/显式前台消费，以及 Desktop 确认式惰性 Skill 安装。

当前界面提供：

- Run 列表、状态过滤、目标/预算/租约摘要；
- 注册 Workspace 选择，以及 Profile/Code-Cyber/Plan-Deliver 配置下的受控 Run 创建；
- 有界 root/Specialist Agent 图、operator-gated delegation 审查/应用/调度摘要；
- 只读 Fan-out 计划与最新 execution/shard 元数据；
- Finding/Report 严重度、来源、Artifact metadata 与生命周期时间线；
- 带 `Last-Event-ID` 恢复的鉴权 SSE 事件流；
- WorkItem、Note、Artifact descriptor 和 Supervisor ToolRound 视图；
- 操作者引导队列计数、顺序与状态元数据；
- 外部 Skill 的有界来源、预算与交付状态元数据；
- Session 列表、绑定 Run 和包含压缩状态的消息历史；
- capability 开启且 Run 为 running/paused 时的 Session composer；成功只显示排队 sequence/status；
- 未 prepared 的 pending 消息取消、Run Start/Pause/Resume 和 `maxSteps=1..8` 的 Run Queue 控件；
- 只读脱敏 Provider/模型路由对话框、显式 Plan 选择/Deliver 控件和 metadata-only Approvals 页；
- Diff 审阅后独立 apply、wake schedule/cancel 后一次前台 consume，以及 Desktop 原生 Skill 预览后的惰性安装确认；
- Go-owned Files 页：只浏览注册 Workspace 的 canonical 相对路径、脱敏有界 UTF-8 与 non-authorizing provenance；
- Go-owned Actions 页：聚合最多 100 条 pending steering/approval/FileEdit/due-wake metadata，只导航到既有视图，不自动处理；
- Evidence 页：只列 exact Run/Session 已附加来源/hash/time 与固定 false instruction authority，source navigation 复用既有 Files 边界；
- `Ctrl+K` 命令面板：只导航既有 Run 页签或刷新当前查询，不提交 mutation、路径、正文、审批、进程或 capability；
- apply/wake/install 的统一 durable operation receipt，严格显示 replay/retry/cleanup 状态；
- `preview|docker|local` 执行档位、固定门禁与 false authority 状态；
- 只驻留页面内存的 read bearer token 和可选 distinct control token。

浏览器不持久化 token，也不把 token 放入 URL、body 或静态资源请求。可选 control token 只驻留页面内存；Go 决定 Workspace/Run/Session 绑定、默认预算、backend、scope、审批、风险、门禁和全部权限位。不确定失败后的 operation key 只在同一未改变意图下复用；消息、lifecycle action、`maxSteps`、Diff 或 wake 意图改变都会轮换 key。除独立 Explorer 返回的有界、脱敏、明确非授权 Workspace 文本外，Artifact/Skill/Session 正文、模型输出、工具参数、私有叙述、lease/fencing token 和宿主/容器进程启动仍不向 Web 开放；FileEdit apply 和 Skill Registry 写入只能走各自的窄 Go control。事件 envelope 版本由 Go/OpenAPI/TypeScript 共用 literal `v1`；SSE 在 parse/transport failure 后先 cancel reader 再重连，避免遗留响应占满浏览器连接。

同一 bundle 也被 Desktop 编译进 Wails v2.13.0 Windows 壳。React 通过四个方法的严格 native bridge 取得内存连接材料、选择/预览 Skill，并消费一次性安装确认句柄；不监听端口、不使用 Local Storage。Desktop 通过同一高水位 cursor 轮询事件，Skill picker 不向 React 暴露路径或 bytes。Actions、Evidence 和命令面板只消费 read bearer/本地导航。除 handle-only 惰性 Skill 安装外，New Run、Session enqueue/cancel、Run lifecycle、bounded execution、Plan/Deliver、Approval、Diff 与 wake mutation 始终走 Go HTTP Handler，并且只在各自 capability 开启时出现。

### 生产式本地运行

```powershell
# repository root
cd web
npm ci
npm run check:api
npm run build
cd ..
$env:CYBERAGENT_API_TOKEN = "<local-read-token>"
$env:CYBERAGENT_API_CONTROL_TOKEN = "<optional-distinct-control-token>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765 --ui-dir web/dist
```

打开 `http://127.0.0.1:8765`，输入同一个 read token；需要切换档位或创建 Run 时再输入 distinct control token。`--ui-dir` 必须指向 Vite 生产输出；Go 会在启动时校验并读入不可变快照。HTML 不缓存，带哈希的静态资源使用 immutable 缓存，未知资源与不安全 fallback fail closed。静态请求匿名可读，但携带 `Authorization`、query、body 或非 GET/HEAD 方法会被拒绝，避免 bearer 意外发送给资源处理器。

### Vite 开发模式

```powershell
# terminal 1: repository root
$env:CYBERAGENT_API_TOKEN = "<local-read-token>"
$env:CYBERAGENT_API_CONTROL_TOKEN = "<optional-distinct-control-token>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765

# terminal 2
cd web
npm ci
npm run check:api
npm run dev
```

打开 `http://127.0.0.1:5173`。默认代理目标为 `http://127.0.0.1:8765`；需要改端口时设置 `CYBERAGENT_API_TARGET`，该值仍必须是 HTTP(S) 回环 URL。

## English

This is the local read-mostly operations UI for CyberAgent Workbench. React/Vite consumes the Go-generated OpenAPI 3.1 DTOs and `api.v1`; it does not reimplement policy, approvals, workspace authorization, Shell, Docker, model routing, or SQLite behavior. Its narrow mutations are non-authorizing profiles, controlled Run creation, Session enqueue/pending cancellation, schema-v73 Run lifecycle/bounded Supervisor handoff, Plan/Deliver, constrained approvals, separate Diff review/apply, wake intent/foreground consumption, and Desktop-confirmed inert Skill installation.

The current UI includes controlled Run creation, Run/Session browsing, redacted model availability, explicit Plan/Delivery, a metadata-only approval queue, steering counts and ordered metadata, Session enqueue/pending cancellation, Start/Pause/Resume, an at-most-eight-step Run Queue action, Agent/delegation/Fan-out/Finding views, resumable events, Work/Notes/Artifact/ToolRound metadata, budgets/leases, execution profiles, external-Skill provenance, separately gated Diff apply and foreground wake consumption, Desktop-confirmed inert Skill installation, durable operation receipts, a bounded Go-owned Workspace Files view, a read-only operator action center, attached-evidence inventory, and a navigation/refresh-only `Ctrl+K` palette. Bearers and intent-bound retry keys stay in page memory and never enter a URL or browser storage. TypeScript submits no backend, scope, gate, private lease, host path, file body, tool argument, or authority field; Explorer navigation uses only exact Go-issued relative child paths and renders redacted file evidence as plain text. Go/OpenAPI/TypeScript share event-envelope literal `v1`, and failed SSE streams are cancelled before reconnect so abandoned responses cannot consume browser connection slots.

For the production-style local path, run `npm run build`, then start `cyberagent api serve --ui-dir web/dist`. Go validates and snapshots the bundle at startup, serves it from the same loopback origin as `api.v1`, applies a strict CSP, disables HTML caching, and gives only hashed assets immutable caching. Static requests are anonymous but reject authorization headers, queries, bodies, and methods other than GET/HEAD. For frontend development, Vite can still proxy same-origin `/api` requests to a loopback Go service and rejects non-loopback targets.

Desktop compiles the same bundle into a Wails v2.13.0 Windows shell. React obtains ephemeral connection material through a strict four-method native bridge and calls the Go API in process without a listening port or browser storage. Desktop consumes `run-event-poll.v1` with the same Run-bound high-water cursor as SSE, while WebView2 and renderer-origin/navigation gates fail closed. Actions, Evidence, and the command palette add no native method. The native `.zip` picker exposes neither path nor bytes; the fourth method accepts only Go's one-time install-confirmation handle. Run creation, Session enqueue/cancel, lifecycle, bounded execution, Plan/Deliver, Approval, Diff, and wake controls remain independent Go HTTP capabilities.

Open `http://127.0.0.1:8765` for the Go-hosted build or `http://127.0.0.1:5173` for Vite development, then enter the same read token shown or configured for the Go process. Enter the distinct control token only for a narrow mutation. Bounded execution may call the configured model and approved structured tools through Go, but no Web control can start a Shell, Local, Docker, or other host/container process.

## Checks

```powershell
npm run check:api
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```
