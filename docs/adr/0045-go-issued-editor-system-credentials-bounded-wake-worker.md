# ADR 0045: Go-Issued Editor, System Credentials, And Bounded Wake Worker

- Status: Accepted
- Date: 2026-07-19
- Scope: non-schema D1-I1, D1-M3, and D1-J1

## Context

The Windows/Desktop workbench needs three ordinary product capabilities that touch
different trust boundaries: interactive file editing, Provider credential setup, and
automatic consumption of an already scheduled wake intent. Combining their authority
would let renderer code write files, read keys, or start an unbounded Agent loop. Each
capability therefore has its own default-off Go gate and threat model.

## Decision

### D1-I1: Go-issued FileEdit proposal source

Assets are Workspace file integrity, file confidentiality, Run/Session identity, and
the existing review/apply separation. The adversary is untrusted renderer state,
stale or replayed handles, a changed file, injected document instructions, and a
request copied to another Run.

Go alone accepts a canonical Workspace-relative path and reloads the exact running
Run, Mission, active Session, and registered Workspace. Source is issued only when the
file is complete, untruncated, valid UTF-8, and contains no redaction marker. The
process keeps a digest of a cryptographically random 256-bit handle with a five-minute
expiry, a 128-entry ceiling, the exact bindings, and the original SHA-256. The handle
is reserved for one immutable proposal intent, so replay or concurrent intent drift
fails closed.

Monaco and its language workers are locally bundled and loaded lazily. The renderer
receives text and an opaque handle, never a host path. Submission contains only the
protocol version, Run ID, handle, and proposed text. Go reloads all bindings, rejects
secret-like content, rechecks Policy and the current file hash, and creates only a
pending `proposed` FileEdit. It cannot approve, apply, or directly write the file.

Residual risk: source text necessarily exists in renderer memory while the editor is
open. Handle state is process-local and is lost on restart, while a successfully
created FileEdit remains durable. A changed/redacted/truncated file requires a fresh
source issue. These constraints are intentional.

### D1-M3: Go and OS-owned Provider credentials

Assets are Provider API keys and the guarantee that status, logs, SQLite, events,
model context, and frontend persistence contain no plaintext. The adversary is a
renderer attempting readback, cross-capability token reuse, malformed provider names,
oversized secrets, storage failures, and accidental serialization.

Windows stores optional `mimo`, `deepseek`, and `anthropic` keys as generic credentials
under `CyberAgentWorkbench/provider/<provider>` in Windows Credential Manager. The
bound is 2,560 bytes, matching the documented Windows generic credential BLOB limit.
Non-Windows builds report an unavailable system store and have no plaintext-file
fallback. Environment variables retain priority during Provider Registry startup.

The read route returns only provider, configured state, store kind/availability,
`plaintext_returned=false`, and restart requirement. The independent control route
accepts exact `set` or `delete` requests with explicit confirmation. It never returns
the secret. Request buffers and mutable copies are cleared on a best-effort basis;
one Provider credential read failure disables that Provider instead of denying the
entire application or the mock Provider.

Residual risk: a secret necessarily exists transiently in the password input, request
body, Go process memory, Windows API call, and Provider client bootstrap. Memory
clearing is defense in depth, not a guarantee against a fully compromised process.
Credential changes currently require process restart so Registry replacement cannot
race an active model call. Windows account compromise remains outside this boundary.

### D1-J1: default-off bounded wake worker

Assets are token/model-time budget, Run lifecycle, cancellation, execution leases,
and the absence of hidden process authority. The adversary is an unbounded polling
loop, duplicate workers, restart races, a due-intent flood, or reuse as a Shell/Local/
Docker launch path.

The worker starts only when `--enable-wake-worker` is explicitly supplied and a
distinct control token is configured. One process owns one serial worker. Polling is
hard-bounded to 250 milliseconds through 60 seconds, with a two-second default. Each
tick asks for at most one due intent and submits exactly one step to the existing
Foreground Run Wake Consumer. That consumer continues through RunSupervisor, Policy,
budgets, checkpoints, leases/fencing, durable wake ownership, and cancellation.

The worker owns no Tool Runner and has no Shell, LocalRunner, Docker, network-scope, or
file-write dependency. Shutdown cancels and waits for the process-lifetime goroutine;
a closed Desktop control plane cannot start it again. Multiple application processes
may poll, but durable claim/fencing selects the effective owner.

Residual risk: enabling the worker can incur Provider cost for a valid due intent and
therefore remains an explicit operator startup decision. Polling from multiple
processes creates bounded extra database reads. It is not a Windows service, startup
task, persistent daemon, or general scheduler.

## Consequences

- TypeScript remains an interface, not a security boundary.
- Proposal, review, apply, credential, wake-intent, foreground-consume, and worker
  capabilities remain independent.
- No SQLite migration is required; schema remains v77.
- Desktop receives exact capability bits from Go bootstrap. Ordinary `api serve` React
  conservatively hides these three new controls until D1-J2 adds an authenticated,
  metadata-only capability projection; directly authorized HTTP routes still work.
- Real Shell, LocalRunner, Docker execution, xterm, install hooks, and CTF automation
  remain separately blocked.

## Verification

The final ordinary integrated gate passed the uncached full Go suite in 327.6 seconds,
`go vet ./...`, secure Desktop-tag tests, deterministic OpenAPI/TypeScript generation,
strict TypeScript, 102 React tests across 28 files, Vite production build, and a
zero-vulnerability npm audit. The reproducible unsigned Windows build has SHA-256
`a0e6aa0a3d15ccc39712f8a0a64d7de06e4a6af426e060b6378b1011c93a1cf6` and correctly
reports `release_ready=false`.

The combined audit fixed exact provider normalization, the Windows credential-size
bound, application-wide failure from one credential read error, restart-after-close
for the Desktop worker, FileEdit ID/intent drift after an uncertain save, internal
errors on post-review replay, `models:null` for unconfigured Providers, frontend
secret-test snapshotting, and Monaco CDN/dependency risk. Desktop and 390x844 mobile
UI smoke pass. No unresolved high- or medium-severity issue is known. No real API key,
Provider network request, Shell, LocalRunner, or Docker operation was used.

GitHub Actions run `29671519260` passed implementation commit `ee36405`: TypeScript
42s, Windows Desktop 2m31s, and Go control plane 3m54s.

## 中文结论

本决策把编辑、密钥和自动唤醒拆成三条互不借权的 Go 边界。Monaco 只能使用
Go 发放的短期句柄创建待审 FileEdit，不能直接写文件；Windows 凭证接口只能
设置、删除和读取配置状态，界面不能回读明文；wake worker 默认关闭，每轮最多
消费一条到期意图并执行一个 Supervisor step，且没有 Shell、LocalRunner 或
Docker 入口。三项能力提升了日常可用性，但不改变真实进程执行仍被关闭的结论。
