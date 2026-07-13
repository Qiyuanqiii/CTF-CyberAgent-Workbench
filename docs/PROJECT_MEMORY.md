# CyberAgent Workbench Project Memory

Last updated: 2026-07-13

## Resume First

CyberAgent Workbench is a local-first, resumable, auditable AI Agent workbench. Go is the only control plane. TypeScript consumes Go-owned HTTP/OpenAPI contracts, and future Rust analyzers must run as deterministic JSON tools behind Go. CTF-specific solving remains deferred until the generic runtime, Skills, Sandbox, and analyzer boundaries are stable.

Read in this order after a long context break:

1. `README.md`
2. This file
3. `docs/PROJECT_STATUS.md`
4. `docs/PROGRESS_BOOK.md`
5. `docs/TASK_BOOK.md`
6. `docs/adr/0001-go-control-plane.md`
7. `docs/adr/0002-run-centric-runtime.md`
8. `docs/adr/0003-run-execution-modes.md`
9. `docs/adr/0004-plan-delivery-workflow.md`

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v42.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v42 adds the Go-owned `plan_delivery.v1` protocol on top of immutable schema v41 modes. During a root Plan turn, the model may call `plan_delivery_propose` to persist exactly three bounded directions. Each direction contains 1-8 ordered modules, explicit acceptance criteria, bounded tradeoffs, and only backward dependency references. The tool is bound to the current root Agent, Supervisor attempt, execution lease, Run scope, Plan revision, Policy, and tool budget. Its result always keeps selection, phase change, and execution unauthorized.

After the proposal has paused and released the execution lease, the operator is the only actor that may choose direction 1, 2, or 3. `run plan choose` requires an exact normalized 16-256-byte operation key and a bounded operator identity. Go atomically writes the immutable selection, the chosen WorkItem dependency graph, a pinned decision Note, digest-only operation facts, and metadata-only Run events. Same-key concurrent calls across two SQLite Stores converge; changed intent conflicts. The Run remains paused in `plan`, and a separate `run phase ... deliver` operation is required.

CLI adds `run plans`, `run plan show`, `run plan choose`, and `run plan selection`. HTTP/OpenAPI, generated TypeScript DTOs, the React Run overview, and the Bubble Tea Plan tab expose read-only state. They omit operation keys, lease/fencing identity, requester internals, and model text, and explicitly project `capability_grant=false`. The embedded cross-Profile `plan-delivery` Skill is guidance only; the hard Go protocol remains authoritative even when that Skill was not selected.

The slice audit found no unresolved high- or medium-severity issue. Hardening rejects protocol-version whitespace, duplicate dependency references, duplicate direction/module titles in both Go and SQLite, malformed operation identities, active-lease selection, stale Plan revisions, direct SQL update/delete, and selection replay drift. Phase fencing now rejects `plan_delivery_propose` before Tool Gateway admission or budget use outside Plan; internal proposal fingerprints no longer enter events or handoff Notes; CLI model text is forced onto one terminal-safe line. Stable numbered handoff titles plus domain-level byte checks guarantee every accepted direction can produce a durable Note even at protocol bounds. The transaction and migration suites cover v41-to-v42 upgrade, rollback, tamper resistance, cross-store concurrency, HTTP privacy, TUI mutation denial, and React read-only rendering.

The complete local release gates, deterministic generation checks, credential/runtime scans, and isolated real-binary smoke passed; exact commands and counts are recorded in the final schema-v42 entry in `docs/PROGRESS_BOOK.md`. GitHub CI is verified after the commit is pushed. No real Provider key, host command, network target, or Sandbox execution belongs in this slice.

## Next Slice

Build schema v43 Delivery audit gates over the selected v42 WorkItems without creating another task engine:

1. Bind each delivery checkpoint to one selected WorkItem, its acceptance criteria, exact source selection, and current Deliver-mode revision.
2. Require focused verification, diff/security audit, and a compact durable handoff Note before that slice can complete.
3. Require a broader functional and robustness gate when a larger module boundary is reached, then let existing Run completion checks consume those facts.
4. Keep model assertions non-authoritative: Go owns gate state, evidence references, phase, budgets, and completion.
5. Audit Specialist Skill minimization separately after the single-root Delivery loop is stable.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
