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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v39.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

The second P7 Skills vertical slice adds schema v39 and a Go-owned immutable `skill_selection.v1`. An operator may select one to eight Skills only while a Run is `created`; the Registry resolves a deterministic name order and pins each version, content SHA-256, UTF-8 byte count, and conservative token upper bound. One Run has at most one selection. Go and SQLite independently enforce Mission/Profile binding, contiguous items, aggregate budget, immutable rows, and one digest-only idempotency operation. Exact replay remains valid after Run start and across Registry content/version drift, while changed intent conflicts.

The only write entry is operator CLI `skill select`; `skill selection` is read-only. Models, HTTP, the Tool Gateway, and child scheduling cannot select Skills. Events contain protocol/Profile/count/budgets and explicit `context_injection=false` / `tool_capability_grant=false`, never Skill names, content, paths, hashes, dependencies, or raw operation keys. No Skill content enters a Provider prompt and no tool authority is granted.

The audit found no unresolved high/medium issue. It fixed four robustness gaps: actionable CLI validation reasons were hidden, replay depended on the current Registry result, duplicate event JSON fields were accepted ambiguously, and historical migration fixtures did not remove v39. Focused tests cover deterministic resolution, Profile/budget/duplicate rejection, underreported accounting, immutable SQL, metadata-only events, rollback, v38 upgrade, post-start replay, Registry-drift replay, and eight concurrent callers over two SQLite connections; the concurrency test also passed 20 repeated runs. Verified gates: uncached full tests, full-repository race detection, `go vet`, zero-warning `staticcheck`, module verification/tidy diff, `govulncheck` with zero findings, OpenAPI/TypeScript drift, 15 Vitest tests, production build, npm audit with zero findings, tracked credential/runtime scans, and an isolated real-binary schema-v39 selection/replay smoke.

## Next Slice

Implement the third P7 Skills vertical slice:

1. Assemble context only from the Run's persisted selection and the immutable embedded Registry.
2. Reverify exact name/version/content-hash/byte tuples before every prepared delivery.
3. Redact and bound selected Skill text under a separate context budget with deterministic ordering.
4. Persist metadata-only preparation/commit provenance without Skill text or paths in events.
5. Start with the root Supervisor only; grant no tools and leave Specialist Skill minimization for a later slice.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
