# ADR 0037: Controlled Session Message Submission

Status: Accepted, 2026-07-18

## Context

Desktop D1-R1 can create a closed Mission/Run/Session graph, while schema v45-v46 already provides a durable, redacted, idempotent operator-steering queue and explicit local drain. The missing product boundary was a narrow way for the browser and Windows Desktop to submit ordinary input to an existing Run-bound Session without creating a second execution engine or exposing model, lease, tool, or process authority to TypeScript.

中文结论：D1-S1 只开放“持久化排队”，不开放“执行”。文件、模型输出和网页仍是证据；只有操作者通过独立 control capability 提交的正文进入既有 Go 队列。提交不会自动启动、恢复或 drain Run，也不会调用模型、工具、Shell、Docker 或网络。

## Decision

1. Go owns immutable protocol `session_message_submission.v1`. `SessionMessageSubmissionService` accepts one canonical Session ID, bounded UTF-8 content, one 16-256-byte idempotency key, and one trusted server-side requester identity.
2. The service reloads the Session and exact bound Run before calling the existing `EnqueueOperatorSteering`. It creates no new queue, table, event source, model path, execution lease, or schema migration. Database schema remains v72.
3. `POST /api/v1/sessions/{session_id}/messages` is independently gated by `SessionMessageEnabled` and the distinct control bearer. It accepts no query, requires exact JSON content type and exactly one `Idempotency-Key`, rejects duplicate or unknown fields, invalid UTF-8, trailing JSON, and oversized bodies.
4. The response is metadata-only: protocol version, Run ID, Session ID, bounded steering lifecycle metadata, replay state, and explicit false `execution_started`, `model_called`, `tool_called`, and `capability_grant`. Content, digest, operation key, requester, internal delivery identity, lease, and fencing fields cannot be represented.
5. Desktop exposes the capability only under explicit `--enable-session-messages`. It is independent from `--enable-profile-control` and `--enable-run-creation`; the native Wails bridge remains exactly three methods, and submission still traverses the authenticated in-process Go HTTP Handler.
6. React renders a Session composer only when Go bootstrap grants the capability, the Session is bound to the selected Run, and the Run is `running` or `paused`. It enforces the 16 KiB UTF-8 client preflight, while Go remains authoritative. An uncertain failure retains the same idempotency key only in module/component memory for unchanged content; editing creates a new retry identity, success clears both, and browser storage remains untouched.
7. UI feedback displays only queue sequence/status and never echoes submitted content from the response. Submission invalidates Run/Session projections so the existing metadata queue remains the durable visible result.

## Authority Boundary

- A successful response means only that one operator message is durably queued or replayed.
- It does not start a `created` Run, resume a paused Run, acquire or transfer an execution lease, drain steering, call a Provider, execute a tool, approve an action, grant a Session capability, or start a host/container process.
- Existing v45-v46 state, ordering, redaction, terminal closure, replay, and pending/prepared/committed/cancelled rules remain authoritative.
- Cancellation, reordering, editing, background wake-up, and automatic delivery are outside this decision.
- A capability-only Desktop launch cannot reach Run creation or execution-profile control. Disabled mutation routes return 404.

## Verification

The three-slice D1-S1 batch covers Application redaction and binding, cross-restart replay/conflict, strict HTTP headers/JSON/UTF-8/body bounds and exact response shape, independent Desktop capability isolation, same-SQLite in-process submission, React retry identity and storage absence, multibyte limits, and fail-closed rendering.

The integrated functional gate passed a direct full ordinary Go run on final code in 255.6 seconds, focused Desktop-tag tests in 80.5 seconds, a Windows production Desktop build, 52 frontend tests across 15 files, strict TypeScript, and the Vite production build. Under the agreed delivery cadence, the complete race/staticcheck/govulncheck robustness gate runs after the next three-slice batch reaches six slices.

## Consequences

Users can now add durable input from Web/Desktop to a currently running or paused Run-bound Session. A newly created D1-R1 Run still needs a separate Go-owned lifecycle transition before it can accept this path, and queued input still needs explicit existing drain/execution behavior. This deliberate split keeps TypeScript as an interface rather than an execution or safety boundary.

The next batch should add separately bounded operator controls rather than silently coupling them to submission: pending-message cancellation, idempotent Run lifecycle transitions, and a Go-owned execution handoff. The sixth-slice gate must include full ordinary/race tests, vet, staticcheck, vulnerability scanning, dependency checks, contract drift, privacy scans, and functional build verification.
