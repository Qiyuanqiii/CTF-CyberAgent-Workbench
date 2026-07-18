# ADR 0040: Provider Diagnostics, Diff Review, And Wake Intent

- Status: Accepted
- Date: 2026-07-18
- Scope: non-schema D1-M2/D1-D1 and schema-v74 D1-Q1

## Context

The workbench could inspect redacted model availability, create file-edit proposals,
and explicitly hand a frozen input batch to the RunSupervisor. Operators still lacked
three ordinary controls:

1. explicitly test a configured Provider and persist a model route;
2. review a bounded Diff without immediately writing the workspace; and
3. persist restart-safe wake/retry intent without creating a hidden worker.

These controls must remain Go-owned. TypeScript must not receive Provider secrets,
file bodies, lease ownership, or process authority.

## Decision

### D1-M2: explicit content-free Provider diagnostics

`model_route_control.v1` validates and persists a route before updating the in-memory
Router. Route mutations serialize the durable write and memory update so concurrent
selections cannot reorder them. Router access is concurrency-safe and no lock is held
across a Provider call.

`provider_diagnostic.v1` runs only after an explicit operator action. It sends one
bounded, content-free, tool-disabled request with a 15-second deadline. The public
result contains protocol, provider/model identity, status, retryability, whether
network/model execution was attempted, and duration. It fixes tool use and returned
content false and exposes no response text, key, endpoint, environment-variable name,
client, or raw error. Each action can still consume one Provider request and incur a
small charge.

### D1-D1: review is not apply

The file-edit review service reloads the exact Run, Mission, Session, Workspace,
proposal, and durable approval. Public list/detail responses contain metadata and a
bounded redacted Diff only; the exact HTTP read uses a SQLite Preview projection that
never selects stored original/proposed file bodies.

`approve_intent` records the durable approval and moves the proposal to `approved` but
does not write the workspace. `deny` records the matching terminal decision. Apply
remains a separate operation. The approval decision commits before edit state; if a
process stops in that window, a same-outcome retry repairs edit state. An opposite
decision, cross-Run binding, terminal Run mutation, or stale identity fails closed.

### D1-Q1: durable wake ownership without execution

Schema v74 adds one active `run_wake_control.v1` intent per Run plus digest-idempotent
schedule/cancel operations and generation-fenced ownership. Go and SQL bound attempt
count, first-wake time, exponential backoff, deadline, lease duration, takeover, and
terminal states. One generation owns an intent at a time; a stale owner cannot record
the next result.

The public API exposes scheduling, cancellation, and a closed-authority state
projection. It omits lease ID, owner, fencing identity, and operation keys and fixes
background-loop and execution authority false. No goroutine, service, model/tool call,
Run execution lease, or automatic Run transition is introduced.

## Desktop And Protocol Boundary

Desktop adds independent `--enable-model-control`, `--enable-file-edit-review`, and
`--enable-run-wake` capabilities. They share the existing ephemeral control bearer but
cannot unlock sibling routes. The Wails native bridge remains exactly `Bootstrap`,
`SelectSkillPackage`, and `PreviewSkillPackage`.

React performs exact allowlist parsing and rejects model content, file bodies, write
authority, lease ownership, or wake execution claims. TypeScript remains a client of
Go's HTTP/OpenAPI protocol and never talks directly to Providers, SQLite, files, or a
scheduler.

## 中文结论

本批把“显式在线诊断”“只审阅不落盘”和“只记录 wake 意图不后台执行”做成三个独立
Go capability。模型诊断每次可能产生一次小额请求，但不返回模型正文或密钥；Diff
批准只形成意图，真正 apply 仍需独立授权；v74 只解决重启恢复、退避和单 owner
fencing，不启动隐藏 worker。TypeScript 不获得 Provider、文件、lease 或进程权限。

## Verification

The final cumulative six-slice gate passed full ordinary/race suites in 278.6s/296.1s,
ordinary and secure-Desktop static/vulnerability checks, 80 React tests, TypeScript and
Vite/Windows production builds, deterministic OpenAPI, isolated CLI smoke, dependency
verification, and repository privacy/encoding/link/artifact/process-entry scans. The
audit additionally fixed expired-final-lease event ordering and rejected a first-wake
delay beyond the total elapsed budget. No live Provider, process, Docker, or file apply
was used.

## Consequences

- SQLite advances from v73 to v74 only for durable Run wake ownership.
- Provider routes and diagnostics reuse existing Provider settings and model ledgers;
  no key is persisted by these controls.
- Review and apply are now explicit separate stages. A future apply control must recheck
  current-file hash, workspace scope, Policy, idempotency, and write result.
- A future wake consumer must be independently enabled, budget-aware, cancellable, and
  fenced through the existing RunSupervisor path. Schema v74 alone authorizes none of it.
