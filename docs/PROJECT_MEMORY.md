# CyberAgent Workbench Project Memory

Last updated: 2026-07-14

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
10. `docs/adr/0005-operator-steering-queue.md`
11. `docs/adr/0006-operator-steering-controls.md`

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v46.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v46 adds explicit controls over the schema v45 operator-steering queue without changing its delivery contract. `run steer cancel` creates one immutable cancellation fact plus a digest-only idempotency operation for an unprepared pending item. Exact retry returns the same fact, changed intent conflicts, and a prepared item cannot be cancelled. Editing and reordering remain unsupported. Failed/cancelled Runs create bounded `run_terminal` cancellation facts before closing all remaining deliveries and messages in the same transaction.

`run steer drain` is an explicit, bounded wake operation. It acquires the Run execution lease before resuming a paused Run, then uses a steering-only Supervisor begin path. It cannot create a default turn, recover an unrelated ordinary failed input, exceed the existing Run budget, or bypass Policy. A conflicting active lease leaves a paused Run paused. Ordinary Run-bound `session send --operation-key` always creates or replays durable steering and never performs a synchronous Provider call; replay works across Store/process restart and after the item commits.

CLI is the only new mutation surface. HTTP/OpenAPI, React, and TUI remain metadata-only and have no cancel/drain action; models and child Agents have no steering mutation path. Raw operation keys, operator cancellation reasons, and requester identity are omitted from Run events. The focused audit found no unresolved high- or medium-severity issue and fixed three low-risk behavior defects before release: terminal error text is now redacted, UTF-8 repaired, control-normalized, and truncated without being able to block a failed/cancelled Run transaction; wake now happens only after the drain owns the execution lease; and an explicitly supplied blank Session operation-key flag fails instead of degrading to a synchronous turn. One staticcheck options-conversion issue was also cleaned up. Focused tests additionally prove SQLite rejects cancellation without both the fact and operator operation ledger, drain never recovers non-steering failed input, concurrency converges, and Session history commits exactly once. Full local release gates pass. GitHub Actions run `29316182580` passed release commit `5559f76`: Go control plane completed in 1m51s and TypeScript console in 19s.

## Next Slice

Perform the independent Specialist Skill-minimization audit before widening autonomy:

1. Define a Go-owned Specialist Skill selection policy that derives the smallest compatible subset from the parent Run's pinned selection and the child assignment.
2. Keep Code and Cyber Skill catalogs distinct. Cyber Specialists receive only narrowly required coding guidance, primarily safe Python analysis support.
3. Persist metadata-only Specialist Skill provenance and recovery facts; never persist Skill bodies or let a child choose its own Skills.
4. Reuse the current root Skill Registry/version/hash validation and separate token budget, while preserving the two-child core limit and 1/2/4/6 read-only Fan-out boundary.
5. Keep Rust analyzers behind the later Go-controlled Sandbox/process JSON protocol; do not begin CTF solving or real command execution in this slice.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v46 code did not modify migrations 1-45, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
