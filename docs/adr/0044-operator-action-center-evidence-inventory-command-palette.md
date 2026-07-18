# ADR 0044: Operator Action Center, Evidence Inventory, And Command Palette

- Status: Accepted
- Date: 2026-07-19
- Scope: non-schema D1-O1, D1-C2, and D1-K1

## Context

The Run workbench already exposed steering, approvals, FileEdit review/apply, wake
intent, Workspace evidence attachment, and durable receipts, but pending operator work
was distributed across independent views. Attached evidence was durable yet lacked a
bounded inventory, and keyboard users had no fast way to navigate the existing Run
surface. These usability gaps must close without creating a second authority plane in
TypeScript, exposing private operation identity, replaying evidence bodies, or turning
navigation into an automatic approval or execution path.

## Decision

### D1-O1: Go-owned bounded operator action center

`operator_action_center.v1` is a read-bearer projection for one exact Run. Store uses
bounded indexed queries and returns no more than 100 pending items across a closed set:
pending steering, pending approval, FileEdit review readiness, approved FileEdit apply
readiness, and due wake intent. Application reloads and verifies the Run, Mission,
Session, Workspace, and due time before projection.

Public IDs are domain-separated opaque hashes. Public kinds and destinations are
closed enums. Source row IDs, operation keys/digests, requesters, messages, commands,
arguments, paths, Diff/content, lease owners, and capability fields stay private. An
item can only navigate to an existing Run view. Listing never approves, denies,
applies, wakes, drains, or executes anything.

### D1-C2: metadata-only attached-evidence inventory

`session_evidence_inventory.v1` lists at most 100 immutable attachments for the exact
Run-bound active Session and Workspace. Each item contains only an opaque attachment
ID, public Run/Session/Workspace IDs, closed source kind, canonical source reference,
SHA-256, attachment time, and fixed `instruction_authorized=false`. Message IDs and
bodies, the attaching operator, private operations, event sequence, and capability
state are omitted.

React may open a source only by passing the Go-issued relative reference back to the
existing Workspace Explorer. Explorer performs its own path and Workspace checks. The
inventory does not replay content into a Session and cannot infer instruction,
approval, tool, process, network, or filesystem authority from evidence.

### D1-K1: navigation and refresh only

The `Ctrl+K` palette has a static, closed command list. Commands navigate among
existing Run tabs or invalidate the current Run's read queries. It submits no path,
content, command, idempotency key, approval decision, capability, or process request.
It cannot invoke a disabled Go control indirectly and does not persist command state in
browser storage.

### Event-stream robustness discovered by browser audit

Go's canonical event envelope version is `v1`. The TypeScript parser had required
`event.v1`, and its failure path released the reader lock without cancelling the
response body. Repeated reconnects could consume every browser per-origin connection
and leave unrelated views loading. The parser now uses `v1`; OpenAPI imports the Go
constant so generated TypeScript carries the literal `"v1"`; and SSE cancellation is
attempted on every parse or transport failure before reconnecting, while preserving
the original error.

## 中文结论

本批新增的是操作者工作台的聚合与导航，不是新的执行层。行动中心只聚合闭集待处理
metadata，证据清单只列出已经附加且固定非授权的来源事实，`Ctrl+K` 只负责导航和刷新。
TypeScript 不取得审批、文件、进程或后台任务权限。浏览器实测还修复了事件版本漂移和
失败重连未关闭响应体导致的连接耗尽问题；协议版本现在由 Go/OpenAPI/TypeScript 共用。

## Verification

The cumulative six-slice robustness gate passed the final uncached Go ordinary and
race suites in 319.6s and 299.8s. Ordinary and secure-Desktop tests, vet,
zero-warning staticcheck, zero-finding govulncheck, module verification/tidy, strict
TypeScript, deterministic OpenAPI/TypeScript generation, 97 React tests across 26
files, Vite production build, zero-vulnerability npm audit, isolated mock-only CLI
smoke, repository privacy/artifact/encoding/link scans, and a reproducible Windows
double build all passed.

OpenAPI contains 51 paths, 55 operations, and 116 schemas. Its deterministic SHA-256
is `B9CD79254D9AE09A2DB4BCC6268F04CA8F4ADD6C638E6BAA4DA42FC223A10181`.
The unsigned Desktop binary SHA-256 is
`a89b2357a5f1e7376ea8a533356028ccd5ea5eaec388b14d7623343fd041f520`;
automated Windows checks pass while `release_ready=false` remains correct.

Real-browser checks at 1440x900 and 390x844 verified the Actions and Evidence tabs,
command filtering/Enter/Escape behavior, local tab scrolling, no document overflow,
canonical source navigation, live SSE recovery, stable connection count, and zero
console errors. The combined audit found no unresolved high- or medium-severity issue.
No real Provider, API key, Shell, LocalRunner, Docker, external network, installer,
registry mutation, startup task, or updater was used.

GitHub Actions run `29665187925` passed implementation commit `1151aaf`: the
TypeScript console completed in 36s, the Windows Desktop shell in 2m23s, and the Go
control plane in 3m35s.

## Consequences

- SQLite remains at schema v77; these are non-schema read/navigation slices.
- Go remains the sole authority and owns all joins, validation, limits, and public
  protocol values.
- TypeScript receives no new mutation, capability, host path, process, or secret
  boundary.
- Evidence remains evidence, never an instruction or authority source.
- Signed/package distribution, the manual Windows 10 matrix, Provider-secret UI,
  background wake automation, real Sandbox/host processes, Rust analyzers, and CTF
  solving remain separately gated work.
