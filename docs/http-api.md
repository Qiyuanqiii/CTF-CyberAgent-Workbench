# 本地 HTTP API / Local HTTP API

CyberAgent Workbench 提供由 Go 控制的本地 `api.v1`。它主要用于检查 SQLite 中的持久化 Agent 状态（包括 schema v41 Run 模式快照与 schema v42 Plan/Delivery 只读状态），并通过可恢复 SSE 投影 Run events；唯一控制操作是经过独立授权、审计优先的活动模型调用取消。API 不选择 Plan 方向、不执行工具、不切换执行阶段，也不替代 Policy、Approval 或 Tool Gateway。

CyberAgent Workbench exposes a Go-controlled local `api.v1`. It primarily inspects durable Agent state in SQLite, including the schema v41 Run-mode snapshot and schema v42 read-only Plan/Delivery state, and projects persisted Run events through resumable SSE. Its only control operation is separately authorized, audit-first cancellation of an active model call. The API cannot select a Plan direction, execute tools, or change execution phase and does not replace Policy, Approval, or the Tool Gateway.

## 启动 / Start

省略 `CYBERAGENT_API_TOKEN` 时，进程会生成并打印一个临时只读 token。取消控制默认关闭；只有设置不同的 `CYBERAGENT_API_CONTROL_TOKEN` 才启用。两个 token 都必须是 32 到 512 字节的规范 UTF-8，不能包含空白或控制字符，且不能相同；CLI 不会回显环境提供的值。

When `CYBERAGENT_API_TOKEN` is absent, the process generates and prints a temporary read token. Cancellation control is disabled by default and is enabled only by a distinct `CYBERAGENT_API_CONTROL_TOKEN`. Both tokens must be 32 to 512 bytes of normalized UTF-8 without whitespace or control characters, and they must differ. The CLI never echoes an environment-provided value.

```powershell
$env:CYBERAGENT_API_TOKEN = "<a-random-token-of-at-least-32-bytes>"
$env:CYBERAGENT_API_CONTROL_TOKEN = "<a-different-random-token-of-at-least-32-bytes>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765 --ui-dir web/dist
```

The command prints:

```text
api_url: http://127.0.0.1:8765/api/v1
api_version: api.v1
api_token_generated: false
api_control_enabled: true
ui_url: http://127.0.0.1:8765/
ui_source: <absolute-path-to-web/dist>
ui_assets: 2
ui_digest: <sha256-bundle-digest>
api_token_source: CYBERAGENT_API_TOKEN
api_control_token_source: CYBERAGENT_API_CONTROL_TOKEN
note: the API is loopback-only; control is separately authorized and tokens are not persisted
```

`--ui-dir` 可选。设置后，Go 会在打开数据库和 listener 前校验 Vite bundle，并把 `index.html` 与 `assets/` 读成不可变内存快照；运行期间磁盘变化不会改变已服务内容。省略该选项时，根路径继续走原有 API 404/鉴权行为，不启动 Web UI。

`--ui-dir` is optional. When set, Go validates the Vite bundle before opening the database or listener and loads `index.html` plus `assets/` into an immutable in-memory snapshot. On-disk changes cannot alter the served process. Without the option, root paths retain the existing authenticated API/404 behavior and no Web UI is enabled.

```powershell
$headers = @{ Authorization = "Bearer $env:CYBERAGENT_API_TOKEN" }
Invoke-RestMethod http://127.0.0.1:8765/api/v1/health -Headers $headers
Invoke-RestMethod "http://127.0.0.1:8765/api/v1/runs?limit=20" -Headers $headers
Invoke-WebRequest http://127.0.0.1:8765/api/v1/openapi.json -Headers $headers
curl.exe -N -H "Authorization: Bearer $env:CYBERAGENT_API_TOKEN" http://127.0.0.1:8765/api/v1/runs/<run-id>/events/stream
$controlHeaders = @{ Authorization = "Bearer $env:CYBERAGENT_API_CONTROL_TOKEN"; "Idempotency-Key" = "cancel-<stable-operation-id>" }
$body = @{ attempt_id = "<active-attempt-id>"; model_attempt = 1; reason = "operator stop" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/active-call/cancel -Headers $controlHeaders -ContentType application/json -Body $body
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/agents/<agent-id>/active-call/cancel -Headers $controlHeaders -ContentType application/json -Body $body
```

`Ctrl+C` cancels the command context and performs a bounded graceful shutdown.

## 安全边界 / Security Boundary

- Listener、HTTP `Host` 与客户端地址都必须是 loopback；`0.0.0.0`、空 host 和公网客户端会被拒绝。
- 每个 `/api` 请求必须有且只有一个正确的 `Authorization: Bearer <token>`。GET 使用 read token；取消 POST 只接受不同的 control token，两种凭据不能互换。Web 静态请求匿名可读，并明确拒绝 Authorization header，避免 bearer 被意外发送到资源路径。
- 所有读取只接受无 body 的 `GET`。两个 POST 只写入精确的 root 或 Specialist 取消意图；没有 CORS 响应头或浏览器跨源授权。
- 启用 UI 时，只在非 `/api` 命名空间接受无 query、无 body 的 `GET`/`HEAD`。HTML 使用 `no-store`；仅允许类型且文件名带哈希的资源使用一年 immutable cache。bundle 的根目录、`assets/`、软链接、文件类型、数量、单文件/总大小与 SPA fallback 深度均受限。
- UI 与 API 共享 loopback、Host、客户端地址、request-target 和规范路径校验。UI 响应使用无 `unsafe-inline`/`unsafe-eval` 的 CSP、同源 opener/resource policy、`nosniff`、`DENY` frame policy 和禁用敏感浏览器能力的 Permissions Policy。
- request target 最大 8 KiB，query 最大 4 KiB，response 最大 8 MiB，header 上限为 32 KiB。
- HTTP handler 构造后只保留两个 token 的 SHA-256 摘要；明文仍可能存在于启动环境或短期进程内存，但不会写入配置、SQLite 或 Run events。
- Artifact API 只返回 descriptor，不读取或返回正文；Run detail 不返回 checkpoint pending input 或 execution fencing token。租约摘要仅包含 owner、generation、状态与时间。
- read token 可以读取该进程数据库暴露的全部只读资源；control token 只能请求取消，不能读取资源。两者都应视为本地管理员凭据。
- 取消请求必须精确绑定 Run/Supervisor/model attempt，或 Run/Specialist Agent/AgentAttempt/model attempt，并携带 16 到 256 字节的 `Idempotency-Key`。客户端不能提交 `lease_id`、generation 或 fencing token；请求 body 上限为 4 KiB，未知字段和尾随 JSON 会被拒绝。
- SSE 使用同一 Authorization header，token 不进入 URL、cursor 或事件数据。默认最多同时 16 条 stream；每条连接最多 32-event 批量、2 MiB 单帧、10,000 events、5 分钟寿命，并对每次写入设置 2 秒 deadline。

- The listener, HTTP `Host`, and client address must all be loopback. `0.0.0.0`, an empty host, and public clients are rejected.
- Every `/api` request must contain exactly one valid `Authorization: Bearer <token>` header. GET uses the read token; cancellation POST accepts only the distinct control token. The credentials are not interchangeable. Static Web requests are anonymous and explicitly reject authorization headers so a bearer is not accidentally sent to an asset path.
- All reads accept only bodyless `GET`. The two POST routes only record exact root or Specialist cancellation intent. There are no CORS response headers or browser cross-origin grants.
- When the UI is enabled, only queryless, bodyless GET/HEAD requests outside the reserved `/api` namespace reach it. HTML is `no-store`; only allowlisted, hash-named assets receive a one-year immutable cache. Bundle roots, `assets/`, symlinks, types, counts, per-file/aggregate size, and SPA-fallback depth are bounded.
- UI and API requests share the loopback, Host, client-address, request-target, and canonical-path boundary. UI responses add a CSP without `unsafe-inline` or `unsafe-eval`, same-origin opener/resource policies, `nosniff`, frame denial, and a Permissions Policy disabling sensitive browser features.
- Request targets are capped at 8 KiB, queries at 4 KiB, responses at 8 MiB, and headers at 32 KiB.
- After construction, the HTTP handler retains only SHA-256 digests of both tokens. Plaintext may still exist in the launch environment or short-lived process memory, but is never written to configuration, SQLite, or Run events.
- Artifact routes return descriptors only and never load content. Run detail omits checkpoint pending input and the execution fencing token; its lease summary contains only owner, generation, status, and timestamps.
- The read token can inspect every exposed read resource; the control token can only request cancellation and cannot read resources. Treat both as local administrator credentials.
- Cancellation must bind either the exact Run/Supervisor/model attempt or the exact Run/Specialist Agent/AgentAttempt/model attempt and carry a 16-to-256-byte `Idempotency-Key`. Clients cannot submit a lease id, generation, or fencing token. The JSON body is capped at 4 KiB; unknown fields and trailing JSON are rejected.
- SSE uses the same Authorization header; the token never enters the URL, cursor, or event data. Defaults allow at most 16 concurrent streams, 32 events per batch, 2 MiB per frame, 10,000 events per connection, a five-minute lifetime, and a two-second deadline on each write.

## Endpoints

| Method | Path | Result / Filters |
| --- | --- | --- |
| `GET` | `/api/v1` | API and application versions plus top-level resources |
| `GET` | `/api/v1/health` | Health and SQLite schema version |
| `GET` | `/api/v1/openapi.json` | Raw deterministic OpenAPI 3.1 JSON document |
| `GET` | `/api/v1/runs` | Runs; `status`, `mission_id`, pagination |
| `GET` | `/api/v1/runs/{run_id}` | Run, Mission, immutable execution-mode snapshot, read-only Plan proposal/selection, checkpoint metadata, tool usage, token-free execution-lease summary |
| `GET` | `/api/v1/runs/{run_id}/events` | Ordered Run events; pagination |
| `GET` | `/api/v1/runs/{run_id}/events/stream` | Bounded SSE projection; opaque `cursor` or `Last-Event-ID` resume |
| `GET` | `/api/v1/runs/{run_id}/agent-graph` | Root/Specialist nodes, budgets, lifecycle, and redacted completion summaries |
| `GET` | `/api/v1/runs/{run_id}/delegations` | Operator-gated proposal/review/application/latest-schedule projection; pagination |
| `GET` | `/api/v1/runs/{run_id}/fanout-plans` | Read-only plan metadata plus latest bounded execution/shard summary; pagination |
| `GET` | `/api/v1/runs/{run_id}/reports` | Finding report metadata and severity counts; pagination |
| `GET` | `/api/v1/runs/{run_id}/reports/{report_id}` | Finding facts, model-assertion provenance, Artifact metadata, and lifecycle timestamps |
| `POST` | `/api/v1/runs/{run_id}/active-call/cancel` | Separately authorized exact active-call cancellation request |
| `POST` | `/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel` | Separately authorized exact Specialist-call cancellation request |
| `GET` | `/api/v1/runs/{run_id}/work-items` | `status`, legacy `owner`, `owner_agent_id`, pagination |
| `GET` | `/api/v1/runs/{run_id}/notes` | `status`, `category`, `visibility`, legacy `owner`, `owner_agent_id`, `tag`, `pinned`, pagination |
| `GET` | `/api/v1/runs/{run_id}/artifacts` | Artifact descriptors; `source_id`, `stream`, pagination |
| `GET` | `/api/v1/runs/{run_id}/tool-rounds` | Historical Supervisor tool rounds and redacted calls; pagination |
| `GET` | `/api/v1/sessions` | Sessions; pagination |
| `GET` | `/api/v1/sessions/{session_id}` | Session and optional bound Run |
| `GET` | `/api/v1/sessions/{session_id}/messages` | Messages; `include_compacted`, pagination |
| `GET` | `/api/v1/work-items/{work_item_id}` | WorkItem detail |
| `GET` | `/api/v1/notes/{note_id}` | Note detail |
| `GET` | `/api/v1/artifacts/{artifact_id}` | Artifact descriptor only |

Nested routes verify their parent first. A missing Run or Session returns `NOT_FOUND` rather than an empty child collection. Unknown query fields and repeated singleton fields are rejected.

Schema v42 Plan/Delivery data is embedded only in Run detail. The API chooses the accepted proposal when a selection exists, otherwise the latest proposal, and returns bounded directions/modules plus selected direction and projected WorkItems. It omits proposal fingerprints, operation digests, requester/root internals, lease identity, and model text. `operator_choice_needed`, `phase_change_needed`, and `capability_grant=false` are display facts only; no HTTP route can choose a direction or move the Run into Deliver.

## OpenAPI Contract

Go DTO 是响应结构的唯一来源。以下命令不启动数据库、不读取 token，并可复现仓库内受测试的 [openapi.json](openapi.json)：

Go DTOs are the single source for response shapes. The following command neither opens the database nor reads an API token, and deterministically reproduces the tested repository [openapi.json](openapi.json):

```powershell
cyberagent api openapi
cyberagent api openapi --output docs/openapi.json
```

运行时的 `/api/v1/openapi.json` 返回同一份原始文档，仍要求 loopback 与 read Bearer 认证，不接受 query 或 body。它使用 `application/vnd.oai.openapi+json`，不套普通 `api.v1` envelope。当前契约有 24 个 path、52 个 schema：22 个只读 GET 使用全局 read capability，两个精确取消 POST 显式覆盖为 `ControlBearerAuth`。测试会逐条命中公开 handler，并确认契约不包含 Artifact 正文、checkpoint pending input、raw Fan-out report、私有审批/生命周期叙述、Plan operation/fencing 身份、`lease_id`、摘要或 API key 字段。

The runtime `/api/v1/openapi.json` returns the same raw document under the loopback and read-bearer boundary and accepts neither a query nor a body. It uses `application/vnd.oai.openapi+json` rather than the ordinary `api.v1` envelope. The contract currently contains 24 paths and 52 schemas: 22 read-only GET operations use the global read capability, while two exact-cancellation POST operations override security with `ControlBearerAuth`. Tests exercise every published handler and verify that the contract omits Artifact content, checkpoint pending input, raw Fan-out reports, private approval/lifecycle narratives, Plan operation/fencing identity, `lease_id`, digests, and API-key fields.

## 主动取消 / Active-Call Cancellation

取消入口写入 schema v18 的 `run_model_cancellations` 与一对一幂等操作账本。首次有效请求返回 `202 Accepted`；相同 key 与相同意图重放原对象，不同意图复用 key 或为同一目标换 key 返回 `409 CONFLICT`。请求只有在 Run 正在运行、execution lease 活跃、Supervisor attempt 完全匹配、目标是最新且尚未终止的 model attempt 时才被接受。

The cancellation route writes schema v18 `run_model_cancellations` plus a one-to-one idempotency-operation ledger. The first valid request returns `202 Accepted`. Replaying the same key and intent returns the original object; changed intent under that key or a different key for the same target returns `409 CONFLICT`. A request is accepted only while the Run is running, its execution lease is active, the Supervisor attempt matches exactly, and the target is the latest non-terminal model attempt.

持有私有 lease 的 worker 每 100 ms 检查当前调用对应的 pending 请求。观察动作事务校验 checkpoint fencing，写入 `model.cancel_observed`，随后才取消进程内 Provider context。模型终态与请求的 `resolved` 状态原子提交；若 worker 崩溃且后续 attempt 接管，旧请求会变为 `resolved/superseded`，绝不会作用到新调用。客户端只能在 SSE/事件中观察进展，不能获得或提交内部 lease token。

The worker holding the private lease checks for a pending request for its current call every 100 ms. Observation transactionally validates checkpoint fencing, appends `model.cancel_observed`, and only then cancels the in-process Provider context. The model terminal event and the request's `resolved` state commit atomically. If a worker crashes and a later attempt takes over, the old request resolves as `superseded` and can never affect the new call. Clients observe progress through SSE/events and can neither obtain nor submit the internal lease token.

schema v29 的 Specialist 路由写入独立的 `specialist_model_cancellations` 与 digest-only operation ledger，不复用按 Run 唯一键控的 root registry。路径中的 `agent_id` 与 body 中的 `attempt_id/model_attempt` 必须精确匹配当前 direct Specialist、running AgentAttempt、最新 started child model call 和活动 Run lease。child worker 先提交 `model.cancel_observed`，再取消该调用自己的 Go context；模型终态与 resolution 原子提交，Attempt crash/interruption/takeover 会将遗留请求解析为 `attempt_terminated` 或 `worker_lost`。响应不包含 reason/requester、内部 subject、模型正文或 fencing 字段。

The schema v29 Specialist route writes a separate `specialist_model_cancellations` table and digest-only operation ledger rather than reusing the Run-keyed root registry. The path `agent_id` and body `attempt_id/model_attempt` must exactly match the current direct Specialist, running AgentAttempt, latest started child model call, and active Run lease. The child worker commits `model.cancel_observed` before cancelling that call's own Go context. Model terminal state and resolution commit atomically, while Attempt crash, interruption, or takeover resolves leftovers as `attempt_terminated` or `worker_lost`. Responses omit reason/requester, internal subjects, model text, and fencing fields.

控制意图只命中所选 child。若该 child 运行在 `SpecialistScheduler` 的并发 round 中，它随后返回的取消错误会触发既有的首错 fan-out，scheduler 可能本地取消同轮 sibling 以保持 round 一致性；这不会为 sibling 创建第二条远程取消请求，也不会扩大 admission、spawn 或工具权限。

The persisted intent targets only the selected child. If that child belongs to a concurrent `SpecialistScheduler` round, its resulting cancellation error activates the existing first-error fan-out and may locally cancel the sibling to preserve round consistency. No second remote request is fabricated, and admission, spawn, and tool authority remain unchanged.

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

Native browser `EventSource` cannot attach the current Bearer header. The React/Vite console therefore uses authenticated `fetch` streaming and never puts the token in a query string or browser storage. Production assets and `api.v1` now share the Go loopback origin when `--ui-dir` is set; Vite provides the same-origin proxy only during development. CORS remains disabled.

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

- No general write API, Web control action, or user-visible model-text stream. The Go-hosted browser UI is read-only; the only API write capability remains exact active-call cancellation under a separate token that the UI does not accept.
- Execution-lease rows coordinate workers, but the API exposes neither `lease_id` nor any operation that accepts a fencing token.
- No Artifact content route. Use the authenticated local CLI `artifact read` when content is explicitly required.
- No real Shell, LocalSandbox, or Docker execution. Existing approvals still resolve to audited dry-run results.
- No per-resource authorization below the process token. Future remote or multi-user use requires a separate identity and authorization design.
