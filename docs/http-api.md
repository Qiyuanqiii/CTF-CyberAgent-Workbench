# 本地 HTTP API / Local HTTP API

CyberAgent Workbench 提供由 Go 控制的本地只读 `api.v1`。它用于检查 SQLite 中的持久化 Agent 状态，并为后续 TUI、WebSocket 与 TypeScript UI 提供协议基础。当前 API 不执行工具、不修改状态，也不替代 Policy、Approval 或 Tool Gateway。

CyberAgent Workbench exposes a Go-controlled, local, read-only `api.v1`. It inspects durable Agent state in SQLite and establishes the protocol foundation for future TUI, WebSocket, and TypeScript clients. The current API cannot execute tools or mutate state and does not replace Policy, Approval, or the Tool Gateway.

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

- The listener, HTTP `Host`, and client address must all be loopback. `0.0.0.0`, an empty host, and public clients are rejected.
- Every request must contain exactly one valid `Authorization: Bearer <token>` header.
- Only bodyless `GET` requests are accepted. There are no write routes, CORS response headers, or browser cross-origin grants.
- Request targets are capped at 8 KiB, queries at 4 KiB, responses at 8 MiB, and headers at 32 KiB.
- After construction, the HTTP handler retains only a SHA-256 token digest. Plaintext may still exist in the launch environment or short-lived process memory, but is never written to configuration, SQLite, or Run events.
- Artifact routes return descriptors only and never load content. Run detail omits checkpoint pending input and the execution fencing token; its lease summary contains only owner, generation, status, and timestamps.
- A valid token can read every API resource exposed from the process database and should be treated as a local administrator credential.

## Endpoints

| Method | Path | Result / Filters |
| --- | --- | --- |
| `GET` | `/api/v1` | API and application versions plus top-level resources |
| `GET` | `/api/v1/health` | Health and SQLite schema version |
| `GET` | `/api/v1/runs` | Runs; `status`, `mission_id`, pagination |
| `GET` | `/api/v1/runs/{run_id}` | Run, Mission, checkpoint metadata, tool usage, token-free execution-lease summary |
| `GET` | `/api/v1/runs/{run_id}/events` | Ordered Run events; pagination |
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

## Envelopes

All successful responses use one versioned envelope:

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

- No write API, WebSocket event stream, OpenAPI document, browser UI, or cross-process active-call cancellation.
- Execution-lease rows coordinate workers, but the API exposes neither `lease_id` nor any write operation that accepts a fencing token.
- No Artifact content route. Use the authenticated local CLI `artifact read` when content is explicitly required.
- No real Shell, LocalSandbox, or Docker execution. Existing approvals still resolve to audited dry-run results.
- No per-resource authorization below the process token. Future remote or multi-user use requires a separate identity and authorization design.
