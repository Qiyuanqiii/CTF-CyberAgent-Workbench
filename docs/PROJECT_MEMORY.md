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
- Database schema: v40.
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

The third P7 Skills vertical slice adds schema v40 and root-only `skill_context.v1`. Every Supervisor turn loads the Run's persisted `skill_selection.v1`, then the immutable embedded Registry rechecks exact name/version/content-hash/byte/token tuples and Profile before exposing any text. Delivery is redacted, stable-name ordered, and bounded by the selection's independent token budget. The four built-in workflow guides are now useful version `1.1.0` content instead of stale metadata placeholders. A hard-bounded embedded history retains at most eight versions per Skill: new selection sees only current entries, while old `1.0.0` selections resume against their exact original body.

Preparation is persisted before the model call and committed in the same SQLite transaction as the first `model.started`. Replay through another Store connection converges on the same preparation; reconstructed context drift conflicts, missing preparation fails model start closed, and a failed model-start event rolls the commit back. The two v40 tables and `skill.context_prepared/committed` events contain aggregate provenance only. Skill text exists only in the in-memory Provider request; text, paths, Skill names, versions, content hashes, and declared tool dependencies never enter the v40 ledger or events.

Skill guidance is subordinate to the root system policy and does not alter the offered tool list. Models, HTTP, the Tool Gateway, and child scheduling still cannot select Skills; Specialist contexts receive none. A missing or mismatched Registry fails before Provider invocation. Focused tests cover deterministic current and archived assembly, redaction, tampering and Registry drift, SQL immutability, atomic commit/rollback, cross-Store replay, v39 upgrade, prompt delivery, metadata-only persistence, no tool grant, and fail-closed Registry loss. Full repository gates and CI are recorded in `docs/PROGRESS_BOOK.md` for the slice.

## Next Slice

Implement the fourth P7 Skills vertical slice as a bounded Plan/Delivery workflow inspired by strong coding-agent product patterns without copying proprietary implementation:

1. Add one embedded cross-Profile planning Skill with three concise operator-selectable directions, a strict bounded plan shape, and no new tool authority.
2. Project accepted modules into existing WorkItems and durable handoff Notes instead of introducing a second task engine.
3. Advance one slice at a time; after each slice require focused verification, a diff/security audit, and a compact handoff update.
4. After each larger module, require broader functional and robustness gates before completion.
5. Keep Go authoritative for workflow state and budgets; Skill text remains guidance, and Specialist Skill minimization follows as a separate audited slice.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
