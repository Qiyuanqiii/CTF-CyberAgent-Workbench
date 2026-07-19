# ADR 0046: Safe Editor Recovery, Provider Generations, And Worker Health

- Status: Accepted
- Date: 2026-07-19
- Scope: non-schema D1-I2, D1-M4, and D1-J2

## Context

D1-I1/M3/J1 made editing, system credentials, and bounded wake consumption useful,
but left three recovery and visibility gaps. An expired editor handle could strand a
renderer draft, a changed credential required a process restart, and ordinary Web
clients could not discover the exact capability set or observe worker shutdown. Each
gap crosses a different trust boundary and must be closed without granting new
filesystem, secret-read, scheduler, or process authority.

## Decision

### D1-I2: safe FileEdit recovery

An expired source handle may be reissued only when Go reloads the exact running Run,
active Session, registered Workspace, and canonical relative path, then proves that
the current file hash equals the digest from the previous source issue. The request
contains no renderer draft. A changed file remains a typed stale conflict instead of
silently rebasing user text.

One durable pending FileEdit may also be recovered through
`file_edit_proposal_recovery.v1`. Go verifies Run/Session/Workspace ownership, pending
status, stored original/proposed hashes, content integrity, and the current Workspace
hash. The response is a read-only Diff projection with `editable=false` and
`review_required=true`; it carries no source handle and cannot approve, apply, create,
or reopen a proposal. A pending create-file proposal uses the closed `missing`
original-hash sentinel only with empty original content.

### D1-M4: generation-safe Provider Registry reload

The Provider Registry now owns monotonically increasing generations. After a supported
system credential is set or deleted, Go constructs a complete candidate Registry,
loads and validates persisted routes, and reads every required system credential
before one atomic swap. A route or credential-read failure leaves the old generation
untouched and returns an error. The response remains status-only and reports
`registry_reloaded`, `registry_generation`, and `restart_required=false` only after a
successful swap.

Router resolution captures the route and immutable Provider instance under one read
lock before releasing it for the call. Calls already in flight therefore finish on
their captured old Provider, while later calls use the new generation. TypeScript
cannot submit a generation, Provider object, endpoint, or plaintext read request.

### D1-J2: read-only capabilities and bounded worker health

Authenticated `GET /api/v1/capabilities` returns the exact Go capability bits plus
`run_wake_worker_health.v1`. Worker state is a closed set of `disabled`, `ready`,
`running`, `draining`, and `stopped`, with fixed concurrency one and maximum one step.
The projection omits token, operation, owner, lease, Run identity, and private error.
It fixes process, Shell, and Docker execution to false and explicitly reports that
runtime enablement and persistent-service installation are unsupported.

The worker is process-lifetime and one-shot. All public `RunOnce` callers are
serialized, nil context is rejected, cancellation fans into `draining`, and shutdown
waits for the active bounded call before `stopped`. API construction requires a
distinct control token whenever the worker is enabled even though capability and
health reads themselves use the read token. React may use the projection to show or
hide controls, but Go routes independently enforce every mutation.

## Consequences

- No SQLite migration is required; schema remains v77.
- Renderer draft recovery does not bypass proposal/review/apply separation.
- Provider credential changes no longer require restart when the generation swap
  succeeds; failure preserves the currently serving generation.
- Ordinary Web and Desktop share the same authority description without making
  TypeScript a security boundary.
- Real Shell, LocalRunner, Docker execution, xterm, install hooks, and CTF automation
  remain separately blocked.

## Verification

The cumulative six-slice gate passed the final uncached ordinary and race Go suites in
322.9s and 352.8s, `go vet`, zero-warning staticcheck, zero-finding govulncheck, module
verification/tidy, secure Desktop-tag tests, strict TypeScript, 108 React tests across
29 files, deterministic OpenAPI/TypeScript generation, Vite production build,
zero-vulnerability npm audit, and a reproducible Windows double build. OpenAPI has 57
paths, 61 operations, and 125 schemas. The unsigned Desktop binary SHA-256 is
`30a3d9d19e02f32f8ea976fc071bc6942ed06fba3e7cad937310a78e46e74dfc`;
automated checks pass while `release_ready=false` remains correct.

The combined audit fixed mixed route/Provider generations, credential-read failure
replacing the active Registry, false failure under concurrent generation advances,
inconsistent generation values within one status list, worker construction without a
control token, concurrent public `RunOnce` calls, nil worker context, recovery of a
pending missing-file proposal, and recovery-dialog close/error behavior. Real-browser
desktop/mobile smoke found no horizontal overflow or console errors. No unresolved
high- or medium-severity issue is known. No real API key, Provider request, Shell,
LocalRunner, Docker, attack traffic, or external network operation was used.

## 中文结论

本决策补齐三条恢复链路而不扩大权限。编辑器只能在原文件哈希未变化时换发短期
句柄；持久化提案恢复只是不可编辑的 Diff，不能审批或写文件。Provider 凭证修改后
由 Go 构建完整候选 generation 并原子切换，失败继续使用旧 generation，正在运行的
调用不被中断。普通浏览器和 Desktop 可读取同一份 capability/worker health，但该
投影不能运行时开 worker、安装服务或取得 Shell、LocalRunner、Docker 权限。
