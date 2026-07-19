# 本地 HTTP API / Local HTTP API

CyberAgent Workbench 提供由 Go 控制的本地 `api.v1`，用于检查 SQLite 持久状态并投影可恢复 Run events。独立 capability 允许受控 Run/Session/Plan/审批、Provider 诊断/路由/系统凭证、FileEdit 提案/只读恢复/审阅/apply、wake 意图/前台消费、不可变操作者验证，以及惰性 Skill 安装。只读面还提供 capability/worker health、exact-root Repository 状态与脱敏 Diff、非原子的多文件 FileEdit 汇总和可重建 Code Handoff。API 不编辑/重排消息、不执行验证或 Skill、不启动 Git/通用宿主/容器进程，也不替代 Policy、Tool Gateway 或 Sandbox 门禁。

CyberAgent Workbench exposes a Go-controlled local `api.v1` for durable SQLite state and resumable Run-event projections. Independent capabilities permit controlled Run/Session/Plan/approval operations, Provider diagnostics/routes/system credentials, FileEdit propose/read-only recovery/review/apply, wake intent/foreground consumption, immutable operator verification, and inert Skill installation. Read-only surfaces also expose capabilities/worker health, exact-root Repository state and redacted Diffs, non-atomic multi-file FileEdit summaries, and a regenerable Code handoff. The API cannot edit/reorder messages, execute verification or a Skill, start Git or a general host/container process, or replace Policy, the Tool Gateway, or Sandbox gates.

## 启动 / Start

省略 `CYBERAGENT_API_TOKEN` 时，进程会生成并打印一个临时只读 token。全部二十三个 control POST 默认关闭；只有设置不同的 `CYBERAGENT_API_CONTROL_TOKEN` 并启用相应 Go capability 才能访问。两个 token 都必须是 32 到 512 字节的规范 UTF-8，不能包含空白或控制字符，且不能相同；CLI 不会回显环境提供的值。

When `CYBERAGENT_API_TOKEN` is absent, the process generates and prints a temporary read token. All twenty-three control POST operations are disabled by default; access requires both a distinct `CYBERAGENT_API_CONTROL_TOKEN` and the corresponding Go capability. Both tokens must be 32 to 512 bytes of normalized UTF-8 without whitespace or control characters, and they must differ. The CLI never echoes an environment-provided value.

```powershell
$env:CYBERAGENT_API_TOKEN = "<a-random-token-of-at-least-32-bytes>"
$env:CYBERAGENT_API_CONTROL_TOKEN = "<a-different-random-token-of-at-least-32-bytes>"
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765 --ui-dir web/dist

# Optional independent controls completed through D1-G2/V1/F2.
go run ./cmd/cyberagent api serve --listen 127.0.0.1:8765 --ui-dir web/dist --enable-file-edit-proposals --enable-provider-credentials --enable-wake-worker
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

普通 `api serve` 与 Windows Desktop 现在都通过已认证只读
`GET /api/v1/capabilities` 取得 Go 的精确 capability 与 worker health。React 可据此
显示独立控件，但该响应不含 token、owner、lease、Run 或私有错误，也不能启用 worker、
安装服务或授予 mutation。每条控制 route 仍独立要求 control token 和对应 Go gate。

Ordinary `api serve` and Windows Desktop now read exact Go capabilities and worker
health from authenticated `GET /api/v1/capabilities`. React may use that response to
render independent controls, but it contains no token, owner, lease, Run, or private
error and cannot enable a worker, install a service, or grant a mutation. Every control
route still requires the control token and its corresponding Go gate independently.

### Windows Desktop 进程内传输 / Windows Desktop In-Process Transport

Desktop 至 D1-G2/V1/F2 复用同一 `api.v1` Handler，但不调用 `ListenAndServe`，也不绑定回环端口。Wails AssetServer 在同一进程内把 React 请求交给 Go；适配层只接受精确 `http://wails.localhost`。默认只生成内存 read token；十九个独立 flag 开放各自窄 route 或 process-start worker。Repository state/Diff、change-set、Journey 与 Handoff 不增加 flag 或 control route；verification evidence 使用自己的默认关闭 flag。任一 control capability 会生成同一个不同于 read token 的内存 control token，未启用 route 仍返回 404。两个 token 都不写磁盘、日志、Local Storage 或注册表。

Desktop through D1-G2/V1/F2 reuses the same `api.v1` Handler without calling `ListenAndServe` or binding a loopback port. The Wails AssetServer passes React requests to Go in process, and a narrow adapter accepts only exact `http://wails.localhost`. Nineteen independent flags expose narrow routes or the process-start worker. Repository state/Diff, change-set, Journey, and Handoff add no flag or control route; verification evidence has its own default-off flag. Any control capability creates one in-memory control token distinct from the read token, while disabled routes remain 404; neither token is written to disk, logs, browser storage, or the registry.

普通浏览器继续使用 `/events/stream` SSE。Wails v2 在 Windows 上不支持 AssetServer response streaming，因此 Desktop 使用 `GET /runs/{run_id}/events/poll` 做一秒有界轮询。该 endpoint 与 SSE 共用同一个绑定 Run 与 sequence 的高水位 cursor，单次最多返回 100 帧并明确给出 `has_more`；poll cursor 可续接 SSE，SSE cursor 也可续接 poll。Renderer 最多在模块内存保存 16 个 Run、每个 500 帧，重挂载继续最后 cursor，失效 cursor 每次挂载最多回退一次；不写 Local/Session Storage，也不再生成伪 cursor。它不会建立新的事件真源。原生 Wails bridge 不是通用业务 API 旁路：前三项只提供 connection bootstrap 和路径隔离 Skill 选择/预览，第四项只消费 Go 发放的一次性确认句柄并调用惰性 Registry。

Ordinary browser clients keep `/events/stream` SSE. Wails v2 does not support AssetServer response streaming on Windows, so Desktop polls `GET /runs/{run_id}/events/poll` at a bounded one-second interval. That endpoint shares the SSE Run/sequence high-water cursor, returns at most 100 frames plus explicit `has_more`, and permits cursor interchange in both directions. The renderer retains at most 16 Runs and 500 frames per Run in module memory, resumes the last cursor after remount, and resets a stale cursor at most once per mount. It writes neither Local nor Session Storage and no longer invents synthetic cursors. This creates no new event source. The native Wails bridge is not a general business-API bypass: its fourth method accepts only Go's one-time inert-install confirmation handle; Run, Diff, and wake mutations still use the authenticated Go HTTP Handler.

```powershell
$headers = @{ Authorization = "Bearer $env:CYBERAGENT_API_TOKEN" }
Invoke-RestMethod http://127.0.0.1:8765/api/v1/health -Headers $headers
Invoke-RestMethod http://127.0.0.1:8765/api/v1/capabilities -Headers $headers
Invoke-RestMethod "http://127.0.0.1:8765/api/v1/runs?limit=20" -Headers $headers
Invoke-RestMethod "http://127.0.0.1:8765/api/v1/workspaces?limit=20" -Headers $headers
Invoke-WebRequest http://127.0.0.1:8765/api/v1/openapi.json -Headers $headers
curl.exe -N -H "Authorization: Bearer $env:CYBERAGENT_API_TOKEN" http://127.0.0.1:8765/api/v1/runs/<run-id>/events/stream
$controlHeaders = @{ Authorization = "Bearer $env:CYBERAGENT_API_CONTROL_TOKEN"; "Idempotency-Key" = "cancel-<stable-operation-id>" }
$body = @{ attempt_id = "<active-attempt-id>"; model_attempt = 1; reason = "operator stop" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/active-call/cancel -Headers $controlHeaders -ContentType application/json -Body $body
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/agents/<agent-id>/active-call/cancel -Headers $controlHeaders -ContentType application/json -Body $body
$controlHeaders["Idempotency-Key"] = "profile-<stable-operation-id>"
$profileBody = @{ profile = "docker"; reason = "prefer isolated execution" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/execution-profile -Headers $controlHeaders -ContentType application/json -Body $profileBody
$controlHeaders["Idempotency-Key"] = "create-run-<stable-operation-id>"
$createBody = @{ version = "run_creation.v1"; goal = "Implement parser"; workspace_id = "<workspace-id>"; profile = "code"; surface = "code"; phase = "deliver" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs -Headers $controlHeaders -ContentType application/json -Body $createBody
$controlHeaders["Idempotency-Key"] = "session-message-<stable-operation-id>"
$messageBody = @{ version = "session_message_submission.v1"; content = "continue with the reviewed change" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/sessions/<session-id>/messages -Headers $controlHeaders -ContentType application/json -Body $messageBody
$controlHeaders["Idempotency-Key"] = "cancel-message-<stable-operation-id>"
$messageCancelBody = @{ version = "session_steering_cancellation.v1" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/sessions/<session-id>/messages/<message-id>/cancel -Headers $controlHeaders -ContentType application/json -Body $messageCancelBody
$controlHeaders["Idempotency-Key"] = "run-start-<stable-operation-id>"
$lifecycleBody = @{ version = "run_lifecycle_control.v1"; action = "start" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/lifecycle -Headers $controlHeaders -ContentType application/json -Body $lifecycleBody
$controlHeaders["Idempotency-Key"] = "run-execute-<stable-operation-id>"
$executeBody = @{ version = "run_execution_handoff.v1"; max_steps = 4 } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/execute -Headers $controlHeaders -ContentType application/json -Body $executeBody
Invoke-RestMethod http://127.0.0.1:8765/api/v1/models -Headers $headers
$controlHeaders["Idempotency-Key"] = "plan-direction-<stable-operation-id>"
$planBody = @{ version = "plan_delivery_control.v1"; proposal_id = "<proposal-id>"; direction = 2 } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/plan/direction -Headers $controlHeaders -ContentType application/json -Body $planBody
$controlHeaders["Idempotency-Key"] = "plan-deliver-<stable-operation-id>"
$deliverBody = @{ version = "plan_delivery_control.v1" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/plan/deliver -Headers $controlHeaders -ContentType application/json -Body $deliverBody
Invoke-RestMethod http://127.0.0.1:8765/api/v1/runs/<run-id>/approvals -Headers $headers
$controlHeaders["Idempotency-Key"] = "approval-<stable-operation-id>"
$approvalBody = @{ version = "approval_control.v1"; action = "approve_once" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/approvals/<approval-id>/decision -Headers $controlHeaders -ContentType application/json -Body $approvalBody
$routeBody = @{ version = "model_route_control.v1"; provider = "mock"; model = "mock-code" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/models/routes/code -Headers $controlHeaders -ContentType application/json -Body $routeBody
$diagnosticBody = @{ version = "provider_diagnostic.v1"; provider = "mock"; model = "mock-code"; confirm_diagnostic = $true } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/models/diagnostics -Headers $controlHeaders -ContentType application/json -Body $diagnosticBody
Invoke-RestMethod http://127.0.0.1:8765/api/v1/models/credentials -Headers $headers
$credentialBody = @{ version = "provider_credential.v1"; action = "set"; secret = "<ephemeral-provider-key>"; confirm = $true } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/models/credentials/mimo -Headers $controlHeaders -ContentType application/json -Body $credentialBody
$source = Invoke-RestMethod "http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edit-proposal-source?path=README.md" -Headers $headers
# Rotate an expired handle only if the Workspace file still matches the old digest.
$source = Invoke-RestMethod "http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edit-proposal-source?path=README.md&expected_sha256=$($source.data.sha256)" -Headers $headers
$proposalBody = @{ version = "file_edit_proposal.v1"; source_handle = $source.data.source_handle; proposed_text = "replacement text" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edit-proposals -Headers $controlHeaders -ContentType application/json -Body $proposalBody
Invoke-RestMethod http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edits -Headers $headers
Invoke-RestMethod http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edit-change-set -Headers $headers
Invoke-RestMethod http://127.0.0.1:8765/api/v1/workspaces/<workspace-id>/repository-state -Headers $headers
Invoke-RestMethod http://127.0.0.1:8765/api/v1/workspaces/<workspace-id>/repository-diff -Headers $headers
Invoke-RestMethod http://127.0.0.1:8765/api/v1/runs/<run-id>/verification-evidence -Headers $headers
$controlHeaders["Idempotency-Key"] = "verification-<stable-operation-id>"
$verificationBody = @{ version = "operator_verification_evidence.v1"; outcome = "pass"; title = "Focused tests"; summary = "Operator observed the focused test suite passing." } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/verification-evidence -Headers $controlHeaders -ContentType application/json -Body $verificationBody
Invoke-RestMethod http://127.0.0.1:8765/api/v1/runs/<run-id>/code-handoff -Headers $headers
Invoke-RestMethod http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edit-proposal-recovery/<edit-id> -Headers $headers
$controlHeaders["Idempotency-Key"] = "review-edit-<stable-operation-id>"
$reviewBody = @{ version = "file_edit_review.v1"; action = "approve_intent" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edits/<edit-id>/review -Headers $controlHeaders -ContentType application/json -Body $reviewBody
$controlHeaders["Idempotency-Key"] = "apply-edit-<stable-operation-id>"
$applyBody = @{ version = "file_edit_apply.v1" } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/file-edits/<edit-id>/apply -Headers $controlHeaders -ContentType application/json -Body $applyBody
Invoke-RestMethod http://127.0.0.1:8765/api/v1/runs/<run-id>/wake-intent -Headers $headers
$controlHeaders["Idempotency-Key"] = "wake-<stable-operation-id>"
$wakeBody = @{ version = "run_wake_control.v1"; max_attempts = 3; initial_delay_seconds = 0; base_backoff_seconds = 5; max_backoff_seconds = 60; max_elapsed_seconds = 300 } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/wake-intent -Headers $controlHeaders -ContentType application/json -Body $wakeBody
$controlHeaders["Idempotency-Key"] = "consume-wake-<stable-operation-id>"
$consumeBody = @{ version = "run_wake_consumer.v1"; max_steps = 1 } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/runs/<run-id>/wake-intent/consume -Headers $controlHeaders -ContentType application/json -Body $consumeBody
$archive = [Convert]::ToBase64String([IO.File]::ReadAllBytes("<package.zip>"))
$controlHeaders["Idempotency-Key"] = "install-skill-<stable-operation-id>"
$installBody = @{ version = "skill_package_installation.v1"; archive_base64 = $archive; surface = "code"; confirm_untrusted = $true } | ConvertTo-Json
Invoke-RestMethod -Method Post http://127.0.0.1:8765/api/v1/skills/packages/install -Headers $controlHeaders -ContentType application/json -Body $installBody
```

`Ctrl+C` cancels the command context and performs a bounded graceful shutdown.

## 安全边界 / Security Boundary

- Listener、HTTP `Host` 与客户端地址都必须是 loopback；`0.0.0.0`、空 host 和公网客户端会被拒绝。
- 每个 `/api` 请求必须有且只有一个正确的 `Authorization: Bearer <token>`。GET 使用 read token；二十三个控制 POST 只接受不同的 control token，两种凭据不能互换。Web 静态请求匿名可读，并明确拒绝 Authorization header，避免 bearer 被意外发送到资源路径。
- 所有读取只接受无 body 的 `GET`。二十三个 POST 只接受契约列出的精确控制；没有 CORS 响应头或浏览器跨源授权。
- 启用 UI 时，只在非 `/api` 命名空间接受无 query、无 body 的 `GET`/`HEAD`。HTML 使用 `no-store`；仅允许类型且文件名带哈希的资源使用一年 immutable cache。bundle 的根目录、`assets/`、软链接、文件类型、数量、单文件/总大小与 SPA fallback 深度均受限。
- UI 与 API 共享 loopback、Host、客户端地址、request-target 和规范路径校验。UI 响应使用无 `unsafe-inline`/`unsafe-eval` 的 CSP、同源 opener/resource policy、`nosniff`、`DENY` frame policy 和禁用敏感浏览器能力的 Permissions Policy。
- request target 最大 8 KiB，query 最大 4 KiB，response 最大 8 MiB，header 上限为 32 KiB。
- HTTP handler 构造后只保留两个 token 的 SHA-256 摘要；明文仍可能存在于启动环境或短期进程内存，但不会写入配置、SQLite 或 Run events。
- Artifact API 只返回 descriptor，不读取或返回正文；Run detail 不返回 checkpoint pending input 或 execution fencing token。租约摘要仅包含 owner、generation、状态与时间。
- read token 可以读取该进程暴露的全部只读资源；control token 只能调用二十三条窄 mutation，不能读取资源。两者都应视为本地管理员凭据。
- 取消请求必须精确绑定 Run/Supervisor/model attempt，或 Run/Specialist Agent/AgentAttempt/model attempt，并携带 16 到 256 字节的 `Idempotency-Key`。客户端不能提交 `lease_id`、generation 或 fencing token；请求 body 上限为 4 KiB，未知字段和尾随 JSON 会被拒绝。
- Session message 请求必须把 path Session 精确绑定到 running/paused Run，使用 `session_message_submission.v1`、1-16384 UTF-8 字节正文和 16-256 字节幂等键。编码 JSON body 上限为 128 KiB，以容纳合法转义；重复/未知字段、尾随数据、非法 UTF-8、query 和重复 header 均被拒绝。响应不返回正文或私有身份。
- Session 取消必须精确绑定 path Session/消息及其 Run，且仅在消息仍为 pending、未 prepared 时接受。生命周期只接受 `start|pause|resume`；有界执行只接受 `max_steps=1..8`，冻结选择后使用私有 lease。两者的响应都不返回正文、模型输出、工具参数或 lease 身份。
- Plan direction 必须绑定 path Run、已持久化 proposal 和 `direction=1..3`；Deliver 必须已有选择。Provider credential 只接受 exact provider、显式确认、2,560-byte 上限并固定不回传明文；候选 Registry/route/credential 全部成功后才原子推进 generation，失败保留旧 generation。FileEdit source 只发给 exact running Run/active Session 的完整安全 UTF-8，五分钟 handle 只创建 pending proposal；带 `expected_sha256` 的换发必须匹配当前文件，recovery 只返回不可编辑 pending Diff。Verification evidence 只记录脱敏操作者观察，不运行命令，也不构成模型断言、审批或授权。二十三条控制响应都不能携带或设置通用进程、Shell、Docker、Session Grant 或 capability authority；只有独立 FileEdit apply 能写一个已审批且重新复核的精确目标。
- SSE 使用同一 Authorization header，token 不进入 URL、cursor 或事件数据。默认最多同时 16 条 stream；每条连接最多 32-event 批量、2 MiB 单帧、10,000 events、5 分钟寿命，并对每次写入设置 2 秒 deadline。
- Event poll 只接受 query `cursor` 与 1-100 的 `limit`，拒绝 `Last-Event-ID`、跨 Run cursor、gap 和未知参数；空批次仍返回可继续使用的高水位 cursor，读取本身不写事件。

- The listener, HTTP `Host`, and client address must all be loopback. `0.0.0.0`, an empty host, and public clients are rejected.
- Every `/api` request must contain exactly one valid `Authorization: Bearer <token>` header. GET uses the read token; the twenty-three control POST routes accept only the distinct control token. The credentials are not interchangeable. Static Web requests are anonymous and explicitly reject authorization headers so a bearer is not accidentally sent to an asset path.
- All reads accept only bodyless `GET`. The twenty-three POST routes accept only their exact generated contracts. There are no CORS response headers or browser cross-origin grants.
- When the UI is enabled, only queryless, bodyless GET/HEAD requests outside the reserved `/api` namespace reach it. HTML is `no-store`; only allowlisted, hash-named assets receive a one-year immutable cache. Bundle roots, `assets/`, symlinks, types, counts, per-file/aggregate size, and SPA-fallback depth are bounded.
- UI and API requests share the loopback, Host, client-address, request-target, and canonical-path boundary. UI responses add a CSP without `unsafe-inline` or `unsafe-eval`, same-origin opener/resource policies, `nosniff`, frame denial, and a Permissions Policy disabling sensitive browser features.
- Request targets are capped at 8 KiB, queries at 4 KiB, responses at 8 MiB, and headers at 32 KiB.
- After construction, the HTTP handler retains only SHA-256 digests of both tokens. Plaintext may still exist in the launch environment or short-lived process memory, but is never written to configuration, SQLite, or Run events.
- Artifact routes return descriptors only and never load content. Run detail omits checkpoint pending input and the execution fencing token; its lease summary contains only owner, generation, status, and timestamps.
- The read token can inspect every exposed read resource; the control token can invoke only the twenty-three narrow mutations and cannot read resources. Treat both as local administrator credentials.
- Cancellation must bind either the exact Run/Supervisor/model attempt or the exact Run/Specialist Agent/AgentAttempt/model attempt and carry a 16-to-256-byte `Idempotency-Key`. Clients cannot submit a lease id, generation, or fencing token. The JSON body is capped at 4 KiB; unknown fields and trailing JSON are rejected.
- Session-message requests must bind the path Session to an exact running or paused Run and use `session_message_submission.v1`, 1-16384 UTF-8 content bytes, and a 16-to-256-byte idempotency key. The encoded JSON body is capped at 128 KiB to permit valid escaping; duplicate/unknown fields, trailing data, invalid UTF-8, query fields, and duplicate headers are rejected. The response returns neither content nor private identities.
- Session cancellation binds the exact path Session/message and Run and is accepted only while the message is pending and unprepared. Lifecycle accepts only `start|pause|resume`; bounded execution accepts only `max_steps=1..8` and uses a private lease after freezing its selection. Neither response exposes content, model output, tool arguments, or lease identity.
- Plan direction binds the path Run, persisted proposal, and `direction=1..3`; Deliver requires an existing selection. Provider credential control accepts an exact provider, explicit confirmation, and at most 2,560 secret bytes and never returns plaintext; a generation advances atomically only after candidate Registry, routes, and credential reads all succeed, otherwise the old generation remains active. FileEdit source is restricted to complete safe UTF-8 for an exact running Run/active Session; its five-minute handle can create only a pending proposal. Reissue with `expected_sha256` must match the current file, and recovery returns only a non-editable pending Diff. Verification evidence records only a redacted operator observation and neither runs a command nor becomes a model assertion, approval, or grant. Verification association exact-binds one earlier plan item and one later observation but does not infer an aggregate outcome. Control responses grant no general filesystem, process, Shell, Docker, Session-Grant, tool, or capability authority; only the separate apply route may write one exact approved and freshly rechecked file.
- SSE uses the same Authorization header; the token never enters the URL, cursor, or event data. Defaults allow at most 16 concurrent streams, 32 events per batch, 2 MiB per frame, 10,000 events per connection, a five-minute lifetime, and a two-second deadline on each write.
- Event polling accepts only query `cursor` and a 1-100 `limit`; it rejects `Last-Event-ID`, cross-Run cursors, sequence gaps, and unknown parameters. An empty batch still returns a reusable high-water cursor, and polling itself writes no event.

## Endpoints

| Method | Path | Result / Filters |
| --- | --- | --- |
| `GET` | `/api/v1` | API and application versions plus top-level resources |
| `GET` | `/api/v1/health` | Health and SQLite schema version |
| `GET` | `/api/v1/capabilities` | Exact Go capability flags plus metadata-only bounded worker health; no runtime enablement/token/owner/lease/private error |
| `GET` | `/api/v1/openapi.json` | Raw deterministic OpenAPI 3.1 JSON document |
| `GET` | `/api/v1/workspaces` | Bounded Workspace ID/name/creation metadata; no host root path |
| `GET` | `/api/v1/workspaces/{workspace_id}/explore` | One bounded directory level or redacted UTF-8 file evidence; canonical relative path only, no host root |
| `GET` | `/api/v1/workspaces/{workspace_id}/search` | One bounded deterministic filename/redacted-text search; canonical evidence references only, no indexer |
| `GET` | `/api/v1/workspaces/{workspace_id}/repository-state` | Bounded exact-root Git metadata only; no parent discovery, host root, body, remote, process, network, or hook |
| `GET` | `/api/v1/workspaces/{workspace_id}/repository-diff` | At most 50 secret-redacted exact-root patches; 64 KiB each/512 KiB total, no raw body, process, remote, network, hook, or mutation |
| `GET` | `/api/v1/workspaces/{workspace_id}/repository-history` | At most 50 first-parent commits and 64 local branches; no author/email/body/remote/root/process/network/hook |
| `GET` | `/api/v1/workspaces/{workspace_id}/repository-commits/{object_id}` | One exact lowercase SHA-1 commit's bounded changed-path/mode metadata; no blob body, checkout, ref mutation, remote, process, network, or hook |
| `GET` | `/api/v1/workspaces/{workspace_id}/repository-commits/{object_id}/file-preview?path={canonical_path}` | One bounded redacted regular/executable UTF-8 file from the exact commit; no raw blob, root, checkout, mutation, process, network, or hook |
| `GET` | `/api/v1/operation-receipts` | At most 100 terminal metadata-only receipts; optional exact `run_id`, no operation key/path/private lease |
| `GET` | `/api/v1/models` | Redacted Provider/model-route availability with one Registry generation; no key, Base URL, environment name, probe, or model call |
| `GET` | `/api/v1/models/credentials` | Supported Provider system-store and Registry generation status only; fixed `plaintext_returned=false` |
| `POST` | `/api/v1/models/credentials/{provider}` | Explicitly set/delete one Windows system credential; secret is write-only and a successful atomic generation reload needs no restart |
| `POST` | `/api/v1/models/diagnostics` | One explicitly confirmed, content-free, tool-disabled Provider diagnostic |
| `POST` | `/api/v1/models/routes/{profile}` | Persist one validated Provider/model route before updating the in-memory Router |
| `GET` | `/api/v1/runs` | Runs; `status`, `mission_id`, pagination |
| `POST` | `/api/v1/runs` | Idempotently create one closed Mission/Run/Session in a registered Workspace |
| `GET` | `/api/v1/runs/{run_id}` | Run, Mission, immutable execution-mode/profile snapshots, read-only Plan/checkpoint/external-Skill metadata, tool usage, token-free execution-lease summary |
| `GET` | `/api/v1/runs/{run_id}/external-skills` | Bounded external-Skill provenance and root/Specialist delivery counts; no content, paths, digests, or private identities |
| `GET` | `/api/v1/runs/{run_id}/events` | Ordered Run events; pagination |
| `GET` | `/api/v1/runs/{run_id}/events/poll` | Bounded non-streaming event batch; shared Run-bound high-water `cursor` |
| `GET` | `/api/v1/runs/{run_id}/events/stream` | Bounded SSE projection; opaque `cursor` or `Last-Event-ID` resume |
| `GET` | `/api/v1/runs/{run_id}/operator-actions` | At most 100 closed pending steering/approval/FileEdit/due-wake metadata items; no source operation or automatic action |
| `GET` | `/api/v1/runs/{run_id}/agent-graph` | Root/Specialist nodes, budgets, lifecycle, and redacted completion summaries |
| `GET` | `/api/v1/runs/{run_id}/delegations` | Operator-gated proposal/review/application/latest-schedule projection; pagination |
| `GET` | `/api/v1/runs/{run_id}/fanout-plans` | Read-only plan metadata plus latest bounded execution/shard summary; pagination |
| `GET` | `/api/v1/runs/{run_id}/reports` | Finding report metadata and severity counts; pagination |
| `GET` | `/api/v1/runs/{run_id}/reports/{report_id}` | Finding facts, model-assertion provenance, Artifact metadata, and lifecycle timestamps |
| `POST` | `/api/v1/runs/{run_id}/active-call/cancel` | Separately authorized exact active-call cancellation request |
| `POST` | `/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel` | Separately authorized exact Specialist-call cancellation request |
| `POST` | `/api/v1/runs/{run_id}/execution-profile` | Select `preview|docker|local` intent; never starts a process or grants authority |
| `POST` | `/api/v1/runs/{run_id}/lifecycle` | Idempotent `start|pause|resume` under exact state/quiescence/lease gates |
| `POST` | `/api/v1/runs/{run_id}/execute` | Freeze and execute at most eight pending inputs through the existing RunSupervisor |
| `POST` | `/api/v1/runs/{run_id}/plan/direction` | Select one persisted direction and create its bounded WorkItems/Note; no phase change or execution |
| `POST` | `/api/v1/runs/{run_id}/plan/deliver` | Explicitly enter Deliver after selection; no Run resume, model/tool call, or execution |
| `GET` | `/api/v1/runs/{run_id}/approvals` | At most 100 pending metadata records and bounded actions; no command, path, content, fingerprint, or reason |
| `POST` | `/api/v1/runs/{run_id}/approvals/{approval_id}/decision` | Policy-rechecked `approve_once|deny`; no Grant, file write, or real process |
| `GET` | `/api/v1/runs/{run_id}/file-edit-proposal-source` | Five-minute Go-issued handle plus complete safe UTF-8; optional `expected_sha256` permits rotation only while current content still matches |
| `POST` | `/api/v1/runs/{run_id}/file-edit-proposals` | Create one pending FileEdit from a Go-issued handle and proposed text; never writes the file |
| `GET` | `/api/v1/runs/{run_id}/file-edit-proposal-recovery/{edit_id}` | Exact durable pending original/proposed bodies as a read-only Diff; reports stale and returns no source handle or mutation authority |
| `GET` | `/api/v1/runs/{run_id}/file-edits` | At most 100 metadata-only FileEdit previews with bounded redacted Diffs |
| `GET` | `/api/v1/runs/{run_id}/file-edit-change-set` | At most 100 exact-bound FileEdit summaries; per-file authority, partial state, no batch/atomic mutation |
| `GET` | `/api/v1/runs/{run_id}/file-edits/{edit_id}` | One exact metadata-only FileEdit preview; no original/proposed body |
| `POST` | `/api/v1/runs/{run_id}/file-edits/{edit_id}/review` | Exact `approve_intent|deny`; review never writes the file |
| `POST` | `/api/v1/runs/{run_id}/file-edits/{edit_id}/apply` | Separately authorized current-Policy/hash-checked apply; renderer submits no path/body |
| `GET` | `/api/v1/runs/{run_id}/evidence-attachments` | At most 100 metadata-only attachments for the exact Run/Session; fixed false instruction authority, no message body |
| `POST` | `/api/v1/runs/{run_id}/evidence-attachments` | Revalidate and append one exact Workspace file as non-authorizing Session evidence; no execution |
| `GET` | `/api/v1/runs/{run_id}/verification-evidence` | At most 100 immutable operator observations with closed outcomes; no command/model/approval/authority inference |
| `POST` | `/api/v1/runs/{run_id}/verification-evidence` | Record one redacted exact-bound `pass|fail|unknown` operator observation; no verification command or execution |
| `GET` | `/api/v1/runs/{run_id}/verification-plan` | At most 50 immutable guidance-only operator plans and ordered checks; no outcome inference |
| `POST` | `/api/v1/runs/{run_id}/verification-plan` | Record one exact-bound 1-32 item operator checklist; no command/model/execution/authority |
| `GET` | `/api/v1/runs/{run_id}/verification-plan-coverage` | Per-item explicit pass/fail/unknown association counts and unobserved state; no aggregate pass |
| `POST` | `/api/v1/runs/{run_id}/verification-plan-associations` | Immutably associate one later evidence record with one earlier plan item; no reassignment, execution, approval, or inference |
| `GET` | `/api/v1/runs/{run_id}/code-handoff` | Regenerable Code-only Plan/queue/change/verification/coverage/action/report summary; no private body, inferred aggregate result, resume, execution, or composite mutation |
| `GET` | `/api/v1/runs/{run_id}/code-handoff/export` | Digest-bound Markdown/JSON export of one stable Handoff including explicit coverage metadata; no import, result inference, resume, mutation, acceptance, or execution |
| `GET` | `/api/v1/runs/{run_id}/wake-intent` | Bounded public wake state without owner/lease identity |
| `POST` | `/api/v1/runs/{run_id}/wake-intent` | Schedule bounded digest-idempotent wake intent; no execution |
| `POST` | `/api/v1/runs/{run_id}/wake-intent/cancel` | Cancel the exact active intent without running it |
| `POST` | `/api/v1/runs/{run_id}/wake-intent/consume` | Explicitly consume one due intent through the existing bounded RunSupervisor handoff |
| `GET` | `/api/v1/runs/{run_id}/work-items` | `status`, legacy `owner`, `owner_agent_id`, pagination |
| `GET` | `/api/v1/runs/{run_id}/notes` | `status`, `category`, `visibility`, legacy `owner`, `owner_agent_id`, `tag`, `pinned`, pagination |
| `GET` | `/api/v1/runs/{run_id}/artifacts` | Artifact descriptors; `source_id`, `stream`, pagination |
| `GET` | `/api/v1/runs/{run_id}/tool-rounds` | Historical Supervisor tool rounds and redacted calls; pagination |
| `GET` | `/api/v1/sessions` | Sessions; pagination |
| `GET` | `/api/v1/sessions/{session_id}` | Session and optional bound Run |
| `GET` | `/api/v1/sessions/{session_id}/messages` | Messages; `include_compacted`, pagination |
| `POST` | `/api/v1/sessions/{session_id}/messages` | Idempotently queue one bounded message for the exact Run-bound Session; no execution |
| `POST` | `/api/v1/sessions/{session_id}/messages/{message_id}/cancel` | Exact pending-only steering cancellation; prepared/committed items are immutable |
| `POST` | `/api/v1/skills/packages/install` | Confirm and register one bounded untrusted package; no content execution or Run selection |
| `GET` | `/api/v1/work-items/{work_item_id}` | WorkItem detail |
| `GET` | `/api/v1/notes/{note_id}` | Note detail |
| `GET` | `/api/v1/artifacts/{artifact_id}` | Artifact descriptor only |

Nested routes verify their parent first. A missing Run or Session returns `NOT_FOUND` rather than an empty child collection. Unknown query fields and repeated singleton fields are rejected.

Session message DTOs expose schema v43 provenance metadata: `provenance_version`, `source_kind`, optional `source_ref`, `content_sha256`, and `instruction_authorized`. These are read-only audit fields. A client must not infer capability from `role` or source text; only Go-owned control operations can grant or exercise authority. Legacy `context_provenance.v0` rows may have an empty stored digest, while all new v1 rows carry a verified lowercase SHA-256.

Schema v42 Plan/Delivery data remains embedded in Run detail. The API chooses the accepted proposal when a selection exists, otherwise the latest proposal, and returns bounded directions/modules plus selected direction and projected WorkItems. It omits proposal fingerprints, operation digests, requester/root internals, lease identity, and model text. D1-P1 adds two separate control routes: direction selection atomically reuses the existing v42 selection/WorkItem/Note transaction without changing phase, while Deliver transition reuses the existing Run-mode ledger and requires a persisted selection. Neither route starts/resumes execution, calls a model/tool, or grants capability.

Schema v44 adds read-only Delivery fields to the same Run detail: `delivery_gate_enforced`, required and ready checkpoint counts, plus bounded checkpoint IDs, WorkItem/module coordinates, pinned handoff Note IDs, source revisions, boundary status, readiness, and timestamps. The projection omits verification/audit text, handoff content, fingerprints/digests, operation keys, and requester identity. Evidence remains available through the existing authenticated Note detail when an operator follows the handoff Note ID. No HTTP mutation records or approves a checkpoint.

Schema v45 adds required `operator_steering` metadata to Run detail. It reports pending, prepared, committed, and cancelled counts plus a bounded ordered list of message IDs, sequence numbers, statuses, derived `prepared`, and lifecycle timestamps. It omits message content, digests, keys, requester, Session-message IDs, and delivery-attempt identity. D1-S1 adds enqueue/replay through `POST /sessions/{session_id}/messages`; D1-S2 adds exact pending-only cancellation through `POST /sessions/{session_id}/messages/{message_id}/cancel`. A prepared, committed, or cancelled item cannot be changed. Neither route edits, reorders, wakes, or directly delivers steering.

Schema v64 adds required `execution_profile` metadata to Run detail. Its profile enum maps to Go-owned backend, approval, filesystem/network, risk, and required-gate fields; `process_enabled`, `execution_authorized`, and `capability_grant` are always false. Selection requires a `created` or quiescent `paused` Run, the distinct control bearer, strict JSON, and a 16-to-256-byte idempotency key. The browser submits only `profile` and an optional redacted reason; it cannot submit derived controls or authority fields. Stored requester/reason audit fields are omitted from browser DTOs. Selecting Docker or Local neither contacts a runner nor satisfies the corresponding production/OS-sandbox gate.

Schema v71 在 Run detail 中可选嵌入 `external_skills`，并提供同内容的独立只读 endpoint。投影最多包含四个固定版本条目和一个 Specialist，只公开 surface/profile、模式修订、token 上界、信任类别、声明工具数量以及 root/Specialist 准备/提交计数。正文、文件路径、字节数、全部 hash/digest/fingerprint、选择/安装/模式快照 ID、operation key、操作者/请求者/attempt/agent 身份均不进入 DTO。`operator_confirmed` 与 `context_delivery_authorized` 只是历史事实，`tool_capability_grant` 固定为 false；该 endpoint 不安装、选择、加载或执行 Skill。

Schema v71 optionally embeds `external_skills` in Run detail and exposes the same projection through a dedicated read-only endpoint. The projection is bounded to four pinned-version items and one Specialist and reveals only surface/profile, mode revision, token bounds, trust class, declared-tool count, and root/Specialist preparation and commit counts. Content, file paths, byte sizes, every hash/digest/fingerprint, selection/installation/mode-snapshot IDs, operation keys, and operator/requester/attempt/agent identities never enter the DTO. `operator_confirmed` and `context_delivery_authorized` are historical facts only, while `tool_capability_grant` is fixed false; the endpoint cannot install, select, load, or execute a Skill.

Schema v72 新增 `run_creation.v1`。创建请求必须指定已注册 Workspace，并可选择规范 `code|review|learn|script` Profile、`code|cyber` surface 和 `plan|deliver` phase；省略时分别默认为 `code`、`code`、`deliver`。目标原始输入为 1-4096 UTF-8 字节，持久化前脱敏。Go/SQLite 固定交互式 created Run、active Session、默认预算、Profile model route、disabled network、空 targets、revision-one mode 与 `preview/noop` execution profile。相同 operation key 与语义返回原对象并标记 `replayed=true`，改变语义返回 conflict。该 route 不发送消息、调用模型、启动 Run、取得 lease 或授予 capability。

Schema v72 adds `run_creation.v1`. A request must name a registered Workspace and may choose canonical `code|review|learn|script` Profile, `code|cyber` surface, and `plan|deliver` phase; omitted values default to `code`, `code`, and `deliver`. The raw goal is 1-4096 UTF-8 bytes and is redacted before persistence. Go and SQLite fix an interactive created Run, active Session, default budget, Profile model route, disabled network, empty targets, revision-one mode, and `preview/noop` execution profile. Same-key/same-intent replay returns the original graph with `replayed=true`; changed intent conflicts. The route sends no message, calls no model, starts no Run, acquires no lease, and grants no capability.

Schema v73 adds `run_lifecycle_control.v1` and `run_execution_handoff.v1`. Lifecycle uses immutable digest-idempotency facts for strict start/pause/resume and returns the current Run state on delayed replay. Execution freezes up to eight pending identities before acquiring a private lease, then uses the existing RunSupervisor, Policy, budgets, model/tool ledgers, checkpoints, and events. Later appends cannot join the batch; an item cancelled before delivery is skipped; an empty selection completes without a lease or model call. Terminal persistence is fenced to the exact lease generation. Public results include only bounded status/count/model-tool booleans and omit content, outputs, arguments, keys, and lease identity.

D1-M1 adds `model_availability.v1`, built from the same Go Registry and persisted routes used by CLI/Desktop execution. The read projection is deterministic and performs no Provider request. Provider keys, Base URLs, environment-variable names, HTTP clients, and raw configuration errors are structurally absent; secret-like model identifiers are rejected or redacted before projection. D1-A1 adds `approval_queue.v1` and `approval_control.v1`. The queue is bounded to 100 pending records and omits commands, arguments, file paths/content, fingerprints, decision reasons, and operation identities. Approval reloads the exact Run/record/source, rejects terminal pending mutation, rechecks current Policy before approve-once, and permits only dry-run Shell or process-disabled ScriptProcess approval. File replacement can only be denied; permanent Policy denial, Session Grant creation, workspace write, Docker/Shell/Local process execution, and capability grant remain unreachable.

Schema v74 exposes only bounded wake schedule/cancel intent. Schema v75 adds a separate
`run_wake_consumer.v1` POST that claims one due generation and hands at most eight steps
to the existing `run_execution_handoff.v1`; it exposes no private owner/lease identity
and starts no background loop. Schema v76 adds `file_edit_apply.v1`, independently from
review, with exact Run/Workspace/approval/current-Policy/original-and-target-hash checks.
The client submits only protocol version and idempotency key, never a path or file body.
Run-bound legacy approval cannot call this route indirectly.

Non-schema D1-G1 adds read-only `repository_state.v1` at the exact registered
Workspace root. Pure-Go inspection accepts only a real `.git` directory, rejects
parent discovery, redirected worktrees and every nested metadata symlink, caps the
metadata/status/output sets, and returns canonical relative status only. Host roots,
file bodies, remote configuration, subprocesses, network, and hooks are absent.
D1-I3 adds `file_edit_change_set.v1`, a metadata-only summary of at most 100 FileEdits
bound to one Run/Session/Workspace. It fixes review/apply independent, atomic/batch
mutation false, and partial state visible; existing per-file routes remain the only
mutation paths.

D1-B1 `skill_package_installation.v1` accepts one archive of at most 64 KiB encoded as
strict whitespace-free canonical standard base64, an exact Code/Cyber surface, explicit
untrusted confirmation, and a 16-256 byte idempotency key. It returns bounded package
identity and six false authority facts. Import may write the content-addressed Registry
but executes no package content, hook, command, Provider, tool, or network request and
does not select the package for a Run.

D1-U1 adds `operation_receipt.v1` to successful HTTP FileEdit apply, foreground wake
consume, and Skill install responses. The receipt contains a closed operation kind,
outcome, replay flag, retry strategy, recovery action, and cleanup state only. It omits
raw operation keys/digests, path/content, requester, model output, and lease identity.
For FileEdit, uncertain retained staging is reported without changing the durable apply
outcome.

D1-E1 `workspace_explorer.v1` uses the read bearer and registered Workspace identity.
The optional `path` is a canonical slash-separated relative path. Go rejects traversal,
absolute/volume paths, links, redirects, controls, normalization aliases, and ambiguous
names. It scans at most 400 directory entries, returns at most 200, reads at most 64 KiB
of valid UTF-8, and caps the redacted projection at 128 KiB. The response excludes the
host root/internal staging and carries `context_provenance.v1` with
`instruction_authorized=false`.

D1-E2 `workspace_search.v1` searches only those bounded redacted Explorer projections.
It accepts one normalized query of at most 128 Unicode code points and scans at most
128 directories, 1,000 entries, 64 regular files, and 50 results. It follows no links,
creates no persistent index, and returns only canonical relative references, bounded
plain-text snippets, and false-authority provenance.

Schema v77 D1-C1 adds the independently gated `session_evidence_attachment.v1` POST.
The browser submits an exact projected reference/hash and an in-memory idempotency key.
Go reloads the Run/Mission/active Session/registered Workspace, reprojects the file, and
atomically stores one tool-role evidence message, metadata event, and immutable
attachment. Go validation and SQLite both require `instruction_authorized=false`;
document text cannot approve tools, widen Scope, or grant capability. The operation
starts no model, tool, process, or network call.

D1-U2 `operation_receipt_history.v1` derives a newest-first view from terminal FileEdit
apply, foreground wake, and inert Skill installation facts. The optional `run_id` is an
exact filter and `limit` is 1-100. Public IDs are opaque domain-separated hashes; raw
operation identities, paths, content digests, archive details, requesters, and leases
are absent. FileEdit cleanup inspection is read-only and reports uncertainty as
`pending_review` without deleting anything.

D1-O1 `operator_action_center.v1` exact-binds one Run/Mission/Session/Workspace and
returns at most 100 closed metadata items for pending steering, pending approvals,
FileEdit review/apply readiness, and due wake intent. Public IDs are domain-separated
opaque hashes. Source row IDs, messages, commands, arguments, paths, Diffs, operation
identity, requesters, leases, and authority fields are omitted. Listing never approves,
applies, wakes, drains, or executes an item.

D1-C2 `session_evidence_inventory.v1` lists at most 100 immutable attachments for the
exact Run-bound active Session and Workspace. It exposes only the source kind,
canonical relative reference, SHA-256, attachment time, and fixed
`instruction_authorized=false`. Message ID/body, attaching operator, event sequence,
private operation, and capability state remain inside Go/Store. Source navigation must
re-enter the existing Workspace Explorer, which independently revalidates the path.

## OpenAPI Contract

Go DTO 是响应结构的唯一来源。以下命令不启动数据库、不读取 token，并可复现仓库内受测试的 [openapi.json](openapi.json)：

Go DTOs are the single source for response shapes. The following command neither opens the database nor reads an API token, and deterministically reproduces the tested repository [openapi.json](openapi.json):

```powershell
cyberagent api openapi
cyberagent api openapi --output docs/openapi.json
```

运行时的 `/api/v1/openapi.json` 返回同一份原始文档，仍要求 loopback 与 read Bearer 认证，不接受 query 或 body。它使用 `application/vnd.oai.openapi+json`，不套普通 `api.v1` envelope。当前契约有 68 个 path、74 个 operation 和 163 个 schema。测试逐条命中公开 handler，并确认普通 DTO 不包含 Workspace root、Artifact/Skill/Session 正文、模型输出、工具参数、私有 lifecycle、operation/fencing/lease owner、API key、Provider Base URL 或环境变量名。Explorer/Search/Repository Diff/History/Commit Detail 绝不返回 Workspace root；它们只提供有界、脱敏且明确非授权的 Workspace 投影。行动中心、证据清单、验证计划/结果/显式关联 coverage 和 Code Handoff 只提供闭集或有界 metadata，不包含 private operation、message/report body 或 capability。Skill 安装请求仍是唯一有界 archive 传输，并明确排除 path/content/command/hook 字段；证据附件请求只包含相对引用与投影摘要，verification evidence POST 只包含闭集 outcome 与有界文本，association POST 只包含精确 plan/item/evidence identity。

The runtime `/api/v1/openapi.json` returns the same raw document under the loopback and read-bearer boundary and accepts neither a query nor a body. It uses `application/vnd.oai.openapi+json` rather than the ordinary `api.v1` envelope. The contract contains 68 paths, 74 operations, and 163 schemas. Tests exercise every handler and verify that ordinary DTOs omit Workspace roots, Artifact/Skill/Session bodies, model output, tool arguments, private lifecycle, operation/fencing/lease-owner identities, API keys, Provider base URLs, and environment-variable names. Explorer, Search, Repository Diff, History, and Commit Detail never return a Workspace root; they are bounded, redacted, explicitly non-authorizing Workspace projections. Operator actions, evidence inventory, verification plans/outcomes/explicit-association coverage, and Code Handoff expose only closed or bounded metadata without private operations, message/report bodies, or capability fields. The Skill-install request remains the sole bounded archive transport and explicitly omits path, content, command, and hook fields; evidence attachment carries only a relative reference and projected digest, verification evidence POST carries only a closed outcome and bounded text, and association POST carries only exact plan/item/evidence identity.

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

`EventView.version` is the literal `v1` imported from Go's canonical event-envelope
constant into OpenAPI and generated TypeScript. A client parse or transport failure
cancels the response reader before reconnecting, preserving the original error while
preventing abandoned streams from exhausting browser per-origin connection slots.

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

## Repository History, Verification Plans, And Handoff Export

`GET /api/v1/workspaces/{workspace_id}/repository-history` returns
`repository_history.v1`. It accepts no query parameters and reads only the exact
registered Workspace root. The projection is capped at 50 first-parent commits, 64
returned local branches, and 1,024 scanned branch references. It excludes author
identity, email, commit body, remote configuration, host root, subprocess, network,
and hook data. Redirected or linked Git metadata fails closed.

`GET /api/v1/workspaces/{workspace_id}/repository-commits/{object_id}` returns
`repository_commit_detail.v1`. `object_id` must be one exact lowercase 40-character
SHA-1 object ID; symbolic refs and revision expressions are rejected. The pure-Go
reader compares that tree with its first parent under bounded entry/depth/change
limits and returns canonical path plus added/modified/deleted and content/mode-change
metadata. Author/email/body, blob content, remote/root, checkout/ref mutation,
subprocess, network, and hook data remain absent. Missing objects and malformed or
linked metadata fail closed rather than yielding a partial success.

`GET /api/v1/runs/{run_id}/verification-plan` lists at most 50 immutable
operator-authored plans. `POST` on the same path uses the existing distinct verification
control capability, strict `operator_verification_plan.v1` JSON, and an in-memory
`Idempotency-Key`. A request carries only title, summary, and 1-32 title/expected-
observation items. Go and SQLite exact-bind the active Code Session and keep
`guidance_only=true`; command execution, model assertion, result inference, approval,
and authority are always false. Results remain on the separate verification-evidence
route.

`GET /api/v1/workspaces/{workspace_id}/repository-commits/{object_id}/file-preview`
requires exactly one `path` query value. The object must be an exact lowercase SHA-1
identity and the path must be canonical, relative, and present in that commit as a
regular or executable file. Binary, linked, missing, and over-64-KiB files fail closed.
The response is secret-redacted, capped at 128 KiB, and carries a SHA-256 over the
projected UTF-8 bytes plus `instruction_authorized=false`. It never returns the raw
blob or host root and performs no checkout, ref update, process, network, or hook.

`POST /api/v1/runs/{run_id}/verification-plan-associations` records one immutable
`operator_verification_plan_evidence_association.v1`. The request contains only exact
plan, item, and evidence IDs plus an in-memory idempotency key. Go and SQLite require
the same Code Run, active Session, Workspace, an earlier plan item, and one unassociated
later evidence record. It cannot reassign evidence, execute a check, approve an action,
or infer a result. `GET /api/v1/runs/{run_id}/verification-plan-coverage` returns only
bounded per-item pass/fail/unknown counts and unobserved state; contradictory outcomes
remain visible and there is no aggregate pass field.

`GET /api/v1/runs/{run_id}/code-handoff/export?format=markdown|json` returns a
`code_handoff_export.v1` envelope with at most 256 KiB of content, source event
high-water, UTF-8 byte count, SHA-256, safe filename, and fixed MIME type. It uses the
same stable Handoff assembly and cannot resume, apply, accept a report, mutate, or
execute. Handoff and export include at most 100 metadata-only verification coverage
items with explicit pass/fail/unknown counts and contradiction totals. They omit plan
and evidence bodies and never infer an aggregate result. The React client recomputes
the export digest before creating a local download.

`GET /api/v1/workspaces/{workspace_id}/repository-file-history?path=...` requires
exactly one canonical relative `path`. The pure-Go projection starts at current HEAD,
walks first-parent history, scans at most 512 commits, and returns at most 50 commits
where that exact path changed. Each item contains only object/time, a bounded redacted
subject, added/modified/deleted status, previous/current kind, and content/mode-change
flags. It returns no raw blob, patch, identity/body, remote/root, rename inference, or
authority, and performs no checkout, ref mutation, process, network, or hook action.

`GET /api/v1/runs/{run_id}/verification-plan-coverage/{plan_id}/items/{ordinal}` returns
`operator_verification_plan_item_coverage.v1` for one exact Run, immutable plan, and
1-based item ordinal. It returns the item digest/counts and at most 100 exact
association records in descending evidence-event order. Associations contain only
opaque evidence/association IDs, explicit pass/fail/unknown outcome, event sequence,
and time. Private plan/evidence bodies, operator identity, aggregate verdicts,
mutation, command/model execution, approval, and authority remain absent.

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

- No general filesystem mutation, install-time Skill execution, runtime worker enable endpoint, or user-visible model-text stream. One exact approved FileEdit can be applied through its dedicated Go capability; one package can be registered inertly; Windows may store/delete one exact Provider credential without readback; an explicitly started worker may consume one due intent/one step at a time. Steering edit/reorder and host/container process execution remain absent.
- Execution-lease rows coordinate workers, but the API exposes neither `lease_id` nor any operation that accepts a fencing token.
- No Artifact content route. Use the authenticated local CLI `artifact read` when content is explicitly required.
- No real Shell, LocalSandbox, or Docker process execution. Schema v64 profile selection records intent only; HTTP exposes no runner start, Sandbox execution, approval, output-export, or Artifact-commit route. Existing approvals still resolve to audited dry-run results.
- No per-resource authorization below the process token. Future remote or multi-user use requires a separate identity and authorization design.
- Repository history, exact-file history, exact commit detail, and redacted commit-file preview have no checkout/fetch/push/ref-update or raw-blob endpoint. Verification plans, associations, exact-item drilldown, and Handoff coverage do not run checks or imply aggregate outcomes. Handoff exports have no import/resume endpoint.
