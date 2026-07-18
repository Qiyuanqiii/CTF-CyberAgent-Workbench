# ADR 0038: Idempotent Run Control And Bounded Handoff

Status: Accepted, 2026-07-18

## Context

Desktop D1-S1 could durably enqueue input, but the product still lacked three operator boundaries needed for an ordinary resumable Agent workflow: cancelling an item that had not entered delivery, moving a Run through explicit lifecycle states, and handing a bounded queue selection to the existing RunSupervisor. Coupling those actions would let a message submission silently start work, while implementing execution in TypeScript or the native shell would create a second control plane.

中文结论：取消、生命周期和执行交接是三项独立能力。Go 冻结输入、检查状态、持有私有 lease、调用既有 Supervisor 并写审计事实；TypeScript 只能提交有界意图和显示有界结果。这里的“执行”不等于开放 Shell、LocalRunner 或 Docker 进程。

## Decision

1. Non-schema protocol `session_steering_cancellation.v1` exposes exact pending-only cancellation through `POST /api/v1/sessions/{session_id}/messages/{message_id}/cancel`. Go reloads the exact Session, Run, and message binding and reuses schema-v46 cancellation facts. A prepared, committed, cancelled, or otherwise non-pending item cannot be rewritten.
2. Public steering projections include only a derived `prepared` boolean in addition to bounded lifecycle metadata. This is a read-only Store derivation, not a mutable column. React offers cancellation only when `status=pending` and `prepared=false`; Go and SQLite remain authoritative if state changes concurrently.
3. Schema v73 protocol `run_lifecycle_control.v1` adds an immutable digest-idempotency operation for `start`, `pause`, and `resume`. Start performs the existing `created -> preparing -> running` sequence. Pause and resume perform `running -> paused` and `paused -> running` respectively. Pause requires a quiescent Supervisor, no active execution lease, no active Agent, and no prepared steering delivery.
4. Lifecycle same-key/same-intent replay returns the immutable original operation plus the Run's current status. Later valid transitions do not invalidate an earlier replay. Changed intent conflicts, and two SQLite connections converge on one operation and one transition event range.
5. Schema v73 protocol `run_execution_handoff.v1` accepts one Run, operator identity, idempotency key, and `max_steps` from 1 through 8. Its preparation transaction freezes the first matching pending steering identities and sequences before private execution begins. Messages appended later cannot enter that operation.
6. An empty selection completes as `queue_empty` without acquiring a lease or calling a model. A non-empty selection uses the existing RunSupervisor, Run execution lease, Policy, cumulative budgets, model ledger, Tool Gateway, checkpoints, and event stream. It skips a selected item that is no longer pending and stops on root `finish`, root `wait`, selection exhaustion, or a typed failure.
7. Handoff completion is fenced to the exact private lease ID and generation. Terminal replay must reproduce the exact status, Run state, stop/error code, steps, selected-state counts, and model/tool intent. A stale lease or changed terminal intent conflicts instead of being accepted as replay.
8. `POST /api/v1/runs/{run_id}/lifecycle` and `POST /api/v1/runs/{run_id}/execute` require the distinct control bearer, strict JSON, no query, and exactly one bounded `Idempotency-Key`. The execution response is metadata-only: protocol, Run/Session, counts, status, stop reason, replay, and model/tool-called booleans. It cannot represent queued content, model output, tool arguments, raw operation keys, or lease/fencing identity.
9. Desktop exposes cancellation, lifecycle, and handoff only through independent `--enable-session-steering-control`, `--enable-run-lifecycle`, and `--enable-run-execution` flags. The Wails native bridge remains exactly three methods; all operations traverse the authenticated in-process Go HTTP Handler.
10. Browser retry identities remain memory-only and are intent-bound. Editing a Session message, changing lifecycle action, or changing `max_steps` rotates the key. Tokens and operation keys never enter a URL, browser storage, SQLite plaintext, or public events.

## Authority Boundary

- Lifecycle control grants no process, Shell, Docker, LocalRunner, network, file-write, approval, Skill-installation, or child-scheduling authority.
- A bounded handoff may call the configured Provider and only the existing Supervisor-approved structured tools. Process tools and real Local/Docker execution remain disabled by their independent Go gates.
- The native shell does not own a worker, lease, scheduler, model router, Policy checker, or retry loop. Closing the window does not create a new execution path.
- This decision adds an explicit operator action, not a restart daemon or background wake/retry scheduler. Automatic queue pickup remains a later opt-in design.
- Model output, repository files, Skill text, and other external content cannot invoke these control routes or supply their bearer/idempotency identity.

## Verification

Focused tests cover exact Session/Run/message binding, pending/prepared races, cancellation replay/conflict, lifecycle state and quiescence gates, delayed replay after later transitions, two-connection convergence, immutable v73 rows, historical v72 upgrade, frozen queue identity, append isolation, selected cancellation, zero-selection behavior, stale-lease fencing, changed terminal replay, strict HTTP/DTO privacy, independent Desktop capabilities, and React uncertain-failure key rotation.

The cumulative six-slice gate passed the final code's full ordinary suite in 268.2 seconds and full race suite in 295.3 seconds. Ordinary and secure-Desktop vet/staticcheck, both govulncheck dependency graphs, module verification/tidy diff, deterministic OpenAPI/TypeScript generation, strict TypeScript, 66 frontend tests across 16 files, Vite and Windows production builds, zero-vulnerability npm audit, and repository privacy/artifact/forbidden-entry scans are green. The unsigned Windows binary is 20,849,664 bytes with SHA-256 `ce3ff2b4609068de996b6362e3a5008c4d2348eae73c48ad0661c4e22739eba5`.

The combined audit fixed delayed lifecycle replay validation, misleading cancellation UI for prepared items, stale-lease and changed-intent handoff completion replay, retry-key reuse after `max_steps` changes, and a frontend test that inspected the wrong mock argument. No unresolved high/medium issue is known.

## Consequences

A user can now create a Run, explicitly start it, enqueue or cancel pending input, execute a bounded frozen batch through the real Go Agent runtime, and pause or resume it from Web/Desktop without granting the renderer process authority. The remaining product gap is not another execution engine: it is an audited background scheduling policy, richer Plan/approval/Diff/Skill mutations, Desktop provider configuration, and eventually separately gated Sandbox process execution.
