# 本地 HTTP API / Local HTTP API

CyberAgent Workbench 提供由 Go 控制的本地只读 `api.v1`。它用于检查 SQLite 中的持久化 Agent 状态，并通过可恢复 SSE 投影持久化 Run events，为后续 TUI 与 TypeScript UI 提供协议基础。当前 API 不执行工具、不修改状态，也不替代 Policy、Approval 或 Tool Gateway。

CyberAgent Workbench exposes a Go-controlled, local, read-only `api.v1`. It inspects durable Agent state in SQLite and projects persisted Run events through resumable SSE for future TUI and TypeScript clients. The current API cannot execute tools or mutate state and does not replace Policy, Approval, or the Tool Gateway.

## 启动 / Start

省略 `CYBERAGENT_API_TOKEN` 时，进程会生成并打印一个临时 token。显式提供 token 时必须是 32 到 512 字节的规范 UTF-8，且不能包含空白或控制字符；CLI 不会回显环境提供的值。

When `CYBERAGENT_API_TOKEN` is absent, the process generates and prints a temporary token. An explicit token must be 32 to 512 bytes of normalized UTF-8 without whitespace or control characters. The CLI does not echo an environment-provided token.

```powershell
$env:CYBERAGENT_API_TOKEN = "<a-random-token-of-at-least-32-bytes>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765
```

The command prints:

```text
api_url: http://127.0.0.1:8765/api/v1
api_version: api.v1
api_token_generated: false
api_token_source: CYBERAGENT_API_TOKEN
note: the API is read-only, loopback-only, and the token is not persisted
```

```powershell
$headers = @{ Authorization = "Bearer $env:CYBERAGENT_API_TOKEN" }
Invoke-RestMethod http://127.0.0.1:8765/api/v1/health -Headers $headers
Invoke-RestMethod "http://127.0.0.1:8765/api/v1/runs?limit=20" -Headers $headers
Invoke-WebRequest http://127.0.0.1:8765/api/v1/openapi.json -Headers $headers
curl.exe -N -H "Authorization: Bearer $env:CYBERAGENT_API_TOKEN" http://127.0.0.1:8765/api/v1/runs/<run-id>/events/stream
```

`Ctrl+C` cancels the command context and performs a bounded graceful shutdown.

## 安全边界 / Security Boundary

- Listener、HTTP `Host` 与客户端地址都必须是 loopback；`0.0.0.0`、空 host 和公网客户端会被拒绝。
- 每个请求必须有且只有一个正确的 `Authorization: Bearer <token>`。
- 只接受无 body 的 `GET`。没有写路由、CORS 响应头或浏览器跨源授权。
- request target 最大 8 KiB，query 最大 4 KiB，response 最大 8 MiB，header 上限为 32 KiB。
- HTTP handler 构造后只保留 token 的 SHA-256 摘要；明文仍可能存在于启动环境或短期进程内存，但不会写入配置、SQLite 或 Run events。
- Artifact API 只返回 descriptor，不读取或返回正文；Run detail 不返回 checkpoint pending input 或 execution fencing token。租约摘要仅包含 owner、generation、状态与时间。
- 一个有效 token 可以读取该进程数据库暴露的全部 API 资源，应把它视为本地管理员凭据。
- SSE 使用同一 Authorization header，token 不进入 URL、cursor 或事件数据。默认最多同时 16 条 stream；每条连接最多 32-event 批量、2 MiB 单帧、10,000 events、5 分钟寿命，并对每次写入设置 2 秒 deadline。

- The listener, HTTP `Host`, and client address must all be loopback. `0.0.0.0`, an empty host, and public clients are rejected.
- Every request must contain exactly one valid `Authorization: Bearer <token>` header.
- Only bodyless `GET` requests are accepted. There are no write routes, CORS response headers, or browser cross-origin grants.
- Request targets are capped at 8 KiB, queries at 4 KiB, responses at 8 MiB, and headers at 32 KiB.
- After construction, the HTTP handler retains only a SHA-256 token digest. Plaintext may still exist in the launch environment or short-lived process memory, but is never written to configuration, SQLite, or Run events.
- Artifact routes return descriptors only and never load content. Run detail omits checkpoint pending input and the execution fencing token; its lease summary contains only owner, generation, status, and timestamps.
- A valid token can read every API resource exposed from the process database and should be treated as a local administrator credential.
- SSE uses the same Authorization header; the token never enters the URL, cursor, or event data. Defaults allow at most 16 concurrent streams, 32 events per batch, 2 MiB per frame, 10,000 events per connection, a five-minute lifetime, and a two-second deadline on each write.

## Endpoints

| Method | Path | Result / Filters |
| --- | --- | --- |
| `GET` | `/api/v1` | API and application versions plus top-level resources |
| `GET` | `/api/v1/health` | Health and SQLite schema version |
| `GET` | `/api/v1/openapi.json` | Raw deterministic OpenAPI 3.1 JSON document |
| `GET` | `/api/v1/runs` | Runs; `status`, `mission_id`, pagination |
| `GET` | `/api/v1/runs/{run_id}` | Run, Mission, checkpoint metadata, tool usage, token-free execution-lease summary |
| `GET` | `/api/v1/runs/{run_id}/events` | Ordered Run events; pagination |
| `GET` | `/api/v1/runs/{run_id}/events/stream` | Bounded SSE projection; opaque `cursor` or `Last-Event-ID` resume |
| `GET` | `/api/v1/runs/{run_id}/work-items` | `status`, `owner`, pagination |
| `GET` | `/api/v1/runs/{run_id}/notes` | `status`, `category`, `visibility`, `owner`, `tag`, `pinned`, pagination |
| `GET` | `/api/v1/runs/{run_id}/artifacts` | Artifact descriptors; `source_id`, `stream`, pagination |
| `GET` | `/api/v1/runs/{run_id}/tool-rounds` | Historical Supervisor tool rounds and redacted calls; pagination |
| `GET` | `/api/v1/sessions` | Sessions; pagination |
| `GET` | `/api/v1/sessions/{session_id}` | Session and optional bound Run |
| `GET` | `/api/v1/sessions/{session_id}/messages` | Messages; `include_compacted`, pagination |
| `GET` | `/api/v1/work-items/{work_item_id}` | WorkItem detail |
| `GET` | `/api/v1/notes/{note_id}` | Note detail |
| `GET` | `/api/v1/artifacts/{artifact_id}` | Artifact descriptor only |

Nested routes verify their parent first. A missing Run or Session returns `NOT_FOUND` rather than an empty child collection. Unknown query fields and repeated singleton fields are rejected.

## OpenAPI Contract

Go DTO 是响应结构的唯一来源。以下命令不启动数据库、不读取 token，并可复现仓库内受测试的 [openapi.json](openapi.json)：

Go DTOs are the single source for response shapes. The following command neither opens the database nor reads an API token, and deterministically reproduces the tested repository [openapi.json](openapi.json):

```powershell
cyberagent api openapi
cyberagent api openapi --output docs/openapi.json
```

运行时的 `/api/v1/openapi.json` 返回同一份原始文档，仍要求 loopback 与 Bearer 认证，不接受 query 或 body。它使用 `application/vnd.oai.openapi+json`，不套普通 `api.v1` envelope。测试会核对生成结果与仓库快照、逐条命中公开 handler、只存在 `GET` operation，并确认契约不包含 Artifact 正文、checkpoint pending input、`lease_id`、fencing token 或 API key 字段。

The runtime `/api/v1/openapi.json` returns the same raw document under the loopback and bearer boundary and accepts neither a query nor a body. It uses `application/vnd.oai.openapi+json` rather than the ordinary `api.v1` envelope. Tests compare generation with the committed snapshot, exercise every published handler, permit only `GET` operations, and verify that the contract omits Artifact content, checkpoint pending input, `lease_id`, fencing tokens, and API-key fields.

## Run Event Stream

SSE endpoint 只读取 append-only `run_events`。首次连接从 sequence 1 开始；每个 `run.event` frame 同时携带持久化 `sequence` 和不透明、与 Run 绑定的 `id`。断线后把最后一个 `id` 放入 `Last-Event-ID` header，或首次请求的 `cursor` query；两者不能同时出现。cursor 只用于定位，不是授权凭据，跨 Run 复用会在发送 SSE headers 前被拒绝。

The SSE endpoint reads only append-only `run_events`. A fresh connection starts at sequence 1. Every `run.event` frame includes the durable sequence and an opaque Run-bound `id`. Reconnect by sending the final id in `Last-Event-ID`, or use the `cursor` query on an initial request; the two cannot be combined. A cursor is positioning data, not authorization, and cross-Run reuse is rejected before SSE headers are committed.

```text
: cyberagent run-events.v1
retry: 1000

id: <opaque-run-bound-cursor>
event: run.event
data: {"version":"run-events.v1","request_id":"req-...","run_id":"run-...","cursor":"...","sequence":42,"event":{...}}

: heartbeat
```

心跳只是 SSE comment，不写入数据库，也不会占用 sequence。达到事件/时间上限或客户端过慢时连接关闭，客户端用最后成功 frame 的 id 恢复。另一个进程写入同一 SQLite 数据库的事件可在下一轮 polling 被观察到；服务器关闭会取消 request context，不等待五分钟连接寿命。stream 复用与 `/events` 完全相同的脱敏 `EventView`，第一版不增加模型正文投影。

Heartbeats are SSE comments and consume neither database rows nor sequences. The connection closes at its event/time limit or when a client misses its write deadline; resume from the last successfully received frame id. Events written by another process to the same SQLite database become visible on a later poll. Server shutdown cancels request contexts instead of waiting for the five-minute lifetime. The stream reuses the same redacted `EventView` as `/events` and adds no user-visible model-text projection.

Native browser `EventSource` cannot attach the current Bearer header. Until the Go process serves a same-origin UI with a separately reviewed browser-auth design, browser clients must use authenticated `fetch` streaming and must never put the token in a query string. CORS remains disabled.

## Envelopes

Except for the raw OpenAPI document and SSE frames, successful responses use one versioned envelope:

```json
{
  "version": "api.v1",
  "request_id": "req-...",
  "data": [],
  "page": {
    "limit": 50,
    "next_cursor": "..."
  }
}
```

Errors never expose internal SQLite details:

```json
{
  "version": "api.v1",
  "request_id": "req-...",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "page limit must be between 1 and 100"
  }
}
```

The `X-Request-ID` header matches `request_id`. Responses also set `Cache-Control: no-store`, a deny-all Content Security Policy, `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, and `X-Frame-Options: DENY`. Stable error meanings and CLI mappings are documented in [errors.md](errors.md).

## Pagination

Collection routes accept `limit` from 1 to 100; the default is 50. `next_cursor` is an opaque, URL-safe cursor bound to the exact route and filter set. A cursor cannot be reused on another endpoint or after changing filters. The Store bounds a cursor window to 100,000 starting rows; if additional data exists beyond that window, `page.truncated` is `true` and no invalid next cursor is emitted.

Clients must not decode, edit, persist indefinitely, or synthesize cursors. Restart pagination from the first page after a filter change or a rejected cursor.

Pagination is a bounded live SQLite projection, not a multi-request snapshot. Append-only event/message order remains stable, but updates to descending activity lists can move rows between requests. Clients that require a fresh consistent view should restart from the first page.

## 当前限制 / Current Limits

- No write API, browser UI, user-visible model-text stream, or cross-process active-call cancellation.
- Execution-lease rows coordinate workers, but the API exposes neither `lease_id` nor any write operation that accepts a fencing token.
- No Artifact content route. Use the authenticated local CLI `artifact read` when content is explicitly required.
- No real Shell, LocalSandbox, or Docker execution. Existing approvals still resolve to audited dry-run results.
- No per-resource authorization below the process token. Future remote or multi-user use requires a separate identity and authorization design.
