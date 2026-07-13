# CyberAgent Web Console

## 中文

这是 CyberAgent Workbench 的本地只读运维界面。React/Vite 只消费 Go 生成的 OpenAPI 3.1 DTO 和 `api.v1`，不会重新实现 Policy、审批、工作区权限、Shell、Docker、模型路由或 SQLite 逻辑。

当前界面提供：

- Run 列表、状态过滤、目标/预算/租约摘要；
- 带 `Last-Event-ID` 恢复的鉴权 SSE 事件流；
- WorkItem、Note、Artifact descriptor 和 Supervisor ToolRound 视图；
- Session 列表、绑定 Run 和包含压缩状态的消息历史；
- 只驻留页面内存的 read bearer token。

浏览器不保存 token，不把 token 放入 URL，也不接触 control token。Vite 将同源 `/api` 代理到回环 Go 服务；跨域目标会被拒绝。Artifact 正文、checkpoint pending input、fencing token 和执行类写操作仍不向 Web 开放。

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

打开 `http://127.0.0.1:5173`，输入同一个 read token。默认代理目标为 `http://127.0.0.1:8765`；需要改端口时设置 `CYBERAGENT_API_TARGET`，该值仍必须是 HTTP(S) 回环 URL。

## English

This is the local read-only operations UI for CyberAgent Workbench. React/Vite consumes the Go-generated OpenAPI 3.1 DTOs and `api.v1`; it does not reimplement policy, approvals, workspace authorization, Shell, Docker, model routing, or SQLite behavior.

The current UI includes Run and Session browsing, bounded pagination, authenticated resumable Run-event SSE, WorkItems, Notes, Artifact descriptors, Supervisor ToolRounds, budgets, leases, and compacted message history. The read bearer remains in page memory, never enters a URL or browser storage, and is distinct from the unavailable control token.

Vite proxies same-origin `/api` requests to the loopback Go service and rejects non-loopback targets. The production bundle is built with `npm run build`; serving that bundle from Go is a later slice, so the current runnable path uses the Vite development server.

## Checks

```powershell
npm run check:api
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```
