# ADR 0026: Run Execution Profile Selection

## Status

Accepted for schema v64. This decision adds a selection control plane only;
real Local and container-process execution remain disabled.

## 中文摘要

每个 Run 现在都有一份由 Go 定义的不可变执行环境档位：`preview`、
`docker` 或 `local`。新建和迁移 Run 默认使用 `preview`。操作者只能在
Run 为 `created`，或处于没有活动 execution lease 的 `paused` 状态时切换。
CLI 与使用独立 control token 的本地 Web 控制台调用同一个 Application/Store
状态机。

档位只记录操作者意图，不是执行许可。schema v64 在 Go 与 SQLite 中把
`process_enabled`、`execution_authorized` 和 `capability_grant` 固定为
`false`。选择 `docker` 仍需通过独立生产启动门；选择 `local` 仍需先实现并
审计 OS sandbox。模型、child Agent、TypeScript、审批和文档内容都不能放宽
这些位。

## Context

The product needs a familiar user choice between preview-only behavior,
container isolation, and host-workspace execution. Exposing a runner switch
directly in TypeScript or mapping selection to process launch would collapse
the existing Policy, approval, lease, and Sandbox gates into one UI action.
The selection therefore has to be a durable Go-owned domain fact that remains
strictly weaker than execution authority.

## Decision

`run_execution_profile.v1` defines three closed mappings:

| Profile | Backend | Approval | Filesystem | Network | Risk | Required gate |
| --- | --- | --- | --- | --- | --- | --- |
| `preview` | `noop` | `none` | `none` | `disabled` | `minimal` | `none` |
| `docker` | `docker` | `always` | `workspace` | `disabled` | `elevated` | `docker_production_start_gate` |
| `local` | `local` | `always` | `workspace` | `disabled` | `high` | `local_os_sandbox_gate` |

All three mappings fix process, execution, and capability authority to false.
The profile enum is the only operator-selectable field. Backend, approval,
filesystem/network scope, risk, required gate, and authority bits are derived
by Go and independently constrained by SQLite.

Every Run receives revision 1 atomically with creation. Migration backfills
legacy Runs with a preview compatibility snapshot without fabricating historical
Run events. Later changes append an immutable snapshot, a digest-only idempotency
operation, and one metadata audit event in a transaction. Same-key/same-intent
replay returns the original snapshot; key reuse for another intent fails.
Concurrent writers serialize through the Run write lock and revision check.

## HTTP And Web Boundary

The loopback API exposes exactly one additional control operation:
`POST /api/v1/runs/{run_id}/execution-profile`. It requires the distinct
control bearer, one `Idempotency-Key`, strict `application/json`, a 4 KiB body
limit, no query parameters, and a request containing only `profile` plus an
optional redacted reason. The read bearer cannot mutate, and the control bearer
cannot read resources.

The React console keeps both credentials only in page memory. It sends the
control credential only in the Authorization header and submits only the profile
enum. Controls are disabled without that credential, outside the allowed Run
states, or while an execution lease is active. Browser state is convenience,
not the security boundary; Go and SQLite repeat every check.

## Security Consequences

- Selecting Docker does not contact Docker, create or start a container, or
  satisfy any v51/v63 production blocker.
- Selecting Local does not invoke `LocalRunner` and cannot expose an unrestricted
  host shell. A separately implemented OS sandbox is mandatory first.
- Approval cannot turn a selected profile into authority. Existing permanent
  Policy denials, Scope, budgets, Tool Gateway, and Sandbox gates remain in force.
- A child Agent cannot select or widen a parent Run profile. TypeScript and
  future Rust analyzers remain behind the Go control plane.
- Initial profile snapshots do not add events, preserving legacy lifecycle
  sequence compatibility; actual transitions remain auditable.

## Validation

Tests cover closed profile mappings, authority tampering, migration backfill,
immutable SQL rows, idempotent replay, conflicting key reuse, active-lease and
Run-state rejection, CLI output, HTTP credential separation and strict JSON,
OpenAPI/live-route coverage, memory-only browser tokens, disabled controls, and
React adoption of only the server-returned snapshot.

## Follow-Up

Docker process lifecycle, production evidence, output export, and Artifact
commit remain separate release gates. Local execution remains unavailable until
an OS-level sandbox is implemented and audited. The content-addressed external
Skill Registry previously planned for v64 moves to v65 or later.
