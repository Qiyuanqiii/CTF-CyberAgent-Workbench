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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v44.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

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

Schema v44 adds immutable `delivery_checkpoint.v1` over schema v42 selected WorkItems. A checkpoint can be recorded only by the operator application/CLI for an `in_progress` selected item while its Run is paused in Deliver phase and has no active execution lease. It pins the proposal and selection, exact source module, acceptance fingerprint, source fingerprint, current mode snapshot/revision, and WorkItem version. Focused verification, diff review, security review, and handoff summary are mandatory; the deterministic final selected module also requires full functional verification and robustness review.

One transaction creates the checkpoint, a digest-only idempotency operation, an immutable pinned handoff Note, and metadata-only Note/checkpoint events. The existing WorkItem transition and both Supervisor/Run completion paths consume these facts. Go and SQLite independently require the exact current Deliver revision and item version. Checkpoint, operation, handoff Note, and all Note relation rows are immutable. Cross-process retries converge; changed intent conflicts. An old selection with any already completed/cancelled item remains explicitly unenrolled instead of receiving fabricated evidence.

CLI adds `run delivery checkpoint/list/show`. HTTP/OpenAPI, generated TypeScript, React, and TUI show only enforcement, required/ready counts, and bounded checkpoint metadata. They omit evidence, internal digests, operation keys, and requester identity. The model has no checkpoint tool; Policy denies obvious Shell/process/script/Sandbox self-invocation of the operator checkpoint command. This is defense in depth, while Local/Docker process execution remains closed.

The focused audit found no unresolved high- or medium-severity issue. It fixed three low-risk boundaries before release: handoff Note titles are now independent of maximum-length model module titles, relation-table update triggers check both old and new Note ownership, and checkpoint events no longer expose internal fingerprints/digests. Full release checks and GitHub CI results are recorded in the v44 progress entry after push.

## Next Slice

Build schema v45 durable operator steering queue without interrupting unsafe boundaries:

1. Accept bounded operator input while a Run is busy and persist it in order instead of returning only “busy”.
2. Deliver queued input exactly once at a safe root-turn boundary, never midway through an active model/tool commit.
3. Reuse PendingInput, Coordinator inbox, execution lease, cancellation, provenance, and idempotency semantics rather than creating a second message engine.
4. Expose queue state read-only first; keep model, HTTP, TypeScript, and child Agents unable to forge operator authority.
5. Keep Specialist Skill minimization as the following independent P7 audit, and keep Rust analyzers behind the later Sandbox/process protocol.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
