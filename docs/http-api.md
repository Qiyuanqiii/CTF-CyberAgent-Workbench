# 本地 HTTP API / Local HTTP API

CyberAgent Workbench 提供由 Go 控制的本地 `api.v1`，用于检查 SQLite 持久状态并投影可恢复 Run events。独立 control capability 允许精确取消活动模型调用、选择不授予执行权的 Run 档位、schema-v72 受控 Run 创建、Session 消息排队/pending-only 取消，以及 schema-v73 Run start/pause/resume 和最多八条冻结输入的显式 RunSupervisor 交接。API 仍不选择 Plan 方向、不写 Delivery 检查点、不编辑/重排消息、不安装 Skill、不启动宿主或容器进程，也不替代 Policy、Approval、Tool Gateway 或 Sandbox 门禁。

CyberAgent Workbench exposes a Go-controlled local `api.v1` for durable SQLite state and resumable Run-event projections. The distinct control capability permits exact active-call cancellation, non-authorizing profile selection, schema-v72 controlled Run creation, Session enqueue/pending-only cancellation, and schema-v73 Run start/pause/resume plus an explicit at-most-eight-item frozen RunSupervisor handoff. The API still cannot choose a Plan direction, write a Delivery checkpoint, edit/reorder messages, install a Skill, start a host/container process, or replace Policy, Approval, the Tool Gateway, or Sandbox gates.

## 启动 / Start

省略 `CYBERAGENT_API_TOKEN` 时，进程会生成并打印一个临时只读 token。全部八个 control POST 默认关闭；只有设置不同的 `CYBERAGENT_API_CONTROL_TOKEN` 才在普通 `api serve` 中启用。两个 token 都必须是 32 到 512 字节的规范 UTF-8，不能包含空白或控制字符，且不能相同；CLI 不会回显环境提供的值。

When `CYBERAGENT_API_TOKEN` is absent, the process generates and prints a temporary read token. All eight control POST operations are disabled by default and enabled for ordinary `api serve` only by a distinct `CYBERAGENT_API_CONTROL_TOKEN`. Both tokens must be 32 to 512 bytes of normalized UTF-8 without whitespace or control characters, and they must differ. The CLI never echoes an environment-provided value.

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

### Windows Desktop 进程内传输 / Windows Desktop In-Process Transport

Desktop D0-A 至 D1-X1 复用同一 `api.v1` Handler，但不调用 `ListenAndServe`，也不绑定回环端口。Wails AssetServer 在同一进程内把 React 请求交给 Go；适配层只接受精确 `http://wails.localhost`。默认只生成内存 read token；六个独立 flag 分别开放 profile、Run 创建、Session 排队、Session 取消、Run 生命周期和有界执行 route。任一 capability 会生成同一个不同于 read token 的内存 control token，未启用 route 仍返回 404。两个 token 都不写磁盘、日志、Local Storage 或注册表。

Desktop D0-A through D1-X1 reuses the same `api.v1` Handler without calling `ListenAndServe` or binding a loopback port. The Wails AssetServer passes React requests to Go in process, and a narrow adapter accepts only exact `http://wails.localhost`. Six independent flags expose profile, Run creation, Session enqueue, Session cancellation, Run lifecycle, and bounded execution routes. Any enabled capability creates one in-memory control token distinct from the read token, while disabled routes remain 404. Neither token is written to disk, logs, browser storage, or the registry.

普通浏览器继续使用 `/events/stream` SSE。Wails v2 在 Windows 上不支持 AssetServer response streaming，因此 Desktop 使用 `GET /runs/{run_id}/events/poll` 做一秒有界轮询。该 endpoint 与 SSE 共用同一个绑定 Run 与 sequence 的高水位 cursor，单次最多返回 100 帧并明确给出 `has_more`；poll cursor 可续接 SSE，SSE cursor 也可续接 poll。Renderer 最多在模块内存保存 16 个 Run、每个 500 帧，重挂载继续最后 cursor，失效 cursor 每次挂载最多回退一次；不写 Local/Session Storage，也不再生成伪 cursor。它不会建立新的事件真源。原生 Wails bridge 不是 HTTP 旁路，只提供 connection bootstrap 和路径隔离 Skill 选择/预览，不提供业务 mutation。

Ordinary browser clients keep `/events/stream` SSE. Wails v2 does not support AssetServer response streaming on Windows, so Desktop polls `GET /runs/{run_id}/events/poll` at a bounded one-second interval. That endpoint shares the SSE Run/sequence high-water cursor, returns at most 100 frames plus explicit `has_more`, and permits cursor interchange in both directions. The renderer retains at most 16 Runs and 500 frames per Run in module memory, resumes the last cursor after remount, and resets a stale cursor at most once per mount. It writes neither Local nor Session Storage and no longer invents synthetic cursors. This creates no new event source. The native Wails bridge is not a business-API bypass: it provides only connection bootstrap and pathless Skill selection/preview; Run creation still goes through the authenticated Go HTTP Handler.

```powershell
$headers = @{ Authorization = "Bearer $env:CYBERAGENT_API_TOKEN" }
Invoke-RestMethod http://127.0.0.1:8765/api/v1/health -Headers $headers
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
```

`Ctrl+C` cancels the command context and performs a bounded graceful shutdown.

## 安全边界 / Security Boundary

- Listener、HTTP `Host` 与客户端地址都必须是 loopback；`0.0.0.0`、空 host 和公网客户端会被拒绝。
- 每个 `/api` 请求必须有且只有一个正确的 `Authorization: Bearer <token>`。GET 使用 read token；八个控制 POST 只接受不同的 control token，两种凭据不能互换。Web 静态请求匿名可读，并明确拒绝 Authorization header，避免 bearer 被意外发送到资源路径。
- 所有读取只接受无 body 的 `GET`。八个 POST 只接受精确的模型取消、非授权档位、关闭 Run 创建、Session 排队/pending 取消、Run 生命周期或有界交接意图；没有 CORS 响应头或浏览器跨源授权。
- 启用 UI 时，只在非 `/api` 命名空间接受无 query、无 body 的 `GET`/`HEAD`。HTML 使用 `no-store`；仅允许类型且文件名带哈希的资源使用一年 immutable cache。bundle 的根目录、`assets/`、软链接、文件类型、数量、单文件/总大小与 SPA fallback 深度均受限。
- UI 与 API 共享 loopback、Host、客户端地址、request-target 和规范路径校验。UI 响应使用无 `unsafe-inline`/`unsafe-eval` 的 CSP、同源 opener/resource policy、`nosniff`、`DENY` frame policy 和禁用敏感浏览器能力的 Permissions Policy。
- request target 最大 8 KiB，query 最大 4 KiB，response 最大 8 MiB，header 上限为 32 KiB。
- HTTP handler 构造后只保留两个 token 的 SHA-256 摘要；明文仍可能存在于启动环境或短期进程内存，但不会写入配置、SQLite 或 Run events。
- Artifact API 只返回 descriptor，不读取或返回正文；Run detail 不返回 checkpoint pending input 或 execution fencing token。租约摘要仅包含 owner、generation、状态与时间。
- read token 可以读取该进程数据库暴露的全部只读资源；control token 只能调用八条窄 mutation，不能读取资源。两者都应视为本地管理员凭据。
- 取消请求必须精确绑定 Run/Supervisor/model attempt，或 Run/Specialist Agent/AgentAttempt/model attempt，并携带 16 到 256 字节的 `Idempotency-Key`。客户端不能提交 `lease_id`、generation 或 fencing token；请求 body 上限为 4 KiB，未知字段和尾随 JSON 会被拒绝。
- Session message 请求必须把 path Session 精确绑定到 running/paused Run，使用 `session_message_submission.v1`、1-16384 UTF-8 字节正文和 16-256 字节幂等键。编码 JSON body 上限为 128 KiB，以容纳合法转义；重复/未知字段、尾随数据、非法 UTF-8、query 和重复 header 均被拒绝。响应不返回正文或私有身份。
- Session 取消必须精确绑定 path Session/消息及其 Run，且仅在消息仍为 pending、未 prepared 时接受。生命周期只接受 `start|pause|resume`；有界执行只接受 `max_steps=1..8`，冻结选择后使用私有 lease。两者的响应都不返回正文、模型输出、工具参数或 lease 身份。
- SSE 使用同一 Authorization header，token 不进入 URL、cursor 或事件数据。默认最多同时 16 条 stream；每条连接最多 32-event 批量、2 MiB 单帧、10,000 events、5 分钟寿命，并对每次写入设置 2 秒 deadline。
- Event poll 只接受 query `cursor` 与 1-100 的 `limit`，拒绝 `Last-Event-ID`、跨 Run cursor、gap 和未知参数；空批次仍返回可继续使用的高水位 cursor，读取本身不写事件。

- The listener, HTTP `Host`, and client address must all be loopback. `0.0.0.0`, an empty host, and public clients are rejected.
- Every `/api` request must contain exactly one valid `Authorization: Bearer <token>` header. GET uses the read token; the eight control POST routes accept only the distinct control token. The credentials are not interchangeable. Static Web requests are anonymous and explicitly reject authorization headers so a bearer is not accidentally sent to an asset path.
- All reads accept only bodyless `GET`. The eight POST routes accept only exact model cancellation, non-authorizing profile, closed Run creation, Session enqueue/pending cancellation, Run lifecycle, or bounded handoff intent. There are no CORS response headers or browser cross-origin grants.
- When the UI is enabled, only queryless, bodyless GET/HEAD requests outside the reserved `/api` namespace reach it. HTML is `no-store`; only allowlisted, hash-named assets receive a one-year immutable cache. Bundle roots, `assets/`, symlinks, types, counts, per-file/aggregate size, and SPA-fallback depth are bounded.
- UI and API requests share the loopback, Host, client-address, request-target, and canonical-path boundary. UI responses add a CSP without `unsafe-inline` or `unsafe-eval`, same-origin opener/resource policies, `nosniff`, frame denial, and a Permissions Policy disabling sensitive browser features.
- Request targets are capped at 8 KiB, queries at 4 KiB, responses at 8 MiB, and headers at 32 KiB.
- After construction, the HTTP handler retains only SHA-256 digests of both tokens. Plaintext may still exist in the launch environment or short-lived process memory, but is never written to configuration, SQLite, or Run events.
- Artifact routes return descriptors only and never load content. Run detail omits checkpoint pending input and the execution fencing token; its lease summary contains only owner, generation, status, and timestamps.
- The read token can inspect every exposed read resource; the control token can invoke only the eight narrow mutations and cannot read resources. Treat both as local administrator credentials.
- Cancellation must bind either the exact Run/Supervisor/model attempt or the exact Run/Specialist Agent/AgentAttempt/model attempt and carry a 16-to-256-byte `Idempotency-Key`. Clients cannot submit a lease id, generation, or fencing token. The JSON body is capped at 4 KiB; unknown fields and trailing JSON are rejected.
- Session-message requests must bind the path Session to an exact running or paused Run and use `session_message_submission.v1`, 1-16384 UTF-8 content bytes, and a 16-to-256-byte idempotency key. The encoded JSON body is capped at 128 KiB to permit valid escaping; duplicate/unknown fields, trailing data, invalid UTF-8, query fields, and duplicate headers are rejected. The response returns neither content nor private identities.
- Session cancellation binds the exact path Session/message and Run and is accepted only while the message is pending and unprepared. Lifecycle accepts only `start|pause|resume`; bounded execution accepts only `max_steps=1..8` and uses a private lease after freezing its selection. Neither response exposes content, model output, tool arguments, or lease identity.
- SSE uses the same Authorization header; the token never enters the URL, cursor, or event data. Defaults allow at most 16 concurrent streams, 32 events per batch, 2 MiB per frame, 10,000 events per connection, a five-minute lifetime, and a two-second deadline on each write.
- Event polling accepts only query `cursor` and a 1-100 `limit`; it rejects `Last-Event-ID`, cross-Run cursors, sequence gaps, and unknown parameters. An empty batch still returns a reusable high-water cursor, and polling itself writes no event.

## Endpoints

| Method | Path | Result / Filters |
| --- | --- | --- |
| `GET` | `/api/v1` | API and application versions plus top-level resources |
| `GET` | `/api/v1/health` | Health and SQLite schema version |
| `GET` | `/api/v1/openapi.json` | Raw deterministic OpenAPI 3.1 JSON document |
| `GET` | `/api/v1/workspaces` | Bounded Workspace ID/name/creation metadata; no host root path |
| `GET` | `/api/v1/runs` | Runs; `status`, `mission_id`, pagination |
| `POST` | `/api/v1/runs` | Idempotently create one closed Mission/Run/Session in a registered Workspace |
| `GET` | `/api/v1/runs/{run_id}` | Run, Mission, immutable execution-mode/profile snapshots, read-only Plan/checkpoint/external-Skill metadata, tool usage, token-free execution-lease summary |
| `GET` | `/api/v1/runs/{run_id}/external-skills` | Bounded external-Skill provenance and root/Specialist delivery counts; no content, paths, digests, or private identities |
| `GET` | `/api/v1/runs/{run_id}/events` | Ordered Run events; pagination |
| `GET` | `/api/v1/runs/{run_id}/events/poll` | Bounded non-streaming event batch; shared Run-bound high-water `cursor` |
| `GET` | `/api/v1/runs/{run_id}/events/stream` | Bounded SSE projection; opaque `cursor` or `Last-Event-ID` resume |
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
| `GET` | `/api/v1/runs/{run_id}/work-items` | `status`, legacy `owner`, `owner_agent_id`, pagination |
| `GET` | `/api/v1/runs/{run_id}/notes` | `status`, `category`, `visibility`, legacy `owner`, `owner_agent_id`, `tag`, `pinned`, pagination |
| `GET` | `/api/v1/runs/{run_id}/artifacts` | Artifact descriptors; `source_id`, `stream`, pagination |
| `GET` | `/api/v1/runs/{run_id}/tool-rounds` | Historical Supervisor tool rounds and redacted calls; pagination |
| `GET` | `/api/v1/sessions` | Sessions; pagination |
| `GET` | `/api/v1/sessions/{session_id}` | Session and optional bound Run |
| `GET` | `/api/v1/sessions/{session_id}/messages` | Messages; `include_compacted`, pagination |
| `POST` | `/api/v1/sessions/{session_id}/messages` | Idempotently queue one bounded message for the exact Run-bound Session; no execution |
| `POST` | `/api/v1/sessions/{session_id}/messages/{message_id}/cancel` | Exact pending-only steering cancellation; prepared/committed items are immutable |
| `GET` | `/api/v1/work-items/{work_item_id}` | WorkItem detail |
| `GET` | `/api/v1/notes/{note_id}` | Note detail |
| `GET` | `/api/v1/artifacts/{artifact_id}` | Artifact descriptor only |

Nested routes verify their parent first. A missing Run or Session returns `NOT_FOUND` rather than an empty child collection. Unknown query fields and repeated singleton fields are rejected.

Session message DTOs expose schema v43 provenance metadata: `provenance_version`, `source_kind`, optional `source_ref`, `content_sha256`, and `instruction_authorized`. These are read-only audit fields. A client must not infer capability from `role` or source text; only Go-owned control operations can grant or exercise authority. Legacy `context_provenance.v0` rows may have an empty stored digest, while all new v1 rows carry a verified lowercase SHA-256.

Schema v42 Plan/Delivery data is embedded only in Run detail. The API chooses the accepted proposal when a selection exists, otherwise the latest proposal, and returns bounded directions/modules plus selected direction and projected WorkItems. It omits proposal fingerprints, operation digests, requester/root internals, lease identity, and model text. `operator_choice_needed`, `phase_change_needed`, and `capability_grant=false` are display facts only; no HTTP route can choose a direction or move the Run into Deliver.

Schema v44 adds read-only Delivery fields to the same Run detail: `delivery_gate_enforced`, required and ready checkpoint counts, plus bounded checkpoint IDs, WorkItem/module coordinates, pinned handoff Note IDs, source revisions, boundary status, readiness, and timestamps. The projection omits verification/audit text, handoff content, fingerprints/digests, operation keys, and requester identity. Evidence remains available through the existing authenticated Note detail when an operator follows the handoff Note ID. No HTTP mutation records or approves a checkpoint.

Schema v45 adds required `operator_steering` metadata to Run detail. It reports pending, prepared, committed, and cancelled counts plus a bounded ordered list of message IDs, sequence numbers, statuses, derived `prepared`, and lifecycle timestamps. It omits message content, digests, keys, requester, Session-message IDs, and delivery-attempt identity. D1-S1 adds enqueue/replay through `POST /sessions/{session_id}/messages`; D1-S2 adds exact pending-only cancellation through `POST /sessions/{session_id}/messages/{message_id}/cancel`. A prepared, committed, or cancelled item cannot be changed. Neither route edits, reorders, wakes, or directly delivers steering.

Schema v64 adds required `execution_profile` metadata to Run detail. Its profile enum maps to Go-owned backend, approval, filesystem/network, risk, and required-gate fields; `process_enabled`, `execution_authorized`, and `capability_grant` are always false. Selection requires a `created` or quiescent `paused` Run, the distinct control bearer, strict JSON, and a 16-to-256-byte idempotency key. The browser submits only `profile` and an optional redacted reason; it cannot submit derived controls or authority fields. Stored requester/reason audit fields are omitted from browser DTOs. Selecting Docker or Local neither contacts a runner nor satisfies the corresponding production/OS-sandbox gate.

Schema v71 在 Run detail 中可选嵌入 `external_skills`，并提供同内容的独立只读 endpoint。投影最多包含四个固定版本条目和一个 Specialist，只公开 surface/profile、模式修订、token 上界、信任类别、声明工具数量以及 root/Specialist 准备/提交计数。正文、文件路径、字节数、全部 hash/digest/fingerprint、选择/安装/模式快照 ID、operation key、操作者/请求者/attempt/agent 身份均不进入 DTO。`operator_confirmed` 与 `context_delivery_authorized` 只是历史事实，`tool_capability_grant` 固定为 false；该 endpoint 不安装、选择、加载或执行 Skill。

Schema v71 optionally embeds `external_skills` in Run detail and exposes the same projection through a dedicated read-only endpoint. The projection is bounded to four pinned-version items and one Specialist and reveals only surface/profile, mode revision, token bounds, trust class, declared-tool count, and root/Specialist preparation and commit counts. Content, file paths, byte sizes, every hash/digest/fingerprint, selection/installation/mode-snapshot IDs, operation keys, and operator/requester/attempt/agent identities never enter the DTO. `operator_confirmed` and `context_delivery_authorized` are historical facts only, while `tool_capability_grant` is fixed false; the endpoint cannot install, select, load, or execute a Skill.

Schema v72 新增 `run_creation.v1`。创建请求必须指定已注册 Workspace，并可选择规范 `code|review|learn|script` Profile、`code|cyber` surface 和 `plan|deliver` phase；省略时分别默认为 `code`、`code`、`deliver`。目标原始输入为 1-4096 UTF-8 字节，持久化前脱敏。Go/SQLite 固定交互式 created Run、active Session、默认预算、Profile model route、disabled network、空 targets、revision-one mode 与 `preview/noop` execution profile。相同 operation key 与语义返回原对象并标记 `replayed=true`，改变语义返回 conflict。该 route 不发送消息、调用模型、启动 Run、取得 lease 或授予 capability。

Schema v72 adds `run_creation.v1`. A request must name a registered Workspace and may choose canonical `code|review|learn|script` Profile, `code|cyber` surface, and `plan|deliver` phase; omitted values default to `code`, `code`, and `deliver`. The raw goal is 1-4096 UTF-8 bytes and is redacted before persistence. Go and SQLite fix an interactive created Run, active Session, default budget, Profile model route, disabled network, empty targets, revision-one mode, and `preview/noop` execution profile. Same-key/same-intent replay returns the original graph with `replayed=true`; changed intent conflicts. The route sends no message, calls no model, starts no Run, acquires no lease, and grants no capability.

Schema v73 adds `run_lifecycle_control.v1` and `run_execution_handoff.v1`. Lifecycle uses immutable digest-idempotency facts for strict start/pause/resume and returns the current Run state on delayed replay. Execution freezes up to eight pending identities before acquiring a private lease, then uses the existing RunSupervisor, Policy, budgets, model/tool ledgers, checkpoints, and events. Later appends cannot join the batch; an item cancelled before delivery is skipped; an empty selection completes without a lease or model call. Terminal persistence is fenced to the exact lease generation. Public results include only bounded status/count/model-tool booleans and omit content, outputs, arguments, keys, and lease identity.

## OpenAPI Contract

Go DTO 是响应结构的唯一来源。以下命令不启动数据库、不读取 token，并可复现仓库内受测试的 [openapi.json](openapi.json)：

Go DTOs are the single source for response shapes. The following command neither opens the database nor reads an API token, and deterministically reproduces the tested repository [openapi.json](openapi.json):

```powershell
cyberagent api openapi
cyberagent api openapi --output docs/openapi.json
```

运行时的 `/api/v1/openapi.json` 返回同一份原始文档，仍要求 loopback 与 read Bearer 认证，不接受 query 或 body。它使用 `application/vnd.oai.openapi+json`，不套普通 `api.v1` envelope。当前契约有 31 个 path、73 个 schema：25 个只读 GET 使用全局 read capability，八个控制 POST 显式覆盖为 `ControlBearerAuth`。测试逐条命中公开 handler，并确认契约不包含 Workspace root、Artifact/Skill/Session 正文、模型输出、工具参数、私有 lifecycle、operation/fencing/lease、摘要或 API key 字段。

The runtime `/api/v1/openapi.json` returns the same raw document under the loopback and read-bearer boundary and accepts neither a query nor a body. It uses `application/vnd.oai.openapi+json` rather than the ordinary `api.v1` envelope. The contract contains 31 paths and 73 schemas: 25 read-only GET operations use the global read capability, while eight control POST operations override security with `ControlBearerAuth`. Tests exercise every handler and verify that the contract omits Workspace roots, Artifact/Skill/Session content, model output, tool arguments, private lifecycle, operation/fencing/lease identities, digests, and API-key fields.

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

- No general file/Plan/approval/Diff/Skill write API or user-visible model-text stream. The Go-hosted browser UI has narrow controls for non-authorizing profiles, closed Run creation, Session enqueue/pending cancellation, Run lifecycle, and bounded Supervisor handoff; exact active-call cancellation remains API-only. Session response text, background wake/retry, steering edit/reorder, and process execution remain absent.
- Execution-lease rows coordinate workers, but the API exposes neither `lease_id` nor any operation that accepts a fencing token.
- No Artifact content route. Use the authenticated local CLI `artifact read` when content is explicitly required.
- No real Shell, LocalSandbox, or Docker process execution. Schema v64 profile selection records intent only; HTTP exposes no runner start, Sandbox execution, approval, output-export, or Artifact-commit route. Existing approvals still resolve to audited dry-run results.
- No per-resource authorization below the process token. Future remote or multi-user use requires a separate identity and authorization design.
