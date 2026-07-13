# CyberAgent Web Console

## 中文

这是 CyberAgent Workbench 的本地只读运维界面。React/Vite 只消费 Go 生成的 OpenAPI 3.1 DTO 和 `api.v1`，不会重新实现 Policy、审批、工作区权限、Shell、Docker、模型路由或 SQLite 逻辑。

当前界面提供：

- Run 列表、状态过滤、目标/预算/租约摘要；
- 有界 root/Specialist Agent 图、operator-gated delegation 审查/应用/调度摘要；
- 只读 Fan-out 计划与最新 execution/shard 元数据；
- Finding/Report 严重度、来源、Artifact metadata 与生命周期时间线；
- 带 `Last-Event-ID` 恢复的鉴权 SSE 事件流；
- WorkItem、Note、Artifact descriptor 和 Supervisor ToolRound 视图；
- Session 列表、绑定 Run 和包含压缩状态的消息历史；
- 只驻留页面内存的 read bearer token。

浏览器不保存 token，不把 token 放入 URL，也不接触 control token。生产模式由 Go 在同一回环 origin 托管已构建资源；Vite 回环代理只用于前端开发，跨域代理目标会被拒绝。Artifact 正文、checkpoint pending input、raw Fan-out report、审批/生命周期私有叙述、digest、lease/fencing token 和执行类写操作仍不向 Web 开放。

### 生产式本地运行

```powershell
# repository root
cd web
npm ci
npm run check:api
npm run build
cd ..
$env:CYBERAGENT_API_TOKEN = "<local-read-token>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765 --ui-dir web/dist
```

打开 `http://127.0.0.1:8765`，输入同一个 read token。`--ui-dir` 必须指向 Vite 生产输出；Go 会在启动时校验并读入不可变快照。HTML 不缓存，带哈希的静态资源使用 immutable 缓存，未知资源与不安全 fallback fail closed。静态请求匿名可读，但携带 `Authorization`、query、body 或非 GET/HEAD 方法会被拒绝，避免 bearer 意外发送给资源处理器。

### Vite 开发模式

```powershell
# terminal 1: repository root
$env:CYBERAGENT_API_TOKEN = "<local-read-token>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765

# terminal 2
cd web
npm ci
npm run check:api
npm run dev
```

打开 `http://127.0.0.1:5173`。默认代理目标为 `http://127.0.0.1:8765`；需要改端口时设置 `CYBERAGENT_API_TARGET`，该值仍必须是 HTTP(S) 回环 URL。

## English

This is the local read-only operations UI for CyberAgent Workbench. React/Vite consumes the Go-generated OpenAPI 3.1 DTOs and `api.v1`; it does not reimplement policy, approvals, workspace authorization, Shell, Docker, model routing, or SQLite behavior.

The current UI includes Run and Session browsing, a bounded root/Specialist Agent graph, operator-gated delegation summaries, read-only Fan-out plan/execution/shard metadata, Finding/Report projections, bounded pagination, authenticated resumable Run-event SSE, WorkItems, Notes, Artifact descriptors, Supervisor ToolRounds, budgets, leases, and compacted message history. The read bearer remains in page memory, never enters a URL or browser storage, and is distinct from the unavailable control token. Raw Fan-out reports, private decision narratives, Artifact content, digests, and lease/fencing identities are omitted from browser DTOs.

For the production-style local path, run `npm run build`, then start `cyberagent api serve --ui-dir web/dist`. Go validates and snapshots the bundle at startup, serves it from the same loopback origin as `api.v1`, applies a strict CSP, disables HTML caching, and gives only hashed assets immutable caching. Static requests are anonymous but reject authorization headers, queries, bodies, and methods other than GET/HEAD. For frontend development, Vite can still proxy same-origin `/api` requests to a loopback Go service and rejects non-loopback targets.

Open `http://127.0.0.1:8765` for the Go-hosted build or `http://127.0.0.1:5173` for Vite development, then enter the same read token shown or configured for the Go process.

## Checks

```powershell
npm run check:api
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```
