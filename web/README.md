# CyberAgent Web Console

## 中文

schema v42 的 Run 概览会只读显示三个 Plan 方向、已选方向、切片数量与是否仍需显式切换到 Deliver。浏览器没有方向选择、阶段切换或执行入口，`capability_grant` 始终为 `false`。

schema v45 的 Run 概览还会只读显示操作者引导队列的 pending/prepared/committed/cancelled 计数和有界消息元数据。schema v46 的本地取消与 drain 不新增 Web mutation：浏览器不接收正文、摘要、操作者或 Session 内部身份，也没有入队、编辑、重排、取消、唤醒或投递按钮。

schema v71 的 External Skills 面板只读显示 Run 固定选择的 surface/profile、版本、信任类别、token 上界、声明工具数量以及 root/Specialist 准备/提交计数。它不接收 Skill 正文、路径、字节数、hash/digest/fingerprint、安装/选择 ID、operation key 或操作者/agent 身份，也没有安装、选择、授权或执行按钮。

这是 CyberAgent Workbench 的本地 read-mostly 运维界面。React/Vite 只消费 Go 生成的 OpenAPI 3.1 DTO 和 `api.v1`，不会重新实现 Policy、审批、工作区权限、Shell、Docker、模型路由或 SQLite 逻辑。当前可见 mutation 只有 schema v64 的非授权 Run 执行档位选择和 schema v72 的受控 Run 创建。

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
- `preview|docker|local` 执行档位、固定门禁与 false authority 状态；
- 只驻留页面内存的 read bearer token 和可选 distinct control token。

浏览器不持久化 token，也不把 token 放入 URL、body 或静态资源请求。可选 control token 只驻留页面内存，当前界面用它提交幂等的执行档位 ID 或受控 Run 创建；Go 决定 Workspace 绑定、默认预算、backend、scope、审批、风险、门禁和全部权限位。创建失败后的 operation key 只在同一内存表单语义下复用，不进入浏览器存储。生产模式由 Go 在同一回环 origin 托管已构建资源；Vite 回环代理只用于前端开发，跨域代理目标会被拒绝。Artifact 正文、checkpoint pending input、操作者引导正文/摘要/身份、外部 Skill 正文/路径/摘要/私有身份、raw Fan-out report、审批/生命周期私有叙述、lease/fencing token、Session message、进程启动和其他执行类写操作仍不向 Web 开放。

同一 bundle 也被 Desktop D0-A/D0-B/D1-R1 编译进 Wails v2.13.0 Windows 壳。React 通过三个方法的严格 native bridge 自动取得内存连接材料并调用进程内 Go API，不监听端口、不使用 Local Storage。Windows AssetServer 不支持 response streaming，因此 Desktop 调用 `run-event-poll.v1`，消费与 SSE 相同的 Run-bound 高水位 cursor 和真实 frame；最多在模块内存保留 16 个 Run、每个 500 帧，浏览器仍使用 SSE。WebView2 在打开 bundle/数据库前只读预检，失败时不下载或安装；精确 renderer origin、外部链接/表单/popup 阻断和 Wails binding-origin allowlist 共同形成导航防线。桌面专属 Skill 按钮只调用无参数原生 `.zip` picker，React 从不接收路径或 bytes，也没有安装按钮。New Run 按钮只在 Go bootstrap 明确返回 `run_creation_enabled=true` 时出现；该 capability 与 profile control 独立。

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

This is the local read-mostly operations UI for CyberAgent Workbench. React/Vite consumes the Go-generated OpenAPI 3.1 DTOs and `api.v1`; it does not reimplement policy, approvals, workspace authorization, Shell, Docker, model routing, or SQLite behavior. Its only visible mutations are schema-v64 non-authorizing Run execution-profile selection and schema-v72 controlled Run creation.

The current UI includes registered-Workspace selection and controlled Run creation, Run and Session browsing, read-only schema v42 three-direction Plan/Delivery state, schema v45 operator-steering counts and ordered metadata after schema v46 controls, a bounded root/Specialist Agent graph, operator-gated delegation summaries, read-only Fan-out plan/execution/shard metadata, Finding/Report projections, bounded pagination, authenticated resumable Run-event SSE, WorkItems, Notes, Artifact descriptors, Supervisor ToolRounds, budgets, leases, compacted message history, schema v64 execution-profile selection, and schema v71 bounded external-Skill provenance/delivery metadata. The Plan, steering, and External Skills panels remain read-only. Both bearers and uncertain-failure creation keys remain in page memory and never enter a URL or browser storage. The optional distinct control bearer is sent only in Authorization for idempotent profile or creation requests; TypeScript submits no backend, scope, gate, approval, or authority field. Raw Fan-out reports, private decision narratives, Artifact content, external-Skill content/paths/digests/private identities, Workspace roots, and lease/fencing identities are omitted from browser DTOs.

For the production-style local path, run `npm run build`, then start `cyberagent api serve --ui-dir web/dist`. Go validates and snapshots the bundle at startup, serves it from the same loopback origin as `api.v1`, applies a strict CSP, disables HTML caching, and gives only hashed assets immutable caching. Static requests are anonymous but reject authorization headers, queries, bodies, and methods other than GET/HEAD. For frontend development, Vite can still proxy same-origin `/api` requests to a loopback Go service and rejects non-loopback targets.

Desktop D0-A/D0-B/D1-R1 compiles the same bundle into a Wails v2.13.0 Windows shell. React obtains ephemeral connection material through a strict three-method native bridge and calls the Go API in process without a listening port or browser storage. Because Windows Wails AssetServer responses do not stream, Desktop consumes `run-event-poll.v1` with the same Run-bound high-water cursor and real frames as SSE; module memory retains at most 16 Runs and 500 frames per Run while browser clients keep SSE. WebView2 is read-only preflighted before the bundle or database opens and no download or installer is started on failure. Exact renderer-origin checks, external link/form/popup blocking, and Wails binding-origin validation protect navigation. The desktop-only Skill button opens a no-argument native `.zip` picker; React receives neither a path nor bytes and exposes no installation action. New Run appears only under the independent Go bootstrap capability and still uses the HTTP control path rather than a native mutation method.

Open `http://127.0.0.1:8765` for the Go-hosted build or `http://127.0.0.1:5173` for Vite development, then enter the same read token shown or configured for the Go process. Enter the distinct control token only when profile selection or Run creation is needed; neither operation can start a process.

## Checks

```powershell
npm run check:api
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```
