# ADR 0035: Desktop Lifecycle And Event Resumption Hardening

## Status

Accepted as non-schema Desktop D0-B. SQLite remains at schema v71. This slice
hardens the existing read-first Windows shell; it does not add an installer,
business mutation, terminal, Runner, or process-execution capability.

## 中文摘要

Desktop D0-B 把桌面数据库与进程内 API 的所有权收敛到可测试的 Go
`desktop.ControlPlane`，并把窗口启动、停止和第二实例恢复收敛到可取消的
`desktop.Lifecycle`。第二实例的参数、工作目录和环境不会进入主实例；停止会等待
正在进行的原生窗口恢复完成，返回后生命周期永久静默。

桌面事件续传不再把普通分页结果伪装成事件流。Go 新增只读
`GET /api/v1/runs/{run_id}/events/poll`，它与 SSE 共用同一个 Run-bound 高水位
cursor 和真实 `run-events.v1` frame。React 仅在进程内存保留最多 16 个 Run、每个
500 帧；组件重挂载从最后确认 cursor 恢复，失效 cursor 每次挂载最多回退一次，
不会写入 Local Storage 或 Session Storage。

WebView2 在 bundle 和数据库打开前执行只读版本预检。缺失、过旧或探测失败均以
稳定 `FAILED_PRECONDITION` 关闭；应用不下载、不安装、不打开 URL，也不接受
renderer 提供的替代 Runtime。进程内适配器只接受精确
`http://wails.localhost`，clone 后清除 URL origin、规范化 `RequestURI` 并固定
loopback 身份。渲染层另以 defense-in-depth 阻止外部链接、表单和 popup；真正的
原生绑定权限仍由 Wails start-origin allowlist 决定。

## Context

ADR 0034 established the embedded read-first shell, but its first event adapter
rebuilt stream-like frames from ordinary paginated events, and lifecycle logic
was coupled to process startup. D0-B needs one durable cursor across Web, Desktop,
process restart, and concurrent SQLite clients, plus explicit prerequisite and
origin failure behavior before any business mutation is considered.

Wails v2 AssetServer responses do not stream on Windows. A bounded polling
contract is therefore required, but it must consume the same append-only event
sequence as SSE rather than introduce a renderer-only offset or event source.

## Decision

`desktop.OpenControlPlane` opens the existing SQLite Store and constructs the
existing `httpapi.API` in process. It owns no listener and adds no authority. Its
idempotent close operation is the sole Desktop Store shutdown boundary. Tests
open the same database through independent CLI and Desktop connections, perform
concurrent writes, close and reopen Desktop, and resume from the previously
confirmed cursor.

`desktop.Lifecycle` owns a cancellable Wails context. Restore requests arriving
before startup are coalesced into one action. Active requests only unminimise
and show the existing window. A dedicated restore mutex serializes native calls
with shutdown, so `Stop` cannot return while a native restore remains active and
no stopped lifecycle can restart. The Wails second-instance callback discards
all supplied launch data before requesting this action.

`run-event-poll.v1` returns at most 100 frames and one final opaque cursor. Every
frame is the actual `run-events.v1` projection with the envelope request ID,
persisted event sequence, and Run-bound cursor. Query fields are allowlisted;
duplicate fields, invalid limits, cross-Run cursors, and `Last-Event-ID` are
rejected. The Store reads `limit+1` rows to derive `has_more`, and Go validates
that the returned batch is contiguous and belongs to the requested Run. A poll
cursor can resume SSE and an SSE cursor can resume polling.

The Desktop React hook uses only this endpoint. A bounded module-memory LRU
retains the final confirmed cursor and visible frames for at most 16 Runs and
500 frames per Run. It survives component remount but not process exit. It
performs at most one invalid-cursor reset per mount, checks cancellation after
every awaited response, and never synthesizes cursors or writes browser storage.

Windows builds require `desktop,production,wv2runtime.error`. Omitting the
explicit WebView2-error capability tag leaves the entry point unbuildable. The
startup preflight accepts WebView2 Runtime `94.0.992.31` or newer and otherwise
returns a bounded, path-free native message. All Wails download/install prompt
fields are empty; dependency remediation remains an operator action through a
trusted Windows software channel.

The in-process request adapter accepts only the exact Wails renderer origin,
rejecting alternate schemes, hosts, ports, user info, fragments, and opaque
URLs. It never trusts the source Host or RemoteAddr, never forwards an absolute
or mismatched `RequestURI`, and does not mutate the source request. A Desktop-only
renderer guard blocks external link, form, and popup navigation without changing
ordinary browser behavior. This guard is defense in depth; Wails binding-origin
validation and the Go API remain the authority boundary.

## Capability Boundary

D0-B adds no:

- Run creation, Session chat, Plan selection, queue, approval, Diff, or Skill
  installation mutation;
- LocalRunner, Docker start/exec, Shell, terminal, unrestricted `os/exec`, or
  Agent-controlled host process;
- API-key read/write, Provider call, external network Scope, or secret access;
- browser-persisted token, path/byte upload, registry value, startup entry,
  background service, updater, installer, or signed release; or
- schema migration, new authorization fact, execution lease, or CTF capability.

## Validation

Go regressions cover no-gap paging, empty high-water polls, cross-connection
visibility, poll-to-SSE resume, cursor/query/auth failures, same-database
CLI/Desktop writes, close/reopen, six simultaneous control-plane opens,
idempotent close, early/active/stopped restore, cancellation, concurrent restore
and stop, and native-restore shutdown serialization. Windows adapter tests cover
WebView2 missing/old/probe failures, path-free guidance, ignored second-instance
data, exact origin rejection, source immutability, canonical `RequestURI`, and
the two observed Wails request shapes.

TypeScript regressions cover exact poll validation, forged final-cursor
rejection, no-gap paging, remount recovery, one-time stale-cursor reset, bounded
memory, absence of browser storage, cancellation, and Desktop-only navigation
blocking. Ordinary browser SSE and navigation remain unchanged.

A secure unsigned GUI was built and smoke-tested on Windows 11 Pro x64
10.0.26200 with an isolated `CYBERAGENT_HOME`. The primary process stayed live,
a second process exited successfully and yielded to it, forced termination left
the SQLite database reopenable, and no Desktop process remained afterward.
Windows 10 real-machine validation remains a D0 release-matrix item and is not
claimed by this ADR.

The completed local gate includes 256.6-second ordinary and 273.5-second race
suites, ordinary and secure-Desktop vet/staticcheck/govulncheck, deterministic
OpenAPI-to-TypeScript generation, strict TypeScript, 37 frontend tests,
production builds, and dependency audits. The first Desktop-tag scan found five
newly reachable `x/net/html@v0.54.0` advisories; upgrading to fixed
`x/net@v0.55.0` and resolved `x/sys@v0.45.0` returned both final scans to zero.
The code audit also fixed repeated stale-cursor fallback, non-canonical source
`RequestURI`, and the narrow native restore/shutdown race. The final unsigned
19,572,224-byte GUI has SHA-256
`f26ea87f42701a7eba8efa789900ea6953ef3c1533ff95106ec4b8e6b02b1160`.
No unresolved high- or medium-severity issue is known.

## Follow-Up

Desktop D1 should introduce its first product mutation as a narrow Go HTTP
control route with a distinct capability token, exact typed body, idempotency,
state and lease fencing, auditable events, and frontend tests. The Wails native
bridge must not become a parallel business control plane. Windows 10 real-machine
startup/recovery remains required before a formal portable or signed release.
